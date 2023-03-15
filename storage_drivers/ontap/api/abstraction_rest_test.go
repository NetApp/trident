// Copyright 2022 NetApp, Inc. All Rights Reserved.

package api_test

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/s_a_n"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
	"github.com/netapp/trident/utils"
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
	resultLun, err := oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath, true)
	assert.Errorf(t, err, "problem reading maps for LUN /dev/sda: error")
	assert.Equal(t, -1, resultLun)

	// lunMapInfo returning nil lun collection
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(nil, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath, true)
	assert.Errorf(t, err, "problem reading maps for LUN /dev/sda")
	assert.Equal(t, -1, resultLun)

	// positive test case where lun == nil, lunGetByName gets called to find the LUN details
	lun := &models.Lun{LunInlineLunMaps: []*models.LunInlineLunMapsInlineArrayItem{{LogicalUnitNumber: number}}}
	rsi.EXPECT().LunGetByName(ctx, lunPath).Return(lun, nil)
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath, true)
	assert.Nil(t, err)
	assert.Equal(t, int(*number), resultLun)

	// record.LogicalUnitNumber == nil and lunGetByName returns error
	rsi.EXPECT().LunGetByName(ctx, lunPath).Return(nil, errors.New("error getting lun by name"))
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath, true)
	assert.Errorf(t, err, "error getting lun by name")
	assert.Equal(t, -1, resultLun)

	// record.LogicalUnitNumber == nil and lunGetByName returns lun == nil
	lunMapCreated := &s_a_n.LunMapCreateCreated{
		Payload: lunPayload,
	}
	rsi.EXPECT().LunGetByName(ctx, lunPath).Return(nil, nil)
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	rsi.EXPECT().LunMap(ctx, initiatorGroup, lunPath, -1).Return(lunMapCreated, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath, true)
	assert.NoError(t, err)
	// As LogicalUnitNumber == nil currently, -1 is returned
	assert.Equal(t, -1, resultLun)

	// positive test case where record.LogicalUnitNumber != nil
	lunMapCollection.Payload.LunMapResponseInlineRecords[0].LogicalUnitNumber = number
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath, true)
	assert.Nil(t, err)
	assert.Equal(t, int(*number), resultLun)

	// If lun not already mapped OR incorrectly mapped
	lunMapCollection.Payload.LunMapResponseInlineRecords[0].Igroup.Name = utils.Ptr("tmp")
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	rsi.EXPECT().LunMap(ctx, initiatorGroup, lunPath, -1).Return(lunMapCreated, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath, false)
	assert.Nil(t, err)
	assert.Equal(t, int(*number), resultLun)
}
