package storage_drivers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReplaceStringIfPresent(t *testing.T) {
	testCases := []struct {
		name        string
		yaml        string
		placeholder string
		spec        map[string]interface{}
		key         string
		expected    string
	}{
		{
			name:        "String value present",
			yaml:        "config:\n  {STORAGE_PREFIX}\n",
			placeholder: "{STORAGE_PREFIX}",
			spec:        map[string]interface{}{"storagePrefix": "trident_"},
			key:         "storagePrefix",
			expected:    "config:\n  storagePrefix: trident_\n",
		},
		{
			name:        "Key not present",
			yaml:        "config:\n  {STORAGE_PREFIX}\n",
			placeholder: "{STORAGE_PREFIX}",
			spec:        map[string]interface{}{},
			key:         "storagePrefix",
			expected:    "config:\n  \n",
		},
		{
			name:        "Nil value",
			yaml:        "config:\n  {STORAGE_PREFIX}\n",
			placeholder: "{STORAGE_PREFIX}",
			spec:        map[string]interface{}{"storagePrefix": nil},
			key:         "storagePrefix",
			expected:    "config:\n  \n",
		},
		{
			name:        "Empty string value",
			yaml:        "config:\n  {STORAGE_PREFIX}\n",
			placeholder: "{STORAGE_PREFIX}",
			spec:        map[string]interface{}{"storagePrefix": ""},
			key:         "storagePrefix",
			expected:    "config:\n  \n",
		},
		{
			name:        "Non-string value converted to string",
			yaml:        "config:\n  {LIMIT_VOLUME_SIZE}\n",
			placeholder: "{LIMIT_VOLUME_SIZE}",
			spec:        map[string]interface{}{"limitVolumeSize": 100},
			key:         "limitVolumeSize",
			expected:    "config:\n  limitVolumeSize: 100\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := replaceStringIfPresent(tc.yaml, tc.placeholder, tc.spec, tc.key)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestReplaceBoolIfPresent(t *testing.T) {
	testCases := []struct {
		name        string
		yaml        string
		placeholder string
		spec        map[string]interface{}
		key         string
		expected    string
	}{
		{
			name:        "Bool true value",
			yaml:        "config:\n  {DEBUG}\n",
			placeholder: "{DEBUG}",
			spec:        map[string]interface{}{"debug": true},
			key:         "debug",
			expected:    "config:\n  debug: true\n",
		},
		{
			name:        "Bool false value",
			yaml:        "config:\n  {DEBUG}\n",
			placeholder: "{DEBUG}",
			spec:        map[string]interface{}{"debug": false},
			key:         "debug",
			expected:    "config:\n  debug: false\n",
		},
		{
			name:        "String true value",
			yaml:        "config:\n  {USE_REST}\n",
			placeholder: "{USE_REST}",
			spec:        map[string]interface{}{"useREST": "true"},
			key:         "useREST",
			expected:    "config:\n  useREST: true\n",
		},
		{
			name:        "String false value",
			yaml:        "config:\n  {USE_REST}\n",
			placeholder: "{USE_REST}",
			spec:        map[string]interface{}{"useREST": "false"},
			key:         "useREST",
			expected:    "config:\n  useREST: false\n",
		},
		{
			name:        "String TRUE value (case insensitive)",
			yaml:        "config:\n  {USE_CHAP}\n",
			placeholder: "{USE_CHAP}",
			spec:        map[string]interface{}{"useCHAP": "TRUE"},
			key:         "useCHAP",
			expected:    "config:\n  useCHAP: true\n",
		},
		{
			name:        "Key not present",
			yaml:        "config:\n  {DEBUG}\n",
			placeholder: "{DEBUG}",
			spec:        map[string]interface{}{},
			key:         "debug",
			expected:    "config:\n  \n",
		},
		{
			name:        "Unsupported type removes placeholder",
			yaml:        "config:\n  {DEBUG}\n",
			placeholder: "{DEBUG}",
			spec:        map[string]interface{}{"debug": 123},
			key:         "debug",
			expected:    "config:\n  \n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := replaceBoolIfPresent(tc.yaml, tc.placeholder, tc.spec, tc.key)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestReplaceArrayIfPresent(t *testing.T) {
	testCases := []struct {
		name        string
		yaml        string
		placeholder string
		spec        map[string]interface{}
		key         string
		expected    string
	}{
		{
			name:        "[]interface{} with values",
			yaml:        "config:\n  {AUTO_EXPORT_CIDRS}\n",
			placeholder: "{AUTO_EXPORT_CIDRS}",
			spec:        map[string]interface{}{"autoExportCIDRs": []interface{}{"10.0.0.0/8", "192.168.0.0/16"}},
			key:         "autoExportCIDRs",
			expected:    "config:\n  autoExportCIDRs: [10.0.0.0/8, 192.168.0.0/16]\n",
		},
		{
			name:        "[]string with values",
			yaml:        "config:\n  {FLEXGROUP_AGGREGATE_LIST}\n",
			placeholder: "{FLEXGROUP_AGGREGATE_LIST}",
			spec:        map[string]interface{}{"flexgroupAggregateList": []string{"aggr1", "aggr2"}},
			key:         "flexgroupAggregateList",
			expected:    "config:\n  flexgroupAggregateList: [aggr1, aggr2]\n",
		},
		{
			name:        "Empty []interface{}",
			yaml:        "config:\n  {AUTO_EXPORT_CIDRS}\n",
			placeholder: "{AUTO_EXPORT_CIDRS}",
			spec:        map[string]interface{}{"autoExportCIDRs": []interface{}{}},
			key:         "autoExportCIDRs",
			expected:    "config:\n  \n",
		},
		{
			name:        "Empty []string",
			yaml:        "config:\n  {FLEXGROUP_AGGREGATE_LIST}\n",
			placeholder: "{FLEXGROUP_AGGREGATE_LIST}",
			spec:        map[string]interface{}{"flexgroupAggregateList": []string{}},
			key:         "flexgroupAggregateList",
			expected:    "config:\n  \n",
		},
		{
			name:        "Key not present",
			yaml:        "config:\n  {AUTO_EXPORT_CIDRS}\n",
			placeholder: "{AUTO_EXPORT_CIDRS}",
			spec:        map[string]interface{}{},
			key:         "autoExportCIDRs",
			expected:    "config:\n  \n",
		},
		{
			name:        "Nil value",
			yaml:        "config:\n  {AUTO_EXPORT_CIDRS}\n",
			placeholder: "{AUTO_EXPORT_CIDRS}",
			spec:        map[string]interface{}{"autoExportCIDRs": nil},
			key:         "autoExportCIDRs",
			expected:    "config:\n  \n",
		},
		{
			name:        "Unsupported type",
			yaml:        "config:\n  {AUTO_EXPORT_CIDRS}\n",
			placeholder: "{AUTO_EXPORT_CIDRS}",
			spec:        map[string]interface{}{"autoExportCIDRs": "not-an-array"},
			key:         "autoExportCIDRs",
			expected:    "config:\n  \n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := replaceArrayIfPresent(tc.yaml, tc.placeholder, tc.spec, tc.key)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestFormatValueForYAML(t *testing.T) {
	testCases := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{
			name:     "String value",
			input:    "test-string",
			expected: "test-string",
		},
		{
			name:     "Bool true",
			input:    true,
			expected: "true",
		},
		{
			name:     "Bool false",
			input:    false,
			expected: "false",
		},
		{
			name:     "Int value",
			input:    42,
			expected: "42",
		},
		{
			name:     "Int64 value",
			input:    int64(100),
			expected: "100",
		},
		{
			name:     "Float64 whole number",
			input:    float64(200),
			expected: "200",
		},
		{
			name:     "Float64 with decimal",
			input:    float64(3.14),
			expected: "3.14",
		},
		{
			name:     "Float32 whole number",
			input:    float32(50),
			expected: "50",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := formatValueForYAML(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestReplaceLabelsIfPresent(t *testing.T) {
	testCases := []struct {
		name        string
		yaml        string
		placeholder string
		spec        map[string]interface{}
		key         string
		expected    string
	}{
		{
			name:        "map[string]interface{} with values",
			yaml:        "config:\n  {BACKEND_LABELS}\n",
			placeholder: "{BACKEND_LABELS}",
			spec:        map[string]interface{}{"labels": map[string]interface{}{"app": "trident", "env": "prod"}},
			key:         "labels",
			expected:    "config:\n  labels:\n",
		},
		{
			name:        "map[string]string with values",
			yaml:        "config:\n  {BACKEND_LABELS}\n",
			placeholder: "{BACKEND_LABELS}",
			spec:        map[string]interface{}{"labels": map[string]string{"app": "trident"}},
			key:         "labels",
			expected:    "config:\n  labels:\n    app: \"trident\"\n\n",
		},
		{
			name:        "Empty map",
			yaml:        "config:\n  {BACKEND_LABELS}\n",
			placeholder: "{BACKEND_LABELS}",
			spec:        map[string]interface{}{"labels": map[string]interface{}{}},
			key:         "labels",
			expected:    "config:\n  \n",
		},
		{
			name:        "Key not present",
			yaml:        "config:\n  {BACKEND_LABELS}\n",
			placeholder: "{BACKEND_LABELS}",
			spec:        map[string]interface{}{},
			key:         "labels",
			expected:    "config:\n  \n",
		},
		{
			name:        "Nil value",
			yaml:        "config:\n  {BACKEND_LABELS}\n",
			placeholder: "{BACKEND_LABELS}",
			spec:        map[string]interface{}{"labels": nil},
			key:         "labels",
			expected:    "config:\n  \n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := replaceLabelsIfPresent(tc.yaml, tc.placeholder, tc.spec, tc.key)
			// For map[string]interface{} the order is not guaranteed, so just check it contains key parts
			if tc.name == "map[string]interface{} with values" {
				assert.Contains(t, result, "labels:")
			} else {
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

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

func TestConstructMountOptions(t *testing.T) {
	testCases := []struct {
		name     string
		input    map[string]string
		expected string
	}{
		{
			name:     "No mount options",
			input:    map[string]string{},
			expected: "",
		},
		{
			name:     "Single mount option",
			input:    map[string]string{"key1": "value1"},
			expected: "mountOptions:\n  - key1=value1\n",
		},
		{
			name:     "Multiple mount options",
			input:    map[string]string{"key1": "value1", "key2": "value2"},
			expected: "mountOptions:\n  - key1=value1\n  - key2=value2\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := constructMountOptions(tc.input)
			assert.Equal(t, tc.expected, result, "Incorrect mount options string returned")
		})
	}
}
