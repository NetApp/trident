// Copyright 2025 NetApp, Inc. All Rights Reserved.

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/config"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
)

func createTestTridentBackend(name, namespace, backendUUID string) *tridentv1.TridentBackend {
	return &tridentv1.TridentBackend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		BackendUUID: backendUUID,
		BackendName: "test-backend",
		State:       "online",
	}
}

func TestDeleteTridentBackendHandler(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	backend := createTestTridentBackend("tbe-backend-123", "default", "backend-uuid-123")

	// This should add item to work queue
	controller.deleteTridentBackendHandler(backend)

	// Check if work item was added
	assert.False(t, controller.workqueue.ShuttingDown())
	workItem, shutdown := controller.workqueue.Get()
	assert.False(t, shutdown)

	keyItem := workItem.(KeyItem)
	assert.Equal(t, EventDelete, keyItem.event)
	assert.Equal(t, ObjectTypeTridentBackend, keyItem.objectType)
	assert.Equal(t, "default/backend-uuid-123", keyItem.key) // Key should be namespace/backendUUID
}

func TestHandleTridentBackend_WrongEventType(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	keyItem := &KeyItem{
		key:        "default/backend-uuid-123",
		event:      EventUpdate, // Wrong event type, should be EventDelete
		ctx:        ctx,
		objectType: ObjectTypeTridentBackend,
	}

	err := controller.handleTridentBackend(keyItem)
	assert.NoError(t, err) // Should handle gracefully
}

func TestHandleTridentBackend_InvalidKey(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	keyItem := &KeyItem{
		key:        "invalid-key-format",
		event:      EventDelete,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackend,
	}

	err := controller.handleTridentBackend(keyItem)
	assert.NoError(t, err) // Should handle invalid key gracefully
}

func TestHandleTridentBackend_BackendConfigNotFound(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	keyItem := &KeyItem{
		key:        "default/non-existent-backend-uuid",
		event:      EventDelete,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackend,
	}

	err := controller.handleTridentBackend(keyItem)
	assert.NoError(t, err) // Should handle not found gracefully
}

func TestHandleTridentBackend_BackendConfigFound(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	// Create backend config
	backendConfig := createTestBackendConfig("test-backend-config", "default")
	backendConfig.Status.Phase = string(tridentv1.PhaseBound)
	backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
		BackendName: "test-backend",
		BackendUUID: "backend-uuid-123",
	}

	_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
	assert.NoError(t, err)

	keyItem := &KeyItem{
		key:        "default/backend-uuid-123",
		event:      EventDelete,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackend,
	}

	err = controller.handleTridentBackend(keyItem)
	assert.NoError(t, err)

	// Check if backend config was added back to queue for force update
	workItem, shutdown := controller.workqueue.Get()
	assert.False(t, shutdown)

	newKeyItem := workItem.(KeyItem)
	assert.Equal(t, EventForceUpdate, newKeyItem.event)
	assert.Equal(t, ObjectTypeTridentBackendConfig, newKeyItem.objectType)
	assert.Equal(t, "default/test-backend-config", newKeyItem.key)
}

func TestHandleTridentBackend_BackendConfigUnboundPhase(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	// Create backend config in unbound phase
	backendConfig := createTestBackendConfig("test-backend-config", "default")
	backendConfig.Status.Phase = string(tridentv1.PhaseUnbound)
	backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
		BackendName: "test-backend",
		BackendUUID: "backend-uuid-123",
	}

	_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
	assert.NoError(t, err)

	keyItem := &KeyItem{
		key:        "default/backend-uuid-123",
		event:      EventDelete,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackend,
	}

	err = controller.handleTridentBackend(keyItem)
	assert.NoError(t, err) // Should return early for unbound phase
}

func TestHandleTridentBackend_BackendConfigDeletingPhase(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	// Create backend config in deleting phase
	backendConfig := createTestBackendConfig("test-backend-config", "default")
	backendConfig.Status.Phase = string(tridentv1.PhaseDeleting)
	backendConfig.Status.BackendInfo = tridentv1.TridentBackendConfigBackendInfo{
		BackendName: "test-backend",
		BackendUUID: "backend-uuid-123",
	}

	_, err := controller.crdClientset.TridentV1().TridentBackendConfigs("default").Create(ctx, backendConfig, metav1.CreateOptions{})
	assert.NoError(t, err)

	keyItem := &KeyItem{
		key:        "default/backend-uuid-123",
		event:      EventDelete,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackend,
	}

	err = controller.handleTridentBackend(keyItem)
	assert.NoError(t, err)

	// Check if backend config was added back to queue for deletion
	workItem, shutdown := controller.workqueue.Get()
	assert.False(t, shutdown)

	newKeyItem := workItem.(KeyItem)
	assert.Equal(t, EventDelete, newKeyItem.event)
	assert.Equal(t, ObjectTypeTridentBackendConfig, newKeyItem.objectType)
	assert.Equal(t, "default/test-backend-config", newKeyItem.key)
}

func setDataLIFRefresh(t *testing.T, enabled bool) {
	t.Helper()
	previous := config.EnableDataLIFRefresh
	config.EnableDataLIFRefresh = enabled
	t.Cleanup(func() { config.EnableDataLIFRefresh = previous })
}

func tridentBackendWithDataLIFs(backendUUID string, dataLIFs ...string) *tridentv1.TridentBackend {
	backend := createTestTridentBackend("tbe-"+backendUUID, "trident", backendUUID)
	if dataLIFs != nil {
		backend.DiscoveredState = &tridentv1.TridentBackendDiscoveredState{DataLIFs: &dataLIFs}
	}
	return backend
}

func tridentBackendWithEmptyDataLIFs(backendUUID string) *tridentv1.TridentBackend {
	backend := createTestTridentBackend("tbe-"+backendUUID, "trident", backendUUID)
	backend.DiscoveredState = &tridentv1.TridentBackendDiscoveredState{DataLIFs: &[]string{}}
	return backend
}

func TestTridentBackendHandlers_EnqueueDataLIFsChange(t *testing.T) {
	const backendUUID = "backend-uuid"

	tests := []struct {
		name         string
		enabled      bool
		event        func(c *TridentCrdController)
		expectQueued bool
	}{
		{
			name:    "first snapshot published",
			enabled: true,
			event: func(c *TridentCrdController) {
				c.updateTridentBackendHandler(tridentBackendWithDataLIFs(backendUUID),
					tridentBackendWithDataLIFs(backendUUID, "192.0.2.10", "192.0.2.11"))
			},
			expectQueued: true,
		},
		{
			name:    "snapshot changed",
			enabled: true,
			event: func(c *TridentCrdController) {
				c.updateTridentBackendHandler(tridentBackendWithDataLIFs(backendUUID, "192.0.2.10"),
					tridentBackendWithDataLIFs(backendUUID, "192.0.2.10", "192.0.2.11"))
			},
			expectQueued: true,
		},
		{
			name:    "empty snapshot is a change",
			enabled: true,
			event: func(c *TridentCrdController) {
				c.updateTridentBackendHandler(tridentBackendWithDataLIFs(backendUUID, "192.0.2.10"),
					tridentBackendWithEmptyDataLIFs(backendUUID))
			},
			expectQueued: true,
		},
		{
			name:    "snapshot unchanged",
			enabled: true,
			event: func(c *TridentCrdController) {
				c.updateTridentBackendHandler(tridentBackendWithDataLIFs(backendUUID, "192.0.2.10"),
					tridentBackendWithDataLIFs(backendUUID, "192.0.2.10"))
			},
		},
		{
			name:    "no snapshot published",
			enabled: true,
			event: func(c *TridentCrdController) {
				c.updateTridentBackendHandler(tridentBackendWithDataLIFs(backendUUID),
					tridentBackendWithDataLIFs(backendUUID))
			},
		},
		{
			name:    "added backend with snapshot",
			enabled: true,
			event: func(c *TridentCrdController) {
				c.addTridentBackendHandler(tridentBackendWithDataLIFs(backendUUID, "192.0.2.10"))
			},
			expectQueued: true,
		},
		{
			name:    "added backend without snapshot",
			enabled: true,
			event: func(c *TridentCrdController) {
				c.addTridentBackendHandler(tridentBackendWithDataLIFs(backendUUID))
			},
		},
		{
			name:    "data LIF refresh disabled",
			enabled: false,
			event: func(c *TridentCrdController) {
				c.updateTridentBackendHandler(tridentBackendWithDataLIFs(backendUUID, "192.0.2.10"),
					tridentBackendWithDataLIFs(backendUUID, "192.0.2.11"))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			controller, mockCtrl, _ := setupBackendConfigTest(t)
			defer mockCtrl.Finish()
			setDataLIFRefresh(t, tc.enabled)

			tc.event(controller)

			if !tc.expectQueued {
				assert.Zero(t, controller.dataLIFWorkqueue.Len())
				return
			}
			require.Equal(t, 1, controller.dataLIFWorkqueue.Len())
			item, shutdown := controller.dataLIFWorkqueue.Get()
			require.False(t, shutdown)
			assert.Equal(t, backendUUID, item)
			assert.Zero(t, controller.workqueue.Len(), "data LIF changes must stay off the main workqueue")
		})
	}
}

func TestPublishedTridentBackendDataLIFs(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	indexer := controller.crdInformer.TridentBackends().Informer().GetIndexer()
	require.NoError(t, indexer.Add(tridentBackendWithDataLIFs("published", "192.0.2.10", "192.0.2.11")))
	require.NoError(t, indexer.Add(tridentBackendWithEmptyDataLIFs("empty")))
	require.NoError(t, indexer.Add(tridentBackendWithDataLIFs("unpublished")))

	dataLIFs, err := controller.publishedTridentBackendDataLIFs("published")
	require.NoError(t, err)
	assert.Equal(t, []string{"192.0.2.10", "192.0.2.11"}, dataLIFs)

	dataLIFs, err = controller.publishedTridentBackendDataLIFs("empty")
	require.NoError(t, err)
	require.NotNil(t, dataLIFs)
	assert.Empty(t, dataLIFs)

	dataLIFs, err = controller.publishedTridentBackendDataLIFs("unpublished")
	require.NoError(t, err)
	assert.Nil(t, dataLIFs)

	dataLIFs, err = controller.publishedTridentBackendDataLIFs("missing")
	require.NoError(t, err)
	assert.Nil(t, dataLIFs)
}

func TestHandleBackendDataLIFsChange_NothingPublished(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	require.NoError(t, controller.crdInformer.TridentBackends().Informer().GetIndexer().Add(
		tridentBackendWithDataLIFs("unpublished")))

	// Returns before touching the VolumeAttachment indexer, which this controller does not have.
	assert.NoError(t, controller.handleBackendDataLIFsChange(context.Background(), "unpublished"))
	assert.Error(t, controller.handleBackendDataLIFsChange(context.Background(), ""))
}
