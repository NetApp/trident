// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"sync"
	"time"

	tridentconfig "github.com/netapp/trident/config"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	"github.com/netapp/trident/frontend/csi/tridentcontroller"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/locks"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/fcp"
	"github.com/netapp/trident/utils/filesystem"
	legacyiscsi "github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/limiter"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount"
	"github.com/netapp/trident/utils/nvme"
	"github.com/netapp/trident/utils/osutils"
)

const (
	NFS   = models.NFS
	SMB   = models.SMB
	FCP   = models.FCP
	NVMe  = models.NVMe
	ISCSI = models.ISCSI
)

var sharedLocksNodeLockTimeout = 60 * time.Second

type Controller interface {
	tridentcontroller.Client
	CHAPInfo(ctx context.Context, volumeID, nodeName string) (*models.IscsiChapInfo, error)
}

// Orchestrator is the frontend-facing node platform contract. It assumes one Core per Trident
// node process (one node pod = one host); CSI, CRD, and any future node operator should share a
// single instance rather than constructing their own.
type Orchestrator interface {
	Bootstrap(ctx context.Context) error
	Deactivate() error
	IsReady() bool
	Attach(ctx context.Context, volumeID string, req AttachRequest) error
	Detach(ctx context.Context, volumeID string, req DetachRequest) error
	Expand(ctx context.Context, volumeID string, req ExpandRequest) error
	Mount(ctx context.Context, volumeID string, req MountRequest) error
	Unmount(ctx context.Context, volumeID string, req UnmountRequest) error
	Graft(ctx context.Context, volumeID string, req GraftRequest) (*models.GraftAttachmentResponse, error)
	Prune(ctx context.Context, volumeID string, req PruneRequest) (*models.PruneAttachmentResponse, error)
}

type Core struct {
	// Core internals
	// protocolLimiters holds per-CSI-workflow, per-protocol (where applicable) semaphores used
	// for admission control on Attach/Detach/Mount/Unmount/Expand. Populated once by
	// initializeLimiters; the frontend has no equivalent of its own since these operations - and
	// the tracking-file reads needed to pick the right protocol bucket - live entirely here.
	protocolLimiters map[string]limiter.Limiter
	volumeLocks      *locks.GCNamedMutex
	localStore       nodehelpers.NodeHelper
	controller       Controller
	readyChan        chan struct{}
	readyOnce        sync.Once
	bootstrapErr     error
	hostName         string
	unsafeDetach     bool

	// hostInfo caches host system discovery (OS/platform info) gathered once, the first time
	// registration builds the node identity to send to the controller.
	hostInfo *models.HostSystem

	// logSettings, if set, receives the controller's log level/workflows/layers after a
	// successful registration so the node can align its local logging with the controller's.
	// Optional: nil means registration does not attempt to propagate log settings.
	logSettings LogSetter

	// aesKey decrypts CHAP credentials that arrive encrypted (e.g. via GraftAttachment) using the
	// same AES key the CSI frontend was configured with.
	aesKey []byte

	// nvmeSelfHealingInterval gates whether disconnectNVMeSubsystemIfNeeded honors the
	// self-healing "disconnect" hint. Zero/negative means self-healing is disabled.
	nvmeSelfHealingInterval time.Duration

	// iSCSISelfHealingInterval/WaitTime gate the periodic iSCSI self-healing sweep.
	// Zero/negative interval means self-healing is disabled.
	iSCSISelfHealingInterval time.Duration
	iSCSISelfHealingWaitTime time.Duration
	iSCSISelfHealingTicker   *time.Ticker
	iSCSISelfHealingChannel  chan struct{}

	nvmeSelfHealingTicker  *time.Ticker
	nvmeSelfHealingChannel chan struct{}

	// enableForceDetach gates the node publication reconciliation loop, which may force-detach
	// volumes with no matching publication record on the Trident controller.
	enableForceDetach       bool
	nodePublicationTimer    *time.Timer
	stopNodePublicationLoop chan bool

	// Host protocol utils
	fcp     fcp.FCP
	cmd     exec.Command
	dev     devices.Devices
	nvme    nvme.NVMeInterface
	iscsi   legacyiscsi.ISCSI
	fs      filesystem.Filesystem
	mount   mount.Mount
	osutils osutils.Utils
}

type Option func(*Core)

func WithFCP(f fcp.FCP) Option {
	return func(core *Core) {
		core.fcp = f
	}
}

func WithNVMe(n nvme.NVMeInterface) Option {
	return func(core *Core) {
		core.nvme = n
	}
}

func WithLegacyISCSI(i legacyiscsi.ISCSI) Option {
	return func(core *Core) {
		core.iscsi = i
	}
}

func WithCommand(cmd exec.Command) Option {
	return func(core *Core) {
		core.cmd = cmd
	}
}

func WithDevices(d devices.Devices) Option {
	return func(core *Core) {
		core.dev = d
	}
}

func WithFilesystem(f filesystem.Filesystem) Option {
	return func(core *Core) {
		core.fs = f
	}
}

func WithMount(m mount.Mount) Option {
	return func(core *Core) {
		core.mount = m
	}
}

func WithOsUtils(o osutils.Utils) Option {
	return func(core *Core) {
		core.osutils = o
	}
}

func WithLocalStore(store nodehelpers.NodeHelper) Option {
	return func(core *Core) {
		core.localStore = store
	}
}

func WithHostName(hostName string) Option {
	return func(core *Core) {
		core.hostName = hostName
	}
}

func WithUnsafeDetach(unsafe bool) Option {
	return func(core *Core) {
		core.unsafeDetach = unsafe
	}
}

func WithAESKey(aesKey []byte) Option {
	return func(core *Core) {
		core.aesKey = aesKey
	}
}

func WithNVMeSelfHealingInterval(interval time.Duration) Option {
	return func(core *Core) {
		core.nvmeSelfHealingInterval = interval
	}
}

func WithISCSISelfHealingInterval(interval time.Duration) Option {
	return func(core *Core) {
		core.iSCSISelfHealingInterval = interval
	}
}

func WithISCSISelfHealingWaitTime(wait time.Duration) Option {
	return func(core *Core) {
		core.iSCSISelfHealingWaitTime = wait
	}
}

func WithController(controller Controller) Option {
	return func(core *Core) {
		core.controller = controller
	}
}

// LogSetter aligns the node's local logging with the controller's after registration.
// core.Orchestrator (the volume orchestrator) already implements this interface, so main.go can
// pass it directly via WithLogSettingsApplier without any adapter.
type LogSetter interface {
	SetLogLevel(ctx context.Context, level string) error
	SetLoggingWorkflows(ctx context.Context, flows string) error
	SetLogLayers(ctx context.Context, layers string) error
}

func WithLogSetter(applier LogSetter) Option {
	return func(core *Core) {
		core.logSettings = applier
	}
}

func WithEnableForceDetach(enable bool) Option {
	return func(core *Core) {
		core.enableForceDetach = enable
	}
}

// SetController finishes wiring the Core's Controller after construction. This exists because
// the Controller adapter typically wraps a specific frontend's REST client and node registration
// logic (see the CSI frontend's nodeControllerAdapter), and that frontend may not exist yet when
// the rest of the Core - and its protocol clients - are built by main.go via functional options.
func (c *Core) SetController(controller Controller) {
	c.controller = controller
}

func NewCore(opts ...Option) *Core {
	core := &Core{
		volumeLocks: locks.NewGCNamedMutex(),
		readyChan:   make(chan struct{}),
		readyOnce:   sync.Once{},
	}
	for _, opt := range opts {
		opt(core)
	}

	// Initialize self-healing maps.
	publishedISCSISessions = models.NewISCSISessions()
	currentISCSISessions = models.NewISCSISessions()

	return core
}

func (c *Core) checkReady() error {
	select {
	case <-c.readyChan:
		return c.bootstrapErr // safe: written before channel was closed
	default:
		return errors.NotReadyError()
	}
}

// IsReady is a non-blocking readiness check used by both the CSI frontend's node-registration
// gRPC interceptor (fast-path rejection of data-path RPCs before parsing) and the node readiness
// HTTP probe kubelet's readinessProbe queries directly (frontend/rest.NodeReadinessCheck) - the
// latter isn't performing a node operation of its own that could return checkReady's error.
func (c *Core) IsReady() bool {
	return c.checkReady() == nil
}

func (c *Core) Bootstrap(ctx context.Context) error {
	var bootstrapErr error

	// Register and Activate (node cleanup, self-healing, publication reconciliation) both
	// dereference c.controller; skip both rather than panicking if it was never wired up (e.g.
	// SetController/WithController omitted by whatever assembled this Core). Register runs first,
	// then Activate, mirroring the old CSI frontend's Activate() ordering (register with the
	// controller, then initialize limiters/cleanup/self-healing/publication reconciliation).
	if c.controller == nil {
		bootstrapErr = errors.Join(bootstrapErr, errors.New("controller is not configured"))
	} else {
		if registerErr := c.register(ctx, 0); registerErr != nil {
			bootstrapErr = errors.Join(bootstrapErr, registerErr)
		}
		if activateErr := c.Activate(); activateErr != nil {
			bootstrapErr = errors.Join(bootstrapErr, activateErr)
		}
	}

	c.bootstrapErr = bootstrapErr
	c.readyOnce.Do(func() {
		close(c.readyChan)
	})

	return bootstrapErr
}

func (c *Core) Activate() error {
	if c.volumeLocks == nil {
		c.volumeLocks = locks.NewGCNamedMutex()
	}

	ctx := context.Background()

	if c.protocolLimiters == nil {
		if err := c.initializeLimiters(ctx); err != nil {
			return err
		}
	}

	if cleanupErr := c.performNodeCleanup(ctx); cleanupErr != nil {
		Logc(ctx).WithError(cleanupErr).Warn("Failed to clean node; self-healing features may be unreliable.")
	}
	c.populatePublishedSessions(ctx)
	c.startSelfHealing(ctx)
	c.startPublicationReconciliation(ctx)

	return nil
}

func (c *Core) Deactivate() error {
	ctx := context.Background()
	c.stopPublicationReconciliation(ctx)
	c.stopSelfHealing(ctx)

	return nil
}

func (c *Core) GetName() string {
	return "TridentNodeOrchestrator"
}

func (c *Core) Version() string {
	return tridentconfig.NodeOrchestratorVersion.String()
}
