// Copyright 2018 NetApp, Inc. All Rights Reserved.

package storage

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"strings"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/utils"
)

type VolumeConfig struct {
	Version                   string                 `json:"version"`
	Name                      string                 `json:"name"`
	InternalName              string                 `json:"internalName"`
	Size                      string                 `json:"size"`
	Protocol                  config.Protocol        `json:"protocol"`
	SpaceReserve              string                 `json:"spaceReserve"`
	SecurityStyle             string                 `json:"securityStyle"`
	SnapshotPolicy            string                 `json:"snapshotPolicy,omitempty"`
	SnapshotReserve           string                 `json:"snapshotReserve,omitempty"`
	SnapshotDir               string                 `json:"snapshotDirectory,omitempty"`
	ExportPolicy              string                 `json:"exportPolicy,omitempty"`
	UnixPermissions           string                 `json:"unixPermissions,omitempty"`
	StorageClass              string                 `json:"storageClass,omitempty"`
	AccessMode                config.AccessMode      `json:"accessMode,omitempty"`
	AccessInfo                utils.VolumeAccessInfo `json:"accessInformation"`
	BlockSize                 string                 `json:"blockSize"`
	FileSystem                string                 `json:"fileSystem"`
	Encryption                string                 `json:"encryption"`
	CloneSourceVolume         string                 `json:"cloneSourceVolume"`
	CloneSourceVolumeInternal string                 `json:"cloneSourceVolumeInternal"`
	CloneSourceSnapshot       string                 `json:"cloneSourceSnapshot"`
	SplitOnClone              string                 `json:"splitOnClone"`
	QoS                       string                 `json:"qos,omitempty"`
	QoSType                   string                 `json:"type,omitempty"`
	ServiceLevel              string                 `json:"serviceLevel,omitempty"`
	ImportOriginalName        string                 `json:"importOriginalName,omitempty"`
}

func (c *VolumeConfig) Validate() error {
	if c.Name == "" || c.Size == "" {
		return fmt.Errorf("the following fields for \"Volume\" are mandatory: name and size")
	}
	if !config.IsValidProtocol(c.Protocol) {
		return fmt.Errorf("%v is an usupported protocol! Acceptable values:  "+
			"%s", c.Protocol,
			strings.Join([]string(config.GetValidProtocolNames()), ", "),
		)
	}
	return nil
}

func (c *VolumeConfig) ConstructClone() *VolumeConfig {
	clone := &VolumeConfig{}
	buff := new(bytes.Buffer)
	enc := gob.NewEncoder(buff)
	dec := gob.NewDecoder(buff)
	_ = enc.Encode(c)
	_ = dec.Decode(clone)
	return clone
}

type Volume struct {
	Config      *VolumeConfig
	BackendUUID string // UUID of the storage backend
	Pool        string // Name of the pool on which this volume was first provisioned
	Orphaned    bool   // An Orphaned volume isn't currently tracked by the storage backend
	State       VolumeState
}

type UpdateVolumeStateRequest struct {
	State string `json:"state"`
}

type VolumeState string

const (
	VolumeStateUnknown  = VolumeState("unknown")
	VolumeStateOnline   = VolumeState("online")
	VolumeStateDeleting = VolumeState("deleting")
	// TODO should Orphaned be moved to a VolumeState?
)

func (s VolumeState) String() string {
	switch s {
	case VolumeStateUnknown, VolumeStateOnline, VolumeStateDeleting:
		return string(s)
	default:
		return "unknown"
	}
}

func (s VolumeState) IsUnknown() bool {
	switch s {
	case VolumeStateOnline, VolumeStateDeleting:
		return false
	case VolumeStateUnknown:
		return true
	default:
		return true
	}
}

func (s VolumeState) IsOnline() bool {
	return s == VolumeStateOnline
}

func (s VolumeState) IsDeleting() bool {
	return s == VolumeStateDeleting
}

func NewVolume(conf *VolumeConfig, backendUUID string, pool string, orphaned bool) *Volume {
	return &Volume{
		Config:      conf,
		BackendUUID: backendUUID,
		Pool:        pool,
		Orphaned:    orphaned,
		State:       VolumeStateOnline,
	}
}

type VolumeExternal struct {
	Config      *VolumeConfig
	Backend     string      `json:"backend"`     // replaced w/ backendUUID, remains to read old records
	BackendUUID string      `json:"backendUUID"` // UUID of the storage backend
	Pool        string      `json:"pool"`
	Orphaned    bool        `json:"orphaned"`
	State       VolumeState `json:"state"`
}

func (v *VolumeExternal) GetCHAPSecretName() string {
	secretName := fmt.Sprintf("trident-chap-%v-%v", v.BackendUUID, v.Config.AccessInfo.IscsiUsername)
	secretName = strings.Replace(secretName, "_", "-", -1)
	secretName = strings.Replace(secretName, ".", "-", -1)
	secretName = strings.ToLower(secretName)
	return secretName
}

func (v *Volume) ConstructExternal() *VolumeExternal {
	return &VolumeExternal{
		Config:      v.Config,
		BackendUUID: v.BackendUUID,
		Pool:        v.Pool,
		Orphaned:    v.Orphaned,
		State:       v.State,
	}
}

// VolumeExternalWrapper is used to return volumes and errors via channels between goroutines
type VolumeExternalWrapper struct {
	Volume *VolumeExternal
	Error  error
}

type ImportVolumeRequest struct {
	Backend      string `json:"backend"`
	InternalName string `json:"internalName"`
	NoManage     bool   `json:"noManage"`
	PVCData      string `json:"pvcData"` // Opaque, base64-encoded
}

func (r *ImportVolumeRequest) Validate() error {
	if r.Backend == "" || r.InternalName == "" {
		return fmt.Errorf("the following fields are mandatory: backend and internalName")
	}
	if _, err := base64.StdEncoding.DecodeString(r.PVCData); err != nil {
		return fmt.Errorf("the pvcData field does not contain valid base64-encoded data: %v", err)
	}
	return nil
}
