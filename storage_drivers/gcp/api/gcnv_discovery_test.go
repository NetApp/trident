// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"testing"
	"time"

	"github.com/netapp/trident/storage"

	"github.com/stretchr/testify/assert"
)

func setCapacityPools(sdk *Client, capacityPools map[string]*CapacityPool) {
	_, updater, unlocker := sdk.sdkClient.resources.LockAndCheckStale(0)
	updater(time.Now(), capacityPools)
	unlocker()
}

func addCapacityPool(sdk *Client, cPool *CapacityPool) {
	m := make(map[string]*CapacityPool)
	sdk.sdkClient.resources.GetCapacityPools().Range(func(k string, v *CapacityPool) bool {
		m[k] = v
		return true
	})
	m[cPool.FullName] = cPool
	setCapacityPools(sdk, m)
}

func clearCapacityPools(sdk *Client) {
	setCapacityPools(sdk, make(map[string]*CapacityPool))
}

func TestCheckForUnsatisfiedPools_NoPools(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool2 := storage.NewStoragePool(nil, "pool2")

	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2})

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Zero(t, len(result), "expected zero unsatisfied pools")
}

func TestCheckForUnsatisfiedPools_EmptyPools(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool1.InternalAttributes()[capacityPools] = ""
	sPool2 := storage.NewStoragePool(nil, "pool2")
	sPool2.InternalAttributes()[capacityPools] = ""

	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2})

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Zero(t, len(result), "expected zero unsatisfied pools for empty pools")
}

func TestCheckForUnsatisfiedPools_ValidPools(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool1.InternalAttributes()[capacityPools] = "CP1,CP3"
	sPool2 := storage.NewStoragePool(nil, "pool2")
	sPool2.InternalAttributes()[capacityPools] = "CP2,CP4"

	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2})

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Zero(t, len(result), "expected zero unsatisfied pools for valid pools")
}

func TestCheckForUnsatisfiedPools_OneInvalidPool(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool1.InternalAttributes()[capacityPools] = "CP3,CP4"
	sPool2 := storage.NewStoragePool(nil, "pool2")
	sPool2.InternalAttributes()[capacityPools] = "CP5"

	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2})

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Equal(t, 1, len(result), "expected 1 invalid pool error")
}

func TestCheckForUnsatisfiedPools_TwoInvalidPools(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool1.InternalAttributes()[serviceLevel] = ServiceLevelStandard
	sPool1.InternalAttributes()[capacityPools] = "CP3"
	sPool2 := storage.NewStoragePool(nil, "pool2")
	sPool2.InternalAttributes()[capacityPools] = "CP5"

	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2})

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Equal(t, 2, len(result), "expected 2 invalid pools error")
}

func TestCheckForNonexistentCapacityPools_NoPools(t *testing.T) {
	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(make(map[string]storage.Pool))

	result := sdk.checkForNonexistentCapacityPools(ctx)

	assert.False(t, result, "expected no nonexistent capacity pools error")
}

func TestCheckForNonexistentCapacityPools_Empty(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[capacityPools] = ""

	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{"pool": sPool})

	result := sdk.checkForNonexistentCapacityPools(ctx)

	assert.False(t, result, "expected no nonexistent capacity pools for empty pools")
}

func TestCheckForNonexistentCapacityPools_OK(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[capacityPools] = "CP1,CP3"

	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{"pool": sPool})

	result := sdk.checkForNonexistentCapacityPools(ctx)

	assert.False(t, result, "expected no nonexistent capacity pools")
}

func TestCheckForNonexistentCapacityPools_Missing(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[capacityPools] = "CP1,CP3,CP5"

	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{"pool": sPool})

	result := sdk.checkForNonexistentCapacityPools(ctx)

	assert.True(t, result, "expected capacity pools missing error")
}

func TestCheckForNonexistentNetworks_Empty(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[network] = ""

	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{"pool": sPool})

	result := sdk.checkForNonexistentNetworks(ctx)

	assert.False(t, result, "expected no nonexistent network error")
}

func TestCheckForNonexistentNetworks_OK(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[network] = NetworkFullName

	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{"pool": sPool})

	result := sdk.checkForNonexistentNetworks(ctx)

	assert.False(t, result, "expected no non existent network error")
}

func TestCheckForNonexistentNetworks_Missing(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[network] = "projects/123456789/locations/myLocation/networks/missing"

	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{"pool": sPool})

	result := sdk.checkForNonexistentNetworks(ctx)

	assert.True(t, result, "expected network missing error")
}

func TestCapacityPools(t *testing.T) {
	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(make(map[string]storage.Pool))

	expected := &[]*CapacityPool{
		sdk.capacityPool("CP1"),
		sdk.capacityPool("CP2"),
		sdk.capacityPool("CP3"),
		sdk.capacityPool("CP4"),
		sdk.capacityPool("CP6"),
	}
	if sdk.capacityPool("CP5") != nil {
		*expected = append(*expected, sdk.capacityPool("CP5"))
	}

	actual := sdk.CapacityPools()
	assert.ElementsMatch(t, *expected, *actual, "Did not get expected capacity pools")
}

func TestCapacityPool_FullNameAndEmpty(t *testing.T) {
	sdk := getFakeSDK(true)

	// Full name should resolve
	fullName := "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP1"
	assert.Equal(t, sdk.capacityPool("CP1"), sdk.capacityPool(fullName))

	// Empty input should return nil
	assert.Nil(t, sdk.capacityPool(""))
}

func TestCapacityPoolsForStoragePools(t *testing.T) {
	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(make(map[string]storage.Pool))

	CP1 := sdk.capacityPool("CP1")
	CP2 := sdk.capacityPool("CP2")
	CP3 := sdk.capacityPool("CP3")

	sPool1 := storage.NewStoragePool(nil, "testPool1")
	sPool1.InternalAttributes()[capacityPools] = "CP3"

	sPool2 := storage.NewStoragePool(nil, "testPool2")
	sPool2.InternalAttributes()[capacityPools] = "CP1,CP2"

	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{sPool1.Name(): sPool1, sPool2.Name(): sPool2})

	expected := []*CapacityPool{CP1, CP2, CP3}

	actual := sdk.CapacityPoolsForStoragePools(context.TODO())

	assert.ElementsMatch(t, expected, actual, "Did not get expected capacity pools for storage pools")
}

func TestRefreshGCNVResources_CacheNotExpired(t *testing.T) {
	sdk := getFakeSDK()
	sdk.config.MaxCacheAge = 1 * time.Hour

	initialUpdateTime := time.Now()

	// Copy existing capacity pools into a plain map so we can set cache state.
	initialPoolMap := make(map[string]*CapacityPool)
	sdk.sdkClient.resources.GetCapacityPools().Range(func(k string, v *CapacityPool) bool {
		initialPoolMap[k] = v
		return true
	})

	// Mark the cache as fresh (so RefreshGCNVResources should return early without re-discovery).
	_, updater, unlocker := sdk.sdkClient.resources.LockAndCheckStale(0)
	updater(initialUpdateTime, initialPoolMap)
	unlocker()

	// Store references to verify they aren't replaced during refresh
	initialPoolCount := sdk.sdkClient.resources.GetCapacityPools().Length()
	initialPoolMapRef := sdk.sdkClient.resources.GetCapacityPools()
	// Store a specific pool reference to verify it's not replaced
	cp1FullName := "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP1"
	initialCP1 := sdk.sdkClient.resources.GetCapacityPools().Get(cp1FullName)

	err := sdk.RefreshGCNVResources(ctx)
	assert.NoError(t, err, "Should not error when cache is fresh")

	// Verify cache was not refreshed: lastUpdateTime should be unchanged
	assert.Equal(t, initialUpdateTime, sdk.sdkClient.resources.lastUpdateTime,
		"lastUpdateTime should not change when cache is fresh")

	// Verify capacity pool map was not modified (same map reference and count)
	assert.Equal(t, initialPoolCount, sdk.sdkClient.resources.GetCapacityPools().Length(),
		"Capacity pool map should not be refreshed when cache is fresh")
	assert.Equal(t, initialPoolMapRef, sdk.sdkClient.resources.GetCapacityPools(),
		"Capacity pool map reference should not change when cache is fresh")

	// Verify specific pool object is unchanged (not replaced)
	if initialCP1 != nil {
		cp1After := sdk.sdkClient.resources.GetCapacityPools().Get(cp1FullName)
		assert.Equal(t, initialCP1, cp1After,
			"Capacity pool object should not be replaced when cache is fresh")
	}
}

func TestCapacityPoolsForStoragePool(t *testing.T) {
	sdk := getFakeSDK(true)
	CP1 := sdk.capacityPool("CP1")
	CP2 := sdk.capacityPool("CP2")
	CP3 := sdk.capacityPool("CP3")
	CP4 := sdk.capacityPool("CP4")
	CP6 := sdk.capacityPool("CP6")

	sPool := storage.NewStoragePool(nil, "testPool")

	tests := []struct {
		network       string
		capacityPools string
		serviceLevel  string
		expected      []*CapacityPool
	}{
		{
			network:       NetworkName,
			capacityPools: "",
			serviceLevel:  "Premium",
			expected:      []*CapacityPool{CP1},
		},
		{
			network:       "",
			capacityPools: "",
			serviceLevel:  "Standard",
			expected:      []*CapacityPool{CP2, CP6},
		},
		{
			network:       "",
			capacityPools: "",
			serviceLevel:  "Extreme",
			expected:      []*CapacityPool{CP3},
		},
		{
			network:       "",
			capacityPools: "",
			serviceLevel:  "Flex",
			expected:      []*CapacityPool{CP4},
		},
		{
			network:       "new-network",
			capacityPools: "",
			serviceLevel:  "Extreme",
			expected:      []*CapacityPool{},
		},
		{
			network:       NetworkName,
			capacityPools: "",
			serviceLevel:  "Standard",
			expected:      []*CapacityPool{CP2, CP6},
		},
		{
			network:       "fk-network",
			capacityPools: "",
			serviceLevel:  "Flex",
			expected:      []*CapacityPool{},
		},
		{
			network:       NetworkName,
			capacityPools: "CP1",
			serviceLevel:  "Premium",
			expected:      []*CapacityPool{CP1},
		},
		{
			network:       NetworkName,
			capacityPools: "CP5",
			serviceLevel:  "",
			expected:      []*CapacityPool{},
		},
		{
			network:       NetworkName,
			capacityPools: "CP1",
			serviceLevel:  "Premium",
			expected:      []*CapacityPool{CP1},
		},
		{
			network:       NetworkName,
			capacityPools: "CP1",
			serviceLevel:  "Standard",
			expected:      []*CapacityPool{},
		},
	}

	for _, test := range tests {

		sPool.InternalAttributes()[network] = test.network
		sPool.InternalAttributes()[capacityPools] = test.capacityPools

		cPools := sdk.CapacityPoolsForStoragePool(context.TODO(), sPool, test.serviceLevel, "")

		assert.ElementsMatch(t, test.expected, cPools, "Did not get expected capacity pools for storage pools")
	}
}

func TestEnsureVolumeInValidCapacityPool(t *testing.T) {
	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(make(map[string]storage.Pool))

	volume := &Volume{
		Name:         "V1",
		CapacityPool: "CP1",
		Location:     "fake-location",
	}
	assert.Nil(t, sdk.EnsureVolumeInValidCapacityPool(context.TODO(), volume),
		"ensureVolumeInvalidCapacityPool result not nil")

	sPool1 := storage.NewStoragePool(nil, "testPool1")
	sPool1.InternalAttributes()[capacityPools] = "CP3"
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{sPool1.Name(): sPool1})

	assert.NotNil(t, sdk.EnsureVolumeInValidCapacityPool(context.TODO(), volume),
		"ensureVolumeInvalidCapacityPool result nil")

	sPool2 := storage.NewStoragePool(nil, "testPool2")
	sPool2.InternalAttributes()[capacityPools] = "CP1"
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{sPool1.Name(): sPool1, sPool2.Name(): sPool2})

	assert.Nil(t, sdk.EnsureVolumeInValidCapacityPool(context.TODO(), volume),
		"ensureVolumeInvalidCapacityPool result not nil")
}

func TestDumpGCNVResources(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[network] = NetworkFullName

	sdk := getFakeSDK(true)
	sdk.sdkClient.resources.SetStoragePools(map[string]storage.Pool{"pool": sPool})

	discoveryTraceEnabled := sdk.config.DebugTraceFlags["api"]

	driverName := sdk.config.StorageDriverName
	sdk.dumpGCNVResources(ctx, driverName, discoveryTraceEnabled)
}

// Auto-Tiering Discovery Tests

func TestCapacityPoolDiscovery_AutoTieringEnabled(t *testing.T) {
	sdk := getFakeSDK()

	// Create a capacity pool with auto-tiering enabled
	cpWithAutoTiering := &CapacityPool{
		Name:            "CP_AutoTier",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP_AutoTier",
		Location:        Location,
		ServiceLevel:    ServiceLevelPremium,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
		AutoTiering:     true,
	}
	addCapacityPool(sdk, cpWithAutoTiering)

	// Verify the capacity pool has auto-tiering enabled
	discoveredPool := sdk.capacityPool("CP_AutoTier")
	assert.NotNil(t, discoveredPool, "capacity pool should be discovered")
	assert.True(t, discoveredPool.AutoTiering, "auto-tiering should be enabled")
}

func TestCapacityPoolDiscovery_AutoTieringDisabled(t *testing.T) {
	sdk := getFakeSDK()

	// Create a capacity pool with auto-tiering disabled
	cpWithoutAutoTiering := &CapacityPool{
		Name:            "CP_NoAutoTier",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP_NoAutoTier",
		Location:        Location,
		ServiceLevel:    ServiceLevelStandard,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
		AutoTiering:     false,
	}
	addCapacityPool(sdk, cpWithoutAutoTiering)

	// Verify the capacity pool has auto-tiering disabled
	discoveredPool := sdk.capacityPool("CP_NoAutoTier")
	assert.NotNil(t, discoveredPool, "capacity pool should be discovered")
	assert.False(t, discoveredPool.AutoTiering, "auto-tiering should be disabled")
}

func TestCapacityPoolDiscovery_MixedAutoTiering(t *testing.T) {
	sdk := getFakeSDK()

	// Create capacity pools with mixed auto-tiering capabilities
	cpWithAutoTiering := &CapacityPool{
		Name:            "CP_WithAutoTier",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP_WithAutoTier",
		Location:        Location,
		ServiceLevel:    ServiceLevelPremium,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
		AutoTiering:     true,
	}
	cpWithoutAutoTiering := &CapacityPool{
		Name:            "CP_WithoutAutoTier",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP_WithoutAutoTier",
		Location:        Location,
		ServiceLevel:    ServiceLevelStandard,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
		AutoTiering:     false,
	}

	addCapacityPool(sdk, cpWithAutoTiering)
	addCapacityPool(sdk, cpWithoutAutoTiering)

	// Verify both pools are discovered with correct auto-tiering settings
	poolWithAutoTier := sdk.capacityPool("CP_WithAutoTier")
	assert.NotNil(t, poolWithAutoTier, "capacity pool with auto-tiering should be discovered")
	assert.True(t, poolWithAutoTier.AutoTiering, "auto-tiering should be enabled")

	poolWithoutAutoTier := sdk.capacityPool("CP_WithoutAutoTier")
	assert.NotNil(t, poolWithoutAutoTier, "capacity pool without auto-tiering should be discovered")
	assert.False(t, poolWithoutAutoTier.AutoTiering, "auto-tiering should be disabled")
}

func TestCapacityPoolDiscovery_DefaultAutoTieringFalse(t *testing.T) {
	sdk := getFakeSDK(true)

	// Verify existing capacity pools have auto-tiering set to false by default
	// (since they were created without the AutoTiering field in getFakeSDK)
	cp1 := sdk.capacityPool("CP1")
	assert.NotNil(t, cp1, "CP1 should exist")
	assert.False(t, cp1.AutoTiering, "auto-tiering should default to false")

	cp2 := sdk.capacityPool("CP2")
	assert.NotNil(t, cp2, "CP2 should exist")
	assert.False(t, cp2.AutoTiering, "auto-tiering should default to false")
}

// Capacity Pool Filtering with Auto-Tiering Tests

func TestCapacityPoolsForStoragePool_WithAutoTiering(t *testing.T) {
	sdk := getFakeSDK()

	// Add capacity pools with auto-tiering enabled
	cpAutoTier1 := &CapacityPool{
		Name:            "CP_AutoTier1",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP_AutoTier1",
		Location:        Location,
		ServiceLevel:    ServiceLevelPremium,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
		AutoTiering:     true,
	}
	cpAutoTier2 := &CapacityPool{
		Name:            "CP_AutoTier2",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP_AutoTier2",
		Location:        Location,
		ServiceLevel:    ServiceLevelStandard,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
		AutoTiering:     true,
	}

	addCapacityPool(sdk, cpAutoTier1)
	addCapacityPool(sdk, cpAutoTier2)

	// Create storage pool that references auto-tiering capacity pools
	sPool := storage.NewStoragePool(nil, "testPool")
	sPool.InternalAttributes()[capacityPools] = "CP_AutoTier1,CP_AutoTier2"

	// Get capacity pools for storage pool
	cPools := sdk.CapacityPoolsForStoragePool(ctx, sPool, "", "")

	// Verify both pools are returned
	assert.Equal(t, 2, len(cPools), "expected 2 capacity pools")

	// Verify both have auto-tiering enabled
	for _, cPool := range cPools {
		assert.True(t, cPool.AutoTiering, "capacity pool should have auto-tiering enabled")
	}
}

func TestCapacityPoolsForStoragePool_FilterByServiceLevelWithAutoTiering(t *testing.T) {
	sdk := getFakeSDK()

	// Clear existing capacity pools from getFakeSDK to have a clean slate
	clearCapacityPools(sdk)

	// Add capacity pools with different service levels and auto-tiering
	cpPremium := &CapacityPool{
		Name:            "CP_Premium_AutoTier",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP_Premium_AutoTier",
		Location:        Location,
		ServiceLevel:    ServiceLevelPremium,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
		AutoTiering:     true,
	}
	cpStandard := &CapacityPool{
		Name:            "CP_Standard_AutoTier",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP_Standard_AutoTier",
		Location:        Location,
		ServiceLevel:    ServiceLevelStandard,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
		AutoTiering:     true,
	}

	addCapacityPool(sdk, cpPremium)
	addCapacityPool(sdk, cpStandard)

	// Create storage pool without specific capacity pool filter
	sPool := storage.NewStoragePool(nil, "testPool")
	sPool.InternalAttributes()[network] = NetworkName

	// Filter by Premium service level
	cPools := sdk.CapacityPoolsForStoragePool(ctx, sPool, ServiceLevelPremium, "")

	// Should only get Premium pool with auto-tiering
	assert.Equal(t, 1, len(cPools), "expected 1 Premium capacity pool")
	assert.Equal(t, "CP_Premium_AutoTier", cPools[0].Name, "expected Premium pool")
	assert.True(t, cPools[0].AutoTiering, "Premium pool should have auto-tiering enabled")
}

func TestCapacityPools_ReturnsAllWithAutoTiering(t *testing.T) {
	sdk := getFakeSDK()

	// Add a capacity pool with auto-tiering
	cpAutoTier := &CapacityPool{
		Name:            "CP_AutoTier",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP_AutoTier",
		Location:        Location,
		ServiceLevel:    ServiceLevelPremium,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
		AutoTiering:     true,
	}
	addCapacityPool(sdk, cpAutoTier)

	// Get all capacity pools
	allPools := sdk.CapacityPools()

	// Should include the new pool with auto-tiering
	found := false
	for _, pool := range *allPools {
		if pool.Name == "CP_AutoTier" {
			found = true
			assert.True(t, pool.AutoTiering, "pool should have auto-tiering enabled")
			break
		}
	}
	assert.True(t, found, "auto-tiering pool should be in the list")
}
