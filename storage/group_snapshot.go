// Copyright 2025 NetApp, Inc. All Rights Reserved.

package storage

import (
	"fmt"
)

const (
	FakeStorageType = "fake"
)

// GroupSnapshotTargetVolumes is a map of unique volume identifiers to a list of Trident-persisted volume configs.
// The outer map is keyed by the storage volume ID and the inner map is keyed by the volume name.
// This enforces uniqueness between the volume name and the storage volume ID and removes the need for any deduplication.
type GroupSnapshotTargetVolumes map[string]map[string]*VolumeConfig

// GroupSnapshotTargetInfo contains a set of internal identifiers and unique volume identifiers for a storage backend.
type GroupSnapshotTargetInfo struct {
	StorageType    string                     // Type of storage system. This must be equal across all backends.
	StorageUUID    string                     // Opaque storage system ID. This must be equal across all backends.
	StorageVolumes GroupSnapshotTargetVolumes // A set of unique storage volumes to their persisted volume counterparts.
}

func NewGroupSnapshotTargetInfo(
	storageType, storageUUID string, storageVols GroupSnapshotTargetVolumes,
) *GroupSnapshotTargetInfo {
	return &GroupSnapshotTargetInfo{
		StorageType:    storageType,
		StorageUUID:    storageUUID,
		StorageVolumes: storageVols,
	}
}

func (b *GroupSnapshotTargetInfo) GetStorageType() string {
	return b.StorageType
}

func (b *GroupSnapshotTargetInfo) GetStorageUUID() string {
	return b.StorageUUID
}

func (b *GroupSnapshotTargetInfo) GetVolumes() GroupSnapshotTargetVolumes {
	return b.StorageVolumes
}

// AddVolumes adds a set of source volume IDs to volume names to volume configs while avoiding duplicates.
func (b *GroupSnapshotTargetInfo) AddVolumes(targetVolumes GroupSnapshotTargetVolumes) {
	// Ensure that the StorageVolumes map is initialized.
	if b.StorageVolumes == nil {
		b.StorageVolumes = make(GroupSnapshotTargetVolumes)
	}

	for sourceVolumeID, volumesToConfigs := range targetVolumes {
		// If an entry for this sourceVolumeID doesn't exist, allocate it.
		if b.StorageVolumes[sourceVolumeID] == nil {
			b.StorageVolumes[sourceVolumeID] = make(map[string]*VolumeConfig)
		}

		// Make a set of maps for each volume config.
		for volumeName, volumeConfig := range volumesToConfigs {
			// The name of the volume config is the key for the inner map.
			// This value should be unique across all storage volume IDs.
			// If the volume name already exists, skip it.
			if _, exists := b.StorageVolumes[sourceVolumeID][volumeName]; exists {
				continue
			}
			b.StorageVolumes[sourceVolumeID][volumeName] = volumeConfig
		}
	}
}

func (b *GroupSnapshotTargetInfo) Validate() error {
	if b.StorageType == FakeStorageType {
		return nil
	}
	if b.StorageType == "" {
		return fmt.Errorf("empty storage type on group snapshot target")
	}
	if b.StorageUUID == "" {
		return fmt.Errorf("empty storage uuid on group snapshot target")
	}
	if b.StorageVolumes == nil {
		return fmt.Errorf("empty source volumes on group snapshot target")
	}
	return nil
}

// IsShared checks if the target identifier and type is shared between two group snapshot target infos.
func (b *GroupSnapshotTargetInfo) IsShared(c *GroupSnapshotTargetInfo) bool {
	if b.StorageType == FakeStorageType {
		if b.StorageUUID != "" && b.StorageUUID != c.GetStorageUUID() {
			return false
		} else {
			return true
		}
	} else if b.StorageType != c.GetStorageType() {
		return false
	} else if b.StorageUUID != c.GetStorageUUID() {
		return false
	}

	return true
}

// GroupSnapshotConfig stores the configuration of a group snapshot.
// Some fields are set in the frontend of Trident, while others are set in the backend.
type GroupSnapshotConfig struct {
	Version      string   `json:"version,omitempty"`
	Name         string   `json:"name,omitempty"` // Name of the group snapshot should always be the group snapshot ID.
	InternalName string   `json:"internalName,omitempty"`
	VolumeNames  []string `json:"volumeNames,omitempty"` // VolumeNames are the source volumes for this group snapshot.
}

// ID returns the <groupsnapshot-UID> of the group snapshot. This is also the external name.
// Example: "groupsnapshot-13695cff-d8ab-486e-98df-971a6b235a67"
func (gsc *GroupSnapshotConfig) ID() string {
	return gsc.Name
}

func (gsc *GroupSnapshotConfig) GetVolumeNames() []string {
	return gsc.VolumeNames
}

func (gsc *GroupSnapshotConfig) Validate() error {
	if gsc.Name == "" {
		return fmt.Errorf("the following field for \"GroupSnapshot\" is mandatory: name")
	}
	if len(gsc.VolumeNames) == 0 {
		return fmt.Errorf("the following field for \"GroupSnapshot\" is mandatory: volumeNames")
	}
	return nil
}

func (gsc *GroupSnapshotConfig) ConstructClone() *GroupSnapshotConfig {
	return &GroupSnapshotConfig{
		Version:      gsc.Version,
		Name:         gsc.Name,
		InternalName: gsc.InternalName,
		VolumeNames:  gsc.VolumeNames,
	}
}

// GroupSnapshot is an internal representation of a group snapshot.
// This should be consumed by the frontends in its GroupSnapshotExternal form
// and by the store in its GroupSnapshotPersistent form.
//
//	NOTE: It does not contain the constituent snapshots; only their IDs.
type GroupSnapshot struct {
	*GroupSnapshotConfig
	// SnapshotIDs is the list of individual snapshot IDs in the group snapshot.
	SnapshotIDs []string `json:"snapshotIDs"`
	Created     string   `json:"dateCreated"`
}

// NewGroupSnapshot creates a new GroupSnapshot instance.
// This is used to construct the internal representation of a group snapshot.
func NewGroupSnapshot(config *GroupSnapshotConfig, snapshotIDs []string, created string) *GroupSnapshot {
	gs := &GroupSnapshot{}
	gs.GroupSnapshotConfig = config
	gs.SnapshotIDs = snapshotIDs
	gs.Created = created
	return gs
}

func (gs *GroupSnapshot) SetConfig(c *GroupSnapshotConfig) {
	gs.GroupSnapshotConfig = c
}

// Config returns a clone of the GroupSnapshotConfig of the group snapshot.
func (gs *GroupSnapshot) Config() *GroupSnapshotConfig {
	if gs.GroupSnapshotConfig == nil {
		return &GroupSnapshotConfig{}
	}
	return gs.GroupSnapshotConfig.ConstructClone()
}

func (gs *GroupSnapshot) GetSnapshotIDs() []string {
	snapshotIDs := make([]string, len(gs.SnapshotIDs))
	for i, id := range gs.SnapshotIDs {
		snapshotIDs[i] = id
	}
	return snapshotIDs
}

func (gs *GroupSnapshot) GetCreated() string {
	return gs.Created
}

func (gs *GroupSnapshot) ConstructClone() *GroupSnapshot {
	clone := &GroupSnapshot{}
	snapshotIDs := make([]string, len(gs.SnapshotIDs))
	for i, id := range gs.SnapshotIDs {
		snapshotIDs[i] = id
	}

	// Copy the group snapshot config and assign the new snapshots.
	clone.GroupSnapshotConfig = gs.Config()
	clone.SnapshotIDs = snapshotIDs
	clone.Created = gs.Created
	return clone
}

// ConstructExternal accepts a list of snapshot objects and returns a GroupSnapshotExternal object
// that embeds the GroupSnapshot object and a list of SnapshotExternal objects.
func (gs *GroupSnapshot) ConstructExternal() *GroupSnapshotExternal {
	clone := gs.ConstructClone()
	return NewGroupSnapshotExternal(clone)
}

func (gs *GroupSnapshot) ConstructPersistent() *GroupSnapshotPersistent {
	clone := gs.ConstructClone()
	return NewGroupSnapshotPersistent(clone)
}

// GroupSnapshotExternal is the external representation of a group snapshot.
// This should be used for API responses and external interactions.
type GroupSnapshotExternal struct {
	GroupSnapshot
}

// NewGroupSnapshotExternal creates a new GroupSnapshotExternal instance.
// This is used to construct the external representation of a group snapshot for API responses.
// This allows Trident treat group snapshots things as a single object in the frontends.
func NewGroupSnapshotExternal(group *GroupSnapshot) *GroupSnapshotExternal {
	return &GroupSnapshotExternal{*group}
}

// GroupSnapshotPersistent is the persistent representation of a group snapshot.
// This should only be used for internal storage and database interactions.
type GroupSnapshotPersistent struct {
	GroupSnapshot
}

func NewGroupSnapshotPersistent(group *GroupSnapshot) *GroupSnapshotPersistent {
	clone := group.ConstructClone()
	return &GroupSnapshotPersistent{*clone}
}

func (gsp *GroupSnapshotPersistent) ConstructInternal() *GroupSnapshot {
	return NewGroupSnapshot(gsp.GroupSnapshotConfig, gsp.SnapshotIDs, gsp.Created)
}

type ByGroupSnapshotExternalID []*GroupSnapshotExternal

func (a ByGroupSnapshotExternalID) Len() int { return len(a) }
func (a ByGroupSnapshotExternalID) Less(i, j int) bool {
	return a[i].ID() < a[j].ID()
}
func (a ByGroupSnapshotExternalID) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
