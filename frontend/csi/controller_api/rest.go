// Copyright 2022 NetApp, Inc. All Rights Reserved.

package controllerAPI

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils"
)

const HTTPClientTimeout = time.Second * 30

type ControllerRestClient struct {
	url        string
	httpClient http.Client
}

func CreateTLSRestClient(url, caFile, certFile, keyFile string) (TridentController, error) {
	tlsConfig := &tls.Config{MinVersion: config.MinClientTLSVersion}
	if "" != caFile {
		caCert, err := os.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
		tlsConfig.ServerName = config.ServerCertName
	} else {
		tlsConfig.InsecureSkipVerify = true
	}
	if "" != certFile && "" != keyFile {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, err
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	return &ControllerRestClient{
		url: url,
		httpClient: http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsConfig,
			},
			Timeout: HTTPClientTimeout,
		},
	}, nil
}

// InvokeAPI makes a REST call to the CSI Controller REST endpoint. The body must be a marshaled JSON byte array (
// or nil). The method is the HTTP verb (i.e. GET, POST, ...).  The resource path is appended to the base URL to
// identify the desired server resource; it should start with '/'.
func (c *ControllerRestClient) InvokeAPI(
	ctx context.Context, requestBody []byte, method, resourcePath string, redactRequestBody,
	redactResponseBody bool,
) (*http.Response, []byte, error) {
	// Build URL
	url := c.url + resourcePath

	var request *http.Request
	var err error
	var prettyRequestBuffer bytes.Buffer
	var prettyResponseBuffer bytes.Buffer

	// Create the request
	if requestBody == nil {
		request, err = http.NewRequestWithContext(ctx, method, url, nil)
	} else {
		request, err = http.NewRequestWithContext(ctx, method, url, bytes.NewBuffer(requestBody))
	}
	if err != nil {
		return nil, nil, err
	}

	request.Header.Set("X-Request-ID", fmt.Sprint(ctx.Value(ContextKeyRequestID)))
	request.Header.Set("Content-Type", "application/json")

	// Log the request
	if requestBody != nil {
		if err = json.Indent(&prettyRequestBuffer, requestBody, "", "  "); err != nil {
			return nil, nil, fmt.Errorf("error formating request body; %v", err)
		}
	}

	utils.LogHTTPRequest(request, prettyRequestBuffer.Bytes(), "", redactRequestBody, false)

	response, err := c.httpClient.Do(request)
	if err != nil {
		err = fmt.Errorf("error communicating with Trident CSI Controller; %v", err)
		return nil, nil, err
	}
	defer func(Body io.ReadCloser) { _ = Body.Close() }(response.Body)

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("error reading response body; %v", err)
	}

	if responseBody != nil {
		if err = json.Indent(&prettyResponseBuffer, responseBody, "", "  "); err != nil {
			return nil, nil, fmt.Errorf("error formating response body; %v", err)
		}
	}
	utils.LogHTTPResponse(ctx, response, prettyResponseBuffer.Bytes(), "", redactResponseBody, false)

	return response, responseBody, err
}

type CreateNodeResponse struct {
	TopologyLabels map[string]string `json:"topologyLabels"`
}

// CreateNode registers the node with the CSI controller server
func (c *ControllerRestClient) CreateNode(ctx context.Context, node *utils.Node) (CreateNodeResponse, error) {
	nodeData, err := json.MarshalIndent(node, "", " ")
	if err != nil {
		return CreateNodeResponse{}, fmt.Errorf("error parsing create node request; %v", err)
	}

	createRequest := func() (*http.Response, []byte, error) {
		resp, respBody, err := c.InvokeAPI(ctx, nodeData, "PUT", config.NodeURL+"/"+node.Name, false, false)
		if err != nil {
			return resp, respBody, fmt.Errorf("could not log into the Trident CSI Controller: %v", err)
		}
		return resp, respBody, nil
	}

	resp, respBody, err := c.requestAndRetry(ctx, createRequest)
	if err != nil {
		return CreateNodeResponse{}, fmt.Errorf("failed during retry for CreateNode: %v", err)
	}

	createResponse := CreateNodeResponse{}
	if err := json.Unmarshal(respBody, &createResponse); err != nil {
		return createResponse, fmt.Errorf("could not parse node : %s; %v", string(respBody), err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return createResponse, fmt.Errorf("could not add CSI node")
	}
	return createResponse, nil
}

type GetNodeResponse struct {
	Node  *utils.NodeExternal `json:"node"`
	Error string              `json:"error,omitempty"`
}

func (c *ControllerRestClient) GetNode(ctx context.Context, nodeName string) (*utils.NodeExternal, error) {
	url := config.NodeURL + "/" + nodeName
	getRequest := func() (*http.Response, []byte, error) {
		resp, body, err := c.InvokeAPI(ctx, nil, "GET", url, false, false)
		if err != nil {
			return resp, body, fmt.Errorf("could not communicate with the Trident CSI Controller: %v", err)
		}
		return resp, body, nil
	}

	// Make the API call to update the node.
	resp, respBody, err := c.requestAndRetry(ctx, getRequest)
	if err != nil {
		return nil, fmt.Errorf("failed during retry for GetNode: %v", err)
	}

	getNodeResponse := GetNodeResponse{}
	if err := json.Unmarshal(respBody, &getNodeResponse); err != nil {
		return nil, fmt.Errorf("could not parse node info : %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := "could not get node info"
		Logc(ctx).WithError(fmt.Errorf(getNodeResponse.Error)).Error(msg)
		return nil, fmt.Errorf(msg)
	}

	return getNodeResponse.Node, nil
}

type UpdateNodeResponse struct {
	Name  string `json:"name"`
	Error string `json:"error,omitempty"`
}

func (c *ControllerRestClient) UpdateNode(
	ctx context.Context, nodeName string, nodeState *utils.NodePublicationStateFlags,
) error {
	body, err := json.Marshal(nodeState)
	if err != nil {
		return fmt.Errorf("error marshalling update node request; %v", err)
	}

	url := fmt.Sprintf("%s/%s/%s", config.NodeURL, nodeName, "publicationState")
	updateRequest := func() (*http.Response, []byte, error) {
		resp, _, err := c.InvokeAPI(ctx, body, "PUT", url, false, false)
		if err != nil {
			return resp, body, fmt.Errorf("could not communicate with the Trident CSI Controller: %v", err)
		}
		return resp, body, nil
	}

	// Make the API call to update the node.
	resp, respBody, err := c.requestAndRetry(ctx, updateRequest)
	if err != nil {
		return fmt.Errorf("failed during retry for UpdateNode: %v", err)
	}

	updateNodeResponse := UpdateNodeResponse{}
	if err := json.Unmarshal(respBody, &updateNodeResponse); err != nil {
		return fmt.Errorf("could not parse node info : %v", err)
	}

	if resp.StatusCode != http.StatusAccepted {
		msg := "could not update node"
		Logc(ctx).WithError(fmt.Errorf(updateNodeResponse.Error)).Error(msg)
		return fmt.Errorf(msg)
	}

	return nil
}

type ListNodesResponse struct {
	Nodes []string `json:"nodes"`
	Error string   `json:"error,omitempty"`
}

// GetNodes returns a list of nodes registered with the controller
func (c *ControllerRestClient) GetNodes(ctx context.Context) ([]string, error) {
	resp, respBody, err := c.InvokeAPI(ctx, nil, "GET", config.NodeURL, false, false)
	if err != nil {
		return nil, fmt.Errorf("could not log into the Trident CSI Controller: %v", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("could not list the CSI nodes")
	}

	// Parse JSON data
	respData := ListNodesResponse{}
	if err := json.Unmarshal(respBody, &respData); err != nil {
		return nil, fmt.Errorf("could not parse node list: %s; %v", string(respBody), err)
	}

	return respData.Nodes, nil
}

// DeleteNode deregisters the node with the CSI controller server
func (c *ControllerRestClient) DeleteNode(ctx context.Context, nodeName string) error {
	resp, _, err := c.InvokeAPI(ctx, nil, "DELETE", config.NodeURL+"/"+nodeName, false, false)
	if err != nil {
		return fmt.Errorf("could not log into the Trident CSI Controller: %v", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNoContent:
	case http.StatusUnprocessableEntity:
	case http.StatusNotFound:
	case http.StatusGone:
		break
	default:
		return fmt.Errorf("could not delete the node")
	}
	return nil
}

type GetCHAPResponse struct {
	CHAP  *utils.IscsiChapInfo `json:"chap"`
	Error string               `json:"error,omitempty"`
}

// GetChap requests the current CHAP credentials for a given volume/node pair from the Trident controller
func (c *ControllerRestClient) GetChap(ctx context.Context, volumeID, nodeName string) (*utils.IscsiChapInfo, error) {
	resp, respBody, err := c.InvokeAPI(ctx, nil, "GET", config.ChapURL+"/"+volumeID+"/"+nodeName, false, true)
	if err != nil {
		return &utils.IscsiChapInfo{}, fmt.Errorf("could not communicate with the Trident CSI Controller: %v", err)
	}
	getResponse := GetCHAPResponse{}
	if err := json.Unmarshal(respBody, &getResponse); err != nil {
		return nil, fmt.Errorf("could not parse CHAP info : %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := "could not get CHAP info"
		Logc(ctx).WithError(fmt.Errorf(getResponse.Error)).Error(msg)
		return nil, fmt.Errorf(msg)
	}
	return getResponse.CHAP, nil
}

type ListVolumePublicationsResponse struct {
	VolumePublications []*utils.VolumePublicationExternal `json:"volumePublications"`
	Error              string                             `json:"error,omitempty"`
}

// ListVolumePublicationsForNode requests volume publications that exist on the host node from Trident controller.
func (c *ControllerRestClient) ListVolumePublicationsForNode(
	ctx context.Context, nodeName string,
) ([]*utils.VolumePublicationExternal, error) {
	// Set up the resource path.
	url := fmt.Sprintf("%s/%s/%s", config.NodeURL, nodeName, "publication")

	// Invoke the controller API.
	resp, respBody, err := c.InvokeAPI(ctx, nil, "GET", url, false, false)
	if err != nil {
		return []*utils.VolumePublicationExternal{},
			fmt.Errorf("could not communicate with the Trident CSI Controller: %v", err)
	}

	listResponse := &ListVolumePublicationsResponse{}
	if err := json.Unmarshal(respBody, &listResponse); err != nil {
		return nil, fmt.Errorf("failed to parse response body: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		msg := "could not list publications"
		Logc(ctx).WithError(fmt.Errorf(listResponse.Error)).Error(msg)
		return nil, fmt.Errorf(msg)
	}

	return listResponse.VolumePublications, nil
}

// requestAndRetry is used when API requests may hit rate limiting. It inspects the headers and checks for
// 429 / TooManyRequests status. The caller is responsible for setting up the request function properly.
func (c *ControllerRestClient) requestAndRetry(
	ctx context.Context, req func() (*http.Response, []byte, error),
) (*http.Response, []byte, error) {
	defer func() {
		if panic := recover(); panic != nil {
			Logc(ctx).Debugf("Panic during retries; Could not parse HTTP response.")
		}
	}()

	res, body, err := req()
	for err == nil && (res != nil && res.StatusCode == http.StatusTooManyRequests) {
		Logc(ctx).Debugf("Request rejected due to rate limiting.")

		// Convert the Retry-After value to a time duration.
		retryAfter, err := time.ParseDuration(res.Header.Get("Retry-After"))
		if err != nil {
			return res, body, fmt.Errorf("could not parse response header: %v; %v", res.Header, err)
		}

		// Sleep for n-seconds supplied from Retry-After.
		Logc(ctx).Debugf("Sleeping for %s seconds to retry operation...", retryAfter)
		time.Sleep(time.Duration(retryAfter.Seconds()))

		// Try to make the update call again.
		res, body, err = req()
	}

	return res, body, err
}

func (c *ControllerRestClient) UpdateVolumeLUKSPassphraseNames(
	ctx context.Context, volumeName string, passphraseNames []string,
) error {
	operations := passphraseNames
	body, err := json.Marshal(operations)
	if err != nil {
		return fmt.Errorf("could not marshal JSON; %v", err)
	}
	url := config.VolumeURL + "/" + volumeName + "/luksPassphraseNames"
	resp, _, err := c.InvokeAPI(ctx, body, "PUT", url, false, false)
	if err != nil {
		return fmt.Errorf("could not log into the Trident CSI Controller: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("could not update volume LUKS passphrase names")
	}
	return nil
}

/*TODO (bpresnel) Enable with rate-limiting later?
// GetLoggingConfig retrieves the current logging configuration for Trident.
func (c *ControllerRestClient) GetLoggingConfig(ctx context.Context) (string, string, string, error) {
	urlFmt := "%s/%s"

	logLevelUrl := fmt.Sprintf(urlFmt, config.LoggingConfigURL, "level")
	resp, respBody, err := c.InvokeAPI(ctx, nil, "GET", logLevelUrl, false, false)
	if err != nil || resp.StatusCode != http.StatusOK {
		Logc(ctx).WithError(err).Error("Error getting controller's log level.")
		return "", "", "", err
	}
	logLevelResp := &common.GetLogLevelResponse{}
	if err = json.Unmarshal(respBody, logLevelResp); err != nil {
		Logc(ctx).WithError(err).Error("Error getting unmarshalling GetLogLevel response.")
		return "", "", "", err
	}

	logWorkflowsUrl := fmt.Sprintf(urlFmt, config.LoggingConfigURL, "workflows")
	resp, respBody, err = c.InvokeAPI(ctx, nil, "GET", logWorkflowsUrl, false, false)
	if err != nil || resp.StatusCode != http.StatusOK {
		Logc(ctx).WithError(err).Error("Error getting controller's selected logging workflows.")
		return "", "", "", err
	}
	getWorkflowsResp := &common.GetLoggingWorkflowsResponse{}
	if err = json.Unmarshal(respBody, getWorkflowsResp); err != nil {
		Logc(ctx).WithError(err).Error("Error getting unmarshalling GetSelectedLoggingWorkflows response.")
		return "", "", "", err
	}

	logLayersUrl := fmt.Sprintf(urlFmt, config.LoggingConfigURL, "layers")
	resp, respBody, err = c.InvokeAPI(ctx, nil, "GET", logLayersUrl, false, false)
	if err != nil || resp.StatusCode != http.StatusOK {
		Logc(ctx).WithError(err).Error("Error getting controller's selected logging layers.")
		return "", "", "", err
	}
	getLayersResp := &common.GetLoggingLayersResponse{}
	if err = json.Unmarshal(respBody, getLayersResp); err != nil {
		Logc(ctx).WithError(err).Error("Error getting unmarshalling GetSelectedLoggingLayers response.")
		return "", "", "", err
	}

	return logLevelResp.LogLevel, getWorkflowsResp.LogWorkflows, getLayersResp.LogLayers, nil
}
*/
