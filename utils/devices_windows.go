// Copyright 2022 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for windows flavor

package utils

import (
	"errors"

	"golang.org/x/net/context"

	. "github.com/netapp/trident/logger"
)

// flushOneDevice unused stub function
func flushOneDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).Debug(">>>> devices_windows.flushOneDevice")
	defer Logc(ctx).Debug("<<<< devices_windows.flushOneDevice")
	return errors.New("flushOneDevice is not supported for windows")
}

// getISCSIDiskSize unused stub function
func getISCSIDiskSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> devices_windows.getISCSIDiskSize")
	defer Logc(ctx).Debug("<<<< devices_windows.getISCSIDiskSize")
	return 0, errors.New("getBlockSize is not supported for windows")
}
