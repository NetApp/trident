// Copyright 2024 NetApp, Inc. All Rights Reserved.

package amzn

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

type Amzn struct {
	*systemctl.Systemctl
}

func New() *Amzn {
	return NewDetailed(systemctl.NewSystemctlDetailed(exec.NewCommand(), defaultCommandTimeout, defaultLogCommandOutput))
}

func NewDetailed(systemctl *systemctl.Systemctl) *Amzn {
	return &Amzn{
		Systemctl: systemctl,
	}
}

func (a *Amzn) EnableIscsiServices(ctx context.Context) error {
	if err := a.EnableServiceWithValidation(ctx, ServiceIscsid); err != nil {
		return err
	}
	if err := a.EnableServiceWithValidation(ctx, ServiceMultipathd); err != nil {
		return err
	}
	return nil
}
