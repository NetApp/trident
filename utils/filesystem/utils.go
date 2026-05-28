// Copyright 2026 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/netapp/trident/config"
)

var permsRegex = regexp.MustCompile(`^[0-7]{4}$`)

// VerifyFilesystemSupport checks for a supported file system type
func VerifyFilesystemSupport(fs string) (string, error) {
	fstype := strings.ToLower(fs)
	switch fstype {
	case Xfs, Ext3, Ext4, Raw:
		return fstype, nil
	default:
		return "", fmt.Errorf("unsupported fileSystemType option: %s", fstype)
	}
}

func ValidateOctalUnixPermissions(perms string) error {
	if !permsRegex.MatchString(perms) {
		return fmt.Errorf("%s is not a valid octal unix permissions value", perms)
	}
	return nil
}

// DetermineFSType resolves the effective filesystem type to use for a volume.
// Priority order:
//  1. publishInfoType — use it as-is when provided (explicit caller intent).
//  2. existingType — adopt the type already on disk when it is known (non-empty,
//     non-UnknownFstype), so that subsequent format/mount logic stays consistent.
//  3. volumeMode fallback — when no usable on-disk type is available, default to
//     ext4 for Filesystem volumes or raw for Block volumes. A Filesystem volume
//     whose on-disk type is UnknownFstype still defaults to ext4 but also returns
//     an error because the LUN's existing format is unrecognisable.
func DetermineFSType(publishInfoType, existingType, volumeMode string) (string, error) {
	fsType := publishInfoType
	if publishInfoType == "" {
		switch {
		case existingType != "" && existingType != UnknownFstype:
			// Adopt whatever is on disk so subsequent format/mount logic uses it.
			fsType = existingType
		case volumeMode == string(config.Filesystem):
			if existingType == UnknownFstype {
				return "", fmt.Errorf("LUN is formatted with unknown filesystem type")
			}
			fsType = Ext4
		case volumeMode == string(config.RawBlock):
			fsType = Raw
		}
	}
	return fsType, nil
}
