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

	// Avoid slow EC2 instance-metadata lookups when AWS SDK clients are constructed in tests.
	_ = os.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	_ = os.Setenv("AWS_ACCESS_KEY_ID", "test")
	_ = os.Setenv("AWS_SECRET_ACCESS_KEY", "test")

	// Speed up ONTAP read-after-write retry paths (e.g. NVMe namespace create lookup).
	api.ConfigureWaitForOntapBackoffForTests()

	os.Exit(m.Run())
}
