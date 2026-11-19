// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package filesystem provides an API for inspecting, mounting and unmounting
// devices and live-filesystems on a host.
package filesystem

import (
	"context"

	"github.com/spf13/afero"

	tridentexec "github.com/netapp/trident/utils/exec"
)

// Adapter wraps the mount binary and /proc/self/mountinfo for mounting
// block devices and querying mount state.
type Adapter interface {
	// IsMounted checks whether source is currently mounted at target.
	// Either source or target may be empty:
	//   - source empty, target set: is anything mounted at target?
	//   - source set, target empty: is source mounted anywhere?
	//   - both set: is source mounted specifically at target?
	IsMounted(ctx context.Context, source, target string) (bool, error)

	// Mount runs the `mount` command to attach source to target.
	// options is a comma-separated mount option string (e.g. "ro,noatime");
	// it may be empty for default mount behaviour.
	Mount(ctx context.Context, source, target, options string) error

	// BindMount runs the `mount` command to bind mount a source device path to a target path.
	// options is a comma-separated mount option string (e.g. "ro,noatime");
	// it may be empty for default mount behaviour.
	BindMount(ctx context.Context, source, target, options string) error

	// Umount runs `umount` command to remove the mount.
	// Any resource at the target path is then removed; a failure to remove it is logged, not returned.
	Umount(ctx context.Context, target string) error

	// RemoveMountPoint removes the unmounted, empty mount point at target. It is a NOP if target does not exist.
	RemoveMountPoint(ctx context.Context, target string) error

	// Resize grows an existing filesystem to fill its underlying block device.
	// The filesystem must be mounted before calling this method.
	Resize(ctx context.Context, devicePath, mountPoint, fsType string) error

	// Size returns the total capacity in bytes of the filesystem mounted at path.
	// The path must be a live mount point.
	Size(ctx context.Context, path string) (uint64, error)
}

type adapter struct {
	cmd tridentexec.Command
	fs  afero.Fs
}

type Option func(*adapter)

func WithCommand(cmd tridentexec.Command) Option { return func(a *adapter) { a.cmd = cmd } }
func WithFs(fs afero.Fs) Option                  { return func(a *adapter) { a.fs = fs } }

func New(opts ...Option) Adapter {
	a := &adapter{
		cmd: tridentexec.NewCommand(),
		fs:  afero.NewOsFs(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}
