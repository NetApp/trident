// Copyright 2025 NetApp, Inc. All Rights Reserved.

package storage

type VolumeOperation string

const (
	// Transactions for synchronous operations
	AddVolume           VolumeOperation = "addVolume"
	DeleteVolume        VolumeOperation = "deleteVolume"
	ImportVolume        VolumeOperation = "importVolume"
	ResizeVolume        VolumeOperation = "resizeVolume"
	AddSnapshot         VolumeOperation = "addSnapshot"
	DeleteSnapshot      VolumeOperation = "deleteSnapshot"
	AddGroupSnapshot    VolumeOperation = "addGroupSnapshot"
	DeleteGroupSnapshot VolumeOperation = "deleteGroupSnapshot"

	// Transactions for long-running operations
	VolumeCreating VolumeOperation = "volumeCreating"
)

type VolumeTransaction struct {
	Config               *VolumeConfig
	VolumeCreatingConfig *VolumeCreatingConfig
	SnapshotConfig       *SnapshotConfig
	GroupSnapshotConfig  *GroupSnapshotConfig
	Op                   VolumeOperation
}

// Name returns a unique identifier for the VolumeTransaction.  Volume transactions should only
// be identified by their name, while snapshot transactions should be identified by their name as
// well as their volume name.  It's possible that some situations will leave a delete transaction
// dangling; an add transaction should overwrite this.
func (t *VolumeTransaction) Name() string {
	switch t.Op {
	case AddSnapshot, DeleteSnapshot:
		return t.SnapshotConfig.ID()
	case AddGroupSnapshot, DeleteGroupSnapshot:
		return t.GroupSnapshotConfig.ID()
	case VolumeCreating:
		return t.VolumeCreatingConfig.Name
	default:
		return t.Config.Name
	}
}
