// Copyright 2022 NetApp, Inc. All Rights Reserved.

package nodehelpers

import (
	"context"
	"os"

	"github.com/netapp/trident/utils/models"
)

//go:generate mockgen -destination=../../../mocks/mock_frontend/mock_csi/mock_node_helpers/mock_node_helpers.go github.com/netapp/trident/frontend/csi/node_helpers NodeHelper,VolumePublishManager,VolumeStatsManager

const (
	KubernetesHelper = "k8s_csi_node_helper"
	PlainCSIHelper   = "plain_csi_node_helper"
)

type NodeHelper interface {
	AddPublishedPath(ctx context.Context, volumeID, pathToAdd string) error
	RemovePublishedPath(ctx context.Context, volumeID, pathToRemove string) error
	VolumePublishManager
	VolumeStatsManager
}

// VolumePublishManager is the common interface used by the "helper" objects used by
// the CSI node. The node_helpers supply CO-specific details at certain
// points of CSI workflows.
type VolumePublishManager interface {
	WriteTrackingInfo(context.Context, string, *models.VolumeTrackingInfo) error
	ReadTrackingInfo(context.Context, string) (*models.VolumeTrackingInfo, error)
	DeleteTrackingInfo(context.Context, string) error
	ListVolumeTrackingInfo(context.Context) (map[string]*models.VolumeTrackingInfo, error)
	UpgradeVolumeTrackingFile(context.Context, string, map[string]struct{}, map[string]string) (bool, error)
	DeleteFailedUpgradeTrackingFile(context.Context, os.FileInfo)
	ValidateTrackingFile(context.Context, string) (bool, error)
	GetVolumeTrackingFiles() ([]os.FileInfo, error)
}

// VolumeStatsManager provides volume statistics retrieval capabilities.
type VolumeStatsManager interface {
	// GetVolumeStatsByID retrieves stats for a volume by ID (auto-detects type)
	GetVolumeStatsByID(ctx context.Context, volumeID string) (*VolumeStats, error)

	// GetBlockDeviceStatsByID retrieves stats for a raw block volume
	GetBlockDeviceStatsByID(ctx context.Context, volumeID string, trackingInfo *models.VolumeTrackingInfo) (*VolumeStats, error)

	// GetFilesystemStatsByID retrieves stats for a filesystem volume (includes inodes)
	GetFilesystemStatsByID(ctx context.Context, volumePath string) (*VolumeStats, error)

	// VerifyVolumePath checks if path exists
	VerifyVolumePath(ctx context.Context, volumePath string) error

	// IsRawBlockVolume checks if the volume is a raw block volume based on tracking info
	IsRawBlockVolume(trackingInfo *models.VolumeTrackingInfo) bool
}

// VolumeStats represents volume statistics for both block and filesystem volumes.
type VolumeStats struct {
	// Byte statistics (populated for both block and filesystem)
	Total          int64
	Used           int64
	Available      int64
	UsedPercentage float32

	// Inode statistics (populated only for filesystem volumes, 0 for block)
	Inodes     int64
	InodesUsed int64
	InodesFree int64
}
