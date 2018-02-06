// Copyright 2018 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"fmt"

	"github.com/netapp/trident/storage"
)

type VolumeOperation string

const (
	AddVolume    VolumeOperation = "addVolume"
	DeleteVolume VolumeOperation = "deleteVolume"
)

type VolumeTransaction struct {
	Config *storage.VolumeConfig
	Op     VolumeOperation
}

// getKey returns a unique identifier for the VolumeTransaction.  Volume
// transactions should only be identified by their name.  It's possible that
// some situations will leave a delete transaction dangling; an add transaction
// should overwrite this.
func (vt *VolumeTransaction) getKey() string {
	return fmt.Sprintf("%s", vt.Config.Name)
}
