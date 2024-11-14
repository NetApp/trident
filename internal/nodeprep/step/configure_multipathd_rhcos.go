// Copyright 2024 NetApp, Inc. All Rights Reserved.

package step

import (
	"context"

	"github.com/spf13/afero"

	"github.com/netapp/trident/internal/nodeprep/mpathconfig"
)

type MultipathConfigureRHCOSStep struct {
	MultipathConfigureStep
}

func NewMultipathConfigureRHCOSStep() *MultipathConfigureRHCOSStep {
	return NewMultipathConfigureRHCOSStepDetailed(
		mpathconfig.MultiPathConfigurationLocation,
		mpathconfig.NewFromFile,
		mpathconfig.New,
		afero.Afero{Fs: afero.NewOsFs()})
}

func NewMultipathConfigureRHCOSStepDetailed(multipathConfigurationLocation string, newConfigurationFromFile NewConfigurationFromFile,
	newConfiguration NewConfiguration, os afero.Afero,
) *MultipathConfigureRHCOSStep {
	multipathConfigurationStep := &MultipathConfigureRHCOSStep{
		MultipathConfigureStep: *NewMultipathConfigureStepDetailed(
			multipathConfigurationLocation,
			newConfigurationFromFile,
			newConfiguration,
			nil,
			os),
	}

	return multipathConfigurationStep
}

func (c *MultipathConfigureRHCOSStep) Apply(ctx context.Context) error {
	var mpathCfg mpathconfig.MpathConfiguration
	var err error
	if c.multipathConfigurationExists() {
		mpathCfg, err = c.newConfigurationFromFile(c.os, c.multipathConfigurationLocation)
		if err != nil {
			return err
		}

		if err = c.UpdateConfiguration(mpathCfg); err != nil {
			return err
		}
	} else {
		mpathCfg, err = c.newConfiguration(c.os)
		if err != nil {
			return err
		}

		if err = c.AddConfiguration(mpathCfg); err != nil {
			return err
		}
	}

	return mpathCfg.SaveConfig(c.multipathConfigurationLocation)
}
