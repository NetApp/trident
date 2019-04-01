/*
 * Copyright 2018 NetApp, Inc. All Rights Reserved.
 */

package kubernetes

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"
	"time"

	k8ssnapshotv1alpha1 "github.com/kubernetes-csi/external-snapshotter/pkg/apis/volumesnapshot/v1alpha1"
	k8ssnapshotclient "github.com/kubernetes-csi/external-snapshotter/pkg/client/clientset/versioned"
	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
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
	"github.com/netapp/trident/k8s_client"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_class"
	drivers "github.com/netapp/trident/storage_drivers"
)

// StorageClassSummary captures relevant fields in the storage class that are needed during PV creation or PV resize.
type StorageClassSummary struct {
	Parameters                    map[string]string
	MountOptions                  []string
	PersistentVolumeReclaimPolicy *v1.PersistentVolumeReclaimPolicy
	AllowVolumeExpansion          *bool
}

type Plugin struct {
	orchestrator                     core.Orchestrator
	kubeClient                       kubernetes.Interface
	kubeSnapClient                   k8ssnapshotclient.Interface
	getNamespacedKubeClient          func(*rest.Config, string) (k8sclient.Interface, error)
	kubeConfig                       rest.Config
	eventRecorder                    record.EventRecorder
	claimController                  cache.Controller
	claimControllerStopChan          chan struct{}
	claimSource                      cache.ListerWatcher
	volumeController                 cache.Controller
	volumeControllerStopChan         chan struct{}
	volumeSource                     cache.ListerWatcher
	classController                  cache.Controller
	classControllerStopChan          chan struct{}
	classSource                      cache.ListerWatcher
	resizeController                 cache.Controller
	resizeControllerStopChan         chan struct{}
	resizeSource                     cache.ListerWatcher
	volumeSnapshotController         cache.Controller
	volumeSnapshotControllerStopChan chan struct{}
	volumeSnapshotSource             cache.ListerWatcher
	mutex                            *sync.Mutex
	pendingClaimMatchMap             map[string]*v1.PersistentVolume
	kubernetesVersion                *k8sversion.Info
	defaultStorageClasses            map[string]bool
	storageClassCache                map[string]*StorageClassSummary
	tridentNamespace                 string
}

func NewPlugin(o core.Orchestrator, apiServerIP, kubeConfigPath string) (*Plugin, error) {
	kubeConfig, err := clientcmd.BuildConfigFromFlags(apiServerIP, kubeConfigPath)
	if err != nil {
		return nil, err
	}

	// Create the CLI-based Kubernetes client
	client, err := clik8sclient.NewKubectlClient("")
	if err != nil {
		return nil, fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	// when running in binary mode, we use the current namespace as determined by the CLI client
	return newKubernetesPlugin(o, kubeConfig, client.Namespace())
}

func NewPluginInCluster(o core.Orchestrator) (*Plugin, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	// when running in a pod, we use the Trident pod's namespace
	bytes, err := ioutil.ReadFile(config.TridentNamespaceFile)
	if err != nil {
		log.WithFields(log.Fields{
			"error":         err,
			"namespaceFile": config.TridentNamespaceFile,
		}).Fatal("Kubernetes frontend failed to obtain Trident's namespace!")
	}
	tridentNamespace := string(bytes)

	return newKubernetesPlugin(o, kubeConfig, tridentNamespace)
}

func newKubernetesPlugin(
	orchestrator core.Orchestrator, kubeConfig *rest.Config, tridentNamespace string,
) (*Plugin, error) {

	log.WithFields(log.Fields{
		"namespace": tridentNamespace,
	}).Info("Initializing Kubernetes frontend.")

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	kubeSnapClient, err := k8ssnapshotclient.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	ret := &Plugin{
		orchestrator:                     orchestrator,
		kubeClient:                       kubeClient,
		kubeSnapClient:                   kubeSnapClient,
		getNamespacedKubeClient:          k8sclient.NewKubeClient,
		kubeConfig:                       *kubeConfig,
		claimControllerStopChan:          make(chan struct{}),
		volumeControllerStopChan:         make(chan struct{}),
		classControllerStopChan:          make(chan struct{}),
		resizeControllerStopChan:         make(chan struct{}),
		volumeSnapshotControllerStopChan: make(chan struct{}),
		mutex:                 &sync.Mutex{},
		pendingClaimMatchMap:  make(map[string]*v1.PersistentVolume),
		defaultStorageClasses: make(map[string]bool, 1),
		storageClassCache:     make(map[string]*StorageClassSummary),
		tridentNamespace:      tridentNamespace,
	}

	ret.kubernetesVersion, err = kubeClient.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("Kubernetes frontend couldn't retrieve API server's version: %v", err)
	}
	log.WithFields(log.Fields{
		"version":    ret.kubernetesVersion.Major + "." + ret.kubernetesVersion.Minor,
		"gitVersion": ret.kubernetesVersion.GitVersion,
	}).Info("Kubernetes frontend determined the container orchestrator  version.")
	_, err = ValidateKubeVersion(ret.kubernetesVersion)
	if err != nil {
		if IsPanicKubeVersion(err) {
			return nil, fmt.Errorf("Kubernetes frontend couldn't validate Kubernetes version: %v", err)
		}
		if IsUnsupportedKubeVersion(err) {
			log.Warnf("%s v%s may not support container orchestrator version %s.%s (%s)! "+
				"Supported versions for Kubernetes are %s-%s.", config.OrchestratorName,
				config.OrchestratorVersion, ret.kubernetesVersion.Major, ret.kubernetesVersion.Minor,
				ret.kubernetesVersion.GitVersion, config.KubernetesVersionMin, config.KubernetesVersionMax)
			log.Warnf("Kubernetes frontend proceeds as if you are running Kubernetes %s!",
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
			return kubeClient.CoreV1().PersistentVolumeClaims(v1.NamespaceAll).List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.CoreV1().PersistentVolumeClaims(v1.NamespaceAll).Watch(options)
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
			return kubeClient.CoreV1().PersistentVolumes().List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.CoreV1().PersistentVolumes().Watch(options)
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
			return kubeClient.StorageV1().StorageClasses().List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.StorageV1().StorageClasses().Watch(options)
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
			return kubeClient.CoreV1().PersistentVolumeClaims(v1.NamespaceAll).List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.CoreV1().PersistentVolumeClaims(v1.NamespaceAll).Watch(options)
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

	// Setting up a watch for VolumeSnapshot CR
	ret.volumeSnapshotSource = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return kubeSnapClient.VolumesnapshotV1alpha1().VolumeSnapshots(v1.NamespaceAll).List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeSnapClient.VolumesnapshotV1alpha1().VolumeSnapshots(v1.NamespaceAll).Watch(options)
		},
	}
	_, ret.volumeSnapshotController = cache.NewInformer(
		ret.volumeSnapshotSource,
		&k8ssnapshotv1alpha1.VolumeSnapshot{},
		KubernetesSyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc: ret.addVolumeSnapshot,
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
	go p.volumeSnapshotController.Run(p.volumeSnapshotControllerStopChan)
	return nil
}

func (p *Plugin) Deactivate() error {
	log.Info("Deactivating Kubernetes frontend.")
	close(p.claimControllerStopChan)
	close(p.volumeControllerStopChan)
	close(p.classControllerStopChan)
	close(p.resizeControllerStopChan)
	close(p.volumeSnapshotControllerStopChan)
	return nil
}

func (p *Plugin) GetName() string {
	return string(config.ContextKubernetes)
}

func (p *Plugin) Version() string {
	return p.kubernetesVersion.GitVersion
}

func (p *Plugin) addClaim(obj interface{}) {
	claim, ok := obj.(*v1.PersistentVolumeClaim)
	if !ok {
		log.Errorf("Kubernetes frontend expected PVC; handler got %v", obj)
		return
	}
	p.processClaim(claim, "add")
}

func (p *Plugin) updateClaim(oldObj, newObj interface{}) {
	claim, ok := newObj.(*v1.PersistentVolumeClaim)
	if !ok {
		log.Errorf("Kubernetes frontend expected PVC; handler got %v", newObj)
		return
	}
	p.processClaim(claim, "update")
}

func (p *Plugin) deleteClaim(obj interface{}) {
	claim, ok := obj.(*v1.PersistentVolumeClaim)
	if !ok {
		log.Errorf("Kubernetes frontend expected PVC; handler got %v", obj)
		return
	}
	p.processClaim(claim, "delete")
}

func (p *Plugin) processClaim(claim *v1.PersistentVolumeClaim, eventType string) {
	// Validating the claim
	size, ok := claim.Spec.Resources.Requests[v1.ResourceStorage]
	if !ok {
		return
	}
	log.WithFields(log.Fields{
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

	// Filter unrelated claims
	provisioner := getClaimProvisioner(claim)
	if claim.Spec.StorageClassName != nil && *claim.Spec.StorageClassName == "" {
		log.WithFields(log.Fields{
			"PVC": claim.Name,
		}).Debug("Kubernetes frontend ignored this PVC as an empty string " +
			"was specified for the storage class!")
		return
	}

	// Reject PVCs whose provisioner was not set to Trident.
	// However, don't reject a PVC whose provisioner wasn't set
	// just because no default storage class existed at the time
	// of the PVC creation.
	if provisioner != AnnOrchestrator &&
		GetPersistentVolumeClaimClass(claim) != "" {
		log.WithFields(log.Fields{
			"PVC":             claim.Name,
			"PVC_provisioner": provisioner,
		}).Debugf("Kubernetes frontend ignored this PVC as it's not "+
			"tagged with %s as the storage provisioner!", AnnOrchestrator)
		return
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
			log.WithFields(log.Fields{
				"PVC": claim.Name,
				"storageClass_default": defaultStorageClassName,
			}).Info("Kubernetes frontend will use the default storage class for this PVC.")
		} else if len(p.defaultStorageClasses) > 1 {
			log.WithFields(log.Fields{
				"PVC": claim.Name,
				"default_storageClasses": p.getDefaultStorageClasses(),
			}).Warn("Kubernetes frontend ignored this PVC as more than " +
				"one default storage class has been configured!")
			p.mutex.Unlock()
			return
		} else {
			log.WithFields(log.Fields{
				"PVC": claim.Name,
			}).Debug("Kubernetes frontend ignored this PVC as no storage class " +
				"was specified and no default storage class was configured!")
			p.mutex.Unlock()
			return
		}
		p.mutex.Unlock()
	}

	// If storage class is still empty (after considering the default storage
	// class), ignore the PVC as Kubernetes doesn't allow a storage class with
	// empty string as the name.
	if GetPersistentVolumeClaimClass(claim) == "" {
		log.WithFields(log.Fields{
			"PVC": claim.Name,
		}).Debug("Kubernetes frontend ignored this PVC as no storage class was specified!")
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
		log.WithFields(log.Fields{
			"PVC":   claim.Name,
			"event": eventType,
		}).Error("Kubernetes frontend didn't recognize the notification event corresponding to the PVC!")
		return
	}

	// Treating add and update events similarly.
	// Making decisions based on a claim's phase, similar to k8s' persistent volume controller.
	switch claim.Status.Phase {
	case v1.ClaimBound:
		p.processBoundClaim(claim)
		return
	case v1.ClaimLost:
		p.processLostClaim(claim)
		return
	case v1.ClaimPending:
		// As of Kubernetes 1.6, selector and storage class are mutually exclusive.
		if claim.Spec.Selector != nil {
			message := "ignored PVCs with label selectors!"
			p.updatePVCWithEvent(claim, v1.EventTypeWarning, "IgnoredClaim", message)
			log.WithFields(log.Fields{
				"PVC": claim.Name,
			}).Debugf("Kubernetes frontend %s", message)
			return
		}
		p.processPendingClaim(claim)
	default:
		log.WithFields(log.Fields{
			"PVC":       claim.Name,
			"PVC_phase": claim.Status.Phase,
		}).Error("Kubernetes frontend doesn't recognize the claim phase.")
	}
}

// processBoundClaim validates whether a Trident-created PV got bound to the intended PVC.
func (p *Plugin) processBoundClaim(claim *v1.PersistentVolumeClaim) {
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
		err := p.deleteVolumeAndPV(pv)
		if err != nil {
			deleteClaim = false
			log.WithFields(log.Fields{
				"PVC":        claim.Name,
				"PVC_volume": boundVolumeName,
				"PV":         pv.Name,
				"PV_volume":  pv.Name,
			}).Warnf("Kubernetes frontend detected PVC isn't bound to the intended PV, but it failed to "+
				"delete the PV or the corresponding provisioned volume (will retry upon resync): %s", err.Error())
			return
		}
		log.WithFields(log.Fields{
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
func (p *Plugin) processPendingClaim(claim *v1.PersistentVolumeClaim) {
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
		if canPVMatchWithPVC(pv, claim) {
			p.mutex.Unlock()
			return
		}
		// Otherwise, we need to delete the old volume and allocate a new one
		if err := p.deleteVolumeAndPV(pv); err != nil {
			log.WithFields(log.Fields{
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
	pv, err := p.createVolumeAndPV(orchestratorClaimName, claim)
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
	log.WithFields(log.Fields{
		"PVC":       claim.Name,
		"PV":        orchestratorClaimName,
		"PV_volume": pv.Name,
	}).Infof("Kubernetes frontend %s", message)
}

func (p *Plugin) createVolumeAndPV(
	uniqueName string, claim *v1.PersistentVolumeClaim,
) (pv *v1.PersistentVolume, err error) {

	var (
		nfsSource          *v1.NFSVolumeSource
		iscsiSource        *v1.ISCSIPersistentVolumeSource
		vol                *storage.VolumeExternal
		storageClassParams map[string]string
	)

	defer func() {
		if vol != nil && err != nil {
			err1 := err
			// Delete the volume on the backend
			err = p.orchestrator.DeleteVolume(vol.Config.Name)
			if err != nil {
				err2 := "Kubernetes frontend couldn't delete the volume after failed creation: " + err.Error()
				log.WithFields(log.Fields{
					"volume":  vol.Config.Name,
					"backend": vol.Backend,
				}).Error(err2)
				err = fmt.Errorf("%s => %s", err1.Error(), err2)
			} else {
				err = err1
			}
		}
	}()

	size, _ := claim.Spec.Resources.Requests[v1.ResourceStorage]
	accessModes := claim.Spec.AccessModes
	annotations := claim.Annotations
	storageClass := GetPersistentVolumeClaimClass(claim)
	if storageClassSummary, found := p.storageClassCache[storageClass]; found {
		storageClassParams = storageClassSummary.Parameters
	}

	// TODO: A quick way to support v1 storage classes before changing unit tests
	if _, found := annotations[AnnClass]; !found {
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[AnnClass] = GetPersistentVolumeClaimClass(claim)
	}

	// Set the file system type based on the value in the storage class
	if _, found := annotations[AnnFileSystem]; !found && storageClassParams != nil {
		if fsType, found := storageClassParams[K8sFsType]; found {
			annotations[AnnFileSystem] = fsType
		}
	}

	k8sClient, err := p.getNamespacedKubeClient(&p.kubeConfig, claim.Namespace)
	if err != nil {
		log.WithFields(log.Fields{
			"namespace": claim.Namespace,
			"error":     err.Error(),
		}).Warn("Kubernetes frontend couldn't create a client to namespace!")
	}

	// Create the volume configuration object
	volConfig := getVolumeConfig(accessModes, uniqueName, size, annotations)
	if volConfig.CloneSourceVolume == "" {
		vol, err = p.orchestrator.AddVolume(volConfig)
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
			log.WithFields(log.Fields{
				"sourcePVC":     volConfig.CloneSourceVolume,
				"PVC":           claim.Name,
				"PVC_namespace": claim.Namespace,
			}).Debugf("Kubernetes frontend detected an invalid configuration "+
				"for cloning from a PVC: %v", err.Error())
			return
		}

		// 2) Validate that storage classes match for the two PVCs
		if GetPersistentVolumeClaimClass(pvc) != GetPersistentVolumeClaimClass(claim) {
			err = fmt.Errorf("cloning from a PVC requires matching storage classes")
			log.WithFields(log.Fields{
				"PVC":                    claim.Name,
				"PVC_storageClass":       GetPersistentVolumeClaimClass(claim),
				"sourcePVC":              volConfig.CloneSourceVolume,
				"sourcePVC_storageClass": GetPersistentVolumeClaimClass(pvc),
			}).Debugf("Kubernetes frontend detected an invalid configuration "+
				"for cloning from a PVC: %v", err.Error())
			return
		}

		// 3) Set the source PVC name as it's understood by Trident.
		volConfig.CloneSourceVolume = getUniqueClaimName(pvc)

		// 4) Clone the existing volume
		vol, err = p.orchestrator.CloneVolume(volConfig)
	}
	if err != nil {
		log.WithFields(log.Fields{
			"volume": uniqueName,
		}).Warnf("Kubernetes frontend couldn't provision a volume: %s "+
			"(will retry upon resync)", err.Error())
		return
	}

	claimRef := v1.ObjectReference{
		Namespace: claim.Namespace,
		Name:      claim.Name,
		UID:       claim.UID,
	}
	pv = &v1.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: uniqueName,
			Annotations: map[string]string{
				AnnClass:                  GetPersistentVolumeClaimClass(claim),
				AnnDynamicallyProvisioned: AnnOrchestrator,
			},
		},
		Spec: v1.PersistentVolumeSpec{
			AccessModes: accessModes,
			Capacity:    v1.ResourceList{v1.ResourceStorage: resource.MustParse(vol.Config.Size)},
			ClaimRef:    &claimRef,
			// Default policy is "Delete".
			PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
		},
	}

	kubeVersion, _ := ValidateKubeVersion(p.kubernetesVersion)

	pv.Spec.StorageClassName = GetPersistentVolumeClaimClass(claim)

	// Apply Storage Class mount options and reclaim policy
	pv.Spec.MountOptions = p.getPVMountOptions(p.storageClassCache[storageClass], vol)
	pv.Spec.PersistentVolumeReclaimPolicy = *p.storageClassCache[storageClass].PersistentVolumeReclaimPolicy

	// We create the CHAP secret in Trident's namespace
	k8sClientCHAP, err := p.getNamespacedKubeClient(&p.kubeConfig, p.tridentNamespace)
	if err != nil {
		log.WithFields(log.Fields{
			"namespace": p.tridentNamespace,
			"error":     err.Error(),
		}).Warn("Kubernetes frontend couldn't create a client to namespace!")
	}

	driverType, _ := p.orchestrator.GetDriverTypeForVolume(vol)
	switch driverType {

	case drivers.SolidfireSANStorageDriverName,
		drivers.OntapSANStorageDriverName,
		drivers.EseriesIscsiStorageDriverName:

		iscsiSource, err = CreateISCSIPersistentVolumeSource(k8sClientCHAP, kubeVersion, vol)
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
		drivers.AWSNFSStorageDriverName:

		nfsSource = CreateNFSVolumeSource(vol)
		pv.Spec.NFS = nfsSource

	case drivers.FakeStorageDriverName:

		if vol.Config.Protocol == config.File {
			nfsSource = CreateNFSVolumeSource(vol)
			pv.Spec.NFS = nfsSource
		} else if vol.Config.Protocol == config.Block {
			iscsiSource, err = CreateISCSIPersistentVolumeSource(k8sClientCHAP, kubeVersion, vol)
			if err != nil {
				return
			}
			pv.Spec.ISCSI = iscsiSource
		}

	default:

		// Unknown driver for the frontend plugin or for Kubernetes.
		// Provisioned volume should get deleted.
		volType, _ := p.orchestrator.GetVolumeType(vol)
		log.WithFields(log.Fields{
			"volume": vol.Config.Name,
			"type":   volType,
			"driver": driverType,
		}).Error("Kubernetes frontend doesn't recognize this type of volume; deleting the provisioned volume.")
		err = fmt.Errorf("unrecognized volume type by Kubernetes")
		return
	}
	pv, err = p.kubeClient.CoreV1().PersistentVolumes().Create(pv)
	return
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

func (p *Plugin) deleteVolumeAndPV(volume *v1.PersistentVolume) error {
	err := p.orchestrator.DeleteVolume(volume.GetName())
	if err != nil && !core.IsNotFoundError(err) {
		message := fmt.Sprintf("failed to delete the volume for PV %s: %s. Volume and PV may "+
			"need to be manually deleted.", volume.GetName(), err.Error())
		p.updateVolumePhaseWithEvent(volume, v1.VolumeFailed, v1.EventTypeWarning, "FailedVolumeDelete", message)
		return fmt.Errorf("Kubernetes frontend %s", message)
	}
	err = p.kubeClient.CoreV1().PersistentVolumes().Delete(volume.GetName(), &metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("Kubernetes frontend failed to contact the API server and delete the PV: %s", err.Error())
	}
	return err
}

func (p *Plugin) addVolume(obj interface{}) {
	volume, ok := obj.(*v1.PersistentVolume)
	if !ok {
		log.Errorf("Kubernetes frontend expected PV; handler got %v", obj)
		return
	}
	p.processVolume(volume, "add")
}

func (p *Plugin) updateVolume(oldObj, newObj interface{}) {
	volume, ok := newObj.(*v1.PersistentVolume)
	if !ok {
		log.Errorf("Kubernetes frontend expected PV; handler got %v", newObj)
		return
	}
	p.processVolume(volume, "update")
}

func (p *Plugin) deleteVolume(obj interface{}) {
	volume, ok := obj.(*v1.PersistentVolume)
	if !ok {
		log.Errorf("Kubernetes frontend expected PV; handler got %v", obj)
		return
	}
	p.processVolume(volume, "delete")
}

func (p *Plugin) processVolume(
	volume *v1.PersistentVolume, eventType string,
) {
	pvSize := volume.Spec.Capacity[v1.ResourceStorage]
	log.WithFields(log.Fields{
		"PV":             volume.Name,
		"PV_phase":       volume.Status.Phase,
		"PV_size":        pvSize.String(),
		"PV_uid":         volume.UID,
		"PV_accessModes": volume.Spec.AccessModes,
		"PV_annotations": volume.Annotations,
		"PV_eventType":   eventType,
	}).Debug("Kubernetes frontend got notified of a PV.")

	// Validating the PV (making sure it's provisioned by Trident)
	if volume.ObjectMeta.Annotations[AnnDynamicallyProvisioned] !=
		AnnOrchestrator {
		return
	}

	switch eventType {
	case "delete":
		p.processDeletedVolume(volume)
		return
	case "add", "update":
		p.processUpdatedVolume(volume)
	default:
		log.WithFields(log.Fields{
			"PV":    volume.Name,
			"event": eventType,
		}).Error("Kubernetes frontend didn't recognize the notification event corresponding to the PV!")
		return
	}
}

// processDeletedVolume cleans up Trident-created PVs.
func (p *Plugin) processDeletedVolume(volume *v1.PersistentVolume) {
	// This method can get called under two scenarios:
	// (1) Deletion of a PVC has resulted in deletion of a PV and the
	//     corresponding volume.
	// (2) An admin has deleted the PV before deleting the PVC. Here we handle
	// this case for versions of Kubernetes that don't support the storage
	// object in use protection feature. This feature was beta in v1.10 and
	// stable in v.11.

	// First, make sure the PVC is present and is in the Lost phase.
	claim, err := p.GetPVCForPV(volume)
	if claim == nil || err != nil {
		return
	}
	if claim.Status.Phase != v1.ClaimLost {
		return
	}

	// Second, take the appropriate action based on the PV's reclaim policy.
	if volume.Spec.PersistentVolumeReclaimPolicy == v1.PersistentVolumeReclaimRetain {
		return
	}

	// Third, we need to delete the corresponding volume.
	if vol, _ := p.orchestrator.GetVolume(volume.Name); vol == nil {
		return
	}
	err = p.orchestrator.DeleteVolume(volume.Name)
	if err != nil {
		message := "failed to delete the provisioned volume for the lost PVC."
		p.updatePVCWithEvent(claim, v1.EventTypeWarning, "FailedVolumeDelete", message)
		log.WithFields(log.Fields{
			"PVC":    claim.Name,
			"volume": volume.Name,
		}).Errorf("Kubernetes frontend %s", message)
	} else {
		message := "deleted the provisioned volume for the lost PVC."
		p.updatePVCWithEvent(claim, v1.EventTypeNormal, "VolumeDelete", message)
		log.WithFields(log.Fields{
			"PVC":    claim.Name,
			"volume": volume.Name,
		}).Infof("Kubernetes frontend %s", message)
	}
	return
}

// processUpdatedVolume processes updated Trident-created PVs.
func (p *Plugin) processUpdatedVolume(volume *v1.PersistentVolume) {
	switch volume.Status.Phase {
	case v1.VolumePending:
		return
	case v1.VolumeAvailable:
		return
	case v1.VolumeBound:
		return
	case v1.VolumeReleased, v1.VolumeFailed:
		if volume.Spec.PersistentVolumeReclaimPolicy != v1.PersistentVolumeReclaimDelete {
			return
		}
		err := p.orchestrator.DeleteVolume(volume.Name)
		if err != nil && !core.IsNotFoundError(err) {
			// Updating the PV's phase to "VolumeFailed", so that a storage admin can take action.
			message := fmt.Sprintf("failed to delete the volume for PV %s: %s. Will eventually retry, "+
				"but volume and PV may need to be manually deleted.", volume.Name, err.Error())
			p.updateVolumePhaseWithEvent(volume, v1.VolumeFailed, v1.EventTypeWarning, "FailedVolumeDelete", message)
			log.Errorf("Kubernetes frontend %s", message)
			// PV needs to be manually deleted by the admin after removing the volume.
			return
		}
		err = p.kubeClient.CoreV1().PersistentVolumes().Delete(volume.Name, &metav1.DeleteOptions{})
		if err != nil {
			if !strings.HasSuffix(err.Error(), "not found") {
				// PVs provisioned by external provisioners seem to end up in
				// the failed state as Kubernetes doesn't recognize them.
				log.Errorf("Kubernetes frontend failed to delete the PV: %s", err.Error())
			}
			return
		}
		log.WithFields(log.Fields{
			"PV":     volume.Name,
			"volume": volume.Name,
		}).Info("Kubernetes frontend deleted the provisioned volume and PV.")
	default:
		log.WithFields(log.Fields{
			"PV":       volume.Name,
			"PV_phase": volume.Status.Phase,
		}).Error("Kubernetes frontend doesn't recognize the volume phase.")
	}
}

// updateVolumePhaseWithEvent saves new volume phase to API server and emits
// given event on the volume. It saves the phase and emits the event only when
// the phase has actually changed from the version saved in API server.
// (Based on pkg/controller/volume/persistentvolume/pv_controller.go)
func (p *Plugin) updateVolumePhaseWithEvent(
	volume *v1.PersistentVolume, phase v1.PersistentVolumePhase, eventtype, reason, message string,
) (*v1.PersistentVolume, error) {

	if volume.Status.Phase == phase {
		// Nothing to do.
		return volume, nil
	}
	newVol, err := p.updateVolumePhase(volume, phase, message)
	if err != nil {
		return nil, err
	}

	p.eventRecorder.Event(newVol, eventtype, reason, message)

	return newVol, nil
}

// updateVolumePhase saves new volume phase to API server.
// (Based on pkg/controller/volume/persistentvolume/pv_controller.go)
func (p *Plugin) updateVolumePhase(
	volume *v1.PersistentVolume, phase v1.PersistentVolumePhase, message string,
) (*v1.PersistentVolume, error) {

	if volume.Status.Phase == phase {
		// Nothing to do.
		return volume, nil
	}

	volumeClone := volume.DeepCopy()

	volumeClone.Status.Phase = phase
	volumeClone.Status.Message = message

	return p.kubeClient.CoreV1().PersistentVolumes().UpdateStatus(volumeClone)
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
	class, ok := obj.(*k8sstoragev1beta.StorageClass)
	if ok {
		p.processClass(convertStorageClassV1BetaToV1(class), "add")
		return
	}
	classV1, ok := obj.(*k8sstoragev1.StorageClass)
	if !ok {
		log.Errorf("Kubernetes frontend expected storage.k8s.io/v1beta1 "+
			"or storage.k8s.io/v1 storage class; handler got %v", obj)
		return
	}
	p.processClass(classV1, "add")
}

func (p *Plugin) updateClass(oldObj, newObj interface{}) {
	class, ok := newObj.(*k8sstoragev1beta.StorageClass)
	if ok {
		p.processClass(convertStorageClassV1BetaToV1(class), "update")
		return
	}
	classV1, ok := newObj.(*k8sstoragev1.StorageClass)
	if !ok {
		log.Errorf("Kubernetes frontend expected storage.k8s.io/v1beta1 "+
			"or storage.k8s.io/v1 storage class; handler got %v", newObj)
		return
	}
	p.processClass(classV1, "update")
}

func (p *Plugin) deleteClass(obj interface{}) {
	class, ok := obj.(*k8sstoragev1beta.StorageClass)
	if ok {
		p.processClass(convertStorageClassV1BetaToV1(class), "delete")
		return
	}
	classV1, ok := obj.(*k8sstoragev1.StorageClass)
	if !ok {
		log.Errorf("Kubernetes frontend expected storage.k8s.io/v1beta1 "+
			"or storage.k8s.io/v1 storage class; handler got %v", obj)
		return
	}
	p.processClass(classV1, "delete")
}

func (p *Plugin) processClass(class *k8sstoragev1.StorageClass, eventType string) {
	log.WithFields(log.Fields{
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
		p.processAddedClass(class)
	case "delete":
		p.processDeletedClass(class)
	case "update":
		// Make sure Trident has a record of this storage class.
		if storageClass, _ := p.orchestrator.GetStorageClass(class.Name); storageClass == nil {
			log.WithFields(log.Fields{
				"storageClass": class.Name,
			}).Warn("Kubernetes frontend has no record of the updated " +
				"storage class; instead it will try to create it.")
			p.processAddedClass(class)
		} else {
			p.processUpdatedClass(class)
		}
	default:
		log.WithFields(log.Fields{
			"storageClass": class.Name,
			"event":        eventType,
		}).Error("Kubernetes frontend didn't recognize the notification event corresponding to the storage class!")
	}
}

func (p *Plugin) processAddedClass(class *k8sstoragev1.StorageClass) {
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
				log.WithFields(log.Fields{
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
				log.WithFields(log.Fields{
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
				log.WithFields(log.Fields{
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
				log.WithFields(log.Fields{
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
	sc, err := p.orchestrator.AddStorageClass(scConfig)
	if err != nil {
		log.WithFields(log.Fields{
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
				log.WithFields(log.Fields{
					"storageClass":           class.Name,
					"default_storageClasses": p.getDefaultStorageClasses(),
				}).Warn("Kubernetes frontend already manages more than one default storage class!")
			}
			p.mutex.Unlock()
		}

		log.WithFields(log.Fields{
			"storageClass":             class.Name,
			"storageClass_provisioner": class.Provisioner,
			"storageClass_parameters":  class.Parameters,
			"default_storageClasses":   p.getDefaultStorageClasses(),
		}).Info("Kubernetes frontend added the storage class.")
	}
}

func (p *Plugin) processDeletedClass(class *k8sstoragev1.StorageClass) {
	// Check if we're deleting the default storage class.
	if getAnnotation(class.Annotations, AnnDefaultStorageClass) == "true" {
		p.mutex.Lock()
		if p.defaultStorageClasses[class.Name] {
			delete(p.defaultStorageClasses, class.Name)
			log.WithFields(log.Fields{
				"storageClass": class.Name,
			}).Info("Kubernetes frontend will be deleting a default storage class.")
		}
		p.mutex.Unlock()
	}

	// Delete the storage class.
	err := p.orchestrator.DeleteStorageClass(class.Name)
	if err != nil {
		log.WithFields(log.Fields{
			"storageClass": class.Name,
		}).Error("Kubernetes frontend couldn't delete the storage class: ", err)
	} else {
		p.mutex.Lock()
		delete(p.storageClassCache, class.Name)
		log.WithFields(log.Fields{
			"storageClass": class.Name,
		}).Info("Kubernetes frontend deleted the storage class.")
		p.mutex.Unlock()
	}
}

func (p *Plugin) processUpdatedClass(class *k8sstoragev1.StorageClass) {
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
			log.WithFields(log.Fields{
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
	oldClaim, ok := oldObj.(*v1.PersistentVolumeClaim)
	if !ok {
		log.Errorf("Kubernetes frontend expected PVC; handler got %v", oldObj)
		return
	}
	newClaim, ok := newObj.(*v1.PersistentVolumeClaim)
	if !ok {
		log.Errorf("Kubernetes frontend expected PVC; handler got %v", oldObj)
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
		log.WithFields(log.Fields{
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
			log.WithFields(log.Fields{
				"PVC":          newClaim.Name,
				"storageClass": storageClass,
			}).Debugf("Kubernetes frontend %s", message)
			return
		}
	} else {
		message := fmt.Sprintf("no knowledge of storage class %s to conduct volume resize!", storageClass)
		p.updatePVCWithEvent(newClaim, v1.EventTypeWarning, "ResizeFailed", message)
		log.WithFields(log.Fields{
			"PVC":          newClaim.Name,
			"storageClass": storageClass,
		}).Debugf("Kubernetes frontend has %s", message)
		return
	}

	// If we get this far, we potentially have a valid resize operation.
	log.WithFields(log.Fields{
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
	pv, err := p.GetPVForPVC(newClaim)
	if err != nil {
		log.WithFields(log.Fields{
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
		log.WithFields(log.Fields{"PVC": newClaim.Name}).Debugf("Kubernetes frontend %s", message)
		return
	}

	// Resize the volume and PV
	if _, err = p.resizeVolumeAndPV(pv, newPVCSize); err != nil {
		message := fmt.Sprintf("failed in resizing the volume or PV: %v", err)
		p.updatePVCWithEvent(newClaim, v1.EventTypeWarning, "ResizeFailed", message)
		log.WithFields(log.Fields{"PVC": newClaim.Name}).Errorf("Kubernetes frontend %v", message)
		return
	}

	// Update the PVC
	updatedClaim, err := p.updatePVCSize(newClaim, newPVCSize)
	if err != nil {
		message := fmt.Sprintf("failed in updating the PVC size: %v.",
			err)
		if updatedClaim == nil {
			p.updatePVCWithEvent(newClaim, v1.EventTypeWarning, "ResizeFailed", message)
		} else {
			p.updatePVCWithEvent(updatedClaim, v1.EventTypeWarning, "ResizeFailed", message)
		}
		log.WithFields(log.Fields{"PVC": newClaim.Name}).Errorf("Kubernetes frontend %v", message)
		return
	}
	message := "resized the PV and volume."
	p.updatePVCWithEvent(updatedClaim, v1.EventTypeNormal, "ResizeSuccess", message)
}

// GetPVForPVC returns the PV for a bound PVC.
func (p *Plugin) GetPVForPVC(claim *v1.PersistentVolumeClaim) (*v1.PersistentVolume, error) {
	if claim.Status.Phase != v1.ClaimBound {
		return nil, nil
	}

	return p.kubeClient.CoreV1().PersistentVolumes().Get(claim.Spec.VolumeName, metav1.GetOptions{})
}

// GetPVCForPV returns the PVC for a PV.
func (p *Plugin) GetPVCForPV(volume *v1.PersistentVolume) (*v1.PersistentVolumeClaim, error) {
	if volume.Spec.ClaimRef == nil {
		return nil, nil
	}

	return p.kubeClient.CoreV1().PersistentVolumeClaims(
		volume.Spec.ClaimRef.Namespace).Get(volume.Spec.ClaimRef.Name, metav1.GetOptions{})
}

// resizeVolumeAndPV resizes the volume on the storage backend and updates the PV size.
func (p *Plugin) resizeVolumeAndPV(pv *v1.PersistentVolume, newSize resource.Quantity) (*v1.PersistentVolume, error) {

	pvSize := pv.Spec.Capacity[v1.ResourceStorage]
	if pvSize.Cmp(newSize) < 0 {
		// Calling the orchestrator to resize the volume on the storage backend.
		if err := p.orchestrator.ResizeVolume(pv.Name,
			fmt.Sprintf("%d", newSize.Value())); err != nil {
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
	pvUpdated, err := PatchPV(p.kubeClient, pv, pvClone)
	if err != nil {
		return pv, err
	}
	updatedSize := pvUpdated.Spec.Capacity[v1.ResourceStorage]
	if updatedSize.Cmp(newSize) != 0 {
		return pvUpdated, err
	}

	log.WithFields(log.Fields{
		"PV":          pv.Name,
		"PV_old_size": pvSize.String(),
		"PV_new_size": updatedSize.String(),
	}).Info("Kubernetes frontend resized the PV.")

	return pvUpdated, nil
}

// updatePVCSize updates the PVC size.
func (p *Plugin) updatePVCSize(
	pvc *v1.PersistentVolumeClaim, newSize resource.Quantity,
) (*v1.PersistentVolumeClaim, error) {

	pvcClone := pvc.DeepCopy()
	pvcClone.Status.Capacity[v1.ResourceStorage] = newSize
	pvcUpdated, err := PatchPVCStatus(p.kubeClient, pvc, pvcClone)
	if err != nil {
		return nil, err
	}
	updatedSize := pvcUpdated.Status.Capacity[v1.ResourceStorage]
	if updatedSize.Cmp(newSize) != 0 {
		return pvcUpdated, err
	}

	oldSize := pvc.Status.Capacity[v1.ResourceStorage]
	log.WithFields(log.Fields{
		"PVC":          pvc.Name,
		"PVC_old_size": oldSize.String(),
		"PVC_new_size": updatedSize.String(),
	}).Info("Kubernetes frontend updated the PVC after resize.")

	return pvcUpdated, nil
}

func (p *Plugin) addVolumeSnapshot(obj interface{}) {
	volumeSnapshot, ok := obj.(*k8ssnapshotv1alpha1.VolumeSnapshot)
	if !ok {
		log.Errorf("Kubernetes frontend expected VolumeSnapshot CR; handler got %v", obj)
		return
	}
	p.processVolumeSnapshot(volumeSnapshot)
}

func (p *Plugin) processVolumeSnapshot(volSnap *k8ssnapshotv1alpha1.VolumeSnapshot) {

	// Validate the VolumeSnapshot CR
	// 1. Error must not be set
	if volSnap.Status.Error != nil {
		return
	}

	// 2. Snapshot must not exist
	if volSnap.Status.CreationTime != nil && volSnap.Status.ReadyToUse {
		log.WithFields(log.Fields{
			"volumeSnapshot": volSnap.Name,
		}).Debug("Kubernetes frontend ignored this VolumeSnapshot as " +
			"the snapshot has already been created")
		return
	}

	// 3. Snapshot source must be of PVC kind
	if volSnap.Spec.Source.Kind != "PersistentVolumeClaim" {
		log.WithFields(log.Fields{
			"volumeSnapshot": volSnap.Name,
		}).Debug("Kubernetes frontend ignored this VolumeSnapshot as " +
			"the source is not a PVC")
		return
	}

	// 4. VolumeSnapshotClass must be present
	volSnapClass := *volSnap.Spec.VolumeSnapshotClassName
	vsClass, err := p.kubeSnapClient.VolumesnapshotV1alpha1().VolumeSnapshotClasses().Get(volSnapClass, metav1.GetOptions{})
	if err != nil {
		log.WithFields(log.Fields{
			"volumeSnapshot":      volSnap.Name,
			"volumeSnapshotClass": volSnapClass,
		}).Debug("Kubernetes frontend ignored this VolumeSnapshot as " +
			"the VolumeSnapshotClass is not present")
		return
	}

	// 5. Trident must set as the snapshotter
	if vsClass.Snapshotter != AnnOrchestrator {
		log.WithFields(log.Fields{
			"volumeSnapshot":      volSnap.Name,
			"volumeSnapshotClass": volSnapClass,
		}).Debugf("Kubernetes frontend ignored this VolumeSnapshot as "+
			"%s is not set as the snapshotter", AnnOrchestrator)
		return
	}

	log.WithFields(log.Fields{
		"volumeSnapshot":      volSnap.Name,
		"namespace":           volSnap.Namespace,
		"volumeSnapshotClass": volSnapClass,
		"sourceKind":          volSnap.Spec.Source.Kind,
		"sourceName":          volSnap.Spec.Source.Name,
	}).Debug("Kubernetes frontend got notified of a VolumeSnapshot.")

	// Validate the source PVC
	// 1. PVC and VolumeSnapshotCR must be in the same namespace
	sourcePVC := volSnap.Spec.Source.Name
	k8sClient, err := p.getNamespacedKubeClient(&p.kubeConfig, volSnap.Namespace)
	if err != nil {
		log.WithFields(log.Fields{
			"namespace": volSnap.Namespace,
			"error":     err.Error(),
		}).Error("Kubernetes frontend couldn't create a client to namespace")
		return
	}
	claim, err := k8sClient.GetPVC(sourcePVC, metav1.GetOptions{})
	if err != nil {
		err = fmt.Errorf("snapshotting a PVC requires the PVC to be in the same namespace as the VolumeSnapshot")
		log.WithFields(log.Fields{
			"volumeSnapshot": volSnap.Name,
			"sourcePVC":      sourcePVC,
		}).Errorf("Kubernetes frontend detected an invalid configuration "+
			"for creating snapshot from a PVC: %v", err.Error())
		return
	}

	// 2. Trident should be set as the provisioner for the PVC
	provisioner := getClaimProvisioner(claim)
	if provisioner != AnnOrchestrator &&
		GetPersistentVolumeClaimClass(claim) != "" {
		log.WithFields(log.Fields{
			"volumeSnapshot":    volSnap.Name,
			"sourcePVC":         claim.Name,
			"sourceProvisioner": provisioner,
		}).Debugf("Kubernetes frontend ignored this VolumeSnapshot as the source PVC is not "+
			"tagged with %s as the storage provisioner!", AnnOrchestrator)
		return
	}

	// 3. PVC must be Bound to a PV
	if claim.Status.Phase != v1.ClaimBound {
		log.WithFields(log.Fields{
			"volumeSnapshot":  volSnap.Name,
			"sourcePVC":       claim.Name,
			"sourcePVCStatus": claim.Status.Phase,
		}).Debug("Kubernetes frontend ignored this VolumeSnapshot as the source PVC is not " +
			"bound to any volume")
		return
	}

	// Create the snapshot
	pvName := claim.Spec.VolumeName
	snapshotExt, err := p.orchestrator.CreateVolumeSnapshot(volSnap.Name, pvName)
	if err != nil {
		log.WithFields(log.Fields{
			"volumeSnapshot": volSnap.Name,
			"sourcePVC":      claim.Name,
			"error":          err.Error(),
		}).Error("Kubernetes frontend failed to create snapshot")
		_, uerr := p.updateVolumeSnapshotErrorStatus(volSnap, err.Error())
		if uerr != nil {
			log.WithError(uerr).Error("Kubernetes frontend failed to update VolumeSnapshot error status")
			return
		}
		return
	}

	// Create the VolumeSnapshotContent
	pv, err := p.kubeClient.CoreV1().PersistentVolumes().Get(pvName, metav1.GetOptions{})
	if err != nil {
		log.WithFields(log.Fields{
			"volumeSnapshot": volSnap.Name,
			"sourcePVC":      claim.Name,
			"error":          err.Error(),
		}).Errorf("Failed to get PV %s", pvName)
		_, uerr := p.updateVolumeSnapshotErrorStatus(volSnap, err.Error())
		if uerr != nil {
			log.WithError(uerr).Error("Kubernetes frontend failed to update VolumeSnapshot error status")
			return
		}
		return
	}
	volumeRef := &v1.ObjectReference{
		Kind:      "PersistentVolume",
		Name:      pvName,
		Namespace: pv.Namespace,
		UID:       pv.UID,
	}

	contentName := getVolumeSnapshotContentName(volSnap)
	volSnapContent := &k8ssnapshotv1alpha1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: contentName,
		},
		Spec: k8ssnapshotv1alpha1.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: &v1.ObjectReference{
				Kind:       "VolumeSnapshot",
				Namespace:  volSnap.Namespace,
				Name:       volSnap.Name,
				UID:        volSnap.UID,
				APIVersion: "snapshot.storage.k8s.io/v1alpha1",
			},
			VolumeSnapshotSource: k8ssnapshotv1alpha1.VolumeSnapshotSource{
				CSI: &k8ssnapshotv1alpha1.CSIVolumeSnapshotSource{
					Driver:         "trident-snapshotter",
					SnapshotHandle: snapshotExt.ID,
				},
			},
			PersistentVolumeRef:     volumeRef,
			VolumeSnapshotClassName: &volSnapClass,
		},
	}

	volSnapContent, err = p.kubeSnapClient.VolumesnapshotV1alpha1().VolumeSnapshotContents().Create(volSnapContent)
	if err != nil {
		log.WithFields(log.Fields{
			"volumeSnapshot": volSnap.Name,
			"sourcePVC":      claim.Name,
			"error":          err.Error(),
		}).Error("Failed to create VolumeSnapshotContent")
		_, uerr := p.updateVolumeSnapshotErrorStatus(volSnap, err.Error())
		if uerr != nil {
			log.WithError(uerr).Error("Kubernetes frontend failed to update VolumeSnapshot error status")
			return
		}
		return
	}

	// Update VolumeSnapshot status with snapshot info and bind with snapshot content
	newVolSnap, err := p.updateVolumeSnapshotStatus(volSnap, volSnapContent, snapshotExt)
	if err != nil {
		log.WithFields(log.Fields{
			"volumeSnapshot":        volSnap.Name,
			"volumeSnapshotContent": volSnapContent.Name,
			"sourcePVC":             claim.Name,
			"error":                 err.Error(),
		}).Error("Kubernetes frontend failed to update VolumeSnapshot status")
		_, uerr := p.updateVolumeSnapshotErrorStatus(volSnap, err.Error())
		if uerr != nil {
			log.WithError(uerr).Error("Kubernetes frontend failed to update VolumeSnapshot error status")
			return
		}
		return
	}

	message := "created a volume snapshot for the PVC."
	log.WithFields(log.Fields{
		"volumeSnapshot":        volSnap.Name,
		"volumeSnapshotContent": volSnapContent.Name,
		"sourcePVC":             claim.Name,
	}).Infof("Kubernetes frontend %s", message)
}

// updateVolumeSnapshotStatus updates the VolumeSnapshot.Status with given info
func (p *Plugin) updateVolumeSnapshotStatus(volSnap *k8ssnapshotv1alpha1.VolumeSnapshot,
	volSnapContent *k8ssnapshotv1alpha1.VolumeSnapshotContent, snapshotExt *storage.SnapshotExternal) (
	*k8ssnapshotv1alpha1.VolumeSnapshot, error) {

	// Parse create time
	createdAt, err := time.Parse(time.RFC3339, snapshotExt.Snapshot.Created)
	if err != nil {
		return nil, fmt.Errorf("failed to parse snapshot creation time: %s", err.Error())
	}

	status := volSnap.Status
	status.ReadyToUse = true
	status.Error = nil
	timeAt := &metav1.Time{
		Time: createdAt,
	}
	if status.CreationTime == nil {
		status.CreationTime = timeAt
	}

	volSnapClone := volSnap.DeepCopy()
	volSnapClone.Spec.SnapshotContentName = volSnapContent.Name
	volSnapClone.Status = status
	return p.kubeSnapClient.VolumesnapshotV1alpha1().VolumeSnapshots(
		volSnapClone.Namespace).Update(volSnapClone)
}

// updateVolumeSnapshotErrorStatus updates VolumeSnapshot.Status with Error
func (p *Plugin) updateVolumeSnapshotErrorStatus(volSnap *k8ssnapshotv1alpha1.VolumeSnapshot,
	message string) (*k8ssnapshotv1alpha1.VolumeSnapshot, error) {

	volSnapClone := volSnap.DeepCopy()
	statusError := &k8sstoragev1beta.VolumeError{
		Time: metav1.Time{
			Time: time.Now(),
		},
		Message: message,
	}
	volSnapClone.Status.Error = statusError
	volSnapClone.Status.ReadyToUse = false

	return p.kubeSnapClient.VolumesnapshotV1alpha1().VolumeSnapshots(
		volSnapClone.Namespace).Update(volSnapClone)
}
