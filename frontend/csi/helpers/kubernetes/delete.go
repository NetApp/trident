// Copyright 2020 NetApp, Inc. All Rights Reserved.
package kubernetes

import (
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"

	"github.com/netapp/trident/frontend/csi"
	"github.com/netapp/trident/utils"
)

/////////////////////////////////////////////////////////////////////////////
//
// This file contains the event handlers that delete legacy Trident PVs.
//
/////////////////////////////////////////////////////////////////////////////

// updateLegacyPV handles deletion of Trident-created legacy (non-CSI) PVs.  This method specifically
// handles the case where the PV transitions to the Released or Failed state after a PVC is deleted.
func (p *Plugin) updateLegacyPV(oldObj, newObj interface{}) {
	ctx := utils.GenerateRequestContext(nil, "", utils.ContextSourceK8S)
	logc := utils.GetLogWithRequestContext(ctx)
	// Ensure we got PV objects
	pv, ok := newObj.(*v1.PersistentVolume)
	if !ok {
		logc.Errorf("K8S helper expected PV; got %v", newObj)
		return
	}

	// Ensure the legacy PV's phase is Released or Failed
	if pv.Status.Phase != v1.VolumeReleased && pv.Status.Phase != v1.VolumeFailed {
		return
	}

	// Ensure the legacy PV was provisioned by Trident
	if pv.ObjectMeta.Annotations[AnnDynamicallyProvisioned] != csi.LegacyProvisioner {
		return
	}

	// Ensure the legacy PV is not a no-manage volume
	if isPVNotManaged(ctx, pv) {
		return
	}

	// Ensure the legacy PV's reclaim policy is Delete
	if pv.Spec.PersistentVolumeReclaimPolicy != v1.PersistentVolumeReclaimDelete {
		return
	}

	// Delete the volume on the backend
	if err := p.orchestrator.DeleteVolume(ctx, pv.Name); err != nil && !utils.IsNotFoundError(err) {
		// Updating the PV's phase to "VolumeFailed", so that a storage admin can take action.
		message := fmt.Sprintf("failed to delete the volume for PV %s: %s. Will eventually retry, "+
			"but the volume and PV may need to be manually deleted.", pv.Name, err.Error())
		_, _ = p.updatePVPhaseWithEvent(ctx, pv, v1.VolumeFailed, v1.EventTypeWarning, "FailedVolumeDelete", message)
		logc.Errorf("K8S helper %s", message)
		// PV must be manually deleted by the admin after removing the volume.
		return
	}

	// Delete the PV
	err := p.kubeClient.CoreV1().PersistentVolumes().Delete(ctx, pv.Name, deleteOpts)
	if err != nil {
		if !strings.HasSuffix(err.Error(), "not found") {
			// PVs provisioned by external provisioners seem to end up in
			// the failed state as Kubernetes doesn't recognize them.
			logc.Errorf("K8S helper failed to delete a PV: %s", err.Error())
		}
		return
	}

	logc.WithField("PV", pv.Name).Info("K8S helper deleted a legacy PV and its backing volume.")
}
