// Copyright 2024 NetApp, Inc. All Rights Reserved.

package storage_drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	mockConfClients "github.com/netapp/trident/mocks/mock_operator/mock_controllers/mock_configurator/mock_clients"
	mockAzureClient "github.com/netapp/trident/mocks/mock_storage_drivers/mock_azure"
	operatorV1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	"github.com/netapp/trident/storage_drivers/azure/api"
)

var ctx = context.TODO()

func getTestANFInstanceAndClients(t *testing.T) (*ANF, *mockConfClients.MockConfiguratorClientInterface, *mockAzureClient.MockAzure) {
	mockCtrl := gomock.NewController(t)
	mockClient := mockConfClients.NewMockConfiguratorClientInterface(mockCtrl)
	mockAzure := mockAzureClient.NewMockAzure(mockCtrl)

	anfConfig := ANFConfig{
		SubscriptionID:    "fake-uuid",
		TenantID:          "fake-uuid",
		Location:          "fake-location",
		ClientID:          "fake-uuid",
		ClientSecret:      "fake-secret",
		ClientCredentials: "fake-cred",
	}

	return &ANF{
		ANFConfig:        anfConfig,
		AZClient:         mockAzure,
		ConfClient:       mockClient,
		TBCNamePrefix:    "anf-tbc",
		TridentNamespace: "ns",
	}, mockClient, mockAzure
}

func getFilteredANFResources() (map[string]*api.CapacityPool, map[string]*api.Subnet) {
	cp1 := &api.CapacityPool{
		ID:            "cp1-uuid",
		ResourceGroup: "fake-rg",
		NetAppAccount: "fake-na",
		Name:          "cp1",
		FullName:      "fake-rg/fake-na/cp1",
		Location:      "fake-location",
		ServiceLevel:  api.ServiceLevelStandard,
	}

	cp2 := &api.CapacityPool{
		ID:            "cp2-uuid",
		ResourceGroup: "fake-rg",
		NetAppAccount: "fake-na",
		Name:          "cp2",
		FullName:      "fake-rg/fake-na/cp2",
		Location:      "fake-location",
		ServiceLevel:  api.ServiceLevelPremium,
	}

	cp3 := &api.CapacityPool{
		ID:            "cp3-uuid",
		ResourceGroup: "fake-rg",
		NetAppAccount: "fake-na",
		Name:          "cp3",
		FullName:      "fake-rg/fake-na/cp3",
		Location:      "fake-location",
		ServiceLevel:  api.ServiceLevelUltra,
	}

	// Adding 2nd CP of same service level to increase the code coverage in ANFVPoolMap/ANFStorageClassMap functions.
	cp4 := &api.CapacityPool{
		ID:            "cp4-uuid",
		ResourceGroup: "fake-rg",
		NetAppAccount: "fake-na",
		Name:          "cp4",
		FullName:      "fake-rg/fake-na/cp4",
		Location:      "fake-location",
		ServiceLevel:  api.ServiceLevelUltra,
	}

	cpMap := make(map[string]*api.CapacityPool, 4)
	cpMap[cp1.FullName] = cp1
	cpMap[cp2.FullName] = cp2
	cpMap[cp3.FullName] = cp3
	cpMap[cp4.FullName] = cp4

	s1 := &api.Subnet{
		ID:             "s1-uuid",
		ResourceGroup:  "fake-rg",
		VirtualNetwork: "fake-vnet",
		Name:           "s1",
		FullName:       "fake-rg/fake-vnet/s1",
		Location:       "fake-location",
	}

	sMap := make(map[string]*api.Subnet, 1)
	sMap[s1.FullName] = s1

	return cpMap, sMap
}

func MustEncode(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}

	return b
}

func TestNewANFInstance(t *testing.T) {
	_, mClient, _ := getTestANFInstanceAndClients(t)
	torcCR := &operatorV1.TridentOrchestrator{}
	tconfCR := &operatorV1.TridentConfigurator{}

	// Test1: Nil TorcCR.
	_, err := NewANFInstance(nil, nil, nil)

	assert.ErrorContains(t, err, "empty torc CR", "Torc CR is not nil.")

	// Test2: Nil ConfiguratorCR.
	_, err = NewANFInstance(torcCR, nil, nil)

	assert.ErrorContains(t, err, "empty ANF configurator CR", "Configurator CR is not nil.")

	// Test3: Nil Configurator Clients.
	_, err = NewANFInstance(torcCR, tconfCR, nil)

	assert.ErrorContains(t, err, "invalid client", "Valid Configurator Client.")

	// Test4: Json Unmarshal error.
	_, err = NewANFInstance(torcCR, tconfCR, mClient)

	assert.Error(t, err, "Configurator Unmarshal success.")

	// Test5: Get new ANF instance successfully.
	tconfCRSpecContents := ANFConfig{SubscriptionID: "fake-uuid"}
	tconfCR.Spec.RawExtension = runtime.RawExtension{Raw: MustEncode(json.Marshal(tconfCRSpecContents))}

	_, err = NewANFInstance(torcCR, tconfCR, mClient)

	assert.NoError(t, err, "Failed to get ANF instance.")
}

func TestANF_Validate_WorkloadIdentityDiscoveryError(t *testing.T) {
	anf, _, mAzure := getTestANFInstanceAndClients(t)

	// Workload Identity
	envVariables := map[string]string{
		"AZURE_CLIENT_ID":            "deadbeef-784c-4b35-8329-460f52a3ad50",
		"AZURE_TENANT_ID":            "deadbeef-4746-4444-a919-3b34af5f0a3c",
		"AZURE_FEDERATED_TOKEN_FILE": "/test/file/path",
		"AZURE_AUTHORITY_HOST":       "https://msft.com/",
	}

	// Set required environment variables for testing
	for key, value := range envVariables {
		_ = os.Setenv(key, value)
	}

	mAzure.EXPECT().DiscoverAzureResources(ctx).Return(fmt.Errorf("failed to discover resources"))

	err := anf.Validate()

	assert.True(t, anf.WorkloadIdentityEnabled, "Workload Identity not enabled.")
	assert.Error(t, err, "Discovered all the resources.")

	// Unset all the environment variables
	for key := range envVariables {
		_ = os.Unsetenv(key)
	}
}

func TestANF_Validate_AMINoFileError(t *testing.T) {
	anf, _, _ := getTestANFInstanceAndClients(t)

	anf.ClientID = ""
	anf.ClientSecret = ""
	anf.AMIEnabled = true

	envVariable := map[string]string{
		"AZURE_CREDENTIAL_FILE": "/no/such/path/azure.json",
	}

	// Set required environment variables for testing
	for key, value := range envVariable {
		_ = os.Setenv(key, value)
	}

	err := anf.Validate()

	assert.ErrorContains(t, err, "error reading from azure config file", "Read azure.json file.")

	// Unset environment variable
	for key := range envVariable {
		_ = os.Unsetenv(key)
	}
}

func TestANF_Validate_AMIUnmarshalFileError(t *testing.T) {
	anf, _, _ := getTestANFInstanceAndClients(t)

	anf.ClientID = ""
	anf.ClientSecret = ""
	anf.AMIEnabled = true

	configFile, _ := os.Getwd()
	envVariable := map[string]string{
		"AZURE_CREDENTIAL_FILE": configFile + "azure.json",
	}

	// Set required environment variables for testing
	for key, value := range envVariable {
		_ = os.Setenv(key, value)
	}

	// Giving string as int to fail Unmarshal
	configFileContent := `
	{
	  "aadClientId": 111,
	}`

	_ = os.WriteFile(envVariable["AZURE_CREDENTIAL_FILE"], []byte(configFileContent), os.ModePerm)

	err := anf.Validate()

	assert.ErrorContains(t, err, "error parsing azureAuthConfig", "Read azure.json file.")

	// Remove the file
	_ = os.Remove(envVariable["AZURE_CREDENTIAL_FILE"])

	// Unset environment variable
	for key := range envVariable {
		_ = os.Unsetenv(key)
	}
}

func TestANF_Validate_AMIDiscoveryError(t *testing.T) {
	anf, _, mAzure := getTestANFInstanceAndClients(t)

	anf.ClientID = ""
	anf.ClientSecret = ""
	anf.AMIEnabled = true

	configFile, _ := os.Getwd()
	envVariable := map[string]string{
		"AZURE_CREDENTIAL_FILE": configFile + "azure.json",
	}

	// Set required environment variables for testing
	for key, value := range envVariable {
		_ = os.Setenv(key, value)
	}

	configFileContent := `
	{
	  "cloud": "AzurePublicCloud",
	  "tenantId": "deadbeef-784c-4b35-8329-460f52a3ad50",
	  "subscriptionId": "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b",
	  "aadClientId": "test-msi",
	  "aadClientSecret": "test-msi",
	  "resourceGroup": "RG1",
	  "location": "fake-location",
	  "useManagedIdentityExtension": true,
	  "userAssignedIdentityID": "deadbeef-173f-4bf4-b5b8-7cba6f53a227"
	}`

	_ = os.WriteFile(envVariable["AZURE_CREDENTIAL_FILE"], []byte(configFileContent), os.ModePerm)

	mAzure.EXPECT().DiscoverAzureResources(ctx).Return(fmt.Errorf("failed to discover resources"))

	err := anf.Validate()

	assert.Equal(t, "deadbeef-173f-4bf4-b5b8-f17f8d2fe43b", anf.SubscriptionID, "Failed to set subscription ID.")
	assert.Error(t, err, "Discovered all the resources.")

	// Remove the file
	_ = os.Remove(envVariable["AZURE_CREDENTIAL_FILE"])

	// Unset environment variable
	for key := range envVariable {
		_ = os.Unsetenv(key)
	}
}

func TestANF_Validate_NoClientSecretError(t *testing.T) {
	anf, mClient, _ := getTestANFInstanceAndClients(t)

	mClient.EXPECT().GetANFSecrets(gomock.Any()).Return("", "", fmt.Errorf("failed to get secret"))

	err := anf.Validate()

	assert.ErrorContains(t, err, "failed to get secret", "Got the Client secret.")
}

func TestANF_Validate_PopulateAZResourcesError(t *testing.T) {
	anf, mClient, mAzure := getTestANFInstanceAndClients(t)
	cMap, _ := getFilteredANFResources()

	// Test1: No capacity pool error.
	mClient.EXPECT().GetANFSecrets(gomock.Any()).Return("fake-uuid", "fake-secret", nil)
	mAzure.EXPECT().DiscoverAzureResources(ctx).Return(nil)
	mAzure.EXPECT().FilteredCapacityPoolMap(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(make(map[string]*api.CapacityPool))

	err := anf.Validate()

	assert.ErrorContains(t, err, "no capacity pools discovered after filtering")

	// Test2: No subnets error.
	mClient.EXPECT().GetANFSecrets(gomock.Any()).Return("fake-uuid", "fake-secret", nil)
	mAzure.EXPECT().DiscoverAzureResources(ctx).Return(nil)
	mAzure.EXPECT().FilteredCapacityPoolMap(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(cMap)
	mAzure.EXPECT().FilteredSubnetMap(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(make(map[string]*api.Subnet))

	err = anf.Validate()

	assert.ErrorContains(t, err, "no ANF subnets discovered after filtering")
}

func TestANF_Validate_AvailabilityZoneError(t *testing.T) {
	anf, mClient, mAzure := getTestANFInstanceAndClients(t)
	cMap, sMap := getFilteredANFResources()

	mClient.EXPECT().GetANFSecrets(gomock.Any()).Return("fake-uuid", "fake-secret", nil)
	mAzure.EXPECT().DiscoverAzureResources(ctx).Return(nil)
	mAzure.EXPECT().FilteredCapacityPoolMap(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(cMap)
	mAzure.EXPECT().FilteredSubnetMap(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(sMap)
	mAzure.EXPECT().AvailabilityZones(gomock.Any()).Return([]string{}, fmt.Errorf("failed to get availability zones"))

	err := anf.Validate()

	assert.ErrorContains(t, err, "failed to get availability zones", "Got the availability zones")
}

func TestANF_Validate_Success(t *testing.T) {
	anf, mClient, mAzure := getTestANFInstanceAndClients(t)
	cMap, sMap := getFilteredANFResources()

	mClient.EXPECT().GetANFSecrets(gomock.Any()).Return("fake-uuid", "fake-secret", nil)
	mAzure.EXPECT().DiscoverAzureResources(ctx).Return(nil)
	mAzure.EXPECT().FilteredCapacityPoolMap(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(cMap)
	mAzure.EXPECT().FilteredSubnetMap(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(sMap)
	mAzure.EXPECT().AvailabilityZones(gomock.Any()).Return([]string{"1", "2", "3"}, nil)

	err := anf.Validate()

	assert.NoError(t, err, "Validation failed.")
}

func TestANF_Create(t *testing.T) {
	anf, mClient, _ := getTestANFInstanceAndClients(t)
	cpMap, sMap := getFilteredANFResources()
	anf.FilteredCapacityPoolMap = cpMap
	anf.FilteredSubnetMap = sMap
	anf.CapacityPools = []string{"cp1"}
	anf.Location = "fake-location"
	anf.AvailabilityZones = []string{"1", "2", "3"}

	// Test1: ANF NFS backend create error.
	mClient.EXPECT().CreateOrPatchObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fmt.Errorf("failed to create ANF NFS backend"))

	_, err := anf.Create()

	assert.ErrorContains(t, err, "failed to create ANF NFS backend", "NFS backend created.")

	// Test2: ANF SMB backend create error.
	mClient.EXPECT().CreateOrPatchObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mClient.EXPECT().CreateOrPatchObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fmt.Errorf("failed to create ANF SMB backend"))

	_, err = anf.Create()

	assert.ErrorContains(t, err, "failed to create ANF SMB backend", "SMB backend created.")

	// Test3: Successfully created both the backends.
	mClient.EXPECT().CreateOrPatchObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mClient.EXPECT().CreateOrPatchObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	backends, err := anf.Create()

	assert.NoError(t, err, "Backend creation failed.")
	assert.Equal(t, "anf-tbc-nfs", backends[0], "Wrong TBC name.")
	assert.Equal(t, "anf-tbc-smb", backends[1], "Wrong TBC name.")
}

func TestANF_CreateStorageClass(t *testing.T) {
	anf, mClient, _ := getTestANFInstanceAndClients(t)
	cpMap, sMap := getFilteredANFResources()
	anf.FilteredCapacityPoolMap = cpMap
	anf.FilteredSubnetMap = sMap

	// Test1: Storage class create failure.
	mClient.EXPECT().CreateOrPatchObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fmt.Errorf("failed to create storage class"))

	err := anf.CreateStorageClass()

	assert.ErrorContains(t, err, "failed to create storage class", "Created Storage class.")

	// Test2: Successfully created all the storage classes.
	mClient.EXPECT().CreateOrPatchObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(6)

	err = anf.CreateStorageClass()

	assert.NoError(t, err, "Failed to create storage classes.")
}

func TestANF_CreateSnapshotClass(t *testing.T) {
	anf, mClient, _ := getTestANFInstanceAndClients(t)

	// Test1: Snapshot class create failure.
	mClient.EXPECT().CreateOrPatchObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(fmt.Errorf("failed to create snapshot class"))

	err := anf.CreateSnapshotClass()

	assert.ErrorContains(t, err, "failed to create snapshot class", "Created Snapshot class.")

	// Test2: Successfully created Snapshot class.
	mClient.EXPECT().CreateOrPatchObject(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	err = anf.CreateSnapshotClass()

	assert.NoError(t, err, "Failed to create Snapshot class.")
}

func TestANF_GetCloudProvider(t *testing.T) {
	anf, _, _ := getTestANFInstanceAndClients(t)

	// Test1: Non-AMI.
	cp := anf.GetCloudProvider()

	assert.Equal(t, "None", cp, "Cloud Provider is Azure.")

	// Test2: AMI or Workload Identity.
	anf.AMIEnabled = true

	cp = anf.GetCloudProvider()

	assert.Equal(t, k8sclient.CloudProviderAzure, cp, "Cloud Provider is None.")
}
