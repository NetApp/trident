// Copyright 2023 NetApp, Inc. All Rights Reserved.

package api_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
)

func TestOntapAPIZAPI_LunGetFSType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockapi.NewMockZapiClientInterface(ctrl)
	oapi, err := api.NewOntapAPIZAPIFromZapiClientInterface(mock)
	assert.NoError(t, err)

	// When value is present in LUN attribute
	mock.EXPECT().LunGetAttribute(ctx, "/vol/volumeName/storagePrefix_lunName",
		"com.netapp.ndvp.fstype").Return("raw", nil)
	fstype, err := oapi.LunGetFSType(ctx, "/vol/volumeName/storagePrefix_lunName")
	assert.Equal(t, "raw", fstype)

	// When value is present in LUN comment
	mock.EXPECT().LunGetAttribute(ctx, "/vol/volumeName/storagePrefix_lunName",
		"com.netapp.ndvp.fstype").Return("", fmt.Errorf("not able to find fstype attribute"))
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
	assert.Equal(t, "ext4", fstype)
}

func TestOntapAPIZAPI_LunGetFSType_Failure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockapi.NewMockZapiClientInterface(ctrl)
	oapi, err := api.NewOntapAPIZAPIFromZapiClientInterface(mock)
	assert.NoError(t, err)

	// Case 1: LunGetComment fails
	mock.EXPECT().LunGetAttribute(ctx, "/vol/volumeName/storagePrefix_lunName",
		"com.netapp.ndvp.fstype").Return("", fmt.Errorf("not able to find fstype attribute"))
	mock.EXPECT().LunGetComment(ctx,
		"/vol/volumeName/storagePrefix_lunName").Return("", fmt.Errorf("failed to get LUN comment"))
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
		"com.netapp.ndvp.fstype").Return("", fmt.Errorf("not able to find fstype attribute"))
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
		"com.netapp.ndvp.fstype").Return("", fmt.Errorf("not able to find fstype attribute"))
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
		"com.netapp.ndvp.fstype").Return("", fmt.Errorf("not able to find fstype attribute"))
	mock.EXPECT().LunGetComment(ctx,
		"/vol/volumeName/storagePrefix_lunName").Return(invalidCommentJSON, nil)
	fstype, err = oapi.LunGetFSType(ctx, "/vol/volumeName/storagePrefix_lunName")
	assert.Empty(t, fstype)
	assert.Error(t, err)
}

func TestLunSetAttributeZapi(t *testing.T) {
	ctrl := gomock.NewController(t)
	zapi := mockapi.NewMockZapiClientInterface(ctrl)
	oapi, err := api.NewOntapAPIZAPIFromZapiClientInterface(zapi)
	assert.NoError(t, err)

	tempLunPath := "/vol/vol0/lun0"
	tempAttribute := "filesystem"

	response := azgo.LunSetAttributeResponse{
		Result: azgo.LunSetAttributeResponseResult{
			ResultStatusAttr: "passed",
		},
	}

	// case 1a: Positive test, update LUN attribute - fsType.
	zapi.EXPECT().LunSetAttribute(tempLunPath, tempAttribute, "fake-FStype").Return(&response, nil).Times(1)
	err = oapi.LunSetAttribute(ctx, tempLunPath, tempAttribute, "fake-FStype", "", "", "")
	assert.NoError(t, err, "error returned while modifying a LUN attribute")

	// case 1b: Negative test, d.api.LunSetAttribute for fsType return error
	zapi.EXPECT().LunSetAttribute(tempLunPath, tempAttribute, "fake-FStype").Return(nil, fmt.Errorf("error")).Times(1)
	err = oapi.LunSetAttribute(ctx, tempLunPath, tempAttribute, "fake-FStype", "", "", "")
	assert.Error(t, err)

	// case 2: Positive test, update LUN attributes those are: context, luks, formatOptions.
	zapi.EXPECT().LunSetAttribute(tempLunPath, gomock.Any(), gomock.Any()).Return(&response, nil).AnyTimes()
	err = oapi.LunSetAttribute(ctx, tempLunPath, "filesystem", "",
		"context", "LUKS", "formatOptions")
	assert.NoError(t, err, "error returned while modifying a LUN attribute")
}

func TestLunGetAttributeZapi(t *testing.T) {
	ctrl := gomock.NewController(t)
	zapi := mockapi.NewMockZapiClientInterface(ctrl)
	oapi, err := api.NewOntapAPIZAPIFromZapiClientInterface(zapi)
	assert.NoError(t, err)

	tempLunPath := "/vol/vol1/lun0"
	tempAttributeName := "fsType"

	// 1 - Negative test, d.api.LunGetAttribute returns error
	zapi.EXPECT().LunGetAttribute(gomock.Any(), tempLunPath, tempAttributeName).Return("", fmt.Errorf("error")).Times(1)
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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockapi.NewMockZapiClientInterface(ctrl)
	oapi, err := api.NewOntapAPIZAPIFromZapiClientInterface(mock)
	assert.NoError(t, err)

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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockapi.NewMockZapiClientInterface(ctrl)
	oapi, err := api.NewOntapAPIZAPIFromZapiClientInterface(mock)
	assert.NoError(t, err)

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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockapi.NewMockZapiClientInterface(ctrl)
	oapi, err := api.NewOntapAPIZAPIFromZapiClientInterface(mock)
	assert.NoError(t, err)

	policyName := "testPolicy"

	mock.EXPECT().ExportRuleGetIterRequest(policyName).Return(nil,
		fmt.Errorf("error listing export policy rules")).Times(1)
	rules, err := oapi.ExportRuleList(ctx, policyName)
	assert.Error(t, err)
	assert.Nil(t, rules)
}

func TestExportRuleList_Zapi_NoRecords(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockapi.NewMockZapiClientInterface(ctrl)
	oapi, err := api.NewOntapAPIZAPIFromZapiClientInterface(mock)
	assert.NoError(t, err)

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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockapi.NewMockZapiClientInterface(ctrl)
	oapi, err := api.NewOntapAPIZAPIFromZapiClientInterface(mock)
	assert.NoError(t, err)

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

func intPtr(i int) *int {
	return &i
}
