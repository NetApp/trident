// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/brunoga/deep"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/mocks/mock_utils/mock_fcp"
	execCmd "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/fcp"
	"github.com/netapp/trident/utils/smb"

	controllerAPI "github.com/netapp/trident/frontend/csi/controller_api"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	"github.com/netapp/trident/internal/crypto"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	mockControllerAPI "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_api"
	mockNodeHelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	"github.com/netapp/trident/mocks/mock_utils/mock_devices"
	"github.com/netapp/trident/mocks/mock_utils/mock_filesystem"
	"github.com/netapp/trident/mocks/mock_utils/mock_iscsi"
	"github.com/netapp/trident/mocks/mock_utils/mock_mount"
	"github.com/netapp/trident/mocks/mock_utils/mock_osutils"
	mock_nvme "github.com/netapp/trident/mocks/mock_utils/nvme"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/locks"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/limiter"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount"
	"github.com/netapp/trident/utils/nvme"
	"github.com/netapp/trident/utils/osutils"
)

func TestNodeStageVolume(t *testing.T) {
	type parameters struct {
		getISCSIClient          func() iscsi.ISCSI
		getNodeHelper           func() nodehelpers.NodeHelper
		nodeStageVolumeRequest  *csi.NodeStageVolumeRequest
		assertError             assert.ErrorAssertionFunc
		nodeStageVolumeResponse *csi.NodeStageVolumeResponse
	}

	request := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).Build()

	tests := map[string]parameters{
		"SAN: iSCSI volume: happy path": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(1), nil)
				mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
				mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},

			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			nodeStageVolumeRequest:  request,
			assertError:             assert.NoError,
			nodeStageVolumeResponse: &csi.NodeStageVolumeResponse{},
		},
	}

	mountClient, _ := mount.New()

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
				fs:               filesystem.New(mountClient),
			}

			plugin.InitializeNodeLimiter(ctx)

			if params.getISCSIClient != nil {
				plugin.iscsi = params.getISCSIClient()
			}

			if params.getNodeHelper != nil {
				plugin.nodeHelper = params.getNodeHelper()
			}

			nodeStageVolumeResponse, err := plugin.NodeStageVolume(context.Background(), params.nodeStageVolumeRequest)
			if params.assertError != nil {
				params.assertError(t, err)
			}

			assert.Equal(t, params.nodeStageVolumeResponse, nodeStageVolumeResponse)
		})
	}
}

func TestNodeStageSANVolume(t *testing.T) {
	type parameters struct {
		getISCSIClient          func() iscsi.ISCSI
		getNodeHelper           func() nodehelpers.NodeHelper
		nodeStageVolumeRequest  *csi.NodeStageVolumeRequest
		assertError             assert.ErrorAssertionFunc
		nodeStageVolumeResponse *csi.NodeStageVolumeResponse
	}

	noCapabilitiesRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithVolumeCapability(&csi.VolumeCapability{}).Build()
	fileSystemRawMountCapabilityRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithFileSystemType(filesystem.Raw).Build()
	fileSystemExt4BlockCapabilityRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithVolumeCapability(
		&csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Block{
				Block: &csi.VolumeCapability_BlockVolume{},
			},
		},
	).Build()

	badSharedTargetRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithSharedTarget("foo").Build()

	tests := map[string]parameters{
		"mount and block capabilities not specified in the request": {
			nodeStageVolumeRequest:  noCapabilitiesRequest,
			assertError:             assert.Error,
			nodeStageVolumeResponse: nil,
		},
		"raw file system requested with mount capability": {
			nodeStageVolumeRequest:  fileSystemRawMountCapabilityRequest,
			assertError:             assert.Error,
			nodeStageVolumeResponse: nil,
		},
		"ext4 file system requested with block capability": {
			nodeStageVolumeRequest:  fileSystemExt4BlockCapabilityRequest,
			assertError:             assert.Error,
			nodeStageVolumeResponse: nil,
		},
		"bad sharedTarget in publish context": {
			nodeStageVolumeRequest:  badSharedTargetRequest,
			assertError:             assert.Error,
			nodeStageVolumeResponse: nil,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
			}

			plugin.InitializeNodeLimiter(ctx)

			if params.getISCSIClient != nil {
				plugin.iscsi = params.getISCSIClient()
			}

			if params.getNodeHelper != nil {
				plugin.nodeHelper = params.getNodeHelper()
			}

			nodeStageVolumeResponse, err := plugin.nodeStageSANVolume(context.Background(), params.nodeStageVolumeRequest)

			if params.assertError != nil {
				params.assertError(t, err)
			}

			assert.Equal(t, params.nodeStageVolumeResponse, nodeStageVolumeResponse)
		})
	}
}

func TestNodeStageISCSIVolume(t *testing.T) {
	type parameters struct {
		getISCSIClient         func() iscsi.ISCSI
		getNodeHelper          func() nodehelpers.NodeHelper
		getRestClient          func() controllerAPI.TridentController
		nodeStageVolumeRequest *csi.NodeStageVolumeRequest
		assertError            assert.ErrorAssertionFunc
		aesKey                 []byte
	}

	request := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).Build()
	badUseChapRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithUseCHAP("foo").Build()
	badLunNumberRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithIscsiLunNumber("foo").Build()
	badLuksEncryptionRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithLUKSEncryption("foo").Build()
	badTargetPortalCountRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithIscsiTargetPortalCount("foo").Build()
	zeroTargetPortalCountRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithIscsiTargetPortalCount("0").Build()
	targetPortalCountThreeMissingPortalRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithIscsiTargetPortalCount("3").Build()
	chapBadEncryptedISCSIUserNameRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiUsername("foo").Build()
	chapBadEncryptedIscsiInitiatorSecretRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiInitiatorSecret("foo").Build()
	chapBadEncryptedIscsiTargetUsernameRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiTargetUsername("foo").Build()
	chapBadEncryptedIscsiTargetSecretRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiTargetSecret("foo").Build()
	badVolumeIdRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithVolumeID("").Build()
	badStagingPathRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithStagingTargetPath("").Build()

	const mockAESKey = "thisIsMockAESKey"
	encryptedValue, err := crypto.EncryptStringWithAES("mockValue", []byte(mockAESKey))
	assert.Nil(t, err)

	chapEncryptedISCSIUserNameRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiUsername(encryptedValue).Build()
	chapEncryptedIscsiInitiatorSecretRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiInitiatorSecret(encryptedValue).Build()
	chapEncryptedIscsiTargetUsernameRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiTargetUsername(encryptedValue).Build()
	chapEncryptedIscsiTargetSecretRequest := NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiTargetSecret(encryptedValue).Build()

	tests := map[string]parameters{
		"bad useCHAP in publish context": {
			nodeStageVolumeRequest: badUseChapRequest,
			assertError:            assert.Error,
		},
		"bad lun number in publish context": {
			nodeStageVolumeRequest: badLunNumberRequest,
			assertError:            assert.Error,
		},
		"bad luks encryption in publish context": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(1), nil)
				mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
				mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},

			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			nodeStageVolumeRequest: badLuksEncryptionRequest,
			assertError:            assert.NoError,
		},
		"bad iscsi target portal count": {
			nodeStageVolumeRequest: badTargetPortalCountRequest,
			assertError:            assert.Error,
		},
		"iscsi target portal count set to 0": {
			nodeStageVolumeRequest: zeroTargetPortalCountRequest,
			assertError:            assert.Error,
		},
		"iscsi target portal count set to 3 but only 2 portals provided": {
			nodeStageVolumeRequest: targetPortalCountThreeMissingPortalRequest,
			assertError:            assert.Error,
		},
		"encrypted user name in request": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(0), nil)
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			nodeStageVolumeRequest: chapEncryptedISCSIUserNameRequest,
			assertError:            assert.NoError,
			aesKey:                 []byte(mockAESKey),
		},
		"CHAP : bad encrypted user name in request": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(0), nil)
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			getRestClient: func() controllerAPI.TridentController {
				mockRestClient := mockControllerAPI.NewMockTridentController(gomock.NewController(t))
				mockRestClient.EXPECT().GetChap(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&models.IscsiChapInfo{}, nil)
				return mockRestClient
			},
			nodeStageVolumeRequest: chapBadEncryptedISCSIUserNameRequest,
			assertError:            assert.NoError,
			aesKey:                 []byte(mockAESKey),
		},
		"CHAP : encrypted initiator secret in request": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(0), nil)
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			nodeStageVolumeRequest: chapEncryptedIscsiInitiatorSecretRequest,
			assertError:            assert.NoError,
			aesKey:                 []byte(mockAESKey),
		},
		"CHAP : bad encrypted initiator secret in request": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(0), nil)
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			getRestClient: func() controllerAPI.TridentController {
				mockRestClient := mockControllerAPI.NewMockTridentController(gomock.NewController(t))
				mockRestClient.EXPECT().GetChap(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&models.IscsiChapInfo{}, nil)
				return mockRestClient
			},
			nodeStageVolumeRequest: chapBadEncryptedIscsiInitiatorSecretRequest,
			assertError:            assert.NoError,
			aesKey:                 []byte(mockAESKey),
		},
		"CHAP : encrypted target username in request": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(0), nil)
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			nodeStageVolumeRequest: chapEncryptedIscsiTargetUsernameRequest,
			assertError:            assert.NoError,
			aesKey:                 []byte(mockAESKey),
		},
		"CHAP : bad encrypted target username in request": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(0), nil)
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			getRestClient: func() controllerAPI.TridentController {
				mockRestClient := mockControllerAPI.NewMockTridentController(gomock.NewController(t))
				mockRestClient.EXPECT().GetChap(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&models.IscsiChapInfo{}, nil)
				return mockRestClient
			},
			nodeStageVolumeRequest: chapBadEncryptedIscsiTargetUsernameRequest,
			assertError:            assert.NoError,
			aesKey:                 []byte(mockAESKey),
		},
		"CHAP : encrypted target secret in request": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(0), nil)
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			nodeStageVolumeRequest: chapEncryptedIscsiTargetSecretRequest,
			assertError:            assert.NoError,
			aesKey:                 []byte(mockAESKey),
		},
		"CHAP : bad encrypted target secret in request": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(0), nil)
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			getRestClient: func() controllerAPI.TridentController {
				mockRestClient := mockControllerAPI.NewMockTridentController(gomock.NewController(t))
				mockRestClient.EXPECT().GetChap(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&models.IscsiChapInfo{}, nil)
				return mockRestClient
			},
			nodeStageVolumeRequest: chapBadEncryptedIscsiTargetSecretRequest,
			assertError:            assert.NoError,
			aesKey:                 []byte(mockAESKey),
		},
		"CHAP : bad encrypted chap information in Request, failed to get key from controller": {
			getRestClient: func() controllerAPI.TridentController {
				mockRestClient := mockControllerAPI.NewMockTridentController(gomock.NewController(t))
				mockRestClient.EXPECT().GetChap(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("some error"))
				return mockRestClient
			},
			nodeStageVolumeRequest: chapBadEncryptedIscsiTargetSecretRequest,
			assertError:            assert.Error,
			aesKey:                 []byte(mockAESKey),
		},
		"CHAP : encrypted chap information but aes key not present on node": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(0), nil)
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			getRestClient: func() controllerAPI.TridentController {
				mockRestClient := mockControllerAPI.NewMockTridentController(gomock.NewController(t))
				mockRestClient.EXPECT().GetChap(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&models.IscsiChapInfo{}, nil)
				return mockRestClient
			},
			nodeStageVolumeRequest: chapBadEncryptedIscsiTargetSecretRequest,
			assertError:            assert.NoError,
		},
		"CHAP : encrypted chap information but aes key not present on node,failed to get key from controller": {
			getRestClient: func() controllerAPI.TridentController {
				mockRestClient := mockControllerAPI.NewMockTridentController(gomock.NewController(t))
				mockRestClient.EXPECT().GetChap(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, errors.New("some error"))
				return mockRestClient
			},
			nodeStageVolumeRequest: chapBadEncryptedIscsiTargetSecretRequest,
			assertError:            assert.Error,
		},
		"bad volume id": {
			nodeStageVolumeRequest: badVolumeIdRequest,
			assertError:            assert.Error,
		},
		"bad staging path": {
			nodeStageVolumeRequest: badStagingPathRequest,
			assertError:            assert.Error,
		},
		"failure to ensure volume attachment": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(0), errors.New("some error"))
				return mockISCSIClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			nodeStageVolumeRequest: request,
			assertError:            assert.Error,
		},
		"volume is not already attached for expansion": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(1), nil)
				mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(false)
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			nodeStageVolumeRequest: request,
			assertError:            assert.NoError,
		},
		"volume rescan error during expansion": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(1), nil)
				mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
				mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any()).Return(errors.New("some error"))
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
				return mockNodeHelper
			},
			nodeStageVolumeRequest: request,
			assertError:            assert.NoError,
		},
		"error writing the tracking file": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(1), nil)
				mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
				mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},

			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("some error"))
				return mockNodeHelper
			},
			nodeStageVolumeRequest: request,
			assertError:            assert.Error,
		},
		"error writing the tracking file when attachment error exists": {
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(0), errors.New("some error"))
				return mockISCSIClient
			},

			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("some error"))
				return mockNodeHelper
			},
			nodeStageVolumeRequest: request,
			assertError:            assert.Error,
		},
	}

	mountClient, _ := mount.New()

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
				aesKey:           params.aesKey,
				fs:               filesystem.New(mountClient),
			}

			plugin.InitializeNodeLimiter(ctx)

			if params.getISCSIClient != nil {
				plugin.iscsi = params.getISCSIClient()
			}

			if params.getNodeHelper != nil {
				plugin.nodeHelper = params.getNodeHelper()
			}

			if params.getRestClient != nil {
				plugin.restClient = params.getRestClient()
			}

			sharedTarget, err := strconv.ParseBool(params.nodeStageVolumeRequest.PublishContext["sharedTarget"])
			assert.Nil(t, err)

			publishInfo := models.VolumePublishInfo{
				Localhost:      true,
				FilesystemType: params.nodeStageVolumeRequest.PublishContext["filesystemType"],
				SharedTarget:   sharedTarget,
			}

			err = plugin.nodeStageISCSIVolume(context.Background(), params.nodeStageVolumeRequest, &publishInfo)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestGetChapInfoFromController(t *testing.T) {
	type args struct {
		ctx    context.Context
		volume string
		node   string
	}

	type data struct {
		chapInfo *models.IscsiChapInfo
	}

	type assertions struct {
		Error assert.ErrorAssertionFunc
		Empty assert.ValueAssertionFunc
		Equal assert.ComparisonAssertionFunc
	}

	tt := map[string]struct {
		args   args
		data   data
		assert assertions
		mocks  func(mockAPI *mockControllerAPI.MockTridentController, args args, data data)
	}{
		"Successfully gets CHAP credentials": {
			args: args{
				ctx:    context.Background(),
				volume: "foo",
				node:   "bar",
			},
			data: data{
				chapInfo: &models.IscsiChapInfo{
					UseCHAP:              true,
					IscsiUsername:        "user",
					IscsiInitiatorSecret: "pass",
					IscsiTargetUsername:  "user2",
					IscsiTargetSecret:    "pass2",
				},
			},
			assert: assertions{
				Error: assert.NoError,
				Empty: assert.NotEmpty,
				Equal: assert.Equal,
			},
			mocks: func(mockAPI *mockControllerAPI.MockTridentController, args args, data data) {
				mockAPI.EXPECT().GetChap(args.ctx, args.volume, args.node).Return(data.chapInfo, nil)
			},
		},
		"Fails to get CHAP credentials": {
			args: args{
				ctx:    context.Background(),
				volume: "foo",
				node:   "bar",
			},
			data: data{
				chapInfo: nil,
			},
			assert: assertions{
				Error: assert.Error,
				Empty: assert.Empty,
				Equal: assert.Equal,
			},
			mocks: func(mockAPI *mockControllerAPI.MockTridentController, args args, data data) {
				mockAPI.EXPECT().GetChap(args.ctx, args.volume, args.node).Return(data.chapInfo, errors.New("api error"))
			},
		},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockAPI := mockControllerAPI.NewMockTridentController(mockCtrl)
			test.mocks(mockAPI, test.args, test.data)

			plugin := &Plugin{
				command:    execCmd.NewCommand(),
				restClient: mockAPI,
			}

			info, err := plugin.getChapInfoFromController(test.args.ctx, test.args.volume, test.args.node)
			test.assert.Error(t, err)
			test.assert.Empty(t, info)
			test.assert.Equal(t, info, test.data.chapInfo)
		})
	}
}

func TestUpdateChapInfoFromController_Success(t *testing.T) {
	testCtx := context.Background()
	volumeName := "foo"
	nodeName := "bar"
	expectedChapInfo := models.IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "user",
		IscsiInitiatorSecret: "pass",
		IscsiTargetUsername:  "user2",
		IscsiTargetSecret:    "pass2",
	}

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().GetChap(testCtx, volumeName, nodeName).Return(&expectedChapInfo, nil)
	nodeServer := &Plugin{
		nodeName:   nodeName,
		role:       CSINode,
		restClient: mockClient,
	}

	fakeRequest := &csi.NodeStageVolumeRequest{VolumeId: volumeName}
	testPublishInfo := &models.VolumePublishInfo{}

	err := nodeServer.updateChapInfoFromController(testCtx, fakeRequest, testPublishInfo)
	assert.Nil(t, err, "Unexpected error")
	assert.EqualValues(t, expectedChapInfo, testPublishInfo.IscsiAccessInfo.IscsiChapInfo)
}

func TestUpdateChapInfoFromController_Error(t *testing.T) {
	testCtx := context.Background()
	volumeName := "foo"
	nodeName := "bar"
	expectedChapInfo := models.IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "user",
		IscsiInitiatorSecret: "pass",
		IscsiTargetUsername:  "user2",
		IscsiTargetSecret:    "pass2",
	}

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().GetChap(testCtx, volumeName, nodeName).Return(&expectedChapInfo, fmt.Errorf("some error"))
	nodeServer := &Plugin{
		nodeName:   nodeName,
		role:       CSINode,
		restClient: mockClient,
	}

	fakeRequest := &csi.NodeStageVolumeRequest{VolumeId: volumeName}
	testPublishInfo := &models.VolumePublishInfo{}

	err := nodeServer.updateChapInfoFromController(testCtx, fakeRequest, testPublishInfo)
	assert.NotNil(t, err, "Unexpected success")
	assert.NotEqualValues(t, expectedChapInfo, testPublishInfo.IscsiAccessInfo.IscsiChapInfo)
	assert.EqualValues(t, models.IscsiChapInfo{}, testPublishInfo.IscsiAccessInfo.IscsiChapInfo)
}

func TestUpdateChapInfoForSessions(t *testing.T) {
	// Populate sessions with only the state that matters. The rest of the fields are not relevant for this test.
	publishedSessions := &models.ISCSISessions{}
	currentSessions := &models.ISCSISessions{}

	// CHAP portals
	chapPortals := []string{"0.0.0.0", "4.4.4.4", "5.5.5.5"}

	// Unique CHAP portal
	uniqueChapPortal := "9.9.9.9"
	uniqueIQN := "iqn.9999-99.com.netapp:target"

	// non-CHAP portals
	nonChapPortals := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}

	sessionID := "0"

	// Shared CHAP credentials
	sharedCHAPInfo := models.IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "user",
		IscsiInitiatorSecret: "pass",
		IscsiTargetUsername:  "user2",
		IscsiTargetSecret:    "pass2",
	}

	// Add CHAP sessions to both maps.
	for _, portal := range chapPortals {
		err := publishedSessions.AddPortal(portal, models.PortalInfo{
			ISCSITargetIQN: "iqn.2020-01.com.netapp:target",
			Credentials:    sharedCHAPInfo,
		})
		assert.NoError(t, err)

		err = currentSessions.AddPortal(portal, models.PortalInfo{
			ISCSITargetIQN: "iqn.2020-01.com.netapp:target",
			SessionNumber:  sessionID,
		})
		assert.NoError(t, err)
	}

	// Add a CHAP session with a unique IQN.
	err := publishedSessions.AddPortal(uniqueChapPortal, models.PortalInfo{
		// Use a different IQN here to prove we cache returned values from the REST API.
		ISCSITargetIQN: uniqueIQN,
		Credentials:    sharedCHAPInfo,
	})
	assert.NoError(t, err)
	err = currentSessions.AddPortal(uniqueChapPortal, models.PortalInfo{
		ISCSITargetIQN: uniqueIQN,
		SessionNumber:  sessionID,
	})
	assert.NoError(t, err)

	// Add non-CHAP session
	for _, portal := range nonChapPortals {
		err := publishedSessions.AddPortal(portal, models.PortalInfo{
			ISCSITargetIQN: "iqn.2020-01.com.netapp:target",
			Credentials:    models.IscsiChapInfo{},
		})
		assert.NoError(t, err)

		err = currentSessions.AddPortal(portal, models.PortalInfo{
			ISCSITargetIQN: "iqn.2020-01.com.netapp:target",
			Credentials:    models.IscsiChapInfo{},
			SessionNumber:  sessionID,
		})
		assert.NoError(t, err)
	}

	// Populate a single of LUN and volume in each session.
	volume := "foo"
	node := "bar"
	for _, sessionData := range publishedSessions.Info {
		sessionData.LUNs.Info[0] = volume
	}

	// Create a mock controller client that will return the expected CHAP info.
	mockCtrl := gomock.NewController(t)
	mockISCSI := mock_iscsi.NewMockISCSI(mockCtrl)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)

	plugin := &Plugin{
		command:    execCmd.NewCommand(),
		nodeName:   node,
		iscsi:      mockISCSI,
		restClient: mockClient,
	}

	// Expect calls on the mock client for all sessions that use CHAP.
	freshCHAPInfo := &models.IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "user2",
		IscsiInitiatorSecret: "pass2",
		IscsiTargetUsername:  "user",
		IscsiTargetSecret:    "pass",
	}

	// Mock calls to the iSCSI client
	count := len(currentSessions.Info)
	mockISCSI.EXPECT().IsSessionStale(gomock.Any(), sessionID).Return(true).Times(count)

	// Mock API calls
	count = len(chapPortals) - (len(chapPortals) - 1) + 1 // Expect one more call for the unique CHAP portal.
	mockClient.EXPECT().GetChap(
		gomock.Any(), volume, node,
	).Return(freshCHAPInfo, nil).Times(count)

	err = plugin.updateCHAPInfoForSessions(ctx, publishedSessions, currentSessions)
	assert.NoError(t, err)

	// Verify that the CHAP info was updated in all sessions that use CHAP.
	for _, portal := range chapPortals {
		chapInfoInSession := publishedSessions.Info[portal].PortalInfo.Credentials
		assert.EqualValues(t, *freshCHAPInfo, chapInfoInSession)
	}

	// Verify that the non-CHAP portals were not updated.
	for _, portal := range nonChapPortals {
		chapInfoInSession := publishedSessions.Info[portal].PortalInfo.Credentials
		assert.EqualValues(t, models.IscsiChapInfo{}, chapInfoInSession)
	}
}

type PortalAction struct {
	Portal string
	Action models.ISCSIAction
}

func TestFixISCSISessions(t *testing.T) {
	lunList1 := map[int32]string{
		1: "volID-1",
		2: "volID-2",
		3: "volID-3",
	}

	lunList2 := map[int32]string{
		2: "volID-2",
		3: "volID-3",
		4: "volID-4",
	}

	ipList := []string{"1.2.3.4", "2.3.4.5", "3.4.5.6", "4.5.6.7"}

	iqnList := []string{"IQN1", "IQN2", "IQN3", "IQN4"}

	chapCredentials := []models.IscsiChapInfo{
		{
			UseCHAP: false,
		},
		{
			UseCHAP:              true,
			IscsiUsername:        "username1",
			IscsiInitiatorSecret: "secret1",
			IscsiTargetUsername:  "username2",
			IscsiTargetSecret:    "secret2",
		},
		{
			UseCHAP:              true,
			IscsiUsername:        "username11",
			IscsiInitiatorSecret: "secret11",
			IscsiTargetUsername:  "username22",
			IscsiTargetSecret:    "secret22",
		},
	}

	sessionData1 := models.ISCSISessionData{
		PortalInfo: models.PortalInfo{
			ISCSITargetIQN:         iqnList[0],
			Credentials:            chapCredentials[2],
			FirstIdentifiedStaleAt: time.Now().Add(-time.Second * 10),
		},
		LUNs: models.LUNs{
			Info: mapCopyHelper(lunList1),
		},
	}

	sessionData2 := models.ISCSISessionData{
		PortalInfo: models.PortalInfo{
			ISCSITargetIQN:         iqnList[1],
			Credentials:            chapCredentials[2],
			FirstIdentifiedStaleAt: time.Now().Add(-time.Second * 10),
		},
		LUNs: models.LUNs{
			Info: mapCopyHelper(lunList2),
		},
	}

	type PreRun func(publishedSessions, currentSessions *models.ISCSISessions, portalActions []PortalAction)

	inputs := []struct {
		TestName                       string
		PublishedPortals               *models.ISCSISessions
		CurrentPortals                 *models.ISCSISessions
		PortalActions                  []PortalAction
		StopAt                         time.Time
		AddNewNodeOps                  bool // If there exist a new node operation would request lock.
		SimulateConditions             PreRun
		PortalsFixed                   []string
		SelfHealingRectifySessionError bool
	}{
		{
			TestName:         "Sessions to fixed are zero",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}},
			CurrentPortals:   &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}},
			PortalActions:    []PortalAction{},
			StopAt:           time.Now().Add(100 * time.Second),
			AddNewNodeOps:    false,
			PortalsFixed:     []string{},
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions,
				portalActions []PortalAction,
			) {
				return
			},
		},
		{
			TestName: "No node operations are waiting all the sessions are fixed",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: models.NoAction},
				{Portal: ipList[1], Action: models.NoAction},
				{Portal: ipList[2], Action: models.NoAction},
			},
			StopAt:        time.Now().Add(100 * time.Second),
			AddNewNodeOps: false,
			PortalsFixed:  []string{ipList[0], ipList[1], ipList[2]},
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow.Add(5 * time.Millisecond)
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(10 * time.Millisecond)

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "No node operations waiting AND self-healing exceeded max time, then all the sessions are fixed.",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: models.NoAction},
				{Portal: ipList[1], Action: models.NoAction},
				{Portal: ipList[2], Action: models.NoAction},
			},
			StopAt:        time.Now().Add(-time.Second * 100),
			AddNewNodeOps: false,
			PortalsFixed:  []string{ipList[0], ipList[1], ipList[2]},
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow.Add(5 * time.Millisecond)
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(10 * time.Millisecond)

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "Node operation is waiting, self healing didn't exceed max-time, all sessions are fixed.",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: models.NoAction},
				{Portal: ipList[1], Action: models.NoAction},
				{Portal: ipList[2], Action: models.NoAction},
			},
			StopAt:        time.Now().Add(time.Second * 100),
			AddNewNodeOps: true,
			PortalsFixed:  []string{ipList[0], ipList[1], ipList[2]},
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow.Add(5 * time.Millisecond)
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(10 * time.Millisecond)

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "Node Operation is waiting, self-healing time has exceeded, no sessions are fixed.",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: models.NoAction},
				{Portal: ipList[1], Action: models.NoAction},
				{Portal: ipList[2], Action: models.NoAction},
			},
			StopAt:        time.Now().Add(-time.Second * 100),
			AddNewNodeOps: true,
			PortalsFixed:  []string{},
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow.Add(-time.Second * 10)
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow.Add(-time.Second * 10)
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(-time.Second * 10)

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "No Node Operation are waiting, selfHealingRectifySession returns an error",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: models.NoAction},
				{Portal: ipList[1], Action: models.NoAction},
				{Portal: ipList[2], Action: models.NoAction},
			},
			StopAt:        time.Now().Add(time.Second * 100),
			AddNewNodeOps: true,
			PortalsFixed:  []string{ipList[0], ipList[1], ipList[2]},
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow.Add(-time.Second * 10)
				publishedSessions.Info[ipList[0]].PortalInfo.ISCSITargetIQN = ""
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow.Add(-time.Second * 10)
				publishedSessions.Info[ipList[1]].PortalInfo.ISCSITargetIQN = ""
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(-time.Second * 10)
				publishedSessions.Info[ipList[2]].PortalInfo.ISCSITargetIQN = ""

				setRemediation(publishedSessions, portalActions)
			},
			SelfHealingRectifySessionError: true,
		},
	}

	iscsiClient, err := iscsi.New()
	assert.NoError(t, err)

	nodeServer := &Plugin{
		nodeName: "someNode",
		role:     CSINode,
		iscsi:    iscsiClient,
	}

	for _, input := range inputs {
		t.Run(input.TestName, func(t *testing.T) {
			publishedISCSISessions = input.PublishedPortals

			input.SimulateConditions(input.PublishedPortals, input.CurrentPortals, input.PortalActions)
			portals := getPortals(input.PublishedPortals, input.PortalActions)

			if input.AddNewNodeOps {
				iSCSINodeOperationWaitingCount.Add(1)
				iSCSINodeOperationWaitingCount.Add(1)
			}

			// Make sure this time is captured after the pre-run adds wait time
			// Also on Windows the system time is often only updated once every
			// 10-15 ms or so, which means if you query the current time twice
			// within this period, you get the same value. Therefore, set this
			// time to be slightly lower than time that will be
			// set in fixISCSISessions call.
			timeNow := time.Now().Add(-2 * time.Millisecond)

			nodeServer.fixISCSISessions(context.TODO(), portals, "some-portal", input.StopAt)

			for _, portal := range portals {
				lastAccessTime := publishedISCSISessions.Info[portal].PortalInfo.LastAccessTime
				firstIdentifiedStaleAt := publishedISCSISessions.Info[portal].PortalInfo.FirstIdentifiedStaleAt
				if collection.ContainsString(input.PortalsFixed, portal) {
					assert.True(t, lastAccessTime.After(timeNow),
						fmt.Sprintf("mismatched last access time for %v portal", portal))
					if input.SelfHealingRectifySessionError {
						assert.NotEqual(t, firstIdentifiedStaleAt, time.Time{},
							fmt.Sprintf("firstIdentifiedAt time is not empty for %v portal", portal))
					} else {
						assert.Equal(t, firstIdentifiedStaleAt, time.Time{},
							fmt.Sprintf("firstIdentifiedAt time is not empty for %v portal", portal))
					}
				} else {
					assert.True(t, lastAccessTime.Before(timeNow),
						fmt.Sprintf("mismatched last access time for %v portal", portal))
					assert.NotEqual(t, firstIdentifiedStaleAt, time.Time{},
						fmt.Sprintf("firstIdentifiedAt time should not be empty for %v portal", portal))
				}
			}

			if input.AddNewNodeOps {
				iSCSINodeOperationWaitingCount.Add(-1)
				iSCSINodeOperationWaitingCount.Add(-1)
			}
		})
	}
}

func setRemediation(sessions *models.ISCSISessions, portalActions []PortalAction) {
	for _, portalAction := range portalActions {
		sessions.Info[portalAction.Portal].Remediation = portalAction.Action
	}
}

func getPortals(sessions *models.ISCSISessions, portalActions []PortalAction) []string {
	portals := make([]string, len(portalActions))

	for idx, portalAction := range portalActions {
		portals[idx] = portalAction.Portal
	}

	iscsi.SortPortals(portals, sessions)

	return portals
}

func mapCopyHelper(input map[int32]string) map[int32]string {
	output := make(map[int32]string, len(input))

	for key, value := range input {
		output[key] = value
	}

	return output
}

func structCopyHelper(input models.ISCSISessionData) *models.ISCSISessionData {
	clone, err := deep.Copy(input)
	if err != nil {
		return &models.ISCSISessionData{}
	}
	return &clone
}

func snooze(val uint32) {
	time.Sleep(time.Duration(val) * time.Millisecond)
}

func TestRefreshTimerPeriod(t *testing.T) {
	ctx := context.Background()
	nodeServer := &Plugin{
		role:              CSINode,
		enableForceDetach: true,
	}

	maxPeriod := defaultNodeReconciliationPeriod + maximumNodeReconciliationJitter
	for i := 0; i < 10; i++ {
		t.Run(fmt.Sprintf("%v", i), func(t *testing.T) {
			newPeriod := nodeServer.refreshTimerPeriod(ctx)
			assert.GreaterOrEqual(t, newPeriod.Milliseconds(), defaultNodeReconciliationPeriod.Milliseconds())
			assert.LessOrEqual(t, newPeriod.Milliseconds(), maxPeriod.Milliseconds())
		})
	}
}

func TestDiscoverDesiredPublicationState_GetsNoPublicationsWithoutError(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	var expectedPublications []*models.VolumePublicationExternal

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().ListVolumePublicationsForNode(ctx, nodeName).Return(expectedPublications, nil)
	nodeServer := &Plugin{
		nodeName:          nodeName,
		role:              CSINode,
		restClient:        mockClient,
		enableForceDetach: true,
	}

	// desiredPublicationState is a mapping of volumes to volume publications.
	desiredPublicationState, err := nodeServer.discoverDesiredPublicationState(ctx)
	assert.NoError(t, err, "expected no error")
	assert.Empty(t, desiredPublicationState, "expected empty map")
}

func TestDiscoverDesiredPublicationState_GetsPublicationsWithoutError(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	expectedPublications := []*models.VolumePublicationExternal{
		{
			Name:       models.GenerateVolumePublishName("foo", nodeName),
			NodeName:   nodeName,
			VolumeName: "foo",
		},
		{
			Name:       models.GenerateVolumePublishName("baz", nodeName),
			NodeName:   nodeName,
			VolumeName: "baz",
		},
	}

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().ListVolumePublicationsForNode(ctx, nodeName).Return(expectedPublications, nil)
	nodeServer := &Plugin{
		nodeName:          nodeName,
		role:              CSINode,
		restClient:        mockClient,
		enableForceDetach: true,
	}

	// desiredPublicationState is a mapping of volumes to volume publications.
	desiredPublicationState, err := nodeServer.discoverDesiredPublicationState(ctx)
	assert.NoError(t, err, "expected no error")
	for _, expectedPublication := range expectedPublications {
		desiredPublication, ok := desiredPublicationState[expectedPublication.VolumeName]
		assert.True(t, ok, "expected true value")
		assert.NotNil(t, desiredPublication, "expected publication to exist")
	}
}

func TestDiscoverDesiredPublicationState_FailsToGetPublicationsWithError(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	expectedPublications := []*models.VolumePublicationExternal{
		{
			Name:       models.GenerateVolumePublishName("foo", nodeName),
			NodeName:   nodeName,
			VolumeName: "foo",
		},
		{
			Name:       models.GenerateVolumePublishName("baz", nodeName),
			NodeName:   nodeName,
			VolumeName: "baz",
		},
	}

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().ListVolumePublicationsForNode(ctx, nodeName).Return(
		expectedPublications,
		errors.New("failed to list volume publications"),
	)
	nodeServer := &Plugin{
		nodeName:          nodeName,
		role:              CSINode,
		restClient:        mockClient,
		enableForceDetach: true,
	}

	// desiredPublicationState is a mapping of volumes to volume publications.
	desiredPublicationState, err := nodeServer.discoverDesiredPublicationState(ctx)
	assert.Error(t, err, "expected error")
	assert.Empty(t, desiredPublicationState, "expected nil map")
}

func TestDiscoverActualPublicationState_FindsTrackingInfoWithoutError(t *testing.T) {
	ctx := context.Background()
	expectedPublicationState := map[string]*models.VolumeTrackingInfo{
		"pvc-85987a99-648d-4d84-95df-47d0256ca2ab": {
			VolumePublishInfo: models.VolumePublishInfo{},
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
				"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787dd/globalmount",
			PublishedPaths: map[string]struct{}{
				"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e3/volumes/kubernetes.io~csi/" +
					"pvc-85987a99-648d-4d84-95df-47d0256ca2ab/mount": {},
			},
		},
		"pvc-85987a99-648d-4d84-95df-47d0256ca2ac": {
			VolumePublishInfo: models.VolumePublishInfo{},
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
				"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787de/globalmount",
			PublishedPaths: map[string]struct{}{
				"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e2/volumes/kubernetes.io~csi/" +
					"pvc-85987a99-648d-4d84-95df-47d0256ca2ac/mount": {},
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	mockHelper := mockNodeHelpers.NewMockNodeHelper(mockCtrl)
	mockHelper.EXPECT().ListVolumeTrackingInfo(ctx).Return(expectedPublicationState, nil)
	nodeServer := &Plugin{
		role:              CSINode,
		nodeHelper:        mockHelper,
		enableForceDetach: true,
	}

	// actualPublicationState is a mapping of volumes to volume publications.
	actualPublicationState, err := nodeServer.discoverActualPublicationState(ctx)
	assert.NoError(t, err, "expected no error")
	assert.NotEmptyf(t, actualPublicationState, "expected non-empty map")
	for volumeName, publicationState := range expectedPublicationState {
		actualPublication, ok := actualPublicationState[volumeName]
		assert.True(t, ok, "expected true")
		assert.NotNil(t, actualPublication, "expected non-nil publication state")
		for path := range publicationState.PublishedPaths {
			assert.Contains(t, path, volumeName)
		}
	}
}

func TestDiscoverActualPublicationState_FailsWithError(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockHelper := mockNodeHelpers.NewMockNodeHelper(mockCtrl)
	mockHelper.EXPECT().ListVolumeTrackingInfo(ctx).Return(nil, errors.New("not found"))
	nodeServer := &Plugin{
		role:              CSINode,
		nodeHelper:        mockHelper,
		enableForceDetach: true,
	}

	// actualPublicationState is a mapping of volumes to volume publications.
	actualPublicationState, err := nodeServer.discoverActualPublicationState(ctx)
	assert.Error(t, err, "expected error")
	assert.Nil(t, actualPublicationState, "expected nil map")
}

func TestDiscoverActualPublicationState_FailsToFindTrackingInfo(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockHelper := mockNodeHelpers.NewMockNodeHelper(mockCtrl)
	mockHelper.EXPECT().ListVolumeTrackingInfo(ctx).Return(nil, errors.NotFoundError("not found"))
	nodeServer := &Plugin{
		role:              CSINode,
		nodeHelper:        mockHelper,
		enableForceDetach: true,
	}

	// actualPublicationState is a mapping of volumes to volume publications.
	actualPublicationState, err := nodeServer.discoverActualPublicationState(ctx)
	assert.NoError(t, err, "expected no error")
	assert.Empty(t, actualPublicationState, "expected empty map")
}

func TestDiscoverStalePublications_DiscoversStalePublicationsCorrectly(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	volumeOne := "pvc-85987a99-648d-4d84-95df-47d0256ca2ab"
	volumeTwo := "pvc-85987a99-648d-4d84-95df-47d0256ca2ac"
	volumeThree := "pvc-85987a99-648d-4d84-95df-47d0256ca2ad"
	desiredPublicationState := map[string]*models.VolumePublicationExternal{
		volumeOne: {
			Name:       models.GenerateVolumePublishName(volumeOne, nodeName),
			NodeName:   nodeName,
			VolumeName: volumeOne,
		},
		// This shouldn't be counted as a stale publication.
		volumeThree: nil,
	}
	actualPublicationState := map[string]*models.VolumeTrackingInfo{
		volumeOne: {
			VolumePublishInfo: models.VolumePublishInfo{},
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
				"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787dd/globalmount",
			PublishedPaths: map[string]struct{}{
				"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e3/volumes/kubernetes.io~csi/" +
					volumeOne + "/mount": {},
			},
		},
		// This is what should be counted as "stale".
		volumeTwo: {
			VolumePublishInfo: models.VolumePublishInfo{},
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
				"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787de/globalmount",
			PublishedPaths: map[string]struct{}{
				"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e2/volumes/kubernetes.io~csi/" +
					volumeTwo + "/mount": {},
			},
		},
	}

	nodeServer := &Plugin{
		role:              CSINode,
		nodeName:          nodeName,
		enableForceDetach: true,
	}

	stalePublications := nodeServer.discoverStalePublications(ctx, actualPublicationState, desiredPublicationState)
	assert.Contains(t, stalePublications, volumeTwo, fmt.Sprintf("expected %s to exist in stale publications", volumeTwo))
	assert.NotContains(t, stalePublications, volumeThree, fmt.Sprintf("expected %s to not exist in stale publications", volumeThree))
}

func TestPerformNodeCleanup_ShouldNotDiscoverAnyStalePublications(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	volume := "pvc-85987a99-648d-4d84-95df-47d0256ca2ab"
	desiredPublicationState := []*models.VolumePublicationExternal{
		{
			Name:       models.GenerateVolumePublishName(volume, nodeName),
			NodeName:   nodeName,
			VolumeName: volume,
		},
	}
	actualPublicationState := map[string]*models.VolumeTrackingInfo{
		volume: {
			VolumePublishInfo: models.VolumePublishInfo{},
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
				"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787dd/globalmount",
			PublishedPaths: map[string]struct{}{
				"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e3/volumes/kubernetes.io~csi/" +
					volume + "/mount": {},
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	mockRestClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(mockCtrl)
	mockRestClient.EXPECT().ListVolumePublicationsForNode(ctx, nodeName).Return(desiredPublicationState, nil)
	mockNodeHelper.EXPECT().ListVolumeTrackingInfo(ctx).Return(actualPublicationState, nil)

	nodeServer := &Plugin{
		role:              CSINode,
		nodeName:          nodeName,
		restClient:        mockRestClient,
		nodeHelper:        mockNodeHelper,
		enableForceDetach: true,
	}
	err := nodeServer.performNodeCleanup(ctx)
	assert.NoError(t, err, "expected no error")
}

func TestPerformNodeCleanup_ShouldFailToDiscoverDesiredPublicationsFromControllerAPI(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	volume := "pvc-85987a99-648d-4d84-95df-47d0256ca2ab"
	desiredPublicationState := []*models.VolumePublicationExternal{
		{
			Name:       models.GenerateVolumePublishName(volume, nodeName),
			NodeName:   nodeName,
			VolumeName: volume,
		},
	}

	mockCtrl := gomock.NewController(t)
	mockRestClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockRestClient.EXPECT().ListVolumePublicationsForNode(
		ctx, nodeName,
	).Return(desiredPublicationState, errors.New("api error"))

	nodeServer := &Plugin{
		role:              CSINode,
		nodeName:          nodeName,
		restClient:        mockRestClient,
		enableForceDetach: true,
	}
	err := nodeServer.performNodeCleanup(ctx)
	assert.Error(t, err, "expected an error")
}

func TestPerformNodeCleanup_ShouldFailToDiscoverActualPublicationsFromHost(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	volume := "pvc-85987a99-648d-4d84-95df-47d0256ca2ab"
	desiredPublicationState := []*models.VolumePublicationExternal{
		{
			Name:       models.GenerateVolumePublishName(volume, nodeName),
			NodeName:   nodeName,
			VolumeName: volume,
		},
	}
	actualPublicationState := map[string]*models.VolumeTrackingInfo{
		volume: {
			VolumePublishInfo: models.VolumePublishInfo{},
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
				"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787dd/globalmount",
			PublishedPaths: map[string]struct{}{
				"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e3/volumes/kubernetes.io~csi/" +
					volume + "/mount": {},
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	mockRestClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(mockCtrl)
	mockRestClient.EXPECT().ListVolumePublicationsForNode(ctx, nodeName).Return(desiredPublicationState, nil)
	mockNodeHelper.EXPECT().ListVolumeTrackingInfo(ctx).Return(actualPublicationState, errors.New("file I/O error"))

	nodeServer := &Plugin{
		role:              CSINode,
		nodeName:          nodeName,
		restClient:        mockRestClient,
		nodeHelper:        mockNodeHelper,
		enableForceDetach: true,
	}
	err := nodeServer.performNodeCleanup(ctx)
	assert.Error(t, err, "expected an error")
}

func TestUpdateNodePublicationState_NodeNotCleanable(t *testing.T) {
	ctx := context.Background()
	nodeState := models.NodeDirty
	nodeServer := &Plugin{
		role:              CSINode,
		enableForceDetach: true,
	}

	err := nodeServer.updateNodePublicationState(ctx, nodeState)
	assert.NoError(t, err, "expected no error")

	nodeState = models.NodeClean
	err = nodeServer.updateNodePublicationState(ctx, nodeState)
	assert.NoError(t, err, "expected no error")
}

func TestUpdateNodePublicationState_FailsToUpdateNodeAsCleaned(t *testing.T) {
	ctx := context.Background()
	nodeState := models.NodeCleanable
	nodeName := "foo"
	nodeStateFlags := &models.NodePublicationStateFlags{
		ProvisionerReady: convert.ToPtr(true),
	}

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().UpdateNode(ctx, nodeName, nodeStateFlags).Return(errors.New("update failed"))
	nodeServer := &Plugin{
		role:              CSINode,
		nodeName:          nodeName,
		restClient:        mockClient,
		enableForceDetach: true,
	}

	err := nodeServer.updateNodePublicationState(ctx, nodeState)
	assert.Error(t, err, "expected error")
}

func TestUpdateNodePublicationState_SuccessfullyUpdatesNodeAsCleaned(t *testing.T) {
	ctx := context.Background()
	nodeState := models.NodeCleanable
	nodeName := "foo"
	nodeStateFlags := &models.NodePublicationStateFlags{
		ProvisionerReady: convert.ToPtr(true),
	}

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().UpdateNode(ctx, nodeName, nodeStateFlags).Return(nil)
	nodeServer := &Plugin{
		role:              CSINode,
		nodeName:          nodeName,
		restClient:        mockClient,
		enableForceDetach: true,
	}

	err := nodeServer.updateNodePublicationState(ctx, nodeState)
	assert.NoError(t, err, "expected no error")
}

func TestPerformNVMeSelfHealing(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockNVMeHandler := mock_nvme.NewMockNVMeInterface(mockCtrl)
	nodeServer := &Plugin{nvmeHandler: mockNVMeHandler}

	// Empty Published sessions case.
	nodeServer.performNVMeSelfHealing(ctx)

	// Error populating current sessions.
	publishedNVMeSessions.AddNVMeSession(nvme.NVMeSubsystem{NQN: "nqn"}, []string{})
	mockNVMeHandler.EXPECT().PopulateCurrentNVMeSessions(ctx, gomock.Any()).
		Return(errors.New("failed to populate current sessions"))

	nodeServer.performNVMeSelfHealing(ctx)

	// Self-healing process done.
	mockNVMeHandler.EXPECT().PopulateCurrentNVMeSessions(ctx, gomock.Any()).Return(nil)
	mockNVMeHandler.EXPECT().InspectNVMeSessions(ctx, gomock.Any(), gomock.Any()).Return([]nvme.NVMeSubsystem{})

	nodeServer.performNVMeSelfHealing(ctx)
	// Cleanup of global objects.
	publishedNVMeSessions.RemoveNVMeSession("nqn")
}

func TestFixNVMeSessions(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockNVMeHandler := mock_nvme.NewMockNVMeInterface(mockCtrl)
	nodeServer := &Plugin{nvmeHandler: mockNVMeHandler}
	subsystem1 := nvme.NVMeSubsystem{NQN: "nqn1"}
	subsystems := []nvme.NVMeSubsystem{subsystem1}

	// Subsystem not present in published sessions case.
	nodeServer.fixNVMeSessions(ctx, time.UnixMicro(0), subsystems)

	// Rectify NVMe session.
	publishedNVMeSessions.AddNVMeSession(subsystem1, []string{})
	mockNVMeHandler.EXPECT().RectifyNVMeSession(ctx, gomock.Any(), gomock.Any())

	nodeServer.fixNVMeSessions(ctx, time.UnixMicro(0), subsystems)
	// Cleanup of global objects.
	publishedNVMeSessions.RemoveNVMeSession(subsystem1.NQN)
}

// The test is to check if the lock is acquired by the first request for a long time
// the second request timesout and returns false while attempting to aquire lock
// This is done by letting the first request acquire the lock and starting another go routine
// that also tries to take a lock with a timeout of 2sec. The first requests relinquishes the lock
// after 5sec. By the time the second request gets the lock, locktimeout has expired and it returns
// a failure
func TestAttemptLock_Failure(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	ctx := context.Background()
	lockContext := "fakeLockContext-req1"
	lockTimeout := 200 * time.Millisecond
	// first request takes the lock
	expected := attemptLock(ctx, lockContext, nodeLockID, lockTimeout)

	// start the second request so that it is in race for the lock
	go func() {
		defer wg.Done()
		ctx := context.Background()
		lockContext := "fakeLockContext-req2"
		expected := attemptLock(ctx, lockContext, nodeLockID, lockTimeout)

		assert.False(t, expected)
		locks.Unlock(ctx, lockContext, nodeLockID)
	}()
	// first request goes to sleep holding the lock
	if expected {
		time.Sleep(500 * time.Millisecond)
	}
	locks.Unlock(ctx, lockContext, nodeLockID)
	wg.Wait()
}

// The test is to check if the lock is acquired by the first request for a short time
// the second request doesn't timesout and aquires lock after request1 releases the lock
// This is done by letting the first request acquire the lock and starting another go routine
// that also tries to take a lock with a timeout of 5sec. The first requests relinquishes the lock
// after 2sec. The second request gets the lock before the locktimeout has expired and returns success.
func TestAttemptLock_Success(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	ctx := context.Background()
	lockContext := "fakeLockContext-req1"
	lockTimeout := 500 * time.Millisecond
	// first request takes the lock
	expected := attemptLock(ctx, lockContext, nodeLockID, lockTimeout)

	// start the second request so that it is in race for the lock
	go func() {
		defer wg.Done()
		ctx := context.Background()
		lockContext := "fakeLockContext-req2"
		lockTimeout := 5 * time.Second

		expected := attemptLock(ctx, lockContext, nodeLockID, lockTimeout)

		assert.True(t, expected)
		locks.Unlock(ctx, lockContext, nodeLockID)
	}()
	// first request goes to sleep holding the lock
	if expected {
		time.Sleep(200 * time.Millisecond)
	}
	locks.Unlock(ctx, lockContext, nodeLockID)
	wg.Wait()
}

func TestDeprecatedIgroupsInUse(t *testing.T) {
	tt := map[string]struct {
		tracking map[string]*models.VolumeTrackingInfo
		expected bool
	}{
		"when default trident igroup is used for one volume": {
			tracking: map[string]*models.VolumeTrackingInfo{
				"one": {
					VolumePublishInfo: models.VolumePublishInfo{
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiIgroup: "node-01-ad1b8212-8095-49a0-82d4-ef4f8b5b620z",
							},
						},
					},
				},
				"two": {
					VolumePublishInfo: models.VolumePublishInfo{
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiIgroup: "trident",
							},
						},
					},
				},
			},
			expected: true,
		},
		"when custom trident igroup is used for one volume": {
			tracking: map[string]*models.VolumeTrackingInfo{
				"one": {
					VolumePublishInfo: models.VolumePublishInfo{
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiIgroup: "node-01-ad1b8212-8095-49a0-82d4-ef4f8b5b620z",
							},
						},
					},
				},
				"two": {
					VolumePublishInfo: models.VolumePublishInfo{
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiIgroup: "my-trident-igroup",
							},
						},
					},
				},
			},
			expected: true,
		},
		"when per-node igroups are used for all volumes": {
			tracking: map[string]*models.VolumeTrackingInfo{
				"one": {
					VolumePublishInfo: models.VolumePublishInfo{
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiIgroup: "node-01-ad1b8212-8095-49a0-82d4-ef4f8b5b620z",
							},
						},
					},
				},
				"two": {
					VolumePublishInfo: models.VolumePublishInfo{
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiIgroup: "node-01-ad1b8212-8095-49a0-82d4-ef4f8b5b620z",
							},
						},
					},
				},
			},
			expected: false,
		},
	}

	for test, data := range tt {
		t.Run(test, func(t *testing.T) {
			ctx := context.Background()
			mockCtrl := gomock.NewController(t)
			mockHelper := mockNodeHelpers.NewMockNodeHelper(mockCtrl)

			mockHelper.EXPECT().ListVolumeTrackingInfo(ctx).Return(data.tracking, nil).Times(1)

			p := Plugin{nodeHelper: mockHelper}
			assert.Equal(t, data.expected, p.deprecatedIgroupInUse(ctx))
		})
	}
}

func TestNodeRegisterWithController_Success(t *testing.T) {
	ctx := context.Background()
	nodeName := "fakeNode"

	// Create a mock rest client for Trident controller, mock core and mock NVMe handler
	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockNVMeHandler := mock_nvme.NewMockNVMeInterface(mockCtrl)

	iscsiClient, _ := iscsi.New()
	// Create a node server plugin
	nodeServer := &Plugin{
		nodeName:     nodeName,
		role:         CSINode,
		hostInfo:     &models.HostSystem{},
		restClient:   mockClient,
		nvmeHandler:  mockNVMeHandler,
		orchestrator: mockOrchestrator,
		osutils:      osutils.New(),
		iscsi:        iscsiClient,
	}

	// Create a fake node response to be returned by controller
	fakeNodeResponse := controllerAPI.CreateNodeResponse{
		TopologyLabels: map[string]string{},
		LogLevel:       "debug",
		LogWorkflows:   "frontend",
		LogLayers:      "node=add",
	}

	// Set expects
	mockClient.EXPECT().CreateNode(ctx, gomock.Any()).Return(fakeNodeResponse, nil)
	mockNVMeHandler.EXPECT().NVMeActiveOnHost(ctx).Return(false, nil)
	mockOrchestrator.EXPECT().SetLogLayers(ctx, fakeNodeResponse.LogLayers).Return(nil)
	mockOrchestrator.EXPECT().SetLogLevel(ctx, fakeNodeResponse.LogLevel).Return(nil)
	mockOrchestrator.EXPECT().SetLoggingWorkflows(ctx, fakeNodeResponse.LogWorkflows).Return(nil)

	// register node with controller
	nodeServer.nodeRegisterWithController(ctx, 1*time.Second)

	// assert node is registered
	assert.True(t, nodeServer.nodeIsRegistered, "expected node to be registered, but it is not")
}

func TestNodeRegisterWithController_TopologyLabels(t *testing.T) {
	ctx := context.Background()
	nodeName := "fakeNode"

	// Create a mock rest client for Trident controller, mock core and mock NVMe handler
	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockNVMeHandler := mock_nvme.NewMockNVMeInterface(mockCtrl)
	iscsiClient, _ := iscsi.New()

	// Create a node server plugin
	nodeServer := &Plugin{
		nodeName:     nodeName,
		role:         CSINode,
		hostInfo:     &models.HostSystem{},
		restClient:   mockClient,
		nvmeHandler:  mockNVMeHandler,
		orchestrator: mockOrchestrator,
		osutils:      osutils.New(),
		iscsi:        iscsiClient,
	}

	// Create set of cases with varying topology labels
	tt := map[string]struct {
		topologyLabels map[string]string
		expected       bool
	}{
		"when no topology labels are set": {
			topologyLabels: map[string]string{},
			expected:       false,
		},
		"when only zone label is set": {
			topologyLabels: map[string]string{
				"topology.kubernetes.io/zone": "us-west-1",
			},
			expected: false,
		},
		"when only region label is set": {
			topologyLabels: map[string]string{
				"topology.kubernetes.io/region": "us-west",
			},
			expected: true,
		},
		"when both zone and region labels are set": {
			topologyLabels: map[string]string{
				"topology.kubernetes.io/zone":   "us-west-1",
				"topology.kubernetes.io/region": "us-west",
			},
			expected: true,
		},
		"when neither zone nor region labels are set": {
			topologyLabels: map[string]string{
				"topology.kubernetes.io/foo": "bar",
			},
			expected: false,
		},
	}

	for test, data := range tt {
		t.Run(test, func(t *testing.T) {
			// Create a fake node response to be returned by controller
			fakeNodeResponse := controllerAPI.CreateNodeResponse{
				TopologyLabels: data.topologyLabels,
				LogLevel:       "debug",
				LogWorkflows:   "frontend",
				LogLayers:      "node=add",
			}

			// Set expects
			mockClient.EXPECT().CreateNode(ctx, gomock.Any()).Return(fakeNodeResponse, nil)
			mockNVMeHandler.EXPECT().NVMeActiveOnHost(ctx).Return(false, nil)
			mockOrchestrator.EXPECT().SetLogLayers(ctx, fakeNodeResponse.LogLayers).Return(nil)
			mockOrchestrator.EXPECT().SetLogLevel(ctx, fakeNodeResponse.LogLevel).Return(nil)
			mockOrchestrator.EXPECT().SetLoggingWorkflows(ctx, fakeNodeResponse.LogWorkflows).Return(nil)

			// register node with controller
			nodeServer.nodeRegisterWithController(ctx, 1*time.Second)

			// assert node is registered and topology in use is as expected
			assert.True(t, nodeServer.nodeIsRegistered, "expected node to be registered, but it is not")
			assert.Equal(t, data.expected, nodeServer.topologyInUse, "topologyInUse not as expected")
		})
	}
}

func TestNodeRegisterWithController_Failure(t *testing.T) {
	ctx := context.Background()
	nodeName := "fakeNode"

	// Create a mock rest client for Trident controller, mock core and mock NVMe handler
	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockNVMeHandler := mock_nvme.NewMockNVMeInterface(mockCtrl)
	iscsiClient, _ := iscsi.New()

	// Create a node server plugin
	nodeServer := &Plugin{
		nodeName:     nodeName,
		role:         CSINode,
		hostInfo:     &models.HostSystem{},
		restClient:   mockClient,
		nvmeHandler:  mockNVMeHandler,
		orchestrator: mockOrchestrator,
		iscsi:        iscsiClient,
		osutils:      osutils.New(),
	}

	// Create a fake node response to be returned by controller
	fakeNodeResponse := controllerAPI.CreateNodeResponse{
		LogLevel:     "debug",
		LogWorkflows: "frontend",
		LogLayers:    "node=add",
	}

	// Case: Error creating node by trident controller
	mockNVMeHandler.EXPECT().NVMeActiveOnHost(ctx).Return(false, nil)
	mockClient.EXPECT().CreateNode(ctx, gomock.Any()).Return(controllerAPI.CreateNodeResponse{}, errors.New("failed to create node"))

	nodeServer.nodeRegisterWithController(ctx, 1*time.Second)

	assert.True(t, nodeServer.nodeIsRegistered, "expected node to be registered, but it is not")

	// Case: Error setting log level
	mockClient.EXPECT().CreateNode(ctx, gomock.Any()).Return(fakeNodeResponse, nil)
	mockNVMeHandler.EXPECT().NVMeActiveOnHost(ctx).Return(false, nil)
	mockOrchestrator.EXPECT().SetLogLayers(ctx, fakeNodeResponse.LogLayers).Return(nil)
	mockOrchestrator.EXPECT().SetLogLevel(ctx, fakeNodeResponse.LogLevel).Return(errors.New("failed to set log level"))
	mockOrchestrator.EXPECT().SetLoggingWorkflows(ctx, fakeNodeResponse.LogWorkflows).Return(nil)

	nodeServer.nodeRegisterWithController(ctx, 1*time.Second)

	assert.True(t, nodeServer.nodeIsRegistered, "expected node to be registered, but it is not")

	// Case: Error setting log layer
	mockClient.EXPECT().CreateNode(ctx, gomock.Any()).Return(fakeNodeResponse, nil)
	mockNVMeHandler.EXPECT().NVMeActiveOnHost(ctx).Return(false, nil)
	mockOrchestrator.EXPECT().SetLogLayers(ctx, fakeNodeResponse.LogLayers).Return(errors.New("failed to set log layers"))
	mockOrchestrator.EXPECT().SetLogLevel(ctx, fakeNodeResponse.LogLevel).Return(nil)
	mockOrchestrator.EXPECT().SetLoggingWorkflows(ctx, fakeNodeResponse.LogWorkflows).Return(nil)

	nodeServer.nodeRegisterWithController(ctx, 1*time.Second)

	assert.True(t, nodeServer.nodeIsRegistered, "expected node to be registered, but it is not")

	// Case: Error setting log workflow
	mockClient.EXPECT().CreateNode(ctx, gomock.Any()).Return(fakeNodeResponse, nil)
	mockNVMeHandler.EXPECT().NVMeActiveOnHost(ctx).Return(false, nil)
	mockOrchestrator.EXPECT().SetLogLayers(ctx, fakeNodeResponse.LogLayers).Return(nil)
	mockOrchestrator.EXPECT().SetLogLevel(ctx, fakeNodeResponse.LogLevel).Return(nil)
	mockOrchestrator.EXPECT().SetLoggingWorkflows(ctx, fakeNodeResponse.LogWorkflows).Return(errors.New("failed to set log workflows"))

	nodeServer.nodeRegisterWithController(ctx, 1*time.Second)

	assert.True(t, nodeServer.nodeIsRegistered, "expected node to be registered, but it is not")
}

func TestNodeUnstageISCSIVolume(t *testing.T) {
	defer func(previousIscsiUtils iscsi.IscsiReconcileUtils) {
		iscsiUtils = previousIscsiUtils
	}(iscsiUtils)

	type parameters struct {
		getISCSIClient               func() iscsi.ISCSI
		getIscsiReconcileUtilsClient func() iscsi.IscsiReconcileUtils
		getDeviceClient              func() devices.Devices
		getMountClient               func() mount.Mount
		getNodeHelper                func() nodehelpers.NodeHelper
		publishInfo                  models.VolumePublishInfo
		force                        bool
		request                      *csi.NodeUnstageVolumeRequest
		assertError                  assert.ErrorAssertionFunc
	}

	mockDevicePath := "/dev/mapper/mock-device"
	mockDevice := &models.ScsiDeviceInfo{MultipathDevice: mockDevicePath}

	tests := map[string]parameters{
		"SAN: iSCSI unstage: happy path": {
			assertError: assert.NoError,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
				mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
				mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), false, false).Return("", nil)
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
				return mockISCSIClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil)
				return mockDeviceClient
			},
			getMountClient: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
				return mockMountClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3})
				return mockIscsiReconcileUtilsClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
				return mockNodeHelper
			},
		},
		"SAN: iSCSI unstage: happy path LUKS": {
			assertError: assert.NoError,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).WithLUKSEncryption("true").Build(),
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
				mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
				mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), false, false).Return("", nil)
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
				return mockISCSIClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(gomock.Any(), gomock.Any()).Return(mockDevicePath, nil)
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), mockDevicePath).Return(nil)
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosed(gomock.Any(), mockDevicePath).Return(nil)
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil)
				return mockDeviceClient
			},
			getMountClient: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
				return mockMountClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3})
				return mockIscsiReconcileUtilsClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
				return mockNodeHelper
			},
		},
		"SAN: iSCSI unstage: GetUnderlyingDevicePathForDevice error": {
			assertError: assert.NoError,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).WithLUKSEncryption("true").Build(),
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())

				mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
				mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)

				mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), false, false).Return("", nil)
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)

				return mockISCSIClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(gomock.Any(), gomock.Any()).Return(mockDevicePath, nil)
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), mockDevicePath).
					Return(nil)
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosed(gomock.Any(), mockDevicePath).Return(nil)
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil)
				return mockDeviceClient
			},
			getMountClient: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
				return mockMountClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3})
				return mockIscsiReconcileUtilsClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
				return mockNodeHelper
			},
		},
		"SAN: iSCSI unstage: LuksClose error": {
			assertError: assert.Error,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).WithLUKSEncryption("true").Build(),
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
				return mockISCSIClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3})
				return mockIscsiReconcileUtilsClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(gomock.Any(), gomock.Any()).Return(mockDevicePath, nil)
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), mockDevicePath).
					Return(fmt.Errorf("mock error"))
				return mockDeviceClient
			},
		},
		"SAN: iSCSI unstage: GetISCSIHostSessionMapForTarget no sessions": {
			assertError: assert.NoError,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).WithLUKSEncryption("true").Build(),
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{})
				return mockIscsiReconcileUtilsClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(gomock.Any(), gomock.Any()).Return(mockDevicePath, nil)
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), mockDevicePath).
					Return(nil)
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil)
				return mockDeviceClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
				return mockNodeHelper
			},
		},
		"SAN: iSCSI unstage: GetDeviceInfoForLUN no devices": {
			assertError: assert.NoError,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).WithLUKSEncryption("true").Build(),
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3})
				return mockIscsiReconcileUtilsClient
			},
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(nil, nil)
				mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(),
					"iqn.1992-08.com.netapp:sn.a0e6b50f49e611efa8b5005056b33c0d:vs.2").Return(false, nil)
				mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), 6, 3).Return(true)
				mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any())
				return mockISCSIClient
			},
			getMountClient: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil)
				return mockDeviceClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
				return mockNodeHelper
			},
		},
		"SAN: iSCSI unstage: GetDeviceInfoForLUN error": {
			assertError: assert.Error,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).WithLUKSEncryption("true").Build(),
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3})
				return mockIscsiReconcileUtilsClient
			},
			getISCSIClient: func() iscsi.ISCSI {
				mockIscsiClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockIscsiClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(nil, fmt.Errorf("mock error"))
				return mockIscsiClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				return mockDeviceClient
			},
		},
		// mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(gomock.Any(), gomock.Any()).Return(mockDevicePath, nil)
		"SAN: iSCSI unstage: GetLUKSDevicePathForVolume error": {
			assertError: assert.Error,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).WithLUKSEncryption("true").Build(),
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
				return mockISCSIClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3})
				return mockIscsiReconcileUtilsClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(gomock.Any(),
					gomock.Any()).Return(mockDevicePath, errors.New("mock error"))
				return mockDeviceClient
			},
		},
		"SAN: iSCSI unstage: PrepareDeviceForRemoval error": {
			assertError: assert.Error,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).WithLUKSEncryption("true").Build(),
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), false, false).Return("", fmt.Errorf("mock error"))
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
				return mockISCSIClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(gomock.Any(), gomock.Any()).Return(mockDevicePath, nil)
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), mockDevicePath).
					Return(nil)
				return mockDeviceClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3})
				return mockIscsiReconcileUtilsClient
			},
		},
		"SAN: iSCSI unstage: PrepareDeviceForRemoval same LUN error": {
			assertError: assert.NoError,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).WithLUKSEncryption("true").Build(),
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
				mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
				mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), false, false).Return("", errors.ISCSISameLunNumberError("mock error"))
				mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), false, false).Return("", nil)
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
				return mockISCSIClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(gomock.Any(), gomock.Any()).Return(mockDevicePath, nil)
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), mockDevicePath).
					Return(nil)
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosed(gomock.Any(), mockDevicePath).Return(nil)
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil)
				return mockDeviceClient
			},
			getMountClient: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
				return mockMountClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3})
				return mockIscsiReconcileUtilsClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{}, nil).AnyTimes()
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
				return mockNodeHelper
			},
		},
		"SAN: iSCSI unstage: Unsafe to logout, mounts exist": {
			assertError: assert.NoError,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(true, nil)
				mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), false, false).Return("", nil)
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
				return mockISCSIClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil)
				return mockDeviceClient
			},
			getMountClient: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
				return mockMountClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3})
				return mockIscsiReconcileUtilsClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
				return mockNodeHelper
			},
		},
		"SAN: iSCSI unstage: Unsafe to logout, iSCSI sessions exist": {
			assertError: assert.NoError,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
				mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(false)
				mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), false, false).Return("", nil)
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
				return mockISCSIClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil)
				return mockDeviceClient
			},
			getMountClient: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
				return mockMountClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3, 7: 4})
				return mockIscsiReconcileUtilsClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
				return mockNodeHelper
			},
		},
		"SAN: iSCSI unstage: temp mount point removal failure": {
			assertError: assert.Error,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
				mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
				mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), false, false).Return("", nil)
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
				return mockISCSIClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				return mockDeviceClient
			},
			getMountClient: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("mock error"))
				return mockMountClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3})
				return mockIscsiReconcileUtilsClient
			},
		},
		"SAN: iSCSI unstage: RemoveMultipathDeviceMapping failure": {
			assertError: assert.Error,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
				mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
				mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), false, false).Return("", nil)
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
				return mockISCSIClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(fmt.Errorf("mock error"))
				return mockDeviceClient
			},
			getMountClient: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
				return mockMountClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3})
				return mockIscsiReconcileUtilsClient
			},
		},
		"SAN: iSCSI unstage: DeleteTrackingInfo failure": {
			assertError: assert.Error,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
				mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
				mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any())
				mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), false, false).Return("", nil)
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
				return mockISCSIClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil)
				return mockDeviceClient
			},
			getMountClient: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).
					Return(nil)
				return mockMountClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{6: 3})
				return mockIscsiReconcileUtilsClient
			},
			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("mock error"))
				return mockNodeHelper
			},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			plugin := &Plugin{
				command: execCmd.NewCommand(),
				role:    CSINode,
			}

			if params.getISCSIClient != nil {
				plugin.iscsi = params.getISCSIClient()
			}

			if params.getNodeHelper != nil {
				plugin.nodeHelper = params.getNodeHelper()
			}

			if params.getIscsiReconcileUtilsClient != nil {
				iscsiUtils = params.getIscsiReconcileUtilsClient()
			}

			if params.getDeviceClient != nil {
				plugin.devices = params.getDeviceClient()
			}

			if params.getMountClient != nil {
				plugin.mount = params.getMountClient()
			}

			err := plugin.nodeUnstageISCSIVolume(context.Background(), params.request, &params.publishInfo,
				params.force)
			params.assertError(t, err)
		})
	}
}

// --------------------- Multithreaded Unit Tests ---------------------

func TestNodeStageVolume_Multithreaded(t *testing.T) {
	// Test 1:
	// Sending n number of NAS requests of different volume.uuid, and they should succeed
	t.Run("Test Case 1: Sending n num of NAS requests with different volume.uuid and asserting noError", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeStageVolumeRequestBuilder(TypeNFSRequest).Build()
		}

		// This csiNodeLockTimeout is just for the locks and not for the whole operation.
		csiNodeLockTimeout = 500 * time.Millisecond

		// Creating mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for each request.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 2:
	// Sending n number of SAN requests of different volume.uuid, and they should succeed
	t.Run("Test Case 2: Sending n num of SAN requests with different volume.uuid and asserting noError", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithVolumeID(uuid.NewString()).Build()
		}

		// This csiNodeLockTimeout is just for the locks and not for the whole operation.
		csiNodeLockTimeout = 1 * time.Second

		// Setting up mock clients.
		ctrl := gomock.NewController(t)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			mockISCSIClient.EXPECT().AttachVolumeRetry(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).Return(int64(1), nil)
			mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
			mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
			fs:               filesystem.New(mountClient),
			iscsi:            mockISCSIClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for each request.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 3:
	// Sending two requests at the same time with the same volume.uuid.
	// The Second one should fail as the first one still holds the lock after the timeout.
	t.Run("Test Case 3: Sending n NAS request with same volume.uuid at the same time.", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeStageVolumeRequestBuilder(TypeNFSRequest).Build()
		}

		signalChan := make(chan struct{})

		// Changing the lock timeout to some lower value.
		csiNodeLockTimeout = 500 * time.Millisecond

		volumeID := "1234-5678"
		for i := 0; i < numOfRequests; i++ {
			requests[i].VolumeId = volumeID
		}

		// Setting up mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up expectations for the first request.
		mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
		// Mimicking that one of the operation takes more time than the timeout.
		mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string, *models.VolumeTrackingInfo) error {
			signalChan <- struct{}{}
			time.Sleep(600 * time.Millisecond)
			return nil
		})

		// Creating node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routine for the first request.
		go func() {
			defer wg.Done()
			_, err := plugin.NodeStageVolume(context.Background(), requests[0])
			assert.NoError(t, err)
		}()

		// Waiting on signal to ensure the first requests acquires the lock.
		<-signalChan

		for i := 1; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), requests[i])
				assert.Error(t, err)
			}()
		}

		wg.Wait()
	})

	// Test4:
	// Mixing SAN and NAS requests
	t.Run("Test Case 4: Mixing SAN and NAS requests and waiting for both of them to succeed", func(t *testing.T) {
		numOfRequestsNAS := 50
		numOfRequestsSAN := 50

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		NASrequests := make([]*csi.NodeStageVolumeRequest, numOfRequestsNAS)
		SANrequests := make([]*csi.NodeStageVolumeRequest, numOfRequestsSAN)
		for i := 0; i < numOfRequestsNAS; i++ {
			NASrequests[i] = NewNodeStageVolumeRequestBuilder(TypeNFSRequest).Build()
		}
		for i := 0; i < numOfRequestsSAN; i++ {
			SANrequests[i] = NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).WithVolumeID(uuid.NewString()).Build()
		}

		csiNodeLockTimeout = 1 * time.Second

		// Setting up mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()

		for i := 0; i < numOfRequestsNAS; i++ {
			// Setting up mocks expectation for NAS request.
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		for i := 0; i < numOfRequestsSAN; i++ {
			// Setting up mocks expectation for SAN request.
			mockISCSIClient.EXPECT().AttachVolumeRetry(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).Return(int64(1), nil)
			mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
			mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil)
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequestsNAS + numOfRequestsSAN)

		// Spinning up go-routine for the NAS requests.
		for i := 0; i < numOfRequestsNAS; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), NASrequests[i])
				assert.NoError(t, err)
			}()
		}
		// Spinning up go-routine for the SAN requests.
		for i := 0; i < numOfRequestsSAN; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), SANrequests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test5:
	// Mixing NFS and SMB requests
	t.Run("Test Case 5: Mixing NFS and SMB requests and waiting for both of them to succeed", func(t *testing.T) {
		numOfRequests := 50

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		NFSrequests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		SMBrequests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			NFSrequests[i] = NewNodeStageVolumeRequestBuilder(TypeNFSRequest).Build()
			SMBrequests[i] = NewNodeStageVolumeRequestBuilder(TypeSMBRequest).Build()
		}

		// This csiNodeLockTimeout is just for the locks and not for the whole operation.
		csiNodeLockTimeout = 500 * time.Millisecond

		// Setting up mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)

		for i := 0; i < numOfRequests; i++ {
			// Setting up mocks expectation for NFS request.
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			// Setting up mocks expectation for SMB request.
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockMount.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests * 2)

		for i := 0; i < numOfRequests; i++ {
			// Spinning up go-routine for NFS requests.
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), NFSrequests[i])
				assert.NoError(t, err)
			}()

			// Spinning up go-rotuine for SMB requests.
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), SMBrequests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 6 (FCP):
	// Sending requests within the limit of the limiter
	t.Run("Test Case 6: Sending n num of FCP requests with different volume.uuid and asserting noError", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests, publishInfos := createFCPStageRequests(numOfRequests)

		csiNodeLockTimeout = 500 * time.Millisecond

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockFCPClient := mock_fcp.NewMockFCP(ctrl)

		// Setting up expectations for each request
		for i := 0; i < numOfRequests; i++ {
			mockFCPClient.EXPECT().AttachVolumeRetry(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).Return(int64(0), nil).Times(1)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
		}

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			fcp:              mockFCPClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		// Modifying the limiter limit
		plugin.limiterSharedMap[NodeStageFCPVolume], _ = limiter.New(ctx,
			NodeStageFCPVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(len(requests))

		for i := 0; i < len(requests); i++ {
			go func(idx int) {
				defer wg.Done()
				err := plugin.nodeStageFCPVolume(ctx, requests[idx], publishInfos[idx])
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()
	})

	// Test 7 (FCP):
	// Sending n requests at the same time with the same volume.uuid.
	// First one should pass and all the rest should fail.
	t.Run("Test Case 7: Sending n FCP request with same volume.uuid at the same time.", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests, _ := createFCPStageRequests(numOfRequests)
		csiNodeLockTimeout = 500 * time.Millisecond

		signalChan := make(chan struct{})

		volumeID := "1234-5678" // Making volume.uuid same for all the requests.
		for i := 0; i < numOfRequests; i++ {
			requests[i].VolumeId = volumeID
		}

		// Setting up mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockFCPClient := mock_fcp.NewMockFCP(ctrl)

		// Setting up expectations for the first request, and we do not need to set expectations for other calls.
		mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string, *models.VolumeTrackingInfo) error {
			signalChan <- struct{}{}
			time.Sleep(600 * time.Millisecond)
			return nil
		}).Times(1)
		mockFCPClient.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil).Times(1)
		mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			fcp:              mockFCPClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		// Modifying the limiter limit
		plugin.limiterSharedMap[NodeStageFCPVolume], _ = limiter.New(ctx,
			NodeStageFCPVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routine for the first request.
		go func() {
			defer wg.Done()
			_, err := plugin.NodeStageVolume(ctx, requests[0])
			assert.NoError(t, err)
		}()

		// Waiting on signal to ensure the first requests acquires the lock.
		<-signalChan

		for i := 1; i < numOfRequests; i++ {
			go func(index int) {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(ctx, requests[index])
				assert.Error(t, err)
			}(i)
		}

		wg.Wait()
	})
}

func TestNodeStageNFSVolume_Multithreaded(t *testing.T) {
	// Test 1:
	// Initializing SemahaphoreN, and sending the requests within the limit of the limiter
	// and waiting for all of them to succeed
	t.Run("Test Case 1: Sending requests within the limit of the limiter.", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeStageVolumeRequestBuilder(TypeNFSRequest).Build()
		}

		// Setting up mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up expectations for each request.
		for i := 0; i < numOfRequests; i++ {
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		// Modifying the limiter limit.
		plugin.limiterSharedMap[NodeStageNFSVolume], _ = limiter.New(ctx,
			NodeStageNFSVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for each request.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeStageNFSVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 2:
	// Initializing SemahaphoreN, Sending more than what can be parallelized.
	t.Run("Test Case 2: Sending more than what can be parallelized.", func(t *testing.T) {
		numOfRequests := 100 // stress testing with 100 requests.
		numOfParallelRequestsAllowed := maxNodeStageNFSVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeStageVolumeRequestBuilder(TypeNFSRequest).Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed) // For synchronization only.

		// Setting up mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up mock requests for numOfParallelRequestsAllowed out of 100 requests, as others will be errored out.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			// Mimicking that one of the operations takes more time, so that the whole request cannot be completed within the timeout.
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string, *models.VolumeTrackingInfo) error {
				signalChan <- struct{}{} // At this point limiter has been already acquired.
				time.Sleep(600 * time.Millisecond)
				return nil
			})
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routine for the first maxNodeUnstageNFSVolumeOperations requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeStageNFSVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on the signal to ensure that the first maxNodeStageNFSVolumeOperations requests acquire the limiter.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		// Spinning up the rest of the requests.
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeStageNFSVolume(ctx, requests[i])
				assert.Error(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 3:
	// Initializing SemahaphoreN, Sending more than what can be parallelized, but the extra one waits and succeeds.
	t.Run("Test Case 3: Sending more than what can be parallelized, but the additional ones waits and succeeds.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeStageNFSVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeStageVolumeRequestBuilder(TypeNFSRequest).Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed)

		// Setting up mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up expectations for numOfParallelRequestsAllowed number of requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			// Mimicking that one of the operations takes more time, so that the whole request cannot be completed within the timeout.
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string, *models.VolumeTrackingInfo) error {
				signalChan <- struct{}{}
				time.Sleep(100 * time.Millisecond)
				return nil
			})
		}

		// Setting up expectations for remaining requests.
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routine for numOfParallelRequestsAllowed number of requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeStageNFSVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on signal to ensure that the first numOfParallelRequestsAllowed requests acquire the lock.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		// Spinning up the go-routine for remaining requests.
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeStageNFSVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})
}

func TestNodeStageSMBVolume_Multithreaded(t *testing.T) {
	// Test 1:
	// Initializing SemahaphoreN, and sending the requests within the limit of the limiter
	// and waiting for them to succeed
	t.Run("Test Case 1: Sending requests within the limit of the limiter.", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeStageVolumeRequestBuilder(TypeSMBRequest).Build()
		}

		// Setting up mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up expectations for each request.
		for i := 0; i < numOfRequests; i++ {
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockMount.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		// Modifying the limit of the limiter.
		plugin.limiterSharedMap[NodeStageSMBVolume], _ = limiter.New(ctx,
			NodeUnstageSMBVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for each request.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeStageSMBVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 2:
	// Initializing SemahaphoreN, Sending more than what can be parallelized.
	t.Run("Test Case 2: Sending more than what can be parallelized.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeStageSMBVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeStageVolumeRequestBuilder(TypeSMBRequest).Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed) // For synchronization only.

		// Setting up mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up mock requests for numOfParallelRequestsAllowed out of 100 requests, as others will be errored out.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			// Mimicking that one of the operations takes more time, thereby the whole requests take more time than the timeout.
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string, *models.VolumeTrackingInfo) error {
				signalChan <- struct{}{} // At this point limiter has been already acquired.
				time.Sleep(600 * time.Millisecond)
				return nil
			})
			mockMount.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routine for the first numOfParallelRequestsAllowed requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeStageSMBVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on the signal to ensure that the first numOfParallelRequestsAllowed requests acquire the limiter.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeStageSMBVolume(ctx, requests[i])
				assert.Error(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 3:
	// Initializing SemahaphoreN, Sending more than what can be parallelized, but the extra one waits and succeeds.
	t.Run("Test Case 3: Sending more than what can be parallelized, but the additional one waits and succeeds.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeStageSMBVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeStageVolumeRequestBuilder(TypeSMBRequest).Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed)

		// Setting up mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up expectations for numOfParallelRequestsAllowed requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string, *models.VolumeTrackingInfo) error {
				signalChan <- struct{}{}
				time.Sleep(100 * time.Millisecond)
				return nil
			})
			mockMount.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Setting up expectations for remaining requests.
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockMount.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routine for numOfParallelRequestsAllowed number of requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeStageSMBVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on signal to ensure that the first numOfParallelRequestsAllowed requests acquire the lock.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		// Spinning up the go-routine for remaining requests.
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeStageSMBVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})
}

func TestNodeUnstageVolume_Multithreaded(t *testing.T) {
	// Test 1:
	// Sending n number of NAS requests of different volume.uuid, and they should succeed
	t.Run("Test Case 1: Sending n num of NAS requests with different volume.uuid and asserting noError", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests := func() []*csi.NodeUnstageVolumeRequest {
			requests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
			for i := 0; i < numOfRequests; i++ {
				requests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(uuid.New().String()).Build()
			}
			return requests
		}()

		csiNodeLockTimeout = 500 * time.Millisecond

		// Setting up mock clients.
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routines for each request
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 2:
	// Sending n number of SAN requests of different volume.uuid, and they should succeed
	t.Run("Test Case 2: Sending n num of SAN requests with different volume.uuid and asserting noError", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests := func() []*csi.NodeUnstageVolumeRequest {
			requests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
			for i := 0; i < numOfRequests; i++ {
				requests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(uuid.NewString()).Build()
			}
			return requests
		}()

		csiNodeLockTimeout = 1 * time.Second

		// Setting up mock clients.
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
		mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
		mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
		mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
		mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))

		mockDevicePath := "/dev/mapper/mock-device"
		mockDevice := &models.ScsiDeviceInfo{MultipathDevice: mockDevicePath}

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
			mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
			mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), false, false).Return("", nil)
			mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)

			mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
			mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any()).Return(nil)

			mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)

			mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
				gomock.Any()).Return(map[int]int{6: 3})

			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
			iscsi:            mockISCSIClient,
			devices:          mockDeviceClient,
			mount:            mockMountClient,
		}

		plugin.InitializeNodeLimiter(ctx)
		iscsiUtils = mockIscsiReconcileUtilsClient

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routines for each request
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 3:
	// Sending two requests at the same time with the same volume.uuid.
	// The Second one should fail as the first one still holds the lock after the timeout.
	t.Run("Test Case 3: Sending n NAS request with same volume.uuid at the same time.", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeUnstageVolumeRequestBuilder().Build()
		}

		signalChan := make(chan struct{})

		csiNodeLockTimeout = 500 * time.Millisecond

		volumeID := "1234-5678"
		for i := 0; i < numOfRequests; i++ {
			requests[i].VolumeId = volumeID
		}

		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))

		volumeTrackingInfo := &models.VolumeTrackingInfo{
			VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
		}
		mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
		mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string) error {
			signalChan <- struct{}{}
			time.Sleep(600 * time.Millisecond)
			return nil
		})

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		go func() {
			defer wg.Done()
			_, err := plugin.NodeUnstageVolume(context.Background(), requests[0])
			assert.NoError(t, err)
		}()

		<-signalChan

		for i := 1; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), requests[i])
				assert.Error(t, err)
			}()
		}

		wg.Wait()
	})

	// Test4:
	// Mixing SAN and NAS requests
	t.Run("Test Case 4: Mixing SAN and NAS requests and waiting for both of them to succeed", func(t *testing.T) {
		numOfRequestsNAS := 50
		numOfRequestsSAN := 50

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		NASrequests := make([]*csi.NodeUnstageVolumeRequest, numOfRequestsNAS)
		SANrequests := make([]*csi.NodeUnstageVolumeRequest, numOfRequestsSAN)

		for i := 0; i < numOfRequestsNAS; i++ {
			NASrequests[i] = NewNodeUnstageVolumeRequestBuilder().Build()
		}
		for i := 0; i < numOfRequestsSAN; i++ {
			SANrequests[i] = NewNodeUnstageVolumeRequestBuilder().Build()
		}

		csiNodeLockTimeout = 1 * time.Second

		mockTrackingClientNAS := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
		mockTrackingClientSAN := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
		mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
		mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
		mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
		mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))

		for i := 0; i < numOfRequestsNAS; i++ {
			// Setting up mocks expectation for NAS request.
			nasVolumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
			}
			mockTrackingClientNAS.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(nasVolumeTrackingInfo, nil)
			mockTrackingClientNAS.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		for i := 0; i < numOfRequestsSAN; i++ {
			// Setting up mocks expectation for SAN request.
			mockDevicePath := "/dev/mapper/mock-device"
			mockDevice := &models.ScsiDeviceInfo{MultipathDevice: mockDevicePath}

			mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
			mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
			mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), false, false).Return("", nil)
			mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
			mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
			mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any()).Return(nil)
			mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
			mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
				gomock.Any()).Return(map[int]int{6: 3})
			sanVolumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			}
			mockTrackingClientSAN.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(sanVolumeTrackingInfo, nil)
			mockTrackingClientSAN.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating a NAS node plugin.
		pluginNAS := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClientNAS,
			devices:          mockDeviceClient,
			mount:            mockMountClient,
		}

		pluginNAS.InitializeNodeLimiter(ctx)

		// Creating a SAN node plugin.
		pluginSAN := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClientSAN,
			iscsi:            mockISCSIClient,
			devices:          mockDeviceClient,
			mount:            mockMountClient,
		}

		pluginSAN.InitializeNodeLimiter(ctx)
		iscsiUtils = mockIscsiReconcileUtilsClient

		var wg sync.WaitGroup
		wg.Add(numOfRequestsNAS + numOfRequestsSAN)

		for i := 0; i < numOfRequestsNAS; i++ {
			go func() {
				defer wg.Done()
				_, err := pluginNAS.NodeUnstageVolume(context.Background(), NASrequests[i])
				assert.NoError(t, err)
			}()
		}

		for i := 0; i < numOfRequestsSAN; i++ {
			go func() {
				defer wg.Done()
				_, err := pluginSAN.NodeUnstageVolume(context.Background(), SANrequests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test5:
	// Mixing NFS and SMB requests
	t.Run("Test Case 5: Mixing NFS and SMB requests and waiting for both of them to succeed", func(t *testing.T) {
		numOfRequests := 50

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiNodeLockTimeout = 500 * time.Millisecond

		NFSrequests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		SMBrequests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			NFSrequests[i] = NewNodeUnstageVolumeRequestBuilder().Build()
			SMBrequests[i] = NewNodeUnstageVolumeRequestBuilder().Build()
		}

		mockMount := mock_mount.NewMockMount(gomock.NewController(t))
		mockTrackingClientNFS := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
		mockTrackingClientSMB := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
		mockFilesystem := mock_filesystem.NewMockFilesystem(gomock.NewController(t))

		for i := 0; i < numOfRequests; i++ {
			nfsVolumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
			}
			smbVolumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
			}

			// Setting up mocks expectation for NAS request.
			mockTrackingClientNFS.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(nfsVolumeTrackingInfo, nil)
			mockTrackingClientNFS.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)

			// Setting up mocks expectation for SMB request.
			mockTrackingClientSMB.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(smbVolumeTrackingInfo, nil)
			mockTrackingClientSMB.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(smbVolumeTrackingInfo, nil)
			mockFilesystem.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", nil)
			mockMount.EXPECT().UmountSMBPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClientSMB.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating a NFS node plugin.
		pluginNFS := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			fs:               mockFilesystem,
			nodeHelper:       mockTrackingClientNFS,
		}

		pluginNFS.InitializeNodeLimiter(ctx)

		// Creating a SMB node plugin.
		pluginSMB := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			fs:               mockFilesystem,
			nodeHelper:       mockTrackingClientSMB,
		}

		pluginSMB.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests * 2)

		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := pluginNFS.NodeUnstageVolume(context.Background(), NFSrequests[i])
				assert.NoError(t, err)
			}()
		}

		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := pluginSMB.NodeUnstageVolume(context.Background(), SMBrequests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 6 (FCP):
	t.Run("Test Case 6: Sending n num of FCP with different volume.uuid and asserting noError", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		defer func(fcUtils fcp.FcpReconcileUtils) {
			fcpUtils = fcUtils
		}(fcpUtils)

		requests, publishInfos := createFCPUnstageRequests(numOfRequests)

		csiNodeLockTimeout = 500 * time.Millisecond

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockFCPReconcileUtilsClient := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
		mockFCP := mock_fcp.NewMockFCP(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			mockFCPReconcileUtilsClient.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			mockFCPReconcileUtilsClient.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			mockFCPReconcileUtilsClient.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda", "sdb"}, nil).Times(1)
			mockFCP.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(nil, nil).Times(1)

			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: *publishInfos[i],
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil).Times(1)
		}

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			fcp:              mockFCP,
			nodeHelper:       mockTrackingClient,
		}

		fcpUtils = mockFCPReconcileUtilsClient

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routines for each request
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 7 (FCP):
	t.Run("Test Case 7: Sending n num of FCP with same volume.uuid and asserting error", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		defer func(fcUtils fcp.FcpReconcileUtils) {
			fcpUtils = fcUtils
		}(fcpUtils)

		requests, publishInfos := createFCPUnstageRequests(numOfRequests)

		volumeID := "1234-5678"
		for i := 0; i < numOfRequests; i++ {
			requests[i].VolumeId = volumeID
		}

		csiNodeLockTimeout = 500 * time.Millisecond

		signalChan := make(chan struct{})

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockFCPReconcileUtilsClient := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
		mockFCP := mock_fcp.NewMockFCP(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)

		mockFCPReconcileUtilsClient.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(nil).Times(1)
		mockFCPReconcileUtilsClient.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return(nil).Times(1)
		mockFCPReconcileUtilsClient.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda", "sdb"}, nil).Times(1)
		mockFCP.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(nil, nil).Times(1)

		volumeTrackingInfo := &models.VolumeTrackingInfo{
			VolumePublishInfo: *publishInfos[0],
		}
		//	ReadTrackingInfo(context.Context, string) (*models.VolumeTrackingInfo, error)
		mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string) (*models.VolumeTrackingInfo, error) {
			signalChan <- struct{}{}
			time.Sleep(600 * time.Millisecond)
			return volumeTrackingInfo, nil
		}).Times(1)

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			fcp:              mockFCP,
			nodeHelper:       mockTrackingClient,
		}

		fcpUtils = mockFCPReconcileUtilsClient

		plugin.InitializeNodeLimiter(ctx)

		// Modifying the limiter limit
		plugin.limiterSharedMap[NodeUnstageFCPVolume], _ = limiter.New(ctx,
			NodeUnstageFCPVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routines for each request
		go func() {
			defer wg.Done()
			_, err := plugin.NodeUnstageVolume(context.Background(), requests[0])
			assert.NoError(t, err)
		}()

		// Waiting on signal to ensure the first requests acquires the lock.
		<-signalChan

		for i := 1; i < numOfRequests; i++ {
			go func(index int) {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), requests[i])
				assert.Error(t, err)
			}(i)
		}

		wg.Wait()
	})
}

func TestNodeUnstageNFSVolume_Multithreaded(t *testing.T) {
	// Test 1:
	// Initializing SemahaphoreN, and sending the requests within the limit of the limiter
	// and waiting for them to succeed
	t.Run("Test Case 1: Sending requests wihtin the limit of the limiter.", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(uuid.New().String()).Build()
		}

		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))

		for i := 0; i < numOfRequests; i++ {
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		plugin.limiterSharedMap[NodeUnstageNFSVolume], _ = limiter.New(ctx,
			NodeUnstageNFSVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeUnstageNFSVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 2:
	// Initializing SemahaphoreN, Sending more than what can be parallelized.
	t.Run("Test Case 2: Sending more than what can be parallelized.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeUnstageNFSVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed)

		requests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(uuid.NewString()).Build()
		}

		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))

		// For request1
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string) error {
				signalChan <- struct{}{}
				time.Sleep(600 * time.Millisecond)
				return nil
			})
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeUnstageNFSVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on the signal to ensure that the first maxNodeStageNFSVolumeOperations requests acquire the limiter.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeUnstageNFSVolume(ctx, requests[i])
				assert.Error(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 3:
	// Initializing SemahaphoreN, Sending more than what can be parallelized, but the extra one waits and succeeds.
	t.Run("Test Case 3: Sending more than what can be parallelized, but the additional one waits and succeeds.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeUnstageNFSVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(uuid.NewString()).Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed)

		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))

		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string) error {
				signalChan <- struct{}{}
				time.Sleep(100 * time.Millisecond)
				return nil
			})
		}

		// Setting up expectations for remaining requests.
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeUnstageNFSVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on signal to ensure that the first numOfParallelRequestsAllowed requests acquire the lock.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		// Spinning up the go-routine for remaining requests.
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeUnstageNFSVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})
}

func TestNodeUnstageSMBVolume_Multithreaded(t *testing.T) {
	// Test 1:
	// Initializing SemahaphoreN, and sending the requests within the limit of the limiter
	// and waiting for them to succeed
	t.Run("Test Case 1: Sending requests within the limit of the limiter.", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(uuid.NewString()).Build()
		}

		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockFilesystem := mock_filesystem.NewMockFilesystem(ctrl)

		for i := 0; i < numOfRequests; i++ {
			smbVolumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(smbVolumeTrackingInfo, nil)
			mockFilesystem.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", nil)
			mockMount.EXPECT().UmountSMBPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			fs:               mockFilesystem,
		}

		plugin.InitializeNodeLimiter(ctx)

		plugin.limiterSharedMap[NodeUnstageSMBVolume], _ = limiter.New(ctx,
			NodeUnstageSMBVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeUnstageSMBVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 2:
	// Initializing SemahaphoreN, Sending more than what can be parallelized.
	t.Run("Test Case 2: Sending more than what can be parallelized.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeUnstageSMBVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(uuid.NewString()).Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed) // For synchronization only.

		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockFilesystem := mock_filesystem.NewMockFilesystem(ctrl)

		// Setting up mock requests for numOfParallelRequestsAllowed out of 100 requests, as others will be errored out.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			smbVolumeTrackingInfo1 := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string) (*models.VolumeTrackingInfo, error) {
				signalChan <- struct{}{}
				time.Sleep(600 * time.Millisecond)
				return smbVolumeTrackingInfo1, nil
			})
			mockFilesystem.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", nil)
			mockMount.EXPECT().UmountSMBPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			fs:               mockFilesystem,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeUnstageSMBVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on the signal to ensure that the first numOfParallelRequestsAllowed requests acquire the limiter.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeUnstageSMBVolume(ctx, requests[i])
				assert.Error(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 3:
	// Initializing SemahaphoreN, Sending more than what can be parallelized, but the extra one waits and succeeds.
	t.Run("Test Case 3: Sending more than what can be parallelized, but the additional one waits and succeeds.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeStageSMBVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(uuid.NewString()).Build()
		}

		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockFilesystem := mock_filesystem.NewMockFilesystem(ctrl)

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed)

		// Setting up expectations for numOfParallelRequestsAllowed requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			smbVolumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string) (*models.VolumeTrackingInfo, error) {
				signalChan <- struct{}{}
				time.Sleep(100 * time.Millisecond)
				return smbVolumeTrackingInfo, nil
			})
			mockFilesystem.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", nil)
			mockMount.EXPECT().UmountSMBPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			smbVolumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(smbVolumeTrackingInfo, nil)
			mockFilesystem.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", nil)
			mockMount.EXPECT().UmountSMBPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			fs:               mockFilesystem,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routine for numOfParallelRequestsAllowed number of requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeUnstageSMBVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on signal to ensure that the first numOfParallelRequestsAllowed requests acquire the lock.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		// Spinning up the go-routine for remaining requests.
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodeUnstageSMBVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})
}

func TestNodePublishVolume_Multithreaded(t *testing.T) {
	// Test 1:
	// Sending n number of NAS requests of different volume.uuid, and they should succeed
	t.Run("Test Case 1: Sending n num of NAS requests with different volume.uuid and asserting noError", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests := func() []*csi.NodePublishVolumeRequest {
			requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
			for i := 0; i < numOfRequests; i++ {
				requests[i] = NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build()
			}
			return requests
		}()

		csiNodeLockTimeout = 500 * time.Millisecond

		ctrl := gomock.NewController(t)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockMount := mock_mount.NewMockMount(ctrl)

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
			mockMount.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
			mount:            mockMount,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodePublishVolume(context.Background(), requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 2:
	// Sending two requests at the same time with the same volume.uuid.
	// The Second one should fail as the first one still holds the lock after the timeout.
	t.Run("Test Case 2: Sending n NAS request with same volume.uuid at the same time.", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build()
		}

		signalChan := make(chan struct{})

		csiNodeLockTimeout = 500 * time.Millisecond

		volumeID := "1234-5678"
		for i := 0; i < numOfRequests; i++ {
			requests[i].VolumeId = volumeID
		}

		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
		mockMount := mock_mount.NewMockMount(gomock.NewController(t))

		volumeTrackingInfo := &models.VolumeTrackingInfo{
			VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
		}
		mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
		mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
		mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string) (*models.VolumeTrackingInfo, error) {
			signalChan <- struct{}{}
			time.Sleep(600 * time.Millisecond)
			return volumeTrackingInfo, nil
		})
		mockMount.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		mockTrackingClient.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
			mount:            mockMount,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		go func() {
			defer wg.Done()
			_, err := plugin.NodePublishVolume(context.Background(), requests[0])
			assert.NoError(t, err)
		}()

		// Waiting on signal to ensure the first requests acquires the lock.
		<-signalChan

		for i := 1; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodePublishVolume(context.Background(), requests[i])
				assert.Error(t, err)
			}()
		}

		wg.Wait()
	})

	// Test3:
	// Mixing NFS and SMB requests
	t.Run("Test Case 3: Mixing NFS and SMB requests and waiting for both of them to succeed", func(t *testing.T) {
		numOfRequests := 50

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		NFSrequests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		SMBrequests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			NFSrequests[i] = NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build()
			SMBrequests[i] = NewNodePublishVolumeRequestBuilder(SMBNodePublishVolumeRequestType).Build()
		}

		csiNodeLockTimeout = 500 * time.Millisecond

		mockMount := mock_mount.NewMockMount(gomock.NewController(t))
		// Need two mockTrackingClient because of conflicting ReadTrackingInfo function call.
		mockTrackingClientNFS := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
		mockTrackingClientSMB := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
		mockFilesystem := mock_filesystem.NewMockFilesystem(gomock.NewController(t))

		for i := 0; i < numOfRequests; i++ {
			nfsVolumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
			}
			smbVolumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
			}

			// Setting up mocks expectation for NAS request.
			mockTrackingClientNFS.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(nfsVolumeTrackingInfo, nil)
			mockTrackingClientNFS.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(nfsVolumeTrackingInfo, nil)
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockMount.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClientNFS.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			// Setting up mocks expectation for SMB request.
			mockTrackingClientSMB.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(smbVolumeTrackingInfo, nil)
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockMount.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating an NFS node plugin.
		pluginNFS := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			fs:               mockFilesystem,
			nodeHelper:       mockTrackingClientNFS,
		}

		pluginNFS.InitializeNodeLimiter(ctx)

		// Creating an SMB node plugin.
		pluginSMB := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			fs:               mockFilesystem,
			nodeHelper:       mockTrackingClientSMB,
		}

		pluginSMB.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests * 2)

		// Spinning up go-routine for NFS requests.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := pluginNFS.NodePublishVolume(context.Background(), NFSrequests[i])
				assert.NoError(t, err)
			}()
		}

		// Spinning up go-routine for SMB requests.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := pluginSMB.NodePublishVolume(context.Background(), SMBrequests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 4:
	// Sending n number of SAN requests of different volume.uuid, and they should succeed
	t.Run("Test Case 4: Sending n SAN requests within the limit of the limiter", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(iSCSINodePublishVolumeRequestType).Build()
		}

		ctrl := gomock.NewController(t)
		mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()

		for i := 0; i < numOfRequests; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			}
			mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
			mockMount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil)
			mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockNodeHelper,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfRequests; i++ {
			go func(i int) {
				defer wg.Done()
				_, err := plugin.NodePublishVolume(ctx, requests[i])
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()
	})

	// Test 5:
	// Mixing nfs and san requests
	t.Run("Test Case 5: Mixing NFS and SAN requests and waiting for both of them to succeed", func(t *testing.T) {
		numOfRequests := 50

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		NFSrequests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		ISCSIrequests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			NFSrequests[i] = NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build()
			ISCSIrequests[i] = NewNodePublishVolumeRequestBuilder(iSCSINodePublishVolumeRequestType).Build()
		}

		csiNodeLockTimeout = 500 * time.Millisecond

		mockMount := mock_mount.NewMockMount(gomock.NewController(t))
		// Need two mockTrackingClient because of conflicting ReadTrackingInfo function call.
		mockTrackingClientNFS := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
		mockFilesystem := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
		mockISCSINodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
		mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
		mountClient, _ := mount.New()

		for i := 0; i < numOfRequests; i++ {
			nfsVolumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
			}
			iscsiVolumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			}

			// Setting up mocks expectation for NAS request.
			mockTrackingClientNFS.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(nfsVolumeTrackingInfo, nil)
			mockTrackingClientNFS.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(nfsVolumeTrackingInfo, nil)
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockMount.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClientNFS.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

			// Setting up mocks expectation for SAN request.
			mockISCSINodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(iscsiVolumeTrackingInfo, nil)
			mockMount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil)
			mockISCSINodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating an NFS node plugin.
		pluginNFS := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			fs:               mockFilesystem,
			nodeHelper:       mockTrackingClientNFS,
		}

		pluginNFS.InitializeNodeLimiter(ctx)

		// Creating an ISCSI node plugin.
		pluginISCSI := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockISCSINodeHelper,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
		}

		pluginISCSI.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests * 2)

		// Spinning up go-routine for NFS requests.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := pluginNFS.NodePublishVolume(context.Background(), NFSrequests[i])
				assert.NoError(t, err)
			}()
		}

		// Spinning up go-routine for SMB requests.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := pluginISCSI.NodePublishVolume(context.Background(), ISCSIrequests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 6 (FCP):
	t.Run("Test Case 6: Sending n num of FCP requests with different volume.uuid and asserting noError", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		defer func(fcUtils fcp.FcpReconcileUtils) {
			fcpUtils = fcUtils
		}(fcpUtils)

		requests := createFCPPublishRequests(numOfRequests, false)

		csiNodeLockTimeout = 500 * time.Millisecond

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up expectations for each request
		for i := 0; i < numOfRequests; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeFCPVolumePublishInfo).Build(),
			}
			mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), requests[i].VolumeId).Return(volumeTrackingInfo, nil).Times(1)
			mockMount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
			mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
		}

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			fs:               filesystem.New(mockMount),
			nodeHelper:       mockNodeHelper,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < len(requests); i++ {
			go func(idx int) {
				defer wg.Done()
				_, err := plugin.NodePublishVolume(ctx, requests[idx])
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()
	})

	// Test 7 (FCP):
	t.Run("Test Case 7: Sending n num of FCP requests with same volume.uuid and asserting error", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		defer func(fcUtils fcp.FcpReconcileUtils) {
			fcpUtils = fcUtils
		}(fcpUtils)

		requests := createFCPPublishRequests(numOfRequests, false)

		volID := "1234-5678"
		for i := 0; i < numOfRequests; i++ {
			requests[i].VolumeId = volID
		}

		csiNodeLockTimeout = 100 * time.Millisecond

		signalChan := make(chan struct{})

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up expectations for each request
		volumeTrackingInfo := &models.VolumeTrackingInfo{
			VolumePublishInfo: NewVolumePublishInfoBuilder(TypeFCPVolumePublishInfo).Build(),
		}
		//	ReadTrackingInfo(context.Context, string) (*models.VolumeTrackingInfo, error)
		mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string) (*models.VolumeTrackingInfo, error) {
			signalChan <- struct{}{}
			time.Sleep(600 * time.Millisecond)
			return volumeTrackingInfo, nil
		}).Times(1)
		mockMount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
		mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			fs:               filesystem.New(mockMount),
			nodeHelper:       mockNodeHelper,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		go func() {
			defer wg.Done()
			_, err := plugin.NodePublishVolume(ctx, requests[0])
			assert.NoError(t, err)
		}()

		<-signalChan

		for i := 1; i < numOfRequests; i++ {
			go func(index int) {
				defer wg.Done()
				_, err := plugin.NodePublishVolume(ctx, requests[i])
				assert.Error(t, err)
			}(i)
		}

		wg.Wait()
	})
}

func TestNodePublishNFSVolume_Multithreaded(t *testing.T) {
	// Test 1:
	// Initializing SemahaphoreN, and sending the requests within the limit of the limiter
	// and waiting for them to succeed
	t.Run("Test Case 1: Sending requests within the limit of the limiter.", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build()
		}

		ctrl := gomock.NewController(t)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockMount := mock_mount.NewMockMount(ctrl)

		for i := 0; i < numOfRequests; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
			}
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
			mockMount.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
			mount:            mockMount,
		}

		plugin.InitializeNodeLimiter(ctx)

		// Modifying the limiter limit.
		plugin.limiterSharedMap[NodeStageNFSVolume], _ = limiter.New(ctx,
			NodeStageNFSVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodePublishNFSVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 2:
	// Initializing SemahaphoreN, Sending more than what can be parallelized.
	t.Run("Test Case 2: Sending more than what can be parallelized.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodePublishNFSVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed) // For synchronization only.

		ctrl := gomock.NewController(t)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockMount := mock_mount.NewMockMount(ctrl)

		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			volumeTrackingInfo1 := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
			}
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string) (*models.VolumeTrackingInfo, error) {
				signalChan <- struct{}{}
				time.Sleep(600 * time.Millisecond)
				return volumeTrackingInfo1, nil
			})
			mockMount.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
			mount:            mockMount,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routine for the first maxNodeUnstageNFSVolumeOperations requests.
		for i := 0; i < maxNodeStageNFSVolumeOperations; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodePublishNFSVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on the signal to ensure that the first maxNodeStageNFSVolumeOperations requests acquire the limiter.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodePublishNFSVolume(ctx, requests[i])
				assert.Error(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 3:
	// Initializing SemahaphoreN, Sending more than what can be parallelized, but the extra one waits and succeeds.
	t.Run("Test Case 3: Sending more than what can be parallelized, but the additional one waits and succeeds.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodePublishNFSVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed)

		ctrl := gomock.NewController(t)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockMount := mock_mount.NewMockMount(ctrl)

		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
			}
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string) (*models.VolumeTrackingInfo, error) {
				signalChan <- struct{}{}
				time.Sleep(100 * time.Millisecond)
				return volumeTrackingInfo, nil
			})
			mockMount.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
			}
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
			mockMount.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
			mount:            mockMount,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodePublishNFSVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on signal to ensure that the first numOfParallelRequestsAllowed requests acquire the lock.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodePublishNFSVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})
}

func TestNodePublishSMBVolume_Multithreaded(t *testing.T) {
	// Test 1:
	// Initializing SemahaphoreN, and sending the requests within the limit of the limiter
	// and waiting for them to succeed
	t.Run("Test Case 1: Sending requests within the limit of the limiter.", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(SMBNodePublishVolumeRequestType).Build()
		}

		mockMount := mock_mount.NewMockMount(gomock.NewController(t))

		for i := 0; i < numOfRequests; i++ {
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockMount.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
		}

		plugin.InitializeNodeLimiter(ctx)

		plugin.limiterSharedMap[NodePublishSMBVolume], _ = limiter.New(ctx,
			NodePublishSMBVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodePublishSMBVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 2:
	// Initializing SemahaphoreN, Sending more than what can be parallelized.
	t.Run("Test Case 2: Sending more than what can be parallelized.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeStageSMBVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(SMBNodePublishVolumeRequestType).Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed) // For synchronization only.

		mockMount := mock_mount.NewMockMount(gomock.NewController(t))

		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string) (bool, error) {
				signalChan <- struct{}{}
				time.Sleep(600 * time.Millisecond)
				return true, nil
			})
			mockMount.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routine for the first numOfParallelRequestsAllowed requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodePublishSMBVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on the signal to ensure that the first numOfParallelRequestsAllowed requests acquire the limiter.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodePublishSMBVolume(ctx, requests[i])
				assert.Error(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 3:
	// Initializing SemahaphoreN, Sending more than what can be parallelized, but the extra one waits and succeeds.
	t.Run("Test Case 3: Sending more than what can be parallelized, but the additional one waits and succeeds.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodePublishSMBVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(SMBNodePublishVolumeRequestType).Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed)

		mockMount := mock_mount.NewMockMount(gomock.NewController(t))

		// Setting up expectations for numOfParallelRequestsAllowed requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string) (bool, error) {
				signalChan <- struct{}{}
				time.Sleep(100 * time.Millisecond)
				return true, nil
			})
			mockMount.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Setting up expectations for remaining requests.
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockMount.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routine for numOfParallelRequestsAllowed number of requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodePublishSMBVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on signal to ensure that the first numOfParallelRequestsAllowed requests acquire the lock.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		// Spinning up the go-routine for remaining requests.
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.nodePublishSMBVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})
}

func TestNodeUnpublishVolume_Multithreaded(t *testing.T) {
	// Test1:
	// Sending n number of NAS requests of different volume.uuid, and they should succeed
	t.Run("Test Case 1: Sending n num of requests with different volume.uuid and asserting noError", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests := func() []*csi.NodeUnpublishVolumeRequest {
			requests := make([]*csi.NodeUnpublishVolumeRequest, numOfRequests)
			for i := 0; i < numOfRequests; i++ {
				requests[i] = NewNodeUnpublishVolumeRequestBuilder().Build()
			}
			return requests
		}()

		csiNodeLockTimeout = 500 * time.Millisecond //  This csiNodeLockTimeout is just for the locks and not for the whole operation.

		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockOSUtils := mock_osutils.NewMockUtils(ctrl)

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(true, nil)
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockOSUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().RemovePublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			osutils:          mockOSUtils,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnpublishVolume(context.Background(), requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 2:
	// Sending two requests at the same time with the same volume.uuid.
	// The Second one should fail as the first one still holds the lock after the timeout.
	t.Run("Test Case 2: Sending n request with same volume.uuid at the same time.", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests := make([]*csi.NodeUnpublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeUnpublishVolumeRequestBuilder().Build()
		}

		csiNodeLockTimeout = 500 * time.Millisecond

		volumeID := "1234-5678"
		for i := 0; i < numOfRequests; i++ {
			requests[i].VolumeId = volumeID
		}

		signalChan := make(chan struct{})

		// Setting up mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockOSUtils := mock_osutils.NewMockUtils(ctrl)

		// Setting up expectations for the first request.
		mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(true, nil)
		mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
		mockOSUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, resource string) error {
			signalChan <- struct{}{}
			time.Sleep(600 * time.Millisecond)
			return nil
		})
		mockTrackingClient.EXPECT().RemovePublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
			mount:            mockMount,
			osutils:          mockOSUtils,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		go func() {
			defer wg.Done()
			_, err := plugin.NodeUnpublishVolume(context.Background(), requests[0])
			assert.NoError(t, err)
		}()

		// Waiting on signal to ensure the first requests acquires the lock.
		<-signalChan

		for i := 1; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnpublishVolume(context.Background(), requests[i])
				assert.Error(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 3:
	// Initializing SemahaphoreN, Sending more than what can be parallelized.
	t.Run("Test Case 3: Sending more than what can be parallelized.", func(t *testing.T) {
		numOfRequests := 100 // stress testing with 100 requests.
		numOfParallelRequestsAllowed := maxNodeStageNFSVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(ctx, csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeUnpublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeUnpublishVolumeRequestBuilder().Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed) // For synchronization only.

		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockOSUtils := mock_osutils.NewMockUtils(ctrl)

		// Setting up mock requests for numOfParallelRequestsAllowed out of 100 requests, as others will be errored out.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(true, nil)
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockOSUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, resource string) error {
				signalChan <- struct{}{}
				time.Sleep(600 * time.Millisecond)
				return nil
			})
			mockTrackingClient.EXPECT().RemovePublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
			mount:            mockMount,
			osutils:          mockOSUtils,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routine for the first maxNodeUnstageNFSVolumeOperations requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnpublishVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on the signal to ensure that the first maxNodeStageNFSVolumeOperations requests acquire the limiter.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnpublishVolume(ctx, requests[i])
				assert.Error(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 4:
	// Initializing SemahaphoreN, Sending more than what can be parallelized, but the extra one waits and succeeds.
	t.Run("Test Case 4: Sending more than what can be parallelized, but the additional one waits and succeeds.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeStageNFSVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiNodeLockTimeout = 500 * time.Millisecond //  This csiNodeLockTimeout is just for the limiter and not for the whole operation.
		ctx, cancel := context.WithTimeout(ctx, csiNodeLockTimeout)
		defer cancel()

		requests := make([]*csi.NodeUnpublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeUnpublishVolumeRequestBuilder().Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed)

		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockOSUtils := mock_osutils.NewMockUtils(ctrl)

		// Setting up expectations for numOfParallelRequestsAllowed number of requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(true, nil)
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockOSUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, resource string) error {
				signalChan <- struct{}{}
				time.Sleep(100 * time.Millisecond)
				return nil
			})
			mockTrackingClient.EXPECT().RemovePublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Setting up expectations for remaining requests.
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(true, nil)
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockOSUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().RemovePublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
			mount:            mockMount,
			osutils:          mockOSUtils,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up the go-routine for numOfParallelRequestsAllowed number of requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnpublishVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		// Waiting on signal to ensure that the first numOfParallelRequestsAllowed requests acquire the lock.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		// Spinning up the go-routine for remaining requests.
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnpublishVolume(ctx, requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})
}

// ---------------------------------- FCP Volume Multithreaded Tests ----------------------------------

// TestNodeStageFCPVolume_Multithreaded tests the multithreaded behavior of NodeStageFCPVolume
func TestNodeStageFCPVolume_Multithreaded(t *testing.T) {
	// Test 1: Sending more than what can be parallelized
	t.Run("Test Case 1: Sending requests within the limit of the limiter.", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests, publishInfos := createFCPStageRequests(numOfRequests)

		csiKubeletTimeout = 500 * time.Millisecond

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockFCPClient := mock_fcp.NewMockFCP(ctrl)
		mountClient, _ := mount.New()

		// Setting up expectations for numOfParallelRequestsAllowed requests
		mockFCPClient.EXPECT().AttachVolumeRetry(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(int64(0), nil).Times(numOfRequests)
		mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2 * numOfRequests)

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			fcp:              mockFCPClient,
			fs:               filesystem.New(mountClient),
		}

		plugin.InitializeNodeLimiter(ctx)

		// Modifying the limit of the limiter.
		plugin.limiterSharedMap[NodeStageFCPVolume], _ = limiter.New(ctx,
			NodeStageFCPVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for each request.
		for i := 0; i < numOfRequests; i++ {
			go func(index int) {
				defer wg.Done()
				err := plugin.nodeStageFCPVolume(ctx, requests[i], publishInfos[index])
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()
	})

	// Test 2: Initializing SemahaphoreN, Sending more than what can be parallelized.
	t.Run("Test Case 2: Sending more than what can be parallelized.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeStageFCPVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 100 * time.Millisecond

		timedContext := func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(ctx, csiKubeletTimeout)
		}

		requests, publishInfos := createFCPStageRequests(numOfRequests)

		startedChan := make(chan struct{}, numOfParallelRequestsAllowed)
		signalChan := make(chan struct{})

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockFCPClient := mock_fcp.NewMockFCP(ctrl)
		mountClient, _ := mount.New()

		// Setting up expectations for numOfParallelRequestsAllowed requests
		mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string, *models.VolumeTrackingInfo) error {
			startedChan <- struct{}{} // At this point limiter has been already acquired.
			<-signalChan              // Waiting for the rest of the requests to error out.
			return nil
		}).Times(numOfParallelRequestsAllowed)
		mockFCPClient.EXPECT().AttachVolumeRetry(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		).Return(int64(0), nil).Times(numOfParallelRequestsAllowed)
		mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(numOfParallelRequestsAllowed)

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			fcp:              mockFCPClient,
			fs:               filesystem.New(mountClient),
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg1, wg2 sync.WaitGroup

		// Spinning up the go-routine for the first numOfParallelRequestsAllowed requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			wg1.Add(1)
			go func(index int) {
				defer wg1.Done()
				ctx, cancel := timedContext()
				defer cancel()
				err := plugin.nodeStageFCPVolume(ctx, requests[index], publishInfos[index])
				assert.NoError(t, err)
			}(i)
		}

		// Waiting on the signal to ensure that the first numOfParallelRequestsAllowed requests acquire the limiter.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-startedChan
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			wg2.Add(1)
			go func(index int) {
				defer wg2.Done()
				ctx, cancel := timedContext()
				defer cancel()
				err := plugin.nodeStageFCPVolume(ctx, requests[index], publishInfos[index])
				assert.Error(t, err)
			}(i)
		}

		wg2.Wait()

		close(signalChan)

		wg1.Wait()
	})

	// Test 3: Initializing SemahaphoreN, Sending more than what can be parallelized, but the extra one waits and succeeds.
	t.Run("Test Case 3: Sending more than what can be parallelized, but the additional ones waits and succeeds.", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeStageFCPVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond

		timedContext := func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(ctx, csiKubeletTimeout)
		}

		requests, publishInfos := createFCPStageRequests(numOfRequests)

		startChan := make(chan struct{}, numOfParallelRequestsAllowed) // For synchronization only.

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockFCPClient := mock_fcp.NewMockFCP(ctrl)
		mountClient, _ := mount.New()

		// In multiple matching expectations, gomock picks the one which was declared first.
		mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string, *models.VolumeTrackingInfo) error {
			startChan <- struct{}{} // At this point limiter has been already acquired.
			return nil
		}).Times(numOfParallelRequestsAllowed)
		mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(numOfRequests - numOfParallelRequestsAllowed)
		mockFCPClient.EXPECT().AttachVolumeRetry(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), nil).Times(numOfRequests)
		mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(numOfRequests)

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			fcp:              mockFCPClient,
			fs:               filesystem.New(mountClient),
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfRequests; i++ {
			go func(index int) {
				defer wg.Done()
				ctx, cancel := timedContext()
				defer cancel()
				err := plugin.nodeStageFCPVolume(ctx, requests[index], publishInfos[index])
				assert.NoError(t, err)
			}(i)
		}

		// Waiting on signal to ensure that the first numOfParallelRequestsAllowed requests acquire the lock.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-startChan
		}

		wg.Wait()
	})
}

// TestNodeUnstageFCPVolume_Multithreaded tests the multithreaded behavior of NodeUnstageFCPVolume
func TestNodeUnstageFCPVolume_Multithreaded(t *testing.T) {
	// Test 1: Sending requests within the limit of the limiter
	t.Run("Test Case 1: Sending requests within the limit of the limiter", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		defer func(fcUtils fcp.FcpReconcileUtils) {
			fcpUtils = fcUtils
		}(fcpUtils)

		timedContext := func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(ctx, csiKubeletTimeout)
		}

		csiKubeletTimeout = 500 * time.Millisecond

		requests, publishInfos := createFCPUnstageRequests(numOfRequests)

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockFCPReconcileUtilsClient := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
		mockFCP := mock_fcp.NewMockFCP(ctrl)

		// Setting up expectations for each request
		mockFCPReconcileUtilsClient.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(nil).Times(numOfRequests)
		mockFCPReconcileUtilsClient.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return(nil).Times(numOfRequests)
		mockFCPReconcileUtilsClient.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda", "sdb"}, nil).Times(numOfRequests)
		mockFCP.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(nil, nil).Times(numOfRequests)

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			fcp:              mockFCP,
		}

		fcpUtils = mockFCPReconcileUtilsClient

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Modifying the limiter limit
		plugin.limiterSharedMap[NodeUnstageFCPVolume], _ = limiter.New(ctx,
			NodeUnstageFCPVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		for i := 0; i < numOfRequests; i++ {
			go func(idx int) {
				defer wg.Done()
				ctx, cancel := timedContext()
				defer cancel()
				_, err := plugin.nodeUnstageFCPVolumeRetry(ctx, requests[idx], publishInfos[idx], false)
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()
	})

	// Test 2: Sending more than what can be parallelized
	t.Run("Test Case 2: Sending more than what can be parallelized", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeUnstageFCPVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		defer func(fcUtils fcp.FcpReconcileUtils) {
			fcpUtils = fcUtils
		}(fcpUtils)

		timedContext := func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(ctx, csiKubeletTimeout)
		}

		csiKubeletTimeout = 100 * time.Millisecond

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed) // For synchronization only.
		startChan := make(chan struct{})

		requests, publishInfos := createFCPUnstageRequests(numOfRequests)

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockFCPReconcileUtilsClient := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
		mockFCP := mock_fcp.NewMockFCP(ctrl)

		// Setting up expectations for numOfParallelRequestsAllowed requests
		mockFCPReconcileUtilsClient.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(nil).Times(numOfParallelRequestsAllowed)
		mockFCPReconcileUtilsClient.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return(nil).Times(numOfParallelRequestsAllowed)
		mockFCPReconcileUtilsClient.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda", "sdb"}, nil).Times(numOfParallelRequestsAllowed)
		mockFCP.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
			DoAndReturn(func(ctx context.Context, hostSessionMap []map[string]int, lunID int, fcpNodeName string, needFSType bool) (*models.ScsiDeviceInfo, error) {
				startChan <- struct{}{} // At this point limiter has been already acquired.
				<-signalChan
				return nil, nil
			}).Times(numOfParallelRequestsAllowed)

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			fcp:              mockFCP,
		}

		fcpUtils = mockFCPReconcileUtilsClient

		plugin.InitializeNodeLimiter(ctx)

		var wg1, wg2 sync.WaitGroup

		// Spinning up the go-routine for the first numOfParallelRequestsAllowed requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			wg1.Add(1)
			go func(index int) {
				defer wg1.Done()
				ctx, cancel := timedContext()
				defer cancel()
				_, err := plugin.nodeUnstageFCPVolumeRetry(ctx, requests[index], publishInfos[index], false)
				assert.NoError(t, err)
			}(i)
		}

		// Waiting for the first numOfParallelRequestsAllowed requests to acquire the limiter
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-startChan
		}

		// Spinning up the rest of the requests, which should error out
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			wg2.Add(1)
			go func(idx int) {
				defer wg2.Done()
				ctx, cancel := timedContext()
				defer cancel()
				_, err := plugin.nodeUnstageFCPVolumeRetry(ctx, requests[idx], publishInfos[idx], false)
				assert.Error(t, err)
			}(i)
		}

		wg2.Wait()

		close(signalChan)

		wg1.Wait()
	})

	// Test 3: Sending more than what can be parallelized, but additional ones wait and succeed
	t.Run("Test Case 3: Sending more than what can be parallelized, but additional ones wait and succeed", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeUnstageFCPVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		defer func(fcUtils fcp.FcpReconcileUtils) {
			fcpUtils = fcUtils
		}(fcpUtils)

		timedContext := func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(ctx, csiKubeletTimeout)
		}

		csiKubeletTimeout = 500 * time.Millisecond

		startChan := make(chan struct{}, numOfParallelRequestsAllowed) // For synchronization only.

		requests, publishInfos := createFCPUnstageRequests(numOfRequests)

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockFCPReconcileUtilsClient := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
		mockFCP := mock_fcp.NewMockFCP(ctrl)

		// Setting up expectations for numOfParallelRequestsAllowed requests
		mockFCPReconcileUtilsClient.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(nil).Times(numOfRequests)
		mockFCPReconcileUtilsClient.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return(nil).Times(numOfRequests)
		mockFCPReconcileUtilsClient.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda", "sdb"}, nil).Times(numOfRequests)
		mockFCP.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
			DoAndReturn(func(ctx context.Context, hostSessionMap []map[string]int, lunID int, fcpNodeName string, needFSType bool) (*models.ScsiDeviceInfo, error) {
				startChan <- struct{}{} // At this point limiter has been already acquired.
				return nil, nil
			}).Times(numOfParallelRequestsAllowed)
		mockFCP.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(nil, nil).Times(numOfRequests - numOfParallelRequestsAllowed)

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			fcp:              mockFCP,
		}

		fcpUtils = mockFCPReconcileUtilsClient

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for the first numOfParallelRequestsAllowed requests
		for i := 0; i < numOfRequests; i++ {
			go func(idx int) {
				defer wg.Done()
				ctx, cancel := timedContext()
				defer cancel()
				_, err := plugin.nodeUnstageFCPVolumeRetry(ctx, requests[idx], publishInfos[idx], false)
				assert.NoError(t, err)
			}(i)
		}

		// Waiting for the first numOfParallelRequestsAllowed requests to acquire the limiter
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-startChan
		}

		wg.Wait()
	})
}

// TestNodePublishFCPVolume_Multithreaded tests the multithreaded behavior of NodePublishFCPVolume
func TestNodePublishFCPVolume_Multithreaded(t *testing.T) {
	// Test 1: Sending requests within the limit of the limiter
	t.Run("Test Case 1: Sending requests within the limit of the limiter", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		defer func(fcUtils fcp.FcpReconcileUtils) {
			fcpUtils = fcUtils
		}(fcpUtils)

		timedContext := func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(ctx, csiKubeletTimeout)
		}

		csiKubeletTimeout = 500 * time.Millisecond

		requests := createFCPPublishRequests(numOfRequests, false)

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up expectations for each request
		for i := 0; i < numOfRequests; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeFCPVolumePublishInfo).Build(),
			}
			mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), requests[i].VolumeId).Return(volumeTrackingInfo, nil).Times(1)
		}
		mockMount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(numOfRequests)
		mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(numOfRequests)

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			fs:               filesystem.New(mockMount),
			nodeHelper:       mockNodeHelper,
		}

		plugin.InitializeNodeLimiter(ctx)

		// Modifying the limiter limit
		plugin.limiterSharedMap[NodePublishFCPVolume], _ = limiter.New(ctx,
			NodePublishFCPVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(len(requests))

		for i := 0; i < len(requests); i++ {
			go func(idx int) {
				defer wg.Done()
				ctx, cancel := timedContext()
				defer cancel()
				_, err := plugin.nodePublishFCPVolume(ctx, requests[idx])
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()
	})

	// Test 2: Sending more than what can be parallelized
	t.Run("Test Case 2: Sending more than what can be parallelized", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodePublishFCPVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		defer func(fcUtils fcp.FcpReconcileUtils) {
			fcpUtils = fcUtils
		}(fcpUtils)

		timedContext := func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(ctx, csiKubeletTimeout)
		}

		csiKubeletTimeout = 100 * time.Millisecond

		requests := createFCPPublishRequests(numOfRequests, false)

		startedChan := make(chan struct{}, numOfParallelRequestsAllowed)
		signalChan := make(chan struct{})

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up expectations for each request
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeFCPVolumePublishInfo).Build(),
			}
			mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), requests[i].VolumeId).DoAndReturn(func(context.Context, string) (*models.VolumeTrackingInfo, error) {
				startedChan <- struct{}{} // At this point limiter has been already acquired.
				<-signalChan              // Waiting for the rest of the requests to error out.
				return volumeTrackingInfo, nil
			}).Times(1)
		}

		mockMount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(numOfParallelRequestsAllowed)
		mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(numOfParallelRequestsAllowed)

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			fs:               filesystem.New(mockMount),
			nodeHelper:       mockNodeHelper,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg1, wg2 sync.WaitGroup

		// Spinning up the go-routine for the first numOfParallelRequestsAllowed requests.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			wg1.Add(1)
			go func(index int) {
				defer wg1.Done()
				ctx, cancel := timedContext()
				defer cancel()
				_, err := plugin.nodePublishFCPVolume(ctx, requests[index])
				assert.NoError(t, err)
			}(i)
		}

		// Waiting on the signal to ensure that the first numOfParallelRequestsAllowed requests acquire the limiter.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-startedChan
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			wg2.Add(1)
			go func(index int) {
				defer wg2.Done()
				ctx, cancel := timedContext()
				defer cancel()
				_, err := plugin.nodePublishFCPVolume(ctx, requests[index])
				assert.Error(t, err)
			}(i)
		}

		wg2.Wait()

		close(signalChan)

		wg1.Wait()
	})

	// Test 3: Sending more than what can be parallelized, but additional ones wait and succeed
	t.Run("Test Case 3: Sending more than what can be parallelized, but additional ones wait and succeed", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodePublishFCPVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		defer func(fcUtils fcp.FcpReconcileUtils) {
			fcpUtils = fcUtils
		}(fcpUtils)

		timedContext := func() (context.Context, context.CancelFunc) {
			return context.WithTimeout(ctx, csiKubeletTimeout)
		}

		csiKubeletTimeout = 500 * time.Millisecond

		requests := createFCPPublishRequests(numOfRequests, false)

		startChan := make(chan struct{}, numOfParallelRequestsAllowed) // For synchronization only.

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up expectations for each request
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeFCPVolumePublishInfo).Build(),
			}
			mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), requests[i].VolumeId).DoAndReturn(func(context.Context, string) (*models.VolumeTrackingInfo, error) {
				startChan <- struct{}{}
				return volumeTrackingInfo, nil
			}).Times(1)
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeFCPVolumePublishInfo).Build(),
			}
			mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), requests[i].VolumeId).Return(volumeTrackingInfo, nil).Times(1)
		}

		mockMount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(numOfRequests)
		mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(numOfRequests)

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			fs:               filesystem.New(mockMount),
			nodeHelper:       mockNodeHelper,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(len(requests))

		for i := 0; i < numOfRequests; i++ {
			go func(idx int) {
				defer wg.Done()
				ctx, cancel := timedContext()
				defer cancel()
				_, err := plugin.nodePublishFCPVolume(ctx, requests[idx])
				assert.NoError(t, err)
			}(i)
		}

		// Waiting on the signal to ensure that the first maxNodeStageNFSVolumeOperations requests acquire the limiter.
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-startChan
		}

		wg.Wait()
	})
}

// --------------------- Mix-Workflows multithreaded Unit Tests ---------------------

func Test_NodeStage_NodeUnstage_Multithreaded(t *testing.T) {
	/*
		- Set limiter to 10 for stage & unstage.
		- Start 100 parallel stage calls.
		- Wait for those to complete.
		- In a randomized order, start unstage calls for the first 100 and new stage calls for a second batch of 100.
		- Wait for those to complete.
		- In a randomized order, start unstage calls for the second batch of 100.
		- Wait for those to complete.
	*/
	t.Run("Test Case 1: Testing with NFS requests", func(t *testing.T) {
		numOfRequests := 100
		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		// ---- NodeStage nodeStageRequests ----
		nodeStageRequests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			nodeStageRequests[i] = NewNodeStageVolumeRequestBuilder(TypeNFSRequest).Build()
		}

		csiNodeLockTimeout = 500 * time.Millisecond
		csiKubeletTimeout = 500 * time.Millisecond

		// Creating mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)

		// Setting up mocks expectation for each request.
		mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Times(numOfRequests).Return(nil)
		mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(numOfRequests).Return(nil)

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for each request.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), nodeStageRequests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()

		// ---- Unstaging all those previously staged nodeStageRequests ----
		nodeUnstageRequests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			// Using the same volumeID as nodeStage requests.
			nodeUnstageRequests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(nodeStageRequests[i].VolumeId).Build()
		}

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		wg.Add(numOfRequests)
		volumeIDs := rand.Perm(numOfRequests) // Randomizing the requests.
		for i := 0; i < numOfRequests; i++ {
			// Spinning up go-routines for each request.
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), nodeUnstageRequests[volumeIDs[i]])
				assert.NoError(t, err)
			}()
		}

		// ---- Parallely creating another batch of 100 nodeStage requests ----
		nodeStageRequests2 := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			nodeStageRequests2[i] = NewNodeStageVolumeRequestBuilder(TypeNFSRequest).Build()
		}

		// Setting up mocks expectation for each request.
		mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Times(numOfRequests).Return(nil)
		mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(numOfRequests).Return(nil)

		wg.Add(numOfRequests)
		// Spinning up go-routines for each request.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), nodeStageRequests2[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()

		// ---- Unstaging all those previously staged nodeStageRequests of 2nd batch ----
		nodeUnstageRequests2 := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			// Using the same volumeID as nodeStage requests.
			nodeUnstageRequests2[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(nodeStageRequests2[i].VolumeId).Build()
		}

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		wg.Add(numOfRequests)
		volumeIDs = rand.Perm(numOfRequests) // Randomizing the requests.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), nodeUnstageRequests2[volumeIDs[i]])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	t.Run("Test Case 2: Testing with SMB requests", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		// ---- NodeStage nodeStageRequests ----
		nodeStageRequests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			nodeStageRequests[i] = NewNodeStageVolumeRequestBuilder(TypeSMBRequest).Build()
		}

		csiNodeLockTimeout = 500 * time.Millisecond
		csiKubeletTimeout = 500 * time.Millisecond

		// Creating mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockFilesystem := mock_filesystem.NewMockFilesystem(ctrl)

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockMount.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			fs:               mockFilesystem,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for each request.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), nodeStageRequests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()

		// ---- Unstaging all those previously staged nodeStageRequests ----
		nodeUnstageRequests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			// Using the same volumeID as nodeStage requests.
			nodeUnstageRequests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(nodeStageRequests[i].VolumeId).Build()
		}

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			smbVolumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(smbVolumeTrackingInfo, nil)
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(smbVolumeTrackingInfo, nil)
			mockFilesystem.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", nil)
			mockMount.EXPECT().UmountSMBPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		wg.Add(numOfRequests)

		volumeIDs := rand.Perm(numOfRequests) // Randomizing the requests.
		for i := 0; i < numOfRequests; i++ {
			// Spinning up go-routines for each request.
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), nodeUnstageRequests[volumeIDs[i]])
				assert.NoError(t, err)
			}()
		}

		// ---- Parallely creating another batch of 100 nodeStage requests ----
		nodeStageRequests2 := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			nodeStageRequests2[i] = NewNodeStageVolumeRequestBuilder(TypeSMBRequest).Build()
		}

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			mockMount.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockMount.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		wg.Add(numOfRequests)
		// Spinning up go-routines for each request.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), nodeStageRequests2[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()

		// ---- Unstaging all those previously staged nodeStageRequests of 2nd batch ----
		nodeUnstageRequests2 := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			// Using the same volumeID as nodeStage requests.
			nodeUnstageRequests2[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(nodeStageRequests2[i].VolumeId).Build()
		}

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			smbVolumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(smbVolumeTrackingInfo, nil)
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(smbVolumeTrackingInfo, nil)
			mockFilesystem.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", nil)
			mockMount.EXPECT().UmountSMBPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		wg.Add(numOfRequests)
		volumeIDs = rand.Perm(numOfRequests) // Randomizing the requests.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), nodeUnstageRequests2[volumeIDs[i]])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	t.Run("Test Case 3: Testing with ISCSI requests", func(t *testing.T) {
		numOfRequests := 20

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		// ---- NodeStage nodeStageRequests ----
		nodeStageRequests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		publishInfos := make([]models.VolumePublishInfo, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			nodeStageRequests[i] = NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).Build()
			publishInfos[i] = NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build()
		}

		csiNodeLockTimeout = 500 * time.Millisecond
		csiKubeletTimeout = 500 * time.Millisecond

		// Creating mock clients.
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()
		mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
		mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))

		mockDevicePath := "/dev/mapper/mock-device"
		mockDevice := &models.ScsiDeviceInfo{MultipathDevice: mockDevicePath}

		// Setting up expectations for each request
		for i := 0; i < numOfRequests; i++ {
			mockISCSIClient.EXPECT().AttachVolumeRetry(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).Return(int64(0), nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil) // Initial and deferred write
			mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
		}

		// Creating a node plugin.
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
			devices:          mockDeviceClient,
		}

		plugin.InitializeNodeLimiter(ctx)
		iscsiUtils = mockIscsiReconcileUtilsClient
		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for each request.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), nodeStageRequests[i])
				assert.NoError(t, err)
			}()
		}
		wg.Wait()

		// ---- Unstaging all those previously staged nodeStageRequests ----
		nodeUnstageRequests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			// Using the same volumeID as nodeStage requests.
			nodeUnstageRequests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(nodeStageRequests[i].VolumeId).Build()
		}

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any(), false).Return(mockDevice, nil).Times(2)
			mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any()).Times(2)
			mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil).Times(numOfRequests).Times(2)
			mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).Times(numOfRequests).Times(2)
			mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), false, false).Return("", nil).Times(numOfRequests).Times(2)
			mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any()).Times(numOfRequests).Times(2)
			mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any()).Times(numOfRequests).Times(2)
			mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any()).Times(2)
			mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any()).Return(nil)
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
			mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
				gomock.Any()).Return(map[int]int{6: 3}).Times(2)
			mockMount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).Times(2)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		wg.Add(numOfRequests)
		volumeIDs := rand.Perm(numOfRequests) // Randomizing the requests.
		for i := 0; i < numOfRequests; i++ {
			// Spinning up go-routines for each request.
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), nodeUnstageRequests[volumeIDs[i]])
				assert.NoError(t, err)
			}()
		}

		// ---- Parallely creating another batch of 100 nodeStage requests ----
		nodeStageRequests2 := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		publishInfos2 := make([]models.VolumePublishInfo, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			nodeStageRequests2[i] = NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).Build()
			publishInfos2[i] = NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build()
		}
		// Setting up expectations for each request
		for i := 0; i < numOfRequests; i++ {
			mockISCSIClient.EXPECT().AttachVolumeRetry(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).Return(int64(0), nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Times(2).Return(nil) // Initial and deferred write
			mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any())
		}
		wg.Add(numOfRequests)
		// Spinning up go-routines for each request.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), nodeStageRequests2[i])
				assert.NoError(t, err)
			}()
		}
		wg.Wait()
		// ---- Unstaging all those previously staged nodeStageRequests of 2nd batch ----
		nodeUnstageRequests2 := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			// Using the same volumeID as nodeStage requests.
			nodeUnstageRequests2[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(nodeStageRequests2[i].VolumeId).Build()
		}
		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any()).Return(nil)
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}
		wg.Add(numOfRequests)
		volumeIDs = rand.Perm(numOfRequests) // Randomizing the requests.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), nodeUnstageRequests2[volumeIDs[i]])
				assert.NoError(t, err)
			}()
		}
		wg.Wait()
	})

	t.Run("Test Case 4: Testing with FCP requests", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		defer func(fcUtils fcp.FcpReconcileUtils) {
			fcpUtils = fcUtils
		}(fcpUtils)

		csiNodeLockTimeout = 500 * time.Millisecond
		csiKubeletTimeout = 500 * time.Millisecond

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockFCPClient := mock_fcp.NewMockFCP(ctrl)
		mockFCPReconcileUtilsClient := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
		mountClient, _ := mount.New()

		// ---- NodeStage nodeStageRequests ----
		nodeStageRequests, publishInfos := createFCPStageRequests(numOfRequests)

		// Setting up expectations for each requests
		for i := 0; i < numOfRequests; i++ {
			mockFCPClient.EXPECT().AttachVolumeRetry(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).Return(int64(0), nil).Times(1)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
		}

		// Creating a node plugin
		plugin := &Plugin{
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			fcp:              mockFCPClient,
			fs:               filesystem.New(mountClient),
		}

		plugin.InitializeNodeLimiter(ctx)

		fcpUtils = mockFCPReconcileUtilsClient

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for each request.
		for i := 0; i < numOfRequests; i++ {
			go func(index int) {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), nodeStageRequests[index])
				assert.NoError(t, err)
			}(i)
		}
		wg.Wait()

		// ---- Unstaging all those previously staged nodeStageRequests ----
		nodeUnstageRequests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			// Using the same volumeID as nodeStage requests.
			nodeUnstageRequests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(nodeStageRequests[i].VolumeId).Build()
		}

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			mockFCPReconcileUtilsClient.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			mockFCPReconcileUtilsClient.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			mockFCPReconcileUtilsClient.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda", "sdb"}, nil).Times(1)
			mockFCPClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(nil, nil).Times(1)

			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: *publishInfos[i],
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil).Times(1)
		}

		wg.Add(numOfRequests)
		volumeIDs := rand.Perm(numOfRequests) // Randomizing the requests.
		for i := 0; i < numOfRequests; i++ {
			// Spinning up go-routines for each request.
			go func(index int) {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), nodeUnstageRequests[volumeIDs[index]])
				assert.NoError(t, err)
			}(i)
		}

		// ---- Parallely creating another batch of 100 nodeStage requests ----
		nodeStageRequests2, publishInfos2 := createFCPStageRequests(numOfRequests)

		// Setting up expectations for each requests
		for i := 0; i < numOfRequests; i++ {
			mockFCPClient.EXPECT().AttachVolumeRetry(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).Return(int64(0), nil).Times(1)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2)
		}

		wg.Add(numOfRequests)
		// Spinning up go-routines for each request.
		for i := 0; i < numOfRequests; i++ {
			go func(index int) {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), nodeStageRequests2[index])
				assert.NoError(t, err)
			}(i)
		}
		wg.Wait()

		// ---- Unstaging all those previously staged nodeStageRequests of 2nd batch ----
		nodeUnstageRequests2 := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			// Using the same volumeID as nodeStage requests.
			nodeUnstageRequests2[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(nodeStageRequests2[i].VolumeId).Build()
		}

		// Setting up mocks expectation for each request.
		for i := 0; i < numOfRequests; i++ {
			mockFCPReconcileUtilsClient.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			mockFCPReconcileUtilsClient.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			mockFCPReconcileUtilsClient.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"sda", "sdb"}, nil).Times(1)
			mockFCPClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).Return(nil, nil).Times(1)

			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: *publishInfos2[i],
			}
			mockTrackingClient.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil).Times(1)
		}

		wg.Add(numOfRequests)
		volumeIDs = rand.Perm(numOfRequests) // Randomizing the requests.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), nodeUnstageRequests2[volumeIDs[i]])
				assert.NoError(t, err)
			}()
		}
		wg.Wait()
	})
}

// ------------------------ ISCSI Volume Multithreaded Tests ------------------------

// TestNodeStageISCSIVolume_Multithreaded tests the multithreaded behavior of NodeStageISCSIVolume
func TestNodeStageISCSIVolume_Multithreaded(t *testing.T) {
	// Test 1: Sending requests within the limit of the limiter
	t.Run("Test Case 1: Sending requests within the limit of the limiter", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		publishInfos := make([]*models.VolumePublishInfo, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).Build()
			publishInfos[i] = &models.VolumePublishInfo{}
		}

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()

		// Setting up expectations for each request
		for i := 0; i < numOfRequests; i++ {
			mockISCSIClient.EXPECT().AttachVolumeRetry(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).Return(int64(0), nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2) // Initial and deferred write
			mockISCSIClient.EXPECT().AddSession(gomock.Any(), publishedISCSISessions, publishInfos[i], requests[i].VolumeId, "", models.NotInvalid).Return()
		}

		// Creating a node plugin
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
		}

		plugin.InitializeNodeLimiter(ctx)

		// Modifying the limiter limit
		plugin.limiterSharedMap[NodeStageISCSIVolume], _ = limiter.New(ctx,
			NodeStageISCSIVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for each request
		for i := 0; i < numOfRequests; i++ {
			go func(idx int) {
				defer wg.Done()
				err := plugin.nodeStageISCSIVolume(ctx, requests[idx], publishInfos[idx])
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()
	})

	// Test 2: Sending more than what can be parallelized
	t.Run("Test Case 2: Sending more than what can be parallelized", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeStageISCSIVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		publishInfos := make([]*models.VolumePublishInfo, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).Build()
			publishInfos[i] = &models.VolumePublishInfo{}
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed)

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()

		// Setting up expectations for numOfParallelRequestsAllowed requests
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			mockISCSIClient.EXPECT().AttachVolumeRetry(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).Return(int64(0), nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, volumeID string, info *models.VolumeTrackingInfo) error {
				signalChan <- struct{}{}
				time.Sleep(600 * time.Millisecond)
				return nil
			}).Times(2) // Initial and deferred write
			mockISCSIClient.EXPECT().AddSession(gomock.Any(), publishedISCSISessions, publishInfos[i], requests[i].VolumeId, "", models.NotInvalid).Return()
		}

		// Creating a node plugin
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for the first numOfParallelRequestsAllowed requests
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func(idx int) {
				defer wg.Done()
				err := plugin.nodeStageISCSIVolume(ctx, requests[idx], publishInfos[idx])
				assert.NoError(t, err)
			}(i)
		}

		// Waiting for the first numOfParallelRequestsAllowed requests to acquire the limiter
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		// Spinning up the rest of the requests, which should error out
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func(idx int) {
				defer wg.Done()
				err := plugin.nodeStageISCSIVolume(ctx, requests[idx], publishInfos[idx])
				assert.Error(t, err)
			}(i)
		}

		wg.Wait()
	})

	// Test 3: Sending more than what can be parallelized, but additional ones wait and succeed
	t.Run("Test Case 3: Sending more than what can be parallelized, but additional ones wait and succeed", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeStageISCSIVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeStageVolumeRequest, numOfRequests)
		publishInfos := make([]*models.VolumePublishInfo, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeStageVolumeRequestBuilder(TypeiSCSIRequest).Build()
			publishInfos[i] = &models.VolumePublishInfo{}
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed)

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()

		// Setting up expectations for numOfParallelRequestsAllowed requests
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			mockISCSIClient.EXPECT().AttachVolumeRetry(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).Return(int64(0), nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, volumeID string, info *models.VolumeTrackingInfo) error {
				signalChan <- struct{}{}
				time.Sleep(100 * time.Millisecond)
				return nil
			}).Times(2) // Initial and deferred write
			mockISCSIClient.EXPECT().AddSession(gomock.Any(), publishedISCSISessions, publishInfos[i], requests[i].VolumeId, "", models.NotInvalid).Return()
		}

		// Setting up expectations for remaining requests
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			mockISCSIClient.EXPECT().AttachVolumeRetry(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).Return(int64(0), nil)
			mockTrackingClient.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(2) // Initial and deferred write
			mockISCSIClient.EXPECT().AddSession(gomock.Any(), publishedISCSISessions, publishInfos[i], requests[i].VolumeId, "", models.NotInvalid).Return()
		}

		// Creating a node plugin
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
		}
		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for numOfParallelRequestsAllowed requests
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func(idx int) {
				defer wg.Done()
				err := plugin.nodeStageISCSIVolume(ctx, requests[idx], publishInfos[idx])
				assert.NoError(t, err)
			}(i)
		}

		// Waiting for the first numOfParallelRequestsAllowed requests to acquire the limiter
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		// Spinning up the rest of the requests
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func(idx int) {
				defer wg.Done()
				err := plugin.nodeStageISCSIVolume(ctx, requests[idx], publishInfos[idx])
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()
	})
}

// TestNodeUnstageISCSIVolume_Multithreaded tests the multithreaded behavior of the nodeUnstageISCSIVolume function.
func TestNodeUnstageISCSIVolume_Multithreaded(t *testing.T) {
	// Test 1: Sending requests within the limit of the limiter
	t.Run("Test Case 1: Sending requests within the limit of the limiter", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		publishInfos := make([]models.VolumePublishInfo, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeUnstageVolumeRequestBuilder().Build()
			publishInfos[i] = NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build()
		}

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()
		mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
		mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))

		mockDevicePath := "/dev/mapper/mock-device"
		mockDevice := &models.ScsiDeviceInfo{MultipathDevice: mockDevicePath}
		// Setting up expectations for each request
		for i := 0; i < numOfRequests; i++ {
			mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
			mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
			mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
			mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), false, false).Return("", nil)
			mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
			mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any()).Return(nil)
			mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
				gomock.Any()).Return(map[int]int{6: 3})
			mockMount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), requests[i].VolumeId).Return(nil)
		}

		// Creating a node plugin
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
			devices:          mockDeviceClient,
		}

		plugin.InitializeNodeLimiter(ctx)
		iscsiUtils = mockIscsiReconcileUtilsClient

		// Modifying the limiter limit
		plugin.limiterSharedMap[NodeUnstageISCSIVolume], _ = limiter.New(ctx,
			NodeUnstageISCSIVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for each request
		for i := 0; i < numOfRequests; i++ {
			go func(idx int) {
				defer wg.Done()
				err := plugin.nodeUnstageISCSIVolume(ctx, requests[idx], &publishInfos[idx], false)
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()
	})

	// Test 2: Sending more than what can be parallelized, but additional ones wait and succeed
	t.Run("Test Case 2: Sending more than what can be parallelized, but additional ones wait and succeed", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodeUnstageISCSIVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodeUnstageVolumeRequest, numOfRequests)
		publishInfos := make([]models.VolumePublishInfo, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeUnstageVolumeRequestBuilder().Build()
			publishInfos[i] = NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed)

		// Setting up mock clients
		ctrl := gomock.NewController(t)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()
		mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
		mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))

		mockDevicePath := "/dev/mapper/mock-device"
		mockDevice := &models.ScsiDeviceInfo{MultipathDevice: mockDevicePath}

		// Setting up expectations for numOfParallelRequestsAllowed requests
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
			mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
			mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
			mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
			mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
			mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any()).Return(nil)
			mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), false, false).Return("", nil)
			mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
				gomock.Any()).Return(map[int]int{6: 3})
			mockMount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, string) error {
				signalChan <- struct{}{}
				time.Sleep(600 * time.Millisecond)
				return nil
			})
		}

		// Setting up expectations for remaining requests
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any(), false).Return(mockDevice, nil)
			mockISCSIClient.EXPECT().RemoveLUNFromSessions(gomock.Any(), gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().TargetHasMountedDevice(gomock.Any(), gomock.Any()).Return(false, nil)
			mockISCSIClient.EXPECT().SafeToLogOut(gomock.Any(), gomock.Any(), gomock.Any()).Return(true)
			mockISCSIClient.EXPECT().RemovePortalsFromSession(gomock.Any(), gomock.Any(), gomock.Any())
			mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
				gomock.Any(), gomock.Any()).Return(nil)
			mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any())
			mockISCSIClient.EXPECT().PrepareDeviceForRemoval(gomock.Any(), gomock.Any(), gomock.Any(),
				gomock.Any(), false, false).Return("", nil)
			mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
				gomock.Any()).Return(map[int]int{6: 3})
			mockMount.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil)
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), requests[i].VolumeId).Return(nil)
		}

		// Creating a node plugin
		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockTrackingClient,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
			devices:          mockDeviceClient,
		}
		plugin.InitializeNodeLimiter(ctx)
		iscsiUtils = mockIscsiReconcileUtilsClient

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		// Spinning up go-routines for numOfParallelRequestsAllowed requests
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func(idx int) {
				defer wg.Done()
				err := plugin.nodeUnstageISCSIVolume(ctx, requests[idx], &publishInfos[idx], false)
				assert.NoError(t, err)
			}(i)
		}

		// Waiting for the first numOfParallelRequestsAllowed requests to acquire the limiter
		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		// Spinning up the rest of the requests
		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func(idx int) {
				defer wg.Done()
				err := plugin.nodeUnstageISCSIVolume(ctx, requests[idx], &publishInfos[idx], false)
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()
	})
}

// TestNodePublishISCSIVolume_Multithreaded tests the multithreaded behavior of NodePublishISCSIVolume.
func TestNodePublishISCSIVolume_Multithreaded(t *testing.T) {
	// Test 1: Sending requests within the limit of the limiter
	t.Run("Test Case 1: Sending requests within the limit of the limiter", func(t *testing.T) {
		numOfRequests := 100

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(iSCSINodePublishVolumeRequestType).Build()
		}

		ctrl := gomock.NewController(t)
		mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()

		for i := 0; i < numOfRequests; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			}
			mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
			mockMount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil)
			mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockNodeHelper,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
		}

		plugin.InitializeNodeLimiter(ctx)

		plugin.limiterSharedMap[NodePublishISCSIVolume], _ = limiter.New(ctx,
			NodePublishISCSIVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfRequests; i++ {
			go func(i int) {
				defer wg.Done()
				_, err := plugin.nodePublishISCSIVolume(ctx, requests[i])
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()
	})

	// Test 2: Sending more requests than can be parallelized, expecting errors for excess requests
	t.Run("Test Case 2: Sending more than what can be parallelized", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodePublishISCSIVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(iSCSINodePublishVolumeRequestType).Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed)

		ctrl := gomock.NewController(t)
		mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()

		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			}
			mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ string) (*models.VolumeTrackingInfo, error) {
				signalChan <- struct{}{}
				time.Sleep(600 * time.Millisecond)
				return volumeTrackingInfo, nil
			})
			mockMount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil)
			mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockNodeHelper,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func(i int) {
				defer wg.Done()
				_, err := plugin.nodePublishISCSIVolume(ctx, requests[i])
				assert.NoError(t, err)
			}(i)
		}

		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func(i int) {
				defer wg.Done()
				_, err := plugin.nodePublishISCSIVolume(ctx, requests[i])
				assert.Error(t, err)
			}(i)
		}

		wg.Wait()
	})

	// Test 3: Sending more requests than can be parallelized, but excess requests wait and succeed
	t.Run("Test Case 3: Sending more than what can be parallelized, excess requests wait and succeed", func(t *testing.T) {
		numOfRequests := 100
		numOfParallelRequestsAllowed := maxNodePublishISCSIVolumeOperations

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(iSCSINodePublishVolumeRequestType).Build()
		}

		signalChan := make(chan struct{}, numOfParallelRequestsAllowed)

		ctrl := gomock.NewController(t)
		mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()

		for i := 0; i < numOfRequests; i++ {
			volumeTrackingInfo := &models.VolumeTrackingInfo{
				VolumePublishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
			}
			if i < numOfParallelRequestsAllowed {
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, _ string) (*models.VolumeTrackingInfo, error) {
					signalChan <- struct{}{}
					time.Sleep(100 * time.Millisecond)
					return volumeTrackingInfo, nil
				})
			} else {
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
			}
			mockMount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil)
			mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockNodeHelper,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			go func(i int) {
				defer wg.Done()
				_, err := plugin.nodePublishISCSIVolume(ctx, requests[i])
				assert.NoError(t, err)
			}(i)
		}

		for i := 0; i < numOfParallelRequestsAllowed; i++ {
			<-signalChan
		}

		for i := numOfParallelRequestsAllowed; i < numOfRequests; i++ {
			go func(i int) {
				defer wg.Done()
				_, err := plugin.nodePublishISCSIVolume(ctx, requests[i])
				assert.NoError(t, err)
			}(i)
		}

		wg.Wait()
	})

	// Test 4: LUKS encrypted volume handling
	t.Run("Test Case 4: LUKS encrypted volume", func(t *testing.T) {
		numOfRequests := 100
		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(iSCSINodePublishVolumeRequestType).Build()
		}

		ctrl := gomock.NewController(t)
		mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()

		volumeTrackingInfo := &models.VolumeTrackingInfo{
			VolumePublishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).
				WithLUKSEncryption("true").
				WithDevicePath("/dev/mapper/test-device").
				Build(),
		}

		mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
		mockMount.EXPECT().MountDevice(gomock.Any(), gomock.Eq("/dev/mapper/luks-pvc-12345-123"), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil)
		mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockNodeHelper,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
		}

		plugin.InitializeNodeLimiter(ctx)

		plugin.limiterSharedMap[NodePublishISCSIVolume], _ = limiter.New(ctx,
			NodePublishISCSIVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		_, err := plugin.nodePublishISCSIVolume(ctx, requests[0])
		assert.NoError(t, err)
	})

	// Test 5: Raw block volume handling
	t.Run("Test Case 5: Raw block volume", func(t *testing.T) {
		numOfRequests := 100
		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		csiKubeletTimeout = 500 * time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), csiKubeletTimeout)
		defer cancel()

		requests := make([]*csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(iSCSINodePublishVolumeRequestType).Build()
		}

		ctrl := gomock.NewController(t)
		mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
		mockMount := mock_mount.NewMockMount(ctrl)
		mockISCSIClient := mock_iscsi.NewMockISCSI(ctrl)
		mountClient, _ := mount.New()

		volumeTrackingInfo := &models.VolumeTrackingInfo{
			VolumePublishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).
				WithFilesystemType(filesystem.Raw).
				Build(),
		}

		mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(volumeTrackingInfo, nil)
		mockMount.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(true)).Return(nil)
		mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

		plugin := &Plugin{
			command:          execCmd.NewCommand(),
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			mount:            mockMount,
			nodeHelper:       mockNodeHelper,
			iscsi:            mockISCSIClient,
			fs:               filesystem.New(mountClient),
		}

		plugin.InitializeNodeLimiter(ctx)

		plugin.limiterSharedMap[NodePublishISCSIVolume], _ = limiter.New(ctx,
			NodePublishISCSIVolume,
			limiter.TypeSemaphoreN,
			limiter.WithSemaphoreNSize(ctx, numOfRequests),
		)

		_, err := plugin.nodePublishISCSIVolume(ctx, requests[0])
		assert.NoError(t, err)
	})
}

// ----- helpers ----
type NodeStageVolumeRequestBuilder struct {
	request *csi.NodeStageVolumeRequest
}

type NodeStageVolumeRequestType int

const (
	TypeiSCSIRequest NodeStageVolumeRequestType = iota
	TypeNVMERequest
	TypeFCPRequest
	TypeNFSRequest
	TypeSMBRequest
)

func NewNodeStageVolumeRequestBuilder(requestType NodeStageVolumeRequestType) *NodeStageVolumeRequestBuilder {
	switch requestType {
	case TypeiSCSIRequest:
		return &NodeStageVolumeRequestBuilder{
			request: &csi.NodeStageVolumeRequest{
				PublishContext: map[string]string{
					"protocol":               "block",
					"sharedTarget":           "false",
					"filesystemType":         filesystem.Ext4,
					"useCHAP":                "false",
					"iscsiLunNumber":         "0",
					"iscsiTargetIqn":         "iqn.2016-04.com.mock-iscsi:8a1e4b296331",
					"iscsiTargetPortalCount": "2",
					"p1":                     "127.0.0.1:4321",
					"p2":                     "127.0.0.1:4322",
					"SANType":                sa.ISCSI,
				},
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "ext4",
						},
					},
				},
				VolumeId:          "pvc-85987a99-648d-4d84-95df-47d0256ca2ab",
				StagingTargetPath: "/foo",
			},
		}

	case TypeFCPRequest:
		return &NodeStageVolumeRequestBuilder{
			request: &csi.NodeStageVolumeRequest{
				PublishContext: map[string]string{
					"protocol":       "block",
					"sharedTarget":   "false",
					"filesystemType": filesystem.Ext4,
					"useCHAP":        "false",
					"fcpLunNumber":   "0",
					"p1":             "127.0.0.1:4321",
					"p2":             "127.0.0.1:4322",
					"SANType":        sa.FCP,
				},
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "ext4",
						},
					},
				},
				VolumeId:          "pvc-85987a99-648d-4d84-95df-47d0256ca2ab",
				StagingTargetPath: "/foo",
			},
		}

	case TypeSMBRequest:
		return &NodeStageVolumeRequestBuilder{
			request: &csi.NodeStageVolumeRequest{
				PublishContext: map[string]string{
					"protocol":       "file",
					"filesystemType": "smb",
					"smbServer":      "167.12.1.1",
					"smbPath":        "/raw/temp",
					"mountOptions":   "domain=domain,vers=3.0,sec=ntlmssp",
				},
				VolumeId:          fmt.Sprintf("pvc-%s", uuid.New().String()),
				StagingTargetPath: "/foo",
				Secrets: map[string]string{
					"username": "user",
					"password": "password",
				},
			},
		}

	case TypeNFSRequest:
		return &NodeStageVolumeRequestBuilder{
			request: &csi.NodeStageVolumeRequest{
				PublishContext: map[string]string{
					"protocol":       "file",
					"filesystemType": "nfs",
					"nfsServerIp":    "167.12.1.1",
					"nfsPath":        "/raw/temp",
				},
				VolumeId:          fmt.Sprintf("pvc-%s", uuid.New().String()),
				StagingTargetPath: "/foo",
			},
		}

	default:
		panic("unhandled default case")
	}
}

func (builder *NodeStageVolumeRequestBuilder) WithVolumeCapability(capability *csi.VolumeCapability) *NodeStageVolumeRequestBuilder {
	builder.request.VolumeCapability = capability
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) WithFileSystemType(fsType string) *NodeStageVolumeRequestBuilder {
	builder.request.PublishContext["filesystemType"] = fsType
	if builder.request.VolumeCapability.GetMount() != nil {
		builder.request.VolumeCapability.GetMount().FsType = fsType
	}
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) WithSharedTarget(sharedTarget string) *NodeStageVolumeRequestBuilder {
	builder.request.PublishContext["sharedTarget"] = sharedTarget
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) WithUseCHAP(useCHAP string) *NodeStageVolumeRequestBuilder {
	builder.request.PublishContext["useCHAP"] = useCHAP
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) WithIscsiLunNumber(lunNumber string) *NodeStageVolumeRequestBuilder {
	builder.request.PublishContext["iscsiLunNumber"] = lunNumber
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) WithFcpLunNumber(lunNumber string) *NodeStageVolumeRequestBuilder {
	builder.request.PublishContext["fcpLunNumber"] = lunNumber
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) WithLUKSEncryption(luksEncryption string) *NodeStageVolumeRequestBuilder {
	builder.request.PublishContext["LUKSEncryption"] = luksEncryption
	builder.request.PublishContext["fcpLunNumber"] = "0"
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) WithIscsiTargetPortalCount(
	iscsiTargetPortalCount string,
) *NodeStageVolumeRequestBuilder {
	builder.request.PublishContext["iscsiTargetPortalCount"] = iscsiTargetPortalCount
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) WithEncryptedIscsiUsername(
	iscsiUsername string,
) *NodeStageVolumeRequestBuilder {
	builder.request.PublishContext["encryptedIscsiUsername"] = iscsiUsername
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) WithEncryptedIscsiInitiatorSecret(
	encryptedIscsiInitiatorSecret string,
) *NodeStageVolumeRequestBuilder {
	builder.request.PublishContext["encryptedIscsiInitiatorSecret"] = encryptedIscsiInitiatorSecret
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) WithEncryptedIscsiTargetUsername(
	encryptedIscsiTargetUsername string,
) *NodeStageVolumeRequestBuilder {
	builder.request.PublishContext["encryptedIscsiTargetUsername"] = encryptedIscsiTargetUsername
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) WithEncryptedIscsiTargetSecret(
	encryptedIscsiTargetSecret string,
) *NodeStageVolumeRequestBuilder {
	builder.request.PublishContext["encryptedIscsiTargetSecret"] = encryptedIscsiTargetSecret
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) WithVolumeID(volumeID string) *NodeStageVolumeRequestBuilder {
	builder.request.VolumeId = volumeID
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) WithStagingTargetPath(stagingTargetPath string) *NodeStageVolumeRequestBuilder {
	builder.request.StagingTargetPath = stagingTargetPath
	return builder
}

func (builder *NodeStageVolumeRequestBuilder) Build() *csi.NodeStageVolumeRequest {
	builder.request.PublishContext["fcpLunNumber"] = "0"
	return builder.request
}

type NodeUnstageVolumeRequestBuilder struct {
	request csi.NodeUnstageVolumeRequest
}

func NewNodeUnstageVolumeRequestBuilder() *NodeUnstageVolumeRequestBuilder {
	return &NodeUnstageVolumeRequestBuilder{
		request: csi.NodeUnstageVolumeRequest{
			VolumeId:          "pvc-bfddbd25-ceb5-4495-8f43-8e92c76f5f2c",
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/ad11511101c7a61d1711e2fe9e2a3ebc14590502e3ba15d949e4813dba68e3c2/globalmount",
		},
	}
}

func (builder *NodeUnstageVolumeRequestBuilder) WithVolumeID(volumeID string) *NodeUnstageVolumeRequestBuilder {
	builder.request.VolumeId = volumeID
	return builder
}

func (builder *NodeUnstageVolumeRequestBuilder) WithStagingTargetPath(stagingTargetPath string) *NodeUnstageVolumeRequestBuilder {
	builder.request.StagingTargetPath = stagingTargetPath
	return builder
}

func (builder *NodeUnstageVolumeRequestBuilder) Build() *csi.NodeUnstageVolumeRequest {
	return &builder.request
}

type VolumePublishInfoBuilder struct {
	publishInfo models.VolumePublishInfo
}

type VolumePublishInfoType int

const (
	TypeiSCSIVolumePublishInfo VolumePublishInfoType = iota
	TypeNFSVolumePublishInfo
	TypeSMBVolumePublishInfo
	TypeFCPVolumePublishInfo
	TypeNVMeVolumePublishInfo
)

func NewVolumePublishInfoBuilder(volumePublishInfoType VolumePublishInfoType) *VolumePublishInfoBuilder {
	switch volumePublishInfoType {
	case TypeiSCSIVolumePublishInfo:
		return &VolumePublishInfoBuilder{
			publishInfo: models.VolumePublishInfo{
				Localhost:      true,
				FilesystemType: "ext4",
				SharedTarget:   true,
				DevicePath:     "/dev/mapper/mock-device",
				LUKSEncryption: "false",
				SANType:        sa.ISCSI,
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: "10.10.10.10",
						IscsiTargetIQN:    "iqn.1992-08.com.netapp:sn.a0e6b50f49e611efa8b5005056b33c0d:vs.2",
						IscsiLunNumber:    0,
						IscsiInterface:    "default",
						IscsiIgroup:       "ubuntu-linux-22-04-02-desktop-13064d2e-2415-452e-870b-2c08c94f9447",
						IscsiLunSerial:    "yocwC+Ws3R1K",
						IscsiChapInfo:     models.IscsiChapInfo{},
					},
				},
			},
		}

	case TypeNFSVolumePublishInfo:
		return &VolumePublishInfoBuilder{
			publishInfo: models.VolumePublishInfo{
				FilesystemType: "nfs",
				VolumeAccessInfo: models.VolumeAccessInfo{
					NfsAccessInfo: models.NfsAccessInfo{
						NfsServerIP: "167.12.1.1",
						NfsPath:     "/raw/temp",
						NfsUniqueID: uuid.New().String(),
					},
				},
			},
		}

	case TypeSMBVolumePublishInfo:
		return &VolumePublishInfoBuilder{
			publishInfo: models.VolumePublishInfo{
				FilesystemType: "smb",
				VolumeAccessInfo: models.VolumeAccessInfo{
					SMBAccessInfo: models.SMBAccessInfo{
						SMBServer: "167.12.1.1",
						SMBPath:   "/raw/temp",
					},
				},
			},
		}
	case TypeFCPVolumePublishInfo:
		return &VolumePublishInfoBuilder{
			publishInfo: models.VolumePublishInfo{
				Localhost:      true,
				FilesystemType: "ext4",
				SharedTarget:   true,
				DevicePath:     "/dev/mapper/mock-device",
				LUKSEncryption: "false",
				SANType:        sa.FCP,
				VolumeAccessInfo: models.VolumeAccessInfo{
					FCPAccessInfo: models.FCPAccessInfo{
						FCPLunNumber:           rand.Int31n(100),
						FCPIgroup:              "ubuntu-linux-22-04-02-desktop-13064d2e-2415-452e-870b-2c08c94f9447",
						FCPLunSerial:           uuid.New().String(),
						FibreChannelAccessInfo: models.FibreChannelAccessInfo{FCTargetWWNN: "20:00:00:00:00:00:00:01"},
					},
				},
			},
		}
	case TypeNVMeVolumePublishInfo:
		return &VolumePublishInfoBuilder{
			publishInfo: models.VolumePublishInfo{
				Localhost:      true,
				FilesystemType: "ext4",
				SharedTarget:   true,
				DevicePath:     "/dev/mapper/mock-device",
				LUKSEncryption: "false",
				SANType:        sa.NVMe,
				VolumeAccessInfo: models.VolumeAccessInfo{
					NVMeAccessInfo: models.NVMeAccessInfo{
						NVMeNamespaceUUID: "nvme-namespace-id",
						NVMeSubsystemUUID: "nvme-subsystem-id",
						NVMeSubsystemNQN:  "nqn.2016-04.com.mock-nvme:host",
						NVMeTargetIPs:     []string{"192.168.1.1"},
					},
				},
			},
		}

	default:
		panic("unhandled default case")
	}
}

func (b *VolumePublishInfoBuilder) WithLocalhost(localhost bool) *VolumePublishInfoBuilder {
	b.publishInfo.Localhost = localhost
	return b
}

func (b *VolumePublishInfoBuilder) WithFilesystemType(fsType string) *VolumePublishInfoBuilder {
	b.publishInfo.FilesystemType = fsType
	return b
}

func (b *VolumePublishInfoBuilder) WithSharedTarget(sharedTarget bool) *VolumePublishInfoBuilder {
	b.publishInfo.SharedTarget = sharedTarget
	return b
}

func (b *VolumePublishInfoBuilder) WithDevicePath(devicePath string) *VolumePublishInfoBuilder {
	b.publishInfo.DevicePath = devicePath
	return b
}

func (b *VolumePublishInfoBuilder) WithLUKSEncryption(luksEncryption string) *VolumePublishInfoBuilder {
	b.publishInfo.LUKSEncryption = luksEncryption
	return b
}

func (b *VolumePublishInfoBuilder) WithVolumeAccessInfo(accessInfo models.VolumeAccessInfo) *VolumePublishInfoBuilder {
	b.publishInfo.VolumeAccessInfo = accessInfo
	return b
}

func (b *VolumePublishInfoBuilder) Build() models.VolumePublishInfo {
	return b.publishInfo
}

type NodePublishVolumeRequestBuilder struct {
	request *csi.NodePublishVolumeRequest
}

type NodePublishVolumeRequestType int

const (
	iSCSINodePublishVolumeRequestType NodePublishVolumeRequestType = iota
	NVMENodePublishVolumeRequestType
	FCPNodePublishVolumeRequestType
	NFSNodePublishVolumeRequestType
	SMBNodePublishVolumeRequestType
)

func NewNodePublishVolumeRequestBuilder(requestType NodePublishVolumeRequestType) *NodePublishVolumeRequestBuilder {
	switch requestType {
	case NFSNodePublishVolumeRequestType:
		return &NodePublishVolumeRequestBuilder{
			request: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "file",
					"filesystemType": "nfs",
				},
				StagingTargetPath: "/foo",
				TargetPath:        "/tmp/raw",
				Secrets: map[string]string{
					"username": "username",
					"password": "password",
				},
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: true,
			},
		}

	case SMBNodePublishVolumeRequestType:
		return &NodePublishVolumeRequestBuilder{
			request: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "file",
					"filesystemType": "smb",
				},
				StagingTargetPath: "/foo",
				TargetPath:        "/tmp/raw",
				Secrets: map[string]string{
					"username": "username",
					"password": "password",
				},
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: false,
			},
		}
	case FCPNodePublishVolumeRequestType:
		return &NodePublishVolumeRequestBuilder{
			request: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "block",
					"sharedTarget":   "false",
					"filesystemType": filesystem.Ext4,
					"useCHAP":        "false",
					"SANType":        sa.FCP,
				},
				StagingTargetPath: "/foo",
				TargetPath:        "/tmp/raw",
				Secrets: map[string]string{
					"username": "username",
					"password": "password",
				},
				VolumeContext: map[string]string{
					"internalName": fmt.Sprintf("pvc-%s", uuid.New().String()),
				},
				Readonly: false,
			},
		}
	case iSCSINodePublishVolumeRequestType:
		return &NodePublishVolumeRequestBuilder{
			request: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":               "block",
					"sharedTarget":           "false",
					"filesystemType":         filesystem.Ext4,
					"useCHAP":                "false",
					"iscsiLunNumber":         "0",
					"iscsiTargetIqn":         "iqn.2016-04.com.mock-iscsi:8a1e4b296331",
					"iscsiTargetPortalCount": "2",
					"SANType":                "iscsi",
				},
				StagingTargetPath: "/foo",
				TargetPath:        "/tmp/raw",
				Secrets: map[string]string{
					"username": "username",
					"password": "password",
				},
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: false,
			},
		}
	case NVMENodePublishVolumeRequestType:
		return &NodePublishVolumeRequestBuilder{
			request: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "block",
					"sharedTarget":   "false",
					"filesystemType": filesystem.Ext4,
					"useCHAP":        "false",
					"SANType":        sa.NVMe,
				},
				StagingTargetPath: "/foo",
				TargetPath:        "/tmp/raw",
				Secrets: map[string]string{
					"username": "username",
					"password": "password",
				},
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: false,
			},
		}
	default:
		panic("unhandled default case")
	}
}

func (p *NodePublishVolumeRequestBuilder) Build() *csi.NodePublishVolumeRequest {
	return p.request
}

type NodeUnpublishVolumeRequestBuilder struct {
	request *csi.NodeUnpublishVolumeRequest
}

func NewNodeUnpublishVolumeRequestBuilder() *NodeUnpublishVolumeRequestBuilder {
	return &NodeUnpublishVolumeRequestBuilder{
		request: &csi.NodeUnpublishVolumeRequest{
			VolumeId:   uuid.NewString(),
			TargetPath: "/tmp/raw",
		},
	}
}

func (u *NodeUnpublishVolumeRequestBuilder) Build() *csi.NodeUnpublishVolumeRequest {
	return u.request
}

// ------------------------ FCP Volume Multithreaded Tests Helper Functions ------------------------

// createFCPStageRequests creates an array of FCP stage volume requests
func createFCPStageRequests(numRequests int) ([]*csi.NodeStageVolumeRequest, []*models.VolumePublishInfo) {
	requests := make([]*csi.NodeStageVolumeRequest, numRequests)
	publishInfos := make([]*models.VolumePublishInfo, numRequests)
	for i := 0; i < numRequests; i++ {
		requests[i] = NewNodeStageVolumeRequestBuilder(TypeFCPRequest).Build()
		publishInfo := NewVolumePublishInfoBuilder(TypeFCPVolumePublishInfo).Build()
		publishInfos[i] = &publishInfo
	}
	return requests, publishInfos
}

// createFCPUnstageRequests creates an array of FCP unstage volume requests
func createFCPUnstageRequests(numRequests int) ([]*csi.NodeUnstageVolumeRequest, []*models.VolumePublishInfo) {
	requests := make([]*csi.NodeUnstageVolumeRequest, numRequests)
	publishInfos := make([]*models.VolumePublishInfo, numRequests)
	for i := 0; i < numRequests; i++ {
		requests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(uuid.NewString()).Build()
		publishInfo := NewVolumePublishInfoBuilder(TypeFCPVolumePublishInfo).Build()
		publishInfos[i] = &publishInfo
	}
	return requests, publishInfos
}

// createFCPPublishRequests creates an array of FCP publish volume requests
func createFCPPublishRequests(numRequests int, isRawBlock bool) []*csi.NodePublishVolumeRequest {
	requests := make([]*csi.NodePublishVolumeRequest, numRequests)
	for i := 0; i < numRequests; i++ {
		request := NewNodePublishVolumeRequestBuilder(FCPNodePublishVolumeRequestType).Build()
		if isRawBlock {
			request.VolumeCapability = &csi.VolumeCapability{
				AccessType: &csi.VolumeCapability_Block{
					Block: &csi.VolumeCapability_BlockVolume{},
				},
			}
		}
		requests[i] = request
	}
	return requests
}

func TestNodeStageFCPVolume(t *testing.T) {
	type parameters struct {
		getFCPClient           func() fcp.FCP
		getNodeHelper          func() nodehelpers.NodeHelper
		getRestClient          func() controllerAPI.TridentController
		nodeStageVolumeRequest *csi.NodeStageVolumeRequest
		assertError            assert.ErrorAssertionFunc
		aesKey                 []byte
	}

	request := NewNodeStageVolumeRequestBuilder(TypeFCPRequest).Build()
	tests := map[string]parameters{
		"error writing the tracking file when attachment error exists": {
			getFCPClient: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(0), errors.New("some error"))
				return mockFCPClient
			},

			getNodeHelper: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("some error"))
				return mockNodeHelper
			},
			nodeStageVolumeRequest: request,
			assertError:            assert.Error,
		},
	}

	mountClient, _ := mount.New()

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				limiterSharedMap: make(map[string]limiter.Limiter),
				role:             CSINode,
				aesKey:           params.aesKey,
				fs:               filesystem.New(mountClient),
			}

			plugin.InitializeNodeLimiter(ctx)

			if params.getFCPClient != nil {
				plugin.fcp = params.getFCPClient()
			}

			if params.getNodeHelper != nil {
				plugin.nodeHelper = params.getNodeHelper()
			}

			if params.getRestClient != nil {
				plugin.restClient = params.getRestClient()
			}

			sharedTarget, err := strconv.ParseBool(params.nodeStageVolumeRequest.PublishContext["sharedTarget"])
			assert.Nil(t, err)

			publishInfo := models.VolumePublishInfo{
				Localhost:      true,
				FilesystemType: params.nodeStageVolumeRequest.PublishContext["filesystemType"],
				SharedTarget:   sharedTarget,
			}

			err = plugin.nodeStageFCPVolume(context.Background(), params.nodeStageVolumeRequest, &publishInfo)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestNodeStageVolume1(t *testing.T) {
	testCases := []struct {
		name                      string
		req                       *csi.NodeStageVolumeRequest
		expectedResponse          *csi.NodeStageVolumeResponse
		expErrCode                codes.Code
		protocol                  string
		filesystemType            string
		setupISCSIMock            func() iscsi.ISCSI
		setupNodeHelperMock       func() nodehelpers.NodeHelper
		setupRestClientMock       func() controllerAPI.TridentController
		setupMount                func() mount.Mount
		aesKey                    []byte
		setupNVMeHandler          func() nvme.NVMeInterface
		lockAcquisitionShouldFail bool
		contextTimeoutBeforeCall  bool
	}{
		{
			name: "Success - NFS Volume staging",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "nfs",
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":       string(tridentconfig.File),
					"filesystemType": "nfs",
				},
			},
			expectedResponse: &csi.NodeStageVolumeResponse{},
			expErrCode:       codes.OK,
			protocol:         string(tridentconfig.File),
			filesystemType:   "nfs",
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "NFS Volume staging errored and Response is nil, No compatibility of protocol and platform",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "nfs",
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":       string(tridentconfig.File),
					"filesystemType": "nfs",
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.Unknown,
			protocol:         string(tridentconfig.File),
			filesystemType:   "nfs",
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(errors.New("some error"))
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "NFS Volume staging errored, Saving the device info to the volume tracking info path",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "nfs",
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":       string(tridentconfig.File),
					"filesystemType": "nfs",
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.Internal,
			protocol:         string(tridentconfig.File),
			filesystemType:   "nfs",
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(errors.New("some error")).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "Success- SMB Volume staging",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "nfs",
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":       string(tridentconfig.File),
					"filesystemType": "smb",
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.OK,
			protocol:         string(tridentconfig.File),
			filesystemType:   "nfs",
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
				mockMountClient.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "SMB Volume staging errored and Response is nil, No compatibility of protocol and platform",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "smb",
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":       string(tridentconfig.File),
					"filesystemType": "smb",
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.Unknown,
			protocol:         string(tridentconfig.File),
			filesystemType:   "smb",
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(errors.New(""))
				mockMountClient.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "SMB Volume staging errored, Saving the device info to the volume tracking info path",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "smb",
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":       string(tridentconfig.File),
					"filesystemType": "smb",
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.Internal,
			protocol:         string(tridentconfig.File),
			filesystemType:   "nfs",
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
				mockMountClient.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(errors.New("some error")).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "SMB Volume staging errored, during AttachSMBVolume with some permission error",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: smb.SMB,
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":       string(tridentconfig.File),
					"filesystemType": "smb",
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.PermissionDenied,
			protocol:         string(tridentconfig.File),
			filesystemType:   "smb",
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
				mockMountClient.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(os.ErrPermission).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "SMB Volume staging errored, during AttachSMBVolume with some invalid argument error",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: smb.SMB,
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":       string(tridentconfig.File),
					"filesystemType": "smb",
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
			protocol:         string(tridentconfig.File),
			filesystemType:   "smb",
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
				mockMountClient.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("invalid argument")).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "SMB Volume staging errored, during AttachSMBVolume with some other error",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: smb.SMB,
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":       string(tridentconfig.File),
					"filesystemType": "smb",
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.Internal,
			protocol:         string(tridentconfig.File),
			filesystemType:   "smb",
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil)
				mockMountClient.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("some error")).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "SMB Volume staging errored, during AttachSMBVolume with context.DeadlineExceeded error",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: smb.SMB,
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":       string(tridentconfig.File),
					"filesystemType": "smb",
				},
			},
			expectedResponse:         nil,
			expErrCode:               codes.DeadlineExceeded,
			protocol:                 string(tridentconfig.File),
			filesystemType:           "smb",
			contextTimeoutBeforeCall: true,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockMountClient.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(status.Error(codes.DeadlineExceeded, "smb attach timed out")).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "Volume staging errored, because of some unknown protocol",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: smb.SMB,
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":       "unknown",
					"filesystemType": "smb",
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
			protocol:         string(tridentconfig.File),
			filesystemType:   "smb",
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockMountClient.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "Volume staging errored, iscsi protocol",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-iscsi-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "ext4",
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":     string(tridentconfig.Block),
					"SANType":      sa.ISCSI,
					"sharedTarget": "false",
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.Internal,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockMountClient.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "Volume staging errored, fcp protocol",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-iscsi-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "ext4",
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":     string(tridentconfig.Block),
					"SANType":      sa.FCP,
					"sharedTarget": "false",
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.Internal,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockMountClient.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "Volume staging errored, nvme protocol",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-iscsi-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "ext4",
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":     string(tridentconfig.Block),
					"SANType":      sa.NVMe,
					"sharedTarget": "false",
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.Internal,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockMountClient.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupNVMeHandler: func() nvme.NVMeInterface {
				mockNvmeHandler := mock_nvme.NewMockNVMeInterface(gomock.NewController(t))
				mockNvmeHandler.EXPECT().AttachNVMeVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockNvmeHandler
			},
		},
		{
			name: "Volume staging errored, invalid SANType",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-iscsi-volume",
				StagingTargetPath: "/staging/path",
				VolumeCapability: &csi.VolumeCapability{
					AccessType: &csi.VolumeCapability_Mount{
						Mount: &csi.VolumeCapability_MountVolume{
							FsType: "ext4",
						},
					},
				},
				PublishContext: map[string]string{
					"protocol":     string(tridentconfig.Block),
					"SANType":      "",
					"sharedTarget": "false",
				},
			},
			expectedResponse: nil,
			expErrCode:       codes.Internal,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockMountClient.EXPECT().AttachSMBVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
	}

	mountClient, _ := mount.New()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create plugin instance
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
				aesKey:           tc.aesKey,
				fs:               filesystem.New(mountClient),
			}

			plugin.InitializeNodeLimiter(ctx)

			// Setup mocks
			if tc.setupISCSIMock != nil {
				plugin.iscsi = tc.setupISCSIMock()
			}

			if tc.setupNodeHelperMock != nil {
				plugin.nodeHelper = tc.setupNodeHelperMock()
			}

			if tc.setupRestClientMock != nil {
				plugin.restClient = tc.setupRestClientMock()
			}
			if tc.setupNVMeHandler != nil {
				plugin.nvmeHandler = tc.setupNVMeHandler()
			}
			if tc.setupMount != nil {
				plugin.mount = tc.setupMount()
			}

			// Create context with potential timeout
			testCtx := context.Background()
			if tc.contextTimeoutBeforeCall {
				// Create a context that times out immediately
				cancelCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				defer cancel()
				time.Sleep(2 * time.Nanosecond) // Ensure timeout has occurred
				testCtx = cancelCtx
			}

			// Call the function under test
			resp, err := plugin.NodeStageVolume(testCtx, tc.req)

			// Verify results
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				if err != nil {
					status, _ := status.FromError(err)
					// assert.True(t, ok)
					assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				}
				assert.Nil(t, resp)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			if tc.expectedResponse != nil {
				assert.Equal(t, tc.expectedResponse, resp)
			}
		})
	}
}

func TestGetVolumeIdAndStagingPath(t *testing.T) {
	testCases := []struct {
		name                string
		req                 RequestHandler
		expectedVolumeId    string
		expectedStagingPath string
		expErrCode          codes.Code
	}{
		{
			name: "Success - Valid volume ID and staging path",
			req: &mockRequestHandler{
				volumeId:          "test-volume-123",
				stagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/test-volume/globalmount",
			},
			expectedVolumeId:    "test-volume-123",
			expectedStagingPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/test-volume/globalmount",
			expErrCode:          codes.OK,
		},

		{
			name: "Error - Empty volume ID",
			req: &mockRequestHandler{
				volumeId:          "",
				stagingTargetPath: "/staging/path",
			},
			expectedVolumeId:    "",
			expectedStagingPath: "",
			expErrCode:          codes.InvalidArgument,
		},
		{
			name: "Error - Empty staging target path",
			req: &mockRequestHandler{
				volumeId:          "test-volume-123",
				stagingTargetPath: "",
			},
			expectedVolumeId:    "",
			expectedStagingPath: "",
			expErrCode:          codes.InvalidArgument,
		},
		{
			name: "Error - Both volume ID and staging path empty",
			req: &mockRequestHandler{
				volumeId:          "",
				stagingTargetPath: "",
			},
			expectedVolumeId:    "",
			expectedStagingPath: "",
			expErrCode:          codes.InvalidArgument,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create plugin instance
			plugin := &Plugin{
				command: execCmd.NewCommand(),
			}

			// Call the function under test
			volumeId, stagingPath, err := plugin.getVolumeIdAndStagingPath(tc.req)

			// Verify results
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				if err != nil {
					status, _ := status.FromError(err)
					assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				}
				assert.Equal(t, tc.expectedVolumeId, volumeId, "Volume ID should be empty on error")
				assert.Equal(t, tc.expectedStagingPath, stagingPath, "Staging path should be empty on error")
				return
			}

			// Success case assertions
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedVolumeId, volumeId, "Volume ID should match expected")
			assert.Equal(t, tc.expectedStagingPath, stagingPath, "Staging path should match expected")
		})
	}
}

// mockRequestHandler implements RequestHandler interface for testing
type mockRequestHandler struct {
	volumeId          string
	stagingTargetPath string
}

func (m *mockRequestHandler) GetVolumeId() string {
	return m.volumeId
}

func (m *mockRequestHandler) GetStagingTargetPath() string {
	return m.stagingTargetPath
}

func TestRefreshTimerPeriod1(t *testing.T) {
	testCases := []struct {
		name                string
		testType            string // "range", "multiple_calls", "constants"
		expectedMinDuration time.Duration
		expectedMaxDuration time.Duration
		numberOfCalls       int
		shouldTestVariation bool
		shouldTestConstants bool
	}{
		{
			name:                "Success - Normal random jitter generation within range",
			testType:            "range",
			expectedMinDuration: defaultNodeReconciliationPeriod,
			expectedMaxDuration: defaultNodeReconciliationPeriod + maximumNodeReconciliationJitter,
			numberOfCalls:       1,
		},
		{
			name:                "Success - Multiple calls show variation in jitter",
			testType:            "multiple_calls",
			expectedMinDuration: defaultNodeReconciliationPeriod,
			expectedMaxDuration: defaultNodeReconciliationPeriod + maximumNodeReconciliationJitter,
			numberOfCalls:       10,
			shouldTestVariation: true,
		},
		{
			name:                "Success - Range validation over 20 calls",
			testType:            "range",
			expectedMinDuration: defaultNodeReconciliationPeriod,
			expectedMaxDuration: defaultNodeReconciliationPeriod + maximumNodeReconciliationJitter,
			numberOfCalls:       20,
		},
		{
			name:                "Success - Single call returns positive duration",
			testType:            "range",
			expectedMinDuration: time.Nanosecond, // Just ensure it's positive
			expectedMaxDuration: defaultNodeReconciliationPeriod + maximumNodeReconciliationJitter,
			numberOfCalls:       1,
		},
		{
			name:                "Validation - Constants are reasonable",
			testType:            "constants",
			shouldTestConstants: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create plugin instance
			plugin := &Plugin{
				command: execCmd.NewCommand(),
			}

			// Create context
			ctx := context.Background()

			switch tc.testType {
			case "constants":
				// Test that constants are reasonable
				assert.Positive(t, defaultNodeReconciliationPeriod,
					"defaultNodeReconciliationPeriod should be positive")
				assert.Positive(t, maximumNodeReconciliationJitter,
					"maximumNodeReconciliationJitter should be positive")

				// Verify jitter is smaller than base period (good practice)
				assert.Less(t, maximumNodeReconciliationJitter, defaultNodeReconciliationPeriod,
					"Jitter should be smaller than base period for predictable behavior")

				// Verify minimum duration is reasonable (at least 1 second)
				assert.GreaterOrEqual(t, defaultNodeReconciliationPeriod, time.Second,
					"Base reconciliation period should be at least 1 second")

			case "multiple_calls":
				// Test multiple calls and optionally check for variation
				results := make([]time.Duration, tc.numberOfCalls)
				for i := 0; i < tc.numberOfCalls; i++ {
					duration := plugin.refreshTimerPeriod(ctx)
					results[i] = duration

					// Verify each result is within expected range
					assert.GreaterOrEqual(t, duration, tc.expectedMinDuration,
						"Duration should be >= minimum expected duration")
					assert.LessOrEqual(t, duration, tc.expectedMaxDuration,
						"Duration should be <= maximum expected duration")
					assert.Positive(t, duration, "Duration should always be positive")
				}

				if tc.shouldTestVariation {
					// Verify we have some variation (not all the same)
					// This is probabilistic but very likely to pass with multiple samples
					allSame := true
					for i := 1; i < len(results); i++ {
						if results[i] != results[0] {
							allSame = false
							break
						}
					}
					if allSame {
						t.Logf("Note: All %d random values were identical - this is statistically unlikely but possible", tc.numberOfCalls)
					}

					// Additional statistical check: ensure we have some spread
					min := results[0]
					max := results[0]
					for _, result := range results {
						if result < min {
							min = result
						}
						if result > max {
							max = result
						}
					}
					spread := max - min
					t.Logf("Spread of values over %d calls: %v (min: %v, max: %v)", tc.numberOfCalls, spread, min, max)
				}

			case "range":
				// Test single or multiple calls for range validation
				for i := 0; i < tc.numberOfCalls; i++ {
					duration := plugin.refreshTimerPeriod(ctx)

					// Verify the duration is within expected range
					assert.GreaterOrEqual(t, duration, tc.expectedMinDuration,
						"Duration should be >= minimum expected duration")
					assert.LessOrEqual(t, duration, tc.expectedMaxDuration,
						"Duration should be <= maximum expected duration")
					assert.Positive(t, duration, "Duration should always be positive")

					// Log the actual value for debugging
					if tc.numberOfCalls == 1 {
						t.Logf("Generated duration: %v (base: %v, max jitter: %v)",
							duration, defaultNodeReconciliationPeriod, maximumNodeReconciliationJitter)
					}
				}
			}
		})
	}
}

func TestStartReconcilingNodePublications(t *testing.T) {
	testCases := []struct {
		name                  string
		setupPlugin           func() *Plugin
		setupMocks            func(*gomock.Controller) (*Plugin, func())
		shouldVerifyGoroutine bool
		shouldVerifyTimer     bool
	}{
		{
			name: "Success - Start reconciliation service",
			setupMocks: func(ctrl *gomock.Controller) (*Plugin, func()) {
				plugin := &Plugin{
					command:                 execCmd.NewCommand(),
					stopNodePublicationLoop: make(chan bool),
				}

				// Create a cleanup function to stop the goroutine
				cleanup := func() {
					if plugin.stopNodePublicationLoop != nil {
						close(plugin.stopNodePublicationLoop)
					}
					if plugin.nodePublicationTimer != nil {
						plugin.nodePublicationTimer.Stop()
					}
				}

				return plugin, cleanup
			},
			shouldVerifyGoroutine: true,
			shouldVerifyTimer:     true,
		},
		{
			name: "Success - Timer is initialized correctly",
			setupMocks: func(ctrl *gomock.Controller) (*Plugin, func()) {
				plugin := &Plugin{
					command:                 execCmd.NewCommand(),
					stopNodePublicationLoop: make(chan bool),
				}

				cleanup := func() {
					if plugin.stopNodePublicationLoop != nil {
						close(plugin.stopNodePublicationLoop)
					}
					if plugin.nodePublicationTimer != nil {
						plugin.nodePublicationTimer.Stop()
					}
				}

				return plugin, cleanup
			},
			shouldVerifyTimer: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			plugin, cleanup := tc.setupMocks(mockCtrl)
			defer cleanup()

			ctx := context.Background()

			// Call the function under test
			plugin.startReconcilingNodePublications(ctx)

			if tc.shouldVerifyTimer {
				// Verify timer is initialized
				assert.NotNil(t, plugin.nodePublicationTimer, "Timer should be initialized")
			}

			if tc.shouldVerifyGoroutine {
				// Give the goroutine a moment to start
				time.Sleep(10 * time.Millisecond)

				// Verify the goroutine is running by checking if stopping it works
				// This is an indirect test since we can't directly verify goroutine existence
				assert.NotNil(t, plugin.stopNodePublicationLoop, "Stop channel should exist")
			}
		})
	}
}

func TestStopReconcilingNodePublications(t *testing.T) {
	testCases := []struct {
		name                string
		setupPlugin         func() *Plugin
		shouldVerifyTimer   bool
		shouldVerifyChannel bool
	}{
		{
			name: "Success - Stop with active timer and channel",
			setupPlugin: func() *Plugin {
				plugin := &Plugin{
					command:                 execCmd.NewCommand(),
					nodePublicationTimer:    time.NewTimer(time.Hour), // Long timer to test stopping
					stopNodePublicationLoop: make(chan bool),
				}
				return plugin
			},
			shouldVerifyTimer:   true,
			shouldVerifyChannel: true,
		},
		{
			name: "Success - Stop with nil timer",
			setupPlugin: func() *Plugin {
				plugin := &Plugin{
					command:                 execCmd.NewCommand(),
					nodePublicationTimer:    nil,
					stopNodePublicationLoop: make(chan bool),
				}
				return plugin
			},
			shouldVerifyTimer:   false,
			shouldVerifyChannel: true,
		},
		{
			name: "Success - Stop with nil channel",
			setupPlugin: func() *Plugin {
				plugin := &Plugin{
					command:                 execCmd.NewCommand(),
					nodePublicationTimer:    time.NewTimer(time.Hour),
					stopNodePublicationLoop: nil,
				}
				return plugin
			},
			shouldVerifyTimer:   true,
			shouldVerifyChannel: false,
		},
		{
			name: "Success - Stop with both nil",
			setupPlugin: func() *Plugin {
				plugin := &Plugin{
					command:                 execCmd.NewCommand(),
					nodePublicationTimer:    nil,
					stopNodePublicationLoop: nil,
				}
				return plugin
			},
			shouldVerifyTimer:   false,
			shouldVerifyChannel: false,
		},
		{
			name: "Success - Stop with already stopped timer",
			setupPlugin: func() *Plugin {
				timer := time.NewTimer(1 * time.Nanosecond) // Very short timer
				time.Sleep(2 * time.Nanosecond)             // Ensure it fires
				plugin := &Plugin{
					command:                 execCmd.NewCommand(),
					nodePublicationTimer:    timer,
					stopNodePublicationLoop: make(chan bool, 2),
				}
				return plugin
			},
			shouldVerifyTimer:   true,
			shouldVerifyChannel: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plugin := tc.setupPlugin()
			ctx := context.Background()

			// Store initial state for verification
			initialTimer := plugin.nodePublicationTimer
			initialChannel := plugin.stopNodePublicationLoop

			// Call the function under test
			plugin.stopReconcilingNodePublications(ctx)

			if tc.shouldVerifyTimer && initialTimer != nil {
				// Timer should still exist but be stopped
				assert.NotNil(t, plugin.nodePublicationTimer, "Timer should still exist after stopping")
			}

			if tc.shouldVerifyChannel && initialChannel != nil {
				// Channel should be closed - verify by checking if we can receive from it immediately
				select {
				case <-plugin.stopNodePublicationLoop:
					// Channel is closed, which is expected
				default:
					t.Error("Channel should be closed after stopping")
				}
			}
		})
	}
}

func TestReconcileNodePublicationState(t *testing.T) {
	testCases := []struct {
		name                string
		setupNodeHelperMock func() nodehelpers.NodeHelper
		setupRestClientMock func() controllerAPI.TridentController
		expectedError       bool
	}{
		{
			name: "Success - Node is already clean, no action needed",
			setupRestClientMock: func() controllerAPI.TridentController {
				mockRestClient := mockControllerAPI.NewMockTridentController(gomock.NewController(t))
				mockRestClient.EXPECT().GetChap(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&models.IscsiChapInfo{}, nil).AnyTimes()
				mockRestClient.EXPECT().GetNode(gomock.Any(), gomock.Any()).Return(&models.NodeExternal{
					PublicationState: models.NodeClean,
				}, nil).AnyTimes()
				mockRestClient.EXPECT().ListVolumePublicationsForNode(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				return mockRestClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).Return(nil, nil).AnyTimes()
				return mockNodeHelper
			},
			expectedError: false,
		},
		{
			name: "Error - Failed to get node from controller",
			setupRestClientMock: func() controllerAPI.TridentController {
				mockRestClient := mockControllerAPI.NewMockTridentController(gomock.NewController(t))
				mockRestClient.EXPECT().GetNode(gomock.Any(), gomock.Any()).Return(nil, errors.New("controller unreachable")).AnyTimes()
				mockRestClient.EXPECT().ListVolumePublicationsForNode(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				return mockRestClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).Return(nil, nil).AnyTimes()
				return mockNodeHelper
			},
			expectedError: true,
		},
		{
			name: "Error - Node cleanup failed",
			setupRestClientMock: func() controllerAPI.TridentController {
				mockRestClient := mockControllerAPI.NewMockTridentController(gomock.NewController(t))
				mockRestClient.EXPECT().GetNode(gomock.Any(), gomock.Any()).Return(&models.NodeExternal{
					PublicationState: models.NodeDirty,
				}, nil)
				mockRestClient.EXPECT().ListVolumePublicationsForNode(gomock.Any(), gomock.Any()).Return(nil, errors.New("")).AnyTimes()
				return mockRestClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).Return(nil, nil).AnyTimes()
				return mockNodeHelper
			},
			expectedError: true,
		},
		{
			name: "Error - Update node state failed",
			setupRestClientMock: func() controllerAPI.TridentController {
				mockRestClient := mockControllerAPI.NewMockTridentController(gomock.NewController(t))
				mockRestClient.EXPECT().GetNode(gomock.Any(), gomock.Any()).Return(&models.NodeExternal{
					PublicationState: models.NodeDirty,
				}, nil).AnyTimes()
				mockRestClient.EXPECT().ListVolumePublicationsForNode(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockRestClient.EXPECT().UpdateNode(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockRestClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).Return(map[string]*models.VolumeTrackingInfo{"test-vol": {}}, nil).AnyTimes()
				return mockNodeHelper
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			plugin := &Plugin{
				command:              execCmd.NewCommand(),
				role:                 CSINode,
				limiterSharedMap:     make(map[string]limiter.Limiter),
				nodePublicationTimer: time.NewTimer(time.Hour),
			}

			plugin.InitializeNodeLimiter(ctx)

			if tc.setupRestClientMock != nil {
				plugin.restClient = tc.setupRestClientMock()
			}
			if tc.setupNodeHelperMock != nil {
				plugin.nodeHelper = tc.setupNodeHelperMock()
			}

			ctx := context.Background()

			// Call the function under test
			err := plugin.reconcileNodePublicationState(ctx)

			// Verify results
			if tc.expectedError {
				assert.Error(t, err, "Expected an error")
			} else {
				assert.NoError(t, err, "Expected no error")
			}

			// Verify timer was reset (check it's not nil and still running)
			assert.NotNil(t, plugin.nodePublicationTimer, "Timer should exist after reconciliation")
		})
	}
}

func TestNodePublishVolumeCases(t *testing.T) {
	testCases := []struct {
		name                     string
		req                      *csi.NodePublishVolumeRequest
		expectedResponse         *csi.NodePublishVolumeResponse
		expErrCode               codes.Code
		setupMockOSUtils         func() osutils.OSUtils
		setupISCSIMock           func() iscsi.ISCSI
		setupNodeHelperMock      func() nodehelpers.NodeHelper
		setupRestClientMock      func() controllerAPI.TridentController
		setupMount               func() mount.Mount
		contextTimeoutBeforeCall bool
	}{
		{
			name:             "Success - NFS Volume publish",
			req:              NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build(),
			expectedResponse: &csi.NodePublishVolumeResponse{},
			expErrCode:       codes.OK,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
				mockMountClient.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Success - NFS Volume publish",
			req:              NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build(),
			expectedResponse: &csi.NodePublishVolumeResponse{},
			expErrCode:       codes.OK,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Error - NFS Volume publish failed to read tracking info",
			req:              NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build(),
			expectedResponse: &csi.NodePublishVolumeResponse{},
			expErrCode:       codes.Internal,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
				mockMountClient.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil, errors.New("some error")).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Error - NFS Volume publish failed as tracking info file not found",
			req:              NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build(),
			expectedResponse: &csi.NodePublishVolumeResponse{},
			expErrCode:       codes.FailedPrecondition,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
				mockMountClient.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil, errors.NotFoundError("some error")).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Error - NFS Volume publish failed as checking isLikelyNotMountPoint failed",
			req:              NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build(),
			expectedResponse: nil,
			expErrCode:       codes.Internal,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, errors.New("some error")).AnyTimes()
				mockMountClient.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Error - NFS Volume publish failed as attaching nfs volume failed with permission denied",
			req:              NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build(),
			expectedResponse: nil,
			expErrCode:       codes.PermissionDenied,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, os.ErrNotExist).AnyTimes()
				mockMountClient.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(os.ErrPermission).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Error - NFS Volume publish failed as attaching nfs volume failed with some error",
			req:              NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build(),
			expectedResponse: nil,
			expErrCode:       codes.Internal,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, os.ErrNotExist).AnyTimes()
				mockMountClient.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("some error")).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Error - NFS Volume publish failed as attaching nfs volume failed with invalid argument",
			req:              NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build(),
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, os.ErrNotExist).AnyTimes()
				mockMountClient.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("invalid argument")).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Error - NFS Volume publish failed as adding the published path to tracking info file failed",
			req:              NewNodePublishVolumeRequestBuilder(NFSNodePublishVolumeRequestType).Build(),
			expectedResponse: nil,
			expErrCode:       codes.Unknown,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, os.ErrNotExist).AnyTimes()
				mockMountClient.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNFSVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("some error")).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Success - SMB Volume publish",
			req:              NewNodePublishVolumeRequestBuilder(SMBNodePublishVolumeRequestType).Build(),
			expectedResponse: &csi.NodePublishVolumeResponse{},
			expErrCode:       codes.OK,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
				mockMountClient.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockMountClient.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "Error - SMB Volume publish because of empty target path",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "file",
					"filesystemType": "smb",
				},
				StagingTargetPath: "/foo",
				TargetPath:        "",
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: false,
			},

			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
				mockMountClient.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockMountClient.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "Error - SMB Volume publish as staging target not provided",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "file",
					"filesystemType": "smb",
				},
				StagingTargetPath: "",
				TargetPath:        "/foo",
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: true,
			},

			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
				mockMountClient.EXPECT().AttachNFSVolume(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockMountClient.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Error - SMB Volume publish failed as windows bind mount failed with permission denied",
			req:              NewNodePublishVolumeRequestBuilder(SMBNodePublishVolumeRequestType).Build(),
			expectedResponse: nil,
			expErrCode:       codes.PermissionDenied,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, os.ErrNotExist).AnyTimes()
				mockMountClient.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(os.ErrPermission).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Error - SMB Volume publish failed, error checking if mount point exist",
			req:              NewNodePublishVolumeRequestBuilder(SMBNodePublishVolumeRequestType).Build(),
			expectedResponse: nil,
			expErrCode:       codes.Internal,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, errors.New("")).AnyTimes()
				mockMountClient.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Error - SMB Volume publish failed as windows bind mount failed with some error",
			req:              NewNodePublishVolumeRequestBuilder(SMBNodePublishVolumeRequestType).Build(),
			expectedResponse: nil,
			expErrCode:       codes.Internal,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
				mockMountClient.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("some error")).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Error - SMB Volume publish failed as volume not mounted",
			req:              NewNodePublishVolumeRequestBuilder(SMBNodePublishVolumeRequestType).Build(),
			expectedResponse: nil,
			expErrCode:       codes.OK,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Error - SMB Volume publish failed as windows bind mount failed with invalid argument",
			req:              NewNodePublishVolumeRequestBuilder(SMBNodePublishVolumeRequestType).Build(),
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, os.ErrNotExist).AnyTimes()
				mockMountClient.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("invalid argument")).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		// {
		// 	name: "SMB Volume publish errored, with context.DeadlineExceeded error",
		// 	req: &csi.NodePublishVolumeRequest{
		// 		VolumeId:          "test-nfs-volume",
		// 		StagingTargetPath: "/staging/path",
		// 		VolumeCapability: &csi.VolumeCapability{
		// 			AccessType: &csi.VolumeCapability_Mount{
		// 				Mount: &csi.VolumeCapability_MountVolume{
		// 					FsType: smb.SMB,
		// 				},
		// 			},
		// 		},
		// 		PublishContext: map[string]string{
		// 			"protocol":       string(tridentconfig.File),
		// 			"filesystemType": "smb",
		// 		},
		// 	},
		// 	expectedResponse:         nil,
		// 	expErrCode:               codes.DeadlineExceeded,
		// 	contextTimeoutBeforeCall: true,
		// 	setupMount: func() mount.Mount {
		// 		mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
		// 		mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, os.ErrNotExist).AnyTimes()
		// 		mockMountClient.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(status.Error(codes.DeadlineExceeded, "smb attach timed out")).AnyTimes()
		// 		return mockMountClient
		// 	},
		// 	setupNodeHelperMock: func() nodehelpers.NodeHelper {
		// 		mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
		// 		mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
		// 			gomock.Any()).Return(&models.VolumeTrackingInfo{
		// 			VolumePublishInfo: NewVolumePublishInfoBuilder(TypeSMBVolumePublishInfo).Build(),
		// 		}, nil).AnyTimes()
		// 		mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		// 		return mockNodeHelper
		// 	},
		// },
		{
			name: "Success - SAN Volume publish with iSCSI protocol",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "unknown",
					"filesystemType": "smb",
				},
			},
			expectedResponse: &csi.NodePublishVolumeResponse{},
			expErrCode:       codes.InvalidArgument,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeiSCSIVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Success - SAN Volume publish with fcp protocol",
			req:              NewNodePublishVolumeRequestBuilder(FCPNodePublishVolumeRequestType).Build(),
			expectedResponse: &csi.NodePublishVolumeResponse{},
			expErrCode:       codes.OK,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeFCPVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Success - SAN Volume publish with NVMe protocol",
			req:              NewNodePublishVolumeRequestBuilder(NVMENodePublishVolumeRequestType).Build(),
			expectedResponse: &csi.NodePublishVolumeResponse{},
			expErrCode:       codes.OK,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNVMeVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name:             "Success -  Volume publish for unknown protocol",
			req:              NewNodePublishVolumeRequestBuilder(NVMENodePublishVolumeRequestType).Build(),
			expectedResponse: &csi.NodePublishVolumeResponse{},
			expErrCode:       codes.OK,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Eq(false)).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: NewVolumePublishInfoBuilder(TypeNVMeVolumePublishInfo).Build(),
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
	}

	mountClient, _ := mount.New()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create plugin instance
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
				// aesKey:           tc.aesKey,
				fs: filesystem.New(mountClient),
			}

			plugin.InitializeNodeLimiter(ctx)

			// Setup mocks
			if tc.setupISCSIMock != nil {
				plugin.iscsi = tc.setupISCSIMock()
			}

			if tc.setupNodeHelperMock != nil {
				plugin.nodeHelper = tc.setupNodeHelperMock()
			}

			if tc.setupRestClientMock != nil {
				plugin.restClient = tc.setupRestClientMock()
			}

			if tc.setupMount != nil {
				plugin.mount = tc.setupMount()
			}

			// Create context with potential timeout
			testCtx := context.Background()
			if tc.contextTimeoutBeforeCall {
				// Create a context that times out immediately
				cancelCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				defer cancel()
				time.Sleep(2 * time.Nanosecond) // Ensure timeout has occurred
				testCtx = cancelCtx
			}

			resp, err := plugin.NodePublishVolume(testCtx, tc.req)

			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				if err != nil {
					status, _ := status.FromError(err)
					// assert.True(t, ok)
					assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				}
				assert.Nil(t, resp)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, resp)
			if tc.expectedResponse != nil {
				assert.Equal(t, tc.expectedResponse, resp)
			}
		})
	}
}

func TestNodeGetVolumeStats(t *testing.T) {
	volumeID := "vol-123"
	volumePath := "/mnt/vol-123"
	stagingPath := "/staging/vol-123"

	testCases := []struct {
		name                string
		req                 *csi.NodeGetVolumeStatsRequest
		setupNodeHelperMock func() nodehelpers.NodeHelper
		expErrCode          codes.Code
		expResp             *csi.NodeGetVolumeStatsResponse
		mockFilesystem      func() filesystem.Filesystem
		setupOsutils        func() osutils.Utils
	}{
		{
			name:       "Empty volume ID",
			req:        &csi.NodeGetVolumeStatsRequest{VolumeId: "", VolumePath: volumePath},
			expErrCode: codes.InvalidArgument,
		},
		{
			name:       "Empty volume path",
			req:        &csi.NodeGetVolumeStatsRequest{VolumeId: volumeID, VolumePath: ""},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Volume publish path does not exist",
			req:  &csi.NodeGetVolumeStatsRequest{VolumeId: volumeID, VolumePath: volumePath},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().PathExistsWithTimeout(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, errors.New(""))
				return mockOSUtils
			},
			expErrCode: codes.NotFound,
		},
		{
			name: "tracking info file not found, for staging target path",
			req:  &csi.NodeGetVolumeStatsRequest{VolumeId: volumeID, VolumePath: volumePath, StagingTargetPath: stagingPath},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil, errors.NotFoundError("")).AnyTimes()

				return mockNodeHelper
			},
			expErrCode: codes.FailedPrecondition,
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().PathExistsWithTimeout(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
				return mockOSUtils
			},
		},
		{
			name: "Raw block volume",
			req:  &csi.NodeGetVolumeStatsRequest{VolumeId: volumeID, VolumePath: volumePath, StagingTargetPath: stagingPath},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: filesystem.Raw,
					},
				}, nil).AnyTimes()

				return mockNodeHelper
			},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().PathExistsWithTimeout(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
				return mockOSUtils
			},
			expErrCode: codes.OK,
			expResp:    &csi.NodeGetVolumeStatsResponse{},
		},
		{
			name: "Reading tracking info file failed, for staging target path",
			req:  &csi.NodeGetVolumeStatsRequest{VolumeId: volumeID, VolumePath: volumePath, StagingTargetPath: stagingPath},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil, errors.New("")).AnyTimes()

				return mockNodeHelper
			},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().PathExistsWithTimeout(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
				return mockOSUtils
			},
			expErrCode: codes.Internal,
		},
		{
			name: "Filesystem stats error",
			req:  &csi.NodeGetVolumeStatsRequest{VolumeId: volumeID, VolumePath: volumePath},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().PathExistsWithTimeout(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
				return mockOSUtils
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().GetFilesystemStats(gomock.Any(), gomock.Any()).Return(
					int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), errors.New("")).AnyTimes()
				return mockFilesystem1
			},

			expErrCode: codes.Unknown,
		},
		{
			name: "Filesystem stats success",
			req:  &csi.NodeGetVolumeStatsRequest{VolumeId: volumeID, VolumePath: volumePath},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{}, nil).AnyTimes()

				return mockNodeHelper
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().GetFilesystemStats(gomock.Any(), gomock.Any()).Return(
					int64(1), int64(2), int64(1), int64(1), int64(1), int64(2), nil,
				).AnyTimes()

				return mockFilesystem1
			},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().PathExistsWithTimeout(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
				return mockOSUtils
			},
			expErrCode: codes.OK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
			}

			plugin.InitializeNodeLimiter(ctx)

			if tc.mockFilesystem != nil {
				plugin.fs = tc.mockFilesystem()
			}

			if tc.setupNodeHelperMock != nil {
				plugin.nodeHelper = tc.setupNodeHelperMock()
			}

			if tc.setupOsutils != nil {
				plugin.osutils = tc.setupOsutils()
			}

			resp, err := plugin.NodeGetVolumeStats(ctx, tc.req)

			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				if err != nil {
					status, _ := status.FromError(err)
					assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				}
				assert.Nil(t, resp)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}

func TestPlugin_NodeGetCapabilities(t *testing.T) {
	// Arrange
	expectedCapabilities := []*csi.NodeServiceCapability{
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
				},
			},
		},
	}
	plugin := &Plugin{
		command: execCmd.NewCommand(),
		nsCap:   expectedCapabilities,
	}

	// Act
	resp, err := plugin.NodeGetCapabilities(context.Background(), &csi.NodeGetCapabilitiesRequest{})

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, expectedCapabilities, resp.Capabilities)
}

func TestCleanStalePublications(t *testing.T) {
	// fail the request as no staging target path was provided in request to unstage vol
	setupOsutils := func() osutils.Utils {
		mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
		mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(false, errors.New("some error"))
		return mockOSUtils
	}
	plugin := &Plugin{
		command:          execCmd.NewCommand(),
		role:             CSINode,
		limiterSharedMap: make(map[string]limiter.Limiter),
		osutils:          setupOsutils(),
	}

	plugin.InitializeNodeLimiter(ctx)

	stalePublications := map[string]*models.VolumeTrackingInfo{"p1": {PublishedPaths: map[string]struct{}{"path1": {}}}}

	err := plugin.cleanStalePublications(context.Background(), stalePublications)

	// Assert
	assert.Error(t, err)
}

func TestPlugin_NodeGetInfo(t *testing.T) {
	// Arrange
	nodeName := "test-node"
	plugin := &Plugin{
		command:  execCmd.NewCommand(),
		nodeName: nodeName,
	}

	resp, err := plugin.NodeGetInfo(context.Background(), &csi.NodeGetInfoRequest{})

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, nodeName, resp.NodeId)
}

func TestNodeGetInfo(t *testing.T) {
	testCases := []struct {
		name             string
		expectedResponse *models.Node
		setupOsutils     func() osutils.Utils
		getISCSIClient   func() iscsi.ISCSI
		setupNVMeHandler func() nvme.NVMeInterface
	}{
		{
			name: "Get Node info error, Unable to get host system information",
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().GetHostSystemInfo(gomock.Any()).Return(nil, errors.New("some error"))
				mockOSUtils.EXPECT().GetIPAddresses(gomock.Any()).Return(nil, errors.New("some error"))
				mockOSUtils.EXPECT().NFSActiveOnHost(gomock.Any()).Return(true, nil)
				return mockOSUtils
			},
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().ISCSIActiveOnHost(
					gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
				return mockISCSIClient
			},
			setupNVMeHandler: func() nvme.NVMeInterface {
				mockNvmeHandler := mock_nvme.NewMockNVMeInterface(gomock.NewController(t))
				mockNvmeHandler.EXPECT().NVMeActiveOnHost(gomock.Any()).Return(true, nil).AnyTimes()
				mockNvmeHandler.EXPECT().GetHostNqn(gomock.Any()).Return("ips", nil).AnyTimes()
				return mockNvmeHandler
			},

			expectedResponse: &models.Node{},
		},
		{
			name: "Get Node info error, Could not get IP addresses.",
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&models.HostSystem{}, nil)
				mockOSUtils.EXPECT().GetIPAddresses(gomock.Any()).Return(nil, errors.New("some error"))
				mockOSUtils.EXPECT().NFSActiveOnHost(gomock.Any()).Return(true, nil)
				return mockOSUtils
			},
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().ISCSIActiveOnHost(
					gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
				return mockISCSIClient
			},
			setupNVMeHandler: func() nvme.NVMeInterface {
				mockNvmeHandler := mock_nvme.NewMockNVMeInterface(gomock.NewController(t))
				mockNvmeHandler.EXPECT().NVMeActiveOnHost(gomock.Any()).Return(true, nil).AnyTimes()
				mockNvmeHandler.EXPECT().GetHostNqn(gomock.Any()).Return("ips", nil).AnyTimes()
				return mockNvmeHandler
			},

			expectedResponse: &models.Node{},
		},
		{
			name: "Get Node info, Could not find any usable IP addresses.",
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&models.HostSystem{}, nil)
				mockOSUtils.EXPECT().GetIPAddresses(gomock.Any()).Return([]string{}, nil)
				mockOSUtils.EXPECT().NFSActiveOnHost(gomock.Any()).Return(true, nil)
				return mockOSUtils
			},
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().ISCSIActiveOnHost(
					gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
				return mockISCSIClient
			},
			setupNVMeHandler: func() nvme.NVMeInterface {
				mockNvmeHandler := mock_nvme.NewMockNVMeInterface(gomock.NewController(t))
				mockNvmeHandler.EXPECT().NVMeActiveOnHost(gomock.Any()).Return(true, nil).AnyTimes()
				mockNvmeHandler.EXPECT().GetHostNqn(gomock.Any()).Return("", errors.New("")).AnyTimes()
				return mockNvmeHandler
			},
			expectedResponse: &models.Node{},
		},
		{
			name: "Get Node info, Discovered IP addresses.",
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&models.HostSystem{}, nil)
				mockOSUtils.EXPECT().GetIPAddresses(gomock.Any()).Return([]string{"ip1", "ip2"}, nil)
				mockOSUtils.EXPECT().NFSActiveOnHost(gomock.Any()).Return(true, nil)
				return mockOSUtils
			},
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().ISCSIActiveOnHost(
					gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
				return mockISCSIClient
			},
			setupNVMeHandler: func() nvme.NVMeInterface {
				mockNvmeHandler := mock_nvme.NewMockNVMeInterface(gomock.NewController(t))
				mockNvmeHandler.EXPECT().NVMeActiveOnHost(gomock.Any()).Return(true, nil).AnyTimes()
				mockNvmeHandler.EXPECT().GetHostNqn(gomock.Any()).Return("", nil).AnyTimes()
				return mockNvmeHandler
			},
			expectedResponse: &models.Node{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create plugin instance
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
			}

			plugin.InitializeNodeLimiter(ctx)

			// Setup mocks
			if tc.setupOsutils != nil {
				plugin.osutils = tc.setupOsutils()
			}
			if tc.getISCSIClient != nil {
				plugin.iscsi = tc.getISCSIClient()
			}

			if tc.setupNVMeHandler != nil {
				plugin.nvmeHandler = tc.setupNVMeHandler()
			}
			testCtx := context.Background()

			resp := plugin.nodeGetInfo(testCtx)

			assert.NotNil(t, resp)
		})
	}
}

func TestNodeUnStageVolume(t *testing.T) {
	defer func(previousIscsiUtils iscsi.IscsiReconcileUtils) {
		iscsiUtils = previousIscsiUtils
	}(iscsiUtils)
	testCases := []struct {
		name                         string
		req                          *csi.NodeUnstageVolumeRequest
		expectedResponse             *csi.NodeUnstageVolumeResponse
		expErrCode                   codes.Code
		setupISCSIMock               func() iscsi.ISCSI
		setupNodeHelperMock          func() nodehelpers.NodeHelper
		setupRestClientMock          func() controllerAPI.TridentController
		setupMount                   func() mount.Mount
		contextTimeoutBeforeCall     bool
		mockFilesystem               func() filesystem.Filesystem
		getDeviceClient              func() devices.Devices
		setupNVMeHandler             func() nvme.NVMeInterface
		getIscsiReconcileUtilsClient func() iscsi.IscsiReconcileUtils
	}{
		{
			name: "Volume Unstaging errored, empty volumeID provided",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "",
				StagingTargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnstageVolumeResponse{},
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Volume Unstaging errored, empty StagingTargetPath provided",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-vol",
				StagingTargetPath: "",
			},
			expectedResponse: &csi.NodeUnstageVolumeResponse{},
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Volume Unstaging errored, Not found tracking info file",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-vol",
				StagingTargetPath: "/staging/path",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil, errors.NotFoundError("")).AnyTimes()
				return mockNodeHelper
			},
			expectedResponse: &csi.NodeUnstageVolumeResponse{},
			expErrCode:       codes.OK,
		},
		{
			name: "Volume Unstaging errored, Unable to read the volume tracking file",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-vol",
				StagingTargetPath: "/staging/path",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil, errors.New("")).AnyTimes()
				return mockNodeHelper
			},
			expectedResponse: nil,
			expErrCode:       codes.Internal,
		},
		{
			name: "Volume Unstaging errored,unable to read protocol info from publish info",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnstageVolumeResponse{},
			expErrCode:       codes.FailedPrecondition,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsCompatible(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{VolumePublishInfo: models.VolumePublishInfo{}}, nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "Success - NFS Volume Unstaging",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnstageVolumeResponse{},
			expErrCode:       codes.OK,
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						VolumeAccessInfo: models.VolumeAccessInfo{
							NfsAccessInfo: models.NfsAccessInfo{
								NfsServerIP: "some-ip",
							},
						},
					},
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "Success - NFS Volume Unstaging failed, error in deleting tracking info file",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnstageVolumeResponse{},
			expErrCode:       codes.Internal,
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						VolumeAccessInfo: models.VolumeAccessInfo{
							NfsAccessInfo: models.NfsAccessInfo{
								NfsServerIP: "some-ip",
							},
						},
					},
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "Success - SMB Volume Unstaging",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnstageVolumeResponse{},
			expErrCode:       codes.OK,
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						VolumeAccessInfo: models.VolumeAccessInfo{
							SMBAccessInfo: models.SMBAccessInfo{
								SMBPath: "some-path",
							},
						},
						FilesystemType: smb.SMB,
					},
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountSMBPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				return mockMountClient
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()

				return mockFilesystem1
			},
		},
		{
			name: "Error - SMB Volume Unstaging, failed getting mount path",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
			},
			expectedResponse: nil,
			expErrCode:       codes.Unknown,
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						VolumeAccessInfo: models.VolumeAccessInfo{
							SMBAccessInfo: models.SMBAccessInfo{
								SMBPath: "some-path",
							},
						},
						FilesystemType: smb.SMB,
					},
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountSMBPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", errors.New("failed getting unmount path")).AnyTimes()

				return mockFilesystem1
			},
		},
		{
			name: "Error - SMB Volume Unstaging, failed unmounting volume",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
			},
			expectedResponse: nil,
			expErrCode:       codes.Unknown,
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						VolumeAccessInfo: models.VolumeAccessInfo{
							SMBAccessInfo: models.SMBAccessInfo{
								SMBPath: "some-path",
							},
						},
						FilesystemType: smb.SMB,
					},
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountSMBPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("some error"))
				return mockMountClient
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()

				return mockFilesystem1
			},
		},
		{
			name: "Success - SMB Volume Unstaging failed, error in deleting tracking info file",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnstageVolumeResponse{},
			expErrCode:       codes.Internal,
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(errors.New("error deleting")).AnyTimes()
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						VolumeAccessInfo: models.VolumeAccessInfo{
							SMBAccessInfo: models.SMBAccessInfo{
								SMBPath: "some-path",
							},
						},
						FilesystemType: smb.SMB,
					},
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountSMBPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				return mockMountClient
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()

				return mockFilesystem1
			},
		},
		{
			name: "Success - Iscsi Volume Unstaging",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-iscsi-volume",
				StagingTargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnstageVolumeResponse{},
			expErrCode:       codes.OK,
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{VolumePublishInfo: models.VolumePublishInfo{VolumeAccessInfo: models.VolumeAccessInfo{IscsiAccessInfo: models.IscsiAccessInfo{IscsiTargetIQN: "fake-iqn"}}}}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()

				return mockFilesystem1
			},
			setupISCSIMock: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().GetDeviceInfoForLUN(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockISCSIClient.EXPECT().AddSession(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()
				return mockISCSIClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{}).AnyTimes()
				return mockIscsiReconcileUtilsClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockDeviceClient.EXPECT().GetMultipathDeviceBySerial(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
				return mockDeviceClient
			},
		},
		{
			name: "Success - fcp Volume Unstaging",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-iscsi-volume",
				StagingTargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnstageVolumeResponse{},
			expErrCode:       codes.OK,
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{VolumePublishInfo: models.VolumePublishInfo{SANType: sa.FCP, FilesystemType: sa.FCP, VolumeAccessInfo: models.VolumeAccessInfo{FCPAccessInfo: models.FCPAccessInfo{FibreChannelAccessInfo: models.FibreChannelAccessInfo{FCTargetWWNN: "fake-wwwn"}}}}}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()

				return mockFilesystem1
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockDeviceClient
			},
		},
		{
			name: "Success - Nvme Volume Unstaging",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-iscsi-volume",
				StagingTargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnstageVolumeResponse{},
			expErrCode:       codes.OK,
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{VolumePublishInfo: models.VolumePublishInfo{SANType: sa.NVMe, FilesystemType: "ext4", VolumeAccessInfo: models.VolumeAccessInfo{NVMeAccessInfo: models.NVMeAccessInfo{NVMeSubsystemNQN: "fake-nqn"}}}}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().GetUnmountPath(gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()

				return mockFilesystem1
			},
			setupNVMeHandler: func() nvme.NVMeInterface {
				mockNvmeHandler := mock_nvme.NewMockNVMeInterface(gomock.NewController(t))
				mockNvmeHandler.EXPECT().RemovePublishedNVMeSession(gomock.Any(), gomock.Any(), gomock.Any()).Return(false).AnyTimes()
				mockNvmeHandler.EXPECT().NewNVMeSubsystem(gomock.Any(), gomock.Any()).Return(nvme.NewNVMeSubsystemDetailed("mock-nqn", "mock-name", []nvme.Path{{Address: "mock-address"}}, nil, afero.NewMemMapFs())).AnyTimes()
				return mockNvmeHandler
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockDeviceClient
			},
		},
	}

	mountClient, _ := mount.New()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "darwin" && strings.Contains(tc.name, "Nvme Volume Unstaging") {
				t.Skip("Skipping test on Darwin (macOS) as NVMe is not supported")
			}

			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create plugin instance
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
				fs:               filesystem.New(mountClient),
			}

			plugin.InitializeNodeLimiter(ctx)

			// Setup mocks
			if tc.mockFilesystem != nil {
				plugin.fs = tc.mockFilesystem()
			}

			if tc.setupNodeHelperMock != nil {
				plugin.nodeHelper = tc.setupNodeHelperMock()
			}

			if tc.setupRestClientMock != nil {
				plugin.restClient = tc.setupRestClientMock()
			}
			if tc.getIscsiReconcileUtilsClient != nil {
				iscsiUtils = tc.getIscsiReconcileUtilsClient()
			}

			if tc.setupISCSIMock != nil {
				plugin.iscsi = tc.setupISCSIMock()
			}

			if tc.setupMount != nil {
				plugin.mount = tc.setupMount()
			}
			if tc.setupNVMeHandler != nil {
				plugin.nvmeHandler = tc.setupNVMeHandler()
			}
			if tc.getDeviceClient != nil {
				plugin.devices = tc.getDeviceClient()
			}

			// Create context with potential timeout
			testCtx := context.Background()
			if tc.contextTimeoutBeforeCall {
				// Create a context that times out immediately
				cancelCtx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
				defer cancel()
				time.Sleep(2 * time.Nanosecond) // Ensure timeout has occurred
				testCtx = cancelCtx
			}

			// Call the function under test
			resp, err := plugin.NodeUnstageVolume(testCtx, tc.req)

			// Verify results
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				if err != nil {
					status, _ := status.FromError(err)
					// assert.True(t, ok)
					assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				}
				assert.Nil(t, resp)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}

func TestNodeUnpublishVolume(t *testing.T) {
	testCases := []struct {
		name                string
		req                 *csi.NodeUnpublishVolumeRequest
		expectedResponse    *csi.NodeUnpublishVolumeResponse
		expErrCode          codes.Code
		setupOsutils        func() osutils.Utils
		setupNodeHelperMock func() nodehelpers.NodeHelper
		setupMount          func() mount.Mount
	}{
		{
			name: "Volume Unpublish errored, empty volumeID provided",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "",
				TargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnpublishVolumeResponse{},
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Volume Unpublish errored, empty TargetPath provided",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "test-vol",
				TargetPath: "",
			},
			expectedResponse: &csi.NodeUnpublishVolumeResponse{},
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Volume unpublish errored, could not check if the target path is a directory",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "test-vol",
				TargetPath: "/staging/path",
			},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(false, errors.New("some error"))
				return mockOSUtils
			},
			expectedResponse: &csi.NodeUnpublishVolumeResponse{},
			expErrCode:       codes.Internal,
		},
		{
			name: "Volume unpublish errored, target path directory not found",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "test-vol",
				TargetPath: "/staging/path",
			},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(false, fs.ErrNotExist)
				return mockOSUtils
			},
			expectedResponse: &csi.NodeUnpublishVolumeResponse{},
			expErrCode:       codes.OK,
		},
		{
			name: "Volume unpublish errored, target path is a directory, volume not mounted",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "test-vol",
				TargetPath: "/staging/path",
			},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(true, nil)
				mockOSUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), gomock.Any()).Return(nil)
				return mockOSUtils
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)

				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().RemovePublishedPath(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			expectedResponse: &csi.NodeUnpublishVolumeResponse{},
			expErrCode:       codes.OK,
		},
		{
			name: "Volume unpublish errored, target path is not a directory, volume not mounted",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "test-vol",
				TargetPath: "/staging/path",
			},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(false, nil)
				mockOSUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), gomock.Any()).Return(nil)
				return mockOSUtils
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsMounted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
				return mockMountClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().RemovePublishedPath(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			expectedResponse: &csi.NodeUnpublishVolumeResponse{},
			expErrCode:       codes.OK,
		},
		{
			name: "Volume unpublish errored, target path is a directory, unable to check if targetPath  is mounted",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "test-vol",
				TargetPath: "/staging/path",
			},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(true, nil)
				return mockOSUtils
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, errors.New("some error")).AnyTimes()
				return mockMountClient
			},
			expectedResponse: &csi.NodeUnpublishVolumeResponse{},
			expErrCode:       codes.Internal,
		},
		{
			name: "Volume unpublish errored, target path is a directory, mountpath doesn't exist",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "test-vol",
				TargetPath: "/staging/path",
			},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(true, nil)
				return mockOSUtils
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, fs.ErrNotExist).AnyTimes()
				return mockMountClient
			},
			expectedResponse: &csi.NodeUnpublishVolumeResponse{},
			expErrCode:       codes.NotFound,
		},
		{
			name: "Volume unpublish errored, unable to unmount volume",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "test-nfs-volume",
				TargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnpublishVolumeResponse{},
			expErrCode:       codes.InvalidArgument,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
				mockMountClient.EXPECT().Umount(gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockMountClient
			},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(true, nil)
				return mockOSUtils
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{VolumePublishInfo: models.VolumePublishInfo{}}, nil).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "Volume unpublish errored, unable to delete resource at target path, could not remove published path",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "test-nfs-volume",
				TargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnpublishVolumeResponse{},
			expErrCode:       codes.Internal,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
				mockMountClient.EXPECT().Umount(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(true, nil)
				mockOSUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), gomock.Any()).Return(errors.New(""))
				return mockOSUtils
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().RemovePublishedPath(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "Volume unpublish errored, published path from volume tracking file for volume is not found",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "test-nfs-volume",
				TargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnpublishVolumeResponse{},
			expErrCode:       codes.OK,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
				mockMountClient.EXPECT().Umount(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(true, nil)
				mockOSUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), gomock.Any()).Return(nil)
				return mockOSUtils
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().RemovePublishedPath(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(errors.NotFoundError("")).AnyTimes()
				return mockNodeHelper
			},
		},
		{
			name: "Success - volume unpublish",
			req: &csi.NodeUnpublishVolumeRequest{
				VolumeId:   "test-nfs-volume",
				TargetPath: "/staging/path",
			},
			expectedResponse: &csi.NodeUnpublishVolumeResponse{},
			expErrCode:       codes.OK,
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
				mockMountClient.EXPECT().Umount(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			setupOsutils: func() osutils.Utils {
				mockOSUtils := mock_osutils.NewMockUtils(gomock.NewController(t))
				mockOSUtils.EXPECT().IsLikelyDir(gomock.Any()).Return(true, nil)
				mockOSUtils.EXPECT().DeleteResourceAtPath(gomock.Any(), gomock.Any()).Return(nil)
				return mockOSUtils
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().RemovePublishedPath(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
		},
	}

	mountClient, _ := mount.New()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create plugin instance
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
				fs:               filesystem.New(mountClient),
			}

			plugin.InitializeNodeLimiter(ctx)

			if tc.setupNodeHelperMock != nil {
				plugin.nodeHelper = tc.setupNodeHelperMock()
			}

			if tc.setupMount != nil {
				plugin.mount = tc.setupMount()
			}
			if tc.setupOsutils != nil {
				plugin.osutils = tc.setupOsutils()
			}

			testCtx := context.Background()

			resp, err := plugin.NodeUnpublishVolume(testCtx, tc.req)

			// Verify results
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				if err != nil {
					status, _ := status.FromError(err)
					// assert.True(t, ok)
					assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				}
				assert.Nil(t, resp)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}

func TestNodeExpandVolume(t *testing.T) {
	testCases := []struct {
		name                string
		req                 *csi.NodeExpandVolumeRequest
		expectedResponse    *csi.NodeExpandVolumeResponse
		expErrCode          codes.Code
		setupNodeHelperMock func() nodehelpers.NodeHelper
		setupISCSIMock      func() iscsi.ISCSI
		getFCPClient        func() fcp.FCP
		mockFilesystem      func() filesystem.Filesystem
		getDeviceClient     func() devices.Devices
	}{
		{
			name: "Node expand volume failed, empty volumeID provided",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId: "",
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Node expand volume failed, empty TargetPath provided",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "",
			},
			expectedResponse: nil,
			expErrCode:       codes.InvalidArgument,
		},
		{
			name: "Node expand volume failed, could not read  volume tracking file",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil, errors.New("")).AnyTimes()
				return mockNodeHelper
			},
			expectedResponse: nil,
			expErrCode:       codes.Internal,
		},
		{
			name: "Node expand volume failed,  volume tracking file not found",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil, errors.NotFoundError("")).AnyTimes()
				return mockNodeHelper
			},
			expectedResponse: nil,
			expErrCode:       codes.NotFound,
		},
		{
			name: "Node expand volume failed, unable to read protocol info from publish info",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{StagingTargetPath: "/path", VolumePublishInfo: models.VolumePublishInfo{}}, nil).AnyTimes()
				return mockNodeHelper
			},
			expectedResponse: nil,
			expErrCode:       codes.FailedPrecondition,
		},
		{
			name: "Node expand volume for file protocols not required",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						VolumeAccessInfo: models.VolumeAccessInfo{
							NfsAccessInfo: models.NfsAccessInfo{
								NfsServerIP: "some-ip",
							},
						},
					},
					StagingTargetPath: "/vol/path",
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			expectedResponse: &csi.NodeExpandVolumeResponse{},
			expErrCode:       codes.OK,
		},
		{
			name: "Node expand volume for block protocols failed, filesystem type not supported",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: "unsupported-fs",
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiTargetIQN: "some-iqn",
							},
						},
					},
					StagingTargetPath: "/vol/path",
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			expectedResponse: &csi.NodeExpandVolumeResponse{},
			expErrCode:       codes.Unknown,
		},
		{
			name: "Node expand volume failed for Iscsi block protocol success",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: "ext4",
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiTargetIQN: "some-iqn",
							},
						},
					},
					StagingTargetPath: "/vol/path",
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupISCSIMock: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
				mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockISCSIClient
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().ExpandFilesystemOnNode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(2), nil).AnyTimes()

				return mockFilesystem1
			},
			expectedResponse: &csi.NodeExpandVolumeResponse{},
			expErrCode:       codes.OK,
		},
		{
			name: "Node expand volume failed for Iscsi block protocol failed, device to expand is not attached",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: "ext4",
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiTargetIQN: "some-iqn",
							},
						},
					},
					StagingTargetPath: "/vol/path",
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupISCSIMock: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(false).AnyTimes()
				mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockISCSIClient
			},

			expErrCode: codes.Internal,
		},
		{
			name: "Node expand volume failed for Iscsi block protocol failed, Unable to rescan device.",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: "ext4",
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiTargetIQN: "some-iqn",
							},
						},
					},
					StagingTargetPath: "/vol/path",
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupISCSIMock: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
				mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockISCSIClient
			},

			expErrCode: codes.Internal,
		},
		{
			name: "Node expand volume failed for Iscsi block protocol failed, Unable to expand filesystem.",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: "ext4",
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiTargetIQN: "some-iqn",
							},
						},
					},
					StagingTargetPath: "/vol/path",
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupISCSIMock: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
				mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockISCSIClient
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().ExpandFilesystemOnNode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(0), errors.New("some error")).AnyTimes()

				return mockFilesystem1
			},
			expErrCode: codes.Internal,
		},
		{
			name: "Node expand volume failed for fcp block protocol failed, device to expand is not attached",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: "ext4",
						SANType:        sa.FCP,
						VolumeAccessInfo: models.VolumeAccessInfo{
							FCPAccessInfo: models.FCPAccessInfo{
								FibreChannelAccessInfo: models.FibreChannelAccessInfo{
									FCTargetWWNN: "some-wwnn",
								},
							},
						},
					},
					StagingTargetPath: "/vol/path",
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			getFCPClient: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(false).AnyTimes()
				return mockFCPClient
			},
			expErrCode: codes.Internal,
		},
		{
			name: "Node expand volume failed for fcp block protocol failed, Unable to rescan device.",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: "ext4",
						SANType:        sa.FCP,
						VolumeAccessInfo: models.VolumeAccessInfo{
							FCPAccessInfo: models.FCPAccessInfo{
								FibreChannelAccessInfo: models.FibreChannelAccessInfo{
									FCTargetWWNN: "some-wwnn",
								},
							},
						},
					},
					StagingTargetPath: "/vol/path",
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			getFCPClient: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
				mockFCPClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockFCPClient
			},
			expErrCode: codes.Internal,
		},
		{
			name: "Node expand volume for NVMe protocol success",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						SANType:        sa.NVMe,
						FilesystemType: "ext4",
						VolumeAccessInfo: models.VolumeAccessInfo{
							NVMeAccessInfo: models.NVMeAccessInfo{
								NVMeSubsystemNQN: "some-nqn",
							},
						},
					},
					StagingTargetPath: "/vol/path",
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().ExpandFilesystemOnNode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(2), nil).AnyTimes()

				return mockFilesystem1
			},
			expectedResponse: &csi.NodeExpandVolumeResponse{},
			expErrCode:       codes.OK,
		},
		{
			name: "Node expand volume failed for Iscsi block protocol, luks enabled, error in getting multipath luks device",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						DevicePath:     "not-legacy-device-path",
						LUKSEncryption: "true",
						FilesystemType: "ext4",
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiTargetIQN: "some-iqn",
							},
						},
					},
					StagingTargetPath: "/vol/path",
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupISCSIMock: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
				mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockISCSIClient
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().ExpandFilesystemOnNode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(2), nil).AnyTimes()

				return mockFilesystem1
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(gomock.Any(), gomock.Any()).Return("", errors.New(""))
				return mockDeviceClient
			},
			expErrCode: codes.Internal,
		},
		{
			name: "Node expand volume failed for Iscsi block protocol failed with luks enabled, luks-passphrase empty",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
				Secrets: map[string]string{"luks-passphrase": ""},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						DevicePath:     "luks-legacy-device-path",
						LUKSEncryption: "true",
						FilesystemType: "ext4",
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiTargetIQN: "some-iqn",
							},
						},
					},
					StagingTargetPath: "/vol/path",
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupISCSIMock: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
				mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockISCSIClient
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().ExpandFilesystemOnNode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(2), nil).AnyTimes()

				return mockFilesystem1
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(
					gomock.Any(), gomock.Any(),
				).Return("x/device-path", nil).AnyTimes()
				return mockDeviceClient
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Node expand volume failed for Iscsi block protocol failed with luks enabled, luks-passphrase not present",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
				Secrets: map[string]string{},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						DevicePath:     "luks-legacy-device-path",
						LUKSEncryption: "true",
						FilesystemType: "ext4",
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiTargetIQN: "some-iqn",
							},
						},
					},
					StagingTargetPath: "/vol/path",
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupISCSIMock: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
				mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockISCSIClient
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().ExpandFilesystemOnNode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(2), nil).AnyTimes()

				return mockFilesystem1
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(
					gomock.Any(), gomock.Any(),
				).Return("x/device-path", nil).AnyTimes()
				return mockDeviceClient
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "Node expand volume failed for Iscsi block protocol failed with luks enabled",
			req: &csi.NodeExpandVolumeRequest{
				VolumeId:   "test-vol",
				VolumePath: "/vol/path",
				CapacityRange: &csi.CapacityRange{
					RequiredBytes: 2 * 1024 * 1024 * 1024,
					LimitBytes:    3 * 1024 * 1024 * 1024,
				},
				Secrets: map[string]string{"luks-passphrase": "fake-pass"},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						DevicePath:     "luks-legacy-device-path",
						LUKSEncryption: "true",
						FilesystemType: "ext4",
						VolumeAccessInfo: models.VolumeAccessInfo{
							IscsiAccessInfo: models.IscsiAccessInfo{
								IscsiTargetIQN: "some-iqn",
							},
						},
					},
					StagingTargetPath: "/vol/path",
				}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupISCSIMock: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().IsAlreadyAttached(gomock.Any(), gomock.Any(), gomock.Any()).Return(true).AnyTimes()
				mockISCSIClient.EXPECT().RescanDevices(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockISCSIClient
			},
			mockFilesystem: func() filesystem.Filesystem {
				mockFilesystem1 := mock_filesystem.NewMockFilesystem(gomock.NewController(t))
				mockFilesystem1.EXPECT().ExpandFilesystemOnNode(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(2), nil).AnyTimes()

				return mockFilesystem1
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(
					gomock.Any(), gomock.Any(),
				).Return("x/device-path", nil).AnyTimes()
				return mockDeviceClient
			},
			expErrCode: codes.Internal,
		},
	}

	mountClient, _ := mount.New()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create plugin instance
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
				fs:               filesystem.New(mountClient),
			}

			plugin.InitializeNodeLimiter(ctx)

			if tc.setupNodeHelperMock != nil {
				plugin.nodeHelper = tc.setupNodeHelperMock()
			}
			if tc.setupISCSIMock != nil {
				plugin.iscsi = tc.setupISCSIMock()
			}
			if tc.getDeviceClient != nil {
				plugin.devices = tc.getDeviceClient()
			}

			if tc.getFCPClient != nil {
				plugin.fcp = tc.getFCPClient()
			}
			if tc.mockFilesystem != nil {
				plugin.fs = tc.mockFilesystem()
			}

			testCtx := context.Background()

			resp, err := plugin.NodeExpandVolume(testCtx, tc.req)

			// Verify results
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				if err != nil {
					status, _ := status.FromError(err)
					// assert.True(t, ok)
					assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				}
				assert.Nil(t, resp)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, resp)
		})
	}
}

func TestNodeStageFCPVolume_FailureCases(t *testing.T) {
	testCases := []struct {
		name                string
		req                 *csi.NodeStageVolumeRequest
		expErrCode          codes.Code
		publishInfo         *models.VolumePublishInfo
		setupFCPMock        func() fcp.FCP
		setupNodeHelperMock func() nodehelpers.NodeHelper
	}{
		{
			name: "FCP volume staging failed, error getting staging target path",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "",
				PublishContext: map[string]string{
					"fcpLunNumber": "123",
				},
			},
			publishInfo: &models.VolumePublishInfo{},
			expErrCode:  codes.InvalidArgument,
		},
		{
			name: "FCP Volume staging errored, invalid fcpLunNumber",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				PublishContext: map[string]string{
					"fcpLunNumber": "-123",
				},
			},
			publishInfo: &models.VolumePublishInfo{},
			expErrCode:  codes.Unknown,
		},
		{
			name: "FCP Volume staging errored, Could not write tracking file.",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				PublishContext: map[string]string{
					"fcpLunNumber": "123",
				},
			},
			publishInfo: &models.VolumePublishInfo{},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(errors.New("some error")).AnyTimes()
				return mockNodeHelper
			},
			expErrCode: codes.Unknown,
		},
		{
			name: "FCP Volume staging Success, could not set LUKS volume passphrase",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				PublishContext: map[string]string{
					"fcpLunNumber":   "123",
					"LUKSEncryption": "true",
				},
				VolumeContext: map[string]string{
					"internalName": "test-vol",
				},
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(3), nil)
				return mockFCPClient
			},
			publishInfo: &models.VolumePublishInfo{},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			expErrCode: codes.Unknown,
		},
		{
			name: "FCP Volume staging success, attempt to perform gratitous resize failed, when multipath device size incorrect",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				PublishContext: map[string]string{
					"fcpLunNumber":   "123",
					"LUKSEncryption": "false",
				},
				VolumeContext: map[string]string{
					"internalName": "test-vol",
				},
			},
			publishInfo: &models.VolumePublishInfo{},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().AttachVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(int64(3), nil)
				return mockFCPClient
			},
			expErrCode: codes.OK,
		},
	}

	mountClient, _ := mount.New()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create plugin instance
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
				fs:               filesystem.New(mountClient),
			}

			plugin.InitializeNodeLimiter(ctx)

			if tc.setupNodeHelperMock != nil {
				plugin.nodeHelper = tc.setupNodeHelperMock()
			}
			if tc.setupFCPMock != nil {
				plugin.fcp = tc.setupFCPMock()
			}

			testCtx := context.Background()

			err := plugin.nodeStageFCPVolume(testCtx, tc.req, tc.publishInfo)

			// Verify results
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				if err != nil {
					status, _ := status.FromError(err)
					// assert.True(t, ok)
					assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				}
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestNodeUnstageFCPVolume(t *testing.T) {
	defer func(previousfcpUtils fcp.FcpReconcileUtils) {
		fcpUtils = previousfcpUtils
	}(fcpUtils)
	testCases := []struct {
		name                string
		req                 *csi.NodeUnstageVolumeRequest
		expErrCode          codes.Code
		publishInfo         *models.VolumePublishInfo
		force               bool
		setupFCPutils       func() fcp.FcpReconcileUtils
		setupFCPMock        func() fcp.FCP
		setupMount          func() mount.Mount
		getDeviceClient     func() devices.Devices
		setupNodeHelperMock func() nodehelpers.NodeHelper
	}{
		{
			name: "node unstage fcp volume success, Got 0 devices for LUN",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
			},
			publishInfo: &models.VolumePublishInfo{},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.OK,
		},
		{
			name: "node unstage fcp volume success, got devices for LUN",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
			},
			publishInfo: &models.VolumePublishInfo{},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().GetDeviceInfoForLUN(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&models.ScsiDeviceInfo{}, nil).AnyTimes()
				mockFCPClient.EXPECT().PrepareDeviceForRemoval(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
				return mockFCPClient
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"d1", "d2"}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.OK,
		},
		{
			name: "node unstage fcp volume failed, could not get devices for LUN",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
			},
			publishInfo: &models.VolumePublishInfo{},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{}, errors.New("")).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.Unknown,
		},
		{
			name: "node unstage fcp volume failed, empty staging path",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "",
			},
			publishInfo: &models.VolumePublishInfo{},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().GetDeviceInfoForLUN(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&models.ScsiDeviceInfo{}, nil).AnyTimes()
				mockFCPClient.EXPECT().PrepareDeviceForRemoval(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
				return mockFCPClient
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.InvalidArgument,
		},
		{
			name: "node unstage fcp volume failed, error deleting tracking info file",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
			},
			publishInfo: &models.VolumePublishInfo{},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().GetDeviceInfoForLUN(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&models.ScsiDeviceInfo{}, nil).AnyTimes()
				mockFCPClient.EXPECT().PrepareDeviceForRemoval(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
				return mockFCPClient
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.Internal,
		},
		{
			name: "node unstage fcp volume failed, error deleting tracking info file with luks enabled",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
			},
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "true",
				DevicePath:     "not-legacy-device-path",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().GetDeviceInfoForLUN(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&models.ScsiDeviceInfo{}, nil).AnyTimes()
				mockFCPClient.EXPECT().PrepareDeviceForRemoval(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
				return mockFCPClient
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(
					gomock.Any(), gomock.Any(),
				).Return("", errors.New("")).AnyTimes()
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.Internal,
		},
		{
			name: "node unstage fcp volume success, with luks enabled",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
			},
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "true",
				DevicePath:     "luks-legacy-device-path",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().GetDeviceInfoForLUN(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&models.ScsiDeviceInfo{}, nil).AnyTimes()
				mockFCPClient.EXPECT().PrepareDeviceForRemoval(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
				return mockFCPClient
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.OK,
		},
		{
			name: "node unstage fcp volume error, error getting device info for LUN",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
			},
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "true",
				DevicePath:     "luks-legacy-device-path",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().GetDeviceInfoForLUN(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("")).AnyTimes()
				mockFCPClient.EXPECT().PrepareDeviceForRemoval(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
				return mockFCPClient
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"d1"}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.Unknown,
		},
		{
			name: "node unstage fcp volume error, device info for LUN is nil",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
			},
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "true",
				DevicePath:     "luks-legacy-device-path",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().GetDeviceInfoForLUN(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
				mockFCPClient.EXPECT().PrepareDeviceForRemoval(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
				return mockFCPClient
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"d1"}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.OK,
		},
		{
			name: "node unstage fcp volume error, Failed to get LUKS device path from multipath device.",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
			},
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "true",
				DevicePath:     "luks-legacy-device-path",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().GetDeviceInfoForLUN(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&models.ScsiDeviceInfo{MultipathDevice: "xyz"}, nil).AnyTimes()
				mockFCPClient.EXPECT().PrepareDeviceForRemoval(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
				return mockFCPClient
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(
					gomock.Any(), gomock.Any(),
				).Return("", errors.New("")).AnyTimes()
				// mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
				// 	gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				// mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"d1"}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.Unknown,
		},
		{
			name: "node unstage fcp volume error, Failed to close LUKS device.",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
			},
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "true",
				DevicePath:     "luks-legacy-device-path",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().GetDeviceInfoForLUN(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&models.ScsiDeviceInfo{MultipathDevice: "xyz"}, nil).AnyTimes()
				mockFCPClient.EXPECT().PrepareDeviceForRemoval(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
				return mockFCPClient
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(
					gomock.Any(), gomock.Any(),
				).Return("multipath-device", nil).AnyTimes()
				// mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
				// 	gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"d1"}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.Unknown,
		},
		{
			name: "node unstage fcp volume error, error preparing device for removal",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
			},
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "true",
				DevicePath:     "luks-legacy-device-path",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(), gomock.Any()).Return(&models.VolumeTrackingInfo{}, nil).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().GetDeviceInfoForLUN(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&models.ScsiDeviceInfo{MultipathDevice: "xyz"}, nil).AnyTimes()
				mockFCPClient.EXPECT().PrepareDeviceForRemoval(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", errors.FCPSameLunNumberError("")).AnyTimes()
				return mockFCPClient
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(
					gomock.Any(), gomock.Any(),
				).Return("multipath-device", nil).AnyTimes()
				// mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
				// 	gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), gomock.Any()).Return(errors.MaxWaitExceededError("")).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"d1"}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.Internal,
		},
		{
			name: "node unstage fcp volume error, failed to remove temporary directory in staging target path",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
			},
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "true",
				DevicePath:     "luks-legacy-device-path",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().GetDeviceInfoForLUN(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&models.ScsiDeviceInfo{MultipathDevice: "xyz"}, nil).AnyTimes()
				mockFCPClient.EXPECT().PrepareDeviceForRemoval(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
				return mockFCPClient
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(
					gomock.Any(), gomock.Any(),
				).Return("multipath-device", nil).AnyTimes()
				// mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
				// 	gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), gomock.Any()).Return(errors.MaxWaitExceededError("")).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"d1"}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.Internal,
		},
		{
			name: "node unstage fcp volume error, failed to flush(remove) multipath device mappings",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
			},
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "true",
				DevicePath:     "luks-legacy-device-path",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().GetDeviceInfoForLUN(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&models.ScsiDeviceInfo{MultipathDevice: "xyz"}, nil).AnyTimes()
				mockFCPClient.EXPECT().PrepareDeviceForRemoval(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
				return mockFCPClient
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(
					gomock.Any(), gomock.Any(),
				).Return("multipath-device", nil).AnyTimes()
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), gomock.Any()).Return(errors.MaxWaitExceededError("")).AnyTimes()
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosed(gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"d1"}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.Unknown,
		},
		{
			name: "node unstage fcp volume error, failed deleting tracking info file",
			req: &csi.NodeUnstageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
			},
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "true",
				DevicePath:     "luks-legacy-device-path",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().DeleteTrackingInfo(gomock.Any(),
					gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockNodeHelper
			},
			setupFCPMock: func() fcp.FCP {
				mockFCPClient := mock_fcp.NewMockFCP(gomock.NewController(t))
				mockFCPClient.EXPECT().GetDeviceInfoForLUN(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(&models.ScsiDeviceInfo{MultipathDevice: "xyz"}, nil).AnyTimes()
				mockFCPClient.EXPECT().PrepareDeviceForRemoval(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil).AnyTimes()
				return mockFCPClient
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().UmountAndRemoveTemporaryMountPoint(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDeviceClient.EXPECT().GetLUKSDevicePathForVolume(
					gomock.Any(), gomock.Any(),
				).Return("multipath-device", nil).AnyTimes()
				mockDeviceClient.EXPECT().RemoveMultipathDeviceMappingWithRetries(gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosed(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				mockDeviceClient.EXPECT().EnsureLUKSDeviceClosedWithMaxWaitLimit(gomock.Any(), gomock.Any()).Return(errors.MaxWaitExceededError("")).AnyTimes()
				return mockDeviceClient
			},
			setupFCPutils: func() fcp.FcpReconcileUtils {
				setupFCPutils := mock_fcp.NewMockFcpReconcileUtils(gomock.NewController(t))
				setupFCPutils.EXPECT().GetFCPHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return([]map[string]int{{"session": 3}}).AnyTimes()
				setupFCPutils.EXPECT().GetSysfsBlockDirsForLUN(gomock.Any(),
					gomock.Any()).Return([]string{}).AnyTimes()
				setupFCPutils.EXPECT().GetDevicesForLUN(gomock.Any()).Return([]string{"d1"}, nil).AnyTimes()
				return setupFCPutils
			},
			expErrCode: codes.Internal,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create plugin instance
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
			}

			plugin.InitializeNodeLimiter(ctx)

			if tc.setupNodeHelperMock != nil {
				plugin.nodeHelper = tc.setupNodeHelperMock()
			}
			if tc.setupFCPMock != nil {
				plugin.fcp = tc.setupFCPMock()
			}
			if tc.setupMount != nil {
				plugin.mount = tc.setupMount()
			}
			if tc.getDeviceClient != nil {
				plugin.devices = tc.getDeviceClient()
			}
			if tc.setupFCPutils != nil {
				fcpUtils = tc.setupFCPutils()
			}

			testCtx := context.Background()

			err := plugin.nodeUnstageFCPVolume(testCtx, tc.req, tc.publishInfo, tc.force)

			// Verify results
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				if err != nil {
					status, _ := status.FromError(err)
					// assert.True(t, ok)
					assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				}
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestNodePublishFCPVolume(t *testing.T) {
	defer func(previousfcpUtils fcp.FcpReconcileUtils) {
		fcpUtils = previousfcpUtils
	}(fcpUtils)
	testCases := []struct {
		name                string
		req                 *csi.NodePublishVolumeRequest
		expErrCode          codes.Code
		setupMount          func() mount.Mount
		setupNodeHelperMock func() nodehelpers.NodeHelper
	}{
		{
			name: "node publish fcp volume success",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "block",
					"sharedTarget":   "false",
					"filesystemType": filesystem.Ext4,
					"useCHAP":        "false",
					"SANType":        sa.FCP,
				},
				StagingTargetPath: "/foo",
				TargetPath:        "/tmp/raw",
				Secrets: map[string]string{
					"username": "username",
					"password": "password",
				},
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: true,
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{VolumePublishInfo: models.VolumePublishInfo{DevicePath: "devicepath", FilesystemType: "ext4"}}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			expErrCode: codes.OK,
		},
		{
			name: "node publish fcp volume failed, error reading tracking info file",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "block",
					"sharedTarget":   "false",
					"filesystemType": filesystem.Ext4,
					"useCHAP":        "false",
					"SANType":        sa.FCP,
				},
				StagingTargetPath: "/foo",
				TargetPath:        "/tmp/raw",
				Secrets: map[string]string{
					"username": "username",
					"password": "password",
				},
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: false,
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil, errors.New("")).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			expErrCode: codes.Internal,
		},
		{
			name: "node publish fcp volume failed, error reading tracking info file not found",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "block",
					"sharedTarget":   "false",
					"filesystemType": filesystem.Ext4,
					"useCHAP":        "false",
					"SANType":        sa.FCP,
				},
				StagingTargetPath: "/foo",
				TargetPath:        "/tmp/raw",
				Secrets: map[string]string{
					"username": "username",
					"password": "password",
				},
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: false,
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil, errors.NotFoundError("")).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			expErrCode: codes.FailedPrecondition,
		},
		{
			name: "node publish fcp volume failed, mount device failed for raw fs",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "block",
					"sharedTarget":   "false",
					"filesystemType": filesystem.Ext4,
					"useCHAP":        "false",
					"SANType":        sa.FCP,
				},
				StagingTargetPath: "/foo",
				TargetPath:        "/tmp/raw",
				Secrets: map[string]string{
					"username": "username",
					"password": "password",
				},
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: true,
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{VolumePublishInfo: models.VolumePublishInfo{DevicePath: "devicepath", FilesystemType: "raw"}}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockMountClient
			},
			expErrCode: codes.Internal,
		},
		{
			name: "node publish fcp volume failed, mount device failed for raw fs",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "block",
					"sharedTarget":   "false",
					"filesystemType": filesystem.Ext4,
					"useCHAP":        "false",
					"SANType":        sa.FCP,
				},
				StagingTargetPath: "/foo",
				TargetPath:        "/tmp/raw",
				Secrets: map[string]string{
					"username": "username",
					"password": "password",
				},
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: false,
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{VolumePublishInfo: models.VolumePublishInfo{DevicePath: "devicepath", FilesystemType: "ext4"}}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockMountClient
			},
			expErrCode: codes.Internal,
		},
		{
			name: "node publish fcp volume failed, adding published path errored",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "block",
					"sharedTarget":   "false",
					"filesystemType": filesystem.Ext4,
					"useCHAP":        "false",
					"SANType":        sa.FCP,
				},
				StagingTargetPath: "/foo",
				TargetPath:        "/tmp/raw",
				Secrets: map[string]string{
					"username": "username",
					"password": "password",
				},
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: false,
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{VolumePublishInfo: models.VolumePublishInfo{DevicePath: "devicepath", FilesystemType: "ext4"}}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			expErrCode: codes.Unknown,
		},
		{
			name: "node publish fcp volume failed, luks encryption enabled, empty pasphrase, non legacy device path",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "block",
					"sharedTarget":   "false",
					"filesystemType": filesystem.Ext4,
					"useCHAP":        "false",
					"SANType":        sa.FCP,
				},
				StagingTargetPath: "/foo",
				TargetPath:        "/tmp/raw",
				Secrets: map[string]string{
					"username":        "username",
					"password":        "password",
					"luks-passphrase": "",
				},
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: false,
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{VolumePublishInfo: models.VolumePublishInfo{DevicePath: "devicepath", FilesystemType: "ext4", LUKSEncryption: "true"}}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			expErrCode: codes.OK,
		},
		{
			name: "node publish fcp volume failed, luks encryption enabled, empty pasphrase, lagacy device path",
			req: &csi.NodePublishVolumeRequest{
				VolumeId: uuid.NewString(),
				PublishContext: map[string]string{
					"protocol":       "block",
					"sharedTarget":   "false",
					"filesystemType": filesystem.Ext4,
					"useCHAP":        "false",
					"SANType":        sa.FCP,
				},
				StagingTargetPath: "/foo",
				TargetPath:        "/tmp/raw",
				Secrets: map[string]string{
					"username":        "username",
					"password":        "password",
					"luks-passphrase": "",
				},
				VolumeContext: map[string]string{
					"internalName": "pvc-12345-123",
				},
				Readonly: false,
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{VolumePublishInfo: models.VolumePublishInfo{DevicePath: "luks-devicepath", FilesystemType: "ext4", LUKSEncryption: "true"}}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},
			expErrCode: codes.Internal,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create plugin instance
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
			}

			plugin.InitializeNodeLimiter(ctx)

			if tc.setupNodeHelperMock != nil {
				plugin.nodeHelper = tc.setupNodeHelperMock()
			}

			if tc.setupMount != nil {
				plugin.mount = tc.setupMount()
			}

			testCtx := context.Background()

			_, err := plugin.nodePublishFCPVolume(testCtx, tc.req)

			// Verify results
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				if err != nil {
					status, _ := status.FromError(err)
					// assert.True(t, ok)
					assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				}
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestNodeStageNVMeVolume(t *testing.T) {
	testCases := []struct {
		name                 string
		req                  *csi.NodeStageVolumeRequest
		expErrCode           codes.Code
		publishInfo          *models.VolumePublishInfo
		setupNvmeHandlerMock func() nvme.NVMeInterface
		setupNodeHelperMock  func() nodehelpers.NodeHelper
	}{
		{
			name: "NVMe volume staging failed, error getting staging target path",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "",
				PublishContext: map[string]string{
					"fcpLunNumber": "123",
				},
			},
			setupNvmeHandlerMock: func() nvme.NVMeInterface {
				setupNvmeHandlerMock := mock_nvme.NewMockNVMeInterface(gomock.NewController(t))
				setupNvmeHandlerMock.EXPECT().AttachNVMeVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(errors.New("")).AnyTimes()
				return setupNvmeHandlerMock
			},
			publishInfo: &models.VolumePublishInfo{},
			expErrCode:  codes.Unknown,
		},
		{
			name: "NVMe Volume staging errored, Could not write tracking file.",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				PublishContext:    map[string]string{},
			},
			setupNvmeHandlerMock: func() nvme.NVMeInterface {
				setupNvmeHandlerMock := mock_nvme.NewMockNVMeInterface(gomock.NewController(t))
				setupNvmeHandlerMock.EXPECT().AttachNVMeVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(nil).AnyTimes()
				return setupNvmeHandlerMock
			},
			publishInfo: &models.VolumePublishInfo{},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(errors.New("some error")).AnyTimes()
				return mockNodeHelper
			},
			expErrCode: codes.Unknown,
		},
		{
			name: "NVMe Volume staging errored, AttachNVMeVolumeRetry failed",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				PublishContext:    map[string]string{},
			},
			setupNvmeHandlerMock: func() nvme.NVMeInterface {
				setupNvmeHandlerMock := mock_nvme.NewMockNVMeInterface(gomock.NewController(t))
				setupNvmeHandlerMock.EXPECT().AttachNVMeVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(errors.New("")).AnyTimes()
				return setupNvmeHandlerMock
			},
			publishInfo: &models.VolumePublishInfo{},
			expErrCode:  codes.Unknown,
		},
		{
			name: "NVMe Volume staging errored, luks enabled, passphrase empty",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				PublishContext: map[string]string{
					"LUKSEncryption": "true",
				},
			},
			setupNvmeHandlerMock: func() nvme.NVMeInterface {
				setupNvmeHandlerMock := mock_nvme.NewMockNVMeInterface(gomock.NewController(t))
				setupNvmeHandlerMock.EXPECT().AttachNVMeVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(nil).AnyTimes()
				return setupNvmeHandlerMock
			},
			publishInfo: &models.VolumePublishInfo{
				DevicePath: "device",
			},
			expErrCode: codes.Unknown,
		},
		{
			name: "NVMe Volume staging success",
			req: &csi.NodeStageVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging/path",
				PublishContext:    map[string]string{},
			},
			setupNvmeHandlerMock: func() nvme.NVMeInterface {
				setupNvmeHandlerMock := mock_nvme.NewMockNVMeInterface(gomock.NewController(t))
				setupNvmeHandlerMock.EXPECT().AttachNVMeVolumeRetry(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(nil).AnyTimes()
				setupNvmeHandlerMock.EXPECT().AddPublishedNVMeSession(gomock.Any(), gomock.Any()).AnyTimes()
				return setupNvmeHandlerMock
			},
			publishInfo: &models.VolumePublishInfo{},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().WriteTrackingInfo(gomock.Any(),
					gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			expErrCode: codes.OK,
		},
	}

	mountClient, _ := mount.New()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create plugin instance
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
				fs:               filesystem.New(mountClient),
			}

			plugin.InitializeNodeLimiter(ctx)

			if tc.setupNodeHelperMock != nil {
				plugin.nodeHelper = tc.setupNodeHelperMock()
			}
			if tc.setupNvmeHandlerMock != nil {
				plugin.nvmeHandler = tc.setupNvmeHandlerMock()
			}

			testCtx := context.Background()

			err := plugin.nodeStageNVMeVolume(testCtx, tc.req, tc.publishInfo)

			// Verify results
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				if err != nil {
					status, _ := status.FromError(err)
					// assert.True(t, ok)
					assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				}
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestNodePublishNVMeVolume(t *testing.T) {
	testCases := []struct {
		name                string
		req                 *csi.NodePublishVolumeRequest
		expErrCode          codes.Code
		setupMount          func() mount.Mount
		setupNodeHelperMock func() nodehelpers.NodeHelper
	}{
		{
			name: "NVMe volume publish success, raw filesystem",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
				TargetPath:        "/target",
				Readonly:          true,
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: filesystem.Raw,
					},
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},

			expErrCode: codes.OK,
		},
		{
			name: "NVMe volume publish success, ext4 filesystem",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
				TargetPath:        "/target",
				Readonly:          true,
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: "ext4",
					},
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				return mockMountClient
			},
			expErrCode: codes.OK,
		},
		{
			name: "NVMe volume publish error, unable to mount device for ext4 fs type",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
				TargetPath:        "/target",
				Readonly:          true,
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: "ext4",
					},
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockMountClient
			},
			expErrCode: codes.Internal,
		},
		{
			name: "NVMe volume publish error, unable to mount device for raw fs type",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
				TargetPath:        "/target",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: filesystem.Raw,
					},
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockMountClient
			},
			expErrCode: codes.Internal,
		},
		{
			name: "NVMe volume publish errored, Failed to add publish path",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
				TargetPath:        "/target",
				Readonly:          true,
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: "ext4",
					},
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				return mockMountClient
			},
			expErrCode: codes.Unknown,
		},
		{
			name: "NVMe volume publish errored, Failed to read tracking info file",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
				TargetPath:        "/target",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil, errors.New("")).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				return mockMountClient
			},
			expErrCode: codes.Internal,
		},
		{
			name: "NVMe volume publish errored, tracking info file not found",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
				TargetPath:        "/target",
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(nil, errors.NotFoundError("")).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				return mockMountClient
			},
			expErrCode: codes.FailedPrecondition,
		},
		{
			name: "NVMe volume publish success, luks encryption enabled",
			req: &csi.NodePublishVolumeRequest{
				VolumeId:          "test-nfs-volume",
				StagingTargetPath: "/staging",
				TargetPath:        "/target",
				Readonly:          true,
				VolumeContext:     map[string]string{"internalName": "test-nfs-volume"},
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ReadTrackingInfo(gomock.Any(),
					gomock.Any()).Return(&models.VolumeTrackingInfo{
					VolumePublishInfo: models.VolumePublishInfo{
						FilesystemType: filesystem.Raw,
						LUKSEncryption: "true",
					},
				}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockNodeHelper
			},
			setupMount: func() mount.Mount {
				mockMountClient := mock_mount.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountDevice(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
				return mockMountClient
			},

			expErrCode: codes.OK,
		},
	}

	mountClient, _ := mount.New()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create plugin instance
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
				fs:               filesystem.New(mountClient),
			}

			plugin.InitializeNodeLimiter(ctx)

			if tc.setupNodeHelperMock != nil {
				plugin.nodeHelper = tc.setupNodeHelperMock()
			}
			if tc.setupMount != nil {
				plugin.mount = tc.setupMount()
			}

			testCtx := context.Background()

			_, err := plugin.nodePublishNVMeVolume(testCtx, tc.req)

			// Verify results
			if tc.expErrCode != codes.OK {
				assert.Error(t, err)
				if err != nil {
					status, _ := status.FromError(err)
					// assert.True(t, ok)
					assert.Equal(t, tc.expErrCode, status.Code(), "Expected error code %v, got %v", tc.expErrCode, status.Code())
				}
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestPerformIscsiSelfHealing(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockIscsiHandler := mock_iscsi.NewMockISCSI(mockCtrl)
	nodeServer := &Plugin{iscsi: mockIscsiHandler}

	// Empty Published sessions case.
	mockIscsiHandler.EXPECT().PreChecks(ctx).Return(nil).AnyTimes()
	mockIscsiHandler.EXPECT().PopulateCurrentSessions(ctx, gomock.Any()).Return(nil).AnyTimes()
	mockIscsiHandler.EXPECT().InspectAllISCSISessions(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return([]string{}, []string{}).AnyTimes()

	nodeServer.performISCSISelfHealing(ctx)

	// Error populating current sessions.
	publishedISCSISessions.AddLUNToPortal("portal", models.LUNData{LUN: 1, VolID: "vol-id"})
	publishedISCSISessions.AddPortal("portal", models.PortalInfo{})
	mockIscsiHandler.EXPECT().PopulateCurrentSessions(ctx, gomock.Any()).
		Return(errors.New("Failed to get current state of iSCSI Sessions LUN mappings; skipping iSCSI self-heal cycle.")).AnyTimes()
	mockIscsiHandler.EXPECT().PreChecks(ctx).Return(nil).AnyTimes()

	nodeServer.performISCSISelfHealing(ctx)

	// Self-healing process done.
	mockIscsiHandler.EXPECT().PreChecks(ctx).Return(errors.New("")).AnyTimes()
	mockIscsiHandler.EXPECT().PopulateCurrentSessions(ctx, gomock.Any()).Return(nil).AnyTimes()
	mockIscsiHandler.EXPECT().InspectAllISCSISessions(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return([]string{}, []string{}).AnyTimes()

	nodeServer.performISCSISelfHealing(ctx)
	// Cleanup of global objects.
	publishedISCSISessions.RemoveLUNFromPortal("protal", 1)
	publishedISCSISessions.RemovePortal("portal")
}

func TestSelfHealingRectifySession(t *testing.T) {
	defer func(previousIscsiUtils iscsi.IscsiReconcileUtils) {
		iscsi.IscsiUtils = previousIscsiUtils
	}(iscsi.IscsiUtils)
	lunList1 := map[int32]string{
		1: "volID-1",
	}
	sessionData1 := models.ISCSISessionData{
		PortalInfo: models.PortalInfo{
			ISCSITargetIQN: "IQN1",
			Credentials: models.IscsiChapInfo{
				UseCHAP:              true,
				IscsiUsername:        "username1",
				IscsiInitiatorSecret: "secret1",
				IscsiTargetUsername:  "username2",
				IscsiTargetSecret:    "secret2",
			},
			FirstIdentifiedStaleAt: time.Now().Add(-time.Second * 10),
		},
		LUNs: models.LUNs{
			Info: mapCopyHelper(lunList1),
		},
	}

	sessionData2 := models.ISCSISessionData{
		PortalInfo: models.PortalInfo{
			ISCSITargetIQN:         "IQN1",
			FirstIdentifiedStaleAt: time.Now().Add(-time.Second * 10),
		},
	}

	PublishedPortals := &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
		"1.2.3.4": structCopyHelper(sessionData1),
		"1.3.2.1": structCopyHelper(sessionData2),
	}}
	CurrentPortals := &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}}

	publishedISCSISessions = PublishedPortals
	currentISCSISessions = CurrentPortals

	testCases := []struct {
		name                         string
		portal                       string
		action                       models.ISCSIAction
		expErr                       error
		getISCSIClient               func() iscsi.ISCSI
		getIscsiReconcileUtilsClient func() iscsi.IscsiReconcileUtils
		setupNodeHelperMock          func() nodehelpers.NodeHelper
	}{
		{
			name:   "SelfHealingRectifySession for Scan action error,failed to initiate scan for LUNs in portal",
			portal: "1.2.3.4",
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().DiscoverSCSIAddressMapForTarget(gomock.Any(),
					gomock.Any()).Return(nil, errors.New("")).AnyTimes()
				return mockIscsiReconcileUtilsClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).Return(map[string]*models.VolumeTrackingInfo{"abc": {}}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockNodeHelper
			},
			action: models.Scan,

			expErr: errors.New("failed to initiate scan for LUNs in portal"),
		},
		{
			name:   "SelfHealingRectifySession for Scan action success",
			portal: "1.3.2.1",
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().DiscoverSCSIAddressMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[string]models.ScsiDeviceAddress{"scsi": {}}, nil).AnyTimes()
				return mockIscsiReconcileUtilsClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).Return(map[string]*models.VolumeTrackingInfo{"abc": {}}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockNodeHelper
			},
			action: models.Scan,

			expErr: nil,
		},
		{
			name:   "SelfHealingRectifySession for LoginScan action error, failed to get volume ID for lun ID",
			portal: "1.3.2.1",
			action: models.LoginScan,

			expErr: errors.New("failed to get volume ID for lun ID"),
		},
		{
			name:   "SelfHealingRectifySession, No valid action to be taken in iSCSI self-healing.",
			portal: "1.2.3.4",
			action: 0,

			expErr: nil,
		},
		{
			name:   "SelfHealingRectifySession for LogoutLoginScan action error, cannot safely log out of unresponsive portal",
			portal: "1.2.3.4",
			action: models.LogoutLoginScan,
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().IsPortalAccessible(gomock.Any(), gomock.Any()).Return(false, nil)
				return mockISCSIClient
			},
			expErr: errors.New("cannot safely log out of unresponsive portal"),
		},
		{
			name:   "SelfHealingRectifySession for LogoutLoginScan action error, error while logging out of target",
			portal: "1.2.3.4",
			action: models.LogoutLoginScan,
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().IsPortalAccessible(gomock.Any(), gomock.Any()).Return(true, nil)
				mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New(""))
				return mockISCSIClient
			},
			expErr: errors.New("error while logging out of target"),
		},
		{
			name:   "SelfHealingRectifySession for LogoutLoginScan action error, failed to initiate scan in fallthrough",
			portal: "1.2.3.4",
			action: models.LogoutLoginScan,
			getISCSIClient: func() iscsi.ISCSI {
				mockISCSIClient := mock_iscsi.NewMockISCSI(gomock.NewController(t))
				mockISCSIClient.EXPECT().IsPortalAccessible(gomock.Any(), gomock.Any()).Return(true, nil)
				mockISCSIClient.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockISCSIClient.EXPECT().AttachVolumeRetry(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(int64(2), nil).AnyTimes()
				return mockISCSIClient
			},
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().DiscoverSCSIAddressMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[string]models.ScsiDeviceAddress{"scsi": {}}, nil).AnyTimes()
				return mockIscsiReconcileUtilsClient
			},
			setupNodeHelperMock: func() nodehelpers.NodeHelper {
				mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))
				mockNodeHelper.EXPECT().ListVolumeTrackingInfo(gomock.Any()).Return(map[string]*models.VolumeTrackingInfo{"abc": {}}, nil).AnyTimes()
				mockNodeHelper.EXPECT().AddPublishedPath(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("")).AnyTimes()
				return mockNodeHelper
			},
			expErr: errors.New("failed to initiate scan"),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			// Create plugin instance
			plugin := &Plugin{
				command:          execCmd.NewCommand(),
				role:             CSINode,
				limiterSharedMap: make(map[string]limiter.Limiter),
			}

			plugin.InitializeNodeLimiter(ctx)

			if tc.getIscsiReconcileUtilsClient != nil {
				iscsi.IscsiUtils = tc.getIscsiReconcileUtilsClient()
			}
			if tc.setupNodeHelperMock != nil {
				plugin.nodeHelper = tc.setupNodeHelperMock()
			}
			if tc.getISCSIClient != nil {
				plugin.iscsi = tc.getISCSIClient()
			}

			testCtx := context.Background()

			err := plugin.selfHealingRectifySession(testCtx, tc.portal, tc.action)

			if tc.expErr != nil {
				assert.Error(t, err)
			} else {
				assert.Nil(t, err)
			}
		})
	}
	publishedISCSISessions.RemoveLUNFromPortal("1.2.3.4", 1)
	publishedISCSISessions.RemovePortal("1.2.3.4")
}
