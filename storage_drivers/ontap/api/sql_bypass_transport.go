// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"net/http"
	"net/url"
	"strings"
)

const (
	// sqlBypassQueryKey is the ONTAP REST query parameter that bypasses the SQL-backed name lookup.
	sqlBypassQueryKey = "SQL"
	// sqlBypassQueryValue is the value sent with sqlBypassQueryKey.
	sqlBypassQueryValue = "false"
)

// SQLBypassTransport adds SQL=false to ONTAP REST GET requests that filter by name,
// so ONTAP skips its SQL-backed name lookup.
type SQLBypassTransport struct {
	base http.RoundTripper
}

// NewSQLBypassTransport wraps base so name-filtered GET requests carry SQL=false.
func NewSQLBypassTransport(base http.RoundTripper) *SQLBypassTransport {
	return &SQLBypassTransport{base: base}
}

// RoundTrip clones the request (per RoundTripper semantics) and adds SQL=false when the
// request is a GET with a name filter and does not already specify SQL.
func (t *SQLBypassTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet || req.URL == nil {
		return t.base.RoundTrip(req)
	}

	query := req.URL.Query()
	if _, ok := query[sqlBypassQueryKey]; ok || !hasNameFilter(query) {
		return t.base.RoundTrip(req)
	}

	query.Set(sqlBypassQueryKey, sqlBypassQueryValue)
	r := req.Clone(req.Context())
	u := *req.URL
	u.RawQuery = query.Encode()
	r.URL = &u
	return t.base.RoundTrip(r)
}

// hasNameFilter reports whether the query filters on a name field, such as
// "name" or "volume.name".
func hasNameFilter(query url.Values) bool {
	for key := range query {
		if key == "name" || strings.HasSuffix(key, ".name") {
			return true
		}
	}
	return false
}
