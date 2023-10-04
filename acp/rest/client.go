// Copyright 2023 NetApp, Inc. All Rights Reserved.

package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/version"
)

// Client is a concrete reference for interfacing directly with ACP HTTP APIs.
type Client struct {
	baseURL    string
	httpClient http.Client
}

// NewClient accepts a base URL and timeout.
// It returns a reference for interfacing directly with Trident-ACP HTTP REST APIs.
func NewClient(url string, timeout time.Duration) *Client {
	return &Client{baseURL: url, httpClient: http.Client{Timeout: timeout}}
}

type getVersionResponse struct {
	Version string `json:"version"`
	Error   string `json:"error,omitempty"`
}

// GetVersion gets the installed ACP version.
// Example: http://<host>:<port>/trident-acp/v1/version
func (c *Client) GetVersion(ctx context.Context) (*version.Version, error) {
	// Create a new HTTP request.
	url := c.baseURL + versionEndpoint
	req, err := c.newRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// Assign headers.
	req.Header.Set("User-Agent", userAgent())

	// Make the request.
	res, data, err := c.invokeAPI(req)
	if err != nil {
		return nil, err
	} else if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-ok status code: [%v]; status %v", res.StatusCode, res.Status)
	}

	// Parse the response data into a struct.
	var versionResponse getVersionResponse
	if err := json.Unmarshal(data, &versionResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body; %v", err)
	} else if versionResponse.Error != "" {
		return nil, fmt.Errorf("failed to get trident-acp version; %v", err)
	}

	return version.ParseDate(versionResponse.Version)
}

// Entitled accepts a feature and makes a request to ACP APIs to check if the supplied feature in Trident is allowed.
// Example: http://<host>:<port>/trident-acp/v1/entitled?feature=<feature>
func (c *Client) Entitled(ctx context.Context, feature string) error {
	// Create a new HTTP request.
	url := c.baseURL + entitledEndpoint
	req, err := c.newRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}

	// Assign headers.
	req.Header.Set("User-Agent", userAgent())

	// Add or Set query parameters.
	params := req.URL.Query()
	params.Set("feature", feature)
	req.URL.RawQuery = params.Encode()

	// Make the request.
	// TODO (websterj): Inspect the response body when the trident-acp API changes.
	res, _, err := c.invokeAPI(req)
	if err != nil {
		return err
	}

	switch res.StatusCode {
	case http.StatusForbidden:
		return errors.UnlicensedError("feature %s requires a valid ACP license", feature)
	case http.StatusOK:
		return nil
	}

	return fmt.Errorf("unexpected status code: [%s]", res.Status)
}

// newRequest accepts necessary fields to construct a new http request.
// It returns a new http request or an error if one occurs.
func (c *Client) newRequest(ctx context.Context, method, url string, data []byte) (*http.Request, error) {
	// Ideally, the context should never be empty. If it is, set it to the default value used in new requests.
	if ctx == nil {
		ctx = context.Background()
	}

	// Construct a new http request.
	var body io.Reader
	if data != nil {
		body = bytes.NewBuffer(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create new request: [%s, %s]; %v", method, url, err)
	}

	return req, nil
}

// invokeAPI accepts a http request, makes the request, and parses the response.
// It returns the http response, the response body or an error if one occurs.
func (c *Client) invokeAPI(req *http.Request) (*http.Response, []byte, error) {
	// Make the request.
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to invoke Trident-ACP REST API: [%s]; %v", req.URL.String(), err)
	}
	defer func() { _ = res.Body.Close() }()

	// Read the response body.
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return res, nil, fmt.Errorf("failed to parse response body; %v", err)
	}

	return res, data, nil
}
