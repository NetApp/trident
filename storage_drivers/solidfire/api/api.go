// Copyright 2016 NetApp, Inc. All Rights Reserved.

package api

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/rand"
	"net/http"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"

	"github.com/netapp/trident/utils"
)

const httpTimeoutSeconds = 10
const httpContentType = "json-rpc"

// Client is used to send API requests to a SolidFire system system
type Client struct {
	SVIP              string
	Endpoint          string
	DefaultAPIPort    int
	DefaultAccountID  int64
	DefaultTenantName string
	VolumeTypes       *[]VolType
	Config            *Config
	AccessGroups      []int64
	DefaultBlockSize  int64
	DebugTraceFlags   map[string]bool
}

// Config holds the configuration data for the Client to communicate with a SolidFire storage system
type Config struct {
	TenantName       string
	EndPoint         string
	MountPoint       string
	SVIP             string
	InitiatorIFace   string //iface to use of iSCSI initiator
	Types            *[]VolType
	LegacyNamePrefix string
	AccessGroups     []int64
	DefaultBlockSize int64
	DebugTraceFlags  map[string]bool
}

// VolType holds quality of service configuration data
type VolType struct {
	Type string
	QOS  QoS
}

// NewFromParameters is a factory method to create a new sfapi.Client object using the supplied parameters
func NewFromParameters(pendpoint string, psvip string, pcfg Config, pdefaultTenantName string) (c *Client, err error) {
	rand.Seed(time.Now().UTC().UnixNano())
	SFClient := &Client{
		Endpoint:          pendpoint,
		SVIP:              psvip,
		Config:            &pcfg,
		DefaultAPIPort:    443,
		VolumeTypes:       pcfg.Types,
		DefaultTenantName: pdefaultTenantName,
		DefaultBlockSize:  pcfg.DefaultBlockSize,
		DebugTraceFlags:   pcfg.DebugTraceFlags,
	}
	return SFClient, nil
}

// Request performs a json-rpc POST to the configured endpoint
func (c *Client) Request(method string, params interface{}, id int) ([]byte, error) {

	var err error
	var request *http.Request
	var response *http.Response
	var prettyJSON bytes.Buffer

	if c.Endpoint == "" {
		log.Error("endpoint is not set, unable to issue json-rpc requests")
		err = errors.New("no endpoint set")
		return nil, err
	}

	requestBody, err := json.Marshal(map[string]interface{}{
		"method": method,
		"id":     id,
		"params": params,
	})

	// Create the request
	request, err = http.NewRequest("POST", c.Endpoint, strings.NewReader(string(requestBody)))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", httpContentType)

	// Log the request
	if c.Config.DebugTraceFlags["api"] {
		json.Indent(&prettyJSON, requestBody, "", "  ")
		utils.LogHttpRequest(request, prettyJSON.Bytes())
	}

	// Send the request
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{
		Transport: tr,
		Timeout:   time.Duration(httpTimeoutSeconds * time.Second),
	}
	response, err = httpClient.Do(request)
	if err != nil {
		log.Errorf("error response from SolidFire API request: %v", err)
		return nil, errors.New("device API error")
	}

	// Handle HTTP errors such as 401 (Unauthorized)
	httpError := utils.NewHTTPError(response)
	if httpError != nil {
		log.WithFields(log.Fields{
			"request":        method,
			"responseCode":   response.StatusCode,
			"responseStatus": response.Status,
		}).Errorf("API request failed.")
		return nil, *httpError
	}

	defer response.Body.Close()
	responseBody, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return responseBody, err
	}

	// Log the response
	if c.Config.DebugTraceFlags["api"] {
		if c.shouldLogResponseBody(method) {
			json.Indent(&prettyJSON, responseBody, "", "  ")
			utils.LogHttpResponse(response, prettyJSON.Bytes())

		} else {
			utils.LogHttpResponse(response, []byte("<suppressed>"))
		}
	}

	// Look for any errors returned from the controller
	apiError := APIError{}
	json.Unmarshal([]byte(responseBody), &apiError)
	if apiError.Fields.Code != 0 {
		log.WithFields(log.Fields{
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
		return c.Config.DebugTraceFlags["sensitive"]
	case "GetClusterHardwareInfo":
		return c.Config.DebugTraceFlags["hardwareInfo"]
	default:
		return true
	}
}

// NewReqID generates a random id for a request
func NewReqID() int {
	return rand.Intn(1000-1) + 1
}
