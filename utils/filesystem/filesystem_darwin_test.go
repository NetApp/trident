// Copyright 2022 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func TestGetFilesystemSize(t *testing.T) {
	ctx := context.Background()
	fsClient := New(nil)

	result, err := fsClient.getFilesystemSize(ctx, "")
	assert.Equal(t, result, int64(0), "got non-zero filesystem size")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestGetFilesystemStats(t *testing.T) {
	ctx := context.Background()
	fsClient := New(nil)

	result1, result2, result3, result4, result5, result6, err := fsClient.GetFilesystemStats(ctx, "")
	assert.Equal(t, result1, int64(0), "got non-zero available size")
	assert.Equal(t, result2, int64(0), "got non-zero capacity")
	assert.Equal(t, result3, int64(0), "got non-zero usage")
	assert.Equal(t, result4, int64(0), "got non-zero inodes")
	assert.Equal(t, result5, int64(0), "got non-zero inodesFree")
	assert.Equal(t, result6, int64(0), "got non-zero inodesUsed")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestGetUnmountPath(t *testing.T) {
	ctx := context.Background()
	fsClient := New(nil)

	result, err := fsClient.GetUnmountPath(ctx, &models.VolumeTrackingInfo{})
	assert.Equal(t, result, "", "got unmount path")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestGenerateAnonymousMemFile(t *testing.T) {
	fsClient := New(nil)
	result, err := fsClient.GenerateAnonymousMemFile("", "")
	assert.Equal(t, 0, result)
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}
