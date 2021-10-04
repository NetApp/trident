// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

//go:generate mockgen -destination=../../../mocks/mock_storage_drivers/mock_ontap/mock_api.go github.com/netapp/trident/storage_drivers/ontap/api OntapAPI

type Volume struct {
	AccessType        string
	Aggregates        []string
	Comment           string
	Encrypt           bool
	ExportPolicy      string
	JunctionPath      string
	Name              string
	Qos               QosPolicyGroup
	SecurityStyle     string
	Size              string
	SnapshotDir       bool
	SnapshotPolicy    string
	SnapshotReserve   int
	SnapshotSpaceUsed int
	SpaceReserve      string
	TieringPolicy     string
	UnixPermissions   string
	UUID              string
	DPVolume          bool
}

type CLIVolume struct {
	Vserver             string `json:"vserver"`
	Name                string `json:"volume"`
	Aggregate           string `json:"aggregate"`
	State               string `json:"state"`
	VolumeStyleExtended string `json:"volume_style_extended"`
	SpaceGuarantee      string `json:"space_guarantee"`
	SnapdirAccess       bool   `json:"snapdir_access"`
	SnapshotPolicy      string `json:"snapshot_policy"`
	Encrypt             bool   `json:"encrypt"`
	TieringPolicy       string `json:"tiering_policy"`
}

type Volumes []*Volume
type VolumeNameList []string

type Snapshot struct {
	CreateTime string
	Name       string
}

type Snapshots []Snapshot

type Lun struct {
	Comment        string
	Enabled        bool
	LunMaps        []LunMap
	Name           string
	Qos            QosPolicyGroup
	Size           string
	Mapped         bool
	UUID           string
	SerialNumber   string
	State          string
	VolumeName     string
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
	SVMName                string
	ChapUser               string
	ChapPassphrase         string
	ChapOutboundUser       string
	ChapOutboundPassphrase string
	Initiator              string
	AuthType               string
}

type Qtree struct {
	ExportPolicy    string
	Name            string
	UnixPermissions string
	SecurityStyle   string
	Volume          string
	Vserver         string
}

type Qtrees []*Qtree
type QtreeNameList []string

type QuotaEntry struct {
	Target         string
	DiskLimitBytes int64
}
type QuotaEntries []*QuotaEntry
