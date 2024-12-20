// Copyright 2022 NetApp, Inc. All Rights Reserved.

package iscsi

import (
	"context"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func TestISCSIActiveOnHost(t *testing.T) {
	ctx := context.Background()
	host := models.HostSystem{
		OS: models.SystemOS{
			Distro: "windows",
		},
		Services: []string{"srv-1", "srv-2"},
	}

	iscsiClient := NewDetailed("", nil, nil, nil, nil, nil, nil, nil, afero.Afero{Fs: afero.NewMemMapFs()}, nil)
	result, err := iscsiClient.ISCSIActiveOnHost(ctx, host)
	assert.False(t, result, "iscsi is active on host")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}
