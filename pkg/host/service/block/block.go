// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package block provides methods to inspect and act on block devices on the host.
// It owns: reading, measuring, and discovering device relationships; detecting the
// on-disk filesystem signature; formatting a device; and running filesystem repair
// (fsck). It has no knowledge of live-filesystem operations (mount, expand, size).
//
// It is agnostic to all protocol and storage stack knowledge.
package block

import (
	"context"
	"errors"
	"fmt"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/host/adapter/block"
	utilserrors "github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

var (
	// ErrOrphanMapperDevice indicates a device-mapper device has slaves on the host, but none of them can be resolved.
	// This is what a mapper looks like after the block device under it was removed.
	// It is returned, wrapped, by ResolveDeviceStack so callers can tell it apart from a failure to read the host.
	ErrOrphanMapperDevice = errors.New("device-mapper device has no live device under it")

	// ErrUnknownFilesystem indicates a device holds a filesystem signature that is not recognized.
	ErrUnknownFilesystem = errors.New("unknown filesystem type")
)

// maxStackDepth bounds the slaves and holders walks, as a defense against cycles.
const maxStackDepth = 8

type (
	Device         = block.Device
	FilesystemType = models.FilesystemType
)

// Service defines device-level utilities composed of the block adapter.
// This interface should remain agnostic to the higher transport protocol.
type Service interface {
	// IsReadable verifies devicePath is accessible by attempting a single
	// 4 KiB read. Returns an error if the read fails or the byte count is
	// short.
	IsReadable(ctx context.Context, devicePath string) error

	// IsEmpty reports whether the leading 2 MiB of devicePath are all zero
	// bytes. It makes no interpretation of what the bytes mean; filesystem
	// signature detection belongs in the fs service.
	//
	// Returns:
	//   - (true, nil)    - leading 2 MiB are all zeros; device appears empty.
	//   - (false, nil)   - at least one non-zero byte found; device has data.
	//   - (false, error) - I/O error or short read; result is unknown.
	IsEmpty(ctx context.Context, devicePath string) (bool, error)

	// ResolveTopDevice walks each candidate name's holder chain in
	// /sys/block/<name>/holders/ and returns the first device whose top
	// holder differs from itself. Multiple paths to the same underlying
	// disk can share the same higher-layer holder (multipath, NVMe, etc.);
	// trying each candidate absorbs transient races where one path's
	// sysfs entry is incomplete.
	//
	// With a single candidate the device's own top holder is returned
	// (the candidate itself if there are no holders above it).
	//
	// With multiple candidates, returning the input device itself is
	// treated as an error: paths exist but no higher-layer holder has
	// claimed them.
	//
	// If the holders chain exceeds an internal depth bound (defense
	// against pathological cycles), an error is returned.
	ResolveTopDevice(ctx context.Context, deviceNames ...string) (*Device, error)

	// ResolveDeviceStack accepts any block device name in a stack and returns
	// the full ordered stack from bottom (index 0 = raw disk) to top (last
	// index = topmost layer like dm-crypt). When multiple slaves exist at the
	// bottom, only the first valid path is followed.
	//
	// If a device has slaves but none of them resolve (the block device under a
	// mapper was removed), the error wraps ErrOrphanMapperDevice.
	ResolveDeviceStack(ctx context.Context, blockDeviceName string) ([]*Device, error)

	// IsMapperStale reports whether the device-mapper device with dmUUID no longer sits on the block device
	// baseDeviceName (kernel name, e.g. "nvme0n1"). It is stale when it is orphaned (no slaves, or every slave
	// is gone) or when the bottom of its stack is some other device. A mapper that is not on the host is not
	// stale. An error means the stack could not be read, and staleness is unknown.
	IsMapperStale(ctx context.Context, dmUUID, baseDeviceName string) (bool, error)

	// ResolveType returns the filesystem type to use for a volume. An explicit
	// requestedFS is used as-is. Otherwise the type already on the device is
	// adopted (skipped when skipProbe is set), then volumeMode decides:
	// "Filesystem" defaults to ext4 and "Block" to raw. An unrecognized on-disk
	// filesystem on a "Filesystem" volume returns ErrUnknownFilesystem.
	// An empty result means no type could be resolved.
	ResolveType(ctx context.Context, devicePath, requestedFS, volumeMode string, skipProbe bool) (FilesystemType, error)

	// EnsureFormat ensures the device at devicePath is formatted with
	// requestedFS. It is a no-op if the device already carries that
	// filesystem. probe selects the checks run before formatting; see ProbeMode.
	EnsureFormat(ctx context.Context, devicePath, requestedFS, formatOptions string, probe ProbeMode) error

	// Repair runs filesystem repair (fsck) on the device. Best-effort: errors
	// are logged but not returned. No-op if the filesystem is "raw". The caller
	// must ensure the device is not mounted.
	Repair(ctx context.Context, devicePath string, fsType FilesystemType)
}

type service struct {
	block block.Adapter
}

type Option func(*service)

func WithBlockAdapter(b block.Adapter) Option { return func(s *service) { s.block = b } }

func New(opts ...Option) Service {
	s := &service{
		block: block.New(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// IsReadable attempts a single 4 KiB read from devicePath and returns nil on
// success. A short read or non-nil read error is returned as-is.
func (s *service) IsReadable(ctx context.Context, devicePath string) error {
	path := block.DevicePath(devicePath)
	Logc(ctx).WithField("devicePath", path).Trace(">>>> device.IsReadable")
	defer Logc(ctx).WithField("devicePath", path).Trace("<<<< device.IsReadable")

	const blockSize = block.ProbeBlockSize
	buf, err := s.block.Read(ctx, path, block.DataOptions{BlockSize: blockSize, BlockCount: 1})
	if err != nil {
		Logc(ctx).WithFields(LogFields{"devicePath": path, "bytesRead": len(buf)}).
			WithError(err).Error("Failed to read device.")
		return err
	}
	if len(buf) != blockSize {
		return fmt.Errorf("expected %d bytes but read %d bytes from %s", blockSize, len(buf), path)
	}
	return nil
}

// IsEmpty reports whether the leading 2 MiB of devicePath are all zero.
func (s *service) IsEmpty(ctx context.Context, devicePath string) (bool, error) {
	path := block.DevicePath(devicePath)
	Logc(ctx).WithField("devicePath", path).Trace(">>>> device.IsEmpty")
	defer Logc(ctx).WithField("devicePath", path).Trace("<<<< device.IsEmpty")

	buf, err := s.block.Read(ctx, path, block.DataOptions{
		BlockSize:  block.ProbeBlockSize,
		BlockCount: block.EmptyCheckBlockCount,
	})
	if err != nil {
		return false, fmt.Errorf("failed to read device %s: %w", path, err)
	}
	if len(buf) != block.EmptyCheckSize {
		return false, fmt.Errorf("read %d bytes (expected %d) from %s", len(buf), block.EmptyCheckSize, path)
	}

	for _, b := range buf {
		if b != 0 {
			return false, nil
		}
	}
	return true, nil
}

// ResolveDeviceStack accepts any block device name in a device stack and
// returns the full stack ordered bottom-to-top (index 0 = raw disk, last
// index = topmost layer such as dm-crypt). When multiple slaves exist at the
// bottom layer (e.g. multiple SCSI paths), only the first valid one is
// followed - the caller typically already knows the multipath device and
// enters there.
func (s *service) ResolveDeviceStack(ctx context.Context, blockDeviceName string) ([]*Device, error) {
	Logc(ctx).WithField("blockDeviceName", blockDeviceName).Trace(">>>> device.ResolveDeviceStack")
	defer Logc(ctx).Trace("<<<< device.ResolveDeviceStack")

	entryDev, err := s.block.DeviceByName(ctx, blockDeviceName)
	if err != nil {
		return nil, translateErr(err)
	}

	// Walk DOWN from the entry through slaves to find the bottom device.
	bottom, err := s.walkToBottom(ctx, entryDev, maxStackDepth)
	if err != nil {
		return nil, err
	}

	// Walk UP from the bottom through holders to produce the ordered stack.
	stack, err := s.walkToTop(ctx, bottom, maxStackDepth)
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithField("stack", stack).Debug("Resolved device stack.")
	return stack, nil
}

func (s *service) IsMapperStale(ctx context.Context, dmUUID, baseDeviceName string) (bool, error) {
	fields := LogFields{"dmUUID": dmUUID, "baseDeviceName": baseDeviceName}
	Logc(ctx).WithFields(fields).Trace(">>>> device.IsMapperStale")
	defer Logc(ctx).Trace("<<<< device.IsMapperStale")

	mapper, err := s.block.DeviceByDMUUID(ctx, dmUUID)
	if err != nil {
		if block.IsDeviceNotFoundError(err) {
			return false, nil
		}
		return false, err
	}

	// A mapper with no slaves is its own bottom device.
	bottom, err := s.walkToBottom(ctx, mapper, maxStackDepth)
	if err != nil {
		if errors.Is(err, ErrOrphanMapperDevice) {
			return true, nil
		}
		return false, err
	}
	return bottom.Name() != baseDeviceName, nil
}

// ResolveTopDevice walks each candidate's holder chain and returns the first
// candidate whose top holder differs from itself. With multiple candidates,
// returning the input device itself is treated as an error: at least one path
// exists but no higher-layer holder has claimed it.
func (s *service) ResolveTopDevice(ctx context.Context, deviceNames ...string) (*Device, error) {
	Logc(ctx).WithField("deviceNames", deviceNames).Trace(">>>> device.ResolveTopDevice")
	defer Logc(ctx).Trace("<<<< device.ResolveTopDevice")

	if len(deviceNames) == 0 {
		return nil, fmt.Errorf("at least one block device name is required")
	}

	var fallback *Device
	for _, name := range deviceNames {
		if name == "" {
			continue
		}

		top, err := s.topHolder(ctx, name)
		if err != nil {
			Logc(ctx).WithField("blockDevice", name).WithError(err).
				Debug("Could not resolve top holder for block device; continuing.")
			continue
		}
		if top.Name() != name {
			return top, nil
		}

		if fallback == nil {
			fallback = top
		}
	}

	if fallback == nil {
		return nil, fmt.Errorf("no valid holders for block devices: %v", deviceNames)
	}
	if len(deviceNames) > 1 {
		return nil, fmt.Errorf(
			"block device paths exist %v but none have a holder; a higher layer "+
				"(multipath, nvme, LUKS, etc.) should have claimed them but have not", deviceNames,
		)
	}

	return fallback, nil
}

// walkToBottom follows the slaves chain from dev until it reaches a device
// with no slaves. When a device has multiple slaves (e.g. multipath → multiple
// sd* paths), the first valid slave is chosen.
func (s *service) walkToBottom(ctx context.Context, dev *Device, remaining int) (*Device, error) {
	if remaining <= 0 {
		return nil, fmt.Errorf("max depth exceeded walking slaves from %s", dev.Name())
	}

	slaves := dev.Slaves()
	if len(slaves) == 0 {
		return dev, nil
	}

	for _, slaveName := range slaves {
		slave, err := s.block.DeviceByName(ctx, slaveName)
		if err != nil {
			if !block.IsDeviceNotFoundError(err) {
				// A transient read error is not evidence the slave is gone; propagate it.
				return nil, fmt.Errorf("reading slave %s of %s: %w", slaveName, dev.Name(), err)
			}
			Logc(ctx).WithField("slave", slaveName).WithError(err).
				Debug("Slave device not found.")
			continue
		}
		return s.walkToBottom(ctx, slave, remaining-1)
	}

	// Every slave in the list was confirmed gone (NotFoundError for each).
	return nil, fmt.Errorf("%s is orphaned; %w", dev.Name(), ErrOrphanMapperDevice)
}

// walkToTop follows the holders chain from dev, appending each device to the
// stack. When a device has multiple holders (uncommon), the first valid holder
// is followed.
func (s *service) walkToTop(ctx context.Context, dev *Device, remaining int) ([]*Device, error) {
	if remaining <= 0 {
		return nil, fmt.Errorf("max depth exceeded walking holders from %s", dev.Name())
	}

	stack := []*Device{dev}

	holders := dev.Holders()
	if len(holders) == 0 {
		return stack, nil
	}

	for _, holderName := range holders {
		holder, err := s.block.DeviceByName(ctx, holderName)
		if err != nil {
			Logc(ctx).WithField("holder", holderName).WithError(err).
				Debug("Skipping unresolvable holder.")
			continue
		}
		rest, err := s.walkToTop(ctx, holder, remaining-1)
		if err != nil {
			return nil, err
		}
		return append(stack, rest...), nil
	}

	return stack, nil
}

// topHolder walks the holders chain for a single device name and returns
// the top-most *Device. Cycles should never occur in holder chains, but a
// small depth bound prevents pathological inputs from looping forever.
func (s *service) topHolder(ctx context.Context, deviceName string) (*Device, error) {
	fields := LogFields{"deviceName": deviceName}
	Logc(ctx).WithFields(fields).Trace(">>>> device.topHolder")
	defer Logc(ctx).WithFields(fields).Trace("<<<< device.topHolder")

	if deviceName == "" {
		return nil, fmt.Errorf("device name is required")
	}

	dev, err := s.block.DeviceByName(ctx, deviceName)
	if err != nil {
		return nil, err
	}

	const maxHolderDepth = 5
	for i := 0; len(dev.Holders()) > 0 && i < maxHolderDepth; i++ {
		holder := dev.Holders()[0]
		next, err := s.block.DeviceByName(ctx, holder)
		if err != nil {
			return nil, err
		}
		if next != nil {
			dev = next
		}
	}

	if len(dev.Holders()) > 0 {
		Logc(ctx).WithFields(LogFields{
			"foundDevice":   dev,
			"deviceHolders": dev.Holders(),
		}).Error("Max holder depth exceeded.")
		return nil, fmt.Errorf("max holder depth reached but top device still had holders")
	}

	Logc(ctx).WithFields(fields).WithField("topDevice", dev).Debug("Resolved top-most holder.")
	return dev, nil
}

func translateErr(err error) error {
	if block.IsDeviceNotFoundError(err) {
		return utilserrors.NotFoundError(err.Error())
	}
	return err
}

// ResolveType returns the filesystem type to apply to a volume.
func (s *service) ResolveType(ctx context.Context, devicePath, requestedFS, volumeMode string, skipProbe bool) (FilesystemType, error) {
	fields := LogFields{
		"devicePath":  devicePath,
		"requestedFS": requestedFS,
		"volumeMode":  volumeMode,
		"skipProbe":   skipProbe,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> device.ResolveType")
	defer Logc(ctx).WithFields(fields).Debug("<<<< device.ResolveType")

	if requestedFS != "" {
		return requestedFS, nil
	}

	var onDisk string
	if !skipProbe {
		detectedFS, err := s.detect(ctx, devicePath)
		switch {
		case err == nil:
			onDisk = detectedFS
		case errors.Is(err, ErrUnknownFilesystem):
			onDisk = models.FilesystemEmpty
		default:
			return "", err
		}
	}

	switch {
	case onDisk != "" && onDisk != models.FilesystemEmpty:
		return onDisk, nil
	case volumeMode == string(config.Filesystem):
		if onDisk == models.FilesystemEmpty {
			return "", ErrUnknownFilesystem
		}
		return models.FilesystemExt4, nil
	case volumeMode == string(config.RawBlock):
		return models.FilesystemRaw, nil
	}
	return "", nil
}

// ProbeMode selects what EnsureFormat checks on a device before formatting it.
type ProbeMode int

const (
	// ProbeFull refuses to format a device that holds a different filesystem or any non-zero data.
	ProbeFull ProbeMode = iota
	// ProbeFilesystem refuses to format a device that holds a different filesystem, but formats one with
	// no filesystem even if its data is not zero. Use it for an opened LUKS mapper, whose unformatted
	// contents are ciphertext.
	ProbeFilesystem
	// ProbeNone formats without reading the device. Use it for a LUKS mapper formatted by the same attach.
	ProbeNone
)

// EnsureFormat ensures the device at devicePath is formatted with requestedFS.
func (s *service) EnsureFormat(
	ctx context.Context, devicePath, requestedFS, formatOptions string, probe ProbeMode,
) error {
	fields := LogFields{
		"devicePath":  devicePath,
		"requestedFS": requestedFS,
		"formatOpts":  formatOptions,
		"probe":       probe,
	}
	Logc(ctx).WithFields(fields).Debug(">>>> device.EnsureFormat")
	defer Logc(ctx).WithFields(fields).Debug("<<<< device.EnsureFormat")

	if devicePath == "" {
		return fmt.Errorf("device path is empty")
	}
	if requestedFS == "" {
		return fmt.Errorf("requested filesystem type is empty")
	}

	if requestedFS == models.FilesystemRaw {
		Logc(ctx).Debug("No filesystem requested; skipping format.")
		return nil
	}

	if probe != ProbeNone {
		detectedFS, err := s.detect(ctx, devicePath)
		if err != nil {
			Logc(ctx).WithFields(fields).WithError(err).Error("Could not detect filesystem on device.")
			return err
		}

		if detectedFS == requestedFS {
			Logc(ctx).WithFields(LogFields{
				"device":     devicePath,
				"detectedFS": detectedFS,
			}).Info("Device already has the requested filesystem; skipping format.")
			return nil
		}

		if detectedFS != "" {
			return fmt.Errorf("device %s has existing filesystem %s; cannot reformat as %s",
				devicePath, detectedFS, requestedFS)
		}
	}
	if probe == ProbeFull {
		if err := s.requireEmpty(ctx, devicePath); err != nil {
			Logc(ctx).WithFields(fields).WithError(err).Error("Device is not safe to format.")
			return err
		}
	}

	Logc(ctx).WithFields(LogFields{
		"device":        devicePath,
		"requestedFS":   requestedFS,
		"formatOptions": formatOptions,
	}).Info("Formatting device with filesystem.")
	return s.block.Format(ctx, devicePath, requestedFS, formatOptions)
}

// Repair runs fsck on the device. Best-effort: errors are logged but not
// returned. The caller is responsible for ensuring the device is unmounted
// before calling Repair; running fsck on a mounted device risks data corruption.
func (s *service) Repair(ctx context.Context, devicePath string, fsType FilesystemType) {
	Logc(ctx).WithFields(LogFields{
		"device": devicePath,
		"fsType": fsType,
	}).Debug(">>>> device.Repair")
	defer Logc(ctx).Debug("<<<< device.Repair")

	if fsType == models.FilesystemRaw || fsType == "" {
		return
	}

	if err := s.block.Repair(ctx, devicePath, fsType); err != nil {
		if block.IsFsckCorrectedError(err) {
			Logc(ctx).WithField("device", devicePath).Info("Repair reported filesystem errors that were corrected.")
			return
		}
		Logc(ctx).WithError(err).WithFields(LogFields{
			"device": devicePath,
			"fsType": fsType,
		}).Warn("Filesystem repair reported issues.")
	}
}

// detect returns the filesystem type on devicePath, or "" if blkid found no
// recognizable signature. Pure observation; no mutation.
func (s *service) detect(ctx context.Context, devicePath string) (string, error) {
	fields := LogFields{"devicePath": devicePath}
	Logc(ctx).WithFields(fields).Trace(">>>> device.detect")
	defer Logc(ctx).WithFields(fields).Trace("<<<< device.detect")

	if devicePath == "" {
		return "", fmt.Errorf("device path is required")
	}

	detectedFS, err := s.block.Type(ctx, devicePath)
	if err != nil {
		if block.IsNoFilesystemSignatureError(err) {
			Logc(ctx).WithField("devicePath", devicePath).Debug("Device has no filesystem signature.")
			return "", nil
		}
		return "", fmt.Errorf("blkid failed on %s: %w", devicePath, err)
	}

	Logc(ctx).WithFields(LogFields{
		"devicePath": devicePath,
		"detectedFS": detectedFS,
	}).Debug("Detected filesystem.")

	switch detectedFS {
	case models.FilesystemExt3:
		return models.FilesystemExt3, nil
	case models.FilesystemExt4:
		return models.FilesystemExt4, nil
	case models.FilesystemXFS:
		return models.FilesystemXFS, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnknownFilesystem, detectedFS)
	}
}

// requireEmpty returns nil if the device's leading 2 MiB are all zero, or an
// error if the device contains any non-zero bytes.
func (s *service) requireEmpty(ctx context.Context, devicePath string) error {
	fields := LogFields{"devicePath": devicePath}
	Logc(ctx).WithFields(fields).Trace(">>>> device.requireEmpty")
	defer Logc(ctx).WithFields(fields).Trace("<<<< device.requireEmpty")

	isEmpty, err := s.IsEmpty(ctx, devicePath)
	if err != nil {
		return fmt.Errorf("could not determine if device is empty: %w", err)
	}
	if !isEmpty {
		return fmt.Errorf("device %s is not empty", devicePath)
	}
	return nil
}
