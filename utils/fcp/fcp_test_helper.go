package fcp

import (
	"fmt"
	"testing"

	"github.com/spf13/afero"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/models"

	"github.com/netapp/trident/mocks/mock_utils/mock_devices"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/mocks/mock_utils/mock_fcp"
	"github.com/netapp/trident/mocks/mock_utils/mock_filesystem"
	"github.com/netapp/trident/mocks/mock_utils/mock_mount"
)

type aferoFileWrapper struct {
	WriteStringError error
	WriteStringCount int
	afero.File
}
type aferoWrapper struct {
	openFileError    error
	openFileResponse afero.File
	openResponse     afero.File
	openError        error
	afero.Fs
}

var mockPublushInfo = models.VolumePublishInfo{
	FilesystemType: filesystem.Ext4,
	VolumeAccessInfo: models.VolumeAccessInfo{
		FCPAccessInfo: models.FCPAccessInfo{
			FCPLunNumber: 1,
			FCPIgroup:    "10:00:00:00:C9:30:15:6E",
		},
	},
}

// Helper Functions
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

func NewDrivers() (*mockexec.MockCommand, *mock_fcp.MockOS, *mock_devices.MockDevices, *mock_filesystem.MockFilesystem,
	*mock_mount.MockMount, *mock_fcp.MockFcpReconcileUtils, *mock_fcp.MockOS,
	afero.Fs,
) {
	// var controller *gomock.Controller
	t := new(testing.T)
	controller := gomock.NewController(t)

	mockCommand := mockexec.NewMockCommand(controller)
	mockOsClient := mock_fcp.NewMockOS(controller)
	mockDevices := mock_devices.NewMockDevices(controller)

	mockFileSystem := mock_filesystem.NewMockFilesystem(controller)

	mockMount := mock_mount.NewMockMount(controller)
	mockReconcileUtils := mock_fcp.NewMockFcpReconcileUtils(controller)
	mockOS := mock_fcp.NewMockOS(controller)

	fs := afero.NewMemMapFs()

	return mockCommand, mockOsClient, mockDevices, mockFileSystem, mockMount, mockReconcileUtils, mockOS, fs
}

func vpdpg80SerialBytes(serial string) []byte {
	return append([]byte{0, 128, 0, 20}, []byte(serial)...)
}
