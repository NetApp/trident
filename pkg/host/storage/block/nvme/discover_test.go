// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterblock "github.com/netapp/trident/pkg/host/adapter/block"
	"github.com/netapp/trident/pkg/host/service/block"
	"github.com/netapp/trident/pkg/host/service/luks"
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

type discoverBlock struct {
	block.Service
	stack []*adapterblock.Device
}

func (b *discoverBlock) ResolveDeviceStack(context.Context, string) ([]*adapterblock.Device, error) {
	return b.stack, nil
}

type discoverLUKS struct {
	luks.Service
	dev *luks.Device
	err error
}

func (l *discoverLUKS) DeviceByUUID(context.Context, string) (*luks.Device, error) {
	return l.dev, l.err
}

func TestDiscover_CryptLayer(t *testing.T) {
	ctx := context.Background()
	publishInfo := &models.VolumePublishInfo{}
	publishInfo.NVMeSubsystemNQN = "nqn.sub"
	publishInfo.NVMeNamespaceUUID = "ns-1"
	stack := []*adapterblock.Device{
		adapterblock.NewDeviceForTest("nvme0n1"),
		adapterblock.NewCryptDeviceForTest("dm-0", "luks-trident_pvc_1", "CRYPT-LUKS2-x"),
	}
	discover := func(l *discoverLUKS) (*models.VolumeAttachState, error) {
		d := New(WithBlock(&discoverBlock{stack: stack}), WithLUKS(l), WithNVMe(&mountNVMe{}))
		return d.Discover(ctx, publishInfo)
	}

	t.Run("an open mapper is recorded on top of the namespace", func(t *testing.T) {
		state, err := discover(&discoverLUKS{dev: luks.NewDeviceForTest("dm-0", "luks-trident_pvc_1", "CRYPT-LUKS2-x")})

		require.NoError(t, err)
		require.Len(t, state.Devices, 2)
		assert.Equal(t, models.BlockDeviceCrypt, state.Devices[1].Type)
		assert.Equal(t, "/dev/dm-0", state.DevicePath)
	})

	t.Run("a mapper that disappears is skipped", func(t *testing.T) {
		state, err := discover(&discoverLUKS{err: tridenterrors.NotFoundError("gone")})

		require.NoError(t, err)
		require.NotNil(t, state)
		require.Len(t, state.Devices, 1)
		assert.Equal(t, models.BlockDeviceDisk, state.Devices[0].Type)
		assert.Equal(t, "/dev/nvme0n1", state.DevicePath)
	})

	t.Run("a nil mapper with no error is an error", func(t *testing.T) {
		state, err := discover(&discoverLUKS{})

		require.Error(t, err)
		assert.Nil(t, state)
	})
}
