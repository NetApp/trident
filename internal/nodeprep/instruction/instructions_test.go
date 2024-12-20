package instruction

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/internal/nodeprep/nodeinfo"
	"github.com/netapp/trident/internal/nodeprep/protocol"
	"github.com/netapp/trident/mocks/mock_internal/mock_nodeprep/mock_nodeinfo"
	"github.com/netapp/trident/mocks/mock_utils/mock_osutils"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/osutils"
)

func TestInstructions(t *testing.T) {
	RHELYumISCSI := newRHELYumISCSI()
	DebianAptISCSI := newDebianAptISCSI()
	YumISCSI := newYumISCSI()
	AptISCSI := newAptISCSI()

	resetInstructions := ScopedInstructions(nil)
	defer resetInstructions()

	setDefaultInstructions := func() {
		scopedInstructions := map[Key]Instructions{}
		scopedInstructions[Key{Protocol: protocol.ISCSI, Distro: nodeinfo.DistroAmzn, PkgMgr: nodeinfo.PkgMgrYum}] = RHELYumISCSI
		scopedInstructions[Key{Protocol: protocol.ISCSI, Distro: nodeinfo.DistroUbuntu, PkgMgr: nodeinfo.PkgMgrApt}] = DebianAptISCSI
		scopedInstructions[Key{Protocol: protocol.ISCSI, Distro: "", PkgMgr: nodeinfo.PkgMgrYum}] = YumISCSI
		scopedInstructions[Key{Protocol: protocol.ISCSI, Distro: "", PkgMgr: nodeinfo.PkgMgrApt}] = AptISCSI
		ScopedInstructions(scopedInstructions)
	}

	type parameters struct {
		getOSClient          func(controller *gomock.Controller) osutils.Utils
		getBinaryClient      func(controller *gomock.Controller) nodeinfo.Binary
		expectedNodeInfo     *nodeinfo.NodeInfo
		setInstructions      func()
		expectedInstructions []Instructions
		assertError          assert.ErrorAssertionFunc
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

	const fooDistro nodeinfo.Distro = "foo"

	fooHostSystemResponse := models.HostSystem{
		OS: models.SystemOS{
			Distro: fooDistro,
		},
	}

	tests := map[string]parameters{
		"node info returns supported distro ubuntu": {
			getOSClient: func(controller *gomock.Controller) osutils.Utils {
				os := mock_osutils.NewMockUtils(controller)
				os.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&ubuntuHostSystemResponse, nil)
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
				HostSystem: ubuntuHostSystemResponse,
				Distro:     nodeinfo.DistroUbuntu,
			},
			setInstructions:      setDefaultInstructions,
			expectedInstructions: []Instructions{DebianAptISCSI},
			assertError:          assert.NoError,
		},
		"node info returns supported distro amazon linux": {
			getOSClient: func(controller *gomock.Controller) osutils.Utils {
				os := mock_osutils.NewMockUtils(controller)
				os.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&amazonHostSystemResponse, nil)
				return os
			},
			getBinaryClient: func(controller *gomock.Controller) nodeinfo.Binary {
				binary := mock_nodeinfo.NewMockBinary(controller)
				binary.EXPECT().FindPath(nodeinfo.PkgMgrYum).Return("some path")
				return binary
			},
			expectedNodeInfo: &nodeinfo.NodeInfo{
				PkgMgr:     nodeinfo.PkgMgrYum,
				HostSystem: amazonHostSystemResponse,
				Distro:     nodeinfo.DistroAmzn,
			},
			expectedInstructions: []Instructions{RHELYumISCSI},
			setInstructions:      setDefaultInstructions,
			assertError:          assert.NoError,
		},
		"node info returns unknown linux distro with package manager": {
			getOSClient: func(controller *gomock.Controller) osutils.Utils {
				os := mock_osutils.NewMockUtils(controller)
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
				Distro:     fooDistro,
			},
			expectedInstructions: []Instructions{RHELYumISCSI},
			setInstructions:      setDefaultInstructions,
			assertError:          assert.NoError,
		},
		"node info returns unknown linux distro without package manager": {
			getOSClient: func(controller *gomock.Controller) osutils.Utils {
				os := mock_osutils.NewMockUtils(controller)
				os.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&fooHostSystemResponse, nil)
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
				HostSystem: fooHostSystemResponse,
				Distro:     fooDistro,
			},
			expectedInstructions: []Instructions{},
			setInstructions:      setDefaultInstructions,
			assertError:          assert.Error,
		},
		"node info on host with no package manager": {
			getOSClient: func(controller *gomock.Controller) osutils.Utils {
				os := mock_osutils.NewMockUtils(controller)
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
			expectedInstructions: nil,
			setInstructions:      setDefaultInstructions,
			assertError:          assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			node := nodeinfo.NewDetailed(params.getOSClient(ctrl), params.getBinaryClient(ctrl))
			nodeInfo, err := node.GetInfo(nil)
			params.setInstructions()
			i, err := GetInstructions(nodeInfo, []protocol.Protocol{protocol.ISCSI})
			if params.assertError != nil {
				params.assertError(t, err)
			} else {
				require.Len(t, params.expectedInstructions, 1)
				assert.Equal(t, params.expectedInstructions, i)
			}
		})
	}
}
