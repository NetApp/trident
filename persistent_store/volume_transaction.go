// Copyright 2018 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"github.com/netapp/trident/storage"
)

type VolumeOperation string

const (
	AddVolume      VolumeOperation = "addVolume"
	DeleteVolume   VolumeOperation = "deleteVolume"
	ImportVolume   VolumeOperation = "importVolume"
	ResizeVolume   VolumeOperation = "resizeVolume"
	AddSnapshot    VolumeOperation = "addSnapshot"
	DeleteSnapshot VolumeOperation = "deleteSnapshot"
)

type VolumeTransaction struct {
	Config         *storage.VolumeConfig
	SnapshotConfig *storage.SnapshotConfig
	Op             VolumeOperation
}

// getKey returns a unique identifier for the VolumeTransaction.  Volume
// transactions should only be identified by their name.  It's possible that
// some situations will leave a delete transaction dangling; an add transaction
// should overwrite this.
func (t *VolumeTransaction) getKey() string {
	switch t.Op {
	case AddVolume, DeleteVolume, ImportVolume, ResizeVolume:
		return t.Config.Name
	case AddSnapshot, DeleteSnapshot:
		return t.SnapshotConfig.ID()
	default:
		return ""
	}
}
