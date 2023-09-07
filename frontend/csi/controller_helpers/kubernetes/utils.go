// Copyright 2023 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"fmt"

	v1 "k8s.io/api/core/v1"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	versionutils "github.com/netapp/trident/utils/version"
)

// validateKubeVersion logs a warning if the detected Kubernetes version is outside the supported range.
func (h *helper) validateKubeVersion() error {
	// Parse Kubernetes version into a SemVer object for simple comparisons
	if version, err := versionutils.ParseSemantic(h.kubeVersion.GitVersion); err != nil {
		return err
	} else if !version.AtLeast(versionutils.MustParseMajorMinorVersion(config.KubernetesVersionMin)) {
		Log().Warningf("%s v%s may not support container orchestrator version %s.%s (%s)! Supported "+
			"Kubernetes versions are %s-%s. K8S helper frontend proceeds as if you are running Kubernetes %s!",
			config.OrchestratorName, config.OrchestratorVersion, h.kubeVersion.Major, h.kubeVersion.Minor,
			h.kubeVersion.GitVersion, config.KubernetesVersionMin, config.KubernetesVersionMax,
			config.KubernetesVersionMax)
	}
	return nil
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
