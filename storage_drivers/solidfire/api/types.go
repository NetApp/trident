// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import "fmt"

// Error wrapper
type Error struct {
	ID     int `json:"id"`
	Fields struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Name    string `json:"name"`
	} `json:"error"`
}

func (e Error) Error() string {
	return fmt.Sprintf("device API error: %+v", e.Fields.Name)
}

// QoS settings
type QoS struct {
	MinIOPS   int64 `json:"minIOPS,omitempty"`
	MaxIOPS   int64 `json:"maxIOPS,omitempty"`
	BurstIOPS int64 `json:"burstIOPS,omitempty"`
	BurstTime int64 `json:"-"`
}

// VolumePair settings
type VolumePair struct {
	ClusterPairID    int64  `json:"clusterPairID"`
	RemoteVolumeID   int64  `json:"remoteVolumeID"`
	RemoteSliceID    int64  `json:"remoteSliceID"`
	RemoteVolumeName string `json:"remoteVolumeName"`
	VolumePairUUID   string `json:"volumePairUUID"`
}

// Volume settings
type Volume struct {
	VolumeID           int64        `json:"volumeID"`
	Name               string       `json:"name"`
	AccountID          int64        `json:"accountID"`
	CreateTime         string       `json:"createTime"`
	Status             string       `json:"status"`
	Access             string       `json:"access"`
	Enable512e         bool         `json:"enable512e"`
	Iqn                string       `json:"iqn"`
	ScsiEUIDeviceID    string       `json:"scsiEUIDeviceID"`
	ScsiNAADeviceID    string       `json:"scsiNAADeviceID"`
	Qos                QoS          `json:"qos"`
	VolumeAccessGroups []int64      `json:"volumeAccessGroups"`
	VolumePairs        []VolumePair `json:"volumePairs"`
	DeleteTime         string       `json:"deleteTime"`
	PurgeTime          string       `json:"purgeTime"`
	SliceCount         int64        `json:"sliceCount"`
	TotalSize          int64        `json:"totalSize"`
	BlockSize          int64        `json:"blockSize"`
	VirtualVolumeID    string       `json:"virtualVolumeID"`
	Attributes         interface{}  `json:"attributes"`
}

type Snapshot struct {
	SnapshotID int64       `json:"snapshotID"`
	VolumeID   int64       `json:"volumeID"`
	Name       string      `json:"name"`
	Checksum   string      `json:"checksum"`
	Status     string      `json:"status"`
	TotalSize  int64       `json:"totalSize"`
	GroupID    int64       `json:"groupID"`
	CreateTime string      `json:"createTime"`
	Attributes interface{} `json:"attributes"`
}

// ListVolumesRequest
type ListVolumesRequest struct {
	Accounts      []int64 `json:"accounts"`
	StartVolumeID *int64  `json:"startVolumeID,omitempty"`
	Limit         *int64  `json:"limit,omitempty"`
}

// ListVolumesForAccountRequest
type ListVolumesForAccountRequest struct {
	AccountID int64 `json:"accountID"`
}

// ListActiveVolumesRequest
type ListActiveVolumesRequest struct {
	StartVolumeID int64 `json:"startVolumeID"`
	Limit         int64 `json:"limit"`
}

// ListVolumesResult
type ListVolumesResult struct {
	ID     int `json:"id"`
	Result struct {
		Volumes []Volume `json:"volumes"`
	} `json:"result"`
}

// CreateVolumeRequest
type CreateVolumeRequest struct {
	Name       string      `json:"name"`
	AccountID  int64       `json:"accountID"`
	TotalSize  int64       `json:"totalSize"`
	Enable512e bool        `json:"enable512e"`
	Qos        QoS         `json:"qos,omitempty"`
	Attributes interface{} `json:"attributes"`
}

// CreateVolumeResult
type CreateVolumeResult struct {
	ID     int `json:"id"`
	Result struct {
		VolumeID int64 `json:"volumeID"`
	} `json:"result"`
}

// DeleteVolumeRequest
type DeleteVolumeRequest struct {
	VolumeID int64 `json:"volumeID"`
}

type CloneVolumeRequest struct {
	VolumeID   int64       `json:"volumeID"`
	Name       string      `json:"name"`
	SnapshotID int64       `json:"snapshotID"`
	Attributes interface{} `json:"attributes"`
}

type CloneVolumeResult struct {
	ID     int `json:"id"`
	Result struct {
		CloneID     int64 `json:"cloneID"`
		VolumeID    int64 `json:"volumeID"`
		AsyncHandle int64 `json:"asyncHandle"`
	} `json:"result"`
}

type CreateSnapshotRequest struct {
	VolumeID                int64       `json:"volumeID"`
	SnapshotID              int64       `json:"snapshotID"`
	Name                    string      `json:"name"`
	EnableRemoteReplication bool        `json:"enableRemoteReplication"`
	Retention               string      `json:"retention"`
	Attributes              interface{} `json:"attributes"`
}

type CreateSnapshotResult struct {
	ID     int `json:"id"`
	Result struct {
		SnapshotID int64  `json:"snapshotID"`
		Checksum   string `json:"checksum"`
	} `json:"result"`
}

type ListSnapshotsRequest struct {
	VolumeID int64 `json:"volumeID"`
}

type ListSnapshotsResult struct {
	ID     int `json:"id"`
	Result struct {
		Snapshots []Snapshot `json:"snapshots"`
	} `json:"result"`
}

type RollbackToSnapshotRequest struct {
	VolumeID         int64       `json:"volumeID"`
	SnapshotID       int64       `json:"snapshotID"`
	SaveCurrentState bool        `json:"saveCurrentState"`
	Name             string      `json:"name"`
	Attributes       interface{} `json:"attributes"`
}

type RollbackToSnapshotResult struct {
	ID     int `json:"id"`
	Result struct {
		Checksum   string `json:"checksum"`
		SnapshotID int64  `json:"snapshotID"`
	} `json:"result"`
}

type DeleteSnapshotRequest struct {
	SnapshotID int64 `json:"snapshotID"`
}

// AddVolumesToVolumeAccessGroupRequest
type AddVolumesToVolumeAccessGroupRequest struct {
	VolumeAccessGroupID int64   `json:"volumeAccessGroupID"`
	Volumes             []int64 `json:"volumes"`
}

// CreateVolumeAccessGroupRequest
type CreateVolumeAccessGroupRequest struct {
	Name       string   `json:"name"`
	Volumes    []int64  `json:"volumes,omitempty"`
	Initiators []string `json:"initiators,omitempty"`
}

// CreateVolumeAccessGroupResult
type CreateVolumeAccessGroupResult struct {
	ID     int `json:"id"`
	Result struct {
		VagID int64 `json:"volumeAccessGroupID"`
	} `json:"result"`
}

// AddInitiatorsToVolumeAccessGroupRequest
type AddInitiatorsToVolumeAccessGroupRequest struct {
	Initiators []string `json:"initiators"`
	VAGID      int64    `json:"volumeAccessGroupID"`
}

// ListVolumeAccessGroupsRequest
type ListVolumeAccessGroupsRequest struct {
	StartVAGID int64 `json:"startVolumeAccessGroupID,omitempty"`
	Limit      int64 `json:"limit,omitempty"`
}

// ListVolumesAccessGroupsResult
type ListVolumesAccessGroupsResult struct {
	ID     int `json:"id"`
	Result struct {
		Vags []VolumeAccessGroup `json:"volumeAccessGroups"`
	} `json:"result"`
}

// EmptyResponse
type EmptyResponse struct {
	ID     int `json:"id"`
	Result struct {
	} `json:"result"`
}

// VolumeAccessGroup
type VolumeAccessGroup struct {
	Initiators     []string    `json:"initiators"`
	Attributes     interface{} `json:"attributes"`
	DeletedVolumes []int64     `json:"deletedVolumes"`
	Name           string      `json:"name"`
	VAGID          int64       `json:"volumeAccessGroupID"`
	Volumes        []int64     `json:"volumes"`
}

// GetAccountByNameRequest
type GetAccountByNameRequest struct {
	Name string `json:"username"`
}

// GetAccountByIDRequest
type GetAccountByIDRequest struct {
	AccountID int64 `json:"accountID"`
}

// GetAccountResult
type GetAccountResult struct {
	ID     int `json:"id"`
	Result struct {
		Account Account `json:"account"`
	} `json:"result"`
}

// Account
type Account struct {
	AccountID       int64       `json:"accountID,omitempty"`
	Username        string      `json:"username,omitempty"`
	Status          string      `json:"status,omitempty"`
	Volumes         []int64     `json:"volumes,omitempty"`
	InitiatorSecret string      `json:"initiatorSecret,omitempty"`
	TargetSecret    string      `json:"targetSecret,omitempty"`
	Attributes      interface{} `json:"attributes,omitempty"`
}

// AddAccountRequest
type AddAccountRequest struct {
	Username        string      `json:"username"`
	InitiatorSecret string      `json:"initiatorSecret,omitempty"`
	TargetSecret    string      `json:"targetSecret,omitempty"`
	Attributes      interface{} `json:"attributes,omitempty"`
}

// AddAccountResult
type AddAccountResult struct {
	ID     int `json:"id"`
	Result struct {
		AccountID int64 `json:"accountID"`
	} `json:"result"`
}

type ClusterCapacity struct {
	ActiveBlockSpace             int64  `json:"activeBlockSpace"`
	ActiveSessions               int64  `json:"activeSessions"`
	AverageIOPS                  int64  `json:"averageIOPS"`
	ClusterRecentIOSize          int64  `json:"clusterRecentIOSize"`
	CurrentIOPS                  int64  `json:"currentIOPS"`
	MaxIOPS                      int64  `json:"maxIOPS"`
	MaxOverProvisionableSpace    int64  `json:"maxOverProvisionableSpace"`
	MaxProvisionedSpace          int64  `json:"maxProvisionedSpace"`
	MaxUsedMetadataSpace         int64  `json:"maxUsedMetadataSpace"`
	MaxUsedSpace                 int64  `json:"maxUsedSpace"`
	NonZeroBlocks                int64  `json:"nonZeroBlocks"`
	PeakActiveSessions           int64  `json:"peakActiveSessions"`
	PeakIOPS                     int64  `json:"peakIOPS"`
	ProvisionedSpace             int64  `json:"provisionedSpace"`
	Timestamp                    string `json:"timestamp"`
	TotalOps                     int64  `json:"totalOps"`
	UniqueBlocks                 int64  `json:"uniqueBlocks"`
	UniqueBlocksUsedSpace        int64  `json:"uniqueBlocksUsedSpace"`
	UsedMetadataSpace            int64  `json:"usedMetadataSpace"`
	UsedMetadataSpaceInSnapshots int64  `json:"usedMetadataSpaceInSnapshots"`
	UsedSpace                    int64  `json:"usedSpace"`
	ZeroBlocks                   int64  `json:"zeroBlocks"`
}

type GetClusterCapacityRequest struct {
}

type GetClusterCapacityResult struct {
	ID     int `json:"id"`
	Result struct {
		ClusterCapacity ClusterCapacity `json:"clusterCapacity"`
	} `json:"result"`
}

type GetClusterHardwareInfoResult struct {
	ID     int `json:"id"`
	Result struct {
		ClusterHardwareInfo ClusterHardwareInfo `json:"clusterHardwareInfo"`
	} `json:"result"`
}

type DefaultQoSRequest struct {
}

type DefaultQoSResult struct {
	ID     int `json:"id"`
	Result struct {
		BurstIOPS int64 `json:"burstIOPS"`
		MaxIOPS   int64 `json:"maxIOPS"`
		MinIOPS   int64 `json:"minIOPS"`
	} `json:"result"`
}

type ClusterHardwareInfo struct {
	Drives interface{} `json:"drives"`
	Nodes  interface{} `json:"nodes"`
}

type ModifyVolumeRequest struct {
	VolumeID   int64       `json:"volumeID"`
	AccountID  int64       `json:"accountID,omitempty"`
	Access     string      `json:"access,omitempty"`
	Qos        QoS         `json:"qos,omitempty"`
	TotalSize  int64       `json:"totalSize,omitempty"`
	Attributes interface{} `json:"attributes,omitempty"`
}

type ModifyVolumeResult struct {
	Volume Volume `json:"volume,omitempty"`
	Curve  QoS    `json:"curve,omitempty"`
}
