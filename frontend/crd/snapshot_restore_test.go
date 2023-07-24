// Copyright 2023 NetApp, Inc. All Rights Reserved.

package crd

import (
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
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
	snapRestorePVC1        = "pvc1"
	snapRestorePV1         = "pv1"
	snapRestoreSnap1       = "snap1"
	snapRestoreSnap2       = "snap2"
	snapRestoreSnap3       = "snap3"
	snapRestoreTsnap1      = "snapshot1"
	snapRestoreTsnap3      = "snapshot3"
	snapRestoreVSC1        = "snapcontent1"
	snapRestoreVSC2        = "snapcontent2"
	snapRestoreVSC3        = "snapcontent3"
	snapRestoreSnapHandle1 = "pv1/snapshot1"
	snapRestoreSnapHandle2 = "pv1/snapshot2"
	snapRestoreSnapHandle3 = "pv1/snapshot3"
	tasr1                  = "tasr1"
)

func fakeSnapRestorePVC(pvcName, pvcNamespace, pvName string) *v1.PersistentVolumeClaim {
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

func fakePV(pvcName, pvcNamespace, pvName string) *v1.PersistentVolume {
	return &v1.PersistentVolume{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "PersistentVolume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
		Spec: v1.PersistentVolumeSpec{
			ClaimRef: &v1.ObjectReference{
				Namespace: pvcNamespace,
				Name:      pvcName,
			},
		},
		Status: v1.PersistentVolumeStatus{
			Phase: v1.VolumeBound,
		},
	}
}

func fakeVS(vsName, vsNamespace, vscName, pvcName string, creationTime time.Time) *snapshotv1.VolumeSnapshot {
	return &snapshotv1.VolumeSnapshot{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snapshot.storage.k8s.io/v1",
			Kind:       "VolumeSnapshot",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      vsName,
			Namespace: vsNamespace,
		},
		Spec: snapshotv1.VolumeSnapshotSpec{
			Source: snapshotv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: utils.Ptr(pvcName),
			},
		},
		Status: &snapshotv1.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: &vscName,
			CreationTime:                   utils.Ptr(metav1.NewTime(creationTime)),
			ReadyToUse:                     utils.Ptr(true),
		},
	}
}

func fakeVSC(vsName, vsNamespace, vscName, vsHandle string, creationTime time.Time) *snapshotv1.VolumeSnapshotContent {
	return &snapshotv1.VolumeSnapshotContent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snapshot.storage.k8s.io/v1",
			Kind:       "VolumeSnapshotContent",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: vscName,
		},
		Spec: snapshotv1.VolumeSnapshotContentSpec{
			VolumeSnapshotRef: v1.ObjectReference{
				Namespace: vsNamespace,
				Name:      vsName,
			},
		},
		Status: &snapshotv1.VolumeSnapshotContentStatus{
			SnapshotHandle: utils.Ptr(vsHandle),
			CreationTime:   utils.Ptr(creationTime.UnixNano()),
			ReadyToUse:     utils.Ptr(true),
		},
	}
}

func fakeTASR(name, namespace, pvcName, vsName string) *netappv1.TridentActionSnapshotRestore {
	return &netappv1.TridentActionSnapshotRestore{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "snapshot.storage.k8s.io/v1",
			Kind:       "VolumeSnapshotContent",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: netappv1.TridentActionSnapshotRestoreSpec{
			PVCName:            pvcName,
			VolumeSnapshotName: vsName,
		},
	}
}

func TestHandleActionSnapshotRestore(t *testing.T) {
	defer func() { config.DisableExtraFeatures = true }()
	config.DisableExtraFeatures = false

	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}

	orchestrator.EXPECT().RestoreSnapshot(gomock.Any(), snapRestorePV1, snapRestoreTsnap3).Return(nil).Times(1)

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating; %v", err)
	}
	time.Sleep(250 * time.Millisecond)

	pvc := fakeSnapRestorePVC(snapRestorePVC1, namespace1, snapRestorePV1)
	_, _ = kubeClient.CoreV1().PersistentVolumeClaims(namespace1).Create(ctx(), pvc, createOpts)

	pv := fakePV(snapRestorePVC1, namespace1, snapRestorePV1)
	_, _ = kubeClient.CoreV1().PersistentVolumes().Create(ctx(), pv, createOpts)

	vs1Time := time.Now()
	vs2Time := vs1Time.Add(1 * time.Second)
	vs3Time := vs2Time.Add(1 * time.Second)

	vs1 := fakeVS(snapRestoreSnap1, namespace1, snapRestoreVSC1, snapRestorePVC1, vs1Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Create(ctx(), vs1, createOpts)

	vsc1 := fakeVSC(snapRestoreSnap1, namespace1, snapRestoreVSC1, snapRestoreSnapHandle1, vs1Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx(), vsc1, createOpts)

	vs2 := fakeVS(snapRestoreSnap2, namespace1, snapRestoreVSC2, snapRestorePVC1, vs2Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Create(ctx(), vs2, createOpts)

	vsc2 := fakeVSC(snapRestoreSnap2, namespace1, snapRestoreVSC2, snapRestoreSnapHandle2, vs2Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx(), vsc2, createOpts)

	vs3 := fakeVS(snapRestoreSnap3, namespace1, snapRestoreVSC3, snapRestorePVC1, vs3Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Create(ctx(), vs3, createOpts)

	vsc3 := fakeVSC(snapRestoreSnap3, namespace1, snapRestoreVSC3, snapRestoreSnapHandle3, vs3Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx(), vsc3, createOpts)

	tasr := fakeTASR(tasr1, namespace1, snapRestorePVC1, snapRestoreSnap3)
	_, _ = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Create(ctx(), tasr, createOpts)

	// Wait until the operation completes
	for i := 0; i < 20; i++ {
		time.Sleep(250 * time.Millisecond)

		tasr, err = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Get(ctx(), tasr1, getOpts)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			break
		} else if tasr.IsComplete() {
			break
		}
	}

	assert.Nil(t, err, "err should be nil")
	assert.True(t, tasr.Succeeded(), "TASR operation failed")
	assert.NotZero(t, tasr.Status.StartTime.Time, "Start time should not be zero")
	assert.NotZero(t, tasr.Status.CompletionTime.Time, "Completion time should not be zero")
	assert.False(t, tasr.Status.CompletionTime.Time.Before(tasr.Status.StartTime.Time),
		"Completion time is before start time")

	// Now that the main path has been validated, use the existing state to test some error paths

	_, err = crdController.validateActionSnapshotRestoreCR(ctx(), namespace1, tasr1)

	assert.Error(t, err, "Completed TASR should not have been accepted")

	_ = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Delete(ctx(), tasr1, deleteOptions)

	// Wait until the TASR disappears
	for i := 0; i < 20; i++ {
		tasr, err = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Get(ctx(), tasr1, getOpts)
		if apierrors.IsNotFound(err) {
			break
		}

		time.Sleep(250 * time.Millisecond)
	}

	tasr, err = crdController.validateActionSnapshotRestoreCR(ctx(), namespace1, tasr1)

	assert.True(t, apierrors.IsNotFound(err), "TASR should not have been found")
}

func TestHandleActionSnapshotRestore_Disabled(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating; %v", err)
	}
	time.Sleep(250 * time.Millisecond)

	pvc := fakeSnapRestorePVC(snapRestorePVC1, namespace1, snapRestorePV1)
	_, _ = kubeClient.CoreV1().PersistentVolumeClaims(namespace1).Create(ctx(), pvc, createOpts)

	pv := fakePV(snapRestorePVC1, namespace1, snapRestorePV1)
	_, _ = kubeClient.CoreV1().PersistentVolumes().Create(ctx(), pv, createOpts)

	vs1Time := time.Now()
	vs2Time := vs1Time.Add(1 * time.Second)
	vs3Time := vs2Time.Add(1 * time.Second)

	vs1 := fakeVS(snapRestoreSnap1, namespace1, snapRestoreVSC1, snapRestorePVC1, vs1Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Create(ctx(), vs1, createOpts)

	vsc1 := fakeVSC(snapRestoreSnap1, namespace1, snapRestoreVSC1, snapRestoreSnapHandle1, vs1Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx(), vsc1, createOpts)

	vs2 := fakeVS(snapRestoreSnap2, namespace1, snapRestoreVSC2, snapRestorePVC1, vs2Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Create(ctx(), vs2, createOpts)

	vsc2 := fakeVSC(snapRestoreSnap2, namespace1, snapRestoreVSC2, snapRestoreSnapHandle2, vs2Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx(), vsc2, createOpts)

	vs3 := fakeVS(snapRestoreSnap3, namespace1, snapRestoreVSC3, snapRestorePVC1, vs3Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Create(ctx(), vs3, createOpts)

	vsc3 := fakeVSC(snapRestoreSnap3, namespace1, snapRestoreVSC3, snapRestoreSnapHandle3, vs3Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx(), vsc3, createOpts)

	tasr := fakeTASR(tasr1, namespace1, snapRestorePVC1, snapRestoreSnap3)
	_, _ = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Create(ctx(), tasr, createOpts)

	// Wait until the operation completes
	for i := 0; i < 20; i++ {
		time.Sleep(250 * time.Millisecond)

		tasr, err = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Get(ctx(), tasr1, getOpts)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			break
		} else if tasr.IsComplete() {
			break
		}
	}

	assert.True(t, tasr.Failed(), "TASR operation did not fail")
}

func TestHandleActionSnapshotRestore_InProgressError(t *testing.T) {
	defer func() { config.DisableExtraFeatures = true }()
	config.DisableExtraFeatures = false

	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}

	// Fail in the driver a couple times with InProgressError, expecting a retry
	orchestrator.EXPECT().RestoreSnapshot(gomock.Any(), snapRestorePV1, snapRestoreTsnap3).
		Return(errors.InProgressError("failed")).Times(2)

	// Succeed on the third call to the driver
	orchestrator.EXPECT().RestoreSnapshot(gomock.Any(), snapRestorePV1, snapRestoreTsnap3).Return(nil).Times(1)

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating; %v", err)
	}
	time.Sleep(250 * time.Millisecond)

	pvc := fakeSnapRestorePVC(snapRestorePVC1, namespace1, snapRestorePV1)
	_, _ = kubeClient.CoreV1().PersistentVolumeClaims(namespace1).Create(ctx(), pvc, createOpts)

	pv := fakePV(snapRestorePVC1, namespace1, snapRestorePV1)
	_, _ = kubeClient.CoreV1().PersistentVolumes().Create(ctx(), pv, createOpts)

	vs1Time := time.Now()
	vs2Time := vs1Time.Add(1 * time.Second)
	vs3Time := vs2Time.Add(1 * time.Second)

	vs1 := fakeVS(snapRestoreSnap1, namespace1, snapRestoreVSC1, snapRestorePVC1, vs1Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Create(ctx(), vs1, createOpts)

	vsc1 := fakeVSC(snapRestoreSnap1, namespace1, snapRestoreVSC1, snapRestoreSnapHandle1, vs1Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx(), vsc1, createOpts)

	vs2 := fakeVS(snapRestoreSnap2, namespace1, snapRestoreVSC2, snapRestorePVC1, vs2Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Create(ctx(), vs2, createOpts)

	vsc2 := fakeVSC(snapRestoreSnap2, namespace1, snapRestoreVSC2, snapRestoreSnapHandle2, vs2Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx(), vsc2, createOpts)

	vs3 := fakeVS(snapRestoreSnap3, namespace1, snapRestoreVSC3, snapRestorePVC1, vs3Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Create(ctx(), vs3, createOpts)

	vsc3 := fakeVSC(snapRestoreSnap3, namespace1, snapRestoreVSC3, snapRestoreSnapHandle3, vs3Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx(), vsc3, createOpts)

	tasr := fakeTASR(tasr1, namespace1, snapRestorePVC1, snapRestoreSnap3)
	_, _ = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Create(ctx(), tasr, createOpts)

	// Wait until the operation completes
	for i := 0; i < 20; i++ {
		time.Sleep(250 * time.Millisecond)

		tasr, err = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Get(ctx(), tasr1, getOpts)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			break
		} else if tasr.IsComplete() {
			break
		}
	}

	assert.True(t, tasr.Succeeded(), "TASR operation failed")
}

func TestHandleActionSnapshotRestore_InProgressAtStartup(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}

	tasr := fakeTASR(tasr1, namespace1, snapRestorePVC1, snapRestoreSnap3)
	tasr.Status.State = netappv1.TridentActionStateInProgress
	_, _ = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Create(ctx(), tasr, createOpts)

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating; %v", err)
	}

	// Wait until the operation completes
	for i := 0; i < 20; i++ {
		time.Sleep(250 * time.Millisecond)

		tasr, err = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Get(ctx(), tasr1, getOpts)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			break
		} else if tasr.IsComplete() {
			break
		}
	}

	assert.True(t, tasr.Failed(), "TASR operation did not fail")
}

func TestHandleActionSnapshotRestore_WrongAction(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}

	keyItem := &KeyItem{
		key:        "key",
		event:      EventUpdate,
		ctx:        ctx(),
		objectType: ObjectTypeTridentActionSnapshotRestore,
	}

	err = crdController.handleActionSnapshotRestore(keyItem)

	assert.NoError(t, err, "update KeyItem event should be ignored")

	keyItem.event = EventDelete

	err = crdController.handleActionSnapshotRestore(keyItem)

	assert.NoError(t, err, "delete KeyItem event should be ignored")
}

func TestHandleActionSnapshotRestore_InvalidKey(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}

	keyItem := &KeyItem{
		key:        "cluster/namespace/name",
		event:      EventAdd,
		ctx:        ctx(),
		objectType: ObjectTypeTridentActionSnapshotRestore,
	}

	err = crdController.handleActionSnapshotRestore(keyItem)

	assert.Error(t, err, "invalid KeyItem key did not fail")
}

func TestHandleActionSnapshotRestore_ValidateFailure(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}

	keyItem := &KeyItem{
		key:        "trident/tasr1",
		event:      EventAdd,
		ctx:        ctx(),
		objectType: ObjectTypeTridentActionSnapshotRestore,
	}

	err = crdController.handleActionSnapshotRestore(keyItem)

	assert.Error(t, err, "validation did not fail")
}

func TestHandleActionSnapshotRestore_NotNewest(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating; %v", err)
	}
	time.Sleep(250 * time.Millisecond)

	pvc := fakeSnapRestorePVC(snapRestorePVC1, namespace1, snapRestorePV1)
	_, _ = kubeClient.CoreV1().PersistentVolumeClaims(namespace1).Create(ctx(), pvc, createOpts)

	pv := fakePV(snapRestorePVC1, namespace1, snapRestorePV1)
	_, _ = kubeClient.CoreV1().PersistentVolumes().Create(ctx(), pv, createOpts)

	vs1Time := time.Now()
	vs2Time := vs1Time.Add(1 * time.Second)
	vs3Time := vs2Time.Add(1 * time.Second)

	vs1 := fakeVS(snapRestoreSnap1, namespace1, snapRestoreVSC1, snapRestorePVC1, vs1Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Create(ctx(), vs1, createOpts)

	vsc1 := fakeVSC(snapRestoreSnap1, namespace1, snapRestoreVSC1, snapRestoreSnapHandle1, vs1Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx(), vsc1, createOpts)

	vs2 := fakeVS(snapRestoreSnap2, namespace1, snapRestoreVSC2, snapRestorePVC1, vs2Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Create(ctx(), vs2, createOpts)

	vsc2 := fakeVSC(snapRestoreSnap2, namespace1, snapRestoreVSC2, snapRestoreSnapHandle2, vs2Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx(), vsc2, createOpts)

	vs3 := fakeVS(snapRestoreSnap3, namespace1, snapRestoreVSC3, snapRestorePVC1, vs3Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Create(ctx(), vs3, createOpts)

	vsc3 := fakeVSC(snapRestoreSnap3, namespace1, snapRestoreVSC3, snapRestoreSnapHandle3, vs3Time)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx(), vsc3, createOpts)

	tasr := fakeTASR(tasr1, namespace1, snapRestorePVC1, snapRestoreSnap2)
	_, _ = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Create(ctx(), tasr, createOpts)

	// Wait until the operation completes
	for i := 0; i < 20; i++ {
		time.Sleep(250 * time.Millisecond)

		tasr, err = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Get(ctx(), tasr1, getOpts)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			break
		} else if tasr.IsComplete() {
			break
		}
	}

	assert.True(t, tasr.Failed(), "TASR operation failed")
}

func TestUpdateActionSnapshotRestoreCRInProgress(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}

	err = crdController.updateActionSnapshotRestoreCRInProgress(ctx(), namespace1, tasr1)

	assert.True(t, apierrors.IsNotFound(err), "TASR should not have been found")

	tasr := fakeTASR(tasr1, namespace1, snapRestorePVC1, snapRestoreSnap1)
	_, _ = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Create(ctx(), tasr, createOpts)

	err = crdController.updateActionSnapshotRestoreCRInProgress(ctx(), namespace1, tasr1)

	assert.NoError(t, err, "TASR update should have succeeded")

	inProgressTASR, err := crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Get(ctx(), tasr1, getOpts)

	assert.NoError(t, err, "TASR get should have succeeded")
	assert.True(t, inProgressTASR.HasTridentFinalizers())
	assert.Equal(t, netappv1.TridentActionStateInProgress, inProgressTASR.Status.State, "TASR not in progress")
	assert.Zero(t, inProgressTASR.Status.Message, "TASR should not have error message")
	assert.NotZero(t, inProgressTASR.Status.StartTime, "Start time should not be zero")
}

func TestUpdateActionSnapshotRestoreCRComplete_Succeeded(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}

	err = crdController.updateActionSnapshotRestoreCRComplete(ctx(), namespace1, tasr1, nil)

	assert.NoError(t, err, "TASR should not have been found")

	tasr := fakeTASR(tasr1, namespace1, snapRestorePVC1, snapRestoreSnap1)
	tasr.Status.State = netappv1.TridentActionStateInProgress
	_, _ = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Create(ctx(), tasr, createOpts)

	err = crdController.updateActionSnapshotRestoreCRComplete(ctx(), namespace1, tasr1, nil)

	assert.NoError(t, err, "TASR update should have succeeded")

	completeTASR, err := crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Get(ctx(), tasr1, getOpts)

	assert.NoError(t, err, "TASR get should have succeeded")
	assert.False(t, completeTASR.HasTridentFinalizers())
	assert.Equal(t, netappv1.TridentActionStateSucceeded, completeTASR.Status.State, "TASR not in succeeded state")
	assert.Zero(t, completeTASR.Status.Message, "TASR should not have error message")
}

func TestUpdateActionSnapshotRestoreCRComplete_Failed(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}

	err = crdController.updateActionSnapshotRestoreCRComplete(ctx(), namespace1, tasr1, nil)

	assert.NoError(t, err, "TASR should not have been found")

	tasr := fakeTASR(tasr1, namespace1, snapRestorePVC1, snapRestoreSnap1)
	tasr.Status.State = netappv1.TridentActionStateInProgress
	_, _ = crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Create(ctx(), tasr, createOpts)

	err = crdController.updateActionSnapshotRestoreCRComplete(ctx(), namespace1, tasr1, fmt.Errorf("failed"))

	assert.NoError(t, err, "TASR update should have succeeded")

	completeTASR, err := crdClient.TridentV1().TridentActionSnapshotRestores(namespace1).Get(ctx(), tasr1, getOpts)

	assert.NoError(t, err, "TASR get should have succeeded")
	assert.False(t, completeTASR.HasTridentFinalizers())
	assert.Equal(t, netappv1.TridentActionStateFailed, completeTASR.Status.State, "TASR not in failed state")
	assert.Equal(t, "failed", completeTASR.Status.Message, "TASR should have error message")
}

func TestGetKubernetesObjectsForActionSnapshotRestore(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend; %v", err)
	}

	tasr := fakeTASR(tasr1, namespace1, snapRestorePVC1, snapRestoreSnap1)

	// Missing PVC
	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.True(t, apierrors.IsNotFound(err), "PVC was found")

	// Non-bound PVC
	pvc := fakeSnapRestorePVC(snapRestorePVC1, namespace1, snapRestorePV1)
	pvc.Status.Phase = v1.ClaimPending
	_, _ = kubeClient.CoreV1().PersistentVolumeClaims(namespace1).Create(ctx(), pvc, createOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// Missing PV
	pvc.Status.Phase = v1.ClaimBound
	_, _ = kubeClient.CoreV1().PersistentVolumeClaims(namespace1).Update(ctx(), pvc, updateOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// Non-bound PV
	pv := fakePV(snapRestorePVC1, namespace1, snapRestorePV1)
	pv.Status.Phase = v1.VolumePending
	pv.Spec.ClaimRef = nil
	_, _ = kubeClient.CoreV1().PersistentVolumes().Create(ctx(), pv, createOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// Missing PV ClaimRef
	pv.Status.Phase = v1.VolumeBound
	_, _ = kubeClient.CoreV1().PersistentVolumes().Update(ctx(), pv, updateOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// Wrong ClaimRef
	pv.Spec.ClaimRef = &v1.ObjectReference{
		Namespace: namespace1,
		Name:      "otherPVC",
	}
	_, _ = kubeClient.CoreV1().PersistentVolumes().Update(ctx(), pv, updateOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// Missing VS
	pv.Spec.ClaimRef = &v1.ObjectReference{
		Namespace: namespace1,
		Name:      snapRestorePVC1,
	}
	_, _ = kubeClient.CoreV1().PersistentVolumes().Update(ctx(), pv, updateOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// Unbound VS
	vsTime := time.Now()

	vs := fakeVS(snapRestoreSnap1, namespace1, snapRestoreVSC1, snapRestorePVC1, vsTime)
	vs.Status.BoundVolumeSnapshotContentName = nil
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Create(ctx(), vs, createOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// No VS snapshot creation time
	vs.Status.BoundVolumeSnapshotContentName = utils.Ptr(snapRestoreVSC1)
	vs.Status.CreationTime = nil
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Update(ctx(), vs, updateOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// Zero VS snapshot creation time
	vs.Status.CreationTime = utils.Ptr(metav1.NewTime(time.Time{}))
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Update(ctx(), vs, updateOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// VS nil ready to use
	vs.Status.CreationTime = utils.Ptr(metav1.NewTime(time.Now()))
	vs.Status.ReadyToUse = nil
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Update(ctx(), vs, updateOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// VS not ready to use
	vs.Status.ReadyToUse = utils.Ptr(false)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Update(ctx(), vs, updateOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// Missing VSC
	vs.Status.ReadyToUse = utils.Ptr(true)
	_, _ = snapClient.SnapshotV1().VolumeSnapshots(namespace1).Update(ctx(), vs, updateOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// Wrong VSC VolumeSnapshotRef name
	vsc := fakeVSC(snapRestoreSnap1, namespace1, snapRestoreVSC1, snapRestoreSnapHandle1, vsTime)
	vsc.Spec.VolumeSnapshotRef.Name = snapRestoreSnap2
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx(), vsc, createOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// VSC nil ready to use
	vsc.Spec.VolumeSnapshotRef.Name = snapRestoreSnap1
	vsc.Status.ReadyToUse = nil
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Update(ctx(), vsc, updateOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// VSC not ready to use
	vsc.Status.ReadyToUse = utils.Ptr(false)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Update(ctx(), vsc, updateOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// VSC nil snapshot handle
	vsc.Status.ReadyToUse = utils.Ptr(true)
	vsc.Status.SnapshotHandle = nil
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Update(ctx(), vsc, updateOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// VSC invalid snapshot handle
	vsc.Status.SnapshotHandle = utils.Ptr(snapRestoreSnap1)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Update(ctx(), vsc, updateOpts)

	_, _, err = crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.Error(t, err)

	// All OK
	vsc.Status.SnapshotHandle = utils.Ptr(snapRestoreSnapHandle1)
	_, _ = snapClient.SnapshotV1().VolumeSnapshotContents().Update(ctx(), vsc, updateOpts)

	tridentVolume, tridentSnapshot, err := crdController.getKubernetesObjectsForActionSnapshotRestore(ctx(), tasr)

	assert.NoError(t, err, "call should not have failed")
	assert.Equal(t, snapRestorePV1, tridentVolume)
	assert.Equal(t, snapRestoreTsnap1, tridentSnapshot)
}
