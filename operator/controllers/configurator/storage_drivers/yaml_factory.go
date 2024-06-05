// Copyright 2024 NetApp, Inc. All Rights Reserved.

package storage_drivers

import (
	"fmt"
	"strings"

	sa "github.com/netapp/trident/storage_attribute"

	"github.com/netapp/trident/utils"
)

func getANFTBCYaml(anf *ANF, vPools, nasType string) string {
	tbcYaml := ANFTBCYaml

	tbcYaml = strings.ReplaceAll(tbcYaml, "{TBC_NAME}", getANFBackendName(anf.TBCNamePrefix, nasType))
	tbcYaml = strings.ReplaceAll(tbcYaml, "{NAMESPACE}", anf.TridentNamespace)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{SUBSCRIPTION_ID}", anf.SubscriptionID)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{TENANT_ID}", anf.TenantID)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{LOCATION}", anf.Location)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{RESOURCE_GROUPS}", strings.Join(anf.ResourceGroups, ","))
	tbcYaml = strings.ReplaceAll(tbcYaml, "{NETAPP_ACCOUNTS}", strings.Join(anf.NetappAccounts, ","))
	tbcYaml = strings.ReplaceAll(tbcYaml, "{VIRTUAL_NETWORK}", anf.VirtualNetwork)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{SUBNET}", anf.Subnet)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{NAS_TYPE}", nasType)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{V_POOLS}", vPools)
	tbcYaml = utils.ReplaceMultilineYAMLTag(tbcYaml, "CUSTOMER_ENCRYPTION_KEYS", constructEncryptionKeys(anf.CustomerEncryptionKeys))
	tbcYaml = strings.ReplaceAll(tbcYaml, "{SUPPORTED_TOPOLOGIES}",
		constructANFSupportedTopologies(anf.Location, anf.AvailabilityZones))

	if !anf.AMIEnabled && !anf.WorkloadIdentityEnabled {
		tbcYaml = strings.ReplaceAll(tbcYaml, "{CLIENT_CREDENTIALS}", constructClientCredentials(anf.ClientCredentials))
	} else {
		tbcYaml = strings.ReplaceAll(tbcYaml, "{CLIENT_CREDENTIALS}", "")
	}

	return tbcYaml
}

const ANFTBCYaml = `---
apiVersion: trident.netapp.io/v1
kind: TridentBackendConfig
metadata:
  name: {TBC_NAME}
  namespace: {NAMESPACE}
spec:
  version: 1
  storageDriverName: azure-netapp-files
  nasType: {NAS_TYPE}
  resourceGroups: [{RESOURCE_GROUPS}]
  netappAccounts: [{NETAPP_ACCOUNTS}]
  virtualNetwork: {VIRTUAL_NETWORK}
  subnet: {SUBNET}
  subscriptionID: {SUBSCRIPTION_ID}
  tenantID: {TENANT_ID}
  location: {LOCATION}
  {CLIENT_CREDENTIALS}
  debugTraceFlags:
    discovery: true
    method: true
    api: true
  {CUSTOMER_ENCRYPTION_KEYS}
  {SUPPORTED_TOPOLOGIES}
  storage:
  {V_POOLS}
`

func constructEncryptionKeys(m map[string]string) (encryptionKeys string) {
	if len(m) == 0 {
		return
	}

	encryptionKeys += "customerEncryptionKeys:\n"
	for key, value := range m {
		encryptionKeys += fmt.Sprintf("  %s: %s\n", key, value)
	}

	return encryptionKeys
}

func getANFVPoolYAML(serviceLevel, nasType, cPools string) string {
	anfVPool := ANFVPoolYAML

	anfVPool = strings.ReplaceAll(anfVPool, "{SERVICE_LEVEL}", serviceLevel)
	anfVPool = strings.ReplaceAll(anfVPool, "{NAS_TYPE}", nasType)
	anfVPool = strings.ReplaceAll(anfVPool, "{CAPACITY_POOLS}", cPools)

	return anfVPool
}

const ANFVPoolYAML = `
  - serviceLevel: {SERVICE_LEVEL}
    capacityPools: [{CAPACITY_POOLS}]
    labels:
      serviceLevel: {SERVICE_LEVEL}
      nasType: {NAS_TYPE}
`

func constructANFSupportedTopologies(region string, zones []string) string {
	if len(zones) == 0 {
		return ""
	}

	supportedTopologiesYAML := "supportedTopologies:\n"
	for _, zone := range zones {
		if !strings.HasPrefix(zone, region+"-") {
			zone = region + "-" + zone
		}
		supportedTopologiesYAML += fmt.Sprintf("  - topology.kubernetes.io/region: %s\n", region)
		supportedTopologiesYAML += fmt.Sprintf("    topology.kubernetes.io/zone: %s\n", zone)
	}
	return supportedTopologiesYAML
}

func constructClientCredentials(clientCred string) string {
	cred := "credentials:\n"
	cred += fmt.Sprintf("    name: %s", clientCred)
	return cred
}

func getANFStorageClassYAML(sc ANFStorageClass, backendType, namespace string) string {
	scYAML := anfStorageClassTemplate

	scYAML = strings.ReplaceAll(scYAML, "{NAME}", sc.Name)
	scYAML = strings.ReplaceAll(scYAML, "{BACKEND_TYPE}", backendType)
	scYAML = strings.ReplaceAll(scYAML, "{SERVICE_LEVEL}", sc.ServiceLevel)
	scYAML = strings.ReplaceAll(scYAML, "{NAS_TYPE}", sc.NASType)

	if sc.NASType == sa.SMB {
		scYAML = strings.ReplaceAll(scYAML, "{AAD_SECRET}", constructAADSecret(namespace))
	} else {
		scYAML = strings.ReplaceAll(scYAML, "{AAD_SECRET}", "")
	}

	return scYAML
}

const anfStorageClassTemplate = `---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: {NAME}
provisioner: csi.trident.netapp.io
parameters:
  backendType: {BACKEND_TYPE}
  selector: serviceLevel={SERVICE_LEVEL};nasType={NAS_TYPE}
  {AAD_SECRET}
volumeBindingMode: Immediate
allowVolumeExpansion: true
`

func constructAADSecret(namespace string) string {
	//nolint:gosec
	aadSecret := "csi.storage.k8s.io/node-stage-secret-name: 'smbcreds'\n"
	aadSecret += fmt.Sprintf("  csi.storage.k8s.io/node-stage-secret-namespace: '%s'", namespace)
	return aadSecret
}

func GetVolumeSnapshotClassYAML(name string) string {
	vscYAML := volumeSnapshotClassTemplate

	vscYAML = strings.ReplaceAll(vscYAML, "{NAME}", name)

	return vscYAML
}

const volumeSnapshotClassTemplate = `---
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: {NAME}
driver: csi.trident.netapp.io
deletionPolicy: Delete
`
