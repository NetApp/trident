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
	"github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
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

			deviceClient := NewDetailed(mockCommand, afero.NewMemMapFs(), NewDiskSizeGetter())
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
			client := NewDetailed(nil, afero.Afero{Fs: params.getFileSystemUtils()}, NewDiskSizeGetter())

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
			client := NewDetailed(nil, afero.Afero{Fs: params.getFileSystemUtils()}, NewDiskSizeGetter())
			response, err := client.GetLunSerial(context.TODO(), devicePath)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedResponse, response)
		})
	}
}

func TestClient_EnsureDeviceReadable(t *testing.T) {
	devicePath := "/dev/mock-0"
	tests := map[string]struct {
		getMockCmd  func() exec.Command
		expectError bool
	}{
		"Happy Path": {
			getMockCmd: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				outBytes := 4096
				out := make([]byte, outBytes)
				for i := range outBytes {
					out[i] = 0
				}
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "dd", 5*time.Second, false, "if="+devicePath,
					"bs=4096", "count=1", "status=none").Return(out, nil)
				return mockCommand
			},
			expectError: false,
		},
		"Fail to read device": {
			getMockCmd: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "dd", 5*time.Second, false, "if="+devicePath,
					"bs=4096", "count=1", "status=none").Return([]byte(""), fmt.Errorf("error"))
				return mockCommand
			},
			expectError: true,
		},
		"NoDataRead": {
			getMockCmd: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "dd", 5*time.Second, false, "if="+devicePath,
					"bs=4096", "count=1", "status=none").Return([]byte(""), nil)
				return mockCommand
			},
			expectError: true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			deviceClient := NewDetailed(params.getMockCmd(), afero.NewMemMapFs(), NewDiskSizeGetter())
			err := deviceClient.EnsureDeviceReadable(context.TODO(), devicePath)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCanFlushMultipathDevice(t *testing.T) {
	devicePath := "/dev/mock-0"
	tests := map[string]struct {
		getMockCmd  func() exec.Command
		expectError bool
	}{
		"Happy Path": {
			getMockCmd: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "multipath", 5*time.Second, true,
					"-C", devicePath).Return([]byte(""), nil)
				return mockCommand
			},
			expectError: false,
		},
		"Device Not Ready For Flush": {
			getMockCmd: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "multipath", 5*time.Second, true,
					"-C", devicePath).Return([]byte(""), fmt.Errorf("error"))
				return mockCommand
			},
			expectError: true,
		},
		"Device Unavailable": {
			getMockCmd: func() exec.Command {
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "multipath", 5*time.Second, true,
					"-C", devicePath).Return([]byte("no usable paths found"), fmt.Errorf("error"))
				return mockCommand
			},
			expectError: true,
		},
		"Flush timeout exceeded": {
			getMockCmd: func() exec.Command {
				volumeFlushExceptions[devicePath] = time.Now().Add(-1 * time.Hour)
				mockCommand := mockexec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "multipath", 5*time.Second, true,
					"-C", devicePath).Return([]byte("no usable paths found"), fmt.Errorf("error"))
				return mockCommand
			},
			expectError: true,
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			deviceClient := NewDetailed(params.getMockCmd(), afero.NewMemMapFs(), NewDiskSizeGetter())
			err := deviceClient.canFlushMultipathDevice(context.TODO(), devicePath)
			delete(volumeFlushExceptions, devicePath)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFindDevicesForMultipathDevice(t *testing.T) {
	dm := "dm-0"
	device := "sda"
	tests := map[string]struct {
		getFs  func() afero.Fs
		expect []string
	}{
		"Happy Path": {
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				fs.Create(fmt.Sprintf("/sys/block/%s/slaves/%s", dm, device))
				return fs
			},
			expect: []string{device},
		},
		"Device Not Found": {
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				return fs
			},
			expect: []string{},
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			deviceClient := NewDetailed(nil, params.getFs(), nil)
			s := deviceClient.FindDevicesForMultipathDevice(context.TODO(), dm)
			assert.Equal(t, params.expect, s)
		})
	}
}

func TestVerifyMultipathDevice(t *testing.T) {
	tests := map[string]struct {
		getFs           func() afero.Fs
		publishInfo     *models.VolumePublishInfo
		deviceInfo      *models.ScsiDeviceInfo
		allPublishInfos []models.VolumePublishInfo
		expectError     bool
	}{
		"CompareWithPublishedDevicePath Happy Path": {
			publishInfo: &models.VolumePublishInfo{
				DevicePath: "/dev/dm-0",
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "/dev/dm-0",
			},
			getFs: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			expectError: false,
		},
		"CompareWithPublishedDevicePath Incorrect Multipath": {
			publishInfo: &models.VolumePublishInfo{
				DevicePath: "/dev/dm-0",
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "/dev/dm-1",
			},
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				fs.Mkdir("/sys/block/dm-0/slaves/sda", 0o755)
				return fs
			},
			expectError: false,
		},
		"CompareWithPublishedDevicePath Incorrect Multipath, Ghost Device": {
			publishInfo: &models.VolumePublishInfo{
				DevicePath: "/dev/dm-0",
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "/dev/dm-1",
			},
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				fs.Mkdir("/sys/block/dm-0/slaves", 0o755)
				return fs
			},
			expectError: false,
		},
		"CompareWithPublishedSerialNumber Happy Path": {
			publishInfo: &models.VolumePublishInfo{
				DevicePath: "",
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiLunSerial: "1234",
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
				return fs
			},
			expectError: false,
		},
		"CompareWithPublishedSerialNumber GetMultipathDeviceUUID Error": {
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
				fs.Mkdir("/sys/block/dm-0", 0o755)
				return fs
			},
			expectError: true,
		},
		"CompareWithPublishedSerialNumber Missing Multipath Device": {
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
				return fs
			},
			expectError: true,
		},
		"CompareWithPublishedSerialNumber Fail Getting LUN Serial": {
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
				// LUN serial is not defined
				return fs
			},
			expectError: true,
		},
		"CompareWithPublishedSerialNumber Ghost Device": {
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
				// Defines LUN serial
				afero.WriteFile(fs, "/dev/sda/vpd_pg80", []byte{
					0, 128, 0, 12, 121, 111, 99, 119, 66, 63, 87, 108,
					55, 120, 50, 108,
				}, 0o755)
				afero.WriteFile(fs, "/sys/block/dm-1/dm/uuid", []byte("mpath-3600a0980796f6377423f576c3778326c"), 0o755)
				fs.Mkdir("/sys/block/dm-1/slaves/sdb", 0o755)
				return fs
			},
			expectError: false,
		},
		"CompareWithPublishedSerialNumber Not Ghost Device": {
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
				fs.Mkdir("/sys/block/dm-1/slaves/", 0o755)
				return fs
			},
			expectError: false,
		},
		"CompareWithAllPublishInfos Happy Path": {
			publishInfo: &models.VolumePublishInfo{
				DevicePath: "",
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiLunSerial: "",
					},
				},
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "/dev/dm-0",
				DevicePaths:     []string{"/dev/sda"},
			},
			allPublishInfos: []models.VolumePublishInfo{
				{
					VolumeAccessInfo: models.VolumeAccessInfo{
						IscsiAccessInfo: models.IscsiAccessInfo{
							IscsiLunNumber: 1,
						},
					},
				},
			},
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				afero.WriteFile(fs, "/dev/sda/vpd_pg80", []byte{
					0, 128, 0, 12, 121, 111, 99, 119, 66, 63, 87, 108,
					55, 120, 50, 108,
				}, 0o755)
				afero.WriteFile(fs, "/sys/block/dm-1/dm/uuid", []byte("mpath-3600a0980796f6377423f576c3778326c"), 0o755)
				fs.Mkdir("/sys/block/dm-1/slaves/", 0o755)
				return fs
			},
			expectError: false,
		},
		"CompareWithAllPublishInfos Missing All Publish Info": {
			publishInfo: &models.VolumePublishInfo{
				DevicePath: "",
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiLunSerial: "",
					},
				},
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "/dev/dm-0",
				DevicePaths:     []string{"/dev/sda"},
			},
			allPublishInfos: []models.VolumePublishInfo{},
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				afero.WriteFile(fs, "/dev/sda/vpd_pg80", []byte{
					0, 128, 0, 12, 121, 111, 99, 119, 66, 63, 87, 108,
					55, 120, 50, 108,
				}, 0o755)
				afero.WriteFile(fs, "/sys/block/dm-1/dm/uuid", []byte("mpath-3600a0980796f6377423f576c3778326c"), 0o755)
				fs.Mkdir("/sys/block/dm-1/slaves/", 0o755)
				return fs
			},
			expectError: true,
		},
		"CompareWithAllPublishInfos Duplicate LUNs": {
			publishInfo: &models.VolumePublishInfo{
				DevicePath: "",
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiLunSerial: "",
						IscsiLunNumber: 2,
					},
				},
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "/dev/dm-0",
				DevicePaths:     []string{"/dev/sda"},
			},
			allPublishInfos: []models.VolumePublishInfo{
				{
					VolumeAccessInfo: models.VolumeAccessInfo{
						IscsiAccessInfo: models.IscsiAccessInfo{
							IscsiLunNumber: 2,
						},
					},
				},
				{
					VolumeAccessInfo: models.VolumeAccessInfo{
						IscsiAccessInfo: models.IscsiAccessInfo{
							IscsiLunNumber: 2,
						},
					},
				},
			},
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				afero.WriteFile(fs, "/dev/sda/vpd_pg80", []byte{
					0, 128, 0, 12, 121, 111, 99, 119, 66, 63, 87, 108,
					55, 120, 50, 108,
				}, 0o755)
				afero.WriteFile(fs, "/sys/block/dm-1/dm/uuid", []byte("mpath-3600a0980796f6377423f576c3778326c"), 0o755)
				fs.Mkdir("/sys/block/dm-1/slaves/", 0o755)
				return fs
			},
			expectError: true,
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			deviceClient := NewDetailed(nil, params.getFs(), nil)
			_, err := deviceClient.VerifyMultipathDevice(context.TODO(), params.publishInfo,
				params.allPublishInfos,
				params.deviceInfo)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRemoveDevice(t *testing.T) {
	tests := map[string]struct {
		getFs       func() afero.Fs
		deviceInfo  *models.ScsiDeviceInfo
		expectError bool
	}{
		"Happy Path": {
			deviceInfo: &models.ScsiDeviceInfo{
				Devices: []string{"sda"},
			},
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				fs.Create("/sys/block/sda/device/delete")
				return fs
			},
			expectError: false,
		},
		"Error Opening File": {
			deviceInfo: &models.ScsiDeviceInfo{
				Devices: []string{"sda"},
			},
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				return fs
			},
			expectError: true,
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			deviceClient := NewDetailed(nil, params.getFs(), NewDiskSizeGetter())
			err := deviceClient.RemoveDevice(context.TODO(), params.deviceInfo.Devices, false)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetLUKSDeviceForMultipathDevice(t *testing.T) {
	multipathDevice := "/dev/dm-0"
	tests := map[string]struct {
		getFs       func() afero.Fs
		expectError bool
	}{
		"Happy Path": {
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				fs.Create("/sys/block/dm-0/holders/dm-1")
				afero.WriteFile(fs, "/sys/block/dm-1/dm/uuid", []byte("CRYPT-LUKS2-b600e061186e46ac8b05590f389da249-luks-trident_pvc_4b7874ba_58d7_4d93_8d36_09a09b837f81"), 0o755)
				return fs
			},
			expectError: false,
		},
		"No Holders Found": {
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				fs.Mkdir("/sys/block/dm-0/holders/", 0o755)
				// afero.WriteFile(fs, "/sys/block/dm-1/dm/uuid", []byte("CRYPT-LUKS2-b600e061186e46ac8b05590f389da249-luks-trident_pvc_4b7874ba_58d7_4d93_8d36_09a09b837f81"), 0755)
				return fs
			},
			expectError: true,
		},
		"Multiple Holders Found": {
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				fs.Create("/sys/block/dm-0/holders/dm-1")
				fs.Create("/sys/block/dm-0/holders/dm-2")
				// afero.WriteFile(fs, "/sys/block/dm-1/dm/uuid", []byte("CRYPT-LUKS2-b600e061186e46ac8b05590f389da249-luks-trident_pvc_4b7874ba_58d7_4d93_8d36_09a09b837f81"), 0755)
				return fs
			},
			expectError: true,
		},
		"Not LUKS Device": {
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				fs.Create("/sys/block/dm-0/holders/dm-1")
				afero.WriteFile(fs, "/sys/block/dm-1/dm/uuid", []byte("not-a-luks-uuid"), 0o755)
				return fs
			},
			expectError: true,
		},
		"Error Reading Holders Dir": {
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				return fs
			},
			expectError: true,
		},
		"Error Reading UUID": {
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				fs.Create("/sys/block/dm-0/holders/dm-1")
				return fs
			},
			expectError: true,
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			deviceClient := NewDetailed(nil, params.getFs(), NewDiskSizeGetter())
			_, err := deviceClient.GetLUKSDeviceForMultipathDevice(multipathDevice)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFindMultipathDeviceForDevice(t *testing.T) {
	device := "sda"
	tests := map[string]struct {
		getFs  func() afero.Fs
		expect string
	}{
		"Happy Path": {
			getFs: func() afero.Fs {
				fs := afero.NewMemMapFs()
				fs.Create(fmt.Sprintf("/sys/block/%s/holders/dm-1", device))
				afero.WriteFile(fs, "/sys/block/dm-1/dm/uuid", []byte("CRYPT-LUKS2-b600e061186e46ac8b05590f389da249-luks-trident_pvc_4b7874ba_58d7_4d93_8d36_09a09b837f81"), 0o755)
				return fs
			},
			expect: "dm-1",
		},
		"Not Found": {
			getFs: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			expect: "",
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			deviceClient := NewDetailed(nil, params.getFs(), NewDiskSizeGetter())
			foundDevice := deviceClient.FindMultipathDeviceForDevice(context.TODO(), device)
			assert.Equal(t, params.expect, foundDevice)
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
			devices := NewDetailed(nil, fs, nil)
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
