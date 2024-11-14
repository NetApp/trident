// Copyright 2024 NetApp, Inc. All Rights Reserved.

package step_test

import (
	"context"
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/nodeprep/mpathconfig"
	"github.com/netapp/trident/internal/nodeprep/step"
	"github.com/netapp/trident/mocks/mock_internal/mock_nodeprep/mock_mpathconfig"
	"github.com/netapp/trident/utils/errors"
)

func TestNewMultipathConfigureRHCOSStep(t *testing.T) {
	assert.NotNil(t, step.NewMultipathConfigureRHCOSStep())
}

func TestNewMultipathConfigureStepRHCOSDetailed(t *testing.T) {
	type parameters struct {
		getFileSystem func() afero.Fs
	}

	tests := map[string]parameters{
		"running outside the container": {
			getFileSystem: func() afero.Fs {
				return afero.NewMemMapFs()
			},
		},
		"running inside the container": {
			getFileSystem: func() afero.Fs {
				fs := afero.NewMemMapFs()
				_, err := fs.Create(config.NamespaceFile)
				assert.NoError(t, err)
				return fs
			},
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			mpathConfigStep := step.NewMultipathConfigureRHCOSStepDetailed(mpathconfig.MultiPathConfigurationLocation, nil, nil,
				afero.Afero{Fs: params.getFileSystem()})
			assert.NotNil(t, mpathConfigStep)
		})
	}
}

func TestMultipathConfigureRHCOSStep_Apply(t *testing.T) {
	fs := afero.NewMemMapFs()
	osFs := afero.Afero{Fs: fs}

	type parameters struct {
		mpathConfigLocation    string
		getMpathConfiguration  func() mpathconfig.MpathConfiguration
		configConstructorError error
		assertError            assert.ErrorAssertionFunc
	}

	ctx := context.Background()

	tests := map[string]parameters{
		"multipath rhcos tools already installed and configuration file exists ": {
			mpathConfigLocation: os.DevNull,
			getMpathConfiguration: func() mpathconfig.MpathConfiguration {
				mpathConfig, err := mpathconfig.New(osFs)
				assert.NoError(t, err)
				return mpathConfig
			},
			assertError: assert.NoError,
		},
		"multipath rhcos tools already installed and configuration file does not exist": {
			mpathConfigLocation: os.DevNull,
			getMpathConfiguration: func() mpathconfig.MpathConfiguration {
				mpathConfig, err := mpathconfig.New(osFs)
				assert.NoError(t, err)
				return mpathConfig
			},
			assertError: assert.NoError,
		},
		"multipath rhcos tools installed and configuration exists: error getting config": {
			mpathConfigLocation: os.DevNull,
			getMpathConfiguration: func() mpathconfig.MpathConfiguration {
				mpathConfig, err := mpathconfig.New(osFs)
				assert.NoError(t, err)
				return mpathConfig
			},
			configConstructorError: errors.New("some error"),
			assertError:            assert.Error,
		},
		"multipath rhcos tools installed and configuration exists: error updating config": {
			mpathConfigLocation: os.DevNull,
			getMpathConfiguration: func() mpathconfig.MpathConfiguration {
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
		"multipath rhcos tools config does not exist: error adding config": {
			mpathConfigLocation: "9d0010ae-479a-49c3-9460-16726606b458.conf",
			getMpathConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)

				section := mock_mpathconfig.NewMockMpathConfigurationSection(mockCtrl)
				section.EXPECT().AddSection(mpathconfig.DefaultsSectionName).Return(nil, errors.New("some error"))

				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				mpathConfig.EXPECT().GetRootSection().Return(section)
				return mpathConfig
			},
			assertError: assert.Error,
		},
		"multipath rhcos tools config does not exist: error constructing config": {
			mpathConfigLocation: "9d0010ae-479a-49c3-9460-16726606b458.conf",
			getMpathConfiguration: func() mpathconfig.MpathConfiguration {
				mockCtrl := gomock.NewController(t)
				mpathConfig := mock_mpathconfig.NewMockMpathConfiguration(mockCtrl)
				return mpathConfig
			},
			configConstructorError: errors.New("some error"),
			assertError:            assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			configConstructors := mockMpathConfigConstructor{
				config: params.getMpathConfiguration(),
				err:    params.configConstructorError,
			}

			_, err := fs.Create(os.DevNull)
			assert.NoError(t, err)
			configurator := step.NewMultipathConfigureRHCOSStepDetailed(params.mpathConfigLocation,
				configConstructors.NewFromFile, configConstructors.New,
				osFs)

			err = configurator.Apply(ctx)
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}
