// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storage

// Snapshot contains the normalized volume snapshot format we report to Docker
type Snapshot struct {
	Name    string // The snapshot name or other identifier you would use to reference it
	Created string // The UTC time that the snapshot was created, in RFC3339 format
	ID      string // Unique ID assigned to the snapshot at the time of creation
	Backend string // Name of the storage backend
}

type SnapshotExternal struct {
	Snapshot
}

func (s *Snapshot) ConstructExternal() *SnapshotExternal {
	return &SnapshotExternal{*s}
}
