// Copyright 2025 NetApp, Inc. All Rights Reserved.

package storage_drivers

import (
	"fmt"
	"sort"
	"strings"

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
func getFsxnTBCYaml(svm SVM, tridentNamespace, backendName, protocolType, tconfName string,
	scManagedTconf bool, tconfSpec map[string]interface{}, storageDriverName string) string {
	tbcYaml := FsxnTBCYaml
	tbcYaml = strings.ReplaceAll(tbcYaml, "{TBC_NAME}", backendName)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{NAMESPACE}", tridentNamespace)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{MANAGEMENT_LIF}", svm.ManagementLIF)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{FILE_SYSTEM_ID}", svm.FsxnID)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{SVM_NAME}", svm.SvmName)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{AWS_ARN}", svm.SecretARNName)
	tbcYaml = strings.ReplaceAll(tbcYaml, "{DRIVER_TYPE}", storageDriverName)

	if scManagedTconf {
		labels := fmt.Sprintf("labels:\n    trident.netapp.io/configurator: %s\n", tconfName)
		tbcYaml = strings.ReplaceAll(tbcYaml, "{LABELS}", labels)

		tbcYaml = replaceBoolIfPresent(tbcYaml, "{USE_REST}", tconfSpec, "useREST")
		tbcYaml = replaceBoolIfPresent(tbcYaml, "{AUTO_EXPORT_POLICY}", tconfSpec, "autoExportPolicy")

		tbcYaml = replaceStringIfPresent(tbcYaml, "{DATA_LIF}", tconfSpec, "dataLIF")
		tbcYaml = replaceStringIfPresent(tbcYaml, "{STORAGE_PREFIX}", tconfSpec, "storagePrefix")
		tbcYaml = replaceStringIfPresent(tbcYaml, "{LIMIT_VOLUME_SIZE}", tconfSpec, "limitVolumeSize")
		tbcYaml = replaceStringIfPresent(tbcYaml, "{CLIENT_PRIVATE_KEY}", tconfSpec, "clientPrivateKey")
		tbcYaml = replaceStringIfPresent(tbcYaml, "{CLIENT_CERTIFICATE}", tconfSpec, "clientCertificate")
		tbcYaml = replaceStringIfPresent(tbcYaml, "{TRUSTED_CA_CERTIFICATE}", tconfSpec, "trustedCACertificate")
		tbcYaml = replaceStringIfPresent(tbcYaml, "{NFS_MOUNT_OPTIONS}", tconfSpec, "nfsMountOptions")
		tbcYaml = replaceStringIfPresent(tbcYaml, "{SMB_SHARE}", tconfSpec, "smbShare")
		tbcYaml = replaceStringIfPresent(tbcYaml, "{QTREES_PER_FLEXVOL}", tconfSpec, "qtreesPerFlexvol")
		tbcYaml = replaceStringIfPresent(tbcYaml, "{LUNS_PER_FLEXVOL}", tconfSpec, "lunsPerFlexvol")

		tbcYaml = replaceStringIfPresent(tbcYaml, "{API_REGION}", tconfSpec, "apiRegion")
		tbcYaml = replaceStringIfPresent(tbcYaml, "{API_KEY}", tconfSpec, "apiKey")
		tbcYaml = replaceStringIfPresent(tbcYaml, "{SECRET_KEY}", tconfSpec, "secretKey")

		tbcYaml = replaceArrayIfPresent(tbcYaml, "{AUTO_EXPORT_CIDRS}", tconfSpec, "autoExportCIDRs")

		tbcYaml = replaceLabelsIfPresent(tbcYaml, "{BACKEND_LABELS}", tconfSpec, "labels")

		tbcYaml = replaceDefaultsSection(tbcYaml, tconfSpec)
	} else {
		tbcYaml = strings.ReplaceAll(tbcYaml, "{LABELS}", "")
		tbcYaml = setEmptyOptionalFields(tbcYaml)
	}

	if protocolType == sa.NFS {
		tbcYaml = strings.ReplaceAll(tbcYaml, "{NAS_TYPE}", strings.Join([]string{"nasType:", protocolType}, " "))
	} else if protocolType == sa.ISCSI {
		tbcYaml = strings.ReplaceAll(tbcYaml, "{NAS_TYPE}", "")
	}

	return tbcYaml
}

// replaceStringIfPresent handles string type fields
func replaceStringIfPresent(yaml string, placeholder string, spec map[string]interface{}, key string) string {
	if value, ok := spec[key]; ok && value != nil {
		var strValue string
		switch v := value.(type) {
		case string:
			strValue = v
		default:
			strValue = fmt.Sprintf("%v", v)
		}
		if strValue != "" {
			yamlLine := fmt.Sprintf("%s: %s", key, strValue)
			return strings.ReplaceAll(yaml, placeholder, yamlLine)
		}
	}
	return strings.ReplaceAll(yaml, placeholder, "")
}

// replaceBoolIfPresent handles boolean type fields
func replaceBoolIfPresent(yaml string, placeholder string, spec map[string]interface{}, key string) string {
	if value, ok := spec[key]; ok && value != nil {
		var boolValue bool
		switch v := value.(type) {
		case bool:
			boolValue = v
		case string:
			boolValue = strings.EqualFold(v, "true")
		default:
			return strings.ReplaceAll(yaml, placeholder, "")
		}
		yamlLine := fmt.Sprintf("%s: %t", key, boolValue)
		return strings.ReplaceAll(yaml, placeholder, yamlLine)
	}
	return strings.ReplaceAll(yaml, placeholder, "")
}

// replaceLabelsIfPresent handles map[string]string type fields for labels
func replaceLabelsIfPresent(yaml string, placeholder string, spec map[string]interface{}, key string) string {
	value, ok := spec[key]
	if !ok || value == nil {
		return strings.ReplaceAll(yaml, placeholder, "")
	}

	var labelsYAML string

	switch v := value.(type) {
	case map[string]interface{}:
		if len(v) == 0 {
			return strings.ReplaceAll(yaml, placeholder, "")
		}
		labelsYAML = "labels:\n"
		for labelKey, labelValue := range v {
			labelsYAML += fmt.Sprintf("    %s: \"%v\"\n", labelKey, labelValue)
		}
	case map[string]string:
		if len(v) == 0 {
			return strings.ReplaceAll(yaml, placeholder, "")
		}
		labelsYAML = "labels:\n"
		for labelKey, labelValue := range v {
			labelsYAML += fmt.Sprintf("    %s: \"%s\"\n", labelKey, labelValue)
		}
	default:
		return strings.ReplaceAll(yaml, placeholder, "")
	}

	return strings.ReplaceAll(yaml, placeholder, labelsYAML)
}

// replaceArrayIfPresent handles array/slice type fields ([]string in OntapStorageDriverConfig)
func replaceArrayIfPresent(yaml string, placeholder string, spec map[string]interface{}, key string) string {
	value, ok := spec[key]
	if !ok || value == nil {
		return strings.ReplaceAll(yaml, placeholder, "")
	}

	var strValues []string

	switch v := value.(type) {
	case []interface{}:
		// JSON unmarshalling produces []interface{}
		if len(v) == 0 {
			return strings.ReplaceAll(yaml, placeholder, "")
		}
		strValues = make([]string, len(v))
		for i, elem := range v {
			strValues[i] = fmt.Sprintf("%v", elem)
		}
	case []string:
		// Direct []string type
		if len(v) == 0 {
			return strings.ReplaceAll(yaml, placeholder, "")
		}
		strValues = v
	default:
		// Unsupported type, remove placeholder
		return strings.ReplaceAll(yaml, placeholder, "")
	}

	// Format as YAML key with inline array
	arrayStr := fmt.Sprintf("%s: [%s]", key, strings.Join(strValues, ", "))
	return strings.ReplaceAll(yaml, placeholder, arrayStr)
}

// Helper function to construct defaults section
// Parameters per FSx documentation: https://docs.netapp.com/us-en/trident/trident-use/trident-fsx-storage-backend.html
func replaceDefaultsSection(yaml string, spec map[string]interface{}) string {
	defaultParams := []string{
		"spaceAllocation", "spaceReserve", "snapshotPolicy", "snapshotReserve",
		"splitOnClone", "encryption", "LUKSEncryption", "tieringPolicy",
		"unixPermissions", "securityStyle", "qosPolicy", "adaptiveQosPolicy",
	}

	defaultsYAML := ""
	foundDefaults := false

	for _, param := range defaultParams {
		if value, ok := spec[param]; ok && value != nil {
			strValue := formatValueForYAML(value)
			if strValue != "" {
				if !foundDefaults {
					defaultsYAML = "defaults:\n"
					foundDefaults = true
				}
				defaultsYAML += fmt.Sprintf("    %s: \"%s\"\n", param, strValue)
			}
		}
	}

	if foundDefaults {
		return strings.ReplaceAll(yaml, "{DEFAULTS}", defaultsYAML)
	}

	return strings.ReplaceAll(yaml, "{DEFAULTS}", "")
}

// formatValueForYAML converts various types to their string representation for YAML
func formatValueForYAML(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", v)
	case int32:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		// Check if it's a whole number
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%v", v)
	case float32:
		if v == float32(int32(v)) {
			return fmt.Sprintf("%d", int32(v))
		}
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Helper function to clear all optional field placeholders when not in scManagedTconf mode
func setEmptyOptionalFields(yaml string) string {
	// List of all optional placeholders to clear (per FSx documentation)
	placeholders := []string{
		"{DATA_LIF}", "{STORAGE_PREFIX}", "{LIMIT_VOLUME_SIZE}",
		"{CLIENT_PRIVATE_KEY}", "{CLIENT_CERTIFICATE}", "{TRUSTED_CA_CERTIFICATE}",
		"{API_REGION}", "{API_KEY}", "{SECRET_KEY}",
		"{USE_REST}", "{QTREES_PER_FLEXVOL}", "{LUNS_PER_FLEXVOL}",
		"{NFS_MOUNT_OPTIONS}", "{SMB_SHARE}",
		"{AUTO_EXPORT_POLICY}", "{AUTO_EXPORT_CIDRS}",
		"{BACKEND_LABELS}", "{DEFAULTS}",
	}

	for _, placeholder := range placeholders {
		yaml = strings.ReplaceAll(yaml, placeholder, "")
	}

	return yaml
}

// FsxnTBCYaml is a template for the FsxN driver Trident backend config YAML
// Parameters per FSx documentation: https://docs.netapp.com/us-en/trident/trident-use/trident-fsx-storage-backend.html
const FsxnTBCYaml = `---
apiVersion: trident.netapp.io/v1
kind: TridentBackendConfig
metadata:
  name: {TBC_NAME}
  namespace: {NAMESPACE}
  {LABELS}
spec:
  version: 1
  storageDriverName: {DRIVER_TYPE}
  {NAS_TYPE}
  managementLIF: {MANAGEMENT_LIF}
  {DATA_LIF}
  svm: {SVM_NAME}
  credentials:
    name: {AWS_ARN}
    type: awsarn
  aws:
    fsxFilesystemID: {FILE_SYSTEM_ID}
    {API_REGION}
    {API_KEY}
    {SECRET_KEY}
  {STORAGE_PREFIX}
  {LIMIT_VOLUME_SIZE}
  {CLIENT_PRIVATE_KEY}
  {CLIENT_CERTIFICATE}
  {TRUSTED_CA_CERTIFICATE}
  {USE_REST}
  {QTREES_PER_FLEXVOL}
  {LUNS_PER_FLEXVOL}
  {NFS_MOUNT_OPTIONS}
  {SMB_SHARE}
  {AUTO_EXPORT_POLICY}
  {AUTO_EXPORT_CIDRS}
  {BACKEND_LABELS}
  {DEFAULTS}
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
