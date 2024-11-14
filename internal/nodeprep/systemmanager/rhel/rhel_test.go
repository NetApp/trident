// Copyright 2024 NetApp, Inc. All Rights Reserved.

package rhel_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/internal/nodeprep/systemmanager/rhel"
	"github.com/netapp/trident/internal/nodeprep/systemmanager/systemctl"
	"github.com/netapp/trident/mocks/mock_utils/mock_exec"
)

func TestNew(t *testing.T) {
	RHELClient := rhel.New()
	assert.NotNil(t, RHELClient)
}

func TestNewDetailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	command := mock_exec.NewMockCommand(ctrl)
	RHELClient := rhel.NewDetailed(systemctl.NewSystemctlDetailed(command, 1*time.Second, true))
	assert.NotNil(t, RHELClient)
}

func TestRHEL_EnableIscsiServices(t *testing.T) {
	type parameters struct {
		getCommand  func(controller *gomock.Controller) *mock_exec.MockCommand
		assertError assert.ErrorAssertionFunc
	}

	const commandTimeout = 1 * time.Second
	const logCommandOutput = true
	const activeStateActive = "ActiveState=active\n"
	const activeStateInactive = "ActiveState=inactive\n"

	tests := map[string]parameters{
		"error enabling iscsi service": {
			getCommand: func(controller *gomock.Controller) *mock_exec.MockCommand {
				command := mock_exec.NewMockCommand(controller)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "enable", "--now", rhel.ServiceIscsid).Return(nil, assert.AnError)
				return command
			},
			assertError: assert.Error,
		},
		"error validating that iscsi service is enabled": {
			getCommand: func(controller *gomock.Controller) *mock_exec.MockCommand {
				command := mock_exec.NewMockCommand(controller)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "enable", "--now", rhel.ServiceIscsid).Return(nil, nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "show", rhel.ServiceIscsid, "--property=ActiveState").Return(nil, assert.AnError)
				return command
			},
			assertError: assert.Error,
		},
		"failure validating that iscsi service is enabled": {
			getCommand: func(controller *gomock.Controller) *mock_exec.MockCommand {
				command := mock_exec.NewMockCommand(controller)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "enable", "--now", rhel.ServiceIscsid).Return(nil, nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "show", rhel.ServiceIscsid, "--property=ActiveState").Return([]byte(activeStateInactive), nil)
				return command
			},
			assertError: assert.Error,
		},
		"error enabling  multipath service is enabled": {
			getCommand: func(controller *gomock.Controller) *mock_exec.MockCommand {
				command := mock_exec.NewMockCommand(controller)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "enable", "--now", rhel.ServiceIscsid).Return(nil, nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "show", rhel.ServiceIscsid, "--property=ActiveState").Return([]byte(activeStateActive), nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "enable", "--now", rhel.ServiceMultipathd).Return(nil, assert.AnError)
				return command
			},
			assertError: assert.Error,
		},
		"error validating that multipath service is enabled": {
			getCommand: func(controller *gomock.Controller) *mock_exec.MockCommand {
				command := mock_exec.NewMockCommand(controller)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "enable", "--now", rhel.ServiceIscsid).Return(nil, nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "show", rhel.ServiceIscsid, "--property=ActiveState").Return([]byte(activeStateActive), nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "enable", "--now", rhel.ServiceMultipathd).Return(nil, nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "show", rhel.ServiceMultipathd, "--property=ActiveState").Return(nil, assert.AnError)
				return command
			},
			assertError: assert.Error,
		},
		"failure validating that multipath service is enabled": {
			getCommand: func(controller *gomock.Controller) *mock_exec.MockCommand {
				command := mock_exec.NewMockCommand(controller)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "enable", "--now", rhel.ServiceIscsid).Return(nil, nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "show", rhel.ServiceIscsid, "--property=ActiveState").Return([]byte(activeStateActive), nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "enable", "--now", rhel.ServiceMultipathd).Return(nil, nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "show", rhel.ServiceMultipathd, "--property=ActiveState").Return([]byte(activeStateInactive), nil)
				return command
			},
			assertError: assert.Error,
		},
		"happy path": {
			getCommand: func(controller *gomock.Controller) *mock_exec.MockCommand {
				command := mock_exec.NewMockCommand(controller)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "enable", "--now", rhel.ServiceIscsid).Return(nil, nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "show", rhel.ServiceIscsid, "--property=ActiveState").Return([]byte(activeStateActive), nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "enable", "--now", rhel.ServiceMultipathd).Return(nil, nil)
				command.EXPECT().ExecuteWithTimeout(context.TODO(), "systemctl", commandTimeout,
					logCommandOutput, "show", rhel.ServiceMultipathd, "--property=ActiveState").Return([]byte(activeStateActive), nil)
				return command
			},
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			RHELClient := rhel.NewDetailed(systemctl.NewSystemctlDetailed(params.getCommand(ctrl), commandTimeout, logCommandOutput))
			err := RHELClient.EnableIscsiServices(context.TODO())
			if params.assertError(t, err) {
				params.assertError(t, err)
			}
		})
	}
}
