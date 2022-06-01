// Copyright 2022 NetApp, Inc. All Rights Reserved.

// File to contain iscsi protocol related functionalities for linux flavor

package utils

import (
	"context"

	. "github.com/netapp/trident/logger"
)

// ISCSIActiveOnHost will return if the iscsi daemon is active on the given host
func ISCSIActiveOnHost(ctx context.Context, host HostSystem) (bool, error) {
	Logc(ctx).Debug(">>>> iscsi_linux.ISCSIActiveOnHost")
	defer Logc(ctx).Debug("<<<< iscsi_linux.ISCSIActiveOnHost")

	var serviceName string

	switch host.OS.Distro {
	case Ubuntu, Debian:
		serviceName = "open-iscsi"
	default:
		serviceName = "iscsid"
	}

	return ServiceActiveOnHost(ctx, serviceName)
}
