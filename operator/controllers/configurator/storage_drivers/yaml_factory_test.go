package storage_drivers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConstructEncryptionKeys(t *testing.T) {
	testCases := []struct {
		name     string
		input    map[string]string
		expected string
	}{
		{
			name:     "Empty Map",
			input:    map[string]string{},
			expected: "",
		},
		{
			name:     "Single Element Map",
			input:    map[string]string{"key1": "value1"},
			expected: "customerEncryptionKeys:\n  key1: value1\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := constructEncryptionKeys(tc.input)
			assert.Equal(t, tc.expected, result, "Incorrect string returned")
		})
	}
}

func TestConstructEncryptionKeys_MultiElementMap(t *testing.T) {
	input := map[string]string{"key1": "value1", "key2": "value2"}
	expected1 := "customerEncryptionKeys:\n  key1: value1\n  key2: value2\n"
	expected2 := "customerEncryptionKeys:\n  key2: value2\n  key1: value1\n"

	result := constructEncryptionKeys(input)
	assert.True(t, result == expected1 || result == expected2, "Incorrect string returned")
}

func TestConstructANFSupportedTopologies(t *testing.T) {
	testCases := []struct {
		name     string
		region   string
		zones    []string
		expected string
	}{
		{
			name:     "Empty Zones",
			region:   "region1",
			zones:    []string{},
			expected: "",
		},
		{
			name:   "Single Zone",
			region: "region1",
			zones:  []string{"zone1"},
			expected: "supportedTopologies:\n  - topology.kubernetes.io/region: region1\n" +
				"    topology.kubernetes.io/zone: region1-zone1\n",
		},
		{
			name:   "Single Zone with Region Prefix",
			region: "region1",
			zones:  []string{"region1-zone1"},
			expected: "supportedTopologies:\n  - topology.kubernetes.io/region: region1\n" +
				"    topology.kubernetes.io/zone: region1-zone1\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := constructANFSupportedTopologies(tc.region, tc.zones)
			assert.Equal(t, tc.expected, result, "Incorrect supported topology string returned")
		})
	}
}
