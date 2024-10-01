// Copyright 2024 NetApp, Inc. All Rights Reserved.

package apt_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/internal/nodeprep/packagemanager/apt"
	mockexec "github.com/netapp/trident/mocks/mock_utils/mock_exec"
	"github.com/netapp/trident/utils/errors"
)

func TestNew(t *testing.T) {
	aptPackageManager := apt.New()
	assert.NotNil(t, aptPackageManager)
}

func TestNewDetailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockCommand := mockexec.NewMockCommand(ctrl)
	aptPackageManager := apt.NewDetailed(mockCommand, 1*time.Second, false)
	assert.NotNil(t, aptPackageManager)
}

func TestApt_MultipathToolsInstalled(t *testing.T) {
	type parameters struct {
		getCommand     func(controller *gomock.Controller) *mockexec.MockCommand
		assertResponse assert.BoolAssertionFunc
	}
	const timeout = 1 * time.Second
	const logCommandOutput = false
	const alreadyInstalledOutput = "foo baz"
	const notInstalledOutput = "Unable to locate package " + apt.PackageMultipathTools

	tests := map[string]parameters{
		"error executing command": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageMultipathTools).Return(nil, errors.New("some error"))
				return mockCommand
			},
			assertResponse: assert.False,
		},
		"multipath tools already installed": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageMultipathTools).Return([]byte(alreadyInstalledOutput), nil)
				return mockCommand
			},
			assertResponse: assert.True,
		},
		"multipath tools not installed": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(gomock.Any(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageMultipathTools).Return([]byte(notInstalledOutput), nil)
				return mockCommand
			},
			assertResponse: assert.False,
		},
	}

	ctx := context.Background()

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			aptPackageManager := apt.NewDetailed(params.getCommand(ctrl), timeout, logCommandOutput)

			response := aptPackageManager.MultipathToolsInstalled(ctx)
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
	const successfulpackageInstallOutput = "foo baz"
	const failedPackageInstallOutput = "Unable to locate package %s"

	tests := map[string]parameters{
		"happy path: error installing lsscsi": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageLsscsi).Return(nil, errors.New("some error"))
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageOpenIscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageOpenIscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageMultipathTools).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageMultipathTools).Return([]byte(successfulpackageInstallOutput), nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"happy path: error getting lsscsi package info": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageLsscsi).Return(nil, errors.New("some error"))
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageOpenIscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageOpenIscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageMultipathTools).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageMultipathTools).Return([]byte(successfulpackageInstallOutput), nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"happy path: failure installing lsscsi": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageLsscsi).Return([]byte(fmt.Sprintf(failedPackageInstallOutput, apt.PackageLsscsi)), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageOpenIscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageOpenIscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageMultipathTools).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageMultipathTools).Return([]byte(successfulpackageInstallOutput), nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"happy path": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageLsscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageLsscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageOpenIscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageOpenIscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageMultipathTools).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageMultipathTools).Return([]byte(successfulpackageInstallOutput), nil)
				return mockCommand
			},
			assertError: assert.NoError,
		},
		"error installing open-iscsi package": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageLsscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageOpenIscsi).Return(nil, errors.New("some error"))
				return mockCommand
			},
			assertError: assert.Error,
		},
		"error getting open-iscsi package info": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageLsscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageOpenIscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageOpenIscsi).Return(nil, errors.New("some error"))
				return mockCommand
			},
			assertError: assert.Error,
		},
		"failure installing open-iscsi package": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageLsscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageOpenIscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageOpenIscsi).Return([]byte(fmt.Sprintf(failedPackageInstallOutput, apt.PackageOpenIscsi)), nil)
				return mockCommand
			},
			assertError: assert.Error,
		},
		"error installing multipath-tools package": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageLsscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageOpenIscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageOpenIscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageMultipathTools).Return(nil, errors.New("some error"))
				return mockCommand
			},
			assertError: assert.Error,
		},
		"error getting multipath-tools package info": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageLsscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageOpenIscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageOpenIscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageMultipathTools).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageMultipathTools).Return(nil, errors.New("some error"))
				return mockCommand
			},
			assertError: assert.Error,
		},
		"failure installing multipath-tools package": {
			getCommand: func(controller *gomock.Controller) *mockexec.MockCommand {
				mockCommand := mockexec.NewMockCommand(controller)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageLsscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageLsscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageOpenIscsi).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageOpenIscsi).Return([]byte(successfulpackageInstallOutput), nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "install",
					"-y", apt.PackageMultipathTools).Return(nil, nil)
				mockCommand.EXPECT().ExecuteWithTimeout(context.TODO(), "apt", timeout, logCommandOutput, "list",
					"--installed", apt.PackageMultipathTools).Return([]byte(fmt.Sprintf(failedPackageInstallOutput, apt.PackageMultipathTools)), nil)
				return mockCommand
			},
			assertError: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			aptPackageManager := apt.NewDetailed(params.getCommand(ctrl), timeout, logCommandOutput)

			err := aptPackageManager.InstallIscsiRequirements(context.TODO())
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}
