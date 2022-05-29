// Copyright 2022 Netapp Inc. All Rights Reserved.

// File to contain iscsi protocol related functionalities for Darwin flavor

package utils

import (
	"context"

	. "github.com/netapp/trident/logger"
)

// ISCSIActiveOnHost will return if the iscsi daemon is active on the given host
func ISCSIActiveOnHost(ctx context.Context, host HostSystem) (bool, error) {
	Logc(ctx).Debug(">>>> iscsi_darwin.ISCSIActiveOnHost")
	defer Logc(ctx).Debug("<<<< iscsi_darwin.ISCSIActiveOnHost")
	msg := "ISCSIActiveOnHost is not supported for darwin"
	return false, UnsupportedError(msg)
}
