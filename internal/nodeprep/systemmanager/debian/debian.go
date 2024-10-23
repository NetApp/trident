// Copyright 2024 NetApp, Inc. All Rights Reserved.

package debian

import (
	"context"
	"time"

	"github.com/netapp/trident/internal/nodeprep/systemmanager/systemctl"
	"github.com/netapp/trident/utils/exec"
)

const (
	ServiceIscsid         = "iscsid"
	ServiceMultipathtools = "multipathd"
)

const (
	defaultCommandTimeout   = 10 * time.Second
	defaultLogCommandOutput = true
)

type Debian struct {
	*systemctl.Systemctl
}

func New() *Debian {
	return NewDetailed(exec.NewCommand(), defaultCommandTimeout, defaultLogCommandOutput)
}

func NewDetailed(command exec.Command, commandTimeout time.Duration, logCommandOutput bool) *Debian {
	return &Debian{
		Systemctl: systemctl.NewSystemctlDetailed(command, commandTimeout, logCommandOutput),
	}
}

func (d *Debian) EnableIscsiServices(ctx context.Context) error {
	if err := d.EnableServiceWithValidation(ctx, ServiceIscsid); err != nil {
		return err
	}
	if err := d.EnableServiceWithValidation(ctx, ServiceMultipathtools); err != nil {
		return err
	}
	return nil
}
