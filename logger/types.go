package logger

const (
	ContextKeyRequestID     ContextKey = "requestID"
	ContextKeyRequestSource ContextKey = "requestSource"

	ContextSourceCRD      = "CRD"
	ContextSourceREST     = "REST"
	ContextSourceK8S      = "Kubernetes"
	ContextSourceDocker   = "Docker"
	ContextSourceCSI      = "CSI"
	ContextSourceInternal = "Internal"
)

// ContextKey is used for context.Context value. The value requires a key that is not primitive type.
type ContextKey string // ContextKeyRequestID is the ContextKey for RequestID
