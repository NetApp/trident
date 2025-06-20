// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/internal/crypto"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

const httpContentType = "json-rpc"

// Client is used to send API requests to a SolidFire system
type Client struct {
	SVIP             string
	Endpoint         string
	Config           *Config
	DefaultAPIPort   int
	VolumeTypes      *[]VolType
	AccessGroups     []int64
	DefaultBlockSize int64
	DebugTraceFlags  map[string]bool
	AccountID        int64
	httpClient       *http.Client
}

// Config holds the configuration data for the Client to communicate with a SolidFire storage system
type Config struct {
	TenantName           string
	EndPoint             string
	MountPoint           string
	SVIP                 string
	InitiatorIFace       string // iface to use of iSCSI initiator
	Types                *[]VolType
	LegacyNamePrefix     string
	AccessGroups         []int64
	DefaultBlockSize     int64
	DebugTraceFlags      map[string]bool
	TrustedCACertificate string
}

// VolType holds quality of service configuration data
type VolType struct {
	Type string
	QOS  QoS
}

// NewFromParameters is a factory method to create a new sfapi.Client object using the supplied parameters
func NewFromParameters(pendpoint, psvip string, pcfg Config) (c *Client, err error) {
	tcfg := tls.Config{MinVersion: tridentconfig.MinClientTLSVersion, InsecureSkipVerify: true}
	if pcfg.TrustedCACertificate != "" {
		caCert, err := base64.StdEncoding.DecodeString(pcfg.TrustedCACertificate)
		if err != nil {
			return nil, err
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to append CA certificate, certificate may be invalid or malformed")
		}
		tcfg.RootCAs = caCertPool
		tcfg.InsecureSkipVerify = false
	}
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tcfg,
		},
		Timeout: tridentconfig.StorageAPITimeoutSeconds * time.Second,
	}
	SFClient := &Client{
		Endpoint:         pendpoint,
		SVIP:             psvip,
		Config:           &pcfg,
		DefaultAPIPort:   443,
		VolumeTypes:      pcfg.Types,
		DefaultBlockSize: pcfg.DefaultBlockSize,
		DebugTraceFlags:  pcfg.DebugTraceFlags,
		httpClient:       httpClient,
	}
	return SFClient, nil
}

// Request performs a json-rpc POST to the configured endpoint
func (c *Client) Request(ctx context.Context, method string, params interface{}, id int) ([]byte, error) {
	var err error
	var request *http.Request
	var response *http.Response
	var prettyRequestBuffer bytes.Buffer
	var prettyResponseBuffer bytes.Buffer

	if c.Endpoint == "" {
		Logc(ctx).Error("endpoint is not set, unable to issue json-rpc requests")
		err = errors.New("no endpoint set")
		return nil, err
	}

	requestBody, err := json.Marshal(map[string]interface{}{
		"method": method,
		"id":     id,
		"params": params,
	})

	// Create the request
	request, err = http.NewRequestWithContext(ctx, "POST", c.Endpoint, strings.NewReader(string(requestBody)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", httpContentType)

	// Log the request
	if err := json.Indent(&prettyRequestBuffer, requestBody, "", "  "); err != nil {
		Logd(ctx, tridentconfig.SolidfireSANStorageDriverName, c.Config.DebugTraceFlags["api"]).
			Errorf("Could not format API request for logging; %v", err)
	}
	RedactedHTTPRequest(request, prettyRequestBuffer.Bytes(), LogLayerSolidfireDriver.String(), false,
		c.Config.DebugTraceFlags["api"])

	// Send the request
	response, err = c.httpClient.Do(request)
	if err != nil {
		Logc(ctx).Errorf("Error response from SolidFire API request: %v", err)
		return nil, errors.New("device API error")
	}

	// Handle HTTP errors such as 401 (Unauthorized)
	httpError := NewHTTPError(response)
	if httpError != nil {
		Logc(ctx).WithFields(LogFields{
			"request":        method,
			"responseCode":   response.StatusCode,
			"responseStatus": response.Status,
		}).Errorf("API request failed.")
		return nil, *httpError
	}

	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return responseBody, err
	}

	// Log the response
	if c.shouldLogResponseBody(method) {
		if err := json.Indent(&prettyResponseBuffer, responseBody, "", "  "); err != nil {
			Logd(ctx, tridentconfig.SolidfireSANStorageDriverName, c.Config.DebugTraceFlags["api"]).
				Errorf("Could not format API request for logging; %v", err)
		} else {
			RedactedHTTPResponse(ctx, response, prettyResponseBuffer.Bytes(), LogLayerSolidfireDriver.String(),
				false, c.Config.DebugTraceFlags["api"])
		}
	} else {
		RedactedHTTPResponse(ctx, response, []byte("<suppressed>"), LogLayerSolidfireDriver.String(), true,
			c.Config.DebugTraceFlags["api"])
	}

	// Look for any errors returned from the controller
	apiError := Error{}
	if err = json.Unmarshal(responseBody, &apiError); err != nil {
		Logc(ctx).Errorf("Could not format API request for logging; %v", err)
	} else {
		RedactedHTTPRequest(request, prettyRequestBuffer.Bytes(), "", false, false)
	}
	if apiError.Fields.Code != 0 {
		Logc(ctx).WithFields(LogFields{
			"ID":      apiError.ID,
			"code":    apiError.Fields.Code,
			"message": apiError.Fields.Message,
			"name":    apiError.Fields.Name,
		}).Error("Error detected in API response.")
		return nil, apiError
	}

	return responseBody, nil
}

// shouldLogResponseBody prevents logging the REST response body for APIs that are
// extremely lengthy for no good reason or that return sensitive data like iSCSI secrets.
func (c *Client) shouldLogResponseBody(method string) bool {
	switch method {
	case "GetAccountByName", "GetAccountByID", "ListAccounts":
		return false
	case "GetClusterHardwareInfo":
		return c.Config.DebugTraceFlags["hardwareInfo"]
	default:
		return true
	}
}

// NewReqID generates a random id for a request
func NewReqID() int {
	return crypto.RandomNumber(1000-1) + 1
}

type HTTPError struct {
	Status     string
	StatusCode int
}

func (e HTTPError) Error() string {
	return fmt.Sprintf("HTTP error: %s", e.Status)
}

func NewHTTPError(response *http.Response) *HTTPError {
	if response.StatusCode < 300 {
		return nil
	}
	return &HTTPError{response.Status, response.StatusCode}
}
