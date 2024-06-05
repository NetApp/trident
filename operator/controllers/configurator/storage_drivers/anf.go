// Copyright 2024 NetApp, Inc. All Rights Reserved.

package storage_drivers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/cloud-provider-azure/pkg/azclient"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	confClients "github.com/netapp/trident/operator/controllers/configurator/clients"
	operatorV1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_drivers/azure/api"
)

type ANF struct {
	ANFConfig

	AZClient   api.Azure
	ConfClient confClients.ConfiguratorClientInterface

	FilteredCapacityPoolMap map[string]*api.CapacityPool
	FilteredSubnetMap       map[string]*api.Subnet
	AvailabilityZones       []string

	AMIEnabled              bool
	WorkloadIdentityEnabled bool
	TBCNamePrefix           string
	TridentNamespace        string
}

type ANFConfig struct {
	// Access related
	SubscriptionID    string `json:"subscriptionID"`
	TenantID          string `json:"tenantID"`
	Location          string `json:"location"`
	ClientID          string `json:"clientID"`
	ClientSecret      string `json:"clientSecret"`
	ClientCredentials string `json:"clientCredentials"` // Client credential secret name.

	// Filters
	CapacityPools  []string `json:"capacityPools"`
	NetappAccounts []string `json:"netappAccounts"`
	ResourceGroups []string `json:"resourceGroups"`
	VirtualNetwork string   `json:"virtualNetwork"`
	Subnet         string   `json:"subnet"`

	// Encryption: Map of NetApp accounts and customer keys.
	CustomerEncryptionKeys map[string]string `json:"customerEncryptionKeys"`
}

func NewANFInstance(
	torcCR *operatorV1.TridentOrchestrator, configuratorCR *operatorV1.TridentConfigurator,
	client confClients.ConfiguratorClientInterface,
) (*ANF, error) {
	if torcCR == nil {
		return nil, fmt.Errorf("empty torc CR")
	}

	if configuratorCR == nil {
		return nil, fmt.Errorf("empty ANF configurator CR")
	}

	if client == nil {
		return nil, fmt.Errorf("invalid client")
	}

	anfConfig := ANFConfig{}
	if err := json.Unmarshal(configuratorCR.Spec.Raw, &anfConfig); err != nil {
		return nil, err
	}

	Log().Debug("ANF Config: ", anfConfig)

	return &ANF{
		ANFConfig:        anfConfig,
		ConfClient:       client,
		AMIEnabled:       torcCR.Spec.CloudProvider == k8sclient.CloudProviderAzure,
		TBCNamePrefix:    configuratorCR.Name,
		TridentNamespace: torcCR.Spec.Namespace,
	}, nil
}

func (a *ANF) Validate() error {
	var err error

	clientConfig := api.ClientConfig{
		AzureAuthConfig: azclient.AzureAuthConfig{
			AADClientID:     a.ClientID,
			AADClientSecret: a.ClientSecret,
		},
		TenantID:          a.TenantID,
		SubscriptionID:    a.SubscriptionID,
		Location:          a.Location,
		StorageDriverName: config.AzureNASStorageDriverName,
		DebugTraceFlags:   map[string]bool{"method": true, "api": true},
		SDKTimeout:        api.DefaultSDKTimeout,
		MaxCacheAge:       api.DefaultMaxCacheAge,
	}

	if os.Getenv("AZURE_CLIENT_ID") != "" && os.Getenv("AZURE_TENANT_ID") != "" && os.Getenv("AZURE_FEDERATED_TOKEN_FILE") != "" && os.Getenv("AZURE_AUTHORITY_HOST") != "" {
		Log().Debug("Using Azure workload identity.")
		a.WorkloadIdentityEnabled = true
	} else if a.AMIEnabled {
		Log().Debug("Using Azure managed identity.")

		if a.ANFConfig.ClientSecret == "" && a.ANFConfig.ClientID == "" && os.Getenv("AZURE_CREDENTIAL_FILE") != "" {
			credFilePath := os.Getenv("AZURE_CREDENTIAL_FILE")
			Log().WithField("credFilePath", credFilePath).Debug("Using Azure credential config file.")

			credFile, err := os.ReadFile(credFilePath)
			if err != nil {
				return fmt.Errorf("error reading from azure config file: " + err.Error())
			}
			if err = json.Unmarshal(credFile, &clientConfig); err != nil {
				return fmt.Errorf("error parsing azureAuthConfig: " + err.Error())
			}

			// Set SubscriptionID and Location.
			a.ANFConfig.SubscriptionID = clientConfig.SubscriptionID
			a.ANFConfig.Location = clientConfig.Location
		}
	} else {
		a.ClientID, a.ClientSecret, err = a.ConfClient.GetANFSecrets(a.ClientCredentials)
		if err != nil {
			Log().Errorf("ANF secrets not provided, %v", err)
			return err
		}
		// Set ClientID and ClientSecret
		clientConfig.AADClientID = a.ClientID
		clientConfig.AADClientSecret = a.ClientSecret
	}

	client, err := api.NewDriver(clientConfig)
	if err != nil {
		return err
	}

	// Unit tests mock the API layer, so we only use the real API interface if it doesn't already exist.
	if a.AZClient == nil {
		a.AZClient = client
	}

	if err = a.AZClient.DiscoverAzureResources(context.TODO()); err != nil {
		return err
	}

	if err = a.populateAndValidateAZResources(); err != nil {
		return err
	}

	if a.AvailabilityZones, err = a.AZClient.AvailabilityZones(context.TODO()); err != nil {
		return err
	}

	return nil
}

func (a *ANF) Create() ([]string, error) {
	nfsBackendYAML, smbBackendYAML := a.buildTridentANFBackendTBC()
	nfsBackendName := getANFBackendName(a.TBCNamePrefix, sa.NFS)
	smbBackendName := getANFBackendName(a.TBCNamePrefix, sa.SMB)

	if err := a.ConfClient.CreateOrPatchObject(confClients.OBackend, nfsBackendName,
		a.TridentNamespace, nfsBackendYAML); err != nil {
		return []string{}, err
	}

	if err := a.ConfClient.CreateOrPatchObject(confClients.OBackend, smbBackendName,
		a.TridentNamespace, smbBackendYAML); err != nil {
		return []string{}, err
	}

	return []string{nfsBackendName, smbBackendName}, nil
}

func (a *ANF) CreateStorageClass() error {
	anfStorageClassMap := NewANFStorageClassMap()

	for _, cPool := range a.FilteredCapacityPoolMap {
		switch cPool.ServiceLevel {
		case api.ServiceLevelStandard:
			anfStorageClassMap.Add(ANFStorageClassHardwareStandard, api.ServiceLevelStandard, sa.NFS)
			anfStorageClassMap.Add(ANFStorageClassHardwareStandardSMB, api.ServiceLevelStandard, sa.SMB)
		case api.ServiceLevelPremium:
			anfStorageClassMap.Add(ANFStorageClassHardwarePremium, api.ServiceLevelPremium, sa.NFS)
			anfStorageClassMap.Add(ANFStorageClassHardwarePremiumSMB, api.ServiceLevelPremium, sa.SMB)
		case api.ServiceLevelUltra:
			anfStorageClassMap.Add(ANFStorageClassHardwareUltra, api.ServiceLevelUltra, sa.NFS)
			anfStorageClassMap.Add(ANFStorageClassHardwareUltraSMB, api.ServiceLevelUltra, sa.SMB)
		}
	}

	for _, sc := range anfStorageClassMap.SCMap {
		scYAML := getANFStorageClassYAML(sc, config.AzureNASStorageDriverName, a.TridentNamespace)
		err := a.ConfClient.CreateOrPatchObject(confClients.OStorageClass, sc.Name, "", scYAML)
		if err != nil {
			return err
		}
	}

	return nil
}

func (a *ANF) CreateSnapshotClass() error {
	anfSnapClassYAML := GetVolumeSnapshotClassYAML(NetAppSnapshotClassName)
	return a.ConfClient.CreateOrPatchObject(confClients.OSnapshotClass, NetAppSnapshotClassName,
		"", anfSnapClassYAML)
}

func (a *ANF) GetCloudProvider() string {
	if a.AMIEnabled {
		return k8sclient.CloudProviderAzure
	}
	return "None"
}

func (a *ANF) populateAndValidateAZResources() error {
	a.FilteredCapacityPoolMap = a.AZClient.FilteredCapacityPoolMap(context.TODO(), a.ResourceGroups,
		a.NetappAccounts, a.CapacityPools)
	if len(a.FilteredCapacityPoolMap) == 0 {
		return fmt.Errorf("no capacity pools discovered after filtering")
	}

	a.FilteredSubnetMap = a.AZClient.FilteredSubnetMap(context.TODO(), a.ResourceGroups, a.VirtualNetwork, a.Subnet)
	if len(a.FilteredSubnetMap) == 0 {
		return fmt.Errorf("no ANF subnets discovered after filtering")
	}

	return nil
}

func (a *ANF) buildTridentANFBackendTBC() (string, string) {
	var addPools bool
	if len(a.CapacityPools) > 0 {
		addPools = true
	}

	anfVPoolMap := NewANFVPoolMap(addPools)

	for cPoolFullName, cPool := range a.FilteredCapacityPoolMap {
		switch cPool.ServiceLevel {
		case api.ServiceLevelStandard:
			anfVPool := anfVPoolMap.Add(api.ServiceLevelStandard)
			anfVPool.AddCPool(cPoolFullName)
		case api.ServiceLevelPremium:
			anfVPool := anfVPoolMap.Add(api.ServiceLevelPremium)
			anfVPool.AddCPool(cPoolFullName)
		case api.ServiceLevelUltra:
			anfVPool := anfVPoolMap.Add(api.ServiceLevelUltra)
			anfVPool.AddCPool(cPoolFullName)
		}
	}

	return getANFTBCYaml(a, anfVPoolMap.GetYAMLs(sa.NFS), sa.NFS),
		getANFTBCYaml(a, anfVPoolMap.GetYAMLs(sa.SMB), sa.SMB)
}

func getANFBackendName(backendPrefix, nasType string) string {
	backendPrefix = strings.TrimSuffix(backendPrefix, "-configurator")
	return backendPrefix + "-" + nasType
}

// Backend VPool functions

type ANFVPool struct {
	ServiceLevel string
	CPools       []string
	AddPool      bool
}

func NewANFVPool(serviceLevel string, addPool bool) *ANFVPool {
	return &ANFVPool{ServiceLevel: serviceLevel, AddPool: addPool}
}

func (avp *ANFVPool) AddCPool(cPoolName string) {
	if avp.AddPool {
		avp.CPools = append(avp.CPools, cPoolName)
	}
}

func (avp *ANFVPool) GetYAML(nasType string) string {
	return getANFVPoolYAML(avp.ServiceLevel, nasType, strings.Join(avp.CPools, ","))
}

type ANFVPoolMap struct {
	VPoolMap map[string]*ANFVPool
	AddPools bool
}

func NewANFVPoolMap(addPools bool) *ANFVPoolMap {
	return &ANFVPoolMap{
		VPoolMap: make(map[string]*ANFVPool, MaxNumberOfANFServiceLevels),
		AddPools: addPools,
	}
}

func (avm *ANFVPoolMap) Add(serviceLevel string) *ANFVPool {
	avp, ok := avm.VPoolMap[serviceLevel]
	if ok {
		return avp
	}

	avp = NewANFVPool(serviceLevel, avm.AddPools)
	avm.VPoolMap[serviceLevel] = avp
	return avp
}

func (avm *ANFVPoolMap) GetYAMLs(nasType string) string {
	vPoolsYAML := ""
	for _, vPool := range avm.VPoolMap {
		vPoolsYAML += vPool.GetYAML(nasType)
	}

	return vPoolsYAML
}

// ANF Storage Class functions

type ANFStorageClass struct {
	Name         string
	ServiceLevel string
	NASType      string
}

type ANFStorageClassMap struct {
	SCMap map[string]ANFStorageClass
}

func NewANFStorageClassMap() *ANFStorageClassMap {
	return &ANFStorageClassMap{SCMap: make(map[string]ANFStorageClass, MaxNumberOfANFStorageClasses)}
}

func (asm *ANFStorageClassMap) Add(name, serviceLevel, nasType string) {
	_, ok := asm.SCMap[name]
	if ok {
		return
	}

	asm.SCMap[name] = ANFStorageClass{name, serviceLevel, nasType}
}
