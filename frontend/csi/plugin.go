// Copyright 2018 NetApp, Inc. All Rights Reserved.

package csi

import (
	"errors"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
)

type Plugin struct {
	orchestrator core.Orchestrator

	name     string
	nodeName string
	version  string
	endpoint string

	grpc NonBlockingGRPCServer

	csCap []*csi.ControllerServiceCapability
	nsCap []*csi.NodeServiceCapability
	vCap  []*csi.VolumeCapability_AccessMode
}

func NewPlugin(nodeName, endpoint string, orchestrator core.Orchestrator) (*Plugin, error) {

	log.WithFields(log.Fields{
		"name":    csiPluginName,
		"version": tridentconfig.OrchestratorVersion,
	}).Info("Initializing CSI frontend.")

	if endpoint == "" {
		err := errors.New("endpoint missing")
		log.Error(err)
		return nil, err
	}

	if nodeName == "" {
		err := errors.New("node name missing")
		log.Error(err)
		return nil, err
	}

	p := &Plugin{
		orchestrator: orchestrator,
		name:         csiPluginName,
		nodeName:     nodeName,
		version:      tridentconfig.OrchestratorVersion.ShortString(),
		endpoint:     endpoint,
	}

	// Define controller capabilities
	p.addControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
	})

	// Define node capabilities
	p.addNodeServiceCapabilities([]csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
	})

	// Define volume capabilities
	p.addVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	return p, nil
}

func (p *Plugin) Activate() error {
	go func() {
		log.Info("Activating CSI frontend.")
		p.grpc = NewNonBlockingGRPCServer()
		p.grpc.Start(p.endpoint, p, p, p)
	}()
	return nil
}

func (p *Plugin) Deactivate() error {
	log.Info("Deactivating CSI frontend.")
	p.grpc.GracefulStop()
	return nil
}

func (p *Plugin) GetName() string {
	return string(tridentconfig.ContextCSI)
}

func (p *Plugin) Version() string {
	return csiVersion
}

func (p *Plugin) addControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) {

	var csCap []*csi.ControllerServiceCapability

	for _, c := range cl {
		log.WithField("capability", c.String()).Info("Enabling controller service capability.")
		csCap = append(csCap, NewControllerServiceCapability(c))
	}

	p.csCap = csCap
}

func (p *Plugin) addNodeServiceCapabilities(cl []csi.NodeServiceCapability_RPC_Type) {

	var nsCap []*csi.NodeServiceCapability

	for _, c := range cl {
		log.WithField("capability", c.String()).Info("Enabling node service capability.")
		nsCap = append(nsCap, NewNodeServiceCapability(c))
	}

	p.nsCap = nsCap
}

func (p *Plugin) addVolumeCapabilityAccessModes(vc []csi.VolumeCapability_AccessMode_Mode) {

	var vCap []*csi.VolumeCapability_AccessMode

	for _, c := range vc {
		log.WithField("mode", c.String()).Info("Enabling volume access mode.")
		vCap = append(vCap, NewVolumeCapabilityAccessMode(c))
	}

	p.vCap = vCap
}

func (p *Plugin) getCSIErrorForOrchestratorError(err error) error {
	if core.IsNotReadyError(err) {
		return status.Error(codes.Unavailable, err.Error())
	} else if core.IsBootstrapError(err) {
		return status.Error(codes.FailedPrecondition, err.Error())
	} else if core.IsNotFoundError(err) {
		return status.Error(codes.NotFound, err.Error())
	} else {
		return status.Error(codes.Unknown, err.Error())
	}
}
