// Copyright 2024 NetApp, Inc. All Rights Reserved.

package fcp

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/spf13/afero/mem"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
)

var multipathConf = `
defaults {
    user_friendly_names yes
    find_multipaths no
}

`

func TestNew_DockerPluginTrue(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}
	originalPluginMode := os.Getenv("DOCKER_PLUGIN_MODE")
	defer func(pluginMode string) {
		err := os.Setenv("DOCKER_PLUGIN_MODE", pluginMode)
		assert.NoError(t, err)
	}(originalPluginMode)

	type parameters struct {
		setUpEnvironment func()
	}
	params := parameters{
		setUpEnvironment: func() {
			assert.Nil(t, os.Setenv("DOCKER_PLUGIN_MODE", "true"))
		},
	}
	if params.setUpEnvironment != nil {
		params.setUpEnvironment()
	}

	fcpClient, err := New()
	assert.NoError(t, err, "New returns error")
	assert.NotNil(t, fcpClient, "fcpClient not created")
}

func TestNew_DockerPluginFalse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}
	originalPluginMode := os.Getenv("DOCKER_PLUGIN_MODE")
	defer func(pluginMode string) {
		err := os.Setenv("DOCKER_PLUGIN_MODE", pluginMode)
		assert.NoError(t, err)
	}(originalPluginMode)

	type parameters struct {
		setUpEnvironment func()
	}
	params := parameters{
		setUpEnvironment: func() {
			assert.Nil(t, os.Setenv("DOCKER_PLUGIN_MODE", "false"))
		},
	}
	if params.setUpEnvironment != nil {
		params.setUpEnvironment()
	}
	fcpClient, err := New()
	assert.NoError(t, err, "New returns error")
	assert.NotNil(t, fcpClient, "fcpClient not created")
}

func TestNewDetailed(t *testing.T) {
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, _, _, _ := NewDrivers()
	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion, mockOsClient, mockDevices,
		mockFileSystem,
		mockMount, nil, afero.Afero{Fs: afero.NewMemMapFs()})
	assert.NotNil(t, fcpClient, "NewDetailed create nil fcpClient")
}

func TestMultipathdIsRunning_IsTrue(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockExec := mockexec.NewMockCommand(mockCtrl)
	ctx := context.Background()
	execOut := "1234"
	expectedValue := true

	// Setup mock calls.
	mockExec.EXPECT().Execute(
		ctx, gomock.Any(), gomock.Any(),
	).Return([]byte(execOut), nil)

	fcpClient := NewDetailed("", mockExec, nil, nil, nil, nil, nil, nil, afero.Afero{Fs: afero.NewMemMapFs()})

	actualValue := fcpClient.multipathdIsRunning(context.Background())
	assert.Equal(t, expectedValue, actualValue)
}

func TestMultipathdIsRunning_IsFalse(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockExec := mockexec.NewMockCommand(mockCtrl)
	ctx := context.Background()
	execOut := ""
	expectedValue := true

	// Setup mock calls.
	mockExec.EXPECT().Execute(
		ctx, gomock.Any(), gomock.Any(),
	).Return([]byte(execOut), nil).Times(2)

	fcpClient := NewDetailed("", mockExec, nil, nil, nil, nil, nil, nil, afero.Afero{Fs: afero.NewMemMapFs()})

	actualValue := fcpClient.multipathdIsRunning(context.Background())
	assert.NotEqual(t, expectedValue, actualValue)
}

func TestMultipathdIsRunning_IsError(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockExec := mockexec.NewMockCommand(mockCtrl)
	ctx := context.Background()
	execOut := "1234"
	expectedValue := false

	// Setup mock calls.
	mockExec.EXPECT().Execute(
		ctx, gomock.Any(), gomock.Any(),
	).Return([]byte(execOut), errors.New("cmd error")).Times(2)

	fcpClient := NewDetailed("", mockExec, nil, nil, nil, nil, nil, nil, afero.Afero{Fs: afero.NewMemMapFs()})

	actualValue := fcpClient.multipathdIsRunning(context.Background())
	assert.Equal(t, expectedValue, actualValue, "multipathdIsRunning failed")
}

func TestClient_AttachVolumeRetry_preCheckFailure(t *testing.T) {
	testTimeout := 2 * time.Second

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil).AnyTimes()
	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
		"config").Return([]byte(multipathConf), nil).AnyTimes()

	mockReconcileUtils.EXPECT().CheckZoningExistsWithTarget(context.TODO(), "").Return(false,
		errors.New("some-error")).AnyTimes()
	publishInfo := models.VolumePublishInfo{}

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	_, err := fcpClient.AttachVolumeRetry(context.TODO(), "", "", &publishInfo, nil,
		testTimeout)
	assert.NotNil(t, err, "AttachVolumeRetry returns nil error")
}

func TestClient_AttachVolumeRetry_attachVolumeFailure(t *testing.T) {
	testTimeout := 2 * time.Second

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil).AnyTimes()
	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
		"config").Return([]byte("    find_multipaths: yes"), nil).AnyTimes()

	mockReconcileUtils.EXPECT().CheckZoningExistsWithTarget(context.TODO(), "").Return(false,
		errors.New("some-error")).AnyTimes()
	publishInfo := models.VolumePublishInfo{}

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	_, err := fcpClient.AttachVolumeRetry(context.TODO(), "", "", &publishInfo, nil,
		testTimeout)
	assert.NotNil(t, err, "AttachVolumeRetry returns nil error")
}

func TestClient_AttachVolumeRetry(t *testing.T) {
	testTimeout := 2 * time.Second
	publishInfo := models.VolumePublishInfo{
		FilesystemType: filesystem.Ext4,
	}

	deviceAddresses := make([]models.ScsiDeviceAddress, 0)
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil).AnyTimes()
	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
		"config").Return([]byte(multipathConfig("no", false)), nil).AnyTimes()

	mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)

	mockDevices.EXPECT().ListAllDevices(context.TODO()).AnyTimes()
	mockDevices.EXPECT().ScanTargetLUN(context.TODO(), deviceAddresses)
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
	mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
		nil)
	mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
	mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(filesystem.Ext4, nil)

	mockMount.EXPECT().IsMounted(context.TODO(), "/dev/dm-0", "", "").Return(true, nil)

	mockReconcileUtils.EXPECT().CheckZoningExistsWithTarget(context.TODO(), "").Return(true,
		nil).AnyTimes()
	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		"").Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
		Times(3)
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)

	assert.NoError(t, fs.Mkdir("/sys/block/sda/holders", 777))
	_, err := fs.Create("/sys/block/sda/holders/dm-0")
	assert.NoError(t, err)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})
	_, attachVolumeErr := fcpClient.AttachVolumeRetry(context.TODO(), "", "", &publishInfo, nil,
		testTimeout)
	assert.Nil(t, attachVolumeErr, "AttachVolumeRetry returns error")
}

func TestClient_RescanDevices_deviceInformationError(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()
	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return(nil)
	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	err := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 0)
	assert.NotNil(t, err, "RescanDevices return nil error")
}

func TestClient_RescanDevices_returnsNilPath(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return(nil)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	err := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 0)
	assert.NotNil(t, err, "RescanDevices return nil error")
}

func TestClient_RescanDevices_gettingNilDevices(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return(nil, nil)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	err := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 0)
	assert.NotNil(t, err, "RescanDevices return nil error")
}

func TestClient_RescanDevices_errorGettingDiskSize(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)

	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), errors.New("some error"))
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	err := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 0)
	assert.NotNil(t, err, "RescanDevices return nil error")
}

func TestClient_RescanDevices_rescanDiskFailedAllLargeTrue(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)

	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil).Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil).Times(1)
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").AnyTimes()
	mockDevices.EXPECT().ListAllDevices(context.TODO()).AnyTimes()

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	err := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 1)
	assert.NotNil(t, err, "RescanDevices return nil error")
}

func TestClient_RescanDevices_rescanDiskFailedAllLargeFalse(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)

	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil).Times(2)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil).Times(1)
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").AnyTimes()
	mockDevices.EXPECT().ListAllDevices(context.TODO()).AnyTimes()
	f := &aferoFileWrapper{
		WriteStringCount: 0,
		File:             mem.NewFileHandle(&mem.FileData{}),
	}
	memFs := afero.NewMemMapFs()
	_, err := memFs.Create("/sys/block/sda/device/rescan")
	assert.NoError(t, err)

	fs := &aferoWrapper{
		openFileResponse: f,
		Fs:               memFs,
	}

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})
	rescanErr := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 1)
	assert.NotNil(t, rescanErr, "RescanDevices return nil error")
}

func TestClient_RescanDevices_rescanDiskFailedAllLargeFalseFailed(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)

	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil).Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), errors.New("ut-err")).Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil).Times(1)
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").AnyTimes()
	mockDevices.EXPECT().ListAllDevices(context.TODO()).AnyTimes()
	f := &aferoFileWrapper{
		WriteStringCount: 0,
		File:             mem.NewFileHandle(&mem.FileData{}),
	}
	memFs := afero.NewMemMapFs()
	_, err := memFs.Create("/sys/block/sda/device/rescan")
	assert.NoError(t, err)

	fs := &aferoWrapper{
		openFileResponse: f,
		Fs:               memFs,
	}

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})
	rescanErr := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 1)
	assert.NotNil(t, rescanErr, "RescanDevices return nil error")
}

func TestClient_RescanDevices_MultipathNotNil(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)

	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil).Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), errors.New("ut-err")).Times(1)
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").AnyTimes()
	mockDevices.EXPECT().ListAllDevices(context.TODO()).AnyTimes()

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	err := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 0)
	assert.NotNil(t, err, "RescanDevices return nil error")
}

func TestClient_RescanDevices_MultipathNotNilSmall(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipath", 10*time.Second, true, "-r",
		"/dev/dm-0").Return([]byte(multipathConfig("no", false)), nil)

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)

	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil).Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil).Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil).Times(1)

	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").AnyTimes()
	mockDevices.EXPECT().ListAllDevices(context.TODO()).AnyTimes()

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	err := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 1)
	assert.NotNil(t, err, "RescanDevices return nil error")
}

func TestClient_RescanDevices_MultipathNotNilSmallFailed(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipath", 10*time.Second, true, "-r",
		"/dev/dm-0").Return([]byte(multipathConfig("no", false)), errors.New("ut-err"))

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)

	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil).Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil).Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil).Times(1)

	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").AnyTimes()
	mockDevices.EXPECT().ListAllDevices(context.TODO()).AnyTimes()

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	err := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 1)
	assert.NotNil(t, err, "RescanDevices return nil error")
}

func TestClient_RescanDevices_MultipathNotNilSmallRetryFailed(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipath", 10*time.Second, true, "-r",
		"/dev/dm-0").Return([]byte(multipathConfig("no", false)), nil)

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)

	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil).Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil).Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), errors.New("ut-err")).Times(1)

	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").AnyTimes()
	mockDevices.EXPECT().ListAllDevices(context.TODO()).AnyTimes()

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	err := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 1)
	assert.NotNil(t, err, "RescanDevices return nil error")
}

func TestClient_RescanDevices_resizeDiskFailed(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)

	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), errors.New("ut-err")).Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), errors.New("ut-err")).AnyTimes()
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").AnyTimes()
	mockDevices.EXPECT().ListAllDevices(context.TODO()).AnyTimes()

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	err := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 0)
	assert.NotNil(t, err, "RescanDevices return nil error")
}

func TestClient_RescanDevices_resizeSuccess(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)

	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil).Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(1), nil).Times(1)

	_, err := fs.Create("/sys/block/sda/device/rescan")
	assert.NoError(t, err)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	rescanErr := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 0)
	assert.Nil(t, rescanErr, "RescanDevices returns error")
}

func TestClient_RescanDevices_failureMultipathsize(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil).Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(1), nil).Times(1)

	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(1), errors.New("some error"))
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")
	mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)

	_, err := fs.Create("/sys/block/sda/device/rescan")
	_, err = fs.Create("/sys/block/sda/holders/dm-0")
	assert.NoError(t, err)

	_, err = fs.Create("/sys/block/sda/device/rescan")
	assert.NoError(t, err)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	rescanErr := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 0)
	assert.Nil(t, rescanErr, "RescanDevices returns error")
}

func TestClient_RescanDevices_multipathsizeGreaterThanMin(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)

	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(1), nil)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(1), nil)
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")
	mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)

	_, err := fs.Create("/sys/block/sda/device/rescan")
	_, err = fs.Create("/sys/block/sda/holders/dm-0")
	assert.NoError(t, err)

	_, err = fs.Create("/sys/block/sda/device/rescan")
	assert.NoError(t, err)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	rescanErr := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 0)
	assert.Nil(t, rescanErr, "RescanDevices returns error")
}

func TestClient_RescanDevices(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()
	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)

	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(1)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/sda").Return(int64(0), nil)
	mockDevices.EXPECT().GetDiskSize(context.TODO(), "/dev/dm-0").Return(int64(0), nil)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})
	rescanErr := fcpClient.RescanDevices(context.TODO(), targetNodeName, 0, 0)
	assert.Nil(t, rescanErr, "RescanDevices returns error")
}

func TestClient_AttachVolume(t *testing.T) {
	vpdpg80Serial := "SYA5GZFJ8G1M905GVH7H"
	deviceAddresses := make([]models.ScsiDeviceAddress, 0)
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil)
	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
		"config").Return([]byte(multipathConfig("no", false)), nil)

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(), "").
		Return([]map[string]int{{"": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
		AnyTimes()
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil).Times(2)
	mockReconcileUtils.EXPECT().CheckZoningExistsWithTarget(context.TODO(), "").Return(true,
		nil)

	mockMount.EXPECT().IsMounted(context.TODO(), "/dev/dm-0", "", "").Return(false, nil)
	mockMount.EXPECT().MountDevice(context.TODO(), "/dev/dm-0", "/mnt/test-volume", "",
		false).Return(nil)

	mockFileSystem.EXPECT().RepairVolume(context.TODO(), "/dev/dm-0", filesystem.Ext4)

	mockOsClient.EXPECT().PathExists("/dev/sda/block").Return(true, nil)

	mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return(vpdpg80Serial, nil).AnyTimes()
	mockDevices.EXPECT().ScanTargetLUN(context.TODO(), deviceAddresses)
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0").Times(2)
	mockDevices.EXPECT().VerifyMultipathDeviceSize(context.TODO(), "dm-0", "sda").Return(int64(0), true,
		nil)
	mockDevices.EXPECT().WaitForDevice(context.TODO(), "/dev/dm-0").Return(nil)
	mockDevices.EXPECT().GetDeviceFSType(context.TODO(), "/dev/dm-0").Return(filesystem.UnknownFstype, nil)
	mockDevices.EXPECT().GetMultipathDeviceUUID("dm-0").Return(
		"mpath-53594135475a464a3847314d3930354756483748", nil).AnyTimes()

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

	publishInfo := models.VolumePublishInfo{
		FilesystemType: filesystem.Ext4,
		VolumeAccessInfo: models.VolumeAccessInfo{
			FCPAccessInfo: models.FCPAccessInfo{
				FCPLunSerial: vpdpg80Serial,
			},
		},
	}
	volumeAuthSecrets := make(map[string]string, 0)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})

	_, attachErr := fcpClient.AttachVolume(context.TODO(), "test-volume", "/mnt/test-volume",
		&publishInfo, volumeAuthSecrets)
	assert.Nil(t, attachErr, "AttachVolume returns error")
}

func TestClient_AttachVolume_preCheckFailure(t *testing.T) {
	publishInfo := models.VolumePublishInfo{}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil).AnyTimes()
	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
		"config").Return([]byte("    find_multipaths: yes"), nil).AnyTimes()
	mockReconcileUtils.EXPECT().CheckZoningExistsWithTarget(context.TODO(), "").Return(false,
		errors.New("some-error")).AnyTimes()

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})

	_, attachErr := fcpClient.AttachVolume(context.TODO(), "test-volume", "/mnt/test-volume",
		&publishInfo, nil)
	assert.NotNil(t, attachErr, "AttachVolume returns nil error")
}

func TestClient_AttachVolume_preCheckFailureMultiPathFailure(t *testing.T) {
	publishInfo := models.VolumePublishInfo{
		FilesystemType: filesystem.Ext4,
	}
	volumeName := "test-volume"
	volumeMountPoint := "/mnt/test-volume"
	volumeAuthSecrets := make(map[string]string, 0)
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").
		Return(nil, errors.New("some error"))
	mockCommand.EXPECT().Execute(context.TODO(), "multipathd", "show", "daemon").
		Return(nil, errors.New("some error"))

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})

	_, attachErr := fcpClient.AttachVolume(context.TODO(), volumeName, volumeMountPoint,
		&publishInfo, volumeAuthSecrets)
	assert.NotNil(t, attachErr, "AttachVolume returns nil error")
}

func TestClient_AttachVolume_zoningCheckFailed(t *testing.T) {
	publishInfo := models.VolumePublishInfo{
		FilesystemType: filesystem.Ext4,
	}
	volumeName := "test-volume"
	volumeMountPoint := "/mnt/test-volume"
	volumeAuthSecrets := make(map[string]string, 0)
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").
		Return(nil, errors.New("some error"))
	mockCommand.EXPECT().Execute(context.TODO(), "multipathd", "show", "daemon").
		Return([]byte("pid 2509 idle"), nil)
	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
		"config").Return(nil, errors.New("some error"))
	mockReconcileUtils.EXPECT().CheckZoningExistsWithTarget(context.TODO(), "").Return(false,
		errors.New("some-error"))

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})

	_, attachErr := fcpClient.AttachVolume(context.TODO(), volumeName, volumeMountPoint,
		&publishInfo, volumeAuthSecrets)
	assert.NotNil(t, attachErr, "AttachVolume returns nil error")
}

func TestClient_AttachVolume_hostSessionNil(t *testing.T) {
	publishInfo := models.VolumePublishInfo{
		FilesystemType: filesystem.Ext4,
	}
	volumeName := "test-volume"
	volumeMountPoint := "/mnt/test-volume"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil).AnyTimes()
	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
		"config").Return([]byte("    find_multipaths: no"), nil).AnyTimes()

	mockReconcileUtils.EXPECT().CheckZoningExistsWithTarget(context.TODO(), "").Return(true, nil).AnyTimes()
	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(), "").
		Return([]map[string]int{})
	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})

	_, attachErr := fcpClient.AttachVolume(context.TODO(), volumeName, volumeMountPoint,
		&publishInfo, nil)
	assert.NotNil(t, attachErr, "AttachVolume returns nil error")
}

func TestClient_AttachVolume_invalidSerials(t *testing.T) {
	vpdpg80Serial := "SYA5GZFJ8G1M905GVH7H"
	publishInfo := models.VolumePublishInfo{
		FilesystemType: filesystem.Ext4,
		VolumeAccessInfo: models.VolumeAccessInfo{
			FCPAccessInfo: models.FCPAccessInfo{
				FCPLunSerial: vpdpg80Serial,
			},
		},
	}
	volumeName := "test-volume"
	volumeMountPoint := "/mnt/test-volume"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil).AnyTimes()
	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
		"config").Return([]byte("    find_multipaths: no"), nil).AnyTimes()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
		Times(1)
	mockReconcileUtils.EXPECT().CheckZoningExistsWithTarget(context.TODO(), "").Return(true, nil).AnyTimes()
	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(), "").
		Return([]map[string]int{{"": 0}, {}})
	mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return("wrong-serial", nil).Times(1)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})

	_, attachErr := fcpClient.AttachVolume(context.TODO(), volumeName, volumeMountPoint,
		&publishInfo, nil)
	assert.NotNil(t, attachErr, "AttachVolume returns nil error")
}

func TestClient_AttachVolume_invalidSerialsErr(t *testing.T) {
	vpdpg80Serial := "SYA5GZFJ8G1M905GVH7H"
	publishInfo := models.VolumePublishInfo{
		FilesystemType: filesystem.Ext4,
		VolumeAccessInfo: models.VolumeAccessInfo{
			FCPAccessInfo: models.FCPAccessInfo{
				FCPLunSerial: vpdpg80Serial,
			},
		},
	}
	volumeName := "test-volume"
	volumeMountPoint := "/mnt/test-volume"

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil).AnyTimes()
	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
		"config").Return([]byte("    find_multipaths: no"), nil).AnyTimes()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"}).
		Times(2)
	mockReconcileUtils.EXPECT().CheckZoningExistsWithTarget(context.TODO(), "").Return(true, nil).AnyTimes()
	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(), "").
		Return([]map[string]int{{"": 0}, {}})
	mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return("wrong-serial", os.ErrNotExist).Times(1)
	mockDevices.EXPECT().GetLunSerial(context.TODO(), "/dev/sda").Return("wrong-serial", errors.New("ut-err")).Times(1)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})

	_, attachErr := fcpClient.AttachVolume(context.TODO(), volumeName, volumeMountPoint,
		&publishInfo, nil)
	assert.NotNil(t, attachErr, "AttachVolume returns nil error")
}

func TestClient_AttachVolume_AttachVolumeFailed(t *testing.T) {
	publishInfo := models.VolumePublishInfo{
		FilesystemType: filesystem.Ext4,
	}
	volumeName := "test-volume"
	volumeMountPoint := "/mnt/test-volume"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, _ := NewDrivers()

	mockCommand.EXPECT().Execute(context.TODO(), "pgrep", "multipathd").Return([]byte("150"), nil).AnyTimes()
	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipathd", 5*time.Second, false, "show",
		"config").Return([]byte("    find_multipaths: yes"), nil).AnyTimes()
	mockReconcileUtils.EXPECT().CheckZoningExistsWithTarget(context.TODO(), "").Return(false,
		errors.New("some-error")).AnyTimes()

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: afero.NewMemMapFs()})

	_, attachErr := fcpClient.AttachVolume(context.TODO(), volumeName, volumeMountPoint,
		&publishInfo, nil)
	assert.NotNil(t, attachErr, "AttachVolume returns nil error")
}

func TestClient_rescanDisk(t *testing.T) {
	_, _, mockDevices, _, _, _, _, fs := NewDrivers()
	mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)

	_, err := fs.Create(fmt.Sprintf("/sys/block/%s/device/rescan", "sda"))
	assert.NoError(t, err)

	fcpClient := NewDetailed("", nil, nil,
		nil, mockDevices, nil,
		nil, nil, afero.Afero{Fs: fs})

	err = fcpClient.rescanDisk(context.TODO(), "sda")
	assert.Nil(t, err, "rescanDisk returns error")
}

func TestClient_rescanDisk_PathDoesNotExist(t *testing.T) {
	_, _, mockDevices, _, _, _, _, fs := NewDrivers()
	mockDevices.EXPECT().ListAllDevices(context.TODO()).Times(2)

	fcpClient := NewDetailed("", nil, nil,
		nil, mockDevices, nil,
		nil, nil, afero.Afero{Fs: fs})

	err := fcpClient.rescanDisk(context.TODO(), "sda")
	assert.NotNil(t, err, "rescanDisk returns nil error")
}

func TestPidRunningOrIdleRegex_EmptyString(t *testing.T) {
	result := pidRunningOrIdleRegex.MatchString("")
	assert.False(t, result, "Expected the regex to not match an empty string")
}

func TestPidRunningOrIdleRegex_NegativeDigit(t *testing.T) {
	result := pidRunningOrIdleRegex.MatchString("pid -5 running")
	assert.False(t, result, "Expected the regex to not match given string")
}

func TestPidRunningOrIdleRegex_MissingDigit(t *testing.T) {
	result := pidRunningOrIdleRegex.MatchString("pid running")
	assert.False(t, result, "Expected the regex to not match given string")
}

func TestPidRunningOrIdleRegex_CorrectDigit(t *testing.T) {
	pidRunningOrIdleRegex = regexp.MustCompile(`pid \d+ (running|idle)`)
	result := pidRunningOrIdleRegex.MatchString("pid 5 running")
	assert.True(t, result, "Expected the regex to match the string 'pid 5 running'")
}

func TestPidRunningOrIdleRegex_CorrectDigits(t *testing.T) {
	result := pidRunningOrIdleRegex.MatchString("pid 2509 running")
	assert.True(t, result, "Expected the regex to match the string 'pid 2509 running'")
}

func TestClient_IsAlreadyAttached(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"a": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return([]string{"sda"}, nil)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	attached := fcpClient.IsAlreadyAttached(context.TODO(), 0, targetNodeName)

	assert.True(t, attached, "IsAlreadyAttached returns false")
}

func TestClient_IsAlreadyAttached_noHostSessions(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(), targetNodeName).Return(nil)
	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	attached := fcpClient.IsAlreadyAttached(context.TODO(), 0, targetNodeName)

	assert.False(t, attached, "IsAlreadyAttached returns true")
}

func TestClient_IsAlreadyAttached_GetLUNDevicesFailed(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"a": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return(nil, errors.New("some error"))

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	attached := fcpClient.IsAlreadyAttached(context.TODO(), 0, targetNodeName)

	assert.False(t, attached, "IsAlreadyAttached returns true")
}

func TestClient_IsAlreadyAttached_noLUNDevices(t *testing.T) {
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetFCPHostSessionMapForTarget(context.TODO(),
		targetNodeName).Return([]map[string]int{{"a": 0}, {}})
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{"/dev/sda"})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{"/dev/sda"}).Return(nil, nil)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	attached := fcpClient.IsAlreadyAttached(context.TODO(), 0, targetNodeName)

	assert.False(t, attached, "IsAlreadyAttached returns true")
}

func TestClient_reloadMultipathDevice(t *testing.T) {
	multipathDeviceName := "dm-0"
	moultipathDevicePath := "/dev/" + multipathDeviceName
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipath", 10*time.Second, true, "-r",
		moultipathDevicePath).Return(nil, nil)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	err := fcpClient.reloadMultipathDevice(context.TODO(), multipathDeviceName)
	assert.Nil(t, err, "reloadMultipathDevice returns error")
}

func TestClient_reloadMultipathDevice_noDevice(t *testing.T) {
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	err := fcpClient.reloadMultipathDevice(context.TODO(), "")
	assert.NotNil(t, err, "reloadMultipathDevice returns error")
}

func TestClient_reloadMultipathDevice_GetMultiPathFailed(t *testing.T) {
	multipathDeviceName := "dm-0"
	moultipathDevicePath := "/dev/" + multipathDeviceName
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "multipath", 10*time.Second, true, "-r",
		moultipathDevicePath).Return(nil, errors.New("some error"))

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	err := fcpClient.reloadMultipathDevice(context.TODO(), multipathDeviceName)
	assert.NotNil(t, err, "reloadMultipathDevice returns error")
}

func TestClient_purgeOneLun_FileOpenFailed(t *testing.T) {
	devicePath := "/dev/sda"
	err := purgeOneLun(context.TODO(), devicePath)
	assert.NotNil(t, err, "purgeOneLun return nil error")
}

func TestClient_rescanOneLun_FileOpenFailed(t *testing.T) {
	devicePath := "/dev/sda"
	err := rescanOneLun(context.TODO(), devicePath)
	assert.NotNil(t, err, "rescanOneLun returns nil error")
}

func TestClient_purgeOneLun_FileNotPresent(t *testing.T) {
	devicePath := "/dev/sda"
	err := purgeOneLun(context.TODO(), devicePath)
	assert.Error(t, err, "Expected no error when purging a non-existent file")
}

func TestClient_purgeOneLun_writeFile(t *testing.T) {
	type parameters struct {
		path               string
		getFileSystemUtils func() afero.Fs
	}
	devicePath := "/dev/sda"
	tests := parameters{
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
	}

	err := purgeOneLun(context.TODO(), tests.path)
	assert.NotNil(t, err, "Expected a non-nil error when writing a file")
}

func TestClient_rescanOneLun(t *testing.T) {
	type parameters struct {
		path               string
		getFileSystemUtils func() afero.Fs
		assertError        assert.ErrorAssertionFunc
	}

	devicePath := "/dev/sda"

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
			err := rescanOneLun(context.TODO(), params.path)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestClient_getDeviceInfoForLUN(t *testing.T) {
	fcpNodeName := "fcpNode"
	multipathDeviceName := "dm-0"
	multipathDevicePath := "/dev/" + multipathDeviceName
	deviceName := "sda"
	devicePath := "/dev/" + deviceName

	hostSessionMap := []map[string]int{{"a": 0}, {}}
	lunID := 0
	needFS := true

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return([]string{deviceName}, nil)

	mockDevices.EXPECT().EnsureDeviceReadable(context.TODO(), multipathDevicePath).Return(nil)
	mockDevices.EXPECT().GetDeviceFSType(context.TODO(), multipathDevicePath).Return(filesystem.Ext4, nil)
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})
	err := fs.Mkdir(fmt.Sprintf("/sys/block/sda/holders/%s", multipathDeviceName), 777)
	assert.NoError(t, err)

	deviceInfo, lunErr := fcpClient.GetDeviceInfoForLUN(context.TODO(), hostSessionMap, lunID,
		fcpNodeName, needFS)
	assert.Nil(t, lunErr, "GetDeviceInfoForLUN returns error")
	assert.NotNil(t, deviceInfo, "GetDeviceInfoForLUN returns nil deviInformation")
}

func TestClient_getDeviceInfoForLUN_noHostInfo(t *testing.T) {
	fcpNodeName := "fcpNode"
	multipathDeviceName := "dm-0"

	hostSessionMap := []map[string]int{{"a": 0}, {}}
	lunID := 0
	needFS := true

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return(make([]string, 0))
	mockReconcileUtils.EXPECT().GetDevicesForLUN(gomock.Any()).Return(make([]string, 0), nil)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})
	err := fs.Mkdir(fmt.Sprintf("/sys/block/sda/holders/%s", multipathDeviceName), 777)
	assert.NoError(t, err)

	_, lunErr := fcpClient.GetDeviceInfoForLUN(context.TODO(), hostSessionMap, lunID,
		fcpNodeName, needFS)
	assert.NotNil(t, lunErr, "GetDeviceInfoForLUN returns error")
}

func TestClient_getDeviceInfoForLUN_noLUN(t *testing.T) {
	fcpNodeName := "fcpNode"
	multipathDeviceName := "dm-0"
	deviceName := "sda"
	devicePath := "/dev/" + deviceName

	hostSessionMap := []map[string]int{{"a": 0}, {}}
	lunID := 0
	needFS := true

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return(nil, nil)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})
	err := fs.Mkdir(fmt.Sprintf("/sys/block/sda/holders/%s", multipathDeviceName), 777)
	assert.NoError(t, err)

	_, lunErr := fcpClient.GetDeviceInfoForLUN(context.TODO(), hostSessionMap, lunID,
		fcpNodeName, needFS)
	assert.NotNil(t, lunErr, "GetDeviceInfoForLUN returns nil error")
}

func TestClient_getDeviceInfoForLUN_getLUN(t *testing.T) {
	fcpNodeName := "fcpNode"
	multipathDeviceName := "dm-0"
	deviceName := "sda"
	devicePath := "/dev/" + deviceName

	hostSessionMap := []map[string]int{{"a": 0}, {}}
	lunID := 0
	needFS := true

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return(nil, errors.New("some error"))

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})
	err := fs.Mkdir(fmt.Sprintf("/sys/block/sda/holders/%s", multipathDeviceName), 777)
	assert.NoError(t, err)

	_, lunErr := fcpClient.GetDeviceInfoForLUN(context.TODO(), hostSessionMap, lunID,
		fcpNodeName, needFS)
	assert.NotNil(t, lunErr, "GetDeviceInfoForLUN returns nil error")
}

func TestClient_getDeviceInfoForLUN_noMultiPath(t *testing.T) {
	fcpNodeName := "fcpNode"
	deviceName := "sda"
	devicePath := "/dev/" + deviceName

	hostSessionMap := []map[string]int{{"a": 0}, {}}
	lunID := 0
	needFS := false

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return([]string{deviceName}, nil)

	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("")

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	_, lunErr := fcpClient.GetDeviceInfoForLUN(context.TODO(), hostSessionMap, lunID,
		fcpNodeName, needFS)
	assert.NoError(t, lunErr, "GetDeviceInfoForLUN returns error")
}

func TestClient_getDeviceInfoForLUN_errMultipath(t *testing.T) {
	fcpNodeName := "fcpNode"
	multipathDeviceName := "dm-0"
	multipathDevicePath := "/dev/" + multipathDeviceName
	deviceName := "sda"
	devicePath := "/dev/" + deviceName

	hostSessionMap := []map[string]int{{"a": 0}, {}}
	lunID := 0
	needFS := true

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return([]string{deviceName}, nil)

	mockDevices.EXPECT().EnsureDeviceReadable(context.TODO(), multipathDevicePath).Return(errors.New("some error"))
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")

	err := fs.Mkdir(fmt.Sprintf("/sys/block/sda/holders/%s", multipathDeviceName), 777)
	assert.NoError(t, err)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	_, lunErr := fcpClient.GetDeviceInfoForLUN(context.TODO(), hostSessionMap, lunID,
		fcpNodeName, needFS)
	assert.NotNil(t, lunErr, "GetDeviceInfoForLUN returns nil error")
}

func TestClient_getDeviceInfoForLUN_errFS(t *testing.T) {
	fcpNodeName := "fcpNode"
	multipathDeviceName := "dm-0"
	multipathDevicePath := "/dev/" + multipathDeviceName
	deviceName := "sda"
	devicePath := "/dev/" + deviceName

	hostSessionMap := []map[string]int{{"a": 0}, {}}
	lunID := 0
	needFS := true

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return([]string{deviceName}, nil)

	mockDevices.EXPECT().EnsureDeviceReadable(context.TODO(), multipathDevicePath).Return(nil)
	mockDevices.EXPECT().GetDeviceFSType(context.TODO(), multipathDevicePath).Return("", errors.New("some error"))
	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")

	err := fs.Mkdir(fmt.Sprintf("/sys/block/sda/holders/%s", multipathDeviceName), 777)
	assert.NoError(t, err)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	_, lunErr := fcpClient.GetDeviceInfoForLUN(context.TODO(), hostSessionMap, lunID,
		fcpNodeName, needFS)
	assert.NotNil(t, lunErr, "GetDeviceInfoForLUN returns nil error")
}

func TestClient_waitForDeviceScan_allDevice(t *testing.T) {
	lunID := 0
	fcpNodeName := "fcpNode"
	devicePath1 := "/dev/sda"
	devicePath2 := "/dev/sdb"
	deviceAddresses := make([]models.ScsiDeviceAddress, 0)

	hostSessionMap := []map[string]int{{"a": 0}, {}}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{
		devicePath1,
		devicePath2,
	})

	mockOsClient.EXPECT().PathExists(devicePath1+"/block").Return(true, nil)
	mockOsClient.EXPECT().PathExists(devicePath2+"/block").Return(true, nil)

	mockDevices.EXPECT().ScanTargetLUN(context.TODO(), deviceAddresses)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	err := fcpClient.waitForDeviceScan(context.TODO(), hostSessionMap, lunID, fcpNodeName)
	assert.NoError(t, err, "waitForDeviceScan returns error")
}

func TestClient_waitForDevice_noMapping(t *testing.T) {
	lunID := 0
	fcpNodeName := "fcpNode"
	deviceAddresses := make([]models.ScsiDeviceAddress, 0)

	hostSessionMap := []map[string]int{{"a": 0}, {}}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockCommand.EXPECT().Execute(context.TODO(), gomock.Any(), gomock.Any()).Times(6).Return(nil, nil)

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return(make([]string, 0))

	mockDevices.EXPECT().ScanTargetLUN(context.TODO(), deviceAddresses)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	err := fcpClient.waitForDeviceScan(context.TODO(), hostSessionMap, lunID, fcpNodeName)
	assert.NotNil(t, err, "waitForDeviceScan return error")
}

func TestClient_waitForDevice_someDevice(t *testing.T) {
	lunID := 0
	fcpNodeName := "fcpNode"
	devicePath1 := "/dev/sda"
	devicePath2 := "/dev/sdb"
	devicePath3 := "/dev/sdc"
	deviceAddresses := make([]models.ScsiDeviceAddress, 0)

	hostSessionMap := []map[string]int{{"a": 0}, {}}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{
		devicePath1,
		devicePath2, devicePath3,
	})

	mockOsClient.EXPECT().PathExists(devicePath1+"/block").Return(true, nil)
	mockOsClient.EXPECT().PathExists(devicePath2+"/block").Return(false, nil)
	mockOsClient.EXPECT().PathExists(devicePath3+"/block").Return(false, errors.New("some error"))

	mockDevices.EXPECT().ScanTargetLUN(context.TODO(), deviceAddresses)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	err := fcpClient.waitForDeviceScan(context.TODO(), hostSessionMap, lunID, fcpNodeName)
	assert.NoError(t, err, "waitForDeviceScan return error")
}

func TestClient_waitForDevice_noDevice(t *testing.T) {
	lunID := 0
	fcpNodeName := "fcpNode"
	deviceAddresses := make([]models.ScsiDeviceAddress, 0)

	hostSessionMap := []map[string]int{{"a": 0}, {}}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockCommand.EXPECT().Execute(context.TODO(), "ls", gomock.Any()).Return(nil,
		errors.New("some error")).Times(3)
	mockCommand.EXPECT().Execute(context.TODO(), "lsscsi").Return(nil, errors.New("some error"))
	mockCommand.EXPECT().Execute(context.TODO(), "lsscsi", "-t").Return(nil, errors.New("some error"))
	mockCommand.EXPECT().Execute(context.TODO(), "free").Return(nil, errors.New("some error"))

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return(nil)

	mockDevices.EXPECT().ScanTargetLUN(context.TODO(), deviceAddresses)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	err := fcpClient.waitForDeviceScan(context.TODO(), hostSessionMap, lunID, fcpNodeName)
	assert.NotNil(t, err, "waitForDeviceScan return error")
}

func TestClient_handleInvalidSerials(t *testing.T) {
	lunID := 0
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	vpdpg80Serial := "SYA5GZFJ8G1M905GVH7H"
	devicePath := "/dev/sda"
	expectedSerial := vpdpg80Serial

	hostSessionMap := []map[string]int{{"a": 0}, {}}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	var handlerError error
	mockHandler := func(ctx context.Context, path string) error {
		return handlerError
	}

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})

	mockDevices.EXPECT().GetLunSerial(context.TODO(), devicePath).Return(vpdpg80Serial, nil)

	f, err := fs.Create(fmt.Sprintf("%s/vpd_pg80", devicePath))
	assert.NoError(t, err)
	_, err = f.Write(vpdpg80SerialBytes(vpdpg80Serial))
	assert.NoError(t, err)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	result := fcpClient.handleInvalidSerials(context.TODO(), hostSessionMap, lunID, targetNodeName, expectedSerial,
		mockHandler)
	assert.NoError(t, result, "handleInvalidSerials returns error")
}

func TestClient_handleInvalidSerials_emptySerial(t *testing.T) {
	lunID := 0
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	expectedSerial := ""

	hostSessionMap := []map[string]int{{"a": 0}, {}}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	var handlerError error
	mockHandler := func(ctx context.Context, path string) error {
		return handlerError
	}
	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	result := fcpClient.handleInvalidSerials(context.TODO(), hostSessionMap, lunID, targetNodeName, expectedSerial,
		mockHandler)
	assert.NoError(t, result, "handleInvalidSerials returns error")
}

func TestClient_handleInvalidSerials_errSerialFiles(t *testing.T) {
	lunID := 0
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	vpdpg80Serial := "SYA5GZFJ8G1M905GVH7H"
	devicePath := "/dev/sda"
	expectedSerial := vpdpg80Serial

	hostSessionMap := []map[string]int{{"a": 0}, {}}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	fs = &aferoWrapper{
		openError: errors.New("some error"),
		Fs:        afero.NewMemMapFs(),
	}
	var handlerError error
	mockHandler := func(ctx context.Context, path string) error {
		return handlerError
	}

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})

	mockDevices.EXPECT().GetLunSerial(context.TODO(), devicePath).Return("", fmt.Errorf("mock error"))

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	result := fcpClient.handleInvalidSerials(context.TODO(), hostSessionMap, lunID, targetNodeName, expectedSerial,
		mockHandler)
	assert.NotNil(t, result, "handleInvalidSerials returns error")
}

func TestClient_handleInvalidSerials_nilPath(t *testing.T) {
	lunID := 0
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	vpdpg80Serial := "SYA5GZFJ8G1M905GVH7H"
	devicePath := "/dev/sda"
	expectedSerial := vpdpg80Serial

	hostSessionMap := []map[string]int{{"a": 0}, {}}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	var handlerError error
	mockHandler := func(ctx context.Context, path string) error {
		return handlerError
	}
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{})

	f, err := fs.Create(fmt.Sprintf("%s/vpd_pg80", devicePath))
	assert.NoError(t, err)
	_, err = f.Write(vpdpg80SerialBytes(vpdpg80Serial))
	assert.NoError(t, err)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	result := fcpClient.handleInvalidSerials(context.TODO(), hostSessionMap, lunID, targetNodeName, expectedSerial,
		mockHandler)
	assert.NoError(t, result, "handleInvalidSerials returns error")
}

func TestClient_handleInvalidSerials_serialNotExists(t *testing.T) {
	lunID := 0
	targetNodeName := "10:00:00:00:C9:30:15:6E"
	vpdpg80Serial := "SYA5GZFJ8G1M905GVH7H"
	devicePath := "/dev/sda"
	expectedSerial := vpdpg80Serial

	hostSessionMap := []map[string]int{{"a": 0}, {}}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()
	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
	mockDevices.EXPECT().GetLunSerial(context.TODO(), devicePath).Return(vpdpg80Serial, nil)

	var handlerError error
	mockHandler := func(ctx context.Context, path string) error {
		return handlerError
	}

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	result := fcpClient.handleInvalidSerials(context.TODO(), hostSessionMap, lunID, targetNodeName, expectedSerial,
		mockHandler)
	assert.NoError(t, result, "handleInvalidSerials returns error")
}

func TestClient_waitForMultipathDeviceForLUN(t *testing.T) {
	lunID := 0
	fcpNodeName := "fcp-node"
	deviceName := "sda"
	devicePath := "/dev/" + deviceName
	multipathDeviceName := "dm-0"
	holdersDirectory := "/sys/block/" + deviceName + "/holders/" + multipathDeviceName

	hostSessionMap := []map[string]int{{"a": 0}, {}}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return([]string{deviceName}, nil)

	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("dm-0")

	_, err := fs.Create(holdersDirectory)
	assert.NoError(t, err)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	result := fcpClient.waitForMultipathDeviceForLUN(context.TODO(), hostSessionMap, lunID, fcpNodeName)
	assert.NoError(t, result, "waitForMultipathDeviceForLUN returns error")
}

func TestClient_waitForMultipathDeviceForLUN_noHostMapping(t *testing.T) {
	lunID := 0
	fcpNodeName := "fcp-node"

	hostSessionMap := []map[string]int{{"a": 0}, {}}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return(make([]string, 0))
	mockReconcileUtils.EXPECT().GetDevicesForLUN(gomock.Any()).Return(make([]string, 0), nil)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	result := fcpClient.waitForMultipathDeviceForLUN(context.TODO(), hostSessionMap, lunID, fcpNodeName)
	assert.NotNil(t, result, "waitForMultipathDeviceForLUN returns error")
}

func TestClient_waitForMultipathDeviceForLUN_errorGetLUN(t *testing.T) {
	lunID := 0
	fcpNodeName := "fcp-node"
	deviceName := "sda"
	devicePath := "/dev/" + deviceName

	hostSessionMap := []map[string]int{{"a": 0}, {}}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return(nil, errors.New("some error"))

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	result := fcpClient.waitForMultipathDeviceForLUN(context.TODO(), hostSessionMap, lunID, fcpNodeName)
	assert.NotNil(t, result, "waitForMultipathDeviceForLUN returns error")
}

func TestClient_waitForMultipathDeviceForLUN_errorGetMultiPath(t *testing.T) {
	lunID := 0
	fcpNodeName := "fcp-node"
	deviceName := "sda"
	devicePath := "/dev/" + deviceName

	hostSessionMap := []map[string]int{{"a": 0}, {}}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockReconcileUtils.EXPECT().GetSysfsBlockDirsForLUN(0, gomock.Any()).Return([]string{devicePath})
	mockReconcileUtils.EXPECT().GetDevicesForLUN([]string{devicePath}).Return([]string{deviceName}, nil)

	mockDevices.EXPECT().FindMultipathDeviceForDevice(context.TODO(), "sda").Return("")

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	result := fcpClient.waitForMultipathDeviceForLUN(context.TODO(), hostSessionMap, lunID, fcpNodeName)
	assert.NotNil(t, result, "waitForMultipathDeviceForLUN returns error")
}

func TestClient_verifyMultipathDeviceSerial(t *testing.T) {
	multipathDeviceName := "dm-0"
	vpdpg80Serial := "SYA5GZFJ8G1M905GVH7H"
	lunSerial := vpdpg80Serial

	lunSerialHex := hex.EncodeToString([]byte(vpdpg80Serial))
	multipathdeviceSerial := fmt.Sprintf("mpath-%s", lunSerialHex)

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockDevices.EXPECT().GetMultipathDeviceUUID(multipathDeviceName).Return(multipathdeviceSerial, nil)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	err := fcpClient.verifyMultipathDeviceSerial(context.TODO(), multipathDeviceName, lunSerial)
	assert.NoError(t, err, "verifyMultipathDeviceSerial returns error")
}

func TestClient_verifyMultipathDeviceSerial_emptyLunSerial(t *testing.T) {
	multipathDeviceName := "dm-0"
	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()
	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	err := fcpClient.verifyMultipathDeviceSerial(context.TODO(), multipathDeviceName, "")
	assert.NoError(t, err, "verifyMultipathDeviceSerial returns error")
}

func TestClient_verifyMultipathDeviceSerial_multiPathNotPresent(t *testing.T) {
	multipathDeviceName := "dm-0"
	vpdpg80Serial := "SYA5GZFJ8G1M905GVH7H"
	lunSerial := vpdpg80Serial

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockDevices.EXPECT().GetMultipathDeviceUUID(multipathDeviceName).Return("",
		errors.NotFoundError("not found"))

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	err := fcpClient.verifyMultipathDeviceSerial(context.TODO(), multipathDeviceName, lunSerial)
	assert.NoError(t, err, "verifyMultipathDeviceSerial returns error")
}

func TestClient_verifyMultipathDeviceSerial_errorGetMultipath(t *testing.T) {
	multipathDeviceName := "dm-0"
	vpdpg80Serial := "SYA5GZFJ8G1M905GVH7H"
	lunSerial := vpdpg80Serial

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockDevices.EXPECT().GetMultipathDeviceUUID(multipathDeviceName).Return("",
		errors.New("some error"))

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	err := fcpClient.verifyMultipathDeviceSerial(context.TODO(), multipathDeviceName, lunSerial)
	assert.NotNil(t, err, "verifyMultipathDeviceSerial returns error")
}

func TestClient_verifyMultipathDeviceSerial_lunNotPresent(t *testing.T) {
	multipathDeviceName := "dm-0"
	vpdpg80Serial := "SYA5GZFJ8G1M905GVH7H"
	lunSerial := vpdpg80Serial

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockDevices.EXPECT().GetMultipathDeviceUUID(multipathDeviceName).Return("foo", nil)

	fcpClient := NewDetailed("", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	err := fcpClient.verifyMultipathDeviceSerial(context.TODO(), multipathDeviceName, lunSerial)
	assert.NotNil(t, err, "verifyMultipathDeviceSerial returns error")
}

func TestPrepareDeviceForRemoval(t *testing.T) {
	publishInfo := &mockPublushInfo
	allPublishInfos := []models.VolumePublishInfo{}
	ignoreErrors := false
	force := false
	deviceInfo := &models.ScsiDeviceInfo{
		MultipathDevice: "dm-0",
		Devices:         []string{"sda", "sdb"},
	}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockDevices.EXPECT().VerifyMultipathDevice(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return(true, nil)
	mockDevices.EXPECT().ListAllDevices(gomock.Any()).Times(2)
	mockDevices.EXPECT().MultipathFlushDevice(gomock.Any(), gomock.Any()).Return(nil)
	mockDevices.EXPECT().FlushDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockDevices.EXPECT().RemoveDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockDevices.EXPECT().WaitForDevicesRemoval(gomock.Any(), DevPrefix, gomock.Any(),
		devicesRemovalMaxWaitTime).Return(nil)

	fcpClient := NewDetailed("/host", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	_, err := fcpClient.PrepareDeviceForRemoval(
		context.TODO(),
		deviceInfo,
		publishInfo,
		allPublishInfos,
		ignoreErrors,
		force,
	)
	assert.Nil(t, err, "PrepareDeviceForRemoval returns error")
}

func TestPrepareDeviceForRemoval_deferrefDeviceRemoval(t *testing.T) {
	publishInfo := &mockPublushInfo
	allPublishInfos := []models.VolumePublishInfo{}
	ignoreErrors := false
	force := true
	deviceInfo := &models.ScsiDeviceInfo{
		MultipathDevice: "dm-0",
		Devices:         []string{"sda", "sdb"},
	}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockDevices.EXPECT().VerifyMultipathDevice(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return(true, nil)
	mockDevices.EXPECT().ListAllDevices(gomock.Any()).Times(2)
	mockDevices.EXPECT().RemoveDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockDevices.EXPECT().WaitForDevicesRemoval(gomock.Any(), DevPrefix, gomock.Any(),
		devicesRemovalMaxWaitTime).Return(nil)

	fcpClient := NewDetailed("/host", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	_, err := fcpClient.PrepareDeviceForRemoval(
		context.TODO(),
		deviceInfo,
		publishInfo,
		allPublishInfos,
		ignoreErrors,
		force,
	)
	assert.Nil(t, err, "PrepareDeviceForRemoval returns error")
}

func TestPrepareDeviceForRemoval_VerifyMpathError(t *testing.T) {
	publishInfo := &mockPublushInfo
	allPublishInfos := []models.VolumePublishInfo{}
	ignoreErrors := false
	force := false
	deviceInfo := &models.ScsiDeviceInfo{
		MultipathDevice: "dm-0",
		Devices:         []string{"sda", "sdb"},
	}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockDevices.EXPECT().VerifyMultipathDevice(gomock.Any(), gomock.Any(), gomock.Any(),
		gomock.Any()).Return(true, fmt.Errorf("error"))

	fcpClient := NewDetailed("/host", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	_, err := fcpClient.PrepareDeviceForRemoval(
		context.TODO(),
		deviceInfo,
		publishInfo,
		allPublishInfos,
		ignoreErrors,
		force,
	)
	assert.NotNil(t, err, "PrepareDeviceForRemoval returns nil error")
}

func TestRemoveSCSIDevice_TimeOutError(t *testing.T) {
	ignoreErrors := false
	deviceInfo := &models.ScsiDeviceInfo{
		MultipathDevice: "dm-0",
		Devices:         []string{"sda", "sdb"},
	}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockDevices.EXPECT().ListAllDevices(gomock.Any()).Times(2)
	mockDevices.EXPECT().MultipathFlushDevice(gomock.Any(), gomock.Any()).Return(errors.TimeoutError("timeout"))
	mockDevices.EXPECT().FlushDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockDevices.EXPECT().RemoveDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockDevices.EXPECT().WaitForDevicesRemoval(gomock.Any(), DevPrefix, gomock.Any(),
		devicesRemovalMaxWaitTime).Return(nil)

	fcpClient := NewDetailed("/host", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	_, err := fcpClient.removeSCSIDevice(context.TODO(), deviceInfo, ignoreErrors, false)

	assert.Nil(t, err, "removeSCSIDevice returns error")
}

func TestGetFindMultipathValue(t *testing.T) {
	testCases := map[string]struct {
		input       string
		expected    string
		description string
	}{
		"Default value": {
			input:       multipathConf,
			expected:    "no",
			description: "Default value",
		},
		"Commented out": {
			input:       strings.ReplaceAll(multipathConf, "find_multipaths", "#find_multipaths"),
			expected:    "",
			description: "Commented out",
		},
		"Removed line": {
			input:       strings.ReplaceAll(multipathConf, "find_multipaths no", ""),
			expected:    "",
			description: "Removed line",
		},
		"Replaced with 'yes'": {
			input:       strings.ReplaceAll(multipathConf, "no", "yes"),
			expected:    "yes",
			description: "Replaced with 'yes'",
		},
		"Replaced with 'yes' in quotes": {
			input:       strings.ReplaceAll(multipathConf, "no", "'yes'"),
			expected:    "yes",
			description: "Replaced with 'yes' in quotes",
		},
		"Replaced with 'on' in quotes": {
			input:       strings.ReplaceAll(multipathConf, "no", "'on'"),
			expected:    "yes",
			description: "Replaced with 'on' in quotes",
		},
		// Add more test cases here
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			findMultipathsValue := getFindMultipathValue(tc.input)
			assert.Equal(t, tc.expected, findMultipathsValue, tc.description)
		})
	}
}

func TestRemoveSCSIDevice_multiPathFlushFailed(t *testing.T) {
	ignoreErrors := false
	deviceInfo := &models.ScsiDeviceInfo{
		MultipathDevice: "dm-0",
		Devices:         []string{"sda", "sdb"},
	}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockDevices.EXPECT().ListAllDevices(gomock.Any())
	mockDevices.EXPECT().MultipathFlushDevice(gomock.Any(), gomock.Any()).Return(fmt.Errorf("error"))

	fcpClient := NewDetailed("/host", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	_, err := fcpClient.removeSCSIDevice(context.TODO(), deviceInfo, ignoreErrors, false)

	assert.NotNil(t, err, "removeSCSIDevice returns nil error")
}

func TestRemoveSCSIDevice_deviceFlushFailed(t *testing.T) {
	ignoreErrors := false
	deviceInfo := &models.ScsiDeviceInfo{
		MultipathDevice: "dm-0",
		Devices:         []string{"sda", "sdb"},
	}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockDevices.EXPECT().ListAllDevices(gomock.Any())
	mockDevices.EXPECT().MultipathFlushDevice(gomock.Any(), gomock.Any()).Return(nil)
	mockDevices.EXPECT().FlushDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("error"))

	fcpClient := NewDetailed("/host", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	_, err := fcpClient.removeSCSIDevice(context.TODO(), deviceInfo, ignoreErrors, false)

	assert.NotNil(t, err, "removeSCSIDevice returns nil error")
}

func TestRemoveSCSIDevice_RemoveDeviceFailed(t *testing.T) {
	ignoreErrors := false
	deviceInfo := &models.ScsiDeviceInfo{
		MultipathDevice: "dm-0",
		Devices:         []string{"sda", "sdb"},
	}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockDevices.EXPECT().ListAllDevices(gomock.Any())
	mockDevices.EXPECT().MultipathFlushDevice(gomock.Any(), gomock.Any())
	mockDevices.EXPECT().FlushDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockDevices.EXPECT().RemoveDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("error"))

	fcpClient := NewDetailed("/host", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	_, err := fcpClient.removeSCSIDevice(context.TODO(), deviceInfo, ignoreErrors, false)

	assert.NotNil(t, err, "removeSCSIDevice returns nil error")
}

func TestRemoveSCSIDevice_WaitForRemovalFailed(t *testing.T) {
	ignoreErrors := false
	deviceInfo := &models.ScsiDeviceInfo{
		MultipathDevice: "dm-0",
		Devices:         []string{"sda", "sdb"},
	}

	mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, _, fs := NewDrivers()

	mockDevices.EXPECT().ListAllDevices(gomock.Any())
	mockDevices.EXPECT().MultipathFlushDevice(gomock.Any(), gomock.Any())
	mockDevices.EXPECT().FlushDevice(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockDevices.EXPECT().RemoveDevice(gomock.Any(), gomock.Any(), gomock.Any())
	mockDevices.EXPECT().WaitForDevicesRemoval(gomock.Any(), DevPrefix, gomock.Any(),
		devicesRemovalMaxWaitTime).Return(fmt.Errorf("error"))

	fcpClient := NewDetailed("/host", mockCommand, DefaultSelfHealingExclusion,
		mockOsClient, mockDevices, mockFileSystem,
		mockMount, mockReconcileUtils, afero.Afero{Fs: fs})

	_, err := fcpClient.removeSCSIDevice(context.TODO(), deviceInfo, ignoreErrors, false)

	assert.NotNil(t, err, "removeSCSIDevice returns nil error")
}
