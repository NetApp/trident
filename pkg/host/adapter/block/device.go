// Copyright 2026 NetApp, Inc. All Rights Reserved.

package block

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/netapp/trident/pkg/collection"
)

const devicePathPrefix = "/dev/"

// Device is a read-only snapshot of a block device on the host.
//
// It contains ONLY block-layer attributes: name, path, size, slaves, holders,
// and for DM devices, the device-mapper metadata.
//
// Protocol-specific identity (SCSI serial, NVMe NGUID, etc.) is NOT here.
// That information belongs to the respective subsystem adapter and is
// accessed separately, correlated by the device name.
type Device struct {
	name    string   // kernel name: "sda", "nvme0n1", "dm-0"
	path    string   // dev path: "/dev/sda", "/dev/nvme0n1", "/dev/dm-0"
	major   uint32   // from /sys/block/<name>/dev — kernel-assigned major device number
	minor   uint32   // from /sys/block/<name>/dev — kernel-assigned minor device number
	size    uint64   // size in bytes (from /sys/block/<name>/size sectors * 512)
	slaves  []string // from /sys/block/<name>/slaves/ — device names
	holders []string // from /sys/block/<name>/holders/ — device names
	dmInfo  *DMDevice
}

func (d *Device) Name() string      { return d.name }
func (d *Device) Path() string      { return d.path }
func (d *Device) Size() uint64      { return d.size }
func (d *Device) Major() uint32     { return d.major }
func (d *Device) Minor() uint32     { return d.minor }
func (d *Device) DMInfo() *DMDevice { return d.dmInfo }
func (d *Device) IsMpath() bool     { return d.dmInfo != nil && d.dmInfo.IsMpath() }
func (d *Device) IsCrypt() bool     { return d.dmInfo != nil && d.dmInfo.IsCrypt() }

// RuntimeID returns "major:minor" (e.g. "253:3") as found in
// /proc/self/mountinfo field 3.
//
// This is a runtime-only kernel identifier: it is reassigned on every
// boot and must never be stored or used as a persistent device identity.
func (d *Device) RuntimeID() string {
	if d.major == 0 && d.minor == 0 {
		return ""
	}
	return fmt.Sprintf("%d:%d", d.major, d.minor)
}

func (d *Device) Slaves() []string {
	if d.slaves == nil {
		return nil
	}
	out := make([]string, len(d.slaves))
	copy(out, d.slaves)
	return out
}

func (d *Device) Holders() []string {
	if d.holders == nil {
		return nil
	}
	out := make([]string, len(d.holders))
	copy(out, d.holders)
	return out
}

func (d *Device) String() string {
	var dmStr string
	if d.dmInfo != nil {
		dmStr = d.dmInfo.String()
	} else {
		dmStr = "<nil>"
	}
	return fmt.Sprintf(
		"{name: %s, path: %s, size: %d, slaves: %v, holders: %v, dmInfo: %s}",
		d.name, d.path, d.size, d.slaves, d.holders, dmStr,
	)
}

func (d *Device) IsEqual(other *Device) bool {
	if d == other {
		return true
	}
	if other == nil {
		return false
	}
	if d.name != other.name {
		return false
	}
	if d.path != other.path {
		return false
	}
	if d.major != other.major || d.minor != other.minor {
		return false
	}
	if d.size != other.size {
		return false
	}
	if !collection.EqualValues(d.slaves, other.slaves) {
		return false
	}
	if !collection.EqualValues(d.holders, other.holders) {
		return false
	}
	if !d.dmInfo.IsEqual(other.dmInfo) {
		return false
	}

	return true
}

// DeviceName returns the kernel device name without the /dev/ prefix.
// It does not work with mapper paths.
//
// Example: "/dev/sda" -> "sda".
func DeviceName(input string) string {
	if strings.HasPrefix(input, devicePathPrefix) {
		return input[len(devicePathPrefix):]
	}
	return filepath.Base(input)
}

// DevicePath returns the full device path with the /dev/ prefix.
// It does not work with mapper names.
//
// Example: "sda" -> "/dev/sda"
func DevicePath(input string) string {
	if strings.HasPrefix(input, devicePathPrefix) {
		return input
	}
	return devicePathPrefix + input
}
