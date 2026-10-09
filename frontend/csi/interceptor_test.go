// Copyright 2026 NetApp, Inc. All Rights Reserved.

package csi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
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

const (
	testSentinelUser = "SENTINEL_USER_7f2a"
	testSentinelPass = "SENTINEL_PAS_9d3f"
)

// captureLog points the package-global logger at a buffer, runs fn, and returns what was logged.
// The logger is process-global state shared with every other test in this package, so the setup
// TestMain applies is restored on cleanup.
func captureLog(t *testing.T, level, format string, fn func()) string {
	t.Helper()

	t.Cleanup(func() {
		if err := InitLogLevel("info"); err != nil {
			t.Fatalf("InitLogLevel(info): %v", err)
		}
		if err := InitLogFormat(TextFormat); err != nil {
			t.Fatalf("InitLogFormat(%s): %v", TextFormat, err)
		}
		InitAuditLogger(true)
		InitLogOutput(io.Discard)
	})

	if err := InitLogLevel(level); err != nil {
		t.Fatalf("InitLogLevel(%q): %v", level, err)
	}
	if err := InitLogFormat(format); err != nil {
		t.Fatalf("InitLogFormat(%q): %v", format, err)
	}
	InitAuditLogger(true)

	buf := &bytes.Buffer{}
	InitLogOutput(buf)
	fn()

	return buf.String()
}

// secretBearingRequests are generated CSI types rather than hand-written strings: a literal fixture
// would keep passing after a protobuf-runtime or CSI-spec bump changed how a real request renders,
// which is how the regex redaction of these lines went stale without any test noticing.
func secretBearingRequests() []struct {
	method string
	req    interface{}
} {
	secrets := map[string]string{"Username": testSentinelUser, "Password": testSentinelPass}

	return []struct {
		method string
		req    interface{}
	}{
		{csi.Controller_CreateVolume_FullMethodName, &csi.CreateVolumeRequest{
			Name:    "pvc-aaaa-bbbb",
			Secrets: secrets,
		}},
		{csi.Controller_ControllerPublishVolume_FullMethodName, &csi.ControllerPublishVolumeRequest{
			VolumeId: "pvc-aaaa-bbbb",
			NodeId:   "node-1",
			Secrets:  secrets,
		}},
		{csi.Controller_ControllerExpandVolume_FullMethodName, &csi.ControllerExpandVolumeRequest{
			VolumeId: "pvc-aaaa-bbbb",
			Secrets:  secrets,
		}},
		{csi.Node_NodeStageVolume_FullMethodName, &csi.NodeStageVolumeRequest{
			VolumeId:          "pvc-aaaa-bbbb",
			StagingTargetPath: "/var/lib/kubelet/plugins/kubernetes.io/csi",
			Secrets:           secrets,
		}},
		{csi.Node_NodePublishVolume_FullMethodName, &csi.NodePublishVolumeRequest{
			VolumeId:   "pvc-aaaa-bbbb",
			TargetPath: "/var/lib/kubelet/pods/uid/volumes/kubernetes.io~csi/pvc-aaaa-bbbb/mount",
			Secrets:    secrets,
		}},
		{csi.Node_NodeExpandVolume_FullMethodName, &csi.NodeExpandVolumeRequest{
			VolumeId:   "pvc-aaaa-bbbb",
			VolumePath: "/var/lib/kubelet/pods/uid/volumes/kubernetes.io~csi/pvc-aaaa-bbbb/mount",
			Secrets:    secrets,
		}},
	}
}

func TestLogGRPCInterceptorRedactsSecrets(t *testing.T) {
	handler := grpc.UnaryHandler(func(_ context.Context, _ interface{}) (interface{}, error) {
		return &csi.NodeStageVolumeResponse{}, nil
	})

	for _, tc := range secretBearingRequests() {
		// Vacuity guard: a pass below has to mean the log path scrubbed the sentinels, not that the
		// fixture lost them.
		if raw := fmt.Sprintf("%+v", tc.req); !strings.Contains(raw, testSentinelPass) {
			t.Fatalf("%s fixture carries no sentinel, so this test cannot detect a leak: %s", tc.method, raw)
		}

		for _, level := range []string{"debug", "trace"} {
			for _, format := range []string{TextFormat, JSONFormat} {
				out := captureLog(t, level, format, func() {
					if _, err := logGRPCInterceptor(context.Background(), tc.req,
						serverInfo(nil, tc.method), handler); err != nil {
						t.Fatalf("%s: logGRPCInterceptor: %v", tc.method, err)
					}
				})

				if !strings.Contains(out, "pvc-aaaa-bbbb") {
					t.Fatalf("%s level=%s format=%s: request never rendered, guard is vacuous\n%s",
						tc.method, level, format, out)
				}
				// The pass must come from stripping the field, not from a render that omitted the
				// request wholesale.
				if !strings.Contains(out, "***stripped***") {
					t.Errorf("%s level=%s format=%s: secrets field was not marked as stripped:\n%s",
						tc.method, level, format, out)
				}
				for _, sentinel := range []string{testSentinelUser, testSentinelPass} {
					if strings.Contains(out, sentinel) {
						t.Errorf("%s leaked sentinel %q at level=%s format=%s:\n%s",
							tc.method, sentinel, level, format, out)
					}
				}
			}
		}
	}
}

// TestLogGRPCInterceptorLogsResponseWithoutSecrets covers the trace-side render of the response. No
// CSI response message carries a csi_secret field (spec v1.12.0 annotates requests only), so this
// asserts the branch renders a non-secret response rather than asserting a strip.
func TestLogGRPCInterceptorLogsResponseWithoutSecrets(t *testing.T) {
	publishContext := map[string]string{"iscsiTargetIqn": "iqn.1992-08.com.netapp:sn.19f193c5"}
	handler := grpc.UnaryHandler(func(_ context.Context, _ interface{}) (interface{}, error) {
		return &csi.ControllerPublishVolumeResponse{PublishContext: publishContext}, nil
	})

	for _, format := range []string{TextFormat, JSONFormat} {
		out := captureLog(t, "trace", format, func() {
			if _, err := logGRPCInterceptor(context.Background(),
				&csi.ControllerPublishVolumeRequest{VolumeId: "pvc-aaaa-bbbb", NodeId: "node-1"},
				serverInfo(nil, csi.Controller_ControllerPublishVolume_FullMethodName), handler); err != nil {
				t.Fatalf("logGRPCInterceptor: %v", err)
			}
		})

		if !strings.Contains(out, "GRPC response:") {
			t.Fatalf("format=%s: response line never emitted, guard is vacuous\n%s", format, out)
		}
		if !strings.Contains(out, publishContext["iscsiTargetIqn"]) {
			t.Errorf("format=%s: non-secret response content dropped from the log:\n%s", format, out)
		}
	}
}

func TestTimeoutInterceptorNonPluginServerDoesNotLogRequest(t *testing.T) {
	req := &csi.NodeStageVolumeRequest{
		VolumeId: "pvc-aaaa-bbbb",
		Secrets:  map[string]string{"Username": testSentinelUser, "Password": testSentinelPass},
	}
	if raw := fmt.Sprintf("%+v", req); !strings.Contains(raw, testSentinelPass) {
		t.Fatalf("fixture carries no sentinel, so this test cannot detect a leak: %s", raw)
	}

	// The info level keeps the request line suppressed, so a sentinel in the output can only have
	// come from the warning's own fields.
	out := captureLog(t, "info", TextFormat, func() {
		if _, err := timeoutInterceptor(context.Background(), req,
			serverInfo("not-a-plugin", csi.Node_NodeStageVolume_FullMethodName),
			grpc.UnaryHandler(func(_ context.Context, _ interface{}) (interface{}, error) {
				return &csi.NodeStageVolumeResponse{}, nil
			})); err != nil {
			t.Fatalf("timeoutInterceptor: %v", err)
		}
	})

	if !strings.Contains(out, "gRPC unary server is not a Trident CSI plugin.") {
		t.Fatalf("expected warning never emitted, guard is vacuous\n%s", out)
	}
	for _, sentinel := range []string{testSentinelUser, testSentinelPass} {
		if strings.Contains(out, sentinel) {
			t.Errorf("warning leaked sentinel %q:\n%s", sentinel, out)
		}
	}
}
