// Copyright 2024 NetApp, Inc. All Rights Reserved.

package yum

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/exec"
)

const (
	PackageLsscsi                = "lsscsi"
	PackageIscsiInitiatorUtils   = "iscsi-initiator-utils"
	PackageDeviceMapperMultipath = "device-mapper-multipath"
)

const (
	defaultCommandTimeout   = 10 * time.Second
	defaultLogCommandOutput = true
)

type Yum struct {
	command          exec.Command
	commandTimeout   time.Duration
	logCommandOutput bool
}

func New() *Yum {
	return NewDetailed(exec.NewCommand(), defaultCommandTimeout, defaultLogCommandOutput)
}

func NewDetailed(command exec.Command, commandTimeout time.Duration, logCommandOutput bool) *Yum {
	return &Yum{
		command:          command,
		commandTimeout:   commandTimeout,
		logCommandOutput: logCommandOutput,
	}
}

func (y *Yum) MultipathToolsInstalled(ctx context.Context) bool {
	err := y.validatePackageInstall(ctx, PackageDeviceMapperMultipath)
	return err == nil
}

func (y *Yum) InstallIscsiRequirements(ctx context.Context) error {
	if err := y.installPackageWithValidation(ctx, PackageLsscsi); err != nil {
		Log().WithError(err).Error("Failed to install lsscsi package, continuing installation")
	}
	if err := y.installPackageWithValidation(ctx, PackageIscsiInitiatorUtils); err != nil {
		return err
	}
	if err := y.installPackageWithValidation(ctx, PackageDeviceMapperMultipath); err != nil {
		return err
	}
	return nil
}

func (y *Yum) installPackageWithValidation(ctx context.Context, packageName string) error {
	output, err := y.installPackage(ctx, packageName)
	if err != nil {
		return fmt.Errorf("failed to install %s package: %v", packageName, err)
	}

	Log().WithFields(LogFields{
		"package": packageName,
		"output":  output,
	}).Debug("install output for package")

	if err = y.validatePackageInstall(ctx, packageName); err != nil {
		return err
	}

	Log().WithField("packageName", packageName).Info("Successfully installed package")
	return nil
}

func (y *Yum) validatePackageInstall(ctx context.Context, packageName string) error {
	output, err := y.getPackageInfo(ctx, packageName)
	if err != nil {
		return fmt.Errorf("failed to validate %s package install: %s", packageName, err)
	}

	if !strings.Contains(output, packageName) {
		return fmt.Errorf("failed to install %s package : %s", packageName, output)
	}

	return nil
}

func (y *Yum) installPackage(ctx context.Context, packageName string) (string, error) {
	output, err := y.command.ExecuteWithTimeout(ctx, "yum",
		y.commandTimeout, y.logCommandOutput, "install", "-y", packageName)
	return string(output), err
}

func (y *Yum) getPackageInfo(ctx context.Context, packageName string) (string, error) {
	output, err := y.command.ExecuteWithTimeout(ctx, "yum",
		y.commandTimeout, y.logCommandOutput, "info", "--installed", packageName)
	return string(output), err
}
