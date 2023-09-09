// Copyright 2022 NetApp, Inc. All Rights Reserved.

//go:build linux

package utils

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	mockluks "github.com/netapp/trident/mocks/mock_utils/mock_luks"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
)

var luksError = fmt.Errorf("luks error")

func mockCryptsetupIsLuks(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", luksCommandTimeout, true, "", "isLuks", gomock.Any(),
	)
}

func mockCryptsetupLuksFormat(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", luksCommandTimeout, true, gomock.Any(),
		"luksFormat", gomock.Any(), "--type", "luks2", "-c", "aes-xts-plain64",
	)
}

func mockCryptsetupLuksStatus(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", luksCommandTimeout, true, "", "status", gomock.Any(),
	)
}

func mockCryptsetupLuksOpen(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", luksCommandTimeout, true, gomock.Any(),
		"open", gomock.Any(), gomock.Any(), "--type", "luks2",
	)
}

func mockCryptsetupLuksClose(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", luksCommandTimeout, true, "", "luksClose", gomock.Any(),
	)
}

// mockDiscDump mocks out the "dd" binary executed via command interface. See devices.go#isDeviceUnformatted.
func mockDiscDump(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeout(
		gomock.Any(), "dd", deviceOperationsTimeout, false,
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	)
}

func mockCryptsetupLuksStatusWithDevicePath(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", luksCommandTimeout, true, "", "status", gomock.Any(),
	)
}

func mockCryptsetupLuksTestPassphrase(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", luksCommandTimeout, true, gomock.Any(), "open", gomock.Any(),
		gomock.Any(), "--type", "luks2", "--test-passphrase",
	)
}

func mockCryptsetupLuksChangeKey(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", luksCommandTimeout, true, gomock.Any(),
		"luksChangeKey", "-d", gomock.Any(), gomock.Any(),
	)
}

func mockCryptsetupLuksResize(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", luksCommandTimeout, true,
		gomock.Any(), "resize", gomock.Any(),
	)
}

func TestLUKSDevice_IsLUKSFormatted(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := LUKSDevice{mappingName: "pvc-test", rawDevicePath: "/dev/sdb"}
	assert.Equal(t, "/dev/mapper/pvc-test", luksDevice.MappedDevicePath())
	assert.Equal(t, "/dev/sdb", luksDevice.RawDevicePath())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupIsLuks(mockCommand).Return([]byte(""), nil)
	command = mockCommand

	isFormatted, err := luksDevice.IsLUKSFormatted(context.Background())
	assert.NoError(t, err)
	assert.True(t, isFormatted)
}

func TestLUKSDevice_LUKSFormat(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := LUKSDevice{mappingName: "pvc-test", rawDevicePath: "/dev/sdb"}
	assert.Equal(t, "/dev/mapper/pvc-test", luksDevice.MappedDevicePath())
	assert.Equal(t, "/dev/sdb", luksDevice.RawDevicePath())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksFormat(mockCommand).Return([]byte(""), nil)
	command = mockCommand

	err := luksDevice.LUKSFormat(context.Background(), "passphrase")
	assert.NoError(t, err)
}

func TestLUKSDevice_Open(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := LUKSDevice{mappingName: "pvc-test", rawDevicePath: "/dev/sdb"}
	assert.Equal(t, "/dev/mapper/pvc-test", luksDevice.MappedDevicePath())
	assert.Equal(t, "/dev/sdb", luksDevice.RawDevicePath())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), nil)
	command = mockCommand

	err := luksDevice.Open(context.Background(), "passphrase")
	assert.NoError(t, err)
}

func TestLUKSDevice_IsOpen(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := LUKSDevice{mappingName: "pvc-test", rawDevicePath: "/dev/sdb"}
	assert.Equal(t, "/dev/mapper/pvc-test", luksDevice.MappedDevicePath())
	assert.Equal(t, "/dev/sdb", luksDevice.RawDevicePath())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksStatus(mockCommand).Return([]byte(""), nil)
	command = mockCommand

	isOpen, err := luksDevice.IsOpen(context.Background())
	assert.NoError(t, err)
	assert.True(t, isOpen)
}

func TestLUKSDevice_Close(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := LUKSDevice{mappingName: "pvc-test", rawDevicePath: "/dev/sdb"}
	assert.Equal(t, "/dev/mapper/pvc-test", luksDevice.MappedDevicePath())
	assert.Equal(t, "/dev/sdb", luksDevice.RawDevicePath())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksClose(mockCommand).Return([]byte(""), nil)
	command = mockCommand

	err := luksDevice.Close(context.Background())
	assert.NoError(t, err)
}

func TestLUKSDevice_MissingDevicePath(t *testing.T) {
	luksDevice := LUKSDevice{mappingName: "pvc-test", rawDevicePath: ""}
	isFormatted, err := luksDevice.IsLUKSFormatted(context.Background())
	assert.Error(t, err)
	assert.False(t, isFormatted)

	err = luksDevice.LUKSFormat(context.Background(), "passphrase")
	assert.Error(t, err)

	err = luksDevice.Open(context.Background(), "passphrase")
	assert.Error(t, err)
}

func TestLUKSDevice_ExecErrors(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := LUKSDevice{mappingName: "pvc-test", rawDevicePath: "/dev/sdb"}

	// Setup mock calls and reassign any clients to their mock counterparts.
	gomock.InOrder(
		mockCryptsetupIsLuks(mockCommand).Return([]byte(""), luksError),
		mockCryptsetupLuksFormat(mockCommand).Return([]byte(""), luksError),
		mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), luksError),
		mockCryptsetupLuksStatus(mockCommand).Return([]byte(""), luksError),
		mockCryptsetupLuksClose(mockCommand).Return([]byte(""), luksError),
	)
	command = mockCommand

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

func TestEnsureLUKSDevice_FailsWithExecError(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockLUKSDevice := mockluks.NewMockLUKSDeviceInterface(mockCtrl)

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockLUKSDevice.EXPECT().IsOpen(ctx).Return(false, luksError)

	luksFormatted, err := ensureLUKSDevice(context.Background(), mockLUKSDevice, "mysecretlukspassphrase")
	assert.Error(t, err)
	assert.Equal(t, false, luksFormatted)
}

func TestEnsureLUKSDevice_IsOpen(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockLUKSDevice := mockluks.NewMockLUKSDeviceInterface(mockCtrl)

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockLUKSDevice.EXPECT().IsOpen(ctx).Return(true, nil)

	luksFormatted, err := ensureLUKSDevice(context.Background(), mockLUKSDevice, "mysecretlukspassphrase")
	assert.NoError(t, err)
	assert.Equal(t, false, luksFormatted)
}

func TestEnsureLUKSDevice_FailsCheckingIfFormatted(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockLUKSDevice := mockluks.NewMockLUKSDeviceInterface(mockCtrl)

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockLUKSDevice.EXPECT().IsOpen(ctx).Return(false, nil)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(ctx).Return(false, luksError)

	luksFormatted, err := ensureLUKSDevice(context.Background(), mockLUKSDevice, "mysecretlukspassphrase")
	assert.Error(t, err)
	assert.Equal(t, false, luksFormatted)
}

func TestEnsureLUKSDevice_FailsToFormat(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockLUKSDevice := mockluks.NewMockLUKSDeviceInterface(mockCtrl)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockLUKSDevice.EXPECT().IsOpen(ctx).Return(false, nil)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(ctx).Return(false, nil)
	mockLUKSDevice.EXPECT().RawDevicePath().Return("/dev/")
	mockDiscDump(mockCommand).Return([]byte(""), luksError)
	command = mockCommand

	luksFormatted, err := ensureLUKSDevice(context.Background(), mockLUKSDevice, "mysecretlukspassphrase")
	assert.Error(t, err)
	assert.Equal(t, false, luksFormatted)
}

func TestEnsureLUKSDevice_IsFormatted(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockLUKSDevice := mockluks.NewMockLUKSDeviceInterface(mockCtrl)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockLUKSDevice.EXPECT().IsOpen(ctx).Return(false, nil)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(ctx).Return(false, nil)
	mockLUKSDevice.EXPECT().RawDevicePath().Return("/dev/")
	mockDiscDump(mockCommand).Return([]byte("0"), nil)
	command = mockCommand

	luksFormatted, err := ensureLUKSDevice(context.Background(), mockLUKSDevice, "mysecretlukspassphrase")
	assert.Error(t, err)
	assert.Equal(t, false, luksFormatted)
}

func TestEnsureLUKSDevice_FailsToFormatDeviceForLUKS(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockLUKSDevice := mockluks.NewMockLUKSDeviceInterface(mockCtrl)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	// This is needed for mocking out exec command calls within isDeviceUnformatted.
	luksPassphrase := "mysecretlukspassphrase"
	outSlice := make([]string, 2097152)
	for i := range outSlice {
		outSlice[i] = "\x00"
	}
	out := []byte(strings.Join(outSlice, ""))

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockLUKSDevice.EXPECT().IsOpen(ctx).Return(false, nil)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(ctx).Return(false, nil)
	mockLUKSDevice.EXPECT().RawDevicePath().Return("/dev/")
	mockDiscDump(mockCommand).Return(out, nil)
	command = mockCommand
	mockLUKSDevice.EXPECT().LUKSFormat(ctx, luksPassphrase).Return(luksError)

	luksFormatted, err := ensureLUKSDevice(context.Background(), mockLUKSDevice, luksPassphrase)
	assert.Error(t, err)
	assert.Equal(t, false, luksFormatted)
}

func TestEnsureLUKSDevice_FormatsDeviceForLUKS(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockLUKSDevice := mockluks.NewMockLUKSDeviceInterface(mockCtrl)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	// This is needed for mocking out exec command calls within isDeviceUnformatted.
	luksPassphrase := "mysecretlukspassphrase"
	outSlice := make([]string, 2097152)
	for i := range outSlice {
		outSlice[i] = "\x00"
	}
	out := []byte(strings.Join(outSlice, ""))

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockLUKSDevice.EXPECT().IsOpen(ctx).Return(false, nil)
	mockLUKSDevice.EXPECT().IsLUKSFormatted(ctx).Return(false, nil)
	mockLUKSDevice.EXPECT().RawDevicePath().Return("/dev/")
	mockDiscDump(mockCommand).Return(out, nil)
	mockLUKSDevice.EXPECT().LUKSFormat(ctx, luksPassphrase).Return(nil)
	mockLUKSDevice.EXPECT().Open(ctx, luksPassphrase).Return(nil)
	command = mockCommand

	luksFormatted, err := ensureLUKSDevice(context.Background(), mockLUKSDevice, luksPassphrase)
	assert.NoError(t, err)
	assert.Equal(t, true, luksFormatted)
}

func TestEnsureLUKSDeviceClosed_DeviceDoesNotExist(t *testing.T) {
	defer func() {
		osFs = afero.NewOsFs()
	}()

	osFs = afero.NewMemMapFs()
	err := EnsureLUKSDeviceClosed(context.Background(), "/dev/mapper/luks-test-dev")
	assert.NoError(t, err)
}

func TestEnsureLUKSDeviceClosed_FailsToDetectDevice(t *testing.T) {
	defer func() {
		osFs = afero.NewOsFs()
	}()

	osFs = afero.NewOsFs()
	var b strings.Builder
	b.Grow(1025)
	for i := 0; i < 1025; i++ {
		b.WriteByte('a')
	}
	s := b.String()
	err := EnsureLUKSDeviceClosed(context.Background(), "/dev/mapper/"+s)
	assert.Error(t, err)
}

func TestEnsureLUKSDeviceClosed_FailsToCloseDevice(t *testing.T) {
	defer func() {
		osFs = afero.NewOsFs()
	}()

	ctx := context.Background()
	devicePath := "/dev/mapper/luks-test-dev"
	osFs = afero.NewMemMapFs()
	osFs.Create(devicePath)

	err := EnsureLUKSDeviceClosed(ctx, devicePath)
	assert.Error(t, err)
}

func TestEnsureLUKSDeviceClosed_ClosesDevice(t *testing.T) {
	defer func(previousCommand exec.Command) {
		osFs = afero.NewOsFs()
		command = previousCommand
	}(command)

	ctx := context.Background()
	devicePath := "/dev/mapper/luks-test-dev"
	osFs = afero.NewMemMapFs()
	osFs.Create(devicePath)

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksClose(mockCommand).Return([]byte(""), nil)
	command = mockCommand

	err := EnsureLUKSDeviceClosed(ctx, devicePath)
	assert.NoError(t, err)
}

func TestLUKSDeviceOpen_FailsWithBadPassphrase(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	fakeExitError := exec.NewFakeExitError(2, "") // Exit code of 2 means the passphrase was bad.

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), fakeExitError)
	command = mockCommand

	luksDevice := LUKSDevice{mappingName: "pvc-test", rawDevicePath: "/dev/sdb"}
	err := luksDevice.Open(ctx, "passphrase")
	assert.Error(t, err)
}

func TestLUKSDeviceOpen_MiscError(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	fakeExitError := exec.NewFakeExitError(2, "") // Exit code of 2 means the passphrase was bad.

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), fakeExitError)
	command = mockCommand

	luksDevice := LUKSDevice{mappingName: "pvc-test", rawDevicePath: "/dev/sdb"}
	err := luksDevice.Open(ctx, "passphrase")
	assert.Error(t, err)
}

func TestLUKSDeviceOpen_ExecError(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	fakeExitError := exec.NewFakeExitError(1, "") // Exit code of 1 means the exec command failed.

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), fakeExitError)
	command = mockCommand

	luksDevice := LUKSDevice{mappingName: "pvc-test", rawDevicePath: "/dev/sdb"}
	err := luksDevice.Open(ctx, "passphrase")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "could not open LUKS device; ")
}

func TestRotateLUKSDevicePassphrase_NoError(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksChangeKey(mockCommand).Return([]byte(""), nil)
	command = mockCommand

	luksDevice := &LUKSDevice{"/dev/sdb", luksDeviceName}
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "previous", "newpassphrase")
	assert.NoError(t, err)
}

func TestRotateLUKSDevicePassphrase_OldPassphraseEmpty(t *testing.T) {
	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"
	luksDevice := &LUKSDevice{"/dev/sdb", luksDeviceName}
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "", "newpassphrase")
	assert.Error(t, err)
}

func TestRotateLUKSDevicePassphrase_NewPassphraseEmpty(t *testing.T) {
	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"
	luksDevice := &LUKSDevice{"/dev/sdb", luksDeviceName}
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "oldpassphrase", "")
	assert.Error(t, err)
}

func TestRotateLUKSDevicePassphrase_CommandError(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksChangeKey(mockCommand).Return([]byte(""), luksError)
	command = mockCommand

	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"
	luksDevice := &LUKSDevice{"/dev/sdb", luksDeviceName}
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "previous", "newpassphrase")
	assert.Error(t, err)
}

func TestRotateLUKSDevicePassphrase_NoRawDevicePath(t *testing.T) {
	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"
	luksDevice := &LUKSDevice{"", luksDeviceName}
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "previous", "newpassphrase")
	assert.Error(t, err)
}

func TestGetUnderlyingDevicePathForLUKSDevice_Succeeds(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	execReturnValue = `/dev/mapper/luks-trident_pvc_0c6202cb_be41_46b7_bea9_7f2c5c2c4a41 is active and is in use.
  type:    LUKS2
  cipher:  aes-xts-plain64
  keysize: 512 bits
  key location: keyring
  device:  /dev/mapper/3600a09807770457a795d526950374c76
  sector size:  512
  offset:  32768 sectors
  size:    2064384 sectors
  mode:    read/write`

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksStatusWithDevicePath(mockCommand).Return([]byte(execReturnValue), nil)
	command = mockCommand

	devicePath, err := GetUnderlyingDevicePathForLUKSDevice(ctx, "")
	assert.NoError(t, err)
	assert.Equal(t, "/dev/mapper/3600a09807770457a795d526950374c76", devicePath)
}

func TestGetUnderlyingDevicePathForLUKSDevice_FailsWithBadOutput(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	execReturnValue = `bad output`

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksStatusWithDevicePath(mockCommand).Return([]byte(execReturnValue), nil)
	command = mockCommand

	devicePath, err := GetUnderlyingDevicePathForLUKSDevice(ctx, "")
	assert.Error(t, err)
	assert.Equal(t, "", devicePath)
}

func TestGetUnderlyingDevicePathForLUKSDevice_FailsWithNoLUKSDevice(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	execReturnValue = `/dev/mapper/luks-trident_pvc_0c6202cb_be41_46b7_bea9_7f2c5c2c4a41 is active and is in use.
  type:    LUKS2
  cipher:  aes-xts-plain64
  keysize: 512 bits
  key location: keyring
  device:
  sector size:  512
  offset:  32768 sectors
  size:    2064384 sectors
  mode:    read/write`

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksStatusWithDevicePath(mockCommand).Return([]byte(execReturnValue), nil)
	command = mockCommand

	devicePath, err := GetUnderlyingDevicePathForLUKSDevice(ctx, "")
	assert.Error(t, err)
	assert.Equal(t, "", devicePath)
}

func TestGetUnderlyingDevicePathForLUKSDevice_FailsWithDeviceIncorrectlyFormatted(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	execReturnValue = `/dev/mapper/luks-trident_pvc_0c6202cb_be41_46b7_bea9_7f2c5c2c4a41 is active and is in use.
  type:    LUKS2
  cipher:  aes-xts-plain64
  keysize: 512 bits
  key location: keyring
  device:  /dev/mapper/3600a09807770457a795d526950374c76 <some extra stuff on line!>
  sector size:  512
  offset:  32768 sectors
  size:    2064384 sectors
  mode:    read/write`

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksStatusWithDevicePath(mockCommand).Return([]byte(execReturnValue), nil)
	command = mockCommand

	_, err := GetUnderlyingDevicePathForLUKSDevice(ctx, "")
	assert.Error(t, err)
}

func TestCheckPassphrase_Succeeds(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	mockCryptsetupLuksTestPassphrase(mockCommand).Return([]byte(""), nil)
	command = mockCommand

	luksDevice, err := NewLUKSDevice("", "test-pvc")
	assert.NoError(t, err)

	correct, err := luksDevice.CheckPassphrase(ctx, "passphrase")
	assert.True(t, correct)
	assert.NoError(t, err)
}

func TestCheckPassphrase_FailsWithExecError(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksTestPassphrase(mockCommand).Return([]byte(""), luksError)
	command = mockCommand

	luksDevice, err := NewLUKSDevice("", "test-pvc")
	assert.NoError(t, err)

	correct, err := luksDevice.CheckPassphrase(ctx, "passphrase")
	assert.False(t, correct)
	assert.Error(t, err)
}

func TestCheckPassphrase_DetectsPassphraseIsBad(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	fakeExitError := exec.NewFakeExitError(2, "") // Exit code of 2 means the passphrase was bad.

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksTestPassphrase(mockCommand).Return([]byte(""), fakeExitError)
	command = mockCommand

	luksDevice, err := NewLUKSDevice("", "test-pvc")
	assert.NoError(t, err)

	correct, err := luksDevice.CheckPassphrase(ctx, "passphrase")
	assert.False(t, correct)
	assert.NoError(t, err)
}

func TestNewLUKSDeviceFromMappingPath_Succeeds(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	execReturnValue = `/dev/mapper/luks-pvc-test is active and is in use.
  type:    LUKS2
  cipher:  aes-xts-plain64
  keysize: 512 bits
  key location: keyring
  device:  /dev/sdb
  sector size:  512
  offset:  32768 sectors
  size:    2064384 sectors
  mode:    read/write`

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksStatusWithDevicePath(mockCommand).Return([]byte(execReturnValue), nil)
	command = mockCommand

	luksDevice, err := NewLUKSDeviceFromMappingPath(ctx, "/dev/mapper/luks-pvc-test", "pvc-test")
	assert.NoError(t, err)
	assert.Equal(t, luksDevice.RawDevicePath(), "/dev/sdb")
	assert.Equal(t, luksDevice.MappedDevicePath(), "/dev/mapper/luks-pvc-test")
	assert.Equal(t, luksDevice.MappedDeviceName(), "luks-pvc-test")
}

func TestNewLUKSDeviceFromMappingPath_Fails(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	execReturnValue = `bad output`

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksStatusWithDevicePath(mockCommand).Return([]byte(execReturnValue), nil)
	command = mockCommand

	luksDevice, err := NewLUKSDeviceFromMappingPath(ctx, "/dev/mapper/luks-pvc-test", "pvc-test")
	assert.Error(t, err)
	assert.Nil(t, luksDevice)
}

func TestResize_Succeeds(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksResize(mockCommand).Return([]byte(""), nil)
	command = mockCommand

	luksDeviceName := "luks-test_pvc"
	luksDevice := &LUKSDevice{"/dev/sdb", luksDeviceName}
	err := luksDevice.Resize(ctx, "testpassphrase")
	assert.NoError(t, err)
}

func TestResize_FailsWithBadPassphrase(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	fakeExitError := exec.NewFakeExitError(2, "") // Exit code of 2 means the passphrase was bad.

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksResize(mockCommand).Return([]byte(""), fakeExitError)
	command = mockCommand

	luksDeviceName := "luks-test_pvc"
	luksDevice := &LUKSDevice{"/dev/sdb", luksDeviceName}
	err := luksDevice.Resize(ctx, "testpassphrase")
	assert.Error(t, err)

	expectedError := errors.IncorrectLUKSPassphraseError("")
	assert.ErrorAs(t, err, &expectedError)
}

func TestResize_FailsWithExecError(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	ctx := context.Background()
	fakeExitError := exec.NewFakeExitError(4, "")

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksResize(mockCommand).Return([]byte(""), fakeExitError)
	command = mockCommand

	luksDeviceName := "luks-test_pvc"
	luksDevice := &LUKSDevice{"/dev/sdb", luksDeviceName}
	err := luksDevice.Resize(ctx, "testpassphrase")
	assert.Error(t, err)

	unexpectedError := errors.IncorrectLUKSPassphraseError("")
	assert.NotErrorIs(t, err, unexpectedError)
}
