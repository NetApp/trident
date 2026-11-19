// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package block provides a platform-agnostic API for managing block devices.
// This package allows consumers to observe and interact with block devices on the host.
// It is transport-agnostic and iSCSI, NVMe, FCP, etc. semantics are not modeled here.
package block

import (
	"context"

	"github.com/spf13/afero"

	tridentexec "github.com/netapp/trident/utils/exec"
)

// TopologyAdapter covers block device discovery and DM device lifecycle.
type TopologyAdapter interface {
	// DeviceByName queries the host for a device by kernel name (no leading path).
	DeviceByName(ctx context.Context, deviceName string) (*Device, error)

	// DeviceByDMUUID queries for the dm-* device whose dm/uuid matches.
	DeviceByDMUUID(ctx context.Context, dmUUID string) (*Device, error)

	// DeviceByDMName queries for the dm-* device whose mapper name matches.
	DeviceByDMName(ctx context.Context, dmName string) (*Device, error)

	// Slaves returns the names of devices that are slaves of this device.
	Slaves(ctx context.Context, deviceName string) ([]string, error)

	// Holders returns the names of devices that hold (use) this device.
	Holders(ctx context.Context, deviceName string) ([]string, error)

	// RemoveByDMUUID removes the dm-* device whose dm/uuid matches dmUUID via
	// dmsetup remove -u <dmUUID>. The kernel atomically rejects the ioctl if
	// the UUID no longer matches the dm minor, closing the minor-reuse race
	// present in path-based removal. Callers must ensure no active I/O is in
	// flight (e.g. the device has no slaves) before calling this.
	//
	// When force is true, dmsetup remove -f is passed, which asks the kernel
	// to detach the device even when it is still open. Use only as a last resort.
	RemoveByDMUUID(ctx context.Context, dmUUID string, force bool) error
}

// FormatAdapter covers offline filesystem tools that operate on block devices.
// The device must be unmounted before calling any method here.
type FormatAdapter interface {
	// Type returns the filesystem type for the block device at devicePath.
	// Returns NoFilesystemSignatureError when blkid finds no recognizable signature.
	Type(ctx context.Context, devicePath string) (string, error)

	// Format creates a filesystem on the block device at devicePath.
	// fsType must be one of "ext3", "ext4", or "xfs".
	// options is a space-separated string of additional mkfs flags; it may be empty.
	Format(ctx context.Context, devicePath, fsType, options string) error

	// Repair runs the appropriate fsck tool for fsType on devicePath.
	// The device must be unmounted. Returns FsckFilesystemCorrectedError for
	// non-fatal corrected errors.
	Repair(ctx context.Context, devicePath, fsType string) error

	// Wipe removes the filesystem signature from the block device at devicePath.
	Wipe(ctx context.Context, devicePath string, opts EraseOptions) error
}

// IOAdapter covers raw block device I/O.
type IOAdapter interface {
	// Read reads raw bytes from devicePath and returns them.
	Read(ctx context.Context, devicePath string, opts DataOptions) ([]byte, error)

	// Copy copies raw data from inputPath to outputPath.
	Copy(ctx context.Context, inputPath, outputPath string, opts DataOptions) ([]byte, error)

	// Zero overwrites the leading bytes of devicePath with zeros.
	// opts.Force must be true to confirm the destructive write.
	Zero(ctx context.Context, devicePath string, opts DataOptions) error

	// Flush flushes outstanding I/O for a block device.
	Flush(ctx context.Context, devicePath string) error
}

// Adapter is the full block adapter interface. Services that need all
// capabilities inject this; services that need only a subset inject the
// appropriate sub-interface (IOAdapter, TopologyAdapter, or FormatAdapter).
type Adapter interface {
	TopologyAdapter
	FormatAdapter
	IOAdapter
}

// adapter is a concrete implementation of the Adapter interface.
type adapter struct {
	chrt string
	cmd  tridentexec.Command
	fs   afero.Fs
}

type Option func(*adapter)

func WithChrootPrefix(prefix string) Option      { return func(c *adapter) { c.chrt = prefix } }
func WithCommand(cmd tridentexec.Command) Option { return func(a *adapter) { a.cmd = cmd } }
func WithFs(fs afero.Fs) Option                  { return func(c *adapter) { c.fs = fs } }

func New(opts ...Option) Adapter {
	a := &adapter{
		chrt: "",
		cmd:  tridentexec.NewCommand(),
		fs:   afero.NewOsFs(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}
