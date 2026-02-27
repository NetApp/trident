// Copyright 2025 NetApp, Inc. All Rights Reserved.

package luks

//go:generate mockgen -destination=../../../mocks/mock_utils/mock_devices/mock_luks/mock_luks.go -package mock_luks github.com/netapp/trident/utils/devices/luks OS,Device

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/afero"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/errors"
	execCmd "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/lsblk"
	"github.com/netapp/trident/utils/models"
)

const (
	devicePrefix = "luks-"
	// MetadataSize is ~16MiB for overhead for LUKS2 headers. Default to 18MiB just in case.
	MetadataSize = 18874368
)

// OS is an abstraction over afero.Fs, afero.LinkReader and os utility functions
// to provide a concise interface for LUKSDevice's to interact with the filesystem.
// It should not be used outside of this package, but it is exported for mock generation.
type OS interface {
	Stat(name string) (os.FileInfo, error)
	Glob(pattern string) ([]string, error)
	ReadDir(dirname string) ([]os.FileInfo, error)
	ReadFile(filename string) ([]byte, error)
	ReadlinkIfPossible(name string) (string, error)
}

// osFs is a helper that wraps operations on the OS filesystem and implements osFsAbs.
// It should not be used outside of this package.
type osFs struct {
	fs afero.Fs
}

// Ensure osFs always implements OS.
var _ OS = &osFs{}

func newOsFs(fs afero.Fs) OS {
	return &osFs{fs: fs}
}

func (o *osFs) Stat(name string) (os.FileInfo, error) {
	return o.fs.Stat(name)
}

func (o *osFs) Glob(pattern string) ([]string, error) {
	return afero.Glob(o.fs, pattern)
}

func (o *osFs) ReadDir(dirname string) ([]os.FileInfo, error) {
	return afero.ReadDir(o.fs, dirname)
}

func (o *osFs) ReadFile(filename string) ([]byte, error) {
	return afero.ReadFile(o.fs, filename)
}

func (o *osFs) ReadlinkIfPossible(name string) (string, error) {
	if lr, ok := o.fs.(afero.LinkReader); ok {
		return lr.ReadlinkIfPossible(name)
	}
	// Symlinks don't work with in-memory filesystems.
	// Regardless, fall back to os.Readlink.
	return os.Readlink(name)
}

type Device interface {
	EnsureDeviceMappedOnHost(ctx context.Context, name string, secrets map[string]string) (bool, bool, error)
	MappedDevicePath() string
	MappedDeviceName() string
	RawDevicePath() string
	EnsureFormattedAndOpen(ctx context.Context, luksPassphrase string) (bool, bool, error)
	CheckPassphrase(ctx context.Context, luksPassphrase string) (bool, error)
	RotatePassphrase(ctx context.Context, volumeId, previousLUKSPassphrase, luksPassphrase string) error
	IsMappingStale(ctx context.Context) bool
}

type LUKSDevice struct {
	rawDevicePath    string
	mappedDeviceName string
	command          execCmd.Command
	devices          devices.Devices
	osFs             OS
}

func NewDevice(rawDevicePath, volumeId string, command execCmd.Command, devices devices.Devices) *LUKSDevice {
	luksDeviceName := devicePrefix + volumeId
	return NewDetailed(rawDevicePath, luksDeviceName, command, devices, afero.NewOsFs())
}

func NewDetailed(
	rawDevicePath, mappedDeviceName string, command execCmd.Command, devices devices.Devices, osFs afero.Fs,
) *LUKSDevice {
	return &LUKSDevice{
		rawDevicePath:    rawDevicePath,
		mappedDeviceName: mappedDeviceName,
		command:          command,
		devices:          devices,
		osFs:             newOsFs(osFs),
	}
}

func NewDeviceFromMappingPath(
	ctx context.Context, command execCmd.Command, devices devices.Devices, mappingPath, volumeId string,
) (*LUKSDevice, error) {
	rawDevicePath, err := GetUnderlyingDevicePathForDevice(ctx, command, mappingPath)
	if err != nil {
		return nil, fmt.Errorf("could not determine underlying device for LUKS mapping; %v", err)
	}
	return NewDevice(rawDevicePath, volumeId, command, devices), nil
}

// EnsureDeviceMappedOnHost ensures the specified device is LUKS formatted, opened, and has the current passphrase.
func (d *LUKSDevice) EnsureDeviceMappedOnHost(ctx context.Context, name string, secrets map[string]string) (bool, bool, error) {
	// Try to Open with current luks passphrase
	luksPassphraseName, luksPassphrase, previousLUKSPassphraseName, previousLUKSPassphrase := GetLUKSPassphrasesFromSecretMap(secrets)
	if luksPassphrase == "" {
		return false, false, errors.New("LUKS passphrase cannot be empty")
	}
	if luksPassphraseName == "" {
		return false, false, errors.New("LUKS passphrase name cannot be empty")
	}

	Logc(ctx).WithFields(LogFields{
		"volume":               name,
		"luks-passphrase-name": luksPassphraseName,
	}).Info("Opening encrypted volume.")
	luksFormatted, safeToFormat, err := d.EnsureFormattedAndOpen(ctx, luksPassphrase)

	// If we fail due to a format issue there is no need to try to open the device.
	if err == nil || errors.IsFormatError(err) {
		return luksFormatted, safeToFormat, err
	}

	// If we failed to open, try previous passphrase
	if previousLUKSPassphrase == "" {
		// Return original error if there is no previous passphrase to use
		return luksFormatted, safeToFormat, fmt.Errorf("could not open LUKS device; %v", err)
	}
	if luksPassphrase == previousLUKSPassphrase {
		return luksFormatted, safeToFormat, errors.New("could not open LUKS device, previous passphrase matches current")
	}
	if previousLUKSPassphraseName == "" {
		return luksFormatted, safeToFormat, errors.New("could not open LUKS device, no previous passphrase name provided")
	}
	Logc(ctx).WithFields(LogFields{
		"volume":               name,
		"luks-passphrase-name": previousLUKSPassphraseName,
	}).Info("Opening encrypted volume.")
	luksFormatted, safeToFormat, err = d.EnsureFormattedAndOpen(ctx, previousLUKSPassphrase)
	if err != nil {
		return luksFormatted, safeToFormat, fmt.Errorf("could not open LUKS device; %v", err)
	}

	return luksFormatted, safeToFormat, nil
}

// MappedDevicePath returns the location of the LUKS device when opened.
func (d *LUKSDevice) MappedDevicePath() string {
	return devices.DevMapperRoot + d.mappedDeviceName
}

// MappedDeviceName returns the name of the LUKS device when opened.
func (d *LUKSDevice) MappedDeviceName() string {
	return d.mappedDeviceName
}

// RawDevicePath returns the original device location, such as the iscsi device or multipath device. Ex: /dev/sdb
func (d *LUKSDevice) RawDevicePath() string {
	return d.rawDevicePath
}

// EnsureFormattedAndOpen ensures the specified device is LUKS formatted and opened.
// Returns two booleans: the first indicates if the device is crypt formatted,
// the second indicates if the device is safe to file format, meaning the device was empty before being crypt formatted.
func (d *LUKSDevice) EnsureFormattedAndOpen(ctx context.Context, luksPassphrase string) (
	formatted, safeToFileFormat bool, err error,
) {
	return d.ensureLUKSDevice(ctx, luksPassphrase)
}

// IsMappingStale checks if the LUKS mapping is stale (i.e., mapped but the underlying device is gone).
// Currently, this is only supported for LUKS w/NVMe devices.
func (d *LUKSDevice) IsMappingStale(ctx context.Context) bool {
	fields := LogFields{
		"devicePath": d.rawDevicePath,
		"mappedPath": d.MappedDevicePath(),
	}
	Logc(ctx).WithFields(fields).Debug(">>>> luks.IsMappingStale")
	defer Logc(ctx).WithFields(fields).Debug("<<<< luks.IsMappingStale")
	return d.isMappingStale(ctx)
}

// ensureLUKSDevice ensures the device is LUKS formatted and opened.
// Returns two booleans: the first indicates if the device is formatted,
// the second indicates if the device is safe to file format, meaning the device was empty before being crypt formatted.
func (d *LUKSDevice) ensureLUKSDevice(ctx context.Context, luksPassphrase string) (bool, bool, error) {
	// First check if LUKS device is already opened. This is OK to check even if the device isn't LUKS formatted.
	if isOpen, err := d.IsOpen(ctx); err != nil {
		// If the LUKS device isn't found, it means that we need to check if the device is LUKS formatted.
		// If it isn't, then we should format it and attempt to open it.
		// If any other error occurs, bail out.
		if !errors.IsNotFoundError(err) {
			Logc(ctx).WithError(err).Error("Could not check if device is an open LUKS device.")
			return false, false, err
		}
	} else if isOpen {
		Logc(ctx).Debug("Device is LUKS formatted and open.")
		return true, false, nil
	}

	var safeToFileFormat bool
	var err error
	if safeToFileFormat, err = d.formatUnformattedDevice(ctx, luksPassphrase); err != nil {
		Logc(ctx).WithError(err).Error("Could not LUKS format device.")
		return false, safeToFileFormat, fmt.Errorf("could not LUKS format device; %w", err)
	}

	// At this point, we should be able to open the device.
	if err := d.Open(ctx, luksPassphrase); err != nil {
		// At this point, we couldn't open the LUKS device, but we do know
		// the device is LUKS formatted because LUKSFormat didn't fail.
		Logc(ctx).WithError(err).Error("Could not open LUKS formatted device.")
		return true, safeToFileFormat, fmt.Errorf("could not open LUKS device; %v", err)
	}

	Logc(ctx).Debug("Device is LUKS formatted and open.")
	return true, safeToFileFormat, nil
}

func GetLUKSPassphrasesFromSecretMap(secrets map[string]string) (string, string, string, string) {
	var luksPassphraseName, luksPassphrase, previousLUKSPassphraseName, previousLUKSPassphrase string
	luksPassphrase = secrets["luks-passphrase"]
	luksPassphraseName = secrets["luks-passphrase-name"]
	previousLUKSPassphrase = secrets["previous-luks-passphrase"]
	previousLUKSPassphraseName = secrets["previous-luks-passphrase-name"]

	return luksPassphraseName, luksPassphrase, previousLUKSPassphraseName, previousLUKSPassphrase
}

// IsLegacyDevicePath returns true if the device path points to mapped LUKS device instead of mpath device.
func IsLegacyDevicePath(devicePath string) bool {
	return strings.Contains(devicePath, "luks")
}

// GetDmDevicePathFromLUKSLegacyPath returns the device mapper path (e.g. /dev/dm-0) for a legacy LUKS device
// path (e.g. /dev/mapper/luks-<UUID>).
func GetDmDevicePathFromLUKSLegacyPath(
	ctx context.Context, command execCmd.Command, devicePath string,
) (string, error) {
	if !IsLegacyDevicePath(devicePath) {
		return "", errors.New("device path is not a legacy LUKS device path")
	}
	lsblk := lsblk.NewLsblkUtilDetailed(command)
	dev, err := lsblk.GetParentDeviceKname(ctx, devicePath)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"devicePath": devicePath,
		}).WithError(err).Error("Unable to determine LUKS parent device.")
	}
	return devices.DevPrefix + dev, nil
}

// IsLuksDevice parses the VolumePublishInfo to determine if it is a LUKS device.
func IsLuksDevice(publishInfo *models.VolumePublishInfo) (bool, error) {
	var err error
	var isLUKSDevice bool
	if publishInfo.LUKSEncryption != "" {
		isLUKSDevice, err = strconv.ParseBool(publishInfo.LUKSEncryption)
		if err != nil {
			return false, fmt.Errorf("could not parse LUKSEncryption into a bool, got %v",
				publishInfo.LUKSEncryption)
		}
	}
	return isLUKSDevice, nil
}

// EnsureCryptsetupFormattedAndMappedOnHost checks if the device is a LUKS device, and ensures it is formatted and
// mapped on the host.
func EnsureCryptsetupFormattedAndMappedOnHost(
	ctx context.Context, name string, publishInfo *models.VolumePublishInfo, secrets map[string]string, command execCmd.Command,
	devices devices.Devices,
) (bool, bool, error) {
	Logc(ctx).Debug(">>>> EnsureCryptsetupFormattedAndMappedOnHost")
	defer Logc(ctx).Debug("<<<< EnsureCryptsetupFormattedAndMappedOnHost")

	isLUKSDevice, err := IsLuksDevice(publishInfo)
	if err != nil {
		return isLUKSDevice, false, err
	}

	var luksFormatted bool
	var safeToFsFormat bool
	if isLUKSDevice {
		luksDevice := NewDevice(publishInfo.DevicePath, name, command, devices)
		luksFormatted, safeToFsFormat, err = luksDevice.EnsureDeviceMappedOnHost(ctx, name, secrets)
		if err != nil {
			return luksFormatted, safeToFsFormat, err
		}
	}
	return luksFormatted, safeToFsFormat, nil
}
