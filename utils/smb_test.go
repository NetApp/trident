//go:build !windows

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAttachSMBVolume(t *testing.T) {
	ctx := context.Background()

	volumePublishInfo := &VolumePublishInfo{
		VolumeAccessInfo: VolumeAccessInfo{
			SMBAccessInfo: SMBAccessInfo{
				SMBServer: "tri-fb18.TRIDENT.COM",
				SMBPath:   "\\test-smb-path",
			},
		},
	}

	result := AttachSMBVolume(ctx, "test-vol", "/pods", "test-user", "password",
		volumePublishInfo)
	assert.Error(t, result, "call mount smb path succeeded")
}
