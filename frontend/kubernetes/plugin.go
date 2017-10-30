// Copyright 2016 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"fmt"
	"strings"
	"sync"

	log "github.com/Sirupsen/logrus"
	dvp "github.com/netapp/netappdvp/storage_drivers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/apimachinery/pkg/runtime"
	k8s_version "k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	core_v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	k8s_storage_v1 "k8s.io/client-go/pkg/apis/storage/v1"
	k8s_storage_v1beta "k8s.io/client-go/pkg/apis/storage/v1beta1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	k8s_util_version "k8s.io/kubernetes/pkg/util/version"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/drivers/fake"
	"github.com/netapp/trident/k8s_client"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_class"
)

func ValidateKubeVersion(versionInfo *k8s_version.Info) (kubeVersion *k8s_util_version.Version, err error) {
	kubeVersion, err = nil, nil
	defer func() {
		if r := recover(); r != nil {
			log.WithFields(log.Fields{
				"panic": r,
			}).Errorf("Kubernetes frontend recovered from a panic!")
			err = fmt.Errorf("Kubernetes frontend recovered from a panic: %v", r)
		}
	}()
	kubeVersion = k8s_util_version.MustParseSemantic(versionInfo.GitVersion)
	if !kubeVersion.AtLeast(k8s_util_version.MustParseSemantic(KubernetesVersionMin)) {
		err = fmt.Errorf("Kubernetes frontend only works with Kubernetes %s or later!",
			KubernetesVersionMin)
	}
	return
}

type KubernetesPlugin struct {
	orchestrator             core.Orchestrator
	kubeClient               kubernetes.Interface
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
	mutex                    *sync.Mutex
	pendingClaimMatchMap     map[string]*v1.PersistentVolume
	kubernetesVersion        *k8s_version.Info
	defaultStorageClasses    map[string]bool
}

func NewPlugin(o core.Orchestrator, apiServerIP, kubeConfigPath string) (*KubernetesPlugin, error) {
	kubeConfig, err := clientcmd.BuildConfigFromFlags(apiServerIP, kubeConfigPath)
	if err != nil {
		return nil, err
	}
	return newKubernetesPlugin(o, kubeConfig)
}

func NewPluginInCluster(o core.Orchestrator) (*KubernetesPlugin, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return newKubernetesPlugin(o, kubeConfig)
}

func newKubernetesPlugin(orchestrator core.Orchestrator, kubeConfig *rest.Config) (*KubernetesPlugin, error) {
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	ret := &KubernetesPlugin{
		orchestrator:             orchestrator,
		kubeClient:               kubeClient,
		kubeConfig:               *kubeConfig,
		claimControllerStopChan:  make(chan struct{}),
		volumeControllerStopChan: make(chan struct{}),
		classControllerStopChan:  make(chan struct{}),
		mutex:                 &sync.Mutex{},
		pendingClaimMatchMap:  make(map[string]*v1.PersistentVolume),
		defaultStorageClasses: make(map[string]bool, 1),
	}

	ret.kubernetesVersion, err = kubeClient.Discovery().ServerVersion()
	if err != nil {
		return nil,
			fmt.Errorf("Kubernetes frontend couldn't retrieve API server's "+
				"version: %v", err)
	}
	log.WithFields(log.Fields{
		"version":    ret.kubernetesVersion.Major + "." + ret.kubernetesVersion.Minor,
		"gitVersion": ret.kubernetesVersion.GitVersion,
	}).Info("Kubernetes frontend determined the container orchestrator ",
		"version.")
	kubeVersion, err := ValidateKubeVersion(ret.kubernetesVersion)
	if err != nil &&
		strings.Contains(err.Error(),
			"Kubernetes frontend only works with Kubernetes") {
		return nil, err
	} else if err != nil {
		log.Warnf("%s v%s may not support "+
			"container orchestrator version %s.%s (%s)! "+
			"Supported versions for Kubernetes are %s-%s.",
			config.OrchestratorName, config.OrchestratorVersion,
			ret.kubernetesVersion.Major, ret.kubernetesVersion.Minor,
			ret.kubernetesVersion.GitVersion, KubernetesVersionMin,
			KubernetesVersionMax)
		log.Warnf("Kubernetes frontend proceeds as if you are running "+
			"Kubernetes %s!", KubernetesVersionMax)
		kubeVersion = k8s_util_version.MustParseSemantic(KubernetesVersionMax)
	}

	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(
		&core_v1.EventSinkImpl{
			Interface: kubeClient.Core().Events(""),
		})
	ret.eventRecorder = broadcaster.NewRecorder(api.Scheme,
		v1.EventSource{Component: AnnOrchestrator})

	// Setting up a watch for PVCs
	ret.claimSource = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return kubeClient.Core().PersistentVolumeClaims(
				v1.NamespaceAll).List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.Core().PersistentVolumeClaims(
				v1.NamespaceAll).Watch(options)
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
			return kubeClient.Core().PersistentVolumes().List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return kubeClient.Core().PersistentVolumes().Watch(options)
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
	switch {
	case kubeVersion.AtLeast(k8s_util_version.MustParseSemantic("v1.6.0")):
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
			&k8s_storage_v1.StorageClass{},
			KubernetesSyncPeriod,
			cache.ResourceEventHandlerFuncs{
				AddFunc:    ret.addClass,
				UpdateFunc: ret.updateClass,
				DeleteFunc: ret.deleteClass,
			},
		)
	case kubeVersion.AtLeast(k8s_util_version.MustParseSemantic("v1.4.0")):
		ret.classSource = &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				return kubeClient.StorageV1beta1().StorageClasses().List(options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				return kubeClient.StorageV1beta1().StorageClasses().Watch(options)
			},
		}
		_, ret.classController = cache.NewInformer(
			ret.classSource,
			&k8s_storage_v1beta.StorageClass{},
			KubernetesSyncPeriod,
			cache.ResourceEventHandlerFuncs{
				AddFunc:    ret.addClass,
				UpdateFunc: ret.updateClass,
				DeleteFunc: ret.deleteClass,
			},
		)
	}
	return ret, nil
}

func (p *KubernetesPlugin) Activate() error {
	go p.claimController.Run(p.claimControllerStopChan)
	go p.volumeController.Run(p.volumeControllerStopChan)
	go p.classController.Run(p.classControllerStopChan)
	return nil
}

func (p *KubernetesPlugin) Deactivate() error {
	close(p.claimControllerStopChan)
	close(p.volumeControllerStopChan)
	close(p.classControllerStopChan)
	return nil
}

func (p *KubernetesPlugin) GetName() string {
	return "kubernetes"
}

func (p *KubernetesPlugin) Version() string {
	return p.kubernetesVersion.GitVersion
}

func getUniqueClaimName(claim *v1.PersistentVolumeClaim) string {
	id := string(claim.UID)
	r := strings.NewReplacer("-", "", "_", "", " ", "", ",", "")
	id = r.Replace(id)
	if len(id) > 5 {
		id = id[:5]
	}
	return fmt.Sprintf("%s-%s-%s", claim.Namespace, claim.Name, id)
}

func (p *KubernetesPlugin) addClaim(obj interface{}) {
	claim, ok := obj.(*v1.PersistentVolumeClaim)
	if !ok {
		log.Panicf("Kubernetes frontend expected PVC; handler got %v", obj)
	}
	p.processClaim(claim, "add")
}

func (p *KubernetesPlugin) updateClaim(oldObj, newObj interface{}) {
	claim, ok := newObj.(*v1.PersistentVolumeClaim)
	if !ok {
		log.Panicf("Kubernetes frontend expected PVC; handler got %v", newObj)
	}
	p.processClaim(claim, "update")
}

func (p *KubernetesPlugin) deleteClaim(obj interface{}) {
	claim, ok := obj.(*v1.PersistentVolumeClaim)
	if !ok {
		log.Panicf("Kubernetes frontend expected PVC; handler got %v", obj)
	}
	p.processClaim(claim, "delete")
}

func (p *KubernetesPlugin) processClaim(
	claim *v1.PersistentVolumeClaim,
	eventType string,
) {
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
	if claim.Spec.StorageClassName != nil &&
		*claim.Spec.StorageClassName == "" {
		log.WithFields(log.Fields{
			"PVC": claim.Name,
		}).Debug("Kubernetes frontend ignores this PVC as an empty string " +
			"was specified for the storage class!")
		return
	}
	kubeVersion, _ := ValidateKubeVersion(p.kubernetesVersion)
	switch {
	case kubeVersion.AtLeast(k8s_util_version.MustParseSemantic("v1.5.0")):
		// Reject PVCs whose provisioner was not set to Trident.
		// However, don't reject a PVC whose provisioner wasn't set
		// just because no default storage class existed at the time
		// of the PVC creation.
		if provisioner != AnnOrchestrator &&
			GetPersistentVolumeClaimClass(claim) != "" {
			log.WithFields(log.Fields{
				"PVC":             claim.Name,
				"PVC_provisioner": provisioner,
			}).Debugf("Kubernetes frontend ignores this PVC as it's not "+
				"tagged with %s as the storage provisioner!", AnnOrchestrator)
			return
		}
		if kubeVersion.AtLeast(k8s_util_version.MustParseSemantic("v1.6.0")) &&
			GetPersistentVolumeClaimClass(claim) == "" &&
			claim.Spec.StorageClassName == nil {
			// Check if the default storage class should be used.
			p.mutex.Lock()
			if len(p.defaultStorageClasses) == 1 {
				// Using the default storage class
				var defaultStorageClassName string
				for defaultStorageClassName, _ = range p.defaultStorageClasses {
					claim.Spec.StorageClassName = &defaultStorageClassName
					break
				}
				log.WithFields(log.Fields{
					"PVC": claim.Name,
					"storageClass_default": defaultStorageClassName,
				}).Info("Kubernetes frontend will use the default storage " +
					"class for this PVC.")
			} else if len(p.defaultStorageClasses) > 1 {
				log.WithFields(log.Fields{
					"PVC": claim.Name,
					"default_storageClasses": p.getDefaultStorageClasses(),
				}).Warn("Kubernetes frontend ignores this PVC as more than " +
					"one default storage class has been configured!")
				p.mutex.Unlock()
				return
			} else {
				log.WithFields(log.Fields{
					"PVC": claim.Name,
				}).Debug("Kubernetes frontend ignores this PVC as no storage class " +
					"was specified and no default storage class was configured!")
				p.mutex.Unlock()
				return
			}
			p.mutex.Unlock()
		}
	}
	// If storage class is still empty (after considering the default storage
	// class), ignore the PVC as Kubernetes doesn't allow a storage class with
	// empty string as the name.
	if GetPersistentVolumeClaimClass(claim) == "" {
		log.WithFields(log.Fields{
			"PVC": claim.Name,
		}).Debug("Kubernetes frontend ignores this PVC as no storage class " +
			"was specified!")
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
		}).Error("Kubernetes frontend didn't recognize the notification event ",
			"corresponding to the PVC!")
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
			message := "Kubernetes frontend ignores PVCs with label selectors!"
			p.updateClaimWithEvent(claim, v1.EventTypeWarning, "IgnoredClaim",
				message)
			log.WithFields(log.Fields{
				"PVC": claim.Name,
			}).Debug(message)
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
func (p *KubernetesPlugin) processBoundClaim(claim *v1.PersistentVolumeClaim) {
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
			}).Warnf("Kubernetes frontend detected PVC isn't bound to the "+
				"intended PV, but it failed to delete the PV or the "+
				"corresponding provisioned volume "+
				"(will retry upon resync): %s", err.Error())
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
func (p *KubernetesPlugin) processLostClaim(claim *v1.PersistentVolumeClaim) {
	volName := getUniqueClaimName(claim)

	defer func() {
		// Remove the pending claim, if present.
		p.mutex.Lock()
		delete(p.pendingClaimMatchMap, volName)
		p.mutex.Unlock()
	}()

	// A PVC is in the "Lost" phase when the corresponding PV is deleted.
	// Check whether we need to recycle the claim and the corresponding volume.
	if getClaimReclaimPolicy(claim) == string(v1.PersistentVolumeReclaimRetain) {
		return
	}

	// We need to delete the corresponding volume.
	if p.orchestrator.GetVolume(volName) == nil {
		return
	}
	_, err := p.orchestrator.DeleteVolume(volName)
	if err != nil {
		message := "Kubernetes frontend failed to delete the provisioned " +
			"volume for the lost PVC (will retry upon resync)."
		p.updateClaimWithEvent(claim, v1.EventTypeWarning,
			"FailedVolumeDelete", message)
		log.WithFields(log.Fields{
			"PVC":    claim.Name,
			"volume": volName,
		}).Error(message)
	} else {
		message := "Kubernetes frontend successfully deleted the " +
			"provisioned volume for the lost PVC."
		p.updateClaimWithEvent(claim, v1.EventTypeNormal,
			"VolumeDelete", message)
		log.WithFields(log.Fields{
			"PVC":    claim.Name,
			"volume": volName,
		}).Info(message)
	}
	return
}

// processDeletedClaim cleans up Trident-created PVs.
func (p *KubernetesPlugin) processDeletedClaim(claim *v1.PersistentVolumeClaim) {
	// No major action needs to be taken as deleting a claim would result in
	// the corresponding PV to end up in the "Released" phase, which gets
	// handled by processUpdatedVolume.
	// Remove the pending claim, if present.
	p.mutex.Lock()
	delete(p.pendingClaimMatchMap, getUniqueClaimName(claim))
	p.mutex.Unlock()
}

// processPendingClaim processes PVCs in the pending phase.
func (p *KubernetesPlugin) processPendingClaim(claim *v1.PersistentVolumeClaim) {
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
			}).Error("Kubernetes frontend failed in processing "+
				"an updated claim (will try again upon resync): ",
				err)
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
			p.updateClaimWithEvent(claim, v1.EventTypeNormal,
				"ProvisioningFailed", err.Error())
		} else {
			p.updateClaimWithEvent(claim, v1.EventTypeWarning,
				"ProvisioningFailed", err.Error())
		}
		return
	}
	p.mutex.Lock()
	p.pendingClaimMatchMap[orchestratorClaimName] = pv
	p.mutex.Unlock()
	message := "Kubernetes frontend provisioned a volume and a PV for the PVC."
	p.updateClaimWithEvent(claim, v1.EventTypeNormal,
		"ProvisioningSuccess", message)
	log.WithFields(log.Fields{
		"PVC":       claim.Name,
		"PV":        orchestratorClaimName,
		"PV_volume": pv.Name,
	}).Info(message)
}

func (p *KubernetesPlugin) createVolumeAndPV(uniqueName string,
	claim *v1.PersistentVolumeClaim,
) (pv *v1.PersistentVolume, err error) {
	var (
		nfsSource   *v1.NFSVolumeSource
		iscsiSource *v1.ISCSIVolumeSource
		vol         *storage.VolumeExternal
	)

	defer func() {
		if vol != nil && err != nil {
			err1 := err
			// Delete the volume on the backend
			_, err = p.orchestrator.DeleteVolume(vol.Config.Name)
			if err != nil {
				err2 := "Kubernetes frontend couldn't delete the volume " +
					"after failed creation: " + err.Error()
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

	// TODO: A quick way to support v1 storage classes before changing unit tests
	if _, found := annotations[AnnClass]; !found {
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[AnnClass] = GetPersistentVolumeClaimClass(claim)
	}

	vol, err = p.orchestrator.AddVolume(
		getVolumeConfig(accessModes, uniqueName, size, annotations))
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
			Capacity:    v1.ResourceList{v1.ResourceStorage: size},
			ClaimRef:    &claimRef,
			// Default policy is "Delete".
			PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
		},
	}
	kubeVersion, _ := ValidateKubeVersion(p.kubernetesVersion)
	switch {
	case kubeVersion.AtLeast(k8s_util_version.MustParseSemantic("v1.6.0")):
		pv.Spec.StorageClassName = GetPersistentVolumeClaimClass(claim)
	}
	if getClaimReclaimPolicy(claim) ==
		string(v1.PersistentVolumeReclaimRetain) {
		// Extra flexibility in our implementation.
		pv.Spec.PersistentVolumeReclaimPolicy =
			v1.PersistentVolumeReclaimRetain
	}

	k8sClient, newKubeClientErr := k8s_client.NewKubeClient(&p.kubeConfig, claim.Namespace)
	if newKubeClientErr != nil {
		log.WithFields(log.Fields{
			"claim.Namespace": claim.Namespace,
		}).Warnf("Kubernetes frontend couldn't create a client to namespace: %v error: %v",
			claim.Namespace, newKubeClientErr.Error())
	}

	driverType := p.orchestrator.GetDriverTypeForVolume(vol)
	switch {
	case driverType == dvp.SolidfireSANStorageDriverName ||
		driverType == dvp.OntapSANStorageDriverName ||
		driverType == dvp.EseriesIscsiStorageDriverName:
		iscsiSource, err = CreateISCSIVolumeSource(k8sClient, kubeVersion, vol)
		if err != nil {
			return
		}
		pv.Spec.ISCSI = iscsiSource
	case driverType == dvp.OntapNASStorageDriverName ||
		driverType == dvp.OntapNASQtreeStorageDriverName:
		nfsSource = CreateNFSVolumeSource(vol)
		pv.Spec.NFS = nfsSource
	case driverType == fake.FakeStorageDriverName:
		if vol.Config.Protocol == config.File {
			nfsSource = CreateNFSVolumeSource(vol)
			pv.Spec.NFS = nfsSource
		} else if vol.Config.Protocol == config.Block {
			iscsiSource, err = CreateISCSIVolumeSource(k8sClient, kubeVersion, vol)
			if err != nil {
				return
			}
			pv.Spec.ISCSI = iscsiSource
		}
	default:
		// Unknown driver for the frontend plugin or for Kubernetes.
		// Provisioned volume should get deleted.
		log.WithFields(log.Fields{
			"volume": vol.Config.Name,
			"type":   p.orchestrator.GetVolumeType(vol),
			"driver": driverType,
		}).Error("Kubernetes frontend doesn't recognize this type of volume; ",
			"deleting the provisioned volume.")
		err = fmt.Errorf("Unrecognized volume type by Kubernetes")
		return
	}
	pv, err = p.kubeClient.Core().PersistentVolumes().Create(pv)
	return
}

func (p *KubernetesPlugin) deleteVolumeAndPV(volume *v1.PersistentVolume) error {
	found, err := p.orchestrator.DeleteVolume(volume.GetName())
	if found && err != nil {
		message := fmt.Sprintf(
			"Kubernetes frontend failed to delete the volume "+
				"for PV %s: %s. Volume and PV may need to be manually deleted.",
			volume.GetName(), err.Error())
		p.updateVolumePhaseWithEvent(volume, v1.VolumeFailed,
			v1.EventTypeWarning, "FailedVolumeDelete", message)
		return fmt.Errorf(message)
	}
	err = p.kubeClient.Core().PersistentVolumes().Delete(volume.GetName(),
		&metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("Kubernetes frontend failed to contact the "+
			"API server and delete the PV: %s", err.Error())
	}
	return err
}

// getClaimReclaimPolicy returns the reclaim policy for a claim
// (similar to ReclaimPolicy for PVs)
func getClaimReclaimPolicy(claim *v1.PersistentVolumeClaim) string {
	if policy, found := claim.Annotations[AnnReclaimPolicy]; found {
		return policy
	}
	return ""
}

// getClaimProvisioner returns the provisioner for a claim
func getClaimProvisioner(claim *v1.PersistentVolumeClaim) string {
	if provisioner, found := claim.Annotations[AnnStorageProvisioner]; found {
		return provisioner
	}
	return ""
}

func (p *KubernetesPlugin) addVolume(obj interface{}) {
	volume, ok := obj.(*v1.PersistentVolume)
	if !ok {
		log.Panicf("Kubernetes frontend expected PV; handler got %v", obj)
	}
	p.processVolume(volume, "add")
}

func (p *KubernetesPlugin) updateVolume(oldObj, newObj interface{}) {
	volume, ok := newObj.(*v1.PersistentVolume)
	if !ok {
		log.Panicf("Kubernetes frontend expected PV; handler got %v", newObj)
	}
	p.processVolume(volume, "update")
}

func (p *KubernetesPlugin) deleteVolume(obj interface{}) {
	volume, ok := obj.(*v1.PersistentVolume)
	if !ok {
		log.Panicf("Kubernetes frontend expected PV; handler got %v", obj)
	}
	p.processVolume(volume, "delete")
}

func (p *KubernetesPlugin) processVolume(
	volume *v1.PersistentVolume,
	eventType string,
) {
	log.WithFields(log.Fields{
		"PV":             volume.Name,
		"PV_phase":       volume.Status.Phase,
		"PV_size":        volume.Spec.Capacity[v1.ResourceStorage],
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
		}).Error("Kubernetes frontend didn't recognize the notification event ",
			"corresponding to the PV!")
		return
	}
}

// processDeletedVolume cleans up Trident-created PVs.
func (p *KubernetesPlugin) processDeletedVolume(volume *v1.PersistentVolume) {
	// This method can get called under two scenarios:
	// (1) Deletion of a PVC has resulted in deletion of a PV and the
	//     corresponding volume.
	// (2) An admin has deleted the PV before deleting PVC. processLostClaim
	//     should handle this scenario.
	// Therefore, no action needs to be taken here.
}

// processUpdatedVolume processes updated Trident-created PVs.
func (p *KubernetesPlugin) processUpdatedVolume(volume *v1.PersistentVolume) {
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
		found, err := p.orchestrator.DeleteVolume(volume.Name)
		if found && err != nil {
			// Updating the PV's phase to "VolumeFailed", so that
			// a storage admin can take action.
			message := fmt.Sprintf(
				"Kubernetes frontend failed to delete the volume "+
					"for PV %s: %s. Will eventually retry, but volume and PV "+
					"may need to be manually deleted.", volume.Name,
				err.Error())
			p.updateVolumePhaseWithEvent(volume, v1.VolumeFailed,
				v1.EventTypeWarning, "FailedVolumeDelete", message)
			log.Errorf(message)
			// PV needs to be manually deleted by the admin after removing
			// the volume.
			return
		}
		err = p.kubeClient.Core().PersistentVolumes().Delete(volume.Name,
			&metav1.DeleteOptions{})
		if err != nil {
			if !strings.HasSuffix(err.Error(), "not found") {
				// PVs provisioned by external provisioners seem to end up in
				// the failed state as Kubernetes doesn't recognize them.
				log.Errorf("Kubernetes frontend failed to delete the PV: %s",
					err.Error())
			}
			return
		}
		log.WithFields(log.Fields{
			"PV":     volume.Name,
			"volume": volume.Name,
		}).Info("Kubernetes frontend successfully deleted the provisioned " +
			"volume and PV.")
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
func (p *KubernetesPlugin) updateVolumePhaseWithEvent(
	volume *v1.PersistentVolume, phase v1.PersistentVolumePhase, eventtype,
	reason, message string) (*v1.PersistentVolume, error) {
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
func (p *KubernetesPlugin) updateVolumePhase(volume *v1.PersistentVolume, phase v1.PersistentVolumePhase, message string) (*v1.PersistentVolume, error) {
	if volume.Status.Phase == phase {
		// Nothing to do.
		return volume, nil
	}

	clone, err := conversion.NewCloner().DeepCopy(volume)
	if err != nil {
		return nil, fmt.Errorf("Error cloning claim: %v", err)
	}
	volumeClone, ok := clone.(*v1.PersistentVolume)
	if !ok {
		return nil, fmt.Errorf("Unexpected volume cast error : %v", volumeClone)
	}

	volumeClone.Status.Phase = phase
	volumeClone.Status.Message = message

	newVol, err := p.kubeClient.Core().PersistentVolumes().UpdateStatus(volumeClone)
	if err != nil {
		return newVol, err
	}
	return newVol, err
}

// updateClaimWithEvent emits given event on the claim.
// (Based on pkg/controller/volume/persistentvolume/pv_controller.go)
func (p *KubernetesPlugin) updateClaimWithEvent(
	claim *v1.PersistentVolumeClaim, eventtype, reason, message string,
) (*v1.PersistentVolumeClaim, error) {
	p.eventRecorder.Event(claim, eventtype, reason, message)
	return claim, nil
}

func convertStorageClassV1BetaToV1(class *k8s_storage_v1beta.StorageClass) *k8s_storage_v1.StorageClass {
	// For now we just copy the fields used by Trident.
	v1Class := &k8s_storage_v1.StorageClass{
		Provisioner: class.Provisioner,
		Parameters:  class.Parameters,
	}
	v1Class.Name = class.Name
	return v1Class
}

func (p *KubernetesPlugin) addClass(obj interface{}) {
	class, ok := obj.(*k8s_storage_v1beta.StorageClass)
	if ok {
		p.processClass(convertStorageClassV1BetaToV1(class), "add")
		return
	}
	classV1, ok := obj.(*k8s_storage_v1.StorageClass)
	if !ok {
		log.Panicf("Kubernetes frontend expected storage.k8s.io/v1beta1 "+
			"or storage.k8s.io/v1 storage class; handler got %v", obj)
	}
	p.processClass(classV1, "add")
}

func (p *KubernetesPlugin) updateClass(oldObj, newObj interface{}) {
	class, ok := newObj.(*k8s_storage_v1beta.StorageClass)
	if ok {
		p.processClass(convertStorageClassV1BetaToV1(class), "update")
		return
	}
	classV1, ok := newObj.(*k8s_storage_v1.StorageClass)
	if !ok {
		log.Panicf("Kubernetes frontend expected storage.k8s.io/v1beta1 "+
			"or storage.k8s.io/v1 storage class; handler got %v", newObj)
	}
	p.processClass(classV1, "update")
}

func (p *KubernetesPlugin) deleteClass(obj interface{}) {
	class, ok := obj.(*k8s_storage_v1beta.StorageClass)
	if ok {
		p.processClass(convertStorageClassV1BetaToV1(class), "delete")
		return
	}
	classV1, ok := obj.(*k8s_storage_v1.StorageClass)
	if !ok {
		log.Panicf("Kubernetes frontend expected storage.k8s.io/v1beta1 "+
			"or storage.k8s.io/v1 storage class; handler got %v", obj)
	}
	p.processClass(classV1, "delete")
}

func (p *KubernetesPlugin) processClass(
	class *k8s_storage_v1.StorageClass,
	eventType string,
) {
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
		storageClass := p.orchestrator.GetStorageClass(class.Name)
		if storageClass == nil {
			log.WithFields(log.Fields{
				"storageClass": class.Name,
			}).Warn("Kubernetes frontend has no record of the updated " +
				"storage class; instead it will try to create it.")
			p.processAddedClass(class)
			return
		}
		p.processUpdatedClass(class)
	default:
		log.WithFields(log.Fields{
			"storageClass": class.Name,
			"event":        eventType,
		}).Error("Kubernetes frontend didn't recognize the notification event corresponding to the storage class!")
		return
	}
}

func (p *KubernetesPlugin) processAddedClass(class *k8s_storage_v1.StorageClass) {
	scConfig := new(storage_class.Config)
	scConfig.Name = class.Name
	scConfig.Attributes = make(map[string]storage_attribute.Request)

	// Populate storage class config attributes and backend storage pools
	for k, v := range class.Parameters {
		if k == storage_attribute.BackendStoragePools {
			// format:     backendStoragePools: "backend1:vc1,vc2;backend2:vc1"
			backendVCs, err := storage_attribute.CreateBackendStoragePoolsMapFromEncodedString(v)
			if err != nil {
				log.WithFields(log.Fields{
					"storageClass":             class.Name,
					"storageClass_provisioner": class.Provisioner,
					"storageClass_parameters":  class.Parameters,
				}).Error("Kubernetes frontend couldn't process %s parameter: ",
					storage_attribute.BackendStoragePools, err)
			}
			scConfig.BackendStoragePools = backendVCs
			continue
		}
		// format:     attribute: "type:value"
		req, err := storage_attribute.CreateAttributeRequestFromTypedValue(k, v)
		if err != nil {
			log.WithFields(log.Fields{
				"storageClass":             class.Name,
				"storageClass_provisioner": class.Provisioner,
				"storageClass_parameters":  class.Parameters,
			}).Error("Kubernetes frontend couldn't process the encoded storage class attribute: ", err)
			return
		}
		scConfig.Attributes[k] = req
	}

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
				}).Warn("Kubernetes frontend already manages more than one " +
					"default storage class!")
			}
			p.mutex.Unlock()
		}
		log.WithFields(log.Fields{
			"storageClass":             class.Name,
			"storageClass_provisioner": class.Provisioner,
			"storageClass_parameters":  class.Parameters,
			"default_storageClasses":   p.getDefaultStorageClasses(),
		}).Info("Kubernetes frontend successfully added the storage class.")
	}
	return
}

func (p *KubernetesPlugin) processDeletedClass(class *k8s_storage_v1.StorageClass) {
	// Check if we're deleting the default storage class.
	if getAnnotation(class.Annotations, AnnDefaultStorageClass) == "true" {
		p.mutex.Lock()
		if p.defaultStorageClasses[class.Name] {
			delete(p.defaultStorageClasses, class.Name)
			log.WithFields(log.Fields{
				"storageClass": class.Name,
			}).Info("Kubernetes frontend will be deleting a default " +
				"storage class.")
		}
		p.mutex.Unlock()
	}

	// Delete the storage class.
	deleted, err := p.orchestrator.DeleteStorageClass(class.Name)
	if err != nil {
		log.WithFields(log.Fields{
			"storageClass": class.Name,
		}).Error("Kubernetes frontend couldn't delete the storage class: ", err)
		return
	}
	if deleted {
		log.WithFields(log.Fields{
			"storageClass": class.Name,
		}).Info("Kubernetes frontend successfully deleted the storage class.")
	}
	return
}

func (p *KubernetesPlugin) processUpdatedClass(class *k8s_storage_v1.StorageClass) {
	// Here we only check for updates associated with the default storage class.
	p.mutex.Lock()
	defer func() {
		p.mutex.Unlock()
	}()

	if p.defaultStorageClasses[class.Name] {
		// It's an update to a default storage class.
		// Check to see if it's still a default storage class.
		if getAnnotation(class.Annotations, AnnDefaultStorageClass) != "true" {
			delete(p.defaultStorageClasses, class.Name)
			return
		}
		log.WithFields(log.Fields{
			"storageClass":           class.Name,
			"default_storageClasses": p.getDefaultStorageClasses(),
		}).Debug("Kubernetes frontend only supports updating the " +
			"default storage class tag.")
		return
	} else {
		// It's an update to a non-default storage class.
		if getAnnotation(class.Annotations, AnnDefaultStorageClass) == "true" {
			// The update defines a new default storage class.
			p.defaultStorageClasses[class.Name] = true
			log.WithFields(log.Fields{
				"storageClass":           class.Name,
				"default_storageClasses": p.getDefaultStorageClasses(),
			}).Info("Kubernetes frontend added a new default storage class.")
			return
		}
		log.WithFields(log.Fields{
			"storageClass":           class.Name,
			"default_storageClasses": p.getDefaultStorageClasses(),
		}).Debug("Kubernetes frontend only supports updating the " +
			"default storage class tag.")
		return
	}
}

func (p *KubernetesPlugin) getDefaultStorageClasses() string {
	classes := make([]string, 0)
	for class, _ := range p.defaultStorageClasses {
		classes = append(classes, class)
	}
	return strings.Join(classes, ",")
}

// GetPersistentVolumeClaimClass returns StorageClassName. If no storage class was
// requested, it returns "".
func GetPersistentVolumeClaimClass(claim *v1.PersistentVolumeClaim) string {
	// Use beta annotation first
	if class, found := claim.Annotations[api.BetaStorageClassAnnotation]; found {
		return class
	}

	if claim.Spec.StorageClassName != nil {
		return *claim.Spec.StorageClassName
	}

	return ""
}
