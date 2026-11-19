// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package nvme provides an adapter for NVMe over Fabrics host operations: tooling and transport
// availability checks, host identity, subsystem state, connect, disconnect and flush. Platform-specific
// mechanisms are implemented in _<GOOS>.go files behind the Adapter interface. The adapter reports facts
// and applies no policy.
package nvme

import (
	"context"

	"github.com/spf13/afero"

	tridentexec "github.com/netapp/trident/utils/exec"
)

// Adapter defines the NVMe host API.
type Adapter interface {
	// Version returns the version string of the host nvme-cli tooling.
	// Use as an availability check. If the tool is not installed, this fails.
	//
	// Errors:
	//   - *Error on any nvme non-zero exit.
	//   - wrapped os/exec error if nvme could not be executed (e.g. not installed, not on PATH).
	Version(ctx context.Context) (string, error)

	// HostNQN returns the NVMe qualified name of this host: the first line of /etc/nvme/hostnqn.
	//
	// Errors:
	//   - wrapped exec error if the file cannot be read (e.g. nvme-cli is not installed).
	HostNQN(ctx context.Context) (string, error)

	// TransportLoaded reports whether the kernel provides the NVMe-oF transport on the host.
	// The transport is available when /sys/module/nvme_<transport> is a directory, which covers a loaded
	// module and a transport built into the kernel, or when lsmod lists the nvme_<transport> module.
	// A false result with a nil error means the transport was not found.
	//
	// Errors:
	//   - an error if transport is not a valid transport name.
	//   - wrapped exec error if lsmod fails and sysfs did not already show the transport.
	TransportLoaded(ctx context.Context, transport Transport) (bool, error)

	// Subsystem returns the subsystem with the given NQN as the host sees it in
	// /sys/class/nvme-subsystem: its paths (controllers) and the namespace head devices on this host.
	// A namespace whose UUID cannot be read is reported with an empty UUID. A subsystem directory whose
	// subsysnqn cannot be read is skipped. A path attribute that cannot be read is reported empty.
	//
	// Errors:
	//   - *SubsystemNotFoundError if the subsystem has no paths on this host, including when
	//     /sys/class/nvme-subsystem does not exist.
	//   - wrapped filesystem error if a sysfs directory cannot be listed.
	Subsystem(ctx context.Context, nqn string) (*Subsystem, error)

	// Connect creates one path from this host to the subsystem at addr. By default it connects over
	// NVMe/TCP to port 4420 with the controller loss timeout disabled (-l -1), so the path is never dropped
	// while the address is down:
	//
	//	nvme connect -t tcp -n <nqn> -a <addr> -s 4420 -l -1
	//
	// addr is the target transport address, an IP address for tcp.
	// Options select another transport (WithTransport) or port (WithPort).
	// The command is killed when ctx is done. With no deadline on ctx it is not bounded.
	//
	// Errors:
	//   - an error if an argument is empty or malformed.
	//   - *Error on any nvme non-zero exit.
	//   - a timeout error (utils/errors.IsTimeoutError) if ctx ended before the command did.
	//   - wrapped os/exec error if nvme could not be executed.
	Connect(ctx context.Context, nqn, addr string, opts ...ConnectOption) error

	// Disconnect removes every path to the subsystem from this host, which deletes its namespaces here:
	//
	//	nvme disconnect -n <nqn>
	//
	// The command is killed when ctx is done. With no deadline on ctx it is not bounded.
	//
	// Errors:
	//   - *Error on any nvme non-zero exit.
	//   - wrapped os/exec error if nvme could not be executed.
	Disconnect(ctx context.Context, nqn string) error

	// DisconnectPath removes one path (controller) from this host and leaves the subsystem's other paths in
	// place. controller is the controller name, for example nvme0, as reported by Path.Name:
	//
	//	nvme disconnect -d <controller>
	//
	// The command is killed when ctx is done or after 10 seconds, whichever comes first.
	//
	// Errors:
	//   - an error if controller is empty, starts with '-' or contains '/'.
	//   - *Error on any nvme non-zero exit.
	//   - a timeout error (utils/errors.IsTimeoutError) if the command was killed before it ended.
	//   - wrapped os/exec error if nvme could not be executed.
	DisconnectPath(ctx context.Context, controller string) error

	// Flush asks the controller to commit the device's volatile write cache to media:
	//
	//	nvme flush <devicePath>
	//
	// The command is killed when ctx is done. With no deadline on ctx it is not bounded.
	//
	// Errors:
	//   - *Error on any nvme non-zero exit.
	//   - wrapped os/exec error if nvme could not be executed.
	Flush(ctx context.Context, devicePath string) error
}

// adapter is a concrete implementation of the Adapter interface.
type adapter struct {
	cmd  tridentexec.Command
	chrt string
	fs   afero.Fs
}

type Option func(*adapter)

func WithCommand(cmd tridentexec.Command) Option {
	return func(a *adapter) { a.cmd = cmd }
}
func WithFs(fs afero.Fs) Option                 { return func(a *adapter) { a.fs = fs } }
func WithChrootPrefix(rootPrefix string) Option { return func(a *adapter) { a.chrt = rootPrefix } }

func New(opts ...Option) Adapter {
	a := &adapter{
		cmd:  tridentexec.NewCommand(),
		chrt: "",
		fs:   afero.NewOsFs(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}
