// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"strings"

	"golang.org/x/net/context"

	. "github.com/netapp/trident/logger"
)

func getFilesystemSize(ctx context.Context, _ string) (int64, error) {
	Logc(ctx).Debug(">>>> filesystem_windows.getFilesystemSize")
	defer Logc(ctx).Debug("<<<< filesystem_windows.getFilesystemSize")
	return 0, UnsupportedError("getFilesystemSize is not supported for windows")
}

// GetDeviceFilePath returns the staging path for volume.
func GetDeviceFilePath(ctx context.Context, path, volumeId string) (string, error) {
	Logc(ctx).Debug(">>>> filesystem_windows.GetDeviceFilePath")
	defer Logc(ctx).Debug("<<<< filesystem_windows.GetDeviceFilePath")

	path = "\\var\\lib\\trident\\tracking" + "\\" + volumeId

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(path, os.ModePerm)
		if err != nil {
			return "", err
		}
	}

	return path, nil
}

// GetUnmountPath returns unmount path for volume
func GetUnmountPath(ctx context.Context, resourcePath, arg string) (string, error) {
	Logc(ctx).Debug(">>>> osutils_windows.GetUnmountPath")
	defer Logc(ctx).Debug("<<<< osutils_windows.GetUnmountPath")

	publishInfo := &VolumePublishInfo{}

	filename := resourcePath + "\\" + arg
	contents, err := ioutil.ReadFile(filename)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal([]byte(contents), &publishInfo)
	if err != nil {
		return "", err
	}

	path := "\\" + publishInfo.SMBServer + publishInfo.SMBPath
	return strings.Replace(path, "/", "\\", -1), nil
}
