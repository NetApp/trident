// Copyright 2025 NetApp, Inc. All Rights Reserved.

package capacity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToBytes(t *testing.T) {
	d := make(map[string]string)
	d["512"] = "512"
	d["1KB"] = "1000"
	d["1Ki"] = "1024"
	d["1KiB"] = "1024"
	d["4k"] = "4096"
	d["1gi"] = "1073741824"
	d["1Gi"] = "1073741824"
	d["1GiB"] = "1073741824"
	d["1gb"] = "1000000000"
	d["1g"] = "1073741824"

	for k, v := range d {
		s, err := ToBytes(k)
		if err != nil {
			t.Errorf("Encountered '%v' running ToBytes('%v')", err, k)
		} else if s != v {
			t.Errorf("Expected ToBytes('%v') == '%v' but was %v", k, v, s)
		}
	}
}

func TestGetVolumeSizeBytes(t *testing.T) {
	type input struct {
		opts        map[string]string
		defaultSize string
	}

	tt := map[string]struct {
		input     input
		assertRes assert.ValueAssertionFunc
		assertErr assert.ErrorAssertionFunc
	}{
		"test when opts is nil": {
			input: input{
				nil, "1G",
			},
			assertRes: assert.NotZero,
			assertErr: assert.NoError,
		},
		"test when default size is invalid": {
			input: input{
				nil, "G",
			},
			assertRes: assert.Zero,
			assertErr: assert.Error,
		},
		"test when default size is empty": {
			input: input{
				nil, "",
			},
			assertRes: assert.Zero,
			assertErr: assert.Error,
		},
		"test when opts supplied and default size is not empty": {
			input: input{
				map[string]string{
					"size": "500",
				},
				"1G",
			},
			assertRes: assert.NotZero,
			assertErr: assert.NoError,
		},
		"test when invalid opts supplied and default size is not empty": {
			input: input{
				map[string]string{
					"size": "size",
				},
				"1G",
			},
			assertRes: assert.Zero,
			assertErr: assert.Error,
		},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			input := test.input
			result, err := GetVolumeSizeBytes(context.Background(), input.opts, input.defaultSize)
			test.assertErr(t, err)
			test.assertRes(t, result)
		})
	}
}

func TestVolumeSizeWithinTolerance(t *testing.T) {
	delta := int64(50000000) // 50mb

	volSizeTests := []struct {
		requestedSize int64
		currentSize   int64
		delta         int64
		expected      bool
	}{
		{50000000000, 50000003072, delta, true},
		{50000000001, 50000000000, delta, true},
		{50049999999, 50000000000, delta, true},
		{50000000000, 50049999900, delta, true},
		{50050000001, 50000000000, delta, false},
		{50000000000, 50050000001, delta, false},
	}

	for _, vst := range volSizeTests {
		isSameSize := VolumeSizeWithinTolerance(vst.requestedSize, vst.currentSize, vst.delta)
		assert.Equal(t, vst.expected, isSameSize)
	}
}
