// Copyright 2022 NetApp, Inc. All Rights Reserved.

package logging

import (
	"context"
	"io"
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func TestLogc(t *testing.T) {
	ctx := context.Background()
	result := Logc(ctx)
	assert.NotNil(t, result, "log entry is nil")
}

func TestLogc_ContextWithKeyAndValue(t *testing.T) {
	ctx := context.WithValue(context.Background(), CRDControllerEvent, "add")
	result := Logc(ctx)
	assert.NotNil(t, result, "log entry is nil")
}

func TestLogc_ContextWithWorkflowAndLogLayer(t *testing.T) {
	type test struct {
		reqId         string
		reqSrc        string
		flow          Workflow
		layer         LogLayer
		expectedFlow  Workflow
		expectedLayer LogLayer
	}

	testCases := []test{
		{
			reqId: "foo", reqSrc: "bar", flow: WorkflowPluginCreate, layer: LogLayerCSIFrontend,
			expectedFlow: WorkflowPluginCreate, expectedLayer: LogLayerCSIFrontend,
		},
		{
			reqId: "foo", reqSrc: "bar", flow: WorkflowNone, layer: LogLayerCSIFrontend,
			expectedFlow: WorkflowNone, expectedLayer: LogLayerCSIFrontend,
		},
		{
			reqId: "foo", reqSrc: "bar", flow: WorkflowPluginCreate, layer: LogLayerNone,
			expectedFlow: WorkflowPluginCreate, expectedLayer: LogLayerNone,
		},
		{
			reqId: "foo", reqSrc: "bar", flow: WorkflowNone, layer: LogLayerNone,
			expectedFlow: WorkflowNone, expectedLayer: LogLayerNone,
		},
	}

	for _, tc := range testCases {
		ctx := context.Background()
		if tc.expectedFlow != WorkflowNone {
			ctx = context.WithValue(ctx, ContextKeyWorkflow, tc.flow)
		}

		if tc.expectedLayer != LogLayerNone {
			ctx = context.WithValue(ctx, ContextKeyLogLayer, tc.layer)
		}

		entry := Logc(ctx)

		wf, _ := entry.Data(string(ContextKeyWorkflow))
		ll, _ := entry.Data(string(ContextKeyLogLayer))

		if tc.expectedFlow != WorkflowNone {
			assert.Equal(t, tc.expectedFlow, wf, "expected the workflow that was passed in")
		} else {
			assert.Nil(t, wf, "expected the workflow to not exist in log entry")
		}

		if tc.expectedLayer != LogLayerNone {
			assert.Equal(t, tc.expectedLayer, ll, "expected the log layer that was passed in")
		} else {
			assert.Nil(t, ll, "expected the log layer to not exist in log entry")
		}
	}
}

func TestLogd_DebugTraceFlags(t *testing.T) {
	ctx := context.WithValue(context.Background(), CRDControllerEvent, "add")
	result := Logd(ctx, "ontap-san", true)
	assert.NotNil(t, result, "log entry is nil")
}

func TestLogd_NoDebugTraceFlags(t *testing.T) {
	ctx := context.WithValue(context.Background(), CRDControllerEvent, "add")
	result := Logd(ctx, "ontap-san", false)
	assert.NotNil(t, result, "log entry is nil")
}

func TestGenerateRequestContext(t *testing.T) {
	type test struct {
		ctx           context.Context
		requestID     string
		requestSource string
	}

	tests := []test{
		{ctx: context.Background(), requestID: "crdControllerEvent", requestSource: "kubernetes"},
		{ctx: nil, requestID: "requestID", requestSource: "CRD"},
		{
			ctx: context.WithValue(context.Background(), ContextKeyRequestID, "1234"), requestID: "requestID",
			requestSource: "CRD",
		},
		{
			ctx: context.WithValue(context.Background(), ContextKeyRequestSource, "1234"), requestID: "requestSource",
			requestSource: "CRD",
		},
		{ctx: context.Background(), requestID: "", requestSource: "CRD"},
		{ctx: context.Background(), requestID: "test-id", requestSource: ""},
	}

	for _, tc := range tests {
		result := GenerateRequestContext(tc.ctx, tc.requestID, tc.requestSource, WorkflowNone, LogLayerNone)
		assert.NotNil(t, result, "context is nil")
	}
}

func TestGenerateRequestContextForLayer(t *testing.T) {
	type test struct {
		name        string
		ctx         context.Context
		logLayer    LogLayer
		expectedCtx context.Context
	}

	tests := []test{
		{
			name:        "non-recursive context",
			ctx:         context.Background(),
			logLayer:    LogLayerCore,
			expectedCtx: context.WithValue(context.Background(), ContextKeyLogLayer, LogLayerCore),
		},
		{
			name:        "recursive context",
			ctx:         context.WithValue(context.Background(), ContextKeyLogLayer, LogLayerCore),
			logLayer:    LogLayerCore,
			expectedCtx: context.WithValue(context.Background(), ContextKeyLogLayer, LogLayerCore),
		},
		{
			name:        "log layer none",
			ctx:         context.Background(),
			logLayer:    LogLayerNone,
			expectedCtx: context.Background(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateRequestContextForLayer(tc.ctx, tc.logLayer)

			switch tc.logLayer {
			case LogLayerNone:
				assert.Nil(t, result.Value(ContextKeyLogLayer), "log layer is not none")
			default:
				assert.Equal(t, tc.logLayer, result.Value(ContextKeyLogLayer), "context values don't match")
			}

			assert.Equal(t, tc.expectedCtx, result, "contexts don't match")
		})
	}
}

func TestGenerateRequestContext_VerifyWorkflowAndLogLayers(t *testing.T) {
	type test struct {
		ctx           context.Context
		requestID     string
		requestSource string
		workflow      Workflow
		logLayer      LogLayer
		expectedFlow  Workflow
		expectedLayer LogLayer
	}

	tests := []test{
		{
			ctx: context.Background(), requestID: "crdControllerEvent", requestSource: "kubernetes",
			workflow: WorkflowPluginActivate, logLayer: LogLayerCSIFrontend, expectedFlow: WorkflowPluginActivate,
			expectedLayer: LogLayerCSIFrontend,
		},
		{
			ctx: context.Background(), requestID: "crdControllerEvent", requestSource: "kubernetes",
			workflow: WorkflowPluginActivate, logLayer: LogLayerNone, expectedFlow: WorkflowPluginActivate,
			expectedLayer: LogLayerNone,
		},
		{
			ctx: context.Background(), requestID: "crdControllerEvent", requestSource: "kubernetes",
			workflow: WorkflowNone, logLayer: LogLayerCSIFrontend, expectedFlow: WorkflowNone,
			expectedLayer: LogLayerCSIFrontend,
		},
		{
			ctx: context.Background(), requestID: "crdControllerEvent", requestSource: "kubernetes",
			workflow: WorkflowNone, logLayer: LogLayerNone, expectedFlow: WorkflowNone,
			expectedLayer: LogLayerNone,
		},
	}

	for _, tc := range tests {
		result := GenerateRequestContext(tc.ctx, tc.requestID, tc.requestSource, tc.workflow, tc.logLayer)
		wf := result.Value(ContextKeyWorkflow)
		ll := result.Value(ContextKeyLogLayer)

		if tc.expectedFlow != WorkflowNone {
			assert.Equal(t, tc.expectedFlow, wf, "expected the workflow that was passed in")
		} else {
			assert.Nil(t, wf, "expected the workflow to not exist")
		}

		if tc.expectedLayer != LogLayerNone {
			assert.Equal(t, tc.expectedLayer, ll, "expected the log layer that was passed in")
		} else {
			assert.Nil(t, ll, "expected the log layer to not exist")
		}
	}
}

func TestFormatMessageForLog(t *testing.T) {
	msg := "this is a test"
	punctuated := "this is a test."
	titled := "This is a test"
	caps := "THIS IS A TEST"
	capsPunc := "THIS IS A TEST."
	noChangesNeeded := "This is a test."

	expectedMsg := "This is a test."
	assertErr := "Expected the string to be titled and punctuated."

	type Args struct {
		TestString string
		Expected   string
		TestName   string
	}

	testCases := []Args{
		{msg, expectedMsg, "All lowercase, no punctuation"},
		{punctuated, expectedMsg, "All lowercase, but already has punctuation"},
		{titled, expectedMsg, "Already a title, but no punctuation"},
		{caps, expectedMsg, "All caps, no punctuation"},
		{capsPunc, expectedMsg, "All caps, with punctuation"},
		{noChangesNeeded, expectedMsg, "Already titled and punctuated"},
	}

	for _, args := range testCases {
		t.Run(args.TestName, func(t *testing.T) {
			assert.Equal(t, args.Expected, FormatMessageForLog(args.TestString), assertErr)
		})
	}
}

func TestProcessWorkflowString(t *testing.T) {
	type test struct {
		name        string
		flows       string
		testFlow    Workflow
		traceOn     bool
		expectedErr bool
	}

	testCases := []test{
		{name: "category, no operation", flows: "volume=", testFlow: WorkflowNone, traceOn: false, expectedErr: true},
		{name: "one valid workflow", flows: "volume=create", testFlow: WorkflowVolumeCreate, traceOn: true, expectedErr: false},
		{name: "empty string", flows: "", testFlow: WorkflowNone, expectedErr: true},
		{name: "workflow separator but only one workflow", testFlow: WorkflowNone, flows: "volume=create:", expectedErr: true},
		{name: "multiple incomplete", flows: "volume=create:core", testFlow: WorkflowNone, expectedErr: true},
		{name: "multiple valid", flows: "volume=create:core=init", testFlow: WorkflowVolumeCreate, traceOn: true, expectedErr: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			selectedWorkflows = make(map[WorkflowCategory]map[WorkflowOperation]bool)
			err := processWorkflowString(tc.flows)
			assert.Equal(t, tc.expectedErr, err != nil)
			assert.True(t, tc.traceOn == selectedWorkflows[tc.testFlow.Category][tc.testFlow.Operation])
		})
	}
}

func TestProcessLogLayersString(t *testing.T) {
	type test struct {
		name           string
		layers         string
		testLayer      LogLayer
		tracingEnabled bool
		errExpected    bool
	}

	testCases := []test{
		{name: "valid layer", layers: "csi_frontend", testLayer: LogLayerCSIFrontend, tracingEnabled: true, errExpected: false},
		{name: "all layers", layers: "all", testLayer: LogLayerAll, tracingEnabled: true, errExpected: false},
		{name: "no layers", layers: "none", testLayer: LogLayerNone, tracingEnabled: false, errExpected: false},
		{name: "invalid layer", layers: "volume=create:", testLayer: LogLayerNone, tracingEnabled: false, errExpected: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			selectedLogLayers = make(map[LogLayer]bool)
			err := processLogLayersString(tc.layers)
			assert.Equal(t, tc.errExpected, err != nil)
			if !tc.errExpected {
				assert.True(t, tc.tracingEnabled == selectedLogLayers[tc.testLayer])
			}
		})
	}
}

func TestGetWorkflowsString(t *testing.T) {
	type test struct {
		name     string
		flows    map[WorkflowCategory]map[WorkflowOperation]bool
		expected string
	}

	testCases := []test{
		{
			name: "empty string", flows: make(map[WorkflowCategory]map[WorkflowOperation]bool), expected: "",
		},
		{
			name: "volume create", flows: map[WorkflowCategory]map[WorkflowOperation]bool{
				CategoryVolume: {
					OpCreate: true,
				},
			}, expected: "volume=create",
		},
		{
			name: "volume create,delete and core bootstrap, negated init", flows: map[WorkflowCategory]map[WorkflowOperation]bool{
				CategoryCore: {
					OpInit:      true,
					OpBootstrap: true,
				},
				CategoryVolume: {
					OpCreate: true,
					OpDelete: true,
				},
			}, expected: "core=bootstrap,init:volume=create,delete",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getWorkflowsString(tc.flows)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestGetLogLayersString(t *testing.T) {
	type test struct {
		name           string
		layers         map[LogLayer]bool
		expected       string
		layersAdditive bool
	}

	testCases := []test{
		{
			name: "empty string", layers: make(map[LogLayer]bool),
			expected: "", layersAdditive: false,
		},
		{
			name: "layer none", layers: map[LogLayer]bool{
				LogLayerNone: true,
			}, expected: "none", layersAdditive: false,
		},
		{
			name: "layer all", layers: map[LogLayer]bool{
				LogLayerAll: true,
			},
			expected: "all", layersAdditive: false,
		},
		{
			name: "csi frontend", layers: map[LogLayer]bool{
				LogLayerCSIFrontend: true,
			},
			expected: "csi_frontend", layersAdditive: false,
		},
		{
			name: "multiple layers", layers: map[LogLayer]bool{
				LogLayerRESTFrontend:    true,
				LogLayerPersistentStore: true,
			},
			expected: "persistent_store,rest_frontend", layersAdditive: false,
		},
		{
			name: "additive csi frontend", layers: map[LogLayer]bool{
				LogLayerCSIFrontend: true,
			},
			expected: "+csi_frontend", layersAdditive: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			defer func(layersAdditive bool) { areLogLayersAdditive = layersAdditive }(areLogLayersAdditive)
			areLogLayersAdditive = tc.layersAdditive
			result := getLogLayersString(tc.layers)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestDetermineLogLevelForWorkflow(t *testing.T) {
	type test struct {
		name     string
		flow     Workflow
		flowMap  map[WorkflowCategory]map[WorkflowOperation]bool
		expected int
	}

	testCases := []test{
		{
			name: "trace all", flowMap: map[WorkflowCategory]map[WorkflowOperation]bool{
				CategoryCore: {
					OpAll: true,
				},
			}, expected: Trace, flow: WorkflowCoreInit,
		},
		{
			name: "core init trace", flowMap: map[WorkflowCategory]map[WorkflowOperation]bool{
				CategoryCore: {
					OpInit: true,
				},
			},
			expected: Trace, flow: WorkflowCoreInit,
		},
		{
			name: "workflow category in map, but not op", flowMap: map[WorkflowCategory]map[WorkflowOperation]bool{
				CategoryCore: {
					OpInit: true,
				},
			}, expected: UseDefault, flow: WorkflowCoreBootstrap,
		},
		{
			name: "no category", flowMap: make(map[WorkflowCategory]map[WorkflowOperation]bool), expected: UseDefault,
			flow: WorkflowPluginCreate,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			selectedWorkflows = tc.flowMap
			defer func() { selectedWorkflows = make(map[WorkflowCategory]map[WorkflowOperation]bool) }()
			result := determineLogLevelForWorkflow(tc.flow)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestDetermineLogLevelForLayer(t *testing.T) {
	type test struct {
		name     string
		layer    LogLayer
		layerMap map[LogLayer]bool
		expected int
	}

	testCases := []test{
		{
			name: "suppress csi frontend", layerMap: map[LogLayer]bool{
				LogLayerCSIFrontend: true,
			}, expected: Trace, layer: LogLayerCSIFrontend,
		},
		{
			name: "trace csi frontend", layerMap: map[LogLayer]bool{
				LogLayerCSIFrontend: true,
			}, expected: Trace, layer: LogLayerCSIFrontend,
		},
		{
			name: "layer not in map", layerMap: make(map[LogLayer]bool), expected: UseDefault,
			layer: LogLayerCSIFrontend,
		},
		{
			name: "no layer", layerMap: make(map[LogLayer]bool), expected: UseDefault,
			layer: LogLayerCSIFrontend,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			selectedLogLayers = tc.layerMap
			defer func() { selectedLogLayers = make(map[LogLayer]bool) }()
			result := determineLogLevelForLayer(tc.layer)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestHandleWorkflowsAndLayersCase(t *testing.T) {
	type test struct {
		name           string
		layersAdditive bool
		flowLevel      int
		layerLevel     int
		expected       int
	}

	testCases := []test{
		{
			name: "layers additive, workflow or layer trace = trace", layersAdditive: true, flowLevel: Trace,
			layerLevel: UseDefault, expected: Trace,
		},
		{
			name: "layers not additive, workflow and layer trace = trace", layersAdditive: false, flowLevel: Trace,
			layerLevel: Trace, expected: Trace,
		},
		{
			name: "layers not additive, but workflow trace and layer use_default = default level", layersAdditive: false,
			flowLevel: Trace, layerLevel: UseDefault, expected: UseDefault,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			origVal := areLogLayersAdditive
			areLogLayersAdditive = tc.layersAdditive
			defer func() { areLogLayersAdditive = origVal }()
			level := handleWorkflowsAndLayersCase(tc.flowLevel, tc.layerLevel)
			assert.Equal(t, tc.expected, level)
		})
	}
}

func TestGetWorkflowTypeFromContext(t *testing.T) {
	type test struct {
		name     string
		ctx      context.Context
		expected Workflow
	}

	testCases := []test{
		{
			name:     "Valid workflow in context",
			ctx:      context.WithValue(context.Background(), ContextKeyWorkflow, WorkflowPluginCreate),
			expected: WorkflowPluginCreate,
		},
		{
			name:     "No workflow in context",
			ctx:      context.Background(),
			expected: WorkflowNone,
		},
		{
			name:     "Value in context for workflow key, but not a workflow",
			ctx:      context.WithValue(context.Background(), ContextKeyWorkflow, "foo"),
			expected: WorkflowNone,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			flow := getWorkflowTypeFromContext(tc.ctx)
			assert.Equal(t, tc.expected, flow)
		})
	}
}

func TestGetLogLayerFromContext(t *testing.T) {
	type test struct {
		name     string
		ctx      context.Context
		expected LogLayer
	}

	testCases := []test{
		{
			name:     "Valid layer in context",
			ctx:      context.WithValue(context.Background(), ContextKeyLogLayer, LogLayerCSIFrontend),
			expected: LogLayerCSIFrontend,
		},
		{
			name:     "No layer in context",
			ctx:      context.Background(),
			expected: LogLayerNone,
		},
		{
			name:     "Value in context for layer key, but not a layer",
			ctx:      context.WithValue(context.Background(), ContextKeyLogLayer, "foo"),
			expected: LogLayerNone,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			flow := getLogLayerFromContext(tc.ctx)
			assert.Equal(t, tc.expected, flow)
		})
	}
}

func TestIsTracingEnabledForOperation(t *testing.T) {
	type test struct {
		name         string
		category     WorkflowCategory
		op           string
		expectedBool bool
		expectedErr  bool
	}

	testCases := []test{
		{
			name:         "Volume create expected trace no error",
			category:     CategoryVolume,
			op:           "create",
			expectedBool: true,
			expectedErr:  false,
		},
		{
			name:         "Invalid category",
			category:     CategoryNone,
			op:           "create",
			expectedBool: false,
			expectedErr:  true,
		},
		{
			name:         "Valid category, invalid operation",
			category:     CategoryVolume,
			op:           "foo",
			expectedBool: false,
			expectedErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, tracing, err := isTracingEnabledForOperation(tc.category, tc.op)
			assert.Equal(t, tc.expectedBool, tracing)
			assert.Equal(t, tc.expectedErr, err != nil)
		})
	}
}

func TestIsTracingEnabledForLogLayer(t *testing.T) {
	type test struct {
		name         string
		layer        string
		expectedBool bool
		expectedErr  bool
	}

	testCases := []test{
		{
			name:         "Valid layer, expected trace",
			layer:        "csi_frontend",
			expectedBool: true,
			expectedErr:  false,
		},
		{
			name:         "Invalid layer",
			layer:        "foo",
			expectedBool: false,
			expectedErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, tracing, err := isTracingEnabledForLogLayer(tc.layer)
			assert.Equal(t, tc.expectedBool, tracing)
			assert.Equal(t, tc.expectedErr, err != nil)
		})
	}
}

func TestIsLogLevelDebugOrHigher(t *testing.T) {
	assert.True(t, IsLogLevelDebugOrHigher("trace"))
	assert.False(t, IsLogLevelDebugOrHigher("warn"))
}

func TestSetDefaultLogLevel(t *testing.T) {
	defer func(level log.Level) { defaultLogLevel = level }(defaultLogLevel)

	err := SetDefaultLogLevel("foo")
	assert.Error(t, err)

	err = SetDefaultLogLevel("trace")
	assert.NoError(t, err)
}

func TestGetDefaultLogLevel(t *testing.T) {
	defer func(level log.Level) { defaultLogLevel = level }(defaultLogLevel)
	defaultLogLevel = log.TraceLevel
	assert.Equal(t, "trace", GetDefaultLogLevel())
}

func TestGetLogLevelForEntry(t *testing.T) {
	defer func(level log.Level) { defaultLogLevel = level }(defaultLogLevel)

	defaultLogLevel = log.ErrorLevel

	assert.Equal(t, log.TraceLevel, getLogLevelForEntry(Trace))

	assert.Equal(t, log.ErrorLevel, getLogLevelForEntry(UseDefault))
}

func TestGetLogLevelByName(t *testing.T) {
	_, err := getLogLevelByName("trace")
	assert.NoError(t, err)

	_, err = getLogLevelByName("warn")
	assert.NoError(t, err)

	_, err = getLogLevelByName("debug")
	assert.NoError(t, err)

	_, err = getLogLevelByName("warning")
	assert.NoError(t, err)

	_, err = getLogLevelByName("foo")
	assert.Error(t, err)
}

func TestSetContextWorkflow(t *testing.T) {
	ctx := context.Background()
	ctx = SetContextWorkflow(ctx, WorkflowBackendCreate)
	assert.NotNil(t, WorkflowBackendCreate, ctx.Value(ContextKeyWorkflow))
	assert.Equal(t, WorkflowBackendCreate, ctx.Value(ContextKeyWorkflow))
}

func TestSetContextLogLayer(t *testing.T) {
	ctx := context.Background()
	ctx = SetContextLogLayer(ctx, LogLayerCSIFrontend)
	assert.NotNil(t, WorkflowBackendCreate, ctx.Value(ContextKeyLogLayer))
	assert.Equal(t, LogLayerCSIFrontend, ctx.Value(ContextKeyLogLayer))
}

func TestListLogLayers(t *testing.T) {
	assert.Equal(t, []string{
		"all", "azure-netapp-files", "azure-netapp-files-subvolume", "core",
		"crd_frontend", "csi_frontend", "docker_frontend", "fake", "gcp-cvs", "ontap-nas",
		"ontap-nas-economy", "ontap-nas-flexgroup", "ontap-san", "ontap-san-economy",
		"persistent_store", "rest_frontend", "solidfire-san",
	}, ListLogLayers())
}

func TestGetSelectedLogLayers(t *testing.T) {
	selectedLogLayers[LogLayerCore] = true
	assert.Equal(t, "core", GetSelectedLogLayers())

	selectedLogLayers[LogLayerCSIFrontend] = true
	assert.Equal(t, "core,csi_frontend", GetSelectedLogLayers())
}
