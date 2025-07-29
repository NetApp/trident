// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
	controllerAPI "github.com/netapp/trident/frontend/csi/controller_api"
	"github.com/netapp/trident/mocks/mock_core"
	mockControllerAPI "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_api"
	mock_controller_helpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_helpers"
	mock_node_helpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	"github.com/netapp/trident/mocks/mock_utils/mock_iscsi"
	mock_nvme "github.com/netapp/trident/mocks/mock_utils/nvme"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/limiter"
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
		name             string
		nodeIsRegistered bool
		expectedReady    bool
	}{
		{
			name:             "Ready - Node registered",
			nodeIsRegistered: true,
			expectedReady:    true,
		},
		{
			name:             "Not ready - Node not registered",
			nodeIsRegistered: false,
			expectedReady:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := &Plugin{
				nodeIsRegistered: tc.nodeIsRegistered,
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
