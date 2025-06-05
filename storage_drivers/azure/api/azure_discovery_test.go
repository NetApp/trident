// Copyright 2023 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/storage"
)

var ctx = context.TODO()

func getFakeSDK() *Client {
	// Enable tests with randomness
	rand.Seed(time.Now().UnixNano())

	sdk := &Client{
		config: &ClientConfig{
			SubscriptionID:  "mySubscription",
			Location:        "myLocation",
			DebugTraceFlags: map[string]bool{"method": true, "api": true, "discovery": true},
		},
		sdkClient: new(AzureClient),
	}

	// RG1:
	//   NA1:
	//     CP1 (ultra)
	//     CP2 (premium)
	//   NA2:
	//     CP1 (ultra)
	//     CP2 (premium)
	//   VN1:
	//     SN1
	//   VN2:
	//     SN2
	//     SN3
	// RG2:
	//   NA1:
	//     CP1 (ultra)
	//     CP2 (premium)
	//   NA2:
	//     CP3 (standard)
	//   VN1:
	//     SN1
	//   VN2:
	//     SN2
	//   VN3:
	//     SN3

	// Resource groups
	sdk.sdkClient.ResourceGroupMap = make(map[string]*ResourceGroup)

	RG1 := &ResourceGroup{
		Name: "RG1",
	}
	sdk.sdkClient.ResourceGroupMap[RG1.Name] = RG1

	RG2 := &ResourceGroup{
		Name: "RG2",
	}
	sdk.sdkClient.ResourceGroupMap[RG2.Name] = RG2

	// NetApp accounts
	sdk.sdkClient.NetAppAccountMap = make(map[string]*NetAppAccount)

	RG1_NA1 := &NetAppAccount{
		ResourceGroup: "RG1",
		Name:          "NA1",
		FullName:      "RG1/NA1",
		Location:      "myLocation",
	}
	sdk.sdkClient.NetAppAccountMap[RG1_NA1.FullName] = RG1_NA1

	RG1_NA2 := &NetAppAccount{
		ResourceGroup: "RG1",
		Name:          "NA2",
		FullName:      "RG1/NA2",
		Location:      "myLocation",
	}
	sdk.sdkClient.NetAppAccountMap[RG1_NA2.FullName] = RG1_NA2

	RG2_NA1 := &NetAppAccount{
		ResourceGroup: "RG2",
		Name:          "NA1",
		FullName:      "RG2/NA1",
		Location:      "myLocation",
	}
	sdk.sdkClient.NetAppAccountMap[RG2_NA1.FullName] = RG2_NA1

	RG2_NA2 := &NetAppAccount{
		ResourceGroup: "RG2",
		Name:          "NA2",
		FullName:      "RG2/NA2",
		Location:      "myLocation",
	}
	sdk.sdkClient.NetAppAccountMap[RG2_NA2.FullName] = RG2_NA2

	// Capacity pools
	sdk.sdkClient.CapacityPoolMap = make(map[string]*CapacityPool)

	RG1_NA1_CP1 := &CapacityPool{
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		Name:              "CP1",
		FullName:          "RG1/NA1/CP1",
		Location:          "myLocation",
		ServiceLevel:      ServiceLevelUltra,
		QosType:           QOSAuto,
		ProvisioningState: "Auto",
	}
	sdk.sdkClient.CapacityPoolMap[RG1_NA1_CP1.FullName] = RG1_NA1_CP1

	RG1_NA1_CP2 := &CapacityPool{
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA1",
		Name:              "CP2",
		FullName:          "RG1/NA1/CP2",
		Location:          "myLocation",
		ServiceLevel:      ServiceLevelPremium,
		QosType:           QOSAuto,
		ProvisioningState: "Auto",
	}
	sdk.sdkClient.CapacityPoolMap[RG1_NA1_CP2.FullName] = RG1_NA1_CP2

	RG1_NA2_CP1 := &CapacityPool{
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA2",
		Name:              "CP1",
		FullName:          "RG1/NA2/CP1",
		Location:          "myLocation",
		ServiceLevel:      ServiceLevelUltra,
		QosType:           QOSAuto,
		ProvisioningState: StateAvailable,
	}
	sdk.sdkClient.CapacityPoolMap[RG1_NA2_CP1.FullName] = RG1_NA2_CP1

	RG1_NA2_CP2 := &CapacityPool{
		ResourceGroup:     "RG1",
		NetAppAccount:     "NA2",
		Name:              "CP2",
		FullName:          "RG1/NA2/CP2",
		Location:          "myLocation",
		ServiceLevel:      ServiceLevelPremium,
		QosType:           QOSAuto,
		ProvisioningState: StateAvailable,
	}
	sdk.sdkClient.CapacityPoolMap[RG1_NA2_CP2.FullName] = RG1_NA2_CP2

	RG2_NA1_CP1 := &CapacityPool{
		ResourceGroup:     "RG2",
		NetAppAccount:     "NA1",
		Name:              "CP1",
		FullName:          "RG2/NA1/CP1",
		Location:          "myLocation",
		ServiceLevel:      ServiceLevelUltra,
		QosType:           QOSManual,
		ProvisioningState: StateAvailable,
	}
	sdk.sdkClient.CapacityPoolMap[RG2_NA1_CP1.FullName] = RG2_NA1_CP1

	RG2_NA1_CP2 := &CapacityPool{
		ResourceGroup:     "RG2",
		NetAppAccount:     "NA1",
		Name:              "CP2",
		FullName:          "RG2/NA1/CP2",
		Location:          "myLocation",
		ServiceLevel:      ServiceLevelPremium,
		QosType:           QOSManual,
		ProvisioningState: StateAvailable,
	}
	sdk.sdkClient.CapacityPoolMap[RG2_NA1_CP2.FullName] = RG2_NA1_CP2

	RG2_NA2_CP3 := &CapacityPool{
		ResourceGroup:     "RG2",
		NetAppAccount:     "NA2",
		Name:              "CP3",
		FullName:          "RG2/NA2/CP3",
		Location:          "myLocation",
		ServiceLevel:      ServiceLevelStandard,
		QosType:           QOSManual,
		ProvisioningState: StateAvailable,
	}
	sdk.sdkClient.CapacityPoolMap[RG2_NA2_CP3.FullName] = RG2_NA2_CP3

	// Virtual networks
	sdk.sdkClient.VirtualNetworkMap = make(map[string]*VirtualNetwork)

	RG1_VN1 := &VirtualNetwork{
		ResourceGroup: "RG1",
		Name:          "VN1",
		FullName:      "RG1/VN1",
		Location:      "myLocation",
	}
	sdk.sdkClient.VirtualNetworkMap[RG1_VN1.FullName] = RG1_VN1

	RG1_VN2 := &VirtualNetwork{
		ResourceGroup: "RG1",
		Name:          "VN2",
		FullName:      "RG1/VN2",
		Location:      "myLocation",
	}
	sdk.sdkClient.VirtualNetworkMap[RG1_VN2.FullName] = RG1_VN2

	RG2_VN1 := &VirtualNetwork{
		ResourceGroup: "RG2",
		Name:          "VN1",
		FullName:      "RG2/VN1",
		Location:      "myLocation",
	}
	sdk.sdkClient.VirtualNetworkMap[RG2_VN1.FullName] = RG2_VN1

	RG2_VN2 := &VirtualNetwork{
		ResourceGroup: "RG2",
		Name:          "VN2",
		FullName:      "RG2/VN2",
		Location:      "myLocation",
	}
	sdk.sdkClient.VirtualNetworkMap[RG2_VN2.FullName] = RG2_VN2

	RG2_VN3 := &VirtualNetwork{
		ResourceGroup: "RG2",
		Name:          "VN3",
		FullName:      "RG2/VN3",
		Location:      "myLocation",
	}
	sdk.sdkClient.VirtualNetworkMap[RG2_VN3.FullName] = RG2_VN3

	// Subnets
	sdk.sdkClient.SubnetMap = make(map[string]*Subnet)

	RG1_VN1_SN1 := &Subnet{
		ResourceGroup:  "RG1",
		VirtualNetwork: "VN1",
		Name:           "SN1",
		FullName:       "RG1/VN1/SN1",
		Location:       "myLocation",
	}
	sdk.sdkClient.SubnetMap[RG1_VN1_SN1.FullName] = RG1_VN1_SN1

	RG1_VN2_SN2 := &Subnet{
		ResourceGroup:  "RG1",
		VirtualNetwork: "VN2",
		Name:           "SN2",
		FullName:       "RG1/VN2/SN2",
		Location:       "myLocation",
	}
	sdk.sdkClient.SubnetMap[RG1_VN2_SN2.FullName] = RG1_VN2_SN2

	RG1_VN2_SN3 := &Subnet{
		ResourceGroup:  "RG1",
		VirtualNetwork: "VN2",
		Name:           "SN3",
		FullName:       "RG1/VN2/SN3",
		Location:       "myLocation",
	}
	sdk.sdkClient.SubnetMap[RG1_VN2_SN3.FullName] = RG1_VN2_SN3

	RG2_VN1_SN1 := &Subnet{
		ResourceGroup:  "RG2",
		VirtualNetwork: "VN1",
		Name:           "SN1",
		FullName:       "RG2/VN1/SN1",
		Location:       "myLocation",
	}
	sdk.sdkClient.SubnetMap[RG2_VN1_SN1.FullName] = RG2_VN1_SN1

	RG2_VN2_SN2 := &Subnet{
		ResourceGroup:  "RG2",
		VirtualNetwork: "VN2",
		Name:           "SN2",
		FullName:       "RG2/VN2/SN2",
		Location:       "myLocation",
	}
	sdk.sdkClient.SubnetMap[RG2_VN2_SN2.FullName] = RG2_VN2_SN2

	RG2_VN3_SN3 := &Subnet{
		ResourceGroup:  "RG2",
		VirtualNetwork: "VN3",
		Name:           "SN3",
		FullName:       "RG2/VN3/SN3",
		Location:       "myLocation",
	}
	sdk.sdkClient.SubnetMap[RG2_VN3_SN3.FullName] = RG2_VN3_SN3

	return sdk
}

func TestCheckForUnsatisfiedPools_NoPools(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool2 := storage.NewStoragePool(nil, "pool2")

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2}

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Zero(t, len(result), "expected no errors")
}

func TestCheckForUnsatisfiedPools_EmptyPools(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool1.InternalAttributes()[capacityPools] = ""
	sPool2 := storage.NewStoragePool(nil, "pool2")
	sPool2.InternalAttributes()[capacityPools] = ""

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2}

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Zero(t, len(result), "expected no errors")
}

func TestCheckForUnsatisfiedPools_ValidPools(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool1.InternalAttributes()[capacityPools] = "RG1/NA1/CP1,RG1/NA1/CP2,CP3"
	sPool2 := storage.NewStoragePool(nil, "pool2")
	sPool2.InternalAttributes()[capacityPools] = "CP1,CP2"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2}

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Zero(t, len(result), "expected no errors")
}

func TestCheckForUnsatisfiedPools_OneInvalidPool(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool1.InternalAttributes()[capacityPools] = "RG1/NA1/CP1,RG1/NA1/CP2,CP4"
	sPool2 := storage.NewStoragePool(nil, "pool2")
	sPool2.InternalAttributes()[capacityPools] = "CP4"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2}

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Equal(t, 1, len(result), "expected no errors")
}

func TestCheckForUnsatisfiedPools_TwoInvalidPools(t *testing.T) {
	sPool1 := storage.NewStoragePool(nil, "pool1")
	sPool1.InternalAttributes()[serviceLevel] = ServiceLevelUltra
	sPool1.InternalAttributes()[capacityPools] = "CP2"
	sPool2 := storage.NewStoragePool(nil, "pool2")
	sPool2.InternalAttributes()[resourceGroups] = "RG1"
	sPool2.InternalAttributes()[netappAccounts] = "NA1"
	sPool2.InternalAttributes()[capacityPools] = "CP4"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool1": sPool1, "pool2": sPool2}

	result := sdk.checkForUnsatisfiedPools(ctx)

	assert.Equal(t, 2, len(result), "expected no errors")
}

func TestCheckForNonexistentResourceGroups_NoPools(t *testing.T) {
	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = make(map[string]storage.Pool)

	result := sdk.checkForNonexistentResourceGroups(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentResourceGroups_Empty(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[resourceGroups] = ""

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentResourceGroups(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentResourceGroups_OK(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[resourceGroups] = "RG1,RG2"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentResourceGroups(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentResourceGroups_Missing(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[resourceGroups] = "RG1,RG2,RG3"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentResourceGroups(ctx)

	assert.True(t, result, "expected error")
}

func TestCheckForNonexistentNetAppAccounts_NoPools(t *testing.T) {
	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = make(map[string]storage.Pool)

	result := sdk.checkForNonexistentNetAppAccounts(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentNetAppAccounts_Empty(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[netappAccounts] = ""

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentNetAppAccounts(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentNetAppAccounts_OK(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[netappAccounts] = "RG1/NA1,RG2/NA1,NA2"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentNetAppAccounts(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentNetAppAccounts_Missing(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[netappAccounts] = "RG1/NA1,RG2/NA1,NA3"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentNetAppAccounts(ctx)

	assert.True(t, result, "expected error")
}

func TestCheckForNonexistentCapacityPools_NoPools(t *testing.T) {
	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = make(map[string]storage.Pool)

	result := sdk.checkForNonexistentCapacityPools(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentCapacityPools_Empty(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[capacityPools] = ""

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentCapacityPools(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentCapacityPools_OK(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[capacityPools] = "RG1/NA1/CP1,RG1/NA1/CP2,CP3"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentCapacityPools(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentCapacityPools_Missing(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[capacityPools] = "RG1/NA1/CP1,RG1/NA1/CP2,CP4"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentCapacityPools(ctx)

	assert.True(t, result, "expected error")
}

func TestCheckForNonexistentVirtualNetworks_NoPools(t *testing.T) {
	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = make(map[string]storage.Pool)

	result := sdk.checkForNonexistentVirtualNetworks(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentVirtualNetworks_Empty(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[virtualNetwork] = ""

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentVirtualNetworks(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentVirtualNetworks_OK(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[virtualNetwork] = "RG1/VN1"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentVirtualNetworks(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentVirtualNetworks_Missing(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[virtualNetwork] = "VN4"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentVirtualNetworks(ctx)

	assert.True(t, result, "expected error")
}

func TestCheckForNonexistentSubnets_NoPools(t *testing.T) {
	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = make(map[string]storage.Pool)

	result := sdk.checkForNonexistentSubnets(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentSubnets_Empty(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[subnet] = ""

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentSubnets(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentSubnets_OK(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[subnet] = "RG1/VN2/SN3"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentSubnets(ctx)

	assert.False(t, result, "expected no error")
}

func TestCheckForNonexistentSubnets_Missing(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "pool")
	sPool.InternalAttributes()[subnet] = "RG1/VN2/SN4"

	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = map[string]storage.Pool{"pool": sPool}

	result := sdk.checkForNonexistentSubnets(ctx)

	assert.True(t, result, "expected error")
}

func TestFeatures(t *testing.T) {
	sdk := getFakeSDK()

	tests := []struct {
		features map[string]bool
		feature  string
		expected bool
	}{
		{
			features: map[string]bool{"feature1": true, "feature2": false},
			feature:  "feature1",
			expected: true,
		},
		{
			features: map[string]bool{"feature1": true, "feature2": false},
			feature:  "feature2",
			expected: false,
		},
		{
			features: map[string]bool{"feature1": true, "feature2": false},
			feature:  "feature3",
			expected: false,
		},
		{
			features: map[string]bool{},
			feature:  "feature1",
			expected: false,
		},
	}

	for _, test := range tests {

		sdk.sdkClient.Features = test.features

		featuresResult := sdk.Features()
		hasFeatureResult := sdk.HasFeature(test.feature)

		// Change original to ensure we made a copy
		sdk.sdkClient.Features = make(map[string]bool)

		assert.Equal(t, test.features, featuresResult, "features mismatch")
		assert.Equal(t, test.expected, hasFeatureResult, "feature mismatch")
	}
}

func TestCapacityPools(t *testing.T) {
	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = make(map[string]storage.Pool)

	expected := &[]*CapacityPool{
		sdk.capacityPool("RG1/NA1/CP1"),
		sdk.capacityPool("RG1/NA1/CP2"),
		sdk.capacityPool("RG1/NA2/CP1"),
		sdk.capacityPool("RG1/NA2/CP2"),
		sdk.capacityPool("RG2/NA1/CP1"),
		sdk.capacityPool("RG2/NA1/CP2"),
		sdk.capacityPool("RG2/NA2/CP3"),
	}

	actual := sdk.CapacityPools()

	assert.ElementsMatch(t, *expected, *actual)
}

func TestCapacityPoolsForStoragePools(t *testing.T) {
	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = make(map[string]storage.Pool)

	RG1_NA1_CP1 := sdk.capacityPool("RG1/NA1/CP1")
	RG1_NA1_CP2 := sdk.capacityPool("RG1/NA1/CP2")
	RG2_NA2_CP3 := sdk.capacityPool("RG2/NA2/CP3")

	RG1_NA1_CP1.ServiceLevel = ServiceLevelUltra
	RG1_NA1_CP1.QosType = QOSAuto

	RG1_NA1_CP2.ServiceLevel = ServiceLevelPremium
	RG1_NA1_CP2.QosType = QOSAuto

	RG2_NA2_CP3.ServiceLevel = ServiceLevelStandard
	RG2_NA2_CP3.QosType = QOSManual

	sPool1 := storage.NewStoragePool(nil, "testPool1")
	sPool1.InternalAttributes()[capacityPools] = "CP3"
	sdk.sdkClient.StoragePoolMap[sPool1.Name()] = sPool1

	sPool2 := storage.NewStoragePool(nil, "testPool2")
	sPool2.InternalAttributes()[capacityPools] = "RG1/NA1/CP1,RG1/NA1/CP2"
	sdk.sdkClient.StoragePoolMap[sPool2.Name()] = sPool2

	expected := []*CapacityPool{RG1_NA1_CP1, RG1_NA1_CP2, RG2_NA2_CP3}

	actual := sdk.CapacityPoolsForStoragePools(context.TODO())

	assert.ElementsMatch(t, expected, actual)
}

func TestCapacityPoolsForStoragePool(t *testing.T) {
	sdk := getFakeSDK()
	RG1_NA1_CP1 := sdk.capacityPool("RG1/NA1/CP1")
	RG1_NA1_CP2 := sdk.capacityPool("RG1/NA1/CP2")
	RG1_NA2_CP1 := sdk.capacityPool("RG1/NA2/CP1")
	RG1_NA2_CP2 := sdk.capacityPool("RG1/NA2/CP2")
	RG2_NA1_CP1 := sdk.capacityPool("RG2/NA1/CP1")
	RG2_NA1_CP2 := sdk.capacityPool("RG2/NA1/CP2")
	RG2_NA2_CP3 := sdk.capacityPool("RG2/NA2/CP3")

	sPool := storage.NewStoragePool(nil, "testPool")

	tests := []struct {
		resourceGroups string
		netappAccounts string
		capacityPools  string
		serviceLevel   string
		qosType        string
		expected       []*CapacityPool
	}{
		{
			resourceGroups: "",
			netappAccounts: "",
			capacityPools:  "",
			serviceLevel:   "Ultra",
			qosType:        "",
			expected:       []*CapacityPool{RG1_NA1_CP1, RG1_NA2_CP1, RG2_NA1_CP1},
		},
		{
			resourceGroups: "",
			netappAccounts: "",
			capacityPools:  "",
			serviceLevel:   "Ultra",
			qosType:        QOSAuto,
			expected:       []*CapacityPool{RG1_NA1_CP1, RG1_NA2_CP1},
		},
		{
			resourceGroups: "",
			netappAccounts: "",
			capacityPools:  "",
			serviceLevel:   "Ultra",
			qosType:        QOSManual,
			expected:       []*CapacityPool{RG2_NA1_CP1},
		},
		{
			resourceGroups: "",
			netappAccounts: "NA2",
			capacityPools:  "",
			serviceLevel:   "Premium",
			qosType:        "",
			expected:       []*CapacityPool{RG1_NA2_CP2},
		},
		{
			resourceGroups: "",
			netappAccounts: "",
			capacityPools:  "",
			serviceLevel:   "Standard",
			qosType:        "",
			expected:       []*CapacityPool{RG2_NA2_CP3},
		},
		{
			resourceGroups: "RG1",
			netappAccounts: "",
			capacityPools:  "",
			serviceLevel:   "Ultra",
			qosType:        "",
			expected:       []*CapacityPool{RG1_NA1_CP1, RG1_NA2_CP1},
		},
		{
			resourceGroups: "RG3",
			netappAccounts: "",
			capacityPools:  "",
			serviceLevel:   "Ultra",
			qosType:        "",
			expected:       []*CapacityPool{},
		},
		{
			resourceGroups: "",
			netappAccounts: "NA1",
			capacityPools:  "",
			serviceLevel:   "Ultra",
			qosType:        "",
			expected:       []*CapacityPool{RG1_NA1_CP1, RG2_NA1_CP1},
		},
		{
			resourceGroups: "",
			netappAccounts: "RG1/NA1",
			capacityPools:  "",
			serviceLevel:   "Premium",
			qosType:        "",
			expected:       []*CapacityPool{RG1_NA1_CP2},
		},
		{
			resourceGroups: "",
			netappAccounts: "RG1/NA1,RG2/NA1",
			capacityPools:  "",
			serviceLevel:   "Premium",
			qosType:        "",
			expected:       []*CapacityPool{RG1_NA1_CP2, RG2_NA1_CP2},
		},
		{
			resourceGroups: "RG1,RG2",
			netappAccounts: "RG1/NA1,RG2/NA1",
			capacityPools:  "",
			serviceLevel:   "Premium",
			qosType:        "",
			expected:       []*CapacityPool{RG1_NA1_CP2, RG2_NA1_CP2},
		},
		{
			resourceGroups: "",
			netappAccounts: "NA3",
			capacityPools:  "",
			serviceLevel:   "Premium",
			qosType:        "",
			expected:       []*CapacityPool{},
		},
		{
			resourceGroups: "",
			netappAccounts: "",
			capacityPools:  "CP1",
			serviceLevel:   "Ultra",
			qosType:        "",
			expected:       []*CapacityPool{RG1_NA1_CP1, RG1_NA2_CP1, RG2_NA1_CP1},
		},
		{
			resourceGroups: "",
			netappAccounts: "",
			capacityPools:  "CP1,RG1/NA1/CP2",
			serviceLevel:   "",
			qosType:        "",
			expected:       []*CapacityPool{RG1_NA1_CP1, RG1_NA2_CP1, RG2_NA1_CP1, RG1_NA1_CP2},
		},
		{
			resourceGroups: "",
			netappAccounts: "",
			capacityPools:  "CP4",
			serviceLevel:   "",
			qosType:        "",
			expected:       []*CapacityPool{},
		},
		{
			resourceGroups: "RG1",
			netappAccounts: "NA1",
			capacityPools:  "CP2",
			serviceLevel:   "Premium",
			qosType:        QOSAuto,
			expected:       []*CapacityPool{RG1_NA1_CP2},
		},
		{
			resourceGroups: "RG1",
			netappAccounts: "NA1",
			capacityPools:  "CP2",
			serviceLevel:   "Premium",
			qosType:        QOSManual,
			expected:       []*CapacityPool{},
		},
		{
			resourceGroups: "RG1",
			netappAccounts: "NA1",
			capacityPools:  "CP2",
			serviceLevel:   "Standard",
			qosType:        "",
			expected:       []*CapacityPool{},
		},
		{
			resourceGroups: "RG1,RG2,RG3",
			netappAccounts: "NA1,NA2,NA3",
			capacityPools:  "RG1/NA1/CP1",
			serviceLevel:   "Ultra",
			qosType:        "",
			expected:       []*CapacityPool{RG1_NA1_CP1},
		},
		{
			resourceGroups: "RG2",
			netappAccounts: "",
			capacityPools:  "RG1/NA1/CP1",
			serviceLevel:   "",
			qosType:        "",
			expected:       []*CapacityPool{},
		},
	}

	for _, test := range tests {

		sPool.InternalAttributes()[resourceGroups] = test.resourceGroups
		sPool.InternalAttributes()[netappAccounts] = test.netappAccounts
		sPool.InternalAttributes()[capacityPools] = test.capacityPools

		cPools := sdk.CapacityPoolsForStoragePool(context.TODO(), sPool, test.serviceLevel, test.qosType)

		assert.ElementsMatch(t, test.expected, cPools)
	}
}

func TestEnsureVolumeInValidCapacityPool(t *testing.T) {
	sdk := getFakeSDK()
	sdk.sdkClient.StoragePoolMap = make(map[string]storage.Pool)

	volume := &FileSystem{
		ResourceGroup: "RG1",
		NetAppAccount: "NA1",
		CapacityPool:  "CP1",
		CreationToken: "V1",
	}

	assert.Nil(t, sdk.EnsureVolumeInValidCapacityPool(context.TODO(), volume), "result not nil")

	sPool1 := storage.NewStoragePool(nil, "testPool1")
	sPool1.InternalAttributes()[capacityPools] = "CP3"
	sdk.sdkClient.StoragePoolMap[sPool1.Name()] = sPool1

	assert.NotNil(t, sdk.EnsureVolumeInValidCapacityPool(context.TODO(), volume), "result nil")

	sPool2 := storage.NewStoragePool(nil, "testPool2")
	sPool2.InternalAttributes()[capacityPools] = "RG1/NA1/CP1,RG1/NA1/CP2"
	sdk.sdkClient.StoragePoolMap[sPool2.Name()] = sPool2

	assert.Nil(t, sdk.EnsureVolumeInValidCapacityPool(context.TODO(), volume), "result not nil")
}

func TestSubnetsForStoragePoolAndFilteredSubnetMap(t *testing.T) {
	sdk := getFakeSDK()
	RG1_VN1_SN1 := sdk.subnet("RG1/VN1/SN1")
	RG1_VN2_SN2 := sdk.subnet("RG1/VN2/SN2")
	RG1_VN2_SN3 := sdk.subnet("RG1/VN2/SN3")
	RG2_VN1_SN1 := sdk.subnet("RG2/VN1/SN1")
	RG2_VN2_SN2 := sdk.subnet("RG2/VN2/SN2")
	RG2_VN3_SN3 := sdk.subnet("RG2/VN3/SN3")

	sPool := storage.NewStoragePool(nil, "testPool")

	tests := []struct {
		resourceGroups string
		virtualNetwork string
		subnet         string
		expected       []*Subnet
	}{
		{
			resourceGroups: "",
			virtualNetwork: "",
			subnet:         "",
			expected:       []*Subnet{RG1_VN1_SN1, RG1_VN2_SN2, RG1_VN2_SN3, RG2_VN1_SN1, RG2_VN2_SN2, RG2_VN3_SN3},
		},
		{
			resourceGroups: "RG1",
			virtualNetwork: "",
			subnet:         "",
			expected:       []*Subnet{RG1_VN1_SN1, RG1_VN2_SN2, RG1_VN2_SN3},
		},
		{
			resourceGroups: "RG1,RG2",
			virtualNetwork: "",
			subnet:         "",
			expected:       []*Subnet{RG1_VN1_SN1, RG1_VN2_SN2, RG1_VN2_SN3, RG2_VN1_SN1, RG2_VN2_SN2, RG2_VN3_SN3},
		},
		{
			resourceGroups: "",
			virtualNetwork: "VN1",
			subnet:         "",
			expected:       []*Subnet{RG1_VN1_SN1, RG2_VN1_SN1},
		},
		{
			resourceGroups: "",
			virtualNetwork: "RG1/VN2",
			subnet:         "",
			expected:       []*Subnet{RG1_VN2_SN2, RG1_VN2_SN3},
		},
		{
			resourceGroups: "",
			virtualNetwork: "",
			subnet:         "SN1",
			expected:       []*Subnet{RG1_VN1_SN1, RG2_VN1_SN1},
		},
		{
			resourceGroups: "",
			virtualNetwork: "",
			subnet:         "RG1/VN1/SN1",
			expected:       []*Subnet{RG1_VN1_SN1},
		},
		{
			resourceGroups: "RG1,RG2",
			virtualNetwork: "RG1/VN2",
			subnet:         "SN3",
			expected:       []*Subnet{RG1_VN2_SN3},
		},
		{
			resourceGroups: "RG3",
			virtualNetwork: "",
			subnet:         "",
			expected:       []*Subnet{},
		},
		{
			resourceGroups: "",
			virtualNetwork: "VN4",
			subnet:         "",
			expected:       []*Subnet{},
		},
		{
			resourceGroups: "",
			virtualNetwork: "",
			subnet:         "SN4",
			expected:       []*Subnet{},
		},
		{
			resourceGroups: "RG1",
			virtualNetwork: "VN2",
			subnet:         "SN3",
			expected:       []*Subnet{RG1_VN2_SN3},
		},
		{
			resourceGroups: "RG1",
			virtualNetwork: "VN1",
			subnet:         "SN3",
			expected:       []*Subnet{},
		},
		{
			resourceGroups: "RG1,RG2,RG3",
			virtualNetwork: "RG1/VN1",
			subnet:         "",
			expected:       []*Subnet{RG1_VN1_SN1},
		},
		{
			resourceGroups: "RG1",
			virtualNetwork: "",
			subnet:         "RG2/VN2/SN2",
			expected:       []*Subnet{},
		},
	}

	for _, test := range tests {

		sPool.InternalAttributes()[resourceGroups] = test.resourceGroups
		sPool.InternalAttributes()[virtualNetwork] = test.virtualNetwork
		sPool.InternalAttributes()[subnet] = test.subnet

		subnets := sdk.SubnetsForStoragePool(context.TODO(), sPool)
		subnet := sdk.RandomSubnetForStoragePool(context.TODO(), sPool)

		assert.ElementsMatch(t, test.expected, subnets)

		if len(test.expected) > 0 {
			assert.Contains(t, test.expected, subnet)
		} else {
			assert.Nil(t, subnet)
		}
	}

	for _, test := range tests {
		var rgFilter []string
		if test.resourceGroups != "" {
			rgFilter = strings.Split(test.resourceGroups, ",")
		}

		subnetMap := sdk.FilteredSubnetMap(context.TODO(), rgFilter, test.virtualNetwork, test.subnet)
		var subnetList []*Subnet
		for _, subnet := range subnetMap {
			subnetList = append(subnetList, subnet)
		}

		if len(test.expected) > 0 {
			assert.ElementsMatch(t, test.expected, subnetList)
		} else {
			assert.Nil(t, subnetList)
		}
	}
}

func TestFilteredCapacityPoolMap(t *testing.T) {
	sdk := getFakeSDK()
	RG1_NA1_CP1 := sdk.capacityPool("RG1/NA1/CP1")
	RG1_NA1_CP2 := sdk.capacityPool("RG1/NA1/CP2")
	RG1_NA2_CP1 := sdk.capacityPool("RG1/NA2/CP1")
	RG1_NA2_CP2 := sdk.capacityPool("RG1/NA2/CP2")
	RG2_NA1_CP1 := sdk.capacityPool("RG2/NA1/CP1")
	RG2_NA1_CP2 := sdk.capacityPool("RG2/NA1/CP2")
	RG2_NA2_CP3 := sdk.capacityPool("RG2/NA2/CP3")

	tests := []struct {
		resourceGroups []string
		netappAccounts []string
		capacityPools  []string
		expected       []*CapacityPool
	}{
		{
			resourceGroups: []string{"RG1"},
			netappAccounts: []string{},
			capacityPools:  []string{},
			expected:       []*CapacityPool{RG1_NA1_CP1, RG1_NA1_CP2, RG1_NA2_CP1, RG1_NA2_CP2},
		},
		{
			resourceGroups: []string{"RG2"},
			netappAccounts: []string{},
			capacityPools:  []string{},
			expected:       []*CapacityPool{RG2_NA1_CP1, RG2_NA1_CP2, RG2_NA2_CP3},
		},
		{
			resourceGroups: []string{},
			netappAccounts: []string{},
			capacityPools:  []string{},
			expected:       []*CapacityPool{RG1_NA1_CP1, RG1_NA1_CP2, RG1_NA2_CP1, RG1_NA2_CP2, RG2_NA1_CP1, RG2_NA1_CP2, RG2_NA2_CP3},
		},
		{
			resourceGroups: []string{},
			netappAccounts: []string{"NA1"},
			capacityPools:  []string{},
			expected:       []*CapacityPool{RG1_NA1_CP1, RG1_NA1_CP2, RG2_NA1_CP1, RG2_NA1_CP2},
		},
		{
			resourceGroups: []string{},
			netappAccounts: []string{"NA2"},
			capacityPools:  []string{},
			expected:       []*CapacityPool{RG1_NA2_CP1, RG1_NA2_CP2, RG2_NA2_CP3},
		},
		{
			resourceGroups: []string{"RG2"},
			netappAccounts: []string{"NA2"},
			capacityPools:  []string{"CP3"},
			expected:       []*CapacityPool{RG2_NA2_CP3},
		},
		{
			resourceGroups: []string{},
			netappAccounts: []string{},
			capacityPools:  []string{"RG1/NA1/CP2"},
			expected:       []*CapacityPool{RG1_NA1_CP2},
		},
		{
			resourceGroups: []string{},
			netappAccounts: []string{},
			capacityPools:  []string{"CP2"},
			expected:       []*CapacityPool{RG1_NA1_CP2, RG1_NA2_CP2, RG2_NA1_CP2},
		},
		{
			resourceGroups: []string{"RG3"},
			netappAccounts: []string{},
			capacityPools:  []string{},
			expected:       []*CapacityPool{},
		},
		{
			resourceGroups: []string{},
			netappAccounts: []string{"NA3"},
			capacityPools:  []string{},
			expected:       []*CapacityPool{},
		},
		{
			resourceGroups: []string{},
			netappAccounts: []string{},
			capacityPools:  []string{"CP4"},
			expected:       []*CapacityPool{},
		},
		{
			resourceGroups: []string{},
			netappAccounts: []string{},
			capacityPools:  []string{"RG3/NA3/CP4"},
			expected:       []*CapacityPool{},
		},
	}

	for _, test := range tests {
		cpMap := sdk.FilteredCapacityPoolMap(context.TODO(), test.resourceGroups, test.netappAccounts, test.capacityPools)

		var cpList []*CapacityPool
		for _, cp := range cpMap {
			cpList = append(cpList, cp)
		}

		if len(test.expected) > 0 {
			assert.ElementsMatch(t, test.expected, cpList)
		} else {
			assert.Nil(t, cpList)
		}
	}
}
