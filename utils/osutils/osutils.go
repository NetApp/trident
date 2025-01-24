// Copyright 2024 NetApp, Inc. All Rights Reserved.

package osutils

//go:generate mockgen -destination=../../mocks/mock_utils/mock_osutils/mock_osutils.go github.com/netapp/trident/utils/osutils Utils
//go:generate mockgen -destination=../../mocks/mock_utils/mock_osutils/mock_netlink.go github.com/netapp/trident/utils/osutils NetLink

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/spf13/afero"

	"github.com/netapp/trident/internal/syswrap"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/models"
)

const (
	Centos = "centos"
	RHEL   = "rhel"
	Ubuntu = "ubuntu"
	Debian = "debian"
)

var ChrootPathPrefix string

type Utils interface {
	NFSActiveOnHost(ctx context.Context) (bool, error)
	ServiceActiveOnHost(ctx context.Context, service string) (bool, error)
	GetHostSystemInfo(ctx context.Context) (*models.HostSystem, error)
	GetIPAddresses(ctx context.Context) ([]string, error)
	PathExists(path string) (bool, error)
	PathExistsWithTimeout(ctx context.Context, path string, timeout time.Duration) (bool, error)
	IsLikelyDir(mountpoint string) (bool, error)
	DeleteResourceAtPath(ctx context.Context, resource string) error
	WaitForResourceDeletionAtPath(ctx context.Context, resource string, maxDuration time.Duration) error
	EnsureFileExists(ctx context.Context, path string) error
	EnsureDirExists(ctx context.Context, path string) error
	EvalSymlinks(path string) (string, error)
}

type OSUtils struct {
	command exec.Command
	osFs    afero.Fs
}

func New() *OSUtils {
	return NewDetailed(exec.NewCommand(), afero.NewOsFs())
}

func NewDetailed(command exec.Command, osFs afero.Fs) *OSUtils {
	return &OSUtils{
		command: command,
		osFs:    osFs,
	}
}

func init() {
	if os.Getenv("DOCKER_PLUGIN_MODE") != "" {
		SetChrootPathPrefix("/host")
	} else {
		SetChrootPathPrefix("")
	}
}

func SetChrootPathPrefix(prefix string) {
	Logc(context.Background()).Debugf("SetChrootPathPrefix = '%s'", prefix)
	ChrootPathPrefix = prefix
}

// GetIPAddresses returns the sorted list of Global Unicast IP addresses available to Trident
func (o *OSUtils) GetIPAddresses(ctx context.Context) ([]string, error) {
	Logc(ctx).Debug(">>>> osutils.GetIPAddresses")
	defer Logc(ctx).Debug("<<<< osutils.GetIPAddresses")

	ipAddrs := make([]string, 0)
	addrsMap := make(map[string]struct{})

	// Get the set of potentially viable IP addresses for this host in an OS-appropriate way.
	addrs, err := o.getIPAddresses(ctx)
	if err != nil {
		err = fmt.Errorf("could not gather system IP addresses; %v", err)
		Logc(ctx).Error(err)
		return nil, err
	}

	// Strip netmask and use a map to ensure addresses are deduplicated.
	for _, addr := range addrs {

		// net.Addr are of form 1.2.3.4/32, but IP needs 1.2.3.4, so we must strip the netmask (also works for IPv6)
		parsedAddr := net.ParseIP(strings.Split(addr.String(), "/")[0])

		Logc(ctx).WithField("IPAddress", parsedAddr.String()).Debug("Discovered potentially viable IP address.")

		addrsMap[parsedAddr.String()] = struct{}{}
	}

	for addr := range addrsMap {
		ipAddrs = append(ipAddrs, addr)
	}
	sort.Strings(ipAddrs)
	return ipAddrs, nil
}

// PathExists returns if path exists. This should only be used if the call will not block if the path or volume is
// inaccessible.
func (o *OSUtils) PathExists(path string) (bool, error) {
	_, err := o.osFs.Stat(path)
	return err == nil, nil
}

// PathExistsWithTimeout returns if path exists, and can return a timeout error on linux; windows ignores the timeout.
// Context timeouts may be ignored and are handled by the underlying implementation. This should be used instead of
// PathExists if there is a chance the call will block if the path or volume is inaccessible, such as on an NFS mount.
func (o *OSUtils) PathExistsWithTimeout(ctx context.Context, path string, timeout time.Duration) (bool, error) {
	return syswrap.Exists(ctx, path, timeout)
}

// EnsureFileExists makes sure that file of given name exists
func (o *OSUtils) EnsureFileExists(ctx context.Context, path string) error {
	fields := LogFields{"path": path}
	if info, err := o.osFs.Stat(path); err == nil {
		if info.IsDir() {
			Logc(ctx).WithFields(fields).Error("Path exists but is a directory")
			return fmt.Errorf("path exists but is a directory: %s", path)
		}
		return nil
	} else if !os.IsNotExist(err) {
		Logc(ctx).WithFields(fields).Errorf("Can't determine if file exists; %s", err)
		return fmt.Errorf("can't determine if file %s exists; %s", path, err)
	}

	file, err := o.osFs.OpenFile(path, os.O_CREATE|os.O_TRUNC, 0o600)
	if nil != err {
		Logc(ctx).WithFields(fields).Errorf("OpenFile failed; %s", err)
		return fmt.Errorf("failed to create file %s; %s", path, err)
	}
	file.Close()

	return nil
}

// DeleteResourceAtPath makes sure that given named file or (empty) directory is removed
func (o *OSUtils) DeleteResourceAtPath(ctx context.Context, resource string) error {
	fields := LogFields{"resource": resource}

	// Check if resource exists
	if _, err := o.osFs.Stat(resource); err != nil {
		if os.IsNotExist(err) {
			Logc(ctx).WithFields(fields).Debugf("Resource not found.")
			return nil
		} else {
			Logc(ctx).WithFields(fields).Debugf("Can't determine if resource exists; %s", err)
			return fmt.Errorf("can't determine if resource %s exists; %s", resource, err)
		}
	}

	// Remove resource
	if err := o.osFs.Remove(resource); err != nil {
		Logc(ctx).WithFields(fields).Debugf("Failed to remove resource, %s", err)
		return fmt.Errorf("failed to remove resource %s; %s", resource, err)
	}

	return nil
}

// WaitForResourceDeletionAtPath accepts a resource name and waits until it is deleted and returns error if it times out
func (o *OSUtils) WaitForResourceDeletionAtPath(ctx context.Context, resource string, maxDuration time.Duration) error {
	fields := LogFields{"resource": resource}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.WaitForResourceDeletionAtPath")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.WaitForResourceDeletionAtPath")

	checkResourceDeletion := func() error {
		return o.DeleteResourceAtPath(ctx, resource)
	}

	deleteNotify := func(err error, duration time.Duration) {
		Logc(ctx).WithField("increment", duration).Debug("Resource not deleted yet, waiting.")
	}

	deleteBackoff := backoff.NewExponentialBackOff()
	deleteBackoff.InitialInterval = 1 * time.Second
	deleteBackoff.Multiplier = 1.414 // approx sqrt(2)
	deleteBackoff.RandomizationFactor = 0.1
	deleteBackoff.MaxElapsedTime = maxDuration

	// Run the check using an exponential backoff
	if err := backoff.RetryNotify(checkResourceDeletion, deleteBackoff, deleteNotify); err != nil {
		return fmt.Errorf("could not delete resource after %3.2f seconds", maxDuration.Seconds())
	} else {
		Logc(ctx).WithField("resource", resource).Debug("Resource deleted.")
		return nil
	}
}

// EnsureDirExists makes sure that given directory structure exists
func (o *OSUtils) EnsureDirExists(ctx context.Context, path string) error {
	fields := LogFields{
		"path": path,
	}
	if info, err := o.osFs.Stat(path); err == nil {
		if !info.IsDir() {
			Logc(ctx).WithFields(fields).Error("Path exists but is not a directory")
			return fmt.Errorf("path exists but is not a directory: %s", path)
		}
		return nil
	} else if !os.IsNotExist(err) {
		Logc(ctx).WithFields(fields).Errorf("Can't determine if directory exists; %s", err)
		return fmt.Errorf("can't determine if directory %s exists; %s", path, err)
	}

	err := o.osFs.MkdirAll(path, 0o755)
	if err != nil {
		Logc(ctx).WithFields(fields).Errorf("Mkdir failed; %s", err)
		return fmt.Errorf("failed to mkdir %s; %s", path, err)
	}

	return nil
}

// Detect if code is running in a container or not
func runningInContainer() bool {
	return os.Getenv("CSI_ENDPOINT") != ""
}

func (o *OSUtils) EvalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
