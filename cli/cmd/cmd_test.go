// Copyright 2021 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"io"
	"os"
	"testing"
	"time"

	. "github.com/netapp/trident/logging"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)

	// Speed up deleteWithRetry paths exercised by obliviate_crd tests.
	deleteWithRetryMaxElapsedTime = 200 * time.Millisecond
	deleteWithRetryInitialInterval = 1 * time.Millisecond
	deleteWithRetryMaxInterval = 10 * time.Millisecond

	os.Exit(m.Run())
}
