// Copyright 2026 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

func (a *adapter) IsMounted(ctx context.Context, _, _ string) (bool, error) {
	Logc(ctx).Trace(">>>> mount_darwin.IsMounted")
	defer Logc(ctx).Trace("<<<< mount_darwin.IsMounted")
	return false, errors.UnsupportedError("mount is not supported on darwin")
}

func (a *adapter) Mount(ctx context.Context, _, _, _ string) error {
	Logc(ctx).Trace(">>>> mount_darwin.Mount")
	defer Logc(ctx).Trace("<<<< mount_darwin.Mount")
	return errors.UnsupportedError("mount is not supported on darwin")
}

func (a *adapter) BindMount(ctx context.Context, _, _, _ string) error {
	Logc(ctx).Trace(">>>> mount_darwin.BindMount")
	defer Logc(ctx).Trace("<<<< mount_darwin.BindMount")
	return errors.UnsupportedError("mount is not supported on darwin")
}

func (a *adapter) Umount(ctx context.Context, _ string) error {
	Logc(ctx).Trace(">>>> mount_darwin.Umount")
	defer Logc(ctx).Trace("<<<< mount_darwin.Umount")
	return errors.UnsupportedError("umount is not supported on darwin")
}

func (a *adapter) RemoveMountPoint(ctx context.Context, _ string) error {
	Logc(ctx).Trace(">>>> mount_darwin.RemoveMountPoint")
	defer Logc(ctx).Trace("<<<< mount_darwin.RemoveMountPoint")
	return errors.UnsupportedError("mount is not supported on darwin")
}

func (a *adapter) Resize(ctx context.Context, _, _, _ string) error {
	Logc(ctx).Trace(">>>> mount_darwin.Resize")
	defer Logc(ctx).Trace("<<<< mount_darwin.Resize")
	return errors.UnsupportedError("filesystem resize is not supported on darwin")
}

func (a *adapter) Size(ctx context.Context, _ string) (uint64, error) {
	Logc(ctx).Trace(">>>> mount_darwin.Size")
	defer Logc(ctx).Trace("<<<< mount_darwin.Size")
	return 0, errors.UnsupportedError("statfs is not supported on darwin")
}
