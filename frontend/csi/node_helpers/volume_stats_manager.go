// Copyright 2025 NetApp, Inc. All Rights Reserved.

package nodehelpers

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/netapp/trident/logging"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/blockdevice"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount"
	"github.com/netapp/trident/utils/osutils"
)

const (
	fsUnavailableTimeout = 5 * time.Second
)

// volumeStatsManager handles volume statistics operations for CSI node.
type volumeStatsManager struct {
	volumePublishManager VolumePublishManager
	osutils              osutils.Utils
	fs                   filesystem.Filesystem
	bd                   blockdevice.BlockDevice
}

// NewVolumeStatsManager creates a new VolumeStatsManager instance.
// Utilities (osutils, filesystem, blockdevice) are initialized internally.
func NewVolumeStatsManager(volumePublishManager VolumePublishManager) (VolumeStatsManager, error) {
	ctx := context.Background()
	Logc(ctx).Debug(">>>> NewVolumeStatsManager")
	defer Logc(ctx).Debug("<<<< NewVolumeStatsManager")

	// Initialize mount client for filesystem operations
	mountClient, err := mount.New()
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to initialize mount client.")
		return nil, fmt.Errorf("could not initialize mount client: %w", err)
	}

	return &volumeStatsManager{
		volumePublishManager: volumePublishManager,
		osutils:              osutils.New(),
		fs:                   filesystem.New(mountClient),
		bd:                   blockdevice.New(),
	}, nil
}

// VerifyVolumePath ensures the volume is published at the specified path.
func (v *volumeStatsManager) VerifyVolumePath(ctx context.Context, volumePath string) error {
	Logc(ctx).WithField("volumePath", volumePath).Debug(">>>> VerifyVolumePath")
	defer Logc(ctx).WithField("volumePath", volumePath).Debug("<<<< VerifyVolumePath")

	exists, err := v.osutils.PathExistsWithTimeout(ctx, volumePath, fsUnavailableTimeout)
	if !exists || err != nil {
		return fmt.Errorf("could not find volume mount at path: %s; %w", volumePath, err)
	}
	return nil
}

// isRawBlockVolume checks if the volume is a raw block volume based on tracking info.
func (v *volumeStatsManager) IsRawBlockVolume(trackingInfo *models.VolumeTrackingInfo) bool {
	ctx := context.Background()
	Logc(ctx).Debug(">>>> IsRawBlockVolume")
	defer Logc(ctx).Debug("<<<< IsRawBlockVolume")

	if trackingInfo == nil {
		Logc(ctx).Debug("Tracking info is nil. Thus, assuming it is not a raw block volume.")
		return false
	}

	isRawBlock := trackingInfo.VolumePublishInfo.FilesystemType == filesystem.Raw

	fields := LogFields{
		"stagingTargetPath": trackingInfo.StagingTargetPath,
		"devicePath":        trackingInfo.VolumePublishInfo.DevicePath,
		"sanType":           trackingInfo.VolumePublishInfo.SANType,
		"filesystemType":    trackingInfo.VolumePublishInfo.FilesystemType,
		"isRawBlock":        isRawBlock,
	}
	Logc(ctx).WithFields(fields).Debug("Successfully determined if volume is a raw block volume.")

	return isRawBlock
}

// GetBlockDeviceStatsByID retrieves volume statistics for raw block devices.
func (v *volumeStatsManager) GetBlockDeviceStatsByID(
	ctx context.Context, volumeID string, trackingInfo *models.VolumeTrackingInfo,
) (*VolumeStats, error) {
	fields := LogFields{
		"volumeID": volumeID,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> GetBlockDeviceStatsByID")
	defer Logc(ctx).WithFields(fields).Debug("<<<< GetBlockDeviceStatsByID")

	if trackingInfo == nil {
		Logc(ctx).WithFields(fields).Error("Tracking info is nil for volume")
		return nil, fmt.Errorf("tracking info is nil for volume %s", volumeID)
	}

	publishInfo := &trackingInfo.VolumePublishInfo

	// Determine device path
	devicePath := publishInfo.DevicePath
	if devicePath == "" {
		devicePath = publishInfo.RawDevicePath
	}

	if devicePath == "" {
		Logc(ctx).WithFields(fields).Error("Unable to determine device path for raw block volume.")
		return nil, fmt.Errorf("unable to determine device path for raw block volume: %s", volumeID)
	}

	// Determine protocol
	protocol := v.determineProtocol(publishInfo.SANType)

	// Get block device statistics
	used, available, capacity, err := v.bd.GetBlockDeviceStats(ctx, devicePath, protocol)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeID":   volumeID,
			"devicePath": devicePath,
			"protocol":   protocol,
		}).WithError(err).Error("Failed to get block device statistics.")
		return nil, fmt.Errorf("failed to get block device volume statistics for %s: %w", volumeID, err)
	}

	// Calculate used percentage from total capacity for raw block volumes
	var usedPercentage float32
	if capacity > 0 {
		usedPercentage = (float32(used) / float32(capacity)) * 100
	}

	stats := &VolumeStats{
		Total:          capacity,
		Used:           used,
		Available:      available,
		UsedPercentage: usedPercentage,

		// Block devices do not have inodes; thus set inodes to 0
		Inodes:     0,
		InodesUsed: 0,
		InodesFree: 0,
	}

	// Log detailed statistics
	v.logBlockDeviceStats(ctx, volumeID, devicePath, protocol, stats)

	return stats, nil
}

// determineProtocol determines the protocol from SANType, normalizing to expected values.
func (v *volumeStatsManager) determineProtocol(sanType string) string {
	ctx := context.Background()
	Logc(ctx).WithField("sanType", sanType).Debug(">>>> determineProtocol")
	defer Logc(ctx).WithField("sanType", sanType).Debug("<<<< determineProtocol")

	protocol := strings.ToLower(sanType)
	if protocol != sa.ISCSI && protocol != sa.NVMe && protocol != sa.FCP {
		return "" // Let blockdevice auto-detect
	}
	return protocol
}

// logBlockDeviceStats logs detailed block device statistics.
func (v *volumeStatsManager) logBlockDeviceStats(ctx context.Context, volumeID, devicePath, protocol string, stats *VolumeStats) {
	fields := LogFields{
		"volumeID":   volumeID,
		"devicePath": devicePath,
		"protocol":   protocol,
	}

	Logc(ctx).WithFields(fields).Debug(">>>> logBlockDeviceStats")
	defer Logc(ctx).WithFields(fields).Debug("<<<< logBlockDeviceStats")

	// For pointer parameters (stats), check for nil and log key fields
	if stats == nil {
		Logc(ctx).WithFields(fields).Warn("Block device statistics are nil.")
		return
	}

	Logc(ctx).WithFields(LogFields{
		"volumeID":       volumeID,
		"devicePath":     devicePath,
		"protocol":       protocol,
		"usedBytes":      stats.Used,
		"usedMiB":        stats.Used / (1024 * 1024),
		"usedGiB":        stats.Used / (1024 * 1024 * 1024),
		"capacityBytes":  stats.Total,
		"capacityMiB":    stats.Total / (1024 * 1024),
		"capacityGiB":    stats.Total / (1024 * 1024 * 1024),
		"availableBytes": stats.Available,
		"availableMiB":   stats.Available / (1024 * 1024),
		"availableGiB":   stats.Available / (1024 * 1024 * 1024),
		"usedPercentage": stats.UsedPercentage,
	}).Debug("Block device statistics")
}

// GetFilesystemStatsByID retrieves volume statistics for filesystem volumes.
func (v *volumeStatsManager) GetFilesystemStatsByID(
	ctx context.Context, volumePath string,
) (*VolumeStats, error) {
	fields := LogFields{
		"volumePath": volumePath,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> GetFilesystemStatsByID")
	defer Logc(ctx).WithFields(fields).Debug("<<<< GetFilesystemStatsByID")

	available, capacity, used, inodes, inodesFree, inodesUsed, err := v.fs.GetFilesystemStats(ctx, volumePath)
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Failed to get filesystem statistics.")
		return nil, fmt.Errorf("failed to get filesystem stats at path %s: %w", volumePath, err)
	}

	// Calculate used percentage
	// Note: Used percentage is calculated as (used / (used + available)) * 100, rather than (used / capacity) * 100
	// to match used percentage calculation done by 'df -h'. Capacity includes root-reserved space (typically 5%)
	// which df excludes from its percentage calculation, counting only space available to non-root users.
	var usedPercentage float32
	if (used + available) > 0 {
		usedPercentage = (float32(used) / float32(used+available)) * 100
	}

	stats := &VolumeStats{
		Total:          capacity,
		Used:           used,
		Available:      available,
		UsedPercentage: usedPercentage,
		Inodes:         inodes,
		InodesUsed:     inodesUsed,
		InodesFree:     inodesFree,
	}

	// Log detailed statistics
	v.logFilesystemStats(ctx, volumePath, stats)

	return stats, nil
}

// logFilesystemStats logs detailed filesystem statistics.
func (v *volumeStatsManager) logFilesystemStats(ctx context.Context, volumePath string, stats *VolumeStats) {
	fields := LogFields{
		"volumePath": volumePath,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> logFilesystemStats")
	defer Logc(ctx).WithFields(fields).Debug("<<<< logFilesystemStats")

	// For pointer parameters (stats), check for nil and log key fields
	if stats == nil {
		Logc(ctx).WithFields(fields).Warn("Filesystem statistics are nil.")
		return
	}

	Logc(ctx).WithFields(LogFields{
		"volumePath":     volumePath,
		"usedBytes":      stats.Used,
		"usedMiB":        stats.Used / (1024 * 1024),
		"usedGiB":        stats.Used / (1024 * 1024 * 1024),
		"availableBytes": stats.Available,
		"availableMiB":   stats.Available / (1024 * 1024),
		"availableGiB":   stats.Available / (1024 * 1024 * 1024),
		"capacityBytes":  stats.Total,
		"capacityMiB":    stats.Total / (1024 * 1024),
		"capacityGiB":    stats.Total / (1024 * 1024 * 1024),
		"inodes":         stats.Inodes,
		"inodesUsed":     stats.InodesUsed,
		"inodesFree":     stats.InodesFree,
		"usedPercentage": stats.UsedPercentage,
	}).Debug("Filesystem statistics")
}

// GetVolumeStatsByID retrieves volume statistics for a given volumeID.
// This method internally determines if the volume is raw block or filesystem
// and returns the appropriate statistics.
func (v *volumeStatsManager) GetVolumeStatsByID(ctx context.Context, volumeID string) (*VolumeStats, error) {
	fields := LogFields{
		"volumeID": volumeID,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> GetVolumeStatsByID")
	defer Logc(ctx).WithFields(fields).Debug("<<<< GetVolumeStatsByID")

	// Read tracking info to determine volume type and path
	trackingInfo, err := v.volumePublishManager.ReadTrackingInfo(ctx, volumeID)
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Failed to read tracking info.")
		return nil, fmt.Errorf("error reading tracking file for volume %s: %w", volumeID, err)
	}

	// Handle raw block volumes
	if v.IsRawBlockVolume(trackingInfo) {
		return v.GetBlockDeviceStatsByID(ctx, volumeID, trackingInfo)
	}

	// Handle filesystem volumes - get volume path from tracking info
	volumePath := v.getVolumePathFromTrackingInfo(trackingInfo)
	if volumePath == "" {
		Logc(ctx).WithFields(fields).Error("Unable to determine volume path.")
		return nil, fmt.Errorf("unable to determine volume path for volumeID: %s", volumeID)
	}

	return v.GetFilesystemStatsByID(ctx, volumePath)
}

// getVolumePathFromTrackingInfo extracts the volume path from tracking info.
// It tries published paths first, then falls back to staging target path.
func (v *volumeStatsManager) getVolumePathFromTrackingInfo(trackingInfo *models.VolumeTrackingInfo) string {
	ctx := context.Background()
	Logc(ctx).Debug(">>>> getVolumePathFromTrackingInfo")
	defer Logc(ctx).Debug("<<<< getVolumePathFromTrackingInfo")

	if trackingInfo == nil {
		Logc(ctx).Debug("Tracking info is nil.")
		return ""
	}

	// Try to get the first published path
	var selectedPath string
	publishedPathsList := make([]string, 0, len(trackingInfo.PublishedPaths))

	for path := range trackingInfo.PublishedPaths {
		publishedPathsList = append(publishedPathsList, path)
		// Select the first path we encounter
		if selectedPath == "" {
			selectedPath = path
		}
	}

	// Log all published paths and staging target path for visibility
	Logc(ctx).WithFields(LogFields{
		"publishedPathsCount": len(trackingInfo.PublishedPaths),
		"publishedPaths":      publishedPathsList,
		"stagingTargetPath":   trackingInfo.StagingTargetPath,
	}).Debug("Volume paths in tracking info.")

	// Use published path if available, otherwise fall back to staging target path
	if selectedPath != "" {
		Logc(ctx).WithField("selectedPath", selectedPath).Debug("Using published path as volume path.")
		return selectedPath
	}

	// Fallback to staging target path
	Logc(ctx).WithField("stagingTargetPath", trackingInfo.StagingTargetPath).Debug("Using staging target path as volume path.")
	return trackingInfo.StagingTargetPath
}
