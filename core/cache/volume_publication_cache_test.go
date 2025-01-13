// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/models"
)

func TestVolumePublicationCacheSet(t *testing.T) {
	// Create test variables.
	volumeID := "foo"
	nodeID := "bar"

	// Create test table.
	tests := map[string]struct {
		volumeID    string
		nodeID      string
		publication *models.VolumePublication
		expectError bool
	}{
		"set publication with valid keys": {
			volumeID: volumeID,
			nodeID:   nodeID,
			publication: &models.VolumePublication{
				Name: models.GenerateVolumePublishName(volumeID, nodeID),
			},
			expectError: false,
		},
		"set publication without nodeID": {
			volumeID: volumeID,
			nodeID:   "",
			publication: &models.VolumePublication{
				Name: models.GenerateVolumePublishName(volumeID, ""),
			},
			expectError: true,
		},
		"set publication without volumeID": {
			volumeID: "",
			nodeID:   nodeID,
			publication: &models.VolumePublication{
				Name: models.GenerateVolumePublishName("", nodeID),
			},
			expectError: true,
		},
		"set publication without nodeID or volumeID": {
			volumeID: "",
			nodeID:   "",
			publication: &models.VolumePublication{
				Name: models.GenerateVolumePublishName("", ""),
			},
			expectError: true,
		},
		"set publication without a publication": {
			volumeID:    "foo",
			nodeID:      "bar",
			publication: nil,
			expectError: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Create a publication cache.
			vpc := NewVolumePublicationCache()

			// Extract test variables.
			volumeID, nodeID, publication := test.volumeID, test.nodeID, test.publication

			// Set an entry in the cache.
			err := vpc.Set(volumeID, nodeID, publication)

			// If an error is expected with this case, assert that it exists and exit.
			if test.expectError {
				assert.Error(t, err, "expected an error")
				return
			}

			assert.NoError(t, err, "expected no error")

			// Get the publication to check if the stored value has distinct memory addressing
			// from the original value used in Set.
			cachedPublication, found := vpc.TryGet(volumeID, nodeID)
			if !found {
				t.Fatal("expected entry to already exist in the cache")
			}

			// Change values on the original publication and assert the cachePub doesn't change.
			// This proves that the stored value from a Set is not the same pointer as the value from get.
			publication.Name = models.GenerateVolumePublishName(volumeID, "changed")
			assert.True(t, cachedPublication != publication, "expected different addresses")
			assert.NotEqualValues(t, *cachedPublication, *publication, "expected different values")
		})
	}
}

func TestVolumePublicationCacheGet(t *testing.T) {
	// Create test variables.
	volumeID := "foo"
	nodeID := "bar"
	publication := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeID, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeID,
	}

	// Create a volume publication cache.
	vpc := NewVolumePublicationCache()

	// Verify Get returns nil with invalid inputs or if an entry doesn't exist for valid inputs.
	assert.Nil(t, vpc.Get("", nodeID), "expected nil value")
	assert.Nil(t, vpc.Get(volumeID, nodeID), "expected nil value")

	// Add a new value to the cache and verify the retrieved value.
	if err := vpc.Set(volumeID, nodeID, publication); err != nil {
		t.Fatal("failed to set entry in the cache")
	}
	cachedPublication := vpc.Get(volumeID, nodeID)
	cachedPublication.NodeName = "baz"

	// Verify Get returns an entry for valid inputs and has a different address than the original stored value.
	assert.True(t, cachedPublication != publication, "expected different addresses")
	assert.NotEqualValues(t, *cachedPublication, *publication, "expected different values")
}

func TestVolumePublicationCacheTryGet(t *testing.T) {
	// Create test variables.
	volumeID := "foo"
	nodeID := "bar"
	publication := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeID, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeID,
	}

	// Create a volume publication cache.
	vpc := NewVolumePublicationCache()

	// Test when invalid inputs are supplied.
	cachedPublication, found := vpc.TryGet("", nodeID)
	assert.Nil(t, cachedPublication, "expected nil value")
	assert.True(t, !found, "expected false")

	// Test when valid inputs are supplied, but entry doesn't exist.
	cachedPublication, found = vpc.TryGet(volumeID, nodeID)
	assert.Nil(t, cachedPublication, "expected nil value")
	assert.True(t, !found, "expected false")

	// Add a new value to the cache and verify the retrieved value.
	if err := vpc.Set(volumeID, nodeID, publication); err != nil {
		t.Fatal("failed to set entry in the cache")
	}

	cachedPublication, found = vpc.TryGet(volumeID, nodeID)
	assert.NotNil(t, cachedPublication, "expected non-nil value")
	assert.True(t, found, "expected true")
	cachedPublication.NodeName = "baz"

	// Verify Get returns an entry for valid inputs and has a different address than the original stored value.
	assert.True(t, cachedPublication != publication, "expected different addresses")
	assert.NotEqualValues(t, *cachedPublication, *publication, "expected different values")
}

func TestVolumePublicationCacheDelete(t *testing.T) {
	// Create test variables.
	volumeID := "foo"
	nodeID := "bar"
	publication := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeID, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeID,
	}

	// Create a volume publication cache.
	vpc := NewVolumePublicationCache()

	// Test when invalid keys are supplied.
	err := vpc.Delete("", "nodeID")
	assert.Error(t, err, "expected an error")

	// Test when valid keys are supplied, but entry doesn't exist.
	err = vpc.Delete(volumeID, nodeID)
	assert.NoError(t, err, "expected no error")

	// Test when valid keys are supplied and entry does exist.
	if err = vpc.Set(volumeID, nodeID, publication); err != nil {
		t.Fatal("failed to set entry in the cache")
	}
	totalCacheSize := len(vpc.ListPublications())
	err = vpc.Delete(volumeID, nodeID)
	assert.NoError(t, err, "expected no error")

	// Attempt to Get a now deleted value.
	cachedPublication := vpc.Get(volumeID, nodeID)
	assert.Nil(t, cachedPublication, "expected nil value")

	expectedSizeAfterDelete := len(vpc.ListPublications())
	assert.NotEqual(t, expectedSizeAfterDelete, totalCacheSize, "expected inequality")
}

func TestVolumePublicationCacheListPublications(t *testing.T) {
	// Create test variables.
	volumeID := "foo"
	nodeID := "bar"
	volumeIDTwo := "baz"
	pubOne := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeIDTwo, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeIDTwo,
	}
	pubTwo := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeIDTwo, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeIDTwo,
	}
	data := map[string]map[string]*models.VolumePublication{
		volumeID: {
			nodeID: pubOne,
		},
		volumeIDTwo: {
			nodeID: pubTwo,
		},
	}

	// Create a volume publication cache.
	vpc := NewVolumePublicationCache()

	// List cache values. Right now, the cache should be empty.
	publications := vpc.ListPublications()
	assert.Empty(t, publications, "expected empty list")

	// Set the internal map to values held in data.
	vpc.SetMap(data)

	// Look at each listed publication and ensure the original
	// data contains what we'd expect.
	publications = vpc.ListPublications()
	assert.NotEmpty(t, publications, "expected non-empty list")
	for _, pub := range publications {
		publication := data[pub.VolumeName][pub.NodeName]
		assert.NotNil(t, publication, "expected non-nil value")
	}
	assert.Contains(t, publications, pubOne, "expected list to contain publication")
	assert.Contains(t, publications, pubTwo, "expected list to contain publication")
}

func TestVolumePublicationCacheListPublicationsForVolume(t *testing.T) {
	// Create test variables.
	volumeID := "foo"
	nodeID := "bar"
	volumeIDTwo := "baz"
	pubOne := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeIDTwo, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeIDTwo,
	}
	pubTwo := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeIDTwo, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeIDTwo,
	}
	data := map[string]map[string]*models.VolumePublication{
		volumeID: {
			nodeID: pubOne,
		},
		volumeIDTwo: {
			nodeID: pubTwo,
		},
	}

	tests := map[string]struct {
		volumeID string
		nodeID   string
		data     map[string]map[string]*models.VolumePublication
	}{
		"lists publications when volume ID is valid and cache data exists": {
			volumeID: volumeID,
			data:     data,
		},
		"lists publications when volume ID is valid and cache data doesn't exist": {
			volumeID: volumeID,
			data:     nil,
		},
		"lists publications when volume ID is invalid and cache data exists": {
			volumeID: "",
			data:     data,
		},
		"lists publications when volume ID is invalid and when cache data is empty": {
			volumeID: "",
			data:     nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Extract test variables
			volumeID, data := test.volumeID, test.data

			// Create a volume publication cache.
			vpc := NewVolumePublicationCache()

			// List cache values
			publications := vpc.ListPublicationsForVolume(volumeID)
			assert.Empty(t, publications, "expected empty list")

			// Set the internal map to values held in data.
			vpc.SetMap(data)

			// Look at each listed publication and ensure the original
			// data contains what we'd expect.
			publications = vpc.ListPublicationsForVolume(volumeID)
			if data == nil || volumeID == "" {
				assert.Empty(t, publications, "expected empty list")
				assert.NotContains(t, publications, pubOne, "expected list to not contain publication")
				assert.NotContains(t, publications, pubTwo, "expected list to not contain publication")
			} else {
				assert.NotEmpty(t, publications, "expected non-empty list")
				for _, pub := range publications {
					nodeToPub := data[pub.VolumeName][pub.NodeName]
					assert.NotNil(t, nodeToPub, "expected non-nil value")
				}
				assert.Contains(t, publications, pubOne, "expected list to contain publication")
				assert.Contains(t, publications, pubTwo, "expected list to contain publication")
			}
		})
	}
}

func TestVolumePublicationCacheListPublicationsForNode(t *testing.T) {
	// Create test variables.
	volumeID := "foo"
	nodeID := "bar"
	volumeIDTwo := "baz"
	pubOne := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeIDTwo, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeIDTwo,
	}
	pubTwo := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeIDTwo, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeIDTwo,
	}
	data := map[string]map[string]*models.VolumePublication{
		volumeID: {
			nodeID: pubOne,
		},
		volumeIDTwo: {
			nodeID: pubTwo,
		},
	}

	tests := map[string]struct {
		volumeID string
		nodeID   string
		data     map[string]map[string]*models.VolumePublication
	}{
		"lists publications when node ID is valid and cache data exists": {
			nodeID: nodeID,
			data:   data,
		},
		"lists publications when node ID is valid and cache data doesn't exist": {
			nodeID: nodeID,
			data:   nil,
		},
		"lists publications when node ID is invalid and cache data exists": {
			nodeID: "",
			data:   data,
		},
		"lists publications when node ID is invalid and when cache data is empty": {
			nodeID: "",
			data:   nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Extract test variables
			nodeID, data := test.nodeID, test.data

			// Create a volume publication cache.
			vpc := NewVolumePublicationCache()

			// List cache values
			publications := vpc.ListPublicationsForNode(nodeID)
			assert.Empty(t, publications, "expected empty list")

			// Set the internal map to values held in data.
			vpc.SetMap(data)

			// Look at each listed publication and ensure the original
			// data contains what we'd expect.
			publications = vpc.ListPublicationsForNode(nodeID)
			if data == nil || nodeID == "" {
				assert.Empty(t, publications, "expected empty list")
				assert.NotContains(t, publications, pubOne, "expected list to not contain publication")
				assert.NotContains(t, publications, pubTwo, "expected list to not contain publication")
			} else {
				assert.NotEmpty(t, publications, "expected non-empty list")
				for _, pub := range publications {
					nodeToPub := data[pub.VolumeName][pub.NodeName]
					assert.NotNil(t, nodeToPub, "expected non-nil value")
				}
				assert.Contains(t, publications, pubOne, "expected list to contain publication")
				assert.Contains(t, publications, pubTwo, "expected list to contain publication")
			}
		})
	}
}

func TestVolumePublicationCacheLen(t *testing.T) {
	// Create test variables.
	volumeID := "foo"
	nodeID := "bar"
	publication := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeID, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeID,
	}

	volumeIDTwo := "baz"
	publicationTwo := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeIDTwo, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeIDTwo,
	}

	// Create a map to set the internal cache to.
	data := map[string]map[string]*models.VolumePublication{
		volumeID: {
			nodeID: publication,
		},
		volumeIDTwo: {
			nodeID: publicationTwo,
		},
	}

	// Create a volume publication cache and set the internal map to values held in data.
	vpc := NewVolumePublicationCache()
	vpc.SetMap(data)
	assert.Equal(t, len(data), len(vpc.ListPublications()))
	assert.Equal(t, len(data[volumeID]), len(vpc.ListPublicationsForVolume(volumeID)))
	assert.NotNil(t, 1, vpc.Get(volumeID, nodeID))

	// Build up how many nodes have a volume published to them here.
	nodeCount := 0
	for _, nodes := range data {
		if nodes[nodeID] != nil {
			nodeCount += 1
		}
	}
	assert.Equal(t, nodeCount, len(vpc.ListPublicationsForNode(nodeID)))

	// Delete a volume -> nodeToPublications entry in the cache, then ask for the Len() of
	// that volume map.
	if err := vpc.Delete(volumeID, nodeID); err != nil {
		t.Fatal("unable to delete entry on the cache")
	}
	assert.Nil(t, vpc.Get(volumeID, nodeID), "expected nil value")
}

func TestVolumePublicationCacheMap(t *testing.T) {
	// Create test variables.
	volumeID := "foo"
	nodeID := "bar"
	publication := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeID, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeID,
	}

	volumeIDTwo := "baz"
	publicationTwo := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeIDTwo, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeIDTwo,
	}

	// Create a map to set the internal cache to.
	data := map[string]map[string]*models.VolumePublication{
		volumeID: {
			nodeID: publication,
		},
		volumeIDTwo: {
			nodeID: publicationTwo,
		},
	}

	// Create a volume publication cache and get a copy of the internal map.
	vpc := NewVolumePublicationCache()
	cacheCopy := vpc.Map()
	assert.Empty(t, cacheCopy, "expected empty map")

	// Create a volume publication cache, set the internal map and call Map.
	vpc.SetMap(data)
	cacheCopy = vpc.Map()
	assert.EqualValues(t, data, cacheCopy, "expected map values to be equal")

	// Create a volume publication cache, set the internal map and call Map.
	vpc.SetMap(data)
	cacheCopy = vpc.Map()
	assert.Equal(t, data, cacheCopy)
}

func TestVolumePublicationCacheSetMap(t *testing.T) {
	// Create test variables.
	volumeID := "foo"
	nodeID := "bar"
	publication := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeID, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeID,
	}

	volumeIDTwo := "baz"
	publicationTwo := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeIDTwo, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeIDTwo,
	}

	// Create a map to set the internal cache to.
	data := map[string]map[string]*models.VolumePublication{
		volumeID: {
			nodeID: publication,
		},
		volumeIDTwo: {
			nodeID: publicationTwo,
		},
	}

	// Create a volume publication cache and set the internal map.
	vpc := NewVolumePublicationCache()
	vpc.SetMap(data)
	cachedPublication, found := vpc.TryGet(volumeID, nodeID)
	if !found || cachedPublication == nil {
		t.Fatal("expected to find entry in the cache")
	}
	assert.EqualValues(t, *publication, *cachedPublication, "expected equal values")
	assert.True(t, publication != cachedPublication, "expected different memory addresses")
	assert.Equal(t, len(data), len(vpc.ListPublications()), "expected parity between data and cache size")

	// Delete an entry from the original data map and verify the entry on the cache hasn't changed.
	delete(data[volumeID], nodeID)
	originalPublication := data[volumeID][nodeID]
	cachedPublication = vpc.Get(volumeID, nodeID)
	assert.NotContainsf(t, data[volumeID], nodeID, "expected value to not exist")
	assert.Nil(t, originalPublication, "expected nil value")
	assert.NotNil(t, cachedPublication, "expected non-nil value")

	// Delete an entry from the cache and assert the representation in the original data map hasn't changed.
	err := vpc.Delete(volumeIDTwo, nodeID)
	if err != nil {
		t.Fatal("expected no deletion error")
	}
	cachedPublication = vpc.Get(volumeIDTwo, nodeID)
	originalPublication = data[volumeIDTwo][nodeID]
	assert.NotContainsf(t, vpc.Map()[volumeIDTwo], nodeID, "expected value to not exist")
	assert.Nil(t, cachedPublication, "expected nil value")
	assert.NotNil(t, originalPublication, "expected non-nil value")
}

func TestVolumePublicationCacheClear(t *testing.T) {
	// Create test variables.
	volumeID := "foo"
	nodeID := "bar"
	publication := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volumeID, nodeID),
		NodeName:   nodeID,
		VolumeName: volumeID,
	}

	// Create a volume publication cache.
	vpc := NewVolumePublicationCache()

	// Test when valid keys are supplied and entry does exist.
	if err := vpc.Set(volumeID, nodeID, publication); err != nil {
		t.Fatal("failed to set entry in the cache")
	}
	totalCacheSize := len(vpc.ListPublications())
	vpc.Clear()

	// Attempt to Get a now deleted value.
	cachedPublication := vpc.Get(volumeID, nodeID)
	assert.Nil(t, cachedPublication, "expected nil value")

	expectedSizeAfterDelete := len(vpc.ListPublications())
	assert.NotEqual(t, expectedSizeAfterDelete, totalCacheSize, "expected inequality")
}
