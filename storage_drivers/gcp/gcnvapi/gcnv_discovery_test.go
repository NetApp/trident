// Copyright 2024 NetApp, Inc. All Rights Reserved.

package gcnvapi

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/netapp/trident/storage"
	storagedrivers "github.com/netapp/trident/storage_drivers"

	"github.com/stretchr/testify/assert"
)

func getFakeSDK() *Client {
	// Enable tests with randomness
	rand.Seed(time.Now().UnixNano())

	APIKey := storagedrivers.GCPPrivateKey{
		Type:                    "random_account",
		ProjectID:               ProjectID,
		PrivateKeyID:            PrivateKeyID,
		PrivateKey:              PrivateKey,
		ClientEmail:             ClientEmail,
		ClientID:                ClientID,
		AuthURI:                 AuthURI,
		TokenURI:                TokenURI,
		AuthProviderX509CertURL: AuthProviderX509CertURL,
		ClientX509CertURL:       ClientX509CertURL,
	}

	sdk := &Client{
		config: &ClientConfig{
			ProjectNumber:   ProjectNumber,
			APIKey:          &APIKey,
			Location:        Location,
			DebugTraceFlags: debugTraceFlags,
		},
		sdkClient: new(GCNVClient),
	}

	// Capacity pools
	sdk.sdkClient.CapacityPoolMap = make(map[string]*CapacityPool)

	GCNV_CP1 := &CapacityPool{
		Name:            "CP1",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP1",
		Location:        Location,
		ServiceLevel:    ServiceLevelPremium,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
	}
	sdk.sdkClient.CapacityPoolMap[GCNV_CP1.FullName] = GCNV_CP1

	GCNV_CP2 := &CapacityPool{
		Name:            "CP2",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP2",
		Location:        Location,
		ServiceLevel:    ServiceLevelStandard,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
	}
	sdk.sdkClient.CapacityPoolMap[GCNV_CP2.FullName] = GCNV_CP2

	GCNV_CP3 := &CapacityPool{
		Name:            "CP3",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP3",
		Location:        Location,
		ServiceLevel:    ServiceLevelExtreme,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
	}
	sdk.sdkClient.CapacityPoolMap[GCNV_CP3.FullName] = GCNV_CP3

	GCNV_CP4 := &CapacityPool{
		Name:            "CP4",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP4",
		Location:        Location,
		ServiceLevel:    ServiceLevelFlex,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
	}
	sdk.sdkClient.CapacityPoolMap[GCNV_CP4.FullName] = GCNV_CP4

	GCNV_CP6 := &CapacityPool{
		Name:            "CP6",
		FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP6",
		Location:        Location,
		ServiceLevel:    ServiceLevelStandard,
		State:           StateReady,
		NetworkName:     NetworkName,
		NetworkFullName: NetworkFullName,
	}
	sdk.sdkClient.CapacityPoolMap[GCNV_CP6.FullName] = GCNV_CP6

	return sdk
}

func TestCheckForUnsatisfiedPools_NoPools(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool2 := storage.NewStoragePool(nil, "pool2")

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2}

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Zero(t, len(result), "expected zero unsatisfied pools")
}

func TestCheckForUnsatisfiedPools_EmptyPools(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool1.InternalAttributes()[capacityPools] = ""
	sPool2 := storage.NewStoragePool(nil, "pool2")
	sPool2.InternalAttributes()[capacityPools] = ""

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2}

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Zero(t, len(result), "expected zero unsatisfied pools for empty pools")
}

func TestCheckForUnsatisfiedPools_ValidPools(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool1.InternalAttributes()[capacityPools] = "CP1,CP3"
	sPool2 := storage.NewStoragePool(nil, "pool2")
	sPool2.InternalAttributes()[capacityPools] = "CP2,CP4"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2}

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Zero(t, len(result), "expected zero unsatisfied pools for valid pools")
}

func TestCheckForUnsatisfiedPools_OneInvalidPool(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool1.InternalAttributes()[capacityPools] = "CP3,CP4"
	sPool2 := storage.NewStoragePool(nil, "pool2")
	sPool2.InternalAttributes()[capacityPools] = "CP5"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2}

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Equal(t, 1, len(result), "expected 1 invalid pool error")
}

func TestCheckForUnsatisfiedPools_TwoInvalidPools(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool1.InternalAttributes()[serviceLevel] = ServiceLevelStandard
	sPool1.InternalAttributes()[capacityPools] = "CP3"
	sPool2 := storage.NewStoragePool(nil, "pool2")
	sPool2.InternalAttributes()[capacityPools] = "CP5"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2}

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Equal(t, 2, len(result), "expected 2 invalid pools error")
}

func TestCheckForNonexistentCapacityPools_NoPools(t *testing.T) {
	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = make(map[string]storage.Pool)

	result := sdk.checkForNonexistentCapacityPools(ctx)

	assert.False(t, result, "expected no nonexistent capacity pools error")
}

func TestCheckForNonexistentCapacityPools_Empty(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[capacityPools] = ""

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentCapacityPools(ctx)

	assert.False(t, result, "expected no nonexistent capacity pools for empty pools")
}

func TestCheckForNonexistentCapacityPools_OK(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[capacityPools] = "CP1,CP3"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentCapacityPools(ctx)

	assert.False(t, result, "expected no nonexistent capacity pools")
}

func TestCheckForNonexistentCapacityPools_Missing(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[capacityPools] = "CP1,CP3,CP5"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentCapacityPools(ctx)

	assert.True(t, result, "expected capacity pools missing error")
}

func TestCheckForNonexistentNetworks_Empty(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[network] = ""

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentNetworks(ctx)

	assert.False(t, result, "expected no nonexistent network error")
}

func TestCheckForNonexistentNetworks_OK(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[network] = NetworkFullName

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentNetworks(ctx)

	assert.False(t, result, "expected no non existent network error")
}

func TestCheckForNonexistentNetworks_Missing(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[network] = "projects/123456789/locations/myLocation/networks/missing"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentNetworks(ctx)

	assert.True(t, result, "expected network missing error")
}

func TestCapacityPools(t *testing.T) {
	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = make(map[string]storage.Pool)

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

func TestCapacityPoolsForStoragePools(t *testing.T) {
	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = make(map[string]storage.Pool)

	CP1 := sdk.capacityPool("CP1")
	CP2 := sdk.capacityPool("CP2")
	CP3 := sdk.capacityPool("CP3")

	sPool1 := storage.NewStoragePool(nil, "testPool1")
	sPool1.InternalAttributes()[capacityPools] = "CP3"
	sdk.sdkClient.StoragePoolMap[sPool1.Name()] = sPool1

	sPool2 := storage.NewStoragePool(nil, "testPool2")
	sPool2.InternalAttributes()[capacityPools] = "CP1,CP2"
	sdk.sdkClient.StoragePoolMap[sPool2.Name()] = sPool2

	expected := []*CapacityPool{CP1, CP2, CP3}

	actual := sdk.CapacityPoolsForStoragePools(context.TODO())

	assert.ElementsMatch(t, expected, actual, "Did not get expected capacity pools for storage pools")
}

func TestCapacityPoolsForStoragePool(t *testing.T) {
	sdk := getFakeSDK()
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

		cPools := sdk.CapacityPoolsForStoragePool(context.TODO(), sPool, test.serviceLevel)

		assert.ElementsMatch(t, test.expected, cPools, "Did not get expected capacity pools for storage pools")
	}
}

func TestEnsureVolumeInValidCapacityPool(t *testing.T) {
	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = make(map[string]storage.Pool)

	volume := &Volume{
		Name:         "V1",
		CapacityPool: "CP1",
		Location:     "fake-location",
	}
	assert.Nil(t, sdk.EnsureVolumeInValidCapacityPool(context.TODO(), volume),
		"ensureVolumeInvalidCapacityPool result not nil")

	sPool1 := storage.NewStoragePool(nil, "testPool1")
	sPool1.InternalAttributes()[capacityPools] = "CP3"
	sdk.sdkClient.StoragePoolMap[sPool1.Name()] = sPool1

	assert.NotNil(t, sdk.EnsureVolumeInValidCapacityPool(context.TODO(), volume),
		"ensureVolumeInvalidCapacityPool result nil")

	sPool2 := storage.NewStoragePool(nil, "testPool2")
	sPool2.InternalAttributes()[capacityPools] = "CP1"
	sdk.sdkClient.StoragePoolMap[sPool2.Name()] = sPool2

	assert.Nil(t, sdk.EnsureVolumeInValidCapacityPool(context.TODO(), volume),
		"ensureVolumeInvalidCapacityPool result not nil")
}

func TestDumpGCNVResources(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[network] = NetworkFullName

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	discoveryTraceEnabled := sdk.config.DebugTraceFlags["api"]

	driverName := sdk.config.StorageDriverName
	sdk.dumpGCNVResources(ctx, driverName, discoveryTraceEnabled)
}
