// Copyright 2025 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

func (p *Plugin) Probe(
	ctx context.Context, req *csi.ProbeRequest,
) (*csi.ProbeResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowIdentityProbe)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "Probe", "Type": "CSI_Identity"}
	Logc(ctx).WithFields(fields).Trace(">>>> Probe")
	defer Logc(ctx).WithFields(fields).Trace("<<<< Probe")

	// Ensure Trident bootstrapped OK.  We only return an error if Trident bootstrapping
	// failed (i.e. unrecoverable), not if Trident is still initializing.
	_, err := p.orchestrator.GetVersion(ctx)
	if errors.IsBootstrapError(err) {
		return &csi.ProbeResponse{}, status.Error(codes.FailedPrecondition, err.Error())
	}

	return &csi.ProbeResponse{}, nil
}

func (p *Plugin) GetPluginInfo(
	ctx context.Context, req *csi.GetPluginInfoRequest,
) (*csi.GetPluginInfoResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowIdentityGetInfo)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "GetPluginInfo", "Type": "CSI_Identity"}
	Logc(ctx).WithFields(fields).Trace(">>>> GetPluginInfo")
	defer Logc(ctx).WithFields(fields).Trace("<<<< GetPluginInfo")

	return &csi.GetPluginInfoResponse{
		Name:          p.name,
		VendorVersion: p.version,
	}, nil
}

func (p *Plugin) GetPluginCapabilities(
	ctx context.Context, req *csi.GetPluginCapabilitiesRequest,
) (*csi.GetPluginCapabilitiesResponse, error) {
	ctx = SetContextWorkflow(ctx, WorkflowIdentityGetCapabilities)
	ctx = GenerateRequestContextForLayer(ctx, LogLayerCSIFrontend)

	fields := LogFields{"Method": "GetPluginCapabilities", "Type": "CSI_Identity", "topologyInUse": p.topologyInUse}
	Logc(ctx).WithFields(fields).Trace(">>>> GetPluginCapabilities")
	defer Logc(ctx).WithFields(fields).Trace("<<<< GetPluginCapabilities")

	// Add controller service capability
	csiPluginCap := []*csi.PluginCapability{
		{
			Type: &csi.PluginCapability_Service_{
				Service: &csi.PluginCapability_Service{
					Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
				},
			},
		},
		{
			Type: &csi.PluginCapability_Service_{
				Service: &csi.PluginCapability_Service{
					Type: csi.PluginCapability_Service_GROUP_CONTROLLER_SERVICE,
				},
			},
		},
		{
			Type: &csi.PluginCapability_VolumeExpansion_{
				VolumeExpansion: &csi.PluginCapability_VolumeExpansion{
					Type: csi.PluginCapability_VolumeExpansion_ONLINE,
				},
			},
		},
	}

	// If topology is in use, add VOLUME_ACCESSIBILITY_CONSTRAINTS capability
	if p.topologyInUse {
		Logc(ctx).WithFields(fields).Info("Topology is in use. Adding VOLUME_ACCESSIBILITY_CONSTRAINTS capability.")
		csiPluginCap = append(csiPluginCap, &csi.PluginCapability{
			Type: &csi.PluginCapability_Service_{
				Service: &csi.PluginCapability_Service{
					Type: csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS,
				},
			},
		})
	}

	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: csiPluginCap,
	}, nil
}
