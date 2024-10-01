// Copyright 2024 NetApp, Inc. All Rights Reserved.

package systemctl_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/internal/nodeprep/systemmanager/systemctl"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
)

func TestNewSystemctlDetailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	command := mockexec.NewMockCommand(ctrl)
	systemctlClient := systemctl.NewSystemctlDetailed(command, 1*time.Second, true)
	assert.NotNil(t, systemctlClient)
}

func TestSystemctl_EnableServiceWithValidation(t *testing.T) {
	type parameters struct {
		getCommand  func(controller *gomock.Controller) *mockexec.MockCommand
		assertError assert.ErrorAssertionFunc
	}

	const commandTimeout = 1 * time.Second
	const logCommandOutput = true
	const serviceName = "foo"
	const activeStateInactive = "ActiveState=inactive\n"
	const activeStateActive = "ActiveState=active\n"

	tests := map[string]parameters{
		"error enabling service": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				command := mockexec.NewMockCommand(controller)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout, logCommandOutput,
					"enable", "--now", serviceName).Return(nil, assert.AnError)
				return command
			},
			assertError: assert.Error,
		},
		"error determining if service is active": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				command := mockexec.NewMockCommand(controller)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout, logCommandOutput,
					"enable", "--now", serviceName).Return(nil, nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout, logCommandOutput,
					"show", serviceName, "--property=ActiveState").Return(nil, assert.AnError)
				return command
			},
			assertError: assert.Error,
		},
		"unable to enable service": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				command := mockexec.NewMockCommand(controller)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout, logCommandOutput,
					"enable", "--now", serviceName).Return(nil, nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout, logCommandOutput,
					"show", serviceName, "--property=ActiveState").Return([]byte(activeStateInactive), nil)
				return command
			},
			assertError: assert.Error,
		},
		"happy path": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				command := mockexec.NewMockCommand(controller)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout, logCommandOutput,
					"enable", "--now", serviceName).Return(nil, nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout, logCommandOutput,
					"show", serviceName, "--property=ActiveState").Return([]byte(activeStateActive), nil)
				return command
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			systemctlClient := systemctl.NewSystemctlDetailed(params.getCommand(ctrl), commandTimeout, logCommandOutput)
			err := systemctlClient.EnableServiceWithValidation(context.TODO(), serviceName)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}
