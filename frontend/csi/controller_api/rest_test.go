// Copyright 2022 NetApp, Inc. All Rights Reserved.

package controllerAPI

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/models"
)

var (
	ctx              = context.Background()
	tridentNodeTable = make(map[string]string)
	chap             = GetCHAPResponse{}
)

func TestMain(m *testing.M) {
	InitLogOutput(io.Discard)
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

func mockInvalidResponse(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusNotFound
	createResponse(w, "xyz", sc)
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
		chapInfo := models.IscsiChapInfo{}
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

func mockInvokeAPIInternalError(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("500 - Something bad happened!"))
}

func mockIOUtilError(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Length", "50")
	w.Write([]byte("500 - Something bad happened!"))
}

func mockGetNodeNegative(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode("xyz\n}")
}

func TestInvokeAPI(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	s := "{\"id\":\" uuid\"}"
	ctx = context.Background()
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		httpStatus      int
		isErrorExpected bool
	}{
		{mockFunction: mockGetNodeResponse, httpStatus: http.StatusOK, isErrorExpected: false},
		{mockFunction: mockResourceNotFound, httpStatus: http.StatusNotFound, isErrorExpected: false},
		{mockFunction: mockInvalidResponse, httpStatus: http.StatusNotFound, isErrorExpected: false},
		{mockFunction: mockInvokeAPIInternalError, isErrorExpected: true},
		{mockFunction: mockIOUtilError, isErrorExpected: true},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("CreateNode: %d", i), func(t *testing.T) {
			server := getHttpServer(config.NodeURL, test.mockFunction)
			controllerRestClient.url = server.URL
			response, _, err := controllerRestClient.InvokeAPI(ctx, []byte(s), "GET", "/trident/v1/node?id=1", true, true)
			server.Close()
			if test.isErrorExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if response != nil {
				assert.Equal(t, test.httpStatus, response.StatusCode)
			}
		})
	}
}

func TestInvokeAPIInvalidInput(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	s := "name\":\" VSM\"}"
	ctx = context.Background()
	tests := []struct {
		ctx             context.Context
		isErrorExpected bool
	}{
		{ctx: ctx, isErrorExpected: false},
		{ctx: nil, isErrorExpected: false},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("CreateNode: %d", i), func(t *testing.T) {
			_, _, err := controllerRestClient.InvokeAPI(test.ctx, []byte(s), "GET", "/trident/v1/node?id=1", true, true)
			assert.Error(t, err)
		})
	}
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
		node := models.Node{}
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
	node := models.Node{}
	node.Name = "VSM1"
	_, err := controllerRestClient.CreateNode(ctx, &node)

	assert.Error(t, err)
}

func TestGetNodes(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	ctx = context.Background()
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{mockFunction: mockGetNodeResponse, isErrorExpected: false},
		{mockFunction: mockResourceNotFound, isErrorExpected: true},
		{mockFunction: mockGetNodeNegative, isErrorExpected: true},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("GetNodes: %d", i), func(t *testing.T) {
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

func TestGetNode(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	ctx = context.Background()
	tests := map[string]struct {
		mockHandlerGenerator func(string, string) func(w http.ResponseWriter, r *http.Request)
		isErrorExpected      bool
	}{
		"passes with status ok": {
			mockHandlerGenerator: func(
				url, nodeName string,
			) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json; charset=UTF-8")
					r.URL.Path = url
					fakeNodeResponse := &GetNodeResponse{
						Node: &models.NodeExternal{
							Name: nodeName,
						},
						Error: "",
					}
					responseData, err := json.Marshal(fakeNodeResponse)
					if err != nil {
						t.Fatalf("Failed to setup fake response for GetNode")
					}
					w.WriteHeader(http.StatusOK)
					if _, err := w.Write(responseData); err != nil {
						t.Fatalf("Failed to write fake response data for GetNode")
					}
				}
			},
			isErrorExpected: false,
		},
		"fails when api never writes a response": {
			mockHandlerGenerator: func(
				_, nodeName string,
			) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json; charset=UTF-8")
					r.URL.Path = ""
				}
			},
			isErrorExpected: true,
		},
		"fails with unexpected status": {
			mockHandlerGenerator: func(
				url, nodeName string,
			) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json; charset=UTF-8")
					r.URL.Path = url
					responseData, err := json.Marshal(&GetNodeResponse{})
					if err != nil {
						t.Fatalf("Failed to setup fake response for GetNode")
					}
					w.WriteHeader(http.StatusAccepted)
					if _, err := w.Write(responseData); err != nil {
						t.Fatalf("Failed to write fake response data for GetNode")
					}
				}
			},
			isErrorExpected: true,
		},
		"fails to parse response body": {
			mockHandlerGenerator: func(
				url, nodeName string,
			) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json; charset=UTF-8")
					r.URL.Path = url
					responseData, err := json.Marshal(`{{////}`)
					if err != nil {
						t.Fatalf("Failed to setup fake response for GetNode")
					}
					w.WriteHeader(http.StatusOK)
					if _, err := w.Write(responseData); err != nil {
						t.Fatalf("Failed to write fake response data for GetNode")
					}
				}
			},
			isErrorExpected: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup fake handler and server.
			nodeName := "foo"
			url := config.NodeURL + "/" + nodeName
			handler := test.mockHandlerGenerator(url, nodeName)
			server := getHttpServer(url, handler)
			defer server.Close()

			// Connect the rest client to the fake server.
			controllerRestClient.url = server.URL

			// Call the unit in test.
			node, err := controllerRestClient.GetNode(ctx, nodeName)

			// Assertions.
			if test.isErrorExpected {
				assert.Error(t, err)
				assert.Nil(t, node)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, node)
			}
		})
	}
}

func TestUpdateNode(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	ctx = context.Background()
	tests := map[string]struct {
		nodeState            *models.NodePublicationStateFlags
		mockHandlerGenerator func(string, string) func(w http.ResponseWriter, r *http.Request)
		isErrorExpected      bool
	}{
		"passes with status accepted": {
			nodeState: &models.NodePublicationStateFlags{},
			mockHandlerGenerator: func(
				url, nodeName string,
			) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json; charset=UTF-8")
					r.URL.Path = url
					responseData, err := json.Marshal(&UpdateNodeResponse{
						Name: nodeName,
					})
					if err != nil {
						t.Fatalf("Failed to setup fake response for UpdateNode")
					}
					w.WriteHeader(http.StatusAccepted)
					if _, err := w.Write(responseData); err != nil {
						t.Fatalf("Failed to write fake response data for UpdateNode")
					}
				}
			},
			isErrorExpected: false,
		},
		"fails when api never writes a response": {
			nodeState: &models.NodePublicationStateFlags{},
			mockHandlerGenerator: func(
				_, nodeName string,
			) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json; charset=UTF-8")
					r.URL.Path = ""
				}
			},
			isErrorExpected: true,
		},
		"fails with unexpected status": {
			nodeState: &models.NodePublicationStateFlags{},
			mockHandlerGenerator: func(
				url, nodeName string,
			) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json; charset=UTF-8")
					r.URL.Path = url
					responseData, err := json.Marshal(&UpdateNodeResponse{
						Name: nodeName,
					})
					if err != nil {
						t.Fatalf("Failed to setup fake response for UpdateNode")
					}
					// Should be StatusAccepted.
					w.WriteHeader(http.StatusOK)
					if _, err := w.Write(responseData); err != nil {
						t.Fatalf("Failed to write fake response data for UpdateNode")
					}
				}
			},
			isErrorExpected: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup fake handler and server.
			nodeName := "foo"
			url := fmt.Sprintf("%s/%s/%s", config.NodeURL, nodeName, "publicationState")
			handler := test.mockHandlerGenerator(url, nodeName)
			server := getHttpServer(url, handler)
			defer server.Close()

			// Connect the rest client to the fake server.
			controllerRestClient.url = server.URL

			// Call the unit in test.
			err := controllerRestClient.UpdateNode(ctx, nodeName, test.nodeState)

			// Assertions.
			if test.isErrorExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
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
		t.Run(fmt.Sprintf("DeleteNode: %d", i), func(t *testing.T) {
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
		t.Run(fmt.Sprintf("GetChap: %d", i), func(t *testing.T) {
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
	tests := []struct {
		caFileName      string
		certFileName    string
		keyfileName     string
		isErrorExpected bool
	}{
		{caFileName: fileHandler.Name(), certFileName: certFile, keyfileName: keyFile, isErrorExpected: false},
		{caFileName: "", certFileName: certFile, keyfileName: keyFile, isErrorExpected: false},
		{caFileName: fileHandler.Name(), certFileName: fileHandler.Name(), keyfileName: fileHandler.Name(), isErrorExpected: true},
	}
	for i, test := range tests {
		t.Run(fmt.Sprintf("CreateNode: %d", i), func(t *testing.T) {
			server := getHttpServer("/trident/v1/chap/volume/xyz", mockGetChap)
			controllerRestClient.url = server.URL
			_, err := CreateTLSRestClient(controllerRestClient.url, test.caFileName, test.certFileName, test.keyfileName)
			server.Close()
			if test.isErrorExpected {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
	fileHandler.Close()
	e = os.Remove("data.txt")
	assert.NoError(t, e)
}

func TestUpdateVolumeLUKSPassphraseNames(t *testing.T) {
	// Positive
	controllerRestClient := ControllerRestClient{}
	ctx = context.Background()
	mockUpdateVolumeLUKSPassphraseNames := func(w http.ResponseWriter, r *http.Request) {
		passphraseNames := new([]string)
		body, err := io.ReadAll(io.LimitReader(r.Body, config.MaxRESTRequestSize))
		assert.NoError(t, err)

		err = json.Unmarshal(body, passphraseNames)
		assert.NoError(t, err, "Got: ", body)

		createResponse(w, "", http.StatusOK)
	}

	server := getHttpServer(config.VolumeURL+"/"+"test-vol/luksPassphraseNames", mockUpdateVolumeLUKSPassphraseNames)
	controllerRestClient.url = server.URL
	err := controllerRestClient.UpdateVolumeLUKSPassphraseNames(ctx, "test-vol", []string{"A"})
	assert.NoError(t, err)
	server.Close()

	// Negative: Volume not found
	controllerRestClient = ControllerRestClient{}
	ctx = context.Background()
	mockUpdateVolumeLUKSPassphraseNames = func(w http.ResponseWriter, r *http.Request) {
		createResponse(w, "", http.StatusNotFound)
	}

	server = getHttpServer(config.VolumeURL+"/"+"test-vol/luksPassphraseNames", mockUpdateVolumeLUKSPassphraseNames)
	controllerRestClient.url = server.URL
	err = controllerRestClient.UpdateVolumeLUKSPassphraseNames(ctx, "test-vol", []string{"A"})
	assert.Error(t, err)
	server.Close()

	// Negative: Cannot connect to trident api
	controllerRestClient = ControllerRestClient{}
	ctx = context.Background()
	err = controllerRestClient.UpdateVolumeLUKSPassphraseNames(ctx, "test-vol", []string{"A"})
	assert.Error(t, err)
}

func TestListVolumePublicationsForNode(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	ctx = context.Background()
	tests := map[string]struct {
		handlerGenerator func(string, string, string) func(w http.ResponseWriter, r *http.Request)
		isErrorExpected  bool
	}{
		"passes with status ok": {
			handlerGenerator: func(
				url, volumeName, nodeName string,
			) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json; charset=UTF-8")
					r.URL.Path = url
					fakeListVolumePublicationsResponse := &ListVolumePublicationsResponse{
						VolumePublications: []*models.VolumePublicationExternal{
							{
								Name:       utils.GenerateVolumePublishName(volumeName, nodeName),
								VolumeName: volumeName,
								NodeName:   nodeName,
							},
						},
						Error: "",
					}
					responseData, err := json.Marshal(fakeListVolumePublicationsResponse)
					if err != nil {
						t.Fatalf("Failed to setup fake response for ListVolumePublicationsForNode")
					}
					w.WriteHeader(http.StatusOK)
					if _, err := w.Write(responseData); err != nil {
						t.Fatalf("Failed to write fake response data for ListVolumePublicationsForNode")
					}
				}
			},
			isErrorExpected: false,
		},
		"fails when api never writes a response": {
			handlerGenerator: func(
				_, _, _ string,
			) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json; charset=UTF-8")
					r.URL.Path = ""
				}
			},
			isErrorExpected: true,
		},
		"fails with unexpected status": {
			handlerGenerator: func(
				url, volumeName, nodeName string,
			) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json; charset=UTF-8")
					r.URL.Path = url
					responseData, err := json.Marshal(&ListVolumePublicationsResponse{})
					if err != nil {
						t.Fatalf("Failed to setup fake response for GetNode")
					}
					w.WriteHeader(http.StatusAccepted)
					if _, err := w.Write(responseData); err != nil {
						t.Fatalf("Failed to write fake response data for GetNode")
					}
				}
			},
			isErrorExpected: true,
		},
		"fails to parse response body": {
			handlerGenerator: func(
				url, _, _ string,
			) func(w http.ResponseWriter, r *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json; charset=UTF-8")
					r.URL.Path = url
					responseData, err := json.Marshal(`{{////}`)
					if err != nil {
						t.Fatalf("Failed to setup fake response for GetNode")
					}
					w.WriteHeader(http.StatusOK)
					if _, err := w.Write(responseData); err != nil {
						t.Fatalf("Failed to write fake response data for GetNode")
					}
				}
			},
			isErrorExpected: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Setup fake handler and server.
			nodeName := "foo"
			volumeName := "bar"
			url := fmt.Sprintf("%s/%s/%s", config.NodeURL, nodeName, "publication")
			handler := test.handlerGenerator(url, volumeName, nodeName)
			server := getHttpServer(url, handler)
			defer server.Close()

			// Connect the rest client to the fake server.
			controllerRestClient.url = server.URL

			// Call the unit in test.
			publications, err := controllerRestClient.ListVolumePublicationsForNode(ctx, nodeName)

			// Assertions.
			if test.isErrorExpected {
				assert.Error(t, err)
				assert.Empty(t, publications)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, publications)
			}
		})
	}
}

func TestRequestAndRetry(t *testing.T) {
	controllerRestClient := ControllerRestClient{}

	tests := map[string]struct {
		requestFunc     func() (*http.Response, []byte, error)
		isErrorExpected bool
	}{
		"passes with status ok": {
			requestFunc: func() (*http.Response, []byte, error) {
				response := &http.Response{
					StatusCode: http.StatusOK,
				}
				return response, []byte{}, nil
			},
			isErrorExpected: false,
		},
		"passes with status accepted": {
			requestFunc: func() (*http.Response, []byte, error) {
				response := &http.Response{
					StatusCode: http.StatusAccepted,
				}
				return response, []byte{}, nil
			},
			isErrorExpected: false,
		},
		"fails with too many requests but no retry after header": {
			requestFunc: func() (*http.Response, []byte, error) {
				response := &http.Response{
					StatusCode: http.StatusTooManyRequests,
				}
				response.Header.Del("Retry-After")
				return response, []byte{}, nil
			},
			isErrorExpected: true,
		},
		"fails with http error": {
			requestFunc: func() (*http.Response, []byte, error) {
				return nil, nil, fmt.Errorf("could not communicate with HTTP controller server")
			},
			isErrorExpected: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := controllerRestClient.requestAndRetry(ctx, test.requestFunc)
			if test.isErrorExpected {
				assert.Error(t, err, "expected error")
			} else {
				assert.NoError(t, err, "unexpected error")
			}
		})
	}
}

func TestRequestAndRetrySucceedsAfterTooManyRequests(t *testing.T) {
	controllerRestClient := ControllerRestClient{}
	wg := &sync.WaitGroup{}

	// Setting this up here will allow changing the response data
	// even after requestAndRetry starts calling the requestFunc.
	response := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     make(http.Header, 0),
	}

	// Ensure this change happens after the first request but before the second.
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Sleep for a bit to give requestAndRetry a chance to retry.
		time.Sleep(100 * time.Millisecond)
		// Changing this here will allow requestAndRetry to exit.
		response.StatusCode = http.StatusAccepted
	}()

	// Set up a closure so the response can be modified after this has been passed to requestAndRetry.
	requestFunc := func() (*http.Response, []byte, error) {
		// Set up some delay here so that requestAndRetry can run a few iterations.
		response.Header.Set("Retry-After", "200ms")
		return response, []byte{}, nil
	}

	_, _, err := controllerRestClient.requestAndRetry(ctx, requestFunc)
	wg.Wait()
	assert.NoError(t, err, "expected no error")
}
