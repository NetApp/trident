// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/frontend/csi/tridentcontroller"
	mockNode "github.com/netapp/trident/mocks/mock_core/mock_node"
	mockNodeHelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	mockTridentController "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_tridentcontroller"
	"github.com/netapp/trident/mocks/mock_utils/mock_devices"
	"github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/mocks/mock_utils/mock_fcp"
	"github.com/netapp/trident/mocks/mock_utils/mock_filesystem"
	"github.com/netapp/trident/mocks/mock_utils/mock_iscsi"
	"github.com/netapp/trident/mocks/mock_utils/mock_mount"
	"github.com/netapp/trident/mocks/mock_utils/mock_osutils"
	mock_nvme "github.com/netapp/trident/mocks/mock_utils/nvme"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// This package's tests must not use t.Parallel(): Attach/Detach/Graft/Prune/self-healing all
// read and write package-level globals (publishedISCSISessions, currentISCSISessions,
// iSCSISelfHealingLock, nvmeSelfHealingLock, NVMeNamespacesFlushRetry). NewCore resets the
// session maps, but the locks and flush-retry map are shared across every Core instance in the
// process, so concurrently-running tests that exercise them would race/interfere with each other.

// fakeController implements node.Controller by embedding a mockgen tridentcontroller.Client mock
// (registration, desired publication state, cleanup status) and a mockgen ChapClient mock (CHAP
// lookups), since no mock exists for the combined interface.
type fakeController struct {
	*mockTridentController.MockClient
	*mockNode.MockChapClient
}

func (f *fakeController) CHAPInfo(
	ctx context.Context, volumeID, nodeName string,
) (*models.IscsiChapInfo, error) {
	return f.MockChapClient.GetChap(ctx, volumeID, nodeName)
}

var _ Controller = (*fakeController)(nil)

// testMocks bundles every mock the Core depends on so individual tests can set expectations on
// exactly the ones they need and ignore the rest.
type testMocks struct {
	ISCSI      *mock_iscsi.MockISCSI
	FCP        *mock_fcp.MockFCP
	NVMe       *mock_nvme.MockNVMeInterface
	Devices    *mock_devices.MockDevices
	Filesystem *mock_filesystem.MockFilesystem
	Mount      *mock_mount.MockMount
	OsUtils    *mock_osutils.MockUtils
	Command    *mock_exec.MockCommand
	NodeHelper *mockNodeHelpers.MockNodeHelper
	Controller *fakeController
}

// newTestMocks constructs a fresh, ungenerated-expectation set of mocks for every Core
// dependency. Callers set only the expectations relevant to the scenario under test; gomock will
// fail the test if an unexpected call is made on a strict mock.
func newTestMocks(t *testing.T) *testMocks {
	ctrl := gomock.NewController(t)
	return &testMocks{
		ISCSI:      mock_iscsi.NewMockISCSI(ctrl),
		FCP:        mock_fcp.NewMockFCP(ctrl),
		NVMe:       mock_nvme.NewMockNVMeInterface(ctrl),
		Devices:    mock_devices.NewMockDevices(ctrl),
		Filesystem: mock_filesystem.NewMockFilesystem(ctrl),
		Mount:      mock_mount.NewMockMount(ctrl),
		OsUtils:    mock_osutils.NewMockUtils(ctrl),
		Command:    mock_exec.NewMockCommand(ctrl),
		NodeHelper: mockNodeHelpers.NewMockNodeHelper(ctrl),
		Controller: &fakeController{
			MockClient:     mockTridentController.NewMockClient(ctrl),
			MockChapClient: mockNode.NewMockChapClient(ctrl),
		},
	}
}

// expectNodeInfoDiscovery sets up permissive expectations for the host discovery calls
// Core.buildNodeInfo makes while assembling the node identity to register. Tests that exercise
// Bootstrap/register (rather than mocking register out entirely) need this so the strict mocks
// don't fail on host discovery calls unrelated to the scenario under test.
func expectNodeInfoDiscovery(mocks *testMocks) {
	mocks.OsUtils.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&models.HostSystem{}, nil).AnyTimes()
	mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return(nil, nil).AnyTimes()
	mocks.OsUtils.EXPECT().NFSActiveOnHost(gomock.Any()).Return(false, nil).AnyTimes()
	mocks.ISCSI.EXPECT().ISCSIActiveOnHost(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	mocks.NVMe.EXPECT().NVMeActiveOnHost(gomock.Any()).Return(false, nil).AnyTimes()
}

// newTestCore builds a Core wired to a fresh set of mocks, applying any additional options after
// the mock-backed defaults so callers can override individual dependencies. The returned Core is
// bootstrapped (readyChan closed) so checkReady-gated methods report ready; use
// newUnbootstrappedTestCore to test bootstrap gating itself.
func newTestCore(t *testing.T, extraOpts ...Option) (*Core, *testMocks) {
	core, mocks := newUnbootstrappedTestCore(t, extraOpts...)
	core.bootstrapErr = nil
	core.readyOnce.Do(func() { close(core.readyChan) })
	return core, mocks
}

// newUnbootstrappedTestCore is like newTestCore but leaves the Core's readyChan open, so
// checkReady-gated methods report NotReadyError until the test closes it (directly, or via
// Bootstrap/Activate).
func newUnbootstrappedTestCore(t *testing.T, extraOpts ...Option) (*Core, *testMocks) {
	mocks := newTestMocks(t)

	opts := []Option{
		WithLegacyISCSI(mocks.ISCSI),
		WithFCP(mocks.FCP),
		WithNVMe(mocks.NVMe),
		WithDevices(mocks.Devices),
		WithFilesystem(mocks.Filesystem),
		WithMount(mocks.Mount),
		WithOsUtils(mocks.OsUtils),
		WithCommand(mocks.Command),
		WithLocalStore(mocks.NodeHelper),
		WithController(mocks.Controller),
		WithHostName("test-node"),
	}
	opts = append(opts, extraOpts...)

	core := NewCore(opts...)
	return core, mocks
}

// closeReady closes a test Core's readyChan directly, bypassing Bootstrap/Activate, for tests
// that only want checkReady to stop reporting NotReadyError without exercising node cleanup/
// self-healing startup.
func closeReady(core *Core) {
	core.readyOnce.Do(func() { close(core.readyChan) })
}

// samplePublishInfo returns a minimal, valid VolumePublishInfo for the given protocol, with
// fields commonly required by that protocol's Attach/Detach/Mount/Unmount/Expand code paths
// already populated. Tests should override/add fields specific to their scenario.
func samplePublishInfo(protocol models.StorageProtocol) *models.VolumePublishInfo {
	pi := &models.VolumePublishInfo{
		StorageProtocol: protocol,
		GlobalMount:     "/var/lib/trident/staging/test-volume",
		FilesystemType:  "ext4",
	}
	switch protocol {
	case ISCSI:
		pi.IscsiTargetIQN = "iqn.1992-08.com.netapp:sn.test:vs.test"
		pi.IscsiTargetPortal = "192.0.2.1:3260"
		pi.IscsiPortals = []string{"192.0.2.1:3260"}
		pi.IscsiLunNumber = 0
		pi.DevicePath = "/dev/sdz"
	case FCP:
		pi.FCTargetWWNN = "20:00:00:25:00:00:00:01"
		pi.FCPLunNumber = 0
		pi.DevicePath = "/dev/sdz"
	case NVMe:
		pi.NVMeSubsystemNQN = "nqn.1992-08.com.netapp:sn.test:subsys.test"
		pi.NVMeNamespaceUUID = "11111111-1111-1111-1111-111111111111"
		pi.DevicePath = "/dev/nvme0n1"
	case SMB:
		pi.SMBPath = `\\server\share`
	case NFS:
		pi.NfsServerIP = "192.0.2.1"
		pi.NfsPath = "/export/test"
	}
	return pi
}

// sampleTrackingInfo wraps samplePublishInfo in a VolumeTrackingInfo. GlobalMount (embedded via
// VolumePublishInfo) is the sole staging path field; see the VolumeTrackingInfo doc comment.
func sampleTrackingInfo(protocol models.StorageProtocol) *models.VolumeTrackingInfo {
	pi := samplePublishInfo(protocol)
	return &models.VolumeTrackingInfo{
		VolumePublishInfo: *pi,
		PublishedPaths:    map[string]struct{}{},
	}
}

func TestNewCore_Defaults(t *testing.T) {
	core := NewCore()

	require.NotNil(t, core)
	assert.NotNil(t, core.volumeLocks)
	assert.NotNil(t, core.readyChan)
	assert.NotNil(t, publishedISCSISessions, "NewCore must initialize the package-level published session maps")
	assert.NotNil(t, currentISCSISessions)
}

func TestNewCore_OptionsApply(t *testing.T) {
	mocks := newTestMocks(t)

	core := NewCore(
		WithLegacyISCSI(mocks.ISCSI),
		WithFCP(mocks.FCP),
		WithNVMe(mocks.NVMe),
		WithDevices(mocks.Devices),
		WithFilesystem(mocks.Filesystem),
		WithMount(mocks.Mount),
		WithOsUtils(mocks.OsUtils),
		WithCommand(mocks.Command),
		WithLocalStore(mocks.NodeHelper),
		WithController(mocks.Controller),
		WithHostName("my-node"),
		WithUnsafeDetach(true),
		WithAESKey([]byte("0123456789abcdef")),
		WithNVMeSelfHealingInterval(5*time.Second),
		WithISCSISelfHealingInterval(6*time.Second),
		WithISCSISelfHealingWaitTime(7*time.Second),
		WithEnableForceDetach(true),
	)

	assert.Equal(t, mocks.ISCSI, core.iscsi)
	assert.Equal(t, mocks.FCP, core.fcp)
	assert.Equal(t, mocks.NVMe, core.nvme)
	assert.Equal(t, mocks.Devices, core.dev)
	assert.Equal(t, mocks.Filesystem, core.fs)
	assert.Equal(t, mocks.Mount, core.mount)
	assert.Equal(t, mocks.OsUtils, core.osutils)
	assert.Equal(t, mocks.Command, core.cmd)
	assert.Equal(t, mocks.NodeHelper, core.localStore)
	assert.Equal(t, mocks.Controller, core.controller)
	assert.Equal(t, "my-node", core.hostName)
	assert.True(t, core.unsafeDetach)
	assert.Equal(t, []byte("0123456789abcdef"), core.aesKey)
	assert.Equal(t, 5*time.Second, core.nvmeSelfHealingInterval)
	assert.Equal(t, 6*time.Second, core.iSCSISelfHealingInterval)
	assert.Equal(t, 7*time.Second, core.iSCSISelfHealingWaitTime)
	assert.True(t, core.enableForceDetach)
}

func TestCore_SetController(t *testing.T) {
	core := NewCore()
	mocks := newTestMocks(t)

	core.SetController(mocks.Controller)

	assert.Equal(t, mocks.Controller, core.controller)
}

func TestCore_GetNameAndVersion(t *testing.T) {
	core := NewCore()

	assert.Equal(t, "TridentNodeOrchestrator", core.GetName())
	assert.NotEmpty(t, core.Version())
}

func TestCheckReady_NotReadyBeforeClose(t *testing.T) {
	core, _ := newUnbootstrappedTestCore(t)

	err := core.checkReady()
	require.Error(t, err)
	assert.True(t, errors.IsNotReadyError(err))
	assert.False(t, core.IsReady())
}

func TestCheckReady_NilAfterCleanClose(t *testing.T) {
	core, _ := newUnbootstrappedTestCore(t)
	closeReady(core)

	assert.NoError(t, core.checkReady())
	assert.True(t, core.IsReady())
}

func TestCheckReady_PropagatesBootstrapErr(t *testing.T) {
	core, _ := newUnbootstrappedTestCore(t)
	core.bootstrapErr = errors.New("bootstrap failed")
	closeReady(core)

	err := core.checkReady()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bootstrap failed")
	assert.False(t, core.IsReady())
}

// expectCleanBootstrap sets up the mock expectations for a Bootstrap/Activate call that finds no
// stale publications: no tracking files on the node and no published-volume records on the
// controller.
func expectCleanBootstrap(mocks *testMocks) {
	mocks.Controller.MockClient.EXPECT().
		GetDesiredPublications(gomock.Any(), gomock.Any()).
		Return(nil, nil)
	mocks.NodeHelper.EXPECT().
		ListVolumeTrackingInfo(gomock.Any()).
		Return(map[string]*models.VolumeTrackingInfo{}, nil)
}

// expectSuccessfulRegistration sets up the mock expectations for a Bootstrap call whose register
// step succeeds: host discovery plus a successful RegisterNode.
func expectSuccessfulRegistration(mocks *testMocks) {
	expectNodeInfoDiscovery(mocks)
	mocks.Controller.MockClient.EXPECT().
		RegisterNode(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&tridentcontroller.RegistrationInfo{}, nil)
}

func TestBootstrap_Success(t *testing.T) {
	core, mocks := newUnbootstrappedTestCore(t)
	expectCleanBootstrap(mocks)
	expectSuccessfulRegistration(mocks)

	err := core.Bootstrap(context.Background())
	require.NoError(t, err)

	// readyChan must be closed and checkReady must report ready.
	assert.NoError(t, core.checkReady())
	assert.True(t, core.IsReady())
}

// TestBootstrap_NilControllerReturnsErrorInsteadOfPanicking is a regression test: Bootstrap must
// not dereference a nil Controller (e.g. if SetController/WithController was never called by the
// entrypoint wiring the Core together), and instead return a clear error.
func TestBootstrap_NilControllerReturnsErrorInsteadOfPanicking(t *testing.T) {
	core, _ := newUnbootstrappedTestCore(t)
	core.controller = nil
	// Activate (node cleanup, self-healing, publication reconciliation) must be skipped entirely
	// alongside Register, since it also dereferences c.controller; no mock expectations should
	// be set up here.

	require.NotPanics(t, func() {
		err := core.Bootstrap(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "controller is not configured")
	})

	// readyChan must still be closed even though bootstrap failed.
	readyErr := core.checkReady()
	require.Error(t, readyErr)
	assert.False(t, core.IsReady())
}

func TestBootstrap_RegisterErrorStillUnblocksReady(t *testing.T) {
	core, mocks := newUnbootstrappedTestCore(t)
	expectCleanBootstrap(mocks)
	expectNodeInfoDiscovery(mocks)
	mocks.Controller.MockClient.EXPECT().
		RegisterNode(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("register failed")).
		AnyTimes() // register retries with backoff until ctx is done; a permanent failure means >1 call

	// register() retries forever (timeout=0) unless ctx is done, so give Bootstrap a short-lived
	// ctx here rather than actually waiting out the (10s-initial-interval) backoff schedule. Once
	// ctx expires mid-retry, register's backoff loop reports ctx.Err() rather than the
	// permanently-failing operation's own error.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := core.Bootstrap(ctx)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)

	// Even though bootstrap failed, readyChan must still be closed (cleanup/self-healing best-effort).
	readyErr := core.checkReady()
	require.Error(t, readyErr)
	assert.ErrorIs(t, readyErr, context.DeadlineExceeded)
}

func TestBootstrap_OnlyUnblocksReadyOnce(t *testing.T) {
	core, mocks := newUnbootstrappedTestCore(t)
	expectCleanBootstrap(mocks)
	expectSuccessfulRegistration(mocks)

	require.NoError(t, core.Bootstrap(context.Background()))

	// A second call to Activate must not panic (readyOnce guards the channel close), and
	// performNodeCleanup/self-healing/publication-reconciliation should be safely re-runnable.
	mocks.Controller.MockClient.EXPECT().
		GetDesiredPublications(gomock.Any(), gomock.Any()).
		Return(nil, nil)
	mocks.NodeHelper.EXPECT().
		ListVolumeTrackingInfo(gomock.Any()).
		Return(map[string]*models.VolumeTrackingInfo{}, nil)
	assert.NoError(t, core.Activate())
}

func TestActivate_NilVolumeLocksIsInitialized(t *testing.T) {
	core, mocks := newUnbootstrappedTestCore(t)
	core.volumeLocks = nil
	expectCleanBootstrap(mocks)

	require.NoError(t, core.Activate())
	assert.NotNil(t, core.volumeLocks)
}

func TestDeactivate_NoSelfHealingOrReconciliationConfigured(t *testing.T) {
	core, _ := newTestCore(t)
	// Self-healing tickers/reconciliation loop were never started (disabled by default), so
	// Deactivate must be a safe no-op rather than panicking on nil tickers/channels.
	assert.NoError(t, core.Deactivate())
}

func TestActivate_InitializesLimitersOnlyOnce(t *testing.T) {
	core, mocks := newUnbootstrappedTestCore(t)
	expectCleanBootstrap(mocks)

	require.NoError(t, core.Activate())
	require.NotNil(t, core.protocolLimiters)
	original := core.protocolLimiters

	// A second Activate (e.g. via TestBootstrap_OnlyUnblocksReadyOnce-style re-run) must not
	// clobber the already-initialized limiter map.
	mocks.Controller.MockClient.EXPECT().
		GetDesiredPublications(gomock.Any(), gomock.Any()).
		Return(nil, nil)
	mocks.NodeHelper.EXPECT().
		ListVolumeTrackingInfo(gomock.Any()).
		Return(map[string]*models.VolumeTrackingInfo{}, nil)
	require.NoError(t, core.Activate())
	assert.Equal(t, original, core.protocolLimiters)
}
