// Copyright 2024 NetApp, Inc. All Rights Reserved.

package mpathconfig

//go:generate mockgen -destination=../../../mocks/mock_nodeprep/mock_mpathconfig/mock_config.go  github.com/netapp/trident/internal/nodeprep/mpathconfig MpathConfiguration

import (
	"bufio"
	"fmt"
	"os"

	"github.com/hpe-storage/common-host-libs/mpathconfig"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

const (
	MultiPathConfigurationLocation = mpathconfig.MPATHCONF
)

type MpathConfiguration interface {
	GetRootSection() MpathConfigurationSection
	GetSection(sectionName string) (MpathConfigurationSection, error)
	PrintConf() []string
	SaveConfig(filePath string) error
}

var _ MpathConfiguration = &Configuration{}

type Configuration struct {
	configuration *mpathconfig.Configuration
}

func (c *Configuration) GetRootSection() MpathConfigurationSection {
	return &Section{configuration: c.configuration, section: c.configuration.GetRoot()}
}

func (c *Configuration) GetSection(sectionName string) (MpathConfigurationSection, error) {
	if c.configuration == nil {
		return nil, errors.UnsupportedError(fmt.Sprintf("error getting section %s: empty configuration", sectionName))
	}

	section, err := c.configuration.GetSection(sectionName, "")
	if err != nil {
		return nil, errors.NotFoundError("error getting section %s: %v", sectionName, err)
	}
	return &Section{configuration: c.configuration, section: section}, nil
}

func (c *Configuration) PrintConf() []string {
	if c.configuration == nil {
		return nil
	}
	return c.configuration.PrintConf()
}

func (c *Configuration) SaveConfig(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}

	defer func() {
		_ = f.Close()
	}()

	s := c.PrintConf()
	w := bufio.NewWriter(f)
	for _, line := range s {
		if _, err = w.WriteString(line); err != nil {
			Log().WithField("fileName", filePath).WithError(err).Error("Error writing to multipath configuration file")
		}
	}
	if err = w.Flush(); err != nil {
		Log().WithField("fileName", filePath).WithError(err).Error("Error flushing multipath configuration file")
		return errors.New(fmt.Sprintf("error flushing multipath configuration file: %v", err))
	}

	return err
}

func New() (MpathConfiguration, error) {
	return NewFromFile(os.DevNull)
}

func NewFromFile(filename string) (MpathConfiguration, error) {
	mpathCfg, err := mpathconfig.ParseConfig(filename)
	if err != nil {
		return nil, fmt.Errorf("error creating mpath config: %v", err)
	}
	return &Configuration{configuration: mpathCfg}, nil
}
