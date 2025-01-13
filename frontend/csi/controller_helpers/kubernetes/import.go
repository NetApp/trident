// Copyright 2025 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/frontend/csi"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
)

// ///////////////////////////////////////////////////////////////////////////
//
// This file contains the event handlers that import existing volumes from backends.
//
// ///////////////////////////////////////////////////////////////////////////
type errorHandler func(context.Context, *helper, *v1.PersistentVolumeClaim, error) (bool, error)

func (h *helper) ImportVolume(
	ctx context.Context, request *storage.ImportVolumeRequest,
) (*storage.VolumeExternal, error) {
	Logc(ctx).WithField("request", request).Trace(">>>> ImportVolume")
	defer Logc(ctx).Trace("<<<< ImportVolume")

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

	Logc(ctx).WithFields(LogFields{
		"PVC":               claim.Name,
		"PVC_storageClass":  getStorageClassForPVC(claim),
		"PVC_accessModes":   claim.Spec.AccessModes,
		"PVC_annotations":   claim.Annotations,
		"PVC_volume":        claim.Spec.VolumeName,
		"PVC_requestedSize": claim.Spec.Resources.Requests[v1.ResourceStorage],
	}).Trace()

	// Ensure the new PVC specified a storage class
	if err = h.checkValidStorageClassReceived(ctx, claim); err != nil {
		return nil, err
	}

	// Ensure the new PVC specified a namespace
	if len(claim.Namespace) == 0 {
		return nil, fmt.Errorf("a valid PVC namespace is required for volume import")
	}

	// Lookup backend from given name
	backend, err := h.orchestrator.GetBackend(ctx, request.Backend)
	if err != nil {
		return nil, fmt.Errorf("could not find backend %s; %v", request.Backend, err)
	}

	// Set required annotations
	if claim.Annotations == nil {
		claim.Annotations = map[string]string{}
	}
	claim.Annotations[AnnNotManaged] = strconv.FormatBool(request.NoManage)
	claim.Annotations[AnnImportOriginalName] = request.InternalName
	claim.Annotations[AnnImportBackendUUID] = backend.BackendUUID
	claim.Annotations[AnnStorageProvisioner] = csi.Provisioner

	// Find the volume on the storage system
	extantVol, err := h.orchestrator.GetVolumeForImport(ctx, request.InternalName, request.Backend)
	if err != nil {
		return nil, fmt.Errorf("volume import failed to get size of volume: %v", err)
	}

	// Ensure this volume isn't already managed
	managedVol, err := h.orchestrator.GetVolumeByInternalName(ctx, extantVol.Config.InternalName)
	if err == nil {
		return nil, errors.FoundError("PV %s already exists for volume %s", managedVol, request.InternalName)
	}

	// Set the PVC's storage field to the actual volume data size
	totalSize, err := strconv.ParseUint(extantVol.Config.Size, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("volume import failed as volume %v has invalid size %v: %v",
			request.InternalName, extantVol.Config.Size, err)
	}

	snapshotReserve := 0
	if extantVol.Config.SnapshotReserve != "" {
		snapshotReserve, err = strconv.Atoi(extantVol.Config.SnapshotReserve)
		if err != nil {
			return nil, fmt.Errorf("volume import failed as volume %v has invalid snapshot reserve %v: %v",
				request.InternalName, extantVol.Config.SnapshotReserve, err)
		}
	}

	dataSize := h.getDataSizeFromTotalSize(ctx, totalSize, snapshotReserve)

	// LUKS annotation should be accepted as either "LUKSEncryption" or "luksEncryption" to match storage pools.
	if luksAnnotation := getCaseFoldedAnnotation(claim.GetObjectMeta().GetAnnotations(), AnnLUKSEncryption); luksAnnotation != "" {
		if convert.ToBool(luksAnnotation) {
			dataSize -= luks.LUKSMetadataSize
		}
	}

	if claim.Spec.Resources.Requests == nil {
		claim.Spec.Resources.Requests = v1.ResourceList{}
	}
	claim.Spec.Resources.Requests[v1.ResourceStorage] = resource.MustParse(strconv.FormatUint(dataSize, 10))

	Logc(ctx).WithFields(LogFields{
		"total size":       extantVol.Config.Size,
		"snapshot reserve": snapshotReserve,
		"data size":        dataSize,
		"claimSize":        claim.Spec.Resources.Requests[v1.ResourceStorage],
	}).Debug("Volume import determined volume size.")

	pvc, pvcErr := h.createImportPVC(ctx, claim)
	if pvcErr != nil {
		Logc(ctx).WithFields(LogFields{
			"claim": claim.Name,
			"error": pvcErr,
		}).Warn("ImportVolume: error occurred during PVC creation.")
		return nil, pvcErr
	}
	Logc(ctx).WithField("PVC", pvc).Debug("ImportVolume: created pending PVC.")

	pvName := fmt.Sprintf("pvc-%s", pvc.GetUID())

	// Wait for the import to happen and the PV to be created by the sidecar
	// after which we can then request the volume object from the core to return
	_, err = h.waitForCachedPVByName(ctx, pvc, pvName, ImportPVCacheWaitPeriod, checkAndHandleUnrecoverableError)
	if err != nil {
		return nil, fmt.Errorf("error waiting for PV %s; %v", pvName, err)
	}

	volume, err := h.orchestrator.GetVolume(ctx, pvName)
	if err != nil {
		return nil, fmt.Errorf("error getting volume %s; %v", pvName, err)
	}

	return volume, nil
}

func (h *helper) createImportPVC(
	ctx context.Context, claim *v1.PersistentVolumeClaim,
) (*v1.PersistentVolumeClaim, error) {
	Logc(ctx).Trace(">>>> createImportPVC")
	defer Logc(ctx).Trace("<<<< createImportPVC")

	Logc(ctx).WithFields(LogFields{
		"claim":     claim,
		"namespace": claim.Namespace,
	}).Trace("CreateImportPVC: ready to create PVC")

	createOpts := metav1.CreateOptions{}
	pvc, err := h.kubeClient.CoreV1().PersistentVolumeClaims(claim.Namespace).Create(ctx, claim, createOpts)
	if err != nil {
		return nil, fmt.Errorf("error occurred during PVC creation: %v", err)
	}

	return pvc, nil
}

func checkAndHandleUnrecoverableError(ctx context.Context, h *helper, pvc *v1.PersistentVolumeClaim, pvError error,
) (bool, error) {
	Logc(ctx).Debugf(">>>> checkAndHandleUnrecoverableError")
	defer Logc(ctx).Debugf("<<<< checkAndHandleUnrecoverableError")

	// if PV is not found, check the PVC events to determine why the PV was not created
	var appendString string
	if pvError != nil {
		events, err := h.kubeClient.CoreV1().Events(pvc.Namespace).List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("involvedObject.uid=%s,involvedObject.kind=PersistentVolumeClaim", pvc.UID),
		})
		if err != nil {
			return false, err
		}
		if events != nil {
			for _, event := range events.Items {
				if event.Reason == "ProvisioningFailed" {
					if strings.Contains(event.Message, "import volume failed") {
						// we know the PV is not going to be created so it is better to delete the PVC and abort the import operation
						err := h.kubeClient.CoreV1().PersistentVolumeClaims(pvc.Namespace).Delete(ctx, pvc.Name, metav1.DeleteOptions{})
						if err != nil {
							appendString = fmt.Sprintf("unable to delete pvc %v: %v", pvc.Name, err)
						} else {
							appendString = fmt.Sprintf("deleted pvc %v", pvc.Name)
						}
						return true, backoff.Permanent(fmt.Errorf("%s;%s", event.Message, appendString))
					}
				}
			}
		}
	}
	return false, pvError
}

func (h *helper) waitForCachedPVByName(
	ctx context.Context, pvc *v1.PersistentVolumeClaim, name string, maxElapsedTime time.Duration, handler errorHandler,
) (*v1.PersistentVolume, error) {
	var pv *v1.PersistentVolume
	var unrecoverableError bool
	Logc(ctx).WithFields(LogFields{
		"Name":           name,
		"MaxElapsedTime": maxElapsedTime,
		"namespace":      pvc.Namespace,
		"pvc":            pvc.Name,
	}).Infof(">>>> waitForCachedPVByName")
	defer Logc(ctx).Info("<<<< waitForCachedPVByName")

	pvBackoff := backoff.NewExponentialBackOff()
	pvBackoff.InitialInterval = CacheBackoffInitialInterval
	pvBackoff.RandomizationFactor = CacheBackoffRandomizationFactor
	pvBackoff.Multiplier = CacheBackoffMultiplier
	pvBackoff.MaxInterval = CacheBackoffMaxInterval
	pvBackoff.MaxElapsedTime = maxElapsedTime

	checkForCachedPV := func() error {
		var pvError error
		pv, pvError = h.getCachedPVByName(ctx, name)
		unrecoverableError, pvError = handler(ctx, h, pvc, pvError)
		return pvError
	}
	pvNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"name":      name,
			"increment": duration,
		}).Trace("PV not yet in cache, waiting.")
	}

	if err := backoff.RetryNotify(checkForCachedPV, pvBackoff, pvNotify); err != nil {
		if unrecoverableError {
			return nil, err
		}
		return nil, fmt.Errorf("PV %s was not in cache after %3.2f seconds", name, maxElapsedTime.Seconds())
	}
	return pv, nil
}
