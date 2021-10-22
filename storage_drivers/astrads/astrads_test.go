package astrads

import (
	"context"
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

func TestPadVolumeSizeWithSnapshotReserve(t *testing.T) {
	tests := []struct {
		Name            string
		SnapshotReserve int32
		SizeBytes       uint64
		PaddedSizeBytes uint64
	}{
		{
			Name:            "no reserve",
			SnapshotReserve: 0,
			SizeBytes:       1000000000,
			PaddedSizeBytes: 1000000000,
		},
		{
			Name:            "10% reserve",
			SnapshotReserve: 10,
			SizeBytes:       1000000000,
			PaddedSizeBytes: 1111111111,
		},
		{
			Name:            "reserve too low",
			SnapshotReserve: -1,
			SizeBytes:       1000000000,
			PaddedSizeBytes: 1000000000,
		},
		{
			Name:            "reserve too high",
			SnapshotReserve: 91,
			SizeBytes:       1000000000,
			PaddedSizeBytes: 1000000000,
		},
	}

	d := StorageDriver{}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			paddedSizeBytes := d.padVolumeSizeWithSnapshotReserve(context.TODO(), "volume",
				test.SizeBytes, test.SnapshotReserve)
			assert.Equal(t, test.PaddedSizeBytes, paddedSizeBytes, "incorrect size")
		})
	}
}

func TestUnpadVolumeSizeWithSnapshotReserve(t *testing.T) {
	tests := []struct {
		Name              string
		SnapshotReserve   int32
		SizeBytes         uint64
		UnpaddedSizeBytes uint64
	}{
		{
			Name:              "no reserve",
			SnapshotReserve:   0,
			SizeBytes:         1000000000,
			UnpaddedSizeBytes: 1000000000,
		},
		{
			Name:              "10% reserve",
			SnapshotReserve:   10,
			SizeBytes:         1000000000,
			UnpaddedSizeBytes: 900000000,
		},
		{
			Name:              "reserve too low",
			SnapshotReserve:   -1,
			SizeBytes:         1000000000,
			UnpaddedSizeBytes: 1000000000,
		},
		{
			Name:              "reserve too high",
			SnapshotReserve:   91,
			SizeBytes:         1000000000,
			UnpaddedSizeBytes: 1000000000,
		},
	}

	d := StorageDriver{}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			unpaddedSizeBytes := d.unpadVolumeSizeWithSnapshotReserve(context.TODO(), "volume",
				test.SizeBytes, test.SnapshotReserve)
			assert.Equal(t, test.UnpaddedSizeBytes, unpaddedSizeBytes, "incorrect size")
		})
	}
}
