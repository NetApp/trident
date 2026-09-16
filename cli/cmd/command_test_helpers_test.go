// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"
	"os/exec"
	"testing"

	"go.uber.org/mock/gomock"

	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
)

func withMockCommand(t *testing.T) *mockexec.MockCommand {
	t.Helper()
	mockCtrl := gomock.NewController(t)
	mockCmd := mockexec.NewMockCommand(mockCtrl)
	prev := command
	command = mockCmd
	t.Cleanup(func() { command = prev })
	return mockCmd
}

func withMockExecKubernetesCLIRaw(t *testing.T) {
	t.Helper()
	prev := execKubernetesCLIRaw
	execKubernetesCLIRaw = func(args ...string) *exec.Cmd {
		return exec.Command("false")
	}
	t.Cleanup(func() { execKubernetesCLIRaw = prev })
}

func expectKubernetesCLIUnavailable(mock *mockexec.MockCommand) {
	mock.EXPECT().ExecuteWithoutLog(gomock.Any(), CLIOpenshift, "version").
		Return(nil, fmt.Errorf("executable file not found"))
	mock.EXPECT().ExecuteWithoutLog(gomock.Any(), CLIKubernetes, "version").
		Return(nil, fmt.Errorf("executable file not found"))
}

func expectKubectlCLIAvailable(mock *mockexec.MockCommand) {
	mock.EXPECT().ExecuteWithoutLog(gomock.Any(), CLIOpenshift, "version").
		Return(nil, fmt.Errorf("not found"))
	mock.EXPECT().ExecuteWithoutLog(gomock.Any(), CLIKubernetes, "version").
		Return([]byte("ok"), nil)
}

func expectOpenShiftCLIAvailable(mock *mockexec.MockCommand) {
	mock.EXPECT().ExecuteWithoutLog(gomock.Any(), CLIOpenshift, "version").
		Return([]byte("ok"), nil)
}

func expectKubernetesCLIUnavailableForTest(t *testing.T) {
	t.Helper()
	mock := withMockCommand(t)
	expectKubernetesCLIUnavailable(mock)
}
