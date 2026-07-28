// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/multierr"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/nvme"
)

const (
	iSCSILoginTimeout               = 10 * time.Second
	iSCSISelfHealingTimeout         = 60 * time.Second
	defaultNodeReconciliationPeriod = 1 * time.Minute
	maximumNodeReconciliationJitter = 5000 * time.Millisecond

	// LockID Constants for the self-healing global locks.
	iSCSISelfHealingSessionLock = "iSCSISelfHealingSessionLock"
	nvmeSelfHealingSessionLock  = "nvmeSelfHealingSessionLock"
)

// currentISCSISessions/currentNVMeSessions track the sessions actually observed on this host
// during the most recent self-healing sweep, for comparison against the published sessions
// recorded from tracking files.
var (
	currentISCSISessions   *models.ISCSISessions
	publishedISCSISessions *models.ISCSISessions

	iSCSISelfHealingLock           = sync.RWMutex{}
	iSCSINodeOperationWaitingCount atomic.Int32

	currentNVMeSessions   nvme.NVMeSessions
	publishedNVMeSessions nvme.NVMeSessions

	// NVMe Self-healing lock and counter for parallelism
	nvmeSelfHealingLock           = sync.RWMutex{}
	nvmeNodeOperationWaitingCount atomic.Int32
)

// populatePublishedSessions rebuilds the in-memory published iSCSI/NVMe session bookkeeping
// from the on-disk tracking files. Callers should invoke this once at startup, before self-healing
// begins, so self-healing has an accurate view of what should be attached.
func (c *Core) populatePublishedSessions(ctx context.Context) {
	volumeIDs := iscsi.GetAllVolumeIDs(ctx, tridentDeviceInfoPath)
	for _, volumeID := range volumeIDs {
		trackingInfo, err := c.localStore.ReadTrackingInfo(ctx, volumeID)
		if err != nil || trackingInfo == nil {
			Logc(ctx).WithFields(LogFields{
				"volumeID": volumeID,
				"error":    err,
				"isEmpty":  trackingInfo == nil,
			}).Error("Volume tracking file info not found or is empty.")
			continue
		}

		publishInfo := &trackingInfo.VolumePublishInfo
		if publishInfo.StorageProtocol != NVMe {
			newCtx := context.WithValue(ctx, iscsi.SessionInfoSource, iscsi.SessionSourceTrackingInfo)
			c.iscsi.AddSession(newCtx, publishedISCSISessions, publishInfo, volumeID, "", models.NotInvalid)
		} else {
			c.nvme.AddPublishedNVMeSession(&publishedNVMeSessions, publishInfo)
		}
	}
}

// startSelfHealing starts the periodic iSCSI and NVMe self-healing background sweeps. It is an
// internal lifecycle detail driven by Activate/Deactivate; callers outside this package have no
// business starting or stopping self-healing independently of the Core's own lifecycle.
func (c *Core) startSelfHealing(ctx context.Context) {
	c.startISCSISelfHealingThread(ctx)
	c.startNVMeSelfHealingThread(ctx)
}

// stopSelfHealing gracefully stops the periodic iSCSI and NVMe self-healing background sweeps.
func (c *Core) stopSelfHealing(ctx context.Context) {
	c.stopISCSISelfHealingThread(ctx)
	c.stopNVMeSelfHealingThread(ctx)
}

// startISCSISelfHealingThread starts the iSCSI self-healing thread to heal faulty sessions.
func (c *Core) startISCSISelfHealingThread(ctx context.Context) {
	if c.iSCSISelfHealingInterval <= 0 {
		Logc(ctx).Info("iSCSI self-healing is disabled.")
		return
	}
	if c.iSCSISelfHealingWaitTime < c.iSCSISelfHealingInterval {
		// Stale session wait time is not advised to be smaller than self-heal interval
		c.iSCSISelfHealingWaitTime = time.Duration(1.5 * float64(c.iSCSISelfHealingInterval))
	}

	Logc(ctx).WithFields(LogFields{
		"iSCSISelfHealingInterval": c.iSCSISelfHealingInterval,
		"iSCSISelfHealingWaitTime": c.iSCSISelfHealingWaitTime,
	}).Info("iSCSI self-healing is enabled.")
	c.iSCSISelfHealingTicker = time.NewTicker(c.iSCSISelfHealingInterval)
	c.iSCSISelfHealingChannel = make(chan struct{})

	go func() {
		ctx = GenerateRequestContext(nil, "", ContextSourcePeriodic, WorkflowNodeHealISCSI, LogLayerCore)

		for {
			select {
			case tick := <-c.iSCSISelfHealingTicker.C:
				Logc(ctx).WithField("tick", tick).Debug("iSCSI self-healing is running.")
				c.performISCSISelfHealing(ctx)
			case <-c.iSCSISelfHealingChannel:
				Logc(ctx).Info("iSCSI self-healing stopped.")
				return
			}
		}
	}()
}

// stopISCSISelfHealingThread stops the iSCSI self-healing thread.
func (c *Core) stopISCSISelfHealingThread(_ context.Context) {
	if c.iSCSISelfHealingTicker != nil {
		c.iSCSISelfHealingTicker.Stop()
	}
	if c.iSCSISelfHealingChannel != nil {
		close(c.iSCSISelfHealingChannel)
	}
}

// startNVMeSelfHealingThread starts the NVMe self-healing thread to heal faulty sessions.
func (c *Core) startNVMeSelfHealingThread(ctx context.Context) {
	if c.nvmeSelfHealingInterval <= 0 {
		Logc(ctx).Info("NVMe self-healing is disabled.")
		return
	}

	Logc(ctx).WithFields(LogFields{
		"NVMeSelfHealingInterval": c.nvmeSelfHealingInterval,
	}).Info("NVMe self-healing is enabled.")
	// Halve the interval initially to jitter the iSCSI and NVMe self-healing threads apart; reset
	// to the proper interval after the first run.
	c.nvmeSelfHealingTicker = time.NewTicker(c.nvmeSelfHealingInterval / 2)
	c.nvmeSelfHealingChannel = make(chan struct{})

	go func() {
		ctx = GenerateRequestContext(nil, "", ContextSourcePeriodic, WorkflowNodeHealNVMe, LogLayerCore)
		resetTicker := true

		for {
			select {
			case tick := <-c.nvmeSelfHealingTicker.C:
				Logc(ctx).WithField("tick", tick).Debug("NVMe self-healing is running.")
				c.performNVMeSelfHealing(ctx)
				if resetTicker {
					c.nvmeSelfHealingTicker.Reset(c.nvmeSelfHealingInterval)
					resetTicker = false
				}
			case <-c.nvmeSelfHealingChannel:
				Logc(ctx).Info("NVMe self-healing stopped.")
				return
			}
		}
	}()
}

// stopNVMeSelfHealingThread stops the NVMe self-healing thread.
func (c *Core) stopNVMeSelfHealingThread(_ context.Context) {
	if c.nvmeSelfHealingTicker != nil {
		c.nvmeSelfHealingTicker.Stop()
	}
	if c.nvmeSelfHealingChannel != nil {
		close(c.nvmeSelfHealingChannel)
	}
}

// performISCSISelfHealing inspects the desired state of the iSCSI sessions against the current
// state and remediates any sessions that have drifted. Invoked periodically.
func (c *Core) performISCSISelfHealing(ctx context.Context) {
	iSCSISelfHealingLock.Lock()
	defer iSCSISelfHealingLock.Unlock()

	lockContext := "performISCSISelfHealing.SessionLock"
	defer locks.Unlock(ctx, lockContext, iSCSISelfHealingSessionLock)
	if !attemptLock(ctx, lockContext, iSCSISelfHealingSessionLock, sharedLocksNodeLockTimeout) {
		Logc(ctx).WithError(fmt.Errorf("request waited too long for the lock"))
		return
	}

	defer func() {
		if r := recover(); r != nil {
			Logc(ctx).Errorf("Panic in iSCSISelfHealing. \nStack Trace: %v", string(debug.Stack()))
		}
	}()

	stopSelfHealingAt := time.Now().Add(iSCSISelfHealingTimeout)
	if publishedISCSISessions.IsEmpty() {
		Logc(ctx).Debug("Skipping iSCSI self-heal cycle; no iSCSI volumes published on the host.")
		return
	}

	if err := c.iscsi.PreChecks(ctx); err != nil {
		Logc(ctx).Errorf("Skipping iSCSI self-heal cycle; pre-checks failed: %v.", err)
		return
	}

	currentISCSISessions = models.NewISCSISessions()

	if err := publishedISCSISessions.ResetAllRemediationValues(); err != nil {
		Logc(ctx).WithError(err).Error("Failed to reset remediation value(s) for published iSCSI sessions.")
	}

	if err := c.iscsi.PopulateCurrentSessions(ctx, currentISCSISessions); err != nil {
		Logc(ctx).WithError(err).
			Error("Failed to get current state of iSCSI Sessions LUN mappings; skipping iSCSI self-heal cycle.")
		return
	}

	if currentISCSISessions.IsEmpty() {
		Logc(ctx).Debug("No iSCSI sessions LUN mappings found.")
	}

	if err := c.updateCHAPInfoForSessions(ctx, publishedISCSISessions, currentISCSISessions); err != nil {
		Logc(ctx).WithError(err).Error("Failed to update CHAP credentials for published iSCSI sessions.")
	}

	Logc(ctx).Debugf("Published iSCSI Sessions: %v", publishedISCSISessions)
	Logc(ctx).Debugf("Current iSCSI Sessions: %v", currentISCSISessions)

	// SELF-HEAL STEP 1: Identify all sorted candidate stale portals and sorted candidate non-stale portals.
	staleISCSIPortals, nonStaleISCSIPortals := c.iscsi.InspectAllISCSISessions(ctx, publishedISCSISessions,
		currentISCSISessions, c.iSCSISelfHealingWaitTime)

	// SELF-HEAL STEP 2: Attempt to fix all the stale portals.
	c.fixISCSISessions(ctx, staleISCSIPortals, "stale", stopSelfHealingAt)

	// SELF-HEAL STEP 3: Attempt to fix at-least one of the non-stale portals.
	c.fixISCSISessions(ctx, nonStaleISCSIPortals, "non-stale", stopSelfHealingAt)
}

// fixISCSISessions iterates through the given portals, identifies their respective remediation,
// and rectifies each one.
func (c *Core) fixISCSISessions(ctx context.Context, portals []string, portalType string, stopAt time.Time) {
	if len(portals) == 0 {
		Logc(ctx).Debugf("No %s iSCSI portal found.", portalType)
		return
	}

	Logc(ctx).Debugf("Found %s portal(s) that require remediation.", portalType)

	for _, portal := range portals {
		if iSCSINodeOperationWaitingCount.Load() > 0 {
			if time.Now().After(stopAt) {
				Logc(ctx).Debug("Self-healing has exceeded maximum runtime; preempting iSCSI session self-healing.")
				break
			}
		}

		fixAction := publishedISCSISessions.Info[portal].Remediation

		Logc(ctx).Debugf("Attempting to fix iSCSI portal %v it requires %s", portal, fixAction)

		publishedISCSISessions.Info[portal].PortalInfo.LastAccessTime = time.Now()

		if err := c.selfHealingRectifySession(ctx, portal, fixAction); err != nil {
			Logc(ctx).WithError(err).Errorf("Encountered error while attempting to fix portal %v.", portal)
		} else {
			Logc(ctx).Debugf("Fixed portal %v it required %s", portal, fixAction)
			publishedISCSISessions.Info[portal].PortalInfo.ResetFirstIdentifiedStaleAt()
		}
	}
}

// selfHealingRectifySession rectifies a session identified as a ghost session: re-logs in if
// necessary and scans for LUNs.
func (c *Core) selfHealingRectifySession(ctx context.Context, portal string, action models.ISCSIAction) error {
	Logc(ctx).WithFields(LogFields{
		"portal": portal,
		"action": action,
	}).Debug("ISCSI self-healing rectify session is invoked.")
	publishInfo, err := publishedISCSISessions.GeneratePublishInfo(portal)
	if err != nil {
		return fmt.Errorf("failed to get publish info for session on portal '%s'; %v", portal, err)
	}

	switch action {
	case models.LogoutLoginScan:
		if isAccessible, err := c.iscsi.IsPortalAccessible(ctx, portal); !isAccessible {
			Logc(ctx).WithError(err).Warnf("Cannot safely log out of unresponsive portal '%s'.", portal)
			return fmt.Errorf("cannot safely log out of unresponsive portal '%s'", portal)
		}

		if err = c.iscsi.Logout(ctx, publishInfo.IscsiTargetIQN, portal); err != nil {
			return fmt.Errorf("error while logging out of target %s", publishInfo.IscsiTargetIQN)
		}
		Logc(ctx).Debug("Logout is successful.")

		fallthrough
	case models.LoginScan:
		// Only heal session connectivity here; skip mount/filesystem work.
		publishInfo.FilesystemType = filesystem.Raw
		volumeID, err := publishedISCSISessions.VolumeIDForPortalAndLUN(portal, publishInfo.IscsiLunNumber)
		if err != nil {
			return fmt.Errorf("failed to get volume ID for lun ID; %v", err)
		}

		publishedCHAPCredentials := publishInfo.IscsiChapInfo
		if _, err = c.ensureAttachISCSIVolume(ctx, volumeID, publishInfo, iSCSILoginTimeout); err != nil {
			return fmt.Errorf("failed to login to the target")
		}

		if publishedCHAPCredentials != publishInfo.IscsiChapInfo {
			fields := LogFields{"portal": portal, "CHAPInUse": true}
			if updateErr := publishedISCSISessions.UpdateCHAPForPortal(portal, publishInfo.IscsiChapInfo); updateErr != nil {
				Logc(ctx).WithFields(fields).Warn("Failed to update published CHAP information.")
			}
			Logc(ctx).WithFields(fields).Debug("Updated published CHAP information after successful login.")
		}

		Logc(ctx).Debug("Login to target is successful.")
		fallthrough
	case models.Scan:
		// This detection may be useful for support in the future. Retain until there's a good
		// reason to remove it.
		_ = c.deprecatedIgroupInUse(ctx)

		luns, err := publishedISCSISessions.LUNsForPortal(portal)
		if err != nil {
			return fmt.Errorf("failed to get LUNs for portal: %s; %w", portal, err)
		}

		if err = iscsi.InitiateScanForLuns(ctx, luns, publishInfo.IscsiTargetIQN); err != nil {
			Logc(ctx).WithError(err).Error("Could not initiate scan for some LUNs.")
			return fmt.Errorf("failed to initiate scan for LUNs in portal: %s; %w", portal, err)
		}

		Logc(ctx).WithFields(LogFields{
			"portal": portal,
			"luns":   luns,
			"target": publishInfo.IscsiTargetIQN,
		}).Debug("Successfully initiated iSCSI scan(s).")
	default:
		Logc(ctx).Debug("No valid action to be taken in iSCSI self-healing.")
	}

	return nil
}

// deprecatedIgroupInUse looks through the tracking files for deprecated igroups and reports if
// any are in use. Precise LUN scanning removes the need for this, but it remains useful for
// debugging and support, and the calculation is cheap.
func (c *Core) deprecatedIgroupInUse(ctx context.Context) bool {
	volumeTrackingInfo, _ := c.localStore.ListVolumeTrackingInfo(ctx)
	for id, info := range volumeTrackingInfo {
		if !iscsi.IsPerNodeIgroup(info.IscsiIgroup) {
			Logc(ctx).WithFields(LogFields{
				"volumeID": id,
				"lunID":    info.IscsiLunNumber,
				"igroup":   info.IscsiIgroup,
			}).Debug("Detected a deprecated igroup.")
			return true
		}
	}

	Logc(ctx).Debug("No deprecated igroups detected.")
	return false
}

// updateCHAPInfoForSessions provides a best attempt to populate up-to-date CHAP credentials
// within iSCSI self-healing's published sessions, tracking credentials by unique IQN to reduce
// calls to the controller.
func (c *Core) updateCHAPInfoForSessions(ctx context.Context, publishedSessions, currentSessions *models.ISCSISessions) error {
	if publishedSessions == nil || currentSessions == nil {
		return nil
	}

	// Timebox this operation so it can't block self-healing or other node operations indefinitely.
	cancelCtx, cancel := context.WithTimeout(ctx, iSCSISelfHealingTimeout/3)
	defer cancel()

	// IQNs should be unique between SVMs and CHAP credentials are scoped at the SVM level.
	iqnToCHAP := make(map[string]*models.IscsiChapInfo)
	var errs error

	for portal, publishedData := range publishedSessions.Info {
		data, ok := currentSessions.Info[portal]
		if ok && !c.iscsi.IsSessionStale(cancelCtx, data.PortalInfo.SessionNumber) {
			continue
		} else if !publishedData.PortalInfo.CHAPInUse() || !publishedData.PortalInfo.HasTargetIQN() {
			continue
		}

		chapInfo, ok := iqnToCHAP[publishedData.PortalInfo.ISCSITargetIQN]
		if !ok {
			volumeID, err := publishedSessions.VolumeIDForPortal(portal)
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf("failed to get volume ID for portal: '%s'; %w", portal, err))
				continue
			}

			chapInfo, err = c.controller.CHAPInfo(cancelCtx, volumeID, c.hostName)
			if err != nil {
				errs = errors.Join(errs, fmt.Errorf("failed to get CHAP info for portal: '%s'; %w", portal, err))
				continue
			}

			iqnToCHAP[publishedData.PortalInfo.ISCSITargetIQN] = chapInfo
		}

		publishedData.PortalInfo.UpdateCHAPCredentials(*chapInfo)
		Logc(cancelCtx).WithField("portal", portal).Debug("Updated CHAP info for portal.")
	}

	if errs != nil {
		Logc(cancelCtx).WithError(errs).Error("Failed to get updated CHAP info for portal(s).")
		return errs
	} else if len(iqnToCHAP) == 0 {
		Logc(cancelCtx).Debug("No outdated CHAP info found in published sessions.")
		return nil
	}

	Logc(cancelCtx).Debug("Updated CHAP info for published sessions.")
	return nil
}

// performNVMeSelfHealing inspects the desired state of the NVMe sessions against the current
// state and remediates any that have drifted. Invoked periodically.
func (c *Core) performNVMeSelfHealing(ctx context.Context) {
	nvmeSelfHealingLock.Lock()
	defer nvmeSelfHealingLock.Unlock()

	lockContext := "performNVMeSelfHealing.SessionLock"
	defer locks.Unlock(ctx, lockContext, nvmeSelfHealingSessionLock)
	if !attemptLock(ctx, lockContext, nvmeSelfHealingSessionLock, sharedLocksNodeLockTimeout) {
		Logc(ctx).WithError(fmt.Errorf("request waited too long for the lock"))
		return
	}

	defer func() {
		if r := recover(); r != nil {
			Logc(ctx).Errorf("Panic in NVMeSelfHealing. \nStack Trace: %v", string(debug.Stack()))
		}
	}()

	if publishedNVMeSessions.IsEmpty() {
		Logc(ctx).Debug("No NVMe volumes are published. Self healing is not required.")
		return
	}

	stopSelfHealingAt := time.Now().Add(60 * time.Second)

	publishedNVMeSessions.ResetRemediationForAll()
	currentNVMeSessions = nvme.NVMeSessions{}

	if err := c.nvme.PopulateCurrentNVMeSessions(ctx, &currentNVMeSessions); err != nil {
		Logc(ctx).Errorf("Failed to populate current sessions %v.", err)
		return
	}

	Logc(ctx).Debugf("Published NVMe sessions %v.", publishedNVMeSessions)
	Logc(ctx).Debugf("Current NVMe sessions %v.", currentNVMeSessions)

	subsToFix := c.nvme.InspectNVMeSessions(ctx, &publishedNVMeSessions, &currentNVMeSessions)

	Logc(ctx).Debug("Start NVMe healing.")
	c.fixNVMeSessions(ctx, stopSelfHealingAt, subsToFix)
	Logc(ctx).Debug("NVMe healing finished.")
}

func (c *Core) fixNVMeSessions(ctx context.Context, stopAt time.Time, subsystems []nvme.NVMeSubsystem) {
	for index, sub := range subsystems {
		if !publishedNVMeSessions.CheckNVMeSessionExists(sub.NQN) {
			continue
		}

		if index > 0 && nvmeNodeOperationWaitingCount.Load() > 0 && time.Now().After(stopAt) {
			Logc(ctx).Info("Self-healing has exceeded maximum runtime; preempting NVMe session self-healing.")
			break
		}

		c.nvme.RectifyNVMeSession(ctx, sub, &publishedNVMeSessions)
	}
}

// startPublicationReconciliation starts an infinite background task to periodically reconcile the node's
// actual volume publication state against the desired state known to the Trident controller. Stale
// publications (volumes attached/staged on the node with no matching publication record on the controller)
// are force-detached. This is a no-op if force detach is not enabled. Like startSelfHealing, this is an
// internal lifecycle detail owned by Activate/Deactivate, not something external callers should drive.
func (c *Core) startPublicationReconciliation(ctx context.Context) {
	if !c.enableForceDetach {
		return
	}

	Logc(ctx).Info("Activating node publication reconciliation service.")
	c.nodePublicationTimer = time.NewTimer(defaultNodeReconciliationPeriod)
	c.stopNodePublicationLoop = make(chan bool)

	go func() {
		ctx := GenerateRequestContext(nil, "", ContextSourcePeriodic, WorkflowNodeReconcilePubs, LogLayerCore)

		for {
			select {
			case <-c.stopNodePublicationLoop:
				return

			case <-c.nodePublicationTimer.C:
				Logc(ctx).Debug("Reconciling node publication state.")
				if err := c.reconcileNodePublicationState(ctx); err != nil {
					Logc(ctx).WithError(err).Debug("Failed to reconcile node publication state.")
					continue
				}
				Logc(ctx).Debug("Reconciled node publication state.")
			}
		}
	}()
}

// stopPublicationReconciliation gracefully stops the node publication reconciliation background task.
func (c *Core) stopPublicationReconciliation(ctx context.Context) {
	if !c.enableForceDetach {
		return
	}

	Logc(ctx).Info("Stopping the node publication reconciliation service.")

	if c.nodePublicationTimer != nil {
		if !c.nodePublicationTimer.Stop() {
			<-c.nodePublicationTimer.C
		}
	}

	if c.stopNodePublicationLoop != nil {
		close(c.stopNodePublicationLoop)
	}
}

// refreshTimerPeriod resets the time period between node cleanup executions.
// It introduces randomness (jitter) between reconciliation periods to avoid a thundering herd on the controller API.
func (c *Core) refreshTimerPeriod(ctx context.Context) time.Duration {
	Logc(ctx).Debug("Refreshing node publication reconcile timer")

	jitter := maximumNodeReconciliationJitter
	if n, err := rand.Int(rand.Reader, big.NewInt(int64(maximumNodeReconciliationJitter))); err == nil {
		jitter = time.Duration(n.Int64())
	}
	return defaultNodeReconciliationPeriod + jitter
}

// reconcileNodePublicationState cleans any stale published path for volumes on the node by rectifying the actual
// state of publications (published paths on the node) against the desired state of publications from the Trident
// controller. If all published paths are cleaned successfully and the node is cleanable, it updates the Trident
// node CR via the controller API. If a node is not in a cleanable state, it will not mark the node as clean.
func (c *Core) reconcileNodePublicationState(ctx context.Context) error {
	defer func() {
		// Reset the Timer only after the cleanup process is complete, regardless of if it fails or not.
		c.nodePublicationTimer.Reset(c.refreshTimerPeriod(ctx))
	}()

	// For force detach purposes, always get the node's cleanup state and check if it needs updating.
	nodeState, err := c.controller.GetNodeCleanupStatus(ctx, c.hostName)
	if err != nil {
		Logc(ctx).WithError(err).Error("Failed to get node state from the Trident controller.")
		return err
	}

	// For now, only cleanup the node iff the node is not clean.
	if nodeState == models.NodeClean {
		Logc(ctx).Debug("Node is clean, nothing to do.")
		return nil
	}

	if err := c.performNodeCleanup(ctx); err != nil {
		Logc(ctx).WithError(err).Error("Failed to clean stale node publications.")
		return err
	}

	return c.updateNodePublicationState(ctx, nodeState)
}

// PerformNodeCleanup will discover the difference between the volume tracking information stored on the node, and
// the publication records stored in the controller's persistence. It will then force unstage any stale volume
// attachments and remove their relevant tracking files. This is only intended to be called after the node has
// registered with the controller, and Bootstrap does so (register, then Activate, mirroring the old CSI frontend's
// ordering). Unlike the old frontend, the data-path gate here does not open until this - and the rest of Activate -
// has also finished, closing a race where an Attach could run concurrently with cleanup's tracking-file/publication
// comparison and be misclassified as stale.
func (c *Core) performNodeCleanup(ctx context.Context) error {
	Logc(ctx).Debug("Performing node cleanup.")

	// Discover the desired publication state.
	desiredPublicationState, err := c.discoverDesiredPublicationState(ctx)
	if err != nil {
		return errors.WrapWithReconcileFailedError(err, "reconcile failed")
	}

	// Discover the actual publication state.
	actualPublicationState, err := c.discoverActualPublicationState(ctx)
	if err != nil {
		return errors.WrapWithReconcileFailedError(err, "reconcile failed")
	}

	// Check for stale publication records. If any exist, clean them.
	stalePublications := c.discoverStalePublications(ctx, actualPublicationState, desiredPublicationState)
	if len(stalePublications) != 0 {
		if err = c.cleanStalePublications(ctx, stalePublications); err != nil {
			return errors.WrapWithReconcileFailedError(err, "reconcile failed")
		}
	}

	return nil
}

// discoverDesiredPublicationState discovers the desired state of published volumes on the Trident controller and
// returns a mapping of volumeID -> publications.
func (c *Core) discoverDesiredPublicationState(ctx context.Context) (
	map[string]*models.VolumePublicationExternal, error,
) {
	Logc(ctx).Debug("Discovering desired publication state.")

	desiredPublicationState, err := c.controller.GetDesiredPublications(ctx, c.hostName)
	if err != nil {
		return nil, fmt.Errorf("failed to get desired publication state")
	}

	return desiredPublicationState, nil
}

// discoverActualPublicationState discovers the actual state of published volumes on the node and returns
// a mapping of volumeID -> tracking information.
func (c *Core) discoverActualPublicationState(ctx context.Context) (map[string]*models.VolumeTrackingInfo, error) {
	Logc(ctx).Debug("Discovering actual publication state.")

	actualPublicationState, err := c.localStore.ListVolumeTrackingInfo(ctx)
	if err != nil && !errors.IsNotFoundError(err) {
		return nil, fmt.Errorf("failed to get actual publication state")
	}

	return actualPublicationState, nil
}

// discoverStalePublications compares the actual state of publications with the desired state
// of publications in the controller and returns the delta between the two.
func (c *Core) discoverStalePublications(
	ctx context.Context,
	actualPublicationState map[string]*models.VolumeTrackingInfo,
	desiredPublicationState map[string]*models.VolumePublicationExternal,
) map[string]*models.VolumeTrackingInfo {
	Logc(ctx).Debug("Discovering stale volume publications.")

	// Track the delta between actual (node-side) and desired (controller-side) publication state.
	stalePublications := make(map[string]*models.VolumeTrackingInfo, 0)

	// Reconcile the actual state of publications to the desired state of publications.
	for volumeID, trackingInfo := range actualPublicationState {
		fields := LogFields{"volumeID": volumeID}

		// If we find the publication in the desired state, then we don't want to do anything.
		// Otherwise, remove the published paths and tracking info on the node.
		if _, ok := desiredPublicationState[volumeID]; !ok {
			Logc(ctx).WithFields(fields).Debug("Volume has no matching volume publication record.")
			stalePublications[volumeID] = trackingInfo
		}
	}

	return stalePublications
}

// cleanStalePublications cleans published paths on the host node for attached volumes with no matching publication
// object in the Trident controller. It should never publish volumes to the node.
func (c *Core) cleanStalePublications(
	ctx context.Context, stalePublications map[string]*models.VolumeTrackingInfo,
) error {
	Logc(ctx).Debug("Cleaning stale node publication state.")

	// Clean stale volume publication state.
	var err error
	for volumeID, trackingInfo := range stalePublications {
		forceDetach := func() error {
			release, lockErr := c.acquireVolumeLock(ctx, volumeID)
			if lockErr != nil {
				return lockErr
			}
			defer release()

			var forceDetachErr error
			var fields LogFields

			// If no published paths exist for a still staged volume, then it means CO / kubelet
			// died before it could finish CSI unpublish and unstage for this given volume.
			// These unpublish calls act as a best-effort to abide by and act within the CSI workflow.
			for targetPath := range trackingInfo.PublishedPaths {
				fields := LogFields{
					"volumeID":   volumeID,
					"targetPath": targetPath,
				}

				unpublishErr := c.unmountGeneric(ctx, volumeID, targetPath)

				if unpublishErr != nil {
					Logc(ctx).WithFields(fields).WithError(unpublishErr).Debug("Failed to unpublish volume.")
					forceDetachErr = fmt.Errorf("failed to unpublish volume; %v", unpublishErr)
				} else {
					Logc(ctx).WithFields(fields).Debug("Unpublished stale volume.")
				}
			}
			fields = LogFields{
				"volumeID":          volumeID,
				"stagingTargetPath": trackingInfo.GlobalMount,
			}

			if unstageErr := c.detach(ctx, volumeID, true); unstageErr != nil {
				Logc(ctx).WithFields(fields).WithError(unstageErr).Debug("Failed to force unstage volume.")
				forceDetachErr = fmt.Errorf("failed to force unstage volume; %v", unstageErr)
			} else {
				Logc(ctx).WithFields(fields).Debug("Force detached stale volume attachment.")
			}
			return forceDetachErr
		}

		err = multierr.Combine(err, forceDetach())
	}

	return err
}

// updateNodePublicationState marks the node as clean/ready with the Trident controller once local cleanup
// has succeeded, but only if the controller believes the node is cleanable.
func (c *Core) updateNodePublicationState(ctx context.Context, nodeState models.NodePublicationState) error {
	if nodeState != models.NodeCleanable {
		Logc(ctx).Debugf("Controller node state is not cleanable; state was: [%s]", nodeState)
		return nil
	}

	Logc(ctx).Debug("Updating node publication state.")
	if err := c.controller.MarkNodeCleanupComplete(ctx, c.hostName); err != nil {
		Logc(ctx).WithError(err).Error("Failed to update node publication state.")
		return err
	}
	Logc(ctx).Debug("Updated node publication state.")

	return nil
}
