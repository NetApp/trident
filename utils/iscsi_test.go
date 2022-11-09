// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	. "github.com/netapp/trident/logger"
)

func TestFormatPortal(t *testing.T) {
	type IPAddresses struct {
		InputPortal  string
		OutputPortal string
	}
	tests := []IPAddresses{
		{
			InputPortal:  "203.0.113.1",
			OutputPortal: "203.0.113.1:3260",
		},
		{
			InputPortal:  "203.0.113.1:3260",
			OutputPortal: "203.0.113.1:3260",
		},
		{
			InputPortal:  "203.0.113.1:3261",
			OutputPortal: "203.0.113.1:3261",
		},
		{
			InputPortal:  "[2001:db8::1]",
			OutputPortal: "[2001:db8::1]:3260",
		},
		{
			InputPortal:  "[2001:db8::1]:3260",
			OutputPortal: "[2001:db8::1]:3260",
		},
		{
			InputPortal:  "[2001:db8::1]:3261",
			OutputPortal: "[2001:db8::1]:3261",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputPortal, func(t *testing.T) {
			assert.Equal(t, testCase.OutputPortal, formatPortal(testCase.InputPortal), "Portal not correctly formatted")
		})
	}
}

func TestFilterTargets(t *testing.T) {
	type FilterCase struct {
		CommandOutput string
		InputPortal   string
		OutputIQNs    []string
	}
	tests := []FilterCase{
		{
			// Simple positive test, expect first
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"203.0.113.3:3260,-1 iqn.2010-01.com.solidfire:baz\n",
			InputPortal: "203.0.113.1:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:foo"},
		},
		{
			// Simple positive test, expect second
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:bar"},
		},
		{
			// Expect empty list
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n",
			InputPortal: "203.0.113.3:3260",
			OutputIQNs:  []string{},
		},
		{
			// Expect multiple
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:baz\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:bar", "iqn.1992-08.com.netapp:baz"},
		},
		{
			// Bad input
			CommandOutput: "" +
				"Foobar\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{},
		},
		{
			// Good and bad input
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"Foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"Bar\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:baz\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:bar", "iqn.1992-08.com.netapp:baz"},
		},
		{
			// Try nonstandard port number
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"203.0.113.2:3261,1025 iqn.1992-08.com.netapp:baz\n",
			InputPortal: "203.0.113.2:3261",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:baz"},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputPortal, func(t *testing.T) {
			targets := filterTargets(testCase.CommandOutput, testCase.InputPortal)
			assert.Equal(t, testCase.OutputIQNs, targets, "Wrong targets returned")
		})
	}
}

func TestParseInitiatorIQNs(t *testing.T) {
	ctx := context.TODO()
	tests := map[string]struct {
		input     string
		output    []string
		predicate func(string) []string
	}{
		"Single valid initiator": {
			input:  "InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de",
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"initiator with space": {
			input:  "InitiatorName=iqn 2005-03.org.open-iscsi:123abc456de",
			output: []string{"iqn"},
		},
		"Multiple valid initiators": {
			input: `InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de
InitiatorName=iqn.2005-03.org.open-iscsi:secondIQN12`,
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de", "iqn.2005-03.org.open-iscsi:secondIQN12"},
		},
		"Ignore comment initiator": {
			input: `#InitiatorName=iqn.1994-05.demo.netapp.com
InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de`,
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Ignore inline comment": {
			input:  "InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de #inline comment in file",
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Tolerate space around equal sign": {
			input:  "InitiatorName = iqn.2005-03.org.open-iscsi:123abc456de",
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Tolerate leading space": {
			input:  " InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de",
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Tolerate trailing space multiple initiators": {
			input: `InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de
InitiatorName=iqn.2005-03.org.open-iscsi:secondIQN12 `,
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de", "iqn.2005-03.org.open-iscsi:secondIQN12"},
		},
		"Full iscsi file": {
			input: `## DO NOT EDIT OR REMOVE THIS FILE!
## If you remove this file, the iSCSI daemon will not start.
## If you change the InitiatorName, existing access control lists
## may reject this initiator.  The InitiatorName must be unique
## for each iSCSI initiator.  Do NOT duplicate iSCSI InitiatorNames.
InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de
#InitiatorName=iqn.1994-05.demo.netapp.com`,
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Full iscsi file no initiator": {
			input: `## DO NOT EDIT OR REMOVE THIS FILE!
## If you remove this file, the iSCSI daemon will not start.
## If you change the InitiatorName, existing access control lists
## may reject this initiator.  The InitiatorName must be unique
## for each iSCSI initiator.  Do NOT duplicate iSCSI InitiatorNames.
#InitiatorName=iqn.1994-05.demo.netapp.com`,
			output: []string{},
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			iqns := parseInitiatorIQNs(ctx, test.input)
			assert.Equal(t, test.output, iqns, "Failed to parse initiators")
		})
	}
}

func TestPortalLUNMapping_IsEmpty(t *testing.T) {
	testCtx := context.Background()
	var p *PortalLUNMapping
	var m PortalLUNMapping
	var err error

	// Null pointer operations.
	t.Run("Verifying null pointer references", func(t *testing.T) {
		assert.True(t, p.IsEmpty(), "expected the null pointer returns empty map")
		// Code coverage
		p.RemoveAllPortals()
	})

	t.Run("Verifying invalid portal info", func(t *testing.T) {
		err = p.AddPortal("testPortal", PortalInfo{ISCSITargetIQN: "", LastFixAttempt: 0})
		assert.NotNil(t, err, "expected error adding entry to null map pointer")
	})

	t.Run("Verifying valid portal but null pointer of map", func(t *testing.T) {
		err = p.AddPortal("testportal", PortalInfo{ISCSITargetIQN: "dummy", LastFixAttempt: 100})
		assert.NotNil(t, err, "expected error adding portal to null pointer")
		assert.True(t, p.IsEmpty())
	})

	t.Run("Verifying addLun to non-existing portal", func(t *testing.T) {
		err = p.AddLUNToPortal("testPortal", LUNData{0, ""})
		assert.NotNil(t, err, "expected error adding LUN to non-existing portal")
		assert.True(t, p.IsEmpty(), "expected no entries added to map")
	})

	t.Run("Checking for empty portal map", func(t *testing.T) {
		assert.True(t, m.IsEmpty())
		m.RemoveLUNFromPortal("non-existing", 101)
		assert.True(t, m.IsEmpty())
	})

	Logc(testCtx).Debugln(m)
	Logc(testCtx).Debugf("test for empty map print: %s\n", m.String())

	portalInfo := PortalInfo{ISCSITargetIQN: "dummy", LastFixAttempt: 0}
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

	// Code coverage
	Logc(testCtx).Debugf("portal-LUN mapping: %s\n", m.String())
}

func TestPortalLUNMapping_RemovePortal(t *testing.T) {
	var err error
	m := PortalLUNMapping{PortalToLUNMapping: make(map[string]PortalLUNData)}

	t.Run("Verifying empty mapping", func(t *testing.T) {
		assert.True(t, m.IsEmpty())
	})
	portal := "10.191.21.163:3260"
	portalInfo := PortalInfo{ISCSITargetIQN: "dummy", LastFixAttempt: 0}
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
	m := PortalLUNMapping{PortalToLUNMapping: make(map[string]PortalLUNData)}
	portal := "10.191.21.163:3260"
	portalInfo := PortalInfo{ISCSITargetIQN: "dummy", LastFixAttempt: 0}
	err = m.AddPortal(portal, portalInfo)
	assert.Nil(t, err, "expected no error adding portal")
	err = m.AddLUNToPortal(portal, LUNData{101, ""})
	assert.Nil(t, err, "expected no error while adding single LUN")
	err = m.AddLUNToPortal(portal, LUNData{202, ""})
	assert.Nil(t, err, "expected no error while adding a LUN")

	portalLUNData := m.GetPortalLUNMapping(portal)
	t.Run("Checking portal entry added", func(t *testing.T) {
		assert.NotNil(t, portalLUNData, "expected non null PortalLunData")
	})
	time.Sleep(10 * time.Millisecond)
	t.Run("Checking the entry added", func(t *testing.T) {
		assert.True(t, portalLUNData.LUNInfoValue.CheckLUNExists(101))
	})

	m.RemoveLUNFromPortal(portal, 101)
	t.Run("Checking the entry added", func(t *testing.T) {
		assert.False(t, portalLUNData.LUNInfoValue.CheckLUNExists(101), "expected LUN should exist")
	})
	m.RemoveLUNFromPortal(portal, 202)
}

func TestPortalLUNMapping_IncLastFixAttemptForPortal(t *testing.T) {
	var err error
	var lfa int32

	m := PortalLUNMapping{PortalToLUNMapping: make(map[string]PortalLUNData)}
	portal := "10.191.21.163:3260"
	portalInfo := PortalInfo{ISCSITargetIQN: "dummy", LastFixAttempt: 0}
	err = m.AddPortal(portal, portalInfo)
	assert.Nil(t, err, "expected no error while adding portal to map")
	err = m.AddLUNToPortal(portal, LUNData{101, ""})
	assert.Nil(t, err, "expected no error while adding a LUN")

	// increment twice
	err = m.IncLastFixAttemptForPortal(portal)
	assert.Nil(t, err, "expected no error incrementing counter")
	err = m.IncLastFixAttemptForPortal(portal)
	assert.Nil(t, err, "expected no error incrementing counter")

	// check the counter equals to
	if lfa, err = m.GetLastFixAttemptForPortal(portal); err == nil {
		t.Run("Verifying the LastFixAttempt", func(t *testing.T) {
			assert.True(t, lfa == 2, "expected last fix attempt counter to be 2")
		})
	}

	err = m.SetLastFixAttemptForPortal(portal, 0)
	assert.Nil(t, err, "expected no error resetting fix attempt counter")
	if lfa, err = m.GetLastFixAttemptForPortal(portal); err == nil {
		t.Run("Verifying counter reset", func(t *testing.T) {
			assert.Zero(t, lfa, "expected last fix attempt to be zero")
		})
	}
}

func TestLUNInfo_IdentifyMissingLUNs_NonEmpty(t *testing.T) {
	l := LUNInfo{LUNs: make(map[int32]string)}
	l.AddLUNs([]LUNData{{11, ""}, {22, ""}, {33, ""}, {44, ""}, {55, ""}})

	m := LUNInfo{LUNs: make(map[int32]string)}
	m.AddLUN(LUNData{101, ""})
	m.AddLUNs([]LUNData{{101, ""}, {102, ""}, {103, ""}})
	m.AddLUNs([]LUNData{{22, ""}, {33, ""}})

	ret1 := l.IdentifyMissingLUNs(m)
	retExpected := []int32{55, 11, 44}
	t.Run("Verifying missed LUNs", func(t *testing.T) {
		assert.ElementsMatch(t, ret1, retExpected, "expected two lists should be equal")
	})
}

func TestLUNInfo_IdentifyMissingLUNs_Empty(t *testing.T) {
	var l LUNInfo
	m := LUNInfo{LUNs: make(map[int32]string)}
	ret := l.IdentifyMissingLUNs(m)
	t.Run("Verifying empty LUNs", func(t *testing.T) {
		assert.Zero(t, len(ret), "expected empty list of LUNs")
	})

	l = LUNInfo{LUNs: make(map[int32]string)}

	ret = l.IdentifyMissingLUNs(m)
	t.Run("Verifying empty LUNs", func(t *testing.T) {
		assert.Zero(t, len(ret), "expected empty list of LUNs")
	})

	nilVolID := ""
	l1 := []int32{11}
	l.AddLUNs([]LUNData{{11, ""}})

	ret = l.IdentifyMissingLUNs(m)
	t.Run("Verifying missed LUNs", func(t *testing.T) {
		assert.ElementsMatch(t, l1, ret, "expected two lists should be equal")
	})

	m.AddLUN(LUNData{11, nilVolID})
	m.AddLUN(LUNData{22, nilVolID})
	m.AddLUNs([]LUNData{{22, nilVolID}, {44, nilVolID}, {55, nilVolID}})

	ret = l.IdentifyMissingLUNs(m)
	t.Run("Verifying all LUNs existed", func(t *testing.T) {
		assert.Nil(t, ret, "expected nil list")
	})
}

func TestPortalLUNMapping_GetLUNsForPortal(t *testing.T) {
	testctx := context.Background()
	var err error
	var m PortalLUNMapping
	portalInfo := PortalInfo{ISCSITargetIQN: "dummy", LastFixAttempt: 0}

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

	// test coverage for non-existing
	if lfa, err := m.GetLastFixAttemptForPortal("non-existing"); err != nil {
		Logc(testctx).Debugf("Error reading lfa: %v and lfa returned: %d\n", err, lfa)
	} else {
		Logc(testctx).Debugf("LFA read for non-existing portal - %d\n", lfa)
	}

	if luns, err := m.GetLUNsForPortal("non-existing"); err != nil {
		Logc(testctx).Debugf("Error: %v\n", err)
	} else {
		Logc(testctx).Debugf("Luns returned: %v\n", luns)
	}

	if luns, err := m.GetLUNsForPortal("testportal2"); err != nil {
		Logc(testctx).Debugf("Error: %v\n", err)
	} else {
		Logc(testctx).Debugf("portal(testportal1) Luns returned: %v\n", luns)
		pLuns := []int32{201, 202, 203, 204, 205}
		t.Run("Verifying LUNs extracted", func(t *testing.T) {
			assert.ElementsMatch(t, luns, pLuns, "expected equal set of elements")
		})
	}

	Logc(testctx).Debugf("portal(testportal2) - Mapped Luns: %v\n", m.GetPortalLUNMapping("testportal2").PortalInfoValue)

	err = m.SetLastFixAttemptForPortal("testportal1", 1000)
	assert.Nil(t, err, "expected no error setting fix attempt counter")
	err = m.SetLastFixAttemptForPortal("testportal2", 2000)
	assert.Nil(t, err, "expected no error setting fix attempt counter to second portal")
	v, _ := m.GetLastFixAttemptForPortal("testportal1")
	t.Run("Verifying set LFA", func(t *testing.T) {
		assert.True(t, 1000 == v, "expected the value as it was set prior")
	})
	m.ResetAllLastFixAttempt()
	v, err = m.GetLastFixAttemptForPortal("testportal2")
	t.Run("verifying reset LFA", func(t *testing.T) {
		assert.Nil(t, err, "expected no error in querying for counter")
		assert.Zero(t, v, "expected the value should be zero")
	})

	// test coverage
	err = m.SetLastFixAttemptForPortal("non-existing", 100)
	assert.NotNil(t, err, "expected error setting fix attempt counter to non-existing portal")
	err = m.IncLastFixAttemptForPortal("non-existing")
	assert.NotNil(t, err, "expected portal entry not found error")
	m.RemoveAllPortals()
	t.Run("verifying clear contents", func(t *testing.T) {
		assert.True(t, m.IsEmpty(), "expected an empty map")
	})

	// test coverage
	m.ResetAllLastFixAttempt()
}

func TestPortalLUNMapping_CheckPortalExists(t *testing.T) {
	var m PortalLUNMapping
	var err error
	portal := "10.191.21.163:3260"

	t.Run("verifying nil mapping", func(t *testing.T) {
		assert.False(t, m.CheckPortalExists(portal), "expected portal should not exist in map")
	})
	m = PortalLUNMapping{PortalToLUNMapping: make(map[string]PortalLUNData)}

	portalInfo := PortalInfo{ISCSITargetIQN: "", LastFixAttempt: 0}
	t.Run("verifying non-existing portal", func(t *testing.T) {
		assert.False(t, m.CheckPortalExists(portal))
	})

	// should not be added
	t.Run("verifying error on adding invalid portal info", func(t *testing.T) {
		err = m.AddPortal(portal, portalInfo)
		assert.NotNil(t, err, "expected error to be invalid portal, no IQN found")
	})

	credentials := CHAPCredentials{
		ISCSIUsername:        "username",
		ISCSIInitiatorSecret: "password",
		ISCSITargetUsername:  "username_1",
		ISCSITargetSecret:    "password_1",
	}
	portalInfo1 := PortalInfo{
		ISCSITargetIQN: "IQN:2022 com.netapp-test",
		UseCHAP:        true,
		Credentials:    credentials,
		LastFixAttempt: 101,
	}
	err = m.AddPortal(portal, portalInfo1)
	assert.Nil(t, err, "expected no error adding a portal")
	err = m.AddPortal(portal, portalInfo1)
	assert.NotNil(t, err, "expected info as error saying portal already exits")

	t.Run("verifying existence of portal", func(t *testing.T) {
		assert.True(t, m.CheckPortalExists(portal), "expected portal should exist in map")
	})

	portalLUNData := m.GetPortalLUNMapping(portal)
	t.Run("verifying IQN", func(t *testing.T) {
		assert.True(t, portalLUNData.PortalInfoValue.GetTargetIQN() == "IQN:2022 com.netapp-test")
	})
	time.Sleep(10 * time.Millisecond)
	t.Run("verifying chap in use", func(t *testing.T) {
		assert.True(t, portalLUNData.PortalInfoValue.CHAPInUse())
	})
	time.Sleep(10 * time.Millisecond)

	credentials.ISCSITargetSecret = "password_2"
	portalInfoValue := m.GetPortalLUNMapping(portal).PortalInfoValue
	portalInfoValue.UpdateCHAPCredentials(credentials)

	c2 := portalInfoValue.GetCHAPCredentials()

	t.Run("verifying target secret", func(t *testing.T) {
		assert.True(t, c2.ISCSITargetSecret == "password_2")
	})
	t.Run("verifying chap credentials", func(t *testing.T) {
		assert.True(t, c2.ISCSIUsername == "username")
	})

	c3 := m.GetPortalLUNMapping(portal).PortalInfoValue.GetCHAPCredentials()
	t.Run("verifying target secret", func(t *testing.T) {
		assert.True(t, c3.ISCSITargetSecret == "password_2")
	})

	lData := LUNData{22, "pvc-234dre66-1234-random"}
	_ = m.AddLUNToPortal(portal, lData)
	t.Run("checking volume ID", func(t *testing.T) {
		lInfo := m.GetPortalLUNMapping(portal).LUNInfoValue
		if v, ok := lInfo.GetVolumeId(22); ok == nil {
			assert.True(t, v == "pvc-234dre66-1234-random")
		}
		_, ok := lInfo.GetVolumeId(33) // non-existing, should return some error
		assert.NotNil(t, ok, "expected an error on querying non-existing entry")
	})
	t.Run("Checking removal of an entry", func(t *testing.T) {
		m.RemovePortal(portal)
		assert.True(t, m.IsEmpty(), "expected map should be empty")
	})
}

func TestLUNInfo_IsEmpty(t *testing.T) {
	// Verifying null pointer checks for LUNInfo
	var p *LUNInfo
	t.Run("Verifying methods on null pointer", func(t *testing.T) {
		assert.True(t, p.IsEmpty())
		assert.False(t, p.CheckLUNExists(121))

		p.RemoveLUN(121)
		listLuns := p.GetAllLUNs()
		assert.Zero(t, len(listLuns), "expected empty list")

		p.AddLUN(LUNData{120, "dummy"})
		// Should not have added any as p is nil.
		assert.True(t, p.IsEmpty())

		l := []LUNData{{120, ""}, {13, ""}}
		p.AddLUNs(l)
		// should not have added any as p is nil.
		assert.True(t, p.IsEmpty())

		v, err := p.GetVolumeId(22)
		assert.Empty(t, v, "expected volume id should be empty")
		assert.NotNil(t, err, "expected error querying non-existing LUN id")
	})

	p1 := LUNInfo{LUNs: nil}
	t.Run("Verifying methods on empty struct", func(t *testing.T) {
		assert.True(t, p1.IsEmpty())
		assert.False(t, p1.CheckLUNExists(121))

		listLuns := p1.GetAllLUNs()
		assert.Empty(t, listLuns, "expected empty list of LUNs")

		p1.AddLUN(LUNData{120, "dummy"})
		assert.False(t, p1.IsEmpty())

		v, err := p.GetVolumeId(22)
		assert.True(t, v == "")
		assert.NotNil(t, err, "expected error trying to get VolumeId for non-existing LUN id")
	})

	p2 := LUNInfo{LUNs: nil}
	t.Run("Verifying add LUNs on empty map", func(t *testing.T) {
		l := []LUNData{{120, "pvc-One-Hundred-And-Twenty"}, {13, "pvc-Thirteen"}}
		p2.AddLUNs(l)
		assert.False(t, p2.IsEmpty(), "expected non-empty LUN info")

		v, err := p2.GetVolumeId(120)
		assert.True(t, v == "pvc-One-Hundred-And-Twenty", "expected volume id to be as set earlier")
		assert.Nil(t, err, "expected no error on getting volume ID")

		p2.RemoveLUN(120)
		p2.RemoveLUN(13)
		assert.True(t, p2.IsEmpty(), "expected LUN info to be empty")
	})
}

func TestPortalInfo_IsValid(t *testing.T) {
	var p *PortalInfo
	t.Run("verifying methods on null pointer portalinfp", func(t *testing.T) {
		assert.False(t, p.IsValid(), "expected portal info to be invalid")
		assert.True(t, p.String() == "", "expected portal info to be empty")
		assert.True(t, p.GetTargetIQN() == "", "expected target IQN to be empty")
		assert.False(t, p.CHAPInUse(), "expected CHAP in use to be false")

		c := p.GetCHAPCredentials()
		assert.True(t, c.ISCSIUsername == "")
		assert.True(t, p.GetLastFixAttempt() == -1)
	})

	v := PortalInfo{}
	t.Run("verifying methods on null pointer portalinfo", func(t *testing.T) {
		assert.False(t, v.IsValid(), "expected portal info to be invalid")
		assert.False(t, v.String() == "", "expected portal info to be non-empty")
		assert.True(t, v.GetTargetIQN() == "", "expected target IQN to be empty")
		assert.False(t, v.CHAPInUse(), "expected CHAP in use to be false")

		c := v.GetCHAPCredentials()
		assert.True(t, c.ISCSIUsername == "", "expected iscsi user name to be empty")
		assert.True(t, v.GetLastFixAttempt() == 0, "expected fix attempt counter to be 0")
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
	var p PortalLUNMapping

	portal := "10.191.21.163:3260"
	unknownPortal := "10.10.10.10:3260"

	credentials := CHAPCredentials{
		ISCSIUsername:        "username",
		ISCSIInitiatorSecret: "password",
		ISCSITargetUsername:  "username_1",
		ISCSITargetSecret:    "password_1",
	}
	portalInfo := PortalInfo{
		ISCSITargetIQN: "IQN:2022 com.netapp-123-456",
		UseCHAP:        true,
		Credentials:    credentials,
		LastFixAttempt: 101,
	}

	err = p.AddPortal(portal, portalInfo)
	assert.True(t, err == nil)

	t.Run("Verifying get portal info", func(t *testing.T) {
		portalInfo, err := p.GetPortalInfo(portal)
		assert.True(t, err == nil)
		assert.Equal(t, portalInfo.ISCSITargetIQN, "IQN:2022 com.netapp-123-456")
	})

	t.Run("Verifying invalid portal case", func(t *testing.T) {
		_, err = p.GetPortalInfo("")
		assert.True(t, err != nil)
	})

	t.Run("Verifying unknown portal case", func(t *testing.T) {
		_, err = p.GetPortalInfo(unknownPortal)
		assert.True(t, err != nil)
	})

	t.Run("Verifying empty portal lun mapping case", func(t *testing.T) {
		pe := &PortalLUNMapping{PortalToLUNMapping: make(map[string]PortalLUNData)}
		_, err = pe.GetPortalInfo(portal)
		assert.True(t, err != nil)
	})
}

func TestPortalLUNMapping_GetLUNInfo(t *testing.T) {
	var err error
	var p PortalLUNMapping
	portal := "10.191.21.163:3260"

	unknownPortal := "10.10.10.10:3260"
	credentials := CHAPCredentials{
		ISCSIUsername:        "username",
		ISCSIInitiatorSecret: "password",
		ISCSITargetUsername:  "username_1",
		ISCSITargetSecret:    "password_1",
	}
	portalInfo := PortalInfo{
		ISCSITargetIQN: "IQN:2022 com.netapp-123-456",
		UseCHAP:        true,
		Credentials:    credentials,
		LastFixAttempt: 101,
	}

	err = p.AddPortal(portal, portalInfo)
	assert.True(t, err == nil)

	p.AddLUNToPortal(portal, LUNData{100, "pvc-1111111-2222-3333-4444-55555555555555"})

	t.Run("Verifying get LUN info", func(t *testing.T) {
		LUNData, err := p.GetLunInfo(portal)
		assert.True(t, err == nil)
		assert.Equal(t, LUNData.GetAllLUNs(), []int32{100})

		volID, err := LUNData.GetVolumeId(100)
		assert.True(t, err == nil)
		assert.Equal(t, volID, "pvc-1111111-2222-3333-4444-55555555555555")
	})

	t.Run("Verifying invalid portal case", func(t *testing.T) {
		_, err := p.GetLunInfo("")
		assert.True(t, err != nil)
	})

	t.Run("Verifying unknown portal case", func(t *testing.T) {
		_, err := p.GetLunInfo(unknownPortal)
		assert.True(t, err != nil)
	})

	t.Run("Verifying empty portal LUN mapping case", func(t *testing.T) {
		pe := &PortalLUNMapping{PortalToLUNMapping: make(map[string]PortalLUNData)}
		_, err := pe.GetLunInfo(unknownPortal)
		assert.True(t, err != nil)
	})
}

func TestPortalLUNMapping_CreatePublishInfo(t *testing.T) {
	var err error
	var p PortalLUNMapping
	var publishInfo *VolumePublishInfo

	portal := "10.191.21.163:3260"
	unknownPortal := "10.10.10.10:3260"

	credentials := CHAPCredentials{
		ISCSIUsername:        "username",
		ISCSIInitiatorSecret: "password",
		ISCSITargetUsername:  "username_1",
		ISCSITargetSecret:    "password_1",
	}
	portalInfo := PortalInfo{
		ISCSITargetIQN: "IQN:2022 com.netapp-123-456",
		UseCHAP:        true,
		Credentials:    credentials,
		LastFixAttempt: 101,
	}

	err = p.AddPortal(portal, portalInfo)
	assert.True(t, err == nil)

	t.Run("Verifying create publish info", func(t *testing.T) {
		publishInfo, err = p.CreatePublishInfo(portal)
		assert.True(t, err == nil)
		assert.Equal(t, publishInfo.IscsiUsername, "username")
	})

	t.Run("Verifying invalid portal case", func(t *testing.T) {
		_, err = p.CreatePublishInfo("")
		assert.True(t, err != nil)
	})

	t.Run("Verifying unknown portal case", func(t *testing.T) {
		_, err = p.CreatePublishInfo(unknownPortal)
		assert.True(t, err != nil)
	})

	t.Run("Verifying empty portal LUN mapping case", func(t *testing.T) {
		pe := &PortalLUNMapping{PortalToLUNMapping: make(map[string]PortalLUNData)}
		publishInfo, err = pe.CreatePublishInfo(portal)
		assert.True(t, err != nil)
	})
}

func TestPortalLUNMapping_GetVolumeIDForPortal(t *testing.T) {
	var err error
	var p PortalLUNMapping
	var pe PortalLUNMapping

	portal := "10.191.21.163:3260"
	unknownPortal := "10.10.10.10:3260"

	credentials := CHAPCredentials{
		ISCSIUsername:        "username",
		ISCSIInitiatorSecret: "password",
		ISCSITargetUsername:  "username_1",
		ISCSITargetSecret:    "password_1",
	}
	portalInfo := PortalInfo{
		ISCSITargetIQN: "IQN:2022 com.netapp-123-456",
		UseCHAP:        true,
		Credentials:    credentials,
		LastFixAttempt: 101,
	}

	err = p.AddPortal(portal, portalInfo)
	assert.True(t, err == nil)

	p.AddLUNToPortal(portal, LUNData{100, "pvc-1111111-2222-3333-4444-55555555555555"})
	assert.False(t, p.IsEmpty())

	t.Run("Verifying get volume id for a given portal", func(t *testing.T) {
		volumeID, _ := p.GetVolumeIDForPortal(portal)
		assert.Equal(t, volumeID, "pvc-1111111-2222-3333-4444-55555555555555")
	})

	t.Run("Verifying invalid portal case", func(t *testing.T) {
		_, err = p.GetVolumeIDForPortal("")
		assert.True(t, err != nil)
	})

	t.Run("Verifying unknown portal case", func(t *testing.T) {
		_, err = p.GetVolumeIDForPortal(unknownPortal)
		assert.True(t, err != nil)
	})

	t.Run("Verifying empty portal LUN mapping case", func(t *testing.T) {
		pe = PortalLUNMapping{PortalToLUNMapping: make(map[string]PortalLUNData)}
		_, err = pe.GetVolumeIDForPortal(portal)
		assert.True(t, err != nil)
	})

	err = pe.AddPortal(portal, portalInfo)
	assert.True(t, err == nil)

	pe.AddLUNToPortal(portal, LUNData{101, ""})
	assert.False(t, pe.IsEmpty())

	t.Run("Verifying for an invalid volume ID", func(t *testing.T) {
		_, err := pe.GetVolumeIDForPortal(portal)
		assert.True(t, err != nil)
	})
}

func TestPortalLUNMapping_UpdateChapInfoForPortal(t *testing.T) {
	var err error
	var p PortalLUNMapping
	var publishInfo *VolumePublishInfo

	portal := "10.191.21.163:3260"
	unknownPortal := "10.10.10.10:3260"

	credentials := CHAPCredentials{
		ISCSIUsername:        "username",
		ISCSIInitiatorSecret: "password",
		ISCSITargetUsername:  "username_1",
		ISCSITargetSecret:    "password_1",
	}
	portalInfo := PortalInfo{
		ISCSITargetIQN: "IQN:2022 com.netapp-123-456",
		UseCHAP:        true,
		Credentials:    credentials,
		LastFixAttempt: 101,
	}

	err = p.AddPortal(portal, portalInfo)
	assert.True(t, err == nil)

	publishInfo, err = p.CreatePublishInfo(portal)
	assert.True(t, err == nil)

	publishInfo.IscsiTargetUsername = "username_new"

	t.Run("Verify update chap info for a portal", func(t *testing.T) {
		err = p.UpdateChapInfoForPortal(portal, publishInfo)
		assert.True(t, err == nil)

		portalInfoNew, err := p.GetPortalInfo(portal)
		assert.True(t, err == nil)

		newChapInfo := portalInfoNew.GetCHAPCredentials()
		assert.Equal(t, newChapInfo.ISCSITargetUsername, "username_new")
	})

	t.Run("Verifying invalid portal case", func(t *testing.T) {
		err = p.UpdateChapInfoForPortal("", publishInfo)
		assert.True(t, err != nil)
	})

	// To cover invalid portal case
	t.Run("Verifying unknown portal case", func(t *testing.T) {
		err = p.UpdateChapInfoForPortal(unknownPortal, publishInfo)
		assert.True(t, err != nil)
	})

	t.Run("Verifying empty portal LUN mapping case", func(t *testing.T) {
		pe := &PortalLUNMapping{PortalToLUNMapping: make(map[string]PortalLUNData)}
		err = pe.UpdateChapInfoForPortal(portal, publishInfo)
		assert.True(t, err != nil)
	})
}
