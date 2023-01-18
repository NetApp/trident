package utils

import (
	"context"
	"fmt"
	"os/exec"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/mocks/mock_utils/mock_luks"
)

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
func TestNewLUKSDevice(t *testing.T) {
	luksDevice := NewLUKSDevice("/dev/sdb", "pvc-test")

	assert.Equal(t, luksDevice.RawDevicePath(), "/dev/sdb")
	assert.Equal(t, luksDevice.MappedDevicePath(), "/dev/mapper/luks-pvc-test")
	assert.Equal(t, luksDevice.MappedDeviceName(), "luks-pvc-test")
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
func TestMountLUKSDevice(t *testing.T) {
	execCmd = fakeExecCommand
	execReturnCode = 0
	// Reset exec command after tests
	defer func() {
		execCmd = exec.CommandContext
	}()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test first passphrase works
	secrets := map[string]string{"luks-passphrase": "secretA", "luks-passphrase-name": "A"}
	mockCtrl := gomock.NewController(t)
	mockLUKSDevice := mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().EnsureFormattedAndOpen(gomock.Any(), "secretA").Return(false, nil)
	luksFormatted, err := EnsureLUKSDeviceMappedOnHost(context.Background(), mockLUKSDevice, "pvc-test", secrets)
	assert.NoError(t, err)
	assert.False(t, luksFormatted)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Test second passphrase works
	secrets = map[string]string{
		"luks-passphrase":               "secretA",
		"luks-passphrase-name":          "A",
		"previous-luks-passphrase":      "secretB",
		"previous-luks-passphrase-name": "B",
	}
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().EnsureFormattedAndOpen(gomock.Any(), "secretA").Return(false, fmt.Errorf("bad passphrase"))
	mockLUKSDevice.EXPECT().EnsureFormattedAndOpen(gomock.Any(), "secretB").Return(false, nil)

	luksFormatted, err = EnsureLUKSDeviceMappedOnHost(context.Background(), mockLUKSDevice, "pvc-test", secrets)
	assert.NoError(t, err)
	assert.False(t, luksFormatted)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: passphrase rotation fails
	secrets = map[string]string{
		"luks-passphrase":               "secretA",
		"luks-passphrase-name":          "A",
		"previous-luks-passphrase":      "secretB",
		"previous-luks-passphrase-name": "B",
	}
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().EnsureFormattedAndOpen(gomock.Any(), "secretA").Return(false, fmt.Errorf("bad passphrase"))
	mockLUKSDevice.EXPECT().EnsureFormattedAndOpen(gomock.Any(), "secretB").Return(false, nil)

	luksFormatted, err = EnsureLUKSDeviceMappedOnHost(context.Background(), mockLUKSDevice, "pvc-test", secrets)
	assert.NoError(t, err)
	assert.False(t, luksFormatted)
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
func TestMountLUKSDevice_Negative(t *testing.T) {
	execCmd = fakeExecCommand
	execReturnCode = 0
	// Reset exec command after tests
	defer func() {
		execCmd = exec.CommandContext
	}()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test no passphrase specified
	mockCtrl := gomock.NewController(t)
	mockLUKSDevice := mock_luks.NewMockLUKSDeviceInterface(mockCtrl)

	secrets := map[string]string{}
	luksFormatted, err := EnsureLUKSDeviceMappedOnHost(context.Background(), mockLUKSDevice, "pvc-test", secrets)
	assert.Error(t, err)
	assert.False(t, luksFormatted)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test no passphrase name specified
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)

	secrets = map[string]string{
		"luks-passphrase": "secretA",
	}
	luksFormatted, err = EnsureLUKSDeviceMappedOnHost(context.Background(), mockLUKSDevice, "pvc-test", secrets)
	assert.Error(t, err)
	assert.False(t, luksFormatted)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test no second passphrase name specified
	secrets = map[string]string{
		"luks-passphrase":          "secretA",
		"luks-passphrase-name":     "A",
		"previous-luks-passphrase": "secretB",
	}
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().EnsureFormattedAndOpen(gomock.Any(), "secretA").Return(false, fmt.Errorf("bad passphrase"))

	luksFormatted, err = EnsureLUKSDeviceMappedOnHost(context.Background(), mockLUKSDevice, "pvc-test", secrets)
	assert.Error(t, err)
	assert.False(t, luksFormatted)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test first passphrase fails, no second specified
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().EnsureFormattedAndOpen(gomock.Any(), "secretA").Return(false, fmt.Errorf("bad passphrase")).Times(1)

	secrets = map[string]string{
		"luks-passphrase":      "secretA",
		"luks-passphrase-name": "A",
	}
	luksFormatted, err = EnsureLUKSDeviceMappedOnHost(context.Background(), mockLUKSDevice, "pvc-test", secrets)
	assert.Error(t, err)
	assert.False(t, luksFormatted)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test first passphrase fails, second is blank
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().EnsureFormattedAndOpen(gomock.Any(), "secretA").Return(false, fmt.Errorf("bad passphrase")).Times(1)
	secrets = map[string]string{
		"luks-passphrase":               "secretA",
		"luks-passphrase-name":          "A",
		"previous-luks-passphrase":      "",
		"previous-luks-passphrase-name": "",
	}
	luksFormatted, err = EnsureLUKSDeviceMappedOnHost(context.Background(), mockLUKSDevice, "pvc-test", secrets)
	assert.Error(t, err)
	assert.False(t, luksFormatted)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test first passphrase fails, second is the same
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().EnsureFormattedAndOpen(gomock.Any(), "secretA").Return(false, fmt.Errorf("bad passphrase")).Times(1)
	secrets = map[string]string{
		"luks-passphrase":               "secretA",
		"luks-passphrase-name":          "A",
		"previous-luks-passphrase":      "secretA",
		"previous-luks-passphrase-name": "A",
	}
	luksFormatted, err = EnsureLUKSDeviceMappedOnHost(context.Background(), mockLUKSDevice, "pvc-test", secrets)
	assert.Error(t, err)
	assert.False(t, luksFormatted)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test first passphrase is blank
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	secrets = map[string]string{
		"luks-passphrase":               "",
		"luks-passphrase-name":          "",
		"previous-luks-passphrase":      "secretB",
		"previous-luks-passphrase-name": "B",
	}
	luksFormatted, err = EnsureLUKSDeviceMappedOnHost(context.Background(), mockLUKSDevice, "pvc-test", secrets)
	assert.Error(t, err)
	assert.False(t, luksFormatted)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Passphrase rotation fails
	execReturnCode = 4
	secrets = map[string]string{
		"luks-passphrase":               "secretB",
		"luks-passphrase-name":          "B",
		"previous-luks-passphrase":      "secretA",
		"previous-luks-passphrase-name": "A",
	}
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().EnsureFormattedAndOpen(gomock.Any(), "secretB").Return(false, fmt.Errorf("bad passphrase")).Times(1)
	mockLUKSDevice.EXPECT().EnsureFormattedAndOpen(gomock.Any(), "secretA").Return(false, nil)

	luksFormatted, err = EnsureLUKSDeviceMappedOnHost(context.Background(), mockLUKSDevice, "pvc-test", secrets)
	assert.NoError(t, err)
	assert.False(t, luksFormatted)

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Test second passphrase is also incorrect
	secrets = map[string]string{
		"luks-passphrase":               "secretA",
		"luks-passphrase-name":          "A",
		"previous-luks-passphrase":      "secretB",
		"previous-luks-passphrase-name": "B",
	}
	mockCtrl = gomock.NewController(t)
	mockLUKSDevice = mock_luks.NewMockLUKSDeviceInterface(mockCtrl)
	mockLUKSDevice.EXPECT().EnsureFormattedAndOpen(gomock.Any(), "secretA").Return(false, fmt.Errorf("bad passphrase"))
	mockLUKSDevice.EXPECT().EnsureFormattedAndOpen(gomock.Any(), "secretB").Return(false, fmt.Errorf("bad passphrase"))

	luksFormatted, err = EnsureLUKSDeviceMappedOnHost(context.Background(), mockLUKSDevice, "pvc-test", secrets)
	assert.Error(t, err)
	assert.False(t, luksFormatted)
}
