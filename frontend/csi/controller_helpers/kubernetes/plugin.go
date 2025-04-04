// Copyright 2025 NetApp, Inc. All Rights Reserved.

package kubernetes

//go:generate mockgen -destination=../../../../mocks/mock_frontend/mock_csi/mock_controller_helpers/mock_kubernetes_helper/mock_kubernetes_helper.go github.com/netapp/trident/frontend/csi/controller_helpers/kubernetes K8SControllerHelperPlugin

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	csiaccessmodes "github.com/kubernetes-csi/csi-lib-utils/accessmodes"
	k8ssnapshot "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	v1 "k8s.io/api/core/v1"
	k8sstoragev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	clik8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/frontend/csi"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	. "github.com/netapp/trident/logging"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	clientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	storageattribute "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
	versionutils "github.com/netapp/trident/utils/version"
)

const (
	uidIndex  = "uid"
	nameIndex = "name"
	vrefIndex = "vref"

	eventAdd    = "add"
	eventUpdate = "update"
	eventDelete = "delete"

	outOfServiceTaintKey = "node.kubernetes.io/out-of-service"

	// maxResourceNameLength is the maximum possible length of a Kubernetes resource name.
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#dns-subdomain-names
	maxResourceNameLength = 253
)

var (
	uidRegex = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	pvcRegex = regexp.MustCompile(
		`^pvc-(?P<uid>[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)
)

type K8SControllerHelperPlugin interface {
	frontend.Plugin
	ImportVolume(ctx context.Context, request *storage.ImportVolumeRequest) (*storage.VolumeExternal, error)
	GetNodePublicationState(ctx context.Context, nodeName string) (*models.NodePublicationStateFlags, error)
}

type helper struct {
	orchestrator      core.Orchestrator
	tridentClient     clientset.Interface
	restConfig        rest.Config
	kubeClient        kubernetes.Interface
	snapClient        k8ssnapshot.Interface
	kubeVersion       *k8sversion.Info
	namespace         string
	eventRecorder     record.EventRecorder
	enableForceDetach bool

	pvcIndexer            cache.Indexer
	pvcController         cache.SharedIndexInformer
	pvcControllerStopChan chan struct{}
	pvcSource             cache.ListerWatcher

	pvIndexer            cache.Indexer
	pvController         cache.SharedIndexInformer
	pvControllerStopChan chan struct{}
	pvSource             cache.ListerWatcher

	scIndexer            cache.Indexer
	scController         cache.SharedIndexInformer
	scControllerStopChan chan struct{}
	scSource             cache.ListerWatcher

	nodeIndexer            cache.Indexer
	nodeController         cache.SharedIndexInformer
	nodeControllerStopChan chan struct{}
	nodeSource             cache.ListerWatcher

	mrIndexer            cache.Indexer
	mrController         cache.SharedIndexInformer
	mrControllerStopChan chan struct{}
	mrSource             cache.ListerWatcher

	vrefIndexer            cache.Indexer
	vrefController         cache.SharedIndexInformer
	vrefControllerStopChan chan struct{}
	vrefSource             cache.ListerWatcher
}

// NewHelper instantiates this plugin when running outside a pod.
func NewHelper(
	orchestrator core.Orchestrator, masterURL, kubeConfigPath string, enableForceDetach bool,
) (frontend.Plugin, error) {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginCreate, LogLayerCSIFrontend)

	Logc(ctx).Info("Initializing K8S helper frontend.")

	clients, err := clik8sclient.CreateK8SClients(masterURL, kubeConfigPath, "")
	if err != nil {
		return nil, err
	}

	kubeClient := clients.KubeClient

	// Get the Kubernetes version
	kubeVersion, err := kubeClient.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("K8S helper frontend could not retrieve API server's version: %v", err)
	}

	p := &helper{
		orchestrator:           orchestrator,
		tridentClient:          clients.TridentClient,
		restConfig:             *clients.RestConfig,
		kubeClient:             clients.KubeClient,
		snapClient:             clients.SnapshotClient,
		kubeVersion:            kubeVersion,
		namespace:              clients.Namespace,
		enableForceDetach:      enableForceDetach,
		pvcControllerStopChan:  make(chan struct{}),
		pvControllerStopChan:   make(chan struct{}),
		scControllerStopChan:   make(chan struct{}),
		nodeControllerStopChan: make(chan struct{}),
		mrControllerStopChan:   make(chan struct{}),
		vrefControllerStopChan: make(chan struct{}),
	}

	Logc(ctx).WithFields(LogFields{
		"version":    p.kubeVersion.Major + "." + p.kubeVersion.Minor,
		"gitVersion": p.kubeVersion.GitVersion,
	}).Info("K8S helper determined the container orchestrator version.")

	if err = p.validateKubeVersion(); err != nil {
		return nil, fmt.Errorf("K8S helper frontend could not validate Kubernetes version: %v", err)
	}

	// Set up event broadcaster
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&corev1.EventSinkImpl{Interface: kubeClient.CoreV1().Events("")})
	p.eventRecorder = broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: csi.Provisioner})

	// Set up a watch for PVCs
	p.pvcSource = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return kubeClient.CoreV1().PersistentVolumeClaims(v1.NamespaceAll).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.CoreV1().PersistentVolumeClaims(v1.NamespaceAll).Watch(ctx, options)
		},
	}

	// Set up the PVC indexing controller
	p.pvcController = cache.NewSharedIndexInformer(
		p.pvcSource,
		&v1.PersistentVolumeClaim{},
		CacheSyncPeriod,
		cache.Indexers{uidIndex: MetaUIDKeyFunc},
	)
	p.pvcIndexer = p.pvcController.GetIndexer()

	// Add handlers for CSI-provisioned PVCs
	_, _ = p.pvcController.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    p.addPVC,
			UpdateFunc: p.updatePVC,
			DeleteFunc: p.deletePVC,
		},
	)

	// Set up a watch for PVs
	p.pvSource = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return kubeClient.CoreV1().PersistentVolumes().List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.CoreV1().PersistentVolumes().Watch(ctx, options)
		},
	}

	// Set up the PV indexing controller
	p.pvController = cache.NewSharedIndexInformer(
		p.pvSource,
		&v1.PersistentVolume{},
		CacheSyncPeriod,
		cache.Indexers{uidIndex: MetaUIDKeyFunc},
	)
	p.pvIndexer = p.pvController.GetIndexer()

	// Set up a watch for storage classes
	p.scSource = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return kubeClient.StorageV1().StorageClasses().List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.StorageV1().StorageClasses().Watch(ctx, options)
		},
	}

	// Set up the storage class indexing controller
	p.scController = cache.NewSharedIndexInformer(
		p.scSource,
		&k8sstoragev1.StorageClass{},
		CacheSyncPeriod,
		cache.Indexers{uidIndex: MetaUIDKeyFunc},
	)
	p.scIndexer = p.scController.GetIndexer()

	// Add handler for registering storage classes with Trident
	_, _ = p.scController.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    p.addStorageClass,
			UpdateFunc: p.updateStorageClass,
			DeleteFunc: p.deleteStorageClass,
		},
	)

	// Set up a watch for k8s nodes
	p.nodeSource = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return kubeClient.CoreV1().Nodes().List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.CoreV1().Nodes().Watch(ctx, options)
		},
	}

	// Set up the k8s node indexing controller
	p.nodeController = cache.NewSharedIndexInformer(
		p.nodeSource,
		&v1.Node{},
		CacheSyncPeriod,
		cache.Indexers{nameIndex: MetaNameKeyFunc},
	)
	p.nodeIndexer = p.nodeController.GetIndexer()

	// Add handler for registering k8s nodes with Trident
	_, _ = p.nodeController.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    p.addNode,
			UpdateFunc: p.updateNode,
			DeleteFunc: p.deleteNode,
		},
	)

	// Set up a watch for TridentMirrorRelationships
	p.mrSource = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return clients.TridentClient.TridentV1().TridentMirrorRelationships(v1.NamespaceAll).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return clients.TridentClient.TridentV1().TridentMirrorRelationships(v1.NamespaceAll).Watch(ctx, options)
		},
	}
	// Set up the TMR indexing controller
	p.mrController = cache.NewSharedIndexInformer(
		p.mrSource,
		&netappv1.TridentMirrorRelationship{},
		CacheSyncPeriod,
		cache.Indexers{nameIndex: MetaNameKeyFunc},
	)
	p.mrIndexer = p.mrController.GetIndexer()

	// Set up a watch for TridentVolumeReferences
	p.vrefSource = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return clients.TridentClient.TridentV1().TridentVolumeReferences(v1.NamespaceAll).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return clients.TridentClient.TridentV1().TridentVolumeReferences(v1.NamespaceAll).Watch(ctx, options)
		},
	}
	// Set up the TridentVolumeReferences indexing controller
	p.vrefController = cache.NewSharedIndexInformer(
		p.vrefSource,
		&netappv1.TridentVolumeReference{},
		CacheSyncPeriod,
		cache.Indexers{vrefIndex: TridentVolumeReferenceKeyFunc},
	)
	p.vrefIndexer = p.vrefController.GetIndexer()

	Logc(ctx).Info("K8S helper frontend initialized.")

	return p, nil
}

func (h *helper) GetNode(ctx context.Context, nodeName string) (*v1.Node, error) {
	Logc(ctx).WithField("nodeName", nodeName).Trace(">>>> GetNode")
	defer Logc(ctx).Trace("<<<< GetNode")

	node, err := h.kubeClient.CoreV1().Nodes().Get(ctx, nodeName, getOpts)
	return node, err
}

// MetaUIDKeyFunc is a KeyFunc which knows how to make keys for API objects
// which implement meta.Interface.  The key is the object's UID.
func MetaUIDKeyFunc(obj interface{}) ([]string, error) {
	Logc(context.Background()).Trace(">>>> MetaUIDKeyFunc")
	defer Logc(context.Background()).Trace("<<<< MetaUIDKeyFunc")

	if key, ok := obj.(string); ok && uidRegex.MatchString(key) {
		return []string{key}, nil
	}
	objectMeta, err := meta.Accessor(obj)
	if err != nil {
		return []string{""}, fmt.Errorf("object has no meta: %v", err)
	}
	if len(objectMeta.GetUID()) == 0 {
		return []string{""}, fmt.Errorf("object has no UID: %v", err)
	}
	return []string{string(objectMeta.GetUID())}, nil
}

// MetaNameKeyFunc is a KeyFunc which knows how to make keys for API objects
// which implement meta.Interface.  The key is the object's name.
func MetaNameKeyFunc(obj interface{}) ([]string, error) {
	Logc(context.Background()).Trace(">>>> MetaNameKeyFunc")
	defer Logc(context.Background()).Trace("<<<< MetaNameKeyFunc")

	if key, ok := obj.(string); ok {
		return []string{key}, nil
	}
	objectMeta, err := meta.Accessor(obj)
	if err != nil {
		return []string{""}, fmt.Errorf("object has no meta: %v", err)
	}
	if len(objectMeta.GetName()) == 0 {
		return []string{""}, fmt.Errorf("object has no name: %v", err)
	}
	return []string{objectMeta.GetName()}, nil
}

// TridentVolumeReferenceKeyFunc is a KeyFunc which knows how to make keys for TridentVolumeReference
// CRs.  The key is of the format "<CR namespace>_<referenced PVC namespce>/<referenced PVC name>".
func TridentVolumeReferenceKeyFunc(obj interface{}) ([]string, error) {
	Logc(context.Background()).Trace(">>>> TridentVolumeReferenceKeyFunc")
	defer Logc(context.Background()).Trace("<<<< TridentVolumeReferenceKeyFunc")

	if key, ok := obj.(string); ok {
		return []string{key}, nil
	}

	vref, ok := obj.(*netappv1.TridentVolumeReference)
	if !ok {
		return []string{""}, fmt.Errorf("object is not a TridentVolumeReference CR")
	}
	if vref.Spec.PVCName == "" {
		return []string{""}, fmt.Errorf("TridentVolumeReference CR has no PVCName set")
	}
	if vref.Spec.PVCNamespace == "" {
		return []string{""}, fmt.Errorf("TridentVolumeReference CR has no PVCNamespace set")
	}

	return []string{vref.CacheKey()}, nil
}

// Activate starts this Trident frontend.
func (h *helper) Activate() error {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginActivate, LogLayerCSIFrontend)

	Logc(ctx).Info("Activating K8S helper frontend.")

	// TODO (websterj): Revisit or remove this reconcile once Trident v21.10.1 has reached EOL;
	// At that point, all supported Trident versions will include volume publications.
	reconcileRequired, err := h.isPublicationReconcileRequired(ctx)
	if err != nil {
		return csi.TerminalReconciliationError(err.Error())
	}
	if reconcileRequired {
		Logc(ctx).Debug("Publication reconciliation is required.")
		// This call expects the Trident orchestrator core to bootstrapped successfully.
		if err := h.reconcileVolumePublications(ctx); err != nil {
			Logc(ctx).Errorf("Error reconciling legacy volumes; %v", err)
			return csi.TerminalReconciliationError(err.Error())
		}
	} else {
		Logc(ctx).Debug("Publication reconciliation is unnecessary.")
	}

	go h.pvcController.Run(h.pvcControllerStopChan)
	go h.pvController.Run(h.pvControllerStopChan)
	go h.scController.Run(h.scControllerStopChan)
	go h.nodeController.Run(h.nodeControllerStopChan)
	go h.mrController.Run(h.mrControllerStopChan)
	go h.vrefController.Run(h.vrefControllerStopChan)
	go h.reconcileNodes(ctx)

	// Configure telemetry
	config.OrchestratorTelemetry.Platform = string(config.PlatformKubernetes)
	config.OrchestratorTelemetry.PlatformVersion = h.Version()

	Logc(ctx).Debug("Activated K8S helper frontend.")

	return nil
}

// Deactivate stops this Trident frontend.
func (h *helper) Deactivate() error {
	close(h.pvcControllerStopChan)
	close(h.pvControllerStopChan)
	close(h.scControllerStopChan)
	close(h.nodeControllerStopChan)
	close(h.mrControllerStopChan)
	close(h.vrefControllerStopChan)
	Log().Debug("Deactivated K8S helper frontend.")
	return nil
}

// GetName returns the name of this Trident frontend.
func (h *helper) GetName() string {
	return controllerhelpers.KubernetesHelper
}

// Version returns the version of this Trident frontend (the detected K8S version).
func (h *helper) Version() string {
	return h.kubeVersion.GitVersion
}

// listClusterNodes returns the list of worker node names as a map for kubernetes cluster
func (h *helper) listClusterNodes(ctx context.Context) (map[string]bool, error) {
	Logc(ctx).Trace(">>>> listClusterNodes")
	defer Logc(ctx).Trace("<<<< listClusterNodes")
	nodeNames := make(map[string]bool)
	nodes, err := h.kubeClient.CoreV1().Nodes().List(ctx, listOpts)
	if err != nil {
		err = fmt.Errorf("error reading kubernetes nodes; %v", err)
		Logc(ctx).Error(err)
		return nodeNames, err
	}
	for _, node := range nodes.Items {
		nodeNames[node.Name] = true
	}
	return nodeNames, nil
}

// reconcileNodes will make sure that Trident's list of nodes does not include any unnecessary node
func (h *helper) reconcileNodes(ctx context.Context) {
	Logc(ctx).Debug("Performing node reconciliation.")
	clusterNodes, err := h.listClusterNodes(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Error("Unable to list nodes in Kubernetes; aborting node reconciliation.")
		return
	}
	tridentNodes, err := h.orchestrator.ListNodes(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Error("Unable to list nodes in Trident; aborting node reconciliation.")
		return
	}

	for _, node := range tridentNodes {
		if _, ok := clusterNodes[node.Name]; !ok {
			// Trident node no longer exists in cluster, remove it
			Logc(ctx).WithField("node", node.Name).Debug("Node not found in Kubernetes; removing from Trident.")
			err = h.orchestrator.DeleteNode(ctx, node.Name)
			if err != nil {
				Logc(ctx).WithField("node", node.Name).WithError(err).Error("Error removing node from Trident.")
			}
		}
	}
	Logc(ctx).Info("Node reconciliation complete.")
}

// isPublicationReconcileRequired looks at Trident's persistent TridentVersion CR and checks if publications are synced.
// If they are, no reconcile is required. Otherwise, attempt to reconcile.
func (h *helper) isPublicationReconcileRequired(ctx context.Context) (bool, error) {
	Logc(ctx).Debug("Checking if publication reconciliation is required.")
	version, err := h.tridentClient.TridentV1().TridentVersions(h.namespace).Get(ctx, config.OrchestratorName, getOpts)
	if err != nil {
		return false, fmt.Errorf("unable to get Trident version; %v", err)
	}

	return !version.PublicationsSynced, nil
}

// listVolumeAttachments returns the list of volume attachments in kubernetes.
func (h *helper) listVolumeAttachments(ctx context.Context) ([]k8sstoragev1.VolumeAttachment, error) {
	attachments, err := h.kubeClient.StorageV1().VolumeAttachments().List(ctx, listOpts)
	if err != nil {
		statusErr, ok := err.(*apierrors.StatusError)
		if ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return nil, errors.NotFoundError("no volume attachments found; %s", err.Error())
		}
		return nil, err
	}

	if attachments == nil {
		return nil, errors.NotFoundError("no volume attachments found")
	}

	return attachments.Items, nil
}

// reconcileVolumePublications upgrades any legacy volumes in Trident by creating publications for
// K8s volumeAttachments with corresponding Trident volumes that have no publication associated with it.
// This should happen as part of activating the K8s controller helper and after the Trident core has bootstrapped
// volumes and publications.
// NOTE: Not all volumes have a volume publication. Volume attachments should be used as the source of
// truth in this reconciliation because if a volume attachment exists && it is attached, it means
// CSI ControllerPublishVolume has happened before for that volume.
func (h *helper) reconcileVolumePublications(ctx context.Context) error {
	Logc(ctx).Debug("Performing publication reconciliation.")
	attachments, err := h.listVolumeAttachments(ctx)
	if err != nil {
		if !errors.IsNotFoundError(err) {
			Logc(ctx).Errorf("Unable list volume attachments; aborting publication reconciliation; %v", err)
			return err
		}
		Logc(ctx).Debugf("No volume attachments found; continuing reconciliation; %v", err)
	}

	// Reconcile even if no attachments are found so volume publications are synced from Trident core's perspective.
	publications, err := h.listAttachmentsAsPublications(ctx, attachments)
	if err != nil {
		Logc(ctx).Errorf(
			"Unable list volume attachments as publications; aborting publication reconciliation; %v", err)
		return err
	}

	err = h.orchestrator.ReconcileVolumePublications(ctx, publications)
	if err != nil {
		Logc(ctx).Errorf(
			"Unable to reconcile volume publications; aborting publication reconciliation; %v", err)
		return err
	}

	Logc(ctx).Debug("Publication reconciliation complete.")
	return nil
}

// listAttachmentsAsPublications returns a list of Kubernetes volume attachments as Trident volume publications.
func (h *helper) listAttachmentsAsPublications(
	ctx context.Context, attachments []k8sstoragev1.VolumeAttachment,
) ([]*models.VolumePublicationExternal, error) {
	Logc(ctx).Debug("Converting Kubernetes volume attachments to Trident publications.")
	defer Logc(ctx).Debug("Converted Kubernetes volume attachments to Trident publications.")

	publications := make([]*models.VolumePublicationExternal, 0)
	for _, attachment := range attachments {

		// If the attachment is invalid, don't add it.
		if !h.isAttachmentValid(ctx, attachment) {
			continue
		}

		volumeName := *attachment.Spec.Source.PersistentVolumeName
		nodeName := attachment.Spec.NodeName
		tridentVolume, err := h.orchestrator.GetVolume(ctx, volumeName)
		if err != nil {
			if errors.IsNotFoundError(err) {
				Logc(ctx).WithFields(LogFields{
					"attachment": attachment.Name,
					"volume":     volumeName,
					"node":       nodeName,
				}).Warning("Unable to find a corresponding Trident volume for this attachment; ignoring.")
				continue
			}
			return publications, err
		}

		// Build the access modes for this attached volume the same way the CSI external-attacher does.
		accessModes := []v1.PersistentVolumeAccessMode{v1.PersistentVolumeAccessMode(tridentVolume.Config.AccessMode)}
		// "supportsSingleNodeMultiWriter" will always be false; it relies on capabilities of the CSI Controller Server.
		accessMode, err := csiaccessmodes.ToCSIAccessMode(accessModes, false)
		if err != nil {
			return publications, fmt.Errorf("invalid access mode on attached volume; %v", err)
		}

		// Build an external publication to supply to the core
		publication := &models.VolumePublicationExternal{
			Name:       models.GenerateVolumePublishName(volumeName, nodeName),
			VolumeName: volumeName,
			NodeName:   nodeName,
			ReadOnly:   tridentVolume.Config.AccessInfo.ReadOnly, // should always default to false.
			AccessMode: int32(accessMode),
		}
		publications = append(publications, publication)
	}

	return publications, nil
}

// isAttachmentValid is used to determine if a volume attachment is valid.
//
//	There are 4 conditions when an attachment will be marked as invalid:
//	1. If the provisioner / attacher is not Trident.
//	2. If the volume attachment is not attached.
//	3. If the volume attachment lacks a source volume.
//	4. If the volume attachment lacks a node.
func (h *helper) isAttachmentValid(ctx context.Context, attachment k8sstoragev1.VolumeAttachment) bool {
	fields := LogFields{
		"attachment": attachment.Name,
	}

	if attachment.Spec.Attacher != csi.Provisioner {
		Logc(ctx).WithFields(fields).Debug("Volume attachment isn't attached by Trident; ignoring.")
		return false
	}
	if !attachment.Status.Attached {
		Logc(ctx).WithFields(fields).Debug("Volume attachment isn't attached; ignoring.")
		return false
	}
	if attachment.Spec.Source.PersistentVolumeName == nil || *attachment.Spec.Source.PersistentVolumeName == "" {
		Logc(ctx).WithFields(fields).Debug("Volume attachment has invalid source volume; ignoring.")
		return false
	}
	if attachment.Spec.NodeName == "" {
		Logc(ctx).WithFields(fields).Debug("Volume attachment has no node; ignoring.")
		return false
	}

	return true
}

// addPVC is the add handler for the PVC watcher.
func (h *helper) addPVC(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S, WorkflowVolumeCreate, LogLayerCSIFrontend)

	switch pvc := obj.(type) {
	case *v1.PersistentVolumeClaim:
		h.processPVC(ctx, pvc, eventAdd)
	default:
		Logc(ctx).Errorf("K8S helper expected PVC; got %v", obj)
	}
}

// updatePVC is the update handler for the PVC watcher.
func (h *helper) updatePVC(_, newObj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S, WorkflowVolumeUpdate, LogLayerCSIFrontend)
	Logc(ctx).Trace(">>>> updatePVC")
	defer Logc(ctx).Trace("<<<< updatePVC")

	switch pvc := newObj.(type) {
	case *v1.PersistentVolumeClaim:
		h.processPVC(ctx, pvc, eventUpdate)
	default:
		Logc(ctx).Errorf("K8S helper expected PVC; got %v", newObj)
	}
}

// deletePVC is the delete handler for the PVC watcher.
func (h *helper) deletePVC(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S, WorkflowVolumeDelete, LogLayerCSIFrontend)
	Logc(ctx).Trace(">>>> deletePVC")
	defer Logc(ctx).Trace("<<<< deletePVC")

	switch pvc := obj.(type) {
	case *v1.PersistentVolumeClaim:
		h.processPVC(ctx, pvc, eventDelete)
	default:
		Logc(ctx).Errorf("K8S helper expected PVC; got %v", obj)
	}
}

// processPVC logs the add/update/delete PVC events.
func (h *helper) processPVC(ctx context.Context, pvc *v1.PersistentVolumeClaim, eventType string) {
	Logc(ctx).Trace(">>>> processPVC")
	defer Logc(ctx).Trace("<<<< processPVC")
	// Validate the PVC
	size, ok := pvc.Spec.Resources.Requests[v1.ResourceStorage]
	if !ok {
		Logc(ctx).WithField("name", pvc.Name).Debug("Rejecting PVC, no size specified.")
		return
	}

	logFields := LogFields{
		"name":         pvc.Name,
		"phase":        pvc.Status.Phase,
		"size":         size.String(),
		"uid":          pvc.UID,
		"storageClass": getStorageClassForPVC(pvc),
		"accessModes":  pvc.Spec.AccessModes,
		"pv":           pvc.Spec.VolumeName,
	}

	switch eventType {
	case eventAdd:
		Logc(ctx).WithFields(logFields).Trace("PVC added to cache.")
	case eventUpdate:
		Logc(ctx).WithFields(logFields).Trace("PVC updated in cache.")
	case eventDelete:
		Logc(ctx).WithFields(logFields).Trace("PVC deleted from cache.")
	}
}

// getCachedPVCByName returns a PVC (identified by namespace/name) from the client's cache,
// or an error if not found.  In most cases it may be better to call waitForCachedPVCByName().
func (h *helper) getCachedPVCByName(ctx context.Context, name, namespace string) (*v1.PersistentVolumeClaim, error) {
	logFields := LogFields{"name": name, "namespace": namespace}

	Logc(ctx).Trace(">>>> getCachedPVCByName")
	defer Logc(ctx).Trace("<<<< getCachedPVCByName")

	item, exists, err := h.pvcIndexer.GetByKey(namespace + "/" + name)
	if err != nil {
		Logc(ctx).WithFields(logFields).Error("Could not search cache for PVC by name.")
		return nil, fmt.Errorf("could not search cache for PVC %s/%s: %v", namespace, name, err)
	} else if !exists {
		Logc(ctx).WithFields(logFields).Debug("PVC object not found in cache by name.")
		return nil, fmt.Errorf("PVC %s/%s not found in cache", namespace, name)
	} else if pvc, ok := item.(*v1.PersistentVolumeClaim); !ok {
		Logc(ctx).WithFields(logFields).Error("Non-PVC cached object found by name.")
		return nil, fmt.Errorf("non-PVC object %s/%s found in cache", namespace, name)
	} else {
		Logc(ctx).WithFields(logFields).Debug("Found cached PVC by name.")
		return pvc, nil
	}
}

// waitForCachedPVCByUID returns a PVC (identified by namespace/name) from the client's cache, waiting in a
// backoff loop for the specified duration for the PVC to become available.
func (h *helper) waitForCachedPVCByName(
	ctx context.Context, name, namespace string, maxElapsedTime time.Duration,
) (*v1.PersistentVolumeClaim, error) {
	var pvc *v1.PersistentVolumeClaim

	Logc(ctx).WithFields(LogFields{
		"Name":           name,
		"Namespace":      namespace,
		"MaxElapsedTime": maxElapsedTime,
	}).Trace(">>>> waitForCachedPVCByName")
	defer Logc(ctx).Trace("<<<< waitForCachedPVCByName")

	checkForCachedPVC := func() error {
		var pvcError error
		pvc, pvcError = h.getCachedPVCByName(ctx, name, namespace)
		return pvcError
	}
	pvcNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"name":      name,
			"namespace": namespace,
			"increment": duration,
		}).Trace("PVC not yet in cache, waiting.")
	}
	pvcBackoff := backoff.NewExponentialBackOff()
	pvcBackoff.InitialInterval = CacheBackoffInitialInterval
	pvcBackoff.RandomizationFactor = CacheBackoffRandomizationFactor
	pvcBackoff.Multiplier = CacheBackoffMultiplier
	pvcBackoff.MaxInterval = CacheBackoffMaxInterval
	pvcBackoff.MaxElapsedTime = maxElapsedTime

	if err := backoff.RetryNotify(checkForCachedPVC, pvcBackoff, pvcNotify); err != nil {
		return nil, fmt.Errorf("PVC %s/%s was not in cache after %3.2f seconds",
			namespace, name, maxElapsedTime.Seconds())
	}

	return pvc, nil
}

// getCachedPVCByUID returns a PVC (identified by UID) from the client's cache,
// or an error if not found.  In most cases it may be better to call waitForCachedPVCByUID().
func (h *helper) getCachedPVCByUID(ctx context.Context, uid string) (*v1.PersistentVolumeClaim, error) {
	Logc(ctx).WithField("uid", uid).Trace(">>>> getCachedPVCByUID")
	defer Logc(ctx).Trace("<<<< getCachedPVCByUID")

	items, err := h.pvcIndexer.ByIndex(uidIndex, uid)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Could not search cache for PVC by UID.")
		return nil, fmt.Errorf("could not search cache for PVC with UID %s: %v", uid, err)
	} else if len(items) == 0 {
		Logc(ctx).WithField("uid", uid).Debug("PVC object not found in cache by UID.")
		return nil, fmt.Errorf("PVC with UID %s not found in cache", uid)
	} else if len(items) > 1 {
		Logc(ctx).WithField("uid", uid).Error("Multiple cached PVC objects found by UID.")
		return nil, fmt.Errorf("multiple PVC objects with UID %s found in cache", uid)
	} else if pvc, ok := items[0].(*v1.PersistentVolumeClaim); !ok {
		Logc(ctx).WithField("uid", uid).Error("Non-PVC cached object found by UID.")
		return nil, fmt.Errorf("non-PVC object with UID %s found in cache", uid)
	} else {
		Logc(ctx).WithFields(LogFields{
			"name":      pvc.Name,
			"namespace": pvc.Namespace,
			"uid":       pvc.UID,
		}).Debug("Found cached PVC by UID.")
		return pvc, nil
	}
}

// waitForCachedPVCByUID returns a PVC (identified by UID) from the client's cache, waiting in a
// backoff loop for the specified duration for the PVC to become available.
func (h *helper) waitForCachedPVCByUID(
	ctx context.Context, uid string, maxElapsedTime time.Duration,
) (*v1.PersistentVolumeClaim, error) {
	var pvc *v1.PersistentVolumeClaim

	Logc(ctx).WithField("uid", uid).Trace(">>>> waitForCachedPVCByUID")
	defer Logc(ctx).Trace("<<<< waitForCachedPVCByUID")

	checkForCachedPVC := func() error {
		var pvcError error
		pvc, pvcError = h.getCachedPVCByUID(ctx, uid)
		return pvcError
	}
	pvcNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"uid":       uid,
			"increment": duration,
		}).Debug("PVC not yet in cache, waiting.")
	}
	pvcBackoff := backoff.NewExponentialBackOff()
	pvcBackoff.InitialInterval = CacheBackoffInitialInterval
	pvcBackoff.RandomizationFactor = CacheBackoffRandomizationFactor
	pvcBackoff.Multiplier = CacheBackoffMultiplier
	pvcBackoff.MaxInterval = CacheBackoffMaxInterval
	pvcBackoff.MaxElapsedTime = maxElapsedTime

	if err := backoff.RetryNotify(checkForCachedPVC, pvcBackoff, pvcNotify); err != nil {
		return nil, fmt.Errorf("PVC %s was not in cache after %3.2f seconds", uid, maxElapsedTime.Seconds())
	}

	return pvc, nil
}

func (h *helper) getCachedPVByName(ctx context.Context, name string) (*v1.PersistentVolume, error) {
	logFields := LogFields{"name": name}
	Logc(ctx).WithFields(logFields).Trace(">>>> getCachedPVByName")
	defer Logc(ctx).Trace("<<<< getCachedPVByName")

	item, exists, err := h.pvIndexer.GetByKey(name)
	if err != nil {
		Logc(ctx).WithFields(logFields).Error("Could not search cache for PV by name.")
		return nil, fmt.Errorf("could not search cache for PV: %v", err)
	} else if !exists {
		Logc(ctx).WithFields(logFields).Debug("Could not find cached PV object by name.")
		return nil, fmt.Errorf("could not find PV in cache")
	} else if pv, ok := item.(*v1.PersistentVolume); !ok {
		Logc(ctx).WithFields(logFields).Error("Non-PV cached object found by name.")
		return nil, fmt.Errorf("non-PV object found in cache")
	} else {
		Logc(ctx).WithFields(logFields).Debug("Found cached PV by name.")
		return pv, nil
	}
}

func (h *helper) isPVInCache(ctx context.Context, name string) (bool, error) {
	logFields := LogFields{"name": name}
	Logc(ctx).WithFields(logFields).Trace(">>>> isPVInCache")
	defer Logc(ctx).Trace("<<<< isPVInCache")

	item, exists, err := h.pvIndexer.GetByKey(name)
	if err != nil {
		Logc(ctx).WithFields(logFields).Error("Could not search cache for PV.")
		return false, fmt.Errorf("could not search cache for PV: %v", err)
	} else if !exists {
		Logc(ctx).WithFields(logFields).Debug("Could not find cached PV object by name.")
		return false, nil
	} else if _, ok := item.(*v1.PersistentVolume); !ok {
		Logc(ctx).WithFields(logFields).Error("Non-PV cached object found by name.")
		return false, fmt.Errorf("non-PV object found in cache")
	} else {
		Logc(ctx).WithFields(logFields).Debug("Found cached PV by name.")
		return true, nil
	}
}

// addStorageClass is the add handler for the storage class watcher.
func (h *helper) addStorageClass(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S, WorkflowStorageClassCreate, LogLayerCSIFrontend)
	Logc(ctx).Trace(">>>> addStorageClass")
	defer Logc(ctx).Trace("<<<< addStorageClass")

	sc, ok := obj.(*k8sstoragev1.StorageClass)
	if !ok {
		Logc(ctx).Errorf("K8S helper expected storage.k8s.io/v1 storage class; got %v", obj)
		return
	}

	if sc.Provisioner != csi.Provisioner {
		return
	}

	Logc(ctx).WithFields(LogFields{
		"name":        sc.Name,
		"provisioner": sc.Provisioner,
		"parameters":  sc.Parameters,
	}).Trace("Storage class added to cache.")

	h.processStorageClass(ctx, sc, false)
}

// updateStorageClass is the update handler for the storage class watcher.
func (h *helper) updateStorageClass(oldObj, newObj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S, WorkflowStorageClassUpdate, LogLayerCSIFrontend)
	Logc(ctx).Trace(">>>> updateStorageClass")
	defer Logc(ctx).Trace("<<<< updateStorageClass")

	var oldSC, newSC *k8sstoragev1.StorageClass
	var ok bool

	if oldSC, ok = oldObj.(*k8sstoragev1.StorageClass); !ok {
		Logc(ctx).Errorf("K8S helper expected storage.k8s.io/v1 storage class; got %v", oldObj)
		return
	}
	if newSC, ok = newObj.(*k8sstoragev1.StorageClass); !ok {
		Logc(ctx).Errorf("K8S helper expected storage.k8s.io/v1 storage class; got %v", newObj)
		return
	}

	if newSC.Provisioner != csi.Provisioner {
		return
	}

	logFields := LogFields{
		"name":        newSC.Name,
		"provisioner": newSC.Provisioner,
		"parameters":  newSC.Parameters,
	}

	changed := oldSC.ResourceVersion != newSC.ResourceVersion
	if changed {

		knownToCore := true
		if storageClass, _ := h.orchestrator.GetStorageClass(ctx, newSC.Name); storageClass == nil {
			Logc(ctx).WithFields(logFields).Warning("K8S helper has no record of the updated " +
				"storage class; instead it will try to create it.")
			knownToCore = false
		}

		Logc(ctx).WithFields(logFields).Trace("Storage class updated in cache.")
		h.processStorageClass(ctx, newSC, knownToCore)
	}
}

// deleteStorageClass is the delete handler for the storage class watcher.
func (h *helper) deleteStorageClass(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S, WorkflowStorageClassDelete, LogLayerCSIFrontend)
	Logc(ctx).Trace(">>>> deleteStorageClass")
	defer Logc(ctx).Trace("<<<< deleteStorageClass")

	sc, ok := obj.(*k8sstoragev1.StorageClass)
	if !ok {
		Logc(ctx).Errorf("K8S helper expected storage.k8s.io/v1 storage class; got %v", obj)
		return
	}

	if sc.Provisioner != csi.Provisioner {
		return
	}

	Logc(ctx).WithFields(LogFields{
		"name":        sc.Name,
		"provisioner": sc.Provisioner,
		"parameters":  sc.Parameters,
	}).Trace("Storage class deleted from cache.")

	h.processDeletedStorageClass(ctx, sc)
}

func removeSCParameterPrefix(key string) string {
	Logc(context.Background()).WithField("key", key).Trace(">>>> removeSCParameterPrefix")
	defer Logc(context.Background()).Trace("<<<< removeSCParameterPrefix")

	if strings.HasPrefix(key, annPrefix) {
		scParamKV := strings.SplitN(key, "/", 2)
		if len(scParamKV) != 2 || scParamKV[0] == "" || scParamKV[1] == "" {
			Log().Errorf("Storage class parameter %s does not have the right format.", key)
			return key
		}
		key = scParamKV[1]
	}
	return key
}

// processStorageClass informs the orchestrator of a new or updated storage class.
func (h *helper) processStorageClass(ctx context.Context, sc *k8sstoragev1.StorageClass, update bool) {
	Logc(ctx).Trace(">>>> processStorageClass")
	defer Logc(ctx).Trace("<<<< processStorageClass")
	logFields := LogFields{
		"name":        sc.Name,
		"provisioner": sc.Provisioner,
		"parameters":  sc.Parameters,
	}

	scConfig := new(storageclass.Config)
	scConfig.Name = sc.Name
	scConfig.Attributes = make(map[string]storageattribute.Request)

	// Populate storage class config attributes and backend storage pools
	for k, v := range sc.Parameters {

		// Ignore Kubernetes-defined storage class parameters handled by CSI
		if strings.HasPrefix(k, CSIParameterPrefix) || k == K8sFsType {
			continue
		}

		newKey := removeSCParameterPrefix(k)
		switch newKey {

		case storageattribute.RequiredStorage, storageattribute.AdditionalStoragePools:
			// format:  additionalStoragePools: "backend1:pool1,pool2;backend2:pool1"
			additionalPools, err := storageattribute.CreateBackendStoragePoolsMapFromEncodedString(v)
			if err != nil {
				Logc(ctx).WithFields(logFields).WithError(err).Errorf(
					"K8S helper could not process the storage class parameter %s", newKey)
			}
			scConfig.AdditionalPools = additionalPools

		case storageattribute.ExcludeStoragePools:
			// format:  excludeStoragePools: "backend1:pool1,pool2;backend2:pool1"
			excludeStoragePools, err := storageattribute.CreateBackendStoragePoolsMapFromEncodedString(v)
			if err != nil {
				Logc(ctx).WithFields(logFields).WithError(err).Errorf(
					"K8S helper could not process the storage class parameter %s", newKey)
			}
			scConfig.ExcludePools = excludeStoragePools

		case storageattribute.StoragePools:
			// format:  storagePools: "backend1:pool1,pool2;backend2:pool1"
			pools, err := storageattribute.CreateBackendStoragePoolsMapFromEncodedString(v)
			if err != nil {
				Logc(ctx).WithFields(logFields).WithError(err).Errorf(
					"K8S helper could not process the storage class parameter %s", newKey)
			}
			scConfig.Pools = pools

		default:
			// format:  attribute: "value"
			req, err := storageattribute.CreateAttributeRequestFromAttributeValue(newKey, v)
			if err != nil {
				Logc(ctx).WithFields(logFields).WithError(err).Errorf(
					"K8S helper could not process the storage class attribute %s", newKey)
				return
			}
			scConfig.Attributes[newKey] = req
		}
	}

	// To allow for atomic selector updates, replace selector with an annotation if it exists
	for k, v := range sc.ObjectMeta.Annotations {
		if k == AnnSelector {
			req, err := storageattribute.CreateAttributeRequestFromAttributeValue("selector", v)
			if err != nil {
				Logc(ctx).WithFields(logFields).WithField(k, v).WithError(err).Errorf(
					"K8S helper could not process the storage class annotation %s.", k)
				continue
			}
			scConfig.Attributes["selector"] = req
		}
	}

	if update {
		// Update the storage class
		if _, err := h.orchestrator.UpdateStorageClass(ctx, scConfig); err != nil {
			Logc(ctx).WithFields(logFields).WithError(err).Warning("K8S helper could not update a storage class.")
			return
		}
		Logc(ctx).WithFields(logFields).Trace("K8S helper updated a storage class.")
	} else {
		// Add the storage class
		if _, err := h.orchestrator.AddStorageClass(ctx, scConfig); err != nil {
			Logc(ctx).WithFields(logFields).WithError(err).Warning("K8S helper could not add a storage class.")
			return
		}
		Logc(ctx).WithFields(logFields).Trace("K8S helper added a storage class.")
	}
}

// processDeletedStorageClass informs the orchestrator of a deleted storage class.
func (h *helper) processDeletedStorageClass(ctx context.Context, sc *k8sstoragev1.StorageClass) {
	logFields := LogFields{"name": sc.Name}
	Logc(ctx).WithFields(logFields).Trace(">>>> processDeletedStorageClass")
	defer Logc(ctx).Trace("<<<< processDeletedStorageClass")

	// Delete the storage class from Trident
	if err := h.orchestrator.DeleteStorageClass(ctx, sc.Name); err != nil {
		Logc(ctx).WithFields(logFields).Errorf("K8S helper could not delete the storage class: %v", err)
	} else {
		Logc(ctx).WithFields(logFields).Trace("K8S helper deleted the storage class.")
	}
}

// getCachedStorageClassByName returns a storage class (identified by name) from the client's cache,
// or an error if not found.  In most cases it may be better to call waitForCachedStorageClassByName().
func (h *helper) getCachedStorageClassByName(ctx context.Context, name string) (*k8sstoragev1.StorageClass, error) {
	logFields := LogFields{"name": name}
	Logc(ctx).WithFields(logFields).Trace(">>>> getCachedStorageClassByName")
	defer Logc(ctx).Trace("<<<< getCachedStorageClassByName")

	item, exists, err := h.scIndexer.GetByKey(name)
	if err != nil {
		Logc(ctx).WithFields(logFields).Error("Could not search cache for storage class by name.")
		return nil, fmt.Errorf("could not search cache for storage class %s: %v", name, err)
	} else if !exists {
		Logc(ctx).WithFields(logFields).Debug("storage class object not found in cache by name.")
		return nil, fmt.Errorf("storage class %s not found in cache", name)
	} else if sc, ok := item.(*k8sstoragev1.StorageClass); !ok {
		Logc(ctx).WithFields(logFields).Error("Non-SC cached object found by name.")
		return nil, fmt.Errorf("non-SC object %s found in cache", name)
	} else {
		Logc(ctx).WithFields(logFields).Debug("Found cached storage class by name.")
		return sc, nil
	}
}

// waitForCachedStorageClassByName returns a storage class (identified by name) from the client's cache,
// waiting in a backoff loop for the specified duration for the storage class to become available.
func (h *helper) waitForCachedStorageClassByName(
	ctx context.Context, name string, maxElapsedTime time.Duration,
) (*k8sstoragev1.StorageClass, error) {
	var sc *k8sstoragev1.StorageClass

	Logc(ctx).WithFields(LogFields{
		"Name":           name,
		"MaxElapsedTime": maxElapsedTime,
	}).Trace(">>>> waitForCachedStorageClassByName")
	defer Logc(ctx).Trace("<<<< waitForCachedStorageClassByName")

	checkForCachedSC := func() error {
		var scError error
		sc, scError = h.getCachedStorageClassByName(ctx, name)
		return scError
	}
	scNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"name":      name,
			"increment": duration,
		}).Trace("Storage class not yet in cache, waiting.")
	}
	scBackoff := backoff.NewExponentialBackOff()
	scBackoff.InitialInterval = CacheBackoffInitialInterval
	scBackoff.RandomizationFactor = CacheBackoffRandomizationFactor
	scBackoff.Multiplier = CacheBackoffMultiplier
	scBackoff.MaxInterval = CacheBackoffMaxInterval
	scBackoff.MaxElapsedTime = maxElapsedTime

	if err := backoff.RetryNotify(checkForCachedSC, scBackoff, scNotify); err != nil {
		return nil, fmt.Errorf("storage class %s was not in cache after %3.2f seconds",
			name, maxElapsedTime.Seconds())
	}

	return sc, nil
}

// addNode is the add handler for the node watcher.
func (h *helper) addNode(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S, WorkflowNodeCreate, LogLayerCSIFrontend)
	Logc(ctx).Trace(">>>> addNode")
	defer Logc(ctx).Trace("<<<< addNode")

	node, ok := obj.(*v1.Node)
	if !ok {
		Logc(ctx).Errorf("K8S helper expected Node; got %v", obj)
		return
	}

	Logc(ctx).WithField("name", node.Name).Debug("Node added to cache.")
}

// updateNode is the update handler for the node watcher.
func (h *helper) updateNode(_, obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S, WorkflowNodeUpdate, LogLayerCSIFrontend)
	Logc(ctx).Trace(">>>> updateNode")
	defer Logc(ctx).Trace("<<<< updateNode")

	node, ok := obj.(*v1.Node)
	if !ok {
		Logc(ctx).Errorf("K8S helper expected Node; got %v", obj)
		return
	}

	logFields := LogFields{"name": node.Name}
	Logc(ctx).WithFields(logFields).Debug("Node updated in cache.")

	if h.enableForceDetach {
		if err := h.orchestrator.UpdateNode(ctx, node.Name, h.getNodePublicationState(node)); err != nil {
			Logc(ctx).WithFields(logFields).WithError(err).Error("Could not update node in Trident's database.")
		}
	}
}

// deleteNode is the delete handler for the node watcher.
func (h *helper) deleteNode(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S, WorkflowNodeDelete, LogLayerCSIFrontend)
	Logc(ctx).Trace(">>>> deleteNode")
	defer Logc(ctx).Trace("<<<< deleteNode")

	node, ok := obj.(*v1.Node)
	if !ok {
		Logc(ctx).Errorf("K8S helper expected Node; got %v", obj)
		return
	}

	logFields := LogFields{"name": node.Name}
	Logc(ctx).WithFields(logFields).Debug("Node deleted from cache.")

	if err := h.orchestrator.DeleteNode(ctx, node.Name); err != nil {
		if !errors.IsNotFoundError(err) {
			Logc(ctx).WithFields(logFields).WithError(err).Error("Could not delete node from Trident's database.")
		}
	}
}

// processNode logs and handles the add/update/delete node events.
func (h *helper) processNode(ctx context.Context, node *v1.Node, eventType string) {
	logFields := LogFields{"name": node.Name}
	Logc(ctx).WithFields(logFields).Trace(">>>> processNode")
	defer Logc(ctx).Trace("<<<< processNode")

	switch eventType {
	case eventAdd:
		Logc(ctx).WithFields(logFields).Trace("Node added to cache.")
	case eventUpdate:
		Logc(ctx).WithFields(logFields).Trace("Node updated in cache.")
	case eventDelete:
		err := h.orchestrator.DeleteNode(ctx, node.Name)
		if err != nil {
			if !errors.IsNotFoundError(err) {
				Logc(ctx).WithFields(logFields).Errorf("error deleting node from Trident's database; %v", err)
			}
		}
		Logc(ctx).WithFields(logFields).Trace("Node deleted from cache.")
	}
}

// GetNodePublicationState returns a set of flags that indicate whether, in certain circumstances,
// a node may safely publish volumes.  If such checking is not enabled or otherwise appropriate,
// this function returns nil.
func (h *helper) GetNodePublicationState(
	ctx context.Context, nodeName string,
) (*models.NodePublicationStateFlags, error) {
	if !h.enableForceDetach {
		return nil, nil
	}

	node, err := h.kubeClient.CoreV1().Nodes().Get(ctx, nodeName, getOpts)
	if err != nil {
		statusErr, ok := err.(*apierrors.StatusError)
		if ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return nil, errors.NotFoundError("node %s not found", nodeName)
		}
		return nil, err
	}

	return h.getNodePublicationState(node), nil
}

// getNodePublicationState returns a set of flags that indicate whether a node may safely publish volumes.
// To be safe, a node must have a Ready=true condition and not have the "node.kubernetes.io/out-of-service" taint.
func (h *helper) getNodePublicationState(node *v1.Node) *models.NodePublicationStateFlags {
	ready, outOfService := false, false

	// Look for non-Ready condition
	for _, condition := range node.Status.Conditions {
		if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
			ready = true
			break
		}
	}

	// Look for OutOfService taint
	for _, taint := range node.Spec.Taints {
		if taint.Key == outOfServiceTaintKey {
			outOfService = true
			break
		}
	}

	return &models.NodePublicationStateFlags{
		OrchestratorReady:  convert.ToPtr(ready),
		AdministratorReady: convert.ToPtr(!outOfService),
	}
}

func (h *helper) GetNodeTopologyLabels(ctx context.Context, nodeName string) (map[string]string, error) {
	Logc(ctx).WithField("nodeName", nodeName).Trace(">>>> GetNode")
	defer Logc(ctx).Trace("<<<< GetNode")

	node, err := h.kubeClient.CoreV1().Nodes().Get(ctx, nodeName, getOpts)
	if err != nil {
		return nil, err
	}
	topologyLabels := make(map[string]string)
	for k, v := range node.Labels {
		for _, prefix := range config.TopologyKeyPrefixes {
			if strings.HasPrefix(k, prefix) {
				topologyLabels[k] = v
			}
		}
	}
	return topologyLabels, err
}

// getCachedVolumeReference returns a share (identified by referenced PVC namespace/name) from the client's cache,
// or an error if not found.
func (h *helper) getCachedVolumeReference(
	ctx context.Context, vrefNamespace, pvcName, pvcNamespace string,
) (*netappv1.TridentVolumeReference, error) {
	logFields := LogFields{"vrefNamespace": vrefNamespace, "pvcName": pvcName, "pvcNamespace": pvcNamespace}
	Logc(ctx).WithFields(logFields).Trace(">>>> getCachedVolumeReference")
	defer Logc(ctx).Trace("<<<< getCachedVolumeReference")

	key := vrefNamespace + "_" + pvcNamespace + "/" + pvcName
	items, err := h.vrefIndexer.ByIndex(vrefIndex, key)
	if err != nil {
		Logc(ctx).WithFields(logFields).Error("Could not search cache for volume reference.")
		return nil, fmt.Errorf("could not search cache for volume reference %s: %v", key, err)
	} else if len(items) == 0 {
		Logc(ctx).WithFields(logFields).Debug("volume reference object not found in cache.")
		return nil, fmt.Errorf("volume reference %s not found in cache", key)
	} else if len(items) > 1 {
		Logc(ctx).WithFields(logFields).Error("Multiple cached volume reference objects found.")
		return nil, fmt.Errorf("multiple volume reference objects with key %s found in cache", key)
	} else if vref, ok := items[0].(*netappv1.TridentVolumeReference); !ok {
		Logc(ctx).WithFields(logFields).Error("Non-volume-reference cached object found.")
		return nil, fmt.Errorf("non-volume-reference object %s found in cache", key)
	} else {
		Logc(ctx).WithFields(logFields).Debug("Found cached volume reference.")
		return vref, nil
	}
}

// SupportsFeature accepts a CSI feature and returns true if the
// feature exists and is supported.
func (h *helper) SupportsFeature(ctx context.Context, feature controllerhelpers.Feature) bool {
	Logc(ctx).Trace(">>>> SupportsFeature")
	defer Logc(ctx).Trace("<<<< SupportsFeature")

	kubeSemVersion, err := versionutils.ParseSemantic(h.kubeVersion.GitVersion)
	if err != nil {
		Logc(ctx).WithField("version", h.kubeVersion.GitVersion).WithError(err).Errorf(
			"Unable to parse Kubernetes version.")
		return false
	}

	if minVersion, ok := features[feature]; ok {
		return kubeSemVersion.AtLeast(minVersion)
	} else {
		return false
	}
}

func (h *helper) IsTopologyInUse(ctx context.Context) bool {
	Logc(ctx).Trace(">>>> IsTopologyInUse")
	defer Logc(ctx).Trace("<<<< IsTopologyInUse")

	// Get one node with a region topology label.
	listOpts := metav1.ListOptions{
		LabelSelector: csi.K8sTopologyRegionLabel,
		Limit:         1,
	}

	nodes, err := h.kubeClient.CoreV1().Nodes().List(ctx, listOpts)
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to list nodes with topology label. Assuming topology in use to be 'false' by default.")
		return false
	}

	// If there exists even a single node with topology label, we consider topology to be in use.
	topologyInUse := false
	if nodes != nil && len(nodes.Items) > 0 {
		topologyInUse = true
	}

	fields := LogFields{"topologyInUse": topologyInUse}
	Logc(ctx).WithFields(fields).Info("Successfully determined if topology is in use.")

	return topologyInUse
}
