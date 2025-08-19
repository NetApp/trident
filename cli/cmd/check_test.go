// Copyright 2024 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestCheckCmd_PersistentPreRunE(t *testing.T) {
	cmd := &cobra.Command{}
	args := []string{}

	assert.NotPanics(t, func() {
		_ = checkCmd.PersistentPreRunE(cmd, args)
	})
}
