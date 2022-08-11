// Copyright 2022 NetApp, Inc. All Rights Reserved.

package plain

import (
	"io/ioutil"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}
