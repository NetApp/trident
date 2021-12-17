// Copyright 2021 NetApp, Inc. All Rights Reserved.

package utils

type VolumeAccessInfo struct {
	IscsiAccessInfo
	NfsAccessInfo
	MountOptions string `json:"mountOptions,omitempty"`
}

type IscsiAccessInfo struct {
	IscsiTargetPortal    string   `json:"iscsiTargetPortal,omitempty"`
	IscsiPortals         []string `json:"iscsiPortals,omitempty"`
	IscsiTargetIQN       string   `json:"iscsiTargetIqn,omitempty"`
	IscsiLunNumber       int32    `json:"iscsiLunNumber,omitempty"`
	IscsiInterface       string   `json:"iscsiInterface,omitempty"`
	IscsiIgroup          string   `json:"iscsiIgroup,omitempty"`
	IscsiVAGs            []int64  `json:"iscsiVags,omitempty"`
	IscsiUsername        string   `json:"iscsiUsername,omitempty"`
	IscsiInitiatorSecret string   `json:"iscsiInitiatorSecret,omitempty"`
	IscsiTargetUsername  string   `json:"iscsiTargetUsername,omitempty"`
	IscsiTargetSecret    string   `json:"iscsiTargetSecret,omitempty"`
	IscsiLunSerial       string   `json:"iscsiLunSerial,omitempty"`
}

type NfsAccessInfo struct {
	NfsServerIP string `json:"nfsServerIp,omitempty"`
	NfsPath     string `json:"nfsPath,omitempty"`
}

type VolumePublishInfo struct {
	Localhost      bool     `json:"localhost,omitempty"`
	HostIQN        []string `json:"hostIQN,omitempty"`
	HostIP         []string `json:"hostIP,omitempty"`
	BackendUUID    string   `json:"backendUUID,omitempty"`
	Nodes          []*Node  `json:"nodes,omitempty"`
	HostName       string   `json:"hostName,omitempty"`
	FilesystemType string   `json:"fstype,omitempty"`
	UseCHAP        bool     `json:"useCHAP,omitempty"`
	SharedTarget   bool     `json:"sharedTarget,omitempty"`
	DevicePath     string   `json:"devicePath,omitempty"`
	Unmanaged      bool     `json:"unmanaged,omitempty"`
	VolumeAccessInfo
}

type VolumeTrackingPublishInfo struct {
	StagingTargetPath string `json:"stagingTargetPath"`
}

type VolumePublication struct {
	Name       string `json:"name"`
	NodeName   string `json:"node"`
	VolumeName string `json:"volume"`
	ReadOnly   bool   `json:"readOnly"`
	// The access mode values are defined by CSI
	// See https://github.com/container-storage-interface/spec/blob/release-1.5/lib/go/csi/csi.pb.go#L135
	AccessMode int32 `json:"accessMode"`
}

type Node struct {
	Name           string            `json:"name"`
	IQN            string            `json:"iqn,omitempty"`
	IPs            []string          `json:"ips,omitempty"`
	TopologyLabels map[string]string `json:"topologyLabels,omitempty"`
	NodePrep       *NodePrep         `json:"nodePrep"`
	HostInfo       *HostSystem       `json:"hostInfo,omitempty"`
}

type NodePrep struct {
	Enabled            bool           `json:"enabled"`
	NFS                NodePrepStatus `json:"nfs,omitempty"`
	NFSStatusMessage   string         `json:"nfsStatusMessage,omitempty"`
	ISCSI              NodePrepStatus `json:"iscsi,omitempty"`
	ISCSIStatusMessage string         `json:"iscsiStatusMessage,omitempty"`
}

type NodePrepStatus string

type HostSystem struct {
	OS SystemOS `json:"os"`
}

type SystemOS struct {
	Distro  string `json:"distro"` // ubuntu/centos/rhel
	Version string `json:"version"`
	Release string `json:"release"`
}

type NodePrepBreadcrumb struct {
	TridentVersion string `json:"tridentVersion"`
	NFS            string `json:"nfs,omitempty"`
	ISCSI          string `json:"iscsi,omitempty"`
}
