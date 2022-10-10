// Copyright 2022 NetApp, Inc. All Rights Reserved.

package docker

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/utils"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}

func TestDeriveHostVolumePath_negative(t *testing.T) {
	ctx := context.TODO()

	////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case:  missing both mountinfos
	hostMountInfo := []utils.MountInfo{}
	selfMountInfo := []utils.MountInfo{}
	_, err := deriveHostVolumePath(ctx, hostMountInfo, selfMountInfo)
	assert.Error(t, err, "no error")
	assert.Equal(t, "cannot derive host volume path, missing /proc/1/mountinfo data", err.Error())

	////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case: no data in selfMountInfo
	hostMountInfo = append(hostMountInfo, utils.MountInfo{
		Root:       "/lib/docker/plugins/9722f031f38b0188233463043f8a76b09d6c8b1d194ef46c0b16191f84ccf8e9/propagated-mount",
		MountPoint: "/dev/lib/docker/plugins/9722f031f38b0188233463043f8a76b09d6c8b1d194ef46c0b16191f84ccf8e9/propagated-mount",
	})
	_, err = deriveHostVolumePath(ctx, hostMountInfo, selfMountInfo)
	assert.Equal(t, "cannot derive host volume path, missing /proc/self/mountinfo data", err.Error())

	////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// negative case: missing /var/lib/docker-volumes in selfMountInfo
	selfMountInfo = append(selfMountInfo, utils.MountInfo{
		Root:       "/",
		MountPoint: "/dev/shm",
	})
	_, err = deriveHostVolumePath(ctx, hostMountInfo, selfMountInfo)
	assert.Equal(t, "could not find proc mount entry for /var/lib/docker-volumes", err.Error())
}

func TestDeriveHostVolumePath_positive(t *testing.T) {
	ctx := context.TODO()

	////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// positive case: have all the data we need
	hostMountInfo := []utils.MountInfo{
		{Root: "/", MountPoint: "/dev/shm"},
		{
			Root:       "/lib/docker/plugins/9722f031f38b0188233463043f8a76b09d6c8b1d194ef46c0b16191f84ccf8e9/propagated-mount",
			MountPoint: "/dev/lib/docker/plugins/9722f031f38b0188233463043f8a76b09d6c8b1d194ef46c0b16191f84ccf8e9/propagated-mount",
		},
	}
	selfMountInfo := []utils.MountInfo{
		{Root: "/", MountPoint: "/dev/shm"},
		{
			Root:       "/lib/docker/plugins/9722f031f38b0188233463043f8a76b09d6c8b1d194ef46c0b16191f84ccf8e9/propagated-mount",
			MountPoint: "/var/lib/docker-volumes/netapp",
		},
	}
	hostVolumePath, err := deriveHostVolumePath(ctx, hostMountInfo, selfMountInfo)
	assert.Nil(t, err)
	assert.Equal(t, filepath.FromSlash("/dev/lib/docker/plugins/9722f031f38b0188233463043f8a76b09d6c8b1d194ef46c0b16191f84ccf8e9/propagated-mount"), hostVolumePath)
}
