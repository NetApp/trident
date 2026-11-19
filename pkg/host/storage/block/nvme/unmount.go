// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/host/service/filesystem"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// Unmount unmounts an NVMe volume from a given target path.
// This method should be idempotent.
// The target path to remove should be a valid mount point.
//
// There is no NVMe-specific teardown step; the filesystem unmount is protocol-agnostic.
// An unmounted target path is left in place; the caller owns its removal.
func (d *Driver) Unmount(ctx context.Context, volumeID string, spec *models.VolumeUnmountSpec) error {
	targetPath := spec.MountPoint
	fields := LogFields{
		"volumeID":   volumeID,
		"targetPath": targetPath,
	}
	Logc(ctx).WithFields(fields).Info("Unmounting NVMe volume.")

	release, err := d.acquireLimiter(ctx, unmountKey)
	if err != nil {
		return err
	}
	defer release()

	// Phase 0: Ensure the target path is unmounted.
	// This should handle unmounting the target path (a directory or file) from a container path.
	mounted, err := d.filesystem.IsMounted(ctx, filesystem.Source{}, targetPath)
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Failed to check if the target path is mounted.")
		return errors.InternalError("unable to check if targetPath (%s) is mounted; %v", targetPath, err)
	}
	if !mounted {
		Logc(ctx).WithFields(fields).Debug("Volume not mounted; nothing to unmount.")
		return nil
	}

	if err = d.filesystem.EnsureUnmount(ctx, targetPath, false); err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Failed to unmount NVMe volume.")
		return errors.InvalidInputError("unable to unmount volume; %v", err)
	}

	Logc(ctx).WithFields(fields).Info("NVMe volume unmounted.")
	return nil
}
