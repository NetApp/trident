// Copyright 2024 NetApp, Inc. All Rights Reserved.

package yum_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/internal/nodeprep/packagemanager/yum"
)

func TestNew(t *testing.T) {
	yumPackageManager := yum.New()
	assert.NotNil(t, yumPackageManager)
}

func TestYum_MultipathToolsInstalled(t *testing.T) {
	// TODO implement this test
	yumPackageManager := yum.New()
	assert.True(t, yumPackageManager.MultipathToolsInstalled())
}
