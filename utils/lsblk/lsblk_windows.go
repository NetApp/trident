package lsblk

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

func (l *LsblkUtil) GetBlockDevices(ctx context.Context) ([]BlockDevice, error) {
	Logc(ctx).Debug(">>>> lsblk_windows.GetBlockDevices")
	defer Logc(ctx).Debug("<<<< lsblk_windows.GetBlockDevices")
	return nil, errors.UnsupportedError("GetBlockDevices is not supported for windows")
}

func (l *LsblkUtil) FindParentDevice(ctx context.Context, deviceName string, devices []BlockDevice,
	parent *BlockDevice,
) (*BlockDevice, error) {
	Logc(ctx).Debug(">>>> lsblk_windows.FindParentDevice")
	defer Logc(ctx).Debug("<<<< lsblk_windows.FindParentDevice")
	return nil, errors.UnsupportedError("FindParentDevice is not supported for windows")
}

func (l *LsblkUtil) GetParentDeviceKname(ctx context.Context, deviceName string) (string, error) {
	Logc(ctx).Debug(">>>> lsblk_windows.GetParentDeviceKname")
	defer Logc(ctx).Debug("<<<< lsblk_windows.GetParentDeviceKname")
	return "", errors.UnsupportedError("GetParentDeviceKname is not supported for windows")
}
