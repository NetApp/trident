// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// Graft connects the subsystem's paths to the request's NVMe target IPs that have none, then waits for the
// namespace to be visible. It never removes a path; see Prune. The subsystem and namespace come from state; a
// request naming a different one, or no target IPs, is a terminal reconciliation error.
// The caller bounds the wait through ctx and owns the tracking file. It returns a copy of state with the
// namespace layer refreshed, and DevicePath too when the namespace is the top device; state itself is not
// changed.
func (d *Driver) Graft(
	ctx context.Context, state *models.VolumeAttachState, req *models.GraftAttachmentRequest,
) (*models.VolumeAttachState, error) {
	nqn := state.NVMeSubsystemNQN
	nsUUID := state.NVMeNamespaceUUID
	targetIPs := req.NVMeTargetIPs

	fields := LogFields{
		"volumeID":  req.VolumeName,
		"subsystem": nqn,
		"namespace": nsUUID,
		"targetIPs": targetIPs,
	}
	Logc(ctx).WithFields(fields).Info("Grafting NVMe attachment.")

	if len(targetIPs) == 0 {
		return nil, errors.TerminalReconciliationError("no NVMe target IPs specified")
	}
	if err := validateIdentity(state, req.VolumeAccessInfo); err != nil {
		return nil, err
	}

	release, err := d.acquireLimiter(ctx, graftKey)
	if err != nil {
		return nil, err
	}
	defer release()

	d.healingLock.RLock()
	defer d.healingLock.RUnlock()

	if err := d.nvme.EnsureConnections(ctx, nqn, targetIPs); err != nil {
		return nil, err
	}
	dev, err := d.nvme.EnsureDevice(ctx, nqn, nsUUID)
	if err != nil {
		return nil, err
	}

	// The namespace may come back under a new kernel name.
	grafted := *state
	grafted.BlockAttachInfo.Devices = make(models.BlockDeviceStack, 0, len(state.BlockAttachInfo.Devices)+1)
	grafted.BlockAttachInfo.Devices = append(grafted.BlockAttachInfo.Devices, dev.Info())
	for _, layer := range state.BlockAttachInfo.Devices {
		if layer.Type != models.BlockDeviceDisk {
			grafted.BlockAttachInfo.Devices = append(grafted.BlockAttachInfo.Devices, layer)
		}
	}
	if _, encrypted := grafted.BlockAttachInfo.Devices.Find(models.BlockDeviceCrypt); !encrypted {
		grafted.DevicePath = dev.Path()
	}

	d.sessionLocker.Lock()
	d.sessionStore.AddPublished(nqn, nsUUID, targetIPs)
	d.sessionLocker.Unlock()

	Logc(ctx).WithFields(fields).Info("NVMe attachment grafted.")
	return &grafted, nil
}

// Prune removes the subsystem's paths to target IPs that are not in the request's NVMe target IPs, keeping at
// least one path. A subsystem with no host paths has nothing to prune. Either way, the request's target IPs
// become the subsystem's published target IPs. The subsystem comes from state; a request naming a different
// subsystem or namespace is a terminal reconciliation error. The caller owns the tracking file.
func (d *Driver) Prune(ctx context.Context, state *models.VolumeAttachState, req *models.PruneAttachmentRequest) error {
	nqn := state.NVMeSubsystemNQN
	targetIPs := req.NVMeTargetIPs
	fields := LogFields{
		"volumeID":  req.VolumeName,
		"subsystem": nqn,
		"namespace": state.NVMeNamespaceUUID,
		"targetIPs": targetIPs,
	}

	if err := validateIdentity(state, req.VolumeAccessInfo); err != nil {
		return err
	}

	release, err := d.acquireLimiter(ctx, pruneKey)
	if err != nil {
		return err
	}
	defer release()

	d.healingLock.RLock()
	defer d.healingLock.RUnlock()

	if err := d.nvme.PrunePaths(ctx, nqn, targetIPs); err != nil {
		if !errors.IsNotFoundError(err) {
			return err
		}
		Logc(ctx).WithFields(fields).Debug(
			"NVMe subsystem has no host paths; recording the desired targets without pruning.")
	}
	d.republish(nqn, state.NVMeNamespaceUUID, targetIPs)
	return nil
}

// validateIdentity rejects a request whose NVMe identity differs from the attachment's. Empty identity fields in
// the request mean "the attached volume".
func validateIdentity(state *models.VolumeAttachState, req models.VolumeAccessInfo) error {
	if req.NVMeSubsystemNQN != "" && req.NVMeSubsystemNQN != state.NVMeSubsystemNQN {
		return errors.TerminalReconciliationError("NVMe subsystem NQN mismatch")
	}
	if req.NVMeNamespaceUUID != "" && req.NVMeNamespaceUUID != state.NVMeNamespaceUUID {
		return errors.TerminalReconciliationError("NVMe namespace UUID mismatch")
	}
	return nil
}
