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
	DPVolume        bool
}

type Volumes []Volume
type VolumeNameList []string

type Snapshot struct {
	CreateTime string
	Name       string
}

type Snapshots []Snapshot

type Lun struct {
	Comment      string
	Enabled      bool
	LunMaps      []LunMap
	Name         string
	Qos          QosPolicyGroup
	Size         string
	Mapped       bool
	UUID         string
	SerialNumber string
	State        string
	VolumeName   string
	OsType         string
	SpaceReserved  *bool
	SpaceAllocated *bool
}

type LunMap struct {
	IgroupName string
	LunID      int
}

type Luns []Lun

type IscsiInitiatorAuth struct {
	SVMName string
	ChapUser string
	ChapPassphrase string
	ChapOutboundUser string
	ChapOutboundPassphrase string
	Initiator string
	AuthType string
}
