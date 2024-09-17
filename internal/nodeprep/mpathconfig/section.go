// Copyright 2024 NetApp, Inc. All Rights Reserved.

package mpathconfig

//go:generate mockgen -destination=../../../mocks/mock_nodeprep/mock_mpathconfig/mock_section.go  github.com/netapp/trident/internal/nodeprep/mpathconfig MpathConfigurationSection

import (
	"fmt"

	"github.com/hpe-storage/common-host-libs/mpathconfig"

	"github.com/netapp/trident/utils/errors"
)

// there are additional sections that are described in the man pages for multipath.conf
// these are the ones that are most relevant to the current implementation,
// additional sections can be added as needed
const (
	DefaultsSectionName            = "defaults"
	BlacklistSectionName           = "blacklist"
	BlacklistExceptionsSectionName = "blacklist_exceptions"
	DevicesSectionName             = "devices"
	MultipathsSectionName          = "multipaths"
	OverridesSectionName           = "overrides"
	DeviceSectionName              = "device"
	MultipathSectionName           = "multipath"
)

type MpathConfigurationSection interface {
	AddSection(sectionName string) (MpathConfigurationSection, error)
	GetDeviceSection(vendor, product, revision string) (MpathConfigurationSection, error)
	GetProperty(key string) (string, error)
	HasProperty(key string) bool
	SetProperty(key, value string) error
}

var _ MpathConfigurationSection = &Section{}

type Section struct {
	configuration *mpathconfig.Configuration
	section       *mpathconfig.Section
}

func (s *Section) AddSection(sectionName string) (MpathConfigurationSection, error) {
	if err := s.validateSectionName(sectionName); err != nil {
		return nil, err
	}
	childSection, err := s.configuration.AddSection(sectionName, s.section)
	if err != nil {
		return nil, fmt.Errorf("error adding section %s: %v", sectionName, err)
	}
	return &Section{configuration: s.configuration, section: childSection}, nil
}

func (s *Section) GetDeviceSection(vendor, product, revision string) (MpathConfigurationSection, error) {
	if s.section == nil {
		return nil, errors.UnsupportedError("section is nil")
	}

	if s.section.GetChildren().Len() > 0 {
		for e := s.section.GetChildren().Front(); e != nil; e = e.Next() {
			childSection := e.Value.(*mpathconfig.Section)
			if childSection.GetName() != DeviceSectionName {
				continue
			}
			if "" != vendor && childSection.GetProperties()["vendor"] != vendor {
				continue
			}
			if "" != product && childSection.GetProperties()["product"] != product {
				continue
			}
			if "" != revision && childSection.GetProperties()["revision"] != revision {
				continue
			}
			return &Section{configuration: s.configuration, section: childSection}, nil
		}
	}
	return nil, fmt.Errorf("device section not found: vendor: %s, product: %s, revision: %s", vendor, product, revision)
}

func (s *Section) GetProperty(key string) (string, error) {
	if s.section == nil {
		return "", errors.UnsupportedError("section is nil")
	}
	return s.section.GetProperties()[key], nil
}

func (s *Section) HasProperty(key string) bool {
	if s.section == nil {
		return false
	}
	_, ok := s.section.GetProperties()[key]
	return ok
}

func (s *Section) SetProperty(key, value string) error {
	if s.section == nil {
		return errors.UnsupportedError("section is nil")
	}
	if s.isRootSection() {
		return fmt.Errorf("cannot set property on root section: %s", s.section.GetName())
	}
	s.section.GetProperties()[key] = value
	return nil
}

func (s *Section) validateSectionName(sectionName string) error {
	if s.isRootSection() && !isValidRootSection(sectionName) {
		return fmt.Errorf("invalid root section name: %s", sectionName)
	}
	if !s.isRootSection() && !isValidSection(sectionName) {
		return fmt.Errorf("invalid section name: %s", sectionName)
	}
	if s.section.GetName() == DefaultsSectionName || s.section.GetName() == OverridesSectionName {
		return fmt.Errorf("cannot add section to %v section", s.section.GetName())
	}
	return nil
}

func (s *Section) isRootSection() bool {
	return s.section.GetName() == s.configuration.GetRoot().GetName()
}

func isValidRootSection(sectionName string) bool {
	switch sectionName {
	case DefaultsSectionName:
		fallthrough
	case BlacklistSectionName:
		fallthrough
	case BlacklistExceptionsSectionName:
		fallthrough
	case MultipathsSectionName:
		fallthrough
	case DevicesSectionName:
		fallthrough
	case OverridesSectionName:
		return true
	default:
		return false
	}
}

func isValidSection(sectionName string) bool {
	switch sectionName {
	case DeviceSectionName:
		fallthrough
	case MultipathSectionName:
		return true
	default:
		return false
	}
}
