// Copyright 2022 NetApp, Inc. All Rights Reserved.
package utils

import (
	"context"
	"errors"

	. "github.com/netapp/trident/logger"
)

func IsLikelyNotMountPoint(ctx context.Context, mountpoint string) (bool, error) {
	Logc(ctx).Debug(">>>> k8s_utils_windows.IsLikelyNotMountPoint")
	defer Logc(ctx).Debug("<<<< k8s_utils_windows.IsLikelyNotMountPoint")
	return false, errors.New("IsLikelyNotMountPoint is not supported for Windows")
}
