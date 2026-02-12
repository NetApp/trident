// Copyright 2026 NetApp, Inc. All Rights Reserved.

package assorter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/frontend/autogrow/scheduler/assorter/periodic"
	"github.com/netapp/trident/frontend/autogrow/scheduler/assorter/types"
)

// ============================================================================
// Helper Functions
// ============================================================================

func getTestContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func mockPublishFunc(ctx context.Context, volumeNames []string) error {
	return nil
}

// fakeConfig is a fake config type for testing unsupported config scenarios
type fakeConfig struct{}

func (f *fakeConfig) AssorterConfig() {}
func (f *fakeConfig) Copy() types.Config {
	return &fakeConfig{}
}

// ============================================================================
// Test: New
// ============================================================================

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() (context.Context, []types.Config)
		verify  func(*testing.T, types.Assorter, error)
		cleanup func()
	}{
		{
			name: "Error_NoConfig_UsesDefaultButFailsWithoutPublishFunc",
			setup: func() (context.Context, []types.Config) {
				ctx, cancel := getTestContext()
				t.Cleanup(cancel)
				return ctx, nil
			},
			verify: func(t *testing.T, a types.Assorter, err error) {
				require.Error(t, err, "should fail without publishFunc in default config")
				assert.Nil(t, a, "assorter should be nil on error")
				assert.Contains(t, err.Error(), "PublishFunc is required")
			},
		},
		{
			name: "Success_WithPeriodicConfig_CreatesAssorter",
			setup: func() (context.Context, []types.Config) {
				ctx, cancel := getTestContext()
				t.Cleanup(cancel)
				cfg := periodic.NewConfig(
					periodic.WithPeriod(2*time.Minute),
					periodic.WithPublishFunc(mockPublishFunc),
				)
				return ctx, []types.Config{cfg}
			},
			verify: func(t *testing.T, a types.Assorter, err error) {
				require.NoError(t, err, "should create assorter with valid config")
				require.NotNil(t, a, "assorter should not be nil")
			},
		},
		{
			name: "Success_WithNilConfigInSlice_UsesDefault",
			setup: func() (context.Context, []types.Config) {
				ctx, cancel := getTestContext()
				t.Cleanup(cancel)
				return ctx, []types.Config{nil}
			},
			verify: func(t *testing.T, a types.Assorter, err error) {
				require.Error(t, err, "should fail with default config (no publishFunc)")
				assert.Nil(t, a)
			},
		},
		{
			name: "Error_UnsupportedConfigType_ReturnsError",
			setup: func() (context.Context, []types.Config) {
				ctx, cancel := getTestContext()
				t.Cleanup(cancel)
				return ctx, []types.Config{&fakeConfig{}}
			},
			verify: func(t *testing.T, a types.Assorter, err error) {
				require.Error(t, err, "should fail with unsupported config type")
				assert.Nil(t, a)
				assert.Contains(t, err.Error(), "unsupported config type")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cfgs := tt.setup()

			assorter, err := New[types.Assorter](ctx, cfgs...)

			tt.verify(t, assorter, err)

			if tt.cleanup != nil {
				tt.cleanup()
			}
		})
	}
}

// ============================================================================
// Test: defaultConfigFor
// ============================================================================

func TestDefaultConfigFor(t *testing.T) {
	t.Run("Success_ReturnsPeriodicConfig", func(t *testing.T) {
		cfg := defaultConfigFor[types.Assorter]()

		require.NotNil(t, cfg, "default config should not be nil")

		// Should be periodic.Config
		_, ok := cfg.(*periodic.Config)
		assert.True(t, ok, "default config should be periodic.Config")
	})
}

// ============================================================================
// Test: MustNew
// ============================================================================

func TestMustNew(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() (context.Context, []types.Config)
		shouldPanic bool
		verify      func(*testing.T, types.Assorter)
	}{
		{
			name: "Success_WithValidConfig_ReturnsAssorter",
			setup: func() (context.Context, []types.Config) {
				ctx, cancel := getTestContext()
				t.Cleanup(cancel)
				cfg := periodic.NewConfig(
					periodic.WithPeriod(1*time.Minute),
					periodic.WithPublishFunc(mockPublishFunc),
				)
				return ctx, []types.Config{cfg}
			},
			shouldPanic: false,
			verify: func(t *testing.T, a types.Assorter) {
				require.NotNil(t, a, "assorter should not be nil")
			},
		},
		{
			name: "Panic_WithInvalidConfig_Panics",
			setup: func() (context.Context, []types.Config) {
				ctx, cancel := getTestContext()
				t.Cleanup(cancel)
				// No publishFunc - will cause error
				return ctx, nil
			},
			shouldPanic: true,
			verify: func(t *testing.T, a types.Assorter) {
				// Should not reach here
				t.Error("Should have panicked")
			},
		},
		{
			name: "Panic_WithUnsupportedConfig_Panics",
			setup: func() (context.Context, []types.Config) {
				ctx, cancel := getTestContext()
				t.Cleanup(cancel)
				return ctx, []types.Config{&fakeConfig{}}
			},
			shouldPanic: true,
			verify: func(t *testing.T, a types.Assorter) {
				// Should not reach here
				t.Error("Should have panicked")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cfgs := tt.setup()

			if tt.shouldPanic {
				assert.Panics(t, func() {
					MustNew[types.Assorter](ctx, cfgs...)
				}, "MustNew should panic on error")
			} else {
				assert.NotPanics(t, func() {
					assorter := MustNew[types.Assorter](ctx, cfgs...)
					tt.verify(t, assorter)
				}, "MustNew should not panic with valid config")
			}
		})
	}
}

// ============================================================================
// Test: New with Multiple Config Types
// ============================================================================

func TestNewWithDifferentConfigTypes(t *testing.T) {
	t.Run("Success_PeriodicConfigWithCustomPeriod", func(t *testing.T) {
		ctx, cancel := getTestContext()
		defer cancel()

		cfg := periodic.NewConfig(
			periodic.WithPeriod(5*time.Minute),
			periodic.WithPublishFunc(mockPublishFunc),
		)

		assorter, err := New[types.Assorter](ctx, cfg)

		require.NoError(t, err)
		require.NotNil(t, assorter)
	})

	t.Run("Success_PeriodicConfigWithDefaultPeriod", func(t *testing.T) {
		ctx, cancel := getTestContext()
		defer cancel()

		cfg := periodic.NewConfig(
			periodic.WithPublishFunc(mockPublishFunc),
		)

		assorter, err := New[types.Assorter](ctx, cfg)

		require.NoError(t, err)
		require.NotNil(t, assorter)
	})
}
