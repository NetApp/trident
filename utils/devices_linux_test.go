// Copyright 2022 NetApp, Inc. All Rights Reserved.

//go:build linux

package utils

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

func TestLUKSDeviceStruct_Positive(t *testing.T) {
	execCmd = fakeExecCommand
	// Reset exec command after tests
	defer func() {
		execCmd = exec.CommandContext
	}()
	execReturnValue = ""
	execReturnCode = 0

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test getters
	luksDevice := LUKSDevice{luksDeviceName: "pvc-test", devicePath: "/dev/sdb"}

	assert.Equal(t, "/dev/mapper/pvc-test", luksDevice.LUKSDevicePath())
	assert.Equal(t, "/dev/sdb", luksDevice.DevicePath())

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: device is already luks formatted
	execReturnValue = ""
	execReturnCode = 0
	// Return code of 0 means it is formatted
	isFormatted, err := luksDevice.IsLUKSFormatted(context.Background())
	assert.NoError(t, err)
	assert.True(t, isFormatted)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: device is not LUKS formatted
	execReturnValue = ""
	execReturnCode = 1
	// Return code of 0 means it is formatted
	isFormatted, err = luksDevice.IsLUKSFormatted(context.Background())
	assert.NoError(t, err)
	assert.False(t, isFormatted)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: format device with LUKS
	execReturnValue = ""
	execReturnCode = 0
	// Return code of 0 means it is formatted
	err = luksDevice.LUKSFormat(context.Background(), "passphrase")
	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: open LUKS device
	execReturnValue = ""
	execReturnCode = 0
	// Return code of 0 means it opened
	err = luksDevice.Open(context.Background(), "passphrase")
	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: LUKS device is open
	execReturnValue = ""
	execReturnCode = 0
	// Return code of 0 means it opened
	isOpen, err := luksDevice.IsOpen(context.Background())
	assert.NoError(t, err)
	assert.True(t, isOpen)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: close LUKS device
	execReturnValue = ""
	execReturnCode = 0
	// Return code of 0 means it opened
	err = luksDevice.Close(context.Background())
	assert.NoError(t, err)
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Negative cases: Test ExitError (bad command) on running command for all LUKSDevice methods
func TestLUKSDeviceStruct_Negative_ExitError(t *testing.T) {
	execCmd = fakeExecCommandExitError
	// Reset exec command after tests
	defer func() {
		execCmd = exec.CommandContext
	}()

	luksDevice := LUKSDevice{luksDeviceName: "pvc-test", devicePath: "/dev/sdb"}

	isFormatted, err := luksDevice.IsLUKSFormatted(context.Background())
	assert.Error(t, err)
	assert.False(t, isFormatted)

	err = luksDevice.LUKSFormat(context.Background(), "passphrase")
	assert.Error(t, err)

	err = luksDevice.Open(context.Background(), "passphrase")
	assert.Error(t, err)

	isOpen, err := luksDevice.IsOpen(context.Background())
	assert.Error(t, err)
	assert.False(t, isOpen)

	err = luksDevice.Close(context.Background())
	assert.Error(t, err)
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Negative cases: Test non-zero exit code on running command for all LUKSDevice methods
func TestLUKSDeviceStruct_Negative_ExitCode1(t *testing.T) {
	execCmd = fakeExecCommand
	execReturnCode = 1
	// Reset exec command after tests
	defer func() {
		execCmd = exec.CommandContext
	}()

	luksDevice := LUKSDevice{luksDeviceName: "pvc-test", devicePath: "/dev/sdb"}

	isFormatted, err := luksDevice.IsLUKSFormatted(context.Background())
	assert.NoError(t, err)
	assert.False(t, isFormatted)

	err = luksDevice.LUKSFormat(context.Background(), "passphrase")
	assert.Error(t, err)

	err = luksDevice.Open(context.Background(), "passphrase")
	assert.Error(t, err)

	isOpen, err := luksDevice.IsOpen(context.Background())
	assert.NoError(t, err)
	assert.False(t, isOpen)

	err = luksDevice.Close(context.Background())
	assert.Error(t, err)
}

func TestEnsureLUKSDevice(t *testing.T) {
	execCmd = fakeExecCommand
	// Reset exec command after tests
	defer func() {
		execCmd = exec.CommandContext
	}()
	execReturnValue = ""
	execReturnCode = 0
	luksFormatted, luksDevicePath, err := EnsureLUKSDevice(context.Background(), "/dev/sda", "pvc-123", "mysecretlukspassphrase")
	assert.Nil(t, err)
	assert.Equal(t, "/dev/mapper/luks-pvc-123", luksDevicePath)
	assert.Equal(t, false, luksFormatted)
}

/*
func TestEnsureLUKSDevice_Positive(t *testing.T) {
	// Reset exec command after tests
	defer func() {
		execCmd = exec.CommandContext
	}()
	fakePassphrase := "mysecretlukspassphrase"
	mockCtrl := gomock.NewController(t)
	mockLUKSDevice := mockutils.NewMockLUKSDeviceInterface(mockCtrl)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test LUKS device already open
	mockLUKSDevice.EXPECT().IsOpen(gomock.Any()).Return(true, nil).Times(1)
	mockLUKSDevice.EXPECT().LUKSDevicePath().Return("/dev/mapper/luks-pvc-123").Times(1)

	luksFormatted, luksDevicePath, err := ensureLUKSDevice(context.Background(), mockLUKSDevice, fakePassphrase)
	assert.Nil(t, err)
	assert.Equal(t, "/dev/mapper/luks-pvc-123", luksDevicePath)
	assert.Equal(t, false, luksFormatted)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test device is luks but not open
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mockutils.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().IsOpen(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(gomock.Any()).Return(true, nil).Times(1)
	mockLUKSDevice.EXPECT().Open(gomock.Any(), fakePassphrase).Return(nil).Times(1)
	mockLUKSDevice.EXPECT().LUKSDevicePath().Return("/dev/mapper/luks-pvc-123").Times(1)
	execCmd = fakeExecCommandPaddedOutput
	fakeData := ""
	execReturnValue = string(fakeData)
	execReturnCode = 0
	execPadding = 2097152

	luksFormatted, _, err = ensureLUKSDevice(context.Background(), mockLUKSDevice, fakePassphrase)
	assert.Nil(t, err)
	assert.Equal(t, false, luksFormatted)

	execCmd = exec.CommandContext
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test device already has data
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mockutils.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().IsOpen(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().DevicePath().Return("/dev/sdb").Times(1)
	execCmd = fakeExecCommandPaddedOutput
	// set non-zero bytes
	execReturnValue = "a"
	execReturnCode = 0
	execPadding = 2097152

	luksFormatted, _, err = ensureLUKSDevice(context.Background(), mockLUKSDevice, fakePassphrase)
	assert.NotNil(t, err)
	assert.Equal(t, false, luksFormatted)

	execCmd = exec.CommandContext
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test device too small
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mockutils.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().IsOpen(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().DevicePath().Return("/dev/sdb").Times(1)
	execCmd = fakeExecCommandPaddedOutput
	// set non-zero bytes
	execReturnValue = "a"
	execReturnCode = 0
	execPadding = 2097

	luksFormatted, _, err = ensureLUKSDevice(context.Background(), mockLUKSDevice, fakePassphrase)
	assert.NotNil(t, err)
	assert.Equal(t, false, luksFormatted)

	execCmd = exec.CommandContext
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test device is empty
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mockutils.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().IsOpen(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().DevicePath().Return("/dev/sdb").Times(1)
	mockLUKSDevice.EXPECT().LUKSFormat(gomock.Any(), fakePassphrase).Return(nil).Times(1)
	mockLUKSDevice.EXPECT().Open(gomock.Any(), fakePassphrase).Return(nil).Times(1)
	mockLUKSDevice.EXPECT().LUKSDevicePath().Return("/dev/mapper/luks-pvc-123").Times(1)
	execCmd = fakeExecCommandPaddedOutput
	execReturnValue = ""
	execReturnCode = 0
	execPadding = 2097152

	luksFormatted, _, err = ensureLUKSDevice(context.Background(), mockLUKSDevice, fakePassphrase)
	t.Logf("%v", err)
	assert.Nil(t, err)
	assert.Equal(t, true, luksFormatted)

	execCmd = exec.CommandContext
	mockCtrl.Finish()
}

func TestEnsureLUKSDevice_Negative(t *testing.T) {
	// Reset exec command after tests
	defer func() {
		execCmd = exec.CommandContext
	}()
	fakePassphrase := "mysecretlukspassphrase"
	mockCtrl := gomock.NewController(t)
	mockLUKSDevice := mockutils.NewMockLUKSDeviceInterface(mockCtrl)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test device already has data
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mockutils.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().IsOpen(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().DevicePath().Return("/dev/sdb").Times(1)
	execCmd = fakeExecCommandPaddedOutput
	// set non-zero bytes
	execReturnValue = "a"
	execReturnCode = 0
	execPadding = 2097152

	luksFormatted, _, err := ensureLUKSDevice(context.Background(), mockLUKSDevice, fakePassphrase)
	assert.NotNil(t, err)
	assert.Equal(t, false, luksFormatted)

	execCmd = exec.CommandContext
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Open with incorrect passphrase
	fakeError := fmt.Errorf("wrong passphrase")
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mockutils.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().IsOpen(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(gomock.Any()).Return(true, nil).Times(1)
	mockLUKSDevice.EXPECT().Open(gomock.Any(), fakePassphrase).Return(fakeError).Times(1)

	luksFormatted, _, err = ensureLUKSDevice(context.Background(), mockLUKSDevice, fakePassphrase)
	assert.Error(t, err)
	assert.Equal(t, false, luksFormatted)

	execCmd = exec.CommandContext
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Cannot check if device is already open
	fakeError = fmt.Errorf("error")
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mockutils.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().IsOpen(gomock.Any()).Return(false, fakeError).Times(1)

	luksFormatted, _, err = ensureLUKSDevice(context.Background(), mockLUKSDevice, fakePassphrase)
	assert.Error(t, err)
	assert.Equal(t, false, luksFormatted)

	execCmd = exec.CommandContext
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Cannot check if device is already luks formatted
	fakeError = fmt.Errorf("error")
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mockutils.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().IsOpen(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(gomock.Any()).Return(true, fakeError).Times(1)

	luksFormatted, _, err = ensureLUKSDevice(context.Background(), mockLUKSDevice, fakePassphrase)
	assert.Error(t, err)
	assert.Equal(t, false, luksFormatted)

	execCmd = exec.CommandContext
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Cannot check if device is formatted
	fakeError = fmt.Errorf("error")
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mockutils.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().IsOpen(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().DevicePath().Return("/dev/sdb").Times(1)
	execCmd = fakeExecCommandPaddedOutput
	// set non-zero bytes
	execReturnValue = "fake error"
	execReturnCode = 1

	luksFormatted, _, err = ensureLUKSDevice(context.Background(), mockLUKSDevice, fakePassphrase)
	assert.Error(t, err)
	assert.Equal(t, false, luksFormatted)

	execCmd = exec.CommandContext
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Fail to LUKS format device
	fakeError = fmt.Errorf("error")
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mockutils.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().IsOpen(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(gomock.Any()).Return(false, nil).Times(1)
	mockLUKSDevice.EXPECT().DevicePath().Return("/dev/sdb").Times(1)
	mockLUKSDevice.EXPECT().LUKSFormat(gomock.Any(), fakePassphrase).Return(fakeError).Times(1)
	execCmd = fakeExecCommandPaddedOutput
	// set non-zero bytes
	execReturnValue = ""
	execReturnCode = 0
	execPadding = 2097152

	luksFormatted, _, err = ensureLUKSDevice(context.Background(), mockLUKSDevice, fakePassphrase)
	assert.Error(t, err)
	assert.Equal(t, false, luksFormatted)

	execCmd = exec.CommandContext
	mockCtrl.Finish()
}
*/

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Negative cases: Test ExitError on running command for all LUKSDevice methods
func TestEnsureLUKSDeviceClosed_Negative(t *testing.T) {
	execCmd = fakeExecCommand
	execReturnValue = ""
	execReturnCode = 0

	// Reset exec command and osFs after tests
	defer func() {
		execCmd = exec.CommandContext
		osFs = afero.NewOsFs()
	}()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test device does not exist
	// Use a mem map fs to ensure the file does not exist
	osFs = afero.NewMemMapFs()
	err := EnsureLUKSDeviceClosed(context.Background(), "/dev/mapper/luks-test-dev")
	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test failure to stat file (filename too long)
	// afero MemMapFs does normalization of the filename, so we need to actually use osFs here
	osFs = afero.NewOsFs()
	var b strings.Builder
	b.Grow(1025)
	for i := 0; i < 1025; i++ {
		b.WriteByte('a')
	}
	s := b.String()
	err = EnsureLUKSDeviceClosed(context.Background(), "/dev/mapper/"+s)
	assert.Error(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test luksClose works
	osFs = afero.NewMemMapFs()
	osFs.Create("/dev/mapper/luks-test-dev")
	err = EnsureLUKSDeviceClosed(context.Background(), "/dev/mapper/luks-test-dev")
	assert.NoError(t, err)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test luksClose fails
	osFs = afero.NewMemMapFs()
	osFs.Create("/dev/mapper/luks-test-dev")
	execReturnValue = "error"
	execReturnCode = 1
	err = EnsureLUKSDeviceClosed(context.Background(), "/dev/mapper/luks-test-dev")
	assert.Error(t, err)
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
// Negative cases: Test exit code 2 on running command for Open
func TestLUKSDeviceStruct_Open_BadPassphrase(t *testing.T) {
	execCmd = fakeExecCommand
	execReturnCode = 2
	// Reset exec command after tests
	defer func() {
		execCmd = exec.CommandContext
	}()

	luksDevice := LUKSDevice{luksDeviceName: "pvc-test", devicePath: "/dev/sdb"}
	err := luksDevice.Open(context.Background(), "passphrase")
	assert.Error(t, err)
}
