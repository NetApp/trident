// Copyright 2022 NetApp, Inc. All Rights Reserved.

package main

import (
	"flag"
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

func TestPrintFlag(t *testing.T) {
	// Set debug level to ensure log is captured
	InitLogLevel("debug")

	// Create a test flag
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("test-flag", "test-value", "test flag")
	fs.Set("test-flag", "test-value")

	// Call printFlag to cover the function
	f := fs.Lookup("test-flag")
	printFlag(f)
}
