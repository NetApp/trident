// Copyright 2016 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"fmt"
	"strings"

	log "github.com/Sirupsen/logrus"
	dvp "github.com/netapp/netappdvp/storage_drivers"
	"k8s.io/client-go/kubernetes"
	core_v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/unversioned"
	"k8s.io/client-go/pkg/api/v1"
	k8s_storage "k8s.io/client-go/pkg/apis/storage/v1beta1"
	"k8s.io/client-go/pkg/conversion"
	"k8s.io/client-go/pkg/runtime"
	"k8s.io/client-go/pkg/watch"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"

	"github.com/netapp/trident/core"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_class"
)

type KubernetesPlugin struct {
	orchestrator             core.Orchestrator
	kubeClient               kubernetes.Interface
	eventRecorder            record.EventRecorder
	pendingClaimMatchMap     map[string]*v1.PersistentVolume
	claimController          *cache.Controller
	claimControllerStopChan  chan struct{}
	claimSource              cache.ListerWatcher
	volumeController         *cache.Controller
	volumeControllerStopChan chan struct{}
	volumeSource             cache.ListerWatcher
	classController          *cache.Controller
	classControllerStopChan  chan struct{}
	classSource              cache.ListerWatcher
}

func NewPlugin(
	o core.Orchestrator, apiServerIP string) (*KubernetesPlugin, error) {
	kubeConfig, err := clientcmd.BuildConfigFromFlags(apiServerIP, "")
	if err != nil {
		return nil, err
	}
	return newForConfig(o, kubeConfig)
}

func NewPluginInCluster(o core.Orchestrator) (*KubernetesPlugin, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	return newForConfig(o, kubeConfig)
}

func newForConfig(
	o core.Orchestrator, kubeConfig *rest.Config,
) (*KubernetesPlugin, error) {
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}
	return newKubernetesPlugin(kubeClient, o), nil
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

func newKubernetesPlugin(
	kubeClient kubernetes.Interface,
	orchestrator core.Orchestrator,
) *KubernetesPlugin {
	ret := &KubernetesPlugin{
		orchestrator:             orchestrator,
		kubeClient:               kubeClient,
		claimControllerStopChan:  make(chan struct{}),
		volumeControllerStopChan: make(chan struct{}),
		classControllerStopChan:  make(chan struct{}),
		pendingClaimMatchMap:     make(map[string]*v1.PersistentVolume),
	}
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(
		&core_v1.EventSinkImpl{
			Interface: kubeClient.Core().Events(""),
		})
	ret.eventRecorder = broadcaster.NewRecorder(
		v1.EventSource{Component: AnnProvisioner})

	// Setting up a watch for PVCs
	ret.claimSource = &cache.ListWatch{
		ListFunc: func(options api.ListOptions) (runtime.Object, error) {
			var v1Options v1.ListOptions
			v1.Convert_api_ListOptions_To_v1_ListOptions(&options, &v1Options,
				nil)
			return kubeClient.Core().PersistentVolumeClaims(
				v1.NamespaceAll).List(v1Options)
		},
		WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
			var v1Options v1.ListOptions
			v1.Convert_api_ListOptions_To_v1_ListOptions(&options, &v1Options,
				nil)
			return kubeClient.Core().PersistentVolumeClaims(
				v1.NamespaceAll).Watch(v1Options)
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
		ListFunc: func(options api.ListOptions) (runtime.Object, error) {
			var v1Options v1.ListOptions
			v1.Convert_api_ListOptions_To_v1_ListOptions(&options, &v1Options,
				nil)
			return kubeClient.Core().PersistentVolumes().List(v1Options)
		},
		WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
			var v1Options v1.ListOptions
			v1.Convert_api_ListOptions_To_v1_ListOptions(&options, &v1Options,
				nil)
			return kubeClient.Core().PersistentVolumes().Watch(v1Options)
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

	// Setting up a watch for StorageClasses
	ret.classSource = &cache.ListWatch{
		ListFunc: func(options api.ListOptions) (runtime.Object, error) {
			var v1Options v1.ListOptions
			v1.Convert_api_ListOptions_To_v1_ListOptions(&options, &v1Options,
				nil)
			return kubeClient.Storage().StorageClasses().List(v1Options)
		},
		WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
			var v1Options v1.ListOptions
			v1.Convert_api_ListOptions_To_v1_ListOptions(&options, &v1Options,
				nil)
			return kubeClient.Storage().StorageClasses().Watch(v1Options)
		},
	}
	_, ret.classController = cache.NewInformer(
		ret.classSource,
		&k8s_storage.StorageClass{},
		KubernetesSyncPeriod,
		cache.ResourceEventHandlerFuncs{
			AddFunc:    ret.addClass,
			UpdateFunc: ret.updateClass,
			DeleteFunc: ret.deleteClass,
		},
	)
	return ret
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

func (km *KubernetesPlugin) GetName() string {
	return "kubernetes"
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
		"PVC":             claim.Name,
		"PVC_phase":       claim.Status.Phase,
		"PVC_size":        size.String(),
		"PVC_uid":         claim.UID,
		"PVC_accessModes": claim.Spec.AccessModes,
		"PVC_annotations": claim.Annotations,
		"PVC_volume":      claim.Spec.VolumeName,
		"PVC_eventType":   eventType,
	}).Debug("Kubernetes frontend got notified of a PVC.")

	// Filtering unrelated claims
	provisioner := getClaimProvisioner(claim)
	if provisioner == AnnProvisioner {
		// For k8s version >= 1.5
	} else if provisioner == "" {
		// For k8s version < 1.5
		pvcStorageClass := getClaimClass(claim)
		if pvcStorageClass == "" {
			return
		}
		if p.orchestrator.GetStorageClass(pvcStorageClass) == nil {
			return
		}
	} else {
		return
	}
	if claim.Spec.Selector != nil {
		// For now selector and storage class are mutally exclusive.
		message := "Kubernetes frontend ignores PVCs with label selectors!"
		p.updateClaimWithEvent(claim, v1.EventTypeWarning,
			"IgnoredClaim", message)
		log.WithFields(log.Fields{
			"PVC": claim.Name,
		}).Warn(message)
		return
	}

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
		}).Warn("Kubernetes frontend didn't recognize the notification event ",
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
		p.processPendingClaim(claim)
	default:
		log.WithFields(log.Fields{
			"PVC":       claim.Name,
			"PVC_phase": claim.Status.Phase,
		}).Warn("Kubernetes frontend doesn't recognize the claim phase.")
	}
}

// processBoundClaim validates whether a Trident-created PV got bound to the intended PVC.
func (p *KubernetesPlugin) processBoundClaim(claim *v1.PersistentVolumeClaim) {
	orchestratorClaimName := getUniqueClaimName(claim)
	deleteClaim := true

	defer func() {
		// Remove the pending claim, if present.
		if deleteClaim {
			delete(p.pendingClaimMatchMap, orchestratorClaimName)
		}
	}()

	pv, ok := p.pendingClaimMatchMap[orchestratorClaimName]
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
		delete(p.pendingClaimMatchMap, volName)
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
	delete(p.pendingClaimMatchMap, getUniqueClaimName(claim))
}

// processPendingClaim processes PVCs in the pending phase.
func (p *KubernetesPlugin) processPendingClaim(claim *v1.PersistentVolumeClaim) {
	orchestratorClaimName := getUniqueClaimName(claim)
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
			return
		}
		delete(p.pendingClaimMatchMap, orchestratorClaimName)
	}

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
	p.pendingClaimMatchMap[orchestratorClaimName] = pv
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

	// TODO: log volume creation in etcd
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
		TypeMeta: unversioned.TypeMeta{
			Kind:       "PersistentVolume",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: uniqueName,
			Annotations: map[string]string{
				AnnClass:                  getClaimClass(claim),
				AnnDynamicallyProvisioned: AnnProvisioner,
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
	if getClaimReclaimPolicy(claim) ==
		string(v1.PersistentVolumeReclaimRetain) {
		// Extra flexibility in our implementation.
		pv.Spec.PersistentVolumeReclaimPolicy =
			v1.PersistentVolumeReclaimRetain
	}

	driverType := p.orchestrator.GetDriverTypeForVolume(vol)
	switch {
	case driverType == dvp.SolidfireSANStorageDriverName ||
		driverType == dvp.OntapSANStorageDriverName:
		iscsiSource = CreateISCSIVolumeSource(vol.Config)
		pv.Spec.ISCSI = iscsiSource
	case driverType == dvp.OntapNASStorageDriverName:
		nfsSource = CreateNFSVolumeSource(vol.Config)
		pv.Spec.NFS = nfsSource
	default:
		// Unknown driver for the frontend plugin or for Kubernetes.
		// Provisioned volume should get deleted.
		log.WithFields(log.Fields{
			"volume": vol.Config.Name,
			"type":   p.orchestrator.GetVolumeType(vol),
			"driver": driverType,
		}).Warn("Kubernetes frontend does n't recognize this type of volume; ",
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
		&v1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("Kubernetes frontend failed to contact the "+
			"API server and delete the PV: %s", err.Error())
	}
	return err
}

// getClaimClass returns name of class that is requested by given claim.
// Request for `nil` class is interpreted as request for class "",
// i.e. for a classless PV.
func getClaimClass(claim *v1.PersistentVolumeClaim) string {
	// TODO: change to PersistentVolumeClaim.Spec.Class value when this
	// attribute is introduced.
	if class, found := claim.Annotations[AnnClass]; found {
		return class
	}
	return ""
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
		AnnProvisioner {
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
		}).Warn("Kubernetes frontend didn't recognize the notification event ",
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
			&v1.DeleteOptions{})
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
		}).Warn("Kubernetes frontend doesn't recognize the volume phase.")
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

func (p *KubernetesPlugin) addClass(obj interface{}) {
	class, ok := obj.(*k8s_storage.StorageClass)
	if !ok {
		log.Panicf("Kubernetes frontend expected StorageClass; handler got %v", obj)
	}
	p.processClass(class, "add")
}

func (p *KubernetesPlugin) updateClass(oldObj, newObj interface{}) {
	class, ok := newObj.(*k8s_storage.StorageClass)
	if !ok {
		log.Panicf("Kubernetes frontend expected StorageClass; handler got %v", newObj)
	}
	p.processClass(class, "update")
}

func (p *KubernetesPlugin) deleteClass(obj interface{}) {
	class, ok := obj.(*k8s_storage.StorageClass)
	if !ok {
		log.Panicf("Kubernetes frontend expected StorageClass; handler got %v", obj)
	}
	p.processClass(class, "delete")
}

func (p *KubernetesPlugin) processClass(
	class *k8s_storage.StorageClass,
	eventType string,
) {
	log.WithFields(log.Fields{
		"StorageClass":             class.Name,
		"StorageClass_provisioner": class.Provisioner,
		"StorageClass_parameters":  class.Parameters,
		"StorageClass_eventType":   eventType,
	}).Debug("Kubernetes frontend got notified of a StorageClass.")
	if class.Provisioner != AnnProvisioner {
		return
	}
	switch eventType {
	case "add":
		p.processAddedClass(class)
	case "delete":
		p.processDeletedClass(class)
	case "update":
		p.processUpdatedClass(class)
	default:
		log.WithFields(log.Fields{
			"StorageClass": class.Name,
			"event":        eventType,
		}).Warn("Kubernetes frontend didn't recognize the notification event corresponding to the StorageClass!")
		return
	}
}

func (p *KubernetesPlugin) processAddedClass(class *k8s_storage.StorageClass) {
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
					"StorageClass":             class.Name,
					"StorageClass_provisioner": class.Provisioner,
					"StorageClass_parameters":  class.Parameters,
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
				"StorageClass":             class.Name,
				"StorageClass_provisioner": class.Provisioner,
				"StorageClass_parameters":  class.Parameters,
			}).Error("Kubernetes frontend couldn't process the encoded StorageClass attribute: ", err)
			return
		}
		scConfig.Attributes[k] = req
	}

	// Add the storge class
	sc, err := p.orchestrator.AddStorageClass(scConfig)
	if err != nil {
		log.WithFields(log.Fields{
			"StorageClass":             class.Name,
			"StorageClass_provisioner": class.Provisioner,
			"StorageClass_parameters":  class.Parameters,
		}).Error("Kubernetes frontend couldn't add the StorageClass: ", err)
		return
	}
	if sc != nil {
		log.WithFields(log.Fields{
			"StorageClass":             class.Name,
			"StorageClass_provisioner": class.Provisioner,
			"StorageClass_parameters":  class.Parameters,
		}).Info("Kubernetes frontend successfully added the StorageClass.")
	}
	return
}

func (p *KubernetesPlugin) processDeletedClass(class *k8s_storage.StorageClass) {
	deleted, err := p.orchestrator.DeleteStorageClass(class.Name)
	if err != nil {
		log.WithFields(log.Fields{
			"StorageClass": class.Name,
		}).Error("Kubernetes frontend couldn't delete the StorageClass: ", err)
		return
	}
	if deleted {
		log.WithFields(log.Fields{
			"StorageClass": class.Name,
		}).Info("Kubernetes frontend successfully deleted the StorageClass.")
	}
	return
}

func (p *KubernetesPlugin) processUpdatedClass(class *k8s_storage.StorageClass) {

}
