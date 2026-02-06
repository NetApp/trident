// Copyright 2025 NetApp, Inc. All Rights Reserved.

// Package api provides a high-level interface to the Google Cloud NetApp Volumes SDK
package api

import (
	"context"
	"time"

	"github.com/netapp/trident/storage"
)

//go:generate mockgen -package mock_api -destination=../../../mocks/mock_storage_drivers/mock_gcp/mock_api.go github.com/netapp/trident/storage_drivers/gcp/api GCNV

type GCNV interface {
	Init(context.Context, map[string]storage.Pool) error

	RefreshGCNVResources(context.Context) error
	DiscoverGCNVResources(context.Context, GCNVResourceUpdater) error

	CapacityPools() *[]*CapacityPool
	CapacityPoolsForStoragePools(context.Context) []*CapacityPool
	CapacityPoolsForStoragePool(context.Context, storage.Pool, string, string) []*CapacityPool
	FilterCapacityPoolsOnTopology(context.Context, []*CapacityPool, []map[string]string, []map[string]string) []*CapacityPool
	EnsureVolumeInValidCapacityPool(context.Context, *Volume) error

	Volumes(context.Context) (*[]*Volume, error)
	Volume(context.Context, *storage.VolumeConfig) (*Volume, error)
	VolumeByName(context.Context, string) (*Volume, error)
	VolumeExists(context.Context, *storage.VolumeConfig) (bool, *Volume, error)
	VolumeExistsByName(context.Context, string) (bool, *Volume, error)
	VolumeByID(context.Context, string) (*Volume, error)
	VolumeExistsByID(context.Context, string) (bool, *Volume, error)
	VolumeByNameOrID(context.Context, string) (*Volume, error)
	WaitForVolumeState(context.Context, *Volume, string, []string, time.Duration) (string, error)
	CreateVolume(context.Context, *VolumeCreateRequest) (*Volume, error)
	UpdateNASVolume(context.Context, *Volume, map[string]string, *string, *bool, *ExportRule) error
	ResizeVolume(context.Context, *Volume, int64) error
	DeleteVolume(context.Context, *Volume) error

	SnapshotsForVolume(context.Context, *Volume) (*[]*Snapshot, error)
	SnapshotForVolume(context.Context, *Volume, string) (*Snapshot, error)
	CreateSnapshot(context.Context, *Volume, string, time.Duration) (*Snapshot, error)
	RestoreSnapshot(context.Context, *Volume, *Snapshot) error
	DeleteSnapshot(context.Context, *Volume, *Snapshot, time.Duration) error

	// SAN/Block storage methods
	ISCSITargetInfo(context.Context, *Volume) (*ISCSITargetInfo, error)
	UpdateSANVolume(context.Context, *Volume, *VolumeUpdateRequest) (*Volume, error)
	AddHostGroupToVolume(context.Context, string, string) error
	RemoveHostGroupFromVolume(context.Context, string, string) error
	VolumeMappedHostGroups(context.Context, string) ([]string, error)

	// Host group methods
	HostGroups(context.Context) ([]*HostGroup, error)
	HostGroupByName(context.Context, string) (*HostGroup, error)
	CreateHostGroup(context.Context, *HostGroupCreateRequest) (*HostGroup, error)
	UpdateHostGroup(context.Context, *HostGroup, []string) error
	DeleteHostGroup(context.Context, *HostGroup) error
	AddInitiatorsToHostGroup(context.Context, *HostGroup, []string) error
	RemoveInitiatorsFromHostGroup(context.Context, *HostGroup, []string) error
	HostGroupVolumes(context.Context, string) ([]string, error)
}

// GCNVResourceUpdater allows updating GCNV resources in a thread-safe manner.
type GCNVResourceUpdater func(time.Time, map[string]*CapacityPool)
