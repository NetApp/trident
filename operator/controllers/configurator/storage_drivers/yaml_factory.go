// Copyright 2025 NetApp, Inc. All Rights Reserved.

package storage_drivers

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/pkg/yaml"
	sa "github.com/netapp/trident/storage_attribute"
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
	tbcYaml = strings.ReplaceAll(tbcYaml, "{NETWORK_FEATURES}", NetworkFeaturesStandard)
	tbcYaml = yaml.ReplaceMultilineTag(tbcYaml, "CUSTOMER_ENCRYPTION_KEYS", constructEncryptionKeys(anf.CustomerEncryptionKeys))
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
  networkFeatures: {NETWORK_FEATURES}
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

func getANFStorageClassYAML(sc ANFStorageClass, backendType, namespace, nconnect string) string {
	scYAML := anfStorageClassTemplate

	scYAML = strings.ReplaceAll(scYAML, "{NAME}", sc.Name)
	scYAML = strings.ReplaceAll(scYAML, "{BACKEND_TYPE}", backendType)
	scYAML = strings.ReplaceAll(scYAML, "{SERVICE_LEVEL}", sc.ServiceLevel)
	scYAML = strings.ReplaceAll(scYAML, "{NAS_TYPE}", sc.NASType)

	if sc.NASType == sa.SMB {
		scYAML = strings.ReplaceAll(scYAML, "{AAD_SECRET}", constructAADSecret(namespace))
		scYAML = strings.ReplaceAll(scYAML, "{MOUNT_OPTIONS}", "")
	} else {
		scYAML = strings.ReplaceAll(scYAML, "{AAD_SECRET}", "")
		mountOptions := map[string]string{"nconnect": nconnect}
		scYAML = strings.ReplaceAll(scYAML, "{MOUNT_OPTIONS}", constructMountOptions(mountOptions))
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
{MOUNT_OPTIONS}
`

func constructAADSecret(namespace string) string {
	//nolint:gosec
	aadSecret := "csi.storage.k8s.io/node-stage-secret-name: 'smbcreds'\n"
	aadSecret += fmt.Sprintf("  csi.storage.k8s.io/node-stage-secret-namespace: '%s'", namespace)
	return aadSecret
}

// constructMountOptions constructs the mount options for the storage class
func constructMountOptions(mountOptions map[string]string) string {
	if len(mountOptions) == 0 {
		return ""
	}

	keys := make([]string, 0, len(mountOptions))
	for key := range mountOptions {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	mountOptionsYAML := "mountOptions:\n"
	for _, key := range keys {
		mountOptionsYAML += fmt.Sprintf("  - %s=%s\n", key, mountOptions[key])
	}

	return mountOptionsYAML
}

// GetVolumeSnapshotClassYAML returns the VolumeSnapshotClass YAML
func GetVolumeSnapshotClassYAML(name string) string {
	vscYAML := volumeSnapshotClassTemplate

	vscYAML = strings.ReplaceAll(vscYAML, "{NAME}", name)

	return vscYAML
}

// volumeSnapshotClassTemplate is a template for the VolumeSnapshotClass YAML
const volumeSnapshotClassTemplate = `---
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: {NAME}
driver: csi.trident.netapp.io
deletionPolicy: Delete
`

// getFsxnTBCYaml returns the FsxN Trident backend config YAML
func getFsxnTBCYaml(svm SVM, tridentNamespace, backendName, protocolType string) string {
	tbcYaml := FsxnTBCYaml
	tbcYaml = strings.ReplaceAll(tbcYaml, "{TBC_NAME}", backendName)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{NAMESPACE}", tridentNamespace)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{MANAGEMENT_LIF}", svm.ManagementLIF)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{FILE_SYSTEM_ID}", svm.FsxnID)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{SVM_NAME}", svm.SvmName)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{AWS_ARN}", svm.SecretARNName)
	if protocolType == sa.NFS {
		tbcYaml = strings.ReplaceAll(tbcYaml, "{DRIVER_TYPE}", config.OntapNASStorageDriverName)
		tbcYaml = strings.ReplaceAll(tbcYaml, "{NAS_TYPE}", strings.Join([]string{"nasType:", protocolType}, " "))
	} else if protocolType == sa.ISCSI {
		tbcYaml = strings.ReplaceAll(tbcYaml, "{DRIVER_TYPE}", config.OntapSANStorageDriverName)
		tbcYaml = strings.ReplaceAll(tbcYaml, "{NAS_TYPE}", "")
	}
	return tbcYaml
}

// FsxnTBCYaml is a template for the FsxN san driver Trident backend config YAML
const FsxnTBCYaml = `---
apiVersion: trident.netapp.io/v1
kind: TridentBackendConfig
metadata:
 name: {TBC_NAME}
 namespace: {NAMESPACE}
spec:
 version: 1
 storageDriverName: {DRIVER_TYPE}
 {NAS_TYPE}
 managementLIF: {MANAGEMENT_LIF}
 aws:
   fsxFileSystemID: {FILE_SYSTEM_ID}
 svm: {SVM_NAME}
 credentials:
   name: {AWS_ARN}
   type: awsarn
`

// getFsxnStorageClassYaml returns the Fsxn storage class YAML
func getFsxnStorageClassYaml(name, backendType string) string {
	scYaml := fsxnStorageClassTemplate
	scYaml = strings.ReplaceAll(scYaml, "{NAME}", name)
	scYaml = strings.ReplaceAll(scYaml, "{BACKEND_TYPE}", backendType)
	return scYaml
}

// fsxnStorageClassTemplate is a template for the Fsxn storage class YAML
const fsxnStorageClassTemplate = `---
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
 name: {NAME}
provisioner: csi.trident.netapp.io
parameters:
 backendType: {BACKEND_TYPE}
volumeBindingMode: Immediate
allowVolumeExpansion: true
`
