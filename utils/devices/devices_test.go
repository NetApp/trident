// Copyright 2024 NetApp, Inc. All Rights Reserved.

package devices

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/spf13/afero/mem"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	tridentError "github.com/netapp/trident/utils/errors"
)

func TestRemoveMultipathDeviceMapping(t *testing.T) {
	mockCommand := mockexec.NewMockCommand(gomock.NewController(t))

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

			deviceClient := NewDetailed(mockCommand, afero.NewMemMapFs())
			err := deviceClient.RemoveMultipathDeviceMapping(context.TODO(), tt.devicePath)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

type aferoWrapper struct {
	openFileError    error
	openFileResponse afero.File
	openResponse     afero.File
	openError        error
	afero.Fs
}

type aferoFileWrapper struct {
	WriteStringError error
	WriteStringCount int
	afero.File
}

func (a *aferoWrapper) OpenFile(_ string, _ int, _ os.FileMode) (afero.File, error) {
	return a.openFileResponse, a.openFileError
}

func (a *aferoWrapper) Open(_ string) (afero.File, error) {
	return a.openResponse, a.openError
}

func (a *aferoFileWrapper) WriteString(_ string) (ret int, err error) {
	return a.WriteStringCount, a.WriteStringError
}

func TestClient_scanTargetLUN(t *testing.T) {
	type parameters struct {
		assertError        assert.ErrorAssertionFunc
		getFileSystemUtils func() afero.Fs
	}

	const lunID = 0
	const host1 = 1
	const host2 = 2

	tests := map[string]parameters{
		"scan files not present": {
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError: assert.Error,
		},
		"error writing to scan files": {
			getFileSystemUtils: func() afero.Fs {
				memFs := afero.NewMemMapFs()
				_, err := memFs.Create(fmt.Sprintf("/sys/class/scsi_host/host%d/scan", host1))
				assert.NoError(t, err)
				_, err = memFs.Create(fmt.Sprintf("/sys/class/scsi_host/host%d/scan", host2))
				assert.NoError(t, err)

				f := &aferoFileWrapper{
					WriteStringError: errors.New("some error"),
					File:             mem.NewFileHandle(&mem.FileData{}),
				}

				fs := &aferoWrapper{
					openFileResponse: f,
					openResponse:     f,
					Fs:               memFs,
				}

				return fs
			},
			assertError: assert.Error,
		},
		"failed to write to scan files": {
			getFileSystemUtils: func() afero.Fs {
				memFs := afero.NewMemMapFs()
				_, err := memFs.Create(fmt.Sprintf("/sys/class/scsi_host/host%d/scan", host1))
				assert.NoError(t, err)
				_, err = memFs.Create(fmt.Sprintf("/sys/class/scsi_host/host%d/scan", host2))
				assert.NoError(t, err)

				f := &aferoFileWrapper{
					WriteStringCount: 0,
					File:             mem.NewFileHandle(&mem.FileData{}),
				}

				fs := &aferoWrapper{
					openFileResponse: f,
					openResponse:     f,
					Fs:               memFs,
				}

				return fs
			},
			assertError: assert.Error,
		},
		"happy path": {
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create(fmt.Sprintf("/sys/class/scsi_host/host%d/scan", host1))
				assert.NoError(t, err)
				_, err = fs.Create(fmt.Sprintf("/sys/class/scsi_host/host%d/scan", host2))
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			client := NewDetailed(nil, afero.Afero{Fs: params.getFileSystemUtils()})

			err := client.ScanTargetLUN(context.TODO(), lunID, []int{host1, host2})
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func vpdpg80SerialBytes(serial string) []byte {
	return append([]byte{0, 128, 0, 20}, []byte(serial)...)
}

func TestClient_getLunSerial(t *testing.T) {
	type parameters struct {
		getFileSystemUtils func() afero.Fs
		expectedResponse   string
		assertError        assert.ErrorAssertionFunc
	}

	const devicePath = "/dev/sda"
	const vpdpg80Serial = "SYA5GZFJ8G1M905GVH7H"

	tests := map[string]parameters{
		"error reading serial file": {
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				return fs
			},
			expectedResponse: "",
			assertError:      assert.Error,
		},
		"invalid serial in file len < 4 bytes": {
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				f, err := fs.Create(devicePath + "/vpd_pg80")
				assert.NoError(t, err)
				_, err = f.Write([]byte("123"))
				assert.NoError(t, err)
				return fs
			},
			expectedResponse: "",
			assertError:      assert.Error,
		},
		"invalid serial bytes[1] != 0x80": {
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				f, err := fs.Create(devicePath + "/vpd_pg80")
				assert.NoError(t, err)
				_, err = f.Write([]byte{0x81, 0x00, 0x00, 0x00, 0x00})
				assert.NoError(t, err)
				return fs
			},
			expectedResponse: "",
			assertError:      assert.Error,
		},
		"invalid serial bad length": {
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				f, err := fs.Create(devicePath + "/vpd_pg80")
				assert.NoError(t, err)
				_, err = f.Write([]byte{0x81, 0x80, 0x01, 0x01, 0x02})
				assert.NoError(t, err)
				return fs
			},
			expectedResponse: "",
			assertError:      assert.Error,
		},
		"happy path": {
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				f, err := fs.Create(devicePath + "/vpd_pg80")
				assert.NoError(t, err)
				_, err = f.Write(vpdpg80SerialBytes(vpdpg80Serial))
				assert.NoError(t, err)
				return fs
			},
			expectedResponse: vpdpg80Serial,
			assertError:      assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			client := NewDetailed(nil, afero.Afero{Fs: params.getFileSystemUtils()})
			response, err := client.GetLunSerial(context.TODO(), devicePath)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedResponse, response)
		})
	}
}

// NOTE: Since this is now in the devices package,
// we cannot unit test this without finding a way to mock file's Fd() function.
// Afero does not support this.
//func TestClient_verifyMultipathDeviceSize(t *testing.T) {
//	type parameters struct {
//		getDevicesClient   func(controller *gomock.Controller) Devices
//		getFs              func(controller *gomock.Controller) afero.Fs
//		assertError        assert.ErrorAssertionFunc
//		assertValid        assert.BoolAssertionFunc
//		expectedDeviceSize int64
//	}
//
//	const deviceName = "sda"
//	const multipathDeviceName = "dm-0"
//
//	tests := map[string]parameters{
//		"error getting device size": {
//			getDevicesClient: func(controller *gomock.Controller) Devices {
//				mockDevices := mock_devices.NewMockDevices(controller)
//				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(0),
//					errors.New("some error"))
//				return mockDevices
//			},
//			assertError:        assert.Error,
//			assertValid:        assert.False,
//			expectedDeviceSize: 0,
//		},
//		"error getting multipath device size": {
//			getDevicesClient: func(controller *gomock.Controller) Devices {
//				mockDevices := mock_devices.NewMockDevices(controller)
//				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(1), nil)
//				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(0),
//					errors.New("some error"))
//				return mockDevices
//			},
//			assertError:        assert.Error,
//			assertValid:        assert.False,
//			expectedDeviceSize: 0,
//		},
//		"device size != multipath device size": {
//			getDevicesClient: func(controller *gomock.Controller) Devices {
//				mockDevices := mock_devices.NewMockDevices(controller)
//				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(1), nil)
//				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(0), nil)
//				return mockDevices
//			},
//			assertError:        assert.NoError,
//			assertValid:        assert.False,
//			expectedDeviceSize: 1,
//		},
//		"happy path": {
//			getDevicesClient: func(controller *gomock.Controller) Devices {
//				mockDevices := mock_devices.NewMockDevices(controller)
//				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(1), nil)
//				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(1), nil)
//				return mockDevices
//			},
//			getFs: func(controller *gomock.Controller) afero.Fs {
//				fs := afero.NewMemMapFs()
//				fs.Create(DevPrefix + deviceName)
//				fs.Create(multipathDeviceName)
//				return fs
//			},
//			assertError:        assert.NoError,
//			assertValid:        assert.True,
//			expectedDeviceSize: 0,
//		},
//	}
//
//	for name, params := range tests {
//		t.Run(name, func(t *testing.T) {
//			ctrl := gomock.NewController(t)
//			var fs afero.Fs
//			if params.getFs != nil {
//				fs = params.getFs(ctrl)
//			}
//			client := NewDetailed(execCmd.NewCommand(), fs)
//			deviceSize, valid, err := client.VerifyMultipathDeviceSize(context.TODO(), multipathDeviceName, deviceName)
//			if params.assertError != nil {
//				params.assertError(t, err)
//			}
//			if params.assertValid != nil {
//				params.assertValid(t, valid)
//			}
//			assert.Equal(t, params.expectedDeviceSize, deviceSize)
//		})
//	}
//}

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
			expectedError: tridentError.TimeoutError(errMsg),
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
			expectedError: tridentError.TimeoutError(errMsg),
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
			expectedError: tridentError.TimeoutError(errMsg),
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			fs, err := params.getOsFs()
			assert.NoError(t, err)
			devices := NewDetailed(nil, fs)
			err = devices.WaitForDevicesRemoval(context.Background(), params.devicePathPrefix, params.deviceNames,
				params.maxWaitTime)
			if params.expectedError != nil {
				assert.EqualError(t, err, params.expectedError.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
