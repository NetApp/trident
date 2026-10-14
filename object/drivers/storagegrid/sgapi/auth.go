// Copyright 2026 NetApp, Inc. All Rights Reserved.

package sgapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// tokenRefreshMargin is how long before expiry a cached token is renewed.
const tokenRefreshMargin = 60 * time.Second

// credentials is the POST /api/v4/authorize request body.
type credentials struct {
	// AccountID is what makes this a tenant login rather than a grid
	// administrator login, and it must serialize as a string: the account ID is
	// 20 digits, which exceeds the range of a 64-bit integer, so an unquoted
	// value is silently rounded and authentication fails with an opaque 401.
	AccountID string `json:"accountId"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

// Authorize exchanges the tenant credentials for a bearer token and caches it
// along with the expiry the grid reports.
func (c *Client) Authorize(ctx context.Context) (string, error) {
	if strings.TrimSpace(c.cfg.AccountID) == "" {
		// A request without accountId authenticates against the grid
		// administrator realm, where a tenant user does not exist, so the grid
		// answers 401 even for credentials that work in the Tenant Manager UI.
		return "", &ValidationError{
			Field:  "account ID",
			Reason: "is required for a tenant login; it is the 20-digit tenant account ID, not the tenant name",
		}
	}
	if c.cfg.Password == "" {
		return "", &ValidationError{Field: "password", Reason: "is required to authorize"}
	}

	username := c.cfg.Username
	if username == "" {
		username = DefaultUsername
	}

	resp, err := c.do(ctx, request{
		method: http.MethodPost,
		path:   "/authorize",
		noAuth: true,
		body: credentials{
			AccountID: c.cfg.AccountID,
			Username:  username,
			Password:  c.cfg.Password,
		},
	})
	if err != nil {
		if IsBadRequest(err) {
			// A 400 here is configuration, not credentials: the grid is
			// refusing password authentication.
			return "", fmt.Errorf(
				"single sign-on appears to be enabled on this grid, so password authentication is refused: %w", err)
		}
		return "", err
	}

	// The token is returned as a bare string in the envelope's data field.
	var token string
	if err := decodeData(resp, &token); err != nil {
		return "", fmt.Errorf("authorize: %w", err)
	}
	if token == "" {
		return "", errors.New("authorize: grid returned an empty token")
	}

	expiry := parseTokenExpiry(resp.Header, time.Now())
	c.mu.Lock()
	c.token = token
	c.expiry = expiry
	c.static = false
	c.mu.Unlock()

	if !expiry.IsZero() {
		c.tracef("... token acquired, expires %s\n", expiry.UTC().Format(time.RFC3339))
	} else {
		c.tracef("... token acquired, grid reported no expiry\n")
	}
	return token, nil
}

// ReleaseToken invalidates the session on the grid. It is worth calling on a
// graceful exit but is not required.
func (c *Client) ReleaseToken(ctx context.Context) error {
	c.mu.Lock()
	token := c.token
	c.mu.Unlock()
	if token == "" {
		return nil
	}

	_, err := c.do(ctx, request{method: http.MethodDelete, path: "/authorize"})
	c.invalidateToken()
	if err != nil && !IsUnauthorized(err) {
		return err
	}
	return nil
}

// ensureToken returns a usable token, authorizing or refreshing as needed.
func (c *Client) ensureToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	token, expiry, static := c.token, c.expiry, c.static
	c.mu.Unlock()

	if token != "" {
		// A caller-supplied token cannot be refreshed, and a grid that reported
		// no expiry leaves nothing to pre-empt; in both cases rely on the 401
		// replay path instead.
		if static || expiry.IsZero() || time.Now().Add(tokenRefreshMargin).Before(expiry) {
			return token, nil
		}
		c.tracef("... cached token is within %s of expiry, re-authorizing\n", tokenRefreshMargin)
	}
	return c.Authorize(ctx)
}

func (c *Client) invalidateToken() {
	c.mu.Lock()
	c.token = ""
	c.expiry = time.Time{}
	c.mu.Unlock()
}

// canReauthorize reports whether the client holds enough to obtain a fresh
// token. A caller-supplied token with no password cannot be renewed.
func (c *Client) canReauthorize() bool {
	c.mu.Lock()
	static := c.static
	c.mu.Unlock()
	return !static && c.cfg.Password != "" && strings.TrimSpace(c.cfg.AccountID) != ""
}

var maxAgePattern = regexp.MustCompile(`(?i)\bmax-age\s*=\s*(\d+)`)

// parseTokenExpiry reads the session lifetime from the response headers, which
// the grid reports in Cache-Control and Expires. A zero time means the grid said
// nothing useful.
func parseTokenExpiry(header http.Header, now time.Time) time.Time {
	if match := maxAgePattern.FindStringSubmatch(header.Get("Cache-Control")); match != nil {
		if seconds, err := strconv.Atoi(match[1]); err == nil && seconds > 0 {
			return now.Add(time.Duration(seconds) * time.Second)
		}
	}
	if expires := header.Get("Expires"); expires != "" {
		if t, err := http.ParseTime(expires); err == nil {
			return t
		}
	}
	return time.Time{}
}
