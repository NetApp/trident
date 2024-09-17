// Copyright 2024 NetApp, Inc. All Rights Reserved.

package nodeprep_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/internal/nodeprep"
)

func TestPrepareNode(t *testing.T) {
	type parameters struct {
		protocols        []string
		expectedExitCode int
	}
	tests := map[string]parameters{
		"no protocols": {
			protocols:        nil,
			expectedExitCode: 0,
		},
		"happy path": {
			protocols:        []string{"iscsi"},
			expectedExitCode: 0,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			exitCode := nodeprep.PrepareNode(params.protocols)
			assert.Equal(t, params.expectedExitCode, exitCode)
		})
	}
}
