// Copyright 2024 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"testing"

	"github.com/mitchellh/copystructure"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/models"
)

func structCopyHelper(input models.ISCSISessionData) *models.ISCSISessionData {
	clone, err := copystructure.Copy(input)
	if err != nil {
		return &models.ISCSISessionData{}
	}

	output, ok := clone.(models.ISCSISessionData)
	if !ok {
		return &models.ISCSISessionData{}
	}

	return &output
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
