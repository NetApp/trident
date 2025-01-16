//go:build windows || darwin

package syswrap

import (
	"context"
	"os"
	"time"
)

func Exists(_ context.Context, path string, _ time.Duration) (bool, error) {
	_, err := os.Stat(path)
	return err == nil, nil
}
