// Copyright 2022 NetApp, Inc. All Rights Reserved.

package api_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	nas "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/n_a_s"
	nvme "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/n_v_me"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/networking"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/s_a_n"
	"github.com/netapp/trident/storage_drivers/ontap/api/rest/client/storage"
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
	mock.EXPECT().NVMeNamespaceCreate(ctx, ns).Return("fakeUUID", nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	UUID, err := oapi.NVMeNamespaceCreate(ctx, ns)
	assert.NoError(t, err)
	assert.Equal(t, ns.UUID, UUID)

	// case 2: Error returned while creating namespace
	mock.EXPECT().NVMeNamespaceCreate(ctx, ns).Return("", fmt.Errorf("Error while creating namespace"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	_, err = oapi.NVMeNamespaceCreate(ctx, ns)
	assert.Error(t, err)
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
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name).Return(ns, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	namespace, err := oapi.NVMeNamespaceGetByName(ctx, Name)
	assert.NoError(t, err)
	assert.Equal(t, UUID, namespace.UUID)
	assert.Equal(t, Name, namespace.Name)

	// case 2: error while getting namespace
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name).Return(nil, fmt.Errorf("Error while getting namespace"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	namespace, err = oapi.NVMeNamespaceGetByName(ctx, Name)
	assert.Error(t, err)

	// case 3: no error while getting namespace but response is nil
	mock.EXPECT().NVMeNamespaceGetByName(ctx, Name).Return(nil, nil)
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
	mock.EXPECT().NVMeNamespaceList(ctx, Name).Return(nsResp, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	namespaces, err := oapi.NVMeNamespaceList(ctx, Name)
	assert.NoError(t, err)
	assert.Equal(t, Name, namespaces[0].Name)

	// case 2: error while getting namespace list
	mock.EXPECT().NVMeNamespaceList(ctx, Name).Return(nil, fmt.Errorf("Error getting namespace list"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	_, err = oapi.NVMeNamespaceList(ctx, Name)
	assert.Error(t, err)

	// case 3: no error while getting namespace list but list is nil
	nsResp1 := &nvme.NvmeNamespaceCollectionGetOK{
		Payload: &models.NvmeNamespaceResponse{
			NvmeNamespaceResponseInlineRecords: []*models.NvmeNamespace{nil},
		},
	}
	mock.EXPECT().NVMeNamespaceList(ctx, Name).Return(nsResp1, nil).AnyTimes()
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	_, err = oapi.NVMeNamespaceList(ctx, Name)
	assert.Error(t, err)
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
	mock.EXPECT().NVMeSubsystemAddNamespace(ctx, subsysUUID, nsUUID).Return(fmt.Errorf("Error while adding NS to subsystem"))
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
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).Return(fmt.Errorf("Error while removing NS to subsystem"))
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
	assert.Equal(t, int64(1), count)
	assert.NoError(t, err)

	// case 2: Error returned while adding namespace to subsystem
	subsystemUUID = "fakeSubsystemUUID"
	mock.EXPECT().NVMeNamespaceCount(ctx, subsystemUUID).Return(int64(0), fmt.Errorf("Error while removing NS to subsystem"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	count, err = oapi.NVMeSubsystemGetNamespaceCount(ctx, subsystemUUID)
	assert.Equal(t, int64(0), count)
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
	mock.EXPECT().NVMeSubsystemDelete(ctx, subsystemUUID).Return(fmt.Errorf("Error while deleting subsystem"))
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
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return(nil, fmt.Errorf("Error while getting hosts for subsystem"))
	mock.EXPECT().NVMeAddHostNqnToSubsystem(ctx, hostNQN, subsystemUUID).AnyTimes().Return(nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	err = oapi.NVMeAddHostToSubsystem(ctx, hostNQN, subsystemUUID)
	assert.Error(t, err)

	// case 3: Error returned while adding host to subsystem
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil)
	mock.EXPECT().NVMeAddHostNqnToSubsystem(ctx, hostNQN1, subsystemUUID).Return(fmt.Errorf("Error while adding hosts to subsystem"))
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

	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return(nil, fmt.Errorf("Error while getting hosts for subsystem"))
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
	mock.EXPECT().NVMeRemoveHostFromSubsystem(ctx, hostNQN, subsystemUUID).Return(fmt.Errorf("Error while removing host"))

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
	subsysUUID := "fakeUUID"
	targetNQN := "fakeTargetNQN"
	subsys := &models.NvmeSubsystem{
		Name:      &subsystemName,
		UUID:      &subsysUUID,
		TargetNqn: &targetNQN,
	}

	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName).Return(subsys, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	subsystem, err := oapi.NVMeSubsystemCreate(ctx, subsystemName)
	assert.NoError(t, err)
	assert.Equal(t, subsystem.UUID, subsysUUID)
	assert.Equal(t, subsystem.Name, subsystemName)
	assert.Equal(t, subsystem.NQN, targetNQN)

	// case 2: Error getting susbsystem info from backend
	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName).Return(nil, fmt.Errorf("Error getting susbsystem info"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	_, err = oapi.NVMeSubsystemCreate(ctx, subsystemName)
	assert.Error(t, err)

	// case 3: Subsystem not present, create a new one successfully
	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName).Return(nil, nil)
	mock.EXPECT().NVMeSubsystemCreate(ctx, subsystemName).Return(subsys, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	newsubsys, err := oapi.NVMeSubsystemCreate(ctx, subsystemName)
	assert.NoError(t, err)
	assert.Equal(t, newsubsys.UUID, subsysUUID)
	assert.Equal(t, newsubsys.Name, subsystemName)
	assert.Equal(t, newsubsys.NQN, targetNQN)

	// case 4: Subsystem not present, create a new one with failure
	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName).Return(nil, nil)
	mock.EXPECT().NVMeSubsystemCreate(ctx, subsystemName).Return(nil, fmt.Errorf("Error creating susbsystem"))
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	newsubsys, err = oapi.NVMeSubsystemCreate(ctx, subsystemName)
	assert.Error(t, err)

	// case 5: Subsystem not present, create a new one but returned nil
	mock.EXPECT().NVMeSubsystemGetByName(ctx, subsystemName).Return(nil, nil)
	mock.EXPECT().NVMeSubsystemCreate(ctx, subsystemName).Return(nil, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	newsubsys, err = oapi.NVMeSubsystemCreate(ctx, subsystemName)
	assert.Error(t, err)
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
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystem.UUID, nsUUID).Return(false, fmt.Errorf("Error getting namespace subsystem mapping"))

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
	mock.EXPECT().NVMeSubsystemAddNamespace(ctx, subsystem.UUID, nsUUID).Return(fmt.Errorf("Error adding host to subsystem"))

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
	host1 := &models.NvmeSubsystemHost{Nqn: &hostNQN}
	host2 := &models.NvmeSubsystemHost{Nqn: &host2NQN}
	var removePublishInfo bool

	// case 1: Error getting namespace from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(false, fmt.Errorf("Error getting namespace subsystem mapping")).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo)
	assert.Error(t, err)

	// case 2: Namespace is not mapped
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(false, nil).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, true, removePublishInfo)
	assert.NoError(t, err)

	// case 3: Failed to get hosts of the subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return(nil, fmt.Errorf("failed to get hosts")).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo)
	assert.Error(t, err)

	// case 4: hosts of the subsystem not returned
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return(nil, nil).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo)
	assert.Error(t, err)

	// case 5: multiple hosts of the subsystem returned  but error while removing host from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1, host2}, nil).Times(1)
	mock.EXPECT().NVMeRemoveHostFromSubsystem(ctx, hostNQN, subsystemUUID).Return(fmt.Errorf("Error removing host from subsystem")).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo)
	assert.Error(t, err)

	// case 6: multiple hosts of the subsystem returned  and success while removing host from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1, host2}, nil).Times(1)
	mock.EXPECT().NVMeRemoveHostFromSubsystem(ctx, hostNQN, subsystemUUID).Return(nil).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo)
	assert.NoError(t, err)

	// case 7: Error removing namespace from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil).Times(1)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).Return(fmt.Errorf("Error removing namespace from subsystem")).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo)
	assert.Error(t, err)

	// case 8: Error getting namespace count from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil).Times(1)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).Return(nil).Times(1)
	mock.EXPECT().NVMeNamespaceCount(ctx, subsystemUUID).Return(int64(0), fmt.Errorf("Error getting namespace count from subsystem")).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo)
	assert.Error(t, err)

	// case 9: Error deleting subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).Return(nil).Times(1)
	mock.EXPECT().NVMeNamespaceCount(ctx, subsystemUUID).Return(int64(0), nil).Times(1)
	mock.EXPECT().NVMeSubsystemDelete(ctx, subsystemUUID).Return(fmt.Errorf("Error deleting subsystem")).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, false, removePublishInfo)
	assert.Error(t, err)

	// case 10: Success deleting subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil).Times(1)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).AnyTimes().Return(nil).Times(1)
	mock.EXPECT().NVMeNamespaceCount(ctx, subsystemUUID).Return(int64(0), nil).Times(1)
	mock.EXPECT().NVMeSubsystemDelete(ctx, subsystemUUID).Return(nil).Times(1)
	mock.EXPECT().NVMeGetHostsOfSubsystem(ctx, subsystemUUID).Return([]*models.NvmeSubsystemHost{host1}, nil).Times(1)

	removePublishInfo, err = oapi.NVMeEnsureNamespaceUnmapped(ctx, hostNQN, subsystemUUID, nsUUID)

	assert.Equal(t, true, removePublishInfo)
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
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(false, fmt.Errorf(" Error checking if namespace is mapped"))

	_, err = oapi.NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID)

	assert.Error(t, err)

	// case 2: namespace is mapped
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil)

	isMapped, err := oapi.NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID)

	assert.NoError(t, err)
	assert.Equal(t, true, isMapped)

	// case 2: namespace is not mapped
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(false, nil)

	isMapped, err = oapi.NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID)

	assert.NoError(t, err)
	assert.Equal(t, false, isMapped)
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
	mock.EXPECT().VolumeGetByName(ctx, volName).AnyTimes().Return(nil, fmt.Errorf("Error getting the volume"))

	currentState, err := oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, abortStates, maxElapsedTime)

	assert.Error(t, err)
	assert.Equal(t, currentState, "")

	ctrl.Finish()

	// Test2: Error - Volume not found
	ctrl = gomock.NewController(t)
	mock = mockapi.NewMockRestClientInterface(ctrl)
	oapi, err = api.NewOntapAPIRESTFromRestClientInterface(mock)
	assert.NoError(t, err)

	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().VolumeGetByName(ctx, volName).AnyTimes().Return(nil, nil)

	currentState, err = oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, abortStates, maxElapsedTime)

	assert.Error(t, err)
	assert.Equal(t, currentState, "")

	ctrl.Finish()

	// Test3: Error - Volume state not in desired states
	ctrl = gomock.NewController(t)
	mock = mockapi.NewMockRestClientInterface(ctrl)
	oapi, err = api.NewOntapAPIRESTFromRestClientInterface(mock)

	mock.EXPECT().VolumeGetByName(ctx, volName).Return(volume, nil)
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	currentState, err = oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, abortStates, maxElapsedTime)

	assert.NoError(t, err)
	assert.Equal(t, currentState, *volume.State)

	ctrl.Finish()

	// Test4: Error - Volume reaches an abort state
	ctrl = gomock.NewController(t)
	mock = mockapi.NewMockRestClientInterface(ctrl)
	oapi, err = api.NewOntapAPIRESTFromRestClientInterface(mock)
	desiredStates = []string{""}
	abortStates = []string{"error"}
	volume.State = &errorState

	mock.EXPECT().VolumeGetByName(ctx, volName).Return(volume, nil).AnyTimes()
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	currentState, err = oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, abortStates, maxElapsedTime)

	assert.Error(t, err)
	assert.Equal(t, currentState, *volume.State)

	ctrl.Finish()

	// Test5: Error - Volume doesn't reach desired state or abort state
	ctrl = gomock.NewController(t)
	mock = mockapi.NewMockRestClientInterface(ctrl)
	oapi, err = api.NewOntapAPIRESTFromRestClientInterface(mock)
	desiredStates = []string{""}
	abortStates = []string{"fakeerrorState"}
	volume.State = &errorState

	mock.EXPECT().VolumeGetByName(ctx, volName).Return(volume, nil).AnyTimes()
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	currentState, err = oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, abortStates, maxElapsedTime)

	assert.Error(t, err)
	assert.Equal(t, currentState, *volume.State)

	ctrl.Finish()

	// Test6: Error - Volume is in unknown state
	ctrl = gomock.NewController(t)
	mock = mockapi.NewMockRestClientInterface(ctrl)
	oapi, err = api.NewOntapAPIRESTFromRestClientInterface(mock)
	desiredStates = []string{""}
	var newAbortState []string
	volState := "unknown"
	volume.State = &volState

	mock.EXPECT().VolumeGetByName(ctx, volName).Return(volume, nil).AnyTimes()
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	currentState, err = oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, newAbortState, maxElapsedTime)

	assert.Error(t, err)
	assert.Equal(t, currentState, *volume.State)

	ctrl.Finish()

	// Test7: Success - Volume is in desired state
	ctrl = gomock.NewController(t)
	mock = mockapi.NewMockRestClientInterface(ctrl)
	oapi, err = api.NewOntapAPIRESTFromRestClientInterface(mock)
	desiredStates = []string{"online"}
	volState = "online"
	volume.State = &volState

	mock.EXPECT().VolumeGetByName(ctx, volName).Return(volume, nil).AnyTimes()
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()

	currentState, err = oapi.VolumeWaitForStates(ctx, "fakeVolName", desiredStates, newAbortState, maxElapsedTime)

	assert.NoError(t, err)
	assert.Equal(t, currentState, *volume.State)

	ctrl.Finish()
}

func newMockOntapAPIREST(t *testing.T) (api.OntapAPIREST, *mockapi.MockRestClientInterface) {
	ctrl := gomock.NewController(t)

	rsi := mockapi.NewMockRestClientInterface(ctrl)
	ontapAPIREST, _ := api.NewOntapAPIRESTFromRestClientInterface(rsi)
	return ontapAPIREST, rsi
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
	mock.EXPECT().NVMeNamespaceSetSize(ctx, "fakeUUID", int64(3221225472)).Return(fmt.Errorf(
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
	mock.EXPECT().IgroupList(ctx, "").Return(&igroupResponseOK, nil)

	igroups, err1 := oapi.IgroupList(ctx)
	assert.NoError(t, err1, "error while getting Igroup list")
	assert.Equal(t, len(igroups), len(igroupList))
	assert.Equal(t, igroups[0], igroupName)

	// case 2: Error returned while getting igroup list
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().IgroupList(ctx, "").Return(nil, fmt.Errorf("failed to get igroup"))

	_, err1 = oapi.IgroupList(ctx)
	assert.Error(t, err1, "No error while getting Igroup list")

	// case 3: Empty payload returned while getting igroup list
	igroupResponseOK = s_a_n.IgroupCollectionGetOK{Payload: nil}
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().IgroupList(ctx, "").Return(&igroupResponseOK, nil)

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
	mock.EXPECT().IgroupList(ctx, "").Return(&igroupResponseOK, nil)

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
	mock.EXPECT().IgroupRemove(ctx, initiator, initiatorGroup).Return(fmt.Errorf("failed to remove igroup"))

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
	mock.EXPECT().IgroupGetByName(ctx, initiatorGroup).Return(&igroup, nil)

	mappedIQNs, err1 := oapi.IgroupGetByName(ctx, initiatorGroup)
	assert.NoError(t, err1, "error returned while getting igroup by name")
	assert.Equal(t, 2, len(mappedIQNs))
	assert.Equal(t, true, mappedIQNs[initiator1])
	assert.Equal(t, true, mappedIQNs[initiator2])

	// case 2: Error returned while getting igroup by name.
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().IgroupGetByName(ctx, initiatorGroup).Return(nil, fmt.Errorf("Failed to get igroup by name"))

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
	mock.EXPECT().NetworkIPInterfacesList(ctx).Return(nil, fmt.Errorf("failed to get network info"))
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
	mock.EXPECT().GetSVMState(ctx).Return("", fmt.Errorf("failed to get SVM state"))

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
	mock.EXPECT().SMBShareCreate(ctx, gomock.Any(), gomock.Any()).Return(fmt.Errorf("failed to create SMB share"))
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
	mock.EXPECT().SMBShareExists(ctx, gomock.Any()).Return(false, fmt.Errorf("failed to verify SMB share"))
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
	mock.EXPECT().SMBShareDestroy(ctx, "").Return(fmt.Errorf("failed to destroy SMB share"))
	err1 = oapi.SMBShareDestroy(ctx, "")
	assert.Error(t, err1, "no error returned while deleting SMB share")
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
		Return(fmt.Errorf("Volume create failed"))
	err = oapi.VolumeCreate(ctx, volume)
	assert.Error(t, err, "no error returned while creating volume")
}

func TestVolumeDestroy(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Delete volume, Positive test
	rsi.EXPECT().VolumeDestroy(ctx, "vol1").Return(nil)
	err := oapi.VolumeDestroy(ctx, "vol1", false)
	assert.NoError(t, err, "error returned while deleting a volume")

	// case 2: Delete volume, returned error.
	rsi.EXPECT().VolumeDestroy(ctx, "vol1").Return(fmt.Errorf("failed to delete volume"))
	err = oapi.VolumeDestroy(ctx, "vol1", false)
	assert.Error(t, err, "no error returned while deleting a volume")
}

func TestVolumeInfo(t *testing.T) {
	volume := getVolumeInfo()

	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Get volume. Returned volume list
	rsi.EXPECT().VolumeGetByName(ctx, "vol1").Return(volume, nil)
	_, err := oapi.VolumeInfo(ctx, "vol1")
	assert.NoError(t, err, "error returned while getting a volume")

	// case 2: Get volume. Returned empty list
	rsi.EXPECT().VolumeGetByName(ctx, "vol1").Return(nil, nil)
	_, err = oapi.VolumeInfo(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a volume")

	// case 3: Get volume. Backend returned an error.
	rsi.EXPECT().VolumeGetByName(ctx, "vol1").Return(nil, fmt.Errorf("Failed to get volume"))
	_, err = oapi.VolumeInfo(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a volume")

	// case 4: Get volume. Returned volume list with volume name nil
	// Want an error from "VolumeInfoFromRestAttrsHelper" function.
	volume.Name = nil
	rsi.EXPECT().VolumeGetByName(ctx, "vol1").Return(volume, nil)
	_, err = oapi.VolumeInfo(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a volume")

	// case 5: Get volume. Returned volume list with volume aggregates nil
	volume.Name = utils.Ptr("vol1")
	volume.VolumeInlineAggregates = nil
	rsi.EXPECT().VolumeGetByName(ctx, "vol1").Return(volume, nil)
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
		Return(fmt.Errorf("Could not generate Autosupport message"))
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
		Return(fmt.Errorf("flexgroup volume creation failed"))
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
		fmt.Errorf("flexgroup volume splits clone failed"))
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
		fmt.Errorf("flexgroup volume disable snapshot directory failed"))
	err = oapi.FlexgroupModifySnapshotDirectoryAccess(ctx, "fake-volume", false)
	assert.Error(t, err, "No error returned while disable snapshot dir access of a flexgroup volume")
}

func TestFlexgroupInfo(t *testing.T) {
	volume := getVolumeInfo()
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup get by name
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(volume, nil)
	_, err := oapi.FlexgroupInfo(ctx, "vol1")
	assert.NoError(t, err, "error returned while getting a flexgroup volume")

	// case 2: Flexgroup get by name returned volume info nil
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(nil, nil)
	_, err = oapi.FlexgroupInfo(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup volume")

	// case 3: Flexgroup get by name, returned error.
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(nil, fmt.Errorf("failed to get flexgroup volume"))
	_, err = oapi.FlexgroupInfo(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup volume")

	// case 4: Flexgroup get by name. Response contain volume name is nil
	volume.Name = nil
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(volume, nil)
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
		fmt.Errorf("failed to update comment on flexgroup volume"))
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
		fmt.Errorf("failed to update comment on flexgroup volume"))
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
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(volume, nil)
	rsi.EXPECT().SnapshotCreateAndWait(ctx, volumeUUID, "fake-snapshot").Return(nil)
	err := oapi.FlexgroupSnapshotCreate(ctx, "fake-snapshot", "vol1")
	assert.NoError(t, err, "error returned while creating snapshot of a flexgroup")

	// case 2: Flexgroup snapshot create. Could not get the volume info
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(nil, fmt.Errorf("Failed to get flexgroup"))
	err = oapi.FlexgroupSnapshotCreate(ctx, "fake-snapshot", "vol1")
	assert.Error(t, err, "no error returned while creating snapshot of a flexgroup")

	// case 3: Flexgroup snapshot create returned response nil
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(nil, nil)
	err = oapi.FlexgroupSnapshotCreate(ctx, "fake-snapshot", "vol1")
	assert.Error(t, err, "no error returned while creating snapshot of a flexgroup")

	// case 4: Flexgroup snapshot create. Could not create snapshot.
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(volume, nil)
	rsi.EXPECT().SnapshotCreateAndWait(ctx, volumeUUID, "fake-snapshot").Return(fmt.Errorf("snapshot creation failed"))
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
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(volume, nil)
	rsi.EXPECT().SnapshotList(ctx, volumeUUID).Return(&snapshotResponseOK, nil)
	snapshots, err := oapi.FlexgroupSnapshotList(ctx, "vol1")
	assert.NoError(t, err, "error returned while getting a flexgroup")
	assert.NotNil(t, snapshots)
	assert.Equal(t, "snapvol1", snapshots[0].Name)
	assert.Equal(t, createTime1.String(), snapshots[0].CreateTime)

	// case 2: Flexgroup get by name returned error
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(nil, fmt.Errorf("failed to get flexgroup"))
	_, err = oapi.FlexgroupSnapshotList(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup")

	// case 3: Flexgroup get by name returned nil
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(nil, nil)
	_, err = oapi.FlexgroupSnapshotList(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup")

	// case 4: Flexgroup snapshot list returned nil
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(volume, nil)
	rsi.EXPECT().SnapshotList(ctx, volumeUUID).Return(nil, nil)
	snapshots, err = oapi.FlexgroupSnapshotList(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup")

	// case 5: Flexgroup snapshot list returned error
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(volume, nil)
	rsi.EXPECT().SnapshotList(ctx, volumeUUID).Return(nil, fmt.Errorf("failed to get snapshot info"))
	snapshots, err = oapi.FlexgroupSnapshotList(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a flexgroup")

	// case 6: Flexgroup snapshot list returned payload nil
	snapshotResponseOK = storage.SnapshotCollectionGetOK{Payload: nil}
	rsi.EXPECT().FlexGroupGetByName(ctx, "vol1").Return(volume, nil)
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
		fmt.Errorf("failed to modify unix permission on flexgroup volume"))
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
		fmt.Errorf("failed to mount flexgroup volume"))
	err = oapi.FlexgroupMount(ctx, "trident-pvc-1234", "/trident-pvc-1234")
	assert.Error(t, err, "no error returned while mounting a volume")
}

func TestFlexgroupDestroy(t *testing.T) {
	oapi, rsi := newMockOntapAPIREST(t)

	// case 1: Flexgroup destroy
	rsi.EXPECT().FlexgroupUnmount(ctx, "trident-pvc-1234").Return(nil)
	rsi.EXPECT().FlexGroupDestroy(ctx, "trident-pvc-1234").Return(nil)
	err := oapi.FlexgroupDestroy(ctx, "trident-pvc-1234", true)
	assert.NoError(t, err, "error returned while deleting a volume")

	// case 2: Flexgroup destroyed returned error
	rsi.EXPECT().FlexgroupUnmount(ctx, "trident-pvc-1234").Return(nil)
	rsi.EXPECT().FlexGroupDestroy(ctx, "trident-pvc-1234").Return(
		fmt.Errorf("failed to delete flexgroup volume"))
	err = oapi.FlexgroupDestroy(ctx, "trident-pvc-1234", true)
	assert.Error(t, err, "no error returned while deleting a volume")

	// case 3: Flexgroup unmount returned error
	rsi.EXPECT().FlexgroupUnmount(ctx, "trident-pvc-1234").Return(
		fmt.Errorf("failed to unmount flexgroup volume"))
	err = oapi.FlexgroupDestroy(ctx, "trident-pvc-1234", true)
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
	rsi.EXPECT().FlexGroupGetAll(ctx, *volume.Name+"*").Return(&volumeResponse, nil)
	volumeInfo, err := oapi.FlexgroupListByPrefix(ctx, *volume.Name)
	assert.NoError(t, err, "error returned while getting a volume")
	assert.Equal(t, *volume.Name, volumeInfo[0].Name)

	// case 2: Flexgroup get returned empty list
	rsi.EXPECT().FlexGroupGetAll(ctx, *volume.Name+"*").Return(nil, nil)
	volumeInfo, err = oapi.FlexgroupListByPrefix(ctx, *volume.Name)
	assert.Error(t, err, "no error returned while getting a volume")

	// case 3: Flexgroup get returned error
	rsi.EXPECT().FlexGroupGetAll(ctx, *volume.Name+"*").Return(nil, fmt.Errorf("failed to get flexgroup"))
	volumeInfo, err = oapi.FlexgroupListByPrefix(ctx, *volume.Name)
	assert.Error(t, err, "no error returned while getting a volume")

	// case 4: Flexgroup get list, response contain volume name nil
	volume.Name = nil
	rsi.EXPECT().FlexGroupGetAll(ctx, "*").Return(&volumeResponse, nil)
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
		fmt.Errorf("failed to update flexgroup volume size"))
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
		fmt.Errorf("failed to get flexgroup volume size"))
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
		fmt.Errorf("failed to get flexgroup volume used size"))
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
		fmt.Errorf("failed to modify export policy"))
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
		fmt.Errorf("failed to unmount flexgroup volume"))
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
	rsi.EXPECT().AggregateList(ctx, gomock.Any()).Return(aggregateResponse, nil)
	aggrList, err := oapi.GetSVMAggregateAttributes(ctx)
	assert.NoError(t, err, "error returned while getting a svm aggregate")
	assert.Equal(t, "fc", aggrList["aggr1"])

	// case 2: Get SVM aggregate returned nil list
	rsi.EXPECT().AggregateList(ctx, gomock.Any()).Return(nil, nil)
	aggrList, err = oapi.GetSVMAggregateAttributes(ctx)
	assert.Error(t, err, "no error returned while getting a svm aggregate")
	assert.Nil(t, aggrList)

	aggregateResponse = getAggregateInfo("", "fc", int64(1), nil)

	// case 3: Get SVM aggregate returned empty list
	rsi.EXPECT().AggregateList(ctx, gomock.Any()).Return(aggregateResponse, nil)
	aggrList, err = oapi.GetSVMAggregateAttributes(ctx)
	assert.NoError(t, err, "error returned while getting a svm aggregate")
	assert.Empty(t, aggrList)

	// case 4: Get SVM aggregate returned error
	aggregateResponse = getAggregateInfo("", "fc", int64(0), nil)
	rsi.EXPECT().AggregateList(ctx, gomock.Any()).Return(aggregateResponse, nil)
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
		fmt.Errorf("failed to delete export policy"))
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
		fmt.Errorf("flexgroup volume does not exists"))
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
	rsi.EXPECT().AggregateList(ctx, "aggr1").Return(aggrResponse, nil)
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
	rsi.EXPECT().AggregateList(ctx, "aggr1").Return(aggrResponse2, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr1")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	aggrSpace3 := models.AggregateInlineSpace{
		BlockStorage: &models.AggregateInlineSpaceInlineBlockStorage{
			Size: &size,
		},
	}

	// case 3: Get SVM aggregate space, Footprint and used size nil
	aggrResponse3 := getAggregateInfo("aggr1", "fc", int64(1), &aggrSpace3)
	rsi.EXPECT().AggregateList(ctx, "aggr1").Return(aggrResponse3, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr1")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	aggrSpace4 := models.AggregateInlineSpace{BlockStorage: &models.AggregateInlineSpaceInlineBlockStorage{}}

	// case 4: Get SVM aggregate space, Size nil
	aggrResponse4 := getAggregateInfo("aggr1", "fc", int64(1), &aggrSpace4)
	rsi.EXPECT().AggregateList(ctx, "aggr1").Return(aggrResponse4, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr1")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	// case 5: Get SVM aggregate space, aggregate space nil
	aggrResponse5 := getAggregateInfo("aggr1", "fc", int64(1), nil)
	rsi.EXPECT().AggregateList(ctx, "aggr1").Return(aggrResponse5, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr1")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	// case 6: Get SVM aggregate space, aggregate name not present
	aggrResponse6 := getAggregateInfo("aggr1", "fc", int64(1), nil)
	rsi.EXPECT().AggregateList(ctx, "aggr2").Return(aggrResponse6, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr2")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	// case 7: Get SVM aggregate space, aggregate name is empty
	aggrResponse7 := getAggregateInfo("", "fc", int64(1), nil)
	rsi.EXPECT().AggregateList(ctx, "aggr2").Return(aggrResponse7, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr2")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	// case 8: Get SVM aggregate space, payload is empty object
	aggrResponse8 := storage.AggregateCollectionGetOK{
		Payload: &models.AggregateResponse{},
	}
	rsi.EXPECT().AggregateList(ctx, "aggr2").Return(&aggrResponse8, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr2")
	assert.NoError(t, err, "error returned while getting a aggregate space")

	// case 9: Get SVM aggregate space, payload is nil
	aggrResponse9 := storage.AggregateCollectionGetOK{}
	rsi.EXPECT().AggregateList(ctx, "aggr2").Return(&aggrResponse9, nil)
	_, err = oapi.GetSVMAggregateSpace(ctx, "aggr2")
	assert.Error(t, err, "no error returned while getting a aggregate space")

	// case 10: Get SVM aggregate space, aggregate list is nil
	rsi.EXPECT().AggregateList(ctx, "aggr2").Return(nil, nil)
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
		Return(fmt.Errorf("error disabling snapshot directory access"))
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
		Return(fmt.Errorf("failed to mount volume"))
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
		fmt.Errorf("failed to update comment on volume"))
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
		fmt.Errorf("failed to verify export policy"))
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
		fmt.Errorf("export policy creation failed"))
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
		fmt.Errorf("failed to update volume size"))
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
		fmt.Errorf("failed to modify unix permission on flexgroup volume"))
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

	rsi.EXPECT().VolumeList(ctx, *volume.Name+"*").Return(&volumeResponse1, nil)
	volumeInfo, err := oapi.VolumeListByPrefix(ctx, *volume.Name)
	assert.NoError(t, err, "error returned while getting list of volumes")
	assert.Equal(t, *volume.Name, volumeInfo[0].Name)

	volumeResponse2 := storage.VolumeCollectionGetOK{
		Payload: &models.VolumeResponse{
			VolumeResponseInlineRecords: []*models.Volume{nil},
		},
	}

	// case 1: Get volume positive test
	rsi.EXPECT().VolumeList(ctx, *volume.Name+"*").Return(&volumeResponse2, nil)
	_, err = oapi.VolumeListByPrefix(ctx, *volume.Name)
	assert.Error(t, err, "no error returned while getting a volume info")

	// case 2: Get volume negative test
	rsi.EXPECT().VolumeList(ctx, *volume.Name+"*").Return(nil, fmt.Errorf("failed to get volume"))
	_, err = oapi.VolumeListByPrefix(ctx, *volume.Name)
	assert.Error(t, err, "no error returned while getting a volume info")
}

func TestVolumeListByAttrs(t *testing.T) {
	volume := api.Volume{Name: "vol1"}
	volumes := []*api.Volume{&volume}
	oapi, rsi := newMockOntapAPIREST(t)

	rsi.EXPECT().VolumeListByAttrs(ctx, &volume).Return(volumes, nil)
	volumeList, err := oapi.VolumeListByAttrs(ctx, &volume)
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
		[]string{"any"}, []string{"any"}).Return(nil, fmt.Errorf("failed to get export rule"))
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
	rsi.EXPECT().ExportRuleDestroy(ctx, "fake-policy", 1).Return(nil, fmt.Errorf("failed to get export rule"))
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
		fmt.Errorf("failed to modify export policy"))
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
		fmt.Errorf("failed to verify export policy"))
	isExists, err = oapi.ExportPolicyExists(ctx, "fake-exportPolicy")
	assert.Error(t, err, "no error returned while getting an export policy")
	assert.Equal(t, false, isExists)
}

func TestExportRuleList(t *testing.T) {
	exportClient := "exportClientMatch"
	ruleIndex := int64(1)
	numRecords := int64(1)
	oapi, rsi := newMockOntapAPIREST(t)

	exportRule := models.ExportRuleResponse{
		ExportRuleResponseInlineRecords: []*models.ExportRule{
			{
				ExportRuleInlineClients: []*models.ExportClients{{Match: &exportClient}},
				Index:                   &ruleIndex,
			},
		},
		NumRecords: &numRecords,
	}

	exportRuleResponse := nas.ExportRuleCollectionGetOK{
		Payload: &exportRule,
	}

	// case 1: Export rule get list
	rsi.EXPECT().ExportRuleList(ctx, "fake-policy").Return(&exportRuleResponse, nil)
	rules, err := oapi.ExportRuleList(ctx, "fake-policy")
	assert.NoError(t, err, "error returned while getting a export rule list")
	assert.Equal(t, 1, rules["exportClientMatch"])

	// case 2: Export rule get list returned error
	rsi.EXPECT().ExportRuleList(ctx, "fake-policy").Return(nil, fmt.Errorf("failed to get export rule"))
	rules, err = oapi.ExportRuleList(ctx, "fake-policy")
	assert.Error(t, err, "error returned while getting a export rule list")
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
	rsi.EXPECT().QtreeList(ctx, gomock.Any(), gomock.Any()).Return(&qtreeResponse, nil)
	volumeInfo, err := oapi.QtreeListByPrefix(ctx, *qtree.Name, *qtree.Volume.Name)
	assert.NoError(t, err, "error returned while getting a Qtree list")
	assert.Equal(t, *qtree.Name, volumeInfo[0].Name)

	// case 2: Get the Qtree list failed. Backend returned an error
	rsi.EXPECT().QtreeList(ctx, gomock.Any(), gomock.Any()).Return(nil,
		fmt.Errorf("failed to get qtree for given prefix"))
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
	rsi.EXPECT().QtreeGet(ctx, "vol1", "").Return(&qtree, fmt.Errorf("Failed to get qtree"))
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

	rsi.EXPECT().QuotaEntryList(ctx, quotaVolumeName).Return(nil, fmt.Errorf("failed to quota entry"))
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
	rsi.EXPECT().QuotaOff(ctx, quotaVolumeName).Return(fmt.Errorf("error disabling quota"))
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
	rsi.EXPECT().QuotaOn(ctx, quotaVolumeName).Return(fmt.Errorf("error enabling quota"))
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
	rsi.EXPECT().VolumeGetByName(ctx, "vol1").Return(volume, nil)
	quotaStatus, err := oapi.QuotaStatus(ctx, "vol1")
	assert.NoError(t, err, "error returned while getting a quota status")
	assert.Equal(t, *volume.Quota.State, quotaStatus)

	rsi.EXPECT().VolumeGetByName(ctx, "vol1").Return(nil, fmt.Errorf("error enabling quota"))
	quotaStatus, err = oapi.QuotaStatus(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a quota status")

	volume.Quota.State = nil

	// case 2: Quota status in volume is nil
	rsi.EXPECT().VolumeGetByName(ctx, "vol1").Return(volume, nil)
	quotaStatus, err = oapi.QuotaStatus(ctx, "vol1")
	assert.Error(t, err, "no error returned while getting a quota status")

	volume.Quota = nil

	// case 3: Quota in volume is nil
	rsi.EXPECT().VolumeGetByName(ctx, "vol1").Return(volume, nil)
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
		&quotaRule, fmt.Errorf("error getting quota rule"))
	_, err = oapi.QuotaGetEntry(ctx, quotaVolumeName, quotaQtreeName, quotaType)
	assert.NoError(t, err, "no error returned while getting a quota")
}
