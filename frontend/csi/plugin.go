// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	controllerAPI "github.com/netapp/trident/frontend/csi/controller_api"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/errors"
	execCmd "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/fcp"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/limiter"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount"
	"github.com/netapp/trident/utils/nvme"
	"github.com/netapp/trident/utils/osutils"
)

const (
	CSIController = "controller"
	CSINode       = "node"
	CSIAllInOne   = "allInOne"
)

type Plugin struct {
	orchestrator core.Orchestrator

	name     string
	nodeName string
	version  string
	endpoint string
	role     string

	unsafeDetach      bool
	enableForceDetach bool

	hostInfo *models.HostSystem

	command execCmd.Command

	restClient       controllerAPI.TridentController
	controllerHelper controllerhelpers.ControllerHelper
	nodeHelper       nodehelpers.NodeHelper

	aesKey []byte

	grpc NonBlockingGRPCServer

	csCap  []*csi.ControllerServiceCapability
	nsCap  []*csi.NodeServiceCapability
	vCap   []*csi.VolumeCapability_AccessMode
	gcsCap []*csi.GroupControllerServiceCapability

	topologyInUse bool

	opCache sync.Map

	nodeIsRegistered bool

	limiterSharedMap map[string]limiter.Limiter

	iSCSISelfHealingTicker   *time.Ticker
	iSCSISelfHealingChannel  chan struct{}
	iSCSISelfHealingInterval time.Duration
	iSCSISelfHealingWaitTime time.Duration

	stopNodePublicationLoop chan bool
	nodePublicationTimer    *time.Timer

	nvmeHandler nvme.NVMeInterface

	nvmeSelfHealingTicker   *time.Ticker
	nvmeSelfHealingChannel  chan struct{}
	nvmeSelfHealingInterval time.Duration

	iscsi   iscsi.ISCSI
	devices devices.Devices
	mount   mount.Mount
	fs      filesystem.Filesystem
	fcp     fcp.FCP
	osutils osutils.Utils

	csi.UnimplementedIdentityServer
	csi.UnimplementedNodeServer
	csi.UnimplementedControllerServer
	csi.UnimplementedGroupControllerServer
}

func NewControllerPlugin(
	nodeName, endpoint, aesKeyFile string, orchestrator core.Orchestrator, helper controllerhelpers.ControllerHelper,
	enableForceDetach bool,
) (*Plugin, error) {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginCreate, LogLayerCSIFrontend)

	Logc(ctx).Info("Initializing CSI controller frontend.")

	p := &Plugin{
		orchestrator:      orchestrator,
		name:              Provisioner,
		nodeName:          nodeName,
		version:           tridentconfig.OrchestratorVersion.ShortString(),
		endpoint:          endpoint,
		role:              CSIController,
		controllerHelper:  helper,
		enableForceDetach: enableForceDetach,
		opCache:           sync.Map{},
		command:           execCmd.NewCommand(),
		osutils:           osutils.New(),
	}

	var err error
	p.aesKey, err = ReadAESKey(ctx, aesKeyFile)
	if err != nil {
		return nil, err
	}

	// Define controller capabilities
	p.addControllerServiceCapabilities(ctx, []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES,
		csi.ControllerServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
	})

	// Define volume capabilities
	p.addVolumeCapabilityAccessModes(ctx, []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	// Define group controller capabilities
	p.addGroupControllerServiceCapabilities(ctx, []csi.GroupControllerServiceCapability_RPC_Type{
		csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT,
	})

	return p, nil
}

func NewNodePlugin(
	nodeName, endpoint, caCert, clientCert, clientKey, aesKeyFile string, orchestrator core.Orchestrator,
	unsafeDetach bool, helper nodehelpers.NodeHelper, enableForceDetach bool,
	iSCSISelfHealingInterval, iSCSIStaleSessionWaitTime, nvmeSelfHealingInterval time.Duration,
) (*Plugin, error) {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginCreate, LogLayerCSIFrontend)

	Logc(ctx).Info("Initializing CSI node frontend.")

	msg := "Force detach feature %s"
	if enableForceDetach {
		msg = fmt.Sprintf(msg, "enabled.")
	} else {
		msg = fmt.Sprintf(msg, "disabled.")
	}
	Logc(ctx).Info(msg)

	// NewClient() must plugin default implementation of the various package clients.
	iscsiClient, err := iscsi.New()
	if err != nil {
		return nil, err
	}

	mountClient, err := mount.New()
	if err != nil {
		return nil, err
	}

	fs := filesystem.New(mountClient)

	fcpClient, err := fcp.New()
	if err != nil {
		return nil, err
	}

	p := &Plugin{
		orchestrator:             orchestrator,
		name:                     Provisioner,
		nodeName:                 nodeName,
		version:                  tridentconfig.OrchestratorVersion.ShortString(),
		endpoint:                 endpoint,
		role:                     CSINode,
		nodeHelper:               helper,
		enableForceDetach:        enableForceDetach,
		unsafeDetach:             unsafeDetach,
		opCache:                  sync.Map{},
		limiterSharedMap:         make(map[string]limiter.Limiter),
		iSCSISelfHealingInterval: iSCSISelfHealingInterval,
		iSCSISelfHealingWaitTime: iSCSIStaleSessionWaitTime,
		nvmeHandler:              nvme.NewNVMeHandler(),
		nvmeSelfHealingInterval:  nvmeSelfHealingInterval,
		iscsi:                    iscsiClient,
		// NewClient() must plugin default implementation of the various package clients.
		fcp:     fcpClient,
		devices: devices.New(),
		mount:   mountClient,
		fs:      fs,
		command: execCmd.NewCommand(),
		osutils: osutils.New(),
	}

	if runtime.GOOS == "windows" {
		p.addNodeServiceCapabilities(
			[]csi.NodeServiceCapability_RPC_Type{
				csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
				csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
			},
		)
	} else {
		p.addNodeServiceCapabilities(
			[]csi.NodeServiceCapability_RPC_Type{
				csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
				csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
				csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
				csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
			},
		)
	}

	port := os.Getenv("TRIDENT_CSI_SERVICE_PORT")
	if port == "" {
		port = "34571"
	}

	hostname := os.Getenv("TRIDENT_CSI_SERVICE_HOST")
	if hostname == "" {
		hostname = tridentconfig.ServerCertName
	}

	restURL := "https://" + hostname + ":" + port
	p.restClient, err = controllerAPI.CreateTLSRestClient(restURL, caCert, clientCert, clientKey)
	if err != nil {
		return nil, err
	}

	p.aesKey, err = ReadAESKey(ctx, aesKeyFile)
	if err != nil {
		return nil, err
	}
	// Define volume capabilities
	p.addVolumeCapabilityAccessModes(ctx,
		[]csi.VolumeCapability_AccessMode_Mode{
			csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
			csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
			csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
			csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
			csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		},
	)

	return p, nil
}

// The NewAllInOnePlugin is required to support the CSI Sanity test suite.
// CSI Sanity expects a single process to respond to controller, node, and
// identity interfaces.
func NewAllInOnePlugin(
	nodeName, endpoint, caCert, clientCert, clientKey, aesKeyFile string, orchestrator core.Orchestrator,
	controllerHelper controllerhelpers.ControllerHelper, nodeHelper nodehelpers.NodeHelper, unsafeDetach bool,
	iSCSISelfHealingInterval, iSCSIStaleSessionWaitTime, nvmeSelfHealingInterval time.Duration,
) (*Plugin, error) {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginCreate, LogLayerCSIFrontend)

	Logc(ctx).Info("Initializing CSI all-in-one frontend.")

	// TODO (vivintw) the adaptors are being plugged in here as a temporary measure to prevent cyclic dependencies.
	// NewClient() must plugin default implementation of the various package clients.
	iscsiClient, err := iscsi.New()
	if err != nil {
		return nil, err
	}

	mountClient, err := mount.New()
	if err != nil {
		return nil, err
	}

	fs := filesystem.New(mountClient)

	fcpClient, err := fcp.New()
	if err != nil {
		return nil, err
	}

	p := &Plugin{
		orchestrator:             orchestrator,
		name:                     Provisioner,
		nodeName:                 nodeName,
		version:                  tridentconfig.OrchestratorVersion.ShortString(),
		endpoint:                 endpoint,
		role:                     CSIAllInOne,
		unsafeDetach:             unsafeDetach,
		controllerHelper:         controllerHelper,
		nodeHelper:               nodeHelper,
		opCache:                  sync.Map{},
		limiterSharedMap:         make(map[string]limiter.Limiter),
		iSCSISelfHealingInterval: iSCSISelfHealingInterval,
		iSCSISelfHealingWaitTime: iSCSIStaleSessionWaitTime,
		nvmeHandler:              nvme.NewNVMeHandler(),
		nvmeSelfHealingInterval:  nvmeSelfHealingInterval,
		iscsi:                    iscsiClient,
		fcp:                      fcpClient,
		devices:                  devices.New(),
		mount:                    mountClient,
		fs:                       fs,
		command:                  execCmd.NewCommand(),
		osutils:                  osutils.New(),
	}

	port := "34571"
	for _, envVar := range os.Environ() {
		values := strings.Split(envVar, "=")
		if values[0] == "TRIDENT_CSI_SERVICE_PORT" {
			port = values[1]
			break
		}
	}
	restURL := "https://" + tridentconfig.ServerCertName + ":" + port
	p.restClient, err = controllerAPI.CreateTLSRestClient(restURL, caCert, clientCert, clientKey)
	if err != nil {
		return nil, err
	}

	p.aesKey, err = ReadAESKey(ctx, aesKeyFile)
	if err != nil {
		return nil, err
	}

	// Define controller capabilities
	p.addControllerServiceCapabilities(ctx, []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES,
		csi.ControllerServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
	})

	p.addNodeServiceCapabilities([]csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
		csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
		csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
	})

	// Define volume capabilities
	p.addVolumeCapabilityAccessModes(ctx, []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	p.addGroupControllerServiceCapabilities(ctx, []csi.GroupControllerServiceCapability_RPC_Type{
		csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT,
	})

	return p, nil
}

func (p *Plugin) Activate() error {
	go func() {
		ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginActivate, LogLayerCSIFrontend)
		p.grpc = NewNonBlockingGRPCServer()

		fields := LogFields{"nodeName": p.nodeName, "role": p.role}
		Logc(ctx).WithFields(fields).Info("Activating CSI frontend.")

		if p.role == CSINode || p.role == CSIAllInOne {
			p.nodeRegisterWithController(ctx, 0) // Retry indefinitely

			// Initialize node scalability limiter.
			p.InitializeNodeLimiter(ctx)

			// Initialize publishedISCSISessions and currentISCSISessions,
			// during activation to prevent concurrent threads from setting it to nil for each other.
			// Just an additional safeguard.
			publishedISCSISessions = models.NewISCSISessions()
			currentISCSISessions = models.NewISCSISessions()

			// Cleanup any stale volume publication state immediately so self-healing works with current data.
			if err := p.performNodeCleanup(ctx); err != nil {
				Logc(ctx).WithError(err).Warn("Failed to clean node; self-healing features may be unreliable.")
			}

			// Populate the published sessions IFF iSCSI/NVMe self-healing is enabled.
			if p.iSCSISelfHealingInterval > 0 || p.nvmeSelfHealingInterval > 0 {
				p.populatePublishedSessions(ctx)
			}

			p.startISCSISelfHealingThread(ctx)
			p.startNVMeSelfHealingThread(ctx)

			if p.enableForceDetach {
				p.startReconcilingNodePublications(ctx)
			}
		}

		if p.role == CSIController || p.role == CSIAllInOne {
			// Check if topology is in use and store it in the plugin
			topologyInUse := p.controllerHelper.IsTopologyInUse(ctx)
			p.topologyInUse = topologyInUse
		}

		p.grpc.Start(p.endpoint, p, p, p, p)
	}()
	return nil
}

func (p *Plugin) Deactivate() error {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginDeactivate, LogLayerCSIFrontend)

	// stopReconcilingNodePublications
	if p.role == CSINode || p.role == CSIAllInOne {
		if p.enableForceDetach {
			p.stopReconcilingNodePublications(ctx)
		}
	}

	Logc(ctx).Info("Deactivating CSI frontend.")
	p.grpc.GracefulStop()

	// Stop iSCSI self-healing thread
	p.stopISCSISelfHealingThread(ctx)
	p.stopNVMeSelfHealingThread(ctx)

	return nil
}

func (p *Plugin) GetName() string {
	return string(tridentconfig.ContextCSI)
}

func (p *Plugin) Version() string {
	return tridentconfig.OrchestratorVersion.String()
}

func (p *Plugin) addControllerServiceCapabilities(ctx context.Context, cl []csi.ControllerServiceCapability_RPC_Type) {
	var csCap []*csi.ControllerServiceCapability

	for _, c := range cl {
		Logc(ctx).WithField("capability", c.String()).Info("Enabling controller service capability.")
		csCap = append(csCap, NewControllerServiceCapability(c))
	}

	p.csCap = csCap
}

func (p *Plugin) addNodeServiceCapabilities(cl []csi.NodeServiceCapability_RPC_Type) {
	var nsCap []*csi.NodeServiceCapability

	for _, c := range cl {
		Log().WithField("capability", c.String()).Info("Enabling node service capability.")
		nsCap = append(nsCap, NewNodeServiceCapability(c))
	}

	p.nsCap = nsCap
}

func (p *Plugin) addVolumeCapabilityAccessModes(ctx context.Context, vc []csi.VolumeCapability_AccessMode_Mode) {
	var vCap []*csi.VolumeCapability_AccessMode

	for _, c := range vc {
		Logc(ctx).WithField("mode", c.String()).Info("Enabling volume access mode.")
		vCap = append(vCap, NewVolumeCapabilityAccessMode(c))
	}

	p.vCap = vCap
}

func (p *Plugin) addGroupControllerServiceCapabilities(ctx context.Context, cl []csi.GroupControllerServiceCapability_RPC_Type) {
	var gcsCap []*csi.GroupControllerServiceCapability

	for _, c := range cl {
		Logc(ctx).WithField("capability", c.String()).Info("Enabling group controller service capability.")
		gcsCap = append(gcsCap, NewGroupControllerServiceCapability(c))
	}

	p.gcsCap = gcsCap
}

func (p *Plugin) getCSIErrorForOrchestratorError(err error) error {
	if errors.IsNotReadyError(err) {
		return status.Error(codes.Unavailable, err.Error())
	} else if errors.IsBootstrapError(err) {
		return status.Error(codes.FailedPrecondition, err.Error())
	} else if errors.IsNotFoundError(err) {
		return status.Error(codes.NotFound, err.Error())
	} else if ok, errPtr := errors.HasUnsupportedCapacityRangeError(err); ok && errPtr != nil {
		return status.Error(codes.OutOfRange, errPtr.Error())
	} else if errors.IsFoundError(err) {
		return status.Error(codes.AlreadyExists, err.Error())
	} else if errors.IsNodeNotSafeToPublishForBackendError(err) {
		return status.Error(codes.FailedPrecondition, err.Error())
	} else if errors.IsVolumeCreatingError(err) {
		return status.Error(codes.DeadlineExceeded, err.Error())
	} else if errors.IsVolumeDeletingError(err) {
		return status.Error(codes.DeadlineExceeded, err.Error())
	} else if ok, errPtr := errors.HasResourceExhaustedError(err); ok && errPtr != nil {
		return status.Error(codes.ResourceExhausted, err.Error())
	} else if errors.IsInvalidInputError(err) {
		return status.Error(codes.InvalidArgument, err.Error())
	} else {
		return status.Error(codes.Unknown, err.Error())
	}
}

func ReadAESKey(ctx context.Context, aesKeyFile string) ([]byte, error) {
	var aesKey []byte
	var err error

	if "" != aesKeyFile {
		aesKey, err = os.ReadFile(aesKeyFile)
		if err != nil {
			return nil, err
		}
	} else {
		Logc(ctx).Warn("AES encryption key not provided!")
	}
	return aesKey, nil
}

func (p *Plugin) IsReady() bool {
	return p.nodeIsRegistered
}

// startISCSISelfHealingThread starts the iSCSI self-healing thread to heal faulty sessions.
func (p *Plugin) startISCSISelfHealingThread(ctx context.Context) {
	// provision to disable the iSCSI self-healing feature
	if p.iSCSISelfHealingInterval <= 0 {
		Logc(ctx).Info("iSCSI self-healing is disabled.")
		return
	}
	if p.iSCSISelfHealingWaitTime < p.iSCSISelfHealingInterval {
		// Stale session wait time is not advised to be smaller than self-heal interval
		p.iSCSISelfHealingWaitTime = time.Duration(1.5 * float64(p.iSCSISelfHealingInterval))
	}

	Logc(ctx).WithFields(LogFields{
		"iSCSISelfHealingInterval": p.iSCSISelfHealingInterval,
		"iSCSISelfHealingWaitTime": p.iSCSISelfHealingWaitTime,
	}).Info("iSCSI self-healing is enabled.")
	p.iSCSISelfHealingTicker = time.NewTicker(p.iSCSISelfHealingInterval)
	p.iSCSISelfHealingChannel = make(chan struct{})

	go func() {
		ctx = GenerateRequestContext(nil, "", ContextSourcePeriodic, WorkflowNodeHealISCSI, LogLayerCSIFrontend)

		for {
			select {
			case tick := <-p.iSCSISelfHealingTicker.C:
				Logc(ctx).WithField("tick", tick).Debug("iSCSI self-healing is running.")
				// perform self healing here
				p.performISCSISelfHealing(ctx)
			case <-p.iSCSISelfHealingChannel:
				Logc(ctx).Info("iSCSI self-healing stopped.")
				return
			}
		}
	}()

	return
}

// stopISCSISelfHealingThread stops the iSCSI self-healing thread.
func (p *Plugin) stopISCSISelfHealingThread(_ context.Context) {
	if p.iSCSISelfHealingTicker != nil {
		p.iSCSISelfHealingTicker.Stop()
	}

	if p.iSCSISelfHealingChannel != nil {
		close(p.iSCSISelfHealingChannel)
	}

	return
}

// startNVMeSelfHealingThread starts the NVMe self-healing thread to heal faulty sessions.
func (p *Plugin) startNVMeSelfHealingThread(ctx context.Context) {
	// provision to disable the NVMe self-healing feature
	if p.nvmeSelfHealingInterval <= 0 {
		Logc(ctx).Info("NVMe self-healing is disabled.")
		return
	}

	Logc(ctx).WithFields(LogFields{
		"NVMeSelfHealingInterval": p.nvmeSelfHealingInterval,
	}).Info("NVMe self-healing is enabled.")
	// We are halving the nvmeSelfHealingInterval as it will be used to add some jitter between the iSCSI and NVMe
	// self-healing threads initially. We will reset the ticker after the first NVMe self-healing run.
	p.nvmeSelfHealingTicker = time.NewTicker(p.nvmeSelfHealingInterval / 2)
	p.nvmeSelfHealingChannel = make(chan struct{})

	go func() {
		ctx = GenerateRequestContext(nil, "", ContextSourcePeriodic, WorkflowNodeHealNVMe, LogLayerCSIFrontend)
		resetTicker := true

		for {
			select {
			case tick := <-p.nvmeSelfHealingTicker.C:
				Logc(ctx).WithField("tick", tick).Debug("NVMe self-healing is running.")
				p.performNVMeSelfHealing(ctx)
				// Resetting the ticket to a proper NVMeSelfHealingInterval.
				if resetTicker {
					p.nvmeSelfHealingTicker.Reset(p.nvmeSelfHealingInterval)
					resetTicker = false
				}
			case <-p.nvmeSelfHealingChannel:
				Logc(ctx).Info("NVMe self-healing stopped.")
				return
			}
		}
	}()

	return
}

// stopNVMeSelfHealingThread stops the NVMe self-healing thread.
func (p *Plugin) stopNVMeSelfHealingThread(_ context.Context) {
	if p.nvmeSelfHealingTicker != nil {
		p.nvmeSelfHealingTicker.Stop()
	}

	if p.nvmeSelfHealingChannel != nil {
		close(p.nvmeSelfHealingChannel)
	}

	return
}

// InitializeNodeLimiter initializes the sharedMapLimiter of node with various csi workflow related limiters.
// This function sets up limiters for different volume operations such as staging, unstaging,
// publishing, and unpublishing NFS and SMB volumes. Each limiter is configured with a semaphoreN
// to control the maximum number of concurrent operations allowed for each type of volume operation.
func (p *Plugin) InitializeNodeLimiter(ctx context.Context) {
	var err error

	Logc(ctx).Debug("Initializing node limiters.")
	defer Logc(ctx).Debug("Node limiters initialized.")

	if p.limiterSharedMap[NodeStageNFSVolume], err = limiter.New(ctx,
		NodeStageNFSVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodeStageNFSVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodeStageNFSVolume, err)
	}

	if p.limiterSharedMap[NodeStageSMBVolume], err = limiter.New(ctx,
		NodeStageSMBVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodeStageSMBVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodeStageSMBVolume, err)
	}

	if p.limiterSharedMap[NodeUnstageNFSVolume], err = limiter.New(ctx,
		NodeUnstageNFSVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodeUnstageNFSVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodeUnstageNFSVolume, err)
	}

	if p.limiterSharedMap[NodeUnstageSMBVolume], err = limiter.New(ctx,
		NodeUnstageSMBVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodeUnstageSMBVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodeUnstageSMBVolume, err)
	}

	if p.limiterSharedMap[NodePublishNFSVolume], err = limiter.New(ctx,
		NodePublishNFSVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodePublishNFSVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodePublishNFSVolume, err)
	}

	if p.limiterSharedMap[NodePublishSMBVolume], err = limiter.New(ctx,
		NodePublishSMBVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodePublishSMBVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodePublishSMBVolume, err)
	}

	// Initializing iSCSI limiters:
	if p.limiterSharedMap[NodeStageISCSIVolume], err = limiter.New(ctx,
		NodeStageISCSIVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodeStageISCSIVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodeStageISCSIVolume, err)
	}

	if p.limiterSharedMap[NodeUnstageISCSIVolume], err = limiter.New(ctx,
		NodeUnstageISCSIVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodeUnstageISCSIVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodeUnstageISCSIVolume, err)
	}

	if p.limiterSharedMap[NodePublishISCSIVolume], err = limiter.New(ctx,
		NodePublishISCSIVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodePublishISCSIVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodePublishISCSIVolume, err)
	}

	// Initializing FCP limiters:
	if p.limiterSharedMap[NodeStageFCPVolume], err = limiter.New(ctx,
		NodeStageFCPVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodeStageFCPVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodeStageFCPVolume, err)
	}

	if p.limiterSharedMap[NodeUnstageFCPVolume], err = limiter.New(ctx,
		NodeUnstageFCPVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodeUnstageFCPVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodeUnstageFCPVolume, err)
	}

	if p.limiterSharedMap[NodePublishFCPVolume], err = limiter.New(ctx,
		NodePublishFCPVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodePublishFCPVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodePublishFCPVolume, err)
	}

	// Initializing NVMe limiters:
	if p.limiterSharedMap[NodeStageNVMeVolume], err = limiter.New(ctx,
		NodeStageNVMeVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodeStageNVMeVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodeStageNVMeVolume, err)
	}

	if p.limiterSharedMap[NodeUnstageNVMeVolume], err = limiter.New(ctx,
		NodeUnstageNVMeVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodeUnstageNVMeVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodeUnstageNVMeVolume, err)
	}

	if p.limiterSharedMap[NodePublishNVMeVolume], err = limiter.New(ctx,
		NodePublishNVMeVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodePublishNVMeVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodePublishNVMeVolume, err)
	}

	// NodeUnpublish is common for all protocols
	if p.limiterSharedMap[NodeUnpublishVolume], err = limiter.New(ctx,
		NodeUnpublishVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodeUnpublishVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodeUnpublishVolume, err)
	}

	if p.limiterSharedMap[NodeExpandVolume], err = limiter.New(ctx,
		NodeExpandVolume,
		limiter.TypeSemaphoreN,
		limiter.WithSemaphoreNSize(ctx, maxNodeExpandVolumeOperations),
	); err != nil {
		Logc(ctx).Fatalf("Failed to initialize limiter for %s: %v", NodeExpandVolume, err)
	}
}
