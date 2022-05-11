// Copyright 2021 NetApp, Inc. All Rights Reserved.

package core

import (
	"io/ioutil"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
)

// TestMain is the entry point for all unit tests.
// It checks for any command line arguments and if none are found,
// it sets the log output to ioutil.Discard to disable any standard log output.
// Parameters:
//     m: the testing.M object
// Returns:
//     none
// Example:
//   

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}
