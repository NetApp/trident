// Copyright 2025 NetApp, Inc. All Rights Reserved.

package logging

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSetAndGetContextWorkflow(t *testing.T) {
	// Arrange
	ctx := context.Background()

	// Act
	ctx = setContextWorkflow(ctx, WorkflowNone)
	wf := getContextWorkflow(ctx)
	assert.Equal(t, WorkflowNone, wf)

	ctx = setContextWorkflow(ctx, Workflow{"", ""})
	assert.Equal(t, WorkflowNone, getContextWorkflow(ctx))
}

func TestSetAndGetContextLayer(t *testing.T) {
	ctx := context.Background()

	ctx = setContextLogLayer(ctx, LogLayerNone)
	layer := getContextLayer(ctx)
	assert.Equal(t, LogLayerNone, layer)

	ctx2 := setContextLogLayer(ctx, LogLayer("data"))
	layer2 := getContextLayer(ctx2)
	assert.Equal(t, LogLayer("data"), layer2)
}

func TestSetAndGetRequestValues(t *testing.T) {
	ctx := context.Background()
	ctx = setContextSource(ctx, "")
	ctx = setContextClient(ctx, "")
	ctx = setContextTarget(ctx, "")
	ctx = setContextAddress(ctx, "")
	ctx = setContextRoute(ctx, "")
	ctx = setContextMethod(ctx, "")
	assert.Equal(t, ContextSource(ContextSourceUnknown), getContextSource(ctx))
	assert.Equal(t, ContextRequestClientUnknown, getContextClient(ctx))
	assert.Equal(t, ContextRequestTargetUnknown, getContextTarget(ctx))
	assert.Equal(t, ContextRequestAddressUnknown, getContextAddress(ctx))
	assert.Equal(t, ContextRequestRouteUnknown, getContextRoute(ctx))
	assert.Equal(t, ContextRequestMethodUnknown, getContextMethod(ctx))

	ctx = setContextSource(ctx, ContextSource(ContextSourceREST))
	ctx = setContextClient(ctx, ContextRequestClientTridentCLI)
	ctx = setContextTarget(ctx, ContextRequestTargetONTAP)
	ctx = setContextAddress(ctx, "10.0.0.1")
	ctx = setContextRoute(ctx, "/v1/test")
	ctx = setContextMethod(ctx, "GET")
	assert.Equal(t, ContextSource(ContextSourceREST), getContextSource(ctx))
	assert.Equal(t, ContextRequestClientTridentCLI, getContextClient(ctx))
	assert.Equal(t, ContextRequestTargetONTAP, getContextTarget(ctx))
	assert.Equal(t, ContextRequestAddress("10.0.0.1"), getContextAddress(ctx))
	assert.Equal(t, ContextRequestRoute("/v1/test"), getContextRoute(ctx))
	assert.Equal(t, ContextRequestMethod("GET"), getContextMethod(ctx))
}

func TestMergeContextWithPriority(t *testing.T) {
	parent := context.Background()
	priority := context.Background()
	parent = context.WithValue(parent, ContextKeyRequestID, "parentID")
	parent = context.WithValue(parent, ContextKeyRequestClient, ContextRequestClientCSINodeClient)
	priority = context.WithValue(priority, ContextKeyRequestID, "priorityID")
	priority = context.WithValue(priority, ContextKeyRequestTarget, ContextRequestTargetKubernetes)

	merged := mergeContextWithPriority(parent, priority)
	assert.Equal(t, "priorityID", merged.Value(ContextKeyRequestID))
	assert.Equal(t, ContextRequestClientCSINodeClient, merged.Value(ContextKeyRequestClient))
	assert.Equal(t, ContextRequestTargetKubernetes, merged.Value(ContextKeyRequestTarget))
}

func TestMakeContextBasedTelemetry(t *testing.T) {
	ctx := context.Background()
	telemetryCalled := 0
	telem := func(c context.Context) Recorder {
		assert.NotNil(t, c)
		return func(err *error) { telemetryCalled++ }
	}

	newCtx, recorder := makeContextBasedTelemetry(ctx, telem, telem)
	assert.NotNil(t, newCtx)
	assert.NotNil(t, recorder)

	// Act: invoke recorder with success
	var noErr error
	recorder(&noErr)
	assert.Equal(t, 2, telemetryCalled)

	someErr := errors.New("boom")
	recorder(&someErr)
	assert.Equal(t, 4, telemetryCalled)
}
