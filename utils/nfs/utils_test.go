// Copyright 2025 NetApp, Inc. All Rights Reserved.

package nfs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNFSVersionFromMountOptions(t *testing.T) {
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
		{"", "", supportedVersions, "", false},
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
