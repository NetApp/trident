// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tridentconfig "github.com/netapp/trident/config"
	drivers "github.com/netapp/trident/storage_drivers"
)

func TestStorageAPITimeoutFromOntapConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  *drivers.OntapStorageDriverConfig
		want    time.Duration
		wantErr bool
	}{
		{
			name:   "nil config returns not configured",
			config: nil,
			want:   0,
		},
		{
			name:   "empty returns not configured",
			config: &drivers.OntapStorageDriverConfig{},
			want:   0,
		},
		{
			name: "valid override",
			config: &drivers.OntapStorageDriverConfig{
				StorageAPITimeout: "180",
			},
			want: 180 * time.Second,
		},
		{
			name: "invalid value",
			config: &drivers.OntapStorageDriverConfig{
				StorageAPITimeout: "not-a-number",
			},
			wantErr: true,
		},
		{
			name: "zero rejected",
			config: &drivers.OntapStorageDriverConfig{
				StorageAPITimeout: "0",
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := StorageAPITimeoutFromOntapConfig(tc.config)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEffectiveZapiStorageAPITimeout(t *testing.T) {
	t.Parallel()

	assert.Equal(t, tridentconfig.StorageAPITimeout, effectiveZapiStorageAPITimeout(0))
	assert.Equal(t, 120*time.Second, effectiveZapiStorageAPITimeout(120*time.Second))
}
