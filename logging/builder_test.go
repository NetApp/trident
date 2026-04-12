// Copyright 2026 NetApp, Inc. All Rights Reserved.

package logging

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewContextBuilder_Defaults(t *testing.T) {
	var nilCtx context.Context
	cb := NewContextBuilder(nilCtx)
	ctx, tels := cb()
	assert.NotNil(t, ctx)
	assert.Empty(t, tels)
}

func TestContextBuilder_WithContext_MergesValuesKeepsCancel(t *testing.T) {
	parent := context.Background()
	parent = context.WithValue(parent, ContextKeyRequestID, "parentID")
	child := context.Background()
	child = context.WithValue(child, ContextKeyRequestTarget, ContextRequestTargetKubernetes)
	cb := NewContextBuilder(parent)

	ctx, _ := cb.WithContext(child)()
	assert.Equal(t, ContextRequestTargetKubernetes, ctx.Value(ContextKeyRequestTarget))
	assert.Equal(t, "parentID", ctx.Value(ContextKeyRequestID))
}

func TestContextBuilder_WithParent_ReparentsContext(t *testing.T) {
	orig := context.Background()
	orig = context.WithValue(orig, ContextKeyRequestClient, ContextRequestClientTridentCLI)
	parent := context.Background()
	parent = context.WithValue(parent, ContextKeyRequestID, "newParentID")
	cb := NewContextBuilder(orig)

	ctx, _ := cb.WithParent(parent)()
	assert.Equal(t, "newParentID", ctx.Value(ContextKeyRequestID))
	assert.Equal(t, ContextRequestClientTridentCLI, ctx.Value(ContextKeyRequestClient))
}

func TestContextBuilder_WithWorkflowAndLayer(t *testing.T) {
	cb := NewContextBuilder(context.Background())
	ctx, _ := cb.WithWorkflow(WorkflowNone).WithLayer(LogLayer("adapter"))()
	assert.Equal(t, WorkflowNone, getContextWorkflow(ctx))
	assert.Equal(t, LogLayer("adapter"), getContextLayer(ctx))
}

func TestContextBuilder_RequestInfoSetters(t *testing.T) {
	cb := NewContextBuilder(context.Background())

	ctx, _ := cb.
		WithSource(string(ContextSourceREST)).
		WithClient(ContextRequestClientCSINodeClient).
		WithTarget(ContextRequestTargetONTAP).
		WithAddress(ContextRequestAddress("10.0.0.2")).
		WithRoute(ContextRequestRoute("/v1/volumes")).
		WithMethod(ContextRequestMethod("POST"))()

	assert.Equal(t, ContextSource(ContextSourceREST), getContextSource(ctx))
	assert.Equal(t, ContextRequestClientCSINodeClient, getContextClient(ctx))
	assert.Equal(t, ContextRequestTargetONTAP, getContextTarget(ctx))
	assert.Equal(t, ContextRequestAddress("10.0.0.2"), getContextAddress(ctx))
	assert.Equal(t, ContextRequestRoute("/v1/volumes"), getContextRoute(ctx))
	assert.Equal(t, ContextRequestMethod("POST"), getContextMethod(ctx))
}

func TestContextBuilder_BuildContext(t *testing.T) {
	cb := NewContextBuilder(context.Background()).WithClient(ContextRequestClientTridentCLI)
	ctx := cb.BuildContext()
	assert.Equal(t, ContextRequestClientTridentCLI, getContextClient(ctx))
}

func TestContextBuilder_ChainingOrderPreserved(t *testing.T) {
	cb := NewContextBuilder(context.Background())
	c1 := cb.WithClient(ContextRequestClientTridentCLI)
	c2 := c1.WithClient(ContextRequestClientCSINodeClient)
	ctx := c2.BuildContext()
	assert.Equal(t, ContextRequestClientCSINodeClient, getContextClient(ctx))
}

// ---- WithIncomingAPIMetrics ----

func TestContextBuilder_WithIncomingAPIMetrics_SetsContextValues(t *testing.T) {
	client := ContextRequestClient(uniqueLabel(t, "client"))
	method := ContextRequestMethod(uniqueLabel(t, "method"))

	ctx := NewContextBuilder(context.Background()).
		WithIncomingAPIMetrics(client, method).
		BuildContext()

	assert.Equal(t, client, getContextClient(ctx))
	assert.Equal(t, method, getContextMethod(ctx))
}

func TestContextBuilder_WithIncomingAPIMetrics_RecorderAffectsMetrics(t *testing.T) {
	client := ContextRequestClient(uniqueLabel(t, "client"))
	method := ContextRequestMethod(uniqueLabel(t, "method"))
	clientStr, methodStr := string(client), string(method)

	_, recorder := NewContextBuilder(context.Background()).
		WithIncomingAPIMetrics(client, method).
		BuildContextAndTelemetry()

	// In-flight should be 1 immediately after the telemeter is staged.
	assert.Equal(t, float64(1), readGauge(incomingAPIRequestsInFlight, clientStr, methodStr))

	var err error
	recorder(&err)

	// In-flight should return to 0 after the recorder fires.
	assert.Equal(t, float64(0), readGauge(incomingAPIRequestsInFlight, clientStr, methodStr))
	// Exactly one duration observation should have been recorded.
	assert.Equal(t, uint64(1),
		readHistogramCount(incomingAPIRequestDurationSeconds, metricStatusSuccess, clientStr, methodStr),
		"expected exactly one duration observation with status=success",
	)
}

func TestContextBuilder_WithIncomingAPIMetrics_RecorderIsIdempotent(t *testing.T) {
	// Calling the Recorder more than once must be a no-op (sync.Once guarantee).
	client := ContextRequestClient(uniqueLabel(t, "client"))
	method := ContextRequestMethod(uniqueLabel(t, "method"))
	clientStr, methodStr := string(client), string(method)

	_, recorder := NewContextBuilder(context.Background()).
		WithIncomingAPIMetrics(client, method).
		BuildContextAndTelemetry()

	var err error
	recorder(&err)
	recorder(&err) // second call must not mutate any metric

	assert.Equal(t, float64(0), readGauge(incomingAPIRequestsInFlight, clientStr, methodStr),
		"in-flight gauge must not go negative when the recorder is called more than once",
	)
	assert.Equal(t, uint64(1),
		readHistogramCount(incomingAPIRequestDurationSeconds, metricStatusSuccess, clientStr, methodStr),
		"duration must only be observed once even when the recorder is called multiple times",
	)
}

// ---- WithOutgoingAPIMetrics ----

func TestContextBuilder_WithOutgoingAPIMetrics_SetsContextValues(t *testing.T) {
	target := ContextRequestTarget(uniqueLabel(t, "target"))
	address := ContextRequestAddress(uniqueLabel(t, "address"))
	method := ContextRequestMethod(uniqueLabel(t, "method"))

	ctx := NewContextBuilder(context.Background()).
		WithOutgoingAPIMetrics(target, address, method).
		BuildContext()

	assert.Equal(t, target, getContextTarget(ctx))
	assert.Equal(t, address, getContextAddress(ctx))
	assert.Equal(t, method, getContextMethod(ctx))
}

func TestContextBuilder_WithOutgoingAPIMetrics_RecorderAffectsMetrics(t *testing.T) {
	target := ContextRequestTarget(uniqueLabel(t, "target"))
	address := ContextRequestAddress(uniqueLabel(t, "address"))
	method := ContextRequestMethod(uniqueLabel(t, "method"))
	targetStr, addrStr, methodStr := string(target), string(address), string(method)

	_, recorder := NewContextBuilder(context.Background()).
		WithOutgoingAPIMetrics(target, address, method).
		BuildContextAndTelemetry()

	// In-flight should be 1 immediately after the telemeter is staged.
	assert.Equal(t, float64(1), readGauge(outgoingAPIRequestsInFlight, targetStr, addrStr, methodStr))

	var err error
	recorder(&err)

	// In-flight should return to 0 after the recorder fires.
	assert.Equal(t, float64(0), readGauge(outgoingAPIRequestsInFlight, targetStr, addrStr, methodStr))
	// Exactly one duration observation should have been recorded.
	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestDurationSeconds, metricStatusSuccess, targetStr, addrStr, methodStr),
		"expected exactly one duration observation with status=success",
	)
}

func TestContextBuilder_WithOutgoingAPIMetrics_RecorderIsIdempotent(t *testing.T) {
	// Calling the Recorder more than once must be a no-op (sync.Once guarantee).
	target := ContextRequestTarget(uniqueLabel(t, "target"))
	address := ContextRequestAddress(uniqueLabel(t, "address"))
	method := ContextRequestMethod(uniqueLabel(t, "method"))
	targetStr, addrStr, methodStr := string(target), string(address), string(method)

	_, recorder := NewContextBuilder(context.Background()).
		WithOutgoingAPIMetrics(target, address, method).
		BuildContextAndTelemetry()

	var err error
	recorder(&err)
	recorder(&err) // second call must not mutate any metric

	assert.Equal(t, float64(0), readGauge(outgoingAPIRequestsInFlight, targetStr, addrStr, methodStr),
		"in-flight gauge must not go negative when the recorder is called more than once",
	)
	assert.Equal(t, uint64(1),
		readHistogramCount(outgoingAPIRequestDurationSeconds, metricStatusSuccess, targetStr, addrStr, methodStr),
		"duration must only be observed once even when the recorder is called multiple times",
	)
}
