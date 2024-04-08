// Copyright 2024 NetApp, Inc. All Rights Reserved.

package configurator

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
