// Copyright 2024 NetApp, Inc. All Rights Reserved.

package apt

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
)

const (
	PackageLsscsi         = "lsscsi"
	PackageOpenIscsi      = "open-iscsi"
	PackageMultipathTools = "multipath-tools"
)

const (
	defaultCommandTimeout   = time.Minute
	defaultLogCommandOutput = true
)

type Apt struct {
	command          exec.Command
	commandTimeout   time.Duration
	logCommandOutput bool
}

func New() *Apt {
	return NewDetailed(exec.NewCommand(), defaultCommandTimeout, defaultLogCommandOutput)
}

func NewDetailed(command exec.Command, commandTimeout time.Duration, logCommandOutput bool) *Apt {
	return &Apt{
		command:          command,
		commandTimeout:   commandTimeout,
		logCommandOutput: logCommandOutput,
	}
}

func (a *Apt) MultipathToolsInstalled(ctx context.Context) bool {
	err := a.validatePackageInstall(ctx, PackageMultipathTools)
	return err == nil
}

func (a *Apt) InstallIscsiRequirements(ctx context.Context) error {
	if err := a.installPackageWithValidation(ctx, PackageLsscsi); err != nil {
		Log().WithError(err).Error("Failed to install lsscsi package, continuing installation.")
	}
	if err := a.installPackageWithValidation(ctx, PackageOpenIscsi); err != nil {
		return err
	}
	if err := a.installPackageWithValidation(ctx, PackageMultipathTools); err != nil {
		return err
	}
	return nil
}

func (a *Apt) installPackageWithValidation(ctx context.Context, packageName string) error {
	output, err := a.installPackage(ctx, packageName)
	if err != nil {
		return errors.New(fmt.Sprintf("failed to install %s package: %v", packageName, err))
	}

	Log().WithFields(LogFields{
		"package": packageName,
		"output":  output,
	}).Debug("Installed output for package.")

	if err = a.validatePackageInstall(ctx, packageName); err != nil {
		return err
	}

	Log().WithField("packageName", packageName).Info("Successfully installed package.")
	return nil
}

func (a *Apt) validatePackageInstall(ctx context.Context, packageName string) error {
	output, err := a.getPackageInfo(ctx, packageName)
	if err != nil {
		return errors.New(fmt.Sprintf("failed to validate %s package install: %s", packageName, output))
	}

	if !strings.Contains(output, packageName) {
		return errors.New(fmt.Sprintf("failed to install %s package : %s", packageName, output))
	}

	return nil
}

func (a *Apt) installPackage(ctx context.Context, packageName string) (string, error) {
	output, err := a.command.ExecuteWithTimeout(ctx, "apt",
		a.commandTimeout, a.logCommandOutput, "install", "-y", packageName)
	if err != nil && (errors.IsTimeoutError(err) || err.Error() == "exit status 100") {
		// attempt fix and let k8s handle the retry
		out, fixErr := a.command.ExecuteWithTimeout(ctx, "dpkg", a.commandTimeout, a.logCommandOutput, "--configure", "--pending")
		if fixErr != nil {
			Log().WithError(fixErr).WithField("output", string(out)).Error("Failed fix using dpkg")
		}
	}
	return string(output), err
}

func (a *Apt) getPackageInfo(ctx context.Context, packageName string) (string, error) {
	output, err := a.command.ExecuteWithTimeout(ctx, "apt",
		a.commandTimeout, a.logCommandOutput, "list", "--installed", packageName)
	return string(output), err
}
