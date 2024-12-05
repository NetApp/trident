// Copyright 2024 NetApp, Inc. All Rights Reserved.

package devices

import (
	"os"
)

func PathExists(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return true, nil
	}
	return false, nil
}
