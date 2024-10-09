// Copyright 2024 NetApp, Inc. All Rights Reserved

package durations

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/errors"
)

func TestInitStartTime(t *testing.T) {
	tests := []struct {
		name      string
		td        TimeDuration
		key       string
		expectErr bool
	}{
		{
			name:      "Initialized map",
			td:        TimeDuration{},
			key:       "testKey",
			expectErr: false,
		},
		{
			name:      "Uninitialized map",
			td:        nil,
			key:       "testKey",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.td.InitStartTime(tt.key)
			_, exists := tt.td[tt.key]
			assert.True(t, exists, "Start time should be initialized")
		})
	}
}

func TestGetCurrentDuration(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		initKey   bool
		expectErr bool
	}{
		{
			name:      "Key does not exist",
			key:       "nonExistentKey",
			initKey:   false,
			expectErr: true,
		},
		{
			name:      "Key exists",
			key:       "testKey",
			initKey:   true,
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			td := TimeDuration{}
			if tt.initKey {
				td.InitStartTime(tt.key)
			}

			duration, err := td.GetCurrentDuration(tt.key)
			if tt.expectErr {
				assert.Error(t, err, "Should return an error when key does not exist")
				assert.Equal(t, errors.NotFoundError("not tracking duration for key '%s'", tt.key), err)
			} else {
				assert.NoError(t, err, "Should not return an error when key exists")
				assert.GreaterOrEqual(t, duration, time.Duration(0), "Duration should be non-negative")
			}
		})
	}
}

func TestRemoveDurationTracking(t *testing.T) {
	td := TimeDuration{}
	key := "testKey"

	td.InitStartTime(key)
	td.RemoveDurationTracking(key)
	_, exists := td[key]
	assert.False(t, exists, "Key should be removed from tracking")
}
