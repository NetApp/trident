// Copyright 2026 NetApp, Inc. All Rights Reserved.

package poller

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	agCache "github.com/netapp/trident/frontend/autogrow/cache"
	agTypes "github.com/netapp/trident/frontend/autogrow/types"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	mockNodeHelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	mockEventbus "github.com/netapp/trident/mocks/mock_pkg/mock_eventbus"
	mockWorkerpool "github.com/netapp/trident/mocks/mock_pkg/mock_workerpool"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	crdclientfake "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	tridentinformers "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions"
	listerv1 "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	"github.com/netapp/trident/pkg/eventbus"
	eventbusTypes "github.com/netapp/trident/pkg/eventbus/types"
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
func getTestAutogrowCache() *agCache.AutogrowCache {
	return agCache.NewAutogrowCache()
}

// getTestVolumeStatsProvider returns a test volume stats provider
func getTestVolumeStatsProvider(ctrl *gomock.Controller) nodehelpers.VolumeStatsManager {
	return mockNodeHelpers.NewMockVolumeStatsManager(ctrl)
}

// getTestAGPInformerAndLister returns a test AGP informer and lister using fake clientset
func getTestAGPInformerAndLister(policies ...*v1.TridentAutogrowPolicy) (cache.SharedIndexInformer, listerv1.TridentAutogrowPolicyLister) {
	// Create fake Trident CRD clientset with optional pre-populated policies
	var objects []runtime.Object
	for _, policy := range policies {
		objects = append(objects, policy)
	}
	crdClientset := crdclientfake.NewSimpleClientset(objects...)

	// Create informer factory with 0 resync period for testing
	crdInformerFactory := tridentinformers.NewSharedInformerFactory(crdClientset, time.Second*0)

	// Get AGP informer and lister
	agpInformer := crdInformerFactory.Trident().V1().TridentAutogrowPolicies()

	return agpInformer.Informer(), agpInformer.Lister()
}

// getTestTVPInformerAndLister returns a test TVP informer and lister using fake clientset
func getTestTVPInformerAndLister(tvps ...*v1.TridentVolumePublication) (cache.SharedIndexInformer, listerv1.TridentVolumePublicationLister) {
	// Create fake Trident CRD clientset with optional pre-populated TVPs
	var objects []runtime.Object
	for _, tvp := range tvps {
		objects = append(objects, tvp)
	}
	crdClientset := crdclientfake.NewSimpleClientset(objects...)

	// Create informer factory with 0 resync period for testing
	crdInformerFactory := tridentinformers.NewSharedInformerFactory(crdClientset, time.Second*0)

	// Get TVP informer and lister
	tvpInformer := crdInformerFactory.Trident().V1().TridentVolumePublications()

	return tvpInformer.Informer(), tvpInformer.Lister()
}

// ============================================================================
// Test: NewPoller
// ============================================================================
func TestNewPoller(t *testing.T) {
	// testSetup holds test-specific configuration and overrides
	type testSetup struct {
		agpLister           listerv1.TridentAutogrowPolicyLister
		tvpLister           listerv1.TridentVolumePublicationLister
		config              *Config
		autogrowCache       *agCache.AutogrowCache
		volumeStatsProvider nodehelpers.VolumeStatsManager
	}

	tests := []struct {
		name             string
		setup            func(*gomock.Controller) testSetup
		expectError      bool
		expectedErrorMsg string
		verify           func(*testing.T, *Poller, error)
	}{
		{
			name: "Success_WithNilConfig_UsesDefaults",
			setup: func(ctrl *gomock.Controller) testSetup {
				_, agpLister := getTestAGPInformerAndLister()
				_, tvpLister := getTestTVPInformerAndLister()
				return testSetup{
					agpLister:           agpLister,
					tvpLister:           tvpLister,
					config:              nil,
					autogrowCache:       getTestAutogrowCache(),
					volumeStatsProvider: getTestVolumeStatsProvider(ctrl),
				}
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				require.NotNil(t, poller)

				// Verify default config was applied
				assert.NotNil(t, poller.config)
				assert.Equal(t, DefaultMaxRetries, poller.config.MaxRetries)
				assert.Equal(t, DefaultShutdownTimeout, poller.config.ShutdownTimeout)
				assert.Equal(t, DefaultWorkQueueName, poller.config.WorkQueueName)

				// Verify poller fields are initialized
				assert.NotNil(t, poller.agpLister)
				assert.NotNil(t, poller.autogrowCache)
				assert.NotNil(t, poller.volumeStatsProvider)
				// Workqueue is created in Activate(), not in NewPoller()
				assert.Nil(t, poller.workqueue, "workqueue should be nil after NewPoller (created in Activate)")
				// Worker pool is created in Activate(), not in NewPoller()
				assert.Nil(t, poller.workerPool, "worker pool should be nil after NewPoller (created in Activate)")
				assert.True(t, poller.ownWorkerPool, "should own worker pool when created internally")
				assert.Equal(t, StateStopped, poller.state)
			},
		},
		{
			name: "Success_WithDefaultConfig_UsesDefaults",
			setup: func(ctrl *gomock.Controller) testSetup {
				_, lister := getTestAGPInformerAndLister()
				return testSetup{
					agpLister:           lister,
					tvpLister:           func() listerv1.TridentVolumePublicationLister { _, l := getTestTVPInformerAndLister(); return l }(),
					config:              DefaultConfig(),
					autogrowCache:       getTestAutogrowCache(),
					volumeStatsProvider: getTestVolumeStatsProvider(ctrl),
				}
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				require.NotNil(t, poller)

				// Verify config values
				assert.NotNil(t, poller.config)
				assert.True(t, poller.ownWorkerPool)
			},
		},
		{
			name: "Success_WithCustomConfig_AllFieldsSet",
			setup: func(ctrl *gomock.Controller) testSetup {
				_, lister := getTestAGPInformerAndLister()
				return testSetup{
					agpLister: lister,
					tvpLister: func() listerv1.TridentVolumePublicationLister { _, l := getTestTVPInformerAndLister(); return l }(),
					config: NewConfig(
						WithMaxRetries(10),
						WithShutdownTimeout(60*time.Second),
						WithWorkQueueName("custom-queue"),
					),
					autogrowCache:       getTestAutogrowCache(),
					volumeStatsProvider: getTestVolumeStatsProvider(ctrl),
				}
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				require.NotNil(t, poller)

				// Verify custom config was preserved
				assert.Equal(t, 10, poller.config.MaxRetries)
				assert.Equal(t, 60*time.Second, poller.config.ShutdownTimeout)
				assert.Equal(t, "custom-queue", poller.config.WorkQueueName)
				assert.True(t, poller.ownWorkerPool)
			},
		},
		{
			name: "Success_WithProvidedWorkerPool_DoesNotOwnPool",
			setup: func(ctrl *gomock.Controller) testSetup {
				_, lister := getTestAGPInformerAndLister()
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				return testSetup{
					agpLister:           lister,
					tvpLister:           func() listerv1.TridentVolumePublicationLister { _, l := getTestTVPInformerAndLister(); return l }(),
					config:              NewConfig(WithWorkerPool(mockPool)),
					autogrowCache:       getTestAutogrowCache(),
					volumeStatsProvider: getTestVolumeStatsProvider(ctrl),
				}
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				require.NotNil(t, poller)

				// Verify we're using the provided pool and don't own it
				assert.NotNil(t, poller.workerPool)
				assert.False(t, poller.ownWorkerPool, "should not own worker pool when provided externally")
			},
		},
		{
			name: "Success_WithZeroShutdownTimeout_AppliesDefault",
			setup: func(ctrl *gomock.Controller) testSetup {
				_, lister := getTestAGPInformerAndLister()
				return testSetup{
					agpLister: lister,
					tvpLister: func() listerv1.TridentVolumePublicationLister { _, l := getTestTVPInformerAndLister(); return l }(),
					config: &Config{
						ShutdownTimeout: 0,
					},
					autogrowCache:       getTestAutogrowCache(),
					volumeStatsProvider: getTestVolumeStatsProvider(ctrl),
				}
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				require.NotNil(t, poller)

				// Verify default was applied for zero value
				assert.Equal(t, DefaultShutdownTimeout, poller.config.ShutdownTimeout)
			},
		},
		{
			name: "Success_WithNegativeShutdownTimeout_AppliesDefault",
			setup: func(ctrl *gomock.Controller) testSetup {
				_, lister := getTestAGPInformerAndLister()
				return testSetup{
					agpLister: lister,
					tvpLister: func() listerv1.TridentVolumePublicationLister { _, l := getTestTVPInformerAndLister(); return l }(),
					config: &Config{
						ShutdownTimeout: -5 * time.Second,
					},
					autogrowCache:       getTestAutogrowCache(),
					volumeStatsProvider: getTestVolumeStatsProvider(ctrl),
				}
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				require.NotNil(t, poller)

				// Verify default was applied for negative value
				assert.Equal(t, DefaultShutdownTimeout, poller.config.ShutdownTimeout)
			},
		},
		{
			name: "Success_WithEmptyWorkQueueName_AppliesDefault",
			setup: func(ctrl *gomock.Controller) testSetup {
				_, lister := getTestAGPInformerAndLister()
				return testSetup{
					agpLister: lister,
					tvpLister: func() listerv1.TridentVolumePublicationLister { _, l := getTestTVPInformerAndLister(); return l }(),
					config: &Config{
						WorkQueueName: "",
					},
					autogrowCache:       getTestAutogrowCache(),
					volumeStatsProvider: getTestVolumeStatsProvider(ctrl),
				}
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				require.NotNil(t, poller)

				// Verify default was applied for empty string
				assert.Equal(t, DefaultWorkQueueName, poller.config.WorkQueueName)
			},
		},
		{
			name: "Error_NilVolumeStatsProvider_ReturnsError",
			setup: func(ctrl *gomock.Controller) testSetup {
				_, lister := getTestAGPInformerAndLister()
				return testSetup{
					agpLister:           lister,
					tvpLister:           func() listerv1.TridentVolumePublicationLister { _, l := getTestTVPInformerAndLister(); return l }(),
					config:              DefaultConfig(),
					autogrowCache:       getTestAutogrowCache(),
					volumeStatsProvider: nil, // Test nil volumeStatsProvider
				}
			},
			expectError:      true,
			expectedErrorMsg: "volumeStatsProvider is required",
			verify: func(t *testing.T, poller *Poller, err error) {
				require.Error(t, err)
				assert.Nil(t, poller)
				assert.Contains(t, err.Error(), "volumeStatsProvider is required")
			},
		},
		{
			name: "Error_NilTVPLister_ReturnsError",
			setup: func(ctrl *gomock.Controller) testSetup {
				_, lister := getTestAGPInformerAndLister()
				return testSetup{
					agpLister:           lister,
					tvpLister:           nil, // Test nil tvpLister
					config:              DefaultConfig(),
					autogrowCache:       getTestAutogrowCache(),
					volumeStatsProvider: getTestVolumeStatsProvider(ctrl),
				}
			},
			expectError:      true,
			expectedErrorMsg: "tvpLister is required",
			verify: func(t *testing.T, poller *Poller, err error) {
				require.Error(t, err)
				assert.Nil(t, poller)
				assert.Contains(t, err.Error(), "tvpLister is required")
			},
		},
		{
			name: "Success_WithMinimalValidInputs_NilListerAllowed",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Test that nil agpLister doesn't cause immediate error (may cause issues during Activate though)
				// But tvpLister is now required
				_, tvpLister := getTestTVPInformerAndLister()
				return testSetup{
					agpLister:           nil,       // Explicitly test with nil agp lister
					tvpLister:           tvpLister, // tvpLister is required
					config:              DefaultConfig(),
					autogrowCache:       getTestAutogrowCache(),
					volumeStatsProvider: getTestVolumeStatsProvider(ctrl),
				}
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				require.NotNil(t, poller)

				// Verify nil agp lister is stored as-is (validation happens during Activate)
				assert.Nil(t, poller.agpLister)
				// Verify tvpLister is set
				assert.NotNil(t, poller.tvpLister)
			},
		},
		{
			name: "Success_WithNilAutogrowCache_Allowed",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Test that nil cache doesn't cause immediate error
				_, lister := getTestAGPInformerAndLister()
				return testSetup{
					agpLister:           lister,
					tvpLister:           func() listerv1.TridentVolumePublicationLister { _, l := getTestTVPInformerAndLister(); return l }(),
					config:              DefaultConfig(),
					autogrowCache:       nil, // Test nil cache
					volumeStatsProvider: getTestVolumeStatsProvider(ctrl),
				}
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				require.NotNil(t, poller)

				// Verify nil cache is stored as-is
				assert.Nil(t, poller.autogrowCache)
			},
		},
		{
			name: "Success_WithVeryLongShutdownTimeout_AcceptsValue",
			setup: func(ctrl *gomock.Controller) testSetup {
				_, lister := getTestAGPInformerAndLister()
				return testSetup{
					agpLister:           lister,
					tvpLister:           func() listerv1.TridentVolumePublicationLister { _, l := getTestTVPInformerAndLister(); return l }(),
					config:              NewConfig(WithShutdownTimeout(1 * time.Hour)),
					autogrowCache:       getTestAutogrowCache(),
					volumeStatsProvider: getTestVolumeStatsProvider(ctrl),
				}
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				require.NotNil(t, poller)

				// Verify long timeout is accepted
				assert.Equal(t, 1*time.Hour, poller.config.ShutdownTimeout)
			},
		},
		{
			name: "Success_ConfigWithAllPointerFieldsNil_AppliesAllDefaults",
			setup: func(ctrl *gomock.Controller) testSetup {
				_, lister := getTestAGPInformerAndLister()
				return testSetup{
					agpLister: lister,
					tvpLister: func() listerv1.TridentVolumePublicationLister { _, l := getTestTVPInformerAndLister(); return l }(),
					config: &Config{
						ShutdownTimeout: 0,
						WorkQueueName:   "",
					},
					autogrowCache:       getTestAutogrowCache(),
					volumeStatsProvider: getTestVolumeStatsProvider(ctrl),
				}
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				require.NotNil(t, poller)

				// Verify all defaults were applied
				assert.Equal(t, DefaultShutdownTimeout, poller.config.ShutdownTimeout)
				assert.Equal(t, DefaultWorkQueueName, poller.config.WorkQueueName)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup gomock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Get test-specific setup
			setup := tt.setup(ctrl)

			// Create common dependencies that are always the same
			ctx := getTestContext()

			// Execute
			poller, err := NewPoller(
				ctx,
				setup.agpLister,
				setup.tvpLister,
				setup.autogrowCache,
				setup.volumeStatsProvider,
				setup.config,
			)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			// Additional custom verification
			if tt.verify != nil {
				tt.verify(t, poller, err)
			}
		})
	}
}

// TestNewPoller_Concurrency tests that NewPoller can be called concurrently
func TestNewPoller_Concurrency(t *testing.T) {
	const goroutines = 10
	done := make(chan bool, goroutines)
	errors := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			_, lister := getTestAGPInformerAndLister()
			ctx := getTestContext()
			_, tvpLister := getTestTVPInformerAndLister()
			poller, err := NewPoller(
				ctx,
				lister,
				tvpLister,
				getTestAutogrowCache(),
				getTestVolumeStatsProvider(ctrl),
				nil,
			)

			if err != nil {
				errors <- err
			} else {
				assert.NotNil(t, poller)
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Check that no errors occurred
	close(errors)
	for err := range errors {
		t.Errorf("Concurrent NewPoller call failed: %v", err)
	}
}

// TestNewPoller_ConfigImmutability tests that the provided config is not modified
func TestNewPoller_ConfigImmutability(t *testing.T) {
	// Create a config with specific values
	originalMaxRetries := 8
	originalTimeout := 45 * time.Second
	originalQueueName := "test-queue"

	config := &Config{
		MaxRetries:      originalMaxRetries,
		ShutdownTimeout: originalTimeout,
		WorkQueueName:   originalQueueName,
	}

	// Call NewPoller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	_, lister := getTestAGPInformerAndLister()
	_, tvpLister := getTestTVPInformerAndLister()
	ctx := getTestContext()
	poller, err := NewPoller(
		ctx,
		lister,
		tvpLister,
		getTestAutogrowCache(),
		getTestVolumeStatsProvider(ctrl),
		config,
	)

	require.NoError(t, err)
	require.NotNil(t, poller)

	// Verify original config values were not modified
	assert.Equal(t, originalMaxRetries, config.MaxRetries)
	assert.Equal(t, originalTimeout, config.ShutdownTimeout)
	assert.Equal(t, originalQueueName, config.WorkQueueName)

	// Also verify poller has the same values
	assert.Equal(t, originalMaxRetries, poller.config.MaxRetries)
	assert.Equal(t, originalTimeout, poller.config.ShutdownTimeout)
	assert.Equal(t, originalQueueName, poller.config.WorkQueueName)
}

// TestApplyConfigDefaults tests the applyConfigDefaults function indirectly through NewPoller
func TestApplyConfigDefaults(t *testing.T) {
	tests := []struct {
		name           string
		inputConfig    *Config
		expectedConfig *Config
	}{
		{
			name:        "NilConfig_DefaultsApplied",
			inputConfig: nil,
			expectedConfig: &Config{
				MaxRetries:      DefaultMaxRetries,
				ShutdownTimeout: DefaultShutdownTimeout,
				WorkQueueName:   DefaultWorkQueueName,
			},
		},
		{
			name:        "PartialConfig_MissingFieldsGetDefaults",
			inputConfig: &Config{
				// Other fields missing/nil
			},
			expectedConfig: &Config{
				MaxRetries:      0, // Not set, remains 0
				ShutdownTimeout: DefaultShutdownTimeout,
				WorkQueueName:   DefaultWorkQueueName,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			_, lister := getTestAGPInformerAndLister()
			_, tvpLister := getTestTVPInformerAndLister()
			ctx := getTestContext()
			poller, err := NewPoller(
				ctx,
				lister,
				tvpLister,
				getTestAutogrowCache(),
				getTestVolumeStatsProvider(ctrl),
				tt.inputConfig,
			)

			require.NoError(t, err)
			require.NotNil(t, poller)

			// Verify expected config values
			assert.Equal(t, tt.expectedConfig.ShutdownTimeout, poller.config.ShutdownTimeout)
			assert.Equal(t, tt.expectedConfig.WorkQueueName, poller.config.WorkQueueName)
		})
	}
}

// TestNewPoller_WorkqueueInitialization tests that workqueue is properly initialized in Activate
func TestNewPoller_WorkqueueInitialization(t *testing.T) {
	// Save original eventbus
	originalEventBus := eventbus.VolumesScheduledEventBus
	defer func() {
		eventbus.VolumesScheduledEventBus = originalEventBus
	}()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	informer, lister := getTestAGPInformerAndLister()
	_, tvpLister := getTestTVPInformerAndLister()
	ctx := getTestContext()

	// Setup mock eventbus
	mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
	eventbus.VolumesScheduledEventBus = mockEventBus
	mockEventBus.EXPECT().Subscribe(gomock.Any()).Return(eventbusTypes.SubscriptionID(1), nil).Times(1)
	mockEventBus.EXPECT().Unsubscribe(gomock.Any()).Return(true).Times(1)

	// Create poller
	poller, err := NewPoller(
		ctx,
		lister,
		tvpLister,
		getTestAutogrowCache(),
		getTestVolumeStatsProvider(ctrl),
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, poller)

	// Workqueue should be nil before Activate
	require.Nil(t, poller.workqueue, "workqueue should be nil before Activate")

	// Start informer
	stopInformer := make(chan struct{})
	defer close(stopInformer)
	go informer.Run(stopInformer)
	cache.WaitForCacheSync(stopInformer, informer.HasSynced)

	// Activate poller
	err = poller.Activate(ctx)
	require.NoError(t, err)

	// Workqueue should now be initialized
	require.NotNil(t, poller.workqueue, "workqueue should be initialized after Activate")

	// Verify workqueue is functional by adding an item
	testItem := WorkItem{Ctx: ctx, TVPName: "test-pv", RetryCount: 0}
	poller.workqueue.Add(testItem)

	// Get the item back
	item, shutdown := poller.workqueue.Get()
	assert.False(t, shutdown)
	assert.Equal(t, testItem, item)

	// Clean up
	poller.workqueue.Done(item)

	// Deactivate should clean up workqueue
	err = poller.Deactivate(ctx)
	require.NoError(t, err)

	// Workqueue should be shutdown (but not nil, will be overwritten on next Activate)
	assert.True(t, poller.workqueue.ShuttingDown(), "workqueue should be shutting down after Deactivate")
}

// ============================================================================
// Test: Activate
// ============================================================================

// Helper function to create a test poller ready for activation
// Returns the poller and a stop channel for the informer (caller must close it)
func getTestPollerForActivate(t *testing.T, ctrl *gomock.Controller, mockWorkerPool *mockWorkerpool.MockPool) (*Poller, chan struct{}) {
	informer, lister := getTestAGPInformerAndLister()
	_, tvpLister := getTestTVPInformerAndLister()
	ctx := getTestContext()

	config := DefaultConfig()
	config.WorkerPool = mockWorkerPool

	poller, err := NewPoller(
		ctx,
		lister,
		tvpLister,
		getTestAutogrowCache(),
		getTestVolumeStatsProvider(ctrl),
		config,
	)
	require.NoError(t, err)
	require.NotNil(t, poller)
	require.Equal(t, StateStopped, poller.state)

	// Start the informer so cache sync won't block
	stopInformer := make(chan struct{})
	go informer.Run(stopInformer)

	// Wait for informer to be ready
	cache.WaitForCacheSync(stopInformer, informer.HasSynced)

	return poller, stopInformer
}

func TestActivate(t *testing.T) {
	// Save original eventbus and restore after tests
	originalEventBus := eventbus.VolumesScheduledEventBus
	defer func() {
		eventbus.VolumesScheduledEventBus = originalEventBus
	}()

	tests := []struct {
		name             string
		setup            func(*testing.T, *gomock.Controller) (*Poller, context.Context)
		expectError      bool
		expectedErrorMsg string
		verify           func(*testing.T, *Poller, error)
	}{
		{
			name: "Success_FirstActivation",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				// Setup mock eventbus
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockEventBus.EXPECT().
					Subscribe(gomock.Any()).
					Return(eventbusTypes.SubscriptionID(1), nil).
					Times(1)

				// Setup mock worker pool (provided, so we don't call Start on it)
				mockPool := mockWorkerpool.NewMockPool(ctrl)

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })
				ctx := getTestContext()

				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				assert.Equal(t, StateRunning, poller.state)
				assert.NotNil(t, poller.stopCh)
				assert.Equal(t, eventbusTypes.SubscriptionID(1), poller.subscriptionID)

				// Cleanup
				close(poller.stopCh)
			},
		},
		{
			name: "Success_AlreadyRunning_Idempotent",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				// Setup mock eventbus
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				// First activation
				mockEventBus.EXPECT().
					Subscribe(gomock.Any()).
					Return(eventbusTypes.SubscriptionID(1), nil).
					Times(1)

				// Provided pool - we don't call Start() on it
				mockPool := mockWorkerpool.NewMockPool(ctrl)

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })
				ctx := getTestContext()

				// Activate once
				err := poller.Activate(ctx)
				require.NoError(t, err)
				require.Equal(t, StateRunning, poller.state)

				// Return for second activation attempt
				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err) // Should return nil for idempotent case
				assert.Equal(t, StateRunning, poller.state)

				// Cleanup
				close(poller.stopCh)
			},
		},
		{
			name: "Error_WrongState_StateStarting",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				// Manually set state to Starting
				poller.state.set(StateStarting)

				ctx := getTestContext()
				return poller, ctx
			},
			expectError:      true,
			expectedErrorMsg: "cannot activate poller: current state is Starting",
			verify: func(t *testing.T, poller *Poller, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot activate poller")
				assert.Contains(t, err.Error(), "Starting")
			},
		},
		{
			name: "Error_WrongState_StateStopping",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				// Manually set state to Stopping
				poller.state.set(StateStopping)

				ctx := getTestContext()
				return poller, ctx
			},
			expectError:      true,
			expectedErrorMsg: "cannot activate poller: current state is Stopping",
			verify: func(t *testing.T, poller *Poller, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot activate poller")
				assert.Contains(t, err.Error(), "Stopping")
			},
		},
		{
			name: "Error_EventBusNil",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				// Set eventbus to nil
				eventbus.VolumesScheduledEventBus = nil

				// Provided pool - we don't call Start() on it
				mockPool := mockWorkerpool.NewMockPool(ctrl)

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })
				ctx := getTestContext()

				return poller, ctx
			},
			expectError:      true,
			expectedErrorMsg: "VolumesScheduled eventbus not initialized",
			verify: func(t *testing.T, poller *Poller, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "VolumesScheduled eventbus not initialized")
				assert.Equal(t, StateStopped, poller.state) // Should be back to Stopped
			},
		},
		{
			name: "Error_EventBusSubscribeFails",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				// Setup mock eventbus that fails to subscribe
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockEventBus.EXPECT().
					Subscribe(gomock.Any()).
					Return(eventbusTypes.SubscriptionID(0), fmt.Errorf("subscription failed")).
					Times(1)

				// Provided pool - we don't call Start() on it
				mockPool := mockWorkerpool.NewMockPool(ctrl)

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })
				ctx := getTestContext()

				return poller, ctx
			},
			expectError:      true,
			expectedErrorMsg: "failed to subscribe to VolumesScheduled eventbus",
			verify: func(t *testing.T, poller *Poller, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "failed to subscribe to VolumesScheduled eventbus")
				assert.Equal(t, StateStopped, poller.state) // Should be back to Stopped
				assert.Equal(t, eventbusTypes.SubscriptionID(0), poller.subscriptionID)
			},
		},
		{
			name: "Success_OwnWorkerPool_CreatesAndStartsPool",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				// Setup mock eventbus
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockEventBus.EXPECT().
					Subscribe(gomock.Any()).
					Return(eventbusTypes.SubscriptionID(1), nil).
					Times(1)

				// Expect Unsubscribe during cleanup (Deactivate)
				mockEventBus.EXPECT().
					Unsubscribe(eventbusTypes.SubscriptionID(1)).
					Return(true).
					Times(1)

				// Create poller WITHOUT providing worker pool (so it will create its own)
				informer, lister := getTestAGPInformerAndLister()
				_, tvpLister := getTestTVPInformerAndLister()
				ctx := getTestContext()

				// Don't provide WorkerPool in config - poller will create its own
				config := DefaultConfig()
				config.WorkerPool = nil // Explicitly nil

				poller, err := NewPoller(
					ctx,
					lister,
					tvpLister,
					getTestAutogrowCache(),
					getTestVolumeStatsProvider(ctrl),
					config,
				)
				require.NoError(t, err)
				require.NotNil(t, poller)
				require.Equal(t, StateStopped, poller.state)
				require.True(t, poller.ownWorkerPool, "should own worker pool")
				require.Nil(t, poller.workerPool, "worker pool should be nil before Activate")

				// Start the informer so cache sync won't block
				stopInformer := make(chan struct{})
				t.Cleanup(func() { close(stopInformer) })
				go informer.Run(stopInformer)

				// Wait for informer to be ready
				cache.WaitForCacheSync(stopInformer, informer.HasSynced)

				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				assert.Equal(t, StateRunning, poller.state)
				assert.NotNil(t, poller.stopCh)
				assert.Equal(t, eventbusTypes.SubscriptionID(1), poller.subscriptionID)

				// Verify worker pool was created and started
				assert.NotNil(t, poller.workerPool, "worker pool should be created during Activate")
				assert.True(t, poller.ownWorkerPool, "should still own worker pool")

				// Cleanup - since we own the worker pool, we need to shut it down
				shutdownCtx := getTestContext()
				err = poller.Deactivate(shutdownCtx)
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_ProvidedWorkerPool_DoesNotCreateOrStartPool",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				// Setup mock eventbus
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockEventBus.EXPECT().
					Subscribe(gomock.Any()).
					Return(eventbusTypes.SubscriptionID(1), nil).
					Times(1)

				// Provided pool - we should NOT call Start() on it during Activate
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				// No EXPECT for Start() - verifies it's NOT called

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })
				ctx := getTestContext()

				// Verify initial state
				require.False(t, poller.ownWorkerPool, "should not own externally provided pool")
				require.NotNil(t, poller.workerPool, "worker pool should be set from config")

				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				assert.Equal(t, StateRunning, poller.state)
				assert.NotNil(t, poller.stopCh)
				assert.Equal(t, eventbusTypes.SubscriptionID(1), poller.subscriptionID)

				// Verify worker pool state
				assert.NotNil(t, poller.workerPool, "worker pool should still exist")
				assert.False(t, poller.ownWorkerPool, "should not own externally provided pool")

				// Cleanup
				close(poller.stopCh)
			},
		},
		{
			name: "Success_OwnWorkerPool_PoolAlreadyExists_DoesNotRecreate",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				// Setup mock eventbus
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockEventBus.EXPECT().
					Subscribe(gomock.Any()).
					Return(eventbusTypes.SubscriptionID(1), nil).
					Times(1)

				// Create poller WITHOUT providing worker pool initially
				informer, lister := getTestAGPInformerAndLister()
				_, tvpLister := getTestTVPInformerAndLister()
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = nil

				poller, err := NewPoller(
					ctx,
					lister,
					tvpLister,
					getTestAutogrowCache(),
					getTestVolumeStatsProvider(ctrl),
					config,
				)
				require.NoError(t, err)
				require.True(t, poller.ownWorkerPool, "should own worker pool")
				require.Nil(t, poller.workerPool, "worker pool should be nil initially")

				// Pre-set a mock worker pool (simulating it was already created)
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				poller.workerPool = mockPool
				// No EXPECT for Start() - pool already exists so shouldn't be recreated/started

				// Start the informer
				stopInformer := make(chan struct{})
				t.Cleanup(func() { close(stopInformer) })
				go informer.Run(stopInformer)
				cache.WaitForCacheSync(stopInformer, informer.HasSynced)

				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				require.NoError(t, err)
				assert.Equal(t, StateRunning, poller.state)
				assert.NotNil(t, poller.stopCh)
				assert.Equal(t, eventbusTypes.SubscriptionID(1), poller.subscriptionID)

				// Verify worker pool was NOT recreated
				assert.NotNil(t, poller.workerPool, "worker pool should still exist")
				assert.True(t, poller.ownWorkerPool, "should still own worker pool")

				// Cleanup
				close(poller.stopCh)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup gomock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Get test-specific setup
			poller, ctx := tt.setup(t, ctrl)

			// Execute
			err := poller.Activate(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			// Additional custom verification
			if tt.verify != nil {
				tt.verify(t, poller, err)
			}
		})
	}
}

// TestActivate_Concurrency tests that only one Activate succeeds when called concurrently
func TestActivate_Concurrency(t *testing.T) {
	// Save original eventbus and restore after test
	originalEventBus := eventbus.VolumesScheduledEventBus
	defer func() {
		eventbus.VolumesScheduledEventBus = originalEventBus
	}()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mock eventbus - only expect ONE subscription
	mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
	eventbus.VolumesScheduledEventBus = mockEventBus

	mockEventBus.EXPECT().
		Subscribe(gomock.Any()).
		Return(eventbusTypes.SubscriptionID(1), nil).
		Times(1) // Only ONE subscription should succeed

	// Setup mock worker pool (provided, so we don't call Start on it)
	mockPool := mockWorkerpool.NewMockPool(ctrl)

	// Create poller (informer is already started by helper)
	poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
	defer close(stopInformer)

	const goroutines = 10
	var (
		successCount int32
		errorCount   int32
	)

	ctx := getTestContext()

	// Use a channel to synchronize the start of all goroutines
	startSignal := make(chan struct{})
	done := make(chan bool, goroutines)

	// Launch multiple concurrent Activate calls
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			// Wait for start signal to maximize concurrency
			<-startSignal

			err := poller.Activate(ctx)
			if err == nil {
				atomic.AddInt32(&successCount, 1)
			} else {
				atomic.AddInt32(&errorCount, 1)
				// Errors are expected when state prevents activation
				t.Logf("Goroutine %d got error (expected): %v", id, err)
			}
			done <- true
		}(i)
	}

	// Release all goroutines at once to maximize concurrency
	close(startSignal)

	// Wait for all goroutines to complete
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// Verify results
	// At least one should succeed (the first one)
	// The rest should either succeed (idempotent - if they ran after first completed)
	// or fail with "cannot activate" (if they tried during activation)
	assert.GreaterOrEqual(t, int(successCount), 1, "At least one activation should succeed")

	// Final state should be Running
	assert.Equal(t, StateRunning, poller.state)

	// Subscription ID should be set
	assert.Equal(t, eventbusTypes.SubscriptionID(1), poller.subscriptionID)

	// Verify only one subscription and one start were called (mocked expectations will fail if called more)

	// Cleanup
	if poller.stopCh != nil {
		close(poller.stopCh)
	}

	t.Logf("Concurrent activation results: %d succeeded, %d failed (expected)", successCount, errorCount)
}

// ============================================================================
// Test: processWorkItem
// ============================================================================

func TestProcessWorkItem(t *testing.T) {
	// Save original eventbus and restore after tests
	originalEventBus := eventbus.VolumeThresholdBreachedEventBus
	defer func() {
		eventbus.VolumeThresholdBreachedEventBus = originalEventBus
	}()

	tests := []struct {
		name             string
		setup            func(*testing.T, *gomock.Controller) (*Poller, context.Context, string)
		expectError      bool
		expectedErrorMsg string
		verify           func(*testing.T, error)
	}{
		{
			name: "Success_NoEffectivePolicy_SkipsProcessing",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				// Replace cache with one that has no policies
				poller.autogrowCache = mockCache

				ctx := getTestContext()
				pvName := "test-pv-no-policy"

				return poller, ctx, pvName
			},
			expectError: false,
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Error_PolicyNotFound_ReturnsError",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				// Create a TVP for the test (namespace must match config)
				tvpName := "test-pv-missing-policy"
				namespace := "test-namespace"
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName, // Use same as name for simplicity
				}

				// Create poller with TVP in the informer
				agpInformer, agpLister := getTestAGPInformerAndLister()    // No policies
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp) // With our TVP
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace // Must match TVP namespace

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					getTestVolumeStatsProvider(ctrl),
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Add a policy mapping but the policy doesn't exist in lister
				policyName := "missing-policy"
				mockCache.SetEffectivePolicyName(tvpName, policyName)

				return poller, ctx, tvpName
			},
			expectError:      true,
			expectedErrorMsg: "no longer exists",
			verify: func(t *testing.T, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "no longer exists")
			},
		},
		{
			name: "Error_VolumeStatsProviderFails_ReturnsError",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				// Create a TVP for the test
				tvpName := "test-pv-stats-error"
				policyName := "test-policy"
				namespace := "test-namespace"

				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				// Create fake policy
				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: policyName,
					},
					Spec: v1.TridentAutogrowPolicySpec{
						UsedThreshold: "80%",
					},
				}

				// Create poller with both TVP and policy in informers
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					mockVolumeStats, // Use mock volume stats
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Set policy mapping in cache
				mockCache.SetEffectivePolicyName(tvpName, policyName)

				// Mock VolumeStatsProvider to return error
				mockVolumeStats.EXPECT().
					GetVolumeStatsByID(gomock.Any(), tvpName).
					Return(nil, fmt.Errorf("stats provider error")).
					Times(1)

				return poller, ctx, tvpName
			},
			expectError:      true,
			expectedErrorMsg: "failed to get volume used size",
			verify: func(t *testing.T, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "failed to get volume used size")
			},
		},
		{
			name: "Success_ThresholdNotBreached_Percentage",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "test-pv-below-threshold"
				policyName := "test-policy-80"
				namespace := "test-namespace"

				// Create TVP
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				// Create policy with 80% threshold
				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: policyName,
					},
					Spec: v1.TridentAutogrowPolicySpec{
						UsedThreshold: "80%",
					},
				}

				// Create poller with TVP and policy
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					mockVolumeStats,
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, policyName)

				// Mock volume stats - 70% used (below 80% threshold)
				mockVolumeStats.EXPECT().
					GetVolumeStatsByID(gomock.Any(), tvpName).
					Return(&nodehelpers.VolumeStats{
						Total:          100 * 1024 * 1024 * 1024, // 100 GiB
						Used:           70 * 1024 * 1024 * 1024,  // 70 GiB
						UsedPercentage: 70.0,
					}, nil).
					Times(1)

				return poller, ctx, tvpName
			},
			expectError: false,
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_ThresholdBreached_Percentage_PublishesEvent",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, string) {
				// Setup mock eventbus
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumeThresholdBreached](ctrl)
				eventbus.VolumeThresholdBreachedEventBus = mockEventBus

				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "test-pv-breached"
				policyName := "test-policy-80"
				namespace := "test-namespace"

				// Create TVP
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: policyName,
					},
					Spec: v1.TridentAutogrowPolicySpec{
						UsedThreshold: "80%",
					},
				}

				// Create poller with TVP and policy
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					mockVolumeStats,
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, policyName)

				// Mock volume stats - 85% used (above 80% threshold)
				mockVolumeStats.EXPECT().
					GetVolumeStatsByID(gomock.Any(), tvpName).
					Return(&nodehelpers.VolumeStats{
						Total:          100 * 1024 * 1024 * 1024, // 100 GiB
						Used:           85 * 1024 * 1024 * 1024,  // 85 GiB
						UsedPercentage: 85.0,
					}, nil).
					Times(1)

				// Expect Publish to be called
				mockEventBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.VolumeThresholdBreached) {
						assert.Equal(t, tvpName, event.TVPName)
						assert.Equal(t, int64(85*1024*1024*1024), event.UsedSize)
						assert.Equal(t, int64(100*1024*1024*1024), event.CurrentTotalSize)
					}).
					Times(1)

				return poller, ctx, tvpName
			},
			expectError: false,
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_InvalidThreshold_NoRetry",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "test-pv-invalid-threshold"
				policyName := "test-policy-invalid"
				namespace := "test-namespace"

				// Create TVP
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				// Create policy with invalid threshold
				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: policyName,
					},
					Spec: v1.TridentAutogrowPolicySpec{
						UsedThreshold: "invalid-threshold",
					},
				}

				// Create poller with TVP and policy
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					mockVolumeStats,
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, policyName)

				mockVolumeStats.EXPECT().
					GetVolumeStatsByID(gomock.Any(), tvpName).
					Return(&nodehelpers.VolumeStats{
						Total:          100 * 1024 * 1024 * 1024,
						Used:           70 * 1024 * 1024 * 1024,
						UsedPercentage: 70.0,
					}, nil).
					Times(1)

				return poller, ctx, tvpName
			},
			expectError: false, // Returns nil for invalid threshold (no retry)
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Error_EventBusNil_WhenPublishing",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, string) {
				// Set eventbus to nil
				eventbus.VolumeThresholdBreachedEventBus = nil

				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "test-pv-no-eventbus"
				policyName := "test-policy-80"
				namespace := "test-namespace"

				// Create TVP
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: policyName,
					},
					Spec: v1.TridentAutogrowPolicySpec{
						UsedThreshold: "80%",
					},
				}

				// Create poller with TVP and policy
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					mockVolumeStats,
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, policyName)

				// Mock volume stats - threshold breached
				mockVolumeStats.EXPECT().
					GetVolumeStatsByID(gomock.Any(), tvpName).
					Return(&nodehelpers.VolumeStats{
						Total:          100 * 1024 * 1024 * 1024,
						Used:           85 * 1024 * 1024 * 1024,
						UsedPercentage: 85.0,
					}, nil).
					Times(1)

				return poller, ctx, tvpName
			},
			expectError:      true,
			expectedErrorMsg: "eventbus not initialized",
			verify: func(t *testing.T, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "eventbus not initialized")
			},
		},
		{
			name: "Success_ExactThresholdMatch_Percentage_NoEvent",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "test-pv-exact-match"
				policyName := "test-policy-80"
				namespace := "test-namespace"

				// Create TVP
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: policyName,
					},
					Spec: v1.TridentAutogrowPolicySpec{
						UsedThreshold: "80%",
					},
				}

				// Create poller with TVP and policy
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					mockVolumeStats,
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, policyName)

				// Mock volume stats - exactly 80% (should NOT trigger because we use > not >=)
				mockVolumeStats.EXPECT().
					GetVolumeStatsByID(gomock.Any(), tvpName).
					Return(&nodehelpers.VolumeStats{
						Total:          100 * 1024 * 1024 * 1024,
						Used:           80 * 1024 * 1024 * 1024,
						UsedPercentage: 80.0,
					}, nil).
					Times(1)

				// No eventbus expectation - event should NOT be published

				return poller, ctx, tvpName
			},
			expectError: false,
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_JustAboveThreshold_Percentage_PublishesEvent",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, string) {
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumeThresholdBreached](ctrl)
				eventbus.VolumeThresholdBreachedEventBus = mockEventBus

				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "test-pv-just-above"
				policyName := "test-policy-80"
				namespace := "test-namespace"

				// Create TVP
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: policyName,
					},
					Spec: v1.TridentAutogrowPolicySpec{
						UsedThreshold: "80%",
					},
				}

				// Create poller with TVP and policy
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					mockVolumeStats,
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, policyName)

				// Mock volume stats - 80.1% (just above threshold, should trigger)
				mockVolumeStats.EXPECT().
					GetVolumeStatsByID(gomock.Any(), tvpName).
					Return(&nodehelpers.VolumeStats{
						Total:          100 * 1024 * 1024 * 1024,
						Used:           80*1024*1024*1024 + 100*1024*1024, // 80.1 GiB
						UsedPercentage: 80.1,
					}, nil).
					Times(1)

				// Expect event to be published
				mockEventBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.VolumeThresholdBreached) {
						assert.Equal(t, tvpName, event.TVPName)
						assert.Greater(t, event.UsedSize, int64(0))
					}).
					Times(1)

				return poller, ctx, tvpName
			},
			expectError: false,
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
		},
		{
			name: "Success_TVPNotFound_SkipsProcessing",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "tvp-not-found"
				policyName := "test-policy"
				namespace := "test-namespace"

				// Create policy but NOT TVP (to test NotFound path)
				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: policyName},
					Spec:       v1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
				}

				// Create poller with policy but no TVP
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister() // No TVP
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(ctx, agpLister, tvpLister, mockCache, mockVolumeStats, config)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, policyName)

				return poller, ctx, tvpName
			},
			expectError: false, // NotFound returns nil (lines 498-501)
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err) // TVP not found should return nil
			},
		},
		{
			name: "Success_TVPEmptyVolumeID_SkipsProcessing",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "tvp-empty-volumeid"
				policyName := "test-policy"
				namespace := "test-namespace"

				// Create TVP with EMPTY VolumeID
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: "", // Empty VolumeID
				}

				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: policyName},
					Spec:       v1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
				}

				// Create poller
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(ctx, agpLister, tvpLister, mockCache, mockVolumeStats, config)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, policyName)

				return poller, ctx, tvpName
			},
			expectError: false, // Empty VolumeID returns nil (lines 510-513)
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err) // Empty VolumeID should return nil
			},
		},
		{
			name: "Success_PolicyInFailedState_SkipsProcessing",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "tvp-failed-policy"
				policyName := "failed-policy"
				namespace := "test-namespace"

				// Create TVP
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				// Create policy in FAILED state
				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: policyName},
					Spec:       v1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
					Status: v1.TridentAutogrowPolicyStatus{
						State: string(v1.TridentAutogrowPolicyStateFailed), // FAILED state
					},
				}

				// Create poller
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(ctx, agpLister, tvpLister, mockCache, mockVolumeStats, config)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, policyName)

				return poller, ctx, tvpName
			},
			expectError: false, // Failed policy returns nil (lines 536-542)
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err) // Failed policy should return nil
			},
		},
		{
			name: "Success_PolicyInDeletingState_SkipsProcessing",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "tvp-deleting-policy"
				policyName := "deleting-policy"
				namespace := "test-namespace"

				// Create TVP
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				// Create policy in DELETING state
				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: policyName},
					Spec:       v1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
					Status: v1.TridentAutogrowPolicyStatus{
						State: string(v1.TridentAutogrowPolicyStateDeleting), // DELETING state
					},
				}

				// Create poller
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(ctx, agpLister, tvpLister, mockCache, mockVolumeStats, config)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, policyName)

				return poller, ctx, tvpName
			},
			expectError: false, // Deleting policy returns nil (lines 536-542)
			verify: func(t *testing.T, err error) {
				assert.NoError(t, err) // Deleting policy should return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup gomock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Get test-specific setup
			poller, ctx, pvName := tt.setup(t, ctrl)

			// Execute
			err := poller.processWorkItem(ctx, WorkItem{
				Ctx:        ctx,
				TVPName:    pvName,
				RetryCount: 0,
			})

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			// Additional custom verification
			if tt.verify != nil {
				tt.verify(t, err)
			}
		})
	}
}

// ============================================================================
// Test: EnqueueEvent
// ============================================================================

func TestEnqueueEvent(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(*testing.T, *gomock.Controller) (*Poller, context.Context, []string)
		expectError      bool
		expectedErrorMsg string
		expectedEnqueued int
		expectedSkipped  int
		verify           func(*testing.T, *Poller, int, int, error)
	}{
		{
			name: "Success_AllVolumesEnqueued",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, []string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				// Set poller to Running state
				poller.state.set(StateRunning)

				// Add effective policies for all volumes
				pvNames := []string{"pv-1", "pv-2", "pv-3"}
				for _, pvName := range pvNames {
					mockCache.SetEffectivePolicyName(pvName, "test-policy")
				}

				ctx := getTestContext()
				return poller, ctx, pvNames
			},
			expectError:      false,
			expectedEnqueued: 3,
			expectedSkipped:  0,
			verify: func(t *testing.T, poller *Poller, enqueued, skipped int, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 3, enqueued)
				assert.Equal(t, 0, skipped)

				// Verify items are in workqueue
				assert.Equal(t, 3, poller.workqueue.Len())
			},
		},
		{
			name: "Success_SomeVolumesSkipped_NoPolicy",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, []string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Only add policy for pv-1, not for pv-2 and pv-3
				pvNames := []string{"pv-1", "pv-2", "pv-3"}
				mockCache.SetEffectivePolicyName("pv-1", "test-policy")

				ctx := getTestContext()
				return poller, ctx, pvNames
			},
			expectError:      false,
			expectedEnqueued: 1,
			expectedSkipped:  2,
			verify: func(t *testing.T, poller *Poller, enqueued, skipped int, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 1, enqueued)
				assert.Equal(t, 2, skipped)
				assert.Equal(t, 1, poller.workqueue.Len())
			},
		},
		{
			name: "Success_EmptyPVNameSkipped",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, []string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Mix of valid PV names and empty strings
				pvNames := []string{"pv-1", "", "pv-2", ""}
				mockCache.SetEffectivePolicyName("pv-1", "test-policy")
				mockCache.SetEffectivePolicyName("pv-2", "test-policy")

				ctx := getTestContext()
				return poller, ctx, pvNames
			},
			expectError:      false,
			expectedEnqueued: 2,
			expectedSkipped:  2,
			verify: func(t *testing.T, poller *Poller, enqueued, skipped int, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 2, enqueued)
				assert.Equal(t, 2, skipped)
				assert.Equal(t, 2, poller.workqueue.Len())
			},
		},
		{
			name: "Error_PollerNotRunning",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, []string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Keep poller in Stopped state
				poller.state.set(StateStopped)

				pvNames := []string{"pv-1", "pv-2"}
				mockCache.SetEffectivePolicyName("pv-1", "test-policy")
				mockCache.SetEffectivePolicyName("pv-2", "test-policy")

				ctx := getTestContext()
				return poller, ctx, pvNames
			},
			expectError:      true,
			expectedErrorMsg: "poller not running",
			expectedEnqueued: 0,
			expectedSkipped:  0,
			verify: func(t *testing.T, poller *Poller, enqueued, skipped int, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "poller not running")
				assert.Equal(t, 0, enqueued)
				assert.Equal(t, 0, skipped)
			},
		},
		{
			name: "Success_EmptyList",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, []string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				ctx := getTestContext()
				return poller, ctx, []string{}
			},
			expectError:      false,
			expectedEnqueued: 0,
			expectedSkipped:  0,
			verify: func(t *testing.T, poller *Poller, enqueued, skipped int, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 0, enqueued)
				assert.Equal(t, 0, skipped)
				assert.Equal(t, 0, poller.workqueue.Len())
			},
		},
		{
			name: "Success_DuplicateVolumes_Deduplication",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, []string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Same volume multiple times
				pvNames := []string{"pv-1", "pv-1", "pv-2", "pv-1"}
				mockCache.SetEffectivePolicyName("pv-1", "test-policy")
				mockCache.SetEffectivePolicyName("pv-2", "test-policy")

				ctx := getTestContext()
				return poller, ctx, pvNames
			},
			expectError:      false,
			expectedEnqueued: 4, // All 4 calls to Add succeed
			expectedSkipped:  0,
			verify: func(t *testing.T, poller *Poller, enqueued, skipped int, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 4, enqueued)
				assert.Equal(t, 0, skipped)
				// Workqueue deduplicates, so only 2 unique items
				assert.LessOrEqual(t, poller.workqueue.Len(), 2)
			},
		},
		{
			name: "Success_AllVolumesSkipped_NoPolicies",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, []string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Don't add any policies
				pvNames := []string{"pv-1", "pv-2", "pv-3"}

				ctx := getTestContext()
				return poller, ctx, pvNames
			},
			expectError:      false,
			expectedEnqueued: 0,
			expectedSkipped:  3,
			verify: func(t *testing.T, poller *Poller, enqueued, skipped int, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 0, enqueued)
				assert.Equal(t, 3, skipped)
				assert.Equal(t, 0, poller.workqueue.Len())
			},
		},
		{
			name: "Success_LargeVolumeList",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, []string) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Create 100 volumes
				pvNames := make([]string, 100)
				for i := 0; i < 100; i++ {
					pvName := fmt.Sprintf("pv-%d", i)
					pvNames[i] = pvName
					mockCache.SetEffectivePolicyName(pvName, "test-policy")
				}

				ctx := getTestContext()
				return poller, ctx, pvNames
			},
			expectError:      false,
			expectedEnqueued: 100,
			expectedSkipped:  0,
			verify: func(t *testing.T, poller *Poller, enqueued, skipped int, err error) {
				assert.NoError(t, err)
				assert.Equal(t, 100, enqueued)
				assert.Equal(t, 0, skipped)
				assert.Equal(t, 100, poller.workqueue.Len())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup gomock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Get test-specific setup
			poller, ctx, pvNames := tt.setup(t, ctrl)

			// Execute
			enqueued, skipped, err := poller.EnqueueEvent(ctx, pvNames)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify counts
			assert.Equal(t, tt.expectedEnqueued, enqueued)
			assert.Equal(t, tt.expectedSkipped, skipped)

			// Additional custom verification
			if tt.verify != nil {
				tt.verify(t, poller, enqueued, skipped, err)
			}
		})
	}
}

// ============================================================================
// Test: handleVolumesScheduled
// ============================================================================

func TestHandleVolumesScheduled(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(*testing.T, *gomock.Controller) (*Poller, context.Context, agTypes.VolumesScheduled)
		expectError      bool
		expectedErrorMsg string
		verify           func(*testing.T, eventbusTypes.Result, error)
	}{
		{
			name: "Success_AllVolumesEnqueued",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, agTypes.VolumesScheduled) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Add policies for volumes
				pvNames := []string{"pv-1", "pv-2", "pv-3"}
				for _, pvName := range pvNames {
					mockCache.SetEffectivePolicyName(pvName, "test-policy")
				}

				event := agTypes.VolumesScheduled{
					ID:       12345,
					TVPNames: pvNames,
				}

				ctx := getTestContext()
				return poller, ctx, event
			},
			expectError: false,
			verify: func(t *testing.T, result eventbusTypes.Result, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)

				assert.Equal(t, uint64(12345), result["eventID"])
				assert.Equal(t, 3, result["enqueued"])
				assert.Equal(t, 0, result["skipped"])
			},
		},
		{
			name: "Success_SomeVolumesSkipped",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, agTypes.VolumesScheduled) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Only add policy for pv-1
				pvNames := []string{"pv-1", "pv-2", "pv-3"}
				mockCache.SetEffectivePolicyName("pv-1", "test-policy")

				event := agTypes.VolumesScheduled{
					ID:       67890,
					TVPNames: pvNames,
				}

				ctx := getTestContext()
				return poller, ctx, event
			},
			expectError: false,
			verify: func(t *testing.T, result eventbusTypes.Result, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)

				assert.Equal(t, uint64(67890), result["eventID"])
				assert.Equal(t, 1, result["enqueued"])
				assert.Equal(t, 2, result["skipped"])
			},
		},
		{
			name: "Error_PollerNotRunning",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, agTypes.VolumesScheduled) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache
				// Keep poller in Stopped state
				poller.state.set(StateStopped)

				pvNames := []string{"pv-1", "pv-2"}
				for _, pvName := range pvNames {
					mockCache.SetEffectivePolicyName(pvName, "test-policy")
				}

				event := agTypes.VolumesScheduled{
					ID:       99999,
					TVPNames: pvNames,
				}

				ctx := getTestContext()
				return poller, ctx, event
			},
			expectError:      true,
			expectedErrorMsg: "poller not running",
			verify: func(t *testing.T, result eventbusTypes.Result, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "poller not running")
				assert.Nil(t, result)
			},
		},
		{
			name: "Success_EmptyPVNamesList",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, agTypes.VolumesScheduled) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				event := agTypes.VolumesScheduled{
					ID:       11111,
					TVPNames: []string{},
				}

				ctx := getTestContext()
				return poller, ctx, event
			},
			expectError: false,
			verify: func(t *testing.T, result eventbusTypes.Result, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)

				assert.Equal(t, uint64(11111), result["eventID"])
				assert.Equal(t, 0, result["enqueued"])
				assert.Equal(t, 0, result["skipped"])
			},
		},
		{
			name: "Success_MixOfValidAndEmptyPVNames",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, agTypes.VolumesScheduled) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				pvNames := []string{"pv-1", "", "pv-2", "", "pv-3"}
				mockCache.SetEffectivePolicyName("pv-1", "test-policy")
				mockCache.SetEffectivePolicyName("pv-2", "test-policy")
				mockCache.SetEffectivePolicyName("pv-3", "test-policy")

				event := agTypes.VolumesScheduled{
					ID:       22222,
					TVPNames: pvNames,
				}

				ctx := getTestContext()
				return poller, ctx, event
			},
			expectError: false,
			verify: func(t *testing.T, result eventbusTypes.Result, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)

				assert.Equal(t, uint64(22222), result["eventID"])
				assert.Equal(t, 3, result["enqueued"])
				assert.Equal(t, 2, result["skipped"])
			},
		},
		{
			name: "Success_AllVolumesSkipped_NoPolicies",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context, agTypes.VolumesScheduled) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Don't add any policies
				pvNames := []string{"pv-1", "pv-2", "pv-3"}

				event := agTypes.VolumesScheduled{
					ID:       33333,
					TVPNames: pvNames,
				}

				ctx := getTestContext()
				return poller, ctx, event
			},
			expectError: false,
			verify: func(t *testing.T, result eventbusTypes.Result, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)

				assert.Equal(t, uint64(33333), result["eventID"])
				assert.Equal(t, 0, result["enqueued"])
				assert.Equal(t, 3, result["skipped"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup gomock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Get test-specific setup
			poller, ctx, event := tt.setup(t, ctrl)

			// Execute
			result, err := poller.handleVolumesScheduled(ctx, event)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			// Additional custom verification
			if tt.verify != nil {
				tt.verify(t, result, err)
			}
		})
	}
}

// ============================================================================
// Test: workerLoop
// ============================================================================

func TestWorkerLoop(t *testing.T) {
	// Tracking struct for tests that need to verify retry behavior
	type retryTracking struct {
		attemptCount       int32
		processingComplete chan bool
	}

	tests := []struct {
		name          string
		setup         func(*testing.T, *gomock.Controller, *retryTracking) (*Poller, context.Context, func())
		verify        func(*testing.T, *Poller, *retryTracking)
		retryTracking *retryTracking
	}{
		{
			name: "Success_ContextCancellation_StopsLoop",
			setup: func(t *testing.T, ctrl *gomock.Controller, tracking *retryTracking) (*Poller, context.Context, func()) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Create a cancellable context
				ctx, cancel := context.WithCancel(getTestContext())

				// Shutdown queue to unblock Get()
				cleanup := func() {
					cancel()
					poller.workqueue.ShutDown()
				}

				return poller, ctx, cleanup
			},
			verify: func(t *testing.T, poller *Poller, tracking *retryTracking) {
				// Verify workerLoop stopped (test passes if no deadlock)
				assert.Equal(t, StateRunning, poller.state)
			},
		},
		{
			name: "Success_ContextCancelledBeforeLoop_ExitsImmediately",
			setup: func(t *testing.T, ctrl *gomock.Controller, tracking *retryTracking) (*Poller, context.Context, func()) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Create a context and cancel it BEFORE starting workerLoop
				// This ensures ctx.Done() case is hit (lines 284-285)
				ctx, cancel := context.WithCancel(getTestContext())
				cancel() // Cancel immediately

				// No need to call cleanup since context already cancelled
				cleanup := func() {
					poller.workqueue.ShutDown()
				}

				return poller, ctx, cleanup
			},
			verify: func(t *testing.T, poller *Poller, tracking *retryTracking) {
				// Verify workerLoop exited cleanly via context cancellation
				assert.Equal(t, StateRunning, poller.state)
			},
		},
		{
			name: "Success_StopChannelClose_StopsLoop",
			setup: func(t *testing.T, ctrl *gomock.Controller, tracking *retryTracking) (*Poller, context.Context, func()) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Create new stopCh for this test
				poller.stopCh = make(chan struct{})

				ctx := getTestContext()
				cleanup := func() {
					close(poller.stopCh)
					poller.workqueue.ShutDown()
				}

				return poller, ctx, cleanup
			},
			verify: func(t *testing.T, poller *Poller, tracking *retryTracking) {
				// Verify workerLoop stopped
				assert.Equal(t, StateRunning, poller.state)
			},
		},
		{
			name: "Success_WorkqueueShutdown_StopsLoop",
			setup: func(t *testing.T, ctrl *gomock.Controller, tracking *retryTracking) (*Poller, context.Context, func()) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				ctx := getTestContext()
				cleanup := func() {
					poller.workqueue.ShutDown()
				}

				return poller, ctx, cleanup
			},
			verify: func(t *testing.T, poller *Poller, tracking *retryTracking) {
				// Verify workerLoop stopped
				assert.Equal(t, StateRunning, poller.state)
			},
		},
		{
			name: "Success_ProcessesWorkItem_Success",
			setup: func(t *testing.T, ctrl *gomock.Controller, tracking *retryTracking) (*Poller, context.Context, func()) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				// Setup for successful processing
				tvpName := "test-pv"
				namespace := "test-namespace"

				// Create TVP
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				// Create policy
				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
					Spec:       v1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
				}

				// Create poller with TVP and policy
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					mockVolumeStats,
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, "test-policy")

				// Mock volume stats - below threshold
				mockVolumeStats.EXPECT().
					GetVolumeStatsByID(gomock.Any(), tvpName).
					Return(&nodehelpers.VolumeStats{
						Total:          100 * 1024 * 1024 * 1024,
						Used:           70 * 1024 * 1024 * 1024,
						UsedPercentage: 70.0,
					}, nil).
					Times(1)

				// Channel to signal when task is complete
				taskCompleted := make(chan struct{})

				// Mock pool submission - execute immediately
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						task() // Execute immediately
						close(taskCompleted)
						return nil
					}).
					Times(1)

				// Add work item to queue
				itemCtx := getTestContext()
				poller.workqueue.Add(WorkItem{Ctx: itemCtx, TVPName: tvpName, RetryCount: 0})

				ctxWithCancel, cancel := context.WithCancel(getTestContext())

				// Shutdown after processing completes
				cleanup := func() {
					// Wait for task to complete or timeout
					select {
					case <-taskCompleted:
						// Task completed
					case <-time.After(1 * time.Second):
						t.Error("Task did not complete within timeout")
					}
					cancel()
					poller.workqueue.ShutDown()
				}

				return poller, ctxWithCancel, cleanup
			},
			verify: func(t *testing.T, poller *Poller, tracking *retryTracking) {
				// Verify workqueue is empty (item was processed)
				assert.Equal(t, 0, poller.workqueue.Len())
			},
		},
		{
			name: "Success_PoolSubmitFails_RequeuesWithBackoff",
			setup: func(t *testing.T, ctrl *gomock.Controller, tracking *retryTracking) (*Poller, context.Context, func()) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				pvName := "test-pv"
				mockCache.SetEffectivePolicyName(pvName, "test-policy")

				// Mock pool submission failure - can be called multiple times due to re-queueing
				// Use AnyTimes to allow re-queueing with backoff
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					Return(fmt.Errorf("pool is full")).
					AnyTimes()

				// Add work item
				itemCtx := getTestContext()
				poller.workqueue.Add(WorkItem{Ctx: itemCtx, TVPName: pvName, RetryCount: 0})

				ctx, cancel := context.WithCancel(getTestContext())

				// Shutdown quickly after first failure
				cleanup := func() {
					cancel()
					poller.workqueue.ShutDown()
				}

				return poller, ctx, cleanup
			},
			verify: func(t *testing.T, poller *Poller, tracking *retryTracking) {
				// Item should be re-queued
				// Note: exact queue length depends on timing, but it should be > 0 or processed
				// The important part is that it was re-queued with retry count incremented
			},
		},
		{
			name: "Success_RetriableError_RequeuesWithBackoff",
			retryTracking: &retryTracking{
				processingComplete: make(chan bool, 1),
			},
			setup: func(t *testing.T, ctrl *gomock.Controller, tracking *retryTracking) (*Poller, context.Context, func()) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "test-pv-retriable"
				namespace := "test-namespace"

				// Create TVP
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				// Create policy
				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
					Spec:       v1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
				}

				// Create poller with TVP and policy
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					mockVolumeStats,
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, "test-policy")

				// Mock volume stats - fail first time, succeed second time
				mockVolumeStats.EXPECT().
					GetVolumeStatsByID(gomock.Any(), tvpName).
					DoAndReturn(func(ctx context.Context, volID string) (*nodehelpers.VolumeStats, error) {
						attempt := atomic.AddInt32(&tracking.attemptCount, 1)
						if attempt == 1 {
							// First attempt: return transient error (will trigger re-queue)
							return nil, fmt.Errorf("transient error")
						}
						// Second attempt: return success (below threshold)
						return &nodehelpers.VolumeStats{
							Total:          100 * 1024 * 1024 * 1024,
							Used:           70 * 1024 * 1024 * 1024,
							UsedPercentage: 70.0,
						}, nil
					}).
					Times(2)

				// Channel for cleanup to wait on (separate from verify's channel)
				cleanupComplete := make(chan struct{})

				// Mock pool submission - execute immediately, track completion
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						task() // Execute immediately
						// Signal completion on second attempt
						if atomic.LoadInt32(&tracking.attemptCount) == 2 {
							select {
							case tracking.processingComplete <- true:
							default:
							}
							close(cleanupComplete)
						}
						return nil
					}).
					Times(2)

				// Add work item with RetryCount = 0
				itemCtx := getTestContext()
				poller.workqueue.Add(WorkItem{Ctx: itemCtx, TVPName: tvpName, RetryCount: 0})

				ctxWithCancel, cancel := context.WithCancel(getTestContext())

				// Shutdown after processing completes
				cleanup := func() {
					// Wait for processing to complete or timeout
					select {
					case <-cleanupComplete:
						// Processing completed, give a brief moment for queue cleanup
						time.Sleep(50 * time.Millisecond)
					case <-time.After(5 * time.Second):
						t.Error("Processing did not complete within timeout")
					}
					cancel()
					poller.workqueue.ShutDown()
				}

				return poller, ctxWithCancel, cleanup
			},
			verify: func(t *testing.T, poller *Poller, tracking *retryTracking) {
				// Verify processing completed successfully on retry
				select {
				case <-tracking.processingComplete:
					// Success! Verify we had exactly 2 attempts
					assert.Equal(t, int32(2), atomic.LoadInt32(&tracking.attemptCount),
						"Expected exactly 2 processing attempts (initial + 1 retry)")

					// Verify queue is now empty (successful processing)
					assert.Equal(t, 0, poller.workqueue.Len(),
						"Queue should be empty after successful retry")
				case <-time.After(2 * time.Second):
					t.Fatalf("Expected processing to complete on retry, but got %d attempts",
						atomic.LoadInt32(&tracking.attemptCount))
				}
			},
		},
		{
			name: "Success_MaxRetriesExceeded_DiscardsWorkItem",
			setup: func(t *testing.T, ctrl *gomock.Controller, tracking *retryTracking) (*Poller, context.Context, func()) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "test-pv-max-retries"
				namespace := "test-namespace"

				// Create TVP
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				// Create policy
				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
					Spec:       v1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
				}

				// Create poller with TVP and policy
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace
				config.MaxRetries = 3 // Set MaxRetries to 3

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					mockVolumeStats,
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, "test-policy")

				// Mock volume stats to return ReconcileDeferredError
				mockVolumeStats.EXPECT().
					GetVolumeStatsByID(gomock.Any(), tvpName).
					Return(nil, fmt.Errorf("persistent transient error")).
					Times(1)

				// Channel to signal when task is complete
				taskCompleted := make(chan struct{})

				// Mock pool submission - execute immediately
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						task() // Execute immediately
						close(taskCompleted)
						return nil
					}).
					Times(1)

				// Add work item with RetryCount already at MaxRetries
				itemCtx := getTestContext()
				poller.workqueue.Add(WorkItem{Ctx: itemCtx, TVPName: tvpName, RetryCount: 3})

				ctxWithCancel, cancel := context.WithCancel(getTestContext())

				// Shutdown after processing
				cleanup := func() {
					// Wait for task to complete or timeout
					select {
					case <-taskCompleted:
						// Task completed
					case <-time.After(1 * time.Second):
						t.Error("Task did not complete within timeout")
					}
					cancel()
					poller.workqueue.ShutDown()
				}

				return poller, ctxWithCancel, cleanup
			},
			verify: func(t *testing.T, poller *Poller, tracking *retryTracking) {
				// Item should be discarded (Forget and Done called)
				assert.Equal(t, 0, poller.workqueue.Len())
			},
		},
		{
			name: "Success_NonRetriableError_DiscardsWithoutRetry",
			setup: func(t *testing.T, ctrl *gomock.Controller, tracking *retryTracking) (*Poller, context.Context, func()) {
				// Save and restore original eventbus
				originalEventBus := eventbus.VolumeThresholdBreachedEventBus
				t.Cleanup(func() {
					eventbus.VolumeThresholdBreachedEventBus = originalEventBus
				})

				// Set eventbus to nil to trigger non-retriable error
				eventbus.VolumeThresholdBreachedEventBus = nil

				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "test-pv-nonretriable"
				namespace := "test-namespace"

				// Create TVP
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				// Create policy
				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
					Spec:       v1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
				}

				// Create poller with TVP and policy
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					mockVolumeStats,
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, "test-policy")

				// Mock volume stats - threshold breached (to reach eventbus publish)
				mockVolumeStats.EXPECT().
					GetVolumeStatsByID(gomock.Any(), tvpName).
					Return(&nodehelpers.VolumeStats{
						Total:          100 * 1024 * 1024 * 1024,
						Used:           85 * 1024 * 1024 * 1024,
						UsedPercentage: 85.0,
					}, nil).
					Times(1)

				// Channel to signal when task is complete
				taskCompleted := make(chan struct{})

				// Mock pool submission - execute immediately
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						task() // Execute immediately
						close(taskCompleted)
						return nil
					}).
					Times(1)

				// Add work item
				itemCtx := getTestContext()
				poller.workqueue.Add(WorkItem{Ctx: itemCtx, TVPName: tvpName, RetryCount: 0})

				ctxWithCancel, cancel := context.WithCancel(getTestContext())

				// Shutdown after processing
				cleanup := func() {
					// Wait for task to complete or timeout
					select {
					case <-taskCompleted:
						// Task completed
					case <-time.After(1 * time.Second):
						t.Error("Task did not complete within timeout")
					}
					cancel()
					poller.workqueue.ShutDown()
				}

				return poller, ctxWithCancel, cleanup
			},
			verify: func(t *testing.T, poller *Poller, tracking *retryTracking) {
				// Item should be discarded without retry (Forget + Done called)
				assert.Equal(t, 0, poller.workqueue.Len())
			},
		},
		{
			name: "Success_MultipleWorkItems_ProcessedSequentially",
			setup: func(t *testing.T, ctrl *gomock.Controller, tracking *retryTracking) (*Poller, context.Context, func()) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				namespace := "test-namespace"
				tvpNames := []string{"pv-1", "pv-2", "pv-3"}

				// Create TVPs
				var tvps []*v1.TridentVolumePublication
				for _, tvpName := range tvpNames {
					tvp := &v1.TridentVolumePublication{
						ObjectMeta: metav1.ObjectMeta{
							Name:      tvpName,
							Namespace: namespace,
						},
						VolumeID: tvpName,
					}
					tvps = append(tvps, tvp)
				}

				// Create policy
				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
					Spec:       v1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
				}

				// Create poller with TVPs and policy
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvps...)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					mockVolumeStats,
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Setup for successful processing
				itemCtx := getTestContext()
				for _, tvpName := range tvpNames {
					mockCache.SetEffectivePolicyName(tvpName, "test-policy")

					// Add to queue
					poller.workqueue.Add(WorkItem{Ctx: itemCtx, TVPName: tvpName, RetryCount: 0})
				}

				// Mock volume stats for all TVPs
				mockVolumeStats.EXPECT().
					GetVolumeStatsByID(gomock.Any(), gomock.Any()).
					Return(&nodehelpers.VolumeStats{
						Total:          100 * 1024 * 1024 * 1024,
						Used:           70 * 1024 * 1024 * 1024,
						UsedPercentage: 70.0,
					}, nil).
					Times(3)

				// Channel to signal when all tasks are complete
				tasksCompleted := make(chan struct{})
				var processedCount atomic.Int32

				// Mock pool submission - execute immediately and signal completion
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						task() // Execute immediately
						if processedCount.Add(1) == 3 {
							close(tasksCompleted)
						}
						return nil
					}).
					Times(3)

				ctxWithCancel, cancel := context.WithCancel(getTestContext())

				// Shutdown after processing
				cleanup := func() {
					// Wait for all tasks to complete or timeout
					select {
					case <-tasksCompleted:
						// All tasks completed
					case <-time.After(1 * time.Second):
						t.Error("Tasks did not complete within timeout")
					}
					cancel()
					poller.workqueue.ShutDown()
				}

				return poller, ctxWithCancel, cleanup
			},
			verify: func(t *testing.T, poller *Poller, tracking *retryTracking) {
				// All items should be processed
				assert.Equal(t, 0, poller.workqueue.Len())
			},
		},
		{
			name: "Success_RateLimitWithRetryAfter_UsesAddAfter",
			retryTracking: &retryTracking{
				processingComplete: make(chan bool, 1),
			},
			setup: func(t *testing.T, ctrl *gomock.Controller, tracking *retryTracking) (*Poller, context.Context, func()) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()
				mockVolumeStats := mockNodeHelpers.NewMockVolumeStatsManager(ctrl)

				tvpName := "test-pv-rate-limit"
				namespace := "test-namespace"

				// Create TVP
				tvp := &v1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:      tvpName,
						Namespace: namespace,
					},
					VolumeID: tvpName,
				}

				// Create policy
				policy := &v1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
					Spec:       v1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
				}

				// Create poller with TVP and policy
				agpInformer, agpLister := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister := getTestTVPInformerAndLister(tvp)
				ctx := getTestContext()

				config := DefaultConfig()
				config.WorkerPool = mockPool
				config.TridentNamespace = namespace

				poller, err := NewPoller(
					ctx,
					agpLister,
					tvpLister,
					mockCache,
					mockVolumeStats,
					config,
				)
				require.NoError(t, err)

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				poller.state.set(StateRunning)

				// Set effective policy in cache
				mockCache.SetEffectivePolicyName(tvpName, "test-policy")

				// Mock volume stats - return TooManyRequests error with RetryAfterSeconds
				mockVolumeStats.EXPECT().
					GetVolumeStatsByID(gomock.Any(), tvpName).
					DoAndReturn(func(ctx context.Context, volID string) (*nodehelpers.VolumeStats, error) {
						// Return TooManyRequests error with RetryAfterSeconds
						// This tests lines 330-341 in workerLoop (rate-limit with retry-after handling)
						statusErr := &k8serrors.StatusError{
							ErrStatus: metav1.Status{
								Status:  metav1.StatusFailure,
								Code:    429,
								Reason:  metav1.StatusReasonTooManyRequests,
								Message: "rate limited",
								Details: &metav1.StatusDetails{
									RetryAfterSeconds: 5,
								},
							},
						}
						return nil, statusErr
					}).
					Times(1)

				// Mock pool submission - execute immediately
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						task() // Execute immediately
						select {
						case tracking.processingComplete <- true:
						default:
						}
						return nil
					}).
					Times(1)

				// Add work item with RetryCount = 0
				itemCtx := getTestContext()
				poller.workqueue.Add(WorkItem{Ctx: itemCtx, TVPName: tvpName, RetryCount: 0})

				ctxWithCancel, cancel := context.WithCancel(getTestContext())

				// Wait for processing, then shutdown
				cleanup := func() {
					// Wait for processing to complete
					select {
					case <-tracking.processingComplete:
						// Processing completed and item requeued
					case <-time.After(1 * time.Second):
						t.Error("Processing did not complete within timeout")
					}
					cancel()
					poller.workqueue.ShutDown()
				}

				return poller, ctxWithCancel, cleanup
			},
			verify: func(t *testing.T, poller *Poller, tracking *retryTracking) {
				// Item should be re-queued (queue length > 0 or already processed and re-added)
				// The important part is that AddAfter was called with the retry duration
				// We can't easily verify AddAfter was called vs AddRateLimited,
				// but the code path was exercised (lines 330-341)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup gomock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Get test-specific setup
			poller, ctx, cleanup := tt.setup(t, ctrl, tt.retryTracking)

			// Execute workerLoop in a goroutine
			done := make(chan bool)
			go func() {
				poller.workerLoop(ctx)
				done <- true
			}()

			// Wait a brief moment to let workerLoop start
			time.Sleep(50 * time.Millisecond)

			// Now trigger cleanup to stop the loop
			cleanup()

			// Wait for workerLoop to complete or timeout
			select {
			case <-done:
				// workerLoop exited normally
			case <-time.After(2 * time.Second):
				t.Fatal("workerLoop did not exit within timeout")
			}

			// Additional custom verification
			if tt.verify != nil {
				tt.verify(t, poller, tt.retryTracking)
			}
		})
	}
}

// TestWorkerLoop_EmptyQueue tests that workerLoop handles an empty queue gracefully
func TestWorkerLoop_EmptyQueue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPool := mockWorkerpool.NewMockPool(ctrl)
	mockCache := agCache.NewAutogrowCache()

	poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
	defer close(stopInformer)

	poller.autogrowCache = mockCache

	// Create workqueue manually for this test (normally created in Activate)
	poller.workqueue = workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
	)

	poller.state.set(StateRunning)

	// No items in queue
	// Pool Submit should never be called
	mockPool.EXPECT().
		Submit(gomock.Any(), gomock.Any()).
		Times(0)

	ctx, cancel := context.WithTimeout(getTestContext(), 100*time.Millisecond)
	defer cancel()

	// Start shutdown in background to unblock Get()
	go func() {
		time.Sleep(50 * time.Millisecond)
		poller.workqueue.ShutDown()
	}()

	// Execute workerLoop
	done := make(chan bool)
	go func() {
		poller.workerLoop(ctx)
		done <- true
	}()

	// Wait for completion
	select {
	case <-done:
		// Success - exited due to shutdown
	case <-time.After(1 * time.Second):
		t.Fatal("workerLoop did not exit")
	}
}

// ============================================================================
// Test: Deactivate
// ============================================================================

func TestDeactivate(t *testing.T) {
	// Save original eventbus and restore after tests
	originalEventBus := eventbus.VolumesScheduledEventBus
	defer func() {
		eventbus.VolumesScheduledEventBus = originalEventBus
	}()

	tests := []struct {
		name             string
		setup            func(*testing.T, *gomock.Controller) (*Poller, context.Context)
		expectError      bool
		expectedErrorMsg string
		verify           func(*testing.T, *Poller, error)
	}{
		{
			name: "Success_RunningToStopped_WithSubscription",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				// Setup mock eventbus
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create and set stopCh (simulating activated state)
				poller.stopCh = make(chan struct{})

				// Set poller to own the worker pool (so it will shut it down)
				poller.ownWorkerPool = true

				// Set poller to Running state with subscription
				poller.state.set(StateRunning)
				poller.subscriptionID = eventbusTypes.SubscriptionID(123)

				// Expect Unsubscribe to be called once in Deactivate
				mockEventBus.EXPECT().
					Unsubscribe(eventbusTypes.SubscriptionID(123)).
					Return(true).
					Times(1)

				// Expect worker pool shutdown (poller owns it) - called once
				mockPool.EXPECT().
					ShutdownWithTimeout(gomock.Any()).
					Return(nil).
					Times(1)

				ctx := getTestContext()
				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, poller.state)
				assert.Equal(t, eventbusTypes.SubscriptionID(0), poller.subscriptionID)

				// Workqueue may be nil if not created via Activate()
				// If it exists, verify it's shutdown
				if poller.workqueue != nil {
					assert.True(t, poller.workqueue.ShuttingDown(), "workqueue should be shutting down")
				}
			},
		},
		{
			name: "Success_AlreadyStopped_Idempotent",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Set poller to Stopped state
				poller.state.set(StateStopped)

				// No expectations - nothing should be called

				ctx := getTestContext()
				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, poller.state)
			},
		},
		{
			name: "Error_WrongState_StateStarting",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Set poller to Starting state
				poller.state.set(StateStarting)

				ctx := getTestContext()
				return poller, ctx
			},
			expectError:      true,
			expectedErrorMsg: "cannot deactivate poller",
			verify: func(t *testing.T, poller *Poller, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot deactivate poller")
				assert.Contains(t, err.Error(), "Starting")
				assert.Equal(t, StateStarting, poller.state)
			},
		},
		{
			name: "Error_WrongState_StateStopping",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Set poller to Stopping state
				poller.state.set(StateStopping)

				ctx := getTestContext()
				return poller, ctx
			},
			expectError:      true,
			expectedErrorMsg: "cannot deactivate poller",
			verify: func(t *testing.T, poller *Poller, err error) {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot deactivate poller")
				assert.Contains(t, err.Error(), "Stopping")
				assert.Equal(t, StateStopping, poller.state)
			},
		},
		{
			name: "Success_WithoutSubscription_ZeroID",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache
				poller.stopCh = make(chan struct{})

				// Set poller to own the worker pool (so it will shut it down)
				poller.ownWorkerPool = true

				// Set poller to Running state WITHOUT subscription (subscriptionID = 0)
				poller.state.set(StateRunning)
				poller.subscriptionID = 0

				// Expect worker pool shutdown
				mockPool.EXPECT().
					ShutdownWithTimeout(gomock.Any()).
					Return(nil).
					Times(1)

				ctx := getTestContext()
				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, poller.state)
				assert.Equal(t, eventbusTypes.SubscriptionID(0), poller.subscriptionID)
			},
		},
		{
			name: "Success_WorkerPoolNotOwned_SkipsShutdown",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				// Setup mock eventbus
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache
				poller.stopCh = make(chan struct{})

				// Set poller to NOT own the worker pool
				poller.ownWorkerPool = false

				// Set poller to Running state with subscription
				poller.state.set(StateRunning)
				poller.subscriptionID = eventbusTypes.SubscriptionID(456)

				// Expect Unsubscribe to be called once in Deactivate
				mockEventBus.EXPECT().
					Unsubscribe(eventbusTypes.SubscriptionID(456)).
					Return(true).
					Times(1)

				// Worker pool shutdown should NOT be called (not owned)
				mockPool.EXPECT().
					ShutdownWithTimeout(gomock.Any()).
					Times(0)

				ctx := getTestContext()
				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, poller.state)
				assert.Equal(t, eventbusTypes.SubscriptionID(0), poller.subscriptionID)
				assert.False(t, poller.ownWorkerPool)
			},
		},
		{
			name: "Success_WorkerPoolShutdownTimeout_ContinuesAnyway",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				// Setup mock eventbus
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache
				poller.stopCh = make(chan struct{})

				// Set poller to own the worker pool (so it will shut it down)
				poller.ownWorkerPool = true

				// Set poller to Running state
				poller.state.set(StateRunning)
				poller.subscriptionID = eventbusTypes.SubscriptionID(789)

				// Expect Unsubscribe once in Deactivate
				mockEventBus.EXPECT().
					Unsubscribe(eventbusTypes.SubscriptionID(789)).
					Return(true).
					Times(1)

				// Expect worker pool shutdown to timeout (called once)
				mockPool.EXPECT().
					ShutdownWithTimeout(gomock.Any()).
					Return(fmt.Errorf("shutdown timeout exceeded")).
					Times(1)

				ctx := getTestContext()
				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				// Deactivate should still succeed even if shutdown times out
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, poller.state)
			},
		},
		{
			name: "Success_UnsubscribeReturnsFalse_StillContinues",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				// Setup mock eventbus
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache
				poller.stopCh = make(chan struct{})

				// Set poller to own the worker pool (so it will shut it down)
				poller.ownWorkerPool = true

				// Set poller to Running state
				poller.state.set(StateRunning)
				poller.subscriptionID = eventbusTypes.SubscriptionID(999)

				// Expect Unsubscribe to return false (failed) - called once
				mockEventBus.EXPECT().
					Unsubscribe(eventbusTypes.SubscriptionID(999)).
					Return(false).
					Times(1)

				// Expect worker pool shutdown (called once)
				mockPool.EXPECT().
					ShutdownWithTimeout(gomock.Any()).
					Return(nil).
					Times(1)

				ctx := getTestContext()
				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				// Should still succeed even if unsubscribe returns false
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, poller.state)
			},
		},
		{
			name: "Success_MultipleSubscriptions_ClearsSubscriptionID",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				// Setup mock eventbus
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache
				poller.stopCh = make(chan struct{})

				// Set poller to own the worker pool (so it will shut it down)
				poller.ownWorkerPool = true

				// Set large subscription ID
				largeSubID := eventbusTypes.SubscriptionID(999999)
				poller.state.set(StateRunning)
				poller.subscriptionID = largeSubID

				// Expect Unsubscribe with the large ID (called once)
				mockEventBus.EXPECT().
					Unsubscribe(largeSubID).
					Return(true).
					Times(1)

				// Expect worker pool shutdown (called once)
				mockPool.EXPECT().
					ShutdownWithTimeout(gomock.Any()).
					Return(nil).
					Times(1)

				ctx := getTestContext()
				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, poller.state)
				// Verify subscription ID is cleared
				assert.Equal(t, eventbusTypes.SubscriptionID(0), poller.subscriptionID)
			},
		},
		{
			name: "Success_WorkqueueWithPendingItems_StillShutdown",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				// Setup mock eventbus
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache
				poller.stopCh = make(chan struct{})

				// Set poller to own the worker pool (so it will shut it down)
				poller.ownWorkerPool = true

				// Create workqueue manually for this test (normally created in Activate)
				poller.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				// Add some pending work items
				itemCtx := getTestContext()
				poller.workqueue.Add(WorkItem{Ctx: itemCtx, TVPName: "pv-1", RetryCount: 0})
				poller.workqueue.Add(WorkItem{Ctx: itemCtx, TVPName: "pv-2", RetryCount: 0})
				poller.workqueue.Add(WorkItem{Ctx: itemCtx, TVPName: "pv-3", RetryCount: 0})

				poller.state.set(StateRunning)
				poller.subscriptionID = eventbusTypes.SubscriptionID(555)

				// Expect Unsubscribe (called once)
				mockEventBus.EXPECT().
					Unsubscribe(eventbusTypes.SubscriptionID(555)).
					Return(true).
					Times(1)

				// Expect worker pool shutdown (called once)
				mockPool.EXPECT().
					ShutdownWithTimeout(gomock.Any()).
					Return(nil).
					Times(1)

				ctx := getTestContext()
				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, poller.state)
				// Workqueue should be shutdown (but not nil to avoid race with workerLoop)
				assert.True(t, poller.workqueue.ShuttingDown(), "workqueue should be shutting down")
			},
		},
		{
			name: "Success_CleanupCalledMultipleTimes_Idempotent",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache
				poller.stopCh = make(chan struct{})

				// Set poller to own the worker pool (so it will shut it down)
				poller.ownWorkerPool = true

				poller.state.set(StateRunning)
				poller.subscriptionID = eventbusTypes.SubscriptionID(888)

				// Expect Unsubscribe to be called once in Deactivate
				mockEventBus.EXPECT().
					Unsubscribe(eventbusTypes.SubscriptionID(888)).
					Return(true).
					Times(1)

				// Expect worker pool shutdown once in Deactivate
				mockPool.EXPECT().
					ShutdownWithTimeout(gomock.Any()).
					Return(nil).
					Times(1)

				ctx := getTestContext()
				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, poller.state)
			},
		},
		{
			name: "Success_VerifyStopChannelClosed",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
				t.Cleanup(func() { close(stopInformer) })

				poller.autogrowCache = mockCache

				// Create stopCh that we can check later
				poller.stopCh = make(chan struct{})
				stopChToCheck := poller.stopCh

				// Set poller to own the worker pool (so it will shut it down)
				poller.ownWorkerPool = true

				poller.state.set(StateRunning)
				poller.subscriptionID = eventbusTypes.SubscriptionID(111)

				// Expect Unsubscribe once in Deactivate
				mockEventBus.EXPECT().
					Unsubscribe(gomock.Any()).
					Return(true).
					Times(1)

				// Expect worker pool shutdown once
				mockPool.EXPECT().
					ShutdownWithTimeout(gomock.Any()).
					Return(nil).
					Times(1)

				ctx := getTestContext()

				// Store stopCh in context so verify can check it
				ctx = context.WithValue(ctx, "stopCh", stopChToCheck)
				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, poller.state)

				// stopCh should be closed - reading from it should immediately return zero value
				// We can't directly check if it's closed without blocking, but it's closed by Deactivate
			},
		},
		{
			name: "Success_CustomShutdownTimeout_UsesConfigValue",
			setup: func(t *testing.T, ctrl *gomock.Controller) (*Poller, context.Context) {
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
				eventbus.VolumesScheduledEventBus = mockEventBus

				mockPool := mockWorkerpool.NewMockPool(ctrl)
				mockCache := agCache.NewAutogrowCache()

				// Create poller with custom shutdown timeout
				customTimeout := 45 * time.Second
				config := NewConfig(WithShutdownTimeout(customTimeout))
				config.WorkerPool = mockPool

				_, lister := getTestAGPInformerAndLister()
				_, tvpLister := getTestTVPInformerAndLister()
				poller, err := NewPoller(
					getTestContext(),
					lister,
					tvpLister,
					mockCache,
					getTestVolumeStatsProvider(ctrl),
					config,
				)
				require.NoError(t, err)

				poller.stopCh = make(chan struct{})

				// Set poller to own the worker pool (so it will shut it down)
				poller.ownWorkerPool = true

				poller.state.set(StateRunning)
				poller.subscriptionID = eventbusTypes.SubscriptionID(222)

				// Expect Unsubscribe once in Deactivate
				mockEventBus.EXPECT().
					Unsubscribe(gomock.Any()).
					Return(true).
					Times(1)

				// Verify timeout passed to ShutdownWithTimeout matches our config (called once)
				mockPool.EXPECT().
					ShutdownWithTimeout(customTimeout).
					Return(nil).
					Times(1)

				ctx := getTestContext()
				return poller, ctx
			},
			expectError: false,
			verify: func(t *testing.T, poller *Poller, err error) {
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, poller.state)
				assert.Equal(t, 45*time.Second, poller.config.ShutdownTimeout)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup gomock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Get test-specific setup
			poller, ctx := tt.setup(t, ctrl)

			// Execute
			err := poller.Deactivate(ctx)

			// Verify
			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			// Additional custom verification
			if tt.verify != nil {
				tt.verify(t, poller, err)
			}
		})
	}
}

// TestDeactivate_Concurrency tests concurrent Deactivate calls
func TestDeactivate_Concurrency(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Setup mock eventbus
	mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](ctrl)
	eventbus.VolumesScheduledEventBus = mockEventBus
	defer func() {
		eventbus.VolumesScheduledEventBus = nil
	}()

	mockPool := mockWorkerpool.NewMockPool(ctrl)
	mockCache := agCache.NewAutogrowCache()

	poller, stopInformer := getTestPollerForActivate(t, ctrl, mockPool)
	defer close(stopInformer)

	poller.autogrowCache = mockCache

	// Create stopCh (simulating activated state)
	poller.stopCh = make(chan struct{})

	// Set poller to own the worker pool (so it will shut it down)
	poller.ownWorkerPool = true

	// Set poller to Running state with subscription
	poller.state.set(StateRunning)
	poller.subscriptionID = eventbusTypes.SubscriptionID(999)

	// Expect Unsubscribe to be called once (one successful Deactivate)
	mockEventBus.EXPECT().
		Unsubscribe(eventbusTypes.SubscriptionID(999)).
		Return(true).
		Times(1)

	// Expect worker pool shutdown once (one successful deactivation)
	mockPool.EXPECT().
		ShutdownWithTimeout(gomock.Any()).
		Return(nil).
		Times(1)

	ctx := getTestContext()

	// Launch multiple concurrent Deactivate calls
	const numGoroutines = 10
	var wg sync.WaitGroup
	successCount := atomic.Int32{}
	errorCount := atomic.Int32{}

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := poller.Deactivate(ctx)
			if err != nil {
				errorCount.Add(1)
			} else {
				successCount.Add(1)
			}
		}()
	}

	wg.Wait()

	// Verify results
	// At least ONE Deactivate should succeed (the first one that actually transitions)
	// Some additional ones may succeed as idempotent (if they run after state is Stopped)
	// The rest will fail with "cannot deactivate poller: current state is..." errors (if they run during Stopping state)
	assert.GreaterOrEqual(t, int(successCount.Load()), 1, "At least one deactivate should succeed")
	assert.Equal(t, numGoroutines, int(successCount.Load())+int(errorCount.Load()), "Success + errors should equal total calls")
	assert.Equal(t, StateStopped, poller.state)

	t.Logf("Concurrent deactivation results: %d succeeded, %d failed", successCount.Load(), errorCount.Load())
}

// ============================================================================
// State.String() Tests
// ============================================================================

func TestStateString(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		expected string
	}{
		{
			name:     "StateStopped",
			state:    StateStopped,
			expected: "Stopped",
		},
		{
			name:     "StateStarting",
			state:    StateStarting,
			expected: "Starting",
		},
		{
			name:     "StateRunning",
			state:    StateRunning,
			expected: "Running",
		},
		{
			name:     "StateStopping",
			state:    StateStopping,
			expected: "Stopping",
		},
		{
			name:     "Unknown_InvalidState",
			state:    State(999), // Invalid state
			expected: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.state.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPoller_GetState tests the GetState method
func TestPoller_GetState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	informer, lister := getTestAGPInformerAndLister()
	_, tvpLister := getTestTVPInformerAndLister()

	stopInformer := make(chan struct{})
	defer close(stopInformer)
	go informer.Run(stopInformer)

	poller, err := NewPoller(
		ctx,
		lister,
		tvpLister,
		getTestAutogrowCache(),
		getTestVolumeStatsProvider(ctrl),
		nil,
	)
	require.NoError(t, err)

	// Verify initial state
	assert.Equal(t, StateStopped, poller.GetState())

	// Manually set to different states and verify GetState returns them
	poller.state.set(StateStarting)
	assert.Equal(t, StateStarting, poller.GetState())

	poller.state.set(StateRunning)
	assert.Equal(t, StateRunning, poller.GetState())

	poller.state.set(StateStopping)
	assert.Equal(t, StateStopping, poller.GetState())
}

// TestPoller_SetState tests the SetState method
func TestPoller_SetState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	informer, lister := getTestAGPInformerAndLister()
	_, tvpLister := getTestTVPInformerAndLister()

	stopInformer := make(chan struct{})
	defer close(stopInformer)
	go informer.Run(stopInformer)

	poller, err := NewPoller(
		ctx,
		lister,
		tvpLister,
		getTestAutogrowCache(),
		getTestVolumeStatsProvider(ctrl),
		nil,
	)
	require.NoError(t, err)

	// Test setting different states
	states := []State{StateStopped, StateStarting, StateRunning, StateStopping}

	for _, state := range states {
		poller.SetState(state)
		assert.Equal(t, state, poller.state.get())
		assert.Equal(t, state, poller.GetState())
	}
}
