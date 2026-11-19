// Copyright 2026 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

func (a *adapter) IsMounted(ctx context.Context, _, _ string) (bool, error) {
	Logc(ctx).Trace(">>>> mount_windows.IsMounted")
	defer Logc(ctx).Trace("<<<< mount_windows.IsMounted")
	return false, errors.UnsupportedError("mount is not supported on windows")
}

func (a *adapter) Mount(ctx context.Context, _, _, _ string) error {
	Logc(ctx).Trace(">>>> mount_windows.Mount")
	defer Logc(ctx).Trace("<<<< mount_windows.Mount")
	return errors.UnsupportedError("mount is not supported on windows")
}

func (a *adapter) BindMount(ctx context.Context, _, _, _ string) error {
	Logc(ctx).Trace(">>>> mount_windows.BindMount")
	defer Logc(ctx).Trace("<<<< mount_windows.BindMount")
	return errors.UnsupportedError("mount is not supported on windows")
}

func (a *adapter) Umount(ctx context.Context, _ string) error {
	Logc(ctx).Trace(">>>> mount_windows.Umount")
	defer Logc(ctx).Trace("<<<< mount_windows.Umount")
	return errors.UnsupportedError("umount is not supported on windows")
}

func (a *adapter) RemoveMountPoint(ctx context.Context, _ string) error {
	Logc(ctx).Trace(">>>> mount_windows.RemoveMountPoint")
	defer Logc(ctx).Trace("<<<< mount_windows.RemoveMountPoint")
	return errors.UnsupportedError("mount is not supported on windows")
}

func (a *adapter) Resize(ctx context.Context, _, _, _ string) error {
	Logc(ctx).Trace(">>>> mount_windows.Resize")
	defer Logc(ctx).Trace("<<<< mount_windows.Resize")
	return errors.UnsupportedError("filesystem resize is not supported on windows")
}

func (a *adapter) Size(ctx context.Context, _ string) (uint64, error) {
	Logc(ctx).Trace(">>>> mount_windows.Size")
	defer Logc(ctx).Trace("<<<< mount_windows.Size")
	return 0, errors.UnsupportedError("statfs is not supported on windows")
}
