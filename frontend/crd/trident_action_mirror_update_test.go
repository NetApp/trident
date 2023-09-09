// Copyright 2023 NetApp, Inc. All Rights Reserved.

package crd

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/config"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

const (
	localVolHandle       = "vs2:vs2_pvc_456"
	remoteVolHandle      = "vs1:vs1_pvc_123"
	replicationPolicy    = "MirrorAllSnapshots"
	replicationSchedule  = "1min"
	tmrName1             = "tmr1"
	pvc1                 = "pvc1"
	pv1                  = "pvc-456"
	snapName1            = "snapshot-123"
	snapHandle1          = "pvc-1/snapshot-123"
	namespace1           = "namespace1"
	tamu1                = "tamu1"
	previousTransferTime = "2023-05-22T19:56:33Z"
)

func fakePVC(pvcName, pvcNamespace, pvName string) *v1.PersistentVolumeClaim {
	return &v1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "PersistentVolumeClaim",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: pvcNamespace,
			Name:      pvcName,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: pvName,
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
		},
	}
}

func fakeTMR(tmrName, tmrNamespace, pvcName string) *netappv1.TridentMirrorRelationship {
	return &netappv1.TridentMirrorRelationship{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TridentMirrorRelationship",
			APIVersion: "trident.netapp.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      tmrName,
			Namespace: tmrNamespace,
		},
		Spec: netappv1.TridentMirrorRelationshipSpec{
			MirrorState:         "established",
			ReplicationPolicy:   replicationPolicy,
			ReplicationSchedule: replicationSchedule,
			VolumeMappings: []*netappv1.TridentMirrorRelationshipVolumeMapping{
				{
					RemoteVolumeHandle: remoteVolHandle,
					LocalPVCName:       pvcName,
				},
			},
		},
		Status: netappv1.TridentMirrorRelationshipStatus{Conditions: []*netappv1.TridentMirrorRelationshipCondition{
			{
				MirrorState:         "established",
				Message:             "",
				ObservedGeneration:  2,
				LocalVolumeHandle:   localVolHandle,
				LocalPVCName:        pvcName,
				RemoteVolumeHandle:  remoteVolHandle,
				ReplicationPolicy:   replicationPolicy,
				ReplicationSchedule: replicationSchedule,
			},
		}},
	}
}

func fakeTAMU(name, namespace, tmrName, snapshotHandle string) *netappv1.TridentActionMirrorUpdate {
	return &netappv1.TridentActionMirrorUpdate{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TridentActionMirrorUpdate",
			APIVersion: "trident.netapp.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: netappv1.TridentActionMirrorUpdateSpec{
			TMRName:        tmrName,
			SnapshotHandle: snapshotHandle,
		},
	}
}

func TestHandleActionMirrorUpdate(t *testing.T) {
	defer func() { config.DisableExtraFeatures = false }()
	config.DisableExtraFeatures = false

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

	orchestrator.EXPECT().GetMirrorTransferTime(gomock.Any(), pv1).Return(nil, nil).Times(1)
	orchestrator.EXPECT().UpdateMirror(gomock.Any(), pv1, snapName1).Return(nil).Times(1)

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating: %v", err.Error())
	}
	delaySeconds(1)

	pvc := fakePVC(pvc1, namespace1, pv1)
	_, _ = kubeClient.CoreV1().PersistentVolumeClaims(namespace1).Create(ctx(), pvc, createOpts)

	tmr := fakeTMR(tmrName1, namespace1, pvc1)
	_, _ = crdClient.TridentV1().TridentMirrorRelationships(namespace1).Create(ctx(), tmr, createOpts)

	tamu := fakeTAMU(tamu1, namespace1, tmrName1, snapHandle1)
	_, _ = crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Create(ctx(), tamu, createOpts)

	// Wait until the operation completes
	for i := 0; i < 5; i++ {
		time.Sleep(250 * time.Millisecond)

		tamu, err = crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Get(ctx(), tamu1, getOpts)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			break
		} else if tamu.IsComplete() {
			break
		}
	}

	assert.True(t, tamu.Succeeded(), "TAMU operation failed")

	// Now that the main path has been validated, use the existing state to test some error paths
	_, err = crdController.validateActionMirrorUpdateCR(ctx(), namespace1, tamu1)

	assert.Error(t, err, "Completed TAMU should not have been accepted")

	_ = crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Delete(ctx(), tamu1, deleteOptions)

	// Wait until the TASR disappears
	for i := 0; i < 5; i++ {
		tamu, err = crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Get(ctx(), tamu1, getOpts)
		if apierrors.IsNotFound(err) {
			break
		}

		time.Sleep(250 * time.Millisecond)
	}

	tamu, err = crdController.validateActionMirrorUpdateCR(ctx(), namespace1, tamu1)

	assert.True(t, apierrors.IsNotFound(err), "TAMU should not have been found")
}

func TestHandleActionMirrorUpdate_WrongAction(t *testing.T) {
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

	keyItem := &KeyItem{
		key:        "key",
		event:      EventUpdate,
		ctx:        ctx(),
		objectType: ObjectTypeTridentActionMirrorUpdate,
	}

	err = crdController.handleActionMirrorUpdate(keyItem)

	assert.NoError(t, err, "update KeyItem event should be ignored")

	keyItem.event = EventDelete

	err = crdController.handleActionMirrorUpdate(keyItem)

	assert.NoError(t, err, "delete KeyItem event should be ignored")
}

func TestHandleActionMirrorUpdate_InvalidKey(t *testing.T) {
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

	keyItem := &KeyItem{
		key:        "cluster/namespace/name",
		event:      EventAdd,
		ctx:        ctx(),
		objectType: ObjectTypeTridentActionMirrorUpdate,
	}

	err = crdController.handleActionMirrorUpdate(keyItem)

	assert.Error(t, err, "invalid KeyItem key did not fail")
}

func TestHandleActionMirrorUpdate_ValidateFailure(t *testing.T) {
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

	keyItem := &KeyItem{
		key:        "trident/tasr1",
		event:      EventAdd,
		ctx:        ctx(),
		objectType: ObjectTypeTridentActionMirrorUpdate,
	}

	err = crdController.handleActionMirrorUpdate(keyItem)

	assert.Error(t, err, "validation did not fail")
}

func TestHandleActionMirrorUpdate_InProgress(t *testing.T) {
	defer func() { config.DisableExtraFeatures = false }()
	config.DisableExtraFeatures = false

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

	// Fail in the driver a couple times with InProgressError, expecting a retry
	orchestrator.EXPECT().UpdateMirror(gomock.Any(), pv1, snapName1).Return(
		errors.InProgressError("failed")).Times(1)

	orchestrator.EXPECT().CheckMirrorTransferState(gomock.Any(), pv1).Return(nil,
		errors.InProgressError("transferring")).Times(2)

	transferTime, _ := time.Parse(utils.TimestampFormat, previousTransferTime)
	orchestrator.EXPECT().GetMirrorTransferTime(gomock.Any(), pv1).Return(&transferTime, nil).Times(1)

	// Succeed on the third call to the driver
	endTransferTime := time.Now()
	endTransferTime = endTransferTime.Add(10 * time.Second)
	// formattedTransferTime := endTransferTime.Format(transferFormat)
	orchestrator.EXPECT().CheckMirrorTransferState(gomock.Any(), pv1).Return(&endTransferTime, nil).Times(1)

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating: %v", err.Error())
	}
	delaySeconds(1)

	pvc := fakePVC(pvc1, namespace1, pv1)
	_, _ = kubeClient.CoreV1().PersistentVolumeClaims(namespace1).Create(ctx(), pvc, createOpts)

	tmr := fakeTMR(tmrName1, namespace1, pvc1)
	_, _ = crdClient.TridentV1().TridentMirrorRelationships(namespace1).Create(ctx(), tmr, createOpts)

	tamu := fakeTAMU(tamu1, namespace1, tmrName1, snapHandle1)
	_, _ = crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Create(ctx(), tamu, createOpts)

	// Wait until the operation completes
	for i := 0; i < 5; i++ {
		time.Sleep(250 * time.Millisecond)

		tamu, err = crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Get(ctx(), tamu1, getOpts)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			break
		} else if tamu.IsComplete() {
			break
		}
	}

	assert.True(t, endTransferTime.After(tamu.Status.PreviousTransferTime.Time), "End transfer time is not after start")
	assert.True(t, tamu.Succeeded(), "TAMU operation failed")
}

func TestHandleActionMirrorUpdate_InProgress_Disabled(t *testing.T) {
	defer func() { config.DisableExtraFeatures = false }()
	config.DisableExtraFeatures = true

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

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating: %v", err.Error())
	}
	delaySeconds(1)

	pvc := fakePVC(pvc1, namespace1, pv1)
	_, _ = kubeClient.CoreV1().PersistentVolumeClaims(namespace1).Create(ctx(), pvc, createOpts)

	tmr := fakeTMR(tmrName1, namespace1, pvc1)
	_, _ = crdClient.TridentV1().TridentMirrorRelationships(namespace1).Create(ctx(), tmr, createOpts)

	tamu := fakeTAMU(tamu1, namespace1, tmrName1, snapHandle1)
	_, _ = crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Create(ctx(), tamu, createOpts)

	// Wait until the operation completes
	for i := 0; i < 5; i++ {
		time.Sleep(250 * time.Millisecond)

		tamu, err = crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Get(ctx(), tamu1, getOpts)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			break
		} else if tamu.IsComplete() {
			break
		}
	}

	assert.True(t, tamu.Failed(), "TAMU operation was not disabled")
}

func TestHandleActionMirrorUpdate_InProgressAtStartup(t *testing.T) {
	defer func() { config.DisableExtraFeatures = false }()
	config.DisableExtraFeatures = false

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

	tamu := fakeTAMU(tamu1, namespace1, tmrName1, snapHandle1)
	tamu.Status.State = netappv1.TridentActionStateInProgress
	_, _ = crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Create(ctx(), tamu, createOpts)

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating: %v", err.Error())
	}
	delaySeconds(1)

	// Wait until the operation completes
	for i := 0; i < 5; i++ {
		time.Sleep(250 * time.Millisecond)

		tamu, err = crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Get(ctx(), tamu1, getOpts)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			break
		} else if tamu.IsComplete() {
			break
		}
	}

	assert.True(t, tamu.Failed(), "TAMU operation did not fail")
}

func TestUpdateActionMirrorUpdateCRInProgress(t *testing.T) {
	defer func() { config.DisableExtraFeatures = false }()
	config.DisableExtraFeatures = false

	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	transferTime, _ := time.Parse(utils.TimestampFormat, previousTransferTime)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	err = crdController.updateActionMirrorUpdateCRInProgress(ctx(), namespace1, tamu1, localVolHandle,
		remoteVolHandle, snapName1, &transferTime)

	assert.True(t, apierrors.IsNotFound(err), "TAMU should not have been found")

	tamu := fakeTAMU(tamu1, namespace1, tmrName1, snapHandle1)
	_, _ = crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Create(ctx(), tamu, createOpts)

	err = crdController.updateActionMirrorUpdateCRInProgress(ctx(), namespace1, tamu1, localVolHandle,
		remoteVolHandle, snapName1, &transferTime)

	assert.NoError(t, err, "TAMU update should have succeeded")

	inProgressTAMU, err := crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Get(ctx(), tamu1, getOpts)

	assert.NoError(t, err, "TAMU get should have succeeded")
	assert.True(t, inProgressTAMU.HasTridentFinalizers())
	assert.Equal(t, netappv1.TridentActionStateInProgress, inProgressTAMU.Status.State, "TAMU not in progress")
	assert.Zero(t, inProgressTAMU.Status.Message, "TAMU should not have error message")
}

func TestUpdateActionMirrorUpdateCRComplete_Succeeded(t *testing.T) {
	defer func() { config.DisableExtraFeatures = false }()
	config.DisableExtraFeatures = false

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

	err = crdController.updateActionMirrorUpdateCRComplete(ctx(), namespace1, tamu1, nil)

	assert.NoError(t, err, "TAMU should not have been found")

	tamu := fakeTAMU(tamu1, namespace1, tmrName1, snapHandle1)
	tamu.Status.State = netappv1.TridentActionStateInProgress
	_, _ = crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Create(ctx(), tamu, createOpts)

	err = crdController.updateActionMirrorUpdateCRComplete(ctx(), namespace1, tamu1, nil)

	assert.NoError(t, err, "TAMU update should have succeeded")

	completeTAMU, err := crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Get(ctx(), tamu1, getOpts)

	assert.NoError(t, err, "TAMU get should have succeeded")
	assert.False(t, completeTAMU.HasTridentFinalizers())
	assert.Equal(t, netappv1.TridentActionStateSucceeded, completeTAMU.Status.State, "TAMU not in succeeded state")
	assert.Zero(t, completeTAMU.Status.Message, "TAMU should not have error message")
}

func TestUpdateActionMirrorUpdateCRComplete_Failed(t *testing.T) {
	defer func() { config.DisableExtraFeatures = false }()
	config.DisableExtraFeatures = false

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

	err = crdController.updateActionMirrorUpdateCRComplete(ctx(), namespace1, tamu1, nil)

	assert.NoError(t, err, "TAMU should not have been found")

	tamu := fakeTAMU(tamu1, namespace1, tmrName1, snapHandle1)
	tamu.Status.State = netappv1.TridentActionStateInProgress
	_, _ = crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Create(ctx(), tamu, createOpts)

	err = crdController.updateActionMirrorUpdateCRComplete(ctx(), namespace1, tamu1, fmt.Errorf("failed"))

	assert.NoError(t, err, "TAMU update should have succeeded")

	completeTAMU, err := crdClient.TridentV1().TridentActionMirrorUpdates(namespace1).Get(ctx(), tamu1, getOpts)

	assert.NoError(t, err, "TAMU get should have succeeded")
	assert.False(t, completeTAMU.HasTridentFinalizers())
	assert.Equal(t, netappv1.TridentActionStateFailed, completeTAMU.Status.State, "TAMU should be in failed state")
	assert.Equal(t, "failed", completeTAMU.Status.Message, "TAMU should have error message")
}

func TestGetTMR(t *testing.T) {
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
	tmr := fakeTMR(tmrName1, namespace1, pvc1)
	_, _ = crdClient.TridentV1().TridentMirrorRelationships(namespace1).Create(ctx(), tmr, createOpts)
	tamu := fakeTAMU(tamu1, namespace1, tmrName1, snapHandle1)

	k8sTMR, err := crdController.getTMR(ctx(), tamu)
	assert.Equal(t, tmr.Name, k8sTMR.Name, "TMR should have been found")
}

func TestGetTMR_NotFound(t *testing.T) {
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

	tamu := fakeTAMU(tamu1, namespace1, tmrName1, snapHandle1)

	_, err = crdController.getTMR(ctx(), tamu)
	assert.Error(t, err, "TMR should not have been found")
}
