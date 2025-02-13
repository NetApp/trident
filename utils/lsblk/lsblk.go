package lsblk

import (
	"context"

	"github.com/netapp/trident/utils/exec"
)

type Lsblk interface {
	GetBlockDevices(ctx context.Context) ([]BlockDevice, error)
	FindParentDevice(ctx context.Context, deviceName string, devices []BlockDevice,
		parent *BlockDevice) (*BlockDevice, error)
	GetParentDeviceKname(ctx context.Context, deviceName string) (string, error)
}

type LsblkResults struct {
	BlockDevices []BlockDevice `json:"blockdevices"`
}

// BlockDevice represents a block device in the lsblk output
type BlockDevice struct {
	Name       string        `json:"name"`
	KName      string        `json:"kname"`
	Type       string        `json:"type"`
	Size       string        `json:"size"`
	Mountpoint string        `json:"mountpoint"`
	Children   []BlockDevice `json:"children"`
}

type LsblkUtil struct {
	command exec.Command
}

func NewLsblkUtil() *LsblkUtil {
	return &LsblkUtil{
		command: exec.NewCommand(),
	}
}

func NewLsblkUtilDetailed(command exec.Command) *LsblkUtil {
	return &LsblkUtil{
		command: command,
	}
}
