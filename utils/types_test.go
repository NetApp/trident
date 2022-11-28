/*
 * Copyright (c) 2022 NetApp, Inc. All Rights Reserved.
 */

package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TODO(arorar): Re-visit all the UTs, re-write many of them and improve coverage.

func TestPortalLUNMapping_IsEmpty(t *testing.T) {
	var p *ISCSISessions
	var m ISCSISessions
	var err error

	// Null pointer operations.
	t.Run("Verifying null pointer references", func(t *testing.T) {
		assert.True(t, p.IsEmpty(), "expected the null pointer returns empty map")
		// Code coverage
		p.RemoveAllPortals()
	})

	t.Run("Verifying invalid portal info", func(t *testing.T) {
		err = p.AddPortal("testPortal", PortalInfo{ISCSITargetIQN: "", FirstIdentifiedStaleAt: time.Time{}})
		assert.NotNil(t, err, "expected error adding entry to null map pointer")
	})

	t.Run("Verifying valid portal but null pointer of map", func(t *testing.T) {
		assert.Panics(t, func() {
			p.AddPortal("testportal", PortalInfo{ISCSITargetIQN: "dummy",
				FirstIdentifiedStaleAt: time.Now()})
		},
			"expected panic when adding portal to null pointer")
		assert.True(t, p.IsEmpty())
	})

	t.Run("Verifying addLun to non-existing portal", func(t *testing.T) {
		q := &ISCSISessions{Info: make(map[string]*ISCSISessionData)}
		err = q.AddLUNToPortal("testPortal", LUNData{0, ""})
		assert.NotNil(t, err, "expected error adding LUN to non-existing portal")
		assert.True(t, q.IsEmpty(), "expected no entries added to map")
	})

	t.Run("Checking for empty portal map", func(t *testing.T) {
		assert.True(t, m.IsEmpty())
		m.RemoveLUNFromPortal("non-existing", 101)
		assert.True(t, m.IsEmpty())
	})

	portalInfo := PortalInfo{ISCSITargetIQN: "dummy", FirstIdentifiedStaleAt: time.Time{}}
	err = m.AddPortal("testPortal", portalInfo)
	assert.Nil(t, err, "expected no error")
	err = m.AddLUNToPortal("testPortal", LUNData{100, ""})
	assert.Nil(t, err, "expected nil error")

	// for test coverage
	err = m.AddLUNToPortal("testPortal", LUNData{101, ""})
	assert.Nil(t, err, "expected no error")
	err = m.AddLUNsToPortal("testPortal", []LUNData{{102, ""}, {103, ""}})
	assert.Nil(t, err, "expected no error")

	err = m.AddPortal("NewPortal", portalInfo)
	assert.Nil(t, err, "expected no error adding a portal")
	err = m.AddLUNsToPortal("NewPortal", []LUNData{{201, ""}, {202, ""}, {203, ""}, {203, ""}})
	assert.Nil(t, err, "expected no error while adding LUNs")

	t.Run("Checking for non-empty portal map", func(t *testing.T) {
		assert.False(t, m.IsEmpty(), "expected map with entries added")
	})
}

func TestPortalLUNMapping_UpdatePortalInfo(t *testing.T) {
	p := &ISCSISessions{Info: make(map[string]*ISCSISessionData)}

	t.Run("Verifying invalid portal info", func(t *testing.T) {
		_, err := p.UpdateAndRecordPortalInfoChanges("testPortal",
			PortalInfo{ISCSITargetIQN: "", FirstIdentifiedStaleAt: time.Time{}})
		assert.NotNil(t, err, "expected error adding entry to null map pointer")
	})

	t.Run("Add new Portal/LUN info", func(t *testing.T) {
		assert.True(t, p.IsEmpty())
		err := p.AddPortal("1.1.1.1", PortalInfo{ISCSITargetIQN: "IQN1", FirstIdentifiedStaleAt: time.Time{},
			Credentials: IscsiChapInfo{UseCHAP: false}})
		assert.Nil(t, err, "failed to add new portal information")
		assert.False(t, p.IsEmpty())
	})

	t.Run("Update existing Portal/LUN CHAP information", func(t *testing.T) {
		portalLunData, err := p.ISCSISessionData("1.1.1.1")
		assert.Nil(t, err, "unable to get portal lun information")
		assert.False(t, portalLunData.PortalInfo.Credentials.UseCHAP)

		update, err := p.UpdateAndRecordPortalInfoChanges("1.1.1.1",
			PortalInfo{ISCSITargetIQN: "IQN1", FirstIdentifiedStaleAt: time.Time{},
				Credentials: IscsiChapInfo{UseCHAP: true}})
		assert.Nil(t, err, "failed to update new portal information")
		assert.Contains(t, update, "CHAP changed", "Missing CHAP update notification")

		portalLunData, err = p.ISCSISessionData("1.1.1.1")
		assert.Nil(t, err, "unable to get portal lun information")
		assert.True(t, portalLunData.PortalInfo.Credentials.UseCHAP)
	})

	t.Run("Updating a non-existing Portal/LUN CHAP information should result in an error", func(t *testing.T) {
		_, err := p.UpdateAndRecordPortalInfoChanges("2.2.2.2",
			PortalInfo{ISCSITargetIQN: "IQN1", FirstIdentifiedStaleAt: time.Time{}, Credentials: IscsiChapInfo{UseCHAP: true}})
		assert.NotNil(t, err, "expected error when updating non-existent portal information")
	})

	p.Info = nil
	t.Run("Updating an empty Portal/LUN CHAP information should result in an error", func(t *testing.T) {
		_, err := p.UpdateAndRecordPortalInfoChanges("2.2.2.2",
			PortalInfo{ISCSITargetIQN: "IQN1", FirstIdentifiedStaleAt: time.Time{}, Credentials: IscsiChapInfo{UseCHAP: true}})
		assert.NotNil(t, err, "expected error when updating non-existent portal information")
	})
}

func TestPortalLUNMapping_RemovePortal(t *testing.T) {
	var err error
	m := ISCSISessions{Info: make(map[string]*ISCSISessionData)}

	t.Run("Verifying empty mapping", func(t *testing.T) {
		assert.True(t, m.IsEmpty())
	})
	portal := "10.191.21.163:3260"
	portalInfo := PortalInfo{ISCSITargetIQN: "dummy", FirstIdentifiedStaleAt: time.Time{}}
	err = m.AddPortal(portal, portalInfo)
	assert.Nil(t, err, "expected no error")

	t.Run("Checking removal of non-existing", func(t *testing.T) {
		m.RemovePortal("Non-Existing")
		assert.False(t, m.IsEmpty())
	})

	t.Run("Checking removal of an entry", func(t *testing.T) {
		m.RemovePortal(portal)
		assert.True(t, m.IsEmpty(), "expected map to be empty")
	})
}

func TestPortalLUNMapping_RemoveLUNFromPortal(t *testing.T) {
	var err error
	m := ISCSISessions{Info: make(map[string]*ISCSISessionData)}
	portal := "10.191.21.163:3260"
	portalInfo := PortalInfo{ISCSITargetIQN: "dummy", FirstIdentifiedStaleAt: time.Time{}}
	err = m.AddPortal(portal, portalInfo)
	assert.Nil(t, err, "expected no error adding portal")
	err = m.AddLUNToPortal(portal, LUNData{101, ""})
	assert.Nil(t, err, "expected no error while adding single LUN")
	err = m.AddLUNToPortal(portal, LUNData{202, ""})
	assert.Nil(t, err, "expected no error while adding a LUN")

	portalLunData, err := m.ISCSISessionData(portal)
	assert.Nil(t, err, "unable to get portal lun information")

	t.Run("Checking portal entry added", func(t *testing.T) {
		assert.NotNil(t, portalLunData, "expected non null PortalLunData")
	})
	time.Sleep(10 * time.Millisecond)
	t.Run("Checking the entry added", func(t *testing.T) {
		assert.True(t, portalLunData.LUNs.CheckLUNExists(101))
	})

	m.RemoveLUNFromPortal(portal, 101)
	t.Run("Checking the entry added", func(t *testing.T) {
		assert.False(t, portalLunData.LUNs.CheckLUNExists(101), "expected LUN should exist")
	})
	m.RemoveLUNFromPortal(portal, 202)
}

func TestLUNInfo_IdentifyMissingLUNs_NonEmpty(t *testing.T) {
	l := LUNs{Info: make(map[int32]string)}
	l.AddLUNs([]LUNData{{11, ""}, {22, ""}, {33, ""}, {44, ""}, {55, ""}})

	m := LUNs{Info: make(map[int32]string)}
	m.AddLUN(LUNData{101, ""})
	m.AddLUNs([]LUNData{{101, ""}, {102, ""}, {103, ""}})
	m.AddLUNs([]LUNData{{22, ""}, {33, ""}})

	ret1 := l.IdentifyMissingLUNs(m)
	retExpected := []int32{101, 102, 103}
	t.Run("Verifying missed LUNs", func(t *testing.T) {
		assert.ElementsMatch(t, ret1, retExpected, "expected two lists should be equal")
	})
}

func TestLUNInfo_IdentifyMissingLUNs_Empty(t *testing.T) {
	var l LUNs
	m := LUNs{Info: make(map[int32]string)}
	ret := l.IdentifyMissingLUNs(m)
	t.Run("Verifying empty LUNs", func(t *testing.T) {
		assert.Zero(t, len(ret), "expected empty list of LUNs")
	})

	l = LUNs{Info: make(map[int32]string)}

	ret = l.IdentifyMissingLUNs(m)
	t.Run("Verifying empty LUNs", func(t *testing.T) {
		assert.Zero(t, len(ret), "expected empty list of LUNs")
	})

	nilVolID := ""
	l1 := []int32{11}
	l.AddLUNs([]LUNData{{11, ""}})

	ret = m.IdentifyMissingLUNs(l)
	t.Run("Verifying missed LUNs", func(t *testing.T) {
		assert.ElementsMatch(t, l1, ret, "expected two lists should be equal")
	})

	m.AddLUN(LUNData{11, nilVolID})
	m.AddLUN(LUNData{22, nilVolID})
	m.AddLUNs([]LUNData{{22, nilVolID}, {44, nilVolID}, {55, nilVolID}})

	ret = m.IdentifyMissingLUNs(l)
	t.Run("Verifying all LUNs existed", func(t *testing.T) {
		assert.Nil(t, ret, "expected nil list")
	})
}

func TestPortalLUNMapping_GetLUNsForPortal(t *testing.T) {
	var err error
	var m ISCSISessions
	portalInfo := PortalInfo{ISCSITargetIQN: "dummy", FirstIdentifiedStaleAt: time.Time{}}

	lData1 := []LUNData{{101, ""}, {102, ""}, {103, ""}, {104, ""}}
	err = m.AddPortal("testportal1", portalInfo)
	assert.Nil(t, err, "expected no error adding a portal")
	err = m.AddLUNsToPortal("testportal1", lData1)
	assert.Nil(t, err, "expected no error adding list of LUNs")

	lData2 := []LUNData{{201, ""}, {202, ""}, {203, ""}, {204, ""}, {204, ""}, {205, ""}}
	err = m.AddPortal("testportal2", portalInfo)
	assert.Nil(t, err, "expected no error adding second portal")
	err = m.AddLUNsToPortal("testportal2", lData2)
	assert.Nil(t, err, "expected no error adding LUNs")

	err = m.ResetFirstIdentifiedStaleTimeForPortal("testportal1")
	assert.Nil(t, err, "expected no error setting fix attempt counter")
	err = m.ResetFirstIdentifiedStaleTimeForPortal("testportal2")
	assert.Nil(t, err, "expected no error setting fix attempt counter to second portal")
	v, _ := m.FirstIdentifiedStaleTimeForPortal("testportal1")
	t.Run("Verifying set LFA", func(t *testing.T) {
		assert.True(t, v == time.Time{}, "expected the value as it was set prior")
	})
	v, err = m.FirstIdentifiedStaleTimeForPortal("testportal2")
	t.Run("verifying reset LFA", func(t *testing.T) {
		assert.Nil(t, err, "expected no error in querying for counter")
		assert.Zero(t, v, "expected the value should be zero")
	})

	// test coverage
	err = m.ResetFirstIdentifiedStaleTimeForPortal("non-existing")
	assert.NotNil(t, err, "expected error setting fix attempt counter to non-existing portal")
	err = m.SetFirstIdentifiedStaleTimeForPortal("non-existing", time.Now())
	assert.NotNil(t, err, "expected portal entry not found error")
	m.RemoveAllPortals()
	t.Run("verifying clear contents", func(t *testing.T) {
		assert.True(t, m.IsEmpty(), "expected an empty map")
	})
}

func TestPortalLUNMapping_CheckPortalExists(t *testing.T) {
	var m ISCSISessions
	var err error
	portal := "10.191.21.163:3260"

	t.Run("verifying nil mapping", func(t *testing.T) {
		assert.False(t, m.CheckPortalExists(portal), "expected portal should not exist in map")
	})
	m = ISCSISessions{Info: make(map[string]*ISCSISessionData)}

	portalInfo := PortalInfo{ISCSITargetIQN: "", FirstIdentifiedStaleAt: time.Time{}}
	t.Run("verifying non-existing portal", func(t *testing.T) {
		assert.False(t, m.CheckPortalExists(portal))
	})

	// should not be added
	t.Run("verifying error on adding invalid portal info", func(t *testing.T) {
		err = m.AddPortal(portal, portalInfo)
		assert.NotNil(t, err, "expected error to be invalid portal, no IQN found")
	})

	credentials := IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "username",
		IscsiInitiatorSecret: "password",
		IscsiTargetUsername:  "username_1",
		IscsiTargetSecret:    "password_1",
	}
	portalInfo1 := PortalInfo{
		ISCSITargetIQN:         "IQN:2022 com.netapp-test",
		Credentials:            credentials,
		FirstIdentifiedStaleAt: time.Time{},
	}
	err = m.AddPortal(portal, portalInfo1)
	assert.Nil(t, err, "expected no error adding a portal")
	err = m.AddPortal(portal, portalInfo1)
	assert.NotNil(t, err, "expected info as error saying portal already exits")

	t.Run("verifying existence of portal", func(t *testing.T) {
		assert.True(t, m.CheckPortalExists(portal), "expected portal should exist in map")
	})

	portalLunData, err := m.ISCSISessionData(portal)
	assert.Nil(t, err, "unable to get portal lun information")

	t.Run("verifying IQN", func(t *testing.T) {
		assert.True(t, portalLunData.PortalInfo.ISCSITargetIQN == "IQN:2022 com.netapp-test")
	})
	time.Sleep(10 * time.Millisecond)
	t.Run("verifying chap in use", func(t *testing.T) {
		assert.True(t, portalLunData.PortalInfo.CHAPInUse())
	})
	time.Sleep(10 * time.Millisecond)

	credentials.IscsiTargetSecret = "password_2"
	portalLunData, err = m.ISCSISessionData(portal)
	assert.Nil(t, err, "unable to get portal lun information")
	portalLunData.PortalInfo.UpdateCHAPCredentials(credentials)

	c2 := portalLunData.PortalInfo.Credentials

	t.Run("verifying target secret", func(t *testing.T) {
		assert.True(t, c2.IscsiTargetSecret == "password_2")
	})
	t.Run("verifying chap credentials", func(t *testing.T) {
		assert.True(t, c2.IscsiUsername == "username")
	})

	portalLunData, err = m.ISCSISessionData(portal)
	assert.Nil(t, err, "unable to get portal lun information")
	c3 := portalLunData.PortalInfo.Credentials
	t.Run("verifying target secret", func(t *testing.T) {
		assert.True(t, c3.IscsiTargetSecret == "password_2")
	})

	lData := LUNData{22, "pvc-234dre66-1234-random"}
	_ = m.AddLUNToPortal(portal, lData)
	t.Run("checking volume ID", func(t *testing.T) {
		portalLunData, err = m.ISCSISessionData(portal)
		assert.Nil(t, err, "unable to get portal lun information")

		lInfo := portalLunData.LUNs
		if v, ok := lInfo.VolumeId(22); ok == nil {
			assert.True(t, v == "pvc-234dre66-1234-random")
		}
		_, ok := lInfo.VolumeId(33) // non-existing, should return some error
		assert.NotNil(t, ok, "expected an error on querying non-existing entry")
	})
	t.Run("Checking removal of an entry", func(t *testing.T) {
		m.RemovePortal(portal)
		assert.True(t, m.IsEmpty(), "expected map should be empty")
	})
}

func TestLUNInfo_IsEmpty(t *testing.T) {
	p1 := LUNs{Info: nil}
	t.Run("Verifying methods on empty struct", func(t *testing.T) {
		assert.True(t, p1.IsEmpty())
		assert.False(t, p1.CheckLUNExists(121))

		listLuns := p1.AllLUNs()
		assert.Empty(t, listLuns, "expected empty list of LUNs")

		p1.AddLUN(LUNData{120, "dummy"})
		assert.False(t, p1.IsEmpty())

		v, err := p1.VolumeId(22)
		assert.True(t, v == "")
		assert.NotNil(t, err, "expected error trying to get VolumeId for non-existing LUN id")
	})

	p2 := LUNs{Info: nil}
	t.Run("Verifying add LUNs on empty map", func(t *testing.T) {
		l := []LUNData{{120, "pvc-One-Hundred-And-Twenty"}, {13, "pvc-Thirteen"}}
		p2.AddLUNs(l)
		assert.False(t, p2.IsEmpty(), "expected non-empty LUN info")

		v, err := p2.VolumeId(120)
		assert.True(t, v == "pvc-One-Hundred-And-Twenty", "expected volume id to be as set earlier")
		assert.Nil(t, err, "expected no error on getting volume ID")

		p2.RemoveLUN(120)
		p2.RemoveLUN(13)
		assert.True(t, p2.IsEmpty(), "expected LUN info to be empty")
	})
}

func TestPortalInfo_IsValid(t *testing.T) {
	v := PortalInfo{}
	t.Run("verifying methods on null pointer portalinfo", func(t *testing.T) {
		assert.False(t, v.HasTargetIQN(), "expected portal info to be missing Target IQN")
		assert.False(t, v.String() == "", "expected portal info to be non-empty")
		assert.True(t, v.ISCSITargetIQN == "", "expected target IQN to be empty")
		assert.False(t, v.CHAPInUse(), "expected CHAP in use to be false")

		c := v.Credentials
		assert.True(t, c.IscsiUsername == "", "expected iscsi user name to be empty")
		assert.True(t, v.FirstIdentifiedStaleAt == time.Time{}, "expected fix attempt counter to be 0")
	})
}

func TestBuildLunData(t *testing.T) {
	listLUNs := []int32{11, 22, 33}
	listLUNData := BuildLUNData(listLUNs)
	t.Run("Verifying volumeIDs to be blank", func(t *testing.T) {
		for _, lData := range listLUNData {
			assert.Empty(t, lData.VolumeId, "expected volumeID to be empty string")
		}
	})
}

func TestPortalLunMapping_GetPortalInfo(t *testing.T) {
	var err error
	var p ISCSISessions

	portal := "10.191.21.163:3260"
	unknownPortal := "10.10.10.10:3260"

	credentials := IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "username",
		IscsiInitiatorSecret: "password",
		IscsiTargetUsername:  "username_1",
		IscsiTargetSecret:    "password_1",
	}
	portalInfo := PortalInfo{
		ISCSITargetIQN:         "IQN:2022 com.netapp-123-456",
		Credentials:            credentials,
		FirstIdentifiedStaleAt: time.Time{},
	}

	err = p.AddPortal(portal, portalInfo)
	assert.True(t, err == nil)

	t.Run("Verifying get portal info", func(t *testing.T) {
		portalInfo, err := p.PortalInfo(portal)
		assert.True(t, err == nil)
		assert.Equal(t, portalInfo.ISCSITargetIQN, "IQN:2022 com.netapp-123-456")
	})

	t.Run("Verifying invalid portal case", func(t *testing.T) {
		_, err = p.PortalInfo("")
		assert.True(t, err != nil)
	})

	t.Run("Verifying unknown portal case", func(t *testing.T) {
		_, err = p.PortalInfo(unknownPortal)
		assert.True(t, err != nil)
	})

	t.Run("Verifying empty portal lun mapping case", func(t *testing.T) {
		pe := &ISCSISessions{Info: make(map[string]*ISCSISessionData)}
		_, err = pe.PortalInfo(portal)
		assert.True(t, err != nil)
	})
}

func TestPortalLUNMapping_GetLUNInfo(t *testing.T) {
	var err error
	var p ISCSISessions
	portal := "10.191.21.163:3260"

	unknownPortal := "10.10.10.10:3260"
	credentials := IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "username",
		IscsiInitiatorSecret: "password",
		IscsiTargetUsername:  "username_1",
		IscsiTargetSecret:    "password_1",
	}
	portalInfo := PortalInfo{
		ISCSITargetIQN:         "IQN:2022 com.netapp-123-456",
		Credentials:            credentials,
		FirstIdentifiedStaleAt: time.Time{},
		LastAccessTime:         time.Time{},
	}

	err = p.AddPortal(portal, portalInfo)
	assert.True(t, err == nil)

	p.AddLUNToPortal(portal, LUNData{100, "pvc-1111111-2222-3333-4444-55555555555555"})

	t.Run("Verifying get LUN info", func(t *testing.T) {
		LUNData, err := p.LUNInfo(portal)
		assert.True(t, err == nil)
		assert.Equal(t, LUNData.AllLUNs(), []int32{100})

		volID, err := LUNData.VolumeId(100)
		assert.True(t, err == nil)
		assert.Equal(t, volID, "pvc-1111111-2222-3333-4444-55555555555555")
	})

	t.Run("Verifying invalid portal case", func(t *testing.T) {
		_, err := p.LUNInfo("")
		assert.True(t, err != nil)
	})

	t.Run("Verifying unknown portal case", func(t *testing.T) {
		_, err := p.LUNInfo(unknownPortal)
		assert.True(t, err != nil)
	})

	t.Run("Verifying empty portal LUN mapping case", func(t *testing.T) {
		pe := &ISCSISessions{Info: make(map[string]*ISCSISessionData)}
		_, err := pe.LUNInfo(unknownPortal)
		assert.True(t, err != nil)
	})
}

func TestPortalLUNMapping_CreatePublishInfo(t *testing.T) {
	var err error
	var p ISCSISessions
	var publishInfo VolumePublishInfo

	portal := "10.191.21.163:3260"
	unknownPortal := "10.10.10.10:3260"

	credentials := IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "username",
		IscsiInitiatorSecret: "password",
		IscsiTargetUsername:  "username_1",
		IscsiTargetSecret:    "password_1",
	}
	portalInfo := PortalInfo{
		ISCSITargetIQN:         "IQN:2022 com.netapp-123-456",
		Credentials:            credentials,
		FirstIdentifiedStaleAt: time.Time{},
		LastAccessTime:         time.Time{},
	}

	err = p.AddPortal(portal, portalInfo)
	assert.True(t, err == nil)

	t.Run("Verifying create publish info", func(t *testing.T) {
		publishInfo, err = p.GeneratePublishInfo(portal)
		assert.True(t, err == nil)
		assert.Equal(t, publishInfo.IscsiUsername, "username")
	})

	t.Run("Verifying invalid portal case", func(t *testing.T) {
		_, err = p.GeneratePublishInfo("")
		assert.True(t, err != nil)
	})

	t.Run("Verifying unknown portal case", func(t *testing.T) {
		_, err = p.GeneratePublishInfo(unknownPortal)
		assert.True(t, err != nil)
	})

	t.Run("Verifying empty portal LUN mapping case", func(t *testing.T) {
		pe := &ISCSISessions{Info: make(map[string]*ISCSISessionData)}
		publishInfo, err = pe.GeneratePublishInfo(portal)
		assert.True(t, err != nil)
	})
}

func TestPortalLUNMapping_UpdateChapInfoForPortal(t *testing.T) {
	var err error
	var p ISCSISessions

	portal := "10.191.21.163:3260"
	unknownPortal := "10.10.10.10:3260"

	credentials := IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "username",
		IscsiInitiatorSecret: "password",
		IscsiTargetUsername:  "username_1",
		IscsiTargetSecret:    "password_1",
	}
	portalInfo := PortalInfo{
		ISCSITargetIQN:         "IQN:2022 com.netapp-123-456",
		Credentials:            credentials,
		FirstIdentifiedStaleAt: time.Time{},
		LastAccessTime:         time.Time{},
	}

	newCredentials := IscsiChapInfo{
		UseCHAP:              true,
		IscsiUsername:        "username",
		IscsiInitiatorSecret: "password",
		IscsiTargetUsername:  "username_new",
		IscsiTargetSecret:    "password_1",
	}

	err = p.AddPortal(portal, portalInfo)
	assert.True(t, err == nil)

	/*t.Run("Verify update chap info for a portal", func(t *testing.T) {
		err = p.UpdateCHAPForPortal(portal, newCredentials)
		assert.True(t, err == nil)

		portalInfoNew, err := p.PortalInfo(portal)
		assert.True(t, err == nil)

		newChapInfo := portalInfoNew.GetCHAPCredentials()
		assert.Equal(t, "username_new", newChapInfo.IscsiTargetUsername)
	})*/

	t.Run("Verifying invalid portal case", func(t *testing.T) {
		err = p.UpdateCHAPForPortal("", newCredentials)
		assert.True(t, err != nil)
	})

	// To cover invalid portal case
	t.Run("Verifying unknown portal case", func(t *testing.T) {
		err = p.UpdateCHAPForPortal(unknownPortal, newCredentials)
		assert.True(t, err != nil)
	})

	t.Run("Verifying empty portal LUN mapping case", func(t *testing.T) {
		pe := &ISCSISessions{Info: make(map[string]*ISCSISessionData)}
		err = pe.UpdateCHAPForPortal(portal, newCredentials)
		assert.True(t, err != nil)
	})
}
