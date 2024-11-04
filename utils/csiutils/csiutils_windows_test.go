// Copyright 2022 NetApp, Inc. All Rights Reserved.

package csiutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeWindowsPath(t *testing.T) {
	path := "\\test\\volume\\path"

	result := normalizeWindowsPath(path)
	assert.Equal(t, result, "c:\\test\\volume\\path", "path mismatch")
}
