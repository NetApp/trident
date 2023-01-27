// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"fmt"

	. "github.com/netapp/trident/logging"
)

// AttachNFSVolume attaches the volume to the local host.
// This method must be able to accomplish its task using only the data passed in.
// It may be assumed that this method always runs on the host to which the volume will be attached.
func AttachNFSVolume(ctx context.Context, name, mountpoint string, publishInfo *VolumePublishInfo) error {
	Logc(ctx).Debug(">>>> nfs.AttachNFSVolume")
	defer Logc(ctx).Debug("<<<< nfs.AttachNFSVolume")

	exportPath := fmt.Sprintf("%s:%s", publishInfo.NfsServerIP, publishInfo.NfsPath)
	options := publishInfo.MountOptions

	Logc(ctx).WithFields(LogFields{
		"volume":     name,
		"exportPath": exportPath,
		"mountpoint": mountpoint,
		"options":    options,
	}).Debug("Publishing NFS volume.")

	return mountNFSPath(ctx, exportPath, mountpoint, options)
}
