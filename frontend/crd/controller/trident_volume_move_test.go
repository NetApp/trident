// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktesting "k8s.io/client-go/testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mockcore "github.com/netapp/trident/mocks/mock_core"
	mockindexers "github.com/netapp/trident/mocks/mock_frontend/crd/controller/indexers"
	mockindexer "github.com/netapp/trident/mocks/mock_frontend/crd/controller/indexers/indexer"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	crdclientfake "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

func TestIsTerminalVolumeMoveError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		terminal bool
	}{
		{
			name:     "nil error is not terminal",
			err:      nil,
			terminal: false,
		},
		{
			name:     "plain fmt.Errorf is not terminal",
			err:      fmt.Errorf("transient: connection refused"),
			terminal: false,
		},
		{
			name:     "ReconcileDeferredError is not terminal",
			err:      errors.ReconcileDeferredError("processErr me"),
			terminal: false,
		},
		{
			name:     "InvalidInputError is terminal",
			err:      errors.InvalidInputError("ONTAP rejected volume move request"),
			terminal: true,
		},
		{
			name:     "NotFoundError is terminal",
			err:      errors.NotFoundError("volume not found"),
			terminal: true,
		},
		{
			name:     "VolumeStateError is terminal",
			err:      errors.VolumeStateError("volume offline"),
			terminal: true,
		},
		{
			name:     "AuthError is terminal",
			err:      errors.AuthError("authentication failed"),
			terminal: true,
		},
		{
			name:     "TerminalReconciliationError is terminal",
			err:      errors.TerminalReconciliationError("ONTAP volume move job failed: some reason"),
			terminal: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isTerminalVolumeMoveError(tt.err)
			assert.Equal(t, tt.terminal, got, "unexpected result for error: %v", tt.err)
		})
	}
}

// TestIsTerminalVolumeMoveError_OntapErrorPropagation verifies that the error
// types produced by RestClient.VolumeMove (InvalidInputError for 4xx, NotFoundError
// for missing volume) are correctly classified as terminal by the controller.
func TestIsTerminalVolumeMoveError_OntapErrorPropagation(t *testing.T) {
	// Simulate what ontap_rest.go wraps 4xx ONTAP rejections into.
	err4xx := errors.InvalidInputError("ONTAP rejected volume move request: [PATCH /storage/volumes/{uuid}][400] ...")
	assert.True(t, isTerminalVolumeMoveError(err4xx), "4xx ONTAP error must be terminal")

	// Simulate what ontap_rest.go returns when the volume is missing during the lookup.
	errNotFound := errors.NotFoundError("could not find volume with name vol-abc")
	assert.True(t, isTerminalVolumeMoveError(errNotFound), "volume not found must be terminal")

	// Simulate a raw network error from the REST client (not a typed error).
	// This must be treated as transient so the reconciler requeues.
	errNetwork := fmt.Errorf("dial tcp: connection refused")
	assert.False(t, isTerminalVolumeMoveError(errNetwork), "network error must not be terminal")
}

func TestShouldDeleteTVM(t *testing.T) {
	t.Parallel()

	now := metav1.Now()
	sixMin := &metav1.Duration{Duration: 6 * time.Minute}
	completionRecent := metav1.NewTime(now.Add(-1 * time.Minute))
	completionElapsed := metav1.NewTime(now.Add(-10 * time.Minute))

	tests := []struct {
		name             string
		tvm              *netappv1.TridentVolumeMove
		wantDelete       bool
		wantDeferred     bool
		wantRequeueAfter time.Duration
	}{
		{
			name:       "nil TVM",
			tvm:        nil,
			wantDelete: false,
		},
		{
			name:       "non-succeeded state",
			tvm:        tvmForShouldDelete(models.VolumeMoveStateFailed, &completionRecent, sixMin),
			wantDelete: false,
		},
		{
			name:       "unset deleteAfterSuccess",
			tvm:        tvmForShouldDelete(models.VolumeMoveStateSucceeded, &completionRecent, nil),
			wantDelete: false,
		},
		{
			name:       "unset completion time",
			tvm:        tvmForShouldDelete(models.VolumeMoveStateSucceeded, nil, sixMin),
			wantDelete: false,
		},
		{
			name:       "zero duration deletes immediately",
			tvm:        tvmForShouldDelete(models.VolumeMoveStateSucceeded, &completionRecent, &metav1.Duration{}),
			wantDelete: true,
		},
		{
			name: "negative duration deletes immediately",
			tvm: tvmForShouldDelete(models.VolumeMoveStateSucceeded, &completionRecent,
				&metav1.Duration{Duration: -1 * time.Second}),
			wantDelete: true,
		},
		{
			name:       "retention elapsed",
			tvm:        tvmForShouldDelete(models.VolumeMoveStateSucceeded, &completionElapsed, sixMin),
			wantDelete: true,
		},
		{
			name:             "retention not elapsed requeues with remaining duration",
			tvm:              tvmForShouldDelete(models.VolumeMoveStateSucceeded, &completionRecent, sixMin),
			wantDelete:       false,
			wantDeferred:     true,
			wantRequeueAfter: 5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			gotDelete, err := shouldDeleteTVM(tt.tvm)
			assert.Equal(t, tt.wantDelete, gotDelete)

			if tt.wantDeferred {
				require.Error(t, err)
				assert.True(t, errors.IsReconcileDeferredWithDuration(err))
				gotDuration, ok := errors.ReconcileDeferredWithDurationValue(err)
				require.True(t, ok)
				// Allow sub-second drift from time.Since in shouldDeleteTVM.
				assert.InDelta(t, tt.wantRequeueAfter.Seconds(), gotDuration.Seconds(), 1.0)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func tvmForShouldDelete(
	state models.VolumeMoveState, completionTime *metav1.Time, deleteAfterSuccess *metav1.Duration,
) *netappv1.TridentVolumeMove {
	return &netappv1.TridentVolumeMove{
		Spec: netappv1.TridentVolumeMoveSpec{
			DeleteAfterSuccess: deleteAfterSuccess,
		},
		Status: netappv1.TridentVolumeMoveStatus{
			State:          state,
			CompletionTime: completionTime,
		},
	}
}

func TestValidateDeleteAfterSuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		d          func() *time.Duration
		assertFunc assert.ErrorAssertionFunc
	}{
		{
			name:       "nil is valid",
			d:          func() *time.Duration { return nil },
			assertFunc: assert.NoError,
		},
		{
			name:       "zero is valid",
			d:          func() *time.Duration { return new(time.Duration) },
			assertFunc: assert.NoError,
		},
		{
			name:       "positive is valid",
			d:          func() *time.Duration { return new(time.Duration(10) * time.Second) },
			assertFunc: assert.NoError,
		},
		{
			name:       "negative is invalid",
			d:          func() *time.Duration { return new(time.Duration(-1) * time.Second) },
			assertFunc: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Require test table preconditions
			require.NotNil(t, tt.assertFunc)
			require.NotNil(t, tt.d)

			// Assert the invariants and post conditions
			tt.assertFunc(t, validateDeleteAfterSuccess(tt.d()))
		})
	}
}

// TestValidateVolumeMoveCR covers what validateVolumeMoveCR is responsible for
// today: fetching the TVM from the API and returning either the object or a
// NotFound. Spec-level validation (e.g. of DeleteAfterSuccess) lives in
// handleVolumeMove's per-state handlers, not here, and is covered by
// TestValidateDeleteAfterSuccess.
func TestValidateVolumeMoveCR(t *testing.T) {
	t.Parallel()

	const (
		testNamespace = "trident"
		testName      = "test-tvm"
	)

	ctx := context.Background()

	t.Run("existing TVM is returned", func(t *testing.T) {
		t.Parallel()
		tvm := fakeTridentVolumeMove(testNamespace, testName, nil)
		controller := &TridentCrdController{crdClientset: NewFakeClientset(tvm)}

		got, err := controller.validateVolumeMoveCR(ctx, testNamespace, testName)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, testName, got.Name)
		assert.Equal(t, testNamespace, got.Namespace)
	})

	t.Run("missing TVM surfaces NotFound", func(t *testing.T) {
		t.Parallel()
		controller := &TridentCrdController{crdClientset: NewFakeClientset()}

		got, err := controller.validateVolumeMoveCR(ctx, testNamespace, testName)
		require.Error(t, err)
		assert.True(t, errors.IsNotFoundError(err))
		assert.Nil(t, got)
	})

	t.Run("negative deleteAfterSuccess no longer rejected at fetch", func(t *testing.T) {
		// Regression guard: the function used to fail on invalid
		// DeleteAfterSuccess, but that validation moved into
		// handleVolumeMove's Pending handler. Make sure we don't
		// re-introduce the layering accidentally.
		t.Parallel()
		tvm := fakeTridentVolumeMove(testNamespace, testName, func(tvm *netappv1.TridentVolumeMove) {
			tvm.Spec.DeleteAfterSuccess = &metav1.Duration{Duration: -1 * time.Second}
		})
		controller := &TridentCrdController{crdClientset: NewFakeClientset(tvm)}

		got, err := controller.validateVolumeMoveCR(ctx, testNamespace, testName)
		require.NoError(t, err)
		require.NotNil(t, got)
	})
}

func fakeTridentVolumeMove(
	namespace, name string, mutate func(*netappv1.TridentVolumeMove),
) *netappv1.TridentVolumeMove {
	tvm := &netappv1.TridentVolumeMove{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: netappv1.TridentVolumeMoveSpec{
			SourceNode: "source-node",
			SourcePool: "source-pool",
			TargetNode: "target-node",
			TargetPool: "target-pool",
		},
		Status: netappv1.TridentVolumeMoveStatus{
			State: models.VolumeMoveStateSucceeded,
		},
	}
	if mutate != nil {
		mutate(tvm)
	}
	return tvm
}

func setupVolumeMoveController(
	t *testing.T, tvms ...*netappv1.TridentVolumeMove,
) (*TridentCrdController, *mockcore.MockOrchestrator) {
	t.Helper()

	mockCtrl := gomock.NewController(t)
	t.Cleanup(mockCtrl.Finish)

	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockIndexers, mockVaIndexer := setupMockIndexers(mockCtrl)

	// handleVolumeMove unconditionally calls reconcileAttachments,
	// which goes through GetCachedVolumeAttachmentsByVolume. Return an
	// empty list so callers that don't care about attachments don't have
	// to set this up themselves.
	mockVaIndexer.EXPECT().GetCachedVolumeAttachmentsByVolume(gomock.Any(), gomock.Any()).
		Return(nil, nil).AnyTimes()

	objects := make([]runtime.Object, len(tvms))
	for i, tvm := range tvms {
		objects[i] = tvm
	}

	crdClient := NewFakeClientset(objects...)
	controller, err := newTridentCrdControllerImpl(orchestrator, "trident", GetTestKubernetesClientset(), GetTestSnapshotClientset(), crdClient, mockIndexers, nil)
	require.NoError(t, err)
	return controller, orchestrator
}

func TestHandleVolumeMoveSucceededDelete(t *testing.T) {
	const (
		testNamespace = "trident"
		testName      = "test-tvm"
	)

	t.Run("phase 1 sets completion time without deleting", func(t *testing.T) {
		tvm := fakeTridentVolumeMove(testNamespace, testName, func(tvm *netappv1.TridentVolumeMove) {
			tvm.Spec.DeleteAfterSuccess = &metav1.Duration{}
		})
		controller, orchestrator := setupVolumeMoveController(t, tvm)
		orchestrator.EXPECT().ReconcileVolumeMove(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)

		err := controller.handleVolumeMove(&KeyItem{
			key: fmt.Sprintf("%s/%s", testNamespace, testName),
			ctx: ctx(),
		})
		require.NoError(t, err)

		got, getErr := controller.crdClientset.TridentV1().TridentVolumeMoves(testNamespace).Get(
			ctx(), testName, metav1.GetOptions{})
		require.NoError(t, getErr)
		require.NotNil(t, got.Status.CompletionTime, "first succeeded reconcile must persist completion time")
		assert.Equal(t, models.VolumeMoveStateSucceeded, got.Status.State)
	})

	t.Run("phase 2 zero deleteAfterSuccess deletes CR", func(t *testing.T) {
		completion := metav1.Now()
		tvm := fakeTridentVolumeMove(testNamespace, testName, func(tvm *netappv1.TridentVolumeMove) {
			tvm.Spec.DeleteAfterSuccess = &metav1.Duration{}
			tvm.Status.CompletionTime = &completion
		})
		controller, _ := setupVolumeMoveController(t, tvm)

		err := controller.handleVolumeMove(&KeyItem{
			key: fmt.Sprintf("%s/%s", testNamespace, testName),
			ctx: ctx(),
		})
		require.NoError(t, err)

		_, getErr := controller.crdClientset.TridentV1().TridentVolumeMoves(testNamespace).Get(
			ctx(), testName, metav1.GetOptions{})
		assert.True(t, apierrors.IsNotFound(getErr))
	})

	t.Run("retain CR when deleteAfterSuccess omitted after completion", func(t *testing.T) {
		completion := metav1.Now()
		tvm := fakeTridentVolumeMove(testNamespace, testName, func(tvm *netappv1.TridentVolumeMove) {
			tvm.Status.CompletionTime = &completion
		})
		controller, _ := setupVolumeMoveController(t, tvm)

		err := controller.handleVolumeMove(&KeyItem{
			key: fmt.Sprintf("%s/%s", testNamespace, testName),
			ctx: ctx(),
		})
		require.NoError(t, err)

		_, getErr := controller.crdClientset.TridentV1().TridentVolumeMoves(testNamespace).Get(
			ctx(), testName, metav1.GetOptions{})
		require.NoError(t, getErr)
	})

	t.Run("retention not elapsed requeues with duration", func(t *testing.T) {
		completion := metav1.NewTime(metav1.Now().Add(-1 * time.Minute))
		tvm := fakeTridentVolumeMove(testNamespace, testName, func(tvm *netappv1.TridentVolumeMove) {
			tvm.Spec.DeleteAfterSuccess = &metav1.Duration{Duration: 6 * time.Minute}
			tvm.Status.CompletionTime = &completion
		})
		controller, _ := setupVolumeMoveController(t, tvm)

		err := controller.handleVolumeMove(&KeyItem{
			key: fmt.Sprintf("%s/%s", testNamespace, testName),
			ctx: ctx(),
		})
		require.Error(t, err)
		assert.True(t, errors.IsReconcileDeferredWithDuration(err))
		gotDuration, ok := errors.ReconcileDeferredWithDurationValue(err)
		require.True(t, ok)
		assert.InDelta(t, (5 * time.Minute).Seconds(), gotDuration.Seconds(), 1.0)
	})
}

// TestUpdateTridentVolumeMove_PreservesFreshAttachments verifies that the
// controller does not clobber node-written attachment states when it writes a
// status update sourced from a stale reconciler snapshot.
//
// Scenario: During NodeStaging, node-1 has already written its attachment state
// to Bridged via the node controller.  The deployment controller's reconciler
// still holds a stale snapshot where node-1 is Pending.  After
// updateTridentVolumeMove, the API object must retain node-1=Bridged.
func TestUpdateTridentVolumeMove_PreservesFreshAttachments(t *testing.T) {
	t.Parallel()

	const (
		ns   = "trident"
		name = "test-tvm"
	)

	// Seed the fake API with the "truth": node-1 already Bridged by the node controller.
	tvm := fakeTridentVolumeMove(ns, name, func(tvm *netappv1.TridentVolumeMove) {
		tvm.Status.State = models.VolumeMoveStateNodeStaging
		tvm.Status.Attachments = []*netappv1.TridentVolumeMoveAttachmentStatus{
			{NodeName: "node-1", State: models.VolumeMoveAttachmentStateBridged, Message: "bridged by node"},
			{NodeName: "node-2", State: models.VolumeMoveAttachmentStatePending},
		}
	})

	controller := &TridentCrdController{crdClientset: NewFakeClientset(tvm)}

	// The reconciler's stale snapshot has both attachments as Pending.
	staleStatus := &netappv1.TridentVolumeMoveStatus{
		State: models.VolumeMoveStateNodeStaging,
		Attachments: []*netappv1.TridentVolumeMoveAttachmentStatus{
			{NodeName: "node-1", State: models.VolumeMoveAttachmentStatePending},
			{NodeName: "node-2", State: models.VolumeMoveAttachmentStatePending},
		},
	}

	err := controller.updateTridentVolumeMove(ctx(), ns, name, staleStatus)
	require.NoError(t, err)

	got, err := controller.crdClientset.TridentV1().TridentVolumeMoves(ns).Get(ctx(), name, metav1.GetOptions{})
	require.NoError(t, err)

	node1 := got.Status.AttachmentStatus("node-1")
	require.NotNil(t, node1, "node-1 attachment must exist")
	assert.Equal(t, models.VolumeMoveAttachmentStateBridged, node1.State,
		"node-1 must retain the fresh Bridged state, not be reverted to Pending by the controller")
	assert.Equal(t, "bridged by node", node1.Message)

	node2 := got.Status.AttachmentStatus("node-2")
	require.NotNil(t, node2, "node-2 attachment must exist")
	assert.Equal(t, models.VolumeMoveAttachmentStatePending, node2.State)

	assert.Equal(t, models.VolumeMoveStateNodeStaging, got.Status.State,
		"global TVM state must still be updated from the statusUpdate")
}

// TestUpdateTridentVolumeMove_DetachedOverrideApplied verifies that a
// controller-set Detached state (from reconcileAttachments) is not silently
// dropped when fresh attachments are restored.
func TestUpdateTridentVolumeMove_DetachedOverrideApplied(t *testing.T) {
	t.Parallel()

	const (
		ns   = "trident"
		name = "test-tvm"
	)

	// API state: both nodes are Bridged (the fresh read).
	tvm := fakeTridentVolumeMove(ns, name, func(tvm *netappv1.TridentVolumeMove) {
		tvm.Status.State = models.VolumeMoveStateNodeStaging
		tvm.Status.Attachments = []*netappv1.TridentVolumeMoveAttachmentStatus{
			{NodeName: "node-1", State: models.VolumeMoveAttachmentStateBridged, Message: "bridged"},
			{NodeName: "node-2", State: models.VolumeMoveAttachmentStateBridged, Message: "bridged"},
		}
	})

	controller := &TridentCrdController{crdClientset: NewFakeClientset(tvm)}

	// The controller's reconcileAttachments detected that node-1's K8s
	// VolumeAttachment disappeared and marked it Detached in statusUpdate.
	staleStatus := &netappv1.TridentVolumeMoveStatus{
		State: models.VolumeMoveStateNodeStaging,
		Attachments: []*netappv1.TridentVolumeMoveAttachmentStatus{
			{NodeName: "node-1", State: models.VolumeMoveAttachmentStateDetached, Message: "VolumeAttachment removed"},
			{NodeName: "node-2", State: models.VolumeMoveAttachmentStateBridged, Message: "bridged"},
		},
	}

	err := controller.updateTridentVolumeMove(ctx(), ns, name, staleStatus)
	require.NoError(t, err)

	got, err := controller.crdClientset.TridentV1().TridentVolumeMoves(ns).Get(ctx(), name, metav1.GetOptions{})
	require.NoError(t, err)

	node1 := got.Status.AttachmentStatus("node-1")
	require.NotNil(t, node1)
	assert.Equal(t, models.VolumeMoveAttachmentStateDetached, node1.State,
		"controller-set Detached must not be dropped by the fresh-attachment restore")
	assert.Equal(t, "VolumeAttachment removed", node1.Message)

	node2 := got.Status.AttachmentStatus("node-2")
	require.NotNil(t, node2)
	assert.Equal(t, models.VolumeMoveAttachmentStateBridged, node2.State,
		"node-2 must retain its fresh state")
}

// TestUpdateTridentVolumeMove_DoesNotRevertCleanedToDetached verifies that a
// stale Detached in the controller's statusUpdate does not overwrite a Cleaned
// state that the node wrote between the two API reads.
func TestUpdateTridentVolumeMove_DoesNotRevertCleanedToDetached(t *testing.T) {
	t.Parallel()

	const (
		ns   = "trident"
		name = "test-tvm"
	)

	// The fresh API read shows node-1 already Cleaned by the node controller.
	tvm := fakeTridentVolumeMove(ns, name, func(tvm *netappv1.TridentVolumeMove) {
		tvm.Status.State = models.VolumeMoveStateNodeStaging
		tvm.Status.Attachments = []*netappv1.TridentVolumeMoveAttachmentStatus{
			{NodeName: "node-1", State: models.VolumeMoveAttachmentStateCleaned, Message: "cleaned by node"},
		}
	})

	controller := &TridentCrdController{crdClientset: NewFakeClientset(tvm)}

	// The controller's first Get (in handleVolumeMove) had node-1 as Detached.
	staleStatus := &netappv1.TridentVolumeMoveStatus{
		State: models.VolumeMoveStateNodeStaging,
		Attachments: []*netappv1.TridentVolumeMoveAttachmentStatus{
			{NodeName: "node-1", State: models.VolumeMoveAttachmentStateDetached, Message: "VolumeAttachment removed"},
		},
	}

	err := controller.updateTridentVolumeMove(ctx(), ns, name, staleStatus)
	require.NoError(t, err)

	got, err := controller.crdClientset.TridentV1().TridentVolumeMoves(ns).Get(ctx(), name, metav1.GetOptions{})
	require.NoError(t, err)

	node1 := got.Status.AttachmentStatus("node-1")
	require.NotNil(t, node1)
	assert.Equal(t, models.VolumeMoveAttachmentStateCleaned, node1.State,
		"stale Detached must not revert a node-set Cleaned state")
	assert.Equal(t, "cleaned by node", node1.Message)
}

func TestDeleteTridentVolumeMove(t *testing.T) {
	t.Parallel()

	const (
		testNamespace = "trident"
		testName      = "test-tvm"
	)

	ctx := context.Background()

	t.Run("not found is success", func(t *testing.T) {
		t.Parallel()
		controller := &TridentCrdController{crdClientset: crdclientfake.NewSimpleClientset()}
		err := controller.deleteTridentVolumeMove(ctx, testNamespace, testName)
		require.NoError(t, err)
	})

	t.Run("delete succeeds", func(t *testing.T) {
		t.Parallel()
		tvm := &netappv1.TridentVolumeMove{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testName,
				Namespace: testNamespace,
			},
		}
		controller := &TridentCrdController{crdClientset: crdclientfake.NewSimpleClientset(tvm)}
		err := controller.deleteTridentVolumeMove(ctx, testNamespace, testName)
		require.NoError(t, err)
		_, getErr := controller.crdClientset.TridentV1().TridentVolumeMoves(testNamespace).Get(
			ctx, testName, metav1.GetOptions{})
		assert.True(t, apierrors.IsNotFound(getErr))
	})

	t.Run("delete failure returns reconcile deferred", func(t *testing.T) {
		t.Parallel()
		deleteErr := apierrors.NewInternalError(fmt.Errorf("API server error"))
		crdClient := crdclientfake.NewSimpleClientset()
		crdClient.PrependReactor("delete", "tridentvolumemoves", func(action ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, deleteErr
		})
		controller := &TridentCrdController{crdClientset: crdClient}

		err := controller.deleteTridentVolumeMove(ctx, testNamespace, testName)
		require.Error(t, err)
		assert.True(t, errors.IsReconcileDeferredError(err))
		assert.ErrorContains(t, err, "failed to delete TVM")
		assert.True(t, apierrors.IsInternalError(errors.Unwrap(err)))
	})
}

// TestHandleVolumeMoveControllerStagingAttachmentSeeding verifies that the
// immutable TVM attachment set is seeded during ControllerStaging (not Pending)
// purely from the live Kubernetes VolumeAttachments that exist for the volume.
//
// Seeding is deferred to ControllerStaging because commitState is the sole
// projector of volume.Config.MoveInfo and runs AFTER the state handler: the
// first MoveInfo projection therefore lands on the Pending commit, and the
// snapshot must only be taken once MoveInfo is guaranteed present in the cache.
//
// Seeding is anchored on VolumeAttachment.Spec and is Attached-agnostic: we do
// NOT gate on va.Status.Attached (patched asynchronously by the external-attacher
// after the publish RPC returns) nor on the existence of a Trident
// VolumePublication. A VA whose attach is mid-flight (Attached=false) must STILL
// be seeded, otherwise the TVM would drop a node whose CSI flow is in progress
// and leave it with stale Source AccessInfo after the move.
func TestHandleVolumeMoveControllerStagingAttachmentSeeding(t *testing.T) {
	t.Parallel()

	const (
		testNamespace = "trident"
		testName      = "test-tvm"
	)

	fakeVA := func(nodeName string, attached bool) *storagev1.VolumeAttachment {
		return &storagev1.VolumeAttachment{
			ObjectMeta: metav1.ObjectMeta{Name: "va-" + nodeName},
			Spec: storagev1.VolumeAttachmentSpec{
				NodeName: nodeName,
				Source: storagev1.VolumeAttachmentSource{
					PersistentVolumeName: strPtr(testName),
				},
			},
			Status: storagev1.VolumeAttachmentStatus{
				Attached: attached,
			},
		}
	}

	tests := []struct {
		name              string
		k8sVAs            []*storagev1.VolumeAttachment
		wantAttachments   []string
		wantAttachmentLen int
	}{
		{
			name:              "all published nodes have fully-attached VAs",
			k8sVAs:            []*storagev1.VolumeAttachment{fakeVA("node-1", true), fakeVA("node-2", true)},
			wantAttachments:   []string{"node-1", "node-2"},
			wantAttachmentLen: 2,
		},
		{
			// A VA whose external-attacher patch of Status.Attached has not landed
			// yet (Attached=false) must STILL be seeded, otherwise the TVM would
			// drop a node whose CSI flow is mid-completion and leave it with stale
			// Source AccessInfo after the move.
			name:              "mid-attach VA (Attached=false) is still seeded",
			k8sVAs:            []*storagev1.VolumeAttachment{fakeVA("node-mid", false)},
			wantAttachments:   []string{"node-mid"},
			wantAttachmentLen: 1,
		},
		{
			name:              "mixed attached and mid-attach VAs are all seeded",
			k8sVAs:            []*storagev1.VolumeAttachment{fakeVA("node-1", true), fakeVA("node-2", false)},
			wantAttachments:   []string{"node-1", "node-2"},
			wantAttachmentLen: 2,
		},
		{
			name:              "no VAs means no attachments seeded",
			k8sVAs:            nil,
			wantAttachments:   nil,
			wantAttachmentLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockCtrl := gomock.NewController(t)
			t.Cleanup(mockCtrl.Finish)

			orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			mockIdxs := mockindexers.NewMockIndexers(mockCtrl)
			mockVaIdx := mockindexer.NewMockVolumeAttachmentIndexer(mockCtrl)

			mockVaIdx.EXPECT().WaitForCacheSync(gomock.Any()).Return(true).AnyTimes()
			mockIdxs.EXPECT().VolumeAttachmentIndexer().Return(mockVaIdx).AnyTimes()

			// reconcileAttachments uses the indexer; populateAttachments uses
			// the live kubeClientset list. Both paths are exercised in this test.
			mockVaIdx.EXPECT().GetCachedVolumeAttachmentsByVolume(gomock.Any(), testName).
				Return(tt.k8sVAs, nil).AnyTimes()

			// MoveVolume dry-run (Pending) only validates the move; it no longer
			// seeds nodes or projects MoveInfo.
			orchestrator.EXPECT().MoveVolume(gomock.Any(), gomock.Any()).Return(nil).Times(1)

			// ControllerStaging grafts the target reporting node onto the igroups.
			orchestrator.EXPECT().StageVolumeMove(gomock.Any(), gomock.Any()).Return(nil).Times(1)

			// commitState projects MoveInfo on every commit: once for the Pending
			// reconcile and once for the ControllerStaging reconcile.
			orchestrator.EXPECT().ReconcileVolumeMove(gomock.Any(), testName, gomock.Any()).Return(nil).Times(2)

			tvm := fakeTridentVolumeMove(testNamespace, testName, func(tvm *netappv1.TridentVolumeMove) {
				tvm.Status.State = models.VolumeMoveStatePending
			})
			crdClient := NewFakeClientset(tvm)

			// Seed the fake kubeClientset with the test's VAs so the live
			// populateAttachments list call returns them.
			kubeObjects := make([]runtime.Object, 0, len(tt.k8sVAs))
			for _, va := range tt.k8sVAs {
				kubeObjects = append(kubeObjects, va)
			}
			kubeClient := GetTestKubernetesClientset(kubeObjects...)

			controller, err := newTridentCrdControllerImpl(
				orchestrator, "trident",
				kubeClient, GetTestSnapshotClientset(),
				crdClient, mockIdxs, nil,
			)
			require.NoError(t, err)

			key := fmt.Sprintf("%s/%s", testNamespace, testName)

			// Reconcile 1: Pending -> ControllerStaging. No attachments are
			// seeded yet; this commit projects MoveInfo into the cache.
			require.NoError(t, controller.handleVolumeMove(&KeyItem{key: key, ctx: ctx()}))

			afterPending, getErr := crdClient.TridentV1().TridentVolumeMoves(testNamespace).Get(
				ctx(), testName, metav1.GetOptions{},
			)
			require.NoError(t, getErr)
			assert.Equal(t, models.VolumeMoveStateControllerStaging, afterPending.Status.State,
				"TVM must transition from Pending to ControllerStaging")
			assert.Empty(t, afterPending.Status.Attachments,
				"attachments must NOT be seeded during Pending; MoveInfo is not yet projected")

			// Reconcile 2: ControllerStaging seeds the immutable attachment
			// snapshot now that MoveInfo is guaranteed present in the cache.
			require.NoError(t, controller.handleVolumeMove(&KeyItem{key: key, ctx: ctx()}))

			got, getErr := crdClient.TridentV1().TridentVolumeMoves(testNamespace).Get(
				ctx(), testName, metav1.GetOptions{},
			)
			require.NoError(t, getErr)

			assert.NotContains(t,
				[]models.VolumeMoveState{models.VolumeMoveStatePending, models.VolumeMoveStateControllerStaging},
				got.Status.State,
				"TVM must advance past ControllerStaging once attachments are seeded")
			assert.Len(t, got.Status.Attachments, tt.wantAttachmentLen,
				"seeded attachment count must match the live K8s VolumeAttachments")

			gotNodes := make([]string, 0, len(got.Status.Attachments))
			for _, a := range got.Status.Attachments {
				gotNodes = append(gotNodes, a.NodeName)
				assert.Equal(t, models.VolumeMoveAttachmentStatePending, a.State,
					"all seeded attachments must start as Pending")
			}
			assert.ElementsMatch(t, tt.wantAttachments, gotNodes)
		})
	}
}

func strPtr(s string) *string { return &s }
