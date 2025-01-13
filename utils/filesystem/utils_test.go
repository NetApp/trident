// Copyright 2025 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
