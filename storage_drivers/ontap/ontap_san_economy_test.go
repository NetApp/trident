// Copyright 2019 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	drivers "github.com/netapp/trident/storage_drivers"
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

	volName := helper.GetVolumeName("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
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

	volName := helper.GetVolumeName("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "myLun", volName, "Strings not equal")

	bucketName := helper.GetBucketName("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	assert.Equal(t, "myBucket", bucketName, "Strings not equal")
}

func TestGetComponentsNoSnapshot(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)
	snapName := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun")
	assert.Equal(t, "", snapName, "Strings not equal")

	volName := helper.GetVolumeName("/vol/myBucket/storagePrefix_myLun")
	assert.Equal(t, "myLun", volName, "Strings not equal")

	bucketName := helper.GetBucketName("/vol/myBucket/storagePrefix_myLun")
	assert.Equal(t, "myBucket", bucketName, "Strings not equal")

	snapName2 := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun")
	assert.Equal(t, "", snapName2, "Strings not equal")

	volName2 := helper.GetVolumeName("myBucket/storagePrefix_myLun")
	assert.NotEqual(t, "myLun", volName2, "Strings are equal")
	assert.Equal(t, "", volName2, "Strings are NOT equal")
}
