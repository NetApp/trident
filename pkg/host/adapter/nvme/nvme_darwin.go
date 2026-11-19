// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

func (a *adapter) Version(ctx context.Context) (string, error) {
	Logc(ctx).Trace(">>>> nvme_darwin.Version")
	defer Logc(ctx).Trace("<<<< nvme_darwin.Version")
	return "", errors.UnsupportedError("nvme is not supported on darwin")
}

func (a *adapter) HostNQN(ctx context.Context) (string, error) {
	Logc(ctx).Trace(">>>> nvme_darwin.HostNQN")
	defer Logc(ctx).Trace("<<<< nvme_darwin.HostNQN")
	return "", errors.UnsupportedError("nvme is not supported on darwin")
}

func (a *adapter) TransportLoaded(ctx context.Context, transport Transport) (bool, error) {
	Logc(ctx).Trace(">>>> nvme_darwin.TransportLoaded")
	defer Logc(ctx).Trace("<<<< nvme_darwin.TransportLoaded")
	return false, errors.UnsupportedError("nvme is not supported on darwin")
}

func (a *adapter) Subsystem(ctx context.Context, nqn string) (*Subsystem, error) {
	Logc(ctx).Trace(">>>> nvme_darwin.Subsystem")
	defer Logc(ctx).Trace("<<<< nvme_darwin.Subsystem")
	return nil, errors.UnsupportedError("nvme is not supported on darwin")
}

func (a *adapter) Connect(ctx context.Context, nqn, addr string, opts ...ConnectOption) error {
	Logc(ctx).Trace(">>>> nvme_darwin.Connect")
	defer Logc(ctx).Trace("<<<< nvme_darwin.Connect")
	return errors.UnsupportedError("nvme is not supported on darwin")
}

func (a *adapter) Disconnect(ctx context.Context, nqn string) error {
	Logc(ctx).Trace(">>>> nvme_darwin.Disconnect")
	defer Logc(ctx).Trace("<<<< nvme_darwin.Disconnect")
	return errors.UnsupportedError("nvme is not supported on darwin")
}

func (a *adapter) DisconnectPath(ctx context.Context, controller string) error {
	Logc(ctx).Trace(">>>> nvme_darwin.DisconnectPath")
	defer Logc(ctx).Trace("<<<< nvme_darwin.DisconnectPath")
	return errors.UnsupportedError("nvme is not supported on darwin")
}

func (a *adapter) Flush(ctx context.Context, devicePath string) error {
	Logc(ctx).Trace(">>>> nvme_darwin.Flush")
	defer Logc(ctx).Trace("<<<< nvme_darwin.Flush")
	return errors.UnsupportedError("nvme is not supported on darwin")
}
