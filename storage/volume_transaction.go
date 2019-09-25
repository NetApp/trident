// Copyright 2019 NetApp, Inc. All Rights Reserved.

package storage

import v1 "k8s.io/api/core/v1"

type VolumeOperation string

const (
	// Transactions for synchronous operations
	AddVolume      VolumeOperation = "addVolume"
	DeleteVolume   VolumeOperation = "deleteVolume"
	ImportVolume   VolumeOperation = "importVolume"
	ResizeVolume   VolumeOperation = "resizeVolume"
	UpgradeVolume  VolumeOperation = "upgradeVolume"
	AddSnapshot    VolumeOperation = "addSnapshot"
	DeleteSnapshot VolumeOperation = "deleteSnapshot"

	// Transactions for long-running operations
	VolumeCreating VolumeOperation = "volumeCreating"
)

type VolumeTransaction struct {
	Config               *VolumeConfig
	VolumeCreatingConfig *VolumeCreatingConfig
	SnapshotConfig       *SnapshotConfig
	PVUpgradeConfig      *PVUpgradeConfig
	Op                   VolumeOperation
}

type PVUpgradeConfig struct {
	PVCConfig       *v1.PersistentVolumeClaim `json:"pvcConfig,omitempty"`
	PVConfig        *v1.PersistentVolume      `json:"pvConfig,omitempty"`
	OwnedPodsForPVC []string                  `json:"ownedPodsForPVC,omitempty"`
}

// Name returns a unique identifier for the VolumeTransaction.  Volume transactions should only
// be identified by their name, while snapshot transactions should be identified by their name as
// well as their volume name.  It's possible that some situations will leave a delete transaction
// dangling; an add transaction should overwrite this.
func (t *VolumeTransaction) Name() string {
	switch t.Op {
	case AddSnapshot, DeleteSnapshot:
		return t.SnapshotConfig.ID()
	case VolumeCreating:
		return t.VolumeCreatingConfig.Name
	default:
		return t.Config.Name
	}
}
