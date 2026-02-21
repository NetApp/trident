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

// TestRoundUpToDecimals verifies round-up-to-N-decimals behavior, including decimals <= 0 (Ceil).
func TestRoundUpToDecimals(t *testing.T) {
	tests := []struct {
		name     string
		val      float64
		decimals int
		want     float64
	}{
		{"decimals 0 uses Ceil", 20.3, 0, 21},
		{"decimals 0 exact integer", 20.0, 0, 20},
		{"decimals negative uses Ceil", 20.3, -1, 21},
		{"decimals 1 round up", 20.31, 1, 20.4},
		{"decimals 2 round up 20.736", 20.736, 2, 20.74},
		{"decimals 2 round up 20.731", 20.731, 2, 20.74},
		{"decimals 2 exact", 20.0, 2, 20.0},
		{"decimals 2 tiny over", 20.001, 2, 20.01},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := roundUpToDecimals(tt.val, tt.decimals)
			assert.Equal(t, tt.want, got, "roundUpToDecimals(%v, %d)", tt.val, tt.decimals)
		})
	}
}

// TestQuantityToHumanReadableString_SmallBytes verifies values below 1KiB are returned as plain byte counts.
func TestQuantityToHumanReadableString_SmallBytes(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{512, "512"},
		{1023, "1023"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			q := *resource.NewQuantity(tt.bytes, resource.BinarySI)
			got := QuantityToHumanReadableString(q)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestBytesToDecimalSizeString verifies decimal (SI) formatting for display, e.g. "51MB", "1.50GB".
func TestBytesToDecimalSizeString(t *testing.T) {
	tests := []struct {
		name  string
		bytes int64
		want  string
	}{
		{"zero", 0, "0"},
		{"negative", -100, "-100"},
		{"1 byte", 1, "1B"},
		{"999 bytes", 999, "999B"},
		{"1 KB", 1000, "1KB"},
		{"51 MB (resize delta)", 51 * 1e6, "51MB"},
		{"1.5 GB", 1500000000, "1.50GB"},
		{"1 GB", 1e9, "1GB"},
		{"2.25 GB", 2250000000, "2.25GB"},
		{"100 GB", 100 * 1e9, "100GB"},
		{"1 TB", 1e12, "1000GB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BytesToDecimalSizeString(tt.bytes)
			assert.Equal(t, tt.want, got, "BytesToDecimalSizeString(%d)", tt.bytes)
		})
	}
}

// TestParseQuantityFloorBytes verifies quantity string is parsed and floored to bytes.
func TestParseQuantityFloorBytes(t *testing.T) {
	oneGiB := uint64(1 << 30)
	oneMiB := uint64(1 << 20)

	tests := []struct {
		name     string
		input    string
		expected uint64
	}{
		{"empty string", "", 0},
		{"whitespace only", "   ", 0},
		{"trimmed input", "  100Gi  ", 100 * oneGiB},
		{"1Gi", "1Gi", oneGiB},
		{"1.7Gi floors to 1825361100 not 1825361101", "1.7Gi", 1825361100},
		{"100Mi", "100Mi", 100 * oneMiB},
		{"0", "0", 0},
		{"0Gi", "0Gi", 0},
		{"invalid returns 0", "not-a-size", 0},
		{"negative returns 0", "-1Gi", 0},
		{"decimal percentage-like returns 0", "80%", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseQuantityFloorBytes(tt.input)
			assert.Equal(t, tt.expected, got, "ParseQuantityFloorBytes(%q)", tt.input)
		})
	}
}

// TestQuantityFloorBytes verifies quantity pointer is floored to bytes; nil or invalid returns 0.
func TestQuantityFloorBytes(t *testing.T) {
	oneGiB := int64(1 << 30)

	tests := []struct {
		name     string
		qty      *resource.Quantity
		expected uint64
	}{
		{"nil returns 0", nil, 0},
		{"1Gi", resource.NewQuantity(oneGiB, resource.BinarySI), 1073741824},
		{"parsed 1.7Gi floors to 1825361100", ptrQuantity(resource.MustParse("1.7Gi")), 1825361100},
		{"zero", resource.NewQuantity(0, resource.BinarySI), 0},
		{"negative returns 0", resource.NewQuantity(-1, resource.BinarySI), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuantityFloorBytes(tt.qty)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func ptrQuantity(q resource.Quantity) *resource.Quantity {
	return &q
}
