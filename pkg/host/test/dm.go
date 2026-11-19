// Copyright 2026 NetApp, Inc. All Rights Reserved.

package test

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/afero"
)

// The device-mapper tree in /sys/block is cross-cutting substrate: the block
// adapter parses it, the multipath and cryptsetup adapters mutate it. To keep
// the adapter fakes free of sibling imports (no block<->multipath coupling),
// the shared dm-tree write/remove primitives live here on the FakeHost's
// substrate, and every adapter fake reaches them through the test package it
// already depends on.
//
// These constants mirror the real block adapter's sysfs layout (kernel ABI:
// /sys/block/dm-N/{dev,size,dm/{uuid,name},slaves,holders}, /dev/dm-N). The
// block fake stages via WriteDMNode and then reads back through the REAL block
// parser, so any drift between these paths and the parser surfaces as a loud
// failure in block's own fake tests — that composed read is the drift guard
// that replaces reusing block's unexported path constructors here.
const (
	dmBlockPath    = "/sys/block"
	dmDevicePrefix = "dm-"
	dmDevPrefix    = "/dev/"
	dmMapperPrefix = "/dev/mapper/"
	dmDefaultMajor = 253
)

func dmDevicePath(name string) string     { return filepath.Join(dmBlockPath, name) }
func dmAttrPath(name, attr string) string { return filepath.Join(dmDevicePath(name), "dm", attr) }
func dmSlavesPath(name string) string     { return filepath.Join(dmDevicePath(name), "slaves") }
func dmHoldersPath(name string) string    { return filepath.Join(dmDevicePath(name), "holders") }
func dmGlobPath(attr string) string {
	return filepath.Join(dmBlockPath, dmDevicePrefix+"*", "dm", attr)
}
func dmDevNode(name string) string { return dmDevPrefix + name }

// AllocDMMinor returns the lowest-free dm minor under /sys/block, reproducing
// the kernel's minor-reuse behavior so staged maps get deterministic dm-N names.
func AllocDMMinor(fs afero.Fs) int {
	used := map[int]bool{}
	entries, _ := afero.ReadDir(fs, dmBlockPath)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), dmDevicePrefix) {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimPrefix(e.Name(), dmDevicePrefix)); err == nil {
			used[n] = true
		}
	}
	for i := 0; ; i++ {
		if !used[i] {
			return i
		}
	}
}

// WriteDMNode seeds a device-mapper node over the named slaves: it allocates the
// lowest-free dm minor, writes dev/size/dm{uuid,name}, wires the slave<->holder
// edges, and drops the /dev byte node. It returns the kernel name ("dm-N").
//
// This is the single dm-tree write primitive shared by the adapter fakes, so
// none of them import another to stage a map.
func WriteDMNode(fs afero.Fs, mapperName, uuid string, slaves ...string) string {
	minor := AllocDMMinor(fs)
	name := fmt.Sprintf("%s%d", dmDevicePrefix, minor)

	_ = dmWriteFile(fs, filepath.Join(dmDevicePath(name), "dev"),
		[]byte(fmt.Sprintf("%d:%d\n", dmDefaultMajor, minor)))
	_ = dmWriteFile(fs, filepath.Join(dmDevicePath(name), "size"), []byte("0\n"))
	_ = dmWriteFile(fs, dmAttrPath(name, "uuid"), []byte(uuid+"\n"))
	_ = dmWriteFile(fs, dmAttrPath(name, "name"), []byte(mapperName+"\n"))
	_ = fs.MkdirAll(dmSlavesPath(name), 0o755)
	_ = fs.MkdirAll(dmHoldersPath(name), 0o755)

	for _, s := range slaves {
		// The map is a slave-of-many and a holder of each slave.
		_ = dmWriteFile(fs, filepath.Join(dmSlavesPath(name), s), nil)
		_ = dmWriteFile(fs, filepath.Join(dmHoldersPath(s), name), nil)
	}
	_ = dmWriteFile(fs, dmDevNode(name), nil)
	return name
}

// DMExistsByUUID reports whether a dm node with the given dm/uuid is present.
func DMExistsByUUID(fs afero.Fs, uuid string) bool {
	_, ok := dmNameByAttr(fs, "uuid", uuid)
	return ok
}

// DMSlaveCountByRef returns the slave (path) count of the dm node referenced by
// ref and whether such a node exists. A present node with zero slaves is a
// "ghost" map: all its paths are gone, so `multipath -f` cannot flush it and the
// caller must fall back to a UUID-addressed removal.
func DMSlaveCountByRef(fs afero.Fs, ref string) (count int, found bool) {
	name, ok := resolveDMRef(fs, ref)
	if !ok {
		return 0, false
	}
	entries, _ := afero.ReadDir(fs, dmSlavesPath(name))
	return len(entries), true
}

// DMHolderCountByRef returns the holder count of the dm node referenced by ref
// and whether such a node exists. A map with one or more holders is still open
// (a stacked layer or a mount holds it), so `multipath -f` reports "map in use"
// and the caller must retry until the holder is released.
func DMHolderCountByRef(fs afero.Fs, ref string) (count int, found bool) {
	name, ok := resolveDMRef(fs, ref)
	if !ok {
		return 0, false
	}
	entries, _ := afero.ReadDir(fs, dmHoldersPath(name))
	return len(entries), true
}

// AddDMHolder records that holderName is stacked on top of the dm node
// referenced by ref — the sysfs edge the kernel creates when a consumer (an
// opened dm-crypt layer, a mounted filesystem, an open fd) holds the map. A map
// with a holder is open and cannot be flushed by `multipath -f`. Returns true if
// the referenced node was found.
func AddDMHolder(fs afero.Fs, ref, holderName string) bool {
	name, ok := resolveDMRef(fs, ref)
	if !ok {
		return false
	}
	_ = dmWriteFile(fs, filepath.Join(dmHoldersPath(name), holderName), nil)
	return true
}

// RemoveDMByUUID removes the dm node whose dm/uuid matches, detaching its holder
// edges and dropping its /dev node. Returns true if a node was removed.
func RemoveDMByUUID(fs afero.Fs, uuid string) bool {
	name, ok := dmNameByAttr(fs, "uuid", uuid)
	if !ok {
		return false
	}
	removeDM(fs, name)
	return true
}

// RemoveDMByRef removes the dm node referenced by ref — a kernel name ("dm-N"),
// a mapper name (matched against dm/name), or their /dev forms — mirroring the
// teardown `multipath -f` and `dmsetup remove <name>` perform on /sys/block.
// Returns true if a node was removed.
func RemoveDMByRef(fs afero.Fs, ref string) bool {
	name, ok := resolveDMRef(fs, ref)
	if !ok {
		return false
	}
	removeDM(fs, name)
	return true
}

// DMSetupRemoveHandler is the fake `dmsetup` exec handler, serving both forms
// used by the real adapters:
//
//   - `dmsetup remove -u <uuid>` (block.RemoveByDMUUID): the kernel atomically
//     rejects an unknown uuid with a non-zero exit — NOT a DeviceNotFoundError.
//   - `dmsetup remove <name|path>` (multipath.RemoveDevice): an absent device
//     prints "device does not exist", which the caller treats as already-gone.
//
// Every fake that issues dmsetup (block, multipath, cryptsetup) registers this
// one handler, so composing them on a shared host is order-independent — the
// logic has a single home and each registration installs the same function.
func DMSetupRemoveHandler(_ context.Context, fs afero.Fs, _ string, args []string) ([]byte, error) {
	if len(args) == 0 || args[0] != "remove" {
		return nil, &ExitError{Code: 1, Msg: "dmsetup: unsupported invocation"}
	}

	if uuid := dmFlagValue(args, "-u"); uuid != "" {
		if !RemoveDMByUUID(fs, uuid) {
			return nil, &ExitError{
				Code: 1,
				Msg:  fmt.Sprintf("device-mapper: remove ioctl on uuid %s failed: No such device", uuid),
			}
		}
		return nil, nil
	}

	ref := args[len(args)-1]
	if ref == "" || ref == "remove" {
		return nil, &ExitError{Code: 1, Msg: "dmsetup remove: device name required"}
	}
	if RemoveDMByRef(fs, ref) {
		return nil, nil
	}
	return []byte("device does not exist"), &ExitError{Code: 1, Msg: "dmsetup remove failed"}
}

// dmNameByAttr returns the kernel dm name whose dm/<attr> file equals value.
func dmNameByAttr(fs afero.Fs, attr, value string) (string, bool) {
	matches, _ := afero.Glob(fs, dmGlobPath(attr))
	for _, p := range matches {
		data, err := afero.ReadFile(fs, p)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(data)) == value {
			// p = /sys/block/<dm-N>/dm/<attr> -> dm-N
			return filepath.Base(filepath.Dir(filepath.Dir(p))), true
		}
	}
	return "", false
}

// resolveDMRef maps a device-mapper reference to a kernel dm name. It accepts a
// kernel name ("dm-N"), a mapper name (matched against dm/name), or their /dev
// forms ("/dev/dm-N", "/dev/mapper/<name>").
func resolveDMRef(fs afero.Fs, ref string) (string, bool) {
	name := ref
	switch {
	case strings.HasPrefix(ref, dmMapperPrefix):
		name = strings.TrimPrefix(ref, dmMapperPrefix)
	case strings.HasPrefix(ref, dmDevPrefix):
		name = strings.TrimPrefix(ref, dmDevPrefix)
	}
	if strings.HasPrefix(name, dmDevicePrefix) {
		if ok, _ := afero.DirExists(fs, dmDevicePath(name)); ok {
			return name, true
		}
	}
	return dmNameByAttr(fs, "name", name)
}

func removeDM(fs afero.Fs, name string) {
	_ = fs.RemoveAll(dmDevicePath(name))
	// Detach holder edges: drop <slave>/holders/<name> wherever present.
	entries, _ := afero.ReadDir(fs, dmBlockPath)
	for _, e := range entries {
		_ = fs.Remove(filepath.Join(dmHoldersPath(e.Name()), name))
	}
	_ = fs.Remove(dmDevNode(name))
}

func dmWriteFile(fs afero.Fs, path string, content []byte) error {
	if err := fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return afero.WriteFile(fs, path, content, 0o644)
}

func dmFlagValue(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}
