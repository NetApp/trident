// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"fmt"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/models"
)

// AttachSMBVolume attaches the volume to the local host. This method must be able to accomplish its task using only the data passed in.
// It may be assumed that this method always runs on the host to which the volume will be attached.

func AttachSMBVolume(
	ctx context.Context, name, mountpoint, username, password string, publishInfo *models.VolumePublishInfo,
) error {
	Logc(ctx).Debug(">>>> smb.AttachSMBSVolume")
	defer Logc(ctx).Debug("<<<< smb.AttachSMBVolume")

	exportPath := fmt.Sprintf("\\\\%s%s", publishInfo.SMBServer, publishInfo.SMBPath)

	Logc(ctx).WithFields(LogFields{
		"volume":     name,
		"exportPath": exportPath,
		"mountpoint": mountpoint,
	}).Debug("Publishing SMB volume.")

	return mountSMBPath(ctx, exportPath, mountpoint, username, password)
}
