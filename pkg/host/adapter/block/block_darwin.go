// Copyright 2026 NetApp, Inc. All Rights Reserved.

package block

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

func (a *adapter) DeviceByName(ctx context.Context, _ string) (*Device, error) {
	Logc(ctx).Trace(">>>> block_darwin.DeviceByName")
	defer Logc(ctx).Trace("<<<< block_darwin.DeviceByName")
	return nil, errors.UnsupportedError("block device operations are not supported on darwin")
}

func (a *adapter) DeviceByDMUUID(ctx context.Context, _ string) (*Device, error) {
	Logc(ctx).Trace(">>>> block_darwin.DeviceByDMUUID")
	defer Logc(ctx).Trace("<<<< block_darwin.DeviceByDMUUID")
	return nil, errors.UnsupportedError("block device operations are not supported on darwin")
}

func (a *adapter) DeviceByDMName(ctx context.Context, _ string) (*Device, error) {
	Logc(ctx).Trace(">>>> block_darwin.DeviceByDMName")
	defer Logc(ctx).Trace("<<<< block_darwin.DeviceByDMName")
	return nil, errors.UnsupportedError("block device operations are not supported on darwin")
}

func (a *adapter) Slaves(ctx context.Context, _ string) ([]string, error) {
	Logc(ctx).Trace(">>>> block_darwin.Slaves")
	defer Logc(ctx).Trace("<<<< block_darwin.Slaves")
	return nil, errors.UnsupportedError("block device operations are not supported on darwin")
}

func (a *adapter) Holders(ctx context.Context, _ string) ([]string, error) {
	Logc(ctx).Trace(">>>> block_darwin.Holders")
	defer Logc(ctx).Trace("<<<< block_darwin.Holders")
	return nil, errors.UnsupportedError("block device operations are not supported on darwin")
}

func (a *adapter) RemoveByDMUUID(ctx context.Context, _ string, _ bool) error {
	Logc(ctx).Trace(">>>> block_darwin.RemoveByDMUUID")
	defer Logc(ctx).Trace("<<<< block_darwin.RemoveByDMUUID")
	return errors.UnsupportedError("dmsetup is not supported on darwin")
}

func (a *adapter) Flush(ctx context.Context, _ string) error {
	Logc(ctx).Trace(">>>> block_darwin.Flush")
	defer Logc(ctx).Trace("<<<< block_darwin.Flush")
	return errors.UnsupportedError("blockdev is not supported on darwin")
}

func (a *adapter) Copy(ctx context.Context, _, _ string, _ DataOptions) ([]byte, error) {
	Logc(ctx).Trace(">>>> block_darwin.Copy")
	defer Logc(ctx).Trace("<<<< block_darwin.Copy")
	return nil, errors.UnsupportedError("dd is not supported on darwin")
}

func (a *adapter) Read(ctx context.Context, _ string, _ DataOptions) ([]byte, error) {
	Logc(ctx).Trace(">>>> block_darwin.Read")
	defer Logc(ctx).Trace("<<<< block_darwin.Read")
	return nil, errors.UnsupportedError("dd is not supported on darwin")
}

func (a *adapter) Zero(ctx context.Context, _ string, _ DataOptions) error {
	Logc(ctx).Trace(">>>> block_darwin.Zero")
	defer Logc(ctx).Trace("<<<< block_darwin.Zero")
	return errors.UnsupportedError("dd is not supported on darwin")
}

func (a *adapter) Type(ctx context.Context, _ string) (string, error) {
	Logc(ctx).Trace(">>>> block_darwin.Type")
	defer Logc(ctx).Trace("<<<< block_darwin.Type")
	return "", errors.UnsupportedError("blkid is not supported on darwin")
}

func (a *adapter) Format(ctx context.Context, _, _, _ string) error {
	Logc(ctx).Trace(">>>> block_darwin.Format")
	defer Logc(ctx).Trace("<<<< block_darwin.Format")
	return errors.UnsupportedError("mkfs is not supported on darwin")
}

func (a *adapter) Repair(ctx context.Context, _, _ string) error {
	Logc(ctx).Trace(">>>> block_darwin.Repair")
	defer Logc(ctx).Trace("<<<< block_darwin.Repair")
	return errors.UnsupportedError("fsck is not supported on darwin")
}

func (a *adapter) Wipe(ctx context.Context, _ string, _ EraseOptions) error {
	Logc(ctx).Trace(">>>> block_darwin.Wipe")
	defer Logc(ctx).Trace("<<<< block_darwin.Wipe")
	return errors.UnsupportedError("wipefs is not supported on darwin")
}
