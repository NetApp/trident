// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	drivers "github.com/netapp/trident/storage_drivers"
)

// roundTripFunc adapts a function into an http.RoundTripper for tests.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestSQLBypassTransport_RoundTrip(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		url       string
		wantQuery string // expected raw query seen by the base transport
	}{
		{"name filter", http.MethodGet, "http://x/api/storage/volumes?name=vol1", "SQL=false&name=vol1"},
		{"dotted name filter", http.MethodGet, "http://x/api/storage/qtrees?volume.name=v1&fields=id", "SQL=false&fields=id&volume.name=v1"},
		{"wildcard name", http.MethodGet, "http://x/api/storage/luns?name=*", "SQL=false&name=%2A"},
		{"uuid path only", http.MethodGet, "http://x/api/storage/volumes/abc?fields=name", "fields=name"},
		{"fields value is not a filter", http.MethodGet, "http://x/api/storage/volumes?fields=name", "fields=name"},
		{"no query", http.MethodGet, "http://x/api/cluster", ""},
		{"existing SQL kept", http.MethodGet, "http://x/api/storage/volumes?name=v&SQL=true", "name=v&SQL=true"},
		{"POST untouched", http.MethodPost, "http://x/api/storage/volumes?name=v", "name=v"},
		{"PATCH untouched", http.MethodPatch, "http://x/api/storage/volumes?name=v", "name=v"},
		{"DELETE untouched", http.MethodDelete, "http://x/api/storage/volumes?name=v", "name=v"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var seen *http.Request
			base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
				seen = r
				return &http.Response{
					StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header),
				}, nil
			})

			req, err := http.NewRequest(tc.method, tc.url, nil)
			require.NoError(t, err)
			origQuery := req.URL.RawQuery

			resp, err := NewSQLBypassTransport(base).RoundTrip(req)
			require.NoError(t, err)
			_ = resp.Body.Close()

			require.NotNil(t, seen)
			assert.Equal(t, tc.wantQuery, seen.URL.RawQuery)
			assert.Equal(t, origQuery, req.URL.RawQuery, "caller's request must not be mutated")
		})
	}
}

func TestHasNameFilter(t *testing.T) {
	assert.True(t, hasNameFilter(map[string][]string{"name": {"a"}}))
	assert.True(t, hasNameFilter(map[string][]string{"svm.name": {"a"}}))
	assert.False(t, hasNameFilter(map[string][]string{"fields": {"name"}}))
	assert.False(t, hasNameFilter(map[string][]string{"username": {"a"}}))
	assert.False(t, hasNameFilter(nil))
}

// TestRestClient_SQLBypass verifies the option end to end through the REST client: name-filtered GETs carry
// SQL=false when enabled, UUID-keyed GETs never do, and nothing changes when the option is off.
func TestRestClient_SQLBypass(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
			var mu sync.Mutex
			queries := map[string]string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				queries[r.URL.Path] = r.URL.RawQuery
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"records":[],"num_records":0}`))
			}))
			defer server.Close()

			rs, err := NewRestClient(ctx, ClientConfig{
				ManagementLIF:                  server.Listener.Addr().String(),
				Username:                       "u",
				Password:                       "p",
				SQLBypass:                      enabled,
				unitTestTransportConfigSchemes: "http",
			}, "svm0", "ontap-nas")
			require.NoError(t, err)
			rs.svmUUID = "1234"

			_, _ = rs.LunList(ctx, "lun1", []string{"name"})
			_, _ = rs.SnapshotGet(ctx, "voluuid", "snapuuid")

			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, enabled, strings.Contains(queries["/api/storage/luns"], "SQL=false"))
			assert.Contains(t, queries["/api/storage/luns"], "name=lun1")
			assert.NotContains(t, queries["/api/storage/volumes/voluuid/snapshots/snapuuid"], "SQL")
		})
	}
}

func TestClientConfigFromOntapConfig_SQLBypass(t *testing.T) {
	tr, fl := true, false
	tests := []struct {
		name string
		val  *bool
		want bool
	}{
		{"nil", nil, false},
		{"true", &tr, true},
		{"false", &fl, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := clientConfigFromOntapConfig(&drivers.OntapStorageDriverConfig{
				CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{},
				UseRESTSQLBypass:          tc.val,
			}, 0)
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.SQLBypass)
		})
	}
}
