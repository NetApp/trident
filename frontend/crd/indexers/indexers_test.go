// Copyright 2025 NetApp, Inc. All Rights Reserved.

package indexers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"k8s.io/client-go/kubernetes/fake"

	mock_indexer "github.com/netapp/trident/mocks/mock_frontend/crd/indexers/indexer"
)

func TestNewIndexers(t *testing.T) {
	// Create a fake Kubernetes client
	kubeClient := fake.NewSimpleClientset()

	// Test creating new indexers
	indexers := NewIndexers(kubeClient)

	// Verify indexers is not nil
	require.NotNil(t, indexers)

	// Verify vaIndexer is initialized
	assert.NotNil(t, indexers.vaIndexer)

	// Verify VolumeAttachmentIndexer returns the correct indexer
	vaIndexer := indexers.VolumeAttachmentIndexer()
	assert.NotNil(t, vaIndexer)
	assert.Equal(t, indexers.vaIndexer, vaIndexer)
}

func TestK8sIndexers_Activate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock VolumeAttachmentIndexer
	mockVAIndexer := mock_indexer.NewMockVolumeAttachmentIndexer(ctrl)

	// Create k8sIndexers with mock
	indexers := &k8sIndexers{
		vaIndexer: mockVAIndexer,
	}

	// Set expectation for Activate call
	mockVAIndexer.EXPECT().Activate().Times(1)

	// Test Activate
	indexers.Activate()
}

func TestK8sIndexers_Deactivate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock VolumeAttachmentIndexer
	mockVAIndexer := mock_indexer.NewMockVolumeAttachmentIndexer(ctrl)

	// Create k8sIndexers with mock
	indexers := &k8sIndexers{
		vaIndexer: mockVAIndexer,
	}

	// Set expectation for Deactivate call
	mockVAIndexer.EXPECT().Deactivate().Times(1)

	// Test Deactivate
	indexers.Deactivate()
}

func TestK8sIndexers_VolumeAttachmentIndexer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock VolumeAttachmentIndexer
	mockVAIndexer := mock_indexer.NewMockVolumeAttachmentIndexer(ctrl)

	// Create k8sIndexers with mock
	indexers := &k8sIndexers{
		vaIndexer: mockVAIndexer,
	}

	// Test VolumeAttachmentIndexer returns correct indexer
	result := indexers.VolumeAttachmentIndexer()
	assert.Equal(t, mockVAIndexer, result)
}
