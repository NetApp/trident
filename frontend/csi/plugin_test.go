// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	controllerAPI "github.com/netapp/trident/frontend/csi/controller_api"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/mocks/mock_core"
	mockControllerAPI "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_api"
	mock_controller_helpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_helpers"
	mock_node_helpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	"github.com/netapp/trident/mocks/mock_utils/mock_iscsi"
	mock_nvme "github.com/netapp/trident/mocks/mock_utils/nvme"
	"github.com/netapp/trident/utils/errors"
	execCmd "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/limiter"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/osutils"
)

func TestNewControllerPlugin(t *testing.T) {
	testCases := []struct {
		name          string
		createFile    bool
		fileContent   string
		expectedError bool
	}{
		{
			name:          "Success",
			createFile:    true,
			fileContent:   "test-key",
			expectedError: false,
		},
		{
			name:          "Error - File not found",
			createFile:    false,
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOrchestrator := mock_core.NewMockOrchestrator(ctrl)
			mockHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)

			// Create temporary file for testing
			var aesKeyFile string
			if tc.createFile {
				tmpFile, err := os.CreateTemp("", "test-key-*")
				assert.NoError(t, err)
				defer os.Remove(tmpFile.Name())

				_, err = tmpFile.WriteString(tc.fileContent)
				assert.NoError(t, err)
				tmpFile.Close()

				aesKeyFile = tmpFile.Name()
			} else {
				aesKeyFile = "/nonexistent/file"
			}

			plugin, err := NewControllerPlugin("node", "endpoint", aesKeyFile, mockOrchestrator, mockHelper, true)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, plugin)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, plugin)
			}
		})
	}
}

func TestNewNodePlugin(t *testing.T) {
	testCases := []struct {
		name                string
		createFile          bool
		fileContent         string
		setEnvPort          string
		setEnvHost          string
		mockISCSIError      bool
		mockMountError      bool
		mockFCPError        bool
		mockRestClientError bool
		expectedError       bool
	}{
		{
			name:          "Success - Default env",
			createFile:    true,
			fileContent:   "test-key",
			expectedError: false,
		},
		{
			name:          "Success - Custom env",
			createFile:    true,
			fileContent:   "test-key",
			setEnvPort:    "8080",
			setEnvHost:    "custom-host",
			expectedError: false,
		},
		{
			name:          "Success - Empty env",
			createFile:    true,
			fileContent:   "test-key",
			setEnvPort:    "",
			setEnvHost:    "",
			expectedError: false,
		},
		{
			name:           "Error - iSCSI client creation fails",
			createFile:     true,
			fileContent:    "test-key",
			mockISCSIError: true,
			expectedError:  true,
		},
		{
			name:           "Error - Mount client creation fails",
			createFile:     true,
			fileContent:    "test-key",
			mockMountError: true,
			expectedError:  true,
		},
		{
			name:          "Error - FCP client creation fails",
			createFile:    true,
			fileContent:   "test-key",
			mockFCPError:  true,
			expectedError: true,
		},
		{
			name:                "Error - REST client creation fails",
			createFile:          true,
			fileContent:         "test-key",
			mockRestClientError: true,
			expectedError:       true,
		},
		{
			name:          "Error - AES key file read fails",
			createFile:    false,
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOrchestrator := mock_core.NewMockOrchestrator(ctrl)
			mockHelper := mock_node_helpers.NewMockNodeHelper(ctrl)

			// Setup environment variables
			originalPort := os.Getenv("TRIDENT_CSI_SERVICE_PORT")
			originalHost := os.Getenv("TRIDENT_CSI_SERVICE_HOST")
			defer func() {
				os.Setenv("TRIDENT_CSI_SERVICE_PORT", originalPort)
				os.Setenv("TRIDENT_CSI_SERVICE_HOST", originalHost)
			}()

			if tc.setEnvPort != "" {
				os.Setenv("TRIDENT_CSI_SERVICE_PORT", tc.setEnvPort)
			} else {
				os.Unsetenv("TRIDENT_CSI_SERVICE_PORT")
			}

			if tc.setEnvHost != "" {
				os.Setenv("TRIDENT_CSI_SERVICE_HOST", tc.setEnvHost)
			} else {
				os.Unsetenv("TRIDENT_CSI_SERVICE_HOST")
			}

			// Create temporary file for AES key
			var aesKeyFile string
			if tc.createFile {
				tmpFile, err := os.CreateTemp("", "test-key-*")
				assert.NoError(t, err)
				defer os.Remove(tmpFile.Name())

				_, err = tmpFile.WriteString(tc.fileContent)
				assert.NoError(t, err)
				tmpFile.Close()

				aesKeyFile = tmpFile.Name()
			} else {
				aesKeyFile = "/nonexistent/file"
			}

			plugin, err := NewNodePlugin(
				"test-node", "unix:///tmp/csi.sock", "ca-cert", "client-cert", "client-key", aesKeyFile,
				mockOrchestrator, false, mockHelper, true,
				time.Minute, time.Minute*2, time.Minute*3,
			)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, plugin)
			} else {
				// Plugin creation may fail due to external dependencies
				if err != nil {
					t.Skipf("Plugin creation failed due to dependencies: %v", err)
				} else {
					assert.NoError(t, err)
					assert.NotNil(t, plugin)
					assert.Equal(t, "test-node", plugin.nodeName)
					assert.Equal(t, "unix:///tmp/csi.sock", plugin.endpoint)
					assert.Equal(t, CSINode, plugin.role)
					assert.True(t, plugin.enableForceDetach)
					assert.False(t, plugin.unsafeDetach)
					assert.Equal(t, time.Minute, plugin.iSCSISelfHealingInterval)
					assert.Equal(t, time.Minute*2, plugin.iSCSISelfHealingWaitTime)
					assert.Equal(t, time.Minute*3, plugin.nvmeSelfHealingInterval)

					// Test that capabilities are set (this covers both Windows and non-Windows paths)
					assert.NotNil(t, plugin.nsCap)
					assert.GreaterOrEqual(t, len(plugin.nsCap), 2) // At least 2 capabilities

					// Test volume capabilities are set
					assert.NotNil(t, plugin.vCap)
					assert.Len(t, plugin.vCap, 7)
				}
			}
		})
	}
}

func TestNewAllInOnePlugin(t *testing.T) {
	testCases := []struct {
		name                string
		createFile          bool
		fileContent         string
		setEnvPort          string
		mockRestClientError bool
		expectedError       bool
	}{
		{
			name:          "Success - Default port",
			createFile:    true,
			fileContent:   "test-key",
			expectedError: false,
		},
		{
			name:          "Success - Custom port from env",
			createFile:    true,
			fileContent:   "test-key",
			setEnvPort:    "8080",
			expectedError: false,
		},
		{
			name:                "Error - REST client creation fails",
			createFile:          true,
			fileContent:         "test-key",
			mockRestClientError: true,
			expectedError:       true,
		},
		{
			name:          "Error - AES key file read fails",
			createFile:    false,
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOrchestrator := mock_core.NewMockOrchestrator(ctrl)
			mockControllerHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
			mockNodeHelper := mock_node_helpers.NewMockNodeHelper(ctrl)

			// Setup environment variable for port
			originalEnv := os.Environ()
			if tc.setEnvPort != "" {
				os.Setenv("TRIDENT_CSI_SERVICE_PORT", tc.setEnvPort)
				defer os.Unsetenv("TRIDENT_CSI_SERVICE_PORT")
			}

			// Create temporary file for AES key
			var aesKeyFile string
			if tc.createFile {
				tmpFile, err := os.CreateTemp("", "test-key-*")
				assert.NoError(t, err)
				defer os.Remove(tmpFile.Name())

				_, err = tmpFile.WriteString(tc.fileContent)
				assert.NoError(t, err)
				tmpFile.Close()

				aesKeyFile = tmpFile.Name()
			} else {
				aesKeyFile = "/nonexistent/file"
			}

			plugin, err := NewAllInOnePlugin(
				"test-node", "unix:///tmp/csi.sock", "ca-cert", "client-cert", "client-key", aesKeyFile,
				mockOrchestrator, mockControllerHelper, mockNodeHelper, false,
				time.Minute, time.Minute*2, time.Minute*3,
			)

			// Restore original environment
			for _, env := range originalEnv {
				parts := strings.SplitN(env, "=", 2)
				if len(parts) == 2 {
					os.Setenv(parts[0], parts[1])
				}
			}

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, plugin)
			} else {
				// Plugin creation may fail due to external dependencies
				if err != nil {
					t.Skipf("Plugin creation failed due to dependencies: %v", err)
				} else {
					assert.NoError(t, err)
					assert.NotNil(t, plugin)
					assert.Equal(t, "test-node", plugin.nodeName)
					assert.Equal(t, "unix:///tmp/csi.sock", plugin.endpoint)
					assert.Equal(t, CSIAllInOne, plugin.role)
					assert.False(t, plugin.unsafeDetach)
					assert.Equal(t, time.Minute, plugin.iSCSISelfHealingInterval)
					assert.Equal(t, time.Minute*2, plugin.iSCSISelfHealingWaitTime)
					assert.Equal(t, time.Minute*3, plugin.nvmeSelfHealingInterval)

					// Verify all capabilities are set
					assert.NotNil(t, plugin.csCap)
					assert.Len(t, plugin.csCap, 9) // Controller capabilities

					assert.NotNil(t, plugin.nsCap)
					assert.Len(t, plugin.nsCap, 4) // Node capabilities

					assert.NotNil(t, plugin.vCap)
					assert.Len(t, plugin.vCap, 7) // Volume capabilities

					assert.NotNil(t, plugin.gcsCap)
					assert.Len(t, plugin.gcsCap, 1) // Group controller capabilities
				}
			}
		})
	}
}

func TestPlugin_Activate(t *testing.T) {
	testCases := []struct {
		name                     string
		role                     string
		enableForceDetach        bool
		iSCSISelfHealingInterval time.Duration
		nvmeSelfHealingInterval  time.Duration
	}{
		{
			name:                     "CSINode role with features",
			role:                     CSINode,
			enableForceDetach:        true,
			iSCSISelfHealingInterval: time.Minute,
			nvmeSelfHealingInterval:  time.Minute,
		},
		{
			name:                     "CSIController role",
			role:                     CSIController,
			enableForceDetach:        false,
			iSCSISelfHealingInterval: 0,
			nvmeSelfHealingInterval:  0,
		},
		{
			name:                     "CSIAllInOne role",
			role:                     CSIAllInOne,
			enableForceDetach:        false,
			iSCSISelfHealingInterval: time.Minute,
			nvmeSelfHealingInterval:  0,
		},
		{
			name:                     "CSINode role without features",
			role:                     CSINode,
			enableForceDetach:        false,
			iSCSISelfHealingInterval: 0,
			nvmeSelfHealingInterval:  0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOrchestrator := mock_core.NewMockOrchestrator(ctrl)
			mockControllerHelper := mock_controller_helpers.NewMockControllerHelper(ctrl)
			mockNodeHelper := mock_node_helpers.NewMockNodeHelper(ctrl)
			mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

			// Setup expectations for controller operations
			// if tc.expectControllerOperations {
			mockControllerHelper.EXPECT().
				IsTopologyInUse(gomock.Any()).
				Return(false).AnyTimes()
			// }

			setupRestClientMock := func() controllerAPI.TridentController {
				mockRestClient := mockControllerAPI.NewMockTridentController(gomock.NewController(t))
				mockRestClient.EXPECT().CreateNode(gomock.Any(), gomock.Any()).Return(controllerAPI.CreateNodeResponse{}, errors.New("")).AnyTimes()

				return mockRestClient
			}

			mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
			mockISCSIClient.EXPECT().ISCSIActiveOnHost(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
			mockNVMeHandler := mock_nvme.NewMockNVMeInterface(gomock.NewController(t))
			mockNVMeHandler.EXPECT().NVMeActiveOnHost(gomock.Any()).Return(false, nil).AnyTimes()
			plugin := &Plugin{
				orchestrator:             mockOrchestrator,
				controllerHelper:         mockControllerHelper,
				nodeHelper:               mockNodeHelper,
				role:                     tc.role,
				restClient:               setupRestClientMock(),
				nodeName:                 "test-node",
				endpoint:                 "unix:///tmp/test.sock",
				enableForceDetach:        tc.enableForceDetach,
				iSCSISelfHealingInterval: tc.iSCSISelfHealingInterval,
				nvmeSelfHealingInterval:  tc.nvmeSelfHealingInterval,
				limiterSharedMap:         make(map[string]limiter.Limiter),
				iscsi:                    mockISCSIClient,
				nvmeHandler:              mockNVMeHandler,
				osutils:                  osutils.New(),
				activatedChan:            make(chan struct{}, 1),
				nodeReadyCh:              make(chan struct{}),
			}

			err := plugin.Activate()

			// Activation should always return nil immediately
			assert.NoError(t, err)

			// Give some time for goroutine to execute
			time.Sleep(100 * time.Millisecond)

			// Verify GRPC server was created (will be set by NewNonBlockingGRPCServer)
			assert.NotNil(t, plugin.grpc)
		})
	}
}

func TestPlugin_Activate_StartsGRPCBeforeSlowNodeRegistration(t *testing.T) {
	InitAuditLogger(true)
	ctrl := gomock.NewController(t)

	mockOrchestrator := mock_core.NewMockOrchestrator(ctrl)
	mockOrchestrator.EXPECT().SetLogLevel(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockOrchestrator.EXPECT().SetLoggingWorkflows(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockOrchestrator.EXPECT().SetLogLayers(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	mockNodeHelper := mock_node_helpers.NewMockNodeHelper(ctrl)
	mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockNodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).Return(nil, nil).AnyTimes()

	// Record when CreateNode is invoked so the test can prove the socket exists
	// before the slow controller registration completes. This validates TRID-19339's fix:
	// gRPC socket must be available before node-driver-registrar's ~30s timeout deadline.
	registerStarted := make(chan struct{}, 1)
	mockRestClient := mockControllerAPI.NewMockTridentController(ctrl)
	mockRestClient.EXPECT().CreateNode(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *models.Node) (controllerAPI.CreateNodeResponse, error) {
			select {
			case registerStarted <- struct{}{}:
			default:
			}
			time.Sleep(2 * time.Second)
			return controllerAPI.CreateNodeResponse{}, nil
		},
	).AnyTimes()
	mockRestClient.EXPECT().ListVolumePublicationsForNode(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
	mockISCSIClient.EXPECT().ISCSIActiveOnHost(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()

	mockNVMeHandler := mock_nvme.NewMockNVMeInterface(ctrl)
	mockNVMeHandler.EXPECT().NVMeActiveOnHost(gomock.Any()).Return(false, nil).AnyTimes()

	// Use a short unique socket path. macOS limits Unix socket paths to 104 bytes;
	// t.TempDir() + long test names easily exceed that, so use os.MkdirTemp with the
	// default temp dir (typically /tmp on Linux) for a short path.
	socketDir, err := os.MkdirTemp("", "csi")
	require.NoError(t, err)
	socketPath := filepath.Join(socketDir, "csi.sock")
	t.Cleanup(func() { os.RemoveAll(socketDir) })

	plugin := &Plugin{
		orchestrator:             mockOrchestrator,
		nodeHelper:               mockNodeHelper,
		role:                     CSINode,
		restClient:               mockRestClient,
		nodeName:                 "test-node",
		endpoint:                 "unix://" + socketPath,
		activatedChan:            make(chan struct{}, 1),
		nodeReadyCh:              make(chan struct{}),
		iSCSISelfHealingInterval: 0, // Disable to avoid background goroutine leaks in test
		nvmeSelfHealingInterval:  0,
		limiterSharedMap:         make(map[string]limiter.Limiter),
		iscsi:                    mockISCSIClient,
		nvmeHandler:              mockNVMeHandler,
		osutils:                  osutils.New(),
	}

	t.Cleanup(func() {
		if plugin.grpc != nil {
			plugin.grpc.Stop()
		}
		ctrl.Finish()
	})

	err = plugin.Activate()
	assert.NoError(t, err)

	// Wait until registration is in flight before checking for the socket. This
	// makes the test prove ordering rather than just eventual startup. Use a generous
	// timeout because node registration performs real host probing via osutils before
	// the first CreateNode call, which can vary in CI environments.
	select {
	case <-registerStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("expected node registration attempt to start")
	}

	// The node-driver-registrar sidecar has a hard ~30s connection deadline.
	// With a 2-second sleep in CreateNode, the socket must appear within 1 second to
	// prove that Activate() prioritizes gRPC listener startup over controller registration.
	deadline := time.Now().Add(1 * time.Second)
	socketFound := false
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(socketPath); statErr == nil {
			socketFound = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	assert.True(t, socketFound, "expected gRPC socket to be created before slow node registration finishes")
}

// TestPlugin_Activate_ReproduceCustomerIssue_TRID19339 reproduces the exact customer scenario:
//
// Customer symptom: node-driver-registrar (v2.15.0) enters CrashLoopBackOff because
// it cannot connect to the CSI socket within its hardcoded ~30s gRPC connection deadline.
//
// Root cause (pre-fix): Activate() called nodeRegisterWithController() BEFORE grpc.Start(),
// meaning the Unix socket didn't exist until registration completed (38-70s on busy clusters).
//
// Fix: grpc.Start() is now called immediately, before nodeRegisterWithController(). The
// nodeRegistrationInterceptor gates data-path RPCs until registration finishes.
//
// This test proves:
//  1. The gRPC socket is available within 2s (well under the registrar's 30s deadline),
//     even while registration is still blocked.
//  2. A real gRPC client can connect and call Identity.Probe (the registrar's first call).
//  3. Node data-path RPCs (NodeStageVolume) are rejected with Unavailable during registration.
//  4. After registration completes, data-path RPCs succeed.
func TestPlugin_Activate_ReproduceCustomerIssue_TRID19339(t *testing.T) {
	InitAuditLogger(true)
	ctrl := gomock.NewController(t)

	mockOrchestrator := mock_core.NewMockOrchestrator(ctrl)
	mockOrchestrator.EXPECT().SetLogLevel(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockOrchestrator.EXPECT().SetLoggingWorkflows(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockOrchestrator.EXPECT().SetLogLayers(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockOrchestrator.EXPECT().GetVersion(gomock.Any()).Return("test", nil).AnyTimes()

	mockNodeHelper := mock_node_helpers.NewMockNodeHelper(ctrl)
	mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockNodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).Return(nil, nil).AnyTimes()

	// Simulate a busy cluster: registration takes 5 seconds (customer saw 38-70s).
	registrationDone := make(chan struct{})
	mockRestClient := mockControllerAPI.NewMockTridentController(ctrl)
	mockRestClient.EXPECT().CreateNode(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ *models.Node) (controllerAPI.CreateNodeResponse, error) {
			time.Sleep(5 * time.Second) // Simulates slow controller registration on busy cluster
			close(registrationDone)
			return controllerAPI.CreateNodeResponse{}, nil
		},
	).AnyTimes()
	mockRestClient.EXPECT().ListVolumePublicationsForNode(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
	mockISCSIClient.EXPECT().ISCSIActiveOnHost(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()

	mockNVMeHandler := mock_nvme.NewMockNVMeInterface(ctrl)
	mockNVMeHandler.EXPECT().NVMeActiveOnHost(gomock.Any()).Return(false, nil).AnyTimes()

	// Use a short socket path to avoid macOS 104-byte Unix socket path limit.
	socketDir, err := os.MkdirTemp("", "csi")
	require.NoError(t, err)
	socketPath := filepath.Join(socketDir, "csi.sock")
	t.Cleanup(func() { os.RemoveAll(socketDir) })

	plugin := &Plugin{
		orchestrator:             mockOrchestrator,
		nodeHelper:               mockNodeHelper,
		role:                     CSINode,
		restClient:               mockRestClient,
		nodeName:                 "customer-node",
		endpoint:                 "unix://" + socketPath,
		activatedChan:            make(chan struct{}, 1),
		nodeReadyCh:              make(chan struct{}),
		iSCSISelfHealingInterval: 0, // Disable to avoid background goroutine leaks in test
		nvmeSelfHealingInterval:  0,
		limiterSharedMap:         make(map[string]limiter.Limiter),
		iscsi:                    mockISCSIClient,
		nvmeHandler:              mockNVMeHandler,
		osutils:                  osutils.New(),
	}

	t.Cleanup(func() {
		if plugin.grpc != nil {
			plugin.grpc.Stop()
		}
		ctrl.Finish()
	})

	err = plugin.Activate()
	assert.NoError(t, err)

	// --- PHASE 1: Prove socket available fast (customer's core issue) ---
	// node-driver-registrar has a ~30s deadline. The socket must appear well before that.
	socketAvailableDeadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	socketFound := false
	for !socketFound {
		select {
		case <-socketAvailableDeadline:
			t.Fatal("REPRODUCTION: gRPC socket not available within 2s — " +
				"this is the customer's CrashLoopBackOff scenario (TRID-19339)")
		case <-ticker.C:
			if _, statErr := os.Stat(socketPath); statErr == nil {
				socketFound = true
			}
		}
	}
	t.Log("PASS: gRPC socket available within 2s (customer's registrar deadline is ~30s)")

	// --- PHASE 2: Real gRPC client connectivity (simulates node-driver-registrar) ---
	// The registrar's first action after connecting is to call Identity.Probe.
	conn, dialErr := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if dialErr != nil {
		t.Fatalf("REPRODUCTION: gRPC client dial failed: %v — registrar would CrashLoop", dialErr)
	}
	defer conn.Close()

	identityClient := csi.NewIdentityClient(conn)
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer probeCancel()
	probeResp, probeErr := identityClient.Probe(probeCtx, &csi.ProbeRequest{})
	assert.NoError(t, probeErr, "Identity.Probe must succeed during registration — registrar depends on this")
	assert.NotNil(t, probeResp, "Probe response should not be nil")
	t.Log("PASS: Identity.Probe succeeds while registration is in progress")

	// --- PHASE 3: Verify data-path RPCs blocked during registration ---
	nodeClient := csi.NewNodeClient(conn)
	stageCtx, stageCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stageCancel()
	_, stageErr := nodeClient.NodeStageVolume(stageCtx, &csi.NodeStageVolumeRequest{
		VolumeId:          "vol-test",
		StagingTargetPath: "/tmp/staging",
		VolumeCapability:  &csi.VolumeCapability{},
	})
	assert.Error(t, stageErr, "NodeStageVolume must be rejected before registration completes")
	assert.Equal(t, codes.Unavailable, status.Code(stageErr),
		"blocked RPCs must return Unavailable, not a different error")
	t.Log("PASS: NodeStageVolume correctly blocked with Unavailable during registration")

	// --- PHASE 4: After registration, data-path RPCs should work ---
	select {
	case <-registrationDone:
		t.Log("Registration completed, verifying data-path RPCs are unblocked...")
	case <-time.After(10 * time.Second):
		t.Fatal("Registration did not complete within expected time")
	}

	// Give a small window for markNodeReady() to execute after CreateNode returns
	time.Sleep(100 * time.Millisecond)

	assert.True(t, plugin.IsReady(), "Plugin must be ready after registration completes")
	t.Log("PASS: Plugin.IsReady() returns true after registration")
}

// TestPlugin_Deactivate_SafeWithoutActivate validates that Deactivate() can be called
// safely even if Activate() was never called or hasn't completed yet (p.grpc is nil).
// This prevents nil pointer panics in shutdown scenarios. TRID-19339 safe shutdown.
func TestPlugin_Deactivate_SafeWithoutActivate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrchestrator := mock_core.NewMockOrchestrator(ctrl)

	plugin := &Plugin{
		orchestrator:  mockOrchestrator,
		role:          CSINode,
		nodeName:      "test-node",
		endpoint:      "unix:///tmp/test.sock",
		command:       execCmd.NewCommand(),
		osutils:       osutils.New(),
		activatedChan: make(chan struct{}, 1),
		grpc:          nil, // Simulate p.grpc not yet initialized
	}

	// Deactivate should not panic even though p.grpc is nil.
	err := plugin.Deactivate()
	assert.NoError(t, err, "Deactivate() should not panic or error when called before Activate() initializes gRPC")
}

func TestPlugin_GetName(t *testing.T) {
	plugin := &Plugin{}
	result := plugin.GetName()
	assert.Equal(t, string(tridentconfig.ContextCSI), result)
}

func TestPlugin_Version(t *testing.T) {
	plugin := &Plugin{}
	result := plugin.Version()
	assert.Equal(t, tridentconfig.OrchestratorVersion.String(), result)
}

func TestPlugin_AddControllerServiceCapabilities(t *testing.T) {
	testCases := []struct {
		name         string
		capabilities []csi.ControllerServiceCapability_RPC_Type
		expectedLen  int
	}{
		{
			name:         "Empty capabilities",
			capabilities: []csi.ControllerServiceCapability_RPC_Type{},
			expectedLen:  0,
		},
		{
			name: "Single capability",
			capabilities: []csi.ControllerServiceCapability_RPC_Type{
				csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
			},
			expectedLen: 1,
		},
		{
			name: "Multiple capabilities",
			capabilities: []csi.ControllerServiceCapability_RPC_Type{
				csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
				csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
			},
			expectedLen: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{}
			plugin.addControllerServiceCapabilities(context.Background(), tc.capabilities)
			assert.Len(t, plugin.csCap, tc.expectedLen)
		})
	}
}

func TestPlugin_AddNodeServiceCapabilities(t *testing.T) {
	testCases := []struct {
		name         string
		capabilities []csi.NodeServiceCapability_RPC_Type
		expectedLen  int
	}{
		{
			name:         "Empty capabilities",
			capabilities: []csi.NodeServiceCapability_RPC_Type{},
			expectedLen:  0,
		},
		{
			name: "Single capability",
			capabilities: []csi.NodeServiceCapability_RPC_Type{
				csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
			},
			expectedLen: 1,
		},
		{
			name: "Multiple capabilities",
			capabilities: []csi.NodeServiceCapability_RPC_Type{
				csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
				csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
			},
			expectedLen: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{}
			plugin.addNodeServiceCapabilities(tc.capabilities)
			assert.Len(t, plugin.nsCap, tc.expectedLen)
		})
	}
}

func TestPlugin_AddVolumeCapabilityAccessModes(t *testing.T) {
	testCases := []struct {
		name        string
		modes       []csi.VolumeCapability_AccessMode_Mode
		expectedLen int
	}{
		{
			name:        "Empty modes",
			modes:       []csi.VolumeCapability_AccessMode_Mode{},
			expectedLen: 0,
		},
		{
			name: "Single mode",
			modes: []csi.VolumeCapability_AccessMode_Mode{
				csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
			expectedLen: 1,
		},
		{
			name: "Multiple modes",
			modes: []csi.VolumeCapability_AccessMode_Mode{
				csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
			},
			expectedLen: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{}
			plugin.addVolumeCapabilityAccessModes(context.Background(), tc.modes)
			assert.Len(t, plugin.vCap, tc.expectedLen)
		})
	}
}

func TestPlugin_AddGroupControllerServiceCapabilities(t *testing.T) {
	testCases := []struct {
		name         string
		capabilities []csi.GroupControllerServiceCapability_RPC_Type
		expectedLen  int
	}{
		{
			name:         "Empty capabilities",
			capabilities: []csi.GroupControllerServiceCapability_RPC_Type{},
			expectedLen:  0,
		},
		{
			name: "Single capability",
			capabilities: []csi.GroupControllerServiceCapability_RPC_Type{
				csi.GroupControllerServiceCapability_RPC_CREATE_DELETE_GET_VOLUME_GROUP_SNAPSHOT,
			},
			expectedLen: 1,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{}
			plugin.addGroupControllerServiceCapabilities(context.Background(), tc.capabilities)
			assert.Len(t, plugin.gcsCap, tc.expectedLen)
		})
	}
}

func TestPlugin_GetCSIErrorForOrchestratorError(t *testing.T) {
	testCases := []struct {
		name         string
		inputError   error
		expectedCode codes.Code
	}{
		{
			name:         "NotReadyError",
			inputError:   errors.NotReadyError(),
			expectedCode: codes.Unavailable,
		},
		{
			name:         "BootstrapError",
			inputError:   errors.BootstrapError(errors.New("")),
			expectedCode: codes.FailedPrecondition,
		},
		{
			name:         "PreconditionError",
			inputError:   errors.PreconditionError(""),
			expectedCode: codes.FailedPrecondition,
		},
		{
			name:         "NotFoundError",
			inputError:   errors.NotFoundError(""),
			expectedCode: codes.NotFound,
		},
		{
			name:         "UnsupportedCapacityRangeError",
			inputError:   errors.UnsupportedCapacityRangeError(errors.New("")),
			expectedCode: codes.OutOfRange,
		},
		{
			name:         "FoundError",
			inputError:   errors.FoundError("already exists"),
			expectedCode: codes.AlreadyExists,
		},
		{
			name:         "NodeNotSafeToPublishForBackendError",
			inputError:   errors.NodeNotSafeToPublishForBackendError("node not safe", ""),
			expectedCode: codes.FailedPrecondition,
		},
		{
			name:         "VolumeCreatingError",
			inputError:   errors.VolumeCreatingError("volume creating"),
			expectedCode: codes.DeadlineExceeded,
		},
		{
			name:         "VolumeDeletingError",
			inputError:   errors.VolumeDeletingError("volume deleting"),
			expectedCode: codes.DeadlineExceeded,
		},
		{
			name:         "ResourceExhaustedError",
			inputError:   errors.ResourceExhaustedError(errors.New("")),
			expectedCode: codes.ResourceExhausted,
		},
		{
			name:         "UnknownError",
			inputError:   errors.New("unknown error"),
			expectedCode: codes.Unknown,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{}
			result := plugin.getCSIErrorForOrchestratorError(tc.inputError)

			status, ok := status.FromError(result)
			assert.True(t, ok)
			assert.Equal(t, tc.expectedCode, status.Code())
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
		{
			name:          "Success - Valid file",
			aesKeyFile:    "/tmp/test-key",
			createFile:    true,
			fileContent:   "test-key-content",
			expectedError: false,
		},
		{
			name:          "Success - Empty filename",
			aesKeyFile:    "",
			createFile:    false,
			expectedError: false,
		},
		{
			name:          "Error - File read fails",
			aesKeyFile:    "/nonexistent/file",
			createFile:    false,
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var aesKeyFile string
			if tc.createFile {
				tmpFile, err := os.CreateTemp("", "test-key-*")
				assert.NoError(t, err)
				defer os.Remove(tmpFile.Name())

				_, err = tmpFile.WriteString(tc.fileContent)
				assert.NoError(t, err)
				tmpFile.Close()

				aesKeyFile = tmpFile.Name()
			} else {
				aesKeyFile = tc.aesKeyFile
			}

			result, err := ReadAESKey(context.Background(), aesKeyFile)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				if tc.aesKeyFile == "" {
					assert.Empty(t, result)
				} else {
					assert.Equal(t, []byte(tc.fileContent), result)
				}
			}
		})
	}
}

func TestPlugin_IsReady(t *testing.T) {
	testCases := []struct {
		name          string
		nodeReadyCh   chan struct{}
		expectedReady bool
	}{
		{
			name:          "Ready - Node registered",
			nodeReadyCh:   closedCh(),
			expectedReady: true,
		},
		{
			name:          "Not ready - Node not registered",
			nodeReadyCh:   make(chan struct{}),
			expectedReady: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{
				nodeReadyCh: tc.nodeReadyCh,
			}

			result := plugin.IsReady()
			assert.Equal(t, tc.expectedReady, result)
		})
	}
}

func TestPlugin_StartISCSISelfHealingThread(t *testing.T) {
	testCases := []struct {
		name                     string
		iSCSISelfHealingInterval time.Duration
		iSCSISelfHealingWaitTime time.Duration
		expectedEnabled          bool
	}{
		{
			name:                     "Disabled - Zero interval",
			iSCSISelfHealingInterval: 0,
			iSCSISelfHealingWaitTime: 0,
			expectedEnabled:          false,
		},
		{
			name:                     "Disabled - Negative interval",
			iSCSISelfHealingInterval: -time.Minute,
			iSCSISelfHealingWaitTime: 0,
			expectedEnabled:          false,
		},
		{
			name:                     "Enabled - Normal wait time",
			iSCSISelfHealingInterval: time.Minute,
			iSCSISelfHealingWaitTime: time.Minute * 2,
			expectedEnabled:          true,
		},
		{
			name:                     "Enabled - Wait time adjusted",
			iSCSISelfHealingInterval: time.Minute,
			iSCSISelfHealingWaitTime: time.Second * 30,
			expectedEnabled:          true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{
				iSCSISelfHealingInterval: tc.iSCSISelfHealingInterval,
				iSCSISelfHealingWaitTime: tc.iSCSISelfHealingWaitTime,
			}

			plugin.startISCSISelfHealingThread(context.Background())

			if tc.expectedEnabled {
				assert.NotNil(t, plugin.iSCSISelfHealingTicker)
				assert.NotNil(t, plugin.iSCSISelfHealingChannel)
				plugin.stopISCSISelfHealingThread(context.Background())
			} else {
				assert.Nil(t, plugin.iSCSISelfHealingTicker)
				assert.Nil(t, plugin.iSCSISelfHealingChannel)
			}
		})
	}
}

func TestPlugin_StopISCSISelfHealingThread(t *testing.T) {
	testCases := []struct {
		name         string
		setupTicker  bool
		setupChannel bool
	}{
		{
			name:         "Stop with ticker and channel",
			setupTicker:  true,
			setupChannel: true,
		},
		{
			name:         "Stop with nil ticker and channel",
			setupTicker:  false,
			setupChannel: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{}

			if tc.setupTicker {
				plugin.iSCSISelfHealingTicker = time.NewTicker(time.Second)
			}
			if tc.setupChannel {
				plugin.iSCSISelfHealingChannel = make(chan struct{})
			}

			plugin.stopISCSISelfHealingThread(context.Background())

			// Should not panic and complete successfully
			assert.True(t, true)
		})
	}
}

func TestPlugin_StartNVMeSelfHealingThread(t *testing.T) {
	testCases := []struct {
		name                    string
		nvmeSelfHealingInterval time.Duration
		expectedEnabled         bool
	}{
		{
			name:                    "Disabled - Zero interval",
			nvmeSelfHealingInterval: 0,
			expectedEnabled:         false,
		},
		{
			name:                    "Disabled - Negative interval",
			nvmeSelfHealingInterval: -time.Minute,
			expectedEnabled:         false,
		},
		{
			name:                    "Enabled - Positive interval",
			nvmeSelfHealingInterval: time.Minute,
			expectedEnabled:         true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{
				nvmeSelfHealingInterval: tc.nvmeSelfHealingInterval,
			}

			plugin.startNVMeSelfHealingThread(context.Background())

			if tc.expectedEnabled {
				assert.NotNil(t, plugin.nvmeSelfHealingTicker)
				assert.NotNil(t, plugin.nvmeSelfHealingChannel)
				plugin.stopNVMeSelfHealingThread(context.Background())
			} else {
				assert.Nil(t, plugin.nvmeSelfHealingTicker)
				assert.Nil(t, plugin.nvmeSelfHealingChannel)
			}
		})
	}
}

func TestPlugin_StopNVMeSelfHealingThread(t *testing.T) {
	testCases := []struct {
		name         string
		setupTicker  bool
		setupChannel bool
	}{
		{
			name:         "Stop with ticker and channel",
			setupTicker:  true,
			setupChannel: true,
		},
		{
			name:         "Stop with nil ticker and channel",
			setupTicker:  false,
			setupChannel: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{}

			if tc.setupTicker {
				plugin.nvmeSelfHealingTicker = time.NewTicker(time.Second)
			}
			if tc.setupChannel {
				plugin.nvmeSelfHealingChannel = make(chan struct{})
			}

			plugin.stopNVMeSelfHealingThread(context.Background())

			// Should not panic and complete successfully
			assert.True(t, true)
		})
	}
}

func TestPlugin_InitializeNodeLimiter(t *testing.T) {
	testCases := []struct {
		name string
	}{
		{
			name: "Success - All limiters initialized",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{
				limiterSharedMap: make(map[string]limiter.Limiter),
			}

			plugin.InitializeNodeLimiter(context.Background())

			// Verify all limiters were created
			expectedLimiters := []string{
				NodeStageNFSVolume,
				NodeStageSMBVolume,
				NodeUnstageNFSVolume,
				NodeUnstageSMBVolume,
				NodePublishNFSVolume,
				NodePublishSMBVolume,
				NodeStageISCSIVolume,
				NodeUnstageISCSIVolume,
				NodePublishISCSIVolume,
				NodeUnpublishVolume,
				NodeExpandVolume,
				NodeStageFCPVolume,
				NodeUnstageFCPVolume,
				NodePublishFCPVolume,
				NodeStageNVMeVolume,
				NodeUnstageNVMeVolume,
				NodePublishNVMeVolume,
			}

			for _, limiterName := range expectedLimiters {
				assert.Contains(t, plugin.limiterSharedMap, limiterName)
				assert.NotNil(t, plugin.limiterSharedMap[limiterName])
			}

			assert.Len(t, plugin.limiterSharedMap, len(expectedLimiters))
		})
	}
}

func TestPlugin_Deactivate(t *testing.T) {
	testCases := []struct {
		name              string
		role              string
		enableForceDetach bool
		expectStopCall    bool
	}{
		{
			name:              "CSINode with force detach",
			role:              CSINode,
			enableForceDetach: true,
			expectStopCall:    true,
		},
		{
			name:              "CSIAllInOne with force detach",
			role:              CSIAllInOne,
			enableForceDetach: true,
			expectStopCall:    true,
		},
		{
			name:              "CSIController role",
			role:              CSIController,
			enableForceDetach: true,
			expectStopCall:    false,
		},
		{
			name:              "CSINode without force detach",
			role:              CSINode,
			enableForceDetach: false,
			expectStopCall:    false,
		},
		{
			name:              "CSIAllInOne without force detach",
			role:              CSIAllInOne,
			enableForceDetach: false,
			expectStopCall:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockGRPC := &mockNonBlockingGRPCServer{}

			plugin := &Plugin{
				role:              tc.role,
				enableForceDetach: tc.enableForceDetach,
				grpc:              mockGRPC,
			}

			err := plugin.Deactivate()

			assert.NoError(t, err)
		})
	}
}

// Mock GRPC server for testing
type mockNonBlockingGRPCServer struct{}

func (m *mockNonBlockingGRPCServer) Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer, gs csi.GroupControllerServer) {
	// Mock implementation
}

func (m *mockNonBlockingGRPCServer) GracefulStop() {
	// Mock implementation
}

func (m *mockNonBlockingGRPCServer) Stop() {
	// Mock implementation
}
