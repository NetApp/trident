// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/pkg/host/service/block"
	"github.com/netapp/trident/pkg/host/service/luks"
	"github.com/netapp/trident/pkg/host/service/nvme"
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

type staleLUKS struct {
	luks.Service
	mapper   *luks.Device
	err      error
	closed   string
	closeErr error
}

func (l *staleLUKS) DeviceByInternalID(context.Context, string) (*luks.Device, error) {
	return l.mapper, l.err
}

func (l *staleLUKS) EnsureClose(_ context.Context, dmUUID string, _ bool) error {
	l.closed = dmUUID
	return l.closeErr
}

type staleBlock struct {
	block.Service
	stale        bool
	err          error
	uuid, device string
}

func (b *staleBlock) IsMapperStale(_ context.Context, dmUUID, baseDeviceName string) (bool, error) {
	b.uuid, b.device = dmUUID, baseDeviceName
	return b.stale, b.err
}

func TestCloseStaleMapper(t *testing.T) {
	ctx := context.Background()
	mapper := luks.NewDeviceForTest("dm-0", "luks-trident_pvc_1", "CRYPT-LUKS2-x")

	tests := map[string]struct {
		luks      *staleLUKS
		block     *staleBlock
		wantClose bool
		wantErr   string
	}{
		"no mapper open":          {luks: &staleLUKS{err: tridenterrors.NotFoundError("gone")}, block: &staleBlock{}},
		"mapper on the namespace": {luks: &staleLUKS{mapper: mapper}, block: &staleBlock{}},
		"stale mapper":            {luks: &staleLUKS{mapper: mapper}, block: &staleBlock{stale: true}, wantClose: true},
		"unreadable stack is closed": {
			luks: &staleLUKS{mapper: mapper}, block: &staleBlock{err: errors.New("sysfs unreadable")}, wantClose: true,
		},
		"lookup failure": {
			luks: &staleLUKS{err: errors.New("sysfs unreadable")}, block: &staleBlock{},
			wantErr: "failed to look up LUKS mapper: sysfs unreadable",
		},
		"close failure": {
			luks: &staleLUKS{mapper: mapper, closeErr: errors.New("busy")}, block: &staleBlock{stale: true},
			wantClose: true, wantErr: "could not remove LUKS mapping 'luks-trident_pvc_1' for device 'nvme0n1'; busy",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			d := New(WithLUKS(tc.luks), WithBlock(tc.block))

			err := d.closeStaleMapper(ctx, "trident_pvc_1", "nvme0n1")

			if tc.wantErr != "" {
				assert.EqualError(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
			if tc.luks.mapper != nil {
				assert.Equal(t, "CRYPT-LUKS2-x", tc.block.uuid)
				assert.Equal(t, "nvme0n1", tc.block.device)
			}
			if tc.wantClose {
				assert.Equal(t, "CRYPT-LUKS2-x", tc.luks.closed)
			} else {
				assert.Empty(t, tc.luks.closed)
			}
		})
	}
}

func TestNewAttachSpec(t *testing.T) {
	publishInfo := &models.VolumePublishInfo{FilesystemType: "ext4", LUKSEncryption: "true", VolumeMode: "Filesystem"}
	publishInfo.InternalID = "trident_pvc_internal"
	publishInfo.NVMeSubsystemNQN = "nqn.test"
	publishInfo.NVMeNamespaceUUID = "ns-uuid"
	publishInfo.NVMeTargetIPs = []string{"192.0.2.10"}
	publishInfo.MountOptions = "discard"
	secrets := map[string]string{"luks-passphrase": "x"}

	spec := NewAttachSpec(publishInfo, "vol1", "/mnt/vol1", secrets)

	assert.Equal(t, "vol1", spec.InternalID)
	assert.Equal(t, "/mnt/vol1", spec.MountPoint)
	assert.Equal(t, "nqn.test", spec.NVMeSubsystemNQN)
	assert.Equal(t, "ns-uuid", spec.NVMeNamespaceUUID)
	assert.Equal(t, []string{"192.0.2.10"}, spec.NVMeTargetIPs)
	assert.Equal(t, "discard", spec.MountOptions)
	assert.Equal(t, "ext4", spec.Filesystem)
	assert.Equal(t, "Filesystem", spec.VolumeMode)
	assert.Equal(t, models.EncryptionLUKS, spec.Encryption)
	assert.Equal(t, secrets, spec.Secrets)

	publishInfo.LUKSEncryption = ""
	assert.NotEqual(t, models.EncryptionLUKS, NewAttachSpec(publishInfo, "vol1", "", nil).Encryption)
}

// failingConnectNVMe fails every connect and records disconnects.
type failingConnectNVMe struct {
	nvme.Service
	disconnects []string
}

func (n *failingConnectNVMe) PreChecks(context.Context) error { return nil }

func (n *failingConnectNVMe) EnsureConnections(context.Context, string, []string) error {
	return errors.New("connection refused")
}

func (n *failingConnectNVMe) EnsureDisconnect(_ context.Context, nqn, _ string, _ bool) error {
	n.disconnects = append(n.disconnects, nqn)
	return nil
}

func TestAttach_FailedAttachByDriverContext(t *testing.T) {
	spec := &models.VolumeAttachSpec{}
	spec.NVMeSubsystemNQN = "nqn.sub"
	spec.NVMeNamespaceUUID = "ns-1"
	spec.NVMeTargetIPs = []string{"10.0.0.1"}

	t.Run("CSI removes the session it published and disconnects", func(t *testing.T) {
		n := &failingConnectNVMe{}
		d := New(WithDriverContext(config.ContextCSI), WithNVMe(n))

		_, err := d.Attach(context.Background(), "pvc-1", spec)

		require.ErrorContains(t, err, "connection refused")
		assert.False(t, d.sessionStore.SessionExists("nqn.sub"))
		assert.Equal(t, []string{"nqn.sub"}, n.disconnects)
	})

	t.Run("Docker publishes nothing and leaves the subsystem connected", func(t *testing.T) {
		n := &failingConnectNVMe{}
		d := New(WithDriverContext(config.ContextDocker), WithNVMe(n))

		_, err := d.Attach(context.Background(), "pvc-1", spec)

		require.ErrorContains(t, err, "connection refused")
		assert.False(t, d.sessionStore.SessionExists("nqn.sub"))
		assert.Empty(t, n.disconnects)
	})
}

type passphraseLUKS struct {
	luks.Service
	internalID, devicePath string
	opts                   *luks.EnsurePassphraseOptions
	err                    error
}

func (l *passphraseLUKS) ExtractSecrets(map[string]string) (luks.Secret, luks.Secret) {
	return luks.Secret{}, luks.Secret{}
}

func (l *passphraseLUKS) EnsurePassphrase(
	_ context.Context, internalID, devicePath string, _, _ luks.Secret, opts *luks.EnsurePassphraseOptions,
) error {
	l.internalID, l.devicePath, l.opts = internalID, devicePath, opts
	return l.err
}

func TestEnsureAttachedPassphrase(t *testing.T) {
	spec := &models.VolumeAttachSpec{InternalID: "trident_pvc_1"}
	spec.Encryption = models.EncryptionLUKS

	t.Run("records against the volume", func(t *testing.T) {
		l := &passphraseLUKS{}
		d := New(WithLUKS(l))

		require.NoError(t, d.ensureAttachedPassphrase(context.Background(), "pvc-1", spec, "/dev/nvme0n1"))
		assert.Equal(t, "trident_pvc_1", l.internalID)
		assert.Equal(t, "/dev/nvme0n1", l.devicePath)
		assert.Equal(t, "pvc-1", l.opts.VolumeID)
	})

	t.Run("failure is a passphrase error", func(t *testing.T) {
		rotateErr := errors.New("no working passphrase provided")
		d := New(WithLUKS(&passphraseLUKS{err: rotateErr}))

		err := d.ensureAttachedPassphrase(context.Background(), "pvc-1", spec, "/dev/nvme0n1")

		assert.True(t, IsLUKSPassphraseError(err))
		assert.ErrorIs(t, err, rotateErr)
		assert.EqualError(t, err, "could not set LUKS volume passphrase; no working passphrase provided")
	})

	t.Run("not encrypted", func(t *testing.T) {
		l := &passphraseLUKS{}
		d := New(WithLUKS(l))

		require.NoError(t, d.ensureAttachedPassphrase(context.Background(), "pvc-1", &models.VolumeAttachSpec{}, ""))
		assert.Nil(t, l.opts)
	})
}
