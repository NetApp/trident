// Copyright 2023 NetApp, Inc. All Rights Reserved.

package exec

import (
	"context"
	b64 "encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type fakeExecResults struct {
	out     string
	code    int
	padding int
	delay   time.Duration
}

// NewFakeExitError uses a fake exec command shell to generate a fake exit error for testing.
func NewFakeExitError(exitCode int, output string) *exec.ExitError {
	returns := fakeExecResults{
		code: exitCode,
		out:  output,
	}
	_, err := newFakeExecCommand(returns)(context.Background(), "bad cmd").CombinedOutput()
	return err.(*exec.ExitError)
}

// newFakeExecCommand creates a function that matches the signature of an exec.Cmd call.
// The returned function will set expected results within a fake exec.Cmd for testing.
func newFakeExecCommand(
	results fakeExecResults,
) func(ctx context.Context, command string, args ...string) *exec.Cmd {
	returnValue := results.out
	returnCode := results.code
	padding := results.padding
	delay := results.delay

	fakeExecCmd := func(ctx context.Context, command string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestShellProcess", "--", command}
		cs = append(cs, args...)

		// G204 is subprocess launched with potential tainted input or cmd args; since
		// this is needed for generating fake exit errors in other low-level utilities
		// as well as command_test.go, it must live here in testing.go. Otherwise, it is not visible.
		cmd := exec.CommandContext(ctx, os.Args[0], cs...) // #nosec G204

		returnString := b64.StdEncoding.EncodeToString([]byte(returnValue))
		envVars := []string{
			"GO_TEST=1",
			fmt.Sprintf("GOCOVERDIR=%s", os.Getenv("GOCOVERDIR")),
			fmt.Sprintf("GO_TEST_RETURN_VALUE=%s", returnString),
			fmt.Sprintf("GO_TEST_RETURN_CODE=%d", returnCode),
		}

		if padding != 0 {
			envVars = append(envVars, fmt.Sprintf("GO_TEST_RETURN_PADDING_LENGTH=%d", padding))
		}
		if delay != 0 {
			envVars = append(envVars, fmt.Sprintf("GO_TEST_DELAY=%s", delay))
		}

		cmd.Env = envVars
		return cmd
	}

	return fakeExecCmd
}
