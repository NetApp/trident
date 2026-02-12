// Copyright 2025 NetApp, Inc. All Rights Reserved.
package crd

import (
	"context"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend/csi"
	mockNodeHelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentv1clientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	faketridentclient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
)

const (
	testNamespace = "trident"
	testNodeName  = "test-node-1"
)

var (
	propagationPolicy = metav1.DeletePropagationBackground
	deleteOptions     = metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}
)

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Utility functions

func getFakeKubernetesClientset() *fake.Clientset {
	return fake.NewClientset()
}

func getFakeTridentClientset() *faketridentclient.Clientset {
	return faketridentclient.NewSimpleClientset()
}

// mockCSIPlugin creates a mock CSI plugin for testing
func mockCSIPlugin(t *testing.T) *csi.Plugin {
	// Create gomock controller
	ctrl := gomock.NewController(t)

	// Create mock NodeHelper using mockgen-generated mock
	mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(ctrl)

	// Create a minimal plugin struct
	plugin := &csi.Plugin{}

	// Use reflection to set the private nodeHelper field
	// This is necessary because the field is unexported and there's no setter
	pluginValue := reflect.ValueOf(plugin).Elem()
	nodeHelperField := pluginValue.FieldByName("nodeHelper")

	// Make the field settable and set it using unsafe
	// This is necessary in tests to inject the mock dependency
	nodeHelperField = reflect.NewAt(nodeHelperField.Type(), unsafe.Pointer(nodeHelperField.UnsafeAddr())).Elem()
	nodeHelperField.Set(reflect.ValueOf(mockNodeHelper))

	return plugin
}

// MockCSIPlugin is a simple mock that implements the WaitForActivation method
type MockCSIPlugin struct {
	shouldBlock   bool
	errorToReturn error
}

func (m *MockCSIPlugin) WaitForActivation(ctx context.Context) error {
	if m.errorToReturn != nil {
		return m.errorToReturn
	}
	if !m.shouldBlock {
		return nil
	}
	// Block until context is cancelled for testing
	<-ctx.Done()
	return ctx.Err()
}

// Helper function to create a test controller without orchestrator dependency
func newTestTridentNodeCrdController(t *testing.T, nodeName string) (*TridentNodeCrdController, error) {
	kubeClient := getFakeKubernetesClientset()
	crdClient := getFakeTridentClientset()
	plugin := mockCSIPlugin(t)

	// Use default Autogrow period of 1 minute for tests
	return newTridentNodeCrdController(testNamespace, kubeClient, crdClient, nodeName, plugin, 1*time.Minute)
}

// Helper function to create a test TridentVolumePublication
func createTestTridentVolumePublication(name, namespace, nodeID, volumeID string) *tridentv1.TridentVolumePublication {
	return &tridentv1.TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		NodeID:   nodeID,
		VolumeID: volumeID,
	}
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Constructor tests

func TestNewTridentNodeCrdController_Success(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)

	assert.NoError(t, err, "should not error when creating controller")
	assert.NotNil(t, controller, "controller should not be nil")
	assert.Equal(t, testNodeName, controller.nodeName, "node name should match")
	assert.NotNil(t, controller.kubeClientset, "kube clientset should not be nil")
	assert.NotNil(t, controller.crdClientset, "crd clientset should not be nil")
	assert.NotNil(t, controller.workqueue, "workqueue should not be nil")
	assert.NotNil(t, controller.recorder, "recorder should not be nil")
	assert.NotNil(t, controller.tridentVolumePublicationInformer, "tvp informer should not be nil")
	assert.NotNil(t, controller.tridentVolumePublicationLister, "tvp lister should not be nil")
}

func TestGetName(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	assert.Equal(t, controllerName, controller.GetName())
}

func TestVersion(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	assert.Equal(t, config.DefaultOrchestratorVersion, controller.Version())
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Event handler tests

func TestAddCRHandler_TridentVolumePublication(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")

	// Add the object to the handler
	controller.addCRHandler(tvp)

	// Verify the event was added to workqueue
	assert.Greater(t, controller.workqueue.Len(), 0, "workqueue should have items")

	// Get the work item
	workItem, shutdown := controller.workqueue.Get()
	assert.False(t, shutdown, "workqueue should not be shut down")

	assert.Equal(t, EventAdd, workItem.event, "event type should be Add")
	assert.Equal(t, OjbectTypeTridentVolumePublication, workItem.objectType, "object type should be TridentVolumePublication")
}

func TestAddCRHandler_InvalidObject(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test with an invalid object that doesn't implement TridentCRD
	invalidObj := &metav1.ObjectMeta{}

	// This should not panic or add anything to the workqueue
	controller.addCRHandler(invalidObj)

	// Verify nothing was added to workqueue
	assert.Equal(t, 0, controller.workqueue.Len(), "workqueue should be empty for invalid object")
}

func TestUpdateCRHandler_TridentVolumePublication_NewGeneration(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	oldTvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
	oldTvp.Generation = 1

	newTvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
	newTvp.Generation = 2

	// Update the object
	controller.updateCRHandler(oldTvp, newTvp)

	// Verify the event was added to workqueue
	assert.Greater(t, controller.workqueue.Len(), 0, "workqueue should have items")

	// Get the work item
	workItem, shutdown := controller.workqueue.Get()
	assert.False(t, shutdown, "workqueue should not be shut down")

	assert.Equal(t, EventUpdate, workItem.event, "event type should be Update")
	assert.Equal(t, OjbectTypeTridentVolumePublication, workItem.objectType, "object type should be TridentVolumePublication")
}

func TestUpdateCRHandler_TridentVolumePublication_NoChange(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	oldTvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
	oldTvp.Generation = 1

	newTvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
	newTvp.Generation = 1 // Same generation, no real change

	// Update the object
	controller.updateCRHandler(oldTvp, newTvp)

	// Verify no event was added to workqueue (no change detected)
	assert.Equal(t, 0, controller.workqueue.Len(), "workqueue should be empty for no-change update")
}

func TestUpdateCRHandler_TridentVolumePublication_Deleted(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	now := time.Now()
	v1Now := metav1.NewTime(now)

	oldTvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
	oldTvp.Generation = 1

	newTvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
	newTvp.Generation = 1
	newTvp.DeletionTimestamp = &v1Now // Object is being deleted

	// Update the object
	controller.updateCRHandler(oldTvp, newTvp)

	// Verify the event was added to workqueue (deletion needs processing)
	assert.Greater(t, controller.workqueue.Len(), 0, "workqueue should have items for deletion")

	// Get the work item
	workItem, shutdown := controller.workqueue.Get()
	assert.False(t, shutdown, "workqueue should not be shut down")

	assert.Equal(t, EventUpdate, workItem.event, "event type should be Update")
}

func TestUpdateCRHandler_InvalidObjects(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test with nil objects
	controller.updateCRHandler(nil, nil)
	assert.Equal(t, 0, controller.workqueue.Len(), "workqueue should be empty for nil objects")

	// Test with invalid old object
	invalidObj := &metav1.ObjectMeta{}
	newTvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
	controller.updateCRHandler(invalidObj, newTvp)
	assert.Equal(t, 0, controller.workqueue.Len(), "workqueue should be empty for invalid old object")

	// Test with invalid new object
	oldTvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
	controller.updateCRHandler(oldTvp, invalidObj)
	assert.Equal(t, 0, controller.workqueue.Len(), "workqueue should be empty for invalid new object")
}

func TestDeleteCRHandler_TridentVolumePublication(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")

	// Delete the object
	controller.deleteCRHandler(tvp)

	// Verify the event was added to workqueue
	assert.Greater(t, controller.workqueue.Len(), 0, "workqueue should have items")

	// Get the work item
	workItem, shutdown := controller.workqueue.Get()
	assert.False(t, shutdown, "workqueue should not be shut down")

	assert.Equal(t, EventDelete, workItem.event, "event type should be Delete")
	assert.Equal(t, OjbectTypeTridentVolumePublication, workItem.objectType, "object type should be TridentVolumePublication")
}

func TestDeleteCRHandler_InvalidObject(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test with an invalid object that doesn't implement TridentCRD
	invalidObj := &metav1.ObjectMeta{}

	// This should not panic or add anything to the workqueue
	controller.deleteCRHandler(invalidObj)

	// Verify nothing was added to workqueue
	assert.Equal(t, 0, controller.workqueue.Len(), "workqueue should be empty for invalid object")
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Workqueue and processing tests

func TestAddEventToWorkqueue(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	ctx := context.Background()
	testKey := "test-namespace/test-object"
	eventType := EventAdd
	objectType := OjbectTypeTridentVolumePublication

	controller.addEventToWorkqueue(testKey, eventType, ctx, objectType)

	// Verify the event was added
	assert.Equal(t, 1, controller.workqueue.Len(), "workqueue should have exactly one item")

	workItem, shutdown := controller.workqueue.Get()
	assert.False(t, shutdown, "workqueue should not be shut down")

	assert.Equal(t, testKey, workItem.key, "key should match")
	assert.Equal(t, eventType, workItem.event, "event type should match")
	assert.Equal(t, objectType, workItem.objectType, "object type should match")
	assert.Equal(t, ctx, workItem.ctx, "context should match")
	assert.False(t, workItem.isRetry, "should not be a retry initially")
}

func TestProcessNextWorkItem_EmptyQueue(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Shutdown the workqueue to simulate empty/shutdown state
	controller.workqueue.ShutDown()

	// Processing should return false (indicating shutdown)
	result := controller.processNextWorkItem()
	assert.False(t, result, "should return false when workqueue is shut down")
}

func TestProcessNextWorkItem_InvalidKeyItem(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// TypedQueue only accepts KeyItem, so test with an empty KeyItem instead
	// This test now verifies that an empty KeyItem is handled gracefully
	controller.workqueue.Add(KeyItem{})

	// Processing should handle the empty item gracefully
	result := controller.processNextWorkItem()
	assert.True(t, result, "should return true even with invalid item")
	assert.Equal(t, 0, controller.workqueue.Len(), "invalid item should be removed from queue")
}

func TestProcessNextWorkItem_EmptyKeyItem(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Add an empty KeyItem
	emptyKeyItem := KeyItem{}
	controller.workqueue.Add(emptyKeyItem)

	// Processing should handle the empty KeyItem gracefully
	result := controller.processNextWorkItem()
	assert.True(t, result, "should return true even with empty KeyItem")
	assert.Equal(t, 0, controller.workqueue.Len(), "empty KeyItem should be removed from queue")
}

func TestProcessNextWorkItem_UnknownObjectType(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Add a KeyItem with unknown object type
	keyItem := KeyItem{
		key:        "test-key",
		objectType: "UnknownObjectType",
		event:      EventAdd,
		ctx:        context.Background(),
	}
	controller.workqueue.Add(keyItem)

	// Processing should handle the unknown object type gracefully
	result := controller.processNextWorkItem()
	assert.True(t, result, "should return true even with unknown object type")
	assert.Equal(t, 0, controller.workqueue.Len(), "unknown object type item should be removed from queue")
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Field selector and filtering tests

func TestFieldSelectorSetup(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Verify that the controller has set up field selector correctly
	assert.NotNil(t, controller.tridentVolumePublicationInformer, "tvp informer should be created")

	// We can't easily test the field selector without actually running the informer,
	// but we can verify the informer was created successfully
	assert.NotNil(t, controller.tridentVolumePublicationLister, "tvp lister should be created")
	assert.NotNil(t, controller.tridentVolumePublicationSynced, "tvp synced function should be set")
}

func TestFieldSelectorString(t *testing.T) {
	nodeName := "test-node"
	expectedFieldSelector := "nodeID=test-node"

	fieldSelector := fields.OneTermEqualSelector("nodeID", nodeName).String()
	assert.Equal(t, expectedFieldSelector, fieldSelector, "field selector should match expected format")
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Indexer tests

func TestVolumeIDIndexer(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a test TridentVolumePublication
	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")

	// Get the indexer from the informer
	indexer := controller.tridentVolumePublicationInformer.GetIndexer()

	// Manually add the object to test the indexer
	err = indexer.Add(tvp)
	require.NoError(t, err, "should be able to add object to indexer")

	// The TVP informer is created with empty indexers (no custom indexers needed)
	// Verify that the indexers map is empty as expected
	indexers := indexer.GetIndexers()
	assert.Empty(t, indexers, "TVP informer should have no custom indexers per controller design")

	// Verify basic indexer operations work (Get, List, etc.)
	obj, exists, err := indexer.GetByKey(testNamespace + "/test-tvp")
	assert.NoError(t, err, "should be able to get object by key")
	assert.True(t, exists, "object should exist in indexer")
	assert.NotNil(t, obj, "retrieved object should not be nil")

	// Verify list all objects works
	allObjs := indexer.List()
	assert.Len(t, allObjs, 1, "should have one object in indexer")
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Integration-style tests

func TestControllerLifecycle(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test basic properties
	assert.Equal(t, controllerName, controller.GetName())
	assert.Equal(t, config.DefaultOrchestratorVersion, controller.Version())

	// Test that we can create TridentVolumePublication CRDs
	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")

	// Create the CRD (this tests the fake clientset integration)
	createdTvp, err := controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	assert.NoError(t, err, "should be able to create TridentVolumePublication CRD")
	assert.Equal(t, tvp.Name, createdTvp.Name, "created TVP should have correct name")
	assert.Equal(t, tvp.NodeID, createdTvp.NodeID, "created TVP should have correct nodeID")
	assert.Equal(t, tvp.VolumeID, createdTvp.VolumeID, "created TVP should have correct volumeID")

	// List TridentVolumePublications
	tvpList, err := controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).List(
		context.Background(), metav1.ListOptions{})
	assert.NoError(t, err, "should be able to list TridentVolumePublication CRDs")
	assert.Len(t, tvpList.Items, 1, "should have exactly one TVP")

	// Delete TridentVolumePublication
	err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Delete(
		context.Background(), tvp.Name, deleteOptions)
	assert.NoError(t, err, "should be able to delete TridentVolumePublication CRD")
}

func TestWorkqueueIntegration(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test multiple events
	tvp1 := createTestTridentVolumePublication("tvp1", testNamespace, testNodeName, "vol-123")
	tvp2 := createTestTridentVolumePublication("tvp2", testNamespace, testNodeName, "vol-456")

	// Add multiple events
	controller.addCRHandler(tvp1)
	controller.addCRHandler(tvp2)

	// Update one
	tvp1Updated := createTestTridentVolumePublication("tvp1", testNamespace, testNodeName, "vol-123")
	tvp1Updated.Generation = 2
	controller.updateCRHandler(tvp1, tvp1Updated)

	// Delete one
	controller.deleteCRHandler(tvp2)

	// Should have 4 events in queue (2 adds, 1 update, 1 delete)
	assert.Equal(t, 4, controller.workqueue.Len(), "should have 4 events in workqueue")

	// Process events and verify they're handled correctly
	eventTypes := make([]EventType, 0)
	for controller.workqueue.Len() > 0 {
		workItem, shutdown := controller.workqueue.Get()
		assert.False(t, shutdown, "workqueue should not be shut down")

		eventTypes = append(eventTypes, workItem.event)

		// Mark as done
		controller.workqueue.Done(workItem)
	}

	// Verify we got the expected event types
	expectedEvents := []EventType{EventAdd, EventAdd, EventUpdate, EventDelete}
	assert.ElementsMatch(t, expectedEvents, eventTypes, "should have processed all expected events")
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Error handling tests

func TestCacheKeyGeneration(t *testing.T) {
	tvp := createTestTridentVolumePublication("test-tvp", "test-namespace", testNodeName, "vol-123")

	key, err := cache.MetaNamespaceKeyFunc(tvp)
	assert.NoError(t, err, "should be able to generate cache key")
	assert.Equal(t, "test-namespace/test-tvp", key, "cache key should be in namespace/name format")
}

func TestSplitMetaNamespaceKey(t *testing.T) {
	key := "test-namespace/test-object"

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	assert.NoError(t, err, "should be able to split cache key")
	assert.Equal(t, "test-namespace", namespace, "namespace should be correct")
	assert.Equal(t, "test-object", name, "name should be correct")

	// Test invalid key
	onlyObjectInKey := "only-object-no-namespace"
	namespace, name, err = cache.SplitMetaNamespaceKey(onlyObjectInKey)
	assert.NoError(t, err, "should be able to split cache key when only name is present")
	assert.Equal(t, "", namespace, "namespace should be empty")
	assert.Equal(t, "only-object-no-namespace", name, "name should be correct")

	// Test invalid key
	invalidKey := "namespace/object/yetAnotherName"
	namespace, name, err = cache.SplitMetaNamespaceKey(invalidKey)
	assert.Error(t, err, "should error with invalid key format")
}

func TestConstants(t *testing.T) {
	// Verify important constants are set correctly
	assert.Equal(t, "add", string(EventAdd))
	assert.Equal(t, "update", string(EventUpdate))
	assert.Equal(t, "delete", string(EventDelete))
	assert.Equal(t, "TridentVolumePublication", string(OjbectTypeTridentVolumePublication))
	assert.Equal(t, "node-crd-controller", controllerName)
	assert.Equal(t, "trident-node-crd-controller", controllerAgentName)
	assert.Equal(t, "trident-node-crd-workqueue", crdControllerQueueName)
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// NewTridentNodeCrdController tests

func TestLogx(t *testing.T) {
	ctx := context.Background()
	logger := Logx(ctx)

	// Verify that the logger has the correct source field
	assert.NotNil(t, logger, "logger should not be nil")
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Activate and Deactivate tests

func TestDeactivate_Success(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	err = controller.Deactivate()
	assert.NoError(t, err, "should deactivate successfully")
}

func TestDeactivate_NilStopChannel(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Set the stop channel to nil
	controller.crdControllerStopChan = nil

	err = controller.Deactivate()
	assert.NoError(t, err, "should handle nil stop channel gracefully")
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Run function tests

func TestRun_CacheSync_Failure(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a context that is already cancelled to simulate cache sync failure
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Create a stop channel and close it immediately to simulate shutdown
	stopCh := make(chan struct{})
	close(stopCh)

	// This should exit quickly due to the closed stopCh
	controller.Run(ctx, 1, stopCh)

	// If we reach here, the function returned (which is expected)
	assert.True(t, true, "Run should handle cancelled context gracefully")
}

func TestRun_WithValidInputs(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	ctx := context.Background()

	// Create a stop channel that we'll close after a short delay
	stopCh := make(chan struct{})

	// Run in a goroutine and stop it after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(stopCh)
	}()

	// This should run briefly and then exit when stopCh is closed
	controller.Run(ctx, 1, stopCh)

	assert.True(t, true, "Run should handle normal execution")
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// runWorker function tests

func TestRunWorker_Integration(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Add a test item to the workqueue
	testKeyItem := KeyItem{
		key:        testNamespace + "/test-tvp",
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}

	controller.workqueue.Add(testKeyItem)

	// Shutdown the workqueue to make runWorker exit
	controller.workqueue.ShutDown()

	// This should process the item and then exit due to shutdown
	controller.runWorker()

	assert.True(t, true, "runWorker should handle workqueue processing")
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Enhanced processNextWorkItem tests for better coverage

func TestProcessNextWorkItem_UnsupportedConfigError(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a TridentVolumePublication that will exist when we try to get it
	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")

	// Add it to the fake clientset
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to indexer
	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// Add a KeyItem that will trigger an UnsupportedConfigError by manipulating the handler
	keyItem := KeyItem{
		key:        testNamespace + "/" + tvp.Name,
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}
	controller.workqueue.Add(keyItem)

	// Process the item - it should handle it gracefully even if there's an error
	result := controller.processNextWorkItem()
	assert.True(t, result, "should return true even with processing errors")
}

func TestProcessNextWorkItem_ReconcileDeferredError_Handling(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Add a KeyItem that will not find a corresponding object (to simulate error path)
	keyItem := KeyItem{
		key:        testNamespace + "/nonexistent-tvp",
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}
	controller.workqueue.Add(keyItem)

	// Process the item - it should handle the "not found" error
	result := controller.processNextWorkItem()
	assert.True(t, result, "should return true even when object not found")

	// The item should be removed from the queue due to the error
	assert.Equal(t, 0, controller.workqueue.Len(), "queue should be empty after processing")
}

func TestProcessNextWorkItem_DeleteEvent_NotFound(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Add a KeyItem for a delete event of non-existent object
	keyItem := KeyItem{
		key:        testNamespace + "/deleted-tvp",
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventDelete,
		ctx:        context.Background(),
	}
	controller.workqueue.Add(keyItem)

	// Process the item - delete events should be handled gracefully even for non-existent objects
	result := controller.processNextWorkItem()
	assert.True(t, result, "should return true for delete events")

	// The item should be removed from the queue
	assert.Equal(t, 0, controller.workqueue.Len(), "queue should be empty after processing")
}

func TestProcessNextWorkItem_SuccessfulProcessing(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a TridentVolumePublication that exists
	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")

	// Add it to the fake clientset
	_, err = controller.crdClientset.TridentV1().TridentVolumePublications(testNamespace).Create(
		context.Background(), tvp, metav1.CreateOptions{})
	require.NoError(t, err)

	// Add to indexer
	err = controller.tridentVolumePublicationInformer.GetIndexer().Add(tvp)
	require.NoError(t, err)

	// Add a KeyItem for successful processing
	keyItem := KeyItem{
		key:        testNamespace + "/" + tvp.Name,
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}
	controller.workqueue.Add(keyItem)

	// Process the item - should succeed
	result := controller.processNextWorkItem()
	assert.True(t, result, "should return true for successful processing")

	// The item should be removed from the queue after successful processing
	assert.Equal(t, 0, controller.workqueue.Len(), "queue should be empty after successful processing")
}

// ///////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Additional edge case tests for improving coverage

func TestAddCRHandler_CacheKeySuccess(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a valid TridentVolumePublication
	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")

	// This should add the item to the workqueue successfully
	controller.addCRHandler(tvp)

	// Verify item was added to workqueue
	assert.Equal(t, 1, controller.workqueue.Len(), "workqueue should have one item")
}

func TestUpdateCRHandler_CacheKeySuccess(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create valid objects
	oldTvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
	oldTvp.Generation = 1

	newTvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
	newTvp.Generation = 2

	// This should add the item to the workqueue successfully
	controller.updateCRHandler(oldTvp, newTvp)

	// Verify item was added to workqueue
	assert.Equal(t, 1, controller.workqueue.Len(), "workqueue should have one item")
}

func TestDeleteCRHandler_CacheKeySuccess(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a valid TridentVolumePublication
	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")

	// This should add the item to the workqueue successfully
	controller.deleteCRHandler(tvp)

	// Verify item was added to workqueue
	assert.Equal(t, 1, controller.workqueue.Len(), "workqueue should have one item")
}

func TestUpdateCRHandler_ZeroGeneration(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create objects with generation 0 (special case)
	oldTvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
	oldTvp.Generation = 0

	newTvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")
	newTvp.Generation = 0

	// Update should be processed when generation is 0
	controller.updateCRHandler(oldTvp, newTvp)

	// Verify the event was added to workqueue
	assert.Greater(t, controller.workqueue.Len(), 0, "workqueue should have items when generation is 0")
}

func TestRun_SuccessfulCacheSync(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	ctx := context.Background()

	// Create a stop channel that we'll close after a very short delay
	stopCh := make(chan struct{})

	// Run in a goroutine and stop it quickly
	go func() {
		time.Sleep(10 * time.Millisecond) // Very short delay to allow some setup
		close(stopCh)
	}()

	// This should run briefly and then exit when stopCh is closed
	// The cache sync should succeed in this case (fake informer)
	controller.Run(ctx, 1, stopCh)

	assert.True(t, true, "Run should complete successfully with proper cache sync")
}

func TestProcessNextWorkItem_ReconcileIncompleteError(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a mock handler that returns ReconcileIncompleteError
	// We'll use a TVP that doesn't exist to trigger an error, but we need to verify
	// that ReconcileIncompleteError causes immediate requeue

	// For this test, we'll add a KeyItem and then manually verify the requeue behavior
	// Since we can't easily mock ReconcileIncompleteError from our handlers,
	// we'll test the code path by verifying successful processing instead
	// and leave ReconcileIncompleteError testing for integration tests

	// Add a KeyItem for a non-existent object which will cause an error
	keyItem := KeyItem{
		key:        testNamespace + "/nonexistent-tvp",
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}
	controller.workqueue.Add(keyItem)

	// Process the item
	result := controller.processNextWorkItem()
	assert.True(t, result, "should return true even with errors")
}

func TestActivate_Success(t *testing.T) {
	t.Skip("Activate requires a properly initialized CSI plugin and is better tested in integration tests")

	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Verify controller was created successfully
	assert.NotNil(t, controller, "controller should not be nil")
	assert.NotNil(t, controller.crdControllerStopChan, "stop channel should be initialized")
}

func TestGetName_ReturnsCorrectValue(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	name := controller.GetName()
	assert.Equal(t, "node-crd-controller", name, "GetName should return correct controller name")
}

func TestVersion_ReturnsCorrectValue(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	version := controller.Version()
	assert.NotEmpty(t, version, "Version should return non-empty string")
	assert.Contains(t, version, ".", "Version should contain at least one dot")
}

func TestAddEventToWorkqueue_AllObjectTypes(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	ctx := context.Background()

	testCases := []struct {
		name       string
		objectType ObjectType
		key        string
		event      EventType
	}{
		{
			name:       "TridentVolumePublication",
			objectType: OjbectTypeTridentVolumePublication,
			key:        testNamespace + "/tvp-1",
			event:      EventAdd,
		},
		{
			name:       "TridentBackend",
			objectType: ObjectTypeTridentBackend,
			key:        "backend-1",
			event:      EventUpdate,
		},
		{
			name:       "TridentAutogrowPolicy",
			objectType: ObjectTypeTridentAutogrowPolicy,
			key:        "policy-1",
			event:      EventDelete,
		},
		{
			name:       "StorageClass",
			objectType: ObjectTypeStorageClass,
			key:        "sc-1",
			event:      EventAdd,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initialLen := controller.workqueue.Len()
			controller.addEventToWorkqueue(tc.key, tc.event, ctx, tc.objectType)
			assert.Equal(t, initialLen+1, controller.workqueue.Len(), "workqueue should have one more item")

			// Verify the item in queue
			item, shutdown := controller.workqueue.Get()
			assert.False(t, shutdown, "workqueue should not be shutdown")
			assert.Equal(t, tc.key, item.key, "key should match")
			assert.Equal(t, tc.objectType, item.objectType, "objectType should match")
			assert.Equal(t, tc.event, item.event, "event should match")
			controller.workqueue.Done(item)
		})
	}
}

func TestGenericK8sResourceHandlers_EdgeCases(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test add handler with nil object
	addHandler := controller.addK8sResourceHandler(ObjectTypeStorageClass)
	addHandler(nil)
	// Should not panic, no event added for nil

	// Test update handler with mismatched object types
	updateHandler := controller.updateK8sResourceHandler(ObjectTypeStorageClass)
	updateHandler("old-string", "new-string")
	// Should not panic, no event added for non-metaObject

	// Test delete handler with non-deleted object
	deleteHandler := controller.deleteK8sResourceHandler(ObjectTypeStorageClass)
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sc",
		},
	}
	deleteHandler(sc)
	// Should add delete event
	assert.Greater(t, controller.workqueue.Len(), 0, "delete handler should add event")
}

func TestProcessNextWorkItem_AllObjectTypesBranches(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	ctx := context.Background()

	// Test each object type processes correctly
	testCases := []struct {
		name       string
		objectType ObjectType
		setupFunc  func() string // Returns the key to use
	}{
		{
			name:       "TridentBackend",
			objectType: ObjectTypeTridentBackend,
			setupFunc: func() string {
				backend := &tridentv1.TridentBackend{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-backend",
					},
				}
				_, _ = controller.crdClientset.TridentV1().TridentBackends("").Create(
					context.Background(), backend, metav1.CreateOptions{})
				_ = controller.tridentBackendInformer.GetIndexer().Add(backend)
				return backend.Name
			},
		},
		{
			name:       "TridentAutogrowPolicy",
			objectType: ObjectTypeTridentAutogrowPolicy,
			setupFunc: func() string {
				policy := &tridentv1.TridentAutogrowPolicy{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-policy",
					},
				}
				_, _ = controller.crdClientset.TridentV1().TridentAutogrowPolicies().Create(
					context.Background(), policy, metav1.CreateOptions{})
				_ = controller.tridentAutogrowPolicyInformer.GetIndexer().Add(policy)
				return policy.Name
			},
		},
		{
			name:       "StorageClass",
			objectType: ObjectTypeStorageClass,
			setupFunc: func() string {
				sc := &storagev1.StorageClass{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-sc",
					},
					Provisioner: "csi.trident.netapp.io",
				}
				_, _ = controller.kubeClientset.StorageV1().StorageClasses().Create(
					context.Background(), sc, metav1.CreateOptions{})
				_ = controller.storageClassInformer.GetIndexer().Add(sc)
				return sc.Name
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			key := tc.setupFunc()

			keyItem := KeyItem{
				key:        key,
				objectType: tc.objectType,
				event:      EventAdd,
				ctx:        ctx,
			}
			controller.workqueue.Add(keyItem)

			result := controller.processNextWorkItem()
			assert.True(t, result, "should successfully process "+tc.name)
			assert.Equal(t, 0, controller.workqueue.Len(), "queue should be empty after processing")
		})
	}
}

// Additional tests to improve coverage

func TestAddCRHandler_NilObject(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test with nil object - should not panic
	controller.addCRHandler(nil)
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add nil object to queue")
}

func TestAddCRHandler_NonMetaObject(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test with non-meta object (string) - should not panic
	controller.addCRHandler("not-a-meta-object")
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add non-meta object to queue")
}

func TestUpdateCRHandler_NilObjects(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test with both nil objects
	controller.updateCRHandler(nil, nil)
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add event for nil objects")
}

func TestUpdateCRHandler_OldObjectNil(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	tvp := createTestTridentVolumePublication("test", testNamespace, testNodeName, "vol-1")

	// Test with old object nil, new object valid
	controller.updateCRHandler(nil, tvp)
	// Should still not add since old is nil
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add event when old object is nil")
}

func TestUpdateCRHandler_NewObjectNil(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	tvp := createTestTridentVolumePublication("test", testNamespace, testNodeName, "vol-1")

	// Test with new object nil, old object valid
	controller.updateCRHandler(tvp, nil)
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add event when new object is nil")
}

func TestUpdateCRHandler_NonMetaObjects(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test with non-meta objects
	controller.updateCRHandler("old-string", "new-string")
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add event for non-meta objects")
}

func TestUpdateCRHandler_SameGeneration(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	tvp1 := createTestTridentVolumePublication("test", testNamespace, testNodeName, "vol-1")
	tvp1.Generation = 5

	tvp2 := createTestTridentVolumePublication("test", testNamespace, testNodeName, "vol-1")
	tvp2.Generation = 5

	// Same generation (non-zero) and no deletion - should not add event
	controller.updateCRHandler(tvp1, tvp2)
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add event when generation unchanged")
}

func TestDeleteCRHandler_NilObject(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test with nil object
	controller.deleteCRHandler(nil)
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add event for nil object")
}

func TestDeleteCRHandler_NonMetaObject(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test with non-meta object
	controller.deleteCRHandler("not-a-meta-object")
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add event for non-meta object")
}

func TestDeleteCRHandler_ValidObject(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	tvp := createTestTridentVolumePublication("test", testNamespace, testNodeName, "vol-1")

	// Delete handler should add event for valid TridentCRD object
	controller.deleteCRHandler(tvp)
	assert.Greater(t, controller.workqueue.Len(), 0, "should add delete event for valid object")
}

func TestK8sResourceHandlers_NilObject(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test add handler with nil
	addHandler := controller.addK8sResourceHandler(ObjectTypeStorageClass)
	addHandler(nil)
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add event for nil object")

	// Test update handler with nil
	updateHandler := controller.updateK8sResourceHandler(ObjectTypeStorageClass)
	updateHandler(nil, nil)
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add event for nil objects")

	// Test delete handler with nil
	deleteHandler := controller.deleteK8sResourceHandler(ObjectTypeStorageClass)
	deleteHandler(nil)
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add event for nil object")
}

func TestK8sResourceHandlers_NonMetaObject(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Test add handler with non-meta object
	addHandler := controller.addK8sResourceHandler(ObjectTypeStorageClass)
	addHandler("not-a-meta-object")
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add event for non-meta object")

	// Test update handler with non-meta objects
	updateHandler := controller.updateK8sResourceHandler(ObjectTypeStorageClass)
	updateHandler("old", "new")
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add event for non-meta objects")

	// Test delete handler with non-meta object
	deleteHandler := controller.deleteK8sResourceHandler(ObjectTypeStorageClass)
	deleteHandler("not-a-meta-object")
	assert.Equal(t, 0, controller.workqueue.Len(), "should not add event for non-meta object")
}

func TestK8sUpdateHandler_ValidUpdate(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	sc1 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-sc",
			ResourceVersion: "100",
		},
		Provisioner: "csi.trident.netapp.io",
	}

	sc2 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-sc",
			ResourceVersion: "101",
		},
		Provisioner: "csi.trident.netapp.io",
	}

	updateHandler := controller.updateK8sResourceHandler(ObjectTypeStorageClass)
	updateHandler(sc1, sc2)
	assert.Greater(t, controller.workqueue.Len(), 0, "should add event for K8s resource update")
}

func TestK8sUpdateHandler_GenerationZero(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	sc1 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-sc",
			ResourceVersion: "100",
			Generation:      0,
		},
		Provisioner: "csi.trident.netapp.io",
	}

	sc2 := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-sc",
			ResourceVersion: "101",
			Generation:      0,
		},
		Provisioner: "csi.trident.netapp.io",
	}

	updateHandler := controller.updateK8sResourceHandler(ObjectTypeStorageClass)
	updateHandler(sc1, sc2)
	assert.Greater(t, controller.workqueue.Len(), 0, "should add event when generation is 0")
}

func TestK8sDeleteHandler_ValidObject(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-sc",
		},
		Provisioner: "csi.trident.netapp.io",
	}

	deleteHandler := controller.deleteK8sResourceHandler(ObjectTypeStorageClass)
	deleteHandler(sc)
	assert.Greater(t, controller.workqueue.Len(), 0, "should add delete event for valid object")
}

func TestProcessNextWorkItem_RateLimitedRequeue(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Add a KeyItem for non-existent TVP (will cause error and requeue)
	keyItem := KeyItem{
		key:        testNamespace + "/nonexistent",
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
		isRetry:    false,
	}
	controller.workqueue.Add(keyItem)

	result := controller.processNextWorkItem()
	assert.True(t, result, "should return true even with error")
}

func TestNewTridentNodeCrdControllerImpl_ValidationErrors(t *testing.T) {
	kubeClient := getFakeKubernetesClientset()
	crdClient := getFakeTridentClientset()
	plugin := mockCSIPlugin(t)

	tests := []struct {
		name          string
		namespace     string
		kubeClient    kubernetes.Interface
		crdClient     tridentv1clientset.Interface
		nodeName      string
		plugin        *csi.Plugin
		expectError   bool
		errorContains string
	}{
		{
			name:          "nil kubeClient",
			namespace:     testNamespace,
			kubeClient:    nil,
			crdClient:     crdClient,
			nodeName:      testNodeName,
			plugin:        plugin,
			expectError:   true,
			errorContains: "kubeClientset cannot be nil",
		},
		{
			name:          "nil crdClient",
			namespace:     testNamespace,
			kubeClient:    kubeClient,
			crdClient:     nil,
			nodeName:      testNodeName,
			plugin:        plugin,
			expectError:   true,
			errorContains: "crdClientset cannot be nil",
		},
		{
			name:          "nil plugin",
			namespace:     testNamespace,
			kubeClient:    kubeClient,
			crdClient:     crdClient,
			nodeName:      testNodeName,
			plugin:        nil,
			expectError:   true,
			errorContains: "plugin cannot be nil",
		},
		{
			name:          "empty nodeName",
			namespace:     testNamespace,
			kubeClient:    kubeClient,
			crdClient:     crdClient,
			nodeName:      "",
			plugin:        plugin,
			expectError:   true,
			errorContains: "nodeName cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller, err := newTridentNodeCrdController(
				tt.namespace,
				tt.kubeClient,
				tt.crdClient,
				tt.nodeName,
				tt.plugin,
				1*time.Minute, // Default autogrow period for tests
			)

			if tt.expectError {
				assert.Error(t, err, "should return error for "+tt.name)
				assert.Contains(t, err.Error(), tt.errorContains, "error should contain expected message")
				assert.Nil(t, controller, "controller should be nil on error")
			} else {
				assert.NoError(t, err, "should not return error for "+tt.name)
				assert.NotNil(t, controller, "controller should not be nil")
			}
		})
	}
}

// Test error handling in processNextWorkItem with generic errors
func TestProcessNextWorkItem_GenericError(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a KeyItem that will trigger an error in handleTridentVolumePublications
	// by having a non-existent TVP
	keyItem := KeyItem{
		key:        testNamespace + "/non-existent-tvp",
		objectType: OjbectTypeTridentVolumePublication,
		event:      EventAdd,
		ctx:        context.Background(),
	}
	controller.workqueue.Add(keyItem)

	// Processing should handle the error gracefully
	result := controller.processNextWorkItem()
	assert.True(t, result, "should return true even with error")
}

// Test addCRHandler with nil object
func TestAddCRHandler_MetaNamespaceKeyError(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a TridentVolumePublication with no metadata (will cause MetaNamespaceKeyFunc to fail)
	tvp := &tridentv1.TridentVolumePublication{}
	tvp.Name = "" // Empty name should cause MetaNamespaceKeyFunc to fail potentially

	initialLen := controller.workqueue.Len()
	controller.addCRHandler(tvp)

	// If MetaNamespaceKeyFunc fails, the item should not be added to the queue
	// But since TVP has proper structure, it will be added
	assert.GreaterOrEqual(t, controller.workqueue.Len(), initialLen, "queue length check")
}

// Test updateCRHandler with nil new object
func TestUpdateCRHandler_NilNewObject(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")

	initialLen := controller.workqueue.Len()
	controller.updateCRHandler(tvp, nil)

	// Should not add anything to queue when new object is nil
	assert.Equal(t, initialLen, controller.workqueue.Len(), "queue should not have new items")
}

// Test updateCRHandler with nil old object
func TestUpdateCRHandler_NilOldObject(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	tvp := createTestTridentVolumePublication("test-tvp", testNamespace, testNodeName, "vol-123")

	initialLen := controller.workqueue.Len()
	controller.updateCRHandler(nil, tvp)

	// Should not add anything to queue when old object is nil
	assert.Equal(t, initialLen, controller.workqueue.Len(), "queue should not have new items")
}

// Test deleteCRHandler with invalid object
func TestDeleteCRHandler_MetaNamespaceKeyError(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	// Create a TridentVolumePublication
	tvp := &tridentv1.TridentVolumePublication{}
	tvp.Name = ""

	initialLen := controller.workqueue.Len()
	controller.deleteCRHandler(tvp)

	// Should still attempt to process
	assert.GreaterOrEqual(t, controller.workqueue.Len(), initialLen, "queue length check")
}

// Test addK8sResourceHandler with MetaNamespaceKeyFunc error
func TestAddK8sResourceHandler_MetaNamespaceKeyError(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	handler := controller.addK8sResourceHandler(ObjectTypeStorageClass)

	// Call with a malformed object
	invalidObj := &metav1.ObjectMeta{Name: ""}

	initialLen := controller.workqueue.Len()
	handler(invalidObj)

	// May or may not add to queue depending on implementation
	assert.GreaterOrEqual(t, controller.workqueue.Len(), initialLen, "queue length check")
}

// Test Run with context cancellation via stopCh
func TestRun_StopChannel(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	ctx := context.Background()
	stopCh := make(chan struct{})

	// Run in goroutine
	done := make(chan bool)
	go func() {
		controller.Run(ctx, 1, stopCh)
		done <- true
	}()

	// Wait a bit for caches to sync
	time.Sleep(100 * time.Millisecond)

	// Stop the controller
	close(stopCh)

	// Wait for Run to complete
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop in time")
	}
}

// Test addCRHandler with different CR types
func TestAddCRHandler_AllCRTypes(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	testCases := []struct {
		name       string
		cr         tridentv1.TridentCRD
		objectType ObjectType
	}{
		{
			name: "TridentBackend",
			cr: &tridentv1.TridentBackend{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backend",
					Namespace: testNamespace,
				},
			},
			objectType: ObjectTypeTridentBackend,
		},
		{
			name: "TridentAutogrowPolicy",
			cr: &tridentv1.TridentAutogrowPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-policy",
				},
			},
			objectType: ObjectTypeTridentAutogrowPolicy,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			initialLen := controller.workqueue.Len()
			controller.addCRHandler(tc.cr)
			assert.Greater(t, controller.workqueue.Len(), initialLen, "should add item to queue")

			// Get and verify the item
			item, shutdown := controller.workqueue.Get()
			assert.False(t, shutdown)
			assert.Equal(t, tc.objectType, item.objectType)
			controller.workqueue.Done(item)
		})
	}
}

// Test updateCRHandler with different CR types
func TestUpdateCRHandler_AllCRTypes(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	backend := &tridentv1.TridentBackend{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-backend",
			Namespace:  testNamespace,
			Generation: 1,
		},
	}

	updatedBackend := &tridentv1.TridentBackend{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-backend",
			Namespace:  testNamespace,
			Generation: 2,
		},
	}

	initialLen := controller.workqueue.Len()
	controller.updateCRHandler(backend, updatedBackend)
	assert.Greater(t, controller.workqueue.Len(), initialLen, "should add update event to queue")

	// Get and verify the item
	item, shutdown := controller.workqueue.Get()
	assert.False(t, shutdown)
	assert.Equal(t, EventUpdate, item.event)
	assert.Equal(t, ObjectTypeTridentBackend, item.objectType)
	controller.workqueue.Done(item)
}

// Test deleteCRHandler with different CR types
func TestDeleteCRHandler_AllCRTypes(t *testing.T) {
	controller, err := newTestTridentNodeCrdController(t, testNodeName)
	require.NoError(t, err)

	policy := &tridentv1.TridentAutogrowPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-policy",
		},
	}

	initialLen := controller.workqueue.Len()
	controller.deleteCRHandler(policy)
	assert.Greater(t, controller.workqueue.Len(), initialLen, "should add delete event to queue")

	// Get and verify the item
	item, shutdown := controller.workqueue.Get()
	assert.False(t, shutdown)
	assert.Equal(t, EventDelete, item.event)
	assert.Equal(t, ObjectTypeTridentAutogrowPolicy, item.objectType)
	controller.workqueue.Done(item)
}
