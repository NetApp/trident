// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetFilesystemSize(t *testing.T) {
	ctx := context.Background()

	result, err := getFilesystemSize(ctx, "")
	assert.Equal(t, result, int64(0), "got non-zero filesystem size")
	assert.Error(t, err, "no error")
	assert.True(t, IsUnsupportedError(err), "not UnsupportedError")
}
