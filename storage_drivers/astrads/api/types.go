// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"time"

	"github.com/RoaringBitmap/roaring"
)

//go:generate mockgen -destination=../../../mocks/mock_storage_drivers/mock_astrads/mock_api.go github.com/netapp/trident/storage_drivers/astrads/api AstraDS

type AstraDS interface {
	Init(context.Context, string, string, string) (*Cluster, string, error)

	Volumes(context.Context) ([]*Volume, error)
	Volume(context.Context, string) (*Volume, error)
	VolumeExists(context.Context, string) (bool, *Volume, error)
	CreateVolume(context.Context, *Volume) (*Volume, error)
	SetVolumeAttributes(context.Context, *Volume, *roaring.Bitmap) error
	DeleteVolume(context.Context, *Volume) error
	WaitForVolumeReady(context.Context, *Volume, time.Duration) error
	WaitForVolumeDeleted(context.Context, *Volume, time.Duration) error
	WaitForVolumeResize(context.Context, *Volume, int64, time.Duration) error

	ExportPolicyExists(context.Context, string) (bool, *ExportPolicy, error)
	EnsureExportPolicyExists(context.Context, string) (*ExportPolicy, error)
	SetExportPolicyAttributes(context.Context, *ExportPolicy, *roaring.Bitmap) error
	DeleteExportPolicy(context.Context, string) error

	Snapshots(context.Context, *Volume) ([]*Snapshot, error)
	Snapshot(context.Context, string) (*Snapshot, error)
	SnapshotExists(context.Context, string) (bool, *Snapshot, error)
	CreateSnapshot(context.Context, *Snapshot) (*Snapshot, error)
	SetSnapshotAttributes(context.Context, *Snapshot, *roaring.Bitmap) error
	DeleteSnapshot(context.Context, *Snapshot) error
	WaitForSnapshotReady(context.Context, *Snapshot, time.Duration) error
	WaitForSnapshotDeleted(context.Context, *Snapshot, time.Duration) error

	QosPolicies(ctx context.Context) ([]*QosPolicy, error)
}
