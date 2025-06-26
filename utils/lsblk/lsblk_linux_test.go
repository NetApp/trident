package lsblk

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/exec"
)

var mockLsblkOutput = `
{
   "blockdevices": [
      {
         "name": "loop0",
         "kname": "loop0",
         "type": "loop",
         "size": "4K",
         "mountpoint": "/snap/bare/5"
      },{
         "name": "loop1",
         "kname": "loop1",
         "type": "loop",
         "size": "49.1M",
         "mountpoint": "/snap/core18/2826"
      },{
         "name": "sda",
         "kname": "sda",
         "type": "disk",
         "size": "96G",
         "mountpoint": null,
         "children": [
            {
               "name": "sda1",
               "kname": "sda1",
               "type": "part",
               "size": "1G",
               "mountpoint": "/boot/efi"
            },{
               "name": "sda2",
               "kname": "sda2",
               "type": "part",
               "size": "94.9G",
               "mountpoint": "/"
            }
         ]
      },{
         "name": "sr0",
         "kname": "sr0",
         "type": "rom",
         "size": "1024M",
         "mountpoint": null
      },{
         "name": "3600a0980796f6377432b577333523168",
         "kname": "dm-0",
         "type": "mpath",
         "size": "1G",
         "mountpoint": null,
         "children": [
            {
               "name": "luks-trident_pvc_43c2e279_9964_4af5_a0ef_5ec2c8815b16",
               "kname": "dm-1",
               "type": "crypt",
               "size": "1G",
               "mountpoint": null
            }
         ]
      }
   ]
}
`

func TestGetNamespaceCount(t *testing.T) {
	tests := map[string]struct {
		getCommand   func(ctrl *gomock.Controller) exec.Command
		devicePath   string
		expectDevice string
		expectError  bool
	}{
		"Full Path": {
			getCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCmd := mock_exec.NewMockCommand(ctrl)
				mockCmd.EXPECT().ExecuteWithTimeout(gomock.Any(), "lsblk", gomock.Any(), false, "-o",
					"NAME,KNAME,TYPE,SIZE,MOUNTPOINT", "--json").Return([]byte(mockLsblkOutput), nil)
				return mockCmd
			},
			devicePath:   "/dev/mapper/luks-trident_pvc_43c2e279_9964_4af5_a0ef_5ec2c8815b16",
			expectDevice: "dm-0",
			expectError:  false,
		},
		"Not full path, device name only": {
			getCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCmd := mock_exec.NewMockCommand(ctrl)
				mockCmd.EXPECT().ExecuteWithTimeout(gomock.Any(), "lsblk", gomock.Any(), false, "-o",
					"NAME,KNAME,TYPE,SIZE,MOUNTPOINT", "--json").Return([]byte(mockLsblkOutput), nil)
				return mockCmd
			},
			devicePath:   "luks-trident_pvc_43c2e279_9964_4af5_a0ef_5ec2c8815b16",
			expectDevice: "dm-0",
			expectError:  false,
		},
		"Exec error": {
			getCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCmd := mock_exec.NewMockCommand(ctrl)
				mockCmd.EXPECT().ExecuteWithTimeout(gomock.Any(), "lsblk", gomock.Any(), false, "-o",
					"NAME,KNAME,TYPE,SIZE,MOUNTPOINT", "--json").Return([]byte{}, errors.New("error"))
				return mockCmd
			},
			devicePath:   "luks-trident_pvc_43c2e279_9964_4af5_a0ef_5ec2c8815b16",
			expectDevice: "",
			expectError:  true,
		},
		"Bad json": {
			getCommand: func(ctrl *gomock.Controller) exec.Command {
				mockCmd := mock_exec.NewMockCommand(ctrl)
				mockCmd.EXPECT().ExecuteWithTimeout(gomock.Any(), "lsblk", gomock.Any(), false, "-o",
					"NAME,KNAME,TYPE,SIZE,MOUNTPOINT", "--json").Return([]byte{'b', 'a', 'd'}, nil)
				return mockCmd
			},
			devicePath:   "luks-trident_pvc_43c2e279_9964_4af5_a0ef_5ec2c8815b16",
			expectDevice: "",
			expectError:  true,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			lsblk := NewLsblkUtilDetailed(params.getCommand(gomock.NewController(t)))
			parentDevice, err := lsblk.GetParentDeviceKname(context.Background(), params.devicePath)
			if params.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, params.expectDevice, parentDevice)
			}
		})
	}
}
