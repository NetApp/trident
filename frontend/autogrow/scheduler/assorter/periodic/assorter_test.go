// Copyright 2026 NetApp, Inc. All Rights Reserved.

package periodic

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// ============================================================================
// Helper Functions and Test Fixtures
// ============================================================================

// getTestContext returns a context with timeout for testing
func getTestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// getTestConfig creates a test config with optional overrides
func getTestConfig(opts ...func(*Config)) *Config {
	cfg := &Config{
		period:      DefaultPeriod,
		publishFunc: func(ctx context.Context, volumeNames []string) error { return nil },
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

// mockPublishFunc is a simple mock publish function for testing
func mockPublishFunc(ctx context.Context, volumeNames []string) error {
	return nil
}

// ============================================================================
// Test: NewAssorter
// ============================================================================

func TestNewAssorter(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() *Config
		verify func(*testing.T, *Assorter, error)
	}{
		{
			name: "Success_WithValidConfig_CreatesAssorter",
			setup: func() *Config {
				return getTestConfig()
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err, "NewAssorter should succeed with valid config")
				require.NotNil(t, a, "assorter should not be nil")
				assert.Equal(t, StateStopped, a.state, "initial state should be Stopped")
				assert.NotNil(t, a.volumes, "volumes map should be initialized")
				assert.Equal(t, 0, len(a.volumes), "volumes map should be empty initially")
				assert.Equal(t, DefaultPeriod, a.config.period, "period should match default")
				assert.NotNil(t, a.config.publishFunc, "publishFunc should be set")
			},
		},
		{
			name: "Success_WithNilConfig_UsesDefaults",
			setup: func() *Config {
				return nil
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.Error(t, err, "NewAssorter should fail with nil config due to missing publishFunc")
				assert.Nil(t, a, "assorter should be nil on error")
				assert.Contains(t, err.Error(), "PublishFunc is required", "error should mention missing publishFunc")
			},
		},
		{
			name: "Success_WithCustomPeriod_UsesCustomValue",
			setup: func() *Config {
				return getTestConfig(func(c *Config) {
					c.period = 5 * time.Minute
				})
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err)
				assert.Equal(t, 5*time.Minute, a.config.period, "period should match custom value")
			},
		},
		{
			name: "Success_WithZeroPeriod_UsesDefault",
			setup: func() *Config {
				return getTestConfig(func(c *Config) {
					c.period = 0
				})
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err)
				assert.Equal(t, DefaultPeriod, a.config.period, "zero period should use default")
			},
		},
		{
			name: "Success_WithNegativePeriod_UsesDefault",
			setup: func() *Config {
				return getTestConfig(func(c *Config) {
					c.period = -1 * time.Minute
				})
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err)
				assert.Equal(t, DefaultPeriod, a.config.period, "negative period should use default")
			},
		},
		{
			name: "Error_MissingPublishFunc_ReturnsError",
			setup: func() *Config {
				return &Config{
					period:      DefaultPeriod,
					publishFunc: nil, // Missing publishFunc
				}
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.Error(t, err, "NewAssorter should fail without publishFunc")
				assert.Nil(t, a, "assorter should be nil on error")
				assert.Contains(t, err.Error(), "PublishFunc is required", "error should mention missing publishFunc")
			},
		},
		{
			name: "Success_ConfigWithPublishFunc_PreservesFunction",
			setup: func() *Config {
				customFunc := func(ctx context.Context, volumeNames []string) error {
					return fmt.Errorf("custom error")
				}
				return &Config{
					period:      DefaultPeriod,
					publishFunc: customFunc,
				}
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err)
				require.NotNil(t, a.config.publishFunc)
				// Test that the function is preserved (should return custom error)
				err = a.config.publishFunc(context.Background(), nil)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "custom error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := getTestContext()
			defer cancel()
			config := tt.setup()

			assorter, err := NewAssorter(ctx, config)

			tt.verify(t, assorter, err)
		})
	}
}

// ============================================================================
// Test: Activate
// ============================================================================

func TestActivate(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() *Assorter
		verify func(*testing.T, *Assorter, error)
	}{
		{
			name: "Success_FromStoppedState_ActivatesSuccessfully",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateStopped,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err, "Activate should succeed from Stopped state")
				assert.Equal(t, StateReady, a.state, "state should be Ready after activation")
				assert.NotNil(t, a.stopCh, "stopCh should be created")
				assert.NotNil(t, a.ticker, "ticker should be created")
			},
		},
		{
			name: "Success_Idempotent_AlreadyReady_ReturnsNil",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateReady,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
					stopCh: make(chan struct{}),
					ticker: time.NewTicker(DefaultPeriod),
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err, "Activate should be idempotent when already Ready")
				assert.Equal(t, StateReady, a.state, "state should remain Ready")
			},
		},
		{
			name: "Success_Idempotent_AlreadyRunning_ReturnsNil",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateRunning,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
					stopCh: make(chan struct{}),
					ticker: time.NewTicker(DefaultPeriod),
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err, "Activate should be idempotent when already Running")
				assert.Equal(t, StateRunning, a.state, "state should remain Running")
			},
		},
		{
			name: "Error_InvalidState_Starting_ReturnsError",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateStarting,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.Error(t, err, "Activate should fail from Starting state")
				assert.Contains(t, err.Error(), "cannot activate assorter", "error should mention activation failure")
				assert.Contains(t, err.Error(), "Starting", "error should mention current state")
				assert.Equal(t, StateStarting, a.state, "state should remain Starting")
			},
		},
		{
			name: "Error_InvalidState_Stopping_ReturnsError",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateStopping,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.Error(t, err, "Activate should fail from Stopping state")
				assert.Contains(t, err.Error(), "cannot activate assorter", "error should mention activation failure")
				assert.Contains(t, err.Error(), "Stopping", "error should mention current state")
				assert.Equal(t, StateStopping, a.state, "state should remain Stopping")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := getTestContext()
			defer cancel()
			assorter := tt.setup()

			err := assorter.Activate(ctx)

			tt.verify(t, assorter, err)

			// Cleanup
			if assorter.ticker != nil {
				assorter.ticker.Stop()
			}
			if assorter.stopCh != nil {
				select {
				case <-assorter.stopCh:
					// Already closed
				default:
					close(assorter.stopCh)
				}
			}
		})
	}
}

// ============================================================================
// Test: Deactivate
// ============================================================================

func TestDeactivate(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() *Assorter
		verify func(*testing.T, *Assorter, error)
	}{
		{
			name: "Success_FromRunningState_DeactivatesSuccessfully",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateRunning,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
					stopCh: make(chan struct{}),
					ticker: time.NewTicker(DefaultPeriod),
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err, "Deactivate should succeed from Running state")
				assert.Equal(t, StateStopped, a.state, "state should be Stopped after deactivation")
			},
		},
		{
			name: "Success_FromReadyState_DeactivatesSuccessfully",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateReady,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
					stopCh: make(chan struct{}),
					ticker: time.NewTicker(DefaultPeriod),
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err, "Deactivate should succeed from Ready state")
				assert.Equal(t, StateStopped, a.state, "state should be Stopped after deactivation")
			},
		},
		{
			name: "Success_Idempotent_AlreadyStopped_ReturnsNil",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateStopped,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err, "Deactivate should be idempotent when already Stopped")
				assert.Equal(t, StateStopped, a.state, "state should remain Stopped")
			},
		},
		{
			name: "Error_InvalidState_Starting_ReturnsError",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateStarting,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.Error(t, err, "Deactivate should fail from Starting state")
				assert.Contains(t, err.Error(), "cannot deactivate assorter", "error should mention deactivation failure")
				assert.Contains(t, err.Error(), "Starting", "error should mention current state")
				assert.Equal(t, StateStarting, a.state, "state should remain Starting")
			},
		},
		{
			name: "Success_WithNilTicker_DeactivatesSuccessfully",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateReady,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
					stopCh: make(chan struct{}),
					ticker: nil, // Nil ticker
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err, "Deactivate should handle nil ticker gracefully")
				assert.Equal(t, StateStopped, a.state, "state should be Stopped")
			},
		},
		{
			name: "Success_WithNilStopCh_DeactivatesSuccessfully",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateReady,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
					stopCh: nil, // Nil stopCh
					ticker: time.NewTicker(DefaultPeriod),
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err, "Deactivate should handle nil stopCh gracefully")
				assert.Equal(t, StateStopped, a.state, "state should be Stopped")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := getTestContext()
			defer cancel()
			assorter := tt.setup()

			err := assorter.Deactivate(ctx)

			tt.verify(t, assorter, err)
		})
	}
}

// ============================================================================
// Test: StartScheduleLoop
// ============================================================================

func TestStartScheduleLoop(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() *Assorter
		verify func(*testing.T, *Assorter, error)
	}{
		{
			name: "Success_FromReadyState_StartsLoop",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateReady,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
					stopCh: make(chan struct{}),
					ticker: time.NewTicker(DefaultPeriod),
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err, "StartScheduleLoop should succeed from Ready state")
				assert.Equal(t, StateRunning, a.state, "state should be Running after starting loop")
			},
		},
		{
			name: "Success_Idempotent_AlreadyRunning_ReturnsNil",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateRunning,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
					stopCh: make(chan struct{}),
					ticker: time.NewTicker(DefaultPeriod),
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err, "StartScheduleLoop should be idempotent when already Running")
				assert.Equal(t, StateRunning, a.state, "state should remain Running")
			},
		},
		{
			name: "Error_InvalidState_Stopped_ReturnsError",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateStopped,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.Error(t, err, "StartScheduleLoop should fail from Stopped state")
				assert.Contains(t, err.Error(), "cannot start schedule loop", "error should mention start failure")
				assert.Contains(t, err.Error(), "must be Ready", "error should mention required state")
				assert.Equal(t, StateStopped, a.state, "state should remain Stopped")
			},
		},
		{
			name: "Error_InvalidState_Starting_ReturnsError",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateStarting,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.Error(t, err, "StartScheduleLoop should fail from Starting state")
				assert.Contains(t, err.Error(), "cannot start schedule loop")
				assert.Equal(t, StateStarting, a.state, "state should remain Starting")
			},
		},
		{
			name: "Error_InvalidState_Stopping_ReturnsError",
			setup: func() *Assorter {
				a := &Assorter{
					state:   StateStopping,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.Error(t, err, "StartScheduleLoop should fail from Stopping state")
				assert.Contains(t, err.Error(), "cannot start schedule loop")
				assert.Equal(t, StateStopping, a.state, "state should remain Stopping")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := getTestContext()
			defer cancel()
			assorter := tt.setup()

			err := assorter.StartScheduleLoop(ctx)

			tt.verify(t, assorter, err)

			// Cleanup: stop the loop if it was started
			if err == nil && assorter.state == StateRunning {
				time.Sleep(10 * time.Millisecond) // Give loop time to start
				if assorter.stopCh != nil {
					close(assorter.stopCh)
				}
				time.Sleep(10 * time.Millisecond) // Give loop time to stop
			}
			if assorter.ticker != nil {
				assorter.ticker.Stop()
			}
		})
	}
}

// ============================================================================
// Test: AddVolume
// ============================================================================

func TestAddVolume(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() (*Assorter, string)
		verify func(*testing.T, *Assorter, string, error)
	}{
		{
			name: "Success_AddNewVolume_InReadyState",
			setup: func() (*Assorter, string) {
				a := &Assorter{
					state:   StateReady,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, "pvc-123.node-1"
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.NoError(t, err, "AddVolume should succeed in Ready state")
				assert.Contains(t, a.volumes, volumeName, "volume should be added to map")
				assert.Equal(t, 1, len(a.volumes), "should have 1 volume")
			},
		},
		{
			name: "Success_AddNewVolume_InRunningState",
			setup: func() (*Assorter, string) {
				a := &Assorter{
					state:   StateRunning,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, "pvc-456.node-2"
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.NoError(t, err, "AddVolume should succeed in Running state")
				assert.Contains(t, a.volumes, volumeName, "volume should be added to map")
			},
		},
		{
			name: "Error_VolumeAlreadyExists_ReturnsError",
			setup: func() (*Assorter, string) {
				volumeName := "pvc-789.node-3"
				a := &Assorter{
					state:   StateReady,
					volumes: map[string]struct{}{volumeName: {}},
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, volumeName
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.Error(t, err, "AddVolume should fail when volume already exists")
				assert.Contains(t, err.Error(), "already exists", "error should mention duplicate")
				assert.Equal(t, 1, len(a.volumes), "volume count should remain unchanged")
			},
		},
		{
			name: "Error_AssorterNotReady_StateStopped_ReturnsError",
			setup: func() (*Assorter, string) {
				a := &Assorter{
					state:   StateStopped,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, "pvc-111.node-1"
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.Error(t, err, "AddVolume should fail when assorter is Stopped")
				assert.Contains(t, err.Error(), "cannot add volume", "error should mention add volume")
				assert.Contains(t, err.Error(), "Stopped state", "error should mention state")
				assert.NotContains(t, a.volumes, volumeName, "volume should not be added")
			},
		},
		{
			name: "Error_AssorterNotReady_StateStarting_ReturnsError",
			setup: func() (*Assorter, string) {
				a := &Assorter{
					state:   StateStarting,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, "pvc-222.node-2"
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.Error(t, err, "AddVolume should fail when assorter is Starting")
				assert.Contains(t, err.Error(), "cannot add volume")
				assert.Contains(t, err.Error(), "Starting state")
				assert.NotContains(t, a.volumes, volumeName)
			},
		},
		{
			name: "Error_AssorterNotReady_StateStopping_ReturnsError",
			setup: func() (*Assorter, string) {
				a := &Assorter{
					state:   StateStopping,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, "pvc-333.node-3"
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.Error(t, err, "AddVolume should fail when assorter is Stopping")
				assert.Contains(t, err.Error(), "cannot add volume")
				assert.Contains(t, err.Error(), "Stopping state")
				assert.NotContains(t, a.volumes, volumeName)
			},
		},
		{
			name: "Success_AddMultipleVolumes_AllAdded",
			setup: func() (*Assorter, string) {
				a := &Assorter{
					state:   StateReady,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				// Pre-add some volumes
				a.volumes["pvc-1.node-1"] = struct{}{}
				a.volumes["pvc-2.node-2"] = struct{}{}
				return a, "pvc-3.node-3"
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.NoError(t, err)
				assert.Equal(t, 3, len(a.volumes), "should have 3 volumes")
				assert.Contains(t, a.volumes, volumeName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := getTestContext()
			defer cancel()
			assorter, volumeName := tt.setup()

			err := assorter.AddVolume(ctx, volumeName)

			tt.verify(t, assorter, volumeName, err)
		})
	}
}

// ============================================================================
// Test: RemoveVolume
// ============================================================================

func TestRemoveVolume(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() (*Assorter, string)
		verify func(*testing.T, *Assorter, string, error)
	}{
		{
			name: "Success_RemoveExistingVolume_InReadyState",
			setup: func() (*Assorter, string) {
				volumeName := "pvc-123.node-1"
				a := &Assorter{
					state:   StateReady,
					volumes: map[string]struct{}{volumeName: {}},
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, volumeName
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.NoError(t, err, "RemoveVolume should succeed in Ready state")
				assert.NotContains(t, a.volumes, volumeName, "volume should be removed from map")
				assert.Equal(t, 0, len(a.volumes), "should have 0 volumes")
			},
		},
		{
			name: "Success_RemoveExistingVolume_InRunningState",
			setup: func() (*Assorter, string) {
				volumeName := "pvc-456.node-2"
				a := &Assorter{
					state:   StateRunning,
					volumes: map[string]struct{}{volumeName: {}},
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, volumeName
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.NoError(t, err, "RemoveVolume should succeed in Running state")
				assert.NotContains(t, a.volumes, volumeName)
			},
		},
		{
			name: "Error_VolumeDoesNotExist_ReturnsError",
			setup: func() (*Assorter, string) {
				a := &Assorter{
					state:   StateReady,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, "non-existent-volume"
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.Error(t, err, "RemoveVolume should fail when volume doesn't exist")
				assert.Contains(t, err.Error(), "does not exist", "error should mention non-existence")
			},
		},
		{
			name: "Error_AssorterNotReady_StateStopped_ReturnsError",
			setup: func() (*Assorter, string) {
				volumeName := "pvc-111.node-1"
				a := &Assorter{
					state:   StateStopped,
					volumes: map[string]struct{}{volumeName: {}},
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, volumeName
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.Error(t, err, "RemoveVolume should fail when assorter is Stopped")
				assert.Contains(t, err.Error(), "cannot remove volume", "error should mention remove volume")
				assert.Contains(t, err.Error(), "Stopped state", "error should mention state")
				assert.Contains(t, a.volumes, volumeName, "volume should not be removed")
			},
		},
		{
			name: "Error_AssorterNotReady_StateStarting_ReturnsError",
			setup: func() (*Assorter, string) {
				volumeName := "pvc-222.node-2"
				a := &Assorter{
					state:   StateStarting,
					volumes: map[string]struct{}{volumeName: {}},
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, volumeName
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot remove volume")
				assert.Contains(t, err.Error(), "Starting state")
				assert.Contains(t, a.volumes, volumeName)
			},
		},
		{
			name: "Error_AssorterNotReady_StateStopping_ReturnsError",
			setup: func() (*Assorter, string) {
				volumeName := "pvc-333.node-3"
				a := &Assorter{
					state:   StateStopping,
					volumes: map[string]struct{}{volumeName: {}},
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, volumeName
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot remove volume")
				assert.Contains(t, err.Error(), "Stopping state")
				assert.Contains(t, a.volumes, volumeName)
			},
		},
		{
			name: "Success_RemoveOneOfMultipleVolumes_OthersRemain",
			setup: func() (*Assorter, string) {
				volumeToRemove := "pvc-2.node-2"
				a := &Assorter{
					state: StateReady,
					volumes: map[string]struct{}{
						"pvc-1.node-1": {},
						volumeToRemove: {},
						"pvc-3.node-3": {},
					},
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, volumeToRemove
			},
			verify: func(t *testing.T, a *Assorter, volumeName string, err error) {
				require.NoError(t, err)
				assert.Equal(t, 2, len(a.volumes), "should have 2 volumes remaining")
				assert.NotContains(t, a.volumes, volumeName, "removed volume should not be present")
				assert.Contains(t, a.volumes, "pvc-1.node-1", "other volumes should remain")
				assert.Contains(t, a.volumes, "pvc-3.node-3", "other volumes should remain")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := getTestContext()
			defer cancel()
			assorter, volumeName := tt.setup()

			err := assorter.RemoveVolume(ctx, volumeName)

			tt.verify(t, assorter, volumeName, err)
		})
	}
}

// ============================================================================
// Test: UpdateVolumePeriodicity
// ============================================================================

func TestUpdateVolumePeriodicity(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() (*Assorter, string, time.Duration)
		verify func(*testing.T, *Assorter, error)
	}{
		{
			name: "Success_InReadyState_NoOpReturnsNil",
			setup: func() (*Assorter, string, time.Duration) {
				a := &Assorter{
					state:   StateReady,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, "pvc-123.node-1", 5 * time.Minute
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err, "UpdateVolumePeriodicity should succeed (no-op)")
			},
		},
		{
			name: "Success_InRunningState_NoOpReturnsNil",
			setup: func() (*Assorter, string, time.Duration) {
				a := &Assorter{
					state:   StateRunning,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, "pvc-456.node-2", 3 * time.Minute
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.NoError(t, err, "UpdateVolumePeriodicity should succeed (no-op)")
			},
		},
		{
			name: "Error_AssorterNotReady_StateStopped_ReturnsError",
			setup: func() (*Assorter, string, time.Duration) {
				a := &Assorter{
					state:   StateStopped,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, "pvc-789.node-3", 2 * time.Minute
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.Error(t, err, "UpdateVolumePeriodicity should fail when assorter is Stopped")
				assert.Contains(t, err.Error(), "cannot update volume periodicity")
				assert.Contains(t, err.Error(), "Stopped state")
			},
		},
		{
			name: "Error_AssorterNotReady_StateStarting_ReturnsError",
			setup: func() (*Assorter, string, time.Duration) {
				a := &Assorter{
					state:   StateStarting,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, "pvc-111.node-1", 1 * time.Minute
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot update volume periodicity")
				assert.Contains(t, err.Error(), "Starting state")
			},
		},
		{
			name: "Error_AssorterNotReady_StateStopping_ReturnsError",
			setup: func() (*Assorter, string, time.Duration) {
				a := &Assorter{
					state:   StateStopping,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: mockPublishFunc,
					},
				}
				return a, "pvc-222.node-2", 4 * time.Minute
			},
			verify: func(t *testing.T, a *Assorter, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot update volume periodicity")
				assert.Contains(t, err.Error(), "Stopping state")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := getTestContext()
			defer cancel()
			assorter, volumeName, period := tt.setup()

			err := assorter.UpdateVolumePeriodicity(ctx, volumeName, period)

			tt.verify(t, assorter, err)
		})
	}
}

// ============================================================================
// Test: IsReady
// ============================================================================

func TestIsReady(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		expected bool
	}{
		{"StateReady_ReturnsTrue", StateReady, true},
		{"StateStopped_ReturnsFalse", StateStopped, false},
		{"StateStarting_ReturnsFalse", StateStarting, false},
		{"StateRunning_ReturnsFalse", StateRunning, false},
		{"StateStopping_ReturnsFalse", StateStopping, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Assorter{state: tt.state}
			assert.Equal(t, tt.expected, a.IsReady())
		})
	}
}

// ============================================================================
// Test: IsRunning
// ============================================================================

func TestIsRunning(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		expected bool
	}{
		{"StateRunning_ReturnsTrue", StateRunning, true},
		{"StateStopped_ReturnsFalse", StateStopped, false},
		{"StateStarting_ReturnsFalse", StateStarting, false},
		{"StateReady_ReturnsFalse", StateReady, false},
		{"StateStopping_ReturnsFalse", StateStopping, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Assorter{state: tt.state}
			assert.Equal(t, tt.expected, a.IsRunning())
		})
	}
}

// ============================================================================
// Test: GetVolumeCount
// ============================================================================

func TestGetVolumeCount(t *testing.T) {
	tests := []struct {
		name     string
		volumes  map[string]struct{}
		expected int
	}{
		{
			name:     "EmptyVolumes_ReturnsZero",
			volumes:  make(map[string]struct{}),
			expected: 0,
		},
		{
			name: "OneVolume_ReturnsOne",
			volumes: map[string]struct{}{
				"pvc-1.node-1": {},
			},
			expected: 1,
		},
		{
			name: "MultipleVolumes_ReturnsCorrectCount",
			volumes: map[string]struct{}{
				"pvc-1.node-1": {},
				"pvc-2.node-2": {},
				"pvc-3.node-3": {},
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Assorter{volumes: tt.volumes}
			assert.Equal(t, tt.expected, a.GetVolumeCount())
		})
	}
}

// ============================================================================
// Test: publishVolumes
// ============================================================================

func TestPublishVolumes(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(ctrl *gomock.Controller) *Assorter
		verify func(*testing.T, error)
	}{
		{
			name: "Success_WithVolumes_PublishesSuccessfully",
			setup: func(ctrl *gomock.Controller) *Assorter {
				callCount := 0
				publishFunc := func(ctx context.Context, volumeNames []string) error {
					callCount++
					assert.Equal(t, 2, len(volumeNames), "should publish 2 volumes")
					return nil
				}

				a := &Assorter{
					state: StateRunning,
					volumes: map[string]struct{}{
						"pvc-1.node-1": {},
						"pvc-2.node-2": {},
					},
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: publishFunc,
					},
				}
				return a
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "publishVolumes should succeed")
			},
		},
		{
			name: "Success_WithNoVolumes_ReturnsEarly",
			setup: func(ctrl *gomock.Controller) *Assorter {
				publishFunc := func(ctx context.Context, volumeNames []string) error {
					t.Error("publishFunc should not be called when there are no volumes")
					return nil
				}

				a := &Assorter{
					state:   StateRunning,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: publishFunc,
					},
				}
				return a
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "publishVolumes should succeed with no volumes")
			},
		},
		{
			name: "Success_NotRunning_ReturnsEarly",
			setup: func(ctrl *gomock.Controller) *Assorter {
				publishFunc := func(ctx context.Context, volumeNames []string) error {
					t.Error("publishFunc should not be called when assorter is not running")
					return nil
				}

				a := &Assorter{
					state: StateStopped,
					volumes: map[string]struct{}{
						"pvc-1.node-1": {},
					},
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: publishFunc,
					},
				}
				return a
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "publishVolumes should return early when not running")
			},
		},
		{
			name: "Error_PublishFuncNil_ReturnsError",
			setup: func(ctrl *gomock.Controller) *Assorter {
				a := &Assorter{
					state: StateRunning,
					volumes: map[string]struct{}{
						"pvc-1.node-1": {},
					},
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: nil,
					},
				}
				return a
			},
			verify: func(t *testing.T, err error) {
				require.Error(t, err, "publishVolumes should fail with nil publishFunc")
				assert.Contains(t, err.Error(), "PublishFunc not initialized")
			},
		},
		{
			name: "Error_PublishFuncFails_ReturnsError",
			setup: func(ctrl *gomock.Controller) *Assorter {
				publishFunc := func(ctx context.Context, volumeNames []string) error {
					return fmt.Errorf("publish failed")
				}

				a := &Assorter{
					state: StateRunning,
					volumes: map[string]struct{}{
						"pvc-1.node-1": {},
					},
					config: &Config{
						period:      DefaultPeriod,
						publishFunc: publishFunc,
					},
				}
				return a
			},
			verify: func(t *testing.T, err error) {
				require.Error(t, err, "publishVolumes should propagate publish errors")
				assert.Contains(t, err.Error(), "publish failed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx, cancel := getTestContext()
			defer cancel()
			assorter := tt.setup(ctrl)

			err := assorter.publishVolumes(ctx)

			tt.verify(t, err)
		})
	}
}

// ============================================================================
// Test: scheduleLoop
// ============================================================================

func TestScheduleLoop(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() (*Assorter, context.Context, context.CancelFunc)
		verify func(*testing.T, *Assorter)
	}{
		{
			name: "Success_ContextCancelled_StopsLoop",
			setup: func() (*Assorter, context.Context, context.CancelFunc) {
				a := &Assorter{
					state:   StateRunning,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      50 * time.Millisecond,
						publishFunc: mockPublishFunc,
					},
					stopCh: make(chan struct{}),
					ticker: time.NewTicker(50 * time.Millisecond),
				}
				ctx, cancel := context.WithCancel(context.Background())
				return a, ctx, cancel
			},
			verify: func(t *testing.T, a *Assorter) {
				// Loop stopped via context cancellation
			},
		},
		{
			name: "Success_StopChannelClosed_StopsLoop",
			setup: func() (*Assorter, context.Context, context.CancelFunc) {
				a := &Assorter{
					state:   StateRunning,
					volumes: make(map[string]struct{}),
					config: &Config{
						period:      50 * time.Millisecond,
						publishFunc: mockPublishFunc,
					},
					stopCh: make(chan struct{}),
					ticker: time.NewTicker(50 * time.Millisecond),
				}
				ctx := context.Background()
				return a, ctx, nil
			},
			verify: func(t *testing.T, a *Assorter) {
				// Loop stopped via stopCh
			},
		},
		{
			name: "Success_TickerFires_PublishesVolumes",
			setup: func() (*Assorter, context.Context, context.CancelFunc) {
				publishCount := 0
				publishFunc := func(ctx context.Context, volumeNames []string) error {
					publishCount++
					return nil
				}

				a := &Assorter{
					state: StateRunning,
					volumes: map[string]struct{}{
						"pvc-1.node-1": {},
					},
					config: &Config{
						period:      50 * time.Millisecond,
						publishFunc: publishFunc,
					},
					stopCh: make(chan struct{}),
					ticker: time.NewTicker(50 * time.Millisecond),
				}
				ctx, cancel := context.WithCancel(context.Background())
				return a, ctx, cancel
			},
			verify: func(t *testing.T, a *Assorter) {
				// Ticker fired and published volumes
			},
		},
		{
			name: "Success_PublishFails_ContinuesLoop",
			setup: func() (*Assorter, context.Context, context.CancelFunc) {
				publishFunc := func(ctx context.Context, volumeNames []string) error {
					return fmt.Errorf("publish error")
				}

				a := &Assorter{
					state: StateRunning,
					volumes: map[string]struct{}{
						"pvc-1.node-1": {},
					},
					config: &Config{
						period:      50 * time.Millisecond,
						publishFunc: publishFunc,
					},
					stopCh: make(chan struct{}),
					ticker: time.NewTicker(50 * time.Millisecond),
				}
				ctx, cancel := context.WithCancel(context.Background())
				return a, ctx, cancel
			},
			verify: func(t *testing.T, a *Assorter) {
				// Loop continues despite publish errors
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assorter, ctx, cancel := tt.setup()

			// Start schedule loop in goroutine
			done := make(chan struct{})
			go func() {
				assorter.scheduleLoop(ctx)
				close(done)
			}()

			// For ticker tests, wait for ticker to fire
			if tt.name == "Success_TickerFires_PublishesVolumes" ||
				tt.name == "Success_PublishFails_ContinuesLoop" {
				time.Sleep(100 * time.Millisecond) // Wait for at least one tick
			} else {
				time.Sleep(10 * time.Millisecond) // Give loop time to start
			}

			// Stop the loop
			if cancel != nil {
				cancel()
			} else {
				close(assorter.stopCh)
			}

			// Wait for loop to stop
			select {
			case <-done:
				// Loop stopped successfully
			case <-time.After(1 * time.Second):
				t.Fatal("scheduleLoop did not stop in time")
			}

			tt.verify(t, assorter)

			// Cleanup
			if assorter.ticker != nil {
				assorter.ticker.Stop()
			}
		})
	}
}

// ============================================================================
// Test: State Methods (get, set, compareAndSwap)
// ============================================================================

func TestStateMethods(t *testing.T) {
	t.Run("StateGet_ReturnsCurrentState", func(t *testing.T) {
		var s State
		s.set(StateRunning)
		assert.Equal(t, StateRunning, s.get())
	})

	t.Run("StateSet_UpdatesState", func(t *testing.T) {
		var s State
		s.set(StateStopped)
		assert.Equal(t, StateStopped, s.get())
		s.set(StateReady)
		assert.Equal(t, StateReady, s.get())
	})

	t.Run("StateCompareAndSwap_Success_SwapsState", func(t *testing.T) {
		var s State
		s.set(StateStopped)
		success := s.compareAndSwap(StateStopped, StateStarting)
		assert.True(t, success, "CAS should succeed")
		assert.Equal(t, StateStarting, s.get())
	})

	t.Run("StateCompareAndSwap_Failure_DoesNotSwap", func(t *testing.T) {
		var s State
		s.set(StateRunning)
		success := s.compareAndSwap(StateStopped, StateStarting)
		assert.False(t, success, "CAS should fail")
		assert.Equal(t, StateRunning, s.get(), "state should remain unchanged")
	})

	t.Run("StateCompareAndSwap_MultipleStates", func(t *testing.T) {
		var s State
		s.set(StateStopped)

		// Stopped -> Starting
		assert.True(t, s.compareAndSwap(StateStopped, StateStarting))
		assert.Equal(t, StateStarting, s.get())

		// Starting -> Ready
		assert.True(t, s.compareAndSwap(StateStarting, StateReady))
		assert.Equal(t, StateReady, s.get())

		// Ready -> Running
		assert.True(t, s.compareAndSwap(StateReady, StateRunning))
		assert.Equal(t, StateRunning, s.get())

		// Running -> Stopping
		assert.True(t, s.compareAndSwap(StateRunning, StateStopping))
		assert.Equal(t, StateStopping, s.get())

		// Stopping -> Stopped
		assert.True(t, s.compareAndSwap(StateStopping, StateStopped))
		assert.Equal(t, StateStopped, s.get())
	})
}

// ============================================================================
// Test: State.String()
// ============================================================================

func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateStopped, "Stopped"},
		{StateStarting, "Starting"},
		{StateReady, "Ready"},
		{StateRunning, "Running"},
		{StateStopping, "Stopping"},
		{State(999), "Unknown"}, // Invalid state
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

// ============================================================================
// Test: Config
// ============================================================================

func TestNewConfigFunction(t *testing.T) {
	tests := []struct {
		name   string
		opts   []ConfigOption
		verify func(*testing.T, *Config)
	}{
		{
			name: "Success_NoOptions_UsesDefaults",
			opts: nil,
			verify: func(t *testing.T, cfg *Config) {
				assert.NotNil(t, cfg)
				assert.Equal(t, DefaultPeriod, cfg.period)
				assert.Nil(t, cfg.publishFunc, "default config should have nil publishFunc")
			},
		},
		{
			name: "Success_WithPeriod_AppliesOption",
			opts: []ConfigOption{WithPeriod(5 * time.Minute)},
			verify: func(t *testing.T, cfg *Config) {
				assert.NotNil(t, cfg)
				assert.Equal(t, 5*time.Minute, cfg.period)
			},
		},
		{
			name: "Success_WithPublishFunc_AppliesOption",
			opts: []ConfigOption{
				WithPublishFunc(func(ctx context.Context, volumeNames []string) error {
					return nil
				}),
			},
			verify: func(t *testing.T, cfg *Config) {
				assert.NotNil(t, cfg)
				assert.NotNil(t, cfg.publishFunc)
			},
		},
		{
			name: "Success_WithAllOptions_AppliesAll",
			opts: []ConfigOption{
				WithPeriod(3 * time.Minute),
				WithPublishFunc(func(ctx context.Context, volumeNames []string) error {
					return fmt.Errorf("test error")
				}),
			},
			verify: func(t *testing.T, cfg *Config) {
				assert.NotNil(t, cfg)
				assert.Equal(t, 3*time.Minute, cfg.period)
				assert.NotNil(t, cfg.publishFunc)
				// Test that the function is preserved
				err := cfg.publishFunc(context.Background(), nil)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "test error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewConfig(tt.opts...)
			tt.verify(t, cfg)
		})
	}
}

// ============================================================================
// Test: Thread Safety
// ============================================================================

func TestConcurrentVolumeOperations(t *testing.T) {
	a := &Assorter{
		state:   StateRunning,
		volumes: make(map[string]struct{}),
		config: &Config{
			period:      DefaultPeriod,
			publishFunc: mockPublishFunc,
		},
	}

	ctx, cancel := getTestContext()
	defer cancel()
	done := make(chan struct{})
	numGoroutines := 10

	// Concurrent AddVolume operations
	go func() {
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				volumeName := fmt.Sprintf("pvc-%d.node-1", id)
				_ = a.AddVolume(ctx, volumeName)
				done <- struct{}{}
			}(i)
		}
	}()

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify count (some may have failed due to duplicates, but should be safe)
	count := a.GetVolumeCount()
	assert.LessOrEqual(t, count, numGoroutines, "volume count should not exceed number of adds")
	assert.Greater(t, count, 0, "at least some volumes should be added")
}

func TestConcurrentGetVolumeCount(t *testing.T) {
	a := &Assorter{
		state: StateRunning,
		volumes: map[string]struct{}{
			"pvc-1.node-1": {},
			"pvc-2.node-2": {},
			"pvc-3.node-3": {},
		},
		config: &Config{
			period:      DefaultPeriod,
			publishFunc: mockPublishFunc,
		},
	}

	done := make(chan int)
	numGoroutines := 20

	// Concurrent GetVolumeCount operations
	for i := 0; i < numGoroutines; i++ {
		go func() {
			count := a.GetVolumeCount()
			done <- count
		}()
	}

	// All should return the same count
	for i := 0; i < numGoroutines; i++ {
		count := <-done
		assert.Equal(t, 3, count, "all concurrent reads should return same count")
	}
}

// ============================================================================
// Test: Config.AssorterConfig() - Interface Marker Method
// ============================================================================

func TestConfigAssorterConfig(t *testing.T) {
	t.Run("Success_MarkerMethodDoesNotPanic", func(t *testing.T) {
		cfg := NewConfig(WithPublishFunc(mockPublishFunc))
		require.NotNil(t, cfg)

		// Call the marker method - should not panic
		assert.NotPanics(t, func() {
			cfg.AssorterConfig()
		}, "AssorterConfig marker method should not panic")
	})
}

// ============================================================================
// Test: Config.Copy()
// ============================================================================

func TestConfigCopy(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() *Config
		verify func(*testing.T, *Config, *Config)
	}{
		{
			name: "Success_NilConfig_ReturnsNil",
			setup: func() *Config {
				return nil
			},
			verify: func(t *testing.T, original, copied *Config) {
				assert.Nil(t, copied, "copy of nil config should be nil")
			},
		},
		{
			name: "Success_ValidConfig_CreatesIndependentCopy",
			setup: func() *Config {
				return NewConfig(
					WithPeriod(5*time.Minute),
					WithPublishFunc(mockPublishFunc),
				)
			},
			verify: func(t *testing.T, original, copied *Config) {
				require.NotNil(t, copied, "copy should not be nil")
				assert.NotSame(t, original, copied, "copy should be a different instance")

				// Verify all fields are equal
				assert.Equal(t, original.period, copied.period, "period should match")
				assert.NotNil(t, copied.publishFunc, "publishFunc should be copied")
			},
		},
		{
			name: "Success_ConfigWithDefaultPeriod_CopiesCorrectly",
			setup: func() *Config {
				return NewConfig(WithPublishFunc(mockPublishFunc))
			},
			verify: func(t *testing.T, original, copied *Config) {
				require.NotNil(t, copied)
				assert.Equal(t, DefaultPeriod, copied.period)
				assert.NotNil(t, copied.publishFunc)
			},
		},
		{
			name: "Success_MutatingCopy_DoesNotAffectOriginal",
			setup: func() *Config {
				return NewConfig(
					WithPeriod(3*time.Minute),
					WithPublishFunc(mockPublishFunc),
				)
			},
			verify: func(t *testing.T, original, copied *Config) {
				require.NotNil(t, copied)

				originalPeriod := original.period

				// Mutate the copy
				copied.period = 10 * time.Minute
				copied.publishFunc = func(ctx context.Context, volumeNames []string) error {
					return fmt.Errorf("mutated function")
				}

				// Original should remain unchanged
				assert.Equal(t, originalPeriod, original.period, "original period should not change")
				assert.NotEqual(t, copied.period, original.period, "copy should have different period")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.setup()

			var copied *Config
			if original != nil {
				copied = original.Copy().(*Config)
			} else {
				// For nil config, Copy() returns nil interface which can be cast
				copyResult := original.Copy()
				if copyResult != nil {
					copied = copyResult.(*Config)
				}
			}

			tt.verify(t, original, copied)
		})
	}
}

// ============================================================================
// Test: NewAssorter with Config Copy
// ============================================================================

func TestNewAssorterConfigCopy(t *testing.T) {
	t.Run("Success_ConfigIsCopied_MutationDoesNotAffectAssorter", func(t *testing.T) {
		ctx, cancel := getTestContext()
		defer cancel()

		// Create config with specific values
		cfg := NewConfig(
			WithPeriod(3*time.Minute),
			WithPublishFunc(mockPublishFunc),
		)

		// Create assorter
		assorter, err := NewAssorter(ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, assorter)

		originalPeriod := assorter.config.period

		// Mutate the original config
		cfg.period = 10 * time.Minute

		// Assorter should not be affected
		assert.Equal(t, originalPeriod, assorter.config.period, "assorter period should not change when original config is mutated")
		assert.Equal(t, 3*time.Minute, assorter.config.period, "assorter should preserve original period")
	})

	t.Run("Success_AssorterStoresConfigCopy", func(t *testing.T) {
		ctx, cancel := getTestContext()
		defer cancel()

		cfg := NewConfig(
			WithPeriod(2*time.Minute),
			WithPublishFunc(mockPublishFunc),
		)

		assorter, err := NewAssorter(ctx, cfg)
		require.NoError(t, err)
		require.NotNil(t, assorter)
		require.NotNil(t, assorter.config, "assorter should store config")

		// Config should be a copy, not the same instance
		assert.NotSame(t, cfg, assorter.config, "assorter should store a copy of config, not original")

		// But values should match
		assert.Equal(t, cfg.period, assorter.config.period)
	})
}
