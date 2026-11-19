// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/pkg/host/service/filesystem"
	"github.com/netapp/trident/pkg/host/service/luks"
	"github.com/netapp/trident/pkg/host/service/nvme"
	tridenterrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

type disconnectCall struct {
	nqn, nsUUID string
	force       bool
}

type detachNVMe struct {
	nvme.Service
	deviceErr   error
	flushOpts   []nvme.FlushOptions
	flushErr    error
	disconnects []disconnectCall
}

func (n *detachNVMe) HasLivePath(context.Context, string) bool { return false }

func (n *detachNVMe) DeviceByUUID(context.Context, string, string) (*nvme.Device, error) {
	if n.deviceErr != nil {
		return nil, n.deviceErr
	}
	return &nvme.Device{}, nil
}

func (n *detachNVMe) EnsureFlush(_ context.Context, _ *nvme.Device, opts nvme.FlushOptions) error {
	n.flushOpts = append(n.flushOpts, opts)
	return n.flushErr
}

func (n *detachNVMe) EnsureDisconnect(_ context.Context, nqn, nsUUID string, force bool) error {
	n.disconnects = append(n.disconnects, disconnectCall{nqn, nsUUID, force})
	return nil
}

type closeCall struct {
	uuid  string
	force bool
}

type detachLUKS struct {
	luks.Service
	closes   []closeCall
	closeErr error
	mapper   *luks.Device
}

func (l *detachLUKS) DeviceByInternalID(context.Context, string) (*luks.Device, error) {
	if l.mapper == nil {
		return nil, tridenterrors.NotFoundError("no mapper")
	}
	return l.mapper, nil
}

func (l *detachLUKS) EnsureClose(_ context.Context, uuid string, force bool) error {
	l.closes = append(l.closes, closeCall{uuid, force})
	return l.closeErr
}

type detachFS struct{ filesystem.Service }

func (detachFS) EnsureUnmount(context.Context, string, bool) error { return nil }
func (detachFS) RemoveExpandMount(context.Context, string) error   { return nil }

func detachState() *models.VolumeAttachState {
	state := &models.VolumeAttachState{}
	state.NVMeSubsystemNQN = "nqn.sub"
	state.NVMeNamespaceUUID = "ns-1"
	state.Encryption = models.EncryptionLUKS
	state.BlockAttachInfo.Devices = models.BlockDeviceStack{
		{ID: "ns-1", Type: models.BlockDeviceDisk},
		{ID: "CRYPT-LUKS2-x", Type: models.BlockDeviceCrypt, Base: "ns-1"},
	}
	return state
}

func TestDetach_PublishedSessions(t *testing.T) {
	ctx := context.Background()

	t.Run("disconnects the subsystem of the last published namespace and removes the mapper", func(t *testing.T) {
		n, l := &detachNVMe{}, &detachLUKS{}
		d := New(WithNVMe(n), WithLUKS(l), WithFilesystem(detachFS{}))
		d.Register(ctx, "pvc-1", trackingInfo("nqn.sub", "ns-1", "1.1.1.1"))

		err := d.Detach(ctx, "pvc-1", detachState(), &models.VolumeDetachSpec{IgnoreFlushErrors: true})

		require.NoError(t, err)
		assert.False(t, d.sessionStore.SessionExists("nqn.sub"))
		assert.Equal(t, []disconnectCall{{"nqn.sub", "ns-1", true}}, n.disconnects)
		assert.Equal(t, []closeCall{{"CRYPT-LUKS2-x", false}, {"CRYPT-LUKS2-x", true}}, l.closes)
		assert.Equal(t, []nvme.FlushOptions{{IgnoreErrors: true}}, n.flushOpts)
	})

	t.Run("keeps the subsystem while another namespace is published", func(t *testing.T) {
		n := &detachNVMe{}
		d := New(WithNVMe(n), WithLUKS(&detachLUKS{}), WithFilesystem(detachFS{}))
		d.Register(ctx, "pvc-1", trackingInfo("nqn.sub", "ns-1", "1.1.1.1"))
		d.Register(ctx, "pvc-2", trackingInfo("nqn.sub", "ns-2", "1.1.1.1"))

		require.NoError(t, d.Detach(ctx, "pvc-1", detachState(), &models.VolumeDetachSpec{}))

		assert.Equal(t, 1, d.sessionStore.GetNamespaceCountForSession("nqn.sub"))
		assert.Empty(t, n.disconnects)
	})

	t.Run("a failed block stack detach keeps the namespace published", func(t *testing.T) {
		n := &detachNVMe{flushErr: &nvme.FlushError{}}
		d := New(WithNVMe(n), WithLUKS(&detachLUKS{}), WithFilesystem(detachFS{}))
		d.Register(ctx, "pvc-1", trackingInfo("nqn.sub", "ns-1", "1.1.1.1"))

		assert.Error(t, d.Detach(ctx, "pvc-1", detachState(), &models.VolumeDetachSpec{}))

		assert.Equal(t, 1, d.sessionStore.GetNamespaceCountForSession("nqn.sub"))
		assert.Empty(t, n.disconnects)
	})
}

func TestDetach_ResolvesLegacyState(t *testing.T) {
	legacyState := func() *models.VolumeAttachState {
		state := detachState()
		state.BlockAttachInfo.Devices = nil
		return state
	}

	t.Run("closes the mapper of a namespace that is gone", func(t *testing.T) {
		n := &detachNVMe{deviceErr: tridenterrors.NotFoundError("namespace gone")}
		l := &detachLUKS{mapper: luks.NewDeviceForTest("dm-0", "luks-trident_pvc_1", "CRYPT-LUKS2-x")}
		d := New(WithNVMe(n), WithLUKS(l), WithFilesystem(detachFS{}))
		state := legacyState()

		err := d.Detach(context.Background(), "pvc-1", state, &models.VolumeDetachSpec{InternalID: "trident_pvc_1"})

		require.NoError(t, err)
		assert.Equal(t, []closeCall{{"CRYPT-LUKS2-x", false}, {"CRYPT-LUKS2-x", true}}, l.closes)
		assert.Equal(t, models.BlockDeviceStack{
			{ID: "CRYPT-LUKS2-x", Type: models.BlockDeviceCrypt, Name: "luks-trident_pvc_1", Base: "ns-1"},
		}, state.BlockAttachInfo.Devices)
		assert.Empty(t, n.flushOpts)
	})

	t.Run("no mapper is looked up without an internal ID", func(t *testing.T) {
		n := &detachNVMe{deviceErr: tridenterrors.NotFoundError("namespace gone")}
		l := &detachLUKS{mapper: luks.NewDeviceForTest("dm-0", "luks-trident_pvc_1", "CRYPT-LUKS2-x")}
		d := New(WithNVMe(n), WithLUKS(l), WithFilesystem(detachFS{}))

		require.NoError(t, d.Detach(context.Background(), "pvc-1", legacyState(), &models.VolumeDetachSpec{}))

		assert.Empty(t, l.closes)
	})

	t.Run("a host read failure fails the detach", func(t *testing.T) {
		readErr := errors.New("sysfs unreadable")
		n := &detachNVMe{deviceErr: readErr}
		d := New(WithNVMe(n), WithLUKS(&detachLUKS{}), WithFilesystem(detachFS{}))

		err := d.Detach(context.Background(), "pvc-1", legacyState(), &models.VolumeDetachSpec{})

		assert.ErrorIs(t, err, readErr)
		assert.Empty(t, n.disconnects)
	})
}

func TestDetach_ForceStillWaitsForLUKSClose(t *testing.T) {
	n, l := &detachNVMe{}, &detachLUKS{}
	d := New(WithNVMe(n), WithLUKS(l), WithFilesystem(detachFS{}))

	require.NoError(t, d.Detach(context.Background(), "pvc-1", detachState(), &models.VolumeDetachSpec{Force: true}))
	assert.Equal(t, closeCall{"CRYPT-LUKS2-x", false}, l.closes[0])
	assert.Equal(t, []nvme.FlushOptions{{Force: true}}, n.flushOpts)
}

func TestDetach_FlushFailureWindow(t *testing.T) {
	ctx := context.Background()
	n := &detachNVMe{flushErr: nvme.NewFlushError("io error", nil)}
	d := New(WithNVMe(n), WithLUKS(&detachLUKS{}), WithFilesystem(detachFS{}))

	err := d.Detach(ctx, "pvc-1", detachState(), &models.VolumeDetachSpec{})
	assert.EqualError(t, err, "failed to flush NVMe device; io error")
	require.Contains(t, d.nvmeFlushFailedAt, "ns-1", "the first failure starts the window")
	assert.Empty(t, n.disconnects)

	d.nvmeFlushFailedAt["ns-1"] = time.Now().Add(-flushMaxWait - time.Second)
	require.NoError(t, d.Detach(ctx, "pvc-1", detachState(), &models.VolumeDetachSpec{}),
		"past the window the detach goes on")
	assert.NotContains(t, d.nvmeFlushFailedAt, "ns-1")
	assert.Len(t, n.disconnects, 1)
}

func TestDetach_LUKSCloseFailureWindow(t *testing.T) {
	ctx := context.Background()
	closeErr := errors.New("busy")
	n := &detachNVMe{}
	d := New(WithNVMe(n), WithLUKS(&detachLUKS{closeErr: closeErr}), WithFilesystem(detachFS{}))

	err := d.Detach(ctx, "pvc-1", detachState(), &models.VolumeDetachSpec{})
	assert.Same(t, closeErr, err)
	require.Contains(t, d.luksCloseFailedAt, "CRYPT-LUKS2-x", "the first failure starts the window")
	assert.Empty(t, n.flushOpts)

	d.luksCloseFailedAt["CRYPT-LUKS2-x"] = time.Now().Add(-luksCloseMaxWait - time.Second)
	require.NoError(t, d.Detach(ctx, "pvc-1", detachState(), &models.VolumeDetachSpec{}),
		"past the window the detach goes on")
	assert.Len(t, n.flushOpts, 1)
	assert.NotContains(t, d.luksCloseFailedAt, "CRYPT-LUKS2-x", "a finished detach clears the window")
}

type expandMountFS struct {
	detachFS
	removed []string
	err     error
}

func (f *expandMountFS) RemoveExpandMount(_ context.Context, stagingPath string) error {
	f.removed = append(f.removed, stagingPath)
	return f.err
}

func TestDetach_RemovesExpandMount(t *testing.T) {
	ctx := context.Background()
	state := detachState()
	state.GlobalMount = "/staging/pvc-1"

	t.Run("removed before the LUKS close", func(t *testing.T) {
		fs, l := &expandMountFS{}, &detachLUKS{}
		d := New(WithNVMe(&detachNVMe{}), WithLUKS(l), WithFilesystem(fs))

		require.NoError(t, d.Detach(ctx, "pvc-1", state, &models.VolumeDetachSpec{}))
		assert.Equal(t, []string{"/staging/pvc-1"}, fs.removed)
	})

	t.Run("a failed removal fails the detach", func(t *testing.T) {
		fs, l := &expandMountFS{err: errors.New("busy")}, &detachLUKS{}
		d := New(WithNVMe(&detachNVMe{}), WithLUKS(l), WithFilesystem(fs))

		err := d.Detach(ctx, "pvc-1", state, &models.VolumeDetachSpec{})
		assert.EqualError(t, err, "failed to remove temporary directory in staging target path /staging/pvc-1; busy")
		assert.Empty(t, l.closes)
	})
}

func TestRemoveMapper(t *testing.T) {
	t.Run("force closes the crypt layer", func(t *testing.T) {
		l := &detachLUKS{closeErr: errors.New("ignored")}
		d := New(WithLUKS(l))

		d.removeMapper(context.Background(), detachState())

		assert.Equal(t, []closeCall{{"CRYPT-LUKS2-x", true}}, l.closes)
	})

	t.Run("no crypt layer", func(t *testing.T) {
		l := &detachLUKS{}
		d := New(WithLUKS(l))
		state := detachState()
		state.BlockAttachInfo.Devices = state.BlockAttachInfo.Devices[:1]

		d.removeMapper(context.Background(), state)

		assert.Empty(t, l.closes)
	})
}
