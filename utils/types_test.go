/*
 * Copyright (c) 2022 NetApp, Inc. All Rights Reserved.
 */

package utils

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var (
	fakeVolumePublication = &VolumePublication{
		Name:       "node1volume1",
		VolumeName: "volume1",
		NodeName:   "node1",
		ReadOnly:   true,
		AccessMode: 1,
	}

	fakeNode = &Node{
		Name:           "node1",
		IQN:            "fakeIQN",
		IPs:            []string{"10.10.10.10", "10.10.10.20"},
		TopologyLabels: map[string]string{"region": "region1", "zone": "zone1"},
		HostInfo: &HostSystem{
			OS: SystemOS{
				Distro:  "ubuntu",
				Release: "16.04.7",
				Version: "16.04",
			},
			Services: []string{"NFS", "iSCSI"},
		},
		Deleted:          false,
		PublicationState: NodeClean,
	}
)

func TestVolumePublicationCopy(t *testing.T) {
	result := fakeVolumePublication.Copy()
	assert.Equal(t, fakeVolumePublication, result, "Volume publication copy does not match.")
}

func TestVolumePublicationConstructExternal(t *testing.T) {
	expectedPub := &VolumePublicationExternal{
		Name:       "node1volume1",
		VolumeName: "volume1",
		NodeName:   "node1",
		ReadOnly:   true,
		AccessMode: 1,
	}

	result := fakeVolumePublication.ConstructExternal()

	assert.True(t, reflect.DeepEqual(expectedPub, result), "External volume publication does not match.")
}

func TestNodeCopy(t *testing.T) {
	result := fakeNode.Copy()

	assert.Equal(t, fakeNode, result, "Node copy does not match.")
}

func TestNodeConstructExternal(t *testing.T) {
	expectedNode := &NodeExternal{
		Name:           "node1",
		IQN:            "fakeIQN",
		IPs:            []string{"10.10.10.10", "10.10.10.20"},
		TopologyLabels: map[string]string{"region": "region1", "zone": "zone1"},
		HostInfo: &HostSystem{
			OS: SystemOS{
				Distro:  "ubuntu",
				Release: "16.04.7",
				Version: "16.04",
			},
			Services: []string{"NFS", "iSCSI"},
		},
		Deleted:          Ptr(false),
		PublicationState: NodeClean,
	}

	result := fakeNode.ConstructExternal()

	assert.True(t, reflect.DeepEqual(expectedNode, result), "External node does not match.")

	// Ensure publication state is always set
	nodeCopy := fakeNode.Copy()
	nodeCopy.PublicationState = ""

	result = nodeCopy.ConstructExternal()

	assert.True(t, reflect.DeepEqual(expectedNode, result), "External node does not match.")
}

func TestISCSIAction(t *testing.T) {
	assert.Equal(t, NoAction.String(), "no action", "String output mismatch")
	assert.Equal(t, Scan.String(), "LUN scanning", "String output mismatch")
	assert.Equal(t, LoginScan.String(), "login and LUN scanning", "String output mismatch")
	assert.Equal(t, LogoutLoginScan.String(), "re-login and LUN scanning", "String output mismatch")
	assert.Equal(t, ISCSIAction(10).String(), "unknown", "String output mismatch")
}

func TestPortalInvalid(t *testing.T) {
	assert.Equal(t, NotInvalid.String(), "not invalid", "String output mismatch")
	assert.Equal(t, MissingTargetIQN.String(), "missing target IQN", "String output mismatch")
	assert.Equal(t, MissingMpathDevice.String(), "missing multipath device", "String output mismatch")
	assert.Equal(t, DuplicatePortals.String(), "duplicate portals found", "String output mismatch")
	assert.Equal(t, PortalInvalid(10).String(), "unknown", "String output mismatch")
}

func TestLUNs(t *testing.T) {
	LUNdata := []LUNData{
		{0, "volID-0"},
		{1, "volID-1"},
		{2, ""},
	}

	for _, input := range LUNdata {
		// Before adding a LUN
		var l LUNs
		helperTestLUNWhenNotFound(t, l, input)

		// After adding a LUN
		l.AddLUN(input)
		helperTestLUNWhenFound(t, l, input)

		// After adding the same LUN twice - no impact
		l.AddLUN(input)
		helperTestLUNWhenFound(t, l, input)

		// Remove an invalid LUN
		l.RemoveLUN(999999)
		helperTestLUNWhenFound(t, l, input)

		// After removing a valid LUN
		l.RemoveLUN(input.LUN)
		helperTestLUNWhenNotFound(t, l, input)
	}
}

func helperTestLUNWhenNotFound(t *testing.T, l LUNs, input LUNData) {
	assert.False(t, l.CheckLUNExists(input.LUN), "LUN already exist")

	volID, err := l.VolumeID(input.LUN)
	assert.True(t, IsNotFoundError(err), "Expecting volume ID not found error")
	assert.Empty(t, volID, "volume ID should be empty")

	assert.True(t, l.IsEmpty(), "LUN info should be empty")
	assert.Equal(t, len(l.AllLUNs()), 0, "LUN info should return 0 LUNs")
	assert.Equal(t, l.String(), "LUNs: []", "String format mismatch")
}

func helperTestLUNWhenFound(t *testing.T, l LUNs, input LUNData) {
	assert.True(t, l.CheckLUNExists(input.LUN), "LUN not found")

	volID, err := l.VolumeID(input.LUN)
	assert.Nil(t, err, "unable to get volume ID")
	assert.Equal(t, volID, input.VolID, "volume ID mismatch error")

	assert.False(t, l.IsEmpty(), "LUN info is empty")
	assert.Equal(t, len(l.AllLUNs()), len(l.Info), "LUN info length mismatch")
	assert.Equal(t, l.String(), fmt.Sprintf("LUNs: [%d]", input.LUN), "String format mismatch")
}

func TestLUNs_IdentifyMissingLUNs(t *testing.T) {
	LUNdata1 := []LUNData{
		{0, "volID-0"},
		{1, "volID-1"},
		{2, ""},
		{3, "volID-99"},
	}

	LUNdata2 := []LUNData{
		{0, "volID-0"},
		{0o0, "volID-00"},
		{1, ""},
		{0o1, ""},
		{2, "volID-1"},
		{4, "volID-4"},
		{99, "volID-99"},
	}

	var l LUNs
	for _, input := range LUNdata1 {
		l.AddLUN(input)
	}

	var m LUNs
	for _, input := range LUNdata2 {
		m.AddLUN(input)
	}

	var n LUNs

	assert.Equal(t, []int32{}, l.IdentifyMissingLUNs(n), "Missing LUN mismatch when input is empty")
	containsAll, _ := SliceContainsElements([]int32{4, 99}, l.IdentifyMissingLUNs(m))
	assert.True(t, containsAll, "Missing LUN mismatch")
	containsAll, _ = SliceContainsElements(l.AllLUNs(), n.IdentifyMissingLUNs(l))
	assert.True(t, containsAll, "Missing LUN mismatch when struct is not initalized or empty")
}

func TestPortalInfo(t *testing.T) {
	type PortalInfoCheck struct {
		portalInfo   PortalInfo
		isValid      bool
		hasTargetIQN bool
		chapInUse    bool
		staleTimeSet bool
	}

	inputs := []PortalInfoCheck{
		{
			portalInfo: PortalInfo{
				ISCSITargetIQN: "IQN1",
				ReasonInvalid:  NotInvalid,
			},
			isValid:      true,
			hasTargetIQN: true,
			chapInUse:    false,
			staleTimeSet: false,
		},
		{
			portalInfo: PortalInfo{
				ISCSITargetIQN:         "",
				Credentials:            IscsiChapInfo{},
				ReasonInvalid:          NotInvalid,
				FirstIdentifiedStaleAt: time.Now(),
			},
			isValid:      true,
			hasTargetIQN: false,
			chapInUse:    false,
			staleTimeSet: true,
		},
		{
			portalInfo: PortalInfo{
				ISCSITargetIQN: "IQN2",
				Credentials: IscsiChapInfo{
					UseCHAP: false,
				},
				ReasonInvalid:          MissingTargetIQN,
				FirstIdentifiedStaleAt: time.Now(),
			},
			isValid:      false,
			hasTargetIQN: true,
			chapInUse:    false,
			staleTimeSet: true,
		},
		{
			portalInfo: PortalInfo{
				ISCSITargetIQN: "IQN2",
				Credentials: IscsiChapInfo{
					UseCHAP: true,
				},
				ReasonInvalid:          MissingTargetIQN,
				FirstIdentifiedStaleAt: time.Time{},
			},
			isValid:      false,
			hasTargetIQN: true,
			chapInUse:    true,
			staleTimeSet: false,
		},
	}

	newCredentials := IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "username1",
		IscsiInitiatorSecret: "secret1",
		IscsiTargetUsername:  "username2",
		IscsiTargetSecret:    "secret2",
	}

	for _, input := range inputs {
		assert.Equal(t, input.isValid, input.portalInfo.IsValid(), "IsValid value mismatch")
		assert.Equal(t, input.hasTargetIQN, input.portalInfo.HasTargetIQN(), "HasTargetIQN value mismatch")
		assert.Equal(t, input.chapInUse, input.portalInfo.CHAPInUse(), "CHAPInUse value mismatch")
		assert.Equal(t, input.staleTimeSet, input.portalInfo.IsFirstIdentifiedStaleAtSet(),
			"IsFirstIdentifiedStaleAtSet value mismatch")

		// Update PortalInfo
		input.portalInfo.ResetFirstIdentifiedStaleAt()
		input.portalInfo.UpdateCHAPCredentials(newCredentials)
		assert.Equal(t, true, input.portalInfo.CHAPInUse(), "CHAPInUse value should be true")
		assert.Equal(t, false, input.portalInfo.IsFirstIdentifiedStaleAtSet(),
			"IsFirstIdentifiedStaleAtSet value should return false")
	}
}

func TestPortalInfo_RecordChanges(t *testing.T) {
	basePortalInfo := PortalInfo{
		ISCSITargetIQN:         "IQN1",
		SessionNumber:          "1",
		Credentials:            IscsiChapInfo{},
		ReasonInvalid:          NotInvalid,
		LastAccessTime:         time.Time{},
		FirstIdentifiedStaleAt: time.Time{},
	}

	newPortal := basePortalInfo

	newPortal.ISCSITargetIQN = "IQN2"
	assert.Contains(t, basePortalInfo.RecordChanges(newPortal), "TargetIQN changed")

	newPortal.SessionNumber = "2"
	assert.Contains(t, basePortalInfo.RecordChanges(newPortal), "Session number changed")

	newPortal.ReasonInvalid = MissingMpathDevice
	assert.Contains(t, basePortalInfo.RecordChanges(newPortal), "Reason invalid changed")

	newPortal.Credentials = IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "username1",
		IscsiInitiatorSecret: "secret1",
		IscsiTargetUsername:  "username2",
		IscsiTargetSecret:    "secret2",
	}
	portalChanges := basePortalInfo.RecordChanges(newPortal)
	assert.Contains(t, portalChanges, "CHAP changed from 'false' to 'true'")
	assert.Contains(t, portalChanges, "CHAP credentials have changed")

	// Ensure record change did not override any of the values
	assert.Contains(t, basePortalInfo.String(), "IQN1")
	assert.Contains(t, basePortalInfo.String(), "1")
	assert.Contains(t, basePortalInfo.String(), "not invalid")
	assert.Contains(t, basePortalInfo.String(), "useCHAP: false")
}

func TestISCSISessions_IsEmpty(t *testing.T) {
	var iSCSISessionsEmpty ISCSISessions
	iSCSISessionNotEmpty, _, _, _, portal := helperISCSISessionsCreateInputs()
	iSCSISessionsEmpty2 := ISCSISessions{
		Info: map[string]*ISCSISessionData{},
	}

	iSCSISessionsWithJustPortal1 := ISCSISessions{
		Info: map[string]*ISCSISessionData{
			portal: nil,
		},
	}

	assert.True(t, iSCSISessionsEmpty.IsEmpty(), "iSCSISessions should be empty")
	assert.True(t, iSCSISessionsEmpty2.IsEmpty(), "iSCSISessions should be empty")
	assert.False(t, iSCSISessionsWithJustPortal1.IsEmpty(), "iSCSISessions should NOT be empty")
	assert.False(t, iSCSISessionNotEmpty.IsEmpty(), "iSCSISessions should NOT be empty")
}

func TestISCSISessions_ISCSISessionData(t *testing.T) {
	var iSCSISessionsEmpty ISCSISessions
	iSCSISessionNotEmpty, iSCSISessionData, _, _, portal := helperISCSISessionsCreateInputs()

	// Try with empty iSCSISessions
	sessionData, err := iSCSISessionsEmpty.ISCSISessionData(portal)
	assert.NotNil(t, err, "empty session should return error when ISCSISessionData is called")

	// Try with empty portal value
	sessionData, err = iSCSISessionsEmpty.ISCSISessionData("")
	assert.NotNil(t, err, "empty portal should return error when ISCSISessionData is called")

	// Try non-existent portal
	sessionData, err = iSCSISessionNotEmpty.ISCSISessionData("5.6.7.8")
	assert.True(t, IsNotFoundError(err), "Non-empty session should return NotFoundError")

	// Try existing portal
	sessionData, err = iSCSISessionNotEmpty.ISCSISessionData(portal)
	assert.Nil(t, err, "Non-empty session should NOT return error when ISCSISessionData is called")
	assert.Equal(t, *sessionData, iSCSISessionData, "iSCSISessions should NOT be empty")
}

func TestISCSISessions_AddPortal(t *testing.T) {
	var iSCSISessionsEmpty, iSCSISessionsEmpty2, iSCSISessionsWithJustPortal1 ISCSISessions
	var iSCSISessionsUninitialized *ISCSISessions
	iSCSISessionNotEmpty, _, portalInfo, _, portal := helperISCSISessionsCreateInputs()

	iSCSISessionsEmpty2 = ISCSISessions{
		Info: map[string]*ISCSISessionData{},
	}

	iSCSISessionsWithJustPortal1 = ISCSISessions{
		Info: map[string]*ISCSISessionData{
			portal: nil,
		},
	}

	// Test AddPortal when ISCSISessions is uninitialized
	assert.NotNil(t, iSCSISessionsUninitialized.AddPortal(portal, portalInfo), "expecting uninitialized error")

	// Test AddPortal without IQN
	portalInfoNoTargetIQN := portalInfo
	portalInfoNoTargetIQN.ISCSITargetIQN = ""
	assert.NotNil(t, iSCSISessionsEmpty.AddPortal(portal, portalInfoNoTargetIQN), "expecting target IQN error")
	assert.NotNil(t, iSCSISessionNotEmpty.AddPortal(portal, portalInfoNoTargetIQN), "expecting target IQN error")

	// Test AddPortal without portal value
	assert.NotNil(t, iSCSISessionsEmpty.AddPortal("", portalInfo), "expecting portal add to fail")
	assert.NotNil(t, iSCSISessionsEmpty.AddPortal("   ", portalInfo), "expecting portal add to fail")

	// Test AddPortal when portal entry already exist
	assert.NotNil(t, iSCSISessionNotEmpty.AddPortal(portal, portalInfo), "expecting already exists error")
	assert.NotNil(t, iSCSISessionsWithJustPortal1.AddPortal(portal, portalInfo), "expecting already exists error")

	// Test AddPortal with a valid portal entry
	assert.Nil(t, iSCSISessionsEmpty.AddPortal(portal, portalInfo), "expecting portal add to succeed")
	assert.Nil(t, iSCSISessionsEmpty2.AddPortal(portal, portalInfo), "expecting portal add to succeed")
	assert.Nil(t, iSCSISessionNotEmpty.AddPortal("5.6.7.8", portalInfo), "expecting portal add to succeed")
}

func TestISCSISessions_UpdateAndRecordPortalInfoChanges(t *testing.T) {
	var iSCSISessionsEmpty, iSCSISessionsWithJustPortal ISCSISessions
	var iSCSISessionsUninitialized *ISCSISessions
	iSCSISessionNotEmpty, _, portalInfo, _, portal := helperISCSISessionsCreateInputs()

	iSCSISessionsWithJustPortal = ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"5.6.7.8": nil,
			"":        {},
		},
	}

	// Test UpdateAndRecordPortal when ISCSISessions is uninitialized
	changes, err := iSCSISessionsUninitialized.UpdateAndRecordPortalInfoChanges(portal, portalInfo)
	assert.NotNil(t, err, "expecting error")

	// Test UpdateAndRecordPortal without IQN
	portalInfoNoTargetIQN := portalInfo
	portalInfoNoTargetIQN.ISCSITargetIQN = ""
	changes, err = iSCSISessionsEmpty.UpdateAndRecordPortalInfoChanges(portal, portalInfoNoTargetIQN)
	assert.NotNil(t, err, "expecting target IQN error")
	changes, err = iSCSISessionNotEmpty.UpdateAndRecordPortalInfoChanges(portal, portalInfoNoTargetIQN)
	assert.NotNil(t, err, "expecting target IQN error")
	changes, err = iSCSISessionsWithJustPortal.UpdateAndRecordPortalInfoChanges(portal, portalInfoNoTargetIQN)
	assert.NotNil(t, err, "expecting target IQN error")

	// Test UpdateAndRecordPortal without portal value
	changes, err = iSCSISessionNotEmpty.UpdateAndRecordPortalInfoChanges("", portalInfo)
	assert.NotNil(t, err, "expecting portal update to fail")
	changes, err = iSCSISessionNotEmpty.UpdateAndRecordPortalInfoChanges("   ", portalInfo)
	assert.NotNil(t, err, "expecting portal update to fail")

	// Test UpdateAndRecordPortal when portal entry does not exist
	changes, err = iSCSISessionsEmpty.UpdateAndRecordPortalInfoChanges("5.6.7.8", portalInfo)
	assert.NotNil(t, err, "expecting target IQN error")
	changes, err = iSCSISessionNotEmpty.UpdateAndRecordPortalInfoChanges("5.6.7.8", portalInfo)
	assert.NotNil(t, err, "expecting not found error")

	// Test UpdateAndRecordPortal with valid entries
	changes, err = iSCSISessionNotEmpty.UpdateAndRecordPortalInfoChanges(portal, portalInfo)
	assert.Nil(t, err, "expecting no error")
	assert.Empty(t, changes, "No changes expected")
	changes, err = iSCSISessionsWithJustPortal.UpdateAndRecordPortalInfoChanges("5.6.7.8", portalInfo)
	assert.Nil(t, err, "expecting no error")
	assert.NotEmpty(t, changes, "Changes expected")
}

func TestISCSISessions_UpdateCHAPForPortal(t *testing.T) {
	var iSCSISessionsEmpty ISCSISessions
	iSCSISessionNotEmpty, _, _, _, portal := helperISCSISessionsCreateInputs()

	iSCSISessionsWithJustPortal := ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"5.6.7.8": nil,
		},
	}

	iSCSISessionNotEmpty.Info["5.6.7.8"] = &ISCSISessionData{
		PortalInfo: PortalInfo{
			ISCSITargetIQN:         "",
			SessionNumber:          "2",
			Credentials:            IscsiChapInfo{},
			ReasonInvalid:          NotInvalid,
			LastAccessTime:         time.Time{},
			FirstIdentifiedStaleAt: time.Time{},
		},
		LUNs: LUNs{},
	}

	newCredentials := IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "user1",
		IscsiInitiatorSecret: "password1",
		IscsiTargetUsername:  "user2",
		IscsiTargetSecret:    "password2",
	}

	// Negative Scenarios
	assert.NotNil(t, iSCSISessionsEmpty.UpdateCHAPForPortal(portal, newCredentials), "expected CHAP update to fail")
	assert.NotNil(t, iSCSISessionsWithJustPortal.UpdateCHAPForPortal("5.6.7.8", newCredentials),
		"expected CHAP update to fail")
	assert.NotNil(t, iSCSISessionNotEmpty.UpdateCHAPForPortal("5.6.7.8", newCredentials),
		"expected CHAP update to fail because of missing target IQN")

	// Positive Scenarios
	assert.Nil(t, iSCSISessionNotEmpty.UpdateCHAPForPortal(portal, newCredentials),
		"expected CHAP update to pass")
	assert.True(t, iSCSISessionNotEmpty.Info[portal].PortalInfo.Credentials.UseCHAP == newCredentials.UseCHAP)
	assert.True(t,
		iSCSISessionNotEmpty.Info[portal].PortalInfo.Credentials.IscsiUsername == newCredentials.IscsiUsername)
	assert.True(t,
		iSCSISessionNotEmpty.Info[portal].PortalInfo.Credentials.IscsiInitiatorSecret == newCredentials.IscsiInitiatorSecret)
	assert.True(t,
		iSCSISessionNotEmpty.Info[portal].PortalInfo.Credentials.IscsiTargetUsername == newCredentials.IscsiTargetUsername)
	assert.True(t,
		iSCSISessionNotEmpty.Info[portal].PortalInfo.Credentials.IscsiTargetSecret == newCredentials.IscsiTargetSecret)
}

func TestISCSISessions_AddLUNToPortal(t *testing.T) {
	var iSCSISessionsEmpty ISCSISessions
	iSCSISessionNotEmpty, _, _, _, portal := helperISCSISessionsCreateInputs()

	iSCSISessionsWithJustPortal := ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"5.6.7.8": nil,
		},
	}

	iSCSISessionNotEmpty.Info["5.6.7.8"] = &ISCSISessionData{
		PortalInfo: PortalInfo{
			ISCSITargetIQN:         "",
			SessionNumber:          "2",
			Credentials:            IscsiChapInfo{},
			ReasonInvalid:          NotInvalid,
			LastAccessTime:         time.Time{},
			FirstIdentifiedStaleAt: time.Time{},
		},
	}

	iSCSISessionNotEmpty.Info["9.10.11.12"] = &ISCSISessionData{
		PortalInfo: PortalInfo{
			ISCSITargetIQN:         "IQN2",
			SessionNumber:          "2",
			Credentials:            IscsiChapInfo{},
			ReasonInvalid:          NotInvalid,
			LastAccessTime:         time.Time{},
			FirstIdentifiedStaleAt: time.Time{},
		},
	}

	newLUNData := LUNData{
		LUN:   1,
		VolID: "volID-1",
	}

	// Negative Scenarios
	assert.NotNil(t, iSCSISessionsEmpty.AddLUNToPortal(portal, newLUNData), "expected LUN add to fail")
	assert.NotNil(t, iSCSISessionsWithJustPortal.AddLUNToPortal("5.6.7.8", newLUNData),
		"expected LUN add to fail")
	assert.NotNil(t, iSCSISessionNotEmpty.AddLUNToPortal("5.6.7.8", newLUNData),
		"expected LUN add to fail because of missing target IQN")

	// Positive Scenarios
	assert.Nil(t, iSCSISessionNotEmpty.AddLUNToPortal("9.10.11.12", newLUNData),
		"expected LUN add to pass")
	assert.Nil(t, iSCSISessionNotEmpty.AddLUNToPortal(portal, newLUNData),
		"expected LUN add to pass")
}

func TestISCSISessions_RemoveLUNFromPortal(t *testing.T) {
	var iSCSISessionsEmpty, iSCSISessionsWithJustPortal ISCSISessions
	iSCSISessionNotEmpty, _, portalInfo, _, portal := helperISCSISessionsCreateInputs()

	iSCSISessionsWithJustPortal = ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"5.6.7.8":    nil,
			"9.10.11.12": {},
		},
	}

	iSCSISessionNotEmpty.Info[portal] = &ISCSISessionData{
		PortalInfo: portalInfo,
		LUNs: LUNs{Info: map[int32]string{
			1: "volID-1",
			2: "volID-2",
		}},
	}

	// Remove LUN from empty Sessions
	iSCSISessionsEmpty.RemoveLUNFromPortal(portal, 1)
	assert.True(t, iSCSISessionsEmpty.IsEmpty(), "expected Sessions to be empty")

	// Remove LUN from Portal with nil SessionData
	iSCSISessionsWithJustPortal.RemoveLUNFromPortal("5.6.7.8", 1)
	iSCSISessionData, ok := iSCSISessionsWithJustPortal.Info["5.6.7.8"]
	assert.True(t, ok, "expected to find the portal")
	assert.Nil(t, iSCSISessionData, "expected iSCSISessionData to be nil")
	assert.False(t, iSCSISessionsWithJustPortal.IsEmpty(), "expected Sessions to be NOT empty")

	// Remove LUN from Portal with empty SessionData
	iSCSISessionsWithJustPortal.RemoveLUNFromPortal("9.10.11.12", 1)
	iSCSISessionData, ok = iSCSISessionsWithJustPortal.Info["9.10.11.12"]
	assert.False(t, ok, "expected to NOT find the portal")
	assert.Nil(t, iSCSISessionData, "expected iSCSISessionData to be nil")
	assert.False(t, iSCSISessionsWithJustPortal.IsEmpty(), "expected Sessions to be NOT empty")

	// Remove LUN from not existent portal
	iSCSISessionNotEmpty.RemoveLUNFromPortal("9.10.11.12", 1)
	iSCSISessionData, ok = iSCSISessionNotEmpty.Info["9.10.11.12"]
	assert.False(t, ok, "expected to NOT find the portal")
	assert.Nil(t, iSCSISessionData, "expected iSCSISessionData to be nil")
	assert.False(t, iSCSISessionNotEmpty.IsEmpty(), "expected Sessions to be NOT empty")

	// Remove LUN from existing portal but NOT last LUN
	iSCSISessionNotEmpty.RemoveLUNFromPortal(portal, 1)
	iSCSISessionData, ok = iSCSISessionNotEmpty.Info[portal]
	assert.True(t, ok, "expected to find the portal")
	assert.NotNil(t, iSCSISessionData, "expected iSCSISessionData to be NOT nil")
	assert.False(t, iSCSISessionNotEmpty.IsEmpty(), "expected Sessions to be NOT empty")

	// Remove LUN from existing portal but last LUN
	iSCSISessionNotEmpty.RemoveLUNFromPortal(portal, 2)
	iSCSISessionData, ok = iSCSISessionNotEmpty.Info[portal]
	assert.False(t, ok, "expected to NOT find the portal")
	assert.Nil(t, iSCSISessionData, "expected iSCSISessionData to be nil")
	assert.True(t, iSCSISessionNotEmpty.IsEmpty(), "expected Sessions to be empty")
}

func TestISCSISessions_RemovePortal(t *testing.T) {
	var iSCSISessionsEmpty, iSCSISessionsWithJustPortal ISCSISessions
	var iSCSISessionsUninitialized *ISCSISessions
	iSCSISessionNotEmpty, _, _, _, portal := helperISCSISessionsCreateInputs()

	iSCSISessionsWithJustPortal = ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"5.6.7.8": nil,
		},
	}

	// Remove Portal from uninitialized Sessions
	iSCSISessionsUninitialized.RemovePortal(portal)
	assert.True(t, iSCSISessionsUninitialized.IsEmpty(), "expected Sessions to be empty")

	// Remove Portal from empty Sessions
	iSCSISessionsEmpty.RemovePortal(portal)
	assert.True(t, iSCSISessionsEmpty.IsEmpty(), "expected Sessions to be empty")

	// Remove not existent portal
	iSCSISessionsWithJustPortal.RemovePortal("9.10.11.12")
	_, ok := iSCSISessionsWithJustPortal.Info["9.10.11.12"]
	assert.False(t, ok, "expected NOT to find the portal")
	assert.False(t, iSCSISessionsWithJustPortal.IsEmpty(), "expected Sessions to be NOT empty")

	// Remove Portal with nil SessionData
	assert.True(t, iSCSISessionsWithJustPortal.CheckPortalExists("5.6.7.8"), "expected to find the portal")
	iSCSISessionsWithJustPortal.RemovePortal("5.6.7.8")
	assert.False(t, iSCSISessionsWithJustPortal.CheckPortalExists("5.6.7.8"), "expected NOT to find the portal")
	assert.True(t, iSCSISessionsWithJustPortal.IsEmpty(), "expected Sessions to be empty")

	// Remove a valid Portal with multiple LUNs
	assert.True(t, iSCSISessionNotEmpty.CheckPortalExists(portal), "expected to find the portal")
	iSCSISessionNotEmpty.RemovePortal(portal)
	assert.False(t, iSCSISessionNotEmpty.CheckPortalExists(portal), "expected NOT to find the portal")
	assert.True(t, iSCSISessionNotEmpty.IsEmpty(), "expected Sessions to be empty")
}

func TestISCSISessions_PortalInfo(t *testing.T) {
	var iSCSISessionsEmpty, iSCSISessionsWithJustPortal ISCSISessions
	iSCSISessionNotEmpty, _, inputPortalInfo, _, portal := helperISCSISessionsCreateInputs()

	iSCSISessionsWithJustPortal = ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"5.6.7.8":    nil,
			"9.10.11.12": {},
		},
	}

	// Get PortalInfo from empty Sessions
	portalInfo, err := iSCSISessionsEmpty.PortalInfo(portal)
	assert.NotNil(t, err, "expected error when getting portalInfo")

	// Get PortalInfo for Portal with nil SessionData
	portalInfo, err = iSCSISessionsWithJustPortal.PortalInfo("5.6.7.8")
	assert.NotNil(t, err, "expected error when getting portalInfo")

	// Get PortalInfo for Portal with empty SessionData
	portalInfo, err = iSCSISessionsWithJustPortal.PortalInfo("9.10.11.12")
	assert.Nil(t, err, "expected no error when getting portalInfo")
	assert.Empty(t, *portalInfo, "expected portalInfo to be empty")

	// Get PortalInfo for non existent portal
	portalInfo, err = iSCSISessionNotEmpty.PortalInfo("9.10.11.12")
	assert.NotNil(t, err, "expected error when getting portalInfo")

	// Get PortalInfo for existing portal with a valid SessionData
	portalInfo, err = iSCSISessionNotEmpty.PortalInfo(portal)
	assert.Nil(t, err, "expected no error when getting portalInfo")
	assert.NotEmpty(t, *portalInfo, "expected portalInfo to be NOT empty")
	assert.Equal(t, inputPortalInfo, *portalInfo, "expected PortalInfos to be equal")
}

func TestISCSISessions_LUNInfo(t *testing.T) {
	var iSCSISessionsEmpty, iSCSISessionsWithJustPortal ISCSISessions
	iSCSISessionNotEmpty, _, _, inputLUNInfo, portal := helperISCSISessionsCreateInputs()

	iSCSISessionsWithJustPortal = ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"5.6.7.8":    nil,
			"9.10.11.12": {},
		},
	}

	// Get LUNInfo from empty Sessions
	lunInfo, err := iSCSISessionsEmpty.LUNInfo(portal)
	assert.NotNil(t, err, "expected error when getting LUNInfo")

	// Get LUNInfo for Portal with nil SessionData
	lunInfo, err = iSCSISessionsWithJustPortal.LUNInfo("5.6.7.8")
	assert.NotNil(t, err, "expected error when getting LUNInfo")

	// Get LUNInfo for Portal with empty SessionData
	lunInfo, err = iSCSISessionsWithJustPortal.LUNInfo("9.10.11.12")
	assert.Nil(t, err, "expected no error when getting LUNInfo")
	assert.Empty(t, *lunInfo, "expected LUNInfo to be empty")

	// Get LUNInfo for non existent portal
	lunInfo, err = iSCSISessionNotEmpty.LUNInfo("9.10.11.12")
	assert.NotNil(t, err, "expected error when getting LUNInfo")

	// Get LUNInfo for existing portal with a valid SessionData
	lunInfo, err = iSCSISessionNotEmpty.LUNInfo(portal)
	assert.Nil(t, err, "expected no error when getting LUNInfo")
	assert.NotEmpty(t, *lunInfo, "expected LUNInfo to be NOT empty")
	assert.Equal(t, inputLUNInfo, *lunInfo, "expected LUNInfos to be equal")
}

func TestISCSISessions_LUNsForPortal(t *testing.T) {
	var iSCSISessionsEmpty, iSCSISessionsWithJustPortal ISCSISessions
	iSCSISessionNotEmpty, _, _, inputLUNInfo, portal := helperISCSISessionsCreateInputs()

	iSCSISessionsWithJustPortal = ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"5.6.7.8":    nil,
			"9.10.11.12": {},
		},
	}

	// Get LUNsForPortal from empty Sessions
	luns, err := iSCSISessionsEmpty.LUNsForPortal(portal)
	assert.NotNil(t, err, "expected error when getting LUNsForPortal")

	// Get LUNsForPortal for Portal with nil SessionData
	luns, err = iSCSISessionsWithJustPortal.LUNsForPortal("5.6.7.8")
	assert.NotNil(t, err, "expected error when getting LUNsForPortal")

	// Get LUNsForPortal for Portal with empty SessionData
	luns, err = iSCSISessionsWithJustPortal.LUNsForPortal("9.10.11.12")
	assert.Nil(t, err, "expected no error when getting LUNsForPortal")
	assert.Empty(t, luns, "expected LUNsForPortal to be empty")

	// Get LUNsForPortal for non existent portal
	luns, err = iSCSISessionNotEmpty.LUNsForPortal("9.10.11.12")
	assert.NotNil(t, err, "expected error when getting LUNsForPortal")

	// Get LUNsForPortal for existing portal with a valid SessionData
	luns, err = iSCSISessionNotEmpty.LUNsForPortal(portal)
	assert.Nil(t, err, "expected no error when getting LUNsForPortal")
	assert.NotEmpty(t, luns, "expected LUNsForPortal to be NOT empty")
	containsAll, _ := SliceContainsElements(luns, inputLUNInfo.AllLUNs())
	assert.True(t, containsAll, "expected LUNs to be equal")
}

func TestISCSISessions_VolumeIDForPortal(t *testing.T) {
	var iSCSISessionsEmpty, iSCSISessionsWithJustPortal ISCSISessions
	iSCSISessionNotEmpty, _, _, _, portal := helperISCSISessionsCreateInputs()

	iSCSISessionsWithJustPortal = ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"5.6.7.8":    nil,
			"9.10.11.12": {},
		},
	}

	iSCSISessionsWithStrangeLUNs := ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"1.2.3.4": {
				LUNs: LUNs{
					Info: nil,
				},
			},
			"5.6.7.8": {
				LUNs: LUNs{
					Info: map[int32]string{},
				},
			},
			"9.10.11.12": {
				LUNs: LUNs{
					Info: map[int32]string{
						1: "",
						2: "",
						3: "",
					},
				},
			},
		},
	}

	// Get VolumeIDForPortal from empty Sessions
	volID, err := iSCSISessionsEmpty.VolumeIDForPortal(portal)
	assert.NotNil(t, err, "expected error when getting VolumeIDForPortal")

	// Get VolumeIDForPortal for Portal with nil SessionData
	volID, err = iSCSISessionsWithJustPortal.VolumeIDForPortal("5.6.7.8")
	assert.NotNil(t, err, "expected error when getting VolumeIDForPortal")

	// Get VolumeIDForPortal for Portal with empty SessionData
	volID, err = iSCSISessionsWithJustPortal.VolumeIDForPortal("9.10.11.12")
	assert.NotNil(t, err, "expected error when getting VolumeIDForPortal")

	// Get VolumeIDForPortal for non existent portal
	volID, err = iSCSISessionsWithJustPortal.VolumeIDForPortal("9.10.11.12")
	assert.NotNil(t, err, "expected error when getting VolumeIDForPortal")

	// Get VolumeIDForPortal for portal with nil LUNInfo
	volID, err = iSCSISessionsWithStrangeLUNs.VolumeIDForPortal("1.2.3.4")
	assert.NotNil(t, err, "expected error when getting VolumeIDForPortal")

	// Get VolumeIDForPortal for portal with empty LUNInfo
	volID, err = iSCSISessionsWithStrangeLUNs.VolumeIDForPortal("5.6.7.8")
	assert.NotNil(t, err, "expected error when getting VolumeIDForPortal")

	// Get VolumeIDForPortal for portal with empty Volume ID
	volID, err = iSCSISessionsWithStrangeLUNs.VolumeIDForPortal("9.10.11.12")
	assert.NotNil(t, err, "expected error when getting VolumeIDForPortal")

	// Get VolumeIDForPortal for existing portal with a valid SessionData
	volID, err = iSCSISessionNotEmpty.VolumeIDForPortal(portal)
	assert.Nil(t, err, "expected no error when getting VolumeIDForPortal")
	assert.NotEmpty(t, volID, "expected volID to be NOT empty")
}

func TestISCSISessions_ResetPortalRemediationValue(t *testing.T) {
	var iSCSISessionsEmpty, iSCSISessionsWithJustPortal ISCSISessions
	iSCSISessionNotEmpty, _, _, _, portal := helperISCSISessionsCreateInputs()

	iSCSISessionsWithJustPortal = ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"5.6.7.8":    nil,
			"9.10.11.12": {},
		},
	}

	// ResetPortalRemediationValue from empty Sessions
	err := iSCSISessionsEmpty.ResetPortalRemediationValue(portal)
	assert.NotNil(t, err, "expected error when resetting remediation value")

	// ResetPortalRemediationValue for Portal with nil SessionData
	err = iSCSISessionsWithJustPortal.ResetPortalRemediationValue("5.6.7.8")
	assert.NotNil(t, err, "expected error when resetting remediation value")

	// ResetPortalRemediationValue for Portal with empty SessionData
	err = iSCSISessionsWithJustPortal.ResetPortalRemediationValue("9.10.11.12")
	assert.Nil(t, err, "expected NO error when resetting remediation value")
	assert.True(t, iSCSISessionsWithJustPortal.Info["9.10.11.12"].Remediation == NoAction,
		"expected PublishInfo to be NOT empty")

	// GenerateResetPortalRemediationValuePublishInfo for non existent portal
	err = iSCSISessionNotEmpty.ResetPortalRemediationValue("9.10.11.12")
	assert.NotNil(t, err, "expected error when resetting remediation value")

	// ResetPortalRemediationValue for existing portal with a valid SessionData
	err = iSCSISessionNotEmpty.ResetPortalRemediationValue(portal)
	assert.Nil(t, err, "expected error when resetting remediation value")
	assert.True(t, iSCSISessionsWithJustPortal.Info["9.10.11.12"].Remediation == NoAction,
		"expected PublishInfo to be NOT empty")
}

func TestISCSISessions_ResetAllRemediationValues(t *testing.T) {
	var iSCSISessionsEmpty, iSCSISessionsWithJustPortal ISCSISessions
	iSCSISessionNotEmpty, _, _, _, _ := helperISCSISessionsCreateInputs()

	iSCSISessionsWithJustPortal = ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"5.6.7.8":    nil,
			"5.6.7.9":    nil,
			"9.10.11.12": {},
		},
	}

	// ResetPortalRemediationValue from empty Sessions
	err := iSCSISessionsEmpty.ResetAllRemediationValues()
	assert.Nil(t, err, "expected NO error when resetting all remediation value")

	// ResetPortalRemediationValue for Portal with nil SessionData
	err = iSCSISessionsWithJustPortal.ResetAllRemediationValues()
	assert.NotNil(t, err, "expected error when resetting remediation value")
	assert.True(t, iSCSISessionsWithJustPortal.Info["9.10.11.12"].Remediation == NoAction,
		"expected PublishInfo to be NOT empty")

	// ResetPortalRemediationValue for existing portal with a valid SessionData
	err = iSCSISessionNotEmpty.ResetAllRemediationValues()
	assert.Nil(t, err, "expected error when resetting remediation value")
	assert.True(t, iSCSISessionsWithJustPortal.Info["9.10.11.12"].Remediation == NoAction,
		"expected PublishInfo to be NOT empty")
}

func TestISCSISessions_GeneratePublishInfo(t *testing.T) {
	var iSCSISessionsEmpty, iSCSISessionsWithJustPortal ISCSISessions
	iSCSISessionNotEmpty, _, _, _, portal := helperISCSISessionsCreateInputs()

	iSCSISessionsWithJustPortal = ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"5.6.7.8":    nil,
			"9.10.11.12": {},
		},
	}

	// GeneratePublishInfo from empty Sessions
	publishInfo, err := iSCSISessionsEmpty.GeneratePublishInfo(portal)
	assert.NotNil(t, err, "expected error when generating PublishInfo")

	// GeneratePublishInfo for Portal with nil SessionData
	publishInfo, err = iSCSISessionsWithJustPortal.GeneratePublishInfo("5.6.7.8")
	assert.NotNil(t, err, "expected error when generating PublishInfo")

	// GeneratePublishInfo for Portal with empty SessionData
	publishInfo, err = iSCSISessionsWithJustPortal.GeneratePublishInfo("9.10.11.12")
	assert.NotNil(t, err, "expected error when generating PublishInfo")

	// GeneratePublishInfo for non existent portal
	publishInfo, err = iSCSISessionNotEmpty.GeneratePublishInfo("9.10.11.12")
	assert.NotNil(t, err, "expected error when generating PublishInfo")

	// GeneratePublishInfo for existing portal with a valid SessionData
	publishInfo, err = iSCSISessionNotEmpty.GeneratePublishInfo(portal)
	assert.Nil(t, err, "expected no error when generating PublishInfo")
	assert.NotEmpty(t, publishInfo, "expected PublishInfo to be NOT empty")
}

func TestISCSISessions_StringAndGoString(t *testing.T) {
	var iSCSISessionsEmpty, iSCSISessionsWithJustPortal ISCSISessions
	iSCSISessionNotEmpty, _, _, _, _ := helperISCSISessionsCreateInputs()

	iSCSISessionsWithJustPortal = ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"5.6.7.8":    nil,
			"9.10.11.12": {},
			"":           {},
			"  ":         nil,
		},
	}

	iSCSISessionsWithStrangeLUNs := ISCSISessions{
		Info: map[string]*ISCSISessionData{
			"1.2.3.4": {
				LUNs: LUNs{
					Info: nil,
				},
			},
			"5.6.7.8": {
				LUNs: LUNs{
					Info: map[int32]string{},
				},
			},
			"9.10.11.12": {
				LUNs: LUNs{
					Info: map[int32]string{
						1: "",
						2: "",
						3: "",
					},
				},
			},
			"": {
				LUNs: LUNs{},
			},
			"  ":   {},
			"    ": nil,
		},
	}

	// Get String and GoString output for empty Sessions
	assert.Equal(t, iSCSISessionsEmpty.String(), "empty portal to LUN mapping",
		"String output mismatch")
	assert.Equal(t, iSCSISessionsEmpty.GoString(), "empty portal to LUN mapping",
		"GoString output mismatch")

	// Get String and GoString output for Portal with nil SessionData
	assert.True(t, strings.Contains(iSCSISessionsWithJustPortal.String(), "portal value missing"))
	assert.True(t, strings.Contains(iSCSISessionsWithJustPortal.String(), "session information is missing"))
	assert.True(t, strings.Contains(iSCSISessionsWithJustPortal.GoString(), "portal value missing"))
	assert.True(t, strings.Contains(iSCSISessionsWithJustPortal.GoString(), "session information is missing"))

	// Get String and GoString output for Portal with strange LUN information
	assert.True(t, strings.Contains(iSCSISessionsWithStrangeLUNs.String(), "LUNInfo: {Info:map[]}"))
	assert.True(t, strings.Contains(iSCSISessionsWithStrangeLUNs.GoString(), "LUNInfo: {Info:map[]}"))

	// Get String and GoString output for portal with valid SessionData
	assert.NotEmpty(t, iSCSISessionNotEmpty.String(), "LUNInfo: {Info:map[0:volID-0 1:volID-1 2:]}")
	assert.NotEmpty(t, iSCSISessionNotEmpty.GoString(), "LUNInfo: {Info:map[0:volID-0 1:volID-1 2:]}")
}

func helperISCSISessionsCreateInputs() (ISCSISessions, ISCSISessionData, PortalInfo, LUNs, string) {
	LUNdata := []LUNData{
		{0, "volID-0"},
		{1, "volID-1"},
		{2, ""},
	}

	var luns LUNs
	for _, input := range LUNdata {
		luns.AddLUN(input)
	}

	portalInfo := PortalInfo{
		ISCSITargetIQN:         "IQN1",
		SessionNumber:          "1",
		Credentials:            IscsiChapInfo{},
		ReasonInvalid:          NotInvalid,
		LastAccessTime:         time.Time{},
		FirstIdentifiedStaleAt: time.Time{},
	}

	portal := "1.2.3.4"

	iSCSISessionData := ISCSISessionData{
		PortalInfo:  portalInfo,
		LUNs:        luns,
		Remediation: LoginScan,
	}

	iSCSISession := ISCSISessions{
		Info: map[string]*ISCSISessionData{
			portal: &iSCSISessionData,
		},
	}

	return iSCSISession, iSCSISessionData, portalInfo, luns, portal
}

func TestNodePublicationStateFlags_IsNodeDirty(t *testing.T) {
	tests := map[string]struct {
		orchestratorReady, administratorReady *bool
		expected, shouldFail                  bool
	}{
		"returns false when OrchestratorReady is true but AdministratorReady is not": {
			orchestratorReady:  Ptr(true),
			administratorReady: nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is false but AdministratorReady is not": {
			orchestratorReady:  Ptr(false),
			administratorReady: nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is not set but AdministratorReady is true": {
			orchestratorReady:  nil,
			administratorReady: Ptr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set but AdministratorReady is false": {
			orchestratorReady:  nil,
			administratorReady: Ptr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady and AdministratorReady are not set": {
			orchestratorReady:  nil,
			administratorReady: nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is true and AdministratorReady is false": {
			orchestratorReady:  Ptr(true),
			administratorReady: Ptr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is false and AdministratorReady is true": {
			orchestratorReady:  Ptr(false),
			administratorReady: Ptr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is true and AdministratorReady is true": {
			orchestratorReady:  Ptr(true),
			administratorReady: Ptr(true),
			expected:           false,
		},
		"returns true when OrchestratorReady is false and AdministratorReady is false": {
			orchestratorReady:  Ptr(false),
			administratorReady: Ptr(false),
			expected:           true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			f := NodePublicationStateFlags{
				OrchestratorReady:  test.orchestratorReady,
				AdministratorReady: test.administratorReady,
			}

			assert.Equal(t, test.expected, f.IsNodeDirty())
		})
	}
}

func TestNodePublicationStateFlags_IsNodeCleanable(t *testing.T) {
	tests := map[string]struct {
		orchestratorReady, administratorReady *bool
		expected, shouldFail                  bool
	}{
		"returns false when OrchestratorReady is true but AdministratorReady is not set": {
			orchestratorReady:  Ptr(true),
			administratorReady: nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is false but AdministratorReady is not set": {
			orchestratorReady:  Ptr(false),
			administratorReady: nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is not set but AdministratorReady is true": {
			orchestratorReady:  nil,
			administratorReady: Ptr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set but AdministratorReady is false": {
			orchestratorReady:  nil,
			administratorReady: Ptr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady and AdministratorReady are not set": {
			orchestratorReady:  nil,
			administratorReady: nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is true and AdministratorReady is false": {
			orchestratorReady:  Ptr(true),
			administratorReady: Ptr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is false and AdministratorReady is true": {
			orchestratorReady:  Ptr(false),
			administratorReady: Ptr(true),
			expected:           false,
		},
		"returns true when OrchestratorReady is true and AdministratorReady is true": {
			orchestratorReady:  Ptr(true),
			administratorReady: Ptr(true),
			expected:           true,
		},
		"returns false when OrchestratorReady is false and AdministratorReady is false": {
			orchestratorReady:  Ptr(false),
			administratorReady: Ptr(false),
			expected:           false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			f := NodePublicationStateFlags{
				OrchestratorReady:  test.orchestratorReady,
				AdministratorReady: test.administratorReady,
			}

			assert.Equal(t, test.expected, f.IsNodeCleanable())
		})
	}
}

func TestNodePublicationStateFlags_IsNodeCleaned(t *testing.T) {
	tests := map[string]struct {
		orchestratorReady, administratorReady, provisionerReady *bool
		expected, shouldFail                                    bool
	}{
		"returns false when OrchestratorReady, AdministratorReady, and ProvisionerReady are not set": {
			orchestratorReady:  nil,
			administratorReady: nil,
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is true but AdministratorReady and ProvisionerReady are not set": {
			orchestratorReady:  Ptr(true),
			administratorReady: nil,
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is false but AdministratorReady and ProvisionerReady are not set": {
			orchestratorReady:  Ptr(false),
			administratorReady: nil,
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is true, and ProvisionerReady is not set": {
			orchestratorReady:  Ptr(true),
			administratorReady: Ptr(true),
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is false, and ProvisionerReady is not set": {
			orchestratorReady:  Ptr(true),
			administratorReady: Ptr(false),
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is true, and ProvisionerReady is not set": {
			orchestratorReady:  Ptr(false),
			administratorReady: Ptr(true),
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is not set, and ProvisionerReady is true": {
			orchestratorReady:  Ptr(true),
			administratorReady: nil,
			provisionerReady:   Ptr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is not set, and ProvisionerReady is false": {
			orchestratorReady:  Ptr(true),
			administratorReady: nil,
			provisionerReady:   Ptr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is not set, and ProvisionerReady is true": {
			orchestratorReady:  Ptr(false),
			administratorReady: nil,
			provisionerReady:   Ptr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is not set, and ProvisionerReady is false": {
			orchestratorReady:  Ptr(false),
			administratorReady: nil,
			provisionerReady:   Ptr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is true, and ProvisionerReady is not set": {
			orchestratorReady:  nil,
			administratorReady: Ptr(true),
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is false, and ProvisionerReady is not set": {
			orchestratorReady:  nil,
			administratorReady: Ptr(false),
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is true, and ProvisionerReady is false": {
			orchestratorReady:  nil,
			administratorReady: Ptr(true),
			provisionerReady:   Ptr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is false, and ProvisionerReady is true": {
			orchestratorReady:  nil,
			administratorReady: Ptr(false),
			provisionerReady:   Ptr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is true, and ProvisionerReady is true": {
			orchestratorReady:  nil,
			administratorReady: Ptr(true),
			provisionerReady:   Ptr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is false, and ProvisionerReady is false": {
			orchestratorReady:  nil,
			administratorReady: Ptr(false),
			provisionerReady:   Ptr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is not set, and ProvisionerReady is true": {
			orchestratorReady:  nil,
			administratorReady: nil,
			provisionerReady:   Ptr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is not set, and ProvisionerReady is false": {
			orchestratorReady:  nil,
			administratorReady: nil,
			provisionerReady:   Ptr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is false, and ProvisionerReady is false": {
			orchestratorReady:  Ptr(true),
			administratorReady: Ptr(false),
			provisionerReady:   Ptr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is true, and ProvisionerReady is false": {
			orchestratorReady:  Ptr(true),
			administratorReady: Ptr(true),
			provisionerReady:   Ptr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is false, and ProvisionerReady is true": {
			orchestratorReady:  Ptr(true),
			administratorReady: Ptr(false),
			provisionerReady:   Ptr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is true, and ProvisionerReady is false": {
			orchestratorReady:  Ptr(false),
			administratorReady: Ptr(true),
			provisionerReady:   Ptr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is true, and ProvisionerReady is true": {
			orchestratorReady:  Ptr(false),
			administratorReady: Ptr(true),
			provisionerReady:   Ptr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is false, and ProvisionerReady is true": {
			orchestratorReady:  Ptr(false),
			administratorReady: Ptr(false),
			provisionerReady:   Ptr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is false, and ProvisionerReady is false": {
			orchestratorReady:  Ptr(false),
			administratorReady: Ptr(false),
			provisionerReady:   Ptr(false),
			expected:           false,
		},
		"returns true when OrchestratorReady is true, AdministratorReady is true, and ProvisionerReady is true": {
			orchestratorReady:  Ptr(true),
			administratorReady: Ptr(true),
			provisionerReady:   Ptr(true),
			expected:           true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			f := NodePublicationStateFlags{
				OrchestratorReady:  test.orchestratorReady,
				AdministratorReady: test.administratorReady,
				ProvisionerReady:   test.provisionerReady,
			}

			assert.Equal(t, test.expected, f.IsNodeCleaned())
		})
	}
}
