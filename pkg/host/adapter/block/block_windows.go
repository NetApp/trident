// Copyright 2026 NetApp, Inc. All Rights Reserved.

package block

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

func (a *adapter) DeviceByName(ctx context.Context, _ string) (*Device, error) {
	Logc(ctx).Trace(">>>> block_windows.DeviceByName")
	defer Logc(ctx).Trace("<<<< block_windows.DeviceByName")
	return nil, errors.UnsupportedError("block device operations are not supported on windows")
}

func (a *adapter) DeviceByDMUUID(ctx context.Context, _ string) (*Device, error) {
	Logc(ctx).Trace(">>>> block_windows.DeviceByDMUUID")
	defer Logc(ctx).Trace("<<<< block_windows.DeviceByDMUUID")
	return nil, errors.UnsupportedError("block device operations are not supported on windows")
}

func (a *adapter) DeviceByDMName(ctx context.Context, _ string) (*Device, error) {
	Logc(ctx).Trace(">>>> block_windows.DeviceByDMName")
	defer Logc(ctx).Trace("<<<< block_windows.DeviceByDMName")
	return nil, errors.UnsupportedError("block device operations are not supported on windows")
}

func (a *adapter) Slaves(ctx context.Context, _ string) ([]string, error) {
	Logc(ctx).Trace(">>>> block_windows.Slaves")
	defer Logc(ctx).Trace("<<<< block_windows.Slaves")
	return nil, errors.UnsupportedError("block device operations are not supported on windows")
}

func (a *adapter) Holders(ctx context.Context, _ string) ([]string, error) {
	Logc(ctx).Trace(">>>> block_windows.Holders")
	defer Logc(ctx).Trace("<<<< block_windows.Holders")
	return nil, errors.UnsupportedError("block device operations are not supported on windows")
}

func (a *adapter) RemoveByDMUUID(ctx context.Context, _ string, _ bool) error {
	Logc(ctx).Trace(">>>> block_windows.RemoveByDMUUID")
	defer Logc(ctx).Trace("<<<< block_windows.RemoveByDMUUID")
	return errors.UnsupportedError("dmsetup is not supported on windows")
}

func (a *adapter) Flush(ctx context.Context, _ string) error {
	Logc(ctx).Trace(">>>> block_windows.Flush")
	defer Logc(ctx).Trace("<<<< block_windows.Flush")
	return errors.UnsupportedError("blockdev is not supported on windows")
}

func (a *adapter) Copy(ctx context.Context, _, _ string, _ DataOptions) ([]byte, error) {
	Logc(ctx).Trace(">>>> block_windows.Copy")
	defer Logc(ctx).Trace("<<<< block_windows.Copy")
	return nil, errors.UnsupportedError("dd is not supported on windows")
}

func (a *adapter) Read(ctx context.Context, _ string, _ DataOptions) ([]byte, error) {
	Logc(ctx).Trace(">>>> block_windows.Read")
	defer Logc(ctx).Trace("<<<< block_windows.Read")
	return nil, errors.UnsupportedError("dd is not supported on windows")
}

func (a *adapter) Zero(ctx context.Context, _ string, _ DataOptions) error {
	Logc(ctx).Trace(">>>> block_windows.Zero")
	defer Logc(ctx).Trace("<<<< block_windows.Zero")
	return errors.UnsupportedError("dd is not supported on windows")
}

func (a *adapter) Type(ctx context.Context, _ string) (string, error) {
	Logc(ctx).Trace(">>>> block_windows.Type")
	defer Logc(ctx).Trace("<<<< block_windows.Type")
	return "", errors.UnsupportedError("blkid is not supported on windows")
}

func (a *adapter) Format(ctx context.Context, _, _, _ string) error {
	Logc(ctx).Trace(">>>> block_windows.Format")
	defer Logc(ctx).Trace("<<<< block_windows.Format")
	return errors.UnsupportedError("mkfs is not supported on windows")
}

func (a *adapter) Repair(ctx context.Context, _, _ string) error {
	Logc(ctx).Trace(">>>> block_windows.Repair")
	defer Logc(ctx).Trace("<<<< block_windows.Repair")
	return errors.UnsupportedError("fsck is not supported on windows")
}

func (a *adapter) Wipe(ctx context.Context, _ string, _ EraseOptions) error {
	Logc(ctx).Trace(">>>> block_windows.Wipe")
	defer Logc(ctx).Trace("<<<< block_windows.Wipe")
	return errors.UnsupportedError("wipefs is not supported on windows")
}
