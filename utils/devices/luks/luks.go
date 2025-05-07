// Copyright 2024 NetApp, Inc. All Rights Reserved.

package luks

//go:generate mockgen -destination=../../../mocks/mock_utils/mock_devices/mock_luks/mock_luks.go -package mock_luks github.com/netapp/trident/utils/devices/luks Device

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/afero"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/errors"
	execCmd "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/lsblk"
)

const (
	devicePrefix = "luks-"
	// LUKS2 requires ~16MiB for overhead. Default to 18MiB just in case.
	MetadataSize = 18874368
)

type Device interface {
	EnsureDeviceMappedOnHost(ctx context.Context, name string, secrets map[string]string) (bool, error)
	MappedDevicePath() string
	MappedDeviceName() string
	RawDevicePath() string
	EnsureFormattedAndOpen(ctx context.Context, luksPassphrase string) (bool, error)
	CheckPassphrase(ctx context.Context, luksPassphrase string) (bool, error)
	RotatePassphrase(ctx context.Context, volumeId, previousLUKSPassphrase, luksPassphrase string) error
}

type LUKSDevice struct {
	rawDevicePath    string
	mappedDeviceName string
	command          execCmd.Command
	devices          devices.Devices
	osFs             afero.Fs
}

func NewDevice(rawDevicePath, volumeId string, command execCmd.Command) *LUKSDevice {
	luksDeviceName := devicePrefix + volumeId
	devices := devices.New()
	osFs := afero.NewOsFs()
	return NewDetailed(rawDevicePath, luksDeviceName, command, devices, osFs)
}

func NewDetailed(rawDevicePath, mappedDeviceName string, command execCmd.Command, devices devices.Devices,
	osFs afero.Fs,
) *LUKSDevice {
	return &LUKSDevice{
		rawDevicePath:    rawDevicePath,
		mappedDeviceName: mappedDeviceName,
		command:          command,
		devices:          devices,
		osFs:             osFs,
	}
}

func NewDeviceFromMappingPath(
	ctx context.Context, command execCmd.Command, mappingPath, volumeId string,
) (*LUKSDevice, error) {
	rawDevicePath, err := GetUnderlyingDevicePathForDevice(ctx, command, mappingPath)
	if err != nil {
		return nil, fmt.Errorf("could not determine underlying device for LUKS mapping; %v", err)
	}
	return NewDevice(rawDevicePath, volumeId, command), nil
}

// EnsureLUKSDeviceMappedOnHost ensures the specified device is LUKS formatted, opened, and has the current passphrase.
func (d *LUKSDevice) EnsureDeviceMappedOnHost(ctx context.Context, name string, secrets map[string]string) (bool, error) {
	// Try to Open with current luks passphrase
	luksPassphraseName, luksPassphrase, previousLUKSPassphraseName, previousLUKSPassphrase := GetLUKSPassphrasesFromSecretMap(secrets)
	if luksPassphrase == "" {
		return false, fmt.Errorf("LUKS passphrase cannot be empty")
	}
	if luksPassphraseName == "" {
		return false, fmt.Errorf("LUKS passphrase name cannot be empty")
	}

	Logc(ctx).WithFields(LogFields{
		"volume":               name,
		"luks-passphrase-name": luksPassphraseName,
	}).Info("Opening encrypted volume.")
	luksFormatted, err := d.EnsureFormattedAndOpen(ctx, luksPassphrase)

	// If we fail due to a format issue there is no need to try to open the device.
	if err == nil || errors.IsFormatError(err) {
		return luksFormatted, err
	}

	// If we failed to open, try previous passphrase
	if previousLUKSPassphrase == "" {
		// Return original error if there is no previous passphrase to use
		return luksFormatted, fmt.Errorf("could not open LUKS device; %v", err)
	}
	if luksPassphrase == previousLUKSPassphrase {
		return luksFormatted, fmt.Errorf("could not open LUKS device, previous passphrase matches current")
	}
	if previousLUKSPassphraseName == "" {
		return luksFormatted, fmt.Errorf("could not open LUKS device, no previous passphrase name provided")
	}
	Logc(ctx).WithFields(LogFields{
		"volume":               name,
		"luks-passphrase-name": previousLUKSPassphraseName,
	}).Info("Opening encrypted volume.")
	luksFormatted, err = d.EnsureFormattedAndOpen(ctx, previousLUKSPassphrase)
	if err != nil {
		return luksFormatted, fmt.Errorf("could not open LUKS device; %v", err)
	}

	return luksFormatted, nil
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
func (d *LUKSDevice) EnsureFormattedAndOpen(ctx context.Context, luksPassphrase string) (formatted bool, err error) {
	return d.ensureLUKSDevice(ctx, luksPassphrase)
}

func (d *LUKSDevice) ensureLUKSDevice(ctx context.Context, luksPassphrase string) (bool, error) {
	// First check if LUKS device is already opened. This is OK to check even if the device isn't LUKS formatted.
	if isOpen, err := d.IsOpen(ctx); err != nil {
		// If the LUKS device isn't found, it means that we need to check if the device is LUKS formatted.
		// If it isn't, then we should format it and attempt to open it.
		// If any other error occurs, bail out.
		if !errors.IsNotFoundError(err) {
			Logc(ctx).WithError(err).Error("Could not check if device is an open LUKS device.")
			return false, err
		}
	} else if isOpen {
		Logc(ctx).Debug("Device is LUKS formatted and open.")
		return true, nil
	}

	if err := d.formatUnformattedDevice(ctx, luksPassphrase); err != nil {
		Logc(ctx).WithError(err).Error("Could not LUKS format device.")
		return false, fmt.Errorf("could not LUKS format device; %w", err)
	}

	// At this point, we should be able to open the device.
	if err := d.Open(ctx, luksPassphrase); err != nil {
		// At this point, we couldn't open the LUKS device, but we do know
		// the device is LUKS formatted because LUKSFormat didn't fail.
		Logc(ctx).WithError(err).Error("Could not open LUKS formatted device.")
		return true, fmt.Errorf("could not open LUKS device; %v", err)
	}

	Logc(ctx).Debug("Device is LUKS formatted and open.")
	return true, nil
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
		return "", fmt.Errorf("device path is not a legacy LUKS device path")
	}
	lsblk := lsblk.NewLsblkUtilDetailed(command)
	dev, err := lsblk.GetParentDeviceKname(ctx, devicePath)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"devicePath": devicePath,
		}).WithError(err).Error("Unable to determine luks parent device.")
	}
	return devices.DevPrefix + dev, nil
}
