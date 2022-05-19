// Copyright 2017 The Kubernetes Authors.
// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
)

// IsLikelyNotMountPoint uses heuristics to determine if a directory is not a mountpoint.
// It should return ErrNotExist when the directory does not exist.
// IsLikelyNotMountPoint does NOT properly detect all mountpoint types
// most notably Linux bind mounts and symbolic links. For callers that do not
// care about such situations, this is a faster alternative to scanning the list of mounts.
// A return value of false means the directory is definitely a mount point.
// A return value of true means it's not a mount this function knows how to find,
// but it could still be a mount point.
func IsLikelyNotMountPoint(ctx context.Context, mountpoint string) (bool, error) {
	fields := log.Fields{"mountpoint": mountpoint}
	Logc(ctx).WithFields(fields).Debug(">>>> k8s_utils.IsLikelyNotMountPoint")
	defer Logc(ctx).WithFields(fields).Debug("<<<< k8s_utils.IsLikelyNotMountPoint")

	stat, err := os.Stat(mountpoint)
	if err != nil {
		return true, err
	}
	rootStat, err := os.Lstat(filepath.Dir(strings.TrimSuffix(mountpoint, "/")))
	if err != nil {
		return true, err
	}
	// If the directory has a different device as parent, then it is a mountpoint.
	if stat.Sys().(*syscall.Stat_t).Dev != rootStat.Sys().(*syscall.Stat_t).Dev {
		Logc(ctx).WithFields(fields).Debug("Path is a mountpoint.")
		return false, nil
	}

	Logc(ctx).WithFields(fields).Debug("Path is likely not a mountpoint.")
	return true, nil
}
