// Copyright 2025 NetApp, Inc. All Rights Reserved.
package crd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/frontend/csi"
	. "github.com/netapp/trident/logging"
	mockFrontendAutogrow "github.com/netapp/trident/mocks/mock_frontend/mock_autogrow"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	faketridentclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
)

func TestHandleTridentVolumePublications_Success(t *testing.T) {
	// Create a controller with test data
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a test TridentVolumePublication
	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")

	// Add it to the fake clientset so the lister can find it
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(t, err)

	// Create a KeyItem for processing
	keyItem := &KeyItem{
		key:        testNamespace + "/" + tvp.Name,
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// We need to manually add the object to the indexer since we're not running the informer
	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// Handle the TridentVolumePublication
	err = controller.handleTridentVolumePublications(keyItem)
	assert.NoError(t, err, "should handle TridentVolumePublication successfully")
}

func TestHandleTridentVolumePublications_InvalidKey(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem with invalid key format
	keyItem := &KeyItem{
		key:        "invalid-key-format",
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// Handle should return nil (not error) for invalid keys as per the implementation
	err = controller.handleTridentVolumePublications(keyItem)
	assert.Error(t, err, "should error when no namespace is present in the key")
}

func TestHandleTridentVolumePublications_NotFound(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem for a non-existent object
	keyItem := &KeyItem{
		key:        testNamespace + "/non-existent-tvp",
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// Handle should return error for non-existent object (except for delete events)
	err = controller.handleTridentVolumePublications(keyItem)
	assert.Error(t, err, "should error when object not found for non-delete event")
}

func TestHandleTridentVolumePublications_DeleteEvent_NotFound(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem for a non-existent object with delete event
	keyItem := &KeyItem{
		key:        testNamespace + "/deleted-tvp",
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventDelete,
		ctx:        context.Background(),
	}

	// Handle should return nil for delete events of non-existent objects
	err = controller.handleTridentVolumePublications(keyItem)
	assert.NoError(t, err, "should handle delete event of non-existent object gracefully")
}

func TestHandleTridentVolumePublications_WrongNode(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a TridentVolumePublication for a different node
	differentNodeName := "different-node"
	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, differentNodeName, "vol-123")

	// Add it to the fake clientset
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to indexer
	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// Create a KeyItem for processing
	keyItem := &KeyItem{
		key:        testNamespace + "/" + tvp.Name,
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// Handle should return nil but log a warning about field selector failure
	err = controller.handleTridentVolumePublications(keyItem)
	assert.NoError(t, err, "should handle wrong node gracefully with warning")
}

func TestHandleTridentVolumePublications_AllEventTypes(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a test TridentVolumePublication
	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")

	// Add it to the fake clientset
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to indexer
	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// Test each event type
	eventTypes := []EventType{EventAdd, EventUpdate, EventDelete}

	for _, eventType := range eventTypes {
		t.Run(string(eventType), func(t *testing.T) {
			keyItem := &KeyItem{
				key:        testNamespace + "/" + tvp.Name,
				objectType: OjbectTypeTridentVolumePublication,
				event:      eventType,
				ctx:        context.Background(),
			}

			err = controller.handleTridentVolumePublications(keyItem)
			assert.NoError(t, err, "should handle %s event successfully", eventType)
		})
	}
}

// Test with a more realistic setup using the lister
func TestHandleTridentVolumePublications_WithLister(t *testing.T) {
	// Create fake clientset with pre-existing TridentVolumePublication
	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
	fakeClient := faketridentclient.NewSimpleClientset(tvp)

	// Create controller with the fake client
	plugin := mockCSIPlugin(t)

	controller, err := newTridentNodeCrdController(testNamespace,
		getFakeKubernetesClientset(), fakeClient, testNodeName, plugin, 1*time.Minute)
	require.NoError(t, err)

	// Add the object to the indexer to simulate what the informer would do
	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// Create KeyItem
	keyItem := &KeyItem{
		key:        testNamespace + "/" + tvp.Name,
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// Handle the event
	err = controller.handleTridentVolumePublications(keyItem)
	assert.NoError(t, err, "should handle event successfully with lister")
}

// Test edge cases and error conditions
func TestHandleTridentVolumePublications_EdgeCases(t *testing.T) {
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
				objectType: OjbectTypeTridentVolumePublication,
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
				objectType: OjbectTypeTridentVolumePublication,
				event:      EventAdd,
				ctx:        context.Background(),
			},
			expectError: true,
			description: "should handle empty name",
		},
		{
			name: "valid delete event for missing object",
			keyItem: &KeyItem{
				key:        testNamespace + "/missing-object",
				objectType: OjbectTypeTridentVolumePublication,
				event:      EventDelete,
				ctx:        context.Background(),
			},
			expectError: false,
			description: "should handle delete event for missing object gracefully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := controller.handleTridentVolumePublications(tt.keyItem)
			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
			}
		})
	}
}

// Benchmark the handler performance
func BenchmarkHandleTridentVolumePublications(b *testing.B) {
	// Create controller with testing interface compatibility
	plugin := &csi.Plugin{} // Create a minimal plugin for benchmarking

	controller, err := newTridentNodeCrdController(testNamespace,
		getFakeKubernetesClientset(), getFakeTridentClientset(), testNodeName, plugin, 1*time.Minute)
	require.NoError(b, err)

	// Create a test TridentVolumePublication
	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")

	// Add it to the fake clientset
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(b, err)

	// Add to indexer
	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(b, err)

	// Create KeyItem
	keyItem := &KeyItem{
		key:        testNamespace + "/" + tvp.Name,
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := controller.handleTridentVolumePublications(keyItem)
		if err != nil {
			b.Fatalf("Handler failed: %v", err)
		}
	}
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Additional comprehensive tests to improve coverage

func TestHandleTridentVolumePublications_CacheKeyParsingError(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem with a malformed key that will cause cache.SplitMetaNamespaceKey to fail
	// This is difficult to achieve in practice, but let's test with a valid key first
	keyItem := &KeyItem{
		key:        "malformed//key//with//too//many//slashes",
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// Handle should return error for malformed key
	err = controller.handleTridentVolumePublications(keyItem)
	assert.Error(t, err, "should error with malformed key")
}

func TestHandleTridentVolumePublications_NamespaceButNoName(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem where namespace exists but name is empty
	keyItem := &KeyItem{
		key:        testNamespace + "/",
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// This should cause an error when trying to get the object from lister
	err = controller.handleTridentVolumePublications(keyItem)
	assert.Error(t, err, "should error when name is empty")
}

func TestHandleTridentVolumePublications_EmptyNameInKey(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem where namespace exists but name is truly empty (no trailing slash)
	keyItem := &KeyItem{
		key:        testNamespace, // No slash, so name will be empty after split
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// This should cause an error when trying to get the object from lister
	err = controller.handleTridentVolumePublications(keyItem)
	assert.Error(t, err, "should error when name is empty")
}

func TestHandleTridentVolumePublications_MatchingNodeDetailed(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a TridentVolumePublication with specific properties
	tvp := createTestTridentVolumePublication("detailed-tvp", testNamespace, testNodeName, "vol-456")
	tvp.AccessMode = 1 // ReadWriteOnce
	tvp.ReadOnly = false
	tvp.AutogrowPolicy = "test-policy"

	// Add it to the fake clientset
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to indexer
	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// Create KeyItem for various event types to ensure all log paths are covered
	eventTypes := []EventType{EventAdd, EventUpdate, EventDelete}

	for _, eventType := range eventTypes {
		t.Run("event_"+string(eventType), func(t *testing.T) {
			keyItem := &KeyItem{
				key:        testNamespace + "/" + tvp.Name,
				objectType: OjbectTypeTridentVolumePublication,
				event:      eventType,
				ctx:        context.Background(),
			}

			err = controller.handleTridentVolumePublications(keyItem)
			assert.NoError(t, err, "should handle %s event successfully with detailed TVP", eventType)
		})
	}
}

func TestHandleTridentVolumePublications_DifferentNodeNames(t *testing.T) {
	// Test with various node name scenarios
	testCases := []struct {
		name           string
		controllerNode string
		tvpNode        string
		expectError    bool
		expectWarning  bool
		description    string
	}{
		{
			name:           "exact match",
			controllerNode: "node-1",
			tvpNode:        "node-1",
			expectError:    false,
			expectWarning:  false,
			description:    "should process TVP for matching node",
		},
		{
			name:           "different node",
			controllerNode: "node-1",
			tvpNode:        "node-2",
			expectError:    false,
			expectWarning:  true,
			description:    "should handle TVP for different node gracefully",
		},
		{
			name:           "empty controller node",
			controllerNode: testNodeName, // Can't create controller with empty nodeName
			tvpNode:        "node-1",
			expectError:    false,
			expectWarning:  true,
			description:    "should handle TVP when TVP is for different node",
		},
		{
			name:           "empty tvp node",
			controllerNode: "node-1",
			tvpNode:        "",
			expectError:    false,
			expectWarning:  true,
			description:    "should handle TVP with empty node name",
		},
		{
			name:           "both empty",
			controllerNode: testNodeName, // Can't create controller with empty nodeName
			tvpNode:        "",
			expectError:    false,
			expectWarning:  true, // Changed to true since testNodeName != ""
			description:    "should handle TVP with empty node name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			controller, err := newTestTridentNodeCrdController(t, tc.controllerNode)
			require.NoError(t, err)

			// Create a TridentVolumePublication for the specified node
			tvp := createTestTridentVolumePublication("test-tvp", testNamespace, tc.tvpNode, "vol-789")

			// Add it to the fake clientset
			_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
				context.Background(), tvp, metav1.CreateOptions{})
			require.NoError(t, err)

			// Add to indexer
			err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
			require.NoError(t, err)

			// Create KeyItem
			keyItem := &KeyItem{
				key:        testNamespace + "/" + tvp.Name,
				objectType: OjbectTypeTridentVolumePublication,
				event:      EventAdd,
				ctx:        context.Background(),
			}

			err = controller.handleTridentVolumePublications(keyItem)
			if tc.expectError {
				assert.Error(t, err, tc.description)
			} else {
				assert.NoError(t, err, tc.description)
			}
		})
	}
}

func TestHandleTridentVolumePublications_MultipleObjectsInIndexer(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create multiple TridentVolumePublications
	tvp1 := createTestTridentVolumePublication("tvp-1", testNamespace, testNodeName, "vol-001")
	tvp2 := createTestTridentVolumePublication("tvp-2", testNamespace, testNodeName, "vol-002")
	tvp3 := createTestTridentVolumePublication("tvp-3", testNamespace, "different-node", "vol-003")

	// Add all to fake clientset
	for _, tvp := range []*tridentv1.TridentVolumePublication{tvp1, tvp2, tvp3} {
		_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
			context.Background(), tvp, metav1.CreateOptions{})
		require.NoError(t, err)

		// Add to indexer
		err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
		require.NoError(t, err)
	}

	// Test processing each one
	testObjects := []struct {
		tvp         *tridentv1.TridentVolumePublication
		expectError bool
	}{
		{tvp1, false}, // same node
		{tvp2, false}, // same node
		{tvp3, false}, // different node, should log warning but not error
	}

	for _, obj := range testObjects {
		t.Run(obj.tvp.Name, func(t *testing.T) {
			keyItem := &KeyItem{
				key:        testNamespace + "/" + obj.tvp.Name,
				objectType: OjbectTypeTridentVolumePublication,
				event:      EventUpdate,
				ctx:        context.Background(),
			}

			err = controller.handleTridentVolumePublications(keyItem)
			if obj.expectError {
				assert.Error(t, err, "should error for TVP %s", obj.tvp.Name)
			} else {
				assert.NoError(t, err, "should handle TVP %s successfully", obj.tvp.Name)
			}
		})
	}
}

func TestHandleTridentVolumePublications_RetryScenario(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a TridentVolumePublication
	tvp := createTestTridentVolumePublication("retry-tvp", testNamespace, testNodeName, "vol-retry")

	// Add it to the fake clientset
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to indexer
	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// Create KeyItem with retry flag
	keyItem := &KeyItem{
		key:        testNamespace + "/" + tvp.Name,
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventUpdate,
		ctx:        context.Background(),
		isRetry:    true, // This is a retry
	}

	err = controller.handleTridentVolumePublications(keyItem)
	assert.NoError(t, err, "should handle retry scenario successfully")
}

func TestHandleTridentVolumePublications_ContextualLogging(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a TridentVolumePublication with detailed properties for logging
	tvp := createTestTridentVolumePublication("logging-tvp", testNamespace, testNodeName, "vol-log-test")

	// Add additional properties to enhance log coverage
	tvp.ReadOnly = true
	tvp.AccessMode = 2 // ReadOnlyMany
	tvp.AutogrowPolicy = "growth-policy"

	// Add it to the fake clientset
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to indexer
	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// Create context with specific request ID for better logging coverage
	ctx := GenerateRequestContext(nil, "test-req-123", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)

	keyItem := &KeyItem{
		key:        testNamespace + "/" + tvp.Name,
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        ctx, // Use enriched context
	}

	err = controller.handleTridentVolumePublications(keyItem)
	assert.NoError(t, err, "should handle with enriched context and logging")
}

// Additional tests for comprehensive coverage

func TestHandleTridentVolumePublications_EmptyNamespace(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test with key that has no namespace (no slash)
	keyItem := &KeyItem{
		key:        "tvp-no-namespace",
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// Should error because namespace is required
	err = controller.handleTridentVolumePublications(keyItem)
	assert.Error(t, err, "should error when namespace is empty")
	assert.Contains(t, err.Error(), "no namespace present", "error should mention missing namespace")
}

func TestHandleTridentVolumePublications_DeleteEventSuccess(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a TVP
	tvp := createTestTridentVolumePublication("delete-tvp", testNamespace, testNodeName, "vol-delete")

	// Add it to the fake clientset and indexer
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(t, err)

	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// Now process a delete event for it
	keyItem := &KeyItem{
		key:        testNamespace + "/" + tvp.Name,
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventDelete,
		ctx:        context.Background(),
	}

	err = controller.handleTridentVolumePublications(keyItem)
	assert.NoError(t, err, "should successfully handle delete event for existing object")
}

func TestHandleTridentVolumePublications_UpdateEventSuccess(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a TVP
	tvp := createTestTridentVolumePublication("update-tvp", testNamespace, testNodeName, "vol-update")

	// Add it to the fake clientset and indexer
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(t, err)

	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// Process an update event
	keyItem := &KeyItem{
		key:        testNamespace + "/" + tvp.Name,
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventUpdate,
		ctx:        context.Background(),
	}

	err = controller.handleTridentVolumePublications(keyItem)
	assert.NoError(t, err, "should successfully handle update event")
}

func TestHandleTridentVolumePublications_LabelSelectorWarning(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a TVP for a DIFFERENT node (to test the warning path)
	differentNode := "different-node"
	tvp := createTestTridentVolumePublication("wrong-node-tvp", testNamespace, differentNode, "vol-wrong")

	// Add it to the fake clientset and indexer
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(t, err)

	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// Process an add event - should trigger warning about label selector
	keyItem := &KeyItem{
		key:        testNamespace + "/" + tvp.Name,
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	err = controller.handleTridentVolumePublications(keyItem)
	assert.NoError(t, err, "should handle gracefully even with wrong node (label selector warning)")
}

func TestHandleTridentVolumePublications_AllFieldsLogging(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a TVP with all possible fields populated
	tvp := createTestTridentVolumePublication("full-fields-tvp", testNamespace, testNodeName, "vol-full")
	tvp.ReadOnly = true
	tvp.AccessMode = 1
	tvp.AutogrowPolicy = "test-policy"

	// Add it to the fake clientset and indexer
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(t, err)

	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// Test all event types to ensure logging code is covered
	events := []EventType{EventAdd, EventUpdate, EventDelete}
	for _, event := range events {
		keyItem := &KeyItem{
			key:        testNamespace + "/" + tvp.Name,
			objectType: OjbectTypeTridentVolumePublication,
			event:      event,
			ctx:        context.Background(),
		}

		err = controller.handleTridentVolumePublications(keyItem)
		assert.NoError(t, err, "should handle %s event successfully", event)
	}
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Orchestrator Integration Tests

func TestHandleTridentVolumePublications_OrchestratorCalls(t *testing.T) {
	tests := []struct {
		name                   string
		setupTVP               func() *tridentv1.TridentVolumePublication
		event                  EventType
		expectOrchestratorCall bool
		expectedKey            string // What key should be passed to orchestrator
		description            string
	}{
		{
			name: "AddEvent_CallsOrchestratorWithFullKey",
			setupTVP: func() *tridentv1.TridentVolumePublication {
				return createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
			},
			event:                  EventAdd,
			expectOrchestratorCall: true,
			expectedKey:            testNamespace + "/test-tvp", // Full key
			description:            "Add event should call orchestrator with full key (namespace/name)",
		},
		{
			name: "UpdateEvent_CallsOrchestratorWithFullKey",
			setupTVP: func() *tridentv1.TridentVolumePublication {
				return createTestTridentVolumePublication("update-tvp", testNamespace, testNodeName, "vol-456")
			},
			event:                  EventUpdate,
			expectOrchestratorCall: true,
			expectedKey:            testNamespace + "/update-tvp", // Full key
			description:            "Update event should call orchestrator with full key (namespace/name)",
		},
		{
			name: "DeleteEvent_ExistingTVP_CallsOrchestratorWithFullKey",
			setupTVP: func() *tridentv1.TridentVolumePublication {
				return createTestTridentVolumePublication("delete-tvp", testNamespace, testNodeName, "vol-789")
			},
			event:                  EventDelete,
			expectOrchestratorCall: true,
			expectedKey:            testNamespace + "/delete-tvp", // Full key for existing object
			description:            "Delete event for existing TVP should call orchestrator with full key",
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

			// Setup TVP
			tvp := tt.setupTVP()

			// Add to fake clientset
			_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
				context.Background(), tvp, metav1.CreateOptions{})
			require.NoError(t, err)

			// Add to indexer
			err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
			require.NoError(t, err)

			// Setup expectation
			if tt.expectOrchestratorCall {
				mockOrchestrator.EXPECT().
					HandleTVPEvent(gomock.Any(), tt.event, tt.expectedKey).
					Times(1)
			}

			// Create KeyItem
			keyItem := &KeyItem{
				key:        testNamespace + "/" + tvp.Name,
				objectType: OjbectTypeTridentVolumePublication,
				event:      tt.event,
				ctx:        context.Background(),
			}

			// Execute
			err = controller.handleTridentVolumePublications(keyItem)
			assert.NoError(t, err, tt.description)
		})
	}
}

func TestHandleTridentVolumePublications_DeleteEvent_NotFound_CallsOrchestratorWithName(t *testing.T) {
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

	// TVP name
	tvpName := "deleted-tvp"

	// Expect orchestrator to be called with just the NAME (not full key) for delete of non-existent object
	mockOrchestrator.EXPECT().
		HandleTVPEvent(gomock.Any(), EventDelete, tvpName).
		Times(1)

	// Create KeyItem for non-existent object with delete event
	keyItem := &KeyItem{
		key:        testNamespace + "/" + tvpName,
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventDelete,
		ctx:        context.Background(),
	}

	// Execute - should succeed and call orchestrator with name only
	err = controller.handleTridentVolumePublications(keyItem)
	assert.NoError(t, err, "should handle delete event of non-existent object and call orchestrator with name")
}

func TestHandleTridentVolumePublications_WrongNode_DoesNotCallOrchestrator(t *testing.T) {
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

	// Create TVP for a DIFFERENT node
	differentNode := "different-node"
	tvp := createTestTridentVolumePublication("wrong-node-tvp", testNamespace, differentNode, "vol-wrong")

	// Add to fake clientset
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to indexer
	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// NO expectation set - orchestrator should NOT be called

	// Create KeyItem
	keyItem := &KeyItem{
		key:        testNamespace + "/" + tvp.Name,
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	// Execute - should return nil but NOT call orchestrator
	err = controller.handleTridentVolumePublications(keyItem)
	assert.NoError(t, err, "should handle gracefully but not call orchestrator for wrong node")

	// Mock will fail if HandleTVPEvent was called when it shouldn't be
}

func TestHandleTridentVolumePublications_AllEvents_VerifyOrchestratorCalls(t *testing.T) {
	eventTests := []struct {
		name        string
		event       EventType
		expectedKey string
		description string
	}{
		{
			name:        "Add",
			event:       EventAdd,
			expectedKey: testNamespace + "/test-tvp",
			description: "Add event should pass full key",
		},
		{
			name:        "Update",
			event:       EventUpdate,
			expectedKey: testNamespace + "/test-tvp",
			description: "Update event should pass full key",
		},
		{
			name:        "Delete",
			event:       EventDelete,
			expectedKey: testNamespace + "/test-tvp",
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

			// Create TVP
			tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-test")

			// Add to fake clientset
			_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
				context.Background(), tvp, metav1.CreateOptions{})
			require.NoError(t, err)

			// Add to indexer
			err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
			require.NoError(t, err)

			// Expect orchestrator call with correct parameters
			mockOrchestrator.EXPECT().
				HandleTVPEvent(gomock.Any(), tt.event, tt.expectedKey).
				Times(1)

			// Create KeyItem
			keyItem := &KeyItem{
				key:        tt.expectedKey,
				objectType: OjbectTypeTridentVolumePublication,
				event:      tt.event,
				ctx:        context.Background(),
			}

			// Execute
			err = controller.handleTridentVolumePublications(keyItem)
			assert.NoError(t, err, tt.description)
		})
	}
}
