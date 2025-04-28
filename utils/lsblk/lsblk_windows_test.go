package lsblk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_exec"
)

func TestGetBlockDevices(t *testing.T) {
	ctrl := gomock.NewController(t)
	lsblk := NewLsblkUtilDetailed(mock_exec.NewMockCommand(ctrl))
	devices, err := lsblk.GetBlockDevices(context.Background())
	assert.Nil(t, devices)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestFindParentDevice(t *testing.T) {
	ctrl := gomock.NewController(t)
	lsblk := NewLsblkUtilDetailed(mock_exec.NewMockCommand(ctrl))
	devices, err := lsblk.FindParentDevice(context.Background(), "deviceName", nil, nil)
	assert.Nil(t, devices)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}

func TestGetParentDeviceKname(t *testing.T) {
	ctrl := gomock.NewController(t)
	lsblk := NewLsblkUtilDetailed(mock_exec.NewMockCommand(ctrl))
	devices, err := lsblk.GetParentDeviceKname(context.Background(), "deviceName")
	assert.Empty(t, devices)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")
}
