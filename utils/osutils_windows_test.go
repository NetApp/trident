// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestGetHostSystemInfo(t *testing.T) {
	ctx := context.Background()
	result, err := GetHostSystemInfo(ctx)
	assert.NotNil(t, result, "host system information is not populated")
	assert.NoError(t, err, "no error")
}

func TestNFSActiveOnHost(t *testing.T) {
	ctx := context.Background()
	result, err := NFSActiveOnHost(ctx)
	assert.False(t, result, "nfs is present on the host")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestGetTargetFilePath(t *testing.T) {
	ctx := context.Background()
	result := GetTargetFilePath(ctx, "/host/path1", "/test/path")
	assert.NotEmpty(t, result, "path is empty")
}

func TestGetIPAddresses(t *testing.T) {
	ctx := context.Background()
	result, err := getIPAddresses(ctx)
	assert.Nil(t, result, "got ip address")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestSMBActiveOnHost(t *testing.T) {
	ctx := context.Background()
	result, err := SMBActiveOnHost(ctx)
	assert.True(t, result, "smb is not active on the host")
	assert.NoError(t, err, "error")
}
