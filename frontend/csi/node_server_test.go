// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/brunoga/deep"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_fcp"
	"github.com/netapp/trident/utils/fcp"

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
		nodeStageVolumeRequest  csi.NodeStageVolumeRequest
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

			nodeStageVolumeResponse, err := plugin.NodeStageVolume(context.Background(), &params.nodeStageVolumeRequest)
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
		nodeStageVolumeRequest  csi.NodeStageVolumeRequest
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

			nodeStageVolumeResponse, err := plugin.nodeStageSANVolume(context.Background(), &params.nodeStageVolumeRequest)

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
		nodeStageVolumeRequest csi.NodeStageVolumeRequest
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

			err = plugin.nodeStageISCSIVolume(context.Background(), &params.nodeStageVolumeRequest, &publishInfo)
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
		request                      csi.NodeUnstageVolumeRequest
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
				mockDeviceClient.EXPECT().GetLUKSDeviceForMultipathDevice(gomock.Any()).Return(mockDevicePath, nil)
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
				mockDeviceClient.EXPECT().GetLUKSDeviceForMultipathDevice(gomock.Any()).Return(mockDevicePath, nil)
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

				mockDeviceClient.EXPECT().GetLUKSDeviceForMultipathDevice(gomock.Any()).Return(mockDevicePath, nil)
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
				mockDeviceClient.EXPECT().GetLUKSDeviceForMultipathDevice(gomock.Any()).Return(mockDevicePath, nil)
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
				return mockDeviceClient
			},
		},
		"SAN: iSCSI unstage: GetLUKSDeviceForMultipathDevice error": {
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
				mockDeviceClient.EXPECT().GetLUKSDeviceForMultipathDevice(gomock.Any()).Return("", fmt.Errorf(
					"mock error"))
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
				mockDeviceClient.EXPECT().GetLUKSDeviceForMultipathDevice(gomock.Any()).Return(mockDevicePath, nil)
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
				mockDeviceClient.EXPECT().GetLUKSDeviceForMultipathDevice(gomock.Any()).Return(mockDevicePath, nil)
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
				role: CSINode,
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

			err := plugin.nodeUnstageISCSIVolume(context.Background(), &params.request, &params.publishInfo,
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

		requests := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeStageVolume(context.Background(), &requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 2:
	// Sending n number of SAN requests of different volume.uuid, and they should succeed
	t.Run("Test Case 2: Sending n num of SAN requests with different volume.uuid and asserting noError", func(t *testing.T) {
		// SAN is not parallelized yet, so not stress testing that much as others.
		numOfRequests := 10

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeStageVolume(context.Background(), &requests[i])
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

		requests := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
			_, err := plugin.NodeStageVolume(context.Background(), &requests[0])
			assert.NoError(t, err)
		}()

		// Waiting on signal to ensure the first requests acquires the lock.
		<-signalChan

		for i := 1; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), &requests[i])
				assert.Error(t, err)
			}()
		}

		wg.Wait()
	})

	// Test4:
	// Mixing SAN and NAS requests
	t.Run("Test Case 4: Mixing SAN and NAS requests and waiting for both of them to succeed", func(t *testing.T) {
		numOfRequestsNAS := 50
		numOfRequestsSAN := 10 // SAN is not parallelized yet, not stress testing that much as others.

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		NASrequests := make([]csi.NodeStageVolumeRequest, numOfRequestsNAS)
		SANrequests := make([]csi.NodeStageVolumeRequest, numOfRequestsSAN)
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
				_, err := plugin.NodeStageVolume(context.Background(), &NASrequests[i])
				assert.NoError(t, err)
			}()
		}
		// Spinning up go-routine for the SAN requests.
		for i := 0; i < numOfRequestsSAN; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), &SANrequests[i])
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

		NFSrequests := make([]csi.NodeStageVolumeRequest, numOfRequests)
		SMBrequests := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeStageVolume(context.Background(), &NFSrequests[i])
				assert.NoError(t, err)
			}()

			// Spinning up go-rotuine for SMB requests.
			go func() {
				defer wg.Done()
				_, err := plugin.NodeStageVolume(context.Background(), &SMBrequests[i])
				assert.NoError(t, err)
			}()
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

		requests := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
				_, err := plugin.nodeStageNFSVolume(ctx, &requests[i])
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

		requests := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
				_, err := plugin.nodeStageNFSVolume(ctx, &requests[i])
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
				_, err := plugin.nodeStageNFSVolume(ctx, &requests[i])
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

		requests := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
				_, err := plugin.nodeStageNFSVolume(ctx, &requests[i])
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
				_, err := plugin.nodeStageNFSVolume(ctx, &requests[i])
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

		requests := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
				_, err := plugin.nodeStageSMBVolume(ctx, &requests[i])
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

		requests := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
				_, err := plugin.nodeStageSMBVolume(ctx, &requests[i])
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
				_, err := plugin.nodeStageSMBVolume(ctx, &requests[i])
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

		requests := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
				_, err := plugin.nodeStageSMBVolume(ctx, &requests[i])
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
				_, err := plugin.nodeStageSMBVolume(ctx, &requests[i])
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

		requests := func() []csi.NodeUnstageVolumeRequest {
			requests := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeUnstageVolume(context.Background(), &requests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})

	// Test 2:
	// Sending n number of SAN requests of different volume.uuid, and they should succeed
	t.Run("Test Case 2: Sending n num of SAN requests with different volume.uuid and asserting noError", func(t *testing.T) {
		numOfRequests := 10

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		requests := func() []csi.NodeUnstageVolumeRequest {
			requests := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeUnstageVolume(context.Background(), &requests[i])
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

		requests := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
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
			role:             CSINode,
			limiterSharedMap: make(map[string]limiter.Limiter),
			nodeHelper:       mockTrackingClient,
		}

		plugin.InitializeNodeLimiter(ctx)

		var wg sync.WaitGroup
		wg.Add(numOfRequests)

		go func() {
			defer wg.Done()
			_, err := plugin.NodeUnstageVolume(context.Background(), &requests[0])
			assert.NoError(t, err)
		}()

		<-signalChan

		for i := 1; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnstageVolume(context.Background(), &requests[i])
				assert.Error(t, err)
			}()
		}

		wg.Wait()
	})

	// Test4:
	// Mixing SAN and NAS requests
	t.Run("Test Case 4: Mixing SAN and NAS requests and waiting for both of them to succeed", func(t *testing.T) {
		numOfRequestsNAS := 50
		numOfRequestsSAN := 10

		defer func(csiNodeLockTimeoutTemp, csiKubeletTimeoutTemp time.Duration) {
			csiNodeLockTimeout = csiNodeLockTimeoutTemp
			csiKubeletTimeout = csiKubeletTimeoutTemp
		}(csiNodeLockTimeout, csiKubeletTimeout)

		NASrequests := make([]csi.NodeUnstageVolumeRequest, numOfRequestsNAS)
		SANrequests := make([]csi.NodeUnstageVolumeRequest, numOfRequestsSAN)

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
				_, err := pluginNAS.NodeUnstageVolume(context.Background(), &NASrequests[i])
				assert.NoError(t, err)
			}()
		}

		for i := 0; i < numOfRequestsSAN; i++ {
			go func() {
				defer wg.Done()
				_, err := pluginSAN.NodeUnstageVolume(context.Background(), &SANrequests[i])
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

		NFSrequests := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
		SMBrequests := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
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
				_, err := pluginNFS.NodeUnstageVolume(context.Background(), &NFSrequests[i])
				assert.NoError(t, err)
			}()
		}

		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := pluginSMB.NodeUnstageVolume(context.Background(), &SMBrequests[i])
				assert.NoError(t, err)
			}()
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

		requests := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodeUnstageVolumeRequestBuilder().WithVolumeID(uuid.New().String()).Build()
		}

		mockTrackingClient := mockNodeHelpers.NewMockNodeHelper(gomock.NewController(t))

		for i := 0; i < numOfRequests; i++ {
			mockTrackingClient.EXPECT().DeleteTrackingInfo(gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
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
				_, err := plugin.nodeUnstageNFSVolume(ctx, &requests[i])
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

		requests := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
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
				_, err := plugin.nodeUnstageNFSVolume(ctx, &requests[i])
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
				_, err := plugin.nodeUnstageNFSVolume(ctx, &requests[i])
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

		requests := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
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
				_, err := plugin.nodeUnstageNFSVolume(ctx, &requests[i])
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
				_, err := plugin.nodeUnstageNFSVolume(ctx, &requests[i])
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

		requests := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
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
				_, err := plugin.nodeUnstageSMBVolume(ctx, &requests[i])
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

		requests := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
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
				_, err := plugin.nodeUnstageSMBVolume(ctx, &requests[i])
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
				_, err := plugin.nodeUnstageSMBVolume(ctx, &requests[i])
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

		requests := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
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
				_, err := plugin.nodeUnstageSMBVolume(ctx, &requests[i])
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
				_, err := plugin.nodeUnstageSMBVolume(ctx, &requests[i])
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

		requests := func() []csi.NodePublishVolumeRequest {
			requests := make([]csi.NodePublishVolumeRequest, numOfRequests)
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
				_, err := plugin.NodePublishVolume(context.Background(), &requests[i])
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

		requests := make([]csi.NodePublishVolumeRequest, numOfRequests)
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
			_, err := plugin.NodePublishVolume(context.Background(), &requests[0])
			assert.NoError(t, err)
		}()

		// Waiting on signal to ensure the first requests acquires the lock.
		<-signalChan

		for i := 1; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodePublishVolume(context.Background(), &requests[i])
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

		NFSrequests := make([]csi.NodePublishVolumeRequest, numOfRequests)
		SMBrequests := make([]csi.NodePublishVolumeRequest, numOfRequests)
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
				_, err := pluginNFS.NodePublishVolume(context.Background(), &NFSrequests[i])
				assert.NoError(t, err)
			}()
		}

		// Spinning up go-routine for SMB requests.
		for i := 0; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := pluginSMB.NodePublishVolume(context.Background(), &SMBrequests[i])
				assert.NoError(t, err)
			}()
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

		requests := make([]csi.NodePublishVolumeRequest, numOfRequests)
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
				_, err := plugin.nodePublishNFSVolume(ctx, &requests[i])
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

		requests := make([]csi.NodePublishVolumeRequest, numOfRequests)
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
				_, err := plugin.nodePublishNFSVolume(ctx, &requests[i])
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
				_, err := plugin.nodePublishNFSVolume(ctx, &requests[i])
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

		requests := make([]csi.NodePublishVolumeRequest, numOfRequests)
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
				_, err := plugin.nodePublishNFSVolume(ctx, &requests[i])
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
				_, err := plugin.nodePublishNFSVolume(ctx, &requests[i])
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

		requests := make([]csi.NodePublishVolumeRequest, numOfRequests)
		for i := 0; i < numOfRequests; i++ {
			requests[i] = NewNodePublishVolumeRequestBuilder(SMBNodePublishVolumeRequestType).Build()
		}

		mockMount := mock_mount.NewMockMount(gomock.NewController(t))

		for i := 0; i < numOfRequests; i++ {
			mockMount.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil)
			mockMount.EXPECT().WindowsBindMount(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
		}

		plugin := &Plugin{
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
				_, err := plugin.nodePublishSMBVolume(ctx, &requests[i])
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

		requests := make([]csi.NodePublishVolumeRequest, numOfRequests)
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
				_, err := plugin.nodePublishSMBVolume(ctx, &requests[i])
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
				_, err := plugin.nodePublishSMBVolume(ctx, &requests[i])
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

		requests := make([]csi.NodePublishVolumeRequest, numOfRequests)
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
				_, err := plugin.nodePublishSMBVolume(ctx, &requests[i])
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
				_, err := plugin.nodePublishSMBVolume(ctx, &requests[i])
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

		requests := func() []csi.NodeUnpublishVolumeRequest {
			requests := make([]csi.NodeUnpublishVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeUnpublishVolume(context.Background(), &requests[i])
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

		requests := make([]csi.NodeUnpublishVolumeRequest, numOfRequests)
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
			_, err := plugin.NodeUnpublishVolume(context.Background(), &requests[0])
			assert.NoError(t, err)
		}()

		// Waiting on signal to ensure the first requests acquires the lock.
		<-signalChan

		for i := 1; i < numOfRequests; i++ {
			go func() {
				defer wg.Done()
				_, err := plugin.NodeUnpublishVolume(context.Background(), &requests[i])
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

		requests := make([]csi.NodeUnpublishVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeUnpublishVolume(ctx, &requests[i])
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
				_, err := plugin.NodeUnpublishVolume(ctx, &requests[i])
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

		requests := make([]csi.NodeUnpublishVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeUnpublishVolume(ctx, &requests[i])
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
				_, err := plugin.NodeUnpublishVolume(ctx, &requests[i])
				assert.NoError(t, err)
			}()
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
		nodeStageRequests := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeStageVolume(context.Background(), &nodeStageRequests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()

		// ---- Unstaging all those previously staged nodeStageRequests ----
		nodeUnstageRequests := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeUnstageVolume(context.Background(), &nodeUnstageRequests[volumeIDs[i]])
				assert.NoError(t, err)
			}()
		}

		// ---- Parallely creating another batch of 100 nodeStage requests ----
		nodeStageRequests2 := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeStageVolume(context.Background(), &nodeStageRequests2[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()

		// ---- Unstaging all those previously staged nodeStageRequests of 2nd batch ----
		nodeUnstageRequests2 := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeUnstageVolume(context.Background(), &nodeUnstageRequests2[volumeIDs[i]])
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
		nodeStageRequests := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeStageVolume(context.Background(), &nodeStageRequests[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()

		// ---- Unstaging all those previously staged nodeStageRequests ----
		nodeUnstageRequests := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeUnstageVolume(context.Background(), &nodeUnstageRequests[volumeIDs[i]])
				assert.NoError(t, err)
			}()
		}

		// ---- Parallely creating another batch of 100 nodeStage requests ----
		nodeStageRequests2 := make([]csi.NodeStageVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeStageVolume(context.Background(), &nodeStageRequests2[i])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()

		// ---- Unstaging all those previously staged nodeStageRequests of 2nd batch ----
		nodeUnstageRequests2 := make([]csi.NodeUnstageVolumeRequest, numOfRequests)
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
				_, err := plugin.NodeUnstageVolume(context.Background(), &nodeUnstageRequests2[volumeIDs[i]])
				assert.NoError(t, err)
			}()
		}

		wg.Wait()
	})
}

// ----- helpers ----
type NodeStageVolumeRequestBuilder struct {
	request csi.NodeStageVolumeRequest
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
			request: csi.NodeStageVolumeRequest{
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
			request: csi.NodeStageVolumeRequest{
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
			request: csi.NodeStageVolumeRequest{
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
			request: csi.NodeStageVolumeRequest{
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

func (builder *NodeStageVolumeRequestBuilder) Build() csi.NodeStageVolumeRequest {
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

func (builder *NodeUnstageVolumeRequestBuilder) Build() csi.NodeUnstageVolumeRequest {
	return builder.request
}

type VolumePublishInfoBuilder struct {
	publishInfo models.VolumePublishInfo
}

type VolumePublishInfoType int

const (
	TypeiSCSIVolumePublishInfo VolumePublishInfoType = iota
	TypeNFSVolumePublishInfo
	TypeSMBVolumePublishInfo
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
	request csi.NodePublishVolumeRequest
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
			request: csi.NodePublishVolumeRequest{
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
				Readonly: false,
			},
		}

	case SMBNodePublishVolumeRequestType:
		return &NodePublishVolumeRequestBuilder{
			request: csi.NodePublishVolumeRequest{
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
	default:
		panic("unhandled default case")
	}
}

func (p *NodePublishVolumeRequestBuilder) Build() csi.NodePublishVolumeRequest {
	return p.request
}

type NodeUnpublishVolumeRequestBuilder struct {
	request csi.NodeUnpublishVolumeRequest
}

func NewNodeUnpublishVolumeRequestBuilder() *NodeUnpublishVolumeRequestBuilder {
	return &NodeUnpublishVolumeRequestBuilder{
		request: csi.NodeUnpublishVolumeRequest{
			VolumeId:   uuid.NewString(),
			TargetPath: "/tmp/raw",
		},
	}
}

func (u *NodeUnpublishVolumeRequestBuilder) Build() csi.NodeUnpublishVolumeRequest {
	return u.request
}

func TestNodeStageFCPVolume(t *testing.T) {
	type parameters struct {
		getFCPClient           func() fcp.FCP
		getNodeHelper          func() nodehelpers.NodeHelper
		getRestClient          func() controllerAPI.TridentController
		nodeStageVolumeRequest csi.NodeStageVolumeRequest
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
				role:   CSINode,
				aesKey: params.aesKey,
				fs:     filesystem.New(mountClient),
			}

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

			err = plugin.nodeStageFCPVolume(context.Background(), &params.nodeStageVolumeRequest, &publishInfo)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}
