// Copyright 2016 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"flag"
	"os"
	"testing"

	log "github.com/Sirupsen/logrus"
)

var (
	debug = flag.Bool("debug", false, "Enable debugging output")
)

func TestMain(m *testing.M) {
	flag.Parse()
	if *debug {
		log.SetLevel(log.DebugLevel)
	}
	os.Exit(m.Run())
}
