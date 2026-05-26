// Copyright 2026 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
)

func TestVerifyFilesystemSupport(t *testing.T) {
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

func TestDetermineFSType(t *testing.T) {
	fsVolumeMode := string(config.Filesystem)
	blockVolumeMode := string(config.RawBlock)

	tests := []struct {
		name            string
		publishInfoType string
		existingType    string
		volumeMode      string
		expectedFSType  string
		errNotNil       bool
	}{
		// publishInfoType takes precedence regardless of other inputs.
		{
			name:            "publishInfoType provided — returned directly",
			publishInfoType: Xfs,
			existingType:    "",
			volumeMode:      fsVolumeMode,
			expectedFSType:  Xfs,
		},
		{
			name:            "publishInfoType overrides existing on-disk type",
			publishInfoType: Ext4,
			existingType:    Xfs,
			volumeMode:      fsVolumeMode,
			expectedFSType:  Ext4,
		},

		// No publishInfoType — adopt the known on-disk type.
		{
			name:           "existing xfs adopted when publishInfoType absent",
			existingType:   Xfs,
			volumeMode:     fsVolumeMode,
			expectedFSType: Xfs,
		},
		{
			name:           "existing ext3 adopted when publishInfoType absent",
			existingType:   Ext3,
			volumeMode:     fsVolumeMode,
			expectedFSType: Ext3,
		},

		// No publishInfoType, no usable existing type — fall back by volumeMode.
		{
			name:           "filesystem volume with empty existing type defaults to ext4",
			existingType:   "",
			volumeMode:     fsVolumeMode,
			expectedFSType: Ext4,
		},
		{
			name:           "raw-block volume with empty existing type defaults to raw",
			existingType:   "",
			volumeMode:     blockVolumeMode,
			expectedFSType: Raw,
		},
		{
			name:           "raw-block volume with unknown existing type defaults to raw",
			existingType:   UnknownFstype,
			volumeMode:     blockVolumeMode,
			expectedFSType: Raw,
		},

		// Filesystem volume with an unknown on-disk type: default to ext4 but surface an error.
		{
			name:           "filesystem volume with unknown existing type returns ext4 and error",
			existingType:   UnknownFstype,
			volumeMode:     fsVolumeMode,
			expectedFSType: "",
			errNotNil:      true,
		},

		// No publishInfoType, no matching volumeMode — nothing to fall back to.
		{
			name:           "empty inputs yield empty fsType and no error",
			existingType:   "",
			volumeMode:     "",
			expectedFSType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsType, err := DetermineFSType(tt.publishInfoType, tt.existingType, tt.volumeMode)
			assert.Equal(t, tt.expectedFSType, fsType)
			assert.Equal(t, tt.errNotNil, err != nil)
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
