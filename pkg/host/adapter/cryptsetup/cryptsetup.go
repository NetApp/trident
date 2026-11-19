// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package cryptsetup provides a platform-agnostic adapter for
// cryptsetup operations: format, open, close, passphrase rotation.
// Platform-specific mechanisms are implemented in _<GOOS>.go files
// behind the Adapter interface.
package cryptsetup

import (
	"context"

	tridentexec "github.com/netapp/trident/utils/exec"
)

const (
	// LUKS format options.
	luksCipherMode      = "aes-xts-plain64"
	luksType            = "luks2"
	luksHash            = "sha256"
	luksPBKDF           = "pbkdf2"
	luksPBKDFIterations = "1000"
)

type FormatOptions struct {
	Type            string // default "luks2"
	Cipher          string // default "aes-xts-plain64"
	Hash            string // default "sha256"
	PBKDF           string // default "pbkdf2"
	PBKDFIterations string // default "1000"
}

func (f FormatOptions) GoString() string {
	return "FormatOptions{Type: " + f.Type +
		", Cipher: " + f.Cipher +
		", Hash: " + f.Hash +
		", PBKDF: " + f.PBKDF +
		", PBKDFIterations: " + f.PBKDFIterations + "}"
}

func (f FormatOptions) String() string {
	return f.GoString()
}

func (f FormatOptions) WithType(luksType string) FormatOptions {
	f.Type = luksType
	return f
}

func (f FormatOptions) WithCipher(cipher string) FormatOptions {
	f.Cipher = cipher
	return f
}

func (f FormatOptions) WithHash(hash string) FormatOptions {
	f.Hash = hash
	return f
}

func (f FormatOptions) WithPBKDF(pbkdf string) FormatOptions {
	f.PBKDF = pbkdf
	return f
}

func (f FormatOptions) WithPBKDFIterations(iterations string) FormatOptions {
	f.PBKDFIterations = iterations
	return f
}

func DefaultFormatOptions() FormatOptions {
	return FormatOptions{
		Type:            luksType,
		Cipher:          luksCipherMode,
		Hash:            luksHash,
		PBKDF:           luksPBKDF,
		PBKDFIterations: luksPBKDFIterations,
	}
}

type OpenOptions struct {
	Type          string
	AllowDiscards bool
}

func (o OpenOptions) WithType(luksType string) OpenOptions {
	o.Type = luksType
	return o
}

func (o OpenOptions) WithAllowDiscards(allowDiscards bool) OpenOptions {
	o.AllowDiscards = allowDiscards
	return o
}

func DefaultOpenOptions() OpenOptions {
	return OpenOptions{
		Type:          luksType,
		AllowDiscards: true,
	}
}

type Adapter interface {
	// Version returns the cryptsetup version string.  If cryptsetup is not
	// installed the call fails, so this doubles as an availability check.
	Version(ctx context.Context) (string, error)

	// Format initializes LUKS headers on a raw block device.
	// Expects a full raw device path (e.g., "/dev/dm-3", "/dev/sda", "/dev/nvme0n1").
	// Passphrase is provided via stdin.
	Format(ctx context.Context, devicePath string, passphrase string, opts FormatOptions) (string, error)

	// Open creates an active LUKS mapping for a raw block device.
	// Expects a full raw device path and a logical mapper name (no "/dev/mapper/" prefix).
	// Example: devicePath="/dev/sdb", mapperName="luks-pvc_123".
	// Passphrase is provided via stdin.
	Open(ctx context.Context, devicePath string, mapperName string, passphrase string, opts OpenOptions) error

	// Close deactivates a LUKS mapping.
	// Accepts either the mapper name (recommended) or the full mapper path ("/dev/mapper/<name>").
	// Example: mapperPath="/dev/mapper/luks-pvc_123".
	Close(ctx context.Context, mapperPath string) error

	// Resize grows a LUKS mapping to match the underlying block device.
	// Accepts either the mapper name or the full mapper path.
	// Mapping must be open; passphrase may be required via stdin depending on system config.
	Resize(ctx context.Context, mapperPath string, passphrase string) error

	// IsLUKS checks whether a raw block device contains a LUKS header.
	// Expects a full raw device path (e.g., "/dev/sdb").
	IsLUKS(ctx context.Context, devicePath string) error

	// Status returns the cryptsetup status output for a LUKS mapping.
	// Accepts the mapper name (recommended). For a full mapper path, trim to name first.
	// Example: mapperName="luks-pvc_123".
	Status(ctx context.Context, mapperName string) (string, error)

	// ChangeKey updates the passphrase for a LUKS device in place.
	// Expects a full raw device path (e.g., "/dev/sdb").
	// The old passphrase is supplied via a file descriptor; the new via stdin.
	ChangeKey(ctx context.Context, devicePath string, uuid string, oldPassphrase, newPassphrase string) error

	// TestPassphrase validates a passphrase against a LUKS device without creating a mapping.
	// Expects a full raw device path and a logical mapper name (e.g., "luks-pvc_123"). Passphrase via stdin.
	TestPassphrase(ctx context.Context, devicePath string, mapperName string, passphrase string) error
}

type adapter struct {
	cmd tridentexec.Command
}

type Option func(*adapter)

func WithCommand(cmd tridentexec.Command) Option {
	return func(c *adapter) { c.cmd = cmd }
}

func New(opts ...Option) Adapter {
	a := &adapter{
		cmd: tridentexec.NewCommand(),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}
