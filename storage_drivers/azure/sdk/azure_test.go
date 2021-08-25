// Copyright 2021 NetApp, Inc. All Rights Reserved.

package sdk

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
		t.Run(testName, func(t *testing.T) {
			assert.True(t, test.predicate(test.input, test.output), "Predicate failed")
		})
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
		t.Run(testName, func(t *testing.T) {
			assert.True(t, test.predicate(test.input, test.output), "Predicate failed")
		})
	}
}
