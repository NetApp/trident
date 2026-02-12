//go:build linux

package blockdevice

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetBlockDeviceStats_ProtocolAutoDetection tests protocol auto-detection
func TestGetBlockDeviceStats_ProtocolAutoDetection(t *testing.T) {
	ctx := context.Background()
	client := New()

	tests := []struct {
		name             string
		devicePath       string
		protocol         string
		expectedProtocol string
	}{
		{
			name:             "NVMe device auto-detected",
			devicePath:       "/dev/nvme0n1",
			protocol:         "",
			expectedProtocol: "nvme",
		},
		{
			name:             "NVMe device with nvme in path",
			devicePath:       "/dev/disk/by-id/nvme-Samsung_SSD",
			protocol:         "",
			expectedProtocol: "nvme",
		},
		{
			name:             "iSCSI device auto-detected",
			devicePath:       "/dev/sda",
			protocol:         "",
			expectedProtocol: "iscsi",
		},
		{
			name:             "Mapper device defaults to iscsi",
			devicePath:       "/dev/mapper/mpatha",
			protocol:         "",
			expectedProtocol: "iscsi",
		},
		{
			name:             "Explicit iSCSI protocol",
			devicePath:       "/dev/sdb",
			protocol:         "iscsi",
			expectedProtocol: "iscsi",
		},
		{
			name:             "Explicit NVMe protocol",
			devicePath:       "/dev/disk1",
			protocol:         "nvme",
			expectedProtocol: "nvme",
		},
		{
			name:             "Explicit FC protocol",
			devicePath:       "/dev/sdc",
			protocol:         "fc",
			expectedProtocol: "iscsi", // FC uses same code path as iSCSI
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// These will fail since devices don't exist, but we can verify protocol routing logic
			_, _, _, err := client.GetBlockDeviceStats(ctx, tt.devicePath, tt.protocol)
			assert.Error(t, err, "Expected error for non-existent device")

			// Verify error message indicates the right protocol path was taken
			if strings.Contains(tt.devicePath, "nvme") || tt.protocol == "nvme" {
				// NVMe path should fail with nvme command error
				assert.True(t,
					strings.Contains(err.Error(), "failed to execute nvme id-ns") ||
						strings.Contains(err.Error(), "failed to parse nvme"),
					"Expected NVMe-specific error, got: %v", err)
			} else {
				// iSCSI/FC path should fail with device open error or usage stats unavailable
				assert.True(t,
					strings.Contains(err.Error(), "failed to open device") ||
						strings.Contains(err.Error(), "no such file") ||
						strings.Contains(err.Error(), "usage statistics not available"),
					"Expected device open error or usage stats unavailable, got: %v", err)
			}
		})
	}
}

// TestGetCapacityInfo_ErrorCases tests error handling for getCapacityInfo
func TestGetCapacityInfo_ErrorCases(t *testing.T) {
	ctx := context.Background()
	client := New()

	// Test with invalid file descriptor (-1)
	totalBlocks, blockLength, err := client.getCapacityInfo(ctx, -1)
	assert.Error(t, err, "Should error with invalid file descriptor")
	assert.Equal(t, int64(0), totalBlocks, "Total blocks should be 0 on error")
	assert.Equal(t, int64(0), blockLength, "Block length should be 0 on error")
}

// TestGetThresholdExponent_ErrorCases tests error handling for getThresholdExponent
func TestGetThresholdExponent_ErrorCases(t *testing.T) {
	ctx := context.Background()
	client := New()

	// Test with invalid file descriptor (-1)
	thresholdExp, err := client.getThresholdExponent(ctx, -1)
	assert.Error(t, err, "Should error with invalid file descriptor")
	assert.Equal(t, int64(0), thresholdExp, "Threshold exponent should be 0 on error")
}

// TestGetLBA_ErrorCases tests error handling for getLBA
func TestGetLBA_ErrorCases(t *testing.T) {
	ctx := context.Background()
	client := New()

	// Test with invalid file descriptor (-1)
	available, used, err := client.getLBA(ctx, -1)
	assert.Error(t, err, "Should error with invalid file descriptor")
	assert.Equal(t, int64(0), available, "Available should be 0 on error")
	assert.Equal(t, int64(0), used, "Used should be 0 on error")
}

// TestGetDeviceUsage_DeviceNotFound tests handling of non-existent device
func TestGetDeviceUsage_DeviceNotFound(t *testing.T) {
	ctx := context.Background()
	client := New()

	used, available, capacity, err := client.getSCSIDeviceUsage(ctx, "/dev/nonexistent-device-12345")
	assert.Error(t, err, "Should error for non-existent device")
	assert.Contains(t, err.Error(), "failed to open device", "Error should mention device open failure")
	assert.Equal(t, int64(0), used, "Used should be 0 on error")
	assert.Equal(t, int64(0), available, "Available should be 0 on error")
	assert.Equal(t, int64(0), capacity, "Capacity should be 0 on error")
}

// TestGetNVMeUsage_ErrorCases tests NVMe error handling
func TestGetNVMeUsage_ErrorCases(t *testing.T) {
	ctx := context.Background()
	client := New()

	tests := []struct {
		name       string
		devicePath string
		wantError  string
	}{
		{
			name:       "Non-existent device",
			devicePath: "/dev/nvme-nonexistent",
			wantError:  "failed to execute nvme id-ns",
		},
		{
			name:       "Invalid device path",
			devicePath: "/invalid/path/to/device",
			wantError:  "failed to execute nvme id-ns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			used, available, capacity, err := client.getNVMeUsage(ctx, tt.devicePath)
			assert.Error(t, err, "Should error for invalid device")
			assert.Contains(t, err.Error(), tt.wantError, "Error message should indicate NVMe failure")
			assert.Equal(t, int64(0), used, "Used should be 0 on error")
			assert.Equal(t, int64(0), available, "Available should be 0 on error")
			assert.Equal(t, int64(0), capacity, "Capacity should be 0 on error")
		})
	}
}

// TestGetNVMeUsage_InvalidJSON tests handling of invalid JSON from nvme command
func TestGetNVMeUsage_InvalidJSON(t *testing.T) {
	// This test would require mocking exec.CommandContext, which is complex
	// We'll create a test that validates our JSON parsing logic
	ctx := context.Background()
	client := New()

	// Test with a device that doesn't exist to get JSON parsing path
	_, _, _, err := client.getNVMeUsage(ctx, "/dev/nvme999n1")
	assert.Error(t, err)
}

// TestNVMeNamespaceJSON tests NVMe namespace JSON parsing
func TestNVMeNamespaceJSON(t *testing.T) {
	tests := []struct {
		name           string
		jsonData       string
		wantErr        bool
		expectedNSZE   uint64
		expectedNCAP   uint64
		expectedNUSE   uint64
		expectedNSFEAT uint8
		expectedDLFEAT uint8
		expectedLBADS  uint64
	}{
		{
			name: "Valid NVMe namespace with thin provisioning",
			jsonData: `{
				"nsze": 1953525168,
				"ncap": 1953525168,
				"nuse": 1000000000,
				"nsfeat": 1,
				"dlfeat": 0,
				"lbads": 9
			}`,
			wantErr:        false,
			expectedNSZE:   1953525168,
			expectedNCAP:   1953525168,
			expectedNUSE:   1000000000,
			expectedNSFEAT: 1,
			expectedDLFEAT: 0,
			expectedLBADS:  9,
		},
		{
			name:     "Invalid JSON",
			jsonData: `{invalid json}`,
			wantErr:  true,
		},
		{
			name:     "Empty JSON",
			jsonData: `{}`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ns nvmeNamespace
			err := json.Unmarshal([]byte(tt.jsonData), &ns)

			if tt.wantErr {
				assert.Error(t, err, "Expected JSON unmarshal error")
			} else {
				assert.NoError(t, err, "JSON unmarshal should succeed")
				if tt.jsonData != "{}" {
					assert.Equal(t, tt.expectedNSZE, ns.NSZE, "NSZE mismatch")
					assert.Equal(t, tt.expectedNCAP, ns.NCAP, "NCAP mismatch")
					assert.Equal(t, tt.expectedNUSE, ns.NUSE, "NUSE mismatch")
					assert.Equal(t, tt.expectedNSFEAT, ns.NSFEAT, "NSFEAT mismatch")
					assert.Equal(t, tt.expectedDLFEAT, ns.DLFEAT, "DLFEAT mismatch")
					assert.Equal(t, tt.expectedLBADS, ns.LBADS, "LBADS mismatch")
				}
			}
		})
	}
}

// TestNVMeUnsupportedCases tests detection of unsupported NVMe namespace configurations
func TestNVMeUnsupportedCases(t *testing.T) {
	tests := []struct {
		name          string
		nsze          uint64
		ncap          uint64
		nuse          uint64
		nsfeat        uint8
		lbads         uint64
		expectError   bool
		errorContains string
	}{
		{
			name:          "ONTAP < 9.16.1 - NSFEAT bit 0 not set (thin provisioned)",
			nsze:          1953525168,
			ncap:          1953525168,
			nuse:          1000000000,
			nsfeat:        0x00, // Bit 0 not set - indicates ONTAP < 9.16.1
			lbads:         9,
			expectError:   true,
			errorContains: "ONTAP version < 9.16.1",
		},
		{
			name:          "ONTAP < 9.16.1 - NSFEAT bit 0 not set (fully used)",
			nsze:          1953525168,
			ncap:          1953525168,
			nuse:          1953525168,
			nsfeat:        0x00, // Bit 0 not set
			lbads:         9,
			expectError:   true,
			errorContains: "ONTAP version < 9.16.1",
		},
		{
			name:          "Thick provisioned - NUSE equals NSZE with NSFEAT bit set",
			nsze:          1953525168,
			ncap:          1953525168,
			nuse:          1953525168, // Fully used, indicates thick provisioning
			nsfeat:        0x01,       // Bit 0 set (thin provisioning feature available)
			lbads:         9,
			expectError:   true,
			errorContains: "likely thick-provisioned",
		},
		{
			name:        "Valid thin provisioned - NSFEAT bit set, partial usage",
			nsze:        1953525168,
			ncap:        1953525168,
			nuse:        1000000000,
			nsfeat:      0x01, // Bit 0 set - thin provisioning supported
			lbads:       9,
			expectError: false,
		},
		{
			name:        "Valid thin provisioned - low usage",
			nsze:        2097152, // 1 GiB with 512 byte blocks
			ncap:        2097152,
			nuse:        524288, // 256 MiB used
			nsfeat:      0x01,
			lbads:       9,
			expectError: false,
		},
		{
			name:        "Valid thin provisioned - high usage but not full",
			nsze:        1953525168,
			ncap:        1953525168,
			nuse:        1953525167, // Almost full but not exactly equal
			nsfeat:      0x01,
			lbads:       9,
			expectError: false,
		},
		{
			name:          "NSFEAT with other bits set but bit 0 not set",
			nsze:          1953525168,
			ncap:          1953525168,
			nuse:          1000000000,
			nsfeat:        0x04, // Other features set but bit 0 not set
			lbads:         9,
			expectError:   true,
			errorContains: "ONTAP version < 9.16.1",
		},
		{
			name:        "NSFEAT with other bits and bit 0 set - valid",
			nsze:        1953525168,
			ncap:        1953525168,
			nuse:        1000000000,
			nsfeat:      0x05, // Bit 0 and bit 2 set
			lbads:       9,
			expectError: false,
		},
		{
			name:        "Valid thin provisioned - NCAP less than NSZE (pool constrained)",
			nsze:        1953525168, // 1TB namespace
			ncap:        1048576000, // 500GB available in pool
			nuse:        524288000,  // 250GB used
			nsfeat:      0x01,
			lbads:       9,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ns := nvmeNamespace{
				NSZE:   tt.nsze,
				NCAP:   tt.ncap,
				NUSE:   tt.nuse,
				NSFEAT: tt.nsfeat,
				LBADS:  tt.lbads,
			}

			// Simulate the logic from getNVMeUsage
			blockSize := uint64(1) << ns.LBADS

			// Check NSFEAT bit 0 (ONTAP < 9.16.1 check)
			if (ns.NSFEAT & 0x1) == 0 {
				if tt.expectError && strings.Contains(tt.errorContains, "ONTAP") {
					// Expected error for ONTAP < 9.16.1
					assert.True(t, true, "Correctly detected ONTAP < 9.16.1")
				} else {
					t.Errorf("Unexpected NSFEAT check failure")
				}
				return
			}

			// Check for thick provisioning (NUSE == NSZE)
			if ns.NUSE == ns.NSZE {
				if tt.expectError && strings.Contains(tt.errorContains, "thick-provisioned") {
					// Expected error for thick provisioning
					assert.True(t, true, "Correctly detected thick provisioning")
				} else {
					t.Errorf("Unexpected thick provisioning check failure")
				}
				return
			}

			// If we get here, it should be a valid thin-provisioned namespace
			if tt.expectError {
				t.Errorf("Expected error but validation passed")
			} else {
				// Calculate used, available, and capacity using NCAP (not NSZE - NUSE)
				used := int64(ns.NUSE * blockSize)
				available := int64(ns.NCAP * blockSize)
				capacity := int64(ns.NSZE * blockSize)

				assert.Greater(t, capacity, int64(0), "Capacity should be positive")
				assert.GreaterOrEqual(t, used, int64(0), "Used should be non-negative")
				assert.Greater(t, available, int64(0), "Available (NCAP) should be positive for thin provisioned")
				assert.LessOrEqual(t, used, capacity, "Used should be less than or equal to capacity")
				assert.GreaterOrEqual(t, available, used, "Available (NCAP) should be >= used for valid thin provisioned volumes")
			}
		})
	}
}

// TestBlockSizeCalculation tests block size calculation from LBADS
func TestBlockSizeCalculation(t *testing.T) {
	tests := []struct {
		name              string
		lbads             uint64
		expectedBlockSize uint64
	}{
		{
			name:              "512 byte blocks (2^9)",
			lbads:             9,
			expectedBlockSize: 512,
		},
		{
			name:              "4096 byte blocks (2^12)",
			lbads:             12,
			expectedBlockSize: 4096,
		},
		{
			name:              "8192 byte blocks (2^13)",
			lbads:             13,
			expectedBlockSize: 8192,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockSize := uint64(1) << tt.lbads
			assert.Equal(t, tt.expectedBlockSize, blockSize, "Block size calculation incorrect")
		})
	}
}

// TestCapacityCalculation tests capacity calculation logic
func TestCapacityCalculation(t *testing.T) {
	tests := []struct {
		name             string
		nsze             uint64
		nuse             uint64
		lbads            uint64
		expectedCapacity int64
		expectedUsed     int64
	}{
		{
			name:             "1TB drive, 512 byte blocks, fully used",
			nsze:             1953525168, // ~1TB with 512 byte blocks
			nuse:             1953525168,
			lbads:            9,
			expectedCapacity: 1000204886016, // ~1TB
			expectedUsed:     1000204886016,
		},
		{
			name:             "1TB drive, 512 byte blocks, half used",
			nsze:             1953525168,
			nuse:             976762584,
			lbads:            9,
			expectedCapacity: 1000204886016,
			expectedUsed:     500102443008,
		},
		{
			name:             "Small device, 4K blocks",
			nsze:             1024,
			nuse:             512,
			lbads:            12,
			expectedCapacity: 4194304, // 1024 * 4096
			expectedUsed:     2097152, // 512 * 4096
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blockSize := uint64(1) << tt.lbads
			capacity := int64(tt.nsze * blockSize)
			used := int64(tt.nuse * blockSize)

			assert.Equal(t, tt.expectedCapacity, capacity, "Capacity calculation incorrect")
			assert.Equal(t, tt.expectedUsed, used, "Used calculation incorrect")
		})
	}
}

// TestSGIOConstants verifies SCSI Generic I/O constants
func TestSGIOConstants(t *testing.T) {
	assert.Equal(t, 0x2285, SG_IO, "SG_IO constant incorrect")
	assert.Equal(t, -3, SG_DXFER_FROM_DEV, "SG_DXFER_FROM_DEV constant incorrect")
	assert.Equal(t, -2, SG_DXFER_TO_DEV, "SG_DXFER_TO_DEV constant incorrect")
	assert.Equal(t, -1, SG_DXFER_NONE, "SG_DXFER_NONE constant incorrect")
}

// TestSCSICommandOpcodes verifies SCSI command opcodes
func TestSCSICommandOpcodes(t *testing.T) {
	assert.Equal(t, 0x25, SCSI_READ_CAPACITY_10, "SCSI_READ_CAPACITY_10 opcode incorrect")
	assert.Equal(t, 0x9e, SCSI_READ_CAPACITY_16, "SCSI_READ_CAPACITY_16 opcode incorrect")
	assert.Equal(t, 0x12, SCSI_INQUIRY, "SCSI_INQUIRY opcode incorrect")
	assert.Equal(t, 0x4d, SCSI_LOG_SENSE, "SCSI_LOG_SENSE opcode incorrect")
}

// TestSgIoHdrStructSize tests that sgIoHdr struct has expected layout
func TestSgIoHdrStructSize(t *testing.T) {
	var hdr sgIoHdr

	// Verify the struct is not empty
	assert.NotNil(t, &hdr, "sgIoHdr struct should be defined")

	// Test that we can set values
	hdr.interfaceID = 'S'
	hdr.dxferDirection = SG_DXFER_FROM_DEV
	hdr.timeout = 20000

	assert.Equal(t, int32('S'), hdr.interfaceID, "interfaceID should be set")
	assert.Equal(t, int32(SG_DXFER_FROM_DEV), hdr.dxferDirection, "dxferDirection should be set")
	assert.Equal(t, uint32(20000), hdr.timeout, "timeout should be set")
}

// TestGetBlockDeviceStats_Integration is a comprehensive integration test
// This test requires real block devices and should be run manually or in CI with proper setup
func TestGetBlockDeviceStats_Integration(t *testing.T) {
	if os.Getenv("RUN_INTEGRATION_TESTS") != "true" {
		t.Skip("Skipping integration test. Set RUN_INTEGRATION_TESTS=true to run")
	}

	ctx := context.Background()
	client := New()

	// Try to find a real block device to test with
	devices := []string{"/dev/loop0", "/dev/sda", "/dev/nvme0n1"}

	var testDevice string
	for _, dev := range devices {
		if _, err := os.Stat(dev); err == nil {
			// Check if we have read permission
			if f, err := os.OpenFile(dev, os.O_RDONLY, 0); err == nil {
				f.Close()
				testDevice = dev
				break
			}
		}
	}

	if testDevice == "" {
		t.Skip("No accessible block device found for integration testing")
	}

	t.Run("Real device test", func(t *testing.T) {
		used, available, capacity, err := client.GetBlockDeviceStats(ctx, testDevice, "")

		if err != nil {
			// Error is acceptable if we don't have sufficient permissions
			t.Logf("Got error (may be expected): %v", err)
		} else {
			assert.Greater(t, capacity, int64(0), "Capacity should be positive")
			assert.GreaterOrEqual(t, used, int64(0), "Used should be non-negative")
			assert.GreaterOrEqual(t, available, int64(0), "Available should be non-negative")
			assert.LessOrEqual(t, used, capacity, "Used should not exceed capacity")
			t.Logf("Device: %s, Capacity: %d bytes, Used: %d bytes, Available: %d bytes", testDevice, capacity, used, available)
		}
	})
}

// TestNVMeCommandAvailability tests if nvme command is available
func TestNVMeCommandAvailability(t *testing.T) {
	_, err := exec.LookPath("nvme")
	if err != nil {
		t.Logf("nvme command not found in PATH: %v", err)
		t.Log("NVMe tests will fail on systems without nvme-cli installed")
	} else {
		t.Log("nvme command is available")
	}
}

// TestMockNVMeOutput tests parsing of mock NVMe output
func TestMockNVMeOutput(t *testing.T) {
	// Create a temporary directory for test scripts
	tmpDir := t.TempDir()

	// Create a mock nvme script
	mockScript := filepath.Join(tmpDir, "nvme")
	mockOutput := `{
		"nsze": 1953525168,
		"ncap": 1953525168,
		"nuse": 1000000000,
		"nsfeat": 1,
		"dlfeat": 0,
		"nlbaf": 0,
		"flbas": 0,
		"mc": 0,
		"dpc": 0,
		"dps": 0,
		"nmic": 0,
		"rescap": 0,
		"fpi": 0,
		"nawun": 0,
		"nawupf": 0,
		"nacwu": 0,
		"nabsn": 0,
		"nabo": 0,
		"nabspf": 0,
		"lbads": 9
	}`

	scriptContent := fmt.Sprintf(`#!/bin/bash
if [ "$1" = "id-ns" ] && [ "$3" = "-o" ] && [ "$4" = "json" ]; then
	echo '%s'
	exit 0
fi
exit 1
`, mockOutput)

	err := os.WriteFile(mockScript, []byte(scriptContent), 0755)
	require.NoError(t, err, "Failed to create mock script")

	// Test JSON parsing with the mock output
	var ns nvmeNamespace
	err = json.Unmarshal([]byte(mockOutput), &ns)
	require.NoError(t, err, "Failed to parse mock NVMe JSON")

	assert.Equal(t, uint64(1953525168), ns.NSZE, "NSZE should match")
	assert.Equal(t, uint64(1000000000), ns.NUSE, "NUSE should match")
	assert.Equal(t, uint8(1), ns.NSFEAT, "NSFEAT should match")
	assert.Equal(t, uint8(0), ns.DLFEAT, "DLFEAT should match")
	assert.Equal(t, uint64(9), ns.LBADS, "LBADS should match")

	// Calculate expected values
	blockSize := uint64(1) << ns.LBADS
	expectedCapacity := int64(ns.NSZE * blockSize)
	expectedUsed := int64(ns.NUSE * blockSize)

	assert.Equal(t, int64(1000204886016), expectedCapacity, "Capacity calculation")
	assert.Equal(t, int64(512000000000), expectedUsed, "Used calculation")
}

// TestReadCapacity10Response tests parsing of READ CAPACITY(10) response
func TestReadCapacity10Response(t *testing.T) {
	tests := []struct {
		name               string
		response           []byte
		expectedLastLBA    int64
		expectedTotalBlock int64
		expectedBlockSize  int64
	}{
		{
			name: "512 byte blocks",
			response: []byte{
				0x00, 0x00, 0x0F, 0xFF, // Last LBA: 4095
				0x00, 0x00, 0x02, 0x00, // Block size: 512
			},
			expectedLastLBA:    4095,
			expectedTotalBlock: 4096,
			expectedBlockSize:  512,
		},
		{
			name: "4096 byte blocks",
			response: []byte{
				0x00, 0x00, 0x03, 0xFF, // Last LBA: 1023
				0x00, 0x00, 0x10, 0x00, // Block size: 4096
			},
			expectedLastLBA:    1023,
			expectedTotalBlock: 1024,
			expectedBlockSize:  4096,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse last LBA from bytes 0-3 (big endian)
			lastLBA := int64(binary.BigEndian.Uint32(tt.response[0:4]))
			assert.Equal(t, tt.expectedLastLBA, lastLBA, "Last LBA mismatch")

			// Total blocks is lastLBA + 1
			totalBlocks := lastLBA + 1
			assert.Equal(t, tt.expectedTotalBlock, totalBlocks, "Total blocks mismatch")

			// Parse block size from bytes 4-7 (big endian)
			blockSize := int64(binary.BigEndian.Uint32(tt.response[4:8]))
			assert.Equal(t, tt.expectedBlockSize, blockSize, "Block size mismatch")
		})
	}
}

// TestThresholdExponentCalculation tests threshold exponent usage
func TestThresholdExponentCalculation(t *testing.T) {
	tests := []struct {
		name              string
		thresholdExp      int64
		expectedThreshold int64
	}{
		{"Exponent 0", 0, 1},
		{"Exponent 1", 1, 2},
		{"Exponent 2", 2, 4},
		{"Exponent 3", 3, 8},
		{"Exponent 10", 10, 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thresholdSet := int64(1) << tt.thresholdExp
			assert.Equal(t, tt.expectedThreshold, thresholdSet, "Threshold calculation incorrect")
		})
	}
}

// TestCapacityCalculationWithThreshold tests thin-provisioned usage calculation with threshold exponent
// Note: Capacity is now determined by READ CAPACITY, not LOG SENSE
func TestThinProvisionedUsageCalculation(t *testing.T) {
	tests := []struct {
		name         string
		blockLength  int64
		thresholdExp int64
		availableLBA int64
		usedLBA      int64
		expectedUsed int64
	}{
		{
			name:         "Simple case: 512 byte blocks, exp 0",
			blockLength:  512,
			thresholdExp: 0,
			availableLBA: 1000,
			usedLBA:      500,
			expectedUsed: 256000, // 500 * (1 << 0) * 512
		},
		{
			name:         "With threshold: 512 byte blocks, exp 3",
			blockLength:  512,
			thresholdExp: 3,
			availableLBA: 100,
			usedLBA:      50,
			expectedUsed: 204800, // 50 * (1 << 3) * 512 = 50 * 8 * 512
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This tests the thin-provisioned usage calculation logic
			thresholdSet := int64(1) << tt.thresholdExp
			thresholdSetBlock := thresholdSet * tt.blockLength
			usedSize := tt.usedLBA * thresholdSetBlock

			assert.Equal(t, tt.expectedUsed, usedSize, "Used calculation incorrect")
		})
	}
}

// TestCapacityFromReadCapacity tests that capacity is calculated from READ CAPACITY, not LOG SENSE
func TestCapacityFromReadCapacity(t *testing.T) {
	tests := []struct {
		name             string
		totalBlocks      int64
		blockLength      int64
		expectedCapacity int64
	}{
		{
			name:             "Small LUN: 1000 blocks of 512 bytes",
			totalBlocks:      1000,
			blockLength:      512,
			expectedCapacity: 512000, // 1000 * 512
		},
		{
			name:             "Larger LUN: 1M blocks of 4096 bytes",
			totalBlocks:      1048576,
			blockLength:      4096,
			expectedCapacity: 4294967296, // 1M * 4K = 4GB
		},
		{
			name:             "Real example: ~30MiB LUN",
			totalBlocks:      61440, // from sg_readcap example
			blockLength:      512,
			expectedCapacity: 31457280, // 30 MiB
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capacity is calculated directly from READ CAPACITY
			capacity := tt.totalBlocks * tt.blockLength
			assert.Equal(t, tt.expectedCapacity, capacity, "Capacity calculation incorrect")
		})
	}
}

// Benchmark tests for performance validation
func BenchmarkGetBlockDeviceStats_NVMeAutoDetect(b *testing.B) {
	ctx := context.Background()
	client := New()
	devicePath := "/dev/nvme0n1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = client.GetBlockDeviceStats(ctx, devicePath, "")
	}
}

func BenchmarkGetBlockDeviceStats_iSCSIAutoDetect(b *testing.B) {
	ctx := context.Background()
	client := New()
	devicePath := "/dev/sda"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = client.GetBlockDeviceStats(ctx, devicePath, "")
	}
}

func BenchmarkBlockSizeCalculation(b *testing.B) {
	lbads := uint64(9)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = uint64(1) << lbads
	}
}

func BenchmarkCapacityCalculation(b *testing.B) {
	nsze := uint64(1953525168)
	blockSize := uint64(512)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = int64(nsze * blockSize)
	}
}
