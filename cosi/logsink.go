// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cosi

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	. "github.com/netapp/trident/logging"
)

// logSink bridges controller-runtime's logr to Trident's logrus, so COSI logs use the same
// format, dynamic level, and per-layer filtering as the rest of Trident.
type logSink struct {
	ctx    context.Context
	name   string
	fields LogFields
}

// logr V-levels. The vendored reconcilers use V(0) for state changes worth
// seeing by default and V(1) for per-reconcile detail.
const (
	debugVLevel = 1
	traceVLevel = 2
)

var _ logr.LogSink = &logSink{}

// newLogger returns a logr.Logger that writes through Trident's logger under the COSI log
// layer. The context is built once and reused, since logr.LogSink methods take no context.
func newLogger() logr.Logger {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowNone, LogLayerCOSIFrontend)
	return logr.New(&logSink{ctx: ctx, fields: LogFields{}})
}

func (s *logSink) Init(logr.RuntimeInfo) {}

// Enabled always reports true; level-based suppression happens downstream in Trident's logger,
// since this sink cannot read whether the cosi_frontend layer is enabled.
func (s *logSink) Enabled(int) bool { return true }

func (s *logSink) Info(level int, msg string, keysAndValues ...any) {
	entry := s.entry(keysAndValues)

	switch {
	case level >= traceVLevel:
		entry.Trace(msg)
	case level == debugVLevel:
		entry.Debug(msg)
	default:
		entry.Info(msg)
	}
}

func (s *logSink) Error(err error, msg string, keysAndValues ...any) {
	s.entry(keysAndValues).WithError(err).Error(msg)
}

func (s *logSink) WithValues(keysAndValues ...any) logr.LogSink {
	return &logSink{
		ctx:    s.ctx,
		name:   s.name,
		fields: mergeFields(s.fields, fieldsFrom(keysAndValues)),
	}
}

// WithName accumulates a dotted path, so each reconciler's Named(...) is distinguishable in a
// single log stream.
func (s *logSink) WithName(name string) logr.LogSink {
	qualified := name
	if s.name != "" {
		qualified = s.name + "." + name
	}
	return &logSink{ctx: s.ctx, name: qualified, fields: s.fields}
}

func (s *logSink) entry(keysAndValues []any) LogEntry {
	entry := Logc(s.ctx).WithFields(s.fields)

	if s.name != "" {
		entry = entry.WithField("logger", s.name)
	}
	if len(keysAndValues) > 0 {
		entry = entry.WithFields(fieldsFrom(keysAndValues))
	}

	return entry
}

// fieldsFrom converts logr's alternating key/value slice into LogFields.
func fieldsFrom(keysAndValues []any) LogFields {
	fields := make(LogFields, len(keysAndValues)/2)

	for i := 0; i+1 < len(keysAndValues); i += 2 {
		key, ok := keysAndValues[i].(string)
		if !ok {
			key = fmt.Sprintf("%v", keysAndValues[i])
		}
		fields[key] = keysAndValues[i+1]
	}

	// logr permits a trailing key with no value; keep it rather than drop it, so
	// a caller's mistake is visible in the log instead of silently vanishing.
	if len(keysAndValues)%2 == 1 {
		last := keysAndValues[len(keysAndValues)-1]
		key, ok := last.(string)
		if !ok {
			key = fmt.Sprintf("%v", last)
		}
		fields[key] = "<no value>"
	}

	return fields
}

// mergeFields returns a new map so sinks derived from a shared parent cannot
// write into each other's fields.
func mergeFields(base, extra LogFields) LogFields {
	merged := make(LogFields, len(base)+len(extra))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	return merged
}
