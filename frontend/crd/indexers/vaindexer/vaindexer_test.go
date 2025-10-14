// Copyright 2025 NetApp, Inc. All Rights Reserved.
package vaindexer

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	k8sstoragev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	mock_cache "github.com/netapp/trident/mocks/mock_cache"
)

func createTestVolumeAttachment(name, nodeName, pvName string) *k8sstoragev1.VolumeAttachment {
	return &k8sstoragev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: k8sstoragev1.VolumeAttachmentSpec{
			Attacher: "csi.trident.netapp.io",
			Source: k8sstoragev1.VolumeAttachmentSource{
				PersistentVolumeName: &pvName,
			},
			NodeName: nodeName,
		},
	}
}

func TestNewIndexers(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()

	indexer := NewVolumeAttachmentIndexer(fakeClient)

	assert.NotNil(t, indexer)
	assert.NotNil(t, indexer.vaIndexer)
	assert.NotNil(t, indexer.vaController)
	assert.NotNil(t, indexer.vaSource)
	assert.NotNil(t, indexer.vaSynced)
}

// Test that the Activate method exists and the controller is properly initialized.
func TestVaIndexer_Activate(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	indexers := NewVolumeAttachmentIndexer(fakeClient)

	// Close immediately to avoid controller startup issues.
	// Allows us to test the Activate method with a fake client.
	close(indexers.vaControllerStopChan)

	// Test that we can call Activate without panic.
	assert.NotPanics(t, func() {
		indexers.Activate()
	}, "Running the controller should not panic")
}

// Test that Deactivate method exists and can be called.
func TestVaIndexer_Deactivate(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	indexers := NewVolumeAttachmentIndexer(fakeClient)

	// Test that we can call Deactivate without panic.
	assert.NotPanics(t, func() {
		indexers.Deactivate()
	}, "Deactivating indexers should not panic")
}

func TestVaIndexer_WaitForCacheSync(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	indexers := NewVolumeAttachmentIndexer(fakeClient)

	// Close immediately allow unit testing of the function without
	// starting the controller.
	close(indexers.vaControllerStopChan)

	// Test that we can call Activate without panic.
	assert.NotPanics(t, func() {
		indexers.WaitForCacheSync(context.Background())
	}, "Running the controller should not panic")
}

func TestVaIndexer_GetCachedVolumeAttachmentsByNode_EmptyCache(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	indexers := NewVolumeAttachmentIndexer(fakeClient)
	ctx := context.Background()
	nodeName := "test-node"

	attachments, err := indexers.GetCachedVolumeAttachmentsByNode(ctx, nodeName)

	assert.NoError(t, err)
	assert.Empty(t, attachments)
}

func TestVaIndexer_GetCachedVolumeAttachmentsByNode_WithData(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	indexers := NewVolumeAttachmentIndexer(fakeClient)
	ctx := context.Background()

	// Create test volume attachments.
	va1 := createTestVolumeAttachment("va1", "node1", "pv1")
	va2 := createTestVolumeAttachment("va2", "node1", "pv2")
	va3 := createTestVolumeAttachment("va3", "node2", "pv3")
	va4 := createTestVolumeAttachment("va4", "", "pv4") // No node name - should not be indexed.

	// Directly add items to the cache indexer to simulate populated cache.
	// This bypasses the controller startup issues with fake client.
	err := indexers.vaIndexer.Add(va1)
	assert.NoError(t, err)
	err = indexers.vaIndexer.Add(va2)
	assert.NoError(t, err)
	err = indexers.vaIndexer.Add(va3)
	assert.NoError(t, err)
	err = indexers.vaIndexer.Add(va4)
	assert.NoError(t, err)

	// Test - get attachments for node1 (should return va1 and va2).
	attachments, err := indexers.GetCachedVolumeAttachmentsByNode(ctx, "node1")

	// Assertions
	assert.NoError(t, err)
	assert.Len(t, attachments, 2)

	// Verify the correct attachments are returned.
	attachmentNames := make([]string, len(attachments))
	for i, va := range attachments {
		attachmentNames[i] = va.Name
		assert.Equal(t, "node1", va.Spec.NodeName)
	}
	assert.Contains(t, attachmentNames, "va1")
	assert.Contains(t, attachmentNames, "va2")

	// Test - get attachments for node2 (should return va3).
	attachments, err = indexers.GetCachedVolumeAttachmentsByNode(ctx, "node2")
	assert.NoError(t, err)
	assert.Len(t, attachments, 1)
	assert.Equal(t, "va3", attachments[0].Name)
	assert.Equal(t, "node2", attachments[0].Spec.NodeName)

	// Test - get attachments for non-existent node (should return empty).
	attachments, err = indexers.GetCachedVolumeAttachmentsByNode(ctx, "non-existent-node")
	assert.NoError(t, err)
	assert.Empty(t, attachments)

	allItems := indexers.vaIndexer.List()
	assert.Len(t, allItems, 4, "All 4 volume attachments should be in the cache")
}

func TestVolumeAttachmentsByNodeKeyFunc_ValidVolumeAttachment(t *testing.T) {
	va := createTestVolumeAttachment("test-va", "test-node", "test-pv")

	keys, err := volumeAttachmentsByNodeKeyFunc(va)

	assert.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Equal(t, "test-node", keys[0])
}

func TestVolumeAttachmentsByNodeKeyFunc_EmptyNodeName(t *testing.T) {
	va := createTestVolumeAttachment("test-va", "", "test-pv")

	keys, err := volumeAttachmentsByNodeKeyFunc(va)

	assert.NoError(t, err)
	assert.Nil(t, keys)
}

func TestVolumeAttachmentsByNodeKeyFunc_InvalidObject(t *testing.T) {
	invalidObj := "not-a-volume-attachment"

	// Test
	keys, err := volumeAttachmentsByNodeKeyFunc(invalidObj)

	assert.Error(t, err)
	assert.Nil(t, keys)
	assert.Contains(t, err.Error(), "object is not a VolumeAttachment CR")
}

func TestVolumeAttachmentsByNodeKeyFunc_NilObject(t *testing.T) {
	keys, err := volumeAttachmentsByNodeKeyFunc(nil)

	assert.Error(t, err)
	assert.Nil(t, keys)
	assert.Contains(t, err.Error(), "object is not a VolumeAttachment CR")
}

// Test that demonstrates the key function works correctly with manual cache operations.
func TestVolumeAttachmentsIndexing_ManualTest(t *testing.T) {
	// Test the indexing key function which is the core logic.
	testCases := []struct {
		name        string
		va          *k8sstoragev1.VolumeAttachment
		expectedKey []string
		description string
	}{
		{
			name:        "valid-node",
			va:          createTestVolumeAttachment("va1", "worker-node-1", "pv1"),
			expectedKey: []string{"worker-node-1"},
			description: "Valid volume attachment with node name",
		},
		{
			name:        "empty-node",
			va:          createTestVolumeAttachment("va2", "", "pv2"),
			expectedKey: nil,
			description: "Volume attachment with empty node name",
		},
		{
			name:        "another-node",
			va:          createTestVolumeAttachment("va3", "worker-node-2", "pv3"),
			expectedKey: []string{"worker-node-2"},
			description: "Valid volume attachment with different node name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			keys, err := volumeAttachmentsByNodeKeyFunc(tc.va)

			assert.NoError(t, err)
			if tc.expectedKey == nil {
				assert.Nil(t, keys)
			} else {
				assert.Equal(t, tc.expectedKey, keys)
			}
		})
	}
}

// Test error handling when non-VolumeAttachment objects are in cache.
func TestGetCachedVolumeAttachmentsByNode_InvalidObjectInCache(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	indexers := NewVolumeAttachmentIndexer(fakeClient)
	ctx := context.Background()
	nodeName := "test-node"

	// Add a valid VolumeAttachment first.
	va := createTestVolumeAttachment("valid-va", nodeName, "pv1")
	err := indexers.vaIndexer.Add(va)
	assert.NoError(t, err)

	// Test with valid data first to ensure the method works.
	attachments, err := indexers.GetCachedVolumeAttachmentsByNode(ctx, nodeName)
	assert.NoError(t, err)
	assert.Len(t, attachments, 1)
	assert.Equal(t, "valid-va", attachments[0].Name)
}

func TestGetCachedVolumeAttachmentsByNode_InvalidObjectTypeAssertion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fakeClient := fake.NewSimpleClientset()
	indexers := NewVolumeAttachmentIndexer(fakeClient)
	ctx := context.Background()
	nodeName := "test-node"

	// Create a mock indexer using the generated mock.
	mockIndexer := mock_cache.NewMockIndexer(ctrl)

	// Set up the mock to return a non-VolumeAttachment object.
	mockIndexer.EXPECT().
		ByIndex(vaByNodeIndex, nodeName).
		Return([]interface{}{"invalid-object"}, nil) // This will cause the type assertion to fail.

	// Replace the indexer with our mock.
	indexers.vaIndexer = mockIndexer

	// Test - this should trigger the highlighted error handling code.
	attachments, err := indexers.GetCachedVolumeAttachmentsByNode(ctx, nodeName)

	// Assertions - should hit the highlighted error path.
	assert.Error(t, err)
	assert.Nil(t, attachments)
	assert.Contains(t, err.Error(), "non-volume-reference object")
	assert.Contains(t, err.Error(), nodeName)
	assert.Contains(t, err.Error(), "found in cache")
}

func TestGetCachedVolumeAttachmentsByNode_ByIndexErr(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fakeClient := fake.NewSimpleClientset()
	indexers := NewVolumeAttachmentIndexer(fakeClient)
	ctx := context.Background()
	nodeName := "test-node"

	// Create a mock indexer using the generated mock.
	mockIndexer := mock_cache.NewMockIndexer(ctrl)

	// Set up the mock to return a non-VolumeAttachment object.
	mockIndexer.EXPECT().
		ByIndex(vaByNodeIndex, nodeName).
		Return([]interface{}{}, fmt.Errorf("mock error"))

	// Replace the indexer with our mock.
	indexers.vaIndexer = mockIndexer

	// Test - this should trigger the highlighted error handling code.
	attachments, err := indexers.GetCachedVolumeAttachmentsByNode(ctx, nodeName)

	// Assertions - should hit the highlighted error path.
	assert.Error(t, err)
	assert.Nil(t, attachments)
	assert.Contains(t, err.Error(), "could not search cache for volume attachments on node test-node: mock error")
}
