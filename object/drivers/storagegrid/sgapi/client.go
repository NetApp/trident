// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package sgapi is a dependency-free client for the StorageGRID Tenant
// Management API (/api/v4/org/*).
//
// It covers the calls a COSI object-store driver needs for the bucket
// lifecycle (create, inspect, empty, delete) and for grant/revoke of S3
// access (groups, users, access keys). It uses only the standard library so
// the storage-grid driver can import it without extra dependencies.
package sgapi

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// APIBasePath is the version prefix shared by every endpoint. Both API 4.2 and
// 4.3 advertise major version 4, and no call sequence here depends on which of
// the two it runs against.
const APIBasePath = "/api/v4"

// DefaultTimeout bounds a single HTTP request.
const DefaultTimeout = 60 * time.Second

// DefaultUsername is the tenant user a grid always has.
const DefaultUsername = "root"

// Config describes how to reach one StorageGRID tenant.
type Config struct {
	// Endpoint is the admin node base URL. A bare host is assumed to be https.
	Endpoint string
	// AccountID is the 20-digit tenant account ID, not the tenant display name.
	AccountID string
	Username  string
	Password  string
	// Token reuses an existing bearer token instead of authorizing. Without a
	// password it cannot be refreshed, so a 401 becomes terminal.
	Token string
	// Insecure skips TLS verification, which admin nodes with the default
	// self-signed certificate require.
	Insecure bool
	// CACert is PEM data to trust instead of the system roots.
	CACert  []byte
	Timeout time.Duration
	// Debug, when set, receives a trace of every request and response with
	// credentials redacted.
	Debug io.Writer
}

// Client talks to one tenant's Tenant Management API. It is safe for concurrent
// use.
type Client struct {
	cfg     Config
	baseURL *url.URL
	http    *http.Client

	mu     sync.Mutex
	token  string
	expiry time.Time
	// static records that the token came from the caller, so there are no
	// credentials to re-authorize with.
	static bool
}

// New validates cfg and builds a client. It performs no network I/O.
func New(cfg Config) (*Client, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, &ValidationError{Field: "endpoint", Reason: "is required"}
	}

	raw := strings.TrimSpace(cfg.Endpoint)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	base, err := url.Parse(raw)
	if err != nil {
		return nil, &ValidationError{Field: "endpoint", Value: cfg.Endpoint, Reason: err.Error()}
	}
	if base.Host == "" {
		return nil, &ValidationError{Field: "endpoint", Value: cfg.Endpoint, Reason: "must include a host"}
	}
	base.Path = strings.TrimSuffix(base.Path, "/")
	// Only the scheme, host, and any path prefix are meaningful; a query or
	// fragment on the endpoint would corrupt every request built from it.
	base.RawQuery = ""
	base.Fragment = ""

	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: cfg.Insecure, // #nosec G402 -- opt-in via insecureSkipVerify; StorageGRID admin nodes ship a self-signed cert by default
	}
	if len(cfg.CACert) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(cfg.CACert) {
			return nil, &ValidationError{Field: "ca-cert", Reason: "contained no PEM certificates"}
		}
		tlsConfig.RootCAs = pool
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	c := &Client{
		cfg:     cfg,
		baseURL: base,
		http: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
				Proxy:           http.ProxyFromEnvironment,
			},
		},
	}
	if cfg.Token != "" {
		c.token = cfg.Token
		c.static = true
	}
	return c, nil
}

// Response is one completed exchange. Body is retained so that undocumented
// failures can be inspected.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	Envelope   *Envelope
}

type request struct {
	method string
	// path is relative to APIBasePath and must already be escaped.
	path   string
	body   any
	query  url.Values
	noAuth bool
}

// do performs one request without any retry handling.
func (c *Client) do(ctx context.Context, r request) (*Response, error) {
	endpoint := *c.baseURL
	// r.path arrives already escaped, so both forms have to be set: URL.String
	// writes RawPath when it is a valid encoding of Path, and assigning Path
	// alone would escape the percent signs a second time.
	escapedPath := c.baseURL.EscapedPath() + APIBasePath + r.path
	unescapedPath, err := url.PathUnescape(escapedPath)
	if err != nil {
		return nil, fmt.Errorf("%s %s: malformed path: %w", r.method, escapedPath, err)
	}
	endpoint.Path = unescapedPath
	endpoint.RawPath = escapedPath
	if len(r.query) > 0 {
		endpoint.RawQuery = r.query.Encode()
	}
	uri := endpoint.RequestURI()

	var payload []byte
	if r.body != nil {
		encoded, err := json.Marshal(r.body)
		if err != nil {
			return nil, fmt.Errorf("%s %s: encoding request body: %w", r.method, uri, err)
		}
		payload = encoded
	}

	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, r.method, endpoint.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", r.method, uri, err)
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if !r.noAuth {
		token, err := c.ensureToken(ctx)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	c.traceRequest(r.method, uri, payload)
	started := time.Now()
	httpResp, err := c.http.Do(req)
	if err != nil {
		c.tracef("<-- transport error after %s: %v\n", elapsed(started), err)
		return nil, fmt.Errorf("%s %s: %w", r.method, uri, err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s: reading response: %w", r.method, uri, err)
	}
	c.traceResponse(httpResp.StatusCode, body, started)

	resp := &Response{StatusCode: httpResp.StatusCode, Header: httpResp.Header, Body: body}
	if len(bytes.TrimSpace(body)) > 0 {
		var env Envelope
		if json.Unmarshal(body, &env) == nil {
			resp.Envelope = &env
		}
	}
	if resp.Envelope != nil && resp.Envelope.Deprecated {
		c.tracef("!!! %s %s reports deprecated=true\n", r.method, uri)
	}
	if httpResp.StatusCode >= http.StatusBadRequest {
		return resp, newAPIError(r.method, uri, resp)
	}
	return resp, nil
}

// doRetrying performs a request and, on a 401, treats the token as expired:
// it re-authorizes once and replays the call. A second 401 is returned as-is,
// since that indicates bad credentials rather than an expired token.
func (c *Client) doRetrying(ctx context.Context, r request) (*Response, error) {
	resp, err := c.do(ctx, r)
	if err == nil || r.noAuth {
		return resp, err
	}
	if !IsUnauthorized(err) || !c.canReauthorize() {
		return resp, err
	}

	c.tracef("... 401 received, re-authorizing once and replaying\n")
	c.invalidateToken()
	if _, authErr := c.Authorize(ctx); authErr != nil {
		return resp, authErr
	}
	return c.do(ctx, r)
}

// decodeData unmarshals the envelope's data field into out.
func decodeData(resp *Response, out any) error {
	if resp == nil || resp.Envelope == nil || len(resp.Envelope.Data) == 0 {
		return errors.New("response contained no data field")
	}
	if err := json.Unmarshal(resp.Envelope.Data, out); err != nil {
		return fmt.Errorf("decoding response data: %w", err)
	}
	return nil
}

// responseAlerts extracts alerts from a successful response.
func responseAlerts(resp *Response) []Alert {
	if resp == nil || resp.Envelope == nil {
		return nil
	}
	if resp.Envelope.Metadata != nil && len(resp.Envelope.Metadata.Alerts) > 0 {
		return resp.Envelope.Metadata.Alerts
	}
	// The schema places metadata alongside data, but tolerate it nested inside.
	var nested struct {
		Metadata *Metadata `json:"metadata"`
	}
	if len(resp.Envelope.Data) > 0 &&
		json.Unmarshal(resp.Envelope.Data, &nested) == nil &&
		nested.Metadata != nil {
		return nested.Metadata.Alerts
	}
	return nil
}

// escapePath joins and escapes path segments. Bucket names and short names can
// contain characters that must not be taken as path structure.
func escapePath(segments ...string) string {
	escaped := make([]string, 0, len(segments))
	for _, s := range segments {
		escaped = append(escaped, url.PathEscape(s))
	}
	return "/" + strings.Join(escaped, "/")
}

// Versions reports the major API versions the grid supports. It needs no
// authentication and is the cheapest connectivity check, so confirming that 4 is
// among them turns a future incompatibility into a clear error rather than a
// pile of 404s.
func (c *Client) Versions(ctx context.Context) ([]int, error) {
	resp, err := c.do(ctx, request{method: http.MethodGet, path: "/versions", noAuth: true})
	if err != nil {
		return nil, err
	}
	var versions []int
	if decodeData(resp, &versions) == nil {
		return versions, nil
	}
	var wrapped struct {
		Versions []int `json:"versions"`
	}
	if err := decodeData(resp, &wrapped); err != nil {
		return nil, fmt.Errorf("reading supported versions: %w", err)
	}
	return wrapped.Versions, nil
}

// redactPattern matches the credential fields that must never be traced.
var redactPattern = regexp.MustCompile(`("(?:password|secretAccessKey)"\s*:\s*)"[^"]*"`)

func redact(b []byte) []byte {
	return redactPattern.ReplaceAll(b, []byte(`${1}"[REDACTED]"`))
}

func elapsed(started time.Time) time.Duration {
	return time.Since(started).Round(time.Millisecond)
}

func (c *Client) tracef(format string, a ...any) {
	if c.cfg.Debug == nil {
		return
	}
	fmt.Fprintf(c.cfg.Debug, format, a...)
}

func (c *Client) traceRequest(method, uri string, payload []byte) {
	if c.cfg.Debug == nil {
		return
	}
	fmt.Fprintf(c.cfg.Debug, "--> %s %s\n", method, uri)
	if len(payload) > 0 {
		fmt.Fprintf(c.cfg.Debug, "    %s\n", redact(payload))
	}
}

func (c *Client) traceResponse(status int, body []byte, started time.Time) {
	if c.cfg.Debug == nil {
		return
	}
	fmt.Fprintf(c.cfg.Debug, "<-- %d %s (%s)\n", status, http.StatusText(status), elapsed(started))
	if len(bytes.TrimSpace(body)) > 0 {
		fmt.Fprintf(c.cfg.Debug, "    %s\n", redact(body))
	}
}
