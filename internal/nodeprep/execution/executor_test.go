// Copyright 2024 NetApp, Inc. All Rights Reserved.

package execution_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/internal/nodeprep/execution"
	"github.com/netapp/trident/internal/nodeprep/instruction"
	"github.com/netapp/trident/mocks/mock_internal/mock_nodeprep/mock_instruction"
	"github.com/netapp/trident/utils/errors"
)

func TestExecute(t *testing.T) {
	type parameters struct {
		getInstructions func(controller *gomock.Controller) []instruction.Instructions
		assertError     assert.ErrorAssertionFunc
	}

	tests := map[string]parameters{
		"execute single instruction successfully": {
			getInstructions: func(controller *gomock.Controller) []instruction.Instructions {
				mockInstruction := mock_instruction.NewMockInstructions(controller)
				mockInstruction.EXPECT().GetName().Return("mockInstruction")
				mockInstruction.EXPECT().PreCheck(gomock.Any()).Return(nil)
				mockInstruction.EXPECT().Apply(gomock.Any()).Return(nil)
				mockInstruction.EXPECT().PostCheck(gomock.Any()).Return(nil)
				return []instruction.Instructions{mockInstruction}
			},
			assertError: assert.NoError,
		},
		"execute single instruction pre check failure": {
			getInstructions: func(controller *gomock.Controller) []instruction.Instructions {
				mockInstruction := mock_instruction.NewMockInstructions(controller)
				mockInstruction.EXPECT().GetName().Return("mockInstruction")
				mockInstruction.EXPECT().PreCheck(gomock.Any()).Return(errors.New("mock error"))
				return []instruction.Instructions{mockInstruction}
			},
			assertError: assert.Error,
		},
		"execute single instruction apply failure": {
			getInstructions: func(controller *gomock.Controller) []instruction.Instructions {
				mockInstruction := mock_instruction.NewMockInstructions(controller)
				mockInstruction.EXPECT().GetName().Return("mockInstruction")
				mockInstruction.EXPECT().PreCheck(gomock.Any()).Return(nil)
				mockInstruction.EXPECT().Apply(gomock.Any()).Return(errors.New("mock error"))
				return []instruction.Instructions{mockInstruction}
			},
			assertError: assert.Error,
		},
		"execute single instruction post check failure": {
			getInstructions: func(controller *gomock.Controller) []instruction.Instructions {
				mockInstruction := mock_instruction.NewMockInstructions(controller)
				mockInstruction.EXPECT().GetName().Return("mockInstruction")
				mockInstruction.EXPECT().PreCheck(gomock.Any()).Return(nil)
				mockInstruction.EXPECT().Apply(gomock.Any()).Return(nil)
				mockInstruction.EXPECT().PostCheck(gomock.Any()).Return(errors.New("mock error"))
				return []instruction.Instructions{mockInstruction}
			},
			assertError: assert.Error,
		},
		"execute  multi instruction successfully": {
			getInstructions: func(controller *gomock.Controller) []instruction.Instructions {
				mockInstruction1 := mock_instruction.NewMockInstructions(controller)
				mockInstruction1.EXPECT().GetName().Return("mockInstruction")
				mockInstruction1.EXPECT().PreCheck(gomock.Any()).Return(nil)
				mockInstruction1.EXPECT().Apply(gomock.Any()).Return(nil)
				mockInstruction1.EXPECT().PostCheck(gomock.Any()).Return(nil)

				mockInstruction2 := mock_instruction.NewMockInstructions(controller)
				mockInstruction2.EXPECT().GetName().Return("mockInstruction")
				mockInstruction2.EXPECT().PreCheck(gomock.Any()).Return(nil)
				mockInstruction2.EXPECT().Apply(gomock.Any()).Return(nil)
				mockInstruction2.EXPECT().PostCheck(gomock.Any()).Return(nil)

				return []instruction.Instructions{mockInstruction1, mockInstruction2}
			},
			assertError: assert.NoError,
		},
		"execute  multi instruction first instruction failure": {
			getInstructions: func(controller *gomock.Controller) []instruction.Instructions {
				mockInstruction1 := mock_instruction.NewMockInstructions(controller)
				mockInstruction1.EXPECT().GetName().Return("mockInstruction")
				mockInstruction1.EXPECT().PreCheck(gomock.Any()).Return(nil)
				mockInstruction1.EXPECT().Apply(gomock.Any()).Return(errors.New("mock error"))

				mockInstruction2 := mock_instruction.NewMockInstructions(controller)

				return []instruction.Instructions{mockInstruction1, mockInstruction2}
			},
			assertError: assert.Error,
		},
		"execute  multi instruction second instruction failure": {
			getInstructions: func(controller *gomock.Controller) []instruction.Instructions {
				mockInstruction1 := mock_instruction.NewMockInstructions(controller)
				mockInstruction1.EXPECT().GetName().Return("mockInstruction")
				mockInstruction1.EXPECT().PreCheck(gomock.Any()).Return(nil)
				mockInstruction1.EXPECT().Apply(gomock.Any()).Return(nil)
				mockInstruction1.EXPECT().PostCheck(gomock.Any()).Return(nil)

				mockInstruction2 := mock_instruction.NewMockInstructions(controller)
				mockInstruction2.EXPECT().GetName().Return("mockInstruction")
				mockInstruction2.EXPECT().PreCheck(gomock.Any()).Return(nil)
				mockInstruction2.EXPECT().Apply(gomock.Any()).Return(errors.New("mock error"))

				return []instruction.Instructions{mockInstruction1, mockInstruction2}
			},
			assertError: assert.Error,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			err := execution.Execute(context.TODO(), params.getInstructions(ctrl))
			if params.assertError != nil {
				params.assertError(t, err)
			}
		})
	}
}
