// Copyright 2025 NetApp, Inc. All Rights Reserved.

package luks

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_devices"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/models"
)

func TestNewDevice(t *testing.T) {
	luksDevice := NewDevice("/dev/sdb", "pvc-test", nil, nil)
	assert.Equal(t, luksDevice.RawDevicePath(), "/dev/sdb")
	assert.Equal(t, luksDevice.MappedDevicePath(), "/dev/mapper/luks-pvc-test")
	assert.Equal(t, luksDevice.MappedDeviceName(), "luks-pvc-test")
}

func TestIsLegacyDevicePath(t *testing.T) {
	tests := map[string]struct {
		name       string
		devicePath string
		expected   bool
	}{
		"legacy luks device path": {
			devicePath: "/dev/mapper/luks-trident_pvc_4b7874ba_58d7_4d93_8d36_09a09b837f81",
			expected:   true,
		},
		"non-legacy luks device path": {
			devicePath: "/dev/mapper/mpath-36001405b09b0d1f4d0000000000000a1",
			expected:   false,
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, params.expected, IsLegacyDevicePath(params.devicePath))
		})
	}
}

func TestEnsureCryptsetupFormattedAndMappedOnHost(t *testing.T) {
	ctx := context.TODO()

	tests := map[string]struct {
		name          string
		publishInfo   *models.VolumePublishInfo
		secrets       map[string]string
		setupMocks    func(*gomock.Controller) *mockexec.MockCommand
		expectedBool  bool
		expectedError bool
		errorContains string
	}{
		"non-LUKS device returns false": {
			name: "test-volume",
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "false",
				DevicePath:     "/dev/sda",
			},
			secrets: map[string]string{},
			setupMocks: func(ctrl *gomock.Controller) *mockexec.MockCommand {
				return mockexec.NewMockCommand(ctrl)
			},
			expectedBool:  false,
			expectedError: false,
		},
		"empty LUKS encryption field returns false": {
			name: "test-volume",
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "",
				DevicePath:     "/dev/sda",
			},
			secrets: map[string]string{},
			setupMocks: func(ctrl *gomock.Controller) *mockexec.MockCommand {
				return mockexec.NewMockCommand(ctrl)
			},
			expectedBool:  false,
			expectedError: false,
		},
		"invalid LUKS encryption value returns error": {
			name: "test-volume",
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "invalid",
				DevicePath:     "/dev/sda",
			},
			secrets: map[string]string{},
			setupMocks: func(ctrl *gomock.Controller) *mockexec.MockCommand {
				return mockexec.NewMockCommand(ctrl)
			},
			expectedBool:  false,
			expectedError: true,
			errorContains: "could not parse LUKSEncryption into a bool",
		},
		"LUKS device with missing passphrase returns error": {
			name: "test-volume",
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "true",
				DevicePath:     "/dev/sda",
			},
			secrets: map[string]string{
				"luks-passphrase-name": "A",
			},
			setupMocks: func(ctrl *gomock.Controller) *mockexec.MockCommand {
				return mockexec.NewMockCommand(ctrl)
			},
			expectedBool:  false,
			expectedError: true,
			errorContains: "LUKS passphrase cannot be empty",
		},
		"LUKS device with missing passphrase name returns error": {
			name: "test-volume",
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "true",
				DevicePath:     "/dev/sda",
			},
			secrets: map[string]string{
				"luks-passphrase": "secret123",
			},
			setupMocks: func(ctrl *gomock.Controller) *mockexec.MockCommand {
				return mockexec.NewMockCommand(ctrl)
			},
			expectedBool:  false,
			expectedError: true,
			errorContains: "LUKS passphrase name cannot be empty",
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockCommand := params.setupMocks(ctrl)
			mockDevices := mock_devices.NewMockDevices(ctrl)

			luksFormatted, err := EnsureCryptsetupFormattedAndMappedOnHost(
				ctx, params.name, params.publishInfo, params.secrets, mockCommand, mockDevices,
			)

			assert.Equal(t, params.expectedBool, luksFormatted)
			if params.expectedError {
				assert.Error(t, err)
				if params.errorContains != "" {
					assert.Contains(t, err.Error(), params.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsLuksDevice(t *testing.T) {
	tests := map[string]struct {
		publishInfo   *models.VolumePublishInfo
		expected      bool
		expectedError bool
		errorContains string
	}{
		"LUKS encryption set to true": {
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "true",
			},
			expected:      true,
			expectedError: false,
		},
		"LUKS encryption set to false": {
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "false",
			},
			expected:      false,
			expectedError: false,
		},
		"LUKS encryption set to 1": {
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "1",
			},
			expected:      true,
			expectedError: false,
		},
		"LUKS encryption set to 0": {
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "0",
			},
			expected:      false,
			expectedError: false,
		},
		"LUKS encryption empty": {
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "",
			},
			expected:      false,
			expectedError: false,
		},
		"LUKS encryption invalid value": {
			publishInfo: &models.VolumePublishInfo{
				LUKSEncryption: "invalid",
			},
			expected:      false,
			expectedError: true,
			errorContains: "could not parse LUKSEncryption into a bool",
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			isLUKS, err := IsLuksDevice(params.publishInfo)

			assert.Equal(t, params.expected, isLUKS)
			if params.expectedError {
				assert.Error(t, err)
				if params.errorContains != "" {
					assert.Contains(t, err.Error(), params.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetLUKSPassphrasesFromSecretMap(t *testing.T) {
	tests := map[string]struct {
		secrets                        map[string]string
		expectedPassphraseName         string
		expectedPassphrase             string
		expectedPreviousPassphraseName string
		expectedPreviousPassphrase     string
	}{
		"all secrets present": {
			secrets: map[string]string{
				"luks-passphrase":               "current-secret",
				"luks-passphrase-name":          "A",
				"previous-luks-passphrase":      "old-secret",
				"previous-luks-passphrase-name": "B",
			},
			expectedPassphraseName:         "A",
			expectedPassphrase:             "current-secret",
			expectedPreviousPassphraseName: "B",
			expectedPreviousPassphrase:     "old-secret",
		},
		"only current secrets present": {
			secrets: map[string]string{
				"luks-passphrase":      "current-secret",
				"luks-passphrase-name": "A",
			},
			expectedPassphraseName:         "A",
			expectedPassphrase:             "current-secret",
			expectedPreviousPassphraseName: "",
			expectedPreviousPassphrase:     "",
		},
		"empty secrets map": {
			secrets:                        map[string]string{},
			expectedPassphraseName:         "",
			expectedPassphrase:             "",
			expectedPreviousPassphraseName: "",
			expectedPreviousPassphrase:     "",
		},
		"partial secrets": {
			secrets: map[string]string{
				"luks-passphrase": "current-secret",
			},
			expectedPassphraseName:         "",
			expectedPassphrase:             "current-secret",
			expectedPreviousPassphraseName: "",
			expectedPreviousPassphrase:     "",
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			passphraseName, passphrase, prevPassphraseName, prevPassphrase := GetLUKSPassphrasesFromSecretMap(params.secrets)

			assert.Equal(t, params.expectedPassphraseName, passphraseName)
			assert.Equal(t, params.expectedPassphrase, passphrase)
			assert.Equal(t, params.expectedPreviousPassphraseName, prevPassphraseName)
			assert.Equal(t, params.expectedPreviousPassphrase, prevPassphrase)
		})
	}
}

func TestLUKSDevice_MappedDevicePath(t *testing.T) {
	luksDevice := NewDevice("/dev/sda", "pvc-test", nil, nil)
	assert.Equal(t, "/dev/mapper/luks-pvc-test", luksDevice.MappedDevicePath())
}

func TestLUKSDevice_MappedDeviceName(t *testing.T) {
	luksDevice := NewDevice("/dev/sda", "pvc-test", nil, nil)
	assert.Equal(t, "luks-pvc-test", luksDevice.MappedDeviceName())
}

func TestLUKSDevice_RawDevicePath(t *testing.T) {
	luksDevice := NewDevice("/dev/sda", "pvc-test", nil, nil)
	assert.Equal(t, "/dev/sda", luksDevice.RawDevicePath())
}
