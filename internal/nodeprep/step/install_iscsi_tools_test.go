package step

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_internal/mock_nodeprep/mock_packagemanager"
)

func TestNewInstallIscsiTools(t *testing.T) {
	expectedError := errors.New("test error")

	type parameters struct {
		name     string
		required bool
		getPM    func(ctrl *gomock.Controller) *mock_packagemanager.MockPackageManager
		err      error
	}

	tests := map[string]parameters{
		"apply success": {
			name:     "install iSCSI tools",
			required: true,
			getPM: func(ctrl *gomock.Controller) *mock_packagemanager.MockPackageManager {
				pm := mock_packagemanager.NewMockPackageManager(ctrl)
				pm.EXPECT().InstallIscsiRequirements(gomock.Any()).Return(nil)
				return pm
			},
			err: nil,
		},
		"apply error": {
			name:     "install iSCSI tools",
			required: true,
			getPM: func(ctrl *gomock.Controller) *mock_packagemanager.MockPackageManager {
				pm := mock_packagemanager.NewMockPackageManager(ctrl)
				pm.EXPECT().InstallIscsiRequirements(gomock.Any()).Return(expectedError)
				return pm
			},
			err: expectedError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			pm := params.getPM(gomock.NewController(t))
			step := NewInstallIscsiTools(pm)
			require.Equal(t, params.name, step.GetName())
			require.Equal(t, params.required, step.IsRequired())
			err := step.Apply(nil)
			require.Equal(t, params.err, err)
		})
	}
}
