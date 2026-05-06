// Copyright 2022 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling the filesystem for Darwin flavor

package filesystem

import (
	"golang.org/x/net/context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// GetFilesystemStats unused stub function
func (f *FSClient) GetFilesystemStats(ctx context.Context, _ string) (int64, int64, int64, int64, int64, int64, error) {
	Logc(ctx).Debug(">>>> filesystem_darwin.GetFilesystemStats")
	defer Logc(ctx).Debug("<<<< filesystem_darwin.GetFilesystemStats")
	return 0, 0, 0, 0, 0, 0, errors.UnsupportedError("GetFilesystemStats is not supported for darwin")
}

// getBlockDeviceSize unused stub function.
func (f *FSClient) getBlockDeviceSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> filesystem_darwin.getBlockDeviceSize")
	defer Logc(ctx).Debug("<<<< filesystem_darwin.getBlockDeviceSize")
	return 0, errors.UnsupportedError("getBlockDeviceSize is not supported for darwin")
}

// GetDeviceFilePath returns the staging path for volume.
// This function is dummy for darwin.
func GetDeviceFilePath(ctx context.Context, _, volumeId string) (string, error) {
	Logc(ctx).Debug(">>>> filesystem_darwin.GetDeviceFilePath")
	defer Logc(ctx).Debug("<<<< filesystem_darwin.GetDeviceFilePath")
	return "", errors.UnsupportedError("GetDeviceFilePath is not supported for darwin")
}

// GetUnmountPath is a dummy added for compilation.
func (f *FSClient) GetUnmountPath(ctx context.Context, trackingInfo *models.VolumeTrackingInfo) (string, error) {
	Logc(ctx).Debug(">>>> filesystem_darwin.GetUnmountPath")
	defer Logc(ctx).Debug("<<<< filesystem_darwin.GetUnmountPath")

	return "", errors.UnsupportedError("GetUnmountPath is not supported for darwin")
}
