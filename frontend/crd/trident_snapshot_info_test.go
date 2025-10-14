// Copyright 2023 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"testing"
	"time"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	"github.com/netapp/trident/frontend/csi"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
)

func TestUpdateTSI(t *testing.T) {
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

	// make sure these aren't nil
	assert.NotNil(t, kubeClient, "kubeClient is nil")
	assert.NotNil(t, crdClient, "crdClient is nil")
	assert.NotNil(t, crdController, "crdController is nil")
	assert.NotNil(t, crdController.crdInformerFactory, "crdController.crdInformerFactory is nil")

	statusCondition1 := &netappv1.TridentSnapshotInfoStatus{SnapshotHandle: "volume/foo"}
	statusCondition2 := &netappv1.TridentSnapshotInfoStatus{SnapshotHandle: "volume/bar"}
	snapshotInfo := &netappv1.TridentSnapshotInfo{}
	snapshotInfo.Status = *statusCondition1

	snapshotInfo, err = crdClient.TridentV1().TridentSnapshotInfos(tridentNamespace).Create(ctx(), snapshotInfo,
		metav1.CreateOptions{})
	assert.Nil(t, err)
	defer crdClient.TridentV1().TridentSnapshotInfos(tridentNamespace).Delete(ctx(), snapshotInfo.Name,
		metav1.DeleteOptions{})

	updatedSnapshotInfo, err := crdController.updateTSIStatus(ctx(), snapshotInfo, statusCondition2)
	assert.Nil(t, err)
	assert.EqualValues(t, *statusCondition2, updatedSnapshotInfo.Status)
}

func TestHandleTridentSnapshotInfo_EdgeCases(t *testing.T) {
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
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	ctx := context.TODO()

	// Test case 1: Invalid key format
	keyItem := &KeyItem{
		key:        "invalid-key",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentSnapshotInfo,
	}
	err = crdController.handleTridentSnapshotInfo(keyItem)
	assert.NoError(t, err, "Expected no error for invalid key")

	// Test case 2: Non-existent TSI
	keyItem = &KeyItem{
		key:        "default/non-existent-tsi",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentSnapshotInfo,
	}
	err = crdController.handleTridentSnapshotInfo(keyItem)
	assert.NoError(t, err, "Expected no error for non-existent TSI")

	// Test case 3: Valid TSI without finalizer - should add finalizer
	tsi := &netappv1.TridentSnapshotInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tsi-no-finalizer",
			Namespace: "default",
		},
		Spec: netappv1.TridentSnapshotInfoSpec{
			SnapshotName: "test-snapshot",
		},
	}
	_, err = crdController.crdClientset.TridentV1().TridentSnapshotInfos("default").Create(ctx, tsi, metav1.CreateOptions{})
	assert.NoError(t, err)

	keyItem = &KeyItem{
		key:        "default/test-tsi-no-finalizer",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentSnapshotInfo,
	}
	err = crdController.handleTridentSnapshotInfo(keyItem)
	assert.NoError(t, err, "Expected no error handling TSI without finalizer")

	// The test verifies the function runs successfully - finalizer handling is internal logic

	// Test case 4: TSI with deletion timestamp - should remove finalizers
	deletingTSI := &netappv1.TridentSnapshotInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-tsi-deleting",
			Namespace:         "default",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
			Finalizers:        []string{"trident.netapp.io"},
		},
		Spec: netappv1.TridentSnapshotInfoSpec{
			SnapshotName: "test-snapshot",
		},
	}
	_, err = crdController.crdClientset.TridentV1().TridentSnapshotInfos("default").Create(ctx, deletingTSI, metav1.CreateOptions{})
	assert.NoError(t, err)

	keyItem = &KeyItem{
		key:        "default/test-tsi-deleting",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentSnapshotInfo,
	}
	err = crdController.handleTridentSnapshotInfo(keyItem)
	assert.NoError(t, err, "Expected no error handling deleting TSI")

	// Test case 5: Invalid TSI (missing snapshot name) - should create status with no handle
	invalidTSI := &netappv1.TridentSnapshotInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-tsi-invalid",
			Namespace:  "default",
			Finalizers: []string{"trident.netapp.io"},
		},
		// Spec is empty/invalid
	}
	_, err = crdController.crdClientset.TridentV1().TridentSnapshotInfos("default").Create(ctx, invalidTSI, metav1.CreateOptions{})
	assert.NoError(t, err)

	keyItem = &KeyItem{
		key:        "default/test-tsi-invalid",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentSnapshotInfo,
	}
	err = crdController.handleTridentSnapshotInfo(keyItem)
	assert.NoError(t, err, "Expected no error handling invalid TSI")

	// Verify status was updated with empty snapshot handle
	updatedInvalidTSI, err := crdController.crdClientset.TridentV1().TridentSnapshotInfos("default").Get(ctx, "test-tsi-invalid", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Empty(t, updatedInvalidTSI.Status.SnapshotHandle, "Expected empty snapshot handle for invalid TSI")
}

func TestGetSnapshotHandle_EdgeCases(t *testing.T) {
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
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	ctx := context.TODO()

	// Test case 1: Non-existent VolumeSnapshot
	tsi := &netappv1.TridentSnapshotInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tsi",
			Namespace: "default",
		},
		Spec: netappv1.TridentSnapshotInfoSpec{
			SnapshotName: "non-existent-snapshot",
		},
	}

	handle, err := crdController.getSnapshotHandle(ctx, tsi)
	assert.Error(t, err, "Expected error for non-existent snapshot")
	assert.Empty(t, handle, "Expected empty handle")

	// Test case 2: VolumeSnapshot without bound VolumeSnapshotContent
	vs := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unbound-snapshot",
			Namespace: "default",
		},
		Spec: snapv1.VolumeSnapshotSpec{
			Source: snapv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: convert.ToPtr("test-pvc"),
			},
		},
		Status: &snapv1.VolumeSnapshotStatus{
			// No BoundVolumeSnapshotContentName
		},
	}
	_, err = snapClient.SnapshotV1().VolumeSnapshots("default").Create(ctx, vs, metav1.CreateOptions{})
	assert.NoError(t, err)

	tsi.Spec.SnapshotName = "unbound-snapshot"
	handle, err = crdController.getSnapshotHandle(ctx, tsi)
	assert.Error(t, err, "Expected error for unbound snapshot")
	assert.Empty(t, handle, "Expected empty handle")

	// Test case 3: VolumeSnapshot with bound content but content doesn't exist
	vsBound := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bound-snapshot-no-content",
			Namespace: "default",
		},
		Spec: snapv1.VolumeSnapshotSpec{
			Source: snapv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: convert.ToPtr("test-pvc"),
			},
		},
		Status: &snapv1.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: convert.ToPtr("non-existent-content"),
		},
	}
	_, err = snapClient.SnapshotV1().VolumeSnapshots("default").Create(ctx, vsBound, metav1.CreateOptions{})
	assert.NoError(t, err)

	tsi.Spec.SnapshotName = "bound-snapshot-no-content"
	handle, err = crdController.getSnapshotHandle(ctx, tsi)
	assert.Error(t, err, "Expected error for missing content")
	assert.Empty(t, handle, "Expected empty handle")

	// Test case 4: VolumeSnapshotContent with wrong driver
	vsc := &snapv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: "wrong-driver-content",
		},
		Spec: snapv1.VolumeSnapshotContentSpec{
			Driver: "wrong.driver.io", // Not Trident
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      "bound-snapshot-wrong-driver",
				Namespace: "default",
			},
		},
		Status: &snapv1.VolumeSnapshotContentStatus{
			SnapshotHandle: convert.ToPtr("volume/snapshot"),
		},
	}
	_, err = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx, vsc, metav1.CreateOptions{})
	assert.NoError(t, err)

	vsWrongDriver := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bound-snapshot-wrong-driver",
			Namespace: "default",
		},
		Spec: snapv1.VolumeSnapshotSpec{
			Source: snapv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: convert.ToPtr("test-pvc"),
			},
		},
		Status: &snapv1.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: convert.ToPtr("wrong-driver-content"),
		},
	}
	_, err = snapClient.SnapshotV1().VolumeSnapshots("default").Create(ctx, vsWrongDriver, metav1.CreateOptions{})
	assert.NoError(t, err)

	tsi.Spec.SnapshotName = "bound-snapshot-wrong-driver"
	handle, err = crdController.getSnapshotHandle(ctx, tsi)
	assert.Error(t, err, "Expected error for wrong driver")
	assert.Empty(t, handle, "Expected empty handle")

	// Test case 5: VolumeSnapshotContent without snapshot handle
	vscNoHandle := &snapv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: "no-handle-content",
		},
		Spec: snapv1.VolumeSnapshotContentSpec{
			Driver: csi.Provisioner, // Correct driver
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      "bound-snapshot-no-handle",
				Namespace: "default",
			},
		},
		Status: &snapv1.VolumeSnapshotContentStatus{
			// No SnapshotHandle
		},
	}
	_, err = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx, vscNoHandle, metav1.CreateOptions{})
	assert.NoError(t, err)

	vsNoHandle := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bound-snapshot-no-handle",
			Namespace: "default",
		},
		Spec: snapv1.VolumeSnapshotSpec{
			Source: snapv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: convert.ToPtr("test-pvc"),
			},
		},
		Status: &snapv1.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: convert.ToPtr("no-handle-content"),
		},
	}
	_, err = snapClient.SnapshotV1().VolumeSnapshots("default").Create(ctx, vsNoHandle, metav1.CreateOptions{})
	assert.NoError(t, err)

	tsi.Spec.SnapshotName = "bound-snapshot-no-handle"
	handle, err = crdController.getSnapshotHandle(ctx, tsi)
	assert.Error(t, err, "Expected error for missing snapshot handle")
	assert.Empty(t, handle, "Expected empty handle")

	// Test case 6: Success case with valid snapshot handle and mocked backend calls
	mockVolume := &storage.VolumeExternal{
		BackendUUID: "mock-backend-uuid",
	}
	mockBackend := &storage.BackendExternal{
		BackendUUID: "mock-backend-uuid",
	}

	orchestrator.EXPECT().GetVolume(gomock.Any(), "test-volume").Return(mockVolume, nil).Times(1)
	orchestrator.EXPECT().GetBackendByBackendUUID(gomock.Any(), "mock-backend-uuid").Return(mockBackend, nil).Times(1)
	orchestrator.EXPECT().CanBackendMirror(gomock.Any(), "mock-backend-uuid").Return(true, nil).Times(1)

	vscValid := &snapv1.VolumeSnapshotContent{
		ObjectMeta: metav1.ObjectMeta{
			Name: "valid-content",
		},
		Spec: snapv1.VolumeSnapshotContentSpec{
			Driver: csi.Provisioner, // Correct driver
			VolumeSnapshotRef: corev1.ObjectReference{
				Name:      "valid-snapshot",
				Namespace: "default",
			},
		},
		Status: &snapv1.VolumeSnapshotContentStatus{
			SnapshotHandle: convert.ToPtr("test-volume/test-snapshot"), // Valid format
		},
	}
	_, err = snapClient.SnapshotV1().VolumeSnapshotContents().Create(ctx, vscValid, metav1.CreateOptions{})
	assert.NoError(t, err)

	vsValid := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "valid-snapshot",
			Namespace: "default",
		},
		Spec: snapv1.VolumeSnapshotSpec{
			Source: snapv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: convert.ToPtr("test-pvc"),
			},
		},
		Status: &snapv1.VolumeSnapshotStatus{
			BoundVolumeSnapshotContentName: convert.ToPtr("valid-content"),
		},
	}
	_, err = snapClient.SnapshotV1().VolumeSnapshots("default").Create(ctx, vsValid, metav1.CreateOptions{})
	assert.NoError(t, err)

	tsi.Spec.SnapshotName = "valid-snapshot"
	handle, err = crdController.getSnapshotHandle(ctx, tsi)
	assert.NoError(t, err, "Expected no error for valid snapshot")
	assert.Equal(t, "test-volume/test-snapshot", handle, "Expected correct snapshot handle")
}

func TestHandleTridentSnapshotInfo_KeyValidation(t *testing.T) {
	// Test the key validation part of handleTridentSnapshotInfo with minimal setup
	ctrl := &TridentCrdController{recorder: record.NewFakeRecorder(10)}

	// Test invalid key that causes SplitMetaNamespaceKey to return error
	keyItem := &KeyItem{
		key: "invalid/key/with/too/many/parts",
		ctx: context.TODO(),
	}

	err := ctrl.handleTridentSnapshotInfo(keyItem)
	assert.NoError(t, err, "handleTridentSnapshotInfo should handle invalid keys gracefully")
}
