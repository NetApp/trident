// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"fmt"
	"time"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	legacyiscsi "github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/nvme"
)

const (
	tridentDeviceInfoPath = "/var/lib/trident/tracking"
	volumeLockTimeout     = 60 * time.Second

	// nvmeSubsystemDisconnectLock is the LockID for the self-healing global lock serializing
	// GetNamespaceCount() and Disconnect() in disconnectNVMeSubsystemIfNeeded below.
	nvmeSubsystemDisconnectLock = "nvmeSubsystemDisconnectLock"
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
		trackingInfo, err := c.localStore.ReadTrackingInfo(ctx, volumeID)
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

// disconnectNVMeSubsystemIfNeeded checks if the subsystem should be disconnected and performs the disconnect
// operation under lock to prevent race conditions with concurrent unstage operations.
// This lock serializes GetNamespaceCount() and Disconnect() operations to ensure accurate namespace counting
// and prevent race conditions where multiple threads might see the same count simultaneously.
func (c *Core) disconnectNVMeSubsystemIfNeeded(
	ctx context.Context, nvmeSubsys nvme.NVMeSubsystemInterface, publishInfo *models.VolumePublishInfo,
) error {
	lockContext := "disconnectNVMeSubsystemIfNeeded"
	if !attemptLock(ctx, lockContext, nvmeSubsystemDisconnectLock, sharedLocksNodeLockTimeout) {
		locks.Unlock(ctx, lockContext, nvmeSubsystemDisconnectLock)
		return errors.MaxWaitExceededError("request waited too long for the lock")
	}
	defer locks.Unlock(ctx, lockContext, nvmeSubsystemDisconnectLock)

	// publishedNVMeSessions is mutated (Add/Remove) under nvmeSelfHealingSessionLock so this read must
	// take that same lock to avoid a concurrent map read/write with NodeStage, NodeUnstage, or self-healing.
	sessionLockContext := "disconnectNVMeSubsystemIfNeeded.SessionRead"
	if !attemptLock(ctx, sessionLockContext, nvmeSelfHealingSessionLock, sharedLocksNodeLockTimeout) {
		locks.Unlock(ctx, sessionLockContext, nvmeSelfHealingSessionLock)
		return errors.MaxWaitExceededError("request waited too long for the lock")
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

	// In-memory sessions show none left, but a concurrent NodeStage may have already attached a namespace
	// for a new pod without recording its session yet (recorded only after format/mount). Checking the
	// host's ground-truth namespace count; if it's >1, another namespace is active, we don't disconnect.
	if hostNsCount, err := nvmeSubsys.GetNamespaceCount(ctx); err != nil {
		Logc(ctx).WithField("subsystem", publishInfo.NVMeSubsystemNQN).WithError(err).Debug(
			"Could not determine host namespace count; proceeding with disconnect based on published sessions.")
	} else if hostNsCount > 1 {
		Logc(ctx).WithFields(LogFields{
			"subsystem":      publishInfo.NVMeSubsystemNQN,
			"hostNamespaces": hostNsCount,
		}).Info("Subsystem still has namespace devices attached on host; skipping disconnect.")
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
