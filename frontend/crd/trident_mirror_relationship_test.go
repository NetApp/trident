// Copyright 2023 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mockcore "github.com/netapp/trident/mocks/mock_core"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

func TestUpdateMirrorRelationshipNoUpdateNeeded(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	crdClient := GetTestCrdClientset()
	snapClient := GetTestSnapshotClientset()

	// Test no changes to TMR
	controller, _ := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	oldRelationship := &netappv1.TridentMirrorRelationship{
		Status: netappv1.TridentMirrorRelationshipStatus{
			Conditions: []*netappv1.TridentMirrorRelationshipCondition{
				{MirrorState: ""},
			},
		},
	}
	newRelationship := oldRelationship.DeepCopy()

	controller.updateTMRHandler(oldRelationship, newRelationship)

	if controller.workqueue.Len() != 0 {
		t.Fatalf("Did not expect update")
	}

	// Test changes but desired state and actual state are equal
	controller, _ = newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	oldRelationship = &netappv1.TridentMirrorRelationship{
		Spec: netappv1.TridentMirrorRelationshipSpec{
			MirrorState:    netappv1.MirrorStateEstablished,
			VolumeMappings: []*netappv1.TridentMirrorRelationshipVolumeMapping{{LocalPVCName: "blah"}},
		},
		Status: netappv1.TridentMirrorRelationshipStatus{
			Conditions: []*netappv1.TridentMirrorRelationshipCondition{
				{MirrorState: netappv1.MirrorStateEstablished},
			},
		},
	}
	newRelationship = oldRelationship.DeepCopy()
	newRelationship.Spec.VolumeMappings[0].RemoteVolumeHandle = "blah"

	controller.updateTMRHandler(oldRelationship, newRelationship)

	if controller.workqueue.Len() != 0 {
		t.Fatalf("Did not expect update")
	}
}

func TestUpdateMirrorRelationshipNeedsUpdate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	crdClient := GetTestCrdClientset()
	snapClient := GetTestSnapshotClientset()

	// Test the old relationship has no status conditions
	controller, _ := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	newRelationship := &netappv1.TridentMirrorRelationship{}

	controller.updateTMRHandler(newRelationship, newRelationship)

	if controller.workqueue.Len() != 1 {
		t.Fatalf("Expected an update to be required")
	}

	// Test updated relationship has Deletion timestamp
	controller, _ = newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	oldRelationship := &netappv1.TridentMirrorRelationship{
		Status: netappv1.TridentMirrorRelationshipStatus{
			Conditions: []*netappv1.TridentMirrorRelationshipCondition{
				{MirrorState: ""},
			},
		},
	}
	newRelationship = oldRelationship.DeepCopy()
	newRelationship.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: time.Now()}

	controller.updateTMRHandler(oldRelationship, newRelationship)

	if controller.workqueue.Len() != 1 {
		t.Fatalf("Expected an update to be required")
	}

	// Test change in desired state from actual state
	controller, _ = newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	oldRelationship = &netappv1.TridentMirrorRelationship{
		Spec: netappv1.TridentMirrorRelationshipSpec{
			MirrorState: netappv1.MirrorStateEstablished,
		},
		Status: netappv1.TridentMirrorRelationshipStatus{
			Conditions: []*netappv1.TridentMirrorRelationshipCondition{
				{MirrorState: netappv1.MirrorStateEstablished},
			},
		},
	}
	newRelationship = oldRelationship.DeepCopy()
	newRelationship.Spec.MirrorState = netappv1.MirrorStatePromoted
	newRelationship.Generation = oldRelationship.Generation + 1

	controller.updateTMRHandler(oldRelationship, newRelationship)

	if controller.workqueue.Len() != 1 {
		t.Fatalf("Expected an update to be required")
	}
}

func TestUpdateTMRConditionLocalFields(t *testing.T) {
	localPVCName := "test_pvc"
	remoteVolumeHandle := "ontap_svm2:ontap_flexvol"
	expectedStatusCondition := &netappv1.TridentMirrorRelationshipCondition{
		LocalPVCName:       localPVCName,
		RemoteVolumeHandle: remoteVolumeHandle,
	}
	statusCondition := &netappv1.TridentMirrorRelationshipCondition{}
	newStatusCondition, err := updateTMRConditionLocalFields(
		statusCondition, localPVCName, remoteVolumeHandle)
	if err != nil {
		t.Errorf("Got error updating TMR condition")
	}
	if !reflect.DeepEqual(newStatusCondition, expectedStatusCondition) {
		t.Fatalf("Actual does not equal expected, actual: %v, expected: %v",
			newStatusCondition, expectedStatusCondition)
	}

	// Test no flexvol or svm name
	localPVCName = "test_pvc"
	expectedStatusCondition = &netappv1.TridentMirrorRelationshipCondition{
		LocalPVCName:       localPVCName,
		RemoteVolumeHandle: remoteVolumeHandle,
	}
	statusCondition = &netappv1.TridentMirrorRelationshipCondition{}
	newStatusCondition, err = updateTMRConditionLocalFields(
		statusCondition, localPVCName, remoteVolumeHandle)
	if err != nil {
		t.Errorf("Got error updating TMR condition")
	}
	if !reflect.DeepEqual(newStatusCondition, expectedStatusCondition) {
		t.Fatalf("Actual does not equal expected, actual: %v, expected: %v",
			newStatusCondition, expectedStatusCondition)
	}
}

func TestValidateTMRUpdate_InvalidPolicyUpdate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	ctx := context.Background()

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	crdClient := GetTestCrdClientset()
	snapClient := GetTestSnapshotClientset()

	controller, _ := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	conditions := &netappv1.TridentMirrorRelationshipCondition{}
	oldRelationship := &netappv1.TridentMirrorRelationship{}
	newRelationship := &netappv1.TridentMirrorRelationship{}
	errMessage := "replication policy may not be changed"

	oldRelationship.Spec.ReplicationPolicy = "Sync"

	newRelationship.Spec.MirrorState = netappv1.MirrorStateEstablished
	newRelationship.Spec.ReplicationPolicy = "MirrorAllSnapshots"

	conditions.MirrorState = netappv1.MirrorStateEstablishing
	conditions.ReplicationPolicy = "Sync"
	newRelationship.Status.Conditions = append(newRelationship.Status.Conditions, conditions)
	oldRelationship.Status.Conditions = append(oldRelationship.Status.Conditions, conditions)

	conditionCopy, err := controller.validateTMRUpdate(ctx, oldRelationship, newRelationship)

	assert.Error(t, err, "Should be invalid update error")
	assert.Equal(t, netappv1.MirrorStateFailed, conditionCopy.MirrorState,
		"Mirror state should be failed")
	assert.Contains(t, conditionCopy.Message, errMessage, "Wrong error message")
}

func TestValidateTMRUpdate_InvalidScheduleUpdate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	ctx := context.Background()

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	crdClient := GetTestCrdClientset()
	snapClient := GetTestSnapshotClientset()

	controller, _ := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	conditions := &netappv1.TridentMirrorRelationshipCondition{}
	oldRelationship := &netappv1.TridentMirrorRelationship{}
	newRelationship := &netappv1.TridentMirrorRelationship{}
	errMessage := "replication schedule may not be changed"

	oldRelationship.Spec.ReplicationPolicy = "MirrorAllSnapshots"
	oldRelationship.Spec.ReplicationSchedule = "1min"

	newRelationship.Spec.MirrorState = netappv1.MirrorStateReestablished
	newRelationship.Spec.ReplicationPolicy = "MirrorAllSnapshots"
	newRelationship.Spec.ReplicationSchedule = "2min"

	conditions.MirrorState = netappv1.MirrorStateEstablishing
	conditions.ReplicationPolicy = "MirrorAllSnapshots"
	conditions.ReplicationSchedule = "1min"
	newRelationship.Status.Conditions = append(newRelationship.Status.Conditions, conditions)
	oldRelationship.Status.Conditions = append(oldRelationship.Status.Conditions, conditions)

	conditionCopy, err := controller.validateTMRUpdate(ctx, oldRelationship, newRelationship)

	assert.Error(t, err, "Should be invalid update error")
	assert.Equal(t, netappv1.MirrorStateFailed, conditionCopy.MirrorState,
		"Mirror state should be failed")
	assert.Contains(t, conditionCopy.Message, errMessage, "Wrong error message")
}

func TestUpdateTMRConditionReplicationSettings(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	ctx := context.Background()

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	crdClient := GetTestCrdClientset()
	snapClient := GetTestSnapshotClientset()

	controller, _ := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)

	cases := []struct {
		initialPolicy    string
		initialSchedule  string
		returnedPolicy   string
		returnedSchedule string
		returnedSVMName  string
		expectedPolicy   string
		expectedSchedule string
	}{
		{
			"",
			"",
			"",
			"",
			"",
			"",
			"",
		},
		{
			"",
			"",
			"TestReplicationPolicy",
			"TestReplicationSchedule",
			"SVM1",
			"TestReplicationPolicy",
			"TestReplicationSchedule",
		},
		{
			"TestReplicationPolicy_Old",
			"TestReplicationSchedule_Old",
			"TestReplicationPolicy",
			"TestReplicationSchedule",
			"SVM1",
			"TestReplicationPolicy",
			"TestReplicationSchedule",
		},
	}

	for _, c := range cases {
		condition := &netappv1.TridentMirrorRelationshipCondition{ReplicationPolicy: c.initialPolicy, ReplicationSchedule: c.initialSchedule}
		volume := &storage.VolumeExternal{}
		localVolumeInternalName := "pvc_1"

		orchestrator.EXPECT().GetReplicationDetails(ctx, volume.BackendUUID, localVolumeInternalName, "").Return(c.returnedPolicy,
			c.returnedSchedule, c.returnedSVMName, nil).Times(1)

		actualCondition := controller.updateTMRConditionReplicationSettings(ctx, condition, volume, localVolumeInternalName, "")
		expectedVolumeHandle := c.returnedSVMName + ":" + localVolumeInternalName

		assert.Equal(t, c.expectedPolicy, actualCondition.ReplicationPolicy, "Replication Policy does not match")
		assert.Equal(t, c.expectedSchedule, actualCondition.ReplicationSchedule, "Replication Schedule does not match")
		assert.Equal(t, expectedVolumeHandle, actualCondition.LocalVolumeHandle, "LocalVolumeHandle does not match")
	}
}

func TestUpdateTMRConditionReplicationSettings_ErrorGettingDetails(t *testing.T) {
	// Test updateTMRConditionReplicationSettings when the orchestrator fails to return relationship details
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	ctx := context.Background()

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	crdClient := GetTestCrdClientset()
	snapClient := GetTestSnapshotClientset()
	localVolumeInternalName := "pvc_1"

	controller, _ := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)

	cases := []struct {
		initialPolicy    string
		initialSchedule  string
		returnedPolicy   string
		returnedSchedule string
		returnedSVMName  string
		expectedPolicy   string
		expectedSchedule string
	}{
		{
			"",
			"",
			"",
			"",
			"",
			"",
			"",
		},
		{
			"",
			"",
			"TestReplicationPolicy",
			"TestReplicationSchedule",
			"SVM1",
			"",
			"",
		},
		{
			"TestReplicationPolicy_Old",
			"TestReplicationSchedule_Old",
			"TestReplicationPolicy",
			"TestReplicationSchedule",
			"SVM1",
			"TestReplicationPolicy_Old",
			"TestReplicationSchedule_Old",
		},
	}

	for _, c := range cases {
		condition := &netappv1.TridentMirrorRelationshipCondition{ReplicationPolicy: c.initialPolicy, ReplicationSchedule: c.initialSchedule}
		volume := &storage.VolumeExternal{}

		orchestrator.EXPECT().GetReplicationDetails(ctx, volume.BackendUUID, localVolumeInternalName, "").Return(c.returnedPolicy,
			c.returnedSchedule, c.returnedSVMName, fmt.Errorf("Fake error")).Times(1)

		actualCondition := controller.updateTMRConditionReplicationSettings(ctx, condition, volume, localVolumeInternalName, "")
		expectedVolumeHandle := c.returnedSVMName + ":" + localVolumeInternalName

		assert.Equal(t, c.expectedPolicy, actualCondition.ReplicationPolicy, "Replication Policy does not match")
		assert.Equal(t, c.expectedSchedule, actualCondition.ReplicationSchedule, "Replication Schedule does not match")
		assert.Equal(t, expectedVolumeHandle, actualCondition.LocalVolumeHandle, "LocalVolumeHandle does not match")
	}
}

// ===============================
// Comprehensive Tests for Phase 1 - TridentMirrorRelationship
// ===============================

func setupMirrorRelationshipTest(t *testing.T) (*TridentCrdController, *gomock.Controller, *mockcore.MockOrchestrator) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	return crdController, mockCtrl, orchestrator
}

func createTestTMR(name, namespace string) *netappv1.TridentMirrorRelationship {
	return &netappv1.TridentMirrorRelationship{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: []string{netappv1.TridentFinalizerName},
		},
		Spec: netappv1.TridentMirrorRelationshipSpec{
			MirrorState: netappv1.MirrorStateEstablished,
			VolumeMappings: []*netappv1.TridentMirrorRelationshipVolumeMapping{
				{
					LocalPVCName:       "test-pvc",
					RemoteVolumeHandle: "svm1:test-volume",
				},
			},
			ReplicationPolicy:   "MirrorAllSnapshots",
			ReplicationSchedule: "5min",
		},
		Status: netappv1.TridentMirrorRelationshipStatus{
			Conditions: []*netappv1.TridentMirrorRelationshipCondition{
				{
					LocalPVCName:        "test-pvc",
					RemoteVolumeHandle:  "svm1:test-volume",
					MirrorState:         netappv1.MirrorStateEstablished,
					ReplicationPolicy:   "MirrorAllSnapshots",
					ReplicationSchedule: "5min",
				},
			},
		},
	}
}

func TestHandleTridentMirrorRelationship_EdgeCases(t *testing.T) {
	t.Run("InvalidKey", func(t *testing.T) {
		controller, mockCtrl, _ := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		keyItem := &KeyItem{
			key:        "invalid-key-format",
			event:      EventAdd,
			ctx:        ctx,
			objectType: ObjectTypeTridentMirrorRelationship,
		}

		err := controller.handleTridentMirrorRelationship(keyItem)
		assert.NoError(t, err) // Invalid key should return nil
	})

	t.Run("TMRNotFound", func(t *testing.T) {
		controller, mockCtrl, _ := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		keyItem := &KeyItem{
			key:        "default/non-existent-tmr",
			event:      EventAdd,
			ctx:        ctx,
			objectType: ObjectTypeTridentMirrorRelationship,
		}

		err := controller.handleTridentMirrorRelationship(keyItem)
		assert.NoError(t, err) // Not found should return nil
	})

	t.Run("TMRWithoutFinalizers", func(t *testing.T) {
		controller, mockCtrl, _ := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		tmr := createTestTMR("test-tmr", "default")
		tmr.Finalizers = nil // Remove finalizers

		// Create the TMR
		_, err := controller.crdClientset.TridentV1().TridentMirrorRelationships("default").Create(ctx, tmr, metav1.CreateOptions{})
		assert.NoError(t, err)

		keyItem := &KeyItem{
			key:        "default/test-tmr",
			event:      EventAdd,
			ctx:        ctx,
			objectType: ObjectTypeTridentMirrorRelationship,
		}

		// This will not find the TMR in lister (cache not populated in test)
		err = controller.handleTridentMirrorRelationship(keyItem)
		assert.NoError(t, err)
	})

	t.Run("TMRDeletingWithFinalizers", func(t *testing.T) {
		controller, mockCtrl, _ := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		tmr := createTestTMR("test-tmr", "default")
		now := metav1.Now()
		tmr.DeletionTimestamp = &now // Mark for deletion

		// Create the TMR
		_, err := controller.crdClientset.TridentV1().TridentMirrorRelationships("default").Create(ctx, tmr, metav1.CreateOptions{})
		assert.NoError(t, err)

		keyItem := &KeyItem{
			key:        "default/test-tmr",
			event:      EventAdd,
			ctx:        ctx,
			objectType: ObjectTypeTridentMirrorRelationship,
		}

		// This will not find the TMR in lister (cache not populated in test)
		err = controller.handleTridentMirrorRelationship(keyItem)
		assert.NoError(t, err)
	})

	t.Run("InvalidTMR", func(t *testing.T) {
		controller, mockCtrl, _ := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		tmr := createTestTMR("test-tmr", "default")
		tmr.Spec.VolumeMappings = nil // Make it invalid

		// Create the TMR
		_, err := controller.crdClientset.TridentV1().TridentMirrorRelationships("default").Create(ctx, tmr, metav1.CreateOptions{})
		assert.NoError(t, err)

		keyItem := &KeyItem{
			key:        "default/test-tmr",
			event:      EventAdd,
			ctx:        ctx,
			objectType: ObjectTypeTridentMirrorRelationship,
		}

		err = controller.handleTridentMirrorRelationship(keyItem)
		assert.NoError(t, err)
	})
}

func TestHandleIndividualVolumeMapping_EdgeCases(t *testing.T) {
	t.Run("PVCNotFound", func(t *testing.T) {
		controller, mockCtrl, _ := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()
		tmr := createTestTMR("test-tmr", "default")
		volumeMapping := tmr.Spec.VolumeMappings[0]
		condition := &netappv1.TridentMirrorRelationshipCondition{
			LocalPVCName: "test-pvc",
		}

		result, err := controller.handleIndividualVolumeMapping(ctx, tmr, volumeMapping, condition)
		assert.Nil(t, result)
		assert.Error(t, err)
		assert.True(t, errors.IsReconcileDeferredError(err))
		assert.Contains(t, err.Error(), "Local PVC for TridentMirrorRelationship does not yet exist")
	})

	t.Run("PVCExistsButPVNotFound", func(t *testing.T) {
		controller, mockCtrl, _ := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Create PVC without PV
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "default",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "test-pv",
			},
		}
		_, err := controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(ctx, pvc, metav1.CreateOptions{})
		assert.NoError(t, err)

		tmr := createTestTMR("test-tmr", "default")
		volumeMapping := tmr.Spec.VolumeMappings[0]
		condition := &netappv1.TridentMirrorRelationshipCondition{
			LocalPVCName: "test-pvc",
		}

		result, err := controller.handleIndividualVolumeMapping(ctx, tmr, volumeMapping, condition)
		assert.Nil(t, result)
		assert.Error(t, err)
		assert.True(t, errors.IsReconcileDeferredError(err))
		assert.Contains(t, err.Error(), "PV for local PVC for TridentMirrorRelationship does not yet exist")
	})

	t.Run("PVExistsWithoutCSI", func(t *testing.T) {
		controller, mockCtrl, _ := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Create PV without CSI
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pv",
			},
			Spec: corev1.PersistentVolumeSpec{
				// No CSI spec
			},
		}
		_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Create PVC
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "default",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "test-pv",
			},
		}
		_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(ctx, pvc, metav1.CreateOptions{})
		assert.NoError(t, err)

		tmr := createTestTMR("test-tmr", "default")
		volumeMapping := tmr.Spec.VolumeMappings[0]
		condition := &netappv1.TridentMirrorRelationshipCondition{
			LocalPVCName: "test-pvc",
		}

		result, err := controller.handleIndividualVolumeMapping(ctx, tmr, volumeMapping, condition)
		assert.Nil(t, result)
		assert.Error(t, err)
		assert.True(t, errors.IsReconcileDeferredError(err))
		assert.Contains(t, err.Error(), "PV for local PVC for TridentMirrorRelationship does not yet exist")
	})

	t.Run("PVExistsWithoutInternalName", func(t *testing.T) {
		controller, mockCtrl, _ := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Create PV with CSI but without internal name
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pv",
			},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					CSI: &corev1.CSIPersistentVolumeSource{
						VolumeHandle: "test-volume-handle",
						VolumeAttributes: map[string]string{
							// Missing internalName
							"protocol": "file",
						},
					},
				},
			},
		}
		_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Create PVC
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "default",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "test-pv",
			},
		}
		_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(ctx, pvc, metav1.CreateOptions{})
		assert.NoError(t, err)

		tmr := createTestTMR("test-tmr", "default")
		volumeMapping := tmr.Spec.VolumeMappings[0]
		condition := &netappv1.TridentMirrorRelationshipCondition{
			LocalPVCName: "test-pvc",
		}

		result, err := controller.handleIndividualVolumeMapping(ctx, tmr, volumeMapping, condition)
		assert.Nil(t, result)
		assert.Error(t, err)
		assert.True(t, errors.IsReconcileDeferredError(err))
		assert.Contains(t, err.Error(), "does not yet have an internal volume name set")
	})

	t.Run("VolumeNotFoundInOrchestrator", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Create complete PV with CSI and internal name
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pv",
			},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					CSI: &corev1.CSIPersistentVolumeSource{
						VolumeHandle: "test-volume-handle",
						VolumeAttributes: map[string]string{
							"internalName": "test-internal-volume",
							"protocol":     "file",
						},
					},
				},
			},
		}
		_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Create PVC
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "default",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "test-pv",
			},
		}
		_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(ctx, pvc, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Mock orchestrator to return volume not found
		orchestrator.EXPECT().GetVolume(ctx, "test-volume-handle").Return(nil, errors.NotFoundError("volume not found"))

		tmr := createTestTMR("test-tmr", "default")
		volumeMapping := tmr.Spec.VolumeMappings[0]
		condition := &netappv1.TridentMirrorRelationshipCondition{
			LocalPVCName: "test-pvc",
		}

		result, err := controller.handleIndividualVolumeMapping(ctx, tmr, volumeMapping, condition)
		assert.NotNil(t, result)
		assert.NoError(t, err)
		assert.Equal(t, netappv1.MirrorStateFailed, result.MirrorState)
	})

	t.Run("BackendCannotMirror", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Create complete PV with CSI and internal name
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pv",
			},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					CSI: &corev1.CSIPersistentVolumeSource{
						VolumeHandle: "test-volume-handle",
						VolumeAttributes: map[string]string{
							"internalName": "test-internal-volume",
							"protocol":     "file",
						},
					},
				},
			},
		}
		_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Create PVC
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "default",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "test-pv",
			},
		}
		_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(ctx, pvc, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Mock orchestrator to return volume but backend cannot mirror
		mockVolume := &storage.VolumeExternal{
			BackendUUID: "test-backend-uuid",
			Config: &storage.VolumeConfig{
				InternalName: "test-internal-volume",
			},
		}
		orchestrator.EXPECT().GetVolume(ctx, "test-volume-handle").Return(mockVolume, nil)
		orchestrator.EXPECT().CanBackendMirror(ctx, "test-backend-uuid").Return(false, nil)

		tmr := createTestTMR("test-tmr", "default")
		volumeMapping := tmr.Spec.VolumeMappings[0]
		condition := &netappv1.TridentMirrorRelationshipCondition{
			LocalPVCName: "test-pvc",
		}

		result, err := controller.handleIndividualVolumeMapping(ctx, tmr, volumeMapping, condition)
		assert.NotNil(t, result)
		assert.NoError(t, err)
		assert.Equal(t, netappv1.MirrorStateInvalid, result.MirrorState)
		assert.Contains(t, result.Message, "backend does not support mirroring")
	})

	t.Run("PromotedMirrorSkipsBackendCheck", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Create complete PV with CSI and internal name
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pv",
			},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					CSI: &corev1.CSIPersistentVolumeSource{
						VolumeHandle: "test-volume-handle",
						VolumeAttributes: map[string]string{
							"internalName": "test-internal-volume",
							"protocol":     "file",
						},
					},
				},
			},
		}
		_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Create PVC
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "default",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "test-pv",
			},
		}
		_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(ctx, pvc, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Mock orchestrator to return volume
		mockVolume := &storage.VolumeExternal{
			BackendUUID: "test-backend-uuid",
			Config: &storage.VolumeConfig{
				InternalName: "test-internal-volume",
			},
		}
		orchestrator.EXPECT().GetVolume(ctx, "test-volume-handle").Return(mockVolume, nil)
		orchestrator.EXPECT().GetMirrorStatus(ctx, "test-backend-uuid", "test-internal-volume", "svm1:test-volume").Return("promoted", nil)
		// For promoted mirrors, also expect GetReplicationDetails call
		orchestrator.EXPECT().GetReplicationDetails(ctx, "test-backend-uuid", "test-internal-volume", "svm1:test-volume").Return("MirrorAllSnapshots", "5min", "svm1", nil)

		tmr := createTestTMR("test-tmr", "default")
		tmr.Spec.MirrorState = netappv1.MirrorStatePromoted // Set to promoted
		volumeMapping := tmr.Spec.VolumeMappings[0]
		condition := &netappv1.TridentMirrorRelationshipCondition{
			LocalPVCName: "test-pvc",
		}

		result, err := controller.handleIndividualVolumeMapping(ctx, tmr, volumeMapping, condition)
		assert.NotNil(t, result)
		assert.NoError(t, err)
		// Should not call CanBackendMirror for promoted mirrors
	})
}

func TestGetCurrentMirrorState_EdgeCases(t *testing.T) {
	t.Run("UnsupportedError", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Mock orchestrator to return unsupported error
		orchestrator.EXPECT().GetMirrorStatus(ctx, "backend-uuid", "local-volume", "remote-handle").
			Return("", errors.UnsupportedError("mirroring not supported"))

		state, err := controller.getCurrentMirrorState(ctx, netappv1.MirrorStateEstablished, "backend-uuid", "local-volume", "remote-handle")
		assert.NoError(t, err)
		assert.Equal(t, "promoted", state) // Should return "promoted" for unsupported error
	})

	t.Run("OtherError", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Mock orchestrator to return other error
		orchestrator.EXPECT().GetMirrorStatus(ctx, "backend-uuid", "local-volume", "remote-handle").
			Return("", fmt.Errorf("some other error"))

		state, err := controller.getCurrentMirrorState(ctx, netappv1.MirrorStateEstablished, "backend-uuid", "local-volume", "remote-handle")
		assert.Error(t, err)
		assert.Equal(t, "", state)
	})

	t.Run("SuccessfulStatus", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Mock orchestrator to return successful status
		orchestrator.EXPECT().GetMirrorStatus(ctx, "backend-uuid", "local-volume", "remote-handle").
			Return("established", nil)

		state, err := controller.getCurrentMirrorState(ctx, netappv1.MirrorStateEstablished, "backend-uuid", "local-volume", "remote-handle")
		assert.NoError(t, err)
		assert.Equal(t, "established", state)
	})
}

func TestEnsureMirrorReadyForDeletion_EdgeCases(t *testing.T) {
	t.Run("VolumeNotFoundIsReadyForDeletion", func(t *testing.T) {
		controller, mockCtrl, _ := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		tmr := createTestTMR("test-tmr", "default")
		volumeMapping := tmr.Spec.VolumeMappings[0]
		condition := &netappv1.TridentMirrorRelationshipCondition{
			LocalPVCName:      "test-pvc",
			LocalVolumeHandle: "svm1:test-volume",
		}

		// This test will try to handleIndividualVolumeMapping but since we don't have PVC, it will defer

		ready, err := controller.ensureMirrorReadyForDeletion(ctx, tmr, volumeMapping, condition)
		assert.NoError(t, err)
		assert.True(t, ready) // Should be ready when volume doesn't exist (reconcile deferred)
	})

	t.Run("MirrorInPromotedState", func(t *testing.T) {
		controller, mockCtrl, _ := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		tmr := createTestTMR("test-tmr", "default")
		volumeMapping := tmr.Spec.VolumeMappings[0]
		condition := &netappv1.TridentMirrorRelationshipCondition{
			LocalPVCName:      "test-pvc",
			LocalVolumeHandle: "svm1:test-volume",
			MirrorState:       netappv1.MirrorStatePromoted,
		}

		// This test will try to handleIndividualVolumeMapping but since we don't have PVC, it will defer

		ready, err := controller.ensureMirrorReadyForDeletion(ctx, tmr, volumeMapping, condition)
		assert.NoError(t, err)
		assert.True(t, ready) // Should be ready when already promoted
	})

	t.Run("MirrorNotPromotedNeedsPVCCreation", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Create complete PV with CSI and internal name
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pv",
			},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					CSI: &corev1.CSIPersistentVolumeSource{
						VolumeHandle: "test-volume-handle",
						VolumeAttributes: map[string]string{
							"internalName": "test-internal-volume",
							"protocol":     "file",
						},
					},
				},
			},
		}
		_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Create PVC
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "default",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "test-pv",
			},
		}
		_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(ctx, pvc, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Mock orchestrator calls for promotion workflow
		mockVolume := &storage.VolumeExternal{
			BackendUUID: "test-backend-uuid",
			Config: &storage.VolumeConfig{
				InternalName: "test-internal-volume",
			},
		}
		orchestrator.EXPECT().GetVolume(ctx, "test-volume-handle").Return(mockVolume, nil)
		orchestrator.EXPECT().GetMirrorStatus(ctx, "test-backend-uuid", "test-internal-volume", "svm1:test-volume").Return("promoted", nil)
		orchestrator.EXPECT().GetReplicationDetails(ctx, "test-backend-uuid", "test-internal-volume", "svm1:test-volume").Return("MirrorAllSnapshots", "5min", "svm1", nil)

		tmr := createTestTMR("test-tmr", "default")
		volumeMapping := tmr.Spec.VolumeMappings[0]
		condition := &netappv1.TridentMirrorRelationshipCondition{
			LocalPVCName:      "test-pvc",
			LocalVolumeHandle: "svm1:test-volume",
			MirrorState:       netappv1.MirrorStateEstablished,
		}

		ready, err := controller.ensureMirrorReadyForDeletion(ctx, tmr, volumeMapping, condition)
		assert.NoError(t, err)
		assert.True(t, ready) // Should be ready when status is promoted
	})

	t.Run("MirrorNotReadyForDeletion", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Create complete PV with CSI and internal name
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pv",
			},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					CSI: &corev1.CSIPersistentVolumeSource{
						VolumeHandle: "test-volume-handle",
						VolumeAttributes: map[string]string{
							"internalName": "test-internal-volume",
							"protocol":     "file",
						},
					},
				},
			},
		}
		_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Create PVC
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "default",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "test-pv",
			},
		}
		_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(ctx, pvc, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Mock orchestrator calls for promotion that results in "promoting" state (not ready for deletion)
		mockVolume := &storage.VolumeExternal{
			BackendUUID: "test-backend-uuid",
			Config: &storage.VolumeConfig{
				InternalName: "test-internal-volume",
				Name:         "test-volume",
			},
		}
		orchestrator.EXPECT().GetVolume(ctx, "test-volume-handle").Return(mockVolume, nil)
		orchestrator.EXPECT().GetMirrorStatus(ctx, "test-backend-uuid", "test-internal-volume", "svm1:test-volume").Return("established", nil)
		orchestrator.EXPECT().GetReplicationDetails(ctx, "test-backend-uuid", "test-internal-volume", "svm1:test-volume").Return("MirrorAllSnapshots", "5min", "svm1", nil)
		// For promotion attempt that succeeds but results in "promoting" state
		orchestrator.EXPECT().PromoteMirror(ctx, "test-backend-uuid", "test-volume", "test-internal-volume", "svm1:test-volume", "").Return(false, nil)

		tmr := createTestTMR("test-tmr", "default")
		volumeMapping := tmr.Spec.VolumeMappings[0]
		condition := &netappv1.TridentMirrorRelationshipCondition{
			LocalPVCName:      "test-pvc",
			LocalVolumeHandle: "svm1:test-volume",
			MirrorState:       netappv1.MirrorStateEstablished,
		}

		ready, err := controller.ensureMirrorReadyForDeletion(ctx, tmr, volumeMapping, condition)
		assert.NoError(t, err)
		assert.False(t, ready) // Should not be ready when promotion is in progress (MirrorStatePromoting)
	})

	t.Run("MirrorPromotionSucceeds", func(t *testing.T) {
		controller, mockCtrl, orchestrator := setupMirrorRelationshipTest(t)
		defer mockCtrl.Finish()

		ctx := context.Background()

		// Create complete PV with CSI and internal name
		pv := &corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-pv",
			},
			Spec: corev1.PersistentVolumeSpec{
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					CSI: &corev1.CSIPersistentVolumeSource{
						VolumeHandle: "test-volume-handle",
						VolumeAttributes: map[string]string{
							"internalName": "test-internal-volume",
							"protocol":     "file",
						},
					},
				},
			},
		}
		_, err := controller.kubeClientset.CoreV1().PersistentVolumes().Create(ctx, pv, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Create PVC
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pvc",
				Namespace: "default",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				VolumeName: "test-pv",
			},
		}
		_, err = controller.kubeClientset.CoreV1().PersistentVolumeClaims("default").Create(ctx, pvc, metav1.CreateOptions{})
		assert.NoError(t, err)

		// Mock orchestrator calls - when we have a promoted mirror already
		mockVolume := &storage.VolumeExternal{
			BackendUUID: "test-backend-uuid",
			Config: &storage.VolumeConfig{
				InternalName: "test-internal-volume",
			},
		}
		orchestrator.EXPECT().GetVolume(ctx, "test-volume-handle").Return(mockVolume, nil)
		orchestrator.EXPECT().GetMirrorStatus(ctx, "test-backend-uuid", "test-internal-volume", "svm1:test-volume").Return("promoted", nil)
		orchestrator.EXPECT().GetReplicationDetails(ctx, "test-backend-uuid", "test-internal-volume", "svm1:test-volume").Return("MirrorAllSnapshots", "5min", "svm1", nil)

		tmr := createTestTMR("test-tmr", "default")
		volumeMapping := tmr.Spec.VolumeMappings[0]
		condition := &netappv1.TridentMirrorRelationshipCondition{
			LocalPVCName:      "test-pvc",
			LocalVolumeHandle: "svm1:test-volume",
			MirrorState:       netappv1.MirrorStateEstablished,
		}

		ready, err := controller.ensureMirrorReadyForDeletion(ctx, tmr, volumeMapping, condition)
		assert.NoError(t, err)
		assert.True(t, ready) // Should be ready when mirror is already promoted
	})
}
