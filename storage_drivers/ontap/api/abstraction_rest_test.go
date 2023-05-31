// Copyright 2022 NetApp, Inc. All Rights Reserved.

package api_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	nvme "github.com/netapp/trident/storage_drivers/ontap/api/rest/client/n_v_me"
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

	// case 1: Error getting namespace from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(false, fmt.Errorf("Error getting namespace subsystem mapping"))

	err = oapi.NVMeEnsureNamespaceUnmapped(ctx, subsystemUUID, nsUUID)

	assert.Error(t, err)

	// case 2: Namespace is not mapped
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(false, nil)

	err = oapi.NVMeEnsureNamespaceUnmapped(ctx, subsystemUUID, nsUUID)

	assert.NoError(t, err)

	// case 3: Error removing namespace from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).Return(fmt.Errorf("Error removing namespace from subsystem"))

	err = oapi.NVMeEnsureNamespaceUnmapped(ctx, subsystemUUID, nsUUID)

	assert.Error(t, err)

	// case 4: Error getting namespace count from subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).Return(nil)
	mock.EXPECT().NVMeNamespaceCount(ctx, subsystemUUID).Return(int64(0), fmt.Errorf("Error getting namespace count from subsystem"))

	err = oapi.NVMeEnsureNamespaceUnmapped(ctx, subsystemUUID, nsUUID)

	assert.Error(t, err)

	// case 5: Error deleting subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).Return(nil)
	mock.EXPECT().NVMeNamespaceCount(ctx, subsystemUUID).Return(int64(0), nil)
	mock.EXPECT().NVMeSubsystemDelete(ctx, subsystemUUID).Return(fmt.Errorf("Error deleting subsystem"))

	err = oapi.NVMeEnsureNamespaceUnmapped(ctx, subsystemUUID, nsUUID)

	assert.Error(t, err)

	// case 6: Success deleting subsystem
	mock.EXPECT().ClientConfig().Return(clientConfig).AnyTimes()
	mock.EXPECT().NVMeIsNamespaceMapped(ctx, subsystemUUID, nsUUID).Return(true, nil)
	mock.EXPECT().NVMeSubsystemRemoveNamespace(ctx, subsystemUUID, nsUUID).Return(nil)
	mock.EXPECT().NVMeNamespaceCount(ctx, subsystemUUID).Return(int64(0), nil)
	mock.EXPECT().NVMeSubsystemDelete(ctx, subsystemUUID).Return(nil)

	err = oapi.NVMeEnsureNamespaceUnmapped(ctx, subsystemUUID, nsUUID)

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
