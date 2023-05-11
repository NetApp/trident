// Copyright 2022 NetApp, Inc. All Rights Reserved.

package main

import (
	"io"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}
