// Copyright 2022 NetApp, Inc. All Rights Reserved.

package logging

import (
	"context"

	log "github.com/sirupsen/logrus"
)

const (
	ContextKeyRequestID     ContextKey = "requestID"
	ContextKeyRequestSource ContextKey = "requestSource"
	ContextKeyWorkflow      ContextKey = "workflow"
	ContextKeyLogLayer      ContextKey = "logLayer"
	CRDControllerEvent      ContextKey = "crdControllerEvent"

	ContextSourceCRD      = "CRD"
	ContextSourceREST     = "REST"
	ContextSourceK8S      = "Kubernetes"
	ContextSourceDocker   = "Docker"
	ContextSourceCSI      = "CSI"
	ContextSourceInternal = "Internal"
	ContextSourcePeriodic = "Periodic"

	LogSource = "logSource"

	AuditRESTAccess   = AuditEvent("rest")
	AuditGRPCAccess   = AuditEvent("csi")
	AuditDockerAccess = AuditEvent("docker")
)

// ContextKey is used for context.Context value. The value requires a key that is not primitive type.
type ContextKey string // ContextKeyRequestID is the ContextKey for RequestID

type WorkflowCategory string

func (w WorkflowCategory) String() string {
	return string(w)
}

type WorkflowOperation string

func (w WorkflowOperation) String() string {
	return string(w)
}

type Workflow struct {
	Category  WorkflowCategory
	Operation WorkflowOperation
}

func (w Workflow) String() string {
	return w.Category.String() + workflowCategorySeparator + w.Operation.String()
}

type LogLayer string

func (l LogLayer) String() string {
	return string(l)
}

type LogFields log.Fields

type LogEntry interface {
	WithField(key string, value interface{}) LogEntry
	WithFields(fields LogFields) LogEntry
	WithError(err error) LogEntry
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Error(args ...interface{})
	Errorf(format string, args ...interface{})
	Warn(args ...interface{})
	Warnf(format string, args ...interface{})
	Warning(args ...interface{})
	Warningf(format string, args ...interface{})
	Info(args ...interface{})
	Infof(format string, args ...interface{})
	Debug(args ...interface{})
	Debugf(format string, args ...interface{})
	Trace(args ...interface{})
	Tracef(format string, args ...interface{})
	Data(key string) (interface{}, bool)
}

type AuditEvent string

type AuditLogger interface {
	Log(ctx context.Context, event AuditEvent, fields LogFields, message string)
	Logln(ctx context.Context, event AuditEvent, fields LogFields, message string)
	Logf(ctx context.Context, event AuditEvent, fields LogFields, format string, args ...interface{})
}
