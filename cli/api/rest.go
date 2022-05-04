// Copyright 2018 NetApp, Inc. All Rights Reserved.

package api

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

const HTTPClientTimeout = time.Second * 300

func InvokeRESTAPI(method, url string, requestBody []byte, debug bool) (*http.Response, []byte, error) {
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

	if debug {
		LogHTTPRequest(request, requestBody)
	}

	client := &http.Client{Timeout: HTTPClientTimeout}
	response, err := client.Do(request)
	if err != nil {
		err = fmt.Errorf("error communicating with Trident REST API; %v", err)
		return nil, nil, err
	}

	var responseBody []byte

	if response != nil {
		defer response.Body.Close()
		responseBody, err = ioutil.ReadAll(response.Body)
		if err != nil {
			return response, responseBody, fmt.Errorf("error reading response body; %v", err)
		}
	}

	if debug {
		LogHTTPResponse(response, responseBody)
	}

	return response, responseBody, err
}

func LogHTTPRequest(request *http.Request, requestBody []byte) {
	fmt.Fprint(os.Stdout, "--------------------------------------------------------------------------------\n")
	fmt.Fprintf(os.Stdout, "Request Method: %s\n", request.Method)
	fmt.Fprintf(os.Stdout, "Request URL: %v\n", request.URL)
	fmt.Fprintf(os.Stdout, "Request headers: %v\n", request.Header)
	if requestBody == nil {
		requestBody = []byte{}
	}
	fmt.Fprintf(os.Stdout, "Request body: %s\n", string(requestBody))
	fmt.Fprint(os.Stdout, "................................................................................\n")
}

func LogHTTPResponse(response *http.Response, responseBody []byte) {
	if response != nil {
		fmt.Fprintf(os.Stdout, "Response status: %s\n", response.Status)
		fmt.Fprintf(os.Stdout, "Response headers: %v\n", response.Header)
	}

	if responseBody != nil {
		fmt.Fprintf(os.Stdout, "Response body: %s\n", string(responseBody))
	}

	fmt.Fprint(os.Stdout, "================================================================================\n")
}
