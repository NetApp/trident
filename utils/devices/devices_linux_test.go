// Copyright 2024 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for linux flavor

package devices

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_devices"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
)

func mockCryptsetupLuksClose(mock *mockexec.MockCommand) *gomock.Call {
	return mock.EXPECT().ExecuteWithTimeoutAndInput(
		gomock.Any(), "cryptsetup", luksCloseTimeout, true, "", "luksClose", gomock.Any(),
	)
}

func TestEnsureLUKSDeviceClosedWithMaxWaitLimit(t *testing.T) {
	osFs := afero.NewMemMapFs()
	luksDevicePath := "/dev/mapper/luks-test"
	osFs.Create(luksDevicePath)
	client := mockexec.NewMockCommand(gomock.NewController(t))
	deviceClient := NewDetailed(client, osFs, NewDiskSizeGetter())

	type testCase struct {
		name            string
		mockSetup       func(*mockexec.MockCommand)
		expectedError   bool
		expectedErrType error
	}

	testCases := []testCase{
		{
			name: "SucceedsWhenDeviceIsClosed",
			mockSetup: func(mockCommand *mockexec.MockCommand) {
				mockCryptsetupLuksClose(mockCommand).Return([]byte(""), nil)
			},
			expectedError: false,
		},
		{
			name: "FailsBeforeMaxWaitLimit",
			mockSetup: func(mockCommand *mockexec.MockCommand) {
				mockCryptsetupLuksClose(mockCommand).Return([]byte(""), fmt.Errorf("close error"))
			},
			expectedError:   true,
			expectedErrType: fmt.Errorf("%w", errors.New("")),
		},
		{
			name: "FailsWithMaxWaitExceededError",
			mockSetup: func(mockCommand *mockexec.MockCommand) {
				mockCryptsetupLuksClose(mockCommand).Return([]byte(""), fmt.Errorf("close error"))
				LuksCloseDurations[luksDevicePath] = time.Now().Add(-luksCloseMaxWaitDuration - time.Second)
			},
			expectedError:   true,
			expectedErrType: errors.MaxWaitExceededError(""),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.mockSetup(client)
			err := deviceClient.EnsureLUKSDeviceClosedWithMaxWaitLimit(context.TODO(), luksDevicePath)
			if tc.expectedError {
				assert.Error(t, err)
				if tc.expectedErrType != nil {
					assert.IsType(t, tc.expectedErrType, err)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func Test_CloseLUKSDevice(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	deviceClient := NewDetailed(mockCommand, afero.NewMemMapFs(), NewDiskSizeGetter())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksClose(mockCommand).Return([]byte(""), nil)

	err := deviceClient.CloseLUKSDevice(context.Background(), "/dev/sdb")
	assert.NoError(t, err)
}

func TestEnsureLUKSDeviceClosed_DeviceDoesNotExist(t *testing.T) {
	deviceClient := NewDetailed(exec.NewCommand(), afero.NewMemMapFs(), NewDiskSizeGetter())
	err := deviceClient.EnsureLUKSDeviceClosed(context.Background(), "/dev/mapper/luks-test-dev")
	assert.NoError(t, err)
}

func TestEnsureLUKSDeviceClosed_FailsToDetectDevice(t *testing.T) {
	osFs := afero.NewOsFs()
	var b strings.Builder
	b.Grow(1025)
	for i := 0; i < 1025; i++ {
		b.WriteByte('a')
	}
	s := b.String()
	deviceClient := NewDetailed(exec.NewCommand(), osFs, NewDiskSizeGetter())
	err := deviceClient.EnsureLUKSDeviceClosed(context.Background(), "/dev/mapper/"+s)
	assert.Error(t, err)
}

func TestEnsureLUKSDeviceClosed_FailsToCloseDevice(t *testing.T) {
	ctx := context.Background()
	devicePath := "/dev/mapper/luks-test-dev"
	osFs := afero.NewMemMapFs()
	osFs.Create(devicePath)

	deviceClient := NewDetailed(exec.NewCommand(), osFs, NewDiskSizeGetter())
	err := deviceClient.EnsureLUKSDeviceClosed(ctx, devicePath)
	assert.Error(t, err)
}

func TestEnsureLUKSDeviceClosed_ClosesDevice(t *testing.T) {
	ctx := context.Background()
	devicePath := "/dev/mapper/luks-test-dev"
	osFs := afero.NewMemMapFs()
	osFs.Create(devicePath)

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	// Setup mock calls and reassign any clients to their mock counterparts.
	gomock.InOrder(
		mockCryptsetupLuksClose(mockCommand).Return([]byte(""), nil),
	)

	deviceClient := NewDetailed(mockCommand, osFs, NewDiskSizeGetter())
	err := deviceClient.EnsureLUKSDeviceClosed(ctx, devicePath)
	assert.NoError(t, err)
}

func TestFlushOneDevice(t *testing.T) {
	tests := map[string]struct {
		name          string
		devicePath    string
		mockSetup     func(*mockexec.MockCommand)
		expectedError bool
	}{
		"Flush successful": {
			devicePath: "/dev/sda",
			mockSetup: func(mockCommand *mockexec.MockCommand) {
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "blockdev", deviceOperationsTimeout, true, "--flushbufs", "/dev/sda",
				).Return([]byte(""), nil)
			},
			expectedError: false,
		},
		"Flush failed": {
			devicePath: "/dev/sda",
			mockSetup: func(mockCommand *mockexec.MockCommand) {
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "blockdev", deviceOperationsTimeout, true, "--flushbufs", "/dev/sda",
				).Return([]byte(""), fmt.Errorf("flush error"))
			},
			expectedError: true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockCommand := mockexec.NewMockCommand(mockCtrl)
			params.mockSetup(mockCommand)

			client := &Client{
				command: mockCommand,
				osFs:    afero.Afero{Fs: afero.NewMemMapFs()},
			}

			err := client.FlushOneDevice(context.Background(), params.devicePath)
			if params.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetDeviceFSType(t *testing.T) {
	tests := map[string]struct {
		name           string
		device         string
		getMockCommand func() exec.Command
		getMockFs      func() afero.Fs
		expectedFSType string
		expectedError  bool
	}{
		"SuccessfulGetDeviceFSType": {
			device: "/dev/sda",
			getMockCommand: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "blkid", 5*time.Second, true, "/dev/sda",
				).Return([]byte(`TYPE="ext4"`), nil)
				return mockCommand
			},
			getMockFs: func() afero.Fs {
				mockFs := afero.NewMemMapFs()
				mockFs.Create("/dev/sda")
				return mockFs
			},
			expectedFSType: "ext4",
			expectedError:  false,
		},
		"DeviceNotFound": {
			device: "/dev/sdb",
			getMockCommand: func() exec.Command {
				return mockexec.NewMockCommand(gomock.NewController(t))
			},
			getMockFs: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			expectedFSType: "",
			expectedError:  true,
		},
		"UnformattedDevice": {
			device: "/dev/sda",
			getMockCommand: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "blkid", 5*time.Second, true, "/dev/sda",
				).Return([]byte(""), mockexec.NewMockExitError(2, "mock error"))
				return mockCommand
			},
			getMockFs: func() afero.Fs {
				mockFs := afero.NewMemMapFs()
				mockFs.Create("/dev/sda")
				return mockFs
			},
			expectedFSType: "",
			expectedError:  false,
		},
		"UnknownFsType": {
			device: "/dev/sda",
			getMockCommand: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "blkid", 5*time.Second, true, "/dev/sda",
				).Return([]byte("TYPE="), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(
					context.Background(), "dd", 5*time.Second, false, "if=/dev/sda", "bs=4096",
					"count=512", "status=none")
				return mockCommand
			},
			getMockFs: func() afero.Fs {
				mockFs := afero.NewMemMapFs()
				mockFs.Create("/dev/sda")
				return mockFs
			},
			expectedFSType: "",
			expectedError:  true,
		},
		"UnknownFormattedFsType": {
			device: "/dev/sda",
			getMockCommand: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "blkid", 5*time.Second, true, "/dev/sda",
				).Return([]byte("TYPE="), nil)

				dataSize := 2097152
				out := make([]byte, dataSize)
				for i := range dataSize {
					out[i] = 1
				}
				mockCommand.EXPECT().ExecuteWithTimeout(
					context.Background(), "dd", 5*time.Second, false, "if=/dev/sda", "bs=4096",
					"count=512", "status=none").Return(out, nil)
				return mockCommand
			},
			getMockFs: func() afero.Fs {
				mockFs := afero.NewMemMapFs()
				mockFs.Create("/dev/sda")
				return mockFs
			},
			expectedFSType: filesystem.UnknownFstype,
			expectedError:  false,
		},
		"TimeoutError": {
			device: "/dev/sda",
			getMockCommand: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "blkid", 5*time.Second, true, "/dev/sda",
				).Return(nil, errors.TimeoutError("timeout"))
				return mockCommand
			},
			getMockFs: func() afero.Fs {
				mockFs := afero.NewMemMapFs()
				mockFs.Create("/dev/sda")
				return mockFs
			},
			expectedFSType: "",
			expectedError:  true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			client := &Client{
				command: params.getMockCommand(),
				osFs:    afero.Afero{Fs: params.getMockFs()},
			}

			fsType, err := client.GetDeviceFSType(context.Background(), params.device)
			if params.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, params.expectedFSType, fsType)
			}
		})
	}
}

func TestClient_verifyMultipathDeviceSize(t *testing.T) {
	type parameters struct {
		getDevicesClient      func(controller *gomock.Controller) Devices
		getMockDiskSizeGetter func(controller *gomock.Controller) SizeGetter
		assertError           assert.ErrorAssertionFunc
		assertValid           assert.BoolAssertionFunc
		expectedDeviceSize    int64
	}

	const deviceName = "sda"
	const multipathDeviceName = "dm-0"

	tests := map[string]parameters{
		"error getting device size": {
			getMockDiskSizeGetter: func(controller *gomock.Controller) SizeGetter {
				mockGetter := mock_devices.NewMockSizeGetter(controller)
				mockGetter.EXPECT().GetDiskSize(context.TODO(), gomock.Any()).Return(int64(0),
					errors.New("some error"))
				return mockGetter
			},
			assertError:        assert.Error,
			assertValid:        assert.False,
			expectedDeviceSize: 0,
		},
		"error getting multipath device size": {
			getMockDiskSizeGetter: func(controller *gomock.Controller) SizeGetter {
				mockGetter := mock_devices.NewMockSizeGetter(controller)
				mockGetter.EXPECT().GetDiskSize(context.TODO(), gomock.Any()).Return(int64(1), nil)
				mockGetter.EXPECT().GetDiskSize(context.TODO(), gomock.Any()).Return(int64(0),
					errors.New("some error"))
				return mockGetter
			},
			assertError:        assert.Error,
			assertValid:        assert.False,
			expectedDeviceSize: 0,
		},
		"device size != multipath device size": {
			getMockDiskSizeGetter: func(controller *gomock.Controller) SizeGetter {
				mockGetter := mock_devices.NewMockSizeGetter(controller)
				mockGetter.EXPECT().GetDiskSize(context.TODO(), gomock.Any()).Return(int64(1), nil)
				mockGetter.EXPECT().GetDiskSize(context.TODO(), gomock.Any()).Return(int64(0), nil)
				return mockGetter
			},
			assertError:        assert.NoError,
			assertValid:        assert.False,
			expectedDeviceSize: 1,
		},
		"happy path": {
			getMockDiskSizeGetter: func(controller *gomock.Controller) SizeGetter {
				mockGetter := mock_devices.NewMockSizeGetter(controller)
				mockGetter.EXPECT().GetDiskSize(context.TODO(), gomock.Any()).Return(int64(1), nil)
				mockGetter.EXPECT().GetDiskSize(context.TODO(), gomock.Any()).Return(int64(1), nil)
				return mockGetter
			},
			assertError:        assert.NoError,
			assertValid:        assert.True,
			expectedDeviceSize: 0,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewDetailed(exec.NewCommand(), afero.NewMemMapFs(), params.getMockDiskSizeGetter(ctrl))
			deviceSize, valid, err := client.VerifyMultipathDeviceSize(context.TODO(), multipathDeviceName, deviceName)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			if params.assertValid != nil {
				params.assertValid(t, valid)
			}
			assert.Equal(t, params.expectedDeviceSize, deviceSize)
		})
	}
}

func TestMultipathFlushDevice(t *testing.T) {
	devicePath := "/dev/dm-0"
	tests := map[string]struct {
		getMockCmd  func() exec.Command
		deviceInfo  *models.ScsiDeviceInfo
		expectError bool
	}{
		"Happy Path": {
			getMockCmd: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "multipath", 5*time.Second, true,
					"-C", devicePath).Return([]byte(""), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "blockdev", 5*time.Second, true, "--flushbufs",
					devicePath).Return([]byte(""), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "multipath", 10*time.Second, false, "-f",
					devicePath).Return([]byte(""), nil)
				return mockCommand
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
			},
			expectError: false,
		},
		"Missing MultipathDevice": {
			getMockCmd: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				return mockCommand
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "",
			},
			expectError: false,
		},
		"Can Flush Error": {
			getMockCmd: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "multipath", 5*time.Second, true,
					"-C", "/dev/dm-0").Return([]byte(""), errors.ISCSIDeviceFlushError("error"))
				return mockCommand
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
			},
			expectError: true,
		},
		"Timeout Error": {
			getMockCmd: func() exec.Command {
				volumeFlushExceptions[devicePath] = time.Now().Add(-1 * time.Hour)
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "multipath", 5*time.Second, true,
					"-C", "/dev/dm-0").Return([]byte("no usable paths found"), fmt.Errorf("error"))
				return mockCommand
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
			},
			expectError: true,
		},
		"Flush Error": {
			getMockCmd: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "multipath", 5*time.Second, true,
					"-C", devicePath).Return([]byte(""), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "blockdev", 5*time.Second, true, "--flushbufs",
					devicePath).Return([]byte(""), fmt.Errorf("error"))
				return mockCommand
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
			},
			expectError: true,
		},
		"Multipath Remove Error": {
			getMockCmd: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "multipath", 5*time.Second, true,
					"-C", devicePath).Return([]byte(""), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "blockdev", 5*time.Second, true, "--flushbufs",
					devicePath).Return([]byte(""), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "multipath", 10*time.Second, false, "-f",
					devicePath).Return([]byte(""), fmt.Errorf("error"))
				return mockCommand
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
			},
			expectError: true,
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			deviceClient := NewDetailed(params.getMockCmd(), afero.NewMemMapFs(), NewDiskSizeGetter())
			err := deviceClient.MultipathFlushDevice(context.TODO(), params.deviceInfo)
			delete(volumeFlushExceptions, devicePath)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFlushDevice(t *testing.T) {
	device := "sda"
	tests := map[string]struct {
		getMockCmd  func() exec.Command
		deviceInfo  *models.ScsiDeviceInfo
		force       bool
		expectError bool
	}{
		"Happy Path": {
			deviceInfo: &models.ScsiDeviceInfo{
				Devices: []string{device},
			},
			getMockCmd: func() exec.Command {
				mockCmd := mockexec.NewMockCommand(gomock.NewController(t))
				mockCmd.EXPECT().ExecuteWithTimeout(gomock.Any(), "blockdev", 5*time.Second, true, "--flushbufs",
					DevPrefix+device).Return([]byte(""), nil)
				return mockCmd
			},
			expectError: false,
		},
		"No Multipath Device Error": {
			deviceInfo: &models.ScsiDeviceInfo{
				Devices: []string{device},
			},
			getMockCmd: func() exec.Command {
				mockCmd := mockexec.NewMockCommand(gomock.NewController(t))
				mockCmd.EXPECT().ExecuteWithTimeout(gomock.Any(), "blockdev", 5*time.Second, true, "--flushbufs",
					DevPrefix+device).Return([]byte(""), fmt.Errorf("error"))
				return mockCmd
			},
			expectError: true,
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			deviceClient := NewDetailed(params.getMockCmd(), afero.NewMemMapFs(), NewDiskSizeGetter())
			err := deviceClient.FlushDevice(context.TODO(), params.deviceInfo, params.force)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
