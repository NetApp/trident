// Copyright 2021 NetApp, Inc. All Rights Reserved.

package logger

import (
	"context"

	log "github.com/sirupsen/logrus"
)

const (
	ContextKeyRequestID     ContextKey = "requestID"
	ContextKeyRequestSource ContextKey = "requestSource"
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
	AuditGRPCAccess   = AuditEvent("grpc")
	AuditDockerAccess = AuditEvent("docker")
)

// ContextKey is used for context.Context value. The value requires a key that is not primitive type.
type ContextKey string // ContextKeyRequestID is the ContextKey for RequestID

type AuditEvent string

type AuditLogger interface {
	Log(ctx context.Context, event AuditEvent, fields log.Fields, message string)
	Logln(ctx context.Context, event AuditEvent, fields log.Fields, message string)
	Logf(ctx context.Context, event AuditEvent, fields log.Fields, format string, args ...interface{})
}
