package lsblk

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"
)

// GetBlockDevices returns a list of block devices and their details
func (l *LsblkUtil) GetBlockDevices(ctx context.Context) ([]BlockDevice, error) {
	// Run the lsblk command with --json option
	bytes, err := l.command.ExecuteWithTimeout(ctx, "lsblk", 10*time.Second, false, "-o",
		"NAME,KNAME,TYPE,SIZE,MOUNTPOINT", "--json")
	if err != nil {
		return nil, fmt.Errorf("error running lsblk: %v", err)
	}

	// Parse the JSON output
	var results LsblkResults
	err = json.Unmarshal(bytes, &results)
	if err != nil {
		return nil, fmt.Errorf("error parsing lsblk json output: %v", err)
	}
	return results.BlockDevices, nil
}

// FindParentDevice finds the parent block device for a given device name or path
func (l *LsblkUtil) FindParentDevice(
	ctx context.Context, deviceName string, devices []BlockDevice, parent *BlockDevice,
) (*BlockDevice, error) {
	// Recursive function to find a parent device
	var findParentDevice func(devices []BlockDevice, parent *BlockDevice) *BlockDevice
	findParentDevice = func(devices []BlockDevice, parent *BlockDevice) *BlockDevice {
		for _, device := range devices {
			if device.Name == deviceName {
				return parent
			}
			if len(device.Children) > 0 {
				found := findParentDevice(device.Children, &device)
				if found != nil {
					return found
				}
			}
		}
		return nil
	}

	// Find the parent device
	parentDevice := findParentDevice(devices, nil)
	if parentDevice == nil {
		return nil, fmt.Errorf("parent device for '%s' not found", deviceName)
	} else {
		return parentDevice, nil
	}
}

// GetParentDmDeviceKname returns the kernel name (e.g. "dm-0") of the parent device
// (e.g. "/dev/mapper/luks-<UUID>", aka "dm-1") for a given device name
func (l *LsblkUtil) GetParentDeviceKname(ctx context.Context, deviceName string) (string, error) {
	// Get just the name if a path was passed in
	deviceName = filepath.Base(deviceName)
	devices, err := l.GetBlockDevices(ctx)
	if err != nil {
		return "", err
	}

	parentDevice, err := l.FindParentDevice(ctx, deviceName, devices, nil)
	if err != nil {
		return "", err
	}

	return parentDevice.KName, nil
}
