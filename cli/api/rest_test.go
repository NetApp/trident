// Copyright 2022 NetApp, Inc. All Rights Reserved.

package api

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/logging"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	logging.InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func TestInvokeRESTAPI_Success(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		requestBody []byte
		serverResp  string
		statusCode  int
	}{
		{
			name:        "GET request with nil body",
			method:      "GET",
			requestBody: nil,
			serverResp:  `{"result": "success"}`,
			statusCode:  200,
		},
		{
			name:        "POST request with JSON body",
			method:      "POST",
			requestBody: []byte(`{"name": "test"}`),
			serverResp:  `{"id": "123"}`,
			statusCode:  201,
		},
		{
			name:        "PUT request with empty body",
			method:      "PUT",
			requestBody: []byte{},
			serverResp:  `{"updated": true}`,
			statusCode:  200,
		},
		{
			name:        "DELETE request",
			method:      "DELETE",
			requestBody: nil,
			serverResp:  "",
			statusCode:  204,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request method
				assert.Equal(t, tt.method, r.Method)

				// Verify Content-Type header
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				// Verify request body if provided
				if tt.requestBody != nil {
					body, err := io.ReadAll(r.Body)
					require.NoError(t, err)
					assert.Equal(t, tt.requestBody, body)
				}

				// Send response
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.serverResp))
			}))
			defer server.Close()

			// Call function
			resp, body, err := InvokeRESTAPI(tt.method, server.URL, tt.requestBody)

			// Assertions
			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Equal(t, tt.statusCode, resp.StatusCode)
			assert.Equal(t, []byte(tt.serverResp), body)
		})
	}
}

func TestInvokeRESTAPI_InvalidURL(t *testing.T) {
	// Test with invalid URL
	resp, body, err := InvokeRESTAPI("GET", "://invalid-url", nil)

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Nil(t, body)
}

func TestInvokeRESTAPI_NetworkError(t *testing.T) {
	// Test with non-existent server
	resp, body, err := InvokeRESTAPI("GET", "http://localhost:99999/nonexistent", nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error communicating with Trident REST API")
	assert.Nil(t, resp)
	assert.Nil(t, body)
}

func TestInvokeRESTAPI_ServerError(t *testing.T) {
	// Create server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	resp, body, err := InvokeRESTAPI("GET", server.URL, nil)

	require.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	assert.Equal(t, []byte(`{"error": "internal server error"}`), body)
}

func TestInvokeRESTAPI_ReadBodyError(t *testing.T) {
	// Create server that closes connection unexpectedly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100") // Claim more content than we'll send
		w.WriteHeader(200)
		w.Write([]byte("short"))
		// Close connection without sending full content
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, _ := hj.Hijack()
			conn.Close()
		}
	}))
	defer server.Close()

	// This may or may not trigger the read error depending on the HTTP implementation
	// But we still test the code path
	resp, body, err := InvokeRESTAPI("GET", server.URL, nil)

	// The behavior may vary, but we should get either a response or an error
	if err != nil {
		assert.Contains(t, strings.ToLower(err.Error()), "error")
	} else {
		assert.NotNil(t, resp)
	}
	// Body might be partial or nil
	_ = body
}

func TestLogHTTPRequest(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		url         string
		headers     map[string]string
		requestBody []byte
	}{
		{
			name:        "GET request with nil body",
			method:      "GET",
			url:         "http://example.com/api/v1/test",
			headers:     map[string]string{"Authorization": "Bearer token"},
			requestBody: nil,
		},
		{
			name:        "POST request with JSON body",
			method:      "POST",
			url:         "http://example.com/api/v1/create",
			headers:     map[string]string{"Content-Type": "application/json"},
			requestBody: []byte(`{"name": "test", "value": 123}`),
		},
		{
			name:        "PUT request with empty body",
			method:      "PUT",
			url:         "http://example.com/api/v1/update/123",
			headers:     map[string]string{"User-Agent": "test-agent"},
			requestBody: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			var body io.Reader
			if tt.requestBody != nil {
				body = bytes.NewBuffer(tt.requestBody)
			}

			req, err := http.NewRequest(tt.method, tt.url, body)
			require.NoError(t, err)

			// Add headers
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			// Test the function - should not panic
			assert.NotPanics(t, func() {
				LogHTTPRequest(req, tt.requestBody)
			})
		})
	}
}

func TestLogHTTPResponse(t *testing.T) {
	tests := []struct {
		name         string
		response     *http.Response
		responseBody []byte
	}{
		{
			name: "successful response with body",
			response: &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			},
			responseBody: []byte(`{"result": "success"}`),
		},
		{
			name: "error response with body",
			response: &http.Response{
				Status:     "404 Not Found",
				StatusCode: 404,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			},
			responseBody: []byte(`{"error": "not found"}`),
		},
		{
			name:         "nil response",
			response:     nil,
			responseBody: []byte(`{"message": "test"}`),
		},
		{
			name: "response with nil body",
			response: &http.Response{
				Status:     "204 No Content",
				StatusCode: 204,
				Header:     http.Header{},
			},
			responseBody: nil,
		},
		{
			name: "response with empty body",
			response: &http.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Header:     http.Header{"Content-Length": []string{"0"}},
			},
			responseBody: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the function - should not panic
			assert.NotPanics(t, func() {
				LogHTTPResponse(tt.response, tt.responseBody)
			})
		})
	}
}

// Test various edge cases and error conditions
func TestInvokeRESTAPI_EdgeCases(t *testing.T) {
	t.Run("empty method", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
		}))
		defer server.Close()

		resp, body, err := InvokeRESTAPI("", server.URL, nil)
		// Empty method should still work (defaults to GET in http.NewRequest)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 200, resp.StatusCode)
		_ = body
	})

	t.Run("very large request body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			w.WriteHeader(200)
			w.Write([]byte("received"))
			assert.True(t, len(body) > 1000)
		}))
		defer server.Close()

		largeBody := bytes.Repeat([]byte("x"), 10000)
		resp, body, err := InvokeRESTAPI("POST", server.URL, largeBody)

		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, []byte("received"), body)
	})

	t.Run("special characters in body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			w.WriteHeader(200)
			w.Write(body) // Echo back the body
		}))
		defer server.Close()

		specialBody := []byte(`{"text": "Hello 世界! 🌍 Special chars: \n\t\r\"'"}`)
		resp, body, err := InvokeRESTAPI("POST", server.URL, specialBody)

		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, specialBody, body)
	})
}

const (
	sentinelUser = "SENTINEL_USER_4c1e"
	sentinelPass = "SENTINEL_PAS_8b6d"
)

// captureLog points the package-global logger at a buffer, runs fn, and returns what was logged.
// TestMain leaves the logger discarding output for this package's other tests, so that setup is
// restored on cleanup.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()

	t.Cleanup(func() {
		if err := logging.InitLogLevel("info"); err != nil {
			t.Fatalf("InitLogLevel(info): %v", err)
		}
		logging.InitLogOutput(io.Discard)
	})

	if err := logging.InitLogLevel("debug"); err != nil {
		t.Fatalf("InitLogLevel(debug): %v", err)
	}

	buf := &bytes.Buffer{}
	logging.InitLogOutput(buf)
	fn()

	return buf.String()
}

// credentialForms returns every representation of the sentinel pair that could appear in a log
// line, since an Authorization header carries the base64 form rather than the sentinel text.
func credentialForms() []string {
	return []string{
		sentinelUser,
		sentinelPass,
		base64.StdEncoding.EncodeToString([]byte(sentinelUser + ":" + sentinelPass)),
	}
}

// TestLogHTTPRequestDoesNotLogCredentials covers both ways a tridentctl invocation can carry a
// credential into the debug log: an Authorization header, and userinfo in the server URL, which is
// built from a flag or TRIDENT_SERVER and so needs no credential-handling code in this repository.
func TestLogHTTPRequestDoesNotLogCredentials(t *testing.T) {
	request, err := http.NewRequest(
		"POST", "http://"+sentinelUser+":"+sentinelPass+"@trident.trident.svc:17201/trident/v1/backend", nil)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.SetBasicAuth(sentinelUser, sentinelPass)

	// Guards the fixture itself: net/http is what decides that the credential is present at all.
	require.NotNil(t, request.URL.User)
	require.NotEmpty(t, request.Header.Get("Authorization"))

	out := captureLog(t, func() {
		LogHTTPRequest(request, []byte(`{"version":1}`))
	})

	// Without these the absence checks below would pass on a line that was never written.
	require.Contains(t, out, "Request URL:")
	require.Contains(t, out, "Request headers:")

	for _, form := range credentialForms() {
		assert.NotContains(t, out, form)
	}

	assert.Contains(t, out, "trident.trident.svc:17201", "the URL must stay useful for debugging")
	assert.Contains(t, out, "Content-Type", "benign headers must still be logged")
}

func TestLogHTTPResponseDoesNotLogCredentials(t *testing.T) {
	response := &http.Response{
		Status: "401 Unauthorized",
		Header: http.Header{
			"Authorization":    []string{"Basic " + sentinelPass},
			"Content-Type":     []string{"application/json"},
			"Www-Authenticate": []string{"Basic realm=trident"},
		},
	}

	out := captureLog(t, func() {
		LogHTTPResponse(response, []byte(`{"error":"unauthorized"}`))
	})

	require.Contains(t, out, "Response headers:")
	assert.NotContains(t, out, sentinelPass)
	assert.Contains(t, out, "Content-Type", "benign headers must still be logged")
}

// TestLogHTTPRequestDoesNotLogRequestBodyCredentials covers the third way a tridentctl invocation
// carries a credential into the debug log: `tridentctl create backend -d` sends the whole backend
// config as the request body, and the pattern-based redaction covers only the credential keys it was
// written for, so clientSecret, clientPrivateKey, secretKey and apiKey need the key-based walk.
func TestLogHTTPRequestDoesNotLogRequestBodyCredentials(t *testing.T) {
	body := []byte(`{"backend_name":"b1","config":{"storageDriverName":"azure-netapp-files",` +
		`"username":"Body-Sentinel-User","clientSecret":"Body-Sentinel-ClientSecret",` +
		`"clientPrivateKey":"Body-Sentinel-PrivateKey","secretKey":"Body-Sentinel-SecretKey",` +
		`"apiKey":"Body-Sentinel-APIKey","location":"eastus"}}`)

	request, err := http.NewRequest("POST", "http://trident.trident.svc:17201/trident/v1/backend", nil)
	require.NoError(t, err)

	out := captureLog(t, func() {
		LogHTTPRequest(request, body)
	})

	// Without these the absence checks below would pass on a trace that never showed the body.
	require.Contains(t, out, "Request body:")
	require.Contains(t, out, "<REDACTED>", "the credential must be replaced, not merely absent")

	for _, sentinel := range []string{
		"Body-Sentinel-User",
		"Body-Sentinel-ClientSecret",
		"Body-Sentinel-PrivateKey",
		"Body-Sentinel-SecretKey",
		"Body-Sentinel-APIKey",
	} {
		assert.NotContains(t, out, sentinel)
	}

	assert.Contains(t, out, "b1", "the body must stay useful for debugging")
	assert.Contains(t, out, "eastus")
	assert.Contains(t, out, "azure-netapp-files")
}

// TestLogHTTPResponseDoesNotLogResponseBodyCredentials covers the same helper on the way back; the
// body of a Trident REST reply is rendered by the same line.
func TestLogHTTPResponseDoesNotLogResponseBodyCredentials(t *testing.T) {
	response := &http.Response{
		Status: "200 OK",
		Header: http.Header{"Content-Type": []string{"application/json"}},
	}

	out := captureLog(t, func() {
		LogHTTPResponse(response, []byte(
			`{"result":"success","config":{"clientSecret":"Body-Sentinel-ClientSecret"}}`))
	})

	require.Contains(t, out, "Response body:")
	assert.NotContains(t, out, "Body-Sentinel-ClientSecret")
	assert.Contains(t, out, "success", "the reply must stay useful for debugging")
}
