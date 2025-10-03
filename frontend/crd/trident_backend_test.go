// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
