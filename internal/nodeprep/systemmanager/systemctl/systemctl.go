// Copyright 2024 NetApp, Inc. All Rights Reserved.

package systemctl

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/exec"
)

const activeStateActive = "ActiveState=active"

type Systemctl struct {
	command          exec.Command
	commandTimeout   time.Duration
	logCommandOutput bool
}

func NewSystemctlDetailed(command exec.Command, commandTimeout time.Duration, logCommandOutput bool) *Systemctl {
	return &Systemctl{
		command:          command,
		commandTimeout:   commandTimeout,
		logCommandOutput: logCommandOutput,
	}
}

func (s *Systemctl) EnableServiceWithValidation(ctx context.Context, serviceName string) error {
	output, err := s.enableService(ctx, serviceName)
	if err != nil {
		return errors.New(fmt.Sprintf("failed to enable %s: %v", serviceName, err))
	}

	Log().WithFields(LogFields{
		"service": serviceName,
		"output":  output,
	}).Debug("activate output for package")

	if err = s.validateServiceEnabled(ctx, serviceName); err != nil {
		return err
	}

	Log().WithField("serviceName", serviceName).Info("Successfully enabled service")

	return nil
}

func (s *Systemctl) validateServiceEnabled(ctx context.Context, serviceName string) error {
	output, err := s.isServiceActive(ctx, serviceName)
	if err != nil {
		return errors.New(fmt.Sprintf("failed to validate if %s service is enabled: %s", serviceName, err))
	}

	if !strings.Contains(output, activeStateActive) {
		return errors.New(fmt.Sprintf("failed to validate if %s service is enabled: %s", serviceName, output))
	}

	return nil
}

func (s *Systemctl) enableService(ctx context.Context, serviceName string) (string, error) {
	output, err := s.command.ExecuteWithTimeout(ctx, "systemctl", s.commandTimeout, s.logCommandOutput, "enable",
		"--now", serviceName)

	return string(output), err
}

func (s *Systemctl) isServiceActive(ctx context.Context, serviceName string) (string, error) {
	output, err := s.command.ExecuteWithTimeout(ctx, "systemctl", s.commandTimeout, s.logCommandOutput, "show",
		serviceName, "--property=ActiveState")

	return string(output), err
}
