// Copyright 2025 NetApp, Inc. All Rights Reserved.

// Package api provides a high-level interface to the Google Cloud NetApp Volumes SDK
package api

import (
	"time"

	"github.com/netapp/trident/storage"
)

const (
	ProtocolTypeUnknown   = "Unknown"
	ProtocolTypeNFSPrefix = "NFSv"
	ProtocolTypeNFSv3     = ProtocolTypeNFSPrefix + "3"
	ProtocolTypeNFSv41    = ProtocolTypeNFSPrefix + "4.1"
	ProtocolTypeSMB       = "SMB"

	ServiceLevelUnspecified = "Unspecified"
	ServiceLevelFlex        = "Flex"
	ServiceLevelStandard    = "Standard"
	ServiceLevelPremium     = "Premium"
	ServiceLevelExtreme     = "Extreme"

	StoragePoolStateUnspecified = "Unspecified"
	StoragePoolStateReady       = "Ready"
	StoragePoolStateCreating    = "Creating"
	StoragePoolStateDeleting    = "Deleting"
	StoragePoolStateDeleted     = "NoSuchState"
	StoragePoolStateUpdating    = "Updating"
	StoragePoolStateRestoring   = "Restoring"
	StoragePoolStateDisabled    = "Disabled"
	StoragePoolStateError       = "Error"

	VolumeStateUnspecified = "Unspecified"
	VolumeStateReady       = "Ready"
	VolumeStateCreating    = "Creating"
	VolumeStateDeleting    = "Deleting"
	VolumeStateDeleted     = "NoSuchState"
	VolumeStateUpdating    = "Updating"
	VolumeStateRestoring   = "Restoring"
	VolumeStateDisabled    = "Disabled"
	VolumeStateError       = "Error"

	SnapshotStateUnspecified = "Unspecified"
	SnapshotStateReady       = "Ready"
	SnapshotStateCreating    = "Creating"
	SnapshotStateDeleting    = "Deleting"
	SnapshotStateDeleted     = "NoSuchState"
	SnapshotStateUpdating    = "Updating"
	SnapshotStateDisabled    = "Disabled"
	SnapshotStateError       = "Error"

	SecurityStyleUnspecified = "Unspecified"
	SecurityStyleNTFS        = "NTFS"
	SecurityStyleUnix        = "Unix"

	AccessTypeUnspecified = "Unspecified"
	AccessTypeReadOnly    = "ReadOnly"
	AccessTypeReadWrite   = "ReadWrite"
	AccessTypeReadNone    = "ReadNone"
)

// GCNVResources is the toplevel cache for the set of things we discover about our GCNV environment.
type GCNVResources struct {
	CapacityPoolMap map[string]*CapacityPool
	StoragePoolMap  map[string]storage.Pool
	lastUpdateTime  time.Time
}

// CapacityPool records details of a discovered GCNV storage pool.
type CapacityPool struct {
	Name            string
	FullName        string
	Location        string
	ServiceLevel    string
	State           string
	NetworkName     string
	NetworkFullName string
	Zone            string
}

// Volume records details of a discovered GCNV volume.
type Volume struct {
	Name              string
	CreationToken     string
	FullName          string
	Location          string
	State             string
	CapacityPool      string
	NetworkName       string
	NetworkFullName   string
	ServiceLevel      string
	SizeBytes         int64
	ExportPolicy      *ExportPolicy
	ProtocolTypes     []string
	MountTargets      []MountTarget
	UnixPermissions   string
	Labels            map[string]string
	SnapshotReserve   int64
	SnapshotDirectory bool
	SecurityStyle     string
}

// VolumeCreateRequest embodies all the details of a volume to be created.
type VolumeCreateRequest struct {
	Name              string
	CreationToken     string
	CapacityPool      string
	SizeBytes         int64
	ExportPolicy      *ExportPolicy
	ProtocolTypes     []string
	UnixPermissions   string
	Labels            map[string]string
	SnapshotReserve   *int64
	SnapshotDirectory bool
	SecurityStyle     string
	SnapshotID        string
}

// ExportPolicy records details of a discovered GCNV volume export policy.
type ExportPolicy struct {
	Rules []ExportRule
}

// ExportRule records details of a discovered GCNV volume export policy rule.
type ExportRule struct {
	AllowedClients string
	SMB            bool
	Nfsv3          bool
	Nfsv4          bool
	RuleIndex      int32
	AccessType     string
}

// MountTarget records details of a discovered GCNV volume mount target.
type MountTarget struct {
	Export     string
	ExportPath string
	Protocol   string
}

// Snapshot records details of a discovered GCNV snapshot.
type Snapshot struct {
	Name     string
	FullName string
	Volume   string
	Location string
	State    string
	Created  time.Time
	Labels   map[string]string
}
