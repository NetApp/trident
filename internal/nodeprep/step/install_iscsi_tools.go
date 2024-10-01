// Copyright 2024 NetApp, Inc. All Rights Reserved.

package step

import (
	"context"

	"github.com/netapp/trident/internal/nodeprep/packagemanager"
)

type InstallIscsiTools struct {
	DefaultStep
	packageManager packagemanager.PackageManager
}

func NewInstallIscsiTools(packageManager packagemanager.PackageManager) *InstallIscsiTools {
	step := &InstallIscsiTools{}
	step.Name = "install iSCSI tools"
	step.Required = true
	step.packageManager = packageManager
	return step
}

func (s *InstallIscsiTools) Apply(ctx context.Context) error {
	return s.packageManager.InstallIscsiRequirements(ctx)
}
