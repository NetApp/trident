// Copyright 2022 NetApp, Inc. All Rights Reserved.

package storage

import (
	"io/ioutil"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}

func TestParseSnapshotID(t *testing.T) {
	pvc, snapshot, err := ParseSnapshotID("fakeflexvol/fakesnapshot")
	assert.Equal(t, "fakeflexvol", pvc, "Unexpected flexvol name")
	assert.Equal(t, "fakesnapshot", snapshot, "Unexpected snapshot name")
	assert.Equal(t, nil, err, "Unexpected error")

	_, _, err = ParseSnapshotID("fakeflexvol/fakesnapshot/withextra")
	assert.NotEqual(t, nil, err, "Expected error")

	_, _, err = ParseSnapshotID("")
	assert.NotEqual(t, nil, err, "Expected error")

	_, _, err = ParseSnapshotID("fakeflexvol")
	assert.NotEqual(t, nil, err, "Expected error")
}
