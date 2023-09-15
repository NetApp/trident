// Copyright 2023 NetApp, Inc. All Rights Reserved.

package exec

//go:generate mockgen -destination=../../mocks/mock_utils/mock_exec/mock_command.go github.com/netapp/trident/utils/exec Command

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

var (
	xtermControlRegex = regexp.MustCompile(`\x1B\[[0-9;]*[a-zA-Z]`)

	_ Command = NewCommand()
)

// Command defines a set of behaviors for executing commands on a host.
type Command interface {
	Execute(ctx context.Context, name string, args ...string) ([]byte, error)
	ExecuteRedacted(
		ctx context.Context, name string, args []string,
		secretsToRedact map[string]string,
	) ([]byte, error)
	ExecuteWithTimeout(
		ctx context.Context, name string, timeout time.Duration, logOutput bool, args ...string,
	) ([]byte, error)
	ExecuteWithTimeoutAndInput(
		ctx context.Context, name string, timeout time.Duration, logOutput bool, stdin string, args ...string,
	) ([]byte, error)
	ExecuteWithoutLog(ctx context.Context, name string, args ...string) ([]byte, error)
}

// command is a receiver for the Command interface.
type command struct {
	executor func(ctx context.Context, name string, args ...string) *exec.Cmd
}

// NewCommand returns a concrete client for executing OS-level commands.
func NewCommand() Command {
	return &command{
		executor: exec.CommandContext,
	}
}

// Execute invokes an external process.
func (c *command) Execute(ctx context.Context, name string, args ...string) ([]byte, error) {
	Logc(ctx).WithFields(LogFields{
		"command": name,
		"args":    args,
	}).Debug(">>>> command.Execute.")

	// create context with a cancellation
	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, err := c.executor(cancelCtx, name, args...).CombinedOutput()

	Logc(ctx).WithFields(LogFields{
		"command": name,
		"output":  sanitizeExecOutput(string(out)),
		"error":   err,
	}).Debug("<<<< Execute.")

	return out, err
}

// ExecuteRedacted invokes an external process, and redacts sensitive arguments.
func (c *command) ExecuteRedacted(
	ctx context.Context, name string, args []string,
	secretsToRedact map[string]string,
) ([]byte, error) {
	var sanitizedArgs []string
	for _, arg := range args {
		val, ok := secretsToRedact[arg]
		var sanitizedArg string
		if ok {
			sanitizedArg = val
		} else {
			sanitizedArg = arg
		}
		sanitizedArgs = append(sanitizedArgs, sanitizedArg)
	}

	Logc(ctx).WithFields(LogFields{
		"command": name,
		"args":    sanitizedArgs,
	}).Debug(">>>> command.ExecuteRedacted.")

	// create context with a cancellation
	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out, err := c.executor(cancelCtx, name, args...).CombinedOutput()

	Logc(ctx).WithFields(LogFields{
		"command": name,
		"output":  sanitizeExecOutput(string(out)),
		"error":   err,
	}).Debug("<<<< command.ExecuteRedacted.")

	return out, err
}

// ExecuteWithTimeout invokes an external shell command and lets it time out if it exceeds the
// specified interval.
func (c *command) ExecuteWithTimeout(
	ctx context.Context, name string, timeout time.Duration, logOutput bool, args ...string,
) ([]byte, error) {
	Logc(ctx).WithFields(LogFields{
		"command": name,
		"timeout": timeout,
		"args":    args,
	}).Debug(">>>> command.ExecuteWithTimeout.")
	defer Logc(ctx).Debug("<<<< command.ExecuteWithTimeout.")

	return c.executeWithTimeoutAndInput(ctx, name, timeout, logOutput, "", args...)
}

// ExecuteWithTimeoutAndInput invokes an external shell command and lets it time out if it exceeds the
// specified interval.
func (c *command) ExecuteWithTimeoutAndInput(
	ctx context.Context, name string, timeout time.Duration, logOutput bool, stdin string, args ...string,
) ([]byte, error) {
	Logc(ctx).WithFields(LogFields{
		"command":        name,
		"timeoutSeconds": timeout,
		"args":           args,
	}).Debug(">>>> command.ExecuteWithTimeoutAndInput.")
	defer Logc(ctx).Debug("<<<< command.ExecuteWithTimeoutAndInput.")

	return c.executeWithTimeoutAndInput(ctx, name, timeout, logOutput, stdin, args...)
}

// executeWithTimeoutAndInput is a common handler for invoking external shell commands with timeouts.
// It lets commands time out if they exceed the specified interval.
func (c *command) executeWithTimeoutAndInput(
	ctx context.Context, name string, timeout time.Duration, logOutput bool, stdin string, args ...string,
) ([]byte, error) {
	// Create context with a cancellation.
	cancelCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// execCommandResult is used to return shell command results via channels between goroutines.
	type execCommandResult struct {
		Output []byte
		Error  error
	}

	cmd := c.executor(cancelCtx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	done := make(chan execCommandResult, 1)
	var result execCommandResult

	go func() {
		out, err := cmd.CombinedOutput()
		done <- execCommandResult{Output: out, Error: err}
	}()

	select {
	case <-cancelCtx.Done():
		if err := cancelCtx.Err(); err == context.DeadlineExceeded {
			Logc(ctx).WithFields(LogFields{
				"process": name,
			}).Error("process killed after timeout")
			result = execCommandResult{Output: nil, Error: errors.TimeoutError("process killed after timeout")}
		} else {
			Logc(ctx).WithFields(LogFields{
				"process": name,
				"error":   err,
			}).Error("context ended unexpectedly")
			result = execCommandResult{Output: nil, Error: fmt.Errorf("context ended unexpectedly: %v", err)}
		}
	case result = <-done:
		break
	}

	logFields := Logc(ctx).WithFields(LogFields{
		"command": name,
		"error":   result.Error,
	})

	if logOutput {
		logFields.WithFields(LogFields{
			"output": sanitizeExecOutput(string(result.Output)),
		})
	}

	return result.Output, result.Error
}

func sanitizeExecOutput(s string) string {
	// Strip xterm color & movement characters
	s = xtermControlRegex.ReplaceAllString(s, "")
	// Strip trailing newline
	s = strings.TrimSuffix(s, "\n")
	return s
}

// ExecuteWithoutLog invokes an external process, and does not log output returned by the command.
func (c *command) ExecuteWithoutLog(ctx context.Context, name string, args ...string) ([]byte, error) {
	Logc(ctx).WithFields(LogFields{
		"command": name,
		"args":    args,
	}).Debug(">>>> command.ExecuteWithoutLog")

	// create context with a cancellation
	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out, err := c.executor(cancelCtx, name, args...).CombinedOutput()

	Logc(ctx).WithFields(LogFields{
		"command": name,
		"error":   err,
	}).Debug("<<<< command.ExecuteWithoutLog")
	return out, err
}
