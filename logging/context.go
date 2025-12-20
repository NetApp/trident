// Copyright 2025 NetApp, Inc. All Rights Reserved.

package logging

import (
	"context"
	"time"
)

// Context Keys are top-level keys for context.Context values used in Trident logging and context-based
// telemetry. These keys should be unique and not primitive types to avoid collisions.
// When adding new ContextKey constants, ensure they are added to the contextKeys list.
const (
	ContextKeyWorkflow        ContextKey = "workflow"
	ContextKeyLogLayer        ContextKey = "logLayer"
	ContextKeyRequestID       ContextKey = "requestID"
	ContextKeyRequestSource   ContextKey = "requestSource"
	ContextKeyRequestClient   ContextKey = "requestClient"
	ContextKeyRequestTarget   ContextKey = "requestTarget"
	ContextKeyRequestAddress  ContextKey = "requestAddress"
	ContextKeyRequestRoute    ContextKey = "requestRoute"
	ContextKeyRequestMethod   ContextKey = "requestMethod"
	ContextKeyRequestDuration ContextKey = "requestDuration"
	CRDControllerEvent        ContextKey = "crdControllerEvent"
)

// contextKeys is a list of all ContextKey constants used in mergeContextWithPriority.
// When adding new ContextKey constants, ensure they are added to this list.
var contextKeys = []ContextKey{
	ContextKeyWorkflow,
	ContextKeyLogLayer,
	ContextKeyRequestID,
	ContextKeyRequestSource,
	ContextKeyRequestClient,
	ContextKeyRequestTarget,
	ContextKeyRequestAddress,
	ContextKeyRequestRoute,
	ContextKeyRequestMethod,
	ContextKeyRequestDuration,
	CRDControllerEvent,
}

// Context Request Source Values are stock values that should be used with incoming and outgoing requests
// to identify the source of the context and request in Trident. Custom source values can also be used
// based on the source system or component, but generally these should cover most use cases.
const (
	ContextSourceUnknown  string = "unknown"
	ContextSourceInternal string = "Internal"
	ContextSourceCRD      string = "CRD"
	ContextSourceREST     string = "REST"
	ContextSourceK8S      string = "Kubernetes"
	ContextSourceDocker   string = "Docker"
	ContextSourceCSI      string = "CSI"
	ContextSourcePeriodic string = "Periodic"
)

// Context Request Client Values are stock values that should be used with incoming and outgoing requests
// to identify the client system or component in Trident. Custom client values can also be used
// based on the client system or component.
const (
	ContextRequestClientUnknown                ContextRequestClient = "unknown"
	ContextRequestClientCSIProvisioner         ContextRequestClient = "csi-provisioner"
	ContextRequestClientCSIResizer             ContextRequestClient = "csi-resizer"
	ContextRequestClientCSISnapshotter         ContextRequestClient = "csi-snapshotter"
	ContextRequestClientCSIAttacher            ContextRequestClient = "csi-attacher"
	ContextRequestClientTridentCLI             ContextRequestClient = "trident-cli"
	ContextRequestClientCSINodeClient          ContextRequestClient = "csi-node-client"
	ContextRequestClientCSINodeDriverRegistrar ContextRequestClient = "csi-node-registrar"
	ContextRequestClientTridentNode            ContextRequestClient = "trident-node"
)

// Context Request Target Values are stock values that should be used with incoming and outgoing requests
// in Trident. Custom target values can also be used based on the target system or component.
const (
	ContextRequestTargetUnknown    ContextRequestTarget = "unknown"
	ContextRequestTargetTrident    ContextRequestTarget = "trident"
	ContextRequestTargetKubernetes ContextRequestTarget = "kubernetes"
	ContextRequestTargetONTAP      ContextRequestTarget = "ontap"
)

// Misc. context request values. These can be used with incoming and outgoing requests.
const (
	ContextRequestAddressUnknown ContextRequestAddress = "unknown"
	ContextRequestRouteUnknown   ContextRequestRoute   = "unknown"
	ContextRequestMethodUnknown  ContextRequestMethod  = "unknown"
)

type (
	// ContextKey is used for context.Context value. The value requires a key that is not primitive type.
	ContextKey string // ContextKeyRequestID is the ContextKey for RequestID
	// ContextSource models the source system or component of a request in the context.
	ContextSource = string
	// ContextRequestClient models the client of a request in the context.
	ContextRequestClient = string
	// ContextRequestTarget models the target of a request in the context.
	ContextRequestTarget = string
	// ContextRequestAddress models the target of a request in the context.
	ContextRequestAddress = string
	// ContextRequestRoute models the route of a request in the context.
	ContextRequestRoute = string
	// ContextRequestMethod models the method of a request in the context.
	ContextRequestMethod = string
	// ContextRequestDuration models the duration of a request in the context.
	// This should only be used when the duration is already known.
	ContextRequestDuration = time.Duration
)

func setContextWorkflow(ctx context.Context, workflow Workflow) context.Context {
	if existing, ok := ctx.Value(ContextKeyWorkflow).(Workflow); ok && existing == workflow {
		return ctx
	}
	if !workflow.IsValid() {
		workflow = WorkflowNone
	}
	return context.WithValue(ctx, ContextKeyWorkflow, workflow)
}

func setContextLogLayer(ctx context.Context, layer LogLayer) context.Context {
	if existing, ok := ctx.Value(ContextKeyLogLayer).(LogLayer); ok && existing == layer {
		return ctx
	}
	if layer == "" {
		// This may be reflected in telemetry so use LogLayerNone instead.
		layer = LogLayerNone
	}
	return context.WithValue(ctx, ContextKeyLogLayer, layer)
}

func setContextSource(ctx context.Context, source ContextSource) context.Context {
	if existing, ok := ctx.Value(ContextKeyRequestSource).(ContextSource); ok && existing == source {
		return ctx
	}
	if source == "" {
		source = ContextSourceUnknown
	}
	return context.WithValue(ctx, ContextKeyRequestSource, source)
}

func setContextClient(ctx context.Context, client ContextRequestClient) context.Context {
	if existing, ok := ctx.Value(ContextKeyRequestClient).(ContextRequestClient); ok && existing == client {
		return ctx
	}
	if client == "" {
		client = ContextRequestClientUnknown
	}
	return context.WithValue(ctx, ContextKeyRequestClient, client)
}

func setContextTarget(ctx context.Context, target ContextRequestTarget) context.Context {
	if existing, ok := ctx.Value(ContextKeyRequestTarget).(ContextRequestTarget); ok && existing == target {
		return ctx
	}
	if target == "" {
		target = ContextRequestTargetUnknown
	}
	return context.WithValue(ctx, ContextKeyRequestTarget, target)
}

func setContextAddress(ctx context.Context, address ContextRequestAddress) context.Context {
	if existing, ok := ctx.Value(ContextKeyRequestAddress).(ContextRequestAddress); ok && existing == address {
		return ctx
	}
	if address == "" {
		address = ContextRequestAddressUnknown
	}
	return context.WithValue(ctx, ContextKeyRequestAddress, address)
}

func setContextRoute(ctx context.Context, route ContextRequestRoute) context.Context {
	if existing, ok := ctx.Value(ContextKeyRequestRoute).(ContextRequestRoute); ok && existing == route {
		return ctx
	}
	if route == "" {
		route = ContextRequestRouteUnknown
	}
	return context.WithValue(ctx, ContextKeyRequestRoute, route)
}

func setContextMethod(ctx context.Context, method ContextRequestMethod) context.Context {
	if existing, ok := ctx.Value(ContextKeyRequestMethod).(ContextRequestMethod); ok && existing == method {
		return ctx
	}
	if method == "" {
		method = ContextRequestMethodUnknown
	}
	return context.WithValue(ctx, ContextKeyRequestMethod, method)
}

func setContextDuration(ctx context.Context, duration ContextRequestDuration) context.Context {
	if existing, ok := ctx.Value(ContextKeyRequestDuration).(ContextRequestDuration); ok && existing == duration {
		return ctx
	}
	if duration < 0 {
		return ctx
	}
	return context.WithValue(ctx, ContextKeyRequestDuration, duration)
}

func getContextWorkflow(ctx context.Context) Workflow {
	workflow, ok := ctx.Value(ContextKeyWorkflow).(Workflow)
	if !ok {
		return WorkflowNone
	}
	if !workflow.IsValid() {
		workflow = WorkflowNone
	}
	return workflow
}

func getContextLayer(ctx context.Context) LogLayer {
	layer, ok := ctx.Value(ContextKeyLogLayer).(LogLayer)
	if !ok {
		return LogLayerNone
	}
	return layer
}

func getContextSource(ctx context.Context) ContextSource {
	source, ok := ctx.Value(ContextKeyRequestSource).(ContextSource)
	if !ok || source == "" {
		return ContextSourceUnknown
	}
	return source
}

func getContextClient(ctx context.Context) ContextRequestClient {
	client, ok := ctx.Value(ContextKeyRequestClient).(ContextRequestClient)
	if !ok || client == "" {
		return ContextRequestClientUnknown
	}
	return client
}

func getContextTarget(ctx context.Context) ContextRequestTarget {
	target, ok := ctx.Value(ContextKeyRequestTarget).(ContextRequestTarget)
	if !ok || target == "" {
		return ContextRequestTargetUnknown
	}
	return target
}

func getContextAddress(ctx context.Context) ContextRequestAddress {
	address, ok := ctx.Value(ContextKeyRequestAddress).(ContextRequestAddress)
	if !ok || address == "" {
		return ContextRequestAddressUnknown
	}
	return address
}

func getContextRoute(ctx context.Context) ContextRequestRoute {
	route, ok := ctx.Value(ContextKeyRequestRoute).(ContextRequestRoute)
	if !ok || route == "" {
		return ContextRequestRouteUnknown
	}
	return route
}

func getContextMethod(ctx context.Context) ContextRequestMethod {
	method, ok := ctx.Value(ContextKeyRequestMethod).(ContextRequestMethod)
	if !ok || method == "" {
		return ContextRequestMethodUnknown
	}
	return method
}

// getContextDuration retrieves the ContextRequestDuration from the context.
// If the duration is not set or is negative, it returns -1.
func getContextDuration(ctx context.Context) ContextRequestDuration {
	duration, ok := ctx.Value(ContextKeyRequestDuration).(ContextRequestDuration)
	if !ok || duration < 0 {
		return ContextRequestDuration(-1)
	}
	return duration
}

// mergeContextWithPriority merges two contexts where `parent` preserves cancellation/deadline
// and `priority`'s values always win on collisions.
func mergeContextWithPriority(parent, priority context.Context) context.Context {
	switch {
	case parent == nil:
		return priority
	case priority == nil:
		return parent
	}
	merged := parent

	// setContextKeyValue sets the key/value pair in merged only if the value is different.
	// If merged is nil for the key, it will always set.
	setContextKeyValue := func(k ContextKey, v interface{}) {
		if v == nil {
			return
		}
		mv := merged.Value(k)
		if mv != v || mv == nil {
			merged = context.WithValue(merged, k, v)
		}
	}

	// First take all values from priority.
	for _, k := range contextKeys {
		setContextKeyValue(k, priority.Value(k))
	}
	return merged
}

// makeContextBasedTelemetry based on the provided context and telemeters.
// It returns the new context and a Recorder that, when called, will record metrics
// for all initialized telemeters, as well as the workflow and layer telemeters.
func makeContextBasedTelemetry(ctx context.Context, telemeters ...Telemeter) (context.Context, Recorder) {
	// Generate any additional telemeters requested.
	recorders := make([]Recorder, 0, len(telemeters))
	for i := len(telemeters) - 1; i >= 0; i-- {
		// Capture context for each telemeter.
		telemeter := telemeters[i]
		recorder := telemeter(ctx)
		recorders = append(recorders, recorder)
	}

	// Generate both workflow and layer telemeters.
	return ctx, func(err *error) {
		for _, recorder := range recorders {
			recorder(err)
		}
	}
}

// GenerateRequestContextForLayer generates a new context for the specified logging layer.
func GenerateRequestContextForLayer(ctx context.Context, logLayer LogLayer) context.Context {
	// Don't add another context if we are already at the specified layer.
	if logLayer == ctx.Value(ContextKeyLogLayer) {
		return ctx
	}

	if logLayer != LogLayerNone {
		return context.WithValue(ctx, ContextKeyLogLayer, logLayer)
	}

	return ctx
}
