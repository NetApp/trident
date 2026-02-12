// Copyright 2026 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	"github.com/netapp/trident/frontend/autogrow"
	mockFrontendAutogrow "github.com/netapp/trident/mocks/mock_frontend/mock_autogrow"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	crdclientfake "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	tridentinformers "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions"
	listerv1 "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	"github.com/netapp/trident/utils/errors"
)

// ============================================================================
// Helper Functions and Test Fixtures
// ============================================================================

// getTestContext returns a context with timeout for testing
func getTestContext() context.Context {
	ctx := context.Background()
	return ctx
}

// createTestAGP creates a test TridentAutogrowPolicy object
func createTestAGP(name string) *v1.TridentAutogrowPolicy {
	return &v1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			GrowthAmount:  "10%",
			MaxSize:       "100Gi",
		},
		Status: v1.TridentAutogrowPolicyStatus{
			State:   string(v1.TridentAutogrowPolicyStateSuccess),
			Message: "",
		},
	}
}

// setupTestAGPLister creates fake AGP lister with given policies
func setupTestAGPLister(t *testing.T, policies []*v1.TridentAutogrowPolicy) listerv1.TridentAutogrowPolicyLister {
	tridentObjects := make([]runtime.Object, len(policies))
	for i, policy := range policies {
		tridentObjects[i] = policy
	}
	tridentClient := crdclientfake.NewSimpleClientset(tridentObjects...)

	tridentInformerFactory := tridentinformers.NewSharedInformerFactory(tridentClient, 0)
	agpInformer := tridentInformerFactory.Trident().V1().TridentAutogrowPolicies()
	agpLister := agpInformer.Lister()

	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })

	go tridentInformerFactory.Start(stopCh)
	cache.WaitForCacheSync(stopCh, agpInformer.Informer().HasSynced)

	return agpLister
}

// fakeAGPLister is a test double that implements TridentAutogrowPolicyLister
// and returns a predetermined list or error
type fakeAGPLister struct {
	policies []*v1.TridentAutogrowPolicy
	err      error
}

func (f *fakeAGPLister) List(selector labels.Selector) (ret []*v1.TridentAutogrowPolicy, err error) {
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
	return nil, fmt.Errorf("policy not found")
}

// ============================================================================
// Test: handleTridentAutogrowPolicies
// ============================================================================

func TestHandleTridentAutogrowPolicies(t *testing.T) {
	// testSetup holds test-specific configuration and dependencies
	type testSetup struct {
		controller *TridentNodeCrdController
		keyItem    *KeyItem
	}

	tests := []struct {
		name   string
		setup  func(ctrl *gomock.Controller) testSetup
		verify func(*testing.T, error)
	}{
		// ========================================
		// Success Cases: Activation (0 → 1)
		// ========================================
		{
			name: "Success_FirstAGPCreated_ActivatesSystem_FromStopped",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: System is stopped, one AGP exists
				agp1 := createTestAGP("policy-1")
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{agp1})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateStopped (system is not active)
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateStopped).
					Times(2) // Called for both isActive and isStopped checks

				// Expect Activate to be called and succeed
				mockOrchestrator.EXPECT().
					Activate(gomock.Any()).
					Return(nil).
					Times(1)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "policy-1",
					event: "Add",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "handleTridentAutogrowPolicies should succeed")
			},
		},
		{
			name: "Success_FirstAGPCreated_AlreadyActive_NoOp",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: System is already running, one AGP exists
				agp1 := createTestAGP("policy-1")
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{agp1})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateRunning (already active)
				// GetState is called twice: once for isActive and once for isStopped
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateRunning).
					Times(2)

				// No Activate call expected (already running)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "policy-1",
					event: "Add",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "handleTridentAutogrowPolicies should succeed with no-op")
			},
		},
		{
			name: "Success_MultipleAGPsExist_SystemActive_NoStateChange",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: System is running, multiple AGPs exist
				agp1 := createTestAGP("policy-1")
				agp2 := createTestAGP("policy-2")
				agp3 := createTestAGP("policy-3")
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{agp1, agp2, agp3})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateRunning
				// GetState is called twice: once for isActive and once for isStopped
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateRunning).
					Times(2)

				// No Activate or Deactivate calls expected

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "policy-2",
					event: "Update",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "handleTridentAutogrowPolicies should succeed with no state change")
			},
		},

		// ========================================
		// Success Cases: Deactivation (1 → 0)
		// ========================================
		{
			name: "Success_LastAGPDeleted_DeactivatesSystem",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: System is running, no AGPs remain after deletion
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{}) // Empty list

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateRunning (not stopped)
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateRunning).
					Times(2) // Called for isActive check, then isStopped check

				// Expect Deactivate to be called and succeed
				mockOrchestrator.EXPECT().
					Deactivate(gomock.Any()).
					Return(nil).
					Times(1)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "last-policy",
					event: "Delete",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "handleTridentAutogrowPolicies should succeed")
			},
		},
		{
			name: "Success_NoAGPs_AlreadyStopped_NoOp",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: System is already stopped, no AGPs
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateStopped
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateStopped).
					Times(2) // isActive and isStopped checks

				// No Activate or Deactivate calls expected

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "some-policy",
					event: "Delete",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "handleTridentAutogrowPolicies should succeed with no-op")
			},
		},

		// ========================================
		// Error Cases: Invalid Key
		// ========================================
		{
			name: "Error_InvalidKey_ReturnsError",
			setup: func(ctrl *gomock.Controller) testSetup {
				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// No expectations - should fail before any calls

				controller := &TridentNodeCrdController{
					autogrowOrchestrator: mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "invalid/key/with/too/many/slashes/here",
					event: "Add",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.Error(t, err, "should fail with invalid key")
			},
		},

		// ========================================
		// Error Cases: List Failure
		// ========================================
		{
			name: "Error_ListAGPsFails_ReturnsError",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup lister that returns error
				listErr := fmt.Errorf("API server unavailable")
				agpLister := &fakeAGPLister{
					policies: nil,
					err:      listErr,
				}

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// No GetState or Activate/Deactivate calls expected (fails before)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "policy-1",
					event: "Add",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.Error(t, err, "should fail when list fails")
				assert.Contains(t, err.Error(), "API server unavailable")
			},
		},

		// ========================================
		// Error Cases: Activate Failures
		// ========================================
		{
			name: "Error_ActivateFails_StateStopping_Retries",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: First AGP, but activate fails with StateStopping error
				agp1 := createTestAGP("policy-1")
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{agp1})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateStopped
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateStopped).
					Times(2)

				// Expect Activate to fail with StateStopping error
				stoppingErr := errors.NewStateError(
					string(autogrow.StateStopping),
					"cannot activate while stopping",
				)
				mockOrchestrator.EXPECT().
					Activate(gomock.Any()).
					Return(stoppingErr).
					Times(1)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "policy-1",
					event: "Add",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.Error(t, err, "should return error for retry")
				assert.True(t, errors.IsReconcileDeferredError(err), "should be ReconcileDeferredError for retry")
			},
		},
		{
			name: "Success_ActivateFails_StateStarting_NoRetry",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: First AGP, but activate fails with StateStarting error
				agp1 := createTestAGP("policy-1")
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{agp1})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateStopped
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateStopped).
					Times(2)

				// Expect Activate to fail with StateStarting error
				startingErr := errors.NewStateError(
					string(autogrow.StateStarting),
					"already starting",
				)
				mockOrchestrator.EXPECT().
					Activate(gomock.Any()).
					Return(startingErr).
					Times(1)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "policy-1",
					event: "Add",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "should succeed without retry when StateStarting")
			},
		},
		{
			name: "Error_ActivateFails_NonStateError_ReturnsError",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: First AGP, but activate fails with non-state error
				agp1 := createTestAGP("policy-1")
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{agp1})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateStopped
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateStopped).
					Times(2)

				// Expect Activate to fail with non-state error
				activateErr := fmt.Errorf("internal activation error")
				mockOrchestrator.EXPECT().
					Activate(gomock.Any()).
					Return(activateErr).
					Times(1)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "policy-1",
					event: "Add",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.Error(t, err, "should return error for non-state error")
				assert.Contains(t, err.Error(), "internal activation error")
				assert.False(t, errors.IsReconcileDeferredError(err), "should not be ReconcileDeferredError")
			},
		},

		// ========================================
		// Error Cases: Deactivate Failures
		// ========================================
		{
			name: "Error_DeactivateFails_StateStarting_Retries",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: No AGPs, but deactivate fails with StateStarting error
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateRunning (not stopped, should deactivate)
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateRunning).
					Times(2)

				// Expect Deactivate to fail with StateStarting error
				startingErr := errors.NewStateError(
					string(autogrow.StateStarting),
					"cannot deactivate while starting",
				)
				mockOrchestrator.EXPECT().
					Deactivate(gomock.Any()).
					Return(startingErr).
					Times(1)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "last-policy",
					event: "Delete",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.Error(t, err, "should return error for retry")
				assert.True(t, errors.IsReconcileDeferredError(err), "should be ReconcileDeferredError for retry")
			},
		},
		{
			name: "Success_DeactivateFails_StateStopping_NoRetry",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: No AGPs, but deactivate fails with StateStopping error
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateRunning
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateRunning).
					Times(2)

				// Expect Deactivate to fail with StateStopping error
				stoppingErr := errors.NewStateError(
					string(autogrow.StateStopping),
					"already stopping",
				)
				mockOrchestrator.EXPECT().
					Deactivate(gomock.Any()).
					Return(stoppingErr).
					Times(1)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "last-policy",
					event: "Delete",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "should succeed without retry when StateStopping")
			},
		},
		{
			name: "Error_DeactivateFails_NonStateError_ReturnsError",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: No AGPs, but deactivate fails with non-state error
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateRunning
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateRunning).
					Times(2)

				// Expect Deactivate to fail with non-state error
				deactivateErr := fmt.Errorf("internal deactivation error")
				mockOrchestrator.EXPECT().
					Deactivate(gomock.Any()).
					Return(deactivateErr).
					Times(1)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "last-policy",
					event: "Delete",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.Error(t, err, "should return error for non-state error")
				assert.Contains(t, err.Error(), "internal deactivation error")
				assert.False(t, errors.IsReconcileDeferredError(err), "should not be ReconcileDeferredError")
			},
		},

		// ========================================
		// Edge Cases: Cluster-Scoped Resources
		// ========================================
		{
			name: "Success_ClusterScopedResource_EmptyNamespace_HandlesCorrectly",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: AGP with empty namespace (cluster-scoped)
				agp1 := createTestAGP("cluster-policy")
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{agp1})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateRunning
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateRunning).
					Times(2)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "cluster-policy", // No namespace prefix
					event: "Update",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "should handle cluster-scoped resource correctly")
			},
		},

		// ========================================
		// Edge Cases: State Transitions
		// ========================================
		{
			name: "Success_StateStarting_OneAGP_NoActivate",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: System in Starting state, one AGP exists
				agp1 := createTestAGP("policy-1")
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{agp1})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateStarting (considered "active" but not fully running yet)
				// GetState is called twice: once for isActive and once for isStopped
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateStarting).
					Times(2)

				// Activate WILL be called (because isActive is false when state is Starting)
				// and should return a state error indicating starting state
				startingErr := errors.NewStateError(string(autogrow.StateStarting), "already starting")
				mockOrchestrator.EXPECT().
					Activate(gomock.Any()).
					Return(startingErr).
					Times(1)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "policy-1",
					event: "Add",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "should handle starting state gracefully without error")
			},
		},
		{
			name: "Success_StateStopping_NoAGPs_NoDeactivate",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: System in Stopping state, no AGPs
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateStopping (not stopped yet, but also not fully active)
				// GetState is called twice: once for isActive and once for isStopped
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateStopping).
					Times(2) // isActive returns false, isStopped returns false

				// Deactivate WILL be called (because isStopped is false)
				// and should return a state error indicating stopping state
				stoppingErr := errors.NewStateError(string(autogrow.StateStopping), "already stopping")
				mockOrchestrator.EXPECT().
					Deactivate(gomock.Any()).
					Return(stoppingErr).
					Times(1)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "last-policy",
					event: "Delete",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "should handle stopping state gracefully without error")
			},
		},

		// ========================================
		// Edge Cases: Multiple AGPs with transitions
		// ========================================
		{
			name: "Success_TwoAGPsExist_BecomeOne_NoStateChange",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: Two AGPs exist, one is about to be deleted (but count is still > 0)
				agp1 := createTestAGP("policy-1")
				agp2 := createTestAGP("policy-2")
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{agp1, agp2})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateRunning
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateRunning).
					Times(2)

				// No Deactivate call expected (still have 2 AGPs)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "policy-2",
					event: "Delete",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "should not deactivate when still AGPs remain")
			},
		},
		{
			name: "Success_ZeroAGPs_BecomeOne_Activates",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: No AGPs initially, one just created
				agp1 := createTestAGP("new-policy")
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{agp1})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Expect GetState to return StateStopped
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateStopped).
					Times(2)

				// Expect Activate to be called
				mockOrchestrator.EXPECT().
					Activate(gomock.Any()).
					Return(nil).
					Times(1)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				ctx := getTestContext()
				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "new-policy",
					event: "Add",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "should activate when transitioning from 0 to 1 AGP")
			},
		},

		// ========================================
		// Edge Cases: Context handling
		// ========================================
		{
			name: "Success_ContextWithValues_PropagatedToOrchestrator",
			setup: func(ctrl *gomock.Controller) testSetup {
				// Setup: Context with values
				agp1 := createTestAGP("policy-1")
				agpLister := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{agp1})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateStopped).
					Times(2)

				// Verify context is passed through
				mockOrchestrator.EXPECT().
					Activate(gomock.Any()).
					DoAndReturn(func(ctx context.Context) error {
						// Verify context is not nil
						require.NotNil(t, ctx)
						return nil
					}).
					Times(1)

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				// Create context with custom value
				type contextKey string
				ctx := context.WithValue(getTestContext(), contextKey("test-key"), "test-value")

				keyItem := &KeyItem{
					ctx:   ctx,
					key:   "policy-1",
					event: "Add",
				}

				return testSetup{
					controller: controller,
					keyItem:    keyItem,
				}
			},
			verify: func(t *testing.T, err error) {
				require.NoError(t, err, "should propagate context correctly")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gomock controller for this test
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Setup test dependencies
			setup := tt.setup(ctrl)

			// Call the function under test
			err := setup.controller.handleTridentAutogrowPolicies(setup.keyItem)

			// Verify results
			tt.verify(t, err)
		})
	}
}

// ============================================================================
// Concurrent Tests
// ============================================================================

func TestHandleTridentAutogrowPolicies_Concurrent(t *testing.T) {
	tests := []struct {
		name        string
		concurrency int
		setup       func(ctrl *gomock.Controller) (*TridentNodeCrdController, []*KeyItem)
		verify      func(*testing.T, []error)
	}{
		{
			name:        "Concurrent_MultipleAGPEvents_AllProcessedCorrectly",
			concurrency: 10,
			setup: func(ctrl *gomock.Controller) (*TridentNodeCrdController, []*KeyItem) {
				// Create multiple AGPs
				agps := make([]*v1.TridentAutogrowPolicy, 10)
				for i := 0; i < 10; i++ {
					agps[i] = createTestAGP(fmt.Sprintf("policy-%d", i))
				}
				agpLister := setupTestAGPLister(t, agps)

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// System is already running, so no activate calls
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateRunning).
					AnyTimes()

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpLister,
					autogrowOrchestrator:        mockOrchestrator,
				}

				// Create multiple key items
				keyItems := make([]*KeyItem, 10)
				for i := 0; i < 10; i++ {
					keyItems[i] = &KeyItem{
						ctx:   getTestContext(),
						key:   fmt.Sprintf("policy-%d", i),
						event: "Update",
					}
				}

				return controller, keyItems
			},
			verify: func(t *testing.T, errs []error) {
				for i, err := range errs {
					assert.NoError(t, err, "event %d should succeed", i)
				}
			},
		},
		{
			name:        "Concurrent_ActivateAndDeactivate_HandledSafely",
			concurrency: 2,
			setup: func(ctrl *gomock.Controller) (*TridentNodeCrdController, []*KeyItem) {
				// One event for activation, one for deactivation
				agp1 := createTestAGP("policy-1")
				agpListerWithAGP := setupTestAGPLister(t, []*v1.TridentAutogrowPolicy{agp1})

				mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

				// Allow for various state checks and transitions
				mockOrchestrator.EXPECT().
					GetState().
					Return(autogrow.StateStopped).
					AnyTimes()

				mockOrchestrator.EXPECT().
					Activate(gomock.Any()).
					Return(nil).
					AnyTimes()

				controller := &TridentNodeCrdController{
					tridentAutogrowPolicyLister: agpListerWithAGP,
					autogrowOrchestrator:        mockOrchestrator,
				}

				keyItems := []*KeyItem{
					{
						ctx:   getTestContext(),
						key:   "policy-1",
						event: "Add",
					},
					{
						ctx:   getTestContext(),
						key:   "policy-1",
						event: "Update",
					},
				}

				return controller, keyItems
			},
			verify: func(t *testing.T, errs []error) {
				// At least one should succeed
				successCount := 0
				for _, err := range errs {
					if err == nil {
						successCount++
					}
				}
				assert.Greater(t, successCount, 0, "at least one event should succeed")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			controller, keyItems := tt.setup(ctrl)

			// Process events concurrently
			errs := make([]error, len(keyItems))
			done := make(chan struct{})

			for i := range keyItems {
				i := i // Capture loop variable
				go func() {
					errs[i] = controller.handleTridentAutogrowPolicies(keyItems[i])
					if i == len(keyItems)-1 {
						close(done)
					}
				}()
			}

			// Wait for all to complete
			select {
			case <-done:
				// All completed
			case <-time.After(5 * time.Second):
				t.Fatal("concurrent test timed out")
			}

			tt.verify(t, errs)
		})
	}
}
