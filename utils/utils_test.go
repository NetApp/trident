// Copyright 2021 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var testStringSlice = []string{
	"foo",
	"bar",
	"earn",
	"baz",
	"con:1234",
	"silicon:1234",
	"bigstring",
	"verybigstring",
	"superbingstring",
}

var testIntSlice = []int{
	1,
	-1,
	9999,
	0,
	56355634,
}

type CustomString string

var testCustomStringSlice = []CustomString{
	"foo",
	"bar",
	"earn",
	"baz",
	"con:1234",
	"silicon:1234",
	"bigstring",
	"verybigstring",
	"superbingstring",
}

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}

func TestPow(t *testing.T) {
	log.Debug("Running TestPow...")

	if Pow(1024, 0) != 1 {
		t.Error("Expected 1024^0 == 1")
	}

	if Pow(1024, 1) != 1024 {
		t.Error("Expected 1024^1 == 1024")
	}

	if Pow(1024, 2) != 1048576 {
		t.Error("Expected 1024^2 == 1048576")
	}

	if Pow(1024, 3) != 1073741824 {
		t.Error("Expected 1024^3 == 1073741824")
	}
}

func TestConvertSizeToBytes(t *testing.T) {
	log.Debug("Running TestConvertSizeToBytes...")

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
		s, err := ConvertSizeToBytes(k)
		if err != nil {
			t.Errorf("Encountered '%v' running ConvertSizeToBytes('%v')", err, k)
		} else if s != v {
			t.Errorf("Expected ConvertSizeToBytes('%v') == '%v' but was %v", k, v, s)
		}
	}
}

func TestGetV(t *testing.T) {
	log.Debug("Running TestGetV...")

	d := make(map[string]string)
	d["key1"] = "value1"

	if val := GetV(d, "key1", "defaultValue"); val != "value1" {
		t.Errorf("Expected '%v' but was %v", "value1", val)
	}

	if val := GetV(d, "key2", "defaultValue"); val != "defaultValue" {
		t.Errorf("Expected '%v' but was %v", "defaultValue", val)
	}
}

func TestVolumeSizeWithinTolerance(t *testing.T) {
	log.Debug("Running TestVolumeSizeWithinTolerance...")

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

		isSameSize, err := VolumeSizeWithinTolerance(vst.requestedSize, vst.currentSize, vst.delta)
		if err != nil {
			t.Errorf("Encountered '%v' running TestVolumeSizeWithinTolerance", err)
		}

		assert.Equal(t, vst.expected, isSameSize)
	}
}

func TestSliceContainsString(t *testing.T) {
	log.Debug("Running TestSliceContainsString...")

	testCases := []struct {
		Text           string
		ExpectedResult bool
	}{
		// Positive Cases
		{"foo", true},
		{"silicon:1234", true},

		// Negative Cases
		{"Foo", false},
		{"foobar", false},
		{"sIliCon:1234", false},
		{"silicon:12344", false},
		{"", false},
	}

	for _, testCase := range testCases {
		contains := SliceContainsString(testStringSlice, testCase.Text)
		assert.Equal(t, testCase.ExpectedResult, contains)
	}
}

func TestSliceContains(t *testing.T) {
	log.Debug("Running TestSliceContains...")

	testCases := []struct {
		SliceName  interface{}
		SliceValue interface{}
		Contains   bool
	}{
		// Positive Cases
		{testStringSlice, "foo", true},
		{testStringSlice, "silicon:1234", true},
		{testCustomStringSlice, CustomString("foo"), true},
		{testCustomStringSlice, CustomString("silicon:1234"), true},
		{testIntSlice, -1, true},
		{testIntSlice, 0, true},
		{testIntSlice, 56355634, true},

		// Negative Cases
		{testStringSlice, "Foo", false},
		{testStringSlice, "foobar", false},
		{testStringSlice, "sIliCon:1234", false},
		{testStringSlice, "silicon:12344", false},
		{testStringSlice, "", false},
		{testCustomStringSlice, "foo", false},
		{testCustomStringSlice, "silicon:1234", false},
		{testCustomStringSlice, CustomString("Foo"), false},
		{testCustomStringSlice, CustomString("foobar"), false},
		{testCustomStringSlice, CustomString("sIliCon:1234"), false},
		{testCustomStringSlice, CustomString("silicon:12344"), false},
		{testCustomStringSlice, CustomString(""), false},
		{testIntSlice, "-1", false},
		{testIntSlice, -2, false},
		{testIntSlice, -9999, false},
		{testIntSlice, 100000, false},
	}

	for _, testCase := range testCases {
		contains := SliceContains(testCase.SliceName, testCase.SliceValue)
		assert.Equal(t, testCase.Contains, contains)
	}
}

func TestSliceContainElements(t *testing.T) {
	log.Debug("Running TestSliceContainElements...")

	testCases := []struct {
		SliceName    interface{}
		SliceValues  interface{}
		ContainsAll  bool
		ContainsSome bool
	}{
		// ContainsAll
		{testStringSlice, testStringSlice, true, true},
		{testStringSlice, testStringSlice[1:4], true, true},
		{testStringSlice, append(testStringSlice, "foo"), true, true},
		{append(testStringSlice, "foo"), testStringSlice, true, true},
		{testStringSlice, []string{"foo", "bar"}, true, true},
		{testStringSlice, []string{"silicon:1234"}, true, true},

		{testCustomStringSlice, testCustomStringSlice, true, true},
		{testCustomStringSlice, testCustomStringSlice[1:4], true, true},
		{testCustomStringSlice, append(testCustomStringSlice, "foo"), true, true},
		{append(testCustomStringSlice, "foo"), testCustomStringSlice, true, true},
		{testCustomStringSlice, []CustomString{"foo", "bar"}, true, true},
		{testCustomStringSlice, []CustomString{"silicon:1234"}, true, true},

		{testIntSlice, testIntSlice, true, true},
		{testIntSlice, testIntSlice[1:3], true, true},
		{testIntSlice, append(testIntSlice, 1), true, true},
		{append(testIntSlice, 1), testIntSlice, true, true},
		{testIntSlice, []int{1, 56355634}, true, true},
		{testIntSlice, []int{9999}, true, true},

		// ContainsSome
		{testStringSlice, append(testStringSlice, "new"), false, true},
		{[]string{"foo", "bar"}, testStringSlice, false, true},
		{testStringSlice, []string{"foo", "bar1"}, false, true},
		{testStringSlice, []string{"", "foo"}, false, true},

		{testCustomStringSlice, append(testCustomStringSlice, "new"), false, true},
		{[]CustomString{"foo", "bar"}, testCustomStringSlice, false, true},
		{testCustomStringSlice, []CustomString{"foo", "bar1"}, false, true},
		{testCustomStringSlice, []CustomString{"", "foo"}, false, true},

		{testIntSlice, append(testIntSlice, 2), false, true},
		{[]int{1, 56355634}, testIntSlice, false, true},
		{testIntSlice, []int{1, 563556341}, false, true},
		{testIntSlice, []int{2, 56355634}, false, true},

		// Negative Cases
		{testStringSlice, []string{}, false, false},
		{testStringSlice, "", false, false},
		{[]string{}, testStringSlice, false, false},
		{"", testStringSlice, false, false},
		{"", "", false, false},
		{testStringSlice, "foo", false, false},
		{"foo", testStringSlice, false, false},
		{"foo", "foo", false, false},
		{"foo", "", false, false},
		{testStringSlice, []string{"foo1", "bar1"}, false, false},
		{testStringSlice, []string{"sIliCon:1234"}, false, false},
		{testStringSlice, []string{"foobar"}, false, false},
		{testStringSlice, testCustomStringSlice, false, false},
		{testStringSlice, testIntSlice, false, false},

		{testCustomStringSlice, []CustomString{}, false, false},
		{testCustomStringSlice, "", false, false},
		{[]CustomString{}, testCustomStringSlice, false, false},
		{"", testCustomStringSlice, false, false},
		{"", "", false, false},
		{testCustomStringSlice, "foo", false, false},
		{"foo", testCustomStringSlice, false, false},
		{"foo", "foo", false, false},
		{"foo", "", false, false},
		{testCustomStringSlice, []CustomString{"foo1", "bar1"}, false, false},
		{testCustomStringSlice, []CustomString{"sIliCon:1234"}, false, false},
		{testCustomStringSlice, []CustomString{"foobar"}, false, false},
		{testCustomStringSlice, testStringSlice, false, false},
		{testCustomStringSlice, testIntSlice, false, false},

		{testIntSlice, []int{}, false, false},
		{testIntSlice, "", false, false},
		{testIntSlice, 0, false, false},
		{[]int{}, testIntSlice, false, false},
		{"", testIntSlice, false, false},
		{0, testIntSlice, false, false},
		{"", "", false, false},
		{0, 0, false, false},
		{testIntSlice, 1, false, false},
		{1, testIntSlice, false, false},
		{1, 1, false, false},
		{1, "", false, false},
		{1, 0, false, false},
		{testIntSlice, []int{2, -2}, false, false},
		{testIntSlice, []int{100000}, false, false},
		{testIntSlice, []int{-9999}, false, false},
		{testIntSlice, testStringSlice, false, false},
		{testIntSlice, testCustomStringSlice, false, false},
	}

	for _, testCase := range testCases {
		containsAll, containsSome := SliceContainsElements(testCase.SliceName, testCase.SliceValues)
		assert.Equal(t, testCase.ContainsAll, containsAll, "containsAll mismatch")
		assert.Equal(t, testCase.ContainsSome, containsSome, "containsSome mismatch")
	}
}

func TestRemoveStringFromSlice(t *testing.T) {
	log.Debug("Running TestRemoveStringFromSlice...")

	slice := []string{
		"foo",
		"bar",
		"baz",
	}
	updatedSlice := slice

	updatedSlice = RemoveStringFromSlice(updatedSlice, "foo")
	if SliceContainsString(updatedSlice, "foo") {
		t.Errorf("Slice should NOT contain string %v", "foo")
	}

	updatedSlice = RemoveStringFromSlice(updatedSlice, "bar")
	if SliceContainsString(updatedSlice, "bar") {
		t.Errorf("Slice should NOT contain string %v", "bar")
	}

	updatedSlice = RemoveStringFromSlice(updatedSlice, "baz")
	if SliceContainsString(updatedSlice, "foo") {
		t.Errorf("Slice should NOT contain string %v", "baz")
	}

	if len(updatedSlice) != 0 {
		t.Errorf("Slice should be empty")
	}
}

func TestRemoveStringFromSliceConditionally(t *testing.T) {
	log.Debug("Running TestRemoveStringFromSlice...")

	updatedSlice := make([]string, len(testStringSlice))
	copy(updatedSlice, testStringSlice)

	updatedSlice = RemoveStringFromSliceConditionally(updatedSlice, "foo",
		func(val1, val2 string) bool { return val1 == val2 })
	if SliceContainsString(updatedSlice, "foo") {
		t.Errorf("Slice should NOT contain string %v", "foo")
	}

	lenBefore := len(updatedSlice)
	updatedSlice = RemoveStringFromSliceConditionally(updatedSlice, "random",
		func(val1, val2 string) bool { return val1 == val2 })
	lenAfter := len(updatedSlice)
	if lenBefore != lenAfter {
		t.Errorf("Slice should have NOT removed element(s)")
	}

	updatedSlice = RemoveStringFromSliceConditionally(updatedSlice, "earnest",
		func(main, val string) bool { return strings.Contains(main, val) })
	if SliceContainsString(updatedSlice, "ear") {
		t.Errorf("Slice should NOT contain string %v", "ear")
	}
	if SliceContainsString(updatedSlice, "earn") {
		t.Errorf("Slice should NOT contain string %v", "earn")
	}

	updatedSlice = RemoveStringFromSliceConditionally(updatedSlice, "con:3421",
		func(main, val string) bool {
			mainIpAddress := strings.Split(main, ":")[0]
			valIpAddress := strings.Split(val, ":")[0]
			return mainIpAddress == valIpAddress
		})
	if SliceContainsString(updatedSlice, "con:1234") {
		t.Errorf("Slice should NOT contain string %v", "con:1234")
	}
	if !SliceContainsString(updatedSlice, "silicon:1234") {
		t.Errorf("Slice should contain string %v", "silicon:1234")
	}

	updatedSlice = RemoveStringFromSliceConditionally(updatedSlice, "bigstring",
		func(main, val string) bool { return len(val) > len(main) })
	if SliceContainsString(updatedSlice, "verybigstring") {
		t.Errorf("Slice should NOT contain string %v", "verybigstring")
	}
	if SliceContainsString(updatedSlice, "superbingstring") {
		t.Errorf("Slice should NOT contain string %v", "superbingstring")
	}
}

func TestSliceContainsStringCaseInsensitive(t *testing.T) {
	log.Debug("Running TestSliceContainsStringCaseInsensitive...")

	testCases := []struct {
		Text           string
		ExpectedResult bool
	}{
		// Positive Cases
		{"foo", true},
		{"Foo", true},
		{"silicon:1234", true},
		{"sIliCon:1234", true},

		// Negative Cases
		{"foobar", false},
		{"silicon:12344", false},
		{"", false},
	}

	for _, testCase := range testCases {
		contains := SliceContainsStringCaseInsensitive(testStringSlice, testCase.Text)
		assert.Equal(t, testCase.ExpectedResult, contains)
	}
}

func TestSliceContainsStringConditionally(t *testing.T) {
	log.Debug("Running TestSliceContainsStringConditionally...")

	testCases := []struct {
		Text           string
		MatchFunc      func(string, string) bool
		ExpectedResult bool
	}{
		{"foo", func(val1, val2 string) bool { return val1 == val2 }, true},
		{"Foo", func(val1, val2 string) bool { return val1 == val2 }, false},
		{"Foo", func(val1, val2 string) bool { return strings.EqualFold(val1, val2) }, true},
		{"ear", func(val1, val2 string) bool { return val1 == val2 }, false},
		{"Ear", func(val1, val2 string) bool { return val1 == val2 }, false},
		{"Ear", func(val1, val2 string) bool { return strings.EqualFold(val1, val2) }, false},
		{"ear", func(val1, val2 string) bool { return strings.Contains(val1, val2) }, true},
		{"Ear", func(val1, val2 string) bool { return strings.Contains(val1, val2) }, false},
		{
			"Ear",
			func(val1, val2 string) bool { return strings.Contains(strings.ToLower(val1), strings.ToLower(val2)) },
			true,
		},
	}

	for _, testCase := range testCases {
		contains := SliceContainsStringConditionally(testStringSlice, testCase.Text, testCase.MatchFunc)
		assert.Equal(t, testCase.ExpectedResult, contains)
	}
}

func TestSplitImageDomain(t *testing.T) {
	log.Debug("Running TestSplitImageDomain...")

	domain, remainder := SplitImageDomain("netapp/trident:19.10.0")
	assert.Equal(t, "", domain)
	assert.Equal(t, "netapp/trident:19.10.0", remainder)

	domain, remainder = SplitImageDomain("quay.io/k8scsi/csi-node-driver-registrar:v1.0.2")
	assert.Equal(t, "quay.io", domain)
	assert.Equal(t, "k8scsi/csi-node-driver-registrar:v1.0.2", remainder)

	domain, remainder = SplitImageDomain("mydomain:5000/k8scsi/csi-node-driver-registrar:v1.0.2")
	assert.Equal(t, "mydomain:5000", domain)
	assert.Equal(t, "k8scsi/csi-node-driver-registrar:v1.0.2", remainder)
}

func TestReplaceImageRegistry(t *testing.T) {
	log.Debug("Running ReplaceImageRegistry...")

	image := ReplaceImageRegistry("netapp/trident:19.10.0", "")
	assert.Equal(t, "netapp/trident:19.10.0", image)

	image = ReplaceImageRegistry("netapp/trident:19.10.0", "mydomain:5000")
	assert.Equal(t, "mydomain:5000/netapp/trident:19.10.0", image)

	image = ReplaceImageRegistry("quay.io/k8scsi/csi-node-driver-registrar:v1.0.2", "mydomain:5000")
	assert.Equal(t, "mydomain:5000/k8scsi/csi-node-driver-registrar:v1.0.2", image)
}

func TestFilterIPs(t *testing.T) {
	log.Debug("Running TestFilterIPs...")

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
		outputIPs, _ := FilterIPs(ctx(), inputIPs, testIO.inputCIDRs)
		assert.Equal(t, testIO.outputIPs, outputIPs)
	}
}

const inputString = `
text
{REPLACE-1}
  something: else
  {REPLACE-2}
  {NO-REPLACE-1}x
  c{NO-REPLACE-2}
    something: else
    {NO_REPLACE_3}y
    {REPLACE-3}
    {REPLACE-4}
`

func TestGetYAMLTagWithSpaceCount(t *testing.T) {
	log.Debug("Running TestGetYAMLTagWithSpaceCount...")

	tagWithSpaces, spaces := GetYAMLTagWithSpaceCount(inputString, "REPLACE-1")
	assert.Equal(t, tagWithSpaces, "{REPLACE-1}\n")
	assert.Equal(t, spaces, 0)

	inputStringCopy := strings.ReplaceAll(inputString, tagWithSpaces, "#"+tagWithSpaces)

	tagWithSpaces, spaces = GetYAMLTagWithSpaceCount(inputString, "REPLACE-2")
	assert.Equal(t, tagWithSpaces, "  {REPLACE-2}\n")
	assert.Equal(t, spaces, 2)

	inputStringCopy = strings.ReplaceAll(inputStringCopy, tagWithSpaces, "#"+tagWithSpaces)

	tagWithSpaces, spaces = GetYAMLTagWithSpaceCount(inputString, "REPLACE-3")
	assert.Equal(t, tagWithSpaces, "    {REPLACE-3}\n")
	assert.Equal(t, spaces, 4)

	inputStringCopy = strings.ReplaceAll(inputStringCopy, tagWithSpaces, "#"+tagWithSpaces)

	tagWithSpaces, spaces = GetYAMLTagWithSpaceCount(inputString, "REPLACE-4")
	assert.Equal(t, tagWithSpaces, "    {REPLACE-4}\n")
	assert.Equal(t, spaces, 4)
}

func TestCountSpacesBeforeText(t *testing.T) {
	log.Debug("Running TestCountSpacesBeforeText...")

	type TextWithSpaces struct {
		Text   string
		Spaces int
	}

	multipleTextsWithSpaces := []TextWithSpaces{
		{"text", 0},
		{"  text", 2},
		{"  text  ", 2},
		{"      text", 6},
		{"      text      ", 6},
	}

	for _, textWithSpaces := range multipleTextsWithSpaces {
		spaceCount := CountSpacesBeforeText(textWithSpaces.Text)
		assert.Equal(t, spaceCount, textWithSpaces.Spaces)
	}
}

func TestGetNFSVersionFromMountOptions(t *testing.T) {
	log.Debug("Running TestGetNFSVersionFromMountOptions...")

	defaultVersion := "3"
	supportedVersions := []string{"3", "4", "4.1"}

	tests := []struct {
		mountOptions      string
		defaultVersion    string
		supportedVersions []string
		version           string
		errNotNil         bool
	}{
		// Positive tests
		{"", defaultVersion, supportedVersions, defaultVersion, false},
		{"", defaultVersion, nil, defaultVersion, false},
		{"vers=3", defaultVersion, supportedVersions, defaultVersion, false},
		{"tcp, vers=3", defaultVersion, supportedVersions, defaultVersion, false},
		{"-o hard,vers=4", defaultVersion, supportedVersions, "4", false},
		{"nfsvers=3 , timeo=300", defaultVersion, supportedVersions, defaultVersion, false},
		{"retry=1, nfsvers=4", defaultVersion, supportedVersions, "4", false},
		{"retry=1", defaultVersion, supportedVersions, defaultVersion, false},
		{"vers=4,minorversion=1", defaultVersion, supportedVersions, "4.1", false},
		{"vers=4.0,minorversion=1", defaultVersion, supportedVersions, "4.1", false},
		{"vers=2,vers=3", defaultVersion, supportedVersions, "3", false},
		{"vers=5", defaultVersion, nil, "5", false},
		{"minorversion=2,nfsvers=4.1", defaultVersion, supportedVersions, "4.1", false},

		// Negative tests
		{"vers=2", defaultVersion, supportedVersions, "2", true},
		{"vers=3,minorversion=1", defaultVersion, supportedVersions, "3.1", true},
		{"vers=4,minorversion=2", defaultVersion, supportedVersions, "4.2", true},
		{"vers=4.1", defaultVersion, []string{"3", "4.2"}, "4.1", true},
	}

	for _, test := range tests {

		version, err := GetNFSVersionFromMountOptions(test.mountOptions, test.defaultVersion, test.supportedVersions)

		assert.Equal(t, test.version, version)
		assert.Equal(t, test.errNotNil, err != nil)
	}
}

func TestGetNFSVersionMountOptions(t *testing.T) {
	log.Debug("Running TestGetNFSVersionMountOptions...")

	tests := []struct {
		mountOptions    string
		NFSMountOptions []string
	}{
		{"", []string{}},
		{"", []string{}},
		{"vers=3", []string{"vers=3"}},
		{"tcp, vers=3", []string{"vers=3"}},
		{"-o hard,vers=4", []string{"vers=4"}},
		{"nfsvers=3 , timeo=300", []string{"nfsvers=3"}},
		{"retry=1, nfsvers=4", []string{"nfsvers=4"}},
		{"retry=1", []string{}},
		{"vers=4,minorversion=1", []string{"vers=4", "minorversion=1"}},
		{"vers=4.0,minorversion=1", []string{"vers=4.0", "minorversion=1"}},
		{"vers=2,vers=3", []string{"vers=2", "vers=3"}},
		{"vers=5", []string{"vers=5"}},
		{"minorversion=2,nfsvers=4.1", []string{"minorversion=2", "nfsvers=4.1"}},
		{"vers=2", []string{"vers=2"}},
		{"vers=3,minorversion=1", []string{"vers=3", "minorversion=1"}},
		{"vers=4,minorversion=2", []string{"vers=4", "minorversion=2"}},
		{"vers=4.1", []string{"vers=4.1"}},
	}

	for _, test := range tests {

		nfsMountOptions := GetNFSVersionMountOptions(test.mountOptions)
		assert.Equal(t, test.NFSMountOptions, nfsMountOptions)
	}
}

func TestSetNFSVersionMountOptions(t *testing.T) {
	log.Debug("Running TestSetNFSVersionMountOptions...")

	tests := []struct {
		mountOptions      string
		newNFSMountOption string
		newMountOptions   string
	}{
		{"", "vers=3", "vers=3"},
		{"", "vers=4.1", "vers=4.1"},
		{"vers=4.1", "vers=4.1", "vers=4.1"},
		{"vers=4.1", "vers=7", "vers=7"},
		{"vers=3", "vers=4.1", "vers=4.1"},
		{"tcp,vers=3,", "vers=4.1", "tcp,vers=4.1"},
		{"-o hard,vers=4", "vers=4.1", "hard,vers=4.1"},
		{"-o vers=4,hard", "vers=4.1", "hard,vers=4.1"},
		{"nfsvers=3,timeo=300", "vers=3.0", "timeo=300,vers=3.0"},
		{"retry=1,nfsvers=4,", "minorversion=2", "retry=1,minorversion=2"},
		{"retry=1", "nfsvers=3", "retry=1,nfsvers=3"},
		{"vers=4,minorversion=1", "vers=4.1", "vers=4.1"},
		{"vers=4.0,minorversion=1", "vers=3", "vers=3"},
		{"vers=2,vers=3,", "minorversion=2", "minorversion=2"},
		{"vers=5", "vers=4.1", "vers=4.1"},
		{"minorversion=2,nfsvers=4.1", "nfsvers=3", "nfsvers=3"},
		{"vers=2", "minorversion=2,nfsvers=4.1", "minorversion=2,nfsvers=4.1"},
		{"vers=3,minorversion=1,timeo=300", "minorversion=2,nfsvers=4.1", "timeo=300,minorversion=2,nfsvers=4.1"},
		{"vers=4.1", "vers=4.0", "vers=4.0"},
	}

	for _, test := range tests {

		nfsMountOptions := SetNFSVersionMountOptions(test.mountOptions, test.newNFSMountOption)
		assert.Equal(t, test.newMountOptions, nfsMountOptions)
	}
}

const multipathConf = `
defaults {
    user_friendly_names yes
    find_multipaths no
}

`

func TestGetFindMultipathValue(t *testing.T) {
	log.Debug("Running TestGetFindMultipathValue...")

	findMultipathsValue := GetFindMultipathValue(multipathConf)
	assert.Equal(t, "no", findMultipathsValue)

	inputStringCopy := strings.ReplaceAll(multipathConf, "find_multipaths", "#find_multipaths")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "find_multipaths no", "")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "yes")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "yes", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "'yes'")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "yes", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "'on'")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "yes", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "'off'")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "no", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "on")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "yes", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "off")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "no", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "random")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "random", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "smart")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "smart", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "greedy")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "greedy", findMultipathsValue)

	inputStringCopy = strings.ReplaceAll(multipathConf, "no", "'no'")

	findMultipathsValue = GetFindMultipathValue(inputStringCopy)
	assert.Equal(t, "no", findMultipathsValue)
}

func TestSplitString(t *testing.T) {
	log.Debug("Running TestSplitString...")

	ctx := context.TODO()

	stringList := SplitString(ctx, "a,b, c", ",")
	assert.Equal(t, []string{"a", "b", " c"}, stringList)

	stringList = SplitString(ctx, "a,b,c", ",")
	assert.Equal(t, []string{"a", "b", "c"}, stringList)

	stringList = SplitString(ctx, "a,b,c", "")
	assert.Equal(t, []string{"a", ",", "b", ",", "c"}, stringList)

	stringList = SplitString(ctx, "a,b,c", ";")
	assert.Equal(t, []string{"a,b,c"}, stringList)

	stringList = SplitString(ctx, "", ",")
	assert.Equal(t, []string(nil), stringList)

	stringList = SplitString(ctx, "", ";")
	assert.Equal(t, []string(nil), stringList)

	stringList = SplitString(ctx, " ", ";")
	assert.Equal(t, []string{" "}, stringList)

	stringList = SplitString(ctx, ";a;b", ";")
	assert.Equal(t, []string{"", "a", "b"}, stringList)
}

func TestReplaceAtIndex(t *testing.T) {
	actual, err := ReplaceAtIndex("foo", 'f', 0)
	assert.Nil(t, err)
	assert.Equal(t, "foo", actual)

	actual, err = ReplaceAtIndex("boo", 'f', 0)
	assert.Nil(t, err)
	assert.Equal(t, "foo", actual)

	actual, err = ReplaceAtIndex("fof", 'o', 2)
	assert.Nil(t, err)
	assert.Equal(t, "foo", actual)

	actual, err = ReplaceAtIndex("boo", 'f', -1)
	assert.NotNil(t, err)
	assert.Equal(t, "boo", actual)

	actual, err = ReplaceAtIndex("boo", 'f', 3)
	assert.NotNil(t, err)
	assert.Equal(t, "boo", actual)

	actual, err = ReplaceAtIndex("boo", 'f', 50)
	assert.NotNil(t, err)
	assert.Equal(t, "boo", actual)
}

func TestMinInt64(t *testing.T) {
	log.Debug("Running TestMinInt64...")
	assert.Equal(t, int64(2), MinInt64(2, 3))
	assert.Equal(t, int64(2), MinInt64(3, 2))
	assert.Equal(t, int64(-2), MinInt64(-2, 3))
	assert.Equal(t, int64(-2), MinInt64(3, -2))
	assert.Equal(t, int64(-3), MinInt64(3, -3))
	assert.Equal(t, int64(-3), MinInt64(-3, 3))
	assert.Equal(t, int64(0), MinInt64(0, 0))
	assert.Equal(t, int64(2), MinInt64(2, 2))
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

func TestValidateOctalUnixPermissions(t *testing.T) {
	tests := []struct {
		perms     string
		errNotNil bool
	}{
		// Positive tests
		{"0700", false},
		{"0755", false},
		{"0000", false},
		{"7777", false},

		// Negative tests
		{"", true},
		{"777", true},
		{"77777", true},
		{"8777", true},
		{"7778", true},
	}

	for _, test := range tests {

		err := ValidateOctalUnixPermissions(test.perms)

		assert.Equal(t, test.errNotNil, err != nil)
	}
}

func TestMustParseMajorMinorVersion(t *testing.T) {
	majorMinor := "v1.23"
	majorMinorVersion := MustParseSemantic(majorMinor + ".0").ToMajorMinorVersion()
	invalid := "v1.23.0"
	majorOnly := "v1"

	assert.Equal(t, majorMinorVersion, MustParseMajorMinorVersion(majorMinor))
	assert.Panics(t, func() { MustParseMajorMinorVersion(invalid) }, "a version including the patch version"+
		" should cause a panic")
	assert.Panics(t, func() { MustParseMajorMinorVersion(majorOnly) }, "a version consisting of only the"+
		" major version should cause a panic")
}

func TestRedactSecretsFromString(t *testing.T) {
	passphrase := "chap-initiator-secret"
	outboundPassphrase := "chap-target-initiator-secret"
	username := "chap-username"
	outboundUsername := "chap-target-username"

	sampleZapiRequestFormatString := `sending to '10.211.55.19' xml: \n<?xml version="1.0" encoding="UTF-8"?>\n\t\t
<netapp xmlns="http://www.netapp.com/filer/admin\" version="1.21" vfiler="datavserver">\n
<iscsi-initiator-set-default-auth>\n     <auth-type>CHAP</auth-type>\n
<outbound-passphrase>%s</outbound-passphrase>\n
<outbound-user-name>%s</outbound-user-name>\n
<passphrase>%s</passphrase>\n     <user-name>%s</user-name>\n
</iscsi-initiator-set-default-auth>\n          </netapp>`
	sampleZapiRequest := fmt.Sprintf(sampleZapiRequestFormatString, outboundPassphrase, outboundUsername, passphrase,
		username)
	secretFields := []string{"outbound-passphrase", "outbound-user-name", "passphrase", "user-name"}
	requestReplacements := make(map[string]string)
	for _, f := range secretFields {
		fmtString := "<%s>%s</%s>"
		requestReplacements[fmt.Sprintf(fmtString, f, ".*", f)] = fmt.Sprintf(fmtString, f, REDACTED, f)
	}

	expectedRedactedZapiRequest := fmt.Sprintf(sampleZapiRequestFormatString, REDACTED, REDACTED, REDACTED, REDACTED)

	xmlPassphraseString := "<outbound-user-name></outbound-user-name></outbound-user-name>"

	xmlExpected := fmt.Sprintf("<outbound-user-name>%s</outbound-user-name>", REDACTED)
	xmlReplacements := map[string]string{"<outbound-user-name>.*</outbound-user-name>": xmlExpected}

	xmlTagFormatString := "<%s>%s</%s>"
	outboundPassphraseTag := fmt.Sprintf(xmlTagFormatString, "outbound-passphrase", "%s", "outbound-passphrase")

	noRegexPassphrase := "fdsxchj4d@"
	noRegexPassphraseXml := fmt.Sprintf(outboundPassphraseTag, noRegexPassphrase)
	noRegexExpectedXml := fmt.Sprintf(outboundPassphraseTag, REDACTED)
	noRegexReplacements := map[string]string{
		noRegexPassphraseXml: noRegexExpectedXml,
	}

	uncompilableRegexPassphrase := "<?$!abjghjd()^>[</?$!abjghjd()^>"
	uncompilableRegexExpectedString := "regex matching the secret could not compile, so the entire string has been" +
		" redacted"
	uncompilableRegexPassphraseTag := fmt.Sprintf(outboundPassphraseTag, uncompilableRegexPassphrase)
	uncompilableRegexReplacements := map[string]string{
		uncompilableRegexPassphraseTag: "won't be used",
	}

	testCases := []struct {
		description   string
		input         string
		expected      string
		replacements  map[string]string
		useRegex      bool
		assertMessage string
	}{
		{
			"ZAPI request in bug report", sampleZapiRequest, expectedRedactedZapiRequest,
			requestReplacements, true, "expected that the sample usernames" +
				" and passwords from the bug report are redacted in the output string",
		},
		{
			"Passphrase is equal to the closing XML tag", xmlPassphraseString, xmlExpected,
			xmlReplacements, true, "expected that only the passphrase portion of the" +
				" xml string to be redacted when the passphrase is equal to the ending tag",
		},
		{
			"Replacement works properly when a regular expression is not used", noRegexPassphraseXml,
			noRegexExpectedXml, noRegexReplacements, false, "expect only the" +
				" passphrase to be redacted when regex matching is not used",
		},
		{
			"Safe string is returned when regex is invalid", uncompilableRegexPassphrase,
			uncompilableRegexExpectedString, uncompilableRegexReplacements, true,
			"expect the invalid regex error string when the provided regex cannot be compiled",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			actualString := RedactSecretsFromString(tc.input, tc.replacements, tc.useRegex)
			assert.Equal(t, tc.expected, actualString, tc.assertMessage)
		})
	}
}

func TestGetVerifiedBlockFsType(t *testing.T) {
	log.Debug("Running TestGetVerifiedBlockFsType...")

	tests := []struct {
		blockFsType string
		fsType      string
		errNotNil   bool
	}{
		// Positive tests
		{"nfs/ext3", "ext3", false},
		{"nfs/ext4", "ext4", false},
		{"nfs/xfs", "xfs", false},
		{"nfs/raw", "raw", false},

		// Negative tests
		{"abc/ext3", "", true},
		{"ext4/xfs", "", true},
		{"nfs/ext3/ext4", "", true},
		{"ext3", "", true},
		{"nfs/ext99", "", true},
	}

	for _, test := range tests {
		fsType, err := GetVerifiedBlockFsType(test.blockFsType)

		assert.Equal(t, test.fsType, fsType)
		assert.Equal(t, test.errNotNil, err != nil)
	}
}

func TestVerifyFilesystemSupport(t *testing.T) {
	log.Debug("Running TestVerifyFilesystemSupport...")

	tests := []struct {
		fsType       string
		outputFsType string
		errNotNil    bool
	}{
		// Positive tests
		{"ext3", "ext3", false},
		{"ext4", "ext4", false},
		{"xfs", "xfs", false},
		{"raw", "raw", false},

		// Negative tests
		{"ext99", "", true},
		{"nfs/ext3", "", true},
		{"nfs/ext4", "", true},
		{"nfs/xfs", "", true},
		{"nfs/raw", "", true},
		{"abc/ext3", "", true},
		{"ext4/xfs", "", true},
		{"nfs/ext3/ext4", "", true},
		{"nfs/ext99", "", true},
	}

	for _, test := range tests {
		fsType, err := VerifyFilesystemSupport(test.fsType)

		assert.Equal(t, test.outputFsType, fsType)
		assert.Equal(t, test.errNotNil, err != nil)
	}
}

func TestAppendToStringList(t *testing.T) {
	log.Debug("Running TestAppendToStringList...")

	tests := []struct {
		stringList    string
		newItem       string
		sep           string
		newStringList string
	}{
		{"", "xfs", ",", "xfs"},
		{"", "xfs", "/", "xfs"},
		{"", "xfs", " ", "xfs"},
		{"", "xfs", "", "xfs"},
		{"", "", ",", ""},
		{"", "", "/", ""},
		{"", "", " ", ""},
		{"", "", "", ""},
		{"ext3", "xfs", ",", "ext3,xfs"},
		{"ext3", "xfs", "/", "ext3/xfs"},
		{"ext3", "xfs", " ", "ext3 xfs"},
		{"ext3", "xfs", "", "ext3xfs"},
		{"ext3", "xfs,raw", ",", "ext3,xfs,raw"},
		{"ext3", "xfs, raw", "/", "ext3/xfs, raw"},
		{"ext3", "xfs, raw", " ", "ext3 xfs, raw"},
		{"ext3", "xfs, raw", "", "ext3xfs, raw"},
		{"ext3,ext4", "xfs", ",", "ext3,ext4,xfs"},
		{"ext3,ext4", "xfs", "/", "ext3,ext4/xfs"},
		{"ext3,ext4", "xfs", " ", "ext3,ext4 xfs"},
		{"ext3,ext4", "xfs", "", "ext3,ext4xfs"},
		{"ext3, ext4", "xfs", ",", "ext3, ext4,xfs"},
		{"ext3, ext4", "xfs", "/", "ext3, ext4/xfs"},
		{"ext3, ext4", "xfs", " ", "ext3, ext4 xfs"},
		{"ext3, ext4", "xfs", "", "ext3, ext4xfs"},
	}

	for _, test := range tests {
		newStringList := AppendToStringList(test.stringList, test.newItem, test.sep)

		assert.Equal(t, test.newStringList, newStringList)
	}
}

func TestSanitizeMountOptions(t *testing.T) {
	log.Debug("Running TestSanitizeMountOptions...")

	tests := []struct {
		mountOptions          string
		removeMountOptions    []string
		SanitizedMountOptions string
	}{
		{"", []string{"ro"}, ""},
		{"ro", []string{"ro"}, ""},
		{" ro", []string{"ro"}, ""},
		{"rw", []string{"ro"}, "rw"},
		{"ro,nfsvers=3", []string{"ro"}, "nfsvers=3"},
		{"ro, nfsvers=3", []string{"ro"}, "nfsvers=3"},
		{" ro, nfsvers=3", []string{"ro"}, "nfsvers=3"},
		{" ro, nfsvers=3", []string{"bind"}, "ro,nfsvers=3"},
		{"ro,nfsvers=3", []string{"bind"}, "ro,nfsvers=3"},
		{"nouuid,ro,loop,nfsvers=4", []string{"ro"}, "nouuid,loop,nfsvers=4"},
	}

	for _, test := range tests {
		assert.Equal(t, test.SanitizedMountOptions, SanitizeMountOptions(test.mountOptions, test.removeMountOptions))
	}
}

func TestAreMountOptionsInList(t *testing.T) {
	log.Debug("Running TestAreMountOptionsInList...")

	tests := []struct {
		mountOptions string
		optionList   []string
		found        bool
	}{
		{"", []string{"ro"}, false},
		{"ro", []string{"ro"}, true},
		{"rw", []string{"ro"}, false},
		{"ro,nfsvers=3", []string{"ro"}, true},
		{"nouuid,ro,loop,nfsvers=4", []string{"ro"}, true},
		{"nouuid,ro,loop,nfsvers=4", []string{"bind"}, false},
		{"nouuid,ro,loop,bind,nfsvers=4", []string{"bind"}, true},
		{"-o nouuid,ro,loop,bind,nfsvers=4", []string{"bind"}, true},
	}

	for _, test := range tests {
		assert.Equal(t, test.found, AreMountOptionsInList(test.mountOptions, test.optionList))
	}
}
