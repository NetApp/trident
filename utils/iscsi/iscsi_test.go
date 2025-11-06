// Copyright 2025 NetApp, Inc. All Rights Reserved.

package iscsi

import (
	"context"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/brunoga/deep"
	"github.com/spf13/afero"
	"github.com/spf13/afero/mem"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_devices"
	"github.com/netapp/trident/mocks/mock_utils/mock_devices/mock_luks"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/mocks/mock_utils/mock_filesystem"
	"github.com/netapp/trident/mocks/mock_utils/mock_iscsi"
	"github.com/netapp/trident/mocks/mock_utils/mock_mount"
	"github.com/netapp/trident/mocks/mock_utils/mock_osutils"
	"github.com/netapp/trident/pkg/network"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/devices/luks"
	"github.com/netapp/trident/utils/errors"
	tridentexec "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount"
	"github.com/netapp/trident/utils/osutils"
)

var ScsiScanZeros = []models.ScsiDeviceAddress{{Host: "0", Channel: "0", Target: "0", LUN: "0"}}

var mockPublushInfo models.VolumePublishInfo = models.VolumePublishInfo{
	FilesystemType: filesystem.Ext4,
	VolumeAccessInfo: models.VolumeAccessInfo{
		IscsiAccessInfo: models.IscsiAccessInfo{
			IscsiLunNumber:    1,
			IscsiTargetIQN:    "iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33111:vs.2",
			IscsiTargetPortal: "10.0.0.1:3260",
			IscsiPortals:      []string{""},
			IscsiChapInfo: models.IscsiChapInfo{
				UseCHAP:              true,
				IscsiUsername:        "testUser",
				IscsiInitiatorSecret: "testSecret",
				IscsiTargetUsername:  "targetUser",
				IscsiTargetSecret:    "targetSecret",
			},
		},
	},
}

func TestNew(t *testing.T) {
	originalPluginMode := os.Getenv("DOCKER_PLUGIN_MODE")
	defer func(pluginMode string) {
		err := os.Setenv("DOCKER_PLUGIN_MODE", pluginMode)
		assert.NoError(t, err)
	}(originalPluginMode)

	type parameters struct {
		setUpEnvironment func()
	}
	tests := map[string]parameters{
		"docker plugin mode": {
			setUpEnvironment: func() {
				assert.Nil(t, os.Setenv("DOCKER_PLUGIN_MODE", "true"))
			},
		},
		"not docker plugin mode": {},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			if params.setUpEnvironment != nil {
				params.setUpEnvironment()
			}

			iscsiClient, err := New()
			assert.NoError(t, err)
			assert.NotNil(t, iscsiClient)
		})
	}
}

func TestNewDetailed(t *testing.T) {
	const chrootPathPrefix = ""
	ctrl := gomock.NewController(t)
	osClient := mock_iscsi.NewMockOS(ctrl)
	devicesClient := mock_devices.NewMockDevices(ctrl)
	FileSystemClient := mock_filesystem.NewMockFilesystem(ctrl)
	mountClient := mock_mount.NewMockMount(ctrl)
	command := mockexec.NewMockCommand(ctrl)
	iscsiClient := NewDetailed(chrootPathPrefix, command, DefaultSelfHealingExclusion, osClient, devicesClient, FileSystemClient,
		mountClient, nil, afero.Afero{Fs: afero.NewMemMapFs()}, nil)
	assert.NotNil(t, iscsiClient)
}

func TestClient_AttachVolumeRetry(t *testing.T) {
	type parameters struct {
		chrootPathPrefix    string
		getCommand          func(controller *gomock.Controller) tridentexec.Command
		getOSClient         func(controller *gomock.Controller) OS
		getDeviceClient     func(controller *gomock.Controller) devices.Devices
		getFileSystemClient func(controller *gomock.Controller) filesystem.Filesystem
		getMountClient      func(controller *gomock.Controller) mount.Mount
		getReconcileUtils   func(controller *gomock.Controller) IscsiReconcileUtils
		getFileSystemUtils  func() afero.Fs

		publishInfo models.VolumePublishInfo

		assertError       assert.ErrorAssertionFunc
		expectedMpathSize int64
	}
	// the test timeout is set specifically to 2 seconds to ensure that the backoff retry runs one time.
	const testTimeout = 2 * time.Second
	const iscsiadmNodeOutput = `127.0.0.1:3260,1042 iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.25
127.0.0.1:3260,1043 iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.25
`
	const iscsiadmSessionOutput = `tcp: [3] 127.0.0.1:3260,1028 iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.3 (non-flash)
tcp: [4] 127.0.0.1:3260,1029 iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.3 (non-flash)`

	const iscsiadmDiscoveryDBSendTargetsOutput = `127.0.0.1:3260,1042 iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.3
127.0.0.1:3260,1043 iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.3
`
	tests := map[string]parameters{
		"happy path": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil).Times(2)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil).Times(2)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil).Times(2)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "session").
					Return([]byte(iscsiadmSessionOutput), nil).Times(2)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").
					Return([]byte(iscsiadmNodeOutput), nil).Times(2)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", gomock.Any(), "-I", "default", "-D").
					Return([]byte(iscsiadmDiscoveryDBSendTargetsOutput), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T",
					"iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.25", "-p", "127.0.0.1:3260", "-o", "update", "-n",
					"node.session.scan", "-v", "manual").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T",
					"iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.25", "-p", "127.0.0.1:3260", "-o", "update", "-n",
					"node.session.timeo.replacement_timeout", "-v", "5").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T",
					"iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.25", "-p", "127.0.0.1:3260", "--op=update", "--name",
					"node.conn[0].timeo.login_timeout", "--value=10").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T",
					"iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.25", "-p", "127.0.0.1:3260", "--op=update", "--name",
					"node.session.initial_login_retry_max", "--value=1").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"node", "-T", "iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.25", "-p", "127.0.0.1:3260",
					"--login").Return(nil, nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOS := mock_iscsi.NewMockOS(controller)
				mockOS.EXPECT().PathExists("/dev/sda/block").Return(true, nil)
				return mockOS
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(4)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
					nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(filesystem.Ext4, nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) filesystem.Filesystem {
				mockFileSystem := mock_filesystem.NewMockFilesystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) mount.Mount {
				mockMount := mock_mount.NewMockMount(controller)
				mockMount.EXPECT().IsMounted(context.TODO(), "/dev/dm-0", "", "").Return(true, nil)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					"iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.25").Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(3)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				return mockReconcileUtils
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				assert.NoError(t, fs.Mkdir("/sys/block/sda/holders", 777))
				_, err := fs.Create("/sys/block/sda/holders/dm-0")
				assert.NoError(t, err)
				return fs
			},
			publishInfo: models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: "127.0.0.1",
						IscsiPortals:      []string{""},
						IscsiTargetIQN:    "iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.25",
					},
				},
			},
			assertError:       assert.NoError,
			expectedMpathSize: 0,
		},
		"pre check failure": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, errors.New("some error"))
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				return mockDevices
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
				return mockReconcileUtils
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			publishInfo: models.VolumePublishInfo{},
			assertError: assert.Error,
		},
		"attach volume failure": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte("    find_multipaths: yes"), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil,
					errors.New("some error")).Times(2)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				return mockDevices
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
				return mockReconcileUtils
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			publishInfo: models.VolumePublishInfo{},
			assertError: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			iscsiClient := NewDetailed(params.chrootPathPrefix, params.getCommand(ctrl), DefaultSelfHealingExclusion,
				params.getOSClient(ctrl), params.getDeviceClient(ctrl), params.getFileSystemClient(ctrl),
				params.getMountClient(ctrl), params.getReconcileUtils(ctrl),
				afero.Afero{Fs: params.getFileSystemUtils()}, nil)

			mpathSize, err := iscsiClient.AttachVolumeRetry(context.TODO(), "", "", &params.publishInfo, nil,
				testTimeout)
			if params.assertError != nil {
				params.assertError(t, err)
			}

			assert.Equal(t, params.expectedMpathSize, mpathSize)
		})
	}
}

func TestClient_AttachVolume(t *testing.T) {
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
		"pre check failure: iscsiadm command not found": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				command := mockexec.NewMockCommand(controller)
				command.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, errors.New("iscsiadm not found"))
				return command
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				return mockDevices
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
				return mockReconcileUtils
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			publishInfo: models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
			},
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"pre check failure: multipathd not running": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").
					Return(nil, errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "multipathd", "show", "daemon").
					Return(nil, errors.New("some error"))
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				return mockDevices
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
				return mockReconcileUtils
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			publishInfo: models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
			},
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"pre check failure: find_multipaths value set to yes": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").
					Return(nil, errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "multipathd", "show", "daemon").
					Return([]byte("pid 2509 idle"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("yes", false)), nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				return mockDevices
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
				return mockReconcileUtils
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			publishInfo: models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
			},
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"pre check failure: find_multipaths value set to smart": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").
					Return(nil, errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "multipathd", "show", "daemon").
					Return([]byte("pid 2509 idle"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("smart", false)), nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				return mockDevices
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
				return mockReconcileUtils
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			publishInfo: models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
			},
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"portals to login: failed to get session info": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").
					Return(nil, errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "multipathd", "show", "daemon").
					Return([]byte("pid 2509 idle"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return(nil, errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "session").Return(nil, errors.New("some error"))
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				return mockDevices
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
				return mockReconcileUtils
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			publishInfo: models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
			},
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"portals to login: stale session state": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").
					Return(nil, errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "multipathd", "show", "daemon").
					Return([]byte("pid 2509 idle"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return(nil, errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "session").
					Return([]byte(iscsiadmSessionOutputOneSession), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").
					Return([]byte(iscsiadmNodeOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", gomock.Any(), "-I", "default", "-D").
					Return([]byte(iscsiadmDiscoveryDBSendTargetsOutput), nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(context.TODO())
				return mockDevices
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
				return mockReconcileUtils
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()

				f, err := fs.Create("/sys/class/iscsi_session/session3/state")
				assert.NoError(t, err)
				_, err = f.Write([]byte("foo"))
				assert.NoError(t, err)

				f, err = fs.Create("/sys/class/iscsi_session/session4/state")
				assert.NoError(t, err)
				_, err = f.Write([]byte("foo"))
				assert.NoError(t, err)

				return fs
			},
			publishInfo: models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetIQN:    targetIQN,
						IscsiTargetPortal: "127.0.0.1",
						IscsiPortals:      []string{"127.0.0.2"},
					},
				},
			},
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"handle invalid serial: failure rescanning the LUN": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(1)
				return mockDevices
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
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				return mockReconcileUtils
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				f, err := fs.Create("/dev/sda/vpd_pg80")
				assert.NoError(t, err)

				_, err = f.Write(vpdpg80SerialBytes(vpdpg80Serial))
				assert.NoError(t, err)
				return fs
			},
			publishInfo: models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: "127.0.0.1",
						IscsiPortals:      []string{"127.0.0.2"},
						IscsiTargetIQN:    targetIQN,
						IscsiLunSerial:    "SYA5GZFJ8G1M905GVH7I",
					},
				},
			},
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"handle invalid serial: failure purging the LUN": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(2)
				return mockDevices
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
					Times(2)
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
				return fs
			},
			publishInfo: models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: "127.0.0.1",
						IscsiPortals:      []string{"127.0.0.2"},
						IscsiTargetIQN:    targetIQN,
						IscsiLunSerial:    "SYA5GZFJ8G1M905GVH7I",
					},
				},
			},
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"wait for device: device not yet present": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "ls", "-al", "/dev").Return(nil, errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "ls", "-al", "/dev/mapper/").Return(nil,
					errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "ls", "-al", "/dev/disk/by-path").Return(nil,
					errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "lsscsi").Return(nil,
					errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "lsscsi", "-t").Return(nil,
					errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "free").Return(nil,
					errors.New("some error"))
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(false, errors.New("some error"))
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(2)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				return mockDevices
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
					Times(3)
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
				return fs
			},
			publishInfo: models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: "127.0.0.1",
						IscsiPortals:      []string{"127.0.0.2"},
						IscsiTargetIQN:    targetIQN,
						IscsiLunSerial:    "SYA5GZFJ8G1M905GVH7I",
					},
				},
			},
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"handle invalid serial: kernel has stale cache data": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				return mockDevices
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
					Times(4)
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
				return fs
			},
			publishInfo: models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: "127.0.0.1",
						IscsiPortals:      []string{"127.0.0.2"},
						IscsiTargetIQN:    targetIQN,
						IscsiLunSerial:    "SYA5GZFJ8G1M905GVH7I",
					},
				},
			},
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"wait for device: error getting device for LUN": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				return mockDevices
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
					Times(5)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return(nil, errors.New("some error"))
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
				return fs
			},
			publishInfo: models.VolumePublishInfo{
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"error getting device information for LUN": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(1)
				return mockDevices
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
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return(nil, errors.New("some error"))
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"failed to verify multipath device serial": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return("", errors.New("some error"))
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				return mockDevices
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"failure to verify multipath device size": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
					nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(errors.New("some error"))
				return mockDevices
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"verify multipath device size: size mismatch": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(errors.New("some error"))
				mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
					nil)
				return mockDevices
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"failure ensuring LUKS device mapped on host": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
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
				luksDevice.EXPECT().EnsureDeviceMappedOnHost(context.TODO(), "test-volume",
					map[string]string{}).Return(false, errors.New("some error"))
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"failed to get file system type of device": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"failure determining if device is formatted": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
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
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return("", nil)
				mockDevices.EXPECT().IsDeviceUnformatted(context.TODO(), "/dev/dm-0").Return(false, errors.New("some error"))
				mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
					nil)
				return mockDevices
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"device is already formatted": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
					nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return("", nil)
				mockDevices.EXPECT().IsDeviceUnformatted(context.TODO(), "/dev/dm-0").Return(false, nil)
				mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
				return mockDevices
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"failure formatting LUN": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
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
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return("", nil)
				mockDevices.EXPECT().IsDeviceUnformatted(context.TODO(), "/dev/dm-0").Return(true, nil)
				mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
					nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) filesystem.Filesystem {
				mockFileSystem := mock_filesystem.NewMockFilesystem(controller)
				mockFileSystem.EXPECT().FormatVolume(context.TODO(), "/dev/dm-0", filesystem.Ext4, "").Return(errors.New("some error"))
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"existing file system on device does not match the request file system": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
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
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(filesystem.Ext3, nil)
				mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
					nil)
				return mockDevices
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"failure determining if LUN is mounted": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
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
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(filesystem.UnknownFstype, nil)
				mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
					nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) filesystem.Filesystem {
				mockFileSystem := mock_filesystem.NewMockFilesystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) mount.Mount {
				mockMount := mock_mount.NewMockMount(controller)
				mockMount.EXPECT().IsMounted(context.TODO(), "/dev/dm-0", "", "").Return(false, errors.New("some error"))
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"no mount path provided": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
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
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(filesystem.UnknownFstype, nil)
				mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
					nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) filesystem.Filesystem {
				mockFileSystem := mock_filesystem.NewMockFilesystem(controller)
				mockFileSystem.EXPECT().RepairVolume(context.TODO(), "/dev/dm-0", filesystem.Ext4)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) mount.Mount {
				mockMount := mock_mount.NewMockMount(controller)
				mockMount.EXPECT().IsMounted(context.TODO(), "/dev/dm-0", "", "").Return(false, nil)
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
			volumeName:        "test-volume",
			volumeMountPoint:  "",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.NoError,
		},
		"failure mounting LUN": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
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
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(filesystem.UnknownFstype, nil)
				mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
					nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) filesystem.Filesystem {
				mockFileSystem := mock_filesystem.NewMockFilesystem(controller)
				mockFileSystem.EXPECT().RepairVolume(context.TODO(), "/dev/dm-0", filesystem.Ext4)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) mount.Mount {
				mockMount := mock_mount.NewMockMount(controller)
				mockMount.EXPECT().IsMounted(context.TODO(), "/dev/dm-0", "", "").Return(false, nil)
				mockMount.EXPECT().MountDevice(context.TODO(), "/dev/dm-0", "/mnt/test-volume", "",
					false).Return(errors.New("some error"))
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.Error,
		},
		"happy path": {
			chrootPathPrefix: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-V").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
					"config").Return([]byte(multipathConfig("no", false)), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).Times(3)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
				mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
					nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(filesystem.UnknownFstype, nil)
				mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) filesystem.Filesystem {
				mockFileSystem := mock_filesystem.NewMockFilesystem(controller)
				mockFileSystem.EXPECT().RepairVolume(context.TODO(), "/dev/dm-0", filesystem.Ext4)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) mount.Mount {
				mockMount := mock_mount.NewMockMount(controller)
				mockMount.EXPECT().IsMounted(context.TODO(), "/dev/dm-0", "", "").Return(false, nil)
				mockMount.EXPECT().MountDevice(context.TODO(), "/dev/dm-0", "/mnt/test-volume", "",
					false).Return(nil)
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
			volumeName:        "test-volume",
			volumeMountPoint:  "/mnt/test-volume",
			volumeAuthSecrets: make(map[string]string, 0),
			assertError:       assert.NoError,
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

func TestClient_AddSession(t *testing.T) {
	type parameters struct {
		sessions      *models.ISCSISessions
		publishInfo   models.VolumePublishInfo
		volID         string
		sessionNumber string
		reasonInvalid models.PortalInvalid
	}

	const solidfireTargetIQN = "iqn.2010-01.com.solidfire:target-1"
	const netappTargetIQN = "iqn.2010-01.com.netapp:target-1"
	const targetPortal = "127.0.0.1"

	tests := map[string]parameters{
		"empty sessions": {
			sessions: nil,
		},
		"solidfire target portal": {
			sessions: &models.ISCSISessions{},
			publishInfo: models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetIQN: solidfireTargetIQN,
					},
				},
			},
		},
		"portal already exists in session info, missing target IQN": {
			sessions: &models.ISCSISessions{
				Info: map[string]*models.ISCSISessionData{
					targetPortal: {},
				},
			},
			publishInfo: models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: targetPortal,
					},
				},
			},
		},
		"portal already exists in session info": {
			sessions: &models.ISCSISessions{
				Info: map[string]*models.ISCSISessionData{
					targetPortal: {},
				},
			},
			publishInfo: models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: targetPortal,
						IscsiTargetIQN:    netappTargetIQN,
					},
				},
			},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			client, err := New()
			assert.NoError(t, err)
			ctx := context.WithValue(context.TODO(), SessionInfoSource, "test")
			client.AddSession(ctx, params.sessions, &params.publishInfo, params.volID,
				params.sessionNumber, params.reasonInvalid)
		})
	}
}

func TestClient_filterDevicesBySize(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockDevices := mock_devices.NewMockDevices(mockCtrl)

	client := NewDetailed(
		"",
		nil,
		nil,
		nil,
		mockDevices,
		nil,
		nil,
		nil,
		afero.Afero{},
		nil,
	)

	ctx := context.TODO()
	deviceInfo := &models.ScsiDeviceInfo{
		Devices: []string{"sda", "sdb"},
	}
	minSize := int64(10)

	// Negative case.
	mockDevices.EXPECT().GetDiskSize(ctx, "/dev/sda").Return(int64(1), nil)
	mockDevices.EXPECT().GetDiskSize(ctx, "/dev/sdb").Return(int64(0), errors.New("failed to open disk"))
	deviceSizeMap, err := client.filterDevicesBySize(ctx, deviceInfo, minSize)
	assert.Error(t, err)
	assert.Nil(t, deviceSizeMap)
	assert.NotEqual(t, len(deviceInfo.Devices), len(deviceSizeMap))

	// Positive case #1: Only one device needs a resize.
	mockDevices.EXPECT().GetDiskSize(ctx, "/dev/sda").Return(minSize, nil)
	mockDevices.EXPECT().GetDiskSize(ctx, "/dev/sdb").Return(int64(1), nil)
	deviceSizeMap, err = client.filterDevicesBySize(ctx, deviceInfo, minSize)
	assert.NoError(t, err)
	assert.NotNil(t, deviceSizeMap)
	assert.NotEqual(t, len(deviceInfo.Devices), len(deviceSizeMap))

	// Positive case #2: All devices need to resize.
	mockDevices.EXPECT().GetDiskSize(ctx, "/dev/sda").Return(int64(1), nil)
	mockDevices.EXPECT().GetDiskSize(ctx, "/dev/sdb").Return(int64(1), nil)
	deviceSizeMap, err = client.filterDevicesBySize(ctx, deviceInfo, minSize)
	assert.NoError(t, err)
	assert.NotNil(t, deviceSizeMap)
	assert.Equal(t, len(deviceInfo.Devices), len(deviceSizeMap))
}

func TestClient_rescanDevices(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockDevices := mock_devices.NewMockDevices(mockCtrl)

	fs := afero.NewMemMapFs()
	_, err := fs.Create("/sys/block/sda/device/rescan")
	assert.NoError(t, err)

	client := NewDetailed(
		"",
		nil,
		nil,
		nil,
		mockDevices,
		nil,
		nil,
		nil,
		afero.Afero{Fs: fs},
		nil,
	)

	ctx := context.TODO()
	deviceSizeMap := map[string]int64{
		"sda": 1,
		"sdb": 1,
	}

	// Should fail because a device path does not exist.
	mockDevices.EXPECT().ListAllDevices(ctx).AnyTimes()
	err = client.rescanDevices(ctx, deviceSizeMap)
	assert.Error(t, err)

	// Add the missing device path.
	_, err = fs.Create("/sys/block/sdb/device/rescan")
	assert.NoError(t, err)

	// Should succeed now that the device path exists.
	err = client.rescanDevices(ctx, deviceSizeMap)
	assert.NoError(t, err)
}

func TestClient_RescanDevices(t *testing.T) {
	type parameters struct {
		targetIQN string
		lunID     int32
		minSize   int64

		getReconcileUtils  func(controller *gomock.Controller) IscsiReconcileUtils
		getDeviceClient    func(controller *gomock.Controller) devices.Devices
		getCommandClient   func(controller *gomock.Controller) tridentexec.Command
		getFileSystemUtils func() afero.Fs
		assertError        assert.ErrorAssertionFunc
	}

	const targetIQN = "iqn.2010-01.com.netapp:target-1"

	tests := map[string]parameters{
		"error getting device information": {
			targetIQN: targetIQN,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				return NewReconcileUtils()
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				return mockDevices
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				return fs
			},
			assertError: assert.Error,
		},
		"error getting iscsi disk size": {
			targetIQN: targetIQN,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), errors.New("some error"))
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")
				return mockDevices
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				return fs
			},
			assertError: assert.Error,
		},
		"failed to rescan disk": {
			targetIQN: targetIQN,
			minSize:   1,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(1)
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(1)
				return mockDevices
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				return fs
			},
			assertError: assert.Error,
		},
		"failure to resize the disk": {
			targetIQN: targetIQN,
			minSize:   1,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil).Times(2)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(1)
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)
				return mockDevices
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create("/sys/block/sda/device/rescan")
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.Error,
		},
		"error validating if disk is resized": {
			targetIQN: targetIQN,
			minSize:   1,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), errors.New("some error"))
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(1)
				return mockDevices
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create("/sys/block/sda/device/rescan")
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.Error,
		},
		"disk resized successfully": {
			targetIQN: targetIQN,
			minSize:   1,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(1)

				// This will be called twice because we read from each disk twice during an expansion.
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil).Times(1)
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil).Times(1)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(1), nil)
				return mockDevices
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create("/sys/block/sda/device/rescan")
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.NoError,
		},
		"failure getting multipath device size": {
			targetIQN: targetIQN,
			minSize:   1,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(1), errors.New("some error"))
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)
				return mockDevices
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create("/sys/block/sda/device/rescan")
				assert.NoError(t, err)

				_, err = fs.Create("/sys/block/sda/holders/dm-0")
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.Error,
		},
		"multipath device size already greater than min size": {
			targetIQN: targetIQN,
			minSize:   1,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(1), nil)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)
				return mockDevices
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create("/sys/block/sda/device/rescan")
				assert.NoError(t, err)

				_, err = fs.Create("/sys/block/sda/holders/dm-0")
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.NoError,
		},
		"failure reloading multipaths map": {
			targetIQN: targetIQN,
			minSize:   1,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)
				return mockDevices
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipath", 10*time.Second, true, "-r",
					"/dev/dm-0").Return(nil, errors.New("some error"))
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create("/sys/block/sda/device/rescan")
				assert.NoError(t, err)

				_, err = fs.Create("/sys/block/sda/holders/dm-0")
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.Error,
		},
		"error determining the size of the multipath device after reload": {
			targetIQN: targetIQN,
			minSize:   1,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), errors.New("some error"))
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)
				return mockDevices
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipath", 10*time.Second, true, "-r",
					"/dev/dm-0").Return(nil, nil)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create("/sys/block/sda/device/rescan")
				assert.NoError(t, err)

				_, err = fs.Create("/sys/block/sda/holders/dm-0")
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.Error,
		},
		"multipath device too small even after reloading multipath map": {
			targetIQN: targetIQN,
			minSize:   1,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil).Times(2)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)
				return mockDevices
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipath", 10*time.Second, true, "-r",
					"/dev/dm-0").Return(nil, nil)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create("/sys/block/sda/device/rescan")
				assert.NoError(t, err)

				_, err = fs.Create("/sys/block/sda/holders/dm-0")
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.Error,
		},
		"multipath device successfully resized": {
			targetIQN: targetIQN,
			minSize:   1,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(1), nil)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(1)
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)
				return mockDevices
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipath", 10*time.Second, true, "-r",
					"/dev/dm-0").Return(nil, nil)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create("/sys/block/sda/device/rescan")
				assert.NoError(t, err)

				_, err = fs.Create("/sys/block/sda/holders/dm-0")
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.NoError,
		},
		"happy path": {
			minSize:   10,
			targetIQN: targetIQN,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(1)

				// This will be called twice because we read from each disk twice during an expansion.
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil).Times(1)
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(10), nil).Times(1)
				mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(10), nil)
				return mockDevices
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create("/sys/block/sda/device/rescan")
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			controller := gomock.NewController(t)

			client := NewDetailed("", params.getCommandClient(controller), DefaultSelfHealingExclusion, nil,
				params.getDeviceClient(controller), nil, nil, params.getReconcileUtils(controller),
				afero.Afero{Fs: params.getFileSystemUtils()}, nil)

			err := client.RescanDevices(context.TODO(), params.targetIQN, params.lunID, params.minSize)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestClient_reloadMultipathDevice(t *testing.T) {
	type parameters struct {
		multipathDeviceName string
		getCommand          func(controller *gomock.Controller) tridentexec.Command
		assertError         assert.ErrorAssertionFunc
	}

	const multipathDeviceName = "dm-0"
	const moultipathDevicePath = "/dev/" + multipathDeviceName

	tests := map[string]parameters{
		"no multipath device provided": {
			multipathDeviceName: "",
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				return mockCommand
			},
			assertError: assert.Error,
		},
		"error executing multipath map reload": {
			multipathDeviceName: multipathDeviceName,
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipath", 10*time.Second, true, "-r",
					moultipathDevicePath).Return(nil, errors.New("some error"))
				return mockCommand
			},
			assertError: assert.Error,
		},
		"happy path": {
			multipathDeviceName: multipathDeviceName,
			getCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipath", 10*time.Second, true, "-r",
					moultipathDevicePath).Return(nil, nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewDetailed("", params.getCommand(ctrl), nil, nil, nil, nil, nil, nil, afero.Afero{}, nil)

			err := client.reloadMultipathDevice(context.TODO(), params.multipathDeviceName)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestClient_IsAlreadyAttached(t *testing.T) {
	type parameters struct {
		lunID          int
		targetIQN      string
		getIscsiUtils  func(controller *gomock.Controller) IscsiReconcileUtils
		assertResponse assert.BoolAssertionFunc
	}

	const targetIQN = "iqn.2010-01.com.netapp:target-1"

	tests := map[string]parameters{
		"no existing host sessions": {
			lunID:     0,
			targetIQN: targetIQN,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).Return(nil)
				return mockReconcileUtils
			},
			assertResponse: assert.False,
		},
		"error getting devices for LUN": {
			lunID:     0,
			targetIQN: targetIQN,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return(nil, errors.New("some error"))
				return mockReconcileUtils
			},
			assertResponse: assert.False,
		},
		"no devices for LUN": {
			lunID:     0,
			targetIQN: targetIQN,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return(nil, nil)
				return mockReconcileUtils
			},
			assertResponse: assert.False,
		},
		"happy path": {
			lunID:     0,
			targetIQN: targetIQN,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			assertResponse: assert.True,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			controller := gomock.NewController(t)
			client := NewDetailed("", nil, nil, nil, nil, nil, nil, params.getIscsiUtils(controller), afero.Afero{}, nil)

			attached := client.IsAlreadyAttached(context.TODO(), params.lunID, params.targetIQN)
			if params.assertResponse != nil {
				params.assertResponse(t, attached)
			}
		})
	}
}

func TestClient_getDeviceInfoForLUN(t *testing.T) {
	type parameters struct {
		hostSessionMap map[int]int
		lunID          int
		iSCSINodeName  string
		needFS         bool

		getIscsiUtils      func(controller *gomock.Controller) IscsiReconcileUtils
		getDevicesClient   func(controller *gomock.Controller) devices.Devices
		getFileSystemUtils func() afero.Fs

		assertError        assert.ErrorAssertionFunc
		expectedDeviceInfo *models.ScsiDeviceInfo
	}

	const iscisNodeName = "iqn.2010-01.com.netapp:target-1"
	const multipathDeviceName = "dm-0"
	const multipathDevicePath = "/dev/" + multipathDeviceName
	const deviceName = "sda"
	const devicePath = "/dev/" + deviceName

	tests := map[string]parameters{
		"no host session information present": {
			lunID:         0,
			iSCSINodeName: iscisNodeName,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return(make([]string, 0))
				mockIscsiUtils.EXPECT().GetDevicesForLUN(gomock.Any()).Return(make([]string, 0), nil)
				return mockIscsiUtils
			},
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(controller)
				return mockDeviceClient
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:        assert.NoError,
			expectedDeviceInfo: nil,
		},
		"error getting devices for LUN": {
			lunID:         0,
			iSCSINodeName: iscisNodeName,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
				mockIscsiUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return(nil, errors.New("some error"))
				return mockIscsiUtils
			},
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(controller)
				return mockDeviceClient
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:        assert.Error,
			expectedDeviceInfo: nil,
		},
		"no devices for LUN": {
			lunID:         0,
			iSCSINodeName: iscisNodeName,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
				mockIscsiUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return(nil, nil)
				return mockIscsiUtils
			},
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(controller)
				return mockDeviceClient
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:        assert.NoError,
			expectedDeviceInfo: nil,
		},
		"no multipath device found": {
			lunID:         0,
			iSCSINodeName: iscisNodeName,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
				mockIscsiUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return([]string{deviceName}, nil)
				return mockIscsiUtils
			},
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(controller)
				mockDeviceClient.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("")
				return mockDeviceClient
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError: assert.NoError,
			expectedDeviceInfo: &models.ScsiDeviceInfo{
				ScsiDeviceAddress: models.ScsiDeviceAddress{LUN: "0"},
				Devices:           []string{deviceName},
				DevicePaths:       []string{devicePath},
				IQN:               iscisNodeName,
			},
		},
		"error ensuring multipath device is readable": {
			lunID:         0,
			iSCSINodeName: iscisNodeName,
			needFS:        true,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
				mockIscsiReconcileUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return([]string{deviceName}, nil)
				return mockIscsiReconcileUtils
			},
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(controller)
				mockDeviceClient.EXPECT().EnsureDeviceReadable(context.TODO(), multipathDevicePath).Return(errors.New("some error"))
				mockDeviceClient.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")
				return mockDeviceClient
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				err := fs.Mkdir(fmt.Sprintf("/sys/block/sda/holders/%s", multipathDeviceName), 777)
				assert.NoError(t, err)
				return fs
			},
			assertError:        assert.Error,
			expectedDeviceInfo: nil,
		},
		"error getting file system type from multipath device": {
			lunID:         0,
			iSCSINodeName: iscisNodeName,
			needFS:        true,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
				mockIscsiReconcileUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return([]string{deviceName}, nil)
				return mockIscsiReconcileUtils
			},
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(controller)
				mockDeviceClient.EXPECT().EnsureDeviceReadable(context.TODO(), multipathDevicePath).Return(nil)
				mockDeviceClient.EXPECT().GetDeviceFSType(context.TODO(), multipathDevicePath).Return("", errors.New("some error"))
				mockDeviceClient.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")
				return mockDeviceClient
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				err := fs.Mkdir(fmt.Sprintf("/sys/block/sda/holders/%s", multipathDeviceName), 777)
				assert.NoError(t, err)
				return fs
			},
			assertError:        assert.Error,
			expectedDeviceInfo: nil,
		},
		"happy path": {
			hostSessionMap: map[int]int{0: 0},
			lunID:          0,
			needFS:         true,
			iSCSINodeName:  iscisNodeName,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
				mockIscsiUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return([]string{deviceName}, nil)
				return mockIscsiUtils
			},
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDeviceClient := mock_devices.NewMockDevices(controller)
				mockDeviceClient.EXPECT().EnsureDeviceReadable(context.TODO(), multipathDevicePath).Return(nil)
				mockDeviceClient.EXPECT().GetDeviceFSType(context.TODO(), multipathDevicePath).Return(filesystem.Ext4, nil)
				mockDeviceClient.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")
				return mockDeviceClient
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				err := fs.Mkdir(fmt.Sprintf("/sys/block/sda/holders/%s", multipathDeviceName), 777)
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.NoError,
			expectedDeviceInfo: &models.ScsiDeviceInfo{
				ScsiDeviceAddress: models.ScsiDeviceAddress{LUN: "0"},
				Devices:           []string{deviceName},
				DevicePaths:       []string{devicePath},
				MultipathDevice:   multipathDeviceName,
				IQN:               iscisNodeName,
				Filesystem:        filesystem.Ext4,
			},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewDetailed("", nil, nil, nil, params.getDevicesClient(ctrl), nil, nil,
				params.getIscsiUtils(ctrl),
				afero.Afero{
					Fs: params.getFileSystemUtils(),
				}, nil)
			deviceInfo, err := client.GetDeviceInfoForLUN(context.TODO(), params.hostSessionMap, params.lunID,
				params.iSCSINodeName, params.needFS)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedDeviceInfo, deviceInfo)
		})
	}
}

func TestClient_purgeOneLun(t *testing.T) {
	type parameters struct {
		path               string
		getFileSystemUtils func() afero.Fs

		assertError assert.ErrorAssertionFunc
	}
	const devicePath = "/dev/sda"
	tests := map[string]parameters{
		"error opening delete file": {
			path: devicePath,
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				return fs
			},
			assertError: assert.Error,
		},
		"error writing to file": {
			path: devicePath,
			getFileSystemUtils: func() afero.Fs {
				f := &aferoFileWrapper{
					WriteStringError: errors.New("some error"),
					File:             mem.NewFileHandle(&mem.FileData{}),
				}

				memFs := afero.NewMemMapFs()
				_, err := memFs.Create("/dev/sda/delete")
				assert.NoError(t, err)

				fs := &aferoWrapper{
					openFileResponse: f,
					Fs:               memFs,
				}

				return fs
			},
		},
		"unable to  write to file": {
			path: devicePath,
			getFileSystemUtils: func() afero.Fs {
				f := &aferoFileWrapper{
					WriteStringCount: 0,
					File:             mem.NewFileHandle(&mem.FileData{}),
				}

				memFs := afero.NewMemMapFs()
				_, err := memFs.Create("/dev/sda/delete")
				assert.NoError(t, err)

				fs := &aferoWrapper{
					openFileResponse: f,
					Fs:               memFs,
				}

				return fs
			},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			client := NewDetailed("", nil, nil, nil, nil, nil, nil, nil, afero.Afero{Fs: params.getFileSystemUtils()}, nil)
			err := client.purgeOneLun(context.TODO(), params.path)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestClient_rescanOneLun(t *testing.T) {
	type parameters struct {
		path               string
		getFileSystemUtils func() afero.Fs
		assertError        assert.ErrorAssertionFunc
	}

	const devicePath = "/dev/sda"

	tests := map[string]parameters{
		"error opening rescan file": {
			path: devicePath,
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				return fs
			},
			assertError: assert.Error,
		},
		"error writing to rescan file": {
			path: devicePath,
			getFileSystemUtils: func() afero.Fs {
				f := &aferoFileWrapper{
					WriteStringError: errors.New("some error"),
					File:             mem.NewFileHandle(&mem.FileData{}),
				}

				memFs := afero.NewMemMapFs()
				_, err := memFs.Create(fmt.Sprintf("%s/rescan", devicePath))
				assert.NoError(t, err)

				fs := &aferoWrapper{
					openFileResponse: f,
					Fs:               memFs,
				}

				return fs
			},
		},
		"failed to write to rescan file": {
			path: devicePath,
			getFileSystemUtils: func() afero.Fs {
				f := &aferoFileWrapper{
					WriteStringCount: 0,
					File:             mem.NewFileHandle(&mem.FileData{}),
				}

				memFs := afero.NewMemMapFs()
				_, err := memFs.Create(fmt.Sprintf("%s/rescan", devicePath))
				assert.NoError(t, err)

				fs := &aferoWrapper{
					openFileResponse: f,
					Fs:               memFs,
				}

				return fs
			},
		},
		"happy path": {
			path: devicePath,
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create(fmt.Sprintf("%s/rescan", devicePath))
				assert.NoError(t, err)
				return fs
			},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			client := NewDetailed("", nil, nil, nil, nil, nil, nil, nil, afero.Afero{Fs: params.getFileSystemUtils()}, nil)

			err := client.rescanOneLun(context.TODO(), params.path)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestClient_waitForMultipathDeviceForLUN(t *testing.T) {
	type parameters struct {
		hostSessionMap     map[int]int
		getIscsiUtils      func(controller *gomock.Controller) IscsiReconcileUtils
		getFileSystemUtils func() afero.Fs
		assertError        assert.ErrorAssertionFunc
		getDevices         func(controller *gomock.Controller) devices.Devices
	}

	const lunID = 0
	const iscsiNodeName = "iqn.2010-01.com.netapp:target-1"
	const deviceName = "sda"
	const devicePath = "/dev/" + deviceName
	const multipathDeviceName = "dm-0"
	const holdersDirectory = "/sys/block/" + deviceName + "/holders/" + multipathDeviceName

	tests := map[string]parameters{
		"no host session mappings present": {
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return(make([]string, 0))
				mockIscsiUtils.EXPECT().GetDevicesForLUN(gomock.Any()).Return(make([]string, 0), nil)
				return mockIscsiUtils
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError: assert.Error,
		},
		"error getting devices for LUN": {
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
				mockIscsiUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return(nil, errors.New("some error"))
				return mockIscsiUtils
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError: assert.Error,
		},
		"error getting multipath device": {
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
				mockIscsiUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return([]string{deviceName}, nil)
				return mockIscsiUtils
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("")
				return mockDevices
			},
			assertError: assert.Error,
		},
		"happy path": {
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
				mockIscsiUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return([]string{deviceName}, nil)
				return mockIscsiUtils
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create(holdersDirectory)
				assert.NoError(t, err)
				return fs
			},
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")
				return mockDevices
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			var deviceClient devices.Devices
			ctrl := gomock.NewController(t)
			if params.getDevices != nil {
				deviceClient = params.getDevices(ctrl)
			}
			client := NewDetailed("", nil, nil, nil, deviceClient, nil, nil, params.getIscsiUtils(ctrl),
				afero.Afero{Fs: params.getFileSystemUtils()}, nil)

			err := client.waitForMultipathDeviceForLUN(context.TODO(), params.hostSessionMap, lunID, iscsiNodeName)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestClient_waitForDeviceScan(t *testing.T) {
	type parameters struct {
		hostSessionMap     map[int]int
		getIscsiUtils      func(controller *gomock.Controller) IscsiReconcileUtils
		getCommandClient   func(controller *gomock.Controller) tridentexec.Command
		getOsClient        func(controller *gomock.Controller) OS
		getFileSystemUtils func() afero.Fs
		getDevices         func(controller *gomock.Controller) devices.Devices
		assertError        assert.ErrorAssertionFunc
	}

	const lunID = 0
	const iscsiNodeName = "iqn.2010-01.com.netapp:target-1"
	const devicePath1 = "/dev/sda"
	const devicePath2 = "/dev/sdb"
	const devicePath3 = "/dev/sdc"

	tests := map[string]parameters{
		"no host session mappings present": {
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return(make([]string, 0))
				return mockIscsiUtils
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), gomock.Any(), gomock.Any()).Times(6).Return(nil, nil)
				return mockCommand
			},
			getOsClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				return mockOsClient
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(gomock.NewController(t))
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), []models.ScsiDeviceAddress{})
				return mockDevices
			},
			assertError: assert.Error,
		},
		"some devices present": {
			hostSessionMap: map[int]int{0: 0},
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath1, devicePath2, devicePath3})
				return mockIscsiUtils
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				return mockCommand
			},
			getOsClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists(devicePath1+"/block").Return(true, nil)
				mockOsClient.EXPECT().PathExists(devicePath2+"/block").Return(false, nil)
				mockOsClient.EXPECT().PathExists(devicePath3+"/block").Return(false, errors.New("some error"))
				return mockOsClient
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				return mockDevices
			},
			assertError: assert.NoError,
		},
		"all devices present": {
			hostSessionMap: map[int]int{0: 0},
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath1, devicePath2})
				return mockIscsiUtils
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				return mockCommand
			},
			getOsClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists(devicePath1+"/block").Return(true, nil)
				mockOsClient.EXPECT().PathExists(devicePath2+"/block").Return(true, nil)
				return mockOsClient
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				return mockDevices
			},
			assertError: assert.NoError,
		},
		"no devices present": {
			hostSessionMap: map[int]int{0: 0},
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return(nil)
				return mockIscsiUtils
			},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "ls", gomock.Any()).Return(nil,
					errors.New("some error")).Times(3)
				mockCommand.EXPECT().Execute(context.TODO(), "lsscsi").Return(nil, errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "lsscsi", "-t").Return(nil, errors.New("some error"))
				mockCommand.EXPECT().Execute(context.TODO(), "free").Return(nil, errors.New("some error"))
				return mockCommand
			},
			getOsClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				return mockOsClient
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ScanTargetLUN(context.TODO(), ScsiScanZeros)
				return mockDevices
			},
			assertError: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			var deviceClient devices.Devices
			if params.getDevices != nil {
				deviceClient = params.getDevices(ctrl)
			}
			client := NewDetailed("", params.getCommandClient(ctrl), nil, params.getOsClient(ctrl), deviceClient, nil, nil,
				params.getIscsiUtils(ctrl),
				afero.Afero{Fs: params.getFileSystemUtils()}, nil)

			err := client.waitForDeviceScan(context.TODO(), params.hostSessionMap, lunID, iscsiNodeName)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestClient_handleInvalidSerials(t *testing.T) {
	type parameters struct {
		hostSessionMap map[int]int
		expectedSerial string
		handlerError   error

		getIscsiUtils      func(controller *gomock.Controller) IscsiReconcileUtils
		getFileSystemUtils func() afero.Fs
		getDevicesClient   func(controller *gomock.Controller) devices.Devices

		assertError         assert.ErrorAssertionFunc
		assertHandlerCalled assert.BoolAssertionFunc
	}

	const lunID = 0
	const targetIQN = "iqn.2010-01.com.netapp:target-1"
	const vpdpg80Serial = "SYA5GZFJ8G1M905GVH7H"
	const devicePath = "/dev/sda"

	tests := map[string]parameters{
		"empty serial passed in": {
			expectedSerial: "",
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				return mockIscsiUtils
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError: assert.NoError,
		},
		"serial files do not exist": {
			hostSessionMap: map[int]int{0: 0},
			expectedSerial: vpdpg80Serial,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
				return mockIscsiUtils
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), devicePath).Return(vpdpg80Serial, nil)
				return mockDevices
			},
			assertError: assert.NoError,
		},
		"error reading serial files": {
			hostSessionMap: map[int]int{0: 0},
			expectedSerial: vpdpg80Serial,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
				return mockIscsiUtils
			},
			getFileSystemUtils: func() afero.Fs {
				fs := &aferoWrapper{
					openError: errors.New("some error"),
					Fs:        afero.NewMemMapFs(),
				}
				return fs
			},
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), devicePath).Return("", errors.New("mock error"))
				return mockDevices
			},
			assertError: assert.Error,
		},
		"lun serial does not match expected serial": {
			hostSessionMap: map[int]int{0: 0},
			expectedSerial: "SYA5GZFJ8G1M905GVH7I",
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
				return mockIscsiUtils
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				f, err := fs.Create(fmt.Sprintf("%s/vpd_pg80", devicePath))
				assert.NoError(t, err)
				_, err = f.Write(vpdpg80SerialBytes(vpdpg80Serial))
				assert.NoError(t, err)
				return fs
			},
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), devicePath).Return(vpdpg80Serial, nil)
				return mockDevices
			},
			assertHandlerCalled: assert.True,
			handlerError:        errors.New("some error"),
			assertError:         assert.Error,
		},
		"happy path": {
			hostSessionMap: map[int]int{0: 0},
			expectedSerial: vpdpg80Serial,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
				return mockIscsiUtils
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				f, err := fs.Create(fmt.Sprintf("%s/vpd_pg80", devicePath))
				assert.NoError(t, err)
				_, err = f.Write(vpdpg80SerialBytes(vpdpg80Serial))
				assert.NoError(t, err)
				return fs
			},
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetLunSerial(context.TODO(), devicePath).Return(vpdpg80Serial, nil)
				return mockDevices
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			handlerCalled := false
			mockHandler := func(ctx context.Context, path string) error {
				handlerCalled = true
				return params.handlerError
			}

			ctrl := gomock.NewController(t)

			var devicesClient devices.Devices
			if params.getDevicesClient != nil {
				devicesClient = params.getDevicesClient(ctrl)
			}

			client := NewDetailed("", nil, nil, nil, devicesClient, nil, nil, params.getIscsiUtils(ctrl),
				afero.Afero{Fs: params.getFileSystemUtils()}, nil)

			err := client.handleInvalidSerials(context.TODO(), params.hostSessionMap, lunID, targetIQN, params.expectedSerial, mockHandler)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			if params.assertHandlerCalled != nil {
				params.assertHandlerCalled(t, handlerCalled)
			}
		})
	}
}

func TestClient_portalsToLogin(t *testing.T) {
	type parameters struct {
		getCommandClient    func(controller *gomock.Controller) tridentexec.Command
		getFileSystemUtils  func() afero.Fs
		portalsNeedingLogin []string
		assertError         assert.ErrorAssertionFunc
		assertLoggedIn      assert.BoolAssertionFunc
	}

	const targetIQN = "iqn.2010-01.com.netapp:target-1"
	const alternateTargetIQN = "iqn.2010-01.com.netapp:target-2"
	const portal1 = "127.0.0.1"
	const portal2 = "127.0.0.2"
	const iscsiadmSessionOutput = `tcp: [3] 127.0.0.1:3260,1028 ` + targetIQN + ` (non-flash)
tcp: [4] 127.0.0.2:3260,1029 ` + targetIQN + ` (non-flash)`
	const iscsiadmSessionOutputAlternateIQN = `tcp: [3] 127.0.0.1:3260,1028 ` + alternateTargetIQN + ` (non-flash)
tcp: [4] 127.0.0.2:3260,1029 ` + alternateTargetIQN + ` (non-flash)`

	tests := map[string]parameters{
		"error getting session info": {
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "session").Return(nil, errors.New("some error"))
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:         assert.Error,
			assertLoggedIn:      assert.False,
			portalsNeedingLogin: []string{portal1, portal2},
		},
		"targetIQN does not match the session targetIQN": {
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "session").Return([]byte(iscsiadmSessionOutputAlternateIQN), nil)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:         assert.NoError,
			assertLoggedIn:      assert.False,
			portalsNeedingLogin: []string{portal1, portal2},
		},
		"all sessions stale": {
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "session").Return([]byte(iscsiadmSessionOutput), nil)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				f, err := fs.Create("/sys/class/iscsi_session/session3/state")
				assert.NoError(t, err)
				_, err = f.Write([]byte("foo"))
				assert.NoError(t, err)
				f, err = fs.Create("/sys/class/iscsi_session/session4/state")
				assert.NoError(t, err)
				_, err = f.Write([]byte("foo"))
				assert.NoError(t, err)
				return fs
			},
		},
		"happy path": {
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:         assert.NoError,
			assertLoggedIn:      assert.True,
			portalsNeedingLogin: nil,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewDetailed("", params.getCommandClient(ctrl), nil, nil, nil, nil, nil, nil,
				afero.Afero{Fs: params.getFileSystemUtils()}, nil)

			portalsNeedingLogin, loggedIn, err := client.portalsToLogin(context.TODO(), targetIQN, []string{
				portal1,
				portal2,
			})
			if params.assertError != nil {
				params.assertError(t, err)
			}

			if params.assertLoggedIn != nil {
				params.assertLoggedIn(t, loggedIn)
			}
			assert.Equal(t, params.portalsNeedingLogin, portalsNeedingLogin)
		})
	}
}

func TestClient_verifyMultipathDeviceSerial(t *testing.T) {
	type parameters struct {
		lunSerial   string
		getDevices  func(controller *gomock.Controller) devices.Devices
		assertError assert.ErrorAssertionFunc
	}

	const multipathDeviceName = "dm-0"
	const vpdpg80Serial = "SYA5GZFJ8G1M905GVH7H"
	lunSerialHex := hex.EncodeToString([]byte(vpdpg80Serial))
	multipathdeviceSerial := fmt.Sprintf("mpath-%s", lunSerialHex)

	tests := map[string]parameters{
		"empty lun serial": {
			lunSerial:   "",
			assertError: assert.NoError,
		},
		"multipath device not present": {
			lunSerial: vpdpg80Serial,
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetMultipathDeviceUUID(multipathDeviceName).Return("",
					errors.NotFoundError("not found"))
				return mockDevices
			},
			assertError: assert.NoError,
		},
		"error getting multipath device UUID": {
			lunSerial: vpdpg80Serial,
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetMultipathDeviceUUID(multipathDeviceName).Return("",
					errors.New("some error"))
				return mockDevices
			},
			assertError: assert.Error,
		},
		"LUN serial not present in multipath device UUID": {
			lunSerial: vpdpg80Serial,
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetMultipathDeviceUUID(multipathDeviceName).Return("foo", nil)
				return mockDevices
			},
			assertError: assert.Error,
		},
		"happy path": {
			lunSerial: vpdpg80Serial,
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().GetMultipathDeviceUUID(multipathDeviceName).Return(multipathdeviceSerial, nil)
				return mockDevices
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			var devicesClient devices.Devices
			if params.getDevices != nil {
				devicesClient = params.getDevices(ctrl)
			}
			client := NewDetailed("", nil, nil, nil, devicesClient, nil, nil, nil, afero.Afero{}, nil)
			err := client.verifyMultipathDeviceSerial(context.TODO(), multipathDeviceName, params.lunSerial)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestClient_EnsureSessions(t *testing.T) {
	type parameters struct {
		publishInfo        models.VolumePublishInfo
		portals            []string
		getCommandClient   func(controller *gomock.Controller) tridentexec.Command
		getDevices         func(controller *gomock.Controller) devices.Devices
		getFileSystemUtils func() afero.Fs
		assertError        assert.ErrorAssertionFunc
		assertResult       assert.BoolAssertionFunc
	}

	const portal1 = "127.0.0.1"
	const portal2 = "127.0.0.2"
	const targetIQN = "iqn.2010-01.com.netapp:target-1"

	const iscsiadmNodeOutput = portal1 + `:3260,1042 ` + targetIQN + `
` + portal2 + `:3260,1043 ` + targetIQN + `
`
	const iscsiadmSessionOutput = `tcp: [3] 127.0.0.1:3260,1028 ` + targetIQN + ` (non-flash)
tcp: [4] 127.0.0.2:3260,1029 ` + targetIQN + ` (non-flash)`

	tests := map[string]parameters{
		"no portals provided to login": {
			publishInfo: models.VolumePublishInfo{},
			portals:     []string{},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				return mockCommand
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:  assert.Error,
			assertResult: assert.False,
		},
		"failed to ensure target": {
			publishInfo: models.VolumePublishInfo{},
			portals:     []string{portal1},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return(nil, errors.New("some error"))
				return mockCommand
			},
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(context.TODO())
				return mockDevices
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:  assert.Error,
			assertResult: assert.False,
		},
		"error setting node session replacement timeout": {
			publishInfo: models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetIQN: targetIQN,
					},
				},
			},
			portals: []string{portal1},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return([]byte(iscsiadmNodeOutput), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "-o", "update", "-n", "node.session.scan", "-v",
					"manual").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "-o", "update", "-n", "node.session.timeo.replacement_timeout",
					"-v", "5").Return(nil, errors.New("some error"))

				return mockCommand
			},
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(context.TODO())
				return mockDevices
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:  assert.Error,
			assertResult: assert.False,
		},
		"error logging into target": {
			publishInfo: models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetIQN: targetIQN,
					},
				},
			},
			portals: []string{portal1},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return([]byte(iscsiadmNodeOutput), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "-o", "update", "-n", "node.session.scan", "-v",
					"manual").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "-o", "update", "-n", "node.session.timeo.replacement_timeout",
					"-v", "5").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "--op=update", "--name", "node.conn[0].timeo.login_timeout",
					"--value=10").Return(nil, errors.New("some error"))

				return mockCommand
			},
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)
				return mockDevices
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:  assert.Error,
			assertResult: assert.False,
		},
		"error verifying if the session exists": {
			publishInfo: models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetIQN: targetIQN,
					},
				},
			},
			portals: []string{portal1},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return([]byte(iscsiadmNodeOutput), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "-o", "update", "-n", "node.session.scan", "-v",
					"manual").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "-o", "update", "-n", "node.session.timeo.replacement_timeout",
					"-v", "5").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "--op=update", "--name", "node.conn[0].timeo.login_timeout",
					"--value=10").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "--op=update", "--name", "node.session.initial_login_retry_max",
					"--value=1").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"node", "-T", targetIQN, "-p", fmt.Sprintf("%s:3260", portal1), "--login").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return(nil, errors.New("some error"))

				return mockCommand
			},
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(3)
				return mockDevices
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:  assert.Error,
			assertResult: assert.False,
		},
		"session does not exist": {
			publishInfo: models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetIQN: targetIQN,
					},
				},
			},
			portals: []string{portal1},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return([]byte(iscsiadmNodeOutput), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "-o", "update", "-n", "node.session.scan", "-v",
					"manual").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "-o", "update", "-n", "node.session.timeo.replacement_timeout",
					"-v", "5").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "--op=update", "--name", "node.conn[0].timeo.login_timeout",
					"--value=10").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "--op=update", "--name", "node.session.initial_login_retry_max",
					"--value=1").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"node", "-T", targetIQN, "-p", fmt.Sprintf("%s:3260", portal1), "--login").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(""), nil)

				return mockCommand
			},
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(3)
				return mockDevices
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:  assert.Error,
			assertResult: assert.False,
		},
		"happy path": {
			publishInfo: models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetIQN: targetIQN,
					},
				},
			},
			portals: []string{portal1},
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return([]byte(iscsiadmNodeOutput), nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "-o", "update", "-n", "node.session.scan", "-v",
					"manual").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "-o", "update", "-n", "node.session.timeo.replacement_timeout",
					"-v", "5").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "--op=update", "--name", "node.conn[0].timeo.login_timeout",
					"--value=10").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN, "-p",
					fmt.Sprintf("%s:3260", portal1), "--op=update", "--name", "node.session.initial_login_retry_max",
					"--value=1").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"node", "-T", targetIQN, "-p", fmt.Sprintf("%s:3260", portal1), "--login").Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m",
					"session").Return([]byte(iscsiadmSessionOutput), nil)

				return mockCommand
			},
			getDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(3)
				return mockDevices
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:  assert.NoError,
			assertResult: assert.True,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			var devicesClient devices.Devices
			if params.getDevices != nil {
				devicesClient = params.getDevices(ctrl)
			}
			client := NewDetailed("", params.getCommandClient(ctrl), nil, nil, devicesClient, nil, nil, nil,
				afero.Afero{Fs: params.getFileSystemUtils()}, nil)
			successfullyLoggedIn, err := client.EnsureSessions(context.TODO(), &params.publishInfo, params.portals)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			if params.assertResult != nil {
				params.assertResult(t, successfullyLoggedIn)
			}
		})
	}
}

func TestClient_LoginTarget(t *testing.T) {
	type parameters struct {
		publishInfo models.VolumePublishInfo
		portal      string

		getCommandClient func(controller *gomock.Controller) tridentexec.Command
		getDeviceClient  func(controller *gomock.Controller) devices.Devices
		assertError      assert.ErrorAssertionFunc
	}

	const targetIQN = "iqn.2010-01.com.netapp:target-1"
	const portal = "127.0.0.1"

	tests := map[string]parameters{
		"error setting login timeout": {
			publishInfo: models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetIQN: targetIQN,
					},
				},
			},
			portal: portal,
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN,
					"-p", fmt.Sprintf("%s:3260", portal), "--op=update", "--name",
					"node.conn[0].timeo.login_timeout", "--value=10").
					Return(nil, errors.New("some error"))
				return mockCommand
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(context.TODO())
				return mockDevices
			},
			assertError: assert.Error,
		},
		"error setting login max retry": {
			publishInfo: models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetIQN: targetIQN,
					},
				},
			},
			portal: portal,
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN,
					"-p", fmt.Sprintf("%s:3260", portal), "--op=update", "--name",
					"node.conn[0].timeo.login_timeout", "--value=10").
					Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN,
					"-p", fmt.Sprintf("%s:3260", portal), "--op=update", "--name",
					"node.session.initial_login_retry_max", "--value=1").
					Return(nil, errors.New("some error"))
				return mockCommand
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(context.TODO())
				return mockDevices
			},
			assertError: assert.Error,
		},
		"error logging in": {
			publishInfo: models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetIQN: targetIQN,
					},
				},
			},
			portal: portal,
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN,
					"-p", fmt.Sprintf("%s:3260", portal), "--op=update", "--name",
					"node.conn[0].timeo.login_timeout", "--value=10").
					Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN,
					"-p", fmt.Sprintf("%s:3260", portal), "--op=update", "--name",
					"node.session.initial_login_retry_max", "--value=1").
					Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"node", "-T",
					targetIQN,
					"-p", fmt.Sprintf("%s:3260", portal), "--login").
					Return(nil, errors.New("some error"))
				return mockCommand
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(context.TODO())
				return mockDevices
			},
			assertError: assert.Error,
		},
		"happy path": {
			publishInfo: models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetIQN: targetIQN,
					},
				},
			},
			portal: portal,
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN,
					"-p", fmt.Sprintf("%s:3260", portal), "--op=update", "--name",
					"node.conn[0].timeo.login_timeout", "--value=10").
					Return(nil, nil)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node", "-T", targetIQN,
					"-p", fmt.Sprintf("%s:3260", portal), "--op=update", "--name",
					"node.session.initial_login_retry_max", "--value=1").
					Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"node", "-T",
					targetIQN,
					"-p", fmt.Sprintf("%s:3260", portal), "--login").
					Return(nil, nil)
				return mockCommand
			},
			getDeviceClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)
				return mockDevices
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			var deviceClient devices.Devices
			if params.getDeviceClient != nil {
				deviceClient = params.getDeviceClient(ctrl)
			}
			iscsiClient := NewDetailed("", params.getCommandClient(ctrl), nil, nil, deviceClient, nil, nil, nil,
				afero.Afero{Fs: afero.NewMemMapFs()}, nil)

			err := iscsiClient.LoginTarget(context.TODO(), &params.publishInfo, params.portal)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestClient_ensureTarget(t *testing.T) {
	type parameters struct {
		username              string
		password              string
		targetUsername        string
		targetInitiatorSecret string
		getCommandClient      func(controller *gomock.Controller) tridentexec.Command
		assertError           assert.ErrorAssertionFunc
	}

	const targetPortal = "127.0.0.1:3260"
	const portal2 = "127.0.0.2:3260"
	const targetIQN = "iqn.2010-01.com.netapp:target-1"
	const networkInterface = "default"
	const iscsiadmNodeOutput = targetPortal + `,1042 ` + targetIQN + `
` + portal2 + `,1043 ` + targetIQN + `
`
	const iscsiadmDiscoveryDBSendTargetsOutput = targetPortal + `,1042 ` + targetIQN + `
` + portal2 + `,1043 ` + targetIQN + `
`

	tests := map[string]parameters{
		"error getting targets": {
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return(nil,
					errors.New("some error"))
				return mockCommand
			},
			assertError: assert.Error,
		},
		"already logged in": {
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").
					Return([]byte(iscsiadmNodeOutput), nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"failure to discover any targets": {
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-D").Return(nil, nil)
				return mockCommand
			},
			assertError: assert.Error,
		},
		"error setting auth method to chap": {
			username:              "username",
			password:              "password",
			targetUsername:        "targetUsername",
			targetInitiatorSecret: "targetInitiatorSecret",
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "new").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.authmethod", "-v", "CHAP").Return(nil, errors.New("some error"))
				return mockCommand
			},
		},
		"error setting auth username for chap": {
			username:              "username",
			password:              "password",
			targetUsername:        "targetUsername",
			targetInitiatorSecret: "targetInitiatorSecret",
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "new").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.authmethod", "-v", "CHAP").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.username", "-v", "username").Return(nil, errors.New("some error"))
				return mockCommand
			},
		},
		"error setting auth password for chap": {
			username:              "username",
			password:              "password",
			targetUsername:        "targetUsername",
			targetInitiatorSecret: "targetInitiatorSecret",
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "new").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.authmethod", "-v", "CHAP").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.username", "-v", "username").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.password", "-v", "password").Return(nil, errors.New("some error"))
				return mockCommand
			},
		},
		"error setting target auth username for chap": {
			username:              "username",
			password:              "password",
			targetUsername:        "targetUsername",
			targetInitiatorSecret: "targetInitiatorSecret",
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "new").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.authmethod", "-v", "CHAP").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.username", "-v", "username").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.password", "-v", "password").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.username_in", "-v", "targetUsername").Return(nil, errors.New("some error"))
				return mockCommand
			},
		},
		"error setting target auth initiatior secret for chap": {
			username:              "username",
			password:              "password",
			targetUsername:        "targetUsername",
			targetInitiatorSecret: "targetInitiatorSecret",
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "new").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.authmethod", "-v", "CHAP").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.username", "-v", "username").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.password", "-v", "password").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.username_in", "-v", "targetUsername").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-o", "update", "-n",
					"discovery.sendtargets.auth.password_in", "-v", "targetInitiatorSecret").Return(nil, errors.New("some error"))
				return mockCommand
			},
		},
		"error getting portal discovery information": {
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true,
					"-m", "discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-D").
					Return(nil, nil)
				return mockCommand
			},
			assertError: assert.Error,
		},
		"happy path": {
			getCommandClient: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(context.TODO(), "iscsiadm", "-m", "node").Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "iscsiadm", iscsiadmLoginTimeout, true, "-m",
					"discoverydb", "-t", "st", "-p", targetPortal, "-I", networkInterface, "-D").Return([]byte(iscsiadmDiscoveryDBSendTargetsOutput), nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewDetailed("", params.getCommandClient(ctrl), nil, nil, nil, nil, nil, nil, afero.Afero{}, nil)
			err := client.ensureTarget(context.TODO(), targetPortal, targetIQN, params.username,
				params.password, params.targetUsername, params.targetInitiatorSecret, networkInterface)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestClient_multipathdIsRunning(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockExec := mockexec.NewMockCommand(mockCtrl)
	tests := []struct {
		name          string
		execOut       string
		execErr       error
		returnCode    int
		expectedValue bool
	}{
		{name: "True", execOut: "1234", execErr: nil, expectedValue: true},
		{name: "False", execOut: "", execErr: nil, expectedValue: false},
		{name: "Error", execOut: "1234", execErr: errors.New("cmd error"), expectedValue: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// Setup mock calls.
			mockExec.EXPECT().Execute(
				ctx, gomock.Any(), gomock.Any(),
			).Return([]byte(tt.execOut), tt.execErr)

			// Only mock out the second call if the expected value isn't true.
			if !tt.expectedValue {
				mockExec.EXPECT().Execute(
					ctx, gomock.Any(), gomock.Any(), gomock.Any(),
				).Return([]byte(tt.execOut), tt.execErr)
			}
			iscsiClient := NewDetailed("", mockExec, nil, nil, nil, nil, nil, nil, afero.Afero{}, nil)

			actualValue := iscsiClient.multipathdIsRunning(context.Background())
			assert.Equal(t, tt.expectedValue, actualValue)
		})
	}
}

func TestFilterTargets(t *testing.T) {
	type FilterCase struct {
		CommandOutput string
		InputPortal   string
		OutputIQNs    []string
	}
	tests := []FilterCase{
		{
			// Simple positive test, expect first
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"203.0.113.3:3260,-1 iqn.2010-01.com.solidfire:baz\n",
			InputPortal: "203.0.113.1:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:foo"},
		},
		{
			// Simple positive test, expect second
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:bar"},
		},
		{
			// Expect empty list
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n",
			InputPortal: "203.0.113.3:3260",
			OutputIQNs:  []string{},
		},
		{
			// Expect multiple
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:baz\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:bar", "iqn.1992-08.com.netapp:baz"},
		},
		{
			// Bad input
			CommandOutput: "" +
				"Foobar\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{},
		},
		{
			// Good and bad input
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"Foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"Bar\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:baz\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:bar", "iqn.1992-08.com.netapp:baz"},
		},
		{
			// Try nonstandard port number
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"203.0.113.2:3261,1025 iqn.1992-08.com.netapp:baz\n",
			InputPortal: "203.0.113.2:3261",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:baz"},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputPortal, func(t *testing.T) {
			targets := filterTargets(testCase.CommandOutput, testCase.InputPortal)
			assert.Equal(t, testCase.OutputIQNs, targets, "Wrong targets returned")
		})
	}
}

func TestFormatPortal(t *testing.T) {
	type IPAddresses struct {
		InputPortal  string
		OutputPortal string
	}
	tests := []IPAddresses{
		{
			InputPortal:  "203.0.113.1",
			OutputPortal: "203.0.113.1:3260",
		},
		{
			InputPortal:  "203.0.113.1:3260",
			OutputPortal: "203.0.113.1:3260",
		},
		{
			InputPortal:  "203.0.113.1:3261",
			OutputPortal: "203.0.113.1:3261",
		},
		{
			InputPortal:  "[2001:db8::1]",
			OutputPortal: "[2001:db8::1]:3260",
		},
		{
			InputPortal:  "[2001:db8::1]:3260",
			OutputPortal: "[2001:db8::1]:3260",
		},
		{
			InputPortal:  "[2001:db8::1]:3261",
			OutputPortal: "[2001:db8::1]:3261",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputPortal, func(t *testing.T) {
			assert.Equal(t, testCase.OutputPortal, formatPortal(testCase.InputPortal), "Portal not correctly formatted")
		})
	}
}

func TestPidRunningOrIdleRegex(t *testing.T) {
	type parameters struct {
		input          string
		expectedOutput bool
	}
	tests := map[string]parameters{
		// Negative tests
		"Negative input #1": {
			input:          "",
			expectedOutput: false,
		},
		"Negative input #2": {
			input:          "pid -5 running",
			expectedOutput: false,
		},
		"Negative input #3": {
			input:          "pid running",
			expectedOutput: false,
		},
		// Positive tests
		"Positive input #1": {
			input:          "pid 5 running",
			expectedOutput: true,
		},
		// Positive tests
		"Positive input #2": {
			input:          "pid 2509 idle",
			expectedOutput: true,
		},
	}
	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			result := pidRunningOrIdleRegex.MatchString(params.input)
			assert.Equal(t, params.expectedOutput, result)
		})
	}
}

func TestGetFindMultipathValue(t *testing.T) {
	type parameters struct {
		findMultipathsValue         string
		findMultipathsLineCommented bool
		expectedFindMultipathsValue string
	}

	tests := map[string]parameters{
		"find_multipaths no": {
			findMultipathsValue:         "no",
			findMultipathsLineCommented: false,
			expectedFindMultipathsValue: "no",
		},
		"find_multipaths line commented": {
			findMultipathsValue:         "no",
			findMultipathsLineCommented: true,
			expectedFindMultipathsValue: "",
		},
		"empty find_multipaths": {
			findMultipathsValue:         "",
			findMultipathsLineCommented: false,
			expectedFindMultipathsValue: "",
		},
		"find_multipaths yes": {
			findMultipathsValue:         "yes",
			findMultipathsLineCommented: false,
			expectedFindMultipathsValue: "yes",
		},
		"find_multipaths 'yes'": {
			findMultipathsValue:         "'yes'",
			findMultipathsLineCommented: false,
			expectedFindMultipathsValue: "yes",
		},
		"find_multipaths 'on'": {
			findMultipathsValue:         "'on'",
			findMultipathsLineCommented: false,
			expectedFindMultipathsValue: "yes",
		},
		"find_multipaths 'off'": {
			findMultipathsValue:         "'off'",
			findMultipathsLineCommented: false,
			expectedFindMultipathsValue: "no",
		},
		"find_multipaths on": {
			findMultipathsValue:         "on",
			findMultipathsLineCommented: false,
			expectedFindMultipathsValue: "yes",
		},
		"find_multipaths off": {
			findMultipathsValue:         "off",
			findMultipathsLineCommented: false,
			expectedFindMultipathsValue: "no",
		},
		"find_multipaths random": {
			findMultipathsValue:         "random",
			findMultipathsLineCommented: false,
			expectedFindMultipathsValue: "random",
		},
		"find_multipaths smart": {
			findMultipathsValue:         "smart",
			findMultipathsLineCommented: false,
			expectedFindMultipathsValue: "smart",
		},
		"find_multipaths greedy": {
			findMultipathsValue:         "greedy",
			findMultipathsLineCommented: false,
			expectedFindMultipathsValue: "greedy",
		},
		"find_multipaths 'no'": {
			findMultipathsValue:         "'no'",
			findMultipathsLineCommented: false,
			expectedFindMultipathsValue: "no",
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			mpathConfig := multipathConfig(params.findMultipathsValue, params.findMultipathsLineCommented)

			findMultipathsValue := getFindMultipathValue(mpathConfig)
			assert.Equal(t, params.expectedFindMultipathsValue, findMultipathsValue)
		})
	}
}

func TestEnsureHostportFormatted(t *testing.T) {
	type IPAddresses struct {
		InputIP  string
		OutputIP string
	}
	tests := []IPAddresses{
		{
			InputIP:  "1.2.3.4:5678",
			OutputIP: "1.2.3.4:5678",
		},
		{
			InputIP:  "1.2.3.4",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "[1:2:3:4]:5678",
			OutputIP: "[1:2:3:4]:5678",
		},
		{
			InputIP:  "[1:2:3:4]",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "1:2:3:4",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "2607:f8b0:4006:818:0:0:0:2004",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]:5678",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]:5678",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputIP, func(t *testing.T) {
			assert.Equal(t, testCase.OutputIP, network.EnsureHostportFormatted(testCase.InputIP),
				"Hostport not correctly formatted")
		})
	}
}

func TestLoginTarget(t *testing.T) {
	tests := map[string]struct {
		publishInfo    *models.VolumePublishInfo
		getMockDevices func(controller *gomock.Controller) devices.Devices
		getMockCommand func(controller *gomock.Controller) tridentexec.Command
		expectErr      bool
	}{
		"Login success CHAP": {
			publishInfo: &mockPublushInfo,
			getMockDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(gomock.Any()).Times(2)
				return mockDevices
			},
			getMockCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "iscsiadm", "-m", "node", "-T",
					"iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33111:vs.2", "-p", "1.2.3.4:9600", "--op=update", "--name",
					"node.session.auth.authmethod", "--value=CHAP")
				mockCommand.EXPECT().ExecuteRedacted(gomock.Any(), "iscsiadm", []string{
					"-m", "node", "-T",
					"iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33111:vs.2", "-p", "1.2.3.4:9600", "--op=update", "--name",
					"node.session.auth.username", "--value=testUser",
				}, gomock.Any())
				mockCommand.EXPECT().ExecuteRedacted(gomock.Any(), "iscsiadm", []string{
					"-m", "node", "-T",
					"iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33111:vs.2", "-p", "1.2.3.4:9600", "--op=update", "--name",
					"node.session.auth.password", "--value=testSecret",
				}, gomock.Any())
				mockCommand.EXPECT().ExecuteRedacted(gomock.Any(), "iscsiadm", []string{
					"-m", "node", "-T",
					"iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33111:vs.2", "-p", "1.2.3.4:9600", "--op=update", "--name",
					"node.session.auth.username_in", "--value=targetUser",
				}, gomock.Any())
				mockCommand.EXPECT().ExecuteRedacted(gomock.Any(), "iscsiadm", []string{
					"-m", "node", "-T",
					"iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33111:vs.2", "-p", "1.2.3.4:9600", "--op=update", "--name",
					"node.session.auth.password_in", "--value=targetSecret",
				}, gomock.Any())
				mockCommand.EXPECT().Execute(gomock.Any(), "iscsiadm", "-m", "node", "-T",
					"iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33111:vs.2", "-p", "1.2.3.4:9600", "--op=update", "--name",
					"node.conn[0].timeo.login_timeout", "--value=10")
				mockCommand.EXPECT().Execute(gomock.Any(), "iscsiadm", "-m", "node", "-T",
					"iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33111:vs.2", "-p", "1.2.3.4:9600", "--op=update", "--name",
					"node.session.initial_login_retry_max", "--value=1")
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "iscsiadm", 10*time.Second, true, "-m", "node",
					"-T", "iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33111:vs.2", "-p", "1.2.3.4:9600", "--login")
				return mockCommand
			},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewDetailed("/host", params.getMockCommand(ctrl), nil, nil, params.getMockDevices(ctrl), nil,
				nil, nil, afero.Afero{Fs: afero.NewMemMapFs()}, nil)
			err := client.LoginTarget(context.Background(), params.publishInfo, "1.2.3.4:9600")
			if params.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSafeToLogOut(t *testing.T) {
	tests := map[string]struct {
		hostNum    int
		sessionNum int
		expected   bool
		getOsFs    func() afero.Afero
	}{
		"Unsafe to logout": {
			hostNum:    1,
			sessionNum: 2,
			getOsFs: func() afero.Afero {
				fs := afero.NewMemMapFs()
				_ = fs.MkdirAll("/sys/class/iscsi_host/host1/device/session2/target1:0:0/6:0:0:0", 0o755)
				return afero.Afero{Fs: fs}
			},
			expected: false,
		},
		"Safe to logout": {
			hostNum:    1,
			sessionNum: 2,
			getOsFs: func() afero.Afero {
				fs := afero.NewMemMapFs()
				return afero.Afero{Fs: fs}
			},
			expected: true,
		},
		"Safe to logout, no target dirs": {
			hostNum:    1,
			sessionNum: 2,
			getOsFs: func() afero.Afero {
				fs := afero.NewMemMapFs()
				_ = fs.MkdirAll("/sys/class/iscsi_host/host1/device/session2/target1:0:0", 0o755)
				return afero.Afero{Fs: fs}
			},
			expected: true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			client := NewDetailed("/host", nil, nil, nil, nil, nil,
				nil, nil, params.getOsFs(), nil)
			isSafe := client.SafeToLogOut(context.Background(), params.hostNum, params.sessionNum)
			assert.Equal(t, params.expected, isSafe)
		})
	}
}

func TestRemovePortalsFromSession(t *testing.T) {
	tests := map[string]struct {
		publishInfo *models.VolumePublishInfo
		sessions    *models.ISCSISessions
	}{
		"Successful Remove": {
			publishInfo: &models.VolumePublishInfo{
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiPortals:      []string{"192.168.1.100:3260"},
						IscsiTargetPortal: "192.168.1.100:3260",
					},
				},
			},
			sessions: &models.ISCSISessions{
				Info: map[string]*models.ISCSISessionData{
					"192.168.1.100": {
						PortalInfo:  models.PortalInfo{},
						LUNs:        models.LUNs{},
						Remediation: models.Scan,
					},
				},
			},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, 1, len(params.sessions.Info))
			client := NewDetailed("/host", nil, nil, nil, nil, nil,
				nil, nil, afero.Afero{Fs: afero.NewMemMapFs()}, nil)
			client.RemovePortalsFromSession(context.Background(), params.publishInfo, params.sessions)
			assert.Equal(t, 0, len(params.sessions.Info))
		})
	}
}

func TestLogout(t *testing.T) {
	tests := map[string]struct {
		getMockDevices func(controller *gomock.Controller) devices.Devices
		getMockCommand func(controller *gomock.Controller) tridentexec.Command
		expectError    bool
	}{
		"Successful logout": {
			getMockDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(gomock.Any()).Times(2)
				return mockDevices
			},
			getMockCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "iscsiadm", "-m", "node", "-T", gomock.Any(), "--portal",
					gomock.Any(), "-u").Return(nil, nil)
				return mockCommand
			},
			expectError: false,
		},
		"Logout error": {
			getMockDevices: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(gomock.Any()).Times(2)
				return mockDevices
			},
			getMockCommand: func(controller *gomock.Controller) tridentexec.Command {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().Execute(gomock.Any(), "iscsiadm", "-m", "node", "-T", gomock.Any(), "--portal",
					gomock.Any(), "-u").Return(nil, errors.New("error"))
				return mockCommand
			},
			expectError: false,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			client := NewDetailed("/host", params.getMockCommand(mockCtrl), nil, nil, params.getMockDevices(mockCtrl), nil,
				nil, nil,
				afero.Afero{Fs: afero.NewMemMapFs()}, nil)
			err := client.Logout(context.Background(), "iqn.2023-10.com.example:target", "192.168.1.100:3260")

			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPrepareDeviceForRemoval(t *testing.T) {
	tests := map[string]struct {
		getDevicesClient  func(controller *gomock.Controller) devices.Devices
		deviceInfo        *models.ScsiDeviceInfo
		publishInfo       *models.VolumePublishInfo
		allPublishInfos   []models.VolumePublishInfo
		ignoreErrors      bool
		force             bool
		expectedMultipath string
		expectedError     bool
	}{
		"Successful removal": {
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().VerifyMultipathDevice(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any()).Return(true, nil)
				mockDevices.EXPECT().ListAllDevices(gomock.Any()).Times(2)
				mockDevices.EXPECT().MultipathFlushDevice(gomock.Any(), gomock.Any()).Return(nil)
				mockDevices.EXPECT().FlushDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockDevices.EXPECT().RemoveDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockDevices.EXPECT().WaitForDevicesRemoval(gomock.Any(), DevPrefix, gomock.Any(),
					devicesRemovalMaxWaitTime).Return(nil)
				return mockDevices
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
				Devices:         []string{"sda", "sdb"},
			},
			publishInfo:       &mockPublushInfo,
			allPublishInfos:   []models.VolumePublishInfo{},
			ignoreErrors:      false,
			force:             false,
			expectedMultipath: "",
			expectedError:     false,
		},
		"Deferred device removal": {
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().VerifyMultipathDevice(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any()).Return(true, nil)
				mockDevices.EXPECT().ListAllDevices(gomock.Any()).Times(2)
				mockDevices.EXPECT().RemoveDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockDevices.EXPECT().WaitForDevicesRemoval(gomock.Any(), DevPrefix, gomock.Any(),
					devicesRemovalMaxWaitTime).Return(nil)
				return mockDevices
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
				Devices:         []string{"sda", "sdb"},
			},
			publishInfo:       &mockPublushInfo,
			allPublishInfos:   []models.VolumePublishInfo{},
			ignoreErrors:      false,
			force:             true,
			expectedMultipath: "/dev/dm-0",
			expectedError:     false,
		},
		"Verify mPath device error": {
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().VerifyMultipathDevice(gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any()).Return(true, errors.New("error"))
				return mockDevices
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
				Devices:         []string{"sda", "sdb"},
			},
			publishInfo:       &mockPublushInfo,
			allPublishInfos:   []models.VolumePublishInfo{},
			ignoreErrors:      false,
			force:             false,
			expectedMultipath: "",
			expectedError:     true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			client := NewDetailed("/host", nil, nil, nil, params.getDevicesClient(mockCtrl), nil, nil, nil,
				afero.Afero{Fs: afero.NewMemMapFs()}, nil)

			multipath, err := client.PrepareDeviceForRemoval(
				context.TODO(),
				params.deviceInfo,
				params.publishInfo,
				params.allPublishInfos,
				params.ignoreErrors,
				params.force,
			)

			if params.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, params.expectedMultipath, multipath)
		})
	}
}

func TestRemoveSCSIDevice(t *testing.T) {
	tests := map[string]struct {
		getDevicesClient func(controller *gomock.Controller) devices.Devices
		deviceInfo       *models.ScsiDeviceInfo
		ignoreErrors     bool
		skipFlush        bool
		expectedReturn   bool
		expectedError    bool
	}{
		"Timeout error": {
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(gomock.Any()).Times(2)
				mockDevices.EXPECT().MultipathFlushDevice(gomock.Any(), gomock.Any()).Return(errors.TimeoutError("timeout"))
				mockDevices.EXPECT().FlushDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockDevices.EXPECT().RemoveDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockDevices.EXPECT().WaitForDevicesRemoval(gomock.Any(), DevPrefix, gomock.Any(),
					devicesRemovalMaxWaitTime).Return(nil)
				return mockDevices
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
				Devices:         []string{"sda", "sdb"},
			},
			ignoreErrors:   false,
			expectedReturn: true,
			expectedError:  false,
		},
		"Multipath flush error": {
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(gomock.Any())
				mockDevices.EXPECT().MultipathFlushDevice(gomock.Any(), gomock.Any()).Return(errors.New("error"))
				return mockDevices
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
				Devices:         []string{"sda", "sdb"},
			},
			ignoreErrors:   false,
			expectedReturn: false,
			expectedError:  true,
		},
		"Device flush error": {
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(gomock.Any())
				mockDevices.EXPECT().MultipathFlushDevice(gomock.Any(), gomock.Any()).Return(nil)
				mockDevices.EXPECT().FlushDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("error"))
				return mockDevices
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
				Devices:         []string{"sda", "sdb"},
			},
			ignoreErrors:   false,
			expectedReturn: false,
			expectedError:  true,
		},
		"Remove error": {
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(gomock.Any())
				mockDevices.EXPECT().MultipathFlushDevice(gomock.Any(), gomock.Any())
				mockDevices.EXPECT().FlushDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockDevices.EXPECT().RemoveDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(errors.New("error"))
				return mockDevices
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
				Devices:         []string{"sda", "sdb"},
			},
			ignoreErrors:   false,
			expectedReturn: false,
			expectedError:  true,
		},
		"Wait for removal error": {
			getDevicesClient: func(controller *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(controller)
				mockDevices.EXPECT().ListAllDevices(gomock.Any())
				mockDevices.EXPECT().MultipathFlushDevice(gomock.Any(), gomock.Any())
				mockDevices.EXPECT().FlushDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
				mockDevices.EXPECT().RemoveDevice(gomock.Any(), gomock.Any(), gomock.Any())
				mockDevices.EXPECT().WaitForDevicesRemoval(gomock.Any(), DevPrefix, gomock.Any(),
					devicesRemovalMaxWaitTime).Return(errors.New("error"))
				return mockDevices
			},
			deviceInfo: &models.ScsiDeviceInfo{
				MultipathDevice: "dm-0",
				Devices:         []string{"sda", "sdb"},
			},
			ignoreErrors:   false,
			expectedReturn: false,
			expectedError:  true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			client := NewDetailed("/host", nil, nil, nil, params.getDevicesClient(mockCtrl), nil, nil, nil,
				afero.Afero{Fs: afero.NewMemMapFs()}, nil)

			ignoreErrors, err := client.removeSCSIDevice(context.TODO(), params.deviceInfo, params.ignoreErrors, params.skipFlush)

			if params.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, params.expectedReturn, ignoreErrors)
		})
	}
}

func TestRemoveLUNFromSessions(t *testing.T) {
	tests := map[string]struct {
		publishInfo *models.VolumePublishInfo
		sessions    *models.ISCSISessions
		beforeLen   int
		afterLen    int
	}{
		"LUN removed sucessfully": {
			sessions: &models.ISCSISessions{
				Info: map[string]*models.ISCSISessionData{
					"192.168.1.100": {
						PortalInfo: models.PortalInfo{},
						LUNs: models.LUNs{
							Info: map[int32]string{
								0: "iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.25",
							},
						},
						Remediation: models.Scan,
					},
				},
			},
			publishInfo: &models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				VolumeAccessInfo: models.VolumeAccessInfo{
					IscsiAccessInfo: models.IscsiAccessInfo{
						IscsiTargetPortal: "192.168.1.100",
						IscsiPortals:      []string{""},
						IscsiTargetIQN:    "iqn.2016-04.com.open-iscsi:ef9f41e2ffa7:vs.25",
					},
				},
			},
			beforeLen: 1,
			afterLen:  0,
		},
		"No sessions": {
			sessions: &models.ISCSISessions{},
			publishInfo: &models.VolumePublishInfo{
				FilesystemType:   filesystem.Ext4,
				VolumeAccessInfo: mockPublushInfo.VolumeAccessInfo,
			},
			beforeLen: 0,
			afterLen:  0,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, params.beforeLen, len(params.sessions.Info))
			client := NewDetailed("/host", nil, nil, nil, nil, nil, nil, nil,
				afero.Afero{Fs: afero.NewMemMapFs()}, nil)
			client.RemoveLUNFromSessions(context.Background(), params.publishInfo, params.sessions)
			assert.Equal(t, params.afterLen, len(params.sessions.Info))
		})
	}
}

func TestTargetHasMountedDevice(t *testing.T) {
	tests := map[string]struct {
		getMockFs         func() (afero.Afero, error)
		targetIQN         string
		getMockIscsiUtils func(controller *gomock.Controller) IscsiReconcileUtils
		getMountClient    func(controller *gomock.Controller) mount.Mount
		getMockOsUtils    func(controller *gomock.Controller) osutils.Utils
		expectedResult    bool
		expectedError     bool
	}{
		"Target has mounted device": {
			getMockFs: func() (afero.Afero, error) {
				fs := afero.NewMemMapFs()
				err := createMockIscsiSessions(fs)
				if err != nil {
					return afero.Afero{Fs: fs}, err
				}
				return afero.Afero{Fs: fs}, nil
			},
			getMockIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				iscsiClient := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				iscsiClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{1: 2})
				iscsiClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{3: 4})
				return iscsiClient
			},
			getMockOsUtils: func(controller *gomock.Controller) osutils.Utils {
				mockUtils := mock_osutils.NewMockUtils(controller)
				mockUtils.EXPECT().EvalSymlinks(gomock.Any()).Return("/dev/dm-0", nil)
				mockUtils.EXPECT().EvalSymlinks(gomock.Any()).Return("/dev/dm-1", nil)
				return mockUtils
			},
			getMountClient: func(controller *gomock.Controller) mount.Mount {
				client := mock_mount.NewMockMount(controller)
				client.EXPECT().ListProcMountinfo().Return([]models.MountInfo{
					{MountSource: "/dev/dm-0", MountPoint: "/pvc-123"},
					{MountSource: "/dev/dm-1", MountPoint: "/pvc-456"},
				}, nil)
				return client
			},
			targetIQN:      "iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33111:vs.2",
			expectedResult: true,
			expectedError:  false,
		},
		"Target device not mounted": {
			getMockFs: func() (afero.Afero, error) {
				fs := afero.NewMemMapFs()
				fs.MkdirAll(osutils.ChrootPathPrefix+"/sys/class/iscsi_session", 0o755)
				return afero.Afero{Fs: fs}, nil
			},
			getMockIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				iscsiClient := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				return iscsiClient
			},
			getMockOsUtils: func(controller *gomock.Controller) osutils.Utils {
				return mock_osutils.NewMockUtils(controller)
			},
			getMountClient: func(controller *gomock.Controller) mount.Mount {
				client := mock_mount.NewMockMount(controller)
				client.EXPECT().ListProcMountinfo().Return([]models.MountInfo{}, nil)
				return client
			},
			targetIQN:      "iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33333:vs.2",
			expectedResult: false,
			expectedError:  false,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			command := tridentexec.NewCommand()
			fs, err := params.getMockFs()
			assert.NoError(t, err)

			fsClient := filesystem.NewDetailed(command, fs, nil)
			deviceClient := devices.NewDetailed(command, fs, nil)
			iscsiUtils := params.getMockIscsiUtils(mockCtrl)

			client := NewDetailed(osutils.ChrootPathPrefix, nil, nil, osutils.NewDetailed(command, fs), deviceClient, fsClient,
				params.getMountClient(mockCtrl), iscsiUtils, afero.Afero{Fs: fs}, params.getMockOsUtils(mockCtrl))

			mounted, err := client.TargetHasMountedDevice(context.Background(), params.targetIQN)

			if params.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, params.expectedResult, mounted)
		})
	}
}

func TestGetMountedISCSIDevices(t *testing.T) {
	tests := map[string]struct {
		getMockFs         func() (afero.Afero, error)
		getMockIscsiUtils func(controller *gomock.Controller) IscsiReconcileUtils
		getMockOsUtils    func(controller *gomock.Controller) osutils.Utils
		getMountClient    func(controller *gomock.Controller) mount.Mount
		expectedResult    []*models.ScsiDeviceInfo
		expectedResultLen int
		expectedError     bool
	}{
		"Get devices happy path": {
			getMockFs: func() (afero.Afero, error) {
				fs := afero.NewMemMapFs()
				err := createMockIscsiSessions(fs)
				if err != nil {
					return afero.Afero{Fs: fs}, err
				}
				return afero.Afero{Fs: fs}, nil
			},
			getMockIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				iscsiClient := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				iscsiClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{1: 2})
				iscsiClient.EXPECT().GetISCSIHostSessionMapForTarget(gomock.Any(),
					gomock.Any()).Return(map[int]int{3: 4})
				return iscsiClient
			},
			getMockOsUtils: func(controller *gomock.Controller) osutils.Utils {
				mockUtils := mock_osutils.NewMockUtils(controller)
				mockUtils.EXPECT().EvalSymlinks(gomock.Any()).Return("/dev/dm-0", nil)
				mockUtils.EXPECT().EvalSymlinks(gomock.Any()).Return("/dev/dm-1", nil)
				return mockUtils
			},
			getMountClient: func(controller *gomock.Controller) mount.Mount {
				client := mock_mount.NewMockMount(controller)
				client.EXPECT().ListProcMountinfo().Return([]models.MountInfo{
					{MountSource: "/dev/dm-0", MountPoint: "/pvc-123"},
					{MountSource: "/dev/dm-1", MountPoint: "/pvc-456"},
				}, nil)
				return client
			},
			expectedResult: []*models.ScsiDeviceInfo{
				{
					ScsiDeviceAddress: models.ScsiDeviceAddress{
						Host: "6",
						LUN:  "1",
					},
					IQN:             "iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33111:vs.2",
					MultipathDevice: "dm-0",
					SessionNumber:   1,
				},
				{
					ScsiDeviceAddress: models.ScsiDeviceAddress{
						Host: "6",
						LUN:  "0",
					},
					IQN:             "iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33111:vs.2",
					MultipathDevice: "dm-1",
					SessionNumber:   1,
				},
			},
			expectedError:     false,
			expectedResultLen: 2,
		},
		"No mounted devices": {
			getMockFs: func() (afero.Afero, error) {
				fs := afero.NewMemMapFs()
				fs.MkdirAll("/sys/class/iscsi_session", 0o755)
				return afero.Afero{Fs: fs}, nil
			},
			getMockIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				iscsiClient := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				return iscsiClient
			},
			getMockOsUtils: func(controller *gomock.Controller) osutils.Utils {
				return mock_osutils.NewMockUtils(controller)
			},
			getMountClient: func(controller *gomock.Controller) mount.Mount {
				client := mock_mount.NewMockMount(controller)
				client.EXPECT().ListProcMountinfo().Return([]models.MountInfo{}, nil)
				return client
			},
			expectedResult:    []*models.ScsiDeviceInfo{},
			expectedError:     false,
			expectedResultLen: 0,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)

			command := tridentexec.NewCommand()
			fs, err := params.getMockFs()
			assert.NoError(t, err)

			fsClient := filesystem.NewDetailed(command, fs, nil)
			deviceClient := devices.NewDetailed(command, fs, nil)
			iscsiUtils := params.getMockIscsiUtils(mockCtrl)

			client := NewDetailed(osutils.ChrootPathPrefix, nil, nil, osutils.NewDetailed(command, fs), deviceClient, fsClient,
				params.getMountClient(mockCtrl), iscsiUtils, afero.Afero{Fs: fs}, params.getMockOsUtils(mockCtrl))

			results, err := client.GetMountedISCSIDevices(context.Background())

			if params.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, params.expectedResultLen, len(results))
			for i, result := range results {
				assert.Equal(t, params.expectedResult[i].IQN, result.IQN)
				assert.Equal(t, params.expectedResult[i].MultipathDevice, result.MultipathDevice)
				assert.Equal(t, params.expectedResult[i].Host, result.Host)
				assert.Equal(t, params.expectedResult[i].LUN, result.LUN)
				assert.Equal(t, params.expectedResult[i].SessionNumber, result.SessionNumber)
			}
		})
	}
}

func TestReconcileISCSIVolumeInfo(t *testing.T) {
	tests := map[string]struct {
		trackingInfo *models.VolumeTrackingInfo
		getOsFs      func() (afero.Afero, error)
		expectBool   bool
		shouldErr    bool
	}{
		"Successful using host session map": {
			trackingInfo: &models.VolumeTrackingInfo{
				VolumePublishInfo: mockPublushInfo,
			},
			getOsFs: func() (afero.Afero, error) {
				fs := afero.NewMemMapFs()
				err := createMockIscsiSessions(fs)
				return afero.Afero{Fs: fs}, err
			},
			expectBool: true,
			shouldErr:  false,
		},
		"Unsuccessful": {
			trackingInfo: &models.VolumeTrackingInfo{
				VolumePublishInfo: mockPublushInfo,
			},
			getOsFs: func() (afero.Afero, error) {
				fs := afero.NewMemMapFs()
				return afero.Afero{Fs: fs}, nil
			},
			expectBool: false,
			shouldErr:  false,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			// mockCtrl := gomock.NewController(t)
			osFs, err := params.getOsFs()
			assert.NoError(t, err)
			reconcileHelper := NewReconcileUtilsDetailed(osutils.ChrootPathPrefix, osutils.New(), osFs)
			reconciled, err := reconcileHelper.ReconcileISCSIVolumeInfo(context.Background(), params.trackingInfo)
			assert.Equal(t, params.expectBool, reconciled)
			if params.shouldErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetDevicesForLUN(t *testing.T) {
	prefix := osutils.ChrootPathPrefix
	tests := map[string]struct {
		paths   []string
		getOsFs func() (afero.Afero, error)
		expect  []string
	}{
		"Success": {
			getOsFs: func() (afero.Afero, error) {
				fs := afero.NewMemMapFs()
				fs.MkdirAll(prefix+"/sys/class/iscsi_session/session1/device/target6:0:0/6:0:0:1/block/", 0o755)
				fs.MkdirAll(prefix+"/sys/class/iscsi_session/session1/device/target6:0:0/6:0:0:1/block/sdb/holders/dm-0", 0o755)
				fs.MkdirAll(prefix+"/sys/class/iscsi_session/session1/device/target6:0:0/6:0:0:0/block/sdc/holders/dm-1", 0o755)
				return afero.Afero{Fs: fs}, nil
			},
			paths: []string{
				prefix + "/sys/class/iscsi_session/session1/device/doesnt-exist",
				prefix + "/sys/class/iscsi_session/session1/device/target6:0:0/6:0:0:1",
				prefix + "/sys/class/iscsi_session/session1/device/target6:0:0/6:0:0:0",
			},
			expect: []string{"sdb", "sdc"},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			osFs, err := params.getOsFs()
			assert.NoError(t, err)
			reconcileHelper := NewReconcileUtilsDetailed(osutils.ChrootPathPrefix, osutils.NewDetailed(nil, osFs), osFs)
			deviceNames, err := reconcileHelper.GetDevicesForLUN(params.paths)
			assert.Equal(t, params.expect, deviceNames)
		})
	}
}

func TestGetSysfsBlockDirsForLUN(t *testing.T) {
	prefix := osutils.ChrootPathPrefix
	tests := map[string]struct {
		lunID          int
		hostSessionMap map[int]int
		expect         []string
	}{
		"Happy Path": {
			lunID: 1,
			hostSessionMap: map[int]int{
				1: 2,
			},
			expect: []string{
				prefix + "/sys/class/scsi_host/host1/device/session2/iscsi_session/session2/device/target1:0:0/1:0:0:1",
			},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			reconcileHelper := NewReconcileUtils()
			dirs := reconcileHelper.GetSysfsBlockDirsForLUN(params.lunID, params.hostSessionMap)
			assert.Equal(t, params.expect, dirs)
		})
	}
}

// ---- helpers
func multipathConfig(findMultipathsValue string, ValueCommented bool) string {
	const multipathConf = `
defaults {
    user_friendly_names yes
%s}
`
	commentPrefix := ""
	if ValueCommented {
		commentPrefix = "#"
	}

	findMultipathLine := ""
	if findMultipathsValue != "" {
		findMultipathLine = fmt.Sprintf("    %sfind_multipaths %s\n", commentPrefix, findMultipathsValue)
	}

	cfg := fmt.Sprintf(multipathConf, findMultipathLine)
	return cfg
}

func vpdpg80SerialBytes(serial string) []byte {
	return append([]byte{0, 128, 0, 20}, []byte(serial)...)
}

type aferoWrapper struct {
	openFileError    error
	openFileResponse afero.File
	openResponse     afero.File
	openError        error
	afero.Fs
}

func (a *aferoWrapper) OpenFile(_ string, _ int, _ os.FileMode) (afero.File, error) {
	return a.openFileResponse, a.openFileError
}

func (a *aferoWrapper) Open(_ string) (afero.File, error) {
	return a.openResponse, a.openError
}

type aferoFileWrapper struct {
	WriteStringError error
	WriteStringCount int
	afero.File
}

func (a *aferoFileWrapper) WriteString(_ string) (ret int, err error) {
	return a.WriteStringCount, a.WriteStringError
}

func writeToFile(fs afero.Fs, path, content string) error {
	file, err := fs.Create(path)
	if err != nil {
		return fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Write to the file
	_, err = file.WriteString(content)
	if err != nil {
		return fmt.Errorf("failed to write to file: %v", err)
	}
	return nil
}

func createMockIscsiSessions(fs afero.Fs) error {
	prefix := osutils.ChrootPathPrefix
	dirs := []string{
		"/dev/sdb",
		prefix + "/sys/block/sdb/holders/dm-0",
		"/dev/sdc",
		prefix + "/sys/block/sdc/holders/dm-1",
		"/dev/sdd",
		prefix + "/sys/block/sdd/holders/dm-2",
		"/dev/sde",
		prefix + "/sys/block/sde/holders/dm-3",
		prefix + "/sys/class/iscsi_session/session1/device/target6:0:0/6:0:0:0/block/sdc/holders/dm-1",
		prefix + "/sys/class/iscsi_session/session1/device/target6:0:0/6:0:0:1/block/sdb/holders/dm-0",
		prefix + "/sys/class/iscsi_session/session2/device/target7:0:0/7:0:0:0/block/sde/holders/dm-3",
		prefix + "/sys/class/iscsi_session/session2/device/target7:0:0/7:0:0:1/block/sdd/holders/dm-2",
		prefix + "/sys/class/iscsi_host/host1/device/session1",
	}
	for _, dir := range dirs {
		err := fs.MkdirAll(dir, 0o755)
		if err != nil {
			return err
		}
	}

	// Create iscsi session files
	files := map[string]string{
		prefix + "/sys/class/iscsi_session/session1/targetname": "iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33111:vs.2",
		prefix + "/sys/class/iscsi_host/host1/device/session1/iscsi_session/session1/targetname": "iqn.1992-08.com.netapp:sn." +
			"a57de312358411ef8730005056b33111:vs.2",
		prefix + "/sys/class/iscsi_session/session2/targetname": "iqn.1992-08.com.netapp:sn.a57de312358411ef8730005056b33222:vs.2",
	}
	for file, content := range files {
		err := writeToFile(fs, file, content)
		if err != nil {
			return err
		}
	}

	return nil
}

func TestIsPortalAccessible(t *testing.T) {
	tests := []struct {
		name       string
		portal     string
		setupMock  func(*gomock.Controller) *mockexec.MockCommand
		assertErr  assert.ErrorAssertionFunc
		assertBool assert.BoolAssertionFunc
	}{
		{
			name:   "has successful discovery",
			portal: "127.0.0.1:3260",
			setupMock: func(ctrl *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "iscsiadm", iscsiadmAccessiblePortalTimeout, true, "-m", "discovery", "-t",
					"sendtargets", "-p", "127.0.0.1:3260",
				).Return([]byte(""), nil)
				return mockCommand
			},
			assertErr:  assert.NoError,
			assertBool: assert.True,
		},
		{
			name:   "has timeout running the command",
			portal: "127.0.0.1:3260",
			setupMock: func(ctrl *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "iscsiadm", iscsiadmAccessiblePortalTimeout, true, "-m", "discovery", "-t",
					"sendtargets", "-p", "127.0.0.1:3260",
				).Return(nil, errors.TimeoutError("timeout error"))
				return mockCommand
			},
			assertErr:  assert.Error,
			assertBool: assert.False,
		},
		{
			name:   "has iSCSI CHAP auth failure",
			portal: "127.0.0.1:3260",
			setupMock: func(ctrl *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "iscsiadm", iscsiadmAccessiblePortalTimeout, true, "-m", "discovery", "-t",
					"sendtargets", "-p", "127.0.0.1:3260",
				).Return(nil, mockexec.NewMockExitError(errLoginAuthFailed, "auth failed"))
				return mockCommand
			},
			assertErr:  assert.Error,
			assertBool: assert.True,
		},
		{
			name:   "has iSCSI PDU timeout",
			portal: "127.0.0.1:3260",
			setupMock: func(ctrl *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "iscsiadm", iscsiadmAccessiblePortalTimeout, true, "-m", "discovery", "-t",
					"sendtargets", "-p", "127.0.0.1:3260",
				).Return(nil, mockexec.NewMockExitError(errPDUTimeout, "iSCSI PDU timeout"))
				return mockCommand
			},
			assertErr:  assert.Error,
			assertBool: assert.False,
		},
		{
			name:   "has transport timeout error",
			portal: "127.0.0.1:3260",
			setupMock: func(ctrl *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "iscsiadm", iscsiadmAccessiblePortalTimeout, true, "-m", "discovery", "-t",
					"sendtargets", "-p", "127.0.0.1:3260",
				).Return(nil, mockexec.NewMockExitError(errTransportTimeout, "transport timeout"))
				return mockCommand
			},
			assertErr:  assert.Error,
			assertBool: assert.False,
		},
		{
			name:   "has transport error",
			portal: "127.0.0.1:3260",
			setupMock: func(ctrl *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "iscsiadm", iscsiadmAccessiblePortalTimeout, true, "-m", "discovery", "-t",
					"sendtargets", "-p", "127.0.0.1:3260",
				).Return(nil, mockexec.NewMockExitError(errTransport, "transport failure"))
				return mockCommand
			},
			assertErr:  assert.Error,
			assertBool: assert.False,
		},
		{
			name:   "has unrecognized exit error",
			portal: "127.0.0.1:3260",
			setupMock: func(ctrl *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "iscsiadm", iscsiadmAccessiblePortalTimeout, true, "-m", "discovery", "-t",
					"sendtargets", "-p", "127.0.0.1:3260",
				).Return(nil, mockexec.NewMockExitError(math.MaxInt64, "unrecognized exit error"))
				return mockCommand
			},
			assertErr:  assert.Error,
			assertBool: assert.False,
		},
		{
			name:   "has unrecognized error",
			portal: "127.0.0.1:3260",
			setupMock: func(ctrl *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "iscsiadm", iscsiadmAccessiblePortalTimeout, true, "-m", "discovery", "-t",
					"sendtargets", "-p", "127.0.0.1:3260",
				).Return(nil, errors.New("unrecognized error"))
				return mockCommand
			},
			assertErr:  assert.Error,
			assertBool: assert.False,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			client := &Client{}
			client.command = tt.setupMock(mockCtrl)
			isAccessible, err := client.IsPortalAccessible(context.Background(), tt.portal)
			tt.assertErr(t, err)
			tt.assertBool(t, isAccessible)
		})
	}
}

func TestIsStalePortal(t *testing.T) {
	ipList := []string{"1.2.3.4", "2.3.4.5", "3.4.5.6", "4.5.6.7"}

	iqnList := []string{"IQN1", "IQN2", "IQN3", "IQN4"}

	chapCredentials := []models.IscsiChapInfo{
		{
			UseCHAP: false,
		},
		{
			UseCHAP:              true,
			IscsiUsername:        "username1",
			IscsiInitiatorSecret: "secret1",
			IscsiTargetUsername:  "username2",
			IscsiTargetSecret:    "secret2",
		},
		{
			UseCHAP:              true,
			IscsiUsername:        "username11",
			IscsiInitiatorSecret: "secret11",
			IscsiTargetUsername:  "username22",
			IscsiTargetSecret:    "secret22",
		},
	}

	sessionData1 := models.ISCSISessionData{
		PortalInfo: models.PortalInfo{
			ISCSITargetIQN: iqnList[0],
			Credentials:    chapCredentials[2],
		},
	}

	sessionData2 := models.ISCSISessionData{
		PortalInfo: models.PortalInfo{
			ISCSITargetIQN: iqnList[1],
			Credentials:    chapCredentials[2],
		},
	}

	type PreRun func(publishedSessions, currentSessions *models.ISCSISessions, portal string)

	inputs := []struct {
		TestName           string
		PublishedPortals   *models.ISCSISessions
		CurrentPortals     *models.ISCSISessions
		SessionWaitTime    time.Duration
		TimeNow            time.Time
		Portal             string
		ResultAction       models.ISCSIAction
		SimulateConditions PreRun
	}{
		{
			TestName: "CHAP portal is newly identified as stale with up-to-date credentials",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    models.NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.FirstIdentifiedStaleAt = time.Time{}
			},
		},
		{
			TestName: "CHAP portal has not exceeded session wait time",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now().Add(2 * time.Second),
			Portal:          ipList[0],
			ResultAction:    models.NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.FirstIdentifiedStaleAt = time.Now()
			},
		},
		{
			TestName: "CHAP portal exceeds session wait time with outdated credentials",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now().Add(20 * time.Second),
			Portal:          ipList[0],
			ResultAction:    models.LogoutLoginScan,
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.FirstIdentifiedStaleAt = time.Now()
				publishedSessions.Info[portal].PortalInfo.Credentials = chapCredentials[1] // Replace credentials.
			},
		},
		{
			TestName: "non-CHAP portal is newly identified as stale",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    models.NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Credentials = chapCredentials[0]
				publishedSessions.Info[portal].PortalInfo.FirstIdentifiedStaleAt = time.Time{}
			},
		},
		{
			TestName: "non-CHAP portal is stale but has not exceeded stale wait time",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now().Add(2 * time.Second),
			Portal:          ipList[0],
			ResultAction:    models.NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Credentials = chapCredentials[0]
				publishedSessions.Info[portal].PortalInfo.FirstIdentifiedStaleAt = time.Now()
			},
		},
		{
			TestName: "non-CHAP portal exceeds stale wait time",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now().Add(20 * time.Second),
			Portal:          ipList[0],
			ResultAction:    models.LoginScan,
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Credentials = chapCredentials[0]
				publishedSessions.Info[portal].PortalInfo.FirstIdentifiedStaleAt = time.Now()
			},
		},
	}

	for _, input := range inputs {
		t.Run(input.TestName, func(t *testing.T) {
			portal := input.Portal

			input.SimulateConditions(input.PublishedPortals, input.CurrentPortals, portal)

			publishedPortalData, _ := input.PublishedPortals.Info[portal]
			currentPortalData, _ := input.CurrentPortals.Info[portal]

			publishedPortalInfo := publishedPortalData.PortalInfo
			currentPortalInfo := currentPortalData.PortalInfo

			const chrootPathPrefix = ""
			ctrl := gomock.NewController(t)
			iscsiClient := NewDetailed(
				chrootPathPrefix,
				mockexec.NewMockCommand(ctrl),
				DefaultSelfHealingExclusion,
				mock_iscsi.NewMockOS(ctrl),
				mock_devices.NewMockDevices(ctrl),
				mock_filesystem.NewMockFilesystem(ctrl),
				mock_mount.NewMockMount(ctrl),
				nil,
				afero.Afero{Fs: afero.NewMemMapFs()},
				nil,
			)

			action := iscsiClient.isStalePortal(
				context.TODO(), &publishedPortalInfo, &currentPortalInfo, input.SessionWaitTime, input.TimeNow, portal,
			)
			assert.Equal(t, input.ResultAction, action, "Remediation action mismatch")
		})
	}
}

func TestIsNonStalePortal(t *testing.T) {
	lunList1 := map[int32]string{
		1: "volID-1",
		2: "volID-2",
		3: "volID-3",
	}

	lunList2 := map[int32]string{
		2: "volID-2",
		3: "volID-3",
		4: "volID-4",
	}

	ipList := []string{"1.2.3.4", "2.3.4.5", "3.4.5.6", "4.5.6.7"}

	iqnList := []string{"IQN1", "IQN2", "IQN3", "IQN4"}

	chapCredentials := []models.IscsiChapInfo{
		{
			UseCHAP: false,
		},
		{
			UseCHAP:              true,
			IscsiUsername:        "username1",
			IscsiInitiatorSecret: "secret1",
			IscsiTargetUsername:  "username2",
			IscsiTargetSecret:    "secret2",
		},
		{
			UseCHAP:              true,
			IscsiUsername:        "username11",
			IscsiInitiatorSecret: "secret11",
			IscsiTargetUsername:  "username22",
			IscsiTargetSecret:    "secret22",
		},
	}

	sessionData1 := models.ISCSISessionData{
		PortalInfo: models.PortalInfo{
			ISCSITargetIQN: iqnList[0],
			Credentials:    chapCredentials[2],
		},
		LUNs: models.LUNs{
			Info: mapCopyHelper(lunList1),
		},
	}

	sessionData2 := models.ISCSISessionData{
		PortalInfo: models.PortalInfo{
			ISCSITargetIQN: iqnList[1],
			Credentials:    chapCredentials[2],
		},
		LUNs: models.LUNs{
			Info: mapCopyHelper(lunList2),
		},
	}

	type PreRun func(publishedSessions, currentSessions *models.ISCSISessions, portal string)

	inputs := []struct {
		TestName           string
		PublishedPortals   *models.ISCSISessions
		CurrentPortals     *models.ISCSISessions
		SessionWaitTime    time.Duration
		TimeNow            time.Time
		Portal             string
		ResultAction       models.ISCSIAction
		SimulateConditions PreRun
	}{
		{
			TestName: "Current Portal Missing All LUNs",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    models.Scan,
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions, portal string) {
				currentSessions.Info[portal].LUNs = models.LUNs{}
			},
		},
		{
			TestName: "Current Portal Missing One LUNs",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    models.Scan,
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions, portal string) {
				delete(currentSessions.Info[portal].LUNs.Info, 2)
			},
		},
		{
			TestName: "Published Portal Missing All LUNs",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    models.NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions, portal string) {
				publishedSessions.Info[portal].LUNs = models.LUNs{}
			},
		},
		{
			TestName: "CHAP notification, Published and Current portals have CHAP but mismatch",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    models.NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Source = SessionSourceNodeStage
				currentSessions.Info[portal].PortalInfo.Credentials = chapCredentials[1]
			},
		},
		{
			TestName: "CHAP notification, Published portals ha CHAP but not Current Portal",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    models.NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Source = SessionSourceNodeStage
				currentSessions.Info[portal].PortalInfo.Credentials = chapCredentials[0]
			},
		},
		{
			TestName: "CHAP notification, Current portals ha CHAP but not Published Portal",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			CurrentPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{
				ipList[0]: structCopyHelper(sessionData1),
				ipList[1]: structCopyHelper(sessionData2),
				ipList[2]: structCopyHelper(sessionData1),
			}},
			SessionWaitTime: 10 * time.Second,
			TimeNow:         time.Now(),
			Portal:          ipList[0],
			ResultAction:    models.NoAction,
			SimulateConditions: func(publishedSessions, currentSessions *models.ISCSISessions, portal string) {
				publishedSessions.Info[portal].PortalInfo.Source = SessionSourceNodeStage
				publishedSessions.Info[portal].PortalInfo.Credentials = chapCredentials[0]
			},
		},
	}

	for _, input := range inputs {
		t.Run(input.TestName, func(t *testing.T) {
			portal := input.Portal

			input.SimulateConditions(input.PublishedPortals, input.CurrentPortals, portal)

			publishedPortalData, _ := input.PublishedPortals.Info[portal]
			currentPortalData, _ := input.CurrentPortals.Info[portal]

			action := isNonStalePortal(context.TODO(), publishedPortalData, currentPortalData, portal)

			assert.Equal(t, input.ResultAction, action, "Remediation action mismatch")
		})
	}
}

func TestSortPortals(t *testing.T) {
	ipList := []string{"1.2.3.4", "2.3.4.5", "3.4.5.6", "4.5.6.7", "5.6.7.8", "6.7.8.9", "7.8.9.10", "8.9.10.11"}

	sessionData := models.ISCSISessionData{
		PortalInfo: models.PortalInfo{
			ISCSITargetIQN: "IQN1",
		},
	}

	type PreRun func(publishedSessions *models.ISCSISessions, portal []string)

	inputs := []struct {
		TestName           string
		PublishedPortals   *models.ISCSISessions
		InputPortals       []string
		ResultPortals      []string
		SimulateConditions PreRun
	}{
		{
			TestName:         "Zero size list preserves Zero size list",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}},
			InputPortals:     []string{},
			ResultPortals:    []string{},
			SimulateConditions: func(publishedSessions *models.ISCSISessions, portals []string) {
				// Populate Published Portals
				for _, portal := range portals {
					publishedSessions.Info[portal] = structCopyHelper(sessionData)
				}
			},
		},
		{
			TestName:         "One size list preserves One size list",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}},
			InputPortals:     []string{ipList[0]},
			ResultPortals:    []string{ipList[0]},
			SimulateConditions: func(publishedSessions *models.ISCSISessions, portals []string) {
				// Populate Published Portals
				for _, portal := range portals {
					publishedSessions.Info[portal] = structCopyHelper(sessionData)
				}
			},
		},
		{
			TestName:         "Two size sorts on the basis of Access time",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}},
			InputPortals:     []string{ipList[0], ipList[4]},
			ResultPortals:    []string{ipList[4], ipList[0]},
			SimulateConditions: func(publishedSessions *models.ISCSISessions, portals []string) {
				// Populate Published Portals
				for idx := len(portals) - 1; idx >= 0; idx-- {
					publishedSessions.Info[portals[idx]] = structCopyHelper(sessionData)
					publishedSessions.Info[portals[idx]].PortalInfo.LastAccessTime = time.Now().Add(-time.Hour * time.
						Duration(idx))
				}
			},
		},
		{
			TestName:         "Same access time results in the same order",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}},
			InputPortals:     append([]string{}, ipList...),
			ResultPortals:    append([]string{}, ipList...),
			SimulateConditions: func(publishedSessions *models.ISCSISessions, portals []string) {
				// Populate Published Portals
				for _, portal := range portals {
					publishedSessions.Info[portal] = structCopyHelper(sessionData)
				}
			},
		},
		{
			TestName:         "Increasing access time results in the reverse order",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}},
			InputPortals:     append([]string{}, ipList...),
			ResultPortals:    reverseSlice(ipList),
			SimulateConditions: func(publishedSessions *models.ISCSISessions, portals []string) {
				// Populate Published Portals
				for idx := len(portals) - 1; idx >= 0; idx-- {
					publishedSessions.Info[portals[idx]] = structCopyHelper(sessionData)
					publishedSessions.Info[portals[idx]].PortalInfo.LastAccessTime = time.Now().Add(-time.Hour * time.
						Duration(idx))
				}
			},
		},
		{
			TestName:         "Increasing access time results in the reverse order with the exception of 3 items",
			PublishedPortals: &models.ISCSISessions{Info: map[string]*models.ISCSISessionData{}},
			InputPortals:     ipList,
			ResultPortals:    append(ipList[0:3], reverseSlice(ipList[3:])...),
			SimulateConditions: func(publishedSessions *models.ISCSISessions, portals []string) {
				// Populate Published Portals
				for idx := len(portals) - 1; idx >= 0; idx-- {
					publishedSessions.Info[portals[idx]] = structCopyHelper(sessionData)

					if idx >= 3 {
						publishedSessions.Info[portals[idx]].PortalInfo.LastAccessTime = time.Now().Add(-time.
							Hour * time.Duration(idx))
					}
				}
			},
		},
	}

	for _, input := range inputs {
		t.Run(input.TestName, func(t *testing.T) {
			input.SimulateConditions(input.PublishedPortals, input.InputPortals)

			SortPortals(input.InputPortals, input.PublishedPortals)

			assert.Equal(t, input.ResultPortals, input.InputPortals, "sort order mismatch")
		})
	}
}

func mapCopyHelper(input map[int32]string) map[int32]string {
	output := make(map[int32]string, len(input))

	for key, value := range input {
		output[key] = value
	}

	return output
}

func reverseSlice(input []string) []string {
	output := make([]string, 0)

	for idx := len(input) - 1; idx >= 0; idx-- {
		output = append(output, input[idx])
	}

	return output
}

func structCopyHelper(input models.ISCSISessionData) *models.ISCSISessionData {
	clone, err := deep.Copy(input)
	if err != nil {
		return &models.ISCSISessionData{}
	}
	return &clone
}

func TestIsPerNodeIgroup(t *testing.T) {
	tt := map[string]bool{
		"":        false,
		"trident": false,
		"node-01-8095-ad1b8212-49a0-82d4-ef4f8b5b620z":                                   false,
		"trident-ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                                   false,
		"-ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                                          false,
		".ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                                          false,
		"igroup-a-trident-ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                          false,
		"node-01-ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                                   true,
		"trdnt-ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                                     true,
		"worker0.hjonhjc.rtp.openenglab.netapp.com-25426e2a-9f96-4f4d-90a8-72a6cdd6f645": true,
		"worker0-hjonhjc.trdnt-ad1b8212-8095-49a0-82d4-ef4f8b5b620z":                     true,
	}

	for input, expected := range tt {
		assert.Equal(t, expected, IsPerNodeIgroup(input))
	}
}

func TestParseInitiatorIQNs(t *testing.T) {
	ctx := context.TODO()
	tests := map[string]struct {
		input     string
		output    []string
		predicate func(string) []string
	}{
		"Single valid initiator": {
			input:  "InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de",
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"initiator with space": {
			input:  "InitiatorName=iqn 2005-03.org.open-iscsi:123abc456de",
			output: []string{"iqn"},
		},
		"Multiple valid initiators": {
			input: `InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de
InitiatorName=iqn.2005-03.org.open-iscsi:secondIQN12`,
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de", "iqn.2005-03.org.open-iscsi:secondIQN12"},
		},
		"Ignore comment initiator": {
			input: `#InitiatorName=iqn.1994-05.demo.netapp.com
InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de`,
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Ignore inline comment": {
			input:  "InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de #inline comment in file",
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Tolerate space around equal sign": {
			input:  "InitiatorName = iqn.2005-03.org.open-iscsi:123abc456de",
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Tolerate leading space": {
			input:  " InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de",
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Tolerate trailing space multiple initiators": {
			input: `InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de
InitiatorName=iqn.2005-03.org.open-iscsi:secondIQN12 `,
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de", "iqn.2005-03.org.open-iscsi:secondIQN12"},
		},
		"Full iscsi file": {
			input: `## DO NOT EDIT OR REMOVE THIS FILE!
## If you remove this file, the iSCSI daemon will not start.
## If you change the InitiatorName, existing access control lists
## may reject this initiator.  The InitiatorName must be unique
## for each iSCSI initiator.  Do NOT duplicate iSCSI InitiatorNames.
InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de
#InitiatorName=iqn.1994-05.demo.netapp.com`,
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Full iscsi file no initiator": {
			input: `## DO NOT EDIT OR REMOVE THIS FILE!
## If you remove this file, the iSCSI daemon will not start.
## If you change the InitiatorName, existing access control lists
## may reject this initiator.  The InitiatorName must be unique
## for each iSCSI initiator.  Do NOT duplicate iSCSI InitiatorNames.
#InitiatorName=iqn.1994-05.demo.netapp.com`,
			output: []string{},
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			iqns := parseInitiatorIQNs(ctx, test.input)
			assert.Equal(t, test.output, iqns, "Failed to parse initiators")
		})
	}
}

func TestGetInitiatorIqns(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCommand := mockexec.NewMockCommand(ctrl)

	tests := map[string]struct {
		commandOutput string
		commandError  error
		expectedIQNs  []string
		expectedError bool
	}{
		"Valid IQNs": {
			commandOutput: "InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de\nInitiatorName=iqn.2005-03.org.open-iscsi:secondIQN12",
			expectedIQNs:  []string{"iqn.2005-03.org.open-iscsi:123abc456de", "iqn.2005-03.org.open-iscsi:secondIQN12"},
			expectedError: false,
		},
		"Malformed IQNs": {
			commandOutput: "InitiatorName=iqn 2005-03.org.open-iscsi:123abc456de",
			expectedIQNs:  []string{"iqn"},
			expectedError: false,
		},
		"File does not exist": {
			commandError:  errors.New("file not found"),
			expectedIQNs:  nil,
			expectedError: true,
		},
		"Empty file": {
			commandOutput: "",
			expectedIQNs:  []string{},
			expectedError: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockCommand.EXPECT().Execute(gomock.Any(), "cat", "/etc/iscsi/initiatorname.iscsi").
				Return([]byte(test.commandOutput), test.commandError)

			command = mockCommand
			iqns, err := GetInitiatorIqns(context.TODO())

			if test.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.expectedIQNs, iqns)
		})
	}
}

func TestGetAllVolumeIDs(t *testing.T) {
	fs := afero.NewMemMapFs()

	tests := map[string]struct {
		setupFiles     []string
		expectedResult []string
		expectedError  bool
	}{
		"Valid tracking files": {
			setupFiles:     []string{"pvc-123.json", "pvc-456.json"},
			expectedResult: []string{"pvc-123", "pvc-456"},
			expectedError:  false,
		},
		"No tracking files": {
			setupFiles:     []string{},
			expectedResult: nil,
			expectedError:  false,
		},
		"Invalid file names": {
			setupFiles:     []string{"invalid-file.txt", "another-file.log"},
			expectedResult: nil,
			expectedError:  false,
		},
		"Mixed valid and invalid files": {
			setupFiles:     []string{"pvc-123.json", "invalid-file.txt", "pvc-456.json"},
			expectedResult: []string{"pvc-123", "pvc-456"},
			expectedError:  false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup the mock filesystem
			dir := "/tracking"
			_ = fs.MkdirAll(dir, 0o755)
			for _, file := range test.setupFiles {
				_, _ = fs.Create(fmt.Sprintf("%s/%s", dir, file))
			}

			// Use afero.ReadDir directly
			files, err := afero.ReadDir(fs, dir)
			assert.NoError(t, err)

			// Extract volume IDs from file names
			var result []string
			for _, file := range files {
				if filepath.Ext(file.Name()) == ".json" {
					result = append(result, strings.TrimSuffix(file.Name(), ".json"))
				}
			}

			// Validate the result
			assert.Equal(t, test.expectedResult, result)

			// Cleanup
			_ = fs.RemoveAll(dir)
		})
	}
}
