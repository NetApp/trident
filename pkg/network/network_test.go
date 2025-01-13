// Copyright 2025 NetApp, Inc. All Rights Reserved.

package network

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFilterIPs(t *testing.T) {
	inputIPs := []string{
		"10.100.0.2", "192.168.0.1", "192.168.0.2", "10.100.0.1",
		"eb9b::2", "eb9b::1", "bd15::1", "bd15::2",
	}

	// Randomize the input list of IPs
	rand.Seed(time.Now().UnixNano())
	rand.Shuffle(len(inputIPs), func(i, j int) { inputIPs[i], inputIPs[j] = inputIPs[j], inputIPs[i] })

	type filterIPsIO struct {
		inputCIDRs []string
		outputIPs  []string
	}

	testIOs := make([]filterIPsIO, 0)
	// Return sorted list of all IPs
	testIOs = append(testIOs, filterIPsIO{
		inputCIDRs: []string{"0.0.0.0/0", "::/0"},
		outputIPs: []string{
			"10.100.0.1", "10.100.0.2", "192.168.0.1", "192.168.0.2",
			"bd15::1", "bd15::2", "eb9b::1", "eb9b::2",
		},
	})
	// Return sorted list of IPv4 only
	testIOs = append(testIOs, filterIPsIO{
		inputCIDRs: []string{"0.0.0.0/0"},
		outputIPs:  []string{"10.100.0.1", "10.100.0.2", "192.168.0.1", "192.168.0.2"},
	})
	// Return sorted list of IPv6 only
	testIOs = append(testIOs, filterIPsIO{
		inputCIDRs: []string{"::/0"},
		outputIPs:  []string{"bd15::1", "bd15::2", "eb9b::1", "eb9b::2"},
	})
	// Return only one sorted subnet of each IP type
	testIOs = append(testIOs, filterIPsIO{
		inputCIDRs: []string{"bd15::/16", "10.0.0.0/8"},
		outputIPs:  []string{"10.100.0.1", "10.100.0.2", "bd15::1", "bd15::2"},
	})
	// Return a single IPv4 address
	testIOs = append(testIOs, filterIPsIO{
		inputCIDRs: []string{"192.168.0.1/32"},
		outputIPs:  []string{"192.168.0.1"},
	})
	// Return a single IPv6 address
	testIOs = append(testIOs, filterIPsIO{
		inputCIDRs: []string{"eb9b::1/128"},
		outputIPs:  []string{"eb9b::1"},
	})
	// Return no IPs
	testIOs = append(testIOs, filterIPsIO{
		inputCIDRs: []string{"1.1.1.1/32"},
		outputIPs:  []string{},
	})
	// Give bad CIDR
	testIOs = append(testIOs, filterIPsIO{
		inputCIDRs: []string{"foo"},
		outputIPs:  nil,
	})

	for _, testIO := range testIOs {
		outputIPs, _ := FilterIPs(context.Background(), inputIPs, testIO.inputCIDRs)
		assert.Equal(t, testIO.outputIPs, outputIPs)
	}
}

func TestValidateCIDRSet(t *testing.T) {
	ctx := context.TODO()

	validIPv4CIDR := "0.0.0.0/0"
	validIPv6CIDR := "::/0"
	invalidIPv4CIDR := "subnet/invalid-cidr"
	invalidIPv6CIDR := "subnet:1234::/invalid-cidr"
	validIPv4InvalidCIDR := "0.0.0.0"
	validIPv6InvalidCIDR := "2001:0db8:0000:0000:0000:ff00:0042:8329"

	validCIDRSet := []string{
		validIPv4CIDR,
		validIPv6CIDR,
	}

	invalidCIDRSet := []string{
		"",
		" ",
		invalidIPv4CIDR,
		invalidIPv6CIDR,
		validIPv4InvalidCIDR,
		validIPv6InvalidCIDR,
	}

	invalidCIDRWithinSet := []string{
		validIPv4CIDR,
		invalidIPv4CIDR,
		validIPv4CIDR,
		invalidIPv6CIDR,
	}

	// Tests the positive case, where no invalid CIDR blocks are in the config
	t.Run("should not return an error for a set of valid CIDR blocks", func(t *testing.T) {
		assert.Equal(t, nil, ValidateCIDRs(ctx, validCIDRSet))
	})

	// Tests that the error contains all the invalid CIDR blocks
	t.Run("should return an error containing which CIDR blocks were invalid",
		func(t *testing.T) {
			err := ValidateCIDRs(ctx, invalidCIDRWithinSet)
			assert.Contains(t, err.Error(), invalidIPv4CIDR)
			assert.Contains(t, err.Error(), invalidIPv6CIDR)
		})

	// Tests invalid CIDR combinations
	tests := []struct {
		name     string
		cidrs    []string
		expected bool
	}{
		{
			name:     "should return err from a list containing only invalid CIDR blocks",
			cidrs:    invalidCIDRSet,
			expected: true,
		},
		{
			name:     "should return err from a list containing valid and invalid CIDR blocks",
			cidrs:    invalidCIDRWithinSet,
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, assert.Error(t, ValidateCIDRs(ctx, test.cidrs)))
		})
	}
}

func TestParseHostportIP(t *testing.T) {
	type IPAddresses struct {
		InputIP  string
		OutputIP string
	}
	tests := []IPAddresses{
		{
			InputIP:  "1.2.3.4:5678",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "1.2.3.4:5678,1001",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "1.2.3.4",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "[1:2:3:4]:5678",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "[1:2:3:4]:5678,1001",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "[1:2:3:4]",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]:5678",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]:5678,1001",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "2607:f8b0:4006:818:0:0:0:2004",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputIP, func(t *testing.T) {
			assert.Equal(t, testCase.OutputIP, ParseHostportIP(testCase.InputIP), "IP mismatch")
		})
	}
}

func TestEnsureHostportFormatted(t *testing.T) {
	type IPAddresses struct {
		InputIP  string
		OutputIP string
	}
	tests := []IPAddresses{
		{
			InputIP:  "1.2.3.4:5678",
			OutputIP: "1.2.3.4:5678",
		},
		{
			InputIP:  "1.2.3.4",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "[1:2:3:4]:5678",
			OutputIP: "[1:2:3:4]:5678",
		},
		{
			InputIP:  "[1:2:3:4]",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "1:2:3:4",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "2607:f8b0:4006:818:0:0:0:2004",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]:5678",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]:5678",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputIP, func(t *testing.T) {
			assert.Equal(t, testCase.OutputIP, EnsureHostportFormatted(testCase.InputIP),
				"Hostport not correctly formatted")
		})
	}
}

func TestDNS1123Regexes_MatchString(t *testing.T) {
	tests := map[string]struct {
		s        string
		expected bool
	}{
		"finds no match when supplied string is empty": {
			"",
			false,
		},
		"finds no match when supplied string doesn't match regex": {
			"pvc_2eff1a7e-679d-4fc6-892f-a6538cdbe278",
			false,
		},
		"finds no match when supplied string has invalid characters at beginning": {
			"-pvc_2eff1a7e-679d-4fc6-892f-a6538cdbe278",
			false,
		},
		"finds no match when supplied string has invalid characters at end": {
			"pvc-2eff1a7e-679d-4fc6-892f-a6538cdbe278-",
			false,
		},
		"finds match when supplied string matches regex": {
			"snap-2eff1a7e-679d-4fc6-892f-1nridmry3dj",
			true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, DNS1123DomainRegex.MatchString(test.s))
			assert.Equal(t, test.expected, DNS1123LabelRegex.MatchString(test.s))
		})
	}
}

func TestParseIPv6Valid(t *testing.T) {
	tests := map[string]struct {
		input     string
		output    bool
		predicate func(string) bool
	}{
		"IPv6 Address": {
			input:  "fd20:8b1e:b258:2000:f816:3eff:feec:0",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 Address with Brackets": {
			input:  "[fd20:8b1e:b258:2000:f816:3eff:feec:0]",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 Address with Port": {
			input:  "[fd20:8b1e:b258:2000:f816:3eff:feec:0]:8000",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 localhost Address": {
			input:  "::1",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 localhost Address with Brackets": {
			input:  "[::1]",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 Zero Address": {
			input:  "::",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 Zero Address with Brackets": {
			input:  "[::]",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.True(t, test.predicate(test.input), "Predicate failed")
		})
	}
}

func TestParseIPv4Valid(t *testing.T) {
	tests := map[string]struct {
		input     string
		output    bool
		predicate func(string) bool
	}{
		"IPv4 Address": {
			input:  "127.0.0.1",
			output: false,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv4 Address with Brackets": {
			input:  "[127.0.0.1]",
			output: false,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv4 Address with Port": {
			input:  "127.0.0.1:8000",
			output: false,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv4 Zero Address": {
			input:  "0.0.0.0",
			output: false,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.False(t, test.predicate(test.input), "Predicate failed")
		})
	}
}

func TestGetBaseImageName(t *testing.T) {
	remainder := GetBaseImageName("netapp/trident:19.10.0")
	assert.Equal(t, "trident:19.10.0", remainder)

	remainder = GetBaseImageName("")
	assert.Equal(t, "", remainder)

	remainder = GetBaseImageName("trident:19.10.0")
	assert.Equal(t, "trident:19.10.0", remainder)

	remainder = GetBaseImageName("quay.io/k8scsi/csi-node-driver-registrar:v1.0.2")
	assert.Equal(t, "csi-node-driver-registrar:v1.0.2", remainder)

	remainder = GetBaseImageName("mydomain:5000/k8scsi/csi-node-driver-registrar:v1.0.2")
	assert.Equal(t, "csi-node-driver-registrar:v1.0.2", remainder)
}

func TestReplaceImageRegistry(t *testing.T) {
	image := ReplaceImageRegistry("netapp/trident:19.10.0", "")
	assert.Equal(t, "trident:19.10.0", image)

	image = ReplaceImageRegistry("netapp/trident:19.10.0", "mydomain:5000")
	assert.Equal(t, "mydomain:5000/trident:19.10.0", image)

	image = ReplaceImageRegistry("quay.io/k8scsi/csi-node-driver-registrar:v1.0.2", "mydomain:5000")
	assert.Equal(t, "mydomain:5000/csi-node-driver-registrar:v1.0.2", image)
}
