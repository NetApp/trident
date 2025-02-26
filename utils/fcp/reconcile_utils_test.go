package fcp

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/mocks/mock_utils/mock_fcp"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/osutils"
)

func TestReconcileFCPVolumeInfo(t *testing.T) {
	volumeTrackingInfo := models.VolumeTrackingInfo{
		VolumePublishInfo: models.VolumePublishInfo{
			VolumeAccessInfo: models.VolumeAccessInfo{
				FCPAccessInfo: models.FCPAccessInfo{
					FibreChannelAccessInfo: models.FibreChannelAccessInfo{
						FCTargetWWNN: "asdfubiasd",
					},
					FCPLunNumber: 12312,
				},
			},
		},
	}
	osClient := osutils.New()
	client := NewReconcileUtils("", osClient)
	_, err := client.ReconcileFCPVolumeInfo(context.TODO(), &volumeTrackingInfo)
	assert.Nil(t, err, "ReconcileFCPVolumeInfo failed, no LUN found")
}

func TestReconcileFCPVolumeInfo_NoDeviceFound(t *testing.T) {
	volumeTrackingInfo := models.VolumeTrackingInfo{
		VolumePublishInfo: models.VolumePublishInfo{
			VolumeAccessInfo: models.VolumeAccessInfo{
				FCPAccessInfo: models.FCPAccessInfo{
					FibreChannelAccessInfo: models.FibreChannelAccessInfo{
						FCTargetWWNN: "asdfubiasd",
					},
					FCPLunNumber: 12312,
				},
			},
		},
	}

	osClient := osutils.New()
	err := os.MkdirAll("/tmp/sys/class/fc_remote_ports", 0o755)
	assert.NoError(t, err)
	defer os.RemoveAll("/tmp/sys/class/fc_remote_ports/rport_test")

	client := NewReconcileUtils("/tmp", osClient)
	_, reconclineErr := client.ReconcileFCPVolumeInfo(context.TODO(), &volumeTrackingInfo)
	assert.Nil(t, reconclineErr, "ReconcileFCPVolumeInfo failed, no LUN found")
}

func TestReconcileFCPVolumeInfo_DevicesNotNil(t *testing.T) {
	volumeTrackingInfo := models.VolumeTrackingInfo{
		VolumePublishInfo: models.VolumePublishInfo{
			VolumeAccessInfo: models.VolumeAccessInfo{
				FCPAccessInfo: models.FCPAccessInfo{
					FibreChannelAccessInfo: models.FibreChannelAccessInfo{
						FCTargetWWNN: "asdfubiasd",
					},
					FCPLunNumber: 12312,
				},
			},
		},
	}

	osClient := osutils.New()
	err := os.MkdirAll("/tmp/sys/class/fc_remote_ports/rport_test", 0o755)
	assert.NoError(t, err)
	defer os.RemoveAll("/tmp/sys/class/fc_remote_ports/rport_test")

	_, err = os.Create("/tmp/sys/class/fc_remote_ports/rport_test/node_name.txt")
	assert.NoError(t, err)

	client := NewReconcileUtils("/tmp", osClient)
	_, reconclineErr := client.ReconcileFCPVolumeInfo(context.TODO(), &volumeTrackingInfo)
	assert.Nil(t, reconclineErr, "ReconcileFCPVolumeInfo failed, no LUN found")
}

func TestGetDevicesForLUN_PathExists(t *testing.T) {
	paths := []string{"/tmp"}
	controller := gomock.NewController(t)

	filePath := "/tmp/block"
	_, err := os.Create(filePath)
	assert.NoError(t, err)
	defer os.RemoveAll(filePath)
	mockOsClient := mock_fcp.NewMockOS(controller)
	mockOsClient.EXPECT().PathExists("/tmp"+"/block").Return(true, nil)

	client := NewReconcileUtils("tmp", mockOsClient)
	_, getLunErr := client.GetDevicesForLUN(paths)
	assert.Nil(t, getLunErr, "GetDevicesForLUN failed")
}

func TestGetDevicesForLUN_PathDoesNotExist(t *testing.T) {
	controller := gomock.NewController(t)
	paths := []string{"/tmp"}
	mockOsClient := mock_fcp.NewMockOS(controller)
	mockOsClient.EXPECT().PathExists("/tmp"+"/block").Return(false, errors.New("ut-err"))

	client := NewReconcileUtils("tmp", mockOsClient)
	_, getLunErr := client.GetDevicesForLUN(paths)
	assert.Nil(t, getLunErr, "GetDevicesForLUN failed")
}

func TestGetDevicesForLUN_NilPath(t *testing.T) {
	paths := []string{}
	client := NewReconcileUtils("tmp", nil)
	_, getLunErr := client.GetDevicesForLUN(paths)
	assert.Nil(t, getLunErr, "GetDevicesForLUN Failed")
}

func TestGetDevicesForLUNError_EmptyPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}
	paths := []string{"/tmp"}
	controller := gomock.NewController(t)

	mockOsClient := mock_fcp.NewMockOS(controller)
	mockOsClient.EXPECT().PathExists("/tmp"+"/block").Return(true, nil)

	client := NewReconcileUtils("tmp", mockOsClient)

	_, getLunErr := client.GetDevicesForLUN(paths)
	assert.NotNil(t, getLunErr, "GetDevicesForLUN returns nil error")
}

func TestGetDevicesForLUN_EmptyFileAtPath(t *testing.T) {
	paths := []string{"/tmp"}
	controller := gomock.NewController(t)

	filePath := "/tmp/block"
	_, err := os.Create(filePath)
	assert.NoError(t, err)
	defer os.RemoveAll(filePath)

	mockOsClient := mock_fcp.NewMockOS(controller)
	mockOsClient.EXPECT().PathExists("/tmp"+"/block").Return(true, nil)

	client := NewReconcileUtils("tmp", mockOsClient)

	_, getLunErr := client.GetDevicesForLUN(paths)
	assert.Nil(t, getLunErr, "GetDevicesForLUN returns error")
}

func TestCheckZoningExistsWithTarget(t *testing.T) {
	osClient := osutils.New()
	err := os.MkdirAll("/tmp/sys/class/fc_remote_ports/rport_test", 0o755)
	assert.NoError(t, err)
	defer os.RemoveAll("/tmp/sys/class/fc_remote_ports/rport_test")

	f, err := os.Create("/tmp/sys/class/fc_remote_ports/rport_test/node_name")
	assert.NoError(t, err)

	_, err = f.WriteString("testNode")
	assert.NoError(t, err)

	client := NewReconcileUtils("/tmp", osClient)
	_, checkZoningErr := client.CheckZoningExistsWithTarget(context.TODO(), "testNode")
	assert.NotNil(t, checkZoningErr, "CheckZoningExistsWithTarget returns error")
}

func TestCheckZoningExistsWithTarget_DifferentNodeNameAtPath(t *testing.T) {
	osClient := osutils.New()
	err := os.MkdirAll("/tmp/sys/class/fc_remote_ports/rport_test", 0o755)
	assert.NoError(t, err)
	defer os.RemoveAll("/tmp/sys/class/fc_remote_ports/rport_test")

	f, err := os.Create("/tmp/sys/class/fc_remote_ports/rport_test/node_name")
	assert.NoError(t, err)

	_, err = f.WriteString("content")
	assert.NoError(t, err)

	client := NewReconcileUtils("/tmp", osClient)
	_, checkZoningErr := client.CheckZoningExistsWithTarget(context.TODO(), "testNode")
	assert.NotNil(t, checkZoningErr, "CheckZoningExistsWithTarget returns nil error")
}

func TestCheckZoningExistsWithTarget_NodeFileDoesNotExist(t *testing.T) {
	osClient := osutils.New()
	err := os.MkdirAll("/tmp/sys/class/fc_remote_ports/rport_test", 0o755)
	assert.NoError(t, err)

	defer os.RemoveAll("/tmp/sys/class/fc_remote_ports/rport_test")

	f, err := os.Create("/tmp/sys/class/fc_remote_ports/rport_test/port_state")
	assert.NoError(t, err)

	_, err = f.WriteString("content")
	assert.NoError(t, err)

	client := NewReconcileUtils("/tmp", osClient)
	_, checkZoningErr := client.CheckZoningExistsWithTarget(context.TODO(), "testNode")
	assert.NotNil(t, checkZoningErr, "CheckZoningExistsWithTarget returns nil error")
}

func TestCheckZoningExistsWithTarget_ZoningExists(t *testing.T) {
	osClient := osutils.New()
	err := os.MkdirAll("/tmp/sys/class/fc_remote_ports/rport_test", 0o755)
	assert.NoError(t, err)
	defer os.RemoveAll("/tmp/sys/class/fc_remote_ports/rport_test")

	f1, err := os.Create("/tmp/sys/class/fc_remote_ports/rport_test/node_name")
	assert.NoError(t, err)

	_, err = f1.WriteString("testNode")
	assert.NoError(t, err)

	f, err := os.Create("/tmp/sys/class/fc_remote_ports/rport_test/port_state")
	assert.NoError(t, err)

	_, err = f.WriteString("Online")
	assert.NoError(t, err)

	client := NewReconcileUtils("/tmp", osClient)
	_, checkZoningErr := client.CheckZoningExistsWithTarget(context.TODO(), "testNode")
	assert.Nil(t, checkZoningErr, "CheckZoningExistsWithTarget returns error")
}

func TestCheckZoningExistsWithTarget_PathDoesNotExist(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}
	osClient := osutils.New()
	os.RemoveAll("/tmp/sys/class/fc_remote_ports/")
	client := NewReconcileUtils("/tmp", osClient)
	_, err := client.CheckZoningExistsWithTarget(context.TODO(), "testNode")
	assert.NotNil(t, err)
}

func TestCheckZoningExistsWithTargetError_WrongFileAtPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	osClient := osutils.New()
	err := os.MkdirAll("/tmp/sys/class/fc_remote_ports/pport_test", 0o755)
	assert.NoError(t, err)
	defer os.RemoveAll("/tmp/sys/class/fc_remote_ports/pport_test")

	client := NewReconcileUtils("/tmp", osClient)

	_, checkZoningErr := client.CheckZoningExistsWithTarget(context.TODO(), "testNode")
	assert.NotNil(t, checkZoningErr, "CheckZoningExistsWithTarget returns nil error")
}

func TestGetSysfsBlockDirsForLUN(t *testing.T) {
	hostSessionMap := []map[string]int{{"rport-12:abc": 0}, {}, {"rport-a:abc": 0}, {}}

	osClient := osutils.New()
	client := NewReconcileUtils("/tmp", osClient)
	result := client.GetSysfsBlockDirsForLUN(0, hostSessionMap)
	assert.NotNil(t, result, "GetSysfsBlockDirsForLUN result is nil")
}

func TestGetFCPHostSessionMapForTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	osClient := osutils.New()
	err := os.MkdirAll("/tmp/sys/class/fc_remote_ports/rport-12:a/test", 0o755)
	assert.NoError(t, err)

	err = os.MkdirAll("/tmp/sys/class/fc_host/host12/device/rport-12:a", 0o755)
	assert.NoError(t, err)

	defer os.RemoveAll("/tmp/sys/class/fc_remote_ports/rport-12:a")
	defer os.RemoveAll("/tmp/sys/class/fc_host/host12/device/rport-12:a")

	f1, err := os.Create("/tmp/sys/class/fc_remote_ports/rport-12:a/node_name")
	assert.NoError(t, err)

	f2, err := os.Create("/tmp/sys/class/fc_host/host12/device/rport-12:a/target")
	assert.NoError(t, err)

	_, err = f1.WriteString("testNodeName")
	assert.NoError(t, err)

	_, err = f2.WriteString("target")
	assert.NoError(t, err)

	client := NewReconcileUtils("/tmp", osClient)

	result := client.GetFCPHostSessionMapForTarget(context.TODO(), "testNodeName")
	assert.NotNil(t, result, "GetFCPHostSessionMapForTarget result is nil")
}

func TestGetFCPHostSessionMapForTarget_DifferentNameAtTargetPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	osClient := osutils.New()
	err := os.MkdirAll("/tmp/sys/class/fc_remote_ports/rport-12:a/test", 0o755)
	assert.NoError(t, err)

	err = os.MkdirAll("/tmp/sys/class/fc_host/host12/device/rport-12:a", 0o755)
	assert.NoError(t, err)

	defer os.RemoveAll("/tmp/sys/class/fc_remote_ports/rport-12:a")
	defer os.RemoveAll("/tmp/sys/class/fc_host/host12/device/rport-12:a")

	f1, err := os.Create("/tmp/sys/class/fc_remote_ports/rport-12:a/node_name")
	assert.NoError(t, err)

	f2, err := os.Create("/tmp/sys/class/fc_host/host12/device/rport-12:a/node")
	assert.NoError(t, err)

	_, err = f1.WriteString("testNodeName")
	assert.NoError(t, err)

	_, err = f2.WriteString("target")
	assert.NoError(t, err)

	client := NewReconcileUtils("/tmp", osClient)

	result := client.GetFCPHostSessionMapForTarget(context.TODO(), "testNodeName")
	assert.NotNil(t, result, "GetFCPHostSessionMapForTarget result is nil")
}

func TestGetFCPHostSessionMapForTarget_PortNotInt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}

	osClient := osutils.New()
	err := os.MkdirAll("/tmp/sys/class/fc_remote_ports/rport-12:a/test", 0o755)
	assert.NoError(t, err)

	err = os.MkdirAll("/tmp/sys/class/fc_host/host12/device/rport-12:a", 0o755)
	assert.NoError(t, err)
	defer os.RemoveAll("/tmp/sys/class/fc_remote_ports/rport-12:a")
	defer os.RemoveAll("/tmp/sys/class/fc_host/host12/device/rport-12:a")

	f1, err := os.Create("/tmp/sys/class/fc_remote_ports/rport-12:a/node_name")
	assert.NoError(t, err)

	f2, err := os.Create("/tmp/sys/class/fc_host/host12/device/rport-12:a/target:te:st")
	assert.NoError(t, err)

	_, err = f1.WriteString("testNodeName")
	assert.NoError(t, err)

	_, err = f2.WriteString("target")
	assert.NoError(t, err, "")

	client := NewReconcileUtils("/tmp", osClient)

	result := client.GetFCPHostSessionMapForTarget(context.TODO(), "testNodeName")
	assert.NotNil(t, result, "GetFCPHostSessionMapForTarget result is Nil")
}
