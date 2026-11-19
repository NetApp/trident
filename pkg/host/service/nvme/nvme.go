// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package nvme provides the NVMe over Fabrics service layer.
// It owns the lifecycle of NVMe connections, subsystems and namespaces on the host: connecting subsystem
// paths, waiting for the namespace device to appear, flushing it, waiting for it to grow, and disconnecting
// a subsystem that no longer serves any namespace. It composes the nvme adapter (nvme-cli and sysfs) with the
// block adapter (the namespace's block device).
//
// Subsystems are shared: one NQN serves many namespaces, and disconnecting it tears down all of them.
// The service takes no locks. Every method re-reads the host and is safe to repeat: when a concurrent
// operation changes the host underneath it, a method returns an error or retries, and the caller can call it again.
//
// Everything above the namespace device (LUKS, filesystem, mounts) belongs to other services.
package nvme

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/netapp/trident/internal/fiji"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/host/adapter/block"
	"github.com/netapp/trident/pkg/host/adapter/nvme"
	"github.com/netapp/trident/utils/errors"
)

const (
	// ensureConnectionsTimeout bounds how long an attach waits for the subsystem to connect.
	ensureConnectionsInterval = 1 * time.Second
	ensureConnectionsTimeout  = 20 * time.Second
	ensureDeviceInterval      = 500 * time.Millisecond
	ensureDeviceTimeout       = 20 * time.Second
	flushTimeout              = 30 * time.Second

	// The size wait matches the multipath size wait in service/mpath.
	ensureSizeInterval    = 1 * time.Second
	ensureSizeTimeout     = 60 * time.Second
	ensureSizeStableReads = 3
)

// Fault injection points. They are typed as the interface so that tests can replace them in both fiji and
// non-fiji builds.
var (
	beforeNVMeDisconnect  = fiji.Register("beforeNVMeDisconnect", "nvme_linux")
	beforeNVMeFlushDevice = fiji.Register("beforeNVMeFlushDevice", "nvme")
)

// Subsystem and Path are the adapter's types, re-exported for callers of Service.
type (
	Subsystem = nvme.Subsystem
	Path      = nvme.Path
)

// Service defines the NVMe over Fabrics device lifecycle API.
// The driver calls these methods to attach, find, flush, grow and detach an NVMe namespace.
type Service interface {
	// PreChecks verifies that the host has the NVMe tooling (nvme-cli) and the NVMe/TCP transport.
	// Call it before any other operation to fail fast with a clear error.
	PreChecks(ctx context.Context) error

	// HostNQN returns the NVMe qualified name of this host, for node registration.
	HostNQN(ctx context.Context) (string, error)

	// EnsureConnections ensures that the host has a path to the subsystem at the given target IPs.
	// It is idempotent: existing paths are preserved; only missing paths are connected.
	//
	// It tolerates partial failure: it succeeds as long as at least one connect worked or at least one
	// target IP already has a path, so one down LIF does not fail an attach. Otherwise it retries the
	// connects until the context ends. A context without a deadline is bounded to 20 seconds, and each
	// nvme command is killed when the context ends.
	EnsureConnections(ctx context.Context, nqn string, targetIPs []string) error

	// EnsureDevice polls until the namespace is visible on the subsystem and returns its device.
	// It connects nothing: call EnsureConnections first. A context without a deadline is bounded to
	// 20 seconds.
	//
	// On timeout it returns the last attempt's error, which is a NotFoundError when the namespace never
	// appeared. A concurrent EnsureDisconnect can remove the subsystem while this waits; the caller then
	// repeats EnsureConnections and EnsureDevice.
	EnsureDevice(ctx context.Context, nqn, nsUUID string) (*Device, error)

	// DeviceByUUID returns the device for the namespace UUID on the subsystem, without waiting.
	// The UUID is compared without regard to case.
	// If the subsystem or the namespace is not on the host, it fails with NotFoundError.
	DeviceByUUID(ctx context.Context, nqn, nsUUID string) (*Device, error)

	// EnsureFlush flushes the device's write cache, but only while the subsystem has a live path.
	// A nil device or one that is already gone is not an error.
	//
	// With opts.Force nothing is flushed and it returns nil: a forced detach does not wait for the host.
	// Otherwise a flush that cannot run or fails returns a *FlushError, so the caller's retry window still
	// applies, unless opts.IgnoreErrors is set: then it is logged and nil is returned. A flush that cannot
	// run is one on a subsystem with no live path, which would wait for a path to come back.
	// The flush is killed when the context ends. A context without a deadline is bounded to 30 seconds.
	// A fired fault point (beforeNVMeFlushDevice) is returned before anything else.
	EnsureFlush(ctx context.Context, dev *Device, opts FlushOptions) error

	// EnsureSize polls the device until it is at or above targetBytes for several consecutive reads.
	// A context without a deadline is bounded to 60 seconds.
	EnsureSize(ctx context.Context, dev *Device, targetBytes uint64) error

	// EnsureDisconnect disconnects the subsystem when the namespace is the last one it serves on
	// this host. A subsystem with other namespaces is left alone, and so is one the host cannot enumerate.
	// A subsystem that is already gone is not an error.
	//
	// When no path is live the namespaces cannot be enumerated. By default the subsystem is then left
	// alone, because disconnecting could tear down other volumes. With force, it is disconnected anyway,
	// for a caller that knows from its own state that no other volume uses the subsystem.
	// Force does not override namespaces that are visible: those always keep the subsystem.
	//
	// A namespace that becomes visible after the check is not protected. The attach that owns it fails
	// or retries, and the caller repeats it.
	EnsureDisconnect(ctx context.Context, nqn, nsUUID string, force bool) error

	// HasLivePath reports whether the subsystem has at least one path that can carry I/O.
	HasLivePath(ctx context.Context, nqn string) bool

	// Subsystem returns a snapshot of the subsystem as the host sees it, without waiting.
	//
	// Errors:
	//   - NotFoundError if the subsystem is not on the host or has no paths.
	//   - a wrapped adapter error if the host cannot be read.
	Subsystem(ctx context.Context, nqn string) (*Subsystem, error)

	// ConnectPaths makes one attempt to connect the target IPs that have no path at all to the subsystem.
	// A path in any state counts as present, even one that is not live, because the kernel keeps trying to
	// recover it. It does not wait for a path to go live and does not retry: a caller on a timer repeats it.
	// It connects every missing IP, and returns the failures joined, or nil if nothing was missing or
	// every connect worked. Each nvme command is killed when the context ends.
	ConnectPaths(ctx context.Context, nqn string, targetIPs []string) error

	// PrunePaths removes the subsystem's paths to data LIFs that are absent from desiredIPs.
	//
	// Paths are connected without a controller loss timeout, so when a data LIF goes away the kernel keeps
	// its path in a non-live state indefinitely, and that dead path still counts against the subsystem's
	// path limit. Removing it frees the slot for a replacement LIF.
	//
	// Removing a path destroys host state, so a path is kept unless all of these hold:
	//   - its address is known and not in desiredIPs. A LIF that is only unreachable is still desired.
	//   - another path remains. Emptying a subsystem renames the devices of mounted volumes.
	//   - it is not the only live path, so a degraded subsystem that still serves I/O is not stopped.
	//
	// Removal is best effort: a path that cannot be disconnected is logged and left for the next call.
	// It returns NotFoundError if the subsystem is not on the host or has no paths, and a wrapped adapter
	// error if the host cannot be read.
	PrunePaths(ctx context.Context, nqn string, desiredIPs []string) error
}

type service struct {
	nvme  nvme.Adapter
	block block.Adapter

	// Poll intervals of the context loops. They exist so tests can run the loops quickly.
	connectInterval      time.Duration
	ensureDeviceInterval time.Duration
	ensureSizeInterval   time.Duration
}

type Option func(*service)

func WithNVMeAdapter(a nvme.Adapter) Option   { return func(s *service) { s.nvme = a } }
func WithBlockAdapter(b block.Adapter) Option { return func(s *service) { s.block = b } }

func New(opts ...Option) Service {
	s := &service{
		nvme:                 nvme.New(),
		block:                block.New(),
		connectInterval:      ensureConnectionsInterval,
		ensureDeviceInterval: ensureDeviceInterval,
		ensureSizeInterval:   ensureSizeInterval,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// PreChecks verifies nvme-cli is available and the NVMe/TCP transport is in the kernel.
func (s *service) PreChecks(ctx context.Context) error {
	Logc(ctx).Debug(">>>> nvme.PreChecks")
	defer Logc(ctx).Debug("<<<< nvme.PreChecks")

	if _, err := s.nvme.Version(ctx); err != nil {
		Logc(ctx).WithError(err).Debug("Could not read NVMe CLI version; perhaps NVMe CLI is not installed?")
		return fmt.Errorf("nvme-cli version: %w", err)
	}

	loaded, err := s.nvme.TransportLoaded(ctx, nvme.TransportTCP)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Could not read the modules loaded on the host.")
		return fmt.Errorf("NVMe driver info: %w", err)
	}
	if !loaded {
		return ErrTransportNotLoaded
	}

	return nil
}

// HostNQN returns the host NQN from the host's nvme-cli configuration.
func (s *service) HostNQN(ctx context.Context) (string, error) {
	Logc(ctx).Debug(">>>> nvme.HostNQN")
	defer Logc(ctx).Debug("<<<< nvme.HostNQN")

	nqn, err := s.nvme.HostNQN(ctx)
	if err != nil {
		Logc(ctx).WithError(err).Warn("Could not read hostnqn; perhaps NVMe is not installed?")
		return "", fmt.Errorf("failed to get hostnqn; %w", err)
	}
	return nqn, nil
}

func (s *service) EnsureConnections(ctx context.Context, nqn string, targetIPs []string) error {
	fields := LogFields{"subsystem": nqn, "targetIPs": targetIPs}
	Logc(ctx).WithFields(fields).Debug(">>>> nvme.EnsureConnections")
	defer Logc(ctx).WithFields(fields).Debug("<<<< nvme.EnsureConnections")

	if nqn == "" {
		return fmt.Errorf("empty subsystem NQN")
	}
	if len(targetIPs) == 0 {
		return fmt.Errorf("no target IPs provided")
	}

	return s.waitForConnections(ctx, nqn, targetIPs)
}

// waitForConnections drives the host toward having a path to the subsystem at the target IPs.
//
// It follows the same observe → decide → mutate pattern as the iSCSI service:
//  1. Observe:  Read the subsystem from sysfs.
//  2. Decide:   Find target IPs that have no live path.
//  3. Mutate:   Connect those paths via nvme-cli.
//
// It retries until one connect works or a target IP has a path, or until the context ends.
func (s *service) waitForConnections(ctx context.Context, nqn string, targetIPs []string) error {
	fields := LogFields{"subsystem": nqn, "targetIPs": targetIPs}
	Logc(ctx).WithFields(fields).Trace(">>>> nvme.waitForConnections")
	defer Logc(ctx).WithFields(fields).Trace("<<<< nvme.waitForConnections")

	// Always enforce a maximum bound. context.WithTimeout takes the minimum of the parent deadline and
	// the given duration, so a CSI context with a tighter deadline is not widened.
	ctx, cancel := context.WithTimeout(ctx, ensureConnectionsTimeout)
	defer cancel()

	// Always try 1x to initialize the process.
	// The context loop below avoids returning transient errors to callers.
	err := s.ensureConnection(ctx, nqn, targetIPs)
	if err == nil {
		return nil
	}
	Logc(ctx).WithFields(fields).WithError(err).Debug("NVMe paths not yet live; retrying.")

	ticker := time.NewTicker(s.connectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out connecting NVMe subsystem %s: %w", nqn, err)
		case <-ticker.C:
			if err = s.ensureConnection(ctx, nqn, targetIPs); err == nil {
				return nil
			}
			Logc(ctx).WithFields(fields).WithError(err).Debug("NVMe paths not yet live; retrying.")
		}
	}
}

// ensureConnection makes one attempt to connect the target IPs that have no path.
// It returns nil when at least one connect worked or at least one target IP has a path in any state.
func (s *service) ensureConnection(ctx context.Context, nqn string, targetIPs []string) error {
	sub, err := s.readSubsystem(ctx, nqn)
	if err != nil {
		// A subsystem that cannot be read is treated as having no paths, so the connect below still runs.
		Logc(ctx).WithError(err).Warn("Failed to read NVMe subsystem; treating as having no paths.")
		sub = nil
	}

	// A path in any state, even "connecting", is left to the kernel to recover.
	missing := missingPaths(sub, targetIPs)
	if len(missing) == 0 {
		return nil
	}
	Logc(ctx).WithField("targetIPs", missing).Debug("NVMe subsystem is missing desired target paths; connecting.")

	connected, connectErr := s.connect(ctx, nqn, missing)
	switch {
	case connected:
		return nil
	case anyPathPresent(sub, targetIPs):
		// A LIF is already present (possibly in connecting state); the existing path may become live.
		Logc(ctx).WithError(connectErr).Debug("Some paths could not be connected; others are present.")
		return nil
	default:
		return connectErr
	}
}

func (s *service) EnsureDevice(ctx context.Context, nqn, nsUUID string) (*Device, error) {
	fields := LogFields{"subsystem": nqn, "namespace": nsUUID}
	Logc(ctx).WithFields(fields).Debug(">>>> nvme.EnsureDevice")
	defer Logc(ctx).WithFields(fields).Debug("<<<< nvme.EnsureDevice")

	if nqn == "" {
		return nil, fmt.Errorf("empty subsystem NQN")
	}
	if nsUUID == "" {
		return nil, fmt.Errorf("empty namespace UUID")
	}

	return s.waitForDevice(ctx, nqn, nsUUID)
}

// waitForDevice polls the host until the namespace's device is visible or the context ends.
func (s *service) waitForDevice(ctx context.Context, nqn, nsUUID string) (*Device, error) {
	fields := LogFields{"subsystem": nqn, "namespace": nsUUID}
	Logc(ctx).WithFields(fields).Trace(">>>> nvme.waitForDevice")
	defer Logc(ctx).WithFields(fields).Trace("<<<< nvme.waitForDevice")

	// Always enforce a maximum bound. context.WithTimeout takes the minimum of the parent deadline and
	// the given duration, so a CSI context with a tighter deadline is not widened.
	ctx, cancel := context.WithTimeout(ctx, ensureDeviceTimeout)
	defer cancel()

	// Always try 1x to initialize the process.
	// The context loop below avoids returning transient errors to callers.
	dev, err := s.deviceByUUID(ctx, nqn, nsUUID)
	if err == nil {
		return dev, nil
	}
	Logc(ctx).WithFields(fields).WithError(err).Debug("NVMe namespace is not yet visible; retrying.")

	ticker := time.NewTicker(s.ensureDeviceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out waiting for NVMe namespace %s on subsystem %s: %w",
				nsUUID, nqn, err)
		case <-ticker.C:
			if dev, err = s.deviceByUUID(ctx, nqn, nsUUID); err == nil {
				return dev, nil
			}
			Logc(ctx).WithFields(fields).WithError(err).Debug("NVMe namespace is not yet visible; retrying.")
		}
	}
}

// connect connects each target IP in parallel and reports whether any connect worked.
// It always tries every IP. Each goroutine is bounded by the caller's context, so a
// blackholed IP cannot starve connects to healthy IPs.
func (s *service) connect(ctx context.Context, nqn string, ips []string) (bool, error) {
	type result struct {
		ip  string
		err error
	}
	ch := make(chan result, len(ips))

	var wg sync.WaitGroup
	for _, ip := range ips {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			ch <- result{ip: ip, err: s.nvme.Connect(ctx, nqn, ip)}
		}(ip)
	}
	wg.Wait()
	close(ch)

	var (
		connected bool
		errs      error
	)
	for r := range ch {
		if r.err != nil {
			Logc(ctx).WithError(r.err).Errorf("Failed to connect subsystem %s to %s.", nqn, r.ip)
			errs = errors.Join(errs, fmt.Errorf("failed to connect subsystem %s to %s; %w", nqn, r.ip, r.err))
			continue
		}
		connected = true
	}
	return connected, errs
}

// DeviceByUUID finds the namespace device on the subsystem.
func (s *service) DeviceByUUID(ctx context.Context, nqn, nsUUID string) (*Device, error) {
	fields := LogFields{"subsystem": nqn, "namespace": nsUUID}
	Logc(ctx).WithFields(fields).Debug(">>>> nvme.DeviceByUUID")
	defer Logc(ctx).WithFields(fields).Debug("<<<< nvme.DeviceByUUID")

	dev, err := s.deviceByUUID(ctx, nqn, nsUUID)
	if err != nil {
		return nil, err
	}
	return dev, nil
}

// deviceByUUID reads the subsystem and finds the namespace's device in it.
func (s *service) deviceByUUID(ctx context.Context, nqn, nsUUID string) (*Device, error) {
	sub, err := s.readSubsystem(ctx, nqn)
	if err != nil {
		return nil, err
	}
	return s.deviceFromSubsystem(ctx, sub, nqn, nsUUID)
}

// readSubsystem reads the subsystem from the host. A subsystem with no paths is nil and not an error.
func (s *service) readSubsystem(ctx context.Context, nqn string) (*nvme.Subsystem, error) {
	sub, err := s.nvme.Subsystem(ctx, nqn)
	if err != nil {
		if nvme.IsSubsystemNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	return sub, nil
}

// deviceFromSubsystem finds the namespace by UUID in the subsystem and reads its block device.
// A nil subsystem has no namespaces.
func (s *service) deviceFromSubsystem(
	ctx context.Context, sub *nvme.Subsystem, nqn, nsUUID string,
) (*Device, error) {
	if sub == nil {
		return nil, errors.NotFoundError("NVMe subsystem %s not found", nqn)
	}

	for _, ns := range sub.Namespaces() {
		if ns.UUID() == "" || !strings.EqualFold(ns.UUID(), nsUUID) {
			continue
		}

		bd, err := s.block.DeviceByName(ctx, ns.Name())
		if err != nil {
			if block.IsDeviceNotFoundError(err) {
				return nil, errors.NotFoundError("NVMe device %s for namespace %s not found", ns.Name(), nsUUID)
			}
			return nil, fmt.Errorf("failed to read NVMe device %s: %w", ns.Name(), err)
		}
		return newDevice(bd, ns, nqn), nil
	}

	return nil, errors.NotFoundError("no device found for the given namespace %v", nsUUID)
}

func (s *service) EnsureFlush(ctx context.Context, dev *Device, opts FlushOptions) error {
	if err := beforeNVMeFlushDevice.Inject(); err != nil {
		return err
	}

	// A forced detach does not flush.
	if opts.Force || dev == nil {
		return nil
	}

	fields := LogFields{
		"deviceName":   dev.Name(),
		"devicePath":   dev.Path(),
		"subsystem":    dev.SubsystemNQN(),
		"ignoreErrors": opts.IgnoreErrors,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> nvme.EnsureFlush")
	defer Logc(ctx).WithFields(fields).Debug("<<<< nvme.EnsureFlush")

	return s.flushDevice(ctx, dev, opts)
}

// flushDevice checks that the device exists, then flushes it while the subsystem has a live path.
// If opts.IgnoreErrors is set, it ignores a missing live path and flush errors.
func (s *service) flushDevice(ctx context.Context, dev *Device, opts FlushOptions) error {
	fields := LogFields{
		"deviceName":   dev.Name(),
		"devicePath":   dev.Path(),
		"subsystem":    dev.SubsystemNQN(),
		"ignoreErrors": opts.IgnoreErrors,
	}
	Logc(ctx).WithFields(fields).Trace(">>>> nvme.flushDevice")
	defer Logc(ctx).WithFields(fields).Trace("<<<< nvme.flushDevice")

	// A device that is already gone has nothing to flush.
	if _, err := s.block.DeviceByName(ctx, dev.Name()); err != nil && block.IsDeviceNotFoundError(err) {
		Logc(ctx).WithFields(fields).Debug("Device already gone; skipping flush.")
		return nil
	}

	// A flush on a dead path would wait for the path to come back, so it is only safe with a live path.
	if !s.hasLivePath(ctx, dev.SubsystemNQN()) {
		if opts.IgnoreErrors {
			Logc(ctx).WithFields(fields).Warn("NVMe subsystem has no live path; skipping flush.")
			return nil
		}
		return newFlushError(
			fmt.Sprintf("NVMe subsystem %s has no live path; cannot flush device %s", dev.SubsystemNQN(), dev.Path()),
			nil)
	}

	// Always enforce a maximum bound.
	ctx, cancel := context.WithTimeout(ctx, flushTimeout)
	defer cancel()

	if err := s.nvme.Flush(ctx, dev.Path()); err != nil {
		if !opts.IgnoreErrors {
			Logc(ctx).WithFields(fields).WithError(err).Errorf("Failed to flush device: %s", dev.Path())
			return newFlushError(fmt.Sprintf("flush %s: %v", dev.Path(), err), err)
		}
		Logc(ctx).WithFields(fields).WithError(err).Warn("Failed to flush device; ignoring.")
	}

	return nil
}

// HasLivePath reports whether the subsystem has a path that can carry I/O.
// A subsystem that cannot be read has none.
func (s *service) HasLivePath(ctx context.Context, nqn string) bool {
	return s.hasLivePath(ctx, nqn)
}

// hasLivePath is the internal implementation.
func (s *service) hasLivePath(ctx context.Context, nqn string) bool {
	sub, err := s.readSubsystem(ctx, nqn)
	if err != nil {
		Logc(ctx).WithField("subsystem", nqn).WithError(err).Warn("Could not read NVMe subsystem paths.")
		return false
	}
	return sub != nil && sub.HasLivePath()
}

func (s *service) EnsureSize(ctx context.Context, dev *Device, targetBytes uint64) error {
	if dev == nil {
		return fmt.Errorf("no NVMe device specified")
	}
	if targetBytes == 0 {
		return fmt.Errorf("minimum size must be greater than 0")
	}

	fields := LogFields{"deviceName": dev.Name(), "targetSizeBytes": targetBytes}
	Logc(ctx).WithFields(fields).Debug(">>>> nvme.EnsureSize")
	defer Logc(ctx).WithFields(fields).Debug("<<<< nvme.EnsureSize")

	return s.waitForSize(ctx, dev, targetBytes)
}

// waitForSize polls the device until its size is at or above targetBytes.
// Convergence requires multiple consecutive stable size reads at the target size.
// NVMe does not require explicit rescans.
func (s *service) waitForSize(ctx context.Context, dev *Device, targetBytes uint64) error {
	fields := LogFields{"deviceName": dev.Name(), "targetSizeBytes": targetBytes}
	Logc(ctx).WithFields(fields).Trace(">>>> nvme.waitForSize")
	defer Logc(ctx).WithFields(fields).Trace("<<<< nvme.waitForSize")

	// Always enforce a maximum bound.
	ctx, cancel := context.WithTimeout(ctx, ensureSizeTimeout)
	defer cancel()

	var (
		timer    = time.NewTimer(0) // The first read fires immediately.
		stable   int
		known    *block.Device
		lastSize uint64
	)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("NVMe device '%s' size '%d' did not reach target size '%d' bytes: %w",
				dev.Name(), lastSize, targetBytes, ctx.Err())

		case <-timer.C:
			timer.Reset(s.ensureSizeInterval)

			current, err := s.block.DeviceByName(ctx, dev.Name())
			if err != nil {
				Logc(ctx).WithFields(fields).WithError(err).Warn("Failed to read NVMe device; retrying.")
				stable = 0
				continue
			}
			lastSize = current.Size()

			// If the device changed under us, the earlier reads say nothing about it.
			if known == nil || !current.IsEqual(known) {
				if known != nil {
					Logc(ctx).WithFields(LogFields{"currentDevice": current, "previousDevice": known}).Debug(
						"Device state changed during volume expansion; resetting stable reads.")
				}
				known = current
				stable = 0
			}

			if current.Size() < targetBytes {
				stable = 0
				continue
			}

			stable++
			if stable < ensureSizeStableReads {
				Logc(ctx).WithFields(LogFields{
					"currentDevice": current,
					"numberOfReads": stable,
					"requiredReads": ensureSizeStableReads,
				}).Debug("NVMe device at target size; re-reading size to confirm size convergence.")
				continue
			}

			Logc(ctx).WithFields(fields).Info("NVMe device expanded.")
			return nil
		}
	}
}

// EnsureDisconnect disconnects the subsystem when this namespace is its last one on the host.
// A subsystem with other namespaces is left alone, because it belongs to other volumes.
func (s *service) EnsureDisconnect(ctx context.Context, nqn, nsUUID string, force bool) error {
	fields := LogFields{"subsystem": nqn, "namespace": nsUUID, "force": force}
	Logc(ctx).WithFields(fields).Debug(">>>> nvme.EnsureDisconnect")
	defer Logc(ctx).WithFields(fields).Debug("<<<< nvme.EnsureDisconnect")

	if nqn == "" {
		return fmt.Errorf("empty subsystem NQN")
	}
	if nsUUID == "" {
		return fmt.Errorf("empty namespace UUID")
	}

	Logc(ctx).WithFields(fields).Info("Checking if subsystem should be disconnected.")

	sub, err := s.readSubsystem(ctx, nqn)
	if err != nil {
		return fmt.Errorf("failed to read NVMe subsystem %s: %w", nqn, err)
	}
	if sub == nil {
		Logc(ctx).WithFields(fields).Debug("NVMe subsystem already gone; nothing to disconnect.")
		return nil
	}

	// Without a live path the namespaces cannot be enumerated reliably. Disconnecting anyway could tear
	// down other volumes' controllers, so the subsystem is left alone unless the caller forces it.
	// Its paths reconnect on their own.
	if !sub.HasLivePath() {
		if !force {
			Logc(ctx).WithFields(fields).Warn(
				"NVMe paths are down, couldn't enumerate namespaces; skipping disconnect.")
			return nil
		}
		Logc(ctx).WithFields(fields).Warn(
			"NVMe paths are down, couldn't enumerate namespaces; disconnecting because force was requested.")
	} else {
		// A namespace whose UUID cannot be read counts as another namespace, since disconnecting the
		// subsystem would remove a device this host may still be using.
		for _, ns := range sub.Namespaces() {
			if ns.UUID() == "" || !strings.EqualFold(ns.UUID(), nsUUID) {
				Logc(ctx).WithFields(fields).Info(
					"Subsystem still has other namespace devices attached on host; skipping disconnect.")
				return nil
			}
		}
	}

	if err := beforeNVMeDisconnect.Inject(); err != nil {
		return err
	}

	if err := s.nvme.Disconnect(ctx, nqn); err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Errorf("Failed to disconnect subsystem %s.", nqn)
		return fmt.Errorf("disconnect subsystem %s: %w", nqn, err)
	}

	return nil
}

// Subsystem returns the service view of the subsystem.
func (s *service) Subsystem(ctx context.Context, nqn string) (*Subsystem, error) {
	fields := LogFields{"subsystem": nqn}
	Logc(ctx).WithFields(fields).Debug(">>>> nvme.Subsystem")
	defer Logc(ctx).WithFields(fields).Debug("<<<< nvme.Subsystem")

	if nqn == "" {
		return nil, fmt.Errorf("empty subsystem NQN")
	}

	sub, err := s.readSubsystem(ctx, nqn)
	if err != nil {
		return nil, fmt.Errorf("failed to read NVMe subsystem %s: %w", nqn, err)
	}
	if sub == nil || len(sub.Paths()) == 0 {
		return nil, errors.NotFoundError("no subsystem paths found for NVMe subsystem %s", nqn)
	}

	return sub, nil
}

// ConnectPaths connects the target IPs that have no path to the subsystem, once.
func (s *service) ConnectPaths(ctx context.Context, nqn string, targetIPs []string) error {
	fields := LogFields{"subsystem": nqn, "targetIPs": targetIPs}
	Logc(ctx).WithFields(fields).Debug(">>>> nvme.ConnectPaths")
	defer Logc(ctx).WithFields(fields).Debug("<<<< nvme.ConnectPaths")

	if nqn == "" {
		return fmt.Errorf("empty subsystem NQN")
	}

	// A subsystem that cannot be read is an error here: connecting every IP blindly would try to
	// duplicate paths that exist.
	sub, err := s.readSubsystem(ctx, nqn)
	if err != nil {
		return fmt.Errorf("failed to read NVMe subsystem %s: %w", nqn, err)
	}

	missing := missingPaths(sub, targetIPs)
	if len(missing) == 0 {
		return nil
	}
	Logc(ctx).WithFields(fields).WithField("missingIPs", missing).Debug("NVMe subsystem has missing paths; connecting.")

	_, err = s.connect(ctx, nqn, missing)
	return err
}

// PrunePaths removes the paths of data LIFs that no longer serve the subsystem.
func (s *service) PrunePaths(ctx context.Context, nqn string, desiredIPs []string) error {
	fields := LogFields{"subsystem": nqn, "desiredIPs": desiredIPs}
	Logc(ctx).WithFields(fields).Debug(">>>> nvme.PrunePaths")
	defer Logc(ctx).WithFields(fields).Debug("<<<< nvme.PrunePaths")

	sub, err := s.Subsystem(ctx, nqn)
	if err != nil {
		return err
	}

	paths := sub.Paths()
	// The namespace head survives only while the subsystem has a path, so this keeps a prune from renaming
	// /dev/nvmeXnY underneath a mounted volume. Both counts are kept locally as paths are removed.
	remaining := len(paths)
	liveRemaining := 0
	for _, path := range paths {
		if path.IsLive() {
			liveRemaining++
		}
	}

	for _, path := range paths {
		if remaining <= 1 {
			break
		}

		ip := path.Address()
		if ip == "" || slices.Contains(desiredIPs, ip) {
			continue
		}

		if path.IsLive() && liveRemaining <= 1 {
			continue
		}

		Logc(ctx).WithFields(LogFields{
			"subsystem": nqn,
			"path":      path.Name(),
			"targetIP":  ip,
			"state":     path.State(),
		}).Info("Removing a subsystem path whose data LIF no longer serves this subsystem.")

		if err := s.disconnectPath(ctx, path); err != nil {
			// Leave it in place; the next data LIF change re-prunes it.
			Logc(ctx).WithError(err).WithField("path", path.Name()).
				Debug("Could not remove the path; it will be retried on the next data LIF change.")
			continue
		}

		remaining--
		if path.IsLive() {
			liveRemaining--
		}
	}

	return nil
}

// disconnectPath removes one path of a subsystem.
func (s *service) disconnectPath(ctx context.Context, path *Path) error {
	if err := beforeNVMeDisconnect.Inject(); err != nil {
		return err
	}
	return s.nvme.DisconnectPath(ctx, path.Name())
}

// missingPaths returns the target IPs that have no path in the subsystem.
// A nil subsystem has no paths.
func missingPaths(sub *nvme.Subsystem, targetIPs []string) []string {
	var missing []string
	for _, ip := range targetIPs {
		if !pathPresent(sub, ip) {
			missing = append(missing, ip)
		}
	}
	return missing
}

// anyPathPresent reports whether at least one of the target IPs has a path in the subsystem.
func anyPathPresent(sub *nvme.Subsystem, targetIPs []string) bool {
	for _, ip := range targetIPs {
		if pathPresent(sub, ip) {
			return true
		}
	}
	return false
}

func pathPresent(sub *nvme.Subsystem, ip string) bool {
	if sub == nil {
		return false
	}
	for _, p := range sub.Paths() {
		if p.Address() == ip {
			return true
		}
	}
	return false
}
