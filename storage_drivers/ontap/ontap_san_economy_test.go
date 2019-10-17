// Copyright 2019 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"

	tridentconfig "github.com/netapp/trident/config"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/testutils"
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
	testutils.AssertEqual(t, "Strings not equal",
		"_snapshot_snapshot_123", snapName1)

	snapName2 := helper.getInternalSnapshotName("snapshot")
	testutils.AssertEqual(t, "Strings not equal",
		"_snapshot_snapshot", snapName2)

	snapName3 := helper.getInternalSnapshotName("_snapshot")
	testutils.AssertEqual(t, "Strings not equal",
		"_snapshot__snapshot", snapName3)

	snapName4 := helper.getInternalSnapshotName("_____snapshot")
	testutils.AssertEqual(t, "Strings not equal",
		"_snapshot______snapshot", snapName4)

	k8sSnapName1 := helper.getInternalSnapshotName("snapshot-0bf1ec69_da4b_11e9_bd10_000c29e763d8")
	testutils.AssertEqual(t, "Strings not equal",
		"_snapshot_snapshot_0bf1ec69_da4b_11e9_bd10_000c29e763d8", k8sSnapName1)
}

func TestSnapshotNames_KubernetesContext(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextKubernetes)

	k8sSnapName1 := helper.getInternalSnapshotName("snapshot-0bf1ec69_da4b_11e9_bd10_000c29e763d8")
	testutils.AssertEqual(t, "Strings not equal",
		"_snapshot_snapshot_0bf1ec69_da4b_11e9_bd10_000c29e763d8", k8sSnapName1)

	k8sSnapName2 := helper.getInternalSnapshotName("mySnap")
	testutils.AssertEqual(t, "Strings not equal",
		"_snapshot_mySnap", k8sSnapName2)
}

func TestHelperGetters(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)

	snapPathPattern := helper.GetSnapPathPattern("my-Bucket")
	testutils.AssertEqual(t, "Strings not equal",
		"/vol/my_Bucket/storagePrefix_*_snapshot_*", snapPathPattern)

	snapPathPatternForVolume := helper.GetSnapPathPatternForVolume("my-Vol")
	testutils.AssertEqual(t, "Strings not equal",
		"/vol/*/storagePrefix_my_Vol_snapshot_*", snapPathPatternForVolume)

	snapPath := helper.GetSnapPath("my-Bucket", "storagePrefix_my-Lun", "snap-1")
	testutils.AssertEqual(t, "Strings not equal",
		"/vol/my_Bucket/storagePrefix_my_Lun_snapshot_snap_1", snapPath)

	snapName1 := helper.GetSnapshotName("my-Lun", "my-Snapshot")
	testutils.AssertEqual(t, "Strings not equal",
		"storagePrefix_my_Lun_snapshot_my_Snapshot", snapName1)

	snapName2 := helper.GetSnapshotName("my-Lun", "snapshot-123")
	testutils.AssertEqual(t, "Strings not equal",
		"storagePrefix_my_Lun_snapshot_snapshot_123", snapName2)

	internalSnapName := helper.GetInternalSnapshotName("storagePrefix_my-Lun", "my-Snapshot")
	testutils.AssertEqual(t, "Strings not equal",
		"storagePrefix_my_Lun_snapshot_my_Snapshot", internalSnapName)

	internalVolName := helper.GetInternalVolumeName("my-Lun")
	testutils.AssertEqual(t, "Strings not equal",
		"storagePrefix_my_Lun", internalVolName)

	lunPath := helper.GetLUNPath("my-Bucket", "my-Lun")
	testutils.AssertEqual(t, "Strings not equal",
		"/vol/my_Bucket/storagePrefix_my_Lun", lunPath)

	lunPathPatternForVolume := helper.GetLUNPathPattern("my-Vol")
	testutils.AssertEqual(t, "Strings not equal",
		"/vol/*/storagePrefix_my_Vol", lunPathPatternForVolume)
}

func TestValidateLUN(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)

	isValid := helper.IsValidSnapLUNPath("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	testutils.AssertTrue(t, "boolean not true",
		isValid)
}

func TestGetComponents_DockerContext(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)
	snapName := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	testutils.AssertEqual(t, "Strings not equal",
		"mysnap", snapName)

	volName := helper.GetVolumeName("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	testutils.AssertEqual(t, "Strings not equal",
		"myLun", volName)

	bucketName := helper.GetBucketName("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	testutils.AssertEqual(t, "Strings not equal",
		"myBucket", bucketName)
}

func TestGetComponents_KubernetesContext(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextKubernetes)
	snapName := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun_snapshot_snapshot_123")
	testutils.AssertEqual(t, "Strings not equal",
		"snapshot_123", snapName)

	snapName2 := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	testutils.AssertEqual(t, "Strings not equal",
		"mysnap", snapName2)

	volName := helper.GetVolumeName("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	testutils.AssertEqual(t, "Strings not equal",
		"myLun", volName)

	bucketName := helper.GetBucketName("/vol/myBucket/storagePrefix_myLun_snapshot_mysnap")
	testutils.AssertEqual(t, "Strings not equal",
		"myBucket", bucketName)
}

func TestGetComponentsNoSnapshot(t *testing.T) {
	helper := NewTestLUNHelper("storagePrefix_", tridentconfig.ContextDocker)
	snapName := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun")
	testutils.AssertEqual(t, "Strings not equal",
		"", snapName)

	volName := helper.GetVolumeName("/vol/myBucket/storagePrefix_myLun")
	testutils.AssertEqual(t, "Strings not equal",
		"myLun", volName)

	bucketName := helper.GetBucketName("/vol/myBucket/storagePrefix_myLun")
	testutils.AssertEqual(t, "Strings not equal",
		"myBucket", bucketName)

	snapName2 := helper.GetSnapshotNameFromSnapLUNPath("/vol/myBucket/storagePrefix_myLun")
	testutils.AssertEqual(t, "Strings not equal",
		"", snapName2)

	volName2 := helper.GetVolumeName("myBucket/storagePrefix_myLun")
	testutils.AssertNotEqual(t, "Strings are equal",
		"myLun", volName2)
	testutils.AssertEqual(t, "Strings are NOT equal",
		"", volName2)
}
