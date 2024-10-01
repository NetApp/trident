// Copyright 2024 NetApp, Inc. All Rights Reserved.

package yum_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/internal/nodeprep/packagemanager/yum"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/errors"
)

func TestNew(t *testing.T) {
	yumPackageManager := yum.New()
	assert.NotNil(t, yumPackageManager)
}

func TestNewDetailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(ctrl)
	yumPackageManager := yum.NewDetailed(mockCommand, 1*time.Second, false)
	assert.NotNil(t, yumPackageManager)
}

func TestYum_MultipathToolsInstalled(t *testing.T) {
	type parameters struct {
		getCommand     func(controller *gomock.Controller) *mockexec.MockCommand
		assertResponse assert.BoolAssertionFunc
	}
	const timeout = 1 * time.Second
	const logCommandOutput = false
	const alreadyInstalledOutput = "foo baz " + yum.PackageDeviceMapperMultipath + " foo baz"
	const notInstalledOutput = "foo baz foo baz"

	tests := map[string]parameters{
		"error executing command": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageDeviceMapperMultipath).Return(nil, errors.UnsupportedError("some error"))
				return mockCommand
			},
			assertResponse: assert.False,
		},
		"multipath tools already installed": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageDeviceMapperMultipath).Return([]byte(alreadyInstalledOutput), nil)
				return mockCommand
			},
			assertResponse: assert.True,
		},
		"multipath tools not installed": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageDeviceMapperMultipath).Return([]byte(notInstalledOutput), nil)
				return mockCommand
			},
			assertResponse: assert.False,
		},
	}

	ctx := context.Background()

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			yumPackageManager := yum.NewDetailed(params.getCommand(ctrl), timeout, logCommandOutput)

			response := yumPackageManager.MultipathToolsInstalled(ctx)
			if params.assertResponse != nil {
				params.assertResponse(t, response)
			}
		})
	}
}

func TestApt_InstallIscsiRequirements(t *testing.T) {
	type parameters struct {
		getCommand  func(controller *gomock.Controller) *mockexec.MockCommand
		assertError assert.ErrorAssertionFunc
	}

	const timeout = 1 * time.Second
	const logCommandOutput = false
	const successfulpackageInstallOutput = "foo baz %s foo baz"
	const failedPackageInstallOutput = "foo baz"

	tests := map[string]parameters{
		"happy path: error installing lsscsi": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageLsscsi).Return(nil, errors.New("some error"))
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageIscsiInitiatorUtils).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageIscsiInitiatorUtils).Return([]byte(fmt.Sprintf(
					successfulpackageInstallOutput, yum.PackageIscsiInitiatorUtils)), nil)

				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageDeviceMapperMultipath).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageDeviceMapperMultipath).Return([]byte(fmt.Sprintf(
					successfulpackageInstallOutput, yum.PackageDeviceMapperMultipath)), nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"happy path: error getting lsscsi package info": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageLsscsi).Return(nil, errors.New("some error"))
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageIscsiInitiatorUtils).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageIscsiInitiatorUtils).Return([]byte(fmt.Sprintf(
					successfulpackageInstallOutput, yum.PackageIscsiInitiatorUtils)), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageDeviceMapperMultipath).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageDeviceMapperMultipath).Return([]byte(fmt.
					Sprintf(successfulpackageInstallOutput, yum.PackageDeviceMapperMultipath)), nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"happy path: failure installing lsscsi": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageLsscsi).Return([]byte(failedPackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageIscsiInitiatorUtils).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageIscsiInitiatorUtils).Return([]byte(fmt.Sprintf(
					successfulpackageInstallOutput, yum.PackageIscsiInitiatorUtils)), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageDeviceMapperMultipath).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageDeviceMapperMultipath).Return([]byte(fmt.Sprintf(
					successfulpackageInstallOutput, yum.PackageDeviceMapperMultipath)), nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"happy path": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageLsscsi).Return([]byte(fmt.Sprintf(successfulpackageInstallOutput, yum.PackageLsscsi)), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageIscsiInitiatorUtils).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageIscsiInitiatorUtils).Return([]byte(fmt.Sprintf(
					successfulpackageInstallOutput, yum.PackageIscsiInitiatorUtils)), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageDeviceMapperMultipath).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageDeviceMapperMultipath).Return([]byte(fmt.Sprintf(
					successfulpackageInstallOutput, yum.PackageDeviceMapperMultipath)), nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"error installing open-iscsi package": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageLsscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageIscsiInitiatorUtils).Return(nil, errors.New("some error"))
				return mockCommand
			},
			assertError: assert.Error,
		},
		"error getting open-iscsi package info": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageLsscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageIscsiInitiatorUtils).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageIscsiInitiatorUtils).Return(nil, errors.New("some error"))
				return mockCommand
			},
			assertError: assert.Error,
		},
		"failure installing open-iscsi package": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageLsscsi).Return([]byte(fmt.Sprintf(successfulpackageInstallOutput,
					yum.PackageLsscsi)), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageIscsiInitiatorUtils).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageIscsiInitiatorUtils).Return([]byte(failedPackageInstallOutput), nil)
				return mockCommand
			},
			assertError: assert.Error,
		},
		"error installing multipath-tools package": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageLsscsi).Return([]byte(fmt.Sprintf(successfulpackageInstallOutput,
					yum.PackageLsscsi)), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageIscsiInitiatorUtils).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageIscsiInitiatorUtils).Return([]byte(fmt.Sprintf(
					successfulpackageInstallOutput, yum.PackageIscsiInitiatorUtils)), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageDeviceMapperMultipath).Return(nil, errors.New("some error"))
				return mockCommand
			},
			assertError: assert.Error,
		},
		"error getting multipath-tools package info": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageLsscsi).Return([]byte(fmt.Sprintf(successfulpackageInstallOutput,
					yum.PackageLsscsi)), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageIscsiInitiatorUtils).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageIscsiInitiatorUtils).Return([]byte(fmt.Sprintf(
					successfulpackageInstallOutput, yum.PackageIscsiInitiatorUtils)), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageDeviceMapperMultipath).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageDeviceMapperMultipath).Return(nil, errors.New("some error"))
				return mockCommand
			},
			assertError: assert.Error,
		},
		"failure installing multipath-tools package": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageLsscsi).Return([]byte(fmt.Sprintf(successfulpackageInstallOutput,
					yum.PackageLsscsi)), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageIscsiInitiatorUtils).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageIscsiInitiatorUtils).Return([]byte(fmt.Sprintf(
					successfulpackageInstallOutput, yum.PackageIscsiInitiatorUtils)),
					nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "install",
					"-y", yum.PackageDeviceMapperMultipath).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "yum", timeout, logCommandOutput, "info",
					"--installed", yum.PackageDeviceMapperMultipath).Return([]byte(failedPackageInstallOutput), nil)
				return mockCommand
			},
			assertError: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			yunPackageManager := yum.NewDetailed(params.getCommand(ctrl), timeout, logCommandOutput)

			err := yunPackageManager.InstallIscsiRequirements(context.TODO())
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}
