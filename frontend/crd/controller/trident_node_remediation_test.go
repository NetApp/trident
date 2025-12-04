// Copyright 2025 NetApp, Inc. All Rights Reserved.

package controller

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mockcore "github.com/netapp/trident/mocks/mock_core"
	mock_controller_crd "github.com/netapp/trident/mocks/mock_frontend/crd/controller"
	mockindexers "github.com/netapp/trident/mocks/mock_frontend/crd/controller/indexers"
	mockindexer "github.com/netapp/trident/mocks/mock_frontend/crd/controller/indexers/indexer"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

const (
	nodeName = "node-1"
)

// Helper function to create a fake TridentNodeRemediation CR
func fakeTridentNodeRemediation(name, namespace, state string) *netappv1.TridentNodeRemediation {
	tnr := &netappv1.TridentNodeRemediation{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "trident.netapp.io/v1",
			Kind:       "TridentNodeRemediation",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: netappv1.TridentNodeRemediationStatus{
			State:             state,
			VolumeAttachments: map[string]string{},
		},
	}
	return tnr
}

// Helper function to create a fake NodeExternal
func fakeNodeExternal(name string, state models.NodePublicationState) *models.NodeExternal {
	return &models.NodeExternal{
		Name:             name,
		PublicationState: state,
	}
}

// Helper function to create fake VolumePublicationsExternal
func fakeVolumePublicationsExternal(count int, nodeName string) []*models.VolumePublicationExternal {
	publications := make([]*models.VolumePublicationExternal, count)
	for i := 0; i < count; i++ {
		publications[i] = &models.VolumePublicationExternal{
			Name:       fmt.Sprintf("vol-pub-%d", i),
			NodeName:   nodeName,
			VolumeName: fmt.Sprintf("vol-%d", i),
		}
	}
	return publications
}

func setupMockIndexers(
	mockCtrl *gomock.Controller,
) (*mockindexers.MockIndexers, *mockindexer.MockVolumeAttachmentIndexer) {
	mockIndexers := mockindexers.NewMockIndexers(mockCtrl)
	mockVaIndexer := mockindexer.NewMockVolumeAttachmentIndexer(mockCtrl)
	mockVaIndexer.EXPECT().WaitForCacheSync(gomock.Any()).Return(true)
	mockIndexers.EXPECT().VolumeAttachmentIndexer().Return(mockVaIndexer).AnyTimes()
	return mockIndexers, mockVaIndexer
}

// TestHandleTridentNodeRemediation_InvalidKey tests that the handler properly rejects
// malformed key strings that don't follow the expected "namespace/name" format.
// This validates input parsing and error handling for invalid key structures.
func TestHandleTridentNodeRemediation_InvalidKey(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	keyItem := &KeyItem{
		key: "cluster/namespace/name/extra",
		ctx: ctx(),
	}

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.Error(t, err, "invalid key should cause error")
}

// TestHandleTridentNodeRemediation_ValidationFailure tests that the handler fails gracefully
// when trying to process a TridentNodeRemediation CR that doesn't exist in the cluster.
// This validates the CR validation step and proper error handling for missing resources.
func TestHandleTridentNodeRemediation_ValidationFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	keyItem := makeKeyItem(t, "trident", "non-existent-node")

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.Error(t, err, "validation should fail for non-existent CR")
}

// TestHandleTridentNodeRemediation_DefaultStateTransition tests the initial state transition
// from empty/default state to "remediating" state. This validates that new TridentNodeRemediation
// CRs are properly initialized with the remediating state and have finalizers added.
func TestHandleTridentNodeRemediation_DefaultStateTransition(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation with default state (empty)
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, "")
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.NoError(t, err)

	// Check that the state was updated to remediating
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.Equal(t, netappv1.TridentNodeRemediatingState, updatedTNR.Status.State)
	assert.True(t, updatedTNR.HasTridentFinalizers())
}

// TestHandleTridentNodeRemediation_RemediatingStateCleanNode tests the behavior when a node
// is in remediating state but the underlying node is already clean. The handler should return
// a deferred error to retry later since the node needs to be dirty before proceeding.
func TestHandleTridentNodeRemediation_RemediatingStateCleanNode(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation in remediating state
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	// Mock orchestrator calls - use NodeExternal type
	nodeExternal := fakeNodeExternal(nodeName, models.NodeClean)
	orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nodeExternal, nil)
	orchestrator.EXPECT().UpdateNode(gomock.Any(), nodeName, gomock.Any()).Return(nil)

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.Error(t, err, "should return ReconcileDeferredError for clean node")
	assert.True(t, errors.IsReconcileDeferredError(err))

	// Verify state was not changed (should stay remediating until node is dirty)
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.Equal(t, netappv1.TridentNodeRemediatingState, updatedTNR.Status.State)
}

// TestHandleTridentNodeRemediation_RemediatingStateDirtyNodeWithPublications tests that
// when a node is dirty but still has volume publications, the handler waits by returning
// a deferred error. This ensures volumes are unpublished before node cleanup proceeds.
func TestHandleTridentNodeRemediation_RemediatingStateDirtyNodeWithPublications(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockVolAttachment := &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name: "va-0",
		},
		Spec: storagev1.VolumeAttachmentSpec{
			NodeName: nodeName,
			Source: storagev1.VolumeAttachmentSource{
				PersistentVolumeName: func() *string { s := "vol-0"; return &s }(),
			},
		},
	}

	mockIndexers, mockVaIndexer := setupMockIndexers(mockCtrl)
	mockVaIndexer.EXPECT().GetCachedVolumeAttachmentsByNode(gomock.Any(), nodeName).
		Return([]*storagev1.VolumeAttachment{mockVolAttachment}, nil).AnyTimes()
	remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, mockIndexers)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, remediationUtils)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation in remediating state
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	// Add some fake volume attachments to to the status. These are VAs that need to be removed.
	tnr.Status.VolumeAttachments = map[string]string{"va-0": "vol-0", "va-1": "vol-1"}
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	// Mock orchestrator calls - node is dirty with publications
	nodeExternal := fakeNodeExternal(nodeName, models.NodeDirty)
	publications := fakeVolumePublicationsExternal(2, nodeName)
	orchestrator.EXPECT().GetVolume(ctx(), gomock.Any()).Return(&storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			RequestName: "pvc-0",
		},
	}, nil).AnyTimes()
	orchestrator.EXPECT().ListVolumePublicationsForNode(ctx(), nodeName)
	orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nodeExternal, nil)
	orchestrator.EXPECT().ListVolumePublicationsForNode(gomock.Any(), nodeName).Return(publications, nil)
	orchestrator.EXPECT().UpdateNode(ctx(), nodeName, gomock.Any()).Return(nil).AnyTimes()

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.Error(t, err, "should return ReconcileDeferredError while publications exist")
	assert.True(t, errors.IsReconcileDeferredError(err))
	assert.Contains(t, err.Error(), "still has volume attachments to remove")
}

// TestHandleTridentNodeRemediation_RemediatingStateDirtyNodeNoPublications tests the
// successful transition from remediating to NodeRecoveryPending state when a dirty node has no
// remaining volume publications. This validates the core remediation logic progression.
func TestHandleTridentNodeRemediation_RemediatingStateDirtyNodeNoPublications(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, mockVaIndexer := setupMockIndexers(mockCtrl)
	remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, mockIndexers)
	mockVaIndexer.EXPECT().GetCachedVolumeAttachmentsByNode(gomock.Any(), nodeName).Return([]*storagev1.VolumeAttachment{}, nil).AnyTimes()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, remediationUtils)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation in remediating state
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	// Mock orchestrator calls - node is dirty with no publications
	nodeExternal := fakeNodeExternal(nodeName, models.NodeDirty)
	publications := []*models.VolumePublicationExternal{}
	orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nodeExternal, nil)
	orchestrator.EXPECT().ListVolumePublicationsForNode(gomock.Any(), nodeName).Return(publications, nil).Times(2)
	orchestrator.EXPECT().UpdateNode(gomock.Any(), nodeName, gomock.Any()).Return(nil)

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.NoError(t, err)

	// Verify state transitioned to NodeRecoveryPending
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.Equal(t, netappv1.NodeRecoveryPending, updatedTNR.Status.State)
}

// TestHandleTridentNodeRemediation_NodeRecoveryPendingStateNodeClean tests the successful completion
// of node remediation when the node reaches a clean state. This validates the final
// state transition to succeeded and removal of finalizers.
func TestHandleTridentNodeRemediation_NodeRecoveryPendingStateNodeClean(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation in NodeRecoveryPending state
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.NodeRecoveryPending)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	// Mock orchestrator calls - node is clean
	nodeExternal := fakeNodeExternal(nodeName, models.NodeClean)
	orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nodeExternal, nil)

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.NoError(t, err)

	// Verify state transitioned to succeeded
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.Equal(t, netappv1.TridentActionStateSucceeded, updatedTNR.Status.State)
	assert.False(t, updatedTNR.HasTridentFinalizers())
}

// TestHandleTridentNodeRemediation_NodeRecoveryPendingStateNodeNotClean tests that when a node
// is in NodeRecoveryPending state but not yet clean, the handler returns a deferred error to
// retry later. This validates the polling behavior during the cleanup phase.
func TestHandleTridentNodeRemediation_NodeRecoveryPendingStateNodeNotClean(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation in NodeRecoveryPending state
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.NodeRecoveryPending)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	// Mock orchestrator calls - node is not clean yet
	nodeExternal := fakeNodeExternal(nodeName, models.NodeDirty)
	orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nodeExternal, nil)

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.Error(t, err, "should return ReconcileDeferredError for node not clean yet")
	assert.True(t, errors.IsReconcileDeferredError(err))
	assert.Contains(t, err.Error(), "not clean yet")
}

// TestHandleTridentNodeRemediation_SucceededState tests the handling of CRs that are
// already in succeeded state. This validates that completion time is properly set
// and no further state changes occur.
func TestHandleTridentNodeRemediation_SucceededState(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation in succeeded state
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentActionStateSucceeded)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.NoError(t, err)

	// Verify completion time was set
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.Equal(t, netappv1.TridentActionStateSucceeded, updatedTNR.Status.State)
	assert.NotNil(t, updatedTNR.Status.CompletionTime)
}

// TestHandleTridentNodeRemediation_FailedState tests the handling of CRs that are
// already in failed state. This validates that completion time is properly set
// and no further state changes occur for failed remediation attempts.
func TestHandleTridentNodeRemediation_FailedState(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation in failed state
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentActionStateFailed)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.NoError(t, err)

	// Verify completion time was set
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.Equal(t, netappv1.TridentActionStateFailed, updatedTNR.Status.State)
	assert.NotNil(t, updatedTNR.Status.CompletionTime)
}

// TestHandleTridentNodeRemediation_RemediatingStateGetNodeError tests error handling
// when the orchestrator fails to retrieve node information during the remediating state.
// This validates that orchestrator errors cause the CR to transition to failed state.
func TestHandleTridentNodeRemediation_RemediatingStateGetNodeError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation in remediating state
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	// Mock orchestrator calls - error getting node
	orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nil, fmt.Errorf("node not found"))

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.NoError(t, err)

	// Verify state was updated to failed
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.Equal(t, netappv1.TridentActionStateFailed, updatedTNR.Status.State)
	assert.Contains(t, updatedTNR.Status.Message, "Could not get node")
}

// TestHandleTridentNodeRemediation_RemediatingStateListPublicationsError tests error
// handling when listing volume publications fails for a dirty node. This validates
// that publication listing errors don't cause state changes but prevent progression.
func TestHandleTridentNodeRemediation_RemediatingStateListPublicationsError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, mockVaIndexer := setupMockIndexers(mockCtrl)
	mockVaIndexer.EXPECT().GetCachedVolumeAttachmentsByNode(gomock.Any(), nodeName).Return([]*storagev1.VolumeAttachment{}, nil).AnyTimes()
	remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, mockIndexers)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, remediationUtils)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation in remediating state
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	// Mock orchestrator calls - node is dirty but error listing publications
	nodeExternal := fakeNodeExternal(nodeName, models.NodeDirty)
	orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nodeExternal, nil)
	orchestrator.EXPECT().ListVolumePublicationsForNode(gomock.Any(), nodeName).Return([]*models.VolumePublicationExternal{}, nil)
	orchestrator.EXPECT().ListVolumePublicationsForNode(gomock.Any(), nodeName).Return(nil, fmt.Errorf("listing error"))
	// No UpdateNode call expected because the error causes a break

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.NoError(t, err)

	// When there's an error listing publications, the logic breaks and doesn't change state
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	// State should remain remediating when there's an error
	assert.Equal(t, netappv1.TridentNodeRemediatingState, updatedTNR.Status.State)
}

// TestHandleTridentNodeRemediation_RemediatingStateUpdateNodeError tests error handling
// when updating a node fails during the remediating state. This validates that
// orchestrator update errors are properly propagated and cause the handler to fail.
func TestHandleTridentNodeRemediation_RemediatingStateUpdateNodeError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, mockVaIndexer := setupMockIndexers(mockCtrl)
	mockVaIndexer.EXPECT().GetCachedVolumeAttachmentsByNode(gomock.Any(), nodeName).Return([]*storagev1.VolumeAttachment{}, nil).AnyTimes()
	remediationUtils := NewNodeRemediationUtils(kubeClient, orchestrator, mockIndexers)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, remediationUtils)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation in remediating state
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	// Mock orchestrator calls - node is dirty with no publications but update fails
	nodeExternal := fakeNodeExternal(nodeName, models.NodeDirty)
	publications := []*models.VolumePublicationExternal{}
	orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nodeExternal, nil)
	orchestrator.EXPECT().ListVolumePublicationsForNode(gomock.Any(), nodeName).Return(publications, nil).Times(2)
	orchestrator.EXPECT().UpdateNode(gomock.Any(), nodeName, gomock.Any()).Return(fmt.Errorf("update failed"))

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
}

// TestHandleTridentNodeRemediation_NodeRecoveryPendingStateGetNodeError tests error handling
// when the orchestrator fails to retrieve node information during the NodeRecoveryPending state.
// This validates that orchestrator errors cause the CR to transition to failed state.
func TestHandleTridentNodeRemediation_CleaningStateGetNodeError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation in cleaning state
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.NodeRecoveryPending)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	// Mock orchestrator calls - error getting node
	orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nil, fmt.Errorf("node not found"))

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.NoError(t, err)

	// Verify state was updated to failed
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.Equal(t, netappv1.TridentActionStateFailed, updatedTNR.Status.State)
	assert.Contains(t, updatedTNR.Status.Message, "Could not get node")
}

// TestValidateTridentNodeRemediationCR_NotFound tests the validation function's behavior
// when attempting to validate a non-existent TridentNodeRemediation CR. This ensures
// proper error handling for missing resources during validation.
func TestValidateTridentNodeRemediationCR_NotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	_, err = crdController.validateTridentNodeRemediationCR(ctx(), tridentNamespace, "non-existent")
	assert.Error(t, err)
	assert.True(t, apierrors.IsNotFound(err))
}

// TestValidateTridentNodeRemediationCR_Success tests successful validation of an existing
// TridentNodeRemediation CR. This validates that the validation function correctly
// retrieves and returns valid CRs.
func TestValidateTridentNodeRemediationCR_Success(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, "")
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	actionCR, err := crdController.validateTridentNodeRemediationCR(ctx(), tridentNamespace, nodeName)
	assert.NoError(t, err)
	assert.NotNil(t, actionCR)
	assert.Equal(t, nodeName, actionCR.Name)
}

// TestUpdateNodeRemediationCR_NotFound tests the update function's behavior when
// attempting to update a non-existent TridentNodeRemediation CR. This validates
// proper error handling for missing resources during status updates.
func TestUpdateNodeRemediationCR_NotFound(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	statusUpdate := &netappv1.TridentNodeRemediationStatus{
		State: netappv1.TridentActionStateSucceeded,
	}

	err = crdController.updateNodeRemediationCR(ctx(), tridentNamespace, "non-existent", statusUpdate)
	assert.Error(t, err)
	assert.True(t, apierrors.IsNotFound(err))
}

// TestUpdateNodeRemediationCR_SuccessAddsFinalizer tests that the update function
// properly adds finalizers when updating a CR that doesn't have them. This validates
// finalizer management during CR lifecycle.
func TestUpdateNodeRemediationCR_SuccessAddsFinalizer(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation without finalizers
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, "")
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	statusUpdate := &netappv1.TridentNodeRemediationStatus{
		State: netappv1.TridentNodeRemediatingState,
	}

	err = crdController.updateNodeRemediationCR(ctx(), tridentNamespace, nodeName, statusUpdate)
	assert.NoError(t, err)

	// Verify finalizers were added and status was updated
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.True(t, updatedTNR.HasTridentFinalizers())
	assert.Equal(t, netappv1.TridentNodeRemediatingState, updatedTNR.Status.State)
}

// TestUpdateNodeRemediationCR_CompletedRemovesFinalizer tests that finalizers are
// properly removed when a CR transitions to succeeded state. This validates cleanup
// behavior for successfully completed remediation operations.
func TestUpdateNodeRemediationCR_CompletedRemovesFinalizer(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation with finalizers
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	tnr.AddTridentFinalizers()
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	statusUpdate := &netappv1.TridentNodeRemediationStatus{
		State: netappv1.TridentActionStateSucceeded,
	}

	err = crdController.updateNodeRemediationCR(ctx(), tridentNamespace, nodeName, statusUpdate)
	assert.NoError(t, err)

	// Verify finalizers were removed and status was updated
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.False(t, updatedTNR.HasTridentFinalizers())
	assert.Equal(t, netappv1.TridentActionStateSucceeded, updatedTNR.Status.State)
}

// TestUpdateNodeRemediationCR_FailedRemovesFinalizer tests that finalizers are
// properly removed when a CR transitions to failed state. This validates cleanup
// behavior for failed remediation operations.
func TestUpdateNodeRemediationCR_FailedRemovesFinalizer(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation with finalizers
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	tnr.AddTridentFinalizers()
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	statusUpdate := &netappv1.TridentNodeRemediationStatus{
		State: netappv1.TridentActionStateFailed,
	}

	err = crdController.updateNodeRemediationCR(ctx(), tridentNamespace, nodeName, statusUpdate)
	assert.NoError(t, err)

	// Verify finalizers were removed and status was updated
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.False(t, updatedTNR.HasTridentFinalizers())
	assert.Equal(t, netappv1.TridentActionStateFailed, updatedTNR.Status.State)
}

// TestUpdateNodeRemediationCR_DeletedDuringUpdate tests race condition handling
// when a CR is deleted while an update operation is in progress. This validates
// proper error handling for concurrent deletion scenarios.
func TestUpdateNodeRemediationCR_DeletedDuringUpdate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create and then delete the CR to simulate deletion during processing
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, "")
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Delete(ctx(), nodeName, deleteOptions)
	assert.NoError(t, err)

	statusUpdate := &netappv1.TridentNodeRemediationStatus{
		State: netappv1.TridentActionStateSucceeded,
	}

	err = crdController.updateNodeRemediationCR(ctx(), tridentNamespace, nodeName, statusUpdate)
	assert.Error(t, err)
	assert.True(t, apierrors.IsNotFound(err))
}

// TestHandleTridentNodeRemediation_ErrorStatesWithMessages tests error handling
// and message setting for various error scenarios. This validates that appropriate
// error messages are set when the CR transitions to failed state due to orchestrator errors.
func TestHandleTridentNodeRemediation_ErrorStatesWithMessages(t *testing.T) {
	tests := []struct {
		name           string
		initialState   string
		nodeError      error
		expectedState  string
		expectedErrMsg string
	}{
		{
			name:           "remediating state get node error",
			initialState:   netappv1.TridentNodeRemediatingState,
			nodeError:      fmt.Errorf("node access denied"),
			expectedState:  netappv1.TridentActionStateFailed,
			expectedErrMsg: "Could not get node, node access denied",
		},
		{
			name:           "cleaning state get node error",
			initialState:   netappv1.NodeRecoveryPending,
			nodeError:      fmt.Errorf("node not available"),
			expectedState:  netappv1.TridentActionStateFailed,
			expectedErrMsg: "Could not get node, node not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			tridentNamespace := "trident"
			kubeClient := GetTestKubernetesClientset()
			snapClient := GetTestSnapshotClientset()
			crdClient := GetTestCrdClientset()
			mockIndexers, _ := setupMockIndexers(mockCtrl)
			crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
				crdClient, mockIndexers, nil)
			if err != nil {
				t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
			}
			crdController.enableForceDetach = true

			// Create TridentNodeRemediation in specified initial state
			tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, tt.initialState)
			_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
			assert.NoError(t, err)

			// Mock orchestrator calls with error
			orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nil, tt.nodeError)

			keyItem := makeKeyItem(t, tridentNamespace, nodeName)

			err = crdController.handleTridentNodeRemediation(keyItem)
			assert.NoError(t, err)

			// Verify error state and message
			updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedState, updatedTNR.Status.State)
			assert.Equal(t, tt.expectedErrMsg, updatedTNR.Status.Message)
		})
	}
}

// TestUpdateNodeRemediationCR_AlreadyHasFinalizers tests the update behavior when
// a CR already has finalizers. This validates that existing finalizers are preserved
// and not duplicated during status updates.
func TestUpdateNodeRemediationCR_AlreadyHasFinalizers(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation that already has finalizers
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, "")
	tnr.AddTridentFinalizers()
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	statusUpdate := &netappv1.TridentNodeRemediationStatus{
		State: netappv1.TridentNodeRemediatingState,
	}

	err = crdController.updateNodeRemediationCR(ctx(), tridentNamespace, nodeName, statusUpdate)
	assert.NoError(t, err)

	// Verify finalizers are still there and status was updated
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.True(t, updatedTNR.HasTridentFinalizers())
	assert.Equal(t, netappv1.TridentNodeRemediatingState, updatedTNR.Status.State)
}

// TestUpdateNodeRemediationCR_KeepsFinalizersForInProgressStates tests that finalizers
// are maintained for in-progress states like NodeRecoveryPending. This validates that finalizers
// are only removed when the CR reaches a terminal state (succeeded or failed).
func TestUpdateNodeRemediationCR_KeepsFinalizersForInProgressStates(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	// Create a TridentNodeRemediation without finalizers
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, "")
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	statusUpdate := &netappv1.TridentNodeRemediationStatus{
		State: netappv1.NodeRecoveryPending,
	}

	err = crdController.updateNodeRemediationCR(ctx(), tridentNamespace, nodeName, statusUpdate)
	assert.NoError(t, err)

	// Verify finalizers were added for in-progress state
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.True(t, updatedTNR.HasTridentFinalizers())
	assert.Equal(t, netappv1.NodeRecoveryPending, updatedTNR.Status.State)
}

func TestHandleTridentNodeRemediation_NodeDeletedDuringRemediation(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	// Simulate node deleted
	orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nil, errors.NotFoundError("node %s was not found", nodeName))

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.NoError(t, err)

	// CR should be deleted
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestHandleTridentNodeRemediation_NodeDeletedDuringCleaning(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.NodeRecoveryPending)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	// Simulate node deleted
	orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nil, errors.NotFoundError("node %s was not found", nodeName))

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.NoError(t, err)

	// CR should be deleted
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestHandleTridentNodeRemediation_ForceDetachDisabled(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}

	crdController.enableForceDetach = false

	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.Error(t, err)
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.Equal(t, netappv1.TridentActionStateFailed, updatedTNR.Status.State)
	assert.Contains(t, updatedTNR.Status.Message, "'--enable-force-detach' flag to be enabled")
}

func TestHandleTridentNodeRemediation_GetNodeReturnsNodeAndError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	nodeExternal := fakeNodeExternal(nodeName, models.NodeDirty)
	orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nodeExternal, fmt.Errorf("unexpected error"))

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.NoError(t, err)
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.Equal(t, netappv1.TridentActionStateFailed, updatedTNR.Status.State)
	assert.Contains(t, updatedTNR.Status.Message, "unexpected error")
}

func TestHandleTridentNodeRemediation_GetNodeReturnsNilNodeNilError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	mockIndexers, _ := setupMockIndexers(mockCtrl)
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, mockIndexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}
	crdController.enableForceDetach = true

	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	orchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nil, nil)

	keyItem := makeKeyItem(t, tridentNamespace, nodeName)

	err = crdController.handleTridentNodeRemediation(keyItem)
	assert.NoError(t, err)
	updatedTNR, err := crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Get(ctx(), nodeName, getOpts)
	assert.NoError(t, err)
	assert.Equal(t, netappv1.TridentActionStateFailed, updatedTNR.Status.State)
	assert.Contains(t, updatedTNR.Status.Message, "node is nil after GetNode")
}

// Helper to construct a KeyItem with a "namespace/name" key
func makeKeyItem(t *testing.T, namespace, name string) *KeyItem {
	t.Helper()
	return &KeyItem{
		key: fmt.Sprintf("%s/%s", namespace, name),
		ctx: ctx(),
	}
}

// TestAreVolPublicationsRemoved_AllPublicationsRemoved tests that the function returns true
// when all volume publications have been removed from the node. This validates the successful
// completion condition for volume cleanup during node remediation.
func TestAreVolPublicationsRemoved_AllPublicationsRemoved(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	assert.NoError(t, err)

	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	tnr.Status.VolumeAttachments = map[string]string{"va-1": "vol-1", "va-2": "vol-2"}

	// Mock no volume publications remaining
	orchestrator.EXPECT().ListVolumePublicationsForNode(ctx(), nodeName).Return([]*models.VolumePublicationExternal{}, nil)

	removed, err := crdController.areVolPublicationsRemoved(ctx(), tnr, nodeName)
	assert.NoError(t, err)
	assert.True(t, removed)
}

// TestAreVolPublicationsRemoved_PublicationsStillExist tests that the function returns false
// when volume publications still exist on the node. This validates that the cleanup process
// should continue waiting for publications to be removed.
func TestAreVolPublicationsRemoved_PublicationsStillExist(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	assert.NoError(t, err)

	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	tnr.Status.VolumeAttachments = map[string]string{"va-0": "vol-0"}

	// Mock volume publications still existing
	publications := fakeVolumePublicationsExternal(1, nodeName)
	orchestrator.EXPECT().ListVolumePublicationsForNode(ctx(), nodeName).Return(publications, nil)

	removed, err := crdController.areVolPublicationsRemoved(ctx(), tnr, nodeName)
	assert.NoError(t, err)
	assert.False(t, removed)
}

// TestAreVolPublicationsRemoved_ListPublicationsError tests error handling when
// the orchestrator fails to list volume publications. This validates that
// orchestrator errors are properly propagated during publication checking.
func TestAreVolPublicationsRemoved_ListPublicationsError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	assert.NoError(t, err)

	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)

	// Mock error when listing volume publications
	orchestrator.EXPECT().ListVolumePublicationsForNode(ctx(), nodeName).Return(nil, fmt.Errorf("listing failed"))

	removed, err := crdController.areVolPublicationsRemoved(ctx(), tnr, nodeName)
	assert.Error(t, err)
	assert.False(t, removed)
	assert.Contains(t, err.Error(), "listing failed")
}

// TestAreVolPublicationsRemoved_EmptyVolumeAttachments tests the function's behavior
// when the CR has no volume attachments tracked. This validates handling of CRs
// that don't have any volumes to track for removal.
func TestAreVolPublicationsRemoved_EmptyVolumeAttachments(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	assert.NoError(t, err)

	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)

	// Mock no volume publications
	orchestrator.EXPECT().ListVolumePublicationsForNode(ctx(), nodeName).Return([]*models.VolumePublicationExternal{}, nil)

	removed, err := crdController.areVolPublicationsRemoved(ctx(), tnr, nodeName)
	assert.NoError(t, err)
	assert.True(t, removed)
}

// TestFailoverDetach_Success tests the successful execution of failover detach process
// including pod deletion and volume attachment removal for a failed node.
func TestFailoverDetach_Success(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	remediationUtils := mock_controller_crd.NewMockNodeRemediationUtils(mockCtrl)

	testPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "testNs",
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
			Volumes: []corev1.Volume{
				{
					Name: "vol1",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: "pvc-1",
						},
					},
				},
			},
		},
	}
	pvcToTvolMap := map[string]*storage.VolumeExternal{
		"pvc-1": {
			Config: &storage.VolumeConfig{
				Name:        "vol1",
				RequestName: "pvc-1",
			},
		},
	}

	remediationUtils.EXPECT().GetNodePods(ctx(), nodeName).Return([]*corev1.Pod{}, nil)
	remediationUtils.EXPECT().GetPvcToTvolMap(ctx(), nodeName).Return(pvcToTvolMap, nil)
	remediationUtils.EXPECT().GetPodsToDelete(ctx(), gomock.Any(), pvcToTvolMap).Return([]*corev1.Pod{testPod})
	remediationUtils.EXPECT().GetVolumeAttachmentsToDelete(ctx(), []*corev1.Pod{testPod}, pvcToTvolMap, nodeName).
		Return(map[string]string{"va1": "vol1"}, nil)
	remediationUtils.EXPECT().ForceDeletePod(ctx(), testPod).Return(nil)
	remediationUtils.EXPECT().DeleteVolumeAttachment(ctx(), "va1").Return(nil)

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, remediationUtils)
	assert.NoError(t, err)

	// Create test data
	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	err = crdController.failoverDetach(ctx(), tnr, nodeName)
	assert.NoError(t, err)
}

// TestFailoverDetach_GetNodePodsError tests error handling when retrieving node pods fails
func TestFailoverDetach_GetNodePodsError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	remediationUtils := mock_controller_crd.NewMockNodeRemediationUtils(mockCtrl)

	remediationUtils.EXPECT().GetNodePods(ctx(), nodeName).Return([]*corev1.Pod{}, fmt.Errorf("mock error"))

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, remediationUtils)
	assert.NoError(t, err)

	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	err = crdController.failoverDetach(ctx(), tnr, nodeName)
	assert.Error(t, err)
}

// TestFailoverDetach_GetPvcToTvolMapError tests error handling when retrieving PVC to volume mapping fails
func TestFailoverDetach_GetPvcToTvolMapError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	remediationUtils := mock_controller_crd.NewMockNodeRemediationUtils(mockCtrl)

	remediationUtils.EXPECT().GetNodePods(ctx(), nodeName).Return([]*corev1.Pod{}, nil)
	remediationUtils.EXPECT().GetPvcToTvolMap(ctx(), nodeName).Return(nil, fmt.Errorf("mock error"))

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, remediationUtils)
	assert.NoError(t, err)

	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	err = crdController.failoverDetach(ctx(), tnr, nodeName)
	assert.Error(t, err)
}

// TestFailoverDetach_GetVolumeAttachmentsToDeleteError tests error handling when retrieving VAs to delete
func TestFailoverDetach_GetVolumeAttachmentsToDeleteError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	remediationUtils := mock_controller_crd.NewMockNodeRemediationUtils(mockCtrl)

	remediationUtils.EXPECT().GetNodePods(ctx(), nodeName).Return([]*corev1.Pod{}, nil)
	remediationUtils.EXPECT().GetPvcToTvolMap(ctx(), nodeName).Return(map[string]*storage.VolumeExternal{}, nil)
	remediationUtils.EXPECT().GetPodsToDelete(ctx(), gomock.Any(), gomock.Any()).Return([]*corev1.Pod{})
	remediationUtils.EXPECT().GetVolumeAttachmentsToDelete(ctx(), gomock.Any(), gomock.Any(), nodeName).
		Return(map[string]string{}, fmt.Errorf("mock error"))

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, remediationUtils)
	assert.NoError(t, err)

	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
	assert.NoError(t, err)

	err = crdController.failoverDetach(ctx(), tnr, nodeName)
	assert.Error(t, err)
}

// TestFailoverDetach_UpdateCrVolAttachementsError tests error handling when failing to update VA status in the CR
func TestFailoverDetach_UpdateCrVolAttachementsError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	remediationUtils := mock_controller_crd.NewMockNodeRemediationUtils(mockCtrl)

	remediationUtils.EXPECT().GetNodePods(ctx(), nodeName).Return([]*corev1.Pod{}, nil)
	remediationUtils.EXPECT().GetPvcToTvolMap(ctx(), nodeName).Return(map[string]*storage.VolumeExternal{}, nil)
	remediationUtils.EXPECT().GetPodsToDelete(ctx(), gomock.Any(), gomock.Any()).Return([]*corev1.Pod{})
	remediationUtils.EXPECT().GetVolumeAttachmentsToDelete(ctx(), gomock.Any(), gomock.Any(), nodeName).
		Return(map[string]string{}, nil)

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, remediationUtils)
	assert.NoError(t, err)

	tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
	assert.NoError(t, err)

	err = crdController.failoverDetach(ctx(), tnr, nodeName)
	assert.Error(t, err)
	assert.ErrorContains(t, err, fmt.Sprintf("\"%s\" not found", nodeName))
}

// TestFailoverDetach_Scenarios tests various scenarios in the failoverDetach function
// using a parameterized approach to cover success and error cases systematically.
func TestFailoverDetach_Scenarios(t *testing.T) {
	tests := map[string]struct {
		setupMocks             func(*mock_controller_crd.MockNodeRemediationUtils)
		expectError            bool
		expectedErrorSubstring string
	}{
		"Success": {
			setupMocks: func(remediationUtils *mock_controller_crd.MockNodeRemediationUtils) {
				testPod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
				}
				pvcToTvolMap := map[string]*storage.VolumeExternal{
					"pvc1": {Config: &storage.VolumeConfig{Name: "vol1"}},
				}
				volumeAttachments := map[string]string{"va1": "vol1"}

				remediationUtils.EXPECT().GetNodePods(ctx(), nodeName).Return([]*corev1.Pod{}, nil)
				remediationUtils.EXPECT().GetPvcToTvolMap(ctx(), nodeName).Return(pvcToTvolMap, nil)
				remediationUtils.EXPECT().GetPodsToDelete(ctx(), gomock.Any(), pvcToTvolMap).Return([]*corev1.Pod{testPod})
				remediationUtils.EXPECT().GetVolumeAttachmentsToDelete(ctx(), []*corev1.Pod{testPod}, pvcToTvolMap, nodeName).
					Return(volumeAttachments, nil)
				remediationUtils.EXPECT().ForceDeletePod(ctx(), testPod).Return(nil)
				remediationUtils.EXPECT().DeleteVolumeAttachment(ctx(), "va1").Return(nil)
			},
			expectError: false,
		},
		"GetNodePodsError": {
			setupMocks: func(remediationUtils *mock_controller_crd.MockNodeRemediationUtils) {
				remediationUtils.EXPECT().GetNodePods(ctx(), nodeName).Return(nil, fmt.Errorf("mock error"))
			},
			expectError: true,
		},
		"GetPvcToTvolMapError": {
			setupMocks: func(remediationUtils *mock_controller_crd.MockNodeRemediationUtils) {
				remediationUtils.EXPECT().GetNodePods(ctx(), nodeName).Return([]*corev1.Pod{}, nil)
				remediationUtils.EXPECT().GetPvcToTvolMap(ctx(), nodeName).Return(nil, fmt.Errorf("mock error"))
			},
			expectError: true,
		},
		"GetVolumeAttachmentsToDeleteError": {
			setupMocks: func(remediationUtils *mock_controller_crd.MockNodeRemediationUtils) {
				remediationUtils.EXPECT().GetNodePods(ctx(), nodeName).Return([]*corev1.Pod{}, nil)
				remediationUtils.EXPECT().GetPvcToTvolMap(ctx(), nodeName).Return(map[string]*storage.VolumeExternal{}, nil)
				remediationUtils.EXPECT().GetPodsToDelete(ctx(), gomock.Any(), gomock.Any()).Return([]*corev1.Pod{})
				remediationUtils.EXPECT().GetVolumeAttachmentsToDelete(ctx(), gomock.Any(), gomock.Any(), nodeName).
					Return(nil, fmt.Errorf("mock error"))
			},
			expectError: true,
		},
		"ForceDeletePodError": {
			setupMocks: func(remediationUtils *mock_controller_crd.MockNodeRemediationUtils) {
				testPod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
				}
				pvcToTvolMap := map[string]*storage.VolumeExternal{}

				remediationUtils.EXPECT().GetNodePods(ctx(), nodeName).Return([]*corev1.Pod{}, nil)
				remediationUtils.EXPECT().GetPvcToTvolMap(ctx(), nodeName).Return(pvcToTvolMap, nil)
				remediationUtils.EXPECT().GetPodsToDelete(ctx(), gomock.Any(), pvcToTvolMap).Return([]*corev1.Pod{testPod})
				remediationUtils.EXPECT().GetVolumeAttachmentsToDelete(ctx(), []*corev1.Pod{testPod}, pvcToTvolMap, nodeName).
					Return(map[string]string{}, nil)
				remediationUtils.EXPECT().ForceDeletePod(ctx(), testPod).Return(fmt.Errorf("mock error"))
			},
			expectError: true,
		},
		"DeleteVolumeAttachmentError": {
			setupMocks: func(remediationUtils *mock_controller_crd.MockNodeRemediationUtils) {
				testPod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
				}
				pvcToTvolMap := map[string]*storage.VolumeExternal{}
				volumeAttachments := map[string]string{"va1": "vol1"}

				remediationUtils.EXPECT().GetNodePods(ctx(), nodeName).Return([]*corev1.Pod{}, nil)
				remediationUtils.EXPECT().GetPvcToTvolMap(ctx(), nodeName).Return(pvcToTvolMap, nil)
				remediationUtils.EXPECT().GetPodsToDelete(ctx(), gomock.Any(), pvcToTvolMap).Return([]*corev1.Pod{testPod})
				remediationUtils.EXPECT().GetVolumeAttachmentsToDelete(ctx(), []*corev1.Pod{testPod}, pvcToTvolMap, nodeName).
					Return(volumeAttachments, nil)
			},
			expectError:            true,
			expectedErrorSubstring: fmt.Sprintf("\"%s\" not found", nodeName),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			tridentNamespace := "trident"
			kubeClient := GetTestKubernetesClientset()
			snapClient := GetTestSnapshotClientset()
			crdClient := GetTestCrdClientset()
			remediationUtils := mock_controller_crd.NewMockNodeRemediationUtils(mockCtrl)

			tt.setupMocks(remediationUtils)

			crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
				crdClient, nil, remediationUtils)
			assert.NoError(t, err)

			tnr := fakeTridentNodeRemediation(nodeName, tridentNamespace, netappv1.TridentNodeRemediatingState)
			if name != "DeleteVolumeAttachmentError" {
				_, err = crdClient.TridentV1().TridentNodeRemediations(tridentNamespace).Create(ctx(), tnr, createOpts)
			}
			assert.NoError(t, err)

			err = crdController.failoverDetach(ctx(), tnr, nodeName)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrorSubstring != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorSubstring)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
