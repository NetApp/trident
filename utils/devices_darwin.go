// Copyright 2022 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for Darwin flavor

package utils

import (
	"errors"

	"golang.org/x/net/context"

	. "github.com/netapp/trident/logger"
)

func flushOneDevice(ctx context.Context, devicePath string) error {
	Logc(ctx).Debug(">>>> devices_darwin.flushOneDevice")
	defer Logc(ctx).Debug("<<<< devices_darwin.flushOneDevice")
	return errors.New("flushOneDevice is not supported for darwin")
}

func getISCSIDiskSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> devices_darwin.getISCSIDiskSize")
	defer Logc(ctx).Debug("<<<< devices_darwin.getISCSIDiskSize")
	return 0, errors.New("getBlockSize is not supported for darwin")
}
