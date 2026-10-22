// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockNode "github.com/netapp/trident/mocks/mock_core/mock_node"
	mockTridentController "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_tridentcontroller"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// setIPDiscoveryRetries makes IP discovery retry quickly, independent of the platform default.
func setIPDiscoveryRetries(t *testing.T) {
	originalAttempts, originalInterval := ipDiscoveryAttempts, ipDiscoveryRetryInterval
	ipDiscoveryAttempts, ipDiscoveryRetryInterval = 3, time.Millisecond
	t.Cleanup(func() { ipDiscoveryAttempts, ipDiscoveryRetryInterval = originalAttempts, originalInterval })
}

func TestDefaultIPDiscoveryAttempts(t *testing.T) {
	if runtime.GOOS == "linux" {
		assert.Equal(t, 3, defaultIPDiscoveryAttempts())
	} else {
		assert.Equal(t, 1, defaultIPDiscoveryAttempts())
	}
}

func TestDiscoverIPAddresses_NotRetriedWhenSingleAttempt(t *testing.T) {
	setIPDiscoveryRetries(t)
	ipDiscoveryAttempts = 1
	core, mocks := newTestCore(t)
	mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).
		Return(nil, errors.UnsupportedError("getIPAddresses is not supported for windows")).Times(1)

	ips, err := core.discoverIPAddresses(context.Background())

	assert.Error(t, err)
	assert.Nil(t, ips)
}

func TestDiscoverIPAddresses_FirstAttemptSucceeds(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return([]string{"10.58.43.100"}, nil).Times(1)

	ips, err := core.discoverIPAddresses(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, []string{"10.58.43.100"}, ips)
}

func TestDiscoverIPAddresses_RetriesUntilAddressesFound(t *testing.T) {
	setIPDiscoveryRetries(t)
	core, mocks := newTestCore(t)
	gomock.InOrder(
		mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return(nil, errors.New("interrupted")),
		mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return([]string{}, nil),
		mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return([]string{"10.58.43.100", "10.58.79.100"}, nil),
	)

	ips, err := core.discoverIPAddresses(context.Background())

	assert.NoError(t, err)
	assert.Equal(t, []string{"10.58.43.100", "10.58.79.100"}, ips)
}

func TestDiscoverIPAddresses_GivesUpAfterBoundedAttempts(t *testing.T) {
	setIPDiscoveryRetries(t)
	core, mocks := newTestCore(t)
	mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return([]string{}, nil).Times(ipDiscoveryAttempts)

	ips, err := core.discoverIPAddresses(context.Background())

	assert.NoError(t, err)
	assert.Empty(t, ips)
}

func TestDiscoverIPAddresses_StopsWhenContextDone(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return(nil, errors.New("interrupted")).Times(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ips, err := core.discoverIPAddresses(ctx)

	assert.Error(t, err)
	assert.Nil(t, ips)
}

func TestBuildNodeInfo_UsesRediscoveredIPAddresses(t *testing.T) {
	setIPDiscoveryRetries(t)
	core, mocks := newTestCore(t)
	mocks.OsUtils.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&models.HostSystem{}, nil).AnyTimes()
	mocks.OsUtils.EXPECT().NFSActiveOnHost(gomock.Any()).Return(true, nil).AnyTimes()
	mocks.ISCSI.EXPECT().ISCSIActiveOnHost(gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	mocks.NVMe.EXPECT().NVMeActiveOnHost(gomock.Any()).Return(false, nil).AnyTimes()
	gomock.InOrder(
		mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return(nil, errors.New("interrupted")),
		mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return([]string{"10.58.43.100"}, nil),
	)

	node := core.buildNodeInfo(context.Background())

	assert.Equal(t, []string{"10.58.43.100"}, node.IPs)
}

// TestNewController_UpdateVolumeReachable locks in the wiring that lets UpdateVolume flow through
// NewController for free: tridentcontroller.Client is embedded in controllerFromClients, so adding
// UpdateVolume to that interface required no change to NewController or controllerFromClients
// themselves - only the mock needed regenerating.
func TestNewController_UpdateVolumeReachable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := mockTridentController.NewMockClient(ctrl)
	mockChap := mockNode.NewMockChapClient(ctrl)

	names := []string{"a", "b"}
	update := &models.NodeVolumeUpdate{LUKSPassphraseNames: &names}
	mockClient.EXPECT().UpdateVolume(gomock.Any(), "vol1", update).Return(nil)

	controller := NewController(mockClient, mockChap)

	err := controller.UpdateVolume(context.Background(), "vol1", update)

	assert.NoError(t, err)
}
