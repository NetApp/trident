// Copyright 2019 NetApp, Inc. All Rights Reserved.

package sdk

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The Azure NetApp Files backend will report itself as being "online" even when
// provided bad credentials. This will cause PersistentVolumeClaims to fail at
// provisioning time with a "no capacity pools found" error.
//
// This test ensures that Authenticate(c *AzureClient) fails when it cannot
// retrieve an ADAL token with the ServicePrincipal credentials provided.
//
// Because of the way that go-azure is designed, we have to make a dummy API call and
// have it fail in order to verify that we can receive a token. The easiest
// client to do that with is the virtualNetworks client, so that's what we'll use.

func TestConfirmAzureAPIAccessibleFirstFail(t *testing.T) {
	oldAttemptToResolveVirtualNetworks := attemptToResolveVirtualNetworks
	defer func() { attemptToResolveVirtualNetworks = oldAttemptToResolveVirtualNetworks }()
	attemptToResolveVirtualNetworks = func(c *AzureClient, ctx context.Context) error {
		return errors.New("Test failure")
	}
	azureClient := new(AzureClient)
	err := confirmAzureAPIAccessibleFirst(azureClient, context.TODO())
	t.Logf("Running test case: Confirm that Azure API is accessible first, Fail")
	assert.EqualError(t, err, "Failed to access the Azure API: Test failure")
}

func TestConfirmAzureAPIAccessibleFirstSuccess(t *testing.T) {
	oldAttemptToResolveVirtualNetworks := attemptToResolveVirtualNetworks
	defer func() { attemptToResolveVirtualNetworks = oldAttemptToResolveVirtualNetworks }()
	attemptToResolveVirtualNetworks = func(c *AzureClient, ctx context.Context) error {
		return nil
	}
	azureClient := new(AzureClient)
	t.Logf("Running test case: Confirm that Azure API is accessible first, Success")
	obj := confirmAzureAPIAccessibleFirst(azureClient, context.TODO())
	assert.Nil(t, obj)
}

func TestPoolShortname(t *testing.T) {

	tests := map[string]struct {
		input     string
		output    string
		predicate func(string, string) bool
	}{
		"Strip pool prefix: normal (successful)": {
			input:  "east-us-anf-dev-storage/bnaylor-anf",
			output: "bnaylor-anf",
			predicate: func(input, output string) bool {
				return output == poolShortname(input)
			},
		},
		"Strip pool prefix: already stripped (successful)": {
			input:  "bnaylor-anf",
			output: "bnaylor-anf",
			predicate: func(input, output string) bool {
				return output == poolShortname(input)
			},
		},
	}
	for testName, test := range tests {
		t.Logf("Running test case '%s'", testName)
		assert.True(t, test.predicate(test.input, test.output), "Predicate failed")
	}
}

func TestVolumeShortname(t *testing.T) {

	tests := map[string]struct {
		input     string
		output    string
		predicate func(string, string) bool
	}{
		"Strip volume prefix: normal (successful)": {
			input:  "east-us-anf-dev-storage/bnaylor-anf/anf-hairnet-modulus",
			output: "anf-hairnet-modulus",
			predicate: func(input, output string) bool {
				return output == volumeShortname(input)
			},
		},
		"Strip pool prefix: already stripped (successful)": {
			input:  "anf-biplane-omnipresence",
			output: "anf-biplane-omnipresence",
			predicate: func(input, output string) bool {
				return output == volumeShortname(input)
			},
		},
	}
	for testName, test := range tests {
		t.Logf("Running test case '%s'", testName)

		assert.True(t, test.predicate(test.input, test.output), "Predicate failed")
	}
}
