// Copyright 2020 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8sstoragev1 "k8s.io/api/storage/v1"
	k8sstoragev1beta "k8s.io/api/storage/v1beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sversion "k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"

	clik8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend"
	k8sclient "github.com/netapp/trident/k8s_client"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	trident_informers "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions"
	"github.com/netapp/trident/storage"
	storageattribute "github.com/netapp/trident/storage_attribute"
	storageclass "github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils"
)

type KubernetesPlugin interface {
	frontend.Plugin
	ImportVolume(ctx context.Context, request *storage.ImportVolumeRequest) (*storage.VolumeExternal, error)
}

// StorageClassSummary captures relevant fields in the storage class that are needed during PV creation or PV resize.
type StorageClassSummary struct {
	Parameters                    map[string]string
	MountOptions                  []string
	PersistentVolumeReclaimPolicy *v1.PersistentVolumeReclaimPolicy
	AllowVolumeExpansion          *bool
}

type Plugin struct {
	orchestrator             core.Orchestrator
	kubeClient               kubernetes.Interface
	crdClient                *versioned.Clientset
	getNamespacedKubeClient  func(*rest.Config, string) (k8sclient.Interface, error)
	kubeConfig               rest.Config
	eventRecorder            record.EventRecorder
	claimController          cache.Controller
	claimControllerStopChan  chan struct{}
	claimSource              cache.ListerWatcher
	volumeController         cache.Controller
	volumeControllerStopChan chan struct{}
	volumeSource             cache.ListerWatcher
	classController          cache.Controller
	classControllerStopChan  chan struct{}
	classSource              cache.ListerWatcher
	resizeController         cache.Controller
	resizeControllerStopChan chan struct{}
	resizeSource             cache.ListerWatcher
	mutex                    *sync.Mutex
	pendingClaimMatchMap     map[string]*v1.PersistentVolume
	kubernetesVersion        *k8sversion.Info
	defaultStorageClasses    map[string]bool
	storageClassCache        map[string]*StorageClassSummary
	tridentNamespace         string
	crdInformerFactory       trident_informers.SharedInformerFactory
}

func NewPlugin(o core.Orchestrator, apiServerIP, kubeConfigPath string) (*Plugin, error) {

	ctx := GenerateRequestContext(nil, "", ContextSourceInternal)

	kubeConfig, err := clientcmd.BuildConfigFromFlags(apiServerIP, kubeConfigPath)
	if err != nil {
		return nil, err
	}

	// Create the CLI-based Kubernetes client
	client, err := clik8sclient.NewKubectlClient("", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	// when running in binary mode, we use the current namespace as determined by the CLI client
	return newKubernetesPlugin(ctx, o, kubeConfig, client.Namespace())
}

func NewPluginInCluster(o core.Orchestrator) (*Plugin, error) {

	ctx := GenerateRequestContext(nil, "", ContextSourceInternal)

	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	// when running in a pod, we use the Trident pod's namespace
	bytes, err := ioutil.ReadFile(config.TridentNamespaceFile)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"error":         err,
			"namespaceFile": config.TridentNamespaceFile,
		}).Fatal("Kubernetes frontend failed to obtain Trident's namespace!")
	}
	tridentNamespace := string(bytes)

	return newKubernetesPlugin(ctx, o, kubeConfig, tridentNamespace)
}

func newKubernetesPlugin(
	ctx context.Context, orchestrator core.Orchestrator, kubeConfig *rest.Config, tridentNamespace string,
) (*Plugin, error) {

	Logc(ctx).WithFields(log.Fields{
		"namespace": tridentNamespace,
	}).Info("Initializing Kubernetes frontend.")

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	crdClient, err := versioned.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	ret := &Plugin{
		orchestrator:             orchestrator,
		kubeClient:               kubeClient,
		crdClient:                crdClient,
		getNamespacedKubeClient:  k8sclient.NewKubeClient,
		kubeConfig:               *kubeConfig,
		claimControllerStopChan:  make(chan struct{}),
		volumeControllerStopChan: make(chan struct{}),
		classControllerStopChan:  make(chan struct{}),
		resizeControllerStopChan: make(chan struct{}),
		mutex:                    &sync.Mutex{},
		pendingClaimMatchMap:     make(map[string]*v1.PersistentVolume),
		defaultStorageClasses:    make(map[string]bool, 1),
		storageClassCache:        make(map[string]*StorageClassSummary),
		tridentNamespace:         tridentNamespace,
	}

	ret.kubernetesVersion, err = kubeClient.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("Kubernetes frontend couldn't retrieve API server's version: %v", err)
	}
	Logc(ctx).WithFields(log.Fields{
		"version":    ret.kubernetesVersion.Major + "." + ret.kubernetesVersion.Minor,
		"gitVersion": ret.kubernetesVersion.GitVersion,
	}).Info("Kubernetes frontend determined the container orchestrator version.")
	_, err = ValidateKubeVersion(ret.kubernetesVersion)
	if err != nil {
		if IsPanicKubeVersion(err) {
			return nil, fmt.Errorf("Kubernetes frontend couldn't validate Kubernetes version: %v", err)
		}
		if IsUnsupportedKubeVersion(err) {
			Logc(ctx).Warnf("%s v%s may not support container orchestrator version %s.%s (%s)! "+
				"Supported versions for Kubernetes are %s-%s.", config.OrchestratorName,
				config.OrchestratorVersion, ret.kubernetesVersion.Major, ret.kubernetesVersion.Minor,
				ret.kubernetesVersion.GitVersion, config.KubernetesVersionMin, config.KubernetesVersionMax)
			Logc(ctx).Warnf("Kubernetes frontend proceeds as if you are running Kubernetes %s!",
				config.KubernetesVersionMax)
		}
	}

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(
		&corev1.EventSinkImpl{
			Interface: kubeClient.CoreV1().Events(""),
		})
	ret.eventRecorder = broadcaster.NewRecorder(scheme.Scheme, v1.EventSource{Component: AnnOrchestrator})

	// Setting up a watch for PVCs
	ret.claimSource = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return kubeClient.CoreV1().PersistentVolumeClaims(v1.NamespaceAll).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.CoreV1().PersistentVolumeClaims(v1.NamespaceAll).Watch(ctx, options)
		},
	}
	_, ret.claimController = cache.NewInformer(
		ret.claimSource,
		&v1.PersistentVolumeClaim{},
		KubernetesSyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ret.addClaim,
			UpdateFunc: ret.updateClaim,
			DeleteFunc: ret.deleteClaim,
		},
	)

	// Setting up a watch for PVs
	ret.volumeSource = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return kubeClient.CoreV1().PersistentVolumes().List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.CoreV1().PersistentVolumes().Watch(ctx, options)
		},
	}
	_, ret.volumeController = cache.NewInformer(
		ret.volumeSource,
		&v1.PersistentVolume{},
		KubernetesSyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ret.addVolume,
			UpdateFunc: ret.updateVolume,
			DeleteFunc: ret.deleteVolume,
		},
	)

	// Setting up a watch for storage classes
	ret.classSource = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return kubeClient.StorageV1().StorageClasses().List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.StorageV1().StorageClasses().Watch(ctx, options)
		},
	}
	_, ret.classController = cache.NewInformer(
		ret.classSource,
		&k8sstoragev1.StorageClass{},
		KubernetesSyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ret.addClass,
			UpdateFunc: ret.updateClass,
			DeleteFunc: ret.deleteClass,
		},
	)

	// Setting up a watch for PVCs to detect resize
	ret.resizeSource = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return kubeClient.CoreV1().PersistentVolumeClaims(v1.NamespaceAll).List(ctx, options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.CoreV1().PersistentVolumeClaims(v1.NamespaceAll).Watch(ctx, options)
		},
	}
	_, ret.resizeController = cache.NewInformer(
		ret.resizeSource,
		&v1.PersistentVolumeClaim{},
		KubernetesResizeSyncPeriod,
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: ret.updateClaimResize,
		},
	)

	return ret, nil
}

func (p *Plugin) Activate() error {
	log.Info("Activating Kubernetes frontend.")
	go p.claimController.Run(p.claimControllerStopChan)
	go p.volumeController.Run(p.volumeControllerStopChan)
	go p.classController.Run(p.classControllerStopChan)
	go p.resizeController.Run(p.resizeControllerStopChan)

	// Configure telemetry
	config.OrchestratorTelemetry.Platform = string(config.PlatformKubernetes)
	config.OrchestratorTelemetry.PlatformVersion = p.Version()

	return nil
}

func (p *Plugin) Deactivate() error {
	log.Info("Deactivating Kubernetes frontend.")
	close(p.claimControllerStopChan)
	close(p.volumeControllerStopChan)
	close(p.classControllerStopChan)
	close(p.resizeControllerStopChan)
	return nil
}

func (p *Plugin) GetName() string {
	return string(config.ContextKubernetes)
}

func (p *Plugin) Version() string {
	return p.kubernetesVersion.GitVersion
}

func (p *Plugin) addClaim(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S)

	claim, ok := obj.(*v1.PersistentVolumeClaim)
	if !ok {
		Logc(ctx).Errorf("Kubernetes frontend expected PVC; handler got %v", obj)
		return
	}
	p.processClaim(ctx, claim, "add")
}

func (p *Plugin) updateClaim(oldObj, newObj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S)

	claim, ok := newObj.(*v1.PersistentVolumeClaim)
	if !ok {
		Logc(ctx).Errorf("Kubernetes frontend expected PVC; handler got %v", newObj)
		return
	}
	p.processClaim(ctx, claim, "update")
}

func (p *Plugin) deleteClaim(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S)

	claim, ok := obj.(*v1.PersistentVolumeClaim)
	if !ok {
		Logc(ctx).Errorf("Kubernetes frontend expected PVC; handler got %v", obj)
		return
	}
	p.processClaim(ctx, claim, "delete")
}

func (p *Plugin) validStorageClassReceived(ctx context.Context, claim *v1.PersistentVolumeClaim) error {

	// Filter unrelated claims
	if claim.Spec.StorageClassName != nil && *claim.Spec.StorageClassName == "" {
		Logc(ctx).WithFields(log.Fields{
			"PVC": claim.Name,
		}).Debug("Kubernetes frontend ignored this PVC as an empty string " +
			"was specified for the storage class!")
		return fmt.Errorf("ignored PVC %s as an empty string was specified for the storage class", claim.Name)
	}

	return nil
}

func (p *Plugin) validClaimReceived(ctx context.Context, claim *v1.PersistentVolumeClaim, provisioner string) error {

	// Reject PVCs whose provisioner was not set to Trident.
	// However, don't reject a PVC whose provisioner wasn't set
	// just because no default storage class existed at the time
	// of the PVC creation.
	if provisioner != AnnOrchestrator &&
		GetPersistentVolumeClaimClass(claim) != "" {
		Logc(ctx).WithFields(log.Fields{
			"PVC":             claim.Name,
			"PVC_provisioner": provisioner,
		}).Debugf("Kubernetes frontend ignored this PVC as it's not "+
			"tagged with %s as the storage provisioner!", AnnOrchestrator)
		return fmt.Errorf("ignored PVC %s as it's not tagged with %s as the storage provisioner",
			claim.Name, AnnOrchestrator)
	}

	// Check if the default storage class should be used.
	if GetPersistentVolumeClaimClass(claim) == "" && claim.Spec.StorageClassName == nil {
		p.mutex.Lock()
		if len(p.defaultStorageClasses) == 1 {
			// Using the default storage class
			var defaultStorageClassName string
			for defaultStorageClassName = range p.defaultStorageClasses {
				claim.Spec.StorageClassName = &defaultStorageClassName
				break
			}
			Logc(ctx).WithFields(log.Fields{
				"PVC":                  claim.Name,
				"storageClass_default": defaultStorageClassName,
			}).Info("Kubernetes frontend will use the default storage class for this PVC.")
		} else if len(p.defaultStorageClasses) > 1 {
			Logc(ctx).WithFields(log.Fields{
				"PVC":                    claim.Name,
				"default_storageClasses": p.getDefaultStorageClasses(),
			}).Warn("Kubernetes frontend ignored this PVC as more than " +
				"one default storage class has been configured!")
			p.mutex.Unlock()
			return fmt.Errorf("ignored PVC %s as more than one default storage class has been configured",
				claim.Name)
		} else {
			Logc(ctx).WithField("PVC", claim.Name).Debug("Kubernetes frontend ignored this PVC as no storage class " +
				"was specified and no default storage class was configured!")
			p.mutex.Unlock()
			return fmt.Errorf("ignored PVC %s as no storage class was specified and no default storage class was configured",
				claim.Name)
		}
		p.mutex.Unlock()
	}

	// If storage class is still empty (after considering the default storage
	// class), ignore the PVC as Kubernetes doesn't allow a storage class with
	// empty string as the name.
	if GetPersistentVolumeClaimClass(claim) == "" {
		Logc(ctx).WithField("PVC", claim.Name).Debug("Kubernetes frontend ignored this PVC as no storage class was specified!")
		return fmt.Errorf("ignored PVC %s as no storage class was specified", claim.Name)
	}

	return nil
}

func (p *Plugin) isClaimNotManaged(ctx context.Context, claim *v1.PersistentVolumeClaim) error {
	return p.isNotManaged(ctx, claim.Annotations, claim.Name, "PVC")
}

func (p *Plugin) isVolumeNotManaged(ctx context.Context, pv *v1.PersistentVolume) error {
	return p.isNotManaged(ctx, pv.Annotations, pv.Name, "PV")
}

func (p *Plugin) isNotManaged(ctx context.Context, annotations map[string]string, name string, kind string) error {

	// If the notManaged annotation is set to true then this PVC isn't processed any further.
	if value, ok := annotations[AnnNotManaged]; ok {
		notManaged, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s annotation set with invalid value: %v", AnnNotManaged, err)
		}

		if notManaged {
			Logc(ctx).WithField(kind, name).Debugf("Kubernetes frontend ignored this notManaged %s.", kind)
			return fmt.Errorf("ignored %s %s as it's set with annotation %s", kind, name, AnnNotManaged)
		}
	}
	return nil
}

func (p *Plugin) processClaim(ctx context.Context, claim *v1.PersistentVolumeClaim, eventType string) {

	// Validating the claim
	size, ok := claim.Spec.Resources.Requests[v1.ResourceStorage]
	if !ok {
		return
	}
	Logc(ctx).WithFields(log.Fields{
		"PVC":              claim.Name,
		"PVC_phase":        claim.Status.Phase,
		"PVC_size":         size.String(),
		"PVC_uid":          claim.UID,
		"PVC_storageClass": GetPersistentVolumeClaimClass(claim),
		"PVC_accessModes":  claim.Spec.AccessModes,
		"PVC_annotations":  claim.Annotations,
		"PVC_volume":       claim.Spec.VolumeName,
		"PVC_eventType":    eventType,
	}).Debug("Kubernetes frontend got notified of a PVC.")

	if p.validStorageClassReceived(ctx, claim) != nil {
		return
	}

	if p.isClaimNotManaged(ctx, claim) != nil {
		return
	}

	provisioner := getClaimProvisioner(claim)
	if p.validClaimReceived(ctx, claim, provisioner) != nil {
		return
	}

	// It's a valid PVC.
	switch eventType {
	case "delete":
		p.processDeletedClaim(claim)
		return
	case "add":
	case "update":
	default:
		Logc(ctx).WithFields(log.Fields{
			"PVC":   claim.Name,
			"event": eventType,
		}).Error("Kubernetes frontend didn't recognize the notification event corresponding to the PVC!")
		return
	}

	// Treating add and update events similarly.
	// Making decisions based on a claim's phase, similar to k8s' persistent volume controller.
	switch claim.Status.Phase {
	case v1.ClaimBound:
		p.processBoundClaim(ctx, claim)
		return
	case v1.ClaimLost:
		p.processLostClaim(claim)
		return
	case v1.ClaimPending:
		// As of Kubernetes 1.6, selector and storage class are mutually exclusive.
		if claim.Spec.Selector != nil {
			message := "ignored PVCs with label selectors!"
			p.updatePVCWithEvent(claim, v1.EventTypeWarning, "IgnoredClaim", message)
			Logc(ctx).WithField("PVC", claim.Name).Debugf("Kubernetes frontend %s", message)
			return
		}
		if _, ok := claim.Annotations[AnnNotManaged]; ok {
			Logc(ctx).WithFields(log.Fields{
				"PVC":        claim.Name,
				"notManaged": claim.Annotations[AnnNotManaged],
			}).Debug("Kubernetes frontend ignored PVC, in Pending phase, created via volume import")
			return
		}
		p.processPendingClaim(ctx, claim)
	default:
		Logc(ctx).WithFields(log.Fields{
			"PVC":       claim.Name,
			"PVC_phase": claim.Status.Phase,
		}).Error("Kubernetes frontend doesn't recognize the claim phase.")
	}
}

// processBoundClaim validates whether a Trident-created PV got bound to the intended PVC.
func (p *Plugin) processBoundClaim(ctx context.Context, claim *v1.PersistentVolumeClaim) {

	orchestratorClaimName := getUniqueClaimName(claim)
	deleteClaim := true

	defer func() {
		// Remove the pending claim, if present.
		if deleteClaim {
			p.mutex.Lock()
			delete(p.pendingClaimMatchMap, orchestratorClaimName)
			p.mutex.Unlock()
		}
	}()

	p.mutex.Lock()
	pv, ok := p.pendingClaimMatchMap[orchestratorClaimName]
	p.mutex.Unlock()
	if !ok {
		// Ignore the claim if we have no record of it.
		return
	}
	// If the bound volume name doesn't match the volume we provisioned,
	// we need to delete the PV and its backing volume, since something
	// else (e.g., an admin-provisioned volume or another provisioner)
	// was able to take care of the PVC.
	// Names are unique for a given instance in time (volumes aren't namespaced,
	// so namespace is a nonissue), so we only need to check whether the
	// name matches.
	boundVolumeName := claim.Spec.VolumeName
	if pv.Name != boundVolumeName {
		err := p.deleteVolumeAndPV(ctx, pv)
		if err != nil {
			deleteClaim = false
			Logc(ctx).WithFields(log.Fields{
				"PVC":        claim.Name,
				"PVC_volume": boundVolumeName,
				"PV":         pv.Name,
				"PV_volume":  pv.Name,
			}).Warnf("Kubernetes frontend detected PVC isn't bound to the intended PV, but it failed to "+
				"delete the PV or the corresponding provisioned volume (will retry upon resync): %s", err.Error())
			return
		}
		Logc(ctx).WithFields(log.Fields{
			"PVC":        claim.Name,
			"PVC_volume": boundVolumeName,
			"PV":         pv.Name,
			"PV_volume":  pv.Name,
		}).Info("Kubernetes frontend deleted the provisioned volume ",
			"as the intended PVC was bound to a different volume.")
		return
	}
	// The names match, so the PVC is successfully bound to the provisioned PV.
	return
}

// processLostClaim cleans up Trident-created PVs.
func (p *Plugin) processLostClaim(claim *v1.PersistentVolumeClaim) {
	volName := getUniqueClaimName(claim)
	p.mutex.Lock()
	delete(p.pendingClaimMatchMap, volName)
	p.mutex.Unlock()
	return
}

// processDeletedClaim cleans up Trident-created PVs.
func (p *Plugin) processDeletedClaim(claim *v1.PersistentVolumeClaim) {
	// No major action needs to be taken as deleting a claim would result in
	// the corresponding PV to end up in the "Released" phase, which gets
	// handled by processUpdatedVolume.
	// Remove the pending claim, if present.
	p.mutex.Lock()
	delete(p.pendingClaimMatchMap, getUniqueClaimName(claim))
	p.mutex.Unlock()
}

// processPendingClaim processes PVCs in the pending phase.
func (p *Plugin) processPendingClaim(ctx context.Context, claim *v1.PersistentVolumeClaim) {

	orchestratorClaimName := getUniqueClaimName(claim)
	p.mutex.Lock()

	// Check whether we have already provisioned a PV for this claim
	if pv, ok := p.pendingClaimMatchMap[orchestratorClaimName]; ok {
		// If there's an entry for this claim in the pending claim match
		// map, we need to see if the volume that we allocated can actually
		// fit the (now modified) specs for the claim.  Note that by checking
		// whether the volume was bound before we get here, we're assuming
		// users don't alter the specs on their PVC after it's been bound.
		// Note that as of Kubernetes 1.5, this case isn't possible, as PVC
		// specs are immutable.  This remains in case Kubernetes allows PVC
		// modification again.
		if canPVMatchWithPVC(ctx, pv, claim) {
			p.mutex.Unlock()
			Logc(ctx).WithFields(log.Fields{
				"PV name":    pv.Name,
				"PV status":  pv.Status,
				"PVC name":   claim.Name,
				"PVC status": claim.Status,
			}).Debug("Kubernetes frontend verified PV specs fit PVC")
			return
		}
		// Otherwise, we need to delete the old volume and allocate a new one
		if err := p.deleteVolumeAndPV(ctx, pv); err != nil {
			Logc(ctx).WithFields(log.Fields{
				"PVC": claim.Name,
				"PV":  pv.Name,
			}).Error("Kubernetes frontend failed in processing an updated claim (will try again upon resync): ", err)
			p.mutex.Unlock()
			return
		}
		delete(p.pendingClaimMatchMap, orchestratorClaimName)
	}
	p.mutex.Unlock()

	// We need to provision a new volume for this claim.
	pv, err := p.createVolumeAndPV(ctx, orchestratorClaimName, claim)
	if err != nil {
		if pv == nil {
			p.updatePVCWithEvent(claim, v1.EventTypeNormal, "ProvisioningFailed", err.Error())
		} else {
			p.updatePVCWithEvent(claim, v1.EventTypeWarning, "ProvisioningFailed", err.Error())
		}
		return
	}
	p.mutex.Lock()
	p.pendingClaimMatchMap[orchestratorClaimName] = pv
	p.mutex.Unlock()
	message := "provisioned a volume and a PV for the PVC."
	p.updatePVCWithEvent(claim, v1.EventTypeNormal, "ProvisioningSuccess", message)
	Logc(ctx).WithFields(log.Fields{
		"PVC":       claim.Name,
		"PV":        orchestratorClaimName,
		"PV_volume": pv.Name,
	}).Infof("Kubernetes frontend %s", message)
}

func (p *Plugin) ImportVolume(ctx context.Context, request *storage.ImportVolumeRequest) (*storage.VolumeExternal, error) {

	Logc(ctx).WithField("request", request).Debug("ImportVolume")

	// Get PVC from ImportVolumeRequest
	jsonData, err := base64.StdEncoding.DecodeString(request.PVCData)
	if err != nil {
		return nil, fmt.Errorf("the pvcData field does not contain valid base64-encoded data: %v", err)
	}

	claim := &v1.PersistentVolumeClaim{}
	err = json.Unmarshal(jsonData, &claim)
	if err != nil {
		return nil, fmt.Errorf("could not parse JSON PVC: %v", err)
	}

	Logc(ctx).WithFields(log.Fields{
		"PVC":               claim.Name,
		"PVC_storageClass":  GetPersistentVolumeClaimClass(claim),
		"PVC_accessModes":   claim.Spec.AccessModes,
		"PVC_annotations":   claim.Annotations,
		"PVC_volume":        claim.Spec.VolumeName,
		"PVC_requestedSize": claim.Spec.Resources.Requests[v1.ResourceStorage],
	}).Debug("ImportVolume: received PVC data.")

	if err = p.validStorageClassReceived(ctx, claim); err != nil {
		return nil, err
	}

	provisioner := p.getClaimOrClassProvisioner(ctx, claim)
	if err = p.validClaimReceived(ctx, claim, provisioner); err != nil {
		return nil, err
	}

	if len(claim.Namespace) == 0 {
		return nil, fmt.Errorf("a valid PVC namespace is required for volume import")
	}

	// Set required annotations
	if claim.Annotations == nil {
		claim.Annotations = map[string]string{}
	}
	claim.Annotations[AnnNotManaged] = strconv.FormatBool(request.NoManage)
	claim.Annotations[AnnStorageProvisioner] = AnnOrchestrator

	// Set the PVC's storage field to the actual volume size.
	volExternal, err := p.orchestrator.GetVolumeExternal(ctx, request.InternalName, request.Backend)
	if err != nil {
		return nil, fmt.Errorf("volume import failed to get size of volume: %v", err)
	}

	if claim.Spec.Resources.Requests == nil {
		claim.Spec.Resources.Requests = v1.ResourceList{}
	}
	claim.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(volExternal.Config.Size)
	Logc(ctx).WithFields(log.Fields{
		"size":      volExternal.Config.Size,
		"claimSize": claim.Spec.Resources.Requests[v1.ResourceStorage],
	}).Debug("Volume import determined volume size")

	// Need to create the PVC to obtain the PVC UID. The PVC UID is needed to create the volume name
	// used for the PV Name and volume.Config.Name.
	pvc, pvcErr := p.createImportPVC(ctx, claim)
	if pvcErr != nil {
		Logc(ctx).WithFields(log.Fields{
			"claim": claim.Name,
			"error": pvcErr,
		}).Warn("ImportVolume: error occurred during PVC creation.")
		return nil, pvcErr
	}
	Logc(ctx).WithField("PVC", pvc).Debug("ImportVolume: created pending PVC.")

	accessModes := pvc.Spec.AccessModes
	volumeMode := pvc.Spec.VolumeMode
	uniqueName := getUniqueClaimName(pvc)
	storageClass := GetPersistentVolumeClaimClass(pvc)
	annotations := p.processStorageClassAnnotations(pvc, storageClass)
	volConfig := getVolumeConfig(accessModes, volumeMode, uniqueName,
		claim.Spec.Resources.Requests[v1.ResourceStorage], annotations)
	volConfig.ImportOriginalName = request.InternalName
	volConfig.ImportBackendUUID = volExternal.BackendUUID
	volConfig.ImportNotManaged = request.NoManage

	// We don't really know what filesystem is applied to imported volumes, so to be safe we shouldn't set it
	volConfig.FileSystem = ""

	// This func is passed to core when ImportVolume is called. The func is created with the locally scoped
	// context but runs in orchestrator_core inside a volume transaction. This allows needed cleanup to be
	// performed if an error is thrown. The createPVandPVC creates the PV and checks the PVC status.
	createPVandPVC := func(volExternal *storage.VolumeExternal, driverType string) error {
		Logc(ctx).WithFields(log.Fields{
			"volumeName":         volExternal.Config.Name,
			"volumeInternalName": volExternal.Config.InternalName,
			"size":               volExternal.Config.Size,
			"fileSystem":         volExternal.Config.FileSystem,
			"snapshotPolicy":     volExternal.Config.SnapshotPolicy,
			"protocol":           volExternal.Config.Protocol,
			"backendUUID":        volExternal.BackendUUID,
		}).Debug("ImportVolume: ready to create the PV.")

		pv, pvErr := p.createPV(ctx, pvc, volConfig, storageClass, volExternal, true, driverType)
		if pvErr != nil {
			Logc(ctx).WithFields(log.Fields{
				"volume": volExternal.Config.Name,
				"error":  pvErr,
			}).Warn("ImportVolume: error occurred during PV creation.")
			return pvErr
		}

		// Validate that the PV specs fit the PVC specs. This is just a sanity check as
		// canPVMatchWithPVC should not fail.
		if !canPVMatchWithPVC(ctx, pv, pvc) {
			p.deletePV(ctx, pv.Name)
			return fmt.Errorf("the PV specs do not fit requested PVC")
		}

		pvCheck, err := p.kubeClient.CoreV1().PersistentVolumes().Get(ctx, pv.Name, getOpts)
		if err != nil {
			return fmt.Errorf("error occurred checking PV %s status: %v", pv.Name, err)
		}
		Logc(ctx).WithFields(log.Fields{
			"pv.Name":         pvCheck.Name,
			"pv.Status.Phase": pvCheck.Status.Phase,
		}).Debug("ImportVolume: PV created.")

		pvcCheck, err := p.kubeClient.CoreV1().PersistentVolumeClaims(pvc.Namespace).Get(ctx, pvc.Name, getOpts)
		if err != nil {
			return fmt.Errorf("error occurred checking PVC %s status: %v", pvc.Name, err)
		}
		Logc(ctx).WithFields(log.Fields{
			"pvc.Name":         pvcCheck.Name,
			"pvc.Status.Phase": pvcCheck.Status.Phase,
		}).Debug("ImportVolume: PVC status.")

		return nil
	}

	volumeExternal, err := p.orchestrator.LegacyImportVolume(ctx, volConfig, request.Backend, request.NoManage,
		createPVandPVC)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"name":     volConfig.Name,
			"PVC.UID":  pvc.UID,
			"PVC.Name": pvc.Name,
		}).Warn("ImportVolume: error occurred; deleting PVC.")
		p.deletePVC(ctx, pvc)
		return nil, fmt.Errorf("frontend failed to import volume: %v", err)
	}
	Logc(ctx).WithField("volumeExternal", volumeExternal).Debug("ImportVolume: import completed.")

	return volumeExternal, nil
}

func (p *Plugin) deletePV(ctx context.Context, pvName string) {

	gracePeriod := int64(0)
	do := metav1.DeleteOptions{GracePeriodSeconds: &gracePeriod}
	err := p.kubeClient.CoreV1().PersistentVolumes().Delete(ctx, pvName, do)
	if err != nil {
		Logc(ctx).Errorf("error occurred while trying to delete PV %s: %v", pvName, err)
	}
}

func (p *Plugin) deletePVC(ctx context.Context, pvc *v1.PersistentVolumeClaim) {

	err := p.kubeClient.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(ctx, pvc.Name, deleteOpts)
	if err != nil {
		Logc(ctx).Errorf("error occurred while trying to delete PVC %s: %v", pvc.Name, err)
	}
}

func (p *Plugin) createImportPVC(
	ctx context.Context, claim *v1.PersistentVolumeClaim) (*v1.PersistentVolumeClaim, error) {

	Logc(ctx).WithFields(log.Fields{
		"claim":     claim,
		"namespace": claim.Namespace,
	}).Debug("CreateImportPVC: ready to create PVC")

	pvc, err := p.kubeClient.CoreV1().PersistentVolumeClaims(claim.Namespace).Create(ctx, claim, createOpts)
	if err != nil {
		return nil, fmt.Errorf("error occurred during PVC creation: %v", err)
	}

	return pvc, nil
}

func (p *Plugin) createVolumeFromConfig(
	ctx context.Context,
	volConfig *storage.VolumeConfig,
	storageClass string,
	namespace string,
	claimName string,
) (*storage.VolumeExternal, error) {

	var vol *storage.VolumeExternal

	k8sClient, err := p.getNamespacedKubeClient(&p.kubeConfig, namespace)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"namespace": namespace,
			"error":     err.Error(),
		}).Warn("Kubernetes frontend couldn't create a client to namespace!")
	}

	if volConfig.CloneSourceVolume == "" {
		vol, err = p.orchestrator.AddVolume(ctx, volConfig)
		if err != nil {
			return nil, err
		}
	} else {
		var (
			options metav1.GetOptions
			pvc     *v1.PersistentVolumeClaim
		)

		// If cloning an existing PVC, process the source PVC name:
		// 1) Validate that the source PVC is in the same namespace.
		//    TODO: Explore the security and management ramifications of cloning from a PVC in a different namespace.
		if pvc, err = k8sClient.GetPVC(volConfig.CloneSourceVolume, options); err != nil {
			err = fmt.Errorf("cloning from a PVC requires both PVCs be in the same namespace")
			Logc(ctx).WithFields(log.Fields{
				"sourcePVC":     volConfig.CloneSourceVolume,
				"PVC":           claimName,
				"PVC_namespace": namespace,
			}).Debugf("Kubernetes frontend detected an invalid configuration "+
				"for cloning from a PVC: %v", err.Error())
			return nil, err
		}

		// 2) Validate that storage classes match for the two PVCs
		if GetPersistentVolumeClaimClass(pvc) != storageClass {
			err = fmt.Errorf("cloning from a PVC requires matching storage classes")
			Logc(ctx).WithFields(log.Fields{
				"PVC":                    claimName,
				"PVC_storageClass":       storageClass,
				"sourcePVC":              volConfig.CloneSourceVolume,
				"sourcePVC_storageClass": GetPersistentVolumeClaimClass(pvc),
			}).Debugf("Kubernetes frontend detected an invalid configuration "+
				"for cloning from a PVC: %v", err.Error())
			return nil, err
		}

		// 3) Validate that the requested size is <= to the source's size
		pvcSize := pvc.Spec.Resources.Requests[v1.ResourceStorage]
		srcSize := pvcSize.Value()
		reqSize, err := strconv.ParseInt(volConfig.Size, 10, 64)
		if err != nil {
			err = fmt.Errorf("could not read requested size")
			return nil, err
		}

		if srcSize < reqSize {
			err = fmt.Errorf("requested PVC size is too large for the clone source")
			Logc(ctx).WithFields(log.Fields{
				"reqSize": reqSize,
				"srcSize": srcSize,
			}).Error("requested PVC size is too large for the clone source")
			return nil, err
		}

		// 4) Set the source PVC name as it's understood by Trident.
		volConfig.CloneSourceVolume = getUniqueClaimName(pvc)

		// 5) Clone the existing volume
		vol, err = p.orchestrator.CloneVolume(ctx, volConfig)
		if err != nil {
			return nil, err
		}
	}
	return vol, nil
}

func (p *Plugin) createPV(
	ctx context.Context,
	claim *v1.PersistentVolumeClaim,
	volConfig *storage.VolumeConfig,
	storageClass string,
	volume *storage.VolumeExternal,
	isImport bool,
	driverType string,
) (pv *v1.PersistentVolume, err error) {

	var (
		claimRef    v1.ObjectReference
		iscsiSource *v1.ISCSIPersistentVolumeSource
		nfsSource   *v1.NFSVolumeSource
	)

	claimRef = v1.ObjectReference{
		Namespace: claim.Namespace,
		Name:      claim.Name,
		UID:       claim.UID,
	}

	size, err := strconv.ParseInt(volume.Config.Size, 10, 64)
	if err != nil {
		return nil, err
	}
	pv = &v1.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: volConfig.Name,
			Annotations: map[string]string{
				AnnClass:                  storageClass,
				AnnDynamicallyProvisioned: AnnOrchestrator,
			},
		},
		Spec: v1.PersistentVolumeSpec{
			AccessModes: claim.Spec.AccessModes,
			Capacity:    v1.ResourceList{v1.ResourceStorage: *resource.NewQuantity(size, resource.BinarySI)},
			ClaimRef:    &claimRef,
			// Default policy is "Delete".
			PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
		},
	}

	kubeVersion, _ := ValidateKubeVersion(p.kubernetesVersion)

	pv.Spec.StorageClassName = storageClass

	// Apply Storage Class mount options and reclaim policy
	pv.Spec.MountOptions = p.getPVMountOptions(p.storageClassCache[storageClass], volume)
	scSummaryFromCache, ok := p.storageClassCache[storageClass]
	if !ok {
		err = fmt.Errorf("cannot find storageClassSummary %v in cache", storageClass)
		return nil, err
	}
	pv.Spec.PersistentVolumeReclaimPolicy = *scSummaryFromCache.PersistentVolumeReclaimPolicy

	if isImport {
		// Apply annotation to indicate this PV was created by volume import process
		pv.Annotations[AnnNotManaged] = claim.Annotations[AnnNotManaged]
		// Protect the PV from deletion until the PV and PVC are bound for imported volume.
		pv.Spec.PersistentVolumeReclaimPolicy = v1.PersistentVolumeReclaimRetain
	}

	// We create the CHAP secret in Trident's namespace
	k8sClientCHAP, err := p.getNamespacedKubeClient(&p.kubeConfig, p.tridentNamespace)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"namespace": p.tridentNamespace,
			"error":     err.Error(),
		}).Warn("Kubernetes frontend couldn't create a client to namespace!")
	}

	switch driverType {

	case drivers.SolidfireSANStorageDriverName,
		drivers.OntapSANStorageDriverName,
		drivers.EseriesIscsiStorageDriverName,
		drivers.OntapSANEconomyStorageDriverName:

		iscsiSource, err = CreateISCSIPersistentVolumeSource(ctx, k8sClientCHAP, kubeVersion, volume)
		if err != nil {
			return
		}
		pv.Spec.ISCSI = iscsiSource
		if claim.Spec.VolumeMode != nil &&
			*claim.Spec.VolumeMode == v1.PersistentVolumeBlock {
			pv.Spec.VolumeMode = claim.Spec.VolumeMode
		}

	case drivers.OntapNASStorageDriverName,
		drivers.OntapNASQtreeStorageDriverName,
		drivers.OntapNASFlexGroupStorageDriverName,
		drivers.AWSNFSStorageDriverName,
		drivers.AzureNFSStorageDriverName,
		drivers.GCPNFSStorageDriverName:

		nfsSource = CreateNFSVolumeSource(volume)
		pv.Spec.NFS = nfsSource

	case drivers.FakeStorageDriverName:
		if volume.Config.Protocol == config.File {
			nfsSource = CreateNFSVolumeSource(volume)
			pv.Spec.NFS = nfsSource
		} else if volume.Config.Protocol == config.Block {
			iscsiSource, err = CreateISCSIPersistentVolumeSource(ctx, k8sClientCHAP, kubeVersion, volume)
			if err != nil {
				return
			}
			pv.Spec.ISCSI = iscsiSource
		}

	default:
		// Unknown driver for the frontend plugin or for Kubernetes.
		// Provisioned volume should get deleted.
		volType, _ := p.orchestrator.GetVolumeType(ctx, volume)
		Logc(ctx).WithFields(log.Fields{
			"volume": volume.Config.Name,
			"type":   volType,
			"driver": driverType,
		}).Error("Kubernetes frontend doesn't recognize this type of volume; deleting the provisioned volume.")
		err = fmt.Errorf("unrecognized volume type by Kubernetes")
		return
	}

	pv, err = p.kubeClient.CoreV1().PersistentVolumes().Create(ctx, pv, createOpts)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(log.Fields{
		"volume": volume.Config.Name,
		"driver": driverType,
		"pv":     pv,
	}).Debug("PV created")

	return pv, nil
}

// A pending PVC was received and this func provisions the requested volume
// and creates the PV with the PVC's claim ref.
func (p *Plugin) createVolumeAndPV(
	ctx context.Context, uniqueName string, claim *v1.PersistentVolumeClaim,
) (pv *v1.PersistentVolume, err error) {

	var (
		volExternal *storage.VolumeExternal
	)

	defer func() {
		if volExternal != nil && err != nil {
			err1 := err
			// Delete the volume on the backend
			err = p.orchestrator.DeleteVolume(ctx, volExternal.Config.Name)
			if err != nil {
				err2 := "Kubernetes frontend couldn't delete the volume after failed creation: " + err.Error()
				Logc(ctx).WithFields(log.Fields{
					"volume":      volExternal.Config.Name,
					"backendUUID": volExternal.BackendUUID,
				}).Error(err2)
				err = fmt.Errorf("%s => %s", err1.Error(), err2)
			} else {
				err = err1
			}
		}
	}()

	storageClass := GetPersistentVolumeClaimClass(claim)
	annotations := p.processStorageClassAnnotations(claim, storageClass)

	size, _ := claim.Spec.Resources.Requests[v1.ResourceStorage]
	accessModes := claim.Spec.AccessModes
	volumeMode := claim.Spec.VolumeMode
	volConfig := getVolumeConfig(accessModes, volumeMode, uniqueName, size, annotations)
	volExternal, err = p.createVolumeFromConfig(ctx, volConfig, storageClass, claim.Namespace, claim.Name)
	if err != nil {
		return nil, err
	}
	driverType, _ := p.orchestrator.GetDriverTypeForVolume(ctx, volExternal)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"volume": uniqueName,
		}).Warnf("Kubernetes frontend couldn't provision a volume: %s "+
			"(will retry upon resync)", err.Error())
		return nil, err
	}

	pv, err = p.createPV(ctx, claim, volConfig, storageClass, volExternal, false, driverType)
	if err != nil {
		return nil, err
	}

	return pv, err
}

func (p *Plugin) processStorageClassAnnotations(claim *v1.PersistentVolumeClaim, storageClass string,
) map[string]string {
	var storageClassParams map[string]string

	annotations := claim.Annotations
	if storageClassSummary, found := p.storageClassCache[storageClass]; found {
		storageClassParams = storageClassSummary.Parameters
	}

	// TODO: A quick way to support v1 storage classes before changing unit tests
	if _, found := annotations[AnnClass]; !found {
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[AnnClass] = storageClass
	}

	// Set the file system type based on the value in the storage class
	if _, found := annotations[AnnFileSystem]; !found && storageClassParams != nil {
		if fsType, found := storageClassParams[K8sFsType]; found {
			annotations[AnnFileSystem] = fsType
		}
	}

	return annotations
}

func (p *Plugin) getPVMountOptions(scSummary *StorageClassSummary, volume *storage.VolumeExternal) []string {

	if scSummary != nil && len(scSummary.MountOptions) > 0 {
		return scSummary.MountOptions
	}
	if volume != nil && volume.Config.AccessInfo.MountOptions != "" {
		return regexp.MustCompile(`\s*,\s*`).Split(volume.Config.AccessInfo.MountOptions, -1)
	}
	return make([]string, 0)
}

func (p *Plugin) deleteVolumeAndPV(ctx context.Context, pv *v1.PersistentVolume) error {
	err := p.orchestrator.DeleteVolume(ctx, pv.GetName())
	if err != nil && !utils.IsNotFoundError(err) {
		message := fmt.Sprintf("failed to delete the volume for PV %s: %s. Volume and PV may "+
			"need to be manually deleted.", pv.GetName(), err.Error())
		p.updateVolumePhaseWithEvent(ctx, pv, v1.VolumeFailed, v1.EventTypeWarning, "FailedVolumeDelete", message)
		return fmt.Errorf("Kubernetes frontend %s", message)
	}
	err = p.kubeClient.CoreV1().PersistentVolumes().Delete(ctx, pv.GetName(), deleteOpts)
	if err != nil {
		return fmt.Errorf("Kubernetes frontend failed to contact the API server and delete the PV: %s", err.Error())
	}
	return err
}

func (p *Plugin) addVolume(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S)

	volume, ok := obj.(*v1.PersistentVolume)
	if !ok {
		Logc(ctx).Errorf("Kubernetes frontend expected PV; handler got %v", obj)
		return
	}
	p.processVolume(ctx, volume, "add")
}

func (p *Plugin) updateVolume(oldObj, newObj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S)

	volume, ok := newObj.(*v1.PersistentVolume)
	if !ok {
		Logc(ctx).Errorf("Kubernetes frontend expected PV; handler got %v", newObj)
		return
	}
	p.processVolume(ctx, volume, "update")
}

func (p *Plugin) deleteVolume(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S)

	volume, ok := obj.(*v1.PersistentVolume)
	if !ok {
		Logc(ctx).Errorf("Kubernetes frontend expected PV; handler got %v", obj)
		return
	}
	p.processVolume(ctx, volume, "delete")
}

func (p *Plugin) processVolume(ctx context.Context, pv *v1.PersistentVolume, eventType string) {

	pvSize := pv.Spec.Capacity[v1.ResourceStorage]
	Logc(ctx).WithFields(log.Fields{
		"PV":             pv.Name,
		"PV_phase":       pv.Status.Phase,
		"PV_size":        pvSize.String(),
		"PV_uid":         pv.UID,
		"PV_accessModes": pv.Spec.AccessModes,
		"PV_annotations": pv.Annotations,
		"PV_eventType":   eventType,
	}).Debug("Kubernetes frontend got notified of a PV.")

	// Validating the PV (making sure it's provisioned by Trident)
	if pv.ObjectMeta.Annotations[AnnDynamicallyProvisioned] !=
		AnnOrchestrator {
		return
	}

	if p.isVolumeNotManaged(ctx, pv) != nil {
		return
	}

	if pv.Status.Phase == v1.VolumePending || pv.Status.Phase == v1.VolumeAvailable {
		if _, ok := pv.ObjectMeta.Annotations[AnnNotManaged]; ok {
			Logc(ctx).WithFields(log.Fields{
				"PV":         pv.Name,
				"notManaged": pv.ObjectMeta.Annotations[AnnNotManaged],
			}).Debug("Kubernetes frontend ignored PV, in Pending phase, created via volume import")
			return
		}
	}

	switch eventType {
	case "delete":
		p.processDeletedVolume(ctx, pv)
		return
	case "add", "update":
		p.processUpdatedVolume(ctx, pv)
	default:
		Logc(ctx).WithFields(log.Fields{
			"PV":    pv.Name,
			"event": eventType,
		}).Error("Kubernetes frontend didn't recognize the notification event corresponding to the PV!")
		return
	}
}

// processDeletedVolume cleans up Trident-created PVs.
func (p *Plugin) processDeletedVolume(ctx context.Context, pv *v1.PersistentVolume) {
	// This method can get called under two scenarios:
	// (1) Deletion of a PVC has resulted in deletion of a PV and the
	//     corresponding volume.
	// (2) An admin has deleted the PV before deleting the PVC. Here we handle
	// this case for versions of Kubernetes that don't support the storage
	// object in use protection feature. This feature was beta in v1.10 and
	// stable in v.11.

	// First, make sure the PVC is present and is in the Lost phase.
	claim, err := p.GetPVCForPV(ctx, pv)
	if claim == nil || err != nil {
		return
	}
	if claim.Status.Phase != v1.ClaimLost {
		return
	}

	// Second, take the appropriate action based on the PV's reclaim policy.
	if pv.Spec.PersistentVolumeReclaimPolicy == v1.PersistentVolumeReclaimRetain {
		return
	}

	// Third, we need to delete the corresponding volume.
	if vol, _ := p.orchestrator.GetVolume(ctx, pv.Name); vol == nil {
		return
	}
	err = p.orchestrator.DeleteVolume(ctx, pv.Name)
	if err != nil {
		message := "failed to delete the provisioned volume for the lost PVC."
		p.updatePVCWithEvent(claim, v1.EventTypeWarning, "FailedVolumeDelete", message)
		Logc(ctx).WithFields(log.Fields{
			"PVC":    claim.Name,
			"volume": pv.Name,
		}).Errorf("Kubernetes frontend %s", message)
	} else {
		message := "deleted the provisioned volume for the lost PVC."
		p.updatePVCWithEvent(claim, v1.EventTypeNormal, "VolumeDelete", message)
		Logc(ctx).WithFields(log.Fields{
			"PVC":    claim.Name,
			"volume": pv.Name,
		}).Infof("Kubernetes frontend %s", message)
	}
	return
}

// processUpdatedVolume processes updated Trident-created PVs.
func (p *Plugin) processUpdatedVolume(ctx context.Context, pv *v1.PersistentVolume) {

	switch pv.Status.Phase {
	case v1.VolumePending:
		return
	case v1.VolumeAvailable:
		return
	case v1.VolumeBound:
		// Update the reclaim policy to match the StorageClass for an imported volume if needed.
		if _, ok := pv.ObjectMeta.Annotations[AnnNotManaged]; ok {
			if pv.Spec.PersistentVolumeReclaimPolicy == v1.PersistentVolumeReclaimRetain {
				// If the imported volume's reclaim policy is retain then this check will be made every time we receive an PV udpate event.
				p.mutex.Lock()
				class := p.storageClassCache[pv.Spec.StorageClassName]
				classReclaimPolicy := *class.PersistentVolumeReclaimPolicy
				p.mutex.Unlock()
				if classReclaimPolicy != v1.PersistentVolumeReclaimRetain {
					pv.Spec.PersistentVolumeReclaimPolicy = classReclaimPolicy
					_, err := p.kubeClient.CoreV1().PersistentVolumes().Update(ctx, pv, updateOpts)
					if err != nil {
						message := fmt.Sprintf("failed to update PersistentVolumeReclaimPolicy for PV %s:%s "+
							"Will eventually retry.", pv.Name, err.Error())
						p.updateVolumePhaseWithEvent(ctx, pv, v1.VolumeBound, v1.EventTypeWarning,
							"FailedVolumeUpdate", message)
						Logc(ctx).Errorf("Kubernetes frontend %s", message)
						return
					}
					message := "updated the PersistentVolumeReclaimPolicy from Retain to the " +
						"policy set in the storage class. The imported volume's PVC and PV are now bound."
					Logc(ctx).WithFields(log.Fields{
						"PV":                            pv.Name,
						"PersistentVolumeReclaimPolicy": classReclaimPolicy,
						"StorageClassName":              pv.Spec.StorageClassName,
					}).Infof("Kubernetes frontend %s", message)
				}
			}
		}
		return
	case v1.VolumeReleased, v1.VolumeFailed:
		if pv.Spec.PersistentVolumeReclaimPolicy != v1.PersistentVolumeReclaimDelete {
			return
		}
		err := p.orchestrator.DeleteVolume(ctx, pv.Name)
		if err != nil && !utils.IsNotFoundError(err) {
			// Updating the PV's phase to "VolumeFailed", so that a storage admin can take action.
			message := fmt.Sprintf("failed to delete the volume for PV %s: %s. Will eventually retry, "+
				"but volume and PV may need to be manually deleted.", pv.Name, err.Error())
			p.updateVolumePhaseWithEvent(ctx, pv, v1.VolumeFailed, v1.EventTypeWarning, "FailedVolumeDelete", message)
			Logc(ctx).Errorf("Kubernetes frontend %s", message)
			// PV needs to be manually deleted by the admin after removing the volume.
			return
		}
		err = p.kubeClient.CoreV1().PersistentVolumes().Delete(ctx, pv.Name, deleteOpts)
		if err != nil {
			if !strings.HasSuffix(err.Error(), "not found") {
				// PVs provisioned by external provisioners seem to end up in
				// the failed state as Kubernetes doesn't recognize them.
				Logc(ctx).Errorf("Kubernetes frontend failed to delete the PV: %s", err.Error())
			}
			return
		}
		Logc(ctx).WithFields(log.Fields{
			"PV":     pv.Name,
			"volume": pv.Name,
		}).Info("Kubernetes frontend deleted the provisioned volume and PV.")
	default:
		Logc(ctx).WithFields(log.Fields{
			"PV":       pv.Name,
			"PV_phase": pv.Status.Phase,
		}).Error("Kubernetes frontend doesn't recognize the volume phase.")
	}
}

// updateVolumePhaseWithEvent saves new volume phase to API server and emits
// given event on the volume. It saves the phase and emits the event only when
// the phase has actually changed from the version saved in API server.
// (Based on pkg/controller/volume/persistentvolume/pv_controller.go)
func (p *Plugin) updateVolumePhaseWithEvent(
	ctx context.Context, pv *v1.PersistentVolume, phase v1.PersistentVolumePhase, eventtype, reason, message string,
) (*v1.PersistentVolume, error) {

	if pv.Status.Phase == phase {
		// Nothing to do.
		return pv, nil
	}
	newVol, err := p.updateVolumePhase(ctx, pv, phase, message)
	if err != nil {
		return nil, err
	}

	p.eventRecorder.Event(newVol, eventtype, reason, message)

	return newVol, nil
}

// updateVolumePhase saves new volume phase to API server.
// (Based on pkg/controller/volume/persistentvolume/pv_controller.go)
func (p *Plugin) updateVolumePhase(
	ctx context.Context, pv *v1.PersistentVolume, phase v1.PersistentVolumePhase, message string,
) (*v1.PersistentVolume, error) {

	if pv.Status.Phase == phase {
		// Nothing to do.
		return pv, nil
	}

	volumeClone := pv.DeepCopy()

	volumeClone.Status.Phase = phase
	volumeClone.Status.Message = message

	return p.kubeClient.CoreV1().PersistentVolumes().UpdateStatus(ctx, volumeClone, updateOpts)
}

// updatePVCWithEvent emits given event on the PVC.
// (Based on pkg/controller/volume/persistentvolume/pv_controller.go)
func (p *Plugin) updatePVCWithEvent(
	claim *v1.PersistentVolumeClaim, eventtype, reason, message string,
) (*v1.PersistentVolumeClaim, error) {
	p.eventRecorder.Event(claim, eventtype, reason, message)
	return claim, nil
}

// updatePVWithEvent emits given event on the PV.
// (Based on pkg/controller/volume/persistentvolume/pv_controller.go)
func (p *Plugin) updatePVWithEvent(
	pv *v1.PersistentVolume, eventtype, reason, message string,
) (*v1.PersistentVolume, error) {
	p.eventRecorder.Event(pv, eventtype, reason, message)
	return pv, nil
}

func convertStorageClassV1BetaToV1(class *k8sstoragev1beta.StorageClass) *k8sstoragev1.StorageClass {
	// For now we just copy the fields used by Trident.
	v1Class := &k8sstoragev1.StorageClass{
		Provisioner: class.Provisioner,
		Parameters:  class.Parameters,
	}
	v1Class.Name = class.Name
	return v1Class
}

func (p *Plugin) addClass(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S)

	class, ok := obj.(*k8sstoragev1beta.StorageClass)
	if ok {
		p.processClass(ctx, convertStorageClassV1BetaToV1(class), "add")
		return
	}
	classV1, ok := obj.(*k8sstoragev1.StorageClass)
	if !ok {
		Logc(ctx).Errorf("Kubernetes frontend expected storage.k8s.io/v1beta1 "+
			"or storage.k8s.io/v1 storage class; handler got %v", obj)
		return
	}
	p.processClass(ctx, classV1, "add")
}

func (p *Plugin) updateClass(oldObj, newObj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S)

	class, ok := newObj.(*k8sstoragev1beta.StorageClass)
	if ok {
		p.processClass(ctx, convertStorageClassV1BetaToV1(class), "update")
		return
	}
	classV1, ok := newObj.(*k8sstoragev1.StorageClass)
	if !ok {
		Logc(ctx).Errorf("Kubernetes frontend expected storage.k8s.io/v1beta1 "+
			"or storage.k8s.io/v1 storage class; handler got %v", newObj)
		return
	}
	p.processClass(ctx, classV1, "update")
}

func (p *Plugin) deleteClass(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S)

	class, ok := obj.(*k8sstoragev1beta.StorageClass)
	if ok {
		p.processClass(ctx, convertStorageClassV1BetaToV1(class), "delete")
		return
	}
	classV1, ok := obj.(*k8sstoragev1.StorageClass)
	if !ok {
		Logc(ctx).Errorf("Kubernetes frontend expected storage.k8s.io/v1beta1 "+
			"or storage.k8s.io/v1 storage class; handler got %v", obj)
		return
	}
	p.processClass(ctx, classV1, "delete")
}

func (p *Plugin) processClass(ctx context.Context, class *k8sstoragev1.StorageClass, eventType string) {

	Logc(ctx).WithFields(log.Fields{
		"storageClass":             class.Name,
		"storageClass_provisioner": class.Provisioner,
		"storageClass_parameters":  class.Parameters,
		"storageClass_eventType":   eventType,
	}).Debug("Kubernetes frontend got notified of a storage class.")

	if class.Provisioner != AnnOrchestrator {
		return
	}
	switch eventType {
	case "add":
		p.processAddedClass(ctx, class)
	case "delete":
		p.processDeletedClass(ctx, class)
	case "update":
		// Make sure Trident has a record of this storage class.
		if storageClass, _ := p.orchestrator.GetStorageClass(ctx, class.Name); storageClass == nil {
			Logc(ctx).WithFields(log.Fields{
				"storageClass": class.Name,
			}).Warn("Kubernetes frontend has no record of the updated " +
				"storage class; instead it will try to create it.")
			p.processAddedClass(ctx, class)
		} else {
			p.processUpdatedClass(ctx, class)
		}
	default:
		Logc(ctx).WithFields(log.Fields{
			"storageClass": class.Name,
			"event":        eventType,
		}).Error("Kubernetes frontend didn't recognize the notification event corresponding to the storage class!")
	}
}

func (p *Plugin) processAddedClass(ctx context.Context, class *k8sstoragev1.StorageClass) {

	scConfig := new(storageclass.Config)
	scConfig.Name = class.Name
	scConfig.Attributes = make(map[string]storageattribute.Request)
	k8sStorageClassParams := make(map[string]string)

	// Populate storage class config attributes and backend storage pools
	for k, v := range class.Parameters {
		switch k {
		case K8sFsType:
			// Process Kubernetes-defined storage class parameters
			k8sStorageClassParams[k] = v

		case storageattribute.RequiredStorage, storageattribute.AdditionalStoragePools:
			// format:  additionalStoragePools: "backend1:pool1,pool2;backend2:pool1"
			additionalPools, err := storageattribute.CreateBackendStoragePoolsMapFromEncodedString(v)
			if err != nil {
				Logc(ctx).WithFields(log.Fields{
					"storageClass":             class.Name,
					"storageClass_provisioner": class.Provisioner,
					"storageClass_parameters":  class.Parameters,
					"error":                    err,
				}).Errorf("Kubernetes frontend couldn't process the storage class parameter %s", k)
			}
			scConfig.AdditionalPools = additionalPools

		case storageattribute.ExcludeStoragePools:
			// format:  excludeStoragePools: "backend1:pool1,pool2;backend2:pool1"
			excludeStoragePools, err := storageattribute.CreateBackendStoragePoolsMapFromEncodedString(v)
			if err != nil {
				Logc(ctx).WithFields(log.Fields{
					"storageClass":             class.Name,
					"storageClass_provisioner": class.Provisioner,
					"storageClass_parameters":  class.Parameters,
					"error":                    err,
				}).Errorf("Kubernetes frontend couldn't process the storage class parameter %s", k)
			}
			scConfig.ExcludePools = excludeStoragePools

		case storageattribute.StoragePools:
			// format:  storagePools: "backend1:pool1,pool2;backend2:pool1"
			pools, err := storageattribute.CreateBackendStoragePoolsMapFromEncodedString(v)
			if err != nil {
				Logc(ctx).WithFields(log.Fields{
					"storageClass":             class.Name,
					"storageClass_provisioner": class.Provisioner,
					"storageClass_parameters":  class.Parameters,
					"error":                    err,
				}).Errorf("Kubernetes frontend couldn't process the storage class parameter %s", k)
			}
			scConfig.Pools = pools

		default:
			// format:  attribute: "value"
			req, err := storageattribute.CreateAttributeRequestFromAttributeValue(k, v)
			if err != nil {
				Logc(ctx).WithFields(log.Fields{
					"storageClass":             class.Name,
					"storageClass_provisioner": class.Provisioner,
					"storageClass_parameters":  class.Parameters,
					"error":                    err,
				}).Errorf("Kubernetes frontend couldn't process the storage class attribute %s", k)
				return
			}
			scConfig.Attributes[k] = req
		}
	}

	// Update Kubernetes-defined storage class parameters maintained by the
	// frontend. Note that these parameters are only processed by the frontend
	// and not by Trident core.
	p.mutex.Lock()
	storageClassSummary := &StorageClassSummary{
		Parameters:                    k8sStorageClassParams,
		MountOptions:                  class.MountOptions,
		PersistentVolumeReclaimPolicy: class.ReclaimPolicy,
		AllowVolumeExpansion:          class.AllowVolumeExpansion,
	}
	p.storageClassCache[class.Name] = storageClassSummary
	p.mutex.Unlock()

	// Add the storage class
	sc, err := p.orchestrator.AddStorageClass(ctx, scConfig)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"storageClass":             class.Name,
			"storageClass_provisioner": class.Provisioner,
			"storageClass_parameters":  class.Parameters,
		}).Error("Kubernetes frontend couldn't add the storage class: ", err)
		return
	}
	if sc != nil {
		// Check if it's a default storage class
		if getAnnotation(class.Annotations, AnnDefaultStorageClass) == "true" {
			p.mutex.Lock()
			p.defaultStorageClasses[class.Name] = true
			if len(p.defaultStorageClasses) > 1 {
				Logc(ctx).WithFields(log.Fields{
					"storageClass":           class.Name,
					"default_storageClasses": p.getDefaultStorageClasses(),
				}).Warn("Kubernetes frontend already manages more than one default storage class!")
			}
			p.mutex.Unlock()
		}

		Logc(ctx).WithFields(log.Fields{
			"storageClass":             class.Name,
			"storageClass_provisioner": class.Provisioner,
			"storageClass_parameters":  class.Parameters,
			"default_storageClasses":   p.getDefaultStorageClasses(),
		}).Info("Kubernetes frontend added the storage class.")
	}
}

func (p *Plugin) processDeletedClass(ctx context.Context, class *k8sstoragev1.StorageClass) {

	// Check if we're deleting the default storage class.
	if getAnnotation(class.Annotations, AnnDefaultStorageClass) == "true" {
		p.mutex.Lock()
		if p.defaultStorageClasses[class.Name] {
			delete(p.defaultStorageClasses, class.Name)
			Logc(ctx).WithFields(log.Fields{
				"storageClass": class.Name,
			}).Info("Kubernetes frontend will be deleting a default storage class.")
		}
		p.mutex.Unlock()
	}

	// Delete the storage class.
	err := p.orchestrator.DeleteStorageClass(ctx, class.Name)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"storageClass": class.Name,
		}).Error("Kubernetes frontend couldn't delete the storage class: ", err)
	} else {
		p.mutex.Lock()
		delete(p.storageClassCache, class.Name)
		Logc(ctx).WithFields(log.Fields{
			"storageClass": class.Name,
		}).Info("Kubernetes frontend deleted the storage class.")
		p.mutex.Unlock()
	}
}

func (p *Plugin) processUpdatedClass(ctx context.Context, class *k8sstoragev1.StorageClass) {

	// Here we only check for updates associated with the default storage class.
	p.mutex.Lock()
	defer func() {
		p.mutex.Unlock()
	}()

	// Updating the storage class cache.
	storageClassSummary, found := p.storageClassCache[class.Name]
	if !found || storageClassSummary == nil {
		// Should never reach here.
	} else {
		// Updating the mutable fields in StorageClassSummary.
		// See kubernetes/kubernetes/pkg/apis/storage/validation.
		storageClassSummary.AllowVolumeExpansion = class.AllowVolumeExpansion
		storageClassSummary.MountOptions = class.MountOptions
	}

	// Handling updates related to the default storage class.
	if p.defaultStorageClasses[class.Name] {
		// It's an update to a default storage class.
		// Check to see if it's still a default storage class.
		if getAnnotation(class.Annotations, AnnDefaultStorageClass) != "true" {
			delete(p.defaultStorageClasses, class.Name)
		}
	} else {
		// It's an update to a non-default storage class.
		if getAnnotation(class.Annotations, AnnDefaultStorageClass) == "true" {
			// The update defines a new default storage class.
			p.defaultStorageClasses[class.Name] = true
			Logc(ctx).WithFields(log.Fields{
				"storageClass":           class.Name,
				"default_storageClasses": p.getDefaultStorageClasses(),
			}).Info("Kubernetes frontend added a new default storage class.")
		}
	}
}

func (p *Plugin) getDefaultStorageClasses() string {
	classes := make([]string, 0)
	for class := range p.defaultStorageClasses {
		classes = append(classes, class)
	}
	return strings.Join(classes, ",")
}

// GetPersistentVolumeClaimClass returns StorageClassName. If no storage class was
// requested, it returns "".
func GetPersistentVolumeClaimClass(claim *v1.PersistentVolumeClaim) string {
	// Use beta annotation first
	if class, found := claim.Annotations[AnnClass]; found {
		return class
	}

	if claim.Spec.StorageClassName != nil {
		return *claim.Spec.StorageClassName
	}

	return ""
}

func (p *Plugin) updateClaimResize(oldObj, newObj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S)

	oldClaim, ok := oldObj.(*v1.PersistentVolumeClaim)
	if !ok {
		Logc(ctx).Errorf("Kubernetes frontend expected PVC; handler got %v", oldObj)
		return
	}
	newClaim, ok := newObj.(*v1.PersistentVolumeClaim)
	if !ok {
		Logc(ctx).Errorf("Kubernetes frontend expected PVC; handler got %v", oldObj)
		return
	}

	// Filtering PVCs for volume resize.
	// We intentionally don't use feature gates to see whether volume expansion
	// is enabled or not. We also don't check for k8s version 1.12 or later
	// as Trident can support volume resize with earlier versions of k8s if
	// admission controller doesn't restrict volume resize through
	// --disable-admission-plugins=PersistentVolumeClaimResize.

	// We only allow resizing bound claims.
	if newClaim.Status.Phase != v1.ClaimBound {
		return
	}

	// Verify the storage class is managed by Trident.
	storageClass := GetPersistentVolumeClaimClass(newClaim)
	if storageClass == "" ||
		getClaimProvisioner(newClaim) != AnnOrchestrator {
		return
	}

	// We only allow expanding volumes.
	oldPVCSize := oldClaim.Spec.Resources.Requests[v1.ResourceStorage]
	newPVCSize := newClaim.Spec.Resources.Requests[v1.ResourceStorage]
	currentSize := newClaim.Status.Capacity[v1.ResourceStorage]
	if newPVCSize.Cmp(oldPVCSize) < 0 ||
		(newPVCSize.Cmp(oldPVCSize) == 0 && currentSize.Cmp(newPVCSize) > 0) {
		message := "shrinking a PV isn't allowed!"
		p.updatePVCWithEvent(newClaim, v1.EventTypeWarning, "ResizeFailed", message)
		Logc(ctx).WithFields(log.Fields{
			"PVC": newClaim.Name,
		}).Debug("Kubernetes frontend doesn't allow shrinking a PV!")
		return
	} else if newPVCSize.Cmp(oldPVCSize) == 0 && currentSize.Cmp(newPVCSize) == 0 {
		// Everything has the right size. No work to be done!
		return
	}

	// Verify the storage class allows volume expansion.
	if storageClassSummary, found := p.storageClassCache[storageClass]; found {
		if storageClassSummary.AllowVolumeExpansion == nil ||
			!*storageClassSummary.AllowVolumeExpansion {
			message := "can't resize a PV whose storage class doesn't allow volume expansion!"
			p.updatePVCWithEvent(newClaim, v1.EventTypeWarning, "ResizeFailed", message)
			Logc(ctx).WithFields(log.Fields{
				"PVC":          newClaim.Name,
				"storageClass": storageClass,
			}).Debugf("Kubernetes frontend %s", message)
			return
		}
	} else {
		message := fmt.Sprintf("no knowledge of storage class %s to conduct volume resize!", storageClass)
		p.updatePVCWithEvent(newClaim, v1.EventTypeWarning, "ResizeFailed", message)
		Logc(ctx).WithFields(log.Fields{
			"PVC":          newClaim.Name,
			"storageClass": storageClass,
		}).Debugf("Kubernetes frontend has %s", message)
		return
	}

	// If we get this far, we potentially have a valid resize operation.
	Logc(ctx).WithFields(log.Fields{
		"PVC":          newClaim.Name,
		"PVC_old_size": currentSize.String(),
		"PVC_new_size": newPVCSize.String(),
	}).Debug("Kubernetes frontend detected a PVC suited for volume resize.")

	// We intentionally don't set the PVC condition to "PersistentVolumeClaimResizing"
	// as NFS resize happens instantaneously. Note that updating the condition
	// should only happen when the condition isn't set. Otherwise, we may have
	// infinite calls to updateClaimResize. Currently, we don't keep track of
	// outstanding resize operations. This may need to change when resize takes
	// non-negligible amount of time.

	// We only allow resizing NFS PVs as it doesn't require a host-side
	// footprint to resize the file system.
	pv, err := p.GetPVForPVC(ctx, newClaim)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"PVC":   newClaim.Name,
			"PV":    newClaim.Spec.VolumeName,
			"error": err,
		}).Error("Kubernetes frontend couldn't retrieve the matching PV for the PVC!")
		return
	}
	if pv == nil {
		return
	} else if pv.Spec.NFS == nil {
		message := "can't resize a non-NFS PV!"
		p.updatePVCWithEvent(newClaim, v1.EventTypeWarning, "ResizeFailed", message)
		Logc(ctx).WithFields(log.Fields{"PVC": newClaim.Name}).Debugf("Kubernetes frontend %s", message)
		return
	}

	// Resize the volume and PV
	if _, err = p.resizeVolumeAndPV(ctx, pv, newPVCSize); err != nil {
		message := fmt.Sprintf("failed in resizing the volume or PV: %v", err)
		p.updatePVCWithEvent(newClaim, v1.EventTypeWarning, "ResizeFailed", message)
		Logc(ctx).WithFields(log.Fields{"PVC": newClaim.Name}).Errorf("Kubernetes frontend %v", message)
		return
	}

	// Update the PVC
	updatedClaim, err := p.updatePVCSize(ctx, newClaim, newPVCSize)
	if err != nil {
		message := fmt.Sprintf("failed in updating the PVC size: %v.",
			err)
		if updatedClaim == nil {
			p.updatePVCWithEvent(newClaim, v1.EventTypeWarning, "ResizeFailed", message)
		} else {
			p.updatePVCWithEvent(updatedClaim, v1.EventTypeWarning, "ResizeFailed", message)
		}
		Logc(ctx).WithFields(log.Fields{"PVC": newClaim.Name}).Errorf("Kubernetes frontend %v", message)
		return
	}
	message := "resized the PV and volume."
	p.updatePVCWithEvent(updatedClaim, v1.EventTypeNormal, "ResizeSuccess", message)
}

// GetPVForPVC returns the PV for a bound PVC.
func (p *Plugin) GetPVForPVC(ctx context.Context, claim *v1.PersistentVolumeClaim) (*v1.PersistentVolume, error) {
	if claim.Status.Phase != v1.ClaimBound {
		return nil, nil
	}

	return p.kubeClient.CoreV1().PersistentVolumes().Get(ctx, claim.Spec.VolumeName, getOpts)
}

// GetPVCForPV returns the PVC for a PV.
func (p *Plugin) GetPVCForPV(ctx context.Context, pv *v1.PersistentVolume) (*v1.PersistentVolumeClaim, error) {
	if pv.Spec.ClaimRef == nil {
		return nil, nil
	}

	return p.kubeClient.CoreV1().PersistentVolumeClaims(
		pv.Spec.ClaimRef.Namespace).Get(ctx, pv.Spec.ClaimRef.Name, getOpts)
}

// resizeVolumeAndPV resizes the volume on the storage backend and updates the PV size.
func (p *Plugin) resizeVolumeAndPV(
	ctx context.Context, pv *v1.PersistentVolume, newSize resource.Quantity,
) (*v1.PersistentVolume, error) {

	pvSize := pv.Spec.Capacity[v1.ResourceStorage]
	if pvSize.Cmp(newSize) < 0 {
		// Calling the orchestrator to resize the volume on the storage backend.
		if err := p.orchestrator.ResizeVolume(ctx, pv.Name, fmt.Sprintf("%d", newSize.Value())); err != nil {
			return pv, err
		}
	} else if pvSize.Cmp(newSize) == 0 {
		return pv, nil
	} else {
		return pv, fmt.Errorf("cannot shrink PV %q!", pv.Name)
	}

	// Update the PV
	pvClone := pv.DeepCopy()
	pvClone.Spec.Capacity[v1.ResourceStorage] = newSize
	pvUpdated, err := PatchPV(ctx, p.kubeClient, pv, pvClone)
	if err != nil {
		return pv, err
	}
	updatedSize := pvUpdated.Spec.Capacity[v1.ResourceStorage]
	if updatedSize.Cmp(newSize) != 0 {
		return pvUpdated, err
	}

	Logc(ctx).WithFields(log.Fields{
		"PV":          pv.Name,
		"PV_old_size": pvSize.String(),
		"PV_new_size": updatedSize.String(),
	}).Info("Kubernetes frontend resized the PV.")

	return pvUpdated, nil
}

// updatePVCSize updates the PVC size.
func (p *Plugin) updatePVCSize(
	ctx context.Context, pvc *v1.PersistentVolumeClaim, newSize resource.Quantity,
) (*v1.PersistentVolumeClaim, error) {

	pvcClone := pvc.DeepCopy()
	pvcClone.Status.Capacity[v1.ResourceStorage] = newSize
	pvcUpdated, err := PatchPVCStatus(ctx, p.kubeClient, pvc, pvcClone)
	if err != nil {
		return nil, err
	}
	updatedSize := pvcUpdated.Status.Capacity[v1.ResourceStorage]
	if updatedSize.Cmp(newSize) != 0 {
		return pvcUpdated, err
	}

	oldSize := pvc.Status.Capacity[v1.ResourceStorage]
	Logc(ctx).WithFields(log.Fields{
		"PVC":          pvc.Name,
		"PVC_old_size": oldSize.String(),
		"PVC_new_size": updatedSize.String(),
	}).Info("Kubernetes frontend updated the PVC after resize.")

	return pvcUpdated, nil
}

// This func was added to validate the provisioner for volume import. Once processClaim
// also checks the storageClass for the provisioner the code can be refactored with getClaimProvisioner
func (p *Plugin) getClaimOrClassProvisioner(ctx context.Context, claim *v1.PersistentVolumeClaim) string {

	provisioner := getClaimProvisioner(claim)
	if provisioner != "" {
		return provisioner
	}
	storageClassName := *claim.Spec.StorageClassName
	_, err := p.orchestrator.GetStorageClass(ctx, storageClassName)
	if err != nil {
		Logc(ctx).WithField("storageClass", storageClassName).Errorf("error occurred getting storageclass: %v", err)
		return ""
	} else {
		// We have the storageClass so Trident is the provisioner
		return AnnOrchestrator
	}
}
