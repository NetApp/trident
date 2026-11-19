// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

// TestUnsupported verifies every Adapter method reports UnsupportedError on darwin.
func TestUnsupported(t *testing.T) {
	ctx := context.Background()
	ad := New()

	t.Run("Version", func(t *testing.T) {
		out, err := ad.Version(ctx)
		assert.Empty(t, out)
		assert.True(t, errors.IsUnsupportedError(err))
	})

	t.Run("HostNQN", func(t *testing.T) {
		out, err := ad.HostNQN(ctx)
		assert.Empty(t, out)
		assert.True(t, errors.IsUnsupportedError(err))
	})

	t.Run("TransportLoaded", func(t *testing.T) {
		loaded, err := ad.TransportLoaded(ctx, TransportTCP)
		assert.False(t, loaded)
		assert.True(t, errors.IsUnsupportedError(err))
	})

	t.Run("Subsystem", func(t *testing.T) {
		sub, err := ad.Subsystem(ctx, "nqn.test")
		assert.Nil(t, sub)
		assert.True(t, errors.IsUnsupportedError(err))
	})

	t.Run("Connect", func(t *testing.T) {
		assert.True(t, errors.IsUnsupportedError(ad.Connect(ctx, "nqn.test", "10.0.0.1")))
		assert.True(t, errors.IsUnsupportedError(ad.Connect(ctx, "nqn.test", "10.0.0.1", WithPort("4421"))))
	})

	t.Run("Disconnect", func(t *testing.T) {
		assert.True(t, errors.IsUnsupportedError(ad.Disconnect(ctx, "nqn.test")))
	})

	t.Run("DisconnectPath", func(t *testing.T) {
		assert.True(t, errors.IsUnsupportedError(ad.DisconnectPath(ctx, "nvme0")))
	})

	t.Run("Flush", func(t *testing.T) {
		assert.True(t, errors.IsUnsupportedError(ad.Flush(ctx, "/dev/nvme0n1")))
	})
}
