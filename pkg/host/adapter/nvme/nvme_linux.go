// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/afero"

	. "github.com/netapp/trident/logging"
	tridentErrors "github.com/netapp/trident/utils/errors"
	tridentexec "github.com/netapp/trident/utils/exec"
)

var (
	// namespaceRegex matches namespace head devices, for example nvme0n1. It skips the hidden
	// per-controller devices (nvme0c0n1), the generic character devices (ng0n1) and controllers (nvme0).
	namespaceRegex = regexp.MustCompile(`^nvme([0-9]+)n([0-9]+)$`)
	// controllerRegex matches controller (path) directories, for example nvme0.
	controllerRegex = regexp.MustCompile(`^nvme([0-9]+)$`)
)

/** nvme constants */
const (
	nvmeCLI = "nvme"
	lsmod   = "lsmod"
	cat     = "cat"

	// moduleNamePrefix precedes the transport name in the kernel module that provides it, e.g. nvme_tcp.
	moduleNamePrefix = "nvme_"

	// ctrlLossTimeoutForever (-l -1) makes a path undroppable even if its address is down indefinitely.
	ctrlLossTimeoutForever = "-1"

	// noTimeout is the bound for a command whose caller set no deadline: effectively none.
	noTimeout = time.Duration(math.MaxInt64)

	// execTimeout is the Go process-kill deadline for the short, read-only commands in this file.
	execTimeout = 10 * time.Second

	// disconnectPathTimeout bounds the removal of one path, which runs on a periodic sweep where a command
	// that never returns would stall every later subsystem.
	disconnectPathTimeout = 10 * time.Second
)

/** sysfs and host file constants */
const (
	hostNQNPath       = "/etc/nvme/hostnqn"
	sysfsModuleDir    = "/sys/module"
	sysfsSubsystemDir = "/sys/class/nvme-subsystem"

	sysfsSubsysNQNFileName = "subsysnqn"
	sysfsStateFileName     = "state"
	sysfsAddressFileName   = "address"
	sysfsTransportFileName = "transport"
	sysfsNamespaceUUIDFile = "uuid"
)

// commandTimeout returns the bound to give ExecuteWithTimeout for a command that the caller's context governs:
// the time left until ctx's deadline, or effectively no bound when ctx has none. ExecuteWithTimeout also stops
// the command when ctx is canceled. Execute would not: it ignores ctx.
func commandTimeout(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		return time.Until(deadline)
	}
	return noTimeout
}

func parseExecError(cmd string, out []byte, err error) error {
	// A timeout is passed through unwrapped so callers can classify it with errors.IsTimeoutError.
	if tridentErrors.IsTimeoutError(err) {
		return err
	}

	var exitErr tridentexec.ExitError
	if errors.As(err, &exitErr) {
		return &Error{bin: nvmeCLI, cmd: cmd, code: exitErr.ExitCode(), out: string(out)}
	}

	return fmt.Errorf("%s %s failed: %w", nvmeCLI, cmd, err)
}

// Version returns the nvme-cli version string.
//
// Errors:
//   - *Error on any nvme non-zero exit (no exit codes are classified here).
//   - wrapped os/exec error if nvme could not be executed
//     (e.g. not installed, not on PATH).
func (a *adapter) Version(ctx context.Context) (string, error) {
	Logc(ctx).Trace(">>>> nvme_linux.Version")
	defer Logc(ctx).Trace("<<<< nvme_linux.Version")

	out, err := a.cmd.ExecuteWithTimeout(ctx, nvmeCLI, execTimeout, false, "version")
	if err != nil {
		return "", parseExecError("version", out, err)
	}

	return strings.TrimSpace(string(out)), nil
}

// HostNQN returns the first line of /etc/nvme/hostnqn.
//
// The file is read with cat. In a Trident container cat is a chwrap binary that runs
// in the host root, so this reads the host's file. Reading the container's own /etc through afero
// would return a different (or no) hostnqn.
//
// Errors:
//   - wrapped exec error if cat fails (e.g. the file does not exist).
func (a *adapter) HostNQN(ctx context.Context) (string, error) {
	Logc(ctx).Trace(">>>> nvme_linux.HostNQN")
	defer Logc(ctx).Trace("<<<< nvme_linux.HostNQN")

	out, err := a.cmd.ExecuteWithTimeout(ctx, cat, execTimeout, false, hostNQNPath)
	if err != nil {
		return "", err
	}

	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line), nil
}

// TransportLoaded reports whether the NVMe-oF transport is available.
//
// A stat failure on the sysfs directory is not fatal; lsmod still runs.
//
// Errors:
//   - an error if transport is not a valid transport name.
//   - wrapped exec error if lsmod fails and sysfs did not already show the transport.
func (a *adapter) TransportLoaded(ctx context.Context, transport Transport) (bool, error) {
	fields := LogFields{"transport": transport}
	Logc(ctx).WithFields(fields).Trace(">>>> nvme_linux.TransportLoaded")
	defer Logc(ctx).WithFields(fields).Trace("<<<< nvme_linux.TransportLoaded")

	if err := transport.validate(); err != nil {
		return false, err
	}
	module := moduleNamePrefix + string(transport)

	// /sys/module/nvme_<transport> exists for a loaded module and for a transport built into the kernel.
	switch present, err := a.moduleDirPresent(module); {
	case err != nil:
		Logc(ctx).WithFields(fields).WithError(err).Warn("Could not inspect the NVMe transport kernel module in sysfs.")
	case present:
		return true, nil
	}

	out, err := a.cmd.ExecuteWithTimeout(ctx, lsmod, execTimeout, false)
	if err != nil {
		return false, fmt.Errorf("failed to get NVMe driver info; %w", err)
	}

	// Match the module column exactly. A substring match would take nvme_fcloop for nvme_fc.
	for _, line := range strings.Split(string(out), "\n") {
		if fields := strings.Fields(line); len(fields) > 0 && fields[0] == module {
			return true, nil
		}
	}

	return false, nil
}

// moduleDirPresent reports whether the module's sysfs directory exists.
// A missing path is false with a nil error. Non-directories are treated as absent.
func (a *adapter) moduleDirPresent(module string) (bool, error) {
	info, err := a.fs.Stat(filepath.Join(a.chrt, sysfsModuleDir, module))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return info.IsDir(), nil
}

// Subsystem returns the subsystem with the given NQN as read from /sys/class/nvme-subsystem.
//
// Errors:
//   - *SubsystemNotFoundError if the subsystem has no paths on this host.
//   - wrapped filesystem error if a sysfs directory cannot be listed.
func (a *adapter) Subsystem(ctx context.Context, nqn string) (*Subsystem, error) {
	fields := LogFields{"nqn": nqn}
	Logc(ctx).WithFields(fields).Trace(">>>> nvme_linux.Subsystem")
	defer Logc(ctx).WithFields(fields).Trace("<<<< nvme_linux.Subsystem")

	if nqn == "" {
		return nil, errors.New("subsystem NQN is empty")
	}

	entries, err := afero.ReadDir(a.fs, filepath.Join(a.chrt, sysfsSubsystemDir))
	if os.IsNotExist(err) {
		Logc(ctx).WithFields(fields).Debug("NVMe subsystem directory does not exist.")
		return nil, &SubsystemNotFoundError{nqn: nqn}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list NVMe subsystems; %w", err)
	}

	for _, entry := range entries {
		dir := filepath.Join(sysfsSubsystemDir, entry.Name())

		// Example subsysnqn: nqn.1992-08.com.netapp:sn.6628417f7bec11ef9bf2005056b3e634:subsystem.scspa3014048001-...
		subsysNQN, err := a.readAttr(filepath.Join(a.chrt, dir, sysfsSubsysNQNFileName))
		if err != nil {
			Logc(ctx).WithFields(fields).WithField("dir", dir).WithError(err).Warn(
				"Could not read NVMe subsystem NQN; skipping subsystem.")
			continue
		}
		if subsysNQN != nqn {
			continue
		}

		sub, err := a.readSubsystem(ctx, nqn, dir)
		if err != nil {
			return nil, err
		}
		if len(sub.paths) == 0 {
			return nil, &SubsystemNotFoundError{nqn: nqn}
		}

		Logc(ctx).WithFields(fields).WithFields(LogFields{
			"dir":        sub.dir,
			"paths":      len(sub.paths),
			"namespaces": len(sub.namespaces),
		}).Debug("Gathered NVMe subsystem information.")

		return sub, nil
	}

	return nil, &SubsystemNotFoundError{nqn: nqn}
}

// readSubsystem reads the paths and namespaces under a subsystem's sysfs directory.
func (a *adapter) readSubsystem(ctx context.Context, nqn, dir string) (*Subsystem, error) {
	entries, err := afero.ReadDir(a.fs, filepath.Join(a.chrt, dir))
	if err != nil {
		return nil, fmt.Errorf("failed to read NVMe subsystem directory %s; %w", dir, err)
	}

	sub := &Subsystem{nqn: nqn, dir: dir}
	for _, entry := range entries {
		name := entry.Name()
		entryDir := filepath.Join(a.chrt, dir, name)

		switch {
		case controllerRegex.MatchString(name):
			sub.paths = append(sub.paths, a.readPath(ctx, name, entryDir))
		case namespaceRegex.MatchString(name):
			sub.namespaces = append(sub.namespaces, a.readNamespace(ctx, name, entryDir))
		}
	}

	return sub, nil
}

// readPath reads a controller's state, address and transport. An attribute that cannot be read is
// left empty, so a controller that is being torn down reports a state that is not live.
func (a *adapter) readPath(ctx context.Context, name, ctrlDir string) *Path {
	state := a.readAttrOrEmpty(ctx, filepath.Join(ctrlDir, sysfsStateFileName))
	address := a.readAttrOrEmpty(ctx, filepath.Join(ctrlDir, sysfsAddressFileName))
	transport := a.readAttrOrEmpty(ctx, filepath.Join(ctrlDir, sysfsTransportFileName))

	traddr, port := parseAddress(address)
	if traddr == "" {
		Logc(ctx).WithFields(LogFields{"controller": name, "address": address}).Debug(
			"Could not find the target address of the NVMe controller.")
	}

	return &Path{name: name, address: traddr, port: port, transport: Transport(transport), state: PathState(state)}
}

// readNamespace reads a namespace head device's UUID. An unreadable UUID is reported as empty.
func (a *adapter) readNamespace(ctx context.Context, name, nsDir string) *Namespace {
	return &Namespace{name: name, uuid: a.readAttrOrEmpty(ctx, filepath.Join(nsDir, sysfsNamespaceUUIDFile))}
}

// readAttr reads a sysfs attribute and trims surrounding whitespace.
func (a *adapter) readAttr(path string) (string, error) {
	b, err := afero.ReadFile(a.fs, path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// readAttrOrEmpty reads a sysfs attribute, returning "" and logging if it cannot be read.
func (a *adapter) readAttrOrEmpty(ctx context.Context, path string) string {
	value, err := a.readAttr(path)
	if err != nil {
		Logc(ctx).WithField("path", path).WithError(err).Debug("Could not read NVMe sysfs attribute.")
		return ""
	}
	return value
}

// Connect creates a path to the subsystem at addr. The command is killed when ctx is done.
//
// Errors:
//   - an error if an argument is empty or malformed.
//   - *Error on any nvme non-zero exit.
//   - a timeout error (utils/errors.IsTimeoutError) if ctx ended before the command did.
//   - wrapped os/exec error if nvme could not be executed.
func (a *adapter) Connect(ctx context.Context, nqn, addr string, opts ...ConnectOption) error {
	cfg := newConnectConfig(opts)
	fields := LogFields{"nqn": nqn, "addr": addr, "transport": cfg.transport}
	Logc(ctx).WithFields(fields).Trace(">>>> nvme_linux.Connect")
	defer Logc(ctx).WithFields(fields).Trace("<<<< nvme_linux.Connect")

	args, err := connectArgs(nqn, addr, cfg)
	if err != nil {
		return err
	}

	if out, err := a.cmd.ExecuteWithTimeout(ctx, nvmeCLI, commandTimeout(ctx), true, args...); err != nil {
		return parseExecError("connect", out, err)
	}

	return nil
}

// connectArgs validates the connect request and builds the nvme arguments. With the default options it
// is: connect -t tcp -n <nqn> -a <addr> -s 4420 -l -1
func connectArgs(nqn, addr string, cfg connectConfig) ([]string, error) {
	if nqn == "" {
		return nil, errors.New("subsystem NQN is empty")
	}
	if addr == "" {
		return nil, errors.New("target address is empty")
	}
	if strings.HasPrefix(addr, "-") {
		return nil, errors.New("target address must not start with '-'")
	}
	if err := cfg.transport.validate(); err != nil {
		return nil, err
	}
	port := cfg.port
	switch {
	case port != "" && !cfg.transport.usesPort():
		return nil, fmt.Errorf("transport %s does not use a port", cfg.transport)
	case port != "":
		if _, err := strconv.ParseUint(port, 10, 16); err != nil {
			return nil, fmt.Errorf("invalid NVMe port %q", port)
		}
	case cfg.transport.usesPort():
		port = defaultPort
	}

	args := []string{"connect", "-t", string(cfg.transport), "-n", nqn, "-a", addr}
	if port != "" {
		args = append(args, "-s", port)
	}
	return append(args, "-l", ctrlLossTimeoutForever), nil
}

// Disconnect removes every path to the subsystem. The command is killed when ctx is done.
//
// Errors:
//   - an error if nqn is empty.
//   - *Error on any nvme non-zero exit.
//   - a timeout error (utils/errors.IsTimeoutError) if ctx ended before the command did.
//   - wrapped os/exec error if nvme could not be executed.
func (a *adapter) Disconnect(ctx context.Context, nqn string) error {
	fields := LogFields{"nqn": nqn}
	Logc(ctx).WithFields(fields).Trace(">>>> nvme_linux.Disconnect")
	defer Logc(ctx).WithFields(fields).Trace("<<<< nvme_linux.Disconnect")

	if nqn == "" {
		return errors.New("subsystem NQN is empty")
	}

	if out, err := a.cmd.ExecuteWithTimeout(ctx, nvmeCLI, commandTimeout(ctx), true,
		"disconnect", "-n", nqn); err != nil {
		return parseExecError("disconnect", out, err)
	}

	return nil
}

// DisconnectPath removes one path of a subsystem. The command is killed when ctx is done or after
// disconnectPathTimeout.
//
// Errors:
//   - an error if controller is empty, starts with '-' or contains '/'.
//   - *Error on any nvme non-zero exit.
//   - a timeout error (utils/errors.IsTimeoutError) if the command was killed before it ended.
//   - wrapped os/exec error if nvme could not be executed.
func (a *adapter) DisconnectPath(ctx context.Context, controller string) error {
	fields := LogFields{"controller": controller}
	Logc(ctx).WithFields(fields).Trace(">>>> nvme_linux.DisconnectPath")
	defer Logc(ctx).WithFields(fields).Trace("<<<< nvme_linux.DisconnectPath")

	if controller == "" {
		return errors.New("controller name is empty")
	}
	if strings.HasPrefix(controller, "-") || strings.Contains(controller, "/") {
		return fmt.Errorf("invalid controller name %q", controller)
	}

	if out, err := a.cmd.ExecuteWithTimeout(ctx, nvmeCLI, min(disconnectPathTimeout, commandTimeout(ctx)), true,
		"disconnect", "-d", controller); err != nil {
		return parseExecError("disconnect", out, err)
	}

	return nil
}

// Flush commits the device's volatile write cache to media. The command is killed when ctx is done.
//
// Errors:
//   - an error if devicePath is empty or starts with '-'.
//   - *Error on any nvme non-zero exit.
//   - a timeout error (utils/errors.IsTimeoutError) if ctx ended before the command did.
//   - wrapped os/exec error if nvme could not be executed.
func (a *adapter) Flush(ctx context.Context, devicePath string) error {
	fields := LogFields{"devicePath": devicePath}
	Logc(ctx).WithFields(fields).Trace(">>>> nvme_linux.Flush")
	defer Logc(ctx).WithFields(fields).Trace("<<<< nvme_linux.Flush")

	if devicePath == "" {
		return errors.New("device path is empty")
	}
	if strings.HasPrefix(devicePath, "-") {
		return fmt.Errorf("invalid device path %q", devicePath)
	}

	if out, err := a.cmd.ExecuteWithTimeout(ctx, nvmeCLI, commandTimeout(ctx), true,
		"flush", devicePath); err != nil {
		return parseExecError("flush", out, err)
	}

	return nil
}
