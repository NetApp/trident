// Copyright 2022 NetApp, Inc. All Rights Reserved.

package iscsi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func TestISCSIActiveOnHost(t *testing.T) {
	ctx := context.Background()
	host := models.HostSystem{
		OS: models.SystemOS{
			Distro: "darwin",
		},
		Services: []string{"srv-1", "srv-2"},
	}

	iscsiClient, err := New()
	assert.NoError(t, err)
	result, err := iscsiClient.ISCSIActiveOnHost(ctx, host)
	assert.False(t, result, "iscsi is active on host")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}
