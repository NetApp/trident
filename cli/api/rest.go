// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	. "github.com/netapp/trident/logging"
)

const HTTPClientTimeout = time.Second * 300

func InvokeRESTAPI(method, url string, requestBody []byte) (*http.Response, []byte, error) {
	var request *http.Request
	var err error

	if requestBody == nil {
		request, err = http.NewRequest(method, url, nil)
	} else {
		request, err = http.NewRequest(method, url, bytes.NewBuffer(requestBody))
	}
	if err != nil {
		return nil, nil, err
	}

	request.Header.Set("Content-Type", "application/json")

	LogHTTPRequest(request, requestBody)

	client := &http.Client{Timeout: HTTPClientTimeout}
	response, err := client.Do(request)
	if err != nil {
		err = fmt.Errorf("error communicating with Trident REST API; %v", err)
		return nil, nil, err
	}

	var responseBody []byte

	if response != nil {
		defer func() { _ = response.Body.Close() }()
		responseBody, err = io.ReadAll(response.Body)
		if err != nil {
			return response, responseBody, fmt.Errorf("error reading response body; %v", err)
		}
	}

	LogHTTPResponse(response, responseBody)

	return response, responseBody, err
}

func LogHTTPRequest(request *http.Request, requestBody []byte) {
	Log().Debug("--------------------------------------------------------------------------------\n")
	Log().Debugf("Request Method: %s\n", request.Method)
	Log().Debugf("Request URL: %v\n", RedactedURL(request.URL))
	Log().Debugf("Request headers: %v\n", RedactedHeaders(request.Header))
	if requestBody == nil {
		requestBody = []byte{}
	}
	// The body of `tridentctl create backend -d` is the whole backend config, and the pattern-based
	// redaction covers only the credential keys it was written for, so walk the decoded body instead.
	Log().Debugf("Request body: %s\n", RedactJSONBody(requestBody))
	Log().Debug("................................................................................\n")
}

func LogHTTPResponse(response *http.Response, responseBody []byte) {
	if response != nil {
		Log().Debugf("Response status: %s\n", response.Status)
		Log().Debugf("Response headers: %v\n", RedactedHeaders(response.Header))
	}

	if responseBody != nil {
		Log().Debugf("Response body: %s\n", RedactJSONBody(responseBody))
	}

	Log().Debug("================================================================================\n")
}
