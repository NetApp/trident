// Copyright 2019 NetApp, Inc. All Rights Reserved.

package sdk

import (
	"github.com/Azure/go-autorest/autorest/date"
)

const (
	StateInProgress = "InProgress" // confirm this is still a real state
	StateCreating   = "Creating"
	StateAvailable  = "Succeeded"
	StateDeleting   = "Deleting"
	StateDeleted    = "NoSuchState"
	StateError      = "Failed"

	ProtocolTypeNFSv3  = "NFSv3"
	ProtocolTypeNFSv41 = "NFSv4.1"
	//ProtocolTypeCIFS   = "CIFS"

	ServiceLevelStandard = "Standard"
	ServiceLevelPremium  = "Premium"
	ServiceLevelUltra    = "Ultra"
)

type CapacityPool struct {
	Location          string      `json:"location,omitempty"`
	ID                string      `json:"id,omitempty"`
	Name              string      `json:"name,omitempty"`
	Fullname          string      `json:"fullname,omitempty"`
	ResourceGroup     string      `json:"resourceGroup,omitempty"`
	NetAppAccount     string      `json:"netAppAccount,omitempty"`
	Type              string      `json:"type,omitempty"`
	Tags              interface{} `json:"tags,omitempty"`
	PoolID            string      `json:"poolId,omitempty"`
	Size              int64       `json:"size,omitempty"`
	ServiceLevel      string      `json:"serviceLevel,omitempty"`
	ProvisioningState string      `json:"provisioningState,omitempty"`
}

type FileSystem struct {
	ExportPolicy      ExportPolicy  `json:"exportPolicy,omitempty"`
	Labels            []string      `json:"labels,omitempty"`
	FileSystemID      string        `json:"fileSystemId,omitempty"`
	ID                string        `json:"id,omitempty"`
	ProvisioningState string        `json:"provisioningState,omitempty"`
	Name              string        `json:"name,omitempty"`
	CapacityPoolName  string        `json:"capacityPoolName,omitempty"`
	OwnerID           string        `json:"ownerId,omitempty"`
	Location          string        `json:"location,omitempty"`
	CreationToken     string        `json:"creationToken,omitempty"`
	ProtocolTypes     []string      `json:"protocolTypes,omitempty"`
	QuotaInBytes      int64         `json:"quotaInBytes,omitempty"`
	ServiceLevel      string        `json:"serviceLevel,omitempty"`
	Timezone          string        `json:"timezone,omitempty"`
	UsedBytes         int           `json:"usedBytes,omitempty"`
	Subnet            string        `json:"subnet,omitempty"`
	MountTargets      []MountTarget `json:"mountTargets,omitempty"`
}

type FilesystemCreateRequest struct {
	Name           string       `json:"name"`
	Location       string       `json:"location"`
	CapacityPool   string       `json:"capacityPool"`
	VirtualNetwork string       `json:"virtualNetwork"`
	Subnet         string       `json:"subnet"`
	CreationToken  string       `json:"creationToken"`
	ExportPolicy   ExportPolicy `json:"exportPolicy,omitempty"`
	Labels         []string     `json:"labels,omitempty"`
	PoolID         string       `json:"poolId,omitempty"`
	ProtocolTypes  []string     `json:"protocolTypes,omitempty"`
	QuotaInBytes   int64        `json:"quotaInBytes"`
	ServiceLevel   string       `json:"serviceLevel"`
	Timezone       string       `json:"timezone,omitempty"`
	SnapshotID     string       `json:"snapshotId,omitempty"`
}

type FilesystemRenameRequest struct {
	Name          string `json:"name"`
	Location      string `json:"location"`
	CreationToken string `json:"creationToken"`
	ServiceLevel  string `json:"serviceLevel"`
}

type FilesystemResizeRequest struct {
	Location      string `json:"location"`
	CreationToken string `json:"creationToken"`
	QuotaInBytes  int64  `json:"quotaInBytes"`
	ServiceLevel  string `json:"serviceLevel"`
}

type FilesystemRenameRelabelRequest struct {
	Name          string   `json:"name"`
	Location      string   `json:"location"`
	CreationToken string   `json:"creationToken"`
	ServiceLevel  string   `json:"serviceLevel"`
	Labels        []string `json:"labels"`
}

type ExportPolicy struct {
	Rules []ExportRule `json:"rules"`
}

type ExportRule struct {
	AllowedClients string `json:"allowedClients"`
	Cifs           bool   `json:"cifs"`
	Nfsv3          bool   `json:"nfsv3"`
	Nfsv41         bool   `json:"nfsv41"`
	RuleIndex      int    `json:"ruleIndex"`
	UnixReadOnly   bool   `json:"unixReadOnly"`
	UnixReadWrite  bool   `json:"unixReadWrite"`
}

type MountTarget struct {
	MountTargetID string `json:"mountTargetId"`
	FileSystemID  string `json:"fileSystemId"`
	IPAddress     string `json:"ipAddress"`
	Subnet        string `json:"subnet"`
	StartIP       string `json:"startIP"`
	EndIP         string `json:"endIP"`
	Gateway       string `json:"gateway"`
	Netmask       string `json:"netmask"`
	SmbServerFqdn string `json:"smbServerFqdn"`
}

type Snapshot struct {
	Created           date.Time `json:"created"`
	FileSystemID      string    `json:"fileSystemId"`
	ProvisioningState string    `json:"provisioningState"`
	Name              string    `json:"name"`
	OwnerID           string    `json:"ownerId"`
	Location          string    `json:"location"`
	SnapshotID        string    `json:"snapshotId"`
	UsedBytes         int       `json:"usedBytes"`
}

type SnapshotCreateRequest struct {
	FileSystemID string      `json:"fileSystemId"`
	Volume       *FileSystem `json:"fileSystem"`
	Name         string      `json:"name"`
	Location     string      `json:"location"`
}
