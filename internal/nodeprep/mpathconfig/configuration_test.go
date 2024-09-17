// Copyright 2024 NetApp, Inc. All Rights Reserved.

package mpathconfig_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/internal/nodeprep/mpathconfig"
)

func TestNew(t *testing.T) {
	config, err := mpathconfig.New()
	assert.Nil(t, err)
	assert.NotNil(t, config)
	assert.IsType(t, &mpathconfig.Configuration{}, config)
}

func TestNewFromFile(t *testing.T) {
	type parameters struct {
		fileName     string
		assertError  assert.ErrorAssertionFunc
		assertConfig assert.ValueAssertionFunc
	}

	tests := map[string]parameters{
		"test with a valid file": {
			fileName:     os.DevNull,
			assertError:  assert.NoError,
			assertConfig: assert.NotNil,
		},
		"test with an invalid file": {
			fileName:     "dd435124-8ef6-4c0c-b926-13f7c1f53ede.conf",
			assertError:  assert.Error,
			assertConfig: assert.Nil,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			config, err := mpathconfig.NewFromFile(params.fileName)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			if params.assertConfig != nil {
				params.assertConfig(t, config)
			}
		})
	}
}

func TestConfiguration_GetRootSection(t *testing.T) {
	config, err := mpathconfig.New()
	assert.Nil(t, err)
	assert.NotNil(t, config)

	section := config.GetRootSection()
	assert.NotNil(t, section)
	assert.IsType(t, &mpathconfig.Section{}, section)
}

func TestConfiguration_GetSection(t *testing.T) {
	type parameters struct {
		getConfig     func() mpathconfig.MpathConfiguration
		sectionName   string
		assertError   assert.ErrorAssertionFunc
		assertSection assert.ValueAssertionFunc
	}

	tests := map[string]parameters{
		"get default section from uninitialized configuration": {
			getConfig: func() mpathconfig.MpathConfiguration {
				return &mpathconfig.Configuration{}
			},
			sectionName:   mpathconfig.DefaultsSectionName,
			assertError:   assert.Error,
			assertSection: assert.Nil,
		},
		"get default section from empty configuration": {
			getConfig: func() mpathconfig.MpathConfiguration {
				config, err := mpathconfig.New()
				assert.Nil(t, err)
				return config
			},
			sectionName:   mpathconfig.DefaultsSectionName,
			assertError:   assert.Error,
			assertSection: assert.Nil,
		},
		"get default section from configuration that has a default section": {
			getConfig: func() mpathconfig.MpathConfiguration {
				config, err := mpathconfig.New()
				assert.Nil(t, err)

				_, err = config.GetRootSection().AddSection(mpathconfig.DefaultsSectionName)
				assert.Nil(t, err)

				return config
			},
			sectionName:   mpathconfig.DefaultsSectionName,
			assertError:   assert.NoError,
			assertSection: assert.NotNil,
		},
		"get invalid section from configuration that has a default section": {
			getConfig: func() mpathconfig.MpathConfiguration {
				config, err := mpathconfig.New()
				assert.Nil(t, err)

				_, err = config.GetRootSection().AddSection(mpathconfig.DefaultsSectionName)
				assert.Nil(t, err)

				return config
			},
			sectionName:   "invalid",
			assertError:   assert.Error,
			assertSection: assert.Nil,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			config := params.getConfig()

			section, err := config.GetSection(params.sectionName)
			if params.assertError != nil {
				params.assertError(t, err)
			}
			if params.assertSection != nil {
				params.assertSection(t, section)
			}
		})
	}
}

func TestConfiguration_PrintConf(t *testing.T) {
	type parameters struct {
		getConfig      func() mpathconfig.MpathConfiguration
		expectedOutput []string
	}

	tests := map[string]parameters{
		"print uninitialized configuration": {
			getConfig: func() mpathconfig.MpathConfiguration {
				return &mpathconfig.Configuration{}
			},
			expectedOutput: nil,
		},
		"print empty configuration": {
			getConfig: func() mpathconfig.MpathConfiguration {
				config, err := mpathconfig.New()
				assert.Nil(t, err)
				return config
			},
			expectedOutput: nil,
		},
		"print configuration with a default section": {
			getConfig: func() mpathconfig.MpathConfiguration {
				config, err := mpathconfig.New()
				assert.Nil(t, err)

				_, err = config.GetRootSection().AddSection(mpathconfig.DefaultsSectionName)
				assert.Nil(t, err)

				return config
			},
			expectedOutput: []string{"defaults {\n", "}\n"},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			config := params.getConfig()
			output := config.PrintConf()
			assert.Equal(t, params.expectedOutput, output)
		})
	}
}

func TestConfiguration_SaveConfig(t *testing.T) {
	config, err := mpathconfig.New()
	assert.NoError(t, err)
	defaultSection, err := config.GetRootSection().AddSection(mpathconfig.DefaultsSectionName)
	assert.NoError(t, err)
	err = defaultSection.SetProperty("find_multipaths", "no")
	assert.NoError(t, err)
	assert.Nil(t, config.SaveConfig(os.DevNull))
}
