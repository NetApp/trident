// Copyright 2026 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/convert"
	tridentutils "github.com/netapp/trident/utils/errors"
)

const (
	devPrefix             = "/dev/"
	procSelfMountInfoPath = "/proc/self/mountinfo"
	umountTimeout         = 10 * time.Second
	umountNotMounted      = "not mounted"
	umountBusyTarget      = "target is busy"

	// mountinfoSpaceEscape is the kernel's octal encoding of ASCII space (0x20)
	// used in /proc/self/mountinfo to avoid ambiguity with the field separator.
	// See proc(5): "spaces are encoded as \040".
	mountinfoSpaceEscape = `\040`
)

// mountInfo represents a single parsed line from /proc/self/mountinfo.
// Fields correspond to the kernel format documented in proc(5).
type mountInfo struct {
	mountID     int
	parentID    int
	deviceID    string   // major:minor
	root        string   // root of mount within the filesystem
	mountPoint  string   // mount point relative to process root
	options     []string // per-mount options
	fsType      string
	mountSource string
	superOpts   []string // per-superblock options
}

// parseMountInfoLine parses one line from /proc/self/mountinfo into a mountInfo.
// Returns false when the line is malformed or the "-" separator is missing.
func parseMountInfoLine(line string) (mountInfo, bool) {
	fields := strings.Fields(line)
	if len(fields) < 10 {
		return mountInfo{}, false
	}

	sepIdx := -1
	for i, f := range fields {
		if f == "-" {
			sepIdx = i
			break
		}
	}
	if sepIdx < 0 || sepIdx+2 >= len(fields) {
		return mountInfo{}, false
	}

	mountID, err := strconv.Atoi(fields[0])
	if err != nil {
		return mountInfo{}, false
	}
	parentID, err := strconv.Atoi(fields[1])
	if err != nil {
		return mountInfo{}, false
	}

	mp := strings.ReplaceAll(fields[4], mountinfoSpaceEscape, " ")

	var superOpts []string
	if sepIdx+3 < len(fields) {
		superOpts = strings.Split(fields[sepIdx+3], ",")
	}

	return mountInfo{
		mountID:     mountID,
		parentID:    parentID,
		deviceID:    fields[2],
		root:        fields[3],
		mountPoint:  mp,
		options:     strings.Split(fields[5], ","),
		fsType:      fields[sepIdx+1],
		mountSource: fields[sepIdx+2],
		superOpts:   superOpts,
	}, true
}

func classifyError(binary string, out []byte, err error) error {
	if binary != "umount" {
		return err
	}

	output := strings.ToLower(string(out))
	if strings.Contains(output, umountNotMounted) {
		return &NotMountedError{err: err}
	}
	if strings.Contains(output, umountBusyTarget) {
		return &TargetBusyError{err: err}
	}

	return err
}

func parseExecError(binary string, out []byte, err error) error {
	if len(out) > 0 {
		return classifyError(binary, out, err)
	}
	return fmt.Errorf("%s failed: %w", binary, err)
}

// IsMounted checks /proc/self/mountinfo for a mount matching source and/or target.
//
// source may be:
//   - a device path ("/dev/dm-3", "/dev/mapper/X") — matched against mountinfo mount source
//   - a major:minor pair ("253:3") — matched against mountinfo field 3
//   - an NFS/SMB export ("192.168.1.1:/export") — matched against mountinfo mount source
//   - "" — any source matches (target-only check)
func (a *adapter) IsMounted(ctx context.Context, source, target string) (bool, error) {
	Logc(ctx).WithFields(LogFields{
		"source": source,
		"target": target,
	}).Trace(">>>> filesystem_linux.IsMounted")
	defer Logc(ctx).Trace("<<<< filesystem_linux.IsMounted")

	if source == "" && target == "" {
		return false, errors.New("at least one of source or target must be specified")
	}

	f, err := a.fs.Open(procSelfMountInfoPath)
	if err != nil {
		return false, fmt.Errorf("opening %s: %w", procSelfMountInfoPath, err)
	}
	defer func() { _ = f.Close() }()

	return isMountedFromReader(f, source, target)
}

// Mount runs the `mount` command to attach source to target.
func (a *adapter) Mount(ctx context.Context, sourcePath, targetPath, options string) error {
	fields := LogFields{
		"sourcePath": sourcePath,
		"targetPath": targetPath,
		"options":    options,
	}
	Logc(ctx).WithFields(fields).Trace(">>>> mount_linux.Mount")
	defer Logc(ctx).WithFields(fields).Trace("<<<< mount_linux.Mount")

	if sourcePath == "" {
		return errors.New("source path is required")
	}
	if targetPath == "" {
		return errors.New("target path is required")
	}

	if err := a.fs.MkdirAll(targetPath, 0o750); err != nil {
		return fmt.Errorf("creating mount point directory %s: %w", targetPath, err)
	}

	return a.mount(ctx, sourcePath, targetPath, options)
}

// BindMount runs the `mount` command to bind mount a source device path to a target path.
func (a *adapter) BindMount(ctx context.Context, source, target, options string) error {
	fields := LogFields{
		"sourcePath": source,
		"targetPath": target,
		"options":    options,
	}
	Logc(ctx).WithFields(fields).Trace(">>>> mount_linux.BindMount")
	defer Logc(ctx).WithFields(fields).Trace("<<<< mount_linux.BindMount")

	if source == "" {
		return errors.New("source path is required")
	}
	if target == "" {
		return errors.New("target path is required")
	}

	if err := a.fs.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return fmt.Errorf("creating parent directory for %s: %w", target, err)
	}

	// Opening an existing target, which may be a device node created by the caller, opens the device itself.
	if _, err := a.fs.Stat(target); os.IsNotExist(err) {
		f, err := a.fs.OpenFile(target, os.O_CREATE, 0o600)
		if err != nil {
			return fmt.Errorf("creating mount point file %s: %w", target, err)
		}
		_ = f.Close()
	} else if err != nil {
		return fmt.Errorf("checking mount point file %s: %w", target, err)
	}

	bindOpts := "bind"
	if options != "" {
		bindOpts = "bind," + options
	}

	return a.mount(ctx, source, target, bindOpts)
}

func (a *adapter) Umount(ctx context.Context, targetPath string) error {
	fields := LogFields{"targetPath": targetPath}
	Logc(ctx).WithFields(fields).Trace(">>>> mount_linux.Umount")
	defer Logc(ctx).WithFields(fields).Trace("<<<< mount_linux.Umount")

	umountErr, timedOut := a.umount(ctx, targetPath, false)
	if umountErr != nil && !IsNotMountedError(umountErr) {
		if !timedOut {
			return umountErr
		}
		Logc(ctx).WithError(umountErr).Error("Umount timed out, attempting force umount.")

		forceErr, _ := a.umount(ctx, targetPath, true)
		if forceErr != nil && !IsNotMountedError(forceErr) {
			return forceErr
		}
	}

	if err := a.fs.Remove(targetPath); err != nil && !os.IsNotExist(err) {
		Logc(ctx).WithFields(fields).WithError(err).Debug("Could not remove mount point after unmount.")
	}
	return nil
}

func (a *adapter) RemoveMountPoint(ctx context.Context, target string) error {
	fields := LogFields{"target": target}
	Logc(ctx).WithFields(fields).Trace(">>>> mount_linux.RemoveMountPoint")
	defer Logc(ctx).WithFields(fields).Trace("<<<< mount_linux.RemoveMountPoint")

	if err := a.fs.Remove(target); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing mount point %s: %w", target, err)
	}
	return nil
}

func (a *adapter) mount(ctx context.Context, source, target, options string) error {
	fields := LogFields{"source": source, "target": target}
	Logc(ctx).WithFields(fields).Trace(">>>> mount_linux.mount")
	defer Logc(ctx).WithFields(fields).Trace("<<<< mount_linux.mount")

	var args []string
	if options != "" {
		sanitized := strings.TrimPrefix(options, "-o ")
		args = []string{"-o", sanitized, source, target}
	} else {
		args = []string{source, target}
	}

	out, err := a.cmd.Execute(ctx, "mount", args...)
	if err != nil {
		return parseExecError("mount", out, err)
	}
	return nil
}

func (a *adapter) umount(ctx context.Context, targetPath string, force bool) (error, bool) {
	args := []string{targetPath}
	if force {
		args = append(args, "-f")
	}

	out, err := a.cmd.ExecuteWithTimeout(ctx, "umount", umountTimeout, true, args...)
	if err == nil {
		return nil, false
	}

	return parseExecError("umount", out, err), tridentutils.IsTimeoutError(err)
}

// isMountedFromReader scans mountinfo-formatted content for a mount matching
// source and/or target. Extracted from IsMounted so the parsing logic can
// be unit-tested with synthetic data.
//
// source is matched against:
//   - mountInfo.deviceID (major:minor, e.g. "253:3")
//   - mountInfo.mountSource (device path or NAS export string)
//
// No sysfs lookups are performed; the caller supplies the appropriate identifier.
func isMountedFromReader(r io.Reader, source, target string) (bool, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		mi, ok := parseMountInfoLine(scanner.Text())
		if !ok {
			continue
		}

		// Skip entries whose root is marked deleted.
		if strings.Contains(mi.root, "deleted") {
			continue
		}

		// Match source against deviceID (major:minor) or mountSource.
		// mountSource handles device paths and NAS export strings.
		// For raw block bind mounts, the kernel records "devtmpfs" as the source; fall
		// back to the root field stripped of its leading slash.
		var sourceMatch bool
		if source == "" {
			sourceMatch = true
		} else {
			sourceMatch = source == mi.deviceID || source == mi.mountSource
			if !sourceMatch && !strings.HasPrefix(mi.mountSource, devPrefix) {
				// Raw block bind mount: device name is in the root field, not mountSource.
				sourceMatch = strings.TrimPrefix(source, devPrefix) == strings.TrimPrefix(mi.root, "/")
			}
		}

		targetMatch := target == "" || mi.mountPoint == target

		if sourceMatch && targetMatch {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("reading mountinfo: %w", err)
	}

	return false, nil
}

// Resize grows an existing filesystem to fill its underlying block device.
// The filesystem must be mounted before calling this method.
func (a *adapter) Resize(ctx context.Context, devicePath, mountPoint, fsType string) error {
	fields := LogFields{
		"devicePath": devicePath,
		"mountPoint": mountPoint,
		"fsType":     fsType,
	}
	Logc(ctx).WithFields(fields).Trace(">>>> mount_linux.Resize")
	defer Logc(ctx).WithFields(fields).Trace("<<<< mount_linux.Resize")

	if fsType == "" {
		return errors.New("filesystem type is required for filesystem resize")
	}

	var bin, target string
	switch fsType {
	case "ext3", "ext4":
		if devicePath == "" {
			return errors.New("device path is required for ext3/ext4 resize")
		}
		bin = "resize2fs"
		target = devicePath
	case "xfs":
		if mountPoint == "" {
			return errors.New("mount point is required for xfs resize")
		}
		bin = "xfs_growfs"
		target = mountPoint
	default:
		return fmt.Errorf("unsupported filesystem type for resize: %s", fsType)
	}

	// Do not hard-kill a resize that is still growing the filesystem.
	_, err := a.cmd.Execute(ctx, bin, target)
	return err
}

// Size returns the total capacity in bytes of the filesystem mounted at path.
func (a *adapter) Size(ctx context.Context, path string) (uint64, error) {
	Logc(ctx).WithField("path", path).Trace(">>>> mount_linux.Size")
	defer Logc(ctx).WithField("path", path).Trace("<<<< mount_linux.Size")

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("statfs failed for path %s: %w", path, err)
	}
	bsize, err := convert.Int64ToUint64(stat.Bsize)
	if err != nil {
		return 0, fmt.Errorf("invalid filesystem block size %d for path %s: %w", stat.Bsize, path, err)
	}
	return stat.Blocks * bsize, nil
}
