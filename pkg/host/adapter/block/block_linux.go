// Copyright 2026 NetApp, Inc. All Rights Reserved.

package block

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/afero"

	. "github.com/netapp/trident/logging"
	tridentexec "github.com/netapp/trident/utils/exec"
)

const (
	sysfsBlockPath = "/sys/block"
	dmDevicePrefix = "dm-"

	execDDStatusNone          = "none"
	execDDZeroDeviceInputFile = "/dev/zero"
	execDDTimeout             = 5 * time.Second
	execBlkDevTimeout         = 30 * time.Second
	execBlkIDTimeout          = 30 * time.Second
	execWipeFSTimeout         = 10 * time.Second
	execDMSetupRemoveTimeout  = 10 * time.Second
)

var errDoesNotExist = os.ErrNotExist

func (a *adapter) DeviceByName(ctx context.Context, deviceName string) (*Device, error) {
	fields := LogFields{"deviceName": deviceName}
	Logc(ctx).WithFields(fields).Trace(">>>> block_linux.Device")
	defer Logc(ctx).WithFields(fields).Trace("<<<< block_linux.Device")

	return a.device(ctx, deviceName)
}

// DeviceByDMUUID queries the host for a device that has the matching dm uuid.
// DM UUIDs should be unique on a system.
func (a *adapter) DeviceByDMUUID(ctx context.Context, dmUUID string) (*Device, error) {
	fields := LogFields{"dmUUID": dmUUID}
	Logc(ctx).WithFields(fields).Trace(">>>> block_linux.DeviceByDMUUID")
	defer Logc(ctx).WithFields(fields).Trace("<<<< block_linux.DeviceByDMUUID")

	return a.deviceByDMAttribute(ctx, "uuid", dmUUID)
}

// DeviceByDMName queries the host for a dm-* device whose dm/name (the mapper
// name, i.e. the entry under /dev/mapper/) matches the supplied name.
//
// Prefer this over walking holders of the underlying device when the caller
// already has the mapper name in hand (e.g. immediately after cryptsetup
// open): holders are populated lazily by udev and may briefly lag behind the
// dm-* entry itself, whereas /sys/block/dm-N/dm/name is written atomically by
// the kernel when the mapper is created.
func (a *adapter) DeviceByDMName(ctx context.Context, dmName string) (*Device, error) {
	fields := LogFields{"dmName": dmName}
	Logc(ctx).WithFields(fields).Trace(">>>> block_linux.DeviceByDMName")
	defer Logc(ctx).WithFields(fields).Trace("<<<< block_linux.DeviceByDMName")

	return a.deviceByDMAttribute(ctx, "name", dmName)
}

// deviceByDMAttribute is the shared sweep for DeviceByDMUUID and DeviceByDMName.
// It globs /sys/block/dm-*/dm/<file> and returns the first dm-* device whose
// attribute file matches value exactly.
func (a *adapter) deviceByDMAttribute(ctx context.Context, file, value string) (*Device, error) {
	Logc(ctx).Trace(">>>> block_linux.deviceByDMAttribute")
	defer Logc(ctx).Trace("<<<< block_linux.deviceByDMAttribute")

	if file == "" {
		return nil, fmt.Errorf("device attribute file not specified")
	}
	if value == "" {
		return nil, fmt.Errorf("device attribute value not specified")
	}

	pattern := a.constructDMGlobPath(file)
	matches, err := afero.Glob(a.fs, pattern)
	if err != nil {
		return nil, fmt.Errorf("globbing %s: %w", pattern, err)
	}

	for _, path := range matches {
		contents, err := readSysfsFile(a.fs, path)
		if err != nil || contents != value {
			continue
		}

		// Extract kernel device node name: "/sys/block/<dm-#>/dm/<attr>" → "dm-#"
		name := filepath.Base(filepath.Dir(filepath.Dir(path)))
		Logc(ctx).WithFields(LogFields{
			"deviceName":     name,
			"attributePath":  path,
			"attributeValue": contents,
		}).Trace("Found dm device by attribute.")
		return a.device(ctx, name)
	}

	return nil, &DeviceNotFoundError{
		dev: "<nil>",
		err: fmt.Errorf("no dm device found with attribute %s=%s", file, value),
	}
}

func (a *adapter) Slaves(ctx context.Context, deviceName string) ([]string, error) {
	fields := LogFields{"deviceName": deviceName}
	Logc(ctx).WithFields(fields).Trace(">>>> block_linux.Slaves")
	defer Logc(ctx).WithFields(fields).Trace("<<<< block_linux.Slaves")
	return a.slaves(ctx, deviceName)
}

func (a *adapter) Holders(ctx context.Context, deviceName string) ([]string, error) {
	Logc(ctx).Trace(">>>> block_linux.Holders")
	defer Logc(ctx).Trace("<<<< block_linux.Holders")
	return a.holders(ctx, deviceName)
}

// device reads block-layer attributes from /sys/block/<name>.
// It does NOT follow the device/ symlink into protocol subsystems (SCSI, NVMe).
// For DM devices, it reads dm/uuid and dm/name (which are real subdirectories
// within the block layer, not symlinks).
func (a *adapter) device(ctx context.Context, deviceName string) (*Device, error) {
	fields := LogFields{"deviceName": deviceName}
	Logc(ctx).WithFields(fields).Trace(">>>> block_linux.device")
	defer Logc(ctx).WithFields(fields).Trace("<<<< block_linux.device")

	if deviceName == "" {
		return nil, fmt.Errorf("device name is required")
	}

	// Check if the device exists.
	// If it doesn't or the device file is unreadable, there's no point in continuing.
	blockDevPath := a.constructDevicePath(deviceName)
	if _, err := a.fs.Stat(blockDevPath); err != nil {
		if os.IsNotExist(err) {
			Logc(ctx).WithFields(fields).WithField(
				"path", blockDevPath,
			).Debug("Device not found.")
			return nil, &DeviceNotFoundError{dev: deviceName, err: err}
		}

		Logc(ctx).WithFields(fields).WithField(
			"path", blockDevPath,
		).WithError(err).Error("Failed to check for device existence.")
		return nil, err
	}

	// Read all common block-layer attributes.

	// Slave and Holder devices technically can be empty for a block device.
	// Some block devices have slaves (top-most dm devices),
	// others have slaves and holders (mid-stack dm devices),
	// and some only have holders (bottom-stack devices, like disks).
	slaves, err := a.slaves(ctx, deviceName)
	if err != nil && !errors.Is(err, errDoesNotExist) {
		Logc(ctx).WithFields(fields).Error("Failed to read device slaves.")
		return nil, err
	}
	holders, err := a.holders(ctx, deviceName)
	if err != nil && !errors.Is(err, errDoesNotExist) {
		Logc(ctx).WithFields(fields).Error("Failed to read device holders.")
		return nil, err
	}

	// The size and device number should always be populated by the kernel,
	// but we should not fail if they aren't here yet.
	size, _ := readSizeBytes(a.fs, a.constructDeviceSizePath(deviceName))
	major, minor, _ := readDeviceNumber(a.fs, a.constructDeviceMajorMinorPath(deviceName))
	dev := &Device{
		name:    deviceName,
		path:    DevicePath(deviceName),
		major:   major,
		minor:   minor,
		size:    size,
		slaves:  slaves,
		holders: holders,
	}

	// Read DM-specific attributes. The dm/ subdirectory is a real directory
	// within the block layer, not a symlink into another subsystem.
	// All dm entries in /sys/block should have this populated.
	if strings.HasPrefix(deviceName, dmDevicePrefix) {
		dmUUID, err := readSysfsFile(a.fs, a.constructDeviceDMUUIDPath(deviceName))
		if err != nil {
			Logc(ctx).WithFields(fields).WithError(err).Errorf("Error reading device-mapper UUID.")
			return nil, fmt.Errorf("failed to read device-mapper info: %w", err)
		}
		dmName, err := readSysfsFile(a.fs, a.constructDeviceDMNamePath(deviceName))
		if err != nil {
			Logc(ctx).WithFields(fields).WithError(err).Errorf("Error reading device-mapper name.")
			return nil, fmt.Errorf("failed to read device-mapper info: %w", err)
		}

		Logc(ctx).WithFields(LogFields{
			"dmUUID": dmUUID,
			"dmName": dmName,
		}).WithFields(fields).Debug("DM device information gathered.")
		dev.dmInfo = &DMDevice{
			uuid: dmUUID,
			name: dmName,
		}
	}

	Logc(ctx).WithFields(fields).WithField("deviceInfo", dev).Debug("Device information gathered.")
	return dev, nil
}

func (a *adapter) slaves(ctx context.Context, deviceName string) ([]string, error) {
	fields := LogFields{"deviceName": deviceName}
	Logc(ctx).WithFields(fields).Trace(">>>> block_linux.slaves")
	defer Logc(ctx).WithFields(fields).Trace("<<<< block_linux.slaves")

	dirPath := a.constructDeviceSlavesPath(deviceName)
	names, err := readDirNames(a.fs, dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			Logc(ctx).WithField("deviceName", deviceName).Debug("No slaves directory.")
			return nil, fmt.Errorf("no slaves at %s: %w", dirPath, errDoesNotExist)
		}
		return nil, err
	}

	return names, nil
}

func (a *adapter) holders(ctx context.Context, deviceName string) ([]string, error) {
	fields := LogFields{"deviceName": deviceName}
	Logc(ctx).WithFields(fields).Trace(">>>> block_linux.holders")
	defer Logc(ctx).WithFields(fields).Trace("<<<< block_linux.holders")

	dirPath := a.constructDeviceHoldersPath(deviceName)
	names, err := readDirNames(a.fs, dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			Logc(ctx).WithField("deviceName", deviceName).Debug("No holders directory.")
			return nil, fmt.Errorf("no holders at %s: %w", dirPath, errDoesNotExist)
		}
		return nil, err
	}

	return names, nil
}

func readDirNames(fs afero.Fs, dir string) ([]string, error) {
	entries, err := afero.ReadDir(fs, dir)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		// Return nil, not an empty slice, so callers (Slaves/Holders) can honor
		// their nil-when-empty contract.
		return nil, nil
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	return names, nil
}

// readDeviceNumber parses /sys/block/<name>/dev ("major:minor\n") into two
// uint32 values. Returns (0, 0, err) on any parse failure.
func readDeviceNumber(afs afero.Fs, path string) (major, minor uint32, err error) {
	s, err := readSysfsFile(afs, path)
	if err != nil || s == "" {
		return 0, 0, err
	}
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected device number format %q", s)
	}
	maj, err := strconv.ParseUint(parts[0], 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing major from %q: %w", s, err)
	}
	min, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("parsing minor from %q: %w", s, err)
	}
	return uint32(maj), uint32(min), nil
}

func readSizeBytes(afs afero.Fs, path string) (uint64, error) {
	s, err := readSysfsFile(afs, path)
	if err != nil || s == "" {
		return 0, err
	}

	sectors, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, err
	}

	return sectors * 512, nil
}

// Flush flushes outstanding I/O for a block device.
// Equivalent to: blockdev --flushbufs <devicePath>
//
// The devicePath must be a /dev/ path (e.g. /dev/sda, /dev/dm-0).
// This should be called before removing a device to drain the
// kernel's buffer cache and avoid data loss.
// RemoveByDMUUID removes the dm-* device whose dm/uuid matches dmUUID via
// dmsetup remove -u <dmUUID>. The kernel atomically rejects the ioctl if the
// UUID no longer matches the dm minor, so this is safe against minor reuse.
// When force is true, -f is added to ask the kernel to detach the device
// even when it is still open.
func (a *adapter) RemoveByDMUUID(ctx context.Context, dmUUID string, force bool) error {
	Logc(ctx).WithFields(LogFields{"dmUUID": dmUUID, "force": force}).Trace(">>>> block_linux.RemoveByDMUUID")
	defer Logc(ctx).WithFields(LogFields{"dmUUID": dmUUID, "force": force}).Trace("<<<< block_linux.RemoveByDMUUID")

	if dmUUID == "" {
		return fmt.Errorf("dmUUID is required")
	}
	args := []string{"remove", "-u", dmUUID}
	if force {
		args = append(args, "-f")
	}
	out, err := a.cmd.ExecuteWithTimeout(ctx, "dmsetup", execDMSetupRemoveTimeout, true, args...)
	if err != nil {
		return fmt.Errorf("dmsetup remove -u %s failed: %w (output: %s)", dmUUID, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (a *adapter) Flush(ctx context.Context, devicePath string) error {
	Logc(ctx).WithField("devicePath", devicePath).Trace(">>>> block_linux.Flush")
	defer Logc(ctx).WithField("devicePath", devicePath).Trace("<<<< block_linux.Flush")

	if devicePath == "" {
		return fmt.Errorf("device path is required")
	}
	args := []string{"--flushbufs", devicePath}

	_, err := a.cmd.ExecuteWithTimeout(ctx, "blockdev", execBlkDevTimeout, true, args...)
	if err != nil {
		return &DeviceFlushError{err: err, dev: devicePath}
	}

	return nil
}

func (a *adapter) Copy(ctx context.Context, inputPath, outputPath string, opts DataOptions) ([]byte, error) {
	Logc(ctx).Trace(">>>> block_linux.Copy")
	defer Logc(ctx).Trace("<<<< block_linux.Copy")

	if inputPath == "" {
		return nil, fmt.Errorf("input path is required")
	}
	if outputPath == "" {
		return nil, fmt.Errorf("output path is required")
	}

	if err := a.validateDataOptions(opts); err != nil {
		return nil, err
	}

	return a.dd(ctx, inputPath, outputPath, opts)
}

func (a *adapter) Read(ctx context.Context, devicePath string, opts DataOptions) ([]byte, error) {
	Logc(ctx).Trace(">>>> block_linux.Read")
	defer Logc(ctx).Trace("<<<< block_linux.Read")

	if devicePath == "" {
		return nil, fmt.Errorf("device path is required")
	}

	if err := a.validateDataOptions(opts); err != nil {
		return nil, err
	}

	return a.dd(ctx, devicePath, "", opts)
}

func (a *adapter) Zero(ctx context.Context, devicePath string, opts DataOptions) error {
	Logc(ctx).Trace(">>>> block_linux.Zero")
	defer Logc(ctx).Trace("<<<< block_linux.Zero")

	if devicePath == "" {
		return fmt.Errorf("device path is required")
	}
	if !opts.Force {
		return fmt.Errorf("zero operation requires Force to be true")
	}

	if err := a.validateDataOptions(opts); err != nil {
		return err
	}

	_, err := a.dd(ctx, execDDZeroDeviceInputFile, devicePath, opts)
	return err
}

// dd helpers

// validateDataOptions validates the block I/O knobs shared by Copy, Read, and Zero.
func (a *adapter) validateDataOptions(opts DataOptions) error {
	if opts.BlockSize <= 0 {
		return fmt.Errorf("block size must be positive, got %d", opts.BlockSize)
	}
	if opts.BlockCount <= 0 {
		return fmt.Errorf("block count must be positive, got %d", opts.BlockCount)
	}
	if opts.BlockSkip < 0 {
		return fmt.Errorf("block skip must be non-negative, got %d", opts.BlockSkip)
	}

	return nil
}

// dd runs dd with the supplied paths and options and returns stdout.
// outputPath may be empty (read-to-stdout mode).
func (a *adapter) dd(ctx context.Context, inputPath, outputPath string, opts DataOptions) ([]byte, error) {
	fields := LogFields{"inputPath": inputPath, "outputPath": outputPath}
	Logc(ctx).WithFields(fields).Trace(">>>> dd_linux.dd")
	defer Logc(ctx).WithFields(fields).Trace("<<<< dd_linux.dd")

	args := []string{"if=" + inputPath}

	if outputPath != "" {
		args = append(args, "of="+outputPath)
	}

	args = append(args, fmt.Sprintf("bs=%d", opts.BlockSize))

	if opts.BlockCount > 0 {
		args = append(args, fmt.Sprintf("count=%d", opts.BlockCount))
	}

	if opts.BlockSkip > 0 {
		args = append(args, fmt.Sprintf("skip=%d", opts.BlockSkip))
	}

	args = append(args, "status="+execDDStatusNone)

	out, err := a.cmd.ExecuteWithTimeout(ctx, "dd", execDDTimeout, opts.LogOutput, args...)
	if err != nil {
		Logc(ctx).WithField(
			"output", string(out),
		).WithError(err).Error("Failed to execute dd command.")
		return nil, err
	}

	return out, nil
}

// constructBlockPath assembles the sysfs base path for the block device class.
// All block devices on the host appear as a child of this directory.
//
// Example: <chroot>/sys/block/
func (a *adapter) constructBlockPath() string {
	return filepath.Join(
		a.chrt,
		sysfsBlockPath,
	)
}

// constructDevicePath assembles the sysfs base path for a single block device's
// kernel attributes (size, slaves, holders, dm/, etc.).
//
// Example: <chroot>/sys/block/<name>/
func (a *adapter) constructDevicePath(deviceName string) string {
	return filepath.Join(
		a.constructBlockPath(),
		deviceName,
	)
}

// constructDeviceSizePath assembles the sysfs path to a block device's size
// attribute (in 512-byte sectors).
//
// Example: <chroot>/sys/block/<name>/size
func (a *adapter) constructDeviceSizePath(deviceName string) string {
	return filepath.Join(
		a.constructDevicePath(deviceName),
		"size",
	)
}

// constructDeviceMajorMinorPath assembles the sysfs path to a block device's
// dev attribute, which contains the major:minor device number (e.g. "253:3").
//
// Example: <chroot>/sys/block/<name>/dev
func (a *adapter) constructDeviceMajorMinorPath(deviceName string) string {
	return filepath.Join(
		a.constructDevicePath(deviceName),
		"dev",
	)
}

// constructDeviceSlavesPath assembles the sysfs path to a block device's
// slaves directory: the underlying devices this device is built on top of.
//
// Example: <chroot>/sys/block/<name>/slaves
func (a *adapter) constructDeviceSlavesPath(deviceName string) string {
	return filepath.Join(
		a.constructDevicePath(deviceName),
		"slaves",
	)
}

// constructDeviceHoldersPath assembles the sysfs path to a block device's
// holders directory: the devices stacked on top of this device (dm,
// multipath, partitions, etc.).
//
// Example: <chroot>/sys/block/<name>/holders
func (a *adapter) constructDeviceHoldersPath(deviceName string) string {
	return filepath.Join(
		a.constructDevicePath(deviceName),
		"holders",
	)
}

// constructDeviceDMPath assembles the sysfs path to the device-mapper
// subdirectory for a dm-* block device. dm/ is a real directory within the
// block layer, not a symlink into another subsystem.
//
// Example: <chroot>/sys/block/<name>/dm
func (a *adapter) constructDeviceDMPath(deviceName string) string {
	return filepath.Join(
		a.constructDevicePath(deviceName),
		"dm",
	)
}

// constructDeviceDMUUIDPath assembles the sysfs path to a dm-* device's
// dm/uuid attribute.
//
// Example: <chroot>/sys/block/<name>/dm/uuid
func (a *adapter) constructDeviceDMUUIDPath(deviceName string) string {
	return filepath.Join(
		a.constructDeviceDMPath(deviceName),
		"uuid",
	)
}

// constructDeviceDMNamePath assembles the sysfs path to a dm-* device's
// dm/name attribute (the mapper name, i.e. the entry under /dev/mapper/).
//
// Example: <chroot>/sys/block/<name>/dm/name
func (a *adapter) constructDeviceDMNamePath(deviceName string) string {
	return filepath.Join(
		a.constructDeviceDMPath(deviceName),
		"name",
	)
}

// constructDMGlobPath assembles a glob pattern for sweeping a single dm-*
// attribute file (e.g. uuid, name) across all device-mapper devices on the
// host. Used to look up a dm-* device by an attribute value.
//
// Example: <chroot>/sys/block/dm-*/dm/<name|uuid|...>
func (a *adapter) constructDMGlobPath(attr string) string {
	return filepath.Join(
		a.constructBlockPath(),
		dmDevicePrefix+"*",
		"dm",
		attr,
	)
}

// readSysfsFile reads a sysfs text attribute and returns its trimmed content.
// A missing file returns ("", nil). In sysfs, a missing attribute means the
// kernel doesn't expose it for this object.
func readSysfsFile(fs afero.Fs, path string) (string, error) {
	b, err := afero.ReadFile(fs, path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func defaultProbeOptionType() ProbeOptions {
	return ProbeOptions{
		Tags:   []Tag{TagType},
		Cache:  false,
		Output: OutputValue,
	}
}

func (a *adapter) Type(ctx context.Context, devicePath string) (string, error) {
	Logc(ctx).Trace(">>>> block_linux.Type")
	defer Logc(ctx).Trace("<<<< block_linux.Type")

	if devicePath == "" {
		return "", fmt.Errorf("device path is required")
	}

	tags, err := a.probe(ctx, devicePath, defaultProbeOptionType())
	if err != nil {
		return "", err
	}

	val, ok := tags[TagType]
	if !ok {
		return "", &NoFilesystemSignatureError{err: fmt.Errorf("no fs type on device %v", devicePath)}
	}

	return val, nil
}

// Format creates a filesystem on the given block device.
func (a *adapter) Format(ctx context.Context, devicePath, fsType, options string) error {
	fields := LogFields{
		"device":  devicePath,
		"fsType":  fsType,
		"options": options,
	}
	Logc(ctx).WithFields(fields).Trace(">>>> block_linux.Format")
	defer Logc(ctx).WithFields(fields).Trace("<<<< block_linux.Format")

	if devicePath == "" {
		return errors.New("device path is required for filesystem format")
	}
	if fsType == "" {
		return errors.New("filesystem type is required for filesystem format")
	}

	var args []string
	if options != "" {
		args = append(args, strings.Fields(options)...)
	}

	var bin string
	switch fsType {
	case Xfs:
		bin = "mkfs.xfs"
		args = append(args, "-f", devicePath)
	case Ext3:
		bin = "mkfs.ext3"
		args = append(args, "-F", devicePath)
	case Ext4:
		bin = "mkfs.ext4"
		args = append(args, "-F", devicePath)
	default:
		return fmt.Errorf("unsupported filesystem type: %s", fsType)
	}

	if _, err := a.cmd.Execute(ctx, bin, args...); err != nil {
		return err
	}

	return nil
}

// Repair runs the appropriate fsck tool for fsType on devicePath.
// XFS is a deliberate no-op (fsck.xfs does nothing by design).
func (a *adapter) Repair(ctx context.Context, devicePath, fsType string) error {
	fields := LogFields{
		"device": devicePath,
		"fsType": fsType,
	}
	Logc(ctx).WithFields(fields).Trace(">>>> block_linux.Repair")
	defer Logc(ctx).WithFields(fields).Trace("<<<< block_linux.Repair")

	if devicePath == "" {
		return errors.New("device path is required")
	}
	if fsType == "" {
		return errors.New("filesystem type is required for filesystem repair")
	}

	var bin string
	switch fsType {
	case Xfs:
		// fsck.xfs is a no-op by design; XFS uses xfs_repair separately.
		return nil
	case Ext3:
		bin = "fsck.ext3"
	case Ext4:
		bin = "fsck.ext4"
	default:
		return fmt.Errorf("unsupported filesystem type for fsck: %s", fsType)
	}
	args := []string{"-p", devicePath}

	// Do not hard-kill fsck; an interrupted repair is worse than a slow one.
	_, err := a.cmd.Execute(ctx, bin, args...)
	if err != nil {
		var execErr tridentexec.ExitError
		if errors.As(err, &execErr) && isFsckCorrectedExitCode(execErr.ExitCode()) {
			return &FsckFilesystemCorrectedError{err: err, code: execErr.ExitCode()}
		}
		return err
	}

	return nil
}

func (a *adapter) Wipe(ctx context.Context, devicePath string, opts EraseOptions) error {
	fields := LogFields{"devicePath": devicePath, "opts": opts}
	Logc(ctx).WithFields(fields).Trace(">>>> block_linux.Wipe")
	defer Logc(ctx).WithFields(fields).Trace("<<<< block_linux.Wipe")

	if devicePath == "" {
		return fmt.Errorf("device path is required")
	}
	args := []string{"-a"} // always erase every signature visible to libblkid

	if opts.Force {
		args = append(args, "--force")
	}
	if opts.BackupPath != "" {
		args = append(args, "--backup="+opts.BackupPath)
	}
	if len(opts.Types) > 0 {
		args = append(args, "--types", strings.Join(opts.Types, ","))
	}
	if opts.Quiet {
		args = append(args, "--quiet")
	}
	if opts.NoAct {
		args = append(args, "--no-act")
	}
	args = append(args, devicePath)

	_, err := a.cmd.ExecuteWithTimeout(ctx, "wipefs", execWipeFSTimeout, opts.LogOutput, args...)
	if err != nil {
		return err
	}

	return nil
}

// probe runs blkid with the given options and returns parsed tags.
func (a *adapter) probe(ctx context.Context, devicePath string, opts ProbeOptions) (Tags, error) {
	args := make([]string, 0)

	switch opts.Output {
	case OutputValue:
		args = append([]string{"-o", "value"}, args...)
	case OutputExport:
		args = append([]string{"-o", "export"}, args...)
	default:
		// default, no additional args needed.
	}

	if opts.Probe {
		args = append([]string{"-p"}, args...)
	}
	if !opts.Cache {
		args = append([]string{"-c", "/dev/null"}, args...)
	}
	for _, tag := range opts.Tags {
		args = append(args, "-s", string(tag))
	}
	args = append(args, devicePath)

	out, err := a.cmd.ExecuteWithTimeout(ctx, "blkid", execBlkIDTimeout, true, args...)
	if err != nil {
		var execErr tridentexec.ExitError
		if errors.As(err, &execErr) && execErr.ExitCode() == ExitBlkidNoMatch {
			return nil, &NoFilesystemSignatureError{err: err}
		}
		return nil, err
	}
	output := strings.TrimSpace(string(out))

	switch opts.Output {
	case OutputExport:
		return a.parseExportOutput(output, opts.Tags)
	case OutputValue:
		return a.parseValueOutput(output, opts.Tags)
	case OutputList:
		return nil, errors.New("'blkid -o list' is not supported")
	case OutputDefault:
		fallthrough
	default:
		return a.parseDefaultOutput(output, opts.Tags)
	}
}

func (a *adapter) parseExportOutput(output string, tags []Tag) (Tags, error) {
	result := make(Tags)

	whitelist := make(map[Tag]bool, len(tags))
	for _, tag := range tags {
		whitelist[tag] = true
	}
	useWhiteList := len(whitelist) > 0

	for _, rawLine := range strings.Split(output, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		sep := strings.IndexByte(line, '=')
		if sep <= 0 {
			continue
		}

		key := Tag(line[:sep])
		if key == "DEVNAME" {
			continue
		}

		if useWhiteList && !whitelist[key] {
			continue
		}

		result[key] = strings.TrimSpace(line[sep+1:])
	}

	return result, nil
}

func (a *adapter) parseValueOutput(output string, tags []Tag) (Tags, error) {
	result := make(Tags)
	values := make([]string, 0)
	if output != "" {
		values = strings.Split(output, "\n")
	}
	if len(values) != len(tags) {
		return nil, fmt.Errorf(
			"blkid returned %d values for %d tags (%s)",
			len(values),
			len(tags),
			output,
		)
	}

	for i, tag := range tags {
		result[tag] = strings.TrimSpace(values[i])
	}

	return result, nil
}

func (a *adapter) parseDefaultOutput(output string, tags []Tag) (Tags, error) {
	result := make(Tags)

	whitelist := make(map[Tag]bool, len(tags))
	for _, t := range tags {
		whitelist[t] = true
	}
	useWhiteList := len(whitelist) > 0

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if idx := strings.Index(line, ":"); idx >= 0 {
			line = strings.TrimSpace(line[idx+1:])
		}

		for _, field := range strings.Fields(line) {
			parts := strings.SplitN(field, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := Tag(parts[0])

			if useWhiteList && !whitelist[key] {
				continue
			}

			value := strings.TrimSpace(parts[1])
			value = strings.Replace(value, "\"", "", -1)
			result[key] = strings.TrimSpace(value)
		}
	}

	return result, nil
}
