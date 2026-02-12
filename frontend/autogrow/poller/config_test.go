// Copyright 2026 NetApp, Inc. All Rights Reserved.

package poller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mockWorkerpool "github.com/netapp/trident/mocks/mock_pkg/mock_workerpool"
)

func TestConfigCopy(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name   string
		config *Config
	}{
		{
			name:   "Nil config",
			config: nil,
		},
		{
			name: "Config with all fields set",
			config: func() *Config {
				period := 3 * time.Minute
				return &Config{
					WorkerPool:       mockWorkerpool.NewMockPool(ctrl),
					AutogrowPeriod:   &period,
					MaxRetries:       5,
					ShutdownTimeout:  45 * time.Second,
					WorkQueueName:    "test-queue",
					TridentNamespace: "test-namespace",
				}
			}(),
		},
		{
			name: "Config with default values",
			config: &Config{
				MaxRetries:       3,
				ShutdownTimeout:  30 * time.Second,
				WorkQueueName:    "queue",
				TridentNamespace: "namespace",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config == nil {
				// Test nil config copy
				copy := tt.config.Copy()
				assert.Nil(t, copy)
				return
			}

			// Create a copy
			copy := tt.config.Copy()
			require.NotNil(t, copy)

			// Verify the copy has the same values
			assert.Equal(t, tt.config.MaxRetries, copy.MaxRetries)
			assert.Equal(t, tt.config.ShutdownTimeout, copy.ShutdownTimeout)
			assert.Equal(t, tt.config.WorkQueueName, copy.WorkQueueName)
			assert.Equal(t, tt.config.TridentNamespace, copy.TridentNamespace)

			// WorkerPool should be the same reference (intentionally shared)
			assert.Equal(t, tt.config.WorkerPool, copy.WorkerPool)

			// AutogrowPeriod should be deep copied (different pointer, same value)
			if tt.config.AutogrowPeriod != nil {
				require.NotNil(t, copy.AutogrowPeriod)
				assert.Equal(t, *tt.config.AutogrowPeriod, *copy.AutogrowPeriod)
				// Verify it's a different pointer (deep copy)
				assert.NotSame(t, tt.config.AutogrowPeriod, copy.AutogrowPeriod)
			} else {
				assert.Nil(t, copy.AutogrowPeriod)
			}

			// Modify the copy and verify original is unchanged
			copy.MaxRetries = 99
			copy.ShutdownTimeout = 99 * time.Second
			copy.WorkQueueName = "modified-queue"
			copy.TridentNamespace = "modified-namespace"
			modifiedPeriod := 10 * time.Minute
			copy.AutogrowPeriod = &modifiedPeriod

			// Original should be unchanged
			assert.NotEqual(t, tt.config.MaxRetries, copy.MaxRetries)
			assert.NotEqual(t, tt.config.ShutdownTimeout, copy.ShutdownTimeout)
			assert.NotEqual(t, tt.config.WorkQueueName, copy.WorkQueueName)
			assert.NotEqual(t, tt.config.TridentNamespace, copy.TridentNamespace)
			if tt.config.AutogrowPeriod != nil {
				assert.NotEqual(t, *tt.config.AutogrowPeriod, *copy.AutogrowPeriod)
			}
		})
	}
}

func TestNewPollerDoesNotMutateConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	autogrowCache := getTestAutogrowCache()
	volumeStatsProvider := getTestVolumeStatsProvider(ctrl)
	_, agpLister := getTestAGPInformerAndLister()
	_, tvpLister := getTestTVPInformerAndLister()

	// Create original config
	originalConfig := &Config{
		WorkerPool:       mockWorkerpool.NewMockPool(ctrl),
		MaxRetries:       5,
		ShutdownTimeout:  45 * time.Second,
		WorkQueueName:    "original-queue",
		TridentNamespace: "original-namespace",
	}

	// Save original values for comparison
	originalMaxRetries := originalConfig.MaxRetries
	originalShutdownTimeout := originalConfig.ShutdownTimeout
	originalWorkQueueName := originalConfig.WorkQueueName
	originalTridentNamespace := originalConfig.TridentNamespace
	originalWorkerPool := originalConfig.WorkerPool

	// Create poller (which should copy the config)
	_, err := NewPoller(ctx, agpLister, tvpLister, autogrowCache, volumeStatsProvider, originalConfig)
	require.NoError(t, err)

	// Verify original config is unchanged
	assert.Equal(t, originalMaxRetries, originalConfig.MaxRetries)
	assert.Equal(t, originalShutdownTimeout, originalConfig.ShutdownTimeout)
	assert.Equal(t, originalWorkQueueName, originalConfig.WorkQueueName)
	assert.Equal(t, originalTridentNamespace, originalConfig.TridentNamespace)
	assert.Equal(t, originalWorkerPool, originalConfig.WorkerPool)
}

func TestNewPollerWithNilConfigUsesDefaults(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	autogrowCache := getTestAutogrowCache()
	volumeStatsProvider := getTestVolumeStatsProvider(ctrl)
	_, agpLister := getTestAGPInformerAndLister()
	_, tvpLister := getTestTVPInformerAndLister()

	// Create poller with nil config
	poller, err := NewPoller(ctx, agpLister, tvpLister, autogrowCache, volumeStatsProvider, nil)
	require.NoError(t, err)
	require.NotNil(t, poller)

	// Verify defaults were applied
	assert.NotNil(t, poller.config)
	assert.Equal(t, DefaultMaxRetries, poller.config.MaxRetries)
	assert.Equal(t, DefaultShutdownTimeout, poller.config.ShutdownTimeout)
	assert.Equal(t, DefaultWorkQueueName, poller.config.WorkQueueName)
	assert.Equal(t, DefaultTridentNamespace, poller.config.TridentNamespace)
}

func TestConfigOptions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPool := mockWorkerpool.NewMockPool(ctrl)

	tests := []struct {
		name     string
		option   ConfigOption
		validate func(*testing.T, *Config)
	}{
		{
			name:   "WithWorkerPool",
			option: WithWorkerPool(mockPool),
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, mockPool, c.WorkerPool)
			},
		},
		{
			name:   "WithMaxRetries",
			option: WithMaxRetries(10),
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, 10, c.MaxRetries)
			},
		},
		{
			name:   "WithShutdownTimeout",
			option: WithShutdownTimeout(5 * time.Minute),
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, 5*time.Minute, c.ShutdownTimeout)
			},
		},
		{
			name:   "WithWorkQueueName",
			option: WithWorkQueueName("custom-queue"),
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, "custom-queue", c.WorkQueueName)
			},
		},
		{
			name:   "WithTridentNamespace",
			option: WithTridentNamespace("custom-namespace"),
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, "custom-namespace", c.TridentNamespace)
			},
		},
		{
			name:   "WithAutogrowPeriod",
			option: WithAutogrowPeriod(2 * time.Minute),
			validate: func(t *testing.T, c *Config) {
				require.NotNil(t, c.AutogrowPeriod)
				assert.Equal(t, 2*time.Minute, *c.AutogrowPeriod)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{}
			tt.option(config)
			tt.validate(t, config)
		})
	}
}

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name     string
		opts     []ConfigOption
		validate func(*testing.T, *Config)
	}{
		{
			name: "Default config with no options",
			opts: nil,
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, DefaultMaxRetries, c.MaxRetries)
				assert.Equal(t, DefaultShutdownTimeout, c.ShutdownTimeout)
				assert.Equal(t, DefaultWorkQueueName, c.WorkQueueName)
				assert.Equal(t, DefaultTridentNamespace, c.TridentNamespace)
				assert.Nil(t, c.WorkerPool)
			},
		},
		{
			name: "Config with all options",
			opts: []ConfigOption{
				WithMaxRetries(5),
				WithShutdownTimeout(5 * time.Minute),
				WithWorkQueueName("custom-queue"),
				WithTridentNamespace("custom-namespace"),
			},
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, 5, c.MaxRetries)
				assert.Equal(t, 5*time.Minute, c.ShutdownTimeout)
				assert.Equal(t, "custom-queue", c.WorkQueueName)
				assert.Equal(t, "custom-namespace", c.TridentNamespace)
				assert.Nil(t, c.WorkerPool)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewConfig(tt.opts...)
			assert.NotNil(t, config)
			tt.validate(t, config)
		})
	}
}
