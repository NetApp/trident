// Copyright 2026 NetApp, Inc. All Rights Reserved.

package csi

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	. "github.com/netapp/trident/logging"
)

// stubHandler returns a grpc.UnaryHandler that records the context it received and returns the given values.
func stubHandler(capturedCtx *context.Context, resp interface{}, err error) grpc.UnaryHandler {
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		*capturedCtx = ctx
		return resp, err
	}
}

func serverInfo(server interface{}, fullMethod string) *grpc.UnaryServerInfo {
	return &grpc.UnaryServerInfo{Server: server, FullMethod: fullMethod}
}

func TestOperationRegistry_ContainsAllControllerMethods(t *testing.T) {
	controllerMethods := []string{
		csi.Controller_CreateVolume_FullMethodName,
		csi.Controller_DeleteVolume_FullMethodName,
		csi.Controller_ControllerPublishVolume_FullMethodName,
		csi.Controller_ControllerUnpublishVolume_FullMethodName,
		csi.Controller_ValidateVolumeCapabilities_FullMethodName,
		csi.Controller_ListVolumes_FullMethodName,
		csi.Controller_ControllerGetCapabilities_FullMethodName,
		csi.Controller_CreateSnapshot_FullMethodName,
		csi.Controller_DeleteSnapshot_FullMethodName,
		csi.Controller_ListSnapshots_FullMethodName,
		csi.Controller_ControllerExpandVolume_FullMethodName,
	}
	for _, m := range controllerMethods {
		_, ok := operationRegistry[m]
		assert.True(t, ok, "missing registry entry for %s", m)
	}
}

func TestOperationRegistry_ContainsAllNodeMethods(t *testing.T) {
	nodeMethods := []string{
		csi.Node_NodeStageVolume_FullMethodName,
		csi.Node_NodeUnstageVolume_FullMethodName,
		csi.Node_NodePublishVolume_FullMethodName,
		csi.Node_NodeUnpublishVolume_FullMethodName,
		csi.Node_NodeGetVolumeStats_FullMethodName,
		csi.Node_NodeExpandVolume_FullMethodName,
		csi.Node_NodeGetCapabilities_FullMethodName,
		csi.Node_NodeGetInfo_FullMethodName,
	}
	for _, m := range nodeMethods {
		_, ok := operationRegistry[m]
		assert.True(t, ok, "missing registry entry for %s", m)
	}
}

func TestOperationRegistry_ContainsAllIdentityMethods(t *testing.T) {
	identityMethods := []string{
		csi.Identity_Probe_FullMethodName,
		csi.Identity_GetPluginInfo_FullMethodName,
		csi.Identity_GetPluginCapabilities_FullMethodName,
	}
	for _, m := range identityMethods {
		_, ok := operationRegistry[m]
		assert.True(t, ok, "missing registry entry for %s", m)
	}
}

func TestOperationRegistry_ContainsAllGroupControllerMethods(t *testing.T) {
	gcMethods := []string{
		csi.GroupController_GroupControllerGetCapabilities_FullMethodName,
		csi.GroupController_CreateVolumeGroupSnapshot_FullMethodName,
		csi.GroupController_GetVolumeGroupSnapshot_FullMethodName,
		csi.GroupController_DeleteVolumeGroupSnapshot_FullMethodName,
	}
	for _, m := range gcMethods {
		_, ok := operationRegistry[m]
		assert.True(t, ok, "missing registry entry for %s", m)
	}
}

func TestOperationRegistry_TotalEntryCount(t *testing.T) {
	assert.Equal(t, 26, len(operationRegistry))
}

func TestOperationRegistry_WorkflowsAreValid(t *testing.T) {
	for method, meta := range operationRegistry {
		assert.True(t, meta.Workflow.IsValid(), "invalid workflow for %s", method)
	}
}

func TestOperationRegistry_ClientsAreNonEmpty(t *testing.T) {
	for method, meta := range operationRegistry {
		assert.NotEmpty(t, meta.Client, "empty client for %s", method)
	}
}

func TestOperationRegistry_MethodsAreHTTPVerbs(t *testing.T) {
	validMethods := map[string]struct{}{
		http.MethodGet:    {},
		http.MethodPost:   {},
		http.MethodPut:    {},
		http.MethodDelete: {},
		http.MethodPatch:  {},
	}
	for method, meta := range operationRegistry {
		_, ok := validMethods[meta.Method]
		assert.True(t, ok, "unexpected HTTP method %q for %s", meta.Method, method)
	}
}

func TestMetricsInterceptor_RegisteredMethod_SetsContextValues(t *testing.T) {
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, "ok", nil)
	info := serverInfo(nil, csi.Controller_CreateVolume_FullMethodName)

	resp, err := incomingRequestMetricsInterceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "ok", resp)
	assert.Equal(t, ContextSourceCSI, handlerCtx.Value(ContextKeyRequestSource))
	assert.Equal(t, WorkflowVolumeCreate, handlerCtx.Value(ContextKeyWorkflow))
	assert.Equal(t, LogLayerCSIFrontend, handlerCtx.Value(ContextKeyLogLayer))
	assert.Equal(t, ContextRequestRoute(csi.Controller_CreateVolume_FullMethodName), handlerCtx.Value(ContextKeyRequestRoute))
	assert.Equal(t, ContextRequestClientCSIProvisioner, handlerCtx.Value(ContextKeyRequestClient))
	assert.Equal(t, ContextRequestMethod(http.MethodPost), handlerCtx.Value(ContextKeyRequestMethod))
}

func TestMetricsInterceptor_UnregisteredMethod_PassesThrough(t *testing.T) {
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, "passthrough", nil)
	info := serverInfo(nil, "/some.Unknown/Method")

	resp, err := incomingRequestMetricsInterceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, "passthrough", resp)
	// Context should not have workflow set by the interceptor.
	assert.Nil(t, handlerCtx.Value(ContextKeyWorkflow))
}

func TestMetricsInterceptor_PropagatesHandlerError(t *testing.T) {
	handlerErr := assert.AnError
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, handlerErr)
	info := serverInfo(nil, csi.Controller_DeleteVolume_FullMethodName)

	resp, err := incomingRequestMetricsInterceptor(context.Background(), nil, info, handler)

	assert.Nil(t, resp)
	assert.Equal(t, handlerErr, err)
}

func TestMetricsInterceptor_NodeMethod_SetsNodeWorkflow(t *testing.T) {
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, nil)
	info := serverInfo(nil, csi.Node_NodeStageVolume_FullMethodName)

	_, _ = incomingRequestMetricsInterceptor(context.Background(), nil, info, handler)

	assert.Equal(t, WorkflowNodeStage, handlerCtx.Value(ContextKeyWorkflow))
	assert.Equal(t, ContextRequestClientCSINodeClient, handlerCtx.Value(ContextKeyRequestClient))
	assert.Equal(t, ContextRequestMethod(http.MethodPut), handlerCtx.Value(ContextKeyRequestMethod))
}

func TestMetricsInterceptor_IdentityMethod_SetsIdentityWorkflow(t *testing.T) {
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, nil)
	info := serverInfo(nil, csi.Identity_Probe_FullMethodName)

	_, _ = incomingRequestMetricsInterceptor(context.Background(), nil, info, handler)

	assert.Equal(t, WorkflowIdentityProbe, handlerCtx.Value(ContextKeyWorkflow))
	assert.Equal(t, ContextRequestClientCSIAny, handlerCtx.Value(ContextKeyRequestClient))
}

func TestMetricsInterceptor_GroupControllerMethod_SetsGroupWorkflow(t *testing.T) {
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, nil)
	info := serverInfo(nil, csi.GroupController_CreateVolumeGroupSnapshot_FullMethodName)

	_, _ = incomingRequestMetricsInterceptor(context.Background(), nil, info, handler)

	assert.Equal(t, WorkflowGroupSnapshotCreate, handlerCtx.Value(ContextKeyWorkflow))
	assert.Equal(t, ContextRequestClientCSISnapshotter, handlerCtx.Value(ContextKeyRequestClient))
}

func TestTimeoutInterceptor_NodeRole_AppliesDeadline(t *testing.T) {
	plugin := &Plugin{role: CSINode}
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, nil)
	info := serverInfo(plugin, csi.Node_NodeStageVolume_FullMethodName)

	_, err := timeoutInterceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	deadline, ok := handlerCtx.Deadline()
	assert.True(t, ok, "expected deadline to be set")
	assert.WithinDuration(t, time.Now().Add(csiNodeRequestTimeout), deadline, 5*time.Second)
}

func TestTimeoutInterceptor_AllInOneRole_NoDeadline(t *testing.T) {
	plugin := &Plugin{role: CSIAllInOne}
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, nil)
	info := serverInfo(plugin, csi.Node_NodePublishVolume_FullMethodName)

	_, err := timeoutInterceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	_, ok := handlerCtx.Deadline()
	assert.False(t, ok, "unexpected deadline for allInOne role")
}

func TestTimeoutInterceptor_ControllerRole_NoDeadline(t *testing.T) {
	plugin := &Plugin{role: CSIController}
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, nil)
	info := serverInfo(plugin, csi.Controller_CreateVolume_FullMethodName)

	_, err := timeoutInterceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	_, ok := handlerCtx.Deadline()
	assert.False(t, ok, "expected no deadline for controller role")
}

func TestTimeoutInterceptor_NonPluginServer_NoDeadline(t *testing.T) {
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, nil)
	info := serverInfo("not-a-plugin", csi.Node_NodeStageVolume_FullMethodName)

	_, err := timeoutInterceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	_, ok := handlerCtx.Deadline()
	assert.False(t, ok, "expected no deadline when server is not *Plugin")
}

func TestTimeoutInterceptor_PropagatesHandlerError(t *testing.T) {
	plugin := &Plugin{role: CSINode}
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, assert.AnError)
	info := serverInfo(plugin, csi.Node_NodeUnstageVolume_FullMethodName)

	_, err := timeoutInterceptor(context.Background(), nil, info, handler)

	assert.Equal(t, assert.AnError, err)
}

func TestTimeoutInterceptor_PreservesExistingDeadline(t *testing.T) {
	plugin := &Plugin{role: CSIController}
	existingDeadline := time.Now().Add(30 * time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), existingDeadline)
	defer cancel()

	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, nil)
	info := serverInfo(plugin, csi.Controller_ListVolumes_FullMethodName)

	_, err := timeoutInterceptor(ctx, nil, info, handler)

	require.NoError(t, err)
	deadline, ok := handlerCtx.Deadline()
	assert.True(t, ok, "expected existing deadline to survive")
	assert.Equal(t, existingDeadline, deadline)
}

func initAuditForTest(t *testing.T) {
	t.Helper()
	InitAuditLogger(true)
}

func TestLogGRPCInterceptor_SetsRequestID(t *testing.T) {
	initAuditForTest(t)
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, nil)
	info := serverInfo(nil, csi.Controller_CreateVolume_FullMethodName)

	_, err := logGRPCInterceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	reqID := handlerCtx.Value(ContextKeyRequestID)
	assert.NotNil(t, reqID)
	assert.NotEmpty(t, reqID)
}

func TestLogGRPCInterceptor_SetsSourceToCSI(t *testing.T) {
	initAuditForTest(t)
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, nil)
	info := serverInfo(nil, csi.Identity_Probe_FullMethodName)

	_, _ = logGRPCInterceptor(context.Background(), nil, info, handler)

	assert.Equal(t, ContextSourceCSI, handlerCtx.Value(ContextKeyRequestSource))
}

func TestLogGRPCInterceptor_PropagatesResponse(t *testing.T) {
	initAuditForTest(t)
	expected := "the-response"
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, expected, nil)
	info := serverInfo(nil, csi.Controller_ListVolumes_FullMethodName)

	resp, err := logGRPCInterceptor(context.Background(), nil, info, handler)

	require.NoError(t, err)
	assert.Equal(t, expected, resp)
}

func TestLogGRPCInterceptor_PropagatesError(t *testing.T) {
	initAuditForTest(t)
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, assert.AnError)
	info := serverInfo(nil, csi.Controller_DeleteVolume_FullMethodName)

	_, err := logGRPCInterceptor(context.Background(), nil, info, handler)

	assert.Equal(t, assert.AnError, err)
}

func TestChainOrder_TimeoutContextVisibleToMetricsInterceptor(t *testing.T) {
	plugin := &Plugin{role: CSINode}
	var handlerCtx context.Context
	handler := stubHandler(&handlerCtx, nil, nil)
	info := serverInfo(plugin, csi.Node_NodeStageVolume_FullMethodName)

	// Simulate the chain: timeout -> metrics -> handler
	chained := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (interface{}, error) {
		return timeoutInterceptor(ctx, req, info, func(ctx context.Context, req interface{}) (interface{}, error) {
			return incomingRequestMetricsInterceptor(ctx, req, info, h)
		})
	}

	_, err := chained(context.Background(), nil, info, handler)

	require.NoError(t, err)
	// The handler should see both the deadline (from timeout) and the workflow (from metrics).
	_, hasDeadline := handlerCtx.Deadline()
	assert.True(t, hasDeadline, "timeout interceptor's deadline should be visible through the chain")
	assert.Equal(t, WorkflowNodeStage, handlerCtx.Value(ContextKeyWorkflow))
}
