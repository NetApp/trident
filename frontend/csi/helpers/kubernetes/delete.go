package kubernetes

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend/csi"
)

/////////////////////////////////////////////////////////////////////////////
//
// This file contains the event handlers that delete legacy Trident PVs.
//
/////////////////////////////////////////////////////////////////////////////

// updateLegacyPV handles deletion of Trident-created legacy (non-CSI) PVs.  This method specifically
// handles the case where the PV transitions to the Released or Failed state after a PVC is deleted.
func (p *Plugin) updateLegacyPV(oldObj, newObj interface{}) {

	// Ensure we got PV objects
	pv, ok := newObj.(*v1.PersistentVolume)
	if !ok {
		log.Errorf("K8S helper expected PV; got %v", oldObj)
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
	if isPVNotManaged(pv) {
		return
	}

	// Ensure the legacy PV's reclaim policy is Delete
	if pv.Spec.PersistentVolumeReclaimPolicy != v1.PersistentVolumeReclaimDelete {
		return
	}

	// Delete the volume on the backend
	if err := p.orchestrator.DeleteVolume(pv.Name); err != nil && !core.IsNotFoundError(err) {
		// Updating the PV's phase to "VolumeFailed", so that a storage admin can take action.
		message := fmt.Sprintf("failed to delete the volume for PV %s: %s. Will eventually retry, "+
			"but the volume and PV may need to be manually deleted.", pv.Name, err.Error())
		_, _ = p.updatePVPhaseWithEvent(pv, v1.VolumeFailed, v1.EventTypeWarning, "FailedVolumeDelete", message)
		log.Errorf("K8S helper %s", message)
		// PV must be manually deleted by the admin after removing the volume.
		return
	}

	// Delete the PV
	if err := p.kubeClient.CoreV1().PersistentVolumes().Delete(pv.Name, &metav1.DeleteOptions{}); err != nil {
		if !strings.HasSuffix(err.Error(), "not found") {
			// PVs provisioned by external provisioners seem to end up in
			// the failed state as Kubernetes doesn't recognize them.
			log.Errorf("K8S helper failed to delete a PV: %s", err.Error())
		}
		return
	}

	log.WithField("PV", pv.Name).Info("K8S helper deleted a legacy PV and its backing volume.")
}
