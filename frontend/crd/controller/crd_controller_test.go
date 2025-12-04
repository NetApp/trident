// Copyright 2023 NetApp, Inc. All Rights Reserved.
package controller

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	fakesnapshots "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	mockindexers "github.com/netapp/trident/mocks/mock_frontend/crd/controller/indexers"
	persistentstore "github.com/netapp/trident/persistent_store"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage"
	fakeStorage "github.com/netapp/trident/storage/fake"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/fake"
	testutils2 "github.com/netapp/trident/storage_drivers/fake/test_utils"
	"github.com/netapp/trident/utils/errors"
)

var (
	propagationPolicy = metav1.DeletePropagationBackground
	deleteOptions     = metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}
)

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Utility functions

func GetTestKubernetesClientset(objects ...runtime.Object) *k8sfake.Clientset {
	return k8sfake.NewSimpleClientset(objects...)
}

func GetTestSnapshotClientset() *fakesnapshots.Clientset {
	return fakesnapshots.NewSimpleClientset()
}

func GetTestCrdClientset() *Clientset {
	return NewFakeClientset()
}

func delaySeconds(n time.Duration) {
	time.Sleep(n * time.Second)
}

func newFakeStorageDriverConfigJSON(name string) (string, error) {
	volumes := make([]fakeStorage.Volume, 0)
	return fake.NewFakeStorageDriverConfigJSON(name, config.File, testutils2.GenerateFakePools(2), volumes)
}

func TestCRHandlersTableDriven(t *testing.T) {
	// Since the CR handler tests follow identical patterns but with different concrete types,
	// and the interface doesn't expose Generation directly, we'll test each scenario separately
	// for all CRD types to maintain the same coverage

	type crdTestCase struct {
		name               string
		objectType         string
		addFunc            func(controller *TridentCrdController) KeyItem
		updateNoChangeFunc func(controller *TridentCrdController)
		updateNewGenFunc   func(controller *TridentCrdController) KeyItem
		updateDeletedFunc  func(controller *TridentCrdController)
		deleteFunc         func(controller *TridentCrdController) KeyItem
	}

	// Define test cases for each CRD type
	testCases := []crdTestCase{
		{
			name:       "TridentSnapshotInfo",
			objectType: ObjectTypeTridentSnapshotInfo,
			addFunc: func(controller *TridentCrdController) KeyItem {
				tsi := &tridentv1.TridentSnapshotInfo{}
				controller.addCRHandler(tsi)
				workItem, _ := controller.workqueue.Get()
				return workItem.(KeyItem)
			},
			updateNoChangeFunc: func(controller *TridentCrdController) {
				tsi := &tridentv1.TridentSnapshotInfo{}
				tsi.Generation = 1
				tsiNew := &tridentv1.TridentSnapshotInfo{}
				tsiNew.Generation = 1
				controller.updateCRHandler(tsi, tsiNew)
			},
			updateNewGenFunc: func(controller *TridentCrdController) KeyItem {
				tsi := &tridentv1.TridentSnapshotInfo{}
				tsi.Generation = 1
				tsiNew := &tridentv1.TridentSnapshotInfo{}
				tsiNew.Generation = 2
				controller.updateCRHandler(tsi, tsiNew)
				workItem, _ := controller.workqueue.Get()
				return workItem.(KeyItem)
			},
			updateDeletedFunc: func(controller *TridentCrdController) {
				now := time.Now()
				v1Now := metav1.NewTime(now)
				// Test no generation change with deletion
				tsi := &tridentv1.TridentSnapshotInfo{}
				tsi.Generation = 1
				tsiNew := &tridentv1.TridentSnapshotInfo{}
				tsiNew.Generation = 1
				tsiNew.DeletionTimestamp = &v1Now
				controller.updateCRHandler(tsi, tsiNew)
				controller.workqueue.Get() // Clear queue
				// Test generation change with deletion
				tsi.Generation = 0
				tsiNew = &tridentv1.TridentSnapshotInfo{}
				tsiNew.Generation = 1
				tsiNew.DeletionTimestamp = &v1Now
				controller.updateCRHandler(tsi, tsiNew)
				controller.workqueue.Get() // Clear queue
			},
			deleteFunc: func(controller *TridentCrdController) KeyItem {
				tsi := &tridentv1.TridentSnapshotInfo{}
				controller.deleteCRHandler(tsi)
				workItem, _ := controller.workqueue.Get()
				return workItem.(KeyItem)
			},
		},
		{
			name:       "TridentMirrorRelationship",
			objectType: ObjectTypeTridentMirrorRelationship,
			addFunc: func(controller *TridentCrdController) KeyItem {
				tmr := &tridentv1.TridentMirrorRelationship{}
				controller.addCRHandler(tmr)
				workItem, _ := controller.workqueue.Get()
				return workItem.(KeyItem)
			},
			updateNoChangeFunc: func(controller *TridentCrdController) {
				tmr := &tridentv1.TridentMirrorRelationship{}
				tmr.Generation = 1
				tmrNew := &tridentv1.TridentMirrorRelationship{}
				tmrNew.Generation = 1
				controller.updateCRHandler(tmr, tmrNew)
			},
			updateNewGenFunc: func(controller *TridentCrdController) KeyItem {
				tmr := &tridentv1.TridentMirrorRelationship{}
				tmr.Generation = 1
				tmrNew := &tridentv1.TridentMirrorRelationship{}
				tmrNew.Generation = 2
				controller.updateCRHandler(tmr, tmrNew)
				workItem, _ := controller.workqueue.Get()
				return workItem.(KeyItem)
			},
			updateDeletedFunc: func(controller *TridentCrdController) {
				now := time.Now()
				v1Now := metav1.NewTime(now)
				tmr := &tridentv1.TridentMirrorRelationship{}
				tmr.Generation = 1
				tmrNew := &tridentv1.TridentMirrorRelationship{}
				tmrNew.Generation = 1
				tmrNew.DeletionTimestamp = &v1Now
				controller.updateCRHandler(tmr, tmrNew)
				controller.workqueue.Get()
			},
			deleteFunc: func(controller *TridentCrdController) KeyItem {
				tmr := &tridentv1.TridentMirrorRelationship{}
				controller.deleteCRHandler(tmr)
				workItem, _ := controller.workqueue.Get()
				return workItem.(KeyItem)
			},
		},
		{
			name:       "TridentNodeRemediation",
			objectType: ObjectTypeTridentNodeRemediation,
			addFunc: func(controller *TridentCrdController) KeyItem {
				tnr := &tridentv1.TridentNodeRemediation{}
				controller.addCRHandler(tnr)
				workItem, _ := controller.workqueue.Get()
				return workItem.(KeyItem)
			},
			updateNoChangeFunc: func(controller *TridentCrdController) {
				tnr := &tridentv1.TridentNodeRemediation{}
				tnr.Generation = 1
				tnrNew := &tridentv1.TridentNodeRemediation{}
				tnrNew.Generation = 1
				controller.updateCRHandler(tnr, tnrNew)
			},
			updateNewGenFunc: func(controller *TridentCrdController) KeyItem {
				tnr := &tridentv1.TridentNodeRemediation{}
				tnr.Generation = 1
				tnrNew := &tridentv1.TridentNodeRemediation{}
				tnrNew.Generation = 2
				controller.updateCRHandler(tnr, tnrNew)
				workItem, _ := controller.workqueue.Get()
				return workItem.(KeyItem)
			},
			updateDeletedFunc: func(controller *TridentCrdController) {
				now := time.Now()
				v1Now := metav1.NewTime(now)
				tnr := &tridentv1.TridentNodeRemediation{}
				tnr.Generation = 1
				tnrNew := &tridentv1.TridentNodeRemediation{}
				tnrNew.Generation = 1
				tnrNew.DeletionTimestamp = &v1Now
				controller.updateCRHandler(tnr, tnrNew)
				controller.workqueue.Get()
			},
			deleteFunc: func(controller *TridentCrdController) KeyItem {
				tnr := &tridentv1.TridentNodeRemediation{}
				controller.deleteCRHandler(tnr)
				workItem, _ := controller.workqueue.Get()
				return workItem.(KeyItem)
			},
		},
		// Add more types as needed for comprehensive coverage...
	}

	// Test all handler scenarios for each CRD type
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup controller once per CRD type
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

			t.Run("AddCRHandler", func(t *testing.T) {
				keyItem := tc.addFunc(crdController)
				assert.Equal(t, EventAdd, keyItem.event)
				assert.Equal(t, tc.objectType, keyItem.objectType)
			})

			t.Run("UpdateCRHandler_NoChange", func(t *testing.T) {
				tc.updateNoChangeFunc(crdController)
				assert.Equal(t, 0, crdController.workqueue.Len())
			})

			t.Run("UpdateCRHandler_NewGeneration", func(t *testing.T) {
				keyItem := tc.updateNewGenFunc(crdController)
				assert.Equal(t, EventUpdate, keyItem.event)
				assert.Equal(t, tc.objectType, keyItem.objectType)
			})

			t.Run("UpdateCRHandler_CRDeleted", func(t *testing.T) {
				tc.updateDeletedFunc(crdController)
				// Validate that items were added and processed
			})

			t.Run("DeleteCRHandler", func(t *testing.T) {
				keyItem := tc.deleteFunc(crdController)
				assert.Equal(t, EventDelete, keyItem.event)
				assert.Equal(t, tc.objectType, keyItem.objectType)
			})
		})
	}
}

func TestUpdateCRHandler_NewGeneration(t *testing.T) {
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

	// NOTE(ameade): By testing every CRD, we ensure they all meet the TridentCRD interface
	// Test TridentSnapshotInfo
	tsi := &tridentv1.TridentSnapshotInfo{}
	tsi.Generation = 1
	tsiNew := &tridentv1.TridentSnapshotInfo{}
	tsiNew.Generation = 2
	crdController.updateCRHandler(tsi, tsiNew)
	assert.Equal(t, 1, crdController.workqueue.Len())
	workItem, _ := crdController.workqueue.Get()
	keyItem := workItem.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, ObjectTypeTridentSnapshotInfo, keyItem.objectType)

	// Test TridentMirrorRelationship
	tmr := &tridentv1.TridentMirrorRelationship{}
	tmr.Generation = 1
	tmrNew := &tridentv1.TridentMirrorRelationship{}
	tmrNew.Generation = 2
	crdController.updateCRHandler(tmr, tmrNew)
	assert.Equal(t, 1, crdController.workqueue.Len())
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, ObjectTypeTridentMirrorRelationship, keyItem.objectType)

	// Test TridentVolumeReference
	tvr := &tridentv1.TridentVolumeReference{}
	tvr.Generation = 1
	tvrNew := &tridentv1.TridentVolumeReference{}
	tvrNew.Generation = 2
	crdController.updateCRHandler(tvr, tvrNew)
	assert.Equal(t, 1, crdController.workqueue.Len())
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, "TridentVolumeReference", keyItem.objectType)

	// Test TridentVolumeReference
	tvp := &tridentv1.TridentVolumePublication{}
	tvp.Generation = 1
	tvpNew := &tridentv1.TridentVolumePublication{}
	tvpNew.Generation = 2
	crdController.updateCRHandler(tvp, tvpNew)
	assert.Equal(t, 1, crdController.workqueue.Len())
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, "TridentVolumePublication", keyItem.objectType)

	// Test TridentVolume
	tv := &tridentv1.TridentVolume{}
	tv.Generation = 1
	tvNew := &tridentv1.TridentVolume{}
	tvNew.Generation = 2
	crdController.updateCRHandler(tv, tvNew)
	assert.Equal(t, 1, crdController.workqueue.Len())
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, "TridentVolume", keyItem.objectType)

	// Test TridentVersion
	tvers := &tridentv1.TridentVersion{}
	tvers.Generation = 1
	tversNew := &tridentv1.TridentVersion{}
	tversNew.Generation = 2
	crdController.updateCRHandler(tvers, tversNew)
	assert.Equal(t, 1, crdController.workqueue.Len())
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, "TridentVersion", keyItem.objectType)

	// Test TridentTransaction
	tt := &tridentv1.TridentTransaction{}
	tt.Generation = 1
	ttNew := &tridentv1.TridentTransaction{}
	ttNew.Generation = 2
	crdController.updateCRHandler(tt, ttNew)
	assert.Equal(t, 1, crdController.workqueue.Len())
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, "TridentTransaction", keyItem.objectType)

	// Test TridentStorageClass
	tsc := &tridentv1.TridentStorageClass{}
	tsc.Generation = 1
	tscNew := &tridentv1.TridentStorageClass{}
	tscNew.Generation = 2
	crdController.updateCRHandler(tsc, tscNew)
	assert.Equal(t, 1, crdController.workqueue.Len())
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, "TridentStorageClass", keyItem.objectType)

	// Test TridentSnapshot
	ts := &tridentv1.TridentSnapshot{}
	ts.Generation = 1
	tsNew := &tridentv1.TridentSnapshot{}
	tsNew.Generation = 2
	crdController.updateCRHandler(ts, tsNew)
	assert.Equal(t, 1, crdController.workqueue.Len())
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, "TridentSnapshot", keyItem.objectType)

	// Test TridentBackendConfig
	tbc := &tridentv1.TridentBackendConfig{}
	tbc.Generation = 1
	tbcNew := &tridentv1.TridentBackendConfig{}
	tbcNew.Generation = 2
	crdController.updateCRHandler(tbc, tbcNew)
	assert.Equal(t, 1, crdController.workqueue.Len())
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, "TridentBackendConfig", keyItem.objectType)

	// Test TridentBackend
	tb := &tridentv1.TridentBackend{}
	tb.Generation = 1
	tbNew := &tridentv1.TridentBackend{}
	tbNew.Generation = 2
	crdController.updateCRHandler(tb, tbNew)
	assert.Equal(t, 1, crdController.workqueue.Len())
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, "TridentBackend", keyItem.objectType)

	// Test TridentNode
	tn := &tridentv1.TridentNode{}
	tn.Generation = 1
	tnNew := &tridentv1.TridentNode{}
	tnNew.Generation = 2
	crdController.updateCRHandler(tn, tnNew)
	assert.Equal(t, 1, crdController.workqueue.Len())
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, "TridentNode", keyItem.objectType)

	// Test TridentNodeRemediation
	tnr := &tridentv1.TridentNodeRemediation{}
	tnr.Generation = 1
	tnrNew := &tridentv1.TridentNodeRemediation{}
	tnrNew.Generation = 2
	crdController.updateCRHandler(tnr, tnrNew)
	assert.Equal(t, 1, crdController.workqueue.Len())
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, "TridentNodeRemediation", keyItem.objectType)
}

func TestDeleteCRHandler(t *testing.T) {
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

	// Test TridentSnapshotInfo
	tsi := &tridentv1.TridentSnapshotInfo{}
	crdController.deleteCRHandler(tsi)
	workItem, _ := crdController.workqueue.Get()
	keyItem := workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, ObjectTypeTridentSnapshotInfo, keyItem.objectType)

	// Test TridentMirrorRelationship
	tmr := &tridentv1.TridentMirrorRelationship{}
	crdController.deleteCRHandler(tmr)
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, ObjectTypeTridentMirrorRelationship, keyItem.objectType)

	// Test TridentActionMirrorUpdate
	tamu := &tridentv1.TridentActionMirrorUpdate{}
	crdController.deleteCRHandler(tamu)
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, ObjectTypeTridentActionMirrorUpdate, keyItem.objectType)

	// Test TridentNodeRemediation
	tnr := &tridentv1.TridentNodeRemediation{}
	crdController.deleteCRHandler(tnr)
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, ObjectTypeTridentNodeRemediation, keyItem.objectType)

	// Test TridentVolumeReference
	tvr := &tridentv1.TridentVolumeReference{}
	crdController.deleteCRHandler(tvr)
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, "TridentVolumeReference", keyItem.objectType)

	// Test TridentVolumeReference
	tvp := &tridentv1.TridentVolumePublication{}
	crdController.deleteCRHandler(tvp)
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, "TridentVolumePublication", keyItem.objectType)

	// Test TridentVolume
	tv := &tridentv1.TridentVolume{}
	crdController.deleteCRHandler(tv)
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, "TridentVolume", keyItem.objectType)

	// Test TridentVersion
	tvers := &tridentv1.TridentVersion{}
	crdController.deleteCRHandler(tvers)
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, "TridentVersion", keyItem.objectType)

	// Test TridentTransaction
	tt := &tridentv1.TridentTransaction{}
	crdController.deleteCRHandler(tt)
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, "TridentTransaction", keyItem.objectType)

	// Test TridentStorageClass
	tsc := &tridentv1.TridentStorageClass{}
	crdController.deleteCRHandler(tsc)
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, "TridentStorageClass", keyItem.objectType)

	// Test TridentSnapshot
	ts := &tridentv1.TridentSnapshot{}
	crdController.deleteCRHandler(ts)
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, "TridentSnapshot", keyItem.objectType)

	// Test TridentBackendConfig
	tbc := &tridentv1.TridentBackendConfig{}
	crdController.deleteCRHandler(tbc)
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, "TridentBackendConfig", keyItem.objectType)

	// Test TridentBackend
	tb := &tridentv1.TridentBackend{}
	crdController.deleteCRHandler(tb)
	workItem, _ = crdController.workqueue.Get()
	keyItem = workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, "TridentBackend", keyItem.objectType)
}

func TestCrdControllerBackendOperations(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	indexers := mockindexers.NewMockIndexers(mockCtrl)
	indexers.EXPECT().Deactivate()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, indexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// make sure these aren't nil
	assert.NotNil(t, kubeClient, "kubeClient is nil")
	assert.NotNil(t, crdClient, "crdClient is nil")
	assert.NotNil(t, crdController, "crdController is nil")
	assert.NotNil(t, crdController.crdInformerFactory, "crdController.crdInformerFactory is nil")

	expectedVersion := config.DefaultOrchestratorVersion
	if crdController.Version() != expectedVersion {
		t.Fatalf("%v differs:  '%v' != '%v'", "Version()", expectedVersion, crdController.Version())
	}

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating: %v", err.Error())
	}
	delaySeconds(1)

	// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Setup work required for the crdController's logic to work
	// * create a fake backend
	// ** add it to the mock orchestrator
	// ** initialize it
	// * create a CRD version from the fake backend
	fakeConfig := fake.StorageDriver{
		Config: drivers.FakeStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: "fake",
			},
			Protocol: config.File,
		},
	}
	driver := fake.StorageDriver{
		Config: fakeConfig.Config,
	}
	fakeBackend := storage.NewTestStorageBackend()
	fakeBackend.SetDriver(&driver)
	fakeBackend.SetName("fake1")
	fakeBackend.SetBackendUUID(uuid.New().String())

	configJSON, jsonErr := newFakeStorageDriverConfigJSON(fakeBackend.Name())
	if jsonErr != nil {
		t.Fatalf("cannot generate JSON %v", jsonErr.Error())
	}
	commonConfig := fakeConfig.Config.CommonStorageDriverConfig

	if initializeErr := fakeBackend.Driver().Initialize(
		ctx(), "testing", configJSON, commonConfig, nil, uuid.New().String(),
	); initializeErr != nil {
		t.Fatalf("problem initializing storage driver '%s': %v", commonConfig.StorageDriverName, initializeErr)
	}
	fakeBackend.SetOnline(true)
	fakeBackend.SetState(storage.Online)

	// create a k8s CRD Object for use by the client-go bindings and crd persistence layer
	backendCRD, err := tridentv1.NewTridentBackend(ctx(), fakeBackend.ConstructPersistent(ctx()))
	if err != nil {
		t.Fatal("Unable to construct TridentBackend CRD: ", err)
	}
	if backendCRD.BackendName != fakeBackend.Name() {
		t.Fatalf("error creating backend backendCRD.BackendName '%v' != fakeBackend.Name '%v'",
			backendCRD.BackendName, fakeBackend.Name())
	}

	// create a new CRD object through the client-go api
	_, err = crdClient.TridentV1().TridentBackends(tridentNamespace).Create(ctx(), backendCRD, createOpts)
	if err != nil {
		t.Fatalf("error creating backend: %v", err.Error())
	}

	backendList, listErr := crdClient.TridentV1().TridentBackends(tridentNamespace).List(ctx(), listOpts)
	if listErr != nil {
		t.Fatalf("error listing CRD backends: %v", listErr)
	}
	var crdName string
	for _, backend := range backendList.Items {
		Log().WithFields(LogFields{
			"backend.Name":        backend.Name,
			"backend.BackendName": backend.BackendName,
			"backend.BackendUUID": backend.BackendUUID,
		}).Debug("Checking.")
		if backend.BackendName == fakeBackend.Name() {
			Log().WithFields(LogFields{
				"backend.Name":        backend.Name,
				"backend.BackendName": backend.BackendName,
				"backend.BackendUUID": backend.BackendUUID,
			}).Debug("Found.")
			crdName = backend.Name
		}
	}
	if crdName == "" {
		t.Fatalf("error finding CRD with backend.BackendName == '%v' via list", fakeBackend.Name())
	}

	crdByName, getErr := crdClient.TridentV1().TridentBackends(tridentNamespace).Get(ctx(), crdName, getOpts)
	if getErr != nil {
		t.Fatalf("error getting CRD backend '%v' error: %v", crdName, err)
	}
	if crdByName == nil {
		t.Fatalf("error getting CRD backend '%v'", crdName)
	}

	// validate we can detect and remove finalizers
	fmt.Printf(">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>\n")
	if !crdByName.HasTridentFinalizers() {
		t.Fatalf("expected CRD to have finalizers")
	}
	err = crdController.removeBackendFinalizers(ctx(), crdByName)
	if err != nil {
		t.Fatalf("error removing backend finalizers: %v", err)
	}
	// to validate the finalizer removal, we must retrieve it again, after the update
	crdByName, getErr = crdClient.TridentV1().TridentBackends(tridentNamespace).Get(ctx(), crdName, getOpts)
	if getErr != nil {
		t.Fatalf("error getting CRD backend '%v' error: %v", crdName, err)
	}
	if crdByName.HasTridentFinalizers() {
		t.Fatalf("did NOT expect CRD to have finalizers, should've been force removed")
	}
	fmt.Printf("<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<\n")

	// delete backend, make sure it gets removed
	deleteErr := crdClient.TridentV1().TridentBackends(tridentNamespace).Delete(ctx(), crdName, deleteOptions)
	if deleteErr != nil {
		t.Fatalf("error deleting CRD backend '%v': %v", crdName, deleteErr)
	}

	// validate it's gone
	crdByName, getErr = crdClient.TridentV1().TridentBackends(tridentNamespace).Get(ctx(), crdName, getOpts)
	Log().WithFields(LogFields{
		"crdByName": crdByName,
		"getErr":    getErr,
	}).Debug("Checking if backend CRD was deleted.")
	if getErr == nil {
		t.Fatalf("expected the CRD backend '%v' to be deleted", crdName)
	} else if !persistentstore.IsStatusNotFoundError(getErr) {
		t.Fatalf("unexpected error getting CRD backend '%v' error: %v", crdName, getErr)
	}

	// Clean up
	if err = crdController.Deactivate(); err != nil {
		t.Fatalf("error while deactivating: %v", err.Error())
	}
}

func TestCrdControllerFinalizerRemoval(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	indexers := mockindexers.NewMockIndexers(mockCtrl)
	indexers.EXPECT().Deactivate()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, indexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// make sure these aren't nil
	assert.NotNil(t, kubeClient, "kubeClient is nil")
	assert.NotNil(t, crdClient, "crdClient is nil")
	assert.NotNil(t, crdController, "crdController is nil")
	assert.NotNil(t, crdController.crdInformerFactory, "crdController.crdInformerFactory is nil")

	expectedVersion := config.DefaultOrchestratorVersion
	if crdController.Version() != expectedVersion {
		t.Fatalf("%v differs:  '%v' != '%v'", "Version()", expectedVersion, crdController.Version())
	}

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating: %v", err.Error())
	}
	delaySeconds(1)

	// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Setup work required for the crdController's logic to work
	// * create a fake backend
	// ** add it to the mock orchestrator
	// ** initialize it
	// * create a CRD version from the fake backend
	fakeConfig := fake.StorageDriver{
		Config: drivers.FakeStorageDriverConfig{
			CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
				Version:           1,
				StorageDriverName: "fake",
			},
			Protocol: config.File,
		},
	}
	storageDriver := fake.StorageDriver{
		Config: fakeConfig.Config,
	}
	fakeBackend := storage.NewTestStorageBackend()
	fakeBackend.SetDriver(&storageDriver)
	fakeBackend.SetName("fake1")
	fakeBackend.SetBackendUUID(uuid.New().String())

	configJSON, jsonErr := newFakeStorageDriverConfigJSON(fakeBackend.Name())
	if jsonErr != nil {
		t.Fatalf("cannot generate JSON %v", jsonErr.Error())
	}
	commonConfig := fakeConfig.Config.CommonStorageDriverConfig
	if initializeErr := fakeBackend.Driver().Initialize(
		ctx(), "testing", configJSON, commonConfig, nil, uuid.New().String(),
	); initializeErr != nil {
		t.Fatalf("problem initializing storage driver '%s': %v", commonConfig.StorageDriverName, initializeErr)
	}
	fakeBackend.SetOnline(true)
	fakeBackend.SetState(storage.Online)

	// create a k8s CRD Object for use by the client-go bindings and crd persistence layer
	backendCRD, err := tridentv1.NewTridentBackend(ctx(), fakeBackend.ConstructPersistent(ctx()))
	if err != nil {
		t.Fatal("Unable to construct TridentBackend CRD: ", err)
	}
	if backendCRD.BackendName != fakeBackend.Name() {
		t.Fatalf("error creating backend backendCRD.BackendName '%v' != fakeBackend.Name '%v'",
			backendCRD.BackendName, fakeBackend.Name())
	}

	Logc(ctx()).Debug("Creating backend.")

	// create a new Backend CRD object through the client-go api
	backend, err := crdClient.TridentV1().TridentBackends(tridentNamespace).Create(ctx(), backendCRD, createOpts)
	if err != nil {
		t.Fatalf("error creating backend: %v", err.Error())
	}

	Logc(ctx()).Debug("Created backend.")

	// Build a storage.volume
	volConfig := storage.VolumeConfig{
		Version:      config.OrchestratorAPIVersion,
		Name:         "vol1",
		InternalName: "internal_vol1",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}
	vol := &storage.Volume{
		Config:      &volConfig,
		BackendUUID: backendCRD.BackendUUID,
		Pool:        "aggr1",
		State:       storage.VolumeStateOnline,
	}

	// Convert to Kubernetes Object using NewTridentVolume
	volumeCRD, err := tridentv1.NewTridentVolume(ctx(), vol.ConstructExternal())
	if err != nil {
		t.Fatal("Unable to construct TridentVolume CRD: ", err)
	}

	Logc(ctx()).Debug("Creating volume.")

	// create a new Volume CRD object through the client-go api
	_, err = crdClient.TridentV1().TridentVolumes(tridentNamespace).Create(ctx(), volumeCRD, createOpts)
	if err != nil {
		t.Fatalf("error creating volume: %v", err.Error())
	}

	Logc(ctx()).Debug("Created volume.")

	// Build a storage.Snapshot
	testSnapshotConfig := &storage.SnapshotConfig{
		Version:            config.OrchestratorAPIVersion,
		Name:               "snap1",
		InternalName:       "internal_snap1",
		VolumeName:         volConfig.Name,
		VolumeInternalName: volConfig.InternalName,
	}
	now := time.Now().UTC().Format(storage.SnapshotNameFormat)
	size := int64(1000000000)
	snapshot := storage.NewSnapshot(testSnapshotConfig, now, size, storage.SnapshotStateOnline)
	snapshotCRD, err := tridentv1.NewTridentSnapshot(snapshot.ConstructPersistent())
	if err != nil {
		t.Fatal("Unable to construct TridentSnapshot CRD: ", err)
	}

	Logc(ctx()).Debug("Creating snapshot.")

	// create a new Snapshot CRD object through the client-go api
	_, err = crdClient.TridentV1().TridentSnapshots(tridentNamespace).Create(ctx(), snapshotCRD, createOpts)
	if err != nil {
		t.Fatalf("error creating volume: %v", err.Error())
	}

	Logc(ctx()).Debug("Created snapshot.")

	// validate our Volume is present
	volumeList, listErr := crdClient.TridentV1().TridentVolumes(tridentNamespace).List(ctx(), listOpts)
	if listErr != nil {
		t.Fatalf("error listing CRD volumes: %v", listErr.Error())
	}
	if len(volumeList.Items) != 1 {
		t.Fatalf("error while listing volumes, unexpected volume list length: %v", len(volumeList.Items))
	}

	// validate our Snapshot is present
	snapshotList, listErr := crdClient.TridentV1().TridentSnapshots(tridentNamespace).List(ctx(), listOpts)
	if listErr != nil {
		t.Fatalf("error listing CRD snapshots: %v", listErr.Error())
	}
	if len(snapshotList.Items) != 1 {
		t.Fatalf("error while listing snapshots, unexpected snapshot list length: %v", len(snapshotList.Items))
	}

	Logc(ctx()).Debug("Deleting snapshot.")

	// Delete the snapshot
	deleteErr := crdClient.TridentV1().TridentSnapshots(tridentNamespace).Delete(ctx(), "vol1-snap1", deleteOptions)
	if deleteErr != nil {
		t.Fatalf("unable to delete snapshot CRD: %v", deleteErr)
	}

	Logc(ctx()).Debug("Deleted snapshot.")

	snapshotGone := false

	// Wait until the snapshot disappears, which can only happen if the CRD controller removes the finalizers.
	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)

		// Get the latest version of the CRD
		s, getErr := crdClient.TridentV1().TridentSnapshots(tridentNamespace).Get(ctx(), "vol1-snap1", getOpts)

		Log().WithFields(LogFields{
			"snapshot": s,
			"getErr":   getErr,
		}).Debug("Checking if snapshot was deleted.")

		if k8serrors.IsNotFound(getErr) {
			Logc(ctx()).Debug("Snapshot gone.")
			snapshotGone = true
			break
		}
	}

	Logc(ctx()).Debug("Deleting volume.")

	// Delete the snapshot
	deleteErr = crdClient.TridentV1().TridentVolumes(tridentNamespace).Delete(ctx(), "vol1", deleteOptions)
	if deleteErr != nil {
		t.Fatalf("unable to delete volume CRD: %v", deleteErr)
	}

	Logc(ctx()).Debug("Deleted volume.")

	volumeGone := false

	// Wait until the volume disappears, which can only happen if the CRD controller removes the finalizers.
	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)

		// Get the latest version of the CRD
		v, getErr := crdClient.TridentV1().TridentVolumes(tridentNamespace).Get(ctx(), "vol1", getOpts)

		Log().WithFields(LogFields{
			"volume": v,
			"getErr": getErr,
		}).Debug("Checking if volume was deleted.")

		if k8serrors.IsNotFound(getErr) {
			Logc(ctx()).Debug("Volume gone.")
			volumeGone = true
			break
		}
	}

	Logc(ctx()).Debug("Deleting backend.")

	// Delete the snapshot
	deleteErr = crdClient.TridentV1().TridentBackends(tridentNamespace).Delete(ctx(), backend.Name, deleteOptions)
	if deleteErr != nil {
		t.Fatalf("unable to delete backnd CRD: %v", deleteErr)
	}

	Logc(ctx()).Debug("Deleted backend.")

	assert.True(t, snapshotGone, "expected snapshot to be deleted")
	assert.True(t, volumeGone, "expected volume to be deleted")

	// Clean up
	if err = crdController.Deactivate(); err != nil {
		t.Fatalf("error while deactivating: %v", err.Error())
	}
}

func TestCrdControllerTransactionFinalizerRemoval(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	indexers := mockindexers.NewMockIndexers(mockCtrl)
	indexers.EXPECT().Deactivate()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, indexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// make sure these aren't nil
	assert.NotNil(t, kubeClient, "kubeClient is nil")
	assert.NotNil(t, crdClient, "crdClient is nil")
	assert.NotNil(t, crdController, "crdController is nil")
	assert.NotNil(t, crdController.txnInformerFactory, "crdController.txnInformerFactory is nil")

	expectedVersion := config.DefaultOrchestratorVersion
	if crdController.Version() != expectedVersion {
		t.Fatalf("%v differs:  '%v' != '%v'", "Version()", expectedVersion, crdController.Version())
	}

	// Activate the CRD controller and start monitoring
	if err = crdController.Activate(); err != nil {
		t.Fatalf("error while activating: %v", err.Error())
	}
	delaySeconds(1)

	// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Setup work required for the crdController's logic to work
	// * create a fake transaction
	// * create a CRD version from the fake transaction
	// * ensure CRD has Trident finalizer
	// * delete the transaction CRD
	// * ensure the CRD disappears, which can only happen if the controller removes the Trident finalizer

	// Build a volume transaction
	volConfig := storage.VolumeConfig{
		Version:      config.OrchestratorAPIVersion,
		Name:         "vol1",
		InternalName: "internal_vol1",
		Size:         "1GB",
		Protocol:     config.File,
		StorageClass: "gold",
	}
	volume := &storage.Volume{
		Config:      &volConfig,
		BackendUUID: uuid.New().String(),
		Pool:        "aggr1",
		State:       storage.VolumeStateOnline,
	}
	txn := &storage.VolumeTransaction{
		Config: volume.Config,
		Op:     storage.DeleteVolume,
	}

	// Create a k8s CRD Object for use by the client-go bindings and CRD persistence layer
	txnCRD, err := tridentv1.NewTridentTransaction(txn)
	if err != nil {
		t.Fatal("Unable to construct TridentTransaction CRD: ", err)
	}

	Logc(ctx()).Debug("Creating transaction.")

	// Create the CRD
	_, err = crdClient.TridentV1().TridentTransactions(tridentNamespace).Create(ctx(), txnCRD, createOpts)
	if err != nil {
		t.Fatal("Unable to create TridentTransaction CRD: ", err)
	}

	Logc(ctx()).Debug("Created transaction.")
	time.Sleep(1 * time.Second)

	// Get the CRD
	savedTxn, getErr := crdClient.TridentV1().TridentTransactions(tridentNamespace).Get(ctx(), "vol1", getOpts)
	if getErr != nil {
		t.Fatalf("unable to get transaction CRD: %v", getErr)
	}

	Log().WithFields(LogFields{
		"txn": savedTxn,
	}).Debug("Got transaction.")

	// Ensure the CRD was saved with a Trident finalizer
	if !savedTxn.HasTridentFinalizers() {
		t.Fatalf("expected transaction CRD to have Trident finalizer")
	}

	Logc(ctx()).Debug("Deleting transaction.")

	// Delete the CRD
	deleteErr := crdClient.TridentV1().TridentTransactions(tridentNamespace).Delete(ctx(), "vol1", deleteOptions)
	if deleteErr != nil {
		t.Fatalf("unable to delete transaction CRD: %v", getErr)
	}

	Logc(ctx()).Debug("Deleted transaction.")

	transactionGone := false

	// Wait until the CRD disappears, which can only happen if the CRD controller removes the finalizers.
	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)

		// Get the latest version of the CRD
		updatedTxn, getErr := crdClient.TridentV1().TridentTransactions(tridentNamespace).Get(ctx(), "vol1", getOpts)

		Log().WithFields(LogFields{
			"txn":    updatedTxn,
			"getErr": getErr,
		}).Debug("Checking if transaction was deleted.")

		if k8serrors.IsNotFound(getErr) {
			Logc(ctx()).Debug("Transaction gone.")
			transactionGone = true
			break
		}
	}

	assert.True(t, transactionGone, "expected transaction to be deleted")

	// Clean up
	if err = crdController.Deactivate(); err != nil {
		t.Fatalf("error while deactivating: %v", err.Error())
	}
}

// Test NewTridentCrdController public constructor
func TestNewTridentCrdController_ErrorCases(t *testing.T) {
	// Test with nil orchestrator - this should succeed as the function doesn't validate
	controller, err := NewTridentCrdController(nil, "trident", "")
	// The function should succeed, but may fail due to K8S client creation issues in test
	if err != nil {
		// Accept various possible K8S client creation errors that can occur in test environments
		errorMsg := err.Error()
		expectedErrorPhrases := []string{
			"unable to load",
			"found the Kubernetes CLI, but it exited with error",
			"could not find the Kubernetes CLI",
			"connection refused",
			"i/o timeout",
			"dial tcp",
		}
		containsExpectedError := false
		for _, phrase := range expectedErrorPhrases {
			if strings.Contains(errorMsg, phrase) {
				containsExpectedError = true
				break
			}
		}
		assert.True(t, containsExpectedError, "Expected K8S client creation error, got: %s", errorMsg)
	} else {
		assert.NotNil(t, controller, "Expected valid controller even with nil orchestrator")
		assert.Nil(t, controller.orchestrator, "Expected nil orchestrator to be stored")
	}
}

// Test GetName method
func TestGetName_Core(t *testing.T) {
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

	name := crdController.GetName()
	assert.Equal(t, "crd", name, "Expected GetName to return 'crd'")
}

// Test updateTridentBackendConfigCR function
func TestUpdateTridentBackendConfigCR(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Create a backend config
	backendConfig := &tridentv1.TridentBackendConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backend-config",
			Namespace: "default",
		},
		Spec: tridentv1.TridentBackendConfigSpec{
			RawExtension: runtime.RawExtension{Raw: []byte(`{"version": 1, "storageDriverName": "ontap-nas"}`)},
		},
	}
	_, err = crdController.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Test updating the backend config
	backendConfig.Spec.Raw = []byte(`{"version": 1, "storageDriverName": "ontap-san"}`)
	updated, err := crdController.updateTridentBackendConfigCR(ctx, backendConfig)
	assert.NoError(t, err, "Expected no error updating backend config CR")
	assert.NotNil(t, updated, "Expected updated backend config to be returned")

	// Verify the update
	retrieved, err := crdController.crdClientset.TridentV1().TridentBackendConfigs("default").Get(ctx, "test-backend-config", metav1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, backendConfig.Spec.Raw, retrieved.Spec.Raw)
}

// Test various finalizer removal functions using table-driven tests
func TestRemoveFinalizersTableDriven(t *testing.T) {
	type testCase struct {
		name                      string
		createObject              func(*TridentCrdController, context.Context) (metav1.Object, error)
		removeFinalizersFunc      func(*TridentCrdController, context.Context, metav1.Object) error
		getUpdatedObject          func(*TridentCrdController, context.Context, string) (metav1.Object, error)
		expectedFinalizersRemoved bool
	}

	tests := []testCase{
		{
			name: "BackendConfig",
			createObject: func(controller *TridentCrdController, ctx context.Context) (metav1.Object, error) {
				backendConfig := &tridentv1.TridentBackendConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-backend-config",
						Namespace:  "default",
						Finalizers: []string{"trident.netapp.io"},
					},
					Spec: tridentv1.TridentBackendConfigSpec{
						RawExtension: runtime.RawExtension{Raw: []byte(`{"version": 1, "storageDriverName": "ontap-nas"}`)},
					},
				}
				created, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
				return created, err
			},
			removeFinalizersFunc: func(controller *TridentCrdController, ctx context.Context, obj metav1.Object) error {
				return controller.removeBackendConfigFinalizers(ctx, obj.(*tridentv1.TridentBackendConfig))
			},
			getUpdatedObject: func(controller *TridentCrdController, ctx context.Context, name string) (metav1.Object, error) {
				return controller.crdClientset.TridentV1().TridentBackendConfigs("default").Get(ctx, name, metav1.GetOptions{})
			},
			expectedFinalizersRemoved: true,
		},
		{
			name: "Node",
			createObject: func(controller *TridentCrdController, ctx context.Context) (metav1.Object, error) {
				node := &tridentv1.TridentNode{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-node",
						Namespace:  "default",
						Finalizers: []string{"trident.netapp.io"},
					},
				}
				created, err := controller.crdClientset.TridentV1().TridentNodes("default").Create(ctx, node, metav1.CreateOptions{})
				return created, err
			},
			removeFinalizersFunc: func(controller *TridentCrdController, ctx context.Context, obj metav1.Object) error {
				return controller.removeNodeFinalizers(ctx, obj.(*tridentv1.TridentNode))
			},
			getUpdatedObject: func(controller *TridentCrdController, ctx context.Context, name string) (metav1.Object, error) {
				return controller.crdClientset.TridentV1().TridentNodes("default").Get(ctx, name, metav1.GetOptions{})
			},
			expectedFinalizersRemoved: true,
		},
		{
			name: "StorageClass",
			createObject: func(controller *TridentCrdController, ctx context.Context) (metav1.Object, error) {
				storageClass := &tridentv1.TridentStorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-storage-class",
						Namespace:  "default",
						Finalizers: []string{"trident.netapp.io"},
					},
				}
				created, err := controller.crdClientset.TridentV1().TridentStorageClasses("default").Create(ctx, storageClass, metav1.CreateOptions{})
				return created, err
			},
			removeFinalizersFunc: func(controller *TridentCrdController, ctx context.Context, obj metav1.Object) error {
				return controller.removeStorageClassFinalizers(ctx, obj.(*tridentv1.TridentStorageClass))
			},
			getUpdatedObject: func(controller *TridentCrdController, ctx context.Context, name string) (metav1.Object, error) {
				return controller.crdClientset.TridentV1().TridentStorageClasses("default").Get(ctx, name, metav1.GetOptions{})
			},
			expectedFinalizersRemoved: true,
		},
		{
			name: "Version",
			createObject: func(controller *TridentCrdController, ctx context.Context) (metav1.Object, error) {
				version := &tridentv1.TridentVersion{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-version",
						Namespace:  "default",
						Finalizers: []string{"trident.netapp.io"},
					},
				}
				created, err := controller.crdClientset.TridentV1().TridentVersions("default").Create(ctx, version, metav1.CreateOptions{})
				return created, err
			},
			removeFinalizersFunc: func(controller *TridentCrdController, ctx context.Context, obj metav1.Object) error {
				return controller.removeVersionFinalizers(ctx, obj.(*tridentv1.TridentVersion))
			},
			getUpdatedObject: func(controller *TridentCrdController, ctx context.Context, name string) (metav1.Object, error) {
				return controller.crdClientset.TridentV1().TridentVersions("default").Get(ctx, name, metav1.GetOptions{})
			},
			expectedFinalizersRemoved: true,
		},
		{
			name: "VolumePublication",
			createObject: func(controller *TridentCrdController, ctx context.Context) (metav1.Object, error) {
				volPub := &tridentv1.TridentVolumePublication{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-volume-publication",
						Namespace:  "default",
						Finalizers: []string{"trident.netapp.io"},
					},
				}
				created, err := controller.crdClientset.TridentV1().TridentVolumePublications("default").Create(ctx, volPub, metav1.CreateOptions{})
				return created, err
			},
			removeFinalizersFunc: func(controller *TridentCrdController, ctx context.Context, obj metav1.Object) error {
				return controller.removeVolumePublicationFinalizers(ctx, obj.(*tridentv1.TridentVolumePublication))
			},
			getUpdatedObject: func(controller *TridentCrdController, ctx context.Context, name string) (metav1.Object, error) {
				return controller.crdClientset.TridentV1().TridentVolumePublications("default").Get(ctx, name, metav1.GetOptions{})
			},
			expectedFinalizersRemoved: true,
		},
		{
			name: "TMR",
			createObject: func(controller *TridentCrdController, ctx context.Context) (metav1.Object, error) {
				tmr := &tridentv1.TridentMirrorRelationship{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-tmr",
						Namespace:  "default",
						Finalizers: []string{"trident.netapp.io"},
					},
				}
				created, err := controller.crdClientset.TridentV1().TridentMirrorRelationships("default").Create(ctx, tmr, metav1.CreateOptions{})
				return created, err
			},
			removeFinalizersFunc: func(controller *TridentCrdController, ctx context.Context, obj metav1.Object) error {
				return controller.removeTMRFinalizers(ctx, obj.(*tridentv1.TridentMirrorRelationship))
			},
			getUpdatedObject: func(controller *TridentCrdController, ctx context.Context, name string) (metav1.Object, error) {
				return controller.crdClientset.TridentV1().TridentMirrorRelationships("default").Get(ctx, name, metav1.GetOptions{})
			},
			expectedFinalizersRemoved: true,
		},
		{
			name: "TSI",
			createObject: func(controller *TridentCrdController, ctx context.Context) (metav1.Object, error) {
				tsi := &tridentv1.TridentSnapshotInfo{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-tsi",
						Namespace:  "default",
						Finalizers: []string{"trident.netapp.io"},
					},
				}
				created, err := controller.crdClientset.TridentV1().TridentSnapshotInfos("default").Create(ctx, tsi, metav1.CreateOptions{})
				return created, err
			},
			removeFinalizersFunc: func(controller *TridentCrdController, ctx context.Context, obj metav1.Object) error {
				return controller.removeTSIFinalizers(ctx, obj.(*tridentv1.TridentSnapshotInfo))
			},
			getUpdatedObject: func(controller *TridentCrdController, ctx context.Context, name string) (metav1.Object, error) {
				return controller.crdClientset.TridentV1().TridentSnapshotInfos("default").Get(ctx, name, metav1.GetOptions{})
			},
			expectedFinalizersRemoved: true,
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

			crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
				crdClient, nil, nil)
			if err != nil {
				t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
			}

			ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

			// Create the object with finalizers
			createdObj, err := tt.createObject(crdController, ctx)
			assert.NoError(t, err)

			// Test removing finalizers
			err = tt.removeFinalizersFunc(crdController, ctx, createdObj)
			assert.NoError(t, err, "Expected no error removing %s finalizers", tt.name)

			// Verify finalizers are removed
			updated, err := tt.getUpdatedObject(crdController, ctx, createdObj.GetName())
			assert.NoError(t, err)
			if tt.expectedFinalizersRemoved {
				assert.Empty(t, updated.GetFinalizers(), "Expected %s finalizers to be removed", tt.name)
			}
		})
	}
}

// Test updateTSICR function
func TestUpdateTSICR(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Create a TSI
	tsi := &tridentv1.TridentSnapshotInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tsi",
			Namespace: "default",
		},
	}
	_, err = crdController.crdClientset.TridentV1().TridentSnapshotInfos("default").Create(ctx, tsi, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Test updating the TSI CR
	updated, err := crdController.updateTSICR(ctx, tsi)
	assert.NoError(t, err, "Expected no error updating TSI CR")
	assert.NotNil(t, updated, "Expected updated TSI to be returned")
}

// Test handleTridentSnapshotInfo function
func TestHandleTridentSnapshotInfo(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Create a TSI
	tsi := &tridentv1.TridentSnapshotInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tsi",
			Namespace: "default",
		},
	}
	_, err = crdController.crdClientset.TridentV1().TridentSnapshotInfos("default").Create(ctx, tsi, metav1.CreateOptions{})
	assert.NoError(t, err)

	keyItem := &KeyItem{
		key:        "default/test-tsi",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentSnapshotInfo,
	}

	// Test handling the TSI
	err = crdController.handleTridentSnapshotInfo(keyItem)
	assert.NoError(t, err, "Expected no error handling TSI")
}

// Test getSnapshotHandle function
func TestGetSnapshotHandle(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Create a TSI with snapshot name
	tsi := &tridentv1.TridentSnapshotInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tsi",
			Namespace: "default",
		},
		Spec: tridentv1.TridentSnapshotInfoSpec{
			SnapshotName: "test-snapshot",
		},
	}

	// Test getting snapshot handle - should return empty string and error for non-existent snapshot
	handle, err := crdController.getSnapshotHandle(ctx, tsi)
	assert.Error(t, err, "Expected error for non-existent snapshot")
	assert.Empty(t, handle, "Expected empty handle for non-existent snapshot")
}

// Test getSnapshotHandle with successful snapshot creation
func TestGetSnapshotHandle_WithSnapshot(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Create a VolumeSnapshot
	volumeSnapshot := &snapv1.VolumeSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "default",
		},
		Spec: snapv1.VolumeSnapshotSpec{
			Source: snapv1.VolumeSnapshotSource{
				PersistentVolumeClaimName: stringPtr("test-pvc"),
			},
		},
	}
	_, err = crdController.snapshotClientSet.SnapshotV1().VolumeSnapshots("default").Create(ctx, volumeSnapshot, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create a TSI that references the snapshot
	tsi := &tridentv1.TridentSnapshotInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tsi",
			Namespace: "default",
		},
		Spec: tridentv1.TridentSnapshotInfoSpec{
			SnapshotName: "test-snapshot",
		},
	}

	// This should still return an error because snapshot doesn't have bound content
	handle, err := crdController.getSnapshotHandle(ctx, tsi)
	assert.Error(t, err, "Expected error for snapshot without bound content")
	assert.Empty(t, handle, "Expected empty handle for unbound snapshot")
}

// Helper function for string pointer
func stringPtr(s string) *string {
	return &s
}

// Test handleTridentBackendConfig with various edge cases
func TestHandleTridentBackendConfig_EdgeCases(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	// Test with nil keyItem (should return error)
	err := controller.handleTridentBackendConfig(nil)
	assert.Error(t, err, "Expected error with nil keyItem")

	// Test with invalid key format
	keyItem := &KeyItem{
		key:        "invalid-key-format",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackendConfig,
	}
	err = controller.handleTridentBackendConfig(keyItem)
	assert.NoError(t, err, "Expected no error with invalid key (should log and continue)")

	// Test with non-existent backend config (backend not found in lister)
	keyItem = &KeyItem{
		key:        "default/non-existent-backend",
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackendConfig,
	}
	err = controller.handleTridentBackendConfig(keyItem)
	assert.NoError(t, err, "Expected no error when backend not found")
}

func TestProcessNextWorkItem_EdgeCases(t *testing.T) {
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

	// Add a malformed item to the workqueue that should cause an error

	// Add an invalid key to test error handling
	crdController.workqueue.Add("invalid-key-format")

	// This should process the invalid key and return true (continue processing)
	result := crdController.processNextWorkItem()
	assert.True(t, result, "Expected true even with invalid key (should log error and continue)")
}

// Test removeFinalizers function with different CRD types
func TestRemoveFinalizers_VariousTypes(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Test with TridentBackend
	backend := &tridentv1.TridentBackend{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-backend",
			Namespace:  "default",
			Finalizers: []string{"trident.netapp.io"},
		},
	}
	_, err = crdController.crdClientset.TridentV1().TridentBackends("default").Create(ctx, backend, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = crdController.removeFinalizers(ctx, backend, false)
	assert.NoError(t, err, "Expected no error removing backend finalizers")

	// Test with TridentVolume
	volume := &tridentv1.TridentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-volume",
			Namespace:  "default",
			Finalizers: []string{"trident.netapp.io"},
		},
	}
	_, err = crdController.crdClientset.TridentV1().TridentVolumes("default").Create(ctx, volume, metav1.CreateOptions{})
	assert.NoError(t, err)

	err = crdController.removeFinalizers(ctx, volume, false)
	assert.NoError(t, err, "Expected no error removing volume finalizers")
}

// Test updateTMRStatus function with proper parameters
func TestUpdateTMRStatus(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Create a TMR
	tmr := &tridentv1.TridentMirrorRelationship{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-tmr",
			Namespace:  "default",
			Generation: 1,
		},
	}
	_, err = crdController.crdClientset.TridentV1().TridentMirrorRelationships("default").Create(ctx, tmr, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create a status condition
	statusCondition := &tridentv1.TridentMirrorRelationshipCondition{
		MirrorState:        "mirrored",
		Message:            "Test message",
		LocalPVCName:       "test-pvc",
		RemoteVolumeHandle: "remote-handle",
	}

	// Test updating TMR status
	updated, err := crdController.updateTMRStatus(ctx, tmr, statusCondition)
	assert.NoError(t, err, "Expected no error updating TMR status")
	assert.NotNil(t, updated, "Expected updated TMR to be returned")
	assert.Equal(t, 1, statusCondition.ObservedGeneration, "Expected generation to be set")
}

// Test getCurrentMirrorState function with proper parameters
// Test getCurrentMirrorState function with table-driven approach
func TestGetCurrentMirrorStateTableDriven(t *testing.T) {
	type testCase struct {
		name             string
		mockError        error
		mockReturnState  string
		expectedError    bool
		expectedState    string
		expectedErrorMsg string
	}

	tests := []testCase{
		{
			name:             "UnsupportedError",
			mockError:        errors.UnsupportedError("not supported"),
			mockReturnState:  "",
			expectedError:    false,
			expectedState:    "promoted",
			expectedErrorMsg: "",
		},
		{
			name:             "SuccessfulMirrorStatus",
			mockError:        nil,
			mockReturnState:  "mirrored",
			expectedError:    false,
			expectedState:    "mirrored",
			expectedErrorMsg: "",
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

			crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
				crdClient, nil, nil)
			if err != nil {
				t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
			}

			ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

			// Set up mock expectation
			orchestrator.EXPECT().GetMirrorStatus(ctx, "backend-uuid", "local-volume", "remote-handle").
				Return(tt.mockReturnState, tt.mockError)

			// Execute the function under test
			state, err := crdController.getCurrentMirrorState(ctx, "mirrored", "backend-uuid", "local-volume", "remote-handle")

			// Verify results
			if tt.expectedError {
				assert.Error(t, err, "Expected error for %s", tt.name)
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg, "Expected specific error message")
				}
			} else {
				assert.NoError(t, err, "Expected no error for %s", tt.name)
			}
			assert.Equal(t, tt.expectedState, state, "Expected correct state for %s", tt.name)
		})
	}
}

// Test ensureMirrorReadyForDeletion function with proper parameters
func TestEnsureMirrorReadyForDeletion(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Create a TMR
	tmr := &tridentv1.TridentMirrorRelationship{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tmr",
			Namespace: "default",
		},
		Spec: tridentv1.TridentMirrorRelationshipSpec{
			MirrorState: "mirrored",
		},
	}
	_, err = crdController.crdClientset.TridentV1().TridentMirrorRelationships("default").Create(ctx, tmr, metav1.CreateOptions{})
	assert.NoError(t, err)

	// Create volume mapping
	volumeMapping := &tridentv1.TridentMirrorRelationshipVolumeMapping{
		LocalPVCName:           "local-pvc",
		PromotedSnapshotHandle: "snapshot-handle",
	}

	// Create current condition
	currentCondition := &tridentv1.TridentMirrorRelationshipCondition{
		MirrorState:        "mirrored",
		LocalPVCName:       "local-pvc",
		RemoteVolumeHandle: "remote-handle",
	}

	// Mock the orchestrator call for promoting mirror
	orchestrator.EXPECT().PromoteMirror(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(true, nil).AnyTimes()

	// Test ensuring mirror is ready for deletion
	ready, err := crdController.ensureMirrorReadyForDeletion(ctx, tmr, volumeMapping, currentCondition)
	assert.NoError(t, err, "Expected no error ensuring mirror ready for deletion")
	assert.True(t, ready, "Expected mirror to be ready for deletion")
}

func TestHandleTridentBackendConfig_NilKeyItem_Enhanced(t *testing.T) {
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

	// Test nil keyItem
	err = crdController.handleTridentBackendConfig(nil)
	assert.Error(t, err, "Expected error for nil keyItem")
	assert.Contains(t, err.Error(), "keyItem item is nil", "Expected specific error message")
}

// Test removeFinalizers with nil object
func TestRemoveFinalizers_NilObject(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Test with nil object (should return error due to unexpected type)
	err = crdController.removeFinalizers(ctx, nil, false)
	assert.Error(t, err, "Expected error for nil object")
	assert.Contains(t, err.Error(), "unexpected type", "Expected specific error message for nil object")
}

// Test processNextWorkItem with additional edge case
func TestProcessNextWorkItem_AdditionalEdgeCases(t *testing.T) {
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

	// Add a malformed key that should be processed but cause logging
	crdController.workqueue.Add("namespace/name/extra")

	// This should process the malformed key gracefully
	result := crdController.processNextWorkItem()
	assert.True(t, result, "Expected true for malformed key processing")
}

// Test getSnapshotHandle with more edge cases
func TestGetSnapshotHandle_AdditionalEdgeCases(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Test with snapshot info that has empty name
	snapshotInfo := &tridentv1.TridentSnapshotInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "",
			Namespace: "test-namespace",
		},
	}
	handle, err := crdController.getSnapshotHandle(ctx, snapshotInfo)
	assert.Error(t, err, "Expected error for empty snapshot name")
	assert.Empty(t, handle, "Expected empty handle for invalid snapshot")

	// Test with snapshot info that has empty namespace
	snapshotInfo2 := &tridentv1.TridentSnapshotInfo{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-snapshot",
			Namespace: "",
		},
	}
	handle, err = crdController.getSnapshotHandle(ctx, snapshotInfo2)
	assert.Error(t, err, "Expected error for empty namespace")
	assert.Empty(t, handle, "Expected empty handle for invalid namespace")
}

// Test addCRHandler, updateCRHandler, deleteCRHandler with nil object
func TestCRHandlers_NilObject(t *testing.T) {
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

	// Test addCRHandler with nil object
	crdController.addCRHandler(nil)

	// Test updateCRHandler with nil objects
	crdController.updateCRHandler(nil, nil)

	// Test deleteCRHandler with nil object
	crdController.deleteCRHandler(nil)
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Phase 1: Controller Lifecycle Tests

func TestControllerLifecycle_Activate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	indexers := mockindexers.NewMockIndexers(mockCtrl)
	indexers.EXPECT().Deactivate()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, indexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// Test Activate method
	err = crdController.Activate()
	assert.NoError(t, err, "Activate should not return an error")

	// Verify that stop channel is set
	assert.NotNil(t, crdController.crdControllerStopChan, "Stop channel should be initialized")

	// Clean up
	err = crdController.Deactivate()
	assert.NoError(t, err, "Deactivate should not return an error")
}

func TestControllerLifecycle_Deactivate(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	indexers := mockindexers.NewMockIndexers(mockCtrl)
	indexers.EXPECT().Deactivate()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, indexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// First activate to set up stop channel
	err = crdController.Activate()
	assert.NoError(t, err, "Activate should not return an error")

	// Test Deactivate method
	err = crdController.Deactivate()
	assert.NoError(t, err, "Deactivate should not return an error")

	// Test deactivate when already deactivated (channel should be nil after first deactivate)
	// Create a new controller to test deactivate with nil channel
	indexers = mockindexers.NewMockIndexers(mockCtrl)
	indexers.EXPECT().Deactivate()
	crdController2, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, indexers, nil)
	if err != nil {
		t.Fatalf("cannot create second Trident CRD controller frontend, error: %v", err.Error())
	}

	// Set stop channel to nil to test nil handling in Deactivate
	crdController2.crdControllerStopChan = nil
	err = crdController2.Deactivate()
	assert.NoError(t, err, "Deactivate should handle nil stop channel gracefully")
}

func TestControllerLifecycle_GetName(t *testing.T) {
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

	// Test GetName method
	name := crdController.GetName()
	assert.Equal(t, "crd", name, "GetName should return 'crd'")
}

func TestControllerLifecycle_Version(t *testing.T) {
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

	// Test Version method
	version := crdController.Version()
	assert.NotEmpty(t, version, "Version should return a non-empty string")
	assert.Equal(t, config.DefaultOrchestratorVersion, version, "Version should return DefaultOrchestratorVersion")
}

func TestControllerLifecycle_Run(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	stopCh := make(chan struct{})

	// Test Run method with immediate stop
	go func() {
		// Stop immediately to test graceful shutdown
		time.Sleep(100 * time.Millisecond)
		close(stopCh)
	}()

	// This should complete without hanging
	crdController.Run(ctx, 1, stopCh)
	// If we reach here, Run completed successfully
}

func TestControllerLifecycle_RunWorker(t *testing.T) {
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

	// Shutdown the workqueue to make runWorker exit immediately
	crdController.workqueue.ShutDown()

	// Test runWorker method - should exit gracefully when queue is shut down
	crdController.runWorker()
	// If we reach here, runWorker completed successfully
}

func TestControllerLifecycle_AddEventToWorkqueue(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Test addEventToWorkqueue method
	testKey := "test-namespace/test-object"
	testEvent := EventAdd
	testObjectType := ObjectTypeTridentBackend

	// Add event to workqueue
	crdController.addEventToWorkqueue(testKey, testEvent, ctx, testObjectType)

	// Verify the event was added
	assert.Equal(t, 1, crdController.workqueue.Len(), "Workqueue should contain one item")

	// Get the item and verify its contents
	item, shutdown := crdController.workqueue.Get()
	assert.False(t, shutdown, "Workqueue should not be shut down")

	keyItem, ok := item.(KeyItem)
	assert.True(t, ok, "Item should be of type KeyItem")
	assert.Equal(t, testKey, keyItem.key, "Key should match")
	assert.Equal(t, testEvent, keyItem.event, "Event should match")
	assert.Equal(t, testObjectType, keyItem.objectType, "Object type should match")
	assert.Equal(t, ctx, keyItem.ctx, "Context should match")
	assert.False(t, keyItem.isRetry, "isRetry should be false by default")

	// Clean up
	crdController.workqueue.Done(item)
}

func TestControllerLifecycle_LogxFunction(t *testing.T) {
	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Test Logx function
	logger := Logx(ctx)
	assert.NotNil(t, logger, "Logx should return a non-nil logger")

	// Verify the logger has the correct source field
	logger.Debug("Test log message")
}

func TestControllerLifecycle_ProcessNextWorkItem_EmptyQueue(t *testing.T) {
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

	// Shutdown the workqueue
	crdController.workqueue.ShutDown()

	// Test processNextWorkItem with empty/shutdown queue
	result := crdController.processNextWorkItem()
	assert.False(t, result, "processNextWorkItem should return false when queue is shutdown")
}

func TestControllerLifecycle_RunWithDifferentThreadiness(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Test Run with different threadiness values
	testCases := []int{1, 2, 5}

	for _, threadiness := range testCases {
		stopCh := make(chan struct{})

		go func() {
			// Stop immediately to test with different threadiness
			time.Sleep(50 * time.Millisecond)
			close(stopCh)
		}()

		// This should complete without hanging regardless of threadiness
		crdController.Run(ctx, threadiness, stopCh)
	}
}

func TestControllerLifecycle_ActivateDeactivateNilStopChannel(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	indexers := mockindexers.NewMockIndexers(mockCtrl)
	indexers.EXPECT().Deactivate()
	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient,
		crdClient, indexers, nil)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// Set stop channel to nil to test nil handling
	crdController.crdControllerStopChan = nil

	// Test Activate with nil stop channel
	err = crdController.Activate()
	assert.NoError(t, err, "Activate should handle nil stop channel")

	// Test Deactivate with nil stop channel
	err = crdController.Deactivate()
	assert.NoError(t, err, "Deactivate should handle nil stop channel")
}

func TestControllerLifecycle_WorkqueueOperations(t *testing.T) {
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

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Test multiple event types
	eventTypes := []EventType{EventAdd, EventUpdate, EventForceUpdate, EventDelete}
	objectTypes := []string{
		ObjectTypeTridentBackend,
		ObjectTypeTridentBackendConfig,
		ObjectTypeTridentMirrorRelationship,
		ObjectTypeTridentSnapshotInfo,
		ObjectTypeTridentActionSnapshotRestore,
		ObjectTypeSecret,
	}

	// Add multiple events to test workqueue handling
	for i, eventType := range eventTypes {
		for j, objectType := range objectTypes {
			key := fmt.Sprintf("namespace-%d/object-%d-%d", i, i, j)
			crdController.addEventToWorkqueue(key, eventType, ctx, objectType)
		}
	}

	// Verify all events were added
	expectedCount := len(eventTypes) * len(objectTypes)
	assert.Equal(t, expectedCount, crdController.workqueue.Len(), "All events should be in workqueue")

	// Test draining the workqueue
	for crdController.workqueue.Len() > 0 {
		item, shutdown := crdController.workqueue.Get()
		if shutdown {
			break
		}
		crdController.workqueue.Done(item)
	}

	assert.Equal(t, 0, crdController.workqueue.Len(), "Workqueue should be empty after draining")
}

// Additional core lifecycle tests
func TestNewTridentCrdController_Success(t *testing.T) {
	// Test the internal implementation function directly
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	controller, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient,
		nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, controller)
	assert.Equal(t, "crd", controller.GetName())
	assert.Equal(t, config.DefaultOrchestratorVersion, controller.Version())
	assert.NotNil(t, controller.workqueue)
	assert.NotNil(t, controller.orchestrator)
}

func TestActivateDeactivate_FullCycle(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	indexers := mockindexers.NewMockIndexers(mockCtrl)
	indexers.EXPECT().Deactivate()
	controller, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient,
		indexers, nil)
	assert.NoError(t, err)

	// Test GetName and Version first
	assert.Equal(t, "crd", controller.GetName())
	assert.Equal(t, config.DefaultOrchestratorVersion, controller.Version())

	// Test Activate
	err = controller.Activate()
	assert.NoError(t, err)

	// Give some time for goroutines to start
	time.Sleep(100 * time.Millisecond)

	// Verify the controller is active
	assert.NotNil(t, controller.crdControllerStopChan)

	// Test Deactivate
	err = controller.Deactivate()
	assert.NoError(t, err)
}

func TestRun_CacheSyncAndWorkerManagement(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	controller, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient,
		nil, nil)
	assert.NoError(t, err)

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	stopCh := make(chan struct{})

	// Start Run in a goroutine
	done := make(chan bool)
	go func() {
		controller.Run(ctx, 2, stopCh)
		done <- true
	}()

	// Give time for workers to start
	time.Sleep(200 * time.Millisecond)

	// Stop the controller
	close(stopCh)

	// Wait for Run to complete
	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not complete within timeout")
	}
}

func TestRunWorker_ProcessingLoop(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	controller, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient,
		nil, nil)
	assert.NoError(t, err)

	// Add a test item to the workqueue
	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	keyItem := KeyItem{
		key:        "test-namespace/test-key",
		event:      EventAdd,
		ctx:        ctx,
		objectType: "UnknownType", // This will cause the worker to handle gracefully
	}
	controller.workqueue.Add(keyItem)

	// Shutdown the workqueue after one item to test worker exit
	go func() {
		time.Sleep(100 * time.Millisecond)
		controller.workqueue.ShutDown()
	}()

	// This should process one item and then exit due to shutdown
	controller.runWorker()
}

func TestAddEventToWorkqueue_VariousEvents(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	controller, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient,
		nil, nil)
	assert.NoError(t, err)

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Test the Logx function first
	logEntry := Logx(ctx)
	assert.NotNil(t, logEntry)

	// Test various event types and object types
	testCases := []struct {
		key        string
		event      EventType
		objectType string
	}{
		{"ns1/backend1", EventAdd, ObjectTypeTridentBackend},
		{"ns2/config1", EventUpdate, ObjectTypeTridentBackendConfig},
		{"ns3/secret1", EventUpdate, ObjectTypeSecret},
		{"ns4/mirror1", EventDelete, ObjectTypeTridentMirrorRelationship},
		{"ns5/snapshot1", EventAdd, ObjectTypeTridentSnapshotInfo},
	}

	for _, tc := range testCases {
		controller.addEventToWorkqueue(tc.key, tc.event, ctx, tc.objectType)
	}

	// Verify all events were added
	assert.Equal(t, len(testCases), controller.workqueue.Len())

	// Verify the events can be retrieved correctly
	for i := 0; i < len(testCases); i++ {
		item, shutdown := controller.workqueue.Get()
		assert.False(t, shutdown)

		keyItem, ok := item.(KeyItem)
		assert.True(t, ok)
		assert.NotEmpty(t, keyItem.key)
		assert.NotEmpty(t, keyItem.objectType)

		controller.workqueue.Done(item)
	}
}

func TestLogx_Function(t *testing.T) {
	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Test that Logx function returns a valid log entry
	logEntry := Logx(ctx)
	assert.NotNil(t, logEntry)
}

func TestRemoveFinalizers_AllTypes(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	controller, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient,
		nil, nil)
	assert.NoError(t, err)

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Test with unsupported type
	unsupportedObj := &corev1.Pod{}
	err = controller.removeFinalizers(ctx, unsupportedObj, false)
	assert.Error(t, err, "Expected error when calling removeFinalizers with unsupported Pod type")

	// Test with TridentBackend - should do nothing
	backend := &tridentv1.TridentBackend{}
	err = controller.removeFinalizers(ctx, backend, false)
	assert.NoError(t, err)

	// Test force removal with TridentBackendConfig
	backendConfig := &tridentv1.TridentBackendConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: tridentNamespace,
		},
	}
	err = controller.removeFinalizers(ctx, backendConfig, true)
	assert.NoError(t, err)
}

func TestProcessNextWorkItem_ErrorHandling(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	controller, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient,
		nil, nil)
	assert.NoError(t, err)

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Test with invalid object type
	controller.workqueue.Add("invalid-object")
	result := controller.processNextWorkItem()
	assert.True(t, result) // Should return true to continue processing

	// Test with empty KeyItem
	controller.workqueue.Add(KeyItem{})
	result = controller.processNextWorkItem()
	assert.True(t, result) // Should return true to continue processing

	// Test with unknown object type
	keyItem := KeyItem{
		key:        "test-namespace/test-key",
		event:      EventAdd,
		ctx:        ctx,
		objectType: "UnknownType",
	}
	controller.workqueue.Add(keyItem)
	result = controller.processNextWorkItem()
	assert.True(t, result) // Should return true to continue processing
}

func TestGetTridentBackend_EdgeCases(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()
	controller, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient,
		nil, nil)
	assert.NoError(t, err)

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Test getting non-existent backend
	backend, err := controller.getTridentBackend(ctx, tridentNamespace, "non-existent-uuid")
	assert.NoError(t, err)
	assert.Nil(t, backend)

	// Create a test backend first
	testBackend := &tridentv1.TridentBackend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backend",
			Namespace: tridentNamespace,
		},
		BackendUUID: "test-uuid-123",
	}

	_, err = crdClient.TridentV1().TridentBackends(tridentNamespace).Create(ctx, testBackend, createOpts)
	assert.NoError(t, err)

	// Test getting existing backend
	backend, err = controller.getTridentBackend(ctx, tridentNamespace, "test-uuid-123")
	assert.NoError(t, err)
	assert.NotNil(t, backend)
	assert.Equal(t, "test-uuid-123", backend.BackendUUID)
}
