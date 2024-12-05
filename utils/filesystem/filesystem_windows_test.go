// Copyright 2022 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestGetFilesystemSize(t *testing.T) {
	ctx := context.Background()
	fsClient := New(nil)

	result, err := fsClient.getFilesystemSize(ctx, "")
	assert.Equal(t, result, int64(0), "got non-zero filesystem size")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}
