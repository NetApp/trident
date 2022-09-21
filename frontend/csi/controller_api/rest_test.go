// Copyright 2022 NetApp, Inc. All Rights Reserved.

package controllerAPI

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/utils"
)

var (
	ctx              = context.Background()
	tridentNodeTable = make(map[string]string)
	chap             = GetCHAPResponse{}
)

func TestMain(m *testing.M) {
	log.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}

func getHttpServer(url string, mockFunction func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == url {
			mockFunction(w, r)
		} else {
			sc := http.StatusBadRequest
			createResponse(w, "", sc)
		}
	}))
}

func mockCreateNode(w http.ResponseWriter, r *http.Request) {
	s := strings.TrimSpace(r.URL.Path)
	str := strings.Split(s, "/")
	name := str[4]
	sc := http.StatusCreated
	if name == "" {
		sc = http.StatusBadRequest
	}
	if name == "invalidResponse" {
		msg := "invalidResponse"
		createResponse(w, msg, sc)
	} else {
		indexNumber := len(tridentNodeTable) + 1
		nodeId := strconv.Itoa(indexNumber)
		tridentNodeTable[nodeId] = name
		msg := ListNodesResponse{}
		nodes := append(msg.Nodes, name)
		msg.Nodes = nodes
		createResponse(w, msg, sc)
	}
}

func mockGetNodeResponse(w http.ResponseWriter, r *http.Request) {
	ids, ok := r.URL.Query()["id"]
	sc := http.StatusOK

	msg := ListNodesResponse{}
	if !ok || len(ids[0]) == 0 {
		values := make([]string, 0, len(tridentNodeTable))
		for _, v := range tridentNodeTable {
			updatedNodes := append(msg.Nodes, v)
			msg.Nodes = updatedNodes
			values = append(values, v)
		}
	} else {
		for i := 0; i < len(ids); i++ {
			updatedNodes := append(msg.Nodes, tridentNodeTable[ids[i]])
			msg.Nodes = updatedNodes
		}
	}

	createResponse(w, msg, sc)
}

func mockResourceNotFound(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusNotFound
	createResponse(w, "", sc)
}

func mockDeleteNode(w http.ResponseWriter, r *http.Request) {
	str := strings.Split(strings.TrimSpace(r.URL.Path), "/")
	name := str[4]

	if name == "StatusUnprocessableEntity" {
		createResponse(w, "", http.StatusUnprocessableEntity)
	} else if name == "StatusNotFound" {
		createResponse(w, "", http.StatusNotFound)
	} else if name == "StatusGone" {
		createResponse(w, "", http.StatusGone)
	} else if name == "StatusBadRequest" {
		createResponse(w, "", http.StatusBadRequest)
	} else {
		for k, v := range tridentNodeTable {
			if v == name {
				delete(tridentNodeTable, k)
				msg := ListNodesResponse{}
				x := append(msg.Nodes, name)
				msg.Nodes = x
				createResponse(w, msg, http.StatusOK)
			}
		}
	}
}

func createResponse(w http.ResponseWriter, msg any, sc int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(msg)
}

func mockGetChap(w http.ResponseWriter, r *http.Request) {
	str := strings.Split(r.URL.Path, "/")
	sc := http.StatusCreated

	if str[5] == "node1" {
		chapInfo := utils.IscsiChapInfo{}
		chapInfo.UseCHAP = true
		chap.CHAP = &chapInfo
		sc = http.StatusOK
		createResponse(w, chap, sc)
	} else if str[5] == "invalidResponse" {
		sc = http.StatusBadRequest
		msg := "invalidResponse"
		createResponse(w, msg, sc)
	} else {
		msg := GetCHAPResponse{}
		createResponse(w, msg, sc)
	}
}

func TestInvokeAPI(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	s := "{\"id\":\" uuid\"}"
	ctx = context.Background()
	server := getHttpServer(config.NodeURL, mockGetNodeResponse)
	controllerRestClient.url = server.URL
	response, _, err := controllerRestClient.InvokeAPI(ctx, []byte(s), "GET", "/trident/v1/node?id=1", true, true)
	server.Close()
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
}

func TestInvokeAPIResponseResourceNotFound(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	s := "{\"id\":\" uuid\"}"
	ctx = context.Background()
	server := getHttpServer("/trident/v1/notFound", mockResourceNotFound)
	controllerRestClient.url = server.URL
	response, _, err := controllerRestClient.InvokeAPI(ctx, []byte(s), "GET", "/trident/v1/notFound", true, true)
	server.Close()
	assert.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestCreateNode(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	tridentNodeTable["1"] = "tridentNode1"
	tridentNodeTable["2"] = "tridentNode2"

	tridentNodeTableLength := len(tridentNodeTable)

	ctx = context.Background()

	tests := []struct {
		nodeName        string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{nodeName: "VSM1", mockFunction: mockCreateNode, isErrorExpected: false},
		{nodeName: "", mockFunction: mockCreateNode, isErrorExpected: true},
		{nodeName: "invalidResponse", mockFunction: mockCreateNode, isErrorExpected: true},
	}

	for i, test := range tests {
		node := utils.Node{}
		node.Name = test.nodeName
		t.Run(fmt.Sprintf("CreateNode: %d", i), func(t *testing.T) {
			server := getHttpServer(config.NodeURL+"/"+test.nodeName, test.mockFunction)
			controllerRestClient.url = server.URL
			_, result := controllerRestClient.CreateNode(ctx, &node)
			if !test.isErrorExpected {
				assert.NoError(t, result)
				assert.Equal(t, tridentNodeTableLength+1, len(tridentNodeTable))
			} else {
				assert.Error(t, result)
			}
			server.Close()
		})
	}
}

func TestCreateNodeFailedInvokeAPICall(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	ctx = context.Background()
	controllerRestClient.url = ""
	controllerRestClient.httpClient = *http.DefaultClient
	node := utils.Node{}
	node.Name = "VSM1"
	_, err := controllerRestClient.CreateNode(ctx, &node)

	assert.Error(t, err)
}

func TestGetNode(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	ctx = context.Background()
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{mockFunction: mockGetNodeResponse, isErrorExpected: false},
		{mockFunction: mockResourceNotFound, isErrorExpected: true},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("GetNode: %d", i), func(t *testing.T) {
			server := getHttpServer(config.NodeURL, test.mockFunction)
			controllerRestClient.url = server.URL
			response, result := controllerRestClient.GetNodes(ctx)
			if !test.isErrorExpected {
				assert.NoError(t, result)
				assert.Equal(t, len(response), len(tridentNodeTable))
			} else {
				assert.Error(t, result)
			}
			server.Close()
		})
	}
}

func TestGetNodesInvokeAPIError(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	ctx = context.Background()
	controllerRestClient.url = ""

	_, err := controllerRestClient.GetNodes(ctx)
	assert.Error(t, err)
}

func TestDeleteNode(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	tridentNodeTable["1"] = "tridentNode1"
	tridentNodeTable["2"] = "tridentNode2"
	ctx = context.Background()
	tridentNodeTableLength := len(tridentNodeTable)

	tests := []struct {
		nodeName        string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{nodeName: "tridentNode1", mockFunction: mockDeleteNode, isErrorExpected: false},
		{nodeName: "StatusUnprocessableEntity", mockFunction: mockDeleteNode, isErrorExpected: false},
		{nodeName: "StatusNotFound", mockFunction: mockDeleteNode, isErrorExpected: false},
		{nodeName: "StatusGone", mockFunction: mockDeleteNode, isErrorExpected: false},
		{nodeName: "StatusBadRequest", mockFunction: mockDeleteNode, isErrorExpected: true},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("GetNode: %d", i), func(t *testing.T) {
			server := getHttpServer(config.NodeURL+"/"+test.nodeName, test.mockFunction)
			controllerRestClient.url = server.URL
			result := controllerRestClient.DeleteNode(ctx, test.nodeName)
			if !test.isErrorExpected {
				assert.NoError(t, result)
				assert.Equal(t, tridentNodeTableLength-1, len(tridentNodeTable))
			} else {
				assert.Error(t, result)
			}
			server.Close()
		})
	}
}

func TestDeleteNodeInvokeAPICallFailed(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	controllerRestClient.url = ""
	err := controllerRestClient.DeleteNode(ctx, "VSM1")
	assert.Error(t, err)
}

func TestGetChap(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	ctx = context.Background()

	tests := []struct {
		nodeName        string
		volumeName      string
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{nodeName: "node1", volumeName: "volume", mockFunction: mockGetChap, isErrorExpected: false},
		{nodeName: "invalidResponse", volumeName: "volume", mockFunction: mockGetChap, isErrorExpected: true},
		{nodeName: "nodeNotPresent", volumeName: "volume", mockFunction: mockGetChap, isErrorExpected: true},
	}

	for i, test := range tests {
		t.Run(fmt.Sprintf("GetNode: %d", i), func(t *testing.T) {
			server := getHttpServer(config.ChapURL+"/"+test.volumeName+"/"+test.nodeName, test.mockFunction)
			controllerRestClient.url = server.URL
			response, result := controllerRestClient.GetChap(ctx, test.volumeName, test.nodeName)
			if !test.isErrorExpected {
				assert.NoError(t, result)
				assert.Equal(t, chap.CHAP, response)
			} else {
				assert.Error(t, result)
			}
			server.Close()
		})
	}
}

func TestGetChapInvokeAPICallFailed(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	controllerRestClient.url = ""
	_, err := controllerRestClient.GetChap(ctx, "volume", "xyz1")
	assert.Error(t, err)
}

func TestCreateTLSRestClient(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	certFile := os.Getenv("CERT")
	keyFile := os.Getenv("KEY")

	fileHandler, e := os.Create("data.txt")
	assert.NoError(t, e)

	server := getHttpServer("/trident/v1/chap/volume/xyz", mockGetChap)
	controllerRestClient.url = server.URL
	_, err := CreateTLSRestClient(controllerRestClient.url, fileHandler.Name(), certFile, keyFile)
	server.Close()
	assert.NoError(t, err)
	fileHandler.Close()
	e = os.Remove("data.txt")
	assert.NoError(t, e)
}

func TestCreateTLSRestClientEmptyFile(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	certFile := os.Getenv("CERT")
	keyFile := os.Getenv("KEY")

	server := getHttpServer("/trident/v1/chap/volume/xyz", mockGetChap)
	controllerRestClient.url = server.URL
	_, err := CreateTLSRestClient(controllerRestClient.url, "", certFile, keyFile)
	server.Close()
	assert.NoError(t, err)
}
