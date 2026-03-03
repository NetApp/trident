// Copyright 2025 NetApp, Inc. All Rights Reserved.

package models

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/network"
	"github.com/netapp/trident/utils/errors"
)

var (
	fakeVolumePublication = &VolumePublication{
		Name:           "node1volume1",
		VolumeName:     "volume1",
		NodeName:       "node1",
		ReadOnly:       true,
		AccessMode:     1,
		AutogrowPolicy: "aggressive",
		StorageClass:   "gold",
		BackendUUID:    "backend-uuid-123",
		Pool:           "pool1",
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
		Name:           "node1volume1",
		VolumeName:     "volume1",
		NodeName:       "node1",
		ReadOnly:       true,
		AccessMode:     1,
		AutogrowPolicy: "aggressive",
		StorageClass:   "gold",
		BackendUUID:    "backend-uuid-123",
		Pool:           "pool1",
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
		Deleted:          convert.ToPtr(false),
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

func TestScsiDeviceAddress_String(t *testing.T) {
	tests := map[string]struct {
		address  ScsiDeviceAddress
		expected string
	}{
		"fully populated": {
			address: ScsiDeviceAddress{
				Host:    "1",
				Channel: "0",
				Target:  "0",
				LUN:     "0",
			},
			expected: "1:0:0:0",
		},
		"zero value": {
			address:  ScsiDeviceAddress{},
			expected: ":::",
		},
		"multi-digit values": {
			address: ScsiDeviceAddress{
				Host:    "12",
				Channel: "0",
				Target:  "3",
				LUN:     "42",
			},
			expected: "12:0:3:42",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.address.String())
		})
	}
}

func TestScsiDeviceInfo_String(t *testing.T) {
	t.Run("fully populated struct returns non-empty string", func(t *testing.T) {
		info := &ScsiDeviceInfo{
			ScsiDeviceAddress: ScsiDeviceAddress{
				Host:    "1",
				Channel: "0",
				Target:  "0",
				LUN:     "0",
			},
			Devices:         []string{"sda", "sdb"},
			DevicePaths:     []string{"/dev/sda", "/dev/sdb"},
			MultipathDevice: "dm-0",
			Filesystem:      "ext4",
			IQN:             "iqn.2010-01.com.netapp:target",
			WWNN:            "0x5000cca",
			SessionNumber:   3,
		}
		result := info.String()
		assert.NotEmpty(t, result)
	})

	t.Run("zero value struct returns non-empty string", func(t *testing.T) {
		info := &ScsiDeviceInfo{}
		result := info.String()
		assert.NotEmpty(t, result)
	})
}

func TestScsiDeviceInfo_Copy(t *testing.T) {
	t.Run("all fields are copied", func(t *testing.T) {
		original := &ScsiDeviceInfo{
			ScsiDeviceAddress: ScsiDeviceAddress{
				Host:    "1",
				Channel: "0",
				Target:  "0",
				LUN:     "0",
			},
			Devices:         []string{"sda", "sdb"},
			DevicePaths:     []string{"/dev/sda", "/dev/sdb"},
			MultipathDevice: "dm-0",
			Filesystem:      "ext4",
			IQN:             "iqn.2010-01.com.netapp:target",
			WWNN:            "0x5000cca",
			SessionNumber:   3,
			CHAPInfo: IscsiChapInfo{
				UseCHAP:              true,
				IscsiUsername:        "user",
				IscsiInitiatorSecret: "secret",
				IscsiTargetUsername:  "tuser",
				IscsiTargetSecret:    "tsecret",
			},
		}

		copied := original.Copy()

		assert.NotNil(t, copied)
		assert.True(t, original.Equal(copied))
		assert.Equal(t, original.ScsiDeviceAddress, copied.ScsiDeviceAddress)
		assert.Equal(t, original.MultipathDevice, copied.MultipathDevice)
		assert.Equal(t, original.Filesystem, copied.Filesystem)
		assert.Equal(t, original.IQN, copied.IQN)
		assert.Equal(t, original.WWNN, copied.WWNN)
		assert.Equal(t, original.SessionNumber, copied.SessionNumber)
		assert.Equal(t, original.CHAPInfo, copied.CHAPInfo)
		assert.Equal(t, original.Devices, copied.Devices)
		assert.Equal(t, original.DevicePaths, copied.DevicePaths)
	})

	t.Run("copy is not the same pointer", func(t *testing.T) {
		original := &ScsiDeviceInfo{
			Devices:     []string{"sda"},
			DevicePaths: []string{"/dev/sda"},
		}

		copied := original.Copy()
		assert.False(t, original == copied)
	})

	t.Run("modifying copy slices does not affect original", func(t *testing.T) {
		original := &ScsiDeviceInfo{
			ScsiDeviceAddress: ScsiDeviceAddress{Host: "1", Channel: "0", Target: "0", LUN: "0"},
			Devices:           []string{"sda", "sdb"},
			DevicePaths:       []string{"/dev/sda", "/dev/sdb"},
			MultipathDevice:   "dm-0",
		}

		copied := original.Copy()

		// Mutate the copy's slices.
		copied.Devices[0] = "sdc"
		copied.DevicePaths[0] = "/dev/sdc"
		copied.MultipathDevice = "dm-1"

		// Original should be unaffected.
		assert.Equal(t, "sda", original.Devices[0])
		assert.Equal(t, "/dev/sda", original.DevicePaths[0])
		assert.Equal(t, "dm-0", original.MultipathDevice)
	})

	t.Run("copy with empty slices", func(t *testing.T) {
		original := &ScsiDeviceInfo{
			Devices:     []string{},
			DevicePaths: []string{},
		}

		copied := original.Copy()
		assert.NotNil(t, copied)
		assert.Empty(t, copied.Devices)
		assert.Empty(t, copied.DevicePaths)
	})

	t.Run("copy with nil slices", func(t *testing.T) {
		original := &ScsiDeviceInfo{}
		copied := original.Copy()
		assert.NotNil(t, copied)
		assert.Empty(t, copied.Devices)
		assert.Empty(t, copied.DevicePaths)
	})
}

func TestScsiDeviceInfo_Equal(t *testing.T) {
	base := func() *ScsiDeviceInfo {
		return &ScsiDeviceInfo{
			ScsiDeviceAddress: ScsiDeviceAddress{Host: "1", Channel: "0", Target: "0", LUN: "0"},
			Devices:           []string{"sda", "sdb"},
			DevicePaths:       []string{"/dev/sda", "/dev/sdb"},
			MultipathDevice:   "dm-0",
			Filesystem:        "ext4",
			IQN:               "iqn.test",
			WWNN:              "0x5000",
			SessionNumber:     1,
			CHAPInfo:          IscsiChapInfo{UseCHAP: true, IscsiUsername: "user"},
		}
	}

	tests := map[string]struct {
		a     *ScsiDeviceInfo
		b     *ScsiDeviceInfo
		equal bool
	}{
		"identical structs": {
			a:     base(),
			b:     base(),
			equal: true,
		},
		"devices in different order": {
			a: base(),
			b: func() *ScsiDeviceInfo {
				s := base()
				s.Devices = []string{"sdb", "sda"}
				return s
			}(),
			equal: true,
		},
		"device paths in different order": {
			a: base(),
			b: func() *ScsiDeviceInfo {
				s := base()
				s.DevicePaths = []string{"/dev/sdb", "/dev/sda"}
				return s
			}(),
			equal: true,
		},
		"different address": {
			a: base(),
			b: func() *ScsiDeviceInfo {
				s := base()
				s.ScsiDeviceAddress.Host = "2"
				return s
			}(),
			equal: false,
		},
		"different multipath device": {
			a: base(),
			b: func() *ScsiDeviceInfo {
				s := base()
				s.MultipathDevice = "dm-1"
				return s
			}(),
			equal: false,
		},
		"different filesystem": {
			a: base(),
			b: func() *ScsiDeviceInfo {
				s := base()
				s.Filesystem = "xfs"
				return s
			}(),
			equal: false,
		},
		"different IQN": {
			a: base(),
			b: func() *ScsiDeviceInfo {
				s := base()
				s.IQN = "iqn.other"
				return s
			}(),
			equal: false,
		},
		"different WWNN": {
			a: base(),
			b: func() *ScsiDeviceInfo {
				s := base()
				s.WWNN = "0x6000"
				return s
			}(),
			equal: false,
		},
		"different session number": {
			a: base(),
			b: func() *ScsiDeviceInfo {
				s := base()
				s.SessionNumber = 99
				return s
			}(),
			equal: false,
		},
		"different CHAP info": {
			a: base(),
			b: func() *ScsiDeviceInfo {
				s := base()
				s.CHAPInfo = IscsiChapInfo{UseCHAP: false}
				return s
			}(),
			equal: false,
		},
		"extra device in one": {
			a: base(),
			b: func() *ScsiDeviceInfo {
				s := base()
				s.Devices = append(s.Devices, "sdc")
				return s
			}(),
			equal: false,
		},
		"extra device path in one": {
			a: base(),
			b: func() *ScsiDeviceInfo {
				s := base()
				s.DevicePaths = append(s.DevicePaths, "/dev/sdc")
				return s
			}(),
			equal: false,
		},
		"one has empty devices other has populated": {
			a: base(),
			b: func() *ScsiDeviceInfo {
				s := base()
				s.Devices = []string{}
				return s
			}(),
			equal: false,
		},
		"both have empty slices": {
			a: &ScsiDeviceInfo{
				Devices:     []string{},
				DevicePaths: []string{},
			},
			b: &ScsiDeviceInfo{
				Devices:     []string{},
				DevicePaths: []string{},
			},
			equal: true,
		},
		"nil slices equal empty slices": {
			a: &ScsiDeviceInfo{
				Devices:     nil,
				DevicePaths: nil,
			},
			b: &ScsiDeviceInfo{
				Devices:     []string{},
				DevicePaths: []string{},
			},
			equal: true,
		},
		"both have nil slices": {
			a:     &ScsiDeviceInfo{},
			b:     &ScsiDeviceInfo{},
			equal: true,
		},
		"copy equals original": {
			a: func() *ScsiDeviceInfo {
				return &ScsiDeviceInfo{
					ScsiDeviceAddress: ScsiDeviceAddress{Host: "1", Channel: "0", Target: "0", LUN: "0"},
					Devices:           []string{"sda"},
					MultipathDevice:   "dm-0",
				}
			}(),
			b: func() *ScsiDeviceInfo {
				s := &ScsiDeviceInfo{
					ScsiDeviceAddress: ScsiDeviceAddress{Host: "1", Channel: "0", Target: "0", LUN: "0"},
					Devices:           []string{"sda"},
					MultipathDevice:   "dm-0",
				}
				return s.Copy()
			}(),
			equal: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.equal, tc.a.Equal(tc.b))
		})
	}
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
	assert.True(t, errors.IsNotFoundError(err), "Expecting volume ID not found error")
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
	containsAll, _ := collection.ContainsElements([]int32{4, 99}, l.IdentifyMissingLUNs(m))
	assert.True(t, containsAll, "Missing LUN mismatch")
	containsAll, _ = collection.ContainsElements(l.AllLUNs(), n.IdentifyMissingLUNs(l))
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
	assert.True(t, errors.IsNotFoundError(err), "Non-empty session should return NotFoundError")

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
		LUNs: LUNs{
			Info: map[int32]string{
				1: "volID-1",
				2: "volID-2",
			},
		},
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
	containsAll, _ := collection.ContainsElements(luns, inputLUNInfo.AllLUNs())
	assert.True(t, containsAll, "expected LUNs to be equal")
}

func TestISCSISessions_VolumeIDForPortalAndLUN(t *testing.T) {
	t.Run("with empty portal specified", func(t *testing.T) {
		sessions := ISCSISessions{}
		id, err := sessions.VolumeIDForPortalAndLUN("", 0)
		assert.Error(t, err)
		assert.Empty(t, id)
	})

	t.Run("with negative LUN ID specified", func(t *testing.T) {
		sessions := ISCSISessions{}
		id, err := sessions.VolumeIDForPortalAndLUN("non-empty", -1)
		assert.Error(t, err)
		assert.Empty(t, id)
	})

	t.Run("with non-empty iSCSI sessions but invalid portal", func(t *testing.T) {
		iscsiSession, _, _, _, _ := helperISCSISessionsCreateInputs()
		id, err := iscsiSession.VolumeIDForPortalAndLUN("-.-.-.-", 0)
		assert.Error(t, err)
		assert.Empty(t, id)
	})

	t.Run("with non-empty iSCSI sessions but no volume ID found for lun ID", func(t *testing.T) {
		iscsiSession, _, _, _, portal := helperISCSISessionsCreateInputs()
		// lun ID "100" is not present in iscsiSession.
		id, err := iscsiSession.VolumeIDForPortalAndLUN(portal, 100)
		assert.Error(t, err)
		assert.Empty(t, id)
	})

	t.Run("with non-empty iSCSI sessions and volume ID found lun ID", func(t *testing.T) {
		iscsiSession, _, _, luns, portal := helperISCSISessionsCreateInputs()
		// lun ID "0" is present in iscsiSession.
		id, err := iscsiSession.VolumeIDForPortalAndLUN(portal, 0)
		assert.NoError(t, err)
		assert.NotEmpty(t, id)

		expectedID, err := luns.VolumeID(int32(0))
		assert.NoError(t, err)
		assert.Equal(t, expectedID, id)
	})
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
	assert.NotNil(t, publishInfo, "expected PublishInfo to be NOT empty")
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
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is false but AdministratorReady is not": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is not set but AdministratorReady is true": {
			orchestratorReady:  nil,
			administratorReady: convert.ToPtr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set but AdministratorReady is false": {
			orchestratorReady:  nil,
			administratorReady: convert.ToPtr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady and AdministratorReady are not set": {
			orchestratorReady:  nil,
			administratorReady: nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is true and AdministratorReady is false": {
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: convert.ToPtr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is false and AdministratorReady is true": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: convert.ToPtr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is true and AdministratorReady is true": {
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: convert.ToPtr(true),
			expected:           false,
		},
		"returns true when OrchestratorReady is false and AdministratorReady is false": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: convert.ToPtr(false),
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
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is false but AdministratorReady is not set": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is not set but AdministratorReady is true": {
			orchestratorReady:  nil,
			administratorReady: convert.ToPtr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set but AdministratorReady is false": {
			orchestratorReady:  nil,
			administratorReady: convert.ToPtr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady and AdministratorReady are not set": {
			orchestratorReady:  nil,
			administratorReady: nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is true and AdministratorReady is false": {
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: convert.ToPtr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is false and AdministratorReady is true": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: convert.ToPtr(true),
			expected:           false,
		},
		"returns true when OrchestratorReady is true and AdministratorReady is true": {
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: convert.ToPtr(true),
			expected:           true,
		},
		"returns false when OrchestratorReady is false and AdministratorReady is false": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: convert.ToPtr(false),
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
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: nil,
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is false but AdministratorReady and ProvisionerReady are not set": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: nil,
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is true, and ProvisionerReady is not set": {
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: convert.ToPtr(true),
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is false, and ProvisionerReady is not set": {
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: convert.ToPtr(false),
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is true, and ProvisionerReady is not set": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: convert.ToPtr(true),
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is not set, and ProvisionerReady is true": {
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: nil,
			provisionerReady:   convert.ToPtr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is not set, and ProvisionerReady is false": {
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: nil,
			provisionerReady:   convert.ToPtr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is not set, and ProvisionerReady is true": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: nil,
			provisionerReady:   convert.ToPtr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is not set, and ProvisionerReady is false": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: nil,
			provisionerReady:   convert.ToPtr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is true, and ProvisionerReady is not set": {
			orchestratorReady:  nil,
			administratorReady: convert.ToPtr(true),
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is false, and ProvisionerReady is not set": {
			orchestratorReady:  nil,
			administratorReady: convert.ToPtr(false),
			provisionerReady:   nil,
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is true, and ProvisionerReady is false": {
			orchestratorReady:  nil,
			administratorReady: convert.ToPtr(true),
			provisionerReady:   convert.ToPtr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is false, and ProvisionerReady is true": {
			orchestratorReady:  nil,
			administratorReady: convert.ToPtr(false),
			provisionerReady:   convert.ToPtr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is true, and ProvisionerReady is true": {
			orchestratorReady:  nil,
			administratorReady: convert.ToPtr(true),
			provisionerReady:   convert.ToPtr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is false, and ProvisionerReady is false": {
			orchestratorReady:  nil,
			administratorReady: convert.ToPtr(false),
			provisionerReady:   convert.ToPtr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is not set, and ProvisionerReady is true": {
			orchestratorReady:  nil,
			administratorReady: nil,
			provisionerReady:   convert.ToPtr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is not set, AdministratorReady is not set, and ProvisionerReady is false": {
			orchestratorReady:  nil,
			administratorReady: nil,
			provisionerReady:   convert.ToPtr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is false, and ProvisionerReady is false": {
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: convert.ToPtr(false),
			provisionerReady:   convert.ToPtr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is true, and ProvisionerReady is false": {
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: convert.ToPtr(true),
			provisionerReady:   convert.ToPtr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is true, AdministratorReady is false, and ProvisionerReady is true": {
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: convert.ToPtr(false),
			provisionerReady:   convert.ToPtr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is true, and ProvisionerReady is false": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: convert.ToPtr(true),
			provisionerReady:   convert.ToPtr(false),
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is true, and ProvisionerReady is true": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: convert.ToPtr(true),
			provisionerReady:   convert.ToPtr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is false, and ProvisionerReady is true": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: convert.ToPtr(false),
			provisionerReady:   convert.ToPtr(true),
			expected:           false,
		},
		"returns false when OrchestratorReady is false, AdministratorReady is false, and ProvisionerReady is false": {
			orchestratorReady:  convert.ToPtr(false),
			administratorReady: convert.ToPtr(false),
			provisionerReady:   convert.ToPtr(false),
			expected:           false,
		},
		"returns true when OrchestratorReady is true, AdministratorReady is true, and ProvisionerReady is true": {
			orchestratorReady:  convert.ToPtr(true),
			administratorReady: convert.ToPtr(true),
			provisionerReady:   convert.ToPtr(true),
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

func TestParseHostportIP(t *testing.T) {
	type IPAddresses struct {
		InputIP  string
		OutputIP string
	}
	tests := []IPAddresses{
		{
			InputIP:  "1.2.3.4:5678",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "1.2.3.4:5678,1001",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "1.2.3.4",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "[1:2:3:4]:5678",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "[1:2:3:4]:5678,1001",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "[1:2:3:4]",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]:5678",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]:5678,1001",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "2607:f8b0:4006:818:0:0:0:2004",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputIP, func(t *testing.T) {
			assert.Equal(t, testCase.OutputIP, network.ParseHostportIP(testCase.InputIP), "IP mismatch")
		})
	}
}

func TestVolumePublication_SmartCopy(t *testing.T) {
	// Create a VolumePublication object
	volumePublication := &VolumePublication{
		VolumeName: "vol-123",
		NodeName:   "node-456",
	}

	// Create a deep copy of the VolumePublication object
	copiedVolumePublication := volumePublication.SmartCopy().(*VolumePublication)

	// Check that the copied volume publication is deeply equal to the original
	assert.NotNil(t, copiedVolumePublication)
	assert.Equal(t, volumePublication, copiedVolumePublication)

	// Check that the copied volume publication does not point to the same memory
	assert.False(t, volumePublication == copiedVolumePublication)
}

// TestVolumePublicationCopy_WithNewFields tests that the Copy method preserves all fields including the new ones
func TestVolumePublicationCopy_WithNewFields(t *testing.T) {
	original := &VolumePublication{
		Name:           "test-pub",
		VolumeName:     "test-volume",
		NodeName:       "test-node",
		ReadOnly:       false,
		AccessMode:     5,
		AutogrowPolicy: "conservative",
		StorageClass:   "premium",
		BackendUUID:    "backend-uuid-abc",
		Pool:           "pool-xyz",
	}

	copied := original.Copy()

	// Verify all fields are copied
	assert.Equal(t, original.Name, copied.Name)
	assert.Equal(t, original.VolumeName, copied.VolumeName)
	assert.Equal(t, original.NodeName, copied.NodeName)
	assert.Equal(t, original.ReadOnly, copied.ReadOnly)
	assert.Equal(t, original.AccessMode, copied.AccessMode)
	assert.Equal(t, original.AutogrowPolicy, copied.AutogrowPolicy)
	assert.Equal(t, original.StorageClass, copied.StorageClass)
	assert.Equal(t, original.BackendUUID, copied.BackendUUID)
	assert.Equal(t, original.Pool, copied.Pool)

	// Verify it's a deep copy
	assert.False(t, original == copied)

	// Modify copied and verify original is unchanged
	copied.StorageClass = "changed"
	copied.BackendUUID = "changed"
	copied.Pool = "changed"
	assert.Equal(t, "premium", original.StorageClass)
	assert.Equal(t, "backend-uuid-abc", original.BackendUUID)
	assert.Equal(t, "pool-xyz", original.Pool)
}

// TestVolumePublicationConstructExternal_WithNewFields tests that ConstructExternal preserves all fields
func TestVolumePublicationConstructExternal_WithNewFields(t *testing.T) {
	internal := &VolumePublication{
		Name:           "test-pub",
		VolumeName:     "test-volume",
		NodeName:       "test-node",
		ReadOnly:       true,
		AccessMode:     3,
		AutogrowPolicy: "aggressive",
		StorageClass:   "gold",
		BackendUUID:    "backend-123",
		Pool:           "pool-456",
	}

	external := internal.ConstructExternal()

	// Verify all fields are preserved in external representation
	assert.Equal(t, internal.Name, external.Name)
	assert.Equal(t, internal.VolumeName, external.VolumeName)
	assert.Equal(t, internal.NodeName, external.NodeName)
	assert.Equal(t, internal.ReadOnly, external.ReadOnly)
	assert.Equal(t, internal.AccessMode, external.AccessMode)
	assert.Equal(t, internal.AutogrowPolicy, external.AutogrowPolicy)
	assert.Equal(t, internal.StorageClass, external.StorageClass)
	assert.Equal(t, internal.BackendUUID, external.BackendUUID)
	assert.Equal(t, internal.Pool, external.Pool)
}

// TestVolumePublicationNewFields_EmptyValues tests that empty values for new fields are handled correctly
func TestVolumePublicationNewFields_EmptyValues(t *testing.T) {
	pub := &VolumePublication{
		Name:           "test-pub",
		VolumeName:     "test-volume",
		NodeName:       "test-node",
		ReadOnly:       false,
		AccessMode:     1,
		AutogrowPolicy: "",
		StorageClass:   "",
		BackendUUID:    "",
		Pool:           "",
	}

	// Test Copy with empty values
	copied := pub.Copy()
	assert.Equal(t, "", copied.StorageClass)
	assert.Equal(t, "", copied.BackendUUID)
	assert.Equal(t, "", copied.Pool)

	// Test ConstructExternal with empty values
	external := pub.ConstructExternal()
	assert.Equal(t, "", external.StorageClass)
	assert.Equal(t, "", external.BackendUUID)
	assert.Equal(t, "", external.Pool)
}

// TestVolumePublicationNewFields_SpecialCharacters tests new fields with special characters
func TestVolumePublicationNewFields_SpecialCharacters(t *testing.T) {
	testCases := []struct {
		name         string
		storageClass string
		backendUUID  string
		pool         string
	}{
		{
			name:         "with dashes",
			storageClass: "storage-class-with-dashes",
			backendUUID:  "backend-uuid-with-dashes-123",
			pool:         "pool-with-dashes",
		},
		{
			name:         "with underscores",
			storageClass: "storage_class_with_underscores",
			backendUUID:  "backend_uuid_with_underscores",
			pool:         "pool_with_underscores",
		},
		{
			name:         "with mixed characters",
			storageClass: "storage-class_123.test",
			backendUUID:  "backend-uuid_abc-def.123",
			pool:         "pool-name_123.xyz",
		},
		{
			name:         "with numbers",
			storageClass: "storage123",
			backendUUID:  "backend456",
			pool:         "pool789",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pub := &VolumePublication{
				Name:         "test-pub",
				VolumeName:   "test-volume",
				NodeName:     "test-node",
				ReadOnly:     false,
				AccessMode:   1,
				StorageClass: tc.storageClass,
				BackendUUID:  tc.backendUUID,
				Pool:         tc.pool,
			}

			// Test Copy preserves special characters
			copied := pub.Copy()
			assert.Equal(t, tc.storageClass, copied.StorageClass)
			assert.Equal(t, tc.backendUUID, copied.BackendUUID)
			assert.Equal(t, tc.pool, copied.Pool)

			// Test ConstructExternal preserves special characters
			external := pub.ConstructExternal()
			assert.Equal(t, tc.storageClass, external.StorageClass)
			assert.Equal(t, tc.backendUUID, external.BackendUUID)
			assert.Equal(t, tc.pool, external.Pool)
		})
	}
}

func TestAutogrowPolicyReason_String(t *testing.T) {
	tests := []struct {
		name     string
		reason   AutogrowPolicyReason
		expected string
	}{
		{
			name:     "Active reason",
			reason:   AutogrowPolicyReasonActive,
			expected: "Autogrow policy is active",
		},
		{
			name:     "NotConfigured reason",
			reason:   AutogrowPolicyReasonNotConfigured,
			expected: "No autogrow policy configured",
		},
		{
			name:     "Disabled reason",
			reason:   AutogrowPolicyReasonDisabled,
			expected: "Autogrow explicitly disabled via 'none' annotation",
		},
		{
			name:     "NotFound reason",
			reason:   AutogrowPolicyReasonNotFound,
			expected: "Configured autogrow policy does not exist",
		},
		{
			name:     "Unusable reason",
			reason:   AutogrowPolicyReasonUnusable,
			expected: "Autogrow policy exists but is in Failed or Deleting state",
		},
		{
			name:     "Unknown reason (custom value)",
			reason:   AutogrowPolicyReason("CustomReason"),
			expected: "CustomReason",
		},
		{
			name:     "Empty reason",
			reason:   AutogrowPolicyReason(""),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.reason.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestVolumePublicationNewFields_MaxLength tests with very long field values
func TestVolumePublicationNewFields_MaxLength(t *testing.T) {
	// Create a very long value to test maximum field lengths
	longValue := strings.Repeat("a", 1000)

	pub := &VolumePublication{
		Name:         "test-pub",
		VolumeName:   "test-volume",
		NodeName:     "test-node",
		ReadOnly:     false,
		AccessMode:   1,
		StorageClass: longValue,
		BackendUUID:  longValue,
		Pool:         longValue,
	}

	// Test Copy with long values
	copied := pub.Copy()
	assert.Equal(t, longValue, copied.StorageClass)
	assert.Equal(t, longValue, copied.BackendUUID)
	assert.Equal(t, longValue, copied.Pool)

	// Test ConstructExternal with long values
	external := pub.ConstructExternal()
	assert.Equal(t, longValue, external.StorageClass)
	assert.Equal(t, longValue, external.BackendUUID)
	assert.Equal(t, longValue, external.Pool)
}

// TestVolumePublicationCopy_Independence tests that copied publications are independent
func TestVolumePublicationCopy_Independence(t *testing.T) {
	original := &VolumePublication{
		Name:           "test-pub",
		VolumeName:     "test-volume",
		NodeName:       "test-node",
		ReadOnly:       false,
		AccessMode:     1,
		AutogrowPolicy: "original-policy",
		StorageClass:   "original-class",
		BackendUUID:    "original-backend",
		Pool:           "original-pool",
	}

	// Create copy
	copied := original.Copy()

	// Modify copy - should not affect original
	copied.StorageClass = "modified-class"
	copied.BackendUUID = "modified-backend"
	copied.Pool = "modified-pool"
	copied.AutogrowPolicy = "modified-policy"
	copied.VolumeName = "modified-volume"
	copied.NodeName = "modified-node"
	copied.ReadOnly = true
	copied.AccessMode = 99

	// Verify original is unchanged
	assert.Equal(t, "original-class", original.StorageClass)
	assert.Equal(t, "original-backend", original.BackendUUID)
	assert.Equal(t, "original-pool", original.Pool)
	assert.Equal(t, "original-policy", original.AutogrowPolicy)
	assert.Equal(t, "test-volume", original.VolumeName)
	assert.Equal(t, "test-node", original.NodeName)
	assert.Equal(t, false, original.ReadOnly)
	assert.Equal(t, int32(1), original.AccessMode)
}

// TestVolumePublicationExternal_AllFieldsPresent verifies VolumePublicationExternal has all fields
func TestVolumePublicationExternal_AllFieldsPresent(t *testing.T) {
	external := &VolumePublicationExternal{
		Name:           "test-pub",
		VolumeName:     "test-volume",
		NodeName:       "test-node",
		ReadOnly:       true,
		AccessMode:     3,
		AutogrowPolicy: "aggressive",
		StorageClass:   "platinum",
		BackendUUID:    "backend-xyz",
		Pool:           "pool-abc",
	}

	// Verify all fields can be set and retrieved
	assert.Equal(t, "test-pub", external.Name)
	assert.Equal(t, "test-volume", external.VolumeName)
	assert.Equal(t, "test-node", external.NodeName)
	assert.Equal(t, true, external.ReadOnly)
	assert.Equal(t, int32(3), external.AccessMode)
	assert.Equal(t, "aggressive", external.AutogrowPolicy)
	assert.Equal(t, "platinum", external.StorageClass)
	assert.Equal(t, "backend-xyz", external.BackendUUID)
	assert.Equal(t, "pool-abc", external.Pool)
}

func TestAutogrowPolicyReason_MarshalJSON(t *testing.T) {
	tests := []struct {
		name         string
		reason       AutogrowPolicyReason
		expectedJSON string
	}{
		{
			name:         "Active reason",
			reason:       AutogrowPolicyReasonActive,
			expectedJSON: `"Autogrow policy is active"`,
		},
		{
			name:         "NotConfigured reason",
			reason:       AutogrowPolicyReasonNotConfigured,
			expectedJSON: `"No autogrow policy configured"`,
		},
		{
			name:         "Disabled reason",
			reason:       AutogrowPolicyReasonDisabled,
			expectedJSON: `"Autogrow explicitly disabled via 'none' annotation"`,
		},
		{
			name:         "NotFound reason",
			reason:       AutogrowPolicyReasonNotFound,
			expectedJSON: `"Configured autogrow policy does not exist"`,
		},
		{
			name:         "Unusable reason",
			reason:       AutogrowPolicyReasonUnusable,
			expectedJSON: `"Autogrow policy exists but is in Failed or Deleting state"`,
		},
		{
			name:         "Unknown reason",
			reason:       AutogrowPolicyReason("CustomReason"),
			expectedJSON: `"CustomReason"`,
		},
		{
			name:         "Empty reason",
			reason:       AutogrowPolicyReason(""),
			expectedJSON: `""`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonBytes, err := tt.reason.MarshalJSON()
			assert.NoError(t, err, "MarshalJSON should not return error")
			assert.Equal(t, tt.expectedJSON, string(jsonBytes))
		})
	}
}

func TestAutogrowPolicyReason_Constants(t *testing.T) {
	tests := []struct {
		name     string
		constant AutogrowPolicyReason
		expected string
	}{
		{
			name:     "Active constant value",
			constant: AutogrowPolicyReasonActive,
			expected: "Active",
		},
		{
			name:     "NotConfigured constant value",
			constant: AutogrowPolicyReasonNotConfigured,
			expected: "NotConfigured",
		},
		{
			name:     "Disabled constant value",
			constant: AutogrowPolicyReasonDisabled,
			expected: "Disabled",
		},
		{
			name:     "NotFound constant value",
			constant: AutogrowPolicyReasonNotFound,
			expected: "NotFound",
		},
		{
			name:     "Unusable constant value",
			constant: AutogrowPolicyReasonUnusable,
			expected: "Unusable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, string(tt.constant))
		})
	}
}

func TestEffectiveAutogrowPolicyInfo(t *testing.T) {
	tests := []struct {
		name       string
		policyInfo EffectiveAutogrowPolicyInfo
	}{
		{
			name: "Active policy",
			policyInfo: EffectiveAutogrowPolicyInfo{
				PolicyName: "gold-policy",
				Reason:     AutogrowPolicyReasonActive,
			},
		},
		{
			name: "Not configured",
			policyInfo: EffectiveAutogrowPolicyInfo{
				PolicyName: "",
				Reason:     AutogrowPolicyReasonNotConfigured,
			},
		},
		{
			name: "Disabled policy",
			policyInfo: EffectiveAutogrowPolicyInfo{
				PolicyName: "",
				Reason:     AutogrowPolicyReasonDisabled,
			},
		},
		{
			name: "Not found policy",
			policyInfo: EffectiveAutogrowPolicyInfo{
				PolicyName: "missing-policy",
				Reason:     AutogrowPolicyReasonNotFound,
			},
		},
		{
			name: "Unusable policy",
			policyInfo: EffectiveAutogrowPolicyInfo{
				PolicyName: "failed-policy",
				Reason:     AutogrowPolicyReasonUnusable,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify PolicyName field
			assert.Equal(t, tt.policyInfo.PolicyName, tt.policyInfo.PolicyName)

			// Verify Reason field
			assert.Equal(t, tt.policyInfo.Reason, tt.policyInfo.Reason)

			// Verify Reason.String() works
			reasonStr := tt.policyInfo.Reason.String()
			assert.NotEmpty(t, reasonStr)
		})
	}
}

// TestVolumeAutogrowStatus verifies VolumeAutogrowStatus struct fields and JSON round-trip
func TestVolumeAutogrowStatus(t *testing.T) {
	attemptedAt := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	successAt := time.Date(2025, 1, 16, 12, 0, 0, 0, time.UTC)

	status := &VolumeAutogrowStatus{
		LastAutogrowPolicyUsed:   "aggressive",
		LastAutogrowAttemptedAt:  &attemptedAt,
		LastProposedSize:         "10Gi",
		LastSuccessfulAutogrowAt: &successAt,
		LastSuccessfulSize:       "10Gi",
		LastError:                "",
		TotalAutogrowAttempted:   5,
		TotalSuccessfulAutogrow:  4,
	}

	assert.Equal(t, "aggressive", status.LastAutogrowPolicyUsed)
	assert.Equal(t, "10Gi", status.LastProposedSize)
	assert.Equal(t, "10Gi", status.LastSuccessfulSize)
	assert.Equal(t, "", status.LastError)
	assert.Equal(t, 5, status.TotalAutogrowAttempted)
	assert.Equal(t, 4, status.TotalSuccessfulAutogrow)
	assert.NotNil(t, status.LastAutogrowAttemptedAt)
	assert.Equal(t, attemptedAt, *status.LastAutogrowAttemptedAt)
	assert.NotNil(t, status.LastSuccessfulAutogrowAt)
	assert.Equal(t, successAt, *status.LastSuccessfulAutogrowAt)

	// JSON round-trip
	jsonBytes, err := json.Marshal(status)
	assert.NoError(t, err)
	var decoded VolumeAutogrowStatus
	err = json.Unmarshal(jsonBytes, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, status.LastAutogrowPolicyUsed, decoded.LastAutogrowPolicyUsed)
	assert.Equal(t, status.LastProposedSize, decoded.LastProposedSize)
	assert.Equal(t, status.TotalAutogrowAttempted, decoded.TotalAutogrowAttempted)
	assert.Equal(t, status.TotalSuccessfulAutogrow, decoded.TotalSuccessfulAutogrow)
}
