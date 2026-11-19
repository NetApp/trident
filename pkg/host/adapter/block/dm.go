// Copyright 2026 NetApp, Inc. All Rights Reserved.

package block

import (
	"fmt"
	"strings"
)

const (
	MapperPathPrefix = "/dev/mapper/"

	dmTargetPrefixMpath = "mpath-" // set by multipathd.
	dmTargetPrefixCrypt = "crypt-" // set by cryptsetup.
)

// DMTarget identifies the device-mapper target type, derived from the dm UUID prefix.
type DMTarget string

const (
	DMTargetUnknown DMTarget = ""      // Unknown or unrecognized DM target
	DMTargetMpath   DMTarget = "mpath" // Multipath target (UUID prefix "mpath-")
	DMTargetCrypt   DMTarget = "crypt" // Crypt target (UUID prefix "CRYPT-")
)

// DMDevice holds device-mapper specific information about a block device.
// DM devices are not backed by specific protocols like SCSI or NVMe; instead,
// they're virtual block devices managed by the kernel.
//
// This data is populated from /sys/block/dm-*/dm/ which is an aspect of block
// devices /sys/block/ (not a symlink), and therefore part of the block layer.
type DMDevice struct {
	uuid string // uuid is the ID of the DM device (/sys/block/dm-#/dm/uuid).
	name string // name is the assigned name of the DM device (/sys/block/dm-#/dm/name).
}

func (d *DMDevice) UUID() string   { return d.uuid }
func (d *DMDevice) Name() string   { return d.name }
func (d *DMDevice) String() string { return fmt.Sprintf("{uuid: %s, name: %s}", d.uuid, d.name) }

// Target returns the device-mapper target type, derived from the UUID prefix.
func (d *DMDevice) Target() DMTarget {
	switch {
	case d.IsMpath():
		return DMTargetMpath
	case d.IsCrypt():
		return DMTargetCrypt
	default:
		return DMTargetUnknown
	}
}

func (d *DMDevice) IsMpath() bool {
	return strings.HasPrefix(strings.ToLower(d.uuid), dmTargetPrefixMpath)
}
func (d *DMDevice) IsCrypt() bool {
	return strings.HasPrefix(strings.ToLower(d.uuid), dmTargetPrefixCrypt)
}

func (d *DMDevice) IsEqual(other *DMDevice) bool {
	// Are these the same pointer?
	if d == other {
		return true
	}
	// Is one of these nil?
	if d == nil || other == nil {
		return false
	}
	// Is the content of the two equivalent?
	return *d == *other
}

// MapperName strips the /dev/mapper/ prefix, returning just the mapper name.
// It does not work with non-mapper device paths.
//
// Example: "/dev/mapper/luks-pvc_123" -> "luks-pvc_123"
func MapperName(input string) string {
	if strings.HasPrefix(input, MapperPathPrefix) {
		return strings.TrimPrefix(input, MapperPathPrefix)
	}
	return input
}

// MapperPath returns the full /dev/mapper/ path for a mapper name.
// It does not work with non-mapper device names.
//
// Example: "luks-pvc_123" -> "/dev/mapper/luks-pvc_123"
func MapperPath(input string) string {
	if strings.HasPrefix(input, MapperPathPrefix) {
		return input
	}
	return MapperPathPrefix + input
}
