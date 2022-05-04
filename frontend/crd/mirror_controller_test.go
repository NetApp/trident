// Copyright 2021 NetApp, Inc. All Rights Reserved.

package crd

import (
	"reflect"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mockcore "github.com/netapp/trident/mocks/mock_core"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
)

func TestUpdateMirrorRelationshipNoUpdateNeeded(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	testingCache := NewTestingCache()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	crdClient := GetTestCrdClientset()
	snapClient := GetTestSnapshotClientset()
	addCrdTestReactors(crdClient, testingCache)

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

	controller.updateMirrorRelationship(oldRelationship, newRelationship)

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

	controller.updateMirrorRelationship(oldRelationship, newRelationship)

	if controller.workqueue.Len() != 0 {
		t.Fatalf("Did not expect update")
	}
}

func TestUpdateMirrorRelationshipNeedsUpdate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	testingCache := NewTestingCache()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	crdClient := GetTestCrdClientset()
	snapClient := GetTestSnapshotClientset()
	addCrdTestReactors(crdClient, testingCache)

	// Test the old relationship has no status conditions
	controller, _ := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	newRelationship := &netappv1.TridentMirrorRelationship{}

	controller.updateMirrorRelationship(newRelationship, newRelationship)

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

	controller.updateMirrorRelationship(oldRelationship, newRelationship)

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

	controller.updateMirrorRelationship(oldRelationship, newRelationship)

	if controller.workqueue.Len() != 1 {
		t.Fatalf("Expected an update to be required")
	}
}

func TestUpdateTMRConditionLocalFields(t *testing.T) {
	localVolumeHandle := "ontap_svm:ontap_flexvol"
	localPVCName := "test_pvc"
	remoteVolumeHandle := "ontap_svm2:ontap_flexvol"
	expectedStatusCondition := &netappv1.TridentMirrorRelationshipCondition{
		LocalVolumeHandle:  localVolumeHandle,
		LocalPVCName:       localPVCName,
		RemoteVolumeHandle: remoteVolumeHandle,
	}
	statusCondition := &netappv1.TridentMirrorRelationshipCondition{}
	newStatusCondition, err := updateTMRConditionLocalFields(
		statusCondition, localVolumeHandle, localPVCName, remoteVolumeHandle)
	if err != nil {
		t.Errorf("Got error updating TMR condition")
	}
	if !reflect.DeepEqual(newStatusCondition, expectedStatusCondition) {
		t.Fatalf("Actual does not equal expected, actual: %v, expected: %v",
			newStatusCondition, expectedStatusCondition)
	}

	// Test no flexvol or svm name
	localVolumeHandle = ""
	localPVCName = "test_pvc"
	expectedStatusCondition = &netappv1.TridentMirrorRelationshipCondition{
		LocalVolumeHandle:  localVolumeHandle,
		LocalPVCName:       localPVCName,
		RemoteVolumeHandle: remoteVolumeHandle,
	}
	statusCondition = &netappv1.TridentMirrorRelationshipCondition{}
	newStatusCondition, err = updateTMRConditionLocalFields(
		statusCondition, localVolumeHandle, localPVCName, remoteVolumeHandle)

	if err != nil {
		t.Errorf("Got error updating TMR condition")
	}
	if !reflect.DeepEqual(newStatusCondition, expectedStatusCondition) {
		t.Fatalf("Actual does not equal expected, actual: %v, expected: %v",
			newStatusCondition, expectedStatusCondition)
	}
}
