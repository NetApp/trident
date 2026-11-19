// Copyright 2026 NetApp, Inc. All Rights Reserved.

package test

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/spf13/afero"

	"github.com/netapp/trident/utils/exec"
)

var (
	_ afero.Fs       = &FakeHost{}
	_ exec.Command   = &FakeHost{}
	_ exec.ExitError = &ExitError{}
)

// ExecHandler is a fake implementation of one binary. stdin carries the process
// input that the real adapters pipe via ExecuteWithTimeoutAndInput (e.g. a LUKS
// passphrase, a cryptsetup token JSON); handlers that don't consume input take
// it as `_`.
type ExecHandler func(ctx context.Context, fs afero.Fs, stdin string, args []string) ([]byte, error)

// ExitError carries a process exit code so adapters that branch on a binary's
// exit status (e.g. blkid "no signature" = 2, fsck "corrected" = 1, a dd I/O
// error = 1) via errors.As behave exactly as they do against the real command.
// Fake exec handlers return it to signal a non-zero exit with a given code.
type ExitError struct {
	Code int
	Msg  string
}

func (e *ExitError) Error() string { return e.Msg }
func (e *ExitError) ExitCode() int { return e.Code }

type FakeHost struct {
	afero.Fs
	mu   *sync.Mutex
	cmds map[string]ExecHandler
}

func NewFakeHost() *FakeHost {
	return &FakeHost{
		Fs:   afero.NewMemMapFs(),
		mu:   &sync.Mutex{},
		cmds: make(map[string]ExecHandler),
	}
}

// Stage seeds the FakeHost state with arbitrary filesystem elements.
// This allows domain drivers to fake out the structure of the filesystem.
func (fh *FakeHost) Stage(seeds ...func(afero.Fs)) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	for _, seed := range seeds {
		seed(fh.Fs)
	}
}

func (fh *FakeHost) OnExec(binary string, h ExecHandler) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	fh.cmds[binary] = h
}

// Execute implements exec.Command itself (a router) — do NOT rely on the
// embedded nil interface, or Execute panics on first call.
func (fh *FakeHost) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	return fh.execute(ctx, name, "", args)
}

// execute is the single routing core for every Execute* variant; stdin is "" for
// the input-less variants and the piped input for ExecuteWithTimeoutAndInput.
func (fh *FakeHost) execute(ctx context.Context, name, stdin string, args []string) ([]byte, error) {
	fh.mu.Lock()
	defer fh.mu.Unlock()
	cmd, ok := fh.cmds[name]
	if !ok {
		return nil, fmt.Errorf("fakehost has no handler for %s", name)
	}
	return cmd(ctx, fh.Fs, stdin, args)
}

func (fh *FakeHost) ExecuteWithoutLog(ctx context.Context, name string, args ...string) ([]byte, error) {
	return fh.execute(ctx, name, "", args)
}

func (fh *FakeHost) ExecuteRedacted(ctx context.Context, name string, args []string, _ map[string]string) ([]byte, error) {
	return fh.execute(ctx, name, "", args)
}

func (fh *FakeHost) ExecuteWithTimeout(ctx context.Context, name string, _ time.Duration, _ bool, args ...string) ([]byte, error) {
	return fh.execute(ctx, name, "", args)
}

func (fh *FakeHost) ExecuteWithTimeoutAndInput(ctx context.Context, name string, _ time.Duration, _ bool, stdin string, args ...string) ([]byte, error) {
	return fh.execute(ctx, name, stdin, args)
}
