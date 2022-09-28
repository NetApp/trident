// Copyright 2022 NetApp, Inc. All Rights Reserved.

//go:build !linux

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAttachNFSVolume(t *testing.T) {
	ctx := context.Background()

	volumePublishInfo := &VolumePublishInfo{
		VolumeAccessInfo: VolumeAccessInfo{
			NfsAccessInfo: NfsAccessInfo{
				NfsServerIP: "1.1.1.1",
				NfsPath:     "/test/nfs/path",
			},
		},
	}

	result := AttachNFSVolume(ctx, "test-vol", "/pods", volumePublishInfo)
	assert.Error(t, result, "call mount nfs path succeeded")
}
