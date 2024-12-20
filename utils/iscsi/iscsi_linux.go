// Copyright 2024 NetApp, Inc. All Rights Reserved.

package iscsi

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/osutils"
)

// ISCSIActiveOnHost will return if the iscsi daemon is active on the given host
func (client *Client) ISCSIActiveOnHost(ctx context.Context, host models.HostSystem) (bool, error) {
	Logc(ctx).Debug(">>>> iscsi_linux.ISCSIActiveOnHost")
	defer Logc(ctx).Debug("<<<< iscsi_linux.ISCSIActiveOnHost")

	var serviceName string

	switch host.OS.Distro {
	case osutils.Ubuntu, osutils.Debian:
		serviceName = "open-iscsi"
	default:
		serviceName = "iscsid"
	}

	return client.osUtils.ServiceActiveOnHost(ctx, serviceName)
}
