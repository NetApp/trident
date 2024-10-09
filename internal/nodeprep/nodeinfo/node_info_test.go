// Copyright 2024 NetApp, Inc. All Rights Reserved.

package nodeinfo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/internal/nodeprep/nodeinfo"
	"github.com/netapp/trident/mocks/mock_internal/mock_nodeprep/mock_nodeinfo"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func TestNew(t *testing.T) {
	node := nodeinfo.New()
	assert.NotNil(t, node)
}

func TestNewDetailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	os := mock_nodeinfo.NewMockOS(ctrl)
	binary := mock_nodeinfo.NewMockBinary(ctrl)
	node := nodeinfo.NewDetailed(os, binary)
	assert.NotNil(t, node)
}

func TestNode_GetInfo(t *testing.T) {
	type parameters struct {
		getOSClient      func(controller *gomock.Controller) nodeinfo.OS
		getBinaryClient  func(controller *gomock.Controller) nodeinfo.Binary
		expectedNodeInfo *nodeinfo.NodeInfo
		assertError      assert.ErrorAssertionFunc
	}

	ubuntuHostSystemResponse := models.HostSystem{
		OS: models.SystemOS{
			Distro: nodeinfo.DistroUbuntu,
		},
	}

	amazonHostSystemResponse := models.HostSystem{
		OS: models.SystemOS{
			Distro: nodeinfo.DistroAmzn,
		},
	}

	fooHostSystemResponse := models.HostSystem{
		OS: models.SystemOS{
			Distro: "",
		},
	}

	tests := map[string]parameters{
		"error getting host system info": {
			getOSClient: func(controller *gomock.Controller) nodeinfo.OS {
				os := mock_nodeinfo.NewMockOS(controller)
				os.EXPECT().GetHostSystemInfo(gomock.Any()).Return(nil, errors.New("some error"))
				return os
			},
			getBinaryClient: func(controller *gomock.Controller) nodeinfo.Binary {
				binary := mock_nodeinfo.NewMockBinary(controller)
				return binary
			},
			expectedNodeInfo: nil,
			assertError:      assert.Error,
		},
		"node info returns unsupported distro": {
			getOSClient: func(controller *gomock.Controller) nodeinfo.OS {
				os := mock_nodeinfo.NewMockOS(controller)
				os.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&fooHostSystemResponse, nil)
				return os
			},
			getBinaryClient: func(controller *gomock.Controller) nodeinfo.Binary {
				binary := mock_nodeinfo.NewMockBinary(controller)
				binary.EXPECT().FindPath(nodeinfo.PkgMgrYum).Return("some path")
				return binary
			},
			expectedNodeInfo: &nodeinfo.NodeInfo{
				PkgMgr:     nodeinfo.PkgMgrYum,
				HostSystem: fooHostSystemResponse,
				Distro:     nodeinfo.DistroUnknown,
			},
			assertError: assert.NoError,
		},
		"node info returns supported distro ubuntu": {
			getOSClient: func(controller *gomock.Controller) nodeinfo.OS {
				os := mock_nodeinfo.NewMockOS(controller)
				os.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&ubuntuHostSystemResponse, nil)
				return os
			},
			getBinaryClient: func(controller *gomock.Controller) nodeinfo.Binary {
				binary := mock_nodeinfo.NewMockBinary(controller)
				binary.EXPECT().FindPath(nodeinfo.PkgMgrYum).Return("some path")
				return binary
			},
			expectedNodeInfo: &nodeinfo.NodeInfo{
				PkgMgr:     nodeinfo.PkgMgrYum,
				HostSystem: ubuntuHostSystemResponse,
				Distro:     nodeinfo.DistroUbuntu,
			},
			assertError: assert.NoError,
		},
		"node info returns supported distro amazon linux": {
			getOSClient: func(controller *gomock.Controller) nodeinfo.OS {
				os := mock_nodeinfo.NewMockOS(controller)
				os.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&amazonHostSystemResponse, nil)
				return os
			},
			getBinaryClient: func(controller *gomock.Controller) nodeinfo.Binary {
				binary := mock_nodeinfo.NewMockBinary(controller)
				binary.EXPECT().FindPath(nodeinfo.PkgMgrYum).Return("")
				binary.EXPECT().FindPath(nodeinfo.PkgMgrApt).Return("some path")
				return binary
			},
			expectedNodeInfo: &nodeinfo.NodeInfo{
				PkgMgr:     nodeinfo.PkgMgrApt,
				HostSystem: amazonHostSystemResponse,
				Distro:     nodeinfo.DistroAmzn,
			},
			assertError: assert.NoError,
		},
		"node info on host with no package manager": {
			getOSClient: func(controller *gomock.Controller) nodeinfo.OS {
				os := mock_nodeinfo.NewMockOS(controller)
				os.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&ubuntuHostSystemResponse, nil)
				return os
			},
			getBinaryClient: func(controller *gomock.Controller) nodeinfo.Binary {
				binary := mock_nodeinfo.NewMockBinary(controller)
				binary.EXPECT().FindPath(nodeinfo.PkgMgrYum).Return("")
				binary.EXPECT().FindPath(nodeinfo.PkgMgrApt).Return("")
				return binary
			},
			expectedNodeInfo: &nodeinfo.NodeInfo{
				PkgMgr:     nodeinfo.PkgMgrNone,
				HostSystem: ubuntuHostSystemResponse,
				Distro:     nodeinfo.DistroUbuntu,
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			node := nodeinfo.NewDetailed(params.getOSClient(ctrl), params.getBinaryClient(ctrl))
			nodeInfo, err := node.GetInfo(nil)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedNodeInfo, nodeInfo)
		})
	}
}
