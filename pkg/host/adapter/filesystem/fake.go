// Copyright 2026 NetApp, Inc. All Rights Reserved.

//go:build linux

// The filesystem adapter mixes an fs seam (it reads /proc/self/mountinfo and
// creates/removes mount-point paths through afero) with an exec seam (mount,
// umount, resize2fs, xfs_growfs). This fake embeds the REAL adapter wired to a
// shared *test.FakeHost so IsMounted runs the production mountinfo parser over
// the host substrate, and routes the mount/umount CLIs through the exec router
// into handlers that mutate that same /proc/self/mountinfo. The result: Mount
// then IsMounted, or Umount then IsMounted, agree because they read and write
// one mount table.
//
// Size is the one method that cannot be faked: it calls syscall.Statfs on a real
// path, bypassing both seams. The fake overrides it with a staged value map.
//
// It is linux-tagged because it reuses the linux adapter's mountinfo parser and
// path constant; the handlers are otherwise platform-neutral.

package filesystem

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/spf13/afero"

	"github.com/netapp/trident/pkg/host/test"
)

var _ Adapter = (*FakeAdapter)(nil)

type FakeAdapter struct {
	Adapter
	fh *test.FakeHost

	mu    sync.Mutex
	sizes map[string]uint64 // path -> bytes, for the un-fakeable Size()
}

// NewFake builds a filesystem FakeAdapter over the shared host. It seeds an
// empty /proc/self/mountinfo so IsMounted succeeds before any mount, and
// registers the mount/umount/resize exec handlers.
func NewFake(fh *test.FakeHost) *FakeAdapter {
	fa := &FakeAdapter{
		Adapter: New(WithFs(fh), WithCommand(fh)),
		fh:      fh,
		sizes:   make(map[string]uint64),
	}
	fa.fh.Stage(func(fs afero.Fs) {
		if ok, _ := afero.Exists(fs, procSelfMountInfoPath); !ok {
			_ = writeFileFake(fs, procSelfMountInfoPath, nil)
		}
	})
	fa.registerHandlers()
	return fa
}

// FakeHost exposes the shared host so callers can compose multiple adapters over
// one substrate.
func (fa *FakeAdapter) FakeHost() *test.FakeHost { return fa.fh }

func (fa *FakeAdapter) registerHandlers() {
	fa.fh.OnExec("mount", handleMount)
	fa.fh.OnExec("umount", handleUmount)
	// Resize is a no-op against the in-memory substrate; the growth it performs
	// is in the (un-fakeable) statfs size, which StageFilesystemSize models.
	fa.fh.OnExec("resize2fs", handleResizeNoop)
	fa.fh.OnExec("xfs_growfs", handleResizeNoop)
}

// ---- Stage* seeders ------------------------------------------------------

// StageMount seeds a mount of source at target (fsType optional, default ext4)
// directly into /proc/self/mountinfo, for IsMounted/Resize scenarios that don't
// go through Mount. It also creates the target path so Umount can remove it.
func (fa *FakeAdapter) StageMount(source, target, fsType string) {
	fa.fh.Stage(func(fs afero.Fs) {
		_ = fs.MkdirAll(target, 0o750)
		appendMountLine(fs, source, target, fsType)
	})
}

// StageFilesystemSize sets the byte capacity Size() reports for a mount path.
// Size() calls syscall.Statfs on the real host, which afero cannot model, so it
// is served from this staged map instead.
func (fa *FakeAdapter) StageFilesystemSize(path string, bytes uint64) {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	fa.sizes[path] = bytes
}

// ---- overridden un-fakeable method ---------------------------------------

// Size returns the staged capacity for path. Unlike every other method it does
// not touch the shared substrate (the real adapter calls syscall.Statfs), so it
// is served entirely from the staged map; an unstaged path is an error.
func (fa *FakeAdapter) Size(_ context.Context, path string) (uint64, error) {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	if sz, ok := fa.sizes[path]; ok {
		return sz, nil
	}
	return 0, fmt.Errorf("fake filesystem size not staged for %s (use StageFilesystemSize)", path)
}

// ---- fake exec handlers --------------------------------------------------

// handleMount models `mount [-o opts] <source> <target>`: it appends a mountinfo
// line so a subsequent IsMounted observes the mount. The real adapter has
// already created the target path before calling this.
func handleMount(_ context.Context, fs afero.Fs, _ string, args []string) ([]byte, error) {
	source, target := mountSourceTarget(args)
	if source == "" || target == "" {
		return nil, &test.ExitError{Code: 1, Msg: "mount: source and target required"}
	}
	appendMountLine(fs, source, target, "")
	return nil, nil
}

// handleUmount models `umount <target> [-f]`: it drops the mountinfo line for
// target. An unmounted target exits non-zero with "not mounted", which the real
// adapter classifies as NotMountedError and tolerates.
func handleUmount(_ context.Context, fs afero.Fs, _ string, args []string) ([]byte, error) {
	target := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			target = a
			break
		}
	}
	if target == "" {
		return nil, &test.ExitError{Code: 1, Msg: "umount: target required"}
	}
	if !removeMountLine(fs, target) {
		return []byte("umount: " + target + ": not mounted"),
			&test.ExitError{Code: 1, Msg: "umount failed"}
	}
	return nil, nil
}

func handleResizeNoop(_ context.Context, _ afero.Fs, _ string, _ []string) ([]byte, error) {
	return nil, nil
}

// ---- mountinfo helpers ---------------------------------------------------

// mountSourceTarget extracts the source and target from a mount argv, which is
// either "[-o opts] source target" — the last two positional args.
func mountSourceTarget(args []string) (source, target string) {
	var pos []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-o" {
			i++ // skip the option value
			continue
		}
		if strings.HasPrefix(args[i], "-") {
			continue
		}
		pos = append(pos, args[i])
	}
	if len(pos) < 2 {
		return "", ""
	}
	return pos[len(pos)-2], pos[len(pos)-1]
}

// appendMountLine writes one synthetic /proc/self/mountinfo line. The format
// matches what parseMountInfoLine expects: 10+ fields with a "-" separator, the
// source in the mountSource field and the target in the mountPoint field so
// IsMounted matches on either.
func appendMountLine(fs afero.Fs, source, target, fsType string) {
	if fsType == "" {
		fsType = "ext4"
	}
	lines := readMountLines(fs)
	id := 36 + len(lines)
	esc := strings.ReplaceAll(target, " ", mountinfoSpaceEscape)
	line := fmt.Sprintf("%d %d 0:0 / %s rw shared:1 - %s %s rw", id, id-1, esc, fsType, source)
	lines = append(lines, line)
	writeMountLines(fs, lines)
}

// removeMountLine drops every mountinfo line whose mount point equals target.
// Returns true if at least one line was removed.
func removeMountLine(fs afero.Fs, target string) bool {
	lines := readMountLines(fs)
	kept := make([]string, 0, len(lines))
	removed := false
	for _, l := range lines {
		if mi, ok := parseMountInfoLine(l); ok && mi.mountPoint == target {
			removed = true
			continue
		}
		kept = append(kept, l)
	}
	if removed {
		writeMountLines(fs, kept)
	}
	return removed
}

func readMountLines(fs afero.Fs) []string {
	data, err := afero.ReadFile(fs, procSelfMountInfoPath)
	if err != nil || len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func writeMountLines(fs afero.Fs, lines []string) {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	_ = writeFileFake(fs, procSelfMountInfoPath, []byte(b.String()))
}

func writeFileFake(fs afero.Fs, path string, content []byte) error {
	if err := fs.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	return afero.WriteFile(fs, path, content, 0o644)
}

func dirOf(path string) string {
	if i := strings.LastIndexByte(path, '/'); i > 0 {
		return path[:i]
	}
	return "/"
}
