// Copyright 2026 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	tridentErrors "github.com/netapp/trident/utils/errors"
	tridentexec "github.com/netapp/trident/utils/exec"
)

type fakeExitError struct {
	code int
	msg  string
}

func (e *fakeExitError) Error() string { return e.msg }
func (e *fakeExitError) ExitCode() int { return e.code }

func newTestAdapter(cmd *mockexec.MockCommand) Adapter {
	return New(WithCommand(cmd), WithFs(afero.NewMemMapFs()))
}

func TestMount_EmptySource(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := newTestAdapter(mockexec.NewMockCommand(ctrl))

	err := a.Mount(context.Background(), "", "/mnt/vol", "")
	assert.Error(t, err)
}

func TestMount_EmptyTarget(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := newTestAdapter(mockexec.NewMockCommand(ctrl))

	err := a.Mount(context.Background(), "/dev/sda1", "", "")
	assert.Error(t, err)
}

func TestMount_NoOptions(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)

	cmd.EXPECT().Execute(
		gomock.Any(), "mount",
		"/dev/sda1", "/mnt/vol",
	).Return([]byte(""), nil)

	a := newTestAdapter(cmd)
	err := a.Mount(context.Background(), "/dev/sda1", "/mnt/vol", "")
	assert.NoError(t, err)
}

func TestMount_WithOptions(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)

	cmd.EXPECT().Execute(
		gomock.Any(), "mount",
		"-o", "ro,noatime", "/dev/dm-0", "/mnt/data",
	).Return([]byte(""), nil)

	a := newTestAdapter(cmd)
	err := a.Mount(context.Background(), "/dev/dm-0", "/mnt/data", "ro,noatime")
	assert.NoError(t, err)
}

func TestMount_OptionsWithDashOPrefix(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)

	// If the caller accidentally passes "-o ro,noatime", the adapter
	// strips the "-o " prefix and re-adds it properly.
	cmd.EXPECT().Execute(
		gomock.Any(), "mount",
		"-o", "ro,noatime", "/dev/sda1", "/mnt/vol",
	).Return([]byte(""), nil)

	a := newTestAdapter(cmd)
	err := a.Mount(context.Background(), "/dev/sda1", "/mnt/vol", "-o ro,noatime")
	assert.NoError(t, err)
}

func TestMount_CommandError_ExitError(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)

	cmd.EXPECT().Execute(
		gomock.Any(), "mount",
		"/dev/sda1", "/mnt/vol",
	).Return([]byte("mount: /mnt/vol: already mounted"), &fakeExitError{code: 32, msg: "exit status 32"})

	a := newTestAdapter(cmd)
	err := a.Mount(context.Background(), "/dev/sda1", "/mnt/vol", "")
	assert.Error(t, err)

	var exitErr tridentexec.ExitError
	if assert.ErrorAs(t, err, &exitErr) {
		assert.Equal(t, 32, exitErr.ExitCode())
	}
}

func TestMount_CommandError_NonExitError(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)

	cmd.EXPECT().Execute(
		gomock.Any(), "mount",
		"/dev/sda1", "/mnt/vol",
	).Return([]byte(""), errors.New("exec: mount not found"))

	a := newTestAdapter(cmd)
	err := a.Mount(context.Background(), "/dev/sda1", "/mnt/vol", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "mount failed")
}

// noOpenFs fails every OpenFile, as opening a device node does when the device cgroup denies it.
type noOpenFs struct {
	afero.Fs
}

func (f noOpenFs) OpenFile(name string, _ int, _ os.FileMode) (afero.File, error) {
	return nil, &os.PathError{Op: "open", Path: name, Err: os.ErrPermission}
}

func TestBindMount_CreatesMissingTarget(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)
	fs := afero.NewMemMapFs()
	target := "/var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/publish/pvc-1/uuid"

	cmd.EXPECT().Execute(gomock.Any(), "mount", "-o", "bind", "/dev/dm-3", target).Return([]byte(""), nil)

	a := New(WithCommand(cmd), WithFs(fs))
	assert.NoError(t, a.BindMount(context.Background(), "/dev/dm-3", target, ""))

	exists, err := afero.Exists(fs, target)
	assert.NoError(t, err)
	assert.True(t, exists)
}

func TestBindMount_ExistingTargetIsNotOpened(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)
	base := afero.NewMemMapFs()
	target := "/var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/publish/pvc-1/uuid"
	assert.NoError(t, afero.WriteFile(base, target, nil, 0o660))

	cmd.EXPECT().Execute(gomock.Any(), "mount", "-o", "bind", "/dev/dm-3", target).Return([]byte(""), nil)

	a := New(WithCommand(cmd), WithFs(noOpenFs{Fs: base}))
	assert.NoError(t, a.BindMount(context.Background(), "/dev/dm-3", target, ""))
}

func TestUmount_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)
	fs := afero.NewMemMapFs()

	_ = afero.WriteFile(fs, "/mnt/vol", []byte{}, 0o644)

	cmd.EXPECT().ExecuteWithTimeout(
		gomock.Any(), "umount", umountTimeout, true, "/mnt/vol",
	).Return([]byte(""), nil)

	a := New(WithCommand(cmd), WithFs(fs))
	err := a.Umount(context.Background(), "/mnt/vol")
	assert.NoError(t, err)

	exists, _ := afero.Exists(fs, "/mnt/vol")
	assert.False(t, exists)
}

func TestUmount_RemoveFailure_IsNotReturned(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)
	mem := afero.NewMemMapFs()

	_ = afero.WriteFile(mem, "/mnt/vol", []byte{}, 0o644)

	cmd.EXPECT().ExecuteWithTimeout(
		gomock.Any(), "umount", umountTimeout, true, "/mnt/vol",
	).Return([]byte(""), nil)

	a := New(WithCommand(cmd), WithFs(afero.NewReadOnlyFs(mem)))
	err := a.Umount(context.Background(), "/mnt/vol")
	assert.NoError(t, err)

	exists, _ := afero.Exists(mem, "/mnt/vol")
	assert.True(t, exists)
}

func TestRemoveMountPoint(t *testing.T) {
	ctx := context.Background()

	t.Run("removes the directory", func(t *testing.T) {
		fs := afero.NewMemMapFs()
		require.NoError(t, fs.MkdirAll("/staging/tmp_mnt", 0o750))

		require.NoError(t, New(WithFs(fs)).RemoveMountPoint(ctx, "/staging/tmp_mnt"))
		exists, _ := afero.Exists(fs, "/staging/tmp_mnt")
		assert.False(t, exists)
	})

	t.Run("missing directory", func(t *testing.T) {
		assert.NoError(t, New(WithFs(afero.NewMemMapFs())).RemoveMountPoint(ctx, "/staging/tmp_mnt"))
	})

	t.Run("remove failure is returned", func(t *testing.T) {
		mem := afero.NewMemMapFs()
		require.NoError(t, mem.MkdirAll("/staging/tmp_mnt", 0o750))

		err := New(WithFs(afero.NewReadOnlyFs(mem))).RemoveMountPoint(ctx, "/staging/tmp_mnt")
		assert.ErrorContains(t, err, "removing mount point /staging/tmp_mnt")
	})
}

func TestUmount_NotMounted_Idempotent(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)
	fs := afero.NewMemMapFs()

	_ = afero.WriteFile(fs, "/mnt/vol", []byte{}, 0o644)

	cmd.EXPECT().ExecuteWithTimeout(
		gomock.Any(), "umount", umountTimeout, true, "/mnt/vol",
	).Return([]byte("umount: /mnt/vol: not mounted"), &fakeExitError{code: 1, msg: "exit status 1"})

	a := New(WithCommand(cmd), WithFs(fs))
	err := a.Umount(context.Background(), "/mnt/vol")
	assert.NoError(t, err)

	exists, _ := afero.Exists(fs, "/mnt/vol")
	assert.False(t, exists)
}

func TestUmount_Error_Structured(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)
	fs := afero.NewMemMapFs()

	cmd.EXPECT().ExecuteWithTimeout(
		gomock.Any(), "umount", umountTimeout, true, "/mnt/vol",
	).Return([]byte("umount: /mnt/vol: target is busy"), &fakeExitError{code: 1, msg: "exit status 1"})

	a := New(WithCommand(cmd), WithFs(fs))
	err := a.Umount(context.Background(), "/mnt/vol")
	assert.Error(t, err)
	assert.True(t, IsTargetBusyError(err))
}

func TestUmount_Timeout_ForceRetry(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)
	fs := afero.NewMemMapFs()

	_ = afero.WriteFile(fs, "/mnt/vol", []byte{}, 0o644)

	gomock.InOrder(
		cmd.EXPECT().ExecuteWithTimeout(
			gomock.Any(), "umount", umountTimeout, true, "/mnt/vol",
		).Return([]byte(""), tridentErrors.TimeoutError("umount timed out")),

		cmd.EXPECT().ExecuteWithTimeout(
			gomock.Any(), "umount", umountTimeout, true, "/mnt/vol", "-f",
		).Return([]byte(""), nil),
	)

	a := New(WithCommand(cmd), WithFs(fs))
	err := a.Umount(context.Background(), "/mnt/vol")
	assert.NoError(t, err)

	exists, _ := afero.Exists(fs, "/mnt/vol")
	assert.False(t, exists)
}

func TestUmount_Timeout_ForceAlsoFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)
	fs := afero.NewMemMapFs()

	gomock.InOrder(
		cmd.EXPECT().ExecuteWithTimeout(
			gomock.Any(), "umount", umountTimeout, true, "/mnt/vol",
		).Return([]byte(""), tridentErrors.TimeoutError("umount timed out")),

		cmd.EXPECT().ExecuteWithTimeout(
			gomock.Any(), "umount", umountTimeout, true, "/mnt/vol", "-f",
		).Return([]byte("umount: /mnt/vol: target is busy"), &fakeExitError{code: 1, msg: "exit status 1"}),
	)

	a := New(WithCommand(cmd), WithFs(fs))
	err := a.Umount(context.Background(), "/mnt/vol")
	assert.Error(t, err)
	assert.True(t, IsTargetBusyError(err))
}

func TestSemanticErrorClassifiers(t *testing.T) {
	cases := map[string]struct {
		err            error
		wantNotMounted bool
		wantBusy       bool
	}{
		"not_mounted_match": {
			err:            &NotMountedError{err: errors.New("wrapped")},
			wantNotMounted: true,
		},
		"busy_match": {
			err:      &TargetBusyError{err: errors.New("wrapped")},
			wantBusy: true,
		},
		"nil error": {
			err: nil,
		},
		"non-mount error": {
			err: errors.New("something else"),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.wantNotMounted, IsNotMountedError(tc.err))
			assert.Equal(t, tc.wantBusy, IsTargetBusyError(tc.err))
		})
	}
}

func TestIsMounted_BothEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	a := newTestAdapter(mockexec.NewMockCommand(ctrl))

	mounted, err := a.IsMounted(context.Background(), "", "")
	assert.Error(t, err)
	assert.False(t, mounted)
	assert.Contains(t, err.Error(), "at least one of source or target must be specified")
}

// Sample /proc/self/mountinfo content for testing.
const sampleMountinfo = `22 1 8:1 / / rw,relatime shared:1 - ext4 /dev/sda1 rw,errors=continue
23 22 0:21 / /proc rw,nosuid,nodev,noexec,relatime shared:5 - proc proc rw
30 22 0:27 / /sys/fs/cgroup ro,nosuid,nodev,noexec shared:9 - tmpfs tmpfs ro
40 22 253:0 / /mnt/data rw,relatime shared:20 - ext4 /dev/dm-0 rw
50 22 253:1 / /mnt/encrypted rw,relatime shared:21 - ext4 /dev/dm-1 rw
`

func TestIsMountedFromReader_SourceAndTarget(t *testing.T) {
	r := strings.NewReader(sampleMountinfo)
	mounted, err := isMountedFromReader(r, "/dev/dm-0", "/mnt/data")
	assert.NoError(t, err)
	assert.True(t, mounted)
}

func TestIsMountedFromReader_SourceOnly(t *testing.T) {
	r := strings.NewReader(sampleMountinfo)
	mounted, err := isMountedFromReader(r, "/dev/sda1", "")
	assert.NoError(t, err)
	assert.True(t, mounted)
}

func TestIsMountedFromReader_TargetOnly(t *testing.T) {
	r := strings.NewReader(sampleMountinfo)
	mounted, err := isMountedFromReader(r, "", "/mnt/encrypted")
	assert.NoError(t, err)
	assert.True(t, mounted)
}

func TestIsMountedFromReader_NoMatch(t *testing.T) {
	r := strings.NewReader(sampleMountinfo)
	mounted, err := isMountedFromReader(r, "/dev/sdb", "/mnt/nonexistent")
	assert.NoError(t, err)
	assert.False(t, mounted)
}

func TestIsMountedFromReader_SourceNotMounted(t *testing.T) {
	r := strings.NewReader(sampleMountinfo)
	mounted, err := isMountedFromReader(r, "/dev/sdc", "")
	assert.NoError(t, err)
	assert.False(t, mounted)
}

func TestIsMountedFromReader_TargetNotMounted(t *testing.T) {
	r := strings.NewReader(sampleMountinfo)
	mounted, err := isMountedFromReader(r, "", "/mnt/nonexistent")
	assert.NoError(t, err)
	assert.False(t, mounted)
}

func TestIsMountedFromReader_SourceWithoutDevPrefix(t *testing.T) {
	// Without the /dev/ prefix, "dm-0" does not match the mountinfo source
	// "/dev/dm-0". Callers must pass either the full path or a RuntimeID.
	r := strings.NewReader(sampleMountinfo)
	mounted, err := isMountedFromReader(r, "dm-0", "")
	assert.NoError(t, err)
	assert.False(t, mounted)

	// Full path matches.
	r = strings.NewReader(sampleMountinfo)
	mounted, err = isMountedFromReader(r, "/dev/dm-0", "")
	assert.NoError(t, err)
	assert.True(t, mounted)
}

func TestIsMountedFromReader_DeletedRootSkipped(t *testing.T) {
	deletedMountinfo := `22 1 8:1 /deleted / rw,relatime shared:1 - ext4 /dev/sda1 rw
40 22 253:0 / /mnt/data rw,relatime shared:20 - ext4 /dev/dm-0 rw
`
	r := strings.NewReader(deletedMountinfo)
	// sda1 entry has deleted root — should be skipped.
	mounted, err := isMountedFromReader(r, "/dev/sda1", "/")
	assert.NoError(t, err)
	assert.False(t, mounted)

	// dm-0 should still match.
	r = strings.NewReader(deletedMountinfo)
	mounted, err = isMountedFromReader(r, "/dev/dm-0", "/mnt/data")
	assert.NoError(t, err)
	assert.True(t, mounted)
}

func TestIsMountedFromReader_ShortLine(t *testing.T) {
	// Lines with fewer than 10 fields should be skipped gracefully.
	shortMountinfo := `22 1 8:1 / / rw
40 22 253:0 / /mnt/data rw,relatime shared:20 - ext4 /dev/dm-0 rw
`
	r := strings.NewReader(shortMountinfo)
	mounted, err := isMountedFromReader(r, "/dev/dm-0", "/mnt/data")
	assert.NoError(t, err)
	assert.True(t, mounted)
}

func TestIsMountedFromReader_NoSeparator(t *testing.T) {
	// Lines without the "-" separator should be skipped.
	badMountinfo := `22 1 8:1 / / rw,relatime shared:1 ext4 /dev/sda1 rw,errors=continue
40 22 253:0 / /mnt/data rw,relatime shared:20 - ext4 /dev/dm-0 rw
`
	r := strings.NewReader(badMountinfo)
	mounted, err := isMountedFromReader(r, "/dev/dm-0", "/mnt/data")
	assert.NoError(t, err)
	assert.True(t, mounted)
}

func TestIsMountedFromReader_EmptyInput(t *testing.T) {
	r := strings.NewReader("")
	mounted, err := isMountedFromReader(r, "/dev/sda1", "")
	assert.NoError(t, err)
	assert.False(t, mounted)
}

func TestIsMountedFromReader_ProcStyleDevice(t *testing.T) {
	// Non-device mount sources (like "proc", "tmpfs") shouldn't match
	// against device paths.
	r := strings.NewReader(sampleMountinfo)
	mounted, err := isMountedFromReader(r, "proc", "")
	assert.NoError(t, err)
	assert.True(t, mounted) // "proc" source does exist in our sample
}

func TestIsMountedFromReader_MountPointWithSpace(t *testing.T) {
	// mountinfo encodes spaces in paths as \040 (octal for ASCII 32).
	// isMountedFromReader must unescape so "/mnt/my data" matches "\040" in mountinfo.
	mountinfo := `22 1 8:1 / /mnt/my\040data rw,relatime shared:1 - ext4 /dev/sda1 rw` + "\n"
	r := strings.NewReader(mountinfo)
	mounted, err := isMountedFromReader(r, "/dev/sda1", "/mnt/my data")
	assert.NoError(t, err)
	assert.True(t, mounted)
}

func TestIsMountedFromReader_MajorMinorFallback(t *testing.T) {
	// When the mount source is /dev/dm-0 but the caller looks for /dev/mapper/mpathX,
	// field-10 string matching fails. Passing the RuntimeID ("253:0") matches
	// field 3 (major:minor) instead — no sysfs lookup required.
	mountinfo := `40 22 253:0 / /mnt/data rw,relatime shared:20 - ext4 /dev/dm-0 rw` + "\n"
	r := strings.NewReader(mountinfo)

	// String match of an alias misses.
	mounted, err := isMountedFromReader(r, "/dev/mapper/mpathX", "/mnt/data")
	assert.NoError(t, err)
	assert.False(t, mounted)

	// RuntimeID ("253:0") matches field 3 directly.
	r = strings.NewReader(mountinfo)
	mounted, err = isMountedFromReader(r, "253:0", "/mnt/data")
	assert.NoError(t, err)
	assert.True(t, mounted)
}

// --- protocol-specific IsMounted tests ---

// TestIsMountedFromReader_iSCSIBlock tests iSCSI block devices mounted via
// device-mapper multipath. The kernel records the dm device (e.g. /dev/dm-0)
// as the source in field 10, but the caller identifies the device by its
// RuntimeID ("major:minor") to avoid alias mismatches.
func TestIsMountedFromReader_iSCSIBlock(t *testing.T) {
	// Realistic mountinfo for an iSCSI volume formatted ext4 and mounted at a
	// kubelet staging path. Field 3 is "253:3", field 10 is "/dev/dm-3".
	mountinfo := `36 25 253:3 / /var/lib/kubelet/pods/abc123/volumes/kubernetes.io~iscsi/pvc-xyz` +
		` rw,relatime shared:18 - ext4 /dev/dm-3 rw,data=ordered` + "\n"
	r := strings.NewReader(mountinfo)

	// RuntimeID match (field 3) — the preferred identifier from device.RuntimeID().
	mounted, err := isMountedFromReader(r, "253:3", "/var/lib/kubelet/pods/abc123/volumes/kubernetes.io~iscsi/pvc-xyz")
	assert.NoError(t, err)
	assert.True(t, mounted)

	// String mismatch: caller has the mpath alias, field 10 has the dm name.
	r = strings.NewReader(mountinfo)
	mounted, err = isMountedFromReader(r, "/dev/mapper/mpatha", "/var/lib/kubelet/pods/abc123/volumes/kubernetes.io~iscsi/pvc-xyz")
	assert.NoError(t, err)
	assert.False(t, mounted)
}

// TestIsMountedFromReader_NFSv4 tests NFS volumes mounted via NFSv4.
// The mount source is an export path ("host:/export"); there is no block device
// or major:minor.
func TestIsMountedFromReader_NFSv4(t *testing.T) {
	// NFS mountinfo: field 3 is "0:38" (pseudo device), field 10 is the NFS export path.
	mountinfo := `37 25 0:38 / /var/lib/kubelet/pods/abc123/volumes/pvc-nfs` +
		` rw,relatime shared:19 - nfs4 192.168.1.100:/exports/vol1 rw,vers=4.1` + "\n"
	r := strings.NewReader(mountinfo)

	// NFS path match (field 10).
	mounted, err := isMountedFromReader(r, "192.168.1.100:/exports/vol1",
		"/var/lib/kubelet/pods/abc123/volumes/pvc-nfs")
	assert.NoError(t, err)
	assert.True(t, mounted)

	// Wrong export path does not match.
	r = strings.NewReader(mountinfo)
	mounted, err = isMountedFromReader(r, "192.168.1.100:/exports/other",
		"/var/lib/kubelet/pods/abc123/volumes/pvc-nfs")
	assert.NoError(t, err)
	assert.False(t, mounted)
}

// TestIsMountedFromReader_SMBCifs tests SMB/CIFS shares.
// The mount source is a UNC path ("//host/share"), field 10.
func TestIsMountedFromReader_SMBCifs(t *testing.T) {
	mountinfo := `38 25 0:40 / /var/lib/kubelet/pods/abc123/volumes/pvc-smb` +
		` rw,relatime shared:20 - cifs //192.168.1.200/share rw,vers=3.0` + "\n"
	r := strings.NewReader(mountinfo)

	// UNC path match (field 10).
	mounted, err := isMountedFromReader(r, "//192.168.1.200/share",
		"/var/lib/kubelet/pods/abc123/volumes/pvc-smb")
	assert.NoError(t, err)
	assert.True(t, mounted)
}

// TestIsMountedFromReader_RawBlockBindMount tests raw block bind mounts used
// for Kubernetes raw-block volumes. The kernel records "devtmpfs" as the source
// in field 10. devtmpfs is mounted at /dev, so a bind mount of /dev/sdb has
// root field "/sdb" (the path within devtmpfs, not the full /dev/sdb path).
func TestIsMountedFromReader_RawBlockBindMount(t *testing.T) {
	// Field 2 (major:minor) is "8:16".
	// Field 3 (root) is "/sdb" — devtmpfs path, not the full /dev/sdb.
	// Field 9 (mount source after separator) is "devtmpfs".
	mountinfo := `39 25 8:16 /sdb /var/lib/kubelet/pods/abc123/volumeDevices/pvc-raw` +
		` rw,relatime shared:21 - devtmpfs devtmpfs rw,size=0k` + "\n"
	r := strings.NewReader(mountinfo)

	// RuntimeID match (field 2) — works regardless of the devtmpfs source.
	mounted, err := isMountedFromReader(r, "8:16",
		"/var/lib/kubelet/pods/abc123/volumeDevices/pvc-raw")
	assert.NoError(t, err)
	assert.True(t, mounted)

	// Device path falls back to root field comparison when field 9 is "devtmpfs".
	// source="/dev/sdb": TrimPrefix("/dev/") = "sdb"; TrimPrefix(fields[3]="/sdb", "/") = "sdb" — match.
	r = strings.NewReader(mountinfo)
	mounted, err = isMountedFromReader(r, "/dev/sdb",
		"/var/lib/kubelet/pods/abc123/volumeDevices/pvc-raw")
	assert.NoError(t, err)
	assert.True(t, mounted)
}

// TestIsMountedFromReader_Ext4Block tests an ordinary ext4 filesystem mounted
// on a plain block device — no multipath, no aliases.
func TestIsMountedFromReader_Ext4Block(t *testing.T) {
	mountinfo := `40 25 8:32 / /var/lib/kubelet/pods/abc123/volumes/pvc-ext4` +
		` rw,relatime shared:22 - ext4 /dev/sdc rw,data=ordered` + "\n"
	r := strings.NewReader(mountinfo)

	// Field-10 string match for a simple device path.
	mounted, err := isMountedFromReader(r, "/dev/sdc",
		"/var/lib/kubelet/pods/abc123/volumes/pvc-ext4")
	assert.NoError(t, err)
	assert.True(t, mounted)

	// RuntimeID match.
	r = strings.NewReader(mountinfo)
	mounted, err = isMountedFromReader(r, "8:32",
		"/var/lib/kubelet/pods/abc123/volumes/pvc-ext4")
	assert.NoError(t, err)
	assert.True(t, mounted)
}

// --- parseMountInfoLine unit tests ---

func TestParseMountInfoLine_ValidLine(t *testing.T) {
	line := `40 22 253:0 / /mnt/data rw,relatime shared:20 - ext4 /dev/dm-0 rw,errors=continue`
	mi, ok := parseMountInfoLine(line)
	assert.True(t, ok)
	assert.Equal(t, 40, mi.mountID)
	assert.Equal(t, 22, mi.parentID)
	assert.Equal(t, "253:0", mi.deviceID)
	assert.Equal(t, "/", mi.root)
	assert.Equal(t, "/mnt/data", mi.mountPoint)
	assert.Equal(t, []string{"rw", "relatime"}, mi.options)
	assert.Equal(t, "ext4", mi.fsType)
	assert.Equal(t, "/dev/dm-0", mi.mountSource)
	assert.Equal(t, []string{"rw", "errors=continue"}, mi.superOpts)
}

func TestParseMountInfoLine_SpaceInMountPoint(t *testing.T) {
	line := `22 1 8:1 / /mnt/my\040data rw,relatime shared:1 - ext4 /dev/sda1 rw`
	mi, ok := parseMountInfoLine(line)
	assert.True(t, ok)
	assert.Equal(t, "/mnt/my data", mi.mountPoint)
}

func TestParseMountInfoLine_ShortLine(t *testing.T) {
	_, ok := parseMountInfoLine(`22 1 8:1 / / rw`)
	assert.False(t, ok)
}

func TestParseMountInfoLine_NoSeparator(t *testing.T) {
	line := `22 1 8:1 / / rw,relatime shared:1 ext4 /dev/sda1 rw,errors=continue`
	_, ok := parseMountInfoLine(line)
	assert.False(t, ok)
}

func TestParseMountInfoLine_MalformedMountID(t *testing.T) {
	line := `xx 22 253:0 / /mnt/data rw,relatime shared:20 - ext4 /dev/dm-0 rw`
	_, ok := parseMountInfoLine(line)
	assert.False(t, ok)
}

func TestParseMountInfoLine_MalformedParentID(t *testing.T) {
	line := `40 xx 253:0 / /mnt/data rw,relatime shared:20 - ext4 /dev/dm-0 rw`
	_, ok := parseMountInfoLine(line)
	assert.False(t, ok)
}

func TestParseMountInfoLine_SeparatorAtEnd(t *testing.T) {
	// "-" present but not enough fields after it — should return false.
	line := `40 22 253:0 / /mnt/data rw,relatime shared:20 - ext4`
	_, ok := parseMountInfoLine(line)
	assert.False(t, ok)
}

func TestParseMountInfoLine_NoSuperOptions(t *testing.T) {
	// Lines where no super-options follow mountSource are valid; superOpts is nil.
	line := `36 25 253:3 / /mnt/iscsi rw,relatime shared:18 - ext4 /dev/dm-3`
	mi, ok := parseMountInfoLine(line)
	assert.True(t, ok)
	assert.Equal(t, "/dev/dm-3", mi.mountSource)
	assert.Nil(t, mi.superOpts)
}

func TestParseMountInfoLine_NFSSource(t *testing.T) {
	line := `37 25 0:38 / /mnt/nfs rw,relatime shared:19 - nfs4 192.168.1.100:/exports/vol1 rw,vers=4.1`
	mi, ok := parseMountInfoLine(line)
	assert.True(t, ok)
	assert.Equal(t, "0:38", mi.deviceID)
	assert.Equal(t, "nfs4", mi.fsType)
	assert.Equal(t, "192.168.1.100:/exports/vol1", mi.mountSource)
}

func TestParseMountInfoLine_Devtmpfs(t *testing.T) {
	// Raw block bind mount: source is devtmpfs, root is the device name within devtmpfs.
	line := `39 25 8:16 /sdb /var/lib/kubelet/pods/abc123/volumeDevices/pvc-raw rw,relatime shared:21 - devtmpfs devtmpfs rw,size=0k`
	mi, ok := parseMountInfoLine(line)
	assert.True(t, ok)
	assert.Equal(t, "8:16", mi.deviceID)
	assert.Equal(t, "/sdb", mi.root)
	assert.Equal(t, "devtmpfs", mi.mountSource)
}
