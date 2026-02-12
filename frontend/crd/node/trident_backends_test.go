// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mockFrontendAutogrow "github.com/netapp/trident/mocks/mock_frontend/mock_autogrow"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
)

func TestHandleTridentBackends_Success(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a test TridentBackend
	backend := createTestTridentBackend("test-backend", testNamespace, "backend-uuid-123")

	// Add it to the fake clientset so the lister can find it
	_, err = controller.crdClientset.TridentV1().TridentBackends(testNamespace).Create(
		context.Background(), backend, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to informer indexer
	err = controller.tridentNSCrdInformerFactory.Trident().V1().TridentBackends().Informer().GetIndexer().Add(backend)
	require.NoError(t, err)

	// Create a KeyItem for processing
	keyItem := &KeyItem{
		key:        testNamespace + "/" + backend.Name,
		objectType: ObjectTypeTridentBackend,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// Handle the TridentBackend
	err = controller.handleTridentBackends(keyItem)
	assert.NoError(t, err, "should handle TridentBackend successfully")
}

func TestHandleTridentBackends_InvalidKey(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem with invalid key format
	keyItem := &KeyItem{
		key:        "invalid-key-format",
		objectType: ObjectTypeTridentBackend,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// Handle should return error for invalid keys
	err = controller.handleTridentBackends(keyItem)
	assert.Error(t, err, "should error when no namespace is present in the key")
}

func TestHandleTridentBackends_NotFound(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem for a non-existent object
	keyItem := &KeyItem{
		key:        testNamespace + "/non-existent-backend",
		objectType: ObjectTypeTridentBackend,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// Handle should return error for non-existent object (except for delete events)
	err = controller.handleTridentBackends(keyItem)
	assert.Error(t, err, "should error when object not found for non-delete event")
}

func TestHandleTridentBackends_DeleteEvent_NotFound(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem for a non-existent object with delete event
	keyItem := &KeyItem{
		key:        testNamespace + "/deleted-backend",
		objectType: ObjectTypeTridentBackend,
		event:      EventDelete,
		ctx:        context.Background(),
	}

	// Handle should return nil for delete events of non-existent objects
	err = controller.handleTridentBackends(keyItem)
	assert.NoError(t, err, "should handle delete event of non-existent object gracefully")
}

func TestHandleTridentBackends_AllEventTypes(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a test TridentBackend
	backend := createTestTridentBackend("test-backend", testNamespace, "backend-uuid-456")

	// Add it to the fake clientset
	_, err = controller.crdClientset.TridentV1().TridentBackends(testNamespace).Create(
		context.Background(), backend, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to informer indexer
	err = controller.tridentNSCrdInformerFactory.Trident().V1().TridentBackends().Informer().GetIndexer().Add(backend)
	require.NoError(t, err)

	// Test each event type
	eventTypes := []EventType{EventAdd, EventUpdate, EventDelete}

	for _, eventType := range eventTypes {
		t.Run(string(eventType), func(t *testing.T) {
			keyItem := &KeyItem{
				key:        testNamespace + "/" + backend.Name,
				objectType: ObjectTypeTridentBackend,
				event:      eventType,
				ctx:        context.Background(),
			}

			err = controller.handleTridentBackends(keyItem)
			assert.NoError(t, err, "should handle %s event successfully", eventType)
		})
	}
}

func TestHandleTridentBackends_EdgeCases(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	tests := []struct {
		name        string
		keyItem     *KeyItem
		expectError bool
		description string
	}{
		{
			name: "empty namespace",
			keyItem: &KeyItem{
				key:        "/empty-namespace",
				objectType: ObjectTypeTridentBackend,
				event:      EventAdd,
				ctx:        context.Background(),
			},
			expectError: true,
			description: "should handle empty namespace",
		},
		{
			name: "empty name",
			keyItem: &KeyItem{
				key:        testNamespace + "/",
				objectType: ObjectTypeTridentBackend,
				event:      EventAdd,
				ctx:        context.Background(),
			},
			expectError: true,
			description: "should handle empty name",
		},
		{
			name: "valid delete event for missing object",
			keyItem: &KeyItem{
				key:        testNamespace + "/missing-backend",
				objectType: ObjectTypeTridentBackend,
				event:      EventDelete,
				ctx:        context.Background(),
			},
			expectError: false,
			description: "should handle delete event for missing object gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := controller.handleTridentBackends(tt.keyItem)
			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}

func TestHandleTridentBackends_RetryScenario(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a TridentBackend
	backend := createTestTridentBackend("retry-backend", testNamespace, "backend-retry-uuid")

	// Add it to the fake clientset
	_, err = controller.crdClientset.TridentV1().TridentBackends(testNamespace).Create(
		context.Background(), backend, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to informer indexer
	err = controller.tridentNSCrdInformerFactory.Trident().V1().TridentBackends().Informer().GetIndexer().Add(backend)
	require.NoError(t, err)

	// Create KeyItem with retry flag
	keyItem := &KeyItem{
		key:        testNamespace + "/" + backend.Name,
		objectType: ObjectTypeTridentBackend,
		event:      EventUpdate,
		ctx:        context.Background(),
		isRetry:    true, // This is a retry
	}

	err = controller.handleTridentBackends(keyItem)
	assert.NoError(t, err, "should handle retry scenario successfully")
}

func TestHandleTridentBackends_InvalidCacheKey(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem with a key that will fail SplitMetaNamespaceKey
	keyItem := &KeyItem{
		key:        "invalid/key/with/too/many/slashes",
		objectType: ObjectTypeTridentBackend,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	err = controller.handleTridentBackends(keyItem)
	assert.Error(t, err, "should error for malformed cache key")
}

func TestHandleTridentBackends_MissingNamespace(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem with no namespace (invalid for TridentBackend)
	keyItem := &KeyItem{
		key:        "backend-without-namespace",
		objectType: ObjectTypeTridentBackend,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	err = controller.handleTridentBackends(keyItem)
	assert.Error(t, err, "should error when namespace is missing")
	assert.Contains(t, err.Error(), "no namespace present", "error should mention missing namespace")
}

// Helper function to create a test TridentBackend
func createTestTridentBackend(name, namespace, backendUUID string) *tridentv1.TridentBackend {
	return &tridentv1.TridentBackend{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		BackendName: name,
		BackendUUID: backendUUID,
		Version:     "1",
		Online:      true,
		State:       "online",
	}
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Orchestrator Integration Tests

func TestHandleTridentBackends_OrchestratorCalls(t *testing.T) {
	tests := []struct {
		name                   string
		setupBackend           func() *tridentv1.TridentBackend
		event                  EventType
		expectOrchestratorCall bool
		expectedKey            string // What key should be passed to orchestrator
		description            string
	}{
		{
			name: "AddEvent_CallsOrchestratorWithFullKey",
			setupBackend: func() *tridentv1.TridentBackend {
				return createTestTridentBackend("test-backend", testNamespace, "backend-uuid-123")
			},
			event:                  EventAdd,
			expectOrchestratorCall: true,
			expectedKey:            testNamespace + "/test-backend", // Full key
			description:            "Add event should call orchestrator with full key (namespace/name)",
		},
		{
			name: "UpdateEvent_CallsOrchestratorWithFullKey",
			setupBackend: func() *tridentv1.TridentBackend {
				return createTestTridentBackend("update-backend", testNamespace, "backend-uuid-456")
			},
			event:                  EventUpdate,
			expectOrchestratorCall: true,
			expectedKey:            testNamespace + "/update-backend", // Full key
			description:            "Update event should call orchestrator with full key (namespace/name)",
		},
		{
			name: "DeleteEvent_ExistingBackend_CallsOrchestratorWithFullKey",
			setupBackend: func() *tridentv1.TridentBackend {
				return createTestTridentBackend("delete-backend", testNamespace, "backend-uuid-789")
			},
			event:                  EventDelete,
			expectOrchestratorCall: true,
			expectedKey:            testNamespace + "/delete-backend", // Full key for existing object
			description:            "Delete event for existing backend should call orchestrator with full key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gomock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mock orchestrator
			mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

			// Setup controller
			controller, err := newTestTridentNodeCrdController(t, testNodeName)
			require.NoError(t, err)

			// Replace orchestrator with mock
			controller.autogrowOrchestrator = mockOrchestrator

			// Setup backend
			backend := tt.setupBackend()

			// Add to fake clientset
			_, err = controller.crdClientset.TridentV1().TridentBackends(testNamespace).Create(
				context.Background(), backend, metav1.CreateOptions{})
			require.NoError(t, err)

			// Add to indexer
			err = controller.tridentNSCrdInformerFactory.Trident().V1().TridentBackends().Informer().GetIndexer().Add(backend)
			require.NoError(t, err)

			// Setup expectation
			if tt.expectOrchestratorCall {
				mockOrchestrator.EXPECT().
					HandleTBEEvent(gomock.Any(), tt.event, tt.expectedKey).
					Times(1)
			}

			// Create KeyItem
			keyItem := &KeyItem{
				key:        testNamespace + "/" + backend.Name,
				objectType: ObjectTypeTridentBackend,
				event:      tt.event,
				ctx:        context.Background(),
			}

			// Execute
			err = controller.handleTridentBackends(keyItem)
			assert.NoError(t, err, tt.description)
		})
	}
}

func TestHandleTridentBackends_DeleteEvent_NotFound_CallsOrchestratorWithFullKey(t *testing.T) {
	// Create gomock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock orchestrator
	mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

	// Setup controller
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Replace orchestrator with mock
	controller.autogrowOrchestrator = mockOrchestrator

	// Backend name
	backendName := "deleted-backend"

	// Expect orchestrator to be called with full key for delete of non-existent object
	mockOrchestrator.EXPECT().
		HandleTBEEvent(gomock.Any(), EventDelete, testNamespace+"/"+backendName).
		Times(1)

	// Create KeyItem for non-existent object with delete event
	keyItem := &KeyItem{
		key:        testNamespace + "/" + backendName,
		objectType: ObjectTypeTridentBackend,
		event:      EventDelete,
		ctx:        context.Background(),
	}

	// Execute - should succeed and call orchestrator with full key
	err = controller.handleTridentBackends(keyItem)
	assert.NoError(t, err, "should handle delete event of non-existent object and call orchestrator with full key")
}

func TestHandleTridentBackends_AllEvents_VerifyOrchestratorCalls(t *testing.T) {
	eventTests := []struct {
		name        string
		event       EventType
		expectedKey string
		description string
	}{
		{
			name:        "Add",
			event:       EventAdd,
			expectedKey: testNamespace + "/test-backend",
			description: "Add event should pass full key",
		},
		{
			name:        "Update",
			event:       EventUpdate,
			expectedKey: testNamespace + "/test-backend",
			description: "Update event should pass full key",
		},
		{
			name:        "Delete",
			event:       EventDelete,
			expectedKey: testNamespace + "/test-backend",
			description: "Delete event for existing object should pass full key",
		},
	}

	for _, tt := range eventTests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gomock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Create mock orchestrator
			mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

			// Setup controller
			controller, err := newTestTridentNodeCrdController(t, testNodeName)
			require.NoError(t, err)

			// Replace orchestrator with mock
			controller.autogrowOrchestrator = mockOrchestrator

			// Create backend
			backend := createTestTridentBackend("test-backend", testNamespace, "backend-uuid-test")

			// Add to fake clientset
			_, err = controller.crdClientset.TridentV1().TridentBackends(testNamespace).Create(
				context.Background(), backend, metav1.CreateOptions{})
			require.NoError(t, err)

			// Add to indexer
			err = controller.tridentNSCrdInformerFactory.Trident().V1().TridentBackends().Informer().GetIndexer().Add(backend)
			require.NoError(t, err)

			// Expect orchestrator call with correct parameters
			mockOrchestrator.EXPECT().
				HandleTBEEvent(gomock.Any(), tt.event, tt.expectedKey).
				Times(1)

			// Create KeyItem
			keyItem := &KeyItem{
				key:        tt.expectedKey,
				objectType: ObjectTypeTridentBackend,
				event:      tt.event,
				ctx:        context.Background(),
			}

			// Execute
			err = controller.handleTridentBackends(keyItem)
			assert.NoError(t, err, tt.description)
		})
	}
}

func TestHandleTridentBackends_MultipleConcurrentEvents(t *testing.T) {
	// Create gomock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock orchestrator
	mockOrchestrator := mockFrontendAutogrow.NewMockAutogrowOrchestrator(ctrl)

	// Setup controller
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Replace orchestrator with mock
	controller.autogrowOrchestrator = mockOrchestrator

	// Create multiple backends
	backend1 := createTestTridentBackend("backend-1", testNamespace, "uuid-1")
	backend2 := createTestTridentBackend("backend-2", testNamespace, "uuid-2")
	backend3 := createTestTridentBackend("backend-3", testNamespace, "uuid-3")

	backends := []*tridentv1.TridentBackend{backend1, backend2, backend3}

	// Add all to fake clientset and indexer
	for _, backend := range backends {
		_, err = controller.crdClientset.TridentV1().TridentBackends(testNamespace).Create(
			context.Background(), backend, metav1.CreateOptions{})
		require.NoError(t, err)

		err = controller.tridentNSCrdInformerFactory.Trident().V1().TridentBackends().Informer().GetIndexer().Add(backend)
		require.NoError(t, err)
	}

	// Expect orchestrator to be called for each backend
	mockOrchestrator.EXPECT().
		HandleTBEEvent(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(3)

	// Process all backends
	for _, backend := range backends {
		keyItem := &KeyItem{
			key:        testNamespace + "/" + backend.Name,
			objectType: ObjectTypeTridentBackend,
			event:      EventAdd,
			ctx:        context.Background(),
		}

		err = controller.handleTridentBackends(keyItem)
		assert.NoError(t, err, "should handle backend %s successfully", backend.Name)
	}
}
