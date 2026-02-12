//go:build linux

// Copyright 2025 NetApp, Inc. All Rights Reserved.

// This file should only contain functions for handling block devices for Linux flavor

package blockdevice

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	. "github.com/netapp/trident/logging"
	sa "github.com/netapp/trident/storage_attribute"
	"github.com/netapp/trident/utils/errors"
)

const (
	// SG_IO ioctl constants (from scsi/sg.h)
	SG_IO              = 0x2285 // ioctl command for SCSI Generic I/O
	SG_DXFER_FROM_DEV  = -3     // Data transfer from device
	SG_DXFER_TO_DEV    = -2     // Data transfer to device
	SG_DXFER_NONE      = -1     // No data transfer
	scsiCommandTimeout = 20000  // SCSI command timeout in milliseconds (20 seconds)

	// SCSI command opcodes
	SCSI_READ_CAPACITY_10 = 0x25 // READ CAPACITY(10)
	SCSI_READ_CAPACITY_16 = 0x9e // SERVICE ACTION IN(16) for READ CAPACITY(16)
	SCSI_INQUIRY          = 0x12 // INQUIRY command
	SCSI_LOG_SENSE        = 0x4d // LOG SENSE command

	// SCSI LOG SENSE page 0x0c (Logical Block Provisioning) offsets
	logSenseAvailableLBAOffset = 8  // Offset for Available LBA count (parameter 0x0001)
	logSenseAvailableLBALength = 4  // Length of Available LBA field (4 bytes)
	logSenseUsedLBAOffset      = 20 // Offset for Used LBA count (parameter 0x0002)
	logSenseUsedLBALength      = 4  // Length of Used LBA field (4 bytes)
	logSenseMinResponseLength  = 24 // Minimum response length required to parse both fields
)

// sgIoHdr represents the sg_io_hdr_t structure from scsi/sg.h
type sgIoHdr struct {
	interfaceID    int32   // 'S' for SCSI generic (required)
	dxferDirection int32   // Data transfer direction
	cmdLen         uint8   // SCSI command length
	mxSbLen        uint8   // Max sense buffer length
	iovCount       uint16  // Scatter/gather segments (0 for direct)
	dxferLen       uint32  // Data transfer byte count
	dxferp         uintptr // Data transfer pointer
	cmdp           uintptr // SCSI command buffer pointer
	sbp            uintptr // Sense buffer pointer
	timeout        uint32  // Command timeout in milliseconds
	flags          uint32  // Control flags
	packID         int32   // Packet identifier
	usrPtr         uintptr // User pointer (optional)
	status         uint8   // SCSI status byte
	maskedStatus   uint8   // Shifted, masked SCSI status
	msgStatus      uint8   // Message status
	sbLenWr        uint8   // Bytes written to sense buffer
	hostStatus     uint16  // Host status
	driverStatus   uint16  // Driver status
	resid          int32   // Residual byte count
	duration       uint32  // Command duration (milliseconds)
	info           uint32  // Auxiliary information
}

// nvmeNamespace represents NVMe namespace info structure for JSON parsing
type nvmeNamespace struct {
	NSZE   uint64 `json:"nsze"`   // Namespace Size (total blocks)
	NCAP   uint64 `json:"ncap"`   // Namespace Capacity (available blocks)
	NUSE   uint64 `json:"nuse"`   // Namespace Utilization (used blocks)
	NSFEAT uint8  `json:"nsfeat"` // Namespace Features (bit 0 of this field indicates thick v/s thin)
	DLFEAT uint8  `json:"dlfeat"` // Deallocate Logical Block Features
	LBADS  uint64 `json:"lbads"`  // LBA Data Size (block size in 2^n format)
}

// GetBlockDeviceStats implements the BlockDevice interface for Linux
func (c *Client) GetBlockDeviceStats(ctx context.Context, devicePath, protocol string) (used, available, capacity int64, err error) {
	Logc(ctx).WithFields(LogFields{
		"devicePath": devicePath,
		"protocol":   protocol,
	}).Debug(">>>> blockdevice.GetBlockDeviceStats")
	defer Logc(ctx).Debug("<<<< blockdevice.GetBlockDeviceStats")

	// Auto-detect protocol if not specified
	if protocol == "" {
		if strings.Contains(devicePath, sa.NVMe) {
			protocol = sa.NVMe
		} else {
			protocol = sa.ISCSI // Default to iSCSI for other devices (includes FC)
		}
	}

	// Route to appropriate method based on protocol
	if protocol == sa.NVMe {
		return c.getNVMeUsage(ctx, devicePath)
	}

	// For iSCSI and FC, use SG_IO ioctl
	return c.getSCSIDeviceUsage(ctx, devicePath)
}

// getSCSIDeviceUsage retrieves block device statistics using SG_IO ioctl
// This works for iSCSI and FC (Fibre Channel) devices
func (c *Client) getSCSIDeviceUsage(ctx context.Context, devicePath string) (used, available, capacity int64, err error) {
	Logc(ctx).WithField("devicePath", devicePath).Debug(">>>> blockdevice.getSCSIDeviceUsage")
	defer Logc(ctx).Debug("<<<< blockdevice.getSCSIDeviceUsage")

	// Open the device
	// Note: O_RDWR is required for SG_IO ioctl, even for read-only SCSI commands
	device, err := os.OpenFile(devicePath, os.O_RDWR, 0)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to open device %s: %w", devicePath, err)
	}
	defer device.Close()

	fd := int(device.Fd())

	// Get total capacity and block length using READ CAPACITY(10)
	// This returns the LUN's advertised size (what the host sees)
	totalBlocks, blockLength, err := c.getCapacityInfo(ctx, fd)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get capacity info: %w", err)
	}

	// Calculate total capacity from READ CAPACITY
	capacity = totalBlocks * blockLength

	// Try to get thin-provisioned volume statistics from LOG SENSE
	// This is optional and only works for thin-provisioned LUNs
	var availableFromLogSense int64
	used, availableFromLogSense, err = c.getThinProvisionedUsage(ctx, fd, blockLength)
	if err != nil {
		// If LOG SENSE fails, the volume is either thick-provisioned or doesn't support usage stats
		// Return a specific error to indicate usage statistics are not available for this volume
		// This allows callers (like autogrow logic) to skip this volume rather than incorrectly
		// assuming it's 100% full
		return 0, 0, 0, errors.UsageStatsUnavailableError(
			"usage statistics not available for device %s (likely thick-provisioned or unsupported): %v",
			devicePath, err)
	}

	// We have two ways to calculate available space:
	// 1. From LOG SENSE page 0x0c (availableLBA) - authoritative value from device
	// 2. Calculated as capacity - used (derived from READ CAPACITY and LOG SENSE usedLBA)
	// Use the LOG SENSE available value as it's the authoritative source from the device.
	available = availableFromLogSense

	Logc(ctx).WithFields(LogFields{
		"capacity":    capacity,
		"used":        used,
		"available":   available,
		"blockLength": blockLength,
		"totalBlocks": totalBlocks,
	}).Debug("SCSI device usage retrieved via SG_IO")

	return used, available, capacity, nil
}

// sgIo executes an SG_IO ioctl command
func (c *Client) sgIo(_ context.Context, fd int, cmd []byte, response []byte) error {
	senseBuf := make([]byte, 32)
	hdr := sgIoHdr{
		interfaceID:    'S',
		dxferDirection: SG_DXFER_FROM_DEV,
		cmdLen:         uint8(len(cmd)),
		mxSbLen:        uint8(len(senseBuf)),
		dxferLen:       uint32(len(response)),
		dxferp:         uintptr(unsafe.Pointer(&response[0])), //nolint:gosec // Required for syscall ioctl
		cmdp:           uintptr(unsafe.Pointer(&cmd[0])),      //nolint:gosec // Required for syscall ioctl
		sbp:            uintptr(unsafe.Pointer(&senseBuf[0])), //nolint:gosec // Required for syscall ioctl
		timeout:        scsiCommandTimeout,
	}

	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(SG_IO),
		uintptr(unsafe.Pointer(&hdr)), //nolint:gosec // Required for syscall ioctl
	)

	if errno != 0 {
		return fmt.Errorf("ioctl SG_IO failed: %w", errno)
	}

	if hdr.status != 0 {
		return fmt.Errorf("SCSI command failed with status: 0x%02x, host_status: 0x%02x, driver_status: 0x%02x",
			hdr.status, hdr.hostStatus, hdr.driverStatus)
	}

	return nil
}

// getCapacityInfo gets total blocks and block length using READ CAPACITY(10) command
// Returns: totalBlocks (number of blocks), blockLength (bytes per block)
func (c *Client) getCapacityInfo(ctx context.Context, fd int) (totalBlocks, blockLength int64, err error) {
	cmd := []byte{SCSI_READ_CAPACITY_10, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	response := make([]byte, 8)

	err = c.sgIo(ctx, fd, cmd, response)
	if err != nil {
		return 0, 0, fmt.Errorf("READ CAPACITY(10) failed: %w", err)
	}

	// Last LBA is in bytes 0-3 (big endian) - this is (total blocks - 1)
	lastLBA := int64(binary.BigEndian.Uint32(response[0:4]))
	totalBlocks = lastLBA + 1

	// Block length is in bytes 4-7 (big endian)
	blockLength = int64(binary.BigEndian.Uint32(response[4:8]))

	return totalBlocks, blockLength, nil
}

// getThresholdExponent gets threshold exponent from VPD page 0xb2
func (c *Client) getThresholdExponent(ctx context.Context, fd int) (int64, error) {
	cmd := []byte{SCSI_INQUIRY, 1, 0xb2, 0, 8, 0} // INQUIRY VPD page 0xb2
	response := make([]byte, 8)

	err := c.sgIo(ctx, fd, cmd, response)
	if err != nil {
		return 0, fmt.Errorf("INQUIRY VPD 0xb2 failed: %w", err)
	}

	if len(response) < 5 {
		return 0, fmt.Errorf("insufficient data in VPD page 0xb2")
	}

	// Threshold exponent is the 5th byte (index 4)
	thresholdExp := int64(response[4])
	return thresholdExp, nil
}

// getThinProvisionedUsage gets used and available space for thin-provisioned LUNs using LOG SENSE page 0x0c
// This is optional and only works for thin-provisioned volumes that support this log page.
// Returns both used and available space as reported by the device's thin-provisioning log.
func (c *Client) getThinProvisionedUsage(ctx context.Context, fd int, blockLength int64) (used, available int64, err error) {
	// Get threshold exponent from VPD page 0xb2
	thresholdExp, err := c.getThresholdExponent(ctx, fd)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get threshold exponent: %w", err)
	}

	// Get LBA counts from LOG SENSE page 0x0c
	availableLBA, usedLBA, err := c.getLBA(ctx, fd)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get LBA counts: %w", err)
	}

	// Calculate used and available sizes
	// For thin-provisioned volumes, usedLBA/availableLBA represent blocks allocated/available in the thin pool
	thresholdSet := int64(1) << thresholdExp
	thresholdSetBlock := thresholdSet * blockLength
	usedSize := usedLBA * thresholdSetBlock
	availableSize := availableLBA * thresholdSetBlock

	Logc(ctx).WithFields(LogFields{
		"thresholdExp":  thresholdExp,
		"usedLBA":       usedLBA,
		"availableLBA":  availableLBA,
		"usedSize":      usedSize,
		"availableSize": availableSize,
	}).Debug("Thin-provisioned SCSI usage retrieved")

	return usedSize, availableSize, nil
}

// getLBA gets LBA counts from LOG SENSE page 0x0c (Logical Block Provisioning log)
func (c *Client) getLBA(ctx context.Context, fd int) (available, used int64, err error) {
	cmd := []byte{SCSI_LOG_SENSE, 0, 0x4c, 0, 0, 0, 0, 0, 64, 0} // LOG SENSE page 0x0c
	response := make([]byte, 64)

	err = c.sgIo(ctx, fd, cmd, response)
	if err != nil {
		return 0, 0, fmt.Errorf("LOG SENSE page 0x0c failed: %w", err)
	}

	// Parse the log page data
	// Available LBA count is at offset 8-11 (parameter 0x0001)
	// Used LBA count is at offset 20-23 (parameter 0x0002)
	if len(response) >= logSenseMinResponseLength {
		available = int64(binary.BigEndian.Uint32(response[logSenseAvailableLBAOffset : logSenseAvailableLBAOffset+logSenseAvailableLBALength]))
		used = int64(binary.BigEndian.Uint32(response[logSenseUsedLBAOffset : logSenseUsedLBAOffset+logSenseUsedLBALength]))
	} else {
		return 0, 0, fmt.Errorf("insufficient data in LOG SENSE page 0x0c")
	}

	return available, used, nil
}

// getNVMeUsage retrieves NVMe namespace statistics using nvme-cli
func (c *Client) getNVMeUsage(ctx context.Context, devicePath string) (used, available, capacity int64, err error) {
	Logc(ctx).WithField("devicePath", devicePath).Debug(">>>> blockdevice.getNVMeUsage")
	defer Logc(ctx).Debug("<<<< blockdevice.getNVMeUsage")

	// Execute: nvme id-ns <device> -o json
	cmd := exec.CommandContext(ctx, sa.NVMe, "id-ns", devicePath, "-o", "json") //nolint:gosec
	output, err := cmd.CombinedOutput()
	if err != nil {
		// CombinedOutput includes stderr, which contains useful error messages from nvme-cli
		Logc(ctx).WithFields(LogFields{
			"devicePath": devicePath,
			"output":     string(output),
		}).WithError(err).Error("Failed to execute nvme id-ns command.")
		return 0, 0, 0, fmt.Errorf("failed to execute nvme id-ns for %s: %w; output: %s", devicePath, err, string(output))
	}

	// Parse JSON output
	var ns nvmeNamespace
	if err := json.Unmarshal(output, &ns); err != nil {
		Logc(ctx).WithFields(LogFields{
			"devicePath": devicePath,
			"output":     string(output),
		}).WithError(err).Error("Failed to parse nvme id-ns JSON output.")
		return 0, 0, 0, fmt.Errorf("failed to parse nvme id-ns output for %s: %w", devicePath, err)
	}

	// If the NSFEAT bit is set to 0, it indicates unsupported case of ONTAP < 9.16.1
	// This bit is set to 0 for ONTAP < 9.16.1 for both thick and thin provisioned namespaces
	if (ns.NSFEAT & 0x1) == 0 {
		return 0, 0, 0, errors.UsageStatsUnavailableError(
			"usage statistics not available for NVMe device %s (ONTAP version < 9.16.1)", devicePath)
	}

	// If the NSFEAT bit is set to 1 (thin provisioning supported), but NUSE equals NSZE,
	// it indicates a thick-provisioned namespace
	// TODO (aparna0508): Verify if the below check is best way to check for unsupported cases
	if ns.NUSE == ns.NSZE {
		return 0, 0, 0, errors.UsageStatsUnavailableError(
			"usage statistics not available for NVMe device %s (likely thick-provisioned or unsupported)", devicePath)
	}

	// Calculate block size from LBADS (LBA Data Size is 2^LBADS)
	blockSize := uint64(1) << ns.LBADS

	// Calculate used, available, and capacity in bytes
	// For thin-provisioned namespaces:
	// - NUSE: Currently used/allocated blocks
	// - NCAP: Maximum allocatable blocks (represents available pool capacity)
	// - NSZE: Total namespace size
	// Using NCAP gives us the true available space in the thin pool
	used = int64(ns.NUSE * blockSize)
	available = int64(ns.NCAP * blockSize)
	capacity = int64(ns.NSZE * blockSize)

	Logc(ctx).WithFields(LogFields{
		"NSZE":      ns.NSZE,
		"NCAP":      ns.NCAP,
		"NUSE":      ns.NUSE,
		"NSFEAT":    fmt.Sprintf("%#x", ns.NSFEAT),
		"DLFEAT":    fmt.Sprintf("%#x", ns.DLFEAT),
		"LBADS":     ns.LBADS,
		"blockSize": blockSize,
		"used":      used,
		"available": available,
		"capacity":  capacity,
	}).Debug("NVMe namespace usage retrieved")

	return used, available, capacity, nil
}
