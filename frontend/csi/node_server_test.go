// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/mitchellh/copystructure"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	controllerAPI "github.com/netapp/trident/frontend/csi/controller_api"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	"github.com/netapp/trident/internal/crypto"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	mockControllerAPI "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_api"
	mockNodeHelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	mockUtils "github.com/netapp/trident/mocks/mock_utils"
	"github.com/netapp/trident/mocks/mock_utils/mock_devices"
	"github.com/netapp/trident/mocks/mock_utils/mock_iscsi"
	"github.com/netapp/trident/mocks/mock_utils/mock_mount"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount"
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

	request := NewNodeStageVolumeRequestBuilder().Build()

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
				role: CSINode,
				fs:   filesystem.New(mountClient),
			}

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

	noCapabilitiesRequest := NewNodeStageVolumeRequestBuilder().WithVolumeCapability(&csi.VolumeCapability{}).Build()
	fileSystemRawMountCapabilityRequest := NewNodeStageVolumeRequestBuilder().WithFileSystemType(filesystem.Raw).Build()
	fileSystemExt4BlockCapabilityRequest := NewNodeStageVolumeRequestBuilder().WithVolumeCapability(
		&csi.VolumeCapability{
			AccessType: &csi.VolumeCapability_Block{
				Block: &csi.VolumeCapability_BlockVolume{},
			},
		},
	).Build()

	badSharedTargetRequest := NewNodeStageVolumeRequestBuilder().WithSharedTarget("foo").Build()

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
				role: CSINode,
			}

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

	request := NewNodeStageVolumeRequestBuilder().Build()
	badUseChapRequest := NewNodeStageVolumeRequestBuilder().WithUseCHAP("foo").Build()
	badLunNumberRequest := NewNodeStageVolumeRequestBuilder().WithIscsiLunNumber("foo").Build()
	badLuksEncryptionRequest := NewNodeStageVolumeRequestBuilder().WithLUKSEncryption("foo").Build()
	badTargetPortalCountRequest := NewNodeStageVolumeRequestBuilder().WithIscsiTargetPortalCount("foo").Build()
	zeroTargetPortalCountRequest := NewNodeStageVolumeRequestBuilder().WithIscsiTargetPortalCount("0").Build()
	targetPortalCountThreeMissingPortalRequest := NewNodeStageVolumeRequestBuilder().WithIscsiTargetPortalCount("3").Build()
	chapBadEncryptedISCSIUserNameRequest := NewNodeStageVolumeRequestBuilder().WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiUsername("foo").Build()
	chapBadEncryptedIscsiInitiatorSecretRequest := NewNodeStageVolumeRequestBuilder().WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiInitiatorSecret("foo").Build()
	chapBadEncryptedIscsiTargetUsernameRequest := NewNodeStageVolumeRequestBuilder().WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiTargetUsername("foo").Build()
	chapBadEncryptedIscsiTargetSecretRequest := NewNodeStageVolumeRequestBuilder().WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiTargetSecret("foo").Build()
	badVolumeIdRequest := NewNodeStageVolumeRequestBuilder().WithVolumeID("").Build()
	badStagingPathRequest := NewNodeStageVolumeRequestBuilder().WithStagingTargetPath("").Build()

	const mockAESKey = "thisIsMockAESKey"
	encryptedValue, err := crypto.EncryptStringWithAES("mockValue", []byte(mockAESKey))
	assert.Nil(t, err)

	chapEncryptedISCSIUserNameRequest := NewNodeStageVolumeRequestBuilder().WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiUsername(encryptedValue).Build()
	chapEncryptedIscsiInitiatorSecretRequest := NewNodeStageVolumeRequestBuilder().WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiInitiatorSecret(encryptedValue).Build()
	chapEncryptedIscsiTargetUsernameRequest := NewNodeStageVolumeRequestBuilder().WithIscsiTargetPortalCount("1").
		WithUseCHAP("true").WithEncryptedIscsiTargetUsername(encryptedValue).Build()
	chapEncryptedIscsiTargetSecretRequest := NewNodeStageVolumeRequestBuilder().WithIscsiTargetPortalCount("1").
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
				role:   CSINode,
				aesKey: params.aesKey,
				fs:     filesystem.New(mountClient),
			}

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
			ISCSITargetIQN: iqnList[0],
			Credentials:    chapCredentials[2],
		},
		LUNs: models.LUNs{
			Info: mapCopyHelper(lunList1),
		},
	}

	sessionData2 := models.ISCSISessionData{
		PortalInfo: models.PortalInfo{
			ISCSITargetIQN: iqnList[1],
			Credentials:    chapCredentials[2],
		},
		LUNs: models.LUNs{
			Info: mapCopyHelper(lunList2),
		},
	}

	type PreRun func(publishedSessions, currentSessions *models.ISCSISessions, portalActions []PortalAction)

	inputs := []struct {
		TestName           string
		PublishedPortals   *models.ISCSISessions
		CurrentPortals     *models.ISCSISessions
		PortalActions      []PortalAction
		StopAt             time.Time
		AddNewNodeOps      bool // If there exist a new node operation would request lock.
		SimulateConditions PreRun
		PortalsFixed       []string
	}{
		{
			TestName: "No current sessions exist then all the non-stale sessions are fixed",
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
			TestName: "No current sessions exist AND self-heal exceeded max time AND NO node operation waiting then" +
				" all the non-stale sessions are fixed",
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
			TestName: "No current sessions exist AND exist a node operation waiting then first non-stale sessions is fixed",
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
			PortalsFixed:  []string{ipList[1]},
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
			TestName: "No current sessions exist AND self-heal exceeded max time AND exist a node operation waiting" +
				" for lock then first non-stale sessions is fixed",
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
			StopAt:        time.Time{},
			AddNewNodeOps: true,
			PortalsFixed:  []string{ipList[1]},
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
			TestName: "Current sessions exist but missing LUNs then all the non-stale sessions are fixed",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
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

				currentSessions.Info[ipList[0]].LUNs = models.LUNs{
					Info: nil,
				}
				currentSessions.Info[ipList[1]].LUNs = models.LUNs{
					Info: nil,
				}
				currentSessions.Info[ipList[2]].LUNs = models.LUNs{
					Info: nil,
				}

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "Current sessions exist but missing LUNs AND exist a node operation waiting" +
				" for lock then first non-stale sessions is fixed",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: models.NoAction},
				{Portal: ipList[1], Action: models.NoAction},
				{Portal: ipList[2], Action: models.NoAction},
			},
			StopAt:        time.Now().Add(100 * time.Second),
			AddNewNodeOps: true,
			PortalsFixed:  []string{ipList[1]},
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow.Add(5 * time.Millisecond)
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(10 * time.Millisecond)

				currentSessions.Info[ipList[0]].LUNs = models.LUNs{
					Info: nil,
				}
				currentSessions.Info[ipList[1]].LUNs = models.LUNs{
					Info: nil,
				}
				currentSessions.Info[ipList[2]].LUNs = models.LUNs{
					Info: nil,
				}

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "Current sessions are stale then all the stale sessions are fixed",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: models.LogoutLoginScan},
				{Portal: ipList[1], Action: models.LogoutLoginScan},
				{Portal: ipList[2], Action: models.LogoutLoginScan},
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
			TestName: "Current sessions are stale AND only exist a node operation waiting" +
				" for lock BUT self-heal has not exceeded then all stale sessions are fixed",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: models.LogoutLoginScan},
				{Portal: ipList[1], Action: models.LogoutLoginScan},
				{Portal: ipList[2], Action: models.LogoutLoginScan},
			},
			StopAt:        time.Now().Add(100 * time.Second),
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
			TestName: "Current sessions are stale AND exist a node operation waiting" +
				" for lock AND self-heal exceeds time then first stale sessions is fixed",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: models.LogoutLoginScan},
				{Portal: ipList[1], Action: models.LogoutLoginScan},
				{Portal: ipList[2], Action: models.LogoutLoginScan},
			},
			StopAt:        time.Time{},
			AddNewNodeOps: true,
			PortalsFixed:  []string{ipList[1]},
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
			publishedISCSISessions = *input.PublishedPortals
			currentISCSISessions = *input.CurrentPortals

			input.SimulateConditions(input.PublishedPortals, input.CurrentPortals, input.PortalActions)
			portals := getPortals(input.PublishedPortals, input.PortalActions)

			if input.AddNewNodeOps {
				go utils.Lock(ctx, "test-lock1", lockID)
				snooze(10)
				go utils.Lock(ctx, "test-lock2", lockID)
				snooze(10)
			}

			// Make sure this time is captured after the pre-run adds wait time
			// Also on Windows the system time is often only updated once every
			// 10-15 ms or so, which means if you query the current time twice
			// within this period, you get the same value. Therefore, set this
			// time to be slightly lower than time set in fixISCSISessions call.
			timeNow := time.Now().Add(-2 * time.Millisecond)

			nodeServer.fixISCSISessions(context.TODO(), portals, "some-portal", input.StopAt)

			for _, portal := range portals {
				lastAccessTime := publishedISCSISessions.Info[portal].PortalInfo.LastAccessTime
				if collection.ContainsString(input.PortalsFixed, portal) {
					assert.True(t, lastAccessTime.After(timeNow),
						fmt.Sprintf("mismatched last access time for %v portal", portal))
				} else {
					assert.True(t, lastAccessTime.Before(timeNow),
						fmt.Sprintf("mismatched lass access time for %v portal", portal))
				}
			}

			if input.AddNewNodeOps {
				utils.Unlock(ctx, "test-lock1", lockID)

				// Wait for the lock to be released
				for utils.WaitQueueSize(lockID) > 1 {
					snooze(10)
				}

				// Give some time for another context to acquire the lock
				snooze(100)
				utils.Unlock(ctx, "test-lock2", lockID)
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

	utils.SortPortals(portals, sessions)

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
	clone, err := copystructure.Copy(input)
	if err != nil {
		return &models.ISCSISessionData{}
	}

	output, ok := clone.(models.ISCSISessionData)
	if !ok {
		return &models.ISCSISessionData{}
	}

	return &output
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
	mockNVMeHandler := mockUtils.NewMockNVMeInterface(mockCtrl)
	nodeServer := &Plugin{nvmeHandler: mockNVMeHandler}

	// Empty Published sessions case.
	nodeServer.performNVMeSelfHealing(ctx)

	// Error populating current sessions.
	publishedNVMeSessions.AddNVMeSession(utils.NVMeSubsystem{NQN: "nqn"}, []string{})
	mockNVMeHandler.EXPECT().PopulateCurrentNVMeSessions(ctx, gomock.Any()).
		Return(errors.New("failed to populate current sessions"))

	nodeServer.performNVMeSelfHealing(ctx)

	// Self-healing process done.
	mockNVMeHandler.EXPECT().PopulateCurrentNVMeSessions(ctx, gomock.Any()).Return(nil)
	mockNVMeHandler.EXPECT().InspectNVMeSessions(ctx, gomock.Any(), gomock.Any()).Return([]utils.NVMeSubsystem{})

	nodeServer.performNVMeSelfHealing(ctx)
	// Cleanup of global objects.
	publishedNVMeSessions.RemoveNVMeSession("nqn")
}

func TestFixNVMeSessions(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockNVMeHandler := mockUtils.NewMockNVMeInterface(mockCtrl)
	nodeServer := &Plugin{nvmeHandler: mockNVMeHandler}
	subsystem1 := utils.NVMeSubsystem{NQN: "nqn1"}
	subsystems := []utils.NVMeSubsystem{subsystem1}

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
	expected := attemptLock(ctx, lockContext, lockTimeout)

	// start the second request so that it is in race for the lock
	go func() {
		defer wg.Done()
		ctx := context.Background()
		lockContext := "fakeLockContext-req2"
		expected := attemptLock(ctx, lockContext, lockTimeout)

		assert.False(t, expected)
		utils.Unlock(ctx, lockContext, lockID)
	}()
	// first request goes to sleep holding the lock
	if expected {
		time.Sleep(500 * time.Millisecond)
	}
	utils.Unlock(ctx, lockContext, lockID)
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
	expected := attemptLock(ctx, lockContext, lockTimeout)

	// start the second request so that it is in race for the lock
	go func() {
		defer wg.Done()
		ctx := context.Background()
		lockContext := "fakeLockContext-req2"
		lockTimeout := 5 * time.Second

		expected := attemptLock(ctx, lockContext, lockTimeout)

		assert.True(t, expected)
		utils.Unlock(ctx, lockContext, lockID)
	}()
	// first request goes to sleep holding the lock
	if expected {
		time.Sleep(200 * time.Millisecond)
	}
	utils.Unlock(ctx, lockContext, lockID)
	wg.Wait()
}

func TestOutdatedAccessControlInUse(t *testing.T) {
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
	mockNVMeHandler := mockUtils.NewMockNVMeInterface(mockCtrl)

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
	mockNVMeHandler := mockUtils.NewMockNVMeInterface(mockCtrl)
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
	mockNVMeHandler := mockUtils.NewMockNVMeInterface(mockCtrl)
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
			publishInfo: NewVolumePublishInfoBuilder().Build(),
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
			publishInfo: NewVolumePublishInfoBuilder().WithLUKSEncryption("true").Build(),
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
		"SAN: iSCSI unstage: GetUnderlyingDevicePathForLUKSDevice error": {
			assertError: assert.NoError,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder().WithLUKSEncryption("true").Build(),
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
			publishInfo: NewVolumePublishInfoBuilder().WithLUKSEncryption("true").Build(),
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
			publishInfo: NewVolumePublishInfoBuilder().WithLUKSEncryption("true").Build(),
			getIscsiReconcileUtilsClient: func() iscsi.IscsiReconcileUtils {
				mockIscsiReconcileUtilsClient := mock_iscsi.NewMockIscsiReconcileUtils(gomock.NewController(t))
				mockIscsiReconcileUtilsClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{})
				return mockIscsiReconcileUtilsClient
			},
		},
		"SAN: iSCSI unstage: GetDeviceInfoForLUN no devices": {
			assertError: assert.NoError,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder().WithLUKSEncryption("true").Build(),
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
				return mockISCSIClient
			},
			getDeviceClient: func() devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(gomock.NewController(t))
				return mockDeviceClient
			},
		},
		"SAN: iSCSI unstage: GetDeviceInfoForLUN error": {
			assertError: assert.Error,
			request:     NewNodeUnstageVolumeRequestBuilder().Build(),
			publishInfo: NewVolumePublishInfoBuilder().WithLUKSEncryption("true").Build(),
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
			publishInfo: NewVolumePublishInfoBuilder().WithLUKSEncryption("true").Build(),
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
			publishInfo: NewVolumePublishInfoBuilder().WithLUKSEncryption("true").Build(),
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
			publishInfo: NewVolumePublishInfoBuilder().WithLUKSEncryption("true").Build(),
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
			publishInfo: NewVolumePublishInfoBuilder().Build(),
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
			publishInfo: NewVolumePublishInfoBuilder().Build(),
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
			publishInfo: NewVolumePublishInfoBuilder().Build(),
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
			publishInfo: NewVolumePublishInfoBuilder().Build(),
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
			publishInfo: NewVolumePublishInfoBuilder().Build(),
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

// ----- helpers ----
type NodeStageVolumeRequestBuilder struct {
	request csi.NodeStageVolumeRequest
}

func NewNodeStageVolumeRequestBuilder() *NodeStageVolumeRequestBuilder {
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

func (builder *NodeStageVolumeRequestBuilder) WithLUKSEncryption(luksEncryption string) *NodeStageVolumeRequestBuilder {
	builder.request.PublishContext["LUKSEncryption"] = luksEncryption
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

func NewVolumePublishInfoBuilder() *VolumePublishInfoBuilder {
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
