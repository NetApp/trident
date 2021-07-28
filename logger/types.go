// Copyright 2021 NetApp, Inc. All Rights Reserved.

package logger

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
)

// ContextKey is used for context.Context value. The value requires a key that is not primitive type.
type ContextKey string // ContextKeyRequestID is the ContextKey for RequestID
