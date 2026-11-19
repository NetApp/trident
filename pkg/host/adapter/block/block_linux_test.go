// Copyright 2026 NetApp, Inc. All Rights Reserved.

package block

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	tridentexec "github.com/netapp/trident/utils/exec"
)

type fakeExitError struct {
	code int
	msg  string
}

func (e *fakeExitError) Error() string { return e.msg }
func (e *fakeExitError) ExitCode() int { return e.code }

func newTestAdapter(t *testing.T) (Adapter, afero.Fs) {
	t.Helper()
	fs := afero.NewMemMapFs()
	a := New(WithFs(fs))
	return a, fs
}

// writeFile creates parent directories and writes content to path in the given fs.
func writeFile(t *testing.T, fs afero.Fs, path, content string) {
	t.Helper()
	require.NoError(t, fs.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, afero.WriteFile(fs, path, []byte(content), 0o644))
}

// mkSCSIDevice creates a minimal sysfs tree for a SCSI block device.
func mkSCSIDevice(t *testing.T, fs afero.Fs, name string, sizeInSectors uint64) {
	t.Helper()
	base := filepath.Join(sysfsBlockPath, name)
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "slaves"), 0o755))
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "holders"), 0o755))
	if sizeInSectors > 0 {
		writeFile(t, fs, filepath.Join(base, "size"), uintToStr(sizeInSectors))
	}
}

// mkNVMeDevice creates a minimal sysfs tree for an NVMe block device.
func mkNVMeDevice(t *testing.T, fs afero.Fs, name string, sizeInSectors uint64) {
	t.Helper()
	base := filepath.Join(sysfsBlockPath, name)
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "slaves"), 0o755))
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "holders"), 0o755))
	if sizeInSectors > 0 {
		writeFile(t, fs, filepath.Join(base, "size"), uintToStr(sizeInSectors))
	}
}

// mkDMDevice creates a minimal sysfs tree for a device-mapper block device.
func mkDMDevice(t *testing.T, fs afero.Fs, name, dmUUID, dmName string, slaves []string) {
	t.Helper()
	base := filepath.Join(sysfsBlockPath, name)
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "slaves"), 0o755))
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "holders"), 0o755))
	writeFile(t, fs, filepath.Join(base, "size"), "0")
	writeFile(t, fs, filepath.Join(base, "dm", "uuid"), dmUUID)
	writeFile(t, fs, filepath.Join(base, "dm", "name"), dmName)
	for _, s := range slaves {
		require.NoError(t, fs.MkdirAll(filepath.Join(base, "slaves", s), 0o755))
	}
}

// addHolder adds a holder entry for deviceName.
func addHolder(t *testing.T, fs afero.Fs, deviceName, holderName string) {
	t.Helper()
	require.NoError(t, fs.MkdirAll(
		filepath.Join(sysfsBlockPath, deviceName, "holders", holderName), 0o755,
	))
}

func uintToStr(n uint64) string {
	return strconv.FormatUint(n, 10) //nolint:gomnd
}

func TestDevice_SCSIDevice(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkSCSIDevice(t, fs, "sda", 2048)

	dev, err := a.DeviceByName(context.Background(), "sda")
	require.NoError(t, err)
	assert.Equal(t, "sda", dev.Name())
	assert.Equal(t, "/dev/sda", dev.Path())
	assert.Equal(t, uint64(2048*512), dev.Size())
	assert.Nil(t, dev.Slaves())
	assert.Nil(t, dev.Holders())

	assert.Nil(t, dev.DMInfo())
}

func TestDevice_NVMeDevice(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkNVMeDevice(t, fs, "nvme0n1", 4096)

	dev, err := a.DeviceByName(context.Background(), "nvme0n1")
	require.NoError(t, err)
	assert.Equal(t, "nvme0n1", dev.Name())
	assert.Equal(t, "/dev/nvme0n1", dev.Path())
	assert.Equal(t, uint64(4096*512), dev.Size())
}

func TestDevice_DMDeviceMpath(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkDMDevice(t, fs, "dm-0", "mpath-360060160abcd1234", "mpathA", []string{"sda", "sdb"})

	dev, err := a.DeviceByName(context.Background(), "dm-0")
	require.NoError(t, err)
	assert.Equal(t, "dm-0", dev.Name())
	assert.Equal(t, "/dev/dm-0", dev.Path())
	assert.ElementsMatch(t, []string{"sda", "sdb"}, dev.Slaves())

	dm := dev.DMInfo()
	require.NotNil(t, dm)
	assert.Equal(t, "mpath-360060160abcd1234", dm.UUID())
	assert.Equal(t, "mpathA", dm.Name())
	assert.True(t, dm.IsMpath())
	assert.False(t, dm.IsCrypt())
	assert.Equal(t, DMTargetMpath, dm.Target())
}

func TestDevice_DMDeviceCrypt(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkDMDevice(t, fs, "dm-1", "CRYPT-LUKS2-abc123-luks_vol", "luks_vol", []string{"dm-0"})

	dev, err := a.DeviceByName(context.Background(), "dm-1")
	require.NoError(t, err)

	dm := dev.DMInfo()
	require.NotNil(t, dm)
	assert.True(t, dm.IsCrypt())
	assert.False(t, dm.IsMpath())
	assert.Equal(t, DMTargetCrypt, dm.Target())
}

func TestDevice_DMDeviceUnknownTarget(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkDMDevice(t, fs, "dm-2", "LVM-somevolumegroup", "lv_root", nil)

	dev, err := a.DeviceByName(context.Background(), "dm-2")
	require.NoError(t, err)

	dm := dev.DMInfo()
	require.NotNil(t, dm)
	assert.Equal(t, DMTargetUnknown, dm.Target())
	assert.False(t, dm.IsMpath())
	assert.False(t, dm.IsCrypt())
}

func TestDevice_NotFound(t *testing.T) {
	a, _ := newTestAdapter(t)

	dev, err := a.DeviceByName(context.Background(), "sda")
	require.Error(t, err)
	assert.Nil(t, dev)
	assert.True(t, IsDeviceNotFoundError(err))
}

func TestDevice_NoSizeFile(t *testing.T) {
	a, fs := newTestAdapter(t)
	// Create device directory without a size file.
	base := filepath.Join(sysfsBlockPath, "sda")
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "slaves"), 0o755))
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "holders"), 0o755))

	dev, err := a.DeviceByName(context.Background(), "sda")
	require.NoError(t, err)
	assert.Equal(t, uint64(0), dev.Size())
}

func TestDevice_WithHolders(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkSCSIDevice(t, fs, "sda", 1024)
	addHolder(t, fs, "sda", "dm-0")
	addHolder(t, fs, "sda", "dm-1")

	dev, err := a.DeviceByName(context.Background(), "sda")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"dm-0", "dm-1"}, dev.Holders())
}

func TestDevice_ChrootPrefix(t *testing.T) {
	fs := afero.NewMemMapFs()
	a := New(WithFs(fs), WithChrootPrefix("/host"))

	base := filepath.Join("/host", sysfsBlockPath, "sda")
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "slaves"), 0o755))
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "holders"), 0o755))
	writeFile(t, fs, filepath.Join(base, "size"), "1024")

	dev, err := a.DeviceByName(context.Background(), "sda")
	require.NoError(t, err)
	assert.Equal(t, "sda", dev.Name())
	assert.Equal(t, uint64(1024*512), dev.Size())
}

func TestSlaves_WithSlaves(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkDMDevice(t, fs, "dm-0", "mpath-123", "mpathA", []string{"sdb", "sda"})

	slaves, err := a.Slaves(context.Background(), "dm-0")
	require.NoError(t, err)
	// readDirNames sorts the result.
	assert.Equal(t, []string{"sda", "sdb"}, slaves)
}

func TestSlaves_EmptyDir(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkSCSIDevice(t, fs, "sda", 1024)

	slaves, err := a.Slaves(context.Background(), "sda")
	require.NoError(t, err)
	assert.Empty(t, slaves)
}

func TestSlaves_NoDir(t *testing.T) {
	a, fs := newTestAdapter(t)
	// Create device dir but no slaves subdirectory.
	require.NoError(t, fs.MkdirAll(filepath.Join(sysfsBlockPath, "sda"), 0o755))

	slaves, err := a.Slaves(context.Background(), "sda")
	require.Error(t, err)
	assert.Nil(t, slaves)
	assert.ErrorIs(t, err, errDoesNotExist)
}

func TestHolders_WithHolders(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkSCSIDevice(t, fs, "sda", 1024)
	addHolder(t, fs, "sda", "dm-1")
	addHolder(t, fs, "sda", "dm-0")

	holders, err := a.Holders(context.Background(), "sda")
	require.NoError(t, err)
	assert.Equal(t, []string{"dm-0", "dm-1"}, holders)
}

func TestHolders_EmptyDir(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkSCSIDevice(t, fs, "sda", 1024)

	holders, err := a.Holders(context.Background(), "sda")
	require.NoError(t, err)
	assert.Empty(t, holders)
}

func TestHolders_NoDir(t *testing.T) {
	a, fs := newTestAdapter(t)
	require.NoError(t, fs.MkdirAll(filepath.Join(sysfsBlockPath, "sda"), 0o755))

	holders, err := a.Holders(context.Background(), "sda")
	require.Error(t, err)
	assert.Nil(t, holders)
	assert.ErrorIs(t, err, errDoesNotExist)
}

func TestDevice_SlavesDefensiveCopy(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkDMDevice(t, fs, "dm-0", "mpath-123", "mpathA", []string{"sda", "sdb"})

	dev, err := a.DeviceByName(context.Background(), "dm-0")
	require.NoError(t, err)

	// Get slaves and mutate the returned slice.
	s1 := dev.Slaves()
	s1[0] = "MUTATED"

	// Second call should return original values.
	s2 := dev.Slaves()
	assert.Equal(t, []string{"sda", "sdb"}, s2)
}

func TestDevice_HoldersDefensiveCopy(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkSCSIDevice(t, fs, "sda", 1024)
	addHolder(t, fs, "sda", "dm-0")

	dev, err := a.DeviceByName(context.Background(), "sda")
	require.NoError(t, err)

	h1 := dev.Holders()
	h1[0] = "MUTATED"

	h2 := dev.Holders()
	assert.Equal(t, []string{"dm-0"}, h2)
}

func TestDevice_SlavesReturnsNilWhenEmpty(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkSCSIDevice(t, fs, "sda", 1024)

	dev, err := a.DeviceByName(context.Background(), "sda")
	require.NoError(t, err)
	assert.Nil(t, dev.Slaves(), "Slaves() should return nil, not empty slice")
}

func TestDevice_HoldersReturnsNilWhenEmpty(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkSCSIDevice(t, fs, "sda", 1024)

	dev, err := a.DeviceByName(context.Background(), "sda")
	require.NoError(t, err)
	assert.Nil(t, dev.Holders(), "Holders() should return nil, not empty slice")
}

// --- DMDevice tests ---

func TestDMDevice_IsMpath(t *testing.T) {
	tests := []struct {
		name     string
		uuid     string
		expected bool
	}{
		{"lowercase mpath prefix", "mpath-360060160abcd", true},
		{"uppercase MPATH prefix", "MPATH-360060160abcd", true},
		{"mixed case Mpath prefix", "Mpath-360060160abcd", true},
		{"crypt prefix", "CRYPT-LUKS2-abc", false},
		{"lvm prefix", "LVM-vgdata", false},
		{"empty uuid", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm := &DMDevice{uuid: tt.uuid}
			assert.Equal(t, tt.expected, dm.IsMpath())
		})
	}
}

func TestDMDevice_IsCrypt(t *testing.T) {
	tests := []struct {
		name     string
		uuid     string
		expected bool
	}{
		{"uppercase CRYPT prefix", "CRYPT-LUKS2-abc123-vol", true},
		{"lowercase crypt prefix", "crypt-luks2-abc123-vol", true},
		{"mixed case Crypt prefix", "Crypt-LUKS2-abc123-vol", true},
		{"mpath prefix", "mpath-360060160abcd", false},
		{"lvm prefix", "LVM-vgdata", false},
		{"empty uuid", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm := &DMDevice{uuid: tt.uuid}
			assert.Equal(t, tt.expected, dm.IsCrypt())
		})
	}
}

func TestDMDevice_Target(t *testing.T) {
	tests := []struct {
		name     string
		uuid     string
		expected DMTarget
	}{
		{"mpath", "mpath-360060160abcd", DMTargetMpath},
		{"crypt", "CRYPT-LUKS2-abc123", DMTargetCrypt},
		{"unknown", "LVM-vgdata", DMTargetUnknown},
		{"empty", "", DMTargetUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm := &DMDevice{uuid: tt.uuid}
			assert.Equal(t, tt.expected, dm.Target())
		})
	}
}

func TestDMDevice_String(t *testing.T) {
	dm := &DMDevice{uuid: "mpath-123", name: "mpathA"}
	s := dm.String()
	assert.Contains(t, s, "mpath-123")
	assert.Contains(t, s, "mpathA")
}

func TestDevice_IsMpath(t *testing.T) {
	tests := []struct {
		name     string
		dmInfo   *DMDevice
		expected bool
	}{
		{"mpath device", &DMDevice{uuid: "mpath-360060160abcd"}, true},
		{"crypt device", &DMDevice{uuid: "CRYPT-LUKS2-abc"}, false},
		{"nil dmInfo", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dev := &Device{dmInfo: tt.dmInfo}
			assert.Equal(t, tt.expected, dev.IsMpath())
		})
	}
}

func TestDevice_IsCrypt(t *testing.T) {
	tests := []struct {
		name     string
		dmInfo   *DMDevice
		expected bool
	}{
		{"crypt device", &DMDevice{uuid: "CRYPT-LUKS2-abc"}, true},
		{"mpath device", &DMDevice{uuid: "mpath-360060160abcd"}, false},
		{"nil dmInfo", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dev := &Device{dmInfo: tt.dmInfo}
			assert.Equal(t, tt.expected, dev.IsCrypt())
		})
	}
}

func TestDevice_String_WithDMInfo(t *testing.T) {
	dev := &Device{
		name:   "dm-0",
		path:   "/dev/dm-0",
		size:   1024 * 512,
		slaves: []string{"sda"},
		dmInfo: &DMDevice{uuid: "mpath-123", name: "mpathA"},
	}
	s := dev.String()
	assert.Contains(t, s, "dm-0")
	assert.Contains(t, s, "mpath-123")
	assert.Contains(t, s, "mpathA")
}

func TestDevice_String_NilDMInfo(t *testing.T) {
	dev := &Device{
		name: "sda",
		path: "/dev/sda",
		size: 2048 * 512,
	}
	// Should not panic when dmInfo is nil.
	s := dev.String()
	assert.Contains(t, s, "sda")
	assert.Contains(t, s, "<nil>")
}

func TestDevice_BadSizeContent(t *testing.T) {
	a, fs := newTestAdapter(t)
	base := filepath.Join(sysfsBlockPath, "sda")
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "slaves"), 0o755))
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "holders"), 0o755))
	// Write non-numeric content.
	writeFile(t, fs, filepath.Join(base, "size"), "not-a-number")

	dev, err := a.DeviceByName(context.Background(), "sda")
	require.NoError(t, err)
	// Bad size content should result in 0 bytes.
	assert.Equal(t, uint64(0), dev.Size())
}

func TestDevice_DMDevice_MissingUUID(t *testing.T) {
	a, fs := newTestAdapter(t)
	base := filepath.Join(sysfsBlockPath, "dm-0")
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "slaves"), 0o755))
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "holders"), 0o755))
	writeFile(t, fs, filepath.Join(base, "size"), "1024")
	// Create dm/name but NOT dm/uuid.
	writeFile(t, fs, filepath.Join(base, "dm", "name"), "mpathA")

	dev, err := a.DeviceByName(context.Background(), "dm-0")
	// uuid file missing → ReadFile returns ("", nil) → empty UUID is set.
	require.NoError(t, err)
	dm := dev.DMInfo()
	require.NotNil(t, dm)
	assert.Equal(t, "", dm.UUID())
	assert.Equal(t, "mpathA", dm.Name())
}

func TestDeviceForDMUUID_MatchesMpathDevice(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkDMDevice(t, fs, "dm-0", "mpath-360060160abcd1234", "mpathA", []string{"sda", "sdb"})
	mkSCSIDevice(t, fs, "sda", 1024)
	mkSCSIDevice(t, fs, "sdb", 1024)

	dev, err := a.DeviceByDMUUID(context.Background(), "mpath-360060160abcd1234")
	require.NoError(t, err)
	assert.Equal(t, "dm-0", dev.Name())
	assert.Equal(t, "/dev/dm-0", dev.Path())

	dm := dev.DMInfo()
	require.NotNil(t, dm)
	assert.Equal(t, "mpath-360060160abcd1234", dm.UUID())
	assert.Equal(t, "mpathA", dm.Name())
}

func TestDeviceForDMUUID_MatchesCryptDevice(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkDMDevice(t, fs, "dm-1", "CRYPT-LUKS2-abc123-luks_vol", "luks_vol", []string{"dm-0"})
	mkDMDevice(t, fs, "dm-0", "mpath-360060160abcd1234", "mpathA", []string{"sda"})

	dev, err := a.DeviceByDMUUID(context.Background(), "CRYPT-LUKS2-abc123-luks_vol")
	require.NoError(t, err)
	assert.Equal(t, "dm-1", dev.Name())

	dm := dev.DMInfo()
	require.NotNil(t, dm)
	assert.True(t, dm.IsCrypt())
}

func TestDeviceForDMUUID_NotFound(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkDMDevice(t, fs, "dm-0", "mpath-111", "mpathA", nil)

	dev, err := a.DeviceByDMUUID(context.Background(), "mpath-999")
	assert.Nil(t, dev)
	require.Error(t, err)
	assert.True(t, IsDeviceNotFoundError(err))
}

func TestDeviceForDMUUID_NoDMDevicesExist(t *testing.T) {
	a, fs := newTestAdapter(t)
	// Only non-dm devices in /sys/block.
	mkSCSIDevice(t, fs, "sda", 1024)
	mkNVMeDevice(t, fs, "nvme0n1", 4096)

	dev, err := a.DeviceByDMUUID(context.Background(), "mpath-360060160abcd1234")
	assert.Nil(t, dev)
	require.Error(t, err)
	assert.True(t, IsDeviceNotFoundError(err))
}

func TestDeviceForDMUUID_EmptyBlockDir(t *testing.T) {
	a, fs := newTestAdapter(t)
	require.NoError(t, fs.MkdirAll(sysfsBlockPath, 0o755))

	dev, err := a.DeviceByDMUUID(context.Background(), "mpath-123")
	assert.Nil(t, dev)
	require.Error(t, err)
	assert.True(t, IsDeviceNotFoundError(err))
}

func TestDeviceForDMUUID_MultipleDevicesReturnsCorrectOne(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkDMDevice(t, fs, "dm-0", "mpath-AAA", "mpathA", nil)
	mkDMDevice(t, fs, "dm-1", "CRYPT-LUKS2-BBB-vol", "vol", nil)
	mkDMDevice(t, fs, "dm-2", "mpath-CCC", "mpathC", nil)

	dev, err := a.DeviceByDMUUID(context.Background(), "mpath-CCC")
	require.NoError(t, err)
	assert.Equal(t, "dm-2", dev.Name())

	dm := dev.DMInfo()
	require.NotNil(t, dm)
	assert.Equal(t, "mpath-CCC", dm.UUID())
}

func TestDeviceForDMUUID_SkipsUnreadableUUID(t *testing.T) {
	a, fs := newTestAdapter(t)
	// dm-0: missing dm/uuid file entirely — should be skipped.
	base := filepath.Join(sysfsBlockPath, "dm-0")
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "slaves"), 0o755))
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "holders"), 0o755))
	writeFile(t, fs, filepath.Join(base, "size"), "0")
	writeFile(t, fs, filepath.Join(base, "dm", "name"), "broken")
	// No dm/uuid file for dm-0.

	// dm-1: has a valid UUID.
	mkDMDevice(t, fs, "dm-1", "mpath-GOOD", "mpathGood", nil)

	dev, err := a.DeviceByDMUUID(context.Background(), "mpath-GOOD")
	require.NoError(t, err)
	assert.Equal(t, "dm-1", dev.Name())
}

func TestDeviceForDMUUID_WithChrootPrefix(t *testing.T) {
	fs := afero.NewMemMapFs()
	a := New(WithFs(fs), WithChrootPrefix("/host"))

	base := filepath.Join("/host", sysfsBlockPath, "dm-0")
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "slaves"), 0o755))
	require.NoError(t, fs.MkdirAll(filepath.Join(base, "holders"), 0o755))
	writeFile(t, fs, filepath.Join(base, "size"), "0")
	writeFile(t, fs, filepath.Join(base, "dm", "uuid"), "mpath-CHROOTED")
	writeFile(t, fs, filepath.Join(base, "dm", "name"), "mpathX")

	dev, err := a.DeviceByDMUUID(context.Background(), "mpath-CHROOTED")
	require.NoError(t, err)
	assert.Equal(t, "dm-0", dev.Name())

	dm := dev.DMInfo()
	require.NotNil(t, dm)
	assert.Equal(t, "mpath-CHROOTED", dm.UUID())
	assert.Equal(t, "mpathX", dm.Name())
}

func TestDeviceByDMName_Match(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkDMDevice(t, fs, "dm-0", "mpath-AAA", "mpathA", nil)
	mkDMDevice(t, fs, "dm-1", "mpath-BBB", "mpathB", nil)

	dev, err := a.DeviceByDMName(context.Background(), "mpathB")
	require.NoError(t, err)
	assert.Equal(t, "dm-1", dev.Name())

	dm := dev.DMInfo()
	require.NotNil(t, dm)
	assert.Equal(t, "mpathB", dm.Name())
	assert.Equal(t, "mpath-BBB", dm.UUID())
}

func TestDeviceByDMName_NotFound(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkDMDevice(t, fs, "dm-0", "mpath-AAA", "mpathA", nil)

	dev, err := a.DeviceByDMName(context.Background(), "noSuchName")
	assert.Nil(t, dev)
	require.Error(t, err)
	assert.True(t, IsDeviceNotFoundError(err))
}

func TestDeviceByDMName_NoDMDevices(t *testing.T) {
	a, fs := newTestAdapter(t)
	mkSCSIDevice(t, fs, "sda", 1024)

	dev, err := a.DeviceByDMName(context.Background(), "mpathA")
	assert.Nil(t, dev)
	require.Error(t, err)
	assert.True(t, IsDeviceNotFoundError(err))
}

func TestFlush_IsDeviceFlushError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mc := mockexec.NewMockCommand(ctrl)
	mc.EXPECT().ExecuteWithTimeout(
		gomock.Any(), "blockdev", execBlkDevTimeout, true, "--flushbufs", "/dev/sda",
	).Return([]byte(""), errors.New("ioctl failed"))

	ad := &adapter{cmd: mc}
	err := ad.Flush(context.Background(), "/dev/sda")
	require.Error(t, err)
	assert.True(t, IsDeviceFlushError(err))
}

func TestFlushBuffers_ExitCode(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)

	cmd.EXPECT().ExecuteWithTimeout(
		gomock.Any(), "blockdev", execBlkDevTimeout, true, "--flushbufs", "/dev/sda",
	).Return([]byte("ioctl failed"), &fakeExitError{code: 1, msg: "exit status 1"})

	ad := &adapter{cmd: cmd}
	err := ad.Flush(context.Background(), "/dev/sda")
	assert.Error(t, err)

	var execErr tridentexec.ExitError
	if assert.ErrorAs(t, err, &execErr) {
		assert.Equal(t, 1, execErr.ExitCode())
	}
}

func TestFlush(t *testing.T) {
	cases := map[string]struct {
		devicePath   string
		mockErr      error
		wantContains string
		expectExec   bool
		assertErr    assert.ErrorAssertionFunc
	}{
		"success": {
			devicePath: "/dev/sda",
			expectExec: true,
			assertErr:  assert.NoError,
		},
		"empty_device_path": {
			devicePath:   "",
			wantContains: "device path is required",
			expectExec:   false,
			assertErr:    assert.Error,
		},
		"command_error": {
			devicePath: "/dev/sda",
			mockErr:    errors.New("ioctl failed"),
			expectExec: true,
			assertErr:  assert.Error,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mc := mockexec.NewMockCommand(ctrl)

			if tc.expectExec {
				mc.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "blockdev", execBlkDevTimeout, true, "--flushbufs", tc.devicePath,
				).Return([]byte(""), tc.mockErr)
			}

			ad := &adapter{cmd: mc}
			err := ad.Flush(context.Background(), tc.devicePath)
			tc.assertErr(t, err)
			if tc.wantContains != "" {
				assert.Contains(t, err.Error(), tc.wantContains)
			}
		})
	}
}

func TestRead_ExitCode(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)

	cmd.EXPECT().ExecuteWithTimeout(
		gomock.Any(), "dd", execDDTimeout, gomock.Any(), gomock.Any(),
	).Return([]byte("dd: /dev/sda: I/O error"), &fakeExitError{code: 1, msg: "exit status 1"})

	ad := &adapter{cmd: cmd}
	out, err := ad.Read(context.Background(), "/dev/sda", DataOptions{BlockSize: 4096, BlockCount: 512})
	assert.Error(t, err)
	assert.Nil(t, out)

	var execErr tridentexec.ExitError
	if assert.ErrorAs(t, err, &execErr) {
		assert.Equal(t, 1, execErr.ExitCode())
	}
}

func TestCopy(t *testing.T) {
	cases := map[string]struct {
		inputPath    string
		outputPath   string
		opts         DataOptions
		mockOut      []byte
		mockErr      error
		wantOut      []byte
		wantContains string
		expectExec   bool
		assertErr    assert.ErrorAssertionFunc
	}{
		"success": {
			inputPath:  "/dev/sda",
			outputPath: "/dev/sdb",
			opts:       DataOptions{BlockSize: 4096, BlockCount: 512},
			mockOut:    []byte("copied"),
			wantOut:    []byte("copied"),
			expectExec: true,
			assertErr:  assert.NoError,
		},
		"empty_input_path": {
			inputPath:    "",
			outputPath:   "/dev/sdb",
			opts:         DataOptions{BlockSize: 4096, BlockCount: 512},
			wantContains: "input path is required",
			assertErr:    assert.Error,
		},
		"empty_output_path": {
			inputPath:    "/dev/sda",
			outputPath:   "",
			opts:         DataOptions{BlockSize: 4096, BlockCount: 512},
			wantContains: "output path is required",
			assertErr:    assert.Error,
		},
		"invalid_block_size": {
			inputPath:    "/dev/sda",
			outputPath:   "/dev/sdb",
			opts:         DataOptions{BlockSize: 0, BlockCount: 512},
			wantContains: "block size must be positive",
			assertErr:    assert.Error,
		},
		"command_error": {
			inputPath:  "/dev/sda",
			outputPath: "/dev/sdb",
			opts:       DataOptions{BlockSize: 4096, BlockCount: 512},
			mockErr:    errors.New("dd failed"),
			expectExec: true,
			assertErr:  assert.Error,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mc := mockexec.NewMockCommand(ctrl)

			if tc.expectExec {
				mc.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "dd", gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(tc.mockOut, tc.mockErr)
			}

			ad := &adapter{cmd: mc}
			out, err := ad.Copy(context.Background(), tc.inputPath, tc.outputPath, tc.opts)
			tc.assertErr(t, err)
			if tc.wantContains != "" {
				assert.Contains(t, err.Error(), tc.wantContains)
			}
			if err == nil {
				assert.Equal(t, tc.wantOut, out)
			}
		})
	}
}

func TestCopy_ExitCode(t *testing.T) {
	ctrl := gomock.NewController(t)
	cmd := mockexec.NewMockCommand(ctrl)

	cmd.EXPECT().ExecuteWithTimeout(
		gomock.Any(), "dd", execDDTimeout, gomock.Any(), gomock.Any(),
	).Return([]byte("dd: error writing"), &fakeExitError{code: 1, msg: "exit status 1"})

	ad := &adapter{cmd: cmd}
	out, err := ad.Copy(context.Background(), "/dev/sda", "/dev/sdb", DataOptions{BlockSize: 4096, BlockCount: 512})
	assert.Error(t, err)
	assert.Nil(t, out)

	var execErr tridentexec.ExitError
	if assert.ErrorAs(t, err, &execErr) {
		assert.Equal(t, 1, execErr.ExitCode())
	}
}

func TestValidateDataOptions(t *testing.T) {
	ad := &adapter{}
	cases := map[string]struct {
		opts         DataOptions
		wantContains string
		assertErr    assert.ErrorAssertionFunc
	}{
		"valid": {
			opts:      DataOptions{BlockSize: 4096, BlockCount: 512},
			assertErr: assert.NoError,
		},
		"zero_block_size": {
			opts:         DataOptions{BlockSize: 0, BlockCount: 1},
			wantContains: "block size must be positive",
			assertErr:    assert.Error,
		},
		"negative_block_size": {
			opts:         DataOptions{BlockSize: -1, BlockCount: 1},
			wantContains: "block size must be positive",
			assertErr:    assert.Error,
		},
		"zero_block_count": {
			opts:         DataOptions{BlockSize: 4096, BlockCount: 0},
			wantContains: "block count must be positive",
			assertErr:    assert.Error,
		},
		"negative_block_skip": {
			opts:         DataOptions{BlockSize: 4096, BlockCount: 1, BlockSkip: -1},
			wantContains: "block skip must be non-negative",
			assertErr:    assert.Error,
		},
		"zero_skip_ok": {
			opts:      DataOptions{BlockSize: 4096, BlockCount: 1, BlockSkip: 0},
			assertErr: assert.NoError,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := ad.validateDataOptions(tc.opts)
			tc.assertErr(t, err)
			if tc.wantContains != "" {
				assert.Contains(t, err.Error(), tc.wantContains)
			}
		})
	}
}

func TestZero_Validation(t *testing.T) {
	cases := map[string]struct {
		devicePath   string
		opts         DataOptions
		wantContains string
		assertErr    assert.ErrorAssertionFunc
	}{
		"empty_device_path": {
			devicePath:   "",
			opts:         DataOptions{BlockSize: 4096, BlockCount: 512, Force: true},
			wantContains: "device path is required",
			assertErr:    assert.Error,
		},
		"force_false": {
			devicePath:   "/dev/sdb",
			opts:         DataOptions{BlockSize: 4096, BlockCount: 512, Force: false},
			wantContains: "zero operation requires Force to be true",
			assertErr:    assert.Error,
		},
		"zero_block_size": {
			devicePath:   "/dev/sdb",
			opts:         DataOptions{BlockSize: 0, BlockCount: 512, Force: true},
			wantContains: "block size must be positive",
			assertErr:    assert.Error,
		},
		"zero_block_count": {
			devicePath:   "/dev/sdb",
			opts:         DataOptions{BlockSize: 4096, BlockCount: 0, Force: true},
			wantContains: "block count must be positive",
			assertErr:    assert.Error,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mc := mockexec.NewMockCommand(ctrl)
			ad := &adapter{cmd: mc}
			err := ad.Zero(context.Background(), tc.devicePath, tc.opts)
			tc.assertErr(t, err)
			if tc.wantContains != "" {
				assert.Contains(t, err.Error(), tc.wantContains)
			}
		})
	}
}

func TestRead(t *testing.T) {
	cases := map[string]struct {
		devicePath string
		opts       DataOptions
		mockOut    []byte
		mockErr    error
		wantOut    []byte
		expectExec bool
		assertErr  assert.ErrorAssertionFunc
	}{
		"success": {
			devicePath: "/dev/sda",
			opts:       DataOptions{BlockSize: 4096, BlockCount: 512},
			mockOut:    []byte("raw-data"),
			mockErr:    nil,
			wantOut:    []byte("raw-data"),
			expectExec: true,
			assertErr:  assert.NoError,
		},
		"command_error": {
			devicePath: "/dev/sda",
			opts:       DataOptions{BlockSize: 4096, BlockCount: 512},
			mockOut:    []byte(""),
			mockErr:    errors.New("dd failed"),
			wantOut:    nil,
			expectExec: true,
			assertErr:  assert.Error,
		},
		"empty_device_path": {
			devicePath: "",
			opts:       DataOptions{BlockSize: 4096, BlockCount: 512},
			expectExec: false,
			assertErr:  assert.Error,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mc := mockexec.NewMockCommand(ctrl)

			if tc.expectExec {
				mc.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "dd", gomock.Any(), gomock.Any(), gomock.Any(),
				).Return(tc.mockOut, tc.mockErr)
			}

			ad := &adapter{cmd: mc}
			out, err := ad.Read(context.Background(), tc.devicePath, tc.opts)
			tc.assertErr(t, err)
			if err == nil {
				assert.Equal(t, tc.wantOut, out)
			}
		})
	}
}

func TestZero(t *testing.T) {
	cases := map[string]struct {
		devicePath string
		opts       DataOptions
		mockErr    error
		expectExec bool
		assertErr  assert.ErrorAssertionFunc
	}{
		"success": {
			devicePath: "/dev/sdb",
			opts:       DataOptions{BlockSize: 4096, BlockCount: 512, Force: true},
			mockErr:    nil,
			expectExec: true,
			assertErr:  assert.NoError,
		},
		"command_error": {
			devicePath: "/dev/sdb",
			opts:       DataOptions{BlockSize: 4096, BlockCount: 512, Force: true},
			mockErr:    errors.New("dd failed"),
			expectExec: true,
			assertErr:  assert.Error,
		},
		"force_false_rejected": {
			devicePath: "/dev/sdb",
			opts:       DataOptions{BlockSize: 4096, BlockCount: 512, Force: false},
			expectExec: false,
			assertErr:  assert.Error,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mc := mockexec.NewMockCommand(ctrl)

			if tc.expectExec {
				mc.EXPECT().ExecuteWithTimeout(
					gomock.Any(), "dd", gomock.Any(), gomock.Any(), gomock.Any(),
				).DoAndReturn(func(_ context.Context, _ string, _ time.Duration, _ bool, args ...string) ([]byte, error) {
					assert.Contains(t, args, "if=/dev/zero")
					assert.Contains(t, args, "of="+tc.devicePath)
					return []byte(""), tc.mockErr
				})
			}

			ad := &adapter{cmd: mc}
			err := ad.Zero(context.Background(), tc.devicePath, tc.opts)
			tc.assertErr(t, err)
		})
	}
}

func TestDDArgConstruction(t *testing.T) {
	cases := map[string]struct {
		inputPath       string
		outputPath      string
		opts            DataOptions
		wantContains    []string
		wantNotContains []string
	}{
		"read_no_output": {
			inputPath:       "/dev/sda",
			outputPath:      "",
			opts:            DataOptions{BlockSize: 4096, BlockCount: 512},
			wantContains:    []string{"if=/dev/sda", "bs=4096", "count=512", "status=none"},
			wantNotContains: []string{"of="},
		},
		"with_output_file": {
			inputPath:    "/dev/sda",
			outputPath:   "/dev/sdb",
			opts:         DataOptions{BlockSize: 4096, BlockCount: 512},
			wantContains: []string{"if=/dev/sda", "of=/dev/sdb", "bs=4096", "count=512"},
		},
		"with_skip": {
			inputPath:    "/dev/sda",
			outputPath:   "",
			opts:         DataOptions{BlockSize: 4096, BlockCount: 512, BlockSkip: 10},
			wantContains: []string{"if=/dev/sda", "skip=10"},
		},
		"custom_block_size": {
			inputPath:    "/dev/sda",
			outputPath:   "",
			opts:         DataOptions{BlockSize: 512, BlockCount: 512},
			wantContains: []string{"bs=512"},
		},
		"zero_skip_omitted": {
			inputPath:       "/dev/sda",
			outputPath:      "",
			opts:            DataOptions{BlockSize: 4096, BlockCount: 512},
			wantNotContains: []string{"skip="},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mc := mockexec.NewMockCommand(ctrl)

			mc.EXPECT().ExecuteWithTimeout(
				gomock.Any(), "dd", gomock.Any(), gomock.Any(), gomock.Any(),
			).DoAndReturn(func(_ context.Context, _ string, _ time.Duration, _ bool, args ...string) ([]byte, error) {
				for _, want := range tc.wantContains {
					assert.Contains(t, args, want, "expected arg %s in %v", want, args)
				}
				for _, notWant := range tc.wantNotContains {
					for _, arg := range args {
						assert.NotContains(t, arg, notWant, "unexpected arg prefix %s in %v", notWant, args)
					}
				}
				return []byte(""), nil
			})

			ad := &adapter{cmd: mc}
			_, _ = ad.dd(context.Background(), tc.inputPath, tc.outputPath, tc.opts)
		})
	}
}

func TestDDTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	mc := mockexec.NewMockCommand(ctrl)

	mc.EXPECT().ExecuteWithTimeout(
		gomock.Any(), "dd", execDDTimeout, gomock.Any(), gomock.Any(),
	).DoAndReturn(func(_ context.Context, _ string, timeout time.Duration, _ bool, args ...string) ([]byte, error) {
		assert.Equal(t, execDDTimeout, timeout)
		return []byte(""), nil
	})

	opts := DataOptions{
		BlockSize:  4096,
		BlockCount: 1,
	}
	ad := &adapter{cmd: mc}
	_, _ = ad.dd(context.Background(), "/dev/sda", "", opts)
}

func TestDDLogOutput(t *testing.T) {
	cases := map[string]struct {
		logOutput bool
	}{
		"enabled":  {logOutput: true},
		"disabled": {logOutput: false},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mc := mockexec.NewMockCommand(ctrl)

			mc.EXPECT().ExecuteWithTimeout(
				gomock.Any(), "dd", execDDTimeout, tc.logOutput, gomock.Any(),
			).Return([]byte(""), nil)

			opts := DataOptions{
				BlockSize:  4096,
				BlockCount: 1,
				LogOutput:  tc.logOutput,
			}
			ad := &adapter{cmd: mc}
			_, _ = ad.dd(context.Background(), "/dev/sda", "", opts)
		})
	}
}
