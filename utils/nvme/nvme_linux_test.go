package nvme

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_devices"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/mocks/mock_utils/mock_filesystem"
	"github.com/netapp/trident/mocks/mock_utils/mock_mount"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/mount"
)

var (
	mockNqn        = "nqn.1992-08.com.netapp:sn.2f3c5176e28711ec8545d039ea1dc5ad:subsystem.ubuntu-linux-22-04-02-deskt-8fd4ddef-9f86-4004-aab1-6d6961542088"
	mockNqnAddress = "traddr=10.193.156.237,trsvcid=4420"
)

func TestGetHostNQN(t *testing.T) {
	// Test1: Success - Able to get Host NQN
	expectedNqn := "fakeNQN"
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCommand.EXPECT().Execute(ctx, "cat", "/etc/nvme/hostnqn").Return([]byte("fakeNQN"), nil)

	handler := NewNVMeHandlerDetailed(mockCommand, nil, nil, nil, nil)

	gotNqn, err := handler.GetHostNqn(ctx)

	assert.Equal(t, expectedNqn, gotNqn)
	assert.NoError(t, err)

	// Test2: Error - Unable to get Host NQN
	expectedNqn = ""
	mockCommand.EXPECT().Execute(ctx, "cat", "/etc/nvme/hostnqn").Return([]byte("fakeNQN"),
		errors.New("Error getting host NQN"))

	gotNqn, err = handler.GetHostNqn(ctx)

	assert.Equal(t, expectedNqn, gotNqn)
	assert.Error(t, err)
}

func TestNVMeActiveOnHost(t *testing.T) {
	// Test1: Success - NVMe is active on host
	expectedValue := true
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "version").Return([]byte(""), nil)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "lsmod", NVMeListCmdTimeoutInSeconds*time.Second,
		false).Return([]byte("nvme_tcp"), nil)

	handler := NewNVMeHandlerDetailed(mockCommand, nil, nil, nil, nil)
	gotValue, err := handler.NVMeActiveOnHost(ctx)

	assert.Equal(t, expectedValue, gotValue)
	assert.NoError(t, err)

	// Test2: Error - NVMe cli is not installed on host
	expectedValue = false
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "version").Return([]byte(""), errors.New("NVMe CLI not installed"))

	gotValue, err = handler.NVMeActiveOnHost(ctx)

	assert.Equal(t, expectedValue, gotValue)
	assert.Error(t, err)

	// Test3: Error - Unable to get driver info
	expectedValue = false
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "version").Return([]byte(""), nil)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "lsmod", NVMeListCmdTimeoutInSeconds*time.Second,
		false).Return([]byte(""), errors.New("error getting NVMe driver info"))

	gotValue, err = handler.NVMeActiveOnHost(ctx)

	assert.Equal(t, expectedValue, gotValue)
	assert.Error(t, err)

	// Test4: Error - NVMe/tcp module not loaded on the host
	expectedValue = false
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "version").Return([]byte(""), nil)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "lsmod", NVMeListCmdTimeoutInSeconds*time.Second,
		false).Return([]byte(""), nil)

	gotValue, err = handler.NVMeActiveOnHost(ctx)

	assert.Equal(t, expectedValue, gotValue)
	assert.Error(t, err)
}

func TestGetNVMeSubsystemListRHEL(t *testing.T) {
	// Test1: Success - Able to get NVMe subsystem list
	expectedSubystem := []Subsystems{
		{
			[]NVMeSubsystem{
				{
					Name: "fakeName",
					NQN:  "fakeNqn",
				},
			},
		},
	}
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	bytesBuffer := new(bytes.Buffer)
	json.NewEncoder(bytesBuffer).Encode(expectedSubystem)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-subsys", "-o", "json").Return(bytesBuffer.Bytes(), nil)

	gotValue, err := GetNVMeSubsystemList(ctx, mockCommand)

	assert.Equal(t, expectedSubystem[0], gotValue)
	assert.NoError(t, err)

	// Test2: Error - Unable to get subsystem list
	expectedSubystem = []Subsystems{
		{},
	}
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-subsys", "-o", "json").Return([]byte(""), errors.New("Error getting NVMe subsystems"))

	_, err = GetNVMeSubsystemList(ctx, mockCommand)

	assert.Error(t, err)

	// Test3: Success - No subsystem present
	expectedSubystem = []Subsystems{}

	bytesBuffer = new(bytes.Buffer)
	json.NewEncoder(bytesBuffer).Encode(expectedSubystem)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-subsys", "-o", "json").Return(bytesBuffer.Bytes(), nil)

	_, err = GetNVMeSubsystemList(ctx, mockCommand)

	assert.NoError(t, err)

	// Test4: Error - Valid json but not mapping to subsystem
	expectedVal := []string{`{"some":"json"}`, `{"foo":"bar"}`}
	bytesBuffer = new(bytes.Buffer)
	json.NewEncoder(bytesBuffer).Encode(expectedVal)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-subsys", "-o", "json").Return(bytesBuffer.Bytes(), nil)

	_, err = GetNVMeSubsystemList(ctx, mockCommand)

	assert.Error(t, err)
}

func TestGetNVMeSubsystemList(t *testing.T) {
	// Test1: Success - Able to get NVMe subsystem list
	expectedSubystem := Subsystems{
		Subsystems: []NVMeSubsystem{
			{
				Name: "fakeName",
				NQN:  "fakeNQN",
			},
		},
	}
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	bytesBuffer := new(bytes.Buffer)
	json.NewEncoder(bytesBuffer).Encode(expectedSubystem)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-subsys", "-o", "json").Return(bytesBuffer.Bytes(), nil)

	gotValue, err := GetNVMeSubsystemList(ctx, mockCommand)

	assert.Equal(t, expectedSubystem, gotValue)
	assert.NoError(t, err)

	// Test2: Error - Valid json but not mapping to subsystem
	expectedVal := `{"some":"json"}`
	bytesBuffer = new(bytes.Buffer)
	json.NewEncoder(bytesBuffer).Encode(expectedVal)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-subsys", "-o", "json").Return(bytesBuffer.Bytes(), nil)

	gotValue, err = GetNVMeSubsystemList(ctx, mockCommand)

	assert.Error(t, err)
}

func TestConnectSubsystemToHost(t *testing.T) {
	// Test1: Success - Able to connect to subsystem
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCommand.EXPECT().Execute(ctx, "nvme", "connect", "-t", "tcp", "-n", gomock.Any(),
		"-a", gomock.Any(), "-s", "4420", "-l", "-1").Return([]byte(""), nil)
	subsystem := NewNVMeSubsystem("fakeNqn", mockCommand, nil)
	err := subsystem.ConnectSubsystemToHost(ctx, "fakeDataLif")

	assert.NoError(t, err)

	// Test2: Error - Unable to connect to subsystem
	mockCommand.EXPECT().Execute(ctx, "nvme", "connect", "-t", "tcp", "-n", gomock.Any(),
		"-a", gomock.Any(), "-s", "4420", "-l", "-1").Return([]byte(""), errors.New("Error connecting to subsystem"))
	err = subsystem.ConnectSubsystemToHost(ctx, "fakeDataLif")

	assert.Error(t, err)
}

func TestDisconnectSubsystemFromHost(t *testing.T) {
	// Test1: Success - Able to disconnect from subsystem
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCommand.EXPECT().Execute(ctx, "nvme", "disconnect",
		"-n", gomock.Any()).Return([]byte(""), nil)

	subsystem := NewNVMeSubsystem("fakeNqn", mockCommand, nil)
	err := subsystem.Disconnect(ctx)

	assert.NoError(t, err)

	// Test2: Error - Unable to disconnect from subsystem
	mockCommand.EXPECT().Execute(ctx, "nvme", "disconnect",
		"-n", gomock.Any()).Return([]byte(""), errors.New("Error disconnecting subsytem"))

	err = subsystem.Disconnect(ctx)

	assert.Error(t, err)
}

func TestGetNamespaceCountForSubsDevice(t *testing.T) {
	// Test1: Success - Able to get namespace count
	ctx := context.Background()
	expectedCount := 2
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-ns", gomock.Any()).Return([]byte("[], []"), nil)

	subsys := NewNVMeSubsystemDetailed("fakeNqn", "fakeSubsysDevice", nil, mockCommand, nil)
	gotCount, err := subsys.GetNamespaceCountForSubsDevice(ctx)

	assert.Equal(t, expectedCount, gotCount)
	assert.NoError(t, err)

	// Test2: Error - Unable to get namespace count
	expectedCount = 0
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-ns", gomock.Any()).Return([]byte(""), errors.New("Error getting namespace count"))

	gotCount, err = subsys.GetNamespaceCountForSubsDevice(ctx)

	assert.Equal(t, expectedCount, gotCount)
	assert.Error(t, err)
}

func getMockNvmeDevices() NVMeDevices {
	return NVMeDevices{
		Devices: []NVMeDevice{
			{
				Device: "fakeDevice",
				UUID:   "fakeUUID",
			},
		},
	}
}

func TestFlushNVMeDevice(t *testing.T) {
	tests := map[string]struct {
		ignoreErrors   bool
		force          bool
		getMockCommand func(ctrl *gomock.Controller) exec.Command
		expectErr      bool
	}{
		"Flush success": {
			ignoreErrors: false,
			force:        false,
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCmd := mockexec.NewMockCommand(ctrl)
				mockCmd.EXPECT().Execute(gomock.Any(), "nvme", "flush", gomock.Any()).Return([]byte(""), nil)
				return mockCmd
			},
			expectErr: false,
		},
		"Flush error": {
			ignoreErrors: false,
			force:        false,
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCmd := mockexec.NewMockCommand(ctrl)
				mockCmd.EXPECT().Execute(context.Background(), "nvme", "flush", gomock.Any()).Return([]byte(""),
					errors.New("Error flushing NVMe device"))
				return mockCmd
			},
			expectErr: true,
		},
		"Force": {
			ignoreErrors: false,
			force:        true,
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				return mockexec.NewMockCommand(ctrl)
			},
			expectErr: false,
		},
		"Ignore errors": {
			ignoreErrors: true,
			force:        false,
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCmd := mockexec.NewMockCommand(ctrl)
				mockCmd.EXPECT().Execute(context.Background(), "nvme", "flush", gomock.Any()).Return([]byte(""),
					errors.New("Error flushing NVMe device"))
				return mockCmd
			},
			expectErr: false,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			device := NVMeDevice{
				Device:  "fakeDevice",
				command: params.getMockCommand(ctrl),
			}
			err := device.FlushDevice(context.Background(), params.ignoreErrors, params.force)
			if params.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func createMockNvmeSubsystem(fs afero.Fs) error {
	subsysPath := NVME_PATH + "/nvme-subsys0"
	nvmePath := subsysPath + "/nvme1"
	fs.MkdirAll(nvmePath, 0o755)
	nvmePath2 := subsysPath + "/nvme0n1"
	fs.MkdirAll(nvmePath2, 0o755)

	nqnFilePath := subsysPath + SUBSYSNQN
	fileContent := []byte(mockNqn)
	err := afero.WriteFile(fs, nqnFilePath, fileContent, 0o644)
	if err != nil {
		return err
	}

	filePath := nvmePath + "/state"
	fileContent = []byte("live")
	err = afero.WriteFile(fs, filePath, fileContent, 0o644)
	if err != nil {
		return err
	}

	filePath = nvmePath + "/transport"
	fileContent = []byte("tcp")
	err = afero.WriteFile(fs, filePath, fileContent, 0o644)
	if err != nil {
		return err
	}

	filePath = nvmePath + "/address"
	fileContent = []byte(mockNqnAddress)
	err = afero.WriteFile(fs, filePath, fileContent, 0o644)
	if err != nil {
		return err
	}

	filePath = nvmePath2 + "/uuid"
	fileContent = []byte("1234")
	err = afero.WriteFile(fs, filePath, fileContent, 0o644)
	if err != nil {
		return err
	}

	return nil
}

func TestNewNVMeSubsystem(t *testing.T) {
	tests := map[string]struct {
		subsNqn  string
		getFs    func(ctrl *gomock.Controller) (afero.Fs, error)
		expected NVMeSubsystem
	}{
		"Subsystem found": {
			subsNqn: mockNqn,
			expected: NVMeSubsystem{
				NQN:  mockNqn,
				Name: "/sys/class/nvme-subsystem/nvme-subsys0",
				Paths: []Path{
					{
						Name:      "/sys/class/nvme-subsystem/nvme-subsys0/nvme1",
						Address:   mockNqnAddress,
						State:     "live",
						Transport: "tcp",
					},
				},
			},
			getFs: func(ctrl *gomock.Controller) (afero.Fs, error) {
				fs := afero.NewMemMapFs()
				err := createMockNvmeSubsystem(fs)
				if err != nil {
					return nil, err
				}
				return fs, err
			},
		},
		"Subsystem not found": {
			subsNqn: "notFound",
			expected: NVMeSubsystem{
				NQN:   "notFound",
				Name:  "",
				Paths: []Path{},
			},
			getFs: func(ctrl *gomock.Controller) (afero.Fs, error) {
				return afero.NewMemMapFs(), nil
			},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			fs, err := params.getFs(ctrl)
			assert.NoError(t, err)
			handler := NewNVMeHandlerDetailed(nil, nil, nil, nil, fs)
			subSystem := handler.NewNVMeSubsystem(context.Background(), params.subsNqn)
			s, ok := subSystem.(*NVMeSubsystem)
			assert.Equal(t, true, ok)
			assert.Equal(t, params.expected.Name, s.Name)
			assert.Equal(t, params.expected.Paths, s.Paths)
			assert.Equal(t, params.expected.NQN, s.NQN)
		})
	}
}

func TestAttachNVMeVolumeRetry(t *testing.T) {
	tests := map[string]struct {
		name        string
		mountpoint  string
		publishInfo *models.VolumePublishInfo
		secrets     map[string]string
		timeout     time.Duration
		getFs       func() (afero.Fs, error)
		expectErr   bool
	}{
		"Attach success on retry": {
			name:       "/sys/class/nvme-subsystem/subsystem0/nvme123n456",
			mountpoint: "/mock/mountpoint",
			publishInfo: &models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				VolumeAccessInfo: models.VolumeAccessInfo{
					NVMeAccessInfo: models.NVMeAccessInfo{
						NVMeSubsystemNQN:  mockNqn,
						NVMeSubsystemUUID: "1234",
						NVMeNamespaceUUID: "1234",
					},
				},
			},
			secrets: map[string]string{},
			timeout: 5 * time.Second,
			getFs: func() (afero.Fs, error) {
				fs := afero.NewMemMapFs()
				err := createMockNvmeSubsystem(fs)
				if err != nil {
					return nil, err
				}
				return fs, nil
			},
			expectErr: false,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			fs, err := params.getFs()
			assert.NoError(t, err)
			handler := NewNVMeHandlerDetailed(nil, nil, nil, nil, fs)
			err = handler.AttachNVMeVolumeRetry(context.Background(), params.publishInfo, params.timeout)
			if params.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEnsureVolumeFormattedAndMounted(t *testing.T) {
	tests := map[string]struct {
		name           string
		mountpoint     string
		publishInfo    *models.VolumePublishInfo
		luksFormatted  bool
		getMockCommand func(ctrl *gomock.Controller) exec.Command
		getMockDevices func(ctrl *gomock.Controller) devices.Devices
		getMockMount   func(ctrl *gomock.Controller) mount.Mount
		getFsClient    func(ctrl *gomock.Controller) filesystem.Filesystem
		expectErr      bool
	}{
		"Mount success, formatted": {
			name:       "mockName",
			mountpoint: "/mock/mountpoint",
			publishInfo: &models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				DevicePath:     "/dev/mock-device",
				VolumeAccessInfo: models.VolumeAccessInfo{
					NVMeAccessInfo: models.NVMeAccessInfo{
						NVMeSubsystemNQN:  "mock-nqn",
						NVMeSubsystemUUID: "1234",
						NVMeNamespaceUUID: "1234",
					},
				},
			},
			luksFormatted: false,
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				return mockexec.NewMockCommand(ctrl)
			},
			getMockDevices: func(ctrl *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(ctrl)
				mockDevices.EXPECT().GetDeviceFSType(gomock.Any(), "/dev/mock-device").Return("", nil)
				mockDevices.EXPECT().IsDeviceUnformatted(gomock.Any(), "/dev/mock-device").Return(true, nil)
				return mockDevices
			},
			getMockMount: func(ctrl *gomock.Controller) mount.Mount {
				mockMount := mock_mount.NewMockMount(ctrl)
				mockMount.EXPECT().IsMounted(gomock.Any(), "/dev/mock-device", "", "").Return(true, nil)
				mockMount.EXPECT().MountDevice(gomock.Any(), "/dev/mock-device", "/mock/mountpoint", "", false).Return(nil)
				return mockMount
			},
			getFsClient: func(ctrl *gomock.Controller) filesystem.Filesystem {
				fsClient := mock_filesystem.NewMockFilesystem(ctrl)
				fsClient.EXPECT().FormatVolume(gomock.Any(), "/dev/mock-device", filesystem.Ext4, "").Return(nil)
				return fsClient
			},
			expectErr: false,
		},
		"Mount success with LUKS formatted device": {
			name:       "mockName",
			mountpoint: "/mock/mountpoint",
			publishInfo: &models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				LUKSEncryption: "true",
				DevicePath:     "/dev/mock-device",
				VolumeAccessInfo: models.VolumeAccessInfo{
					NVMeAccessInfo: models.NVMeAccessInfo{
						NVMeSubsystemNQN:  "mock-nqn",
						NVMeSubsystemUUID: "1234",
						NVMeNamespaceUUID: "1234",
					},
				},
			},
			luksFormatted: true,
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				return mockexec.NewMockCommand(ctrl)
			},
			getMockDevices: func(ctrl *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(ctrl)
				mockDevices.EXPECT().GetDeviceFSType(gomock.Any(), "/dev/mapper/luks-mockName").Return(filesystem.Ext4, nil)
				return mockDevices
			},
			getMockMount: func(ctrl *gomock.Controller) mount.Mount {
				mockMount := mock_mount.NewMockMount(ctrl)
				mockMount.EXPECT().IsMounted(gomock.Any(), "/dev/mapper/luks-mockName", "", "").Return(true, nil)
				mockMount.EXPECT().MountDevice(gomock.Any(), "/dev/mapper/luks-mockName", "/mock/mountpoint", "", false).Return(nil)
				return mockMount
			},
			getFsClient: func(ctrl *gomock.Controller) filesystem.Filesystem {
				return mock_filesystem.NewMockFilesystem(ctrl)
			},
			expectErr: false,
		},
		"Mount failure unknown format": {
			name:       "mockName",
			mountpoint: "/mock/mountpoint",
			publishInfo: &models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				DevicePath:     "/dev/mock-device",
				VolumeAccessInfo: models.VolumeAccessInfo{
					NVMeAccessInfo: models.NVMeAccessInfo{
						NVMeSubsystemNQN:  "mock-nqn",
						NVMeSubsystemUUID: "1234",
						NVMeNamespaceUUID: "1234",
					},
				},
			},
			luksFormatted: false,
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				return mockexec.NewMockCommand(ctrl)
			},
			getMockDevices: func(ctrl *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(ctrl)
				mockDevices.EXPECT().GetDeviceFSType(gomock.Any(), "/dev/mock-device").Return("", nil)
				mockDevices.EXPECT().IsDeviceUnformatted(gomock.Any(), "/dev/mock-device").Return(true, errors.New("error"))
				return mockDevices
			},
			getMockMount: func(ctrl *gomock.Controller) mount.Mount {
				return mock_mount.NewMockMount(ctrl)
			},
			getFsClient: func(ctrl *gomock.Controller) filesystem.Filesystem {
				return mock_filesystem.NewMockFilesystem(ctrl)
			},
			expectErr: true,
		},
		"Mount failure not formatted": {
			name:       "mockName",
			mountpoint: "/mock/mountpoint",
			publishInfo: &models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				DevicePath:     "/dev/mock-device",
				VolumeAccessInfo: models.VolumeAccessInfo{
					NVMeAccessInfo: models.NVMeAccessInfo{
						NVMeSubsystemNQN:  "mock-nqn",
						NVMeSubsystemUUID: "1234",
						NVMeNamespaceUUID: "1234",
					},
				},
			},
			luksFormatted: false,
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				return mockexec.NewMockCommand(ctrl)
			},
			getMockDevices: func(ctrl *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(ctrl)
				mockDevices.EXPECT().GetDeviceFSType(gomock.Any(), "/dev/mock-device").Return("", nil)
				mockDevices.EXPECT().IsDeviceUnformatted(gomock.Any(), "/dev/mock-device").Return(false, nil)
				return mockDevices
			},
			getMockMount: func(ctrl *gomock.Controller) mount.Mount {
				return mock_mount.NewMockMount(ctrl)
			},
			getFsClient: func(ctrl *gomock.Controller) filesystem.Filesystem {
				return mock_filesystem.NewMockFilesystem(ctrl)
			},
			expectErr: true,
		},
		"Mount failure already formated unexpected type": {
			name:       "mockName",
			mountpoint: "/mock/mountpoint",
			publishInfo: &models.VolumePublishInfo{
				FilesystemType: filesystem.Ext3,
				DevicePath:     "/dev/mock-device",
				VolumeAccessInfo: models.VolumeAccessInfo{
					NVMeAccessInfo: models.NVMeAccessInfo{
						NVMeSubsystemNQN:  "mock-nqn",
						NVMeSubsystemUUID: "1234",
						NVMeNamespaceUUID: "1234",
					},
				},
			},
			luksFormatted: false,
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				return mockexec.NewMockCommand(ctrl)
			},
			getMockDevices: func(ctrl *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(ctrl)
				mockDevices.EXPECT().GetDeviceFSType(gomock.Any(), "/dev/mock-device").Return(filesystem.Ext4, nil)
				return mockDevices
			},
			getMockMount: func(ctrl *gomock.Controller) mount.Mount {
				return mock_mount.NewMockMount(ctrl)
			},
			getFsClient: func(ctrl *gomock.Controller) filesystem.Filesystem {
				return mock_filesystem.NewMockFilesystem(ctrl)
			},
			expectErr: true,
		},
		"Mount error": {
			name:       "mockName",
			mountpoint: "/mock/mountpoint",
			publishInfo: &models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				DevicePath:     "/dev/mock-device",
				VolumeAccessInfo: models.VolumeAccessInfo{
					NVMeAccessInfo: models.NVMeAccessInfo{
						NVMeSubsystemNQN:  "mock-nqn",
						NVMeSubsystemUUID: "1234",
						NVMeNamespaceUUID: "1234",
					},
				},
			},
			luksFormatted: false,
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				return mockexec.NewMockCommand(ctrl)
			},
			getMockDevices: func(ctrl *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(ctrl)
				mockDevices.EXPECT().GetDeviceFSType(gomock.Any(), "/dev/mock-device").Return("", nil)
				mockDevices.EXPECT().IsDeviceUnformatted(gomock.Any(), "/dev/mock-device").Return(true, nil)
				return mockDevices
			},
			getMockMount: func(ctrl *gomock.Controller) mount.Mount {
				mockMount := mock_mount.NewMockMount(ctrl)
				mockMount.EXPECT().IsMounted(gomock.Any(), "/dev/mock-device", "", "").Return(true, nil)
				mockMount.EXPECT().MountDevice(gomock.Any(), "/dev/mock-device", "/mock/mountpoint", "",
					false).Return(errors.New("error"))
				return mockMount
			},
			getFsClient: func(ctrl *gomock.Controller) filesystem.Filesystem {
				fsClient := mock_filesystem.NewMockFilesystem(ctrl)
				fsClient.EXPECT().FormatVolume(gomock.Any(), "/dev/mock-device", filesystem.Ext4, "").Return(nil)
				return fsClient
			},
			expectErr: true,
		},
		"Raw filesystem - no formatting or mounting": {
			name:       "mockName",
			mountpoint: "/mock/mountpoint",
			publishInfo: &models.VolumePublishInfo{
				FilesystemType: filesystem.Raw,
				DevicePath:     "/dev/mock-device",
				VolumeAccessInfo: models.VolumeAccessInfo{
					NVMeAccessInfo: models.NVMeAccessInfo{
						NVMeSubsystemNQN:  "mock-nqn",
						NVMeSubsystemUUID: "1234",
						NVMeNamespaceUUID: "1234",
					},
				},
			},
			luksFormatted: false,
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				return mockexec.NewMockCommand(ctrl)
			},
			getMockDevices: func(ctrl *gomock.Controller) devices.Devices {
				return mock_devices.NewMockDevices(ctrl)
			},
			getMockMount: func(ctrl *gomock.Controller) mount.Mount {
				return mock_mount.NewMockMount(ctrl)
			},
			getFsClient: func(ctrl *gomock.Controller) filesystem.Filesystem {
				return mock_filesystem.NewMockFilesystem(ctrl)
			},
			expectErr: false,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler := NewNVMeHandlerDetailed(params.getMockCommand(ctrl), params.getMockDevices(ctrl),
				params.getMockMount(ctrl), params.getFsClient(ctrl), nil)
			err := handler.EnsureVolumeFormattedAndMounted(
				context.Background(), params.name, params.mountpoint, params.publishInfo, params.luksFormatted, false,
			)
			if params.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEnsureCryptsetupFormattedAndMappedOnHost(t *testing.T) {
	tests := map[string]struct {
		name               string
		publishInfo        *models.VolumePublishInfo
		secrets            map[string]string
		getMockCommand     func(ctrl *gomock.Controller) exec.Command
		getMockDevices     func(ctrl *gomock.Controller) devices.Devices
		expectSafeToFormat bool
		expectFormatted    bool
		expectErr          bool
	}{
		"LUKS format and map success": {
			name: "mockName",
			publishInfo: &models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				LUKSEncryption: "true",
				DevicePath:     "/dev/mock-device",
				VolumeAccessInfo: models.VolumeAccessInfo{
					NVMeAccessInfo: models.NVMeAccessInfo{
						NVMeSubsystemNQN:  "mock-nqn",
						NVMeSubsystemUUID: "1234",
						NVMeNamespaceUUID: "1234",
					},
				},
			},
			secrets: map[string]string{
				"luks-passphrase":      "mockPassphrase",
				"luks-passphrase-name": "mockPassphraseName",
			},
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().ExecuteWithTimeoutAndInput(
					gomock.Any(), gomock.Eq("cryptsetup"), gomock.Any(), gomock.Any(), gomock.Eq(""), "status", gomock.Any()).
					Return([]byte{}, mockexec.NewMockExitError(4, "device does not exist")).Times(1)
				mockCommand.EXPECT().ExecuteWithTimeoutAndInput(
					gomock.Any(), gomock.Eq("cryptsetup"), gomock.Any(), gomock.Any(), gomock.Any(), "isLuks", gomock.Any()).
					Return([]byte{}, mockexec.NewMockExitError(1, "not a luks device")).Times(1)
				mockCommand.EXPECT().ExecuteWithTimeoutAndInput(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), "luksFormat", gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
				).Return([]byte{}, nil).Times(1)
				mockCommand.EXPECT().ExecuteWithTimeoutAndInput(
					gomock.Any(), gomock.Eq("cryptsetup"), gomock.Any(), gomock.Any(), gomock.Any(), "isLuks", gomock.Any()).
					Return([]byte{}, nil).Times(1)
				mockCommand.EXPECT().ExecuteWithTimeoutAndInput(
					gomock.Any(), gomock.Eq("cryptsetup"), gomock.Any(), gomock.Any(), gomock.Any(), "open", gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]byte{}, nil).Times(1)
				return mockCommand
			},
			getMockDevices: func(ctrl *gomock.Controller) devices.Devices {
				mockDevices := mock_devices.NewMockDevices(ctrl)
				mockDevices.EXPECT().IsDeviceUnformatted(gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
				return mockDevices
			},
			expectSafeToFormat: true,
			expectFormatted:    true,
			expectErr:          false,
		},
		"LUKS already formatted and mapped": {
			name: "mockName",
			publishInfo: &models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				LUKSEncryption: "true",
				DevicePath:     "/dev/mock-device",
				VolumeAccessInfo: models.VolumeAccessInfo{
					NVMeAccessInfo: models.NVMeAccessInfo{
						NVMeSubsystemNQN:  "mock-nqn",
						NVMeSubsystemUUID: "1234",
						NVMeNamespaceUUID: "1234",
					},
				},
			},
			secrets: map[string]string{
				"luks-passphrase":      "mockPassphrase",
				"luks-passphrase-name": "mockPassphraseName",
			},
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().ExecuteWithTimeoutAndInput(
					gomock.Any(), gomock.Eq("cryptsetup"), gomock.Any(), gomock.Any(), gomock.Eq(""), "status", gomock.Any()).
					Return([]byte{}, mockexec.NewMockExitError(4, "device does not exist")).Times(1)
				mockCommand.EXPECT().ExecuteWithTimeoutAndInput(
					gomock.Any(), gomock.Eq("cryptsetup"), gomock.Any(), gomock.Any(), gomock.Any(), "isLuks", gomock.Any()).
					Return([]byte{}, nil).Times(1)
				mockCommand.EXPECT().ExecuteWithTimeoutAndInput(
					gomock.Any(), gomock.Eq("cryptsetup"), gomock.Any(), gomock.Any(), gomock.Any(), "open", gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return([]byte{}, nil).Times(1)
				return mockCommand
			},
			getMockDevices: func(ctrl *gomock.Controller) devices.Devices {
				return mock_devices.NewMockDevices(ctrl)
			},
			expectSafeToFormat: false,
			expectFormatted:    true,
			expectErr:          false,
		},
		"Non-LUKS device returns false": {
			name: "mockName",
			publishInfo: &models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				LUKSEncryption: "false",
				DevicePath:     "/dev/mock-device",
				VolumeAccessInfo: models.VolumeAccessInfo{
					NVMeAccessInfo: models.NVMeAccessInfo{
						NVMeSubsystemNQN:  "mock-nqn",
						NVMeSubsystemUUID: "1234",
						NVMeNamespaceUUID: "1234",
					},
				},
			},
			secrets: map[string]string{},
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				return mockexec.NewMockCommand(ctrl)
			},
			getMockDevices: func(ctrl *gomock.Controller) devices.Devices {
				return mock_devices.NewMockDevices(ctrl)
			},
			expectSafeToFormat: false,
			expectFormatted:    false,
			expectErr:          false,
		},
		"LUKS with invalid encryption value": {
			name: "mockName",
			publishInfo: &models.VolumePublishInfo{
				FilesystemType: filesystem.Ext4,
				LUKSEncryption: "invalid",
				DevicePath:     "/dev/mock-device",
				VolumeAccessInfo: models.VolumeAccessInfo{
					NVMeAccessInfo: models.NVMeAccessInfo{
						NVMeSubsystemNQN:  "mock-nqn",
						NVMeSubsystemUUID: "1234",
						NVMeNamespaceUUID: "1234",
					},
				},
			},
			secrets: map[string]string{},
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				return mockexec.NewMockCommand(ctrl)
			},
			getMockDevices: func(ctrl *gomock.Controller) devices.Devices {
				return mock_devices.NewMockDevices(ctrl)
			},
			expectSafeToFormat: false,
			expectFormatted:    false,
			expectErr:          true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			handler := NewNVMeHandlerDetailed(params.getMockCommand(ctrl), params.getMockDevices(ctrl),
				nil, nil, nil)
			luksFormatted, safeToFormat, err := handler.EnsureCryptsetupFormattedAndMappedOnHost(context.Background(),
				params.name, params.publishInfo, params.secrets)

			assert.Equal(t, params.expectSafeToFormat, safeToFormat)
			assert.Equal(t, params.expectFormatted, luksFormatted)
			if params.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConnect(t *testing.T) {
	tests := map[string]struct {
		nvmeTargetIps  []string
		getMockCommand func(ctrl *gomock.Controller) exec.Command
		getFs          func() (afero.Fs, error)
		paths          []Path
		connectOnly    bool
		expectErr      bool
	}{
		"Connect only happy path": {
			nvmeTargetIps: []string{"10.193.108.74", "10.193.108.75"},
			paths: []Path{
				{Address: "traddr=10.193.108.74,trsvcid=4420"},
				{Address: "traddr=10.193.108.75,trsvcid=4420"},
			},
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCommand := mockexec.NewMockCommand(ctrl)
				return mockCommand
			},
			getFs: func() (afero.Fs, error) {
				return afero.NewMemMapFs(), nil
			},
			connectOnly: true,
			expectErr:   false,
		},
		"Fail to connect": {
			nvmeTargetIps: []string{"1.2.3.4"},
			paths: []Path{
				{Address: "traddr=10.193.108.74,trsvcid=4420"},
			},
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().Execute(gomock.Any(), "nvme", "connect", "-t", "tcp", "-n", mockNqn,
					"-a", "1.2.3.4", "-s", "4420", "-l", "-1").Return([]byte{}, errors.New("error"))
				return mockCommand
			},
			getFs: func() (afero.Fs, error) {
				return afero.NewMemMapFs(), nil
			},
			connectOnly: true,
			expectErr:   true,
		},
		"Connect and update RHEL": {
			nvmeTargetIps: []string{"10.193.108.74"},
			paths:         []Path{},
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().Execute(gomock.Any(), "nvme", "connect", "-t", "tcp", "-n", mockNqn,
					"-a", "10.193.108.74", "-s", "4420", "-l", "-1").Return([]byte{}, nil)
				return mockCommand
			},
			getFs: func() (afero.Fs, error) {
				fs := afero.NewMemMapFs()
				err := createMockNvmeSubsystem(fs)
				return fs, err
			},
			connectOnly: false,
			expectErr:   false,
		},
		"Connect and update Ubuntu": {
			nvmeTargetIps: []string{"10.193.108.74"},
			paths:         []Path{},
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().Execute(gomock.Any(), "nvme", "connect", "-t", "tcp", "-n", mockNqn,
					"-a", "10.193.108.74", "-s", "4420", "-l", "-1").Return([]byte{}, nil)
				return mockCommand
			},
			getFs: func() (afero.Fs, error) {
				fs := afero.NewMemMapFs()
				err := createMockNvmeSubsystem(fs)
				return fs, err
			},
			connectOnly: false,
			expectErr:   false,
		},
		"Connect and update error": {
			nvmeTargetIps: []string{"10.193.108.74"},
			paths:         []Path{},
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCommand := mockexec.NewMockCommand(ctrl)
				mockCommand.EXPECT().Execute(gomock.Any(), "nvme", "connect", "-t", "tcp", "-n", mockNqn,
					"-a", "10.193.108.74", "-s", "4420", "-l", "-1").Return([]byte{}, errors.New("error"))
				return mockCommand
			},
			getFs: func() (afero.Fs, error) {
				fs := afero.NewMemMapFs()
				err := createMockNvmeSubsystem(fs)
				return fs, err
			},
			connectOnly: false,
			expectErr:   true,
		},
		"Partially connected": {
			nvmeTargetIps: []string{"10.193.108.74", "10.193.108.75"},
			paths: []Path{
				{Address: "traddr=10.193.108.74,trsvcid=4420"},
				{Address: "traddr=10.193.108.75,trsvcid=4420"},
			},
			getMockCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCommand := mockexec.NewMockCommand(ctrl)
				return mockCommand
			},
			getFs: func() (afero.Fs, error) {
				return afero.NewMemMapFs(), nil
			},
			connectOnly: false,
			expectErr:   false,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			fs, err := params.getFs()
			assert.NoError(t, err)
			subsystem := NewNVMeSubsystemDetailed(mockNqn, "/sys/class/nvme-subsystem/nvme-subsys0", params.paths, params.getMockCommand(ctrl),
				fs)
			err = subsystem.Connect(context.Background(), params.nvmeTargetIps, params.connectOnly)
			if params.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestListSubsystemsFromSysFs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	osFs := afero.NewMemMapFs()
	handler := NewNVMeHandlerDetailed(nil, nil, nil, nil, osFs)

	_, err := handler.listSubsystemsFromSysFs(context.Background())
	assert.Error(t, err)

	filePath := "/sys/class/nvme-subsystem/nvme1/subsysnqn"
	fileContent := []byte("This is a test file")
	err = afero.WriteFile(osFs, filePath, fileContent, 0o644)
	if err != nil {
		t.Errorf("Failed to create test file: %v", err)
	}

	_, err = handler.listSubsystemsFromSysFs(context.Background())
	assert.Nil(t, err)
}

func TestGetNVMeSubsystem(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	osFs := afero.NewMemMapFs()
	handler := NewNVMeHandlerDetailed(nil, nil, nil, nil, osFs)

	_, err := handler.GetNVMeSubsystem(context.Background(), "nqn")
	assert.NotNil(t, err)

	filePath := "/sys/class/nvme-subsystem/"
	fileContent := []byte("This is a test file")
	err = afero.WriteFile(osFs, filePath, fileContent, 0o644)
	if err != nil {
		t.Errorf("Failed to create test file: %v", err)
	}

	_, err = handler.GetNVMeSubsystem(context.Background(), "/sys/class/nvme-subsystem")
	assert.NotNil(t, err)
}

func TestGetNVMeSubsystemPaths(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	osFs := afero.NewMemMapFs()

	filePath := "/sys/class/nvme-subsystem/test"
	fileContent := []byte("This is a test file")
	err := afero.WriteFile(osFs, filePath, fileContent, 0o644)
	if err != nil {
		t.Errorf("Failed to create test file: %v", err)
	}
	_, err = GetNVMeSubsystemPaths(context.Background(), osFs, "/sys/class/nvme-subsystem")
	assert.Nil(t, err)
}

func TestGetNVMeDeviceCountAt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	osFs := afero.NewMemMapFs()
	filePath := "/sys/class/nvme-subsystem/test"
	fileContent := []byte("This is a test file")
	err := afero.WriteFile(osFs, filePath, fileContent, 0o644)
	if err != nil {
		t.Errorf("Failed to create test file: %v", err)
	}
	subsys := NewNVMeSubsystemDetailed("mock-nqn", "mock-name", []Path{{Address: "mock-address"}}, nil, osFs)
	_, err = subsys.GetNVMeDeviceCountAt(context.Background(), "transport")
	assert.NotNil(t, err)
}

func TestGetNamespaceCount(t *testing.T) {
	tests := map[string]struct {
		getFs       func() (afero.Fs, error)
		paths       []Path
		expectCount int
		expectErr   bool
	}{
		"Happy path": {
			getFs: func() (afero.Fs, error) {
				fs := afero.NewMemMapFs()
				err := createMockNvmeSubsystem(fs)
				return fs, err
			},
			paths: []Path{
				{
					Address: "traddr=10.193.108.74,trsvcid=4420",
					State:   "live",
				},
			},
			expectCount: 1,
			expectErr:   false,
		},
		"Path not live": {
			getFs: func() (afero.Fs, error) {
				fs := afero.NewMemMapFs()
				err := createMockNvmeSubsystem(fs)
				return fs, err
			},
			paths: []Path{
				{
					Address: "traddr=10.193.108.74,trsvcid=4420",
					State:   "disconnected",
				},
			},
			expectCount: 0,
			expectErr:   true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			fs, err := params.getFs()
			assert.NoError(t, err)
			subsystem := NewNVMeSubsystemDetailed(mockNqn, "/sys/class/nvme-subsystem/nvme-subsys0", params.paths,
				nil, fs)
			count, err := subsystem.GetNamespaceCount(context.Background())
			if params.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, params.expectCount, count)
		})
	}
}

func TestPopulateCurrentNVMeSessions(t *testing.T) {
	tests := map[string]struct {
		getFs           func() (afero.Fs, error)
		currSessions    *NVMeSessions
		key             string
		expectKeyExists bool
		expectErr       bool
	}{
		"Happy path": {
			getFs: func() (afero.Fs, error) {
				fs := afero.NewMemMapFs()
				err := createMockNvmeSubsystem(fs)
				return fs, err
			},
			currSessions:    &NVMeSessions{},
			key:             mockNqn,
			expectKeyExists: true,
			expectErr:       false,
		},
		"Subsystem not found": {
			getFs: func() (afero.Fs, error) {
				return afero.NewMemMapFs(), nil
			},
			currSessions:    &NVMeSessions{},
			key:             mockNqn,
			expectKeyExists: false,
			expectErr:       true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			fs, err := params.getFs()
			assert.NoError(t, err)
			handler := NewNVMeHandlerDetailed(nil, nil, nil, nil, fs)
			err = handler.PopulateCurrentNVMeSessions(context.Background(), params.currSessions)
			if params.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			_, exists := params.currSessions.Info[params.key]
			assert.Equal(t, params.expectKeyExists, exists)
		})
	}
}

func TestUpdateNVMeSubsystemPathAttributes_NegTests(t *testing.T) {
	tests := map[string]struct {
		getFs     func() (afero.Fs, error)
		path      *Path
		expectErr bool
	}{
		"Nil Path": {
			getFs: func() (afero.Fs, error) {
				return afero.NewMemMapFs(), nil
			},
			path:      nil,
			expectErr: true,
		},
		"Missing NVMe state file": {
			getFs: func() (afero.Fs, error) {
				fs := afero.NewMemMapFs()
				err := createMockNvmeSubsystem(fs)
				fs.Remove("/sys/class/nvme-subsystem/nvme-subsys0/nvme1/state")
				return fs, err
			},
			path:      &Path{Name: "/sys/class/nvme-subsystem/nvme-subsys0/nvme1"},
			expectErr: true,
		},
		"Missing NVMe address file": {
			getFs: func() (afero.Fs, error) {
				fs := afero.NewMemMapFs()
				err := createMockNvmeSubsystem(fs)
				fs.Remove("/sys/class/nvme-subsystem/nvme-subsys0/nvme1/address")
				return fs, err
			},
			path:      &Path{Name: "/sys/class/nvme-subsystem/nvme-subsys0/nvme1"},
			expectErr: true,
		},
		"Missing NVMe transport file": {
			getFs: func() (afero.Fs, error) {
				fs := afero.NewMemMapFs()
				err := createMockNvmeSubsystem(fs)
				fs.Remove("/sys/class/nvme-subsystem/nvme-subsys0/nvme1/transport")
				return fs, err
			},
			path:      &Path{Name: "/sys/class/nvme-subsystem/nvme-subsys0/nvme1"},
			expectErr: true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			fs, err := params.getFs()
			assert.NoError(t, err)
			err = updateNVMeSubsystemPathAttributes(context.Background(), fs, params.path)
			if params.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
