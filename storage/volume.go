// Copyright 2016 NetApp, Inc. All Rights Reserved.

package storage

import (
	"fmt"
	"strings"

	"github.com/netapp/trident/config"
)

type VolumeConfig struct {
	Version         string            `json:"version"`
	Name            string            `json:"name"`
	InternalName    string            `json:"internalName"`
	Size            string            `json:"size"`
	Protocol        config.Protocol   `json:"protocol"`
	SnapshotPolicy  string            `json:"snapshotPolicy,omitempty"`
	ExportPolicy    string            `json:"exportPolicy,omitempty"`
	SnapshotDir     string            `json:"snapshotDirectory,omitempty"`
	UnixPermissions string            `json:"unixPermissions,omitempty"`
	StorageClass    string            `json:"storageClass,omitempty"`
	AccessMode      config.AccessMode `json:"accessMode,omitempty"`
	AccessInfo      VolumeAccessInfo  `json:"accessInformation"`
}

type VolumeAccessInfo struct {
	IscsiAccessInfo
	NfsAccessInfo
}

type IscsiAccessInfo struct {
	IscsiTargetPortal string `json:"iscsiTargetPortal,omitempty"`
	IscsiTargetIQN    string `json:"iscsiTargetIqn,omitempty"`
	IscsiLunNumber    int32  `json:"iscsiLunNumber,omitempty"`
	IscsiInterface    string `json:"iscsiInterface,omitempty"`
	IscsiIgroup       string `json:"iscsiIgroup,omitempty"`
	IscsiVAG          int64  `json:"iscsiVag,omitempty"`
}

type NfsAccessInfo struct {
	NfsServerIP string `json:"nfsServerIp,omitempty"`
	NfsPath     string `json:"nfsPath,omitempty"`
}

func (c *VolumeConfig) Validate() error {
	if c.Name == "" || c.Size == "" {
		return fmt.Errorf("The following fields for \"Volume\" are mandatory: name and size")
	}
	if !config.IsValidProtocol(c.Protocol) {
		return fmt.Errorf("%v is an usupported protocol! Acceptable values:  "+
			"%s", c.Protocol,
			strings.Join([]string(config.GetValidProtocolNames()), ", "),
		)
	}
	return nil
}

type Volume struct {
	Config  *VolumeConfig
	Backend *StorageBackend
	Pool    *StoragePool
}

func NewVolume(conf *VolumeConfig, backend *StorageBackend, pool *StoragePool) *Volume {
	return &Volume{
		Config:  conf,
		Backend: backend,
		Pool:    pool,
	}
}

type VolumeExternal struct {
	Config  *VolumeConfig
	Backend string `json:"backend"`
	Pool    string `json:"pool"`
}

func (v *Volume) ConstructExternal() *VolumeExternal {
	return &VolumeExternal{
		Config:  v.Config,
		Backend: v.Backend.Name,
		Pool:    v.Pool.Name,
	}
}
