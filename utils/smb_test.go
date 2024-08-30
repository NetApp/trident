// Copyright 2022 NetApp, Inc. All Rights Reserved.

//go:build !windows

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/models"
)

func TestAttachSMBVolume(t *testing.T) {
	ctx := context.Background()

	volumePublishInfo := &models.VolumePublishInfo{
		VolumeAccessInfo: models.VolumeAccessInfo{
			SMBAccessInfo: models.SMBAccessInfo{
				SMBServer: "tri-fb18.TRIDENT.COM",
				SMBPath:   "\\test-smb-path",
			},
		},
	}

	result := AttachSMBVolume(ctx, "test-vol", "/pods", "test-user", "password",
		volumePublishInfo)
	assert.Error(t, result, "call mount smb path succeeded")
}
