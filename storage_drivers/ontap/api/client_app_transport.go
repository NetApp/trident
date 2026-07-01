// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"net/http"

	tridentconfig "github.com/netapp/trident/config"
)

// ClientAppHeader is the HTTP header name ONTAP uses to identify the calling
// application. ONTAP exposes it via `security session request-statistics
// show-by-application`, allowing admins to track and rate-limit per-app usage.
const ClientAppHeader = "X-Dot-Client-App"

// ClientAppHeaderValue returns the value to send in the X-Dot-Client-App header
// for every REST/ZAPI request Trident issues to ONTAP, e.g. "Trident/26.06.1".
func ClientAppHeaderValue() string {
	return "Trident/" + tridentconfig.OrchestratorVersion.ShortString()
}

// ClientAppTransport is an http.RoundTripper that stamps the X-Dot-Client-App
// header onto every outgoing request before delegating to a base transport.
type ClientAppTransport struct {
	base  http.RoundTripper
	value string
}

// NewClientAppTransport wraps base so every request it sends carries the
// X-Dot-Client-App header set to value.
func NewClientAppTransport(base http.RoundTripper, value string) *ClientAppTransport {
	return &ClientAppTransport{base: base, value: value}
}

// RoundTrip clones the request (per RoundTripper semantics) and sets the
// X-Dot-Client-App header before handing off to the base transport.
func (t *ClientAppTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.Header.Set(ClientAppHeader, t.value)
	return t.base.RoundTrip(r)
}
