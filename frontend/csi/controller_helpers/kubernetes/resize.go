// Copyright 2022 NetApp, Inc. All Rights Reserved.
package kubernetes

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend/csi"
	. "github.com/netapp/trident/logging"
)

/////////////////////////////////////////////////////////////////////////////
//
// This file contains the event handlers that resize CSI Trident PVCs/PVs.
//
/////////////////////////////////////////////////////////////////////////////

// updatePVCResize is the update handler for the PVC watcher whose job is to
// detect PVCs with increased capacity requests and resize the underlying PV
// and PVC to match the request.
func (h *helper) updatePVCResize(oldObj, newObj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S, WorkflowVolumeResize, LogLayerCSIFrontend)
	Logc(ctx).Trace(">>>> updatePVCResize")
	defer Logc(ctx).Trace("<<<< updatePVCResize")

	// Ensure we got PVC objects
	oldPVC, ok := oldObj.(*v1.PersistentVolumeClaim)
	if !ok {
		Logc(ctx).Errorf("K8S helper expected PVC; got %v", oldObj)
		return
	}
	newPVC, ok := newObj.(*v1.PersistentVolumeClaim)
	if !ok {
		Logc(ctx).Errorf("K8S helper expected PVC; got %v", newObj)
		return
	}

	oldPVCSize := oldPVC.Spec.Resources.Requests[v1.ResourceStorage]
	newPVCSize := newPVC.Spec.Resources.Requests[v1.ResourceStorage]
	currentSize := newPVC.Status.Capacity[v1.ResourceStorage]

	logFields := LogFields{
		"OldPVCSize":  oldPVCSize.String(),
		"NewPVCSize":  newPVCSize.String(),
		"CurrentSize": currentSize.String(),
	}
	Logc(ctx).WithFields(logFields).Trace("PVC Resize Info.")

	// Verify there is work to be done
	if newPVCSize.Cmp(oldPVCSize) == 0 && currentSize.Cmp(newPVCSize) == 0 {
		return
	}

	// Verify the PVC is Bound
	if newPVC.Status.Phase != v1.ClaimBound {
		return
	}

	// Verify the PVC is managed by Trident (include legacy volumes)
	pvcProvisioner := getPVCProvisioner(newPVC)
	if pvcProvisioner != csi.Provisioner && pvcProvisioner != csi.LegacyProvisioner {
		return
	}

	// Verify the storage class is available
	scName := getStorageClassForPVC(newPVC)
	if scName == "" {
		Logc(ctx).WithField("name", newPVC.Name).Warning("K8S helper found empty storage class for PVC.")
		return
	}
	sc, err := h.getCachedStorageClassByName(ctx, scName)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"name":         newPVC.Name,
			"storageClass": scName,
		}).Warning("K8S helper could not find storage class for PVC.")
		return
	}

	// Verify the storage class is managed by Trident (all SC's will have been upgraded to the new provisioner)
	if sc.Provisioner != csi.Provisioner {
		Logc(ctx).WithField("name", scName).Warningf("The storage class provisioner is not %s.", csi.Provisioner)
		return
	}

	// Verify the storage class allows resize
	if sc.AllowVolumeExpansion == nil || !(*sc.AllowVolumeExpansion) {
		message := "can't resize a PV whose storage class doesn't allow volume expansion."
		h.eventRecorder.Event(newPVC, v1.EventTypeWarning, "ResizeFailed", message)
		Logc(ctx).WithFields(LogFields{
			"PVC":          newPVC.Name,
			"storageClass": sc.Name,
		}).Debugf("K8S helper %s", message)
		return
	}

	// Verify the volume is being grown
	if newPVCSize.Cmp(oldPVCSize) < 0 || (newPVCSize.Cmp(oldPVCSize) == 0 && currentSize.Cmp(newPVCSize) > 0) {
		message := "can't shrink a PV."
		h.eventRecorder.Event(newPVC, v1.EventTypeWarning, "ResizeFailed", message)
		Logc(ctx).WithField("PVC", newPVC.Name).Warningf("K8S helper %s", message)
		return
	}

	// If we get this far, we potentially have a valid resize operation.
	Logc(ctx).WithFields(LogFields{
		"PVC":          newPVC.Name,
		"PVC_old_size": currentSize.String(),
		"PVC_new_size": newPVCSize.String(),
	}).Debug("K8S helper detected a PVC suited for volume resize.")

	// We intentionally don't set the PVC condition to "PersistentVolumeClaimResizing"
	// as NFS resize happens instantaneously. Note that updating the condition
	// should only happen when the condition isn't set. Otherwise, we may have
	// infinite calls to updatePVCResize. Currently, we don't keep track of
	// outstanding resize operations. This may need to change when resize takes
	// non-negligible amount of time.

	// Verify Trident knows about the volume
	volume, err := h.orchestrator.GetVolume(ctx, newPVC.Spec.VolumeName)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"PVC":   newPVC.Name,
			"PV":    newPVC.Spec.VolumeName,
			"error": err,
		}).Error("K8S helper couldn't find the backend volume for the PVC.")
		return
	}

	// We only allow resizing NFS PVs as it doesn't require a host-side component to resize the file system.
	if volume.Config.Protocol != tridentconfig.File {
		message := "can't resize a non-NFS PV."
		h.eventRecorder.Event(newPVC, v1.EventTypeWarning, "ResizeFailed", message)
		Logc(ctx).WithFields(LogFields{"PVC": newPVC.Name}).Errorf("K8S helper %s", message)
		return
	}

	// Get the PV from Kubernetes
	pv, err := h.getPVForPVC(ctx, newPVC)
	if err != nil || pv == nil {
		Logc(ctx).WithFields(LogFields{
			"PVC":   newPVC.Name,
			"PV":    newPVC.Spec.VolumeName,
			"error": err,
		}).Error("K8S helper couldn't retrieve the matching PV for the PVC.")
		return
	}

	// Resize the volume and PV
	if err = h.resizeVolumeAndPV(ctx, pv, newPVCSize); err != nil {
		message := fmt.Sprintf("failed in resizing the volume or PV: %v", err)
		h.eventRecorder.Event(newPVC, v1.EventTypeWarning, "ResizeFailed", message)
		Logc(ctx).WithFields(LogFields{"PVC": newPVC.Name}).Errorf("K8S helper %v", message)
		return
	}

	// Update the PVC
	updatedPVC, err := h.resizePVC(ctx, newPVC, newPVCSize)
	if err != nil {
		message := fmt.Sprintf("failed to update the PVC size: %v.", err)
		if updatedPVC == nil {
			h.eventRecorder.Event(newPVC, v1.EventTypeWarning, "ResizeFailed", message)
		} else {
			h.eventRecorder.Event(updatedPVC, v1.EventTypeWarning, "ResizeFailed", message)
		}
		Logc(ctx).WithFields(LogFields{"PVC": newPVC.Name}).Errorf("K8S helper %v", message)
		return
	}

	Logc(ctx).Tracef("Resize successful for PVC with name: %s .", newPVC.Name)
	h.eventRecorder.Event(updatedPVC, v1.EventTypeNormal, "ResizeSuccess", "resized the PV and volume.")
}

// resizeVolumeAndPV resizes the volume on the storage backend and updates the PV size.
func (h *helper) resizeVolumeAndPV(ctx context.Context, pv *v1.PersistentVolume, newSize resource.Quantity) error {
	Logc(ctx).WithField("newSize", newSize.String()).Trace(">>>> resizeVolumeAndPV")
	defer Logc(ctx).Trace("<<<< resizeVolumeAndPV")

	pvSize := pv.Spec.Capacity[v1.ResourceStorage]
	if pvSize.Cmp(newSize) < 0 {
		Logc(ctx).Tracef("Calling the orchestrator to resize the volume: %s on the storage backend.", pv.Name)
		if err := h.orchestrator.ResizeVolume(ctx, pv.Name, fmt.Sprintf("%d", newSize.Value())); err != nil {
			return err
		}
	} else if pvSize.Cmp(newSize) == 0 {
		Logc(ctx).Tracef("PV: %s already the requested size: %s , no need to do anything.", pv.Name,
			newSize.String())
		return nil
	} else {
		return fmt.Errorf("cannot shrink PV %q", pv.Name)
	}

	// Update the PV
	pvClone := pv.DeepCopy()
	pvClone.Spec.Capacity[v1.ResourceStorage] = newSize
	pvUpdated, err := h.patchPV(ctx, pv, pvClone)
	if err != nil {
		return err
	}
	updatedSize := pvUpdated.Spec.Capacity[v1.ResourceStorage]
	if updatedSize.Cmp(newSize) != 0 {
		return fmt.Errorf("PV capacity was not updated as expected")
	}

	Logc(ctx).WithFields(LogFields{
		"PV":          pv.Name,
		"PV_old_size": pvSize.String(),
		"PV_new_size": updatedSize.String(),
	}).Debug("K8S helper resized the PV.")

	return nil
}

// resizePVC updates the PVC size.
func (h *helper) resizePVC(
	ctx context.Context, pvc *v1.PersistentVolumeClaim, newSize resource.Quantity,
) (*v1.PersistentVolumeClaim, error) {
	Logc(ctx).WithFields(LogFields{
		"Name":    pvc.Name,
		"NewSize": newSize.String(),
	}).Trace(">>>> resizePVC")
	defer Logc(ctx).Trace("<<<< resizePVC")

	pvcClone := pvc.DeepCopy()
	pvcClone.Status.Capacity[v1.ResourceStorage] = newSize
	pvcUpdated, err := h.patchPVCStatus(ctx, pvc, pvcClone)
	if err != nil {
		return nil, err
	}
	updatedSize := pvcUpdated.Status.Capacity[v1.ResourceStorage]
	if updatedSize.Cmp(newSize) != 0 {
		return pvcUpdated, fmt.Errorf("PVC capacity was not updated as expected")
	}

	oldSize := pvc.Status.Capacity[v1.ResourceStorage]
	Logc(ctx).WithFields(LogFields{
		"PVC":          pvc.Name,
		"PVC_old_size": oldSize.String(),
		"PVC_new_size": updatedSize.String(),
	}).Info("K8S helper updated the PVC after resize.")

	return pvcUpdated, nil
}
