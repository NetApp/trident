// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/netapp/trident/logging"
	mockcore "github.com/netapp/trident/mocks/mock_core"
)

func createTestSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Generation:      1,
			ResourceVersion: "123",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("secret"),
		},
	}
}

func TestUpdateSecretHandler_Success(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	if err != nil {
		t.Fatalf("cannot create Trident CRD controller frontend, error: %v", err.Error())
	}

	// Create test secrets with different generations
	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret",
			Namespace:       "default",
			Generation:      1,
			ResourceVersion: "1000",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("password123"),
		},
	}

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret",
			Namespace:       "default",
			Generation:      2,
			ResourceVersion: "1001",
		},
		Data: map[string][]byte{
			"username": []byte("admin"),
			"password": []byte("newpassword123"),
		},
	}

	// Test updateSecretHandler with generation change
	crdController.updateSecretHandler(oldSecret, newSecret)

	// Verify that an event was added to the workqueue
	assert.Greater(t, crdController.workqueue.Len(), 0)

	// Get the item and verify it's correct
	item, shutdown := crdController.workqueue.Get()
	assert.False(t, shutdown)

	keyItem, ok := item.(KeyItem)
	assert.True(t, ok)
	assert.Equal(t, EventUpdate, keyItem.event)
	assert.Equal(t, ObjectTypeSecret, keyItem.objectType)
	assert.Equal(t, "default/test-secret", keyItem.key)

	crdController.workqueue.Done(item)
}

func TestUpdateSecretHandler_NoGenerationChange(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	oldSecret := createTestSecret("test-secret", "default")
	oldSecret.Generation = 1
	newSecret := createTestSecret("test-secret", "default")
	newSecret.Generation = 1 // Same generation
	newSecret.ResourceVersion = "124"

	initialQueueLen := controller.workqueue.Len()

	// This should not add item to work queue
	controller.updateSecretHandler(oldSecret, newSecret)

	// Check if work item was NOT added (when generation doesn't change)
	assert.Equal(t, initialQueueLen, controller.workqueue.Len())
}

func TestUpdateSecretHandler_NoResourceVersionChange(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	oldSecret := createTestSecret("test-secret", "default")
	newSecret := createTestSecret("test-secret", "default")
	// Same resource version

	initialQueueLen := controller.workqueue.Len()

	// This should not add item to work queue
	controller.updateSecretHandler(oldSecret, newSecret)

	// Check if work item was NOT added
	assert.Equal(t, initialQueueLen, controller.workqueue.Len())
}

func TestHandleSecret_BackendConfigFound(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()

	keyItem := &KeyItem{
		key:        "default/test-secret",
		event:      EventUpdate,
		ctx:        ctx,
		objectType: ObjectTypeSecret,
	}

	err := controller.handleSecret(keyItem)
	assert.NoError(t, err)
}

func TestHandleSecret_BackendConfigNotFound(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	keyItem := &KeyItem{
		key:        "default/non-existent-secret",
		event:      EventUpdate,
		ctx:        ctx,
		objectType: ObjectTypeSecret,
	}

	err := controller.handleSecret(keyItem)
	assert.NoError(t, err) // Should not error when no backend configs found
}

func TestHandleSecret_WrongEventType(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	keyItem := &KeyItem{
		key:        "default/test-secret",
		event:      EventDelete, // Wrong event type
		ctx:        ctx,
		objectType: ObjectTypeSecret,
	}

	err := controller.handleSecret(keyItem)
	assert.NoError(t, err) // Should not error but should do nothing
}

func TestHandleSecret_InvalidKey(t *testing.T) {
	controller, mockCtrl, _ := setupBackendConfigTest(t)
	defer mockCtrl.Finish()

	ctx := context.Background()
	keyItem := &KeyItem{
		key:        "invalid-key-format",
		event:      EventUpdate,
		ctx:        ctx,
		objectType: ObjectTypeSecret,
	}

	err := controller.handleSecret(keyItem)
	assert.NoError(t, err) // Should handle invalid key gracefully
}

// Additional comprehensive secret handler tests
func TestUpdateSecretHandler_EdgeCases(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	assert.NoError(t, err)

	// Test case 1: Valid secret update with generation change
	oldSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret",
			Namespace:       "default",
			Generation:      1,
			ResourceVersion: "1000",
		},
	}

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret",
			Namespace:       "default",
			Generation:      2,
			ResourceVersion: "1001",
		},
	}

	initialQueueLen := crdController.workqueue.Len()
	crdController.updateSecretHandler(oldSecret, newSecret)
	// Should add an item to the queue since generation changed
	assert.Greater(t, crdController.workqueue.Len(), initialQueueLen)

	// Drain the queue
	for crdController.workqueue.Len() > 0 {
		item, _ := crdController.workqueue.Get()
		crdController.workqueue.Done(item)
	}

	// Test case 2: Same generation (0) but different resource version
	oldSecret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret2",
			Namespace:       "default",
			Generation:      0, // Generation 0 allows resource version check
			ResourceVersion: "1000",
		},
	}

	newSecret2 := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret2",
			Namespace:       "default",
			Generation:      0,      // Same generation (0)
			ResourceVersion: "1002", // Different resource version
		},
	}

	initialQueueLen = crdController.workqueue.Len()
	crdController.updateSecretHandler(oldSecret2, newSecret2)
	// Should add an item to the queue since resource version changed
	assert.Greater(t, crdController.workqueue.Len(), initialQueueLen)
}

func TestHandleSecret_EdgeCases(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	orchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	tridentNamespace := "trident"
	kubeClient := GetTestKubernetesClientset()
	snapClient := GetTestSnapshotClientset()
	crdClient := GetTestCrdClientset()

	crdController, err := newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClient, snapClient, crdClient)
	assert.NoError(t, err)

	ctx := GenerateRequestContext(context.TODO(), "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	// Test wrong event type
	keyItem := &KeyItem{
		key:        "default/test-secret",
		event:      EventAdd, // Wrong event type
		ctx:        ctx,
		objectType: ObjectTypeSecret,
	}

	err = crdController.handleSecret(keyItem)
	assert.NoError(t, err) // Should handle gracefully

	// Test valid secret handling
	keyItem.event = EventUpdate
	err = crdController.handleSecret(keyItem)
	assert.NoError(t, err)

	// Test invalid key format
	keyItem.key = "invalid-key"
	err = crdController.handleSecret(keyItem)
	assert.NoError(t, err) // Should handle gracefully
}
