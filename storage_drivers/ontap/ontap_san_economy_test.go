// Copyright 2020 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

// ToStringPointer takes a string and returns a string pointer
func ToStringPointer(s string) *string {
	return &s
}

func NewTestLUNHelper(storagePrefix string, context tridentconfig.DriverContext) *LUNHelper {
	commonConfigJSON := fmt.Sprintf(`
{   
    "managementLIF":     "10.0.207.8",
    "dataLIF":           "10.0.207.7",
    "svm":               "iscsi_vs",
    "aggregate":         "aggr1",
    "username":          "admin",
    "password":          "password",
    "storageDriverName": "ontap-san-economy",
    "storagePrefix":     "%v",
    "debugTraceFlags":   {"method": true, "api": true},
    "version":1
}
`, storagePrefix)
	// parse commonConfigJSON into a CommonStorageDriverConfig object
	commonConfig, err := drivers.ValidateCommonSettings(commonConfigJSON)
	if err != nil {
		log.Errorf("could not decode JSON configuration: %v", err)
		return nil
	}
	config := &drivers.OntapStorageDriverConfig{}
	config.CommonStorageDriverConfig = commonConfig
	helper := NewLUNHelper(*config, context)
	return helper
}

func TestSnapshotNames_DockerContext(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)

	snapName1 := helper.getInternalSnapshotName("snapshot-123")
	assert.Equal(t, "_snapshot_snapshot_123", snapName1, "Strings not equal")

	snapName2 := helper.getInternalSnapshotName("snapshot")
	assert.Equal(t, "_snapshot_snapshot", snapName2, "Strings not equal")

	snapName3 := helper.getInternalSnapshotName("_snapshot")
	assert.Equal(t, "_snapshot__snapshot", snapName3, "Strings not equal")

	snapName4 := helper.getInternalSnapshotName("_____snapshot")
	assert.Equal(t, "_snapshot______snapshot", snapName4, "Strings not equal")

	k8sSnapName1 := helper.getInternalSnapshotName("snapshot-0bf1ec69_da4b_11e9_bd10_000c29e763d8")
	assert.Equal(t, "_snapshot_snapshot_0bf1ec69_da4b_11e9_bd10_000c29e763d8", k8sSnapName1, "Strings not equal")
}

func TestSnapshotNames_KubernetesContext(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextKubernetes)

	k8sSnapName1 := helper.getInternalSnapshotName("snapshot-0bf1ec69_da4b_11e9_bd10_000c29e763d8")
	assert.Equal(t, "_snapshot_snapshot_0bf1ec69_da4b_11e9_bd10_000c29e763d8", k8sSnapName1, "Strings not equal")

	k8sSnapName2 := helper.getInternalSnapshotName("mySnap")
	assert.Equal(t, "_snapshot_mySnap", k8sSnapName2, "Strings not equal")
}

func TestHelperGetters(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)

	snapPathPattern := helper.GetSnapPathPattern("my-Bucket")
	assert.Equal(t, "/vol/my_Bucket/storagePrefix_*_snapshot_*", snapPathPattern, "Strings not equal")

	snapPathPatternForVolume := helper.GetSnapPathPatternForVolume("my-Vol")
	assert.Equal(t, "/vol/*/storagePrefix_my_Vol_snapshot_*", snapPathPatternForVolume, "Strings not equal")

	snapPath := helper.GetSnapPath("my-Bucket", "storagePrefix_my-Lun", "snap-1")
	assert.Equal(t, "/vol/my_Bucket/storagePrefix_my_Lun_snapshot_snap_1", snapPath, "Strings not equal")

	snapName1 := helper.GetSnapshotName("my-Lun", "my-Snapshot")
	assert.Equal(t, "storagePrefix_my_Lun_snapshot_my_Snapshot", snapName1, "Strings not equal")

	snapName2 := helper.GetSnapshotName("my-Lun", "snapshot-123")
	assert.Equal(t, "storagePrefix_my_Lun_snapshot_snapshot_123", snapName2, "Strings not equal")

	internalSnapName := helper.GetInternalSnapshotName("storagePrefix_my-Lun", "my-Snapshot")
	assert.Equal(t, "storagePrefix_my_Lun_snapshot_my_Snapshot", internalSnapName, "Strings not equal")

	internalVolName := helper.GetInternalVolumeName("my-Lun")
	assert.Equal(t, "storagePrefix_my_Lun", internalVolName, "Strings not equal")

	lunPath := helper.GetLUNPath("my-Bucket", "my-Lun")
	assert.Equal(t, "/vol/my_Bucket/storagePrefix_my_Lun", lunPath, "Strings not equal")

	lunName := helper.GetInternalVolumeNameFromPath(lunPath)
	assert.Equal(t, "storagePrefix_my_Lun", lunName, "Strings not equal")

	lunPathPatternForVolume := helper.GetLUNPathPattern("my-Vol")
	assert.Equal(t, "/vol/*/storagePrefix_my_Vol", lunPathPatternForVolume, "Strings not equal")
}

func TestValidateLUN(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)

	isValid := helper.IsValidSnapLUNPath("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.True(t, isValid, "boolean not true")
}

func TestGetComponents_DockerContext(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)
	snapName := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "mysnap", snapName, "Strings not equal")

	volName := helper.GetExternalVolumeNameFromPath("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "myLun", volName, "Strings not equal")

	bucketName := helper.GetBucketName("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "myBucket", bucketName, "Strings not equal")
}

func TestGetComponents_KubernetesContext(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextKubernetes)
	snapName := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun_snapshot_snapshot_123")
	assert.Equal(t, "snapshot_123", snapName, "Strings not equal")

	snapName2 := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "mysnap", snapName2, "Strings not equal")

	volName := helper.GetExternalVolumeNameFromPath("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "myLun", volName, "Strings not equal")

	bucketName := helper.GetBucketName("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "myBucket", bucketName, "Strings not equal")
}

func TestGetComponentsNoSnapshot(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)
	snapName := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun")
	assert.Equal(t, "", snapName, "Strings not equal")

	volName := helper.GetExternalVolumeNameFromPath("/vol/myBucket/storagePrefix_myLun")
	assert.Equal(t, "myLun", volName, "Strings not equal")

	bucketName := helper.GetBucketName("/vol/myBucket/storagePrefix_myLun")
	assert.Equal(t, "myBucket", bucketName, "Strings not equal")

	snapName2 := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun")
	assert.Equal(t, "", snapName2, "Strings not equal")

	volName2 := helper.GetExternalVolumeNameFromPath("myBucket/storagePrefix_myLun")
	assert.NotEqual(t, "myLun", volName2, "Strings are equal")
	assert.Equal(t, "", volName2, "Strings are NOT equal")
}

func newTestOntapSanEcoDriver(showSensitive *bool) *SANEconomyStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	if showSensitive != nil {
		config.CommonStorageDriverConfig.DebugTraceFlags["sensitive"] = *showSensitive
	}

	config.ManagementLIF = "127.0.0.1"
	config.SVM = "SVM1"
	config.Aggregate = "aggr1"
	config.Username = "ontap-san-economy-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-san-economy"
	config.StoragePrefix = sp("test_")

	sanEcoDriver := &SANEconomyStorageDriver{}
	sanEcoDriver.Config = *config

	// ClientConfig holds the configuration data for Client objects
	clientConfig := api.ClientConfig{
		ManagementLIF:           config.ManagementLIF,
		SVM:                     "SVM1",
		Username:                "client_username",
		Password:                "client_password",
		DriverContext:           tridentconfig.DriverContext("driverContext"),
		ContextBasedZapiRecords: 100,
		DebugTraceFlags:         nil,
	}

	sanEcoDriver.API = api.NewClient(clientConfig)
	sanEcoDriver.Telemetry = &Telemetry{
		Plugin:        sanEcoDriver.Name(),
		SVM:           sanEcoDriver.GetConfig().SVM,
		StoragePrefix: *sanEcoDriver.GetConfig().StoragePrefix,
		Driver:        sanEcoDriver,
		done:          make(chan struct{}),
	}

	return sanEcoDriver
}

func TestOntapSanEcoStorageDriverConfigString(t *testing.T) {

	var sanEcoDrivers = []SANEconomyStorageDriver{
		*newTestOntapSanEcoDriver(&[]bool{true}[0]),
		*newTestOntapSanEcoDriver(&[]bool{false}[0]),
		*newTestOntapSanEcoDriver(nil),
	}

	sensitiveIncludeList := map[string]string{
		"username":        "ontap-san-economy-user",
		"password":        "password1!",
		"client username": "client_username",
		"client password": "client_password",
	}

	sensitiveExcludeList := map[string]string{
		"some information": "<REDACTED>",
	}

	externalIncludeList := map[string]string{
		"<REDACTED>":                   "<REDACTED>",
		"username":                     "Username:<REDACTED>",
		"password":                     "Password:<REDACTED>",
		"api":                          "API:<REDACTED>",
		"chap username":                "ChapUsername:<REDACTED>",
		"chap initiator secret":        "ChapInitiatorSecret:<REDACTED>",
		"chap target username":         "ChapTargetUsername:<REDACTED>",
		"chap target initiator secret": "ChapTargetInitiatorSecret:<REDACTED>",
	}

	for _, sanEcoDriver := range sanEcoDrivers {
		sensitive, ok := sanEcoDriver.Config.DebugTraceFlags["sensitive"]

		switch {

		case !ok || (ok && !sensitive):
			for key, val := range externalIncludeList {
				assert.Contains(t, sanEcoDriver.String(), val,
					"ontap-san-economy driver does not contain %v", key)
				assert.Contains(t, sanEcoDriver.GoString(), val,
					"ontap-san-economy driver does not contain %v", key)
			}

			for key, val := range sensitiveIncludeList {
				assert.NotContains(t, sanEcoDriver.String(), val,
					"ontap-san-economy driver contains %v", key)
				assert.NotContains(t, sanEcoDriver.GoString(), val,
					"ontap-san-economy driver contains %v", key)
			}

		case ok && sensitive:
			for key, val := range sensitiveIncludeList {
				assert.Contains(t, sanEcoDriver.String(), val,
					"ontap-san-economy driver does not contain %v", key)
				assert.Contains(t, sanEcoDriver.GoString(), val,
					"ontap-san-economy driver does not contain %v", key)
			}

			for key, val := range sensitiveExcludeList {
				assert.NotContains(t, sanEcoDriver.String(), val,
					"ontap-san-economy driver redacts %v", key)
				assert.NotContains(t, sanEcoDriver.GoString(), val,
					"ontap-san-economy driver redacts %v", key)
			}
		}
	}
}
