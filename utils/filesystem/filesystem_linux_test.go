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

// makeConstantSizeFn returns a size function that always returns the given value.
func makeConstantSizeFn(size int64) func(context.Context, string) (int64, error) {
	return func(_ context.Context, _ string) (int64, error) {
		return size, nil
	}
}

// makeErrorSizeFn returns a size function that always returns an error.
func makeErrorSizeFn(err error) func(context.Context, string) (int64, error) {
	return func(_ context.Context, _ string) (int64, error) {
		return 0, err
	}
}

// TestExpandFilesystem exercises expandFilesystem directly.
func TestExpandFilesystem(t *testing.T) {
	const (
		postExpandSize = int64(2 * 1024 * 1024 * 1024) // 2 GiB
	)

	type parameters struct {
		cmdName           string
		cmdArguments      string
		tmpMountPoint     string
		getFilesystemSize func(context.Context, string) (int64, error)
		getCommandClient  func() execCmd.Command
		expectError       bool
	}

	tests := map[string]parameters{
		"Happy path xfs": {
			cmdName:           "xfs_growfs",
			cmdArguments:      "./",
			tmpMountPoint:     "./",
			getFilesystemSize: makeConstantSizeFn(postExpandSize),
			getCommandClient: func() execCmd.Command {
				m := mock_exec.NewMockCommand(gomock.NewController(t))
				m.EXPECT().Execute(context.Background(), "xfs_growfs", "./").Return(nil, nil)
				return m
			},
		},
		"Happy path ext3": {
			cmdName:           "resize2fs",
			cmdArguments:      "/mock/path",
			tmpMountPoint:     "./",
			getFilesystemSize: makeConstantSizeFn(postExpandSize),
			getCommandClient: func() execCmd.Command {
				m := mock_exec.NewMockCommand(gomock.NewController(t))
				m.EXPECT().Execute(context.Background(), "resize2fs", "/mock/path").Return(nil, nil)
				return m
			},
		},
		"Expand command failed": {
			cmdName:           "resize2fs",
			cmdArguments:      "/mock/path",
			tmpMountPoint:     "./",
			getFilesystemSize: makeConstantSizeFn(0),
			getCommandClient: func() execCmd.Command {
				m := mock_exec.NewMockCommand(gomock.NewController(t))
				m.EXPECT().Execute(context.Background(), "resize2fs", "/mock/path").Return(nil, errors.New("resize error"))
				return m
			},
			expectError: true,
		},
		"Post-expand stat error": {
			cmdName:           "resize2fs",
			cmdArguments:      "/mock/path",
			tmpMountPoint:     "./",
			getFilesystemSize: makeErrorSizeFn(errors.New("statfs error")),
			getCommandClient: func() execCmd.Command {
				m := mock_exec.NewMockCommand(gomock.NewController(t))
				m.EXPECT().Execute(context.Background(), "resize2fs", "/mock/path").Return(nil, nil)
				return m
			},
			expectError: true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			size, err := expandFilesystem(
				context.Background(), params.getCommandClient(),
				params.cmdName, params.cmdArguments, params.tmpMountPoint,
				params.getFilesystemSize,
			)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, postExpandSize, size)
			}
		})
	}
}

// TestExpandFilesystemOnNode tests the mount orchestration and fsType dispatch in
// ExpandFilesystemOnNode. Expand logic is covered by TestExpandFilesystem above.
func TestExpandFilesystemOnNode(t *testing.T) {
	mockVolumeInfo := &models.VolumePublishInfo{
		DevicePath: "/mock/path",
	}

	type parameters struct {
		publishInfo      *models.VolumePublishInfo
		sharedTargetPath string
		fsType           string
		mountOptions     string
		getMountClient   func() Mount
		getCommandClient func() execCmd.Command
		expectError      bool
	}

	tests := map[string]parameters{
		"Mount error": {
			publishInfo:      mockVolumeInfo,
			sharedTargetPath: "",
			fsType:           "ext4",
			getMountClient: func() Mount {
				m := mock_filesystem.NewMockMount(gomock.NewController(t))
				m.EXPECT().MountFilesystemForResize(gomock.Any(), mockVolumeInfo.DevicePath, "", "").
					Return("", errors.New("mount error"))
				return m
			},
			expectError: true,
		},
		"Unsupported filesystem type": {
			publishInfo:      mockVolumeInfo,
			sharedTargetPath: "",
			fsType:           "fake-fs",
			getMountClient: func() Mount {
				m := mock_filesystem.NewMockMount(gomock.NewController(t))
				m.EXPECT().MountFilesystemForResize(gomock.Any(), mockVolumeInfo.DevicePath, "", "").
					Return("/mount/point", nil)
				m.EXPECT().RemoveMountPoint(gomock.Any(), "/mount/point").Return(nil)
				return m
			},
			expectError: true,
		},
		"Stat FS error on empty mount point (real syscall on empty path)": {
			publishInfo:      mockVolumeInfo,
			sharedTargetPath: "",
			fsType:           "ext3",
			getMountClient: func() Mount {
				m := mock_filesystem.NewMockMount(gomock.NewController(t))
				m.EXPECT().MountFilesystemForResize(gomock.Any(), mockVolumeInfo.DevicePath, "", "").
					Return("", nil)
				m.EXPECT().RemoveMountPoint(gomock.Any(), "").Return(nil)
				return m
			},
			getCommandClient: func() execCmd.Command {
				m := mock_exec.NewMockCommand(gomock.NewController(t))
				m.EXPECT().Execute(gomock.Any(), "resize2fs", mockVolumeInfo.DevicePath).Return(nil, nil)
				return m
			},
			expectError: true,
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

			_, err := fsClient.ExpandFilesystemOnNode(
				context.Background(), params.publishInfo,
				params.publishInfo.DevicePath, params.sharedTargetPath,
				params.fsType, params.mountOptions,
				0,
			)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
