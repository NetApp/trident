// Copyright 2026 NetApp, Inc. All Rights Reserved.

package node

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/frontend/csi/tridentcontroller"
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

	ips, err := core.discoverIPAddresses(context.Background(), true)

	assert.Error(t, err)
	assert.Nil(t, ips)
}

func TestDiscoverIPAddresses_FirstAttemptSucceeds(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return([]string{"10.58.43.100"}, nil).Times(1)

	ips, err := core.discoverIPAddresses(context.Background(), true)

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

	ips, err := core.discoverIPAddresses(context.Background(), true)

	assert.NoError(t, err)
	assert.Equal(t, []string{"10.58.43.100", "10.58.79.100"}, ips)
}

func TestDiscoverIPAddresses_GivesUpAfterBoundedAttempts(t *testing.T) {
	setIPDiscoveryRetries(t)
	core, mocks := newTestCore(t)
	mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return([]string{}, nil).Times(ipDiscoveryAttempts)

	ips, err := core.discoverIPAddresses(context.Background(), true)

	assert.NoError(t, err)
	assert.Empty(t, ips)
}

func TestDiscoverIPAddresses_StopsWhenContextDone(t *testing.T) {
	core, mocks := newTestCore(t)
	mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return(nil, errors.New("interrupted")).Times(1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ips, err := core.discoverIPAddresses(ctx, true)

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

	node, _ := core.buildNodeInfo(context.Background(), false)

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

func TestRefreshNodeInfo(t *testing.T) {
	const nqn1 = "nqn.2014-08.org.nvmexpress:uuid:host1"
	const nqn2 = "nqn.2014-08.org.nvmexpress:uuid:host2"
	ips1, ips12 := []string{"192.0.2.1"}, []string{"192.0.2.1", "192.0.2.2"}

	tests := []struct {
		name          string
		before, after fakeHost
		wantRegister  bool
	}{
		{"unchanged identity", fakeHost{ips1, nqn1}, fakeHost{ips1, nqn1}, false},
		{"NQN appears", fakeHost{ips1, ""}, fakeHost{ips1, nqn1}, true},
		{"NQN changes", fakeHost{ips1, nqn1}, fakeHost{ips1, nqn2}, true},
		{"IP address added", fakeHost{ips1, nqn1}, fakeHost{ips12, nqn1}, true},
		{"partial IP discovery keeps the registered IPs", fakeHost{ips12, nqn1}, fakeHost{ips1, nqn1}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			core, mocks := newTestCore(t)
			host := tc.before
			expectHost(mocks, &host)
			core.registeredNode, _ = core.buildNodeInfo(context.Background(), false)
			host = tc.after
			want := tc.before
			if tc.wantRegister {
				want = tc.after
				mocks.Controller.MockClient.EXPECT().RegisterNode(gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&tridentcontroller.RegistrationInfo{}, nil)
			}

			assert.NoError(t, core.register(context.Background(), nodeInfoRefreshInterval, false))
			assert.Equal(t, want.ips, core.registeredNode.IPs)
			assert.Equal(t, want.nqn, core.registeredNode.NQN)
		})
	}

	t.Run("a successful probe drops a stopped service", func(t *testing.T) {
		core, mocks := newTestCore(t)
		host := fakeHost{ips1, nqn1}
		expectHost(mocks, &host)
		core.registeredNode, _ = core.buildNodeInfo(context.Background(), false)
		// The NVMe probe now succeeds and reports NVMe inactive, so the host also reports no NQN.
		host.nqn = ""
		mocks.Controller.MockClient.EXPECT().RegisterNode(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&tridentcontroller.RegistrationInfo{}, nil)

		assert.NoError(t, core.register(context.Background(), nodeInfoRefreshInterval, false))
		assert.Empty(t, core.registeredNode.HostInfo.Services)
		assert.Equal(t, nqn1, core.registeredNode.NQN)
	})

	t.Run("a failed probe keeps the registered service", func(t *testing.T) {
		core, mocks := newTestCore(t)
		var nvmeErr error
		// Declared before expectHost because gomock uses the first matching expectation.
		mocks.NVMe.EXPECT().NVMeActiveOnHost(gomock.Any()).DoAndReturn(
			func(context.Context) (bool, error) { return nvmeErr == nil, nvmeErr }).AnyTimes()
		host := fakeHost{ips1, nqn1}
		expectHost(mocks, &host)
		core.registeredNode, _ = core.buildNodeInfo(context.Background(), false)
		nvmeErr = errors.New("nvme probe failed")

		// The mocks fail the test if register calls RegisterNode.
		assert.NoError(t, core.register(context.Background(), nodeInfoRefreshInterval, false))
		assert.Equal(t, []string{"nvme"}, core.registeredNode.HostInfo.Services)
		assert.Equal(t, nqn1, core.registeredNode.NQN)
	})

	t.Run("a probe that keeps failing does not block other services", func(t *testing.T) {
		core, mocks := newTestCore(t)
		iscsiActive := false
		// The NVMe probe fails on every scan, as on a host without the nvme CLI. Declared before
		// expectNodeInfoDiscovery because gomock uses the first matching expectation.
		mocks.NVMe.EXPECT().NVMeActiveOnHost(gomock.Any()).Return(false, errors.New("nvme not found")).AnyTimes()
		mocks.ISCSI.EXPECT().ISCSIActiveOnHost(gomock.Any(), gomock.Any()).DoAndReturn(
			func(context.Context, models.HostSystem) (bool, error) { return iscsiActive, nil }).AnyTimes()
		expectNodeInfoDiscovery(mocks)
		core.registeredNode, _ = core.buildNodeInfo(context.Background(), false)
		iscsiActive = true
		mocks.Controller.MockClient.EXPECT().RegisterNode(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&tridentcontroller.RegistrationInfo{}, nil)

		assert.NoError(t, core.register(context.Background(), nodeInfoRefreshInterval, false))
		assert.Equal(t, []string{"iSCSI"}, core.registeredNode.HostInfo.Services)
	})

	t.Run("registration failure keeps the previous identity", func(t *testing.T) {
		core, mocks := newTestCore(t)
		host := fakeHost{ips: ips1}
		expectHost(mocks, &host)
		core.registeredNode, _ = core.buildNodeInfo(context.Background(), false)
		host.nqn = nqn1
		mocks.Controller.MockClient.EXPECT().RegisterNode(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("controller unavailable"))
		// The deadline stops register's backoff after the first attempt.
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		assert.Error(t, core.register(ctx, nodeInfoRefreshInterval, false))
		assert.Empty(t, core.registeredNode.NQN)
	})
}

func TestKeepRegisteredIdentity(t *testing.T) {
	registered := &models.Node{
		IQN:      "iqn.1994-05.com.redhat:host",
		NQN:      "nqn.2014-08.org.nvmexpress:uuid:host",
		IPs:      []string{"192.0.2.1"},
		HostInfo: &models.HostSystem{Services: []string{"iSCSI", "nvme"}},
		HostWWPNMap: map[string][]string{
			"0x1000000000000001": {"0x2000000000000001"},
			"0x1000000000000002": {"0x2000000000000001"},
		},
	}

	t.Run("failed probes keep the registered identity", func(t *testing.T) {
		discovered := &models.Node{HostInfo: &models.HostSystem{}}
		keepRegisteredIdentity(discovered, registered, []string{"iSCSI", "nvme"})
		assert.Equal(t, registered, discovered)
	})

	t.Run("services follow the probes that succeeded", func(t *testing.T) {
		// iSCSI was probed and is inactive, the NVMe probe failed, and NFS is new.
		discovered := &models.Node{HostInfo: &models.HostSystem{Services: []string{"NFS"}}}
		keepRegisteredIdentity(discovered, registered, []string{"nvme"})
		assert.Equal(t, []string{"nvme", "NFS"}, discovered.HostInfo.Services)
	})

	t.Run("a partial FC scan keeps the registered initiators", func(t *testing.T) {
		// The second initiator's port name could not be read, and the first reaches a new target.
		discovered := &models.Node{
			HostInfo:    &models.HostSystem{},
			HostWWPNMap: map[string][]string{"0x1000000000000001": {"0x2000000000000001", "0x2000000000000002"}},
		}
		keepRegisteredIdentity(discovered, registered, nil)
		assert.Equal(t, map[string][]string{
			"0x1000000000000001": {"0x2000000000000001", "0x2000000000000002"},
			"0x1000000000000002": {"0x2000000000000001"},
		}, discovered.HostWWPNMap)
	})
}

func TestBuildNodeInfoReusesHostOS(t *testing.T) {
	ctx := context.Background()
	ubuntu := models.SystemOS{Distro: "ubuntu", Version: "24.04"}
	core, mocks := newTestCore(t)
	// The OS is looked up until a lookup succeeds, then reused for iSCSI discovery without another lookup.
	gomock.InOrder(
		mocks.OsUtils.EXPECT().GetHostSystemInfo(gomock.Any()).Return(nil, errors.New("lookup failed")),
		mocks.OsUtils.EXPECT().GetHostSystemInfo(gomock.Any()).Return(&models.HostSystem{OS: ubuntu}, nil),
	)
	mocks.ISCSI.EXPECT().ISCSIActiveOnHost(gomock.Any(), models.HostSystem{}).Return(false, nil)
	mocks.ISCSI.EXPECT().ISCSIActiveOnHost(gomock.Any(), models.HostSystem{OS: ubuntu}).Return(true, nil).Times(2)
	mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return([]string{"192.0.2.10"}, nil).AnyTimes()
	mocks.OsUtils.EXPECT().NFSActiveOnHost(gomock.Any()).Return(false, nil).AnyTimes()
	mocks.NVMe.EXPECT().NVMeActiveOnHost(gomock.Any()).Return(false, nil).AnyTimes()

	node, _ := core.buildNodeInfo(ctx, false)
	assert.Empty(t, node.HostInfo.OS)
	for range 2 {
		node, _ = core.buildNodeInfo(ctx, false)
		assert.Equal(t, ubuntu, node.HostInfo.OS)
		assert.Equal(t, []string{"iSCSI"}, node.HostInfo.Services)
	}
}

func TestBuildNodeInfoReportsUnknownServices(t *testing.T) {
	core, mocks := newTestCore(t)
	// Declared before expectNodeInfoDiscovery because gomock uses the first matching expectation.
	mocks.OsUtils.EXPECT().NFSActiveOnHost(gomock.Any()).Return(false, errors.New("systemctl failed")).AnyTimes()
	mocks.NVMe.EXPECT().NVMeActiveOnHost(gomock.Any()).Return(false, errors.New("nvme not found")).AnyTimes()
	expectNodeInfoDiscovery(mocks)

	node, unknownServices := core.buildNodeInfo(context.Background(), false)

	// SMB discovery is unsupported off Windows, which does not make SMB unknown.
	assert.Equal(t, []string{"NFS", "nvme"}, unknownServices)
	assert.Empty(t, node.HostInfo.Services)
}

func TestNodeInfoRefreshLoop(t *testing.T) {
	interval, enabled := nodeInfoRefreshInterval, nodeInfoRefreshEnabled
	nodeInfoRefreshInterval, nodeInfoRefreshEnabled = 10*time.Millisecond, true
	t.Cleanup(func() { nodeInfoRefreshInterval, nodeInfoRefreshEnabled = interval, enabled })
	core, mocks := newTestCore(t)
	// The host reports a new IP address after the first refresh; the old one stays registered.
	mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return([]string{"192.0.2.1"}, nil)
	mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).Return([]string{"192.0.2.2"}, nil).AnyTimes()
	expectNodeInfoDiscovery(mocks)
	registered := make(chan []string, 2)
	mocks.Controller.MockClient.EXPECT().RegisterNode(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(
			_ context.Context, node *models.Node, _ time.Duration,
		) (*tridentcontroller.RegistrationInfo, error) {
			registered <- node.IPs
			return &tridentcontroller.RegistrationInfo{}, nil
		}).Times(2)

	core.startNodeInfoRefresh()
	for _, want := range [][]string{{"192.0.2.1"}, {"192.0.2.1", "192.0.2.2"}} {
		select {
		case ips := <-registered:
			assert.Equal(t, want, ips)
		case <-time.After(5 * time.Second):
			t.Fatalf("node info refresh did not register %v", want)
		}
	}
	core.stopNodeInfoRefresh()
}

func TestNodeInfoRefreshDisabled(t *testing.T) {
	enabled := nodeInfoRefreshEnabled
	nodeInfoRefreshEnabled = false
	t.Cleanup(func() { nodeInfoRefreshEnabled = enabled })
	// The mocks fail the test on any host discovery or registration call.
	core, _ := newTestCore(t)

	core.startNodeInfoRefresh()

	assert.Nil(t, core.cancelNodeInfoRefresh)
	core.stopNodeInfoRefresh()
}

// fakeHost is what the mocked host discovery reports; tests change it between refreshes.
type fakeHost struct {
	ips []string
	nqn string
}

// expectHost sets up host discovery that reports host's current IP addresses and NVMe host NQN.
func expectHost(mocks *testMocks, host *fakeHost) {
	// Declared before expectNodeInfoDiscovery because gomock uses the first matching expectation.
	mocks.OsUtils.EXPECT().GetIPAddresses(gomock.Any()).DoAndReturn(
		func(context.Context) ([]string, error) { return host.ips, nil }).AnyTimes()
	mocks.NVMe.EXPECT().NVMeActiveOnHost(gomock.Any()).DoAndReturn(
		func(context.Context) (bool, error) { return host.nqn != "", nil }).AnyTimes()
	mocks.NVMe.EXPECT().GetHostNqn(gomock.Any()).DoAndReturn(
		func(context.Context) (string, error) { return host.nqn, nil }).AnyTimes()
	expectNodeInfoDiscovery(mocks)
}
