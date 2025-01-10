//go:build linux

package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/filesystem"
)

func TestGetHostNQN(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	// Test1: Success - Able to get Host NQN
	expectedNqn := "fakeNQN"
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCommand.EXPECT().Execute(ctx, "cat", "/etc/nvme/hostnqn").Return([]byte("fakeNQN"), nil)
	command = mockCommand

	gotNqn, err := GetHostNqn(ctx)

	assert.Equal(t, expectedNqn, gotNqn)
	assert.NoError(t, err)

	// Test2: Error - Unable to get Host NQN
	expectedNqn = ""
	mockCommand.EXPECT().Execute(ctx, "cat", "/etc/nvme/hostnqn").Return([]byte("fakeNQN"),
		fmt.Errorf("Error getting host NQN"))
	command = mockCommand

	gotNqn, err = GetHostNqn(ctx)

	assert.Equal(t, expectedNqn, gotNqn)
	assert.Error(t, err)
}

func TestNVMeActiveOnHost(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	// Test1: Success - NVMe is active on host
	expectedValue := true
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "version").Return([]byte(""), nil)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "lsmod", NVMeListCmdTimeoutInSeconds*time.Second,
		false).Return([]byte("nvme_tcp"), nil)
	command = mockCommand

	gotValue, err := NVMeActiveOnHost(ctx)

	assert.Equal(t, expectedValue, gotValue)
	assert.NoError(t, err)

	// Test2: Error - NVMe cli is not installed on host
	expectedValue = false
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "version").Return([]byte(""), fmt.Errorf("NVMe CLI not installed"))
	command = mockCommand

	gotValue, err = NVMeActiveOnHost(ctx)

	assert.Equal(t, expectedValue, gotValue)
	assert.Error(t, err)

	// Test3: Error - Unable to get driver info
	expectedValue = false
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "version").Return([]byte(""), nil)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "lsmod", NVMeListCmdTimeoutInSeconds*time.Second,
		false).Return([]byte(""), fmt.Errorf("error getting NVMe driver info"))
	command = mockCommand

	gotValue, err = NVMeActiveOnHost(ctx)

	assert.Equal(t, expectedValue, gotValue)
	assert.Error(t, err)

	// Test4: Error - NVMe/tcp module not loaded on the host
	expectedValue = false
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "version").Return([]byte(""), nil)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "lsmod", NVMeListCmdTimeoutInSeconds*time.Second,
		false).Return([]byte(""), nil)
	command = mockCommand

	gotValue, err = NVMeActiveOnHost(ctx)

	assert.Equal(t, expectedValue, gotValue)
	assert.Error(t, err)
}

func TestGetNVMeSubsystemListRHEL(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

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
	command = mockCommand

	gotValue, err := GetNVMeSubsystemList(ctx)

	assert.Equal(t, expectedSubystem[0], gotValue)
	assert.NoError(t, err)

	// Test2: Error - Unable to get subsystem list
	expectedSubystem = []Subsystems{
		{},
	}
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-subsys", "-o", "json").Return([]byte(""), fmt.Errorf("Error getting NVMe subsystems"))
	command = mockCommand

	_, err = GetNVMeSubsystemList(ctx)

	assert.Error(t, err)

	// Test3: Success - No subsystem present
	expectedSubystem = []Subsystems{}

	bytesBuffer = new(bytes.Buffer)
	json.NewEncoder(bytesBuffer).Encode(expectedSubystem)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-subsys", "-o", "json").Return(bytesBuffer.Bytes(), nil)
	command = mockCommand

	_, err = GetNVMeSubsystemList(ctx)

	assert.NoError(t, err)

	// Test4: Error - Valid json but not mapping to subsystem
	expectedVal := []string{`{"some":"json"}`, `{"foo":"bar"}`}
	bytesBuffer = new(bytes.Buffer)
	json.NewEncoder(bytesBuffer).Encode(expectedVal)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-subsys", "-o", "json").Return(bytesBuffer.Bytes(), nil)
	command = mockCommand

	_, err = GetNVMeSubsystemList(ctx)

	assert.Error(t, err)
}

func TestGetNVMeSubsystemList(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

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
	command = mockCommand

	gotValue, err := GetNVMeSubsystemList(ctx)

	assert.Equal(t, expectedSubystem, gotValue)
	assert.NoError(t, err)

	// Test2: Error - Valid json but not mapping to subsystem
	expectedVal := `{"some":"json"}`
	bytesBuffer = new(bytes.Buffer)
	json.NewEncoder(bytesBuffer).Encode(expectedVal)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-subsys", "-o", "json").Return(bytesBuffer.Bytes(), nil)
	command = mockCommand

	gotValue, err = GetNVMeSubsystemList(ctx)

	assert.Error(t, err)
}

func TestConnectSubsystemToHost(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	// Test1: Success - Able to connect to subsystem
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCommand.EXPECT().Execute(ctx, "nvme", "connect", "-t", "tcp", "-n", gomock.Any(),
		"-a", gomock.Any(), "-s", "4420", "-l", "-1").Return([]byte(""), nil)
	command = mockCommand
	err := ConnectSubsystemToHost(ctx, "fakeNqn", "fakeDataLif")

	assert.NoError(t, err)

	// Test2: Error - Unable to connect to subsystem
	mockCommand.EXPECT().Execute(ctx, "nvme", "connect", "-t", "tcp", "-n", gomock.Any(),
		"-a", gomock.Any(), "-s", "4420", "-l", "-1").Return([]byte(""), fmt.Errorf("Error connecting to subsystem"))
	command = mockCommand
	err = ConnectSubsystemToHost(ctx, "fakeNqn", "fakeDataLif")

	assert.Error(t, err)
}

func TestDisconnectSubsystemFromHost(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	// Test1: Success - Able to disconnect from subsystem
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCommand.EXPECT().Execute(ctx, "nvme", "disconnect",
		"-n", gomock.Any()).Return([]byte(""), nil)
	command = mockCommand

	err := DisconnectSubsystemFromHost(ctx, "fakeNqn")

	assert.NoError(t, err)

	// Test2: Error - Unable to disconnect from subsystem
	mockCommand.EXPECT().Execute(ctx, "nvme", "disconnect",
		"-n", gomock.Any()).Return([]byte(""), fmt.Errorf("Error disconnecting subsytem"))
	command = mockCommand

	err = DisconnectSubsystemFromHost(ctx, "fakeNqn")

	assert.Error(t, err)
}

func TestGetNamespaceCountForSubsDevice(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	// Test1: Success - Able to get namespace count
	ctx := context.Background()
	expectedCount := 2
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-ns", gomock.Any()).Return([]byte("[], []"), nil)
	command = mockCommand

	gotCount, err := GetNamespaceCountForSubsDevice(ctx, "fakeSubsysDevice")

	assert.Equal(t, expectedCount, gotCount)
	assert.NoError(t, err)

	// Test2: Error - Unable to get namespace count
	expectedCount = 0
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "list-ns", gomock.Any()).Return([]byte(""), fmt.Errorf("Error getting namespace count"))
	command = mockCommand

	gotCount, err = GetNamespaceCountForSubsDevice(ctx, "fakeSubsysDevice")

	assert.Equal(t, expectedCount, gotCount)
	assert.Error(t, err)
}

func TestGetNVMeDeviceList(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	// Test1: Success - Able to get Device list
	expectedNVMeDevices := NVMeDevices{
		Devices: []NVMeDevice{
			{
				Device: "fakeDevice",
				UUID:   "fakeUUID",
			},
		},
	}
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	bytesBuffer := new(bytes.Buffer)
	json.NewEncoder(bytesBuffer).Encode(expectedNVMeDevices)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "netapp", "ontapdevices", "-o", "json").Return(bytesBuffer.Bytes(), nil)
	command = mockCommand

	gotNVMeDevices, err := GetNVMeDeviceList(ctx)

	assert.Equal(t, expectedNVMeDevices, gotNVMeDevices)
	assert.NoError(t, err)

	// Test2: Error - Not able to get Device list
	expectedNVMeDevices = NVMeDevices{}
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "netapp", "ontapdevices", "-o", "json").Return(bytesBuffer.Bytes(), fmt.Errorf("Error getting device list"))
	command = mockCommand

	gotNVMeDevices, err = GetNVMeDeviceList(ctx)

	assert.Equal(t, expectedNVMeDevices, gotNVMeDevices)
	assert.Error(t, err)

	// Test3: Empty output and no error case
	expectedNVMeDevices = NVMeDevices{}
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "netapp", "ontapdevices", "-o", "json").Return([]byte(""), nil)
	command = mockCommand

	gotNVMeDevices, err = GetNVMeDeviceList(ctx)

	assert.Equal(t, expectedNVMeDevices, gotNVMeDevices)
	assert.NoError(t, err)

	// Test4: Error - Valid json but not mapping to NVMe Device
	expectedValue := `{"some":"json"}`
	bytesBuffer = new(bytes.Buffer)
	json.NewEncoder(bytesBuffer).Encode(expectedValue)
	mockCommand.EXPECT().ExecuteWithTimeout(ctx, "nvme", NVMeListCmdTimeoutInSeconds*time.Second,
		false, "netapp", "ontapdevices", "-o", "json").Return(bytesBuffer.Bytes(), nil)
	command = mockCommand

	gotNVMeDevices, err = GetNVMeDeviceList(ctx)

	assert.Error(t, err)
}

func TestFlushNVMeDevice(t *testing.T) {
	defer func(previousCommand exec.Command) {
		command = previousCommand
	}(command)

	// Test1: Success - Able to flush NVMe device
	ctx := context.Background()
	mockCtrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(mockCtrl)
	mockCommand.EXPECT().Execute(ctx, "nvme", "flush", gomock.Any()).Return([]byte(""), nil)
	command = mockCommand

	err := FlushNVMeDevice(ctx, "fakeDevice")

	assert.NoError(t, err)

	// Test2: Error - Unable to flush NVMe device
	mockCommand.EXPECT().Execute(ctx, "nvme", "flush", gomock.Any()).Return([]byte(""), fmt.Errorf("Error flushing NVMe device"))
	command = mockCommand

	err = FlushNVMeDevice(ctx, "fakeDevice")

	assert.Error(t, err)
}

// MockDirEntry is a mock implementation of os.DirEntry.
type MockDirEntry struct {
	name string
}

// Name returns the name of the mock directory entry.
func (m *MockDirEntry) Name() string {
	return m.name
}

// IsDir always returns false for the mock directory entry.
func (m *MockDirEntry) IsDir() bool {
	return false
}

// Type returns the type of the mock directory entry.
func (m *MockDirEntry) Type() fs.FileMode {
	return fs.ModeDir
}

// Info returns the file info of the mock directory entry.
func (m *MockDirEntry) Info() (fs.FileInfo, error) {
	// Return a dummy file info for the mock entry.
	return nil, nil
}

func TestListSubsystemsFromSysFs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	osFs := afero.NewMemMapFs()
	mockFsClient := filesystem.NewDetailed(nil, osFs, nil)
	_, err := listSubsystemsFromSysFs(*mockFsClient, ctx())
	assert.Nil(t, err)

	filePath := "/sys/class/nvme-subsystem/nvme1/subsysnqn"
	fileContent := []byte("This is a test file")
	err = afero.WriteFile(osFs, filePath, fileContent, 0o644)
	if err != nil {
		t.Errorf("Failed to create test file: %v", err)
	}

	_, err = listSubsystemsFromSysFs(*mockFsClient, ctx())
	assert.Nil(t, err)
}

func TestGetNVMeSubsystem(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	osFs := afero.NewMemMapFs()
	mockFsClient := filesystem.NewDetailed(nil, osFs, nil)

	_, err := GetNVMeSubsystem(ctx(), *mockFsClient, "nqn")
	assert.NotNil(t, err)

	filePath := "/sys/class/nvme-subsystem/"
	fileContent := []byte("This is a test file")
	err = afero.WriteFile(osFs, filePath, fileContent, 0o644)
	if err != nil {
		t.Errorf("Failed to create test file: %v", err)
	}

	_, err = GetNVMeSubsystem(ctx(), *mockFsClient, "/sys/class/nvme-subsystem")
	assert.NotNil(t, err)
}

func TestGetNVMeSubsystemPaths(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	osFs := afero.NewMemMapFs()
	mockFsClient := filesystem.NewDetailed(nil, osFs, nil)

	filePath := "/sys/class/nvme-subsystem/test"
	fileContent := []byte("This is a test file")
	err := afero.WriteFile(osFs, filePath, fileContent, 0o644)
	if err != nil {
		t.Errorf("Failed to create test file: %v", err)
	}
	_, err = GetNVMeSubsystemPaths(ctx(), *mockFsClient, "/sys/class/nvme-subsystem")
	assert.Nil(t, err)
}

func TestGetNVMeDeviceCountAt(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	osFs := afero.NewMemMapFs()
	mockFsClient := filesystem.NewDetailed(nil, osFs, nil)
	filePath := "/sys/class/nvme-subsystem/test"
	fileContent := []byte("This is a test file")
	err := afero.WriteFile(osFs, filePath, fileContent, 0o644)
	if err != nil {
		t.Errorf("Failed to create test file: %v", err)
	}
	_, err = GetNVMeDeviceCountAt(ctx(), *mockFsClient, "transport")
	assert.NotNil(t, err)
}
