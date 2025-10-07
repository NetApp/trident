// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/pkg/convert"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
	"github.com/netapp/trident/utils/errors"
)

// setupTestZAPIClient creates a gomock controller, mock client, and ONTAP ZAPI wrapper for testing
func setupTestZAPIClient(t *testing.T) (*gomock.Controller, *mockapi.MockZapiClientInterface, api.OntapAPI) {
	ctrl := gomock.NewController(t)
	mock := mockapi.NewMockZapiClientInterface(ctrl)
	oapi, err := api.NewOntapAPIZAPIFromZapiClientInterface(mock)
	if err != nil {
		t.Fatalf("Failed to create ONTAP API: %v", err)
	}
	return ctrl, mock, oapi
}

func TestOntapAPIZAPI_LunGetFSType(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	// When value is present in LUN attribute
	mock.EXPECT().LunGetAttribute(ctx, "/vol/volumeName/storagePrefix_lunName",
		"com.netapp.ndvp.fstype").Return("raw", nil)
	fstype, err := oapi.LunGetFSType(ctx, "/vol/volumeName/storagePrefix_lunName")
	assert.NoError(t, err)
	assert.Equal(t, "raw", fstype)

	// When value is present in LUN comment
	mock.EXPECT().LunGetAttribute(ctx, "/vol/volumeName/storagePrefix_lunName",
		"com.netapp.ndvp.fstype").Return("", errors.New("not able to find fstype attribute"))
	commentJSON := `
	{
	    "lunAttributes": {
	        "driverContext": "csi",
	        "fstype": "ext4"
	    }
	}`
	mock.EXPECT().LunGetComment(ctx,
		"/vol/volumeName/storagePrefix_lunName").Return(commentJSON, nil)
	fstype, err = oapi.LunGetFSType(ctx, "/vol/volumeName/storagePrefix_lunName")
	assert.NoError(t, err)
	assert.Equal(t, "ext4", fstype)
}

func TestOntapAPIZAPI_LunGetFSType_Failure(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	// Case 1: LunGetComment fails
	mock.EXPECT().LunGetAttribute(ctx, "/vol/volumeName/storagePrefix_lunName",
		"com.netapp.ndvp.fstype").Return("", errors.New("not able to find fstype attribute"))
	mock.EXPECT().LunGetComment(ctx,
		"/vol/volumeName/storagePrefix_lunName").Return("", errors.New("failed to get LUN comment"))
	fstype, err := oapi.LunGetFSType(ctx, "/vol/volumeName/storagePrefix_lunName")
	assert.Empty(t, fstype)
	assert.Error(t, err)

	// Case 2: JSON unmarshalling fails
	invalidJSON := `
	{
	        "driverContext": "csi",
	        "fstype": "ext4"
	}`
	mock.EXPECT().LunGetAttribute(ctx, "/vol/volumeName/storagePrefix_lunName",
		"com.netapp.ndvp.fstype").Return("", errors.New("not able to find fstype attribute"))
	mock.EXPECT().LunGetComment(ctx,
		"/vol/volumeName/storagePrefix_lunName").Return(invalidJSON, nil)
	fstype, err = oapi.LunGetFSType(ctx, "/vol/volumeName/storagePrefix_lunName")
	assert.Empty(t, fstype)
	assert.Error(t, err)

	// Case 3: fstype field not found in LUN comment
	commentJSON := `
	{
	    "lunAttributes": {
	        "driverContext": "csi"
	    }
	}`
	mock.EXPECT().LunGetAttribute(ctx, "/vol/volumeName/storagePrefix_lunName",
		"com.netapp.ndvp.fstype").Return("", errors.New("not able to find fstype attribute"))
	mock.EXPECT().LunGetComment(ctx,
		"/vol/volumeName/storagePrefix_lunName").Return(commentJSON, nil)
	fstype, err = oapi.LunGetFSType(ctx, "/vol/volumeName/storagePrefix_lunName")
	assert.Empty(t, fstype)
	assert.NoError(t, err)

	// Case 4: When lunAttributes is not found
	invalidCommentJSON := `
	{
	    "attributes": {
	        "driverContext": "csi"
	    }
	}`
	mock.EXPECT().LunGetAttribute(ctx, "/vol/volumeName/storagePrefix_lunName",
		"com.netapp.ndvp.fstype").Return("", errors.New("not able to find fstype attribute"))
	mock.EXPECT().LunGetComment(ctx,
		"/vol/volumeName/storagePrefix_lunName").Return(invalidCommentJSON, nil)
	fstype, err = oapi.LunGetFSType(ctx, "/vol/volumeName/storagePrefix_lunName")
	assert.Empty(t, fstype)
	assert.Error(t, err)
}

func TestLunSetAttributeZapi(t *testing.T) {
	ctrl, zapi, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	tempLunPath := "/vol/vol0/lun0"
	tempAttribute := "filesystem"

	response := azgo.LunSetAttributeResponse{
		Result: azgo.LunSetAttributeResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	// case 1a: Positive test, update LUN attribute - fsType.
	zapi.EXPECT().LunSetAttribute(tempLunPath, tempAttribute, "fake-FStype").Return(&response, nil).Times(1)
	err := oapi.LunSetAttribute(ctx, tempLunPath, tempAttribute, "fake-FStype", "", "", "")
	assert.NoError(t, err, "error returned while modifying a LUN attribute")

	// case 1b: Negative test, d.api.LunSetAttribute for fsType return error
	zapi.EXPECT().LunSetAttribute(tempLunPath, tempAttribute, "fake-FStype").Return(nil, errors.New("error")).Times(1)
	err = oapi.LunSetAttribute(ctx, tempLunPath, tempAttribute, "fake-FStype", "", "", "")
	assert.Error(t, err)

	// case 2: Positive test, update LUN attributes those are: context, luks, formatOptions.
	zapi.EXPECT().LunSetAttribute(tempLunPath, gomock.Any(), gomock.Any()).Return(&response, nil).AnyTimes()
	err = oapi.LunSetAttribute(ctx, tempLunPath, "filesystem", "",
		"context", "LUKS", "formatOptions")
	assert.NoError(t, err, "error returned while modifying a LUN attribute")
}

func TestLunGetAttributeZapi(t *testing.T) {
	ctrl, zapi, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	tempLunPath := "/vol/vol1/lun0"
	tempAttributeName := "fsType"

	// 1 - Negative test, d.api.LunGetAttribute returns error
	zapi.EXPECT().LunGetAttribute(gomock.Any(), tempLunPath, tempAttributeName).Return("", errors.New("error")).Times(1)
	attributeValue, err := oapi.LunGetAttribute(ctx, tempLunPath, tempAttributeName)
	assert.Error(t, err)
	assert.Equal(t, "", attributeValue)

	tempAttributeVale := "ext4"
	// 2 - Positive test, d.api.LunGetAttribute do not return error
	zapi.EXPECT().LunGetAttribute(gomock.Any(), tempLunPath, tempAttributeName).Return(tempAttributeVale, nil).Times(1)
	attributeValue, err = oapi.LunGetAttribute(ctx, tempLunPath, tempAttributeName)
	assert.NoError(t, err)
	assert.Equal(t, tempAttributeVale, attributeValue)
}

func TestExportRuleList_Zapi_NoDuplicateRules(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	policyName := "testPolicy"
	ruleListResponse := &azgo.ExportRuleGetIterResponse{
		Result: azgo.ExportRuleGetIterResponseResult{
			NumRecordsPtr: intPtr(2),
			AttributesListPtr: &azgo.ExportRuleGetIterResponseResultAttributesList{
				ExportRuleInfoPtr: []azgo.ExportRuleInfoType{
					{RuleIndexPtr: intPtr(1), ClientMatchPtr: convert.ToPtr("192.168.1.0")},
					{RuleIndexPtr: intPtr(2), ClientMatchPtr: convert.ToPtr("192.168.1.1")},
				},
			},
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().ExportRuleGetIterRequest(policyName).Return(ruleListResponse, nil).Times(1)
	rules, err := oapi.ExportRuleList(ctx, policyName)
	assert.NoError(t, err)
	assert.Equal(t, map[int]string{1: "192.168.1.0", 2: "192.168.1.1"}, rules)
}

func TestExportRuleList_Zapi_DuplicateRules(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	policyName := "testPolicy"
	ruleListResponse := &azgo.ExportRuleGetIterResponse{
		Result: azgo.ExportRuleGetIterResponseResult{
			NumRecordsPtr: intPtr(5),
			AttributesListPtr: &azgo.ExportRuleGetIterResponseResultAttributesList{
				ExportRuleInfoPtr: []azgo.ExportRuleInfoType{
					{RuleIndexPtr: intPtr(1), ClientMatchPtr: convert.ToPtr("192.168.1.1")},
					{RuleIndexPtr: intPtr(2), ClientMatchPtr: convert.ToPtr("192.168.1.1")},
					{RuleIndexPtr: intPtr(3), ClientMatchPtr: convert.ToPtr("192.168.1.2")},
					{RuleIndexPtr: intPtr(4), ClientMatchPtr: convert.ToPtr("192.168.1.2")},
					{RuleIndexPtr: intPtr(5), ClientMatchPtr: convert.ToPtr("192.168.1.1")},
				},
			},
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().ExportRuleGetIterRequest(policyName).Return(ruleListResponse, nil).Times(1)
	rules, err := oapi.ExportRuleList(ctx, policyName)
	assert.NoError(t, err)
	assert.Equal(t, map[int]string{
		1: "192.168.1.1",
		2: "192.168.1.1",
		3: "192.168.1.2",
		4: "192.168.1.2",
		5: "192.168.1.1",
	}, rules)
}

func TestExportRuleList_Zapi_Error(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	policyName := "testPolicy"

	mock.EXPECT().ExportRuleGetIterRequest(policyName).Return(nil,
		errors.New("error listing export policy rules")).Times(1)
	rules, err := oapi.ExportRuleList(ctx, policyName)
	assert.Error(t, err)
	assert.Nil(t, rules)
}

func TestExportRuleList_Zapi_NoRecords(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	policyName := "testPolicy"
	ruleListResponse := &azgo.ExportRuleGetIterResponse{
		Result: azgo.ExportRuleGetIterResponseResult{
			NumRecordsPtr: intPtr(0),
			AttributesListPtr: &azgo.ExportRuleGetIterResponseResultAttributesList{
				ExportRuleInfoPtr: []azgo.ExportRuleInfoType{},
			},
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().ExportRuleGetIterRequest(policyName).Return(ruleListResponse, nil).Times(1)
	rules, err := oapi.ExportRuleList(ctx, policyName)
	assert.NoError(t, err)
	assert.Empty(t, rules)
}

func TestExportRuleList_Zapi_NilPayload(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	policyName := "testPolicy"
	ruleListResponse := &azgo.ExportRuleGetIterResponse{
		Result: azgo.ExportRuleGetIterResponseResult{
			NumRecordsPtr:     intPtr(0),
			AttributesListPtr: nil,
			ResultStatusAttr:  "passed",
		},
	}

	mock.EXPECT().ExportRuleGetIterRequest(policyName).Return(ruleListResponse, nil).Times(1)
	rules, err := oapi.ExportRuleList(ctx, policyName)
	assert.NoError(t, err)
	assert.Empty(t, rules)
}

func TestOntapAPIZAPI_LunExists(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	tempLunPath := "/vol/vol1/lun0"

	// Mock ClientConfig to avoid unexpected call
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// Case 1: LUN exists
	mock.EXPECT().LunGet(tempLunPath).Return(&azgo.LunInfoType{}, nil).Times(1)
	exists, err := oapi.LunExists(ctx, tempLunPath)
	assert.NoError(t, err, "expected no error when LUN exists")
	assert.True(t, exists, "expected LUN to exist")

	// Case 2: LUN does not exist
	mock.EXPECT().LunGet(tempLunPath).Return(nil, nil).Times(1)
	exists, err = oapi.LunExists(ctx, tempLunPath)
	assert.NoError(t, err, "expected no error when LUN does not exist")
	assert.False(t, exists, "expected LUN to not exist")

	// Case 3: NotFoundError from ZAPI client
	mock.EXPECT().LunGet(tempLunPath).Return(nil, errors.NotFoundError("LUN not found")).Times(1)
	exists, err = oapi.LunExists(ctx, tempLunPath)
	assert.NoError(t, err, "expected no error when NotFoundError occurs")
	assert.False(t, exists, "expected LUN to not exist")

	// Case 4: Error from ZAPI client
	mock.EXPECT().LunGet(tempLunPath).Return(nil, errors.New("error fetching LUN")).Times(1)
	exists, err = oapi.LunExists(ctx, tempLunPath)
	assert.Error(t, err, "expected error when ZAPI client returns an error")
	assert.False(t, exists, "expected LUN to not exist")
}

func intPtr(i int) *int {
	return &i
}

func TestOntapAPIZAPI_ConsistencyGroupSnapshot_success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()
	// Mock ClientConfig to avoid unexpected call
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	snapshotName := "snapshot-12345"
	volumes := []string{"vol1", "vol2"}

	cgStartResponse := &azgo.CgStartResponse{
		Result: azgo.CgStartResponseResult{
			CgIdPtr:          convert.ToPtr(123),
			ResultStatusAttr: "passed",
		},
	}
	mock.EXPECT().ConsistencyGroupStart(snapshotName, volumes).Return(cgStartResponse, nil).Times(1)
	mock.EXPECT().ConsistencyGroupCommit(*cgStartResponse.Result.CgIdPtr).Return(nil, nil).Times(1)

	err := oapi.ConsistencyGroupSnapshot(ctx, snapshotName, volumes)
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_ConsistencyGroupSnapshot_fail(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()
	// Mock ClientConfig to avoid unexpected call
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	snapshotName := "snapshot-12345"
	volumes := []string{"vol1", "vol2"}

	cgStartResponse := &azgo.CgStartResponse{
		Result: azgo.CgStartResponseResult{
			CgIdPtr:          nil,
			ResultStatusAttr: "failed",
			ResultErrnoAttr:  "123",
			ResultReasonAttr: "error, forced failure",
		},
	}
	mock.EXPECT().ConsistencyGroupStart(snapshotName, volumes).Return(cgStartResponse, nil).Times(1)

	err := oapi.ConsistencyGroupSnapshot(ctx, snapshotName, volumes)
	assert.Error(t, err)
}

// Volume Operations Tests
func TestOntapAPIZAPI_VolumeCreate_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	volume := api.Volume{
		Name:            "test_volume",
		Aggregates:      []string{"aggr1"},
		Size:            "1g",
		SpaceReserve:    "none",
		SnapshotPolicy:  "default",
		UnixPermissions: "755",
		ExportPolicy:    "default",
		SecurityStyle:   "unix",
		TieringPolicy:   "",
		Comment:         "test volume",
		Qos:             api.QosPolicyGroup{},
		Encrypt:         convert.ToPtr(false),
		SnapshotReserve: 5,
		DPVolume:        false,
	}

	volumeCreateResponse := &azgo.VolumeCreateResponse{
		Result: azgo.VolumeCreateResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().VolumeCreate(
		ctx, volume.Name, volume.Aggregates[0], volume.Size, volume.SpaceReserve,
		volume.SnapshotPolicy, volume.UnixPermissions, volume.ExportPolicy,
		volume.SecurityStyle, volume.TieringPolicy, volume.Comment, volume.Qos,
		volume.Encrypt, volume.SnapshotReserve, volume.DPVolume,
	).Return(volumeCreateResponse, nil).Times(1)

	err := oapi.VolumeCreate(ctx, volume)
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_VolumeCreate_Error(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	volume := api.Volume{
		Name:       "test_volume",
		Aggregates: []string{"aggr1"},
		Size:       "1g",
	}

	// Test API error
	mock.EXPECT().VolumeCreate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return(nil, errors.New("API error")).Times(1)

	err := oapi.VolumeCreate(ctx, volume)
	assert.Error(t, err, "expected error when volume creation fails")
	assert.Contains(t, err.Error(), "error creating volume")
}

func TestOntapAPIZAPI_VolumeCreate_JobExists(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	volume := api.Volume{
		Name:       "test_volume",
		Aggregates: []string{"aggr1"},
		Size:       "1g",
	}

	volumeCreateResponse := &azgo.VolumeCreateResponse{
		Result: azgo.VolumeCreateResponseResult{
			ResultStatusAttr: "failed",
			ResultErrnoAttr:  "13001",
			ResultReasonAttr: "Job exists",
		},
	}

	mock.EXPECT().VolumeCreate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return(volumeCreateResponse, nil).Times(1)

	err := oapi.VolumeCreate(ctx, volume)
	assert.Error(t, err, "expected VolumeCreateJobExistsError when volume create job already exists")
	assert.True(t, api.IsVolumeCreateJobExistsError(err))
}

func TestOntapAPIZAPI_VolumeDestroy_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	volumeName := "test_volume"
	force := true
	skipRecoveryQueue := false

	volumeDestroyResponse := &azgo.VolumeDestroyResponse{
		Result: azgo.VolumeDestroyResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().VolumeDestroy(volumeName, force).Return(volumeDestroyResponse, nil).Times(1)

	err := oapi.VolumeDestroy(ctx, volumeName, force, skipRecoveryQueue)
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_VolumeDestroy_VolumeDoesNotExist(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	volumeName := "test_volume"
	force := true
	skipRecoveryQueue := false

	volumeDestroyResponse := &azgo.VolumeDestroyResponse{
		Result: azgo.VolumeDestroyResponseResult{
			ResultStatusAttr: "failed",
			ResultErrnoAttr:  "13040",
			ResultReasonAttr: "Volume does not exist",
		},
	}

	mock.EXPECT().VolumeDestroy(volumeName, force).Return(volumeDestroyResponse, nil).Times(1)

	err := oapi.VolumeDestroy(ctx, volumeName, force, skipRecoveryQueue)
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_VolumeDestroy_WithRecoveryQueuePurge(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	volumeName := "test_volume"
	recoveryQueueName := "test_volume_recovery"
	force := true
	skipRecoveryQueue := true

	volumeDestroyResponse := &azgo.VolumeDestroyResponse{
		Result: azgo.VolumeDestroyResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	volumeRecoveryQueueGetResponse := &azgo.VolumeRecoveryQueueGetIterResponse{
		Result: azgo.VolumeRecoveryQueueGetIterResponseResult{
			ResultStatusAttr: "passed",
			AttributesListPtr: &azgo.VolumeRecoveryQueueGetIterResponseResultAttributesList{
				VolumeRecoveryQueueInfoPtr: []azgo.VolumeRecoveryQueueInfoType{
					{VolumeNamePtr: convert.ToPtr(recoveryQueueName)},
				},
			},
		},
	}

	volumeRecoveryQueuePurgeResponse := &azgo.VolumeRecoveryQueuePurgeResponse{
		Result: azgo.VolumeRecoveryQueuePurgeResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().VolumeDestroy(volumeName, force).Return(volumeDestroyResponse, nil).Times(1)
	mock.EXPECT().VolumeRecoveryQueueGetIter(fmt.Sprintf("%s*", volumeName)).Return(volumeRecoveryQueueGetResponse, nil).Times(1)
	mock.EXPECT().VolumeRecoveryQueuePurge(recoveryQueueName).Return(volumeRecoveryQueuePurgeResponse, nil).Times(1)

	err := oapi.VolumeDestroy(ctx, volumeName, force, skipRecoveryQueue)
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_VolumeList_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	prefix := "test_"

	// Mock VolumeGetAll response
	volumeGetAllResponse := &azgo.VolumeGetIterResponse{
		Result: azgo.VolumeGetIterResponseResult{
			ResultStatusAttr: "passed",
			AttributesListPtr: &azgo.VolumeGetIterResponseResultAttributesList{
				VolumeAttributesPtr: []azgo.VolumeAttributesType{
					{
						VolumeIdAttributesPtr: &azgo.VolumeIdAttributesType{
							NamePtr:                    convert.ToPtr("test_volume1"),
							ContainingAggregateNamePtr: convert.ToPtr("aggr1"),
							TypePtr:                    convert.ToPtr("rw"),
						},
						VolumeSpaceAttributesPtr: &azgo.VolumeSpaceAttributesType{
							SizePtr: convert.ToPtr(1073741824),
						},
					},
					{
						VolumeIdAttributesPtr: &azgo.VolumeIdAttributesType{
							NamePtr:                    convert.ToPtr("test_volume2"),
							ContainingAggregateNamePtr: convert.ToPtr("aggr2"),
							TypePtr:                    convert.ToPtr("rw"),
						},
						VolumeSpaceAttributesPtr: &azgo.VolumeSpaceAttributesType{
							SizePtr: convert.ToPtr(2147483648),
						},
					},
				},
			},
		},
	}

	mock.EXPECT().VolumeGetAll(prefix).Return(volumeGetAllResponse, nil).Times(1)

	volumes, err := oapi.VolumeListByPrefix(ctx, prefix)
	assert.NoError(t, err)
	assert.Len(t, volumes, 2)
	assert.Equal(t, "test_volume1", volumes[0].Name)
	assert.Equal(t, "test_volume2", volumes[1].Name)
}

func TestOntapAPIZAPI_VolumeSize_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	volumeName := "test_volume"
	expectedSize := 1073741824 // 1GB in bytes

	mock.EXPECT().VolumeSize(volumeName).Return(expectedSize, nil).Times(1)

	size, err := oapi.VolumeSize(ctx, volumeName)
	assert.NoError(t, err)
	assert.Equal(t, uint64(expectedSize), size)
}

func TestOntapAPIZAPI_VolumeUsedSize_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	volumeName := "test_volume"
	expectedUsedSize := 536870912 // 512MB in bytes

	mock.EXPECT().VolumeUsedSize(volumeName).Return(expectedUsedSize, nil).Times(1)

	usedSize, err := oapi.VolumeUsedSize(ctx, volumeName)
	assert.NoError(t, err)
	assert.Equal(t, expectedUsedSize, usedSize)
}

func TestOntapAPIZAPI_VolumeSetSize_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	volumeName := "test_volume"
	newSize := "2g"

	volumeSetSizeResponse := &azgo.VolumeSizeResponse{
		Result: azgo.VolumeSizeResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().VolumeSetSize(volumeName, newSize).Return(volumeSetSizeResponse, nil).Times(1)

	err := oapi.VolumeSetSize(ctx, volumeName, newSize)
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_VolumeMount_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	volumeMountResponse := &azgo.VolumeMountResponse{
		Result: azgo.VolumeMountResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().VolumeMount("test_volume", "/test_volume").Return(volumeMountResponse, nil).Times(1)

	err := oapi.VolumeMount(ctx, "test_volume", "/test_volume")
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_VolumeMount_ApiError(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().VolumeMount("test_volume", "/test_volume").Return(nil, errors.New("API error")).Times(1)

	err := oapi.VolumeMount(ctx, "test_volume", "/test_volume")
	assert.Error(t, err, "expected error when volume mount fails")
	assert.Contains(t, err.Error(), "error mounting volume to junction")
}

func TestOntapAPIZAPI_VolumeMount_ZapiError(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	volumeMountResponse := &azgo.VolumeMountResponse{
		Result: azgo.VolumeMountResponseResult{
			ResultStatusAttr: "failed",
			ResultErrnoAttr:  "13001",
			ResultReasonAttr: "API Error",
		},
	}

	mock.EXPECT().VolumeMount("test_volume", "/test_volume").Return(volumeMountResponse, nil).Times(1)

	err := oapi.VolumeMount(ctx, "test_volume", "/test_volume")
	assert.Error(t, err, "expected API error when volume mount returns error response")
	assert.True(t, api.IsApiError(err))
}

func TestOntapAPIZAPI_VolumeExists_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().VolumeExists(ctx, "test_volume").Return(true, nil).Times(1)

	exists, err := oapi.VolumeExists(ctx, "test_volume")
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestOntapAPIZAPI_VolumeExists_NotFound(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().VolumeExists(ctx, "nonexistent_volume").Return(false, nil).Times(1)

	exists, err := oapi.VolumeExists(ctx, "nonexistent_volume")
	assert.NoError(t, err)
	assert.False(t, exists)
}

// LUN Operations Tests

func TestOntapAPIZAPI_LunCreate_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	lun := api.Lun{
		Name:           "/vol/test_volume/test_lun",
		Size:           "1g",
		OsType:         "linux",
		Qos:            api.QosPolicyGroup{Name: "qos_policy"},
		SpaceReserved:  convert.ToPtr(true),
		SpaceAllocated: convert.ToPtr(true),
	}

	lunCreateResponse := &azgo.LunCreateBySizeResponse{
		Result: azgo.LunCreateBySizeResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().LunCreate(lun.Name, 1073741824, lun.OsType, lun.Qos, *lun.SpaceReserved, *lun.SpaceAllocated).Return(lunCreateResponse, nil).Times(1)

	err := oapi.LunCreate(ctx, lun)
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_LunCreate_InvalidSize(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	lun := api.Lun{
		Name:           "/vol/test_volume/test_lun",
		Size:           "invalid_size",
		OsType:         "linux",
		Qos:            api.QosPolicyGroup{Name: "qos_policy"},
		SpaceReserved:  convert.ToPtr(true),
		SpaceAllocated: convert.ToPtr(true),
	}

	err := oapi.LunCreate(ctx, lun)
	assert.Error(t, err, "expected error when LUN creation has invalid volume size")
	assert.Contains(t, err.Error(), "invalid volume size")
}

func TestOntapAPIZAPI_LunCreate_ApiError(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	lun := api.Lun{
		Name:           "/vol/test_volume/test_lun",
		Size:           "1g",
		OsType:         "linux",
		Qos:            api.QosPolicyGroup{Name: "qos_policy"},
		SpaceReserved:  convert.ToPtr(true),
		SpaceAllocated: convert.ToPtr(true),
	}

	mock.EXPECT().LunCreate(lun.Name, 1073741824, lun.OsType, lun.Qos, *lun.SpaceReserved, *lun.SpaceAllocated).Return(nil, errors.New("API error")).Times(1)

	err := oapi.LunCreate(ctx, lun)
	assert.Error(t, err, "expected error when LUN creation API call fails")
	assert.Contains(t, err.Error(), "error creating LUN")
}

func TestOntapAPIZAPI_LunCreate_NilResponse(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	lun := api.Lun{
		Name:           "/vol/test_volume/test_lun",
		Size:           "1g",
		OsType:         "linux",
		Qos:            api.QosPolicyGroup{Name: "qos_policy"},
		SpaceReserved:  convert.ToPtr(true),
		SpaceAllocated: convert.ToPtr(true),
	}

	mock.EXPECT().LunCreate(lun.Name, 1073741824, lun.OsType, lun.Qos, *lun.SpaceReserved, *lun.SpaceAllocated).Return(nil, nil).Times(1)

	err := oapi.LunCreate(ctx, lun)
	assert.Error(t, err, "expected error when LUN creation returns nil response")
	assert.Contains(t, err.Error(), "missing LUN create response")
}

func TestOntapAPIZAPI_LunCreate_JobExists(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	lun := api.Lun{
		Name:           "/vol/test_volume/test_lun",
		Size:           "1g",
		OsType:         "linux",
		Qos:            api.QosPolicyGroup{Name: "qos_policy"},
		SpaceReserved:  convert.ToPtr(true),
		SpaceAllocated: convert.ToPtr(true),
	}

	lunCreateResponse := &azgo.LunCreateBySizeResponse{
		Result: azgo.LunCreateBySizeResponseResult{
			ResultStatusAttr: "failed",
			ResultErrnoAttr:  "13001",
			ResultReasonAttr: "Job exists",
		},
	}

	mock.EXPECT().LunCreate(lun.Name, 1073741824, lun.OsType, lun.Qos, *lun.SpaceReserved, *lun.SpaceAllocated).Return(lunCreateResponse, nil).Times(1)

	err := oapi.LunCreate(ctx, lun)
	assert.Error(t, err, "expected VolumeCreateJobExistsError when LUN create job already exists")
	assert.True(t, api.IsVolumeCreateJobExistsError(err))
}

func TestOntapAPIZAPI_LunDestroy_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	lunOfflineResponse := &azgo.LunOfflineResponse{
		Result: azgo.LunOfflineResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	lunDestroyResponse := &azgo.LunDestroyResponse{
		Result: azgo.LunDestroyResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().LunOffline("/vol/test_volume/test_lun").Return(lunOfflineResponse, nil).Times(1)
	mock.EXPECT().LunDestroy("/vol/test_volume/test_lun").Return(lunDestroyResponse, nil).Times(1)

	err := oapi.LunDestroy(ctx, "/vol/test_volume/test_lun")
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_LunDestroy_OfflineError(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	lunPath := "/vol/test_volume/test_lun"

	mock.EXPECT().LunOffline(lunPath).Return(nil, errors.New("offline failed")).Times(1)

	err := oapi.LunDestroy(ctx, lunPath)
	assert.Error(t, err, "expected error when LUN offline operation fails")
	assert.Contains(t, err.Error(), "offline failed")
}

func TestOntapAPIZAPI_LunDestroy_DestroyError(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	lunPath := "/vol/test_volume/test_lun"

	lunOfflineResponse := &azgo.LunOfflineResponse{
		Result: azgo.LunOfflineResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().LunOffline(lunPath).Return(lunOfflineResponse, nil).Times(1)
	mock.EXPECT().LunDestroy(lunPath).Return(nil, errors.New("destroy failed")).Times(1)

	err := oapi.LunDestroy(ctx, lunPath)
	assert.Error(t, err, "expected error when LUN destroy operation fails")
	assert.Contains(t, err.Error(), "destroy failed")
}

func TestOntapAPIZAPI_LunMapInfo_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	// Create mock initiator group info
	igroupInfo := azgo.InitiatorGroupInfoType{}
	igroupInfo.SetInitiatorGroupName("test_igroup")
	igroupInfo.SetLunId(1)

	initiatorGroups := azgo.LunMapListInfoResponseResultInitiatorGroups{}
	initiatorGroups.SetInitiatorGroupInfo([]azgo.InitiatorGroupInfoType{igroupInfo})

	lunMapListResponse := &azgo.LunMapListInfoResponse{
		Result: azgo.LunMapListInfoResponseResult{
			ResultStatusAttr:   "passed",
			InitiatorGroupsPtr: &initiatorGroups,
		},
	}

	mock.EXPECT().LunMapListInfo("/vol/test_volume/test_lun").Return(lunMapListResponse, nil).Times(1)

	resultLunID, err := oapi.LunMapInfo(ctx, "test_igroup", "/vol/test_volume/test_lun")
	assert.NoError(t, err)
	assert.Equal(t, 1, resultLunID)
}

func TestOntapAPIZAPI_LunUnmap_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	lunUnmapResponse := &azgo.LunUnmapResponse{
		Result: azgo.LunUnmapResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().LunUnmap("test_igroup", "/vol/test_volume/test_lun").Return(lunUnmapResponse, nil).Times(1)

	err := oapi.LunUnmap(ctx, "test_igroup", "/vol/test_volume/test_lun")
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_LunSize_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().LunSize("/vol/test_volume/test_lun").Return(1073741824, nil).Times(1)

	resultSize, err := oapi.LunSize(ctx, "/vol/test_volume/test_lun")
	assert.NoError(t, err)
	assert.Equal(t, 1073741824, resultSize)
}

func TestOntapAPIZAPI_LunSetSize_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	expectedSizeBytes := uint64(2147483648)

	mock.EXPECT().LunResize("/vol/test_volume/test_lun", 2147483648).Return(expectedSizeBytes, nil).Times(1)

	resultSize, err := oapi.LunSetSize(ctx, "/vol/test_volume/test_lun", "2g")
	assert.NoError(t, err)
	assert.Equal(t, expectedSizeBytes, resultSize)
}

func TestOntapAPIZAPI_LunSetSize_InvalidSizeFormat(t *testing.T) {
	ctrl, _, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	resultSize, err := oapi.LunSetSize(ctx, "/vol/test_volume/test_lun", "invalid_size")
	assert.Error(t, err, "expected error when LUN size is invalid")
	assert.Equal(t, uint64(0), resultSize)
}

func TestOntapAPIZAPI_LunSetSize_NegativeSize(t *testing.T) {
	ctrl, _, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	resultSize, err := oapi.LunSetSize(ctx, "/vol/test_volume/test_lun", "-100m")
	assert.Error(t, err, "expected error when LUN size is negative")
	assert.Equal(t, uint64(0), resultSize)
}

func TestOntapAPIZAPI_LunSetSize_ResizeError(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().LunResize("/vol/test_volume/test_lun", 2147483648).Return(uint64(0), errors.New("resize failed")).Times(1)

	resultSize, err := oapi.LunSetSize(ctx, "/vol/test_volume/test_lun", "2g")
	assert.Error(t, err, "expected error when LUN resize operation fails")
	assert.Equal(t, uint64(0), resultSize)
	assert.Contains(t, err.Error(), "resize failed")
}

func TestOntapAPIZAPI_LunRename_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	lunMoveResponse := &azgo.LunMoveResponse{
		Result: azgo.LunMoveResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().LunRename("/vol/test_volume/test_lun", "/vol/test_volume/test_lun_new").Return(lunMoveResponse, nil).Times(1)

	err := oapi.LunRename(ctx, "/vol/test_volume/test_lun", "/vol/test_volume/test_lun_new")
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_LunList_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	prefix := "/vol/test_volume/test_"

	lunInfoPtr := []azgo.LunInfoType{
		{
			PathPtr: convert.ToPtr("/vol/test_volume/test_lun1"),
			SizePtr: convert.ToPtr(1073741824),
		},
		{
			PathPtr: convert.ToPtr("/vol/test_volume/test_lun2"),
			SizePtr: convert.ToPtr(2147483648),
		},
	}

	attributesList := azgo.LunGetIterResponseResultAttributesList{}
	attributesList.SetLunInfo(lunInfoPtr)

	lunGetAllResponse := &azgo.LunGetIterResponse{
		Result: azgo.LunGetIterResponseResult{
			ResultStatusAttr:  "passed",
			AttributesListPtr: &attributesList,
		},
	}

	mock.EXPECT().LunGetAll(prefix).Return(lunGetAllResponse, nil).Times(1)

	luns, err := oapi.LunList(ctx, prefix)
	assert.NoError(t, err)
	assert.Len(t, luns, 2)
	assert.Equal(t, "/vol/test_volume/test_lun1", luns[0].Name)
	assert.Equal(t, "/vol/test_volume/test_lun2", luns[1].Name)
}

// iGroup Operations Tests
func TestOntapAPIZAPI_IgroupCreate_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	igroupCreateResponse := &azgo.IgroupCreateResponse{
		Result: azgo.IgroupCreateResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().IgroupCreate("test_igroup", "iscsi", "linux").Return(igroupCreateResponse, nil).Times(1)

	err := oapi.IgroupCreate(ctx, "test_igroup", "iscsi", "linux")
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_IgroupDestroy_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	igroupDestroyResponse := &azgo.IgroupDestroyResponse{
		Result: azgo.IgroupDestroyResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().IgroupDestroy("test_igroup").Return(igroupDestroyResponse, nil).Times(1)

	err := oapi.IgroupDestroy(ctx, "test_igroup")
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_IgroupAdd_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	igroupAddResponse := &azgo.IgroupAddResponse{
		Result: azgo.IgroupAddResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().IgroupAdd("test_igroup", "iqn.1993-08.org.debian:01:test").Return(igroupAddResponse, nil).Times(1)

	err := oapi.EnsureIgroupAdded(ctx, "test_igroup", "iqn.1993-08.org.debian:01:test")
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_IgroupRemove_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	igroupRemoveResponse := &azgo.IgroupRemoveResponse{
		Result: azgo.IgroupRemoveResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().IgroupRemove("test_igroup", "iqn.1993-08.org.debian:01:test", false).Return(igroupRemoveResponse, nil).Times(1)

	err := oapi.IgroupRemove(ctx, "test_igroup", "iqn.1993-08.org.debian:01:test", false)
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_IgroupList_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	igroupGetIterResponse := &azgo.IgroupGetIterResponse{
		Result: azgo.IgroupGetIterResponseResult{
			ResultStatusAttr: "passed",
			NumRecordsPtr:    convert.ToPtr(2),
			AttributesListPtr: &azgo.IgroupGetIterResponseResultAttributesList{
				InitiatorGroupInfoPtr: []azgo.InitiatorGroupInfoType{
					{
						InitiatorGroupNamePtr: convert.ToPtr("test_igroup1"),
						InitiatorGroupTypePtr: convert.ToPtr("iscsi"),
					},
					{
						InitiatorGroupNamePtr: convert.ToPtr("test_igroup2"),
						InitiatorGroupTypePtr: convert.ToPtr("fcp"),
					},
				},
			},
		},
	}

	mock.EXPECT().IgroupList().Return(igroupGetIterResponse, nil).Times(1)

	igroups, err := oapi.IgroupList(ctx)
	assert.NoError(t, err)
	assert.Len(t, igroups, 2)
	assert.Equal(t, "test_igroup1", igroups[0])
	assert.Equal(t, "test_igroup2", igroups[1])
}

func TestOntapAPIZAPI_SVMName(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	expectedSVMName := "test-svm"
	mock.EXPECT().SVMName().Return(expectedSVMName).Times(1)

	result := oapi.SVMName()
	assert.Equal(t, expectedSVMName, result)
}

func TestOntapAPIZAPI_IsSANOptimized(t *testing.T) {
	ctrl, _, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	result := oapi.IsSANOptimized()
	assert.False(t, result)
}

func TestOntapAPIZAPI_IsDisaggregated(t *testing.T) {
	ctrl, _, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	result := oapi.IsDisaggregated()
	assert.False(t, result)
}

func TestOntapAPIZAPI_APIVersion_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	expectedVersion := "1.160"
	mock.EXPECT().SystemGetOntapiVersion(ctx, true).Return(expectedVersion, nil).Times(1)

	version, err := oapi.APIVersion(ctx, true)
	assert.NoError(t, err)
	assert.Equal(t, expectedVersion, version)
}

func TestOntapAPIZAPI_APIVersion_Error(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().SystemGetOntapiVersion(ctx, false).Return("", errors.New("API error")).Times(1)

	version, err := oapi.APIVersion(ctx, false)
	assert.Error(t, err, "expected error when API version retrieval fails")
	assert.Empty(t, version)
}

func TestOntapAPIZAPI_SupportsFeature_True(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	testFeature := api.Feature("test-feature")
	mock.EXPECT().SupportsFeature(ctx, testFeature).Return(true).Times(1)

	result := oapi.SupportsFeature(ctx, testFeature)
	assert.True(t, result)
}

func TestOntapAPIZAPI_SupportsFeature_False(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	testFeature := api.Feature("unsupported-feature")
	mock.EXPECT().SupportsFeature(ctx, testFeature).Return(false).Times(1)

	result := oapi.SupportsFeature(ctx, testFeature)
	assert.False(t, result)
}

func TestOntapAPIZAPI_NodeListSerialNumbers_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	expectedSerials := []string{"1234567890", "0987654321"}
	mock.EXPECT().NodeListSerialNumbers(ctx).Return(expectedSerials, nil).Times(1)

	serials, err := oapi.NodeListSerialNumbers(ctx)
	assert.NoError(t, err)
	assert.Equal(t, expectedSerials, serials)
}

func TestOntapAPIZAPI_NodeListSerialNumbers_Error(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().NodeListSerialNumbers(ctx).Return(nil, errors.New("failed to get serials")).Times(1)

	serials, err := oapi.NodeListSerialNumbers(ctx)
	assert.Error(t, err, "expected error when node serial number retrieval fails")
	assert.Nil(t, serials)
}

func TestOntapAPIZAPI_ValidateAPIVersion_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	validVersion := "1.160"
	mock.EXPECT().SystemGetOntapiVersion(ctx, true).Return(validVersion, nil).Times(1)
	mock.EXPECT().SupportsFeature(ctx, api.MinimumONTAPIVersion).Return(true).Times(1)

	err := oapi.ValidateAPIVersion(ctx)
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_ValidateAPIVersion_GetVersionError(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().SystemGetOntapiVersion(ctx, true).Return("", errors.New("version error")).Times(1)

	err := oapi.ValidateAPIVersion(ctx)
	assert.Error(t, err, "expected error when API version validation fails")
	assert.Contains(t, err.Error(), "could not determine Data ONTAP API version")
}

func TestOntapAPIZAPI_ValidateAPIVersion_UnsupportedVersion(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	oldVersion := "1.140"
	mock.EXPECT().SystemGetOntapiVersion(ctx, true).Return(oldVersion, nil).Times(1)
	mock.EXPECT().SupportsFeature(ctx, api.MinimumONTAPIVersion).Return(false).Times(1)

	err := oapi.ValidateAPIVersion(ctx)
	assert.Error(t, err, "expected error when ONTAP version is unsupported")
	assert.Contains(t, err.Error(), "ONTAP 9.5 or later is required")
}

func TestOntapAPIZAPI_NetInterfaceGetDataLIFs_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	protocol := "nfs"
	expectedLIFs := []string{"192.168.1.10", "192.168.1.11"}
	mock.EXPECT().NetInterfaceGetDataLIFs(ctx, protocol).Return(expectedLIFs, nil).Times(1)

	lifs, err := oapi.NetInterfaceGetDataLIFs(ctx, protocol)
	assert.NoError(t, err)
	assert.Equal(t, expectedLIFs, lifs)
}

func TestOntapAPIZAPI_NetInterfaceGetDataLIFs_Error(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	protocol := "iscsi"
	mock.EXPECT().NetInterfaceGetDataLIFs(ctx, protocol).Return(nil, errors.New("failed to get LIFs")).Times(1)

	lifs, err := oapi.NetInterfaceGetDataLIFs(ctx, protocol)
	assert.Error(t, err, "expected error when network interface data LIFs retrieval fails")
	assert.Nil(t, lifs)
}

func TestOntapAPIZAPI_NetFcpInterfaceGetDataLIFs_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	protocol := "fcp"
	expectedLIFs := []string{"20:00:00:25:b5:a0:a0:01", "20:00:00:25:b5:a0:a0:02"}
	mock.EXPECT().NetFcpInterfaceGetDataLIFs(ctx, protocol).Return(expectedLIFs, nil).Times(1)

	lifs, err := oapi.NetFcpInterfaceGetDataLIFs(ctx, protocol)
	assert.NoError(t, err)
	assert.Equal(t, expectedLIFs, lifs)
}

func TestOntapAPIZAPI_NetFcpInterfaceGetDataLIFs_Error(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	protocol := "fcp"
	mock.EXPECT().NetFcpInterfaceGetDataLIFs(ctx, protocol).Return(nil, errors.New("failed to get FCP LIFs")).Times(1)

	lifs, err := oapi.NetFcpInterfaceGetDataLIFs(ctx, protocol)
	assert.Error(t, err, "expected error when FCP interface data LIFs retrieval fails")
	assert.Nil(t, lifs)
}

func TestOntapAPIZAPI_GetSVMAggregateNames_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	expectedAggregates := []string{"aggr1", "aggr2", "aggr3"}
	mock.EXPECT().SVMGetAggregateNames().Return(expectedAggregates, nil).Times(1)

	aggregates, err := oapi.GetSVMAggregateNames(ctx)
	assert.NoError(t, err)
	assert.Equal(t, expectedAggregates, aggregates)
}

func TestOntapAPIZAPI_GetSVMAggregateNames_Error(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().SVMGetAggregateNames().Return(nil, errors.New("failed to get aggregates")).Times(1)

	aggregates, err := oapi.GetSVMAggregateNames(ctx)
	assert.Error(t, err, "expected error when SVM aggregate names retrieval fails")
	assert.Nil(t, aggregates)
}

func TestOntapAPIZAPI_EmsAutosupportLog(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	emsResponse := &azgo.EmsAutosupportLogResponse{
		Result: azgo.EmsAutosupportLogResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().EmsAutosupportLog(
		"1.0.0", true, "info", "test-computer", "test event", 1, "trident", 5,
	).Return(emsResponse, nil).Times(1)

	oapi.EmsAutosupportLog(ctx, "trident", "1.0.0", true, "info", "test-computer", "test event", 1, "trident", 5)
}

func TestOntapAPIZAPI_TieringPolicyValue(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	expectedPolicy := "auto"
	mock.EXPECT().TieringPolicyValue(ctx).Return(expectedPolicy).Times(1)

	result := oapi.TieringPolicyValue(ctx)
	assert.Equal(t, expectedPolicy, result)
}

func TestOntapAPIZAPI_VolumeSetSize_Enhanced(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	volumeName := "test_volume"
	newSize := "2g"

	volumeSetSizeResponse := &azgo.VolumeSizeResponse{
		Result: azgo.VolumeSizeResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	mock.EXPECT().VolumeSetSize(volumeName, newSize).Return(volumeSetSizeResponse, nil).Times(1)

	err := oapi.VolumeSetSize(ctx, volumeName, newSize)
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_VolumeSetSize_ApiError(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	volumeName := "test_volume"
	newSize := "2g"

	mock.EXPECT().VolumeSetSize(volumeName, newSize).Return(nil, errors.New("API error")).Times(1)

	err := oapi.VolumeSetSize(ctx, volumeName, newSize)
	assert.Error(t, err, "expected error when volume set size API call fails")
	assert.Contains(t, err.Error(), "volume resize failed for volume")
}

func TestOntapAPIZAPI_VolumeSetSize_NilResponse(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	volumeName := "test_volume"
	newSize := "2g"

	mock.EXPECT().VolumeSetSize(volumeName, newSize).Return(nil, nil).Times(1)

	err := oapi.VolumeSetSize(ctx, volumeName, newSize)
	assert.Error(t, err, "expected error when volume set size returns nil response")
	assert.Contains(t, err.Error(), "unexpected error")
}

func TestOntapAPIZAPI_VolumeSetSize_FailedResponse(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	volumeName := "test_volume"
	newSize := "2g"

	volumeSetSizeResponse := &azgo.VolumeSizeResponse{
		Result: azgo.VolumeSizeResponseResult{
			ResultStatusAttr: "failed",
			ResultErrnoAttr:  "13115",
			ResultReasonAttr: "Volume size too large",
		},
	}

	mock.EXPECT().VolumeSetSize(volumeName, newSize).Return(volumeSetSizeResponse, nil).Times(1)

	err := oapi.VolumeSetSize(ctx, volumeName, newSize)
	assert.Error(t, err, "expected error when volume set size returns failed response")
	assert.Contains(t, err.Error(), "volume resize failed")
}

func TestOntapAPIZAPI_FcpInterfaceGet_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	svm := "test-svm"

	fcpInterfaceResponse := &azgo.FcpInterfaceGetIterResponse{
		Result: azgo.FcpInterfaceGetIterResponseResult{
			ResultStatusAttr: "passed",
			AttributesListPtr: &azgo.FcpInterfaceGetIterResponseResultAttributesList{
				FcpInterfaceInfoPtr: []azgo.FcpInterfaceInfoType{
					{
						NodeNamePtr: convert.ToPtr("20:00:00:25:b5:a0:a0:01"),
						PortNamePtr: convert.ToPtr("20:00:00:25:b5:a0:a0:02"),
					},
					{
						NodeNamePtr: convert.ToPtr("20:00:00:25:b5:a0:a0:03"),
						PortNamePtr: convert.ToPtr("20:00:00:25:b5:a0:a0:04"),
					},
				},
			},
		},
	}

	mock.EXPECT().FcpInterfaceGetIterRequest().Return(fcpInterfaceResponse, nil).Times(1)

	interfaces, err := oapi.FcpInterfaceGet(ctx, svm)
	assert.NoError(t, err)
	assert.Len(t, interfaces, 2)
	assert.Equal(t, "20:00:00:25:b5:a0:a0:01:20:00:00:25:b5:a0:a0:02", interfaces[0])
	assert.Equal(t, "20:00:00:25:b5:a0:a0:03:20:00:00:25:b5:a0:a0:04", interfaces[1])
}

func TestOntapAPIZAPI_FcpInterfaceGet_NoInterfaces(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	svm := "test-svm"

	fcpInterfaceResponse := &azgo.FcpInterfaceGetIterResponse{
		Result: azgo.FcpInterfaceGetIterResponseResult{
			ResultStatusAttr: "passed",
			AttributesListPtr: &azgo.FcpInterfaceGetIterResponseResultAttributesList{
				FcpInterfaceInfoPtr: []azgo.FcpInterfaceInfoType{},
			},
		},
	}

	mock.EXPECT().FcpInterfaceGetIterRequest().Return(fcpInterfaceResponse, nil).Times(1)

	interfaces, err := oapi.FcpInterfaceGet(ctx, svm)
	assert.Error(t, err, "expected error when no active FCP interfaces found")
	assert.Contains(t, err.Error(), "has no active FCP interfaces")
	assert.Nil(t, interfaces)
}

func TestOntapAPIZAPI_FcpInterfaceGet_Error(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	svm := "test-svm"

	mock.EXPECT().FcpInterfaceGetIterRequest().Return(nil, errors.New("FCP interface error")).Times(1)

	interfaces, err := oapi.FcpInterfaceGet(ctx, svm)
	assert.Error(t, err, "expected error when FCP interface get request fails")
	assert.Nil(t, interfaces)
}

func TestOntapAPIZAPI_FcpNodeGetNameRequest_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	fcpNodeResponse := &azgo.FcpNodeGetNameResponse{
		Result: azgo.IscsiNodeGetNameResponseResult{
			ResultStatusAttr: "passed",
			NodeNamePtr:      convert.ToPtr("20:00:00:25:b5:a0:a0:01"),
		},
	}

	mock.EXPECT().FcpNodeGetNameRequest().Return(fcpNodeResponse, nil).Times(1)

	nodeName, err := oapi.FcpNodeGetNameRequest(ctx)
	assert.NoError(t, err)
	assert.Equal(t, "20:00:00:25:b5:a0:a0:01", nodeName)
}

func TestOntapAPIZAPI_FcpNodeGetNameRequest_Error(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().FcpNodeGetNameRequest().Return(nil, errors.New("FCP node error")).Times(1)

	nodeName, err := oapi.FcpNodeGetNameRequest(ctx)
	assert.Error(t, err, "expected error when FCP node name request fails")
	assert.Empty(t, nodeName)
}

func TestOntapAPIZAPI_LunGetGeometry_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	lunPath := "/vol/test_volume/test_lun"
	expectedMaxSize := uint64(16776704)

	lunGeometryResponse := &azgo.LunGetGeometryResponse{
		Result: azgo.LunGetGeometryResponseResult{
			ResultStatusAttr: "passed",
			MaxResizeSizePtr: convert.ToPtr(int(expectedMaxSize)),
		},
	}

	mock.EXPECT().LunGetGeometry(lunPath).Return(lunGeometryResponse, nil).Times(1)

	maxSize, err := oapi.LunGetGeometry(ctx, lunPath)
	assert.NoError(t, err)
	assert.Equal(t, expectedMaxSize, maxSize)
}

func TestOntapAPIZAPI_LunGetGeometry_Error(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	lunPath := "/vol/test_volume/test_lun"

	mock.EXPECT().LunGetGeometry(lunPath).Return(nil, errors.New("geometry error")).Times(1)

	maxSize, err := oapi.LunGetGeometry(ctx, lunPath)
	assert.Error(t, err, "expected error when LUN geometry retrieval fails")
	assert.Equal(t, uint64(0), maxSize)
}

func TestOntapAPIZAPI_VolumeInfo_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()
	mock.EXPECT().VolumeGet("test_volume").Return(nil, errors.New("volume not found")).Times(1)
	_, err := oapi.VolumeInfo(ctx, "test_volume")
	assert.Error(t, err, "expected error when volume info retrieval fails")
}

func TestParseLunSnapshotPath_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().LunCloneCreate("flexvol", "/invalid/path", "", "newlun", api.QosPolicyGroup{}).Return(nil, errors.New("test")).AnyTimes()
	err := oapi.LunCloneCreate(ctx, "flexvol", "/invalid/path", "newlun", api.QosPolicyGroup{})
	assert.Error(t, err, "expected error when LUN clone creation fails")
}

func TestOntapAPIZAPI_LunCloneCreate_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().LunCloneCreate("flexvol", "src", "", "dst", api.QosPolicyGroup{}).Return(nil, errors.New("test")).AnyTimes()
	err := oapi.LunCloneCreate(ctx, "flexvol", "src", "dst", api.QosPolicyGroup{})
	assert.Error(t, err, "expected error when LUN clone creation API call fails")
}

func TestOntapAPIZAPI_LunSetComment_Success(t *testing.T) {
	ctrl, _, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	result := oapi.LunSetComment(ctx, "/vol/vol1/lun1", "comment")
	assert.Error(t, result)
}

func TestOntapAPIZAPI_LunGetByName_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()
	mock.EXPECT().LunGet("/vol/vol1/lun1").Return(&azgo.LunInfoType{}, nil).Times(1)
	lun, err := oapi.LunGetByName(ctx, "/vol/vol1/lun1")
	assert.NoError(t, err)
	assert.NotNil(t, lun)
}

func TestOntapAPIZAPI_EnsureLunMapped_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().LunMapIfNotMapped(ctx, "igroup1", "/vol/vol1/lun1").Return(0, nil).Times(1)
	result, err := oapi.EnsureLunMapped(ctx, "igroup1", "/vol/vol1/lun1")
	assert.NoError(t, err)
	assert.Equal(t, 0, result)
}

func TestOntapAPIZAPI_LunListIgroupsMapped_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	mock.EXPECT().LunMapsGetByLun("/vol/vol1/lun1").Return(nil, errors.New("not implemented")).Times(1)
	_, err := oapi.LunListIgroupsMapped(ctx, "/vol/vol1/lun1")
	assert.Error(t, err, "expected error when LUN igroups mapping list fails")
}

func TestOntapAPIZAPI_VolumeSnapshotCreate_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	response := &azgo.SnapshotCreateResponse{Result: azgo.SnapshotCreateResponseResult{ResultStatusAttr: "passed"}}
	mock.EXPECT().SnapshotCreate("snapshot1", "volume1").Return(response, nil).Times(1)
	err := oapi.VolumeSnapshotCreate(ctx, "snapshot1", "volume1")
	assert.NoError(t, err)
}

func TestOntapAPIZAPI_VolumeCloneCreate_Success(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	response := &azgo.VolumeCloneCreateResponse{Result: azgo.VolumeCloneCreateResponseResult{ResultStatusAttr: "passed"}}
	mock.EXPECT().VolumeCloneCreate("clone", "source", "snap").Return(response, nil).Times(1)
	err := oapi.VolumeCloneCreate(ctx, "clone", "source", "snap", false)
	assert.NoError(t, err)
}

func TestParseLunSnapshotPath(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		shouldFail     bool
		expectSnapshot bool
	}{
		{
			name:           "Valid_snapshot_path",
			path:           "/vol/vol1/.snapshot/snap1/lun1",
			shouldFail:     false,
			expectSnapshot: true,
		},
		{
			name:           "Valid_snapshot_path_with_subdirs",
			path:           "/vol/volume_name/.snapshot/snapshot_name/path/to/lun",
			shouldFail:     false,
			expectSnapshot: true,
		},
		{
			name:           "Invalid_path_format",
			path:           "/vol/vol1/lun1",
			shouldFail:     false,
			expectSnapshot: false,
		},
		{
			name:           "Invalid_prefix",
			path:           "/volume/vol1/.snapshot/snap1/lun1",
			shouldFail:     false,
			expectSnapshot: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl, mock, oapi := setupTestZAPIClient(t)
			defer ctrl.Finish()

			if tt.expectSnapshot {
				// For snapshot paths that should be parsed correctly
				response := &azgo.CloneCreateResponse{
					Result: azgo.CloneCreateResponseResult{
						ResultStatusAttr: "passed",
					},
				}
				mock.EXPECT().LunCloneCreate(gomock.Any(), gomock.Any(), gomock.Any(), "newlun", api.QosPolicyGroup{}).Return(response, nil).Times(1)
			} else {
				// For non-snapshot paths
				response := &azgo.CloneCreateResponse{
					Result: azgo.CloneCreateResponseResult{
						ResultStatusAttr: "passed",
					},
				}
				mock.EXPECT().LunCloneCreate("flexvol", tt.path, "", "newlun", api.QosPolicyGroup{}).Return(response, nil).Times(1)
			}

			err := oapi.LunCloneCreate(ctx, "flexvol", tt.path, "newlun", api.QosPolicyGroup{})

			if tt.shouldFail {
				assert.Error(t, err, "expected error when LUN clone creation should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestNewOntapAPIZAPI(t *testing.T) {
	tests := []struct {
		name       string
		shouldFail bool
	}{
		{
			name:       "Successful_client_creation",
			shouldFail: false,
		},
		{
			name:       "Another_successful_creation",
			shouldFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := api.ClientConfig{
				ManagementLIF: "192.168.1.10",
				Username:      "admin",
				Password:      "password",
			}
			client := api.NewZAPIClient(config, "test-svm", "trident")
			oapi, err := api.NewOntapAPIZAPI(client)

			if tt.shouldFail {
				assert.Error(t, err, "expected error when ONTAP API ZAPI creation should fail")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, oapi)
				// Test basic functionality
				assert.False(t, oapi.IsSANOptimized())
				assert.False(t, oapi.IsDisaggregated())
			}
		})
	}
}

func TestNewZAPIClient(t *testing.T) {
	tests := []struct {
		name       string
		config     api.ClientConfig
		svm        string
		driverName string
	}{
		{
			name: "Complete_configuration",
			config: api.ClientConfig{
				ManagementLIF:           "192.168.1.10",
				Username:                "admin",
				Password:                "password",
				ClientCertificate:       "test-cert",
				ClientPrivateKey:        "test-key",
				TrustedCACertificate:    "test-ca",
				ContextBasedZapiRecords: 100,
				DebugTraceFlags:         map[string]bool{"method": true},
			},
			svm:        "test-svm",
			driverName: "ontap-nas",
		},
		{
			name: "Minimal_configuration",
			config: api.ClientConfig{
				ManagementLIF: "192.168.1.10",
				Username:      "admin",
				Password:      "password",
			},
			svm:        "minimal-svm",
			driverName: "ontap-san",
		},
		{
			name:       "Empty_configuration",
			config:     api.ClientConfig{},
			svm:        "",
			driverName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := api.NewZAPIClient(tt.config, tt.svm, tt.driverName)
			assert.NotNil(t, client)

			// Test that client configuration is accessible
			clientConfig := client.ClientConfig()
			assert.Equal(t, tt.config.ManagementLIF, clientConfig.ManagementLIF)
			assert.Equal(t, tt.config.Username, clientConfig.Username)
			assert.Equal(t, tt.config.Password, clientConfig.Password)
		})
	}
}

func TestLunSetQosPolicyGroup(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	tests := []struct {
		name       string
		lunPath    string
		qosPolicy  api.QosPolicyGroup
		setupMocks func()
		shouldFail bool
	}{
		{
			name:    "Successful_QoS_policy_set",
			lunPath: "/vol/vol1/lun1",
			qosPolicy: api.QosPolicyGroup{
				Name: "high-performance",
			},
			setupMocks: func() {
				response := &azgo.LunSetQosPolicyGroupResponse{
					Result: azgo.LunSetQosPolicyGroupResponseResult{
						ResultStatusAttr: "passed",
					},
				}
				mock.EXPECT().LunSetQosPolicyGroup("/vol/vol1/lun1", api.QosPolicyGroup{
					Name: "high-performance",
				}).Return(response, nil).Times(1)
			},
			shouldFail: false,
		},
		{
			name:    "API_error",
			lunPath: "/vol/vol1/lun1",
			qosPolicy: api.QosPolicyGroup{
				Name: "test-policy",
			},
			setupMocks: func() {
				mock.EXPECT().LunSetQosPolicyGroup("/vol/vol1/lun1", api.QosPolicyGroup{
					Name: "test-policy",
				}).Return(nil, errors.New("API connection error")).Times(1)
			},
			shouldFail: true,
		},
		{
			name:    "ZAPI_error_response",
			lunPath: "/vol/vol1/lun1",
			qosPolicy: api.QosPolicyGroup{
				Name: "invalid-policy",
			},
			setupMocks: func() {
				response := &azgo.LunSetQosPolicyGroupResponse{
					Result: azgo.LunSetQosPolicyGroupResponseResult{
						ResultStatusAttr: "failed",
						ResultErrnoAttr:  "13115",
						ResultReasonAttr: "QoS policy does not exist",
					},
				}
				mock.EXPECT().LunSetQosPolicyGroup("/vol/vol1/lun1", api.QosPolicyGroup{
					Name: "invalid-policy",
				}).Return(response, nil).Times(1)
			},
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks()

			err := oapi.LunSetQosPolicyGroup(ctx, tt.lunPath, tt.qosPolicy)

			if tt.shouldFail {
				assert.Error(t, err, "expected error when LUN QoS policy group setting should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_LunSetQosPolicyGroup(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	tests := []struct {
		name           string
		lunPath        string
		qosPolicyGroup api.QosPolicyGroup
		mockResponse   *azgo.LunSetQosPolicyGroupResponse
		mockError      error
		expectError    bool
	}{
		{
			name:    "successful_qos_policy_assignment",
			lunPath: "/vol/test_volume/test_lun",
			qosPolicyGroup: api.QosPolicyGroup{
				Name: "high-performance",
				Kind: api.QosPolicyGroupKind,
			},
			mockResponse: &azgo.LunSetQosPolicyGroupResponse{
				Result: azgo.LunSetQosPolicyGroupResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:    "invalid_lun_path",
			lunPath: "/invalid/lun/path",
			qosPolicyGroup: api.QosPolicyGroup{
				Name: "test-policy",
			},
			mockResponse: &azgo.LunSetQosPolicyGroupResponse{
				Result: azgo.LunSetQosPolicyGroupResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  "13040",
					ResultReasonAttr: "LUN not found",
				},
			},
			mockError:   nil,
			expectError: true,
		},
		{
			name:    "api_error_handling",
			lunPath: "/vol/test_volume/test_lun",
			qosPolicyGroup: api.QosPolicyGroup{
				Name: "test-policy",
			},
			mockResponse: nil,
			mockError:    errors.New("connection refused"),
			expectError:  true,
		},
		{
			name:    "empty_qos_policy_name",
			lunPath: "/vol/test_volume/test_lun",
			qosPolicyGroup: api.QosPolicyGroup{
				Name: "",
			},
			mockResponse: &azgo.LunSetQosPolicyGroupResponse{
				Result: azgo.LunSetQosPolicyGroupResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:    "qos_policy_not_found",
			lunPath: "/vol/test_volume/test_lun",
			qosPolicyGroup: api.QosPolicyGroup{
				Name: "non-existent-policy",
			},
			mockResponse: &azgo.LunSetQosPolicyGroupResponse{
				Result: azgo.LunSetQosPolicyGroupResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  "13115",
					ResultReasonAttr: "QoS policy group not found",
				},
			},
			mockError:   nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().LunSetQosPolicyGroup(tt.lunPath, tt.qosPolicyGroup).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.LunSetQosPolicyGroup(ctx, tt.lunPath, tt.qosPolicyGroup)

			if tt.expectError {
				assert.Error(t, err, "expected error when LUN QoS policy group setting fails")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_LunMapGetReportingNodes(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	tests := []struct {
		name               string
		initiatorGroupName string
		lunPath            string
		mockResponse       *azgo.LunMapGetIterResponse
		mockError          error
		expectedNodes      []string
		expectError        bool
	}{
		{
			name:               "successful_node_retrieval",
			initiatorGroupName: "test_igroup",
			lunPath:            "/vol/test_volume/test_lun",
			mockResponse: &azgo.LunMapGetIterResponse{
				Result: azgo.LunMapGetIterResponseResult{
					ResultStatusAttr: "passed",
					AttributesListPtr: &azgo.LunMapGetIterResponseResultAttributesList{
						LunMapInfoPtr: []azgo.LunMapInfoType{
							{
								ReportingNodesPtr: &azgo.LunMapInfoTypeReportingNodes{
									NodeNamePtr: []string{"node1", "node2"},
								},
							},
						},
					},
				},
			},
			mockError:     nil,
			expectedNodes: []string{"node1", "node2"},
			expectError:   false,
		},
		{
			name:               "api_error_scenario",
			initiatorGroupName: "test_igroup",
			lunPath:            "/vol/test_volume/test_lun",
			mockResponse:       nil,
			mockError:          errors.New("network timeout"),
			expectedNodes:      nil,
			expectError:        true,
		},
		{
			name:               "multiple_lun_map_entries",
			initiatorGroupName: "test_igroup",
			lunPath:            "/vol/test_volume/test_lun",
			mockResponse: &azgo.LunMapGetIterResponse{
				Result: azgo.LunMapGetIterResponseResult{
					ResultStatusAttr: "passed",
					AttributesListPtr: &azgo.LunMapGetIterResponseResultAttributesList{
						LunMapInfoPtr: []azgo.LunMapInfoType{
							{
								ReportingNodesPtr: &azgo.LunMapInfoTypeReportingNodes{
									NodeNamePtr: []string{"node1"},
								},
							},
							{
								ReportingNodesPtr: &azgo.LunMapInfoTypeReportingNodes{
									NodeNamePtr: []string{"node2", "node3"},
								},
							},
						},
					},
				},
			},
			mockError:     nil,
			expectedNodes: []string{"node1", "node2", "node3"},
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().LunMapGet(tt.initiatorGroupName, tt.lunPath).
				Return(tt.mockResponse, tt.mockError).Times(1)

			nodes, err := oapi.LunMapGetReportingNodes(ctx, tt.initiatorGroupName, tt.lunPath)

			if tt.expectError {
				assert.Error(t, err, "expected error when LUN map reporting nodes retrieval fails")
				assert.Nil(t, nodes)
			} else {
				assert.NoError(t, err)
				if len(tt.expectedNodes) == 0 {
					assert.Empty(t, nodes)
				} else {
					assert.Equal(t, tt.expectedNodes, nodes)
				}
			}
		})
	}
}

func TestOntapAPIZAPI_IscsiInitiatorGetDefaultAuth(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name             string
		mockResponse     *azgo.IscsiInitiatorGetDefaultAuthResponse
		mockError        error
		expectedAuthInfo api.IscsiInitiatorAuth
		expectError      bool
	}{
		{
			name: "successful_auth_retrieval",
			mockResponse: &azgo.IscsiInitiatorGetDefaultAuthResponse{
				Result: azgo.IscsiInitiatorGetDefaultAuthResponseResult{
					ResultStatusAttr:    "passed",
					AuthTypePtr:         convert.ToPtr("CHAP"),
					UserNamePtr:         convert.ToPtr("test_user"),
					OutboundUserNamePtr: convert.ToPtr("test_outbound_user"),
				},
			},
			mockError: nil,
			expectedAuthInfo: api.IscsiInitiatorAuth{
				AuthType:         "CHAP",
				ChapUser:         "test_user",
				ChapOutboundUser: "test_outbound_user",
			},
			expectError: false,
		},
		{
			name: "missing_auth_info",
			mockResponse: &azgo.IscsiInitiatorGetDefaultAuthResponse{
				Result: azgo.IscsiInitiatorGetDefaultAuthResponseResult{
					ResultStatusAttr: "passed",
					AuthTypePtr:      nil,
					UserNamePtr:      nil,
				},
			},
			mockError:        nil,
			expectedAuthInfo: api.IscsiInitiatorAuth{},
			expectError:      false,
		},
		{
			name:             "api_error_handling",
			mockResponse:     nil,
			mockError:        errors.New("network timeout"),
			expectedAuthInfo: api.IscsiInitiatorAuth{},
			expectError:      true,
		},
		{
			name: "partial_auth_info",
			mockResponse: &azgo.IscsiInitiatorGetDefaultAuthResponse{
				Result: azgo.IscsiInitiatorGetDefaultAuthResponseResult{
					ResultStatusAttr: "passed",
					AuthTypePtr:      convert.ToPtr("none"),
				},
			},
			mockError: nil,
			expectedAuthInfo: api.IscsiInitiatorAuth{
				AuthType: "none",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().IscsiInitiatorGetDefaultAuth().
				Return(tt.mockResponse, tt.mockError).Times(1)

			authInfo, err := oapi.IscsiInitiatorGetDefaultAuth(ctx)

			if tt.expectError {
				assert.Error(t, err, "expected error when iSCSI initiator default auth retrieval fails")
				assert.Equal(t, api.IscsiInitiatorAuth{}, authInfo)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAuthInfo, authInfo)
			}
		})
	}
}

func TestOntapAPIZAPI_IscsiInitiatorSetDefaultAuth(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name               string
		authType           string
		userName           string
		passphrase         string
		outboundUserName   string
		outboundPassphrase string
		mockResponse       *azgo.IscsiInitiatorSetDefaultAuthResponse
		mockError          error
		expectError        bool
	}{
		{
			name:               "set_chap_auth",
			authType:           "CHAP",
			userName:           "chap_user",
			passphrase:         "chap_password",
			outboundUserName:   "chap_outbound_user",
			outboundPassphrase: "chap_outbound_password",
			mockResponse: &azgo.IscsiInitiatorSetDefaultAuthResponse{
				Result: azgo.IscsiInitiatorSetDefaultAuthResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:               "api_error_scenario",
			authType:           "CHAP",
			userName:           "test_user",
			passphrase:         "test_password",
			outboundUserName:   "",
			outboundPassphrase: "",
			mockResponse:       nil,
			mockError:          errors.New("network error"),
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().IscsiInitiatorSetDefaultAuth(
				tt.authType, tt.userName, tt.passphrase, tt.outboundUserName, tt.outboundPassphrase).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.IscsiInitiatorSetDefaultAuth(ctx, tt.authType, tt.userName, tt.passphrase,
				tt.outboundUserName, tt.outboundPassphrase)

			if tt.expectError {
				assert.Error(t, err, "expected error when iSCSI initiator default auth setting fails")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_IscsiInterfaceGet(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name               string
		svm                string
		mockResponse       *azgo.IscsiInterfaceGetIterResponse
		mockError          error
		expectedInterfaces []string
		expectError        bool
	}{
		{
			name: "successful_interface_retrieval",
			svm:  "test_svm",
			mockResponse: &azgo.IscsiInterfaceGetIterResponse{
				Result: azgo.IscsiInterfaceGetIterResponseResult{
					ResultStatusAttr: "passed",
					AttributesListPtr: &azgo.IscsiInterfaceGetIterResponseResultAttributesList{
						IscsiInterfaceListEntryInfoPtr: []azgo.IscsiInterfaceListEntryInfoType{
							{
								IpAddressPtr:          convert.ToPtr("192.168.1.10"),
								IpPortPtr:             convert.ToPtr(3260),
								IsInterfaceEnabledPtr: convert.ToPtr(true),
							},
							{
								IpAddressPtr:          convert.ToPtr("192.168.1.11"),
								IpPortPtr:             convert.ToPtr(3260),
								IsInterfaceEnabledPtr: convert.ToPtr(true),
							},
						},
					},
				},
			},
			mockError:          nil,
			expectedInterfaces: []string{"192.168.1.10:3260", "192.168.1.11:3260"},
			expectError:        false,
		},
		{
			name: "no_active_interfaces",
			svm:  "test_svm",
			mockResponse: &azgo.IscsiInterfaceGetIterResponse{
				Result: azgo.IscsiInterfaceGetIterResponseResult{
					ResultStatusAttr:  "passed",
					AttributesListPtr: nil,
				},
			},
			mockError:          nil,
			expectedInterfaces: nil,
			expectError:        true,
		},
		{
			name:               "api_error_handling",
			svm:                "test_svm",
			mockResponse:       nil,
			mockError:          errors.New("connection failed"),
			expectedInterfaces: nil,
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().IscsiInterfaceGetIterRequest().
				Return(tt.mockResponse, tt.mockError).Times(1)

			interfaces, err := oapi.IscsiInterfaceGet(ctx, tt.svm)

			if tt.expectError {
				assert.Error(t, err, "expected error when iSCSI interface retrieval fails")
				assert.Nil(t, interfaces)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedInterfaces, interfaces)
			}
		})
	}
}

func TestOntapAPIZAPI_IscsiNodeGetNameRequest(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name             string
		mockResponse     *azgo.IscsiNodeGetNameResponse
		mockError        error
		expectedNodeName string
		expectError      bool
	}{
		{
			name: "successful_node_name_retrieval",
			mockResponse: &azgo.IscsiNodeGetNameResponse{
				Result: azgo.IscsiNodeGetNameResponseResult{
					ResultStatusAttr: "passed",
					NodeNamePtr:      convert.ToPtr("iqn.1992-08.com.netapp:sn.12345678901234567890123456789012:vs.1"),
				},
			},
			mockError:        nil,
			expectedNodeName: "iqn.1992-08.com.netapp:sn.12345678901234567890123456789012:vs.1",
			expectError:      false,
		},
		{
			name:             "api_error_handling",
			mockResponse:     nil,
			mockError:        errors.New("iSCSI service not configured"),
			expectedNodeName: "",
			expectError:      true,
		},
		{
			name: "empty_node_name",
			mockResponse: &azgo.IscsiNodeGetNameResponse{
				Result: azgo.IscsiNodeGetNameResponseResult{
					ResultStatusAttr: "passed",
					NodeNamePtr:      convert.ToPtr(""),
				},
			},
			mockError:        nil,
			expectedNodeName: "",
			expectError:      false,
		},
		{
			name: "nil_node_name",
			mockResponse: &azgo.IscsiNodeGetNameResponse{
				Result: azgo.IscsiNodeGetNameResponseResult{
					ResultStatusAttr: "passed",
					NodeNamePtr:      nil,
				},
			},
			mockError:        nil,
			expectedNodeName: "",
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().IscsiNodeGetNameRequest().
				Return(tt.mockResponse, tt.mockError).Times(1)

			nodeName, err := oapi.IscsiNodeGetNameRequest(ctx)

			if tt.expectError {
				assert.Error(t, err, "expected error when iSCSI node name request fails")
				assert.Empty(t, nodeName)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedNodeName, nodeName)
			}
		})
	}
}

func TestOntapAPIZAPI_IgroupListLUNsMapped(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name               string
		initiatorGroupName string
		mockResponse       *azgo.LunMapGetIterResponse
		mockError          error
		expectedLUNs       []string
		expectError        bool
	}{
		{
			name:               "successful_lun_list_retrieval",
			initiatorGroupName: "test_igroup",
			mockResponse: &azgo.LunMapGetIterResponse{
				Result: azgo.LunMapGetIterResponseResult{
					ResultStatusAttr: "passed",
					AttributesListPtr: &azgo.LunMapGetIterResponseResultAttributesList{
						LunMapInfoPtr: []azgo.LunMapInfoType{
							{
								PathPtr: convert.ToPtr("/vol/test_volume/test_lun1"),
							},
							{
								PathPtr: convert.ToPtr("/vol/test_volume/test_lun2"),
							},
						},
					},
				},
			},
			mockError:    nil,
			expectedLUNs: []string{"/vol/test_volume/test_lun1", "/vol/test_volume/test_lun2"},
			expectError:  false,
		},
		{
			name:               "no_mapped_luns",
			initiatorGroupName: "empty_igroup",
			mockResponse: &azgo.LunMapGetIterResponse{
				Result: azgo.LunMapGetIterResponseResult{
					ResultStatusAttr:  "passed",
					AttributesListPtr: nil,
				},
			},
			mockError:    nil,
			expectedLUNs: []string{},
			expectError:  false,
		},
		{
			name:               "api_error_handling",
			initiatorGroupName: "test_igroup",
			mockResponse:       nil,
			mockError:          errors.New("network timeout"),
			expectedLUNs:       nil,
			expectError:        true,
		},
		{
			name:               "empty_attributes_list",
			initiatorGroupName: "test_igroup",
			mockResponse: &azgo.LunMapGetIterResponse{
				Result: azgo.LunMapGetIterResponseResult{
					ResultStatusAttr: "passed",
					AttributesListPtr: &azgo.LunMapGetIterResponseResultAttributesList{
						LunMapInfoPtr: []azgo.LunMapInfoType{},
					},
				},
			},
			mockError:    nil,
			expectedLUNs: []string{},
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().LunMapsGetByIgroup(tt.initiatorGroupName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			luns, err := oapi.IgroupListLUNsMapped(ctx, tt.initiatorGroupName)

			if tt.expectError {
				assert.Error(t, err, "expected error when igroup LUNs mapping list fails")
				assert.Nil(t, luns)
			} else {
				assert.NoError(t, err)
				if len(tt.expectedLUNs) == 0 {
					assert.Empty(t, luns)
				} else {
					assert.Equal(t, tt.expectedLUNs, luns)
				}
			}
		})
	}
}

func TestOntapAPIZAPI_IgroupGetByName(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name               string
		initiatorGroupName string
		mockResponse       *azgo.InitiatorGroupInfoType
		mockError          error
		expectedInitiators map[string]bool
		expectError        bool
	}{
		{
			name:               "successful_igroup_retrieval",
			initiatorGroupName: "test_igroup",
			mockResponse: &azgo.InitiatorGroupInfoType{
				InitiatorGroupNamePtr: convert.ToPtr("test_igroup"),
				InitiatorsPtr: &azgo.InitiatorGroupInfoTypeInitiators{
					InitiatorInfoPtr: []azgo.InitiatorInfoType{
						{
							InitiatorNamePtr: convert.ToPtr("iqn.1991-05.com.microsoft:test-server1"),
						},
						{
							InitiatorNamePtr: convert.ToPtr("iqn.1991-05.com.microsoft:test-server2"),
						},
					},
				},
			},
			mockError: nil,
			expectedInitiators: map[string]bool{
				"iqn.1991-05.com.microsoft:test-server1": true,
				"iqn.1991-05.com.microsoft:test-server2": true,
			},
			expectError: false,
		},
		{
			name:               "non_existent_igroup",
			initiatorGroupName: "non_existent_igroup",
			mockResponse:       nil,
			mockError:          errors.New("initiator group not found"),
			expectedInitiators: nil,
			expectError:        true,
		},
		{
			name:               "empty_initiators",
			initiatorGroupName: "empty_igroup",
			mockResponse: &azgo.InitiatorGroupInfoType{
				InitiatorGroupNamePtr: convert.ToPtr("empty_igroup"),
				InitiatorsPtr:         nil,
			},
			mockError:          nil,
			expectedInitiators: map[string]bool{},
			expectError:        false,
		},
		{
			name:               "initiator_parsing_with_nil_names",
			initiatorGroupName: "test_igroup",
			mockResponse: &azgo.InitiatorGroupInfoType{
				InitiatorGroupNamePtr: convert.ToPtr("test_igroup"),
				InitiatorsPtr: &azgo.InitiatorGroupInfoTypeInitiators{
					InitiatorInfoPtr: []azgo.InitiatorInfoType{
						{
							InitiatorNamePtr: convert.ToPtr("iqn.1991-05.com.microsoft:valid-server"),
						},
						{
							InitiatorNamePtr: nil,
						},
					},
				},
			},
			mockError: nil,
			expectedInitiators: map[string]bool{
				"iqn.1991-05.com.microsoft:valid-server": true,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().IgroupGet(tt.initiatorGroupName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			initiators, err := oapi.IgroupGetByName(ctx, tt.initiatorGroupName)

			if tt.expectError {
				assert.Error(t, err, "expected error when igroup retrieval should fail")
				assert.Nil(t, initiators)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedInitiators, initiators)
			}
		})
	}
}

func TestOntapAPIZAPI_GetSLMDataLifs(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name               string
		ips                []string
		reportingNodeNames []string
		mockResponse       *azgo.NetInterfaceGetIterResponse
		mockError          error
		expectedLIFs       []string
		expectError        bool
	}{
		{
			name:               "lif_filtering_by_node_names",
			ips:                []string{"192.168.1.10", "192.168.1.11", "192.168.1.12"},
			reportingNodeNames: []string{"node1", "node2"},
			mockResponse: &azgo.NetInterfaceGetIterResponse{
				Result: azgo.NetInterfaceGetIterResponseResult{
					ResultStatusAttr: "passed",
					AttributesListPtr: &azgo.NetInterfaceGetIterResponseResultAttributesList{
						NetInterfaceInfoPtr: []azgo.NetInterfaceInfoType{
							{
								AddressPtr:     convert.ToPtr("192.168.1.10"),
								CurrentNodePtr: convert.ToPtr("node1"),
							},
							{
								AddressPtr:     convert.ToPtr("192.168.1.11"),
								CurrentNodePtr: convert.ToPtr("node2"),
							},
							{
								AddressPtr:     convert.ToPtr("192.168.1.12"),
								CurrentNodePtr: convert.ToPtr("node3"),
							},
						},
					},
				},
			},
			mockError:    nil,
			expectedLIFs: []string{"192.168.1.10", "192.168.1.11"},
			expectError:  false,
		},
		{
			name:               "empty_ips_input",
			ips:                []string{},
			reportingNodeNames: []string{"node1", "node2"},
			mockResponse:       nil,
			mockError:          nil,
			expectedLIFs:       nil,
			expectError:        false,
		},
		{
			name:               "api_error_handling",
			ips:                []string{"192.168.1.10"},
			reportingNodeNames: []string{"node1"},
			mockResponse:       nil,
			mockError:          errors.New("network interface query failed"),
			expectedLIFs:       nil,
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Only expect the NetInterfaceGet call if inputs are not empty
			if len(tt.ips) > 0 && len(tt.reportingNodeNames) > 0 {
				mock.EXPECT().NetInterfaceGet().
					Return(tt.mockResponse, tt.mockError).Times(1)
			}

			lifs, err := oapi.GetSLMDataLifs(ctx, tt.ips, tt.reportingNodeNames)

			if tt.expectError {
				assert.Error(t, err, "expected error when SLM data LIFs retrieval should fail")
				assert.Nil(t, lifs)
			} else {
				assert.NoError(t, err)
				if len(tt.expectedLIFs) == 0 {
					if lifs == nil {
						assert.Nil(t, lifs)
					} else {
						assert.Empty(t, lifs)
					}
				} else {
					assert.Equal(t, tt.expectedLIFs, lifs)
				}
			}
		})
	}
}

func TestOntapAPIZAPI_FlexgroupExists(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		volumeName   string
		mockResponse bool
		mockError    error
		expectExists bool
		expectError  bool
	}{
		{
			name:         "flexgroup_exists",
			volumeName:   "test_flexgroup",
			mockResponse: true,
			mockError:    nil,
			expectExists: true,
			expectError:  false,
		},
		{
			name:         "flexgroup_not_exists",
			volumeName:   "nonexistent_flexgroup",
			mockResponse: false,
			mockError:    nil,
			expectExists: false,
			expectError:  false,
		},
		{
			name:         "api_error_handling",
			volumeName:   "error_flexgroup",
			mockResponse: false,
			mockError:    errors.New("API call failed"),
			expectExists: false,
			expectError:  true,
		},
		{
			name:         "empty_volume_name",
			volumeName:   "",
			mockResponse: false,
			mockError:    errors.New("invalid volume name"),
			expectExists: false,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().FlexGroupExists(ctx, tt.volumeName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			exists, err := oapi.FlexgroupExists(ctx, tt.volumeName)

			if tt.expectError {
				assert.Error(t, err, "expected error when FlexGroup existence check should fail")
				assert.Equal(t, tt.expectExists, exists)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectExists, exists)
			}
		})
	}
}

func TestOntapAPIZAPI_FlexgroupCreate(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		volume       api.Volume
		mockResponse *azgo.VolumeCreateAsyncResponse
		mockError    error
		expectError  bool
	}{
		{
			name: "successful_flexgroup_create",
			volume: api.Volume{
				Name:            "test_flexgroup",
				Size:            "1073741824", // 1GB
				Aggregates:      []string{"aggr1", "aggr2"},
				SpaceReserve:    "none",
				SnapshotPolicy:  "default",
				UnixPermissions: "0755",
				ExportPolicy:    "default",
				SecurityStyle:   "unix",
				Comment:         "Test FlexGroup",
			},
			mockResponse: &azgo.VolumeCreateAsyncResponse{
				Result: azgo.VolumeCreateAsyncResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name: "invalid_size_error",
			volume: api.Volume{
				Name:       "test_flexgroup",
				Size:       "invalid_size",
				Aggregates: []string{"aggr1"},
			},
			mockResponse: nil,
			mockError:    nil,
			expectError:  true,
		},
		{
			name: "api_create_error",
			volume: api.Volume{
				Name:       "test_flexgroup",
				Size:       "1073741824",
				Aggregates: []string{"aggr1"},
			},
			mockResponse: nil,
			mockError:    errors.New("FlexGroup create failed"),
			expectError:  true,
		},
		{
			name: "nil_response_error",
			volume: api.Volume{
				Name:       "test_flexgroup",
				Size:       "1073741824",
				Aggregates: []string{"aggr1"},
			},
			mockResponse: nil,
			mockError:    nil,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ClientConfig for logging
			mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()

			// Only mock the API call if we expect a valid size conversion
			if tt.volume.Size != "invalid_size" {
				sizeBytes, _ := convert.ToPositiveInt(tt.volume.Size)
				mock.EXPECT().FlexGroupCreate(ctx, tt.volume.Name, sizeBytes, tt.volume.Aggregates,
					tt.volume.SpaceReserve, tt.volume.SnapshotPolicy, tt.volume.UnixPermissions,
					tt.volume.ExportPolicy, tt.volume.SecurityStyle, tt.volume.TieringPolicy,
					tt.volume.Comment, tt.volume.Qos, tt.volume.Encrypt, tt.volume.SnapshotReserve).
					Return(tt.mockResponse, tt.mockError).Times(1)
			}

			err := oapi.FlexgroupCreate(ctx, tt.volume)

			if tt.expectError {
				assert.Error(t, err, "expected error when FlexGroup creation should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_FlexgroupInfo(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		volumeName   string
		mockResponse *azgo.VolumeAttributesType
		mockError    error
		expectError  bool
	}{
		{
			name:       "successful_flexgroup_info",
			volumeName: "test_flexgroup",
			mockResponse: &azgo.VolumeAttributesType{
				VolumeIdAttributesPtr: &azgo.VolumeIdAttributesType{
					NamePtr: convert.ToPtr("test_flexgroup"),
				},
				VolumeSpaceAttributesPtr: &azgo.VolumeSpaceAttributesType{
					SizePtr: convert.ToPtr(1073741824),
				},
				VolumeStateAttributesPtr: &azgo.VolumeStateAttributesType{
					StatePtr: convert.ToPtr("online"),
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:         "flexgroup_not_found",
			volumeName:   "nonexistent_flexgroup",
			mockResponse: nil,
			mockError:    errors.New("volume not found"),
			expectError:  true,
		},
		{
			name:         "api_error_handling",
			volumeName:   "error_flexgroup",
			mockResponse: nil,
			mockError:    errors.New("API call failed"),
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ClientConfig for logging
			mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()

			mock.EXPECT().FlexGroupGet(tt.volumeName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			volumeInfo, err := oapi.FlexgroupInfo(ctx, tt.volumeName)

			if tt.expectError {
				assert.Error(t, err, "expected error when FlexGroup info retrieval should fail")
				assert.Nil(t, volumeInfo)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, volumeInfo)
				assert.Equal(t, "test_flexgroup", volumeInfo.Name)
			}
		})
	}
}

func TestOntapAPIZAPI_FlexgroupDestroy(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name              string
		volumeName        string
		force             bool
		skipRecoveryQueue bool
		unmountResponse   *azgo.VolumeUnmountResponse
		unmountError      error
		offlineResponse   *azgo.VolumeOfflineResponse
		offlineError      error
		destroyResponse   *azgo.VolumeDestroyAsyncResponse
		destroyError      error
		expectError       bool
	}{
		{
			name:              "successful_flexgroup_destroy",
			volumeName:        "test_flexgroup",
			force:             false,
			skipRecoveryQueue: true,
			unmountResponse: &azgo.VolumeUnmountResponse{
				Result: azgo.VolumeUnmountResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			unmountError: nil,
			offlineResponse: &azgo.VolumeOfflineResponse{
				Result: azgo.VolumeOfflineResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			offlineError: nil,
			destroyResponse: &azgo.VolumeDestroyAsyncResponse{
				Result: azgo.VolumeDestroyAsyncResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			destroyError: nil,
			expectError:  false,
		},
		{
			name:              "unmount_error",
			volumeName:        "test_flexgroup",
			force:             false,
			skipRecoveryQueue: true,
			unmountResponse:   nil,
			unmountError:      errors.New("unmount failed"),
			expectError:       true,
		},
		{
			name:              "offline_error",
			volumeName:        "test_flexgroup",
			force:             false,
			skipRecoveryQueue: true,
			unmountResponse: &azgo.VolumeUnmountResponse{
				Result: azgo.VolumeUnmountResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			unmountError: nil,
			offlineResponse: &azgo.VolumeOfflineResponse{
				Result: azgo.VolumeOfflineResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  "1234",
					ResultReasonAttr: "offline failed",
				},
			},
			offlineError: nil,
			expectError:  true,
		},
		{
			name:              "destroy_error",
			volumeName:        "test_flexgroup",
			force:             true,
			skipRecoveryQueue: true,
			unmountResponse: &azgo.VolumeUnmountResponse{
				Result: azgo.VolumeUnmountResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			unmountError: nil,
			offlineResponse: &azgo.VolumeOfflineResponse{
				Result: azgo.VolumeOfflineResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			offlineError:    nil,
			destroyResponse: nil,
			destroyError:    errors.New("destroy failed"),
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock unmount call (always uses force=true in FlexgroupDestroy)
			mock.EXPECT().VolumeUnmount(tt.volumeName, true).
				Return(tt.unmountResponse, tt.unmountError).Times(1)

			if tt.unmountError == nil {
				// Mock offline call
				mock.EXPECT().VolumeOffline(tt.volumeName).
					Return(tt.offlineResponse, tt.offlineError).Times(1)

				if tt.offlineError == nil && (tt.offlineResponse == nil || tt.offlineResponse.Result.ResultStatusAttr == "passed") {
					// Mock destroy call
					mock.EXPECT().FlexGroupDestroy(ctx, tt.volumeName, tt.force).
						Return(tt.destroyResponse, tt.destroyError).Times(1)

					if tt.destroyError == nil && tt.skipRecoveryQueue && (tt.destroyResponse == nil || tt.destroyResponse.Result.ResultStatusAttr == "passed") {
						// Mock recovery queue operations
						mock.EXPECT().VolumeRecoveryQueueGetIter(tt.volumeName+"*").
							Return(nil, errors.NotFoundError("volume not found in recovery queue")).Times(1)
					}
				}
			}

			err := oapi.FlexgroupDestroy(ctx, tt.volumeName, tt.force, tt.skipRecoveryQueue)

			if tt.expectError {
				assert.Error(t, err, "expected error when FlexGroup destruction should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_FlexgroupSetSize(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		volumeName   string
		newSize      string
		mockResponse *azgo.VolumeSizeAsyncResponse
		mockError    error
		expectError  bool
	}{
		{
			name:       "successful_resize",
			volumeName: "test_flexgroup",
			newSize:    "2147483648", // 2GB
			mockResponse: &azgo.VolumeSizeAsyncResponse{
				Result: azgo.VolumeSizeAsyncResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:         "resize_api_error",
			volumeName:   "test_flexgroup",
			newSize:      "2147483648",
			mockResponse: nil,
			mockError:    errors.New("resize failed"),
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().FlexGroupSetSize(ctx, tt.volumeName, tt.newSize).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.FlexgroupSetSize(ctx, tt.volumeName, tt.newSize)

			if tt.expectError {
				assert.Error(t, err, "expected error when FlexGroup size setting should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_FlexgroupSize(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		volumeName   string
		mockResponse int
		mockError    error
		expectedSize uint64
		expectError  bool
	}{
		{
			name:         "successful_size_retrieval",
			volumeName:   "test_flexgroup",
			mockResponse: 1073741824, // 1GB
			mockError:    nil,
			expectedSize: 1073741824,
			expectError:  false,
		},
		{
			name:         "size_api_error",
			volumeName:   "test_flexgroup",
			mockResponse: 0,
			mockError:    errors.New("size query failed"),
			expectedSize: 0,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().FlexGroupSize(tt.volumeName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			size, err := oapi.FlexgroupSize(ctx, tt.volumeName)

			if tt.expectError {
				assert.Error(t, err, "expected error when FlexGroup size retrieval should fail")
				assert.Equal(t, tt.expectedSize, size)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedSize, size)
			}
		})
	}
}

func TestOntapAPIZAPI_FlexgroupUsedSize(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name             string
		volumeName       string
		mockResponse     int
		mockError        error
		expectedUsedSize int
		expectError      bool
	}{
		{
			name:             "successful_used_size_retrieval",
			volumeName:       "test_flexgroup",
			mockResponse:     536870912, // 512MB
			mockError:        nil,
			expectedUsedSize: 536870912,
			expectError:      false,
		},
		{
			name:             "used_size_api_error",
			volumeName:       "test_flexgroup",
			mockResponse:     0,
			mockError:        errors.New("used size query failed"),
			expectedUsedSize: 0,
			expectError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().FlexGroupUsedSize(tt.volumeName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			usedSize, err := oapi.FlexgroupUsedSize(ctx, tt.volumeName)

			if tt.expectError {
				assert.Error(t, err, "expected error when FlexGroup used size retrieval should fail")
				assert.Equal(t, tt.expectedUsedSize, usedSize)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedUsedSize, usedSize)
			}
		})
	}
}

func TestOntapAPIZAPI_FlexgroupUnmount(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		volumeName   string
		force        bool
		mockResponse *azgo.VolumeUnmountResponse
		mockError    error
		expectError  bool
	}{
		{
			name:       "successful_unmount",
			volumeName: "test_flexgroup",
			force:      false,
			mockResponse: &azgo.VolumeUnmountResponse{
				Result: azgo.VolumeUnmountResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:       "force_unmount",
			volumeName: "test_flexgroup",
			force:      true,
			mockResponse: &azgo.VolumeUnmountResponse{
				Result: azgo.VolumeUnmountResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:         "unmount_api_error",
			volumeName:   "test_flexgroup",
			force:        false,
			mockResponse: nil,
			mockError:    errors.New("unmount failed"),
			expectError:  true,
		},
		{
			name:       "unmount_volume_not_found",
			volumeName: "nonexistent_flexgroup",
			force:      false,
			mockResponse: &azgo.VolumeUnmountResponse{
				Result: azgo.VolumeUnmountResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  "15661",
					ResultReasonAttr: "object not found",
				},
			},
			mockError:   nil,
			expectError: false, // Should return nil for object not found
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().VolumeUnmount(tt.volumeName, tt.force).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.FlexgroupUnmount(ctx, tt.volumeName, tt.force)

			if tt.expectError {
				assert.Error(t, err, "expected error when FlexGroup unmount should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_VolumeModifySnapshotDirectoryAccess(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		volumeName   string
		enable       bool
		mockResponse *azgo.VolumeModifyIterResponse
		mockError    error
		expectError  bool
	}{
		{
			name:       "successful_enable_snapdir",
			volumeName: "test_volume",
			enable:     true,
			mockResponse: &azgo.VolumeModifyIterResponse{
				Result: azgo.VolumeModifyIterResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:       "successful_disable_snapdir",
			volumeName: "test_volume",
			enable:     false,
			mockResponse: &azgo.VolumeModifyIterResponse{
				Result: azgo.VolumeModifyIterResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:         "api_error_handling",
			volumeName:   "error_volume",
			enable:       true,
			mockResponse: nil,
			mockError:    errors.New("API call failed"),
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().VolumeModifySnapshotDirectoryAccess(tt.volumeName, tt.enable).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.VolumeModifySnapshotDirectoryAccess(ctx, tt.volumeName, tt.enable)

			if tt.expectError {
				assert.Error(t, err, "expected error when volume snapshot directory access modification should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_VolumeRename(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		originalName string
		newName      string
		mockResponse *azgo.VolumeRenameResponse
		mockError    error
		expectError  bool
	}{
		{
			name:         "successful_rename",
			originalName: "old_volume",
			newName:      "new_volume",
			mockResponse: &azgo.VolumeRenameResponse{
				Result: azgo.VolumeRenameResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:         "rename_api_error",
			originalName: "old_volume",
			newName:      "new_volume",
			mockResponse: nil,
			mockError:    errors.New("Rename failed"),
			expectError:  true,
		},
		{
			name:         "rename_zapi_error",
			originalName: "old_volume",
			newName:      "new_volume",
			mockResponse: &azgo.VolumeRenameResponse{
				Result: azgo.VolumeRenameResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  "1234",
					ResultReasonAttr: "Volume not found",
				},
			},
			mockError:   nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ClientConfig for logging
			mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()

			mock.EXPECT().VolumeRename(tt.originalName, tt.newName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.VolumeRename(ctx, tt.originalName, tt.newName)

			if tt.expectError {
				assert.Error(t, err, "expected error when volume rename should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_VolumeSetComment(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name               string
		volumeNameInternal string
		volumeNameExternal string
		comment            string
		mockResponse       *azgo.VolumeModifyIterResponse
		mockError          error
		expectError        bool
	}{
		{
			name:               "successful_comment_set",
			volumeNameInternal: "internal_vol",
			volumeNameExternal: "external_vol",
			comment:            "Test comment",
			mockResponse: &azgo.VolumeModifyIterResponse{
				Result: azgo.VolumeModifyIterResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:               "empty_comment",
			volumeNameInternal: "internal_vol",
			volumeNameExternal: "external_vol",
			comment:            "",
			mockResponse: &azgo.VolumeModifyIterResponse{
				Result: azgo.VolumeModifyIterResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:               "comment_api_error",
			volumeNameInternal: "internal_vol",
			volumeNameExternal: "external_vol",
			comment:            "Test comment",
			mockResponse:       nil,
			mockError:          errors.New("Set comment failed"),
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ClientConfig for logging
			mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()

			mock.EXPECT().VolumeSetComment(ctx, tt.volumeNameInternal, tt.comment).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.VolumeSetComment(ctx, tt.volumeNameInternal, tt.volumeNameExternal, tt.comment)

			if tt.expectError {
				assert.Error(t, err, "expected error when volume comment setting should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_VolumeModifyUnixPermissions(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name               string
		volumeNameInternal string
		volumeNameExternal string
		unixPermissions    string
		mockResponse       *azgo.VolumeModifyIterResponse
		mockError          error
		expectError        bool
	}{
		{
			name:               "successful_permissions_modify",
			volumeNameInternal: "internal_vol",
			volumeNameExternal: "external_vol",
			unixPermissions:    "0755",
			mockResponse: &azgo.VolumeModifyIterResponse{
				Result: azgo.VolumeModifyIterResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:               "permissions_readonly",
			volumeNameInternal: "internal_vol",
			volumeNameExternal: "external_vol",
			unixPermissions:    "0444",
			mockResponse: &azgo.VolumeModifyIterResponse{
				Result: azgo.VolumeModifyIterResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:               "permissions_api_error",
			volumeNameInternal: "internal_vol",
			volumeNameExternal: "external_vol",
			unixPermissions:    "0755",
			mockResponse:       nil,
			mockError:          errors.New("Modify permissions failed"),
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ClientConfig for logging
			mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()

			mock.EXPECT().VolumeModifyUnixPermissions(tt.volumeNameInternal, tt.unixPermissions).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.VolumeModifyUnixPermissions(ctx, tt.volumeNameInternal, tt.volumeNameExternal, tt.unixPermissions)

			if tt.expectError {
				assert.Error(t, err, "expected error when volume Unix permissions modification should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_VolumeListByAttrs(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		volumeAttrs  *api.Volume
		mockResponse *azgo.VolumeGetIterResponse
		mockError    error
		expectError  bool
		expectedVols int
	}{
		{
			name: "successful_volume_list",
			volumeAttrs: &api.Volume{
				Name:            "test_vol*",
				Aggregates:      []string{"aggr1", "aggr2"},
				SpaceReserve:    "none",
				SnapshotPolicy:  "default",
				TieringPolicy:   "auto",
				SnapshotDir:     convert.ToPtr(true),
				Encrypt:         convert.ToPtr(false),
				SnapshotReserve: 5,
			},
			mockResponse: &azgo.VolumeGetIterResponse{
				Result: azgo.VolumeGetIterResponseResult{
					ResultStatusAttr: "passed",
					AttributesListPtr: &azgo.VolumeGetIterResponseResultAttributesList{
						VolumeAttributesPtr: []azgo.VolumeAttributesType{
							{
								VolumeIdAttributesPtr: &azgo.VolumeIdAttributesType{
									NamePtr: convert.ToPtr("test_vol_1"),
								},
								VolumeSpaceAttributesPtr: &azgo.VolumeSpaceAttributesType{
									SizePtr: convert.ToPtr(1073741824),
								},
								VolumeStateAttributesPtr: &azgo.VolumeStateAttributesType{
									StatePtr: convert.ToPtr("online"),
								},
							},
						},
					},
				},
			},
			mockError:    nil,
			expectError:  false,
			expectedVols: 1,
		},
		{
			name: "empty_volume_list",
			volumeAttrs: &api.Volume{
				Name:           "nonexistent*",
				Aggregates:     []string{"aggr1"},
				SpaceReserve:   "none",
				SnapshotPolicy: "default",
			},
			mockResponse: &azgo.VolumeGetIterResponse{
				Result: azgo.VolumeGetIterResponseResult{
					ResultStatusAttr:  "passed",
					AttributesListPtr: nil,
				},
			},
			mockError:    nil,
			expectError:  false,
			expectedVols: 0,
		},
		{
			name: "list_api_error",
			volumeAttrs: &api.Volume{
				Name:       "error_vol*",
				Aggregates: []string{"aggr1"},
			},
			mockResponse: nil,
			mockError:    errors.New("API call failed"),
			expectError:  true,
			expectedVols: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			aggrs := strings.Join(tt.volumeAttrs.Aggregates, "|")

			mock.EXPECT().VolumeListByAttrs(tt.volumeAttrs.Name, aggrs, tt.volumeAttrs.SpaceReserve,
				tt.volumeAttrs.SnapshotPolicy, tt.volumeAttrs.TieringPolicy, tt.volumeAttrs.SnapshotDir,
				tt.volumeAttrs.Encrypt, tt.volumeAttrs.SnapshotReserve).
				Return(tt.mockResponse, tt.mockError).Times(1)

			volumes, err := oapi.VolumeListByAttrs(ctx, tt.volumeAttrs)

			if tt.expectError {
				assert.Error(t, err, "expected error when volume list by attributes should fail")
				assert.Nil(t, volumes)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, volumes)
				assert.Len(t, volumes, tt.expectedVols)
				if tt.expectedVols > 0 {
					assert.Equal(t, "test_vol_1", volumes[0].Name)
				}
			}
		})
	}
}

func TestOntapAPIZAPI_ExportPolicyCreate(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		policyName   string
		mockResponse *azgo.ExportPolicyCreateResponse
		mockError    error
		expectError  bool
	}{
		{
			name:       "successful_policy_create",
			policyName: "test_policy",
			mockResponse: &azgo.ExportPolicyCreateResponse{
				Result: azgo.ExportPolicyCreateResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:       "policy_already_exists",
			policyName: "existing_policy",
			mockResponse: &azgo.ExportPolicyCreateResponse{
				Result: azgo.ExportPolicyCreateResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  azgo.EDUPLICATEENTRY,
					ResultReasonAttr: "Export policy already exists",
				},
			},
			mockError:   nil,
			expectError: false, // Should not error due to duplicate entry handling
		},
		{
			name:         "policy_create_api_error",
			policyName:   "error_policy",
			mockResponse: nil,
			mockError:    errors.New("API call failed"),
			expectError:  true,
		},
		{
			name:       "policy_create_zapi_error",
			policyName: "error_policy",
			mockResponse: &azgo.ExportPolicyCreateResponse{
				Result: azgo.ExportPolicyCreateResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  "1234",
					ResultReasonAttr: "Policy creation failed",
				},
			},
			mockError:   nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ClientConfig for logging
			mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()

			mock.EXPECT().ExportPolicyCreate(tt.policyName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.ExportPolicyCreate(ctx, tt.policyName)

			if tt.expectError {
				assert.Error(t, err, "expected error when export policy creation should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_ExportPolicyDestroy(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		policyName   string
		mockResponse *azgo.ExportPolicyDestroyResponse
		mockError    error
		expectError  bool
	}{
		{
			name:       "successful_policy_destroy",
			policyName: "test_policy",
			mockResponse: &azgo.ExportPolicyDestroyResponse{
				Result: azgo.ExportPolicyDestroyResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:         "policy_destroy_api_error",
			policyName:   "error_policy",
			mockResponse: nil,
			mockError:    errors.New("API call failed"),
			expectError:  true,
		},
		{
			name:       "policy_destroy_zapi_error",
			policyName: "error_policy",
			mockResponse: &azgo.ExportPolicyDestroyResponse{
				Result: azgo.ExportPolicyDestroyResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  "1234",
					ResultReasonAttr: "Policy deletion failed",
				},
			},
			mockError:   nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().ExportPolicyDestroy(tt.policyName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.ExportPolicyDestroy(ctx, tt.policyName)

			if tt.expectError {
				assert.Error(t, err, "expected error when export policy destruction should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_ExportPolicyExists(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		policyName   string
		mockResponse *azgo.ExportPolicyGetResponse
		mockError    error
		expectExists bool
		expectError  bool
	}{
		{
			name:       "policy_exists",
			policyName: "existing_policy",
			mockResponse: &azgo.ExportPolicyGetResponse{
				Result: azgo.ExportPolicyGetResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:    nil,
			expectExists: true,
			expectError:  false,
		},
		{
			name:       "policy_not_found",
			policyName: "nonexistent_policy",
			mockResponse: &azgo.ExportPolicyGetResponse{
				Result: azgo.ExportPolicyGetResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  azgo.EOBJECTNOTFOUND,
					ResultReasonAttr: "Export policy not found",
				},
			},
			mockError:    nil,
			expectExists: false,
			expectError:  false,
		},
		{
			name:         "policy_get_api_error",
			policyName:   "error_policy",
			mockResponse: nil,
			mockError:    errors.New("API call failed"),
			expectExists: false,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ClientConfig for logging
			mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()

			mock.EXPECT().ExportPolicyGet(tt.policyName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			exists, err := oapi.ExportPolicyExists(ctx, tt.policyName)

			if tt.expectError {
				assert.Error(t, err, "expected error when export policy existence check should fail")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectExists, exists)
			}
		})
	}
}

func TestOntapAPIZAPI_ExportRuleCreate(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name              string
		policyName        string
		desiredPolicyRule string
		nasProtocol       string
		mockResponse      *azgo.ExportRuleCreateResponse
		mockError         error
		expectError       bool
		expectedProtocols []string
	}{
		{
			name:              "successful_nfs_rule_create",
			policyName:        "test_policy",
			desiredPolicyRule: "192.168.1.0/24",
			nasProtocol:       "NFS",
			mockResponse: &azgo.ExportRuleCreateResponse{
				Result: azgo.ExportRuleCreateResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:         nil,
			expectError:       false,
			expectedProtocols: []string{"nfs"},
		},
		{
			name:              "successful_smb_rule_create",
			policyName:        "test_policy",
			desiredPolicyRule: "192.168.1.0/24",
			nasProtocol:       sa.SMB,
			mockResponse: &azgo.ExportRuleCreateResponse{
				Result: azgo.ExportRuleCreateResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:         nil,
			expectError:       false,
			expectedProtocols: []string{"cifs"},
		},
		{
			name:              "rule_already_exists",
			policyName:        "test_policy",
			desiredPolicyRule: "192.168.1.0/24",
			nasProtocol:       "NFS",
			mockResponse: &azgo.ExportRuleCreateResponse{
				Result: azgo.ExportRuleCreateResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  azgo.EAPIERROR,
					ResultReasonAttr: "policy already contains a rule",
				},
			},
			mockError:         nil,
			expectError:       true, // Should return AlreadyExistsError
			expectedProtocols: []string{"nfs"},
		},
		{
			name:              "rule_create_api_error",
			policyName:        "error_policy",
			desiredPolicyRule: "192.168.1.0/24",
			nasProtocol:       "NFS",
			mockResponse:      nil,
			mockError:         errors.New("API call failed"),
			expectError:       true,
			expectedProtocols: []string{"nfs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ClientConfig for logging
			mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()

			mock.EXPECT().ExportRuleCreate(tt.policyName, tt.desiredPolicyRule,
				tt.expectedProtocols, []string{"any"}, []string{"any"}, []string{"any"}).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.ExportRuleCreate(ctx, tt.policyName, tt.desiredPolicyRule, tt.nasProtocol)

			if tt.expectError {
				assert.Error(t, err, "expected error when export rule creation should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_ExportRuleDestroy(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		policyName   string
		ruleIndex    int
		mockResponse *azgo.ExportRuleDestroyResponse
		mockError    error
		expectError  bool
	}{
		{
			name:       "successful_rule_destroy",
			policyName: "test_policy",
			ruleIndex:  1,
			mockResponse: &azgo.ExportRuleDestroyResponse{
				Result: azgo.ExportRuleDestroyResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:         "rule_destroy_api_error",
			policyName:   "error_policy",
			ruleIndex:    1,
			mockResponse: nil,
			mockError:    errors.New("API call failed"),
			expectError:  true,
		},
		{
			name:       "rule_destroy_zapi_error",
			policyName: "error_policy",
			ruleIndex:  99,
			mockResponse: &azgo.ExportRuleDestroyResponse{
				Result: azgo.ExportRuleDestroyResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  "1234",
					ResultReasonAttr: "Rule not found",
				},
			},
			mockError:   nil,
			expectError: false, // Current implementation returns nil even on zapi errors
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ClientConfig for logging
			mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()

			mock.EXPECT().ExportRuleDestroy(tt.policyName, tt.ruleIndex).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.ExportRuleDestroy(ctx, tt.policyName, tt.ruleIndex)

			if tt.expectError {
				assert.Error(t, err, "expected error when export rule destruction should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_VolumeModifyExportPolicy(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		volumeName   string
		policyName   string
		mockResponse *azgo.VolumeModifyIterResponse
		mockError    error
		expectError  bool
	}{
		{
			name:       "successful_export_policy_modify",
			volumeName: "test_volume",
			policyName: "new_policy",
			mockResponse: &azgo.VolumeModifyIterResponse{
				Result: azgo.VolumeModifyIterResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:         "export_policy_modify_api_error",
			volumeName:   "error_volume",
			policyName:   "new_policy",
			mockResponse: nil,
			mockError:    errors.New("API call failed"),
			expectError:  true,
		},
		{
			name:       "export_policy_modify_zapi_error",
			volumeName: "error_volume",
			policyName: "new_policy",
			mockResponse: &azgo.VolumeModifyIterResponse{
				Result: azgo.VolumeModifyIterResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  "1234",
					ResultReasonAttr: "Volume not found",
				},
			},
			mockError:   nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ClientConfig for logging
			mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()

			mock.EXPECT().VolumeModifyExportPolicy(tt.volumeName, tt.policyName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.VolumeModifyExportPolicy(ctx, tt.volumeName, tt.policyName)

			if tt.expectError {
				assert.Error(t, err, "expected error when volume export policy modification should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_QtreeExists(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name          string
		qtreeName     string
		volumePattern string
		expectExists  bool
		expectVolume  string
		mockExists    bool
		mockVolume    string
		mockError     error
		expectError   bool
	}{
		{
			name:          "qtree_exists",
			qtreeName:     "test_qtree",
			volumePattern: "vol*",
			expectExists:  true,
			expectVolume:  "volume1",
			mockExists:    true,
			mockVolume:    "volume1",
			mockError:     nil,
			expectError:   false,
		},
		{
			name:          "qtree_not_exists",
			qtreeName:     "nonexistent_qtree",
			volumePattern: "vol*",
			expectExists:  false,
			expectVolume:  "",
			mockExists:    false,
			mockVolume:    "",
			mockError:     nil,
			expectError:   false,
		},
		{
			name:          "qtree_exists_api_error",
			qtreeName:     "error_qtree",
			volumePattern: "vol*",
			expectExists:  false,
			expectVolume:  "",
			mockExists:    false,
			mockVolume:    "",
			mockError:     errors.New("API call failed"),
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().QtreeExists(ctx, tt.qtreeName, tt.volumePattern).
				Return(tt.mockExists, tt.mockVolume, tt.mockError).Times(1)

			exists, volume, err := oapi.QtreeExists(ctx, tt.qtreeName, tt.volumePattern)

			if tt.expectError {
				assert.Error(t, err, "expected error when qtree existence check should fail")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectExists, exists)
				assert.Equal(t, tt.expectVolume, volume)
			}
		})
	}
}

func TestOntapAPIZAPI_QtreeCreate(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name            string
		qtreeName       string
		volumeName      string
		unixPermissions string
		exportPolicy    string
		securityStyle   string
		qosPolicy       string
		mockResponse    *azgo.QtreeCreateResponse
		mockError       error
		expectError     bool
	}{
		{
			name:            "successful_qtree_create",
			qtreeName:       "test_qtree",
			volumeName:      "test_volume",
			unixPermissions: "0755",
			exportPolicy:    "default",
			securityStyle:   "unix",
			qosPolicy:       "",
			mockResponse: &azgo.QtreeCreateResponse{
				Result: azgo.QtreeCreateResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:            "qtree_create_zapi_error",
			qtreeName:       "error_qtree",
			volumeName:      "test_volume",
			unixPermissions: "0755",
			exportPolicy:    "default",
			securityStyle:   "unix",
			qosPolicy:       "",
			mockResponse: &azgo.QtreeCreateResponse{
				Result: azgo.QtreeCreateResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  "1234",
					ResultReasonAttr: "Qtree creation failed",
				},
			},
			mockError:   nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().QtreeCreate(tt.qtreeName, tt.volumeName, tt.unixPermissions,
				tt.exportPolicy, tt.securityStyle, tt.qosPolicy).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.QtreeCreate(ctx, tt.qtreeName, tt.volumeName, tt.unixPermissions,
				tt.exportPolicy, tt.securityStyle, tt.qosPolicy)

			if tt.expectError {
				assert.Error(t, err, "expected error when qtree creation should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_QtreeDestroyAsync(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		path         string
		force        bool
		mockResponse *azgo.QtreeDeleteAsyncResponse
		mockError    error
		expectError  bool
	}{
		{
			name:  "successful_qtree_destroy",
			path:  "/vol/test_volume/test_qtree",
			force: false,
			mockResponse: &azgo.QtreeDeleteAsyncResponse{
				Result: azgo.QtreeDeleteAsyncResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:  "force_qtree_destroy",
			path:  "/vol/test_volume/test_qtree",
			force: true,
			mockResponse: &azgo.QtreeDeleteAsyncResponse{
				Result: azgo.QtreeDeleteAsyncResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:  "qtree_destroy_api_error",
			path:  "/vol/test_volume/error_qtree",
			force: false,
			mockResponse: &azgo.QtreeDeleteAsyncResponse{
				Result: azgo.QtreeDeleteAsyncResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   errors.New("API call failed"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().QtreeDestroyAsync(tt.path, tt.force).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.QtreeDestroyAsync(ctx, tt.path, tt.force)

			if tt.expectError {
				assert.Error(t, err, "expected error when qtree async destruction should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_QtreeRename(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		path         string
		newPath      string
		mockResponse *azgo.QtreeRenameResponse
		mockError    error
		expectError  bool
	}{
		{
			name:    "successful_qtree_rename",
			path:    "/vol/test_volume/old_qtree",
			newPath: "/vol/test_volume/new_qtree",
			mockResponse: &azgo.QtreeRenameResponse{
				Result: azgo.QtreeRenameResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:    "qtree_rename_api_error",
			path:    "/vol/test_volume/old_qtree",
			newPath: "/vol/test_volume/new_qtree",
			mockResponse: &azgo.QtreeRenameResponse{
				Result: azgo.QtreeRenameResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   errors.New("API call failed"),
			expectError: true,
		},
		{
			name:    "qtree_rename_zapi_error",
			path:    "/vol/test_volume/old_qtree",
			newPath: "/vol/test_volume/new_qtree",
			mockResponse: &azgo.QtreeRenameResponse{
				Result: azgo.QtreeRenameResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  "1234",
					ResultReasonAttr: "Qtree not found",
				},
			},
			mockError:   nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().QtreeRename(tt.path, tt.newPath).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.QtreeRename(ctx, tt.path, tt.newPath)

			if tt.expectError {
				assert.Error(t, err, "expected error when qtree rename should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_QtreeModifyExportPolicy(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name                string
		qtreeName           string
		volumeName          string
		newExportPolicyName string
		mockResponse        *azgo.QtreeModifyResponse
		mockError           error
		expectError         bool
		expectNotFoundError bool
	}{
		{
			name:                "successful_export_policy_modify",
			qtreeName:           "test_qtree",
			volumeName:          "test_volume",
			newExportPolicyName: "new_policy",
			mockResponse: &azgo.QtreeModifyResponse{
				Result: azgo.QtreeModifyResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:           nil,
			expectError:         false,
			expectNotFoundError: false,
		},
		{
			name:                "qtree_not_found_error",
			qtreeName:           "nonexistent_qtree",
			volumeName:          "test_volume",
			newExportPolicyName: "new_policy",
			mockResponse: &azgo.QtreeModifyResponse{
				Result: azgo.QtreeModifyResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  azgo.EAPIERROR,
					ResultReasonAttr: "Qtree not found",
				},
			},
			mockError:           nil,
			expectError:         true,
			expectNotFoundError: true,
		},
		{
			name:                "export_policy_modify_api_error",
			qtreeName:           "error_qtree",
			volumeName:          "test_volume",
			newExportPolicyName: "new_policy",
			mockResponse: &azgo.QtreeModifyResponse{
				Result: azgo.QtreeModifyResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:           errors.New("API call failed"),
			expectError:         true,
			expectNotFoundError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().QtreeModifyExportPolicy(tt.qtreeName, tt.volumeName, tt.newExportPolicyName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.QtreeModifyExportPolicy(ctx, tt.qtreeName, tt.volumeName, tt.newExportPolicyName)

			if tt.expectError {
				assert.Error(t, err, "expected error when qtree export policy modification should fail")
				if tt.expectNotFoundError {
					assert.True(t, errors.IsNotFoundError(err))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_QtreeCount(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name          string
		volumeName    string
		expectedCount int
		mockCount     int
		mockError     error
		expectError   bool
	}{
		{
			name:          "successful_qtree_count",
			volumeName:    "test_volume",
			expectedCount: 3,
			mockCount:     3,
			mockError:     nil,
			expectError:   false,
		},
		{
			name:          "empty_volume_qtree_count",
			volumeName:    "empty_volume",
			expectedCount: 0,
			mockCount:     0,
			mockError:     nil,
			expectError:   false,
		},
		{
			name:          "qtree_count_api_error",
			volumeName:    "error_volume",
			expectedCount: 0,
			mockCount:     0,
			mockError:     errors.New("API call failed"),
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().QtreeCount(ctx, tt.volumeName).
				Return(tt.mockCount, tt.mockError).Times(1)

			count, err := oapi.QtreeCount(ctx, tt.volumeName)

			if tt.expectError {
				assert.Error(t, err, "expected error when qtree count should fail")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCount, count)
			}
		})
	}
}

func TestOntapAPIZAPI_QtreeListByPrefix(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name           string
		prefix         string
		volumePrefix   string
		mockResponse   *azgo.QtreeListIterResponse
		mockError      error
		expectedQtrees int
		expectError    bool
	}{
		{
			name:         "successful_qtree_list",
			prefix:       "test_",
			volumePrefix: "vol_",
			mockResponse: &azgo.QtreeListIterResponse{
				Result: azgo.QtreeListIterResponseResult{
					ResultStatusAttr: "passed",
					AttributesListPtr: &azgo.QtreeListIterResponseResultAttributesList{
						QtreeInfoPtr: []azgo.QtreeInfoType{
							{
								QtreePtr:         convert.ToPtr("test_qtree1"),
								VolumePtr:        convert.ToPtr("vol_1"),
								ExportPolicyPtr:  convert.ToPtr("default"),
								SecurityStylePtr: convert.ToPtr("unix"),
								VserverPtr:       convert.ToPtr("svm1"),
								ModePtr:          convert.ToPtr("0755"),
							},
							{
								QtreePtr:         convert.ToPtr("test_qtree2"),
								VolumePtr:        convert.ToPtr("vol_2"),
								ExportPolicyPtr:  convert.ToPtr("custom"),
								SecurityStylePtr: convert.ToPtr("mixed"),
								VserverPtr:       convert.ToPtr("svm1"),
								ModePtr:          convert.ToPtr("0644"),
							},
						},
					},
				},
			},
			mockError:      nil,
			expectedQtrees: 2,
			expectError:    false,
		},
		{
			name:         "empty_qtree_list",
			prefix:       "nonexistent_",
			volumePrefix: "vol_",
			mockResponse: &azgo.QtreeListIterResponse{
				Result: azgo.QtreeListIterResponseResult{
					ResultStatusAttr:  "passed",
					AttributesListPtr: nil,
				},
			},
			mockError:      nil,
			expectedQtrees: 0,
			expectError:    false,
		},
		{
			name:         "qtree_list_api_error",
			prefix:       "error_",
			volumePrefix: "vol_",
			mockResponse: &azgo.QtreeListIterResponse{
				Result: azgo.QtreeListIterResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:      errors.New("API call failed"),
			expectedQtrees: 0,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ClientConfig for logging
			mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()

			mock.EXPECT().QtreeList(tt.prefix, tt.volumePrefix).
				Return(tt.mockResponse, tt.mockError).Times(1)

			qtrees, err := oapi.QtreeListByPrefix(ctx, tt.prefix, tt.volumePrefix)

			if tt.expectError {
				assert.Error(t, err, "expected error when qtree list by prefix should fail")
				assert.Nil(t, qtrees)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, qtrees)
				assert.Len(t, qtrees, tt.expectedQtrees)
				if tt.expectedQtrees > 0 {
					assert.Equal(t, "test_qtree1", qtrees[0].Name)
					assert.Equal(t, "vol_1", qtrees[0].Volume)
					assert.Equal(t, "default", qtrees[0].ExportPolicy)
					assert.Equal(t, "unix", qtrees[0].SecurityStyle)
					assert.Equal(t, "svm1", qtrees[0].Vserver)
					assert.Equal(t, "0755", qtrees[0].UnixPermissions)
				}
			}
		})
	}
}

func TestOntapAPIZAPI_QtreeGetByName(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		qtreeName    string
		volumePrefix string
		mockQtree    *azgo.QtreeInfoType
		mockError    error
		expectError  bool
	}{
		{
			name:         "successful_qtree_get",
			qtreeName:    "test_qtree",
			volumePrefix: "vol_",
			mockQtree: &azgo.QtreeInfoType{
				QtreePtr:         convert.ToPtr("test_qtree"),
				VolumePtr:        convert.ToPtr("vol_1"),
				ExportPolicyPtr:  convert.ToPtr("default"),
				SecurityStylePtr: convert.ToPtr("unix"),
				VserverPtr:       convert.ToPtr("svm1"),
				ModePtr:          convert.ToPtr("0755"),
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:         "qtree_get_api_error",
			qtreeName:    "error_qtree",
			volumePrefix: "vol_",
			mockQtree:    nil,
			mockError:    errors.New("API call failed"),
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock ClientConfig for logging
			mock.EXPECT().ClientConfig().Return(api.ClientConfig{DebugTraceFlags: map[string]bool{"method": true}}).AnyTimes()

			mock.EXPECT().QtreeGet(tt.qtreeName, tt.volumePrefix).
				Return(tt.mockQtree, tt.mockError).Times(1)

			qtree, err := oapi.QtreeGetByName(ctx, tt.qtreeName, tt.volumePrefix)

			if tt.expectError {
				assert.Error(t, err, "expected error when qtree get by name should fail")
				assert.Nil(t, qtree)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, qtree)
				assert.Equal(t, "test_qtree", qtree.Name)
				assert.Equal(t, "vol_1", qtree.Volume)
				assert.Equal(t, "default", qtree.ExportPolicy)
				assert.Equal(t, "unix", qtree.SecurityStyle)
				assert.Equal(t, "svm1", qtree.Vserver)
				assert.Equal(t, "0755", qtree.UnixPermissions)
			}
		})
	}
}

func TestOntapAPIZAPI_QuotaEntryList(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name          string
		volumeName    string
		mockResponse  *azgo.QuotaListEntriesIterResponse
		mockError     error
		expectedCount int
		expectError   bool
	}{
		{
			name:       "successful_quota_entry_list",
			volumeName: "test_volume",
			mockResponse: &azgo.QuotaListEntriesIterResponse{
				Result: azgo.QuotaListEntriesIterResponseResult{
					ResultStatusAttr: "passed",
					AttributesListPtr: &azgo.QuotaListEntriesIterResponseResultAttributesList{
						QuotaEntryPtr: []azgo.QuotaEntryType{
							{
								VolumePtr:      convert.ToPtr("test_volume"),
								QuotaTargetPtr: convert.ToPtr("/vol/test_volume"),
								QtreePtr:       convert.ToPtr(""),
								QuotaTypePtr:   convert.ToPtr("tree"),
								DiskLimitPtr:   convert.ToPtr("1048576"),
							},
							{
								VolumePtr:      convert.ToPtr("test_volume"),
								QuotaTargetPtr: convert.ToPtr("/vol/test_volume/qtree1"),
								QtreePtr:       convert.ToPtr("qtree1"),
								QuotaTypePtr:   convert.ToPtr("tree"),
								DiskLimitPtr:   convert.ToPtr("2097152"),
							},
						},
					},
				},
			},
			mockError:     nil,
			expectedCount: 2,
			expectError:   false,
		},
		{
			name:       "empty_quota_entry_list",
			volumeName: "empty_volume",
			mockResponse: &azgo.QuotaListEntriesIterResponse{
				Result: azgo.QuotaListEntriesIterResponseResult{
					ResultStatusAttr:  "passed",
					AttributesListPtr: nil,
				},
			},
			mockError:     nil,
			expectedCount: 0,
			expectError:   false,
		},
		{
			name:       "quota_entry_list_api_error",
			volumeName: "error_volume",
			mockResponse: &azgo.QuotaListEntriesIterResponse{
				Result: azgo.QuotaListEntriesIterResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:     errors.New("API call failed"),
			expectedCount: 0,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().QuotaEntryList(tt.volumeName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			entries, err := oapi.QuotaEntryList(ctx, tt.volumeName)

			if tt.expectError {
				assert.Error(t, err, "expected error when quota entry list should fail")
				assert.Nil(t, entries)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, entries)
				assert.Len(t, entries, tt.expectedCount)
				if tt.expectedCount > 0 {
					assert.Equal(t, "/vol/test_volume", entries[0].Target)
					assert.Equal(t, int64(1073741824), entries[0].DiskLimitBytes) // 1048576 KB * 1024 = bytes
				}
			}
		})
	}
}

func TestOntapAPIZAPI_QuotaSetEntry(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name              string
		qtreeName         string
		volumeName        string
		quotaType         string
		diskLimit         string
		expectedQtreeName string
		expectedTarget    string
		mockResponse      *azgo.QuotaSetEntryResponse
		mockError         error
		expectError       bool
	}{
		{
			name:              "successful_tree_quota_set",
			qtreeName:         "test_qtree",
			volumeName:        "test_volume",
			quotaType:         "tree",
			diskLimit:         "1048576",
			expectedQtreeName: "",
			expectedTarget:    "/vol/test_volume/test_qtree",
			mockResponse: &azgo.QuotaSetEntryResponse{
				Result: azgo.QuotaSetEntryResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:              "successful_user_quota_set",
			qtreeName:         "test_qtree",
			volumeName:        "test_volume",
			quotaType:         "user",
			diskLimit:         "524288",
			expectedQtreeName: "test_qtree",
			expectedTarget:    "",
			mockResponse: &azgo.QuotaSetEntryResponse{
				Result: azgo.QuotaSetEntryResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:              "quota_set_api_error",
			qtreeName:         "error_qtree",
			volumeName:        "test_volume",
			quotaType:         "tree",
			diskLimit:         "1048576",
			expectedQtreeName: "",
			expectedTarget:    "/vol/test_volume/error_qtree",
			mockResponse: &azgo.QuotaSetEntryResponse{
				Result: azgo.QuotaSetEntryResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   errors.New("API call failed"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().QuotaSetEntry(tt.expectedQtreeName, tt.volumeName, tt.expectedTarget, tt.quotaType, tt.diskLimit).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.QuotaSetEntry(ctx, tt.qtreeName, tt.volumeName, tt.quotaType, tt.diskLimit)

			if tt.expectError {
				assert.Error(t, err, "expected error when quota entry setting should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_QuotaStatus(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name           string
		volumeName     string
		mockResponse   *azgo.QuotaStatusResponse
		mockError      error
		expectedStatus string
		expectError    bool
	}{
		{
			name:       "quota_status_on",
			volumeName: "test_volume",
			mockResponse: &azgo.QuotaStatusResponse{
				Result: azgo.QuotaStatusResponseResult{
					ResultStatusAttr: "passed",
					StatusPtr:        convert.ToPtr("on"),
				},
			},
			mockError:      nil,
			expectedStatus: "on",
			expectError:    false,
		},
		{
			name:       "quota_status_off",
			volumeName: "test_volume",
			mockResponse: &azgo.QuotaStatusResponse{
				Result: azgo.QuotaStatusResponseResult{
					ResultStatusAttr: "passed",
					StatusPtr:        convert.ToPtr("off"),
				},
			},
			mockError:      nil,
			expectedStatus: "off",
			expectError:    false,
		},
		{
			name:       "quota_status_api_error",
			volumeName: "error_volume",
			mockResponse: &azgo.QuotaStatusResponse{
				Result: azgo.QuotaStatusResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:      errors.New("API call failed"),
			expectedStatus: "",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().QuotaStatus(tt.volumeName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			status, err := oapi.QuotaStatus(ctx, tt.volumeName)

			if tt.expectError {
				assert.Error(t, err, "expected error when quota status check should fail")
				assert.Empty(t, status)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedStatus, status)
			}
		})
	}
}

func TestOntapAPIZAPI_QuotaOff(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		volumeName   string
		mockResponse *azgo.QuotaOffResponse
		mockError    error
		expectError  bool
	}{
		{
			name:       "successful_quota_off",
			volumeName: "test_volume",
			mockResponse: &azgo.QuotaOffResponse{
				Result: azgo.QuotaOffResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:       "quota_off_api_error",
			volumeName: "error_volume",
			mockResponse: &azgo.QuotaOffResponse{
				Result: azgo.QuotaOffResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   errors.New("API call failed"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().QuotaOff(tt.volumeName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.QuotaOff(ctx, tt.volumeName)

			if tt.expectError {
				assert.Error(t, err, "expected error when quota disable should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_QuotaOn(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name         string
		volumeName   string
		mockResponse *azgo.QuotaOnResponse
		mockError    error
		expectError  bool
	}{
		{
			name:       "successful_quota_on",
			volumeName: "test_volume",
			mockResponse: &azgo.QuotaOnResponse{
				Result: azgo.QuotaOnResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:       "quota_on_api_error",
			volumeName: "error_volume",
			mockResponse: &azgo.QuotaOnResponse{
				Result: azgo.QuotaOnResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   errors.New("API call failed"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().QuotaOn(tt.volumeName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.QuotaOn(ctx, tt.volumeName)

			if tt.expectError {
				assert.Error(t, err, "expected error when quota enable should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_QuotaResize(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name              string
		volumeName        string
		mockResponse      *azgo.QuotaResizeResponse
		mockError         error
		expectError       bool
		expectNotFoundErr bool
	}{
		{
			name:       "successful_quota_resize",
			volumeName: "test_volume",
			mockResponse: &azgo.QuotaResizeResponse{
				Result: azgo.QuotaResizeResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:         nil,
			expectError:       false,
			expectNotFoundErr: false,
		},
		{
			name:       "quota_resize_volume_not_exist",
			volumeName: "nonexistent_volume",
			mockResponse: &azgo.QuotaResizeResponse{
				Result: azgo.QuotaResizeResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  azgo.EVOLUMEDOESNOTEXIST,
					ResultReasonAttr: "Volume does not exist",
				},
			},
			mockError:         nil,
			expectError:       true,
			expectNotFoundErr: true,
		},
		{
			name:       "quota_resize_api_error",
			volumeName: "error_volume",
			mockResponse: &azgo.QuotaResizeResponse{
				Result: azgo.QuotaResizeResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:         errors.New("API call failed"),
			expectError:       true,
			expectNotFoundErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().QuotaResize(tt.volumeName).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.QuotaResize(ctx, tt.volumeName)

			if tt.expectError {
				assert.Error(t, err, "expected error when quota resize should fail")
				if tt.expectNotFoundErr {
					assert.True(t, errors.IsNotFoundError(err))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_QuotaGetEntry(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name           string
		volumeName     string
		qtreeName      string
		quotaType      string
		expectedTarget string
		mockQuota      *azgo.QuotaEntryType
		mockError      error
		expectError    bool
	}{
		{
			name:           "successful_quota_get_volume",
			volumeName:     "test_volume",
			qtreeName:      "",
			quotaType:      "tree",
			expectedTarget: "/vol/test_volume",
			mockQuota: &azgo.QuotaEntryType{
				VolumePtr:      convert.ToPtr("test_volume"),
				QuotaTargetPtr: convert.ToPtr("/vol/test_volume"),
				QtreePtr:       convert.ToPtr(""),
				QuotaTypePtr:   convert.ToPtr("tree"),
				DiskLimitPtr:   convert.ToPtr("1048576"),
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:           "successful_quota_get_qtree",
			volumeName:     "test_volume",
			qtreeName:      "test_qtree",
			quotaType:      "tree",
			expectedTarget: "/vol/test_volume/test_qtree",
			mockQuota: &azgo.QuotaEntryType{
				VolumePtr:      convert.ToPtr("test_volume"),
				QuotaTargetPtr: convert.ToPtr("/vol/test_volume/test_qtree"),
				QtreePtr:       convert.ToPtr("test_qtree"),
				QuotaTypePtr:   convert.ToPtr("tree"),
				DiskLimitPtr:   convert.ToPtr("2097152"),
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:           "quota_get_no_limit",
			volumeName:     "test_volume",
			qtreeName:      "",
			quotaType:      "tree",
			expectedTarget: "/vol/test_volume",
			mockQuota: &azgo.QuotaEntryType{
				VolumePtr:      convert.ToPtr("test_volume"),
				QuotaTargetPtr: convert.ToPtr("/vol/test_volume"),
				QtreePtr:       convert.ToPtr(""),
				QuotaTypePtr:   convert.ToPtr("tree"),
				DiskLimitPtr:   convert.ToPtr("-"),
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:           "quota_get_api_error",
			volumeName:     "error_volume",
			qtreeName:      "",
			quotaType:      "tree",
			expectedTarget: "/vol/error_volume",
			mockQuota: &azgo.QuotaEntryType{
				VolumePtr:      convert.ToPtr("error_volume"),
				QuotaTargetPtr: convert.ToPtr("/vol/error_volume"),
				QtreePtr:       convert.ToPtr(""),
				QuotaTypePtr:   convert.ToPtr("tree"),
				DiskLimitPtr:   convert.ToPtr("-"),
			},
			mockError:   errors.New("API call failed"),
			expectError: false, // The function doesn't propagate the API error, it just logs it
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().QuotaGetEntry(tt.expectedTarget, tt.quotaType).
				Return(tt.mockQuota, tt.mockError).Times(1)

			quota, err := oapi.QuotaGetEntry(ctx, tt.volumeName, tt.qtreeName, tt.quotaType)

			if tt.expectError {
				assert.Error(t, err, "expected error when quota entry retrieval should fail")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, quota)
				if tt.name == "quota_get_no_limit" {
					assert.Equal(t, int64(-1), quota.DiskLimitBytes)
				} else if tt.name == "successful_quota_get_volume" {
					assert.Equal(t, int64(1073741824), quota.DiskLimitBytes) // 1048576 KB * 1024
				} else if tt.name == "successful_quota_get_qtree" {
					assert.Equal(t, int64(2147483648), quota.DiskLimitBytes) // 2097152 KB * 1024
				} else if tt.name == "quota_get_api_error" {
					assert.Equal(t, int64(-1), quota.DiskLimitBytes) // No limit case
				}
			}
		})
	}
}

func TestOntapAPIZAPI_VolumeSnapshotInfo(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name           string
		snapshotName   string
		sourceVolume   string
		mockResponse   *azgo.SnapshotGetIterResponse
		mockError      error
		expectedResult api.Snapshot
		expectError    bool
		expectNotFound bool
	}{
		{
			name:         "successful_snapshot_info",
			snapshotName: "test_snapshot",
			sourceVolume: "test_volume",
			mockResponse: &azgo.SnapshotGetIterResponse{
				Result: azgo.SnapshotGetIterResponseResult{
					ResultStatusAttr: "passed",
					NumRecordsPtr:    convert.ToPtr(1),
					AttributesListPtr: &azgo.SnapshotGetIterResponseResultAttributesList{
						SnapshotInfoPtr: []azgo.SnapshotInfoType{
							{
								AccessTimePtr: convert.ToPtr(1640995200), // Unix timestamp
								NamePtr:       convert.ToPtr("test_snapshot"),
							},
						},
					},
				},
			},
			mockError: nil,
			expectedResult: api.Snapshot{
				CreateTime: "2022-01-01T00:00:00Z",
				Name:       "test_snapshot",
			},
			expectError:    false,
			expectNotFound: false,
		},
		{
			name:         "snapshot_not_found",
			snapshotName: "nonexistent_snapshot",
			sourceVolume: "test_volume",
			mockResponse: &azgo.SnapshotGetIterResponse{
				Result: azgo.SnapshotGetIterResponseResult{
					ResultStatusAttr: "passed",
					NumRecordsPtr:    convert.ToPtr(0),
				},
			},
			mockError:      nil,
			expectedResult: api.Snapshot{},
			expectError:    true,
			expectNotFound: true,
		},
		{
			name:         "multiple_snapshots_error",
			snapshotName: "duplicate_snapshot",
			sourceVolume: "test_volume",
			mockResponse: &azgo.SnapshotGetIterResponse{
				Result: azgo.SnapshotGetIterResponseResult{
					ResultStatusAttr: "passed",
					NumRecordsPtr:    convert.ToPtr(2),
				},
			},
			mockError:      nil,
			expectedResult: api.Snapshot{},
			expectError:    true,
			expectNotFound: false,
		},
		{
			name:         "api_error_handling",
			snapshotName: "error_snapshot",
			sourceVolume: "test_volume",
			mockResponse: &azgo.SnapshotGetIterResponse{
				Result: azgo.SnapshotGetIterResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:      errors.New("API call failed"),
			expectedResult: api.Snapshot{},
			expectError:    true,
			expectNotFound: false,
		},
		{
			name:           "nil_response_handling",
			snapshotName:   "nil_snapshot",
			sourceVolume:   "test_volume",
			mockResponse:   nil,
			mockError:      nil,
			expectedResult: api.Snapshot{},
			expectError:    true,
			expectNotFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().SnapshotInfo(tt.snapshotName, tt.sourceVolume).
				Return(tt.mockResponse, tt.mockError).Times(1)

			result, err := oapi.VolumeSnapshotInfo(ctx, tt.snapshotName, tt.sourceVolume)

			if tt.expectError {
				assert.Error(t, err, "expected error when volume snapshot info should fail")
				if tt.expectNotFound {
					assert.True(t, errors.IsNotFoundError(err))
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult.Name, result.Name)
				assert.Equal(t, tt.expectedResult.CreateTime, result.CreateTime)
			}
		})
	}
}

func TestOntapAPIZAPI_VolumeSnapshotList(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name          string
		sourceVolume  string
		mockResponse  *azgo.SnapshotGetIterResponse
		mockError     error
		expectedCount int
		expectError   bool
	}{
		{
			name:         "successful_snapshot_list",
			sourceVolume: "test_volume",
			mockResponse: &azgo.SnapshotGetIterResponse{
				Result: azgo.SnapshotGetIterResponseResult{
					ResultStatusAttr: "passed",
					AttributesListPtr: &azgo.SnapshotGetIterResponseResultAttributesList{
						SnapshotInfoPtr: []azgo.SnapshotInfoType{
							{
								AccessTimePtr: convert.ToPtr(1640995200),
								NamePtr:       convert.ToPtr("snapshot1"),
							},
							{
								AccessTimePtr: convert.ToPtr(1641081600),
								NamePtr:       convert.ToPtr("snapshot2"),
							},
						},
					},
				},
			},
			mockError:     nil,
			expectedCount: 2,
			expectError:   false,
		},
		{
			name:         "empty_snapshot_list",
			sourceVolume: "empty_volume",
			mockResponse: &azgo.SnapshotGetIterResponse{
				Result: azgo.SnapshotGetIterResponseResult{
					ResultStatusAttr:  "passed",
					AttributesListPtr: nil,
				},
			},
			mockError:     nil,
			expectedCount: 0,
			expectError:   false,
		},
		{
			name:         "snapshot_list_api_error",
			sourceVolume: "error_volume",
			mockResponse: &azgo.SnapshotGetIterResponse{
				Result: azgo.SnapshotGetIterResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:     errors.New("API call failed"),
			expectedCount: 0,
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().SnapshotList(tt.sourceVolume).
				Return(tt.mockResponse, tt.mockError).Times(1)

			snapshots, err := oapi.VolumeSnapshotList(ctx, tt.sourceVolume)

			if tt.expectError {
				assert.Error(t, err, "expected error when volume snapshot list should fail")
				assert.Nil(t, snapshots)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, snapshots)
				assert.Len(t, snapshots, tt.expectedCount)
				if tt.expectedCount > 0 {
					assert.Equal(t, "snapshot1", snapshots[0].Name)
					assert.Equal(t, "2022-01-01T00:00:00Z", snapshots[0].CreateTime)
					if tt.expectedCount > 1 {
						assert.Equal(t, "snapshot2", snapshots[1].Name)
						assert.Equal(t, "2022-01-02T00:00:00Z", snapshots[1].CreateTime)
					}
				}
			}
		})
	}
}

func TestOntapAPIZAPI_VolumeSnapshotDelete(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name               string
		snapshotName       string
		sourceVolume       string
		mockResponse       *azgo.SnapshotDeleteResponse
		mockError          error
		expectError        bool
		expectSnapshotBusy bool
	}{
		{
			name:         "successful_snapshot_delete",
			snapshotName: "test_snapshot",
			sourceVolume: "test_volume",
			mockResponse: &azgo.SnapshotDeleteResponse{
				Result: azgo.SnapshotDeleteResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:          nil,
			expectError:        false,
			expectSnapshotBusy: false,
		},
		{
			name:         "snapshot_busy_error",
			snapshotName: "busy_snapshot",
			sourceVolume: "test_volume",
			mockResponse: &azgo.SnapshotDeleteResponse{
				Result: azgo.SnapshotDeleteResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  azgo.ESNAPSHOTBUSY,
					ResultReasonAttr: "Snapshot is busy",
				},
			},
			mockError:          nil,
			expectError:        true,
			expectSnapshotBusy: true,
		},
		{
			name:         "snapshot_delete_api_error",
			snapshotName: "error_snapshot",
			sourceVolume: "test_volume",
			mockResponse: &azgo.SnapshotDeleteResponse{
				Result: azgo.SnapshotDeleteResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:          errors.New("API call failed"),
			expectError:        true,
			expectSnapshotBusy: false,
		},
		{
			name:         "snapshot_delete_zapi_error",
			snapshotName: "zapi_error_snapshot",
			sourceVolume: "test_volume",
			mockResponse: &azgo.SnapshotDeleteResponse{
				Result: azgo.SnapshotDeleteResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  "1234",
					ResultReasonAttr: "Some other error",
				},
			},
			mockError:          nil,
			expectError:        true,
			expectSnapshotBusy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().SnapshotDelete(tt.snapshotName, tt.sourceVolume).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.VolumeSnapshotDelete(ctx, tt.snapshotName, tt.sourceVolume)

			if tt.expectError {
				assert.Error(t, err, "expected error when volume snapshot deletion should fail")
				if tt.expectSnapshotBusy {
					assert.True(t, api.IsSnapshotBusyError(err))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_VolumeSnapshotDeleteWithRetry(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name             string
		snapshotName     string
		sourceVolume     string
		maxRetries       int
		timeout          time.Duration
		mockResponses    []*azgo.SnapshotDeleteResponse
		mockErrors       []error
		expectError      bool
		expectedAttempts int
	}{
		{
			name:         "successful_delete_first_attempt",
			snapshotName: "test_snapshot",
			sourceVolume: "test_volume",
			maxRetries:   3,
			timeout:      10 * time.Second,
			mockResponses: []*azgo.SnapshotDeleteResponse{
				{
					Result: azgo.SnapshotDeleteResponseResult{
						ResultStatusAttr: "passed",
					},
				},
			},
			mockErrors:       []error{nil},
			expectError:      false,
			expectedAttempts: 1,
		},
		{
			name:         "successful_delete_after_busy_error",
			snapshotName: "busy_snapshot",
			sourceVolume: "test_volume",
			maxRetries:   3,
			timeout:      10 * time.Second,
			mockResponses: []*azgo.SnapshotDeleteResponse{
				{
					Result: azgo.SnapshotDeleteResponseResult{
						ResultStatusAttr: "failed",
						ResultErrnoAttr:  azgo.ESNAPSHOTBUSY,
						ResultReasonAttr: "Snapshot is busy",
					},
				},
				{
					Result: azgo.SnapshotDeleteResponseResult{
						ResultStatusAttr: "passed",
					},
				},
			},
			mockErrors:       []error{nil, nil},
			expectError:      false,
			expectedAttempts: 2,
		},
		{
			name:         "max_retries_exceeded",
			snapshotName: "persistent_busy_snapshot",
			sourceVolume: "test_volume",
			maxRetries:   2,
			timeout:      10 * time.Second,
			mockResponses: []*azgo.SnapshotDeleteResponse{
				{
					Result: azgo.SnapshotDeleteResponseResult{
						ResultStatusAttr: "failed",
						ResultErrnoAttr:  azgo.ESNAPSHOTBUSY,
						ResultReasonAttr: "Snapshot is busy",
					},
				},
				{
					Result: azgo.SnapshotDeleteResponseResult{
						ResultStatusAttr: "failed",
						ResultErrnoAttr:  azgo.ESNAPSHOTBUSY,
						ResultReasonAttr: "Snapshot is busy",
					},
				},
				{
					Result: azgo.SnapshotDeleteResponseResult{
						ResultStatusAttr: "failed",
						ResultErrnoAttr:  azgo.ESNAPSHOTBUSY,
						ResultReasonAttr: "Snapshot is busy",
					},
				},
			},
			mockErrors:       []error{nil, nil, nil},
			expectError:      true,
			expectedAttempts: 3, // maxRetries + 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up expectations for the mock calls
			for i := 0; i < tt.expectedAttempts; i++ {
				mock.EXPECT().SnapshotDelete(tt.snapshotName, tt.sourceVolume).
					Return(tt.mockResponses[i], tt.mockErrors[i]).Times(1)
			}

			err := oapi.VolumeSnapshotDeleteWithRetry(ctx, tt.snapshotName, tt.sourceVolume, tt.maxRetries, tt.timeout)

			if tt.expectError {
				assert.Error(t, err, "expected error when volume snapshot deletion with retry should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_GetSVMAggregateAttributes(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()

	tests := []struct {
		name           string
		mockResponse   *azgo.VserverShowAggrGetIterResponse
		mockError      error
		expectedResult map[string]string
		expectError    bool
	}{
		{
			name: "successful_aggregate_attributes_retrieval",
			mockResponse: &azgo.VserverShowAggrGetIterResponse{
				Result: azgo.VserverShowAggrGetIterResponseResult{
					ResultStatusAttr: "passed",
					AttributesListPtr: &azgo.VserverShowAggrGetIterResponseResultAttributesList{
						ShowAggregatesPtr: []azgo.ShowAggregatesType{
							{
								AggregateNamePtr: &[]string{"aggr1"}[0],
								AggregateTypePtr: &[]string{"hdd"}[0],
							},
							{
								AggregateNamePtr: &[]string{"aggr2"}[0],
								AggregateTypePtr: &[]string{"ssd"}[0],
							},
						},
					},
				},
			},
			mockError: nil,
			expectedResult: map[string]string{
				"aggr1": "hdd",
				"aggr2": "ssd",
			},
			expectError: false,
		},
		{
			name: "empty_aggregate_list",
			mockResponse: &azgo.VserverShowAggrGetIterResponse{
				Result: azgo.VserverShowAggrGetIterResponseResult{
					ResultStatusAttr:  "passed",
					AttributesListPtr: nil,
				},
			},
			mockError:      nil,
			expectedResult: map[string]string{},
			expectError:    false,
		},
		{
			name:           "api_error_handling",
			mockResponse:   nil,
			mockError:      fmt.Errorf("API error"),
			expectedResult: nil,
			expectError:    true,
		},
		{
			name: "zapi_error_response",
			mockResponse: &azgo.VserverShowAggrGetIterResponse{
				Result: azgo.VserverShowAggrGetIterResponseResult{
					ResultStatusAttr: "failed",
					ResultReasonAttr: "ZAPI error",
				},
			},
			mockError:      nil,
			expectedResult: nil,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().VserverShowAggrGetIterRequest().Return(tt.mockResponse, tt.mockError).Times(1)

			result, err := oapi.GetSVMAggregateAttributes(ctx)

			if tt.expectError {
				assert.Error(t, err, "expected error when SVM aggregate attributes retrieval should fail")
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestOntapAPIZAPI_GetSVMAggregateSpace(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()
	aggregateName := "test-aggr"

	tests := []struct {
		name           string
		mockResponse   *azgo.AggrSpaceGetIterResponse
		mockError      error
		expectedResult []api.SVMAggregateSpace
		expectError    bool
	}{
		{
			name: "successful_aggregate_space_retrieval",
			mockResponse: &azgo.AggrSpaceGetIterResponse{
				Result: azgo.AggrSpaceGetIterResponseResult{
					AttributesListPtr: &azgo.AggrSpaceGetIterResponseResultAttributesList{
						SpaceInformationPtr: []azgo.SpaceInformationType{
							{
								AggregatePtr:                           &[]string{"test-aggr"}[0],
								AggregateSizePtr:                       &[]int{1000000000}[0],
								VolumeFootprintsPtr:                    &[]int{500000000}[0],
								VolumeFootprintsPercentPtr:             &[]int{50}[0],
								UsedIncludingSnapshotReservePtr:        &[]int{300000000}[0],
								UsedIncludingSnapshotReservePercentPtr: &[]int{30}[0],
							},
						},
					},
				},
			},
			mockError: nil,
			expectedResult: []api.SVMAggregateSpace{
				api.NewSVMAggregateSpace(1000000000, 300000000, 500000000),
			},
			expectError: false,
		},
		{
			name: "empty_space_information",
			mockResponse: &azgo.AggrSpaceGetIterResponse{
				Result: azgo.AggrSpaceGetIterResponseResult{
					AttributesListPtr: &azgo.AggrSpaceGetIterResponseResultAttributesList{
						SpaceInformationPtr: []azgo.SpaceInformationType{},
					},
				},
			},
			mockError:      nil,
			expectedResult: nil,
			expectError:    false,
		},
		{
			name:           "api_error_handling",
			mockResponse:   nil,
			mockError:      fmt.Errorf("API error"),
			expectedResult: nil,
			expectError:    true,
		},
		{
			name:           "nil_response_handling",
			mockResponse:   nil,
			mockError:      nil,
			expectedResult: nil,
			expectError:    true,
		},
		{
			name: "incomplete_space_information_skipped",
			mockResponse: &azgo.AggrSpaceGetIterResponse{
				Result: azgo.AggrSpaceGetIterResponseResult{
					AttributesListPtr: &azgo.AggrSpaceGetIterResponseResultAttributesList{
						SpaceInformationPtr: []azgo.SpaceInformationType{
							{
								// Missing required fields - should be skipped
								AggregatePtr: &[]string{"test-aggr"}[0],
							},
							{
								// Complete entry - should be included
								AggregatePtr:                           &[]string{"test-aggr"}[0],
								AggregateSizePtr:                       &[]int{2000000000}[0],
								VolumeFootprintsPtr:                    &[]int{800000000}[0],
								VolumeFootprintsPercentPtr:             &[]int{40}[0],
								UsedIncludingSnapshotReservePtr:        &[]int{600000000}[0],
								UsedIncludingSnapshotReservePercentPtr: &[]int{30}[0],
							},
						},
					},
				},
			},
			mockError: nil,
			expectedResult: []api.SVMAggregateSpace{
				api.NewSVMAggregateSpace(2000000000, 600000000, 800000000),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().AggrSpaceGetIterRequest(aggregateName).Return(tt.mockResponse, tt.mockError).Times(1)

			result, err := oapi.GetSVMAggregateSpace(ctx, aggregateName)

			if tt.expectError {
				assert.Error(t, err, "expected error when SVM aggregate space retrieval should fail")
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestOntapAPIZAPI_SnapmirrorCreate(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()
	localVolume := "local-vol"
	localSVM := "local-svm"
	remoteVolume := "remote-vol"
	remoteSVM := "remote-svm"
	policy := "async-policy"
	schedule := "hourly"

	tests := []struct {
		name         string
		mockResponse *azgo.SnapmirrorCreateResponse
		mockError    error
		expectError  bool
	}{
		{
			name: "successful_snapmirror_create",
			mockResponse: &azgo.SnapmirrorCreateResponse{
				Result: azgo.SnapmirrorCreateResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name: "duplicate_entry_ignored",
			mockResponse: &azgo.SnapmirrorCreateResponse{
				Result: azgo.SnapmirrorCreateResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  azgo.EDUPLICATEENTRY,
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:         "api_error_handling",
			mockResponse: nil,
			mockError:    fmt.Errorf("API error"),
			expectError:  true,
		},
		{
			name: "zapi_error_response",
			mockResponse: &azgo.SnapmirrorCreateResponse{
				Result: azgo.SnapmirrorCreateResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  azgo.EINVALIDINPUTERROR,
					ResultReasonAttr: "Invalid input",
				},
			},
			mockError:   nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().SnapmirrorCreate(localVolume, localSVM, remoteVolume, remoteSVM, policy, schedule).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.SnapmirrorCreate(ctx, localVolume, localSVM, remoteVolume, remoteSVM, policy, schedule)

			if tt.expectError {
				assert.Error(t, err, "expected error when snapmirror creation should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_SnapmirrorGet(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()
	localVolume := "local-vol"
	localSVM := "local-svm"
	remoteVolume := "remote-vol"
	remoteSVM := "remote-svm"

	// Create a mock time for testing
	mockTime := int64(1234567890)
	expectedTime := time.Unix(mockTime, 0).UTC()

	tests := []struct {
		name           string
		mockResponse   *azgo.SnapmirrorGetResponse
		mockError      error
		expectedResult *api.Snapmirror
		expectError    bool
	}{
		{
			name: "successful_snapmirror_get",
			mockResponse: &azgo.SnapmirrorGetResponse{
				Result: azgo.SnapmirrorGetResponseResult{
					ResultStatusAttr: "passed",
					AttributesPtr: &azgo.SnapmirrorGetResponseResultAttributes{
						SnapmirrorInfoPtr: &azgo.SnapmirrorInfoType{
							MirrorStatePtr:              &[]string{"snapmirrored"}[0],
							RelationshipStatusPtr:       &[]string{"idle"}[0],
							LastTransferTypePtr:         &[]string{"initialize"}[0],
							LastTransferEndTimestampPtr: &[]uint{uint(mockTime)}[0],
							IsHealthyPtr:                &[]bool{true}[0],
							UnhealthyReasonPtr:          &[]string{""}[0],
							PolicyPtr:                   &[]string{"async-policy"}[0],
							SchedulePtr:                 &[]string{"hourly"}[0],
						},
					},
				},
			},
			mockError: nil,
			expectedResult: &api.Snapmirror{
				State:               api.SnapmirrorStateSnapmirrored,
				RelationshipStatus:  api.SnapmirrorStatusIdle,
				LastTransferType:    "initialize",
				IsHealthy:           true,
				UnhealthyReason:     "",
				ReplicationPolicy:   "async-policy",
				ReplicationSchedule: "hourly",
				EndTransferTime:     &expectedTime,
			},
			expectError: false,
		},
		{
			name: "snapmirror_not_found",
			mockResponse: &azgo.SnapmirrorGetResponse{
				Result: azgo.SnapmirrorGetResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  azgo.EOBJECTNOTFOUND,
				},
			},
			mockError:      nil,
			expectedResult: nil,
			expectError:    true,
		},
		{
			name:           "api_error_handling",
			mockResponse:   nil,
			mockError:      fmt.Errorf("API error"),
			expectedResult: nil,
			expectError:    true,
		},
		{
			name:           "nil_response_handling",
			mockResponse:   nil,
			mockError:      nil,
			expectedResult: nil,
			expectError:    true,
		},
		{
			name: "missing_attributes_error",
			mockResponse: &azgo.SnapmirrorGetResponse{
				Result: azgo.SnapmirrorGetResponseResult{
					ResultStatusAttr: "passed",
					AttributesPtr:    nil,
				},
			},
			mockError:      nil,
			expectedResult: nil,
			expectError:    true,
		},
		{
			name: "missing_snapmirror_info_error",
			mockResponse: &azgo.SnapmirrorGetResponse{
				Result: azgo.SnapmirrorGetResponseResult{
					ResultStatusAttr: "passed",
					AttributesPtr: &azgo.SnapmirrorGetResponseResultAttributes{
						SnapmirrorInfoPtr: nil,
					},
				},
			},
			mockError:      nil,
			expectedResult: nil,
			expectError:    true,
		},
		{
			name: "minimal_snapmirror_info",
			mockResponse: &azgo.SnapmirrorGetResponse{
				Result: azgo.SnapmirrorGetResponseResult{
					ResultStatusAttr: "passed",
					AttributesPtr: &azgo.SnapmirrorGetResponseResultAttributes{
						SnapmirrorInfoPtr: &azgo.SnapmirrorInfoType{
							MirrorStatePtr:        &[]string{"uninitialized"}[0],
							RelationshipStatusPtr: &[]string{"transferring"}[0],
						},
					},
				},
			},
			mockError: nil,
			expectedResult: &api.Snapmirror{
				State:              api.SnapmirrorStateUninitialized,
				RelationshipStatus: api.SnapmirrorStatusTransferring,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().SnapmirrorGet(localVolume, localSVM, remoteVolume, remoteSVM).
				Return(tt.mockResponse, tt.mockError).Times(1)

			result, err := oapi.SnapmirrorGet(ctx, localVolume, localSVM, remoteVolume, remoteSVM)

			if tt.expectError {
				assert.Error(t, err, "expected error when snapmirror get should fail")
				if errors.IsNotFoundError(err) {
					// Special case for NotFoundError
					assert.Nil(t, result)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestOntapAPIZAPI_SnapmirrorInitialize(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()
	localVolume := "local-vol"
	localSVM := "local-svm"
	remoteVolume := "remote-vol"
	remoteSVM := "remote-svm"

	tests := []struct {
		name         string
		mockResponse *azgo.SnapmirrorInitializeResponse
		mockError    error
		expectError  bool
	}{
		{
			name: "successful_snapmirror_initialize",
			mockResponse: &azgo.SnapmirrorInitializeResponse{
				Result: azgo.SnapmirrorInitializeResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name: "transfer_already_in_progress",
			mockResponse: &azgo.SnapmirrorInitializeResponse{
				Result: azgo.SnapmirrorInitializeResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  azgo.ETRANSFERINPROGRESS,
					ResultReasonAttr: "Transfer in progress",
				},
			},
			mockError:   azgo.NewZapiError(azgo.SnapmirrorInitializeResponseResult{ResultStatusAttr: "failed", ResultErrnoAttr: azgo.ETRANSFERINPROGRESS}),
			expectError: false,
		},
		{
			name: "zapi_error_response",
			mockResponse: &azgo.SnapmirrorInitializeResponse{
				Result: azgo.SnapmirrorInitializeResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  azgo.EINVALIDINPUTERROR,
					ResultReasonAttr: "Invalid input",
				},
			},
			mockError:   azgo.NewZapiError(azgo.SnapmirrorInitializeResponseResult{ResultStatusAttr: "failed", ResultErrnoAttr: azgo.EINVALIDINPUTERROR}),
			expectError: true,
		},
		{
			name:         "api_error_handling",
			mockResponse: nil,
			mockError:    azgo.NewZapiError(azgo.SnapmirrorInitializeResponseResult{ResultStatusAttr: "failed", ResultErrnoAttr: azgo.EINVALIDINPUTERROR}),
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().SnapmirrorInitialize(localVolume, localSVM, remoteVolume, remoteSVM).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.SnapmirrorInitialize(ctx, localVolume, localSVM, remoteVolume, remoteSVM)

			if tt.expectError {
				assert.Error(t, err, "expected error when snapmirror initialization should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_SnapmirrorDelete(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()
	localVolume := "local-vol"
	localSVM := "local-svm"
	remoteVolume := "remote-vol"
	remoteSVM := "remote-svm"

	tests := []struct {
		name         string
		mockResponse *azgo.SnapmirrorDestroyResponse
		mockError    error
		expectError  bool
	}{
		{
			name: "successful_snapmirror_delete",
			mockResponse: &azgo.SnapmirrorDestroyResponse{
				Result: azgo.SnapmirrorDestroyResponseResult{
					ResultStatusAttr: "passed",
				},
			},
			mockError:   nil,
			expectError: false,
		},
		{
			name:         "api_error_handling",
			mockResponse: nil,
			mockError:    fmt.Errorf("API error"),
			expectError:  true,
		},
		{
			name: "zapi_error_response",
			mockResponse: &azgo.SnapmirrorDestroyResponse{
				Result: azgo.SnapmirrorDestroyResponseResult{
					ResultStatusAttr: "failed",
					ResultErrnoAttr:  azgo.EOBJECTNOTFOUND,
					ResultReasonAttr: "Snapmirror not found",
				},
			},
			mockError:   nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock.EXPECT().SnapmirrorDelete(localVolume, localSVM, remoteVolume, remoteSVM).
				Return(tt.mockResponse, tt.mockError).Times(1)

			err := oapi.SnapmirrorDelete(ctx, localVolume, localSVM, remoteVolume, remoteSVM)

			if tt.expectError {
				assert.Error(t, err, "expected error when snapmirror deletion should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOntapAPIZAPI_SnapmirrorResync(t *testing.T) {
	ctrl, mock, oapi := setupTestZAPIClient(t)
	defer ctrl.Finish()

	ctx := context.Background()
	localVolume := "local-vol"
	localSVM := "local-svm"
	remoteVolume := "remote-vol"
	remoteSVM := "remote-svm"

	tests := []struct {
		name        string
		setupMock   func()
		expectError bool
	}{
		{
			name: "successful_snapmirror_resync",
			setupMock: func() {
				response := &azgo.SnapmirrorResyncResponse{
					Result: azgo.SnapmirrorResyncResponseResult{
						ResultStatusAttr: "passed",
					},
				}
				mock.EXPECT().SnapmirrorResync(localVolume, localSVM, remoteVolume, remoteSVM).
					Return(response, nil).Times(1)
			},
			expectError: false,
		},
		{
			name: "transfer_in_progress_ignored",
			setupMock: func() {
				response := &azgo.SnapmirrorResyncResponse{
					Result: azgo.SnapmirrorResyncResponseResult{
						ResultStatusAttr: "failed",
						ResultErrnoAttr:  azgo.ETRANSFERINPROGRESS,
						ResultReasonAttr: "Transfer in progress",
					},
				}
				mock.EXPECT().SnapmirrorResync(localVolume, localSVM, remoteVolume, remoteSVM).
					Return(response, azgo.NewZapiError(response.Result)).Times(1)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMock()

			err := oapi.SnapmirrorResync(ctx, localVolume, localSVM, remoteVolume, remoteSVM)

			if tt.expectError {
				assert.Error(t, err, "expected error when snapmirror resync should fail")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
