// Copyright 2022 NetApp, Inc. All Rights Reserved.

package api_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/s_a_n"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

var ctx = context.Background()

func TestEnsureIGroupAdded(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(rsi)
	assert.NoError(t, err)

	initiator := "initiator"
	initiatorGroup := "initiatorGroup"
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	// common call for all subtests
	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	rsi.EXPECT().IgroupGetByName(ctx, initiatorGroup).Return(nil, errors.New("error getting igroup"))
	err = oapi.EnsureIgroupAdded(ctx, initiatorGroup, initiator)
	assert.Errorf(t, err, "error getting igroup")

	// No error and igroup not present
	rsi.EXPECT().IgroupGetByName(ctx, initiatorGroup).Return(nil, nil)
	rsi.EXPECT().IgroupAdd(ctx, initiatorGroup, initiator).Return(nil)
	err = oapi.EnsureIgroupAdded(ctx, initiatorGroup, initiator)
	assert.NoError(t, err)

	// positive test case
	igroup := &models.Igroup{IgroupInlineInitiators: []*models.IgroupInlineInitiatorsInlineArrayItem{{Name: utils.Ptr(initiator)}}}
	rsi.EXPECT().IgroupGetByName(ctx, initiatorGroup).Return(igroup, nil)
	err = oapi.EnsureIgroupAdded(ctx, initiatorGroup, initiator)
	assert.NoError(t, err)
}

func TestEnsureLunMapped(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(rsi)
	assert.NoError(t, err)

	initiatorGroup := "initiatorGroup"
	lunPath := "/dev/sda"
	number := utils.Ptr(int64(100))
	lunPayload := &models.LunMapResponse{
		NumRecords: utils.Ptr(int64(1)),
		LunMapResponseInlineRecords: []*models.LunMap{
			{
				LogicalUnitNumber: nil,
				Igroup: &models.LunMapInlineIgroup{
					Name: utils.Ptr(initiatorGroup),
				},
			},
		},
	}
	lunMapCollection := &s_a_n.LunMapCollectionGetOK{
		Payload: lunPayload,
	}

	// lunMapInfo returning error
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, errors.New("error"))
	resultLun, err := oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath)
	assert.Errorf(t, err, "problem reading maps for LUN /dev/sda: error")
	assert.Equal(t, -1, resultLun)

	// lunMapInfo returning nil lun collection
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(nil, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath)
	assert.Errorf(t, err, "problem reading maps for LUN /dev/sda")
	assert.Equal(t, -1, resultLun)

	// positive test case where lun == nil, lunGetByName gets called to find the LUN details
	lun := &models.Lun{LunInlineLunMaps: []*models.LunInlineLunMapsInlineArrayItem{{LogicalUnitNumber: number}}}
	rsi.EXPECT().LunGetByName(ctx, lunPath).Return(lun, nil)
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath)
	assert.Nil(t, err)
	assert.Equal(t, int(*number), resultLun)

	// record.LogicalUnitNumber == nil and lunGetByName returns error
	rsi.EXPECT().LunGetByName(ctx, lunPath).Return(nil, errors.New("error getting LUN by name"))
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath)
	assert.Errorf(t, err, "error getting LUN by name")
	assert.Equal(t, -1, resultLun)

	// record.LogicalUnitNumber == nil and lunGetByName returns lun == nil
	lunMapCreated := &s_a_n.LunMapCreateCreated{
		Payload: lunPayload,
	}
	rsi.EXPECT().LunGetByName(ctx, lunPath).Return(nil, nil)
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	rsi.EXPECT().LunMap(ctx, initiatorGroup, lunPath, -1).Return(lunMapCreated, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath)
	assert.NoError(t, err)
	// As LogicalUnitNumber == nil currently, -1 is returned
	assert.Equal(t, -1, resultLun)

	// positive test case where record.LogicalUnitNumber != nil
	lunMapCollection.Payload.LunMapResponseInlineRecords[0].LogicalUnitNumber = number
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath)
	assert.Nil(t, err)
	assert.Equal(t, int(*number), resultLun)

	// If lun not already mapped OR incorrectly mapped
	lunMapCollection.Payload.LunMapResponseInlineRecords[0].Igroup.Name = utils.Ptr("tmp")
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	rsi.EXPECT().LunMap(ctx, initiatorGroup, lunPath, -1).Return(lunMapCreated, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath)
	assert.Nil(t, err)
	assert.Equal(t, int(*number), resultLun)
}

func TestOntapAPIREST_LunGetFSType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
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

func TestOntapAPIREST_LunGetFSType_Failure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
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
