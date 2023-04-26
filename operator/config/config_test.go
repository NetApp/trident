// Copyright 2022 NetApp, Inc. All Rights Reserved.

package config

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/netapp/trident/logging"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func TestVersion(t *testing.T) {
	// resetting the state of the world
	// this will evaluate BuildType now, then execute directly before the test function returns.
	// we can't use a closure here because the value of BuildType will be different by the time this executes.
	defer func(buildType string) {
		BuildType = buildType
	}(BuildType)

	tc := []string{"unknown", "stable"}

	for _, val := range tc {
		BuildType = val
		result := Version()
		assert.NotEmpty(t, result, "version is empty")
	}
}
