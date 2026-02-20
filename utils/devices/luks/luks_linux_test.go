// Copyright 2025 NetApp, Inc. All Rights Reserved.

//go:build linux

package luks

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"golang.org/x/sys/unix"

	"github.com/netapp/trident/mocks/mock_utils/mock_devices"
	"github.com/netapp/trident/mocks/mock_utils/mock_devices/mock_luks"
	"github.com/netapp/trident/mocks/mock_utils/mock_exec"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/errors"
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
		"--hash", "sha256", "--pbkdf", "pbkdf2", "--pbkdf-force-iterations", "1000",
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
		"--allow-discards", "--persistent",
	)
}

func mockCryptsetupLuksStatusWithDevicePath(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", luksCommandTimeout, true, "", "status", gomock.Any(),
	)
}

func mockCryptsetupLuksTestPassphrase(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", luksCommandTimeout, true, gomock.Any(),
		"open", gomock.Any(), gomock.Any(), "--type", "luks2", "--test-passphrase",
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
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := NewDetailed("/dev/sdb", "pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())
	assert.Equal(t, "/dev/mapper/pvc-test", luksDevice.MappedDevicePath())
	assert.Equal(t, "/dev/sdb", luksDevice.RawDevicePath())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupIsLuks(mockCommand).Return([]byte(""), nil)

	isFormatted, err := luksDevice.IsLUKSFormatted(context.Background())
	assert.NoError(t, err)
	assert.True(t, isFormatted)
}

func TestLUKSDevice_Format(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	mockCryptsetupIsLuks(mockCommand).Return([]byte(""),
		mock_exec.NewMockExitError(cryptsetupIsLuksDeviceIsNotLuksStatusCode, "not LUKS"))
	mockCryptsetupLuksFormat(mockCommand).Return([]byte(""), nil)
	mockCryptsetupIsLuks(mockCommand).Return([]byte(""), nil)

	mockDevices := mock_devices.NewMockDevices(mockCtrl)
	mockDevices.EXPECT().IsDeviceUnformatted(gomock.Any(), "/dev/sdb").Return(true, nil)

	luksDevice := NewDetailed("/dev/sdb", "pvc-test", mockCommand, mockDevices, afero.NewMemMapFs())
	safeToFormat, err := luksDevice.formatUnformattedDevice(context.Background(), "passphrase")
	assert.True(t, safeToFormat)
	assert.NoError(t, err)
}

func TestLUKSFormat_UnformattedCheckError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	mockCryptsetupIsLuks(mockCommand).Return([]byte(""),
		mock_exec.NewMockExitError(cryptsetupIsLuksDeviceIsNotLuksStatusCode, "not LUKS"))

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockDevices := mock_devices.NewMockDevices(mockCtrl)
	mockDevices.EXPECT().IsDeviceUnformatted(gomock.Any(), "/dev/sdb").Return(false, errors.New("mock error"))

	luksDevice := NewDetailed("/dev/sdb", "pvc-test", mockCommand, mockDevices, afero.NewMemMapFs())
	safeToFormat, err := luksDevice.formatUnformattedDevice(context.Background(), "passphrase")
	assert.False(t, safeToFormat)
	assert.Error(t, err)
}

func TestLUKSFormat_SecondFormatCheckError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupIsLuks(mockCommand).Return([]byte(""),
		mock_exec.NewMockExitError(cryptsetupIsLuksDeviceIsNotLuksStatusCode, "not LUKS"))
	mockCryptsetupLuksFormat(mockCommand).Return([]byte(""), nil)
	mockCryptsetupIsLuks(mockCommand).Return([]byte(""),
		mock_exec.NewMockExitError(cryptsetupIsLuksDeviceIsNotLuksStatusCode, "not LUKS"))

	mockDevices := mock_devices.NewMockDevices(mockCtrl)
	mockDevices.EXPECT().IsDeviceUnformatted(gomock.Any(), "/dev/sdb").Return(true, nil)

	luksDevice := NewDetailed("/dev/sdb", "pvc-test", mockCommand, mockDevices, afero.NewMemMapFs())
	safeToFormat, err := luksDevice.formatUnformattedDevice(context.Background(), "passphrase")
	assert.False(t, safeToFormat)
	assert.Error(t, err)
}

func TestLUKSDevice_LUKSFormat_FailsCheckingIfDeviceIsLUKS(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	// Create fake luks device.
	luksDevice := NewDetailed("/dev/sdb", "pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())
	assert.Equal(t, "/dev/mapper/pvc-test", luksDevice.MappedDevicePath())
	assert.Equal(t, "/dev/sdb", luksDevice.RawDevicePath())

	// Mock any cryptsetup calls that may occur.
	mockCryptsetupIsLuks(mockCommand).Return([]byte(""), luksError)

	safeToFormat, err := luksDevice.formatUnformattedDevice(ctx, "mysecretlukspassphrase")
	assert.False(t, safeToFormat)
	assert.Error(t, err)
}

func TestLUKSDevice_Open(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := NewDetailed("/dev/sdb", "pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())
	assert.Equal(t, "/dev/mapper/pvc-test", luksDevice.MappedDevicePath())
	assert.Equal(t, "/dev/sdb", luksDevice.RawDevicePath())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), nil)

	err := luksDevice.Open(context.Background(), "passphrase")
	assert.NoError(t, err)
}

func TestLUKSDevice_IsOpen(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := NewDetailed("/dev/sdb", "pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())
	assert.Equal(t, "/dev/mapper/pvc-test", luksDevice.MappedDevicePath())
	assert.Equal(t, "/dev/sdb", luksDevice.RawDevicePath())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksStatus(mockCommand).Return([]byte(""), nil)

	isOpen, err := luksDevice.IsOpen(context.Background())
	assert.NoError(t, err)
	assert.True(t, isOpen)
}

func TestIsOpen_NotOpen(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := NewDetailed("/dev/sdb", "pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())
	assert.Equal(t, "/dev/mapper/pvc-test", luksDevice.MappedDevicePath())
	assert.Equal(t, "/dev/sdb", luksDevice.RawDevicePath())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksStatus(mockCommand).Return([]byte(""), mock_exec.NewMockExitError(cryptsetupStatusDeviceDoesExistStatusCode, "mock error"))

	isOpen, err := luksDevice.IsOpen(context.Background())
	assert.NoError(t, err)
	assert.True(t, isOpen)
}

func TestIsOpen_UnknownError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := NewDetailed("/dev/sdb", "pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())
	assert.Equal(t, "/dev/mapper/pvc-test", luksDevice.MappedDevicePath())
	assert.Equal(t, "/dev/sdb", luksDevice.RawDevicePath())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksStatus(mockCommand).Return([]byte(""), mock_exec.NewMockExitError(128, "mock error"))

	isOpen, err := luksDevice.IsOpen(context.Background())
	assert.Error(t, err)
	assert.False(t, isOpen)
}

func TestLUKSDevice_MissingDevicePath(t *testing.T) {
	luksDevice := LUKSDevice{mappedDeviceName: "pvc-test", rawDevicePath: ""}
	isFormatted, err := luksDevice.IsLUKSFormatted(context.Background())
	assert.Error(t, err)
	assert.False(t, isFormatted)

	err = luksDevice.format(context.Background(), "passphrase")
	assert.Error(t, err)

	err = luksDevice.Open(context.Background(), "passphrase")
	assert.Error(t, err)
}

func TestLUKSDevice_ExecErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := NewDetailed("/dev/sdb", devicePrefix+"pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())

	// Setup mock calls and reassign any clients to their mock counterparts.
	gomock.InOrder(
		mockCryptsetupIsLuks(mockCommand).Return([]byte(""), luksError),
		mockCryptsetupLuksFormat(mockCommand).Return([]byte(""), luksError),
		mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), luksError),
		mockCryptsetupLuksStatus(mockCommand).Return([]byte(""), luksError),
	)

	isFormatted, err := luksDevice.IsLUKSFormatted(context.Background())
	assert.Error(t, err)
	assert.False(t, isFormatted)

	err = luksDevice.format(context.Background(), "passphrase")
	assert.Error(t, err)

	err = luksDevice.Open(context.Background(), "passphrase")
	assert.Error(t, err)

	isOpen, err := luksDevice.IsOpen(context.Background())
	assert.Error(t, err)
	assert.False(t, isOpen)
}

func TestEnsureLUKSDevice_FailsWithExecError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	mockCryptsetupLuksStatus(mockCommand).Return([]byte(""), luksError)
	luksDevice := NewDetailed("/dev/sdb", devicePrefix+"pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())

	luksFormatted, safeToFormat, err := luksDevice.ensureLUKSDevice(context.Background(), "mysecretlukspassphrase")
	assert.False(t, safeToFormat)
	assert.Error(t, err)
	assert.Equal(t, false, luksFormatted)
}

func TestEnsureLUKSDevice_IsOpen(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksStatus(mockCommand)
	luksDevice := NewDetailed("/dev/sdb", devicePrefix+"pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())

	luksFormatted, safeToFormat, err := luksDevice.ensureLUKSDevice(context.Background(), "mysecretlukspassphrase")
	assert.False(t, safeToFormat)
	assert.NoError(t, err)
	assert.True(t, luksFormatted)
}

func TestEnsureLUKSDevice_LUKSFormatFails(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	rawDevicePath := "/dev/sdb"

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksStatus(mockCommand).Return([]byte{},
		mock_exec.NewMockExitError(cryptsetupStatusDeviceDoesNotExistStatusCode, "mock error"))
	mockCryptsetupIsLuks(mockCommand).Return([]byte{},
		mock_exec.NewMockExitError(cryptsetupIsLuksDeviceIsNotLuksStatusCode, "mock error"))
	mockCryptsetupLuksFormat(mockCommand).Return([]byte{}, luksError)

	mockDevices := mock_devices.NewMockDevices(mockCtrl)
	mockDevices.EXPECT().IsDeviceUnformatted(gomock.Any(), gomock.Any()).Return(true, nil)
	mockDevices.EXPECT().ClearFormatting(gomock.Any(), rawDevicePath).Return(nil)

	luksDevice := NewDetailed(rawDevicePath, devicePrefix+"pvc-test", mockCommand, mockDevices, afero.NewMemMapFs())

	luksFormatted, safeToFormat, err := luksDevice.ensureLUKSDevice(context.Background(), "mysecretlukspassphrase")
	assert.False(t, safeToFormat)
	assert.Error(t, err)
	assert.False(t, luksFormatted)
}

func TestLUKSDeviceOpen_FailsWithBadPassphrase(t *testing.T) {
	ctx := context.Background()
	fakeExitError := mock_exec.NewMockExitError(2, "mock error") // Exit code of 2 means the passphrase was bad.

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), fakeExitError)

	luksDevice := NewDetailed("/dev/sdb", devicePrefix+"pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())
	err := luksDevice.Open(ctx, "passphrase")
	assert.Error(t, err)
}

func TestLUKSDeviceOpen_MiscError(t *testing.T) {
	ctx := context.Background()
	fakeExitError := mock_exec.NewMockExitError(2, "mock error") // Exit code of 2 means the passphrase was bad.

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), fakeExitError)

	luksDevice := NewDetailed("/dev/sdb", devicePrefix+"pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())
	err := luksDevice.Open(ctx, "passphrase")
	assert.Error(t, err)
}

func TestLUKSDeviceOpen_ExecError(t *testing.T) {
	ctx := context.Background()
	fakeExitError := mock_exec.NewMockExitError(1, "mock error") // Exit code of 1 means the exec command failed.

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), fakeExitError)

	luksDevice := NewDetailed("/dev/sdb", devicePrefix+"pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())
	err := luksDevice.Open(ctx, "passphrase")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "could not open LUKS device; ")
}

func TestRotateLUKSDevicePassphrase_NoError(t *testing.T) {
	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksChangeKey(mockCommand).Return([]byte(""), nil)

	luksDevice := NewDetailed("/dev/sdb", devicePrefix+luksDeviceName, mockCommand, devices.New(), afero.NewMemMapFs())
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "previous", "newpassphrase")
	assert.NoError(t, err)
}

func TestRotateLUKSDevicePassphrase_OldPassphraseEmpty(t *testing.T) {
	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"
	luksDevice := NewDetailed("/dev/sdb", devicePrefix+luksDeviceName, nil, nil, nil)
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "", "newpassphrase")
	assert.Error(t, err)
}

func TestRotateLUKSDevicePassphrase_NewPassphraseEmpty(t *testing.T) {
	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"
	luksDevice := NewDetailed("/dev/sdb", devicePrefix+luksDeviceName, nil, nil, nil)
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "oldpassphrase", "")
	assert.Error(t, err)
}

func TestRotateLUKSDevicePassphrase_CommandError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksChangeKey(mockCommand).Return([]byte(""), luksError)

	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"
	luksDevice := NewDetailed("/dev/sdb", devicePrefix+luksDeviceName, mockCommand, nil, nil)
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "previous", "newpassphrase")
	assert.Error(t, err)
}

func TestRotateLUKSDevicePassphrase_NoRawDevicePath(t *testing.T) {
	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"
	luksDevice := NewDetailed("", devicePrefix+luksDeviceName, nil, nil, nil)
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "previous", "newpassphrase")
	assert.Error(t, err)
}

func TestGetUnderlyingDevicePathForDevice_Succeeds(t *testing.T) {
	ctx := context.Background()
	execReturnValue := `/dev/mapper/luks-trident_pvc_0c6202cb_be41_46b7_bea9_7f2c5c2c4a41 is active and is in use.
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

	devicePath, err := GetUnderlyingDevicePathForDevice(ctx, mockCommand, "")
	assert.NoError(t, err)
	assert.Equal(t, "/dev/mapper/3600a09807770457a795d526950374c76", devicePath)
}

func TestGetUnderlyingDevicePathForDevice_FailsWithBadOutput(t *testing.T) {
	ctx := context.Background()
	execReturnValue := `bad output`

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksStatusWithDevicePath(mockCommand).Return([]byte(execReturnValue), nil)

	devicePath, err := GetUnderlyingDevicePathForDevice(ctx, mockCommand, "")
	assert.Error(t, err)
	assert.Equal(t, "", devicePath)
}

func TestGetUnderlyingDevicePathForDevice_FailsWithNoLUKSDevice(t *testing.T) {
	ctx := context.Background()
	execReturnValue := `/dev/mapper/luks-trident_pvc_0c6202cb_be41_46b7_bea9_7f2c5c2c4a41 is active and is in use.
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

	devicePath, err := GetUnderlyingDevicePathForDevice(ctx, mockCommand, "")
	assert.Error(t, err)
	assert.Equal(t, "", devicePath)
}

func TestGetUnderlyingDevicePathForDevice_FailsWithDeviceIncorrectlyFormatted(t *testing.T) {
	ctx := context.Background()
	execReturnValue := `/dev/mapper/luks-trident_pvc_0c6202cb_be41_46b7_bea9_7f2c5c2c4a41 is active and is in use.
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

	_, err := GetUnderlyingDevicePathForDevice(ctx, mockCommand, "")
	assert.Error(t, err)
}

func TestCheckPassphrase_Succeeds(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	mockCryptsetupLuksTestPassphrase(mockCommand).Return([]byte(""), nil)

	luksDevice := NewDetailed("/dev/sdb", devicePrefix+"test-pvc", mockCommand, nil, nil)

	correct, err := luksDevice.CheckPassphrase(ctx, "passphrase")
	assert.True(t, correct)
	assert.NoError(t, err)
}

func TestCheckPassphrase_FailsWithExecError(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksTestPassphrase(mockCommand).Return([]byte(""), luksError)

	luksDevice := NewDetailed("/dev/sdb", devicePrefix+"test-pvc", mockCommand, nil, nil)

	correct, err := luksDevice.CheckPassphrase(ctx, "passphrase")
	assert.False(t, correct)
	assert.Error(t, err)
}

func TestCheckPassphrase_DetectsPassphraseIsBad(t *testing.T) {
	ctx := context.Background()
	fakeExitError := mock_exec.NewMockExitError(2, "mock error") // Exit code of 2 means the passphrase was bad.

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksTestPassphrase(mockCommand).Return([]byte(""), fakeExitError)

	luksDevice := NewDetailed("/dev/sdb", devicePrefix+"test-pvc", mockCommand, nil, nil)

	correct, err := luksDevice.CheckPassphrase(ctx, "passphrase")
	assert.False(t, correct)
	assert.NoError(t, err)
}

func TestNewDeviceFromMappingPath_Succeeds(t *testing.T) {
	ctx := context.Background()
	execReturnValue := `/dev/mapper/luks-pvc-test is active and is in use.
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

	luksDevice, err := NewDeviceFromMappingPath(ctx, mockCommand, "/dev/mapper/luks-pvc-test", "pvc-test")
	assert.NoError(t, err)
	assert.Equal(t, luksDevice.RawDevicePath(), "/dev/sdb")
	assert.Equal(t, luksDevice.MappedDevicePath(), "/dev/mapper/luks-pvc-test")
	assert.Equal(t, luksDevice.MappedDeviceName(), "luks-pvc-test")
}

func TestNewDeviceFromMappingPath_Fails(t *testing.T) {
	ctx := context.Background()
	execReturnValue := `bad output`

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksStatusWithDevicePath(mockCommand).Return([]byte(execReturnValue), nil)

	luksDevice, err := NewDeviceFromMappingPath(ctx, mockCommand, "/dev/mapper/luks-pvc-test", "pvc-test")
	assert.Error(t, err)
	assert.Nil(t, luksDevice)
}

func TestResize_Succeeds(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksResize(mockCommand).Return([]byte(""), nil)

	luksDeviceName := "luks-test_pvc"
	luksDevice := NewDetailed("/dev/sdb", luksDeviceName, mockCommand, devices.New(), afero.NewMemMapFs())
	err := luksDevice.Resize(ctx, "testpassphrase")
	assert.NoError(t, err)
}

func TestResize_FailsWithBadPassphrase(t *testing.T) {
	ctx := context.Background()
	fakeExitError := mock_exec.NewMockExitError(2, "mock error") // Exit code of 2 means the passphrase was bad.

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksResize(mockCommand).Return([]byte(""), fakeExitError)

	luksDeviceName := "luks-test_pvc"
	luksDevice := NewDetailed("/dev/sdb", luksDeviceName, mockCommand, devices.New(), afero.NewMemMapFs())
	err := luksDevice.Resize(ctx, "testpassphrase")
	assert.Error(t, err)

	expectedError := errors.IncorrectLUKSPassphraseError("")
	assert.ErrorAs(t, err, &expectedError)
}

func TestResize_FailsWithExecError(t *testing.T) {
	ctx := context.Background()
	fakeExitError := mock_exec.NewMockExitError(4, "mock error")

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksResize(mockCommand).Return([]byte(""), fakeExitError)

	luksDeviceName := "luks-test_pvc"
	luksDevice := NewDetailed("/dev/sdb", luksDeviceName, mockCommand, devices.New(), afero.NewMemMapFs())
	err := luksDevice.Resize(ctx, "testpassphrase")
	assert.Error(t, err)

	unexpectedError := errors.IncorrectLUKSPassphraseError("")
	assert.NotErrorIs(t, err, unexpectedError)
}

func TestEnsureFormattedAndOpen(t *testing.T) {
	// Positive case: Test first passphrase works
	passphrase := "secretA"
	mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
	mockCryptsetupLuksStatus(mockCommand)
	luksDevice := NewDetailed("/dev/sdb", "1234", mockCommand, nil, nil)
	formatted, safeToFormat, err := luksDevice.EnsureFormattedAndOpen(context.Background(), passphrase)
	assert.False(t, safeToFormat)
	assert.True(t, formatted)
	assert.NoError(t, err)
}

func TestMountLUKSDevice_firstPassphraseSuccess(t *testing.T) {
	// Positive case: Test first passphrase works
	secrets := map[string]string{"luks-passphrase": "secretA", "luks-passphrase-name": "A"}
	mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
	mockCryptsetupLuksStatus(mockCommand).Return([]byte{},
		mock_exec.NewMockExitError(cryptsetupStatusDeviceDoesNotExistStatusCode, "mock error"))
	mockCryptsetupIsLuks(mockCommand).Return([]byte{}, nil)
	mockCryptsetupLuksOpen(mockCommand).Return([]byte{}, nil)
	luksDevice := NewDetailed("/dev/sdb", "1234", mockCommand, nil, nil)

	luksFormatted, safeToFormat, err := luksDevice.EnsureDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
	assert.False(t, safeToFormat)
	assert.NoError(t, err)
	assert.True(t, luksFormatted)
}

func TestMountLUKSDevice_secondPassphraseSuccess(t *testing.T) {
	// Positive case: Test second passphrase works
	secrets := map[string]string{
		"luks-passphrase":               "secretA",
		"luks-passphrase-name":          "A",
		"previous-luks-passphrase":      "secretB",
		"previous-luks-passphrase-name": "B",
	}
	mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
	mockCryptsetupLuksStatus(mockCommand).Return([]byte{}, fmt.Errorf("mock error"))
	mockCryptsetupLuksStatus(mockCommand).Return([]byte{},
		mock_exec.NewMockExitError(cryptsetupStatusDeviceDoesNotExistStatusCode, "mock error"))
	mockCryptsetupIsLuks(mockCommand).Return([]byte{}, nil)
	mockCryptsetupLuksOpen(mockCommand).Return([]byte{}, nil)
	luksDevice := NewDetailed("/dev/sdb", "1234", mockCommand, nil, nil)

	luksFormatted, safeToFormat, err := luksDevice.EnsureDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
	assert.False(t, safeToFormat)
	assert.NoError(t, err)
	assert.True(t, luksFormatted)
}

func TestMountLUKSDevice_passphraseRotationFails(t *testing.T) {
	// Negative case: passphrase rotation fails
	secrets := map[string]string{
		"luks-passphrase":               "secretA",
		"luks-passphrase-name":          "A",
		"previous-luks-passphrase":      "secretB",
		"previous-luks-passphrase-name": "B",
	}
	mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
	mockCryptsetupLuksStatus(mockCommand).Return([]byte{}, fmt.Errorf("mock error")).Times(2)
	luksDevice := NewDetailed("/dev/sdb", "1234", mockCommand, nil, nil)

	luksFormatted, safeToFormat, err := luksDevice.EnsureDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
	assert.False(t, safeToFormat)
	assert.Error(t, err)
	assert.False(t, luksFormatted)
}

func TestMountLUKSDevice_NoPassphraseFailure(t *testing.T) {
	// Negative case: Test no passphrase specified
	mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
	luksDevice := NewDetailed("/dev/sdb", "1234", mockCommand, nil, nil)

	secrets := map[string]string{}
	luksFormatted, safeToFormat, err := luksDevice.EnsureDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
	assert.False(t, safeToFormat)
	assert.Error(t, err)
	assert.False(t, luksFormatted)
}

func TestMountLUKSDevice_NoPassphraseNameFailure(t *testing.T) {
	// Negative case: Test no passphrase name specified
	mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
	luksDevice := NewDetailed("/dev/sdb", "1234", mockCommand, nil, nil)

	secrets := map[string]string{
		"luks-passphrase": "secretA",
	}
	luksFormatted, safeToFormat, err := luksDevice.EnsureDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
	assert.False(t, safeToFormat)
	assert.Error(t, err)
	assert.False(t, luksFormatted)
}

func TestMountLUKSDevice_NoSecondPassphraseNameFailure(t *testing.T) {
	// Negative case: Test no second passphrase name specified
	secrets := map[string]string{
		"luks-passphrase":          "secretA",
		"luks-passphrase-name":     "A",
		"previous-luks-passphrase": "secretB",
	}
	mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
	luksDevice := NewDetailed("/dev/sdb", "1234", mockCommand, nil, nil)
	mockCryptsetupLuksStatus(mockCommand).Return([]byte{}, fmt.Errorf("mock error")).Times(1)

	luksFormatted, safeToFormat, err := luksDevice.EnsureDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
	assert.False(t, safeToFormat)
	assert.Error(t, err)
	assert.False(t, luksFormatted)
}

func TestMountLUKSDevice_NoSecondPassphraseNameSpecifiedFailure(t *testing.T) {
	//	// Negative case: Test first passphrase fails, no second specified

	mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
	luksDevice := NewDetailed("/dev/sdb", "1234", mockCommand, nil, nil)
	mockCryptsetupLuksStatus(mockCommand).Return([]byte{}, fmt.Errorf("mock error"))

	secrets := map[string]string{
		"luks-passphrase":      "secretA",
		"luks-passphrase-name": "A",
	}
	luksFormatted, safeToFormat, err := luksDevice.EnsureDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
	assert.False(t, safeToFormat)
	assert.Error(t, err)
	assert.False(t, luksFormatted)
}

func TestMountLUKSDevice_NoSecondPassphraseNameBlankFailure(t *testing.T) {
	// Negative case: Test first passphrase fails, second is blank
	mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
	luksDevice := NewDetailed("/dev/sdb", "1234", mockCommand, nil, nil)
	mockCryptsetupLuksStatus(mockCommand).Return([]byte{}, fmt.Errorf("mock error"))
	secrets := map[string]string{
		"luks-passphrase":               "secretA",
		"luks-passphrase-name":          "A",
		"previous-luks-passphrase":      "",
		"previous-luks-passphrase-name": "",
	}
	luksFormatted, safeToFormat, err := luksDevice.EnsureDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
	assert.False(t, safeToFormat)
	assert.Error(t, err)
	assert.False(t, luksFormatted)
}

func TestMountLUKSDevice_DuplicatePassphraseFailure(t *testing.T) {
	// Negative case: Test first passphrase fails, second is the same
	mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
	luksDevice := NewDetailed("/dev/sdb", "1234", mockCommand, nil, nil)
	mockCryptsetupLuksStatus(mockCommand).Return([]byte{}, fmt.Errorf("mock-error"))
	secrets := map[string]string{
		"luks-passphrase":               "secretA",
		"luks-passphrase-name":          "A",
		"previous-luks-passphrase":      "secretA",
		"previous-luks-passphrase-name": "A",
	}
	luksFormatted, safeToFormat, err := luksDevice.EnsureDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
	assert.False(t, safeToFormat)
	assert.Error(t, err)
	assert.False(t, luksFormatted)
}

func TestMountLUKSDevice_FirstPassphraseBlankFailure(t *testing.T) {
	// Negative case: Test first passphrase is blank
	mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
	luksDevice := NewDetailed("/dev/sdb", "1234", mockCommand, nil, nil)
	secrets := map[string]string{
		"luks-passphrase":               "",
		"luks-passphrase-name":          "",
		"previous-luks-passphrase":      "secretB",
		"previous-luks-passphrase-name": "B",
	}
	luksFormatted, safeToFormat, err := luksDevice.EnsureDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
	assert.False(t, safeToFormat)
	assert.Error(t, err)
	assert.False(t, luksFormatted)
}

func TestMountLUKSDevice_FirstPassphraseBlankFailureasdf(t *testing.T) {
	// Negative case: Test second passphrase is also incorrect
	secrets := map[string]string{
		"luks-passphrase":               "secretA",
		"luks-passphrase-name":          "A",
		"previous-luks-passphrase":      "secretB",
		"previous-luks-passphrase-name": "B",
	}
	mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
	luksDevice := NewDetailed("/dev/sdb", "1234", mockCommand, nil, nil)
	mockCryptsetupLuksStatus(mockCommand).Return([]byte{}, fmt.Errorf("mock-error")).Times(2)

	luksFormatted, safeToFormat, err := luksDevice.EnsureDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
	assert.False(t, safeToFormat)
	assert.Error(t, err)
	assert.False(t, luksFormatted)
}

func TestGenerateAnonymousMemFile(t *testing.T) {
	tempFileName := "testFile"
	content := "testContent"

	fd, err := generateAnonymousMemFile(tempFileName, content)
	assert.NoError(t, err, "expected no error creating anonymous mem file")
	assert.Greater(t, fd, 0, "expected valid file descriptor")

	// Read back the content to verify
	readContent := make([]byte, len(content))
	_, err = unix.Read(fd, readContent)
	assert.NoError(t, err, "expected no error reading anonymous mem file")
	assert.Equal(t, content, string(readContent), "expected content to match")

	// Close the file descriptor
	err = unix.Close(fd)
	assert.NoError(t, err, "expected no error closing anonymous mem file")
}

// Stub file info for unit testing
type fakeFileInfo struct {
	name string
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() interface{}   { return nil }

func TestLUKSDevice_IsMappingStale(t *testing.T) {
	type deviceOption func(device *LUKSDevice)
	instrumentDevice := func(opts ...deviceOption) *LUKSDevice {
		device := &LUKSDevice{
			rawDevicePath:    "/dev/sdb",
			mappedDeviceName: "pvc-test",
		}
		for _, opt := range opts {
			opt(device)
		}
		return device
	}

	tt := map[string]struct {
		createOpt  func(*gomock.Controller) deviceOption
		assertBool assert.BoolAssertionFunc
	}{
		"with no mapper not found": {
			createOpt: func(ctrl *gomock.Controller) deviceOption {
				return func(device *LUKSDevice) {
					mockOS := mock_luks.NewMockOS(ctrl)
					mockOS.EXPECT().Stat(device.MappedDevicePath()).Return(nil, os.ErrNotExist).Times(1)

					device.osFs = mockOS
				}
			},
			assertBool: assert.False,
		},
		"with failure to glob dm-* directories": {
			createOpt: func(ctrl *gomock.Controller) deviceOption {
				return func(device *LUKSDevice) {
					mockOS := mock_luks.NewMockOS(ctrl)
					mockOS.EXPECT().Stat(device.MappedDevicePath()).Return(nil, nil)
					mockOS.EXPECT().Glob("/sys/block/dm-*").Return([]string{"/sys/block/dm-0"}, errors.New("mock-error"))

					device.osFs = mockOS
				}
			},
			assertBool: assert.True,
		},
		"with no dm-* directories found": {
			createOpt: func(ctrl *gomock.Controller) deviceOption {
				return func(device *LUKSDevice) {
					mockOS := mock_luks.NewMockOS(ctrl)
					mockOS.EXPECT().Stat(device.MappedDevicePath()).Return(nil, nil)
					mockOS.EXPECT().Glob("/sys/block/dm-*").Return([]string{}, nil)

					device.osFs = mockOS
				}
			},
			assertBool: assert.True,
		},
		"with dm-* directory found, wrong device name": {
			createOpt: func(ctrl *gomock.Controller) deviceOption {
				return func(device *LUKSDevice) {
					mockOS := mock_luks.NewMockOS(ctrl)
					mockOS.EXPECT().Stat(device.MappedDevicePath()).Return(nil, nil)
					mockOS.EXPECT().Glob("/sys/block/dm-*").Return([]string{"/sys/block/dm-0"}, nil)
					mockOS.EXPECT().ReadFile("/sys/block/dm-0/dm/name").Return([]byte("not-the-mapper"), nil)

					device.osFs = mockOS
				}
			},
			assertBool: assert.True,
		},
		"with dm-* directory found, correct device name, no slaves": {
			createOpt: func(ctrl *gomock.Controller) deviceOption {
				return func(device *LUKSDevice) {
					mockOS := mock_luks.NewMockOS(ctrl)
					mockOS.EXPECT().Stat(device.MappedDevicePath()).Return(nil, nil)
					mockOS.EXPECT().Glob("/sys/block/dm-*").Return([]string{"/sys/block/dm-0"}, nil)
					mockOS.EXPECT().ReadFile("/sys/block/dm-0/dm/name").Return([]byte(device.mappedDeviceName), nil)
					mockOS.EXPECT().ReadDir("/sys/block/dm-0/slaves").Return([]os.FileInfo{}, nil)

					device.osFs = mockOS
				}
			},
			assertBool: assert.True,
		},
		"with dm-* directory found, correct device name, slave symlink broken": {
			createOpt: func(ctrl *gomock.Controller) deviceOption {
				return func(device *LUKSDevice) {
					mockOS := mock_luks.NewMockOS(ctrl)
					mockOS.EXPECT().Stat(device.MappedDevicePath()).Return(nil, nil)
					mockOS.EXPECT().Glob("/sys/block/dm-*").Return([]string{"/sys/block/dm-0"}, nil)
					mockOS.EXPECT().ReadFile("/sys/block/dm-0/dm/name").Return([]byte(device.mappedDeviceName), nil)
					// One slave entry matching deviceNode
					slaveInfo := fakeFileInfo{name: "sdb"}
					mockOS.EXPECT().ReadDir("/sys/block/dm-0/slaves").Return([]os.FileInfo{slaveInfo}, nil)
					mockOS.EXPECT().ReadlinkIfPossible("/sys/block/dm-0/slaves/sdb").Return("",
						fmt.Errorf("broken symlink"))

					device.osFs = mockOS
				}
			},
			assertBool: assert.True,
		},
		"with dm-* directory found, correct device name, slave symlink ok, target device missing": {
			createOpt: func(ctrl *gomock.Controller) deviceOption {
				return func(device *LUKSDevice) {
					mockOS := mock_luks.NewMockOS(ctrl)
					mockOS.EXPECT().Stat(device.MappedDevicePath()).Return(nil, nil)
					mockOS.EXPECT().Glob("/sys/block/dm-*").Return([]string{"/sys/block/dm-0"}, nil)
					mockOS.EXPECT().ReadFile("/sys/block/dm-0/dm/name").Return([]byte(device.mappedDeviceName), nil)
					// One slave entry matching deviceNode
					slaveInfo := fakeFileInfo{name: "sdb"}
					mockOS.EXPECT().ReadDir("/sys/block/dm-0/slaves").Return([]os.FileInfo{slaveInfo}, nil)
					mockOS.EXPECT().ReadlinkIfPossible("/sys/block/dm-0/slaves/sdb").Return("/dev/sdb", nil)
					mockOS.EXPECT().Stat("/dev/sdb").Return(nil, os.ErrNotExist)

					device.osFs = mockOS
				}
			},
			assertBool: assert.True,
		},
		"with dm-* directory found, correct device name, slave symlink ok, target device present": {
			createOpt: func(ctrl *gomock.Controller) deviceOption {
				return func(device *LUKSDevice) {
					mockOS := mock_luks.NewMockOS(ctrl)
					mockOS.EXPECT().Stat(device.MappedDevicePath()).Return(nil, nil)
					mockOS.EXPECT().Glob("/sys/block/dm-*").Return([]string{"/sys/block/dm-0"}, nil)
					mockOS.EXPECT().ReadFile("/sys/block/dm-0/dm/name").Return([]byte(device.mappedDeviceName), nil)
					// One slave entry matching deviceNode
					slaveInfo := fakeFileInfo{name: "sdb"}
					mockOS.EXPECT().ReadDir("/sys/block/dm-0/slaves").Return([]os.FileInfo{slaveInfo}, nil)
					mockOS.EXPECT().ReadlinkIfPossible("/sys/block/dm-0/slaves/sdb").Return("/dev/sdb", nil)
					mockOS.EXPECT().Stat("/dev/sdb").Return(nil, nil)

					device.osFs = mockOS
				}
			},
			assertBool: assert.False,
		},
	}

	for name, params := range tt {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ctrl := gomock.NewController(t)
			assertBool := params.assertBool
			device := instrumentDevice(params.createOpt(ctrl))
			assertBool(t, device.IsMappingStale(ctx))
		})
	}
}
