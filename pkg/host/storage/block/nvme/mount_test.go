// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adapterblock "github.com/netapp/trident/pkg/host/adapter/block"
	"github.com/netapp/trident/pkg/host/service/block"
	"github.com/netapp/trident/pkg/host/service/filesystem"
	"github.com/netapp/trident/pkg/host/service/luks"
	"github.com/netapp/trident/pkg/host/service/nvme"
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

type mountNVMe struct {
	nvme.Service
	deviceErr error
}

func (n *mountNVMe) HasLivePath(context.Context, string) bool { return true }

func (n *mountNVMe) DeviceByUUID(_ context.Context, nqn, nsUUID string) (*nvme.Device, error) {
	if n.deviceErr != nil {
		return nil, n.deviceErr
	}
	return nvme.NewDeviceForTest(nqn, nsUUID, "nvme0n1"), nil
}

type mountBlock struct {
	block.Service
	top *adapterblock.Device
	err error
}

func (b *mountBlock) ResolveTopDevice(context.Context, ...string) (*adapterblock.Device, error) {
	return b.top, b.err
}

type mountCall struct {
	source, target, options string
	fsType                  filesystem.Type
}

type mountFS struct {
	filesystem.Service
	mounts     []mountCall
	mountErr   error
	mounted    bool
	mountedErr error
	unmounts   []string
	unmountErr error
}

func (f *mountFS) EnsureMount(
	_ context.Context, source filesystem.Source, target, options string, fsType filesystem.Type,
) error {
	f.mounts = append(f.mounts, mountCall{source.Path, target, options, fsType})
	return f.mountErr
}

func (f *mountFS) IsMounted(context.Context, filesystem.Source, string) (bool, error) {
	return f.mounted, f.mountedErr
}

func (f *mountFS) EnsureUnmount(_ context.Context, target string, _ bool) error {
	f.unmounts = append(f.unmounts, target)
	return f.unmountErr
}

type mountLUKS struct {
	luks.Service
	called     bool
	internalID string
	secrets    map[string]string
	err        error
	mapper     *luks.Device
	lookups    []string
}

func (l *mountLUKS) DeviceByUUID(_ context.Context, uuid string) (*luks.Device, error) {
	l.lookups = append(l.lookups, "uuid:"+uuid)
	return l.lookup()
}

func (l *mountLUKS) DeviceByInternalID(_ context.Context, internalID string) (*luks.Device, error) {
	l.lookups = append(l.lookups, "internalID:"+internalID)
	return l.lookup()
}

func (l *mountLUKS) lookup() (*luks.Device, error) {
	if l.mapper == nil {
		return nil, tridenterrors.NotFoundError("no mapper")
	}
	return l.mapper, nil
}

func (l *mountLUKS) ExtractSecrets(secrets map[string]string) (luks.Secret, luks.Secret) {
	l.secrets = secrets
	return luks.Secret{}, luks.Secret{}
}

func (l *mountLUKS) EnsurePassphrase(
	_ context.Context, internalID, _ string, _, _ luks.Secret, _ *luks.EnsurePassphraseOptions,
) error {
	l.called, l.internalID = true, internalID
	return l.err
}

func mountState(fsType models.FilesystemType, encrypted bool) *models.VolumeAttachState {
	state := &models.VolumeAttachState{Filesystem: fsType}
	state.NVMeSubsystemNQN = "nqn.sub"
	state.NVMeNamespaceUUID = "ns-1"
	if encrypted {
		state.Encryption = models.EncryptionLUKS
	}
	return state
}

func newMountDriver(b *mountBlock, f *mountFS, l *mountLUKS, n *mountNVMe) *Driver {
	return New(WithBlock(b), WithFilesystem(f), WithLUKS(l), WithNVMe(n))
}

func TestMount(t *testing.T) {
	spec := &models.VolumeMountSpec{MountPoint: "/target", MountOptions: "rw,ro", InternalID: "trident_pvc_1"}
	ctx := context.Background()

	t.Run("filesystem mounts the namespace", func(t *testing.T) {
		f := &mountFS{}
		d := newMountDriver(&mountBlock{top: adapterblock.NewDeviceForTest("nvme0n1")}, f, &mountLUKS{}, &mountNVMe{})

		require.NoError(t, d.Mount(ctx, "pvc-1", mountState(models.FilesystemExt4, false), spec))
		assert.Equal(t, []mountCall{{"/dev/nvme0n1", "/target", "rw,ro", models.FilesystemExt4}}, f.mounts)
	})

	t.Run("raw block bind mounts the namespace", func(t *testing.T) {
		f := &mountFS{}
		d := newMountDriver(&mountBlock{top: adapterblock.NewDeviceForTest("nvme0n1")}, f, &mountLUKS{}, &mountNVMe{})

		require.NoError(t, d.Mount(ctx, "pvc-1", mountState(models.FilesystemRaw, false), spec))
		assert.Equal(t, []mountCall{{"/dev/nvme0n1", "/target", "rw,ro", models.FilesystemRaw}}, f.mounts)
	})

	t.Run("a holder on the namespace is ignored and the namespace is mounted", func(t *testing.T) {
		f := &mountFS{}
		lvm := adapterblock.NewMapperDeviceForTest("dm-3", "LVM-x", "nvme0n1")
		d := newMountDriver(&mountBlock{top: lvm}, f, &mountLUKS{}, &mountNVMe{})

		require.NoError(t, d.Mount(ctx, "pvc-1", mountState(models.FilesystemRaw, false), spec))
		assert.Equal(t, []mountCall{{"/dev/nvme0n1", "/target", "rw,ro", models.FilesystemRaw}}, f.mounts)
	})

	t.Run("LUKS mounts the mapper and rotates the passphrase", func(t *testing.T) {
		mapper := luks.NewDeviceForTest("dm-0", "luks-trident_pvc_1", "CRYPT-LUKS2-x")
		f, l := &mountFS{}, &mountLUKS{mapper: mapper}
		d := newMountDriver(&mountBlock{}, f, l, &mountNVMe{})
		secrets := map[string]string{"k": "v"}

		require.NoError(t, d.Mount(ctx, "pvc-1", mountState(models.FilesystemExt4, true),
			&models.VolumeMountSpec{MountPoint: "/target", InternalID: "trident_pvc_1", Secrets: secrets}))
		assert.Equal(t, []mountCall{{"/dev/dm-0", "/target", "", models.FilesystemExt4}}, f.mounts)
		assert.Equal(t, []string{"internalID:trident_pvc_1"}, l.lookups)
		assert.True(t, l.called)
		assert.Equal(t, "trident_pvc_1", l.internalID)
		assert.Equal(t, secrets, l.secrets)
	})

	t.Run("LUKS finds the mapper by the tracked dm UUID", func(t *testing.T) {
		mapper := luks.NewDeviceForTest("dm-0", "luks-trident_pvc_1", "CRYPT-LUKS2-x")
		f, l := &mountFS{}, &mountLUKS{mapper: mapper}
		d := newMountDriver(&mountBlock{}, f, l, &mountNVMe{})
		state := mountState(models.FilesystemExt4, true)
		state.Devices = models.BlockDeviceStack{
			{ID: "ns-1", Name: "nvme0n1", Type: models.BlockDeviceDisk},
			{ID: "CRYPT-LUKS2-x", Name: "dm-0", Type: models.BlockDeviceCrypt, Base: "ns-1"},
		}

		require.NoError(t, d.Mount(ctx, "pvc-1", state, spec))
		assert.Equal(t, []string{"uuid:CRYPT-LUKS2-x"}, l.lookups)
		assert.Equal(t, "/dev/dm-0", f.mounts[0].source)
	})

	t.Run("raw block LUKS bind mounts the mapper's device node", func(t *testing.T) {
		mapper := luks.NewDeviceForTest("dm-0", "luks-trident_pvc_1", "CRYPT-LUKS2-x")
		f := &mountFS{}
		d := newMountDriver(&mountBlock{}, f, &mountLUKS{mapper: mapper}, &mountNVMe{})

		require.NoError(t, d.Mount(ctx, "pvc-1", mountState(models.FilesystemRaw, true), spec))
		assert.Equal(t, []mountCall{{"/dev/dm-0", "/target", "rw,ro", models.FilesystemRaw}}, f.mounts)
	})

	t.Run("a passphrase failure is logged, not returned", func(t *testing.T) {
		mapper := luks.NewDeviceForTest("dm-0", "luks-trident_pvc_1", "CRYPT-LUKS2-x")
		f, l := &mountFS{}, &mountLUKS{mapper: mapper, err: errors.New("no working passphrase provided")}
		d := newMountDriver(&mountBlock{}, f, l, &mountNVMe{})

		require.NoError(t, d.Mount(ctx, "pvc-1", mountState(models.FilesystemExt4, true), spec))
		assert.Len(t, f.mounts, 1)
	})

	t.Run("LUKS with the mapper closed is refused", func(t *testing.T) {
		f, l := &mountFS{}, &mountLUKS{}
		crypt := adapterblock.NewCryptDeviceForTest("dm-7", "luks-other", "CRYPT-LUKS2-other")
		d := newMountDriver(&mountBlock{top: crypt}, f, l, &mountNVMe{})

		err := d.Mount(ctx, "pvc-1", mountState(models.FilesystemExt4, true), spec)

		require.Error(t, err)
		assert.True(t, tridenterrors.IsInternalError(err))
		assert.Contains(t, err.Error(), "unable to mount device; LUKS device for namespace ns-1 is not open")
		assert.Empty(t, f.mounts)
		assert.False(t, l.called)
	})

	t.Run("a missing namespace is an internal error", func(t *testing.T) {
		f := &mountFS{}
		n := &mountNVMe{deviceErr: tridenterrors.NotFoundError("namespace ns-1")}
		d := newMountDriver(&mountBlock{}, f, &mountLUKS{}, n)

		err := d.Mount(ctx, "pvc-1", mountState(models.FilesystemExt4, false), spec)

		require.Error(t, err)
		assert.True(t, tridenterrors.IsInternalError(err))
		assert.False(t, tridenterrors.IsNotFoundError(err))
		assert.Empty(t, f.mounts)
	})

	t.Run("mount failures are worded as master's", func(t *testing.T) {
		for fsType, want := range map[models.FilesystemType]string{
			models.FilesystemExt4: "unable to mount device; mount failed",
			models.FilesystemRaw:  "unable to bind mount raw device; mount failed",
		} {
			f := &mountFS{mountErr: errors.New("mount failed")}
			d := newMountDriver(&mountBlock{top: adapterblock.NewDeviceForTest("nvme0n1")}, f, &mountLUKS{}, &mountNVMe{})

			err := d.Mount(ctx, "pvc-1", mountState(fsType, false), spec)

			require.Error(t, err)
			assert.True(t, tridenterrors.IsInternalError(err))
			assert.EqualError(t, err, want)
		}
	})
}

func TestUnmount(t *testing.T) {
	spec := &models.VolumeUnmountSpec{MountPoint: "/target"}
	ctx := context.Background()

	t.Run("mounted target is unmounted", func(t *testing.T) {
		f := &mountFS{mounted: true}
		d := New(WithFilesystem(f))

		require.NoError(t, d.Unmount(ctx, "pvc-1", spec))
		assert.Equal(t, []string{"/target"}, f.unmounts)
	})

	t.Run("target that is not mounted is left alone", func(t *testing.T) {
		f := &mountFS{}
		d := New(WithFilesystem(f))

		require.NoError(t, d.Unmount(ctx, "pvc-1", spec))
		assert.Empty(t, f.unmounts)
	})

	t.Run("a failed mount check is an internal error", func(t *testing.T) {
		d := New(WithFilesystem(&mountFS{mountedErr: errors.New("boom")}))

		err := d.Unmount(ctx, "pvc-1", spec)

		require.Error(t, err)
		assert.True(t, tridenterrors.IsInternalError(err))
		assert.EqualError(t, err, "unable to check if targetPath (/target) is mounted; boom")
	})

	t.Run("a failed unmount is an invalid input error", func(t *testing.T) {
		d := New(WithFilesystem(&mountFS{mounted: true, unmountErr: errors.New("device busy")}))

		err := d.Unmount(ctx, "pvc-1", spec)

		require.Error(t, err)
		assert.True(t, tridenterrors.IsInvalidInputError(err))
		assert.EqualError(t, err, "unable to unmount volume; device busy")
	})
}
