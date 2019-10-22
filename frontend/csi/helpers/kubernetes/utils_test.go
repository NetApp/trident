// Copyright 2019 NetApp, Inc. All Rights Reserved.
package kubernetes

import (
	"testing"

	"k8s.io/apimachinery/pkg/version"

	"github.com/netapp/trident/frontend/csi"
	"github.com/netapp/trident/testutils"
)

func TestSupportsFeature(t *testing.T) {

	var supportedTests = []struct {
		versionInfo version.Info
		expected    bool
	}{
		{version.Info{GitVersion: "v1.14.3"}, false},
		{version.Info{GitVersion: "v1.16.2"}, true},
		{version.Info{GitVersion: "garbage"}, false},
	}

	for _, tc := range supportedTests {
		plugin := Plugin{kubeVersion: &tc.versionInfo}
		supported := plugin.SupportsFeature(csi.ExpandCSIVolumes)
		if tc.expected {
			print("bluh\n")
			testutils.AssertTrue(t, "Expected true", supported)
		} else {
			print("bar\n")
			testutils.AssertFalse(t, "Expected false", supported)
		}
	}
}
