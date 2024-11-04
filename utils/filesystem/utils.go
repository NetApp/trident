// Copyright 2024 NetApp, Inc. All Rights Reserved.

package filesystem

import (
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// Title minimally replaces the deprecated strings.Title() function.
func Title(str string) string {
	return cases.Title(language.Und, cases.NoLower).String(str)
}

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
