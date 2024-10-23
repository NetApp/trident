package mount

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_mount"
	"github.com/netapp/trident/utils/models"
)

func TestNewDetailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	mountClient := mock_mount.NewMockMount(ctrl)
	client := NewDetailed(mountClient)
	assert.NotNil(t, client)
}

func TestClient_MountFilesystemForResize(t *testing.T) {
	type parameters struct {
		getMountClient func(controller *gomock.Controller) mountOS
		assertError    assert.ErrorAssertionFunc
		expectedResult string
	}

	const devicePath = "/device/path"
	const stagedTargetPath = "/staged/target/path"
	const mountOptions = "mount,options"
	const tempMountPath = stagedTargetPath + "/tmp_mnt"

	tests := map[string]parameters{
		"MountDevice return error": {
			getMountClient: func(controller *gomock.Controller) mountOS {
				mountClient := mock_mount.NewMockMount(controller)
				mountClient.EXPECT().MountDevice(gomock.Any(), devicePath, tempMountPath, mountOptions,
					false).Return(assert.AnError)
				return mountClient
			},
			assertError:    assert.Error,
			expectedResult: "",
		},
		"happy path": {
			getMountClient: func(controller *gomock.Controller) mountOS {
				mountClient := mock_mount.NewMockMount(controller)
				mountClient.EXPECT().MountDevice(gomock.Any(), devicePath, tempMountPath, mountOptions,
					false).Return(nil)
				return mountClient
			},
			assertError:    assert.NoError,
			expectedResult: tempMountPath,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			client := NewDetailed(params.getMountClient(gomock.NewController(t)))

			result, err := client.MountFilesystemForResize(context.Background(), devicePath, stagedTargetPath,
				mountOptions)

			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedResult, result)
		})
	}
}

func TestClient_AttachNFSVolume(t *testing.T) {
	type parameters struct {
		getMountClient func(controller *gomock.Controller) mountOS
		assertError    assert.ErrorAssertionFunc
	}
	const volumeName = "name"
	const mountPoint = "/mountPoint"
	const mountOptions = "mount,options"
	const nfsServerIP = "127.0.0.1"
	const nfsPath = "/nfs/path"

	exportPath := fmt.Sprintf("%s:%s", nfsServerIP, nfsPath)

	tests := map[string]parameters{
		"mount NFS path return error": {
			getMountClient: func(controller *gomock.Controller) mountOS {
				mountClient := mock_mount.NewMockMount(controller)
				mountClient.EXPECT().MountNFSPath(gomock.Any(), exportPath, mountPoint, mountOptions).Return(assert.AnError)
				return mountClient
			},
			assertError: assert.Error,
		},
		"happy path": {
			getMountClient: func(controller *gomock.Controller) mountOS {
				mountClient := mock_mount.NewMockMount(controller)
				mountClient.EXPECT().MountNFSPath(gomock.Any(), exportPath, mountPoint, mountOptions).Return(nil)
				return mountClient
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			publishInfo := &models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					NfsAccessInfo: models.NfsAccessInfo{
						NfsServerIP: nfsServerIP,
						NfsPath:     nfsPath,
					},
					MountOptions: mountOptions,
				},
			}

			client := NewDetailed(params.getMountClient(gomock.NewController(t)))

			err := client.AttachNFSVolume(context.Background(), volumeName, mountPoint, publishInfo)

			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestClient_AttachSMBVolume(t *testing.T) {
	type parameters struct {
		getMountClient func(controller *gomock.Controller) mountOS
		assertError    assert.ErrorAssertionFunc
	}
	const volumeName = "name"
	const mountPoint = "c:\\mountPoint"
	const mountOptions = "mount,options"
	const smbServerIP = "127.0.0.1"
	const smbPath = "\\smb\\path"
	const userName = "user"
	const password = "password"

	exportPath := fmt.Sprintf("\\\\%s%s", smbServerIP, smbPath)

	tests := map[string]parameters{
		"mount SMB path return error": {
			getMountClient: func(controller *gomock.Controller) mountOS {
				mountClient := mock_mount.NewMockMount(controller)
				mountClient.EXPECT().MountSMBPath(gomock.Any(), exportPath, mountPoint, userName, password).
					Return(assert.AnError)
				return mountClient
			},
			assertError: assert.Error,
		},
		"happy path": {
			getMountClient: func(controller *gomock.Controller) mountOS {
				mountClient := mock_mount.NewMockMount(controller)
				mountClient.EXPECT().MountSMBPath(gomock.Any(), exportPath, mountPoint, userName, password).
					Return(nil)
				return mountClient
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			publishInfo := &models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					SMBAccessInfo: models.SMBAccessInfo{
						SMBServer: smbServerIP,
						SMBPath:   smbPath,
					},
					MountOptions: mountOptions,
				},
			}

			client := NewDetailed(params.getMountClient(gomock.NewController(t)))

			err := client.AttachSMBVolume(context.Background(), volumeName, mountPoint, userName, password, publishInfo)

			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}
