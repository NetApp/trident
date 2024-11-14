// Copyright 2024 NetApp, Inc. All Rights Reserved.

package mpathconfig_test

import (
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/internal/nodeprep/mpathconfig"
)

func TestSection_AddSection(t *testing.T) {
	fs := afero.NewMemMapFs()
	os := afero.Afero{Fs: fs}

	type parameters struct {
		getParentSection func() mpathconfig.MpathConfigurationSection
		sectionName      string
		assertError      assert.ErrorAssertionFunc
		assertSection    assert.ValueAssertionFunc
	}

	tests := map[string]parameters{
		"Add defaults section to root section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				return config.GetRootSection()
			},
			sectionName:   mpathconfig.DefaultsSectionName,
			assertError:   assert.NoError,
			assertSection: assert.NotNil,
		},
		"Add blacklist section to root section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				return config.GetRootSection()
			},
			sectionName:   mpathconfig.BlacklistSectionName,
			assertError:   assert.NoError,
			assertSection: assert.NotNil,
		},
		"Add blacklist exceptions section to root section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				return config.GetRootSection()
			},
			sectionName:   mpathconfig.BlacklistExceptionsSectionName,
			assertError:   assert.NoError,
			assertSection: assert.NotNil,
		},
		"Add devices section to root section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				return config.GetRootSection()
			},
			sectionName:   mpathconfig.DevicesSectionName,
			assertError:   assert.NoError,
			assertSection: assert.NotNil,
		},
		"Add multipaths section to root section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				return config.GetRootSection()
			},
			sectionName:   mpathconfig.MultipathsSectionName,
			assertError:   assert.NoError,
			assertSection: assert.NotNil,
		},
		"Add overrides section to root section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				return config.GetRootSection()
			},
			sectionName:   mpathconfig.OverridesSectionName,
			assertError:   assert.NoError,
			assertSection: assert.NotNil,
		},
		"Add device section to root section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				return config.GetRootSection()
			},
			sectionName:   mpathconfig.DeviceSectionName,
			assertError:   assert.Error,
			assertSection: assert.Nil,
		},
		"Add multipath section to root section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				return config.GetRootSection()
			},
			sectionName:   mpathconfig.MultipathSectionName,
			assertError:   assert.Error,
			assertSection: assert.Nil,
		},
		"Add foo section to root section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				return config.GetRootSection()
			},
			sectionName:   "foo",
			assertError:   assert.Error,
			assertSection: assert.Nil,
		},
		"Add section to defaults section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				defaultsSection, err := config.GetRootSection().AddSection(mpathconfig.DefaultsSectionName)
				assert.NoError(t, err)
				return defaultsSection
			},
			sectionName:   mpathconfig.DeviceSectionName,
			assertError:   assert.Error,
			assertSection: assert.Nil,
		},
		"Add section to overrides section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				defaultsSection, err := config.GetRootSection().AddSection(mpathconfig.OverridesSectionName)
				assert.NoError(t, err)
				return defaultsSection
			},
			sectionName:   mpathconfig.DeviceSectionName,
			assertError:   assert.Error,
			assertSection: assert.Nil,
		},
		"Add device section to blacklist section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				defaultsSection, err := config.GetRootSection().AddSection(mpathconfig.BlacklistSectionName)
				assert.NoError(t, err)
				return defaultsSection
			},
			sectionName:   mpathconfig.DeviceSectionName,
			assertError:   assert.NoError,
			assertSection: assert.NotNil,
		},
		"Add foo section to blacklist section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				blacklistSection, err := config.GetRootSection().AddSection(mpathconfig.BlacklistSectionName)
				assert.NoError(t, err)
				return blacklistSection
			},
			sectionName:   "foo",
			assertError:   assert.Error,
			assertSection: assert.Nil,
		},
		"Add device section to blacklist exception section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				blacklistExceptionSection, err := config.GetRootSection().AddSection(mpathconfig.
					BlacklistExceptionsSectionName)
				assert.NoError(t, err)
				return blacklistExceptionSection
			},
			sectionName:   mpathconfig.DeviceSectionName,
			assertError:   assert.NoError,
			assertSection: assert.NotNil,
		},
		"Add foo section to blacklist exception section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				blacklistExceptionSection, err := config.GetRootSection().AddSection(mpathconfig.
					BlacklistExceptionsSectionName)
				assert.NoError(t, err)
				return blacklistExceptionSection
			},
			sectionName:   "foo",
			assertError:   assert.Error,
			assertSection: assert.Nil,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			parentSection := params.getParentSection()
			section, err := parentSection.AddSection(params.sectionName)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			if params.assertSection != nil {
				params.assertSection(t, section)
			}
		})
	}
}

func TestSection_GetDeviceSection(t *testing.T) {
	fs := afero.NewMemMapFs()
	os := afero.Afero{Fs: fs}

	type parameters struct {
		GetParentSection func() mpathconfig.MpathConfigurationSection
		Vendor           string
		Product          string
		Revision         string
		ExpectedError    assert.ErrorAssertionFunc
		ExpectedSection  assert.ValueAssertionFunc
	}

	tests := map[string]parameters{
		"Get device section from uninitialized section": {
			GetParentSection: func() mpathconfig.MpathConfigurationSection {
				return &mpathconfig.Section{}
			},
			Vendor:          "vendor",
			Product:         "product",
			Revision:        "revision",
			ExpectedError:   assert.Error,
			ExpectedSection: assert.Nil,
		},
		"Get device section from root section": {
			GetParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				return config.GetRootSection()
			},
			Vendor:          "vendor",
			Product:         "product",
			Revision:        "revision",
			ExpectedError:   assert.Error,
			ExpectedSection: assert.Nil,
		},
		"Get device section from empty section": {
			GetParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				blacklistSectionName, err := config.GetRootSection().AddSection(mpathconfig.BlacklistSectionName)
				assert.NoError(t, err)
				return blacklistSectionName
			},
			Vendor:          "vendor",
			Product:         "product",
			Revision:        "revision",
			ExpectedError:   assert.Error,
			ExpectedSection: assert.Nil,
		},
		"Get device section from section that has a device section": {
			GetParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				blacklistSectionName, err := config.GetRootSection().AddSection(mpathconfig.BlacklistSectionName)
				assert.NoError(t, err)
				deviceSection, err := blacklistSectionName.AddSection(mpathconfig.DeviceSectionName)
				assert.NoError(t, err)
				assert.NoError(t, deviceSection.SetProperty("vendor", "vendor"))
				assert.NoError(t, deviceSection.SetProperty("product", "product"))
				assert.NoError(t, deviceSection.SetProperty("revision", "revision"))
				return blacklistSectionName
			},
			Vendor:          "vendor",
			Product:         "product",
			Revision:        "revision",
			ExpectedError:   assert.NoError,
			ExpectedSection: assert.NotNil,
		},
		"Get device section from section that has a device section with different vendor": {
			GetParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				blacklistSectionName, err := config.GetRootSection().AddSection(mpathconfig.BlacklistSectionName)
				assert.NoError(t, err)
				deviceSection, err := blacklistSectionName.AddSection(mpathconfig.DeviceSectionName)
				assert.NoError(t, err)
				assert.NoError(t, deviceSection.SetProperty("vendor", "vendor1"))
				assert.NoError(t, deviceSection.SetProperty("product", "product"))
				assert.NoError(t, deviceSection.SetProperty("revision", "revision"))
				return blacklistSectionName
			},
			Vendor:          "vendor",
			Product:         "product",
			Revision:        "revision",
			ExpectedError:   assert.Error,
			ExpectedSection: assert.Nil,
		},
		"Get device section from section that has a device section with different product": {
			GetParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				blacklistSectionName, err := config.GetRootSection().AddSection(mpathconfig.BlacklistSectionName)
				assert.NoError(t, err)
				deviceSection, err := blacklistSectionName.AddSection(mpathconfig.DeviceSectionName)
				assert.NoError(t, err)
				assert.NoError(t, deviceSection.SetProperty("vendor", "vendor"))
				assert.NoError(t, deviceSection.SetProperty("product", "product1"))
				assert.NoError(t, deviceSection.SetProperty("revision", "revision"))
				return blacklistSectionName
			},
			Vendor:          "vendor",
			Product:         "product",
			Revision:        "revision",
			ExpectedError:   assert.Error,
			ExpectedSection: assert.Nil,
		},
		"Get device section from section that has a device section with different revision": {
			GetParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				blacklistSectionName, err := config.GetRootSection().AddSection(mpathconfig.BlacklistSectionName)
				assert.NoError(t, err)
				deviceSection, err := blacklistSectionName.AddSection(mpathconfig.DeviceSectionName)
				assert.NoError(t, err)
				assert.NoError(t, deviceSection.SetProperty("vendor", "vendor"))
				assert.NoError(t, deviceSection.SetProperty("product", "product"))
				assert.NoError(t, deviceSection.SetProperty("revision", "revision1"))
				return blacklistSectionName
			},
			Vendor:          "vendor",
			Product:         "product",
			Revision:        "revision",
			ExpectedError:   assert.Error,
			ExpectedSection: assert.Nil,
		},
		"Get device section from section that has a multiple sections": {
			GetParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)

				blacklistSection, err := config.GetRootSection().AddSection(mpathconfig.BlacklistSectionName)
				assert.NoError(t, err)

				_, err = blacklistSection.AddSection(mpathconfig.MultipathSectionName)
				assert.NoError(t, err)

				deviceSection, err := blacklistSection.AddSection(mpathconfig.DeviceSectionName)
				assert.NoError(t, err)
				assert.NoError(t, deviceSection.SetProperty("vendor", "vendor"))
				assert.NoError(t, deviceSection.SetProperty("product", "product"))
				assert.NoError(t, deviceSection.SetProperty("revision", "revision1"))

				return blacklistSection
			},
			Vendor:          "vendor",
			Product:         "product",
			Revision:        "revision",
			ExpectedError:   assert.Error,
			ExpectedSection: assert.Nil,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			parent := params.GetParentSection()
			section, err := parent.GetDeviceSection(params.Vendor, params.Product, params.Revision)
			if params.ExpectedError != nil {
				params.ExpectedError(t, err)
			}
			if params.ExpectedSection != nil {
				params.ExpectedSection(t, section)
			}
		})
	}
}

func TestSection_HasProperty(t *testing.T) {
	fs := afero.NewMemMapFs()
	os := afero.Afero{Fs: fs}

	type parameters struct {
		getParentSection func() mpathconfig.MpathConfigurationSection
		property         string
		assertResult     assert.BoolAssertionFunc
	}

	tests := map[string]parameters{
		"Check if property exists in uninitialized section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				return &mpathconfig.Section{}
			},
			property:     "property",
			assertResult: assert.False,
		},
		"Check if property exists in empty section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				return config.GetRootSection()
			},
			property:     "property",
			assertResult: assert.False,
		},
		"Check if property exists in section that has the property": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				section, err := config.GetRootSection().AddSection(mpathconfig.DefaultsSectionName)
				assert.NoError(t, err)
				err = section.SetProperty("property", "value")
				assert.Nil(t, err)
				return section
			},
			property:     "property",
			assertResult: assert.True,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			parentSection := params.getParentSection()
			result := parentSection.HasProperty(params.property)
			params.assertResult(t, result)
		})
	}
}

func TestSection_GetProperty(t *testing.T) {
	fs := afero.NewMemMapFs()
	os := afero.Afero{Fs: fs}

	type parameters struct {
		getParentSection func() mpathconfig.MpathConfigurationSection
		property         string
		assertResult     assert.ValueAssertionFunc
		assertError      assert.ErrorAssertionFunc
	}

	tests := map[string]parameters{
		"Get property from uninitialized section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				return &mpathconfig.Section{}
			},
			property:     "property",
			assertResult: assert.Empty,
			assertError:  assert.Error,
		},
		"Get property from empty section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				return config.GetRootSection()
			},
			property:     "property",
			assertResult: assert.Empty,
		},
		"Get property from section that has the property": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				section, err := config.GetRootSection().AddSection(mpathconfig.DefaultsSectionName)
				assert.NoError(t, err)
				err = section.SetProperty("property", "value")
				assert.Nil(t, err)
				return section
			},
			property:     "property",
			assertResult: assert.NotNil,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			parentSection := params.getParentSection()

			result, err := parentSection.GetProperty(params.property)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			if params.assertResult != nil {
				params.assertResult(t, result)
			}
		})
	}
}

func TestSection_SetProperty(t *testing.T) {
	fs := afero.NewMemMapFs()
	os := afero.Afero{Fs: fs}

	type parameters struct {
		getParentSection func() mpathconfig.MpathConfigurationSection
		property         string
		value            string
		assertError      assert.ErrorAssertionFunc
	}

	tests := map[string]parameters{
		"Set property in uninitialized section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				return &mpathconfig.Section{}
			},
			property:    "property",
			value:       "value",
			assertError: assert.Error,
		},
		"Set property in root section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				return config.GetRootSection()
			},
			property:    "property",
			value:       "value",
			assertError: assert.Error,
		},
		"Set property in defaults section": {
			getParentSection: func() mpathconfig.MpathConfigurationSection {
				config, err := mpathconfig.New(os)
				assert.NoError(t, err)
				defaultsSection, err := config.GetRootSection().AddSection(mpathconfig.DefaultsSectionName)
				assert.NoError(t, err)
				return defaultsSection
			},
			property:    "property",
			value:       "value",
			assertError: assert.NoError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			parentSection := params.getParentSection()
			err := parentSection.SetProperty(params.property, params.value)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}
