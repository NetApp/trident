// Copyright 2021 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImages(t *testing.T) {
	unsupportedVersion := "1.9.0"
	invalidVersion := "1.10.12.2240"
	supportedVersion := "1.19.0"

	K8sVersion = unsupportedVersion
	assert.Error(t, listImages(), "Unsupported version %s should return an error.", K8sVersion)

	K8sVersion = invalidVersion
	assert.Error(t, listImages(), "Invalid version %s should return an error.", K8sVersion)

	K8sVersion = supportedVersion
	assert.NoError(t, listImages(), "Supported version %s should not return an error.", K8sVersion)
}
