// Copyright 2026 NetApp, Inc. All Rights Reserved.

package block

const (
	// Block dimensions callers use when probing the start of a device.
	ProbeBlockSize       = 4096
	EmptyCheckBlockCount = 512
	EmptyCheckSize       = ProbeBlockSize * EmptyCheckBlockCount

	// Filesystem type strings used by Format, Repair, and Type.
	Ext3 = "ext3"
	Ext4 = "ext4"
	Xfs  = "xfs"

	// blkid output modes.
	OutputDefault OutputMode = iota
	OutputExport
	OutputValue
	OutputList

	// blkid tag names.
	TagType Tag = "TYPE"
)

// DataOptions configures raw block I/O operations (Copy, Read, Zero).
// BlockSize and BlockCount must be positive; validation rejects zero values.
type DataOptions struct {
	BlockSize  int64 // block size in bytes; must be > 0
	BlockCount int64 // number of blocks to transfer; must be > 0
	BlockSkip  int64 // input blocks to skip before reading; must be >= 0
	LogOutput  bool  // whether to log command output
	Force      bool  // only checked by Zero; must be true to confirm destructive write
}

// EraseOptions describe how wipefs should be invoked.
type EraseOptions struct {
	Types      []string // optional comma list for --types
	Force      bool     // --force
	BackupPath string   // non-empty => --backup[=dir]
	Quiet      bool     // -q
	NoAct      bool     // --no-act (dry run)
	LogOutput  bool
}

func DefaultEraseOptions() EraseOptions {
	return EraseOptions{}
}

// Tag is a blkid attribute key (e.g. "TYPE", "UUID").
type Tag string

// Tags is a map of blkid attribute keys to their values.
type Tags map[Tag]string

// OutputMode controls the blkid output format.
type OutputMode int

// ProbeOptions configures a blkid probe call.
type ProbeOptions struct {
	Tags   []Tag      // requested blkid tags.
	Cache  bool       // for fresh scans where the blkid cache might be stale.
	Probe  bool       // for raw devices, or when you want to inspect content without altering the cache.
	Output OutputMode // in most cases OutputDefault or OutputExport; OutputList is unsupported.
}
