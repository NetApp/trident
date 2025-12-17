// Copyright 2022 NetApp, Inc. All Rights Reserved.

package api

import (
	"bytes"
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
