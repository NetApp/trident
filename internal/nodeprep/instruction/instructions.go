// Copyright 2024 NetApp, Inc. All Rights Reserved.

package instruction

//go:generate mockgen -destination=../../../mocks/mock_internal/mock_nodeprep/mock_instruction/mock_instruction.go github.com/netapp/trident/internal/nodeprep/instruction  Instructions

import (
	"context"
	"fmt"

	"github.com/netapp/trident/internal/nodeprep/nodeinfo"
	"github.com/netapp/trident/internal/nodeprep/protocol"
	"github.com/netapp/trident/internal/nodeprep/step"
	utilserrors "github.com/netapp/trident/utils/errors"
)

// Planning for future distros and protocols which may not align easily with each other and need different instructions
// For example windows might need different instructions for iSCSI than linux or ROSA might be very different from amazon linux
// And some may not need package managers and some will

type Key struct {
	Protocol protocol.Protocol
	Distro   nodeinfo.Distro
	PkgMgr   nodeinfo.PkgMgr
}

var instructionMap = map[Key]Instructions{}

func init() {
	instructionMap[Key{Protocol: protocol.ISCSI, Distro: nodeinfo.DistroAmzn, PkgMgr: nodeinfo.PkgMgrYum}] = newRHELYumISCSI()
	instructionMap[Key{Protocol: protocol.ISCSI, Distro: nodeinfo.DistroUbuntu, PkgMgr: nodeinfo.PkgMgrApt}] = newDebianAptISCSI()
	instructionMap[Key{Protocol: protocol.ISCSI, Distro: nodeinfo.DistroRhcos, PkgMgr: nodeinfo.PkgMgrNone}] = newRHCOSISCSI()
	instructionMap[Key{Protocol: protocol.ISCSI, Distro: nodeinfo.DistroRhel, PkgMgr: nodeinfo.PkgMgrNone}] = newRHCOSISCSI()
	instructionMap[Key{Protocol: protocol.ISCSI, Distro: "", PkgMgr: nodeinfo.PkgMgrYum}] = newYumISCSI()
	instructionMap[Key{Protocol: protocol.ISCSI, Distro: "", PkgMgr: nodeinfo.PkgMgrApt}] = newAptISCSI()
}

type Instructions interface {
	GetName() string
	PreCheck(ctx context.Context) error
	Apply(ctx context.Context) error
	PostCheck(ctx context.Context) error
	GetSteps() []step.Step
}

// ScopedInstructions is used to mock out the default instructions for testing,
// this should not be called anywhere else other than tests
func ScopedInstructions(instructions map[Key]Instructions) func() {
	existingInstructions := instructionMap
	instructionMap = instructions
	return func() {
		instructionMap = existingInstructions
	}
}

func GetInstructions(nodeInfo *nodeinfo.NodeInfo, protocols []protocol.Protocol) ([]Instructions, error) {
	instructions := make([]Instructions, 0)
	for _, p := range protocols {
		if instruction, ok := instructionMap[Key{Protocol: p, Distro: nodeInfo.Distro, PkgMgr: nodeInfo.PkgMgr}]; ok {
			instructions = append(instructions, instruction)
		} else {
			if instruction, ok := instructionMap[Key{Protocol: p, Distro: "", PkgMgr: nodeInfo.PkgMgr}]; ok {
				instructions = append(instructions, instruction)
			}
		}
	}
	if len(instructions) == 0 {
		return nil, utilserrors.UnsupportedError(fmt.Sprintf("distribution %s for protocols %s is not supported", nodeInfo.Distro, protocols))
	}
	return instructions, nil
}
