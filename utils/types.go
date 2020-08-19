// Copyright 2020 NetApp, Inc. All Rights Reserved.

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

type Node struct {
	Name string   `json:"name"`
	IQN  string   `json:"iqn,omitempty"`
	IPs  []string `json:"ips,omitempty"`
}
