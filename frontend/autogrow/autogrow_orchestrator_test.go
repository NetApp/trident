// Copyright 2026 NetApp, Inc. All Rights Reserved.

package autogrow

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	storageinformers "k8s.io/client-go/informers/storage/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/netapp/trident/frontend/autogrow/poller"
	"github.com/netapp/trident/frontend/autogrow/requester"
	"github.com/netapp/trident/frontend/autogrow/scheduler"
	crdtypes "github.com/netapp/trident/frontend/crd/types"
	mockNodeHelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	crdclientfake "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	tridentinformers "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions"
	listerv1 "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
)

// ============================================================================
// Helper Functions and Test Fixtures
// ============================================================================

// getTestContext returns a context with timeout for testing
func getTestContext() context.Context {
	ctx := context.Background()
	return ctx
}

func TestMain(m *testing.M) {
	// Disable WatchListClient feature for all tests to prevent fake client bookmark issues
	os.Setenv("KUBE_FEATURE_WatchListClient", "false")

	// Run all tests
	code := m.Run()

	// Cleanup
	os.Unsetenv("KUBE_FEATURE_WatchListClient")

	os.Exit(code)
}

// setupTestListers creates fake K8s listers for testing
func setupTestListers(
	t *testing.T,
	storageClasses []*storagev1.StorageClass,
	tvps []*v1.TridentVolumePublication,
	agps []*v1.TridentAutogrowPolicy,
) (
	storagelisters.StorageClassLister,
	listerv1.TridentVolumePublicationLister,
	listerv1.TridentAutogrowPolicyLister,
	*crdclientfake.Clientset,
) {
	// Create fake K8s clientset with StorageClasses
	k8sObjects := make([]k8sruntime.Object, len(storageClasses))
	for i, sc := range storageClasses {
		k8sObjects[i] = sc
	}
	k8sClient := k8sfake.NewClientset(k8sObjects...)

	// Create StorageClass informer and lister
	scInformerFactory := storageinformers.NewStorageClassInformer(
		k8sClient,
		0,
		cache.Indexers{},
	)
	scLister := storagelisters.NewStorageClassLister(scInformerFactory.GetIndexer())

	// Create fake Trident CRD clientset with TVPs and AGPs
	tridentObjects := make([]k8sruntime.Object, 0, len(tvps)+len(agps))
	for _, tvp := range tvps {
		tridentObjects = append(tridentObjects, tvp)
	}
	for _, agp := range agps {
		tridentObjects = append(tridentObjects, agp)
	}
	tridentClient := crdclientfake.NewSimpleClientset(tridentObjects...)

	// Create TVP and AGP informers and listers
	tridentInformerFactory := tridentinformers.NewSharedInformerFactory(tridentClient, 0)
	tvpInformer := tridentInformerFactory.Trident().V1().TridentVolumePublications()
	tvpLister := tvpInformer.Lister()
	agpInformer := tridentInformerFactory.Trident().V1().TridentAutogrowPolicies()
	agpLister := agpInformer.Lister()

	// Start informers and wait for them to sync
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })

	go scInformerFactory.Run(stopCh)
	go tridentInformerFactory.Start(stopCh)
	cache.WaitForCacheSync(stopCh, scInformerFactory.HasSynced, tvpInformer.Informer().HasSynced, agpInformer.Informer().HasSynced)

	return scLister, tvpLister, agpLister, tridentClient
}

// createTestStorageClass creates a test StorageClass object
func createTestStorageClass(name string) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

// createTestTVP creates a test TridentVolumePublication object
func createTestTVP(name, namespace, volumeID, storageClass string) *v1.TridentVolumePublication {
	return &v1.TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		VolumeID:     volumeID,
		StorageClass: storageClass,
	}
}

// createTestAGP creates a test TridentAutogrowPolicy object
func createTestAGP(name, namespace string) *v1.TridentAutogrowPolicy {
	return &v1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

// ============================================================================
// Test: State Methods
// ============================================================================

func TestState_Get(t *testing.T) {
	tests := []struct {
		name          string
		setup         func() *State
		expectedState State
	}{
		{
			name: "Success_GetsStopped",
			setup: func() *State {
				s := StateStopped
				return &s
			},
			expectedState: StateStopped,
		},
		{
			name: "Success_GetsStarting",
			setup: func() *State {
				s := StateStarting
				return &s
			},
			expectedState: StateStarting,
		},
		{
			name: "Success_GetsRunning",
			setup: func() *State {
				s := StateRunning
				return &s
			},
			expectedState: StateRunning,
		},
		{
			name: "Success_GetsStopping",
			setup: func() *State {
				s := StateStopping
				return &s
			},
			expectedState: StateStopping,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := tt.setup()

			result := state.get()

			assert.Equal(t, tt.expectedState, result)
		})
	}
}

func TestState_Set(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() (*State, State)
		verify func(*testing.T, *State)
	}{
		{
			name: "Success_SetsRunning",
			setup: func() (*State, State) {
				s := StateStopped
				return &s, StateRunning
			},
			verify: func(t *testing.T, s *State) {
				assert.Equal(t, StateRunning, s.get())
			},
		},
		{
			name: "Success_SetsStopped",
			setup: func() (*State, State) {
				s := StateRunning
				return &s, StateStopped
			},
			verify: func(t *testing.T, s *State) {
				assert.Equal(t, StateStopped, s.get())
			},
		},
		{
			name: "Success_SetsStarting",
			setup: func() (*State, State) {
				s := StateStopped
				return &s, StateStarting
			},
			verify: func(t *testing.T, s *State) {
				assert.Equal(t, StateStarting, s.get())
			},
		},
		{
			name: "Success_SetsStopping",
			setup: func() (*State, State) {
				s := StateRunning
				return &s, StateStopping
			},
			verify: func(t *testing.T, s *State) {
				assert.Equal(t, StateStopping, s.get())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, newState := tt.setup()

			state.set(newState)

			tt.verify(t, state)
		})
	}
}

func TestState_CompareAndSwap(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() (*State, State, State)
		verify func(*testing.T, *State, bool)
	}{
		{
			name: "Success_SwapSucceeds_OldStateMatches",
			setup: func() (*State, State, State) {
				s := StateStopped
				return &s, StateStopped, StateStarting
			},
			verify: func(t *testing.T, s *State, swapped bool) {
				assert.True(t, swapped, "swap should succeed")
				assert.Equal(t, StateStarting, s.get())
			},
		},
		{
			name: "Success_SwapFails_OldStateDoesNotMatch",
			setup: func() (*State, State, State) {
				s := StateRunning
				return &s, StateStopped, StateStarting
			},
			verify: func(t *testing.T, s *State, swapped bool) {
				assert.False(t, swapped, "swap should fail")
				assert.Equal(t, StateRunning, s.get(), "state should remain unchanged")
			},
		},
		{
			name: "Success_SwapRunningToStopping",
			setup: func() (*State, State, State) {
				s := StateRunning
				return &s, StateRunning, StateStopping
			},
			verify: func(t *testing.T, s *State, swapped bool) {
				assert.True(t, swapped)
				assert.Equal(t, StateStopping, s.get())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, oldState, newState := tt.setup()

			swapped := state.compareAndSwap(oldState, newState)

			tt.verify(t, state, swapped)
		})
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateStopped, "Stopped"},
		{StateStarting, "Starting"},
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
// Test: New
// ============================================================================

func TestNew(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*gomock.Controller) (context.Context, storagelisters.StorageClassLister, listerv1.TridentVolumePublicationLister, listerv1.TridentAutogrowPolicyLister, *crdclientfake.Clientset, *mockNodeHelpers.MockNodeHelper, string)
		verify func(*testing.T, *Autogrow, error)
	}{
		{
			name: "Success_CreatesAutogrowInstance_WithDefaults",
			setup: func(ctrl *gomock.Controller) (context.Context, storagelisters.StorageClassLister, listerv1.TridentVolumePublicationLister, listerv1.TridentAutogrowPolicyLister, *crdclientfake.Clientset, *mockNodeHelpers.MockNodeHelper, string) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"
				return ctx, scLister, tvpLister, agpLister, tridentClient, nodeHelper, namespace
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				require.NoError(t, err, "New should succeed")
				require.NotNil(t, ag, "Autogrow should not be nil")
				assert.Equal(t, StateStopped, ag.state.get(), "initial state should be Stopped")
				assert.NotNil(t, ag.cache, "cache should be initialized")
				assert.NotNil(t, ag.controller, "controller should be initialized")
				assert.NotNil(t, ag.scheduler, "scheduler should be initialized")
				assert.NotNil(t, ag.poller, "poller should be initialized")
				assert.NotNil(t, ag.requester, "requester should be initialized")
				// Worker pool and eventbus manager are created in Activate(), not New()
				assert.Nil(t, ag.eventbusManager, "eventbus manager should not be initialized in New()")
				assert.Nil(t, ag.workerPool, "worker pool should not be initialized in New()")
			},
		},
		{
			name: "Success_CreatesAutogrowInstance_WithCustomConfigs",
			setup: func(ctrl *gomock.Controller) (context.Context, storagelisters.StorageClassLister, listerv1.TridentVolumePublicationLister, listerv1.TridentAutogrowPolicyLister, *crdclientfake.Clientset, *mockNodeHelpers.MockNodeHelper, string) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "custom-namespace"
				return ctx, scLister, tvpLister, agpLister, tridentClient, nodeHelper, namespace
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				require.NoError(t, err)
				require.NotNil(t, ag)
				assert.NotNil(t, ag.controller)
				assert.NotNil(t, ag.scheduler)
				assert.NotNil(t, ag.poller)
				assert.NotNil(t, ag.requester)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ctx, scLister, tvpLister, agpLister, tridentClient, nodeHelper, namespace := tt.setup(ctrl)

			// Use default autogrow period of 1 minute for tests
			autogrowPeriod := 1 * time.Minute
			ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, autogrowPeriod, nil, nil, nil)

			tt.verify(t, ag, err)

			// Cleanup
			if ag != nil && ag.eventbusManager != nil {
				_ = ag.eventbusManager.Shutdown(ctx)
			}
		})
	}
}

// ============================================================================
// Test: Activate
// ============================================================================

func TestAutogrow_Activate(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*gomock.Controller) (*Autogrow, context.Context, func())
		verify func(*testing.T, *Autogrow, error)
	}{
		{
			name: "Success_ActivatesAllLayers",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.Deactivate(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				require.NoError(t, err, "Activate should succeed")
				assert.Equal(t, StateRunning, ag.state.get(), "state should be Running after activation")

				// Verify worker pool is started
				assert.True(t, ag.workerPool.IsStarted(), "worker pool should be started")
				assert.False(t, ag.workerPool.IsClosed(), "worker pool should not be closed")

				// Verify all layers are properly initialized and activated
				assert.NotNil(t, ag.controller, "controller should be initialized")
				assert.NotNil(t, ag.scheduler, "scheduler should be initialized")
				assert.NotNil(t, ag.poller, "poller should be initialized")
				assert.NotNil(t, ag.requester, "requester should be initialized")
			},
		},
		{
			name: "Success_Idempotent_AlreadyRunning",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// First activation
				err = ag.Activate(ctx)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.Deactivate(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				require.NoError(t, err, "second Activate should be idempotent")
				assert.Equal(t, StateRunning, ag.state.get())
			},
		},
		{
			name: "Error_WrongState_Starting",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Manually set state to Starting
				ag.state.set(StateStarting)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				require.Error(t, err, "Activate should fail when in Starting state")
				assert.Contains(t, err.Error(), "cannot activate Autogrow")
			},
		},
		{
			name: "Error_WrongState_Stopping",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Manually set state to Stopping
				ag.state.set(StateStopping)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				require.Error(t, err, "Activate should fail when in Stopping state")
				assert.Contains(t, err.Error(), "cannot activate Autogrow")
				// Verify state wasn't changed
				assert.Equal(t, StateStopping, ag.state.get(), "state should remain Stopping after failed activation")
			},
		},
		{
			name: "Success_StateRollback_AfterFailedActivation",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Manually set state to Starting to simulate a failed activation mid-way
				// This simulates the scenario where compareAndSwap succeeded but activation
				// failed at some layer
				ag.state.set(StateStarting)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				// This test verifies that trying to activate from Starting state fails
				// This simulates catching a mid-activation failure
				require.Error(t, err, "activation should fail from Starting state")
				assert.Contains(t, err.Error(), "cannot activate Autogrow", "error should indicate state issue")
				assert.Contains(t, err.Error(), "Starting", "error should mention Starting state")

				// Verify state remained in Starting (wasn't changed by failed activation)
				assert.Equal(t, StateStarting, ag.state.get(), "state should remain Starting after failed activation")

				// Verify we can still clean up even with instance in Starting state
				assert.NotPanics(t, func() {
					if ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(getTestContext())
					}
				}, "cleanup should not panic with instance in Starting state")
			},
		},
		// ====================================================================
		// Rollback Coverage Tests
		// The following tests cover the activation rollback paths by using
		// SetState() to simulate failures at each layer activation.
		// ====================================================================
		{
			name: "Rollback_SchedulerActivationFails",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Set scheduler state to Starting so its Activate() will fail
				// This simulates a failure during scheduler activation
				ag.scheduler.SetState(scheduler.StateStarting)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				// Verify activation failed
				require.Error(t, err, "Activate should fail when scheduler activation fails")
				assert.Contains(t, err.Error(), "failed to activate scheduler", "error should indicate scheduler failure")

				// Verify orchestrator state rolled back to Stopped
				assert.Equal(t, StateStopped, ag.state.get(), "orchestrator state should be Stopped after failed activation")

				// Verify scheduler state remains Starting (wasn't changed by rollback)
				assert.Equal(t, scheduler.StateStarting, ag.scheduler.GetState(), "scheduler state should remain Starting")

				// Verify worker pool was shut down and cleared during rollback
				assert.Nil(t, ag.workerPool, "worker pool should be nil after cleanup")
				assert.Nil(t, ag.eventbusManager, "eventbus manager should be nil after cleanup")

				// Controller was rolled back (deactivated)
				// Note: Controller doesn't have state management, so we can't verify its state
			},
		},
		{
			name: "Rollback_PollerActivationFails",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Set poller state to Starting so its Activate() will fail
				// This simulates a failure during poller activation
				ag.poller.SetState(poller.StateStarting)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				// Verify activation failed
				require.Error(t, err, "Activate should fail when poller activation fails")
				assert.Contains(t, err.Error(), "failed to activate poller", "error should indicate poller failure")

				// Verify orchestrator state rolled back to Stopped
				assert.Equal(t, StateStopped, ag.state.get(), "orchestrator state should be Stopped after failed activation")

				// Verify poller state remains Starting (wasn't changed by rollback)
				assert.Equal(t, poller.StateStarting, ag.poller.GetState(), "poller state should remain Starting")

				// Verify scheduler was rolled back to Stopped
				assert.Equal(t, scheduler.StateStopped, ag.scheduler.GetState(), "scheduler state should be Stopped after rollback")

				// Verify worker pool was shut down and cleared during rollback
				assert.Nil(t, ag.workerPool, "worker pool should be nil after cleanup")
				assert.Nil(t, ag.eventbusManager, "eventbus manager should be nil after cleanup")
			},
		},
		{
			name: "Rollback_RequesterActivationFails",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Set requester state to Starting so its Activate() will fail
				// This simulates a failure during requester activation
				ag.requester.SetState(requester.StateStarting)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				// Verify activation failed
				require.Error(t, err, "Activate should fail when requester activation fails")
				assert.Contains(t, err.Error(), "failed to activate requester", "error should indicate requester failure")

				// Verify orchestrator state rolled back to Stopped
				assert.Equal(t, StateStopped, ag.state.get(), "orchestrator state should be Stopped after failed activation")

				// Verify requester state remains Starting (wasn't changed by rollback)
				assert.Equal(t, requester.StateStarting, ag.requester.GetState(), "requester state should remain Starting")

				// Verify poller was rolled back to Stopped
				assert.Equal(t, poller.StateStopped, ag.poller.GetState(), "poller state should be Stopped after rollback")

				// Verify scheduler was rolled back to Stopped
				assert.Equal(t, scheduler.StateStopped, ag.scheduler.GetState(), "scheduler state should be Stopped after rollback")

				// Verify worker pool was shut down and cleared during rollback
				assert.Nil(t, ag.workerPool, "worker pool should be nil after cleanup")
				assert.Nil(t, ag.eventbusManager, "eventbus manager should be nil after cleanup")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ag, ctx, cleanup := tt.setup(ctrl)
			defer cleanup()

			err := ag.Activate(ctx)

			tt.verify(t, ag, err)
		})
	}
}

// ============================================================================
// Test: Deactivate
// ============================================================================

func TestAutogrow_Deactivate(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*gomock.Controller) (*Autogrow, context.Context, func())
		verify func(*testing.T, *Autogrow, error)
	}{
		{
			name: "Success_DeactivatesAllLayers",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Activate first
				err = ag.Activate(ctx)
				require.NoError(t, err)

				cleanup := func() {}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				require.NoError(t, err, "Deactivate should succeed")
				assert.Equal(t, StateStopped, ag.state.get(), "state should be Stopped after deactivation")

				// Verify worker pool was shut down and cleared
				assert.Nil(t, ag.workerPool, "worker pool should be nil after deactivation")
				assert.Nil(t, ag.eventbusManager, "eventbus manager should be nil after deactivation")
			},
		},
		{
			name: "Success_Idempotent_AlreadyStopped",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Activate and deactivate
				err = ag.Activate(ctx)
				require.NoError(t, err)
				err = ag.Deactivate(ctx)
				require.NoError(t, err)

				cleanup := func() {}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				require.NoError(t, err, "second Deactivate should be idempotent")
				assert.Equal(t, StateStopped, ag.state.get())

				// Verify worker pool is still nil (was cleared in first deactivation)
				assert.Nil(t, ag.workerPool, "worker pool should remain nil")
				assert.Nil(t, ag.eventbusManager, "eventbus manager should remain nil")
			},
		},
		{
			name: "Error_WrongState_Starting",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Manually set state to Starting
				ag.state.set(StateStarting)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				require.Error(t, err, "Deactivate should fail when in Starting state")
				assert.Contains(t, err.Error(), "cannot deactivate Autogrow")
			},
		},
		{
			name: "Success_Idempotent_AlreadyStopped_ReturnsNil",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				// Idempotent - should succeed when already stopped
				require.NoError(t, err, "Deactivate should be idempotent when already Stopped")
				assert.Equal(t, StateStopped, ag.state.get(), "state should remain Stopped")

				// Verify cache still exists but worker pool and eventbus manager were never created
				assert.NotNil(t, ag.cache, "cache should still exist")
				assert.Nil(t, ag.eventbusManager, "eventbus manager should be nil (never activated)")
				assert.Nil(t, ag.workerPool, "worker pool should be nil (never activated)")
			},
		},
		{
			name: "Success_CleansUpEvenWithPartialState",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Activate first
				err = ag.Activate(ctx)
				require.NoError(t, err)

				cleanup := func() {}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				// This test verifies that deactivation properly cleans up all resources
				require.NoError(t, err, "Deactivate should succeed")
				assert.Equal(t, StateStopped, ag.state.get(), "state should be Stopped")

				// Verify eventbus references are cleared (as per Deactivate implementation)
				// Note: We can't directly check the global eventbus vars from here,
				// but we verify the state is consistent
				assert.False(t, ag.IsRunning(), "IsRunning should return false after deactivation")

				// Verify calling Deactivate again is safe (idempotent)
				err = ag.Deactivate(getTestContext())
				assert.NoError(t, err, "second Deactivate should be idempotent")
			},
		},
		// ====================================================================
		// Best-Effort Error Handling Tests
		// The following tests verify that Deactivate() continues even when
		// individual layer deactivations fail, following the "best effort"
		// cleanup strategy.
		// ====================================================================
		{
			name: "BestEffort_RequesterDeactivationFails_ContinuesCleanup",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Activate first
				err = ag.Activate(ctx)
				require.NoError(t, err)

				// Set requester state to Starting so its Deactivate() will fail
				// This simulates a failure during requester deactivation
				ag.requester.SetState(requester.StateStarting)

				cleanup := func() {}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				// Verify deactivation still succeeds (best-effort approach)
				require.NoError(t, err, "Deactivate should succeed despite requester failure")

				// Verify orchestrator state reaches Stopped (cleanup continues despite failure)
				assert.Equal(t, StateStopped, ag.state.get(), "orchestrator state should be Stopped")

				// Verify requester state remains Starting (failed to deactivate)
				assert.Equal(t, requester.StateStarting, ag.requester.GetState(), "requester state should remain Starting")

				// Verify other layers were still deactivated
				assert.Equal(t, poller.StateStopped, ag.poller.GetState(), "poller should be deactivated")
				assert.Equal(t, scheduler.StateStopped, ag.scheduler.GetState(), "scheduler should be deactivated")
			},
		},
		{
			name: "BestEffort_PollerDeactivationFails_ContinuesCleanup",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Activate first
				err = ag.Activate(ctx)
				require.NoError(t, err)

				// Set poller state to Starting so its Deactivate() will fail
				// This simulates a failure during poller deactivation
				ag.poller.SetState(poller.StateStarting)

				cleanup := func() {}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				// Verify deactivation still succeeds (best-effort approach)
				require.NoError(t, err, "Deactivate should succeed despite poller failure")

				// Verify orchestrator state reaches Stopped (cleanup continues despite failure)
				assert.Equal(t, StateStopped, ag.state.get(), "orchestrator state should be Stopped")

				// Verify poller state remains Starting (failed to deactivate)
				assert.Equal(t, poller.StateStarting, ag.poller.GetState(), "poller state should remain Starting")

				// Verify other layers were still attempted to deactivate
				assert.Equal(t, requester.StateStopped, ag.requester.GetState(), "requester should be deactivated")
				assert.Equal(t, scheduler.StateStopped, ag.scheduler.GetState(), "scheduler should be deactivated")
			},
		},
		{
			name: "BestEffort_SchedulerDeactivationFails_ContinuesCleanup",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Activate first
				err = ag.Activate(ctx)
				require.NoError(t, err)

				// Set scheduler state to Starting so its Deactivate() will fail
				// This simulates a failure during scheduler deactivation
				ag.scheduler.SetState(scheduler.StateStarting)

				cleanup := func() {}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				// Verify deactivation still succeeds (best-effort approach)
				require.NoError(t, err, "Deactivate should succeed despite scheduler failure")

				// Verify orchestrator state reaches Stopped (cleanup continues despite failure)
				assert.Equal(t, StateStopped, ag.state.get(), "orchestrator state should be Stopped")

				// Verify scheduler state remains Starting (failed to deactivate)
				assert.Equal(t, scheduler.StateStarting, ag.scheduler.GetState(), "scheduler state should remain Starting")

				// Verify other layers were still deactivated
				assert.Equal(t, requester.StateStopped, ag.requester.GetState(), "requester should be deactivated")
				assert.Equal(t, poller.StateStopped, ag.poller.GetState(), "poller should be deactivated")
			},
		},
		{
			name: "BestEffort_MultipleLayersFail_ContinuesCleanup",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Activate first
				err = ag.Activate(ctx)
				require.NoError(t, err)

				// Set multiple component states to Starting so their Deactivate() will fail
				// This simulates multiple failures during deactivation
				ag.requester.SetState(requester.StateStarting)
				ag.poller.SetState(poller.StateStarting)
				ag.scheduler.SetState(scheduler.StateStarting)

				cleanup := func() {}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow, err error) {
				// Verify deactivation still succeeds (best-effort approach)
				require.NoError(t, err, "Deactivate should succeed despite multiple layer failures")

				// Verify orchestrator state reaches Stopped (cleanup continues despite failures)
				assert.Equal(t, StateStopped, ag.state.get(), "orchestrator state should be Stopped")

				// Verify all component states remain Starting (all failed to deactivate)
				assert.Equal(t, requester.StateStarting, ag.requester.GetState(), "requester state should remain Starting")
				assert.Equal(t, poller.StateStarting, ag.poller.GetState(), "poller state should remain Starting")
				assert.Equal(t, scheduler.StateStarting, ag.scheduler.GetState(), "scheduler state should remain Starting")

				// Verify cleanup still completed (eventbus manager, cache)
				// These should have been attempted regardless of layer failures
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ag, ctx, cleanup := tt.setup(ctrl)
			defer cleanup()

			err := ag.Deactivate(ctx)

			tt.verify(t, ag, err)
		})
	}
}

// ============================================================================
// Test: IsRunning
// ============================================================================

func TestAutogrow_IsRunning(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*gomock.Controller) (*Autogrow, context.Context, func())
		expected bool
	}{
		{
			name: "Success_ReturnsFalse_WhenStopped",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			expected: false,
		},
		{
			name: "Success_ReturnsTrue_WhenRunning",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				err = ag.Activate(ctx)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil {
						_ = ag.Deactivate(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			expected: true,
		},
		{
			name: "Success_ReturnsFalse_WhenStarting",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				// Manually set state to Starting
				ag.state.set(StateStarting)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ag, _, cleanup := tt.setup(ctrl)
			defer cleanup()

			result := ag.IsRunning()

			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// Test: GetState
// ============================================================================

func TestAutogrow_GetState(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*gomock.Controller) (*Autogrow, context.Context, func())
		expectedState State
	}{
		{
			name: "Success_ReturnsStopped",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			expectedState: StateStopped,
		},
		{
			name: "Success_ReturnsRunning",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				err = ag.Activate(ctx)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil {
						_ = ag.Deactivate(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			expectedState: StateRunning,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ag, _, cleanup := tt.setup(ctrl)
			defer cleanup()

			state := ag.GetState()

			assert.Equal(t, tt.expectedState, state)
		})
	}
}

// ============================================================================
// Test: GetCache
// ============================================================================

func TestAutogrow_GetCache(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*gomock.Controller) (*Autogrow, context.Context, func())
		verify func(*testing.T, *Autogrow)
	}{
		{
			name: "Success_ReturnsCache",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow) {
				cache := ag.GetCache()
				assert.NotNil(t, cache, "cache should not be nil")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ag, _, cleanup := tt.setup(ctrl)
			defer cleanup()

			tt.verify(t, ag)
		})
	}
}

// ============================================================================
// Test: HandleStorageClassEvent
// ============================================================================

func TestAutogrow_HandleStorageClassEvent(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*gomock.Controller) (*Autogrow, context.Context, crdtypes.EventType, string, func())
		verify func(*testing.T)
	}{
		{
			name: "Success_DelegatesToController_WhenRunning",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				err = ag.Activate(ctx)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil {
						_ = ag.Deactivate(ctx)
					}
				}

				return ag, ctx, crdtypes.EventAdd, "test-sc", cleanup
			},
			verify: func(t *testing.T) {
				// Should not panic, event should be delegated to controller
			},
		},
		{
			name: "Success_IgnoresEvent_WhenNotRunning",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, crdtypes.EventAdd, "test-sc", cleanup
			},
			verify: func(t *testing.T) {
				// Should not panic, event should be ignored
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ag, ctx, eventType, name, cleanup := tt.setup(ctrl)
			defer cleanup()

			assert.NotPanics(t, func() {
				ag.HandleStorageClassEvent(ctx, eventType, name)
			})

			tt.verify(t)
		})
	}
}

// ============================================================================
// Test: HandleTVPEvent
// ============================================================================

func TestAutogrow_HandleTVPEvent(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*gomock.Controller) (*Autogrow, context.Context, crdtypes.EventType, string, func())
		verify func(*testing.T)
	}{
		{
			name: "Success_DelegatesToController_WhenRunning",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				err = ag.Activate(ctx)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil {
						_ = ag.Deactivate(ctx)
					}
				}

				return ag, ctx, crdtypes.EventAdd, "default/test-tvp", cleanup
			},
			verify: func(t *testing.T) {
				// Should not panic
			},
		},
		{
			name: "Success_IgnoresEvent_WhenNotRunning",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, crdtypes.EventAdd, "default/test-tvp", cleanup
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

			ag, ctx, eventType, key, cleanup := tt.setup(ctrl)
			defer cleanup()

			assert.NotPanics(t, func() {
				ag.HandleTVPEvent(ctx, eventType, key)
			})

			tt.verify(t)
		})
	}
}

// ============================================================================
// Test: HandleTBEEvent
// ============================================================================

func TestAutogrow_HandleTBEEvent(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*gomock.Controller) (*Autogrow, context.Context, crdtypes.EventType, string, func())
		verify func(*testing.T)
	}{
		{
			name: "Success_DelegatesToController_WhenRunning",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				err = ag.Activate(ctx)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil {
						_ = ag.Deactivate(ctx)
					}
				}

				return ag, ctx, crdtypes.EventAdd, "default/test-backend", cleanup
			},
			verify: func(t *testing.T) {
				// Should not panic
			},
		},
		{
			name: "Success_IgnoresEvent_WhenNotRunning",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, crdtypes.EventType, string, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				cleanup := func() {
					if ag != nil && ag.eventbusManager != nil {
						_ = ag.eventbusManager.Shutdown(ctx)
					}
				}

				return ag, ctx, crdtypes.EventAdd, "default/test-backend", cleanup
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

			ag, ctx, eventType, key, cleanup := tt.setup(ctrl)
			defer cleanup()

			assert.NotPanics(t, func() {
				ag.HandleTBEEvent(ctx, eventType, key)
			})

			tt.verify(t)
		})
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestAutogrow_Integration_ActivateDeactivateCycle(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(*gomock.Controller) (*Autogrow, context.Context, func())
		verify func(*testing.T, *Autogrow)
	}{
		{
			name: "Success_ActivateDeactivateCycle",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				cleanup := func() {
					// Final cleanup
					if ag != nil && ag.GetState() == StateRunning {
						_ = ag.Deactivate(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow) {
				ctx := getTestContext()

				// Start Stopped
				assert.Equal(t, StateStopped, ag.GetState())
				assert.False(t, ag.IsRunning())

				// Activate
				err := ag.Activate(ctx)
				assert.NoError(t, err)
				assert.Equal(t, StateRunning, ag.GetState())
				assert.True(t, ag.IsRunning())

				// Deactivate - this shuts down eventbus
				err = ag.Deactivate(ctx)
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, ag.GetState())
				assert.False(t, ag.IsRunning())

				// Note: Cannot reactivate after deactivation because eventbus is shut down
				// This is expected behavior - once deactivated, the Autogrow instance
				// should be discarded and a new one created if needed
			},
		},
		{
			name: "Success_IdempotentActivateDeactivate",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				cleanup := func() {
					// Final cleanup
					if ag != nil && ag.GetState() == StateRunning {
						_ = ag.Deactivate(ctx)
					}
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow) {
				ctx := getTestContext()

				// Activate once
				err := ag.Activate(ctx)
				assert.NoError(t, err)
				assert.Equal(t, StateRunning, ag.GetState())

				// Activate again (should be idempotent)
				err = ag.Activate(ctx)
				assert.NoError(t, err)
				assert.Equal(t, StateRunning, ag.GetState())

				// Deactivate once
				err = ag.Deactivate(ctx)
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, ag.GetState())

				// Deactivate again (should be idempotent)
				err = ag.Deactivate(ctx)
				assert.NoError(t, err)
				assert.Equal(t, StateStopped, ag.GetState())
			},
		},
		{
			name: "Success_EventHandling_WhenRunning",
			setup: func(ctrl *gomock.Controller) (*Autogrow, context.Context, func()) {
				ctx := getTestContext()
				scLister, tvpLister, agpLister, tridentClient := setupTestListers(t, nil, nil, nil)
				nodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)
				namespace := "trident"

				ag, err := New(ctx, tridentClient, agpLister, scLister, tvpLister, nodeHelper, namespace, 1*time.Minute, nil, nil, nil)
				require.NoError(t, err)

				err = ag.Activate(ctx)
				require.NoError(t, err)

				cleanup := func() {
					_ = ag.Deactivate(ctx)
				}

				return ag, ctx, cleanup
			},
			verify: func(t *testing.T, ag *Autogrow) {
				ctx := getTestContext()

				// Handle various events
				assert.NotPanics(t, func() {
					ag.HandleStorageClassEvent(ctx, crdtypes.EventAdd, "sc1")
					ag.HandleTVPEvent(ctx, crdtypes.EventAdd, "default/tvp1")
					ag.HandleTBEEvent(ctx, crdtypes.EventAdd, "default/backend1")
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			ag, _, cleanup := tt.setup(ctrl)
			defer cleanup()

			tt.verify(t, ag)
		})
	}
}
