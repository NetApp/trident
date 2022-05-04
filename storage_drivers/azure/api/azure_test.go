// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"errors"
	"net/http"
	"testing"

	"github.com/Azure/go-autorest/autorest"
	"github.com/stretchr/testify/assert"
)

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

func TestCreateSubvolumeID(t *testing.T) {
	actual := CreateSubvolumeID("mySubscription", "myResourceGroup", "myNetappAccount", "myCapacityPool", "myVolume", "mySubvolume")

	expected := "/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/subvolumes/mySubvolume"

	assert.Equal(t, expected, actual, "subvolume IDs not equal")
}

func TestCreateSubvolumeFullName(t *testing.T) {
	actual := CreateSubvolumeFullName("myResourceGroup", "myNetappAccount", "myCapacityPool", "myVolume", "mySubvolume")

	expected := "myResourceGroup/myNetappAccount/myCapacityPool/myVolume/mySubvolume"

	assert.Equal(t, expected, actual, "subvolume full names not equal")
}

func TestParseSubvolumeID(t *testing.T) {
	subscriptionID, resourceGroup, provider, netappAccount, capacityPool, volume, subvolume, err := ParseSubvolumeID(
		"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/subvolumes/mySubvolume")

	assert.Equal(t, "mySubscription", subscriptionID, "subscriptionID not correct")
	assert.Equal(t, "myResourceGroup", resourceGroup, "resourceGroup not correct")
	assert.Equal(t, "Microsoft.NetApp", provider, "provider not correct")
	assert.Equal(t, "myNetappAccount", netappAccount, "netappAccount not correct")
	assert.Equal(t, "myCapacityPool", capacityPool, "capacityPool not correct")
	assert.Equal(t, "myVolume", volume, "volume not correct")
	assert.Equal(t, "mySubvolume", subvolume, "subvolume not correct")
	assert.NoError(t, err, "error is not nil")
}

func TestParseSubvolumeIDNegative(t *testing.T) {
	tests := []struct {
		description string
		input       string
	}{
		{
			"no subscriptions key",
			"/subscription/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/subvolumes/mySubvolume",
		},
		{
			"no subscriptions value",
			"/subscriptions/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/subvolumes/mySubvolume",
		},
		{
			"no resource groups key",
			"/subscriptions/mySubscription/resourceGroup/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/subvolumes/mySubvolume",
		},
		{
			"no resource groups value",
			"/subscriptions/mySubscription/resourceGroups/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/subvolumes/mySubvolume",
		},
		{
			"no providers key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/provider/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/subvolumes/mySubvolume",
		},
		{
			"no providers value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/subvolumes/mySubvolume",
		},
		{
			"no accounts key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/myNetappAccount/capacityPools/myCapacityPool/volumes/myVolume/subvolumes/mySubvolume",
		},
		{
			"no accounts value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/capacityPools/myCapacityPool/volumes/myVolume/subvolumes/mySubvolume",
		},
		{
			"no capacity pools key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/myCapacityPool/volumes/myVolume/subvolumes/mySubvolume",
		},
		{
			"no capacity pools value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/volumes/myVolume/subvolumes/mySubvolume",
		},
		{
			"no volumes key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/myVolume/subvolumes/mySubvolume",
		},
		{
			"no volumes value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/subvolumes/mySubvolume",
		},
		{
			"no subvolumes key",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/myVolume/subvolume/mySubvolume",
		},
		{
			"no subvolumes value",
			"/subscriptions/mySubscription/resourceGroups/myResourceGroup/providers/Microsoft.NetApp/netAppAccounts/myNetappAccount/capacityPools/myCapacityPool/volumes/subvolumes",
		},
	}

	for _, test := range tests {

		_, _, _, _, _, _, _, err := ParseSubvolumeID(test.input)
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

	assert.Equal(t, 2, len(*exportResult.Rules))
	assert.Equal(t, int32(1), *(*(*exportResult).Rules)[0].RuleIndex)
	assert.Equal(t, "10.10.10.0/24", *(*(*exportResult).Rules)[0].AllowedClients)
	assert.Equal(t, int32(2), *(*(*exportResult).Rules)[1].RuleIndex)
	assert.Equal(t, "10.10.20.0/24", *(*(*exportResult).Rules)[1].AllowedClients)

	importResult := exportPolicyImport(exportResult)

	assert.Equal(t, policy, importResult)
}

func TestIsANFNotFoundError_Nil(t *testing.T) {
	result := IsANFNotFoundError(nil)

	assert.False(t, result, "result should be false")
}

func TestIsANFNotFoundError_NotFound(t *testing.T) {
	err := autorest.DetailedError{
		Response: &http.Response{
			StatusCode: http.StatusNotFound,
		},
	}

	result := IsANFNotFoundError(err)

	assert.True(t, result, "result should be true")
}

func TestIsANFNotFoundError_OtherAutorestError(t *testing.T) {
	err := autorest.DetailedError{
		Response: &http.Response{
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

func TestGetCorrelationIDFromError_Nil(t *testing.T) {
	result := GetCorrelationIDFromError(nil)

	assert.Equal(t, "", result)
}

func TestGetCorrelationIDFromError_NoHeaders(t *testing.T) {
	err := autorest.DetailedError{
		Response: &http.Response{
			StatusCode: http.StatusNotFound,
		},
	}

	result := GetCorrelationIDFromError(err)

	assert.Equal(t, "", result)
}

func TestGetCorrelationIDFromError_NoCorrelationIDHeader(t *testing.T) {
	err := autorest.DetailedError{
		Response: &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(map[string][]string),
		},
	}

	result := GetCorrelationIDFromError(err)

	assert.Equal(t, "", result)
}

func TestGetCorrelationIDFromError_CorrelationIDHeaderEmpty(t *testing.T) {
	err := autorest.DetailedError{
		Response: &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(map[string][]string),
		},
	}
	err.Response.Header[CorrelationIDHeader] = []string{}

	result := GetCorrelationIDFromError(err)

	assert.Equal(t, "", result)
}

func TestGetCorrelationIDFromError_CorrelationIDHeaderPresent(t *testing.T) {
	err := autorest.DetailedError{
		Response: &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     make(map[string][]string),
		},
	}
	err.Response.Header[CorrelationIDHeader] = []string{"correlationID"}

	result := GetCorrelationIDFromError(err)

	assert.Equal(t, "correlationID", result)
}

func TestGetCorrelationIDFromError_OtherError(t *testing.T) {
	err := errors.New("failed")

	result := GetCorrelationIDFromError(err)

	assert.Equal(t, "", result)
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

func TestIsTerminalStateError(t *testing.T) {
	err := TerminalState(errors.New("terminal"))

	assert.True(t, IsTerminalStateError(err), "not terminal state error")
	assert.Equal(t, "terminal", err.Error())

	assert.False(t, IsTerminalStateError(nil))
	assert.False(t, IsTerminalStateError(errors.New("not terminal")))
}
