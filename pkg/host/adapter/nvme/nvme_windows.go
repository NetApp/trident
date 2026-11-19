// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

func (a *adapter) Version(ctx context.Context) (string, error) {
	Logc(ctx).Trace(">>>> nvme_windows.Version")
	defer Logc(ctx).Trace("<<<< nvme_windows.Version")
	return "", errors.UnsupportedError("nvme is not supported on windows")
}

func (a *adapter) HostNQN(ctx context.Context) (string, error) {
	Logc(ctx).Trace(">>>> nvme_windows.HostNQN")
	defer Logc(ctx).Trace("<<<< nvme_windows.HostNQN")
	return "", errors.UnsupportedError("nvme is not supported on windows")
}

func (a *adapter) TransportLoaded(ctx context.Context, transport Transport) (bool, error) {
	Logc(ctx).Trace(">>>> nvme_windows.TransportLoaded")
	defer Logc(ctx).Trace("<<<< nvme_windows.TransportLoaded")
	return false, errors.UnsupportedError("nvme is not supported on windows")
}

func (a *adapter) Subsystem(ctx context.Context, nqn string) (*Subsystem, error) {
	Logc(ctx).Trace(">>>> nvme_windows.Subsystem")
	defer Logc(ctx).Trace("<<<< nvme_windows.Subsystem")
	return nil, errors.UnsupportedError("nvme is not supported on windows")
}

func (a *adapter) Connect(ctx context.Context, nqn, addr string, opts ...ConnectOption) error {
	Logc(ctx).Trace(">>>> nvme_windows.Connect")
	defer Logc(ctx).Trace("<<<< nvme_windows.Connect")
	return errors.UnsupportedError("nvme is not supported on windows")
}

func (a *adapter) Disconnect(ctx context.Context, nqn string) error {
	Logc(ctx).Trace(">>>> nvme_windows.Disconnect")
	defer Logc(ctx).Trace("<<<< nvme_windows.Disconnect")
	return errors.UnsupportedError("nvme is not supported on windows")
}

func (a *adapter) DisconnectPath(ctx context.Context, controller string) error {
	Logc(ctx).Trace(">>>> nvme_windows.DisconnectPath")
	defer Logc(ctx).Trace("<<<< nvme_windows.DisconnectPath")
	return errors.UnsupportedError("nvme is not supported on windows")
}

func (a *adapter) Flush(ctx context.Context, devicePath string) error {
	Logc(ctx).Trace(">>>> nvme_windows.Flush")
	defer Logc(ctx).Trace("<<<< nvme_windows.Flush")
	return errors.UnsupportedError("nvme is not supported on windows")
}
