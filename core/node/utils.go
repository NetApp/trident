// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"fmt"
	"time"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/pkg/locks/distlock"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	legacyiscsi "github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/nvme"
)

const (
	tridentDeviceInfoPath = "/var/lib/trident/tracking"
	volumeLockTimeout     = 60 * time.Second

	nvmeSubsystemOperationLockPrefix = "nvmeSubsystemOperation:"
)

// acquireVolumeLock serializes per-volume node operations. It mirrors the legacy CSI frontend
// attemptLock behavior: block until the lock is acquired, then fail with MaxWaitExceededError
// (mapped to gRPC Aborted) if the caller waited longer than volumeLockTimeout.
func (c *Core) acquireVolumeLock(ctx context.Context, volumeID string) (release func(), err error) {
	startTime := time.Now()
	c.volumeLocks.Lock(volumeID)
	if time.Since(startTime) > volumeLockTimeout {
		c.volumeLocks.Unlock(volumeID)
		Logc(ctx).Debugf("Request spent more than %v waiting for volume lock", volumeLockTimeout)
		return nil, errors.MaxWaitExceededError("request waited too long for the lock")
	}
	return func() { c.volumeLocks.Unlock(volumeID) }, nil
}

func attemptLock(ctx context.Context, lockContext, lockID string, lockTimeout time.Duration) bool {
	startTime := time.Now()
	locks.Lock(ctx, lockContext, lockID)
	// Fail if the gRPC call came in a long time ago to avoid kubelet 120s timeout
	if time.Since(startTime) > lockTimeout {
		Logc(ctx).Debugf("Request spent more than %v in the queue and timed out", lockTimeout)
		return false
	}
	return true
}

func nvmeSubsystemOperationLockID(subsystemNQN string) string {
	return nvmeSubsystemOperationLockPrefix + subsystemNQN
}

func ensureLUKSVolumePassphrase(
	ctx context.Context, luksDevice luks.Device,
	volumeId string, secrets map[string]string, _ bool,
) error {
	luksPassphraseName, luksPassphrase, previousLUKSPassphraseName,
		previousLUKSPassphrase := luks.GetLUKSPassphrasesFromSecretMap(secrets)
	if luksPassphrase == "" {
		return fmt.Errorf("LUKS passphrase cannot be empty")
	}
	if luksPassphraseName == "" {
		return fmt.Errorf("LUKS passphrase name cannot be empty")
	}

	// Check if passphrase is already up-to-date
	current, err := luksDevice.CheckPassphrase(ctx, luksPassphrase)
	if err != nil {
		return fmt.Errorf("could not verify passphrase %s; %v", luksPassphraseName, err)
	}
	if current {
		Logc(ctx).WithFields(LogFields{
			"volume": volumeId,
		}).Debugf("Current LUKS passphrase name '%s'.", luksPassphraseName)
		// Disabled in all supported versions until 26.06.0. Users must track LUKS passphrases for volumes.
		return nil
	}

	// Check if previous passphrase is set, otherwise we can't rotate
	var previous bool
	if previousLUKSPassphrase != "" {
		if previousLUKSPassphraseName == "" {
			return fmt.Errorf("previous LUKS passphrase name cannot be empty if previous LUKS passphrase is also specified")
		}
		previous, err = luksDevice.CheckPassphrase(ctx, previousLUKSPassphrase)
		if err != nil {
			return fmt.Errorf("could not verify passphrase %s; %v", previousLUKSPassphraseName, err)
		}
	}
	if !previous {
		return fmt.Errorf("no working passphrase provided")
	}
	Logc(ctx).WithFields(LogFields{
		"volume": volumeId,
	}).Debugf("Current LUKS passphrase name '%s'.", previousLUKSPassphraseName)

	// Disabled in all supported versions until 26.06.0. Users must track LUKS passphrases for volumes.
	// Rotate
	Logc(ctx).WithFields(LogFields{
		"volume":                       volumeId,
		"current-luks-passphrase-name": previousLUKSPassphraseName,
		"new-luks-passphrase-name":     luksPassphraseName,
	}).Info("Rotating LUKS passphrase.")
	err = luksDevice.RotatePassphrase(ctx, volumeId, previousLUKSPassphrase, luksPassphrase)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"volume":                       volumeId,
			"current-luks-passphrase-name": previousLUKSPassphraseName,
			"new-luks-passphrase-name":     luksPassphraseName,
		}).WithError(err).Errorf("Failed to rotate LUKS passphrase.")
		return fmt.Errorf("failed to rotate LUKS passphrase; %w", err)
	}
	Logc(ctx).Infof("Rotated LUKS passphrase")
	return nil
}

// getVolumeProtocolFromPublishInfo examines the publish info read from the staging target path and determines
// the protocol type from the volume (File or Block).
func getVolumeProtocolFromPublishInfo(publishInfo *models.VolumePublishInfo) (config.Protocol, error) {
	nfsIP := publishInfo.VolumeAccessInfo.NfsServerIP
	iqn := publishInfo.VolumeAccessInfo.IscsiTargetIQN
	smbPath := publishInfo.SMBPath
	nqn := publishInfo.VolumeAccessInfo.NVMeSubsystemNQN
	fcp := publishInfo.VolumeAccessInfo.FCTargetWWNN

	nfsSet := nfsIP != ""
	iqnSet := iqn != ""
	smbSet := smbPath != ""
	nqnSet := nqn != ""
	fcpSet := fcp != ""

	// Exactly one protocol signal must be set; any other combination is ambiguous (e.g. an
	// NFS+NVMe publish info, which previously misclassified as File since isNfs did not
	// exclude nqnSet) and should be treated as an error rather than silently guessed at.
	isSmb := smbSet && !nfsSet && !iqnSet && !nqnSet && !fcpSet
	isNfs := nfsSet && !iqnSet && !smbSet && !nqnSet && !fcpSet
	isIscsi := iqnSet && !nfsSet && !smbSet && !nqnSet && !fcpSet
	isNVMe := nqnSet && !nfsSet && !smbSet && !iqnSet && !fcpSet
	isFCP := fcpSet && !nfsSet && !smbSet && !iqnSet && !nqnSet

	switch {
	case isSmb, isNfs:
		return config.File, nil
	case isIscsi, isNVMe, isFCP:
		return config.Block, nil
	}

	fields := LogFields{
		"SMBPath":          smbPath,
		"IscsiTargetIQN":   iqn,
		"NfsServerIP":      nfsIP,
		"NVMeSubsystemNQN": nqn,
		"FCTargetWWNN":     fcp,
	}

	errMsg := "unable to infer volume protocol"
	Logc(context.Background()).WithFields(fields).Error(FormatMessageForLog(errMsg))

	return "", errors.New(errMsg)
}

// readAllTrackingFiles reads every volume tracking file known to this host. Some protocol
// drivers (FCP, iSCSI) need visibility into every other published volume's publish info to
// safely disambiguate devices that happen to share a LUN number across different backends.
func (c *Core) readAllTrackingFiles(ctx context.Context) []models.VolumePublishInfo {
	publishInfos := make([]models.VolumePublishInfo, 0)
	volumeIDs := legacyiscsi.GetAllVolumeIDs(ctx, tridentDeviceInfoPath)
	for _, volumeID := range volumeIDs {
		trackingInfo, err := c.nodeHelper.ReadTrackingInfo(ctx, volumeID)
		if err != nil || trackingInfo == nil {
			Logc(ctx).WithError(err).WithFields(LogFields{
				"volumeID": volumeID,
				"isEmpty":  trackingInfo == nil,
			}).Error("Volume tracking file info not found or is empty.")
			continue
		}
		publishInfos = append(publishInfos, trackingInfo.VolumePublishInfo)
	}
	return publishInfos
}

// disconnectNVMeSubsystemIfNeeded checks if the subsystem should be disconnected and performs the disconnect.
// The per-subsystem lock orders disconnect against attach and session registration for the same NQN.
func (c *Core) disconnectNVMeSubsystemIfNeeded(
	ctx context.Context, nvmeSubsys nvme.NVMeSubsystemInterface, publishInfo *models.VolumePublishInfo,
) error {
	return c.disconnectNVMeSubsystem(ctx, nvmeSubsys, publishInfo, nvmeDisconnectUnstage)
}

// disconnectNVMeSubsystemForAttachCleanup disconnects a subsystem after failed attach work has
// released its in-memory owner.
func (c *Core) disconnectNVMeSubsystemForAttachCleanup(
	ctx context.Context, nvmeSubsys nvme.NVMeSubsystemInterface, publishInfo *models.VolumePublishInfo,
) error {
	return c.disconnectNVMeSubsystem(ctx, nvmeSubsys, publishInfo, nvmeDisconnectAttachCleanup)
}

type nvmeDisconnectMode int

const (
	nvmeDisconnectUnstage nvmeDisconnectMode = iota
	nvmeDisconnectAttachCleanup
)

func (c *Core) disconnectNVMeSubsystem(
	ctx context.Context, nvmeSubsys nvme.NVMeSubsystemInterface, publishInfo *models.VolumePublishInfo,
	mode nvmeDisconnectMode,
) error {
	lockContext := "disconnectNVMeSubsystem"
	subsystemLockID := nvmeSubsystemOperationLockID(publishInfo.NVMeSubsystemNQN)
	if !attemptLock(ctx, lockContext, subsystemLockID, sharedLocksNodeLockTimeout) {
		Logc(ctx).Warn("NVMe disconnect check waited longer than expected for the subsystem lock.")
		if mode == nvmeDisconnectUnstage {
			locks.Unlock(ctx, lockContext, subsystemLockID)
			return errors.MaxWaitExceededError("request waited too long for the lock")
		}
	}
	defer locks.Unlock(ctx, lockContext, subsystemLockID)

	// publishedNVMeSessions is mutated (Add/Remove) under nvmeSelfHealingSessionLock so this read must
	// take that same lock to avoid a concurrent map read/write with NodeStage, NodeUnstage, or self-healing.
	sessionLockContext := "disconnectNVMeSubsystem.SessionRead"
	if !attemptLock(ctx, sessionLockContext, nvmeSelfHealingSessionLock, sharedLocksNodeLockTimeout) {
		Logc(ctx).Warn("NVMe disconnect check waited longer than expected for the session lock.")
		if mode == nvmeDisconnectUnstage {
			locks.Unlock(ctx, sessionLockContext, nvmeSelfHealingSessionLock)
			return errors.MaxWaitExceededError("request waited too long for the lock")
		}
	}
	numNs := publishedNVMeSessions.GetNamespaceCountForSession(publishInfo.NVMeSubsystemNQN)
	locks.Unlock(ctx, sessionLockContext, nvmeSelfHealingSessionLock)
	Logc(ctx).WithFields(LogFields{
		"subsystem":      publishInfo.NVMeSubsystemNQN,
		"namespaceCount": numNs,
	}).Info("Checking if subsystem should be disconnected.")

	// Another pod still has a published session; we must not disconnect.
	if numNs > 0 {
		return nil
	}

	// In-memory state shows no active work, so confirm the host has no namespace for this subsystem
	// besides the one being released before tearing the whole subsystem down.
	if otherNamespaces, err := nvmeSubsys.HasNamespacesOtherThan(ctx, publishInfo.NVMeNamespaceUUID); err != nil {
		Logc(ctx).WithField("subsystem", publishInfo.NVMeSubsystemNQN).WithError(err).Debug(
			"Could not determine host namespaces; proceeding with disconnect based on published sessions.")
	} else if otherNamespaces {
		Logc(ctx).WithField("subsystem", publishInfo.NVMeSubsystemNQN).Info(
			"Subsystem still has other namespace devices attached on host; skipping disconnect.")
		return nil
	}

	if err := nvmeSubsys.Disconnect(ctx); err != nil {
		Logc(ctx).WithField(
			"subsystem", publishInfo.NVMeSubsystemNQN,
		).WithError(err).Debug("Error disconnecting subsystem.")
		return err
	}
	return nil
}

// orchestratorErrorForLockError converts lock errors from WithLock
// calls into business logic errors. It should wrap lockErr's instead
// of replacing them.
//
// Example:
//
//	fmt.Errorf("%w: %w", orchestratorErr, lockErr)
func orchestratorErrorForLockError(lockErr error) error {
	switch {
	case errors.Is(lockErr, distlock.ErrLockAcquireConflict):
		return fmt.Errorf("%w: %w", errors.VolumeStateError("lock is unavailable"), lockErr)
	case errors.Is(lockErr, distlock.ErrLockDeleteFailed):
		return fmt.Errorf("%w: %w", errors.InternalError("lock could not be deleted"), lockErr)
	default:
		return lockErr
	}
}
