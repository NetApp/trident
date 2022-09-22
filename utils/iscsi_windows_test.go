package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestISCSIActiveOnHost(t *testing.T) {
	ctx := context.Background()
	host := HostSystem{
		OS: SystemOS{
			Distro: "windows",
		},
		Services: []string{"srv-1", "srv-2"},
	}

	result, err := ISCSIActiveOnHost(ctx, host)
	assert.False(t, result, "iscsi is active on host")
	assert.Error(t, err, "no error")
	assert.True(t, IsUnsupportedError(err), "not UnsupportedError")
}
