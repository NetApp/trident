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

	APIServiceLevel1 = "low"
	APIServiceLevel2 = "medium"
	APIServiceLevel3 = "high"

	UserServiceLevel1 = "standard"
	UserServiceLevel2 = "premium"
	UserServiceLevel3 = "extreme"

	AccessReadOnly  = "ReadOnly"
	AccessReadWrite = "ReadWrite"
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

type ServiceLevelsResponse []ServiceLevel

type ServiceLevel struct {
	Name        string `json:"name"`
	Performance string `json:"performance"`
}

type Volume struct {
	Created               time.Time      `json:"created"`
	LifeCycleState        string         `json:"lifeCycleState"`
	LifeCycleStateDetails string         `json:"lifeCycleStateDetails"`
	Name                  string         `json:"name"`
	OwnerID               string         `json:"ownerId"`
	Region                string         `json:"region"`
	VolumeID              string         `json:"volumeId"`
	CreationToken         string         `json:"creationToken"`
	ExportPolicy          ExportPolicy   `json:"exportPolicy"`
	Jobs                  []Job          `json:"jobs"`
	Labels                []string       `json:"labels"`
	MountPoints           []MountPoint   `json:"mountPoints"`
	ProtocolTypes         []string       `json:"protocolTypes"`
	QuotaInBytes          int64          `json:"quotaInBytes"`
	SecurityStyle         string         `json:"securityStyle"`
	ServiceLevel          string         `json:"serviceLevel"`
	SnapReserve           int64          `json:"snapReserve"`
	SnapshotDirectory     bool           `json:"snapshotDirectory"`
	SnapshotPolicy        SnapshotPolicy `json:"snapshotPolicy"`
	Timezone              string         `json:"timezone,omitempty"`
	UsedBytes             int            `json:"usedBytes"`
}

type MountPoint struct {
	Export       string `json:"export"`
	ExportFull   string `json:"exportFull"`
	Instructions string `json:"instructions"`
	ProtocolType string `json:"protocolType"`
	Server       string `json:"server"`
	VlanID       int    `json:"vlanId"`
}

type Job struct{}

type VolumeCreateRequest struct {
	Name              string         `json:"name"`
	Region            string         `json:"region"`
	BackupPolicy      *BackupPolicy  `json:"backupPolicy,omitempty"`
	CreationToken     string         `json:"creationToken"`
	ExportPolicy      ExportPolicy   `json:"exportPolicy,omitempty"`
	Jobs              []Job          `json:"jobs,omitempty"`
	Labels            []string       `json:"labels,omitempty"`
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
	Network           string         `json:"network,omitempty"`
	SnapshotID        string         `json:"snapshotId,omitempty"`
}

type VolumeRenameRequest struct {
	Name          string `json:"name"`
	Region        string `json:"region"`
	CreationToken string `json:"creationToken"`
	ServiceLevel  string `json:"serviceLevel"`
	QuotaInBytes  int64  `json:"quotaInBytes"`
}

type VolumeResizeRequest struct {
	Region        string   `json:"region"`
	CreationToken string   `json:"creationToken"`
	ProtocolTypes []string `json:"protocolTypes"`
	QuotaInBytes  int64    `json:"quotaInBytes"`
	ServiceLevel  string   `json:"serviceLevel"`
}

type VolumeRenameRelabelRequest struct {
	Name          string   `json:"name,omitempty"`
	Region        string   `json:"region"`
	CreationToken string   `json:"creationToken"`
	ProtocolTypes []string `json:"protocolTypes"`
	Labels        []string `json:"labels"`
	QuotaInBytes  int64    `json:"quotaInBytes"`
}

type BackupPolicy struct {
	DailyBackupsToKeep   int  `json:"dailyBackupsToKeep"`
	DeleteAllBackups     bool `json:"deleteAllBackups"`
	Enabled              bool `json:"enabled"`
	MonthlyBackupsToKeep int  `json:"monthlyBackupsToKeep"`
	WeeklyBackupsToKeep  int  `json:"weeklyBackupsToKeep"`
}

type ExportPolicy struct {
	Rules []ExportRule `json:"rules"`
}

type ExportRule struct {
	Access         string  `json:"access"`
	AllowedClients string  `json:"allowedClients"`
	NFSv3          Checked `json:"nfsv3,omitempty"`
	NFSv4          Checked `json:"nfsv4,omitempty"`
}

type Checked struct {
	Checked bool `json:"checked"`
}

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

type Snapshot struct {
	Created               time.Time `json:"created"`
	VolumeID              string    `json:"volumeId"`
	LifeCycleState        string    `json:"lifeCycleState"`
	LifeCycleStateDetails string    `json:"lifeCycleStateDetails"`
	Name                  string    `json:"name"`
	OwnerID               string    `json:"ownerId"`
	Region                string    `json:"region"`
	SnapshotID            string    `json:"snapshotId"`
	UsedBytes             int       `json:"usedBytes"`
}

type SnapshotCreateRequest struct {
	VolumeID string `json:"volumeId"`
	Name     string `json:"name"`
}

type SnapshotRevertRequest struct {
	Name   string `json:"name"`
	Region string `json:"region"`
}
