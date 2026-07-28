// Copyright 2026 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/core/node"
	"github.com/netapp/trident/frontend/csi/tridentcontroller"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/mocks/mock_core"
	mock_node "github.com/netapp/trident/mocks/mock_core/mock_node"
	mock_controller_helpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_helpers"
	mock_node_helpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	mock_tridentcontroller "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_tridentcontroller"
	mock_iscsi "github.com/netapp/trident/mocks/mock_utils/mock_iscsi"
	mock_osutils "github.com/netapp/trident/mocks/mock_utils/mock_osutils"
	mock_nvme "github.com/netapp/trident/mocks/mock_utils/nvme"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// newTestNodeCore builds a *node.Core wired with permissive mocks for every dependency Bootstrap
// touches (controller transport, CHAP lookup, local tracking store, host protocol discovery).
// registerNode customizes RegisterNode's behavior (e.g. to simulate slow registration); it is not
// bootstrapped, leaving that to the caller.
func newTestNodeCore(
	t testing.TB,
	registerNode func(ctx context.Context, node *models.Node, timeout time.Duration) (*tridentcontroller.RegistrationInfo, error),
) *node.Core {
	t.Helper()
	ctrl := gomock.NewController(t)

	mockClient := mock_tridentcontroller.NewMockClient(ctrl)
	mockClient.EXPECT().RegisterNode(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(registerNode).AnyTimes()
	mockClient.EXPECT().GetDesiredPublications(gomock.Any(), gomock.Any()).
		Return(map[string]*models.VolumePublicationExternal{}, nil).AnyTimes()
	mockClient.EXPECT().GetNodeCleanupStatus(gomock.Any(), gomock.Any()).
		Return(models.NodeClean, nil).AnyTimes()
	mockClient.EXPECT().MarkNodeCleanupComplete(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	mockChap := mock_node.NewMockChapClient(ctrl)
	mockChap.EXPECT().GetChap(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&models.IscsiChapInfo{}, nil).AnyTimes()

	mockLocalStore := mock_node_helpers.NewMockNodeHelper(ctrl)
	mockLocalStore.EXPECT().ListVolumeTrackingInfo(gomock.Any()).Return(nil, nil).AnyTimes()
	mockLocalStore.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	mockOsUtils := mock_osutils.NewMockUtils(ctrl)
	mockOsUtils.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&models.HostSystem{}, nil).AnyTimes()
	mockOsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return([]string{"127.0.0.1"}, nil).AnyTimes()
	mockOsUtils.EXPECT().NFSActiveOnHost(gomock.Any()).Return(false, nil).AnyTimes()

	mockISCSI := mock_iscsi.NewMockISCSI(ctrl)
	mockISCSI.EXPECT().ISCSIActiveOnHost(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()

	mockNVMe := mock_nvme.NewMockNVMeInterface(ctrl)
	mockNVMe.EXPECT().NVMeActiveOnHost(gomock.Any()).Return(false, nil).AnyTimes()

	return node.NewCore(
		node.WithController(node.NewController(mockClient, mockChap)),
		node.WithLocalStore(mockLocalStore),
		node.WithHostName("test-node"),
		node.WithOsUtils(mockOsUtils),
		node.WithLegacyISCSI(mockISCSI),
		node.WithNVMe(mockNVMe),
	)
}

// newReadyNodeCore builds a *node.Core wired with permissive mocks and drives it through a
// successful Bootstrap. The node core - not the Plugin - owns registration and readiness now, so
// tests that need Plugin.IsReady() to report true must go through a real (mocked) Bootstrap
// rather than poking at removed Plugin-level fields like the old nodeReadyCh.
func newReadyNodeCore(t testing.TB) *node.Core {
	t.Helper()
	core := newTestNodeCore(t, func(context.Context, *models.Node, time.Duration) (*tridentcontroller.RegistrationInfo, error) {
		return &tridentcontroller.RegistrationInfo{}, nil
	})
	require.NoError(t, core.Bootstrap(context.Background()))
	return core
}

// newNotReadyNodeCore returns a *node.Core that has never been bootstrapped, for tests that need
// Plugin.IsReady() to report false because the node core hasn't finished registering.
func newNotReadyNodeCore() *node.Core {
	return node.NewCore()
}

func TestNewControllerPlugin(t *testing.T) {
	testCases := []struct {
		name          string
		createFile    bool
		fileContent   string
		expectedError bool
	}{
		{name: "Success", createFile: true, fileContent: "test-key", expectedError: false},
		{name: "Error - File not found", createFile: false, expectedError: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset the singleton state between subtests.
			readAESKeysOnce = sync.Once{}
			aesKeySingleton = nil
			aesKeyError = nil

			ctrl := gomock.NewController(t)
			mockOrchestrator := mock_core.NewMockOrchestrator(ctrl)
			mockHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)

			var aesKeyFile string
			if tc.createFile {
				tmpFile, err := os.CreateTemp("", "test-key-*")
				require.NoError(t, err)
				defer os.Remove(tmpFile.Name())
				_, err = tmpFile.WriteString(tc.fileContent)
				require.NoError(t, err)
				tmpFile.Close()
				aesKeyFile = tmpFile.Name()
			} else {
				aesKeyFile = "/nonexistent/file"
			}
			aesKey, aesErr := ReadAESKey(context.Background(), aesKeyFile)

			if tc.expectedError {
				require.Error(t, aesErr)
				return
			}
			require.NoError(t, aesErr)

			plugin, err := NewControllerPlugin("node", "endpoint", aesKey, mockOrchestrator, mockHelper, true)

			require.NoError(t, err)
			require.NotNil(t, plugin)
			assert.Equal(t, CSIController, plugin.role)
			assert.Equal(t, "node", plugin.nodeName)
			assert.NotEmpty(t, plugin.csCap)
			assert.NotEmpty(t, plugin.vCap)
			assert.NotEmpty(t, plugin.gcsCap)
		})
	}
}

func TestNewNodePlugin(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockOrchestrator := mock_core.NewMockOrchestrator(ctrl)
	mockControllerHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
	mockNodeHelper := mock_node_helpers.NewMockNodeHelper(ctrl)

	// NewNodePlugin no longer wires the node core's Controller itself - that is main.go's job
	// (see controllerRestURL/tridentcontroller.WireClient) - so it stores whatever Core it is
	// given as-is, with no NewControllerClient call expected here.
	nodeCore := node.NewCore(node.WithHostName("test-node"))

	plugin, err := NewNodePlugin(
		"test-node", "unix:///tmp/csi.sock", []byte("aes-key"),
		mockOrchestrator, nodeCore, mockControllerHelper, mockNodeHelper,
		true, false,
	)

	require.NoError(t, err)
	require.NotNil(t, plugin)
	assert.Equal(t, "test-node", plugin.nodeName)
	assert.Equal(t, "unix:///tmp/csi.sock", plugin.endpoint)
	assert.Equal(t, CSINode, plugin.role)
	assert.False(t, plugin.enableForceDetach)
	assert.True(t, plugin.unsafeDetach)
	assert.NotNil(t, plugin.nsCap)
	assert.Len(t, plugin.vCap, 7)
	assert.Same(t, nodeCore, plugin.nodeOrchestrator)
}

func TestNewAllInOnePlugin(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockOrchestrator := mock_core.NewMockOrchestrator(ctrl)
	mockControllerHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
	mockNodeHelper := mock_node_helpers.NewMockNodeHelper(ctrl)

	nodeCore := node.NewCore(node.WithHostName("test-node"))

	plugin, err := NewAllInOnePlugin(
		"test-node", "unix:///tmp/csi.sock", []byte("aes-key"),
		mockOrchestrator, nodeCore, mockControllerHelper, mockNodeHelper,
		false,
	)

	require.NoError(t, err)
	require.NotNil(t, plugin)
	assert.Equal(t, "test-node", plugin.nodeName)
	assert.Equal(t, CSIAllInOne, plugin.role)
	assert.NotEmpty(t, plugin.csCap)
	assert.NotEmpty(t, plugin.nsCap)
	assert.Len(t, plugin.vCap, 7)
	assert.NotEmpty(t, plugin.gcsCap)
	assert.Same(t, nodeCore, plugin.nodeOrchestrator)
}

func TestPlugin_Activate_ControllerRole_SetsTopologyInUse(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockControllerHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
	mockControllerHelper.EXPECT().IsTopologyInUse(gomock.Any()).Return(true)

	plugin := &Plugin{
		role:             CSIController,
		nodeName:         "test-node",
		endpoint:         "unix://" + tempSocketPath(t),
		controllerHelper: mockControllerHelper,
	}
	t.Cleanup(func() { _ = plugin.Deactivate() })

	require.NoError(t, plugin.Activate())

	assert.Eventually(t, func() bool { return plugin.getGRPC() != nil }, 2*time.Second, 10*time.Millisecond)
	assert.Eventually(t, plugin.isTopologyInUse, 2*time.Second, 10*time.Millisecond)
}

func TestPlugin_Activate_NodeRole_StartsGRPCWithoutBlockingOnNodeCore(t *testing.T) {
	// Activate() no longer waits on node core Bootstrap at all - that is driven independently
	// by whoever constructed the Core (main.go). A never-bootstrapped node core must not stop
	// the gRPC socket from becoming available.
	plugin := &Plugin{
		role:             CSINode,
		nodeName:         "test-node",
		endpoint:         "unix://" + tempSocketPath(t),
		nodeOrchestrator: newNotReadyNodeCore(),
	}
	t.Cleanup(func() { _ = plugin.Deactivate() })

	require.NoError(t, plugin.Activate())

	assert.Eventually(t, func() bool { return plugin.getGRPC() != nil }, 2*time.Second, 10*time.Millisecond)
	assert.False(t, plugin.IsReady(), "plugin must stay gated while the node core is not ready")
}

// TestPlugin_Activate_ReproducesCustomerIssue_TRID19339 reproduces the customer scenario behind
// TRID-19339 under the current architecture, where node core Bootstrap is driven independently of
// Plugin.Activate() (by main.go, simulated here): node-driver-registrar could not connect to the
// CSI socket within its ~30s deadline because the socket did not exist until controller
// registration finished. This proves the gRPC socket is available - and Identity.Probe succeeds -
// while registration is still in flight, that data-path RPCs are rejected Unavailable until node
// core finishes bootstrapping, and that they are allowed once it does.
func TestPlugin_Activate_ReproducesCustomerIssue_TRID19339(t *testing.T) {
	InitAuditLogger(true)

	ctrl := gomock.NewController(t)
	mockOrchestrator := mock_core.NewMockOrchestrator(ctrl)
	mockOrchestrator.EXPECT().GetVersion(gomock.Any()).Return("test", nil).AnyTimes()

	registrationDone := make(chan struct{})
	nodeCore := newTestNodeCore(t, func(context.Context, *models.Node, time.Duration) (*tridentcontroller.RegistrationInfo, error) {
		time.Sleep(500 * time.Millisecond) // Simulates slow controller registration on a busy cluster.
		defer close(registrationDone)
		return &tridentcontroller.RegistrationInfo{}, nil
	})

	socketPath := tempSocketPath(t)
	plugin := &Plugin{
		role:             CSINode,
		nodeName:         "customer-node",
		endpoint:         "unix://" + socketPath,
		orchestrator:     mockOrchestrator,
		nodeOrchestrator: nodeCore,
	}
	t.Cleanup(func() { _ = plugin.Deactivate() })

	require.NoError(t, plugin.Activate())

	// Bootstrap is driven independently of Activate(), the same as main.go does.
	go func() { _ = nodeCore.Bootstrap(context.Background()) }()

	// PHASE 1: the gRPC socket must be available well within the registrar's ~30s deadline, even
	// though registration is still in flight.
	assert.Eventually(t, func() bool {
		_, statErr := os.Stat(socketPath)
		return statErr == nil
	}, 2*time.Second, 10*time.Millisecond, "expected gRPC socket to be created before slow registration finishes")

	// PHASE 2: a real gRPC client can connect and call Identity.Probe, the registrar's first call.
	conn, dialErr := grpc.NewClient("unix://"+socketPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, dialErr)
	defer conn.Close()

	probeCtx, probeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer probeCancel()
	probeResp, probeErr := csi.NewIdentityClient(conn).Probe(probeCtx, &csi.ProbeRequest{})
	require.NoError(t, probeErr, "Identity.Probe must succeed while registration is in progress")
	assert.NotNil(t, probeResp)

	// PHASE 3: data-path RPCs are rejected Unavailable before registration completes.
	nodeClient := csi.NewNodeClient(conn)
	stageCtx, stageCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stageCancel()
	_, stageErr := nodeClient.NodeStageVolume(stageCtx, &csi.NodeStageVolumeRequest{
		VolumeId:          "vol-test",
		StagingTargetPath: "/tmp/staging",
		VolumeCapability:  &csi.VolumeCapability{},
	})
	require.Error(t, stageErr)
	assert.Equal(t, codes.Unavailable, status.Code(stageErr))

	// PHASE 4: after registration completes, the node core - and therefore data-path RPCs -
	// becomes ready.
	select {
	case <-registrationDone:
	case <-time.After(5 * time.Second):
		t.Fatal("registration did not complete within expected time")
	}
	assert.Eventually(t, plugin.IsReady, 2*time.Second, 10*time.Millisecond)
}

func tempSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "csi")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir + "/csi.sock"
}

// TestPlugin_Deactivate_SafeWithoutActivate validates that Deactivate() can be called
// safely even if Activate() was never called or hasn't completed yet (p.grpc is nil).
// This prevents nil pointer panics in shutdown scenarios. TRID-19339 safe shutdown.
func TestPlugin_Deactivate_SafeWithoutActivate(t *testing.T) {
	plugin := &Plugin{
		role:     CSINode,
		nodeName: "test-node",
		endpoint: "unix:///tmp/test.sock",
		grpc:     nil, // Simulate p.grpc not yet initialized
	}

	err := plugin.Deactivate()
	assert.NoError(t, err, "Deactivate() should not panic or error when called before Activate() initializes gRPC")
}

func TestPlugin_Deactivate(t *testing.T) {
	testCases := []struct {
		name                 string
		role                 string
		withNodeOrchestrator bool
	}{
		{name: "CSINode with node core", role: CSINode, withNodeOrchestrator: true},
		{name: "CSIAllInOne with node core", role: CSIAllInOne, withNodeOrchestrator: true},
		{name: "CSIController has no node core", role: CSIController, withNodeOrchestrator: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{role: tc.role, grpc: &mockNonBlockingGRPCServer{}}
			if tc.withNodeOrchestrator {
				plugin.nodeOrchestrator = node.NewCore()
			}

			assert.NoError(t, plugin.Deactivate())
		})
	}
}

// Mock GRPC server for testing
type mockNonBlockingGRPCServer struct{}

func (m *mockNonBlockingGRPCServer) Start(
	endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer, gs csi.GroupControllerServer,
) {
}

func (m *mockNonBlockingGRPCServer) GracefulStop() {}

func (m *mockNonBlockingGRPCServer) Stop() {}

func TestPlugin_GetName(t *testing.T) {
	plugin := &Plugin{}
	assert.Equal(t, string(tridentconfig.ContextCSI), plugin.GetName())
}

func TestPlugin_Version(t *testing.T) {
	plugin := &Plugin{}
	assert.Equal(t, tridentconfig.OrchestratorVersion.String(), plugin.Version())
}

func TestPlugin_AddControllerServiceCapabilities(t *testing.T) {
	plugin := &Plugin{}
	plugin.addControllerServiceCapabilities(context.Background(), []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
	})
	assert.Len(t, plugin.csCap, 2)
}

func TestPlugin_AddNodeServiceCapabilities(t *testing.T) {
	plugin := &Plugin{}
	plugin.addNodeServiceCapabilities([]csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
	})
	assert.Len(t, plugin.nsCap, 1)
}

func TestPlugin_AddVolumeCapabilityAccessModes(t *testing.T) {
	plugin := &Plugin{}
	plugin.addVolumeCapabilityAccessModes(context.Background(), []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
	})
	assert.Len(t, plugin.vCap, 2)
}

func TestPlugin_AddGroupControllerServiceCapabilities(t *testing.T) {
	plugin := &Plugin{}
	plugin.addGroupControllerServiceCapabilities(context.Background(), []csi.GroupControllerServiceCapability_RPC_Type{
		csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT,
	})
	assert.Len(t, plugin.gcsCap, 1)
}

func TestPlugin_GetCSIErrorForOrchestratorError(t *testing.T) {
	testCases := []struct {
		name         string
		inputError   error
		expectedCode codes.Code
	}{
		{name: "MaxWaitExceededError", inputError: errors.MaxWaitExceededError("waited too long"), expectedCode: codes.Aborted},
		{name: "NotReadyError", inputError: errors.NotReadyError(), expectedCode: codes.Unavailable},
		{name: "BootstrapError", inputError: errors.BootstrapError(errors.New("")), expectedCode: codes.FailedPrecondition},
		{name: "PreconditionError", inputError: errors.PreconditionError(""), expectedCode: codes.FailedPrecondition},
		{name: "NotFoundError", inputError: errors.NotFoundError(""), expectedCode: codes.NotFound},
		{
			name: "UnsupportedCapacityRangeError", inputError: errors.UnsupportedCapacityRangeError(errors.New("")),
			expectedCode: codes.OutOfRange,
		},
		{name: "FoundError", inputError: errors.FoundError("already exists"), expectedCode: codes.AlreadyExists},
		{
			name:         "NodeNotSafeToPublishForBackendError",
			inputError:   errors.NodeNotSafeToPublishForBackendError("node not safe", ""),
			expectedCode: codes.FailedPrecondition,
		},
		{name: "VolumeCreatingError", inputError: errors.VolumeCreatingError("volume creating"), expectedCode: codes.DeadlineExceeded},
		{name: "VolumeDeletingError", inputError: errors.VolumeDeletingError("volume deleting"), expectedCode: codes.DeadlineExceeded},
		{
			name: "ResourceExhaustedError", inputError: errors.ResourceExhaustedError(errors.New("")),
			expectedCode: codes.ResourceExhausted,
		},
		{name: "InvalidInputError", inputError: errors.InvalidInputError("bad input"), expectedCode: codes.InvalidArgument},
		{name: "InternalError", inputError: errors.InternalError("internal failure"), expectedCode: codes.Internal},
		{name: "PermissionDeniedError", inputError: errors.PermissionDeniedError("permission denied"), expectedCode: codes.PermissionDenied},
		{name: "InvalidJSONError", inputError: errors.InvalidJSONError("invalid json"), expectedCode: codes.Internal},
		{name: "UnknownError", inputError: errors.New("unknown error"), expectedCode: codes.Unknown},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{}
			result := plugin.getCSIErrorForOrchestratorError(tc.inputError)

			s, ok := status.FromError(result)
			require.True(t, ok)
			assert.Equal(t, tc.expectedCode, s.Code())
		})
	}
}

func TestReadAESKey(t *testing.T) {
	testCases := []struct {
		name          string
		aesKeyFile    string
		createFile    bool
		fileContent   string
		expectedError bool
	}{
		{name: "Success - Valid file", createFile: true, fileContent: "test-key-content", expectedError: false},
		{name: "Success - Empty filename", aesKeyFile: "", createFile: false, expectedError: false},
		{name: "Error - File read fails", aesKeyFile: "/nonexistent/file", createFile: false, expectedError: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			readAESKeysOnce = sync.Once{}
			aesKeySingleton = nil
			aesKeyError = nil

			aesKeyFile := tc.aesKeyFile
			if tc.createFile {
				tmpFile, err := os.CreateTemp("", "test-key-*")
				require.NoError(t, err)
				defer os.Remove(tmpFile.Name())
				_, err = tmpFile.WriteString(tc.fileContent)
				require.NoError(t, err)
				tmpFile.Close()
				aesKeyFile = tmpFile.Name()
			}

			result, err := ReadAESKey(context.Background(), aesKeyFile)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				if tc.aesKeyFile == "" && !tc.createFile {
					assert.Empty(t, result)
				} else {
					assert.Equal(t, []byte(tc.fileContent), result)
				}
			}
		})
	}
}

func TestPlugin_IsReady(t *testing.T) {
	t.Run("Controller role has no node core - always ready", func(t *testing.T) {
		plugin := &Plugin{role: CSIController}
		assert.True(t, plugin.IsReady())
	})

	t.Run("Node role delegates to a not-yet-bootstrapped node core", func(t *testing.T) {
		plugin := &Plugin{role: CSINode, nodeOrchestrator: newNotReadyNodeCore()}
		assert.False(t, plugin.IsReady())
	})

	t.Run("Node role delegates to a bootstrapped node core", func(t *testing.T) {
		plugin := &Plugin{role: CSINode, nodeOrchestrator: newReadyNodeCore(t)}
		assert.True(t, plugin.IsReady())
	})
}
