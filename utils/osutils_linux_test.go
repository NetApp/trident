// Copyright 2022 NetApp, Inc. All Rights Reserved.

//go:build linux

package utils

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
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
	execCmd = fakeExecCommand
	// Reset exec command after tests
	defer func() {
		execCmd = exec.CommandContext
	}()
	type args struct {
		ctx context.Context
	}
	tests := []struct {
		name        string
		args        args
		want        bool
		wantErr     assert.ErrorAssertionFunc
		returnValue string
		returnCode  int
		delay       string
	}{
		{
			name: "Active", args: args{
				ctx: context.Background(),
			}, want: true, wantErr: assert.NoError, returnValue: "is-active", returnCode: 0, delay: "0s",
		},
		{
			name: "Inactive", args: args{
				ctx: context.Background(),
			}, want: false, wantErr: assert.NoError, returnValue: "not-active", returnCode: 1, delay: "0s",
		},
		// TODO: Reduce time required to induce timeout error condition
		// {
		// 	name: "Error", args: args{
		// 		ctx:     context.Background(),
		// 	}, want: true, wantErr: assert.Error, returnValue: "is-active", returnCode: 0, delay: "31s",
		// },
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execReturnValue = tt.returnValue
			execReturnCode = tt.returnCode
			delay, err := time.ParseDuration(tt.delay)
			if err != nil {
				t.Error("Invalid duration value provided.")
			}
			execDelay = delay
			got, err := NFSActiveOnHost(tt.args.ctx)
			if !tt.wantErr(t, err, fmt.Sprintf("NFSActiveOnHost(%v)", tt.args.ctx)) {
				return
			}
			assert.Equalf(t, tt.want, got, "NFSActiveOnHost(%v)", tt.args.ctx)
		})
	}
}

func TestISCSIActiveOnHost(t *testing.T) {
	execCmd = fakeExecCommand
	// Reset exec command after tests
	defer func() {
		execCmd = exec.CommandContext
	}()
	type args struct {
		ctx  context.Context
		host HostSystem
	}
	tests := []struct {
		name        string
		args        args
		want        bool
		wantErr     assert.ErrorAssertionFunc
		returnValue string
		returnCode  int
		delay       string
	}{
		{
			name: "ActiveCentos", args: args{
				ctx:  context.Background(),
				host: HostSystem{OS: SystemOS{Distro: Centos}},
			}, want: true, wantErr: assert.NoError, returnValue: "is-active", returnCode: 0, delay: "0s",
		},
		{
			name: "ActiveRHEL", args: args{
				ctx:  context.Background(),
				host: HostSystem{OS: SystemOS{Distro: RHEL}},
			}, want: true, wantErr: assert.NoError, returnValue: "is-active", returnCode: 0, delay: "0s",
		},
		{
			name: "ActiveUbuntu", args: args{
				ctx:  context.Background(),
				host: HostSystem{OS: SystemOS{Distro: Ubuntu}},
			}, want: true, wantErr: assert.NoError, returnValue: "is-active", returnCode: 0, delay: "0s",
		},
		{
			name: "InactiveRHEL", args: args{
				ctx:  context.Background(),
				host: HostSystem{OS: SystemOS{Distro: RHEL}},
			}, want: false, wantErr: assert.NoError, returnValue: "not-active", returnCode: 1, delay: "0s",
		},
		// TODO: Reduce time required to induce timeout error condition
		// {
		// 	name: "TimeoutRHEL", args: args{
		// 	ctx: context.Background(),
		// 	host: HostSystem{OS: SystemOS{Distro: RHEL}},
		// }, want: false, wantErr: assert.Error, returnValue: "is-active", returnCode: 0, delay: "31s",
		// },
		{
			name: "UnknownDistro", args: args{
				ctx:  context.Background(),
				host: HostSystem{OS: SystemOS{Distro: "SUSE"}},
			}, want: false, wantErr: assert.NoError, returnValue: "not-found", returnCode: 1, delay: "0s",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execReturnValue = tt.returnValue
			execReturnCode = tt.returnCode
			delay, err := time.ParseDuration(tt.delay)
			if err != nil {
				t.Error("Invalid duration value provided.")
			}
			execDelay = delay
			got, err := ISCSIActiveOnHost(tt.args.ctx, tt.args.host)
			if !tt.wantErr(t, err, fmt.Sprintf("NFSActiveOnHost(%v)", tt.args.ctx)) {
				return
			}
			assert.Equalf(t, tt.want, got, "NFSActiveOnHost(%v)", tt.args.ctx)
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
