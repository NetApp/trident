// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/pkg/host/service/filesystem"
	"github.com/netapp/trident/pkg/host/service/luks"
	"github.com/netapp/trident/pkg/host/service/nvme"
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// expandNVMe returns the namespace sizes in order, one per DeviceByUUID call, repeating the last. A nil entry
// fails the call.
type expandNVMe struct {
	nvme.Service
	sizes       []*uint64
	calls       int
	noLivePath  bool
	waitErr     error
	waited      []uint64
	events      *[]string
	livePathNQN []string
}

func size(n uint64) *uint64 { return &n }

func (n *expandNVMe) DeviceByUUID(_ context.Context, nqn, nsUUID string) (*nvme.Device, error) {
	i := min(n.calls, len(n.sizes)-1)
	n.calls++
	if n.sizes[i] == nil {
		return nil, errors.New("namespace not found")
	}
	return nvme.NewSizedDeviceForTest(nqn, nsUUID, "nvme0n1", *n.sizes[i]), nil
}

func (n *expandNVMe) HasLivePath(_ context.Context, nqn string) bool {
	n.livePathNQN = append(n.livePathNQN, nqn)
	return !n.noLivePath
}

func (n *expandNVMe) EnsureSize(_ context.Context, _ *nvme.Device, targetBytes uint64) error {
	n.waited = append(n.waited, targetBytes)
	return n.waitErr
}

type expandLUKS struct {
	luks.Service
	resizeErr  error
	resized    []string
	passphrase []string
}

func (l *expandLUKS) DeviceByUUID(context.Context, string) (*luks.Device, error) {
	return luks.NewDeviceForTest("dm-0", "luks-pvc-1", "CRYPT-LUKS2-x"), nil
}

func (l *expandLUKS) ExtractSecrets(secrets map[string]string) (current, previous luks.Secret) {
	return luks.Secret{Passphrase: secrets["luks-passphrase"]}, luks.Secret{}
}

func (l *expandLUKS) EnsureResize(_ context.Context, dmUUID string, secret luks.Secret) error {
	l.resized = append(l.resized, dmUUID)
	l.passphrase = append(l.passphrase, secret.Passphrase)
	return l.resizeErr
}

type expandCall struct {
	device, staging, options string
	fsType                   filesystem.Type
}

type expandFS struct {
	filesystem.Service
	before, after uint64
	err           error
	calls         []expandCall
}

func (f *expandFS) EnsureExpand(
	_ context.Context, source filesystem.Source, staging, options string, fsType filesystem.Type,
) (uint64, uint64, error) {
	f.calls = append(f.calls, expandCall{source.Path, staging, options, fsType})
	return f.before, f.after, f.err
}

const expandStagingPath = "/var/lib/trident/staging/pvc-1"

func expandState(fsType models.FilesystemType, luksEncrypted bool) *models.VolumeAttachState {
	state := &models.VolumeAttachState{Filesystem: fsType, GlobalMount: expandStagingPath}
	state.MountOptions = "discard"
	state.NVMeSubsystemNQN = "nqn.sub"
	state.NVMeNamespaceUUID = "ns-1"
	state.BlockAttachInfo.Devices = models.BlockDeviceStack{{ID: "ns-1", Type: models.BlockDeviceDisk}}
	if luksEncrypted {
		state.Encryption = models.EncryptionLUKS
		state.BlockAttachInfo.Devices = append(state.BlockAttachInfo.Devices,
			models.BlockDeviceInfo{ID: "CRYPT-LUKS2-x", Type: models.BlockDeviceCrypt, Base: "ns-1"})
	}
	return state
}

func expandSpec() *models.VolumeExpandSpec {
	return &models.VolumeExpandSpec{
		SizeBytes: 2000,
		Secrets:   map[string]string{"luks-passphrase": "secret"},
	}
}

func TestExpand_Raw(t *testing.T) {
	n := &expandNVMe{sizes: []*uint64{size(1000)}}
	f := &expandFS{}
	d := New(WithNVMe(n), WithLUKS(&expandLUKS{}), WithFilesystem(f))

	err := d.Expand(context.Background(), "pvc-1", expandState(filesystem.Raw, false), expandSpec())

	require.NoError(t, err)
	assert.Empty(t, f.calls)
	assert.Equal(t, []uint64{2000}, n.waited, "the namespace is waited for even without a filesystem")
}

func TestExpand_WaitsForTheNamespace(t *testing.T) {
	t.Run("no live path", func(t *testing.T) {
		n := &expandNVMe{sizes: []*uint64{size(1000)}, noLivePath: true}
		l := &expandLUKS{}
		f := &expandFS{}
		d := New(WithNVMe(n), WithLUKS(l), WithFilesystem(f))

		err := d.Expand(context.Background(), "pvc-1", expandState(filesystem.Ext4, true), expandSpec())

		require.Error(t, err)
		assert.True(t, tridenterrors.IsVolumeStateError(err), "a retriable error")
		assert.Equal(t, "NVMe subsystem nqn.sub has no live path to expand volume pvc-1; retry later", err.Error())
		assert.Equal(t, []string{"nqn.sub"}, n.livePathNQN)
		assert.Empty(t, n.waited)
		assert.Empty(t, l.resized)
		assert.Empty(t, f.calls)
	})

	t.Run("size not reached", func(t *testing.T) {
		n := &expandNVMe{sizes: []*uint64{size(1000)}, waitErr: context.DeadlineExceeded}
		l := &expandLUKS{}
		f := &expandFS{}
		d := New(WithNVMe(n), WithLUKS(l), WithFilesystem(f))

		err := d.Expand(context.Background(), "pvc-1", expandState(filesystem.Ext4, true), expandSpec())

		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.ErrorContains(t, err, "NVMe namespace ns-1 did not reach 2000 bytes")
		assert.Equal(t, []uint64{2000}, n.waited)
		assert.Empty(t, l.resized)
		assert.Empty(t, f.calls)
	})

	t.Run("zero size has nothing to wait for", func(t *testing.T) {
		n := &expandNVMe{sizes: []*uint64{size(1000)}}
		d := New(WithNVMe(n), WithFilesystem(&expandFS{}))
		spec := expandSpec()
		spec.SizeBytes = 0

		require.NoError(t, d.Expand(context.Background(), "pvc-1", expandState(filesystem.Raw, false), spec))
		assert.Empty(t, n.waited)
	})
}

func TestExpand_NamespaceMissing(t *testing.T) {
	d := New(WithNVMe(&expandNVMe{sizes: []*uint64{nil}}), WithFilesystem(&expandFS{}))

	err := d.Expand(context.Background(), "pvc-1", expandState(filesystem.Raw, false), expandSpec())

	assert.ErrorContains(t, err, "failed to find NVMe device: namespace not found")
}

func TestExpand_LUKS(t *testing.T) {
	t.Run("resizes unconditionally", func(t *testing.T) {
		l := &expandLUKS{}
		d := New(WithNVMe(&expandNVMe{sizes: []*uint64{size(1000)}}), WithLUKS(l), WithFilesystem(&expandFS{}))

		err := d.Expand(context.Background(), "pvc-1", expandState(filesystem.Raw, true), expandSpec())

		require.NoError(t, err)
		assert.Equal(t, []string{"CRYPT-LUKS2-x"}, l.resized)
		assert.Equal(t, []string{"secret"}, l.passphrase)
	})

	t.Run("incorrect passphrase", func(t *testing.T) {
		resizeErr := tridenterrors.IncorrectLUKSPassphraseError("no key available with this passphrase; exit status 2")
		d := New(WithNVMe(&expandNVMe{sizes: []*uint64{size(1000)}}),
			WithLUKS(&expandLUKS{resizeErr: resizeErr}), WithFilesystem(&expandFS{}))

		err := d.Expand(context.Background(), "pvc-1", expandState(filesystem.Raw, true), expandSpec())

		require.Error(t, err)
		assert.True(t, tridenterrors.IsInvalidInputError(err))
		assert.Equal(t,
			"could not resize LUKS mapping for volume pvc-1; no key available with this passphrase; exit status 2",
			err.Error())
	})

	t.Run("other resize failure", func(t *testing.T) {
		d := New(WithNVMe(&expandNVMe{sizes: []*uint64{size(1000)}}),
			WithLUKS(&expandLUKS{resizeErr: errors.New("busy")}), WithFilesystem(&expandFS{}))

		err := d.Expand(context.Background(), "pvc-1", expandState(filesystem.Raw, true), expandSpec())

		require.Error(t, err)
		assert.True(t, tridenterrors.IsInternalError(err))
		assert.Equal(t,
			"could not resize LUKS mapping for volume pvc-1; failed to resize LUKS mapping for volume pvc-1: busy",
			err.Error())
	})

	for name, tc := range map[string]struct {
		secrets map[string]string
		message string
	}{
		"missing passphrase": {message: "cannot expand LUKS encrypted volume; no passphrase provided"},
		"empty passphrase": {
			secrets: map[string]string{"luks-passphrase": ""},
			message: "cannot expand LUKS encrypted volume; empty passphrase provided",
		},
	} {
		t.Run(name, func(t *testing.T) {
			l := &expandLUKS{}
			d := New(WithNVMe(&expandNVMe{sizes: []*uint64{size(1000)}}), WithLUKS(l), WithFilesystem(&expandFS{}))
			spec := expandSpec()
			spec.Secrets = tc.secrets

			err := d.Expand(context.Background(), "pvc-1", expandState(filesystem.Raw, true), spec)

			require.Error(t, err)
			assert.True(t, tridenterrors.IsInvalidInputError(err))
			assert.Equal(t, tc.message, err.Error())
			assert.Empty(t, l.resized)
		})
	}

	t.Run("encrypted volume without a recorded crypt layer", func(t *testing.T) {
		l := &expandLUKS{}
		state := expandState(filesystem.Raw, true)
		state.BlockAttachInfo.Devices = state.BlockAttachInfo.Devices[:1]
		d := New(WithNVMe(&expandNVMe{sizes: []*uint64{size(1000)}}), WithLUKS(l), WithFilesystem(&expandFS{}))

		err := d.Expand(context.Background(), "pvc-1", state, expandSpec())

		require.Error(t, err)
		assert.Empty(t, l.resized)
	})

	t.Run("filesystem expands on the mapper", func(t *testing.T) {
		f := &expandFS{before: 100, after: 200}
		d := New(WithNVMe(&expandNVMe{sizes: []*uint64{size(1000)}}), WithLUKS(&expandLUKS{}), WithFilesystem(f))

		require.NoError(t, d.Expand(context.Background(), "pvc-1", expandState(filesystem.Ext4, true), expandSpec()))
		assert.Equal(t, []expandCall{{"/dev/mapper/luks-pvc-1", expandStagingPath, "discard", filesystem.Ext4}}, f.calls)
	})
}

func TestExpand_Filesystem(t *testing.T) {
	t.Run("resizes with the publish mount options", func(t *testing.T) {
		n := &expandNVMe{sizes: []*uint64{size(1000), size(2000)}}
		f := &expandFS{before: 100, after: 200}
		d := New(WithNVMe(n), WithLUKS(&expandLUKS{}), WithFilesystem(f))

		err := d.Expand(context.Background(), "pvc-1", expandState(filesystem.Xfs, false), expandSpec())

		require.NoError(t, err)
		assert.Equal(t, []expandCall{{"/dev/nvme0n1", expandStagingPath, "discard", filesystem.Xfs}}, f.calls)
	})

	t.Run("no growth despite the device growing", func(t *testing.T) {
		n := &expandNVMe{sizes: []*uint64{size(1000), size(2000)}}
		f := &expandFS{before: 100, after: 100}
		d := New(WithNVMe(n), WithFilesystem(f))

		err := d.Expand(context.Background(), "pvc-1", expandState(filesystem.Ext4, false), expandSpec())

		require.Error(t, err)
		assert.True(t, tridenterrors.IsInternalError(err))
		assert.Equal(t, "filesystem size did not grow", err.Error())
	})

	t.Run("no growth is tolerated when the device did not grow", func(t *testing.T) {
		n := &expandNVMe{sizes: []*uint64{size(1000)}}
		f := &expandFS{before: 100, after: 100}
		d := New(WithNVMe(n), WithFilesystem(f))

		require.NoError(t, d.Expand(context.Background(), "pvc-1", expandState(filesystem.Ext4, false), expandSpec()))
	})

	t.Run("unreadable device size skips the growth check", func(t *testing.T) {
		n := &expandNVMe{sizes: []*uint64{size(1000), nil}}
		f := &expandFS{before: 100, after: 100}
		d := New(WithNVMe(n), WithFilesystem(f))

		require.NoError(t, d.Expand(context.Background(), "pvc-1", expandState(filesystem.Ext4, false), expandSpec()))
	})

	t.Run("empty filesystem type", func(t *testing.T) {
		f := &expandFS{}
		d := New(WithNVMe(&expandNVMe{sizes: []*uint64{size(1000)}}), WithFilesystem(f))

		err := d.Expand(context.Background(), "pvc-1", expandState("", false), expandSpec())

		assert.ErrorContains(t, err, "invalid requested filesystem")
		assert.Empty(t, f.calls)
	})

	t.Run("service failure is an internal error", func(t *testing.T) {
		f := &expandFS{err: errors.New("unable to mount device; mount failed: exit status 32")}
		d := New(WithNVMe(&expandNVMe{sizes: []*uint64{size(1000)}}), WithFilesystem(f))

		err := d.Expand(context.Background(), "pvc-1", expandState(filesystem.Ext4, false), expandSpec())

		require.Error(t, err)
		assert.True(t, tridenterrors.IsInternalError(err))
		assert.Equal(t, "failed to expand filesystem for volume pvc-1: "+
			"unable to mount device; mount failed: exit status 32", err.Error())
	})
}
