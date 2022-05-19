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

func TestParseIPv6Valid(t *testing.T) {
	tests := map[string]struct {
		input     string
		output    bool
		predicate func(string) bool
	}{
		"IPv6 Address": {
			input:  "fd20:8b1e:b258:2000:f816:3eff:feec:0",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 Address with Brackets": {
			input:  "[fd20:8b1e:b258:2000:f816:3eff:feec:0]",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 Address with Port": {
			input:  "[fd20:8b1e:b258:2000:f816:3eff:feec:0]:8000",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 localhost Address": {
			input:  "::1",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 localhost Address with Brackets": {
			input:  "[::1]",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 Zero Address": {
			input:  "::",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv6 Zero Address with Brackets": {
			input:  "[::]",
			output: true,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.True(t, test.predicate(test.input), "Predicate failed")
		})
	}
}

func TestParseIPv4Valid(t *testing.T) {
	tests := map[string]struct {
		input     string
		output    bool
		predicate func(string) bool
	}{
		"IPv4 Address": {
			input:  "127.0.0.1",
			output: false,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv4 Address with Brackets": {
			input:  "[127.0.0.1]",
			output: false,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv4 Address with Port": {
			input:  "127.0.0.1:8000",
			output: false,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
		"IPv4 Zero Address": {
			input:  "0.0.0.0",
			output: false,
			predicate: func(input string) bool {
				return IPv6Check(input)
			},
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.False(t, test.predicate(test.input), "Predicate failed")
		})
	}
}

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

func TestGetHostportIP(t *testing.T) {
	type IPAddresses struct {
		InputIP  string
		OutputIP string
	}
	tests := []IPAddresses{
		{
			InputIP:  "1.2.3.4:5678",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "1.2.3.4",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "[1:2:3:4]:5678",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "[1:2:3:4]",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]:5678",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "2607:f8b0:4006:818:0:0:0:2004",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputIP, func(t *testing.T) {
			assert.Equal(t, testCase.OutputIP, getHostportIP(testCase.InputIP), "IP mismatch")
		})
	}
}

func TestEnsureHostportFormatted(t *testing.T) {
	type IPAddresses struct {
		InputIP  string
		OutputIP string
	}
	tests := []IPAddresses{
		{
			InputIP:  "1.2.3.4:5678",
			OutputIP: "1.2.3.4:5678",
		},
		{
			InputIP:  "1.2.3.4",
			OutputIP: "1.2.3.4",
		},
		{
			InputIP:  "[1:2:3:4]:5678",
			OutputIP: "[1:2:3:4]:5678",
		},
		{
			InputIP:  "[1:2:3:4]",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "1:2:3:4",
			OutputIP: "[1:2:3:4]",
		},
		{
			InputIP:  "2607:f8b0:4006:818:0:0:0:2004",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]",
		},
		{
			InputIP:  "[2607:f8b0:4006:818:0:0:0:2004]:5678",
			OutputIP: "[2607:f8b0:4006:818:0:0:0:2004]:5678",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputIP, func(t *testing.T) {
			assert.Equal(t, testCase.OutputIP, ensureHostportFormatted(testCase.InputIP),
				"Hostport not correctly formatted")
		})
	}
}

func TestFormatPortal(t *testing.T) {
	type IPAddresses struct {
		InputPortal  string
		OutputPortal string
	}
	tests := []IPAddresses{
		{
			InputPortal:  "203.0.113.1",
			OutputPortal: "203.0.113.1:3260",
		},
		{
			InputPortal:  "203.0.113.1:3260",
			OutputPortal: "203.0.113.1:3260",
		},
		{
			InputPortal:  "203.0.113.1:3261",
			OutputPortal: "203.0.113.1:3261",
		},
		{
			InputPortal:  "[2001:db8::1]",
			OutputPortal: "[2001:db8::1]:3260",
		},
		{
			InputPortal:  "[2001:db8::1]:3260",
			OutputPortal: "[2001:db8::1]:3260",
		},
		{
			InputPortal:  "[2001:db8::1]:3261",
			OutputPortal: "[2001:db8::1]:3261",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputPortal, func(t *testing.T) {
			assert.Equal(t, testCase.OutputPortal, formatPortal(testCase.InputPortal), "Portal not correctly formatted")
		})
	}
}

func TestFilterTargets(t *testing.T) {
	type FilterCase struct {
		CommandOutput string
		InputPortal   string
		OutputIQNs    []string
	}
	tests := []FilterCase{
		{
			// Simple positive test, expect first
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"203.0.113.3:3260,-1 iqn.2010-01.com.solidfire:baz\n",
			InputPortal: "203.0.113.1:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:foo"},
		},
		{
			// Simple positive test, expect second
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:bar"},
		},
		{
			// Expect empty list
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n",
			InputPortal: "203.0.113.3:3260",
			OutputIQNs:  []string{},
		},
		{
			// Expect multiple
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:baz\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:bar", "iqn.1992-08.com.netapp:baz"},
		},
		{
			// Bad input
			CommandOutput: "" +
				"Foobar\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{},
		},
		{
			// Good and bad input
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"Foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"Bar\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:baz\n",
			InputPortal: "203.0.113.2:3260",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:bar", "iqn.1992-08.com.netapp:baz"},
		},
		{
			// Try nonstandard port number
			CommandOutput: "" +
				"203.0.113.1:3260,1024 iqn.1992-08.com.netapp:foo\n" +
				"203.0.113.2:3260,1025 iqn.1992-08.com.netapp:bar\n" +
				"203.0.113.2:3261,1025 iqn.1992-08.com.netapp:baz\n",
			InputPortal: "203.0.113.2:3261",
			OutputIQNs:  []string{"iqn.1992-08.com.netapp:baz"},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.InputPortal, func(t *testing.T) {
			targets := filterTargets(testCase.CommandOutput, testCase.InputPortal)
			assert.Equal(t, testCase.OutputIQNs, targets, "Wrong targets returned")
		})
	}
}

func TestParseInitiatorIQNs(t *testing.T) {
	ctx := context.TODO()
	tests := map[string]struct {
		input     string
		output    []string
		predicate func(string) []string
	}{
		"Single valid initiator": {
			input:  "InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de",
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"initiator with space": {
			input:  "InitiatorName=iqn 2005-03.org.open-iscsi:123abc456de",
			output: []string{"iqn"},
		},
		"Multiple valid initiators": {
			input: `InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de
InitiatorName=iqn.2005-03.org.open-iscsi:secondIQN12`,
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de", "iqn.2005-03.org.open-iscsi:secondIQN12"},
		},
		"Ignore comment initiator": {
			input: `#InitiatorName=iqn.1994-05.demo.netapp.com
InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de`,
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Ignore inline comment": {
			input:  "InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de #inline comment in file",
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Tolerate space around equal sign": {
			input:  "InitiatorName = iqn.2005-03.org.open-iscsi:123abc456de",
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Tolerate leading space": {
			input:  " InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de",
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Tolerate trailing space multiple initiators": {
			input: `InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de
InitiatorName=iqn.2005-03.org.open-iscsi:secondIQN12 `,
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de", "iqn.2005-03.org.open-iscsi:secondIQN12"},
		},
		"Full iscsi file": {
			input: `## DO NOT EDIT OR REMOVE THIS FILE!
## If you remove this file, the iSCSI daemon will not start.
## If you change the InitiatorName, existing access control lists
## may reject this initiator.  The InitiatorName must be unique
## for each iSCSI initiator.  Do NOT duplicate iSCSI InitiatorNames.
InitiatorName=iqn.2005-03.org.open-iscsi:123abc456de
#InitiatorName=iqn.1994-05.demo.netapp.com`,
			output: []string{"iqn.2005-03.org.open-iscsi:123abc456de"},
		},
		"Full iscsi file no initiator": {
			input: `## DO NOT EDIT OR REMOVE THIS FILE!
## If you remove this file, the iSCSI daemon will not start.
## If you change the InitiatorName, existing access control lists
## may reject this initiator.  The InitiatorName must be unique
## for each iSCSI initiator.  Do NOT duplicate iSCSI InitiatorNames.
#InitiatorName=iqn.1994-05.demo.netapp.com`,
			output: []string{},
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			iqns := parseInitiatorIQNs(ctx, test.input)
			assert.Equal(t, test.output, iqns, "Failed to parse initiators")
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
