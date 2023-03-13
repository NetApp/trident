// Copyright 2022 NetApp, Inc. All Rights Reserved.

package rest

import (
	"io"
	"os"
	"testing"

	"github.com/netapp/trident/logging"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	logging.InitLogOutput(io.Discard)
	logging.InitAuditLogger(true)
	os.Exit(m.Run())
}
