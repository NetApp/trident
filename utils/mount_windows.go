// Copyright 2022 NetApp, Inc. All Rights Reserved.
package utils

import (
	"context"
	"errors"

	. "github.com/netapp/trident/logger"
)

// IsLikelyNotMountPoint  unused stub function
func IsLikelyNotMountPoint(ctx context.Context, mountpoint string) (bool, error) {
	Logc(ctx).Debug(">>>> mount_windows.IsLikelyNotMountPoint")
	defer Logc(ctx).Debug("<<<< mount_windows.IsLikelyNotMountPoint")
	return false, errors.New("IsLikelyNotMountPoint is not supported for Windows")
}
