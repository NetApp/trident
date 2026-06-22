//go:build windows
// +build windows

// Package unix parses string arguments and calls the system call
package unix

import (
	"fmt"
	"os"
)

func Statfs(args []string) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("expected 1 argument")
	}

	return nil, fmt.Errorf("statfs is not supported on windows")
}

func Exists(args []string) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("expected 1 argument")
	}

	_, err := os.Stat(args[0])
	return new(err == nil), err
}
