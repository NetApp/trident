package storageclass

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockstorage "github.com/netapp/trident/mocks/mock_storage"
	"github.com/netapp/trident/storage"
)

var ctx = context.Background

func TestNewPoolMap(t *testing.T) {
	poolMap := NewPoolMap()
	assert.NotNil(t, poolMap)
	assert.Empty(t, poolMap.poolMap)
}

func makeSyncMap(pools map[string]storage.Pool) *sync.Map {
	syncMap := new(sync.Map)
	for name, pool := range pools {
		syncMap.Store(name, pool)
	}
	return syncMap
}

func TestPoolMap_DeepCopy(t *testing.T) {
	original := NewPoolMap()
	original.poolMap["sc1"] = map[string][]string{
		"backend1": {"pool1", "pool2"},
	}

	clone := original.DeepCopy()
	assert.Equal(t, original, clone)
	assert.NotSame(t, original, clone)
	assert.NotSame(t, &original.poolMap, &clone.poolMap)
}

func TestPoolMap_Rebuild(t *testing.T) {
	ctx := context.Background()
	storageClasses := []*StorageClass{
		{config: &Config{Name: "sc1"}},
	}

	mockCtrl := gomock.NewController(t)

	// Create a fake backend with pools
	mockBackend1 := mockstorage.NewMockBackend(mockCtrl)
	mockBackend2 := mockstorage.NewMockBackend(mockCtrl)
	mockPool1 := mockstorage.NewMockPool(mockCtrl)
	mockPool2 := mockstorage.NewMockPool(mockCtrl)

	mockBackend1.EXPECT().State().Return(storage.BackendState("online")).AnyTimes()
	mockBackend1.EXPECT().StoragePools().Return(makeSyncMap(map[string]storage.Pool{"pool1": mockPool1})).AnyTimes()
	mockBackend1.EXPECT().Name().Return("backend1").AnyTimes()

	mockBackend2.EXPECT().State().Return(storage.BackendState("offline")).AnyTimes()
	mockBackend2.EXPECT().StoragePools().Return(makeSyncMap(map[string]storage.Pool{"pool1": mockPool1})).AnyTimes()
	mockBackend2.EXPECT().Name().Return("backend1").AnyTimes()

	mockPool1.EXPECT().Name().Return("pool1").AnyTimes()
	mockPool1.EXPECT().Backend().Return(mockBackend1).AnyTimes()

	mockPool2.EXPECT().Name().Return("pool2").AnyTimes()
	mockPool2.EXPECT().Backend().Return(mockBackend1).AnyTimes()

	backends := []storage.Backend{mockBackend1, mockBackend2}

	actualMap := NewPoolMap()
	actualMap.Rebuild(ctx, storageClasses, backends)

	expectedMap := NewPoolMap()
	expectedMap.poolMap["sc1"] = map[string][]string{"backend1": {"pool1"}}

	assert.Equal(t, expectedMap, actualMap)
}

func TestPoolMap_BackendPoolMapForStorageClass(t *testing.T) {
	ctx := context.Background()
	poolMap := NewPoolMap()
	poolMap.poolMap["sc1"] = map[string][]string{
		"backend1": {"pool1", "pool2"},
	}

	result := poolMap.BackendPoolMapForStorageClass(ctx, "sc1")
	expected := map[string][]string{
		"backend1": {"pool1", "pool2"},
	}
	assert.Equal(t, expected, result)

	result = poolMap.BackendPoolMapForStorageClass(ctx, "sc2")
	assert.Nil(t, result)
}

func TestPoolMap_StorageClassNamesForPoolName(t *testing.T) {
	ctx := context.Background()
	poolMap := NewPoolMap()
	poolMap.poolMap["sc1"] = map[string][]string{
		"backend1": {"pool1", "pool2"},
	}
	poolMap.poolMap["sc2"] = map[string][]string{
		"backend1": {"pool2"},
	}

	result := poolMap.StorageClassNamesForPoolName(ctx, "backend1", "pool2")
	expected := []string{"sc1", "sc2"}
	assert.ElementsMatch(t, expected, result)

	result = poolMap.StorageClassNamesForPoolName(ctx, "backend1", "pool3")
	assert.Empty(t, result)
}

func TestPoolMap_StorageClassNamesForBackendName(t *testing.T) {
	ctx := context.Background()
	poolMap := NewPoolMap()
	poolMap.poolMap["sc1"] = map[string][]string{
		"backend1": {"pool1", "pool2"},
	}
	poolMap.poolMap["sc2"] = map[string][]string{
		"backend1": {"pool2"},
	}

	result := poolMap.StorageClassNamesForBackendName(ctx, "backend1")

	assert.Contains(t, result, "pool1")
	assert.Contains(t, result, "pool2")
	assert.ElementsMatch(t, result["pool1"], []string{"sc1"})
	assert.ElementsMatch(t, result["pool2"], []string{"sc1", "sc2"})

	result = poolMap.StorageClassNamesForBackendName(ctx, "backend2")
	assert.Empty(t, result)
}

func TestBackendMatchesStorageClass(t *testing.T) {
	// Create a new PoolMap and populate it with test data
	poolMap := NewPoolMap()
	poolMap.poolMap["sc1"] = map[string][]string{
		"backend1": {"pool1", "pool2"},
		"backend2": {"pool3"},
	}
	poolMap.poolMap["sc2"] = map[string][]string{
		"backend1": {"pool4"},
	}

	// Test cases
	tests := []struct {
		name             string
		backendName      string
		storageClassName string
		expectedResult   bool
	}{
		{
			name:             "Matching backend and storage class",
			backendName:      "backend1",
			storageClassName: "sc1",
			expectedResult:   true,
		},
		{
			name:             "Non-matching backend for storage class",
			backendName:      "backend3",
			storageClassName: "sc1",
			expectedResult:   false,
		},
		{
			name:             "Non existent storage class name",
			backendName:      "backend1",
			storageClassName: "invalidStorageClass",
			expectedResult:   false,
		},
		{
			name:             "Non-existent storage class",
			backendName:      "backend1",
			storageClassName: "sc3",
			expectedResult:   false,
		},
		{
			name:             "Matching backend and storage class with single pool",
			backendName:      "backend2",
			storageClassName: "sc1",
			expectedResult:   true,
		},
		{
			name:             "Matching backend and storage class with different pool",
			backendName:      "backend1",
			storageClassName: "sc2",
			expectedResult:   true,
		},
		{
			name:             "Empty backend name",
			backendName:      "",
			storageClassName: "sc1",
			expectedResult:   false,
		},
		{
			name:             "Empty storage class name",
			backendName:      "backend1",
			storageClassName: "",
			expectedResult:   false,
		},
	}

	// Run test cases
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := poolMap.BackendMatchesStorageClass(nil, test.backendName, test.storageClassName)
			assert.Equal(t, test.expectedResult, result)
		})
	}
}
