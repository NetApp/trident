// Copyright 2025 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	mockcore "github.com/netapp/trident/mocks/mock_core"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	crdclientfake "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

// TestValidateAutogrowPolicy_ValidFormats tests validation with valid formats
func TestValidateAutogrowPolicy_ValidFormats(t *testing.T) {
	controller := &TridentCrdController{}

	tests := []struct {
		name          string
		usedThreshold string
		growthAmount  string
		maxSize       string
	}{
		{"Percent threshold", "80%", "10%", "100Gi"},
		{"Percent threshold with bytes growth", "80%", "5Gi", "100Gi"},
		{"Valid max size", "80%", "", "1Ti"},
		{"Min valid threshold 1%", "1%", "", "100Gi"},
		{"Max valid threshold 99%", "99%", "", "100Gi"},
		{"Threshold with spaces", " 80% ", "", "100Gi"},
		{"MaxSize with spaces", "80%", "", " 100Gi "},
		{"GrowthAmount with spaces", "80%", " 10% ", "100Gi"},
		{"Valid decimal threshold", "50.5%", "", "100Gi"},
		{"Empty maxSize (optional)", "80%", "10%", ""},
		{"MaxSize zero treated as no limit", "80%", "10%", "0Gi"},
		{"Empty growthAmount (optional)", "80%", "", "100Gi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &tridentv1.TridentAutogrowPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
				Spec: tridentv1.TridentAutogrowPolicySpec{
					UsedThreshold: tt.usedThreshold,
					GrowthAmount:  tt.growthAmount,
					MaxSize:       tt.maxSize,
				},
			}
			err := controller.validateAutogrowPolicy(context.Background(), policy)
			assert.NoError(t, err)
		})
	}
}

// TestValidateAutogrowPolicy_InvalidFormats tests validation with invalid formats
func TestValidateAutogrowPolicy_InvalidFormats(t *testing.T) {
	controller := &TridentCrdController{}

	tests := []struct {
		name          string
		usedThreshold string
		growthAmount  string
		maxSize       string
	}{
		{"Invalid threshold", "invalid", "", "100Gi"},
		{"Invalid max size", "80%", "", "invalid"},
		{"Threshold over 99%", "100%", "", "100Gi"},
		{"Threshold equals 0%", "0%", "", "100Gi"},
		{"Empty threshold", "", "", "100Gi"},
		{"Growth amount 0%", "80%", "0%", "100Gi"},
		{"Invalid growthAmount", "80%", "invalid", "100Gi"},
		{"GrowthAmount zero", "80%", "0Gi", "100Gi"},
		{"MaxSize negative", "80%", "", "-100Gi"},
		{"UsedThreshold as absolute value", "50Gi", "", "100Gi"},
		{"UsedThreshold as absolute decimal", "50.5Gi", "", "100Gi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &tridentv1.TridentAutogrowPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
				Spec: tridentv1.TridentAutogrowPolicySpec{
					UsedThreshold: tt.usedThreshold,
					GrowthAmount:  tt.growthAmount,
					MaxSize:       tt.maxSize,
				},
			}
			err := controller.validateAutogrowPolicy(context.Background(), policy)
			assert.Error(t, err)
		})
	}
}

// TestUpdateAutogrowPolicyCR tests successful CR update (spec/metadata)
func TestUpdateAutogrowPolicyCR(t *testing.T) {
	crdClientset := crdclientfake.NewSimpleClientset()
	controller := &TridentCrdController{crdClientset: crdClientset}

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
		Spec:       tridentv1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
	}

	createdPolicy, err := crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	createdPolicy.Spec.UsedThreshold = "90%"
	updatedPolicy, err := controller.updateAutogrowPolicyCR(context.Background(), createdPolicy)
	assert.NoError(t, err)
	assert.Equal(t, "90%", updatedPolicy.Spec.UsedThreshold)
}

// TestUpdateAutogrowPolicyCRStatus tests successful status update
func TestUpdateAutogrowPolicyCRStatus(t *testing.T) {
	crdClientset := crdclientfake.NewSimpleClientset()
	controller := &TridentCrdController{crdClientset: crdClientset}

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-policy-status"},
		Spec:       tridentv1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
		Status: tridentv1.TridentAutogrowPolicyStatus{
			State:   "",
			Message: "",
		},
	}

	createdPolicy, err := crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Update status fields
	createdPolicy.Status.State = string(tridentv1.TridentAutogrowPolicyStateSuccess)
	createdPolicy.Status.Message = "Policy is ready"
	updatedPolicy, err := controller.updateAutogrowPolicyCRStatus(context.Background(), createdPolicy)
	assert.NoError(t, err)
	assert.Equal(t, string(tridentv1.TridentAutogrowPolicyStateSuccess), updatedPolicy.Status.State)
	assert.Equal(t, "Policy is ready", updatedPolicy.Status.Message)
}

// TestUpdateAutogrowPolicyCRStatus_FailedState tests status update with Failed state
func TestUpdateAutogrowPolicyCRStatus_FailedState(t *testing.T) {
	crdClientset := crdclientfake.NewSimpleClientset()
	controller := &TridentCrdController{crdClientset: crdClientset}

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-policy-failed"},
		Spec:       tridentv1.TridentAutogrowPolicySpec{UsedThreshold: "invalid"},
	}

	createdPolicy, err := crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Update status to Failed
	createdPolicy.Status.State = string(tridentv1.TridentAutogrowPolicyStateFailed)
	createdPolicy.Status.Message = "Validation failed: invalid threshold"
	updatedPolicy, err := controller.updateAutogrowPolicyCRStatus(context.Background(), createdPolicy)
	assert.NoError(t, err)
	assert.Equal(t, string(tridentv1.TridentAutogrowPolicyStateFailed), updatedPolicy.Status.State)
	assert.Contains(t, updatedPolicy.Status.Message, "Validation failed")
}

// TestUpdateAutogrowPolicyCRStatus_DeletingState tests status update with Deleting state
func TestUpdateAutogrowPolicyCRStatus_DeletingState(t *testing.T) {
	crdClientset := crdclientfake.NewSimpleClientset()
	controller := &TridentCrdController{crdClientset: crdClientset}

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy-deleting",
			Finalizers: []string{"trident.netapp.io"},
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
	}

	createdPolicy, err := crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Update status to Deleting
	createdPolicy.Status.State = string(tridentv1.TridentAutogrowPolicyStateDeleting)
	createdPolicy.Status.Message = "Policy is in use by 3 volume(s), waiting for volumes to be disassociated."
	updatedPolicy, err := controller.updateAutogrowPolicyCRStatus(context.Background(), createdPolicy)
	assert.NoError(t, err)
	assert.Equal(t, string(tridentv1.TridentAutogrowPolicyStateDeleting), updatedPolicy.Status.State)
	assert.Contains(t, updatedPolicy.Status.Message, "in use by 3 volume(s)")
}

// TestRemoveAutogrowPolicyFinalizers tests finalizer removal
func TestRemoveAutogrowPolicyFinalizers(t *testing.T) {
	crdClientset := crdclientfake.NewSimpleClientset()
	controller := &TridentCrdController{crdClientset: crdClientset}

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-policy", Finalizers: []string{"trident.netapp.io"}},
		Spec:       tridentv1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
	}

	_, err := crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = controller.removeAutogrowPolicyFinalizers(context.Background(), policy)
	assert.NoError(t, err)

	updatedPolicy, err := crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		context.Background(), policy.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Empty(t, updatedPolicy.ObjectMeta.Finalizers)
}

// TestRemoveAutogrowPolicyFinalizers_NoFinalizers tests when policy has no finalizers
func TestRemoveAutogrowPolicyFinalizers_NoFinalizers(t *testing.T) {
	crdClientset := crdclientfake.NewSimpleClientset()
	controller := &TridentCrdController{crdClientset: crdClientset}

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
		Spec:       tridentv1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
	}

	_, err := crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		context.Background(), policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = controller.removeAutogrowPolicyFinalizers(context.Background(), policy)
	assert.NoError(t, err)
}

// TestUpdateAutogrowPolicyCR_Error tests CR update error
func TestUpdateAutogrowPolicyCR_Error(t *testing.T) {
	crdClientset := crdclientfake.NewSimpleClientset()
	controller := &TridentCrdController{crdClientset: crdClientset}

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "non-existent-policy"},
		Spec:       tridentv1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
	}

	_, err := controller.updateAutogrowPolicyCR(context.Background(), policy)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestUpdateAutogrowPolicyCRStatus_Error tests status update error
func TestUpdateAutogrowPolicyCRStatus_Error(t *testing.T) {
	crdClientset := crdclientfake.NewSimpleClientset()
	controller := &TridentCrdController{crdClientset: crdClientset}

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "non-existent-policy-status"},
		Spec:       tridentv1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
		Status: tridentv1.TridentAutogrowPolicyStatus{
			State:   string(tridentv1.TridentAutogrowPolicyStateSuccess),
			Message: "Should fail",
		},
	}

	_, err := controller.updateAutogrowPolicyCRStatus(context.Background(), policy)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestRemoveAutogrowPolicyFinalizers_Error tests finalizer removal error
func TestRemoveAutogrowPolicyFinalizers_Error(t *testing.T) {
	crdClientset := crdclientfake.NewSimpleClientset()
	controller := &TridentCrdController{crdClientset: crdClientset}

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{Name: "non-existent-policy", Finalizers: []string{"trident.netapp.io"}},
		Spec:       tridentv1.TridentAutogrowPolicySpec{UsedThreshold: "80%"},
	}

	err := controller.removeAutogrowPolicyFinalizers(context.Background(), policy)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestValidateAutogrowPolicy_AdditionalValidFormats tests more valid format combinations
func TestValidateAutogrowPolicy_AdditionalValidFormats(t *testing.T) {
	controller := &TridentCrdController{}

	tests := []struct {
		name          string
		usedThreshold string
		growthAmount  string
		maxSize       string
	}{
		{"Threshold with leading space", " 80%", "", "100Gi"},
		{"Threshold with trailing space", "80% ", "", "100Gi"},
		{"Threshold with both spaces", " 80% ", "", "100Gi"},
		{"Valid decimal max size", "80%", "", "100.5Gi"},
		{"Valid decimal growth amount", "80%", "5.5Gi", "100Gi"},
		{"Valid decimal growth amount percent", "80%", "10.5%", "100Gi"},
		{"Valid threshold with space before % sign", "80 %", "", "100Gi"},
		{"Valid growthAmount with space before % sign", "80%", "10 %", "100Gi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &tridentv1.TridentAutogrowPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
				Spec: tridentv1.TridentAutogrowPolicySpec{
					UsedThreshold: tt.usedThreshold,
					GrowthAmount:  tt.growthAmount,
					MaxSize:       tt.maxSize,
				},
			}
			err := controller.validateAutogrowPolicy(context.Background(), policy)
			assert.NoError(t, err)
		})
	}
}

// TestValidateAutogrowPolicy_AdditionalInvalidFormats tests more invalid scenarios
func TestValidateAutogrowPolicy_AdditionalInvalidFormats(t *testing.T) {
	controller := &TridentCrdController{}

	tests := []struct {
		name          string
		usedThreshold string
		growthAmount  string
		maxSize       string
	}{
		{"Invalid percentage threshold - not a number", "abc%", "", "100Gi"},
		{"Invalid growthAmount percentage - not a number", "80%", "xyz%", "100Gi"},
		{"GrowthAmount absolute with negative", "80%", "-5Gi", "100Gi"},
		{"Threshold with space in middle of number", "8 0%", "", "100Gi"},
		{"GrowthAmount with space in middle", "80%", "1 0%", "100Gi"},
		{"MaxSize with space in middle", "80%", "", "10 0Gi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := &tridentv1.TridentAutogrowPolicy{
				ObjectMeta: metav1.ObjectMeta{Name: "test-policy"},
				Spec: tridentv1.TridentAutogrowPolicySpec{
					UsedThreshold: tt.usedThreshold,
					GrowthAmount:  tt.growthAmount,
					MaxSize:       tt.maxSize,
				},
			}
			err := controller.validateAutogrowPolicy(context.Background(), policy)
			assert.Error(t, err)
		})
	}
}

// TestDeleteAutogrowPolicy tests policy deletion (removed - see TestDeleteAutogrowPolicy_* with orchestrator mocks)

// Helper function to setup Autogrow policy test environment
func setupAutogrowPolicyTest(t *testing.T) (*TridentCrdController, error) {
	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	crdController, err := newTridentCrdControllerImpl(nil, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	if err != nil {
		return nil, err
	}

	return crdController, nil
}

// TestHandleAutogrowPolicy_NilKeyItem tests nil keyItem handling
func TestHandleAutogrowPolicy_NilKeyItem(t *testing.T) {
	controller, err := setupAutogrowPolicyTest(t)
	assert.NoError(t, err)

	err = controller.handleAutogrowPolicy(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "keyItem is nil")
}

// TestHandleAutogrowPolicy_InvalidKey tests invalid key format
func TestHandleAutogrowPolicy_InvalidKey(t *testing.T) {
	controller, err := setupAutogrowPolicyTest(t)
	assert.NoError(t, err)

	ctx := context.Background()
	keyItem := &KeyItem{
		key:        "invalid/key/format/too/many/parts",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentAutogrowPolicy,
	}

	err = controller.handleAutogrowPolicy(keyItem)
	assert.NoError(t, err) // Invalid keys return nil per design
}

// TestHandleAutogrowPolicy_PolicyNotFound tests policy not found case
func TestHandleAutogrowPolicy_PolicyNotFound(t *testing.T) {
	controller, err := setupAutogrowPolicyTest(t)
	assert.NoError(t, err)

	ctx := context.Background()
	keyItem := &KeyItem{
		key:        "non-existent-policy",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentAutogrowPolicy,
	}

	err = controller.handleAutogrowPolicy(keyItem)
	assert.NoError(t, err) // Not found returns nil per design
}

// TestHandleAutogrowPolicy_AddEventWithFinalizers tests adding a policy (removed - see TestHandleAutogrowPolicy_WithOrchestrator_AddSuccess)

// TestHandleAutogrowPolicy_DeleteEvent tests deletion handling (removed - see TestHandleAutogrowPolicy_WithOrchestrator_DeleteWithDeletionTimestamp)

// TestHandleAutogrowPolicy_AddEventWithDeletionTimestamp tests edge case (removed - similar coverage in orchestrator tests)

// TestHandleAutogrowPolicy_ValidationError tests validation failure (removed - see TestSyncAutogrowPolicyToOrchestrator_ValidationErrorStillSyncs)

// TestHandleAutogrowPolicy_UpdateEvent tests update event handling (removed - see TestSyncAutogrowPolicyToOrchestrator_UpdateSuccess)

// TestHandleAutogrowPolicy_UnknownEventType tests unknown event type handling
func TestHandleAutogrowPolicy_UnknownEventType(t *testing.T) {
	controller, mockCtrl, _ := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	// Create a valid policy
	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy-unknown-event",
			Finalizers: []string{"trident.netapp.io"},
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			MaxSize:       "100Gi",
		},
	}

	// Create the policy
	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Add to informer cache
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(createdPolicy)

	// Use an unknown event type (not Add, Update, or Delete)
	keyItem := &KeyItem{
		key:        "test-policy-unknown-event",
		event:      "Unknown", // Unknown event type
		ctx:        ctx,
		objectType: ObjectTypeTridentAutogrowPolicy,
	}

	// Should return nil for unknown event types
	err = controller.handleAutogrowPolicy(keyItem)
	assert.NoError(t, err)
}

// TestHandleAutogrowPolicy_FinalizerUpdateError tests error when finalizer update fails (removed - covered by orchestrator tests)

// TestHandleAutogrowPolicy_ValidationErrorWithUpdateFailure tests both validation and update failing (removed - covered by orchestrator tests)

// TestHandleAutogrowPolicy_NoFinalizersNeeded tests when finalizers already exist (removed - see TestSyncAutogrowPolicyToOrchestrator_UpdateSuccess)

// Tests with Orchestrator Mocks (Complete Integration Tests)

func setupAutogrowPolicyTestWithMock(t *testing.T) (*TridentCrdController, *gomock.Controller, *mockcore.MockOrchestrator) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, nil, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	crdController.recorder = record.NewFakeRecorder(100)

	return crdController, mockCtrl, orchestrator
}

// TestSyncAutogrowPolicyToOrchestrator_AddSuccess tests successful policy addition
func TestSyncAutogrowPolicyToOrchestrator_AddSuccess(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy-add-success",
			Finalizers: []string{"trident.netapp.io"},
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			GrowthAmount:  "20%",
			MaxSize:       "1000Gi",
		},
	}

	// Create policy in fake client
	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Mock orchestrator AddAutogrowPolicy success
	policyExternal := &storage.AutogrowPolicyExternal{
		Name:          "test-policy-add-success",
		UsedThreshold: "80%",
		GrowthAmount:  "20%",
		MaxSize:       "1000Gi",
		State:         storage.AutogrowPolicyStateSuccess,
		Volumes:       []string{},
		VolumeCount:   0,
	}

	orchestrator.EXPECT().
		AddAutogrowPolicy(gomock.Any(), gomock.Any()).
		Return(policyExternal, nil).
		Times(1)

	err = controller.syncAutogrowPolicyToOrchestrator(ctx, createdPolicy, true)
	assert.NoError(t, err)

	// Verify status was updated
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, string(tridentv1.TridentAutogrowPolicyStateSuccess), updatedPolicy.Status.State)
	assert.Contains(t, updatedPolicy.Status.Message, "Autogrow policy validated and ready to use")
}

// TestSyncAutogrowPolicyToOrchestrator_UpdateSuccess tests successful policy update
func TestSyncAutogrowPolicyToOrchestrator_UpdateSuccess(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy-update-success",
			Finalizers: []string{"trident.netapp.io"},
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "85%",
			GrowthAmount:  "15%",
			MaxSize:       "2000Gi",
		},
	}

	// Create policy in fake client
	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Mock orchestrator UpdateAutogrowPolicy success
	policyExternal := &storage.AutogrowPolicyExternal{
		Name:          "test-policy-update-success",
		UsedThreshold: "85%",
		GrowthAmount:  "15%",
		MaxSize:       "2000Gi",
		State:         storage.AutogrowPolicyStateSuccess,
	}

	orchestrator.EXPECT().
		UpdateAutogrowPolicy(gomock.Any(), gomock.Any()).
		Return(policyExternal, nil).
		Times(1)

	err = controller.syncAutogrowPolicyToOrchestrator(ctx, createdPolicy, false)
	assert.NoError(t, err)

	// Verify status
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, string(tridentv1.TridentAutogrowPolicyStateSuccess), updatedPolicy.Status.State)
}

// TestSyncAutogrowPolicyToOrchestrator_AddWithoutFinalizers tests adding finalizers
func TestSyncAutogrowPolicyToOrchestrator_AddWithoutFinalizers(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-policy-no-finalizers",
			// No finalizers initially
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			MaxSize:       "1000Gi",
		},
	}

	// Create policy in fake client
	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Mock orchestrator AddAutogrowPolicy success
	policyExternal := &storage.AutogrowPolicyExternal{
		Name:          "test-policy-no-finalizers",
		UsedThreshold: "80%",
		GrowthAmount:  "",
		MaxSize:       "1000Gi",
		State:         storage.AutogrowPolicyStateSuccess,
	}

	orchestrator.EXPECT().
		AddAutogrowPolicy(gomock.Any(), gomock.Any()).
		Return(policyExternal, nil).
		Times(1)

	err = controller.syncAutogrowPolicyToOrchestrator(ctx, createdPolicy, true)
	assert.NoError(t, err)

	// Verify finalizers were added
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Contains(t, updatedPolicy.Finalizers, "trident.netapp.io")
}

// TestSyncAutogrowPolicyToOrchestrator_OrchestratorAddError tests orchestrator add failure
func TestSyncAutogrowPolicyToOrchestrator_OrchestratorAddError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy-add-error",
			Finalizers: []string{"trident.netapp.io"},
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			MaxSize:       "1000Gi",
		},
	}

	// Create policy in fake client
	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Mock orchestrator AddAutogrowPolicy failure
	orchestratorErr := fmt.Errorf("orchestrator database error")
	orchestrator.EXPECT().
		AddAutogrowPolicy(gomock.Any(), gomock.Any()).
		Return(nil, orchestratorErr).
		Times(1)

	err = controller.syncAutogrowPolicyToOrchestrator(ctx, createdPolicy, true)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileDeferredError(err))

	// Verify status reflects failure
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, string(tridentv1.TridentAutogrowPolicyStateFailed), updatedPolicy.Status.State)
	assert.Contains(t, updatedPolicy.Status.Message, "Failed to sync autogrow policy to orchestrator")
}

// TestSyncAutogrowPolicyToOrchestrator_AlreadyExistsError tests that AlreadyExistsError is handled gracefully
func TestSyncAutogrowPolicyToOrchestrator_AlreadyExistsError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy-already-exists",
			Finalizers: []string{"trident.netapp.io"},
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			MaxSize:       "1000Gi",
		},
	}

	// Create policy in fake client
	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Mock orchestrator AddAutogrowPolicy returning AlreadyExistsError
	// This can happen during reconciliation when the policy already exists in orchestrator
	alreadyExistsErr := errors.AlreadyExistsError("autogrow policy %s already exists", policy.Name)
	orchestrator.EXPECT().
		AddAutogrowPolicy(gomock.Any(), gomock.Any()).
		Return(nil, alreadyExistsErr).
		Times(1)

	err = controller.syncAutogrowPolicyToOrchestrator(ctx, createdPolicy, true)
	// Should NOT return error for AlreadyExistsError - it's handled gracefully
	assert.NoError(t, err)

	// Verify status was NOT set to Failed (should remain unchanged or Success)
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	// Status should NOT be Failed for AlreadyExistsError
	assert.NotEqual(t, string(tridentv1.TridentAutogrowPolicyStateFailed), updatedPolicy.Status.State)
}

// TestSyncAutogrowPolicyToOrchestrator_ValidationErrorStillSyncs tests validation failure
func TestSyncAutogrowPolicyToOrchestrator_ValidationErrorStillSyncs(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy-validation-error",
			Finalizers: []string{"trident.netapp.io"},
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "invalid", // Invalid
			MaxSize:       "1000Gi",
		},
	}

	// Create policy in fake client
	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Even with validation error, still syncs to orchestrator with Failed state
	policyExternal := &storage.AutogrowPolicyExternal{
		Name:  "test-policy-validation-error",
		State: storage.AutogrowPolicyStateFailed,
	}

	orchestrator.EXPECT().
		AddAutogrowPolicy(gomock.Any(), gomock.Any()).
		Return(policyExternal, nil).
		Times(1)

	err = controller.syncAutogrowPolicyToOrchestrator(ctx, createdPolicy, true)
	assert.Error(t, err)
	assert.True(t, errors.IsUnsupportedConfigError(err))

	// Verify status reflects validation failure
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, string(tridentv1.TridentAutogrowPolicyStateFailed), updatedPolicy.Status.State)
	assert.Contains(t, updatedPolicy.Status.Message, "Validation failed")
}

// TestDeleteAutogrowPolicy_Success tests successful deletion
func TestDeleteAutogrowPolicy_Success(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	// Set deletion timestamp to simulate Kubernetes deletion flow
	now := metav1.Now()
	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-policy-delete-success",
			Finalizers:        []string{"trident.netapp.io"},
			DeletionTimestamp: &now,
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			MaxSize:       "1000Gi",
		},
	}

	// Create policy in fake client
	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Mock orchestrator DeleteAutogrowPolicy success (returns nil for hard delete)
	orchestrator.EXPECT().
		DeleteAutogrowPolicy(gomock.Any(), "test-policy-delete-success").
		Return(nil).
		Times(1)

	// Mock GetAutogrowPolicy returning AutogrowPolicyNotFoundError (policy was hard deleted)
	orchestrator.EXPECT().
		GetAutogrowPolicy(gomock.Any(), "test-policy-delete-success").
		Return(nil, errors.AutogrowPolicyNotFoundError("test-policy-delete-success")).
		Times(1)

	err = controller.deleteAutogrowPolicy(ctx, createdPolicy)
	assert.NoError(t, err)

	// Verify finalizers were removed
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	if err == nil {
		// Policy still exists, verify finalizers were removed
		assert.Empty(t, updatedPolicy.Finalizers)
	}
	// If policy is deleted (err != nil), that's also correct behavior
}

// TestDeleteAutogrowPolicy_PolicyInUse tests soft delete when policy has volumes
func TestDeleteAutogrowPolicy_PolicyInUse(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy-in-use",
			Finalizers: []string{"trident.netapp.io"},
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			MaxSize:       "1000Gi",
		},
	}

	// Create policy in fake client
	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Mock orchestrator DeleteAutogrowPolicy success (soft deletes when volumes exist)
	orchestrator.EXPECT().
		DeleteAutogrowPolicy(gomock.Any(), "test-policy-in-use").
		Return(nil).
		Times(1)

	// Mock GetAutogrowPolicy returning soft-deleted policy with volumes
	softDeletedPolicy := &storage.AutogrowPolicyExternal{
		Name:          "test-policy-in-use",
		UsedThreshold: "80%",
		MaxSize:       "1000Gi",
		State:         storage.AutogrowPolicyStateDeleting,
		Volumes:       []string{"vol1", "vol2"},
		VolumeCount:   2,
	}
	orchestrator.EXPECT().
		GetAutogrowPolicy(gomock.Any(), "test-policy-in-use").
		Return(softDeletedPolicy, nil).
		Times(1)

	err = controller.deleteAutogrowPolicy(ctx, createdPolicy)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileDeferredError(err))

	// Verify finalizers were NOT removed (deletion in progress)
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Contains(t, updatedPolicy.Finalizers, "trident.netapp.io")

	// Verify status was updated to Deleting
	assert.Equal(t, string(tridentv1.TridentAutogrowPolicyStateDeleting), updatedPolicy.Status.State)
	assert.Contains(t, updatedPolicy.Status.Message, "in use by 2 volume(s)")
}

// TestDeleteAutogrowPolicy_OrchestratorNotFoundIsOK tests not found is acceptable
func TestDeleteAutogrowPolicy_OrchestratorNotFoundIsOK(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	// Set deletion timestamp to simulate Kubernetes deletion flow
	now := metav1.Now()
	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-policy-not-found",
			Finalizers:        []string{"trident.netapp.io"},
			DeletionTimestamp: &now,
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			MaxSize:       "1000Gi",
		},
	}

	// Create policy in fake client
	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Mock orchestrator DeleteAutogrowPolicy returning not found (acceptable)
	notFoundErr := errors.AutogrowPolicyNotFoundError("test-policy-not-found")
	orchestrator.EXPECT().
		DeleteAutogrowPolicy(gomock.Any(), "test-policy-not-found").
		Return(notFoundErr).
		Times(1)

	err = controller.deleteAutogrowPolicy(ctx, createdPolicy)
	assert.NoError(t, err) // Not found is acceptable

	// Verify finalizers were removed
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	if err == nil {
		// Policy still exists, verify finalizers were removed
		assert.Empty(t, updatedPolicy.Finalizers)
	}
	// If policy is deleted (err != nil), that's also correct behavior
}

// TestDeleteAutogrowPolicy_OrchestratorOtherError tests other orchestrator errors
func TestDeleteAutogrowPolicy_OrchestratorOtherError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy-other-error",
			Finalizers: []string{"trident.netapp.io"},
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			MaxSize:       "1000Gi",
		},
	}

	// Create policy in fake client
	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Mock orchestrator DeleteAutogrowPolicy returning generic error
	genericErr := fmt.Errorf("database connection error")
	orchestrator.EXPECT().
		DeleteAutogrowPolicy(gomock.Any(), "test-policy-other-error").
		Return(genericErr).
		Times(1)

	err = controller.deleteAutogrowPolicy(ctx, createdPolicy)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileDeferredError(err))

	// Verify finalizers were NOT removed
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Contains(t, updatedPolicy.Finalizers, "trident.netapp.io")
}

// TestHandleAutogrowPolicy_WithOrchestrator_AddSuccess tests full add flow
func TestHandleAutogrowPolicy_WithOrchestrator_AddSuccess(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-full-add",
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			GrowthAmount:  "20%",
			MaxSize:       "1000Gi",
		},
	}

	// Create policy
	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Add to informer cache
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(createdPolicy)

	// Mock orchestrator
	policyExternal := &storage.AutogrowPolicyExternal{
		Name:          "test-full-add",
		UsedThreshold: "80%",
		GrowthAmount:  "20%",
		MaxSize:       "1000Gi",
		State:         storage.AutogrowPolicyStateSuccess,
	}

	orchestrator.EXPECT().
		AddAutogrowPolicy(gomock.Any(), gomock.Any()).
		Return(policyExternal, nil).
		Times(1)

	keyItem := &KeyItem{
		key:        "test-full-add",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentAutogrowPolicy,
	}

	err = controller.handleAutogrowPolicy(keyItem)
	assert.NoError(t, err)

	// Verify complete success
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Contains(t, updatedPolicy.Finalizers, "trident.netapp.io")
	assert.Equal(t, string(tridentv1.TridentAutogrowPolicyStateSuccess), updatedPolicy.Status.State)
}

// TestHandleAutogrowPolicy_WithOrchestrator_DeleteWithDeletionTimestamp tests deletion flow
func TestHandleAutogrowPolicy_WithOrchestrator_DeleteWithDeletionTimestamp(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	now := metav1.Now()
	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-full-delete",
			Finalizers:        []string{"trident.netapp.io"},
			DeletionTimestamp: &now,
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			MaxSize:       "1000Gi",
		},
	}

	// Create policy
	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Add to informer cache
	controller.crdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer().GetIndexer().Add(createdPolicy)

	// Mock orchestrator delete success
	orchestrator.EXPECT().
		DeleteAutogrowPolicy(gomock.Any(), "test-full-delete").
		Return(nil).
		Times(1)

	keyItem := &KeyItem{
		key:        "test-full-delete",
		event:      EventAdd, // Event type doesn't matter when deletion timestamp is set
		ctx:        ctx,
		objectType: ObjectTypeTridentAutogrowPolicy,
	}

	// Mock GetAutogrowPolicy returning AutogrowPolicyNotFoundError (policy was hard deleted)
	orchestrator.EXPECT().
		GetAutogrowPolicy(gomock.Any(), "test-full-delete").
		Return(nil, errors.AutogrowPolicyNotFoundError("test-full-delete")).
		Times(1)

	err = controller.handleAutogrowPolicy(keyItem)
	assert.NoError(t, err)

	// Verify finalizers removed (policy may be deleted by now)
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	if err == nil {
		// Policy still exists, verify finalizers were removed
		assert.Empty(t, updatedPolicy.Finalizers)
	}
	// If policy is deleted (err != nil), that's also correct behavior
}

// TestDeleteAutogrowPolicy_AlreadyDeleted tests policy already deleted from orchestrator
func TestDeleteAutogrowPolicy_AlreadyDeleted(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	now := metav1.Now()
	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-policy-already-deleted",
			Finalizers:        []string{"trident.netapp.io"},
			DeletionTimestamp: &now,
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			MaxSize:       "1000Gi",
		},
	}

	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Mock orchestrator DeleteAutogrowPolicy returning AutogrowPolicyNotFoundError
	orchestrator.EXPECT().
		DeleteAutogrowPolicy(gomock.Any(), "test-policy-already-deleted").
		Return(errors.AutogrowPolicyNotFoundError("test-policy-already-deleted")).
		Times(1)

	err = controller.deleteAutogrowPolicy(ctx, createdPolicy)
	assert.NoError(t, err)

	// Verify finalizers were removed
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	if err == nil {
		assert.Empty(t, updatedPolicy.Finalizers)
	}
}

// TestDeleteAutogrowPolicy_SoftDeletedThenHardDeleted tests transition from soft to hard delete
func TestDeleteAutogrowPolicy_SoftDeletedThenHardDeleted(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	// Set deletion timestamp to trigger finalizer removal
	now := metav1.Now()
	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-policy-transition",
			Finalizers:        []string{"trident.netapp.io"},
			DeletionTimestamp: &now,
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			MaxSize:       "1000Gi",
		},
		Status: tridentv1.TridentAutogrowPolicyStatus{
			State:   string(tridentv1.TridentAutogrowPolicyStateDeleting),
			Message: "Policy is in use by 1 volume(s), waiting for volumes to be disassociated: vol1",
		},
	}

	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// First call: DeleteAutogrowPolicy succeeds
	orchestrator.EXPECT().
		DeleteAutogrowPolicy(gomock.Any(), "test-policy-transition").
		Return(nil).
		Times(1)

	// GetAutogrowPolicy returns AutogrowPolicyNotFoundError (volumes were disassociated, policy hard deleted)
	orchestrator.EXPECT().
		GetAutogrowPolicy(gomock.Any(), "test-policy-transition").
		Return(nil, errors.AutogrowPolicyNotFoundError("test-policy-transition")).
		Times(1)

	err = controller.deleteAutogrowPolicy(ctx, createdPolicy)
	assert.NoError(t, err)

	// Verify finalizers were removed (hard delete completed)
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	if err == nil {
		assert.Empty(t, updatedPolicy.Finalizers)
	}
}

// TestDeleteAutogrowPolicy_GetPolicyError tests error when getting policy state
func TestDeleteAutogrowPolicy_GetPolicyError(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy-get-error",
			Finalizers: []string{"trident.netapp.io"},
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			MaxSize:       "1000Gi",
		},
	}

	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Mock DeleteAutogrowPolicy success
	orchestrator.EXPECT().
		DeleteAutogrowPolicy(gomock.Any(), "test-policy-get-error").
		Return(nil).
		Times(1)

	// Mock GetAutogrowPolicy returning unexpected error
	orchestrator.EXPECT().
		GetAutogrowPolicy(gomock.Any(), "test-policy-get-error").
		Return(nil, fmt.Errorf("database connection error")).
		Times(1)

	err = controller.deleteAutogrowPolicy(ctx, createdPolicy)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileDeferredError(err))

	// Verify finalizers were NOT removed
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Contains(t, updatedPolicy.Finalizers, "trident.netapp.io")
}

// TestDeleteAutogrowPolicy_PolicyExistsWithZeroVolumes tests edge case
func TestDeleteAutogrowPolicy_PolicyExistsWithZeroVolumes(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy-zero-volumes",
			Finalizers: []string{"trident.netapp.io"},
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "80%",
			MaxSize:       "1000Gi",
		},
	}

	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Mock DeleteAutogrowPolicy success
	orchestrator.EXPECT().
		DeleteAutogrowPolicy(gomock.Any(), "test-policy-zero-volumes").
		Return(nil).
		Times(1)

	// Mock GetAutogrowPolicy returning policy with 0 volumes (edge case - shouldn't happen)
	policyExternal := &storage.AutogrowPolicyExternal{
		Name:          "test-policy-zero-volumes",
		UsedThreshold: "80%",
		MaxSize:       "1000Gi",
		State:         storage.AutogrowPolicyStateSuccess,
		Volumes:       []string{},
		VolumeCount:   0,
	}
	orchestrator.EXPECT().
		GetAutogrowPolicy(gomock.Any(), "test-policy-zero-volumes").
		Return(policyExternal, nil).
		Times(1)

	err = controller.deleteAutogrowPolicy(ctx, createdPolicy)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileDeferredError(err))
	assert.Contains(t, err.Error(), "policy exists with 0 volumes")

	// Verify finalizers were NOT removed (will retry)
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Contains(t, updatedPolicy.Finalizers, "trident.netapp.io")
}

// TestDeleteAutogrowPolicy_MultipleVolumes tests soft delete with multiple volumes
func TestDeleteAutogrowPolicy_MultipleVolumes(t *testing.T) {
	controller, mockCtrl, orchestrator := setupAutogrowPolicyTestWithMock(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-policy-multi-volumes",
			Finalizers: []string{"trident.netapp.io"},
		},
		Spec: tridentv1.TridentAutogrowPolicySpec{
			UsedThreshold: "85%",
			GrowthAmount:  "15%",
			MaxSize:       "2000Gi",
		},
	}

	createdPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
		ctx, policy, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Mock DeleteAutogrowPolicy success (soft delete)
	orchestrator.EXPECT().
		DeleteAutogrowPolicy(gomock.Any(), "test-policy-multi-volumes").
		Return(nil).
		Times(1)

	// Mock GetAutogrowPolicy returning soft-deleted policy with 5 volumes
	softDeletedPolicy := &storage.AutogrowPolicyExternal{
		Name:          "test-policy-multi-volumes",
		UsedThreshold: "85%",
		GrowthAmount:  "15%",
		MaxSize:       "2000Gi",
		State:         storage.AutogrowPolicyStateDeleting,
		Volumes:       []string{"vol1", "vol2", "vol3", "vol4", "vol5"},
		VolumeCount:   5,
	}
	orchestrator.EXPECT().
		GetAutogrowPolicy(gomock.Any(), "test-policy-multi-volumes").
		Return(softDeletedPolicy, nil).
		Times(1)

	err = controller.deleteAutogrowPolicy(ctx, createdPolicy)
	assert.Error(t, err)
	assert.True(t, errors.IsReconcileDeferredError(err))

	// Verify status was updated
	updatedPolicy, err := controller.crdClientset.TridentV1().TridentAutogrowPolicies().Get(
		ctx, policy.Name, metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, string(tridentv1.TridentAutogrowPolicyStateDeleting), updatedPolicy.Status.State)
	assert.Contains(t, updatedPolicy.Status.Message, "in use by 5 volume(s)")
	assert.Contains(t, updatedPolicy.Status.Message, "waiting for volumes to be disassociated")

	// Verify finalizers still present
	assert.Contains(t, updatedPolicy.Finalizers, "trident.netapp.io")
}
