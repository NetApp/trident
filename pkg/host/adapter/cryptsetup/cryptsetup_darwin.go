// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cryptsetup

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

func (a *adapter) Version(ctx context.Context) (string, error) {
	Logc(ctx).Trace(">>>> cryptsetup_darwin.Version")
	defer Logc(ctx).Trace("<<<< cryptsetup_darwin.Version")
	return "", errors.UnsupportedError("luks is not supported on darwin")
}

func (a *adapter) Format(
	ctx context.Context, _ string, _ string, _ FormatOptions,
) (string, error) {
	Logc(ctx).Trace(">>>> cryptsetup_darwin.Format")
	defer Logc(ctx).Trace("<<<< cryptsetup_darwin.Format")
	return "", errors.UnsupportedError("luks is not supported on darwin")
}

func (a *adapter) Open(
	ctx context.Context, _ string, _ string, _ string, _ OpenOptions,
) error {
	Logc(ctx).Trace(">>>> cryptsetup_darwin.Open")
	defer Logc(ctx).Trace("<<<< cryptsetup_darwin.Open")
	return errors.UnsupportedError("luks is not supported on darwin")
}

func (a *adapter) Close(ctx context.Context, _ string) error {
	Logc(ctx).Trace(">>>> cryptsetup_darwin.Close")
	defer Logc(ctx).Trace("<<<< cryptsetup_darwin.Close")
	return errors.UnsupportedError("luks is not supported on darwin")
}

func (a *adapter) Resize(ctx context.Context, _ string, _ string) error {
	Logc(ctx).Trace(">>>> cryptsetup_darwin.Resize")
	defer Logc(ctx).Trace("<<<< cryptsetup_darwin.Resize")
	return errors.UnsupportedError("luks is not supported on darwin")
}

func (a *adapter) IsLUKS(ctx context.Context, _ string) error {
	Logc(ctx).Trace(">>>> cryptsetup_darwin.IsLUKS")
	defer Logc(ctx).Trace("<<<< cryptsetup_darwin.IsLUKS")
	return errors.UnsupportedError("luks is not supported on darwin")
}

func (a *adapter) Status(ctx context.Context, _ string) (string, error) {
	Logc(ctx).Trace(">>>> cryptsetup_darwin.Status")
	defer Logc(ctx).Trace("<<<< cryptsetup_darwin.Status")
	return "", errors.UnsupportedError("luks is not supported on darwin")
}

func (a *adapter) ChangeKey(
	ctx context.Context, _ string, _ string, _, _ string,
) error {
	Logc(ctx).Trace(">>>> cryptsetup_darwin.ChangeKey")
	defer Logc(ctx).Trace("<<<< cryptsetup_darwin.ChangeKey")
	return errors.UnsupportedError("luks is not supported on darwin")
}

func (a *adapter) TestPassphrase(
	ctx context.Context, _ string, _ string, _ string,
) error {
	Logc(ctx).Trace(">>>> cryptsetup_darwin.TestPassphrase")
	defer Logc(ctx).Trace("<<<< cryptsetup_darwin.TestPassphrase")
	return errors.UnsupportedError("luks is not supported on darwin")
}
