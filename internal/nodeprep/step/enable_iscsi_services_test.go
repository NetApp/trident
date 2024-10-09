package step

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/mocks/mock_internal/mock_nodeprep/mock_systemmanager"
)

func TestNewEnableIscsiServices(t *testing.T) {
	expectedError := errors.New("test error")

	type parameters struct {
		name     string
		required bool
		getMS    func(ctrl *gomock.Controller) *mock_systemmanager.MockSystemManager
		err      error
	}

	tests := map[string]parameters{
		"apply success": {
			name:     "enable iSCSI services step",
			required: true,
			getMS: func(ctrl *gomock.Controller) *mock_systemmanager.MockSystemManager {
				ms := mock_systemmanager.NewMockSystemManager(ctrl)
				ms.EXPECT().EnableIscsiServices(gomock.Any()).Return(nil)
				return ms
			},
			err: nil,
		},
		"apply error": {
			name:     "enable iSCSI services step",
			required: true,
			getMS: func(ctrl *gomock.Controller) *mock_systemmanager.MockSystemManager {
				ms := mock_systemmanager.NewMockSystemManager(ctrl)
				ms.EXPECT().EnableIscsiServices(gomock.Any()).Return(expectedError)
				return ms
			},
			err: expectedError,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ms := params.getMS(gomock.NewController(t))
			step := NewEnableIscsiServices(ms)
			require.Equal(t, params.name, step.GetName())
			require.Equal(t, params.required, step.IsRequired())
			err := step.Apply(nil)
			require.Equal(t, params.err, err)
		})
	}
}
