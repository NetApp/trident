// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	mockFrontendAutogrow "github.com/netapp/trident/mocks/mock_frontend/mock_autogrow"
)

func TestHandleStorageClasses_Success(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a test StorageClass
	sc := createTestStorageClass("test-sc", "csi.trident.netapp.io")

	// Add it to the fake clientset so the informer cache can find it
	_, err = controller.kubeClientset.StorageV1().StorageClasses().Create(
		context.Background(), sc, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to informer indexer
	err = controller.storageClassInformer.GetIndexer().Add(sc)
	require.NoError(t, err)

	// Create a KeyItem for processing (cluster-scoped resources have empty namespace in key)
	keyItem := &KeyItem{
		key:        sc.Name,
		objectType: ObjectTypeStorageClass,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// Handle the StorageClass
	err = controller.handleStorageClasses(keyItem)
	assert.NoError(t, err, "should handle StorageClass successfully")
}

func TestHandleStorageClasses_InvalidKey(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem with invalid key format
	keyItem := &KeyItem{
		key:        "", // Empty key
		objectType: ObjectTypeStorageClass,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// Handle should return error for invalid keys
	err = controller.handleStorageClasses(keyItem)
	assert.Error(t, err, "should error for empty key")
}

func TestHandleStorageClasses_NotFound(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem for a non-existent object
	keyItem := &KeyItem{
		key:        "non-existent-sc",
		objectType: ObjectTypeStorageClass,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// Handle should return error for non-existent object (except for delete events)
	err = controller.handleStorageClasses(keyItem)
	assert.Error(t, err, "should error when object not found for non-delete event")
}

func TestHandleStorageClasses_DeleteEvent_NotFound(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem for a non-existent object with delete event
	keyItem := &KeyItem{
		key:        "deleted-sc",
		objectType: ObjectTypeStorageClass,
		event:      EventDelete,
		ctx:        context.Background(),
	}

	// Handle should return nil for delete events of non-existent objects
	err = controller.handleStorageClasses(keyItem)
	assert.NoError(t, err, "should handle delete event of non-existent object gracefully")
}

func TestHandleStorageClasses_AllEventTypes(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a test StorageClass
	sc := createTestStorageClass("test-sc-events", "csi.trident.netapp.io")

	// Add it to the fake clientset
	_, err = controller.kubeClientset.StorageV1().StorageClasses().Create(
		context.Background(), sc, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to informer indexer
	err = controller.storageClassInformer.GetIndexer().Add(sc)
	require.NoError(t, err)

	// Test each event type
	eventTypes := []EventType{EventAdd, EventUpdate, EventDelete}

	for _, eventType := range eventTypes {
		t.Run(string(eventType), func(t *testing.T) {
			keyItem := &KeyItem{
				key:        sc.Name,
				objectType: ObjectTypeStorageClass,
				event:      eventType,
				ctx:        context.Background(),
			}

			err = controller.handleStorageClasses(keyItem)
			assert.NoError(t, err, "should handle %s event successfully", eventType)
		})
	}
}

func TestHandleStorageClasses_EdgeCases(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	tests := []struct {
		name        string
		keyItem     *KeyItem
		expectError bool
		description string
	}{
		{
			name: "key with namespace separator",
			keyItem: &KeyItem{
				key:        "some-namespace/sc-name", // Unusual but valid for cluster-scoped
				objectType: ObjectTypeStorageClass,
				event:      EventAdd,
				ctx:        context.Background(),
			},
			expectError: true,
			description: "should handle key with namespace separator",
		},
		{
			name: "valid delete event for missing object",
			keyItem: &KeyItem{
				key:        "missing-sc",
				objectType: ObjectTypeStorageClass,
				event:      EventDelete,
				ctx:        context.Background(),
			},
			expectError: false,
			description: "should handle delete event for missing object gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := controller.handleStorageClasses(tt.keyItem)
			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}

func TestHandleStorageClasses_RetryScenario(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a StorageClass
	sc := createTestStorageClass("retry-sc", "csi.trident.netapp.io")

	// Add it to the fake clientset
	_, err = controller.kubeClientset.StorageV1().StorageClasses().Create(
		context.Background(), sc, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to informer indexer
	err = controller.storageClassInformer.GetIndexer().Add(sc)
	require.NoError(t, err)

	// Create KeyItem with retry flag
	keyItem := &KeyItem{
		key:        sc.Name,
		objectType: ObjectTypeStorageClass,
		event:      EventUpdate,
		ctx:        context.Background(),
		isRetry:    true, // This is a retry
	}

	err = controller.handleStorageClasses(keyItem)
	assert.NoError(t, err, "should handle retry scenario successfully")
}

func TestHandleStorageClasses_DifferentProvisioners(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test with various provisioners
	provisioners := []string{
		"csi.trident.netapp.io",
		"kubernetes.io/no-provisioner",
		"ebs.csi.aws.com",
		"pd.csi.storage.gke.io",
	}

	for _, provisioner := range provisioners {
		t.Run(provisioner, func(t *testing.T) {
			sc := createTestStorageClass("sc-"+provisioner, provisioner)

			_, err := controller.kubeClientset.StorageV1().StorageClasses().Create(
				context.Background(), sc, metav1.CreateOptions{})
			require.NoError(t, err)

			// Add to informer indexer
			err = controller.storageClassInformer.GetIndexer().Add(sc)
			require.NoError(t, err)

			keyItem := &KeyItem{
				key:        sc.Name,
				objectType: ObjectTypeStorageClass,
				event:      EventAdd,
				ctx:        context.Background(),
			}

			err = controller.handleStorageClasses(keyItem)
			assert.NoError(t, err, "should handle StorageClass with provisioner %s", provisioner)
		})
	}
}

func TestHandleStorageClasses_WithParameters(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a StorageClass with parameters
	sc := createTestStorageClass("sc-with-params", "csi.trident.netapp.io")
	sc.Parameters = map[string]string{
		"backendType": "ontap-nas",
		"fsType":      "ext4",
		"selector":    "performance=platinum",
	}

	_, err = controller.kubeClientset.StorageV1().StorageClasses().Create(
		context.Background(), sc, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to informer indexer
	err = controller.storageClassInformer.GetIndexer().Add(sc)
	require.NoError(t, err)

	keyItem := &KeyItem{
		key:        sc.Name,
		objectType: ObjectTypeStorageClass,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	err = controller.handleStorageClasses(keyItem)
	assert.NoError(t, err, "should handle StorageClass with parameters")
}

func TestHandleStorageClasses_InvalidCacheKey(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem with a key that will fail SplitMetaNamespaceKey
	keyItem := &KeyItem{
		key:        "invalid/key/with/too/many/slashes",
		objectType: ObjectTypeStorageClass,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	err = controller.handleStorageClasses(keyItem)
	assert.Error(t, err, "should error for malformed cache key")
}

func TestHandleStorageClasses_WrongObjectType(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Add a wrong type of object to the indexer
	wrongObj := &metav1.ObjectMeta{
		Name: "wrong-type",
	}
	err = controller.storageClassInformer.GetIndexer().Add(wrongObj)
	require.NoError(t, err)

	keyItem := &KeyItem{
		key:        "wrong-type",
		objectType: ObjectTypeStorageClass,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	err = controller.handleStorageClasses(keyItem)
	assert.Error(t, err, "should error for wrong object type")
	assert.Contains(t, err.Error(), "unexpected object type", "error should mention unexpected type")
}

func TestHandleStorageClasses_ClusterScopedNamespace(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a StorageClass (cluster-scoped)
	sc := createTestStorageClass("cluster-sc", "csi.trident.netapp.io")

	_, err = controller.kubeClientset.StorageV1().StorageClasses().Create(
		context.Background(), sc, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to informer indexer
	err = controller.storageClassInformer.GetIndexer().Add(sc)
	require.NoError(t, err)

	// For cluster-scoped resources, the key has no namespace prefix
	keyItem := &KeyItem{
		key:        sc.Name,
		objectType: ObjectTypeStorageClass,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	err = controller.handleStorageClasses(keyItem)
	assert.NoError(t, err, "should handle cluster-scoped StorageClass")
}

// Helper function to create a test StorageClass
func createTestStorageClass(name, provisioner string) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner: provisioner,
	}
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Orchestrator Integration Tests
// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func TestHandleStorageClasses_OrchestratorCalls(t *testing.T) {
	tests := []struct {
		name                   string
		setupSC                func() *storagev1.StorageClass
		event                  EventType
		expectOrchestratorCall bool
		expectedName           string // What name should be passed to orchestrator
		description            string
	}{
		{
			name: "AddEvent_CallsOrchestratorWithName",
			setupSC: func() *storagev1.StorageClass {
				return createTestStorageClass("test-sc", "csi.trident.netapp.io")
			},
			event:                  EventAdd,
			expectOrchestratorCall: true,
			expectedName:           "test-sc",
			description:            "Add event should call orchestrator with name",
		},
		{
			name: "UpdateEvent_CallsOrchestratorWithName",
			setupSC: func() *storagev1.StorageClass {
				return createTestStorageClass("update-sc", "csi.trident.netapp.io")
			},
			event:                  EventUpdate,
			expectOrchestratorCall: true,
			expectedName:           "update-sc",
			description:            "Update event should call orchestrator with name",
		},
		{
			name: "DeleteEvent_ExistingSC_CallsOrchestratorWithName",
			setupSC: func() *storagev1.StorageClass {
				return createTestStorageClass("delete-sc", "csi.trident.netapp.io")
			},
			event:                  EventDelete,
			expectOrchestratorCall: true,
			expectedName:           "delete-sc",
			description:            "Delete event for existing SC should call orchestrator with name",
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

			// Setup StorageClass
			sc := tt.setupSC()

			// Add to fake clientset
			_, err = controller.kubeClientset.StorageV1().StorageClasses().Create(
				context.Background(), sc, metav1.CreateOptions{})
			require.NoError(t, err)

			// Add to indexer
			err = controller.storageClassInformer.GetIndexer().Add(sc)
			require.NoError(t, err)

			// Setup expectation
			if tt.expectOrchestratorCall {
				mockOrchestrator.EXPECT().
					HandleStorageClassEvent(gomock.Any(), tt.event, tt.expectedName).
					Times(1)
			}

			// Create KeyItem (no namespace for cluster-scoped resources)
			keyItem := &KeyItem{
				key:        sc.Name,
				objectType: ObjectTypeStorageClass,
				event:      tt.event,
				ctx:        context.Background(),
			}

			// Execute
			err = controller.handleStorageClasses(keyItem)
			assert.NoError(t, err, tt.description)
		})
	}
}

func TestHandleStorageClasses_DeleteEvent_NotFound_CallsOrchestratorWithName(t *testing.T) {
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

	// StorageClass name
	scName := "deleted-sc"

	// Expect orchestrator to be called with just the name for delete of non-existent object
	mockOrchestrator.EXPECT().
		HandleStorageClassEvent(gomock.Any(), EventDelete, scName).
		Times(1)

	// Create KeyItem for non-existent object with delete event
	keyItem := &KeyItem{
		key:        scName,
		objectType: ObjectTypeStorageClass,
		event:      EventDelete,
		ctx:        context.Background(),
	}

	// Execute - should succeed and call orchestrator with name only
	err = controller.handleStorageClasses(keyItem)
	assert.NoError(t, err, "should handle delete event of non-existent object and call orchestrator with name")
}

func TestHandleStorageClasses_AllEvents_VerifyOrchestratorCalls(t *testing.T) {
	eventTests := []struct {
		name         string
		event        EventType
		expectedName string
		description  string
	}{
		{
			name:         "Add",
			event:        EventAdd,
			expectedName: "test-sc",
			description:  "Add event should pass name",
		},
		{
			name:         "Update",
			event:        EventUpdate,
			expectedName: "test-sc",
			description:  "Update event should pass name",
		},
		{
			name:         "Delete",
			event:        EventDelete,
			expectedName: "test-sc",
			description:  "Delete event for existing object should pass name",
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

			// Create StorageClass
			sc := createTestStorageClass("test-sc", "csi.trident.netapp.io")

			// Add to fake clientset
			_, err = controller.kubeClientset.StorageV1().StorageClasses().Create(
				context.Background(), sc, metav1.CreateOptions{})
			require.NoError(t, err)

			// Add to indexer
			err = controller.storageClassInformer.GetIndexer().Add(sc)
			require.NoError(t, err)

			// Expect orchestrator call with correct parameters
			mockOrchestrator.EXPECT().
				HandleStorageClassEvent(gomock.Any(), tt.event, tt.expectedName).
				Times(1)

			// Create KeyItem
			keyItem := &KeyItem{
				key:        tt.expectedName,
				objectType: ObjectTypeStorageClass,
				event:      tt.event,
				ctx:        context.Background(),
			}

			// Execute
			err = controller.handleStorageClasses(keyItem)
			assert.NoError(t, err, tt.description)
		})
	}
}

func TestHandleStorageClasses_MultipleConcurrentEvents(t *testing.T) {
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

	// Create multiple StorageClasses
	sc1 := createTestStorageClass("sc-1", "csi.trident.netapp.io")
	sc2 := createTestStorageClass("sc-2", "csi.trident.netapp.io")
	sc3 := createTestStorageClass("sc-3", "kubernetes.io/no-provisioner")

	storageClasses := []*storagev1.StorageClass{sc1, sc2, sc3}

	// Add all to fake clientset and indexer
	for _, sc := range storageClasses {
		_, err = controller.kubeClientset.StorageV1().StorageClasses().Create(
			context.Background(), sc, metav1.CreateOptions{})
		require.NoError(t, err)

		err = controller.storageClassInformer.GetIndexer().Add(sc)
		require.NoError(t, err)
	}

	// Expect orchestrator to be called for each StorageClass
	mockOrchestrator.EXPECT().
		HandleStorageClassEvent(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(3)

	// Process all StorageClasses
	for _, sc := range storageClasses {
		keyItem := &KeyItem{
			key:        sc.Name,
			objectType: ObjectTypeStorageClass,
			event:      EventAdd,
			ctx:        context.Background(),
		}

		err = controller.handleStorageClasses(keyItem)
		assert.NoError(t, err, "should handle StorageClass %s successfully", sc.Name)
	}
}

func TestHandleStorageClasses_DifferentProvisioners_OrchestratorCalls(t *testing.T) {
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

	// Test with various provisioners - orchestrator should be called for all
	provisioners := []string{
		"csi.trident.netapp.io",
		"kubernetes.io/no-provisioner",
		"ebs.csi.aws.com",
	}

	// Expect orchestrator to be called for each provisioner
	mockOrchestrator.EXPECT().
		HandleStorageClassEvent(gomock.Any(), EventAdd, gomock.Any()).
		Times(len(provisioners))

	for _, provisioner := range provisioners {
		sc := createTestStorageClass("sc-"+provisioner, provisioner)

		_, err := controller.kubeClientset.StorageV1().StorageClasses().Create(
			context.Background(), sc, metav1.CreateOptions{})
		require.NoError(t, err)

		err = controller.storageClassInformer.GetIndexer().Add(sc)
		require.NoError(t, err)

		keyItem := &KeyItem{
			key:        sc.Name,
			objectType: ObjectTypeStorageClass,
			event:      EventAdd,
			ctx:        context.Background(),
		}

		err = controller.handleStorageClasses(keyItem)
		assert.NoError(t, err, "should handle StorageClass with provisioner %s", provisioner)
	}
}
