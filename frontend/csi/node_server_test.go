// Copyright 2022 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/golang/mock/gomock"
	"github.com/mitchellh/copystructure"
	"github.com/stretchr/testify/assert"

	mockControllerAPI "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_api"
	mockNodeHelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	mockUtils "github.com/netapp/trident/mocks/mock_utils"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
)

func TestUpdateChapInfoFromController_Success(t *testing.T) {
	testCtx := context.Background()
	volumeName := "foo"
	nodeName := "bar"
	expectedChapInfo := utils.IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "user",
		IscsiInitiatorSecret: "pass",
		IscsiTargetUsername:  "user2",
		IscsiTargetSecret:    "pass2",
	}

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().GetChap(testCtx, volumeName, nodeName).Return(&expectedChapInfo, nil)
	nodeServer := &Plugin{
		nodeName:   nodeName,
		role:       CSINode,
		restClient: mockClient,
	}

	fakeRequest := &csi.NodeStageVolumeRequest{VolumeId: volumeName}
	testPublishInfo := &utils.VolumePublishInfo{}

	err := nodeServer.updateChapInfoFromController(testCtx, fakeRequest, testPublishInfo)
	assert.Nil(t, err, "Unexpected error")
	assert.EqualValues(t, expectedChapInfo, testPublishInfo.IscsiAccessInfo.IscsiChapInfo)
}

func TestUpdateChapInfoFromController_Error(t *testing.T) {
	testCtx := context.Background()
	volumeName := "foo"
	nodeName := "bar"
	expectedChapInfo := utils.IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "user",
		IscsiInitiatorSecret: "pass",
		IscsiTargetUsername:  "user2",
		IscsiTargetSecret:    "pass2",
	}

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().GetChap(testCtx, volumeName, nodeName).Return(&expectedChapInfo, fmt.Errorf("some error"))
	nodeServer := &Plugin{
		nodeName:   nodeName,
		role:       CSINode,
		restClient: mockClient,
	}

	fakeRequest := &csi.NodeStageVolumeRequest{VolumeId: volumeName}
	testPublishInfo := &utils.VolumePublishInfo{}

	err := nodeServer.updateChapInfoFromController(testCtx, fakeRequest, testPublishInfo)
	assert.NotNil(t, err, "Unexpected success")
	assert.NotEqualValues(t, expectedChapInfo, testPublishInfo.IscsiAccessInfo.IscsiChapInfo)
	assert.EqualValues(t, utils.IscsiChapInfo{}, testPublishInfo.IscsiAccessInfo.IscsiChapInfo)
}

type PortalAction struct {
	Portal string
	Action utils.ISCSIAction
}

func TestFixISCSISessions(t *testing.T) {
	lunList1 := map[int32]string{
		1: "volID-1",
		2: "volID-2",
		3: "volID-3",
	}

	lunList2 := map[int32]string{
		2: "volID-2",
		3: "volID-3",
		4: "volID-4",
	}

	ipList := []string{"1.2.3.4", "2.3.4.5", "3.4.5.6", "4.5.6.7"}

	iqnList := []string{"IQN1", "IQN2", "IQN3", "IQN4"}

	chapCredentials := []utils.IscsiChapInfo{
		{
			UseCHAP: false,
		},
		{
			UseCHAP:              true,
			IscsiUsername:        "username1",
			IscsiInitiatorSecret: "secret1",
			IscsiTargetUsername:  "username2",
			IscsiTargetSecret:    "secret2",
		},
		{
			UseCHAP:              true,
			IscsiUsername:        "username11",
			IscsiInitiatorSecret: "secret11",
			IscsiTargetUsername:  "username22",
			IscsiTargetSecret:    "secret22",
		},
	}

	sessionData1 := utils.ISCSISessionData{
		PortalInfo: utils.PortalInfo{
			ISCSITargetIQN: iqnList[0],
			Credentials:    chapCredentials[2],
		},
		LUNs: utils.LUNs{
			Info: mapCopyHelper(lunList1),
		},
	}

	sessionData2 := utils.ISCSISessionData{
		PortalInfo: utils.PortalInfo{
			ISCSITargetIQN: iqnList[1],
			Credentials:    chapCredentials[2],
		},
		LUNs: utils.LUNs{
			Info: mapCopyHelper(lunList2),
		},
	}

	type PreRun func(publishedSessions, currentSessions *utils.ISCSISessions, portalActions []PortalAction)

	inputs := []struct {
		TestName           string
		PublishedPortals   *utils.ISCSISessions
		CurrentPortals     *utils.ISCSISessions
		PortalActions      []PortalAction
		StopAt             time.Time
		AddNewNodeOps      bool // If there exist a new node operation would request lock.
		SimulateConditions PreRun
		PortalsFixed       []string
	}{
		{
			TestName: "No current sessions exist then all the non-stale sessions are fixed",
			PublishedPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: utils.NoAction},
				{Portal: ipList[1], Action: utils.NoAction},
				{Portal: ipList[2], Action: utils.NoAction},
			},
			StopAt:        time.Now().Add(100 * time.Second),
			AddNewNodeOps: false,
			PortalsFixed:  []string{ipList[0], ipList[1], ipList[2]},
			SimulateConditions: func(publishedSessions, currentSessions *utils.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow.Add(5 * time.Millisecond)
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(10 * time.Millisecond)

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "No current sessions exist AND self-heal exceeded max time AND NO node operation waiting then" +
				" all the non-stale sessions are fixed",
			PublishedPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: utils.NoAction},
				{Portal: ipList[1], Action: utils.NoAction},
				{Portal: ipList[2], Action: utils.NoAction},
			},
			StopAt:        time.Now().Add(-time.Second * 100),
			AddNewNodeOps: false,
			PortalsFixed:  []string{ipList[0], ipList[1], ipList[2]},
			SimulateConditions: func(publishedSessions, currentSessions *utils.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow.Add(5 * time.Millisecond)
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(10 * time.Millisecond)

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "No current sessions exist AND exist a node operation waiting then first non-stale sessions is fixed",
			PublishedPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: utils.NoAction},
				{Portal: ipList[1], Action: utils.NoAction},
				{Portal: ipList[2], Action: utils.NoAction},
			},
			StopAt:        time.Now().Add(time.Second * 100),
			AddNewNodeOps: true,
			PortalsFixed:  []string{ipList[1]},
			SimulateConditions: func(publishedSessions, currentSessions *utils.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow.Add(5 * time.Millisecond)
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(10 * time.Millisecond)

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "No current sessions exist AND self-heal exceeded max time AND exist a node operation waiting" +
				" for lock then first non-stale sessions is fixed",
			PublishedPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: utils.NoAction},
				{Portal: ipList[1], Action: utils.NoAction},
				{Portal: ipList[2], Action: utils.NoAction},
			},
			StopAt:        time.Time{},
			AddNewNodeOps: true,
			PortalsFixed:  []string{ipList[1]},
			SimulateConditions: func(publishedSessions, currentSessions *utils.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow.Add(5 * time.Millisecond)
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(10 * time.Millisecond)

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "Current sessions exist but missing LUNs then all the non-stale sessions are fixed",
			PublishedPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: utils.NoAction},
				{Portal: ipList[1], Action: utils.NoAction},
				{Portal: ipList[2], Action: utils.NoAction},
			},
			StopAt:        time.Now().Add(100 * time.Second),
			AddNewNodeOps: false,
			PortalsFixed:  []string{ipList[0], ipList[1], ipList[2]},
			SimulateConditions: func(publishedSessions, currentSessions *utils.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow.Add(5 * time.Millisecond)
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(10 * time.Millisecond)

				currentSessions.Info[ipList[0]].LUNs = utils.LUNs{
					Info: nil,
				}
				currentSessions.Info[ipList[1]].LUNs = utils.LUNs{
					Info: nil,
				}
				currentSessions.Info[ipList[2]].LUNs = utils.LUNs{
					Info: nil,
				}

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "Current sessions exist but missing LUNs AND exist a node operation waiting" +
				" for lock then first non-stale sessions is fixed",
			PublishedPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: utils.NoAction},
				{Portal: ipList[1], Action: utils.NoAction},
				{Portal: ipList[2], Action: utils.NoAction},
			},
			StopAt:        time.Now().Add(100 * time.Second),
			AddNewNodeOps: true,
			PortalsFixed:  []string{ipList[1]},
			SimulateConditions: func(publishedSessions, currentSessions *utils.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow.Add(5 * time.Millisecond)
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(10 * time.Millisecond)

				currentSessions.Info[ipList[0]].LUNs = utils.LUNs{
					Info: nil,
				}
				currentSessions.Info[ipList[1]].LUNs = utils.LUNs{
					Info: nil,
				}
				currentSessions.Info[ipList[2]].LUNs = utils.LUNs{
					Info: nil,
				}

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "Current sessions are stale then all the stale sessions are fixed",
			PublishedPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: utils.LogoutLoginScan},
				{Portal: ipList[1], Action: utils.LogoutLoginScan},
				{Portal: ipList[2], Action: utils.LogoutLoginScan},
			},
			StopAt:        time.Now().Add(100 * time.Second),
			AddNewNodeOps: false,
			PortalsFixed:  []string{ipList[0], ipList[1], ipList[2]},
			SimulateConditions: func(publishedSessions, currentSessions *utils.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow.Add(5 * time.Millisecond)
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(10 * time.Millisecond)

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "Current sessions are stale AND only exist a node operation waiting" +
				" for lock BUT self-heal has not exceeded then all stale sessions are fixed",
			PublishedPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: utils.LogoutLoginScan},
				{Portal: ipList[1], Action: utils.LogoutLoginScan},
				{Portal: ipList[2], Action: utils.LogoutLoginScan},
			},
			StopAt:        time.Now().Add(100 * time.Second),
			AddNewNodeOps: true,
			PortalsFixed:  []string{ipList[0], ipList[1], ipList[2]},
			SimulateConditions: func(publishedSessions, currentSessions *utils.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow.Add(5 * time.Millisecond)
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(10 * time.Millisecond)

				setRemediation(publishedSessions, portalActions)
			},
		},
		{
			TestName: "Current sessions are stale AND exist a node operation waiting" +
				" for lock AND self-heal exceeds time then first stale sessions is fixed",
			PublishedPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &utils.ISCSISessions{Info: map[string]*utils.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			PortalActions: []PortalAction{
				{Portal: ipList[0], Action: utils.LogoutLoginScan},
				{Portal: ipList[1], Action: utils.LogoutLoginScan},
				{Portal: ipList[2], Action: utils.LogoutLoginScan},
			},
			StopAt:        time.Time{},
			AddNewNodeOps: true,
			PortalsFixed:  []string{ipList[1]},
			SimulateConditions: func(publishedSessions, currentSessions *utils.ISCSISessions,
				portalActions []PortalAction,
			) {
				timeNow := time.Now()
				publishedSessions.Info[ipList[0]].PortalInfo.LastAccessTime = timeNow.Add(5 * time.Millisecond)
				publishedSessions.Info[ipList[1]].PortalInfo.LastAccessTime = timeNow
				publishedSessions.Info[ipList[2]].PortalInfo.LastAccessTime = timeNow.Add(10 * time.Millisecond)

				setRemediation(publishedSessions, portalActions)
			},
		},
	}

	nodeServer := &Plugin{
		nodeName: "someNode",
		role:     CSINode,
	}

	for _, input := range inputs {
		t.Run(input.TestName, func(t *testing.T) {
			publishedISCSISessions = *input.PublishedPortals
			currentISCSISessions = *input.CurrentPortals

			input.SimulateConditions(input.PublishedPortals, input.CurrentPortals, input.PortalActions)
			portals := getPortals(input.PublishedPortals, input.PortalActions)

			if input.AddNewNodeOps {
				go utils.Lock(ctx, "test-lock1", lockID)
				snooze(10)
				go utils.Lock(ctx, "test-lock2", lockID)
				snooze(10)
			}

			// Make sure this time is captured after the pre-run adds wait time
			// Also on Windows the system time is often only updated once every
			// 10-15 ms or so, which means if you query the current time twice
			// within this period, you get the same value. Therefore, set this
			// time to be slightly lower than time set in fixISCSISessions call.
			timeNow := time.Now().Add(-2 * time.Millisecond)

			nodeServer.fixISCSISessions(context.TODO(), portals, "some-portal", input.StopAt)

			for _, portal := range portals {
				lastAccessTime := publishedISCSISessions.Info[portal].PortalInfo.LastAccessTime
				if utils.SliceContainsString(input.PortalsFixed, portal) {
					assert.True(t, lastAccessTime.After(timeNow),
						fmt.Sprintf("mismatched last access time for %v portal", portal))
				} else {
					assert.True(t, lastAccessTime.Before(timeNow),
						fmt.Sprintf("mismatched lass access time for %v portal", portal))
				}
			}

			if input.AddNewNodeOps {
				utils.Unlock(ctx, "test-lock1", lockID)

				// Wait for the lock to be released
				for utils.WaitQueueSize(lockID) > 1 {
					snooze(10)
				}

				// Give some time for another context to acquire the lock
				snooze(100)
				utils.Unlock(ctx, "test-lock2", lockID)
			}
		})
	}
}

func setRemediation(sessions *utils.ISCSISessions, portalActions []PortalAction) {
	for _, portalAction := range portalActions {
		sessions.Info[portalAction.Portal].Remediation = portalAction.Action
	}
}

func getPortals(sessions *utils.ISCSISessions, portalActions []PortalAction) []string {
	portals := make([]string, len(portalActions))

	for idx, portalAction := range portalActions {
		portals[idx] = portalAction.Portal
	}

	utils.SortPortals(portals, sessions)

	return portals
}

func mapCopyHelper(input map[int32]string) map[int32]string {
	output := make(map[int32]string, len(input))

	for key, value := range input {
		output[key] = value
	}

	return output
}

func structCopyHelper(input utils.ISCSISessionData) *utils.ISCSISessionData {
	clone, err := copystructure.Copy(input)
	if err != nil {
		return &utils.ISCSISessionData{}
	}

	output, ok := clone.(utils.ISCSISessionData)
	if !ok {
		return &utils.ISCSISessionData{}
	}

	return &output
}

func snooze(val uint32) {
	time.Sleep(time.Duration(val) * time.Millisecond)
}

func TestDiscoverDesiredPublicationState_GetsNoPublicationsWithoutError(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	var expectedPublications []*utils.VolumePublicationExternal

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().ListVolumePublicationsForNode(ctx, nodeName).Return(expectedPublications, nil)
	nodeServer := &Plugin{
		nodeName:          nodeName,
		role:              CSINode,
		restClient:        mockClient,
		enableForceDetach: true,
	}

	// desiredPublicationState is a mapping of volumes to volume publications.
	desiredPublicationState, err := nodeServer.discoverDesiredPublicationState(ctx)
	assert.NoError(t, err, "expected no error")
	assert.Empty(t, desiredPublicationState, "expected empty map")
}

func TestDiscoverDesiredPublicationState_GetsPublicationsWithoutError(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	expectedPublications := []*utils.VolumePublicationExternal{
		{
			Name:       utils.GenerateVolumePublishName("foo", nodeName),
			NodeName:   nodeName,
			VolumeName: "foo",
		},
		{
			Name:       utils.GenerateVolumePublishName("baz", nodeName),
			NodeName:   nodeName,
			VolumeName: "baz",
		},
	}

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().ListVolumePublicationsForNode(ctx, nodeName).Return(expectedPublications, nil)
	nodeServer := &Plugin{
		nodeName:          nodeName,
		role:              CSINode,
		restClient:        mockClient,
		enableForceDetach: true,
	}

	// desiredPublicationState is a mapping of volumes to volume publications.
	desiredPublicationState, err := nodeServer.discoverDesiredPublicationState(ctx)
	assert.NoError(t, err, "expected no error")
	for _, expectedPublication := range expectedPublications {
		desiredPublication, ok := desiredPublicationState[expectedPublication.VolumeName]
		assert.True(t, ok, "expected true value")
		assert.NotNil(t, desiredPublication, "expected publication to exist")
	}
}

func TestDiscoverDesiredPublicationState_FailsToGetPublicationsWithError(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	expectedPublications := []*utils.VolumePublicationExternal{
		{
			Name:       utils.GenerateVolumePublishName("foo", nodeName),
			NodeName:   nodeName,
			VolumeName: "foo",
		},
		{
			Name:       utils.GenerateVolumePublishName("baz", nodeName),
			NodeName:   nodeName,
			VolumeName: "baz",
		},
	}

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().ListVolumePublicationsForNode(ctx, nodeName).Return(
		expectedPublications,
		errors.New("failed to list volume publications"),
	)
	nodeServer := &Plugin{
		nodeName:          nodeName,
		role:              CSINode,
		restClient:        mockClient,
		enableForceDetach: true,
	}

	// desiredPublicationState is a mapping of volumes to volume publications.
	desiredPublicationState, err := nodeServer.discoverDesiredPublicationState(ctx)
	assert.Error(t, err, "expected error")
	assert.Empty(t, desiredPublicationState, "expected nil map")
}

func TestDiscoverActualPublicationState_FindsTrackingInfoWithoutError(t *testing.T) {
	ctx := context.Background()
	expectedPublicationState := map[string]*utils.VolumeTrackingInfo{
		"pvc-85987a99-648d-4d84-95df-47d0256ca2ab": {
			VolumePublishInfo: utils.VolumePublishInfo{},
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
				"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787dd/globalmount",
			PublishedPaths: map[string]struct{}{
				"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e3/volumes/kubernetes.io~csi/" +
					"pvc-85987a99-648d-4d84-95df-47d0256ca2ab/mount": {},
			},
		},
		"pvc-85987a99-648d-4d84-95df-47d0256ca2ac": {
			VolumePublishInfo: utils.VolumePublishInfo{},
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
				"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787de/globalmount",
			PublishedPaths: map[string]struct{}{
				"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e2/volumes/kubernetes.io~csi/" +
					"pvc-85987a99-648d-4d84-95df-47d0256ca2ac/mount": {},
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	mockHelper := mockNodeHelpers.NewMockNodeHelper(mockCtrl)
	mockHelper.EXPECT().ListVolumeTrackingInfo(ctx).Return(expectedPublicationState, nil)
	nodeServer := &Plugin{
		role:              CSINode,
		nodeHelper:        mockHelper,
		enableForceDetach: true,
	}

	// actualPublicationState is a mapping of volumes to volume publications.
	actualPublicationState, err := nodeServer.discoverActualPublicationState(ctx)
	assert.NoError(t, err, "expected no error")
	assert.NotEmptyf(t, actualPublicationState, "expected non-empty map")
	for volumeName, publicationState := range expectedPublicationState {
		actualPublication, ok := actualPublicationState[volumeName]
		assert.True(t, ok, "expected true")
		assert.NotNil(t, actualPublication, "expected non-nil publication state")
		for path := range publicationState.PublishedPaths {
			assert.Contains(t, path, volumeName)
		}
	}
}

func TestDiscoverActualPublicationState_FailsWithError(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockHelper := mockNodeHelpers.NewMockNodeHelper(mockCtrl)
	mockHelper.EXPECT().ListVolumeTrackingInfo(ctx).Return(nil, errors.New("not found"))
	nodeServer := &Plugin{
		role:              CSINode,
		nodeHelper:        mockHelper,
		enableForceDetach: true,
	}

	// actualPublicationState is a mapping of volumes to volume publications.
	actualPublicationState, err := nodeServer.discoverActualPublicationState(ctx)
	assert.Error(t, err, "expected error")
	assert.Nil(t, actualPublicationState, "expected nil map")
}

func TestDiscoverActualPublicationState_FailsToFindTrackingInfo(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockHelper := mockNodeHelpers.NewMockNodeHelper(mockCtrl)
	mockHelper.EXPECT().ListVolumeTrackingInfo(ctx).Return(nil, errors.NotFoundError("not found"))
	nodeServer := &Plugin{
		role:              CSINode,
		nodeHelper:        mockHelper,
		enableForceDetach: true,
	}

	// actualPublicationState is a mapping of volumes to volume publications.
	actualPublicationState, err := nodeServer.discoverActualPublicationState(ctx)
	assert.NoError(t, err, "expected no error")
	assert.Empty(t, actualPublicationState, "expected empty map")
}

func TestDiscoverStalePublications_DiscoversStalePublicationsCorrectly(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	volumeOne := "pvc-85987a99-648d-4d84-95df-47d0256ca2ab"
	volumeTwo := "pvc-85987a99-648d-4d84-95df-47d0256ca2ac"
	volumeThree := "pvc-85987a99-648d-4d84-95df-47d0256ca2ad"
	desiredPublicationState := map[string]*utils.VolumePublicationExternal{
		volumeOne: {
			Name:       utils.GenerateVolumePublishName(volumeOne, nodeName),
			NodeName:   nodeName,
			VolumeName: volumeOne,
		},
		// This shouldn't be counted as a stale publication.
		volumeThree: nil,
	}
	actualPublicationState := map[string]*utils.VolumeTrackingInfo{
		volumeOne: {
			VolumePublishInfo: utils.VolumePublishInfo{},
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
				"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787dd/globalmount",
			PublishedPaths: map[string]struct{}{
				"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e3/volumes/kubernetes.io~csi/" +
					volumeOne + "/mount": {},
			},
		},
		// This is what should be counted as "stale".
		volumeTwo: {
			VolumePublishInfo: utils.VolumePublishInfo{},
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
				"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787de/globalmount",
			PublishedPaths: map[string]struct{}{
				"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e2/volumes/kubernetes.io~csi/" +
					volumeTwo + "/mount": {},
			},
		},
	}

	nodeServer := &Plugin{
		role:              CSINode,
		nodeName:          nodeName,
		enableForceDetach: true,
	}

	stalePublications := nodeServer.discoverStalePublications(ctx, actualPublicationState, desiredPublicationState)
	assert.Contains(t, stalePublications, volumeTwo, fmt.Sprintf("expected %s to exist in stale publications", volumeTwo))
	assert.NotContains(t, stalePublications, volumeThree, fmt.Sprintf("expected %s to not exist in stale publications", volumeThree))
}

func TestPerformNodeCleanup_ShouldNotDiscoverAnyStalePublications(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	volume := "pvc-85987a99-648d-4d84-95df-47d0256ca2ab"
	desiredPublicationState := []*utils.VolumePublicationExternal{
		{
			Name:       utils.GenerateVolumePublishName(volume, nodeName),
			NodeName:   nodeName,
			VolumeName: volume,
		},
	}
	actualPublicationState := map[string]*utils.VolumeTrackingInfo{
		volume: {
			VolumePublishInfo: utils.VolumePublishInfo{},
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
				"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787dd/globalmount",
			PublishedPaths: map[string]struct{}{
				"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e3/volumes/kubernetes.io~csi/" +
					volume + "/mount": {},
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	mockRestClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(mockCtrl)
	mockRestClient.EXPECT().ListVolumePublicationsForNode(ctx, nodeName).Return(desiredPublicationState, nil)
	mockNodeHelper.EXPECT().ListVolumeTrackingInfo(ctx).Return(actualPublicationState, nil)

	nodeServer := &Plugin{
		role:              CSINode,
		nodeName:          nodeName,
		restClient:        mockRestClient,
		nodeHelper:        mockNodeHelper,
		enableForceDetach: true,
	}
	err := nodeServer.performNodeCleanup(ctx)
	assert.NoError(t, err, "expected no error")
}

func TestPerformNodeCleanup_ShouldFailToDiscoverDesiredPublicationsFromControllerAPI(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	volume := "pvc-85987a99-648d-4d84-95df-47d0256ca2ab"
	desiredPublicationState := []*utils.VolumePublicationExternal{
		{
			Name:       utils.GenerateVolumePublishName(volume, nodeName),
			NodeName:   nodeName,
			VolumeName: volume,
		},
	}

	mockCtrl := gomock.NewController(t)
	mockRestClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockRestClient.EXPECT().ListVolumePublicationsForNode(
		ctx, nodeName,
	).Return(desiredPublicationState, errors.New("api error"))

	nodeServer := &Plugin{
		role:              CSINode,
		nodeName:          nodeName,
		restClient:        mockRestClient,
		enableForceDetach: true,
	}
	err := nodeServer.performNodeCleanup(ctx)
	assert.Error(t, err, "expected an error")
}

func TestPerformNodeCleanup_ShouldFailToDiscoverActualPublicationsFromHost(t *testing.T) {
	ctx := context.Background()
	nodeName := "bar"
	volume := "pvc-85987a99-648d-4d84-95df-47d0256ca2ab"
	desiredPublicationState := []*utils.VolumePublicationExternal{
		{
			Name:       utils.GenerateVolumePublishName(volume, nodeName),
			NodeName:   nodeName,
			VolumeName: volume,
		},
	}
	actualPublicationState := map[string]*utils.VolumeTrackingInfo{
		volume: {
			VolumePublishInfo: utils.VolumePublishInfo{},
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/csi.trident.netapp.io/" +
				"6b1f46a23d50f8d6a2e2f24c63c3b6e73f82e8b982bdb41da4eb1d0b49d787dd/globalmount",
			PublishedPaths: map[string]struct{}{
				"/var/lib/kubelet/pods/b9f476af-47f4-42d8-8cfa-70d49394d9e3/volumes/kubernetes.io~csi/" +
					volume + "/mount": {},
			},
		},
	}

	mockCtrl := gomock.NewController(t)
	mockRestClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockNodeHelper := mockNodeHelpers.NewMockNodeHelper(mockCtrl)
	mockRestClient.EXPECT().ListVolumePublicationsForNode(ctx, nodeName).Return(desiredPublicationState, nil)
	mockNodeHelper.EXPECT().ListVolumeTrackingInfo(ctx).Return(actualPublicationState, errors.New("file I/O error"))

	nodeServer := &Plugin{
		role:              CSINode,
		nodeName:          nodeName,
		restClient:        mockRestClient,
		nodeHelper:        mockNodeHelper,
		enableForceDetach: true,
	}
	err := nodeServer.performNodeCleanup(ctx)
	assert.Error(t, err, "expected an error")
}

func TestUpdateNodePublicationState_NodeNotCleanable(t *testing.T) {
	ctx := context.Background()
	nodeState := utils.NodeDirty
	nodeServer := &Plugin{
		role:              CSINode,
		enableForceDetach: true,
	}

	err := nodeServer.updateNodePublicationState(ctx, nodeState)
	assert.NoError(t, err, "expected no error")

	nodeState = utils.NodeClean
	err = nodeServer.updateNodePublicationState(ctx, nodeState)
	assert.NoError(t, err, "expected no error")
}

func TestUpdateNodePublicationState_FailsToUpdateNodeAsCleaned(t *testing.T) {
	ctx := context.Background()
	nodeState := utils.NodeCleanable
	nodeName := "foo"
	nodeStateFlags := &utils.NodePublicationStateFlags{
		ProvisionerReady: utils.Ptr(true),
	}

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().UpdateNode(ctx, nodeName, nodeStateFlags).Return(errors.New("update failed"))
	nodeServer := &Plugin{
		role:              CSINode,
		nodeName:          nodeName,
		restClient:        mockClient,
		enableForceDetach: true,
	}

	err := nodeServer.updateNodePublicationState(ctx, nodeState)
	assert.Error(t, err, "expected error")
}

func TestUpdateNodePublicationState_SuccessfullyUpdatesNodeAsCleaned(t *testing.T) {
	ctx := context.Background()
	nodeState := utils.NodeCleanable
	nodeName := "foo"
	nodeStateFlags := &utils.NodePublicationStateFlags{
		ProvisionerReady: utils.Ptr(true),
	}

	mockCtrl := gomock.NewController(t)
	mockClient := mockControllerAPI.NewMockTridentController(mockCtrl)
	mockClient.EXPECT().UpdateNode(ctx, nodeName, nodeStateFlags).Return(nil)
	nodeServer := &Plugin{
		role:              CSINode,
		nodeName:          nodeName,
		restClient:        mockClient,
		enableForceDetach: true,
	}

	err := nodeServer.updateNodePublicationState(ctx, nodeState)
	assert.NoError(t, err, "expected no error")
}

func TestPerformNVMeSelfHealing(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockNVMeHandler := mockUtils.NewMockNVMeInterface(mockCtrl)
	nodeServer := &Plugin{nvmeHandler: mockNVMeHandler}

	// Empty Published sessions case.
	nodeServer.performNVMeSelfHealing(ctx)

	// Error populating current sessions.
	publishedNVMeSessions.AddNVMeSession(utils.NVMeSubsystem{NQN: "nqn"}, []string{})
	mockNVMeHandler.EXPECT().PopulateCurrentNVMeSessions(ctx, gomock.Any()).
		Return(errors.New("failed to populate current sessions"))

	nodeServer.performNVMeSelfHealing(ctx)

	// Self-healing process done.
	mockNVMeHandler.EXPECT().PopulateCurrentNVMeSessions(ctx, gomock.Any()).Return(nil)
	mockNVMeHandler.EXPECT().InspectNVMeSessions(ctx, gomock.Any(), gomock.Any()).Return([]utils.NVMeSubsystem{})

	nodeServer.performNVMeSelfHealing(ctx)
	// Cleanup of global objects.
	publishedNVMeSessions.RemoveNVMeSession("nqn")
}

func TestFixNVMeSessions(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockNVMeHandler := mockUtils.NewMockNVMeInterface(mockCtrl)
	nodeServer := &Plugin{nvmeHandler: mockNVMeHandler}
	subsystem1 := utils.NVMeSubsystem{NQN: "nqn1"}
	subsystems := []utils.NVMeSubsystem{subsystem1}

	// Subsystem not present in published sessions case.
	nodeServer.fixNVMeSessions(ctx, time.UnixMicro(0), subsystems)

	// Rectify NVMe session.
	publishedNVMeSessions.AddNVMeSession(subsystem1, []string{})
	mockNVMeHandler.EXPECT().RectifyNVMeSession(ctx, gomock.Any(), gomock.Any())

	nodeServer.fixNVMeSessions(ctx, time.UnixMicro(0), subsystems)
	// Cleanup of global objects.
	publishedNVMeSessions.RemoveNVMeSession(subsystem1.NQN)
}
