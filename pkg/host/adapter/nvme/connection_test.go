// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewConnectConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg := newConnectConfig(nil)

		assert.Equal(t, TransportTCP, cfg.transport)
		assert.Equal(t, "", cfg.port)
	})

	t.Run("with_transport", func(t *testing.T) {
		cfg := newConnectConfig([]ConnectOption{WithTransport(testOtherTransport)})

		assert.Equal(t, testOtherTransport, cfg.transport)
		assert.Equal(t, "", cfg.port)
	})

	t.Run("with_port", func(t *testing.T) {
		cfg := newConnectConfig([]ConnectOption{WithPort("4421")})

		assert.Equal(t, TransportTCP, cfg.transport)
		assert.Equal(t, "4421", cfg.port)
	})

	t.Run("later_option_wins", func(t *testing.T) {
		cfg := newConnectConfig([]ConnectOption{
			WithTransport("one"), WithPort("1"), WithTransport(testOtherTransport), WithPort("2"),
		})

		assert.Equal(t, testOtherTransport, cfg.transport)
		assert.Equal(t, "2", cfg.port)
	})

	t.Run("explicit_empty_transport_is_kept_for_validation", func(t *testing.T) {
		cfg := newConnectConfig([]ConnectOption{WithTransport("")})

		assert.Equal(t, Transport(""), cfg.transport)
	})

	t.Run("nil_slice_and_empty_slice_match", func(t *testing.T) {
		assert.Equal(t, newConnectConfig(nil), newConnectConfig([]ConnectOption{}))
	})
}
