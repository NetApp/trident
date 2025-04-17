// Copyright 2024 NetApp, Inc. All Rights Reserved.

package iscsi

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_devices"
	"github.com/netapp/trident/mocks/mock_utils/mock_devices/mock_luks"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/mocks/mock_utils/mock_filesystem"
	"github.com/netapp/trident/mocks/mock_utils/mock_iscsi"
	"github.com/netapp/trident/mocks/mock_utils/mock_mount"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/devices/luks"
	tridentexec "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount"
	"github.com/netapp/trident/utils/osutils"
)

func TestClient_AttachVolume_LUKS(t *testing.T) {
	type parameters struct {
		chrootPathPrefix    string
		getCommand          func(controller *gomock.Controller) tridentexec.Command
		getOSClient         func(controller *gomock.Controller) OS
		getDeviceClient     func(controller *gomock.Controller) devices.Devices
		getLuksDevice       func(controller *gomock.Controller) luks.Device
		getFileSystemClient func(controller *gomock.Controller) filesystem.Filesystem
		getMountClient      func(controller *gomock.Controller) mount.Mount
		getReconcileUtils   func(controller *gomock.Controller) IscsiReconcileUtils
		getFileSystemUtils  func() afero.Fs

		publishInfo       models.VolumePublishInfo
		volumeName        string
		volumeMountPoint  string
		volumeAuthSecrets map[string]string

		assertError       assert.ErrorAssertionFunc
		expectedMpathSize int64
	}

	const targetIQN = "iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.3"
	const vpdpg80Serial = "SYA5GZFJ8G1M905GVH7H"

	const iscsiadmSessionOutput = `tcp: [3] 127.0.0.1:3260,1028 ` + targetIQN + ` (non-flash)
tcp: [4] 127.0.0.2:3260,1029 ` + targetIQN + ` (non-flash)`
	const iscsiadmSessionOutputOneSession = "tcp: [3] 127.0.0.1:3260,1028  " + targetIQN + " (non-flash)"

	const iscsiadmNodeOutput = `127.0.0.1:3260,1042 ` + targetIQN + `
127.0.0.1:3260,1043 ` + targetIQN + `
`
	const iscsiadmDiscoveryDBSendTargetsOutput = `127.0.0.1:3260,1042 ` + targetIQN + `
127.0.0.1:3260,1043 ` + targetIQN + `
`

	tests := map[string]parameters{
		"LUKS volume with file system raw": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeoutAndInput(context.TODO(), "cryptsetup", 30*time.Second, true, "",
					"status", "/dev/mapper/luks-test-volume")
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
					nil)
				return mockDevices
			},
			getLuksDevice: func(controller *gomock.Controller) luks.Device {
				luksDevice := mock_luks.NewMockDevice(controller)
				luksDevice.EXPECT().MappedDevicePath().Return("/dev/mapper/dm-0")
				luksDevice.EXPECT().EnsureDeviceMappedOnHost(context.TODO(), "test-volume",
					map[string]string{}).Return(true, nil)
				return luksDevice
			},
			getFileSystemClient: func(controller *gomock.Controller) filesystem.Filesystem {
				mockFileSystem := mock_filesystem.NewMockFilesystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) mount.Mount {
				mockMount := mock_mount.NewMockMount(controller)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				return mockReconcileUtils
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				f, err := fs.Create("/dev/sda/vpd_pg80")
				assert.NoError(t, err)

				_, err = f.Write(vpdpg80SerialBytes(vpdpg80Serial))
				assert.NoError(t, err)

				_, err = fs.Create("/dev/sda/rescan")
				assert.NoError(t, err)

				_, err = fs.Create("/dev/sda/delete")
				assert.NoError(t, err)

				err = fs.MkdirAll("/sys/block/sda/holders/dm-0", 777)
				assert.NoError(t, err)
				return fs
			},
			publishInfo: models.VolumePublishInfo{
				LUKSEncryption: "true",
				FilesystemType: filesystem.Raw,
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: "127.0.0.1",
						IscsiPortals:      []string{"127.0.0.2"},
						IscsiTargetIQN:    targetIQN,
						IscsiLunSerial:    vpdpg80Serial,
					},
				},
			},
			volumeName:       "test-volume",
			volumeMountPoint: "/mnt/test-volume",
			volumeAuthSecrets: map[string]string{
				"luks-passphrase":      "secretA",
				"luks-passphrase-name": "A",
			},
			assertError: assert.NoError,
		},
		"LUKS device has no existing file system type and is not LUKS formatted": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeoutAndInput(context.TODO(), "cryptsetup", 30*time.Second, true, "",
					"status",
					"/dev/mapper/luks-test-volume")
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
					nil)
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/mapper/luks-test-volume").Return("", nil)
				return mockDevices
			},
			getLuksDevice: func(controller *gomock.Controller) luks.Device {
				luksDevice := mock_luks.NewMockDevice(controller)
				luksDevice.EXPECT().MappedDevicePath().Return("/dev/mapper/dm-0")
				luksDevice.EXPECT().EnsureDeviceMappedOnHost(context.TODO(), "test-volume",
					map[string]string{}).Return(false, nil)
				return luksDevice
			},
			getFileSystemClient: func(controller *gomock.Controller) filesystem.Filesystem {
				mockFileSystem := mock_filesystem.NewMockFilesystem(controller)
				mockFileSystem.EXPECT().FormatVolume(context.TODO(), "/dev/mapper/luks-test-volume", filesystem.Ext4, "")
				mockFileSystem.EXPECT().RepairVolume(context.TODO(), "/dev/mapper/luks-test-volume", filesystem.Ext4)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) mount.Mount {
				mockMount := mock_mount.NewMockMount(controller)
				mockMount.EXPECT().IsMounted(context.TODO(), "/dev/mapper/luks-test-volume", "", "")
				mockMount.EXPECT().MountDevice(context.TODO(), "/dev/mapper/luks-test-volume", "/mnt/test-volume", "", false)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				return mockReconcileUtils
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				f, err := fs.Create("/dev/sda/vpd_pg80")
				assert.NoError(t, err)

				_, err = f.Write(vpdpg80SerialBytes(vpdpg80Serial))
				assert.NoError(t, err)

				_, err = fs.Create("/dev/sda/rescan")
				assert.NoError(t, err)

				_, err = fs.Create("/dev/sda/delete")
				assert.NoError(t, err)

				err = fs.MkdirAll("/sys/block/sda/holders/dm-0", 777)
				assert.NoError(t, err)
				return fs
			},
			publishInfo: models.VolumePublishInfo{
				LUKSEncryption: "true",
				FilesystemType: filesystem.Ext4,
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: "127.0.0.1",
						IscsiPortals:      []string{"127.0.0.2"},
						IscsiTargetIQN:    targetIQN,
						IscsiLunSerial:    vpdpg80Serial,
					},
				},
			},
			volumeName:       "test-volume",
			volumeMountPoint: "/mnt/test-volume",
			volumeAuthSecrets: map[string]string{
				"luks-passphrase":      "secretA",
				"luks-passphrase-name": "A",
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			iscsiClient := NewDetailed(params.chrootPathPrefix, params.getCommand(ctrl), DefaultSelfHealingExclusion,
				params.getOSClient(ctrl), params.getDeviceClient(ctrl), params.getFileSystemClient(ctrl),
				params.getMountClient(ctrl), params.getReconcileUtils(ctrl),
				afero.Afero{Fs: params.getFileSystemUtils()}, nil)

			mpathSize, err := iscsiClient.AttachVolume(context.TODO(), params.volumeName, params.volumeMountPoint,
				&params.publishInfo, params.volumeAuthSecrets)
			if params.assertError != nil {
				params.assertError(t, err)
			}

			assert.Equal(t, params.expectedMpathSize, mpathSize)
		})
	}
}

func TestISCSIActiveOnHost(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockExec := mockexec.NewMockCommand(mockCtrl)

	type args struct {
		ctx  context.Context
		host models.HostSystem
	}
	type expected struct {
		active bool
		err    error
	}
	tests := map[string]struct {
		name     string
		args     args
		expected expected
		service  string
	}{
		"ActiveCentos": {
			args: args{
				ctx:  context.Background(),
				host: models.HostSystem{OS: models.SystemOS{Distro: osutils.Centos}},
			},
			expected: expected{
				active: true,
				err:    nil,
			},
			service: "iscsid",
		},
		"ActiveRHEL": {
			args: args{
				ctx:  context.Background(),
				host: models.HostSystem{OS: models.SystemOS{Distro: osutils.RHEL}},
			},
			expected: expected{
				active: true,
				err:    nil,
			},
			service: "iscsid",
		},
		"ActiveUbuntu": {
			args: args{
				ctx:  context.Background(),
				host: models.HostSystem{OS: models.SystemOS{Distro: osutils.Ubuntu}},
			},
			expected: expected{
				active: true,
				err:    nil,
			},
			service: "open-iscsi",
		},
		"InactiveRHEL": {
			args: args{
				ctx:  context.Background(),
				host: models.HostSystem{OS: models.SystemOS{Distro: osutils.RHEL}},
			},
			expected: expected{
				active: false,
				err:    nil,
			},
			service: "iscsid",
		},
		"UnknownDistro": {
			args: args{
				ctx:  context.Background(),
				host: models.HostSystem{OS: models.SystemOS{Distro: "SUSE"}},
			},
			expected: expected{
				active: true,
				err:    nil,
			},
			service: "iscsid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec.EXPECT().ExecuteWithTimeout(
				tt.args.ctx, "systemctl", 30*time.Second, true, "is-active", tt.service,
			).Return([]byte(""), tt.expected.err)

			osUtils := osutils.NewDetailed(mockExec, afero.NewMemMapFs())
			iscsiClient := NewDetailed("", mockExec, nil, nil, nil, nil, nil, nil,
				afero.Afero{Fs: afero.NewMemMapFs()}, osUtils)
			active, err := iscsiClient.ISCSIActiveOnHost(tt.args.ctx, tt.args.host)
			if tt.expected.err != nil {
				assert.Error(t, err)
				assert.False(t, active)
			} else {
				assert.NoError(t, err)
				assert.True(t, active)
			}
		})
	}
}
