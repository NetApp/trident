// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"

	"github.com/netapp/trident/utils/errors"
)

func TestAzureError_WithoutDetails(t *testing.T) {
	err := &AzureError{
		AzError: struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"details"`
		}{
			Code:    "BadRequest",
			Message: "Failed to create volume as capacity pool is too small",
		},
	}

	result := err.Error()

	assert.Equal(t, "BadRequest: Failed to create volume as capacity pool is too small", result)
}

func TestAzureError_WithDetails(t *testing.T) {
	err := &AzureError{
		AzError: struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"details"`
		}{
			Code:    "BadRequest",
			Message: "Failed to create volume as capacity pool is too small",
			Details: []struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}([]struct {
				Code    string
				Message string
			}{
				{"PoolSizeTooSmall", "Failed to create volume as capacity pool is too small"},
				{"FakeMockError", "Fake mock error"},
			}),
		},
	}

	result := err.Error()

	assert.Equal(t, "PoolSizeTooSmall: Failed to create volume as capacity pool is too small; FakeMockError: Fake mock error", result)
}

func TestVolumePollerCache_Put_NilKey(t *testing.T) {
	pollCache := AzurePollerResponseCache{}

	err := pollCache.Put(nil, &PollerVolumeCreateResponse{})

	assert.NotNil(t, err, "expected error, got nil")
}

func TestVolumePollerCache_Put(t *testing.T) {
	key := PollerKey{
		ID:        "mock-id",
		Operation: Create,
	}

	pollCache := AzurePollerResponseCache{}

	err := pollCache.Put(&key, &PollerVolumeCreateResponse{})

	assert.Nil(t, err, "expected nil, got error")
}

func TestVolumePollerCache_Get_KeyExists(t *testing.T) {
	key := PollerKey{
		ID:        "mock-id",
		Operation: Create,
	}

	pollCache := AzurePollerResponseCache{}
	pollCache.Put(&key, &PollerVolumeCreateResponse{})

	resp, ok := pollCache.Get(key)

	assert.True(t, ok, "expected true, got false")
	assert.NotNil(t, resp, "expected a value, got nil")
}

func TestVolumePollerCache_Get_KeyNotExists(t *testing.T) {
	keyToFind := PollerKey{
		ID:        "mock-id",
		Operation: Create,
	}

	// Case: Empty cache
	pollCache := AzurePollerResponseCache{}

	resp, ok := pollCache.Get(keyToFind)

	assert.False(t, ok, "expected false, got true")
	assert.Nil(t, resp, "expected nil, got a value")

	// Case: Key not present
	pollCache.Put(&PollerKey{
		ID:        "random-key",
		Operation: Create,
	}, &PollerVolumeCreateResponse{})

	resp, ok = pollCache.Get(keyToFind)

	assert.False(t, ok, "expected false, got true")
	assert.Nil(t, resp, "expected nil, got a value")
}

func TestVolumePollerCache_Delete(t *testing.T) {
	key := PollerKey{
		ID:        "mock-id",
		Operation: Create,
	}

	var value PollerResponse
	value = &PollerVolumeCreateResponse{}

	pollCache := AzurePollerResponseCache{}
	pollCache.Put(&key, value)

	pollCache.Delete(key)
}

func TestCreateVirtualNetworkID(t *testing.T) {
	actual := CreateVirtualNetworkID("mySubscription", "myResourceGroup", "myVnet")

	expected := "/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.Network/virtualNetworks/myVnet"

	assert.Equal(t, expected, actual, "virtual network IDs not equal")
}

func TestCreateVirtualNetworkFullName(t *testing.T) {
	actual := CreateVirtualNetworkFullName("myResourceGroup", "myVirtualNetwork")

	expected := "myResourceGroup/myVirtualNetwork"

	assert.Equal(t, expected, actual, "virtual network full names not equal")
}

func TestCreateSubnetID(t *testing.T) {
	actual := CreateSubnetID("mySubscription", "mySubscription", "myVnet", "mySubnet")

	expected := "/subscriptions/mySubscription/resourceGroups/mySubscription/providers/Microsoft.Network/virtualNetworks/myVnet/subnets/mySubnet"

	assert.Equal(t, expected, actual, "subnet IDs not equal")
}

func TestCreateSubnetFullName(t *testing.T) {
	actual := CreateSubnetFullName("myResourceGroup", "myVirtualNetwork", "mySubnet")

	expected := "myResourceGroup/myVirtualNetwork/mySubnet"

	assert.Equal(t, expected, actual, "subnet full names not equal")
}

func TestParseSubnetID(t *testing.T) {
	subscriptionID, resourceGroup, provider, virtualNetwork, subnet, err := ParseSubnetID(
		"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.Network/virtualNetworks/myVirtualNetwork/subnets/mySubnet")

	assert.Equal(t, "mySubscription", subscriptionID, "subscriptionID not correct")
	assert.Equal(t, "myResourceGroup", resourceGroup, "resourceGroup not correct")
	assert.Equal(t, "Microsoft.Network", provider, "provider not correct")
	assert.Equal(t, "myVirtualNetwork", virtualNetwork, "virtualNetwork not correct")
	assert.Equal(t, "mySubnet", subnet, "subnet not correct")
	assert.NoError(t, err, "error is not nil")
}

func TestParseSubnetIDNegative(t *testing.T) {
	tests := []struct {
		description string
		input       string
	}{
		{
			"no subscriptions key",
			"/subscription/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.Network/virtualNetworks/myVirtualNetwork/subnets/mySubnet",
		},
		{
			"no subscriptions value",
			"/subscriptions/resourceGroups/myResourceGroup/providers/Microsoft.Network/virtualNetworks/myVirtualNetwork/subnets/mySubnet",
		},
		{
			"no resource groups key",
			"/subscriptions/mySubscription/resourceGroup/myResourceGroup/providers/Microsoft.Network/virtualNetworks/myVirtualNetwork/subnets/mySubnet",
		},
		{
			"no resource groups value",
			"/subscriptions/mySubscription/resourceGroups/providers/Microsoft.Network/virtualNetworks/myVirtualNetwork/subnets/mySubnet",
		},
		{
			"no providers key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/provider/Microsoft.Network/virtualNetworks/myVirtualNetwork/subnets/mySubnet",
		},
		{
			"no providers value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/virtualNetworks/myVirtualNetwork/subnets/mySubnet",
		},
		{
			"no virtual networks key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.Network/myVirtualNetwork/subnets/mySubnet",
		},
		{
			"no virtual networks value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.Network/virtualNetworks/subnets/mySubnet",
		},
		{
			"no subnets key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.Network/virtualNetworks/myVirtualNetwork/mySubnet",
		},
		{
			"no subnets value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.Network/virtualNetworks/myVirtualNetwork/subnets",
		},
	}

	for _, test := range tests {

		_, _, _, _, _, err := ParseSubnetID(test.input)
		assert.Error(t, err, test.description)
	}
}

func TestCreateNetappAccountID(t *testing.T) {
	actual := CreateNetappAccountID("mySubscription", "myResourceGroup", "myNetappAccount")

	expected := "/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount"

	assert.Equal(t, expected, actual, "netapp account IDs not equal")
}

func TestCreateNetappAccountFullName(t *testing.T) {
	actual := CreateNetappAccountFullName("myResourceGroup", "myNetappAccount")

	expected := "myResourceGroup/myNetappAccount"

	assert.Equal(t, expected, actual, "netapp account full names not equal")
}

func TestParseCapacityPoolID(t *testing.T) {
	subscriptionID, resourceGroup, provider, netappAccount, capacityPool, err := ParseCapacityPoolID(
		"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool")

	assert.Equal(t, "mySubscription", subscriptionID, "subscriptionID not correct")
	assert.Equal(t, "myResourceGroup", resourceGroup, "resourceGroup not correct")
	assert.Equal(t, "Microsoft.NetApp", provider, "provider not correct")
	assert.Equal(t, "myNetappAccount", netappAccount, "netappAccount not correct")
	assert.Equal(t, "myCapacityPool", capacityPool, "capacityPool not correct")
	assert.NoError(t, err, "error is not nil")
}

func TestParseCapacityPoolIDNegative(t *testing.T) {
	tests := []struct {
		description string
		input       string
	}{
		{
			"no subscriptions key",
			"/subscription/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool",
		},
		{
			"no subscriptions value",
			"/subscriptions/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool",
		},
		{
			"no resource groups key",
			"/subscriptions/mySubscription/resourceGroup/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool",
		},
		{
			"no resource groups value",
			"/subscriptions/mySubscription/resourceGroups/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool",
		},
		{
			"no providers key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/provider/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool",
		},
		{
			"no providers value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool",
		},
		{
			"no netapp accounts key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/myNetappAccount/capacityPools/myCapacityPool",
		},
		{
			"no netapp accounts value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/capacityPools/myCapacityPool",
		},
		{
			"no capacity pools key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/myCapacityPool",
		},
		{
			"no capacity pools value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools",
		},
	}

	for _, test := range tests {

		_, _, _, _, _, err := ParseCapacityPoolID(test.input)
		assert.Error(t, err, test.description)
	}
}

func TestCreateCapacityPoolFullName(t *testing.T) {
	actual := CreateCapacityPoolFullName("myResourceGroup", "myNetappAccount", "myCapacityPool")

	expected := "myResourceGroup/myNetappAccount/myCapacityPool"

	assert.Equal(t, expected, actual, "capacity pool full names not equal")
}

func TestCreateVolumeID(t *testing.T) {
	actual := CreateVolumeID("mySubscription", "myResourceGroup", "myNetappAccount", "myCapacityPool", "myVolume")

	expected := "/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume"

	assert.Equal(t, expected, actual, "volume IDs not equal")
}

func TestCreateVolumeFullName(t *testing.T) {
	actual := CreateVolumeFullName("myResourceGroup", "myNetappAccount", "myCapacityPool", "myVolume")

	expected := "myResourceGroup/myNetappAccount/myCapacityPool/myVolume"

	assert.Equal(t, expected, actual, "volume full names not equal")
}

func TestParseVolumeID(t *testing.T) {
	subscriptionID, resourceGroup, provider, netappAccount, capacityPool, volume, err := ParseVolumeID(
		"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume")

	assert.Equal(t, "mySubscription", subscriptionID, "subscriptionID not correct")
	assert.Equal(t, "myResourceGroup", resourceGroup, "resourceGroup not correct")
	assert.Equal(t, "Microsoft.NetApp", provider, "provider not correct")
	assert.Equal(t, "myNetappAccount", netappAccount, "netappAccount not correct")
	assert.Equal(t, "myCapacityPool", capacityPool, "capacityPool not correct")
	assert.Equal(t, "myVolume", volume, "volume not correct")
	assert.NoError(t, err, "error is not nil")
}

func TestParseVolumeIDNegative(t *testing.T) {
	tests := []struct {
		description string
		input       string
	}{
		{
			"no subscriptions key",
			"/subscription/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume",
		},
		{
			"no subscriptions value",
			"/subscriptions/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume",
		},
		{
			"no resource groups key",
			"/subscriptions/mySubscription/resourceGroup/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume",
		},
		{
			"no resource groups value",
			"/subscriptions/mySubscription/resourceGroups/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume",
		},
		{
			"no providers key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/provider/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume",
		},
		{
			"no providers value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume",
		},
		{
			"no accounts key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume",
		},
		{
			"no accounts value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/capacityPools/myCapacityPool/volumes/myVolume",
		},
		{
			"no capacity pools key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/myCapacityPool/volumes/myVolume",
		},
		{
			"no capacity pools value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/volumes/myVolume",
		},
		{
			"no volumes key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volume/myVolume",
		},
		{
			"no volumes value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes",
		},
	}

	for _, test := range tests {

		_, _, _, _, _, _, err := ParseVolumeID(test.input)
		assert.Error(t, err, test.description)
	}
}

func TestParseVolumeName(t *testing.T) {
	resourceGroup, netappAccount, capacityPool, volume, err := ParseVolumeName("myResourceGroup/myNetappAccount/myCapacityPool/myVolume")

	assert.Nil(t, err)
	assert.Equal(t, "myResourceGroup", resourceGroup)
	assert.Equal(t, "myNetappAccount", netappAccount)
	assert.Equal(t, "myCapacityPool", capacityPool)
	assert.Equal(t, "myVolume", volume)
}

func TestParseVolumeNameNegative(t *testing.T) {
	_, _, _, _, err := ParseVolumeName("myVolume")

	assert.NotNil(t, err)
}

func TestCreateSnapshotID(t *testing.T) {
	actual := CreateSnapshotID("mySubscription", "myResourceGroup", "myNetappAccount", "myCapacityPool", "myVolume", "mySnapshot")

	expected := "/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/snapshots/mySnapshot"

	assert.Equal(t, expected, actual, "snapshot IDs not equal")
}

func TestCreateSnapshotFullName(t *testing.T) {
	actual := CreateSnapshotFullName("myResourceGroup", "myNetappAccount", "myCapacityPool", "myVolume", "mySnapshot")

	expected := "myResourceGroup/myNetappAccount/myCapacityPool/myVolume/mySnapshot"

	assert.Equal(t, expected, actual, "snapshot full names not equal")
}

func TestParseSnapshotID(t *testing.T) {
	subscriptionID, resourceGroup, provider, netappAccount, capacityPool, volume, snapshot, err := ParseSnapshotID(
		"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/snapshots/mySnapshot")

	assert.Equal(t, "mySubscription", subscriptionID, "subscriptionID not correct")
	assert.Equal(t, "myResourceGroup", resourceGroup, "resourceGroup not correct")
	assert.Equal(t, "Microsoft.NetApp", provider, "provider not correct")
	assert.Equal(t, "myNetappAccount", netappAccount, "netappAccount not correct")
	assert.Equal(t, "myCapacityPool", capacityPool, "capacityPool not correct")
	assert.Equal(t, "myVolume", volume, "volume not correct")
	assert.Equal(t, "mySnapshot", snapshot, "snapshot not correct")
	assert.NoError(t, err, "error is not nil")
}

func TestParseSnapshotIDNegative(t *testing.T) {
	tests := []struct {
		description string
		input       string
	}{
		{
			"no subscriptions key",
			"/subscription/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no subscriptions value",
			"/subscriptions/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no resource groups key",
			"/subscriptions/mySubscription/resourceGroup/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no resource groups value",
			"/subscriptions/mySubscription/resourceGroups/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no providers key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/provider/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no providers value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no accounts key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no accounts value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/capacityPools/myCapacityPool/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no capacity pools key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/myCapacityPool/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no capacity pools value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/volumes/myVolume/snapshots/mySnapshot",
		},
		{
			"no volumes key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/myVolume/snapshot/mySnapshot",
		},
		{
			"no volumes value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/snapshots/mySnapshot",
		},
		{
			"no snapshots key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/myVolume/snapshot/mySnapshot",
		},
		{
			"no snapshots value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/snapshots",
		},
	}

	for _, test := range tests {

		_, _, _, _, _, _, _, err := ParseSnapshotID(test.input)
		assert.Error(t, err, test.description)
	}
}

func TestExportPolicyExportImport(t *testing.T) {
	rules := []ExportRule{
		{
			AllowedClients: "10.10.10.0/24",
			Cifs:           false,
			Nfsv3:          true,
			Nfsv41:         false,
			RuleIndex:      1,
			UnixReadOnly:   false,
			UnixReadWrite:  true,
		},
		{
			AllowedClients: "10.10.20.0/24",
			Cifs:           true,
			Nfsv3:          false,
			Nfsv41:         false,
			RuleIndex:      2,
			UnixReadOnly:   true,
			UnixReadWrite:  false,
		},
	}

	policy := &ExportPolicy{
		Rules: rules,
	}

	exportResult := exportPolicyExport(policy)

	assert.Equal(t, 2, len(exportResult.Rules))
	assert.Equal(t, int32(1), *((*exportResult).Rules)[0].RuleIndex)
	assert.Equal(t, "10.10.10.0/24", *((*exportResult).Rules)[0].AllowedClients)
	assert.Equal(t, int32(2), *((*exportResult).Rules)[1].RuleIndex)
	assert.Equal(t, "10.10.20.0/24", *((*exportResult).Rules)[1].AllowedClients)

	importResult := exportPolicyImport(exportResult)

	assert.Equal(t, policy, importResult)
}

func TestIsANFNotFoundError_Nil(t *testing.T) {
	result := IsANFNotFoundError(nil)

	assert.False(t, result, "result should be false")
}

func TestIsANFNotFoundError_NotFound(t *testing.T) {
	err := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusNotFound,
		},
	}

	result := IsANFNotFoundError(err)

	assert.True(t, result, "result should be true")
}

func TestIsANFNotFoundError_OtherAutorestError(t *testing.T) {
	err := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusBadRequest,
		},
	}

	result := IsANFNotFoundError(err)

	assert.False(t, result, "result should be false")
}

func TestIsANFNotFoundError_OtherError(t *testing.T) {
	err := errors.New("failed")

	result := IsANFNotFoundError(err)

	assert.False(t, result, "result should be false")
}

func TestIsANFPoolSizeTooSmallError_Nil(t *testing.T) {
	result, err := IsANFPoolSizeTooSmallError(context.Background(), nil)

	assert.False(t, result, "result should be false")
	assert.Nil(t, err, "err should be nil")
}

func TestIsANFPoolSizeTooSmallError_PoolSizeTooSmall(t *testing.T) {
	body := `
		{
			"error": {
				"code": "BadRequest",
				"message": "Mock PoolSizeTooSmall message",
				"details": [
					{
						"code": "PoolSizeTooSmall",
						"message": "Mock PoolSizeTooSmall message"
					}
				]
			}
		}
	`
	err := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}

	result, azErr := IsANFPoolSizeTooSmallError(context.Background(), err)

	var azureerr *AzureError
	ok := errors.As(azErr, &azureerr)
	assert.True(t, result, "result should be true")
	assert.True(t, ok, "expected azure error, got something else")
}

func TestIsANFPoolSizeTooSmallError_SomeOtherError(t *testing.T) {
	body := `
		{
			"error": {
				"code": "BadRequest",
				"message": "Fake message",
				"details": [
					{
						"code": "FakeANFError",
						"message": "Fake message"
					}
				]
			}
		}
	`
	err := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}

	result, azErr := IsANFPoolSizeTooSmallError(context.Background(), err)

	assert.False(t, result, "result should be false")
	assert.Nil(t, azErr, "azure error should be nil")
}

func TestIsANFPoolSizeTooSmallError_InvalidJsonBody(t *testing.T) {
	body := `
		{
			"error": {
				"code": "BadRequest",
				"message": "Mock PoolSizeTooSmall message",
				"details": [
					{
						"code": "PoolSizeTooSmall",
						"message": "Mock PoolSizeTooSmall message"
					},
				]
			}
		}
	`
	err := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}

	result, _ := IsANFPoolSizeTooSmallError(context.Background(), err)

	assert.False(t, result, "result should be false")
}

func TestGetCorrelationIDFromError_Nil(t *testing.T) {
	result := GetCorrelationIDFromError(nil)

	assert.Equal(t, "", result)
}

func TestGetCorrelationIDFromError_NoHeaders(t *testing.T) {
	err := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusNotFound,
		},
	}

	result := GetCorrelationIDFromError(err)

	assert.Equal(t, "", result)
}

func TestGetCorrelationIDFromError_NoCorrelationIDHeader(t *testing.T) {
	err := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(map[string][]string),
		},
	}

	result := GetCorrelationIDFromError(err)

	assert.Equal(t, "", result)
}

func TestGetCorrelationIDFromError_CorrelationIDHeaderEmpty(t *testing.T) {
	err := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(map[string][]string),
		},
	}
	err.RawResponse.Header[CorrelationIDHeader] = []string{}

	result := GetCorrelationIDFromError(err)

	assert.Equal(t, "", result)
}

func TestGetCorrelationIDFromError_CorrelationIDHeaderPresent(t *testing.T) {
	err := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(map[string][]string),
		},
	}
	err.RawResponse.Header[CorrelationIDHeader] = []string{"correlationID"}

	result := GetCorrelationIDFromError(err)

	assert.Equal(t, "correlationID", result)
}

func TestGetCorrelationIDFromError_OtherError(t *testing.T) {
	err := errors.New("failed")

	result := GetCorrelationIDFromError(err)

	assert.Equal(t, "", result)
}

func TestGetMessageFromError_Nil(t *testing.T) {
	result := GetMessageFromError(context.Background(), nil)

	assert.Nil(t, result, "result should be nil")
}

func TestGetMessageFromError_AzureErrorWithDetails(t *testing.T) {
	body := `
		{
			"error": {
				"code": "BadRequest",
				"message": "Fake message",
				"details": [
					{
						"code": "FakeANFError1",
						"message": "Fake message1"
					},
					{
						"code": "FakeANFError2",
						"message": "Fake message2"
					}
				]
			}
		}
	`
	err := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}

	result := GetMessageFromError(context.Background(), err)

	assert.Equal(t, "FakeANFError1: Fake message1; FakeANFError2: Fake message2", result.Error(), "error message not as expected")
}

func TestGetMessageFromError_NonAzureError(t *testing.T) {
	err := errors.New("failed")

	result := GetMessageFromError(context.Background(), err)

	assert.Equal(t, "failed", result.Error(), "error message not as expected")
}

func TestGetMessageFromError_InvalidJsonBody(t *testing.T) {
	body := `
		{
			"error": {
				"code": "BadRequest",
				"message": "Fake message",
				"details": [
					{
						"code": "FakeANFError",
						"message": "Fake message",
					}
				]
			}
		}
	`
	err := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}

	result := GetMessageFromError(context.Background(), err)

	assert.NotEqual(t, "BadRequest: Fake message; FakeANFError: Fake message", result.Error(), "error message not as expected")
}

func TestGetAzureErrorFromError_Nil(t *testing.T) {
	err := parseAzureErrorFromInputError(context.Background(), nil)

	assert.NotNil(t, err, "error is nil")
}

func TestGetAzureErrorFromError_InvalidJsonBody(t *testing.T) {
	body := `
		{
			"error": {
				"code": "BadRequest",
				"message": "Fake message",
				"details": [
					{
						"code": "FakeANFError",
						"message": "Fake message",
					}
				]
			}
		}
	`
	inputErr := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}

	err := parseAzureErrorFromInputError(context.Background(), inputErr)

	assert.NotNil(t, err, "error is nil")
}

func TestGetAzureErrorFromError_UnmarshallingError(t *testing.T) {
	body := `
		{
			"error": {
				"code": 1234,
				"message": "Fake message",
				"details": [
					{
						"code": "FakeANFError",
						"message": "Fake message"
					}
				]
			}
		}
	`
	inputErr := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}

	err := parseAzureErrorFromInputError(context.Background(), inputErr)

	assert.NotNil(t, err, "error is nil")
}

func TestGetAzureErrorFromError_Success(t *testing.T) {
	body := `
		{
			"error": {
				"code": "BadRequest",
				"message": "Failed to create volume as capacity pool is too small",
				"details": [
					{
						"code": "PoolSizeTooSmall",
						"message": "Failed to create volume as capacity pool is too small"
					}
				]
			}
		}
	`
	inputErr := &azcore.ResponseError{
		RawResponse: &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(body)),
		},
	}

	expected := &AzureError{
		AzError: struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details []struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"details"`
		}{
			Code:    "BadRequest",
			Message: "Failed to create volume as capacity pool is too small",
			Details: []struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}([]struct {
				Code    string
				Message string
			}{{"PoolSizeTooSmall", "Failed to create volume as capacity pool is too small"}}),
		},
	}

	err := parseAzureErrorFromInputError(context.Background(), inputErr)

	var azError *AzureError
	ok := errors.As(err, &azError)

	assert.True(t, ok, "error is not of type AzureError")
	assert.Equal(t, expected.AzError.Code, azError.AzError.Code, "error code not equal")
	assert.Equal(t, expected.AzError.Message, azError.AzError.Message, "error message not equal")
	assert.Equal(t, expected.AzError.Details[0].Code, azError.AzError.Details[0].Code, "error details code not equal")
	assert.Equal(t, expected.AzError.Details[0].Message, azError.AzError.Details[0].Message, "error details message not equal")
}

func TestDerefString(t *testing.T) {
	s := "test"

	testCases := []struct {
		Ptr            *string
		ExpectedResult string
	}{
		{nil, ""},
		{&s, "test"},
	}

	for _, testCase := range testCases {
		result := DerefString(testCase.Ptr)
		assert.Equal(t, testCase.ExpectedResult, result)
	}
}

func TestDerefStringArray(t *testing.T) {
	sa := []string{"test1", "test2"}

	testCases := []struct {
		Ptr            *[]string
		ExpectedResult []string
	}{
		{nil, nil},
		{&sa, []string{"test1", "test2"}},
	}

	for _, testCase := range testCases {
		result := DerefStringArray(testCase.Ptr)
		assert.ElementsMatch(t, testCase.ExpectedResult, result)
	}
}

func TestDerefBool(t *testing.T) {
	b1 := true
	b2 := false

	testCases := []struct {
		Ptr            *bool
		ExpectedResult bool
	}{
		{nil, false},
		{&b1, true},
		{&b2, false},
	}

	for _, testCase := range testCases {
		result := DerefBool(testCase.Ptr)
		assert.Equal(t, testCase.ExpectedResult, result)
	}
}

func TestDerefInt32(t *testing.T) {
	i1 := int32(0)
	i2 := int32(42)

	testCases := []struct {
		Ptr            *int32
		ExpectedResult int32
	}{
		{nil, 0},
		{&i1, 0},
		{&i2, 42},
	}

	for _, testCase := range testCases {
		result := DerefInt32(testCase.Ptr)
		assert.Equal(t, testCase.ExpectedResult, result)
	}
}

func TestDerefInt64(t *testing.T) {
	i1 := int64(0)
	i2 := int64(42)

	testCases := []struct {
		Ptr            *int64
		ExpectedResult int64
	}{
		{nil, 0},
		{&i1, 0},
		{&i2, 42},
	}

	for _, testCase := range testCases {
		result := DerefInt64(testCase.Ptr)
		assert.Equal(t, testCase.ExpectedResult, result)
	}
}

func TestDerefFloat32(t *testing.T) {
	i1 := float32(0.0)
	i2 := float32(42.0)

	testCases := []struct {
		Ptr            *float32
		ExpectedResult float32
	}{
		{nil, 0.0},
		{&i1, 0.0},
		{&i2, 42.0},
	}

	for _, testCase := range testCases {
		result := DerefFloat32(testCase.Ptr)
		assert.Equal(t, testCase.ExpectedResult, result)
	}
}

func TestIsTerminalStateError(t *testing.T) {
	err := TerminalState(errors.New("terminal"))

	assert.True(t, IsTerminalStateError(err), "not terminal state error")
	assert.Equal(t, "terminal", err.Error())

	assert.False(t, IsTerminalStateError(nil))
	assert.False(t, IsTerminalStateError(errors.New("not terminal")))
}

func TestCreateKeyVaultEndpoint(t *testing.T) {
	subnet := "sub123"
	resourceGroup := "resourceGrp123"
	keyVaultEndpoint := "keyVaultEP123"

	expected := "/subscriptions/sub123/resourceGroups/resourceGrp123/providers/Microsoft.Network/privateEndpoints/keyVaultEP123"

	result := CreateKeyVaultEndpoint(subnet, resourceGroup, keyVaultEndpoint)

	assert.Equal(t, expected, result, "endpoint mismatch")
}

// TestValidateCloudConfiguration_Nil tests that nil config returns AzurePublic
func TestValidateCloudConfiguration_Nil(t *testing.T) {
	config, err := ValidateCloudConfiguration(nil)
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "https://login.microsoftonline.com/", config.ActiveDirectoryAuthorityHost)
}

// TestValidateCloudConfiguration_Empty tests that empty config returns AzurePublic
func TestValidateCloudConfiguration_Empty(t *testing.T) {
	config, err := ValidateCloudConfiguration(&CloudConfiguration{})
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "https://login.microsoftonline.com/", config.ActiveDirectoryAuthorityHost)
}

// TestValidateCloudConfiguration_NamedClouds tests various named cloud configurations
func TestValidateCloudConfiguration_NamedClouds(t *testing.T) {
	tests := []struct {
		name                string
		cloudName           string
		expectError         bool
		expectedAuthority   string
		expectedErrorString string
	}{
		{
			name:              "AzurePublic",
			cloudName:         "AzurePublic",
			expectError:       false,
			expectedAuthority: "https://login.microsoftonline.com/",
		},
		{
			name:              "AzureChina",
			cloudName:         "AzureChina",
			expectError:       false,
			expectedAuthority: "https://login.chinacloudapi.cn/",
		},
		{
			name:              "AzureGovernment",
			cloudName:         "AzureGovernment",
			expectError:       false,
			expectedAuthority: "https://login.microsoftonline.us/",
		},
		{
			name:                "InvalidCloudName",
			cloudName:           "InvalidCloud",
			expectError:         true,
			expectedErrorString: "unknown cloudName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ValidateCloudConfiguration(&CloudConfiguration{CloudName: tt.cloudName})
			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, config)
				assert.Contains(t, err.Error(), tt.expectedErrorString)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, config)
				assert.Equal(t, tt.expectedAuthority, config.ActiveDirectoryAuthorityHost)
			}
		})
	}
}

// TestValidateCloudConfiguration_CustomValid tests valid custom configuration
func TestValidateCloudConfiguration_CustomValid(t *testing.T) {
	config, err := ValidateCloudConfiguration(&CloudConfiguration{
		ADAuthorityHost: "https://login.example.com/",
		Audience:        "https://management.example.com",
		Endpoint:        "https://api.example.com",
	})
	assert.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "https://login.example.com/", config.ActiveDirectoryAuthorityHost)
	assert.Equal(t, "https://management.example.com", config.Services["resourceManager"].Audience)
	assert.Equal(t, "https://api.example.com", config.Services["resourceManager"].Endpoint)
}

// TestValidateCloudConfiguration_IncompleteCustomConfig tests incomplete custom configurations
func TestValidateCloudConfiguration_IncompleteCustomConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *CloudConfiguration
	}{
		{
			name: "OnlyADAuthorityHost",
			config: &CloudConfiguration{
				ADAuthorityHost: "https://login.example.com/",
			},
		},
		{
			name: "OnlyAudience",
			config: &CloudConfiguration{
				Audience: "https://management.example.com",
			},
		},
		{
			name: "OnlyEndpoint",
			config: &CloudConfiguration{
				Endpoint: "https://api.example.com",
			},
		},
		{
			name: "ADAuthorityHostAndAudience",
			config: &CloudConfiguration{
				ADAuthorityHost: "https://login.example.com/",
				Audience:        "https://management.example.com",
			},
		},
		{
			name: "ADAuthorityHostAndEndpoint",
			config: &CloudConfiguration{
				ADAuthorityHost: "https://login.example.com/",
				Endpoint:        "https://api.example.com",
			},
		},
		{
			name: "AudienceAndEndpoint",
			config: &CloudConfiguration{
				Audience: "https://management.example.com",
				Endpoint: "https://api.example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ValidateCloudConfiguration(tt.config)
			assert.Error(t, err)
			assert.Nil(t, config)
			assert.Contains(t, err.Error(), "all required")
		})
	}
}

// TestValidateCloudConfiguration_MutuallyExclusive tests that cloudName and custom fields are mutually exclusive
func TestValidateCloudConfiguration_MutuallyExclusive(t *testing.T) {
	tests := []struct {
		name   string
		config *CloudConfiguration
	}{
		{
			name: "CloudNameWithAllCustomFields",
			config: &CloudConfiguration{
				CloudName:       "AzurePublic",
				ADAuthorityHost: "https://login.example.com/",
				Audience:        "https://management.example.com",
				Endpoint:        "https://api.example.com",
			},
		},
		{
			name: "CloudNameWithADAuthorityHost",
			config: &CloudConfiguration{
				CloudName:       "AzureChina",
				ADAuthorityHost: "https://login.example.com/",
			},
		},
		{
			name: "CloudNameWithAudience",
			config: &CloudConfiguration{
				CloudName: "AzureGovernment",
				Audience:  "https://management.example.com",
			},
		},
		{
			name: "CloudNameWithEndpoint",
			config: &CloudConfiguration{
				CloudName: "AzurePublic",
				Endpoint:  "https://api.example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ValidateCloudConfiguration(tt.config)
			assert.Error(t, err)
			assert.Nil(t, config)
			assert.Contains(t, err.Error(), "mutually exclusive")
		})
	}
}

// TestValidateCloudConfiguration_InvalidURL tests invalid URL validation
func TestValidateCloudConfiguration_InvalidURL(t *testing.T) {
	tests := []struct {
		name                string
		config              *CloudConfiguration
		expectedErrorString string
	}{
		{
			name: "InvalidADAuthorityHost",
			config: &CloudConfiguration{
				ADAuthorityHost: "not a valid url://",
				Audience:        "https://management.example.com",
				Endpoint:        "https://api.example.com",
			},
			expectedErrorString: "invalid adAuthorityHost URL",
		},
		{
			name: "InvalidAudience",
			config: &CloudConfiguration{
				ADAuthorityHost: "https://login.example.com/",
				Audience:        "not a valid url://",
				Endpoint:        "https://api.example.com",
			},
			expectedErrorString: "invalid audience URL",
		},
		{
			name: "InvalidEndpoint",
			config: &CloudConfiguration{
				ADAuthorityHost: "https://login.example.com/",
				Audience:        "https://management.example.com",
				Endpoint:        "not a valid url://",
			},
			expectedErrorString: "invalid endpoint URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := ValidateCloudConfiguration(tt.config)
			assert.Error(t, err)
			assert.Nil(t, config)
			assert.Contains(t, err.Error(), tt.expectedErrorString)
		})
	}
}

// ////////////////////////////////////////////////////////////////////////////
// Tests for GetAzureCredential
// ////////////////////////////////////////////////////////////////////////////

// Helper function to create a test ClientConfig
func createTestClientConfig(cloudConfig *CloudConfiguration) ClientConfig {
	return ClientConfig{
		TenantID:        "test-tenant-id",
		CloudConfig:     cloudConfig,
		AzureAuthConfig: azclient.AzureAuthConfig{},
	}
}

func TestGetAzureCredential_NoCloudConfig(t *testing.T) {
	// Test with nil cloud config - should default to AzurePublic
	config := createTestClientConfig(nil)

	// Validate cloud configuration
	cloudConfig, err := ValidateCloudConfiguration(config.CloudConfig)
	assert.NoError(t, err, "ValidateCloudConfiguration should not error with nil config")

	_, err = GetAzureCredential(config, cloudConfig)
	// Since we don't have actual Azure credentials, we expect this to succeed in creating
	// the auth provider structure, even though it won't return a valid credential
	// The important thing is it doesn't error on cloud configuration validation
	assert.NoError(t, err, "GetAzureCredential should not error with nil cloud config")
}

func TestGetAzureCredential_EmptyCloudConfig(t *testing.T) {
	// Test with empty cloud config - should default to AzurePublic
	config := createTestClientConfig(&CloudConfiguration{})

	// Validate cloud configuration
	cloudConfig, err := ValidateCloudConfiguration(config.CloudConfig)
	assert.NoError(t, err, "ValidateCloudConfiguration should not error with empty config")

	_, err = GetAzureCredential(config, cloudConfig)
	assert.NoError(t, err, "GetAzureCredential should not error with empty cloud config")
}

func TestGetAzureCredential_NamedClouds(t *testing.T) {
	tests := []struct {
		name      string
		cloudName string
	}{
		{
			name:      "AzurePublic",
			cloudName: "AzurePublic",
		},
		{
			name:      "AzureChina",
			cloudName: "AzureChina",
		},
		{
			name:      "AzureGovernment",
			cloudName: "AzureGovernment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := createTestClientConfig(&CloudConfiguration{
				CloudName: tt.cloudName,
			})

			// Validate cloud configuration
			cloudConfig, err := ValidateCloudConfiguration(config.CloudConfig)
			assert.NoError(t, err, "ValidateCloudConfiguration should not error with valid cloud name: %s", tt.cloudName)

			_, err = GetAzureCredential(config, cloudConfig)
			assert.NoError(t, err, "GetAzureCredential should not error with valid named cloud: %s", tt.cloudName)
		})
	}
}

func TestGetAzureCredential_CustomCloud(t *testing.T) {
	config := createTestClientConfig(&CloudConfiguration{
		ADAuthorityHost: "https://login.custom.cloud/",
		Audience:        "https://management.custom.cloud",
		Endpoint:        "https://management.custom.cloud",
	})

	// Validate cloud configuration
	cloudConfig, err := ValidateCloudConfiguration(config.CloudConfig)
	assert.NoError(t, err, "ValidateCloudConfiguration should not error with valid custom config")

	// Note: This test will fail with a network error because the custom cloud URL is fake.
	// This is expected behavior - validation of custom cloud URLs happens when the auth
	// provider attempts to connect to the endpoint.
	_, err = GetAzureCredential(config, cloudConfig)
	// We expect an error due to network failure, not a validation error
	assert.Error(t, err, "GetAzureCredential should error when custom cloud endpoint is unreachable")
	assert.Contains(t, err.Error(), "error creating azure auth provider")
}

func TestGetAzureCredential_InvalidCloudName(t *testing.T) {
	config := createTestClientConfig(&CloudConfiguration{
		CloudName: "InvalidCloudName",
	})

	// Validate cloud configuration - should fail for invalid cloud name
	_, err := ValidateCloudConfiguration(config.CloudConfig)
	assert.Error(t, err, "ValidateCloudConfiguration should error with invalid cloud name")
	assert.Contains(t, err.Error(), "unknown cloudName")
}

func TestGetAzureCredential_IncompleteCustomConfig(t *testing.T) {
	config := createTestClientConfig(&CloudConfiguration{
		ADAuthorityHost: "https://login.custom.cloud/",
		Audience:        "https://management.custom.cloud",
		// Missing Endpoint - should cause validation error
	})

	// Validate cloud configuration - should fail for incomplete custom config
	_, err := ValidateCloudConfiguration(config.CloudConfig)
	assert.Error(t, err, "ValidateCloudConfiguration should error with incomplete custom config")
	assert.Contains(t, err.Error(), "adAuthorityHost, audience, and endpoint are all required")
}

func TestGetAzureCredential_MutuallyExclusiveConfig(t *testing.T) {
	config := createTestClientConfig(&CloudConfiguration{
		CloudName:       "AzurePublic",
		ADAuthorityHost: "https://login.custom.cloud/",
		Audience:        "https://management.custom.cloud",
		Endpoint:        "https://management.custom.cloud",
	})

	// Validate cloud configuration - should fail for mutually exclusive config
	_, err := ValidateCloudConfiguration(config.CloudConfig)
	assert.Error(t, err, "ValidateCloudConfiguration should error with mutually exclusive config")
	assert.Contains(t, err.Error(), "mutually exclusive")
}

func TestGetAzureCredential_InvalidURLInCustomConfig(t *testing.T) {
	config := createTestClientConfig(&CloudConfiguration{
		ADAuthorityHost: "https://login.custom.cloud/",
		Audience:        "https://management.custom.cloud",
		Endpoint:        "https://management.custom.cloud",
	})

	// Validate cloud configuration - should succeed (url.Parse is permissive)
	cloudConfig, err := ValidateCloudConfiguration(config.CloudConfig)
	assert.NoError(t, err, "ValidateCloudConfiguration accepts syntactically valid URLs")

	// The actual network error will occur when trying to use the credential
	_, err = GetAzureCredential(config, cloudConfig)
	assert.Error(t, err, "GetAzureCredential should error when custom cloud endpoint is unreachable")
	assert.Contains(t, err.Error(), "error creating azure auth provider")
}
