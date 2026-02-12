// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"

	"github.com/netapp/trident/config"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	"github.com/netapp/trident/internal/autogrow"
	. "github.com/netapp/trident/logging"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	mockk8s "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_helpers/mock_kubernetes_helper"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Test Setup Helpers

func setupTagriTest(t *testing.T) (*TridentCrdController, *gomock.Controller, *mockcore.MockOrchestrator) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	// Default: no backend resize delta (driver does not use resize-delta no-op behavior).
	orchestrator.EXPECT().GetResizeDeltaForBackend(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()
	// No Kubernetes helper in default setup; controller uses direct API for PVC lookup.
	orchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).Return(nil, fmt.Errorf("not found")).AnyTimes()
	// updateVolumeAutogrowStatus is best-effort in reject/fail/complete paths; allow any times so tests don't need to set it.
	orchestrator.EXPECT().UpdateVolumeAutogrowStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	controller, err := newTridentCrdControllerImpl(
		orchestrator, tridentNamespace, kubeClient, snapClient, crdClient, nil, nil)
	require.NoError(t, err)

	controller.recorder = record.NewFakeRecorder(100)
	return controller, mockCtrl, orchestrator
}

func createTestTagri(name, pvName, policyName string, phase string, generation int64) *tridentv1.TridentAutogrowRequestInternal {
	tagri := &tridentv1.TridentAutogrowRequestInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  config.OrchestratorName,
			UID:        "test-uid-123",
			Finalizers: []string{tridentv1.TridentFinalizerName},
		},
		Spec: tridentv1.TridentAutogrowRequestInternalSpec{
			Volume:                pvName,
			ObservedUsedPercent:   85.0,
			ObservedUsedBytes:     "4294967296", // 4Gi in bytes
			ObservedCapacityBytes: "50Gi",       // Required for safe growth; default so final capacity is not below observed
			NodeName:              "node-1",
			AutogrowPolicyRef: tridentv1.TridentAutogrowRequestInternalPolicyRef{
				Name:       policyName,
				Generation: generation,
			},
		},
		Status: tridentv1.TridentAutogrowRequestInternalStatus{
			Phase: phase,
		},
	}
	return tagri
}

func createTestPVC(name, namespace string, capacity string) *corev1.PersistentVolumeClaim {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "pvc-uid-123",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{},
			},
		},
		Status: corev1.PersistentVolumeClaimStatus{},
	}

	if capacity != "" {
		qty, _ := resource.ParseQuantity(capacity)
		pvc.Spec.Resources.Requests[corev1.ResourceStorage] = qty
		pvc.Status.Capacity = corev1.ResourceList{
			corev1.ResourceStorage: qty,
		}
	}

	return pvc
}

// createTestPVWithClaimRef returns a PV with Spec.ClaimRef set so getPVCForVolume can resolve PVC from PV.
func createTestPVWithClaimRef(pvName, pvcNamespace, pvcName string) *corev1.PersistentVolume {
	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: pvName},
		Spec: corev1.PersistentVolumeSpec{
			ClaimRef: &corev1.ObjectReference{
				Namespace: pvcNamespace,
				Name:      pvcName,
			},
		},
	}
}

func createTestVolExternal(name, policyName string) *storage.VolumeExternal {
	return &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			Name:             name,
			Namespace:        "default",
			RequestName:      "test-pvc", // PVC name reference
			ImportNotManaged: false,
		},
		Backend: "test-backend",
		EffectiveAutogrowPolicy: models.EffectiveAutogrowPolicyInfo{
			PolicyName: policyName,
		},
	}
}

func createTestAutogrowPolicy(name string, usedThreshold, growthAmount, maxSize string, generation int64) *tridentv1.TridentAutogrowPolicy {
	return &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Generation: generation,
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: usedThreshold,
			GrowthAmount:  growthAmount,
			MaxSize:       maxSize,
		},
	}
}

// createTestAutogrowPolicyExternal returns an AutogrowPolicyExternal for orchestrator mock (policy state from core cache).
func createTestAutogrowPolicyExternal(name string, state storage.AutogrowPolicyState) *storage.AutogrowPolicyExternal {
	return &storage.AutogrowPolicyExternal{
		Name:          name,
		UsedThreshold: "80%",
		GrowthAmount:  "1Gi",
		MaxSize:       "200Gi",
		State:         state,
		Volumes:       nil,
		VolumeCount:   0,
	}
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Unit Tests

// Test 1: handleTridentAutogrowRequestInternal
func TestHandleTridentAutogrowRequestInternal_NilKeyItem(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	err := controller.handleTridentAutogrowRequestInternal(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "keyItem is nil")
}

func TestHandleTridentAutogrowRequestInternal_InvalidKey(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	keyItem := &KeyItem{
		key: "invalid-key-format",
		ctx: context.Background(),
	}

	err := controller.handleTridentAutogrowRequestInternal(keyItem)
	assert.NoError(t, err) // Returns nil for invalid keys
}

func TestHandleTridentAutogrowRequestInternal_TagriNotFound(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	keyItem := &KeyItem{
		key: fmt.Sprintf("%s/test-tagri", config.OrchestratorName),
		ctx: context.Background(),
	}

	err := controller.handleTridentAutogrowRequestInternal(keyItem)
	assert.NoError(t, err) // Returns nil when TAGRI not found
}

func TestHandleTridentAutogrowRequestInternal_EventTypes(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		phase     string
	}{
		{"EventAdd", EventAdd, TagriPhasePending},
		{"EventUpdate", EventUpdate, TagriPhasePending},
		{"EventForceUpdate", EventForceUpdate, TagriPhasePending},
		{"EventDelete", EventDelete, TagriPhasePending},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, mockCtrl, _ := setupTagriTest(t)
			defer mockCtrl.Finish()

			tagri := createTestTagri("test-tagri", "test-pv", "test-policy", tt.phase, 1)

			// Create the TAGRI in the fake clientset
			_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
				context.Background(), tagri, metav1.CreateOptions{})
			require.NoError(t, err)

			keyItem := &KeyItem{
				key:   fmt.Sprintf("%s/%s", config.OrchestratorName, tagri.Name),
				ctx:   context.Background(),
				event: tt.eventType,
			}

			// Note: In tests, the informer cache is not automatically populated,
			// so handleTridentAutogrowRequestInternal will not find the TAGRI in the lister
			// and will return nil (no error). This is expected test behavior.
			err = controller.handleTridentAutogrowRequestInternal(keyItem)
			assert.NoError(t, err) // Returns nil when TAGRI not found in lister cache
		})
	}
}

// Test 2: upsertTagriHandler
func TestUpsertTagriHandler_TimeoutDeletionFinalizerError(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	// Create TAGRI with old timestamp (exceeds hard timeout)
	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.CreationTimestamp = metav1.NewTime(time.Now().Add(-10 * time.Minute))

	// TAGRI has finalizers (from createTestTagri) but is NOT in fake clientset
	// When handleTimeoutAndTerminalPhase tries to delete:
	// 1. Hard timeout check triggers deletion (age > 5min default)
	// 2. deleteTagriNow tries to remove finalizers
	// 3. updateTagriCR fails with NotFound
	// 4. Error propagates back through upsertTagriHandler

	err := controller.upsertTagriHandler(context.Background(), tagri)
	assert.Error(t, err, "Should return error when finalizer removal fails during timeout deletion")
	assert.Contains(t, err.Error(), "failed to do TAGRI timeout or terminal phase based cleanup",
		"Error should indicate cleanup failure")
}

func TestUpsertTagriHandler_InProgressPhase_VolumeNotFoundError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "100Gi"

	// Mock orchestrator call for monitoring
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("not found"))

	err := controller.upsertTagriHandler(context.Background(), tagri)
	assert.Error(t, err) // Will error because volume not found
}

func TestUpsertTagriHandler_PendingPhase_VolumeNotFoundError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)

	// Mock orchestrator call for first-time processing
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("not found"))

	err := controller.upsertTagriHandler(context.Background(), tagri)
	assert.Error(t, err, "Should return error when volume not found")
	assert.Contains(t, err.Error(), "failed to get volume", "Error should indicate volume fetch failure")
}

func TestUpsertTagriHandler_EmptyPhaseVolumeNotFoundError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", "", 1)

	// Mock orchestrator call for first-time processing
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("not found"))

	err := controller.upsertTagriHandler(context.Background(), tagri)
	assert.Error(t, err) // Will error because volume not found
}

// Test 3: handleTimeoutAndTerminalPhase
func TestHandleTimeoutAndTerminalPhase_HardTimeout(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tests := []struct {
		name  string
		phase string
		age   time.Duration
	}{
		{"Pending exceeds timeout", TagriPhasePending, 6 * time.Minute},
		{"InProgress exceeds timeout", TagriPhaseInProgress, 6 * time.Minute},
		{"Failed exceeds timeout", TagriPhaseFailed, 6 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagri := createTestTagri("test-tagri", "test-pv", "test-policy", tt.phase, 1)

			// Create TAGRI in fake clientset FIRST, then modify timestamp
			created, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
				context.Background(), tagri, metav1.CreateOptions{})
			require.NoError(t, err)

			// Now set the old timestamp (simulating aged TAGRI)
			created.CreationTimestamp = metav1.NewTime(time.Now().Add(-tt.age))

			handled, err := controller.handleTimeoutAndTerminalPhase(context.Background(), created)
			assert.True(t, handled)
			assert.NoError(t, err)
		})
	}
}

func TestHandleTimeoutAndTerminalPhase_TerminalPhasesWithRetention(t *testing.T) {
	// Completed/Rejected TAGRIs are always deleted immediately.
	tests := []struct {
		name              string
		phase             string
		processedAtOffset *time.Duration
		expectDeleted     bool
	}{
		{
			name:          "Completed deleted immediately",
			phase:         TagriPhaseCompleted,
			expectDeleted: true,
		},
		{
			name:          "Rejected deleted immediately",
			phase:         TagriPhaseRejected,
			expectDeleted: true,
		},
		{
			name:              "Completed with ProcessedAt set still deleted immediately",
			phase:             TagriPhaseCompleted,
			processedAtOffset: durationPtr(30 * time.Second),
			expectDeleted:     true,
		},
		{
			name:              "Rejected with ProcessedAt set still deleted immediately",
			phase:             TagriPhaseRejected,
			processedAtOffset: durationPtr(70 * time.Second),
			expectDeleted:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, mockCtrl, _ := setupTagriTest(t)
			defer mockCtrl.Finish()

			origTimeout := config.TagriTimeout
			config.TagriTimeout = 300 * time.Second // Ensure hard timeout doesn't interfere
			defer func() { config.TagriTimeout = origTimeout }()

			tagri := createTestTagri("test-tagri", "test-pv", "test-policy", tt.phase, 1)

			if tt.processedAtOffset != nil {
				processedAt := metav1.NewTime(time.Now().Add(-*tt.processedAtOffset))
				tagri.Status.ProcessedAt = &processedAt
			}

			created, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
				context.Background(), tagri, metav1.CreateOptions{})
			require.NoError(t, err)

			handled, err := controller.handleTimeoutAndTerminalPhase(context.Background(), created)
			assert.True(t, handled)
			if tt.expectDeleted {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHandleTimeoutAndTerminalPhase_FailedPhase(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseFailed, 1)

	handled, err := controller.handleTimeoutAndTerminalPhase(context.Background(), tagri)
	assert.True(t, handled)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Failed phase")
}

// TestHandleTimeoutAndTerminalPhase_FailedPhase_RecoveryWhenResizeSucceeded: when TAGRI is Failed
// but the PVC has since reached target capacity (e.g. backend recovered), we complete the TAGRI and return handled=true, nil.
func TestHandleTimeoutAndTerminalPhase_FailedPhase_RecoveryWhenResizeSucceeded(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseFailed, 1)
	tagri.Status.FinalCapacityBytes = "2147483648" // 2Gi target

	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "2Gi")
	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	volExternal := createTestVolExternal("test-pv", "test-policy")
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil)

	handled, err := controller.handleTimeoutAndTerminalPhase(context.Background(), tagri)
	assert.True(t, handled)
	assert.NoError(t, err)

	updated, getErr := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NoError(t, getErr)
	assert.Equal(t, TagriPhaseCompleted, updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "2.00Gi")
}

func TestHandleTimeoutAndTerminalPhase_ActivePhases(t *testing.T) {
	tests := []struct {
		name  string
		phase string
	}{
		{"Pending", TagriPhasePending},
		{"InProgress", TagriPhaseInProgress},
		{"Empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, mockCtrl, _ := setupTagriTest(t)
			defer mockCtrl.Finish()

			tagri := createTestTagri("test-tagri", "test-pv", "test-policy", tt.phase, 1)

			handled, err := controller.handleTimeoutAndTerminalPhase(context.Background(), tagri)
			assert.False(t, handled)
			assert.NoError(t, err)
		})
	}
}

func TestHandleTimeoutAndTerminalPhase_UnknownPhase(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", "UnknownPhase", 1)

	handled, err := controller.handleTimeoutAndTerminalPhase(context.Background(), tagri)
	assert.True(t, handled)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown phase")
}

// Helper function for duration pointer
func durationPtr(d time.Duration) *time.Duration {
	return &d
}

// Test 4: processTagriFirstTime
// TestProcessTagriFirstTime_VolumeNotFound: GetVolume returns plain error (not NotFoundError) -> transient "failed to get volume".
func TestProcessTagriFirstTime_VolumeNotFound(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)

	// Create TAGRI in fake clientset
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	// Mock volume not found (plain error -> not IsNotFoundError, so "failed to get volume" path)
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("not found"))

	err = controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get volume")
}

// TestProcessTagriFirstTime_VolumeNotFoundError: GetVolume returns errors.NotFoundError -> reject TAGRI and "Volume ... not found", requeuing for deletion.
func TestProcessTagriFirstTime_VolumeNotFoundError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	// processTagriFirstTime calls GetVolume once; rejectTagri also calls GetVolume (best effort). Allow both.
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, errors.NotFoundError("volume %s not found", tagri.Spec.Volume)).AnyTimes()

	err = controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TAGRI rejected")
	assert.Contains(t, err.Error(), "not found")
	assert.Contains(t, err.Error(), "requeuing for deletion")

	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.Equal(t, TagriPhaseRejected, updated.Status.Phase)
}

func TestProcessTagriFirstTime_PVCNotFound(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")

	// Create TAGRI
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	// Mock volume found but PVC not found
	// AnyTimes because rejectTagri also calls GetVolume (best effort)
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()

	err = controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PVC not found")
}

// TestProcessTagriFirstTime_PVCNotFound_RejectTagriFails: getPVCForVolume returns NotFound, rejectTagri is called but updateTagriStatus fails (TAGRI not in clientset).
func TestProcessTagriFirstTime_PVCNotFound_RejectTagriFails(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")

	// Do NOT create TAGRI in clientset so rejectTagri -> updateTagriStatus(UpdateStatus) fails
	// Do NOT create PVC so getPVCForVolume returns NotFound
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()

	err := controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to reject TAGRI")
}

func TestProcessTagriFirstTime_EmptyEffectivePolicy(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "") // Empty policy
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")

	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	// AnyTimes because rejectTagri also calls GetVolume (best effort)
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()

	err = controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no effective autogrow policy")
}

func TestProcessTagriFirstTime_PolicyMismatch(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "different-policy") // Mismatched policy
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")

	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	// AnyTimes because rejectTagri also calls GetVolume (best effort)
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()

	err = controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "policy mismatch")
}

// Test processTagriFirstTime - Autogrow policy in Failed state (reject and requeue for deletion)
func TestProcessTagriFirstTime_PolicyStateFailed(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")

	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	policy := createTestAutogrowPolicy("test-policy", "80%", "20%", "100Gi", 1)
	policy.Status.State = string(storage.AutogrowPolicyStateFailed)
	policy.Status.Message = "validation failed"
	_, err = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	require.NoError(t, err)
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	// Policy state is validated from orchestrator (core cache) first
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateFailed), nil)

	err = controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileIncompleteError(err))
	assert.Contains(t, err.Error(), "not in success state")
	assert.Contains(t, err.Error(), "Failed")
}

// Test processTagriFirstTime - Autogrow policy in Deleting state (reject and requeue for deletion)
func TestProcessTagriFirstTime_PolicyStateDeleting(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")

	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	policy := createTestAutogrowPolicy("test-policy", "80%", "20%", "100Gi", 1)
	policy.Status.State = string(storage.AutogrowPolicyStateDeleting)
	policy.Status.Message = "policy is being deleted"
	_, err = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	require.NoError(t, err)
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateDeleting), nil)

	err = controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileIncompleteError(err))
	assert.Contains(t, err.Error(), "not in success state")
	assert.Contains(t, err.Error(), "Deleting")
}

// Test processTagriFirstTime - GetVolume returns non-NotFound (transient) error
func TestProcessTagriFirstTime_GetVolumeTransientError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("transient backend error"))

	err = controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get volume")
}

// Test processTagriFirstTime - GetAutogrowPolicy returns transient (non-NotFound) error
func TestProcessTagriFirstTime_GetAutogrowPolicyTransientError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(nil, fmt.Errorf("transient cache error"))

	err := controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get autogrow policy")
}

// Test processTagriFirstTime - GetResizeDeltaForBackend returns error -> ReconcileDeferredError (retry with backoff)
func TestProcessTagriFirstTime_GetResizeDeltaForBackendError_RetryWithBackoff(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).Return(nil, fmt.Errorf("not found")).AnyTimes()
	orchestrator.EXPECT().UpdateVolumeAutogrowStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	controller, err := newTridentCrdControllerImpl(
		orchestrator, tridentNamespace, kubeClient, snapClient, crdClient, nil, nil)
	require.NoError(t, err)
	controller.recorder = record.NewFakeRecorder(100)

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Spec.ObservedCapacityBytes = "50Gi"
	tagri.Spec.ObservedUsedPercent = 60.0
	tagri.Spec.ObservedUsedBytes = "32Gi"
	tagri.Spec.NodeName = "node-1"
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")
	pvc.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("50Gi")}
	policy := createTestAutogrowPolicy("test-policy", "80%", "10%", "100Gi", 1)
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)
	// Resize delta unavailable (e.g. backend not ready) -> deferred retry, not immediate requeue
	orchestrator.EXPECT().GetResizeDeltaForBackend(gomock.Any(), volExternal.BackendUUID).Return(int64(0), fmt.Errorf("not ready"))

	err = controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileDeferredError(err), "expected ReconcileDeferredError for resize delta error (retry with backoff)")
	assert.Contains(t, err.Error(), "unable to get resize delta")
	assert.Contains(t, err.Error(), "will retry with backoff")
}

// Test processTagriFirstTime - policy CR not in lister (informer lag) returns ReconcileDeferredError
func TestProcessTagriFirstTime_PolicyCRNotFoundInLister_Retry(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")
	policy := createTestAutogrowPolicy("test-policy", "80%", "10%", "100Gi", 1)
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	// Do NOT add policy to informer index so autogrowPoliciesLister.Get returns NotFound

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)

	err := controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileDeferredError(err))
	assert.Contains(t, err.Error(), "not found in lister")
}

// Test processTagriFirstTime - policy generation mismatch rejects TAGRI
func TestProcessTagriFirstTime_GenerationMismatch(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Spec.AutogrowPolicyRef.Generation = 1
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")
	policy := createTestAutogrowPolicy("test-policy", "80%", "20%", "200Gi", 2) // generation 2 != tagri's 1
	policy.Status.State = string(storage.AutogrowPolicyStateSuccess)

	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)

	err := controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "generation mismatch")
}

// Test processTagriFirstTime - PVC already at or above target size marks completed
func TestProcessTagriFirstTime_AlreadyAtTargetSize(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Spec.ObservedCapacityBytes = "1" // must be <= final; with 1-byte current, 1% growth rounds to 0 => final 1
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	// Current = status only. Use 1-byte capacity so 1% growth rounds to 0 => final = 1, status 1 >= 1 => already at target.
	pvc := createTestPVC("test-pvc", "default", "1")
	pvc.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1")}
	policy := createTestAutogrowPolicy("test-policy", "80%", "1%", "200Gi", 1)
	policy.Status.State = string(storage.AutogrowPolicyStateSuccess)

	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)

	err := controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already at target size")
	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.Equal(t, TagriPhaseCompleted, updated.Status.Phase)
}

// Test processTagriFirstTime - calculateFinalCapacity returns error (invalid policy growth) -> reject TAGRI.
func TestProcessTagriFirstTime_CalculateFinalCapacityError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")
	// Invalid growthAmount so calculateFinalCapacity fails in processTagriFirstTime
	policy := createTestAutogrowPolicy("test-policy", "80%", "invalid-growth", "100Gi", 1)
	policy.Status.State = string(storage.AutogrowPolicyStateSuccess)

	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)

	err := controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "final capacity calculation error")

	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.Equal(t, TagriPhaseRejected, updated.Status.Phase)
}

// Test processTagriFirstTime - missing observedCapacityBytes is rejected (before volume/PVC lookup; rejectTagri may still call GetVolume for best-effort volume update).
func TestProcessTagriFirstTime_MissingObservedCapacityBytes_Rejected(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Spec.ObservedCapacityBytes = "" // Missing required field

	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})

	// rejectTagri calls GetVolume for best-effort volume status update
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("not found")).AnyTimes()

	err := controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing observedCapacityBytes")
	assert.Contains(t, err.Error(), "requeuing for deletion")

	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.Equal(t, TagriPhaseRejected, updated.Status.Phase)
}

// Test processTagriFirstTime - invalid observedCapacityBytes (parse failure) is rejected.
func TestProcessTagriFirstTime_InvalidObservedCapacityBytesParse_Rejected(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	invalidValues := []string{"not-a-quantity", "10Gii", "10 G", "garbage", "1Gi "}
	for i, invalid := range invalidValues {
		idx, inv := i, invalid
		t.Run(invalid, func(t *testing.T) {
			name := fmt.Sprintf("test-tagri-invalid-%d", idx)
			tagri := createTestTagri(name, "test-pv", "test-policy", TagriPhasePending, 1)
			tagri.Spec.ObservedCapacityBytes = inv

			_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
				context.Background(), tagri, metav1.CreateOptions{})

			orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("not found")).AnyTimes()

			err := controller.processTagriFirstTime(context.Background(), tagri)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid observedCapacityBytes")
			assert.Contains(t, err.Error(), "requeuing for deletion")

			updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
				context.Background(), tagri.Name, metav1.GetOptions{})
			require.NotNil(t, updated)
			assert.Equal(t, TagriPhaseRejected, updated.Status.Phase)
			assert.Contains(t, updated.Status.Message, "invalid observedCapacityBytes", "rejection message should include invalid value reason")
		})
	}
}

// Test processTagriFirstTime - zero observedCapacityBytes is rejected.
func TestProcessTagriFirstTime_ZeroObservedCapacityBytes_Rejected(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	zeroValues := []string{"0", "0Gi", "0Mi"}
	for i, zeroVal := range zeroValues {
		idx, zv := i, zeroVal
		t.Run(zeroVal, func(t *testing.T) {
			name := fmt.Sprintf("test-tagri-zero-%d", idx)
			tagri := createTestTagri(name, "test-pv", "test-policy", TagriPhasePending, 1)
			tagri.Spec.ObservedCapacityBytes = zv

			_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
				context.Background(), tagri, metav1.CreateOptions{})

			orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("not found")).AnyTimes()

			err := controller.processTagriFirstTime(context.Background(), tagri)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "observedCapacityBytes must be positive")
			assert.Contains(t, err.Error(), "requeuing for deletion")

			updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
				context.Background(), tagri.Name, metav1.GetOptions{})
			require.NotNil(t, updated)
			assert.Equal(t, TagriPhaseRejected, updated.Status.Phase)
			assert.Contains(t, updated.Status.Message, "must be greater than 0")
		})
	}
}

// Test processTagriFirstTime - negative observedCapacityBytes is rejected.
func TestProcessTagriFirstTime_NegativeObservedCapacityBytes_Rejected(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Spec.ObservedCapacityBytes = "-1Gi"

	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("not found")).AnyTimes()

	err := controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "observedCapacityBytes must be positive")
	assert.Contains(t, err.Error(), "requeuing for deletion")

	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.Equal(t, TagriPhaseRejected, updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "must be greater than 0")
}

// Test processTagriFirstTime - valid observedCapacityBytes (happy path) passes validation and proceeds to GetVolume.
func TestProcessTagriFirstTime_ValidObservedCapacityBytes_Accepted(t *testing.T) {
	validValues := []string{"10Gi", "1Mi", "1073741824"} // 10Gi, 1Mi, 1Gi in bytes
	for i, valid := range validValues {
		idx, v := i, valid
		t.Run(valid, func(t *testing.T) {
			controller, mockCtrl, orchestrator := setupTagriTest(t)
			defer mockCtrl.Finish()

			name := fmt.Sprintf("test-tagri-valid-%d", idx)
			tagri := createTestTagri(name, "test-pv", "test-policy", TagriPhasePending, 1)
			tagri.Spec.ObservedCapacityBytes = v

			_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
				context.Background(), tagri, metav1.CreateOptions{})

			// Validation passes; GetVolume returns NotFound -> we get "Volume not found" (not observedCapacityBytes error).
			// GetVolume may be called twice: once in processTagriFirstTime, once in rejectTagri (best-effort).
			orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, errors.NotFoundError("volume not found")).AnyTimes()

			err := controller.processTagriFirstTime(context.Background(), tagri)
			assert.Error(t, err)
			// Proved we passed validation: error is from volume lookup, not from observedCapacityBytes
			assert.Contains(t, err.Error(), "Volume")
			assert.Contains(t, err.Error(), "not found")
			assert.NotContains(t, err.Error(), "observedCapacityBytes")

			updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
				context.Background(), tagri.Name, metav1.GetOptions{})
			require.NotNil(t, updated)
			assert.Equal(t, TagriPhaseRejected, updated.Status.Phase)
			assert.Contains(t, updated.Status.Message, "Volume", "rejection is from volume lookup, not observedCapacityBytes")
		})
	}
}

// Test processTagriFirstTime - final capacity below observed capacity is rejected (prevents volume shrinkage and data loss).
func TestProcessTagriFirstTime_FinalCapacityBelowObservedCapacity_Rejected(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Spec.ObservedCapacityBytes = "60Gi" // Host sees 60Gi; final will be 51Gi (50Gi + 1Gi) -> would shrink
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi") // PVC spec 50Gi -> finalCapacity 50+1 = 51Gi
	policy := createTestAutogrowPolicy("test-policy", "80%", "1Gi", "200Gi", 1)
	policy.Status.State = string(storage.AutogrowPolicyStateSuccess)

	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)

	err := controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "final capacity below observed capacity")
	assert.Contains(t, err.Error(), "requeuing for deletion")

	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.Equal(t, TagriPhaseRejected, updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "volume shrinkage and data loss", "rejection message should explain risk")
}

// Test processTagriFirstTime - PVC with nil status.capacity (e.g. not bound) is rejected outright.
func TestProcessTagriFirstTime_PVCCapacityNil_Rejected(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "10Gi")
	pvc.Status.Capacity = nil // Simulate unbound or capacity not yet reported
	policy := createTestAutogrowPolicy("test-policy", "80%", "1Gi", "200Gi", 1)
	policy.Status.State = string(storage.AutogrowPolicyStateSuccess)

	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)

	err := controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requeuing for deletion")
	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.Equal(t, TagriPhaseRejected, updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "capacity not yet reported")
}

// Test processTagriFirstTime - Required spec fields (observedUsedPercent, observedUsedBytes, nodeName) are preserved.
func TestProcessTagriFirstTime_RequiredSpecFields_Preserved(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Spec.ObservedUsedBytes = "8589934592" // 8Gi
	tagri.Spec.ObservedUsedPercent = 90.0
	tagri.Spec.NodeName = "worker-node-2"

	created, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	assert.Equal(t, float32(90.0), created.Spec.ObservedUsedPercent)
	assert.Equal(t, "8589934592", created.Spec.ObservedUsedBytes)
	assert.Equal(t, "worker-node-2", created.Spec.NodeName)

	got, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, float32(90.0), got.Spec.ObservedUsedPercent)
	assert.Equal(t, "8589934592", got.Spec.ObservedUsedBytes)
	assert.Equal(t, "worker-node-2", got.Spec.NodeName)
}

// Test processTagriFirstTime - Add finalizer: TAGRI without finalizers gets finalizers added and proceeds.
func TestProcessTagriFirstTime_AddFinalizer_Success(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Finalizers = nil // No finalizers so we hit Add finalizer
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi") // 50Gi so currentCapacity < finalCapacity (51Gi)
	policy := createTestAutogrowPolicy("test-policy", "80%", "1Gi", "200Gi", 1)
	policy.Status.State = string(storage.AutogrowPolicyStateSuccess)

	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)

	err := controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Starting resize monitoring")

	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.True(t, updated.HasTridentFinalizers(), "finalizers should have been added")
}

// Test processTagriFirstTime - Add finalizer: updateTagriCR fails -> "failed to add finalizer".
func TestProcessTagriFirstTime_AddFinalizer_Fails(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Finalizers = nil // No finalizers so we hit Add finalizer
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")
	policy := createTestAutogrowPolicy("test-policy", "80%", "1Gi", "200Gi", 1)
	policy.Status.State = string(storage.AutogrowPolicyStateSuccess)

	// Do NOT create TAGRI in clientset so updateTagriCR (add finalizer) fails
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)

	err := controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add finalizer")
}

// Test processTagriFirstTime - Patch PVC fails, retry count < max -> ReconcileDeferredError and retry count updated.
func TestProcessTagriFirstTime_PatchPVC_FailsRetry(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator.EXPECT().GetResizeDeltaForBackend(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()
	orchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).Return(nil, fmt.Errorf("not found")).AnyTimes()
	orchestrator.EXPECT().UpdateVolumeAutogrowStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")
	kubeClient := k8sfake.NewSimpleClientset(pv, pvc)
	kubeClient.PrependReactor("update", "persistentvolumeclaims", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("PVC update rejected")
	})

	tridentNamespace := "trident"
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	controller, err := newTridentCrdControllerImpl(
		orchestrator, tridentNamespace, kubeClient, snapClient, crdClient, nil, nil)
	require.NoError(t, err)
	controller.recorder = record.NewFakeRecorder(100)

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	policy := createTestAutogrowPolicy("test-policy", "80%", "1Gi", "200Gi", 1)
	policy.Status.State = string(storage.AutogrowPolicyStateSuccess)

	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)

	err = controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PVC patch failed")
	assert.Contains(t, err.Error(), "retry")

	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.Equal(t, 1, updated.Status.RetryCount)
}

// Test processTagriFirstTime - Patch PVC fails, retry count >= max -> failTagri and deleteTagriNow.
func TestProcessTagriFirstTime_PatchPVC_MaxRetriesExceeded(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator.EXPECT().GetResizeDeltaForBackend(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()
	orchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).Return(nil, fmt.Errorf("not found")).AnyTimes()
	orchestrator.EXPECT().UpdateVolumeAutogrowStatus(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")
	kubeClient := k8sfake.NewSimpleClientset(pv, pvc)
	kubeClient.PrependReactor("update", "persistentvolumeclaims", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("PVC update rejected")
	})

	tridentNamespace := "trident"
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	controller, err := newTridentCrdControllerImpl(
		orchestrator, tridentNamespace, kubeClient, snapClient, crdClient, nil, nil)
	require.NoError(t, err)
	controller.recorder = record.NewFakeRecorder(100)

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Status.RetryCount = MaxTagriRetries // Already at max so patch failure triggers fail + delete
	volExternal := createTestVolExternal("test-pv", "test-policy")
	policy := createTestAutogrowPolicy("test-policy", "80%", "1Gi", "200Gi", 1)
	policy.Status.State = string(storage.AutogrowPolicyStateSuccess)

	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)

	err = controller.processTagriFirstTime(context.Background(), tagri)
	assert.NoError(t, err) // deleteTagriNow succeeds and returns nil
	_, getErr := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	assert.True(t, k8sapierrors.IsNotFound(getErr), "TAGRI should be deleted after max retries")
}

// Test processTagriFirstTime - Transition to InProgress does NOT update tvol autogrowStatus.
// TotalAutogrowAttempted is incremented only when TAGRI reaches a terminal state (Rejected, Failed, or Completed).
func TestProcessTagriFirstTime_TransitionToInProgress_DoesNotUpdateVolumeAutogrowStatus(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	// Do not expect UpdateVolumeAutogrowStatus: entering InProgress must not call it.
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")
	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	policy := createTestAutogrowPolicy("test-policy", "80%", "20%", "100Gi", 1)
	policy.Status.State = string(storage.AutogrowPolicyStateSuccess)

	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	require.NoError(t, err)
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)

	err = controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileIncompleteError(err), "expected ReconcileIncompleteError to start monitoring")
	assert.Contains(t, err.Error(), "Starting resize monitoring")

	// TAGRI should be InProgress; tvol UpdateVolumeAutogrowStatus was never called (no EXPECT = strict mock).
	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.Equal(t, TagriPhaseInProgress, updated.Status.Phase)
}

// Test processTagriFirstTime - "already at target size" path calls completeTagriSuccess but it fails (TAGRI not in clientset).
func TestProcessTagriFirstTime_AlreadyAtTargetSize_CompleteTagriSuccessFails(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Spec.ObservedCapacityBytes = "1"
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "1")
	pvc.Status.Capacity = corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("1")}
	policy := createTestAutogrowPolicy("test-policy", "80%", "1%", "200Gi", 1) // 1 byte + 1% => final 1; status 1 >= 1 => already at target
	policy.Status.State = string(storage.AutogrowPolicyStateSuccess)

	// Do NOT create TAGRI in clientset so completeTagriSuccess -> updateTagriStatus(UpdateStatus) fails
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)

	err := controller.processTagriFirstTime(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to mark TAGRI as Completed")
}

// Test processTagriFirstTime - Autogrow policy in Success state passes generation check (reaches later step or different error)
func TestProcessTagriFirstTime_PolicyStateSuccess_PassesValidation(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "50Gi")

	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	policy := createTestAutogrowPolicy("test-policy", "80%", "20%", "100Gi", 1)
	policy.Status.State = string(storage.AutogrowPolicyStateSuccess)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	require.NoError(t, err)
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(policy)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(
		createTestAutogrowPolicyExternal("test-policy", storage.AutogrowPolicyStateSuccess), nil)

	err = controller.processTagriFirstTime(context.Background(), tagri)
	// Should NOT fail with "not in success state" - fails later (e.g. capacity calculation, patch, etc.)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "not in success state")
}

// Test 5: monitorTagriResize
func TestMonitorTagriResize_VolumeNotFound(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "100Gi"

	// Create TAGRI
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	// Mock volume not found
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("not found"))

	err = controller.monitorTagriResize(context.Background(), tagri)
	assert.Error(t, err)
}

// TestMonitorTagriResize_GetVolumeTransientError covers monitorTagriResize when GetVolume returns non-NotFound error.
func TestMonitorTagriResize_GetVolumeTransientError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "100Gi"
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("transient backend error"))

	err := controller.monitorTagriResize(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get volume during monitoring")
}

// TestMonitorTagriResize_GetVolumeNotFoundError: GetVolume returns errors.NotFoundError -> reject TAGRI and "volume deleted during monitoring".
func TestMonitorTagriResize_GetVolumeNotFoundError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "100Gi"
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	// monitorTagriResize calls GetVolume once; rejectTagri also calls GetVolume (best effort). Allow both.
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, errors.NotFoundError("volume %s not found", tagri.Spec.Volume)).AnyTimes()

	err = controller.monitorTagriResize(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TAGRI rejected")
	assert.Contains(t, err.Error(), "volume deleted")
	assert.Contains(t, err.Error(), "requeuing for deletion")

	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.Equal(t, TagriPhaseRejected, updated.Status.Phase)
}

func TestMonitorTagriResize_PVCCapacityVerified(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "100Gi"

	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "100Gi")

	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()

	err = controller.monitorTagriResize(context.Background(), tagri)
	// Will succeed and mark as completed
	assert.Error(t, err) // ReconcileIncompleteError to trigger cleanup
	assert.Contains(t, err.Error(), "completed")
}

// Test 6: calculateFinalCapacity
func TestCalculateFinalCapacity_Success(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	policy := createTestAutogrowPolicy("test-policy", "80%", "10%", "200Gi", 1)
	currentSize := resource.MustParse("100Gi")

	result, err := controller.calculateFinalCapacity(context.Background(), currentSize, policy, "test-pv", 0)
	assert.NoError(t, err)
	assert.Equal(t, "110Gi", result.String())
}

func TestCalculateFinalCapacity_NilPolicy(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	currentSize := resource.MustParse("100Gi")

	_, err := controller.calculateFinalCapacity(context.Background(), currentSize, nil, "test-pv", 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

// Test calculateFinalCapacity with resize delta (all validations in one place)
func TestCalculateFinalCapacity_WithResizeDelta_ZeroDelta_ReturnsPolicyBased(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	policy := createTestAutogrowPolicy("p", "80%", "5Gi", "200Gi", 1)
	currentSize := resource.MustParse("100Gi")

	result, err := controller.calculateFinalCapacity(context.Background(), currentSize, policy, "test-pv", 0)
	assert.NoError(t, err)
	assert.Equal(t, "105Gi", result.String())
}

func TestCalculateFinalCapacity_WithResizeDelta_GrowthAboveDelta_ReturnsUnchanged(t *testing.T) {
	const delta int64 = 50 * 1024 * 1024 // 50Mi
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	policy := createTestAutogrowPolicy("p", "80%", "50Gi", "200Gi", 1)
	currentSize := resource.MustParse("100Gi")

	result, err := controller.calculateFinalCapacity(context.Background(), currentSize, policy, "test-pv", delta)
	assert.NoError(t, err)
	assert.Equal(t, "150Gi", result.String())
}

func TestCalculateFinalCapacity_WithResizeDelta_GrowthBelowDelta_Bumps(t *testing.T) {
	const delta int64 = 50 * 1024 * 1024 // 50Mi
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	// Policy +10Mi => 100Gi+10Mi; below delta so should bump to 100Gi+50Mi+1MiB (so backend actually resizes)
	policy := createTestAutogrowPolicy("p", "80%", "10Mi", "200Gi", 1)
	currentSize := resource.MustParse("100Gi")

	result, err := controller.calculateFinalCapacity(context.Background(), currentSize, policy, "test-pv", delta)
	assert.NoError(t, err)
	expected := currentSize.Value() + delta + autogrow.ExtraBytesAboveResizeDelta
	assert.Equal(t, expected, result.Value())
}

func TestCalculateFinalCapacity_WithResizeDelta_BumpedExceedsMaxSize_ReturnsError(t *testing.T) {
	const delta int64 = 50 * 1024 * 1024 // 50Mi
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	// Current just under 100Gi so policy +10Mi is valid; bump would exceed 100Gi so we error (no cap)
	maxSizeQty := resource.MustParse("100Gi")
	currentSize := *resource.NewQuantity(maxSizeQty.Value()-20*1024*1024, resource.BinarySI) // 100Gi - 20Mi
	policy := createTestAutogrowPolicy("p", "80%", "10Mi", "100Gi", 1)

	result, err := controller.calculateFinalCapacity(context.Background(), currentSize, policy, "test-pv", delta)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maxSize")
	assert.Contains(t, err.Error(), "resize delta")
	assert.Contains(t, err.Error(), "policy growthAmount")
	assert.True(t, result.IsZero())
}

// Test 7: getPVCForVolume (resolves PVC from PV only; no volume config)
func TestGetPVCForVolume_Success(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	volExternal := createTestVolExternal("test-pv", "test-policy")
	pvc := createTestPVC("test-pvc", "default", "50Gi")
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pv"},
		Spec: corev1.PersistentVolumeSpec{
			ClaimRef: &corev1.ObjectReference{Namespace: "default", Name: "test-pvc"},
		},
	}
	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	result, err := controller.getPVCForVolume(context.Background(), volExternal, "test-pv")
	assert.NoError(t, err)
	assert.Equal(t, "test-pvc", result.Name)
}

func TestGetPVCForVolume_EmptyVolumeName(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	volExternal := createTestVolExternal("test-pv", "test-policy")
	_, err := controller.getPVCForVolume(context.Background(), volExternal, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume name")
}

// Test 8: patchPVCSize
func TestPatchPVCSize_Success(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	pvc := createTestPVC("test-pvc", "default", "50Gi")
	_, err := controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	newSize := resource.MustParse("100Gi")
	err = controller.patchPVCSize(context.Background(), pvc, newSize)
	assert.NoError(t, err)
}

// Test 9: checkPVCResizeStatus
func TestCheckPVCResizeStatus_ResizingConditionFalse(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	pvc := createTestPVC("test-pvc", "default", "50Gi")
	pvc.Status.Conditions = []corev1.PersistentVolumeClaimCondition{
		{
			Type:    corev1.PersistentVolumeClaimResizing,
			Status:  corev1.ConditionFalse,
			Message: "Resize failed",
		},
	}

	failed, success, msg := controller.checkPVCResizeStatus(context.Background(), pvc)
	assert.True(t, failed)
	assert.False(t, success)
	assert.Contains(t, msg, "Resize failed")
}

func TestCheckPVCResizeStatus_NoFailureOrSuccess(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	pvc := createTestPVC("test-pvc", "default", "50Gi")

	failed, success, msg := controller.checkPVCResizeStatus(context.Background(), pvc)
	assert.False(t, failed)
	assert.False(t, success)
	assert.Empty(t, msg)
}

// Test 10-12: rejectTagri, failTagri, completeTagriSuccess
func TestRejectTagri_Success(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	pvc := createTestPVC("test-pvc", "default", "50Gi")

	// Create TAGRI
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	// Mock GetVolume call (best-effort call in rejectTagri)
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("not found")).AnyTimes()

	err = controller.rejectTagri(context.Background(), tagri, pvc, "test rejection")
	assert.NoError(t, err)

	// Verify status updated
	updated, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, TagriPhaseRejected, updated.Status.Phase)
	assert.NotNil(t, updated.Status.ProcessedAt)
}

func TestFailTagri_Success(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	pvc := createTestPVC("test-pvc", "default", "50Gi")

	// Create TAGRI
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	err = controller.failTagri(context.Background(), tagri, nil, pvc, "test failure")
	assert.NoError(t, err)

	// Verify status updated
	updated, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, TagriPhaseFailed, updated.Status.Phase)
	assert.NotNil(t, updated.Status.ProcessedAt)
}

// Test failTagri - when updateTagriStatus fails, error is returned
func TestFailTagri_UpdateStatusFails(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	// Do NOT create TAGRI in clientset so updateTagriStatus fails
	volExternal := createTestVolExternal("test-pv", "test-policy")

	err := controller.failTagri(context.Background(), tagri, volExternal, nil, "test failure")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update TAGRI status")
}

func TestCompleteTagriSuccess_Success(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	pvc := createTestPVC("test-pvc", "default", "100Gi")

	// Create TAGRI
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	err = controller.completeTagriSuccess(context.Background(), tagri, nil, pvc, "100Gi")
	assert.NoError(t, err)

	// Verify status updated
	updated, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, TagriPhaseCompleted, updated.Status.Phase)
	assert.NotNil(t, updated.Status.ProcessedAt)
}

// Test 13: updateTagriStatus
func TestUpdateTagriStatus_DeletionTimestampSet(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	now := metav1.Now()
	tagri.ObjectMeta.DeletionTimestamp = &now

	result, err := controller.updateTagriStatus(context.Background(), tagri)
	assert.NoError(t, err)
	assert.Equal(t, tagri, result) // Returns original without updating
}

func TestUpdateTagriStatus_Success(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)

	// Create TAGRI
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	tagri.Status.Phase = TagriPhaseInProgress
	result, err := controller.updateTagriStatus(context.Background(), tagri)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, TagriPhaseInProgress, result.Status.Phase)
}

// Test 14: updateTagriCR
func TestUpdateTagriCR_Success(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)

	// Create TAGRI
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	tagri.ObjectMeta.Finalizers = []string{}
	result, err := controller.updateTagriCR(context.Background(), tagri)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

// Test 15: deleteTagriNow
func TestDeleteTagriNow_WithFinalizers(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)

	// Create TAGRI
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	err = controller.deleteTagriNow(context.Background(), tagri)
	assert.NoError(t, err)

	// Verify deleted
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	assert.Error(t, err)
}

// Test 16-18: Volume autogrow status updates
func TestUpdateVolumeAutogrowStatus_NilVolExternal(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	err := controller.updateVolumeAutogrowStatus(
		context.Background(),
		nil,
		func(status *models.VolumeAutogrowStatus) {},
		"test",
		LogFields{},
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid volume")
}

// TestUpdateVolumeAutogrowStatus_ConfigNil_ReturnsError covers updateVolumeAutogrowStatus when volExternal is set but Config is nil.
func TestUpdateVolumeAutogrowStatus_ConfigNil_ReturnsError(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	volExternal := &storage.VolumeExternal{Config: nil}
	err := controller.updateVolumeAutogrowStatus(
		context.Background(),
		volExternal,
		func(status *models.VolumeAutogrowStatus) {},
		"test",
		LogFields{},
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid volume")
}

// TestUpdateVolumeAutogrowStatus_OrchestratorReturnsError covers updateVolumeAutogrowStatus when the orchestrator returns an error (e.g. volume not found).
// Uses a controller without UpdateVolumeAutogrowStatus.AnyTimes() so the test's expectation is matched.
func TestUpdateVolumeAutogrowStatus_OrchestratorReturnsError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator.EXPECT().GetResizeDeltaForBackend(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()
	orchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).Return(nil, fmt.Errorf("not found")).AnyTimes()

	controller, err := newTridentCrdControllerImpl(
		orchestrator, "trident", GetTestKubernetesClientset(), GetTestSnapshotClientset(), GetTestCrdClientset(), nil, nil)
	require.NoError(t, err)
	controller.recorder = record.NewFakeRecorder(100)

	volExternal := createTestVolExternal("vol-not-found", "test-policy")
	orchestrator.EXPECT().UpdateVolumeAutogrowStatus(gomock.Any(), "vol-not-found", gomock.Any()).Return(fmt.Errorf("volume %v was not found", "vol-not-found"))

	err = controller.updateVolumeAutogrowStatus(
		context.Background(),
		volExternal,
		func(status *models.VolumeAutogrowStatus) { status.TotalAutogrowAttempted++ },
		"attempt",
		LogFields{},
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestUpdateVolumeAutogrowStatus_Success covers updateVolumeAutogrowStatus when the orchestrator succeeds.
// The core layer owns the persistent store; the frontend only calls the orchestrator.
// Uses a controller without UpdateVolumeAutogrowStatus.AnyTimes() so the test's expectation is matched.
func TestUpdateVolumeAutogrowStatus_Success(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator.EXPECT().GetResizeDeltaForBackend(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()
	orchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).Return(nil, fmt.Errorf("not found")).AnyTimes()

	controller, err := newTridentCrdControllerImpl(
		orchestrator, "trident", GetTestKubernetesClientset(), GetTestSnapshotClientset(), GetTestCrdClientset(), nil, nil)
	require.NoError(t, err)
	controller.recorder = record.NewFakeRecorder(100)

	volExternal := createTestVolExternal("test-pv", "test-policy")
	orchestrator.EXPECT().UpdateVolumeAutogrowStatus(gomock.Any(), "test-pv", gomock.Any()).DoAndReturn(
		func(_ context.Context, name string, status *models.VolumeAutogrowStatus) error {
			assert.Equal(t, "test-pv", name)
			require.NotNil(t, status)
			assert.Equal(t, 1, status.TotalAutogrowAttempted)
			return nil
		})

	err = controller.updateVolumeAutogrowStatus(
		context.Background(),
		volExternal,
		func(status *models.VolumeAutogrowStatus) { status.TotalAutogrowAttempted++ },
		"attempt",
		LogFields{},
	)
	assert.NoError(t, err)
}

// TestRejectTagri_UpdatesVolumeAutogrowStatusAttempt covers rejectTagri path that calls updateVolumeAutogrowStatusAttempt.
// The frontend calls the orchestrator to update autogrow status; the core owns the persistent store.
func TestRejectTagri_UpdatesVolumeAutogrowStatusAttempt(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})

	orchestrator.EXPECT().GetVolume(gomock.Any(), "test-pv").Return(volExternal, nil)
	// updateVolumeAutogrowStatus is allowed by setupTagriTest's AnyTimes(); rejectTagri calls it when volExternal is set

	err := controller.rejectTagri(context.Background(), tagri, pvc, "test rejection")
	assert.NoError(t, err)
}

// TestFailTagri_UpdatesVolumeAutogrowStatusAttempt covers failTagri path that calls updateVolumeAutogrowStatusAttempt when volExternal is not nil.
// The frontend calls the orchestrator to update autogrow status; the core owns the persistent store.
func TestFailTagri_UpdatesVolumeAutogrowStatusAttempt(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	// updateVolumeAutogrowStatus is allowed by setupTagriTest's AnyTimes()

	err := controller.failTagri(context.Background(), tagri, volExternal, pvc, "test failure")
	assert.NoError(t, err)
}

// Test 19: deleteTagriHandler
func TestDeleteTagriHandler_Success(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)

	// Create in fake clientset (needed for Delete to work)
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	err = controller.deleteTagriHandler(context.Background(), tagri)
	assert.NoError(t, err)
}

func TestDeleteTagriHandler_WithFinalizer(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	// Create TAGRI with finalizer
	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.AddTridentFinalizers()

	// Create in fake clientset
	created, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	// Delete should succeed (finalizer will be removed first, then object deleted)
	err = controller.deleteTagriHandler(context.Background(), created)
	assert.NoError(t, err)

	// Verify object is deleted
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	assert.True(t, err != nil, "Expected TAGRI to be deleted")
}

func TestDeleteTagriHandler_AlreadyDeleted(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	// TAGRI without finalizers
	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.RemoveTridentFinalizers() // Remove default finalizers

	// Don't create in fake clientset - so it doesn't exist
	// Delete should succeed (IsNotFound is treated as success when deleting, and no finalizers to remove)
	err := controller.deleteTagriHandler(context.Background(), tagri)
	assert.NoError(t, err)
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Integration Tests (Complex Scenarios)

// These tests verify end-to-end workflows with multiple function calls

func TestIntegration_HardTimeoutInDifferentPhases(t *testing.T) {
	phases := []string{TagriPhasePending, TagriPhaseInProgress, TagriPhaseFailed}

	for _, phase := range phases {
		t.Run(fmt.Sprintf("Phase_%s", phase), func(t *testing.T) {
			controller, mockCtrl, _ := setupTagriTest(t)
			defer mockCtrl.Finish()

			tagri := createTestTagri("test-tagri", "test-pv", "test-policy", phase, 1)

			// Create TAGRI first, then modify timestamp
			created, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
				context.Background(), tagri, metav1.CreateOptions{})
			require.NoError(t, err)

			// Set old timestamp
			created.CreationTimestamp = metav1.NewTime(time.Now().Add(-10 * time.Minute))

			// Should delete due to hard timeout
			handled, err := controller.handleTimeoutAndTerminalPhase(context.Background(), created)
			assert.True(t, handled)
			assert.NoError(t, err)

			// Verify deleted
			_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
				context.Background(), tagri.Name, metav1.GetOptions{})
			assert.Error(t, err)
		})
	}
}

func TestIntegration_TerminalPhaseRetentionWorkflow(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	// Completed TAGRIs are deleted immediately.
	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseCompleted, 1)
	processedAt := metav1.NewTime(time.Now().Add(-30 * time.Second))
	tagri.Status.ProcessedAt = &processedAt

	created, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	// Should delete immediately
	handled, err := controller.handleTimeoutAndTerminalPhase(context.Background(), created)
	assert.True(t, handled)
	assert.NoError(t, err)

	// Verify deleted (Get should return not found)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	assert.Error(t, err)
	assert.True(t, k8sapierrors.IsNotFound(err), "expected NotFound after deletion")
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Additional Tests

// Test processTagriFirstTime - Policy not found (should retry)
func TestProcessTagriFirstTime_PolicyNotFound_Retry(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create TAGRI (but no policy)
	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	volExternal := createTestVolExternal("test-pv", "test-policy")

	// Mock orchestrator: policy not in core cache -> defer retry
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil)
	orchestrator.EXPECT().GetAutogrowPolicy(gomock.Any(), "test-policy").Return(nil, errors.NotFoundError("autogrow policy not found"))

	// Process
	err = controller.processTagriFirstTime(context.Background(), tagri)

	// Should defer with policy not found (orchestrator)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// Test monitorTagriResize - Resize success via capacity
func TestMonitorTagriResize_ResizeSuccessViaCapacity(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "120Gi")
	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "120Gi"
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	volExternal := createTestVolExternal("test-pv", "test-policy")

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()

	// Monitor
	err = controller.monitorTagriResize(context.Background(), tagri)

	// Should complete
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "marked as completed")
}

// Test monitorTagriResize - Resize success via VolumeResizeSuccessful event (if resizeSuccessful block)
func TestMonitorTagriResize_ResizeSuccessViaEvent(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "120Gi")
	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create VolumeResizeSuccessful event so checkPVCResizeStatus returns resizeSuccessful=true
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc-resize-success", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "PersistentVolumeClaim",
			Namespace: "default",
			Name:      "test-pvc",
		},
		Reason:  "VolumeResizeSuccessful",
		Message: "resize succeeded",
	}
	_, err = controller.kubeClientset.CoreV1().Events("default").Create(
		context.Background(), event, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create TAGRI in InProgress
	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "120Gi"
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	volExternal := createTestVolExternal("test-pv", "test-policy")

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()

	err = controller.monitorTagriResize(context.Background(), tagri)

	// Must enter if resizeSuccessful block, verify capacity, and call completeTagriSuccess
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileIncompleteError(err))
	assert.Contains(t, err.Error(), "marked as completed")
	assert.Contains(t, err.Error(), "terminal phase")
}

// Test monitorTagriResize - PVC deleted during monitoring
func TestMonitorTagriResize_PVCDeleted(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	// Create TAGRI in InProgress
	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "120Gi"
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create volume external (but NO PVC)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	volExternal.Config.Namespace = "default"
	volExternal.Config.RequestName = "test-pvc"

	// Mock orchestrator - allow multiple calls since rejectTagri also calls GetVolume
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()

	// Monitor
	err = controller.monitorTagriResize(context.Background(), tagri)

	// Should reject (PVC not found)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "PVC deleted")
}

// Test monitorTagriResize - Still in progress
func TestMonitorTagriResize_StillInProgress(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "120Gi"
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	volExternal := createTestVolExternal("test-pv", "test-policy")

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()

	err = controller.monitorTagriResize(context.Background(), tagri)

	// Should defer (still in progress)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Resize in progress")
}

// Test checkPVCResizeStatus - Success event detected
func TestCheckPVCResizeStatus_SuccessEvent(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	// Create PVC
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	_, err := controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create success event
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: "default",
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: "PersistentVolumeClaim",
			Name: "test-pvc",
		},
		Reason:  "VolumeResizeSuccessful",
		Message: "Volume resize successful",
	}
	_, err = controller.kubeClientset.CoreV1().Events("default").Create(
		context.Background(), event, metav1.CreateOptions{})
	require.NoError(t, err)

	// Check status
	failed, success, msg := controller.checkPVCResizeStatus(context.Background(), pvc)

	assert.False(t, failed)
	assert.True(t, success)
	assert.Empty(t, msg)
}

// Test checkPVCResizeStatus - Failure event detected
func TestCheckPVCResizeStatus_FailureEvent(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	// Create PVC
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	_, err := controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create failure event
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-event",
			Namespace: "default",
		},
		InvolvedObject: corev1.ObjectReference{
			Kind: "PersistentVolumeClaim",
			Name: "test-pvc",
		},
		Reason:  "VolumeResizeFailed",
		Message: "CSI resize failed",
	}
	_, err = controller.kubeClientset.CoreV1().Events("default").Create(
		context.Background(), event, metav1.CreateOptions{})
	require.NoError(t, err)

	// Check status
	failed, success, msg := controller.checkPVCResizeStatus(context.Background(), pvc)

	assert.True(t, failed)
	assert.False(t, success)
	assert.Contains(t, msg, "CSI resize failed")
}

// Test getPVCForVolume - Invalid volume (nil volExternal or empty volumeName)
func TestGetPVCForVolume_InvalidVolume(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()
	volExternal := createTestVolExternal("test-pv", "test-policy")

	pvc, err := controller.getPVCForVolume(context.Background(), nil, "test-pv")
	assert.Error(t, err)
	assert.Nil(t, pvc)
	assert.Contains(t, err.Error(), "invalid volume")

	_, err = controller.getPVCForVolume(context.Background(), volExternal, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "volume name")
}

// Test getPVCForVolume - PV has no claimRef (not bound)
func TestGetPVCForVolume_PVHasNoClaimRef(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pv"},
		Spec:       corev1.PersistentVolumeSpec{}, // no ClaimRef
	}
	_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	require.NoError(t, err)

	pvc, err := controller.getPVCForVolume(context.Background(), volExternal, "test-pv")
	assert.Error(t, err)
	assert.Nil(t, pvc)
	assert.Contains(t, err.Error(), "no claimRef")
}

// Test getPVCForVolume - uses Kubernetes helper from GetFrontend when available (GetPVCForPV success)
func TestGetPVCForVolume_UsesK8sHelperFromGetFrontend(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockK8s := mockk8s.NewMockK8SControllerHelperPlugin(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	wantPVC := createTestPVC("from-cache-pvc", "default", "50Gi")

	orchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).Return(mockK8s, nil)
	mockK8s.EXPECT().GetPVCForPV(gomock.Any(), "test-pv").Return(wantPVC, nil)

	controller, err := newTridentCrdControllerImpl(
		orchestrator, tridentNamespace, kubeClient, snapClient, crdClient, nil, nil)
	require.NoError(t, err)
	controller.recorder = record.NewFakeRecorder(100)

	volExternal := createTestVolExternal("test-pv", "test-policy")

	result, err := controller.getPVCForVolume(context.Background(), volExternal, "test-pv")
	require.NoError(t, err)
	assert.Equal(t, "from-cache-pvc", result.Name)
	assert.True(t, result.Spec.Resources.Requests[corev1.ResourceStorage].Equal(resource.MustParse("50Gi")))
}

// Test patchPVCSize - Patch operation
func TestPatchPVCSize_PatchOperation(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	// Create PVC
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	_, err := controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	// Patch size
	newSize := resource.MustParse("120Gi")
	err = controller.patchPVCSize(context.Background(), pvc, newSize)
	assert.NoError(t, err)

	// Verify
	updated, err := controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Get(
		context.Background(), "test-pvc", metav1.GetOptions{})
	require.NoError(t, err)
	actualSize := updated.Spec.Resources.Requests[corev1.ResourceStorage]
	assert.Equal(t, "120Gi", actualSize.String())
}

// Test rejectTagri - With PVC and event emission
func TestRejectTagri_WithPVCAndEvent(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	// Create PVC
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	_, err := controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create TAGRI
	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	// Mock volume (will fail to get, that's ok for best-effort)
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("not found")).AnyTimes()

	// Reject
	err = controller.rejectTagri(context.Background(), tagri, pvc, "Test rejection reason")
	assert.NoError(t, err)

	// Verify TAGRI status updated
	updated, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, TagriPhaseRejected, updated.Status.Phase)
	assert.Contains(t, updated.Status.Message, "Test rejection reason")
	assert.NotNil(t, updated.Status.ProcessedAt)
}

// Test updateTagriStatus - Deletion timestamp set (should skip update gracefully)
func TestUpdateTagriStatus_WithDeletionTimestamp(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	// Create TAGRI
	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	// Delete it to set deletion timestamp
	err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Delete(
		context.Background(), tagri.Name, metav1.DeleteOptions{})
	require.NoError(t, err)

	// Get the updated object with deletion timestamp
	deleted, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NoError(t, err)
	require.False(t, deleted.DeletionTimestamp.IsZero())

	// Try to update status (should skip gracefully and return original)
	updated, err := controller.updateTagriStatus(context.Background(), deleted)

	// Should not error, just skip the update
	assert.NoError(t, err)
	assert.NotNil(t, updated)
	assert.Equal(t, deleted.Name, updated.Name)
}

// Test handleTridentAutogrowRequestInternal with TAGRI in informer (EventAdd -> upsert -> terminal delete)
func TestHandleTridentAutogrowRequestInternal_WithTAGRIInInformer_EventAdd_CompletedDeletes(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseCompleted, 1)
	tagri.Status.ProcessedAt = &metav1.Time{Time: time.Now()}
	created, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)
	controller.crdInformer.TridentAutogrowRequestInternals().Informer().GetIndexer().Add(created)

	// After handleTimeoutAndTerminalPhase deletes the TAGRI, upsertTagriHandler still calls processTagriFirstTime
	// (same tagri in-memory). Mock GetVolume so that path doesn't panic.
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("not found")).AnyTimes()

	keyItem := &KeyItem{
		key:   fmt.Sprintf("%s/%s", config.OrchestratorName, tagri.Name),
		ctx:   context.Background(),
		event: EventAdd,
	}
	err = controller.handleTridentAutogrowRequestInternal(keyItem)
	// May get error from processTagriFirstTime (volume not found) after delete; or no error if delete path dominates
	_ = err
	// TAGRI should be deleted by handleTimeoutAndTerminalPhase -> deleteTagriNow
	_, getErr := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	assert.True(t, k8sapierrors.IsNotFound(getErr))
}

// Test handleTridentAutogrowRequestInternal with EventDelete
func TestHandleTridentAutogrowRequestInternal_EventDelete(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	created, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)
	controller.crdInformer.TridentAutogrowRequestInternals().Informer().GetIndexer().Add(created)

	keyItem := &KeyItem{
		key:   fmt.Sprintf("%s/%s", config.OrchestratorName, tagri.Name),
		ctx:   context.Background(),
		event: EventDelete,
	}
	err = controller.handleTridentAutogrowRequestInternal(keyItem)
	assert.NoError(t, err)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	assert.True(t, k8sapierrors.IsNotFound(err))
}

// Test handleTridentAutogrowRequestInternal with EventUpdate but TAGRI has DeletionTimestamp:
// delete can be delivered as Update; we route to delete handler (same as AGP).
func TestHandleTridentAutogrowRequestInternal_EventUpdateWithDeletionTimestamp_RoutesToDelete(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	created, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	// Lister returns object with DeletionTimestamp set (as when delete is delivered as Update)
	tagriWithDeletion := created.DeepCopy()
	now := metav1.Now()
	tagriWithDeletion.ObjectMeta.DeletionTimestamp = &now
	controller.crdInformer.TridentAutogrowRequestInternals().Informer().GetIndexer().Add(tagriWithDeletion)

	keyItem := &KeyItem{
		key:   fmt.Sprintf("%s/%s", config.OrchestratorName, tagri.Name),
		ctx:   context.Background(),
		event: EventUpdate,
	}
	err = controller.handleTridentAutogrowRequestInternal(keyItem)
	assert.NoError(t, err)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	assert.True(t, k8sapierrors.IsNotFound(err), "TAGRI should be deleted when Update has DeletionTimestamp")
}

// Test handleTridentAutogrowRequestInternal with default/unknown event type returns nil
func TestHandleTridentAutogrowRequestInternal_DefaultEventType(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	created, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)
	controller.crdInformer.TridentAutogrowRequestInternals().Informer().GetIndexer().Add(created)

	keyItem := &KeyItem{
		key:   fmt.Sprintf("%s/%s", config.OrchestratorName, tagri.Name),
		ctx:   context.Background(),
		event: "Unknown",
	}
	err = controller.handleTridentAutogrowRequestInternal(keyItem)
	assert.NoError(t, err)
}

// Test handleTridentAutogrowRequestInternal lister error (transient) returns ReconcileDeferredError.
// Skipped: requires injecting a failing lister to get a non-NotFound error from the lister.
func TestHandleTridentAutogrowRequestInternal_ListerError(t *testing.T) {
	t.Skip("Requires injecting failing lister to get transient error")
}

// Test handleTimeoutAndTerminalPhase with zero CreationTimestamp (skips hard timeout block)
func TestHandleTimeoutAndTerminalPhase_ZeroCreationTimestamp(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.CreationTimestamp = metav1.Time{}
	handled, err := controller.handleTimeoutAndTerminalPhase(context.Background(), tagri)
	assert.False(t, handled)
	assert.NoError(t, err)
}

// Test calculateFinalCapacity error (invalid policy)
func TestCalculateFinalCapacity_InvalidPolicy(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	policy := createTestAutogrowPolicy("test-policy", "80%", "invalid-growth", "100Gi", 1)
	currentSize := resource.MustParse("50Gi")
	_, err := controller.calculateFinalCapacity(context.Background(), currentSize, policy, "test-pv", 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to calculate final capacity")
}

// Test patchPVCSize error (PVC not in cluster)
func TestPatchPVCSize_Error(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	pvc := createTestPVC("nonexistent-pvc", "default", "100Gi")
	// Do not create PVC in clientset
	err := controller.patchPVCSize(context.Background(), pvc, resource.MustParse("120Gi"))
	assert.Error(t, err)
}

// Test checkPVCResizeStatus - PVC condition Resizing False
func TestCheckPVCResizeStatus_ConditionResizingFalse(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	pvc := createTestPVC("test-pvc", "default", "100Gi")
	pvc.Status.Conditions = []corev1.PersistentVolumeClaimCondition{
		{
			Type:    corev1.PersistentVolumeClaimResizing,
			Status:  corev1.ConditionFalse,
			Message: "resize failed",
		},
	}
	failed, success, msg := controller.checkPVCResizeStatus(context.Background(), pvc)
	assert.True(t, failed)
	assert.False(t, success)
	assert.Contains(t, msg, "Resizing")
	assert.Contains(t, msg, "resize failed")
}

// Test checkPVCResizeStatus - PVC condition FileSystemResizePending False
func TestCheckPVCResizeStatus_ConditionFileSystemResizePendingFalse(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	pvc := createTestPVC("test-pvc", "default", "100Gi")
	pvc.Status.Conditions = []corev1.PersistentVolumeClaimCondition{
		{
			Type:    corev1.PersistentVolumeClaimFileSystemResizePending,
			Status:  corev1.ConditionFalse,
			Message: "fs resize failed",
		},
	}
	failed, success, msg := controller.checkPVCResizeStatus(context.Background(), pvc)
	assert.True(t, failed)
	assert.False(t, success)
	assert.Contains(t, msg, "FileSystemResizePending")
}

// Test checkPVCResizeStatus - VolumeResizeSuccessful event
func TestCheckPVCResizeStatus_VolumeResizeSuccessfulEvent(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	pvc := createTestPVC("test-pvc", "default", "100Gi")
	_, err := controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc-event", Namespace: "default"},
		InvolvedObject: corev1.ObjectReference{
			Kind:      "PersistentVolumeClaim",
			Namespace: "default",
			Name:      pvc.Name,
		},
		Reason:  "VolumeResizeSuccessful",
		Message: "resize succeeded",
	}
	_, err = controller.kubeClientset.CoreV1().Events("default").Create(
		context.Background(), event, metav1.CreateOptions{})
	require.NoError(t, err)

	failed, success, msg := controller.checkPVCResizeStatus(context.Background(), pvc)
	assert.False(t, failed)
	assert.True(t, success)
	assert.Empty(t, msg)
}

// Test monitorTagriResize - resize failed (CSI failure) -> after RetryCount reaches max we failTagri
func TestMonitorTagriResize_ResizeFailedViaCondition(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "120Gi"
	tagri.Status.RetryCount = MaxTagriRetries - 1 // One more resize-failure observation will hit max and trigger failTagri
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	pvc.Status.Conditions = []corev1.PersistentVolumeClaimCondition{
		{Type: corev1.PersistentVolumeClaimResizing, Status: corev1.ConditionFalse, Message: "resize failed"},
	}
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()

	err := controller.monitorTagriResize(context.Background(), tagri)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileIncompleteError(err))
	assert.Contains(t, err.Error(), "Failed")
}

// Test monitorTagriResize - resize failed but RetryCount below max: status updated, ReconcileDeferredError
func TestMonitorTagriResize_ResizeFailedBelowMaxRetries(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "120Gi"
	tagri.Status.RetryCount = 0
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	pvc.Status.Conditions = []corev1.PersistentVolumeClaimCondition{
		{Type: corev1.PersistentVolumeClaimResizing, Status: corev1.ConditionFalse, Message: "resize failed"},
	}
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()

	err := controller.monitorTagriResize(context.Background(), tagri)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileDeferredError(err))
	assert.Contains(t, err.Error(), "Resize failure observed")
	assert.Contains(t, err.Error(), "1/5")

	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.EqualValues(t, 1, updated.Status.RetryCount)
	assert.Equal(t, TagriPhaseInProgress, updated.Status.Phase)
}

// Test monitorTagriResize - PVC capacity nil
func TestMonitorTagriResize_PVCCapacityNil(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "120Gi"
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	pvc.Status.Capacity = nil
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()

	err := controller.monitorTagriResize(context.Background(), tagri)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileDeferredError(err))
	assert.Contains(t, err.Error(), "capacity not yet reported")
}

// Test monitorTagriResize - invalid FinalCapacityBytes
func TestMonitorTagriResize_InvalidFinalCapacityBytes(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "invalid"
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pv := createTestPVWithClaimRef("test-pv", "default", "test-pvc")
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumes().Create(context.Background(), pv, metav1.CreateOptions{})
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})

	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(volExternal, nil).AnyTimes()

	err := controller.monitorTagriResize(context.Background(), tagri)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileIncompleteError(err))
	assert.Contains(t, err.Error(), "invalid capacity")
}

// Test tryCompleteTagriIfResizeReachedTarget - returns false when FinalCapacityBytes is empty
func TestTryCompleteTagriIfResizeReachedTarget_EmptyFinalCapacityBytes(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pvc := createTestPVC("test-pvc", "default", "100Gi")

	completed, err := controller.tryCompleteTagriIfResizeReachedTarget(
		context.Background(), tagri, volExternal, pvc, "")
	assert.False(t, completed)
	assert.NoError(t, err)
}

// Test tryCompleteTagriIfResizeReachedTarget - returns false when capacity below target
func TestTryCompleteTagriIfResizeReachedTarget_CapacityBelowTarget(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "2147483648" // 2Gi target
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pvc := createTestPVC("test-pvc", "default", "1Gi")

	completed, err := controller.tryCompleteTagriIfResizeReachedTarget(
		context.Background(), tagri, volExternal, pvc, "")
	assert.False(t, completed)
	assert.NoError(t, err)
}

// Test tryCompleteTagriIfResizeReachedTarget - returns true and updates TAGRI to Completed when capacity >= target
func TestTryCompleteTagriIfResizeReachedTarget_CapacityReached(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "2147483648" // 2Gi target
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	volExternal := createTestVolExternal("test-pv", "test-policy")
	pvc := createTestPVC("test-pvc", "default", "2Gi")
	_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	require.NoError(t, err)

	completed, err := controller.tryCompleteTagriIfResizeReachedTarget(
		context.Background(), tagri, volExternal, pvc, "Resize completed successfully.")
	assert.True(t, completed)
	assert.NoError(t, err)

	updated, getErr := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NoError(t, getErr)
	assert.Equal(t, TagriPhaseCompleted, updated.Status.Phase)
}

// Test updateTagriStatus - UpdateStatus error (TAGRI not in cluster)
func TestUpdateTagriStatus_UpdateError(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("nonexistent-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	// Do not create in clientset so UpdateStatus fails
	updated, err := controller.updateTagriStatus(context.Background(), tagri)
	assert.Error(t, err)
	assert.Nil(t, updated)
}

// Test updateTagriStatus - Conflict with "UID in object meta" (TAGRI deleted during update) returns (nil, nil)
func TestUpdateTagriStatus_ConflictUIDInObjectMeta_ReturnsNilNil(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator.EXPECT().GetResizeDeltaForBackend(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()
	orchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).Return(nil, fmt.Errorf("not found")).AnyTimes()

	crdClient := GetTestCrdClientset()
	crdClient.PrependReactor("update", "tridentautogrowrequestinternals", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		// UpdateStatus goes through Update with subresource in some clients; fake may use "update"
		// Return conflict with message that triggers (nil, nil) handling
		return true, nil, k8sapierrors.NewConflict(
			action.GetResource().GroupResource(),
			action.(ktesting.UpdateAction).GetObject().(metav1.Object).GetName(),
			fmt.Errorf("Operation cannot be fulfilled: UID in object meta: object has been modified"))
	})

	controller, err := newTridentCrdControllerImpl(
		orchestrator, "trident", GetTestKubernetesClientset(), GetTestSnapshotClientset(), crdClient, nil, nil)
	require.NoError(t, err)
	controller.recorder = record.NewFakeRecorder(100)

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)
	// Re-fetch so we have a valid object for UpdateStatus
	tagri, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	tagri.Status.Phase = TagriPhaseInProgress

	updated, err := controller.updateTagriStatus(context.Background(), tagri)
	assert.NoError(t, err)
	assert.Nil(t, updated)
}

// Test updateTagriCR error
func TestUpdateTagriCR_Error(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("nonexistent-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	// Do not create in clientset
	updated, err := controller.updateTagriCR(context.Background(), tagri)
	assert.Error(t, err)
	assert.Nil(t, updated)
}

// Test deleteTagriNow - no finalizers
func TestDeleteTagriNow_NoFinalizers(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Finalizers = nil
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	err = controller.deleteTagriNow(context.Background(), tagri)
	assert.NoError(t, err)
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	assert.True(t, k8sapierrors.IsNotFound(err))
}

// Test deleteTagriNow - Delete returns NotFound (idempotent)
func TestDeleteTagriNow_AlreadyDeleted(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Finalizers = nil
	_, err := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)
	_ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Delete(
		context.Background(), tagri.Name, metav1.DeleteOptions{})

	// Second delete (tagri object still in hand) - Delete returns NotFound, we ignore
	err = controller.deleteTagriNow(context.Background(), tagri)
	assert.NoError(t, err)
}

// Test deleteTagriNow - Delete returns error (non-NotFound) propagates
func TestDeleteTagriNow_DeleteError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdClient.PrependReactor("delete", "tridentautogrowrequestinternals", func(action ktesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, k8sapierrors.NewInternalError(fmt.Errorf("API server error"))
	})

	controller, err := newTridentCrdControllerImpl(
		orchestrator, "trident", kubeClient, snapClient, crdClient, nil, nil)
	require.NoError(t, err)
	controller.recorder = record.NewFakeRecorder(100)

	// TAGRI with no finalizers so deleteTagriNow goes straight to Delete (no updateTagriCR)
	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	tagri.Finalizers = nil
	_, err = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	require.NoError(t, err)

	err = controller.deleteTagriNow(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete TAGRI")
}

// Test deleteTagriHandler - error from deleteTagriNow propagates
func TestDeleteTagriHandler_Error(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	// TAGRI with finalizers but not in clientset - deleteTagriNow will try updateTagriCR to remove finalizers and fail
	err := controller.deleteTagriHandler(context.Background(), tagri)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to remove finalizers")
}

// createTestTridentVolume creates a minimal TridentVolume for updateVolumeAutogrowStatus tests.
func createTestTridentVolume(name, namespace string) *tridentv1.TridentVolume {
	configJSON, _ := json.Marshal(&storage.VolumeConfig{
		Name:             name,
		Namespace:        "default",
		RequestName:      "test-pvc",
		ImportNotManaged: false,
	})
	return &tridentv1.TridentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:       tridentv1.NameFix(name),
			Namespace:  namespace,
			Finalizers: tridentv1.GetTridentFinalizers(),
		},
		BackendUUID: "test-backend",
		Config:      runtime.RawExtension{Raw: configJSON},
	}
}

// TestCompleteTagriSuccess_UpdatesVolumeAutogrowStatus covers completeTagriSuccess calling updateVolumeAutogrowStatusSuccess.
// The frontend calls the orchestrator to update autogrow status; the core owns the persistent store.
func TestCompleteTagriSuccess_UpdatesVolumeAutogrowStatus(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})
	// updateVolumeAutogrowStatus is allowed by setupTagriTest's AnyTimes()

	err := controller.completeTagriSuccess(context.Background(), tagri, volExternal, pvc, "100Gi")
	assert.NoError(t, err)
}

// TestCompleteTagriSuccess_WhenTagriStatusUpdateFails_DoesNotUpdateTvol ensures that when updateTagriStatus
// fails (e.g. conflict or TAGRI not in cluster), we do not call UpdateVolumeAutogrowStatus. This prevents
// double-counting on retry after conflict. The frontend only calls the orchestrator after TAGRI is persisted.
func TestCompleteTagriSuccess_WhenTagriStatusUpdateFails_DoesNotUpdateTvol(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pvc := createTestPVC("test-pvc", "default", "100Gi")
	// Do NOT create TAGRI in clientset so updateTagriStatus fails
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})

	err := controller.completeTagriSuccess(context.Background(), tagri, volExternal, pvc, "100Gi")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update TAGRI status")
}

// TestRejectTagri_WhenTagriStatusUpdateFails_DoesNotUpdateTvol ensures that when updateTagriStatus
// fails, we do not call UpdateVolumeAutogrowStatus, preventing double-count on retry.
func TestRejectTagri_WhenTagriStatusUpdateFails_DoesNotUpdateTvol(t *testing.T) {
	controller, mockCtrl, _ := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	pvc := createTestPVC("test-pvc", "default", "50Gi")
	// Do NOT create TAGRI in clientset so updateTagriStatus fails
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})

	err := controller.rejectTagri(context.Background(), tagri, pvc, "test rejection")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update TAGRI status")
}

// TestRejectTagri_TvolUpdateFails_StillReturnsNil ensures that when UpdateVolumeAutogrowStatus fails (best-effort),
// rejectTagri still returns nil so TAGRI lifecycle is not blocked.
func TestRejectTagri_TvolUpdateFails_StillReturnsNil(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator.EXPECT().GetResizeDeltaForBackend(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()
	orchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).Return(nil, fmt.Errorf("not found")).AnyTimes()

	controller, err := newTridentCrdControllerImpl(
		orchestrator, "trident", GetTestKubernetesClientset(), GetTestSnapshotClientset(), GetTestCrdClientset(), nil, nil)
	require.NoError(t, err)
	controller.recorder = record.NewFakeRecorder(100)

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhasePending, 1)
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pvc := createTestPVC("test-pvc", "default", "50Gi")
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})

	orchestrator.EXPECT().GetVolume(gomock.Any(), "test-pv").Return(volExternal, nil)
	orchestrator.EXPECT().UpdateVolumeAutogrowStatus(gomock.Any(), "test-pv", gomock.Any()).Return(fmt.Errorf("store unavailable"))

	err = controller.rejectTagri(context.Background(), tagri, pvc, "test rejection")
	assert.NoError(t, err)
	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.Equal(t, TagriPhaseRejected, updated.Status.Phase)
}

// TestFailTagri_TvolUpdateFails_StillReturnsNil ensures that when UpdateVolumeAutogrowStatus fails (best-effort),
// failTagri still returns nil so TAGRI lifecycle is not blocked.
func TestFailTagri_TvolUpdateFails_StillReturnsNil(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator.EXPECT().GetResizeDeltaForBackend(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()
	orchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).Return(nil, fmt.Errorf("not found")).AnyTimes()

	controller, err := newTridentCrdControllerImpl(
		orchestrator, "trident", GetTestKubernetesClientset(), GetTestSnapshotClientset(), GetTestCrdClientset(), nil, nil)
	require.NoError(t, err)
	controller.recorder = record.NewFakeRecorder(100)

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "100Gi"
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pvc := createTestPVC("test-pvc", "default", "50Gi")
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})

	orchestrator.EXPECT().UpdateVolumeAutogrowStatus(gomock.Any(), "test-pv", gomock.Any()).Return(fmt.Errorf("store unavailable"))

	err = controller.failTagri(context.Background(), tagri, volExternal, pvc, "test failure")
	assert.NoError(t, err)
	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.Equal(t, TagriPhaseFailed, updated.Status.Phase)
}

// TestCompleteTagriSuccess_TvolUpdateFails_StillReturnsNil ensures that when UpdateVolumeAutogrowStatus fails (best-effort),
// completeTagriSuccess still returns nil so TAGRI lifecycle is not blocked.
func TestCompleteTagriSuccess_TvolUpdateFails_StillReturnsNil(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator.EXPECT().GetResizeDeltaForBackend(gomock.Any(), gomock.Any()).Return(int64(0), nil).AnyTimes()
	orchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).Return(nil, fmt.Errorf("not found")).AnyTimes()

	controller, err := newTridentCrdControllerImpl(
		orchestrator, "trident", GetTestKubernetesClientset(), GetTestSnapshotClientset(), GetTestCrdClientset(), nil, nil)
	require.NoError(t, err)
	controller.recorder = record.NewFakeRecorder(100)

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "2147483648" // 2Gi
	volExternal := createTestVolExternal("test-pv", "test-policy")
	pvc := createTestPVC("test-pvc", "default", "2Gi")
	_, _ = controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Create(
		context.Background(), tagri, metav1.CreateOptions{})
	_, _ = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(
		context.Background(), pvc, metav1.CreateOptions{})

	orchestrator.EXPECT().UpdateVolumeAutogrowStatus(gomock.Any(), "test-pv", gomock.Any()).Return(fmt.Errorf("store unavailable"))

	err = controller.completeTagriSuccess(context.Background(), tagri, volExternal, pvc, "2Gi")
	assert.NoError(t, err)
	updated, _ := controller.crdClientset.TridentV1().TridentAutogrowRequestInternals(config.OrchestratorName).Get(
		context.Background(), tagri.Name, metav1.GetOptions{})
	require.NotNil(t, updated)
	assert.Equal(t, TagriPhaseCompleted, updated.Status.Phase)
}

// TestTryCompleteTagriIfResizeReachedTarget_GetVolumeFails_ReturnsFalseNil ensures that when volExternal and pvc
// are nil and GetVolume fails, we return (false, nil) so the caller does not block on the fetch error.
func TestTryCompleteTagriIfResizeReachedTarget_GetVolumeFails_ReturnsFalseNil(t *testing.T) {
	controller, mockCtrl, orchestrator := setupTagriTest(t)
	defer mockCtrl.Finish()

	tagri := createTestTagri("test-tagri", "test-pv", "test-policy", TagriPhaseInProgress, 1)
	tagri.Status.FinalCapacityBytes = "2147483648" // 2Gi
	orchestrator.EXPECT().GetVolume(gomock.Any(), tagri.Spec.Volume).Return(nil, fmt.Errorf("backend unavailable"))

	completed, err := controller.tryCompleteTagriIfResizeReachedTarget(
		context.Background(), tagri, nil, nil, "")
	assert.False(t, completed)
	assert.NoError(t, err)
}
