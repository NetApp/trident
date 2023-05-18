// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import "time"

//go:generate mockgen -destination=../../../mocks/mock_storage_drivers/mock_ontap/mock_api.go github.com/netapp/trident/storage_drivers/ontap/api OntapAPI,AggregateSpace,Response

type Volume struct {
	AccessType        string
	Aggregates        []string
	Comment           string
	Encrypt           *bool
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

type (
	Volumes        []*Volume
	VolumeNameList []string
)

type Snapshot struct {
	CreateTime string
	Name       string
}

type Snapshots []Snapshot

type Lun struct {
	Comment        string
	CreateTime     string
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

type (
	Qtrees        []*Qtree
	QtreeNameList []string
)

type QuotaEntry struct {
	Target         string
	DiskLimitBytes int64
}
type QuotaEntries []*QuotaEntry

const (
	SnapmirrorPolicyRuleAll = "all_source_snapshots"
)

type SnapmirrorState string

const (
	SnapmirrorStateUninitialized = SnapmirrorState("uninitialized")
	SnapmirrorStateSnapmirrored  = SnapmirrorState("snapmirrored")
	SnapmirrorStateBrokenOffZapi = SnapmirrorState("broken-off")
	SnapmirrorStateBrokenOffRest = SnapmirrorState("broken_off")
	SnapmirrorStateSynchronizing = SnapmirrorState("synchronizing")
	SnapmirrorStateInSync        = SnapmirrorState("in_sync")
)

func (s SnapmirrorState) IsUninitialized() bool {
	return s == SnapmirrorStateUninitialized
}

type SnapmirrorStatus string

const (
	SnapmirrorStatusIdle         = SnapmirrorStatus("idle")
	SnapmirrorStatusAborting     = SnapmirrorStatus("aborting")
	SnapmirrorStatusBreaking     = SnapmirrorStatus("breaking")
	SnapmirrorStatusQuiescing    = SnapmirrorStatus("quiescing")
	SnapmirrorStatusTransferring = SnapmirrorStatus("transferring")
	// Snapmirror transfer status for REST
	SnapmirrorStatusAborted     = SnapmirrorStatus("aborted")
	SnapmirrorStatusFailed      = SnapmirrorStatus("failed")
	SnapmirrorStatusHardAborted = SnapmirrorStatus("hard_aborted")
	SnapmirrorStatusQueued      = SnapmirrorStatus("queued")
	SnapmirrorStatusSuccess     = SnapmirrorStatus("success")
)

func (s SnapmirrorStatus) IsAborting() bool {
	return s == SnapmirrorStatusAborting
}

func (s SnapmirrorStatus) IsIdle() bool {
	return s == SnapmirrorStatusIdle
}

func (s SnapmirrorStatus) IsBreaking() bool {
	return s == SnapmirrorStatusBreaking
}

func (s SnapmirrorStatus) IsTransferring() bool {
	return s == SnapmirrorStatusTransferring
}

type Snapmirror struct {
	State               SnapmirrorState
	RelationshipStatus  SnapmirrorStatus
	LastTransferType    string
	IsHealthy           bool
	UnhealthyReason     string
	ReplicationPolicy   string
	ReplicationSchedule string
	EndTransferTime     *time.Time
}

type SnapmirrorPolicyType string

const (
	SnapmirrorPolicyZAPITypeSync  = SnapmirrorPolicyType("sync_mirror")
	SnapmirrorPolicyZAPITypeAsync = SnapmirrorPolicyType("async_mirror")
	SnapmirrorPolicyRESTTypeSync  = SnapmirrorPolicyType("sync")
	SnapmirrorPolicyRESTTypeAsync = SnapmirrorPolicyType("async")
)

type SnapmirrorPolicy struct {
	Type             SnapmirrorPolicyType
	CopyAllSnapshots bool
}

func (s SnapmirrorPolicyType) IsSnapmirrorPolicyTypeSync() bool {
	return (s == SnapmirrorPolicyZAPITypeSync || s == SnapmirrorPolicyRESTTypeSync)
}

func (s SnapmirrorPolicyType) IsSnapmirrorPolicyTypeAsync() bool {
	return (s == SnapmirrorPolicyZAPITypeAsync || s == SnapmirrorPolicyRESTTypeAsync)
}
