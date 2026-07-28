// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"os"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

// UnmountRequest holds inputs for unpublishing a volume (CSI NodeUnpublishVolume).
type UnmountRequest struct {
	TargetPath string
}

// Unmount removes a volume's pod-visible mount at targetPath. This corresponds to CSI's
// NodeUnpublishVolume. Unlike Detach, Unmount never tears down the underlying block/transport
// attachment - it only removes the bind/filesystem mount created by Mount.
func (c *Core) Unmount(ctx context.Context, volume string, req UnmountRequest) error {
	targetPath := req.TargetPath
	if volume == "" {
		return errors.InvalidInputError("volume is empty")
	}
	if targetPath == "" {
		return errors.InvalidInputError("target path is empty")
	}

	fields := LogFields{
		"Method":     "Unmount",
		"Type":       "Node_Core",
		"Volume":     volume,
		"TargetPath": targetPath,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> Unmount")
	defer Logc(ctx).WithFields(fields).Debug("<<<< Unmount")

	if err := c.checkReady(); err != nil {
		return err
	}
	release, err := c.acquireVolumeLock(ctx, volume)
	if err != nil {
		return err
	}
	defer release()

	return c.unmountGeneric(ctx, volume, targetPath)
}

// unmountGeneric is the stopgap unmount path shared by NFS, SMB, FCP, and NVMe until each has
// its own pkg/host/storage driver. It mirrors the legacy CSI NodeUnpublishVolume behavior:
// best-effort unmount, then best-effort removal of the target path, then removal of the
// published-path bookkeeping entry.
func (c *Core) unmountGeneric(ctx context.Context, volume, targetPath string) error {
	Logc(ctx).Debug(">>>> unmountGeneric")
	defer Logc(ctx).Debug("<<<< unmountGeneric")

	release, err := c.acquireLimiter(ctx, unmountVolumeKey)
	if err != nil {
		return err
	}
	defer release()

	isDir, err := c.osutils.IsLikelyDir(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			Logc(ctx).WithField("targetPath", targetPath).Infof(
				"target path (%s) not found; volume is not mounted.", targetPath)
			return nil
		}
		return errors.InternalError("could not check if the target path (%s) is a directory; %v", targetPath, err)
	}

	var notMountPoint bool
	if isDir {
		notMountPoint, err = c.mount.IsLikelyNotMountPoint(ctx, targetPath)
	} else {
		var mounted bool
		mounted, err = c.mount.IsMounted(ctx, "", targetPath, "")
		notMountPoint = !mounted
	}
	if err != nil {
		if os.IsNotExist(err) {
			return errors.NotFoundError("target path not found")
		}
		return errors.InternalError("unable to check if targetPath (%s) is mounted; %v", targetPath, err)
	}

	if notMountPoint {
		Logc(ctx).Debug("Volume not mounted, proceeding to unpublish volume")
	} else if err = c.mount.Umount(ctx, targetPath); err != nil {
		Logc(ctx).WithFields(LogFields{"path": targetPath, "error": err}).Error("unable to unmount volume.")
		return errors.InvalidInputError("unable to unmount volume; %v", err)
	}

	// As per the CSI spec, Trident is responsible for deleting the target path, however today
	// Kubernetes performs this deletion. Here we are making best efforts to delete the resource
	// at the target path. Sometimes this fails, resulting in another NodeUnpublishVolume call;
	// usually deletion goes through in the second attempt.
	if err = c.osutils.DeleteResourceAtPath(ctx, targetPath); err != nil {
		Logc(ctx).Debugf("Unable to delete resource at target path: %s; %v", targetPath, err)
	}

	if err = c.localStore.RemovePublishedPath(ctx, volume, targetPath); err != nil {
		if errors.IsNotFoundError(err) {
			Logc(ctx).WithFields(LogFields{
				"targetPath": targetPath, "volumeId": volume,
			}).Warning("Could not remove published path from volume tracking file: not found.")
			return nil
		}
		return errors.InternalError(
			"could not remove published path (%s) from volume tracking file for volume %s: %v", targetPath, volume, err)
	}

	return nil
}
