// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"fmt"

	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/host/adapter/block"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"

	. "github.com/netapp/trident/logging"
)

// Discover is a helper for upgrade paths and other situations
// where the entirety of the attach state must be rebuilt outside of Attach.
// It returns a nil state, and no error, when the namespace is not live on the host.
//
// The namespace UUID is the stable identity used to rebuild the stack.
func (d *Driver) Discover(
	ctx context.Context, publishInfo *models.VolumePublishInfo,
) (*models.VolumeAttachState, error) {
	if publishInfo == nil {
		return nil, fmt.Errorf("accessInfo is required")
	}

	nqn := publishInfo.NVMeSubsystemNQN
	nsUUID := publishInfo.NVMeNamespaceUUID
	fields := LogFields{
		"subsystem": nqn,
		"namespace": nsUUID,
	}
	Logc(ctx).WithFields(fields).Info("Discovering NVMe volume attachment info.")

	// Start with what we know.
	attachState := &models.VolumeAttachState{}
	attachState.VolumeAccessInfo = publishInfo.VolumeAccessInfo

	// The namespace head persists in sysfs even when every path is down (ctrl-loss-tmo=-1).
	// Discover is a snapshot for upgrade/reconciliation; a path may come back after it runs,
	// so continue building the attach state rather than returning nil.
	if !d.nvme.HasLivePath(ctx, nqn) {
		Logc(ctx).WithFields(fields).Warn("NVMe subsystem has no live path; attach state may be incomplete.")
	}

	// Phase 1: Find the namespace by its stored identity.
	nvmeDev, err := d.nvme.DeviceByUUID(ctx, nqn, nsUUID)
	if err != nil {
		if errors.IsNotFoundError(err) {
			Logc(ctx).WithFields(fields).WithError(err).Warn("NVMe namespace not found; volume is not live.")
			return nil, nil
		}
		return nil, err
	}

	// Phase 2: Resolve the full device stack from the namespace upward.
	// ResolveDeviceStack walks holders from the bottom (nvme*n*) through any dm layers
	// and returns them ordered bottom-to-top.
	devices, err := d.block.ResolveDeviceStack(ctx, nvmeDev.Name())
	if err != nil {
		return nil, err
	}

	deviceStack := make(models.BlockDeviceStack, 0, len(devices))
	for _, device := range devices {
		if device == nil {
			continue
		}

		dmInfo := device.DMInfo()
		if dmInfo == nil {
			// No dm info means this is the raw namespace. Use the namespace UUID.
			deviceStack = append(deviceStack, nvmeDev.Info())
			attachState.DevicePath = nvmeDev.Path()
			continue
		}

		switch dmInfo.Target() {
		case block.DMTargetCrypt:
			luksDev, err := d.luks.DeviceByUUID(ctx, dmInfo.UUID())
			if err != nil {
				if errors.IsNotFoundError(err) {
					Logc(ctx).WithFields(fields).WithField("dmUUID", dmInfo.UUID()).
						Warn("LUKS device disappeared during discovery; skipping crypt layer.")
					continue
				}
				return nil, err
			}
			if luksDev == nil {
				return nil, fmt.Errorf("no LUKS device returned for dm UUID %s", dmInfo.UUID())
			}

			deviceStack = append(deviceStack, models.BlockDeviceInfo{
				ID:   luksDev.ID(),
				Name: luksDev.Name(),
				Type: models.BlockDeviceCrypt,
				Base: nvmeDev.ID(),
			})
			attachState.DevicePath = luksDev.Path()
		default:
			// dm-multipath over NVMe is unsupported, so an mpath layer is never expected here.
			Logc(ctx).WithField("dmUUID", dmInfo.UUID()).
				Debug("Skipping unrecognized dm target in device stack.")
		}
	}

	attachState.SharedTarget = publishInfo.SharedTarget
	attachState.Filesystem = publishInfo.FilesystemType
	if convert.ToBool(publishInfo.LUKSEncryption) {
		attachState.Encryption = models.EncryptionLUKS
	}
	attachState.GlobalMount = publishInfo.GlobalMount
	attachState.BlockAttachInfo.Devices = deviceStack
	return attachState, nil
}

// resolveDevices fills in the device stack of a state that has none, as a tracking file written by an older
// Trident has. If the namespace is gone, the volume's open LUKS mapper, named from internalID, is used instead
// so that detach can still close it.
func (d *Driver) resolveDevices(ctx context.Context, state *models.VolumeAttachState, internalID string) error {
	if len(state.BlockAttachInfo.Devices) > 0 {
		return nil
	}

	discovered, err := d.Discover(ctx, &models.VolumePublishInfo{VolumeAccessInfo: state.VolumeAccessInfo})
	if err != nil {
		return fmt.Errorf("failed to get NVMe device; %w", err)
	}
	if discovered != nil {
		state.BlockAttachInfo = discovered.BlockAttachInfo
		return nil
	}

	if !state.Encryption.IsLUKS() || internalID == "" {
		return nil
	}
	crypt, err := d.mapperByInternalID(ctx, internalID)
	if err != nil {
		return fmt.Errorf("failed to get NVMe device; %w", err)
	}
	if crypt != nil {
		crypt.Base = state.NVMeNamespaceUUID
		state.BlockAttachInfo.Devices = models.BlockDeviceStack{*crypt}
	}
	return nil
}
