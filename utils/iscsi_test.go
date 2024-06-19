// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/mitchellh/copystructure"
	"github.com/stretchr/testify/assert"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
)

func mapCopyHelper(input map[int32]string) map[int32]string {
	output := make(map[int32]string, len(input))

	for key, value := range input {
		output[key] = value
	}

	return output
}

func structCopyHelper(input ISCSISessionData) *ISCSISessionData {
	clone, err := copystructure.Copy(input)
	if err != nil {
		return &ISCSISessionData{}
	}

	output, ok := clone.(ISCSISessionData)
	if !ok {
		return &ISCSISessionData{}
	}

	return &output
}

func reverseSlice(input []string) []string {
	output := make([]string, 0)

	for idx := len(input) - 1; idx >= 0; idx-- {
		output = append(output, input[idx])
	}

	return output
}

func TestIsPerNodeIgroup(t *testing.T) {
	tt := map[string]bool{
		"":        false,
		"trident": false,
		"node-01-8095-ad1b8212-49a0-82d4-ef4f8b5b620z":                                   false,
		"trident-ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                                   false,
		"-ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                                          false,
		".ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                                          false,
		"igroup-a-trident-ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                          false,
		"node-01-ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                                   true,
		"trdnt-ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                                     true,
		"worker0.hjonhjc.rtp.openenglab.netapp.com-25426e2a-9f96-4f4d-90a8-72a6cdd6f645": true,
		"worker0-hjonhjc.trdnt-ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                     true,
	}

	for input, expected := range tt {
		assert.Equal(t, expected, IsPerNodeIgroup(input))
	}
}

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

func TestIsStalePortal(t *testing.T) {
	ipList := []string{"1.2.3.4", "2.3.4.5", "3.4.5.6", "4.5.6.7"}

	iqnList := []string{"IQN1", "IQN2", "IQN3", "IQN4"}

	chapCredentials := []IscsiChapInfo{
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

	sessionData1 := ISCSISessionData{
		PortalInfo: PortalInfo{
			ISCSITargetIQN: iqnList[0],
			Credentials:    chapCredentials[2],
		},
	}

	sessionData2 := ISCSISessionData{
		PortalInfo: PortalInfo{
			ISCSITargetIQN: iqnList[1],
			Credentials:    chapCredentials[2],
		},
	}

	type PreRun func(publishedSessions, currentSessions *ISCSISessions, portal string)

	inputs := []struct {
		TestName           string
		PublishedPortals   *ISCSISessions
		CurrentPortals     *ISCSISessions
		SessionWaitTime    time.Duration
		TimeNow            time.Time
		Portal             string
		ResultAction       ISCSIAction
		SimulateConditions PreRun
	}{
		{
			TestName: "CHAP in use and Source is NodeStage and Credentials Mismatch",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    LogoutLoginScan,
			SimulateConditions: func(publishedSessions, currentSessions *ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Source = SessionSourceNodeStage
				currentSessions.Info[portal].PortalInfo.Credentials = chapCredentials[1]
			},
		},
		{
			TestName: "CHAP in use and Source is NodeStage and Credentials NOT Mismatch and First time stale",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Source = SessionSourceNodeStage
				publishedSessions.Info[portal].PortalInfo.FirstIdentifiedStaleAt = time.Time{}
			},
		},
		{
			TestName: "CHAP in use and Source is NOT NodeStage and NOT First time stale but NOT exceeds Session Wait" +
				" Time",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now().Add(2 * time.Second),
			Portal:          ipList[0],
			ResultAction:    NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.FirstIdentifiedStaleAt = time.Now()
			},
		},
		{
			TestName: "CHAP in use and Source is NOT NodeStage and NOT First time stale and exceeds Session Wait" +
				" Time",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now().Add(20 * time.Second),
			Portal:          ipList[0],
			ResultAction:    LogoutLoginScan,
			SimulateConditions: func(publishedSessions, currentSessions *ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.FirstIdentifiedStaleAt = time.Now()
			},
		},
		{
			TestName: "CHAP NOT in use and First time stale",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Credentials = chapCredentials[0]
				publishedSessions.Info[portal].PortalInfo.FirstIdentifiedStaleAt = time.Time{}
			},
		},
		{
			TestName: "CHAP NOT in use and NOT First time stale but NOT exceeds Session Wait" +
				" Time",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now().Add(2 * time.Second),
			Portal:          ipList[0],
			ResultAction:    NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Credentials = chapCredentials[0]
				publishedSessions.Info[portal].PortalInfo.FirstIdentifiedStaleAt = time.Now()
			},
		},
		{
			TestName: "CHAP NOT in use and NOT First time stale and exceeds Session Wait" +
				" Time",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now().Add(20 * time.Second),
			Portal:          ipList[0],
			ResultAction:    LogoutLoginScan,
			SimulateConditions: func(publishedSessions, currentSessions *ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Credentials = chapCredentials[0]
				publishedSessions.Info[portal].PortalInfo.FirstIdentifiedStaleAt = time.Now()
			},
		},
	}

	for _, input := range inputs {
		t.Run(input.TestName, func(t *testing.T) {
			portal := input.Portal

			input.SimulateConditions(input.PublishedPortals, input.CurrentPortals, portal)

			publishedPortalData, _ := input.PublishedPortals.Info[portal]
			currentPortalData, _ := input.CurrentPortals.Info[portal]

			publishedPortalInfo := publishedPortalData.PortalInfo
			currentPortalInfo := currentPortalData.PortalInfo

			action := isStalePortal(context.TODO(), &publishedPortalInfo, &currentPortalInfo, input.SessionWaitTime,
				input.TimeNow, portal)

			assert.Equal(t, input.ResultAction, action, "Remediation action mismatch")
		})
	}
}

func TestIsNonStalePortal(t *testing.T) {
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

	chapCredentials := []IscsiChapInfo{
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

	sessionData1 := ISCSISessionData{
		PortalInfo: PortalInfo{
			ISCSITargetIQN: iqnList[0],
			Credentials:    chapCredentials[2],
		},
		LUNs: LUNs{
			Info: mapCopyHelper(lunList1),
		},
	}

	sessionData2 := ISCSISessionData{
		PortalInfo: PortalInfo{
			ISCSITargetIQN: iqnList[1],
			Credentials:    chapCredentials[2],
		},
		LUNs: LUNs{
			Info: mapCopyHelper(lunList2),
		},
	}

	type PreRun func(publishedSessions, currentSessions *ISCSISessions, portal string)

	inputs := []struct {
		TestName           string
		PublishedPortals   *ISCSISessions
		CurrentPortals     *ISCSISessions
		SessionWaitTime    time.Duration
		TimeNow            time.Time
		Portal             string
		ResultAction       ISCSIAction
		SimulateConditions PreRun
	}{
		{
			TestName: "Current Portal Missing All LUNs",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    Scan,
			SimulateConditions: func(publishedSessions, currentSessions *ISCSISessions, portal string) {
				currentSessions.Info[portal].LUNs = LUNs{}
			},
		},
		{
			TestName: "Current Portal Missing One LUNs",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    Scan,
			SimulateConditions: func(publishedSessions, currentSessions *ISCSISessions, portal string) {
				delete(currentSessions.Info[portal].LUNs.Info, 2)
			},
		},
		{
			TestName: "Published Portal Missing All LUNs",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *ISCSISessions, portal string) {
				publishedSessions.Info[portal].LUNs = LUNs{}
			},
		},
		{
			TestName: "CHAP notification, Published and Current portals have CHAP but mismatch",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Source = SessionSourceNodeStage
				currentSessions.Info[portal].PortalInfo.Credentials = chapCredentials[1]
			},
		},
		{
			TestName: "CHAP notification, Published portals ha CHAP but not Current Portal",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Source = SessionSourceNodeStage
				currentSessions.Info[portal].PortalInfo.Credentials = chapCredentials[0]
			},
		},
		{
			TestName: "CHAP notification, Current portals ha CHAP but not Published Portal",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Source = SessionSourceNodeStage
				publishedSessions.Info[portal].PortalInfo.Credentials = chapCredentials[0]
			},
		},
	}

	for _, input := range inputs {
		t.Run(input.TestName, func(t *testing.T) {
			portal := input.Portal

			input.SimulateConditions(input.PublishedPortals, input.CurrentPortals, portal)

			publishedPortalData, _ := input.PublishedPortals.Info[portal]
			currentPortalData, _ := input.CurrentPortals.Info[portal]

			action := isNonStalePortal(context.TODO(), publishedPortalData, currentPortalData, portal)

			assert.Equal(t, input.ResultAction, action, "Remediation action mismatch")
		})
	}
}

func TestSortPortals(t *testing.T) {
	ipList := []string{"1.2.3.4", "2.3.4.5", "3.4.5.6", "4.5.6.7", "5.6.7.8", "6.7.8.9", "7.8.9.10", "8.9.10.11"}

	sessionData := ISCSISessionData{
		PortalInfo: PortalInfo{
			ISCSITargetIQN: "IQN1",
		},
	}

	type PreRun func(publishedSessions *ISCSISessions, portal []string)

	inputs := []struct {
		TestName           string
		PublishedPortals   *ISCSISessions
		InputPortals       []string
		ResultPortals      []string
		SimulateConditions PreRun
	}{
		{
			TestName:         "Zero size list preserves Zero size list",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{}},
			InputPortals:     []string{},
			ResultPortals:    []string{},
			SimulateConditions: func(publishedSessions *ISCSISessions, portals []string) {
				// Populate Published Portals
				for _, portal := range portals {
					publishedSessions.Info[portal] = structCopyHelper(sessionData)
				}
			},
		},
		{
			TestName:         "One size list preserves One size list",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{}},
			InputPortals:     []string{ipList[0]},
			ResultPortals:    []string{ipList[0]},
			SimulateConditions: func(publishedSessions *ISCSISessions, portals []string) {
				// Populate Published Portals
				for _, portal := range portals {
					publishedSessions.Info[portal] = structCopyHelper(sessionData)
				}
			},
		},
		{
			TestName:         "Two size sorts on the basis of Access time",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{}},
			InputPortals:     []string{ipList[0], ipList[4]},
			ResultPortals:    []string{ipList[4], ipList[0]},
			SimulateConditions: func(publishedSessions *ISCSISessions, portals []string) {
				// Populate Published Portals
				for idx := len(portals) - 1; idx >= 0; idx-- {
					publishedSessions.Info[portals[idx]] = structCopyHelper(sessionData)
					publishedSessions.Info[portals[idx]].PortalInfo.LastAccessTime = time.Now().Add(-time.Hour * time.
						Duration(idx))
				}
			},
		},
		{
			TestName:         "Same access time results in the same order",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{}},
			InputPortals:     append([]string{}, ipList...),
			ResultPortals:    append([]string{}, ipList...),
			SimulateConditions: func(publishedSessions *ISCSISessions, portals []string) {
				// Populate Published Portals
				for _, portal := range portals {
					publishedSessions.Info[portal] = structCopyHelper(sessionData)
				}
			},
		},
		{
			TestName:         "Increasing access time results in the reverse order",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{}},
			InputPortals:     append([]string{}, ipList...),
			ResultPortals:    reverseSlice(ipList),
			SimulateConditions: func(publishedSessions *ISCSISessions, portals []string) {
				// Populate Published Portals
				for idx := len(portals) - 1; idx >= 0; idx-- {
					publishedSessions.Info[portals[idx]] = structCopyHelper(sessionData)
					publishedSessions.Info[portals[idx]].PortalInfo.LastAccessTime = time.Now().Add(-time.Hour * time.
						Duration(idx))
				}
			},
		},
		{
			TestName:         "Increasing access time results in the reverse order with the exception of 3 items",
			PublishedPortals: &ISCSISessions{Info: map[string]*ISCSISessionData{}},
			InputPortals:     ipList,
			ResultPortals:    append(ipList[0:3], reverseSlice(ipList[3:])...),
			SimulateConditions: func(publishedSessions *ISCSISessions, portals []string) {
				// Populate Published Portals
				for idx := len(portals) - 1; idx >= 0; idx-- {
					publishedSessions.Info[portals[idx]] = structCopyHelper(sessionData)

					if idx >= 3 {
						publishedSessions.Info[portals[idx]].PortalInfo.LastAccessTime = time.Now().Add(-time.
							Hour * time.Duration(idx))
					}
				}
			},
		},
	}

	for _, input := range inputs {
		t.Run(input.TestName, func(t *testing.T) {
			input.SimulateConditions(input.PublishedPortals, input.InputPortals)

			SortPortals(input.InputPortals, input.PublishedPortals)

			assert.Equal(t, input.ResultPortals, input.InputPortals, "sort order mismatch")
		})
	}
}

func TestMultipathdIsRunning(t *testing.T) {
	// Reset exec command after tests
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	mockCtrl := gomock.NewController(t)
	mockExec := mockexec.NewMockCommand(mockCtrl)
	tests := []struct {
		name          string
		execOut       string
		execErr       error
		returnCode    int
		expectedValue bool
	}{
		{name: "True", execOut: "1234", execErr: nil, expectedValue: true},
		{name: "False", execOut: "", execErr: nil, expectedValue: false},
		{name: "Error", execOut: "1234", execErr: errors.New("cmd error"), expectedValue: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup mock calls.
			mockExec.EXPECT().Execute(
				ctx, gomock.Any(), gomock.Any(),
			).Return([]byte(tt.execOut), tt.execErr)

			// Only mock out the second call if the expected value isn't true.
			if !tt.expectedValue {
				mockExec.EXPECT().Execute(
					ctx, gomock.Any(), gomock.Any(), gomock.Any(),
				).Return([]byte(tt.execOut), tt.execErr)
			}

			// Assign the shared command to the mock.
			command = mockExec

			actualValue := multipathdIsRunning(context.Background())
			assert.Equal(t, tt.expectedValue, actualValue)
		})
	}
}
