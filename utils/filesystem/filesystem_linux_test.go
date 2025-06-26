// Copyright 2022 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/mocks/mock_utils/mock_filesystem"
	"github.com/netapp/trident/utils/errors"
	execCmd "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
)

func TestGetUnmountPath(t *testing.T) {
	ctx := context.Background()
	fsClient := New(nil)

	result, err := fsClient.GetUnmountPath(ctx, &models.VolumeTrackingInfo{})
	assert.Equal(t, result, "", "got unmount path")
	assert.Error(t, err, "no error")
	assert.True(t, errors.IsUnsupportedError(err), "not UnsupportedError")
}

func TestExpandFilesystemOnNode(t *testing.T) {
	mockVolumeInfo := &models.VolumePublishInfo{
		DevicePath: "/mock/path",
	}

	type parameters struct {
		publishInfo      *models.VolumePublishInfo
		sharedTargetPath string
		fsType           string
		mountOptions     string
		err              error
		getMountClient   func() Mount
		getCommandClient func() execCmd.Command
	}

	tests := map[string]parameters{
		"Happy path xfs": {
			publishInfo:      mockVolumeInfo,
			sharedTargetPath: "",
			fsType:           "xfs",
			mountOptions:     "",
			err:              nil,
			getMountClient: func() Mount {
				mockMountClient := mock_filesystem.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountFilesystemForResize(gomock.Any(), mockVolumeInfo.DevicePath,
					"", "").Return("./", nil)
				mockMountClient.EXPECT().RemoveMountPoint(gomock.Any(), "./").Return(nil)
				return mockMountClient
			},
			getCommandClient: func() execCmd.Command {
				mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().Execute(context.Background(), "xfs_growfs", "./").Return(nil, nil)
				return mockCommand
			},
		},
		"Happy path ext3": {
			publishInfo:      mockVolumeInfo,
			sharedTargetPath: "",
			fsType:           "ext3",
			mountOptions:     "",
			err:              nil,
			getMountClient: func() Mount {
				mockMountClient := mock_filesystem.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountFilesystemForResize(gomock.Any(), mockVolumeInfo.DevicePath,
					"", "").Return("./", nil)
				mockMountClient.EXPECT().RemoveMountPoint(gomock.Any(), "./").Return(nil)
				return mockMountClient
			},
			getCommandClient: func() execCmd.Command {
				mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().Execute(context.Background(), "resize2fs", mockVolumeInfo.DevicePath).Return(nil, nil)
				return mockCommand
			},
		},
		"Unsupported filesystem type": {
			publishInfo:      mockVolumeInfo,
			sharedTargetPath: "",
			fsType:           "fake-fs",
			mountOptions:     "",
			err:              errors.New(""),
			getMountClient: func() Mount {
				mockMountClient := mock_filesystem.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountFilesystemForResize(gomock.Any(), mockVolumeInfo.DevicePath,
					"", "").Return("/mount/point", nil)
				mockMountClient.EXPECT().RemoveMountPoint(gomock.Any(), "/mount/point").Return(nil)
				return mockMountClient
			},
		},
		"Mount Error": {
			publishInfo:      mockVolumeInfo,
			sharedTargetPath: "",
			fsType:           "",
			mountOptions:     "",
			err:              errors.New(""),
			getMountClient: func() Mount {
				mockMountClient := mock_filesystem.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountFilesystemForResize(gomock.Any(), mockVolumeInfo.DevicePath,
					"", "").Return("", errors.New("mount error"))
				return mockMountClient
			},
		},
		"Expand failed": {
			publishInfo:      mockVolumeInfo,
			sharedTargetPath: "",
			fsType:           "ext3",
			mountOptions:     "",
			err:              errors.New(""),
			getMountClient: func() Mount {
				mockMountClient := mock_filesystem.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountFilesystemForResize(gomock.Any(), mockVolumeInfo.DevicePath,
					"", "").Return("./", nil)
				mockMountClient.EXPECT().RemoveMountPoint(gomock.Any(), "./").Return(nil)
				return mockMountClient
			},
			getCommandClient: func() execCmd.Command {
				mockCommand := mock_exec.NewMockCommand(gomock.NewController(t))
				mockCommand.EXPECT().Execute(context.Background(), "resize2fs",
					mockVolumeInfo.DevicePath).Return(nil, errors.New("resize error"))
				return mockCommand
			},
		},
		"Stat FS error": {
			publishInfo:      mockVolumeInfo,
			sharedTargetPath: "",
			fsType:           "ext3",
			mountOptions:     "",
			err:              errors.New(""),
			getMountClient: func() Mount {
				mockMountClient := mock_filesystem.NewMockMount(gomock.NewController(t))
				mockMountClient.EXPECT().MountFilesystemForResize(gomock.Any(), mockVolumeInfo.DevicePath,
					"", "").Return("", nil)
				mockMountClient.EXPECT().RemoveMountPoint(gomock.Any(), "").Return(nil)
				return mockMountClient
			},
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			fsClient := NewDetailed(nil, nil, nil)

			if params.getMountClient != nil {
				fsClient.mountClient = params.getMountClient()
			}

			if params.getCommandClient != nil {
				fsClient.command = params.getCommandClient()
			}

			_, err := fsClient.ExpandFilesystemOnNode(context.Background(), params.publishInfo,
				params.publishInfo.DevicePath, params.sharedTargetPath, params.fsType, params.mountOptions)
			if params.err != nil {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
