// Copyright 2021 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"io"
	"os"
	"testing"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	api.ConfigureWaitForOntapBackoffForTests()
	os.Exit(m.Run())
}
