// Package unix parses string arguments and calls the system call
package unix

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func Statfs(args []string) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("expected 1 argument")
	}
	var buf unix.Statfs_t
	err := unix.Statfs(args[0], &buf)
	return &buf, err
}

func Exists(args []string) (interface{}, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("expected 1 argument")
	}
	_, err := os.Stat(args[0])
	exists := err == nil
	return &exists, err
}
