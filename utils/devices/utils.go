// Copyright 2024 NetApp, Inc. All Rights Reserved.

package devices

import (
	"github.com/spf13/afero"
)

func PathExists(fs afero.Fs, path string) (bool, error) {
	if _, err := fs.Stat(path); err == nil {
		return true, nil
	}
	return false, nil
}
