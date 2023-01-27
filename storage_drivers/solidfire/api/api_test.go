// Copyright 2022 NetApp, Inc. All Rights Reserved.

package api

import (
	"io"
	"os"
	"testing"

	. "github.com/netapp/trident/logging"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}
