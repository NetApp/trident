// Copyright 2023 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mockcore "github.com/netapp/trident/mocks/mock_core"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
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
