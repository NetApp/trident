// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"time"

	"github.com/netapp/trident/storage"
)

//go:generate mockgen -destination=../../../mocks/mock_storage_drivers/mock_azure/mock_api.go github.com/netapp/trident/storage_drivers/azure/api Azure

type Azure interface {
	Init(context.Context, map[string]storage.Pool) error

	RefreshAzureResources(context.Context) error
	DiscoverAzureResources(context.Context) error
	EnableAzureFeatures(context.Context, ...string) error
	Features() map[string]bool
	HasFeature(string) bool
	CapacityPools() *[]*CapacityPool
	CapacityPoolsForStoragePools(context.Context) []*CapacityPool
	CapacityPoolsForStoragePool(context.Context, storage.Pool, string) []*CapacityPool
	RandomCapacityPoolForStoragePool(context.Context, storage.Pool, string) *CapacityPool
	EnsureVolumeInValidCapacityPool(context.Context, *FileSystem) error
	SubnetsForStoragePool(context.Context, storage.Pool) []*Subnet
	RandomSubnetForStoragePool(context.Context, storage.Pool) *Subnet

	Volumes(context.Context) (*[]*FileSystem, error)
	Volume(context.Context, *storage.VolumeConfig) (*FileSystem, error)
	VolumeExists(context.Context, *storage.VolumeConfig) (bool, *FileSystem, error)
	VolumeByCreationToken(context.Context, string) (*FileSystem, error)
	VolumeExistsByCreationToken(context.Context, string) (bool, *FileSystem, error)
	VolumeByID(context.Context, string) (*FileSystem, error)
	VolumeExistsByID(context.Context, string) (bool, *FileSystem, error)
	WaitForVolumeState(context.Context, *FileSystem, string, []string, time.Duration) (string, error)
	CreateVolume(context.Context, *FilesystemCreateRequest) (*FileSystem, error)
	ModifyVolume(context.Context, *FileSystem, map[string]string, *string) error
	ResizeVolume(context.Context, *FileSystem, int64) error
	DeleteVolume(context.Context, *FileSystem) error

	Subvolumes(context.Context, []string) (*[]*Subvolume, error)
	Subvolume(context.Context, *storage.VolumeConfig, bool) (*Subvolume, error)
	SubvolumeExists(context.Context, *storage.VolumeConfig, []string) (bool, *Subvolume, error)
	SubvolumeByCreationToken(context.Context, string, []string, bool) (*Subvolume, error)
	SubvolumeExistsByCreationToken(context.Context, string, []string) (bool, *Subvolume, error)
	SubvolumeByID(context.Context, string, bool) (*Subvolume, error)
	SubvolumeExistsByID(context.Context, string) (bool, *Subvolume, error)
	SubvolumeParentVolume(context.Context, *storage.VolumeConfig) (*FileSystem, error)
	WaitForSubvolumeState(context.Context, *Subvolume, string, []string, time.Duration) (string, error)
	CreateSubvolume(context.Context, *SubvolumeCreateRequest) (*Subvolume, PollerResponse, error)
	ResizeSubvolume(context.Context, *Subvolume, int64) error
	DeleteSubvolume(context.Context, *Subvolume) (PollerResponse, error)
	ValidateFilePoolVolumes(context.Context, []string) ([]*FileSystem, error)

	SnapshotsForVolume(context.Context, *FileSystem) (*[]*Snapshot, error)
	SnapshotForVolume(context.Context, *FileSystem, string) (*Snapshot, error)
	WaitForSnapshotState(context.Context, *Snapshot, *FileSystem, string, []string, time.Duration) error
	CreateSnapshot(context.Context, *FileSystem, string) (*Snapshot, error)
	DeleteSnapshot(context.Context, *FileSystem, *Snapshot) error
}
