// Copyright 2022 NetApp, Inc. All Rights Reserved.

package utils

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

var (
	execReturnValue string
	execReturnCode  int
	execDelay       time.Duration
)

func TestSanitizeString(t *testing.T) {
	tests := map[string]struct {
		input  string
		output string
	}{
		"Replace xtermControlRegex#1": {
			input:  "\x1B[A" + "HelloWorld",
			output: "HelloWorld",
		},
		"Strip trailing newline": {
			input:  "\n\n",
			output: "\n",
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			result := sanitizeString(test.input)
			assert.True(t, test.output == result, fmt.Sprintf("Expected %v not %v", test.output, result))
		})
	}
}

func TestPidRunningOrIdleRegex(t *testing.T) {
	log.Debug("Running TestPidRegexes...")

	tests := map[string]struct {
		input          string
		expectedOutput bool
	}{
		// Negative tests
		"Negative input #1": {
			input:          "",
			expectedOutput: false,
		},
		"Negative input #2": {
			input:          "pid -5 running",
			expectedOutput: false,
		},
		"Negative input #3": {
			input:          "pid running",
			expectedOutput: false,
		},
		// Positive tests
		"Positive input #1": {
			input:          "pid 5 running",
			expectedOutput: true,
		},
		// Positive tests
		"Positive input #2": {
			input:          "pid 2509 idle",
			expectedOutput: true,
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			result := pidRunningOrIdleRegex.MatchString(test.input)
			assert.True(t, test.expectedOutput == result)
		})
	}
}

// TestShellProcess is a method that is called as a substitute for a shell command,
// the GO_TEST flag ensures that if it is called as part of the test suite, it is
// skipped. GO_TEST_RETURN_VALUE flag allows the caller to specify what should be returned via stdout,
// GO_TEST_RETURN_CODE flag allows the caller to specify what the return code should be, and
// GO_TEST_DELAY flag allows the caller to inject a delay before the function returns.
func TestShellProcess(t *testing.T) {
	if os.Getenv("GO_TEST") != "1" {
		return
	}
	// Print out the test value to stdout
	fmt.Fprintf(os.Stdout, os.Getenv("GO_TEST_RETURN_VALUE"))
	code, err := strconv.Atoi(os.Getenv("GO_TEST_RETURN_CODE"))
	if err != nil {
		code = -1
	}
	// Pause for some amount of time
	delay, err := time.ParseDuration(os.Getenv("GO_TEST_DELAY"))
	if err == nil {
		time.Sleep(delay)
	}
	os.Exit(code)
}

// fakeExecCommand is a function that initialises a new exec.Cmd, one which will
// simply call TestShellProcess rather than the command it is provided. It will
// also pass through the command and its arguments as an argument to TestShellProcess
func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestShellProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{
		"GO_TEST=1", fmt.Sprintf("GO_TEST_RETURN_VALUE=%s", execReturnValue),
		fmt.Sprintf("GO_TEST_RETURN_CODE=%d", execReturnCode), fmt.Sprintf("GO_TEST_DELAY=%s", execDelay),
	}
	return cmd
}

func Test_multipathdIsRunning(t *testing.T) {
	ExecCommand = fakeExecCommand
	// Reset exec command after tests
	defer func() {
		ExecCommand = exec.Command
	}()
	tests := []struct {
		name          string
		returnValue   string
		returnCode    int
		expectedValue bool
	}{
		{name: "True", returnValue: "1234", returnCode: 0, expectedValue: true},
		{name: "False", returnValue: "", returnCode: 0, expectedValue: false},
		{name: "Error", returnValue: "1234", returnCode: 1, expectedValue: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execReturnValue = tt.returnValue
			execReturnCode = tt.returnCode
			actualValue := multipathdIsRunning(context.Background())
			assert.Equal(t, tt.expectedValue, actualValue)
		})
	}
}

func Test_execCommand(t *testing.T) {
	ExecCommand = fakeExecCommand
	// Reset exec command after tests
	defer func() {
		ExecCommand = exec.Command
	}()
	type args struct {
		ctx  context.Context
		name string
		args []string
	}
	tests := []struct {
		name        string
		args        args
		want        []byte
		wantErr     bool
		returnValue string
		returnCode  int
	}{
		{name: "Success", args: args{
			ctx:  context.Background(),
			name: "foo",
			args: nil,
		}, want: []byte("bar"), wantErr: false, returnValue: "bar", returnCode: 0},
		{name: "Fail", args: args{
			ctx:  context.Background(),
			name: "foo",
			args: nil,
		}, want: []byte("bar"), wantErr: true, returnValue: "bar", returnCode: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execReturnValue = tt.returnValue
			execReturnCode = tt.returnCode
			got, err := execCommand(tt.args.ctx, tt.args.name, tt.args.args...)
			assert.Equalf(t, tt.wantErr, err != nil, "execCommand(%v, %v, %v)", tt.args.ctx, tt.args.name, tt.args.args)
			assert.Equalf(t, tt.want, got, "execCommand(%v, %v, %v)", tt.args.ctx, tt.args.name, tt.args.args)
		})
	}
}

func Test_execCommandRedacted(t *testing.T) {
	ExecCommand = fakeExecCommand
	// Reset exec command after tests
	defer func() {
		ExecCommand = exec.Command
	}()
	type args struct {
		ctx             context.Context
		name            string
		args            []string
		secretsToRedact map[string]string
	}
	tests := []struct {
		name        string
		args        args
		want        []byte
		wantErr     bool
		returnValue string
		returnCode  int
	}{
		{name: "Success", args: args{
			ctx:             context.Background(),
			name:            "foo",
			args:            nil,
			secretsToRedact: map[string]string{},
		}, want: []byte("bar"), wantErr: false, returnValue: "bar", returnCode: 0},
		{name: "Fail", args: args{
			ctx:             context.Background(),
			name:            "foo",
			args:            nil,
			secretsToRedact: map[string]string{},
		}, want: []byte("bar"), wantErr: true, returnValue: "bar", returnCode: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execReturnValue = tt.returnValue
			execReturnCode = tt.returnCode
			got, err := execCommandRedacted(tt.args.ctx, tt.args.name, tt.args.args, tt.args.secretsToRedact)
			assert.Equalf(t, tt.wantErr, err != nil, "execCommandRedacted(%v, %v, %v, %v)", tt.args.ctx, tt.args.name,
				tt.args.args, tt.args.secretsToRedact)
			assert.Equalf(t, tt.want, got, "execCommandRedacted(%v, %v, %v, %v)", tt.args.ctx, tt.args.name,
				tt.args.args, tt.args.secretsToRedact)
		})
	}
}

func Test_execCommandWithTimeout(t *testing.T) {
	ExecCommand = fakeExecCommand
	// Reset exec command after tests
	defer func() {
		ExecCommand = exec.Command
	}()
	type args struct {
		ctx            context.Context
		name           string
		timeoutSeconds time.Duration
		logOutput      bool
		args           []string
	}
	tests := []struct {
		name        string
		args        args
		want        []byte
		wantErr     bool
		returnValue string
		returnCode  int
		delay       string
	}{
		{name: "Success", args: args{
			ctx:            context.Background(),
			name:           "foo",
			timeoutSeconds: 10,
			logOutput:      false,
			args:           nil,
		}, want: []byte("bar"), wantErr: false, returnValue: "bar", returnCode: 0, delay: "0"},
		{name: "Fail", args: args{
			ctx:            context.Background(),
			name:           "foo",
			timeoutSeconds: 10,
			logOutput:      false,
			args:           nil,
		}, want: []byte("bar"), wantErr: true, returnValue: "bar", returnCode: 1, delay: "0"},
		{name: "Timeout", args: args{
			ctx:            context.Background(),
			name:           "foo",
			timeoutSeconds: 1,
			logOutput:      false,
			args:           nil,
		}, want: []byte(nil), wantErr: true, returnValue: "bar", returnCode: 0, delay: "2s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			execReturnValue = tt.returnValue
			execReturnCode = tt.returnCode
			delay, err := time.ParseDuration(tt.delay)
			if err != nil {
				t.Error("Invalid duration value provided.")
			}
			execDelay = delay
			got, err := execCommandWithTimeout(tt.args.ctx, tt.args.name, tt.args.timeoutSeconds, tt.args.logOutput,
				tt.args.args...)
			assert.Equalf(t, tt.wantErr, err != nil, "execCommandWithTimeout(%v, %v, %v, %v, %v)", tt.args.ctx,
				tt.args.name, tt.args.timeoutSeconds, tt.args.logOutput, tt.args.args)
			assert.Equalf(t, tt.want, got, "execCommandWithTimeout(%v, %v, %v, %v, %v)", tt.args.ctx, tt.args.name,
				tt.args.timeoutSeconds, tt.args.logOutput, tt.args.args)
		})
	}
}
