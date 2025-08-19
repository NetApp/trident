// Copyright 2025 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCreateCmd_PersistentPreRunE(t *testing.T) {
	testCases := []struct {
		name        string
		wantErr     bool
		description string
		setup       func()
	}{
		{
			name:        "persistent pre run fails in test environment",
			wantErr:     true,
			description: "discoverOperatingMode fails because no Trident server/K8s cluster is available in test environment",
			setup:       func() {},
		},
		{
			name:        "persistent pre run succeeds with server set",
			wantErr:     false,
			description: "discoverOperatingMode succeeds when Server is pre-configured (direct mode)",
			setup: func() {
				Server = "http://localhost:8000" // Mock server for direct mode
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			originalServer := Server

			tc.setup()
			defer func() {
				Server = originalServer
			}()

			cmd := &cobra.Command{}
			err := createCmd.PersistentPreRunE(cmd, []string{})

			if tc.wantErr {
				assert.Error(t, err, "Expected error due to missing Trident server/K8s cluster in test environment")
			} else {
				assert.NoError(t, err, "Expected success when Server is pre-configured")
			}
		})
	}
}
