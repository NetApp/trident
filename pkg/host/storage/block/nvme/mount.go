// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"fmt"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/host/service/filesystem"
	"github.com/netapp/trident/pkg/host/service/luks"
	"github.com/netapp/trident/pkg/host/service/nvme"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// Mount mounts an NVMe volume to the given target path.
// This method should be idempotent.
// The state should carry the namespace identity so the driver can discover the topmost device in the stack.
// The target path should be a valid mount point.
//
// The device is resolved from the stored namespace UUID rather than a device path,
// which avoids stale paths when the kernel renames nvmeXnY after a reconnect.
// Mount never opens LUKS: the mapper must already exist from Attach.
func (d *Driver) Mount(
	ctx context.Context, volumeID string, state *models.VolumeAttachState, spec *models.VolumeMountSpec,
) error {
	nqn := state.NVMeSubsystemNQN
	nsUUID := state.NVMeNamespaceUUID
	deviceStack := state.Devices
	targetPath := spec.MountPoint // MountPoint is the publish target path (location in a pod).
	filesystemType := state.Filesystem
	mountOptions := spec.MountOptions

	fields := LogFields{
		"volumeID":       volumeID,
		"subsystem":      nqn,
		"namespace":      nsUUID,
		"deviceStack":    deviceStack,
		"targetPath":     targetPath,
		"mountOptions":   mountOptions,
		"filesystemType": filesystemType,
	}
	Logc(ctx).WithFields(fields).Info("Mounting NVMe volume.")

	release, err := d.acquireLimiter(ctx, mountKey)
	if err != nil {
		return err
	}
	defer release()

	// Phase 0: Validate.
	if targetPath == "" {
		Logc(ctx).WithFields(fields).Error("Cannot mount NVMe volume with unspecified target path.")
		return fmt.Errorf("target path is required for mounting NVMe volume")
	}

	// Phase 0: Note the transport state. The staged device is what matters for publish;
	// do not reject — EnsureMount is idempotent and the namespace head may still be accessible.
	if !d.nvme.HasLivePath(ctx, nqn) {
		Logc(ctx).WithFields(fields).Warn("NVMe subsystem has no live path; mount may fail.")
	}

	// Phase 1: Find the top-most device from the stored namespace identity.
	// A missing namespace means no healthy path exists. Reject the mount rather than mount a dead device.
	nvmeDev, err := d.nvme.DeviceByUUID(ctx, nqn, nsUUID)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return mountError(filesystemType, "mount is unsafe while the NVMe namespace %s is missing; %v", nsUUID, err)
		}
		return mountError(filesystemType, "failed to discover NVMe device for namespace %s; %v", nsUUID, err)
	}

	source, err := d.topDevicePath(ctx, nvmeDev, state, spec.InternalID)
	if err != nil {
		if errors.IsNotFoundError(err) {
			return mountError(filesystemType, "LUKS device for namespace %s is not open; refusing to mount", nsUUID)
		}
		return mountError(filesystemType, "failed to find LUKS device for namespace %s; %v", nsUUID, err)
	}
	devicePath := source.Path

	// A passphrase that cannot be verified or rotated does not block the publish.
	if state.Encryption.IsLUKS() {
		if err := d.ensurePassphrase(ctx, volumeID, spec.InternalID, nvmeDev.Path(), spec.Secrets); err != nil {
			Logc(ctx).WithError(err).Error("Failed to ensure current LUKS passphrase.")
		}
	}

	// Phase 2: Mount the device to the target path.
	if err := d.filesystem.EnsureMount(ctx, source, targetPath, mountOptions, filesystemType); err != nil {
		Logc(ctx).WithFields(LogFields{
			"volumeID":    volumeID,
			"devicePath":  devicePath,
			"targetPath":  targetPath,
			"deviceStack": deviceStack,
		}).WithError(err).Error("Failed to mount device at target path.")
		return mountError(filesystemType, "%v", err)
	}

	Logc(ctx).WithFields(fields).Info("NVMe volume mounted.")
	return nil
}

// topDevicePath returns the device to mount: this volume's LUKS mapper when it is encrypted, otherwise the
// namespace, even if another device sits on top of it. The mapper is found by the dm UUID in the tracked stack,
// or by the internal-ID mapper name for a tracking file that has no stack.
func (d *Driver) topDevicePath(
	ctx context.Context, nvmeDev *nvme.Device, state *models.VolumeAttachState, internalID string,
) (filesystem.Source, error) {
	if !state.Encryption.IsLUKS() {
		return filesystem.Source{Path: nvmeDev.Path(), ID: nvmeDev.SourceID()}, nil
	}

	var luksDev *luks.Device
	var err error
	if crypt, ok := state.Devices.Find(models.BlockDeviceCrypt); ok && crypt.IsValid() {
		luksDev, err = d.luks.DeviceByUUID(ctx, crypt.ID)
	} else {
		luksDev, err = d.luks.DeviceByInternalID(ctx, internalID)
	}
	if err != nil {
		return filesystem.Source{}, err
	}
	// The mapper is mounted by its device node (/dev/dm-N), not its /dev/mapper symlink: mountinfo records a raw
	// bind mount under the resolved node, so only the node lets a retried mount find the existing one.
	return filesystem.Source{Path: luksDev.Path(), ID: luksDev.SourceID()}, nil
}

// mountError returns an InternalError for a failed publish, worded for a bind mount when the volume is raw block.
func mountError(filesystemType filesystem.Type, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if filesystemType == filesystem.Raw {
		return errors.InternalError("unable to bind mount raw device; %s", msg)
	}
	return errors.InternalError("unable to mount device; %s", msg)
}
