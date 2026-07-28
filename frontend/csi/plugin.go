// Copyright 2026 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/netapp/trident/core/node"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/errors"
	execCmd "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/fcp"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/mount"
	"github.com/netapp/trident/utils/nvme"
	"github.com/netapp/trident/utils/osutils"
)

const (
	CSIController = "controller"
	CSINode       = "node"
	CSIAllInOne   = "allInOne"
)

type PluginOption func(*Plugin)

// StorageProtocolClients bundles the on-host protocol utilities and drivers used both by the CSI
// node frontend (for host service discovery in nodeGetInfo) and by the Core node orchestrator
// (for attach/detach/mount/unmount/expand). The caller - main.go for the real binary - constructs
// these once and supplies the same bundle to NewNodePlugin/NewAllInOnePlugin and, later, to
// NewNodeOrchestrator, so the frontend and Core always agree on which clients are in use.
type StorageProtocolClients struct {
	ISCSI   iscsi.ISCSI
	Mount   mount.Mount
	FS      filesystem.Filesystem
	FCP     fcp.FCP
	NVMe    nvme.NVMeInterface
	Devices devices.Devices
	Command execCmd.Command
	OsUtils osutils.Utils
}

type Plugin struct {
	orchestrator     core.Orchestrator
	nodeOrchestrator node.Orchestrator

	name     string
	nodeName string
	version  string
	endpoint string
	role     string

	unsafeDetach      bool
	enableForceDetach bool

	controllerHelper controllerhelpers.ControllerHelper
	nodeHelper       nodehelpers.NodeHelper

	aesKey []byte

	grpc     NonBlockingGRPCServer
	grpcLock sync.RWMutex

	csCap  []*csi.ControllerServiceCapability
	nsCap  []*csi.NodeServiceCapability
	vCap   []*csi.VolumeCapability_AccessMode
	gcsCap []*csi.GroupControllerServiceCapability

	topologyInUse atomic.Bool

	opCache sync.Map

	csi.UnimplementedIdentityServer
	csi.UnimplementedNodeServer
	csi.UnimplementedControllerServer
	csi.UnimplementedGroupControllerServer
}

func NewControllerPlugin(
	nodeName, endpoint string, aesKey []byte,
	orchestrator core.Orchestrator, helper controllerhelpers.ControllerHelper,
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
		aesKey:            aesKey,
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
	nodeName, endpoint string, aesKey []byte,
	orchestrator core.Orchestrator, nodeCore node.Orchestrator,
	controllerHelper controllerhelpers.ControllerHelper, nodeHelper nodehelpers.NodeHelper,
	unsafeDetach, enableForceDetach bool,
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

	p := &Plugin{
		name:              Provisioner,
		role:              CSINode,
		nodeName:          nodeName,
		version:           tridentconfig.OrchestratorVersion.ShortString(),
		endpoint:          endpoint,
		orchestrator:      orchestrator,
		nodeOrchestrator:  nodeCore,
		controllerHelper:  controllerHelper,
		nodeHelper:        nodeHelper,
		enableForceDetach: enableForceDetach,
		unsafeDetach:      unsafeDetach,
		opCache:           sync.Map{},
		aesKey:            aesKey,
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
	nodeName, endpoint string, aesKey []byte,
	orchestrator core.Orchestrator, nodeCore node.Orchestrator,
	controllerHelper controllerhelpers.ControllerHelper,
	nodeHelper nodehelpers.NodeHelper,
	unsafeDetach bool,
) (*Plugin, error) {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginCreate, LogLayerCSIFrontend)

	Logc(ctx).Info("Initializing CSI all-in-one frontend.")

	p := &Plugin{
		orchestrator:     orchestrator,
		name:             Provisioner,
		nodeName:         nodeName,
		version:          tridentconfig.OrchestratorVersion.ShortString(),
		endpoint:         endpoint,
		role:             CSIAllInOne,
		nodeOrchestrator: nodeCore,
		unsafeDetach:     unsafeDetach,
		controllerHelper: controllerHelper,
		nodeHelper:       nodeHelper,
		opCache:          sync.Map{},
		aesKey:           aesKey,
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

		// Only add the node registration interceptor for roles that serve Node RPCs.
		// CSIController does not need it, so it is excluded from the interceptor chain entirely.
		var extraInterceptors []grpc.UnaryServerInterceptor
		if p.role == CSINode || p.role == CSIAllInOne {
			extraInterceptors = append(extraInterceptors, nodeRegistrationInterceptor)
		}
		p.setGRPC(NewNonBlockingGRPCServer(extraInterceptors...))

		fields := LogFields{"nodeName": p.nodeName, "role": p.role}
		Logc(ctx).WithFields(fields).Info("Activating CSI frontend.")

		// Start the gRPC server immediately so node-driver-registrar can connect to /plugin/csi.sock
		// within its ~30s connection deadline, even if controller registration (nodeRegisterWithController)
		// is slow or retrying. TRID-19339: Decouples socket availability from controller readiness.
		p.getGRPC().Start(p.endpoint, p, p, p, p)

		if p.role == CSINode || p.role == CSIAllInOne {
			// nodeOrchestrator.Bootstrap() - controller registration, node cleanup, self-healing,
			// publication reconciliation startup, and per-protocol admission control setup - is
			// owned by whoever constructed the Core (main.go for the real binary) so the same
			// Core can be shared and bootstrapped once across multiple frontends. Data-path RPCs
			// block on it via awaitBootstrap.
		}

		if p.role == CSIController || p.role == CSIAllInOne {
			// Check if topology is in use and store it in the plugin
			topologyInUse := p.controllerHelper.IsTopologyInUse(ctx)
			p.topologyInUse.Store(topologyInUse)
		}
	}()
	return nil
}

func (p *Plugin) Deactivate() error {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginDeactivate, LogLayerCSIFrontend)

	// Node core's own lifecycle (starting/stopping self-healing, publication reconciliation, etc.)
	// is driven directly by whoever constructed the Core (main.go), the same as Bootstrap/Activate
	// above. The frontend does not own or reach into that lifecycle.
	Logc(ctx).Info("Deactivating CSI frontend.")
	// Safely stop gRPC server only if it was initialized by Activate().
	if grpcServer := p.getGRPC(); grpcServer != nil {
		grpcServer.GracefulStop()
	}

	return nil
}

func (p *Plugin) setGRPC(server NonBlockingGRPCServer) {
	p.grpcLock.Lock()
	p.grpc = server
	p.grpcLock.Unlock()
}

func (p *Plugin) getGRPC() NonBlockingGRPCServer {
	p.grpcLock.RLock()
	defer p.grpcLock.RUnlock()
	return p.grpc
}

func (p *Plugin) GetName() string {
	return string(tridentconfig.ContextCSI)
}

// GetNodeHelper returns the NodeHelper interface
func (p *Plugin) GetNodeHelper() nodehelpers.NodeHelper {
	return p.nodeHelper
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
	// The core/node layer is transport-agnostic and never constructs gRPC statuses itself.
	// Some paths (e.g. iSCSI/NVMe self-healing lock timeouts) instead return a sentinel
	// errors.MaxWaitExceededError, which callers may have wrapped or joined along the way.
	// Map that to codes.Aborted here since kubelet/CSI sidecars treat it differently
	// (e.g. immediate retry) than an opaque Unknown.
	if s, ok := status.FromError(err); ok && s.Code() != codes.Unknown {
		return err
	}
	if errors.IsMaxWaitExceededError(err) {
		return status.Error(codes.Aborted, err.Error())
	} else if errors.IsNotReadyError(err) {
		return status.Error(codes.Unavailable, err.Error())
	} else if errors.IsBootstrapError(err) {
		return status.Error(codes.FailedPrecondition, err.Error())
	} else if errors.IsPreconditionError(err) {
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
	} else if errors.IsInternalError(err) {
		return status.Error(codes.Internal, err.Error())
	} else if errors.IsPermissionDeniedError(err) {
		return status.Error(codes.PermissionDenied, err.Error())
	} else if errors.IsInvalidJSONError(err) {
		return status.Error(codes.Internal, err.Error())
	} else if errors.IsVolumeStateError(err) {
		return status.Error(codes.Aborted, err.Error())
	} else {
		return status.Error(codes.Unknown, err.Error())
	}
}

// IsReady reports whether this role is ready to serve data-path RPCs. CSIController has no node
// registration gate, so it is always ready. CSINode/CSIAllInOne defer entirely to the node core's
// own readiness, which reflects Bootstrap (registration, node cleanup, self-healing startup)
// having run to completion (successfully or not - see node.Core.IsReady).
func (p *Plugin) IsReady() bool {
	if p.nodeOrchestrator == nil {
		return true
	}
	return p.nodeOrchestrator.IsReady()
}
