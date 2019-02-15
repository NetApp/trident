// Copyright 2019 NetApp, Inc. All Rights Reserved.

package csi

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/netapp/trident/core"
)

func (p *Plugin) Probe(
	ctx context.Context, req *csi.ProbeRequest,
) (*csi.ProbeResponse, error) {

	fields := log.Fields{"Method": "Probe", "Type": "CSI_Identity"}
	log.WithFields(fields).Debug(">>>> Probe")
	defer log.WithFields(fields).Debug("<<<< Probe")

	// Ensure Trident bootstrapped OK.  We only return an error if Trident bootstrapping
	// failed (i.e. unrecoverable), not if Trident is still initializing.
	_, err := p.orchestrator.GetVersion()
	if core.IsBootstrapError(err) {
		return &csi.ProbeResponse{}, status.Error(codes.FailedPrecondition, err.Error())
	}

	return &csi.ProbeResponse{}, nil
}

func (p *Plugin) GetPluginInfo(
	ctx context.Context, req *csi.GetPluginInfoRequest,
) (*csi.GetPluginInfoResponse, error) {

	fields := log.Fields{"Method": "GetPluginInfo", "Type": "CSI_Identity"}
	log.WithFields(fields).Debug(">>>> GetPluginInfo")
	defer log.WithFields(fields).Debug("<<<< GetPluginInfo")

	return &csi.GetPluginInfoResponse{
		Name:          p.name,
		VendorVersion: p.version,
	}, nil
}

func (p *Plugin) GetPluginCapabilities(
	ctx context.Context, req *csi.GetPluginCapabilitiesRequest,
) (*csi.GetPluginCapabilitiesResponse, error) {

	fields := log.Fields{"Method": "GetPluginCapabilities", "Type": "CSI_Identity"}
	log.WithFields(fields).Debug(">>>> GetPluginCapabilities")
	defer log.WithFields(fields).Debug("<<<< GetPluginCapabilities")

	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
		},
	}, nil
}
