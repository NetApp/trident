package instruction

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/internal/nodeprep/step"
	"github.com/netapp/trident/mocks/mock_internal/mock_nodeprep/mock_step"
)

func TestDefault(t *testing.T) {
	i := &Default{}
	assert.Equal(t, "default instructions", i.GetName())
	i.name = "test"
	assert.Equal(t, "test", i.GetName())
	assert.Empty(t, i.GetSteps())
	assert.Nil(t, i.PreCheck(nil))
	assert.Nil(t, i.Apply(nil))
	assert.Nil(t, i.PostCheck(nil))
}

func TestDefaultValues(t *testing.T) {
	type inst struct {
		Default
	}

	type params struct {
		getInstructions func(name string) inst
		assertError     assert.ErrorAssertionFunc
	}

	tests := map[string]params{
		"required with error": {
			getInstructions: func(name string) inst {
				i := inst{}
				ctrl := gomock.NewController(t)
				ms := mock_step.NewMockStep(ctrl)
				ms.EXPECT().GetName().Return("test").AnyTimes()
				ms.EXPECT().Apply(gomock.Any()).Return(errors.New("test error"))
				ms.EXPECT().IsRequired().Return(true)
				i.steps = []step.Step{ms}
				i.name = "test"
				return i
			},
			assertError: assert.Error,
		},
		"not required with error": {
			getInstructions: func(name string) inst {
				i := inst{}
				ctrl := gomock.NewController(t)
				ms := mock_step.NewMockStep(ctrl)
				ms.EXPECT().GetName().Return("test").AnyTimes()
				ms.EXPECT().Apply(gomock.Any()).Return(errors.New("test error"))
				ms.EXPECT().IsRequired().Return(false)
				i.steps = []step.Step{ms}
				i.name = "test"
				return i
			},
			assertError: assert.NoError,
		},
		"required with successful": {
			getInstructions: func(name string) inst {
				i := inst{}
				ctrl := gomock.NewController(t)
				ms := mock_step.NewMockStep(ctrl)
				ms.EXPECT().GetName().Return("test").AnyTimes()
				ms.EXPECT().Apply(gomock.Any()).Return(nil)
				i.steps = []step.Step{ms}
				i.name = "test"
				return i
			},
			assertError: assert.NoError,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			i := test.getInstructions("test")
			assert.Equal(t, "test", i.GetName())
			assert.Nil(t, i.PreCheck(nil))
			test.assertError(t, i.Apply(context.Background()))
			assert.Nil(t, i.PostCheck(nil))
		})
	}
}
