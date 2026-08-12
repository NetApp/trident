// Copyright 2026 NetApp, Inc. All Rights Reserved.

package gcpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/sync/semaphore"

	"github.com/netapp/trident/logging"
	storagedrivers "github.com/netapp/trident/storage_drivers"
)

// staticTokenSource always returns the same token.
type staticTokenSource struct{ token string }

func (s *staticTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: s.token}, nil
}

// failingTokenSource always returns an error.
type failingTokenSource struct{}

func (f *failingTokenSource) Token() (*oauth2.Token, error) {
	return nil, fmt.Errorf("token refresh failed")
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

const (
	prodProxyOrigin     = "https://netapp.googleapis.com"
	autopushProxyOrigin = "https://autopush-netapp.sandbox.googleapis.com"
)

func newTestConfig(proxyURL string) *GCNVOntapModeConfig {
	return newGCNVOntapModeConfig(proxyURL, "123456", "us-central1-a", "pool-1")
}

func newGCNVOntapModeConfig(proxyURL, projectNumber, location, poolID string) *GCNVOntapModeConfig {
	return &GCNVOntapModeConfig{
		ProxyURL:      proxyURL,
		ProjectNumber: projectNumber,
		Location:      location,
		PoolID:        poolID,
		TokenSource:   &staticTokenSource{token: "test-token"},
	}
}

// wantProxyBase mirrors NewGCNVOntapModeTransport: scheme+host from proxyURL, then expert-mode path from config fields.
func wantProxyBase(proxyURL, projectNumber, location, poolID string) string {
	parsed, err := url.Parse(proxyURL)
	if err != nil {
		panic(err)
	}
	origin := strings.TrimRight(fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host), "/")
	return gcnvExpertModeProxyBase(origin, projectNumber, location, poolID)
}

func newTestTransportForTLSServer(t *testing.T, server *httptest.Server) *GCNVOntapModeTransport {
	t.Helper()
	tr, err := NewGCNVOntapModeTransport(newTestConfig(server.URL))
	require.NoError(t, err)
	tr.inner = server.Client().Transport
	return tr
}

func TestNewGCNVOntapModeTransport(t *testing.T) {
	validBaseURL := wantProxyBase(prodProxyOrigin, "123456", "us-central1-a", "pool-1")
	autopushBaseURL := wantProxyBase(autopushProxyOrigin, "711978674048", "us-east4", "autopush-ontap-unified-e4")
	tests := []struct {
		name             string
		config           *GCNVOntapModeConfig
		wantErr          bool
		wantProxyBaseURL string
	}{
		{
			name:             "valid",
			config:           newTestConfig("https://netapp.googleapis.com"),
			wantErr:          false,
			wantProxyBaseURL: validBaseURL,
		},
		{
			name:             "trailing slash normalized",
			config:           newTestConfig("https://netapp.googleapis.com/"),
			wantErr:          false,
			wantProxyBaseURL: validBaseURL,
		},
		{
			name:             "path query fragment ignored",
			config:           newTestConfig("https://netapp.googleapis.com/extra/path?foo=bar#frag"),
			wantErr:          false,
			wantProxyBaseURL: validBaseURL,
		},
		{
			name: "production full expert path ignored uses config project location pool",
			config: newGCNVOntapModeConfig(
				gcnvExpertModeProxyBase(prodProxyOrigin, "999", "wrong-region", "wrong-pool"),
				"123456", "us-central1-a", "pool-1",
			),
			wantErr:          false,
			wantProxyBaseURL: validBaseURL,
		},
		{
			name:             "autopush origin only",
			config:           newGCNVOntapModeConfig(autopushProxyOrigin, "711978674048", "us-east4", "autopush-ontap-unified-e4"),
			wantErr:          false,
			wantProxyBaseURL: autopushBaseURL,
		},
		{
			name:             "autopush trailing slash normalized",
			config:           newGCNVOntapModeConfig(autopushProxyOrigin+"/", "711978674048", "us-east4", "autopush-ontap-unified-e4"),
			wantErr:          false,
			wantProxyBaseURL: autopushBaseURL,
		},
		{
			name: "autopush pasted full expert path ignored",
			config: newGCNVOntapModeConfig(
				gcnvExpertModeProxyBase(autopushProxyOrigin, "711978674048", "us-east4", "autopush-ontap-unified-e4"),
				"711978674048", "us-east4", "autopush-ontap-unified-e4",
			),
			wantErr:          false,
			wantProxyBaseURL: autopushBaseURL,
		},
		{
			name: "autopush pasted path with wrong ids in URL still uses config",
			config: newGCNVOntapModeConfig(
				gcnvExpertModeProxyBase(autopushProxyOrigin, "0", "bad", "bad-pool")+"?alt=json",
				"711978674048", "us-east4", "autopush-ontap-unified-e4",
			),
			wantErr:          false,
			wantProxyBaseURL: autopushBaseURL,
		},
		{
			name:             "autopush explicit port 443",
			config:           newGCNVOntapModeConfig("https://autopush-netapp.sandbox.googleapis.com:443", "711978674048", "us-east4", "autopush-ontap-unified-e4"),
			wantErr:          false,
			wantProxyBaseURL: wantProxyBase("https://autopush-netapp.sandbox.googleapis.com:443", "711978674048", "us-east4", "autopush-ontap-unified-e4"),
		},
		{
			name:             "nil config",
			config:           nil,
			wantErr:          true,
			wantProxyBaseURL: "",
		},
		{
			name:             "empty proxyURL",
			config:           &GCNVOntapModeConfig{ProxyURL: "", ProjectNumber: "1", Location: "l", PoolID: "p"},
			wantErr:          true,
			wantProxyBaseURL: "",
		},
		{
			name:             "empty projectNumber",
			config:           &GCNVOntapModeConfig{ProxyURL: "https://x", ProjectNumber: "", Location: "l", PoolID: "p"},
			wantErr:          true,
			wantProxyBaseURL: "",
		},
		{
			name:             "empty location",
			config:           &GCNVOntapModeConfig{ProxyURL: "https://x", ProjectNumber: "1", Location: "", PoolID: "p"},
			wantErr:          true,
			wantProxyBaseURL: "",
		},
		{
			name:             "empty poolID",
			config:           &GCNVOntapModeConfig{ProxyURL: "https://x", ProjectNumber: "1", Location: "l", PoolID: ""},
			wantErr:          true,
			wantProxyBaseURL: "",
		},
		{
			name:             "http scheme rejected",
			config:           newTestConfig("http://netapp.googleapis.com"),
			wantErr:          true,
			wantProxyBaseURL: "",
		},
		{
			name:             "missing scheme",
			config:           newTestConfig("//netapp.googleapis.com"),
			wantErr:          true,
			wantProxyBaseURL: "",
		},
		{
			name:             "missing host",
			config:           &GCNVOntapModeConfig{ProxyURL: "https://", ProjectNumber: "1", Location: "l", PoolID: "p"},
			wantErr:          true,
			wantProxyBaseURL: "",
		},
		{
			name:             "HTTPS scheme case insensitive",
			config:           newTestConfig("HTTPS://netapp.googleapis.com"),
			wantErr:          false,
			wantProxyBaseURL: validBaseURL,
		},
		{
			name:             "empty StorageDriverName uses default ontap driver",
			config:           &GCNVOntapModeConfig{ProxyURL: "https://netapp.googleapis.com", ProjectNumber: "123456", Location: "us-central1-a", PoolID: "pool-1", TokenSource: &staticTokenSource{token: "t"}},
			wantErr:          false,
			wantProxyBaseURL: validBaseURL,
		},
		{
			name:             "StorageDriverName set is accepted",
			config:           &GCNVOntapModeConfig{ProxyURL: "https://netapp.googleapis.com", ProjectNumber: "123456", Location: "us-central1-a", PoolID: "pool-1", StorageDriverName: "ontap-nas", TokenSource: &staticTokenSource{token: "t"}},
			wantErr:          false,
			wantProxyBaseURL: validBaseURL,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := NewGCNVOntapModeTransport(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, tr)
			assert.Equal(t, tt.wantProxyBaseURL, tr.proxyBaseURL)
		})
	}
}

func TestRoundTrip_URLRewriteAndAuth(t *testing.T) {
	// Fake proxy echoes request path, query, and auth header.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"path":  r.URL.Path,
			"query": r.URL.RawQuery,
			"auth":  r.Header.Get("Authorization"),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tr := newTestTransportForTLSServer(t, server)

	req, _ := http.NewRequest(http.MethodGet, "https://placeholder/api/storage/volumes?fields=name,size", nil)
	req.SetBasicAuth("admin", "password")

	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	parsedBase, err := url.Parse(wantProxyBase(prodProxyOrigin, "123456", "us-central1-a", "pool-1"))
	require.NoError(t, err)
	assert.Contains(t, result["path"], parsedBase.Path+"/api/storage/volumes")
	query := result["query"].(string)
	assert.Contains(t, query, "ontap_fields=")
	assert.NotRegexp(t, `(^|&)fields=`, query, "original 'fields' param should be removed")
	assert.Equal(t, "Bearer test-token", result["auth"])
}

func TestRoundTrip_RequestBodyWrapping(t *testing.T) {
	// POST body should be wrapped in {"body": <original>} for the CCFE gateway.
	var receivedBody []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"body":{"uuid":"vol-123"}}`))
	}))
	defer server.Close()

	tr := newTestTransportForTLSServer(t, server)

	originalBody := `{"name":"vol1","size":1073741824}`
	req, _ := http.NewRequest(http.MethodPost, "https://placeholder/api/storage/volumes", bytes.NewBufferString(originalBody))
	req.ContentLength = int64(len(originalBody))

	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Verify the request body was wrapped.
	var wrapped map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(receivedBody, &wrapped))
	assert.JSONEq(t, originalBody, string(wrapped["body"]), "request body should be wrapped in {\"body\": ...}")
}

func TestGCNVOntapModeTransport_RetryReplaysBodyAfterEOF(t *testing.T) {
	var firstBody []byte
	callCount := 0
	mockTransport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		callCount++
		if req.Body != nil {
			body, _ := io.ReadAll(req.Body)
			req.Body.Close()
			if callCount == 1 {
				firstBody = body
				return nil, io.EOF
			}
			if !bytes.Equal(firstBody, body) {
				return nil, fmt.Errorf("retry body mismatch: first %d bytes, retry %d", len(firstBody), len(body))
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(`{"body":{}}`))}, nil
	})

	transport, err := NewGCNVOntapModeTransport(&GCNVOntapModeConfig{
		ProxyURL: "https://netapp.googleapis.com", ProjectNumber: "1", Location: "loc", PoolID: "p1", TokenSource: nil,
	})
	require.NoError(t, err)
	transport.inner = mockTransport
	wrapped := storagedrivers.NewLimitedRetryTransport(semaphore.NewWeighted(10), transport,
		logging.ContextRequestTargetUnknown)

	body := []byte(`{"name":"vol1","size":1073741824}`)
	req, _ := http.NewRequest(http.MethodPost, "https://proxy/api/storage/volumes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := wrapped.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, 201, resp.StatusCode)
	assert.Equal(t, 2, callCount, "retry should have been attempted after EOF")
	assert.NotEmpty(t, firstBody, "retry request should have had same wrapped body")
}

func TestGCNVOntapModeTransport_BodyWrappedWhenGetBodyAbsent(t *testing.T) {
	// Some go-openapi calls set Body with ContentLength > 0 but omit GetBody; transport
	// must wrap via req.Body rather than failing. (Snapshot restore is query-only; see
	// TestRoundTrip_EmptyBodyPatchWithoutGetBody.)
	var gotBodyBytes []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBodyBytes, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"body":{}}`))
	}))
	defer server.Close()

	tr := newTestTransportForTLSServer(t, server)
	body := []byte(`{"name":"vol1"}`)
	req, err := http.NewRequest(http.MethodPost, "https://placeholder/api/storage/volumes",
		io.NopCloser(bytes.NewReader(body)))
	require.NoError(t, err)
	req.ContentLength = int64(len(body))
	req.GetBody = nil // intentionally absent, as with some go-openapi generated calls

	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Verify the body was wrapped in {"body": ...} envelope at the proxy.
	var wrapped map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(gotBodyBytes, &wrapped))
	require.Contains(t, wrapped, "body")
	assert.JSONEq(t, `{"name":"vol1"}`, string(wrapped["body"]))
}

func TestRoundTrip_EmptyBodyPatchWithoutGetBody(t *testing.T) {
	// Snapshot restore: PATCH with restore_to.snapshot.name query only, no JSON body.
	var gotMethod, gotQuery string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotQuery = r.URL.RawQuery
		assert.Equal(t, int64(0), r.ContentLength)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"body":{"job":{"uuid":"job-1","_links":{"self":{"href":"/api/cluster/jobs/job-1"}}}}}`))
	}))
	defer server.Close()

	tr := newTestTransportForTLSServer(t, server)
	volUUID := "ec85cfe1-5bb1-11f1-92b5-7566334cc35d"
	snapName := "snapshot-e7ece282-bea7-4dec-9da4-7920c786d695"
	rawURL := fmt.Sprintf("https://placeholder/api/storage/volumes/%s?restore_to.snapshot.name=%s", volUUID, snapName)
	req, err := http.NewRequest(http.MethodPatch, rawURL, http.NoBody)
	require.NoError(t, err)
	req.ContentLength = 0
	req.GetBody = nil

	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.MethodPatch, gotMethod)
	assert.Contains(t, gotQuery, "restore_to.snapshot.name="+snapName)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
}

func TestRoundTrip_GetBodyNotWrapped(t *testing.T) {
	// GET requests have no body to wrap, just URL rewrite.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"body":{"version":{"generation":9,"major":18,"minor":1}}}`))
	}))
	defer server.Close()

	tr := newTestTransportForTLSServer(t, server)

	req, _ := http.NewRequest(http.MethodGet, "https://placeholder/api/cluster", nil)
	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	// Response envelope should be unwrapped.
	assert.Contains(t, string(body), `"generation":9`)
	assert.NotContains(t, string(body), `"body"`)
}

func TestRoundTrip_ResponseEnvelopeUnwrap(t *testing.T) {
	// CCFE returns {"body": <ontap>}; transport should unwrap to raw ONTAP JSON.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"body":{"uuid":"vol-123","name":"vol1"}}`))
	}))
	defer server.Close()

	tr := newTestTransportForTLSServer(t, server)

	req, _ := http.NewRequest(http.MethodPost, "https://placeholder/api/storage/volumes", bytes.NewBufferString(`{"name":"vol1"}`))
	req.ContentLength = int64(len(`{"name":"vol1"}`))
	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.JSONEq(t, `{"uuid":"vol-123","name":"vol1"}`, string(body))
	assert.NotContains(t, string(body), `"body"`)
}

func TestRoundTrip_ResponseNoEnvelope(t *testing.T) {
	// Response without {"body": ...} envelope is returned as-is.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"uuid":"vol-123","name":"vol1"}`))
	}))
	defer server.Close()

	tr := newTestTransportForTLSServer(t, server)

	req, _ := http.NewRequest(http.MethodGet, "https://placeholder/api/storage/volumes/vol-123", nil)
	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.JSONEq(t, `{"uuid":"vol-123","name":"vol1"}`, string(body))
}

func TestRoundTrip_ErrorResponseUnwrap(t *testing.T) {
	// CCFE may also envelope error responses.
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"body":{"error":{"code":"4","message":"entry doesn't exist"}}}`))
	}))
	defer server.Close()

	tr := newTestTransportForTLSServer(t, server)

	req, _ := http.NewRequest(http.MethodGet, "https://placeholder/api/storage/volumes/missing", nil)
	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), `"entry doesn't exist"`)
	assert.NotContains(t, string(body), `"body"`)
}

// TestRoundTrip_NotFoundErrorStatusRestored covers inferONTAPErrorStatus: GCNV reports a
// missing volume as 400 on DELETE-by-UUID, and callers need ONTAP's 404 to recognize an
// already-deleted volume. Remap is scoped to that path and to explicit missing-volume signals.
func TestRoundTrip_NotFoundErrorStatusRestored(t *testing.T) {
	// Verbatim GCNV proxy response for a DELETE of a volume that no longer exists: the
	// wire status is 400 and the ONTAP reason is nested inside the message.
	const gcnvVolumeMissing = `{"error":{"code":"400","message":"code: 400, message: {\n  \"code\":  400,\n  ` +
		`\"message\":  \"bad request: volume with UUID 'f2cac576-95a8-11f1-9db7-6538c389414f' not found\"\n}"}}`

	tests := []struct {
		name       string
		method     string
		path       string
		wireStatus int
		body       string
		wantStatus int
	}{
		{
			"GCNV volume missing on delete", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusBadRequest, gcnvVolumeMissing, http.StatusNotFound,
		},
		{
			"ONTAP entry doesn't exist code", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusBadRequest, `{"error":{"code":"4","message":"entry doesn't exist"}}`, http.StatusNotFound,
		},
		{
			"generic does not exist is left alone", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusBadRequest, `{"error":{"code":"400","message":"volume vol1 does not exist"}}`,
			http.StatusBadRequest,
		},
		{
			"unrelated bad request is left alone", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusBadRequest,
			`{"error":{"code":"400","message":"bad request: volume with UUID 'x' is in a transitional state"}}`,
			http.StatusBadRequest,
		},
		{
			"unquoted UUID is remapped", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusBadRequest,
			`{"error":{"code":"400","message":"volume with UUID f2cac576-95a8-11f1-9db7-6538c389414f not found"}}`,
			http.StatusNotFound,
		},
		{
			"unquoted non-UUID token is left alone", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusBadRequest,
			`{"error":{"code":"400","message":"volume with UUID field not found"}}`,
			http.StatusBadRequest,
		},
		{
			"single-quoted non-UUID token is left alone", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusBadRequest,
			`{"error":{"code":"400","message":"volume with UUID 'field' not found"}}`,
			http.StatusBadRequest,
		},
		{
			"double-quoted UUID is remapped", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusBadRequest,
			`{"error":{"code":"400","message":"volume with UUID \"f2cac576-95a8-11f1-9db7-6538c389414f\" not found"}}`,
			http.StatusNotFound,
		},
		{
			"double-quoted non-UUID token is left alone", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusBadRequest,
			`{"error":{"code":"400","message":"volume with UUID \"field\" not found"}}`,
			http.StatusBadRequest,
		},
		{
			"mismatched quotes are left alone", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusBadRequest,
			`{"error":{"code":"400","message":"volume with UUID 'f2cac576-95a8-11f1-9db7-6538c389414f\" not found"}}`,
			http.StatusBadRequest,
		},
		{
			"non-delete method is left alone", http.MethodPatch, "/api/storage/volumes/vol-1",
			http.StatusBadRequest, gcnvVolumeMissing, http.StatusBadRequest,
		},
		{
			"non-volume path is left alone", http.MethodDelete, "/api/cluster",
			http.StatusBadRequest, gcnvVolumeMissing, http.StatusBadRequest,
		},
		{
			"numeric code and null message do not panic", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusBadRequest, `{"error":{"code":6684674,"message":null}}`, http.StatusBadRequest,
		},
		{
			"error payload absent", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusBadRequest, `{"records":[]}`, http.StatusBadRequest,
		},
		{
			"non-JSON body", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusBadRequest, `<html>bad request</html>`, http.StatusBadRequest,
		},
		{
			"other error statuses pass through", http.MethodDelete, "/api/storage/volumes/vol-1",
			http.StatusForbidden, `{"error":{"code":"403","message":"volume with UUID 'x' not found"}}`,
			http.StatusForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.wireStatus)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			tr := newTestTransportForTLSServer(t, server)

			req, _ := http.NewRequest(tt.method, "https://placeholder"+tt.path, nil)
			resp, err := tr.RoundTrip(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, tt.wantStatus, resp.StatusCode)
			assert.Equal(t, fmt.Sprintf("%d %s", tt.wantStatus, http.StatusText(tt.wantStatus)), resp.Status)
		})
	}
}

// TestRoundTrip_NotFoundErrorStatusRestoredThroughEnvelope verifies the not-found status is
// recovered when the proxy also wraps the error in its {"body": ...} envelope.
func TestRoundTrip_NotFoundErrorStatusRestoredThroughEnvelope(t *testing.T) {
	const missingUUID = "f2cac576-95a8-11f1-9db7-6538c389414f"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"body":{"error":{"code":"400","message":"volume with UUID '` + missingUUID + `' not found"}}}`))
	}))
	defer server.Close()

	tr := newTestTransportForTLSServer(t, server)

	req, _ := http.NewRequest(http.MethodDelete, "https://placeholder/api/storage/volumes/"+missingUUID, nil)
	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.NotContains(t, string(body), `"body"`)
}

func TestRoundTrip_TokenError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)
	cfg.TokenSource = &failingTokenSource{}
	tr, err := NewGCNVOntapModeTransport(cfg)
	require.NoError(t, err)
	tr.inner = server.Client().Transport

	req, _ := http.NewRequest(http.MethodGet, "https://placeholder/api/cluster", nil)
	_, err = tr.RoundTrip(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "get GCP token")
}

func TestRoundTrip_NilTokenSource(t *testing.T) {
	var receivedAuth string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)
	cfg.TokenSource = nil
	tr, err := NewGCNVOntapModeTransport(cfg)
	require.NoError(t, err)
	tr.inner = server.Client().Transport

	req, _ := http.NewRequest(http.MethodGet, "https://placeholder/api/cluster", nil)
	req.Header.Set("Authorization", "Basic oldtoken") // should be removed

	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Empty(t, receivedAuth, "no auth header should be sent when tokenSource is nil")
}

func TestRoundTrip_NoQueryParams(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"path":  r.URL.Path,
			"query": r.URL.RawQuery,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	tr := newTestTransportForTLSServer(t, server)

	req, _ := http.NewRequest(http.MethodGet, "https://placeholder/api/cluster", nil)
	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(body, &result))

	assert.Contains(t, result["path"], "/ontap/api/cluster")
	assert.Empty(t, result["query"], "no query params should be appended")
}

// TestRoundTrip_WithStorageDriverNameAndTraceFlags verifies RoundTrip works when
// StorageDriverName and DebugTraceFlags are set (covers Logd path with driver name and trace flag).
func TestRoundTrip_WithStorageDriverNameAndTraceFlags(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"body":{"version":{"generation":9}}}`))
	}))
	defer server.Close()

	cfg := newTestConfig(server.URL)
	cfg.StorageDriverName = "ontap-nas"
	cfg.DebugTraceFlags = map[string]bool{"method": true}
	tr, err := NewGCNVOntapModeTransport(cfg)
	require.NoError(t, err)
	tr.inner = server.Client().Transport

	req, _ := http.NewRequest(http.MethodGet, "https://placeholder/api/cluster", nil)
	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// TestRoundTrip_inferONTAPStatus_AsyncJob exercises inferONTAPStatus: top-level "job" → 202 for POST, PATCH, and DELETE.
func TestRoundTrip_inferONTAPStatus_AsyncJob(t *testing.T) {
	const innerJSON = `{"uuid":"vol-async","job":{"uuid":"job-1"}}`
	envelope := `{"body":` + innerJSON + `}`

	tests := []struct {
		name   string
		method string
	}{
		{"POST", http.MethodPost},
		{"PATCH", http.MethodPatch},
		{"DELETE", http.MethodDelete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, tt.method, r.Method)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(envelope))
			}))
			defer server.Close()

			tr := newTestTransportForTLSServer(t, server)

			var req *http.Request
			if tt.method == http.MethodDelete {
				req, _ = http.NewRequest(http.MethodDelete, "https://placeholder/api/storage/volumes/vol-1", nil)
			} else {
				req, _ = http.NewRequest(tt.method, "https://placeholder/api/storage/volumes/vol-1", bytes.NewBufferString(`{}`))
				req.ContentLength = 2
			}

			resp, err := tr.RoundTrip(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusAccepted, resp.StatusCode, "method=%s", tt.method)
			body, _ := io.ReadAll(resp.Body)
			assert.JSONEq(t, innerJSON, string(body))
		})
	}
}

// TestRoundTrip_inferONTAPStatus_PostNonObjectBodyLeaves200 covers POST bodies that are not a JSON object
// after unwrap (inferONTAPStatus cannot map-unmarshal → leaves proxy 200).
func TestRoundTrip_inferONTAPStatus_PostNonObjectBodyLeaves200(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Inner body is a JSON array — cannot infer POST→201 or job→202 from map lookup.
		_, _ = w.Write([]byte(`{"body":[1,2,3]}`))
	}))
	defer server.Close()

	tr := newTestTransportForTLSServer(t, server)

	req, _ := http.NewRequest(http.MethodPost, "https://placeholder/api/storage/volumes", bytes.NewBufferString(`{}`))
	req.ContentLength = 2

	resp, err := tr.RoundTrip(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, `[1,2,3]`, string(body))
}
