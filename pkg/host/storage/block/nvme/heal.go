// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"runtime/debug"
	"slices"
	"time"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/host/service/nvme"
	"github.com/netapp/trident/pkg/host/storage"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

const (
	// healPassMaxRuntime is how long a heal pass runs before it yields to waiting operations.
	healPassMaxRuntime = 60 * time.Second
	// subsystemLockTimeout fails an operation that queued this long for a subsystem lock, so that it
	// returns before the kubelet's own timeout.
	subsystemLockTimeout = 60 * time.Second
)

// Start starts the periodic self-healing loop. A non-positive cfg.Interval disables it. The first pass runs
// after cfg.Delay, so that the NVMe and iSCSI loops run apart; later passes run every cfg.Interval.
// Start and Stop are each called once.
func (d *Driver) Start(ctx context.Context, cfg *storage.HealConfig) error {
	if d.driverContext != config.ContextCSI {
		Logc(ctx).WithField("driverContext", d.driverContext).Info("NVMe self-healing is not compatible with this context.")
		return nil
	}
	if cfg.Interval <= 0 {
		Logc(ctx).Info("NVMe self-healing is disabled.")
		return nil
	}
	Logc(ctx).WithFields(LogFields{
		"NVMeSelfHealingInterval": cfg.Interval,
		"delay":                   cfg.Delay,
	}).Info("NVMe self-healing is enabled.")

	delay := cfg.Delay
	if delay <= 0 {
		delay = cfg.Interval
	}
	d.healingStopCh = make(chan any)
	d.healingTicker = time.NewTicker(delay)

	go func() {
		resetTicker := true
		for {
			select {
			case tick := <-d.healingTicker.C:
				Logc(ctx).WithField("tick", tick).Debug("NVMe self-healing is running.")
				d.heal(ctx)
				if resetTicker {
					d.healingTicker.Reset(cfg.Interval)
					resetTicker = false
				}
			case <-d.healingStopCh:
				Logc(ctx).Info("NVMe self-healing stopped.")
				return
			}
		}
	}()

	return nil
}

// Stop stops the self-healing loop.
func (d *Driver) Stop() {
	if d.healingTicker != nil {
		d.healingTicker.Stop()
	}
	if d.healingStopCh != nil {
		close(d.healingStopCh)
	}
}

// Register records a volume that is already attached as published, so that self-healing keeps its paths up.
// The caller registers each volume from its tracking file once, before Start.
func (d *Driver) Register(ctx context.Context, volumeID string, info *models.VolumeTrackingInfo) {
	Logc(ctx).WithFields(LogFields{
		"volumeID":  volumeID,
		"subsystem": info.NVMeSubsystemNQN,
		"namespace": info.NVMeNamespaceUUID,
		"targetIPs": info.NVMeTargetIPs,
	}).Debug("Registering published NVMe session.")

	d.sessionLocker.Lock()
	defer d.sessionLocker.Unlock()
	d.sessionStore.AddPublished(info.NVMeSubsystemNQN, info.NVMeNamespaceUUID, info.NVMeTargetIPs)
}

// lockSubsystem takes the lock that orders publish, unpublish and disconnect for one subsystem. The lock is
// held when it returns, also when ok is false because the wait exceeded subsystemLockTimeout.
func lockSubsystem(ctx context.Context, lockContext, nqn string) (unlock func(), ok bool) {
	lockID := "nvmeSubsystemOperation:" + nqn
	start := time.Now()
	locks.Lock(ctx, lockContext, lockID)
	ok = time.Since(start) <= subsystemLockTimeout
	if !ok {
		Logc(ctx).Debugf("Request spent more than %v waiting for the subsystem lock.", subsystemLockTimeout)
	}
	return func() { locks.Unlock(ctx, lockContext, lockID) }, ok
}

// publishForAttach records the namespace as published before attach connects, so that a concurrent detach
// of another namespace does not disconnect the subsystem. It reports whether this call added the namespace.
func (d *Driver) publishForAttach(ctx context.Context, nqn, nsUUID string, targetIPs []string) (bool, error) {
	unlock, ok := lockSubsystem(ctx, "publishForAttach", nqn)
	defer unlock()
	if !ok {
		return false, errors.MaxWaitExceededError("request waited too long for the lock")
	}

	d.sessionLocker.Lock()
	defer d.sessionLocker.Unlock()
	session := d.sessionStore.Info[nqn]
	added := session == nil || !session.Namespaces[nsUUID]
	d.sessionStore.AddPublished(nqn, nsUUID, targetIPs)
	return added, nil
}

// unpublishAfterAttachError removes the namespace that a failed attach published and disconnects the subsystem
// if no published namespace remains on it. Cleanup goes ahead even if the locks were slow.
func (d *Driver) unpublishAfterAttachError(ctx context.Context, state *models.VolumeAttachState) {
	unlock, ok := lockSubsystem(ctx, "unpublishAfterAttachError", state.NVMeSubsystemNQN)
	if !ok {
		Logc(ctx).Warn("NVMe session cleanup waited longer than expected for the subsystem lock.")
	}
	last := d.unpublish(state.NVMeSubsystemNQN, state.NVMeNamespaceUUID)
	unlock()

	if last {
		if err := d.disconnectIfUnpublished(ctx, state, false); err != nil {
			Logc(ctx).WithError(err).Warn("Error during subsystem disconnect check after attach cleanup.")
		}
	}
}

// unpublish removes the namespace from the published sessions and reports whether it was the subsystem's last.
func (d *Driver) unpublish(nqn, nsUUID string) bool {
	d.sessionLocker.Lock()
	defer d.sessionLocker.Unlock()
	return d.sessionStore.RemovePublished(nqn, nsUUID)
}

// republish replaces the subsystem's published target IPs with targetIPs, as graft and prune changed them.
func (d *Driver) republish(nqn, nsUUID string, targetIPs []string) {
	d.sessionLocker.Lock()
	defer d.sessionLocker.Unlock()
	// AddPublished merges target IPs, so the previous set is removed first.
	if session := d.sessionStore.Info[nqn]; session != nil {
		d.sessionStore.RemoveTargetIPsFromSession(nqn, slices.Clone(session.NVMeTargetIPs))
	}
	d.sessionStore.AddPublished(nqn, nsUUID, targetIPs)
}

// disconnectIfUnpublished disconnects the subsystem if no published namespace remains on it. With strict, a
// slow subsystem lock fails with MaxWaitExceeded instead of going ahead.
func (d *Driver) disconnectIfUnpublished(ctx context.Context, state *models.VolumeAttachState, strict bool) error {
	nqn := state.NVMeSubsystemNQN
	unlock, ok := lockSubsystem(ctx, "disconnectIfUnpublished", nqn)
	defer unlock()
	if !ok {
		Logc(ctx).Warn("NVMe disconnect check waited longer than expected for the subsystem lock.")
		if strict {
			return errors.MaxWaitExceededError("request waited too long for the lock")
		}
	}

	d.sessionLocker.RLock()
	numNs := d.sessionStore.GetNamespaceCountForSession(nqn)
	d.sessionLocker.RUnlock()
	Logc(ctx).WithFields(LogFields{
		"subsystem":      nqn,
		"namespaceCount": numNs,
	}).Info("Checking if subsystem should be disconnected.")

	// Another volume still has a published namespace on the subsystem.
	if numNs > 0 {
		return nil
	}
	return d.disconnect(ctx, state)
}

// heal runs one self-healing pass: it reconnects the missing paths of published subsystems. Operations wait
// for the pass; once it has run healPassMaxRuntime, it stops early if any are waiting.
func (d *Driver) heal(ctx context.Context) {
	d.healingLock.Lock()
	defer d.healingLock.Unlock()
	d.sessionLocker.Lock()
	defer d.sessionLocker.Unlock()

	defer func() {
		if r := recover(); r != nil {
			Logc(ctx).Errorf("Panic in NVMeSelfHealing. \nStack Trace: %v", string(debug.Stack()))
		}
	}()

	if d.sessionStore.IsEmpty() {
		Logc(ctx).Debug("No NVMe volumes are published. Self healing is not required.")
		return
	}

	stopAt := time.Now().Add(healPassMaxRuntime)
	d.sessionStore.ResetRemediationForAll()
	Logc(ctx).Debugf("Published NVMe sessions %v.", d.sessionStore)

	nqns := d.inspectSessions(ctx, &d.sessionStore)

	Logc(ctx).Debug("Start NVMe healing.")
	for index, nqn := range nqns {
		if !d.sessionStore.SessionExists(nqn) {
			continue
		}
		if index > 0 && d.healingLock.Waiting() > 0 && time.Now().After(stopAt) {
			Logc(ctx).Info("Self-healing has exceeded maximum runtime; preempting NVMe session self-healing.")
			break
		}
		d.rectifySession(ctx, nqn, &d.sessionStore)
	}
	Logc(ctx).Debug("NVMe healing finished.")
}

// inspectSessions compares the published sessions with what the host has. A published subsystem that has some
// of its published target IPs without a path, while it still has at least one path, needs a connect: its NQN is
// returned and its Remediation is set to ConnectOp. A subsystem with all published paths gets its LastAccessTime
// updated. The returned NQNs are sorted by LastAccessTime, oldest first.
//
// A subsystem that cannot be read from the host is logged and skipped; it does not stop the sweep.
func (d *Driver) inspectSessions(ctx context.Context, published *Sessions) []string {
	var nqnsToFix []string
	if published.IsEmpty() {
		return nqnsToFix
	}

	for nqn, data := range published.Info {
		if data == nil {
			continue
		}

		sub, err := d.nvme.Subsystem(ctx, nqn)
		if err != nil {
			if errors.IsNotFoundError(err) {
				// The service reports an absent subsystem and one with no paths the same way.
				Logc(ctx).Warnf("Published nqn %s not present in current sessions.", nqn)
			} else {
				Logc(ctx).WithField("Error", err).Errorf("Failed to get subsystem %s: %v", nqn, err)
			}
			continue
		}

		if len(missingTargetIPs(sub, data.NVMeTargetIPs)) == 0 {
			// All desired paths are present for the subsystem.
			data.LastAccessTime = time.Now()
			continue
		}

		if len(sub.Paths()) == 0 {
			// We can reconnect with all the subsystem paths in this case, but that leads to change in the NVMe device
			// names which are already mounted. So, we don't do self-healing in this case. It is better that the Admin
			// takes care of this issue and then restart the node.
			Logc(ctx).Warnf("All the paths to %s subsystem are down. Please check the network connectivity to"+
				" these IPs %v.", nqn, data.NVMeTargetIPs)
			continue
		}

		// At least one path of the subsystem is connected. We should try to connect with other remaining paths.
		data.SetRemediation(ConnectOp)
		nqnsToFix = append(nqnsToFix, nqn)
	}

	published.SortByLastAccess(nqnsToFix)
	return nqnsToFix
}

// rectifySession applies the remediation set by inspectSessions on the subsystem to make it working again.
// A path in any state counts as connected: the kernel keeps trying to recover it, so only IPs without a path
// are connected, once.
func (d *Driver) rectifySession(ctx context.Context, nqn string, published *Sessions) {
	if published.IsEmpty() {
		return
	}

	data := published.Info[nqn]
	if data == nil {
		return
	}

	// Updating the access time as we are trying to do some NVMeOperation on this subsystem.
	data.LastAccessTime = time.Now()

	if data.Remediation == ConnectOp {
		if err := d.nvme.ConnectPaths(ctx, nqn, data.NVMeTargetIPs); err != nil {
			Logc(ctx).Errorf("NVMe Self healing failed for subsystem %s; %v", nqn, err)
		} else {
			Logc(ctx).Infof("NVMe Self healing succeeded for %s", nqn)
		}
	}
}

// missingTargetIPs returns the target IPs that have no path, in any state, on the subsystem. Path addresses
// are bare IPs here; the adapter has already parsed them out of the sysfs traddr field.
func missingTargetIPs(sub *nvme.Subsystem, targetIPs []string) []string {
	var missing []string
	paths := sub.Paths()
	for _, ip := range targetIPs {
		present := false
		for _, p := range paths {
			if p.Address() == ip {
				present = true
				break
			}
		}
		if !present {
			missing = append(missing, ip)
		}
	}
	return missing
}
