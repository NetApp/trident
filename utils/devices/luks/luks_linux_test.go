// Copyright 2024 NetApp, Inc. All Rights Reserved.

//go:build linux

package luks

import (
	"context"
	"fmt"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"golang.org/x/sys/unix"

	"github.com/netapp/trident/mocks/mock_utils/mock_devices"
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

func TestLUKSDevice_LUKSFormat(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := NewDetailed("/dev/sdb", "pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())
	assert.Equal(t, "/dev/mapper/pvc-test", luksDevice.MappedDevicePath())
	assert.Equal(t, "/dev/sdb", luksDevice.RawDevicePath())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupIsLuks(mockCommand).Return([]byte(""), nil)

	err := luksDevice.lUKSFormat(context.Background(), "passphrase")
	assert.NoError(t, err)
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

	err := luksDevice.lUKSFormat(ctx, "mysecretlukspassphrase")
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

func TestLUKSDevice_MissingDevicePath(t *testing.T) {
	luksDevice := LUKSDevice{mappedDeviceName: "pvc-test", rawDevicePath: ""}
	isFormatted, err := luksDevice.IsLUKSFormatted(context.Background())
	assert.Error(t, err)
	assert.False(t, isFormatted)

	err = luksDevice.luksFormat(context.Background(), "passphrase")
	assert.Error(t, err)

	err = luksDevice.Open(context.Background(), "passphrase")
	assert.Error(t, err)
}

func TestLUKSDevice_ExecErrors(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+"pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())

	// Setup mock calls and reassign any clients to their mock counterparts.
	gomock.InOrder(
		mockCryptsetupIsLuks(mockCommand).Return([]byte(""), luksError),
		mockCryptsetupLuksFormat(mockCommand).Return([]byte(""), luksError),
		mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), luksError),
		mockCryptsetupLuksStatus(mockCommand).Return([]byte(""), luksError),
		mockCryptsetupLuksClose(mockCommand).Return([]byte(""), luksError),
	)

	isFormatted, err := luksDevice.IsLUKSFormatted(context.Background())
	assert.Error(t, err)
	assert.False(t, isFormatted)

	err = luksDevice.luksFormat(context.Background(), "passphrase")
	assert.Error(t, err)

	err = luksDevice.Open(context.Background(), "passphrase")
	assert.Error(t, err)

	isOpen, err := luksDevice.IsOpen(context.Background())
	assert.Error(t, err)
	assert.False(t, isOpen)

	devicesClient := devices.NewDetailed(mockCommand, afero.NewMemMapFs())
	err = devicesClient.CloseLUKSDevice(context.Background(), luksDevice.MappedDevicePath())
	assert.Error(t, err)
}

func TestEnsureLUKSDevice_FailsWithExecError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	mockCryptsetupLuksStatus(mockCommand).Return([]byte(""), luksError)
	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+"pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())

	luksFormatted, err := luksDevice.ensureLUKSDevice(context.Background(), "mysecretlukspassphrase")
	assert.Error(t, err)
	assert.Equal(t, false, luksFormatted)
}

func TestEnsureLUKSDevice_IsOpen(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksStatus(mockCommand)
	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+"pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())

	luksFormatted, err := luksDevice.ensureLUKSDevice(context.Background(), "mysecretlukspassphrase")
	assert.NoError(t, err)
	assert.True(t, luksFormatted)
}

func TestEnsureLUKSDevice_LUKSFormatFails(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksStatus(mockCommand).Return([]byte{},
		mock_exec.NewMockExitError(cryptsetupStatusDeviceDoesNotExistStatusCode, "mock error"))
	mockCryptsetupIsLuks(mockCommand).Return([]byte{},
		mock_exec.NewMockExitError(cryptsetupIsLuksDeviceIsNotLuksStatusCode, "mock error"))
	mockCryptsetupLuksFormat(mockCommand).Return([]byte{}, luksError)

	mockDevices := mock_devices.NewMockDevices(mockCtrl)
	mockDevices.EXPECT().IsDeviceUnformatted(gomock.Any(), gomock.Any()).Return(true, nil)

	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+"pvc-test", mockCommand, mockDevices, afero.NewMemMapFs())

	luksFormatted, err := luksDevice.ensureLUKSDevice(context.Background(), "mysecretlukspassphrase")
	assert.Error(t, err)
	assert.False(t, luksFormatted)
}

func TestLUKSDeviceOpen_FailsWithBadPassphrase(t *testing.T) {
	ctx := context.Background()
	fakeExitError := mock_exec.NewMockExitError(2, "mock error") // Exit code of 2 means the passphrase was bad.

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), fakeExitError)

	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+"pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())
	err := luksDevice.Open(ctx, "passphrase")
	assert.Error(t, err)
}

func TestLUKSDeviceOpen_MiscError(t *testing.T) {
	ctx := context.Background()
	fakeExitError := mock_exec.NewMockExitError(2, "mock error") // Exit code of 2 means the passphrase was bad.

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), fakeExitError)

	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+"pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())
	err := luksDevice.Open(ctx, "passphrase")
	assert.Error(t, err)
}

func TestLUKSDeviceOpen_ExecError(t *testing.T) {
	ctx := context.Background()
	fakeExitError := mock_exec.NewMockExitError(1, "mock error") // Exit code of 1 means the exec command failed.

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksOpen(mockCommand).Return([]byte(""), fakeExitError)

	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+"pvc-test", mockCommand, devices.New(), afero.NewMemMapFs())
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

	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+luksDeviceName, mockCommand, devices.New(), afero.NewMemMapFs())
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "previous", "newpassphrase")
	assert.NoError(t, err)
}

func TestRotateLUKSDevicePassphrase_OldPassphraseEmpty(t *testing.T) {
	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"
	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+luksDeviceName, nil, nil, nil)
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "", "newpassphrase")
	assert.Error(t, err)
}

func TestRotateLUKSDevicePassphrase_NewPassphraseEmpty(t *testing.T) {
	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"
	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+luksDeviceName, nil, nil, nil)
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "oldpassphrase", "")
	assert.Error(t, err)
}

func TestRotateLUKSDevicePassphrase_CommandError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksChangeKey(mockCommand).Return([]byte(""), luksError)

	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"
	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+luksDeviceName, mockCommand, nil, nil)
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "previous", "newpassphrase")
	assert.Error(t, err)
}

func TestRotateLUKSDevicePassphrase_NoRawDevicePath(t *testing.T) {
	ctx := context.Background()
	luksDeviceName := "luks-pvc-test"
	luksDevice := NewDetailed("", luksDevicePrefix+luksDeviceName, nil, nil, nil)
	err := luksDevice.RotatePassphrase(ctx, "pvc-test", "previous", "newpassphrase")
	assert.Error(t, err)
}

func TestGetUnderlyingDevicePathForLUKSDevice_Succeeds(t *testing.T) {
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

	devicePath, err := GetUnderlyingDevicePathForLUKSDevice(ctx, mockCommand, "")
	assert.NoError(t, err)
	assert.Equal(t, "/dev/mapper/3600a09807770457a795d526950374c76", devicePath)
}

func TestGetUnderlyingDevicePathForLUKSDevice_FailsWithBadOutput(t *testing.T) {
	ctx := context.Background()
	execReturnValue := `bad output`

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksStatusWithDevicePath(mockCommand).Return([]byte(execReturnValue), nil)

	devicePath, err := GetUnderlyingDevicePathForLUKSDevice(ctx, mockCommand, "")
	assert.Error(t, err)
	assert.Equal(t, "", devicePath)
}

func TestGetUnderlyingDevicePathForLUKSDevice_FailsWithNoLUKSDevice(t *testing.T) {
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

	devicePath, err := GetUnderlyingDevicePathForLUKSDevice(ctx, mockCommand, "")
	assert.Error(t, err)
	assert.Equal(t, "", devicePath)
}

func TestGetUnderlyingDevicePathForLUKSDevice_FailsWithDeviceIncorrectlyFormatted(t *testing.T) {
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

	_, err := GetUnderlyingDevicePathForLUKSDevice(ctx, mockCommand, "")
	assert.Error(t, err)
}

func TestCheckPassphrase_Succeeds(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	mockCryptsetupLuksTestPassphrase(mockCommand).Return([]byte(""), nil)

	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+"test-pvc", mockCommand, nil, nil)

	correct, err := luksDevice.CheckPassphrase(ctx, "passphrase")
	assert.True(t, correct)
	assert.NoError(t, err)
}

func TestCheckPassphrase_FailsWithExecError(t *testing.T) {
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksTestPassphrase(mockCommand).Return([]byte(""), luksError)

	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+"test-pvc", mockCommand, nil, nil)

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

	luksDevice := NewDetailed("/dev/sdb", luksDevicePrefix+"test-pvc", mockCommand, nil, nil)

	correct, err := luksDevice.CheckPassphrase(ctx, "passphrase")
	assert.False(t, correct)
	assert.NoError(t, err)
}

func TestNewLUKSDeviceFromMappingPath_Succeeds(t *testing.T) {
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

	luksDevice, err := NewLUKSDeviceFromMappingPath(ctx, mockCommand, "/dev/mapper/luks-pvc-test", "pvc-test")
	assert.NoError(t, err)
	assert.Equal(t, luksDevice.RawDevicePath(), "/dev/sdb")
	assert.Equal(t, luksDevice.MappedDevicePath(), "/dev/mapper/luks-pvc-test")
	assert.Equal(t, luksDevice.MappedDeviceName(), "luks-pvc-test")
}

func TestNewLUKSDeviceFromMappingPath_Fails(t *testing.T) {
	ctx := context.Background()
	execReturnValue := `bad output`

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCryptsetupLuksStatusWithDevicePath(mockCommand).Return([]byte(execReturnValue), nil)

	luksDevice, err := NewLUKSDeviceFromMappingPath(ctx, mockCommand, "/dev/mapper/luks-pvc-test", "pvc-test")
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
	formatted, err := luksDevice.EnsureFormattedAndOpen(context.Background(), passphrase)
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

	luksFormatted, err := luksDevice.EnsureLUKSDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
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

	luksFormatted, err := luksDevice.EnsureLUKSDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
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

	luksFormatted, err := luksDevice.EnsureLUKSDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
	assert.Error(t, err)
	assert.False(t, luksFormatted)
}

func TestMountLUKSDevice_NoPassphraseFailure(t *testing.T) {
	// Negative case: Test no passphrase specified
	mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
	luksDevice := NewDetailed("/dev/sdb", "1234", mockCommand, nil, nil)

	secrets := map[string]string{}
	luksFormatted, err := luksDevice.EnsureLUKSDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
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
	luksFormatted, err := luksDevice.EnsureLUKSDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
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

	luksFormatted, err := luksDevice.EnsureLUKSDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
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
	luksFormatted, err := luksDevice.EnsureLUKSDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
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
	luksFormatted, err := luksDevice.EnsureLUKSDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
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
	luksFormatted, err := luksDevice.EnsureLUKSDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
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
	luksFormatted, err := luksDevice.EnsureLUKSDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
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

	luksFormatted, err := luksDevice.EnsureLUKSDeviceMappedOnHost(context.Background(), "pvc-test", secrets)
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
