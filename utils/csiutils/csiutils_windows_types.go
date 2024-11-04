// Copyright 2022 NetApp, Inc. All Rights Reserved.

package csiutils

import (
	"context"

	"k8s.io/mount-utils"
)

//go:generate mockgen -destination=../mocks/mock_csiutils/mock_csiutils.go github.com/netapp/trident/csiutils CSIProxyUtils

type CSIProxyUtils interface {
	SMBMount(context.Context, string, string, string, string, string) error
	SMBUnmount(context.Context, string, string) error
	MakeDir(context.Context, string) error
	Rmdir(context.Context, string) error
	IsMountPointMatch(context.Context, mount.MountPoint, string) bool
	ExistsPath(context.Context, string) (bool, error)
	GetAPIVersions(ctx context.Context) string
	EvalHostSymlinks(context.Context, string) (string, error)
	GetFilesystemUsage(context.Context, string) (int64, int64, error)
}
