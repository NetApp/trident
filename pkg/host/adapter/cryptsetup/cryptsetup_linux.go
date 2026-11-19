// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cryptsetup

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	. "github.com/netapp/trident/logging"
	tridentexec "github.com/netapp/trident/utils/exec"
)

const (
	cryptsetup  = "cryptsetup"
	execTimeout = time.Second * 60

	// cryptsetup exit codes from src/utils_tools.c translate_errno().
	// All subcommands share these meanings, except for the carve-outs documented
	// inline at the call sites (isLuks).
	exitWrongParam  = 1 // EINVAL/ENOENT/ENOSYS — wrong or missing parameters
	exitNoPerm      = 2 // EPERM — no permission, bad passphrase, FIPS/quality reject
	exitOOM         = 3 // ENOMEM — out of memory (transient)
	exitWrongDevice = 4 // ENODEV/ENOTBLK — wrong device or file specified
	exitBusy        = 5 // EBUSY/EEXIST — device exists or busy
)

func parseExecError(subcmd string, out []byte, err error) error {
	var exitErr tridentexec.ExitError
	if !errors.As(err, &exitErr) {
		return fmt.Errorf("%s %s failed: %w", cryptsetup, subcmd, err)
	}

	base := &Error{bin: cryptsetup, cmd: subcmd, code: exitErr.ExitCode(), out: string(out)}
	switch exitErr.ExitCode() {
	case exitNoPerm:
		return &IncorrectPassphraseError{err: base}
	case exitOOM:
		return &OOMError{err: base}
	case exitWrongDevice:
		return &DeviceNotActiveError{err: base}
	case exitBusy:
		return &BusyDeviceError{err: base}
	case exitWrongParam:
		return &BadParameterError{err: base}
	default:
		return base
	}
}

func (a *adapter) Version(ctx context.Context) (string, error) {
	Logc(ctx).Trace(">>>> cryptsetup_linux.Version")
	defer Logc(ctx).Trace("<<<< cryptsetup_linux.Version")

	out, err := a.cmd.Execute(ctx, cryptsetup, "--version")
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(out)), nil
}

func (a *adapter) Format(
	ctx context.Context, devicePath string, passphrase string, opts FormatOptions,
) (string, error) {
	Logc(ctx).WithField("devicePath", devicePath).Trace(">>>> cryptsetup_linux.Format")
	defer Logc(ctx).WithField("devicePath", devicePath).Trace("<<<< cryptsetup_linux.Format")

	if devicePath == "" {
		return "", errors.New("empty device path")
	}
	if passphrase == "" {
		return "", errors.New("empty passphrase")
	}

	cmd := "luksFormat"
	args := []string{
		cmd,
		devicePath,
	}
	if opts.Type != "" {
		args = append(args, "--type", opts.Type)
	}
	if opts.Cipher != "" {
		args = append(args, "-c", opts.Cipher)
	}
	if opts.Hash != "" {
		args = append(args, "--hash", opts.Hash)
	}
	if opts.PBKDF != "" {
		args = append(args, "--pbkdf", opts.PBKDF)
	}
	if opts.PBKDFIterations != "" {
		args = append(args, "--pbkdf-force-iterations", opts.PBKDFIterations)
	}

	out, err := a.cmd.ExecuteWithTimeoutAndInput(ctx, cryptsetup, execTimeout, true, passphrase, args...)
	if err != nil {
		return "", parseExecError(cmd, out, err)
	}
	return string(out), nil
}

func (a *adapter) Open(
	ctx context.Context, devicePath string, mapperName string, passphrase string, opts OpenOptions,
) error {
	fields := LogFields{"devicePath": devicePath, "mapperName": mapperName}
	Logc(ctx).WithFields(fields).Trace(">>>> cryptsetup_linux.Open")
	defer Logc(ctx).WithFields(fields).Trace("<<<< cryptsetup_linux.Open")

	if devicePath == "" {
		return errors.New("empty device path")
	}
	if mapperName == "" {
		return errors.New("empty mapper name")
	}
	if passphrase == "" {
		return errors.New("empty passphrase")
	}

	cmd := "open"
	args := []string{
		cmd,
		devicePath,
		mapperName,
	}
	if opts.Type != "" {
		args = append(args, "--type", opts.Type)
	}
	if opts.AllowDiscards {
		args = append(args, "--allow-discards")
	}

	out, err := a.cmd.ExecuteWithTimeoutAndInput(ctx, cryptsetup, execTimeout, true, passphrase, args...)
	if err != nil {
		return parseExecError(cmd, out, err)
	}
	return nil
}

func (a *adapter) Close(ctx context.Context, mapperPath string) error {
	Logc(ctx).WithField("mapperPath", mapperPath).Trace(">>>> cryptsetup_linux.Close")
	defer Logc(ctx).WithField("mapperPath", mapperPath).Trace("<<<< cryptsetup_linux.Close")

	if mapperPath == "" {
		return errors.New("empty mapper device path")
	}

	out, err := a.cmd.ExecuteWithTimeoutAndInput(ctx, cryptsetup, execTimeout, true, "", "luksClose", mapperPath)
	if err != nil {
		return parseExecError("luksClose", out, err)
	}

	return nil
}

func (a *adapter) Resize(ctx context.Context, mapperPath string, passphrase string) error {
	Logc(ctx).WithField("mapperPath", mapperPath).Trace(">>>> cryptsetup_linux.Resize")
	defer Logc(ctx).WithField("mapperPath", mapperPath).Trace("<<<< cryptsetup_linux.Resize")

	if mapperPath == "" {
		return errors.New("empty mapper device path")
	}
	if passphrase == "" {
		return errors.New("empty passphrase")
	}

	out, err := a.cmd.ExecuteWithTimeoutAndInput(ctx, cryptsetup, execTimeout, true, passphrase, "resize", mapperPath)
	if err != nil {
		return parseExecError("resize", out, err)
	}

	return nil
}

func (a *adapter) IsLUKS(ctx context.Context, devicePath string) error {
	fields := LogFields{"devicePath": devicePath}
	Logc(ctx).WithFields(fields).Trace(">>>> cryptsetup_linux.IsLUKS")
	defer Logc(ctx).WithFields(fields).Trace("<<<< cryptsetup_linux.IsLUKS")

	if devicePath == "" {
		return errors.New("empty device path")
	}

	out, err := a.cmd.ExecuteWithTimeoutAndInput(ctx, cryptsetup, execTimeout, true, "", "isLuks", devicePath)
	if err == nil {
		return nil
	}

	// isLuks bypasses translate_errno: exit 1 means "valid device, no LUKS header",
	// not "wrong parameters". Re-classify before returning.
	parsedErr := parseExecError("isLuks", out, err)
	var badParam *BadParameterError
	if errors.As(parsedErr, &badParam) {
		return &NotLUKSDeviceError{err: badParam.err}
	}
	return parsedErr
}

func (a *adapter) Status(ctx context.Context, mapperName string) (string, error) {
	Logc(ctx).WithField("mapperName", mapperName).Trace(">>>> cryptsetup_linux.Status")
	defer Logc(ctx).WithField("mapperName", mapperName).Trace("<<<< cryptsetup_linux.Status")

	if mapperName == "" {
		return "", errors.New("empty mapper device name")
	}

	out, err := a.cmd.ExecuteWithTimeoutAndInput(ctx, cryptsetup, execTimeout, true, "", "status", mapperName)
	if err != nil {
		return "", parseExecError("status", out, err)
	}

	return string(out), nil
}

func (a *adapter) ChangeKey(
	ctx context.Context, devicePath string, uuid string, oldPassphrase, newPassphrase string,
) error {
	Logc(ctx).WithField("devicePath", devicePath).Trace(">>>> cryptsetup_linux.ChangeKey")
	defer Logc(ctx).WithField("devicePath", devicePath).Trace("<<<< cryptsetup_linux.ChangeKey")

	if oldPassphrase == "" {
		return errors.New("old passphrase is required to change LUKS passphrase")
	}
	if newPassphrase == "" {
		return errors.New("new passphrase is required to change LUKS passphrase")
	}

	// Write the old passphrase to an anonymous file in memory because we can't provide it on stdin
	// "uuid" is typically a volume_id. Use it to create a unique temp file name.
	tempFileName := fmt.Sprintf("luks_key_%s", uuid)
	fd, err := generateAnonymousMemFile(tempFileName, oldPassphrase)
	if err != nil {
		return fmt.Errorf("failed to create passphrase files for LUKS: %w", err)
	}
	defer func() { _ = unix.Close(fd) }()

	// Rely on Linux's ability to reference already-open files with /dev/fd/*
	oldKeyFilename := fmt.Sprintf("/dev/fd/%d", fd)

	out, err := a.cmd.ExecuteWithTimeoutAndInput(
		ctx, cryptsetup, execTimeout, true,
		newPassphrase, "luksChangeKey", "-d", oldKeyFilename, devicePath,
	)
	if err != nil {
		return parseExecError("luksChangeKey", out, err)
	}

	return nil
}

func (a *adapter) TestPassphrase(ctx context.Context, devicePath string, mapperName string, passphrase string) error {
	fields := LogFields{"devicePath": devicePath, "mapperName": mapperName}
	Logc(ctx).WithFields(fields).Trace(">>>> cryptsetup_linux.TestPassphrase")
	defer Logc(ctx).WithFields(fields).Trace("<<<< cryptsetup_linux.TestPassphrase")

	out, err := a.cmd.ExecuteWithTimeoutAndInput(
		ctx, cryptsetup, execTimeout, true, passphrase,
		"open", devicePath, mapperName, "--type", luksType, "--test-passphrase",
	)
	if err != nil {
		return parseExecError("open --test-passphrase", out, err)
	}

	return nil
}

// generateAnonymousMemFile uses linux syscall memfd_create to create an anonymous, temporary, in-memory file
// with the specified name and contents
func generateAnonymousMemFile(tempFileName, content string) (fd int, err error) {
	fd, err = unix.MemfdCreate(tempFileName, 0)
	if err != nil {
		return -1, fmt.Errorf("failed to create anonymous file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = unix.Close(fd)
		}
	}()

	_, err = unix.Write(fd, []byte(content))
	if err != nil {
		return fd, fmt.Errorf("failed to write anonymous file: %w", err)
	}
	// Rewind back to the beginning
	_, err = unix.Seek(fd, 0, 0)
	if err != nil {
		return fd, fmt.Errorf("failed to rewind anonymous file: %w", err)
	}

	return fd, nil
}
