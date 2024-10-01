// Copyright 2024 NetApp, Inc. All Rights Reserved.

package nodeprep_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/internal/nodeprep"
	"github.com/netapp/trident/internal/nodeprep/instruction"
	"github.com/netapp/trident/internal/nodeprep/nodeinfo"
	"github.com/netapp/trident/internal/nodeprep/protocol"
	"github.com/netapp/trident/mocks/mock_internal/mock_nodeprep/mock_instruction"
	"github.com/netapp/trident/mocks/mock_internal/mock_nodeprep/mock_nodeinfo"
	"github.com/netapp/trident/utils/errors"
)

func TestNew(t *testing.T) {
	nodePrep := nodeprep.New()
	assert.NotNil(t, nodePrep)
}

func TestNewDetailed(t *testing.T) {
	node := mock_nodeinfo.NewMockNode(gomock.NewController(t))
	nodePrep := nodeprep.NewDetailed(node)
	assert.NotNil(t, nodePrep)
}

func TestPrepareNode(t *testing.T) {
	type parameters struct {
		protocols        []string
		getNode          func(controller *gomock.Controller) nodeinfo.Node
		getInstructions  func(controller *gomock.Controller) map[instruction.Key]instruction.Instructions
		expectedExitCode int
	}

	tests := map[string]parameters{
		"no protocols": {
			protocols: nil,
			getNode: func(controller *gomock.Controller) nodeinfo.Node {
				mockNode := mock_nodeinfo.NewMockNode(controller)
				return mockNode
			},
			getInstructions: func(controller *gomock.Controller) map[instruction.Key]instruction.Instructions {
				return nil
			},
			expectedExitCode: 0,
		},
		"error getting node info": {
			protocols: []string{protocol.ISCSI},
			getNode: func(controller *gomock.Controller) nodeinfo.Node {
				mockNode := mock_nodeinfo.NewMockNode(controller)
				mockNode.EXPECT().GetInfo(gomock.Any()).
					Return(nil, errors.New("mock error"))
				return mockNode
			},
			getInstructions: func(controller *gomock.Controller) map[instruction.Key]instruction.Instructions {
				return nil
			},
			expectedExitCode: 1,
		},
		"error getting instructions": {
			protocols: []string{protocol.ISCSI},
			getNode: func(controller *gomock.Controller) nodeinfo.Node {
				mockNode := mock_nodeinfo.NewMockNode(controller)
				mockNode.EXPECT().GetInfo(gomock.Any()).
					Return(
						&nodeinfo.NodeInfo{
							PkgMgr: nodeinfo.PkgMgrNone,
							Distro: nodeinfo.DistroUbuntu,
						},
						nil,
					)
				return mockNode
			},
			getInstructions: func(controller *gomock.Controller) map[instruction.Key]instruction.Instructions {
				return nil
			},
			expectedExitCode: 1,
		},
		"error executing instructions": {
			protocols: []string{protocol.ISCSI},
			getNode: func(controller *gomock.Controller) nodeinfo.Node {
				mockNode := mock_nodeinfo.NewMockNode(controller)
				mockNode.EXPECT().GetInfo(gomock.Any()).
					Return(
						&nodeinfo.NodeInfo{
							PkgMgr: nodeinfo.PkgMgrNone,
							Distro: nodeinfo.DistroUbuntu,
						},
						nil,
					)
				return mockNode
			},
			getInstructions: func(controller *gomock.Controller) map[instruction.Key]instruction.Instructions {
				key := instruction.Key{
					Protocol: protocol.ISCSI,
					Distro:   nodeinfo.DistroUbuntu,
					PkgMgr:   nodeinfo.PkgMgrNone,
				}
				mockInstruction := mock_instruction.NewMockInstructions(controller)
				mockInstruction.EXPECT().GetName().Return("mock instruction")
				mockInstruction.EXPECT().PreCheck(gomock.Any()).Return(nil)
				mockInstruction.EXPECT().Apply(gomock.Any()).Return(errors.New("some error"))
				instructions := map[instruction.Key]instruction.Instructions{
					key: mockInstruction,
				}
				return instructions
			},
			expectedExitCode: 1,
		},
		"happy path": {
			protocols: []string{protocol.ISCSI},
			getNode: func(controller *gomock.Controller) nodeinfo.Node {
				mockNode := mock_nodeinfo.NewMockNode(controller)
				mockNode.EXPECT().GetInfo(gomock.Any()).
					Return(
						&nodeinfo.NodeInfo{
							PkgMgr: nodeinfo.PkgMgrNone,
							Distro: nodeinfo.DistroUbuntu,
						},
						nil,
					)
				return mockNode
			},
			getInstructions: func(controller *gomock.Controller) map[instruction.Key]instruction.Instructions {
				key := instruction.Key{
					Protocol: protocol.ISCSI,
					Distro:   nodeinfo.DistroUbuntu,
					PkgMgr:   nodeinfo.PkgMgrNone,
				}
				return map[instruction.Key]instruction.Instructions{
					key: &instruction.Default{},
				}
			},
			expectedExitCode: 0,
		},
	}

	for name, params := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			defer instruction.ScopedInstructions(params.getInstructions(ctrl))()
			node := nodeprep.NewDetailed(params.getNode(ctrl))

			exitCode := node.Prepare(context.TODO(), params.protocols)
			assert.Equal(t, params.expectedExitCode, exitCode)
		})
	}
}
