// Copyright 2022 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling the filesystem for Darwin flavor

package utils

import (
	"golang.org/x/net/context"

	. "github.com/netapp/trident/logger"
)

// GetFilesystemStats unused stub function
func GetFilesystemStats(ctx context.Context, _ string) (int64, int64, int64, int64, int64, int64, error) {
	Logc(ctx).Debug(">>>> filesystem_darwin.GetFilesystemStats")
	defer Logc(ctx).Debug("<<<< filesystem_darwin.GetFilesystemStats")
	return 0, 0, 0, 0, 0, 0, UnsupportedError("GetFilesystemStats is not supported for darwin")
}

// getFilesystemSize unused stub function
func getFilesystemSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> filesystem_darwin.getFilesystemSize")
	defer Logc(ctx).Debug("<<<< filesystem_darwin.getFilesystemSize")
	return 0, UnsupportedError("getFilesystemSize is not supported for darwin")
}

// GetDeviceFilePath returns the staging path for volume.
// This function is dummy for darwin.
func GetDeviceFilePath(ctx context.Context, _, volumeId string) (string, error) {
	Logc(ctx).Debug(">>>> filesystem_darwin.GetDeviceFilePath")
	defer Logc(ctx).Debug("<<<< filesystem_darwin.GetDeviceFilePath")
	return "", UnsupportedError("GetDeviceFilePath is not supported for darwin")
}

// GetUnmountPath is a dummy added for compilation.
func GetUnmountPath(ctx context.Context, trackingInfo *VolumeTrackingInfo) (string, error) {
	Logc(ctx).Debug(">>>> filesystem_darwin.GetUnmountPath")
	defer Logc(ctx).Debug("<<<< filesystem_darwin.GetUnmountPath")

	return "", UnsupportedError("GetUnmountPath is not supported for darwin")
}
