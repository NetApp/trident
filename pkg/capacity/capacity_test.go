// Copyright 2025 NetApp, Inc. All Rights Reserved.

package capacity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
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

// TestQuantityToHumanReadableString verifies decimal human-readable strings (e.g. "20.74Gi").
// Values are rounded up to HumanReadableDecimals (2) so we never request less than the original.
func TestQuantityToHumanReadableString(t *testing.T) {
	oneGiB := int64(1 << 30)
	oneMiB := int64(1 << 20)
	oneKiB := int64(1 << 10)

	tests := []struct {
		name           string
		input          resource.Quantity
		expectedString string
		roundsUp       bool
	}{
		{
			name:           "12 GiB - 12.00Gi",
			input:          *resource.NewQuantity(12*oneGiB, resource.BinarySI),
			expectedString: "12.00Gi",
		},
		{
			name:           "10 GiB - 10.00Gi",
			input:          *resource.NewQuantity(10*oneGiB, resource.BinarySI),
			expectedString: "10.00Gi",
		},
		{
			name:           "1 GiB - 1.00Gi",
			input:          *resource.NewQuantity(oneGiB, resource.BinarySI),
			expectedString: "1.00Gi",
		},
		{
			name:           "512 MiB - 512.00Mi",
			input:          *resource.NewQuantity(512*oneMiB, resource.BinarySI),
			expectedString: "512.00Mi",
		},
		{
			name:           "100 MiB - 100.00Mi",
			input:          *resource.NewQuantity(100*oneMiB, resource.BinarySI),
			expectedString: "100.00Mi",
		},
		{
			name:           "1024 KiB - 1.00Mi",
			input:          *resource.NewQuantity(1024*oneKiB, resource.BinarySI),
			expectedString: "1.00Mi",
		},
		{
			name:           "512 KiB - 512.00Ki",
			input:          *resource.NewQuantity(512*oneKiB, resource.BinarySI),
			expectedString: "512.00Ki",
		},
		{
			name:           "12Gi+1 byte - 12.01Gi",
			input:          *resource.NewQuantity(12*oneGiB+1, resource.BinarySI),
			expectedString: "12.01Gi",
			roundsUp:       true,
		},
		{
			name:           "22265110461 bytes - 20.74Gi (user example)",
			input:          *resource.NewQuantity(22265110461, resource.BinarySI),
			expectedString: "20.74Gi",
			roundsUp:       true,
		},
		{
			name:           "zero - 0",
			input:          *resource.NewQuantity(0, resource.BinarySI),
			expectedString: "0",
		},
		{
			name:           "parsed 15Gi - 15.00Gi",
			input:          resource.MustParse("15Gi"),
			expectedString: "15.00Gi",
		},
		{
			name:           "parsed 5Gi - 5.00Gi",
			input:          resource.MustParse("5Gi"),
			expectedString: "5.00Gi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := QuantityToHumanReadableString(tt.input)
			assert.Equal(t, tt.expectedString, s, "QuantityToHumanReadableString should match expected")
			// Parsed string should yield value >= input when rounding up
			parsed, err := resource.ParseQuantity(tt.expectedString)
			assert.NoError(t, err)
			if tt.roundsUp {
				assert.GreaterOrEqual(t, parsed.Value(), tt.input.Value(), "Rounded-up value must be >= input")
			} else {
				assert.Equal(t, tt.input.Value(), parsed.Value(), "Value must be unchanged")
			}
		})
	}
}

// TestQuantityToHumanReadableString_Negative verifies negative quantities return q.String()
func TestQuantityToHumanReadableString_Negative(t *testing.T) {
	q := *resource.NewQuantity(-1, resource.BinarySI)
	s := QuantityToHumanReadableString(q)
	assert.Equal(t, q.String(), s)
}
