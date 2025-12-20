// Copyright 2025 NetApp, Inc. All Rights Reserved.

package logging

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
)

const (
	UseDefault = iota
	Trace

	redacted = "<REDACTED>"
)

var (
	// The workflows map has an innermost bool type to match the selectedWorkflows map. If it didn't, a helper function
	// below would have to be duplicated.
	workflows         = make(map[WorkflowCategory]map[WorkflowOperation]bool)
	logLayers         = make(map[LogLayer]bool)
	selectedWorkflows = make(map[WorkflowCategory]map[WorkflowOperation]bool)
	selectedLogLayers = make(map[LogLayer]bool)

	defaultLogLevel      = log.PanicLevel
	areLogLayersAdditive = false
)

func init() {
	// Doing this instead of putting all the constants directly into a map literal, which would require a large number
	// of empty curly braces.
	for _, w := range WorkflowTypes {
		if _, ok := workflows[w.Category]; !ok {
			workflows[w.Category] = make(map[WorkflowOperation]bool)
		}
		workflows[w.Category][w.Operation] = true
	}

	for _, l := range Layers {
		logLayers[l] = true
	}
}

// Log is a convenience function for calling Logc with a background context in places where the context is not important.
func Log() LogEntry {
	return Logc(context.Background())
}

// Logc performs logging for all Trident except for the storage drivers. It dynamically configures the log level
// based on the selected workflows and/or log Layers.
func Logc(ctx context.Context) LogEntry {
	entry := getRequestIDSourceLogEntry(ctx)

	// Second, check for the workflow and layer values in the main context.
	// If these are set, it's legacy code or code that doesn't set the RequestContextOptions.
	// Honor these above the RequestContextOptions values.
	if val := ctx.Value(ContextKeyWorkflow); val != nil && val != WorkflowNone {
		entry = entry.WithField(string(ContextKeyWorkflow), val)
	}
	if val := ctx.Value(ContextKeyLogLayer); val != nil && val != LogLayerNone {
		entry = entry.WithField(string(ContextKeyLogLayer), val)
	}

	isAuditLog := false
	if val := ctx.Value(auditKey); val != nil {
		isAuditLog = true
		entry = entry.WithField(string(auditKey), val)
	}

	// This should always be safe since entry implements logEntry.
	e, _ := entry.(*logEntry)

	// Enforce the logging of audit events.
	if isAuditLog {
		e.dynamicLevel = log.InfoLevel
	} else {
		e.dynamicLevel = getLogLevelForWorkflowAndLayer(getWorkflowTypeFromContext(ctx), getLogLayerFromContext(ctx))
	}

	return entry
}

// Logd performs logging for the storage drivers. It works for legacy debugTraceFlags as well as workflow and layer
// tracing.
func Logd(ctx context.Context, driverName string, debugTraceFlagEnabled bool) LogEntry {
	entry := getRequestIDSourceLogEntry(ctx)
	// This should always be safe since logEntry implements LogEntry.
	e, _ := entry.(*logEntry)

	if debugTraceFlagEnabled {
		e.dynamicLevel = log.TraceLevel
		return entry
	}

	ctx = SetContextLogLayer(ctx, LogLayer(driverName))
	// If debug trace flags aren't enabled, get the log level for the selected workflows and log Layers.
	e.dynamicLevel = getLogLevelForWorkflowAndLayer(getWorkflowTypeFromContext(ctx), getLogLayerFromContext(ctx))

	return entry
}

func getRequestIDSourceLogEntry(ctx context.Context) LogEntry {
	fields := log.Fields{}
	reqId := ctx.Value(ContextKeyRequestID)
	reqSrc := ctx.Value(ContextKeyRequestSource)

	if reqId != nil && reqSrc != nil {
		fields = log.Fields{
			"requestID":     reqId,
			"requestSource": reqSrc,
		}
	}

	entry := newLogEntry(fields)

	if val := ctx.Value(CRDControllerEvent); val != nil {
		entry = entry.WithField(string(CRDControllerEvent), val)
	}

	return entry
}

func GenerateRequestContext(
	ctx context.Context, requestID, requestSource string, workflow Workflow, logLayer LogLayer,
) context.Context {
	if ctx == nil {
		ctx = context.Background()
	} else {
		if v := ctx.Value(ContextKeyRequestID); v != nil {
			requestID = fmt.Sprint(v)
		}
		if v := ctx.Value(ContextKeyRequestSource); v != nil {
			requestSource = fmt.Sprint(v)
		}
	}
	if requestID == "" {
		requestID = uuid.New().String()
	}
	if requestSource == "" {
		requestSource = "Unknown"
	}
	ctx = context.WithValue(ctx, ContextKeyRequestID, requestID)
	ctx = context.WithValue(ctx, ContextKeyRequestSource, requestSource)
	if workflow != WorkflowNone {
		ctx = context.WithValue(ctx, ContextKeyWorkflow, workflow)
	}
	if logLayer != LogLayerNone {
		ctx = context.WithValue(ctx, ContextKeyLogLayer, logLayer)
	}
	return ctx
}

// SetWorkflows takes a list of workflow categories and workflow operations for which to enable trace logging for.
// An example of such a list is:
// controller:publish,unpublish;volume:create,delete
// What this example would do is to enable trace logging for the publish and unpublish operations within the controller
// category. Then, it would enable trace logging for the create and delete operations within the volume category.
// NOTE: The separators on which the string is split into its constituent parts (different sets of categories and their
// operations) are defined at the top of this file. If they are changed, this example will would need to be updated.
func SetWorkflows(wflows string) error {
	selectedWorkflows = make(map[WorkflowCategory]map[WorkflowOperation]bool)

	if wflows == "" {
		Log().Trace("No logging workflows provided.")
		return nil
	}

	if err := processWorkflowString(wflows); err != nil {
		return err
	}

	return nil
}

// processWorkflowString splits the string provided by the --log-workflows flag into <category>:<operations>, then
// splits each of those into category and operations. Finally, it splits up the list of operations for the category,
// and checks if trace logging is set for that operation.
func processWorkflowString(flowStr string) error {
	flows := strings.Split(flowStr, WorkflowFlagSeparator)
	for _, workflow := range flows {
		flowParts := strings.Split(workflow, workflowCategorySeparator)
		if len(flowParts) < 2 {
			return fmt.Errorf(`workflow string: %s did not contain a "%s" character`, workflow,
				workflowCategorySeparator)
		}
		category := WorkflowCategory(flowParts[0])
		opList := flowParts[1]
		ops := strings.Split(opList, workflowOperationSeparator)
		_, ok := selectedWorkflows[category]
		if !ok {
			selectedWorkflows[category] = make(map[WorkflowOperation]bool)
		}
		for _, op := range ops {
			traceAll := op == string(OpAll)

			// Stop processing category operations on the first "all" or "none" operation.
			if traceAll {
				selectedWorkflows[category] = map[WorkflowOperation]bool{OpAll: true}
				break
			}

			o, tracingEnabled, err := isTracingEnabledForOperation(category, op)
			if err != nil {
				return err
			}
			selectedWorkflows[category][o] = tracingEnabled
		}
	}

	return nil
}

func SetLogLayers(layers string) error {
	selectedLogLayers = make(map[LogLayer]bool)

	if layers == "" {
		Log().Trace("No logging Layers provided.")
		return nil
	}

	if err := processLogLayersString(layers); err != nil {
		return err
	}

	return nil
}

func processLogLayersString(theLayers string) error {
	// Check if the Layers are intended to be additive, and then split the string provided by the --log-Layers flag into
	// individual Layers to check whether trace logging is indicated for that layer.
	unionCharUsed := strings.Index(theLayers, additiveModifier) == 0
	if unionCharUsed {
		// Remove the union operator from the Layers list.
		theLayers = theLayers[1:]
		areLogLayersAdditive = true
	} else {
		areLogLayersAdditive = false
	}

	layers := strings.Split(theLayers, LogLayerSeparator)

	for _, layer := range layers {
		l, traceEnabled, err := isTracingEnabledForLogLayer(layer)
		if err != nil {
			return err
		}

		traceAll := l == LogLayerAll

		if traceAll {
			selectedLogLayers = map[LogLayer]bool{LogLayerAll: true}
			return nil
		}

		selectedLogLayers[l] = traceEnabled
	}

	return nil
}

func ListWorkflowTypes() []string {
	types := []string{}
	flows := strings.Split(getWorkflowsString(workflows), WorkflowFlagSeparator)
	for _, category := range flows {
		types = append(types, category)
	}
	return types
}

func GetSelectedWorkFlows() string {
	return getWorkflowsString(selectedWorkflows)
}

func getWorkflowsString(flows map[WorkflowCategory]map[WorkflowOperation]bool) string {
	cats := []string{}
	for k, ops := range flows {
		category := string(k) + workflowCategorySeparator
		catOps := []string{}
		for op := range ops {
			catOps = append(catOps, string(op))
		}
		sort.Strings(catOps)
		for i, op := range catOps {
			sep := workflowOperationSeparator
			if i == len(catOps)-1 {
				sep = ""
			}
			category += op + sep
		}
		cats = append(cats, category)
	}

	sort.Strings(cats)

	return strings.Join(cats, WorkflowFlagSeparator)
}

func ListLogLayers() []string {
	return strings.Split(getLogLayersString(logLayers), LogLayerSeparator)
}

func GetSelectedLogLayers() string {
	return getLogLayersString(selectedLogLayers)
}

func getLogLayersString(layers map[LogLayer]bool) string {
	additiveCharSet := false

	lyrs := []string{}
	for k := range layers {
		layer := string(k)

		if areLogLayersAdditive && !additiveCharSet {
			layer = additiveModifier + layer
			additiveCharSet = true
		}

		lyrs = append(lyrs, layer)
	}

	sort.Strings(lyrs)

	return strings.Join(lyrs, LogLayerSeparator)
}

// SetContextWorkflow sets the workflow type of the context. This WILL replace the workflow type if it already exists.
func SetContextWorkflow(ctx context.Context, w Workflow) context.Context {
	if !w.IsValid() {
		w = WorkflowNone
	}
	return context.WithValue(ctx, ContextKeyWorkflow, w)
}

// SetContextLogLayer sets the associated log layer of the context.
// This WILL replace the log layer if it already exists.
// This will replace the log layer in the RequestContextOptions stored in the context.
func SetContextLogLayer(ctx context.Context, l LogLayer) context.Context {
	// Don't set a log layer if the underlying string value is an empty string. This can happen if a storage driver is
	// initialized with an empty string as the driver name argument.
	if string(l) == "" {
		l = LogLayerNone
	}
	return context.WithValue(ctx, ContextKeyLogLayer, l)
}

func getLogLevelForWorkflowAndLayer(workflow Workflow, layer LogLayer) log.Level {
	workflowsSelected := len(selectedWorkflows) > 0
	layersSelected := len(selectedLogLayers) > 0

	// If we did not select any workflows or log Layers, use the default log level.
	if !workflowsSelected && !layersSelected {
		return defaultLogLevel
	}

	workflowLevel, layerLevel := UseDefault, UseDefault

	if workflowsSelected {
		workflowLevel = determineLogLevelForWorkflow(workflow)
	}

	if layersSelected {
		layerLevel = determineLogLevelForLayer(layer)
	}

	// If we selected workflows, but no log Layers.
	if workflowsSelected && !layersSelected {
		return getLogLevelForEntry(workflowLevel)
	}

	// If we selected log Layers, but no workflows.
	if layersSelected && !workflowsSelected {
		return getLogLevelForEntry(layerLevel)
	}

	return getLogLevelForEntry(handleWorkflowsAndLayersCase(workflowLevel, layerLevel))
}

func handleWorkflowsAndLayersCase(workflowLevel, layerLevel int) int {
	// Workflows and log Layers provided.
	level := UseDefault
	if areLogLayersAdditive {
		if workflowLevel == Trace || layerLevel == Trace {
			level = Trace
		}
	} else {
		if workflowLevel == Trace && layerLevel == Trace {
			level = Trace
		}
	}

	return level
}

// determineLogLevelForWorkflow returns an enum which is UseDefault or Trace.
func determineLogLevelForWorkflow(workflow Workflow) int {
	if _, categoryOk := selectedWorkflows[workflow.Category]; categoryOk {
		catMap := selectedWorkflows[workflow.Category]
		if _, ok := catMap[OpAll]; ok {
			return Trace
		}

		if _, ok := catMap[workflow.Operation]; ok {
			return Trace
		}
	}

	return UseDefault
}

// determineLogLevelForLayer returns an enum which is UseDefault or Trace.
func determineLogLevelForLayer(layer LogLayer) int {
	layers := selectedLogLayers

	if _, ok := layers[LogLayerAll]; ok {
		return Trace
	}

	if _, ok := layers[layer]; ok {
		return Trace
	}

	return UseDefault
}

func getLogLevelForEntry(logOpt int) log.Level {
	switch logOpt {
	case Trace:
		return log.TraceLevel
	default:
		return defaultLogLevel
	}
}

func getWorkflowTypeFromContext(ctx context.Context) Workflow {
	flow := ctx.Value(ContextKeyWorkflow)
	workflow, ok := flow.(Workflow)
	if !ok {
		return WorkflowNone
	}

	return workflow
}

func getLogLayerFromContext(ctx context.Context) LogLayer {
	layer := ctx.Value(ContextKeyLogLayer)
	l, ok := layer.(LogLayer)
	if !ok {
		return LogLayerNone
	}

	return l
}

// isTracingEnabledForOperation returns true if the operation for the provided workflow category is enabled for trace
// logging, false if not. If the provided workflow category is not a defined category, or the operation is not defined
// for a defined category, an error message will be returned.
func isTracingEnabledForOperation(category WorkflowCategory, operation string) (WorkflowOperation, bool, error) {
	op := WorkflowOperation(operation)

	if _, ok := workflows[category]; !ok {
		return "", false, fmt.Errorf("provided workflow category: %s is not a valid workflow category", category)
	}

	if _, ok := workflows[category][op]; !ok {
		return "", false, fmt.Errorf("provided workflow operation: %s is not a valid operation for category: %s", category, op)
	}

	return op, true, nil
}

// isTracingEnabledForLogLayer returns the true if tracing is enabled, otherwise false.
func isTracingEnabledForLogLayer(layer string) (LogLayer, bool, error) {
	if LogLayer(layer) == LogLayerNone {
		return LogLayerNone, false, nil
	}

	l := LogLayer(layer)

	if _, ok := logLayers[l]; !ok {
		return "", false, fmt.Errorf("provided log layer: %s is not a valid log layer", layer)
	}

	return LogLayer(layer), true, nil
}

func SetDefaultLogLevel(level string) error {
	lvl, err := getLogLevelByName(level)
	if err != nil {
		return err
	}
	defaultLogLevel = lvl

	return nil
}

func GetDefaultLogLevel() string {
	return defaultLogLevel.String()
}

func getLogLevelByName(l string) (log.Level, error) {
	lvl, err := log.ParseLevel(l)
	if err != nil {
		return log.PanicLevel, fmt.Errorf("Provided log level: %s not a valid log level!", l)
	}
	return lvl, nil
}

func IsLogLevelDebugOrHigher(level string) bool {
	lvl, err := getLogLevelByName(level)
	if err != nil {
		log.Warnf("Provided log level: %s is not a valid log level.", level)
	}
	return lvl >= log.DebugLevel
}

// FormatMessageForLog capitalizes the first letter of the string, and adds punctuation.
func FormatMessageForLog(msg string) string {
	msg = strings.ToLower(msg)
	runes := []rune(msg)

	sentenceCased := strings.ToUpper(string(runes[0])) + string(runes[1:])
	if !strings.HasSuffix(sentenceCased, ".") {
		sentenceCased += "."
	}

	return sentenceCased
}

func RedactedHTTPRequest(request *http.Request, requestBody []byte, driverName string, redactBody, isDriverLog bool) {
	header := ">>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>>"
	footer := "--------------------------------------------------------------------------------"

	ctx := request.Context()
	ctx = GenerateRequestContextForLayer(ctx, LogLayerUtils)

	requestURL, err := url.Parse(request.URL.String())
	if err != nil {
		if isDriverLog {
			Logd(ctx, driverName, true).WithError(err).Errorf("Unable to parse URL '%s'", request.URL.String())
		} else {
			Logc(ctx).WithError(err).Errorf("Unable to parse URL '%s'", request.URL.String())
		}
	}
	requestURL.User = nil

	headers := make(map[string][]string)
	for k, v := range request.Header {
		headers[k] = v
	}
	delete(headers, "Authorization")
	delete(headers, "Api-Key")
	delete(headers, "Secret-Key")

	var body string
	if requestBody == nil {
		body = "<nil>"
	} else if redactBody {
		body = redacted
	} else {
		body = string(requestBody)
	}

	if isDriverLog {
		Logd(ctx, driverName, true).Tracef("\n%s\n%s %s\nHeaders: %v\nBody: %s\n%s",
			header, request.Method, requestURL, headers, body, footer)
	} else {
		Logc(ctx).Tracef("\n%s\n%s %s\nHeaders: %v\nBody: %s\n%s",
			header, request.Method, requestURL, headers, body, footer)
	}
}

func RedactedHTTPResponse(
	ctx context.Context, response *http.Response, responseBody []byte, driverName string, redactBody, isDriverLog bool,
) {
	ctx = GenerateRequestContextForLayer(ctx, LogLayerUtils)

	header := "<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<"
	footer := "================================================================================"

	headers := make(map[string][]string)
	for k, v := range response.Header {
		headers[k] = v
	}
	delete(headers, "Authorization")
	delete(headers, "Api-Key")
	delete(headers, "Secret-Key")

	var body string
	if responseBody == nil {
		body = "<nil>"
	} else if redactBody {
		body = redacted
	} else {
		body = string(responseBody)
	}

	if isDriverLog {
		Logd(ctx, driverName, true).Tracef("\n%s\nStatus: %s\nHeaders: %v\nBody: %s\n%s", header,
			response.Status, headers, body, footer)
	} else {
		Logc(ctx).Tracef("\n%s\nStatus: %s\nHeaders: %v\nBody: %s\n%s", header,
			response.Status, headers, body, footer)
	}
}
