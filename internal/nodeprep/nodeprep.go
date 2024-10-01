// Copyright 2024 NetApp, Inc. All Rights Reserved.

package nodeprep

import (
	"context"

	"github.com/netapp/trident/internal/nodeprep/execution"
	"github.com/netapp/trident/internal/nodeprep/instruction"
	"github.com/netapp/trident/internal/nodeprep/nodeinfo"
	"github.com/netapp/trident/internal/nodeprep/protocol"
	. "github.com/netapp/trident/logging"
)

type NodePrep struct {
	node nodeinfo.Node
}

func New() *NodePrep {
	return NewDetailed(nodeinfo.New())
}

func NewDetailed(node nodeinfo.Node) *NodePrep {
	return &NodePrep{
		node: node,
	}
}

func (n *NodePrep) Prepare(ctx context.Context, requestedProtocols []protocol.Protocol) (exitCode int) {
	Log().WithField("requestedProtocols", requestedProtocols).Info("Preparing node")
	defer func() {
		Log().WithField("exitCode", exitCode).Info("Node preparation complete")
	}()

	if len(requestedProtocols) == 0 {
		Log().Info("No protocols requested, exiting")
		return 0
	}

	nodeInfo, err := n.node.GetInfo(ctx)
	if err != nil {
		Log().WithError(err).Error("Failed to determine node information")
		return 1
	}

	instructions, err := instruction.GetInstructions(nodeInfo, requestedProtocols)
	if err != nil {
		Log().WithError(err).Error("Failed to get instructions")
		return 1
	}

	if err = execution.Execute(ctx, instructions); err != nil {
		Log().WithError(err).Error("Failed to prepare node")
		return 1
	}

	return 0
}
