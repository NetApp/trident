// Copyright 2019 NetApp, Inc. All Rights Reserved.

package api

import "time"

const (
	StateCreating  = "creating"
	StateAvailable = "available"
	StateUpdating  = "updating"
	StateDisabled  = "disabled"
	StateDeleting  = "deleting"
	StateDeleted   = "deleted"
	StateError     = "error"

	ProtocolTypeNFSv3 = "NFSv3"
	ProtocolTypeNFSv4 = "NFSv4"
	ProtocolTypeCIFS  = "CIFS"

	ServiceLevelStandard = "standard"
	ServiceLevelPremium  = "premium"
	ServiceLevelExtreme  = "extreme"
)

// CallResponseError is used for errors on RESTful calls to return what went wrong
type CallResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type VersionResponse struct {
	APIVersion string `json:"apiVersion"`
	SdeVersion string `json:"sdeVersion"`
}

type RegionsResponse struct {
	Regions []Region
}

type Region struct {
	UUID      string    `json:"UUID"`
	CreatedAt time.Time `json:"createdAt"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type FileSystem struct {
	Created               time.Time      `json:"created,omitempty"`
	ExportPolicy          ExportPolicy   `json:"exportPolicy,omitempty"`
	Labels                []string       `json:"labels,omitempty"`
	FileSystemID          string         `json:"fileSystemId,omitempty"`
	LifeCycleState        string         `json:"lifeCycleState,omitempty"`
	LifeCycleStateDetails string         `json:"lifeCycleStateDetails,omitempty"`
	Name                  string         `json:"name,omitempty"`
	OwnerID               string         `json:"ownerId,omitempty"`
	Region                string         `json:"region,omitempty"`
	CreationToken         string         `json:"creationToken,omitempty"`
	ProtocolTypes         []string       `json:"protocolTypes"`
	QuotaInBytes          int64          `json:"quotaInBytes,omitempty"`
	ServiceLevel          string         `json:"serviceLevel,omitempty"`
	SnapReserve           int            `json:"snapReserve,omitempty"`
	SnapshotDirectory     bool           `json:"snapshotDirectory,omitempty"`
	SnapshotPolicy        SnapshotPolicy `json:"snapshotPolicy,omitempty"`
	Timezone              string         `json:"timezone,omitempty"`
	UsedBytes             int            `json:"usedBytes,omitempty"`
}

type FilesystemCreateRequest struct {
	Name              string         `json:"name"`
	Region            string         `json:"region"`
	BackupPolicy      *BackupPolicy  `json:"backupPolicy,omitempty"`
	CreationToken     string         `json:"creationToken"`
	ExportPolicy      ExportPolicy   `json:"exportPolicy,omitempty"`
	Jobs              []Job          `json:"jobs,omitempty"`
	Labels            []string       `json:"labels,omitempty"`
	PoolID            string         `json:"poolId,omitempty"`
	ProtocolTypes     []string       `json:"protocolTypes"`
	QuotaInBytes      int64          `json:"quotaInBytes"`
	SecurityStyle     string         `json:"securityStyle"`
	ServiceLevel      string         `json:"serviceLevel"`
	SnapReserve       *int64         `json:"snapReserve,omitempty"`
	SnapshotDirectory bool           `json:"snapshotDirectory"`
	SnapshotPolicy    SnapshotPolicy `json:"snapshotPolicy,omitempty"`
	Timezone          string         `json:"timezone,omitempty"`
	VendorID          string         `json:"vendorID,omitempty"`
	BackupID          string         `json:"backupId,omitempty"`
	SnapshotID        string         `json:"snapshotId,omitempty"`
}

type FilesystemRenameRequest struct {
	Name          string `json:"name"`
	Region        string `json:"region"`
	CreationToken string `json:"creationToken"`
	ServiceLevel  string `json:"serviceLevel"`
}

type FilesystemResizeRequest struct {
	Region        string `json:"region"`
	CreationToken string `json:"creationToken"`
	QuotaInBytes  int64  `json:"quotaInBytes"`
	ServiceLevel  string `json:"serviceLevel"`
}

type FilesystemRenameRelabelRequest struct {
	Name          string   `json:"name"`
	Region        string   `json:"region"`
	CreationToken string   `json:"creationToken"`
	ServiceLevel  string   `json:"serviceLevel"`
	Labels        []string `json:"labels"`
}

type BackupPolicy struct {
	DailyBackupsToKeep   int  `json:"dailyBackupsToKeep"`
	Enabled              bool `json:"enabled"`
	MonthlyBackupsToKeep int  `json:"monthlyBackupsToKeep"`
	WeeklyBackupsToKeep  int  `json:"weeklyBackupsToKeep"`
}

type ExportPolicy struct {
	Rules []ExportRule `json:"rules"`
}

type ExportRule struct {
	AllowedClients string `json:"allowedClients"`
	Cifs           bool   `json:"cifs"`
	Nfsv3          bool   `json:"nfsv3"`
	Nfsv4          bool   `json:"nfsv4"`
	RuleIndex      int    `json:"ruleIndex"`
	UnixReadOnly   bool   `json:"unixReadOnly"`
	UnixReadWrite  bool   `json:"unixReadWrite"`
}

type Job struct{}

type SnapshotPolicy struct {
	DailySchedule   DailySchedule   `json:"dailySchedule"`
	Enabled         bool            `json:"enabled"`
	HourlySchedule  HourlySchedule  `json:"hourlySchedule"`
	MonthlySchedule MonthlySchedule `json:"monthlySchedule"`
	WeeklySchedule  WeeklySchedule  `json:"weeklySchedule"`
}

type DailySchedule struct {
	Hour            int `json:"hour"`
	Minute          int `json:"minute"`
	SnapshotsToKeep int `json:"snapshotsToKeep"`
}

type HourlySchedule struct {
	Minute          int `json:"minute"`
	SnapshotsToKeep int `json:"snapshotsToKeep"`
}

type MonthlySchedule struct {
	DaysOfMonth     string `json:"daysOfMonth"`
	Hour            int    `json:"hour"`
	Minute          int    `json:"minute"`
	SnapshotsToKeep int    `json:"snapshotsToKeep"`
}

type WeeklySchedule struct {
	Day             string `json:"day"`
	Hour            int    `json:"hour"`
	Minute          int    `json:"minute"`
	SnapshotsToKeep int    `json:"snapshotsToKeep"`
}

type MountTarget struct {
	Created               time.Time `json:"created"`
	FileSystemID          string    `json:"fileSystemId"`
	LifeCycleState        string    `json:"lifeCycleState"`
	LifeCycleStateDetails string    `json:"lifeCycleStateDetails"`
	OwnerID               string    `json:"ownerId"`
	Region                string    `json:"region"`
	EndIP                 string    `json:"endIP"`
	Gateway               string    `json:"gateway"`
	IPAddress             string    `json:"ipAddress"`
	MountTargetID         string    `json:"mountTargetId"`
	Netmask               string    `json:"netmask"`
	StartIP               string    `json:"startIP"`
	VlanID                int       `json:"vlanId"`
}

type Snapshot struct {
	Created               time.Time `json:"created"`
	FileSystemID          string    `json:"fileSystemId"`
	LifeCycleState        string    `json:"lifeCycleState"`
	LifeCycleStateDetails string    `json:"lifeCycleStateDetails"`
	Name                  string    `json:"name"`
	OwnerID               string    `json:"ownerId"`
	Region                string    `json:"region"`
	SnapshotID            string    `json:"snapshotId"`
	UsedBytes             int       `json:"usedBytes"`
}

type SnapshotCreateRequest struct {
	FileSystemID string `json:"fileSystemId"`
	Name         string `json:"name"`
	Region       string `json:"region"`
}

type SnapshotRevertRequest struct {
	FileSystemID string `json:"fileSystemId"`
	Region       string `json:"region"`
	SnapshotID   string `json:"snapshotId"`
}
