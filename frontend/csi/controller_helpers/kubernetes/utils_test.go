// Copyright 2022 NetApp, Inc. All Rights Reserved.
package kubernetes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/version"

	"github.com/netapp/trident/frontend/csi"
)

func TestSupportsFeature(t *testing.T) {
	supportedTests := []struct {
		versionInfo version.Info
		expected    bool
	}{
		{version.Info{GitVersion: "v1.14.3"}, false},
		{version.Info{GitVersion: "v1.16.2"}, true},
		{version.Info{GitVersion: "garbage"}, false},
	}

	for _, tc := range supportedTests {
		plugin := helper{kubeVersion: &tc.versionInfo}
		supported := plugin.SupportsFeature(context.Background(), csi.ExpandCSIVolumes)
		if tc.expected {
			assert.True(t, supported, "Expected true")
		} else {
			assert.False(t, supported, "Expected false")
		}
	}
}

func TestGetDataSizeFromTotalSize(t *testing.T) {
	var totalSize uint64 = 100
	var snapshotReservePercent int = 20
	var expectedDataSize uint64 = 80

	h := helper{}
	result := h.getDataSizeFromTotalSize(context.Background(), totalSize, snapshotReservePercent)

	assert.Equal(t, expectedDataSize, result, "Data size not as expected")
}
