// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package filesystem provides live-filesystem operations: mounting, unmounting,
// expanding, and querying the size of a mounted filesystem.
//
// Format and repair operations belong to the device service; this package only
// acts on filesystems that are already formatted and potentially mounted.
package filesystem

import (
	"context"
	"fmt"
	"path"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/host/adapter/filesystem"
	"github.com/netapp/trident/utils/models"
)

// temporaryMountDir is the directory under the staging path where EnsureExpand mounts the device.
const temporaryMountDir = "tmp_mnt"

type Type = models.FilesystemType

const (
	Raw  = models.FilesystemRaw
	Ext4 = models.FilesystemExt4
	Ext3 = models.FilesystemExt3
	Xfs  = models.FilesystemXFS
)

// Source is a file to mount.
type Source struct {
	// Path is the device path given to mount, such as "/dev/dm-3".
	Path string
	// ID is the device's major:minor, such as "253:3". A filesystem mount of the device is recorded under it
	// whichever path was mounted (/dev/dm-3 or /dev/mapper/X), so it is what finds an existing mount.
	// If empty, Path is matched instead.
	ID string
}

// Service defines the filesystem service API consumed by drivers.
//
// It owns live-filesystem operations: mounting, unmounting, expanding, and
// querying the size of a mounted filesystem. Format and repair operations
// belong to the device service.
type Service interface {
	// EnsureMount mounts source at mountPoint. It is a NOP if source is already mounted there.
	//
	// fsType is the filesystem type (e.g. "ext4", "xfs"). Pass models.FilesystemRaw
	// for raw block volumes; the service translates this to a bind mount.
	EnsureMount(ctx context.Context, source Source, mountPoint, mountOptions string, fsType Type) error

	// EnsureUnmount unmounts a file or directory from a supplied mount point.
	// If the target mount point doesn't exist, this is a NOP.
	EnsureUnmount(ctx context.Context, mountPoint string, force bool) error

	// EnsureExpand grows the filesystem on devicePath to fill the device, and returns the filesystem's size in
	// bytes before and after the resize.
	//
	// The device is mounted at <stagingPath>/tmp_mnt with mountOptions for the whole resize, because the size can
	// only be read on a mount and xfs_growfs needs one. xfs_growfs runs on that mount point and resize2fs on the
	// device. The temporary mount is removed afterward; a failure to remove it is logged and not returned.
	// A mount failure is returned as "unable to mount device; ...". Raw volumes have no filesystem: it returns
	// zero sizes and no error. The resize is idempotent; at full size it reports equal sizes.
	EnsureExpand(
		ctx context.Context, source Source, stagingPath, mountOptions string, fsType Type,
	) (sizeBefore, sizeAfter uint64, err error)

	// RemoveExpandMount removes a temporary mount that EnsureExpand left under stagingPath (an expand that did not
	// finish), and its directory. It is a NOP if neither is there.
	RemoveExpandMount(ctx context.Context, stagingPath string) error

	// IsMounted reports whether source has a filesystem mount at mountPoint.
	// An empty source checks mountPoint only; an empty mountPoint checks for a mount of source anywhere.
	IsMounted(ctx context.Context, source Source, mountPoint string) (bool, error)

	// GetSize returns the total capacity in bytes of the filesystem mounted at mountPoint.
	// The path must be a live mount point; calling on an unmounted directory returns the host filesystem size.
	GetSize(ctx context.Context, mountPoint string) (uint64, error)
}

type service struct {
	filesystem filesystem.Adapter
}

type Option func(*service)

func WithMountAdapter(m filesystem.Adapter) Option { return func(s *service) { s.filesystem = m } }

func New(opts ...Option) Service {
	s := &service{
		filesystem: filesystem.New(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// EnsureMount mounts source at a supplied target mountpoint.
// Raw block volumes are translated to bind mounts by the service.
func (s *service) EnsureMount(ctx context.Context, source Source, mountPoint, mountOptions string, fsType Type) error {
	Logc(ctx).WithFields(LogFields{
		"sourcePath":   source.Path,
		"sourceID":     source.ID,
		"mountPoint":   mountPoint,
		"filesystem":   fsType,
		"mountOptions": mountOptions,
	}).Debug(">>>> filesystem.EnsureMount")
	defer Logc(ctx).Debug("<<<< filesystem.EnsureMount")

	if mountPoint == "" {
		Logc(ctx).Debug("No mountPoint specified; skipping mount.")
		return fmt.Errorf("no mount point specified")
	}

	mounted, err := s.filesystem.IsMounted(ctx, source.mountKey(fsType), mountPoint)
	if err != nil {
		return fmt.Errorf("checking mount state of %s at %s: %w", source.Path, mountPoint, err)
	}
	if mounted {
		Logc(ctx).Debug("Device already mounted at target.")
		return nil
	}

	// Raw block: service translates to a bind mount; adapter creates a file target.
	if fsType == Raw {
		return s.filesystem.BindMount(ctx, source.Path, mountPoint, mountOptions)
	}
	return s.filesystem.Mount(ctx, source.Path, mountPoint, mountOptions)
}

// mountKey returns the mountinfo source that identifies a mount of s with fsType.
// A raw bind mount is recorded under the filesystem holding the device node, not under the device's
// major:minor, so it is found by the device path.
func (s Source) mountKey(fsType Type) string {
	if fsType != Raw && s.ID != "" {
		return s.ID
	}
	return s.Path
}

func (s *service) EnsureUnmount(ctx context.Context, mountPoint string, force bool) error {
	fields := LogFields{"mountPoint": mountPoint, "force": force}
	Logc(ctx).WithFields(fields).Debug(">>>> filesystem.EnsureUnmount")
	defer Logc(ctx).Debug("<<<< filesystem.EnsureUnmount")

	if mounted, err := s.filesystem.IsMounted(ctx, "", mountPoint); err != nil {
		if !force {
			Logc(ctx).WithFields(fields).Error("Failed to check if filesystem is mounted.")
			return fmt.Errorf("checking mount state of %s: %w", mountPoint, err)
		}

		Logc(ctx).WithFields(fields).Warn("Failed to check if filesystem is mounted; ignoring.")
	} else if !mounted {
		Logc(ctx).Debug("Target not mounted; nothing to do.")
		return nil
	}

	// The mount adapter handles "not mounted" idempotently, so a race
	// between the IsMounted check and the actual umount is harmless.
	if err := s.filesystem.Umount(ctx, mountPoint); err != nil {
		if filesystem.IsTargetBusyError(err) {
			Logc(ctx).WithFields(fields).Error("Target is busy; umount failed.")
			return fmt.Errorf("unmounting %s: device busy: %w", mountPoint, err)
		}
		if !force {
			Logc(ctx).WithFields(fields).Error("Failed to umount filesystem at target.")
			return fmt.Errorf("unmounting %s: %w", mountPoint, err)
		}
		Logc(ctx).WithFields(fields).Warn("Failed to umount filesystem at target; ignoring.")
	}

	return nil
}

func (s *service) EnsureExpand(
	ctx context.Context, source Source, stagingPath, mountOptions string, fsType Type,
) (sizeBefore, sizeAfter uint64, err error) {
	devicePath := source.Path
	fields := LogFields{
		"devicePath":   devicePath,
		"sourceID":     source.ID,
		"stagingPath":  stagingPath,
		"mountOptions": mountOptions,
		"filesystem":   fsType,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> filesystem.EnsureExpand")
	defer Logc(ctx).WithFields(fields).Debug("<<<< filesystem.EnsureExpand")

	if fsType == Raw {
		Logc(ctx).Debug("Raw block volume; skipping filesystem expansion.")
		return 0, 0, nil
	}
	if fsType == "" {
		return 0, 0, fmt.Errorf("filesystem type is empty; cannot expand filesystem")
	}
	if devicePath == "" || stagingPath == "" {
		return 0, 0, fmt.Errorf("device path and staging path are required for filesystem expansion")
	}

	tmpMnt := path.Join(stagingPath, temporaryMountDir)
	if err = s.EnsureMount(ctx, source, tmpMnt, mountOptions, fsType); err != nil {
		return 0, 0, fmt.Errorf("unable to mount device; %w", err)
	}
	defer func() {
		if uErr := s.RemoveExpandMount(ctx, stagingPath); uErr != nil {
			Logc(ctx).WithField("mountPoint", tmpMnt).WithError(uErr).
				Warn("Failed to remove the temporary filesystem expand mount point.")
		}
	}()

	if sizeBefore, err = s.filesystem.Size(ctx, tmpMnt); err != nil {
		return 0, 0, err
	}
	if err = s.filesystem.Resize(ctx, devicePath, tmpMnt, fsType); err != nil {
		return 0, 0, err
	}
	if sizeAfter, err = s.filesystem.Size(ctx, tmpMnt); err != nil {
		return 0, 0, err
	}
	return sizeBefore, sizeAfter, nil
}

// RemoveExpandMount removes a temporary mount that EnsureExpand left under stagingPath, and its directory.
func (s *service) RemoveExpandMount(ctx context.Context, stagingPath string) error {
	tmpMnt := path.Join(stagingPath, temporaryMountDir)
	if err := s.EnsureUnmount(ctx, tmpMnt, false); err != nil {
		return err
	}
	return s.filesystem.RemoveMountPoint(ctx, tmpMnt)
}

// IsMounted reports whether source has a filesystem mount at mountPoint.
func (s *service) IsMounted(ctx context.Context, source Source, mountPoint string) (bool, error) {
	return s.filesystem.IsMounted(ctx, source.mountKey(""), mountPoint)
}

// GetSize returns the total capacity in bytes of the filesystem mounted at mountPoint.
func (s *service) GetSize(ctx context.Context, mountPoint string) (uint64, error) {
	return s.filesystem.Size(ctx, mountPoint)
}
