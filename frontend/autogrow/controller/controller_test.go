// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/frontend/autogrow/cache"
	agTypes "github.com/netapp/trident/frontend/autogrow/types"
	crdtypes "github.com/netapp/trident/frontend/crd/types"
	mockEventbus "github.com/netapp/trident/mocks/mock_pkg/mock_eventbus"
	"github.com/netapp/trident/pkg/eventbus"
)

// ============================================================================
// Helper Functions and Test Fixtures
// ============================================================================

// getTestContext returns a context with timeout for testing
func getTestContext() context.Context {
	ctx := context.Background()
	return ctx
}

// getTestAutogrowCache returns a test autogrow cache instance
func getTestAutogrowCache() *cache.AutogrowCache {
	return cache.NewAutogrowCache()
}

// ============================================================================
// Test: NewController
// ============================================================================

func TestNewController(t *testing.T) {
	// testSetup holds test-specific configuration and dependencies
	type testSetup struct {
		cache *cache.AutogrowCache
	}

	tests := []struct {
		name   string
		setup  func() testSetup
		verify func(*testing.T, *Controller, error)
	}{
		{
			name: "Success_WithValidCache",
			setup: func() testSetup {
				return testSetup{
					cache: getTestAutogrowCache(),
				}
			},
			verify: func(t *testing.T, c *Controller, err error) {
				require.NoError(t, err)
				require.NotNil(t, c)
				assert.NotNil(t, c.cache)
			},
		},
		{
			name: "Success_WithNilCache",
			setup: func() testSetup {
				return testSetup{
					cache: nil,
				}
			},
			verify: func(t *testing.T, c *Controller, err error) {
				require.NoError(t, err)
				require.NotNil(t, c)
				assert.Nil(t, c.cache)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := getTestContext()
			setup := tt.setup()

			controller, err := NewController(ctx, setup.cache)

			tt.verify(t, controller, err)
		})
	}
}

// ============================================================================
// Test: Activate
// ============================================================================

func TestController_Activate(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() (*Controller, context.Context)
		verify func(*testing.T, error)
	}{
		{
			name: "Success_ActivatesSuccessfully",
			setup: func() (*Controller, context.Context) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)
				return controller, ctx
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_MultipleCalls_Idempotent",
			setup: func() (*Controller, context.Context) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)
				// First activation
				_ = controller.Activate(ctx)
				return controller, ctx
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "second activation should succeed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, ctx := tt.setup()

			err := controller.Activate(ctx)

			tt.verify(t, err)
		})
	}
}

// ============================================================================
// Test: Deactivate
// ============================================================================

func TestController_Deactivate(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() (*Controller, context.Context)
		verify func(*testing.T, error)
	}{
		{
			name: "Success_DeactivatesSuccessfully",
			setup: func() (*Controller, context.Context) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)
				return controller, ctx
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_MultipleCalls_Idempotent",
			setup: func() (*Controller, context.Context) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)
				// First deactivation
				_ = controller.Deactivate(ctx)
				return controller, ctx
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "second deactivation should succeed")
			},
		},
		{
			name: "Success_DeactivateWithoutActivate",
			setup: func() (*Controller, context.Context) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)
				return controller, ctx
			},
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err, "should be safe to deactivate without activating first")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, ctx := tt.setup()

			err := controller.Deactivate(ctx)

			tt.verify(t, err)
		})
	}
}

// ============================================================================
// Test: HandleStorageClassEvent
// ============================================================================

func TestController_HandleStorageClassEvent(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func())
		verify func(*testing.T)
	}{
		{
			name: "Success_AddEvent_PublishesCorrectly",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.ControllerEvent) {
						assert.NotZero(t, event.ID)
						assert.Equal(t, crdtypes.ObjectTypeStorageClass, event.ObjectType)
						assert.Equal(t, crdtypes.EventAdd, event.EventType)
						assert.Equal(t, "test-sc", event.Key)
						assert.NotZero(t, event.Timestamp)
						assert.NotNil(t, event.Ctx)
					}).
					Times(1)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return controller, ctx, crdtypes.EventAdd, "test-sc", cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
		{
			name: "Success_UpdateEvent_PublishesCorrectly",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.ControllerEvent) {
						assert.Equal(t, crdtypes.ObjectTypeStorageClass, event.ObjectType)
						assert.Equal(t, crdtypes.EventUpdate, event.EventType)
						assert.Equal(t, "updated-sc", event.Key)
					}).
					Times(1)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return controller, ctx, crdtypes.EventUpdate, "updated-sc", cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
		{
			name: "Success_DeleteEvent_PublishesCorrectly",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.ControllerEvent) {
						assert.Equal(t, crdtypes.ObjectTypeStorageClass, event.ObjectType)
						assert.Equal(t, crdtypes.EventDelete, event.EventType)
						assert.Equal(t, "deleted-sc", event.Key)
					}).
					Times(1)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return controller, ctx, crdtypes.EventDelete, "deleted-sc", cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
		{
			name: "Success_NilEventBus_DoesNotPanic",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = nil

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return controller, ctx, crdtypes.EventAdd, "test-sc", cleanup
			},
			verify: func(t *testing.T) {
				// Should not panic
			},
		},
		{
			name: "Success_EmptyName_HandledCorrectly",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.ControllerEvent) {
						assert.Equal(t, "", event.Key)
					}).
					Times(1)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return controller, ctx, crdtypes.EventAdd, "", cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			controller, ctx, eventType, name, cleanup := tt.setup(ctrl)
			defer cleanup()

			// Should not panic
			assert.NotPanics(t, func() {
				controller.HandleStorageClassEvent(ctx, eventType, name)
			})

			tt.verify(t)
		})
	}
}

// ============================================================================
// Test: HandleTVPEvent
// ============================================================================

func TestController_HandleTVPEvent(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func())
		verify func(*testing.T)
	}{
		{
			name: "Success_AddEvent_PublishesCorrectly",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.ControllerEvent) {
						assert.NotZero(t, event.ID)
						assert.Equal(t, crdtypes.ObjectTypeTridentVolumePublication, event.ObjectType)
						assert.Equal(t, crdtypes.EventAdd, event.EventType)
						assert.Equal(t, "default/test-tvp", event.Key)
						assert.NotZero(t, event.Timestamp)
						assert.NotNil(t, event.Ctx)
					}).
					Times(1)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return controller, ctx, crdtypes.EventAdd, "default/test-tvp", cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
		{
			name: "Success_UpdateEvent_PublishesCorrectly",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.ControllerEvent) {
						assert.Equal(t, crdtypes.ObjectTypeTridentVolumePublication, event.ObjectType)
						assert.Equal(t, crdtypes.EventUpdate, event.EventType)
						assert.Equal(t, "namespace/tvp-name", event.Key)
					}).
					Times(1)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return controller, ctx, crdtypes.EventUpdate, "namespace/tvp-name", cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
		{
			name: "Success_DeleteEvent_PublishesCorrectly",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.ControllerEvent) {
						assert.Equal(t, crdtypes.ObjectTypeTridentVolumePublication, event.ObjectType)
						assert.Equal(t, crdtypes.EventDelete, event.EventType)
						assert.Equal(t, "ns1/tvp-del", event.Key)
					}).
					Times(1)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return controller, ctx, crdtypes.EventDelete, "ns1/tvp-del", cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
		{
			name: "Success_NilEventBus_DoesNotPanic",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = nil

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return controller, ctx, crdtypes.EventAdd, "default/tvp", cleanup
			},
			verify: func(t *testing.T) {
				// Should not panic
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			controller, ctx, eventType, name, cleanup := tt.setup(ctrl)
			defer cleanup()

			assert.NotPanics(t, func() {
				controller.HandleTVPEvent(ctx, eventType, name)
			})

			tt.verify(t)
		})
	}
}

// ============================================================================
// Test: HandleTBEEvent
// ============================================================================

func TestController_HandleTBEEvent(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func())
		verify func(*testing.T)
	}{
		{
			name: "Success_AddEvent_PublishesCorrectly",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.ControllerEvent) {
						assert.NotZero(t, event.ID)
						assert.Equal(t, crdtypes.ObjectTypeTridentBackend, event.ObjectType)
						assert.Equal(t, crdtypes.EventAdd, event.EventType)
						assert.Equal(t, "default/test-backend", event.Key)
						assert.NotZero(t, event.Timestamp)
						assert.NotNil(t, event.Ctx)
					}).
					Times(1)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return controller, ctx, crdtypes.EventAdd, "default/test-backend", cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
		{
			name: "Success_UpdateEvent_PublishesCorrectly",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.ControllerEvent) {
						assert.Equal(t, crdtypes.ObjectTypeTridentBackend, event.ObjectType)
						assert.Equal(t, crdtypes.EventUpdate, event.EventType)
						assert.Equal(t, "ns/backend-upd", event.Key)
					}).
					Times(1)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return controller, ctx, crdtypes.EventUpdate, "ns/backend-upd", cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
		{
			name: "Success_DeleteEvent_PublishesCorrectly",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.ControllerEvent) {
						assert.Equal(t, crdtypes.ObjectTypeTridentBackend, event.ObjectType)
						assert.Equal(t, crdtypes.EventDelete, event.EventType)
						assert.Equal(t, "backend-ns/del-backend", event.Key)
					}).
					Times(1)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return controller, ctx, crdtypes.EventDelete, "backend-ns/del-backend", cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
		{
			name: "Success_NilEventBus_DoesNotPanic",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = nil

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return controller, ctx, crdtypes.EventAdd, "default/backend", cleanup
			},
			verify: func(t *testing.T) {
				// Should not panic
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			controller, ctx, eventType, name, cleanup := tt.setup(ctrl)
			defer cleanup()

			assert.NotPanics(t, func() {
				controller.HandleTBEEvent(ctx, eventType, name)
			})

			tt.verify(t)
		})
	}
}

// ============================================================================
// Test: generateControllerEventID
// ============================================================================

func TestGenerateControllerEventID(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() (time.Time, string, string, string)
		verify func(*testing.T, uint64)
	}{
		{
			name: "Success_UniqueIDs_DifferentTimestamps",
			setup: func() (time.Time, string, string, string) {
				timestamp := time.Now()
				return timestamp, "StorageClass", "add", "sc1"
			},
			verify: func(t *testing.T, id1 uint64) {
				timestamp2 := time.Now().Add(1 * time.Nanosecond)
				id2 := generateControllerEventID(timestamp2, "StorageClass", "add", "sc1")
				assert.NotEqual(t, id1, id2, "different timestamps should produce different IDs")
			},
		},
		{
			name: "Success_UniqueIDs_DifferentObjectTypes",
			setup: func() (time.Time, string, string, string) {
				timestamp := time.Now()
				return timestamp, "StorageClass", "add", "test"
			},
			verify: func(t *testing.T, id1 uint64) {
				timestamp := time.Now()
				id2 := generateControllerEventID(timestamp, "TridentVolumePublication", "add", "test")
				assert.NotEqual(t, id1, id2, "different object types should produce different IDs")
			},
		},
		{
			name: "Success_UniqueIDs_DifferentEventTypes",
			setup: func() (time.Time, string, string, string) {
				timestamp := time.Now()
				return timestamp, "StorageClass", "add", "test"
			},
			verify: func(t *testing.T, id1 uint64) {
				timestamp := time.Now()
				id2 := generateControllerEventID(timestamp, "StorageClass", "update", "test")
				assert.NotEqual(t, id1, id2, "different event types should produce different IDs")
			},
		},
		{
			name: "Success_UniqueIDs_DifferentKeys",
			setup: func() (time.Time, string, string, string) {
				timestamp := time.Now()
				return timestamp, "StorageClass", "add", "sc1"
			},
			verify: func(t *testing.T, id1 uint64) {
				timestamp := time.Now()
				id2 := generateControllerEventID(timestamp, "StorageClass", "add", "sc2")
				assert.NotEqual(t, id1, id2, "different keys should produce different IDs")
			},
		},
		{
			name: "Success_Deterministic_SameInputsSameID",
			setup: func() (time.Time, string, string, string) {
				timestamp := time.Date(2026, 1, 26, 12, 0, 0, 0, time.UTC)
				return timestamp, "StorageClass", "add", "sc1"
			},
			verify: func(t *testing.T, id1 uint64) {
				timestamp := time.Date(2026, 1, 26, 12, 0, 0, 0, time.UTC)
				id2 := generateControllerEventID(timestamp, "StorageClass", "add", "sc1")
				assert.Equal(t, id1, id2, "same inputs should produce same ID")
			},
		},
		{
			name: "Success_EmptyStrings_GeneratesValidID",
			setup: func() (time.Time, string, string, string) {
				timestamp := time.Now()
				return timestamp, "", "", ""
			},
			verify: func(t *testing.T, id1 uint64) {
				assert.NotZero(t, id1, "should generate non-zero ID even with empty strings")
				timestamp := time.Now()
				id2 := generateControllerEventID(timestamp, "x", "", "")
				assert.NotZero(t, id2)
				assert.NotEqual(t, id1, id2)
			},
		},
		{
			name: "Success_NonZeroID_AlwaysGenerated",
			setup: func() (time.Time, string, string, string) {
				timestamp := time.Now()
				return timestamp, "StorageClass", "add", "test"
			},
			verify: func(t *testing.T, id uint64) {
				assert.NotZero(t, id, "ID should always be non-zero")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp, objectType, eventType, key := tt.setup()

			eventID := generateControllerEventID(timestamp, objectType, eventType, key)

			tt.verify(t, eventID)
		})
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestController_Integration_EventFlow(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*gomock.Controller) (*Controller, context.Context, func())
		verify func(*testing.T)
	}{
		{
			name: "Success_MultipleEventsFromSameController",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				publishedEvents := make([]agTypes.ControllerEvent, 0)
				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Times(6).
					DoAndReturn(func(ctx context.Context, event agTypes.ControllerEvent) error {
						publishedEvents = append(publishedEvents, event)
						return nil
					})

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus

					// Verify all events were published
					assert.Len(t, publishedEvents, 6)

					// Verify event types
					assert.Equal(t, crdtypes.ObjectTypeStorageClass, publishedEvents[0].ObjectType)
					assert.Equal(t, crdtypes.ObjectTypeStorageClass, publishedEvents[1].ObjectType)
					assert.Equal(t, crdtypes.ObjectTypeTridentVolumePublication, publishedEvents[2].ObjectType)
					assert.Equal(t, crdtypes.ObjectTypeTridentVolumePublication, publishedEvents[3].ObjectType)
					assert.Equal(t, crdtypes.ObjectTypeTridentBackend, publishedEvents[4].ObjectType)
					assert.Equal(t, crdtypes.ObjectTypeTridentBackend, publishedEvents[5].ObjectType)

					// Verify all IDs are unique
					ids := make(map[uint64]bool)
					for _, event := range publishedEvents {
						assert.False(t, ids[event.ID], "Duplicate event ID detected")
						ids[event.ID] = true
					}
				}

				// Simulate various events
				controller.HandleStorageClassEvent(ctx, crdtypes.EventAdd, "sc1")
				controller.HandleStorageClassEvent(ctx, crdtypes.EventUpdate, "sc1")
				controller.HandleTVPEvent(ctx, crdtypes.EventAdd, "default/tvp1")
				controller.HandleTVPEvent(ctx, crdtypes.EventDelete, "default/tvp2")
				controller.HandleTBEEvent(ctx, crdtypes.EventAdd, "default/backend1")
				controller.HandleTBEEvent(ctx, crdtypes.EventUpdate, "default/backend1")

				return controller, ctx, cleanup
			},
			verify: func(t *testing.T) {
				// Verification done in cleanup
			},
		},
		{
			name: "Success_ActivateDeactivateCycle",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				cleanup := func() {}

				return controller, ctx, cleanup
			},
			verify: func(t *testing.T) {
				// Verification done inline
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			controller, ctx, cleanup := tt.setup(ctrl)
			defer cleanup()

			// For ActivateDeactivateCycle test, perform the cycle
			if tt.name == "Success_ActivateDeactivateCycle" {
				err := controller.Activate(ctx)
				assert.NoError(t, err)

				err = controller.Deactivate(ctx)
				assert.NoError(t, err)

				err = controller.Activate(ctx)
				assert.NoError(t, err)

				err = controller.Deactivate(ctx)
				assert.NoError(t, err)
			}

			tt.verify(t)
		})
	}
}

// ============================================================================
// Edge Case Tests
// ============================================================================

func TestController_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*gomock.Controller) (*Controller, context.Context, func())
		verify func(*testing.T)
	}{
		{
			name: "Success_RapidFireEvents",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				numEvents := 100
				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Times(numEvents)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				// Fire many events rapidly
				for i := 0; i < numEvents; i++ {
					controller.HandleStorageClassEvent(ctx, crdtypes.EventAdd, "sc")
				}

				return controller, ctx, cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
		{
			name: "Success_LongResourceNames",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				longName := ""
				for i := 0; i < 1000; i++ {
					longName += "x"
				}

				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.ControllerEvent) {
						assert.Len(t, event.Key, 1000)
					}).
					Times(1)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				controller.HandleStorageClassEvent(ctx, crdtypes.EventAdd, longName)

				return controller, ctx, cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
		{
			name: "Success_SpecialCharactersInNames",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				specialName := "test-sc!@#$%^&*()_+={}[]|\\:;\"'<>,.?/~`"

				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.ControllerEvent) {
						assert.Equal(t, specialName, event.Key)
					}).
					Times(1)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				controller.HandleStorageClassEvent(ctx, crdtypes.EventAdd, specialName)

				return controller, ctx, cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
		{
			name: "Success_ConcurrentEventHandling",
			setup: func(ctrl *gomock.Controller) (*Controller, context.Context, func()) {
				ctx := getTestContext()
				cache := getTestAutogrowCache()
				controller, _ := NewController(ctx, cache)

				mockBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockBus

				numGoroutines := 10
				eventsPerGoroutine := 10
				expectedCalls := numGoroutines * eventsPerGoroutine

				mockBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Times(expectedCalls)

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				// Launch concurrent goroutines
				done := make(chan struct{})
				for i := 0; i < numGoroutines; i++ {
					go func(id int) {
						for j := 0; j < eventsPerGoroutine; j++ {
							controller.HandleStorageClassEvent(ctx, crdtypes.EventAdd, "sc")
						}
						done <- struct{}{}
					}(i)
				}

				// Wait for all goroutines
				for i := 0; i < numGoroutines; i++ {
					<-done
				}

				return controller, ctx, cleanup
			},
			verify: func(t *testing.T) {
				// Verification done via mock expectations
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			_, _, cleanup := tt.setup(ctrl)
			defer cleanup()

			tt.verify(t)
		})
	}
}
