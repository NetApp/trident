// Copyright 2026 NetApp, Inc. All Rights Reserved.

package requester

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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	agCache "github.com/netapp/trident/frontend/autogrow/cache"
	autogrowTypes "github.com/netapp/trident/frontend/autogrow/types"
	"github.com/netapp/trident/mocks/mock_pkg/mock_eventbus"
	"github.com/netapp/trident/mocks/mock_pkg/mock_workerpool"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	tridentinformers "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions"
	listerv1 "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	"github.com/netapp/trident/pkg/eventbus"
	eventbusTypes "github.com/netapp/trident/pkg/eventbus/types"
	"github.com/netapp/trident/pkg/workerpool"
	"github.com/netapp/trident/pkg/workerpool/ants"
	workerpooltypes "github.com/netapp/trident/pkg/workerpool/types"
	. "github.com/netapp/trident/utils/errors"
)

// ============================================================================
// Fake Listers for Error Injection (following scheduler pattern)
// ============================================================================

// fakeTVPLister returns predetermined TVPs or errors for testing
type fakeTVPLister struct {
	tvps []*v1.TridentVolumePublication
	err  error
}

func (f *fakeTVPLister) List(selector labels.Selector) ([]*v1.TridentVolumePublication, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tvps, nil
}

func (f *fakeTVPLister) TridentVolumePublications(namespace string) listerv1.TridentVolumePublicationNamespaceLister {
	return &fakeTVPNamespaceLister{tvps: f.tvps, err: f.err}
}

type fakeTVPNamespaceLister struct {
	tvps []*v1.TridentVolumePublication
	err  error
}

func (f *fakeTVPNamespaceLister) List(selector labels.Selector) ([]*v1.TridentVolumePublication, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tvps, nil
}

func (f *fakeTVPNamespaceLister) Get(name string) (*v1.TridentVolumePublication, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, tvp := range f.tvps {
		if tvp.Name == name {
			return tvp, nil
		}
	}
	return nil, k8serrors.NewNotFound(schema.GroupResource{Group: "trident.netapp.io", Resource: "tridentvolumepublications"}, name)
}

// fakeAGPLister returns predetermined policies or errors for testing
type fakeAGPLister struct {
	policies []*v1.TridentAutogrowPolicy
	err      error
}

func (f *fakeAGPLister) List(selector labels.Selector) ([]*v1.TridentAutogrowPolicy, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.policies, nil
}

func (f *fakeAGPLister) Get(name string) (*v1.TridentAutogrowPolicy, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, policy := range f.policies {
		if policy.Name == name {
			return policy, nil
		}
	}
	return nil, k8serrors.NewNotFound(schema.GroupResource{Group: "trident.netapp.io", Resource: "tridentautogrowpolicies"}, name)
}

// ============================================================================
// Helper Functions
// ============================================================================

func setupTestEnvironment(t *testing.T, policies ...*v1.TridentAutogrowPolicy) (
	*fake.Clientset,
	*agCache.AutogrowCache,
	tridentinformers.SharedInformerFactory,
	chan struct{},
) {
	return setupTestEnvironmentWithTVPs(t, nil, policies...)
}

func setupTestEnvironmentWithTVPs(
	t *testing.T,
	tvps []*v1.TridentVolumePublication,
	policies ...*v1.TridentAutogrowPolicy,
) (
	*fake.Clientset,
	*agCache.AutogrowCache,
	tridentinformers.SharedInformerFactory,
	chan struct{},
) {
	// Create objects for fake client
	objects := make([]runtime.Object, 0, len(policies)+len(tvps))
	for _, policy := range policies {
		objects = append(objects, policy)
	}
	for _, tvp := range tvps {
		objects = append(objects, tvp)
	}

	// Create fake clientset with objects
	tridentClient := fake.NewSimpleClientset(objects...)

	// Create informer factory
	informerFactory := tridentinformers.NewSharedInformerFactory(tridentClient, 0)
	agpInformer := informerFactory.Trident().V1().TridentAutogrowPolicies().Informer()
	tvpInformer := informerFactory.Trident().V1().TridentVolumePublications().Informer()

	// Create real autogrow cache
	autogrowCache := agCache.NewAutogrowCache()

	// Create and start stop channel
	stopCh := make(chan struct{})
	go informerFactory.Start(stopCh)
	informerFactory.WaitForCacheSync(stopCh)
	cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

	return tridentClient, autogrowCache, informerFactory, stopCh
}

func createTestPolicy(name, phase string) *v1.TridentAutogrowPolicy {
	return &v1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Generation: 1,
		},
		Spec: v1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			GrowthAmount:  "20%",
			MaxSize:       "100Gi",
		},
		Status: v1.TridentAutogrowPolicyStatus{
			State: phase,
		},
	}
}

func createTestTVP(tvpName, namespace, volumeID string) *v1.TridentVolumePublication {
	return &v1.TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tvpName,
			Namespace: namespace,
		},
		VolumeID: volumeID,
	}
}

func createTestEvent(tvpName string, currentSize, usedSize uint64, policyName ...string) autogrowTypes.VolumeThresholdBreached {
	// Use provided policy name, or empty string if not provided (to test fallback)
	tagpName := ""
	if len(policyName) > 0 {
		tagpName = policyName[0]
	}

	return autogrowTypes.VolumeThresholdBreached{
		Ctx:              context.Background(),
		ID:               1,
		TVPName:          tvpName,
		TAGPName:         tagpName,
		CurrentTotalSize: int64(currentSize),
		UsedSize:         int64(usedSize),
		UsedPercentage:   float32(usedSize) / float32(currentSize) * 100.0,
		Timestamp:        time.Now(),
	}
}

// getTestAGPInformerAndLister returns a test AGP informer and lister using fake clientset
func getTestAGPInformerAndLister(policies ...*v1.TridentAutogrowPolicy) (cache.SharedIndexInformer, listerv1.TridentAutogrowPolicyLister, *fake.Clientset) {
	// Create fake Trident CRD clientset with optional pre-populated policies
	var objects []runtime.Object
	for _, policy := range policies {
		objects = append(objects, policy)
	}
	crdClientset := fake.NewSimpleClientset(objects...)

	// Create informer factory with 0 resync period for testing
	crdInformerFactory := tridentinformers.NewSharedInformerFactory(crdClientset, time.Second*0)

	// Get AGP informer and lister
	agpInformer := crdInformerFactory.Trident().V1().TridentAutogrowPolicies()

	return agpInformer.Informer(), agpInformer.Lister(), crdClientset
}

// getTestTVPInformerAndLister returns a test TVP informer and lister using fake clientset
func getTestTVPInformerAndLister(tvps ...*v1.TridentVolumePublication) (cache.SharedIndexInformer, listerv1.TridentVolumePublicationLister, *fake.Clientset) {
	// Create fake Trident CRD clientset with optional pre-populated TVPs
	var objects []runtime.Object
	for _, tvp := range tvps {
		objects = append(objects, tvp)
	}
	crdClientset := fake.NewSimpleClientset(objects...)

	// Create informer factory with 0 resync period for testing
	crdInformerFactory := tridentinformers.NewSharedInformerFactory(crdClientset, time.Second*0)

	// Get TVP informer and lister
	tvpInformer := crdInformerFactory.Trident().V1().TridentVolumePublications()

	return tvpInformer.Informer(), tvpInformer.Lister(), crdClientset
}

// getTestInformersAndListers returns both AGP and TVP informers/listers from a single clientset
func getTestInformersAndListers(
	policies []*v1.TridentAutogrowPolicy,
	tvps []*v1.TridentVolumePublication,
) (cache.SharedIndexInformer, listerv1.TridentAutogrowPolicyLister, cache.SharedIndexInformer, listerv1.TridentVolumePublicationLister, *fake.Clientset) {
	// Create objects for fake client
	objects := make([]runtime.Object, 0, len(policies)+len(tvps))
	for _, policy := range policies {
		objects = append(objects, policy)
	}
	for _, tvp := range tvps {
		objects = append(objects, tvp)
	}

	crdClientset := fake.NewSimpleClientset(objects...)

	// Create informer factory with 0 resync period for testing
	crdInformerFactory := tridentinformers.NewSharedInformerFactory(crdClientset, time.Second*0)

	// Get AGP and TVP informers and listers
	agpInformer := crdInformerFactory.Trident().V1().TridentAutogrowPolicies()
	tvpInformer := crdInformerFactory.Trident().V1().TridentVolumePublications()

	return agpInformer.Informer(), agpInformer.Lister(), tvpInformer.Informer(), tvpInformer.Lister(), crdClientset
}

// Test applyConfigDefaults
func TestApplyConfigDefaults(t *testing.T) {
	tests := []struct {
		name           string
		inputConfig    *Config
		expectedConfig *Config
	}{
		{
			name: "All fields missing - apply defaults",
			inputConfig: &Config{
				ShutdownTimeout:  0,
				WorkQueueName:    "",
				TridentNamespace: "",
			},
			expectedConfig: &Config{
				ShutdownTimeout:  DefaultShutdownTimeout,
				WorkQueueName:    DefaultWorkQueueName,
				TridentNamespace: DefaultTridentNamespace,
				MaxRetries:       DefaultMaxRetries,
			},
		},
		{
			name: "Custom values provided - preserve them",
			inputConfig: &Config{
				ShutdownTimeout:  60 * time.Second,
				WorkQueueName:    "custom-queue",
				TridentNamespace: "custom-namespace",
				MaxRetries:       5,
			},
			expectedConfig: &Config{
				ShutdownTimeout:  60 * time.Second,
				WorkQueueName:    "custom-queue",
				TridentNamespace: "custom-namespace",
				MaxRetries:       5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			applyConfigDefaults(ctx, tt.inputConfig)

			assert.Equal(t, tt.expectedConfig.ShutdownTimeout, tt.inputConfig.ShutdownTimeout)
			assert.Equal(t, tt.expectedConfig.WorkQueueName, tt.inputConfig.WorkQueueName)
			assert.Equal(t, tt.expectedConfig.TridentNamespace, tt.inputConfig.TridentNamespace)
			assert.Equal(t, tt.expectedConfig.MaxRetries, tt.inputConfig.MaxRetries)
		})
	}
}

// Test NewRequester
func TestNewRequester(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		config        *Config
		expectError   bool
		expectOwnPool bool
	}{
		{
			name:          "Create requester with default config",
			config:        nil,
			expectError:   false,
			expectOwnPool: true,
		},
		{
			name: "Create requester with custom config",
			config: &Config{
				ShutdownTimeout:  45 * time.Second,
				WorkQueueName:    "test-queue",
				MaxRetries:       3,
				TridentNamespace: "test-namespace",
			},
			expectError:   false,
			expectOwnPool: true,
		},
		{
			name: "Create requester with provided worker pool",
			config: &Config{
				WorkerPool:       mock_workerpool.NewMockPool(ctrl),
				TridentNamespace: "test-namespace",
			},
			expectError:   false,
			expectOwnPool: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
			defer close(stopCh)

			agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
			tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

			requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache,
				tt.config,
			)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, requester)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, requester)
				assert.Equal(t, tt.expectOwnPool, requester.ownWorkerPool)
				// Workqueue is created in Activate(), not in NewRequester()
				assert.Nil(t, requester.workqueue, "workqueue should be nil after NewRequester (created in Activate)")
				// Worker pool is created in Activate(), not in NewRequester()
				if tt.expectOwnPool {
					assert.Nil(t, requester.workerPool, "worker pool should be nil when owned (created in Activate)")
				} else {
					assert.NotNil(t, requester.workerPool, "worker pool should not be nil when provided")
				}
				assert.Equal(t, StateStopped, requester.state)
			}
		})
	}
}

// Test Activate
func TestActivate(t *testing.T) {
	tests := []struct {
		name          string
		initialState  State
		setupMocks    func(*testing.T, *gomock.Controller, *Requester, *Config) // Setup BEFORE Activate()
		verifyResult  func(*testing.T, *Requester, error)                       // Verify AFTER Activate()
		expectError   bool
		expectedState State
	}{
		{
			name:          "Successful activation",
			initialState:  StateStopped,
			expectError:   false,
			expectedState: StateRunning,
			setupMocks: func(t *testing.T, ctrl *gomock.Controller, req *Requester, config *Config) {
				mockEventBus := mock_eventbus.NewMockEventBusMetricsOptions[autogrowTypes.VolumeThresholdBreached](ctrl)
				mockEventBus.EXPECT().Subscribe(gomock.Any(), gomock.Any()).Return(eventbusTypes.SubscriptionID(123), nil).Times(1)
				// Expect cleanup calls for successful activation (called during Deactivate in cleanup phase)
				mockEventBus.EXPECT().Unsubscribe(gomock.Any()).Return(true).Times(1)
				eventbus.VolumeThresholdBreachedEventBus = mockEventBus
			},
			verifyResult: func(t *testing.T, req *Requester, err error) {
				assert.NoError(t, err)
				assert.Equal(t, StateRunning, req.state.get())
				assert.NotEqual(t, eventbusTypes.SubscriptionID(0), req.subscriptionID,
					"subscription ID should be set after successful activation")
			},
		},
		{
			name:          "Already running - idempotent",
			initialState:  StateRunning,
			expectError:   false,
			expectedState: StateRunning,
			setupMocks: func(t *testing.T, ctrl *gomock.Controller, req *Requester, config *Config) {
				mockEventBus := mock_eventbus.NewMockEventBusMetricsOptions[autogrowTypes.VolumeThresholdBreached](ctrl)
				eventbus.VolumeThresholdBreachedEventBus = mockEventBus
				// No expectations - already running, won't call Subscribe or Start
			},
			verifyResult: func(t *testing.T, req *Requester, err error) {
				assert.NoError(t, err)
				assert.Equal(t, StateRunning, req.state.get())
			},
		},
		{
			name:          "Eventbus not initialized - cleanup should happen",
			initialState:  StateStopped,
			expectError:   true,
			expectedState: StateStopped,
			setupMocks: func(t *testing.T, ctrl *gomock.Controller, req *Requester, config *Config) {
				eventbus.VolumeThresholdBreachedEventBus = nil
			},
			verifyResult: func(t *testing.T, req *Requester, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "eventbus not initialized")

				// Verify cleanup happened
				assert.Equal(t, eventbusTypes.SubscriptionID(0), req.subscriptionID,
					"subscription ID should be 0 after eventbus init failure")

				req.latestEventsMu.RLock()
				numEvents := len(req.latestEvents)
				req.latestEventsMu.RUnlock()
				assert.Equal(t, 0, numEvents, "latestEvents should be empty after cleanup")
			},
		},
		{
			name:          "Subscribe to eventbus fails - cleanup should happen",
			initialState:  StateStopped,
			expectError:   true,
			expectedState: StateStopped,
			setupMocks: func(t *testing.T, ctrl *gomock.Controller, req *Requester, config *Config) {
				mockEventBus := mock_eventbus.NewMockEventBusMetricsOptions[autogrowTypes.VolumeThresholdBreached](ctrl)
				mockEventBus.EXPECT().Subscribe(gomock.Any(), gomock.Any()).
					Return(eventbusTypes.SubscriptionID(0), fmt.Errorf("subscribe error")).Times(1)
				eventbus.VolumeThresholdBreachedEventBus = mockEventBus
			},
			verifyResult: func(t *testing.T, req *Requester, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "subscribe")

				// Verify cleanup happened
				assert.Equal(t, eventbusTypes.SubscriptionID(0), req.subscriptionID,
					"subscription ID should be 0 after subscribe failure")

				req.latestEventsMu.RLock()
				numEvents := len(req.latestEvents)
				req.latestEventsMu.RUnlock()
				assert.Equal(t, 0, numEvents, "latestEvents should be empty after cleanup")
			},
		},
		{
			name:          "Activate from StateStarting - returns StateError",
			initialState:  StateStarting,
			expectError:   true,
			expectedState: StateStarting,
			setupMocks: func(t *testing.T, ctrl *gomock.Controller, req *Requester, config *Config) {
				// State is already set to Starting in the test setup
				// No mocks needed - should fail before attempting any operations
			},
			verifyResult: func(t *testing.T, req *Requester, err error) {
				assert.Error(t, err)
				// Should be a StateError
				assert.True(t, IsStateError(err), "expected StateError")
				assert.Contains(t, err.Error(), "cannot activate requester")
				assert.Contains(t, err.Error(), "Starting")
			},
		},
		{
			name:          "Activate from StateStopping - returns StateError",
			initialState:  StateStopping,
			expectError:   true,
			expectedState: StateStopping,
			setupMocks: func(t *testing.T, ctrl *gomock.Controller, req *Requester, config *Config) {
				// State is already set to Stopping in the test setup
				// No mocks needed - should fail before attempting any operations
			},
			verifyResult: func(t *testing.T, req *Requester, err error) {
				assert.Error(t, err)
				// Should be a StateError
				assert.True(t, IsStateError(err), "expected StateError")
				assert.Contains(t, err.Error(), "cannot activate requester")
				assert.Contains(t, err.Error(), "Stopping")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()

			tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
			defer close(stopCh)

			agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
			tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

			mockPool := mock_workerpool.NewMockPool(ctrl)

			config := &Config{
				WorkerPool:       mockPool,
				TridentNamespace: "default",
				MaxRetries:       5,
				ShutdownTimeout:  30 * time.Second,
			}

			requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, config)
			require.NoError(t, err)

			// Set initial state (needed for idempotent test case)
			requester.state.set(tt.initialState)

			// Phase 1: Setup mocks BEFORE activation
			tt.setupMocks(t, ctrl, requester, config)

			// Phase 2: Execute
			err = requester.Activate(ctx)

			// Phase 3: Verify results AFTER activation (test-specific)
			tt.verifyResult(t, requester, err)

			// Phase 4: Common final state check
			currentState := requester.state.get()
			assert.Equal(t, tt.expectedState, currentState)

			// Cleanup for successful activations (expectations were set in setupMocks)
			if !tt.expectError && tt.initialState != StateRunning {
				requester.Deactivate(ctx)
			}
		})
	}
}

// Test Activate with owned worker pool (covers lines 137-168 of requester.go)
func TestActivateWithOwnedWorkerPool(t *testing.T) {
	tests := []struct {
		name          string
		setupMocks    func(*testing.T, *gomock.Controller, *Requester)
		expectError   bool
		expectedState State
		verifyResult  func(*testing.T, *Requester, error)
	}{
		{
			name: "Successful activation with owned worker pool creation",
			setupMocks: func(t *testing.T, ctrl *gomock.Controller, req *Requester) {
				// Mock eventbus for successful subscription
				mockEventBus := mock_eventbus.NewMockEventBusMetricsOptions[autogrowTypes.VolumeThresholdBreached](ctrl)
				mockEventBus.EXPECT().Subscribe(gomock.Any(), gomock.Any()).Return(eventbusTypes.SubscriptionID(123), nil).Times(1)
				// Expect cleanup (Unsubscribe will be called during test cleanup via Deactivate)
				mockEventBus.EXPECT().Unsubscribe(gomock.Any()).Return(true).Times(1)
				eventbus.VolumeThresholdBreachedEventBus = mockEventBus
			},
			expectError:   false,
			expectedState: StateRunning,
			verifyResult: func(t *testing.T, req *Requester, err error) {
				assert.NoError(t, err)
				assert.Equal(t, StateRunning, req.state.get())

				// Verify worker pool was created and started
				assert.NotNil(t, req.workerPool, "worker pool should be created when owned")
				assert.True(t, req.ownWorkerPool, "ownWorkerPool should be true")

				// Verify subscription ID was set
				assert.NotEqual(t, eventbusTypes.SubscriptionID(0), req.subscriptionID,
					"subscription ID should be set after successful activation")
			},
		},
		{
			name: "Activation when pool already created (ownWorkerPool=true but pool exists) - creates fresh pool",
			setupMocks: func(t *testing.T, ctrl *gomock.Controller, req *Requester) {
				// Pre-create a mock pool and set it on the requester
				// This simulates a scenario where a pool somehow already exists
				mockPool := mock_workerpool.NewMockPool(ctrl)
				req.workerPool = mockPool
				req.ownWorkerPool = true // We own it, but it's already created

				// Mock eventbus for successful subscription
				mockEventBus := mock_eventbus.NewMockEventBusMetricsOptions[autogrowTypes.VolumeThresholdBreached](ctrl)
				mockEventBus.EXPECT().Subscribe(gomock.Any(), gomock.Any()).Return(eventbusTypes.SubscriptionID(456), nil).Times(1)
				mockEventBus.EXPECT().Unsubscribe(gomock.Any()).Return(true).Times(1)
				eventbus.VolumeThresholdBreachedEventBus = mockEventBus

				// With new behavior: Activate() ALWAYS creates a fresh pool if we own it
				// The old mock pool will be replaced by a new real pool
				// So the mock should NOT expect ShutdownWithTimeout (it gets replaced, not shut down)
				// The new pool will be shut down during Deactivate()
			},
			expectError:   false,
			expectedState: StateRunning,
			verifyResult: func(t *testing.T, req *Requester, err error) {
				assert.NoError(t, err)
				assert.Equal(t, StateRunning, req.state.get())

				// Verify a new worker pool was created (not the mock)
				assert.NotNil(t, req.workerPool, "worker pool should exist")
				assert.True(t, req.ownWorkerPool, "ownWorkerPool should be true")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx := context.Background()

			tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
			defer close(stopCh)

			agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
			tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

			// Create config WITHOUT providing a worker pool (this will cause ownWorkerPool=true)
			config := &Config{
				TridentNamespace: "default",
				MaxRetries:       5,
				ShutdownTimeout:  30 * time.Second,
			}

			requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, config)
			require.NoError(t, err)

			// Verify initial state
			assert.True(t, requester.ownWorkerPool, "ownWorkerPool should be true when no pool provided")
			assert.Nil(t, requester.workerPool, "workerPool should be nil initially")

			// Setup mocks
			tt.setupMocks(t, ctrl, requester)

			// Execute Activate
			err = requester.Activate(ctx)

			// Verify results
			tt.verifyResult(t, requester, err)

			// Verify final state
			currentState := requester.state.get()
			assert.Equal(t, tt.expectedState, currentState)

			// Cleanup if activation was successful
			if !tt.expectError && currentState == StateRunning {
				err := requester.Deactivate(ctx)
				assert.NoError(t, err)
			}
		})
	}
}

// Test IsRunning and CanAcceptWork
func TestIsRunning(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name     string
		state    State
		expected bool
	}{
		{"Stopped", StateStopped, false},
		{"Starting", StateStarting, false},
		{"Running", StateRunning, true},
		{"Stopping", StateStopping, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
			defer close(stopCh)

			agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
			tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

			requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, nil)
			require.NoError(t, err)

			requester.state.set(tt.state)

			result := requester.IsRunning()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCanAcceptWork(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name     string
		state    State
		expected bool
	}{
		{"Stopped", StateStopped, false},
		{"Starting", StateStarting, false},
		{"Running", StateRunning, true},
		{"Stopping", StateStopping, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
			defer close(stopCh)

			agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
			tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

			requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, nil)
			require.NoError(t, err)

			requester.state.set(tt.state)

			result := requester.IsRunning()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test EnqueueEvent
func TestEnqueueEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name          string
		event         autogrowTypes.VolumeThresholdBreached
		state         State
		expectError   bool
		errorContains string
	}{
		{
			name:        "Valid event - successfully enqueued",
			event:       createTestEvent("pv-test-1", 1000, 800),
			state:       StateRunning,
			expectError: false,
		},
		{
			name: "Invalid event - missing PV name",
			event: autogrowTypes.VolumeThresholdBreached{
				Ctx:              context.Background(),
				TVPName:          "",
				TAGPName:         "test-policy",
				CurrentTotalSize: 1000,
				UsedSize:         800,
			},
			state:         StateRunning,
			expectError:   true,
			errorContains: "invalid event",
		},
		{
			name: "Invalid event - zero used size",
			event: autogrowTypes.VolumeThresholdBreached{
				Ctx:              context.Background(),
				TVPName:          "pv-test-1",
				TAGPName:         "test-policy",
				CurrentTotalSize: 1000,
				UsedSize:         0,
			},
			state:         StateRunning,
			expectError:   true,
			errorContains: "invalid event",
		},
		{
			name:          "Requester not running",
			event:         createTestEvent("pv-test-1", 1000, 800),
			state:         StateStopped,
			expectError:   true,
			errorContains: "requester not running",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
			defer close(stopCh)

			agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
			tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

			requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, nil)
			require.NoError(t, err)

			// Create workqueue manually for this test (normally created in Activate)
			if tt.state == StateRunning {
				requester.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)
			}

			requester.state.set(tt.state)

			err = requester.EnqueueEvent(ctx, tt.event)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)

				// Verify event was stored in latestEvents
				requester.latestEventsMu.RLock()
				storedEvent, exists := requester.latestEvents[tt.event.TVPName]
				requester.latestEventsMu.RUnlock()

				assert.True(t, exists)
				assert.Equal(t, tt.event.TVPName, storedEvent.TVPName)
			}
		})
	}
}

// Test concurrent EnqueueEvent
func TestEnqueueEventConcurrent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
	defer close(stopCh)

	agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
	tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

	requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, nil)
	require.NoError(t, err)

	// Create workqueue manually for this test (normally created in Activate)
	requester.workqueue = workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
	)

	requester.state.set(StateRunning)

	// Enqueue multiple events concurrently
	const numGoroutines = 50
	const numEventsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < numEventsPerGoroutine; j++ {
				tvpName := fmt.Sprintf("pv-test-%d", goroutineID%10) // Use 10 different TVPs
				event := createTestEvent(tvpName, 1000+uint64(j), 800+uint64(j))

				err := requester.EnqueueEvent(ctx, event)
				assert.NoError(t, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify that we have events for the expected TVPs
	// Only the latest event for each PV should be stored
	requester.latestEventsMu.RLock()
	numEvents := len(requester.latestEvents)
	requester.latestEventsMu.RUnlock()

	assert.LessOrEqual(t, numEvents, 10, "Should have at most 10 TVPs in latestEvents")
}

// Test cleanupEvent
func TestCleanupEvent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
	defer close(stopCh)

	agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
	tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

	requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, nil)
	require.NoError(t, err)

	// Add some events
	requester.latestEventsMu.Lock()
	requester.latestEvents["pv-1"] = createTestEvent("pv-1", 1000, 800)
	requester.latestEvents["pv-2"] = createTestEvent("pv-2", 2000, 1600)
	requester.latestEventsMu.Unlock()

	// Cleanup one event
	requester.cleanupEvent("pv-1")

	// Verify
	requester.latestEventsMu.RLock()
	_, exists1 := requester.latestEvents["pv-1"]
	_, exists2 := requester.latestEvents["pv-2"]
	requester.latestEventsMu.RUnlock()

	assert.False(t, exists1, "pv-1 should be removed")
	assert.True(t, exists2, "pv-2 should still exist")
}

// Test cleanupEvent concurrent access
func TestCleanupEventConcurrent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
	defer close(stopCh)

	agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
	tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

	requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, nil)
	require.NoError(t, err)

	// Add many events
	const numEvents = 100
	for i := 0; i < numEvents; i++ {
		pvName := fmt.Sprintf("pv-%d", i)
		requester.latestEvents[pvName] = createTestEvent(pvName, 1000, 800)
	}

	// Cleanup events concurrently
	var wg sync.WaitGroup
	wg.Add(numEvents)

	for i := 0; i < numEvents; i++ {
		go func(id int) {
			defer wg.Done()
			pvName := fmt.Sprintf("pv-%d", id)
			requester.cleanupEvent(pvName)
		}(i)
	}

	wg.Wait()

	// Verify all events are cleaned up
	requester.latestEventsMu.RLock()
	numRemaining := len(requester.latestEvents)
	requester.latestEventsMu.RUnlock()

	assert.Equal(t, 0, numRemaining, "All events should be cleaned up")
}

// Test State.String()
func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateStopped, "Stopped"},
		{StateStarting, "Starting"},
		{StateRunning, "Running"},
		{StateStopping, "Stopping"},
		{State(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.state.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test processWorkItem
func TestProcessWorkItem(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name                 string
		pvName               string
		policyName           string // If set, policy will be created with this name
		policyPhase          string // Phase for the policy (Failed, Deleting, Accepted, etc.)
		setupFunc            func(*testing.T, *Requester, *fake.Clientset, *agCache.AutogrowCache)
		verifyResult         func(*testing.T, *Requester, *fake.Clientset, string) // Optional: verify side effects
		expectError          bool
		expectReconcileDefer bool
		errorContains        string
		// Fields for fake lister injection
		useFakeTVPLister bool
		useFakeAGPLister bool
		fakeTVPSetup     func(*testing.T, *Requester, *fake.Clientset, *agCache.AutogrowCache) listerv1.TridentVolumePublicationLister
		fakeAGPSetup     func(*testing.T, *Requester, *fake.Clientset, *agCache.AutogrowCache) listerv1.TridentAutogrowPolicyLister
	}{
		{
			name:   "No event found for volume",
			pvName: "pv-missing",
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				// Don't add event to latestEvents
			},
			expectError:   true,
			errorContains: "no event found",
		},
		{
			name:   "No effective policy found - event missing TAGPName and cache empty",
			pvName: "pv-test-1",
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				// TVP is created upfront in test setup
				req.latestEventsMu.Lock()
				// Event without TAGPName, and no policy in cache - should fail
				req.latestEvents["pv-test-1"] = createTestEvent("pv-test-1", 1000, 800)
				req.latestEventsMu.Unlock()
				// Don't set effective policy in cache
			},
			expectError:   true,
			errorContains: "no effective policy found",
		},
		{
			name:        "Fallback to cache - event missing TAGPName but cache has policy",
			pvName:      "pv-test-fallback",
			policyName:  "fallback-policy",
			policyPhase: string(v1.TridentAutogrowPolicyStateSuccess),
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				// TVP is created upfront in test setup
				req.latestEventsMu.Lock()
				// Event without TAGPName - should fallback to cache
				req.latestEvents["pv-test-fallback"] = createTestEvent("pv-test-fallback", 1000, 800)
				req.latestEventsMu.Unlock()

				// Set effective policy in cache (fallback will use this)
				err := cache.SetEffectivePolicyName("pv-test-fallback", "fallback-policy")
				require.NoError(t, err)
			},
			expectError: false, // Should succeed using cache fallback
			verifyResult: func(t *testing.T, req *Requester, client *fake.Clientset, pvName string) {
				// Verify TAGRI was created using the fallback policy
				tagriName := generateTAGRIName(pvName)
				tagri, err := client.TridentV1().TridentAutogrowRequestInternals(req.config.TridentNamespace).
					Get(context.Background(), tagriName, metav1.GetOptions{})
				require.NoError(t, err)
				require.NotNil(t, tagri)

				assert.Equal(t, pvName, tagri.Spec.Volume)
				assert.Equal(t, "fallback-policy", tagri.Spec.AutogrowPolicyRef.Name)
			},
		},
		{
			name:        "Policy in failed state",
			pvName:      "pv-test-1",
			policyName:  "failed-policy",
			policyPhase: string(v1.TridentAutogrowPolicyStateFailed),
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				// TVP is created upfront in test setup
				req.latestEventsMu.Lock()
				// Event with TAGPName set
				req.latestEvents["pv-test-1"] = createTestEvent("pv-test-1", 1000, 800, "failed-policy")
				req.latestEventsMu.Unlock()

				// Set effective policy in cache (for fallback testing)
				err := cache.SetEffectivePolicyName("pv-test-1", "failed-policy")
				require.NoError(t, err)
			},
			expectError: false, // Should return nil for failed policy
		},
		{
			name:        "Policy in deleting state",
			pvName:      "pv-test-deleting",
			policyName:  "deleting-policy",
			policyPhase: string(v1.TridentAutogrowPolicyStateDeleting),
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				// TVP is created upfront in test setup
				req.latestEventsMu.Lock()
				// Event with TAGPName set
				req.latestEvents["pv-test-deleting"] = createTestEvent("pv-test-deleting", 1000, 800, "deleting-policy")
				req.latestEventsMu.Unlock()

				// Set effective policy in cache (for fallback testing)
				err := cache.SetEffectivePolicyName("pv-test-deleting", "deleting-policy")
				require.NoError(t, err)
			},
			expectError: false, // Should return nil for deleting policy
		},
		{
			name:        "Successful TAGRI creation - happy path",
			pvName:      "pv-test-success",
			policyName:  "success-policy",
			policyPhase: string(v1.TridentAutogrowPolicyStateSuccess),
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				// TVP is created upfront in test setup
				req.latestEventsMu.Lock()
				// Event with TAGPName set
				req.latestEvents["pv-test-success"] = createTestEvent("pv-test-success", 1000, 800, "success-policy")
				req.latestEventsMu.Unlock()

				// Set effective policy in cache (for fallback testing)
				err := cache.SetEffectivePolicyName("pv-test-success", "success-policy")
				require.NoError(t, err)

				// Volume metadata no longer cached - not needed for test
			},
			expectError: false, // Should succeed
			verifyResult: func(t *testing.T, req *Requester, client *fake.Clientset, pvName string) {
				// Verify TAGRI was actually created in fake client
				tagriName := generateTAGRIName(pvName)
				tagri, err := client.TridentV1().TridentAutogrowRequestInternals(req.config.TridentNamespace).
					Get(context.Background(), tagriName, metav1.GetOptions{})
				require.NoError(t, err)
				require.NotNil(t, tagri)

				assert.Equal(t, pvName, tagri.Spec.Volume)
				assert.Equal(t, "success-policy", tagri.Spec.AutogrowPolicyRef.Name)
				assert.Equal(t, string(v1.TridentAutogrowRequestInternalPending), tagri.Status.Phase)
			},
		},
		// Lister error injection tests using fake listers
		{
			name:   "TVP lister returns non-NotFound error - should retry",
			pvName: "pv-lister-error",
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				req.latestEventsMu.Lock()
				req.latestEvents["pv-lister-error"] = createTestEvent("pv-lister-error", 1000, 800, "test-policy")
				req.latestEventsMu.Unlock()
			},
			useFakeTVPLister: true,
			fakeTVPSetup: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) listerv1.TridentVolumePublicationLister {
				return &fakeTVPLister{err: fmt.Errorf("tvp lister connection error")}
			},
			expectError:          true,
			expectReconcileDefer: true,
			errorContains:        "failed to get TVP from lister",
		},
		{
			name:   "TVP lister returns NotFound - should not retry",
			pvName: "pv-notfound",
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				req.latestEventsMu.Lock()
				req.latestEvents["pv-notfound"] = createTestEvent("pv-notfound", 1000, 800, "test-policy")
				req.latestEventsMu.Unlock()
			},
			useFakeTVPLister: true,
			fakeTVPSetup: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) listerv1.TridentVolumePublicationLister {
				return &fakeTVPLister{tvps: []*v1.TridentVolumePublication{}, err: nil}
			},
			expectError: false, // NotFound is handled gracefully
		},
		{
			name:   "TVP has empty VolumeID - should not retry",
			pvName: "pv-empty-volumeid",
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				req.latestEventsMu.Lock()
				req.latestEvents["pv-empty-volumeid"] = createTestEvent("pv-empty-volumeid", 1000, 800, "test-policy")
				req.latestEventsMu.Unlock()
			},
			useFakeTVPLister: true,
			fakeTVPSetup: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) listerv1.TridentVolumePublicationLister {
				tvp := createTestTVP("pv-empty-volumeid", req.config.TridentNamespace, "") // Empty volumeID
				return &fakeTVPLister{tvps: []*v1.TridentVolumePublication{tvp}, err: nil}
			},
			expectError: false, // Empty VolumeID is handled gracefully
		},
		{
			name:   "AGP lister returns non-NotFound error - should retry",
			pvName: "pv-agp-lister-error",
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				req.latestEventsMu.Lock()
				req.latestEvents["pv-agp-lister-error"] = createTestEvent("pv-agp-lister-error", 1000, 800, "test-policy")
				req.latestEventsMu.Unlock()

				err := cache.SetEffectivePolicyName("pv-agp-lister-error", "test-policy")
				require.NoError(t, err)
			},
			useFakeTVPLister: true,
			fakeTVPSetup: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) listerv1.TridentVolumePublicationLister {
				tvp := createTestTVP("pv-agp-lister-error", req.config.TridentNamespace, "pv-agp-lister-error")
				return &fakeTVPLister{tvps: []*v1.TridentVolumePublication{tvp}, err: nil}
			},
			useFakeAGPLister: true,
			fakeAGPSetup: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) listerv1.TridentAutogrowPolicyLister {
				return &fakeAGPLister{err: fmt.Errorf("agp lister connection error")}
			},
			expectError:          true,
			expectReconcileDefer: true,
			errorContains:        "failed to get policy from lister",
		},
		{
			name:   "AGP lister returns NotFound - should not retry",
			pvName: "pv-agp-notfound",
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				req.latestEventsMu.Lock()
				req.latestEvents["pv-agp-notfound"] = createTestEvent("pv-agp-notfound", 1000, 800, "missing-policy")
				req.latestEventsMu.Unlock()

				err := cache.SetEffectivePolicyName("pv-agp-notfound", "missing-policy")
				require.NoError(t, err)
			},
			useFakeTVPLister: true,
			fakeTVPSetup: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) listerv1.TridentVolumePublicationLister {
				tvp := createTestTVP("pv-agp-notfound", req.config.TridentNamespace, "pv-agp-notfound")
				return &fakeTVPLister{tvps: []*v1.TridentVolumePublication{tvp}, err: nil}
			},
			useFakeAGPLister: true,
			fakeAGPSetup: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) listerv1.TridentAutogrowPolicyLister {
				return &fakeAGPLister{policies: []*v1.TridentAutogrowPolicy{}, err: nil}
			},
			expectError:   true,
			errorContains: "no longer exists",
		},
		{
			name:        "TAGRI already exists - should not retry",
			pvName:      "pv-tagri-exists",
			policyName:  "test-policy",
			policyPhase: string(v1.TridentAutogrowPolicyStateSuccess),
			setupFunc: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) {
				req.latestEventsMu.Lock()
				req.latestEvents["pv-tagri-exists"] = createTestEvent("pv-tagri-exists", 1000, 800, "test-policy")
				req.latestEventsMu.Unlock()

				err := cache.SetEffectivePolicyName("pv-tagri-exists", "test-policy")
				require.NoError(t, err)

				// Create existing TAGRI
				existingTAGRI := &v1.TridentAutogrowRequestInternal{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("tagri-%s", "pv-tagri-exists"),
						Namespace: req.config.TridentNamespace,
					},
					Spec: v1.TridentAutogrowRequestInternalSpec{
						Volume: "pv-tagri-exists",
					},
				}
				_, err = client.TridentV1().TridentAutogrowRequestInternals(req.config.TridentNamespace).Create(
					context.Background(), existingTAGRI, metav1.CreateOptions{})
				require.NoError(t, err)
			},
			useFakeTVPLister: true,
			fakeTVPSetup: func(t *testing.T, req *Requester, client *fake.Clientset, cache *agCache.AutogrowCache) listerv1.TridentVolumePublicationLister {
				tvp := createTestTVP("pv-tagri-exists", req.config.TridentNamespace, "pv-tagri-exists")
				return &fakeTVPLister{tvps: []*v1.TridentVolumePublication{tvp}, err: nil}
			},
			expectError:   true,
			errorContains: "TAGRI already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup with policy if specified in test case
			var policies []*v1.TridentAutogrowPolicy
			if tt.policyName != "" && tt.policyPhase != "" {
				policies = append(policies, createTestPolicy(tt.policyName, tt.policyPhase))
			}

			// Create TVP upfront so it's in the informer from the start
			const testNamespace = "trident"
			var tvps []*v1.TridentVolumePublication
			if tt.pvName != "" {
				tvp := createTestTVP(tt.pvName, testNamespace, tt.pvName) // volumeID same as pvName
				tvps = append(tvps, tvp)
			}

			tridentClient, autogrowCache, informerFactory, stopCh := setupTestEnvironmentWithTVPs(t, tvps, policies...)
			defer close(stopCh)

			agpLister := informerFactory.Trident().V1().TridentAutogrowPolicies().Lister()
			tvpLister := informerFactory.Trident().V1().TridentVolumePublications().Lister()

			// Create config with matching namespace
			config := &Config{
				TridentNamespace: testNamespace,
			}

			requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, config)
			require.NoError(t, err)

			tt.setupFunc(t, requester, tridentClient, autogrowCache)

			// Replace listers with fake ones if specified
			if tt.useFakeTVPLister && tt.fakeTVPSetup != nil {
				requester.tvpLister = tt.fakeTVPSetup(t, requester, tridentClient, autogrowCache)
			}
			if tt.useFakeAGPLister && tt.fakeAGPSetup != nil {
				requester.agpLister = tt.fakeAGPSetup(t, requester, tridentClient, autogrowCache)
			}

			err = requester.processWorkItem(context.Background(), WorkItem{
				Ctx:        context.Background(),
				TVPName:    tt.pvName,
				RetryCount: 0,
			})

			if tt.expectError {
				assert.Error(t, err)
				if err != nil && tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				if err != nil && tt.expectReconcileDefer {
					assert.True(t, IsReconcileDeferredError(err))
				}
			} else {
				assert.NoError(t, err)

				// Call verifyResult if provided to check side effects
				if tt.verifyResult != nil {
					tt.verifyResult(t, requester, tridentClient, tt.pvName)
				}
			}
		})
	}
}

// Test workerLoop integration with processWorkItem and workqueue
// This tests the full flow: enqueue -> workerLoop -> processWorkItem
func TestWorkerLoopIntegration(t *testing.T) {
	tests := []struct {
		name         string
		setupTest    func(*testing.T, *agCache.AutogrowCache, *gomock.Controller) (*Requester, context.Context, func())
		verifyResult func(*testing.T, *Requester)
	}{
		{
			name: "Successful processing - event cleaned up",
			setupTest: func(t *testing.T, autogrowCache *agCache.AutogrowCache, ctrl *gomock.Controller) (*Requester, context.Context, func()) {
				ctx := context.Background()

				// Create a test policy
				policy := createTestPolicy("test-policy", string(v1.TridentAutogrowPolicyStateSuccess))

				// Create informers with the policy
				agpInformer, agpLister, client := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister, _ := getTestTVPInformerAndLister()

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create mock worker pool
				mockPool := mock_workerpool.NewMockPool(ctrl)

				// Create requester
				config := &Config{
					WorkerPool:       mockPool,
					ShutdownTimeout:  10 * time.Second,
					WorkQueueName:    "test-queue",
					MaxRetries:       3,
					TridentNamespace: "default",
				}

				req, err := NewRequester(ctx, client, agpLister, tvpLister, autogrowCache, config)
				require.NoError(t, err)

				// Create workqueue manually for this test (normally created in Activate)
				req.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				req.state.set(StateRunning)

				// Setup event and cache
				event := createTestEvent("pv-success", 1000, 800)
				req.latestEventsMu.Lock()
				req.latestEvents["pv-success"] = event
				req.latestEventsMu.Unlock()

				err = autogrowCache.SetEffectivePolicyName("pv-success", "test-policy")
				require.NoError(t, err)

				// Channel to signal task completion
				taskCompleted := make(chan struct{})

				// Mock pool to signal completion
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						task() // Execute task
						close(taskCompleted)
						return nil
					}).
					Times(1)

				// Enqueue work item
				workItem := WorkItem{Ctx: ctx, TVPName: "pv-success", RetryCount: 0}
				req.workqueue.Add(workItem)

				cleanup := func() {
					select {
					case <-taskCompleted:
						// Task completed
					case <-time.After(2 * time.Second):
						t.Error("Task did not complete within timeout")
					}
					req.workqueue.ShutDown()
				}

				return req, ctx, cleanup
			},
			verifyResult: func(t *testing.T, req *Requester) {
				// Verify event was cleaned up
				req.latestEventsMu.RLock()
				_, exists := req.latestEvents["pv-success"]
				req.latestEventsMu.RUnlock()
				assert.False(t, exists, "event should be cleaned up after successful processing")

				// Verify queue is empty
				assert.Equal(t, 0, req.workqueue.Len())
			},
		},
		{
			name: "Max retries exceeded - event discarded",
			setupTest: func(t *testing.T, autogrowCache *agCache.AutogrowCache, ctrl *gomock.Controller) (*Requester, context.Context, func()) {
				ctx := context.Background()

				// Create informers (no policies)
				agpInformer, agpLister, client := getTestAGPInformerAndLister()
				tvpInformer, tvpLister, _ := getTestTVPInformerAndLister()

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create mock worker pool
				mockPool := mock_workerpool.NewMockPool(ctrl)

				// Create requester
				config := &Config{
					WorkerPool:       mockPool,
					ShutdownTimeout:  10 * time.Second,
					WorkQueueName:    "test-queue",
					MaxRetries:       3,
					TridentNamespace: "default",
				}

				req, err := NewRequester(ctx, client, agpLister, tvpLister, autogrowCache, config)
				require.NoError(t, err)

				// Create workqueue manually for this test (normally created in Activate)
				req.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				req.state.set(StateRunning)

				// Setup event but no policy (will always fail)
				event := createTestEvent("pv-maxretry", 1000, 800)
				req.latestEventsMu.Lock()
				req.latestEvents["pv-maxretry"] = event
				req.latestEventsMu.Unlock()

				// Don't set effective policy - this will return "no effective policy" error
				// which is non-retriable, so it cleans up immediately (not a max retry scenario)

				// Channel to signal task completion
				taskCompleted := make(chan struct{})

				// Mock pool to signal completion
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						task() // Execute task
						close(taskCompleted)
						return nil
					}).
					Times(1)

				workItem := WorkItem{Ctx: ctx, TVPName: "pv-maxretry", RetryCount: 0}
				req.workqueue.Add(workItem)

				cleanup := func() {
					select {
					case <-taskCompleted:
						// Task completed
					case <-time.After(2 * time.Second):
						t.Error("Task did not complete within timeout")
					}
					req.workqueue.ShutDown()
				}

				return req, ctx, cleanup
			},
			verifyResult: func(t *testing.T, req *Requester) {
				// Non-retriable error means immediate cleanup
				req.latestEventsMu.RLock()
				_, exists := req.latestEvents["pv-maxretry"]
				req.latestEventsMu.RUnlock()
				assert.False(t, exists, "event should be cleaned up for non-retriable error")

				// Queue should be empty
				assert.Equal(t, 0, req.workqueue.Len())
			},
		},
		{
			name: "Policy in failed state - success without retry",
			setupTest: func(t *testing.T, autogrowCache *agCache.AutogrowCache, ctrl *gomock.Controller) (*Requester, context.Context, func()) {
				ctx := context.Background()

				// Create a failed policy
				policy := createTestPolicy("failed-policy", string(v1.TridentAutogrowPolicyStateFailed))

				// Create informers with the failed policy
				agpInformer, agpLister, client := getTestAGPInformerAndLister(policy)
				tvpInformer, tvpLister, _ := getTestTVPInformerAndLister()

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create mock worker pool
				mockPool := mock_workerpool.NewMockPool(ctrl)

				// Create requester
				config := &Config{
					WorkerPool:       mockPool,
					ShutdownTimeout:  10 * time.Second,
					WorkQueueName:    "test-queue",
					MaxRetries:       3,
					TridentNamespace: "default",
				}

				req, err := NewRequester(ctx, client, agpLister, tvpLister, autogrowCache, config)
				require.NoError(t, err)

				// Create workqueue manually for this test (normally created in Activate)
				req.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				req.state.set(StateRunning)

				// Setup event and cache
				event := createTestEvent("pv-failed-policy", 1000, 800)
				req.latestEventsMu.Lock()
				req.latestEvents["pv-failed-policy"] = event
				req.latestEventsMu.Unlock()

				err = autogrowCache.SetEffectivePolicyName("pv-failed-policy", "failed-policy")
				require.NoError(t, err)

				// Channel to signal task completion
				taskCompleted := make(chan struct{})

				// Mock pool to signal completion
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						task() // Execute task
						close(taskCompleted)
						return nil
					}).
					Times(1)

				// Enqueue work item
				workItem := WorkItem{Ctx: ctx, TVPName: "pv-failed-policy", RetryCount: 0}
				req.workqueue.Add(workItem)

				cleanup := func() {
					select {
					case <-taskCompleted:
						// Task completed
					case <-time.After(2 * time.Second):
						t.Error("Task did not complete within timeout")
					}
					req.workqueue.ShutDown()
				}

				return req, ctx, cleanup
			},
			verifyResult: func(t *testing.T, req *Requester) {
				// processWorkItem returns nil for failed policies, so event should be cleaned up
				req.latestEventsMu.RLock()
				_, exists := req.latestEvents["pv-failed-policy"]
				req.latestEventsMu.RUnlock()
				assert.False(t, exists, "event should be cleaned up for failed policy")

				assert.Equal(t, 0, req.workqueue.Len())
			},
		},
		{
			name: "Worker pool submit fails - retry",
			setupTest: func(t *testing.T, autogrowCache *agCache.AutogrowCache, ctrl *gomock.Controller) (*Requester, context.Context, func()) {
				ctx := context.Background()

				// Create informers (no specific policies/TVPs needed)
				agpInformer, agpLister, client := getTestAGPInformerAndLister()
				tvpInformer, tvpLister, _ := getTestTVPInformerAndLister()

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create mock worker pool
				mockPool := mock_workerpool.NewMockPool(ctrl)

				// Create requester
				config := &Config{
					WorkerPool:       mockPool,
					ShutdownTimeout:  10 * time.Second,
					WorkQueueName:    "test-queue",
					MaxRetries:       3,
					TridentNamespace: "default",
				}

				req, err := NewRequester(ctx, client, agpLister, tvpLister, autogrowCache, config)
				require.NoError(t, err)

				// Create workqueue manually for this test (normally created in Activate)
				req.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				req.state.set(StateRunning)

				// Channel to signal first attempt
				taskCompleted := make(chan struct{})

				// Mock pool that fails Submit and signals after first attempt
				// Allow multiple calls since item gets requeued and retried
				var callCount atomic.Int32
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						if callCount.Add(1) == 1 {
							close(taskCompleted)
						}
						return fmt.Errorf("pool full")
					}).
					AnyTimes()

				// Setup event
				event := createTestEvent("pv-pool-fail", 1000, 800)
				req.latestEventsMu.Lock()
				req.latestEvents["pv-pool-fail"] = event
				req.latestEventsMu.Unlock()

				// Enqueue work item
				workItem := WorkItem{Ctx: ctx, TVPName: "pv-pool-fail", RetryCount: 0}
				req.workqueue.Add(workItem)

				cleanup := func() {
					select {
					case <-taskCompleted:
						// First attempt completed
					case <-time.After(2 * time.Second):
						t.Error("First attempt did not complete within timeout")
					}
					req.workqueue.ShutDown()
				}

				return req, ctx, cleanup
			},
			verifyResult: func(t *testing.T, req *Requester) {
				// Event should NOT be cleaned up (kept for retry)
				req.latestEventsMu.RLock()
				_, exists := req.latestEvents["pv-pool-fail"]
				req.latestEventsMu.RUnlock()
				assert.True(t, exists, "event should be kept for retry when pool submit fails")
			},
		},
		{
			name: "Context cancelled - worker loop exits",
			setupTest: func(t *testing.T, autogrowCache *agCache.AutogrowCache, ctrl *gomock.Controller) (*Requester, context.Context, func()) {
				ctx, cancel := context.WithCancel(context.Background())

				// Create informers
				agpInformer, agpLister, client := getTestAGPInformerAndLister()
				tvpInformer, tvpLister, _ := getTestTVPInformerAndLister()

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create mock worker pool
				mockPool := mock_workerpool.NewMockPool(ctrl)

				// Create requester
				config := &Config{
					WorkerPool:       mockPool,
					ShutdownTimeout:  10 * time.Second,
					WorkQueueName:    "test-queue",
					MaxRetries:       3,
					TridentNamespace: "default",
				}

				req, err := NewRequester(ctx, client, agpLister, tvpLister, autogrowCache, config)
				require.NoError(t, err)

				// Create workqueue manually for this test (normally created in Activate)
				req.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				req.state.set(StateRunning)

				// Setup event
				event := createTestEvent("pv-cancelled", 1000, 800)
				req.latestEventsMu.Lock()
				req.latestEvents["pv-cancelled"] = event
				req.latestEventsMu.Unlock()

				// Enqueue work item
				workItem := WorkItem{Ctx: ctx, TVPName: "pv-cancelled", RetryCount: 0}
				req.workqueue.Add(workItem)

				// Cancel context immediately
				cancel()

				cleanup := func() {
					req.workqueue.ShutDown()
				}

				return req, ctx, cleanup
			},
			verifyResult: func(t *testing.T, req *Requester) {
				// Worker loop should exit, event may or may not be processed
				// Just verify we don't hang
			},
		},
		{
			name: "Shutdown while processing - graceful drain",
			setupTest: func(t *testing.T, autogrowCache *agCache.AutogrowCache, ctrl *gomock.Controller) (*Requester, context.Context, func()) {
				ctx := context.Background()

				// Create informers
				agpInformer, agpLister, client := getTestAGPInformerAndLister()
				tvpInformer, tvpLister, _ := getTestTVPInformerAndLister()

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create mock worker pool (no expectations since state is Stopping)
				mockPool := mock_workerpool.NewMockPool(ctrl)

				// Create requester
				config := &Config{
					WorkerPool:       mockPool,
					ShutdownTimeout:  10 * time.Second,
					WorkQueueName:    "test-queue",
					MaxRetries:       3,
					TridentNamespace: "default",
				}

				req, err := NewRequester(ctx, client, agpLister, tvpLister, autogrowCache, config)
				require.NoError(t, err)

				// Create workqueue manually for this test (normally created in Activate)
				req.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				// Set state to Stopping BEFORE adding items
				// This ensures workerLoop will hit the !IsRunning() check (lines 296-299)
				req.state.set(StateStopping)

				// Enqueue multiple items - these will be drained without processing
				for i := 0; i < 3; i++ {
					pvName := fmt.Sprintf("pv-shutdown-%d", i)
					event := createTestEvent(pvName, 1000, 800)
					req.latestEventsMu.Lock()
					req.latestEvents[pvName] = event
					req.latestEventsMu.Unlock()

					workItem := WorkItem{Ctx: ctx, TVPName: pvName, RetryCount: 0}
					req.workqueue.Add(workItem)
				}

				// No mock needed since Submit won't be called (state is Stopping)
				cleanup := func() {
					time.Sleep(50 * time.Millisecond) // Allow drain to start
					req.workqueue.ShutDown()
				}

				return req, ctx, cleanup
			},
			verifyResult: func(t *testing.T, req *Requester) {
				// During shutdown, items are drained without processing (via workqueue.Done())
				// Events should still exist because they were not processed
				req.latestEventsMu.RLock()
				numEvents := len(req.latestEvents)
				req.latestEventsMu.RUnlock()
				assert.Equal(t, 3, numEvents, "events should not be processed during graceful drain")

				// Queue should be drained
				assert.Equal(t, 0, req.workqueue.Len(), "queue should be drained during shutdown")
			},
		},
		{
			name: "No event found - immediate cleanup",
			setupTest: func(t *testing.T, autogrowCache *agCache.AutogrowCache, ctrl *gomock.Controller) (*Requester, context.Context, func()) {
				ctx := context.Background()

				// Create informers
				agpInformer, agpLister, client := getTestAGPInformerAndLister()
				tvpInformer, tvpLister, _ := getTestTVPInformerAndLister()

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create mock worker pool
				mockPool := mock_workerpool.NewMockPool(ctrl)

				// Create requester
				config := &Config{
					WorkerPool:       mockPool,
					ShutdownTimeout:  10 * time.Second,
					WorkQueueName:    "test-queue",
					MaxRetries:       3,
					TridentNamespace: "default",
				}

				req, err := NewRequester(ctx, client, agpLister, tvpLister, autogrowCache, config)
				require.NoError(t, err)

				// Create workqueue manually for this test (normally created in Activate)
				req.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				req.state.set(StateRunning)

				// Channel to signal task completion
				taskCompleted := make(chan struct{})

				// Mock pool to signal completion
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						task() // Execute task
						close(taskCompleted)
						return nil
					}).
					Times(1)

				// Enqueue work item but DON'T add event to latestEvents
				workItem := WorkItem{Ctx: ctx, TVPName: "pv-no-event", RetryCount: 0}
				req.workqueue.Add(workItem)

				cleanup := func() {
					select {
					case <-taskCompleted:
						// Task completed
					case <-time.After(2 * time.Second):
						t.Error("Task did not complete within timeout")
					}
					req.workqueue.ShutDown()
				}

				return req, ctx, cleanup
			},
			verifyResult: func(t *testing.T, req *Requester) {
				// Queue should be empty (non-retriable error)
				assert.Equal(t, 0, req.workqueue.Len())
			},
		},
		{
			name: "Max retries exceeded - event discarded via fake lister",
			setupTest: func(t *testing.T, autogrowCache *agCache.AutogrowCache, ctrl *gomock.Controller) (*Requester, context.Context, func()) {
				ctx := context.Background()

				// Create informers
				agpInformer, agpLister, client := getTestAGPInformerAndLister()
				tvpInformer, tvpLister, _ := getTestTVPInformerAndLister()

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create mock worker pool
				mockPool := mock_workerpool.NewMockPool(ctrl)

				// Create requester
				config := &Config{
					WorkerPool:       mockPool,
					ShutdownTimeout:  10 * time.Second,
					WorkQueueName:    "test-queue",
					MaxRetries:       2, // Low value for testing
					TridentNamespace: "default",
				}

				req, err := NewRequester(ctx, client, agpLister, tvpLister, autogrowCache, config)
				require.NoError(t, err)

				// Create workqueue manually for this test (normally created in Activate)
				req.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				req.state.set(StateRunning)

				tvpName := "pv-max-retries"

				// Add event
				event := createTestEvent(tvpName, 1000, 800, "test-policy")
				req.latestEventsMu.Lock()
				req.latestEvents[tvpName] = event
				req.latestEventsMu.Unlock()

				// Create a fake TVP lister that always returns a retriable error
				fakeTVPLister := &fakeTVPLister{
					err: fmt.Errorf("persistent connection error"),
				}

				// Replace lister with fake one
				req.tvpLister = fakeTVPLister

				// Channel to signal task completion
				taskCompleted := make(chan struct{})

				// Mock pool to signal completion
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						task() // Execute task
						close(taskCompleted)
						return nil
					}).
					Times(1)

				// Add work item with RetryCount already at MaxRetries
				workItem := WorkItem{Ctx: ctx, TVPName: tvpName, RetryCount: 2}
				req.workqueue.Add(workItem)

				cleanup := func() {
					select {
					case <-taskCompleted:
						// Task completed
					case <-time.After(2 * time.Second):
						t.Error("Task did not complete within timeout")
					}
					req.workqueue.ShutDown()
				}

				return req, ctx, cleanup
			},
			verifyResult: func(t *testing.T, req *Requester) {
				// Event should be cleaned up after max retries
				req.latestEventsMu.RLock()
				_, exists := req.latestEvents["pv-max-retries"]
				req.latestEventsMu.RUnlock()
				assert.False(t, exists, "event should be cleaned up after max retries exceeded")

				// Queue should be empty (item forgotten, not re-added)
				assert.Equal(t, 0, req.workqueue.Len())
			},
		},
		{
			name: "Workqueue shutdown - worker exits gracefully",
			setupTest: func(t *testing.T, autogrowCache *agCache.AutogrowCache, ctrl *gomock.Controller) (*Requester, context.Context, func()) {
				ctx := context.Background()

				// Create informers
				agpInformer, agpLister, client := getTestAGPInformerAndLister()
				tvpInformer, tvpLister, _ := getTestTVPInformerAndLister()

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create mock worker pool
				mockPool := mock_workerpool.NewMockPool(ctrl)

				// Create requester
				config := &Config{
					WorkerPool:       mockPool,
					ShutdownTimeout:  10 * time.Second,
					WorkQueueName:    "test-queue",
					MaxRetries:       3,
					TridentNamespace: "default",
				}

				req, err := NewRequester(ctx, client, agpLister, tvpLister, autogrowCache, config)
				require.NoError(t, err)

				// Create workqueue manually for this test (normally created in Activate)
				req.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				req.state.set(StateRunning)

				// Channel to signal when we're ready to proceed
				taskCompleted := make(chan struct{})

				// Mock pool - item might or might not be processed before shutdown
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						task() // Execute task if it gets picked up
						return nil
					}).
					AnyTimes() // 0 or more times depending on timing

				// Add an event
				event := createTestEvent("pv-shutdown-test", 1000, 800)
				req.latestEventsMu.Lock()
				req.latestEvents["pv-shutdown-test"] = event
				req.latestEventsMu.Unlock()

				// Enqueue work item
				workItem := WorkItem{Ctx: ctx, TVPName: "pv-shutdown-test", RetryCount: 0}
				req.workqueue.Add(workItem)

				// Shutdown the queue immediately to trigger shutdown path
				go func() {
					time.Sleep(50 * time.Millisecond)
					req.workqueue.ShutDown()
					close(taskCompleted)
				}()

				cleanup := func() {
					select {
					case <-taskCompleted:
						// Shutdown initiated
					case <-time.After(2 * time.Second):
						t.Error("Shutdown did not complete within timeout")
					}
				}

				return req, ctx, cleanup
			},
			verifyResult: func(t *testing.T, req *Requester) {
				// Verify queue is shutdown (but not nil since Deactivate() wasn't called)
				assert.True(t, req.workqueue.ShuttingDown(), "workqueue should be shutting down")
			},
		},
		{
			name: "Rate limiting error with retry-after - uses AddAfter",
			setupTest: func(t *testing.T, autogrowCache *agCache.AutogrowCache, ctrl *gomock.Controller) (*Requester, context.Context, func()) {
				ctx := context.Background()

				// Create informers
				agpInformer, agpLister, client := getTestAGPInformerAndLister()
				tvpInformer, tvpLister, _ := getTestTVPInformerAndLister()

				// Start informers
				stopCh := make(chan struct{})
				t.Cleanup(func() { close(stopCh) })
				go agpInformer.Run(stopCh)
				go tvpInformer.Run(stopCh)
				cache.WaitForCacheSync(stopCh, agpInformer.HasSynced, tvpInformer.HasSynced)

				// Create mock worker pool
				mockPool := mock_workerpool.NewMockPool(ctrl)

				// Create requester
				config := &Config{
					WorkerPool:       mockPool,
					ShutdownTimeout:  10 * time.Second,
					WorkQueueName:    "test-queue",
					MaxRetries:       3,
					TridentNamespace: "default",
				}

				req, err := NewRequester(ctx, client, agpLister, tvpLister, autogrowCache, config)
				require.NoError(t, err)

				// Create workqueue manually for this test (normally created in Activate)
				req.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)

				req.state.set(StateRunning)

				tvpName := "pv-rate-limit-test"

				// Add event
				event := createTestEvent(tvpName, 1000, 800, "test-policy")
				req.latestEventsMu.Lock()
				req.latestEvents[tvpName] = event
				req.latestEventsMu.Unlock()

				// Create a StatusError with TooManyRequests and RetryAfterSeconds
				statusErr := &k8serrors.StatusError{
					ErrStatus: metav1.Status{
						Status: metav1.StatusFailure,
						Code:   429,
						Reason: metav1.StatusReasonTooManyRequests,
						Details: &metav1.StatusDetails{
							RetryAfterSeconds: 3,
						},
						Message: "rate limited, please retry",
					},
				}

				// Create a fake TVP lister that returns the rate limit error wrapped in ReconcileDeferredError
				// This simulates what happens when createTAGRI gets a rate limit error
				fakeTVPLister := &fakeTVPLister{
					err: WrapWithReconcileDeferredError(statusErr, "failed to get TVP from lister"),
				}

				// Replace lister with fake one
				req.tvpLister = fakeTVPLister

				// Channel to signal task completion
				taskCompleted := make(chan struct{})

				// Mock pool to signal completion
				mockPool.EXPECT().
					Submit(gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, task func()) error {
						task() // Execute task
						close(taskCompleted)
						return nil
					}).
					Times(1)

				// Enqueue work item
				workItem := WorkItem{Ctx: ctx, TVPName: tvpName, RetryCount: 0}
				req.workqueue.Add(workItem)

				cleanup := func() {
					select {
					case <-taskCompleted:
						// Task completed
					case <-time.After(2 * time.Second):
						t.Error("Task did not complete within timeout")
					}
					req.workqueue.ShutDown()
				}

				return req, ctx, cleanup
			},
			verifyResult: func(t *testing.T, req *Requester) {
				// Event should NOT be cleaned up (kept for retry with AddAfter)
				req.latestEventsMu.RLock()
				_, exists := req.latestEvents["pv-rate-limit-test"]
				req.latestEventsMu.RUnlock()
				assert.True(t, exists, "event should remain for retry after rate limit")

				// Note: Queue will have been called with AddAfter(workItem, 3*time.Second)
				// We can't easily verify the exact timing, but we can verify event remains
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create autogrow cache
			autogrowCache := agCache.NewAutogrowCache()

			// Setup test-specific scenario - returns requester, context and cleanup function
			requester, ctx, cleanup := tt.setupTest(t, autogrowCache, ctrl)

			// Start worker loop in background
			done := make(chan bool)
			go func() {
				requester.workerLoop(ctx)
				done <- true
			}()

			// Wait a brief moment to let workerLoop start
			time.Sleep(50 * time.Millisecond)

			// Trigger cleanup to stop the loop
			cleanup()

			// Wait for workerLoop to complete or timeout
			select {
			case <-done:
				// workerLoop exited normally
			case <-time.After(2 * time.Second):
				t.Fatal("workerLoop did not exit within timeout")
			}

			// Verify results
			tt.verifyResult(t, requester)
		})
	}
}

// Test workerLoop concurrent processing
func TestWorkerLoopConcurrent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
	defer close(stopCh)

	agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
	tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

	// Create a real worker pool for testing
	ctx := context.Background()
	workerPool, err := workerpool.New[workerpooltypes.Pool](
		ctx,
		ants.NewConfig(
			ants.WithNumWorkers(4),
			ants.WithPreAlloc(true),
			ants.WithNonBlocking(false),
		),
	)
	require.NoError(t, err)
	require.NoError(t, workerPool.Start(ctx))
	defer workerPool.ShutdownWithTimeout(5 * time.Second)

	// Create requester with multiple workers
	config := &Config{
		ShutdownTimeout:  10 * time.Second,
		WorkQueueName:    "test-queue",
		MaxRetries:       3,
		TridentNamespace: "default",
		WorkerPool:       workerPool,
	}

	requester, err := NewRequester(context.Background(), tridentClient, agpLister, tvpLister, autogrowCache, config)
	require.NoError(t, err)

	// Create workqueue manually for this test (normally created in Activate)
	requester.workqueue = workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
	)

	// Set state to running
	atomic.StoreInt32((*int32)(&requester.state), int32(StateRunning))

	// Create a test policy
	policy := createTestPolicy("concurrent-policy", string(v1.TridentAutogrowPolicyStateSuccess))
	_, err = tridentClient.TridentV1().TridentAutogrowPolicies().Create(context.Background(), policy, metav1.CreateOptions{})
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)

	// Enqueue many work items
	numItems := 20
	for i := 0; i < numItems; i++ {
		pvName := fmt.Sprintf("pv-concurrent-%d", i)
		event := createTestEvent(pvName, 1000, 800)

		requester.latestEventsMu.Lock()
		requester.latestEvents[pvName] = event
		requester.latestEventsMu.Unlock()

		_ = autogrowCache.SetEffectivePolicyName(pvName, "concurrent-policy")

		workItem := WorkItem{Ctx: context.Background(), TVPName: pvName, RetryCount: 0}
		requester.workqueue.Add(workItem)
	}

	// Start worker loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go requester.workerLoop(ctx)

	// Wait for all items to be processed
	timeout := time.After(2 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for work items to be processed")
		case <-ticker.C:
			if requester.workqueue.Len() == 0 {
				// All items processed
				cancel()
				time.Sleep(50 * time.Millisecond)

				// Verify all events cleaned up
				requester.latestEventsMu.RLock()
				numEvents := len(requester.latestEvents)
				requester.latestEventsMu.RUnlock()

				assert.Equal(t, 0, numEvents, "all events should be cleaned up after processing")
				return
			}
		}
	}
}

// Test handleVolumeThresholdBreached
func TestHandleVolumeThresholdBreached(t *testing.T) {
	tests := []struct {
		name           string
		event          autogrowTypes.VolumeThresholdBreached
		requesterState State
		expectError    bool
		expectResult   bool
	}{
		{
			name:           "Valid event - successfully enqueued",
			event:          createTestEvent("pv-handle-1", 1000, 800),
			requesterState: StateRunning,
			expectError:    false,
			expectResult:   true,
		},
		{
			name: "Valid event with nil context - uses handler context",
			event: autogrowTypes.VolumeThresholdBreached{
				Ctx:              nil, // nil context to test fallback (lines 510-512)
				ID:               1,
				TVPName:          "pv-nil-ctx",
				TAGPName:         "test-policy",
				CurrentTotalSize: 1000,
				UsedSize:         800,
			},
			requesterState: StateRunning,
			expectError:    false,
			expectResult:   true,
		},
		{
			name: "Invalid event - missing PV name",
			event: autogrowTypes.VolumeThresholdBreached{
				Ctx:              context.Background(),
				ID:               1,
				TVPName:          "",
				TAGPName:         "test-policy",
				CurrentTotalSize: 1000,
				UsedSize:         800,
			},
			requesterState: StateRunning,
			expectError:    true,
			expectResult:   false,
		},
		{
			name:           "Requester not running",
			event:          createTestEvent("pv-handle-2", 1000, 800),
			requesterState: StateStopped,
			expectError:    true,
			expectResult:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
			defer close(stopCh)

			agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
			tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

			requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, nil)
			require.NoError(t, err)

			// Create workqueue manually for this test (normally created in Activate)
			if tt.requesterState == StateRunning {
				requester.workqueue = workqueue.NewTypedRateLimitingQueue(
					workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
				)
			}

			// Set requester state
			requester.state.set(tt.requesterState)

			// Call handleVolumeThresholdBreached
			result, err := requester.handleVolumeThresholdBreached(ctx, tt.event)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				if tt.expectResult {
					assert.Equal(t, true, result["enqueued"])
					assert.Equal(t, tt.event.TVPName, result["tvpName"]) // Changed from "pvName" to "tvpName"
				}
			}
		})
	}
}

// Test Deactivate
func TestDeactivate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name                string
		shouldActivateFirst bool
		initialState        State
		setupMocks          func(*testing.T, *Requester, *gomock.Controller)
		expectError         bool
		expectedState       State
	}{
		{
			name:                "Deactivate from running state",
			shouldActivateFirst: true,
			initialState:        StateRunning,
			setupMocks: func(t *testing.T, req *Requester, ctrl *gomock.Controller) {
				// Mock eventbus for activation
				mockEventBus := mock_eventbus.NewMockEventBusMetricsOptions[autogrowTypes.VolumeThresholdBreached](ctrl)
				mockEventBus.EXPECT().Subscribe(gomock.Any(), gomock.Any()).Return(eventbusTypes.SubscriptionID(123), nil).Times(1)
				mockEventBus.EXPECT().Unsubscribe(gomock.Any()).Return(true).Times(1)
				eventbus.VolumeThresholdBreachedEventBus = mockEventBus

				// Provided pool - we don't call Start() on it
			},
			expectError:   false,
			expectedState: StateStopped,
		},
		{
			name:                "Deactivate from stopped state - idempotent",
			shouldActivateFirst: false,
			initialState:        StateStopped,
			setupMocks: func(t *testing.T, req *Requester, ctrl *gomock.Controller) {
				// No mocks needed - already stopped
			},
			expectError:   false,
			expectedState: StateStopped,
		},
		{
			name:                "Deactivate from StateStarting - returns StateError",
			shouldActivateFirst: false,
			initialState:        StateStarting,
			setupMocks: func(t *testing.T, req *Requester, ctrl *gomock.Controller) {
				// State will be set to Starting
				// No mocks needed - should fail before attempting any operations
			},
			expectError:   true,
			expectedState: StateStarting,
		},
		{
			name:                "Deactivate from StateStopping - returns StateError",
			shouldActivateFirst: false,
			initialState:        StateStopping,
			setupMocks: func(t *testing.T, req *Requester, ctrl *gomock.Controller) {
				// State will be set to Stopping
				// No mocks needed - should fail before attempting any operations
			},
			expectError:   true,
			expectedState: StateStopping,
		},
		{
			name:                "Deactivate with Unsubscribe failure - logs warning but succeeds",
			shouldActivateFirst: true,
			initialState:        StateRunning,
			setupMocks: func(t *testing.T, req *Requester, ctrl *gomock.Controller) {
				// Mock eventbus for activation
				mockEventBus := mock_eventbus.NewMockEventBusMetricsOptions[autogrowTypes.VolumeThresholdBreached](ctrl)
				mockEventBus.EXPECT().Subscribe(gomock.Any(), gomock.Any()).Return(eventbusTypes.SubscriptionID(123), nil).Times(1)
				// Mock Unsubscribe to return false (failure to unsubscribe)
				mockEventBus.EXPECT().Unsubscribe(gomock.Any()).Return(false).Times(1)
				eventbus.VolumeThresholdBreachedEventBus = mockEventBus

				// Provided pool - we don't call Start() on it
			},
			expectError:   false, // Deactivate still succeeds even if unsubscribe fails
			expectedState: StateStopped,
		},
		{
			name:                "Deactivate with provided worker pool - pool not shut down",
			shouldActivateFirst: true,
			initialState:        StateRunning,
			setupMocks: func(t *testing.T, req *Requester, ctrl *gomock.Controller) {
				// Mock eventbus for activation
				mockEventBus := mock_eventbus.NewMockEventBusMetricsOptions[autogrowTypes.VolumeThresholdBreached](ctrl)
				mockEventBus.EXPECT().Subscribe(gomock.Any(), gomock.Any()).Return(eventbusTypes.SubscriptionID(123), nil).Times(1)
				mockEventBus.EXPECT().Unsubscribe(gomock.Any()).Return(true).Times(1)
				eventbus.VolumeThresholdBreachedEventBus = mockEventBus

				// Provided pool (ownWorkerPool=false) - Deactivate should NOT shut it down
				// No shutdown expectation on the mock pool
			},
			expectError:   false,
			expectedState: StateStopped,
		},
		{
			name:                "Deactivate with provided worker pool - no shutdown attempted",
			shouldActivateFirst: true,
			initialState:        StateRunning,
			setupMocks: func(t *testing.T, req *Requester, ctrl *gomock.Controller) {
				// Mock eventbus for activation
				mockEventBus := mock_eventbus.NewMockEventBusMetricsOptions[autogrowTypes.VolumeThresholdBreached](ctrl)
				mockEventBus.EXPECT().Subscribe(gomock.Any(), gomock.Any()).Return(eventbusTypes.SubscriptionID(123), nil).Times(1)
				mockEventBus.EXPECT().Unsubscribe(gomock.Any()).Return(true).Times(1)
				eventbus.VolumeThresholdBreachedEventBus = mockEventBus

				// Provided pool (ownWorkerPool=false) - Deactivate should NOT shut it down
				// No shutdown expectation on the mock pool
			},
			expectError:   false,
			expectedState: StateStopped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
			defer close(stopCh)

			agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
			tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

			// Create with mock pool
			mockPool := mock_workerpool.NewMockPool(ctrl)
			config := &Config{
				WorkerPool:       mockPool,
				TridentNamespace: "default",
				MaxRetries:       5,
				ShutdownTimeout:  30 * time.Second,
			}

			requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, config)
			require.NoError(t, err)

			// Setup mocks
			tt.setupMocks(t, requester, ctrl)

			// Activate first if needed
			if tt.shouldActivateFirst {
				err = requester.Activate(ctx)
				require.NoError(t, err)
			} else {
				// Set initial state
				requester.state.set(tt.initialState)
			}

			// Call Deactivate
			err = requester.Deactivate(ctx)

			if tt.expectError {
				assert.Error(t, err)
				// For StateError cases, verify it's actually a StateError
				if tt.initialState == StateStarting || tt.initialState == StateStopping {
					assert.True(t, IsStateError(err), "expected StateError for invalid state transition")
					assert.Contains(t, err.Error(), "cannot deactivate requester")
				}
			} else {
				assert.NoError(t, err)
			}

			// Verify final state
			finalState := requester.state.get()
			assert.Equal(t, tt.expectedState, finalState)

			// Verify workqueue is shutdown (only if we activated first)
			if tt.shouldActivateFirst {
				assert.True(t, requester.workqueue.ShuttingDown(), "workqueue should be shutting down after deactivation")
			}
		})
	}
}

// TestRequester_GetState tests the GetState method
func TestRequester_GetState(t *testing.T) {
	ctx := context.Background()

	tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
	defer close(stopCh)

	agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
	tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

	requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, nil)
	require.NoError(t, err)

	// Verify initial state
	assert.Equal(t, StateStopped, requester.GetState())

	// Manually set to different states and verify GetState returns them
	requester.state.set(StateStarting)
	assert.Equal(t, StateStarting, requester.GetState())

	requester.state.set(StateRunning)
	assert.Equal(t, StateRunning, requester.GetState())

	requester.state.set(StateStopping)
	assert.Equal(t, StateStopping, requester.GetState())
}

// TestRequester_SetState tests the SetState method
func TestRequester_SetState(t *testing.T) {
	ctx := context.Background()

	tridentClient, autogrowCache, _, stopCh := setupTestEnvironment(t)
	defer close(stopCh)

	agpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentAutogrowPolicies().Lister()
	tvpLister := tridentinformers.NewSharedInformerFactory(tridentClient, 0).Trident().V1().TridentVolumePublications().Lister()

	requester, err := NewRequester(ctx, tridentClient, agpLister, tvpLister, autogrowCache, nil)
	require.NoError(t, err)

	// Test setting different states
	states := []State{StateStopped, StateStarting, StateRunning, StateStopping}

	for _, state := range states {
		requester.SetState(state)
		assert.Equal(t, state, requester.state.get())
		assert.Equal(t, state, requester.GetState())
	}
}
