package api

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

func InvokeRestApi(method string, url string, requestBody []byte, debug bool) (*http.Response, []byte, error) {

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
		LogHttpRequest(request, requestBody)
	}

	client := &http.Client{}
	response, err := client.Do(request)

	responseBody := []byte{}
	if err == nil {
		if response.ContentLength > 0 {
			responseBody, _ = ioutil.ReadAll(response.Body)
			response.Body.Close()
		}
		if debug {
			LogHttpResponse(response, responseBody)
		}
	}

	return response, responseBody, err
}

func LogHttpRequest(request *http.Request, requestBody []byte) {
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

func LogHttpResponse(response *http.Response, responseBody []byte) {
	fmt.Fprintf(os.Stdout, "Response status: %s\n", response.Status)
	fmt.Fprintf(os.Stdout, "Response headers: %v\n", response.Header)
	if responseBody == nil {
		responseBody = []byte{}
	}
	fmt.Fprintf(os.Stdout, "Response body: %s\n", string(responseBody))
	fmt.Fprint(os.Stdout, "================================================================================\n")
}
