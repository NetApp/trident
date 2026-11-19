// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cryptsetup

import (
	"context"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

func (a *adapter) Version(ctx context.Context) (string, error) {
	Logc(ctx).Trace(">>>> cryptsetup_windows.Version")
	defer Logc(ctx).Trace("<<<< cryptsetup_windows.Version")
	return "", errors.UnsupportedError("luks is not supported on windows")
}

func (a *adapter) Format(
	ctx context.Context, _ string, _ string, _ FormatOptions,
) (string, error) {
	Logc(ctx).Trace(">>>> cryptsetup_windows.Format")
	defer Logc(ctx).Trace("<<<< cryptsetup_windows.Format")
	return "", errors.UnsupportedError("luks is not supported on windows")
}

func (a *adapter) Open(
	ctx context.Context, _ string, _ string, _ string, _ OpenOptions,
) error {
	Logc(ctx).Trace(">>>> cryptsetup_windows.Open")
	defer Logc(ctx).Trace("<<<< cryptsetup_windows.Open")
	return errors.UnsupportedError("luks is not supported on windows")
}

func (a *adapter) Close(ctx context.Context, _ string) error {
	Logc(ctx).Trace(">>>> cryptsetup_windows.Close")
	defer Logc(ctx).Trace("<<<< cryptsetup_windows.Close")
	return errors.UnsupportedError("luks is not supported on windows")
}

func (a *adapter) Resize(ctx context.Context, _ string, _ string) error {
	Logc(ctx).Trace(">>>> cryptsetup_windows.Resize")
	defer Logc(ctx).Trace("<<<< cryptsetup_windows.Resize")
	return errors.UnsupportedError("luks is not supported on windows")
}

func (a *adapter) IsLUKS(ctx context.Context, _ string) error {
	Logc(ctx).Trace(">>>> cryptsetup_windows.IsLUKS")
	defer Logc(ctx).Trace("<<<< cryptsetup_windows.IsLUKS")
	return errors.UnsupportedError("luks is not supported on windows")
}

func (a *adapter) Status(ctx context.Context, _ string) (string, error) {
	Logc(ctx).Trace(">>>> cryptsetup_windows.Status")
	defer Logc(ctx).Trace("<<<< cryptsetup_windows.Status")
	return "", errors.UnsupportedError("luks is not supported on windows")
}

func (a *adapter) ChangeKey(
	ctx context.Context, _ string, _ string, _, _ string,
) error {
	Logc(ctx).Trace(">>>> cryptsetup_windows.ChangeKey")
	defer Logc(ctx).Trace("<<<< cryptsetup_windows.ChangeKey")
	return errors.UnsupportedError("luks is not supported on windows")
}

func (a *adapter) TestPassphrase(
	ctx context.Context, _ string, _ string, _ string,
) error {
	Logc(ctx).Trace(">>>> cryptsetup_windows.TestPassphrase")
	defer Logc(ctx).Trace("<<<< cryptsetup_windows.TestPassphrase")
	return errors.UnsupportedError("luks is not supported on windows")
}
