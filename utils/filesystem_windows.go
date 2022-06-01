// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"errors"

	"golang.org/x/net/context"

	. "github.com/netapp/trident/logger"
)

func GetFilesystemStats(ctx context.Context, _ string) (int64, int64, int64, int64, int64, int64, error) {
	Logc(ctx).Debug(">>>> filesystem_windows.GetFilesystemStats")
	defer Logc(ctx).Debug("<<<< filesystem_windows.GetFilesystemStats")
	return 0, 0, 0, 0, 0, 0, errors.New("GetFilesystemStats is not supported for windows")
}

func getFilesystemSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> filesystem_windows.getFilesystemSize")
	defer Logc(ctx).Debug("<<<< filesystem_windows.getFilesystemSize")
	return 0, errors.New("getFilesystemSize is not supported for windows")
}
