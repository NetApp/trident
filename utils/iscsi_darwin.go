// Copyright 2022 NetApp, Inc. All Rights Reserved.

// File to contain iscsi protocol related functionalities for Darwin flavor

package utils

import (
	"context"

	. "github.com/netapp/trident/logger"
)

// ISCSIActiveOnHost unused stub function
func ISCSIActiveOnHost(ctx context.Context, host HostSystem) (bool, error) {
	Logc(ctx).Debug(">>>> iscsi_darwin.ISCSIActiveOnHost")
	defer Logc(ctx).Debug("<<<< iscsi_darwin.ISCSIActiveOnHost")
	msg := "ISCSIActiveOnHost is not supported for darwin"
	return false, UnsupportedError(msg)
}
