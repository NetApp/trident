// Copyright 2021 NetApp, Inc. All Rights Reserved.

package api

import (
	"io"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(io.Discard)

	// Speed up ONTAP read-after-write retry paths exercised by WaitFor* helpers.
	ConfigureWaitForOntapBackoffForTests()

	os.Exit(m.Run())
}
