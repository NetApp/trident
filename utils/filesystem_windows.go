// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"os"
	"strings"

	"golang.org/x/net/context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func getFilesystemSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> filesystem_windows.getFilesystemSize")
	defer Logc(ctx).Debug("<<<< filesystem_windows.getFilesystemSize")
	return 0, errors.UnsupportedError("getFilesystemSize is not supported for windows")
}

func GetFilesystemStats(ctx context.Context, path string) (int64, int64, int64, int64, int64, int64, error) {
	Logc(ctx).Debug(">>>> mount_windows.GetFilesystemStats")
	defer Logc(ctx).Debug("<<<< mount_windows.GetFilesystemStats")

	csiproxy, err := NewCSIProxyUtils()
	if err != nil {
		Logc(ctx).Errorf("Failed to instantiate CSI proxy clients. Error: %v", err)
	}
	total, used, err := csiproxy.GetFilesystemUsage(ctx, path)
	if err != nil {
		Logc(ctx).Errorf("Failed to instantiate CSI proxy clients. Error: %v", err)
	}
	Logc(ctx).Debugf("Total fs capacity: %d used: %d", total, used)
	return total - used, total, used, 0, 0, 0, nil
}

// GetDeviceFilePath returns the staging path for volume.
func GetDeviceFilePath(ctx context.Context, _, volumeId string) (string, error) {
	Logc(ctx).Debug(">>>> filesystem_windows.GetDeviceFilePath")
	defer Logc(ctx).Debug("<<<< filesystem_windows.GetDeviceFilePath")

	path := "\\var\\lib\\trident\\tracking" + "\\" + volumeId

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(path, os.ModePerm)
		if err != nil {
			return "", err
		}
	}

	return path, nil
}

// GetUnmountPath returns unmount path for volume
func GetUnmountPath(ctx context.Context, trackingInfo *models.VolumeTrackingInfo) (string, error) {
	Logc(ctx).Debug(">>>> osutils_windows.GetUnmountPath")
	defer Logc(ctx).Debug("<<<< osutils_windows.GetUnmountPath")

	path := "\\" + trackingInfo.SMBServer + trackingInfo.SMBPath
	return strings.Replace(path, "/", "\\", -1), nil
}
