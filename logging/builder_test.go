// Copyright 2025 NetApp, Inc. All Rights Reserved.

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

func TestContextBuilder_WithTelemetry_CollectsAndRecords(t *testing.T) {
	cb := NewContextBuilder(context.Background())
	countA := 0
	countB := 0
	teleA := func(ctx context.Context) Recorder { return func(err *error) { countA++ } }
	teleB := func(ctx context.Context) Recorder { return func(err *error) { countB++ } }

	ctx, recorder := cb.WithTelemetry(teleA).WithTelemetry(teleB).BuildContextAndTelemetry()
	assert.NotNil(t, ctx)
	assert.NotNil(t, recorder)

	var ok error
	recorder(&ok)
	recorder(&ok)
	assert.Equal(t, 2, countA)
	assert.Equal(t, 2, countB)
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
