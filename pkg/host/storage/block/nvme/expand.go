// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"fmt"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/host/service/filesystem"
	"github.com/netapp/trident/pkg/host/service/luks"
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// Expand expands the usable space for an NVMe attachment given the current attachment state and expand spec.
//
// NVMe needs no rescan: the kernel resizes the namespace when the controller posts an asynchronous event. Expand
// requires a live path and waits for the namespace to reach the requested size, so a filesystem resize does not
// run against a namespace whose new size has not propagated yet.
func (d *Driver) Expand(
	ctx context.Context, volumeID string, state *models.VolumeAttachState, spec *models.VolumeExpandSpec,
) error {
	release, err := d.acquireLimiter(ctx, expandKey)
	if err != nil {
		return err
	}
	defer release()

	if err := d.resolveDevices(ctx, state, ""); err != nil {
		return err
	}

	nqn := state.NVMeSubsystemNQN
	nsUUID := state.NVMeNamespaceUUID
	deviceStack := state.Devices
	sizeBytes := spec.SizeBytes
	fsType := state.Filesystem
	mountPnt := state.GlobalMount
	fields := LogFields{
		"volumeID":    volumeID,
		"subsystem":   nqn,
		"namespace":   nsUUID,
		"deviceStack": deviceStack,
		"sizeBytes":   sizeBytes,
		"filesystem":  fsType,
		"mountPoint":  mountPnt,
	}
	Logc(ctx).WithFields(fields).Info("Expanding NVMe volume.")

	// Phase 1: The namespace must be present.
	nvmeDev, err := d.nvme.DeviceByUUID(ctx, nqn, nsUUID)
	if err != nil {
		return fmt.Errorf("failed to find NVMe device: %w", err)
	}
	preExpandDeviceSize := nvmeDev.Size()
	fsSource := filesystem.Source{Path: nvmeDev.Path(), ID: nvmeDev.SourceID()}

	// The kernel only learns of a new size over a live path, and a dead path cannot be resized.
	if !d.nvme.HasLivePath(ctx, nqn) {
		return tridenterrors.VolumeStateError("NVMe subsystem %s has no live path to expand volume %s; retry later",
			nqn, volumeID)
	}
	// A zero size is a request without a capacity (CSI's unset required_bytes); there is nothing to wait for.
	if sizeBytes > 0 {
		if err := d.nvme.EnsureSize(ctx, nvmeDev, sizeBytes); err != nil {
			return fmt.Errorf("NVMe namespace %s did not reach %d bytes: %w", nsUUID, sizeBytes, err)
		}
	}

	// Phase 2: Resize the LUKS device. This is a NOP if there is no crypt layer in the attachment state.
	luksInfo, hasLUKS := deviceStack.Find(models.BlockDeviceCrypt)
	hasLUKS = hasLUKS && luksInfo.IsValid()
	if state.Encryption.IsLUKS() && !hasLUKS {
		return fmt.Errorf("no LUKS device recorded for encrypted volume %s", volumeID)
	}
	if hasLUKS {
		Logc(ctx).WithFields(LogFields{
			"namespace": nsUUID,
			"dmUUID":    luksInfo.ID,
		}).Debug("LUKS encryption used on device; resizing.")

		luksDev, err := d.luks.DeviceByUUID(ctx, luksInfo.ID)
		if err != nil {
			return err
		}
		fsSource = filesystem.Source{Path: luksDev.MapperPath(), ID: luksDev.SourceID()}

		if passphrase, ok := spec.Secrets[luks.SecretKeyPassphrase]; !ok {
			return tridenterrors.InvalidInputError("cannot expand LUKS encrypted volume; no passphrase provided")
		} else if passphrase == "" {
			return tridenterrors.InvalidInputError("cannot expand LUKS encrypted volume; empty passphrase provided")
		}
		current, _ := d.luks.ExtractSecrets(spec.Secrets)
		if err := d.luks.EnsureResize(ctx, luksDev.ID(), current); err != nil {
			Logc(ctx).WithFields(fields).WithError(err).Error("Failed to resize LUKS device.")
			if tridenterrors.IsIncorrectLUKSPassphraseError(err) {
				err = tridenterrors.InvalidInputError(err.Error())
			} else {
				err = tridenterrors.InternalError("failed to resize LUKS mapping for volume %s: %v", volumeID, err)
			}
			return fmt.Errorf("could not resize LUKS mapping for volume %s; %w", volumeID, err)
		}
	}

	// Re-read the namespace to get a fresh size after the resize; the device cached its size when it was found.
	var postExpandDeviceSize uint64
	if postDev, err := d.nvme.DeviceByUUID(ctx, nqn, nsUUID); err != nil {
		Logc(ctx).WithFields(fields).WithError(err).
			Warn("Failed to read post-expand device size; skipping device growth check.")
	} else {
		postExpandDeviceSize = postDev.Size()
	}
	devicesGrew := preExpandDeviceSize > 0 && postExpandDeviceSize > preExpandDeviceSize

	// Phase 3: If a filesystem is used, resize it.
	switch fsType {
	case filesystem.Raw:
		Logc(ctx).WithField("volumeId", volumeID).Info("Filesystem expansion completed.")
		return nil
	case "":
		return fmt.Errorf("invalid requested filesystem '%v' for volume expansion", fsType)
	}

	preExpandFSSize, postExpandFSSize, err := d.filesystem.EnsureExpand(ctx, fsSource, mountPnt, state.MountOptions, fsType)
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Failed to expand filesystem.")
		return tridenterrors.InternalError("failed to expand filesystem for volume %s: %v", volumeID, err)
	}

	// If the block device grew but the filesystem did not, the resize tool silently failed.
	if devicesGrew && postExpandFSSize <= preExpandFSSize {
		Logc(ctx).WithFields(LogFields{
			"preExpandFilesystemSize":  preExpandFSSize,
			"postExpandFilesystemSize": postExpandFSSize,
			"preExpandDeviceSize":      preExpandDeviceSize,
			"postExpandDeviceSize":     postExpandDeviceSize,
		}).Error("Filesystem did not grow despite block device growing during expand.")
		return tridenterrors.InternalError("filesystem size did not grow")
	}
	Logc(ctx).WithField("volumeId", volumeID).Info("Filesystem expansion completed.")

	Logc(ctx).WithFields(fields).Info("NVMe volume expanded.")
	return nil
}
