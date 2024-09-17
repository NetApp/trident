// Copyright 2024 NetApp, Inc. All Rights Reserved.

package stage

import (
	"fmt"
	"os"

	"github.com/netapp/trident/internal/nodeprep/mpathconfig"
	"github.com/netapp/trident/internal/nodeprep/packagemanager"
)

type (
	NewConfiguration         func() (mpathconfig.MpathConfiguration, error)
	NewConfigurationFromFile func(filename string) (mpathconfig.MpathConfiguration, error)
)

type Configurator struct {
	multipathConfigurationLocation string
	newConfigurationFromFile       NewConfigurationFromFile
	newConfiguration               NewConfiguration
	packageManager                 packagemanager.PackageManager
}

func NewConfigurator() *Configurator {
	return NewConfiguratorDetailed(mpathconfig.MultiPathConfigurationLocation, mpathconfig.NewFromFile,
		mpathconfig.New, packagemanager.Factory())
}

func NewConfiguratorDetailed(multipathConfigurationLocation string, newConfigurationFromFile NewConfigurationFromFile,
	newConfiguration NewConfiguration, packageManager packagemanager.PackageManager,
) *Configurator {
	return &Configurator{
		newConfigurationFromFile:       newConfigurationFromFile,
		newConfiguration:               newConfiguration,
		multipathConfigurationLocation: multipathConfigurationLocation,
		packageManager:                 packageManager,
	}
}

func (c *Configurator) ConfigureMultipathDaemon() error {
	var mpathCfg mpathconfig.MpathConfiguration
	var err error
	if c.packageManager.MultipathToolsInstalled() {

		if !c.multipathConfigurationExists() {
			return fmt.Errorf("found multipath tools already installed but no configuration file found; customer" +
				" using multipathd with default configuration")
		}

		mpathCfg, err = c.newConfigurationFromFile(c.multipathConfigurationLocation)
		if err != nil {
			return err
		}

		if err = c.UpdateConfiguration(mpathCfg); err != nil {
			return err
		}

	} else {
		mpathCfg, err = c.newConfiguration()
		if err != nil {
			return err
		}

		if err = c.AddConfiguration(mpathCfg); err != nil {
			return err
		}
	}

	return mpathCfg.SaveConfig(c.multipathConfigurationLocation)
}

func (c *Configurator) UpdateConfiguration(mpathCfg mpathconfig.MpathConfiguration) error {
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

func (c *Configurator) AddConfiguration(mpathCfg mpathconfig.MpathConfiguration) error {
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

func (c *Configurator) multipathConfigurationExists() bool {
	_, err := os.Stat(c.multipathConfigurationLocation)
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
