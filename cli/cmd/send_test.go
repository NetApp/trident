// Copyright 2025 NetApp, Inc. All Rights Reserved.
package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestSendCmd(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "PersistentPreRunE executes without panic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sendCmd.PersistentPreRunE(&cobra.Command{}, []string{})
		})
	}
}
