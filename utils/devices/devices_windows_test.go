// Copyright 2022 NetApp, Inc. All Rights Reserved.

package devices

import (
	"context"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
)

func TestFlushOneDevice(t *testing.T) {
	ctx := context.Background()

	devices := NewDetailed(exec.NewCommand(), afero.NewMemMapFs(), nil)
	result := devices.FlushOneDevice(ctx, "/test/path")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestGetDiskSize(t *testing.T) {
	ctx := context.Background()

	devices := NewDetailed(exec.NewCommand(), afero.NewMemMapFs(), NewDiskSizeGetter())
	result, err := devices.GetDiskSize(ctx, "/test/path")
	assert.Equal(t, result, int64(0), "received disk size")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}
