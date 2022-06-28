// Copyright 2022 NetApp, Inc. All Rights Reserved.

// This file should only contain functions for handling the filesystem for Windows flavor

package utils

import (
	"context"
	"errors"

	. "github.com/netapp/trident/logger"
)

// ISCSIActiveOnHost unused stub function
func ISCSIActiveOnHost(ctx context.Context, host HostSystem) (bool, error) {
	Logc(ctx).Debug(">>>> iscsi_windows.ISCSIActiveOnHost")
	defer Logc(ctx).Debug("<<<< iscsi_windows.ISCSIActiveOnHost")
	return false, errors.New("ISCSIActiveOnHost is not supported for windows")
}
