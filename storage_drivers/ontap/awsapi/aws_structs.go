// Copyright 2023 NetApp, Inc. All Rights Reserved.

package awsapi

import "time"

const (
	StateCreating      = "CREATING"
	StateCreated       = "CREATED"
	StateDeleting      = "DELETING"
	StateFailed        = "FAILED"
	StateMisconfigured = "MISCONFIGURED"
	StatePending       = "PENDING"
	StateAvailable     = "AVAILABLE"
	StateDeleted       = "DELETED"
)

type FSxObject struct {
	ARN  string `json:"arn"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Secret struct {
	FSxObject
	SecretARN string            `json:"secretARN"`
	SecretMap map[string]string `json:"secretMap"`
}

type Filesystem struct {
	FSxObject
	Created time.Time `json:"created"`
	OwnerID string    `json:"ownerID"`
	State   string    `json:"state"`
	VPCID   string    `json:"vpcID"`
}

type SVM struct {
	FSxObject
	Created      time.Time `json:"created"`
	FilesystemID string    `json:"filesystemID"`
	State        string    `json:"state"`
	Subtype      string    `json:"subtype"`
	UUID         string    `json:"uuid"`

	IscsiEndpoint *Endpoint `json:"iscsiEndpoint"`
	MgtEndpoint   *Endpoint `json:"mgtEndpoint"`
	NFSEndpoint   *Endpoint `json:"nfsEndpoint"`
	SMBEndpoint   *Endpoint `json:"smbEndpoint"`
}

type Endpoint struct {
	DNSName     string   `json:"dnsName"`
	IPAddresses []string `json:"ipAddresses"`
}

type Volume struct {
	FSxObject
	Created        time.Time         `json:"created"`
	FilesystemID   string            `json:"filesystemID"`
	State          string            `json:"state"`
	JunctionPath   string            `json:"junctionPath"`
	Labels         map[string]string `json:"labels"`
	SecurityStyle  string            `json:"securityStyle"`
	Size           uint64            `json:"size"`
	SnapshotPolicy string            `json:"snapshotPolicy"`
	SVMID          string            `json:"svmID"`
	UUID           string            `json:"uuid"`
}

type VolumeCreateRequest struct {
	SVMID             string            `json:"svmID"`
	Name              string            `json:"name"`
	Zone              string            `json:"zone,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	ProtocolTypes     []string          `json:"protocolTypes"`
	SizeBytes         int64             `json:"sizeBytes"`
	SecurityStyle     string            `json:"securityStyle"`
	SnapReserve       *int64            `json:"snapReserve,omitempty"`
	SnapshotDirectory bool              `json:"snapshotDirectory"`
	SnapshotPolicy    string            `json:"snapshotPolicy,omitempty"`
	UnixPermissions   string            `json:"unixPermissions,omitempty"`
	BackupID          string            `json:"backupId,omitempty"`
	SnapshotID        string            `json:"snapshotId,omitempty"`
}

type SecretCreateRequest struct {
	Name        string            `json:"Name"`
	Description string            `json:"Description"`
	SecretData  map[string]string `json:"SecretData"`
	Tags        map[string]string `json:"Tags"`
}

type SVMCreateRequest struct {
	ClientRequestToken      string `json:"ClientRequestToken"`
	SecretARN               string `json:"SecretARN"`
	Name                    string `json:"Name"`
	RootVolumeSecurityStyle string `json:"RootVolumeSecurityStyle"`
	SvmAdminPassword        string `json:"SvmAdminPassword"`
}
