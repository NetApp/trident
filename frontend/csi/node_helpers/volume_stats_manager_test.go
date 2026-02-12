// Copyright 2025 NetApp, Inc. All Rights Reserved.

package nodehelpers

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mock_blockdevice "github.com/netapp/trident/mocks/mock_utils/mock_blockdevice"
	mock_filesystem "github.com/netapp/trident/mocks/mock_utils/mock_filesystem"
	mock_osutils "github.com/netapp/trident/mocks/mock_utils/mock_osutils"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
)

// Mock VolumePublishManager for testing
type mockVolumePublishManager struct {
	readTrackingInfoFunc func(ctx context.Context, volumeID string) (*models.VolumeTrackingInfo, error)
}

func (m *mockVolumePublishManager) ReadTrackingInfo(ctx context.Context, volumeID string) (*models.VolumeTrackingInfo, error) {
	if m.readTrackingInfoFunc != nil {
		return m.readTrackingInfoFunc(ctx, volumeID)
	}
	return nil, errors.New("not implemented")
}

func (m *mockVolumePublishManager) WriteTrackingInfo(ctx context.Context, volumeID string, trackingInfo *models.VolumeTrackingInfo) error {
	return nil
}

func (m *mockVolumePublishManager) DeleteTrackingInfo(ctx context.Context, volumeID string) error {
	return nil
}

func (m *mockVolumePublishManager) ListVolumeTrackingInfo(ctx context.Context) (map[string]*models.VolumeTrackingInfo, error) {
	return nil, nil
}

func (m *mockVolumePublishManager) UpgradeVolumeTrackingFile(ctx context.Context, volumeID string, publishedPaths map[string]struct{}, pvToDeviceMappings map[string]string) (bool, error) {
	return false, nil
}

func (m *mockVolumePublishManager) DeleteFailedUpgradeTrackingFile(ctx context.Context, fileInfo os.FileInfo) {
}

func (m *mockVolumePublishManager) ValidateTrackingFile(ctx context.Context, volumeID string) (bool, error) {
	return false, nil
}

func (m *mockVolumePublishManager) GetVolumeTrackingFiles() ([]os.FileInfo, error) {
	return nil, nil
}

func TestGetVolumePathFromTrackingInfo(t *testing.T) {
	vsm := &volumeStatsManager{}

	tests := []struct {
		name         string
		trackingInfo *models.VolumeTrackingInfo
		expected     string
	}{
		{
			name:         "Nil tracking info",
			trackingInfo: nil,
			expected:     "",
		},
		{
			name: "Has published path",
			trackingInfo: &models.VolumeTrackingInfo{
				PublishedPaths: map[string]struct{}{
					"/var/lib/kubelet/pods/test/volumes/kubernetes.io~csi/pvc-123/mount": {},
				},
				StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-123/globalmount",
			},
			expected: "/var/lib/kubelet/pods/test/volumes/kubernetes.io~csi/pvc-123/mount",
		},
		{
			name: "No published paths - fallback to staging target path",
			trackingInfo: &models.VolumeTrackingInfo{
				PublishedPaths:    map[string]struct{}{},
				StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-123/globalmount",
			},
			expected: "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-123/globalmount",
		},
		{
			name: "Nil published paths - fallback to staging target path",
			trackingInfo: &models.VolumeTrackingInfo{
				PublishedPaths:    nil,
				StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-456/globalmount",
			},
			expected: "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-456/globalmount",
		},
		{
			name: "Empty published paths and empty staging path",
			trackingInfo: &models.VolumeTrackingInfo{
				PublishedPaths:    map[string]struct{}{},
				StagingTargetPath: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := vsm.getVolumePathFromTrackingInfo(tt.trackingInfo)
			if tt.name == "Has published path" && len(tt.trackingInfo.PublishedPaths) > 1 {
				// For multiple paths, just verify we got one of them
				assert.NotEmpty(t, result)
				_, exists := tt.trackingInfo.PublishedPaths[result]
				assert.True(t, exists, "Result should be one of the published paths")
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestGetBlockDeviceStatsByID(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name             string
		volumeID         string
		trackingInfo     *models.VolumeTrackingInfo
		setupBlockDevice func(*gomock.Controller) *mock_blockdevice.MockBlockDevice
		expectError      bool
		expectedStats    *VolumeStats
		errorContains    string
	}{
		{
			name:     "Successful block device stats retrieval - iSCSI",
			volumeID: "vol-iscsi-123",
			trackingInfo: &models.VolumeTrackingInfo{
				VolumePublishInfo: models.VolumePublishInfo{
					FilesystemType: filesystem.Raw,
					DevicePath:     "/dev/sdb",
					SANType:        "iSCSI",
				},
			},
			setupBlockDevice: func(ctrl *gomock.Controller) *mock_blockdevice.MockBlockDevice {
				mockBD := mock_blockdevice.NewMockBlockDevice(ctrl)
				mockBD.EXPECT().GetBlockDeviceStats(gomock.Any(), "/dev/sdb", "iscsi").
					Return(int64(536870912), int64(536870912), int64(1073741824), nil)
				return mockBD
			},
			expectError: false,
			expectedStats: &VolumeStats{
				Total:          1073741824,
				Used:           536870912,
				Available:      536870912,
				UsedPercentage: 50.0,
				Inodes:         0,
				InodesUsed:     0,
				InodesFree:     0,
			},
		},
		{
			name:     "Successful block device stats retrieval - NVMe",
			volumeID: "vol-nvme-456",
			trackingInfo: &models.VolumeTrackingInfo{
				VolumePublishInfo: models.VolumePublishInfo{
					FilesystemType: filesystem.Raw,
					DevicePath:     "/dev/nvme0n1",
					SANType:        "NVMe",
				},
			},
			setupBlockDevice: func(ctrl *gomock.Controller) *mock_blockdevice.MockBlockDevice {
				mockBD := mock_blockdevice.NewMockBlockDevice(ctrl)
				mockBD.EXPECT().GetBlockDeviceStats(gomock.Any(), "/dev/nvme0n1", "nvme").
					Return(int64(214748364), int64(1932735283), int64(2147483648), nil)
				return mockBD
			},
			expectError: false,
			expectedStats: &VolumeStats{
				Total:          2147483648,
				Used:           214748364,
				Available:      1932735283,
				UsedPercentage: 10.000003576278687, // used/capacity = 214748364/2147483648 * 100
				Inodes:         0,
				InodesUsed:     0,
				InodesFree:     0,
			},
		},
		{
			name:     "Successful block device stats - RawDevicePath fallback",
			volumeID: "vol-raw-789",
			trackingInfo: &models.VolumeTrackingInfo{
				VolumePublishInfo: models.VolumePublishInfo{
					FilesystemType: filesystem.Raw,
					DevicePath:     "", // Empty
					RawDevicePath:  "/dev/sdc",
					SANType:        "FCP",
				},
			},
			setupBlockDevice: func(ctrl *gomock.Controller) *mock_blockdevice.MockBlockDevice {
				mockBD := mock_blockdevice.NewMockBlockDevice(ctrl)
				mockBD.EXPECT().GetBlockDeviceStats(gomock.Any(), "/dev/sdc", "fcp").
					Return(int64(3865470566), int64(429496729), int64(4294967296), nil)
				return mockBD
			},
			expectError: false,
			expectedStats: &VolumeStats{
				Total:          4294967296,
				Used:           3865470566,
				Available:      429496729,
				UsedPercentage: 89.99999987520278, // used/capacity = 3865470566/4294967296 * 100
				Inodes:         0,
				InodesUsed:     0,
				InodesFree:     0,
			},
		},
		{
			name:     "Error - no device path",
			volumeID: "vol-no-path",
			trackingInfo: &models.VolumeTrackingInfo{
				VolumePublishInfo: models.VolumePublishInfo{
					FilesystemType: filesystem.Raw,
					DevicePath:     "",
					RawDevicePath:  "",
					SANType:        "iSCSI",
				},
			},
			setupBlockDevice: func(ctrl *gomock.Controller) *mock_blockdevice.MockBlockDevice {
				return mock_blockdevice.NewMockBlockDevice(ctrl)
			},
			expectError:   true,
			errorContains: "unable to determine device path",
		},
		{
			name:     "Error - GetBlockDeviceStats fails",
			volumeID: "vol-stats-error",
			trackingInfo: &models.VolumeTrackingInfo{
				VolumePublishInfo: models.VolumePublishInfo{
					FilesystemType: filesystem.Raw,
					DevicePath:     "/dev/sdd",
					SANType:        "iSCSI",
				},
			},
			setupBlockDevice: func(ctrl *gomock.Controller) *mock_blockdevice.MockBlockDevice {
				mockBD := mock_blockdevice.NewMockBlockDevice(ctrl)
				mockBD.EXPECT().GetBlockDeviceStats(gomock.Any(), "/dev/sdd", "iscsi").
					Return(int64(0), int64(0), int64(0), errors.New("device not found"))
				return mockBD
			},
			expectError:   true,
			errorContains: "failed to get block device volume statistics",
		},
		{
			name:     "Zero available - 100% used",
			volumeID: "vol-full",
			trackingInfo: &models.VolumeTrackingInfo{
				VolumePublishInfo: models.VolumePublishInfo{
					FilesystemType: filesystem.Raw,
					DevicePath:     "/dev/sde",
					SANType:        "iSCSI",
				},
			},
			setupBlockDevice: func(ctrl *gomock.Controller) *mock_blockdevice.MockBlockDevice {
				mockBD := mock_blockdevice.NewMockBlockDevice(ctrl)
				mockBD.EXPECT().GetBlockDeviceStats(gomock.Any(), "/dev/sde", "iscsi").
					Return(int64(1073741824), int64(0), int64(1073741824), nil)
				return mockBD
			},
			expectError: false,
			expectedStats: &VolumeStats{
				Total:          1073741824,
				Used:           1073741824,
				Available:      0,
				UsedPercentage: 100.0,
				Inodes:         0,
				InodesUsed:     0,
				InodesFree:     0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockBD := tt.setupBlockDevice(ctrl)
			vsm := &volumeStatsManager{
				bd: mockBD,
			}

			stats, err := vsm.GetBlockDeviceStatsByID(ctx, tt.volumeID, tt.trackingInfo)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, stats)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, stats)
				assert.Equal(t, tt.expectedStats.Total, stats.Total)
				assert.Equal(t, tt.expectedStats.Used, stats.Used)
				assert.Equal(t, tt.expectedStats.Available, stats.Available)
				assert.InDelta(t, tt.expectedStats.UsedPercentage, stats.UsedPercentage, 0.1)
				assert.Equal(t, tt.expectedStats.Inodes, stats.Inodes)
				assert.Equal(t, tt.expectedStats.InodesUsed, stats.InodesUsed)
				assert.Equal(t, tt.expectedStats.InodesFree, stats.InodesFree)
			}
		})
	}
}

func TestGetFilesystemStatsByID(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name            string
		volumePath      string
		setupFilesystem func(*gomock.Controller) *mock_filesystem.MockFilesystem
		expectError     bool
		expectedStats   *VolumeStats
		errorContains   string
	}{
		{
			name:       "Successful filesystem stats retrieval - 50% used",
			volumePath: "/var/lib/kubelet/pods/test/volumes/kubernetes.io~csi/pvc-123/mount",
			setupFilesystem: func(ctrl *gomock.Controller) *mock_filesystem.MockFilesystem {
				mockFS := mock_filesystem.NewMockFilesystem(ctrl)
				mockFS.EXPECT().GetFilesystemStats(gomock.Any(), "/var/lib/kubelet/pods/test/volumes/kubernetes.io~csi/pvc-123/mount").
					Return(int64(5368709120), int64(10737418240), int64(5368709120), int64(655360), int64(327680), int64(327680), nil)
				return mockFS
			},
			expectError: false,
			expectedStats: &VolumeStats{
				Total:          10737418240,
				Used:           5368709120,
				Available:      5368709120,
				UsedPercentage: 50.0,
				Inodes:         655360,
				InodesUsed:     327680,
				InodesFree:     327680,
			},
		},
		{
			name:       "Successful filesystem stats retrieval - 10% used",
			volumePath: "/mnt/test",
			setupFilesystem: func(ctrl *gomock.Controller) *mock_filesystem.MockFilesystem {
				mockFS := mock_filesystem.NewMockFilesystem(ctrl)
				mockFS.EXPECT().GetFilesystemStats(gomock.Any(), "/mnt/test").
					Return(int64(19327352832), int64(21474836480), int64(2147483648), int64(1310720), int64(1179648), int64(131072), nil)
				return mockFS
			},
			expectError: false,
			expectedStats: &VolumeStats{
				Total:          21474836480,
				Used:           2147483648,
				Available:      19327352832,
				UsedPercentage: 10.0,
				Inodes:         1310720,
				InodesUsed:     131072,
				InodesFree:     1179648,
			},
		},
		{
			name:       "Successful filesystem stats retrieval - 90% used",
			volumePath: "/data",
			setupFilesystem: func(ctrl *gomock.Controller) *mock_filesystem.MockFilesystem {
				mockFS := mock_filesystem.NewMockFilesystem(ctrl)
				mockFS.EXPECT().GetFilesystemStats(gomock.Any(), "/data").
					Return(int64(4294967296), int64(42949672960), int64(38654705664), int64(2621440), int64(262144), int64(2359296), nil)
				return mockFS
			},
			expectError: false,
			expectedStats: &VolumeStats{
				Total:          42949672960,
				Used:           38654705664,
				Available:      4294967296,
				UsedPercentage: 90.0,
				Inodes:         2621440,
				InodesUsed:     2359296,
				InodesFree:     262144,
			},
		},
		{
			name:       "Empty filesystem - 0% used",
			volumePath: "/empty",
			setupFilesystem: func(ctrl *gomock.Controller) *mock_filesystem.MockFilesystem {
				mockFS := mock_filesystem.NewMockFilesystem(ctrl)
				mockFS.EXPECT().GetFilesystemStats(gomock.Any(), "/empty").
					Return(int64(10737418240), int64(10737418240), int64(0), int64(655360), int64(655360), int64(0), nil)
				return mockFS
			},
			expectError: false,
			expectedStats: &VolumeStats{
				Total:          10737418240,
				Used:           0,
				Available:      10737418240,
				UsedPercentage: 0.0,
				Inodes:         655360,
				InodesUsed:     0,
				InodesFree:     655360,
			},
		},
		{
			name:       "Error - GetFilesystemStats fails",
			volumePath: "/error-path",
			setupFilesystem: func(ctrl *gomock.Controller) *mock_filesystem.MockFilesystem {
				mockFS := mock_filesystem.NewMockFilesystem(ctrl)
				mockFS.EXPECT().GetFilesystemStats(gomock.Any(), "/error-path").
					Return(int64(0), int64(0), int64(0), int64(0), int64(0), int64(0), errors.New("filesystem unavailable"))
				return mockFS
			},
			expectError:   true,
			errorContains: "failed to get filesystem stats",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockFS := tt.setupFilesystem(ctrl)
			vsm := &volumeStatsManager{
				fs: mockFS,
			}

			stats, err := vsm.GetFilesystemStatsByID(ctx, tt.volumePath)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, stats)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, stats)
				assert.Equal(t, tt.expectedStats.Total, stats.Total)
				assert.Equal(t, tt.expectedStats.Used, stats.Used)
				assert.Equal(t, tt.expectedStats.Available, stats.Available)
				assert.InDelta(t, tt.expectedStats.UsedPercentage, stats.UsedPercentage, 0.1)
				assert.Equal(t, tt.expectedStats.Inodes, stats.Inodes)
				assert.Equal(t, tt.expectedStats.InodesUsed, stats.InodesUsed)
				assert.Equal(t, tt.expectedStats.InodesFree, stats.InodesFree)
			}
		})
	}
}

func TestGetVolumeStatsByID(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name          string
		volumeID      string
		setupMocks    func(*gomock.Controller) (*mockVolumePublishManager, *mock_blockdevice.MockBlockDevice, *mock_filesystem.MockFilesystem)
		expectError   bool
		expectedStats *VolumeStats
		errorContains string
	}{
		{
			name:     "Raw block volume - successful stats",
			volumeID: "vol-raw-block",
			setupMocks: func(ctrl *gomock.Controller) (*mockVolumePublishManager, *mock_blockdevice.MockBlockDevice, *mock_filesystem.MockFilesystem) {
				mockPublishMgr := &mockVolumePublishManager{
					readTrackingInfoFunc: func(ctx context.Context, volumeID string) (*models.VolumeTrackingInfo, error) {
						return &models.VolumeTrackingInfo{
							VolumePublishInfo: models.VolumePublishInfo{
								FilesystemType: filesystem.Raw,
								DevicePath:     "/dev/sdb",
								SANType:        "iSCSI",
							},
						}, nil
					},
				}
				mockBD := mock_blockdevice.NewMockBlockDevice(ctrl)
				mockFS := mock_filesystem.NewMockFilesystem(ctrl)

				mockBD.EXPECT().GetBlockDeviceStats(gomock.Any(), "/dev/sdb", "iscsi").
					Return(int64(536870912), int64(536870912), int64(1073741824), nil)

				return mockPublishMgr, mockBD, mockFS
			},
			expectError: false,
			expectedStats: &VolumeStats{
				Total:          1073741824,
				Used:           536870912,
				Available:      536870912,
				UsedPercentage: 50.0,
				Inodes:         0,
				InodesUsed:     0,
				InodesFree:     0,
			},
		},
		{
			name:     "Filesystem volume - successful stats",
			volumeID: "vol-filesystem",
			setupMocks: func(ctrl *gomock.Controller) (*mockVolumePublishManager, *mock_blockdevice.MockBlockDevice, *mock_filesystem.MockFilesystem) {
				mockPublishMgr := &mockVolumePublishManager{
					readTrackingInfoFunc: func(ctx context.Context, volumeID string) (*models.VolumeTrackingInfo, error) {
						return &models.VolumeTrackingInfo{
							VolumePublishInfo: models.VolumePublishInfo{
								FilesystemType: "ext4",
							},
							PublishedPaths: map[string]struct{}{
								"/var/lib/kubelet/pods/test/volumes/pvc-123/mount": {},
							},
						}, nil
					},
				}
				mockBD := mock_blockdevice.NewMockBlockDevice(ctrl)
				mockFS := mock_filesystem.NewMockFilesystem(ctrl)

				mockFS.EXPECT().GetFilesystemStats(gomock.Any(), "/var/lib/kubelet/pods/test/volumes/pvc-123/mount").
					Return(int64(5368709120), int64(10737418240), int64(5368709120), int64(655360), int64(327680), int64(327680), nil)

				return mockPublishMgr, mockBD, mockFS
			},
			expectError: false,
			expectedStats: &VolumeStats{
				Total:          10737418240,
				Used:           5368709120,
				Available:      5368709120,
				UsedPercentage: 50.0,
				Inodes:         655360,
				InodesUsed:     327680,
				InodesFree:     327680,
			},
		},
		{
			name:     "Error - tracking info not found",
			volumeID: "vol-not-found",
			setupMocks: func(ctrl *gomock.Controller) (*mockVolumePublishManager, *mock_blockdevice.MockBlockDevice, *mock_filesystem.MockFilesystem) {
				mockPublishMgr := &mockVolumePublishManager{
					readTrackingInfoFunc: func(ctx context.Context, volumeID string) (*models.VolumeTrackingInfo, error) {
						return nil, errors.New("tracking file not found")
					},
				}
				mockBD := mock_blockdevice.NewMockBlockDevice(ctrl)
				mockFS := mock_filesystem.NewMockFilesystem(ctrl)

				return mockPublishMgr, mockBD, mockFS
			},
			expectError:   true,
			errorContains: "error reading tracking file",
		},
		{
			name:     "Error - filesystem volume with no path",
			volumeID: "vol-no-path",
			setupMocks: func(ctrl *gomock.Controller) (*mockVolumePublishManager, *mock_blockdevice.MockBlockDevice, *mock_filesystem.MockFilesystem) {
				mockPublishMgr := &mockVolumePublishManager{
					readTrackingInfoFunc: func(ctx context.Context, volumeID string) (*models.VolumeTrackingInfo, error) {
						return &models.VolumeTrackingInfo{
							VolumePublishInfo: models.VolumePublishInfo{
								FilesystemType: "xfs",
							},
							PublishedPaths:    map[string]struct{}{},
							StagingTargetPath: "",
						}, nil
					},
				}
				mockBD := mock_blockdevice.NewMockBlockDevice(ctrl)
				mockFS := mock_filesystem.NewMockFilesystem(ctrl)

				return mockPublishMgr, mockBD, mockFS
			},
			expectError:   true,
			errorContains: "unable to determine volume path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockPublishMgr, mockBD, mockFS := tt.setupMocks(ctrl)
			vsm := &volumeStatsManager{
				volumePublishManager: mockPublishMgr,
				bd:                   mockBD,
				fs:                   mockFS,
			}

			stats, err := vsm.GetVolumeStatsByID(ctx, tt.volumeID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, stats)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, stats)
				assert.Equal(t, tt.expectedStats.Total, stats.Total)
				assert.Equal(t, tt.expectedStats.Used, stats.Used)
				assert.Equal(t, tt.expectedStats.Available, stats.Available)
				assert.InDelta(t, tt.expectedStats.UsedPercentage, stats.UsedPercentage, 0.1)
				assert.Equal(t, tt.expectedStats.Inodes, stats.Inodes)
				assert.Equal(t, tt.expectedStats.InodesUsed, stats.InodesUsed)
				assert.Equal(t, tt.expectedStats.InodesFree, stats.InodesFree)
			}
		})
	}
}

func TestVerifyVolumePath(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		volumePath  string
		setupOSUtil func(*gomock.Controller) *mock_osutils.MockUtils
		expectError bool
	}{
		{
			name:       "Volume path exists",
			volumePath: "/var/lib/kubelet/pods/test/volumes/pvc-123/mount",
			setupOSUtil: func(ctrl *gomock.Controller) *mock_osutils.MockUtils {
				mockOSUtil := mock_osutils.NewMockUtils(ctrl)
				mockOSUtil.EXPECT().PathExistsWithTimeout(gomock.Any(), "/var/lib/kubelet/pods/test/volumes/pvc-123/mount", fsUnavailableTimeout).
					Return(true, nil)
				return mockOSUtil
			},
			expectError: false,
		},
		{
			name:       "Volume path does not exist",
			volumePath: "/nonexistent/path",
			setupOSUtil: func(ctrl *gomock.Controller) *mock_osutils.MockUtils {
				mockOSUtil := mock_osutils.NewMockUtils(ctrl)
				mockOSUtil.EXPECT().PathExistsWithTimeout(gomock.Any(), "/nonexistent/path", fsUnavailableTimeout).
					Return(false, nil)
				return mockOSUtil
			},
			expectError: true,
		},
		{
			name:       "Error checking path",
			volumePath: "/error/path",
			setupOSUtil: func(ctrl *gomock.Controller) *mock_osutils.MockUtils {
				mockOSUtil := mock_osutils.NewMockUtils(ctrl)
				mockOSUtil.EXPECT().PathExistsWithTimeout(gomock.Any(), "/error/path", fsUnavailableTimeout).
					Return(false, errors.New("permission denied"))
				return mockOSUtil
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockOSUtil := tt.setupOSUtil(ctrl)
			vsm := &volumeStatsManager{
				osutils: mockOSUtil,
			}

			err := vsm.VerifyVolumePath(ctx, tt.volumePath)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsRawBlockVolume(t *testing.T) {
	vsm := &volumeStatsManager{}

	tests := []struct {
		name         string
		trackingInfo *models.VolumeTrackingInfo
		expected     bool
	}{
		{
			name:         "Nil tracking info",
			trackingInfo: nil,
			expected:     false,
		},
		{
			name: "Raw block volume",
			trackingInfo: &models.VolumeTrackingInfo{
				VolumePublishInfo: models.VolumePublishInfo{
					FilesystemType: filesystem.Raw,
					DevicePath:     "/dev/sdb",
					SANType:        "iSCSI",
				},
			},
			expected: true,
		},
		{
			name: "Filesystem volume - ext4",
			trackingInfo: &models.VolumeTrackingInfo{
				VolumePublishInfo: models.VolumePublishInfo{
					FilesystemType: "ext4",
					DevicePath:     "/dev/sdb",
					SANType:        "iSCSI",
				},
			},
			expected: false,
		},
		{
			name: "Filesystem volume - xfs",
			trackingInfo: &models.VolumeTrackingInfo{
				VolumePublishInfo: models.VolumePublishInfo{
					FilesystemType: "xfs",
				},
			},
			expected: false,
		},
		{
			name: "Filesystem volume - nfs",
			trackingInfo: &models.VolumeTrackingInfo{
				VolumePublishInfo: models.VolumePublishInfo{
					FilesystemType: "nfs",
				},
			},
			expected: false,
		},
		{
			name: "Empty filesystem type",
			trackingInfo: &models.VolumeTrackingInfo{
				VolumePublishInfo: models.VolumePublishInfo{
					FilesystemType: "",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := vsm.IsRawBlockVolume(tt.trackingInfo)
			assert.Equal(t, tt.expected, result)
		})
	}
}
