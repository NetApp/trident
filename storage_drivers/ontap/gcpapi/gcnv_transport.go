// Copyright 2026 NetApp, Inc. All Rights Reserved.

package gcpapi

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
)

const (
	gcnvExpertModeAPIVersion = "v1"
	gcnvExpertModeAPIPathFmt = "%s/" + gcnvExpertModeAPIVersion + "/projects/%s/locations/%s/storagePools/%s/ontap"
)

func gcnvExpertModeProxyBase(proxyOrigin, projectNumber, location, poolID string) string {
	return fmt.Sprintf(gcnvExpertModeAPIPathFmt, proxyOrigin, projectNumber, location, poolID)
}

// GCNVOntapModeTransport is an http.RoundTripper that implements a GCNV Expert Mode
// translation layer: it rewrites ONTAP REST calls onto the GCNV ontap API surface.
// This is not a transparent HTTP proxy—GCP normalizes wire status and response shape,
// so callers must unwrap the envelope and may need to infer ONTAP semantics (see RoundTrip).
//
// Transformations applied per the GCNV Expert Mode spec:
//
//  1. URL rewrite: prepend the Expert Mode base path.
//
//  2. Query param rename: "fields" → "ontap_fields" (GCP reserves "fields" as a system param).
//
//  3. Request body wrap: POST/PATCH/PUT bodies are wrapped in {"body": <original>}.
//
//  4. Auth: replace any ONTAP BasicAuth with a GCP Bearer token.
//
//  5. Response unwrap: extract the inner "body" from the API envelope {"body": ...}.
type GCNVOntapModeTransport struct {
	inner             http.RoundTripper
	proxyBaseURL      string
	tokenSource       oauth2.TokenSource
	storageDriverName string
	debugTraceFlags   map[string]bool
}

// NewGCNVOntapModeTransport creates a transport that routes ONTAP REST calls through the GCNV proxy.
func NewGCNVOntapModeTransport(config *GCNVOntapModeConfig) (*GCNVOntapModeTransport, error) {
	if config == nil || config.ProxyURL == "" || config.ProjectNumber == "" ||
		config.Location == "" || config.PoolID == "" {
		return nil, fmt.Errorf("GCNVOntapModeConfig requires ProxyURL, ProjectNumber, Location, PoolID")
	}
	parsedURL, err := url.Parse(config.ProxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid ProxyURL %q: %w", config.ProxyURL, err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("ProxyURL must include a scheme and host")
	}
	if strings.ToLower(parsedURL.Scheme) != "https" {
		return nil, fmt.Errorf("ProxyURL must use https scheme")
	}
	// Use scheme+host only; ignore any path/query/fragment on ProxyURL so a misconfigured
	// value cannot double-prefix the expert-mode API path.
	proxyOrigin := strings.TrimRight(fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host), "/")
	base := gcnvExpertModeProxyBase(proxyOrigin, config.ProjectNumber, config.Location, config.PoolID)

	driverName := config.StorageDriverName
	if driverName == "" {
		driverName = tridentconfig.OntapSANStorageDriverName
	}

	return &GCNVOntapModeTransport{
		inner: &http.Transport{
			TLSClientConfig: &tls.Config{MinVersion: tridentconfig.MinClientTLSVersion},
		},
		proxyBaseURL:      base,
		tokenSource:       config.TokenSource,
		storageDriverName: driverName,
		debugTraceFlags:   config.DebugTraceFlags,
	}, nil
}

// RoundTrip rewrites the request for the GCNV proxy and unwraps the response.
func (t *GCNVOntapModeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	fields := LogFields{"Method": "RoundTrip", "Type": "GCNVOntapModeTransport"}
	Logd(ctx, t.storageDriverName, t.debugTraceFlags["method"]).WithFields(fields).Trace(">>>> RoundTrip")
	defer Logd(ctx, t.storageDriverName, t.debugTraceFlags["method"]).WithFields(fields).Trace("<<<< RoundTrip")

	if req.Body != nil {
		defer req.Body.Close()
	}

	newReq := req.Clone(ctx)
	// Avoid having the cloned request's Body/GetBody reference the original body,
	// which may be read and closed later in this method. The body-handling logic
	// below will explicitly set newReq.Body/GetBody when needed.
	newReq.Body = http.NoBody
	newReq.GetBody = func() (io.ReadCloser, error) { return http.NoBody, nil }
	newReq.ContentLength = 0

	// GCP reserves "fields" as a system parameter for response field masks.
	// The GCNV proxy uses "ontap_fields" to pass the ONTAP fields parameter through.
	q := req.URL.Query()
	if fieldsParam := q.Get("fields"); fieldsParam != "" {
		q.Set("ontap_fields", fieldsParam)
		q.Del("fields")
	}

	// Rewrite URL: keep the original path (/api/...) and query, prepend the proxy base.
	proxyRaw := t.proxyBaseURL + req.URL.Path
	if encoded := q.Encode(); encoded != "" {
		proxyRaw += "?" + encoded
	}
	parsed, err := url.Parse(proxyRaw)
	if err != nil {
		return nil, fmt.Errorf("gcnv proxy transport: parse rewritten URL: %w", err)
	}
	newReq.URL = parsed
	newReq.Host = parsed.Host

	Logc(ctx).WithFields(LogFields{
		"method": req.Method, "originalURL": req.URL.String(), "proxyURL": parsed.String(),
	}).Trace("Rewriting GCNV proxy request.")

	// Wrap request body in {"body": <original>} only when gcnvMutatingRequestHasBody is true
	// (skips query-only PATCH such as snapshot restore: http.NoBody, ContentLength 0).
	// Prefer req.GetBody when set so LimitedRetryTransport can replay the same *http.Request.
	// When GetBody is absent (some go-openapi calls), read req.Body once without mutating
	// the caller request — intentional per RoundTripper contract; that path is not retry-safe.
	if gcnvMutatingRequestHasBody(req) {
		var bodyReader io.ReadCloser
		if req.GetBody != nil {
			var readErr error
			bodyReader, readErr = req.GetBody()
			if readErr != nil {
				return nil, fmt.Errorf("gcnv proxy transport: get request body: %w", readErr)
			}
		} else {
			// GetBody unavailable: consume req.Body directly (closed by defer above).
			bodyReader = req.Body
		}

		origBody, readErr := io.ReadAll(bodyReader)
		if req.GetBody != nil {
			_ = bodyReader.Close()
		}
		if readErr != nil {
			return nil, fmt.Errorf("gcnv proxy transport: read request body: %w", readErr)
		}

		// Set newReq body when non-empty; proxy gets wrapped envelope. When empty, newReq already has NoBody from above.
		if len(origBody) > 0 {
			wrapped := map[string]json.RawMessage{"body": origBody}
			wrappedBytes, marshalErr := json.Marshal(wrapped)
			if marshalErr != nil {
				return nil, fmt.Errorf("gcnv proxy transport: wrap request body: %w", marshalErr)
			}
			newReq.Body = io.NopCloser(bytes.NewReader(wrappedBytes))
			newReq.ContentLength = int64(len(wrappedBytes))
			newReq.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(wrappedBytes)), nil
			}
		}
	}

	// Replace auth with GCP Bearer token.
	newReq.Header.Del("Authorization")
	if t.tokenSource != nil {
		tok, err := t.tokenSource.Token()
		if err != nil {
			return nil, fmt.Errorf("gcnv proxy transport: get GCP token: %w", err)
		}
		newReq.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	}

	resp, err := t.inner.RoundTrip(newReq)
	if err != nil {
		return nil, err
	}

	// Read and unwrap the proxy response envelope.
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("gcnv proxy transport: read response: %w", err)
	}

	Logc(ctx).WithFields(LogFields{
		"method": req.Method, "proxyURL": parsed.String(), "statusCode": resp.StatusCode, "bodyLen": len(body),
	}).Trace("Received GCNV proxy response.")

	if resp.StatusCode >= http.StatusBadRequest {
		Logc(ctx).WithFields(LogFields{
			"method": req.Method, "proxyURL": parsed.String(), "statusCode": resp.StatusCode,
		}).Warn("Received error response from GCNV proxy.")
		// Log body only at debug, truncated to avoid sensitive or large payloads in logs.
		bodyLog := string(body)
		if len(bodyLog) > MaxErrorBodyLogBytes {
			bodyLog = bodyLog[:MaxErrorBodyLogBytes] + "..."
		}
		Logc(ctx).WithFields(LogFields{
			"method": req.Method, "statusCode": resp.StatusCode, "body": bodyLog,
		}).Debug("GCNV proxy error response body.")
	}

	unwrapped := unwrapProxyResponse(body)

	// GCP's Expert Mode API often returns HTTP 200 on the wire even when ONTAP would
	// have returned a different status (e.g. 202 for async jobs, 201 for sync POST creates).
	// That swallows ONTAP status codes at the transport layer; go-openapi still expects
	// real ONTAP semantics, so we best-effort infer status from the unwrapped body when
	// the wire status is 200.
	ontapStatus := inferONTAPStatus(req.Method, unwrapped)
	if ontapStatus > 0 && resp.StatusCode == http.StatusOK && ontapStatus != http.StatusOK {
		Logc(ctx).WithFields(LogFields{
			"method": req.Method, "proxyURL": parsed.String(), "proxyStatus": resp.StatusCode, "ontapStatus": ontapStatus,
		}).Trace("Restoring ONTAP status code from unwrapped response body.")
		resp.StatusCode = ontapStatus
		resp.Status = fmt.Sprintf("%d %s", ontapStatus, http.StatusText(ontapStatus))
	}

	resp.Body = io.NopCloser(bytes.NewReader(unwrapped))
	resp.ContentLength = int64(len(unwrapped))

	return resp, nil
}

// gcnvMutatingRequestHasBody reports whether the request carries a non-empty body to wrap.
func gcnvMutatingRequestHasBody(req *http.Request) bool {
	switch req.Method {
	case http.MethodPost, http.MethodPatch, http.MethodPut:
	default:
		return false
	}
	if req.Body == nil || req.Body == http.NoBody {
		return false
	}
	// ContentLength 0: no payload (e.g. PATCH restore_to.snapshot.name via query only).
	if req.ContentLength == 0 {
		return false
	}
	return true
}

// inferONTAPStatus approximates the ONTAP HTTP status from the unwrapped body
// (the GCNV proxy normalizes success to 200). The go-openapi client branches on
// these codes: 202 when the body has a top-level "job" object, 201 for other
// POST success, else 0 (keep wire 200).
func inferONTAPStatus(method string, body []byte) int {
	if method == http.MethodGet {
		return 0
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return 0
	}

	if _, hasJob := obj["job"]; hasJob {
		return http.StatusAccepted // 202
	}

	// ONTAP REST POST operations always return 201 (Created) for synchronous
	// resource creation. The only exception is async jobs (202), handled above.
	if method == http.MethodPost {
		return http.StatusCreated // 201
	}

	return 0
}

// unwrapProxyResponse extracts the inner "body" from the GCNV proxy envelope {"body": ...}.
// If the payload is not that envelope, returns data unchanged (after error-code normalization).
func unwrapProxyResponse(data []byte) []byte {
	var envelope proxyResponse
	if err := json.Unmarshal(data, &envelope); err != nil || len(envelope.Body) == 0 {
		return normalizeErrorCodes(data)
	}

	return normalizeErrorCodes(envelope.Body)
}

// normalizeErrorCodes converts numeric "code" fields inside "error" objects to strings
// so the go-openapi ONTAP generated models can unmarshal them.
// e.g. {"error":{"code":6684674,...}} → {"error":{"code":"6684674",...}}
func normalizeErrorCodes(data []byte) []byte {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return data
	}
	errField, ok := raw["error"]
	if !ok {
		return data
	}

	var errObj map[string]json.RawMessage
	if err := json.Unmarshal(errField, &errObj); err != nil {
		return data
	}
	codeRaw, ok := errObj["code"]
	if !ok {
		return data
	}

	// If code is already a string (starts with '"'), nothing to do.
	trimmed := bytes.TrimSpace(codeRaw)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		return data
	}

	// It's a number — wrap it in quotes.
	errObj["code"] = json.RawMessage(`"` + string(trimmed) + `"`)
	newErr, err := json.Marshal(errObj)
	if err != nil {
		return data
	}
	raw["error"] = json.RawMessage(newErr)
	result, err := json.Marshal(raw)
	if err != nil {
		return data
	}
	return result
}
