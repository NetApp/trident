// Copyright 2024 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/mocks/mock_utils/mock_models/mock_luks"
	"github.com/netapp/trident/utils/errors"
)

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
func TestNewLUKSDevice(t *testing.T) {
	luksDevice, _ := NewLUKSDevice("/dev/sdb", "pvc-test")

	assert.Equal(t, luksDevice.RawDevicePath(), "/dev/sdb")
	assert.Equal(t, luksDevice.MappedDevicePath(), "/dev/mapper/luks-pvc-test")
	assert.Equal(t, luksDevice.MappedDeviceName(), "luks-pvc-test")
}

// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
func TestMountLUKSDevice(t *testing.T) {
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

func TestRemoveMultipathDeviceMapping(t *testing.T) {
	originalCmd := command
	// Reset 'command' at the end of the test
	defer func() { command = originalCmd }()

	mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
	command = mockCommand // Set package var to mock

	tests := []struct {
		name        string
		devicePath  string
		mockReturn  []byte
		mockError   error
		expectError bool
	}{
		{
			name:        "Happy Path",
			devicePath:  "/dev/mock-0",
			mockReturn:  []byte("mock output"),
			mockError:   nil,
			expectError: false,
		},
		{
			name:        "Blank Device Path",
			devicePath:  "",
			mockReturn:  nil,
			mockError:   nil,
			expectError: false,
		},
		{
			name:        "Device does not exist",
			devicePath:  "/dev/mapper/doesNotExist",
			mockReturn:  []byte("'/dev/mapper/doesNotExist' is not a valid argument"),
			mockError:   fmt.Errorf("error"),
			expectError: false,
		},
		{
			name:        "Negative case",
			devicePath:  "/dev/mock-0",
			mockReturn:  nil,
			mockError:   fmt.Errorf("error"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.devicePath != "" {
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "multipath", 10*time.Second, false, "-f", tt.devicePath).
					Return(tt.mockReturn, tt.mockError)
			}

			err := RemoveMultipathDeviceMapping(context.TODO(), tt.devicePath)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWaitForDevicesRemoval(t *testing.T) {
	errMsg := "timed out waiting for devices to be removed"
	tests := map[string]struct {
		name             string
		devicePathPrefix string
		deviceNames      []string
		getOsFs          func() (afero.Fs, error)
		maxWaitTime      time.Duration
		expectedError    error
	}{
		"Devices removed successfully": {
			devicePathPrefix: "/dev",
			deviceNames:      []string{"sda", "sdb"},
			getOsFs: func() (afero.Fs, error) {
				return afero.NewMemMapFs(), nil
			},
			maxWaitTime:   1 * time.Second,
			expectedError: nil,
		},
		"Timeout waiting for devices to be removed": {
			devicePathPrefix: "/dev",
			deviceNames:      []string{"sda", "sdb"},
			getOsFs: func() (afero.Fs, error) {
				osFs := afero.NewMemMapFs()
				_, err := osFs.Create("/dev/sda")
				if err != nil {
					return nil, err
				}
				_, err = osFs.Create("/dev/sdb")
				if err != nil {
					return nil, err
				}
				return osFs, nil
			},
			maxWaitTime:   1 * time.Second,
			expectedError: errors.TimeoutError(errMsg),
		},
		"Timeout waiting for last device to be removed": {
			devicePathPrefix: "/dev",
			deviceNames:      []string{"sda", "sdb"},
			getOsFs: func() (afero.Fs, error) {
				osFs := afero.NewMemMapFs()
				_, err := osFs.Create("/dev/sdb")
				if err != nil {
					return nil, err
				}
				return osFs, nil
			},
			maxWaitTime:   1 * time.Second,
			expectedError: errors.TimeoutError(errMsg),
		},
		"Timeout waiting for first device to be removed": {
			devicePathPrefix: "/dev",
			deviceNames:      []string{"sda", "sdb"},
			getOsFs: func() (afero.Fs, error) {
				osFs := afero.NewMemMapFs()
				_, err := osFs.Create("/dev/sda")
				if err != nil {
					return nil, err
				}
				return osFs, nil
			},
			maxWaitTime:   1 * time.Second,
			expectedError: errors.TimeoutError(errMsg),
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			fs, err := params.getOsFs()
			assert.NoError(t, err)
			err = waitForDevicesRemoval(context.Background(), fs, params.devicePathPrefix, params.deviceNames,
				params.maxWaitTime)
			if params.expectedError != nil {
				assert.EqualError(t, err, params.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
