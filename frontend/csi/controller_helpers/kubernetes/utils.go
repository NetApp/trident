// Copyright 2023 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"

	. "github.com/netapp/trident/logging"
	versionutils "github.com/netapp/trident/utils/version"
)

// validateKubeVersion ensures the detected Kubernetes version is parseable.
func (h *helper) validateKubeVersion() error {
	_, err := versionutils.ParseSemantic(h.kubeVersion.GitVersion)
	return err
}

// getStorageClassForPVC returns StorageClassName from a PVC. If no storage class was requested, it returns "".
func getStorageClassForPVC(pvc *v1.PersistentVolumeClaim) string {
	if pvc.Spec.StorageClassName != nil {
		return *pvc.Spec.StorageClassName
	}
	return ""
}

func (h *helper) checkValidStorageClassReceived(ctx context.Context, claim *v1.PersistentVolumeClaim) error {
	// Filter unrelated claims
	if claim.Spec.StorageClassName == nil || *claim.Spec.StorageClassName == "" {
		Logc(ctx).WithField("PVC", claim.Name).Error("PVC has no storage class specified.")
		return fmt.Errorf("PVC %s has no storage class specified", claim.Name)
	}

	return nil
}

// getDataSizeFromTotalSize calculates the data size of by subtracting snapshot reserve from total size
func (h *helper) getDataSizeFromTotalSize(
	ctx context.Context, totalSize uint64, snapshotReserve int,
) uint64 {
	snapReserveMultiplier := 1.0 - (float64(snapshotReserve) / 100.0)
	sizeWithoutSnapReserve := float64(totalSize) * snapReserveMultiplier
	dataSizeBytes := uint64(sizeWithoutSnapReserve)

	Logc(ctx).WithFields(LogFields{
		"totalSize":              totalSize,
		"snapshotReserve":        snapshotReserve,
		"snapReserveMultiplier":  snapReserveMultiplier,
		"sizeWithoutSnapReserve": sizeWithoutSnapReserve,
		"dataSizeBytes":          dataSizeBytes,
	}).Debug("Calculated data size after subtracting snapshot reserve from total size.")

	return dataSizeBytes
}
