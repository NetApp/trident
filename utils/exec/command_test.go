// Copyright 2023 NetApp, Inc. All Rights Reserved.

package exec

import (
	"context"
	b64 "encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestShellProcess is a method that is called as a substitute for a shell command,
// the GO_TEST flag ensures that if it is called as part of the test suite, it is
// skipped. GO_TEST_RETURN_VALUE flag allows the caller to specify a base64 encoded version of what should be returned via stdout,
// GO_TEST_RETURN_CODE flag allows the caller to specify what the return code should be, and
// GO_TEST_DELAY flag allows the caller to inject a delay before the function returns.
// GO_TEST_RETURN_PADDING_LENGTH flag allows the caller to specify how many bytes to return in total,
// if GO_TEST_RETURN_VALUE does not use all the bytes, the end is padded with NUL data
func TestShellProcess(t *testing.T) {
	if os.Getenv("GO_TEST") != "1" {
		return
	}
	// Print out the test value to stdout
	returnString, _ := b64.StdEncoding.DecodeString(os.Getenv("GO_TEST_RETURN_VALUE"))
	fmt.Fprintf(os.Stdout, string(returnString))
	if os.Getenv("GO_TEST_RETURN_PADDING_LENGTH") != "" {
		padLength, _ := strconv.Atoi(os.Getenv("GO_TEST_RETURN_PADDING_LENGTH"))
		padString := make([]byte, padLength-len(returnString))
		fmt.Fprintf(os.Stdout, string(padString))
	}
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

func TestExecute(t *testing.T) {
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
		execResults fakeExecResults
	}{
		{
			name: "Success",
			args: args{
				ctx:  context.Background(),
				name: "foo",
				args: nil,
			},
			want:    []byte("bar"),
			wantErr: false,
			execResults: fakeExecResults{
				out:  "bar",
				code: 0,
			},
		},
		{
			name: "Success output encoding needed",
			args: args{
				ctx:  context.Background(),
				name: "foo",
				args: nil,
			},
			want:    make([]byte, 20),
			wantErr: false,
			execResults: fakeExecResults{
				out:  string(make([]byte, 20)),
				code: 0,
			},
		},
		{
			name: "Fail",
			args: args{
				ctx:  context.Background(),
				name: "foo",
				args: nil,
			},
			want:    []byte("bar"),
			wantErr: true,
			execResults: fakeExecResults{
				out:  "bar",
				code: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &command{
				executor: newFakeExecCommand(tt.execResults),
			}
			got, err := client.Execute(tt.args.ctx, tt.args.name, tt.args.args...)
			assert.Equalf(t, tt.wantErr, err != nil, "Execute(%v, %v, %v)", tt.args.ctx, tt.args.name, tt.args.args)
			assert.Equalf(t, tt.want, got, "Execute(%v, %v, %v)", tt.args.ctx, tt.args.name, tt.args.args)
		})
	}
}

func TestExecutePaddedOutput(t *testing.T) {
	type args struct {
		ctx  context.Context
		name string
		args []string
	}

	expectedWithPadding := make([]byte, 20)
	expectedWithPadding[0] = 'a'

	tests := []struct {
		name        string
		args        args
		want        []byte
		wantErr     bool
		execResults fakeExecResults
	}{
		{
			name: "Success", args: args{
				ctx:  context.Background(),
				name: "foo",
				args: nil,
			},
			want:    []byte("bar"),
			wantErr: false,
			execResults: fakeExecResults{
				out:     "bar",
				code:    0,
				padding: 3,
			},
		},
		{
			name: "Success output encoding needed",
			args: args{
				ctx:  context.Background(),
				name: "foo",
				args: nil,
			},
			want:    make([]byte, 20),
			wantErr: false,
			execResults: fakeExecResults{
				out:     string(make([]byte, 20)),
				code:    0,
				padding: 20,
			},
		},
		{
			name: "Success output padding needed",
			args: args{
				ctx:  context.Background(),
				name: "foo",
				args: nil,
			},
			want:    expectedWithPadding,
			wantErr: false,
			execResults: fakeExecResults{
				out:     "a",
				code:    0,
				padding: 20,
			},
		},
		{
			name: "Fail",
			args: args{
				ctx:  context.Background(),
				name: "foo",
				args: nil,
			},
			want:    []byte("bar"),
			wantErr: true,
			execResults: fakeExecResults{
				out:     "bar",
				code:    1,
				padding: 3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &command{
				executor: newFakeExecCommand(tt.execResults),
			}
			got, err := client.Execute(tt.args.ctx, tt.args.name, tt.args.args...)
			assert.Equalf(t, tt.wantErr, err != nil, "Execute(%v, %v, %v)", tt.args.ctx, tt.args.name, tt.args.args)
			assert.Equalf(t, tt.want, got, "Execute(%v, %v, %v)", tt.args.ctx, tt.args.name, tt.args.args)
		})
	}
}

func TestExecuteRedacted(t *testing.T) {
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
		execResults fakeExecResults
	}{
		{
			name: "Success",
			args: args{
				ctx:             context.Background(),
				name:            "foo",
				args:            nil,
				secretsToRedact: map[string]string{},
			},
			want:    []byte("bar"),
			wantErr: false,
			execResults: fakeExecResults{
				out:  "bar",
				code: 0,
			},
		},
		{
			name: "Fail",
			args: args{
				ctx:             context.Background(),
				name:            "foo",
				args:            nil,
				secretsToRedact: map[string]string{},
			},
			want:    []byte("bar"),
			wantErr: true,
			execResults: fakeExecResults{
				out:  "bar",
				code: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &command{
				executor: newFakeExecCommand(tt.execResults),
			}
			got, err := client.ExecuteRedacted(tt.args.ctx, tt.args.name, tt.args.args, tt.args.secretsToRedact)
			assert.Equalf(t, tt.wantErr, err != nil, "ExecuteRedacted(%v, %v, %v, %v)", tt.args.ctx, tt.args.name,
				tt.args.args, tt.args.secretsToRedact)
			assert.Equalf(t, tt.want, got, "ExecuteRedacted(%v, %v, %v, %v)", tt.args.ctx, tt.args.name,
				tt.args.args, tt.args.secretsToRedact)
		})
	}
}

func TestExecuteWithTimeout(t *testing.T) {
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
		delayStr    string
		execResults fakeExecResults
	}{
		{
			name: "Success",
			args: args{
				ctx:            context.Background(),
				name:           "foo",
				timeoutSeconds: 10 * time.Second,
				logOutput:      false,
				args:           nil,
			},
			want:     []byte("bar"),
			wantErr:  false,
			delayStr: "0",
			execResults: fakeExecResults{
				out:  "bar",
				code: 0,
			},
		},
		{
			name: "Fail",
			args: args{
				ctx:            context.Background(),
				name:           "foo",
				timeoutSeconds: 10 * time.Second,
				logOutput:      false,
				args:           nil,
			},
			want:     []byte("bar"),
			wantErr:  true,
			delayStr: "0",
			execResults: fakeExecResults{
				out:  "bar",
				code: 1,
			},
		},
		{
			name: "Timeout",
			args: args{
				ctx:            context.Background(),
				name:           "foo",
				timeoutSeconds: 1 * time.Second,
				logOutput:      false,
				args:           nil,
			},
			want:     []byte(nil),
			wantErr:  true,
			delayStr: "2s",
			execResults: fakeExecResults{
				out:  "bar",
				code: 0,
			},
		},
		{
			name: "Timeout no delay",
			args: args{
				ctx:            context.Background(),
				name:           "sleep",
				timeoutSeconds: 1 * time.Millisecond,
				logOutput:      false,
				args:           []string{"0.005"},
			},
			want:     []byte(nil),
			wantErr:  true,
			delayStr: "0",
			execResults: fakeExecResults{
				out:  "",
				code: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			tt.execResults.delay, err = time.ParseDuration(tt.delayStr)
			if err != nil {
				t.Error("Invalid duration value provided.")
			}

			client := &command{
				executor: newFakeExecCommand(tt.execResults),
			}
			got, err := client.ExecuteWithTimeout(tt.args.ctx, tt.args.name, tt.args.timeoutSeconds, tt.args.logOutput,
				tt.args.args...)
			assert.Equalf(t, tt.wantErr, err != nil, "ExecuteWithTimeout(%v, %v, %v, %v, %v)", tt.args.ctx,
				tt.args.name, tt.args.timeoutSeconds, tt.args.logOutput, tt.args.args)
			assert.Equalf(t, tt.want, got, "ExecuteWithTimeout(%v, %v, %v, %v, %v)", tt.args.ctx, tt.args.name,
				tt.args.timeoutSeconds, tt.args.logOutput, tt.args.args)
		})
	}
}

func TestExecuteWithTimeoutAndInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "Success", input: "fake_input"},
		{name: "Success - no input", input: ""},
	}

	stdinFile := "/dev/stdin"
	if runtime.GOOS == "windows" {
		t.Skip("Test not valid on windows")
		stdinFile = "conIN$"
	}

	for _, tt := range tests {
		ctx := context.Background()
		t.Run(tt.name, func(t *testing.T) {
			// Run a command that simply outputs stdin
			client := &command{
				executor: exec.CommandContext,
			}
			stdout, err := client.ExecuteWithTimeoutAndInput(ctx, "cat", 300*time.Second, true, tt.input, stdinFile)
			assert.NoError(t, err)
			assert.Equal(t, tt.input, string(stdout))
		})
	}
}

func TestSanitizeExecOutput(t *testing.T) {
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
			result := sanitizeExecOutput(test.input)
			assert.True(t, test.output == result, fmt.Sprintf("Expected %v not %v", test.output, result))
		})
	}
}
