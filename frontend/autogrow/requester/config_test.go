// Copyright 2026 NetApp, Inc. All Rights Reserved.

package requester

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mock_workerpool "github.com/netapp/trident/mocks/mock_pkg/mock_workerpool"
)

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
				assert.Equal(t, DefaultShutdownTimeout, c.ShutdownTimeout)
				assert.Equal(t, DefaultWorkQueueName, c.WorkQueueName)
				assert.Equal(t, DefaultMaxRetries, c.MaxRetries)
				assert.Equal(t, DefaultTridentNamespace, c.TridentNamespace)
				assert.Nil(t, c.WorkerPool)
			},
		},
		{
			name: "Config with all options",
			opts: []ConfigOption{
				WithShutdownTimeout(5 * time.Minute),
				WithWorkQueueName("custom-queue"),
				WithMaxRetries(10),
				WithTridentNamespace("custom-namespace"),
			},
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, 5*time.Minute, c.ShutdownTimeout)
				assert.Equal(t, "custom-queue", c.WorkQueueName)
				assert.Equal(t, 10, c.MaxRetries)
				assert.Equal(t, "custom-namespace", c.TridentNamespace)
				assert.Nil(t, c.WorkerPool)
			},
		},
		{
			name: "Config with WorkerPool option",
			opts: func() []ConfigOption {
				ctrl := gomock.NewController(t)
				mockPool := mock_workerpool.NewMockPool(ctrl)
				return []ConfigOption{
					WithWorkerPool(mockPool),
				}
			}(),
			validate: func(t *testing.T, c *Config) {
				assert.NotNil(t, c.WorkerPool)
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

func TestConfigOptions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPool := mock_workerpool.NewMockPool(ctrl)

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
			name:   "WithShutdownTimeout",
			option: WithShutdownTimeout(10 * time.Second),
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, 10*time.Second, c.ShutdownTimeout)
			},
		},
		{
			name:   "WithWorkQueueName",
			option: WithWorkQueueName("test-queue"),
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, "test-queue", c.WorkQueueName)
			},
		},
		{
			name:   "WithMaxRetries",
			option: WithMaxRetries(5),
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, 5, c.MaxRetries)
			},
		},
		{
			name:   "WithTridentNamespace",
			option: WithTridentNamespace("test-namespace"),
			validate: func(t *testing.T, c *Config) {
				assert.Equal(t, "test-namespace", c.TridentNamespace)
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
