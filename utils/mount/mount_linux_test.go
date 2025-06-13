// Copyright 2024 NetApp, Inc. All Rights Reserved.

package mount

import (
	"context"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/mocks/mock_utils/mock_mount/mock_filepathwrapper"
	"github.com/netapp/trident/mocks/mock_utils/mock_mount/mock_oswrapper"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount/filepathwrapper"
	"github.com/netapp/trident/utils/mount/oswrapper"
)

const (
	selfMountInfoPath = "/proc/self/mountinfo"
	hostMountInfoPath = "/proc/1/mountinfo"
)

func TestNewOsSpecificClient(t *testing.T) {
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func TestNewOsSpecificClientDetailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCommand := mock_exec.NewMockCommand(ctrl)
	mockOsClient := mock_oswrapper.NewMockOS(ctrl)
	mockFilePathClient := mock_filepathwrapper.NewMockFilePath(ctrl)
	client := newOsSpecificClientDetailed(mockOsClient, mockFilePathClient, mockCommand)
	assert.NotNil(t, client)
}

func TestLinuxClient_IsLikelyNotMountPoint(t *testing.T) {
	type parameters struct {
		getOSClient    func(controller *gomock.Controller) oswrapper.OS
		expectedResult bool
		assertError    assert.ErrorAssertionFunc
	}
	const mountPointParent = "/foo"
	const mountPoint = mountPointParent + "/test"

	tests := map[string]parameters{
		"failed to check if mount point exists": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, assert.AnError)
				return mockOsClient
			},
			expectedResult: true,
			assertError:    assert.Error,
		},
		"error getting information about mount point's parent directory": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(&mockFileInfo{device: 1}, nil)
				mockOsClient.EXPECT().Lstat(mountPointParent).Return(nil, assert.AnError)
				return mockOsClient
			},
			expectedResult: true,
			assertError:    assert.Error,
		},
		"parent directory is on a different device than the mount point": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(&mockFileInfo{device: 1}, nil)
				mockOsClient.EXPECT().Lstat(mountPointParent).Return(&mockFileInfo{device: 2}, nil)
				return mockOsClient
			},
			expectedResult: false,
			assertError:    assert.NoError,
		},
		"parent directory is on the same device as the mount point": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(&mockFileInfo{device: 1}, nil)
				mockOsClient.EXPECT().Lstat(mountPointParent).Return(&mockFileInfo{device: 1}, nil)
				return mockOsClient
			},
			expectedResult: true,
			assertError:    assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockCommand := mock_exec.NewMockCommand(ctrl)

			client := newOsSpecificClientDetailed(params.getOSClient(ctrl), filepathwrapper.New(), mockCommand)
			result, err := client.IsLikelyNotMountPoint(context.Background(), mountPoint)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedResult, result)
		})
	}
}

func TestLinuxClient_IsMounted(t *testing.T) {
	type parameters struct {
		mountPoint        string
		sourceDevice      string
		mountOptions      string
		getOSClient       func(controller *gomock.Controller) oswrapper.OS
		getFilePathClient func(controller *gomock.Controller) filepathwrapper.FilePath
		assertError       assert.ErrorAssertionFunc
		expectedResult    bool
	}

	const defaultMountPoint = "/foo/test"
	const badMountOptions = "dummy"
	const defaultSourceDevice = "/dev/sdax"
	const defaultDevicePath = "/dev/sda1" // This is what you get after resolving symlinks.

	tests := map[string]parameters{
		"error getting device path": {
			mountPoint:   "",
			sourceDevice: "",
			mountOptions: "",
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				return mockOsClient
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				return mockFilePathClient
			},
			assertError:    assert.Error,
			expectedResult: false,
		},
		"error listing proc mount information": {
			mountPoint:   defaultMountPoint,
			sourceDevice: defaultSourceDevice,
			mountOptions: "",
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(nil, assert.AnError)
				return mockOsClient
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				mockFilePathClient.EXPECT().EvalSymlinks(strings.TrimPrefix(defaultSourceDevice, "/dev/")).Return(defaultDevicePath, nil)
				return mockFilePathClient
			},
			assertError:    assert.Error,
			expectedResult: false,
		},
		"mount point not present in mountinfo": {
			mountPoint:   defaultMountPoint,
			sourceDevice: defaultSourceDevice,
			mountOptions: "",
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().bytes(), nil).Times(4)
				return mockOsClient
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				mockFilePathClient.EXPECT().EvalSymlinks(strings.TrimPrefix(defaultSourceDevice, "/dev/")).Return(defaultDevicePath, nil)
				return mockFilePathClient
			},
			assertError:    assert.NoError,
			expectedResult: false,
		},
		"mount point present, source device not specified": {
			mountPoint:   defaultMountPoint,
			sourceDevice: "",
			mountOptions: "",
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().
					withMountPoint(defaultMountPoint).bytes(), nil).Times(2)
				return mockOsClient
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				mockFilePathClient.EXPECT().EvalSymlinks("").Return(defaultDevicePath, nil)
				return mockFilePathClient
			},
			assertError:    assert.NoError,
			expectedResult: true,
		},
		"mount point present, source device mismatch": {
			mountPoint:   defaultMountPoint,
			sourceDevice: "/dev/boo",
			mountOptions: "",
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().
					withMountPoint(defaultMountPoint).withMountSource(defaultSourceDevice).bytes(), nil).Times(4)
				return mockOsClient
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				gomock.InOrder(
					mockFilePathClient.EXPECT().EvalSymlinks("boo").Return("/dev/foo", nil).Times(1),
					mockFilePathClient.EXPECT().EvalSymlinks(defaultSourceDevice).Return(defaultDevicePath, nil).Times(4),
				)
				return mockFilePathClient
			},
			assertError:    assert.NoError,
			expectedResult: false,
		},
		"happy path": {
			mountPoint:   defaultMountPoint,
			sourceDevice: defaultSourceDevice,
			mountOptions: "",
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().
					withMountPoint(defaultMountPoint).withMountSource(defaultSourceDevice).bytes(), nil).Times(2)
				return mockOsClient
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				mockFilePathClient.EXPECT().EvalSymlinks(strings.TrimPrefix(defaultSourceDevice, "/dev/")).Return(defaultDevicePath, nil).Times(1)
				return mockFilePathClient
			},
			assertError:    assert.NoError,
			expectedResult: true,
		},
		"error checking mount options": {
			mountPoint:   defaultMountPoint,
			sourceDevice: defaultSourceDevice,
			mountOptions: badMountOptions,
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile("/proc/self/mountinfo").Return(newMountInfoEntry().
					withMountPoint(defaultMountPoint).withMountSource(defaultSourceDevice).bytes(), nil).Times(2)
				return mockOsClient
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				mockFilePathClient.EXPECT().EvalSymlinks(strings.TrimPrefix(defaultSourceDevice, "/dev/")).Return(defaultDevicePath, nil)
				return mockFilePathClient
			},
			assertError:    assert.NoError,
			expectedResult: true,
		},
		"error evaluating symlink mount source": {
			mountPoint:   defaultMountPoint,
			sourceDevice: defaultSourceDevice,
			mountOptions: "",
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().
					withMountPoint(defaultMountPoint).withMountSource("/dev/foo").bytes(), nil).Times(4)
				return mockOsClient
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				gomock.InOrder(
					mockFilePathClient.EXPECT().EvalSymlinks(strings.TrimPrefix(defaultSourceDevice, "/dev/")).Return(defaultDevicePath, nil).Times(1),
					mockFilePathClient.EXPECT().EvalSymlinks("/dev/foo").Return("", assert.AnError).Times(4),
				)
				return mockFilePathClient
			},
			assertError:    assert.NoError,
			expectedResult: false,
		},
		"mount source is a symlink": {
			mountPoint:   defaultMountPoint,
			sourceDevice: defaultSourceDevice,
			mountOptions: "",
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().
					withMountPoint(defaultMountPoint).withMountSource("/dev/foo").bytes(), nil).Times(2)
				return mockOsClient
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				gomock.InOrder(
					mockFilePathClient.EXPECT().EvalSymlinks(strings.TrimPrefix(defaultSourceDevice, "/dev/")).Return(defaultDevicePath, nil).Times(1),
					mockFilePathClient.EXPECT().EvalSymlinks("/dev/foo").Return(defaultDevicePath, nil).Times(2),
				)
				return mockFilePathClient
			},
			assertError:    assert.NoError,
			expectedResult: true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockCommand := mock_exec.NewMockCommand(ctrl)

			client := newOsSpecificClientDetailed(params.getOSClient(ctrl), params.getFilePathClient(ctrl), mockCommand)
			result, err := client.IsMounted(context.Background(), params.sourceDevice, params.mountPoint, params.mountOptions)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedResult, result)
		})
	}
}

func TestLinuxClient_PVMountpointMappings(t *testing.T) {
	type parameters struct {
		getOsClient       func(controller *gomock.Controller) oswrapper.OS
		getFilePathClient func(controller *gomock.Controller) filepathwrapper.FilePath
		expectedMappings  map[string]string
		assertError       assert.ErrorAssertionFunc
	}

	const mountPointFileSystem = "/var/lib/kubelet/pods/b085c3d6-4d92-4b3c-bd52-f3198b1ce200/volumes/kubernetes." +
		"io~csi/pvc-bf0920fa-1051-4794-81ab-4004973284a5/mount"
	const mountSourceFileSystem = "/dev/mapper/3600a098038314461522451715a736f33"
	const devicePath = "/dev/sda1"

	const mountPointBlock = "/var/lib/kubelet/plugins/kubernetes." +
		"io/csi/volumeDevices/publish/pvc-5179b521-6dfe-4e1f-afb9-bff7859cd11a/2ccd5d06-add5-4195-bcf9-58a104fc2ee7"
	const mountSourceBlock = "udev"
	const rootDevicePathBlock = "/dm-0"

	tests := map[string]parameters{
		"error reading mountinfo file": {
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(nil, assert.AnError)
				return mockOsClient
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				return mock_filepathwrapper.NewMockFilePath(controller)
			},
			expectedMappings: nil,
			assertError:      assert.Error,
		},
		"get mappings for volume with volumeMode FileSystem: failure evaluating symlink": {
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().
					withMountPoint(mountPointFileSystem).withMountSource(mountSourceFileSystem).bytes(), nil).Times(2)
				return mockOsClient
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				mockFilePathClient.EXPECT().EvalSymlinks(mountSourceFileSystem).Return("", assert.AnError)
				return mockFilePathClient
			},
			expectedMappings: map[string]string{},
			assertError:      assert.NoError,
		},
		"get mappings for volume with volumeMode FileSystem": {
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().
					withMountPoint(mountPointFileSystem).withMountSource(mountSourceFileSystem).bytes(), nil).Times(2)
				return mockOsClient
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				mockFilePathClient.EXPECT().EvalSymlinks(mountSourceFileSystem).Return(devicePath, nil)
				return mockFilePathClient
			},
			expectedMappings: map[string]string{
				mountPointFileSystem: devicePath,
			},
			assertError: assert.NoError,
		},
		"get mappings for volume with volumeMode Block": {
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().
					withMountPoint(mountPointBlock).withMountSource(mountSourceBlock).withRoot(rootDevicePathBlock).
					bytes(), nil).Times(2)
				return mockOsClient
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				return mock_filepathwrapper.NewMockFilePath(controller)
			},
			expectedMappings: map[string]string{
				mountPointBlock: "/dev" + rootDevicePathBlock,
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockCommand := mock_exec.NewMockCommand(ctrl)

			client := newOsSpecificClientDetailed(params.getOsClient(ctrl), params.getFilePathClient(ctrl), mockCommand)
			mapping, err := client.PVMountpointMappings(context.Background())
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedMappings, mapping)
		})
	}
}

func TestLinuxClient_MountNFSPath(t *testing.T) {
	type parameters struct {
		exportPath  string
		mountPoint  string
		options     string
		getCommand  func(controller *gomock.Controller) exec.Command
		assertError assert.ErrorAssertionFunc
	}

	const mountPoint = "/mount/point"
	const exportPath = "/export/path"
	const options = "rw"
	const inputOptions = "-o " + options

	const optionsv4 = "rw,nfsvers=4"
	const inputOptionsv4 = "-o " + optionsv4
	tests := map[string]parameters{
		"error mounting nfs volume": {
			exportPath: exportPath,
			mountPoint: mountPoint,
			options:    inputOptions,
			getCommand: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "mkdir", "-p", mountPoint).Return(nil, assert.AnError)
				mockCommand.EXPECT().Execute(gomock.Any(), "mount.nfs", "-o", options, exportPath,
					mountPoint).Return(nil, assert.AnError)
				return mockCommand
			},
			assertError: assert.Error,
		},
		"nfs mount happy path": {
			exportPath: exportPath,
			mountPoint: mountPoint,
			options:    inputOptions,
			getCommand: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "mkdir", "-p", mountPoint).Return(nil, assert.AnError)
				mockCommand.EXPECT().Execute(gomock.Any(), "mount.nfs", "-o", options, exportPath,
					mountPoint).Return(nil, nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"error mounting nfs.v4 volume": {
			exportPath: exportPath,
			mountPoint: mountPoint,
			options:    inputOptionsv4,
			getCommand: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "mkdir", "-p", mountPoint).Return(nil, assert.AnError)
				mockCommand.EXPECT().Execute(gomock.Any(), "mount.nfs4", "-o", optionsv4, exportPath,
					mountPoint).Return(nil, assert.AnError)
				return mockCommand
			},
			assertError: assert.Error,
		},
		"nfs.v4 mount happy path": {
			exportPath: exportPath,
			mountPoint: mountPoint,
			options:    inputOptionsv4,
			getCommand: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "mkdir", "-p", mountPoint).Return(nil, assert.AnError)
				mockCommand.EXPECT().Execute(gomock.Any(), "mount.nfs4", "-o", optionsv4, exportPath,
					mountPoint).Return(nil, nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(nil, nil, params.getCommand(ctrl))
			err := client.MountNFSPath(context.Background(), params.exportPath, params.mountPoint, params.options)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestLinuxClient_UmountAndRemoveTemporaryMountPoint(t *testing.T) {
	type parameters struct {
		getOsClient func(controller *gomock.Controller) oswrapper.OS
		assertError assert.ErrorAssertionFunc
	}

	const mountPoint = "/mount/point"

	tests := map[string]parameters{
		"error returned by UmountAndRemoveMountPoint": {
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint+"/tmp_mnt").Return(nil, assert.AnError)
				return mockOsClient
			},
			assertError: assert.Error,
		},
		"happy path": {
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint+"/tmp_mnt").Return(nil, os.ErrNotExist)
				return mockOsClient
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getOsClient(ctrl), nil, nil)

			err := client.UmountAndRemoveTemporaryMountPoint(context.Background(), mountPoint)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestLinuxClient_UmountAndRemoveMountPoint(t *testing.T) {
	type parameters struct {
		getOsClient func(controller *gomock.Controller) oswrapper.OS
		assertError assert.ErrorAssertionFunc
	}

	const mountPoint = "/mount/point"

	tests := map[string]parameters{
		"unable to remove mount point": {
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, nil)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, assert.AnError)
				return mockOsClient
			},
			assertError: assert.Error,
		},
		"error determining if mount point exists": {
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, assert.AnError)
				return mockOsClient
			},
			assertError: assert.Error,
		},
		"happy path": {
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, os.ErrNotExist)
				return mockOsClient
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getOsClient(ctrl), nil, nil)

			err := client.UmountAndRemoveMountPoint(context.Background(), mountPoint)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestLinuxClient_IsNFSShareMounted(t *testing.T) {
	type parameters struct {
		exportPath     string
		mountPoint     string
		getOsClient    func(controller *gomock.Controller) oswrapper.OS
		expectedResult bool
		assertError    assert.ErrorAssertionFunc
	}

	const mountPoint = "/mount/point"
	const exportPath = "/export/path"

	tests := map[string]parameters{
		"error getting mount info": {
			exportPath: exportPath,
			mountPoint: mountPoint,
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(nil, assert.AnError)
				return mockOsClient
			},
			expectedResult: false,
			assertError:    assert.Error,
		},
		"nfs share not mounted": {
			exportPath: exportPath,
			mountPoint: mountPoint,
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().bytes(), nil).Times(2)
				return mockOsClient
			},
			expectedResult: false,
			assertError:    assert.NoError,
		},
		"nfs share found": {
			exportPath: exportPath,
			mountPoint: mountPoint,
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().withMountPoint(
					mountPoint).withMountSource(exportPath).bytes(), nil).Times(2)
				return mockOsClient
			},
			expectedResult: true,
			assertError:    assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getOsClient(ctrl), nil, nil)

			result, err := client.IsNFSShareMounted(context.Background(), params.exportPath, params.mountPoint)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedResult, result)
		})
	}
}

func TestLinuxClient_GetSelfMountInfo(t *testing.T) {
	type parameters struct {
		getOsClient    func(controller *gomock.Controller) oswrapper.OS
		expectedResult []models.MountInfo
		assertError    assert.ErrorAssertionFunc
	}

	test := map[string]parameters{
		"error reading mount info": {
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(nil, assert.AnError)
				return mockOsClient
			},
			expectedResult: nil,
			assertError:    assert.Error,
		},
		"happy path": {
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().bytes(), nil).Times(2)
				return mockOsClient
			},
			expectedResult: []models.MountInfo{
				newMountInfoEntry().mountInfo,
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range test {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getOsClient(ctrl), nil, nil)

			result, err := client.GetSelfMountInfo(context.Background())
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedResult, result)
		})
	}
}

func TestLinuxClient_GetHostMountInfo(t *testing.T) {
	type parameters struct {
		getOsClient    func(controller *gomock.Controller) oswrapper.OS
		expectedResult []models.MountInfo
		assertError    assert.ErrorAssertionFunc
	}

	test := map[string]parameters{
		"error reading mount info": {
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(hostMountInfoPath).Return(nil, assert.AnError)
				return mockOsClient
			},
			expectedResult: nil,
			assertError:    assert.Error,
		},
		"happy path": {
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(hostMountInfoPath).Return(newMountInfoEntry().bytes(), nil).Times(2)
				return mockOsClient
			},
			expectedResult: []models.MountInfo{
				newMountInfoEntry().mountInfo,
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range test {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getOsClient(ctrl), nil, nil)

			result, err := client.GetHostMountInfo(context.Background())
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedResult, result)
		})
	}
}

func TestLinuxClient_MountDevice(t *testing.T) {
	type parameters struct {
		options           string
		isMountPointFile  bool
		getCommand        func(controller *gomock.Controller) exec.Command
		getFilePathClient func(controller *gomock.Controller) filepathwrapper.FilePath
		getOsClient       func(controller *gomock.Controller) oswrapper.OS
		assertError       assert.ErrorAssertionFunc
	}

	const device = "/dev/sda1"
	const mountPoint = "/mount/point"
	const options = "rw"

	tests := map[string]parameters{
		"no options specified": {
			options:          "",
			isMountPointFile: false,
			getCommand: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "mount", device, mountPoint).Return(nil, nil)
				return mockCommand
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				mockFilePathClient.EXPECT().EvalSymlinks(strings.TrimPrefix(device, "/dev/")).Return(device, nil)
				return mockFilePathClient
			},
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().bytes(), nil).Times(4)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, nil)
				return mockOsClient
			},
			assertError: assert.NoError,
		},
		"options specified": {
			options:          options,
			isMountPointFile: false,
			getCommand: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "mount", "-o", options, device, mountPoint).Return(nil, nil)
				return mockCommand
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				mockFilePathClient.EXPECT().EvalSymlinks(strings.TrimPrefix(device, "/dev/")).Return(device, nil)
				return mockFilePathClient
			},
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().bytes(), nil).Times(4)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, nil)
				return mockOsClient
			},
			assertError: assert.NoError,
		},
		"mount point does not exist": {
			options:          options,
			isMountPointFile: false,
			getCommand: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "mount", "-o", options, device, mountPoint).Return(nil, nil)
				return mockCommand
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				mockFilePathClient.EXPECT().EvalSymlinks(strings.TrimPrefix(device, "/dev/")).Return(device, nil)
				return mockFilePathClient
			},
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().bytes(), nil).Times(4)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, assert.AnError)
				mockOsClient.EXPECT().Stat(mountPoint).Return(&mockFileInfo{isDir: false}, nil)
				return mockOsClient
			},
			assertError: assert.NoError,
		},
		"error executing mount command": {
			options:          "",
			isMountPointFile: false,
			getCommand: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "mount", device, mountPoint).Return(nil, assert.AnError)
				return mockCommand
			},
			getFilePathClient: func(controller *gomock.Controller) filepathwrapper.FilePath {
				mockFilePathClient := mock_filepathwrapper.NewMockFilePath(controller)
				mockFilePathClient.EXPECT().EvalSymlinks(strings.TrimPrefix(device, "/dev/")).Return(device, nil)
				return mockFilePathClient
			},
			getOsClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(selfMountInfoPath).Return(newMountInfoEntry().bytes(), nil).Times(4)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, nil)
				return mockOsClient
			},
			assertError: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getOsClient(ctrl), params.getFilePathClient(ctrl), params.getCommand(ctrl))
			err := client.MountDevice(context.Background(), device, mountPoint, params.options, params.isMountPointFile)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestLinuxClient_createMountPoint(t *testing.T) {
	type parameters struct {
		isMountPointFile bool
		getOSClient      func(controller *gomock.Controller) oswrapper.OS
	}
	const mountPoint = "/mount/point"
	tests := map[string]parameters{
		"mount point is a file": {
			isMountPointFile: true,
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, assert.AnError)
				return mockOsClient
			},
		},
		"mount point is a directory": {
			isMountPointFile: false,
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, assert.AnError)
				return mockOsClient
			},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getOSClient(ctrl), nil, nil)
			client.createMountPoint(context.Background(), mountPoint, params.isMountPointFile)
		})
	}
}

func TestLinuxClient_pathExists(t *testing.T) {
	type parameters struct {
		expectedResult bool
		getOSClient    func(controller *gomock.Controller) oswrapper.OS
	}
	const path = "/path"
	tests := map[string]parameters{
		"path exists on the filesystem": {
			expectedResult: true,
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().Stat(path).Return(nil, nil)
				return mockOSClient
			},
		},
		"path does not exist on the filesystem": {
			expectedResult: false,
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().Stat(path).Return(nil, os.ErrNotExist)
				return mockOSClient
			},
		},
		"error checking if path exists": {
			expectedResult: false,
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().Stat(path).Return(nil, assert.AnError)
				return mockOSClient
			},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getOSClient(ctrl), nil, nil)

			assert.Equal(t, params.expectedResult, client.PathExists(path))
		})
	}
}

func TestLinuxClient_EnsureFileExists(t *testing.T) {
	type parameters struct {
		getOSClient func(controller *gomock.Controller) oswrapper.OS
		assertError assert.ErrorAssertionFunc
	}

	const path = "/path"

	tests := map[string]parameters{
		"provided path is a directory": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().Stat(path).Return(&mockFileInfo{isDir: true}, nil)
				return mockOSClient
			},
			assertError: assert.Error,
		},
		"provided path is a file": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().Stat(path).Return(&mockFileInfo{isDir: false}, nil)
				return mockOSClient
			},
			assertError: assert.NoError,
		},
		"unable to determine if path exists": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().Stat(path).Return(nil, assert.AnError)
				return mockOSClient
			},
			assertError: assert.Error,
		},
		"file does not exist and unable to create it": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().Stat(path).Return(nil, os.ErrNotExist)
				mockOSClient.EXPECT().OpenFile(path, os.O_CREATE|os.O_TRUNC, fs.FileMode(0o600)).Return(nil, assert.AnError)
				return mockOSClient
			},
			assertError: assert.Error,
		},
		"file does not exist and is created successfully": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().Stat(path).Return(nil, os.ErrNotExist)
				mockOSClient.EXPECT().OpenFile(path, os.O_CREATE|os.O_TRUNC, fs.FileMode(0o600)).Return(nil, nil)
				return mockOSClient
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getOSClient(ctrl), nil, nil)

			err := client.EnsureFileExists(context.Background(), path)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestLinuxClient_EnsureDirExists(t *testing.T) {
	type parameters struct {
		getOSClient func(controller *gomock.Controller) oswrapper.OS
		assertError assert.ErrorAssertionFunc
	}
	const path = "/path"
	tests := map[string]parameters{
		"path is a file": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().Stat(path).Return(&mockFileInfo{isDir: false}, nil)
				return mockOSClient
			},
			assertError: assert.Error,
		},
		"path is a directory": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().Stat(path).Return(&mockFileInfo{isDir: true}, nil)
				return mockOSClient
			},
			assertError: assert.NoError,
		},
		"unable to determine if path exists": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().Stat(path).Return(nil, assert.AnError)
				return mockOSClient
			},
			assertError: assert.Error,
		},
		"directory does not exist and unable to create it": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().Stat(path).Return(nil, os.ErrNotExist)
				mockOSClient.EXPECT().MkdirAll(path, fs.FileMode(0o755)).Return(assert.AnError)
				return mockOSClient
			},
		},
		"directory does not exist and is created successfully": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().Stat(path).Return(nil, os.ErrNotExist)
				mockOSClient.EXPECT().MkdirAll(path, fs.FileMode(0o755)).Return(nil)
				return mockOSClient
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getOSClient(ctrl), nil, nil)

			err := client.EnsureDirExists(context.Background(), path)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestLinuxClient_RemountDevice(t *testing.T) {
	type parameters struct {
		options     string
		getCommand  func(controller *gomock.Controller) exec.Command
		assertError assert.ErrorAssertionFunc
	}

	const mountPoint = "/mount/point"
	const defaultOptions = "rw"
	const defaultInputOptions = "-o " + defaultOptions

	tests := map[string]parameters{
		"options provided": {
			options: defaultInputOptions,
			getCommand: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "mount", "-o", defaultOptions, mountPoint).Return(nil, nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"no options provided": {
			options: "",
			getCommand: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "mount", mountPoint).Return(nil, nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"error executing command": {
			options: "",
			getCommand: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "mount", mountPoint).Return(nil, assert.AnError)
				return mockCommand
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(nil, nil, params.getCommand(ctrl))

			err := client.RemountDevice(context.Background(), mountPoint, params.options)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestLinuxClient_Umount(t *testing.T) {
	type parameters struct {
		getCommandClient func(controller *gomock.Controller) exec.Command
		assertError      assert.ErrorAssertionFunc
	}

	const mountPoint = "/path"
	tests := map[string]parameters{
		"device does not exist": {
			getCommandClient: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "umount", umountTimeout, true, mountPoint).
					Return([]byte(umountNotMounted), assert.AnError)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"error unmounting device": {
			getCommandClient: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "umount", umountTimeout, true, mountPoint).
					Return(nil, assert.AnError)
				return mockCommand
			},
			assertError: assert.Error,
		},
		"timeout while unmounting device": {
			getCommandClient: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "umount", umountTimeout, true, mountPoint).
					Return(nil, errors.TimeoutError("timeout"))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "umount", umountTimeout, true, mountPoint, "-f").
					Return(nil, nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"device not mounted while force unmounting device": {
			getCommandClient: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "umount", umountTimeout, true, mountPoint).
					Return(nil, errors.TimeoutError("timeout"))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "umount", umountTimeout, true, mountPoint, "-f").
					Return([]byte(umountNotMounted), assert.AnError)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"error force unmounting device": {
			getCommandClient: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "umount", umountTimeout, true, mountPoint).
					Return(nil, errors.TimeoutError("timeout"))
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "umount", umountTimeout, true, mountPoint, "-f").
					Return(nil, assert.AnError)
				return mockCommand
			},
			assertError: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(nil, nil, params.getCommandClient(ctrl))

			err := client.Umount(context.Background(), mountPoint)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestLinuxClient_RemoveMountPoint(t *testing.T) {
	type parameters struct {
		getCommandClient func(controller *gomock.Controller) exec.Command
		getOSClient      func(controller *gomock.Controller) oswrapper.OS
		assertError      assert.ErrorAssertionFunc
	}

	const mountPoint = "/path"

	tests := map[string]parameters{
		"mount point does not exist": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, os.ErrNotExist)
				return mockOsClient
			},
			getCommandClient: func(controller *gomock.Controller) exec.Command {
				mockCommandClient := mock_exec.NewMockCommand(controller)
				return mockCommandClient
			},
			assertError: assert.NoError,
		},
		"error determining if mount point exists": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, assert.AnError)
				return mockOsClient
			},
			getCommandClient: func(controller *gomock.Controller) exec.Command {
				mockCommandClient := mock_exec.NewMockCommand(controller)
				return mockCommandClient
			},
			assertError: assert.Error,
		},
		"error running umount": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, nil)
				return mockOsClient
			},
			getCommandClient: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "umount", umountTimeout, true, mountPoint).
					Return(nil, assert.AnError)
				return mockCommand
			},
			assertError: assert.Error,
		},
		"error removing mount point": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, nil)
				mockOsClient.EXPECT().Remove(mountPoint).Return(assert.AnError)
				return mockOsClient
			},
			getCommandClient: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "umount", umountTimeout, true, mountPoint).
					Return([]byte(umountNotMounted), assert.AnError)
				return mockCommand
			},
			assertError: assert.Error,
		},
		"happy path": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().Stat(mountPoint).Return(nil, nil)
				mockOsClient.EXPECT().Remove(mountPoint).Return(nil)
				return mockOsClient
			},
			getCommandClient: func(controller *gomock.Controller) exec.Command {
				mockCommand := mock_exec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "umount", umountTimeout, true, mountPoint).
					Return([]byte(umountNotMounted), assert.AnError)
				return mockCommand
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getOSClient(ctrl), nil, params.getCommandClient(ctrl))

			err := client.RemoveMountPoint(context.Background(), mountPoint)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestLinuxClient_MountSMBPath(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result := client.MountSMBPath(ctx, "\\export\\path", "\\mount\\path", "test-user", "password")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestLinuxClient_UmountSMBPath(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result := client.UmountSMBPath(ctx, "", "test-target")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestLinuxClient_WindowsBindMount(t *testing.T) {
	ctx := context.Background()
	client, err := newOsSpecificClient()
	assert.NoError(t, err)
	result := client.WindowsBindMount(ctx, "test-source", "test-target", []string{"test-val1", "test-val2"})
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestLinuxClient_IsCompatible(t *testing.T) {
	type parameters struct {
		protocol    string
		assertError assert.ErrorAssertionFunc
	}

	tests := map[string]parameters{
		"nfs": {
			protocol:    "nfs",
			assertError: assert.NoError,
		},
		"smb": {
			protocol:    "smb",
			assertError: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			client, err := newOsSpecificClient()
			assert.NoError(t, err)

			err = client.IsCompatible(context.Background(), params.protocol)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestLinuxClient_ConsistentRead(t *testing.T) {
	type parameters struct {
		attempts        int
		getOSClient     func(controller *gomock.Controller) oswrapper.OS
		expectedContent string
		assertError     assert.ErrorAssertionFunc
	}

	const fileName = "/file"

	tests := map[string]parameters{
		"error reading file": {
			attempts: 0,
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().ReadFile(fileName).Return(nil, assert.AnError)
				return mockOSClient
			},
			expectedContent: "",
			assertError:     assert.Error,
		},
		"error reading the file on second attempt": {
			attempts: 1,
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().ReadFile(fileName).Return(nil, nil)
				mockOSClient.EXPECT().ReadFile(fileName).Return(nil, assert.AnError)
				return mockOSClient
			},
			expectedContent: "",
			assertError:     assert.Error,
		},
		"content mismatch on second attempt ,with only one attempt configured": {
			attempts: 1,
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().ReadFile(fileName).Return([]byte("content"), nil)
				mockOSClient.EXPECT().ReadFile(fileName).Return([]byte("different content"), nil)
				return mockOSClient
			},
			expectedContent: "",
			assertError:     assert.Error,
		},
		"consistent read on second attempt": {
			attempts: 2,
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().ReadFile(fileName).Return([]byte("content"), nil)
				mockOSClient.EXPECT().ReadFile(fileName).Return([]byte("content"), nil)
				return mockOSClient
			},
			expectedContent: "content",
			assertError:     assert.NoError,
		},
		"consistent read on third attempt": {
			attempts: 2,
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOSClient := mock_oswrapper.NewMockOS(controller)
				mockOSClient.EXPECT().ReadFile(fileName).Return([]byte("content"), nil)
				mockOSClient.EXPECT().ReadFile(fileName).Return([]byte("other content"), nil)
				mockOSClient.EXPECT().ReadFile(fileName).Return([]byte("other content"), nil)
				return mockOSClient
			},
			expectedContent: "other content",
			assertError:     assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getOSClient(ctrl), nil, nil)

			content, err := client.ConsistentRead(fileName, params.attempts)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedContent, string(content))
		})
	}
}

func TestParseProcMountInfo(t *testing.T) {
	type parameters struct {
		content     string
		expected    []models.MountInfo
		assertError assert.ErrorAssertionFunc
	}

	tests := map[string]parameters{
		"10-13 fields": {
			content: `40 35 0:34 / /sys/fs/cgroup/cpu,cpuacct rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw,cpu,cpuacct
46 35 0:40 / /sys/fs/cgroup/blkio rw,nosuid,nodev,noexec,relatime shared:21 - cgroup cgroup rw,blkio
47 35 0:41 / /sys/fs/cgroup/rdma rw,nosuid,nodev,noexec,relatime shared:22 master:1 - cgroup cgroup rw,rdma
48 35 0:42 / /sys/fs/cgroup/devices rw,nosuid,nodev,noexec,relatime shared:23 shared:74 master:2 - cgroup cgroup rw,devices
`,
			expected: []models.MountInfo{
				{
					MountId:      40,
					ParentId:     35,
					DeviceId:     "0:34",
					Root:         "/",
					MountPoint:   "/sys/fs/cgroup/cpu,cpuacct",
					MountOptions: []string{"rw", "nosuid", "nodev", "noexec", "relatime"},
					FsType:       "cgroup",
					MountSource:  "cgroup",
					SuperOptions: []string{"rw", "cpu", "cpuacct"},
				},
				{
					MountId:      46,
					ParentId:     35,
					DeviceId:     "0:40",
					Root:         "/",
					MountPoint:   "/sys/fs/cgroup/blkio",
					MountOptions: []string{"rw", "nosuid", "nodev", "noexec", "relatime"},
					FsType:       "cgroup",
					MountSource:  "cgroup",
					SuperOptions: []string{"rw", "blkio"},
				},
				{
					MountId:      47,
					ParentId:     35,
					DeviceId:     "0:41",
					Root:         "/",
					MountPoint:   "/sys/fs/cgroup/rdma",
					MountOptions: []string{"rw", "nosuid", "nodev", "noexec", "relatime"},
					FsType:       "cgroup",
					MountSource:  "cgroup",
					SuperOptions: []string{"rw", "rdma"},
				},
				{
					MountId:      48,
					ParentId:     35,
					DeviceId:     "0:42",
					Root:         "/",
					MountPoint:   "/sys/fs/cgroup/devices",
					MountOptions: []string{"rw", "nosuid", "nodev", "noexec", "relatime"},
					FsType:       "cgroup",
					MountSource:  "cgroup",
					SuperOptions: []string{"rw", "devices"},
				},
			},
			assertError: assert.NoError,
		},
		"one valid one invalid": {
			content: `36 35 0:30 / /sys/fs/cgroup/unified rw,nosuid,nodev,noexec,relatime shared:10 - cgroup2 cgroup2 rw
47 35 0:41 / rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw,rdma`,
			assertError: assert.Error,
		},
		"too few fields": {
			content:     `36 35 0:30 / /sys/fs/cgroup/unified - cgroup2 cgroup2 rw`,
			assertError: assert.Error,
		},
		"separator in 5th position": {
			content:     `49 35 0:43 / /sys/fs/cgroup/pids rw,nosuid,nodev,noexec,relatime - shared:24 cgroup cgroup rw,pids`,
			assertError: assert.Error,
		},
		"no separator": {
			content:     `52 26 0:12 / /sys/kernel/tracing rw,nosuid,nodev,noexec,relatime shared:27 tracefs tracefs rw`,
			assertError: assert.Error,
		},
		"root marked as deleted": {
			content:  newMountInfoEntry().withRoot("deleted").string(),
			expected: []models.MountInfo{},
		},
		"mountID is not a number": {
			content: "a 35 0:34 / /sys/fs/cgroup/cpu,cpuacct rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw," +
				"cpu,cpuacct",
			assertError: assert.Error,
		},
		"rootID is not a number": {
			content: "40 a 0:34 / /sys/fs/cgroup/cpu,cpuacct rw,nosuid,nodev,noexec,relatime - cgroup cgroup rw," +
				"cpu,cpuacct",
			assertError: assert.Error,
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			m, err := parseProcMountInfo([]byte(params.content))
			if params.assertError != nil {
				params.assertError(t, err)
			}

			assert.Equal(t, params.expected, m)
		})
	}
}

func TestLinuxClient_ListProcMounts(t *testing.T) {
	type parameters struct {
		getOSClient    func(controller *gomock.Controller) oswrapper.OS
		expectedResult []models.MountPoint
		assertError    assert.ErrorAssertionFunc
	}

	const mountFilePath = "/proc/mounts"

	tests := map[string]parameters{
		"error reading mounts file": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(mountFilePath).Return(nil, assert.AnError)
				return mockOsClient
			},
			assertError:    assert.Error,
			expectedResult: nil,
		},
		"happy path": {
			getOSClient: func(controller *gomock.Controller) oswrapper.OS {
				mockOsClient := mock_oswrapper.NewMockOS(controller)
				mockOsClient.EXPECT().ReadFile(mountFilePath).Return(nil, nil).Times(2)
				return mockOsClient
			},
			assertError:    assert.NoError,
			expectedResult: []models.MountPoint{},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getOSClient(ctrl), nil, nil)
			result, err := client.ListProcMounts(mountFilePath)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedResult, result)
		})
	}
}

func TestLinuxClient_ConsistentReadMount(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := context.Background

	// Creating a mountInfoFile with random 100 entries
	randomMountInfoFile := func() string {
		var file strings.Builder
		for i := 0; i < 100; i++ {
			tempEntry := newMountInfoEntryRandom(nil)
			file.WriteString(tempEntry.string())
			file.WriteString("\n")
		}
		return file.String()
	}()

	tests := []struct {
		name               string
		mockOSExpectations func(mockOS *mock_oswrapper.MockOS, mountFileInfo string, attempt int)
		attemptFound       int
		err                bool
		matched            bool
	}{
		{
			name: "mountInfo file contains 100 entry, and needed entry is found consistently",
			mockOSExpectations: func(mockOS *mock_oswrapper.MockOS, mountFileInfo string, attempt int) {
				lines := strings.Split(mountFileInfo, "\n")
				for i := 0; i < attempt; i++ {
					indexRandom := rand.Intn(len(lines))
					lines[indexRandom] = newMountInfoEntry().string()
					mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)
				}
			},
			err:          false,
			matched:      true,
			attemptFound: 2,
		},
		{
			name: "mountInfo file contains 100 entry, and needed entry is found in the second and third attempt",
			mockOSExpectations: func(mockOS *mock_oswrapper.MockOS, mountFileInfo string, attempt int) {
				// First time we get the incomplete line.
				lines := strings.Split(mountFileInfo, "\n")
				indexRandom := rand.Intn(len(lines))
				entry := newMountInfoEntry()
				entry.mountInfo.SuperOptions = nil
				entry.mountInfo.MountSource = ""
				lines[indexRandom] = entry.string()
				mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)

				// Then after first attempt, we start getting the full entry
				for i := 1; i < attempt; i++ {
					newEntry := newMountInfoEntry()
					lines[indexRandom] = newEntry.string()
					mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)
				}
			},
			err:          false,
			matched:      true,
			attemptFound: 3,
		},
		{
			name: "mountInfo file contains 100 entry, and needed entry is not found even after exhausting all the retries",
			mockOSExpectations: func(mockOS *mock_oswrapper.MockOS, mountFileInfo string, attempt int) {
				for i := 0; i < attempt; i++ {
					mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(mountFileInfo), nil).Times(1)
				}
			},
			err:          false,
			matched:      false,
			attemptFound: 4,
		},
		{
			name: "mountInfo file contains 100 entry, and needed entry disappears in-between",
			mockOSExpectations: func(mockOS *mock_oswrapper.MockOS, mountFileInfo string, attempt int) {
				// First time we get the needed line.
				lines := strings.Split(mountFileInfo, "\n")
				indexRandom := rand.Intn(len(lines))
				entry := newMountInfoEntry()
				lines[indexRandom] = entry.string()
				mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)

				// Second attempt, the line disappears
				lines[indexRandom] = newMountInfoEntryRandom(nil).string()
				mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)

				// Third attempt, it reappears again
				lines[indexRandom] = entry.string()
				mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)

				// Fourth attempt the line is matched again
				mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)
			},
			err:          false,
			matched:      true,
			attemptFound: 4, // not needed here in this test case
		},
		{
			name: "mountInfo file contains 100 entry, and needed entry was only found once",
			mockOSExpectations: func(mockOS *mock_oswrapper.MockOS, mountFileInfo string, attempt int) {
				// First time we get the needed line.
				lines := strings.Split(mountFileInfo, "\n")
				indexRandom := rand.Intn(len(lines))
				entry := newMountInfoEntry()
				lines[indexRandom] = entry.string()
				mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)

				// Second attempt and afterward it disappears.
				for i := 1; i < attempt; i++ {
					mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(mountFileInfo), nil).Times(1)
				}
			},
			err:          true,
			matched:      false,
			attemptFound: 4, // not needed here in this test case
		},
		{
			name: "mountInfo file contains 100 entry, and needed entry was founc complete in the second and third attempt",
			mockOSExpectations: func(mockOS *mock_oswrapper.MockOS, mountFileInfo string, attempt int) {
				// First time we get the needed line, but data is missing
				lines := strings.Split(mountFileInfo, "\n")
				indexRandom := rand.Intn(len(lines))
				entry := newMountInfoEntry()
				entry.mountInfo.FsType = "incomplete data"
				entry.mountInfo.MountOptions = []string{"incomplete data"}
				lines[indexRandom] = entry.string()
				mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)

				// Second attempt we get the needed line, but one of the data is missing
				entry = newMountInfoEntry()
				entry.mountInfo.MountOptions = []string{"incomplete data"}
				lines[indexRandom] = entry.string()
				mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)

				// Third and Fourth attempt we get the full needed line.
				entry = newMountInfoEntry()
				lines[indexRandom] = entry.string()
				mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(2)
			},
			err:          false,
			matched:      true,
			attemptFound: 4,
		},
		{
			name: "mountInfo file contains 100 entry, and needed entry was found complete at the last attempt.",
			mockOSExpectations: func(mockOS *mock_oswrapper.MockOS, mountFileInfo string, attempt int) {
				// First time we get the needed line, but data is missing
				lines := strings.Split(mountFileInfo, "\n")
				indexRandom := rand.Intn(len(lines))
				entry := newMountInfoEntry()
				entry.mountInfo.FsType = "incomplete data"
				entry.mountInfo.MountOptions = []string{"incomplete data"}
				entry.mountInfo.SuperOptions = []string{"incomplete data"}
				lines[indexRandom] = entry.string()
				mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)

				// Second attempt we get the needed line, but data is missing
				entry = newMountInfoEntry()
				entry.mountInfo.MountOptions = []string{"incomplete data"}
				entry.mountInfo.SuperOptions = []string{"incomplete data"}
				lines[indexRandom] = entry.string()
				mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)

				// Third attempt we get the needed line, but one of the data is still missing.
				entry = newMountInfoEntry()
				entry.mountInfo.SuperOptions = []string{"incomplete data"}
				lines[indexRandom] = entry.string()
				mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)

				// Fourth attempt, found the needed entry, data is complete, but its too late.
				entry = newMountInfoEntry()
				lines[indexRandom] = entry.string()
				mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)
			},
			err:          true,
			matched:      false,
			attemptFound: 4, // not needed here in this test case
		},
		{
			name: "mountInfo file contains 100 entry, parseProcMount returns an error for unrelated entry," +
				" but test passes in the first attempt",
			mockOSExpectations: func(mockOS *mock_oswrapper.MockOS, mountFileInfo string, attempt int) {
				// First time we get the incomplete line.
				lines := strings.Split(mountFileInfo, "\n")
				indexRandom := rand.Intn(len(lines))
				lines[indexRandom] = "Some wrong entry"

				for i := 0; i < attempt; i++ {
					// This indexRandom re-writing the above entry has the probability of (attempt * 1/100).
					indexRandom = rand.Intn(len(lines))
					lines[indexRandom] = newMountInfoEntry().string()
					mockOS.EXPECT().ReadFile(selfMountInfoPath).Return([]byte(strings.Join(lines, "\n")), nil).Times(1)
				}
			},
			err:          false,
			matched:      true,
			attemptFound: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockOS := mock_oswrapper.NewMockOS(ctrl)
			expectedEntry := &newMountInfoEntry().mountInfo
			tt.mockOSExpectations(mockOS, randomMountInfoFile, tt.attemptFound)

			client := &LinuxClient{
				os:       mockOS,
				filepath: nil,
				command:  nil,
			}

			sourcePath := strings.TrimPrefix(expectedEntry.MountSource, "/dev/")
			devicePath := strings.TrimPrefix(expectedEntry.MountSource, "/dev/")

			found, err := client.ConsistentReadMount(ctx(), selfMountInfoPath, expectedEntry.MountPoint, sourcePath, devicePath, maxListTries)
			if !tt.err {
				assert.NoError(t, err, "There shouldn't be any error")
			} else {
				assert.Errorf(t, err, "There should be an error")
			}

			if tt.matched {
				assert.Equal(t, found, &newMountInfoEntry().mountInfo, "Should be equal")
			} else {
				assert.NotEqual(t, found, nil, "Should be equal to nil")
			}
		})
	}
}

func TestParseProcMounts(t *testing.T) {
	type parameters struct {
		input          string
		assertError    assert.ErrorAssertionFunc
		expectedResult []models.MountPoint
	}

	tests := map[string]parameters{
		"less than 6 fields": {
			input:          "a b c d e",
			assertError:    assert.Error,
			expectedResult: nil,
		},
		"happy path": {
			input:       "/dev/loop17 /snap/helm/432 squashfs ro,nodev,relatime,errors=continue 0 0\n",
			assertError: assert.NoError,
			expectedResult: []models.MountPoint{
				{
					Device: "/dev/loop17",
					Path:   "/snap/helm/432",
					Type:   "squashfs",
					Opts:   []string{"ro", "nodev", "relatime", "errors=continue"},
					Freq:   0,
					Pass:   0,
				},
			},
		},
		"invalid frequency": {
			input:          "/dev/loop17 /snap/helm/432 squashfs ro,nodev,relatime,errors=continue a 0\n",
			assertError:    assert.Error,
			expectedResult: nil,
		},
		"invalid Pass": {
			input:          "/dev/loop17 /snap/helm/432 squashfs ro,nodev,relatime,errors=continue 0 a\n",
			assertError:    assert.Error,
			expectedResult: nil,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := parseProcMounts([]byte(params.input))
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedResult, result)
		})
	}
}

// ----------------------------- helpers -----------------------------
type mockFileInfo struct {
	os.FileInfo
	device uint64
	isDir  bool
}

func (s mockFileInfo) Sys() any {
	return &syscall.Stat_t{
		Dev: s.device,
	}
}

func (s mockFileInfo) IsDir() bool {
	return s.isDir
}

type mountInfoEntry struct {
	mountInfo models.MountInfo
}

func newMountInfoEntry() *mountInfoEntry {
	return &mountInfoEntry{
		mountInfo: models.MountInfo{
			MountId:      1,
			ParentId:     2,
			DeviceId:     "98:0",
			Root:         "/mnt1",
			MountPoint:   "/mnt2",
			MountOptions: []string{"rw", "relatime"},
			FsType:       "ext4",
			MountSource:  "/dev/mapper/3600a098038314461522451715a736f33",
			SuperOptions: []string{"rw", "data=ordered"},
		},
	}
}

var (
	randomMountPoint []string = []string{"/mnt3", "/mnt4", "/mnt5", "/mnt6", "/mnt7", "/mnt8", "/mnt9"}
	randomFSType     []string = []string{"ext4", "ext3", "ext2", "ext1", "xfs"}
)

func newMountInfoEntryRandom(mountOptions []string) *mountInfoEntry {
	return &mountInfoEntry{
		models.MountInfo{
			MountId:    rand.Intn(1001),
			ParentId:   rand.Intn(1001),
			DeviceId:   fmt.Sprintf("%d:0", rand.Intn(101)),
			Root:       "/mnt1",
			MountPoint: randomMountPoint[rand.Intn(len(randomMountPoint))],
			MountOptions: func() []string {
				if mountOptions == nil {
					return []string{"rw", "relatime"}
				}
				return mountOptions
			}(),
			FsType:       randomFSType[rand.Intn(len(randomFSType))],
			MountSource:  fmt.Sprintf("/dev/mapper/%s", uuid.New().String()),
			SuperOptions: []string{"rw", "data=ordered"},
		},
	}
}

func (m *mountInfoEntry) withRoot(root string) *mountInfoEntry {
	m.mountInfo.Root = root
	return m
}

func (m *mountInfoEntry) withMountPoint(mountPoint string) *mountInfoEntry {
	m.mountInfo.MountPoint = mountPoint
	return m
}

func (m *mountInfoEntry) withMountSource(mountSource string) *mountInfoEntry {
	m.mountInfo.MountSource = mountSource
	return m
}

func (m *mountInfoEntry) string() string {
	mountOptions := strings.Join(m.mountInfo.MountOptions, ",")
	superOptions := strings.Join(m.mountInfo.SuperOptions, ",")

	return fmt.Sprintf(
		"%d %d %s %s %s %s shared:0 - %s %s %s\n",
		m.mountInfo.MountId,
		m.mountInfo.ParentId,
		m.mountInfo.DeviceId,
		m.mountInfo.Root,
		m.mountInfo.MountPoint,
		mountOptions,
		m.mountInfo.FsType,
		m.mountInfo.MountSource,
		superOptions)
}

func (m *mountInfoEntry) bytes() []byte {
	return []byte(m.string())
}
