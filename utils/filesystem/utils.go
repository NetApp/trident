// Copyright 2025 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"fmt"
	"regexp"
	"strings"
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
