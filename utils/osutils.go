// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	log "github.com/sirupsen/logrus"

	. "github.com/netapp/trident/logger"
)

var (
	iqnRegex              = regexp.MustCompile(`^\s*InitiatorName\s*=\s*(?P<iqn>\S+)(|\s+.*)$`)
	xtermControlRegex     = regexp.MustCompile(`\x1B\[[0-9;]*[a-zA-Z]`)
	portalPortPattern     = regexp.MustCompile(`.+:\d+$`)
	pidRunningOrIdleRegex = regexp.MustCompile(`pid \d+ (running|idle)`)
	pidRegex              = regexp.MustCompile(`^\d+$`)
	deviceRegex           = regexp.MustCompile(`/dev/(?P<device>[\w-]+)`)

	chrootPathPrefix string

	execCmd = exec.CommandContext
)

func init() {
	if os.Getenv("DOCKER_PLUGIN_MODE") != "" {
		SetChrootPathPrefix("/host")
	} else {
		SetChrootPathPrefix("")
	}
}

func SetChrootPathPrefix(prefix string) {
	Logc(context.Background()).Debugf("SetChrootPathPrefix = '%s'", prefix)
	chrootPathPrefix = prefix
}

// GetIPAddresses returns the sorted list of Global Unicast IP addresses available to Trident
func GetIPAddresses(ctx context.Context) ([]string, error) {
	Logc(ctx).Debug(">>>> osutils.GetIPAddresses")
	defer Logc(ctx).Debug("<<<< osutils.GetIPAddresses")

	ipAddrs := make([]string, 0)
	addrsMap := make(map[string]struct{})

	// Get the set of potentially viable IP addresses for this host in an OS-appropriate way.
	addrs, err := getIPAddresses(ctx)
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

// execCommand invokes an external process
func execCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	Logc(ctx).WithFields(log.Fields{
		"command": name,
		"args":    args,
	}).Debug(">>>> osutils.execCommand.")

	// create context with a cancellation
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	out, err := execCmd(cancelCtx, name, args...).CombinedOutput()

	Logc(ctx).WithFields(log.Fields{
		"command": name,
		"output":  sanitizeExecOutput(string(out)),
		"error":   err,
	}).Debug("<<<< osutils.execCommand.")

	return out, err
}

// execCommandRedacted invokes an external process, and redacts sensitive arguments
func execCommandRedacted(
	ctx context.Context, name string, args []string,
	secretsToRedact map[string]string,
) ([]byte, error) {
	var sanitizedArgs []string
	for _, arg := range args {
		val, ok := secretsToRedact[arg]
		var sanitizedArg string
		if ok {
			sanitizedArg = val
		} else {
			sanitizedArg = arg
		}
		sanitizedArgs = append(sanitizedArgs, sanitizedArg)
	}

	Logc(ctx).WithFields(log.Fields{
		"command": name,
		"args":    sanitizedArgs,
	}).Debug(">>>> osutils.execCommand.")

	// create context with a cancellation
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	out, err := execCmd(cancelCtx, name, args...).CombinedOutput()

	Logc(ctx).WithFields(log.Fields{
		"command": name,
		"output":  sanitizeExecOutput(string(out)),
		"error":   err,
	}).Debug("<<<< osutils.execCommand.")

	return out, err
}

// execCommandResult is used to return shell command results via channels between goroutines
type execCommandResult struct {
	Output []byte
	Error  error
}

// execCommandWithTimeout invokes an external shell command and lets it time out if it exceeds the
// specified interval
func execCommandWithTimeout(
	ctx context.Context, name string, timeoutSeconds time.Duration, logOutput bool, args ...string,
) ([]byte, error) {
	timeout := timeoutSeconds * time.Second

	Logc(ctx).WithFields(log.Fields{
		"command":        name,
		"timeoutSeconds": timeout,
		"args":           args,
	}).Debug(">>>> osutils.execCommandWithTimeout.")

	// create context with a cancellation
	cancelCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := execCmd(cancelCtx, name, args...)
	done := make(chan execCommandResult, 1)
	var result execCommandResult

	go func() {
		out, err := cmd.CombinedOutput()
		done <- execCommandResult{Output: out, Error: err}
	}()

	select {
	case <-cancelCtx.Done():
		if err := cancelCtx.Err(); err == context.DeadlineExceeded {
			Logc(ctx).WithFields(log.Fields{
				"process": name,
			}).Error("process killed after timeout")
			result = execCommandResult{Output: nil, Error: TimeoutError("process killed after timeout")}
		} else {
			Logc(ctx).WithFields(log.Fields{
				"process": name,
				"error":   err,
			}).Error("context ended unexpectedly")
			result = execCommandResult{Output: nil, Error: fmt.Errorf("context ended unexpectedly: %v", err)}
		}
	case result = <-done:
		break
	}

	logFields := Logc(ctx).WithFields(log.Fields{
		"command": name,
		"error":   result.Error,
	})

	if logOutput {
		logFields.WithFields(log.Fields{
			"output": sanitizeExecOutput(string(result.Output)),
		})
	}

	logFields.Debug("<<<< osutils.execCommandWithTimeout.")

	return result.Output, result.Error
}

func sanitizeExecOutput(s string) string {
	// Strip xterm color & movement characters
	s = xtermControlRegex.ReplaceAllString(s, "")
	// Strip trailing newline
	s = strings.TrimSuffix(s, "\n")
	return s
}

func PathExists(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return true, nil
	}
	return false, nil
}

// EnsureFileExists makes sure that file of given name exists
func EnsureFileExists(ctx context.Context, path string) error {
	fields := log.Fields{"path": path}
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			Logc(ctx).WithFields(fields).Error("Path exists but is a directory")
			return fmt.Errorf("path exists but is a directory: %s", path)
		}
		return nil
	} else if !os.IsNotExist(err) {
		Logc(ctx).WithFields(fields).Errorf("Can't determine if file exists; %s", err)
		return fmt.Errorf("can't determine if file %s exists; %s", path, err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC, 0o600)
	if nil != err {
		Logc(ctx).WithFields(fields).Errorf("OpenFile failed; %s", err)
		return fmt.Errorf("failed to create file %s; %s", path, err)
	}
	file.Close()

	return nil
}

// DeleteResourceAtPath makes sure that given named file or (empty) directory is removed
func DeleteResourceAtPath(ctx context.Context, resource string) error {
	fields := log.Fields{"resource": resource}

	// Check if resource exists
	if _, err := os.Stat(resource); err != nil {
		if os.IsNotExist(err) {
			Logc(ctx).WithFields(fields).Debugf("Resource not found.")
			return nil
		} else {
			Logc(ctx).WithFields(fields).Debugf("Can't determine if resource exists; %s", err)
			return fmt.Errorf("can't determine if resource %s exists; %s", resource, err)
		}
	}

	// Remove resource
	if err := os.Remove(resource); err != nil {
		Logc(ctx).WithFields(fields).Debugf("Failed to remove resource, %s", err)
		return fmt.Errorf("failed to remove resource %s; %s", resource, err)
	}

	return nil
}

// WaitForResourceDeletionAtPath accepts a resource name and waits until it is deleted and returns error if it times out
func WaitForResourceDeletionAtPath(ctx context.Context, resource string, maxDuration time.Duration) error {
	fields := log.Fields{"resource": resource}
	Logc(ctx).WithFields(fields).Debug(">>>> osutils.WaitForResourceDeletionAtPath")
	defer Logc(ctx).WithFields(fields).Debug("<<<< osutils.WaitForResourceDeletionAtPath")

	checkResourceDeletion := func() error {
		return DeleteResourceAtPath(ctx, resource)
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
func EnsureDirExists(ctx context.Context, path string) error {
	fields := log.Fields{
		"path": path,
	}
	if info, err := os.Stat(path); err == nil {
		if !info.IsDir() {
			Logc(ctx).WithFields(fields).Error("Path exists but is not a directory")
			return fmt.Errorf("path exists but is not a directory: %s", path)
		}
		return nil
	} else if !os.IsNotExist(err) {
		Logc(ctx).WithFields(fields).Errorf("Can't determine if directory exists; %s", err)
		return fmt.Errorf("can't determine if directory %s exists; %s", path, err)
	}

	err := os.MkdirAll(path, 0o755)
	if err != nil {
		Logc(ctx).WithFields(fields).Errorf("Mkdir failed; %s", err)
		return fmt.Errorf("failed to mkdir %s; %s", path, err)
	}

	return nil
}
