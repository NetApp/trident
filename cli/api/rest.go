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

const HTTPTimeout = time.Second * 30

func InvokeRESTAPI(method string, url string, requestBody []byte, debug bool) (*http.Response, []byte, error) {

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

	client := &http.Client{Timeout: HTTPTimeout}
	response, err := client.Do(request)

	responseBody := []byte{}
	if err == nil {

		responseBody, err = ioutil.ReadAll(response.Body)
		response.Body.Close()

		if debug {
			LogHTTPResponse(response, responseBody)
		}
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
	fmt.Fprintf(os.Stdout, "Response status: %s\n", response.Status)
	fmt.Fprintf(os.Stdout, "Response headers: %v\n", response.Header)
	if responseBody == nil {
		responseBody = []byte{}
	}
	fmt.Fprintf(os.Stdout, "Response body: %s\n", string(responseBody))
	fmt.Fprint(os.Stdout, "================================================================================\n")
}
