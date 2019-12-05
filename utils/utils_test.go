// Copyright 2018 NetApp, Inc. All Rights Reserved.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	log "github.com/sirupsen/logrus"
)

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

	var volSizeTests = []struct {
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

	slice := []string{
		"foo",
		"bar",
	}

	if !SliceContainsString(slice, "foo") {
		t.Errorf("Slice SHOULD contain string %v", "foo")
	}
	if !SliceContainsString(slice, "bar") {
		t.Errorf("Slice SHOULD contain string %v", "bar")
	}
	if SliceContainsString(slice, "baz") {
		t.Errorf("Slice should NOT contain string %v", "baz")
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
