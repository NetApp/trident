// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

//go:generate mockgen -destination=../../mocks/mock_core/mock_node/mock_chap_client.go github.com/netapp/trident/core/node ChapClient

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"slices"
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

	ipDiscoveryAttempts      = defaultIPDiscoveryAttempts()
	ipDiscoveryRetryInterval = 2 * time.Second
)

// nodeInfoRefreshInterval is a var so tests can shrink it.
var nodeInfoRefreshInterval = 5 * time.Minute

// nodeInfoRefreshEnabled is false off Linux: on Windows only SMB discovery is supported, so a refresh
// would log the same "unsupported" errors every interval. It is a var so tests can enable it.
var nodeInfoRefreshEnabled = runtime.GOOS == "linux"

// defaultIPDiscoveryAttempts returns 1 (no retries) where IP discovery is not implemented and always
// fails, such as Windows.
func defaultIPDiscoveryAttempts() int {
	if runtime.GOOS == "linux" {
		return 3
	}
	return 1
}

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

// logDiscoveryIssue reports a discovery problem. warn is set for the startup registration, so the
// condition is visible at the default log level. The refresh passes false and logs at Debug:
// discovery repeats on a fixed interval, and a host where the probe keeps failing would otherwise
// emit the same warning for as long as the node runs.
func logDiscoveryIssue(ctx context.Context, err error, message string, warn bool) {
	entry := Logc(ctx)
	if err != nil {
		entry = entry.WithError(err)
	}
	if warn {
		entry.Warn(message)
		return
	}
	entry.Debug(message)
}

// buildNodeInfo discovers this host's identity (OS, protocol initiators, IP addresses, active
// services) and packages it into the models.Node the controller expects at registration.
// warnDiscovery logs a discovery problem at Warn; the refresh passes false. It also returns the
// services whose probe returned an error, so their state is unknown. SMB discovery is unsupported
// off Windows, which does not count as an error.
func (c *Core) buildNodeInfo(ctx context.Context, warnDiscovery bool) (*models.Node, []string) {
	if c.hostOS == nil {
		if host, err := c.osutils.GetHostSystemInfo(ctx); err != nil {
			logDiscoveryIssue(ctx, err, "Unable to get host system information.", warnDiscovery)
		} else {
			c.hostOS = &host.OS
			Logc(ctx).WithFields(LogFields{
				"distro":  host.OS.Distro,
				"version": host.OS.Version,
			}).Debug("Discovered host info.")
		}
	}
	hostInfo := models.HostSystem{}
	if c.hostOS != nil {
		hostInfo.OS = *c.hostOS
	}

	iscsiWWN := ""
	iscsiWWNs, err := legacyiscsi.GetInitiatorIqns(ctx)
	if err != nil {
		logDiscoveryIssue(ctx, err, "Problem getting iSCSI initiator name.", warnDiscovery)
	} else if len(iscsiWWNs) == 0 {
		logDiscoveryIssue(ctx, nil, "Could not find iSCSI initiator name.", warnDiscovery)
	} else {
		iscsiWWN = iscsiWWNs[0]
		Logc(ctx).WithField("IQN", iscsiWWN).Debug("Discovered iSCSI initiator name.")
	}

	ips, err := c.discoverIPAddresses(ctx, warnDiscovery)
	if err != nil {
		logDiscoveryIssue(ctx, err, "Could not get IP addresses.", warnDiscovery)
	} else if len(ips) == 0 {
		logDiscoveryIssue(ctx, nil, "Could not find any usable IP addresses.", warnDiscovery)
	} else {
		Logc(ctx).WithField("IP Addresses", ips).Debug("Discovered IP addresses.")
	}

	var hostWWPNMap map[string][]string
	if hostWWPNMap, err = fcp.GetFCPInitiatorTargetMap(ctx); err != nil {
		logDiscoveryIssue(ctx, err, "Problem getting FCP host node port name association.", warnDiscovery)
	}

	// Discover active protocol services on the host.
	var services, unknownServices []string
	nfsActive, err := c.osutils.NFSActiveOnHost(ctx)
	if err != nil {
		unknownServices = append(unknownServices, "NFS")
		logDiscoveryIssue(ctx, err, "Error discovering NFS service on host.", warnDiscovery)
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
			unknownServices = append(unknownServices, "SMB")
			logDiscoveryIssue(ctx, err, "Error discovering SMB service on host.", warnDiscovery)
		}
	}
	if smbActive {
		services = append(services, "SMB")
	}

	iscsiActive, err := c.iscsi.ISCSIActiveOnHost(ctx, hostInfo)
	if err != nil {
		unknownServices = append(unknownServices, "iSCSI")
		logDiscoveryIssue(ctx, err, "Error discovering iSCSI service on host.", warnDiscovery)
	} else if !iscsiActive {
		logDiscoveryIssue(ctx, nil, "iSCSI is not active on this host.", warnDiscovery)
	}
	if iscsiActive {
		services = append(services, "iSCSI")
	}

	var nvmeNQN string
	isNVMeActive, err := c.nvme.NVMeActiveOnHost(ctx)
	if err != nil {
		unknownServices = append(unknownServices, "nvme")
		logDiscoveryIssue(ctx, err, "Error discovering NVMe service on host.", warnDiscovery)
	} else if !isNVMeActive {
		logDiscoveryIssue(ctx, nil, "NVMe is not active on this host.", warnDiscovery)
	}
	if isNVMeActive {
		services = append(services, "nvme")
		nvmeNQN, err = c.nvme.GetHostNqn(ctx)
		if err != nil {
			logDiscoveryIssue(ctx, err, "Problem getting Host NQN.", warnDiscovery)
		} else {
			Logc(ctx).WithField("NQN", nvmeNQN).Debug("Discovered NQN.")
		}
	}

	hostInfo.Services = services

	return &models.Node{
		Name:        c.hostName,
		IQN:         iscsiWWN,
		NQN:         nvmeNQN,
		HostWWPNMap: hostWWPNMap,
		IPs:         ips,
		NodePrep:    nil,
		HostInfo:    &hostInfo,
		Deleted:     false,
		// If the node is already known to the Trident controller's persistence layer, that
		// state will be used instead. Otherwise, node state defaults to clean.
		PublicationState: models.NodeClean,
	}, unknownServices
}

// discoverIPAddresses retries IP discovery a bounded number of times while it fails or finds no
// usable addresses, which can happen while the host network is still being configured at boot.
// The node's export policy rules are built from these addresses, and the periodic node info refresh
// rediscovers them. After the last attempt, the result is returned as-is so registration never blocks
// on a host that genuinely has no usable addresses.
func (c *Core) discoverIPAddresses(ctx context.Context, warnDiscovery bool) ([]string, error) {
	ips, err := c.osutils.GetIPAddresses(ctx)
	for attempt := 1; attempt < ipDiscoveryAttempts && (err != nil || len(ips) == 0); attempt++ {
		logDiscoveryIssue(ctx, err, fmt.Sprintf("No usable IP addresses discovered, will retry (attempt %d).", attempt), warnDiscovery)

		select {
		case <-ctx.Done():
			return ips, err
		case <-time.After(ipDiscoveryRetryInterval):
		}
		ips, err = c.osutils.GetIPAddresses(ctx)
	}
	return ips, err
}

// register registers this node with the Trident controller, retrying with exponential backoff until
// success (or until timeout elapses; zero retries forever). It does nothing when the host identity
// is unchanged since the last success. On success it records that identity and applies the
// controller's log settings, if a LogSetter was configured. warnDiscovery is true for the startup
// registration and false for the periodic refresh.
func (c *Core) register(ctx context.Context, timeout time.Duration, warnDiscovery bool) error {
	if c.controller == nil {
		return errors.New("controller is not configured")
	}

	nodeInfo, unknownServices := c.buildNodeInfo(ctx, warnDiscovery)
	keepRegisteredIdentity(nodeInfo, c.registeredNode, unknownServices)
	if reflect.DeepEqual(c.registeredNode, nodeInfo) {
		return nil
	}

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

	c.registeredNode = nodeInfo
	c.applyRegistrationInfo(ctx, regInfo)
	Logc(ctx).WithFields(LogFields{
		"node":     c.hostName,
		"IQN":      nodeInfo.IQN,
		"NQN":      nodeInfo.NQN,
		"IPs":      nodeInfo.IPs,
		"services": nodeInfo.HostInfo.Services,
	}).Info("Updated Trident controller with node registration.")

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

// startNodeInfoRefresh rediscovers this host every nodeInfoRefreshInterval for as long as the node
// is active. It is kept separate from self-healing, which users can disable.
func (c *Core) startNodeInfoRefresh() {
	if !nodeInfoRefreshEnabled || c.cancelNodeInfoRefresh != nil {
		return
	}

	ctx, cancel := context.WithCancel(
		GenerateRequestContext(nil, "", ContextSourcePeriodic, WorkflowNodeUpdate, LogLayerCore))
	done := make(chan struct{})
	ticker := time.NewTicker(nodeInfoRefreshInterval)
	c.cancelNodeInfoRefresh = cancel
	c.nodeInfoRefreshDone = done

	go func() {
		defer close(done)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_ = c.register(ctx, nodeInfoRefreshInterval, false)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// stopNodeInfoRefresh stops the node info refresh, if it is running, and waits for it to exit.
func (c *Core) stopNodeInfoRefresh() {
	if c.cancelNodeInfoRefresh == nil {
		return
	}
	c.cancelNodeInfoRefresh()
	<-c.nodeInfoRefreshDone
	c.cancelNodeInfoRefresh = nil
}

// keepRegisteredIdentity keeps the last registered value of anything discovery returned empty.
// Discovery reports a failed probe the same way as a missing value, so the refresh must not clear
// identity the controller already has. IPs and FC initiators are only added to, because interrupted
// probes return partial lists: the controller removes missing node IPs from export policies, and
// PublishLUN adds only registered initiators to the igroup. A removed IP address or HBA stays
// registered until the node pod restarts. A registered service is dropped only when its probe
// succeeded; unknownServices lists the services whose probe returned an error.
func keepRegisteredIdentity(discovered, registered *models.Node, unknownServices []string) {
	if registered == nil {
		return
	}
	discovered.HostInfo.Services = keepServices(
		registered.HostInfo.Services, discovered.HostInfo.Services, unknownServices)
	discovered.IPs = appendMissing(registered.IPs, discovered.IPs)
	if discovered.IQN == "" {
		discovered.IQN = registered.IQN
	}
	if discovered.NQN == "" {
		discovered.NQN = registered.NQN
	}
	discovered.HostWWPNMap = mergeHostWWPNMap(registered.HostWWPNMap, discovered.HostWWPNMap)
}

// keepServices returns the registered services that discovery still reports or could not probe,
// followed by newly discovered ones, so an unchanged host yields exactly the registered list.
func keepServices(registered, discovered, unknown []string) []string {
	var kept []string
	for _, service := range registered {
		if slices.Contains(discovered, service) || slices.Contains(unknown, service) {
			kept = append(kept, service)
		}
	}
	return appendMissing(kept, discovered)
}

// mergeHostWWPNMap adds the discovered initiators and their targets to the registered map, so an
// unchanged host yields the registered map.
func mergeHostWWPNMap(registered, discovered map[string][]string) map[string][]string {
	if len(discovered) == 0 {
		return registered
	}
	if len(registered) == 0 {
		return discovered
	}
	merged := make(map[string][]string, len(registered)+len(discovered))
	for initiator, targets := range registered {
		merged[initiator] = slices.Clone(targets)
	}
	for initiator, targets := range discovered {
		merged[initiator] = appendMissing(merged[initiator], targets)
	}
	return merged
}

// appendMissing returns registered followed by the discovered values it lacks, so an unchanged host
// yields exactly the registered values.
func appendMissing(registered, discovered []string) []string {
	merged := slices.Clone(registered)
	for _, value := range discovered {
		if !slices.Contains(merged, value) {
			merged = append(merged, value)
		}
	}
	return merged
}
