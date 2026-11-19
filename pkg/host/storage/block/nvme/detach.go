// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/host/service/nvme"
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

const (
	// luksCloseMaxWait is how long a busy LUKS mapper fails the detach before the detach goes on without it.
	luksCloseMaxWait = 2 * time.Minute
	// flushMaxWait is how long a failing flush fails the detach before the detach goes on without it.
	flushMaxWait = 6 * time.Minute
)

var (
	afterNvmeLuksDeviceClosed fiji.Injector = fiji.Register("afterNvmeLuksDeviceClosed", "node_core")
	afterNvmeDisconnect       fiji.Injector = fiji.Register("afterNvmeDisconnect", "node_core")
)

// Detach detaches an NVMe namespace from the host: it removes the temporary expand mount and the global mount,
// closes LUKS and flushes the namespace, then unpublishes it and disconnects the subsystem if no other published
// namespace uses it, then closes a LUKS mapper still open. A state without a device stack is resolved from
// the host first.
//
// Failures the caller retries (kubelet retries NodeUnstageVolume):
//   - A busy mapper fails the detach until its first failure is luksCloseMaxWait old; after that the detach goes on
//     and the mapper is closed again after the disconnect; a mapper still busy then is left open.
//   - A failed flush fails the detach until its first failure is flushMaxWait old; after that the detach goes on.
//     spec.IgnoreFlushErrors (unsafe detach) ignores flush failures from the start.
//   - A MaxWaitExceeded error means the subsystem lock was too slow; the subsystem stays connected and the
//     mapper is not removed, so the caller retries. Other disconnect failures are logged and the detach goes on.
//
// A crypt layer found on the host but missing from state (crash between LUKS open and the tracking write) is
// added to state.Devices.
func (d *Driver) Detach(
	ctx context.Context, volumeID string, state *models.VolumeAttachState, spec *models.VolumeDetachSpec,
) error {
	release, err := d.acquireLimiter(ctx, detachKey)
	if err != nil {
		return err
	}
	defer release()

	d.healingLock.RLock()
	defer d.healingLock.RUnlock()

	if err := d.resolveDevices(ctx, state, spec.InternalID); err != nil {
		return err
	}
	if err := d.detachBlockStack(ctx, volumeID, state, spec); err != nil {
		return err
	}

	d.unpublish(state.NVMeSubsystemNQN, state.NVMeNamespaceUUID)
	if err := d.disconnectIfUnpublished(ctx, state, true); err != nil {
		if tridenterrors.IsMaxWaitExceededError(err) {
			return err
		}
		Logc(ctx).WithError(err).Warn("Error during subsystem disconnect check.")
	}
	if err := afterNvmeDisconnect.Inject(); err != nil {
		return err
	}

	d.removeMapper(ctx, state)
	if luksInfo, ok := state.BlockAttachInfo.Devices.Find(models.BlockDeviceCrypt); ok {
		d.clearFailure(d.luksCloseFailedAt, luksInfo.ID)
	}
	return nil
}

// detachBlockStack removes the global mount, closes LUKS and flushes the namespace.
func (d *Driver) detachBlockStack(
	ctx context.Context, volumeID string, state *models.VolumeAttachState, spec *models.VolumeDetachSpec,
) error {
	nqn := state.NVMeSubsystemNQN
	nsUUID := state.NVMeNamespaceUUID
	deviceStack := state.BlockAttachInfo.Devices
	globalMount := state.GlobalMount
	forceDetach := spec.Force

	fields := LogFields{
		"volumeID":    volumeID,
		"subsystem":   nqn,
		"namespace":   nsUUID,
		"forceDetach": forceDetach,
		"deviceStack": deviceStack,
		"globalMount": globalMount,
	}
	Logc(ctx).WithFields(fields).Info("Detaching NVMe volume.")

	// Phase 0: Find what is still on the host.
	// A namespace that is not found does not stop the detach; the remaining host state is still cleaned up.
	var nvmeDev *nvme.Device
	if dev, err := d.nvme.DeviceByUUID(ctx, nqn, nsUUID); err != nil {
		if !tridenterrors.IsNotFoundError(err) {
			return fmt.Errorf("failed to get NVMe device: %w", err)
		}
		Logc(ctx).WithFields(fields).WithError(err).Debug("NVMe namespace not found; continuing with detach.")
	} else {
		nvmeDev = dev
	}

	// Phase 1: Remove the temporary mount a failed expand may have left under the staging path, before the LUKS
	// close: a mapper mounted there cannot close.
	if globalMount != "" {
		if err := d.filesystem.RemoveExpandMount(ctx, globalMount); err != nil {
			Logc(ctx).WithFields(fields).WithError(err).Error("Failed to remove the temporary expand mount.")
			return fmt.Errorf("failed to remove temporary directory in staging target path %s; %w", globalMount, err)
		}
	}

	// Remove the global mount.
	// If force == true, ignore errors.
	// This is a NOP for raw block volumes and for volumes staged without a global mount (CSI).
	if globalMount == "" {
		Logc(ctx).WithFields(fields).Debug("No global mount to remove.")
	} else if err := d.filesystem.EnsureUnmount(ctx, globalMount, forceDetach); err != nil {
		if !forceDetach {
			Logc(ctx).WithFields(fields).Error("Failed to remove global mount for volume.")
			return err
		}
		Logc(ctx).WithFields(fields).Warn("Failed to remove global mount for volume; ignoring.")
	}

	// Phase 2: Close the LUKS mapper.
	// This must run even when the namespace and its paths are gone, because the mapper outlives them.
	var cryptUUID string
	if luksInfo, ok := deviceStack.Find(models.BlockDeviceCrypt); ok {
		cryptUUID = luksInfo.ID
	}
	if cryptUUID == "" && state.Encryption.IsLUKS() && nvmeDev != nil {
		// Tracking written without the crypt layer (crash between open and state write).
		// Walk the namespace's holders for a crypt dm UUID.
		if stack, err := d.block.ResolveDeviceStack(ctx, nvmeDev.Name()); err == nil {
			for _, dev := range stack {
				if dev != nil {
					if dmInfo := dev.DMInfo(); dmInfo != nil && dmInfo.IsCrypt() {
						cryptUUID = dmInfo.UUID()
						state.BlockAttachInfo.Devices = append(state.BlockAttachInfo.Devices, models.BlockDeviceInfo{
							ID:   cryptUUID,
							Type: models.BlockDeviceCrypt,
							Name: dmInfo.Name(),
							Base: nsUUID,
						})
						break
					}
				}
			}
		}
	}

	if cryptUUID != "" {
		Logc(ctx).WithField("UUID", cryptUUID).Info("Closing LUKS device.")
		// A force detach still waits for the mapper to close, up to luksCloseMaxWait.
		if err := d.luks.EnsureClose(ctx, cryptUUID, false); err != nil {
			// Past the wait, removeMapper closes the mapper once the transport is gone.
			if !errors.Is(err, context.DeadlineExceeded) &&
				d.failingFor(d.luksCloseFailedAt, cryptUUID) <= luksCloseMaxWait {
				Logc(ctx).WithField("UUID", cryptUUID).WithError(err).Error("Failed to close LUKS device.")
				return err
			}
			Logc(ctx).WithField("UUID", cryptUUID).WithError(err).Debug("LUKS close wait time exceeded, continuing with device removal.")
		}
		if err := afterNvmeLuksDeviceClosed.Inject(); err != nil {
			return err
		}
	}

	// Phase 3: Flush the namespace.
	// The service flushes only while the namespace has a live path; a flush on a dead path would block.
	if nvmeDev != nil {
		Logc(ctx).WithField("namespace", nsUUID).Info("Flushing NVMe device.")
		opts := nvme.FlushOptions{Force: forceDetach, IgnoreErrors: spec.IgnoreFlushErrors}
		if err := d.nvme.EnsureFlush(ctx, nvmeDev, opts); err != nil {
			if !nvme.IsFlushError(err) {
				Logc(ctx).WithField("namespace", nsUUID).WithError(err).Error("Failed to flush NVMe device.")
				return err
			}
			if elapsed := d.failingFor(d.nvmeFlushFailedAt, nsUUID); elapsed <= flushMaxWait {
				Logc(ctx).WithFields(LogFields{"namespace": nsUUID, "elapsed": elapsed}).WithError(err).
					Debug("Could not flush NVMe device.")
				return fmt.Errorf("failed to flush NVMe device; %v", err)
			}
			Logc(ctx).WithFields(LogFields{"namespace": nsUUID, "maxWait": flushMaxWait}).
				Warn("Could not flush device within expected time period.")
		}
		d.clearFailure(d.nvmeFlushFailedAt, nsUUID)
	}

	Logc(ctx).WithFields(fields).Info("NVMe volume block stack detached.")
	return nil
}

// disconnect disconnects the namespace's subsystem from the host, unless the host still shows other
// namespaces on it. The caller has already found that no published namespace uses the subsystem and holds
// its subsystem lock, so a subsystem whose paths are all down (namespaces cannot be enumerated) is
// disconnected too.
func (d *Driver) disconnect(ctx context.Context, state *models.VolumeAttachState) error {
	fields := LogFields{"subsystem": state.NVMeSubsystemNQN, "namespace": state.NVMeNamespaceUUID}
	if err := d.nvme.EnsureDisconnect(ctx, state.NVMeSubsystemNQN, state.NVMeNamespaceUUID, true); err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Debug("Error disconnecting subsystem.")
		return err
	}
	return nil
}

// removeMapper closes the volume's LUKS mapper if one is still open after the disconnect step, whether or not
// the subsystem was disconnected: a mapper left over may be a ghost with dirty buffers. A mapper that is still
// busy is left open. Failures are logged, not returned.
func (d *Driver) removeMapper(ctx context.Context, state *models.VolumeAttachState) {
	luksInfo, ok := state.BlockAttachInfo.Devices.Find(models.BlockDeviceCrypt)
	if !ok || luksInfo.ID == "" {
		return
	}
	Logc(ctx).WithField("UUID", luksInfo.ID).Info("Ensuring LUKS device is removed.")
	if err := d.luks.EnsureClose(ctx, luksInfo.ID, true); err != nil {
		Logc(ctx).WithField("UUID", luksInfo.ID).WithError(err).Warn("Unable to remove LUKS mapper device.")
	}
}

// failingFor starts the clock for key in failures if this is its first failure, and returns the time since then.
func (d *Driver) failingFor(failures map[string]time.Time, key string) time.Duration {
	d.failuresLock.Lock()
	defer d.failuresLock.Unlock()
	if _, ok := failures[key]; !ok {
		failures[key] = time.Now()
	}
	return time.Since(failures[key])
}

func (d *Driver) clearFailure(failures map[string]time.Time, key string) {
	d.failuresLock.Lock()
	defer d.failuresLock.Unlock()
	delete(failures, key)
}
