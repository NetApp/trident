// Copyright 2024 NetApp, Inc. All Rights Reserved.

package luks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewLUKSDevice(t *testing.T) {
	luksDevice := NewLUKSDevice("/dev/sdb", "pvc-test", nil)
	assert.Equal(t, luksDevice.RawDevicePath(), "/dev/sdb")
	assert.Equal(t, luksDevice.MappedDevicePath(), "/dev/mapper/luks-pvc-test")
	assert.Equal(t, luksDevice.MappedDeviceName(), "luks-pvc-test")
}
