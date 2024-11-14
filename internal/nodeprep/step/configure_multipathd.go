// Copyright 2024 NetApp, Inc. All Rights Reserved.

package step

import (
	"context"
	"fmt"

	"github.com/spf13/afero"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/nodeprep/mpathconfig"
	"github.com/netapp/trident/internal/nodeprep/packagemanager"
)

type (
	NewConfiguration         func(os afero.Afero) (mpathconfig.MpathConfiguration, error)
	NewConfigurationFromFile func(os afero.Afero, filename string) (mpathconfig.MpathConfiguration, error)
)

type MultipathConfigureStep struct {
	DefaultStep
	multipathConfigurationLocation string
	newConfigurationFromFile       NewConfigurationFromFile
	newConfiguration               NewConfiguration
	packageManager                 packagemanager.PackageManager
	os                             afero.Afero
}

func NewMultipathConfigureStep(packageManager packagemanager.PackageManager) *MultipathConfigureStep {
	return NewMultipathConfigureStepDetailed(mpathconfig.MultiPathConfigurationLocation, mpathconfig.NewFromFile,
		mpathconfig.New, packageManager, afero.Afero{Fs: afero.NewOsFs()})
}

func NewMultipathConfigureStepDetailed(multipathConfigurationLocation string, newConfigurationFromFile NewConfigurationFromFile,
	newConfiguration NewConfiguration, packageManager packagemanager.PackageManager, os afero.Afero,
) *MultipathConfigureStep {
	multipathConfigurationStep := &MultipathConfigureStep{
		DefaultStep:                    DefaultStep{Name: "multipath step", Required: true},
		newConfigurationFromFile:       newConfigurationFromFile,
		newConfiguration:               newConfiguration,
		multipathConfigurationLocation: multipathConfigurationLocation,
		packageManager:                 packageManager,
		os:                             os,
	}

	if multipathConfigurationStep.fileExists(config.NamespaceFile) {
		multipathConfigurationStep.multipathConfigurationLocation = fmt.Sprintf("/host%s", multipathConfigurationStep.multipathConfigurationLocation)
	}

	return multipathConfigurationStep
}

func (c *MultipathConfigureStep) Apply(ctx context.Context) error {
	var mpathCfg mpathconfig.MpathConfiguration
	var err error

	if c.packageManager != nil && c.packageManager.MultipathToolsInstalled(ctx) {

		if !c.multipathConfigurationExists() {
			return fmt.Errorf("found multipath tools already installed but no configuration file found; customer" +
				" using multipathd with default configuration")
		}

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

func (c *MultipathConfigureStep) UpdateConfiguration(mpathCfg mpathconfig.MpathConfiguration) error {
	defaultsSection, err := mpathCfg.GetSection(mpathconfig.DefaultsSectionName)
	if err != nil {
		if defaultsSection, err = configureDefaultSection(mpathCfg); err != nil {
			return err
		}
	}

	if !validDefaultsSection(defaultsSection) {
		return fmt.Errorf("current multipath configuration does not  support trident installation" +
			": find_multipaths should be set to no")
	}

	blackListExceptionsSection, err := mpathCfg.GetSection(mpathconfig.BlacklistExceptionsSectionName)
	if err != nil {
		blackListExceptionsSection, err = mpathCfg.GetRootSection().AddSection(mpathconfig.BlacklistExceptionsSectionName)
		if err != nil {
			return err
		}
	}

	if _, err = blackListExceptionsSection.GetDeviceSection("NETAPP", "LUN", ""); err != nil {
		if err = configureBlackListExceptionSection(blackListExceptionsSection); err != nil {
			return err
		}
	}

	return nil
}

func (c *MultipathConfigureStep) AddConfiguration(mpathCfg mpathconfig.MpathConfiguration) error {
	if _, err := configureDefaultSection(mpathCfg); err != nil {
		return err
	}

	blackListSection, err := mpathCfg.GetRootSection().AddSection(mpathconfig.BlacklistSectionName)
	if err != nil {
		return err
	}
	if err = configureBlackListSection(blackListSection); err != nil {
		return err
	}

	blackListExceptionsSection, err := mpathCfg.GetRootSection().AddSection(mpathconfig.BlacklistExceptionsSectionName)
	if err != nil {
		return err
	}
	if err = configureBlackListExceptionSection(blackListExceptionsSection); err != nil {
		return err
	}

	return nil
}

func (c *MultipathConfigureStep) multipathConfigurationExists() bool {
	return c.fileExists(c.multipathConfigurationLocation)
}

func (c *MultipathConfigureStep) fileExists(filename string) bool {
	_, err := c.os.Stat(filename)
	return err == nil
}

func validDefaultsSection(defaultsSection mpathconfig.MpathConfigurationSection) bool {
	if defaultsSection.HasProperty("find_multipaths") {
		value, err := defaultsSection.GetProperty("find_multipaths")
		if err != nil {
			return false
		}
		return value == "no"
	}

	// if the property does not exist, by default it is set to strict
	return false
}

func configureDefaultSection(mpathCfg mpathconfig.MpathConfiguration) (mpathconfig.MpathConfigurationSection, error) {
	defaultsSection, err := mpathCfg.GetRootSection().AddSection(mpathconfig.DefaultsSectionName)
	if err != nil {
		return nil, err
	}
	if err := defaultsSection.SetProperty("find_multipaths", "no"); err != nil {
		return nil, err
	}
	return defaultsSection, nil
}

func configureBlackListExceptionSection(blackListExceptionsSection mpathconfig.MpathConfigurationSection) error {
	netappDevice, err := blackListExceptionsSection.AddSection(mpathconfig.DeviceSectionName)
	if err != nil {
		return err
	}

	if err := netappDevice.SetProperty("vendor", "NETAPP"); err != nil {
		return err
	}
	if err := netappDevice.SetProperty("product", "LUN"); err != nil {
		return err
	}
	return nil
}

func configureBlackListSection(blackListSection mpathconfig.MpathConfigurationSection) error {
	allDevice, err := blackListSection.AddSection(mpathconfig.DeviceSectionName)
	if err != nil {
		return err
	}
	if err := allDevice.SetProperty("vendor", ".*"); err != nil {
		return err
	}
	if err := allDevice.SetProperty("product", ".*"); err != nil {
		return err
	}
	return nil
}
