// Copyright 2026 NetApp, Inc. All Rights Reserved.

//go:build linux

package filesystem_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/pkg/host/adapter/filesystem"
	"github.com/netapp/trident/pkg/host/test"
)

func newFS(t *testing.T) (*test.FakeHost, *filesystem.FakeAdapter) {
	t.Helper()
	fh := test.NewFakeHost()
	return fh, filesystem.NewFake(fh)
}

func TestFakeAdapter_MountThenIsMounted(t *testing.T) {
	ctx := context.Background()
	_, ff := newFS(t)

	mounted, err := ff.IsMounted(ctx, "/dev/dm-3", "/mnt/vol")
	require.NoError(t, err)
	assert.False(t, mounted, "nothing mounted yet")

	require.NoError(t, ff.Mount(ctx, "/dev/dm-3", "/mnt/vol", "ro,noatime"))

	mounted, err = ff.IsMounted(ctx, "/dev/dm-3", "/mnt/vol")
	require.NoError(t, err)
	assert.True(t, mounted, "mount should be visible via the real mountinfo parser")

	// Source-only and target-only queries also match.
	mounted, _ = ff.IsMounted(ctx, "/dev/dm-3", "")
	assert.True(t, mounted)
	mounted, _ = ff.IsMounted(ctx, "", "/mnt/vol")
	assert.True(t, mounted)
}

func TestFakeAdapter_UmountRemovesMount(t *testing.T) {
	ctx := context.Background()
	_, ff := newFS(t)

	ff.StageMount("/dev/dm-3", "/mnt/vol", "xfs")
	mounted, _ := ff.IsMounted(ctx, "/dev/dm-3", "/mnt/vol")
	require.True(t, mounted)

	require.NoError(t, ff.Umount(ctx, "/mnt/vol"))
	mounted, _ = ff.IsMounted(ctx, "/dev/dm-3", "/mnt/vol")
	assert.False(t, mounted, "umount should drop the mountinfo line")

	// Umount of an already-unmounted target is tolerated (NotMounted).
	require.NoError(t, ff.Umount(ctx, "/mnt/vol"))
}

func TestFakeAdapter_BindMountVisible(t *testing.T) {
	ctx := context.Background()
	_, ff := newFS(t)

	require.NoError(t, ff.BindMount(ctx, "/dev/dm-3", "/mnt/bind", ""))
	mounted, err := ff.IsMounted(ctx, "/dev/dm-3", "/mnt/bind")
	require.NoError(t, err)
	assert.True(t, mounted)
}

func TestFakeAdapter_Resize(t *testing.T) {
	ctx := context.Background()
	_, ff := newFS(t)

	ff.StageMount("/dev/dm-3", "/mnt/vol", "ext4")
	require.NoError(t, ff.Resize(ctx, "/dev/dm-3", "/mnt/vol", "ext4"))
	require.NoError(t, ff.Resize(ctx, "/dev/dm-3", "/mnt/vol", "xfs"))
}

func TestFakeAdapter_Size(t *testing.T) {
	ctx := context.Background()
	_, ff := newFS(t)

	_, err := ff.Size(ctx, "/mnt/vol")
	assert.Error(t, err, "unstaged size is an error")

	ff.StageFilesystemSize("/mnt/vol", 1<<30)
	sz, err := ff.Size(ctx, "/mnt/vol")
	require.NoError(t, err)
	assert.Equal(t, uint64(1<<30), sz)
}
