// Copyright 2026 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc"

	. "github.com/netapp/trident/logging"
)

// csiNodeRequestTimeout is the default timeout applied to Node data-path RPCs.
var csiNodeRequestTimeout = 110 * time.Second

type operationMeta struct {
	Workflow Workflow
	Client   ContextRequestClient
	Method   ContextRequestMethod
}

// operationRegistry maps complete CSI routes -> workflow metadata.
var operationRegistry = map[string]operationMeta{
	// Controller (11)
	csi.Controller_CreateVolume_FullMethodName:               {WorkflowVolumeCreate, ContextRequestClientCSIProvisioner, http.MethodPost},
	csi.Controller_DeleteVolume_FullMethodName:               {WorkflowVolumeDelete, ContextRequestClientCSIProvisioner, http.MethodDelete},
	csi.Controller_ControllerPublishVolume_FullMethodName:    {WorkflowControllerPublish, ContextRequestClientCSIAttacher, http.MethodPost},
	csi.Controller_ControllerUnpublishVolume_FullMethodName:  {WorkflowControllerUnpublish, ContextRequestClientCSIAttacher, http.MethodDelete},
	csi.Controller_ValidateVolumeCapabilities_FullMethodName: {WorkflowVolumeGetCapabilities, ContextRequestClientCSIProvisioner, http.MethodGet},
	csi.Controller_ListVolumes_FullMethodName:                {WorkflowVolumeList, ContextRequestClientCSIProvisioner, http.MethodGet},
	csi.Controller_ControllerGetCapabilities_FullMethodName:  {WorkflowControllerGetCapabilities, ContextRequestClientCSIAny, http.MethodGet},
	csi.Controller_CreateSnapshot_FullMethodName:             {WorkflowSnapshotCreate, ContextRequestClientCSISnapshotter, http.MethodPost},
	csi.Controller_DeleteSnapshot_FullMethodName:             {WorkflowSnapshotDelete, ContextRequestClientCSISnapshotter, http.MethodDelete},
	csi.Controller_ListSnapshots_FullMethodName:              {WorkflowSnapshotList, ContextRequestClientCSISnapshotter, http.MethodGet},
	csi.Controller_ControllerExpandVolume_FullMethodName:     {WorkflowVolumeResize, ContextRequestClientCSIResizer, http.MethodPatch},

	// Node (8) — 5 with HasKubeletTimeout: true
	csi.Node_NodeStageVolume_FullMethodName:     {WorkflowNodeStage, ContextRequestClientCSINodeClient, http.MethodPut},
	csi.Node_NodeUnstageVolume_FullMethodName:   {WorkflowNodeUnstage, ContextRequestClientCSINodeClient, http.MethodDelete},
	csi.Node_NodePublishVolume_FullMethodName:   {WorkflowNodePublish, ContextRequestClientCSINodeClient, http.MethodPut},
	csi.Node_NodeUnpublishVolume_FullMethodName: {WorkflowNodeUnpublish, ContextRequestClientCSINodeClient, http.MethodDelete},
	csi.Node_NodeGetVolumeStats_FullMethodName:  {WorkflowVolumeGetStats, ContextRequestClientCSINodeClient, http.MethodGet},
	csi.Node_NodeExpandVolume_FullMethodName:    {WorkflowVolumeResize, ContextRequestClientCSINodeClient, http.MethodPatch},
	csi.Node_NodeGetCapabilities_FullMethodName: {WorkflowNodeGetCapabilities, ContextRequestClientCSINodeClient, http.MethodGet},
	csi.Node_NodeGetInfo_FullMethodName:         {WorkflowNodeGetInfo, ContextRequestClientCSINodeClient, http.MethodGet},

	// Identity (3)
	csi.Identity_Probe_FullMethodName:                 {WorkflowIdentityProbe, ContextRequestClientCSIAny, http.MethodGet},
	csi.Identity_GetPluginInfo_FullMethodName:         {WorkflowIdentityGetInfo, ContextRequestClientCSIAny, http.MethodGet},
	csi.Identity_GetPluginCapabilities_FullMethodName: {WorkflowIdentityGetCapabilities, ContextRequestClientCSIAny, http.MethodGet},

	// GroupController (4)
	csi.GroupController_GroupControllerGetCapabilities_FullMethodName: {WorkflowGroupControllerGetCapabilities, ContextRequestClientCSIAny, http.MethodGet},
	csi.GroupController_CreateVolumeGroupSnapshot_FullMethodName:      {WorkflowGroupSnapshotCreate, ContextRequestClientCSISnapshotter, http.MethodPost},
	csi.GroupController_GetVolumeGroupSnapshot_FullMethodName:         {WorkflowGroupSnapshotGet, ContextRequestClientCSISnapshotter, http.MethodGet},
	csi.GroupController_DeleteVolumeGroupSnapshot_FullMethodName:      {WorkflowGroupSnapshotDelete, ContextRequestClientCSISnapshotter, http.MethodDelete},
}

func incomingRequestMetricsInterceptor(
	ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (resp any, err error) {
	// Look up operation metadata.
	meta, ok := operationRegistry[info.FullMethod]
	if !ok {
		// Unregistered method — pass through without decorating the context or adding any telemetry.
		return handler(ctx, req)
	}

	// Build enriched context with telemetry.
	ctx, rec := NewContextBuilder(ctx).
		WithSource(ContextSourceCSI).
		WithWorkflow(meta.Workflow).
		WithLayer(LogLayerCSIFrontend).
		WithRoute(ContextRequestRoute(info.FullMethod)).
		WithIncomingAPIMetrics(meta.Client, meta.Method).
		BuildContextAndTelemetry()
	defer rec(&err)

	return handler(ctx, req)
}

// timeoutInterceptor applies a default request timeout for Node deployments.
// All RPCs served by the Node role receive the timeout; Controller and AIO deployments are unaffected.
// It must be the second from outermost interceptor in the chain so that downstream interceptors
// capture the timeout-aware context.
func timeoutInterceptor(
	ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (any, error) {
	plugin, ok := info.Server.(*Plugin)
	if !ok {
		Logc(ctx).WithFields(LogFields{
			"server":  info.Server,
			"request": req,
		}).Warn("gRPC unary server is not a Trident CSI plugin.")
		return handler(ctx, req)
	}

	var timeout time.Duration
	switch plugin.role {
	case CSINode:
		timeout = csiNodeRequestTimeout
	case CSIAllInOne, CSIController:
		// No default timeouts for CSI controller or group controller operations.
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	return handler(ctx, req)
}

// logGRPCInterceptor sets the base context, logs and audit logs all incoming gRPC requests.
// It should always be the first interceptor in the chain.
// All gRPCs, regardless of timeout, should always be logged.
func logGRPCInterceptor(
	ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (resp any, err error) {
	// Preserve existing logGRPC behavior: request ID + source.
	ctx = GenerateRequestContext(ctx, "", ContextSourceCSI, WorkflowNone, LogLayerCSIFrontend)
	Audit().Logf(ctx, AuditGRPCAccess, LogFields{}, "GRPC call: %s", info.FullMethod)
	Logc(ctx).WithFields(LogFields{
		"Request": fmt.Sprintf("GRPC request: %+v", req),
	}).Debugf("GRPC call: %s", info.FullMethod)

	resp, err = handler(ctx, req)
	if err != nil {
		Logc(ctx).Errorf("GRPC error: %v", err)
	} else {
		Logc(ctx).Tracef("GRPC response: %+v", resp)
	}

	return
}
