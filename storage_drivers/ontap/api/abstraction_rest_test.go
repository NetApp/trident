// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/pkg/convert"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	nas "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/n_a_s"
	nvme "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/n_v_me"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/networking"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/s_a_n"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/snapmirror"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/storage"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/models"
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

	rsi.EXPECT().IgroupGetByName(ctx, initiatorGroup, gomock.Any()).Return(nil, errors.New("error getting igroup"))
	err = oapi.EnsureIgroupAdded(ctx, initiatorGroup, initiator)
	assert.Errorf(t, err, "error getting igroup")

	// No error and igroup not present
	rsi.EXPECT().IgroupGetByName(ctx, initiatorGroup, gomock.Any()).Return(nil, nil)
	rsi.EXPECT().IgroupAdd(ctx, initiatorGroup, initiator).Return(nil)
	err = oapi.EnsureIgroupAdded(ctx, initiatorGroup, initiator)
	assert.NoError(t, err)

	// positive test case
	igroup := &models.Igroup{IgroupInlineInitiators: []*models.IgroupInlineInitiatorsInlineArrayItem{{Name: convert.ToPtr(initiator)}}}
	rsi.EXPECT().IgroupGetByName(ctx, initiatorGroup, gomock.Any()).Return(igroup, nil)
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
	number := convert.ToPtr(int64(100))
	lunPayload := &models.LunMapResponse{
		NumRecords: convert.ToPtr(int64(1)),
		LunMapResponseInlineRecords: []*models.LunMap{
			{
				LogicalUnitNumber: nil,
				Igroup: &models.LunMapInlineIgroup{
					Name: convert.ToPtr(initiatorGroup),
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
	assert.Equal(t, -1, resultLun, "lun count does not match")

	// lunMapInfo returning nil lun collection
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(nil, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath)
	assert.Errorf(t, err, "problem reading maps for LUN /dev/sda")
	assert.Equal(t, -1, resultLun, "lun count does not match")

	// positive test case where lun == nil, lunGetByName gets called to find the LUN details
	lun := &models.Lun{LunInlineLunMaps: []*models.LunInlineLunMapsInlineArrayItem{{LogicalUnitNumber: number}}}
	rsi.EXPECT().LunGetByName(ctx, lunPath, gomock.Any()).Return(lun, nil)
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath)
	assert.Nil(t, err)
	assert.Equal(t, int(*number), resultLun, "lun count does not match")

	// record.LogicalUnitNumber == nil and lunGetByName returns error
	rsi.EXPECT().LunGetByName(ctx, lunPath, gomock.Any()).Return(nil, errors.New("error getting LUN by name"))
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath)
	assert.Errorf(t, err, "error getting LUN by name")
	assert.Equal(t, -1, resultLun, "lun count does not match")

	// record.LogicalUnitNumber == nil and lunGetByName returns lun == nil
	lunMapCreated := &s_a_n.LunMapCreateCreated{
		Payload: lunPayload,
	}
	rsi.EXPECT().LunGetByName(ctx, lunPath, gomock.Any()).Return(nil, nil)
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	rsi.EXPECT().LunMap(ctx, initiatorGroup, lunPath, -1).Return(lunMapCreated, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath)
	assert.NoError(t, err)
	// As LogicalUnitNumber == nil currently, -1 is returned
	assert.Equal(t, -1, resultLun, "lun count does not match")

	// positive test case where record.LogicalUnitNumber != nil
	lunMapCollection.Payload.LunMapResponseInlineRecords[0].LogicalUnitNumber = number
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath)
	assert.Nil(t, err)
	assert.Equal(t, int(*number), resultLun, "lun count does not match")

	// If lun not already mapped OR incorrectly mapped
	lunMapCollection.Payload.LunMapResponseInlineRecords[0].Igroup.Name = convert.ToPtr("tmp")
	rsi.EXPECT().LunMapInfo(ctx, "", lunPath).Return(lunMapCollection, nil)
	rsi.EXPECT().LunMap(ctx, initiatorGroup, lunPath, -1).Return(lunMapCreated, nil)
	resultLun, err = oapi.EnsureLunMapped(ctx, initiatorGroup, lunPath)
	assert.Nil(t, err)
	assert.Equal(t, int(*number), resultLun, "lun count does not match")
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
	assert.Equal(t, "raw", fstype, "volume filesystem is not raw")

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
	assert.Equal(t, "ext4", fstype, "volume filesystem is not ext4")
}

func TestOntapAPIREST_LunGetFSType_Failure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

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

func TestNVMeNamespaceCreate(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	ns := api.NVMeNamespace{
		Name: "fakeNS",
		UUID: "fakeUUID",
	}
	// case 1: No error while creating namespace
	mock.EXPECT().NVMeNamespaceCreate(ctx, ns).Return(nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeNamespaceCreate(ctx, ns)
	assert.NoError(t, err)

	// case 2: Error returned while creating namespace
	mock.EXPECT().NVMeNamespaceCreate(ctx, ns).Return(errors.New("Error while creating namespace"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeNamespaceCreate(ctx, ns)
	assert.Error(t, err)
}

func TestNVMeNamespaceExists(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)
	Name := "fakeNS"
	UUID := "fakeUUID"
	OsType := "fakeOS"
	Location := &models.NvmeNamespaceInlineLocation{
		Volume: &models.NvmeNamespaceInlineLocationInlineVolume{
			Name: &Name,
		},
	}
	Size := int64(99999)
	BlockSize := int64(4096)
	State := "fakeState"
	Space := &models.NvmeNamespaceInlineSpace{
		BlockSize: &BlockSize,
		Size:      &Size,
	}

	Status := &models.NvmeNamespaceInlineStatus{
		State: &State,
	}

	ns := &models.NvmeNamespace{
		Name:     &Name,
		UUID:     &UUID,
		OsType:   &OsType,
		Location: Location,
		Space:    Space,
		Status:   Status,
	}

	// case 1: No error while getting namespace
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name, gomock.Any()).Return(ns, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	exists, err := oapi.NVMeNamespaceExists(ctx, Name)
	assert.NoError(t, err)
	assert.True(t, exists, "expected namespace to exist")

	// case 2: error while getting namespace
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name, gomock.Any()).Return(nil, errors.New("Error while getting namespace"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	exists, err = oapi.NVMeNamespaceExists(ctx, Name)
	assert.Error(t, err)
	assert.False(t, exists, "expected namespace to not exist")

	// case 3: Not found error while getting namespace
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name, gomock.Any()).Return(nil, errors.NotFoundError("namespace not found"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	exists, err = oapi.NVMeNamespaceExists(ctx, Name)
	assert.NoError(t, err, "expected no error when namespace is not found")
	assert.False(t, exists, "expected namespace to not exist")

	// case 4: No error, but no response either
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name, gomock.Any()).Return(nil, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	exists, err = oapi.NVMeNamespaceExists(ctx, Name)
	assert.Error(t, err, "expected error when neither namespace nor any error is returned")
	assert.False(t, exists, "expected namespace to not exist")
}

func TestNVMeNamespaceGetByName(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)
	Name := "fakeNS"
	UUID := "fakeUUID"
	OsType := "fakeOS"
	Location := &models.NvmeNamespaceInlineLocation{
		Volume: &models.NvmeNamespaceInlineLocationInlineVolume{
			Name: &Name,
		},
	}
	Size := int64(99999)
	BlockSize := int64(4096)
	State := "fakeState"
	Space := &models.NvmeNamespaceInlineSpace{
		BlockSize: &BlockSize,
		Size:      &Size,
	}

	Status := &models.NvmeNamespaceInlineStatus{
		State: &State,
	}

	ns := &models.NvmeNamespace{
		Name:     &Name,
		UUID:     &UUID,
		OsType:   &OsType,
		Location: Location,
		Space:    Space,
		Status:   Status,
	}

	// case 1: No error while getting namespace
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name, gomock.Any()).Return(ns, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	namespace, err := oapi.NVMeNamespaceGetByName(ctx, Name)
	assert.NoError(t, err)
	assert.Equal(t, UUID, namespace.UUID, "namespace uuid does not match")
	assert.Equal(t, Name, namespace.Name, "namespace name does not match")

	// case 2: error while getting namespace
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name, gomock.Any()).Return(nil, errors.New("Error while getting namespace"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	namespace, err = oapi.NVMeNamespaceGetByName(ctx, Name)
	assert.Error(t, err)

	// case 3: no error while getting namespace but response is nil
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name, gomock.Any()).Return(nil, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	namespace, err = oapi.NVMeNamespaceGetByName(ctx, Name)
	assert.Error(t, err)
}

func TestNVMeNamespaceList(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)
	Name := "fakeNS"
	UUID := "fakeUUID"
	OsType := "fakeOS"
	Location := &models.NvmeNamespaceInlineLocation{
		Volume: &models.NvmeNamespaceInlineLocationInlineVolume{
			Name: &Name,
		},
	}
	Size := int64(99999)
	BlockSize := int64(4096)
	State := "fakeState"
	Space := &models.NvmeNamespaceInlineSpace{
		BlockSize: &BlockSize,
		Size:      &Size,
	}

	Status := &models.NvmeNamespaceInlineStatus{
		State: &State,
	}

	ns := &models.NvmeNamespace{
		Name:     &Name,
		UUID:     &UUID,
		OsType:   &OsType,
		Location: Location,
		Space:    Space,
		Status:   Status,
	}

	nsResp := &nvme.NvmeNamespaceCollectionGetOK{
		Payload: &models.NvmeNamespaceResponse{
			NvmeNamespaceResponseInlineRecords: []*models.NvmeNamespace{ns},
		},
	}

	// case 1: No error while getting namespace
	mock.EXPECT().NVMeNamespaceList(ctx, Name, gomock.Any()).Return(nsResp, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	namespaces, err := oapi.NVMeNamespaceList(ctx, Name)
	assert.NoError(t, err)
	assert.Equal(t, Name, namespaces[0].Name, "namespace does not match")

	// case 2: error while getting namespace list
	mock.EXPECT().NVMeNamespaceList(ctx, Name, gomock.Any()).Return(nil, errors.New("Error getting namespace list"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	_, err = oapi.NVMeNamespaceList(ctx, Name)
	assert.Error(t, err)

	// case 3: no error while getting namespace list but list is nil
	nsResp1 := &nvme.NvmeNamespaceCollectionGetOK{
		Payload: &models.NvmeNamespaceResponse{
			NvmeNamespaceResponseInlineRecords: []*models.NvmeNamespace{nil},
		},
	}
	mock.EXPECT().NVMeNamespaceList(ctx, Name, gomock.Any()).Return(nsResp1, nil).AnyTimes()
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	_, err = oapi.NVMeNamespaceList(ctx, Name)
	assert.Error(t, err)
}

func TestNVMeNamespaceDelete(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)
	Name := "fakeNS"
	UUID := "fakeUUID"
	OsType := "fakeOS"
	Location := &models.NvmeNamespaceInlineLocation{
		Volume: &models.NvmeNamespaceInlineLocationInlineVolume{
			Name: &Name,
		},
	}
	Size := int64(99999)
	BlockSize := int64(4096)
	State := "fakeState"
	Space := &models.NvmeNamespaceInlineSpace{
		BlockSize: &BlockSize,
		Size:      &Size,
	}

	Status := &models.NvmeNamespaceInlineStatus{
		State: &State,
	}

	ns := &models.NvmeNamespace{
		Name:     &Name,
		UUID:     &UUID,
		OsType:   &OsType,
		Location: Location,
		Space:    Space,
		Status:   Status,
	}

	// case 1: Successful delete
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name, gomock.Any()).Return(ns, nil)
	mock.EXPECT().NVMeNamespaceDelete(ctx, UUID).Return(nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeNamespaceDelete(ctx, Name)
	assert.NoError(t, err, "No error expected while deleting namespace")

	// case 2: Error while getting namespace
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name, gomock.Any()).Return(nil, errors.New("error while getting namespace"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeNamespaceDelete(ctx, Name)
	assert.Error(t, err, "Expected error while deleting namespace")

	// case 3: Empty response when getting namespace
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name, gomock.Any()).Return(nil, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeNamespaceDelete(ctx, Name)
	assert.Error(t, err, "Expected error while deleting namespace")

	// case 4: Error while deleting namespace
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name, gomock.Any()).Return(ns, nil)
	mock.EXPECT().NVMeNamespaceDelete(ctx, UUID).Return(errors.New("error while deleting namespace"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeNamespaceDelete(ctx, Name)
	assert.Error(t, err, "Expected error while deleting namespace")
}

func TestAddNamespaceToSubsystemMap(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	// case 1: No error while adding namespace to subsystem
	nsUUID := "fakeNsUUID"
	subsysUUID := "fakeSubsystemUUID"
	mock.EXPECT().NVMeSubsystemAddNamespace(ctx, subsysUUID, nsUUID).Return(nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeSubsystemAddNamespace(ctx, subsysUUID, nsUUID)
	assert.NoError(t, err)

	// case 2: Error returned while adding namespace to subsystem
	mock.EXPECT().NVMeSubsystemAddNamespace(ctx, subsysUUID, nsUUID).Return(errors.New("Error while adding NS to subsystem"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeSubsystemAddNamespace(ctx, subsysUUID, nsUUID)
	assert.Error(t, err)
}

func TestNVMeSubsystemRemoveNamespace(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	// case 1: No error while removing namespace to subsystem
	nsUUID := "fakeNsUUID"
	subsystemUUID := "fakeSubsystemUUID"
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).Return(nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID)
	assert.NoError(t, err)

	// case 2: Error returned while adding namespace to subsystem
	nsUUID = "fakeNsUUID"
	subsystemUUID = "fakeSubsystemUUID"
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).Return(errors.New("Error while removing NS to subsystem"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID)
	assert.Error(t, err)
}

func TestNVMeSubsystemGetNamespaceCount(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	// case 1: No error while removing namespace to subsystem
	subsystemUUID := "fakeSubsystemUUID"
	mock.EXPECT().NVMeNamespaceCount(ctx, subsystemUUID).Return(int64(1), nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	count, err := oapi.NVMeSubsystemGetNamespaceCount(ctx, subsystemUUID)
	assert.Equal(t, int64(1), count, "NVMe Subsystem count expected zero. It is non zero")
	assert.NoError(t, err)

	// case 2: Error returned while adding namespace to subsystem
	subsystemUUID = "fakeSubsystemUUID"
	mock.EXPECT().NVMeNamespaceCount(ctx, subsystemUUID).Return(int64(0), errors.New("Error while removing NS to subsystem"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	count, err = oapi.NVMeSubsystemGetNamespaceCount(ctx, subsystemUUID)
	assert.Equal(t, int64(0), count, "NVMe Subsystem count expected zero. It is non zero")
	assert.Error(t, err)
}

func TestNVMeSubsystemDelete(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	// case 1: No error while deleting subsystem
	subsystemUUID := "fakeSubsystemUUID"
	mock.EXPECT().NVMeSubsystemDelete(ctx, subsystemUUID).Return(nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeSubsystemDelete(ctx, subsystemUUID)
	assert.NoError(t, err)

	// case 2: Error returned while deleting subsystem
	subsystemUUID = "fakeSubsystemUUID"
	mock.EXPECT().NVMeSubsystemDelete(ctx, subsystemUUID).Return(errors.New("Error while deleting subsystem"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeSubsystemDelete(ctx, subsystemUUID)
	assert.Error(t, err)
}

func TestNVMeAddHostToSubsystem(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	// case 1: No error while adding host to subsystem
	hostNQN := "fakeNQN"
	hostNQN1 := "dummyNQN"
	host1 := &models.NvmeSubsystemHost{
		Nqn: &hostNQN,
	}
	subsystemUUID := "fakeSubsystemUUID"
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil)
	mock.EXPECT().NVMeAddHostNqnToSubsystem(ctx, hostNQN1, subsystemUUID).Return(nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeAddHostToSubsystem(ctx, hostNQN1, subsystemUUID)
	assert.NoError(t, err)

	// case 2: Error returned while getting host of subsystem
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return(nil, errors.New("Error while getting hosts for subsystem"))
	mock.EXPECT().NVMeAddHostNqnToSubsystem(ctx, hostNQN, subsystemUUID).AnyTimes().Return(nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeAddHostToSubsystem(ctx, hostNQN, subsystemUUID)
	assert.Error(t, err)

	// case 3: Error returned while adding host to subsystem
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil)
	mock.EXPECT().NVMeAddHostNqnToSubsystem(ctx, hostNQN1, subsystemUUID).Return(errors.New("Error while adding hosts to subsystem"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeAddHostToSubsystem(ctx, hostNQN1, subsystemUUID)
	assert.Error(t, err)

	// case 4: hostNQN is alreay added
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil)
	mock.EXPECT().NVMeAddHostNqnToSubsystem(ctx, hostNQN, subsystemUUID).AnyTimes().Return(nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeAddHostToSubsystem(ctx, hostNQN, subsystemUUID)
	assert.NoError(t, err)
}

func TestNVMeRemoveHostFromSubsystem(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	// case 1 : Error removing host from subsystem
	hostNQN := "fakeNQN"
	subsystemUUID := "fakesubsysUUID"
	host1 := &models.NvmeSubsystemHost{}

	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return(nil, errors.New("Error while getting hosts for subsystem"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	err = oapi.NVMeRemoveHostFromSubsystem(ctx, hostNQN, subsystemUUID)

	assert.Error(t, err)

	// case 2 : host not found
	Nqn := "wrongNQN"
	host1.Nqn = &Nqn
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	err = oapi.NVMeRemoveHostFromSubsystem(ctx, hostNQN, subsystemUUID)

	assert.NoError(t, err)

	// case 3 : host found but failed to remove it
	host1.Nqn = &hostNQN
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeRemoveHostFromSubsystem(ctx, hostNQN, subsystemUUID).Return(errors.New("Error while removing host"))

	err = oapi.NVMeRemoveHostFromSubsystem(ctx, hostNQN, subsystemUUID)

	assert.Error(t, err)

	// case 4 : Success- host found and removed it
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeRemoveHostFromSubsystem(ctx, hostNQN, subsystemUUID).Return(nil)

	err = oapi.NVMeRemoveHostFromSubsystem(ctx, hostNQN, subsystemUUID)

	assert.NoError(t, err)
}

func TestNVMeSubsystemCreate(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	// case 1: No error while adding host to subsystem
	subsystemName := "fakeSubsystem"
	subsystemComment := "fakeSubsystemComment"
	subsysUUID := "fakeUUID"
	targetNQN := "fakeTargetNQN"
	subsys := &models.NvmeSubsystem{
		Name:      &subsystemName,
		UUID:      &subsysUUID,
		TargetNqn: &targetNQN,
	}

	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName, gomock.Any()).Return(subsys, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	subsystem, err := oapi.NVMeSubsystemCreate(ctx, subsystemName, subsystemComment)
	assert.NoError(t, err)
	assert.Equal(t, subsystem.UUID, subsysUUID, "subsystem UUID does not match")
	assert.Equal(t, subsystem.Name, subsystemName, "subsystem name does not match")
	assert.Equal(t, subsystem.NQN, targetNQN, "host does not match")

	// case 2: Error getting susbsystem info from backend
	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName, gomock.Any()).Return(nil, errors.New("Error getting susbsystem info"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	_, err = oapi.NVMeSubsystemCreate(ctx, subsystemName, subsystemComment)
	assert.Error(t, err)

	// case 3: Subsystem not present, create a new one successfully
	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName, gomock.Any()).Return(nil, nil)
	mock.EXPECT().NVMeSubsystemCreate(ctx, subsystemName, subsystemComment).Return(subsys, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	newsubsys, err := oapi.NVMeSubsystemCreate(ctx, subsystemName, subsystemComment)
	assert.NoError(t, err)
	assert.Equal(t, newsubsys.UUID, subsysUUID, "subsystem UUID does not match")
	assert.Equal(t, newsubsys.Name, subsystemName, "subsystem name does not match")
	assert.Equal(t, newsubsys.NQN, targetNQN, "host does not match")

	// case 4: Subsystem not present, create a new one with failure
	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName, gomock.Any()).Return(nil, nil)
	mock.EXPECT().NVMeSubsystemCreate(ctx, subsystemName, subsystemComment).Return(nil, errors.New("Error creating susbsystem"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	newsubsys, err = oapi.NVMeSubsystemCreate(ctx, subsystemName, subsystemComment)
	assert.Error(t, err)

	// case 5: Subsystem not present, create a new one but returned nil
	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName, gomock.Any()).Return(nil, nil)
	mock.EXPECT().NVMeSubsystemCreate(ctx, subsystemName, subsystemComment).Return(nil, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	newsubsys, err = oapi.NVMeSubsystemCreate(ctx, subsystemName, subsystemComment)
	assert.Error(t, err)

	// case 6: Subsystem not present, create a new one but returned already exists error with code NVME_SUBSYSTEM_ALREADY_EXISTS
	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName, gomock.Any()).Return(nil, nil).Times(1)
	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName, gomock.Any()).Return(subsys, nil).Times(1)
	mockErr := &nvme.NvmeSubsystemCreateDefault{
		Payload: &models.ErrorResponse{
			Error: &models.ReturnedError{
				Code:    convert.ToPtr(api.NVME_SUBSYSTEM_ALREADY_EXISTS),
				Message: convert.ToPtr("NVMe subsystem already exists"),
			},
		},
	}
	mock.EXPECT().NVMeSubsystemCreate(ctx, subsystemName, subsystemComment).Return(nil, mockErr)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	newsubsys, err = oapi.NVMeSubsystemCreate(ctx, subsystemName, subsystemComment)
	assert.NoError(t, err)
	assert.Equal(t, newsubsys.UUID, subsysUUID, "subsystem UUID does not match")
	assert.Equal(t, newsubsys.Name, subsystemName, "subsystem name does not match")

	// case 7: Subsystem not present, create a new one but returned already exists error with code NVME_SUBSYSTEM_ALREADY_EXISTS
	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName, gomock.Any()).Return(nil, nil).Times(1)
	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName, gomock.Any()).Return(nil, nil).Times(1)
	mockErr = &nvme.NvmeSubsystemCreateDefault{
		Payload: &models.ErrorResponse{
			Error: &models.ReturnedError{
				Code:    convert.ToPtr(api.NVME_SUBSYSTEM_ALREADY_EXISTS),
				Message: convert.ToPtr("NVMe subsystem already exists"),
			},
		},
	}
	mock.EXPECT().NVMeSubsystemCreate(ctx, subsystemName, subsystemComment).Return(nil, mockErr)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	newsubsys, err = oapi.NVMeSubsystemCreate(ctx, subsystemName, subsystemComment)
	assert.Error(t, err)
	assert.Nil(t, newsubsys, "subsystem expected to be nil when error is returned")
}

func TestNVMeEnsureNamespaceMapped(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	subsystem := &api.NVMeSubsystem{
		Name: "fakeSubsysName",
		NQN:  "fakeNQN",
		UUID: "fakeUUID",
	}

	nsUUID := "fakeNsUUID"

	// case 1: Error getting namespace from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystem.UUID, nsUUID).Return(false, errors.New("Error getting namespace subsystem mapping"))

	err = oapi.NVMeEnsureNamespaceMapped(ctx, subsystem.UUID, nsUUID)

	assert.Error(t, err)

	// case 2: Namespace is already mapped
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystem.UUID, nsUUID).Return(true, nil)

	err = oapi.NVMeEnsureNamespaceMapped(ctx, subsystem.UUID, nsUUID)

	assert.NoError(t, err)

	// case 3: Namespace is not mapped but adding host to subsystem returned error
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystem.UUID, nsUUID).Return(false, nil)
	mock.EXPECT().NVMeSubsystemAddNamespace(ctx, subsystem.UUID, nsUUID).Return(errors.New("Error adding host to subsystem"))

	err = oapi.NVMeEnsureNamespaceMapped(ctx, subsystem.UUID, nsUUID)

	assert.Error(t, err)

	// case 4: Namespace is not mapped add host is added to subsystem successfully
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystem.UUID, nsUUID).Return(false, nil)
	mock.EXPECT().NVMeSubsystemAddNamespace(ctx, subsystem.UUID, nsUUID).Return(nil)

	err = oapi.NVMeEnsureNamespaceMapped(ctx, subsystem.UUID, nsUUID)

	assert.NoError(t, err)
}

func TestNVMeNamespaceUnmapped(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	subsystemUUID := "fakeSubsysUUID"
	nsUUID := "fakeNsUUID"
	hostNQN := "fakeHostNQN"
	host2NQN := "fakeHost2NQN"
	nonExistentHostNQN := "fakeHostNQNNonExisting"

	host1 := &models.NvmeSubsystemHost{Nqn: &hostNQN}
	host2 := &models.NvmeSubsystemHost{Nqn: &host2NQN}
	var removePublishInfo bool

	// case 1: Error getting namespace from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(false, errors.New("Error getting namespace subsystem mapping")).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo, "subsystem removed")
	assert.Error(t, err)

	// case 2: Namespace is not mapped
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(false, nil).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, true, removePublishInfo, "subsystem is not removedd")
	assert.NoError(t, err)

	// case 3: Failed to get hosts of the subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return(nil, errors.New("failed to get hosts")).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo, "subsystem removed")
	assert.Error(t, err)

	// case 4: hosts of the subsystem not returned
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return(nil, nil).Times(1)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).AnyTimes().Return(nil).Times(1)
	mock.EXPECT().NVMeNamespaceCount(ctx, subsystemUUID).Return(int64(0), nil).Times(1)
	mock.EXPECT().NVMeSubsystemDelete(ctx, subsystemUUID).Return(nil).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, true, removePublishInfo, "subsystem removed")
	assert.NoError(t, err)

	// case 5: multiple hosts of the subsystem returned  but error while removing host from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1, host2}, nil).Times(1)
	mock.EXPECT().NVMeRemoveHostFromSubsystem(ctx, hostNQN, subsystemUUID).Return(errors.New("Error removing host from subsystem")).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo, "subsystem removed")
	assert.Error(t, err)

	// case 6: multiple hosts of the subsystem returned  and success while removing host from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1, host2}, nil).Times(1)
	mock.EXPECT().NVMeRemoveHostFromSubsystem(ctx, hostNQN, subsystemUUID).Return(nil).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo, "subsystem is not removed")
	assert.NoError(t, err)

	// case 7: Error removing namespace from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil).Times(1)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).Return(errors.New("Error removing namespace from subsystem")).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo, "subsystem removed")
	assert.Error(t, err)

	// case 8: Error getting namespace count from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil).Times(1)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).Return(nil).Times(1)
	mock.EXPECT().NVMeNamespaceCount(ctx, subsystemUUID).Return(int64(0), errors.New("Error getting namespace count from subsystem")).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo, "subsystem removed")
	assert.Error(t, err)

	// case 9: Error deleting subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).Return(nil).Times(1)
	mock.EXPECT().NVMeNamespaceCount(ctx, subsystemUUID).Return(int64(0), nil).Times(1)
	mock.EXPECT().NVMeSubsystemDelete(ctx, subsystemUUID).Return(errors.New("Error deleting subsystem")).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo, "subsystem removed")
	assert.Error(t, err)

	// case 10: Success deleting subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).AnyTimes().Return(nil).Times(1)
	mock.EXPECT().NVMeNamespaceCount(ctx, subsystemUUID).Return(int64(0), nil).Times(1)
	mock.EXPECT().NVMeSubsystemDelete(ctx, subsystemUUID).Return(nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, true, removePublishInfo, "subsystem is not removed")
	assert.NoError(t, err)

	// case 11: Success when the nqn is already removed from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1},
		nil).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, nonExistentHostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo, "nqn is unmapped")
	assert.NoError(t, err)

	// case 12: Success when the nqn is already removed from subsystem with multiple hosts
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1, host2},
		nil).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, nonExistentHostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo, "nqn is unmapped")
	assert.NoError(t, err)
}

func TestNVMeIsNamespaceMapped(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	subsystemUUID := "fakeSubsysUUID"
	nsUUID := "fakeNsUUID"

	// case 1: Error checking if namespace is mapped
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(false, errors.New(" Error checking if namespace is mapped"))

	_, err = oapi.NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID)

	assert.Error(t, err)

	// case 2: namespace is mapped
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil)

	isMapped, err := oapi.NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID)

	assert.NoError(t, err)
	assert.Equal(t, true, isMapped, "namespace is not mapped")

	// case 2: namespace is not mapped
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(false, nil)

	isMapped, err = oapi.NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID)

	assert.NoError(t, err)
	assert.Equal(t, false, isMapped, "namespace is mapped")
}

func TestVolumeWaitForStates(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	ctrl := gomock.NewController(t)
	mock := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	volName := "fakeVolName"
	desiredStates := []string{"online"}
	abortStates := []string{""}
	onlineState := "online"
	errorState := "error"
	volume := &models.Volume{
		State: &onlineState,
	}
	maxElapsedTime := 2 * time.Second

	// Test1: Error - While getting the volume
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().VolumeGetByName(ctx, volName, gomock.Any()).AnyTimes().Return(nil, errors.New("Error getting the volume"))

	currentState, err := oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, abortStates, maxElapsedTime)

	assert.Error(t, err)
	assert.Equal(t, currentState, "", "volume state does not match")

	ctrl.Finish()

	// Test2: Error - Volume not found
	ctrl = gomock.NewController(t)
	mock = mockapi.NewMockRestClientInterface(ctrl)
	oapi, err = api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().VolumeGetByName(ctx, volName, gomock.Any()).AnyTimes().Return(nil, nil)

	currentState, err = oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, abortStates, maxElapsedTime)

	assert.Error(t, err)
	assert.Equal(t, currentState, "", "volume state does not match")

	ctrl.Finish()

	// Test3: Error - Volume state not in desired states
	ctrl = gomock.NewController(t)
	mock = mockapi.NewMockRestClientInterface(ctrl)
	oapi, err = api.NewOntapAPIRESTFromRestClientInterface(mock)

	mock.EXPECT().VolumeGetByName(ctx, volName, gomock.Any()).Return(volume, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	currentState, err = oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, abortStates, maxElapsedTime)

	assert.NoError(t, err)
	assert.Equal(t, currentState, *volume.State, "volume state does not match")

	ctrl.Finish()

	// Test4: Error - Volume reaches an abort state
	ctrl = gomock.NewController(t)
	mock = mockapi.NewMockRestClientInterface(ctrl)
	oapi, err = api.NewOntapAPIRESTFromRestClientInterface(mock)
	desiredStates = []string{""}
	abortStates = []string{"error"}
	volume.State = &errorState

	mock.EXPECT().VolumeGetByName(ctx, volName, gomock.Any()).Return(volume, nil).AnyTimes()
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	currentState, err = oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, abortStates, maxElapsedTime)

	assert.Error(t, err)
	assert.Equal(t, currentState, *volume.State, "volume state does not match")

	ctrl.Finish()

	// Test5: Error - Volume doesn't reach desired state or abort state
	ctrl = gomock.NewController(t)
	mock = mockapi.NewMockRestClientInterface(ctrl)
	oapi, err = api.NewOntapAPIRESTFromRestClientInterface(mock)
	desiredStates = []string{""}
	abortStates = []string{"fakeerrorState"}
	volume.State = &errorState

	mock.EXPECT().VolumeGetByName(ctx, volName, gomock.Any()).Return(volume, nil).AnyTimes()
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	currentState, err = oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, abortStates, maxElapsedTime)

	assert.Error(t, err)
	assert.Equal(t, currentState, *volume.State, "volume state does not match")

	ctrl.Finish()

	// Test6: Error - Volume is in unknown state
	ctrl = gomock.NewController(t)
	mock = mockapi.NewMockRestClientInterface(ctrl)
	oapi, err = api.NewOntapAPIRESTFromRestClientInterface(mock)
	desiredStates = []string{""}
	var newAbortState []string
	volState := "unknown"
	volume.State = &volState

	mock.EXPECT().VolumeGetByName(ctx, volName, gomock.Any()).Return(volume, nil).AnyTimes()
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	currentState, err = oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, newAbortState, maxElapsedTime)

	assert.Error(t, err)
	assert.Equal(t, currentState, *volume.State, "volume state does not match")

	ctrl.Finish()

	// Test7: Success - Volume is in desired state
	ctrl = gomock.NewController(t)
	mock = mockapi.NewMockRestClientInterface(ctrl)
	oapi, err = api.NewOntapAPIRESTFromRestClientInterface(mock)
	desiredStates = []string{"online"}
	volState = "online"
	volume.State = &volState

	mock.EXPECT().VolumeGetByName(ctx, volName, gomock.Any()).Return(volume, nil).AnyTimes()
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	currentState, err = oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, newAbortState, maxElapsedTime)

	assert.NoError(t, err)
	assert.Equal(t, currentState, *volume.State, "volume state does not match")

	ctrl.Finish()
}

func newMockOntapAPIREST(t *testing.T) (api.OntapAPIREST, *mockapi.MockRestClientInterface) {
	ctrl := gomock.NewController(t)

	rsi := mockapi.NewMockRestClientInterface(ctrl)
	ontapAPIREST, _ := api.NewOntapAPIRESTFromRestClientInterface(rsi)
	return ontapAPIREST, rsi
}

func newMockOntapAPIRESTWithController(t *testing.T) (*api.OntapAPIREST, *mockapi.MockRestClientInterface, *gomock.Controller) {
	ctrl := gomock.NewController(t)
	rsi := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(rsi)
	assert.NoError(t, err)
	return &oapi, rsi, ctrl
}

func TestNVMeNamespaceSetSize(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	oapi, mock := newMockOntapAPIREST(t)

	ns := api.NVMeNamespace{
		Name: "fakeNS",
		UUID: "fakeUUID",
	}
	// case 1: No error while upating size
	mock.EXPECT().NVMeNamespaceSetSize(ctx, "fakeUUID", int64(3221225472)).Return(nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err := oapi.NVMeNamespaceSetSize(ctx, ns.UUID, int64(3221225472))
	assert.NoError(t, err, "failed to modify size on NVMe namespace")

	// case 2: Error returned while upating size
	mock.EXPECT().NVMeNamespaceSetSize(ctx, "fakeUUID", int64(3221225472)).Return(errors.New(
		"Error while updating size"))
	err = oapi.NVMeNamespaceSetSize(ctx, ns.UUID, int64(3221225472))
	assert.Error(t, err)
}

func TestIgroupList(t *testing.T) {
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	oapi, mock := newMockOntapAPIREST(t)

	igroupName := "igroup1"
	subsysUUID := "fakeUUID"
	igroup := models.Igroup{
		Name: &igroupName,
		UUID: &subsysUUID,
	}
	igroupList := []*models.Igroup{&igroup}
	numRecords := int64(1)
	igroupResponse := &models.IgroupResponse{
		IgroupResponseInlineRecords: igroupList,
		NumRecords:                  &numRecords,
	}
	igroupResponseOK := s_a_n.IgroupCollectionGetOK{Payload: igroupResponse}

	// case 1: No error while getting Igroup list
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().IgroupList(ctx, "", gomock.Any()).Return(&igroupResponseOK, nil)

	igroups, err1 := oapi.IgroupList(ctx)
	assert.NoError(t, err1, "error while getting Igroup list")
	assert.Equal(t, len(igroups), len(igroupList))
	assert.Equal(t, igroups[0], igroupName)

	// case 2: Error returned while getting igroup list
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().IgroupList(ctx, "", gomock.Any()).Return(nil, errors.New("failed to get igroup"))

	_, err1 = oapi.IgroupList(ctx)
	assert.Error(t, err1, "No error while getting Igroup list")

	// case 3: Empty payload returned while getting igroup list
	igroupResponseOK = s_a_n.IgroupCollectionGetOK{Payload: nil}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().IgroupList(ctx, "", gomock.Any()).Return(&igroupResponseOK, nil)

	igroups, err1 = oapi.IgroupList(ctx)
	assert.NoError(t, err1, "error while getting Igroup list")
	assert.Nil(t, igroups)

	// case 4: Nil NumRecords field returned while getting igroup list
	igroupResponse = &models.IgroupResponse{
		IgroupResponseInlineRecords: igroupList,
		NumRecords:                  nil,
	}
	igroupResponseOK = s_a_n.IgroupCollectionGetOK{Payload: igroupResponse}

	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().IgroupList(ctx, "", gomock.Any()).Return(&igroupResponseOK, nil)

	igroups, err1 = oapi.IgroupList(ctx)
	assert.NoError(t, err1, "error while getting Igroup list")
	assert.Nil(t, igroups)
}

func TestIgroupRemove(t *testing.T) {
	initiator := "initiator"
	initiatorGroup := "initiatorGroup"

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	oapi, mock := newMockOntapAPIREST(t)

	// case 1: No Error returned while deleting the igroup.
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().IgroupRemove(ctx, initiator, initiatorGroup).Return(nil)

	err := oapi.IgroupRemove(ctx, initiator, initiatorGroup, false)
	assert.NoError(t, err, "error returned while deleting the igroup")

	// case 2: Error returned while deleting the igroup.
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().IgroupRemove(ctx, initiator, initiatorGroup).Return(errors.New("failed to remove igroup"))

	err = oapi.IgroupRemove(ctx, initiator, initiatorGroup, false)
	assert.Error(t, err, "No error returned while deleting the igroup.")
}

func TestIgroupGetByName(t *testing.T) {
	initiator1 := "initiator1"
	initiator2 := "initiator2"
	initiatorGroup := "initiatorGroup"

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	oapi, mock := newMockOntapAPIREST(t)

	igroupName := "igroup1"
	subsysUUID := "fakeUUID"
	igroupList1 := models.IgroupInlineInitiatorsInlineArrayItem{
		Name: &initiator1,
	}
	igroupList2 := models.IgroupInlineInitiatorsInlineArrayItem{
		Name: &initiator2,
	}
	igroupResponse := []*models.IgroupInlineInitiatorsInlineArrayItem{&igroupList1, &igroupList2}
	igroup := models.Igroup{
		Name:                   &igroupName,
		UUID:                   &subsysUUID,
		IgroupInlineInitiators: igroupResponse,
	}

	// case 1: No Error returned while getting igroup by name.
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().IgroupGetByName(ctx, initiatorGroup, gomock.Any()).Return(&igroup, nil)

	mappedIQNs, err1 := oapi.IgroupGetByName(ctx, initiatorGroup)
	assert.NoError(t, err1, "error returned while getting igroup by name")
	assert.Equal(t, 2, len(mappedIQNs))
	assert.Equal(t, true, mappedIQNs[initiator1])
	assert.Equal(t, true, mappedIQNs[initiator2])

	// case 2: Error returned while getting igroup by name.
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().IgroupGetByName(ctx, initiatorGroup, gomock.Any()).Return(nil, errors.New("Failed to get igroup by name"))

	_, err1 = oapi.IgroupGetByName(ctx, initiatorGroup)

	assert.Error(t, err1, "no error returned while getting igroup by name")
}

func TestGetSLMDataLifs(t *testing.T) {
	nodeName := "node1"
	ipAddress := models.IPAddress("1.1.1.1")
	ipFamily := models.IPAddressFamily("ipv4")

	oapi, mock := newMockOntapAPIREST(t)

	node := models.IPInterfaceInlineLocationInlineNode{
		Name: &nodeName,
	}

	location := models.IPInterfaceInlineLocation{Node: &node}
	ip := models.IPInfo{Address: &ipAddress, Family: &ipFamily}
	ipInterface := models.IPInterface{Location: &location, IP: &ip}
	ipInterfaceResponse := models.IPInterfaceResponse{
		IPInterfaceResponseInlineRecords: []*models.IPInterface{&ipInterface},
	}
	ipInterfaceResponseOk := networking.NetworkIPInterfacesGetOK{Payload: &ipInterfaceResponse}

	// case 1: No Error returned while getting IP interface.
	mock.EXPECT().NetworkIPInterfacesList(ctx).Return(&ipInterfaceResponseOk, nil)
	reportedDataLIFs, err1 := oapi.GetSLMDataLifs(ctx, []string{"1.1.1.1", "2.2.2.2"}, []string{"node1"})
	assert.NoError(t, err1, "error returned while getting IP interface")
	assert.Equal(t, 1, len(reportedDataLIFs))

	// case 2: Nil response returned while getting IP interface.
	mock.EXPECT().NetworkIPInterfacesList(ctx).Return(nil, nil)
	reportedDataLIFs, err1 = oapi.GetSLMDataLifs(ctx, []string{"1.1.1.1", "2.2.2.2"}, []string{"node1"})
	assert.NoError(t, err1, "error returned while getting IP interface")
	assert.Nil(t, reportedDataLIFs)

	// case 3: Error returned while getting IP interface.
	mock.EXPECT().NetworkIPInterfacesList(ctx).Return(nil, errors.New("failed to get network info"))
	reportedDataLIFs, err1 = oapi.GetSLMDataLifs(ctx, []string{"1.1.1.1", "2.2.2.2"}, []string{"node1"})
	assert.Error(t, err1, "no error returned while getting IP interface")
	assert.Nil(t, reportedDataLIFs)

	// case 4: Payload returned Nil while getting IP interface.
	ipInterfaceResponseOk = networking.NetworkIPInterfacesGetOK{Payload: nil}
	mock.EXPECT().NetworkIPInterfacesList(ctx).Return(&ipInterfaceResponseOk, nil)
	reportedDataLIFs, err1 = oapi.GetSLMDataLifs(ctx, []string{"1.1.1.1", "2.2.2.2"}, []string{"node1"})
	assert.NoError(t, err1, "error returned while getting IP interface")
	assert.Nil(t, reportedDataLIFs)

	// case 5: Location and IP returned NIL while getting IP interface.
	ipInterfaceResponse = models.IPInterfaceResponse{
		IPInterfaceResponseInlineRecords: []*models.IPInterface{
			{Location: nil, IP: nil},
		},
	}
	ipInterfaceResponseOk = networking.NetworkIPInterfacesGetOK{Payload: &ipInterfaceResponse}
	mock.EXPECT().NetworkIPInterfacesList(ctx).Return(&ipInterfaceResponseOk, nil)
	reportedDataLIFs, err1 = oapi.GetSLMDataLifs(ctx, []string{"1.1.1.1", "2.2.2.2"}, []string{"node1"})
	assert.NoError(t, err1, "error returned while getting IP interface")
	assert.Equal(t, 0, len(reportedDataLIFs))

	// case 6: Pass empty data Lifs
	reportedDataLIFs, err1 = oapi.GetSLMDataLifs(ctx, []string{}, []string{"node1"})
	assert.NoError(t, err1, "error returned while getting IP interface")
}

func TestGetSVMUUID(t *testing.T) {
	oapi, mock := newMockOntapAPIREST(t)

	// case 1: No Error returned while getting SVM UUID.
	mock.EXPECT().SVMUUID().Return("12345")
	svmUUID := oapi.GetSVMUUID()
	assert.Equal(t, "12345", svmUUID)
}

func TestGetSVMState(t *testing.T) {
	oapi, mock := newMockOntapAPIREST(t)

	// case 1: No Error returned while getting SVM state.
	mock.EXPECT().GetSVMState(ctx).Return("online", nil)

	state, err1 := oapi.GetSVMState(ctx)
	assert.NoError(t, err1, "error returned while getting SVM state.")
	assert.Equal(t, "online", state)

	// case 2: Error returned while getting SVM state.
	mock.EXPECT().GetSVMState(ctx).Return("", errors.New("failed to get SVM state"))

	_, err1 = oapi.GetSVMState(ctx)
	assert.Error(t, err1, "no error returned while getting SVM state.")
}

func TestSMBShareCreate(t *testing.T) {
	oapi, mock := newMockOntapAPIREST(t)

	// case 1: No Error returned while creating SMB share.
	mock.EXPECT().SMBShareCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
	err1 := oapi.SMBShareCreate(ctx, "", "")
	assert.NoError(t, err1, "error returned while creating SMB share")

	// case 2: Error returned while creating SMB share.
	mock.EXPECT().SMBShareCreate(ctx, gomock.Any(), gomock.Any()).Return(errors.New("failed to create SMB share"))
	err1 = oapi.SMBShareCreate(ctx, "", "")
	assert.Error(t, err1, "no error returned while creating SMB share")
}

func TestSMBShareExists(t *testing.T) {
	oapi, mock := newMockOntapAPIREST(t)

	// case 1: No Error returned while getting SMB share.
	mock.EXPECT().SMBShareExists(ctx, gomock.Any()).Return(true, nil)
	shareExists, err1 := oapi.SMBShareExists(ctx, "")
	assert.NoError(t, err1, "error returned while getting SMB share")
	assert.Equal(t, true, shareExists)

	// case 2: Error returned while getting SMB share.
	mock.EXPECT().SMBShareExists(ctx, gomock.Any()).Return(false, errors.New("failed to verify SMB share"))
	shareExists, err1 = oapi.SMBShareExists(ctx, "")
	assert.Error(t, err1, "no error returned while getting SMB share")
	assert.Equal(t, false, shareExists)
}

func TestSMBShareDestroy(t *testing.T) {
	oapi, mock := newMockOntapAPIREST(t)

	// case 1: No Error returned while deleting SMB share.
	mock.EXPECT().SMBShareDestroy(ctx, "").Return(nil)
	err1 := oapi.SMBShareDestroy(ctx, "")
	assert.NoError(t, err1, "error returned while deleting SMB share")

	// case 2: Error returned while deleting SMB share.
	mock.EXPECT().SMBShareDestroy(ctx, "").Return(errors.New("failed to destroy SMB share"))
	err1 = oapi.SMBShareDestroy(ctx, "")
	assert.Error(t, err1, "no error returned while deleting SMB share")
}

func TestSMBShareAccessControlCreate(t *testing.T) {
	oapi, mock := newMockOntapAPIREST(t)

	// case 1: No Error returned while creating SMB share access control.
	mock.EXPECT().SMBShareAccessControlCreate(ctx, gomock.Any(), gomock.Any()).Return(nil)
	err1 := oapi.SMBShareAccessControlCreate(ctx, "", nil)
	assert.NoError(t, err1, "error returned while creating SMB share access control")

	// case 2: Error returned while creating SMB share access control.
	mock.EXPECT().SMBShareAccessControlCreate(ctx, gomock.Any(), gomock.Any()).Return(
		errors.New("failed to create SMB share access control"))
	err1 = oapi.SMBShareAccessControlCreate(ctx, "", nil)
	assert.Error(t, err1, "no error returned while creating SMB share access control")
}

func TestSMBShareAccessControlDelete(t *testing.T) {
	oapi, mock := newMockOntapAPIREST(t)

	// case 1: No Error returned while deleting SMB share access control.
	mock.EXPECT().SMBShareAccessControlDelete(ctx, gomock.Any(), gomock.Any()).Return(nil)
	err1 := oapi.SMBShareAccessControlDelete(ctx, "", nil)
	assert.NoError(t, err1, "error returned while deleting SMB share access control")

	// case 2: Error returned while deleting SMB share access control.
	mock.EXPECT().SMBShareAccessControlDelete(ctx, gomock.Any(), gomock.Any()).Return(errors.New("failed to delete SMB share access control"))
	err1 = oapi.SMBShareAccessControlDelete(ctx, "", nil)
	assert.Error(t, err1, "no error returned while deleting SMB share access control")
}

func TestGetRestErr(t *testing.T) {
	payload := models.Job{}

	restErr := api.NewRestErrorFromPayload(&payload)
	assert.Error(t, restErr, "no error while creating rest error instance")

	restErr = api.NewRestErrorFromPayload(nil)
	assert.Error(t, restErr, "no error while creating rest error instance")
}

func TestNewOntapAPIREST(t *testing.T) {
	_, err := api.NewOntapAPIREST(nil, "ontap-nas")
	assert.NoError(t, err, "error while ontap api instance")
}

func TestSuccess(t *testing.T) {
	success := "success"
	payload := models.Job{State: &success}

	restErr := api.NewRestErrorFromPayload(&payload)

	is := restErr.IsSuccess()
	assert.Equal(t, true, is)
}

func TestFailure(t *testing.T) {
	success := "failure"
	payload := models.Job{State: &success}

	restErr := api.NewRestErrorFromPayload(&payload)

	is := restErr.IsFailure()
	assert.Equal(t, true, is)
}

func TestError(t *testing.T) {
	success := "success"
	payload := models.Job{State: &success}

	restErr := api.NewRestErrorFromPayload(&payload)

	is := restErr.Error()
	assert.Equal(t, "API status: success", is)

	success = "failure"
	payload = models.Job{State: &success}

	restErr = api.NewRestErrorFromPayload(&payload)

	is = restErr.Error()
	assert.Contains(t, is, "API State: failure")
}

func TestRestError(t *testing.T) {
	uuid := strfmt.UUID("12345")
	description := "rest error"
	state := "success"
	message := ""
	code := int64(1638555)
	startTime := strfmt.NewDateTime()
	endTime := strfmt.NewDateTime()

	payload := models.Job{
		UUID:        &uuid,
		Description: &description,
		State:       &state,
		Message:     &message,
		Code:        &code,
		StartTime:   &startTime,
		EndTime:     &endTime,
	}

	restErr := api.NewRestErrorFromPayload(&payload)

	snapshotBusy := restErr.IsSnapshotBusy()
	assert.Equal(t, true, snapshotBusy)

	errState := restErr.State()
	assert.Equal(t, "success", errState)

	msg := restErr.Message()
	assert.Equal(t, "", msg)

	errCode := restErr.Code()
	assert.Equal(t, "1638555", errCode)
}

func TestSVMName(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: No Error returned while verifying SVM name.
	rsi.EXPECT().SVMName().Return("svm1")
	svmName := oapi.SVMName()
	assert.Equal(t, "svm1", svmName)
}

func TestVolumeCreate(t *testing.T) {
	encrypt := false
	volume := api.Volume{
		Name:            "vol1",
		Aggregates:      []string{"aggr1"},
		Size:            "1g",
		SpaceReserve:    "10",
		SnapshotPolicy:  "none",
		UnixPermissions: "777",
		ExportPolicy:    "",
		SecurityStyle:   "unix",
		TieringPolicy:   "",
		SnapshotReserve: 10,
		Comment:         "",
		DPVolume:        true,
		Encrypt:         &encrypt,
		Qos:             api.QosPolicyGroup{},
	}
	oapi, rsi := newMockOntapAPIREST(t)

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	// common call for all subtests
	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// case 1: Create volume, returned No error
	rsi.EXPECT().VolumeCreate(ctx, volume.Name, volume.Aggregates[0], volume.Size, volume.SpaceReserve,
		volume.SnapshotPolicy, volume.UnixPermissions, volume.ExportPolicy, volume.SecurityStyle,
		volume.TieringPolicy, volume.Comment, volume.Qos, volume.Encrypt, volume.SnapshotReserve, volume.DPVolume).
		Return(nil)
	err := oapi.VolumeCreate(ctx, volume)
	assert.NoError(t, err, "error returned while creating volume")

	// case 2: Create volume, volume creation failed
	rsi.EXPECT().VolumeCreate(ctx, volume.Name, volume.Aggregates[0], volume.Size, volume.SpaceReserve,
		volume.SnapshotPolicy, volume.UnixPermissions, volume.ExportPolicy, volume.SecurityStyle,
		volume.TieringPolicy, volume.Comment, volume.Qos, volume.Encrypt, volume.SnapshotReserve, volume.DPVolume).
		Return(errors.New("Volume create failed"))
	err = oapi.VolumeCreate(ctx, volume)
	assert.Error(t, err, "no error returned while creating volume")
}

func TestVolumeDestroy(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Delete volume, Positive test
	rsi.EXPECT().VolumeDestroy(ctx, "vol1", false).Return(nil)
	err := oapi.VolumeDestroy(ctx, "vol1", false, false)
	assert.NoError(t, err, "error returned while deleting a volume")

	// case 2: Delete volume, returned error.
	rsi.EXPECT().VolumeDestroy(ctx, "vol1", false).Return(errors.New("failed to delete volume"))
	err = oapi.VolumeDestroy(ctx, "vol1", false, false)
	assert.Error(t, err, "no error returned while deleting a volume")
}

func TestVolumeInfo(t *testing.T) {
	volume := getVolumeInfo()

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Get volume. Returned volume list
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	_, err := oapi.VolumeInfo(ctx, "vol1")
	assert.NoError(t, err, "error returned while getting a volume")

	// case 2: Get volume. Returned empty list
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(nil, nil)
	_, err = oapi.VolumeInfo(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a volume")

	// case 3: Get volume. Backend returned an error.
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(nil, errors.New("Failed to get volume"))
	_, err = oapi.VolumeInfo(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a volume")

	// case 4: Get volume. Returned volume list with volume name nil
	// Want an error from "VolumeInfoFromRestAttrsHelper" function.
	volume.Name = nil
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	_, err = oapi.VolumeInfo(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a volume")
}

func TestNodeListSerialNumbers(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Get node serial number
	rsi.EXPECT().NodeListSerialNumbers(ctx).Return([]string{"fakeSerialNumber"}, nil)
	serialNumber, err := oapi.NodeListSerialNumbers(ctx)
	assert.NoError(t, err, "error returned while getting a node serial number")
	assert.Equal(t, "fakeSerialNumber", serialNumber[0])
}

func TestSupportsFeature(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Get supported feature.
	feature := api.Feature("ontapFeature")
	rsi.EXPECT().SupportsFeature(ctx, feature).Return(true)
	isSupported := oapi.SupportsFeature(ctx, feature)
	assert.Equal(t, true, isSupported)
}

func TestNetInterfaceGetDataLIFs(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Get the data Lifs
	rsi.EXPECT().NetInterfaceGetDataLIFs(ctx, "iscsi").Return([]string{"1.1.1.1", "1.2.3.4"}, nil)
	datalifs, err := oapi.NetInterfaceGetDataLIFs(ctx, "iscsi")
	assert.NoError(t, err, "error returned while getting a data Lifs")
	assert.Equal(t, 2, len(datalifs))
	assert.Equal(t, "1.1.1.1", datalifs[0])
}

func TestGetSVMAggregateNames(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Get the SVM aggregate names
	rsi.EXPECT().SVMGetAggregateNames(ctx).Return([]string{"aggr1", "aggr2"}, nil)
	svms, err := oapi.GetSVMAggregateNames(ctx)
	assert.NoError(t, err, "error returned while getting a SVM aggregate names")
	assert.Equal(t, 2, len(svms))
	assert.Equal(t, "aggr1", svms[0])
}

func TestEmsAutosupportLog(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().EmsAutosupportLog(ctx, "9.13.1", true, "", "CN-USERS", "", 123, "", 0).
		Return(nil)
	oapi.EmsAutosupportLog(ctx, "ontap-nas", "9.13.1", true, "", "CN-USERS",
		"", 123, "", 0)

	rsi.EXPECT().EmsAutosupportLog(ctx, "9.13.1", true, "", "CN-USERS", "", 123, "", 0).
		Return(errors.New("Could not generate Autosupport message"))
	oapi.EmsAutosupportLog(ctx, "ontap-nas", "9.13.1", true, "", "CN-USERS",
		"", 123, "", 0)
}

func TestFlexgroupExists(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup exists positive test case
	rsi.EXPECT().FlexGroupExists(ctx, "flexGroupVolume").Return(true, nil)
	isFlexGroupExists, err := oapi.FlexgroupExists(ctx, "flexGroupVolume")
	assert.NoError(t, err, "error returned while getting a flexgroup volume")
	assert.Equal(t, true, isFlexGroupExists)
}

func TestFlexgroupCreate(t *testing.T) {
	encrypt := false
	volume := api.Volume{
		Name:            "vol1",
		Aggregates:      []string{"aggr1"},
		Size:            "1073741824000",
		SpaceReserve:    "10",
		SnapshotPolicy:  "none",
		UnixPermissions: "777",
		ExportPolicy:    "",
		SecurityStyle:   "unix",
		TieringPolicy:   "",
		SnapshotReserve: 10,
		Comment:         "",
		DPVolume:        true,
		Encrypt:         &encrypt,
		Qos:             api.QosPolicyGroup{},
	}
	oapi, rsi := newMockOntapAPIREST(t)

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	// common call for all subtests
	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// case 1: Flexgroup create, positive test case
	rsi.EXPECT().FlexGroupCreate(ctx, volume.Name, 1073741824000, volume.Aggregates, volume.SpaceReserve,
		volume.SnapshotPolicy, volume.UnixPermissions, volume.ExportPolicy, volume.SecurityStyle,
		volume.TieringPolicy, volume.Comment, volume.Qos, volume.Encrypt, volume.SnapshotReserve).
		Return(nil)
	err := oapi.FlexgroupCreate(ctx, volume)
	assert.NoError(t, err, "error returned while creating a flexgroup volume")

	// case 2: Flexgroup create, negative test case
	rsi.EXPECT().FlexGroupCreate(ctx, volume.Name, 1073741824000, volume.Aggregates, volume.SpaceReserve,
		volume.SnapshotPolicy, volume.UnixPermissions, volume.ExportPolicy, volume.SecurityStyle,
		volume.TieringPolicy, volume.Comment, volume.Qos, volume.Encrypt, volume.SnapshotReserve).
		Return(errors.New("flexgroup volume creation failed"))
	err = oapi.FlexgroupCreate(ctx, volume)
	assert.Error(t, err, "no error returned while creating a flexgroup volume")
}

func TestFlexgroupCloneSplitStart(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup spilt clone positive test.
	rsi.EXPECT().FlexgroupCloneSplitStart(ctx, "fake-cloneVolume").Return(nil)
	err := oapi.FlexgroupCloneSplitStart(ctx, "fake-cloneVolume")
	assert.NoError(t, err, "error returned while spilt clone a flexgroup volume")

	// case 2: Flexgroup split clone negative test
	rsi.EXPECT().FlexgroupCloneSplitStart(ctx, "fake-cloneVolume").Return(
		errors.New("flexgroup volume splits clone failed"))
	err = oapi.FlexgroupCloneSplitStart(ctx, "fake-cloneVolume")
	assert.Error(t, err, "no error returned while spilt clone a flexgroup volume")
}

func TestFlexgroupDisableSnapshotDirectoryAccess(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup disable snapshot directory access positive test.
	rsi.EXPECT().FlexGroupVolumeModifySnapshotDirectoryAccess(ctx, "fake-volume", false).Return(nil)
	err := oapi.FlexgroupModifySnapshotDirectoryAccess(ctx, "fake-volume", false)
	assert.NoError(t, err, "error returned while disable snapshot dir access of a flexgroup volume")

	// case 2: Flexgroup disable snapshot directory access negative test.
	rsi.EXPECT().FlexGroupVolumeModifySnapshotDirectoryAccess(ctx, "fake-volume", false).Return(
		errors.New("flexgroup volume disable snapshot directory failed"))
	err = oapi.FlexgroupModifySnapshotDirectoryAccess(ctx, "fake-volume", false)
	assert.Error(t, err, "No error returned while disable snapshot dir access of a flexgroup volume")
}

func TestFlexgroupInfo(t *testing.T) {
	volume := getVolumeInfo()
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup get by name
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	_, err := oapi.FlexgroupInfo(ctx, "vol1")
	assert.NoError(t, err, "error returned while getting a flexgroup volume")

	// case 2: Flexgroup get by name returned volume info nil
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(nil, nil)
	_, err = oapi.FlexgroupInfo(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup volume")

	// case 3: Flexgroup get by name, returned error.
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(nil, errors.New("failed to get flexgroup volume"))
	_, err = oapi.FlexgroupInfo(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup volume")

	// case 4: Flexgroup get by name. Response contain volume name is nil
	volume.Name = nil
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	_, err = oapi.FlexgroupInfo(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup volume")
}

func TestFlexgroupSetComment(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup set comment.
	rsi.EXPECT().FlexGroupSetComment(ctx, "trident-pvc-1234", "flexvol comment").Return(nil)
	err := oapi.FlexgroupSetComment(ctx, "trident-pvc-1234", "vol1", "flexvol comment")
	assert.NoError(t, err, "error returned while modifying a flexgroup comment")

	// case 2: Flexgroup set comment returned error.
	rsi.EXPECT().FlexGroupSetComment(ctx, "trident-pvc-1234", "flexvol comment").Return(
		errors.New("failed to update comment on flexgroup volume"))
	err = oapi.FlexgroupSetComment(ctx, "trident-pvc-1234", "vol1", "flexvol comment")
	assert.Error(t, err, "no error returned while modifying a flexgroup comment")
}

func TestFlexgroupSetQosPolicyGroupName(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup set QoS policy..
	qosPolicy := api.QosPolicyGroup{Name: "fake-qosPolicy"}
	rsi.EXPECT().FlexgroupSetQosPolicyGroupName(ctx, "trident-pvc-1234", qosPolicy).Return(nil)
	err := oapi.FlexgroupSetQosPolicyGroupName(ctx, "trident-pvc-1234", qosPolicy)
	assert.NoError(t, err, "error returned while modifying a QoS policy")

	// case 2: Flexgroup set QoS policy returned error.
	rsi.EXPECT().FlexgroupSetQosPolicyGroupName(ctx, "trident-pvc-1234", qosPolicy).Return(
		errors.New("failed to update comment on flexgroup volume"))
	err = oapi.FlexgroupSetQosPolicyGroupName(ctx, "trident-pvc-1234", qosPolicy)
	assert.Error(t, err, "no error returned while modifying a QoS policy")
}

func getVolumeInfo() *models.Volume {
	volumeName := "fakeVolume"
	volumeUUID := "fakeUUID"
	volumeType := "rw"
	aggregates := "fakeAggr"
	comment := ""
	path := "fakePath"
	unixPermission := int64(777)
	exportPolicy := "fakeExportPolicy"
	size := int64(1073741824)
	guarantee := "none"
	snapshotPolicy := "fakeSnapshotPolicy"
	snapshotReservePercent := int64(20)
	snapshotUsed := int64(1073741810)
	snapshotDir := false
	quotaState := "on"

	volumeGuarantee := models.VolumeInlineGuarantee{Type: &guarantee}

	volumeInlineAggr := models.VolumeInlineAggregatesInlineArrayItem{Name: &aggregates}
	volumeInlineAggrList := []*models.VolumeInlineAggregatesInlineArrayItem{&volumeInlineAggr}

	volumeInlineExportPolicy := models.VolumeInlineNasInlineExportPolicy{Name: &exportPolicy}
	VolumeInlineNas := models.VolumeInlineNas{
		Path: &path, UnixPermissions: &unixPermission,
		ExportPolicy: &volumeInlineExportPolicy,
	}

	volumeSnapshotPolicy := models.VolumeInlineSnapshotPolicy{Name: &snapshotPolicy}
	volumeSpaceShanpshot := models.VolumeInlineSpaceInlineSnapshot{
		ReservePercent: &snapshotReservePercent,
		Used:           &snapshotUsed,
	}
	volumeSpace := models.VolumeInlineSpace{Snapshot: &volumeSpaceShanpshot}
	volumeQuota := models.VolumeInlineQuota{State: &quotaState}

	volume := models.Volume{
		Name:                           &volumeName,
		UUID:                           &volumeUUID,
		Type:                           &volumeType,
		VolumeInlineAggregates:         volumeInlineAggrList,
		Comment:                        &comment,
		Nas:                            &VolumeInlineNas,
		Size:                           &size,
		Guarantee:                      &volumeGuarantee,
		SnapshotPolicy:                 &volumeSnapshotPolicy,
		Space:                          &volumeSpace,
		SnapshotDirectoryAccessEnabled: &snapshotDir,
		Quota:                          &volumeQuota,
	}
	return &volume
}

func TestFlexgroupSnapshotCreate(t *testing.T) {
	volume := getVolumeInfo()
	volumeUUID := *volume.UUID
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup snapshot create
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	rsi.EXPECT().SnapshotCreateAndWait(ctx, volumeUUID, "fake-snapshot").Return(nil)
	err := oapi.FlexgroupSnapshotCreate(ctx, "fake-snapshot", "vol1")
	assert.NoError(t, err, "error returned while creating snapshot of a flexgroup")

	// case 2: Flexgroup snapshot create. Could not get the volume info
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(nil, errors.New("Failed to get flexgroup"))
	err = oapi.FlexgroupSnapshotCreate(ctx, "fake-snapshot", "vol1")
	assert.Error(t, err, "no error returned while creating snapshot of a flexgroup")

	// case 3: Flexgroup snapshot create returned response nil
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(nil, nil)
	err = oapi.FlexgroupSnapshotCreate(ctx, "fake-snapshot", "vol1")
	assert.Error(t, err, "no error returned while creating snapshot of a flexgroup")

	// case 4: Flexgroup snapshot create. Could not create snapshot.
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	rsi.EXPECT().SnapshotCreateAndWait(ctx, volumeUUID, "fake-snapshot").Return(errors.New("snapshot creation failed"))
	err = oapi.FlexgroupSnapshotCreate(ctx, "fake-snapshot", "vol1")
	assert.Error(t, err, "no error returned while creating snapshot of a flexgroup")
}

func TestFlexgroupSnapshotList(t *testing.T) {
	snapshotName1 := "snapvol1"
	snapshotName2 := "snapvol2"
	createTime1 := strfmt.NewDateTime()
	createTime2 := strfmt.NewDateTime()
	volume := getVolumeInfo()
	volumeUUID := *volume.UUID
	oapi, rsi := newMockOntapAPIREST(t)

	snap1 := models.Snapshot{Name: &snapshotName1, CreateTime: &createTime1}
	snap2 := models.Snapshot{Name: &snapshotName2, CreateTime: &createTime2}
	snapshotResponse := models.SnapshotResponse{SnapshotResponseInlineRecords: []*models.Snapshot{&snap1, &snap2}}
	snapshotResponseOK := storage.SnapshotCollectionGetOK{Payload: &snapshotResponse}

	// case 1: Flexgroup get info.
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	rsi.EXPECT().SnapshotList(ctx, volumeUUID).Return(&snapshotResponseOK, nil)
	snapshots, err := oapi.FlexgroupSnapshotList(ctx, "vol1")
	assert.NoError(t, err, "error returned while getting a flexgroup")
	assert.NotNil(t, snapshots)
	assert.Equal(t, "snapvol1", snapshots[0].Name)
	assert.Equal(t, createTime1.String(), snapshots[0].CreateTime)

	// case 2: Flexgroup get by name returned error
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(nil, errors.New("failed to get flexgroup"))
	_, err = oapi.FlexgroupSnapshotList(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup")

	// case 3: Flexgroup get by name returned nil
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(nil, nil)
	_, err = oapi.FlexgroupSnapshotList(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup")

	// case 4: Flexgroup snapshot list returned nil
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	rsi.EXPECT().SnapshotList(ctx, volumeUUID).Return(nil, nil)
	snapshots, err = oapi.FlexgroupSnapshotList(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup")

	// case 5: Flexgroup snapshot list returned error
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	rsi.EXPECT().SnapshotList(ctx, volumeUUID).Return(nil, errors.New("failed to get snapshot info"))
	snapshots, err = oapi.FlexgroupSnapshotList(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup")

	// case 6: Flexgroup snapshot list returned payload nil
	snapshotResponseOK = storage.SnapshotCollectionGetOK{Payload: nil}
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	rsi.EXPECT().SnapshotList(ctx, volumeUUID).Return(&snapshotResponseOK, nil)
	_, err = oapi.FlexgroupSnapshotList(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup")
}

func TestFlexgroupModifyUnixPermissions(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup modify unix permission
	rsi.EXPECT().FlexGroupModifyUnixPermissions(ctx, "trident-pvc-1234", "--rwxrwxrwx").Return(nil)
	err := oapi.FlexgroupModifyUnixPermissions(ctx, "trident-pvc-1234", "vol1", "--rwxrwxrwx")
	assert.NoError(t, err, "error returned while modifying a unix permission")

	// case 2: Flexgroup modify unix permission returned error
	rsi.EXPECT().FlexGroupModifyUnixPermissions(ctx, "trident-pvc-1234", "--rwxrwxrwx").Return(
		errors.New("failed to modify unix permission on flexgroup volume"))
	err = oapi.FlexgroupModifyUnixPermissions(ctx, "trident-pvc-1234", "vol1", "--rwxrwxrwx")
	assert.Error(t, err, "no error returned while modifying a unix permission")
}

func TestFlexgroupMount(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup mount
	rsi.EXPECT().FlexGroupMount(ctx, "trident-pvc-1234", "/trident-pvc-1234").Return(nil)
	err := oapi.FlexgroupMount(ctx, "trident-pvc-1234", "/trident-pvc-1234")
	assert.NoError(t, err, "error returned while mounting a volume")

	// case 2: Flexgroup mount returned error
	rsi.EXPECT().FlexGroupMount(ctx, "trident-pvc-1234", "/trident-pvc-1234").Return(
		errors.New("failed to mount flexgroup volume"))
	err = oapi.FlexgroupMount(ctx, "trident-pvc-1234", "/trident-pvc-1234")
	assert.Error(t, err, "no error returned while mounting a volume")
}

func TestFlexgroupDestroy(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup destroy
	rsi.EXPECT().FlexgroupUnmount(ctx, "trident-pvc-1234").Return(nil)
	rsi.EXPECT().FlexGroupDestroy(ctx, "trident-pvc-1234", false).Return(nil)
	err := oapi.FlexgroupDestroy(ctx, "trident-pvc-1234", true, false)
	assert.NoError(t, err, "error returned while deleting a volume")

	// case 2: Flexgroup destroyed returned error
	rsi.EXPECT().FlexgroupUnmount(ctx, "trident-pvc-1234").Return(nil)
	rsi.EXPECT().FlexGroupDestroy(ctx, "trident-pvc-1234", false).Return(
		errors.New("failed to delete flexgroup volume"))
	err = oapi.FlexgroupDestroy(ctx, "trident-pvc-1234", true, false)
	assert.Error(t, err, "no error returned while deleting a volume")

	// case 3: Flexgroup unmount returned error
	rsi.EXPECT().FlexgroupUnmount(ctx, "trident-pvc-1234").Return(
		errors.New("failed to unmount flexgroup volume"))
	err = oapi.FlexgroupDestroy(ctx, "trident-pvc-1234", true, false)
	assert.Error(t, err, "no error returned while deleting a volume")
}

func TestFlexgroupListByPrefix(t *testing.T) {
	volume := getVolumeInfo()

	volumeResponse := storage.VolumeCollectionGetOK{
		Payload: &models.VolumeResponse{
			VolumeResponseInlineRecords: []*models.Volume{volume},
		},
	}

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup get list by prefix
	rsi.EXPECT().FlexGroupGetAll(ctx, *volume.Name+"*", gomock.Any()).Return(&volumeResponse, nil)
	volumeInfo, err := oapi.FlexgroupListByPrefix(ctx, *volume.Name)
	assert.NoError(t, err, "error returned while getting a volume")
	assert.Equal(t, *volume.Name, volumeInfo[0].Name)

	// case 2: Flexgroup get returned empty list
	rsi.EXPECT().FlexGroupGetAll(ctx, *volume.Name+"*", gomock.Any()).Return(nil, nil)
	volumeInfo, err = oapi.FlexgroupListByPrefix(ctx, *volume.Name)
	assert.Error(t, err, "no error returned while getting a volume")

	// case 3: Flexgroup get returned error
	rsi.EXPECT().FlexGroupGetAll(ctx, *volume.Name+"*", gomock.Any()).Return(nil, errors.New("failed to get flexgroup"))
	volumeInfo, err = oapi.FlexgroupListByPrefix(ctx, *volume.Name)
	assert.Error(t, err, "no error returned while getting a volume")

	// case 4: Flexgroup get list, response contain volume name nil
	volume.Name = nil
	rsi.EXPECT().FlexGroupGetAll(ctx, "*", gomock.Any()).Return(&volumeResponse, nil)
	volumeInfo, err = oapi.FlexgroupListByPrefix(ctx, "*")
	assert.Error(t, err, "no error returned while getting a volume")
}

func TestFlexgroupSetSize(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup modify size
	rsi.EXPECT().FlexGroupSetSize(ctx, "trident-pvc-1234", "107374182400").Return(nil)
	err := oapi.FlexgroupSetSize(ctx, "trident-pvc-1234", "107374182400")
	assert.NoError(t, err, "error returned while modifying a size of a volume")

	// case 2: Flexgroup modify size returned error.
	rsi.EXPECT().FlexGroupSetSize(ctx, "trident-pvc-1234", "107374182400").Return(
		errors.New("failed to update flexgroup volume size"))
	err = oapi.FlexgroupSetSize(ctx, "trident-pvc-1234", "107374182400")
	assert.Error(t, err, "no error returned while modifying a size of a volume")
}

func TestFlexgroupSize(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup get size
	rsi.EXPECT().FlexGroupSize(ctx, "trident-pvc-1234").Return(uint64(107374182400), nil)
	volumeSize, err := oapi.FlexgroupSize(ctx, "trident-pvc-1234")
	assert.NoError(t, err, "error returned while getting a size of a volume")
	assert.Equal(t, uint64(107374182400), volumeSize)

	// case 2: Flexgroup get size returned error
	rsi.EXPECT().FlexGroupSize(ctx, "trident-pvc-1234").Return(uint64(107374182400),
		errors.New("failed to get flexgroup volume size"))
	volumeSize, err = oapi.FlexgroupSize(ctx, "trident-pvc-1234")
	assert.Error(t, err, "no error returned while getting a size of a volume")
}

func TestFlexgroupUsedSize(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup get used size
	rsi.EXPECT().FlexGroupUsedSize(ctx, "trident-pvc-1234").Return(107374182400, nil)
	volumeSize, err := oapi.FlexgroupUsedSize(ctx, "trident-pvc-1234")
	assert.NoError(t, err, "error returned while getting a used size of a volume")
	assert.Equal(t, 107374182400, volumeSize)

	// case 2: Flexgroup get used size returned error
	rsi.EXPECT().FlexGroupUsedSize(ctx, "trident-pvc-1234").Return(107374182400,
		errors.New("failed to get flexgroup volume used size"))
	volumeSize, err = oapi.FlexgroupUsedSize(ctx, "trident-pvc-1234")
	assert.Error(t, err, "no error returned while getting a used size of a volume")
}

func TestFlexgroupModifyExportPolicy(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup modify export policy
	rsi.EXPECT().FlexgroupModifyExportPolicy(ctx, "trident-pvc-1234", "fake-policy").Return(nil)
	err := oapi.FlexgroupModifyExportPolicy(ctx, "trident-pvc-1234", "fake-policy")
	assert.NoError(t, err, "error returned while modifying a export policy of a volume")

	// case 1: Flexgroup modify export policy returned error
	rsi.EXPECT().FlexgroupModifyExportPolicy(ctx, "trident-pvc-1234", "fake-policy").Return(
		errors.New("failed to modify export policy"))
	err = oapi.FlexgroupModifyExportPolicy(ctx, "trident-pvc-1234", "fake-policy")
	assert.Error(t, err, "no error returned while modifying a export policy of a volume")
}

func TestFlexgroupUnmount(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup umount
	rsi.EXPECT().FlexgroupUnmount(ctx, "trident-pvc-1234").Return(nil)
	err := oapi.FlexgroupUnmount(ctx, "trident-pvc-1234", true)
	assert.NoError(t, err, "error returned while unmounting a flexgroup")

	// case 2: Flexgroup umount returned error
	rsi.EXPECT().FlexgroupUnmount(ctx, "trident-pvc-1234").Return(
		errors.New("failed to unmount flexgroup volume"))
	err = oapi.FlexgroupUnmount(ctx, "trident-pvc-1234", false)
	assert.Error(t, err, "no error returned while unmounting a flexgroup")
}

func getAggregateInfo(aggrName, diskType string, numOfRecords int64,
	aggrSpace *models.AggregateInlineSpace,
) *storage.AggregateCollectionGetOK {
	aggrNameAddress := &aggrName
	if aggrName == "" {
		aggrNameAddress = nil
	}

	aggregate := models.Aggregate{
		Name: aggrNameAddress,
		BlockStorage: &models.AggregateInlineBlockStorage{
			Primary: &models.AggregateInlineBlockStorageInlinePrimary{DiskType: &diskType},
		},
		Space: aggrSpace,
	}
	aggregateResponse := storage.AggregateCollectionGetOK{
		Payload: &models.AggregateResponse{
			AggregateResponseInlineRecords: []*models.Aggregate{&aggregate},
			NumRecords:                     &numOfRecords,
		},
	}

	return &aggregateResponse
}

func TestGetSVMAggregateAttributes(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	aggregateResponse := getAggregateInfo("aggr1", "fc", int64(1), nil)

	// case 1: Get SVM aggregate
	rsi.EXPECT().AggregateList(ctx, gomock.Any(), gomock.Any()).Return(aggregateResponse, nil)
	aggrList, err := oapi.GetSVMAggregateAttributes(ctx)
	assert.NoError(t, err, "error returned while getting a svm aggregate")
	assert.Equal(t, "fc", aggrList["aggr1"])

	// case 2: Get SVM aggregate returned nil list
	rsi.EXPECT().AggregateList(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
	aggrList, err = oapi.GetSVMAggregateAttributes(ctx)
	assert.Error(t, err, "no error returned while getting a svm aggregate")
	assert.Nil(t, aggrList)

	aggregateResponse = getAggregateInfo("", "fc", int64(1), nil)

	// case 3: Get SVM aggregate returned empty list
	rsi.EXPECT().AggregateList(ctx, gomock.Any(), gomock.Any()).Return(aggregateResponse, nil)
	aggrList, err = oapi.GetSVMAggregateAttributes(ctx)
	assert.NoError(t, err, "error returned while getting a svm aggregate")
	assert.Empty(t, aggrList)

	// case 4: Get SVM aggregate returned error
	aggregateResponse = getAggregateInfo("", "fc", int64(0), nil)
	rsi.EXPECT().AggregateList(ctx, gomock.Any(), gomock.Any()).Return(aggregateResponse, nil)
	aggrList, err = oapi.GetSVMAggregateAttributes(ctx)
	assert.Error(t, err, "no error returned while getting a svm aggregate")
}

func TestExportPolicyDestroy(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Export policy delete
	exportPolicyDelete := nas.ExportPolicyDeleteOK{}
	rsi.EXPECT().ExportPolicyDestroy(ctx, "trident-pvc-1234").Return(&exportPolicyDelete, nil)
	err := oapi.ExportPolicyDestroy(ctx, "trident-pvc-1234")
	assert.NoError(t, err, "error returned while deleting a export policy")

	// case 2: Export policy delete returned error
	rsi.EXPECT().ExportPolicyDestroy(ctx, "trident-pvc-1234").Return(nil,
		errors.New("failed to delete export policy"))
	err = oapi.ExportPolicyDestroy(ctx, "trident-pvc-1234")
	assert.Error(t, err, "no error returned while deleting a export policy")

	// case 3: Export policy delete returned nil response
	rsi.EXPECT().ExportPolicyDestroy(ctx, "trident-pvc-1234").Return(nil, nil)
	err = oapi.ExportPolicyDestroy(ctx, "trident-pvc-1234")
	assert.Error(t, err, "no error returned while deleting a export policy")
}

func TestVolumeExists(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: volume exists
	rsi.EXPECT().VolumeExists(ctx, "trident-pvc-1234").Return(true, nil)
	isExists, err := oapi.VolumeExists(ctx, "trident-pvc-1234")
	assert.NoError(t, err, "error returned while verifying a volume")
	assert.Equal(t, true, isExists)

	// case 2: volume exists returned error
	rsi.EXPECT().VolumeExists(ctx, "trident-pvc-1234").Return(true,
		errors.New("flexgroup volume does not exists"))
	isExists, err = oapi.VolumeExists(ctx, "trident-pvc-1234")
	assert.Error(t, err, "no error returned while verifying a volume")
}

func TestTieringPolicyValue(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().TieringPolicyValue(ctx).Return("fake-TieringPolicy")
	tieringPolicy := oapi.TieringPolicyValue(ctx)
	assert.Equal(t, "fake-TieringPolicy", tieringPolicy)
}

func TestGetSVMAggregateSpace(t *testing.T) {
	size := int64(3221225472)
	usedSize := int64(2147483648)
	footprint := int64(608896)

	oapi, rsi := newMockOntapAPIREST(t)

	aggrSpace1 := models.AggregateInlineSpace{
		BlockStorage: &models.AggregateInlineSpaceInlineBlockStorage{
			Size: &size,
			Used: &usedSize,
		},
		Footprint: &footprint,
	}

	aggrResponse := getAggregateInfo("aggr1", "fc", int64(0), &aggrSpace1)

	// case 1: Get SVM aggregate space
	rsi.EXPECT().AggregateList(ctx, "aggr1", gomock.Any()).Return(aggrResponse, nil)
	aggrSpaceList, err := oapi.GetSVMAggregateSpace(ctx, "aggr1")
	assert.NoError(t, err, "error returned while getting a aggregate space")
	assert.Equal(t, size, aggrSpaceList[0].Size())
	assert.Equal(t, usedSize, aggrSpaceList[0].Used())
	assert.Equal(t, footprint, aggrSpaceList[0].Footprint())

	aggrSpace2 := models.AggregateInlineSpace{
		BlockStorage: &models.AggregateInlineSpaceInlineBlockStorage{
			Size: &size,
			Used: &usedSize,
		},
	}

	// case 2: Get SVM aggregate space, Footprint nil
	aggrResponse2 := getAggregateInfo("aggr1", "fc", int64(1), &aggrSpace2)
	rsi.EXPECT().AggregateList(ctx, "aggr1", gomock.Any()).Return(aggrResponse2, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr1")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	aggrSpace3 := models.AggregateInlineSpace{
		BlockStorage: &models.AggregateInlineSpaceInlineBlockStorage{
			Size: &size,
		},
	}

	// case 3: Get SVM aggregate space, Footprint and used size nil
	aggrResponse3 := getAggregateInfo("aggr1", "fc", int64(1), &aggrSpace3)
	rsi.EXPECT().AggregateList(ctx, "aggr1", gomock.Any()).Return(aggrResponse3, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr1")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	aggrSpace4 := models.AggregateInlineSpace{BlockStorage: &models.AggregateInlineSpaceInlineBlockStorage{}}

	// case 4: Get SVM aggregate space, Size nil
	aggrResponse4 := getAggregateInfo("aggr1", "fc", int64(1), &aggrSpace4)
	rsi.EXPECT().AggregateList(ctx, "aggr1", gomock.Any()).Return(aggrResponse4, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr1")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	// case 5: Get SVM aggregate space, aggregate space nil
	aggrResponse5 := getAggregateInfo("aggr1", "fc", int64(1), nil)
	rsi.EXPECT().AggregateList(ctx, "aggr1", gomock.Any()).Return(aggrResponse5, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr1")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	// case 6: Get SVM aggregate space, aggregate name not present
	aggrResponse6 := getAggregateInfo("aggr1", "fc", int64(1), nil)
	rsi.EXPECT().AggregateList(ctx, "aggr2", gomock.Any()).Return(aggrResponse6, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr2")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	// case 7: Get SVM aggregate space, aggregate name is empty
	aggrResponse7 := getAggregateInfo("", "fc", int64(1), nil)
	rsi.EXPECT().AggregateList(ctx, "aggr2", gomock.Any()).Return(aggrResponse7, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr2")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	// case 8: Get SVM aggregate space, payload is empty object
	aggrResponse8 := storage.AggregateCollectionGetOK{
		Payload: &models.AggregateResponse{},
	}
	rsi.EXPECT().AggregateList(ctx, "aggr2", gomock.Any()).Return(&aggrResponse8, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr2")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	// case 9: Get SVM aggregate space, payload is nil
	aggrResponse9 := storage.AggregateCollectionGetOK{}
	rsi.EXPECT().AggregateList(ctx, "aggr2", gomock.Any()).Return(&aggrResponse9, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr2")
	assert.Error(t, err, "no error returned while getting a aggregate space")

	// case 10: Get SVM aggregate space, aggregate list is nil
	rsi.EXPECT().AggregateList(ctx, "aggr2", gomock.Any()).Return(nil, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr2")
	assert.Error(t, err, "no error returned while getting a aggregate space")
}

func TestVolumeDisableSnapshotDirectoryAccess(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Volume disable snapshot directory access positive test case
	rsi.EXPECT().VolumeModifySnapshotDirectoryAccess(ctx, "vol1", false).Return(nil)
	err := oapi.VolumeModifySnapshotDirectoryAccess(ctx, "vol1", false)
	assert.NoError(t, err, "error returned disabling a snapshot directory access")

	// case 2: Volume disable snapshot directory access negative test case
	rsi.EXPECT().VolumeModifySnapshotDirectoryAccess(ctx, "vol1", false).
		Return(errors.New("error disabling snapshot directory access"))
	err = oapi.VolumeModifySnapshotDirectoryAccess(ctx, "vol1", false)
	assert.Error(t, err, " no error returned disabling a snapshot directory access")
}

func TestVolumeMount(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Volume mount positive test case
	rsi.EXPECT().VolumeMount(ctx, "vol1", "/vol1").Return(nil)
	err := oapi.VolumeMount(ctx, "vol1", "/vol1")
	assert.NoError(t, err, "error returned mounting a volume")

	// case 2: Volume mount returned nil
	rsi.EXPECT().VolumeMount(ctx, "vol1", "/vol1").Return(nil).
		Return(errors.New("failed to mount volume"))
	err = oapi.VolumeMount(ctx, "vol1", "/vol1")
	assert.Error(t, err, "no error returned mounting a volume")

	code := int64(917536)
	job := models.Job{Code: &code}
	restErr := api.NewRestErrorFromPayload(&job)

	// case 3: Volume mount returned error
	rsi.EXPECT().VolumeMount(ctx, "vol1", "/vol1").Return(nil).Return(restErr)
	err = oapi.VolumeMount(ctx, "vol1", "/vol1")
	assert.Error(t, err, "no error returned while mounting a volume")
}

func TestVolumeRename(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().VolumeRename(ctx, "oldVolume", "newVolume").Return(nil)
	err := oapi.VolumeRename(ctx, "oldVolume", "newVolume")
	assert.NoError(t, err, "error returned while renaming a volume")
}

func TestVolumeSetComment(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Volume set comment positive test
	rsi.EXPECT().VolumeSetComment(ctx, "trident-pvc-1234", "flexvol comment").Return(nil)
	err := oapi.VolumeSetComment(ctx, "trident-pvc-1234", "vol1", "flexvol comment")
	assert.NoError(t, err, "error returned while modifying a comment on volume")

	// case 2: Volume set comment negative test
	rsi.EXPECT().VolumeSetComment(ctx, "trident-pvc-1234", "flexvol comment").Return(
		errors.New("failed to update comment on volume"))
	err = oapi.VolumeSetComment(ctx, "trident-pvc-1234", "vol1", "flexvol comment")
	assert.Error(t, err, "no error returned while modifying a comment on volume")
}

func TestExportPolicyCreate(t *testing.T) {
	exportPolicyName := "fake-exportPolicy"
	exportPolicy := models.ExportPolicy{Name: &exportPolicyName}
	exportPolicyList := []*models.ExportPolicy{&exportPolicy}
	exportPolicyResponse := models.ExportPolicyResponse{ExportPolicyResponseInlineRecords: exportPolicyList}
	exportPolicyCreated := nas.ExportPolicyCreateCreated{Payload: &exportPolicyResponse}

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Volume create export policy positive test
	rsi.EXPECT().ExportPolicyGetByName(ctx, "fake-exportPolicy").Return(nil, nil)
	rsi.EXPECT().ExportPolicyCreate(ctx, "fake-exportPolicy").Return(&exportPolicyCreated, nil)
	err := oapi.ExportPolicyCreate(ctx, "fake-exportPolicy")
	assert.NoError(t, err, "error returned while creating a export policy")

	// case 2: Volume create export policy, policy already present.
	rsi.EXPECT().ExportPolicyGetByName(ctx, "fake-exportPolicy").Return(&exportPolicy, nil)
	err = oapi.ExportPolicyCreate(ctx, "fake-exportPolicy")
	assert.NoError(t, err, "error returned while creating a export policy")

	// case 3: Volume create export policy negative test
	rsi.EXPECT().ExportPolicyGetByName(ctx, "fake-exportPolicy").Return(nil,
		errors.New("failed to verify export policy"))
	err = oapi.ExportPolicyCreate(ctx, "fake-exportPolicy")
	assert.Error(t, err, "no error returned while creating a export policy")

	// case 4: Volume create export policy returned nil
	rsi.EXPECT().ExportPolicyGetByName(ctx, "fake-exportPolicy").Return(nil, nil)
	rsi.EXPECT().ExportPolicyCreate(ctx, "fake-exportPolicy").Return(nil, nil)
	err = oapi.ExportPolicyCreate(ctx, "fake-exportPolicy")
	assert.Error(t, err, "no error returned while creating a export policy")

	// case 5: Volume create export policy returned error
	rsi.EXPECT().ExportPolicyGetByName(ctx, "fake-exportPolicy").Return(nil, nil)
	rsi.EXPECT().ExportPolicyCreate(ctx, "fake-exportPolicy").Return(nil,
		errors.New("export policy creation failed"))
	err = oapi.ExportPolicyCreate(ctx, "fake-exportPolicy")
	assert.Error(t, err, "no error returned while creating a export policy")
}

func TestVolumeSize(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().VolumeSize(ctx, "vol1").Return(uint64(1073741824000), nil)
	size, err := oapi.VolumeSize(ctx, "vol1")
	assert.NoError(t, err, "error returned while getting a volume size")
	assert.Equal(t, uint64(1073741824000), size)
}

func TestVolumeUsedSize(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().VolumeUsedSize(ctx, "vol1").Return(1073741824000, nil)
	size, err := oapi.VolumeUsedSize(ctx, "vol1")
	assert.NoError(t, err, "error returned while getting a volume used size")
	assert.Equal(t, 1073741824000, size)
}

func TestVolumeSetSize(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Volume modify size positive test
	rsi.EXPECT().VolumeSetSize(ctx, "vol1", "1073741824000").Return(nil)
	err := oapi.VolumeSetSize(ctx, "vol1", "1073741824000")
	assert.NoError(t, err, "error returned while modifying a volume size")

	// case 2: Volume modify size negative test
	rsi.EXPECT().VolumeSetSize(ctx, "vol1", "1073741824000").Return(
		errors.New("failed to update volume size"))
	err = oapi.VolumeSetSize(ctx, "vol1", "1073741824000")
	assert.Error(t, err, "no error returned while modifying a volume size")
}

func TestVolumeModifyUnixPermissions(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Volume modify unix permission positive test
	rsi.EXPECT().VolumeModifyUnixPermissions(ctx, "trident-pvc-1234", "--rwxrwxrwx").Return(nil)
	err := oapi.VolumeModifyUnixPermissions(ctx, "trident-pvc-1234", "vol1", "--rwxrwxrwx")
	assert.NoError(t, err, "error returned while modifying a unix permission")

	// case 2: Volume modify unix permission negative test
	rsi.EXPECT().VolumeModifyUnixPermissions(ctx, "trident-pvc-1234", "--rwxrwxrwx").Return(
		errors.New("failed to modify unix permission on flexgroup volume"))
	err = oapi.VolumeModifyUnixPermissions(ctx, "trident-pvc-1234", "vol1", "--rwxrwxrwx")
	assert.Error(t, err, "no error returned while modifying a unix permission")
}

func TestVolumeListByPrefix(t *testing.T) {
	volume := getVolumeInfo()

	volumeResponse1 := storage.VolumeCollectionGetOK{
		Payload: &models.VolumeResponse{
			VolumeResponseInlineRecords: []*models.Volume{volume},
		},
	}

	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().VolumeList(ctx, *volume.Name+"*", gomock.Any()).Return(&volumeResponse1, nil)
	volumeInfo, err := oapi.VolumeListByPrefix(ctx, *volume.Name)
	assert.NoError(t, err, "error returned while getting list of volumes")
	assert.Equal(t, *volume.Name, volumeInfo[0].Name)

	volumeResponse2 := storage.VolumeCollectionGetOK{
		Payload: &models.VolumeResponse{
			VolumeResponseInlineRecords: []*models.Volume{nil},
		},
	}

	// case 1: Get volume positive test
	rsi.EXPECT().VolumeList(ctx, *volume.Name+"*", gomock.Any()).Return(&volumeResponse2, nil)
	_, err = oapi.VolumeListByPrefix(ctx, *volume.Name)
	assert.Error(t, err, "no error returned while getting a volume info")

	// case 2: Get volume negative test
	rsi.EXPECT().VolumeList(ctx, *volume.Name+"*", gomock.Any()).Return(nil, errors.New("failed to get volume"))
	_, err = oapi.VolumeListByPrefix(ctx, *volume.Name)
	assert.Error(t, err, "no error returned while getting a volume info")
}

func TestVolumeListByAttrs(t *testing.T) {
	volName := "vol1"
	volumeResponse := storage.VolumeCollectionGetOK{
		Payload: &models.VolumeResponse{
			VolumeResponseInlineRecords: []*models.Volume{
				{
					Name: &volName,
				},
			},
		},
	}
	volume1 := api.Volume{
		Name: volName,
	}
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().VolumeListByAttrs(ctx, gomock.Any(), gomock.Any()).Return(&volumeResponse, nil)
	volumeList, err := oapi.VolumeListByAttrs(ctx, &volume1)
	assert.NoError(t, err, "error returned while getting a volume list by attribute")
	assert.Equal(t, "vol1", volumeList[0].Name)
}

func TestExportRuleCreate(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// case 1: Export rule create positive test
	rsi.EXPECT().ExportRuleCreate(ctx, "fake-policy", "fake-rule", []string{"cifs"}, []string{"any"},
		[]string{"any"}, []string{"any"}).Return(&nas.ExportRuleCreateCreated{}, nil)
	err := oapi.ExportRuleCreate(ctx, "fake-policy", "fake-rule", sa.SMB)
	assert.NoError(t, err, "error returned while creating a export rule")

	// case 2: Export rule create returned nil
	rsi.EXPECT().ExportRuleCreate(ctx, "fake-policy", "fake-rule", []string{"nfs"}, []string{"any"},
		[]string{"any"}, []string{"any"}).Return(nil, nil)
	err = oapi.ExportRuleCreate(ctx, "fake-policy", "fake-rule", sa.NFS)
	assert.Error(t, err, "no error returned while creating a export rule")

	// case 3: Export rule create returned error
	rsi.EXPECT().ExportRuleCreate(ctx, "fake-policy", "fake-rule", []string{"nfs"}, []string{"any"},
		[]string{"any"}, []string{"any"}).Return(nil, errors.New("failed to get export rule"))
	err = oapi.ExportRuleCreate(ctx, "fake-policy", "fake-rule", sa.NFS)
	assert.Error(t, err, "no error returned while creating a export rule")
}

func TestExportRuleDestroy(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// case 1: Export rule delete positive test
	rsi.EXPECT().ExportRuleDestroy(ctx, "fake-policy", 1).Return(&nas.ExportRuleDeleteOK{}, nil)
	err := oapi.ExportRuleDestroy(ctx, "fake-policy", 1)
	assert.NoError(t, err, "error returned while deleting a export rule")

	// case 2: Export rule delete returned response nil
	rsi.EXPECT().ExportRuleDestroy(ctx, "fake-policy", 1).Return(nil, nil)
	err = oapi.ExportRuleDestroy(ctx, "fake-policy", 1)
	assert.Error(t, err, "no error returned while deleting a export rule")

	// case 3: Export rule delete negative test
	rsi.EXPECT().ExportRuleDestroy(ctx, "fake-policy", 1).Return(nil, errors.New("failed to get export rule"))
	err = oapi.ExportRuleDestroy(ctx, "fake-policy", 1)
	assert.Error(t, err, "no error returned while deleting a export rule")
}

func TestVolumeModifyExportPolicy(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Export rule modify positive test
	rsi.EXPECT().VolumeModifyExportPolicy(ctx, "trident-pvc-1234", "fake-policy").Return(nil)
	err := oapi.VolumeModifyExportPolicy(ctx, "trident-pvc-1234", "fake-policy")
	assert.NoError(t, err, "error returned while modifying a export policy")

	// case 2: Export rule modify negative test
	rsi.EXPECT().VolumeModifyExportPolicy(ctx, "trident-pvc-1234", "fake-policy").Return(
		errors.New("failed to modify export policy"))
	err = oapi.VolumeModifyExportPolicy(ctx, "trident-pvc-1234", "fake-policy")
	assert.Error(t, err, "no error returned while modifying a export policy")
}

func TestExportPolicyExists(t *testing.T) {
	exportPolicyName := "fake-exportPolicy"
	exportPolicy := models.ExportPolicy{Name: &exportPolicyName}

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Export rule get returned nil
	rsi.EXPECT().ExportPolicyGetByName(ctx, "fake-exportPolicy").Return(nil, nil)
	isExists, err := oapi.ExportPolicyExists(ctx, "fake-exportPolicy")
	assert.NoError(t, err, "error returned while getting a export policy")
	assert.Equal(t, false, isExists)

	// case 2: Export rule get positive test
	rsi.EXPECT().ExportPolicyGetByName(ctx, "fake-exportPolicy").Return(&exportPolicy, nil)
	isExists, err = oapi.ExportPolicyExists(ctx, "fake-exportPolicy")
	assert.NoError(t, err, "no error returned while getting a export policy")
	assert.Equal(t, true, isExists)

	// case 3: Export rule get returned error
	rsi.EXPECT().ExportPolicyGetByName(ctx, "fake-exportPolicy").Return(nil,
		errors.New("failed to verify export policy"))
	isExists, err = oapi.ExportPolicyExists(ctx, "fake-exportPolicy")
	assert.Error(t, err, "no error returned while getting an export policy")
	assert.Equal(t, false, isExists)
}

func TestExportRuleList_NoDuplicateRules(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	policyName := "testPolicy"
	expectedRules := map[int]string{
		1: "192.168.1.1",
		2: "192.168.1.2",
	}

	exportRule := models.ExportRuleResponse{
		ExportRuleResponseInlineRecords: []*models.ExportRule{
			{
				ExportRuleInlineClients: []*models.ExportClients{{Match: convert.ToPtr("192.168.1.1")}},
				Index:                   convert.ToPtr(int64(1)),
			},
			{
				ExportRuleInlineClients: []*models.ExportClients{{Match: convert.ToPtr("192.168.1.2")}},
				Index:                   convert.ToPtr(int64(2)),
			},
		},
		NumRecords: convert.ToPtr(int64(2)),
	}

	exportRuleResponse := nas.ExportRuleCollectionGetOK{
		Payload: &exportRule,
	}

	rsi.EXPECT().ExportRuleList(ctx, policyName).Return(&exportRuleResponse, nil)

	rules, err := oapi.ExportRuleList(ctx, policyName)
	assert.NoError(t, err)
	assert.Equal(t, expectedRules, rules)
}

func TestExportRuleList_DuplicateRules(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	policyName := "testPolicy"
	expectedRules := map[int]string{
		1: "192.168.1.1",
		2: "192.168.1.2",
		3: "192.168.1.2",
		4: "192.168.1.2",
		5: "192.168.1.1",
	}

	exportRule := models.ExportRuleResponse{
		ExportRuleResponseInlineRecords: []*models.ExportRule{
			{
				ExportRuleInlineClients: []*models.ExportClients{{Match: convert.ToPtr("192.168.1.1")}},
				Index:                   convert.ToPtr(int64(1)),
			},
			{
				ExportRuleInlineClients: []*models.ExportClients{{Match: convert.ToPtr("192.168.1.2")}},
				Index:                   convert.ToPtr(int64(2)),
			},
			{
				ExportRuleInlineClients: []*models.ExportClients{{Match: convert.ToPtr("192.168.1.2")}},
				Index:                   convert.ToPtr(int64(3)),
			},
			{
				ExportRuleInlineClients: []*models.ExportClients{{Match: convert.ToPtr("192.168.1.2")}},
				Index:                   convert.ToPtr(int64(4)),
			},
			{
				ExportRuleInlineClients: []*models.ExportClients{{Match: convert.ToPtr("192.168.1.1")}},
				Index:                   convert.ToPtr(int64(5)),
			},
		},
		NumRecords: convert.ToPtr(int64(5)),
	}

	exportRuleResponse := nas.ExportRuleCollectionGetOK{
		Payload: &exportRule,
	}

	rsi.EXPECT().ExportRuleList(ctx, policyName).Return(&exportRuleResponse, nil)

	rules, err := oapi.ExportRuleList(ctx, policyName)
	assert.NoError(t, err)
	assert.Equal(t, expectedRules, rules)
}

func TestExportRuleList_MultiIPRule(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	policyName := "testPolicy"
	expectedRules := map[int]string{
		1: "192.168.1.1,10.2.2.1,10.0.0.1",
		2: "192.168.1.2",
		3: "192.168.1.2",
		4: "192.168.1.2",
		5: "192.168.1.1",
	}

	exportRule := models.ExportRuleResponse{
		ExportRuleResponseInlineRecords: []*models.ExportRule{
			{
				ExportRuleInlineClients: []*models.ExportClients{
					{Match: convert.ToPtr("192.168.1.1")},
					{Match: convert.ToPtr("10.2.2.1")},
					{Match: convert.ToPtr("10.0.0.1")},
				},
				Index: convert.ToPtr(int64(1)),
			},
			{
				ExportRuleInlineClients: []*models.ExportClients{{Match: convert.ToPtr("192.168.1.2")}},
				Index:                   convert.ToPtr(int64(2)),
			},
			{
				ExportRuleInlineClients: []*models.ExportClients{{Match: convert.ToPtr("192.168.1.2")}},
				Index:                   convert.ToPtr(int64(3)),
			},
			{
				ExportRuleInlineClients: []*models.ExportClients{{Match: convert.ToPtr("192.168.1.2")}},
				Index:                   convert.ToPtr(int64(4)),
			},
			{
				ExportRuleInlineClients: []*models.ExportClients{{Match: convert.ToPtr("192.168.1.1")}},
				Index:                   convert.ToPtr(int64(5)),
			},
		},
		NumRecords: convert.ToPtr(int64(5)),
	}

	exportRuleResponse := nas.ExportRuleCollectionGetOK{
		Payload: &exportRule,
	}

	rsi.EXPECT().ExportRuleList(ctx, policyName).Return(&exportRuleResponse, nil)

	rules, err := oapi.ExportRuleList(ctx, policyName)
	assert.NoError(t, err)
	assert.Equal(t, expectedRules, rules)
}

func TestExportRuleList_Error(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)
	policyName := "testPolicy"

	rsi.EXPECT().ExportRuleList(ctx, policyName).Return(nil,
		errors.New("error listing export policy rules"))

	rules, err := oapi.ExportRuleList(ctx, policyName)
	assert.Error(t, err)
	assert.Nil(t, rules)
}

func TestExportRuleList_NoRecords(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)
	policyName := "testPolicy"

	exportRule := models.ExportRuleResponse{
		ExportRuleResponseInlineRecords: []*models.ExportRule{},
		NumRecords:                      convert.ToPtr(int64(0)),
	}
	exportRuleResponse := nas.ExportRuleCollectionGetOK{
		Payload: &exportRule,
	}

	rsi.EXPECT().ExportRuleList(ctx, policyName).Return(&exportRuleResponse, nil)

	rules, err := oapi.ExportRuleList(ctx, policyName)
	assert.NoError(t, err)
	assert.Empty(t, rules)
}

func TestExportRuleList_NilPayload(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)
	policyName := "testPolicy"

	exportRuleResponse := nas.ExportRuleCollectionGetOK{
		Payload: nil,
	}

	rsi.EXPECT().ExportRuleList(ctx, policyName).Return(&exportRuleResponse, nil)

	rules, err := oapi.ExportRuleList(ctx, policyName)
	assert.NoError(t, err)
	assert.Empty(t, rules)
}

func TestQtreeExists(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().QtreeExists(ctx, "vol1", "trident_").Return(true, "trident_1234", nil)
	isExists, flexvol, err := oapi.QtreeExists(ctx, "vol1", "trident_")
	assert.NoError(t, err, "error returned while verifying a Qtree")
	assert.Equal(t, true, isExists)
	assert.Equal(t, "trident_1234", flexvol)
}

func TestQtreeCreate(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().QtreeCreate(ctx, "vol1", "trident_", "--rwxrwxrwx", "fake-exportPolicy",
		"unix", "fake-qosPolicy").Return(nil)
	err := oapi.QtreeCreate(ctx, "vol1", "trident_", "--rwxrwxrwx", "fake-exportPolicy",
		"unix", "fake-qosPolicy")
	assert.NoError(t, err, "error returned while creating a Qtree")
}

func TestQtreeDestroyAsync(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().QtreeDestroyAsync(ctx, "/vol1", false).Return(nil)
	err := oapi.QtreeDestroyAsync(ctx, "/vol/vol1", false)
	assert.NoError(t, err, "error returned while deleting a Qtree")
}

func TestQtreeRename(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().QtreeRename(ctx, "/oldName", "/newName").Return(nil)
	err := oapi.QtreeRename(ctx, "/vol/oldName", "/vol/newName")
	assert.NoError(t, err, "error returned while renaming a Qtree")
}

func TestQTreeModifyExportPolicy(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().QtreeModifyExportPolicy(ctx, "vol1", "trident-pvc-1234", "fake-policy").Return(nil)
	err := oapi.QtreeModifyExportPolicy(ctx, "vol1", "trident-pvc-1234", "fake-policy")
	assert.NoError(t, err, "error returned while modifying a export policy for Qtree")
}

func TestQtreeCount(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().QtreeCount(ctx, "vol1").Return(3, nil)
	count, err := oapi.QtreeCount(ctx, "vol1")
	assert.NoError(t, err, "error returned while getting a Qtree count")
	assert.Equal(t, 3, count)
}

func TestQtreeListByPrefix(t *testing.T) {
	qtree := getQtree()

	qtreeResponse := storage.QtreeCollectionGetOK{
		Payload: &models.QtreeResponse{
			QtreeResponseInlineRecords: []*models.Qtree{&qtree},
		},
	}

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Get the Qtree list
	rsi.EXPECT().QtreeList(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(&qtreeResponse, nil)
	volumeInfo, err := oapi.QtreeListByPrefix(ctx, *qtree.Name, *qtree.Volume.Name)
	assert.NoError(t, err, "error returned while getting a Qtree list")
	assert.Equal(t, *qtree.Name, volumeInfo[0].Name)

	// case 2: Get the Qtree list failed. Backend returned an error
	rsi.EXPECT().QtreeList(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil,
		errors.New("failed to get qtree for given prefix"))
	_, err = oapi.QtreeListByPrefix(ctx, *qtree.Name, *qtree.Volume.Name)
	assert.Error(t, err, "no error returned while getting a Qtree list")
}

func getQtree() models.Qtree {
	name := "qtree_vol1"
	securityStyle := models.SecurityStyleUnix
	unixPermission := int64(777)
	exportPolicy := "fake-export-policy"
	volumeName := "vol1"
	svm := "svm1"

	qtreeExportPolicy := models.QtreeInlineExportPolicy{Name: &exportPolicy}
	qtreeSVM := models.QtreeInlineSvm{Name: &svm}
	qtreeVolume := models.QtreeInlineVolume{Name: &volumeName}

	return models.Qtree{
		Name:            &name,
		SecurityStyle:   &securityStyle,
		UnixPermissions: &unixPermission,
		ExportPolicy:    &qtreeExportPolicy,
		Svm:             &qtreeSVM,
		Volume:          &qtreeVolume,
	}
}

func TestQtreeGetByName(t *testing.T) {
	qtree := getQtree()
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Get the Qtree
	rsi.EXPECT().QtreeGet(ctx, "vol1", "").Return(&qtree, nil)
	qtreeVolume, err := oapi.QtreeGetByName(ctx, "vol1", "")
	assert.NoError(t, err, "Failed to get Qtree by name")
	assert.Equal(t, "qtree_vol1", qtreeVolume.Name)
	assert.Equal(t, "unix", qtreeVolume.SecurityStyle)
	assert.Equal(t, "777", qtreeVolume.UnixPermissions)

	// case 2: Get the Qtree failed. Backend returned an error.
	rsi.EXPECT().QtreeGet(ctx, "vol1", "").Return(&qtree, errors.New("Failed to get qtree"))
	qtreeVolume, err = oapi.QtreeGetByName(ctx, "vol1", "")
	assert.Error(t, err, "no error returned while getting a Qtree")
}

func TestQuotaEntryList(t *testing.T) {
	quotaVolumeName := "quotaVolume"
	quotaQtreeName := "quotaQtree"
	hardLimit := int64(1073741810)

	oapi, rsi := newMockOntapAPIREST(t)

	quotaVolume := models.QuotaRuleInlineVolume{Name: &quotaVolumeName}
	quotaQtree := models.QuotaRuleInlineQtree{Name: &quotaQtreeName}
	quotaSpace := models.QuotaRuleInlineSpace{HardLimit: &hardLimit}
	quotaRule := models.QuotaRule{Volume: &quotaVolume, Qtree: &quotaQtree, Space: &quotaSpace}
	quotaRuleResponseInlineRecords := []*models.QuotaRule{&quotaRule}
	quotaRuleResponse := models.QuotaRuleResponse{QuotaRuleResponseInlineRecords: quotaRuleResponseInlineRecords}
	quotaRuleCollection := storage.QuotaRuleCollectionGetOK{Payload: &quotaRuleResponse}

	rsi.EXPECT().QuotaEntryList(ctx, quotaVolumeName).Return(&quotaRuleCollection, nil)

	quotaEntries, err := oapi.QuotaEntryList(ctx, quotaVolumeName)
	assert.NoError(t, err, "Failed to get quota entry")
	assert.Equal(t, 1, len(quotaEntries))
	assert.Equal(t, "/vol/quotaVolume/quotaQtree", quotaEntries[0].Target)
	assert.Equal(t, int64(1073741810), quotaEntries[0].DiskLimitBytes)

	rsi.EXPECT().QuotaEntryList(ctx, quotaVolumeName).Return(nil, errors.New("failed to quota entry"))
	_, err = oapi.QuotaEntryList(ctx, quotaVolumeName)
	assert.Error(t, err, "no error returned while getting a Qtrees")
}

func TestQuotaOff(t *testing.T) {
	quotaVolumeName := "quotaVolume"

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Quota OFF positive test
	rsi.EXPECT().QuotaOff(ctx, quotaVolumeName).Return(nil)
	err := oapi.QuotaOff(ctx, quotaVolumeName)
	assert.NoError(t, err, "error returned while doing quota off")

	// case 2: Quota OFF negative test
	rsi.EXPECT().QuotaOff(ctx, quotaVolumeName).Return(errors.New("error disabling quota"))
	err = oapi.QuotaOff(ctx, quotaVolumeName)
	assert.Error(t, err, "no error returned while doing quota off")
}

func TestQuotaOn(t *testing.T) {
	quotaVolumeName := "quotaVolume"

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Quota ON positive test
	rsi.EXPECT().QuotaOn(ctx, quotaVolumeName).Return(nil)
	err := oapi.QuotaOn(ctx, quotaVolumeName)
	assert.NoError(t, err, "error returned while doing quota on")

	// case 2: Quota ON negative test
	rsi.EXPECT().QuotaOn(ctx, quotaVolumeName).Return(errors.New("error enabling quota"))
	err = oapi.QuotaOn(ctx, quotaVolumeName)
	assert.Error(t, err, "no error returned while doing quota ON")
}

func TestQuotaResize(t *testing.T) {
	quotaVolumeName := "quotaVolume"

	oapi, _ := newMockOntapAPIREST(t)

	// case 1: resize quota space
	err := oapi.QuotaResize(ctx, quotaVolumeName)
	assert.NoError(t, err, "error returned while resizing a quota")
}

func TestQuotaStatus(t *testing.T) {
	volume := getVolumeInfo()

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Quota status positive test
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	quotaStatus, err := oapi.QuotaStatus(ctx, "vol1")
	assert.NoError(t, err, "error returned while getting a quota status")
	assert.Equal(t, *volume.Quota.State, quotaStatus)

	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(nil, errors.New("error enabling quota"))
	quotaStatus, err = oapi.QuotaStatus(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a quota status")

	volume.Quota.State = nil

	// case 2: Quota status in volume is nil
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	quotaStatus, err = oapi.QuotaStatus(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a quota status")

	volume.Quota = nil

	// case 3: Quota in volume is nil
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	quotaStatus, err = oapi.QuotaStatus(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a quota status")
}

func TestQuotaSetEntry(t *testing.T) {
	quotaVolumeName := "quotaVolume"
	quotaQtreeName := "quotaQtreeName"

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Quota modify positive test
	rsi.EXPECT().QuotaSetEntry(ctx, quotaVolumeName, quotaQtreeName, "user", "1099511613440").Return(nil)
	err := oapi.QuotaSetEntry(ctx, quotaVolumeName, quotaQtreeName, "user", "1073741810")
	assert.NoError(t, err, "error returned while modifying a quota")

	// case 2: Quota modify with disk limit empty
	rsi.EXPECT().QuotaSetEntry(ctx, quotaVolumeName, quotaQtreeName, "group", "-1").Return(nil)
	err = oapi.QuotaSetEntry(ctx, quotaVolumeName, quotaQtreeName, "group", "")
	assert.NoError(t, err, "error returned while modifying a quota")

	// case 3: Quota modify with disk limit -1
	rsi.EXPECT().QuotaSetEntry(ctx, quotaVolumeName, quotaQtreeName, "group", "-1024").Return(nil)
	err = oapi.QuotaSetEntry(ctx, quotaVolumeName, quotaQtreeName, "group", "-1")
	assert.NoError(t, err, "error returned while modifying a quota")

	// case 4: Quota modify with invalid disk limit
	err = oapi.QuotaSetEntry(ctx, quotaVolumeName, quotaQtreeName, "group", "invalid_diskLimit")
	assert.Error(t, err, "no error returned while modifying a quota")
}

func TestQuotaGetEntry(t *testing.T) {
	quotaVolumeName := "quotaVolume"
	quotaQtreeName := "quotaQtree"
	hardLimit := int64(1073741810)
	quotaType := "user"

	oapi, rsi := newMockOntapAPIREST(t)

	quotaVolume := models.QuotaRuleInlineVolume{Name: &quotaVolumeName}
	quotaQtree := models.QuotaRuleInlineQtree{Name: &quotaQtreeName}
	quotaSpace := models.QuotaRuleInlineSpace{HardLimit: &hardLimit}
	quotaRule := models.QuotaRule{Volume: &quotaVolume, Qtree: &quotaQtree, Space: &quotaSpace}

	// case 1: Get Quota entry
	rsi.EXPECT().QuotaGetEntry(ctx, quotaVolumeName, quotaQtreeName, quotaType).Return(&quotaRule, nil)
	quotaEntries, err := oapi.QuotaGetEntry(ctx, quotaVolumeName, quotaQtreeName, quotaType)
	assert.NoError(t, err, "error returned while getting a quota")
	assert.Equal(t, "/vol/quotaVolume/quotaQtree", quotaEntries.Target)
	assert.Equal(t, int64(1073741810), quotaEntries.DiskLimitBytes)

	// case 2: Quota get entry failed. Backend returned an error
	rsi.EXPECT().QuotaGetEntry(ctx, quotaVolumeName, quotaQtreeName, quotaType).Return(
		&quotaRule, errors.New("error getting quota rule"))
	_, err = oapi.QuotaGetEntry(ctx, quotaVolumeName, quotaQtreeName, quotaType)
	assert.NoError(t, err, "no error returned while getting a quota")
}

func getSnapshot() *models.Snapshot {
	snapshotName := "fake-snapshot"
	snapshotUUID := "fake-snapshotUUID"
	createTime1 := strfmt.NewDateTime()

	snapshot := models.Snapshot{Name: &snapshotName, CreateTime: &createTime1, UUID: &snapshotUUID}
	return &snapshot
}

func TestVolumeSnapshotCreate(t *testing.T) {
	volume := getVolumeInfo()

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Create volume snapshot
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	rsi.EXPECT().SnapshotCreateAndWait(ctx, *volume.UUID, "fake-snapshot").Return(nil)
	err := oapi.VolumeSnapshotCreate(ctx, "fake-snapshot", "vol1")
	assert.NoError(t, err, "error returned while creating a snapshot")

	// case 2: Create volume snapshot, parent volume verification returned error
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(nil, errors.New("Failed to get flexgroup"))
	err = oapi.VolumeSnapshotCreate(ctx, "fake-snapshot", "vol1")
	assert.Error(t, err, "no error returned while creating a snapshot")

	// case 3: Create volume snapshot returned error
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	rsi.EXPECT().SnapshotCreateAndWait(ctx, *volume.UUID, "fake-snapshot").Return(errors.New(
		"snapshot creation failed"))
	err = oapi.VolumeSnapshotCreate(ctx, "fake-snapshot", "vol1")
	assert.Error(t, err, "no error returned while creating a snapshot")
}

func TestVolumeCloneCreate(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: create volume clone
	rsi.EXPECT().VolumeCloneCreateAsync(ctx, "fake-cloneVolume", "fake-volume", "fake-snaphot").Return(nil)
	err := oapi.VolumeCloneCreate(ctx, "fake-cloneVolume", "fake-volume", "fake-snaphot", true)
	assert.NoError(t, err, "error returned while creating a clone")

	// case 2: create volume clone failed. Backend return an error
	rsi.EXPECT().VolumeCloneCreateAsync(ctx, "fake-cloneVolume", "fake-volume", "fake-snaphot").Return(
		errors.New("error creating clone"))
	err = oapi.VolumeCloneCreate(ctx, "fake-cloneVolume", "fake-volume", "fake-snaphot", true)
	assert.Error(t, err, "no error returned while creating a clone")
}

func TestVolumeSnapshotList(t *testing.T) {
	snapshotName1 := "snapvol1"
	snapshotName2 := "snapvol2"
	createTime1 := strfmt.NewDateTime()
	createTime2 := strfmt.NewDateTime()
	volume := getVolumeInfo()
	volumeUUID := *volume.UUID
	oapi, rsi := newMockOntapAPIREST(t)

	snap1 := models.Snapshot{Name: &snapshotName1, CreateTime: &createTime1}
	snap2 := models.Snapshot{Name: &snapshotName2, CreateTime: &createTime2}
	snapshotResponse := models.SnapshotResponse{SnapshotResponseInlineRecords: []*models.Snapshot{&snap1, &snap2}}
	snapshotResponseOK := storage.SnapshotCollectionGetOK{Payload: &snapshotResponse}

	// case 1: Get volume snapshot list
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	rsi.EXPECT().SnapshotList(ctx, volumeUUID).Return(&snapshotResponseOK, nil)
	snapshots, err := oapi.VolumeSnapshotList(ctx, "vol1")
	assert.NoError(t, err, "error returned while getting a snapshot list")
	assert.NotNil(t, snapshots)
	assert.Equal(t, "snapvol1", snapshots[0].Name, "snapshot name does not match")
	assert.Equal(t, createTime1.String(), snapshots[0].CreateTime, "snapshot creation time does not match")

	// case 2: Get volume snapshot parent volume verification returned error
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(nil, errors.New("failed to get flexgroup"))
	_, err = oapi.VolumeSnapshotList(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a snapshot list")

	// case 3: Get volume snapshot returned nil response
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	rsi.EXPECT().SnapshotList(ctx, volumeUUID).Return(nil, nil)
	snapshots, err = oapi.VolumeSnapshotList(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a snapshot list")

	// case 4: Get volume snapshot returned error
	rsi.EXPECT().VolumeGetByName(ctx, "vol1", gomock.Any()).Return(volume, nil)
	rsi.EXPECT().SnapshotList(ctx, volumeUUID).Return(nil, errors.New("failed to get snapshot info"))
	snapshots, err = oapi.VolumeSnapshotList(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a snapshot list")
}

func TestVolumeSetQosPolicyGroupName(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: modify volume Qos policy
	qosPolicy := api.QosPolicyGroup{Name: "fake-qosPolicy"}
	rsi.EXPECT().VolumeSetQosPolicyGroupName(ctx, "trident-pvc-1234", qosPolicy).Return(nil)
	err := oapi.VolumeSetQosPolicyGroupName(ctx, "trident-pvc-1234", qosPolicy)
	assert.NoError(t, err, "error returned while modifying a qos policy on volume")

	// case 2: Modify volume QoS policy returned error
	rsi.EXPECT().VolumeSetQosPolicyGroupName(ctx, "trident-pvc-1234", qosPolicy).Return(
		errors.New("failed to update comment on volume"))
	err = oapi.VolumeSetQosPolicyGroupName(ctx, "trident-pvc-1234", qosPolicy)
	assert.Error(t, err, "no error returned while modifying a qos policy on volume")
}

func TestVolumeCloneSplitStart(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Modify volume split clone.
	rsi.EXPECT().VolumeCloneSplitStart(ctx, "fake-cloneVolume").Return(nil)
	err := oapi.VolumeCloneSplitStart(ctx, "fake-cloneVolume")
	assert.NoError(t, err, "error returned while doing a split clone on volume")

	// case 2: Modify volume split clone. returned error
	rsi.EXPECT().VolumeCloneSplitStart(ctx, "fake-cloneVolume").Return(
		errors.New("volume splits clone failed"))
	err = oapi.VolumeCloneSplitStart(ctx, "fake-cloneVolume")
	assert.Error(t, err, "no error returned while doing a split clone on volume")
}

func TestSnapshotRestoreVolume(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Snapshot restored
	rsi.EXPECT().SnapshotRestoreVolume(ctx, "fake-snapshot", "fake-volume").Return(nil)
	err := oapi.SnapshotRestoreVolume(ctx, "fake-snapshot", "fake-volume")
	assert.NoError(t, err, "error returned while restoring a snapshot")

	// case 2:  Snapshot restore returned error
	rsi.EXPECT().SnapshotRestoreVolume(ctx, "fake-snapshot", "fake-volume").Return(
		errors.New("error restoring snapshot"))
	err = oapi.SnapshotRestoreVolume(ctx, "fake-snapshot", "fake-volume")
	assert.Error(t, err, "no error returned while restoring a snapshot")
}

func TestSnapshotRestoreFlexgroup(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1:  Flexgroup Snapshot restore
	rsi.EXPECT().SnapshotRestoreFlexgroup(ctx, "fake-snapshot", "fake-volume").Return(nil)
	err := oapi.SnapshotRestoreFlexgroup(ctx, "fake-snapshot", "fake-volume")
	assert.NoError(t, err, "error returned while restoring a snapshot for flexgroup")

	// case 2:  Flexgroup Snapshot restore returned error
	rsi.EXPECT().SnapshotRestoreFlexgroup(ctx, "fake-snapshot", "fake-volume").Return(
		errors.New("error restoring snapshot"))
	err = oapi.SnapshotRestoreFlexgroup(ctx, "fake-snapshot", "fake-volume")
	assert.Error(t, err, "no error returned while restoring a snapshot for flexgroup")
}

func TestSnapshotDeleteByNameAndStyle(t *testing.T) {
	snapshot := getSnapshot()
	jobId := strfmt.UUID("1234")
	jobLink := models.JobLink{UUID: &jobId}
	jobResponse := models.JobLinkResponse{Job: &jobLink}
	snapshotJobResponse := models.SnapshotJobLinkResponse{Job: &jobLink}
	snapResponse := storage.SnapshotDeleteAccepted{Payload: &snapshotJobResponse}
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1:  Snapshot delete by name
	rsi.EXPECT().SnapshotGetByName(ctx, "fake-volumeUUID", "fake-snapshot").Return(snapshot, nil)
	rsi.EXPECT().SnapshotDelete(ctx, "fake-volumeUUID", *snapshot.UUID).Return(&snapResponse, nil)
	rsi.EXPECT().PollJobStatus(ctx, &jobResponse).Return(nil)
	err := oapi.SnapshotDeleteByNameAndStyle(ctx, "fake-snapshot", "fake-volume", "fake-volumeUUID")
	assert.NoError(t, err, "error returned while deleting a snapshot by name")

	// case 2:  Snapshot verification returned error
	rsi.EXPECT().SnapshotGetByName(ctx, "fake-volumeUUID", "fake-snapshot").Return(nil, errors.New("error checking for snapshot"))
	err = oapi.SnapshotDeleteByNameAndStyle(ctx, "fake-snapshot", "fake-volume", "fake-volumeUUID")
	assert.Error(t, err, "no error returned while deleting a snapshot by name")

	// case 3:  Snapshot verification returned nil
	rsi.EXPECT().SnapshotGetByName(ctx, "fake-volumeUUID", "fake-snapshot").Return(nil, nil)
	err = oapi.SnapshotDeleteByNameAndStyle(ctx, "fake-snapshot", "fake-volume", "fake-volumeUUID")
	assert.Error(t, err, "no error returned while deleting a snapshot by name")

	// case 4:  Response contain Snapshot UUID nil
	snapshot.UUID = nil
	rsi.EXPECT().SnapshotGetByName(ctx, "fake-volumeUUID", "fake-snapshot").Return(snapshot, nil)
	err = oapi.SnapshotDeleteByNameAndStyle(ctx, "fake-snapshot", "fake-volume", "fake-volumeUUID")
	assert.Error(t, err, "no error returned while deleting a snapshot by name")

	// case 5:  Snapshot delete returned nil
	snapshot = getSnapshot()
	rsi.EXPECT().SnapshotGetByName(ctx, "fake-volumeUUID", "fake-snapshot").Return(snapshot, nil)
	rsi.EXPECT().SnapshotDelete(ctx, "fake-volumeUUID", *snapshot.UUID).Return(nil, nil)
	err = oapi.SnapshotDeleteByNameAndStyle(ctx, "fake-snapshot", "fake-volume", "fake-volumeUUID")
	assert.Error(t, err, "no error returned while deleting a snapshot by name")

	// case 6:  Snapshot delete returned error
	rsi.EXPECT().SnapshotGetByName(ctx, "fake-volumeUUID", "fake-snapshot").Return(snapshot, nil)
	rsi.EXPECT().SnapshotDelete(ctx, "fake-volumeUUID", *snapshot.UUID).Return(nil, errors.New("error while deleting snapshot"))
	err = oapi.SnapshotDeleteByNameAndStyle(ctx, "fake-snapshot", "fake-volume", "fake-volumeUUID")
	assert.Error(t, err, "no error returned while deleting a snapshot by name")

	// case 7:  Poll job status returned rest error.
	success := "success"
	code := int64(1638555)
	payload := models.Job{State: &success, Code: &code}
	restErr := api.NewRestErrorFromPayload(&payload)

	rsi.EXPECT().SnapshotGetByName(ctx, "fake-volumeUUID", "fake-snapshot").Return(snapshot, nil)
	rsi.EXPECT().SnapshotDelete(ctx, "fake-volumeUUID", *snapshot.UUID).Return(&snapResponse, nil)
	rsi.EXPECT().PollJobStatus(ctx, &jobResponse).Return(restErr)
	err = oapi.SnapshotDeleteByNameAndStyle(ctx, "fake-snapshot", "fake-volume", "fake-volumeUUID")
	assert.Error(t, err, "no error returned while deleting a snapshot by name")
}

func TestFlexgroupSnapshotDelete(t *testing.T) {
	volume := getVolumeInfo()
	volumeUUID := "fake-volumeUUID"
	volume.UUID = &volumeUUID
	snapshot := getSnapshot()
	jobId := strfmt.UUID("1234")
	jobLink := models.JobLink{UUID: &jobId}
	jobResponse := models.JobLinkResponse{Job: &jobLink}
	snapshotJobResponse := models.SnapshotJobLinkResponse{Job: &jobLink}
	snapResponse := storage.SnapshotDeleteAccepted{Payload: &snapshotJobResponse}
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1:  Flexgroup Snapshot delete
	rsi.EXPECT().FlexGroupGetByName(ctx, "fake-volume", gomock.Any()).Return(volume, nil)
	rsi.EXPECT().SnapshotGetByName(ctx, "fake-volumeUUID", "fake-snapshot").Return(snapshot, nil)
	rsi.EXPECT().SnapshotDelete(ctx, "fake-volumeUUID", *snapshot.UUID).Return(&snapResponse, nil)
	rsi.EXPECT().PollJobStatus(ctx, &jobResponse).Return(nil)
	err := oapi.FlexgroupSnapshotDelete(ctx, "fake-snapshot", "fake-volume")
	assert.NoError(t, err, "error returned while deleting a snapshot")

	// case 2:  Flexgroup Snapshot verification returned error
	rsi.EXPECT().FlexGroupGetByName(ctx, "fake-volume", gomock.Any()).Return(nil, errors.New("failed to get volume"))
	err = oapi.FlexgroupSnapshotDelete(ctx, "fake-snapshot", "fake-volume")
	assert.Error(t, err, "no error returned while deleting a snapshot")

	// case 3:  Flexgroup Snapshot verification returned nil
	rsi.EXPECT().FlexGroupGetByName(ctx, "fake-volume", gomock.Any()).Return(nil, nil)
	err = oapi.FlexgroupSnapshotDelete(ctx, "fake-snapshot", "fake-volume")
	assert.Error(t, err, "no error returned while deleting a snapshot")
}

func TestVolumeSnapshotDelete(t *testing.T) {
	volume := getVolumeInfo()
	volumeUUID := "fake-volumeUUID"
	volume.UUID = &volumeUUID
	snapshot := getSnapshot()
	jobId := strfmt.UUID("1234")
	jobLink := models.JobLink{UUID: &jobId}
	jobResponse := models.JobLinkResponse{Job: &jobLink}
	snapshotJobResponse := models.SnapshotJobLinkResponse{Job: &jobLink}
	snapResponse := storage.SnapshotDeleteAccepted{Payload: &snapshotJobResponse}
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1:  Volume Snapshot delete
	rsi.EXPECT().VolumeGetByName(ctx, "fake-volume", gomock.Any()).Return(volume, nil)
	rsi.EXPECT().SnapshotGetByName(ctx, "fake-volumeUUID", "fake-snapshot").Return(snapshot, nil)
	rsi.EXPECT().SnapshotDelete(ctx, "fake-volumeUUID", *snapshot.UUID).Return(&snapResponse, nil)
	rsi.EXPECT().PollJobStatus(ctx, &jobResponse).Return(nil)
	err := oapi.VolumeSnapshotDelete(ctx, "fake-snapshot", "fake-volume")
	assert.NoError(t, err, "error returned while deleting a snapshot")

	// case 2:  Volume Snapshot verification returned error
	rsi.EXPECT().VolumeGetByName(ctx, "fake-volume", gomock.Any()).Return(nil, errors.New("failed to get volume"))
	err = oapi.VolumeSnapshotDelete(ctx, "fake-snapshot", "fake-volume")
	assert.Error(t, err, "no error returned while deleting a snapshot")

	// case 3:  Volume Snapshot verification returned nil
	rsi.EXPECT().VolumeGetByName(ctx, "fake-volume", gomock.Any()).Return(nil, nil)
	err = oapi.VolumeSnapshotDelete(ctx, "fake-snapshot", "fake-volume")
	assert.Error(t, err, "no error returned while deleting a snapshot")
}

func TestVolumeListBySnapshotParent(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1:  Get volume Snapshot list
	childVolumes := []string{"volum1", "volum2", "volum3"}
	rsi.EXPECT().VolumeListAllBackedBySnapshot(ctx, "fake-snapshot", "fake-volume").Return(childVolumes, nil)
	volumeList, err := oapi.VolumeListBySnapshotParent(ctx, "fake-volume", "fake-snapshot")
	assert.NoError(t, err, "error returned while getting a volume")
	assert.Equal(t, "volum1", volumeList[0], "volume name does not match")

	// case 2:  Get volume Snapshot list returned nil
	rsi.EXPECT().VolumeListAllBackedBySnapshot(ctx, "fake-snapshot", "fake-volume").Return([]string{}, nil)
	volumeList, err = oapi.VolumeListBySnapshotParent(ctx, "fake-volume", "fake-snapshot")
	assert.NoError(t, err, "error returned while getting a volume")

	// case 3:  Get volume Snapshot list returned error
	rsi.EXPECT().VolumeListAllBackedBySnapshot(ctx, "fake-snapshot", "fake-volume").
		Return(nil, errors.New("child volume not found"))
	volumeList, err = oapi.VolumeListBySnapshotParent(ctx, "fake-volume", "fake-snapshot")
	assert.Error(t, err, "no error returned while getting a volume")
}

func TestSnapmirrorDeleteViaDestination(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: SnapMirror delete via destination
	rsi.EXPECT().SnapmirrorDeleteViaDestination(ctx, "vol1", "svm").Return(nil)
	rsi.EXPECT().SnapmirrorRelease(ctx, "vol1", "svm").Return(nil)
	err := oapi.SnapmirrorDeleteViaDestination(ctx, "vol1", "svm")
	assert.NoError(t, err, "error returned while deleting a SnapMirror via destination")

	// case 2: SnapMirror delete via destination returned error
	rsi.EXPECT().SnapmirrorDeleteViaDestination(ctx, "vol1", "svm").Return(
		errors.New("error while deleting snapmirror"))
	rsi.EXPECT().SnapmirrorRelease(ctx, "vol1", "svm").Return(errors.New("error while deleting snapmirror"))
	err = oapi.SnapmirrorDeleteViaDestination(ctx, "vol1", "svm")
	assert.Error(t, err, "no error returned while deleting a SnapMirror via destination")
}

func TestSnapmirrorRelease(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: SnapMirror delete
	rsi.EXPECT().SnapmirrorRelease(ctx, "vol1", "svm").Return(nil)
	err := oapi.SnapmirrorRelease(ctx, "vol1", "svm")
	assert.NoError(t, err, "error returned while deleting a SnapMirror")

	// case 2: SnapMirror deletereturned error
	rsi.EXPECT().SnapmirrorRelease(ctx, "vol1", "svm").Return(errors.New("error deleting snapmirror"))
	err = oapi.SnapmirrorRelease(ctx, "vol1", "svm")
	assert.Error(t, err, "no error returned while deleting a SnapMirror")
}

func TestIsSVMDRCapable(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().IsVserverDRCapable(ctx).Return(true, nil)
	SVMDRCapable, err := oapi.IsSVMDRCapable(ctx)
	assert.NoError(t, err, "error returned while verifying a svm is VM DR capable")
	assert.Equal(t, true, SVMDRCapable, "svm is not DR capable")
}

func TestSnapmirrorCreate(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().SnapmirrorCreate(ctx, "", "", "", "", "", "").Return(nil)
	err := oapi.SnapmirrorCreate(ctx, "", "", "", "", "", "")
	assert.NoError(t, err, "error returned while creating a SnapMirror")
}

func TestSnapMirrorGet(t *testing.T) {
	state := "in_sync"
	lastTransferType := "resync"
	stateTransfer := "success"
	endTime := strfmt.NewDateTime()
	healthy := false
	snapMirrorErrMsg := "Destination is running out of disk space"
	policyName := "Asynchronous"
	transferScheduleName := "weekly"

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Get SnapMirror
	policy := models.SnapmirrorRelationshipInlinePolicy{Name: &policyName}
	snapMirrorError := models.SnapmirrorError{Message: &snapMirrorErrMsg}
	snapMirrorErrorList := []*models.SnapmirrorError{&snapMirrorError}
	transfer := models.SnapmirrorRelationshipInlineTransfer{State: &stateTransfer, EndTime: &endTime}
	transferSchedule := models.SnapmirrorRelationshipInlineTransferSchedule{Name: &transferScheduleName}
	snapmirrorRelationship := models.SnapmirrorRelationship{
		Healthy:          &healthy,
		LastTransferType: &lastTransferType,
		State:            &state,
		Transfer:         &transfer,
		SnapmirrorRelationshipInlineUnhealthyReason: snapMirrorErrorList,
		Policy:           &policy,
		TransferSchedule: &transferSchedule,
	}
	rsi.EXPECT().SnapmirrorGet(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM", gomock.Any()).Return(&snapmirrorRelationship, nil)
	snapmirror, err := oapi.SnapmirrorGet(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM")
	assert.NoError(t, err, "error returned while getting a SnapMirror")
	assert.Equal(t, api.SnapmirrorStateInSync, snapmirror.State, "snapmirror state is not sync")
	assert.Equal(t, lastTransferType, snapmirror.LastTransferType, "lastTransferType does not match")
	assert.Equal(t, api.SnapmirrorStatusSuccess, snapmirror.RelationshipStatus, "snapmirror status does not match")
	assert.Equal(t, false, snapmirror.IsHealthy, "snapmirror is not healthy")
	assert.Equal(t, snapMirrorErrMsg, snapmirror.UnhealthyReason,
		"snapmirror error response does not match with unhealthy error message")
	assert.Equal(t, policyName, snapmirror.ReplicationPolicy, "policy name does not match")
}

func TestSnapmirrorInitialize(t *testing.T) {
	success := "failure"
	code := int64(13303812)
	message := "snapshot busy"
	payload := models.Job{State: &success, Code: &code, Message: &message}

	restErr := api.NewRestErrorFromPayload(&payload)

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Initialize SnapMirror
	rsi.EXPECT().SnapmirrorInitialize(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM").Return(nil)
	err := oapi.SnapmirrorInitialize(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM")
	assert.NoError(t, err, "error returned while initialising a SnapMirror")

	// case 2: Initialize SnapMirror returned error
	rsi.EXPECT().SnapmirrorInitialize(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM").Return(restErr)
	err = oapi.SnapmirrorInitialize(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM")
	assert.Error(t, err, "no error returned while initialising a SnapMirror")
}

func TestSnapmirrorDelete(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Delete SnapMirror
	rsi.EXPECT().SnapmirrorDelete(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM").Return(nil)
	err := oapi.SnapmirrorDelete(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM")
	assert.NoError(t, err, "error returned while deleting a SnapMirror")

	// case 2: Delete SnapMirror returned error
	rsi.EXPECT().SnapmirrorDelete(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM").Return(errors.New("unable to delete snapmirror"))
	err = oapi.SnapmirrorDelete(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM")
	assert.Error(t, err, "no error returned while deleting a SnapMirror")
}

func TestSnapmirrorResync(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Resync SnapMirror
	rsi.EXPECT().SnapmirrorResync(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM").Return(nil)
	err := oapi.SnapmirrorResync(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM")
	assert.NoError(t, err, "error returned while resyncing a SnapMirror")

	// case 2: Resync SnapMirror returned error
	rsi.EXPECT().SnapmirrorResync(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume", "fake-remoteSVM").Return(
		errors.New("unable to resync snapmirror"))
	rsi.EXPECT().SnapmirrorDelete(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM").Return(nil)
	err = oapi.SnapmirrorResync(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM")
	assert.Error(t, err, "no error returned while resyncing a SnapMirror")

	// case 3: Delete SnapMirror returned error
	rsi.EXPECT().SnapmirrorResync(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume", "fake-remoteSVM").Return(
		errors.New("unable to resync snapmirror"))
	rsi.EXPECT().SnapmirrorDelete(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM").Return(errors.New("unable to delete snapmirror"))
	err = oapi.SnapmirrorResync(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM")
	assert.Error(t, err, "no error returned while resyncing a SnapMirror")
}

func TestSnapmirrorPolicyGet(t *testing.T) {
	policyName := ""
	policyType := "sync"
	policySyncType := "sync_mirror"
	copyAllSourceSnapshots := false
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Get SnapMirror policy
	snapMirroePolicy := models.SnapmirrorPolicy{
		Name: &policyName, CopyAllSourceSnapshots: &copyAllSourceSnapshots,
		SyncType: &policySyncType, Type: &policyType,
	}
	snapmirrorPolicyRecords := []*models.SnapmirrorPolicy{&snapMirroePolicy}
	snapmirrorPolicyResponse := models.SnapmirrorPolicyResponse{SnapmirrorPolicyResponseInlineRecords: snapmirrorPolicyRecords}
	snapmirrorPoliciesGetOK := snapmirror.SnapmirrorPoliciesGetOK{Payload: &snapmirrorPolicyResponse}

	rsi.EXPECT().SnapmirrorPolicyGet(ctx, "fake-replicationPolicy").Return(&snapmirrorPoliciesGetOK, nil)
	snapmirrorPolicy, err := oapi.SnapmirrorPolicyGet(ctx, "fake-replicationPolicy")
	assert.NoError(t, err, "error returned while getting a SnapMirror policy")
	assert.Equal(t, api.SnapmirrorPolicyZAPITypeSync, snapmirrorPolicy.Type, "SnapMirror type is not sync")
	assert.Equal(t, false, snapmirrorPolicy.CopyAllSnapshots, "SnapMirror policy is CopyAllSnapshots")

	// case 2: Get SnapMirror policy type async
	policyType = "async"
	rsi.EXPECT().SnapmirrorPolicyGet(ctx, "fake-replicationPolicy").Return(&snapmirrorPoliciesGetOK, nil)
	snapmirrorPolicy, err = oapi.SnapmirrorPolicyGet(ctx, "fake-replicationPolicy")
	assert.NoError(t, err, "error returned while getting a SnapMirror policy")
	assert.Equal(t, api.SnapmirrorPolicyRESTTypeAsync, snapmirrorPolicy.Type, "SnapMirror type is not async")
	assert.Equal(t, false, snapmirrorPolicy.CopyAllSnapshots, "SnapMirror policy is CopyAllSnapshots")

	// case 3: Get SnapMirror policy type sync
	policyType = "sync"
	snapMirroePolicy = models.SnapmirrorPolicy{Name: &policyName, Type: &policyType}
	snapmirrorPolicyRecords = []*models.SnapmirrorPolicy{&snapMirroePolicy}
	snapmirrorPolicyResponse = models.SnapmirrorPolicyResponse{SnapmirrorPolicyResponseInlineRecords: snapmirrorPolicyRecords}
	snapmirrorPoliciesGetOK = snapmirror.SnapmirrorPoliciesGetOK{Payload: &snapmirrorPolicyResponse}

	rsi.EXPECT().SnapmirrorPolicyGet(ctx, "fake-replicationPolicy").Return(&snapmirrorPoliciesGetOK, nil)
	snapmirrorPolicy, err = oapi.SnapmirrorPolicyGet(ctx, "fake-replicationPolicy")
	assert.Error(t, err, "no error returned while getting a SnapMirror policy")

	// case 4: Get SnapMirror policy returned error
	snapMirroePolicy = models.SnapmirrorPolicy{Name: &policyName}
	snapmirrorPolicyRecords = []*models.SnapmirrorPolicy{&snapMirroePolicy}
	snapmirrorPolicyResponse = models.SnapmirrorPolicyResponse{SnapmirrorPolicyResponseInlineRecords: snapmirrorPolicyRecords}
	snapmirrorPoliciesGetOK = snapmirror.SnapmirrorPoliciesGetOK{Payload: &snapmirrorPolicyResponse}

	rsi.EXPECT().SnapmirrorPolicyGet(ctx, "fake-replicationPolicy").Return(&snapmirrorPoliciesGetOK, nil)
	snapmirrorPolicy, err = oapi.SnapmirrorPolicyGet(ctx, "fake-replicationPolicy")
	assert.Error(t, err, "no error returned while getting a SnapMirror policy")

	// case 5: Get SnapMirror policy returned nil
	snapmirrorPolicyRecords = []*models.SnapmirrorPolicy{nil}
	snapmirrorPolicyResponse = models.SnapmirrorPolicyResponse{SnapmirrorPolicyResponseInlineRecords: snapmirrorPolicyRecords}
	snapmirrorPoliciesGetOK = snapmirror.SnapmirrorPoliciesGetOK{Payload: &snapmirrorPolicyResponse}

	rsi.EXPECT().SnapmirrorPolicyGet(ctx, "fake-replicationPolicy").Return(&snapmirrorPoliciesGetOK, nil)
	snapmirrorPolicy, err = oapi.SnapmirrorPolicyGet(ctx, "fake-replicationPolicy")
	assert.Error(t, err, "no error returned while getting a SnapMirror policy")

	snapmirrorPoliciesGetOK = snapmirror.SnapmirrorPoliciesGetOK{Payload: nil}

	// case 6: Get SnapMirror policy returned error
	rsi.EXPECT().SnapmirrorPolicyGet(ctx, "fake-replicationPolicy").Return(&snapmirrorPoliciesGetOK, nil)
	snapmirrorPolicy, err = oapi.SnapmirrorPolicyGet(ctx, "fake-replicationPolicy")
	assert.Error(t, err, "no error returned while getting a SnapMirror policy")
}

func TestSnapmirrorQuiesce(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Quiesce a SnapMirror
	rsi.EXPECT().SnapmirrorQuiesce(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM").Return(nil)
	err := oapi.SnapmirrorQuiesce(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM")
	assert.NoError(t, err, "error returned while quiescing a SnapMirror")

	// case 2: Quiesce a SnapMirror failed.
	rsi.EXPECT().SnapmirrorQuiesce(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM").Return(errors.New("quiesce a SnapMirror failed"))
	err = oapi.SnapmirrorQuiesce(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM")
	assert.Error(t, err, "no error returned while quiescing a SnapMirror")
}

func TestSnapmirrorAbort(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1; Abort the SnapMirror
	rsi.EXPECT().SnapmirrorAbort(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM").Return(nil)
	err := oapi.SnapmirrorAbort(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM")
	assert.NoError(t, err, "error returned while aborting a SnapMirror")

	// case 2; Abort the SnapMirror failed. Backend return an error
	rsi.EXPECT().SnapmirrorAbort(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM").Return(errors.New("quiesce a SnapMirror failed"))
	err = oapi.SnapmirrorAbort(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM")
	assert.Error(t, err, "no error returned while aborting a SnapMirror")
}

func TestSnapmirrorBreak(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Positive test, break the SnapMirror.
	rsi.EXPECT().SnapmirrorBreak(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM", "fake-snapshot").Return(nil)
	err := oapi.SnapmirrorBreak(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM", "fake-snapshot")
	assert.NoError(t, err, "error returned while breaking a SnapMirror")

	// case 2: Negative test, break the SnapMirror. backend return an error.
	rsi.EXPECT().SnapmirrorBreak(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM", "fake-snapshot").Return(errors.New("quiesce a SnapMirror failed"))
	err = oapi.SnapmirrorBreak(ctx, "fake-localVolume", "fake-localSvm", "fake-remoteVolume",
		"fake-remoteSVM", "fake-snapshot")
	assert.Error(t, err, "no error returned while breaking a SnapMirror")
}

func TestSnapmirrorUpdate(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Positive test update the SnapMirror.
	rsi.EXPECT().SnapmirrorUpdate(ctx, gomock.Any(), gomock.Any()).Return(nil)
	err := oapi.SnapmirrorUpdate(ctx, "fake-localVolume", "fake-snapshot")
	assert.NoError(t, err, "error returned while updating a SnapMirror")

	// case 2: Negative test update the SnapMirror. Backend return an error.
	rsi.EXPECT().SnapmirrorUpdate(ctx, gomock.Any(), gomock.Any()).Return(errors.New("SnapMirror update failed"))
	err = oapi.SnapmirrorUpdate(ctx, "fake-localVolume", "fake-snapshot")
	assert.Error(t, err, "no error returned while updating a SnapMirror")
}

func TestJobScheduleExists(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Positive test, verify job scheduled
	rsi.EXPECT().JobScheduleExists(ctx, "fake-replicationSchedule").Return(true, nil)
	jobScheduleExists, err := oapi.JobScheduleExists(ctx, "fake-replicationSchedule")
	assert.NoError(t, err, "error returned while verifying a job")
	assert.Equal(t, true, jobScheduleExists, "job does not exist")

	// case 2: Negative test, verify job scheduled
	rsi.EXPECT().JobScheduleExists(ctx, "fake-replicationSchedule").Return(
		false, errors.New("failed to get job"))
	_, err = oapi.JobScheduleExists(ctx, "fake-replicationSchedule")
	assert.Error(t, err, "no error returned while verifying a job")
}

func TestGetSVMPeers(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Positive test get the peer SVM
	rsi.EXPECT().GetPeeredVservers(ctx).Return([]string{"svm1"}, nil)
	svms, err := oapi.GetSVMPeers(ctx)
	assert.NoError(t, err, "error returned while getting a peer system SVM")
	assert.Equal(t, "svm1", svms[0], "svm name does not match")

	// case 2: Negative test get the peer SVM
	rsi.EXPECT().GetPeeredVservers(ctx).Return([]string{}, errors.New("failed to get SVM"))
	_, err = oapi.GetSVMPeers(ctx)
	assert.Error(t, err, "no error returned while getting a peer system SVM")
}

func getLunInfo() *models.Lun {
	comment := "lun for flexvol"
	igroup1 := "igroup1"
	logicalUnitNumber := int64(12345)
	size := int64(2147483648)
	qosPolicyName := "fake-qosPolicy"
	mapStatus := true
	volumeName := "fake-volume"
	createTime1 := strfmt.NewDateTime()
	enabled := false
	lunName := "fake-lunName"
	lunUUID := "fake-lunUUID"
	lunSerialNumber := "fake-serialNumber"
	lunState := "online"

	igroup := models.LunInlineLunMapsInlineArrayItemInlineIgroup{Name: &igroup1}
	lunMap := []*models.LunInlineLunMapsInlineArrayItem{
		{Igroup: &igroup, LogicalUnitNumber: &logicalUnitNumber},
	}

	space := models.LunInlineSpace{Size: &size}
	qosPolicy := models.LunInlineQosPolicy{Name: &qosPolicyName}
	status := models.LunInlineStatus{Mapped: &mapStatus, State: &lunState}
	location := &models.LunInlineLocation{
		Volume: &models.LunInlineLocationInlineVolume{
			Name: &volumeName,
		},
	}

	lun := models.Lun{
		Name:             &lunName,
		UUID:             &lunUUID,
		SerialNumber:     &lunSerialNumber,
		Status:           &status,
		Enabled:          &enabled,
		Comment:          &comment,
		Space:            &space,
		CreateTime:       &createTime1,
		Location:         location,
		QosPolicy:        &qosPolicy,
		LunInlineLunMaps: lunMap,
	}
	return &lun
}

func TestLunList(t *testing.T) {
	lun := getLunInfo()
	oapi, rsi := newMockOntapAPIREST(t)

	lunResponse := s_a_n.LunCollectionGetOK{
		Payload: &models.LunResponse{LunResponseInlineRecords: []*models.Lun{lun}},
	}

	// case 1: Get LUN list
	rsi.EXPECT().LunList(ctx, "", gomock.Any()).Return(&lunResponse, nil)
	luns, err := oapi.LunList(ctx, "")
	assert.NoError(t, err, "error returned while getting a LUN info")
	assert.Equal(t, *lun.Name, luns[0].Name, "LUN name does not match")

	// case 2: Get LUN list returned error
	rsi.EXPECT().LunList(ctx, "", gomock.Any()).Return(nil, errors.New("lun not found with given pattern"))
	luns, err = oapi.LunList(ctx, "")
	assert.Error(t, err, "no error returned while getting a LUN info")

	// case 3: Get LUN list returned nil
	lunResponse1 := s_a_n.LunCollectionGetOK{
		Payload: &models.LunResponse{LunResponseInlineRecords: []*models.Lun{nil}},
	}
	rsi.EXPECT().LunList(ctx, "", gomock.Any()).Return(&lunResponse1, nil)
	luns, err = oapi.LunList(ctx, "")
	assert.Error(t, err, "no error returned while getting a LUN info")
}

func TestLunCreate(t *testing.T) {
	boolValue := true
	lun := api.Lun{
		Comment:        "",
		Enabled:        true,
		Name:           "fake-lun",
		Qos:            api.QosPolicyGroup{},
		Size:           "2147483648",
		Mapped:         true,
		UUID:           "fake-UUID",
		State:          "online",
		OsType:         "linux",
		SpaceReserved:  &boolValue,
		SpaceAllocated: &boolValue,
	}
	oapi, rsi := newMockOntapAPIREST(t)

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	// common call for all subtests
	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// case 1: Create LUN
	rsi.EXPECT().LunCreate(ctx, lun.Name, int64(2147483648), lun.OsType, lun.Qos, lun.SpaceReserved,
		lun.SpaceAllocated).Return(nil)
	err := oapi.LunCreate(ctx, lun)
	assert.NoError(t, err, "error returned while creating a LUN info")

	// case 2: Create LUN returned error
	rsi.EXPECT().LunCreate(ctx, lun.Name, int64(2147483648), lun.OsType, lun.Qos, lun.SpaceReserved,
		lun.SpaceAllocated).Return(errors.New("Failed to create LUN"))
	err = oapi.LunCreate(ctx, lun)
	assert.Error(t, err, "no error returned while creating a LUN info")
}

func TestLunDestroy(t *testing.T) {
	lun := getLunInfo()

	oapi, rsi := newMockOntapAPIREST(t)

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	// common call for all subtests
	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// case 1: Delete LUN
	rsi.EXPECT().LunGetByName(ctx, "/"+*lun.Name, gomock.Any()).Return(lun, nil)
	rsi.EXPECT().LunDelete(ctx, *lun.UUID).Return(nil)
	err := oapi.LunDestroy(ctx, "/"+*lun.Name)
	assert.NoError(t, err, "error returned while deleting a LUN")

	// case 2: Delete LUN returned error
	rsi.EXPECT().LunGetByName(ctx, "/"+*lun.Name, gomock.Any()).Return(lun, nil)
	rsi.EXPECT().LunDelete(ctx, *lun.UUID).Return(errors.New("failed to delete lun"))
	err = oapi.LunDestroy(ctx, "/"+*lun.Name)
	assert.Error(t, err, "no error returned while deleting a LUN")

	// case 3: Delete LUN, LUN verification returned error
	rsi.EXPECT().LunGetByName(ctx, "/"+*lun.Name, gomock.Any()).Return(nil, errors.New("failed to get lun"))
	err = oapi.LunDestroy(ctx, "/"+*lun.Name)
	assert.Error(t, err, "no error returned while deleting a LUN")

	lun.UUID = nil
	// case 4: LUN response contain LUN UUID nil
	rsi.EXPECT().LunGetByName(ctx, "/"+*lun.Name, gomock.Any()).Return(lun, nil)
	err = oapi.LunDestroy(ctx, "/"+*lun.Name)
	assert.Error(t, err, "no error returned while deleting a LUN")
}

func TestLunGetGeometry(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	// common call for all subtests
	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// case 1: Positive test, get the LUN options
	rsi.EXPECT().LunOptions(ctx).Return(&api.LunOptionsResult{}, nil)
	_, err := oapi.LunGetGeometry(ctx, "/")
	assert.NoError(t, err, "error returned while getting a LUN option")

	// case 2: Positive test, get the LUN options returned error.
	rsi.EXPECT().LunOptions(ctx).Return(&api.LunOptionsResult{}, errors.New("failed to get lun option"))
	_, err = oapi.LunGetGeometry(ctx, "/")
	assert.Error(t, err, "no error returned while getting a LUN option")

	// case 1: Positive test, get the LUN options returned nil response.
	rsi.EXPECT().LunOptions(ctx).Return(nil, nil)
	_, err = oapi.LunGetGeometry(ctx, "/")
	assert.Error(t, err, "no error returned while getting a LUN option")
}

func TestLunSetAttribute(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Positive test, update LUN attribute. context is empty.
	rsi.EXPECT().LunSetAttribute(ctx, "/", "filesystem", "fake-FStype").Return(nil)
	err := oapi.LunSetAttribute(ctx, "/", "filesystem", "fake-FStype", "", "", "")
	assert.NoError(t, err, "error returned while modifying a LUN attribute")

	// case 2: Positive test, update LUN attribute. pass the value in context..
	rsi.EXPECT().LunSetAttribute(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	err = oapi.LunSetAttribute(ctx, "/", "filesystem", "fake-FStype",
		"context", "LUKS", "formatOptions")
	assert.NoError(t, err, "error returned while modifying a LUN attribute")

	// case 3 Negative test, update LUN attribute returned error..
	err = oapi.LunSetAttribute(ctx, "failure_7c3a89e2_7d83_457b_9e29_bfdb082c1d8b",
		"filesystem", "fake-FStype", "context", "LUKS", "formatOptions")
	assert.Error(t, err, "no error returned while modifying a LUN attribute")
}

func TestLunGetAttribute(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	tempLunPath := "/vol/vol1/lun0"
	tempAttributeName := "fsType"

	// 1 - Negative test, d.api.LunGetAttribute returns error
	rsi.EXPECT().LunGetAttribute(gomock.Any(), tempLunPath, tempAttributeName).Return("", errors.New("error")).Times(1)
	attributeValue, err := oapi.LunGetAttribute(ctx, tempLunPath, tempAttributeName)
	assert.Error(t, err)
	assert.Equal(t, "", attributeValue)

	tempAttributeVale := "ext4"
	// 2 - Positive test, d.api.LunGetAttribute do not return error
	rsi.EXPECT().LunGetAttribute(gomock.Any(), tempLunPath, tempAttributeName).Return(tempAttributeVale, nil).Times(1)
	attributeValue, err = oapi.LunGetAttribute(ctx, tempLunPath, tempAttributeName)
	assert.NoError(t, err)
	assert.Equal(t, tempAttributeVale, attributeValue)
}

func TestLunCloneCreate(t *testing.T) {
	lun := getLunInfo()
	oapi, rsi := newMockOntapAPIREST(t)

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	// common call for all subtests
	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// case 1, Positive test, create clone
	rsi.EXPECT().LunCloneCreate(ctx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	rsi.EXPECT().LunGetByName(ctx, gomock.Any(), gomock.Any()).Return(lun, nil)
	err := oapi.LunCloneCreate(ctx, "fake-cloneVolume", "fake-volume", "fake-snaphot", api.QosPolicyGroup{})
	assert.NoError(t, err, "error returned while cloning a LUN")

	// case 2, Negative test, Unable to get LUN info. Beckend returned error
	rsi.EXPECT().LunGetByName(ctx, gomock.Any(), gomock.Any()).Return(lun, errors.New("failed to get lun"))
	err = oapi.LunCloneCreate(ctx, "fake-cloneVolume", "fake-volume", "fake-snaphot", api.QosPolicyGroup{})
	assert.Error(t, err, "no error returned while cloning a LUN")

	// case 3, Negative test, Unable to get LUN info. Getting nil response.
	rsi.EXPECT().LunGetByName(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
	err = oapi.LunCloneCreate(ctx, "fake-cloneVolume", "fake-volume", "fake-snaphot", api.QosPolicyGroup{})
	assert.Error(t, err, "no error returned while cloning a LUN")
}

func TestLunSetComments(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().LunSetComment(ctx, gomock.Any(), gomock.Any()).Return(nil)
	err := oapi.LunSetComment(ctx, "/", "fake-context")
	assert.NoError(t, err, "error returned while modifying a comment on LUN")
}

func TestLunSetQosPolicy(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: positive test, update QoS policy.
	rsi.EXPECT().LunSetQosPolicyGroup(ctx, gomock.Any(), gomock.Any()).Return(nil)
	err := oapi.LunSetQosPolicyGroup(ctx, "/vol/lun0", api.QosPolicyGroup{})
	assert.NoError(t, err, "error returned while modifying a qos policy on LUN")
}

func TestLunGetByName(t *testing.T) {
	lun := getLunInfo()
	oapi, rsi := newMockOntapAPIREST(t)

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	// common call for all subtests
	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// case 1: Positive test, get lun info by name
	rsi.EXPECT().LunGetByName(ctx, gomock.Any(), gomock.Any()).Return(lun, nil)
	lunResponse, err := oapi.LunGetByName(ctx, "fake-lun")
	assert.NoError(t, err, "error returned while getting a LUN by name")
	assert.Equal(t, *lun.Name, lunResponse.Name, "Lun name does not match")

	// case 2: Negative test, backend returned error
	rsi.EXPECT().LunGetByName(ctx, gomock.Any(), gomock.Any()).Return(lun, errors.New("failed to get lun"))
	lunResponse, err = oapi.LunGetByName(ctx, "fake-lun")
	assert.Error(t, err, "no error returned while getting a LUN by name")

	// case 3: Negative test, backend returned nil response
	rsi.EXPECT().LunGetByName(ctx, gomock.Any(), gomock.Any()).Return(nil, nil)
	lunResponse, err = oapi.LunGetByName(ctx, "fake-lun")
	assert.Error(t, err, "no error returned while getting a LUN by name")
}

func TestOntapAPIREST_LunExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAPI := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(mockAPI)
	assert.NoError(t, err)

	ctx := context.Background()
	lunPath := "/vol/vol1/lun0"

	// Mock ClientConfig to avoid unexpected call
	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}
	mockAPI.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// Case 1: LUN exists
	mockAPI.EXPECT().LunGetByName(ctx, lunPath, gomock.Any()).Return(&models.Lun{}, nil).Times(1)
	exists, err := oapi.LunExists(ctx, lunPath)
	assert.NoError(t, err, "expected no error when LUN exists")
	assert.True(t, exists, "expected LUN to exist")

	// Case 2: LUN does not exist
	mockAPI.EXPECT().LunGetByName(ctx, lunPath, gomock.Any()).Return(nil, nil).Times(1)
	exists, err = oapi.LunExists(ctx, lunPath)
	assert.NoError(t, err, "expected no error when LUN does not exist")
	assert.False(t, exists, "expected LUN to not exist")

	// Case 3: NotFoundError from REST client
	mockAPI.EXPECT().LunGetByName(ctx, lunPath, gomock.Any()).Return(
		nil, errors.NotFoundError("LUN not found")).Times(1)
	exists, err = oapi.LunExists(ctx, lunPath)
	assert.NoError(t, err, "expected no error when NotFoundError is returned")
	assert.False(t, exists, "expected LUN to not exist")

	// Case 4: Unexpected error
	mockAPI.EXPECT().LunGetByName(ctx, lunPath, gomock.Any()).Return(nil, errors.New("unexpected error")).Times(1)
	exists, err = oapi.LunExists(ctx, lunPath)
	assert.Error(t, err, "expected error when unexpected error is returned")
	assert.False(t, exists, "expected LUN to not exist")
}

func TestLunRename(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	// common call for all subtests
	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// case 1: Positive test, renamed the LUN
	rsi.EXPECT().LunRename(ctx, gomock.Any(), gomock.Any()).Return(nil)
	err := oapi.LunRename(ctx, "fake-lun", "fake-newLun")
	assert.NoError(t, err, "error returned while renaming a LUN")

	// case 2: Negative test, renamed the LUN
	rsi.EXPECT().LunRename(ctx, gomock.Any(), gomock.Any()).Return(errors.New("lun rename failed"))
	err = oapi.LunRename(ctx, "fake-lun", "fake-newLun")
	assert.Error(t, err, "no error returned while renaming a LUN")
}

func TestLunMapInfo(t *testing.T) {
	igroupName := "igroup"
	logicalUnitNumber := int64(1234)

	oapi, rsi := newMockOntapAPIREST(t)

	lunMap := models.LunMap{
		Igroup:            &models.LunMapInlineIgroup{Name: &igroupName},
		LogicalUnitNumber: &logicalUnitNumber,
	}
	lunMapResponse := models.LunMapResponse{LunMapResponseInlineRecords: []*models.LunMap{&lunMap}}
	lunMapresponseList := s_a_n.LunMapCollectionGetOK{Payload: &lunMapResponse}

	// case 1: Positive test, get lun map info
	rsi.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(&lunMapresponseList, nil)
	lunid, err := oapi.LunMapInfo(ctx, igroupName, "/vol/lun0")
	assert.NoError(t, err, "error returned while getting a LUN map info")
	assert.Equal(t, logicalUnitNumber, int64(lunid), "logical unit does not match")

	// case 2: Negative test, get lun map info
	rsi.EXPECT().LunMapInfo(ctx, gomock.Any(), gomock.Any()).Return(nil, errors.New("failed to get lun map"))
	_, err = oapi.LunMapInfo(ctx, igroupName, "/vol/lun0")
	assert.Error(t, err, "no error returned while getting a LUN map info")
}

func TestLunUnmap(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	// common call for all subtests
	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// case 1: Positive test, unmap LUN.
	rsi.EXPECT().LunUnmap(ctx, gomock.Any(), gomock.Any()).Return(nil)
	err := oapi.LunUnmap(ctx, "igroup", "/vol/lun0")
	assert.NoError(t, err, "error returned while unmapping a LUN")

	// case 2: Negative test, unmap LUN.
	rsi.EXPECT().LunUnmap(ctx, gomock.Any(), gomock.Any()).Return(errors.New("failed to unmap lun"))
	err = oapi.LunUnmap(ctx, "igroup", "/vol/lun0")
	assert.Error(t, err, "no error returned while unmapping a LUN")
}

func TestLunListIgroupsMapped(t *testing.T) {
	igroupName := "igroup"
	lunName := "lun0"
	logicalUnitNumber := int64(1234)

	oapi, rsi := newMockOntapAPIREST(t)

	lunMap := models.LunMap{
		Igroup:            &models.LunMapInlineIgroup{Name: &igroupName},
		LogicalUnitNumber: &logicalUnitNumber,
		Lun:               &models.LunMapInlineLun{Name: &lunName},
	}
	lunMapResponse := models.LunMapResponse{LunMapResponseInlineRecords: []*models.LunMap{&lunMap}}
	lunMapResponseList := s_a_n.LunMapCollectionGetOK{Payload: &lunMapResponse}

	// case 1: Positive test, get ia LUN list mapped with igroup
	rsi.EXPECT().LunMapList(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(&lunMapResponseList, nil)
	igroupNames, err := oapi.LunListIgroupsMapped(ctx, "/vol/lun0")
	assert.NoError(t, err, "error returned while getting a LUN list mapped with igroup")
	assert.Equal(t, igroupName, igroupNames[0])

	// case 2: Negative test, get ia LUN list mapped with igroup
	rsi.EXPECT().LunMapList(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("failed to get lun map"))
	_, err = oapi.LunListIgroupsMapped(ctx, "/vol/lun0")
	assert.Error(t, err, "no error returned while getting a LUN list mapped with igroup")
}

func TestIgroupListLUNsMapped(t *testing.T) {
	igroupName := "igroup"
	lunName := "lun0"
	logicalUnitNumber := int64(1234)

	oapi, rsi := newMockOntapAPIREST(t)

	lunMap := models.LunMap{
		Igroup:            &models.LunMapInlineIgroup{Name: &igroupName},
		LogicalUnitNumber: &logicalUnitNumber,
		Lun:               &models.LunMapInlineLun{Name: &lunName},
	}
	lunMapResponse := models.LunMapResponse{LunMapResponseInlineRecords: []*models.LunMap{&lunMap}}
	lunMapResponseList := s_a_n.LunMapCollectionGetOK{Payload: &lunMapResponse}

	// case 1: Positive test, get igroup list mapped with LUN.
	rsi.EXPECT().LunMapList(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(&lunMapResponseList, nil)
	lunNames, err := oapi.IgroupListLUNsMapped(ctx, "/vol/lun0")
	assert.NoError(t, err, "error returned while getting a igroup mapped with lun")
	assert.Equal(t, lunName, lunNames[0])

	// case 2: Negative test, get igroup list mapped with LUN.
	rsi.EXPECT().LunMapList(ctx, gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, errors.New("failed to get lun map"))
	_, err = oapi.IgroupListLUNsMapped(ctx, "/vol/lun0")
	assert.Error(t, err, "no error returned while getting a igroup mapped with lun")
}

func TestLunMapGetReportingNodes(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Positive test: get the lun reporting node.
	rsi.EXPECT().LunMapGetReportingNodes(ctx, gomock.Any(), gomock.Any()).Return([]string{"node1", "node2"}, nil)
	nodes, err := oapi.LunMapGetReportingNodes(ctx, "igroup", "/vol/lun0")
	assert.NoError(t, err, "error returned while getting lun map reporting node")
	assert.Equal(t, "node1", nodes[0], "node name does not match")

	// case 2: Negative test: get the lun reporting node.
	rsi.EXPECT().LunMapGetReportingNodes(ctx, gomock.Any(), gomock.Any()).Return(nil,
		errors.New("failed to get lun map node"))
	_, err = oapi.LunMapGetReportingNodes(ctx, "igroup", "/vol/lun0")
	assert.Error(t, err, "no error returned while getting lun map reporting node")
}

func TestLunSize(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Positive test, get the lun size.
	rsi.EXPECT().LunSize(ctx, "/vol/vol1/lun0").Return(2147483648, nil)
	size, err := oapi.LunSize(ctx, "/vol/vol1/lun0")
	assert.NoError(t, err, "error returned while getting a lun size")
	assert.Equal(t, 2147483648, size)

	// case 2: Negative test, get the lun size.
	rsi.EXPECT().LunSize(ctx, "/vol/vol1/lun0").Return(0, errors.New("failed to get size"))
	_, err = oapi.LunSize(ctx, "/vol/vol1/lun0")
	assert.Error(t, err, "no error returned while getting a lun size")
}

func TestLunSetSize(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	// common call for all subtests
	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// case 1: Positive test modify the LUN size
	rsi.EXPECT().LunSetSize(ctx, gomock.Any(), gomock.Any()).Return(uint64(2147483648), nil)
	size, err := oapi.LunSetSize(ctx, "/vol/lun0", "2147483648")
	assert.NoError(t, err, "error returned while modifying a lun size")
	assert.Equal(t, uint64(2147483648), size, "LUN size does not match")

	// case 2: Negative test modify the LUN size
	rsi.EXPECT().LunSetSize(ctx, gomock.Any(), gomock.Any()).Return(uint64(0), errors.New("failed to set size"))
	_, err = oapi.LunSetSize(ctx, "/vol/lun0", "2147483648")
	assert.Error(t, err, "no error returned while modifying a lun size")
}

func TestIscsiInitiatorGetDefaultAuth(t *testing.T) {
	svmName := "fake-svm"
	chapUser := "admin"
	chapPassword := "********"
	initiator := "iqn.1998-01.com.corp.iscsi:name1"
	authType := "chap"

	oapi, rsi := newMockOntapAPIREST(t)

	svm := models.IscsiCredentialsInlineSvm{Name: &svmName}
	inbound := models.IscsiCredentialsInlineChapInlineInbound{User: &chapUser, Password: &chapPassword}
	outbound := models.IscsiCredentialsInlineChapInlineOutbound{User: &chapUser, Password: &chapPassword}
	chap := models.IscsiCredentialsInlineChap{Inbound: &inbound, Outbound: &outbound}
	iscsiCred := models.IscsiCredentials{Chap: &chap, Initiator: &initiator, AuthenticationType: &authType, Svm: &svm}
	iscsiCredResponse := s_a_n.IscsiCredentialsCollectionGetOK{
		Payload: &models.IscsiCredentialsResponse{
			IscsiCredentialsResponseInlineRecords: []*models.IscsiCredentials{&iscsiCred},
		},
	}

	// case 1: Get the iscsi initialor default auth.
	rsi.EXPECT().IscsiInitiatorGetDefaultAuth(ctx, gomock.Any()).Return(&iscsiCredResponse, nil)
	iscsiInitiatorAuth, err := oapi.IscsiInitiatorGetDefaultAuth(ctx)
	assert.NoError(t, err, "error returned while iscsi initiator default auth")
	assert.Equal(t, iscsiInitiatorAuth.AuthType, authType, "authType does not match")

	// case 2: Failed to get the iscsi initialor default auth.
	rsi.EXPECT().IscsiInitiatorGetDefaultAuth(ctx, gomock.Any()).Return(nil, errors.New("failed to get iscsi initiator auth"))
	iscsiInitiatorAuth, err = oapi.IscsiInitiatorGetDefaultAuth(ctx)
	assert.Error(t, err, "no error returned while iscsi initiator default auth")

	// case 3: iSCSI initiator response has too many records
	numRecords := int64(2)
	iscsiCredResponse1 := s_a_n.IscsiCredentialsCollectionGetOK{
		Payload: &models.IscsiCredentialsResponse{
			IscsiCredentialsResponseInlineRecords: []*models.IscsiCredentials{&iscsiCred},
			NumRecords:                            &numRecords,
		},
	}
	rsi.EXPECT().IscsiInitiatorGetDefaultAuth(ctx, gomock.Any()).Return(&iscsiCredResponse1, nil)
	iscsiInitiatorAuth, err = oapi.IscsiInitiatorGetDefaultAuth(ctx)
	assert.Error(t, err, "no error returned while iscsi initiator default auth")

	// case 4: iSCSI initiator response has no records
	numRecords2 := int64(0)
	iscsiCredResponse2 := s_a_n.IscsiCredentialsCollectionGetOK{
		Payload: &models.IscsiCredentialsResponse{
			IscsiCredentialsResponseInlineRecords: []*models.IscsiCredentials{&iscsiCred},
			NumRecords:                            &numRecords2,
		},
	}
	rsi.EXPECT().IscsiInitiatorGetDefaultAuth(ctx, gomock.Any()).Return(&iscsiCredResponse2, nil)
	iscsiInitiatorAuth, err = oapi.IscsiInitiatorGetDefaultAuth(ctx)
	assert.Error(t, err, "no error returned while iscsi initiator default auth")

	// case 5: iSCSI initiator response contain nil IscsiCredentialsResponseInlineRecords
	iscsiCredResponse3 := s_a_n.IscsiCredentialsCollectionGetOK{
		Payload: &models.IscsiCredentialsResponse{
			IscsiCredentialsResponseInlineRecords: nil,
		},
	}
	rsi.EXPECT().IscsiInitiatorGetDefaultAuth(ctx, gomock.Any()).Return(&iscsiCredResponse3, nil)
	iscsiInitiatorAuth, err = oapi.IscsiInitiatorGetDefaultAuth(ctx)
	assert.Error(t, err, "no error returned while iscsi initiator default auth")

	// case 6: iSCSI initiator response contain nil payload
	iscsiCredResponse4 := s_a_n.IscsiCredentialsCollectionGetOK{
		Payload: nil,
	}
	rsi.EXPECT().IscsiInitiatorGetDefaultAuth(ctx, gomock.Any()).Return(&iscsiCredResponse4, nil)
	iscsiInitiatorAuth, err = oapi.IscsiInitiatorGetDefaultAuth(ctx)
	assert.Error(t, err, "no error returned while iscsi initiator default auth")

	// case 7: iSCSI initiator response is nil
	rsi.EXPECT().IscsiInitiatorGetDefaultAuth(ctx, gomock.Any()).Return(nil, nil)
	iscsiInitiatorAuth, err = oapi.IscsiInitiatorGetDefaultAuth(ctx)
	assert.Error(t, err, "no error returned while iscsi initiator default auth")
}

func TestIscsiInitiatorSetDefaultAuth(t *testing.T) {
	chapUser := "admin"
	authType := "chap"

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Get the default iscsi initiator auth
	rsi.EXPECT().IscsiInitiatorSetDefaultAuth(ctx, authType, chapUser, "", chapUser, "").Return(nil)
	err := oapi.IscsiInitiatorSetDefaultAuth(ctx, authType, chapUser, "", chapUser, "")
	assert.NoError(t, err, "error returned while setting the iscsi initiator auth")

	// case 2: Failed to get the default iscsi initiator auth
	rsi.EXPECT().IscsiInitiatorSetDefaultAuth(ctx, authType, chapUser, "", chapUser, "").Return(
		errors.New("failed to get iscsi initiator auth"))
	err = oapi.IscsiInitiatorSetDefaultAuth(ctx, authType, chapUser, "", chapUser, "")
	assert.Error(t, err, "no error returned while setting the iscsi initiator auth")
}

func TestIscsiInterfaceGet(t *testing.T) {
	svmName := "svm1"
	enabled := true
	targetName := "iqn.1992-08.com.netapp:sn.574caf71890911e8a6b7005056b4ea79"

	oapi, rsi := newMockOntapAPIREST(t)

	iscsiService := models.IscsiService{Enabled: &enabled, Target: &models.IscsiServiceInlineTarget{Name: &targetName}}
	iscsiServiceResponse := s_a_n.IscsiServiceCollectionGetOK{
		Payload: &models.IscsiServiceResponse{IscsiServiceResponseInlineRecords: []*models.
			IscsiService{&iscsiService}},
	}

	// case 1: Positive test, get the iscsi interface test.
	rsi.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return(&iscsiServiceResponse, nil)
	iscsiInterface, err := oapi.IscsiInterfaceGet(ctx, svmName)
	assert.NoError(t, err, "error returned while getting iscsi interface")
	assert.Equal(t, iscsiInterface[0], targetName)

	// case 2: Negative test, backend return error in response.
	rsi.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return(nil, errors.New("failed to get iscsi interface service"))
	_, err = oapi.IscsiInterfaceGet(ctx, svmName)
	assert.Error(t, err, "no error returned while getting iscsi interface")

	// case 3: Negative test, backend return nil response.
	rsi.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return(nil, nil)
	_, err = oapi.IscsiInterfaceGet(ctx, svmName)
	assert.NoError(t, err, "error returned while getting iscsi interface")

	// case 4: Negative test, response contain empty payload.
	iscsiServiceResponse = s_a_n.IscsiServiceCollectionGetOK{
		Payload: &models.IscsiServiceResponse{},
	}
	rsi.EXPECT().IscsiInterfaceGet(ctx, gomock.Any()).Return(&iscsiServiceResponse, nil)
	iscsiInterface, err = oapi.IscsiInterfaceGet(ctx, svmName)
	assert.Error(t, err, "no error returned while getting iscsi interface")
}

func TestIscsiNodeGetNameRequest(t *testing.T) {
	enabled := true
	targetName := "iqn.1992-08.com.netapp:sn.574caf71890911e8a6b7005056b4ea79"

	oapi, rsi := newMockOntapAPIREST(t)

	iscsiService := models.IscsiService{Enabled: &enabled, Target: &models.IscsiServiceInlineTarget{Name: &targetName}}
	iscsiServiceResponse := s_a_n.IscsiServiceGetOK{
		Payload: &iscsiService,
	}

	// case 1: Get the iscsi node name.
	rsi.EXPECT().IscsiNodeGetName(ctx, gomock.Any()).Return(&iscsiServiceResponse, nil)
	iscsiInterface, err := oapi.IscsiNodeGetNameRequest(ctx)
	assert.NoError(t, err, "error returned while getting node name")
	assert.Equal(t, targetName, iscsiInterface)

	// case 2: iscsi name is nil in backend response.
	iscsiService = models.IscsiService{Enabled: &enabled, Target: &models.IscsiServiceInlineTarget{Name: nil}}
	iscsiServiceResponse = s_a_n.IscsiServiceGetOK{
		Payload: &iscsiService,
	}
	rsi.EXPECT().IscsiNodeGetName(ctx, gomock.Any()).Return(&iscsiServiceResponse, nil)
	_, err = oapi.IscsiNodeGetNameRequest(ctx)
	assert.Error(t, err, "no error returned while getting node name")

	// case 3: Target field is nil in backend response.
	iscsiService = models.IscsiService{Enabled: &enabled, Target: nil}
	iscsiServiceResponse = s_a_n.IscsiServiceGetOK{
		Payload: &iscsiService,
	}
	rsi.EXPECT().IscsiNodeGetName(ctx, gomock.Any()).Return(&iscsiServiceResponse, nil)
	_, err = oapi.IscsiNodeGetNameRequest(ctx)
	assert.Error(t, err, "no error returned while getting node name")

	// case 4: Payload is nil in backend response.
	iscsiServiceResponse = s_a_n.IscsiServiceGetOK{
		Payload: nil,
	}
	rsi.EXPECT().IscsiNodeGetName(ctx, gomock.Any()).Return(&iscsiServiceResponse, nil)
	_, err = oapi.IscsiNodeGetNameRequest(ctx)
	assert.Error(t, err, "no error returned while getting node name")

	// case 5: backend returned a nil response.
	rsi.EXPECT().IscsiNodeGetName(ctx, gomock.Any()).Return(nil, nil)
	_, err = oapi.IscsiNodeGetNameRequest(ctx)
	assert.Error(t, err, "no error returned while getting node name")

	// case 6: Unable to get the node name from backend.
	rsi.EXPECT().IscsiNodeGetName(ctx, gomock.Any()).Return(nil, errors.New("iscsi node name not found"))
	_, err = oapi.IscsiNodeGetNameRequest(ctx)
	assert.Error(t, err, "no error returned while getting node name")
}

func TestIgroupCreate(t *testing.T) {
	initiator1 := "initiator1"
	initiator2 := "initiator2"
	initiatorGroup := "initiatorGroup"

	oapi, rsi := newMockOntapAPIREST(t)

	igroupName := "igroup1"
	subsysUUID := "fakeUUID"

	igroupInlineInitiatorsResponse := []*models.IgroupInlineInitiatorsInlineArrayItem{
		{
			Name: &initiator1,
		},
		{
			Name: &initiator2,
		},
	}
	igroup := models.Igroup{
		Name:                   &igroupName,
		UUID:                   &subsysUUID,
		IgroupInlineInitiators: igroupInlineInitiatorsResponse,
	}

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	// common call for all subtests
	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// Positive test, igroup created
	rsi.EXPECT().IgroupGetByName(ctx, initiatorGroup, gomock.Any()).Return(&igroup, nil)
	err := oapi.IgroupCreate(ctx, initiatorGroup, initiator1, "Linux")
	assert.NoError(t, err, "error while creating igroup")

	// Negative test, Unoble to verify igroup exists
	rsi.EXPECT().IgroupGetByName(ctx, initiatorGroup, gomock.Any()).Return(nil, errors.New("failed to verify igroup"))
	err = oapi.IgroupCreate(ctx, initiatorGroup, initiator1, "Linux")
	assert.Error(t, err, "no error while verifying igroup")

	// Negative test, Unoble to verify igroup exists
	rsi.EXPECT().IgroupGetByName(ctx, initiatorGroup, gomock.Any()).Return(nil, nil)
	rsi.EXPECT().IgroupCreate(ctx, initiatorGroup, initiator1, "Linux").Return(nil)
	err = oapi.IgroupCreate(ctx, initiatorGroup, initiator1, "Linux")
	assert.NoError(t, err, "error while verifying igroup")

	// Negative test, igroup creation failed.
	rsi.EXPECT().IgroupGetByName(ctx, initiatorGroup, gomock.Any()).Return(nil, nil)
	rsi.EXPECT().IgroupCreate(ctx, initiatorGroup, initiator1, "Linux").Return(
		errors.New("failed to create igroup"))
	err = oapi.IgroupCreate(ctx, initiatorGroup, initiator1, "Linux")
	assert.Error(t, err, "No error while creating igroup")

	// Negative test, igroup creation failed.
	rsi.EXPECT().IgroupGetByName(ctx, initiatorGroup, gomock.Any()).Return(nil, nil)
	rsi.EXPECT().IgroupCreate(ctx, initiatorGroup, initiator1, "Linux").Return(
		errors.New("404 failed to create igroup"))
	err = oapi.IgroupCreate(ctx, initiatorGroup, initiator1, "Linux")
	assert.Error(t, err, "No error while creating igroup")
}

func TestIgroupDestroy(t *testing.T) {
	initiatorGroup := "initiatorGroup"

	oapi, rsi := newMockOntapAPIREST(t)

	clientConfig := api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}

	// common call for all subtests
	rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	// Positive test, igroup destroyed
	rsi.EXPECT().IgroupDestroy(ctx, initiatorGroup).Return(nil)
	err := oapi.IgroupDestroy(ctx, initiatorGroup)
	assert.NoError(t, err, "error while deleting igroup")

	// Negative test, Unable to delete igroup
	rsi.EXPECT().IgroupDestroy(ctx, initiatorGroup).Return(errors.New("Unable to delete igroup"))
	err = oapi.IgroupDestroy(ctx, initiatorGroup)
	assert.Error(t, err, "No error while deleting igroup")

	// Negative test, igroup is not found. No error returned to user
	rsi.EXPECT().IgroupDestroy(ctx, initiatorGroup).Return(api.NotFoundError("failed to get igroup"))
	err = oapi.IgroupDestroy(ctx, initiatorGroup)
	assert.NoError(t, err, "error while deleting igroup")
}

func TestTerminalStateError(t *testing.T) {
	terminalStateError := api.TerminalState(errors.New("error in getting terminal state"))

	assert.Error(t, terminalStateError)
	assert.Equal(t, "error in getting terminal state", terminalStateError.Error())
}

func TestConsistencyGroupSnapshot(t *testing.T) {
	// volume := getVolumeInfo()
	// volumeUUID := *volume.UUID
	oapi, rsi := newMockOntapAPIREST(t)

	snapshotName := fmt.Sprintf("snapshot-%s", uuid.New().String())
	cgName := fmt.Sprintf("cg-%s", snapshotName)[:30]

	volumes := []string{"vol1", "vol2"}

	deleteErr := errors.New("error deleting cg")
	snapshotErr := errors.New("error creating cg snapshot")
	createErr := errors.New("error creating cg")

	// case 1: CG snapshot create success
	rsi.EXPECT().ConsistencyGroupCreateAndWait(ctx, cgName, volumes).Return(nil).Times(1)
	rsi.EXPECT().ConsistencyGroupSnapshotAndWait(ctx, cgName, snapshotName).Return(nil).Times(1)
	rsi.EXPECT().ConsistencyGroupDelete(ctx, cgName).Return(nil).Times(1)
	err := oapi.ConsistencyGroupSnapshot(ctx, snapshotName, volumes)
	assert.NoError(t, err)

	// case 2: CG snapshot create success, but cg delete failed once
	rsi.EXPECT().ConsistencyGroupCreateAndWait(ctx, cgName, volumes).Return(nil).Times(1)
	rsi.EXPECT().ConsistencyGroupSnapshotAndWait(ctx, cgName, snapshotName).Return(nil).Times(1)
	rsi.EXPECT().ConsistencyGroupDelete(ctx, cgName).Return(deleteErr).Times(1)
	rsi.EXPECT().ConsistencyGroupDelete(ctx, cgName).Return(nil).Times(1)
	err = oapi.ConsistencyGroupSnapshot(ctx, snapshotName, volumes)
	// return deleteErr
	assert.Error(t, err)
	assert.Equal(t, deleteErr, err)

	// case 3: CG create success, but snapshot fails
	rsi.EXPECT().ConsistencyGroupCreateAndWait(ctx, cgName, volumes).Return(nil).Times(1)
	rsi.EXPECT().ConsistencyGroupSnapshotAndWait(ctx, cgName, snapshotName).Return(snapshotErr).Times(1)
	rsi.EXPECT().ConsistencyGroupDelete(ctx, cgName).Return(nil).Times(1)
	err = oapi.ConsistencyGroupSnapshot(ctx, snapshotName, volumes)
	// return snapshotErr
	assert.Error(t, err)
	assert.Equal(t, snapshotErr, err)

	// case 4: CG create fails
	rsi.EXPECT().ConsistencyGroupCreateAndWait(ctx, cgName, volumes).Return(createErr).Times(1)
	rsi.EXPECT().ConsistencyGroupDelete(ctx, cgName).Return(nil).Times(1)
	err = oapi.ConsistencyGroupSnapshot(ctx, snapshotName, volumes)
	// return createErr
	assert.Error(t, err)
	assert.Equal(t, createErr, err)
}

func TestNVMeNamespaceSetComment(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oapi, rsi := newMockOntapAPIREST(t)

	ctx := context.Background()
	namespaceName := "testNamespace"
	comment := "testComment"
	namespaceUUID := "testUUID"

	// Mock the ClientConfig method
	rsi.EXPECT().ClientConfig().Return(api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}).AnyTimes()

	fields := []string{
		"os_type",
		"location.volume.name",
		"space.size",
		"space.block_size",
		"status.state",
		"comment",
	}

	// Case 1: Successfully set comment
	mockNamespace := &models.NvmeNamespace{UUID: &namespaceUUID}
	rsi.EXPECT().NVMeNamespaceGetByName(ctx, namespaceName, fields).Return(mockNamespace, nil)
	rsi.EXPECT().NVMeNamespaceSetComment(ctx, namespaceUUID, comment).Return(nil)

	err := oapi.NVMeNamespaceSetComment(ctx, namespaceName, comment)
	assert.NoError(t, err, "unexpected error while setting namespace comment")

	// Case 2: Namespace response nil
	rsi.EXPECT().NVMeNamespaceGetByName(ctx, namespaceName, fields).Return(nil, nil)

	err = oapi.NVMeNamespaceSetComment(ctx, namespaceName, comment)
	assert.Error(t, err, "expected an error when namespace is not found")
	assert.Contains(t, err.Error(), "namespace response is nil")

	// Case 3: Error while retrieving namespace
	rsi.EXPECT().NVMeNamespaceGetByName(ctx, namespaceName, fields).Return(nil, errors.New("mock error"))

	err = oapi.NVMeNamespaceSetComment(ctx, namespaceName, comment)
	assert.Error(t, err, "expected an error while retrieving namespace")
	assert.Contains(t, err.Error(), "mock error")
}

func TestNVMeNamespaceSetQosPolicyGroup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oapi, rsi := newMockOntapAPIREST(t)

	ctx := context.Background()
	namespaceName := "testNamespace"
	qosPolicyGroup := api.QosPolicyGroup{
		Name: "testQoSGroup",
		Kind: api.QosPolicyGroupKind,
	}
	namespaceUUID := "testUUID"

	fields := []string{
		"os_type",
		"location.volume.name",
		"space.size",
		"space.block_size",
		"status.state",
		"comment",
	}

	// Mock the ClientConfig method
	rsi.EXPECT().ClientConfig().Return(api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}).AnyTimes()

	// Case 1: Successfully set QoS policy group
	mockNamespace := &models.NvmeNamespace{UUID: &namespaceUUID}
	rsi.EXPECT().NVMeNamespaceGetByName(ctx, namespaceName, fields).Return(mockNamespace, nil)
	rsi.EXPECT().NVMeNamespaceSetQosPolicyGroup(ctx, namespaceUUID, qosPolicyGroup).Return(nil)

	err := oapi.NVMeNamespaceSetQosPolicyGroup(ctx, namespaceName, qosPolicyGroup)
	assert.NoError(t, err, "unexpected error while setting QoS policy group")

	// Case 2: Namespace not found
	rsi.EXPECT().NVMeNamespaceGetByName(ctx, namespaceName, fields).Return(nil, nil)

	err = oapi.NVMeNamespaceSetQosPolicyGroup(ctx, namespaceName, qosPolicyGroup)
	assert.Error(t, err, "expected an error when namespace is not found")
	assert.Contains(t, err.Error(), "namespace response is nil")

	// Case 3: Error while retrieving namespace
	rsi.EXPECT().NVMeNamespaceGetByName(ctx, namespaceName, fields).Return(nil, errors.New("mock error"))

	err = oapi.NVMeNamespaceSetQosPolicyGroup(ctx, namespaceName, qosPolicyGroup)
	assert.Error(t, err, "expected an error while retrieving namespace")
	assert.Contains(t, err.Error(), "mock error")
}

func TestStorageUnitExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oapi, rsi := newMockOntapAPIREST(t)

	ctx := context.Background()
	suName := "testStorageUnit"

	// Case 1: Storage unit exists
	mockStorageUnit := &models.StorageUnit{Name: &suName}
	rsi.EXPECT().StorageUnitGetByName(ctx, suName).Return(mockStorageUnit, nil)

	exists, err := oapi.StorageUnitExists(ctx, suName)
	assert.NoError(t, err, "unexpected error")
	assert.True(t, exists, "expected storage unit to exist")

	// Case 2: Storage unit does not exist
	rsi.EXPECT().StorageUnitGetByName(ctx, suName).Return(nil, api.NotFoundError("not found"))

	exists, err = oapi.StorageUnitExists(ctx, suName)
	assert.NoError(t, err, "unexpected error")
	assert.False(t, exists, "expected storage unit to not exist")

	// Case 3: Error while fetching storage unit
	rsi.EXPECT().StorageUnitGetByName(ctx, suName).Return(nil, errors.New("some error"))

	exists, err = oapi.StorageUnitExists(ctx, suName)
	assert.Error(t, err, "expected an error")
	assert.False(t, exists, "expected storage unit to not exist")
}

func TestStorageUnitGetByName(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oapi, rsi := newMockOntapAPIREST(t)

	ctx := context.Background()
	suName := "testStorageUnit"

	// Case 1: Storage unit is found
	expectedStorageUnit := &models.StorageUnit{Name: &suName}
	rsi.EXPECT().StorageUnitGetByName(ctx, suName).Return(expectedStorageUnit, nil)

	storageUnit, err := oapi.StorageUnitGetByName(ctx, suName)
	assert.NoError(t, err, "unexpected error")
	assert.NotNil(t, storageUnit, "expected storage unit to be found")
	assert.Equal(t, suName, *storageUnit.Name, "storage unit name mismatch")

	// Case 2: Storage unit is not found
	rsi.EXPECT().StorageUnitGetByName(ctx, suName).Return(nil, api.NotFoundError("not found"))

	storageUnit, err = oapi.StorageUnitGetByName(ctx, suName)
	assert.Error(t, err, "expected an error")
	assert.Nil(t, storageUnit, "expected no storage unit to be found")

	// Case 3: Error while fetching storage unit
	rsi.EXPECT().StorageUnitGetByName(ctx, suName).Return(nil, errors.New("some error"))

	storageUnit, err = oapi.StorageUnitGetByName(ctx, suName)
	assert.Error(t, err, "expected an error")
	assert.Nil(t, storageUnit, "expected no storage unit to be found")
}

func TestStorageUnitSnapshotCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	oapi, rsi := newMockOntapAPIREST(t)

	ctx := context.Background()
	snapshotName := "testSnapshot"
	suName := "testStorageUnit"
	suUUID := "testUUID"
	mockSU := &models.StorageUnit{
		UUID: convert.ToPtr(suUUID),
		Space: &models.StorageUnitInlineSpace{
			Size: convert.ToPtr(int64(100)),
		},
	}

	// Mock the ClientConfig method
	rsi.EXPECT().ClientConfig().Return(api.ClientConfig{
		DebugTraceFlags: map[string]bool{"method": true},
	}).AnyTimes()

	// Case 1: Successfully create a snapshot
	rsi.EXPECT().StorageUnitGetByName(ctx, suName).Return(mockSU, nil).Times(1)
	rsi.EXPECT().StorageUnitSnapshotCreateAndWait(ctx, suUUID, snapshotName).Return(nil)

	err := oapi.StorageUnitSnapshotCreate(ctx, snapshotName, suName)
	assert.NoError(t, err, "unexpected error while creating snapshot")

	// Case 2: Error while getting storage unit UUID
	rsi.EXPECT().StorageUnitGetByName(ctx, suName).Return(nil, errors.New("mock error")).Times(1)

	err = oapi.StorageUnitSnapshotCreate(ctx, snapshotName, suName)
	assert.Error(t, err, "expected an error while getting storage unit UUID")

	// Case 3: Error while creating snapshot
	rsi.EXPECT().StorageUnitGetByName(ctx, suName).Return(mockSU, nil).Times(1)
	rsi.EXPECT().StorageUnitSnapshotCreateAndWait(ctx, suUUID, snapshotName).Return(errors.New("failed to create snapshot"))

	err = oapi.StorageUnitSnapshotCreate(ctx, snapshotName, suName)
	assert.Error(t, err, "expected an error while creating snapshot")
}

func TestAPIVersion(t *testing.T) {
	oapi, rsi, ctrl := newMockOntapAPIRESTWithController(t)
	defer ctrl.Finish()

	tests := []struct {
		name        string
		cached      bool
		expected    string
		mockErr     error
		expectError bool
	}{
		{"Cached version success", true, "9.12.1", nil, false},
		{"Fresh version success", false, "9.12.1", nil, false},
		{"API error", false, "", fmt.Errorf("API call failed"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectError {
				rsi.EXPECT().SystemGetOntapVersion(ctx, tt.cached).Return("", tt.mockErr)
			} else {
				rsi.EXPECT().SystemGetOntapVersion(ctx, tt.cached).Return(tt.expected, nil)
			}

			version, err := oapi.APIVersion(ctx, tt.cached)

			if tt.expectError {
				assert.Error(t, err, "expected error when getting API version")
				assert.Equal(t, "", version)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, version)
			}
		})
	}
}

func TestValidateAPIVersion(t *testing.T) {
	oapi, rsi, ctrl := newMockOntapAPIRESTWithController(t)
	defer ctrl.Finish()

	tests := []struct {
		name        string
		version     string
		expectError bool
	}{
		{"Valid version", "9.12.1", false},
		{"Valid version 9.13", "9.13.0", false},
		{"Invalid version", "invalid", true},
		{"Too old version", "9.5.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsi.EXPECT().SystemGetOntapVersion(ctx, true).Return(tt.version, nil)
			err := oapi.ValidateAPIVersion(ctx)

			if tt.expectError {
				assert.Error(t, err, "expected error when validating API version")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsSANOptimized(t *testing.T) {
	oapi, rsi, ctrl := newMockOntapAPIRESTWithController(t)
	defer ctrl.Finish()

	tests := []struct {
		name     string
		expected bool
	}{
		{"SAN optimized true", true},
		{"SAN optimized false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsi.EXPECT().IsSANOptimized().Return(tt.expected)
			result := oapi.IsSANOptimized()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsDisaggregated(t *testing.T) {
	oapi, rsi, ctrl := newMockOntapAPIRESTWithController(t)
	defer ctrl.Finish()

	tests := []struct {
		name     string
		expected bool
	}{
		{"Disaggregated true", true},
		{"Disaggregated false", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsi.EXPECT().IsDisaggregated().Return(tt.expected)
			result := oapi.IsDisaggregated()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestVolumeRecoveryQueuePurge(t *testing.T) {
	oapi, rsi, ctrl := newMockOntapAPIRESTWithController(t)
	defer ctrl.Finish()

	tests := []struct {
		name        string
		mockErr     error
		expectError bool
	}{
		{"Success", nil, false},
		{"API error", fmt.Errorf("API call failed"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsi.EXPECT().VolumeRecoveryQueuePurge(ctx, "test_volume").Return(tt.mockErr)
			err := oapi.VolumeRecoveryQueuePurge(ctx, "test_volume")

			if tt.expectError {
				assert.Error(t, err, "expected error when purging volume recovery queue")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestVolumeRecoveryQueueGetName(t *testing.T) {
	oapi, rsi, ctrl := newMockOntapAPIRESTWithController(t)
	defer ctrl.Finish()

	tests := []struct {
		name        string
		expected    string
		mockErr     error
		expectError bool
	}{
		{"Success", "recovery_queue_name", nil, false},
		{"Empty queue", "", nil, false},
		{"API error", "", fmt.Errorf("API call failed"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsi.EXPECT().VolumeRecoveryQueueGetName(ctx, "test_volume").Return(tt.expected, tt.mockErr)
			name, err := oapi.VolumeRecoveryQueueGetName(ctx, "test_volume")

			if tt.expectError {
				assert.Error(t, err, "expected error when getting volume recovery queue name")
				assert.Equal(t, "", name)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, name)
			}
		})
	}
}

func TestNetFcpInterfaceGetDataLIFs(t *testing.T) {
	oapi, rsi, ctrl := newMockOntapAPIRESTWithController(t)
	defer ctrl.Finish()

	tests := []struct {
		name        string
		protocol    string
		expected    []string
		mockErr     error
		expectError bool
	}{
		{"FCP protocol success", "fcp", []string{"fcp_lif1", "fcp_lif2"}, nil, false},
		{"Empty result", "fcp", []string{}, nil, false},
		{"API error", "fcp", nil, fmt.Errorf("API call failed"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rsi.EXPECT().NetFcpInterfaceGetDataLIFs(ctx, tt.protocol).Return(tt.expected, tt.mockErr)

			lifs, err := oapi.NetFcpInterfaceGetDataLIFs(ctx, tt.protocol)

			if tt.expectError {
				assert.Error(t, err, "expected error when getting FCP data LIFs")
				assert.Nil(t, lifs)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, lifs)
			}
		})
	}
}

func TestRestError_Methods(t *testing.T) {
	tests := []struct {
		name     string
		payload  *models.Job
		hasError bool
	}{
		{
			name: "Valid error payload",
			payload: &models.Job{
				Code:        convert.ToPtr(int64(12345)),
				Description: convert.ToPtr("Test error message"),
				State:       convert.ToPtr("failure"),
			},
			hasError: true,
		},
		{
			name:     "Nil payload",
			payload:  nil,
			hasError: false,
		},
		{
			name: "Success payload",
			payload: &models.Job{
				State: convert.ToPtr("success"),
			},
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restErr := api.NewRestErrorFromPayload(tt.payload)

			// Test IsSuccess and IsFailure
			success := restErr.IsSuccess()
			failure := restErr.IsFailure()

			if tt.hasError {
				assert.False(t, success)
				assert.True(t, failure)
			} else if tt.payload != nil {
				assert.True(t, success)
				assert.False(t, failure)
			} else {
				assert.False(t, success)
				assert.False(t, failure)
			}

			// Test other methods for coverage
			errorMsg := restErr.Error()
			assert.IsType(t, "", errorMsg, "Error() should return a string")

			isSnapshotBusy := restErr.IsSnapshotBusy()
			assert.IsType(t, false, isSnapshotBusy, "IsSnapshotBusy() should return a boolean")

			state := restErr.State()
			assert.IsType(t, "", state, "State() should return a string")

			message := restErr.Message()
			assert.IsType(t, "", message, "Message() should return a string")

			code := restErr.Code()
			assert.IsType(t, "", code, "Code() should return a string")
		})
	}
}

func TestVolumeSnapshotInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(rsi)
	assert.NoError(t, err)

	tests := []struct {
		name           string
		snapshotName   string
		sourceVolume   string
		mockVolumeInfo *models.Volume
		mockVolumeErr  error
		mockSnapshots  []*models.Snapshot
		mockSnapErr    error
		expectError    bool
		expectedName   string
	}{
		{
			name:         "Successful snapshot info retrieval",
			snapshotName: "snap1",
			sourceVolume: "vol1",
			mockVolumeInfo: &models.Volume{
				Name: convert.ToPtr("vol1"),
				UUID: convert.ToPtr("volume-uuid-123"),
			},
			mockVolumeErr: nil,
			mockSnapshots: []*models.Snapshot{
				{
					Name:       convert.ToPtr("snap1"),
					CreateTime: &strfmt.DateTime{},
					UUID:       convert.ToPtr("snap-uuid-123"),
				},
			},
			mockSnapErr:  nil,
			expectError:  false,
			expectedName: "snap1",
		},
		{
			name:           "Volume info lookup error",
			snapshotName:   "snap1",
			sourceVolume:   "vol1",
			mockVolumeInfo: nil,
			mockVolumeErr:  fmt.Errorf("volume not found"),
			expectError:    true,
		},
		{
			name:           "Volume info returns nil",
			snapshotName:   "snap1",
			sourceVolume:   "vol1",
			mockVolumeInfo: nil,
			mockVolumeErr:  nil,
			expectError:    true,
		},
		{
			name:         "Snapshot not found",
			snapshotName: "snap1",
			sourceVolume: "vol1",
			mockVolumeInfo: &models.Volume{
				Name: convert.ToPtr("vol1"),
				UUID: convert.ToPtr("volume-uuid-123"),
			},
			mockVolumeErr: nil,
			mockSnapshots: []*models.Snapshot{},
			mockSnapErr:   nil,
			expectError:   true,
		},
		{
			name:         "Multiple snapshots found error",
			snapshotName: "snap1",
			sourceVolume: "vol1",
			mockVolumeInfo: &models.Volume{
				Name: convert.ToPtr("vol1"),
				UUID: convert.ToPtr("volume-uuid-123"),
			},
			mockVolumeErr: nil,
			mockSnapshots: []*models.Snapshot{
				{
					Name:       convert.ToPtr("snap1"),
					CreateTime: &strfmt.DateTime{},
					UUID:       convert.ToPtr("snap-uuid-123"),
				},
				{
					Name:       convert.ToPtr("snap1"),
					CreateTime: &strfmt.DateTime{},
					UUID:       convert.ToPtr("snap-uuid-456"),
				},
			},
			mockSnapErr: nil,
			expectError: true,
		},
		{
			name:         "Snapshot with nil fields",
			snapshotName: "snap1",
			sourceVolume: "vol1",
			mockVolumeInfo: &models.Volume{
				Name: convert.ToPtr("vol1"),
				UUID: convert.ToPtr("volume-uuid-123"),
			},
			mockVolumeErr: nil,
			mockSnapshots: []*models.Snapshot{
				{
					Name:       nil, // Nil name should cause error
					CreateTime: &strfmt.DateTime{},
					UUID:       convert.ToPtr("snap-uuid-123"),
				},
			},
			mockSnapErr: nil,
			expectError: true,
		},
		{
			name:         "Snapshot API error",
			snapshotName: "snap1",
			sourceVolume: "vol1",
			mockVolumeInfo: &models.Volume{
				Name: convert.ToPtr("vol1"),
				UUID: convert.ToPtr("volume-uuid-123"),
			},
			mockVolumeErr: nil,
			mockSnapErr:   fmt.Errorf("API call failed"),
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Mock VolumeGetByName call - this is what VolumeInfo calls internally
			// VolumeInfo calls with specific fields
			expectedFields := []string{
				"type", "size", "comment", "aggregates", "nas", "guarantee",
				"snapshot_policy", "snapshot_directory_access_enabled",
				"space.snapshot.used", "space.snapshot.reserve_percent",
				"nas.export_policy.name",
			}
			rsi.EXPECT().VolumeGetByName(ctx, tt.sourceVolume, expectedFields).Return(tt.mockVolumeInfo, tt.mockVolumeErr)

			// Mock SnapshotListByName call only if volume lookup succeeds and has valid UUID
			if tt.mockVolumeInfo != nil && tt.mockVolumeErr == nil && tt.mockVolumeInfo.UUID != nil {
				var snapResponse *models.SnapshotResponse
				if tt.mockSnapshots != nil {
					snapResponse = &models.SnapshotResponse{
						SnapshotResponseInlineRecords: tt.mockSnapshots,
					}
				}
				rsi.EXPECT().SnapshotListByName(ctx, *tt.mockVolumeInfo.UUID, tt.snapshotName).Return(
					&storage.SnapshotCollectionGetOK{Payload: snapResponse}, tt.mockSnapErr)
			}

			result, err := oapi.VolumeSnapshotInfo(ctx, tt.snapshotName, tt.sourceVolume)

			if tt.expectError {
				assert.Error(t, err, "expected error when getting volume snapshot info")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedName, result.Name)
			}
		})
	}
}

func TestFcpInterfaceGet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	rsi := mockapi.NewMockRestClientInterface(ctrl)
	oapi, err := api.NewOntapAPIRESTFromRestClientInterface(rsi)
	assert.NoError(t, err)

	tests := []struct {
		name             string
		svm              string
		mockInterfaces   []*models.FcpService
		mockErr          error
		expectedNames    []string
		expectError      bool
		expectedErrorMsg string
	}{
		{
			name: "Multiple enabled FCP interfaces",
			svm:  "svm1",
			mockInterfaces: []*models.FcpService{
				{
					Enabled: convert.ToPtr(true),
					Target: &models.FcpServiceInlineTarget{
						Name: convert.ToPtr("20:00:00:50:56:bb:bb:01"),
					},
				},
				{
					Enabled: convert.ToPtr(true),
					Target: &models.FcpServiceInlineTarget{
						Name: convert.ToPtr("20:00:00:50:56:bb:bb:02"),
					},
				},
				{
					Enabled: convert.ToPtr(false), // Should be filtered out
					Target: &models.FcpServiceInlineTarget{
						Name: convert.ToPtr("20:00:00:50:56:bb:bb:03"),
					},
				},
			},
			mockErr:       nil,
			expectedNames: []string{"20:00:00:50:56:bb:bb:01", "20:00:00:50:56:bb:bb:02"},
			expectError:   false,
		},
		{
			name: "No enabled FCP interfaces",
			svm:  "svm1",
			mockInterfaces: []*models.FcpService{
				{
					Enabled: convert.ToPtr(false),
					Target: &models.FcpServiceInlineTarget{
						Name: convert.ToPtr("20:00:00:50:56:bb:bb:01"),
					},
				},
			},
			mockErr:          nil,
			expectError:      true,
			expectedErrorMsg: "SVM svm1 has no active FCP interfaces",
		},
		{
			name:             "Empty interface list",
			svm:              "svm1",
			mockInterfaces:   []*models.FcpService{},
			mockErr:          nil,
			expectError:      true,
			expectedErrorMsg: "SVM svm1 has no active FCP interfaces",
		},
		{
			name:        "API call error",
			svm:         "svm1",
			mockErr:     fmt.Errorf("API call failed"),
			expectError: true,
		},
		{
			name: "Interface with nil target",
			svm:  "svm1",
			mockInterfaces: []*models.FcpService{
				{
					Enabled: convert.ToPtr(true),
					Target:  nil, // Should be handled gracefully
				},
			},
			mockErr:          nil,
			expectError:      true,
			expectedErrorMsg: "SVM svm1 has no active FCP interfaces",
		},
		{
			name: "Interface with nil name",
			svm:  "svm1",
			mockInterfaces: []*models.FcpService{
				{
					Enabled: convert.ToPtr(true),
					Target: &models.FcpServiceInlineTarget{
						Name: nil, // Should be handled gracefully
					},
				},
			},
			mockErr:          nil,
			expectError:      true,
			expectedErrorMsg: "SVM svm1 has no active FCP interfaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response *s_a_n.FcpServiceCollectionGetOK
			if tt.mockInterfaces != nil {
				response = &s_a_n.FcpServiceCollectionGetOK{
					Payload: &models.FcpServiceResponse{
						FcpServiceResponseInlineRecords: tt.mockInterfaces,
					},
				}
			}

			rsi.EXPECT().FcpInterfaceGet(ctx, []string{"target.name", "enabled"}).Return(response, tt.mockErr)

			result, err := oapi.FcpInterfaceGet(ctx, tt.svm)

			if tt.expectError {
				assert.Error(t, err, "expected error when getting FCP interface")
				if tt.expectedErrorMsg != "" {
					assert.Contains(t, err.Error(), tt.expectedErrorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedNames, result)
			}
		})
	}
}

func TestFlexVolInfoFromRestAttrsHelper(t *testing.T) {
	tests := []struct {
		name        string
		volume      models.Volume
		expected    *api.Volume
		expectError bool
	}{
		{
			name: "Complete volume information",
			volume: models.Volume{
				Name: convert.ToPtr("test_volume"),
				UUID: convert.ToPtr("volume-uuid-123"),
				Size: convert.ToPtr(int64(1073741824)),
				Space: &models.VolumeInlineSpace{
					Used: convert.ToPtr(int64(536870912)),
				},
				State: convert.ToPtr("online"),
				Type:  convert.ToPtr("rw"),
				Style: convert.ToPtr("flexvol"),
				Nas: &models.VolumeInlineNas{
					Path:            convert.ToPtr("/test_volume"),
					SecurityStyle:   convert.ToPtr("unix"),
					UnixPermissions: convert.ToPtr(int64(755)),
				},
				Comment: convert.ToPtr("Test volume comment"),
			},
			expected: &api.Volume{
				Name:           "test_volume",
				Aggregates:     []string{},
				Encrypt:        convert.ToPtr(false),
				TieringPolicy:  "none",
				SnapshotDir:    convert.ToPtr(false),
				SpaceReserve:   "",
				SnapshotPolicy: "",
			},
			expectError: false,
		},
		{
			name: "Volume with minimal information",
			volume: models.Volume{
				Name: convert.ToPtr("minimal_volume"),
				UUID: convert.ToPtr("volume-uuid-456"),
			},
			expected: &api.Volume{
				Name:           "minimal_volume",
				Aggregates:     []string{},
				Encrypt:        convert.ToPtr(false),
				TieringPolicy:  "none",
				SnapshotDir:    convert.ToPtr(false),
				SpaceReserve:   "",
				SnapshotPolicy: "",
			},
			expectError: false,
		},
		{
			name: "Volume with nil name",
			volume: models.Volume{
				Name: nil,
				UUID: convert.ToPtr("volume-uuid-789"),
			},
			expected: &api.Volume{
				Name:           "", // Name will be empty when nil
				Aggregates:     []string{},
				Encrypt:        convert.ToPtr(false),
				TieringPolicy:  "none",
				SnapshotDir:    convert.ToPtr(false),
				SpaceReserve:   "",
				SnapshotPolicy: "",
			},
			expectError: false,
		},
		{
			name: "Volume with nil UUID",
			volume: models.Volume{
				Name: convert.ToPtr("test_volume"),
				UUID: nil,
			},
			expected: &api.Volume{
				Name:           "test_volume",
				Aggregates:     []string{},
				Encrypt:        convert.ToPtr(false),
				TieringPolicy:  "none",
				SnapshotDir:    convert.ToPtr(false),
				SpaceReserve:   "",
				SnapshotPolicy: "",
			},
			expectError: false,
		},
		{
			name: "Volume with space information",
			volume: models.Volume{
				Name: convert.ToPtr("space_volume"),
				UUID: convert.ToPtr("volume-uuid-space"),
				Size: convert.ToPtr(int64(2147483648)),
				Space: &models.VolumeInlineSpace{
					Used:      convert.ToPtr(int64(1073741824)),
					Available: convert.ToPtr(int64(1073741824)),
				},
			},
			expected: &api.Volume{
				Name:           "space_volume",
				Aggregates:     []string{},
				Encrypt:        convert.ToPtr(false),
				TieringPolicy:  "none",
				SnapshotDir:    convert.ToPtr(false),
				SpaceReserve:   "",
				SnapshotPolicy: "",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := api.FlexVolInfoFromRestAttrsHelper(&tt.volume)

			if tt.expectError {
				assert.Error(t, err, "expected error when converting REST volume attributes to FlexVol info")
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expected.Name, result.Name)
				assert.Equal(t, len(tt.expected.Aggregates), len(result.Aggregates))
				assert.Equal(t, *tt.expected.Encrypt, *result.Encrypt)
				assert.Equal(t, tt.expected.TieringPolicy, result.TieringPolicy)
				assert.Equal(t, *tt.expected.SnapshotDir, *result.SnapshotDir)
				assert.Equal(t, tt.expected.SpaceReserve, result.SpaceReserve)
				assert.Equal(t, tt.expected.SnapshotPolicy, result.SnapshotPolicy)
			}
		})
	}
}

func TestVolumeListByAttrs_Additional(t *testing.T) {
	tests := []struct {
		name          string
		volumeAttrs   *api.Volume
		mockResponse  *storage.VolumeCollectionGetOK
		mockError     error
		expectError   bool
		expectedCount int
	}{
		{
			name: "Nil response handling with named volume",
			volumeAttrs: &api.Volume{
				Name: "test-volume",
			},
			mockResponse:  &storage.VolumeCollectionGetOK{}, // Empty but not nil to avoid panic
			mockError:     nil,
			expectError:   false,
			expectedCount: 0,
		},
		{
			name: "Nil response handling with unnamed volume",
			volumeAttrs: &api.Volume{
				Name: "",
			},
			mockResponse:  &storage.VolumeCollectionGetOK{}, // Empty but not nil to avoid panic
			mockError:     nil,
			expectError:   false,
			expectedCount: 0,
		},
		{
			name: "API error handling",
			volumeAttrs: &api.Volume{
				Name: "test-volume",
			},
			mockResponse: nil,
			mockError:    errors.New("API connection failed"),
			expectError:  true,
		},
		{
			name: "FlexVolInfoFromRestAttrsHelper error propagation",
			volumeAttrs: &api.Volume{
				Name: "test-volume",
			},
			mockResponse: &storage.VolumeCollectionGetOK{
				Payload: &models.VolumeResponse{
					VolumeResponseInlineRecords: []*models.Volume{
						{
							Name: convert.ToPtr("test-volume"), // Valid name to avoid error
							UUID: convert.ToPtr("test-uuid"),
						},
					},
				},
			},
			mockError:     nil,
			expectError:   false,
			expectedCount: 1,
		},
		{
			name: "Empty payload records",
			volumeAttrs: &api.Volume{
				Name: "test-volume",
			},
			mockResponse: &storage.VolumeCollectionGetOK{
				Payload: &models.VolumeResponse{
					VolumeResponseInlineRecords: []*models.Volume{},
				},
			},
			mockError:     nil,
			expectError:   false,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oapi, rsi := newMockOntapAPIREST(t)

			expectedFields := []string{
				"aggregates",
				"snapshot_directory_access_enabled",
				"encryption.enabled",
				"guarantee.type",
				"snapshot_policy.name",
			}
			rsi.EXPECT().VolumeListByAttrs(ctx, tt.volumeAttrs, expectedFields).Return(tt.mockResponse, tt.mockError)

			result, err := oapi.VolumeListByAttrs(ctx, tt.volumeAttrs)

			if tt.expectError {
				assert.Error(t, err, "expected error when listing volumes by attributes")
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.expectedCount)
			}
		})
	}
}

func TestExportRuleCreate_Additional(t *testing.T) {
	tests := []struct {
		name               string
		policyName         string
		desiredPolicyRules string
		nasProtocol        string
		mockResponse       *nas.ExportRuleCreateCreated
		mockError          error
		expectError        bool
		expectedErrorType  string
	}{
		{
			name:               "Multiple comma-separated rules",
			policyName:         "test-policy",
			desiredPolicyRules: "10.0.0.1,10.0.0.2,10.0.0.3",
			nasProtocol:        sa.NFS,
			mockResponse:       &nas.ExportRuleCreateCreated{},
			mockError:          nil,
			expectError:        false,
		},
		{
			name:               "Export rule already exists error",
			policyName:         "test-policy",
			desiredPolicyRules: "10.0.0.1",
			nasProtocol:        sa.NFS,
			mockResponse:       nil,
			mockError:          errors.New("export rule already exists"),
			expectError:        true,
			expectedErrorType:  "AlreadyExistsError",
		},
		{
			name:               "Generic API error during rule creation",
			policyName:         "test-policy",
			desiredPolicyRules: "10.0.0.1",
			nasProtocol:        sa.SMB,
			mockResponse:       nil,
			mockError:          errors.New("generic API error"),
			expectError:        true,
		},
		{
			name:               "Error extraction failure fallback",
			policyName:         "test-policy",
			desiredPolicyRules: "10.0.0.1",
			nasProtocol:        sa.NFS,
			mockResponse:       nil,
			mockError:          errors.New("malformed error that can't be extracted"),
			expectError:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oapi, rsi := newMockOntapAPIREST(t)

			clientConfig := api.ClientConfig{
				DebugTraceFlags: map[string]bool{"method": true},
			}
			rsi.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

			// Set up expectations for each rule in comma-separated list
			rules := strings.Split(tt.desiredPolicyRules, ",")
			for _, rule := range rules {
				var expectedProtocol []string
				if tt.nasProtocol == sa.SMB {
					expectedProtocol = []string{"cifs"}
				} else {
					expectedProtocol = []string{"nfs"}
				}

				rsi.EXPECT().ExportRuleCreate(ctx, tt.policyName, rule, expectedProtocol,
					[]string{"any"}, []string{"any"}, []string{"any"}).Return(tt.mockResponse, tt.mockError)

				// If we expect an error, it will stop at the first rule
				if tt.expectError {
					break
				}
			}

			err := oapi.ExportRuleCreate(ctx, tt.policyName, tt.desiredPolicyRules, tt.nasProtocol)

			if tt.expectError {
				assert.Error(t, err, "expected error when creating export rule")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
