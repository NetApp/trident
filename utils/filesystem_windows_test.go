// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestGetFilesystemSize(t *testing.T) {
	ctx := context.Background()

	result, err := getFilesystemSize(ctx, "")
	assert.Equal(t, result, int64(0), "got non-zero filesystem size")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}
