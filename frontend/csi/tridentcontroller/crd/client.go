// Copyright 2026 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	crand "crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	clik8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend/csi/tridentcontroller"
	. "github.com/netapp/trident/logging"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	clientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	tridentinformers "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/utils/models"
)

// Timing defaults are vars (not const) so unit tests can shorten waits.
var (
	backupAckPollBase      = 60 * time.Second
	backupAckPollJitter    = 10 * time.Second
	ackInformerSyncTimeout = 30 * time.Second
)

// Client implements tridentcontroller.Client and ChangeNotifier via TridentNode / publication CRDs.
type Client struct {
	kubeConfigPath   string
	tridentClient    clientset.Interface
	tridentNamespace string
	kubeClient       kubernetes.Interface
	clientMutex      sync.Mutex
}

// NewClient returns a CRD-backed controller client for node-side use. The Kubernetes client is created lazily.
func NewClient(kubeConfigPath string) *Client {
	return &Client{kubeConfigPath: kubeConfigPath}
}

func (c *Client) ensureTridentClient() error {
	c.clientMutex.Lock()
	defer c.clientMutex.Unlock()

	if c.tridentClient != nil && c.tridentNamespace != "" {
		return nil
	}

	clients, err := clik8sclient.CreateK8SClients("", c.kubeConfigPath, "")
	if err != nil {
		return err
	}
	c.tridentClient = clients.TridentClient
	c.tridentNamespace = clients.Namespace
	if c.kubeClient == nil && clients.KubeClient != nil {
		c.kubeClient = clients.KubeClient
	}
	return nil
}

// RegisterNode creates or updates the TridentNode CR and waits for controller acknowledgement.
func (c *Client) RegisterNode(
	ctx context.Context, node *models.Node, timeout time.Duration,
) (*tridentcontroller.RegistrationInfo, error) {
	if err := c.ensureTridentClient(); err != nil {
		return nil, err
	}
	nodeCR, err := tridentv1.NewTridentNode(node)
	if err != nil {
		return nil, err
	}
	nodeCR.Spec.ProvisionerReady = convert.ToPtr(false)

	// Apply may populate Status for dual-read conversions; status is controller-owned and must not
	// be established by the node on Create (controller sets it via UpdateStatus).
	nodeCR.Status = tridentv1.TridentNodeStatus{}

	created, err := c.tridentClient.TridentV1().TridentNodes(c.tridentNamespace).Create(
		ctx, nodeCR, metav1.CreateOptions{},
	)
	if err != nil && apierrors.IsAlreadyExists(err) {
		existing, getErr := c.tridentClient.TridentV1().TridentNodes(c.tridentNamespace).Get(
			ctx, nodeCR.Name, metav1.GetOptions{},
		)
		if getErr != nil {
			return nil, getErr
		}
		update := existing.DeepCopy()
		mergeTridentNodeRegistrationSpec(update, nodeCR)
		syncTridentNodeRootFieldsFromSpec(update)
		created, err = c.tridentClient.TridentV1().TridentNodes(c.tridentNamespace).Update(
			ctx, update, metav1.UpdateOptions{},
		)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	waitCtx := ctx
	cancelWait := func() {}
	if timeout > 0 {
		waitCtx, cancelWait = context.WithTimeout(ctx, timeout)
	}
	defer cancelWait()

	ack, err := c.waitForRegistrationInfo(waitCtx, created, timeout)
	if err != nil {
		return nil, err
	}

	return &tridentcontroller.RegistrationInfo{
		TopologyLabels: ack.Status.TopologyLabels,
		LogLevel:       ack.Status.LogLevel,
		LogWorkflows:   ack.Status.LogWorkflows,
		LogLayers:      ack.Status.LogLayers,
	}, nil
}

// mergeTridentNodeRegistrationSpec applies a node re-registration identity refresh onto an
// existing TridentNode spec. dst is the persisted CR; identity is built from local discovery.
// Only node-owned identity fields are updated. spec.provisionerReady is left unchanged so a pod
// restart does not clear MarkNodeCleanupComplete. spec.nodePrep is updated only when discovery
// returns prep data, so an empty local snapshot does not wipe stored prep state.
func mergeTridentNodeRegistrationSpec(dst, identity *tridentv1.TridentNode) {
	dst.Spec.NodeName = identity.Spec.NodeName
	dst.Spec.IQN = identity.Spec.IQN
	dst.Spec.NQN = identity.Spec.NQN
	dst.Spec.IPs = identity.Spec.IPs
	dst.Spec.HostWWPNMap = identity.Spec.HostWWPNMap
	dst.Spec.HostInfo = identity.Spec.HostInfo
	if len(identity.Spec.NodePrep.Raw) > 0 {
		dst.Spec.NodePrep = identity.Spec.NodePrep
	}
}

// syncTridentNodeRootFieldsFromSpec copies node-owned identity from spec onto root-level legacy
// fields after mergeTridentNodeRegistrationSpec. During N+1 dual-write migration, readers may
// still consume root fields; this keeps them aligned with spec. Controller-owned fields
// (publicationState, deleted, logging) are intentionally omitted.
func syncTridentNodeRootFieldsFromSpec(dst *tridentv1.TridentNode) {
	dst.NodeName = dst.Spec.NodeName
	dst.IQN = dst.Spec.IQN
	dst.NQN = dst.Spec.NQN
	dst.IPs = dst.Spec.IPs
	dst.HostWWPNMap = dst.Spec.HostWWPNMap
	dst.NodePrep = dst.Spec.NodePrep
	dst.HostInfo = dst.Spec.HostInfo
}

func isRegistrationComplete(nodeCR *tridentv1.TridentNode) bool {
	return nodeCR != nil && nodeCR.Status.Registered &&
		nodeCR.Status.ObservedGeneration >= nodeCR.GetGeneration()
}

func (c *Client) waitForRegistrationInfo(
	waitCtx context.Context, created *tridentv1.TridentNode, timeout time.Duration,
) (*tridentv1.TridentNode, error) {
	ackCheck := make(chan *tridentv1.TridentNode, 1)
	var watchEvents <-chan *tridentv1.TridentNode
	stopInformer := make(chan struct{})
	defer close(stopInformer)

	informerFactory := tridentinformers.NewSharedInformerFactoryWithOptions(
		c.tridentClient, 0,
		tridentinformers.WithNamespace(c.tridentNamespace),
		tridentinformers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.FieldSelector = fields.OneTermEqualSelector("metadata.name", created.Name).String()
		}),
	)
	nodeInformer := informerFactory.Trident().V1().TridentNodes().Informer()
	_, _ = nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if nodeCR, ok := obj.(*tridentv1.TridentNode); ok && isRegistrationComplete(nodeCR) {
				select {
				case ackCheck <- nodeCR:
				default:
				}
			}
		},
		UpdateFunc: func(_, newObj interface{}) {
			if nodeCR, ok := newObj.(*tridentv1.TridentNode); ok && isRegistrationComplete(nodeCR) {
				select {
				case ackCheck <- nodeCR:
				default:
				}
			}
		},
	})
	go nodeInformer.Run(stopInformer)

	syncTimeout := ackInformerSyncTimeout
	if timeout > 0 && timeout < ackInformerSyncTimeout {
		syncTimeout = timeout
	}
	syncCtx, cancelSync := context.WithTimeout(waitCtx, syncTimeout)
	defer cancelSync()
	if cache.WaitForCacheSync(syncCtx.Done(), nodeInformer.HasSynced) {
		watchEvents = ackCheck
	} else {
		Logc(waitCtx).Warn("TridentNode informer sync did not complete; continuing with backup polling.")
	}

	firstPollDelay := c.nextBackupAckPollInterval()
	if watchEvents == nil {
		firstPollDelay = 0
	}
	ackPollTimer := time.NewTimer(firstPollDelay)
	defer func() {
		if !ackPollTimer.Stop() {
			select {
			case <-ackPollTimer.C:
			default:
			}
		}
	}()

	for {
		select {
		case <-waitCtx.Done():
			return nil, waitCtx.Err()
		case current := <-watchEvents:
			if isRegistrationComplete(current) {
				return current, nil
			}
		case <-ackPollTimer.C:
			current, getErr := c.tridentClient.TridentV1().TridentNodes(c.tridentNamespace).Get(
				waitCtx, created.Name, metav1.GetOptions{},
			)
			if getErr != nil {
				Logc(waitCtx).WithError(getErr).Debug("Backup registration-ack polling GET failed.")
				ackPollTimer.Reset(c.nextBackupAckPollInterval())
				continue
			}
			if isRegistrationComplete(current) {
				return current, nil
			}
			ackPollTimer.Reset(c.nextBackupAckPollInterval())
		}
	}
}

// randomOffsetUpTo returns a uniform random duration in [0, max) using crypto/rand.
// It adds jitter to backup registration-ack poll intervals so nodes do not resync in lockstep.
func randomOffsetUpTo(max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	n, err := crand.Int(crand.Reader, big.NewInt(int64(max)))
	if err != nil {
		Log().WithError(err).Warn("crypto/rand failed for registration ack poll jitter; using minimum offset")
		return 0
	}
	return time.Duration(n.Int64())
}

func (c *Client) nextBackupAckPollInterval() time.Duration {
	span := 2*backupAckPollJitter + time.Nanosecond
	offset := randomOffsetUpTo(span) - backupAckPollJitter
	return backupAckPollBase + offset
}

func notifyNodeCleanupEvent(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// StartNodeCleanupWatch watches TridentNode and publication CRs for cleanup-related changes.
func (c *Client) StartNodeCleanupWatch(ctx context.Context, nodeName string) (<-chan struct{}, func(), error) {
	if err := c.ensureTridentClient(); err != nil {
		return nil, nil, err
	}

	events := make(chan struct{}, 1)
	stopWatch := make(chan struct{})
	var stopOnce sync.Once
	stopFn := func() {
		stopOnce.Do(func() {
			close(stopWatch)
		})
	}

	nodeFactory := tridentinformers.NewSharedInformerFactoryWithOptions(
		c.tridentClient, 0,
		tridentinformers.WithNamespace(c.tridentNamespace),
		tridentinformers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.FieldSelector = fields.OneTermEqualSelector("metadata.name", tridentv1.NameFix(nodeName)).String()
		}),
	)
	pubFactory := tridentinformers.NewSharedInformerFactoryWithOptions(
		c.tridentClient, 0,
		tridentinformers.WithNamespace(c.tridentNamespace),
		tridentinformers.WithTweakListOptions(func(opts *metav1.ListOptions) {
			opts.LabelSelector = fmt.Sprintf("%s=%s", config.TridentNodeNameLabel, nodeName)
		}),
	)

	nodeInformer := nodeFactory.Trident().V1().TridentNodes().Informer()
	pubInformer := pubFactory.Trident().V1().TridentVolumePublications().Informer()
	handler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(_ interface{}) {
			notifyNodeCleanupEvent(events)
		},
		UpdateFunc: func(_, _ interface{}) {
			notifyNodeCleanupEvent(events)
		},
		DeleteFunc: func(_ interface{}) {
			notifyNodeCleanupEvent(events)
		},
	}
	_, _ = nodeInformer.AddEventHandler(handler)
	_, _ = pubInformer.AddEventHandler(handler)

	go nodeInformer.Run(stopWatch)
	go pubInformer.Run(stopWatch)
	syncCtx, cancelSync := context.WithTimeout(ctx, ackInformerSyncTimeout)
	defer cancelSync()
	if !cache.WaitForCacheSync(syncCtx.Done(), nodeInformer.HasSynced, pubInformer.HasSynced) {
		stopFn()
		return nil, nil, fmt.Errorf("timed out waiting for node cleanup informers to sync")
	}

	notifyNodeCleanupEvent(events)
	return events, stopFn, nil
}

// GetDesiredPublications lists TridentVolumePublication CRs labeled for the node.
func (c *Client) GetDesiredPublications(
	ctx context.Context, nodeName string,
) (map[string]*models.VolumePublicationExternal, error) {
	if err := c.ensureTridentClient(); err != nil {
		return nil, err
	}
	publications, err := c.tridentClient.TridentV1().TridentVolumePublications(c.tridentNamespace).List(
		ctx, metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", config.TridentNodeNameLabel, nodeName)},
	)
	if err != nil {
		return nil, err
	}

	desiredPublicationState := make(map[string]*models.VolumePublicationExternal, len(publications.Items))
	for _, pubCR := range publications.Items {
		pub, pubErr := pubCR.Persistent()
		if pubErr != nil {
			return nil, pubErr
		}
		pubExternal := pub.ConstructExternal()
		desiredPublicationState[pubExternal.VolumeName] = pubExternal
	}
	return desiredPublicationState, nil
}

// GetNodeCleanupStatus reads publication cleanup state from the TridentNode CR.
func (c *Client) GetNodeCleanupStatus(ctx context.Context, nodeName string) (models.NodePublicationState, error) {
	if err := c.ensureTridentClient(); err != nil {
		return "", err
	}
	nodeCR, err := c.tridentClient.TridentV1().TridentNodes(c.tridentNamespace).Get(
		ctx, tridentv1.NameFix(nodeName), metav1.GetOptions{},
	)
	if err != nil {
		return "", err
	}
	nodeState := models.NodePublicationState(nodeCR.Status.PublicationState)
	if nodeState == "" {
		nodeState = models.NodePublicationState(nodeCR.PublicationState)
	}
	return nodeState, nil
}

// MarkNodeCleanupComplete sets ProvisionerReady on the TridentNode CR after local cleanup.
func (c *Client) MarkNodeCleanupComplete(ctx context.Context, nodeName string) error {
	if err := c.ensureTridentClient(); err != nil {
		return err
	}
	nodeCR, err := c.tridentClient.TridentV1().TridentNodes(c.tridentNamespace).Get(
		ctx, tridentv1.NameFix(nodeName), metav1.GetOptions{},
	)
	if err != nil {
		return err
	}
	nodeUpdate := nodeCR.DeepCopy()
	nodeUpdate.Spec.ProvisionerReady = convert.ToPtr(true)
	_, err = c.tridentClient.TridentV1().TridentNodes(c.tridentNamespace).Update(
		ctx, nodeUpdate, metav1.UpdateOptions{},
	)
	return err
}
