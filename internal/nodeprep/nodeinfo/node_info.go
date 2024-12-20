// Copyright 2024 NetApp, Inc. All Rights Reserved.

package nodeinfo

//go:generate mockgen -destination=../../../mocks/mock_internal/mock_nodeprep/mock_nodeinfo/mock_nodeinfo.go github.com/netapp/trident/internal/nodeprep/nodeinfo  Node

import (
	"context"
	"strings"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/osutils"
)

type NodeInfo struct {
	PkgMgr     PkgMgr
	Distro     Distro
	HostSystem models.HostSystem
}

type (
	Distro = string
	PkgMgr = string
)

const (
	DistroUbuntu  Distro = osutils.Ubuntu
	DistroAmzn    Distro = "amzn"
	DistroRhcos   Distro = "rhcos"
	DistroUnknown        = "unknown"

	PkgMgrYum  PkgMgr = "yum"
	PkgMgrApt  PkgMgr = "apt"
	PkgMgrNone PkgMgr = "none"
)

type Node interface {
	GetInfo(ctx context.Context) (*NodeInfo, error)
}

type NodeClient struct {
	os     osutils.Utils
	binary Binary
}

func New() *NodeClient {
	return NewDetailed(osutils.New(), NewBinary())
}

func NewDetailed(osUtils osutils.Utils, binary Binary) *NodeClient {
	return &NodeClient{
		os:     osUtils,
		binary: binary,
	}
}

func (n *NodeClient) GetInfo(ctx context.Context) (*NodeInfo, error) {
	hostSystem, err := n.os.GetHostSystemInfo(ctx)
	if err != nil {
		return nil, err
	}

	Log().WithFields(LogFields{
		"HostSystem": hostSystem,
		"OS":         hostSystem.OS,
		"distro":     hostSystem.OS.Distro,
	}).Info("Host system information")

	distro := checkDistro(hostSystem.OS.Distro)

	nodeType := &NodeInfo{
		PkgMgr:     n.getPkgMgr(),
		HostSystem: *hostSystem,
		Distro:     distro,
	}

	return nodeType, nil
}

func (n *NodeClient) getPkgMgr() PkgMgr {
	if fullPath := n.binary.FindPath(PkgMgrYum); fullPath != "" {
		return PkgMgrYum
	}
	if fullPath := n.binary.FindPath(PkgMgrApt); fullPath != "" {
		return PkgMgrApt
	}
	return PkgMgrNone
}

func checkDistro(distro string) Distro {
	if strings.TrimSpace(distro) == "" {
		return DistroUnknown
	}
	return distro
}
