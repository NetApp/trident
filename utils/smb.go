package utils

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
)

// AttachSMBVolume attaches the volume to the local host. This method must be able to accomplish its task using only the data passed in.
// It may be assumed that this method always runs on the host to which the volume will be attached.

func AttachSMBVolume(
	ctx context.Context, name, mountpoint, username, password string, publishInfo *VolumePublishInfo,
) error {
	Logc(ctx).Debug(">>>> smb.AttachSMBSVolume")
	defer Logc(ctx).Debug("<<<< smb.AttachSMBVolume")

	exportPath := fmt.Sprintf("\\\\%s%s", publishInfo.SMBServer, publishInfo.SMBPath)

	Logc(ctx).WithFields(log.Fields{
		"volume":     name,
		"exportPath": exportPath,
		"mountpoint": mountpoint,
	}).Debug("Publishing SMB volume.")

	return mountSMBPath(ctx, exportPath, mountpoint, username, password)
}
