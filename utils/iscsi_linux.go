// Copyright 2022 NetApp, Inc. All Rights Reserved.

// File to contain iscsi protocol related functionalities for linux flavor

package utils

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/models"
)

// ISCSIActiveOnHost will return if the iscsi daemon is active on the given host
func ISCSIActiveOnHost(ctx context.Context, host models.HostSystem) (bool, error) {
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
