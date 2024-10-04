// Copyright 2024 NetApp, Inc. All Rights Reserved.

package iscsi

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/spf13/afero/mem"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/mocks/mock_utils/mock_iscsi"
	"github.com/netapp/trident/mocks/mock_utils/mock_models/mock_luks"
	"github.com/netapp/trident/utils/errors"
	tridentexec "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
)

func TestNew(t *testing.T) {
	ctrl := gomock.NewController(t)
	osClient := mock_iscsi.NewMockOS(ctrl)
	devicesClient := mock_iscsi.NewMockDevices(ctrl)
	FileSystemClient := mock_iscsi.NewMockFileSystem(ctrl)
	mountClient := mock_iscsi.NewMockMount(ctrl)

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

			iscsiClient := New(osClient, devicesClient, FileSystemClient, mountClient)
			assert.NotNil(t, iscsiClient)
		})
	}
}

func TestNewDetailed(t *testing.T) {
	const chrootPathPrefix = ""
	ctrl := gomock.NewController(t)
	osClient := mock_iscsi.NewMockOS(ctrl)
	devicesClient := mock_iscsi.NewMockDevices(ctrl)
	FileSystemClient := mock_iscsi.NewMockFileSystem(ctrl)
	mountClient := mock_iscsi.NewMockMount(ctrl)
	command := mockexec.NewMockCommand(ctrl)
	iscsiClient := NewDetailed(chrootPathPrefix, command, DefaultSelfHealingExclusion, osClient, devicesClient, FileSystemClient,
		mountClient, nil, afero.Afero{Fs: afero.NewMemMapFs()})
	assert.NotNil(t, iscsiClient)
}

func TestClient_AttachVolumeRetry(t *testing.T) {
	type parameters struct {
		chrootPathPrefix    string
		getCommand          func(controller *gomock.Controller) tridentexec.Command
		getOSClient         func(controller *gomock.Controller) OS
		getDeviceClient     func(controller *gomock.Controller) Devices
		getFileSystemClient func(controller *gomock.Controller) FileSystem
		getMountClient      func(controller *gomock.Controller) Mount
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(config.FsExt4, nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				params.getMountClient(ctrl), params.getReconcileUtils(ctrl), afero.Afero{Fs: params.getFileSystemUtils()})

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
		getDeviceClient     func(controller *gomock.Controller) Devices
		getFileSystemClient func(controller *gomock.Controller) FileSystem
		getMountClient      func(controller *gomock.Controller) Mount
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("", errors.New("some error"))
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0),
					errors.New("some error"))
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(1), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(errors.New("some error"))
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsExt4,
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
		"invalid LUKS encryption value in publish info": {
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				LUKSEncryption: "foo",
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().NewLUKSDevice("/dev/dm-0", "test-volume").Return(nil, nil)
				mockDevices.EXPECT().EnsureLUKSDeviceMappedOnHost(context.TODO(), nil, "test-volume",
					map[string]string{}).Return(false, errors.New("some error"))
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsExt4,
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
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)

				device := mock_luks.NewMockLUKSDeviceInterface(controller)
				device.EXPECT().MappedDevicePath().Return("/dev/mapper/dm-0")
				mockDevices.EXPECT().NewLUKSDevice("/dev/dm-0", "test-volume").Return(device, nil)
				mockDevices.EXPECT().EnsureLUKSDeviceMappedOnHost(context.TODO(), device, "test-volume",
					map[string]string{}).Return(true, nil)

				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsRaw,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)

				device := mock_luks.NewMockLUKSDeviceInterface(controller)
				device.EXPECT().MappedDevicePath().Return("/dev/mapper/dm-0")
				mockDevices.EXPECT().NewLUKSDevice("/dev/dm-0", "test-volume").Return(device, nil)
				mockDevices.EXPECT().EnsureLUKSDeviceMappedOnHost(context.TODO(), device, "test-volume",
					map[string]string{}).Return(true, nil)

				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/mapper/dm-0").Return("", errors.New("some error"))

				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsExt4,
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
				return mockCommand
			},
			getOSClient: func(controller *gomock.Controller) OS {
				mockOsClient := mock_iscsi.NewMockOS(controller)
				mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)
				return mockOsClient
			},
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)

				device := mock_luks.NewMockLUKSDeviceInterface(controller)
				device.EXPECT().MappedDevicePath().Return("/dev/mapper/dm-0")
				mockDevices.EXPECT().NewLUKSDevice("/dev/dm-0", "test-volume").Return(device, nil)
				mockDevices.EXPECT().EnsureLUKSDeviceMappedOnHost(context.TODO(), device, "test-volume",
					map[string]string{}).Return(false, nil)

				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/mapper/dm-0").Return("", nil)

				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return("", nil)
				mockDevices.EXPECT().IsDeviceUnformatted(context.TODO(), "/dev/dm-0").Return(false, errors.New("some error"))
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return("", nil)
				mockDevices.EXPECT().IsDeviceUnformatted(context.TODO(), "/dev/dm-0").Return(false, nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return("", nil)
				mockDevices.EXPECT().IsDeviceUnformatted(context.TODO(), "/dev/dm-0").Return(true, nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				mockFileSystem.EXPECT().FormatVolume(context.TODO(), "/dev/dm-0", config.FsExt4, "").Return(errors.New("some error"))
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(config.FsExt3, nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
				return mockMount
			},
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(), targetIQN).
					Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
					Times(6)
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(unknownFstype, nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(unknownFstype, nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				mockFileSystem.EXPECT().RepairVolume(context.TODO(), "/dev/dm-0", config.FsExt4)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(unknownFstype, nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				mockFileSystem.EXPECT().RepairVolume(context.TODO(), "/dev/dm-0", config.FsExt4)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsExt4,
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
				mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(unknownFstype, nil)
				return mockDevices
			},
			getFileSystemClient: func(controller *gomock.Controller) FileSystem {
				mockFileSystem := mock_iscsi.NewMockFileSystem(controller)
				mockFileSystem.EXPECT().RepairVolume(context.TODO(), "/dev/dm-0", config.FsExt4)
				return mockFileSystem
			},
			getMountClient: func(controller *gomock.Controller) Mount {
				mockMount := mock_iscsi.NewMockMount(controller)
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
				mockReconcileUtils.EXPECT().GetMultipathDeviceUUID("dm-0").Return("mpath-53594135475a464a3847314d3930354756483748", nil)
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
				FilesystemType: config.FsExt4,
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
				params.getMountClient(ctrl), params.getReconcileUtils(ctrl), afero.Afero{Fs: params.getFileSystemUtils()})

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
			client := New(nil, nil, nil, nil)
			ctx := context.WithValue(context.TODO(), SessionInfoSource, "test")
			client.AddSession(ctx, params.sessions, &params.publishInfo, params.volID,
				params.sessionNumber, params.reasonInvalid)
		})
	}
}

func TestClient_RescanDevices(t *testing.T) {
	type parameters struct {
		targetIQN string
		lunID     int32
		minSize   int64

		getReconcileUtils  func(controller *gomock.Controller) IscsiReconcileUtils
		getDeviceClient    func(controller *gomock.Controller) Devices
		getCommandClient   func(controller *gomock.Controller) tridentexec.Command
		getFileSystemUtils func() afero.Fs
		assertError        assert.ErrorAssertionFunc
	}

	const targetIQN = "iqn.2010-01.com.netapp:target-1"

	tests := map[string]parameters{
		"error getting device information": {
			targetIQN: targetIQN,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				return NewReconcileUtils("", nil)
			},
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), errors.New("some error"))
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil).Times(2)
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), errors.New("some error"))
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(1), errors.New("some error"))
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(1), nil)
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), errors.New("some error"))
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil).Times(2)
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
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/dm-0").Return(int64(1), nil)
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
			targetIQN: targetIQN,
			getReconcileUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockReconcileUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockReconcileUtils.EXPECT().GetISCSIHostSessionMapForTarget(context.TODO(),
					targetIQN).Return(map[int]int{0: 0})
				mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
				mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
				return mockReconcileUtils
			},
			getDeviceClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
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
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			controller := gomock.NewController(t)

			client := NewDetailed("", params.getCommandClient(controller), DefaultSelfHealingExclusion, nil,
				params.getDeviceClient(controller), nil, nil, params.getReconcileUtils(controller), afero.Afero{Fs: params.getFileSystemUtils()})

			err := client.RescanDevices(context.TODO(), params.targetIQN, params.lunID, params.minSize)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestClient_rescanDisk(t *testing.T) {
	type parameters struct {
		getFileSystemUtils func() afero.Fs
		assertError        assert.ErrorAssertionFunc
	}
	const deviceName = "sda"
	tests := map[string]parameters{
		"happy path": {
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create(fmt.Sprintf("/sys/block/%s/device/rescan", deviceName))
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.NoError,
		},
		"error opening rescan file": {
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				return fs
			},
			assertError: assert.Error,
		},
		"error writing to file": {
			getFileSystemUtils: func() afero.Fs {
				f := &aferoFileWrapper{
					WriteStringError: errors.New("some error"),
					File:             mem.NewFileHandle(&mem.FileData{}),
				}

				memMapFs := afero.NewMemMapFs()
				_, err := memMapFs.Create(fmt.Sprintf("/sys/block/%s/device/rescan", deviceName))
				assert.NoError(t, err)

				fs := &aferoWrapper{
					openFileResponse: f,
					openResponse:     f,
					Fs:               memMapFs,
				}

				return fs
			},
			assertError: assert.Error,
		},
		"failed writing to file": {
			getFileSystemUtils: func() afero.Fs {
				f := &aferoFileWrapper{
					WriteStringCount: 0,
					File:             mem.NewFileHandle(&mem.FileData{}),
				}

				memMapFs := afero.NewMemMapFs()
				_, err := memMapFs.Create(fmt.Sprintf("/sys/block/%s/device/rescan", deviceName))
				assert.NoError(t, err)

				fs := &aferoWrapper{
					openFileResponse: f,
					openResponse:     f,
					Fs:               memMapFs,
				}

				return fs
			},
			assertError: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			client := NewDetailed("", nil, nil, nil, nil, nil, nil, nil, afero.Afero{Fs: params.getFileSystemUtils()})
			err := client.rescanDisk(context.TODO(), deviceName)
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
			client := NewDetailed("", params.getCommand(ctrl), nil, nil, nil, nil, nil, nil, afero.Afero{})

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
			client := NewDetailed("", nil, nil, nil, nil, nil, nil, params.getIscsiUtils(controller), afero.Afero{})

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
		getDevicesClient   func(controller *gomock.Controller) Devices
		getFileSystemUtils func() afero.Fs

		assertError        assert.ErrorAssertionFunc
		expectedDeviceInfo *ScsiDeviceInfo
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
			getDevicesClient: func(controller *gomock.Controller) Devices {
				mockDeviceClient := mock_iscsi.NewMockDevices(controller)
				return mockDeviceClient
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:        assert.Error,
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
			getDevicesClient: func(controller *gomock.Controller) Devices {
				mockDeviceClient := mock_iscsi.NewMockDevices(controller)
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
			getDevicesClient: func(controller *gomock.Controller) Devices {
				mockDeviceClient := mock_iscsi.NewMockDevices(controller)
				return mockDeviceClient
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError:        assert.Error,
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
			getDevicesClient: func(controller *gomock.Controller) Devices {
				mockDeviceClient := mock_iscsi.NewMockDevices(controller)
				return mockDeviceClient
			},
			getFileSystemUtils: func() afero.Fs {
				return afero.NewMemMapFs()
			},
			assertError: assert.NoError,
			expectedDeviceInfo: &ScsiDeviceInfo{
				LUN:         "0",
				Devices:     []string{deviceName},
				DevicePaths: []string{devicePath},
				IQN:         iscisNodeName,
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
			getDevicesClient: func(controller *gomock.Controller) Devices {
				mockDeviceClient := mock_iscsi.NewMockDevices(controller)
				mockDeviceClient.EXPECT().EnsureDeviceReadable(context.TODO(), multipathDevicePath).Return(errors.New("some error"))
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
			getDevicesClient: func(controller *gomock.Controller) Devices {
				mockDeviceClient := mock_iscsi.NewMockDevices(controller)
				mockDeviceClient.EXPECT().EnsureDeviceReadable(context.TODO(), multipathDevicePath).Return(nil)
				mockDeviceClient.EXPECT().GetDeviceFSType(context.TODO(), multipathDevicePath).Return("", errors.New("some error"))
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
			getDevicesClient: func(controller *gomock.Controller) Devices {
				mockDeviceClient := mock_iscsi.NewMockDevices(controller)
				mockDeviceClient.EXPECT().EnsureDeviceReadable(context.TODO(), multipathDevicePath).Return(nil)
				mockDeviceClient.EXPECT().GetDeviceFSType(context.TODO(), multipathDevicePath).Return(config.FsExt4, nil)
				return mockDeviceClient
			},
			getFileSystemUtils: func() afero.Fs {
				fs := afero.NewMemMapFs()
				err := fs.Mkdir(fmt.Sprintf("/sys/block/sda/holders/%s", multipathDeviceName), 777)
				assert.NoError(t, err)
				return fs
			},
			assertError: assert.NoError,
			expectedDeviceInfo: &ScsiDeviceInfo{
				LUN:             "0",
				Devices:         []string{deviceName},
				DevicePaths:     []string{devicePath},
				MultipathDevice: multipathDeviceName,
				IQN:             iscisNodeName,
				Filesystem:      config.FsExt4,
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
				})
			deviceInfo, err := client.getDeviceInfoForLUN(context.TODO(), params.hostSessionMap, params.lunID,
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
			client := NewDetailed("", nil, nil, nil, nil, nil, nil, nil, afero.Afero{Fs: params.getFileSystemUtils()})
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
			client := NewDetailed("", nil, nil, nil, nil, nil, nil, nil, afero.Afero{Fs: params.getFileSystemUtils()})

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
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewDetailed("", nil, nil, nil, nil, nil, nil, params.getIscsiUtils(ctrl), afero.Afero{Fs: params.getFileSystemUtils()})

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
			assertError: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewDetailed("", params.getCommandClient(ctrl), nil, params.getOsClient(ctrl), nil, nil, nil,
				params.getIscsiUtils(ctrl),
				afero.Afero{Fs: params.getFileSystemUtils()})

			err := client.waitForDeviceScan(context.TODO(), params.hostSessionMap, lunID, iscsiNodeName)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
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
			client := NewDetailed("", nil, nil, nil, nil, nil, nil, nil, afero.Afero{Fs: params.getFileSystemUtils()})

			err := client.scanTargetLUN(context.TODO(), lunID, []int{host1, host2})
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
			client := NewDetailed("", nil, nil, nil, nil, nil, nil, params.getIscsiUtils(ctrl), afero.Afero{Fs: params.getFileSystemUtils()})

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
			client := NewDetailed("", nil, nil, nil, nil, nil, nil, nil, afero.Afero{Fs: params.getFileSystemUtils()})
			response, err := client.getLunSerial(context.TODO(), devicePath)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			assert.Equal(t, params.expectedResponse, response)
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
				afero.Afero{Fs: params.getFileSystemUtils()})

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
		lunSerial     string
		getIscsiUtils func(controller *gomock.Controller) IscsiReconcileUtils
		assertError   assert.ErrorAssertionFunc
	}

	const multipathDeviceName = "dm-0"
	const vpdpg80Serial = "SYA5GZFJ8G1M905GVH7H"
	lunSerialHex := hex.EncodeToString([]byte(vpdpg80Serial))
	multipathdeviceSerial := fmt.Sprintf("mpath-%s", lunSerialHex)

	tests := map[string]parameters{
		"empty lun serial": {
			lunSerial: "",
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				return mockIscsiUtils
			},
			assertError: assert.NoError,
		},
		"multipath device not present": {
			lunSerial: vpdpg80Serial,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetMultipathDeviceUUID(multipathDeviceName).Return("",
					errors.NotFoundError("not found"))
				return mockIscsiUtils
			},
			assertError: assert.NoError,
		},
		"error getting multipath device UUID": {
			lunSerial: vpdpg80Serial,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetMultipathDeviceUUID(multipathDeviceName).Return("",
					errors.New("some error"))
				return mockIscsiUtils
			},
			assertError: assert.Error,
		},
		"LUN serial not present in multipath device UUID": {
			lunSerial: vpdpg80Serial,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetMultipathDeviceUUID(multipathDeviceName).Return("foo", nil)
				return mockIscsiUtils
			},
			assertError: assert.Error,
		},
		"happy path": {
			lunSerial: vpdpg80Serial,
			getIscsiUtils: func(controller *gomock.Controller) IscsiReconcileUtils {
				mockIscsiUtils := mock_iscsi.NewMockIscsiReconcileUtils(controller)
				mockIscsiUtils.EXPECT().GetMultipathDeviceUUID(multipathDeviceName).Return(multipathdeviceSerial, nil)
				return mockIscsiUtils
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewDetailed("", nil, nil, nil, nil, nil, nil, params.getIscsiUtils(ctrl), afero.Afero{})
			err := client.verifyMultipathDeviceSerial(context.TODO(), multipathDeviceName, params.lunSerial)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestClient_verifyMultipathDeviceSize(t *testing.T) {
	type parameters struct {
		getDevicesClient   func(controller *gomock.Controller) Devices
		assertError        assert.ErrorAssertionFunc
		assertValid        assert.BoolAssertionFunc
		expectedDeviceSize int64
	}

	const deviceName = "sda"
	const multipathDeviceName = "dm-0"

	tests := map[string]parameters{
		"error getting device size": {
			getDevicesClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(0),
					errors.New("some error"))
				return mockDevices
			},
			assertError:        assert.Error,
			assertValid:        assert.False,
			expectedDeviceSize: 0,
		},
		"error getting multipath device size": {
			getDevicesClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(1), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(0),
					errors.New("some error"))
				return mockDevices
			},
			assertError:        assert.Error,
			assertValid:        assert.False,
			expectedDeviceSize: 0,
		},
		"device size != multipath device size": {
			getDevicesClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(1), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(0), nil)
				return mockDevices
			},
			assertError:        assert.NoError,
			assertValid:        assert.False,
			expectedDeviceSize: 1,
		},
		"happy path": {
			getDevicesClient: func(controller *gomock.Controller) Devices {
				mockDevices := mock_iscsi.NewMockDevices(controller)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(1), nil)
				mockDevices.EXPECT().GetISCSIDiskSize(context.TODO(), gomock.Any()).Return(int64(1), nil)
				return mockDevices
			},
			assertError:        assert.NoError,
			assertValid:        assert.True,
			expectedDeviceSize: 0,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := NewDetailed("", nil, nil, nil, params.getDevicesClient(ctrl), nil, nil, nil, afero.Afero{})
			deviceSize, valid, err := client.verifyMultipathDeviceSize(context.TODO(), multipathDeviceName, deviceName)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			if params.assertValid != nil {
				params.assertValid(t, valid)
			}
			assert.Equal(t, params.expectedDeviceSize, deviceSize)
		})
	}
}

func TestClient_EnsureSessions(t *testing.T) {
	type parameters struct {
		publishInfo        models.VolumePublishInfo
		portals            []string
		getCommandClient   func(controller *gomock.Controller) tridentexec.Command
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
			client := NewDetailed("", params.getCommandClient(ctrl), nil, nil, nil, nil, nil, nil,
				afero.Afero{Fs: params.getFileSystemUtils()})
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
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			iscsiClient := NewDetailed("", params.getCommandClient(ctrl), nil, nil, nil, nil, nil, nil,
				afero.Afero{Fs: afero.NewMemMapFs()})

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
			client := NewDetailed("", params.getCommandClient(ctrl), nil, nil, nil, nil, nil, nil, afero.Afero{})
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
			iscsiClient := NewDetailed("", mockExec, nil, nil, nil, nil, nil, nil, afero.Afero{})

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
			assert.Equal(t, testCase.OutputIP, ensureHostportFormatted(testCase.InputIP),
				"Hostport not correctly formatted")
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
