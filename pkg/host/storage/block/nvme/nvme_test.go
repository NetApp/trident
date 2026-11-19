// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/pkg/host/service/luks"
	"github.com/netapp/trident/pkg/host/service/nvme"
	"github.com/netapp/trident/pkg/host/storage"
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

type mapperLUKS struct {
	luks.Service
	internalID string
	dev        *luks.Device
	err        error
}

func (l *mapperLUKS) DeviceByInternalID(_ context.Context, internalID string) (*luks.Device, error) {
	l.internalID = internalID
	return l.dev, l.err
}

func TestMapperByInternalID(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		l := &mapperLUKS{dev: luks.NewDeviceForTest("dm-0", "luks-trident_pvc_1", "CRYPT-LUKS2-x")}

		info, err := New(WithLUKS(l)).mapperByInternalID(context.Background(), "trident_pvc_1")

		require.NoError(t, err)
		assert.Equal(t, "trident_pvc_1", l.internalID)
		assert.Equal(t,
			&models.BlockDeviceInfo{ID: "CRYPT-LUKS2-x", Type: models.BlockDeviceCrypt, Name: "luks-trident_pvc_1"}, info)
	})

	t.Run("not found", func(t *testing.T) {
		l := &mapperLUKS{err: tridenterrors.NotFoundError("gone")}

		info, err := New(WithLUKS(l)).mapperByInternalID(context.Background(), "trident_pvc_1")

		require.NoError(t, err)
		assert.Nil(t, info)
	})

	t.Run("lookup failure", func(t *testing.T) {
		lookupErr := errors.New("sysfs unreadable")

		info, err := New(WithLUKS(&mapperLUKS{err: lookupErr})).mapperByInternalID(context.Background(), "trident_pvc_1")

		assert.ErrorIs(t, err, lookupErr)
		assert.Nil(t, info)
	})
}

type preCheckNVMe struct {
	nvme.Service
	err error
}

func (n preCheckNVMe) PreChecks(context.Context) error { return n.err }

func TestPreChecks(t *testing.T) {
	ctx := context.Background()

	assert.NoError(t, New(WithNVMe(preCheckNVMe{})).PreChecks(ctx))

	err := New(WithNVMe(preCheckNVMe{err: nvme.ErrTransportNotLoaded})).PreChecks(ctx)
	assert.ErrorIs(t, err, storage.ErrTransportNotLoaded, "the caller sees an inactive protocol")
	assert.ErrorIs(t, err, nvme.ErrTransportNotLoaded)

	cliErr := errors.New("nvme-cli not found")
	err = New(WithNVMe(preCheckNVMe{err: cliErr})).PreChecks(ctx)
	assert.Same(t, cliErr, err)
}
