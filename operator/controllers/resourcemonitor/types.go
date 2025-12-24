// Copyright 2025 NetApp, Inc. All Rights Reserved.

package resourcemonitor

import (
	storagev1 "k8s.io/api/storage/v1"

	netappv1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
)

// StorageDriverHandler defines the interface for driver-specific storage class operations
type StorageDriverHandler interface {
	ShouldManageStorageClass(sc *storagev1.StorageClass) bool

	ValidateStorageClass(sc *storagev1.StorageClass) error

	HasRelevantChanges(oldSC, newSC *storagev1.StorageClass) bool

	BuildAdditionalStoragePoolsValue(sc *storagev1.StorageClass) (string, error)

	BuildTridentConfiguratorSpec(sc *storagev1.StorageClass) (*netappv1.TridentConfiguratorSpec, error)
}
