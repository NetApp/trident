// Copyright 2022 NetApp, Inc. All Rights Reserved.

package storage

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/netapp/trident/logging"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
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
