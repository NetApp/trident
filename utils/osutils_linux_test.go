// Copyright 2022 NetApp, Inc. All Rights Reserved.

//go:build linux

package utils

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
)

func TestGetIPAddresses(t *testing.T) {
	addrs, err := getIPAddresses(context.TODO())
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

func TestGetIPAddressesExceptingDummyInterfaces(t *testing.T) {
	addrs, err := getIPAddressesExceptingDummyInterfaces(context.TODO())
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

func TestGetIPAddressesExceptingNondefaultRoutes(t *testing.T) {
	addrs, err := getIPAddressesExceptingNondefaultRoutes(context.TODO())
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
			expectedErr: fmt.Errorf("failed to check if service is active on host"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset exec command after each test.
			defer func(previousCommand exec.Command) {
				command = previousCommand
			}(command)

			ctx := context.Background()
			mockExec.EXPECT().ExecuteWithTimeout(
				ctx, "systemctl", 30*time.Second, true, "is-active", "rpc-statd",
			).Return([]byte(""), tt.expectedErr)
			command = mockExec

			active, err := NFSActiveOnHost(ctx)
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

func TestISCSIActiveOnHost(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockExec := mockexec.NewMockCommand(mockCtrl)

	type args struct {
		ctx  context.Context
		host models.HostSystem
	}
	type expected struct {
		active bool
		err    error
	}
	tests := []struct {
		name     string
		args     args
		expected expected
		service  string
	}{
		{
			name: "ActiveCentos",
			args: args{
				ctx:  context.Background(),
				host: models.HostSystem{OS: models.SystemOS{Distro: Centos}},
			},
			expected: expected{
				active: true,
				err:    nil,
			},
			service: "iscsid",
		},
		{
			name: "ActiveRHEL",
			args: args{
				ctx:  context.Background(),
				host: models.HostSystem{OS: models.SystemOS{Distro: RHEL}},
			},
			expected: expected{
				active: true,
				err:    nil,
			},
			service: "iscsid",
		},
		{
			name: "ActiveUbuntu",
			args: args{
				ctx:  context.Background(),
				host: models.HostSystem{OS: models.SystemOS{Distro: Ubuntu}},
			},
			expected: expected{
				active: true,
				err:    nil,
			},
			service: "open-iscsi",
		},
		{
			name: "InactiveRHEL",
			args: args{
				ctx:  context.Background(),
				host: models.HostSystem{OS: models.SystemOS{Distro: RHEL}},
			},
			expected: expected{
				active: false,
				err:    nil,
			},
			service: "iscsid",
		},
		{
			name: "UnknownDistro",
			args: args{
				ctx:  context.Background(),
				host: models.HostSystem{OS: models.SystemOS{Distro: "SUSE"}},
			},
			expected: expected{
				active: true,
				err:    nil,
			},
			service: "iscsid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset exec command after each test.
			defer func(previousCommand exec.Command) {
				command = previousCommand
			}(command)

			mockExec.EXPECT().ExecuteWithTimeout(
				tt.args.ctx, "systemctl", 30*time.Second, true, "is-active", tt.service,
			).Return([]byte(""), tt.expected.err)
			command = mockExec

			active, err := ISCSIActiveOnHost(tt.args.ctx, tt.args.host)
			if tt.expected.err != nil {
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
