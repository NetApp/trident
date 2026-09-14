// Copyright 2026 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/netapp/trident/core"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils/errors"
)

const testVolumeReconcileNamespace = "trident"

func newTestVolumeReconcileController(t *testing.T, orchestrator core.Orchestrator) *TridentCrdController {
	t.Helper()
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	controller, err := newTridentCrdControllerImpl(
		orchestrator, testVolumeReconcileNamespace, kubeClient, snapClient, crdClient, nil, nil,
	)
	require.NoError(t, err)
	return controller
}

func createTestTridentVolumeWithConfig(t *testing.T, name, namespace string, config map[string]any) *tridentv1.TridentVolume {
	t.Helper()
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	return &tridentv1.TridentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Config: runtime.RawExtension{Raw: raw},
	}
}

func TestHandleTridentVolume_PushesNodeConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	controller := newTestVolumeReconcileController(t, orchestrator)

	volumeCR := createTestTridentVolumeWithConfig(t, "vol1", testVolumeReconcileNamespace, map[string]any{
		"name":                "vol1",
		"luksPassphraseNames": []string{"a", "b"},
	})
	controller.crdInformerFactory.Trident().V1().TridentVolumes().Informer().GetIndexer().Add(volumeCR)

	orchestrator.EXPECT().RefreshVolumeNodeConfig(gomock.Any(), "vol1", []string{"a", "b"}).Return(nil)

	keyItem := &KeyItem{
		key: testVolumeReconcileNamespace + "/vol1", event: EventUpdate, ctx: context.Background(),
		objectType: ObjectTypeTridentVolume,
	}
	err := controller.handleTridentVolume(keyItem)
	assert.NoError(t, err)
}

func TestHandleTridentVolume_NotFoundIsNoOp(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	controller := newTestVolumeReconcileController(t, orchestrator)

	// No CR seeded into the indexer, and no orchestrator call expected.
	keyItem := &KeyItem{
		key: testVolumeReconcileNamespace + "/nonexistent", event: EventUpdate, ctx: context.Background(),
		objectType: ObjectTypeTridentVolume,
	}
	err := controller.handleTridentVolume(keyItem)
	assert.NoError(t, err)
}

func TestHandleTridentVolume_MalformedConfigErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	controller := newTestVolumeReconcileController(t, orchestrator)

	volumeCR := &tridentv1.TridentVolume{
		ObjectMeta: metav1.ObjectMeta{Name: "vol1", Namespace: testVolumeReconcileNamespace},
		Config:     runtime.RawExtension{Raw: []byte(`not json`)},
	}
	controller.crdInformerFactory.Trident().V1().TridentVolumes().Informer().GetIndexer().Add(volumeCR)

	keyItem := &KeyItem{
		key: testVolumeReconcileNamespace + "/vol1", event: EventUpdate, ctx: context.Background(),
		objectType: ObjectTypeTridentVolume,
	}
	err := controller.handleTridentVolume(keyItem)
	assert.Error(t, err)
}

func TestHandleTridentVolume_OrchestratorNotFoundIsSwallowed(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	controller := newTestVolumeReconcileController(t, orchestrator)

	volumeCR := createTestTridentVolumeWithConfig(t, "vol1", testVolumeReconcileNamespace, map[string]any{
		"name": "vol1", "luksPassphraseNames": []string{"a"},
	})
	controller.crdInformerFactory.Trident().V1().TridentVolumes().Informer().GetIndexer().Add(volumeCR)

	// The controller hasn't heard of this volume (e.g. it hasn't finished bootstrapping yet).
	orchestrator.EXPECT().RefreshVolumeNodeConfig(gomock.Any(), "vol1", []string{"a"}).
		Return(errors.NotFoundError("volume vol1 not found"))

	keyItem := &KeyItem{
		key: testVolumeReconcileNamespace + "/vol1", event: EventUpdate, ctx: context.Background(),
		objectType: ObjectTypeTridentVolume,
	}
	err := controller.handleTridentVolume(keyItem)
	assert.NoError(t, err)
}

func TestUpdateTridentVolumeHandler_NamesUnchangedIsNoOp(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	controller := newTestVolumeReconcileController(t, orchestrator)

	oldVol := createTestTridentVolumeWithConfig(t, "vol1", testVolumeReconcileNamespace, map[string]any{
		"luksPassphraseNames": []string{"a"},
	})
	newVol := createTestTridentVolumeWithConfig(t, "vol1", testVolumeReconcileNamespace, map[string]any{
		"luksPassphraseNames": []string{"a"}, "size": "5Gi",
	})

	initialLen := controller.workqueue.Len()
	controller.updateTridentVolumeHandler(oldVol, newVol)
	assert.Equal(t, initialLen, controller.workqueue.Len())
}

func TestUpdateTridentVolumeHandler_NamesChangedEnqueues(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	controller := newTestVolumeReconcileController(t, orchestrator)

	oldVol := createTestTridentVolumeWithConfig(t, "vol1", testVolumeReconcileNamespace, map[string]any{
		"luksPassphraseNames": []string{"a"},
	})
	newVol := createTestTridentVolumeWithConfig(t, "vol1", testVolumeReconcileNamespace, map[string]any{
		"luksPassphraseNames": []string{"a", "b"},
	})

	controller.updateTridentVolumeHandler(oldVol, newVol)

	require.Equal(t, 1, controller.workqueue.Len())
	item, _ := controller.workqueue.Get()
	keyItem := item.(KeyItem)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, ObjectTypeTridentVolume, keyItem.objectType)
	assert.Equal(t, testVolumeReconcileNamespace+"/vol1", keyItem.key)
}

// TestUpdateTridentVolumeHandler_MetadataGenerationBumpAlone_NotEnqueued pins the "don't use the
// metadata.generation filter" decision: TridentVolume's metadata.generation bumps on ANY change
// (resize, state, ...) since the CRD has no status subresource, but this handler's filter only
// cares about the node-owned luksPassphraseNames key, so a size-only change - which still
// bumps metadata.generation - must not enqueue work.
func TestUpdateTridentVolumeHandler_MetadataGenerationBumpAlone_NotEnqueued(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	controller := newTestVolumeReconcileController(t, orchestrator)

	oldVol := createTestTridentVolumeWithConfig(t, "vol1", testVolumeReconcileNamespace, map[string]any{
		"luksPassphraseNames": []string{"a"}, "size": "1Gi",
	})
	oldVol.ObjectMeta.Generation = 5
	newVol := createTestTridentVolumeWithConfig(t, "vol1", testVolumeReconcileNamespace, map[string]any{
		"luksPassphraseNames": []string{"a"}, "size": "5Gi",
	})
	newVol.ObjectMeta.Generation = 6

	initialLen := controller.workqueue.Len()
	controller.updateTridentVolumeHandler(oldVol, newVol)
	assert.Equal(t, initialLen, controller.workqueue.Len())
}

func TestUpdateTridentVolumeHandler_ReorderedNamesIsNoOp(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	controller := newTestVolumeReconcileController(t, orchestrator)

	oldVol := createTestTridentVolumeWithConfig(t, "vol1", testVolumeReconcileNamespace, map[string]any{
		"luksPassphraseNames": []string{"a", "b"},
	})
	newVol := createTestTridentVolumeWithConfig(t, "vol1", testVolumeReconcileNamespace, map[string]any{
		"luksPassphraseNames": []string{"b", "a"},
	})

	initialLen := controller.workqueue.Len()
	controller.updateTridentVolumeHandler(oldVol, newVol)
	assert.Equal(t, initialLen, controller.workqueue.Len(),
		"the names are an unordered set, so a reorder must not enqueue work")
}

func TestUpdateTridentVolumeHandler_WrongTypeDoesNotPanic(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	controller := newTestVolumeReconcileController(t, orchestrator)

	initialLen := controller.workqueue.Len()
	assert.NotPanics(t, func() {
		controller.updateTridentVolumeHandler("not-a-volume", "also-not-a-volume")
	})
	assert.Equal(t, initialLen, controller.workqueue.Len())
}
