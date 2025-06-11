// Copyright 2024 NetApp, Inc. All Rights Reserved.

package storage

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/brunoga/deep"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/utils/models"
)

type VolumeConfig struct {
	Version                     string                  `json:"version"`
	Name                        string                  `json:"name"`
	InternalName                string                  `json:"internalName"`
	Size                        string                  `json:"size"`
	Protocol                    config.Protocol         `json:"protocol"`
	SpaceReserve                string                  `json:"spaceReserve"`
	SecurityStyle               string                  `json:"securityStyle"`
	SnapshotPolicy              string                  `json:"snapshotPolicy,omitempty"`
	SnapshotReserve             string                  `json:"snapshotReserve,omitempty"`
	SnapshotDir                 string                  `json:"snapshotDirectory,omitempty"`
	ExportPolicy                string                  `json:"exportPolicy,omitempty"`
	UnixPermissions             string                  `json:"unixPermissions,omitempty"`
	StorageClass                string                  `json:"storageClass,omitempty"`
	AccessMode                  config.AccessMode       `json:"accessMode,omitempty"`
	VolumeMode                  config.VolumeMode       `json:"volumeMode,omitempty"`
	AccessInfo                  models.VolumeAccessInfo `json:"accessInformation"`
	BlockSize                   string                  `json:"blockSize"`
	FileSystem                  string                  `json:"fileSystem"`
	FormatOptions               string                  `json:"formatOptions,omitempty"`
	Encryption                  string                  `json:"encryption"`
	LUKSEncryption              string                  `json:"LUKSEncryption,omitempty"`
	CloneSourceVolume           string                  `json:"cloneSourceVolume"`
	CloneSourceVolumeInternal   string                  `json:"cloneSourceVolumeInternal"`
	CloneSourceSnapshot         string                  `json:"cloneSourceSnapshot"`
	CloneSourceSnapshotInternal string                  `json:"cloneSourceSnapshotInternal"`
	SplitOnClone                string                  `json:"splitOnClone"`
	ReadOnlyClone               bool                    `json:"readOnlyClone"`
	QosPolicy                   string                  `json:"qosPolicy,omitempty"`
	AdaptiveQosPolicy           string                  `json:"adaptiveQosPolicy,omitempty"`
	Qos                         string                  `json:"qos,omitempty"`
	QosType                     string                  `json:"type,omitempty"`
	ServiceLevel                string                  `json:"serviceLevel,omitempty"`
	CVSStorageClass             string                  `json:"cvsStorageClass,omitempty"`
	Network                     string                  `json:"network,omitempty"`
	Zone                        string                  `json:"zone,omitempty"`
	ImportOriginalName          string                  `json:"importOriginalName,omitempty"`
	ImportBackendUUID           string                  `json:"importBackendUUID,omitempty"`
	ImportNotManaged            bool                    `json:"importNotManaged,omitempty"`
	MountOptions                string                  `json:"mountOptions,omitempty"`
	RequisiteTopologies         []map[string]string     `json:"requisiteTopologies,omitempty"`
	PreferredTopologies         []map[string]string     `json:"preferredTopologies,omitempty"`
	AllowedTopologies           []map[string]string     `json:"allowedTopologies,omitempty"`
	LUKSPassphraseNames         []string                `json:"luksPassphraseNames,omitempty"`
	// IsMirrorDestination is whether the volume is currently the destination in a mirror relationship
	IsMirrorDestination bool `json:"mirrorDestination,omitempty"`
	// PeerVolumeHandle is the internal volume handle for the source volume if this volume is a mirror destination
	PeerVolumeHandle string `json:"requiredPeerVolumeHandle,omitempty"`
	// InternalID is an optional, backend-specific identifier to help find an object
	InternalID         string                 `json:"internalID,omitempty"`
	ShareSourceVolume  string                 `json:"shareSourceVolume"`
	SubordinateVolumes map[string]interface{} `json:"-"`
	Namespace          string                 `json:"namespace"`
	RequestName        string                 `json:"requestName"`
	SkipRecoveryQueue  string                 `json:"skipRecoveryQueue"`
	SMBShareACL        map[string]string      `json:"smbShareACL,omitempty"`
	// SecureSMBEnabled indicates whether the volume is to be created with secure SMB enabled
	SecureSMBEnabled bool `json:"secureSMBEnabled,omitempty"`
}

type VolumeCreatingConfig struct {
	StartTime   time.Time `json:"startTime"`   // Time this create operation began
	BackendUUID string    `json:"backendUUID"` // UUID of the storage backend
	Pool        string    `json:"pool"`        // Name of the pool on which this volume was first provisioned
	VolumeConfig
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
	clone, err := deep.Copy(c)
	if err != nil {
		return &VolumeConfig{}
	}
	return clone
}

type Volume struct {
	Config      *VolumeConfig
	BackendUUID string // UUID of the storage backend
	Pool        string // Name of the pool on which this volume was first provisioned
	Orphaned    bool   // An Orphaned volume isn't currently tracked by the storage backend
	State       VolumeState
}

type VolumeState string

const (
	VolumeStateUnknown        = VolumeState("unknown")
	VolumeStateOnline         = VolumeState("online")
	VolumeStateDeleting       = VolumeState("deleting")
	VolumeStateUpgrading      = VolumeState("upgrading")
	VolumeStateMissingBackend = VolumeState("missing_backend")
	VolumeStateSubordinate    = VolumeState("subordinate")
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

func (s VolumeState) IsMissingBackend() bool {
	return s == VolumeStateMissingBackend
}

func (s VolumeState) IsSubordinate() bool {
	return s == VolumeStateSubordinate
}

func NewVolume(conf *VolumeConfig, backendUUID, pool string, orphaned bool, state VolumeState) *Volume {
	return &Volume{
		Config:      conf,
		BackendUUID: backendUUID,
		Pool:        pool,
		Orphaned:    orphaned,
		State:       state,
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

func (v *Volume) SmartCopy() interface{} {
	return deep.MustCopy(v)
}

func (v *Volume) IsDeleting() bool {
	return v.State.IsDeleting()
}

func (v *Volume) IsSubordinate() bool {
	return v.State.IsSubordinate()
}

func (v *Volume) GetBackendID() string {
	return v.BackendUUID
}

func (v *Volume) GetVolumeID() string {
	return v.Config.ShareSourceVolume
}

func (v *Volume) GetUniqueKey() string {
	return v.Config.InternalName
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

type UpgradeVolumeRequest struct {
	Type   string `json:"type"`
	Volume string `json:"volume"`
}

func (r *UpgradeVolumeRequest) Validate() error {
	if r.Volume == "" {
		return fmt.Errorf("the following field is mandatory: volume")
	}
	if r.Type != "csi" {
		return fmt.Errorf("the only supported type for volume upgrade is 'csi'")
	}
	return nil
}

type PatchRequestStringSlice struct {
	Op    string   `json:"op"`
	Path  string   `json:"path"`
	Value []string `json:"value"`
}

type ByVolumeExternalName []*VolumeExternal

func (a ByVolumeExternalName) Len() int           { return len(a) }
func (a ByVolumeExternalName) Less(i, j int) bool { return a[i].Config.Name < a[j].Config.Name }
func (a ByVolumeExternalName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
