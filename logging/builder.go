// Copyright 2025 NetApp, Inc. All Rights Reserved.

package logging

import (
	"context"
)

// ContextBuilder builds a context with various context values and telemeters.
// Each method returns a new ContextBuilder, allowing for chaining.
type ContextBuilder func() (context.Context, []Telemeter)

// NewContextBuilder creates a new ContextBuilder with the supplied context as the base.
// This context is maintained as the parent context for all subsequent additions to maintain
// proper context chaining and cancellation.
// If a nil context is supplied, context.TODO() is used.
func NewContextBuilder(ctx context.Context) ContextBuilder {
	if ctx == nil {
		ctx = context.TODO()
	}
	return func() (context.Context, []Telemeter) {
		return ctx, []Telemeter{}
	}
}

// WithContext merges the supplied context into the existing context. It does not change
// the existing context's cancellation or deadline, only its values.
// This will merge all values from the supplied context. Use at your discretion.
func (cb ContextBuilder) WithContext(childCtx context.Context) ContextBuilder {
	return func() (context.Context, []Telemeter) {
		parentCtx, telemeters := cb()
		return mergeContextWithPriority(parentCtx, childCtx), telemeters
	}
}

// WithParent sets a new parent context, adopting a new cancellation and deadline for the existing context.
// This is similar to WithContext, but the parameter name emphasizes that the supplied context is intended
// to be the parent of the existing context. It does not overrule the existing context's values.
// Prefer to supply the parent context at the creation of the ContextBuilder via NewContextBuilder when possible.
func (cb ContextBuilder) WithParent(parentCtx context.Context) ContextBuilder {
	return func() (context.Context, []Telemeter) {
		childCtx, telemeters := cb()
		return mergeContextWithPriority(parentCtx, childCtx), telemeters
	}
}

// WithWorkflow sets the Workflow in the context.
func (cb ContextBuilder) WithWorkflow(w Workflow) ContextBuilder {
	return func() (context.Context, []Telemeter) {
		ctx, telemeters := cb()
		return setContextWorkflow(ctx, w), telemeters
	}
}

// WithLayer sets the LogLayer in the context.
func (cb ContextBuilder) WithLayer(l LogLayer) ContextBuilder {
	return func() (context.Context, []Telemeter) {
		ctx, telemeters := cb()
		return setContextLogLayer(ctx, l), telemeters
	}
}

// WithSource sets the ContextSource in the context.
// This should be set where the context originates.
// If the context originates internally, use ContextSourceInternal.
func (cb ContextBuilder) WithSource(s string) ContextBuilder {
	return func() (context.Context, []Telemeter) {
		ctx, telemeters := cb()
		return setContextSource(ctx, ContextSource(s)), telemeters
	}
}

// WithClient sets the ContextRequestClient in the context.
// This should be set to identify the client making the request, typically in frontend Layers.
func (cb ContextBuilder) WithClient(c ContextRequestClient) ContextBuilder {
	return func() (context.Context, []Telemeter) {
		ctx, telemeters := cb()
		return setContextClient(ctx, c), telemeters
	}
}

// WithTarget sets the ContextRequestTarget in the context.
// This should be set to identify the target of the request, typically in frontend and API Layers.
// Frontends should almost always set this to the orchestrator name, while outgoing APIs should set
// this to the target system name or address.
func (cb ContextBuilder) WithTarget(t ContextRequestTarget) ContextBuilder {
	return func() (context.Context, []Telemeter) {
		ctx, telemeters := cb()
		return setContextTarget(ctx, t), telemeters
	}
}

func (cb ContextBuilder) WithAddress(a ContextRequestAddress) ContextBuilder {
	return func() (context.Context, []Telemeter) {
		ctx, telemeters := cb()
		return setContextAddress(ctx, a), telemeters
	}
}

// WithRoute sets the ContextRequestRoute in the context.
// This should be set to identify the logical route or endpoint of the request within the target system.
func (cb ContextBuilder) WithRoute(r ContextRequestRoute) ContextBuilder {
	return func() (context.Context, []Telemeter) {
		ctx, telemeters := cb()
		return setContextRoute(ctx, r), telemeters
	}
}

func (cb ContextBuilder) WithMethod(m ContextRequestMethod) ContextBuilder {
	return func() (context.Context, []Telemeter) {
		ctx, telemeters := cb()
		return setContextMethod(ctx, m), telemeters
	}
}

// WithDuration sets the ContextRequestDuration in the context.
// This is typically used in situations where the latency of a request is already known.
// This forces any telemeters capturing duration, to use this value instead of measuring it themselves.
func (cb ContextBuilder) WithDuration(t ContextRequestDuration) ContextBuilder {
	return func() (context.Context, []Telemeter) {
		ctx, telemeters := cb()
		return setContextDuration(ctx, t), telemeters
	}
}

// WithTelemetry adds the specified telemeters to those already in the context
// This can be called multiple times to add more telemeters, or once with a batch.
func (cb ContextBuilder) WithTelemetry(telemeters ...Telemeter) ContextBuilder {
	return func() (context.Context, []Telemeter) {
		ctx, existingTelemeters := cb()
		return ctx, append(existingTelemeters, telemeters...)
	}
}

// BuildContext builds and returns the context without telemetry.
func (cb ContextBuilder) BuildContext() context.Context {
	ctx, _ := cb()
	return ctx
}

// BuildTelemetry builds and returns a set a Recorder to capture all supplied Telemetry metrics.
// If this is called without first supplying telemeters via WithTelemetry, the Recorder will be a no-op.
func (cb ContextBuilder) BuildTelemetry() Recorder {
	ctx, telemeters := cb()
	_, recorder := makeContextBasedTelemetry(ctx, telemeters...)
	return recorder
}

// BuildContextAndTelemetry builds the context along with a Recorder to capture all supplied Telemetry metrics.
// If this is called without first supplying telemeters via WithTelemetry, the Recorder will be a no-op.
func (cb ContextBuilder) BuildContextAndTelemetry() (context.Context, Recorder) {
	ctx, telemeters := cb()
	return makeContextBasedTelemetry(ctx, telemeters...)
}
