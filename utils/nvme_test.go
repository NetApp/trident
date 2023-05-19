// Copyright 2023 NetApp, Inc. All Rights Reserved.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractNVMeLIF(t *testing.T) {
	ubuntuAddress := "traddr=10.193.108.74 trsvcid=4420"
	rhelAddress := "traddr=10.193.108.74,trsvcid=4420"

	lif1 := extractNVMeLIF(ubuntuAddress)
	lif2 := extractNVMeLIF(rhelAddress)

	assert.Equal(t, lif1, "10.193.108.74")
	assert.Equal(t, lif2, "10.193.108.74")
}
