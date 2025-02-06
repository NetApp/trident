// Copyright 2025 NetApp, Inc. All Rights Reserved.

package iscsi

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils/models"
)

const (
	sysFsRoot           = "sys"
	sysFsClass          = "class"
	sysFsScsiHost       = "scsi_host"
	sysFsScsiHostDevice = "device"
)

// populateSysFsScsiHost creates the directory at:
// '/sys/class/iscsi_host'.
func populateSysFsScsiHost(t *testing.T, root string, osFs afero.Afero) string {
	t.Helper()

	// Create top-level sysfs scsi host path.
	sysFsScsiHostPath := filepath.Join(root, sysFsRoot, sysFsClass, sysFsScsiHost)
	if err := osFs.MkdirAll(sysFsScsiHostPath, 0x0000); err != nil {
		t.Fatalf("could not fabricate sysfs scsi host")
	}
	info, err := osFs.Stat(sysFsScsiHostPath)
	if err != nil {
		t.Fatalf("could not stat '%s'", sysFsScsiHostPath)
	}
	assert.True(t, info.IsDir())
	return sysFsScsiHostPath
}

// populateSysFsScsiHostNumber creates the directory at:
// '/sys/class/iscsi_host/host#'.
func populateSysFsScsiHostNumber(t *testing.T, path string, osFs afero.Afero) string {
	t.Helper()

	// Create 1 host# dir.
	hostPathDir := "host0"
	scsiHostPath := filepath.Join(path, hostPathDir)
	if err := osFs.Mkdir(scsiHostPath, 0x0000); err != nil {
		t.Fatalf("could not fabricate sysfs scsi host host# directory")
	}
	info, err := osFs.Stat(scsiHostPath)
	if err != nil {
		t.Fatalf("could not stat '%s'", scsiHostPath)
	}
	assert.True(t, info.IsDir())
	return scsiHostPath
}

// populateSysFsScsiHostDevice creates the directory at:
// '/sys/class/iscsi_host/host#/device'.
func populateSysFsScsiHostDevice(t *testing.T, path string, osFs afero.Afero) string {
	t.Helper()

	// Create a file
	hostPathFile := "not-a-dir"
	hostPathFilePath := filepath.Join(path, hostPathFile)
	if _, err := osFs.Create(hostPathFilePath); err != nil {
		t.Fatalf("could not fabricate sysfs scsi host file")
	}
	info, err := osFs.Stat(hostPathFilePath)
	if err != nil {
		t.Fatalf("could not stat '%s'", hostPathFilePath)
	}
	assert.False(t, info.IsDir())

	// Create 1 host#/device dir.
	scsiHostDevicePath := filepath.Join(path, sysFsScsiHostDevice)
	if err := osFs.Mkdir(scsiHostDevicePath, 0x0000); err != nil {
		t.Fatalf("could not fabricate sysfs scsi host host# directory")
	}
	info, err = osFs.Stat(scsiHostDevicePath)
	if err != nil {
		t.Fatalf("could not stat '%s'", scsiHostDevicePath)
	}
	assert.True(t, info.IsDir())
	return scsiHostDevicePath
}

// populateSysFsScsiHostSession creates the directory at:
// '/sys/class/iscsi_host/host#/device/session#/iscsi_session/session#'.
func populateSysFsScsiHostSession(t *testing.T, path string, osFs afero.Afero) string {
	t.Helper()

	// Create 1 host#/device/session# dir.
	hostSessionDir := "session0"
	hostSessionPath := filepath.Join(path, hostSessionDir, "iscsi_session", hostSessionDir)
	// hostSessionPath := filepath.Join(scsiHostDevicePath, hostSessionDir, "iscsi_session", hostSessionDir)
	if err := osFs.MkdirAll(hostSessionPath, 0x0000); err != nil {
		t.Fatalf("could not fabricate sysfs scsi host host# device session# directory")
	}
	info, err := osFs.Stat(hostSessionPath)
	if err != nil {
		t.Fatalf("could not stat '%s'", hostSessionPath)
	}
	assert.True(t, info.IsDir())
	return hostSessionPath
}

// populateSysFsScsiHostSessionTargetName creates the file and targetname entry at:
// '/sys/class/iscsi_host/host#/device/session#/iscsi_session/session#/targetname'.
func populateSysFsScsiHostSessionTargetName(t *testing.T, path string, osFs afero.Afero, target string) string {
	t.Helper()

	// Create a targetname file: '/sys/class/iscsi_host/host#/device/session#/iscsi_session/session#/targetname'
	// target := "iqn.1992-08.com.netapp:sn.any:vs.01"
	targetNameFile := "targetname"
	targetNameFilePath := filepath.Join(path, targetNameFile)
	if _, err := osFs.Create(targetNameFilePath); err != nil {
		t.Fatalf("could not fabricate sysfs scsi host file")
	}
	info, err := osFs.Stat(targetNameFilePath)
	if err != nil {
		t.Fatalf("could not stat '%s'", targetNameFilePath)
	}
	assert.False(t, info.IsDir())

	// Write targetIQN to this.
	if err := osFs.WriteFile(targetNameFilePath, []byte(target), 0x0000); err != nil {
		t.Fatalf(err.Error())
	}
	contents, err := osFs.ReadFile(targetNameFilePath)
	if err != nil {
		t.Fatalf(err.Error())
	}
	assert.Equal(t, target, string(contents))
	return targetNameFilePath
}

// populateSysFsScsiHostSessionDevice creates the parent dir and entries under:
// '/sys/class/iscsi_host/host#/device/session#/iscsi_session/session#/device'.
func populateSysFsScsiHostSessionDevice(t *testing.T, path string, osFs afero.Afero, targetAddress string) string {
	t.Helper()

	deviceDirName := "device"
	deviceDirPath := filepath.Join(path, deviceDirName)
	if err := osFs.Mkdir(deviceDirPath, 0x0000); err != nil {
		t.Fatalf("could not fabricate sysfs scsi host file")
	}
	info, err := osFs.Stat(deviceDirPath)
	if err != nil {
		t.Fatalf("could not stat '%s'", deviceDirPath)
	}
	assert.True(t, info.IsDir())

	// Create a target<h:c:t> dir.
	targetDeviceDir := filepath.Join(deviceDirPath, targetAddress)
	if err := osFs.Mkdir(targetDeviceDir, 0x0000); err != nil {
		t.Fatalf("could not fabricate sysfs scsi host file")
	}
	info, err = osFs.Stat(targetDeviceDir)
	if err != nil {
		t.Fatalf("could not stat '%s'", targetDeviceDir)
	}
	assert.True(t, info.IsDir())

	// Create a file for nop case.
	deviceFileName := "not-a-dir"
	deviceFilePath := filepath.Join(deviceDirPath, deviceFileName)
	if _, err := osFs.Create(deviceFilePath); err != nil {
		t.Fatalf("could not fabricate sysfs scsi host file")
	}
	info, err = osFs.Stat(deviceFilePath)
	if err != nil {
		t.Fatalf("could not stat '%s'", deviceFilePath)
	}
	assert.False(t, info.IsDir())

	return deviceDirPath
}

func TestIscsiReconcileHelper_DiscoverSCSIAddressMapForTarget(t *testing.T) {
	memoryFs := afero.NewMemMapFs()
	osFs := afero.Afero{Fs: memoryFs}
	chrootPathPrefix := "chroot"
	target := "iqn.1992-08.com.netapp:sn.any:vs.01"
	targetAddress := "target8:0:0"

	scsiHostPath := populateSysFsScsiHost(t, chrootPathPrefix, osFs)
	scsiHostNumberPath := populateSysFsScsiHostNumber(t, scsiHostPath, osFs)
	scsiHostDevicePath := populateSysFsScsiHostDevice(t, scsiHostNumberPath, osFs)
	scsiHostSessionPath := populateSysFsScsiHostSession(t, scsiHostDevicePath, osFs)
	_ = populateSysFsScsiHostSessionTargetName(t, scsiHostSessionPath, osFs, target)
	_ = populateSysFsScsiHostSessionDevice(t, scsiHostSessionPath, osFs, targetAddress)

	helper := NewReconcileUtilsDetailed(chrootPathPrefix, nil, osFs)
	addressMap, err := helper.DiscoverSCSIAddressMapForTarget(context.Background(), target)
	assert.NoError(t, err)
	assert.NotNil(t, addressMap)
	assert.Len(t, addressMap, 1)
	var discoveredAddress models.ScsiDeviceAddress
	for _, addr := range addressMap {
		discoveredAddress = addr
	}
	assert.Contains(t, targetAddress, discoveredAddress.Host)
	assert.Contains(t, targetAddress, discoveredAddress.Channel)
	assert.Contains(t, targetAddress, discoveredAddress.Target)
}
