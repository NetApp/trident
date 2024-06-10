// Copyright 2024 NetApp, Inc. All Rights Reserved.

// Package gcnvapi provides a high-level interface to the Google Cloud NetApp Volumes SDK
package gcnvapi

import (
	"context"
	"time"

	"github.com/netapp/trident/storage"
)

//go:generate mockgen -package mock_api -destination=../../../mocks/mock_storage_drivers/mock_gcp/mock_gcnvapi.go github.com/netapp/trident/storage_drivers/gcp/gcnvapi GCNVAPI

type GCNV interface {
	Init(context.Context, map[string]storage.Pool) error

	RefreshGCNVResources(context.Context) error
	DiscoverGCNVResources(context.Context) error

	CapacityPools() *[]*CapacityPool
	CapacityPoolsForStoragePools(context.Context) []*CapacityPool
	CapacityPoolsForStoragePool(context.Context, storage.Pool, string) []*CapacityPool
	EnsureVolumeInValidCapacityPool(context.Context, *Volume) error

	Volumes(context.Context) (*[]*Volume, error)
	Volume(context.Context, *storage.VolumeConfig) (*Volume, error)
	VolumeByName(context.Context, string) (*Volume, error)
	VolumeExists(context.Context, *storage.VolumeConfig) (bool, *Volume, error)
	WaitForVolumeState(context.Context, *Volume, string, []string, time.Duration) (string, error)
	CreateVolume(context.Context, *VolumeCreateRequest) (*Volume, error)
	ModifyVolume(context.Context, *Volume, map[string]string, *string, *bool, *ExportRule) error
	ResizeVolume(context.Context, *Volume, int64) error
	DeleteVolume(context.Context, *Volume) error

	SnapshotsForVolume(context.Context, *Volume) (*[]*Snapshot, error)
	SnapshotForVolume(context.Context, *Volume, string) (*Snapshot, error)
	WaitForSnapshotState(context.Context, *Snapshot, *Volume, string, []string, time.Duration) error
	CreateSnapshot(context.Context, *Volume, string) (*Snapshot, error)
	RestoreSnapshot(context.Context, *Volume, *Snapshot) error
	DeleteSnapshot(context.Context, *Volume, *Snapshot) error
}
