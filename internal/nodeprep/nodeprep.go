// Copyright 2024 NetApp, Inc. All Rights Reserved.

package nodeprep

import (
	"github.com/netapp/trident/internal/chwrap"
	. "github.com/netapp/trident/logging"
	utilserrors "github.com/netapp/trident/utils/errors"
)

type PkgMgr = string

const (
	PkgMgrYum PkgMgr = "yum"
	PkgMgrApt PkgMgr = "apt"
)

type NodePrep struct {
	findBinary func(string) (string, string)
}

func NewNodePrep() *NodePrep {
	return NewNodePrepDetailed(chwrap.FindBinary)
}

func NewNodePrepDetailed(findBinary func(string) (string, string)) *NodePrep {
	return &NodePrep{findBinary: findBinary}
}

func (n *NodePrep) PrepareNode(requestedProtocols []string) (exitCode int) {
	Log().WithField("requestedProtocols", requestedProtocols).Info("Preparing node")
	defer func() {
		Log().WithField("exitCode", exitCode).Info("Node preparation complete")
	}()

	if len(requestedProtocols) == 0 {
		Log().Info("No protocols requested, exiting")
		return 0
	}

	pkgMgr, err := n.getPkgMgr()
	if err != nil {
		Log().WithError(err).Error("Failed to determine package manager")
		return 1
	}
	Log().Info("Package manager found: ", pkgMgr)

	return 0
}

func (n *NodePrep) getPkgMgr() (PkgMgr, error) {
	if _, pkgMgr := n.findBinary(string(PkgMgrYum)); pkgMgr != "" {
		return PkgMgrYum, nil
	}
	if _, pkgMgr := n.findBinary(string(PkgMgrApt)); pkgMgr != "" {
		return PkgMgrApt, nil
	}
	return "", utilserrors.UnsupportedError("a supported package manager was not found")
}
