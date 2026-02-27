// Copyright 2026 NetApp, Inc. All Rights Reserved.

// NOTE: This file should only contain functions for handling devices for linux flavor

package devices

import (
	"context"
	"fmt"
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
			name: "SucceedsWhenDeviceIsAlreadyClosed",
			mockSetup: func(mockCommand *mockexec.MockCommand) {
				mockCryptsetupLuksClose(mockCommand).Return(
					[]byte(""),
					mockexec.NewMockExitError(luksCloseDeviceAlreadyClosedExitCode, "mock error"),
				)
			},
			expectedError: false,
		},
		{
			name: "SucceedsWhenDeviceExecCmdReturnsErrorButLuksCloseHadExitCode0",
			mockSetup: func(mockCommand *mockexec.MockCommand) {
				mockCryptsetupLuksClose(mockCommand).Return(
					[]byte(""),
					mockexec.NewMockExitError(luksCloseDeviceAlreadyClosedExitCode, "mock error"),
				)
			},
			expectedError: false,
		},
		{
			name: "FailsWhenLUKSCloseFailsWithUnexpectedExitCode1",
			mockSetup: func(mockCommand *mockexec.MockCommand) {
				mockCryptsetupLuksClose(mockCommand).Return([]byte(""), mockexec.NewMockExitError(1, "mock error"))
			},
			expectedError:   true,
			expectedErrType: fmt.Errorf("%w", errors.New("")),
		},
		{
			name: "FailsBeforeMaxWaitLimit",
			mockSetup: func(mockCommand *mockexec.MockCommand) {
				mockCryptsetupLuksClose(mockCommand).Return([]byte(""), errors.New("close error"))
			},
			expectedError:   true,
			expectedErrType: fmt.Errorf("%w", errors.New("")),
		},
		{
			name: "FailsWithMaxWaitExceededError",
			mockSetup: func(mockCommand *mockexec.MockCommand) {
				mockCryptsetupLuksClose(mockCommand).Return([]byte(""), errors.New("close error"))
				LuksCloseDurations[luksDevicePath] = time.Now().Add(-luksCloseMaxWaitDuration - time.Second)
			},
			expectedError:   true,
			expectedErrType: errors.MaxWaitExceededError(""),
		},
	}

	for _, tc := range testCases {
		t.Run(
			tc.name, func(t *testing.T) {
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
			},
		)
	}
}

func TestCloseLUKSDevice(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	deviceClient := NewDetailed(mockCommand, afero.NewMemMapFs(), NewDiskSizeGetter())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksClose(mockCommand).Return([]byte(""), nil)

	err := deviceClient.CloseLUKSDevice(context.Background(), "/dev/sdb")
	assert.NoError(t, err)
}

func TestCloseLUKSDevice_DeviceAlreadyClosedOrDoesNotExist(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	deviceClient := NewDetailed(mockCommand, afero.NewMemMapFs(), NewDiskSizeGetter())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksClose(mockCommand).Return(
		[]byte(""),
		mockexec.NewMockExitError(luksCloseDeviceAlreadyClosedExitCode, "closed or doesn't exist"),
	)

	err := deviceClient.CloseLUKSDevice(context.Background(), "/dev/sdb")
	assert.NoError(t, err)

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksClose(mockCommand).Return(
		[]byte(""),
		mockexec.NewMockExitError(luksCloseDeviceSafelyClosedExitCode, "succeeds but has exec error"),
	)

	err = deviceClient.CloseLUKSDevice(context.Background(), "/dev/sdb")
	assert.NoError(t, err)
}

func TestCloseLUKSDevice_DeviceIsBusy(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	deviceClient := NewDetailed(mockCommand, afero.NewMemMapFs(), NewDiskSizeGetter())

	// Setup mock calls and reassign any clients to their mock counterparts.
	mockCryptsetupLuksClose(mockCommand).Return([]byte(""), mockexec.NewMockExitError(5, "busy device"))

	err := deviceClient.CloseLUKSDevice(context.Background(), "/dev/sdb")
	assert.Error(t, err)
}

func TestEnsureLUKSDeviceClosed_DeviceDoesNotExist(t *testing.T) {
	deviceClient := NewDetailed(exec.NewCommand(), afero.NewMemMapFs(), NewDiskSizeGetter())
	err := deviceClient.EnsureLUKSDeviceClosed(context.Background(), "/dev/mapper/luks-test-dev")
	assert.NoError(t, err)
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

func TestEnsureLUKSDeviceClosed_FailsToDetectDevice(t *testing.T) {
	ctx := context.Background()
	devicePath := "/dev/mapper/luks-test-dev"
	osFs := afero.NewOsFs()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	// Setup mock calls and reassign any clients to their mock counterparts.
	gomock.InOrder(
		mockCryptsetupLuksClose(mockCommand).Return([]byte(""), mockexec.NewMockExitError(4, "device not found")),
	)

	deviceClient := NewDetailed(mockCommand, osFs, NewDiskSizeGetter())
	err := deviceClient.EnsureLUKSDeviceClosed(ctx, devicePath)
	assert.NoError(t, err)
}

func TestEnsureLUKSDeviceClosed_FailsToCloseDeviceWithBusyDevice(t *testing.T) {
	ctx := context.Background()
	devicePath := "/dev/mapper/luks-test-dev"
	osFs := afero.NewMemMapFs()
	osFs.Create(devicePath)

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)

	// Setup mock calls and reassign any clients to their mock counterparts.
	gomock.InOrder(
		mockCryptsetupLuksClose(mockCommand).Return([]byte(""), mockexec.NewMockExitError(5, "busy device")),
	)
	deviceClient := NewDetailed(mockCommand, osFs, NewDiskSizeGetter())
	err := deviceClient.EnsureLUKSDeviceClosed(ctx, devicePath)
	assert.Error(t, err)
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
				).Return([]byte(""), errors.New("flush error"))
			},
			expectedError: true,
		},
	}

	for name, params := range tests {
		t.Run(
			name, func(t *testing.T) {
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
			},
		)
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
					"count=512", "status=none",
				)
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
					"count=512", "status=none",
				).Return(out, nil)
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
		t.Run(
			name, func(t *testing.T) {
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
			},
		)
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
				mockGetter.EXPECT().GetDiskSize(context.TODO(), gomock.Any()).Return(
					int64(0), errors.New("get size error"),
				)
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
				mockGetter.EXPECT().GetDiskSize(context.TODO(), gomock.Any()).Return(
					int64(0), errors.New("get size error"),
				)
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
		t.Run(
			name, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				client := NewDetailed(exec.NewCommand(), afero.NewMemMapFs(), params.getMockDiskSizeGetter(ctrl))
				deviceSize, valid, err := client.VerifyMultipathDeviceSize(
					context.TODO(), multipathDeviceName, deviceName,
				)
				if params.assertError != nil {
					params.assertError(t, err)
				}
				if params.assertValid != nil {
					params.assertValid(t, valid)
				}
				assert.Equal(t, params.expectedDeviceSize, deviceSize)
			},
		)
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
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "multipath", 5*time.Second, true, "-C", devicePath,
				).Return([]byte(""), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "blockdev", 5*time.Second, true, "--flushbufs", devicePath,
				).Return([]byte(""), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "multipath", 10*time.Second, false, "-f", devicePath,
				).Return([]byte(""), nil)
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
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "multipath", 5*time.Second, true, "-C", "/dev/dm-0",
				).Return([]byte(""), errors.ISCSIDeviceFlushError("error"))
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
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "multipath", 5*time.Second, true, "-C", "/dev/dm-0",
				).Return([]byte("no usable paths found"), errors.New("error"))
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
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "multipath", 5*time.Second, true, "-C", devicePath,
				).Return([]byte(""), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "blockdev", 5*time.Second, true, "--flushbufs", devicePath,
				).Return([]byte(""), errors.New("error"))
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
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "multipath", 5*time.Second, true, "-C", devicePath,
				).Return([]byte(""), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "blockdev", 5*time.Second, true, "--flushbufs", devicePath,
				).Return([]byte(""), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "multipath", 10*time.Second, false, "-f", devicePath,
				).Return([]byte(""), errors.New("error"))
				return mockCommand
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
			},
			expectError: true,
		},
	}
	for name, params := range tests {
		t.Run(
			name, func(t *testing.T) {
				deviceClient := NewDetailed(params.getMockCmd(), afero.NewMemMapFs(), NewDiskSizeGetter())
				err := deviceClient.MultipathFlushDevice(context.TODO(), params.deviceInfo)
				delete(volumeFlushExceptions, devicePath)
				if params.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			},
		)
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
				mockCmd.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "blockdev", 5*time.Second, true, "--flushbufs", DevPrefix+device,
				).Return([]byte(""), nil)
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
				mockCmd.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "blockdev", 5*time.Second, true, "--flushbufs", DevPrefix+device,
				).Return([]byte(""), errors.New("error"))
				return mockCmd
			},
			expectError: true,
		},
	}
	for name, params := range tests {
		t.Run(
			name, func(t *testing.T) {
				deviceClient := NewDetailed(params.getMockCmd(), afero.NewMemMapFs(), NewDiskSizeGetter())
				err := deviceClient.FlushDevice(context.TODO(), params.deviceInfo, params.force)
				if params.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			},
		)
	}
}

func TestClient_deviceState(t *testing.T) {
	tests := map[string]struct {
		deviceName    string
		setupFs       func(afero.Fs)
		expectedState string
		assertError   assert.ErrorAssertionFunc
	}{
		"happy path: running": {
			deviceName: "sda",
			setupFs: func(fs afero.Fs) {
				fs.MkdirAll("/sys/block/sda/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)
			},
			expectedState: "running",
			assertError:   assert.NoError,
		},
		"transport-offline": {
			deviceName: "sdc",
			setupFs: func(fs afero.Fs) {
				fs.MkdirAll("/sys/block/sdc/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sdc/device/state", []byte("transport-offline\n"), 0o444)
			},
			expectedState: "transport-offline",
			assertError:   assert.NoError,
		},
		"blocked": {
			deviceName: "sdb",
			setupFs: func(fs afero.Fs) {
				fs.MkdirAll("/sys/block/sdb/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sdb/device/state", []byte("blocked\n"), 0o444)
			},
			expectedState: "blocked",
			assertError:   assert.NoError,
		},
		"trims whitespace": {
			deviceName: "sda",
			setupFs: func(fs afero.Fs) {
				fs.MkdirAll("/sys/block/sda/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("  running  \n"), 0o444)
			},
			expectedState: "running",
			assertError:   assert.NoError,
		},
		"state file does not exist": {
			deviceName:  "sda",
			setupFs:     func(fs afero.Fs) {},
			assertError: assert.Error,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			tc.setupFs(fs)
			client := &Client{
				osFs: afero.Afero{Fs: fs},
			}

			state, err := client.deviceState(context.Background(), tc.deviceName)
			tc.assertError(t, err)
			assert.Equal(t, tc.expectedState, state)
		})
	}
}

// --- deviceSize tests ---

func TestClient_deviceSize(t *testing.T) {
	tests := map[string]struct {
		deviceName    string
		setupFs       func(afero.Fs)
		expectedBytes int64
		assertError   assert.ErrorAssertionFunc
	}{
		"happy path: 100G": {
			deviceName: "dm-0",
			setupFs: func(fs afero.Fs) {
				// 209715200 sectors * 512 = 107374182400 bytes (100 GiB)
				afero.WriteFile(fs, "/sys/block/dm-0/size", []byte("209715200\n"), 0o444)
			},
			expectedBytes: 107374182400,
			assertError:   assert.NoError,
		},
		"strips /dev/ prefix": {
			deviceName: "/dev/dm-0",
			setupFs: func(fs afero.Fs) {
				afero.WriteFile(fs, "/sys/block/dm-0/size", []byte("2048\n"), 0o444)
			},
			expectedBytes: 2048 * 512,
			assertError:   assert.NoError,
		},
		"empty device name": {
			deviceName:  "",
			setupFs:     func(fs afero.Fs) {},
			assertError: assert.Error,
		},
		"device name is only /dev/": {
			deviceName:  "/dev/",
			setupFs:     func(fs afero.Fs) {},
			assertError: assert.Error,
		},
		"sysfs file does not exist": {
			deviceName:  "dm-99",
			setupFs:     func(fs afero.Fs) {},
			assertError: assert.Error,
		},
		"sysfs file contains non-numeric data": {
			deviceName: "dm-0",
			setupFs: func(fs afero.Fs) {
				afero.WriteFile(fs, "/sys/block/dm-0/size", []byte("not-a-number\n"), 0o444)
			},
			assertError: assert.Error,
		},
		"sysfs file contains zero sectors": {
			deviceName: "dm-0",
			setupFs: func(fs afero.Fs) {
				afero.WriteFile(fs, "/sys/block/dm-0/size", []byte("0\n"), 0o444)
			},
			assertError: assert.Error,
		},
		"sysfs file contains negative sectors": {
			deviceName: "dm-0",
			setupFs: func(fs afero.Fs) {
				afero.WriteFile(fs, "/sys/block/dm-0/size", []byte("-100\n"), 0o444)
			},
			assertError: assert.Error,
		},
		"trims whitespace": {
			deviceName: "sda",
			setupFs: func(fs afero.Fs) {
				afero.WriteFile(fs, "/sys/block/sda/size", []byte("  1024  \n"), 0o444)
			},
			expectedBytes: 1024 * 512,
			assertError:   assert.NoError,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			tc.setupFs(fs)
			client := &Client{
				osFs: afero.Afero{Fs: fs},
			}

			size, err := client.deviceSize(context.Background(), tc.deviceName)
			tc.assertError(t, err)
			assert.Equal(t, tc.expectedBytes, size)
		})
	}
}

// --- rescanDevice tests ---

func TestClient_rescanDevice(t *testing.T) {
	tests := map[string]struct {
		deviceName  string
		setupFs     func(afero.Fs)
		assertError assert.ErrorAssertionFunc
		verifyFs    func(*testing.T, afero.Fs)
	}{
		"happy path: writes 1 to rescan file": {
			deviceName: "sda",
			setupFs: func(fs afero.Fs) {
				// Create the directory structure and the rescan file.
				fs.MkdirAll("/sys/block/sda/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sda/device/rescan", []byte(""), 0o200)
			},
			assertError: assert.NoError,
			verifyFs: func(t *testing.T, fs afero.Fs) {
				content, err := afero.ReadFile(fs, "/sys/block/sda/device/rescan")
				assert.NoError(t, err)
				assert.Equal(t, "1", string(content))
			},
		},
		"strips /dev/ prefix": {
			deviceName: "/dev/sda",
			setupFs: func(fs afero.Fs) {
				fs.MkdirAll("/sys/block/sda/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sda/device/rescan", []byte(""), 0o200)
			},
			assertError: assert.NoError,
			verifyFs: func(t *testing.T, fs afero.Fs) {
				content, err := afero.ReadFile(fs, "/sys/block/sda/device/rescan")
				assert.NoError(t, err)
				assert.Equal(t, "1", string(content))
			},
		},
		"empty device name": {
			deviceName:  "",
			setupFs:     func(fs afero.Fs) {},
			assertError: assert.Error,
		},
		"device name is only /dev/": {
			deviceName:  "/dev/",
			setupFs:     func(fs afero.Fs) {},
			assertError: assert.Error,
		},
		"rescan file does not exist": {
			deviceName:  "sdb",
			setupFs:     func(fs afero.Fs) {},
			assertError: assert.Error,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			tc.setupFs(fs)
			client := &Client{
				osFs: afero.Afero{Fs: fs},
			}

			err := client.rescanDevice(context.Background(), tc.deviceName)
			tc.assertError(t, err)
			if tc.verifyFs != nil {
				tc.verifyFs(t, fs)
			}
		})
	}
}

// --- rescanUndersizedDevices tests ---

func TestClient_rescanUndersizedDevices(t *testing.T) {
	const targetSizeBytes = int64(107374182400) // 100 GiB

	tests := map[string]struct {
		devices         []string
		targetSizeBytes int64
		setupFs         func(afero.Fs)
		assertError     assert.ErrorAssertionFunc
	}{
		"nil devices": {
			devices:         nil,
			targetSizeBytes: targetSizeBytes,
			setupFs:         func(fs afero.Fs) {},
			assertError:     assert.Error,
		},
		"zero target size": {
			devices:         []string{"sda"},
			targetSizeBytes: 0,
			setupFs:         func(fs afero.Fs) {},
			assertError:     assert.Error,
		},
		"negative target size": {
			devices:         []string{"sda"},
			targetSizeBytes: -1,
			setupFs:         func(fs afero.Fs) {},
			assertError:     assert.Error,
		},
		"all devices already at target size": {
			devices:         []string{"sda", "sdb"},
			targetSizeBytes: targetSizeBytes,
			setupFs: func(fs afero.Fs) {
				// 209715200 sectors * 512 = 107374182400 bytes = targetSizeBytes
				afero.WriteFile(fs, "/sys/block/sda/size", []byte("209715200\n"), 0o444)
				afero.WriteFile(fs, "/sys/block/sdb/size", []byte("209715200\n"), 0o444)
			},
			assertError: assert.NoError,
		},
		"one device undersized, rescan succeeds": {
			devices:         []string{"sda", "sdb"},
			targetSizeBytes: targetSizeBytes,
			setupFs: func(fs afero.Fs) {
				afero.WriteFile(fs, "/sys/block/sda/size", []byte("209715200\n"), 0o444)
				// sdb undersized with rescan file and state file
				afero.WriteFile(fs, "/sys/block/sdb/size", []byte("1024\n"), 0o444)
				fs.MkdirAll("/sys/block/sdb/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sdb/device/rescan", []byte(""), 0o200)
				afero.WriteFile(fs, "/sys/block/sdb/device/state", []byte("running\n"), 0o444)
			},
			assertError: assert.NoError,
		},
		"all devices undersized, all rescans fail": {
			devices:         []string{"sda"},
			targetSizeBytes: targetSizeBytes,
			setupFs: func(fs afero.Fs) {
				// sda undersized, but no rescan file → rescan will fail
				afero.WriteFile(fs, "/sys/block/sda/size", []byte("1024\n"), 0o444)
			},
			assertError: assert.Error,
		},
		"empty devices list": {
			devices:         []string{},
			targetSizeBytes: targetSizeBytes,
			setupFs:         func(fs afero.Fs) {},
			assertError:     assert.Error,
		},
		"device size unreadable but rescan succeeds": {
			devices:         []string{"sda", "sdb"},
			targetSizeBytes: targetSizeBytes,
			setupFs: func(fs afero.Fs) {
				// sda: no size file → treated as undersized, rescan file present
				fs.MkdirAll("/sys/block/sda/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sda/device/rescan", []byte(""), 0o200)
				// sdb: at target size
				afero.WriteFile(fs, "/sys/block/sdb/size", []byte("209715200\n"), 0o444)
			},
			assertError: assert.NoError,
		},
		"partial failure: some rescans succeed, some fail": {
			devices:         []string{"sda", "sdb"},
			targetSizeBytes: targetSizeBytes,
			setupFs: func(fs afero.Fs) {
				// sda: undersized, rescan file present → succeeds
				afero.WriteFile(fs, "/sys/block/sda/size", []byte("1024\n"), 0o444)
				fs.MkdirAll("/sys/block/sda/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sda/device/rescan", []byte(""), 0o200)
				afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)
				// sdb: undersized, no rescan file → fails
				afero.WriteFile(fs, "/sys/block/sdb/size", []byte("1024\n"), 0o444)
				fs.MkdirAll("/sys/block/sdb/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sdb/device/state", []byte("transport-offline\n"), 0o444)
			},
			assertError: assert.NoError,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			tc.setupFs(fs)

			client := &Client{
				osFs: afero.Afero{Fs: fs},
			}

			err := client.rescanUndersizedDevices(context.Background(), tc.devices, tc.targetSizeBytes)
			tc.assertError(t, err)
		})
	}
}

// --- isRunning tests ---

func TestClient_isRunning(t *testing.T) {
	tests := map[string]struct {
		deviceName string
		setupFs    func(afero.Fs)
		expected   bool
	}{
		"running — healthy": {
			deviceName: "sda",
			setupFs: func(fs afero.Fs) {
				fs.MkdirAll("/sys/block/sda/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)
			},
			expected: true,
		},
		"blocked — unhealthy": {
			deviceName: "sda",
			setupFs: func(fs afero.Fs) {
				fs.MkdirAll("/sys/block/sda/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("blocked\n"), 0o444)
			},
			expected: false,
		},
		"transport-offline — unhealthy": {
			deviceName: "sda",
			setupFs: func(fs afero.Fs) {
				fs.MkdirAll("/sys/block/sda/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("transport-offline\n"), 0o444)
			},
			expected: false,
		},
		"offline — unhealthy": {
			deviceName: "sda",
			setupFs: func(fs afero.Fs) {
				fs.MkdirAll("/sys/block/sda/device", 0o755)
				afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("offline\n"), 0o444)
			},
			expected: false,
		},
		"state file missing — treated as unhealthy": {
			deviceName: "sda",
			setupFs:    func(fs afero.Fs) {},
			expected:   false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			tc.setupFs(fs)
			client := &Client{osFs: afero.Afero{Fs: fs}}
			result := client.isRunning(context.Background(), tc.deviceName)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// --- discoverUnhealthyDevices tests ---

func TestClient_discoverUnhealthyDevices(t *testing.T) {
	tests := map[string]struct {
		devices  []string
		setupFs  func(afero.Fs)
		expected []string
	}{
		"all devices healthy": {
			devices: []string{"sda", "sdb"},
			setupFs: func(fs afero.Fs) {
				afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)
				afero.WriteFile(fs, "/sys/block/sdb/device/state", []byte("running\n"), 0o444)
			},
			expected: []string{},
		},
		"all devices unhealthy": {
			devices: []string{"sda", "sdb"},
			setupFs: func(fs afero.Fs) {
				afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("blocked\n"), 0o444)
				afero.WriteFile(fs, "/sys/block/sdb/device/state", []byte("transport-offline\n"), 0o444)
			},
			expected: []string{"sda", "sdb"},
		},
		"mixed healthy and unhealthy": {
			devices: []string{"sda", "sdb"},
			setupFs: func(fs afero.Fs) {
				afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)
				afero.WriteFile(fs, "/sys/block/sdb/device/state", []byte("blocked\n"), 0o444)
			},
			expected: []string{"sdb"},
		},
		"state file missing — treated as unhealthy": {
			devices: []string{"sda"},
			setupFs: func(fs afero.Fs) {},
			// No state file → isRunning returns false.
			expected: []string{"sda"},
		},
		"empty device list": {
			devices:  []string{},
			setupFs:  func(fs afero.Fs) {},
			expected: []string{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			tc.setupFs(fs)
			client := &Client{osFs: afero.Afero{Fs: fs}}
			result := client.discoverUnhealthyDevices(context.Background(), tc.devices)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// --- resizeMultipathMap tests ---

func TestClient_resizeMultipathMap(t *testing.T) {
	const mapperName = "3600a098038314865515d4c5a70644636"

	tests := map[string]struct {
		mockSetup   func(*mockexec.MockCommand)
		assertError assert.ErrorAssertionFunc
	}{
		"resize succeeds": {
			mockSetup: func(m *mockexec.MockCommand) {
				m.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "multipathd", 10*time.Second, true,
					"-kresize map "+mapperName,
				).Return([]byte("ok\n"), nil)
			},
			assertError: assert.NoError,
		},
		"exec error": {
			mockSetup: func(m *mockexec.MockCommand) {
				m.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "multipathd", 10*time.Second, true,
					"-kresize map "+mapperName,
				).Return([]byte(""), errors.New("exec failed"))
			},
			assertError: assert.Error,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockCmd := mockexec.NewMockCommand(ctrl)
			tc.mockSetup(mockCmd)

			client := &Client{
				command: mockCmd,
				osFs:    afero.Afero{Fs: afero.NewMemMapFs()},
			}

			err := client.resizeMultipathMap(context.Background(), mapperName)
			tc.assertError(t, err)
		})
	}
}

// --- getDeviceMapperName tests ---

func TestClient_getDeviceMapperName(t *testing.T) {
	tests := map[string]struct {
		device       string
		setupFs      func(afero.Fs)
		expectedName string
		assertError  assert.ErrorAssertionFunc
	}{
		"happy path": {
			device: "dm-0",
			setupFs: func(fs afero.Fs) {
				fs.MkdirAll("/sys/block/dm-0/dm", 0o755)
				afero.WriteFile(fs, "/sys/block/dm-0/dm/name", []byte("3600a098038314865515d4c5a70644636\n"), 0o444)
			},
			expectedName: "3600a098038314865515d4c5a70644636",
			assertError:  assert.NoError,
		},
		"trims whitespace": {
			device: "dm-0",
			setupFs: func(fs afero.Fs) {
				fs.MkdirAll("/sys/block/dm-0/dm", 0o755)
				afero.WriteFile(fs, "/sys/block/dm-0/dm/name", []byte("  mapname  \n"), 0o444)
			},
			expectedName: "mapname",
			assertError:  assert.NoError,
		},
		"dm/name file does not exist": {
			device:      "dm-0",
			setupFs:     func(fs afero.Fs) {},
			assertError: assert.Error,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			tc.setupFs(fs)
			client := &Client{osFs: afero.Afero{Fs: fs}}
			result, err := client.getDeviceMapperName(context.Background(), tc.device)
			tc.assertError(t, err)
			assert.Equal(t, tc.expectedName, result)
		})
	}
}

// --- ExpandMultipathDevice tests ---

func TestClient_ExpandMultipathDevice(t *testing.T) {
	const (
		targetSizeBytes = int64(107374182400) // 100 GiB
		sectorCount     = "209715200"         // 100 GiB in 512-byte sectors
		smallSectors    = "1024"              // much smaller than target
	)

	makeDeviceInfo := func(mpathDevice string, devices []string) *models.ScsiDeviceInfo {
		return &models.ScsiDeviceInfo{
			ScsiDeviceAddress: models.ScsiDeviceAddress{LUN: "1"},
			MultipathDevice:   mpathDevice,
			Devices:           devices,
		}
	}

	t.Run("nil getter", func(t *testing.T) {
		client := &Client{}
		err := client.ExpandMultipathDevice(context.Background(), nil, targetSizeBytes)
		assert.Error(t, err)
	})

	t.Run("target size zero", func(t *testing.T) {
		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return makeDeviceInfo("dm-0", []string{"sda"}), nil
		}
		client := &Client{}
		err := client.ExpandMultipathDevice(context.Background(), getter, 0)
		assert.Error(t, err)
	})

	t.Run("target size negative", func(t *testing.T) {
		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return makeDeviceInfo("dm-0", []string{"sda"}), nil
		}
		client := &Client{}
		err := client.ExpandMultipathDevice(context.Background(), getter, -100)
		assert.Error(t, err)
	})

	t.Run("already at target size converges quickly", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockCmd := mockexec.NewMockCommand(ctrl)
		fs := afero.NewMemMapFs()

		// All sizes at target.
		afero.WriteFile(fs, "/sys/block/sda/size", []byte(sectorCount+"\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/dm-0/size", []byte(sectorCount+"\n"), 0o444)

		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return makeDeviceInfo("dm-0", []string{"sda"}), nil
		}

		client := &Client{
			command: mockCmd,
			osFs:    afero.Afero{Fs: fs},
		}

		// Needs >6s: 3 stable reads × 3s interval, first fires immediately.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.NoError(t, err)
	})

	t.Run("converges after resize", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockCmd := mockexec.NewMockCommand(ctrl)
		fs := afero.NewMemMapFs()

		const mapperName = "3600a098038314865515d4c5a70644636"

		// Path device undersized initially.
		afero.WriteFile(fs, "/sys/block/sda/size", []byte(smallSectors+"\n"), 0o444)
		fs.MkdirAll("/sys/block/sda/device", 0o755)
		afero.WriteFile(fs, "/sys/block/sda/device/rescan", []byte(""), 0o200)
		afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)
		// Multipath device undersized initially; dm/name needed for resizeMultipathMap.
		afero.WriteFile(fs, "/sys/block/dm-0/size", []byte(smallSectors+"\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/dm-0/dm/name", []byte(mapperName+"\n"), 0o444)

		resizeCount := 0
		mockCmd.EXPECT().ExecuteWithTimeout(
			gomock.Any(), "multipathd", 10*time.Second, true, "-kresize map "+mapperName,
		).DoAndReturn(func(_ context.Context, _ string, _ time.Duration, _ bool, _ ...string) ([]byte, error) {
			resizeCount++
			// After first resize, update the sizes to target.
			afero.WriteFile(fs, "/sys/block/sda/size", []byte(sectorCount+"\n"), 0o444)
			afero.WriteFile(fs, "/sys/block/dm-0/size", []byte(sectorCount+"\n"), 0o444)
			return []byte("ok\n"), nil
		}).AnyTimes()

		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return makeDeviceInfo("dm-0", []string{"sda"}), nil
		}

		client := &Client{
			command: mockCmd,
			osFs:    afero.Afero{Fs: fs},
		}

		// Needs >9s: resize fires at t=0, then 3 stable reads × 3s = 9s total.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.NoError(t, err)
		assert.GreaterOrEqual(t, resizeCount, 1, "should have called multipathd resize at least once")
	})

	t.Run("getter always fails — times out", func(t *testing.T) {
		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return nil, errors.New("discovery failed")
		}

		client := &Client{
			osFs: afero.Afero{Fs: afero.NewMemMapFs()},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("getter returns nil device info — times out", func(t *testing.T) {
		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return nil, nil
		}

		client := &Client{
			osFs: afero.Afero{Fs: afero.NewMemMapFs()},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("no SCSI devices found for LUN — times out", func(t *testing.T) {
		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return makeDeviceInfo("dm-0", []string{}), nil
		}

		client := &Client{
			osFs: afero.Afero{Fs: afero.NewMemMapFs()},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("no multipath device — times out", func(t *testing.T) {
		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return makeDeviceInfo("", []string{"sda"}), nil
		}

		client := &Client{
			osFs: afero.Afero{Fs: afero.NewMemMapFs()},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("multipath device size unreadable — times out", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockCmd := mockexec.NewMockCommand(ctrl)
		fs := afero.NewMemMapFs()

		// Path device OK, but no dm-0 size file.
		afero.WriteFile(fs, "/sys/block/sda/size", []byte(sectorCount+"\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)

		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return makeDeviceInfo("dm-0", []string{"sda"}), nil
		}

		client := &Client{
			command: mockCmd,
			osFs:    afero.Afero{Fs: fs},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("device info change resets stable reads", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockCmd := mockexec.NewMockCommand(ctrl)
		fs := afero.NewMemMapFs()

		// All devices at target size from the start.
		afero.WriteFile(fs, "/sys/block/sda/size", []byte(sectorCount+"\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/sdb/size", []byte(sectorCount+"\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/sdb/device/state", []byte("running\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/dm-0/size", []byte(sectorCount+"\n"), 0o444)

		callCount := 0
		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			callCount++
			if callCount <= 2 {
				// First two calls: single path.
				return makeDeviceInfo("dm-0", []string{"sda"}), nil
			}
			// After that: a new path appears (e.g. late-arriving iSCSI session).
			return makeDeviceInfo("dm-0", []string{"sda", "sdb"}), nil
		}

		client := &Client{
			command: mockCmd,
			osFs:    afero.Afero{Fs: fs},
		}

		// Needs >12s: info changes at call 3 (t=6s), resetting stable reads.
		// Convergence then requires 3 more reads: t=6s, t=9s, t=12s.
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.NoError(t, err)
		// The info change on call 3 resets stable reads, so convergence takes more than 3 getter calls.
		assert.Greater(t, callCount, 3)
	})

	t.Run("resize never fixes size — times out", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockCmd := mockexec.NewMockCommand(ctrl)
		fs := afero.NewMemMapFs()

		const mapperName = "3600a098038314865515d4c5a70644636"

		// Path and multipath devices permanently undersized.
		afero.WriteFile(fs, "/sys/block/sda/size", []byte(smallSectors+"\n"), 0o444)
		fs.MkdirAll("/sys/block/sda/device", 0o755)
		afero.WriteFile(fs, "/sys/block/sda/device/rescan", []byte(""), 0o200)
		afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/dm-0/size", []byte(smallSectors+"\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/dm-0/dm/name", []byte(mapperName+"\n"), 0o444)

		// Resize succeeds but never actually grows the device size.
		mockCmd.EXPECT().ExecuteWithTimeout(
			gomock.Any(), "multipathd", 10*time.Second, true, "-kresize map "+mapperName,
		).Return([]byte("ok\n"), nil).AnyTimes()

		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return makeDeviceInfo("dm-0", []string{"sda"}), nil
		}

		client := &Client{
			command: mockCmd,
			osFs:    afero.Afero{Fs: fs},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("all paths unhealthy — immediate error", func(t *testing.T) {
		fs := afero.NewMemMapFs()

		// sda has no state file → isRunning returns false → unhealthy.
		fs.MkdirAll("/sys/block/sda/device", 0o755)

		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return makeDeviceInfo("dm-0", []string{"sda"}), nil
		}

		client := &Client{
			osFs: afero.Afero{Fs: fs},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.Error(t, err)
		// Should be an immediate error, not a context deadline.
		assert.NotErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("some paths unhealthy — immediate error", func(t *testing.T) {
		fs := afero.NewMemMapFs()

		// sda: healthy, at target size.
		afero.WriteFile(fs, "/sys/block/sda/size", []byte(sectorCount+"\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)

		// sdb: unhealthy (blocked).
		afero.WriteFile(fs, "/sys/block/sdb/device/state", []byte("blocked\n"), 0o444)

		// dm-0 at target size.
		afero.WriteFile(fs, "/sys/block/dm-0/size", []byte(sectorCount+"\n"), 0o444)

		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return makeDeviceInfo("dm-0", []string{"sda", "sdb"}), nil
		}

		client := &Client{
			osFs: afero.Afero{Fs: fs},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.Error(t, err)
		// Any unhealthy path must produce an immediate hard error, not a timeout.
		assert.NotErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("rescanUndersizedDevices fails — retries — times out", func(t *testing.T) {
		fs := afero.NewMemMapFs()

		// sda: undersized, state = running, but NO rescan file → rescanDevice fails.
		afero.WriteFile(fs, "/sys/block/sda/size", []byte(smallSectors+"\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)
		// (no /sys/block/sda/device/rescan file)

		afero.WriteFile(fs, "/sys/block/dm-0/size", []byte(smallSectors+"\n"), 0o444)

		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return makeDeviceInfo("dm-0", []string{"sda"}), nil
		}

		client := &Client{
			osFs: afero.Afero{Fs: fs},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("getDeviceMapperName fails — retries — times out", func(t *testing.T) {
		fs := afero.NewMemMapFs()

		// sda at target size — no rescan needed.
		afero.WriteFile(fs, "/sys/block/sda/size", []byte(sectorCount+"\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)

		// dm-0 undersized but NO dm/name file → getDeviceMapperName returns NotFoundError.
		afero.WriteFile(fs, "/sys/block/dm-0/size", []byte(smallSectors+"\n"), 0o444)

		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return makeDeviceInfo("dm-0", []string{"sda"}), nil
		}

		client := &Client{
			osFs: afero.Afero{Fs: fs},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("resizeMultipathMap fails — warns and retries — times out", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockCmd := mockexec.NewMockCommand(ctrl)
		fs := afero.NewMemMapFs()

		const mapperName = "3600a098038314865515d4c5a70644636"

		// sda at target size — no rescan needed.
		afero.WriteFile(fs, "/sys/block/sda/size", []byte(sectorCount+"\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/sda/device/state", []byte("running\n"), 0o444)

		// dm-0 undersized; dm/name present so getDeviceMapperName succeeds.
		afero.WriteFile(fs, "/sys/block/dm-0/size", []byte(smallSectors+"\n"), 0o444)
		afero.WriteFile(fs, "/sys/block/dm-0/dm/name", []byte(mapperName+"\n"), 0o444)

		// Resize always fails.
		mockCmd.EXPECT().ExecuteWithTimeout(
			gomock.Any(), "multipathd", 10*time.Second, true, "-kresize map "+mapperName,
		).Return(nil, fmt.Errorf("multipathd: resize failed")).AnyTimes()

		getter := func(ctx context.Context) (*models.ScsiDeviceInfo, error) {
			return makeDeviceInfo("dm-0", []string{"sda"}), nil
		}

		client := &Client{
			command: mockCmd,
			osFs:    afero.Afero{Fs: fs},
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := client.ExpandMultipathDevice(ctx, getter, targetSizeBytes)
		assert.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

// TestVerifyMultipathDeviceSerial covers the serial-number-based multipath device verification path,
// which calls GetDeviceMapperUUID and is therefore Linux-only.
func TestVerifyMultipathDeviceSerial(t *testing.T) {
	tests := map[string]struct {
		getFs       func() afero.Fs
		publishInfo *models.VolumePublishInfo
		deviceInfo  *models.ScsiDeviceInfo
		expectError bool
	}{
		"Ghost Device": {
			publishInfo: &models.VolumePublishInfo{
				DevicePath: "",
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiLunSerial: "yocwB?Wl7x2l",
					},
				},
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "/dev/dm-0",
				DevicePaths:     []string{"/dev/sda"},
			},
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				// Defines LUN serial matching the published serial, so the serial path is taken.
				afero.WriteFile(fs, "/dev/sda/vpd_pg80", []byte{
					0, 128, 0, 12, 121, 111, 99, 119, 66, 63, 87, 108,
					55, 120, 50, 108,
				}, 0o755)
				// UUID file for dm-1 contains the hex encoding of the serial.
				afero.WriteFile(fs, "/sys/block/dm-1/dm/uuid", []byte("mpath-3600a0980796f6377423f576c3778326c"), 0o755)
				// Non-empty slaves dir — not a ghost device in the traditional sense,
				// but the test verifies that VerifyMultipathDevice succeeds.
				fs.Mkdir("/sys/block/dm-1/slaves/sdb", 0o755)
				return fs
			},
			expectError: false,
		},
		"Not Ghost Device": {
			publishInfo: &models.VolumePublishInfo{
				DevicePath: "",
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiLunSerial: "yocwB?Wl7x2l",
					},
				},
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "/dev/dm-0",
				DevicePaths:     []string{"/dev/sda"},
			},
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				afero.WriteFile(fs, "/dev/sda/vpd_pg80", []byte{
					0, 128, 0, 12, 121, 111, 99, 119, 66, 63, 87, 108,
					55, 120, 50, 108,
				}, 0o755)
				afero.WriteFile(fs, "/sys/block/dm-1/dm/uuid", []byte("mpath-3600a0980796f6377423f576c3778326c"), 0o755)
				// Empty slaves dir — this is a ghost device (no paths attached).
				fs.Mkdir("/sys/block/dm-1/slaves/", 0o755)
				return fs
			},
			expectError: false,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			deviceClient := NewDetailed(nil, params.getFs(), nil)
			_, err := deviceClient.VerifyMultipathDevice(context.TODO(), params.publishInfo,
				nil, params.deviceInfo)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
