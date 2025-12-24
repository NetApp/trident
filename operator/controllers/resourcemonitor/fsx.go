// Copyright 2025 NetApp, Inc. All Rights Reserved.

package resourcemonitor

import (
	"encoding/json"
	"fmt"
	"strings"

	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/runtime"

	netappv1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	"github.com/netapp/trident/utils/errors"
)

const (
	// Constants for FSxN configuration
	FSxFilesystemIDParam                 = "fsxFilesystemID"
	AdditionalFsxNFileSystemIDAnnotation = "trident.netapp.io/additionalFsxNFileSystemID"

	// Backend naming pattern for FSxN
	BackendNamePattern = "trident-%s-%s" // filesystemID, protocol
)

// FsxStorageDriverHandler implements StorageDriverHandler for FSxN
type FsxStorageDriverHandler struct{}

// NewFsxStorageDriverHandler creates a new FSxN driver handler
func NewFsxStorageDriverHandler() StorageDriverHandler {
	return &FsxStorageDriverHandler{}
}

// ShouldManageStorageClass determines if a StorageClass should be managed by FSxN handler
func (h *FsxStorageDriverHandler) ShouldManageStorageClass(sc *storagev1.StorageClass) bool {
	// Must have parameters
	if sc.Parameters == nil {
		return false
	}

	// Must have fsxFilesystemID parameter (FSxN specific requirement)
	fsxID, hasFsxID := sc.Parameters[FSxFilesystemIDParam]
	if !hasFsxID || fsxID == "" {
		return false
	}

	// Must have storageDriverName
	driverName, hasDriver := sc.Parameters[StorageDriverNameParam]
	if !hasDriver || driverName == "" {
		return false
	}

	// Must have credentialsName
	credentials, hasCreds := sc.Parameters[CredentialsNameParam]
	if !hasCreds || credentials == "" {
		return false
	}

	return true
}

// ValidateStorageClass validates the StorageClass configuration for FSxN
func (h *FsxStorageDriverHandler) ValidateStorageClass(sc *storagev1.StorageClass) error {
	if sc.Parameters == nil {
		return errors.UnsupportedConfigError("StorageClass parameters cannot be nil")
	}

	// Validate required parameters
	requiredParams := []string{FSxFilesystemIDParam, StorageDriverNameParam, CredentialsNameParam}
	for _, param := range requiredParams {
		if val, ok := sc.Parameters[param]; !ok || val == "" {
			return errors.UnsupportedConfigError("required parameter %s is missing or empty", param)
		}
	}

	return nil
}

// HasRelevantChanges checks if there are relevant changes between old and new StorageClass
func (h *FsxStorageDriverHandler) HasRelevantChanges(oldSC, newSC *storagev1.StorageClass) bool {
	// Check if annotations changed (specifically additionalFsxNFileSystemID)
	// When this annotation changes, we automatically update the additionalStoragePools annotation
	oldAdditional := oldSC.Annotations[AdditionalFsxNFileSystemIDAnnotation]
	newAdditional := newSC.Annotations[AdditionalFsxNFileSystemIDAnnotation]
	if oldAdditional != newAdditional {
		return true
	}

	return false
}

// BuildAdditionalStoragePoolsValue constructs the additionalStoragePools annotation value for FSxN
// Format: "backend1:pool1;backend2:pool2" where each backend corresponds to a filesystem
func (h *FsxStorageDriverHandler) BuildAdditionalStoragePoolsValue(sc *storagev1.StorageClass) (string, error) {
	// Get the storage driver to determine protocol
	driverName, ok := sc.Parameters[StorageDriverNameParam]
	if !ok || driverName == "" {
		return "", errors.UnsupportedConfigError("required parameter %s is missing or empty", StorageDriverNameParam)
	}
	protocol := getProtocolFromDriver(driverName)

	var backendPools []string

	// Start with the primary filesystem
	primaryFsxID := sc.Parameters[FSxFilesystemIDParam]
	if primaryFsxID != "" {
		backendName := buildBackendName(primaryFsxID, protocol)
		// Use ".*" as a regex pattern to match all pools for this backend
		backendPools = append(backendPools, fmt.Sprintf("%s:.*", backendName))
	}

	// Handle additional FSx filesystem IDs from annotations
	if additionalIDs, ok := sc.Annotations[AdditionalFsxNFileSystemIDAnnotation]; ok && additionalIDs != "" {
		var idList []string
		if err := json.Unmarshal([]byte(additionalIDs), &idList); err != nil {
			// Try comma-separated format as fallback
			idList = strings.Split(additionalIDs, ",")
			for i := range idList {
				idList[i] = strings.TrimSpace(idList[i])
			}
		}

		// Add additional FSx filesystems
		for _, fsxID := range idList {
			if fsxID != "" {
				backendName := buildBackendName(fsxID, protocol)
				backendPools = append(backendPools, fmt.Sprintf("%s:.*", backendName))
			}
		}
	}

	// Join all backend:pool pairs with semicolons
	if len(backendPools) == 0 {
		return "", nil
	}

	return strings.Join(backendPools, ";"), nil
}

// BuildTridentConfiguratorSpec builds a TridentConfigurator spec from StorageClass for FSxN
func (h *FsxStorageDriverHandler) BuildTridentConfiguratorSpec(sc *storagev1.StorageClass) (*netappv1.TridentConfiguratorSpec, error) {
	// Build the spec as a map following TridentConfigurator format
	specMap := make(map[string]interface{})

	// Set storageDriverName
	storageDriverName := sc.Parameters[StorageDriverNameParam]
	specMap["storageDriverName"] = storageDriverName

	// Determine protocols based on storage driver
	var protocols []string
	switch storageDriverName {
	case "ontap-nas":
		protocols = []string{"nfs"}
	case "ontap-san":
		protocols = []string{"iscsi"}
	default:
		return nil, fmt.Errorf("unsupported storage driver: %s", storageDriverName)
	}

	// Build SVMs array for FSxN
	svms := []map[string]interface{}{}

	// Primary FSx filesystem
	primarySVM := map[string]interface{}{
		"fsxnID":    sc.Parameters[FSxFilesystemIDParam],
		"authType":  "awsarn",
		"protocols": protocols,
	}

	// Add svmName if provided
	if svmName, ok := sc.Parameters["svmName"]; ok && svmName != "" {
		primarySVM["svmName"] = svmName
	}

	// Set credentials (AWS Secrets Manager ARN)
	if credentialsName, ok := sc.Parameters[CredentialsNameParam]; ok && credentialsName != "" {
		specMap["credentialsName"] = credentialsName
	}

	svms = append(svms, primarySVM)

	// Handle additional FSx filesystem IDs from annotations
	if additionalIDs, ok := sc.Annotations[AdditionalFsxNFileSystemIDAnnotation]; ok && additionalIDs != "" {
		var idList []string
		if err := json.Unmarshal([]byte(additionalIDs), &idList); err != nil {
			// Try comma-separated format as fallback
			idList = strings.Split(additionalIDs, ",")
			for i := range idList {
				idList[i] = strings.TrimSpace(idList[i])
			}
		}

		// Add additional FSx filesystems as additional SVMs
		for _, fsxID := range idList {
			if fsxID != "" {
				additionalSVM := map[string]interface{}{
					"fsxnID":    fsxID,
					"authType":  "awsarn",
					"protocols": protocols,
				}
				if svmName, ok := sc.Parameters["svmName"]; ok && svmName != "" {
					additionalSVM["svmName"] = svmName
				}
				svms = append(svms, additionalSVM)
			}
		}
	}

	specMap["svms"] = svms

	// Extract additional backend parameters from StorageClass
	// These parameters will be passed through to the TBC
	backendParams := []string{
		"debug", "storagePrefix", "limitVolumeSize",

		"clientPrivateKey", "clientCertificate", "trustedCACertificate",

		"apiRegion", "apiKey", "secretKey",

		"aggregate", "igroupName", "usageHeartbeat", "useREST",

		"qtreePruneFlexvolsPeriod", "qtreeQuotaResizePeriod",
		"qtreesPerFlexvol", "emptyFlexvolDeferredDeletePeriod",

		"lunsPerFlexvol",

		"flexgroupAggregateList",

		"nfsMountOptions", "limitAggregateUsage", "limitVolumePoolSize",
		"denyNewVolumePools", "cloneSplitDelay",

		"autoExportPolicy", "autoExportCIDRs",

		"useCHAP", "chapUsername", "chapInitiatorSecret",
		"chapTargetUsername", "chapTargetInitiatorSecret",

		"replicationPolicy", "replicationSchedule",

		"size", "nameTemplate", "spaceAllocation", "spaceReserve",
		"snapshotPolicy", "snapshotReserve", "snapshotDir", "unixPermissions",
		"exportPolicy", "securityStyle", "splitOnClone", "fileSystemType",
		"encryption", "LUKSEncryption", "mirroring", "tieringPolicy",
		"qosPolicy", "adaptiveQosPolicy", "formatOptions", "skipRecoveryQueue",
		"adAdminUser",
	}

	// Copy all backend parameters from StorageClass to TConf spec
	for _, param := range backendParams {
		if value, ok := sc.Parameters[param]; ok && value != "" {
			specMap[param] = value
		}
	}

	// Marshal to JSON
	specBytes, err := json.Marshal(specMap)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal TridentConfigurator spec: %v", err)
	}

	return &netappv1.TridentConfiguratorSpec{
		RawExtension: runtime.RawExtension{
			Raw: specBytes,
		},
	}, nil
}

// getProtocolFromDriver returns the protocol type from the storage driver name
func getProtocolFromDriver(driverName string) string {
	switch driverName {
	case "ontap-nas":
		return "nfs"
	case "ontap-san":
		return "iscsi"
	default:
		return "nfs" // default to nfs
	}
}

// buildBackendName generates a backend name using the pattern trident-{filesystemID}-{protocol}
func buildBackendName(filesystemID, protocol string) string {
	return fmt.Sprintf(BackendNamePattern, filesystemID, protocol)
}
