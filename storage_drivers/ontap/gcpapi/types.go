// Copyright 2026 NetApp, Inc. All Rights Reserved.

package gcpapi

import (
	"encoding/json"

	"golang.org/x/oauth2"
)

// MaxErrorBodyLogBytes is the maximum number of bytes of an error response body
// to include in logs. This is a local logging guard only (not an API version
// constant); it avoids dumping large or sensitive payloads at warn/debug.
const MaxErrorBodyLogBytes = 512

// GCNVOntapModeConfig holds the configuration for the GCNV ONTAP proxy (used by GCNVOntapModeTransport).
type GCNVOntapModeConfig struct {
	ProxyURL          string
	ProjectNumber     string
	Location          string
	PoolID            string
	StorageDriverName string
	DebugTraceFlags   map[string]bool
	TokenSource       oauth2.TokenSource
}

// proxyResponse is the response envelope from the GCNV ONTAP proxy.
// Used by GCNVOntapModeTransport to unwrap the inner ONTAP JSON.
type proxyResponse struct {
	Body json.RawMessage `json:"body"`
}
