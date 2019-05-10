// Copyright 2019 NetApp, Inc. All Rights Reserved.
package kubernetes

import (
	"encoding/json"
	"fmt"
	"strconv"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	k8sstoragev1 "k8s.io/api/storage/v1"
	k8sstoragev1beta "k8s.io/api/storage/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	commontypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"

	"github.com/netapp/trident/config"
	tridentutils "github.com/netapp/trident/utils"
)

// validateKubeVersion logs a warning if the detected Kubernetes version is outside the supported range.
func (p *Plugin) validateKubeVersion() error {

	// Parse Kubernetes version into a SemVer object for simple comparisons
	if version, err := tridentutils.ParseSemantic(p.kubeVersion.GitVersion); err != nil {
		return err
	} else if !version.AtLeast(tridentutils.MustParseSemantic(config.KubernetesCSIVersionMinOptional)) {
		log.Warnf("%s v%s may not support container orchestrator version %s.%s (%s)! Supported "+
			"Kubernetes versions are %s-%s. K8S helper frontend proceeds as if you are running Kubernetes %s!",
			config.OrchestratorName, config.OrchestratorVersion, p.kubeVersion.Major, p.kubeVersion.Minor,
			p.kubeVersion.GitVersion, config.KubernetesCSIVersionMinOptional, config.KubernetesVersionMax,
			config.KubernetesVersionMax)
	}
	return nil
}

// updatePVPhaseWithEvent saves new PV phase to API server and emits the
// given event on the PV. It saves the phase and emits the event only when
// the phase has actually changed from the version saved in API server.
// (Based on pkg/controller/volume/persistentvolume/pv_controller.go)
func (p *Plugin) updatePVPhaseWithEvent(
	pv *v1.PersistentVolume, phase v1.PersistentVolumePhase, eventType, reason, message string,
) (*v1.PersistentVolume, error) {

	if pv.Status.Phase == phase {
		// Nothing to do.
		return pv, nil
	}
	newPV, err := p.updatePVPhase(pv, phase, message)
	if err != nil {
		return nil, err
	}

	p.eventRecorder.Event(newPV, eventType, reason, message)

	return newPV, nil
}

// updatePVPhase saves new PV phase to API server.
// (Based on pkg/controller/volume/persistentvolume/pv_controller.go)
func (p *Plugin) updatePVPhase(
	pv *v1.PersistentVolume, phase v1.PersistentVolumePhase, message string,
) (*v1.PersistentVolume, error) {

	if pv.Status.Phase == phase {
		// Nothing to do.
		return pv, nil
	}

	pvClone := pv.DeepCopy()

	pvClone.Status.Phase = phase
	pvClone.Status.Message = message

	return p.kubeClient.CoreV1().PersistentVolumes().UpdateStatus(pvClone)
}

// patchPV patches a PV after an update.
func (p *Plugin) patchPV(oldPV *v1.PersistentVolume, newPV *v1.PersistentVolume) (*v1.PersistentVolume, error) {

	oldPVData, err := json.Marshal(oldPV)
	if err != nil {
		return nil, fmt.Errorf("error marshaling the PV %q: %v", oldPV.Name, err)
	}

	newPVData, err := json.Marshal(newPV)
	if err != nil {
		return nil, fmt.Errorf("error marshaling the PV %q: %v", newPV.Name, err)
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldPVData, newPVData, newPV)
	if err != nil {
		return nil, fmt.Errorf("error creating the two-way merge patch for PV %q: %v", newPV.Name, err)
	}

	return p.kubeClient.CoreV1().PersistentVolumes().Patch(newPV.Name, commontypes.StrategicMergePatchType, patchBytes)
}

// patchPVCStatus patches a PVC status after an update.
func (p *Plugin) patchPVCStatus(
	oldPVC *v1.PersistentVolumeClaim, newPVC *v1.PersistentVolumeClaim,
) (*v1.PersistentVolumeClaim, error) {

	oldPVCData, err := json.Marshal(oldPVC)
	if err != nil {
		return nil, fmt.Errorf("error marshaling the PVC %q: %v", oldPVC.Name, err)
	}

	newPVCData, err := json.Marshal(newPVC)
	if err != nil {
		return nil, fmt.Errorf("error marshaling the PVC %q: %v", newPVC.Name, err)
	}

	patchBytes, err := strategicpatch.CreateTwoWayMergePatch(oldPVCData, newPVCData, newPVC)
	if err != nil {
		return nil, fmt.Errorf("error creating the two-way merge patch for PVC %q: %v", newPVC.Name, err)
	}

	return p.kubeClient.CoreV1().PersistentVolumeClaims(newPVC.Namespace).Patch(newPVC.Name,
		commontypes.StrategicMergePatchType, patchBytes, "status")
}

// getPVForPVC returns the PV for a bound PVC.
func (p *Plugin) getPVForPVC(pvc *v1.PersistentVolumeClaim) (*v1.PersistentVolume, error) {
	if pvc.Status.Phase != v1.ClaimBound || pvc.Spec.VolumeName == "" {
		return nil, nil
	}
	return p.kubeClient.CoreV1().PersistentVolumes().Get(pvc.Spec.VolumeName, metav1.GetOptions{})
}

// getPVCForPV returns the PVC for a PV.
func (p *Plugin) getPVCForPV(pv *v1.PersistentVolume) (*v1.PersistentVolumeClaim, error) {
	if pv.Spec.ClaimRef == nil {
		return nil, nil
	}
	return p.kubeClient.CoreV1().PersistentVolumeClaims(
		pv.Spec.ClaimRef.Namespace).Get(pv.Spec.ClaimRef.Name, metav1.GetOptions{})
}

// isPVCNotManaged examines a PVC and determines whether the notManaged annotation is present.
func isPVCNotManaged(pvc *v1.PersistentVolumeClaim) bool {
	return isNotManaged(pvc.Annotations, pvc.Name, "PVC")
}

// isPVNotManaged examines a PV and determines whether the notManaged annotation is present.
func isPVNotManaged(pv *v1.PersistentVolume) bool {
	return isNotManaged(pv.Annotations, pv.Name, "PV")
}

// isNotManaged returns true if the notManaged annotation is present in the supplied map and
// has a value of anything that doesn't resolve to 'false'.
func isNotManaged(annotations map[string]string, name string, kind string) bool {
	if value, ok := annotations[AnnNotManaged]; ok {
		if notManaged, err := strconv.ParseBool(value); err != nil {
			log.WithField(kind, name).Errorf("%s annotation set with invalid value: %v", AnnNotManaged, err)
			return true
		} else if notManaged {
			log.WithField(kind, name).Debugf("K8S helper ignored this notManaged %s.", kind)
			return true
		}
	}
	return false
}

// convertStorageClassV1BetaToV1 accepts an older (beta) storage class and returns a v1 storage class
// populated with the fields needed by Trident.
func convertStorageClassV1BetaToV1(class *k8sstoragev1beta.StorageClass) *k8sstoragev1.StorageClass {
	// For now we just copy the fields used by Trident.
	v1Class := &k8sstoragev1.StorageClass{
		Provisioner: class.Provisioner,
		Parameters:  class.Parameters,
	}
	v1Class.Name = class.Name
	return v1Class
}

// getStorageClassForPVC returns StorageClassName from a PVC. If no storage class was requested, it returns "".
func getStorageClassForPVC(pvc *v1.PersistentVolumeClaim) string {
	// Use beta annotation first
	if sc, found := pvc.Annotations[AnnClass]; found {
		return sc
	} else if pvc.Spec.StorageClassName != nil {
		return *pvc.Spec.StorageClassName
	}
	return ""
}

// getPVCProvisioner returns the provisioner for a PVC.
func getPVCProvisioner(pvc *v1.PersistentVolumeClaim) string {
	if provisioner, found := pvc.Annotations[AnnStorageProvisioner]; found {
		return provisioner
	}
	return ""
}

func (p *Plugin) checkValidStorageClassReceived(claim *v1.PersistentVolumeClaim) error {

	// Filter unrelated claims
	if claim.Spec.StorageClassName == nil || *claim.Spec.StorageClassName == "" {
		log.WithFields(log.Fields{
			"PVC": claim.Name,
		}).Error("PVC has no storage class specified")
		return fmt.Errorf("PVC %s has no storage class specified", claim.Name)
	}

	return nil
}
