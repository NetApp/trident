// Package syswrap wraps syscalls that need to be canceled
package syswrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"

	"golang.org/x/sys/unix"

	"github.com/netapp/trident/utils/exec"
)

const syswrapBin = "/syswrap"

func Statfs(ctx context.Context, path string, timeout time.Duration) (unix.Statfs_t, error) {
	buf, err := exec.NewCommand().Execute(ctx, syswrapBin, timeout.String(), "statfs", path)
	if err != nil {
		// If syswrap is unavailable fall back to blocking call. This may hang if NFS backend is unreachable
		var pe *os.PathError
		ok := errors.As(err, &pe)
		if !ok {
			return unix.Statfs_t{}, err
		}

		var fsStat unix.Statfs_t
		err = unix.Statfs(path, &fsStat)
		return fsStat, err
	}

	var b unix.Statfs_t
	err = json.Unmarshal(buf, &b)
	return b, err
}

func Exists(ctx context.Context, path string, timeout time.Duration) (bool, error) {
	buf, err := exec.NewCommand().Execute(ctx, syswrapBin, timeout.String(), "exists", path)
	if err != nil {
		// If syswrap is unavailable fall back to blocking call. This may hang if NFS backend is unreachable
		var pe *os.PathError
		ok := errors.As(err, &pe)
		if !ok {
			return false, err
		}

		_, err = os.Stat(path)
		return err == nil, err
	}

	var b bool
	err = json.Unmarshal(buf, &b)
	return b, err
}
