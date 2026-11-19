// Copyright 2026 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/pkg/host/adapter/filesystem"
)

const (
	testStagingPath = "/var/lib/trident/staging/pvc-1"
	testTmpMount    = testStagingPath + "/tmp_mnt"
)

var nvmeSource = Source{Path: "/dev/nvme0n1"}

// fakeAdapter records the calls EnsureExpand makes. Mounts are tracked by mount point.
type fakeAdapter struct {
	filesystem.Adapter
	mounted       map[string]bool
	mountErr      error
	umountErr     error
	removeErr     error
	resizeErr     error
	sizeErrAt     int // the 1-based Size call that fails; 0 never fails
	sizeCalls     int
	sizes         []uint64
	calls         []string
	mountOpts     string
	mountedSource string // the source of the last IsMounted call
}

func (f *fakeAdapter) IsMounted(_ context.Context, source, mountPoint string) (bool, error) {
	f.mountedSource = source
	return f.mounted[mountPoint], nil
}

func (f *fakeAdapter) BindMount(_ context.Context, source, target, _ string) error {
	f.calls = append(f.calls, "bind "+source+" "+target)
	f.mounted[target] = true
	return nil
}

func (f *fakeAdapter) RemoveMountPoint(_ context.Context, target string) error {
	f.calls = append(f.calls, "remove "+target)
	return f.removeErr
}

func (f *fakeAdapter) Mount(_ context.Context, source, target, options string) error {
	f.calls = append(f.calls, "mount "+source+" "+target)
	f.mountOpts = options
	if f.mountErr != nil {
		return f.mountErr
	}
	f.mounted[target] = true
	return nil
}

func (f *fakeAdapter) Umount(_ context.Context, target string) error {
	f.calls = append(f.calls, "umount "+target)
	if f.umountErr != nil {
		return f.umountErr
	}
	delete(f.mounted, target)
	return nil
}

func (f *fakeAdapter) Resize(_ context.Context, devicePath, mountPoint, fsType string) error {
	f.calls = append(f.calls, "resize "+devicePath+" "+mountPoint+" "+fsType)
	return f.resizeErr
}

func (f *fakeAdapter) Size(_ context.Context, path string) (uint64, error) {
	f.calls = append(f.calls, "size "+path)
	f.sizeCalls++
	n := f.sizeCalls
	if n == f.sizeErrAt {
		return 0, errors.New("statfs failed")
	}
	return f.sizes[min(n-1, len(f.sizes)-1)], nil
}

func newFakeAdapter(sizes ...uint64) *fakeAdapter {
	return &fakeAdapter{mounted: map[string]bool{}, sizes: sizes}
}

func TestEnsureExpand(t *testing.T) {
	ctx := context.Background()

	for _, fsType := range []Type{Xfs, Ext4, Ext3} {
		t.Run("grows "+string(fsType)+" on the temporary mount", func(t *testing.T) {
			f := newFakeAdapter(100, 200)
			s := New(WithMountAdapter(f))

			before, after, err := s.EnsureExpand(ctx, nvmeSource, testStagingPath, "discard", fsType)

			require.NoError(t, err)
			assert.Equal(t, uint64(100), before)
			assert.Equal(t, uint64(200), after)
			assert.Equal(t, "discard", f.mountOpts)
			assert.Equal(t, []string{
				"mount /dev/nvme0n1 " + testTmpMount,
				"size " + testTmpMount,
				"resize /dev/nvme0n1 " + testTmpMount + " " + string(fsType),
				"size " + testTmpMount,
				"umount " + testTmpMount,
				"remove " + testTmpMount,
			}, f.calls)
			assert.Empty(t, f.mounted)
		})
	}

	t.Run("raw has no filesystem to grow", func(t *testing.T) {
		f := newFakeAdapter()
		s := New(WithMountAdapter(f))

		before, after, err := s.EnsureExpand(ctx, nvmeSource, testStagingPath, "", Raw)

		require.NoError(t, err)
		assert.Zero(t, before)
		assert.Zero(t, after)
		assert.Empty(t, f.calls)
	})

	t.Run("rejects missing inputs", func(t *testing.T) {
		s := New(WithMountAdapter(newFakeAdapter()))

		_, _, err := s.EnsureExpand(ctx, nvmeSource, testStagingPath, "", "")
		assert.ErrorContains(t, err, "filesystem type is empty")
		_, _, err = s.EnsureExpand(ctx, Source{}, testStagingPath, "", Xfs)
		assert.ErrorContains(t, err, "device path and staging path are required")
		_, _, err = s.EnsureExpand(ctx, nvmeSource, "", "", Xfs)
		assert.ErrorContains(t, err, "device path and staging path are required")
	})

	t.Run("mount failure", func(t *testing.T) {
		f := newFakeAdapter()
		f.mountErr = errors.New("mount failed: exit status 32")
		s := New(WithMountAdapter(f))

		_, _, err := s.EnsureExpand(ctx, nvmeSource, testStagingPath, "", Ext4)

		require.Error(t, err)
		assert.Equal(t, "unable to mount device; mount failed: exit status 32", err.Error())
		assert.Equal(t, []string{"mount /dev/nvme0n1 " + testTmpMount}, f.calls)
	})

	t.Run("resize failure removes the temporary mount", func(t *testing.T) {
		f := newFakeAdapter(100)
		f.resizeErr = errors.New("exit status 1")
		s := New(WithMountAdapter(f))

		_, _, err := s.EnsureExpand(ctx, nvmeSource, testStagingPath, "", Xfs)

		require.Error(t, err)
		assert.Equal(t, "exit status 1", err.Error())
		assert.Equal(t, []string{"umount " + testTmpMount, "remove " + testTmpMount}, f.calls[len(f.calls)-2:])
	})

	t.Run("size read failures remove the temporary mount", func(t *testing.T) {
		for _, failAt := range []int{1, 2} {
			f := newFakeAdapter(100, 200)
			f.sizeErrAt = failAt
			s := New(WithMountAdapter(f))

			_, _, err := s.EnsureExpand(ctx, nvmeSource, testStagingPath, "", Xfs)

			require.Error(t, err)
			assert.Equal(t, "statfs failed", err.Error())
			assert.Equal(t, []string{"umount " + testTmpMount, "remove " + testTmpMount}, f.calls[len(f.calls)-2:])
		}
	})

	t.Run("unmount failure is not returned", func(t *testing.T) {
		f := newFakeAdapter(100, 200)
		f.umountErr = errors.New("busy")
		s := New(WithMountAdapter(f))

		before, after, err := s.EnsureExpand(ctx, nvmeSource, testStagingPath, "", Xfs)

		require.NoError(t, err)
		assert.Equal(t, uint64(100), before)
		assert.Equal(t, uint64(200), after)
	})

	t.Run("a leftover temporary mount is reused and removed", func(t *testing.T) {
		f := newFakeAdapter(100, 200)
		f.mounted[testTmpMount] = true
		s := New(WithMountAdapter(f))

		_, _, err := s.EnsureExpand(ctx, nvmeSource, testStagingPath, "", Xfs)

		require.NoError(t, err)
		assert.NotContains(t, f.calls, "mount /dev/nvme0n1 "+testTmpMount)
		assert.Empty(t, f.mounted)
	})
}

func TestRemoveExpandMount(t *testing.T) {
	ctx := context.Background()

	t.Run("unmounts a leftover temporary mount", func(t *testing.T) {
		f := newFakeAdapter()
		f.mounted[testTmpMount] = true

		require.NoError(t, New(WithMountAdapter(f)).RemoveExpandMount(ctx, testStagingPath))
		assert.Equal(t, []string{"umount " + testTmpMount, "remove " + testTmpMount}, f.calls)
	})

	t.Run("nothing mounted removes the directory", func(t *testing.T) {
		f := newFakeAdapter()

		require.NoError(t, New(WithMountAdapter(f)).RemoveExpandMount(ctx, testStagingPath))
		assert.Equal(t, []string{"remove " + testTmpMount}, f.calls)
	})

	t.Run("umount failure is returned", func(t *testing.T) {
		f := newFakeAdapter()
		f.mounted[testTmpMount] = true
		f.umountErr = errors.New("umount failed")

		assert.Error(t, New(WithMountAdapter(f)).RemoveExpandMount(ctx, testStagingPath))
		assert.NotContains(t, f.calls, "remove "+testTmpMount)
	})

	t.Run("remove failure is returned", func(t *testing.T) {
		f := newFakeAdapter()
		f.removeErr = errors.New("directory not empty")

		assert.ErrorContains(t, New(WithMountAdapter(f)).RemoveExpandMount(ctx, testStagingPath), "directory not empty")
	})
}

func TestEnsureMount_FindsMountBySource(t *testing.T) {
	ctx := context.Background()
	source := Source{Path: "/dev/dm-3", ID: "253:3"}

	t.Run("a filesystem mount is found by major:minor", func(t *testing.T) {
		f := newFakeAdapter()
		f.mounted["/target"] = true

		require.NoError(t, New(WithMountAdapter(f)).EnsureMount(ctx, source, "/target", "", Ext4))
		assert.Equal(t, "253:3", f.mountedSource)
		assert.Empty(t, f.calls)
	})

	t.Run("a raw bind mount is found by path", func(t *testing.T) {
		f := newFakeAdapter()

		require.NoError(t, New(WithMountAdapter(f)).EnsureMount(ctx, source, "/target", "", Raw))
		assert.Equal(t, "/dev/dm-3", f.mountedSource)
		assert.Equal(t, []string{"bind /dev/dm-3 /target"}, f.calls)
	})

	t.Run("no ID falls back to the path", func(t *testing.T) {
		f := newFakeAdapter()

		require.NoError(t, New(WithMountAdapter(f)).EnsureMount(ctx, Source{Path: "/dev/dm-3"}, "/target", "", Ext4))
		assert.Equal(t, "/dev/dm-3", f.mountedSource)
		assert.Equal(t, []string{"mount /dev/dm-3 /target"}, f.calls)
	})
}
