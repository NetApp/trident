// Copyright 2024 NetApp, Inc. All Rights Reserved.

package rhel

import (
	"context"
	"time"

	"github.com/netapp/trident/internal/nodeprep/systemmanager/systemctl"
	"github.com/netapp/trident/utils/exec"
)

const (
	ServiceIscsid     = "iscsid"
	ServiceMultipathd = "multipathd"
)

const (
	defaultCommandTimeout   = 10 * time.Second
	defaultLogCommandOutput = true
)

type RHEL struct {
	*systemctl.Systemctl
}

func New() *RHEL {
	return NewDetailed(systemctl.NewSystemctlDetailed(exec.NewCommand(), defaultCommandTimeout, defaultLogCommandOutput))
}

func NewDetailed(systemctl *systemctl.Systemctl) *RHEL {
	return &RHEL{
		Systemctl: systemctl,
	}
}

func (a *RHEL) EnableIscsiServices(ctx context.Context) error {
	if err := a.EnableServiceWithValidation(ctx, ServiceIscsid); err != nil {
		return err
	}
	if err := a.EnableServiceWithValidation(ctx, ServiceMultipathd); err != nil {
		return err
	}
	return nil
}
