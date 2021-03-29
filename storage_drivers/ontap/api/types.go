// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

type Volume struct {
	AccessType      string
	Aggregates      []string
	Comment         string
	Encrypt         bool
	ExportPolicy    string
	JunctionPath    string
	Name            string
	Qos             QosPolicyGroup
	SecurityStyle   string
	Size            string
	SnapshotDir     bool
	SnapshotPolicy  string
	SnapshotReserve int
	SpaceReserve    string
	TieringPolicy   string
	UnixPermissions string
	UUID            string
}

type Volumes []Volume
type VolumeNameList []string

type Snapshot struct {
	CreateTime string
	Name       string
}

type Snapshots []Snapshot
