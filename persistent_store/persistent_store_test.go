// Copyright 2021 NetApp, Inc. All Rights Reserved.

package persistentstore

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
