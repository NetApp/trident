// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

//go:generate mockgen -destination=../../mocks/mock_core/mock_node/mock_chap_client.go github.com/netapp/trident/core/node ChapClient

import (
	"context"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/netapp/trident/frontend/csi/tridentcontroller"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/fcp"
	legacyiscsi "github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/osutils"
)

// Backoff timing for node registration retries is exposed as vars (not consts) so tests can
// shrink them; see crd.Client's similarly-motivated backupAckPollBase et al.
var (
	registerBackoffInitialInterval     = 10 * time.Second
	registerBackoffMultiplier          = 2.0
	registerBackoffMaxInterval         = 120 * time.Second
	registerBackoffRandomizationFactor = 0.1
)

// ChapClient is the subset of the legacy Trident controller REST surface the node core still
// needs directly: CHAP credential lookup has not been migrated onto the tridentcontroller.Client
// abstraction. Any type satisfying this (e.g. controller_api.TridentController) can be combined
// with a tridentcontroller.Client via NewController.
type ChapClient interface {
	GetChap(ctx context.Context, volumeID, nodeName string) (*models.IscsiChapInfo, error)
}

// controllerFromClients composes a tridentcontroller.Client (registration, desired publication
// state, cleanup status) with a ChapClient (CHAP lookups) into the node core's Controller
// dependency. It holds no reference to the CSI frontend, so wiring it does not create a
// dependency from the node core back to the node server.
type controllerFromClients struct {
	tridentcontroller.Client
	chap ChapClient
}

// CHAPInfo returns the CHAP credentials for the given volume/node pair from the Trident controller.
func (c *controllerFromClients) CHAPInfo(
	ctx context.Context, volumeID, nodeName string,
) (*models.IscsiChapInfo, error) {
	return c.chap.GetChap(ctx, volumeID, nodeName)
}

// NewController combines a controller-transport client (CRD or REST, selected by the caller) with
// the legacy REST CHAP lookup into the node core's Controller dependency. Callers - typically
// whoever assembles the CSI frontend's controller clients - construct the pieces and hand them
// here; the returned Controller is self-contained and does not call back into its caller.
func NewController(client tridentcontroller.Client, chap ChapClient) Controller {
	return &controllerFromClients{Client: client, chap: chap}
}

// buildNodeInfo discovers this host's identity (protocol initiators, IP addresses, active
// services) and packages it into the models.Node the controller expects at registration. Host
// system info is cached after the first successful lookup since it does not change at runtime.
func (c *Core) buildNodeInfo(ctx context.Context) *models.Node {
	if c.hostInfo == nil {
		host, err := c.osutils.GetHostSystemInfo(ctx)
		if err != nil {
			c.hostInfo = &models.HostSystem{}
			Logc(ctx).WithError(err).Warn("Unable to get host system information.")
		} else {
			c.hostInfo = host
			Logc(ctx).WithFields(LogFields{
				"distro":  host.OS.Distro,
				"version": host.OS.Version,
			}).Debug("Discovered host info.")
		}
	}

	iscsiWWN := ""
	iscsiWWNs, err := legacyiscsi.GetInitiatorIqns(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Problem getting iSCSI initiator name.")
	} else if len(iscsiWWNs) == 0 {
		Logc(ctx).Warn("Could not find iSCSI initiator name.")
	} else {
		iscsiWWN = iscsiWWNs[0]
		Logc(ctx).WithField("IQN", iscsiWWN).Info("Discovered iSCSI initiator name.")
	}

	ips, err := c.osutils.GetIPAddresses(ctx)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Could not get IP addresses.")
	} else if len(ips) == 0 {
		Logc(ctx).Warn("Could not find any usable IP addresses.")
	} else {
		Logc(ctx).WithField("IP Addresses", ips).Info("Discovered IP addresses.")
	}

	var hostWWPNMap map[string][]string
	if hostWWPNMap, err = fcp.GetFCPInitiatorTargetMap(ctx); err != nil {
		Logc(ctx).WithError(err).Warn("Problem getting FCP host node port name association.")
	}

	// Discover active protocol services on the host.
	var services []string
	nfsActive, err := c.osutils.NFSActiveOnHost(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Error discovering NFS service on host.")
	}
	if nfsActive {
		services = append(services, "NFS")
	}

	smbActive, err := osutils.SMBActiveOnHost(ctx)
	if err != nil {
		if errors.IsUnsupportedError(err) {
			// SMB discovery is not supported on this platform (e.g. Linux); this is expected on
			// every registration attempt, so it doesn't warrant a warning.
			Logc(ctx).WithError(err).Debug("SMB service discovery is not supported on this platform.")
		} else {
			Logc(ctx).WithError(err).Warn("Error discovering SMB service on host.")
		}
	}
	if smbActive {
		services = append(services, "SMB")
	}

	iscsiActive, err := c.iscsi.ISCSIActiveOnHost(ctx, *c.hostInfo)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Error discovering iSCSI service on host.")
	}
	if iscsiActive {
		services = append(services, "iSCSI")
	}

	var nvmeNQN string
	isNVMeActive, err := c.nvme.NVMeActiveOnHost(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Error discovering NVMe service on host.")
	}
	if isNVMeActive {
		services = append(services, "nvme")
		nvmeNQN, err = c.nvme.GetHostNqn(ctx)
		if err != nil {
			Logc(ctx).WithError(err).Warn("Problem getting Host NQN.")
		} else {
			Logc(ctx).WithField("NQN", nvmeNQN).Debug("Discovered NQN.")
		}
	} else {
		Logc(ctx).Info("NVMe is not active on this host.")
	}

	c.hostInfo.Services = services

	return &models.Node{
		Name:        c.hostName,
		IQN:         iscsiWWN,
		NQN:         nvmeNQN,
		HostWWPNMap: hostWWPNMap,
		IPs:         ips,
		NodePrep:    nil,
		HostInfo:    c.hostInfo,
		Deleted:     false,
		// If the node is already known to the Trident controller's persistence layer, that
		// state will be used instead. Otherwise, node state defaults to clean.
		PublicationState: models.NodeClean,
	}
}

// register builds this node's identity and registers it with the Trident controller, retrying
// with exponential backoff until success (or until timeout elapses; zero retries forever). On
// success it applies the controller's log settings, if a LogSetter was configured.
func (c *Core) register(ctx context.Context, timeout time.Duration) error {
	if c.controller == nil {
		return errors.New("controller is not configured")
	}

	nodeInfo := c.buildNodeInfo(ctx)

	var regInfo *tridentcontroller.RegistrationInfo
	registerNode := func() error {
		info, registerErr := c.controller.RegisterNode(ctx, nodeInfo, timeout)
		if registerErr != nil {
			return registerErr
		}
		regInfo = info
		return nil
	}

	registerNodeNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithFields(LogFields{
			"increment": duration,
			"error":     err,
			"node":      c.hostName,
		}).Warn("Could not update Trident controller with node registration, will retry.")
	}

	registerNodeBackoff := backoff.NewExponentialBackOff()
	registerNodeBackoff.InitialInterval = registerBackoffInitialInterval
	registerNodeBackoff.Multiplier = registerBackoffMultiplier
	registerNodeBackoff.MaxInterval = registerBackoffMaxInterval
	registerNodeBackoff.RandomizationFactor = registerBackoffRandomizationFactor
	registerNodeBackoff.MaxElapsedTime = timeout

	// WithContext makes RetryNotify select on ctx.Done() rather than only ever giving up when
	// MaxElapsedTime elapses (which never happens for the timeout=0/"retry forever" case Bootstrap
	// uses). This lets a canceled/deadline-exceeded ctx abort the retry loop promptly.
	if err := backoff.RetryNotify(registerNode, backoff.WithContext(registerNodeBackoff, ctx), registerNodeNotify); err != nil {
		Logc(ctx).WithError(err).Error("Unable to update Trident controller with node registration.")
		return err
	}

	c.applyRegistrationInfo(ctx, regInfo)
	Logc(ctx).WithField("node", c.hostName).Info("Updated Trident controller with node registration.")

	return nil
}

// applyRegistrationInfo aligns local logging with the controller's settings, if a
// LogSetter is configured. Topology (K8s zone/region labels) is a Kubernetes-native
// concept unrelated to registration or the Trident controller - the CSI frontend already sources
// it directly from the K8s Node object via controllerHelper.GetNodeTopologyLabels/IsTopologyInUse,
// so it is intentionally not part of the node core's responsibilities.
func (c *Core) applyRegistrationInfo(ctx context.Context, info *tridentcontroller.RegistrationInfo) {
	if info == nil || c.logSettings == nil {
		return
	}
	if info.LogLevel != "" {
		if err := c.logSettings.SetLogLevel(ctx, info.LogLevel); err != nil {
			Logc(ctx).WithError(err).Error("Unable to set log level from controller node registration.")
		}
	}
	if info.LogWorkflows != "" {
		if err := c.logSettings.SetLoggingWorkflows(ctx, info.LogWorkflows); err != nil {
			Logc(ctx).WithError(err).Error("Unable to set logging workflows from controller node registration.")
		}
	}
	if info.LogLayers != "" {
		if err := c.logSettings.SetLogLayers(ctx, info.LogLayers); err != nil {
			Logc(ctx).WithError(err).Error("Unable to set logging layers from controller node registration.")
		}
	}
}
