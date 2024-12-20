// Copyright 2024 NetApp, Inc. All Rights Reserved.

package iscsi

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// ISCSIActiveOnHost unused stub function
func (client *Client) ISCSIActiveOnHost(ctx context.Context, host models.HostSystem) (bool, error) {
	Logc(ctx).Debug(">>>> iscsi_windows.ISCSIActiveOnHost")
	defer Logc(ctx).Debug("<<<< iscsi_windows.ISCSIActiveOnHost")
	return false, errors.UnsupportedError("ISCSIActiveOnHost is not supported for windows")
}
