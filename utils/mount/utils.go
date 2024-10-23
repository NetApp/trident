// Copyright 2024 NetApp, Inc. All Rights Reserved.

package mount

import (
	"context"
	"strings"

	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// sliceContainsString checks to see if a []string contains a string
func sliceContainsString(slice []string, s string) bool {
	return sliceContainsStringConditionally(slice, s, func(val1, val2 string) bool { return val1 == val2 })
}

// sliceContainsStringConditionally checks to see if a []string contains a string based on certain criteria
func sliceContainsStringConditionally(slice []string, s string, fn func(string, string) bool) bool {
	for _, item := range slice {
		if fn(item, s) {
			return true
		}
	}
	return false
}

// checkMountOptions check if the new mount options are different from already mounted options.
// Return an error if there is mismatch with the mount options.
func checkMountOptions(ctx context.Context, procMount models.MountInfo, mountOptions string) error {
	// We have the source already mounted. Compare the mount options from the request.
	optionSlice := strings.Split(strings.TrimPrefix(mountOptions, "-o"), ",")
	for _, option := range optionSlice {
		if option != "" && !areMountOptionsInList(option,
			procMount.MountOptions) && !areMountOptionsInList(option,
			procMount.SuperOptions) {

			return errors.New("mismatch in mount option: " + option +
				", this might cause mount failure")
		}
	}
	return nil
}

// areMountOptionsInList returns true if any of the options are in mountOptions
func areMountOptionsInList(mountOptions string, optionList []string) bool {
	if mountOptions == "" || len(optionList) == 0 {
		return false
	}

	mountOptionsSlice := strings.Split(strings.TrimPrefix(mountOptions, "-o"), ",")

	for _, mountOptionItem := range mountOptionsSlice {
		if sliceContainsString(optionList, mountOptionItem) {
			return true
		}
	}
	return false
}

func normalizeWindowsPath(path string) string {
	normalizedPath := strings.Replace(path, "/", "\\", -1)
	if strings.HasPrefix(normalizedPath, "\\") {
		normalizedPath = "c:" + normalizedPath
	}

	return normalizedPath
}
