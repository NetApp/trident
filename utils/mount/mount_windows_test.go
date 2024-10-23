// Copyright 2024 NetApp, Inc. All Rights Reserved.

package mount

import (
	"context"
	"testing"

	v1 "github.com/kubernetes-csi/csi-proxy/client/api/filesystem/v1"
	smb "github.com/kubernetes-csi/csi-proxy/client/api/smb/v1"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_mount/mock_filesystem"
	"github.com/netapp/trident/mocks/mock_utils/mock_mount/mock_smb"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/errors"
)

func TestNewOsSpecificClientDetailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFSClient := mock_filesystem.NewMockFilesystemClient(ctrl)
	mockSmbClient := mock_smb.NewMockSmbClient(ctrl)
	client := newOsSpecificClientDetailed(mockFSClient, mockSmbClient)
	assert.NotNil(t, client)
}

func TestWindowsClient_IsMounted(t *testing.T) {
	type parameters struct {
		getFileSystemClient func(controller *gomock.Controller) FilesystemClient
		expectedResult      bool
		assertError         assert.ErrorAssertionFunc
	}

	const sourceDevice = "source-dev"
	const mountPoint = "c:\\mount\\point"
	const mountOptions = "test-mountoptions"

	request := v1.PathExistsRequest{Path: normalizeWindowsPath(mountPoint)}

	tests := map[string]parameters{
		"path does not exist": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &request).Return(
					&v1.PathExistsResponse{Exists: false}, nil)
				return mockFSClient
			},
			expectedResult: false,
			assertError:    assert.NoError,
		},
		"error determining path existence": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &request).Return(nil,
					assert.AnError)
				return mockFSClient
			},
			expectedResult: false,
			assertError:    assert.Error,
		},
		"path exists": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &request).Return(
					&v1.PathExistsResponse{Exists: true}, nil)
				return mockFSClient
			},
			expectedResult: true,
			assertError:    assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getFileSystemClient(ctrl), nil)
			result, err := client.IsMounted(context.Background(), sourceDevice, mountPoint, mountOptions)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedResult, result)
		})
	}
}

func TestWindowsClient_Umount(t *testing.T) {
	type parameters struct {
		getFileSystemClient func(controller *gomock.Controller) FilesystemClient
		assertError         assert.ErrorAssertionFunc
	}

	const mountPoint = "c:\\mount\\point"

	request := v1.RmdirRequest{Path: normalizeWindowsPath(mountPoint), Force: true}

	tests := map[string]parameters{
		"error deleting mount point": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().Rmdir(gomock.Any(), &request).Return(nil, assert.AnError)
				return mockFSClient
			},
			assertError: assert.Error,
		},
		"happy path": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().Rmdir(gomock.Any(), &request).Return(nil, nil)
				return mockFSClient
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			client := newOsSpecificClientDetailed(params.getFileSystemClient(ctrl), nil)
			err := client.Umount(context.Background(), mountPoint)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestWindowsClient_IsLikelyNotMountPoint(t *testing.T) {
	type parameters struct {
		getFileSystemClient func(controller *gomock.Controller) FilesystemClient
		expectedResult      bool
		assertError         assert.ErrorAssertionFunc
	}

	const mountPoint = "c:\\mount\\point"
	pathExistsRequest := v1.PathExistsRequest{Path: normalizeWindowsPath(mountPoint)}
	isSymlinkRequest := v1.IsSymlinkRequest{Path: normalizeWindowsPath(mountPoint)}

	tests := map[string]parameters{
		"path does not exist": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &pathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: false}, nil)
				return mockFSClient
			},
			expectedResult: true,
			assertError:    assert.Error,
		},
		"error determining path existence": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &pathExistsRequest).Return(nil,
					assert.AnError)
				return mockFSClient
			},
			expectedResult: false,
			assertError:    assert.Error,
		},
		"path is not symlink": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &pathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: true}, nil)
				mockFSClient.EXPECT().IsSymlink(gomock.Any(), &isSymlinkRequest).Return(
					&v1.IsSymlinkResponse{IsSymlink: false}, nil)
				return mockFSClient
			},
			expectedResult: true,
			assertError:    assert.NoError,
		},
		"error determining if path is symlink": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &pathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: true}, nil)
				mockFSClient.EXPECT().IsSymlink(gomock.Any(), &isSymlinkRequest).Return(nil,
					assert.AnError)
				return mockFSClient
			},
			expectedResult: false,
			assertError:    assert.Error,
		},
		"happy path": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &pathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: true}, nil)
				mockFSClient.EXPECT().IsSymlink(gomock.Any(), &isSymlinkRequest).Return(
					&v1.IsSymlinkResponse{IsSymlink: true}, nil)
				return mockFSClient
			},
			expectedResult: false,
			assertError:    assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getFileSystemClient(ctrl), nil)
			result, err := client.IsLikelyNotMountPoint(context.Background(), mountPoint)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedResult, result)
		})
	}
}

func TestWindowsClient_MountNFSPath(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockFSClient := mock_filesystem.NewMockFilesystemClient(ctrl)
	mockSmbClient := mock_smb.NewMockSmbClient(ctrl)
	client := newOsSpecificClientDetailed(mockFSClient, mockSmbClient)

	err := client.MountNFSPath(ctx, "/export/path", "/mount/path", "test-options")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestWindowsClient_MountSMBPath(t *testing.T) {
	type parameters struct {
		username            string
		password            string
		getFileSystemClient func(controller *gomock.Controller) FilesystemClient
		getSMBClient        func(controller *gomock.Controller) SmbClient
		assertError         assert.ErrorAssertionFunc
	}

	const exportPath = "smb:\\\\server\\share"
	const parentDirectory = "c:\\mount"
	const mountPoint = parentDirectory + "\\point"
	const username = "test-username"
	const password = "test-password"

	pathExistsRequest := v1.PathExistsRequest{Path: normalizeWindowsPath(mountPoint)}
	parentDirectoryPathExistsRequest := v1.PathExistsRequest{Path: normalizeWindowsPath(parentDirectory)}
	rmdirRequest := v1.RmdirRequest{Path: normalizeWindowsPath(mountPoint), Force: true}
	mkdirRequest := v1.MkdirRequest{Path: normalizeWindowsPath(parentDirectory)}
	smbRequest := &smb.NewSmbGlobalMappingRequest{
		LocalPath:  normalizeWindowsPath(mountPoint),
		RemotePath: exportPath,
		Username:   username,
		Password:   password,
	}

	tests := map[string]parameters{
		"error determining if path exists": {
			username: username,
			password: password,
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &pathExistsRequest).Return(nil,
					assert.AnError)
				return mockFSClient
			},
			getSMBClient: func(controller *gomock.Controller) SmbClient {
				mockSMBClient := mock_smb.NewMockSmbClient(controller)
				return mockSMBClient
			},
			assertError: assert.Error,
		},
		"error removing path when it already exists": {
			username: username,
			password: password,
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &pathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: true}, nil)
				mockFSClient.EXPECT().Rmdir(gomock.Any(), &rmdirRequest).Return(nil, assert.AnError)
				return mockFSClient
			},
			getSMBClient: func(controller *gomock.Controller) SmbClient {
				mockSMBClient := mock_smb.NewMockSmbClient(controller)
				return mockSMBClient
			},
			assertError: assert.Error,
		},
		"bad username": {
			username: "",
			password: password,
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &pathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: false}, nil)
				return mockFSClient
			},
			getSMBClient: func(controller *gomock.Controller) SmbClient {
				mockSMBClient := mock_smb.NewMockSmbClient(controller)
				return mockSMBClient
			},
			assertError: assert.Error,
		},
		"bad password": {
			username: username,
			password: "",
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &pathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: false}, nil)
				return mockFSClient
			},
			getSMBClient: func(controller *gomock.Controller) SmbClient {
				mockSMBClient := mock_smb.NewMockSmbClient(controller)
				return mockSMBClient
			},
			assertError: assert.Error,
		},
		"error determining if parent directory exists": {
			username: username,
			password: password,
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &pathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: false}, nil)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &parentDirectoryPathExistsRequest).Return(
					nil, assert.AnError)
				return mockFSClient
			},
			getSMBClient: func(controller *gomock.Controller) SmbClient {
				mockSMBClient := mock_smb.NewMockSmbClient(controller)
				return mockSMBClient
			},
			assertError: assert.Error,
		},
		"parent directory does not exist, failure making parent directory": {
			username: username,
			password: password,
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &pathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: false}, nil)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &parentDirectoryPathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: false}, nil)
				mockFSClient.EXPECT().Mkdir(gomock.Any(), &mkdirRequest).Return(nil, assert.AnError)
				return mockFSClient
			},
			getSMBClient: func(controller *gomock.Controller) SmbClient {
				mockSMBClient := mock_smb.NewMockSmbClient(controller)
				return mockSMBClient
			},
			assertError: assert.Error,
		},
		"error adding SMB mapping": {
			username: username,
			password: password,
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &pathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: false}, nil)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &parentDirectoryPathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: true}, nil)
				return mockFSClient
			},
			getSMBClient: func(controller *gomock.Controller) SmbClient {
				mockSMBClient := mock_smb.NewMockSmbClient(controller)
				mockSMBClient.EXPECT().NewSmbGlobalMapping(gomock.Any(), smbRequest).Return(nil, assert.AnError)
				return mockSMBClient
			},
			assertError: assert.Error,
		},
		"happy path": {
			username: username,
			password: password,
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &pathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: false}, nil)
				mockFSClient.EXPECT().PathExists(gomock.Any(), &parentDirectoryPathExistsRequest).Return(
					&v1.PathExistsResponse{Exists: true}, nil)
				return mockFSClient
			},
			getSMBClient: func(controller *gomock.Controller) SmbClient {
				mockSMBClient := mock_smb.NewMockSmbClient(controller)
				mockSMBClient.EXPECT().NewSmbGlobalMapping(gomock.Any(), smbRequest).Return(nil, nil)
				return mockSMBClient
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getFileSystemClient(ctrl), params.getSMBClient(ctrl))

			err := client.MountSMBPath(context.Background(), exportPath, mountPoint, params.username, params.password)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestWindowsClient_UmountSMBPath(t *testing.T) {
	type parameters struct {
		getFileSystemClient func(controller *gomock.Controller) FilesystemClient
		getSMBClient        func(controller *gomock.Controller) SmbClient
		assertError         assert.ErrorAssertionFunc
	}

	const mappingPath = "smb:\\\\server\\share"
	const mountPoint = "c:\\mount\\point"
	smbRemoveMappingRequest := smb.RemoveSmbGlobalMappingRequest{RemotePath: normalizeWindowsPath(mappingPath)}
	tests := map[string]parameters{
		"error deleting mount point": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				request := v1.RmdirRequest{Path: normalizeWindowsPath(mountPoint), Force: true}
				mockFSClient.EXPECT().Rmdir(gomock.Any(), &request).Return(nil, assert.AnError)
				return mockFSClient
			},
			getSMBClient: func(controller *gomock.Controller) SmbClient {
				mockSMBClient := mock_smb.NewMockSmbClient(controller)
				return mockSMBClient
			},
			assertError: assert.Error,
		},
		"error removing SMB mapping": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				request := v1.RmdirRequest{Path: normalizeWindowsPath(mountPoint), Force: true}
				mockFSClient.EXPECT().Rmdir(gomock.Any(), &request).Return(nil, nil)
				return mockFSClient
			},
			getSMBClient: func(controller *gomock.Controller) SmbClient {
				mockSMBClient := mock_smb.NewMockSmbClient(controller)
				mockSMBClient.EXPECT().RemoveSmbGlobalMapping(gomock.Any(), &smbRemoveMappingRequest).Return(
					nil, assert.AnError)
				return mockSMBClient
			},
			assertError: assert.Error,
		},
		"happy path": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)
				request := v1.RmdirRequest{Path: normalizeWindowsPath(mountPoint), Force: true}
				mockFSClient.EXPECT().Rmdir(gomock.Any(), &request).Return(nil, nil)
				return mockFSClient
			},
			getSMBClient: func(controller *gomock.Controller) SmbClient {
				mockSMBClient := mock_smb.NewMockSmbClient(controller)
				mockSMBClient.EXPECT().RemoveSmbGlobalMapping(gomock.Any(), &smbRemoveMappingRequest).Return(
					nil, nil)
				return mockSMBClient
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getFileSystemClient(ctrl), params.getSMBClient(ctrl))

			err := client.UmountSMBPath(context.Background(), mappingPath, mountPoint)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestWindowsClient_WindowsBindMount(t *testing.T) {
	type parameters struct {
		getFileSystemClient func(controller *gomock.Controller) FilesystemClient
		assertError         assert.ErrorAssertionFunc
	}

	const sourcePath = "c:\\source\\path"
	const targetPath = "c:\\target\\path"
	options := []string{"mock", "options"}

	rmdirRequest := v1.RmdirRequest{Path: normalizeWindowsPath(targetPath), Force: true}
	createSymlinkRequest := v1.CreateSymlinkRequest{
		SourcePath: normalizeWindowsPath(sourcePath),
		TargetPath: normalizeWindowsPath(targetPath),
	}

	tests := map[string]parameters{
		"error removing target path": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)

				mockFSClient.EXPECT().Rmdir(gomock.Any(), &rmdirRequest).Return(nil, assert.AnError)
				return mockFSClient
			},
			assertError: assert.Error,
		},
		"error executing the bind mount": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)

				mockFSClient.EXPECT().Rmdir(gomock.Any(), &rmdirRequest).Return(nil, nil)
				mockFSClient.EXPECT().CreateSymlink(gomock.Any(), &createSymlinkRequest).Return(
					nil, assert.AnError)
				return mockFSClient
			},
			assertError: assert.Error,
		},
		"happy path": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFSClient := mock_filesystem.NewMockFilesystemClient(controller)

				mockFSClient.EXPECT().Rmdir(gomock.Any(), &rmdirRequest).Return(nil, nil)
				mockFSClient.EXPECT().CreateSymlink(gomock.Any(), &createSymlinkRequest).Return(nil, nil)
				return mockFSClient
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getFileSystemClient(ctrl), nil)

			err := client.WindowsBindMount(context.Background(), sourcePath, targetPath, options)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestWindowsClient_IsNFSShareMounted(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockFSClient := mock_filesystem.NewMockFilesystemClient(ctrl)
	mockSmbClient := mock_smb.NewMockSmbClient(ctrl)
	client := newOsSpecificClientDetailed(mockFSClient, mockSmbClient)
	result, err := client.IsNFSShareMounted(ctx, "\\export\\path", "\\mount\\path")
	assert.False(t, result, "nfs share is mounted")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestWindowsClient_MountDevice(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockFSClient := mock_filesystem.NewMockFilesystemClient(ctrl)
	mockSmbClient := mock_smb.NewMockSmbClient(ctrl)
	client := newOsSpecificClientDetailed(mockFSClient, mockSmbClient)
	result := client.MountDevice(ctx, "\\device\\path", "\\mount\\path", "", false)
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestWindowsClient_RemoveMountPoint(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockFSClient := mock_filesystem.NewMockFilesystemClient(ctrl)
	mockSmbClient := mock_smb.NewMockSmbClient(ctrl)
	client := newOsSpecificClientDetailed(mockFSClient, mockSmbClient)
	result := client.RemoveMountPoint(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestWindowsClient_UmountAndRemoveTemporaryMountPoint(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockFSClient := mock_filesystem.NewMockFilesystemClient(ctrl)
	mockSmbClient := mock_smb.NewMockSmbClient(ctrl)
	client := newOsSpecificClientDetailed(mockFSClient, mockSmbClient)
	result := client.UmountAndRemoveTemporaryMountPoint(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestWindowsClient_UmountAndRemoveMountPoint(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockFSClient := mock_filesystem.NewMockFilesystemClient(ctrl)
	mockSmbClient := mock_smb.NewMockSmbClient(ctrl)
	client := newOsSpecificClientDetailed(mockFSClient, mockSmbClient)
	result := client.UmountAndRemoveMountPoint(ctx, "\\mount\\path")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestWindowsClient_RemountDevice(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockFSClient := mock_filesystem.NewMockFilesystemClient(ctrl)
	mockSmbClient := mock_smb.NewMockSmbClient(ctrl)
	client := newOsSpecificClientDetailed(mockFSClient, mockSmbClient)
	result := client.RemountDevice(ctx, "\\mount\\path", "")
	assert.Error(t, result, "no error")
	assert.True(t, errors.IsUnsupportedError(result), "not UnsupportedError")
}

func TestWindowsClient_GetSelfMountInfo(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockFSClient := mock_filesystem.NewMockFilesystemClient(ctrl)
	mockSmbClient := mock_smb.NewMockSmbClient(ctrl)
	client := newOsSpecificClientDetailed(mockFSClient, mockSmbClient)
	result, err := client.GetSelfMountInfo(ctx)
	assert.Nil(t, result, "got mount point info")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestWindowsClient_GetHostMountInfo(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	mockFSClient := mock_filesystem.NewMockFilesystemClient(ctrl)
	mockSmbClient := mock_smb.NewMockSmbClient(ctrl)
	client := newOsSpecificClientDetailed(mockFSClient, mockSmbClient)
	result, err := client.GetHostMountInfo(ctx)
	assert.Nil(t, result, "got mount point info")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestWindowsClient_IsCompatible(t *testing.T) {
	type parameters struct {
		protocol    string
		assertError assert.ErrorAssertionFunc
	}

	tests := map[string]parameters{
		"any protocol other than SMB": {
			protocol:    sa.NFS,
			assertError: assert.Error,
		},
		"SMB protocol": {
			protocol:    sa.SMB,
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			client := newOsSpecificClientDetailed(nil, nil)

			err := client.IsCompatible(context.Background(), params.protocol)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestWindowsClient_PVMountpointMappings(t *testing.T) {
	client := newOsSpecificClientDetailed(nil, nil)
	result, err := client.PVMountpointMappings(context.Background())
	assert.NotNil(t, result, "got mount point info")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestWindowsClient_ListProcMounts(t *testing.T) {
	client := newOsSpecificClientDetailed(nil, nil)
	result, err := client.ListProcMounts("c:\\mount\\point")
	assert.Nil(t, result, "got mount point info")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestWindowsClient_ListProcMountinfo(t *testing.T) {
	client := newOsSpecificClientDetailed(nil, nil)
	result, err := client.ListProcMountinfo()
	assert.Nil(t, result, "got mount point info")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestWindowsClient_makeDir(t *testing.T) {
	type parameters struct {
		getFileSystemClient func(controller *gomock.Controller) FilesystemClient
		assertError         assert.ErrorAssertionFunc
	}

	const mountPoint = "c:\\mount\\point"
	mkdirRequest := v1.MkdirRequest{Path: "c:\\mount\\point"}

	tests := map[string]parameters{
		"error making directory": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFileSystemClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFileSystemClient.EXPECT().Mkdir(gomock.Any(), &mkdirRequest).Return(nil, assert.AnError)
				return mockFileSystemClient
			},
			assertError: assert.Error,
		},
		"happy path": {
			getFileSystemClient: func(controller *gomock.Controller) FilesystemClient {
				mockFileSystemClient := mock_filesystem.NewMockFilesystemClient(controller)
				mockFileSystemClient.EXPECT().Mkdir(gomock.Any(), &mkdirRequest).Return(nil, nil)
				return mockFileSystemClient
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := newOsSpecificClientDetailed(params.getFileSystemClient(ctrl), nil)

			err := client.makeDir(context.Background(), mountPoint)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}
