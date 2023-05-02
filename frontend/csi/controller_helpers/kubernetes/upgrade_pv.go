// Copyright 2022 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/frontend/csi"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
)

// ///////////////////////////////////////////////////////////////////////////
//
// This file contains the code to convert NFS/iSCSI PVs to CSI PVs.
//
// ///////////////////////////////////////////////////////////////////////////

func (h *helper) UpgradeVolume(
	ctx context.Context, request *storage.UpgradeVolumeRequest,
) (*storage.VolumeExternal, error) {
	Logc(ctx).WithFields(LogFields{
		"volume": request.Volume,
		"type":   request.Type,
	}).Info("PV upgrade: workflow started.")

	// Check volume exists in Trident
	volume, err := h.orchestrator.GetVolume(ctx, request.Volume)
	if err != nil {
		message := "PV upgrade: could not find the volume to upgrade"
		Logc(ctx).WithFields(LogFields{
			"Volume": request.Volume,
			"error":  err,
		}).Errorf("%s.", message)
		return nil, fmt.Errorf("%s: %v", message, err)
	}
	Logc(ctx).WithFields(LogFields{
		"volume": volume.Config.Name,
		"type":   request.Type,
	}).Infof("PV upgrade: volume found.")

	// Check volume state is online
	if volume.State != storage.VolumeStateOnline {
		message := "PV upgrade: Trident volume to be upgraded must be in online state"
		Logc(ctx).WithFields(LogFields{
			"Volume": volume.Config.Name,
			"State":  volume.State,
		}).Errorf("%s.", message)
		return nil, fmt.Errorf("%s", message)
	}
	Logc(ctx).WithField("volume", volume.Config.Name).Debug("PV upgrade: Trident volume is online.")

	// Set the volume to upgrading state to prevent other operations from running against it
	if err = h.orchestrator.SetVolumeState(ctx, volume.Config.Name, storage.VolumeStateUpgrading); err != nil {
		msg := fmt.Sprintf("PV upgrade: error setting volume to upgrading state; %v", err)
		Logc(ctx).WithField("volume", volume.Config.Name).Error(msg)
		return nil, fmt.Errorf(msg)
	}
	Logc(ctx).WithField("volume", volume.Config.Name).Debug("PV upgrade: Trident volume set to upgrading.")
	defer func() {
		if err = h.orchestrator.SetVolumeState(ctx, volume.Config.Name, storage.VolumeStateOnline); err != nil {
			Logc(ctx).WithField(
				"volume", volume.Config.Name,
			).Errorf("PV upgrade: error setting volume to online state; %v", err)
			return
		}
		Logc(ctx).WithField("volume", volume.Config.Name).Debug("PV upgrade: Trident volume set to online.")
	}()

	// Get PV
	pv, err := h.getCachedPVByName(ctx, request.Volume)
	if err != nil {
		message := "PV upgrade: could not find the PV to upgrade"
		Logc(ctx).WithFields(LogFields{
			"PV":    request.Volume,
			"error": err,
		}).Errorf("%s.", message)
		return nil, fmt.Errorf("%s: %v", message, err)
	}
	Logc(ctx).WithField("PV", pv.Name).Debug("PV upgrade: PV found in cache.")

	// Check volume type is iSCSI or NFS
	if pv.Spec.NFS == nil && pv.Spec.ISCSI == nil {
		message := "PV to be upgraded must be of type NFS or iSCSI"
		Logc(ctx).WithField("PV", pv.Name).Errorf("%s.", message)
		return nil, fmt.Errorf("%s", message)
	} else if pv.Spec.NFS != nil {
		Logc(ctx).WithField("PV", pv.Name).Debug("PV upgrade: volume is NFS.")
	} else if pv.Spec.ISCSI != nil {
		Logc(ctx).WithField("PV", pv.Name).Debug("PV upgrade: volume is iSCSI.")
	}

	// Check PV is bound to a PVC
	if pv.Status.Phase != v1.VolumeBound {
		message := "PV upgrade: PV must be bound to a PVC"
		Logc(ctx).WithField("PV", pv.Name).Errorf("%s.", message)
		return nil, fmt.Errorf("%s", message)
	}
	Logc(ctx).WithField("PV", pv.Name).Debug("PV upgrade: PV state is Bound.")

	// Ensure the legacy PV was provisioned by Trident
	if pv.ObjectMeta.Annotations[AnnDynamicallyProvisioned] != csi.LegacyProvisioner {
		message := "PV upgrade: PV must have been provisioned by Trident"
		Logc(ctx).WithFields(LogFields{
			"PV":          pv.Name,
			"provisioner": csi.LegacyProvisioner,
		}).Errorf("%s.", message)
		return nil, fmt.Errorf("%s", message)
	}
	Logc(ctx).WithFields(LogFields{
		"PV":          pv.Name,
		"provisioner": csi.LegacyProvisioner,
	}).Debug("PV upgrade: PV was provisioned by Trident.")

	namespace := pv.Spec.ClaimRef.Namespace
	pvcDisplayName := namespace + "/" + pv.Spec.ClaimRef.Name

	// Get PVC
	pvc, err := h.getCachedPVCByName(ctx, pv.Spec.ClaimRef.Name, pv.Spec.ClaimRef.Namespace)
	if err != nil {
		message := "PV upgrade: could not find the PVC bound to the PV"
		Logc(ctx).WithFields(LogFields{
			"PV":    pv.Name,
			"PVC":   pvcDisplayName,
			"error": err,
		}).Errorf("%s.", message)
		return nil, fmt.Errorf("%s: %v", message, err)
	}
	Logc(ctx).WithFields(LogFields{
		"PV":  pv.Name,
		"PVC": pvcDisplayName,
	}).Debug("PV upgrade: PVC found in cache.")

	// Trigger failure based on PVC name for testing rollback
	if pvc.Name == "failure-7cef2e1a-b3d6-438a-952c-e94e006e687e" {
		return nil, fmt.Errorf("PV upgrade: pre-transaction error triggered by PVC name " +
			"failure-7cef2e1a-b3d6-438a-952c-e94e006e687e")
	}

	// Ensure no naked pods have PV mounted.  Owned pods will be deleted later in the workflow.
	ownedPodsForPVC, nakedPodsForPVC, err := h.getPodsForPVC(ctx, pvc)
	if err != nil {
		message := "PV upgrade: could not check for pods using the PV"
		Logc(ctx).WithFields(LogFields{
			"PV":    pv.Name,
			"PVC":   pvcDisplayName,
			"error": err,
		}).Errorf("%s.", message)
		return nil, fmt.Errorf("%s: %v", message, err)
	} else if len(nakedPodsForPVC) > 0 {
		message := fmt.Sprintf("PV upgrade: one or more naked pods are using the PV (%s); "+
			"shut down these pods manually and try again", strings.Join(nakedPodsForPVC, ","))
		Logc(ctx).WithFields(LogFields{
			"PV":  pv.Name,
			"PVC": pvcDisplayName,
		}).Errorf("%s.", message)
		return nil, fmt.Errorf("%s", message)
	} else if len(ownedPodsForPVC) > 0 {
		Logc(ctx).WithFields(LogFields{
			"PV":   pv.Name,
			"PVC":  pvcDisplayName,
			"pods": strings.Join(ownedPodsForPVC, ","),
		}).Info("PV upgrade: one or more owned pods are using the PV.")
	} else {
		Logc(ctx).WithFields(LogFields{
			"PV":  pv.Name,
			"PVC": pvcDisplayName,
		}).Info("PV upgrade: no owned pods are using the PV.")
	}

	// Check that PV has at most one finalizer, which must be kubernetes.io/pv-protection
	if pv.Finalizers != nil && len(pv.Finalizers) > 0 {
		if pv.Finalizers[0] != FinalizerPVProtection || len(pv.Finalizers) > 1 {
			message := "PV upgrade: PV has a finalizer other than kubernetes.io/pv-protection"
			Logc(ctx).WithField("PV", pv.Name).Errorf("%s.", message)
			return nil, fmt.Errorf("%s", message)
		}
	}

	upgradeConfig := &storage.PVUpgradeConfig{
		PVConfig:        pv,
		PVCConfig:       pvc,
		OwnedPodsForPVC: ownedPodsForPVC,
	}

	volTxn := &storage.VolumeTransaction{
		Config:          volume.Config,
		PVUpgradeConfig: upgradeConfig,
		Op:              storage.UpgradeVolume,
	}
	txnErr := h.orchestrator.AddVolumeTransaction(ctx, volTxn)
	if utils.IsFoundError(txnErr) {
		oldTxn, getErr := h.orchestrator.GetVolumeTransaction(ctx, volTxn)
		if getErr != nil {
			return nil, fmt.Errorf("PV upgrade: error gathering old upgrade transaction; %v", getErr)
		}
		if oldTxn == nil {
			return nil, fmt.Errorf("PV upgrade: volume transaction was not found")
		}
		if oldTxn.Op == storage.UpgradeVolume {
			if cleanupErr := h.rollBackPVUpgrade(ctx, oldTxn); cleanupErr != nil {
				return nil, fmt.Errorf("PV upgrade: error rolling back previous upgrade attempt; %v", cleanupErr)
			}
			txnErr = h.orchestrator.AddVolumeTransaction(ctx, volTxn)
		}
	}
	if txnErr != nil {
		message := fmt.Sprintf("PV upgrade: error saving volume transaction; %v", txnErr)
		Logc(ctx).WithFields(LogFields{
			"VolConfig":       volTxn.Config,
			"PVConfig":        volTxn.PVUpgradeConfig.PVConfig,
			"PVCConfig":       volTxn.PVUpgradeConfig.PVCConfig,
			"OwnedPodsForPVC": volTxn.PVUpgradeConfig.OwnedPodsForPVC,
			"Op":              volTxn.Op,
		}).Errorf(message)
		return nil, fmt.Errorf(message)
	}

	// Trigger failure based on PVC name for testing rollback
	if pvc.Name == "failure-3d25ba67-8f75-491a-96e2-78417aba1494" {
		return nil, fmt.Errorf("PV upgrade: post-transaction pre-cleanup error triggered by PVC name " +
			"failure-3d25ba67-8f75-491a-96e2-78417aba1494")
	}

	// Defer error handling
	defer func() {
		upgradeTxn, getErr := h.orchestrator.GetVolumeTransaction(ctx, volTxn)
		if getErr != nil {
			Logc(ctx).Errorf("PV upgrade: error verifying upgrade completed; %v", getErr)
		}
		if upgradeTxn != nil && upgradeTxn.Op == storage.UpgradeVolume {
			Logc(ctx).Warning("PV upgrade: error during upgrade; rolling back changes.")
			if cleanupErr := h.rollBackPVUpgrade(ctx, upgradeTxn); cleanupErr != nil {
				Logc(ctx).Errorf("PV upgrade: error during rollback; %v", cleanupErr)
			}
		}
	}()

	// Trigger failure based on PVC name for testing rollback
	if pvc.Name == "failure-e57c5f01-87c1-46d7-a09b-366527d31599" {
		return nil, fmt.Errorf("PV upgrade: post-transaction error triggered by PVC name " +
			"failure-e57c5f01-87c1-46d7-a09b-366527d31599")
	}

	// Delete the PV along with any finalizers
	if err = h.deletePVForUpgrade(ctx, pv); err != nil {
		message := "PV upgrade: could not delete the PV"
		Logc(ctx).WithFields(LogFields{
			"PV":    pv.Name,
			"error": err,
		}).Errorf("%s.", message)
		return nil, fmt.Errorf("%s: %v", message, err)
	}
	Logc(ctx).WithField("PV", pv.Name).Info("PV upgrade: PV deleted.")

	// Wait for PVC to become Lost or Pending
	lostOrPending := []v1.PersistentVolumeClaimPhase{v1.ClaimLost, v1.ClaimPending}
	lostPVC, err := h.waitForPVCPhase(ctx, pvc, lostOrPending, PVDeleteWaitPeriod)
	if err != nil {
		message := "PV upgrade: PVC did not reach the Lost or Pending state"
		Logc(ctx).WithFields(LogFields{
			"PV":    pv.Name,
			"PVC":   pvcDisplayName,
			"error": err,
		}).Errorf("%s.", message)
		return nil, fmt.Errorf("%s: %v", message, err)
	}
	Logc(ctx).WithFields(LogFields{
		"PV":  pv.Name,
		"PVC": pvcDisplayName,
	}).Info("PV upgrade: PVC reached the Lost or Pending state.")

	// Trigger failure based on PVC name for testing rollback
	if pvc.Name == "failure-65ca482e-d013-454f-bb4f-5c340fb835ac" {
		return nil, fmt.Errorf("PV upgrade: post-pv delete error triggered by PVC name " +
			"failure-65ca482e-d013-454f-bb4f-5c340fb835ac")
	}

	// Delete all owned pods that were using the PV
	for _, podName := range ownedPodsForPVC {
		// Delete pod
		if err = h.kubeClient.CoreV1().Pods(namespace).Delete(ctx, podName, deleteOpts); err != nil {
			message := "PV upgrade: could not delete a pod using the PV"
			Logc(ctx).WithFields(LogFields{
				"PV":    pv.Name,
				"PVC":   pvcDisplayName,
				"pod":   podName,
				"error": err,
			}).Errorf("%s.", message)
			return nil, fmt.Errorf("%s: %v", message, err)
		} else {
			Logc(ctx).WithFields(LogFields{
				"PV":  pv.Name,
				"PVC": pvcDisplayName,
				"pod": podName,
			}).Info("PV upgrade: Owned pod deleted.")
		}
	}

	// Wait for all deleted pods to disappear (or reappear in a non-Running state)
	for _, podName := range ownedPodsForPVC {
		// Wait for pod to disappear or become pending
		if _, err = h.waitForDeletedOrNonRunningPod(ctx, podName, namespace, PodDeleteWaitPeriod); err != nil {
			message := "PV upgrade: unexpected pod status"
			Logc(ctx).WithFields(LogFields{
				"PV":    pv.Name,
				"PVC":   pvcDisplayName,
				"pod":   podName,
				"error": err,
			}).Errorf("%s.", message)
			return nil, fmt.Errorf("%s: %v", message, err)
		} else {
			Logc(ctx).WithFields(LogFields{
				"PV":  pv.Name,
				"PVC": pvcDisplayName,
				"pod": podName,
			}).Info("PV upgrade: Pod deleted or non-Running.")
		}
	}

	// Trigger failure based on PVC name for testing rollback
	if pvc.Name == "failure-f900bd0f-fd81-453d-97bc-03148e8a4178" {
		return nil, fmt.Errorf("PV upgrade: post-pod delete error triggered by PVC name " +
			"failure-f900bd0f-fd81-453d-97bc-03148e8a4178")
	}

	// TODO: Do controller stuff (igroups, etc.) (?)

	// Remove bind-completed annotation from PVC
	unboundLostPVC, err := h.removePVCBindCompletedAnnotation(ctx, lostPVC)
	if err != nil {
		message := "PV upgrade: could not remove bind-completed annotation from PVC"
		Logc(ctx).WithFields(LogFields{
			"PVC":   pvcDisplayName,
			"error": err,
		}).Errorf("%s.", message)
		return nil, fmt.Errorf("%s: %v", message, err)
	}
	Logc(ctx).WithField("PVC", pvc.Name).Info("PV upgrade: removed bind-completed annotation from PVC.")

	// Trigger failure based on PVC name for testing rollback
	if pvc.Name == "failure-e41d3d81-1771-47b4-bec3-2a949d0049bd" {
		return nil, fmt.Errorf("PV upgrade: post-pvc update error triggered by PVC name " +
			"failure-e41d3d81-1771-47b4-bec3-2a949d0049bd")
	}

	// Create new PV
	csiPV, err := h.createCSIPVFromPV(ctx, pv, volume)
	if err != nil {
		message := "PV upgrade: could not create the CSI version of PV being upgraded"
		Logc(ctx).WithFields(LogFields{
			"PV":    pv.Name,
			"error": err,
		}).Errorf("PV upgrade: %s.", message)
		return nil, fmt.Errorf("%s: %v", message, err)
	}
	Logc(ctx).WithField("PV", csiPV.Name).Info("PV upgrade: created CSI version of PV.")

	// Wait for PVC to become Bound
	bound := []v1.PersistentVolumeClaimPhase{v1.ClaimBound}
	boundPVC, err := h.waitForPVCPhase(ctx, unboundLostPVC, bound, PVDeleteWaitPeriod)
	if err != nil {
		message := "PV upgrade: PVC did not reach the Bound state"
		Logc(ctx).WithFields(LogFields{
			"PV":    pv.Name,
			"PVC":   pvcDisplayName,
			"error": err,
		}).Errorf("%s.", message)
		return nil, fmt.Errorf("%s: %v", message, err)
	} else if boundPVC != nil {
		Logc(ctx).WithFields(LogFields{
			"PV":  csiPV.Name,
			"PVC": pvcDisplayName,
		}).Info("PV upgrade: PVC bound.")
	}

	// Trigger failure based on PVC name for testing rollback
	if pvc.Name == "failure-f26f23e5-4895-41cc-b983-6fe99d105841" {
		return nil, fmt.Errorf("PV upgrade: post-csi pv create error triggered by PVC name " +
			"failure-f26f23e5-4895-41cc-b983-6fe99d105841")
	}

	if err = h.orchestrator.DeleteVolumeTransaction(ctx, volTxn); err != nil {
		return nil, fmt.Errorf("PV upgrade: unable to delete upgrade transaction; %v", err)
	}

	// Return volume to caller
	return volume, nil
}

func (h *helper) rollBackPVUpgrade(ctx context.Context, volTxn *storage.VolumeTransaction) error {
	// Check volume exists in Trident
	volume, err := h.orchestrator.GetVolume(ctx, volTxn.Config.Name)
	if err != nil {
		message := "PV rollback: could not find the volume to roll back"
		Logc(ctx).WithFields(LogFields{
			"Volume": volTxn.Config.Name,
			"error":  err,
		}).Errorf("%s.", message)
		return fmt.Errorf("%s: %v", message, err)
	}
	Logc(ctx).WithFields(LogFields{
		"volume": volume.Config.Name,
	}).Info("PV rollback: volume found.")

	// Check volume state is upgrading
	if volume.State != storage.VolumeStateUpgrading {
		message := "PV rollback: Trident volume to be rolled back must be in upgrading state"
		Logc(ctx).WithFields(LogFields{
			"Volume": volume.Config.Name,
			"State":  string(volume.State),
		}).Errorf("%s.", message)
		return fmt.Errorf("%s", message)
	}
	Logc(ctx).WithField("volume", volume.Config.Name).Debug("PV rollback: Trident volume is upgrading.")

	namespace := volTxn.PVUpgradeConfig.PVCConfig.Namespace
	pvcDisplayName := namespace + "/" + volTxn.PVUpgradeConfig.PVCConfig.Name

	// Get PVC
	pvc, err := h.getCachedPVCByName(ctx, volTxn.PVUpgradeConfig.PVCConfig.Name, namespace)
	if err != nil {
		message := "PV rollback: could not find the PVC bound to the PV"
		Logc(ctx).WithFields(LogFields{
			"PV":    volTxn.PVUpgradeConfig.PVConfig.Name,
			"PVC":   pvcDisplayName,
			"error": err,
		}).Errorf("%s.", message)
		return fmt.Errorf("%s: %v", message, err)
	}
	Logc(ctx).WithFields(LogFields{
		"PV":  volTxn.PVUpgradeConfig.PVConfig.Name,
		"PVC": pvcDisplayName,
	}).Debug("PV rollback: PVC found in cache.")

	// Does PV exist?
	pv := &v1.PersistentVolume{}
	pvExists, err := h.isPVInCache(ctx, volTxn.Config.Name)
	if err != nil {
		message := "PV rollback: could not check cache for PV to roll back"
		Logc(ctx).WithFields(LogFields{
			"PV":    volTxn.Config.Name,
			"error": err,
		}).Errorf("%s.", message)
		return fmt.Errorf("%s: %v", message, err)
	}

	if pvExists {
		// If the PV exists, delete it. This could be new or old PV so load cached one for most accurate info
		pv, err = h.getCachedPVByName(ctx, volTxn.Config.Name)
		if err != nil {
			message := "PV rollback: could not find the PV to roll back"
			Logc(ctx).WithFields(LogFields{
				"PV":    volTxn.Config.Name,
				"error": err,
			}).Errorf("%s.", message)
			return fmt.Errorf("%s: %v", message, err)
		}
		Logc(ctx).WithField("PV", pv.Name).Debug("PV rollback: PV found in cache.")
		// Check that PV has at most one finalizer, which must be kubernetes.io/pv-protection
		if pv.Finalizers != nil && len(pv.Finalizers) > 0 {
			if pv.Finalizers[0] != FinalizerPVProtection || len(pv.Finalizers) > 1 {
				message := "PV rollback: PV has a finalizer other than kubernetes.io/pv-protection"
				Logc(ctx).WithField("PV", pv.Name).Errorf("%s.", message)
				return fmt.Errorf("%s", message)
			}
		}
		// Delete the PV along with any finalizers
		if err = h.deletePVForUpgrade(ctx, pv); err != nil {
			message := "PV rollback: could not delete the PV"
			Logc(ctx).WithFields(LogFields{
				"PV":    pv.Name,
				"error": err,
			}).Errorf("%s.", message)
			return fmt.Errorf("%s: %v", message, err)
		}
		Logc(ctx).WithField("PV", pv.Name).Info("PV rollback: PV deleted.")

		// Wait for PVC to become Lost
		lostOrPending := []v1.PersistentVolumeClaimPhase{v1.ClaimLost, v1.ClaimPending}
		pvc, err = h.waitForPVCPhase(ctx, pvc, lostOrPending, PVDeleteWaitPeriod)
		if err != nil {
			message := "PV rollback: PVC did not reach the Lost or Pending state"
			Logc(ctx).WithFields(LogFields{
				"PV":    pv.Name,
				"PVC":   pvcDisplayName,
				"error": err,
			}).Errorf("%s.", message)
			return fmt.Errorf("%s: %v", message, err)
		}
		Logc(ctx).WithFields(LogFields{
			"PV":  pv.Name,
			"PVC": pvcDisplayName,
		}).Info("PV rollback: PVC reached the Lost or Pending state.")
	}

	// Load the original PV config
	pv = volTxn.PVUpgradeConfig.PVConfig

	// Try to delete all owned pods that were using the PV
	for _, podName := range volTxn.PVUpgradeConfig.OwnedPodsForPVC {
		// Delete pod
		if err = h.kubeClient.CoreV1().Pods(namespace).Delete(ctx, podName, deleteOpts); err != nil {
			message := "PV rollback: could not delete a pod using the PV"
			Logc(ctx).WithFields(LogFields{
				"PV":    pv.Name,
				"PVC":   pvcDisplayName,
				"pod":   podName,
				"error": err,
			}).Warnf("%s.", message)
		} else {
			Logc(ctx).WithFields(LogFields{
				"PV":  pv.Name,
				"PVC": pvcDisplayName,
				"pod": podName,
			}).Info("PV rollback: Owned pod deleted.")
		}
	}

	// Wait for all deleted pods to disappear (or reappear in a non-Running state)
	for _, podName := range volTxn.PVUpgradeConfig.OwnedPodsForPVC {
		// Wait for pod to disappear or become pending
		if _, err = h.waitForDeletedOrNonRunningPod(ctx, podName, namespace, PodDeleteWaitPeriod); err != nil {
			message := "PV rollback: unexpected pod status"
			Logc(ctx).WithFields(LogFields{
				"PV":    pv.Name,
				"PVC":   pvcDisplayName,
				"pod":   podName,
				"error": err,
			}).Errorf("%s.", message)
			return fmt.Errorf("%s: %v", message, err)
		} else {
			Logc(ctx).WithFields(LogFields{
				"PV":  pv.Name,
				"PVC": pvcDisplayName,
				"pod": podName,
			}).Info("PV rollback: Pod deleted or non-Running.")
		}
	}
	// TODO: Do controller stuff (igroups, etc.) (?)

	// Remove bind-completed annotation from PVC
	pvc, err = h.removePVCBindCompletedAnnotation(ctx, pvc)
	if err != nil {
		message := "PV rollback: could not remove bind-completed annotation from PVC"
		Logc(ctx).WithFields(LogFields{
			"PVC":   pvcDisplayName,
			"error": err,
		}).Errorf("%s.", message)
		return fmt.Errorf("%s: %v", message, err)
	}
	Logc(ctx).WithField("PVC", pvc.Name).Info("PV rollback: removed bind-completed annotation from PVC.")

	// Create old PV
	pv.ResourceVersion = ""
	pv.UID = ""
	oldPV, err := h.kubeClient.CoreV1().PersistentVolumes().Create(ctx, pv, createOpts)
	if err != nil {
		message := "PV rollback: could not create the original version of PV being upgraded"
		Logc(ctx).WithFields(LogFields{
			"PV":    pv.Name,
			"error": err,
		}).Errorf("PV upgrade: %s.", message)
		return fmt.Errorf("%s: %v", message, err)
	}
	Logc(ctx).WithField("PV", oldPV.Name).Info("PV rollback: created original version of PV.")

	// Wait for PVC to become Bound
	bound := []v1.PersistentVolumeClaimPhase{v1.ClaimBound}
	boundPVC, err := h.waitForPVCPhase(ctx, pvc, bound, PVDeleteWaitPeriod)
	if err != nil {
		message := "PV rollback: PVC did not reach the Bound state"
		Logc(ctx).WithFields(LogFields{
			"PV":    oldPV.Name,
			"PVC":   pvcDisplayName,
			"error": err,
		}).Errorf("%s.", message)
		return fmt.Errorf("%s: %v", message, err)
	} else if boundPVC != nil {
		Logc(ctx).WithFields(LogFields{
			"PV":  oldPV.Name,
			"PVC": pvcDisplayName,
		}).Info("PV rollback: PVC bound.")
	}

	// Set volume state to online
	if err = h.orchestrator.SetVolumeState(ctx, volume.Config.Name, storage.VolumeStateOnline); err != nil {
		msg := fmt.Sprintf("PV rollback: error setting volume to online state; %v", err)
		Logc(ctx).WithField("volume", volume.Config.Name).Errorf(msg)
		return fmt.Errorf(msg)
	}

	// Delete the transaction
	if err = h.orchestrator.DeleteVolumeTransaction(ctx, volTxn); err != nil {
		return fmt.Errorf("PV rollback: unable to delete upgrade transaction; %v", err)
	}
	// Return to caller
	return nil
}

func (h *helper) deletePVForUpgrade(ctx context.Context, pv *v1.PersistentVolume) error {
	// Check if PV has finalizers
	hasFinalizers := pv.Finalizers != nil && len(pv.Finalizers) > 0

	// Delete PV if it doesn't have a deletion timestamp
	if pv.DeletionTimestamp == nil {

		// PV hasn't been deleted yet, so send the delete
		if err := h.kubeClient.CoreV1().PersistentVolumes().Delete(ctx, pv.Name, deleteOpts); err != nil {
			return err
		}

		// Wait for disappearance (unlikely) or a deletion timestamp (PV pinned by finalizer)
		if deletedPV, err := h.waitForDeletedPV(ctx, pv.Name, PVDeleteWaitPeriod); err != nil {
			return err
		} else if deletedPV != nil {
			if _, err := h.removePVFinalizers(ctx, deletedPV); err != nil {
				return err
			}
		}
	} else {
		// PV was deleted previously, so just remove any finalizer so it can be fully deleted
		if hasFinalizers {
			if _, err := h.removePVFinalizers(ctx, pv); err != nil {
				return err
			}
		}
	}

	// Wait for PV to have deletion timestamp
	if err := h.waitForPVDisappearance(ctx, pv.Name, PVDeleteWaitPeriod); err != nil {
		return err
	}

	return nil
}

// waitForDeletedPV waits for a PV to be deleted.  The function can return multiple combinations:
//
//	(nil, nil)   --> the PV disappeared from the cache
//	(PV, nil)    --> the PV's deletedTimestamp is set (it may have finalizers set)
//	(nil, error) --> an error occurred checking for the PV in the cache
//	(PV, error)  --> the PV was not deleted before the retry loop timed out
func (h *helper) waitForDeletedPV(
	ctx context.Context, name string, maxElapsedTime time.Duration,
) (*v1.PersistentVolume, error) {
	var pv *v1.PersistentVolume
	var ok bool

	checkForDeletedPV := func() error {
		pv = nil
		if item, exists, err := h.pvIndexer.GetByKey(name); err != nil {
			return err
		} else if !exists {
			return nil
		} else if pv, ok = item.(*v1.PersistentVolume); !ok {
			return fmt.Errorf("non-PV object %s found in cache", name)
		} else if pv.DeletionTimestamp == nil {
			return fmt.Errorf("PV %s deletion timestamp not set", name)
		}
		return nil
	}
	pvNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"pv":        name,
			"increment": duration,
		}).Debug("PV not yet deleted, waiting.")
	}
	pvBackoff := backoff.NewExponentialBackOff()
	pvBackoff.InitialInterval = CacheBackoffInitialInterval
	pvBackoff.RandomizationFactor = CacheBackoffRandomizationFactor
	pvBackoff.Multiplier = CacheBackoffMultiplier
	pvBackoff.MaxInterval = CacheBackoffMaxInterval
	pvBackoff.MaxElapsedTime = maxElapsedTime

	if err := backoff.RetryNotify(checkForDeletedPV, pvBackoff, pvNotify); err != nil {
		return nil, fmt.Errorf("PV %s was not deleted after %3.2f seconds", name, maxElapsedTime.Seconds())
	}

	return pv, nil
}

// waitForPVDisappearance waits for a PV to be fully deleted and gone from the cache.
func (h *helper) waitForPVDisappearance(ctx context.Context, name string, maxElapsedTime time.Duration) error {
	checkForDeletedPV := func() error {
		if item, exists, err := h.pvIndexer.GetByKey(name); err != nil {
			return err
		} else if !exists {
			return nil
		} else if _, ok := item.(*v1.PersistentVolume); !ok {
			return fmt.Errorf("non-PV object %s found in cache", name)
		} else {
			return fmt.Errorf("PV %s still exists", name)
		}
	}
	pvNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"pv":        name,
			"increment": duration,
		}).Debug("PV not yet fully deleted, waiting.")
	}
	pvBackoff := backoff.NewExponentialBackOff()
	pvBackoff.InitialInterval = CacheBackoffInitialInterval
	pvBackoff.RandomizationFactor = CacheBackoffRandomizationFactor
	pvBackoff.Multiplier = CacheBackoffMultiplier
	pvBackoff.MaxInterval = CacheBackoffMaxInterval
	pvBackoff.MaxElapsedTime = maxElapsedTime

	if err := backoff.RetryNotify(checkForDeletedPV, pvBackoff, pvNotify); err != nil {
		return fmt.Errorf("PV %s was not fully deleted after %3.2f seconds", name, maxElapsedTime.Seconds())
	}

	return nil
}

// waitForPVCPhase waits for a PVC to reach the specified phase.
func (h *helper) waitForPVCPhase(
	ctx context.Context, pvc *v1.PersistentVolumeClaim, phases []v1.PersistentVolumeClaimPhase,
	maxElapsedTime time.Duration,
) (*v1.PersistentVolumeClaim, error) {
	var latestPVC *v1.PersistentVolumeClaim
	var err error

	checkForPVCPhase := func() error {
		latestPVC, err = h.getCachedPVCByName(ctx, pvc.Name, pvc.Namespace)
		if err != nil {
			return err
		} else {
			for _, phase := range phases {
				if latestPVC.Status.Phase == phase {
					return nil
				}
			}
			return fmt.Errorf("PVC %s/%s not yet %s, currently %s", pvc.Namespace, pvc.Name, phases,
				latestPVC.Status.Phase)
		}
	}
	pvcNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"name":      pvc.Name,
			"namespace": pvc.Namespace,
			"increment": duration,
		}).Debugf("PVC not yet %s, waiting.", phases)
	}
	pvcBackoff := backoff.NewExponentialBackOff()
	pvcBackoff.InitialInterval = CacheBackoffInitialInterval
	pvcBackoff.RandomizationFactor = CacheBackoffRandomizationFactor
	pvcBackoff.Multiplier = CacheBackoffMultiplier
	pvcBackoff.MaxInterval = CacheBackoffMaxInterval
	pvcBackoff.MaxElapsedTime = maxElapsedTime

	if err := backoff.RetryNotify(checkForPVCPhase, pvcBackoff, pvcNotify); err != nil {
		return nil, fmt.Errorf("PVC %s/%s was not %s after %3.2f seconds",
			pvc.Namespace, pvc.Name, phases, maxElapsedTime.Seconds())
	}

	return latestPVC, nil
}

// removePVFinalizers patches a PV by removing all finalizers.
func (h *helper) removePVFinalizers(ctx context.Context, pv *v1.PersistentVolume) (*v1.PersistentVolume, error) {
	pvClone := pv.DeepCopy()
	pvClone.Finalizers = make([]string, 0)
	if patchedPV, err := h.patchPV(ctx, pv, pvClone); err != nil {

		message := "could not remove finalizers from PV"
		Logc(ctx).WithFields(LogFields{
			"PV":    pv.Name,
			"error": err,
		}).Errorf("PV upgrade: %s.", message)
		return nil, fmt.Errorf("%s: %v", message, err)

	} else {
		Logc(ctx).WithField("PV", pv.Name).Info("PV upgrade: removed finalizers from PV.")
		return patchedPV, nil
	}
}

// removePVCBindCompletedAnnotation patches a PVC by removing the bind-completed annotation.
func (h *helper) removePVCBindCompletedAnnotation(
	ctx context.Context, pvc *v1.PersistentVolumeClaim,
) (*v1.PersistentVolumeClaim, error) {
	pvcClone := pvc.DeepCopy()
	pvcClone.Annotations = make(map[string]string)

	// Copy all annotations except bind-completed
	if pvc.Annotations != nil {
		for k, v := range pvc.Annotations {
			if k != AnnBindCompleted {
				pvcClone.Annotations[k] = v
			}
		}
	}

	if patchedPVC, err := h.patchPVC(ctx, pvc, pvcClone); err != nil {
		return nil, err
	} else {
		return patchedPVC, nil
	}
}

// createCSIPVFromPV accepts an NFS or iSCSI PV plus the corresponding Trident volume, converts the PV
// to a CSI PV, and creates it in Kubernetes.
func (h *helper) createCSIPVFromPV(
	ctx context.Context, pv *v1.PersistentVolume, volume *storage.VolumeExternal,
) (*v1.PersistentVolume, error) {
	fsType := ""
	readOnly := false
	if pv.Spec.NFS != nil {
		readOnly = pv.Spec.NFS.ReadOnly
	} else if pv.Spec.ISCSI != nil {
		readOnly = pv.Spec.ISCSI.ReadOnly
		fsType = pv.Spec.ISCSI.FSType
	}

	volumeAttributes := map[string]string{
		"backendUUID":  volume.BackendUUID,
		"name":         volume.Config.Name,
		"internalName": volume.Config.InternalName,
		"protocol":     string(volume.Config.Protocol),
	}

	csiPV := pv.DeepCopy()
	csiPV.ResourceVersion = ""
	csiPV.UID = ""
	csiPV.Spec.NFS = nil
	csiPV.Spec.ISCSI = nil
	csiPV.Spec.CSI = &v1.CSIPersistentVolumeSource{
		Driver:           csi.Provisioner,
		VolumeHandle:     pv.Name,
		ReadOnly:         readOnly,
		FSType:           fsType,
		VolumeAttributes: volumeAttributes,
	}

	if csiPV.Annotations == nil {
		csiPV.Annotations = make(map[string]string)
	}
	csiPV.Annotations[AnnDynamicallyProvisioned] = csi.Provisioner

	if csiPV, err := h.kubeClient.CoreV1().PersistentVolumes().Create(ctx, csiPV, createOpts); err != nil {
		return nil, err
	} else {
		return csiPV, nil
	}
}

func (h *helper) getPodsForPVC(ctx context.Context, pvc *v1.PersistentVolumeClaim) ([]string, []string, error) {
	nakedPodsForPVC := make([]string, 0)
	ownedPodsForPVC := make([]string, 0)

	podList, err := h.kubeClient.CoreV1().Pods(pvc.Namespace).List(ctx, listOpts)
	if err != nil {
		return nil, nil, err
	} else if podList.Items == nil {
		return ownedPodsForPVC, nakedPodsForPVC, nil
	}

	for _, pod := range podList.Items {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == pvc.Name {
				if pod.OwnerReferences == nil || len(pod.OwnerReferences) == 0 {
					nakedPodsForPVC = append(nakedPodsForPVC, pod.Name)
				} else {
					ownedPodsForPVC = append(ownedPodsForPVC, pod.Name)
				}
			}
		}
	}

	return ownedPodsForPVC, nakedPodsForPVC, nil
}

// waitForDeletedOrNonRunningPod waits for a pod to be fully deleted or be in a non-Running state.
func (h *helper) waitForDeletedOrNonRunningPod(
	ctx context.Context, name, namespace string, maxElapsedTime time.Duration,
) (*v1.Pod, error) {
	var pod *v1.Pod
	var err error

	checkForDeletedPod := func() error {
		if pod, err = h.kubeClient.CoreV1().Pods(namespace).Get(ctx, name, getOpts); err != nil {

			// NotFound is a terminal success condition
			if statusErr, ok := err.(*apierrors.StatusError); ok {
				if statusErr.Status().Reason == metav1.StatusReasonNotFound {
					Logc(ctx).WithField("pod", fmt.Sprintf("%s/%s", namespace, name)).Info("Pod not found.")
					return nil
				}
			}

			// Retry on any other error
			return err

		} else if pod == nil {
			// Shouldn't happen
			return fmt.Errorf("kubernetes API returned nil for pod %s/%s", namespace, name)
		} else if pod.Status.Phase == v1.PodRunning {
			return fmt.Errorf("pod %s/%s phase is %s", namespace, name, pod.Status.Phase)
		} else {
			// Any phase but Running is a terminal success condition
			Logc(ctx).WithField("pod", fmt.Sprintf("%s/%s", namespace, name)).Infof("Pod phase is %s.",
				pod.Status.Phase)
			return nil
		}
	}
	podNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"pod":       name,
			"namespace": namespace,
			"increment": duration,
		}).Debug("Pod not yet deleted, waiting.")
	}
	podBackoff := backoff.NewExponentialBackOff()
	podBackoff.InitialInterval = CacheBackoffInitialInterval
	podBackoff.RandomizationFactor = CacheBackoffRandomizationFactor
	podBackoff.Multiplier = CacheBackoffMultiplier
	podBackoff.MaxInterval = CacheBackoffMaxInterval
	podBackoff.MaxElapsedTime = maxElapsedTime

	if err := backoff.RetryNotify(checkForDeletedPod, podBackoff, podNotify); err != nil {
		return nil, fmt.Errorf("pod %s/%s was not deleted or non-Running after %3.2f seconds",
			namespace, name, maxElapsedTime.Seconds())
	}

	return pod, nil
}

func (h *helper) handleFailedPVUpgrades(ctx context.Context) error {
	volumes, err := h.orchestrator.ListVolumes(ctx)
	if err != nil {
		return fmt.Errorf("could not list known volumes; %v", err)
	}
	for _, volume := range volumes {
		// If the volume is upgrading, we need to clean up the transaction
		if volume.State == storage.VolumeStateUpgrading {
			volTxn := &storage.VolumeTransaction{
				Config: volume.Config,
			}
			volTxn, err = h.orchestrator.GetVolumeTransaction(ctx, volTxn)
			if err != nil {
				return fmt.Errorf("could not get volume upgrade transaction; %v", err)
			}
			if volTxn == nil {
				return fmt.Errorf("PV upgrade: volume transaction was not found")
			}
			if volTxn.Op == storage.UpgradeVolume {
				if err = h.rollBackPVUpgrade(ctx, volTxn); err != nil {
					return fmt.Errorf("error rolling back PV upgrade; %v", err)
				}
			}
		}
	}
	return nil
}
