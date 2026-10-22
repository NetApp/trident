// Copyright 2022 NetApp, Inc. All Rights Reserved.

//go:build linux

package osutils

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/vishvananda/netlink"
	"go.uber.org/mock/gomock"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/mocks/mock_utils/mock_osutils"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
)

func TestGetIPAddresses(t *testing.T) {
	osUtils := New()
	addrs, err := osUtils.GetIPAddresses(context.TODO())
	if err != nil {
		t.Error(err)
	}

	assert.Greater(t, len(addrs), 0, "No IP addresses found")

	for _, ip := range addrs {
		assert.NotNil(t, net.ParseIP(ip), "IP address is not valid")
	}
}

func TestGetIPAddresses_Error(t *testing.T) {
	mockNetLink := mock_osutils.NewMockNetLink(gomock.NewController(t))
	mockNetLink.EXPECT().LinkList().Return(nil, errors.New("error"))

	defer func(originalNetlink NetLink) {
		netLink = originalNetlink
	}(netLink)
	netLink = mockNetLink

	osUtils := NewDetailed(nil, afero.NewMemMapFs())
	addrs, err := osUtils.GetIPAddresses(context.TODO())
	assert.Nil(t, addrs)
	assert.Error(t, err)
}

func TestGetIPAddressesExceptingDummyInterfaces(t *testing.T) {
	osUtils := New()
	addrs, err := osUtils.getIPAddressesExceptingDummyInterfaces(context.TODO())
	if err != nil {
		t.Error(err)
	}

	assert.Greater(t, len(addrs), 0, "No IP addresses found")

	for _, addr := range addrs {

		parsedAddr := net.ParseIP(strings.Split(addr.String(), "/")[0])
		assert.False(t, parsedAddr.IsLoopback(), "Address is loopback")
		assert.True(t, parsedAddr.IsGlobalUnicast(), "Address is not global unicast")
	}
}

func TestGetIPAddressesExceptingDummyInterfaces_NegTests(t *testing.T) {
	mockNetLink := mock_osutils.NewMockNetLink(gomock.NewController(t))
	mockNetLink.EXPECT().LinkList().Return(nil, errors.New("error"))

	defer func(originalNetlink NetLink) {
		netLink = originalNetlink
	}(netLink)
	netLink = mockNetLink

	osUtils := NewDetailed(nil, afero.NewMemMapFs())
	addrs, err := osUtils.getIPAddressesExceptingDummyInterfaces(context.TODO())
	assert.Nil(t, addrs)
	assert.Error(t, err)
}

func TestGetIPAddressesExceptingDummyInterfaces_DummyType(t *testing.T) {
	mockLinks := []netlink.Link{
		&netlink.Dummy{},
	}
	mockNetLink := mock_osutils.NewMockNetLink(gomock.NewController(t))
	mockNetLink.EXPECT().LinkList().Return(mockLinks, nil)

	defer func(originalNetlink NetLink) {
		netLink = originalNetlink
	}(netLink)
	netLink = mockNetLink

	osUtils := NewDetailed(nil, afero.NewMemMapFs())
	addrs, err := osUtils.getIPAddressesExceptingDummyInterfaces(context.TODO())
	assert.Equal(t, 0, len(addrs))
	assert.NoError(t, err)
}

func TestGetIPAddressesExceptingDummyInterfaces_DumpInterrupted(t *testing.T) {
	link := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "ens192"}}
	addrs := []netlink.Addr{{IPNet: &net.IPNet{IP: net.IPv4(10, 58, 43, 100), Mask: net.CIDRMask(24, 32)}}}

	defer func(originalNetlink NetLink) {
		netLink = originalNetlink
	}(netLink)

	t.Run("interfaces returned are used", func(t *testing.T) {
		mockNetLink := mock_osutils.NewMockNetLink(gomock.NewController(t))
		mockNetLink.EXPECT().LinkList().Return([]netlink.Link{link}, netlink.ErrDumpInterrupted)
		mockNetLink.EXPECT().AddrList(link, netlink.FAMILY_ALL).Return(addrs, nil)
		netLink = mockNetLink

		result, err := NewDetailed(nil, afero.NewMemMapFs()).getIPAddressesExceptingDummyInterfaces(context.TODO())

		assert.NoError(t, err)
		assert.Len(t, result, 1)
		assert.Equal(t, "10.58.43.100/24", result[0].String())
	})

	t.Run("no interfaces returned is not an error", func(t *testing.T) {
		mockNetLink := mock_osutils.NewMockNetLink(gomock.NewController(t))
		mockNetLink.EXPECT().LinkList().Return(nil, netlink.ErrDumpInterrupted)
		netLink = mockNetLink

		result, err := NewDetailed(nil, afero.NewMemMapFs()).getIPAddressesExceptingDummyInterfaces(context.TODO())

		assert.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestGetIPAddressesExceptingNondefaultRoutes_DumpInterrupted(t *testing.T) {
	link := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "ens192", Index: 2}}
	addrs := []netlink.Addr{{IPNet: &net.IPNet{IP: net.IPv4(10, 58, 43, 100), Mask: net.CIDRMask(24, 32)}}}

	mockNetLink := mock_osutils.NewMockNetLink(gomock.NewController(t))
	gomock.InOrder(
		mockNetLink.EXPECT().RouteListFiltered(netlink.FAMILY_ALL, gomock.Any(), uint64(netlink.RT_FILTER_DST)).
			Return([]netlink.Route{{LinkIndex: 2}}, netlink.ErrDumpInterrupted),
		mockNetLink.EXPECT().RouteListFiltered(netlink.FAMILY_ALL, gomock.Any(), uint64(netlink.RT_FILTER_DST)).
			Return(nil, nil),
	)
	mockNetLink.EXPECT().LinkByIndex(2).Return(link, nil)
	mockNetLink.EXPECT().AddrList(link, netlink.FAMILY_ALL).Return(addrs, nil)
	defer func(originalNetlink NetLink) {
		netLink = originalNetlink
	}(netLink)
	netLink = mockNetLink

	result, err := NewDetailed(nil, afero.NewMemMapFs()).getIPAddressesExceptingNondefaultRoutes(context.TODO())

	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, "10.58.43.100/24", result[0].String())
}

func TestGetIPAddressesExceptingNondefaultRoutes_Error(t *testing.T) {
	mockNetLink := mock_osutils.NewMockNetLink(gomock.NewController(t))
	mockNetLink.EXPECT().RouteListFiltered(netlink.FAMILY_ALL, gomock.Any(), uint64(netlink.RT_FILTER_DST)).
		Return(nil, errors.New("error"))
	defer func(originalNetlink NetLink) {
		netLink = originalNetlink
	}(netLink)
	netLink = mockNetLink

	result, err := NewDetailed(nil, afero.NewMemMapFs()).getIPAddressesExceptingNondefaultRoutes(context.TODO())

	assert.Nil(t, result)
	assert.Error(t, err)
}

func TestGetIPAddressesExceptingNondefaultRoutes(t *testing.T) {
	osUtils := New()
	addrs, err := osUtils.getIPAddressesExceptingNondefaultRoutes(context.TODO())
	if err != nil {
		t.Error(err)
	}

	assert.Greater(t, len(addrs), 0, "No IP addresses found")

	for _, addr := range addrs {

		parsedAddr := net.ParseIP(strings.Split(addr.String(), "/")[0])
		assert.False(t, parsedAddr.IsLoopback(), "Address is loopback")
		assert.True(t, parsedAddr.IsGlobalUnicast(), "Address is not global unicast")
	}
}

func TestNFSActiveOnHost(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockExec := mockexec.NewMockCommand(mockCtrl)

	tests := []struct {
		name        string
		expectedErr error
	}{
		{
			name:        "Active",
			expectedErr: nil,
		},
		{
			name:        "Inactive",
			expectedErr: nil,
		},
		{
			name:        "Fails",
			expectedErr: errors.New("failed to check if service is active on host"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os := NewDetailed(mockExec, afero.NewMemMapFs())

			ctx := context.Background()
			mockExec.EXPECT().ExecuteWithTimeout(
				ctx, "systemctl", 30*time.Second, true, "is-active", "rpc-statd",
			).Return([]byte(""), tt.expectedErr)

			active, err := os.NFSActiveOnHost(ctx)
			if tt.expectedErr != nil {
				assert.Error(t, err)
				assert.False(t, active)
			} else {
				assert.NoError(t, err)
				assert.True(t, active)
			}
		})
	}
}

func TestGetTargetFilePath(t *testing.T) {
	ctx := context.Background()
	result := GetTargetFilePath(ctx, "/host/path1", "/test/path")
	assert.NotEmpty(t, result, "path is empty")
}

func TestSMBActiveOnHost(t *testing.T) {
	ctx := context.Background()
	result, err := SMBActiveOnHost(ctx)
	assert.False(t, result, "smb is active on the host")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestGetHostSystemInfo(t *testing.T) {
	value, isSet := os.LookupEnv("CSI_ENDPOINT")
	if isSet {
		defer os.Setenv("CSI_ENDPOINT", value)
	}

	ctx := context.Background()

	// Not in container
	os.Unsetenv("CSI_ENDPOINT")
	osUtils := NewDetailed(exec.NewCommand(), afero.NewMemMapFs())
	hostInfo, err := osUtils.GetHostSystemInfo(ctx)
	assert.NotNil(t, hostInfo)
	assert.NoError(t, err, "error is not nil")
	assert.NotEqual(t, hostInfo.OS.Distro, "")

	// In container
	t.Setenv("CSI_ENDPOINT", "unix:///csi/csi.sock")
	mockCmd := mockexec.NewMockCommand(gomock.NewController(t))
	mockData := `{
        "name": "Linux",
		"vendor": "Ubuntu",
        "version": "5.4.0-42-generic",
        "arch": "x86_64"
    }`
	mockCmd.EXPECT().ExecuteWithTimeout(ctx, "tridentctl", 5*time.Second, true, "system", "--chroot-path",
		"/host").Return([]byte(mockData), nil)
	osUtils = NewDetailed(mockCmd, afero.NewMemMapFs())
	hostInfo, err = osUtils.GetHostSystemInfo(ctx)
	assert.NotNil(t, hostInfo)
	assert.NoError(t, err, "error is not nil")
	assert.NotEqual(t, hostInfo.OS.Distro, "")

	// ExecuteWithTimeout error
	mockCmd.EXPECT().ExecuteWithTimeout(ctx, "tridentctl", 5*time.Second, true, "system", "--chroot-path",
		"/host").Return(nil, errors.New("error"))
	osUtils = NewDetailed(mockCmd, afero.NewMemMapFs())
	hostInfo, err = osUtils.GetHostSystemInfo(ctx)
	assert.Error(t, err, "error is not nil")
}

func TestGetUsableAddressesFromLinks(t *testing.T) {
	mockLinks := []netlink.Link{
		&netlink.Veth{},
		&netlink.Veth{},
	}

	mockAddresses := []netlink.Addr{
		// Nil case
		{IPNet: nil},
		// Global Unicast case
		{IPNet: &net.IPNet{IP: net.IPv4(255, 255, 255, 255), Mask: net.CIDRMask(24, 32)}},
		// Valid IP
		{IPNet: &net.IPNet{IP: net.IPv4(10, 10, 10, 10), Mask: net.CIDRMask(24, 32)}},
	}

	mockNetLink := mock_osutils.NewMockNetLink(gomock.NewController(t))
	mockNetLink.EXPECT().AddrList(gomock.Any(), netlink.FAMILY_ALL).Return(nil, errors.New("error"))
	mockNetLink.EXPECT().AddrList(gomock.Any(), netlink.FAMILY_ALL).Return(mockAddresses, nil)
	defer func(originalNetlink NetLink) {
		netLink = originalNetlink
	}(netLink)
	netLink = mockNetLink

	osUtils := NewDetailed(exec.NewCommand(), afero.NewMemMapFs())
	addresses := osUtils.getUsableAddressesFromLinks(context.TODO(), mockLinks)
	assert.Equal(t, 1, len(addresses))
	assert.Equal(t, "10.10.10.10/24", addresses[0].String())
}

func TestGetUsableAddressesFromLinks_DumpInterrupted(t *testing.T) {
	ens192 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "ens192", Index: 2}}
	ens224 := &netlink.Device{LinkAttrs: netlink.LinkAttrs{Name: "ens224", Index: 3}}

	mockNetLink := mock_osutils.NewMockNetLink(gomock.NewController(t))
	mockNetLink.EXPECT().AddrList(ens192, netlink.FAMILY_ALL).Return([]netlink.Addr{
		{IPNet: &net.IPNet{IP: net.IPv4(10, 58, 43, 100), Mask: net.CIDRMask(24, 32)}},
	}, netlink.ErrDumpInterrupted)
	mockNetLink.EXPECT().AddrList(ens224, netlink.FAMILY_ALL).Return([]netlink.Addr{
		{IPNet: &net.IPNet{IP: net.IPv4(10, 58, 79, 100), Mask: net.CIDRMask(24, 32)}},
	}, errors.New("error"))
	defer func(originalNetlink NetLink) {
		netLink = originalNetlink
	}(netLink)
	netLink = mockNetLink

	osUtils := NewDetailed(exec.NewCommand(), afero.NewMemMapFs())
	addresses := osUtils.getUsableAddressesFromLinks(context.TODO(), []netlink.Link{ens192, ens224})

	assert.Len(t, addresses, 1)
	assert.Equal(t, "10.58.43.100/24", addresses[0].String())
}
