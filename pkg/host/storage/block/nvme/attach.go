// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"errors"
	"fmt"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/host/service/block"
	"github.com/netapp/trident/pkg/host/service/filesystem"
	"github.com/netapp/trident/pkg/host/service/luks"
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

var (
	// BetweenAttachAndLUKSPassphrase is exported because the node core's iSCSI and FCP attach fire the same
	// fault point, and a fault name can be registered only once per process.
	BetweenAttachAndLUKSPassphrase fiji.Injector = fiji.Register("betweenAttachAndLUKSPassphrase", "node_core")
	afterFormatBeforeFileSystem    fiji.Injector = fiji.Register("afterFormatBeforeFileSystem", "nvme")
)

// NewAttachSpec builds the spec to attach the volume described by publishInfo. internalID names the LUKS mapper,
// and mountPoint is where the device is globally mounted; it is empty when the volume is only formatted.
func NewAttachSpec(
	publishInfo *models.VolumePublishInfo, internalID, mountPoint string, secrets map[string]string,
) *models.VolumeAttachSpec {
	spec := &models.VolumeAttachSpec{
		VolumeAccessInfo: publishInfo.VolumeAccessInfo,
		InternalID:       internalID,
		MountPoint:       mountPoint,
		Filesystem:       publishInfo.FilesystemType,
		VolumeMode:       publishInfo.VolumeMode,
		Secrets:          secrets,
	}
	if publishInfo.UseLUKS() {
		spec.Encryption = models.EncryptionLUKS
	}
	return spec
}

// Attach attaches an NVMe namespace to the host and returns attachment state.
// This will also globally mount the device if the device has a filesystem.
//
// In the CSI context the namespace is published for self-healing before the subsystem is connected. If the
// attach fails, the namespace is unpublished and the subsystem is disconnected when no other published namespace
// uses it. Other contexts publish nothing and leave a failed attach as it is.
func (d *Driver) Attach(
	ctx context.Context, volumeID string, spec *models.VolumeAttachSpec,
) (state *models.VolumeAttachState, retErr error) {
	subNQN := spec.NVMeSubsystemNQN
	nsUUID := spec.NVMeNamespaceUUID
	targetIPs := spec.NVMeTargetIPs
	fields := LogFields{
		"volumeID":  volumeID,
		"subsystem": subNQN,
		"namespace": nsUUID,
		"targetIPs": targetIPs,
	}
	Logc(ctx).WithFields(fields).Info("Attaching NVMe volume.")

	// State is accumulated progressively; it is returned even on error so
	// the caller knows what was created on the node and can clean up.
	state = &models.VolumeAttachState{}
	state.VolumeAccessInfo = spec.VolumeAccessInfo
	state.Filesystem = spec.Filesystem
	state.Encryption = spec.Encryption
	state.GlobalMount = spec.MountPoint

	release, err := d.acquireLimiter(ctx, attachKey)
	if err != nil {
		return state, err
	}
	defer release()

	// Phase 0 – Preflight: verify host tools are available and configured.
	if err := d.nvme.PreChecks(ctx); err != nil {
		return state, fmt.Errorf("NVMe preflight failed: %w", err)
	}
	if spec.Encryption.IsLUKS() {
		if err := d.luks.PreChecks(ctx); err != nil {
			return state, fmt.Errorf("LUKS preflight failed: %w", err)
		}

		if spec.Secrets == nil {
			return state, fmt.Errorf("LUKS secrets are required for LUKS encryption")
		}
	}
	// Fail fast on a missing subsystem NQN or namespace UUID instead of waiting for the connection timeout.
	// No target IPs is allowed: nothing is connected and the namespace must already be visible.
	if subNQN == "" || nsUUID == "" {
		return state, tridenterrors.InvalidInputError(
			"NVMe subsystem NQN and namespace UUID are required; got NQN '%s', namespace '%s'", subNQN, nsUUID)
	}

	d.healingLock.RLock()
	defer d.healingLock.RUnlock()
	if d.driverContext == config.ContextCSI {
		added, err := d.publishForAttach(ctx, subNQN, nsUUID, targetIPs)
		if err != nil {
			return state, err
		}
		// Once the volume is formatted, the namespace is in use on this node. A later failure (a LUKS passphrase
		// error) leaves it published, so self-healing keeps its paths up and detach tears it down. Disconnecting
		// the subsystem here would pull the device out from under the volume.
		defer func() {
			if added && retErr != nil && !IsLUKSPassphraseError(retErr) {
				d.unpublishAfterAttachError(ctx, state)
			}
		}()
	}

	// Phase 1: Transport and device.
	// Ensure the paths exist to all target IPs, then wait for the namespace to appear.
	if len(targetIPs) > 0 {
		if err := d.nvme.EnsureConnections(ctx, subNQN, targetIPs); err != nil {
			return state, fmt.Errorf("NVMe connection establishment failed: %w", err)
		}
	}

	// There is no lock here. A concurrent Detach may disconnect the subsystem before the namespace is visible.
	// EnsureDevice then times out with NotFound and the caller retries the attach.
	nvmeDev, err := d.nvme.EnsureDevice(ctx, subNQN, nsUUID)
	if err != nil {
		return state, err
	}

	// Record the namespace identity. The namespace UUID is the disk layer ID.
	state.BlockAttachInfo.Devices = append(state.BlockAttachInfo.Devices, models.BlockDeviceInfo{
		ID:   nvmeDev.ID(),
		Type: nvmeDev.Type(),
		Name: nvmeDev.Name(),
	})

	// Use the top-most device in the stack for any filesystem and mount operations.
	devicePath := nvmeDev.Path()
	source := filesystem.Source{Path: devicePath, ID: nvmeDev.SourceID()}
	// The namespace path is recorded as soon as the namespace is found, even if a later step fails.
	state.DevicePath = devicePath
	Logc(ctx).WithFields(LogFields{
		"devicePath": devicePath,
		"deviceInfo": nvmeDev.Info(),
	}).WithFields(fields).Info("Discovered NVMe namespace.")

	// Phase 2: LUKS device
	var (
		luksDev       *luks.Device
		rawDevicePath string
	)
	if spec.Encryption.IsLUKS() {
		// LUKS service wants the "raw" device path.
		// This is the literal device path of the underlying device.
		rawDevicePath = nvmeDev.Path()
		currentSecret, previousSecret := d.luks.ExtractSecrets(spec.Secrets)

		// The mapper is named from the internalID; the passphrase name is recorded against volumeID.
		// EnsureDevice owns the full lifecycle:
		// check formatted → check empty → record passphrase name → format → open → resolve device.
		if err := d.closeStaleMapper(ctx, spec.InternalID, nvmeDev.Name()); err != nil {
			return state, fmt.Errorf("LUKS device setup failed: %w", err)
		}
		opts := luks.DefaultEnsureDeviceOptions()
		opts.VolumeID = volumeID
		var luksErr error
		luksDev, luksErr = d.luks.EnsureDevice(ctx, spec.InternalID, rawDevicePath, currentSecret, previousSecret, opts)
		if luksErr != nil {
			return state, fmt.Errorf("LUKS device setup failed: %w", luksErr)
		}

		// Store the LUKS device identity.
		state.BlockAttachInfo.Devices = append(state.BlockAttachInfo.Devices, models.BlockDeviceInfo{
			ID:   luksDev.ID(),
			Type: luksDev.Type(),
			Name: luksDev.Name(),
			Base: nvmeDev.ID(), // Enrich the luks info with the underlying namespace UUID.
		})

		// Use the top-most device in the stack for any filesystem and mount operations.
		devicePath = luksDev.Path()
		source = filesystem.Source{Path: devicePath, ID: luksDev.SourceID()}
		Logc(ctx).WithFields(LogFields{
			"devicePath": devicePath,
			"mapperPath": luksDev.MapperPath(),
			"deviceInfo": luksDev.Info(),
		}).WithFields(fields).Info("Opened LUKS device.")

		if err := afterFormatBeforeFileSystem.Inject(); err != nil {
			state.DevicePath = devicePath
			return state, err
		}
	}
	state.DevicePath = devicePath

	// A LUKS mapper formatted by this attach holds random data, so it is not probed for a
	// filesystem, and no LUKS mapper is required to be zeroed before it is formatted.
	probe := block.ProbeFull
	if luksDev != nil {
		probe = block.ProbeFilesystem
		if luksDev.IsFreshFormat() {
			probe = block.ProbeNone
		}
	}

	// Resolve the filesystem type: explicit type first, then the type already on the device, then volumeMode.
	fsType, err := d.block.ResolveType(ctx, devicePath, spec.Filesystem, spec.VolumeMode, probe == block.ProbeNone)
	if err != nil {
		if errors.Is(err, block.ErrUnknownFilesystem) {
			return state, fmt.Errorf("namespace %s, device %s is formatted with unknown filesystem type",
				nsUUID, devicePath)
		}
		return state, err
	}
	state.Filesystem = fsType

	// Build the final log message for raw and filesystem operations.
	fields = LogFields{
		"volumeID":       volumeID,
		"namespace":      nsUUID,
		"devicePath":     devicePath,
		"blockDevices":   state.BlockAttachInfo.Devices,
		"filesystemType": fsType,
		"formatOptions":  spec.FormatOptions,
	}

	// Phase 3: Filesystem: detect, format, repair, mount.
	switch fsType {
	case filesystem.Raw:
		if err := d.ensureAttachedPassphrase(ctx, volumeID, spec, rawDevicePath); err != nil {
			return state, err
		}
		Logc(ctx).WithFields(fields).Info("NVMe raw block volume attached.")
		return state, nil
	case "":
		return state, fmt.Errorf("filesystem type is required for non-raw volumes")
	default:
		Logc(ctx).WithFields(fields).Infof("Formatting NVMe volume attached with %s filesystem.", fsType)
	}

	// Ensure we format the device with a filesystem.
	if err := d.block.EnsureFormat(ctx, devicePath, fsType, spec.FormatOptions, probe); err != nil {
		return state, fmt.Errorf("filesystem format failed: %w", err)
	}

	// Best-effort repair. fsck is only safe on a device that is not mounted.
	mounted, err := d.filesystem.IsMounted(ctx, source, "")
	if err != nil {
		return state, err
	}
	if !mounted {
		d.block.Repair(ctx, devicePath, fsType)
	}

	// Globally mount the device. Without a mount point (CSI stage), the volume is only formatted.
	if spec.MountPoint != "" {
		if err := d.filesystem.EnsureMount(ctx, source, spec.MountPoint, spec.MountOptions, fsType); err != nil {
			return state, fmt.Errorf("filesystem mount failed: %w", err)
		}
	}

	if err := d.ensureAttachedPassphrase(ctx, volumeID, spec, rawDevicePath); err != nil {
		return state, err
	}

	Logc(ctx).WithFields(fields).Info("NVMe volume attached.")

	return state, nil
}

// closeStaleMapper closes the volume's LUKS mapper if it is open but no longer on the namespace device nsName.
// luks.EnsureDevice reuses a mapper that exists by name. After a node is removed and re-added, the namespace
// comes back under a new kernel name and the old mapper is left on a device that is gone, so it must be closed
// before the open. A mapper whose stack cannot be read is closed too.
func (d *Driver) closeStaleMapper(ctx context.Context, internalID, nsName string) error {
	mapper, err := d.luks.DeviceByInternalID(ctx, internalID)
	if tridenterrors.IsNotFoundError(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to look up LUKS mapper: %w", err)
	}

	stale, err := d.block.IsMapperStale(ctx, mapper.ID(), nsName)
	if err == nil && !stale {
		return nil
	}

	Logc(ctx).WithFields(LogFields{"mapper": mapper.MapperName(), "device": nsName}).WithError(err).
		Info("Removing stale LUKS mapping.")
	if err := d.luks.EnsureClose(ctx, mapper.ID(), false); err != nil {
		return fmt.Errorf("could not remove LUKS mapping '%s' for device '%s'; %w", mapper.MapperName(), nsName, err)
	}
	return nil
}

// ensureAttachedPassphrase verifies or rotates the LUKS passphrase once the volume is attached.
// Any failure is returned as a *LUKSPassphraseError.
func (d *Driver) ensureAttachedPassphrase(
	ctx context.Context, volumeID string, spec *models.VolumeAttachSpec, rawDevicePath string,
) error {
	if !spec.Encryption.IsLUKS() {
		return nil
	}
	if err := BetweenAttachAndLUKSPassphrase.Inject(); err != nil {
		return &LUKSPassphraseError{err: err}
	}
	if err := d.ensurePassphrase(ctx, volumeID, spec.InternalID, rawDevicePath, spec.Secrets); err != nil {
		return &LUKSPassphraseError{err: fmt.Errorf("could not set LUKS volume passphrase; %w", err)}
	}
	return nil
}
