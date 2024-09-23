// Copyright 2024 NetApp, Inc. All Rights Reserved.

package stage_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/internal/nodeprep/mpathconfig"
	"github.com/netapp/trident/internal/nodeprep/packagemanager"
	"github.com/netapp/trident/internal/nodeprep/stage"
	"github.com/netapp/trident/mocks/mock_nodeprep/mock_mpathconfig"
	"github.com/netapp/trident/mocks/mock_nodeprep/mock_packagemanager"
	"github.com/netapp/trident/utils/errors"
)

func TestNewConfigurator(t *testing.T) {
	assert.NotNil(t, stage.NewConfigurator())
}

func TestNewConfiguratorDetailed(t *testing.T) {
	assert.NotNil(t, stage.NewConfiguratorDetailed(mpathconfig.MultiPathConfigurationLocation, nil, nil, nil))
}

func TestConfigurator_AddConfiguration(t *testing.T) {
	type parameters struct {
		getConfiguration func() mpathconfig.MpathConfiguration
		assertError      assert.ErrorAssertionFunc
	}

	tests := map[string]parameters{
		"happy path": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)

				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				section.EXPECT().SetProperty("find_multipaths", "no").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistSectionName).Return(section, nil)
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, nil)
				section.EXPECT().SetProperty("vendor", ".*").Return(nil)
				section.EXPECT().SetProperty("product", ".*").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistExceptionsSectionName).Return(section, nil)
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, nil)
				section.EXPECT().SetProperty("vendor", "NETAPP").Return(nil)
				section.EXPECT().SetProperty("product", "LUN").Return(nil)

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(3)

				return mpathConfig
			},
			assertError: assert.NoError,
		},
		"default section creation returns an error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)

				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(nil, errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(1)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"setting property in the default section return an error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)

				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				section.EXPECT().SetProperty("find_multipaths", "no").Return(errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(1)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"adding the blacklist section returns an error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)

				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				section.EXPECT().SetProperty("find_multipaths", "no").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistSectionName).Return(section, errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(2)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"adding device section to blacklist section returns an error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)

				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				section.EXPECT().SetProperty("find_multipaths", "no").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistSectionName).Return(section, nil)
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(2)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"adding the vendor key to the device section in blacklist returns an error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)

				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				section.EXPECT().SetProperty("find_multipaths", "no").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistSectionName).Return(section, nil)
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, nil)
				section.EXPECT().SetProperty("vendor", ".*").Return(errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(2)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"adding the product key to the device section in blacklist returns an error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)

				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				section.EXPECT().SetProperty("find_multipaths", "no").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistSectionName).Return(section, nil)
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, nil)
				section.EXPECT().SetProperty("vendor", ".*").Return(nil)
				section.EXPECT().SetProperty("product", ".*").Return(errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(2)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"adding blacklist exception section return an error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)

				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				section.EXPECT().SetProperty("find_multipaths", "no").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistSectionName).Return(section, nil)
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, nil)
				section.EXPECT().SetProperty("vendor", ".*").Return(nil)
				section.EXPECT().SetProperty("product", ".*").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistExceptionsSectionName).Return(section, errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(3)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"adding device section to blacklist exception returns an error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)

				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				section.EXPECT().SetProperty("find_multipaths", "no").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistSectionName).Return(section, nil)
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, nil)
				section.EXPECT().SetProperty("vendor", ".*").Return(nil)
				section.EXPECT().SetProperty("product", ".*").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistExceptionsSectionName).Return(section, nil)
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(3)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"adding the vendor key to the device section in the blacklist exception returns an error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)

				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				section.EXPECT().SetProperty("find_multipaths", "no").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistSectionName).Return(section, nil)
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, nil)
				section.EXPECT().SetProperty("vendor", ".*").Return(nil)
				section.EXPECT().SetProperty("product", ".*").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistExceptionsSectionName).Return(section, nil)
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, nil)
				section.EXPECT().SetProperty("vendor", "NETAPP").Return(errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(3)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"adding the product key to the device section in the blacklist exception returns an error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)

				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				section.EXPECT().SetProperty("find_multipaths", "no").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistSectionName).Return(section, nil)
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, nil)
				section.EXPECT().SetProperty("vendor", ".*").Return(nil)
				section.EXPECT().SetProperty("product", ".*").Return(nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistExceptionsSectionName).Return(section, nil)
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, nil)
				section.EXPECT().SetProperty("vendor", "NETAPP").Return(nil)
				section.EXPECT().SetProperty("product", "LUN").Return(errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(3)

				return mpathConfig
			},
			assertError: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			configurator := stage.NewConfigurator()

			err := configurator.AddConfiguration(params.getConfiguration())
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestConfigurator_UpdateConfiguration(t *testing.T) {
	type parameters struct {
		getConfiguration func() mpathconfig.MpathConfiguration
		assertError      assert.ErrorAssertionFunc
	}

	tests := map[string]parameters{
		"required configuration is already present": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().HasProperty("find_multipaths").Return(true)
				section.EXPECT().GetProperty("find_multipaths").Return("no", nil)

				section.EXPECT().GetDeviceSection("NETAPP", "LUN", "").Return(section, nil)

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				mpathConfig.EXPECT().GetSection(mpathconfig.BlacklistExceptionsSectionName).Return(section, nil)

				return mpathConfig
			},
			assertError: assert.NoError,
		},
		"get property 'find_multipaths' from  the defaults section returns an error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().HasProperty("find_multipaths").Return(true)
				section.EXPECT().GetProperty("find_multipaths").Return("", errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section, nil)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"configuration has 'find_multipaths' set to 'strict' in the defaults section": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().HasProperty("find_multipaths").Return(true)
				section.EXPECT().GetProperty("find_multipaths").Return("strict", nil)

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section, nil)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"configuration has 'find_multipaths' set to 'yes' in the defaults section": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().HasProperty("find_multipaths").Return(true)
				section.EXPECT().GetProperty("find_multipaths").Return("yes", nil)

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section, nil)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"configuration does not have property 'find_multipaths' in the defaults section": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().HasProperty("find_multipaths").Return(false)

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section, nil)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"defaults section not present in the configuration": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				section.EXPECT().SetProperty("find_multipaths", "no").Return(nil)
				section.EXPECT().HasProperty("find_multipaths").Return(true)
				section.EXPECT().GetProperty("find_multipaths").Return("no", nil)

				section.EXPECT().GetDeviceSection("NETAPP", "LUN", "").Return(section, nil)

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section,
					errors.New("not found"))
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(1)
				mpathConfig.EXPECT().GetSection(mpathconfig.BlacklistExceptionsSectionName).Return(section, nil)

				return mpathConfig
			},
			assertError: assert.NoError,
		},
		"defaults section not present: adding defaults section returns an error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(section, errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section,
					errors.New("not found"))
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(1)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"defaults section not present: setting property returns an error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				section.EXPECT().SetProperty("find_multipaths", "no").Return(errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section,
					errors.New("not found"))
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(1)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"blacklist exception section not present": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().HasProperty("find_multipaths").Return(true)
				section.EXPECT().GetProperty("find_multipaths").Return("no", nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistExceptionsSectionName).Return(section, nil)
				section.EXPECT().GetDeviceSection("NETAPP", "LUN", "").Return(nil, errors.New("not found"))
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, nil)
				section.EXPECT().SetProperty("vendor", "NETAPP").Return(nil)
				section.EXPECT().SetProperty("product", "LUN").Return(nil)

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				mpathConfig.EXPECT().GetSection(mpathconfig.BlacklistExceptionsSectionName).Return(nil, errors.New("not found"))
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(1)

				return mpathConfig
			},
			assertError: assert.NoError,
		},
		"blacklist exception section not present: add section error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().HasProperty("find_multipaths").Return(true)
				section.EXPECT().GetProperty("find_multipaths").Return("no", nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistExceptionsSectionName).Return(nil, errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				mpathConfig.EXPECT().GetSection(mpathconfig.BlacklistExceptionsSectionName).Return(nil, errors.New("not found"))
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(1)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"blacklist exception section not present: add device section error": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().HasProperty("find_multipaths").Return(true)
				section.EXPECT().GetProperty("find_multipaths").Return("no", nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistExceptionsSectionName).Return(section, nil)
				section.EXPECT().GetDeviceSection("NETAPP", "LUN", "").Return(nil, errors.New("not found"))
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				mpathConfig.EXPECT().GetSection(mpathconfig.BlacklistExceptionsSectionName).Return(nil, errors.New("not found"))
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(1)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"blacklist exception section not present: error setting property vendor in device section": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().HasProperty("find_multipaths").Return(true)
				section.EXPECT().GetProperty("find_multipaths").Return("no", nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistExceptionsSectionName).Return(section, nil)
				section.EXPECT().GetDeviceSection("NETAPP", "LUN", "").Return(nil, errors.New("not found"))
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, nil)
				section.EXPECT().SetProperty("vendor", "NETAPP").Return(errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				mpathConfig.EXPECT().GetSection(mpathconfig.BlacklistExceptionsSectionName).Return(nil, errors.New("not found"))
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(1)

				return mpathConfig
			},
			assertError: assert.Error,
		},
		"blacklist exception section not present: error setting property product in device section": {
			getConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().HasProperty("find_multipaths").Return(true)
				section.EXPECT().GetProperty("find_multipaths").Return("no", nil)

				section.EXPECT().AddSection(mpathconfig.BlacklistExceptionsSectionName).Return(section, nil)
				section.EXPECT().GetDeviceSection("NETAPP", "LUN", "").Return(nil, errors.New("not found"))
				section.EXPECT().AddSection(mpathconfig.DeviceSectionName).Return(section, nil)
				section.EXPECT().SetProperty("vendor", "NETAPP").Return(nil)
				section.EXPECT().SetProperty("product", "LUN").Return(errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				mpathConfig.EXPECT().GetSection(mpathconfig.BlacklistExceptionsSectionName).Return(nil, errors.New("not found"))
				mpathConfig.EXPECT().GetRootSection().Return(section).Times(1)

				return mpathConfig
			},
			assertError: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			configurator := stage.NewConfigurator()

			err := configurator.UpdateConfiguration(params.getConfiguration())
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

func TestConfigurator_ConfigureMultipathDaemon(t *testing.T) {
	type parameters struct {
		mpathConfigLocation    string
		getMpathConfiguration  func() mpathconfig.MpathConfiguration
		getPackageManager      func() packagemanager.PackageManager
		configConstructorError error
		assertError            assert.ErrorAssertionFunc
	}

	tests := map[string]parameters{
		"multipath tools already installed and configuration file exists": {
			mpathConfigLocation: os.DevNull,
			getMpathConfiguration: func() mpathconfig.MpathConfiguration {
				config, err := mpathconfig.New()
				assert.NoError(t, err)
				return config
			},
			getPackageManager: func() packagemanager.PackageManager {
				mockCtrl := gomock.NewController(t)
				packageManager := mock_packagemanager.NewMockPackageManager(mockCtrl)
				packageManager.EXPECT().MultipathToolsInstalled().Return(true)
				return packageManager
			},
			assertError: assert.NoError,
		},
		"multipath tools installed and configuration exists: error getting config": {
			mpathConfigLocation: os.DevNull,
			getMpathConfiguration: func() mpathconfig.MpathConfiguration {
				config, err := mpathconfig.New()
				assert.NoError(t, err)
				return config
			},
			getPackageManager: func() packagemanager.PackageManager {
				mockCtrl := gomock.NewController(t)
				packageManager := mock_packagemanager.NewMockPackageManager(mockCtrl)
				packageManager.EXPECT().MultipathToolsInstalled().Return(true)
				return packageManager
			},
			configConstructorError: errors.New("some error"),
			assertError:            assert.Error,
		},
		"multipath tools installed and configuration exists: error updating config": {
			mpathConfigLocation: os.DevNull,
			getMpathConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)

				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().HasProperty("find_multipaths").Return(true)
				section.EXPECT().GetProperty("find_multipaths").Return("", errors.New("some error"))

				config := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				config.EXPECT().GetSection(mpathconfig.DefaultsSectionName).Return(section, nil)
				return config
			},
			getPackageManager: func() packagemanager.PackageManager {
				mockCtrl := gomock.NewController(t)
				packageManager := mock_packagemanager.NewMockPackageManager(mockCtrl)
				packageManager.EXPECT().MultipathToolsInstalled().Return(true)
				return packageManager
			},
			assertError: assert.Error,
		},
		"multipath tools already installed and configuration does not exist": {
			mpathConfigLocation: "9d0010ae-479a-49c3-9460-16726606b458.conf",
			getMpathConfiguration: func() mpathconfig.MpathConfiguration {
				config, err := mpathconfig.New()
				assert.NoError(t, err)
				return config
			},
			getPackageManager: func() packagemanager.PackageManager {
				mockCtrl := gomock.NewController(t)
				packageManager := mock_packagemanager.NewMockPackageManager(mockCtrl)
				packageManager.EXPECT().MultipathToolsInstalled().Return(true)
				return packageManager
			},
			assertError: assert.Error,
		},
		"multipath tools does not exist": {
			mpathConfigLocation: os.DevNull,
			getMpathConfiguration: func() mpathconfig.MpathConfiguration {
				config, err := mpathconfig.New()
				assert.NoError(t, err)
				return config
			},
			getPackageManager: func() packagemanager.PackageManager {
				mockCtrl := gomock.NewController(t)
				packageManager := mock_packagemanager.NewMockPackageManager(mockCtrl)
				packageManager.EXPECT().MultipathToolsInstalled().Return(false)
				return packageManager
			},
			assertError: assert.NoError,
		},
		"multipath tools does not exist: error getting config": {
			mpathConfigLocation: os.DevNull,
			getMpathConfiguration: func() mpathconfig.MpathConfiguration {
				config, err := mpathconfig.New()
				assert.NoError(t, err)
				return config
			},
			getPackageManager: func() packagemanager.PackageManager {
				mockCtrl := gomock.NewController(t)
				packageManager := mock_packagemanager.NewMockPackageManager(mockCtrl)
				packageManager.EXPECT().MultipathToolsInstalled().Return(false)
				return packageManager
			},
			configConstructorError: errors.New("some error"),
			assertError:            assert.Error,
		},
		"multipath tools does not exist: error adding config": {
			mpathConfigLocation: os.DevNull,
			getMpathConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)

				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(nil, errors.New("some error"))

				config := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				config.EXPECT().GetRootSection().Return(section)
				return config
			},
			getPackageManager: func() packagemanager.PackageManager {
				mockCtrl := gomock.NewController(t)
				packageManager := mock_packagemanager.NewMockPackageManager(mockCtrl)
				packageManager.EXPECT().MultipathToolsInstalled().Return(false)
				return packageManager
			},
			assertError: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			configConstructors := mockMpathConfigConstructor{
				config: params.getMpathConfiguration(),
				err:    params.configConstructorError,
			}

			configurator := stage.NewConfiguratorDetailed(params.mpathConfigLocation, configConstructors.NewFromFile,
				configConstructors.New, params.getPackageManager())

			err := configurator.ConfigureMultipathDaemon()
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}

// --- helpers ---
type mockMpathConfigConstructor struct {
	config mpathconfig.MpathConfiguration
	err    error
}

func (m *mockMpathConfigConstructor) New() (mpathconfig.MpathConfiguration, error) {
	return m.config, m.err
}

func (m *mockMpathConfigConstructor) NewFromFile(_ string) (mpathconfig.MpathConfiguration, error) {
	return m.config, m.err
}
