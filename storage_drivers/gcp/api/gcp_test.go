// Copyright 2022 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2"

	. "github.com/netapp/trident/logging"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/utils"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

var (
	ctx       = context.TODO()
	testToken = oauth2.Token{
		AccessToken: "--THIS-IS-AN-TEST-TOKEN--BOT-VALID-FOR-ACTUAL-USAGE--" +
			"eyJ0eXAiOiAiSldUIiwgImFsZyI6ICJSUzI1NiIsICJraWQiOiAiZWViYTQxM2RjZjU0OTkyY2JiYjI2NWEzNjliN2RlNmJh" +
			"ZWJlODhmOCJ9.eyJpc3MiOiAiY2xvdWR2b2x1bWVzLWFkbWluLXNhQGNsb3VkLW5hdGl2ZS1kYXRhLmlhbS5nc2VydmljZWF" +
			"jY291bnQuY29tIiwgInN1YiI6ICJjbG91ZHZvbHVtZXMtYWRtaW4tc2FAY2xvdWQtbmF0aXZlLWRhdGEuaWFtLmdzZXJ2aWN" +
			"lYWNjb3VudC5jb20iLCAiaWF0IjogMTY3MzI4MjA5MSwgImV4cCI6IDE2NzMyODU2OTEsICJhdWQiOiAiaHR0cHM6Ly9jbG9" +
			"1ZHZvbHVtZXNnY3AtYXBpLm5ldGFwcC5jb20ifQ.LHM12WCci-1AHKwDvS4cmFdORAeldqEiXbhALb0msCqIxJYzlCEwoPzR" +
			"4kklsLXDaqiXba5OrTCNqO9e0WoPGt5wPljSXPEOl_eqP5hyHW7sSBsb1dZWPr7SYDwGQ1nnj5uxhvKVa6ED1fp5qLBtv8zk" +
			"yl-2K4GGhggXfdu9eqs5GZmtxnpx6Rs6GClLqbHxBzwrmj17GwXD41WaoO7-r9rm2lKlHY-3c2WUl6Nd2YWgDV9fWiJV5FxL" +
			"iUpF2wMQ0tsrnOrGYh-f93AXpthodKDyROwMLFk6DwbBBvmk0xmwiolA00OAQjubdFjfW1TZATiZBDxgcPWJzs12eVEcVg",
		TokenType:    "",
		RefreshToken: "",
		Expiry:       time.Now().Local().Add(60 * time.Second),
	}
)

// Below is a type which is further used to create a tokenSource.
type tokenSourceInterface string

func (tokenSourceInterface) Token() (*oauth2.Token, error) {
	return &testToken, nil
}

// getGCPClient to return the new driver with the unit test is set to true and the URL is set to local server.
func getGCPClient(apiUrl string) *Client {
	var tokenSourceTest oauth2.TokenSource
	var tokenSourceLocal tokenSourceInterface
	tokenSourceTest = tokenSourceLocal

	clientConfig := ClientConfig{
		ProjectNumber:   "project-num-1",
		APIURL:          defaultAPIURL,
		DebugTraceFlags: map[string]bool{},
		APIRegion:       "us-east-1",
	}

	d := &Client{
		config:      &clientConfig,
		tokenSource: &tokenSourceTest,
		token:       &testToken,
		m:           &sync.Mutex{},
		unitTestURL: apiUrl,
	}

	return d
}

// getGCPClientWithProxy to return the new driver with the proxy set, This would fail to invoke the api.
func getGCPClientWithProxy(apiUrl string) *Client {
	var tokenSourceTest oauth2.TokenSource
	var tokenSourceLocal tokenSourceInterface
	tokenSourceTest = tokenSourceLocal

	clientConfig := ClientConfig{
		ProjectNumber:   "project-num-1",
		APIURL:          defaultAPIURL,
		ProxyURL:        "proxy-url",
		DebugTraceFlags: map[string]bool{"api": true},
		APIRegion:       "us-east-1",
	}

	d := &Client{
		config:      &clientConfig,
		tokenSource: &tokenSourceTest,
		token:       &testToken,
		m:           &sync.Mutex{},
		unitTestURL: apiUrl,
	}

	return d
}

// getGCPClientInvalidToken to cover negetive case where token is invalid.
func getGCPClientInvalidToken(apiUrl string) *Client {
	clientConfig := ClientConfig{
		ProjectNumber:   "project-num-1",
		APIURL:          defaultAPIURL,
		DebugTraceFlags: map[string]bool{},
		APIRegion:       "us-east-1",
	}

	d := &Client{
		config:      &clientConfig,
		m:           &sync.Mutex{},
		unitTestURL: apiUrl,
	}

	return d
}

func createResponse(w http.ResponseWriter, msg any, sc int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	json.NewEncoder(w).Encode(msg)
}

// createResponseNonMarshall to induce parse error when the response is handled.
func createResponseNonMarshall(w http.ResponseWriter, msg []byte, sc int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(sc)
	w.Write(msg)
}

func TestNewDriver(t *testing.T) {
	config := ClientConfig{
		MountPoint:      utils.MountPoint{},
		ProjectNumber:   "",
		APIKey:          drivers.GCPPrivateKey{},
		APIRegion:       "",
		ProxyURL:        "",
		DebugTraceFlags: nil,
		APIURL:          "",
		APIAudienceURL:  "",
	}

	d := NewDriver(config)
	assert.NotNil(t, d, "Driver is empty.")
}

func TestMakeURL(t *testing.T) {
	// Here the unit test flag is set to true and hence the url which is passed in as argument itself is returned.
	d := getGCPClient("https://cvgcp-api.netapp.com/v2/projects/project-num-1/locations/us-east-1rs-path")
	assert.Equal(t, d.makeURL(""),
		"https://cvgcp-api.netapp.com/v2/projects/project-num-1/locations/us-east-1rs-path", "Wrong URL is returned")

	// Setting unitTestURL to false so that the new URL is constructed.
	d.unitTestURL = ""
	assert.Equal(t, d.makeURL("rs-path"),
		"https://cloudvolumesgcp-api.netapp.com/v2/projects/project-num-1/locations/us-east-1rs-path",
		"Wrong URL is returned")
}

func TestClient_refreshToken(t *testing.T) {
	d := getGCPClientWithProxy("srv.URL")
	d.token = nil
	_, _, err := d.GetVersion(ctx)
	assert.Error(t, err, "Expecting an error.")
}

// TestClient_InvokeAPI to test the negative test with the proxy set.
func TestClient_InvokeAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(mockGetVersionResponse))
	d := getGCPClientWithProxy(srv.URL)
	_, _, err := d.GetVersion(ctx)

	assert.Error(t, err, "Expecting an error when proxy is set.")
}

// Common response functions for multiple test functions.
func mockResourceNotPresent(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	createResponse(w, nil, sc)
}

func mockResourceNotFound(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusNotFound
	createResponse(w, nil, sc)
}

func mockUnAuthorizedResponseError(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusUnauthorized
	createResponse(w, nil, sc)
}

func mockGetResponseUnMarshalled(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := []byte("invalid response")
	createResponseNonMarshall(w, msg, sc)
}

func mockGetInvalidResponseUnMarshalled(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusUnauthorized
	msg := []byte("invalid response")
	createResponseNonMarshall(w, msg, sc)
}

func mockGetVersionResponse(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := VersionResponse{APIVersion: "1.2.15", SdeVersion: "4.14.59"}
	createResponse(w, msg, sc)
}

func mockGetVersionAPIVersionNotFound(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := VersionResponse{APIVersion: "", SdeVersion: "4.14.59"}
	createResponse(w, msg, sc)
}

func mockGetVersionSDEVersionNotFound(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := VersionResponse{APIVersion: "1.2.15", SdeVersion: ""}
	createResponse(w, msg, sc)
}

func TestClient_GetVersion(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{mockFunction: mockGetVersionResponse, isErrorExpected: false},
		{mockFunction: mockGetVersionAPIVersionNotFound, isErrorExpected: true},
		{mockFunction: mockGetVersionSDEVersionNotFound, isErrorExpected: true},
		{mockFunction: mockResourceNotFound, isErrorExpected: true},
		{mockFunction: mockGetResponseUnMarshalled, isErrorExpected: true},
		{mockFunction: mockUnAuthorizedResponseError, isErrorExpected: true},
		{mockFunction: mockGetInvalidResponseUnMarshalled, isErrorExpected: true},
	}

	for i, entry := range tests {
		t.Run(fmt.Sprintf("GetVersion %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			apiVer, sdeVer, err := d.GetVersion(ctx)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, apiVer.String(), "1.2.15", "Wrong API version is returned")
				assert.Equal(t, sdeVer.String(), "4.14.59", "Wrong SDE version is returned")
			}
			srv.Close()
		})
	}

	t.Run("GetVersion_invalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, _, err := d.GetVersion(ctx)
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func mockGetServiceLevelsResponse(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := []ServiceLevel{{
		Name:        "service1",
		Performance: UserServiceLevel1,
	}}
	createResponse(w, msg, sc)
}

func TestClient_GetServiceLevels(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{mockFunction: mockGetServiceLevelsResponse, isErrorExpected: false},
		{mockFunction: mockResourceNotPresent, isErrorExpected: false},
		{mockFunction: mockGetResponseUnMarshalled, isErrorExpected: true},
		{mockFunction: mockResourceNotFound, isErrorExpected: true},
	}

	for i, entry := range tests {
		t.Run(fmt.Sprintf("GetServiceLevels %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			srvL := ""
			d := getGCPClient(srv.URL)
			srvLevels, err := d.GetServiceLevels(ctx)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected")
			} else {
				assert.NoError(t, err, "No error is expected.")
				for idx := range srvLevels {
					srvL = srvLevels[idx]
				}
				if srvL != "" {
					assert.Equal(t, srvL, UserServiceLevel1, "Incorrect service level is returned.")
				}
			}
			srv.Close()
		})
	}

	t.Run("GetServiceLevels_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.GetServiceLevels(ctx)
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func mockGetVolumesResponse(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := []Volume{
		{
			Created:               time.Now(),
			LifeCycleState:        "available",
			LifeCycleStateDetails: "Available for use",
			Name:                  "big-data",
			VolumeID:              "49b96a2F-4a38-6fa4-2CC6-F598Ef2f8a0E",
			CreationToken:         "suddenly-distinguished-feynman",
		},
	}
	createResponse(w, msg, sc)
}

func TestClient_GetVolumes(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{mockFunction: mockGetVolumesResponse, isErrorExpected: false},
		{mockFunction: mockResourceNotPresent, isErrorExpected: false},
		{mockFunction: mockGetResponseUnMarshalled, isErrorExpected: true},
		{mockFunction: mockResourceNotFound, isErrorExpected: true},
	}

	for i, entry := range tests {
		t.Run(fmt.Sprintf("GetVolumes: %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			_, err := d.GetVolumes(ctx)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
			}
			srv.Close()
		})
	}

	t.Run("GetVolumes_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.GetVolumes(ctx)
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func mockGetVolumeByNameMultipleResponse(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := []Volume{
		{
			LifeCycleState:        "available",
			LifeCycleStateDetails: "Available for use",
			Name:                  "big-data",
			VolumeID:              "49b96a2F-4a38-6fa4-2CC6-F598Ef2f8a0E",
			CreationToken:         "suddenly-distinguished-feynman",
		}, {
			LifeCycleState:        "available",
			LifeCycleStateDetails: "Available for use",
			Name:                  "big-data",
			VolumeID:              "49b96a2F-4a38-6fa4-2CC6-F598Ef2f8a0E",
			CreationToken:         "suddenly-distinguished-feynman",
		},
	}
	createResponse(w, msg, sc)
}

func TestClient_GetVolumeByName(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		volumeName      string
		isErrorExpected bool
	}{
		{mockFunction: mockGetVolumesResponse, volumeName: "big-data", isErrorExpected: false},
		{mockFunction: mockGetVolumesResponse, volumeName: "small-data", isErrorExpected: true},
		{mockFunction: mockGetVolumeByNameMultipleResponse, volumeName: "big-data", isErrorExpected: true},
		{mockFunction: mockGetResponseUnMarshalled, isErrorExpected: true},
		{mockFunction: mockResourceNotPresent, isErrorExpected: true},
		{mockFunction: mockResourceNotFound, isErrorExpected: true},
	}

	for i, entry := range tests {
		t.Run(fmt.Sprintf("GetVolumeByName %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			v1, err := d.GetVolumeByName(ctx, entry.volumeName)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
				assert.Equal(t, v1.Name, "big-data", "Volume does not exist.")
			}
			srv.Close()
		})
	}

	t.Run("GetVolumeByName_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.GetVolumeByName(ctx, "volumeName")
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func TestClient_GetVolumeByCreationToken(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		CreationToken   string
		isErrorExpected bool
	}{
		{mockFunction: mockGetVolumesResponse, CreationToken: "suddenly-distinguished-feynman", isErrorExpected: false},
		{mockFunction: mockResourceNotPresent, CreationToken: "some-other-token", isErrorExpected: true},
		{mockFunction: mockGetResponseUnMarshalled, isErrorExpected: true},
		{mockFunction: mockGetVolumeByNameMultipleResponse, CreationToken: "suddenly-distinguished-feynman", isErrorExpected: true},
		{mockFunction: mockResourceNotPresent, isErrorExpected: true},
		{mockFunction: mockResourceNotFound, isErrorExpected: true},
	}

	for i, entry := range tests {
		t.Run(fmt.Sprintf("GetVolumeByCreationToken %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			v1, err := d.GetVolumeByCreationToken(ctx, entry.CreationToken)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
				assert.Equal(t, v1.Name, "big-data", "Volume does not exist.")
			}
			srv.Close()
		})
	}

	t.Run("GetVolumeByCreationToken_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.GetVolumeByCreationToken(ctx, "CreationToken")
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func TestClient_VolumeExistsByCreationToken(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		CreationToken   string
		isErrorExpected bool
		checkForVolume  bool
	}{
		{
			mockFunction: mockGetVolumesResponse, CreationToken: "suddenly-distinguished-feynman",
			isErrorExpected: false, checkForVolume: true,
		},
		{mockFunction: mockResourceNotPresent, CreationToken: "small-data", isErrorExpected: false},
		{
			mockFunction: mockGetVolumeByNameMultipleResponse, CreationToken: "suddenly-distinguished-feynman",
			isErrorExpected: false, checkForVolume: true,
		},
		{mockFunction: mockGetResponseUnMarshalled, isErrorExpected: true},
		{mockFunction: mockResourceNotPresent, isErrorExpected: false, checkForVolume: false},
		{mockFunction: mockResourceNotFound, isErrorExpected: false, checkForVolume: false},
	}

	for i, entry := range tests {
		t.Run(fmt.Sprintf("VolumeExistsByCreationToken %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			exists, v1, err := d.VolumeExistsByCreationToken(ctx, entry.CreationToken)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
				assert.Equal(t, exists, false, "No volume should be present.")
			} else {
				assert.NoError(t, err)
				if entry.checkForVolume {
					assert.Equal(t, exists, true, "No error is expected.")
					assert.Equal(t, v1.Name, "big-data", "Volume does not exist.")
				}
			}
			srv.Close()
		})
	}

	t.Run("VolumeExistsByCreationToken_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, _, err := d.VolumeExistsByCreationToken(ctx, "CreationToken")
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func mockGetVolumeByIDResponse(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := Volume{
		LifeCycleState: "available",
		Name:           "big-data",
		VolumeID:       "49b96a2F-4a38-6fa4-2CC6-F598Ef2f8a0E",
		CreationToken:  "suddenly-distinguished-feynman",
	}
	createResponse(w, msg, sc)
}

func mockGetVolumeByIDResponseNoLCSDetails(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := Volume{
		Name:           "big-data",
		VolumeID:       "49b96a2F-4a38-6fa4-2CC6-F598Ef2f8a0E",
		CreationToken:  "suddenly-distinguished-feynman",
		LifeCycleState: "pending",
	}
	createResponse(w, msg, sc)
}

func TestClient_GetVolumeByID(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		volumeID        string
		isErrorExpected bool
	}{
		{
			mockFunction: mockGetVolumeByIDResponse, volumeID: "49b96a2F-4a38-6fa4-2CC6-F598Ef2f8a0E",
			isErrorExpected: false,
		},
		{mockFunction: mockResourceNotPresent, volumeID: "random-id", isErrorExpected: false},
		{mockFunction: mockGetResponseUnMarshalled, isErrorExpected: true},
		{mockFunction: mockResourceNotPresent, isErrorExpected: false},
		{mockFunction: mockResourceNotFound, isErrorExpected: true},
	}

	for i, entry := range tests {
		t.Run(fmt.Sprintf("GetVolumeByID %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			_, err := d.GetVolumeByID(ctx, entry.volumeID)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
			}
			srv.Close()
		})
	}

	t.Run("GetVolumeByID_InvalidToekn", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.GetVolumeByID(ctx, "VolumeID")
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func mockGetVolumeByIDResponseNoState(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := Volume{
		LifeCycleState:        "",
		LifeCycleStateDetails: "Available for use",
		Name:                  "big-data",
	}
	createResponse(w, msg, sc)
}

func TestClient_WaitForVolumeStates(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{mockFunction: mockGetVolumeByIDResponseNoState, isErrorExpected: true},
		{mockFunction: mockGetVolumeByIDResponse, isErrorExpected: false},
		{mockFunction: mockUnAuthorizedResponseError, isErrorExpected: true},
		{mockFunction: mockGetVolumeByIDResponseNoLCSDetails, isErrorExpected: true},
	}

	for i, entry := range tests {
		t.Run(fmt.Sprintf("GetVolumeByID %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			volume := Volume{LifeCycleState: ""}
			vol, err := d.WaitForVolumeStates(ctx, &volume, []string{"available"}, []string{"pending"}, 1*time.Second)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
				assert.Equal(t, vol, "available", "Desired volume state is Not reached.")
			}
			srv.Close()
		})
	}
}

func mockCreateVolume(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusCreated
	msg := Volume{Name: "newVolume"}
	createResponse(w, msg, sc)
}

func TestClient_CreateVolume(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		volumeRequest   VolumeCreateRequest
		isErrorExpected bool
	}{
		{mockFunction: mockCreateVolume, volumeRequest: VolumeCreateRequest{Name: "valid Vol"}, isErrorExpected: false},
		{
			mockFunction:  mockUnAuthorizedResponseError,
			volumeRequest: VolumeCreateRequest{Name: "valid Vol"}, isErrorExpected: true,
		},
	}

	for i, entry := range tests {
		t.Run(fmt.Sprintf("CreateVolume %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			err := d.CreateVolume(ctx, &entry.volumeRequest)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "Volume creation failed.")
			}
			srv.Close()
		})
	}

	t.Run("CreateVolume_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		err := d.CreateVolume(ctx, &VolumeCreateRequest{Name: "valid Vol"})
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func TestClient_RenameVolume(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		volume          Volume
		isErrorExpected bool
	}{
		{mockFunction: mockCreateVolume, volume: Volume{Name: "newVolume"}, isErrorExpected: false},
		{mockFunction: mockUnAuthorizedResponseError, volume: Volume{Name: "Invalid Vol"}, isErrorExpected: true},
		{mockFunction: mockGetResponseUnMarshalled, volume: Volume{Name: "Invalid Vol"}, isErrorExpected: true},
	}
	for i, entry := range tests {
		t.Run(fmt.Sprintf("RenameVolume %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			vol, err := d.RenameVolume(ctx, &entry.volume, "newVolume")

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
				assert.Equal(t, vol.Name, "newVolume", "Volume renaming failed.")
			}
			srv.Close()
		})
	}

	t.Run("RenameVolume_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.RenameVolume(ctx, &Volume{Name: "oldVolume"}, "newVolume")
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func TestClient_ChangeVolumeUnixPermissions(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		volume          Volume
		isErrorExpected bool
	}{
		{
			mockFunction: mockCreateVolume, volume: Volume{UnixPermissions: "ro"},
			isErrorExpected: false,
		},
		{
			mockFunction: mockUnAuthorizedResponseError, volume: Volume{UnixPermissions: "Invalid"},
			isErrorExpected: true,
		},
		{
			mockFunction: mockGetResponseUnMarshalled, volume: Volume{UnixPermissions: "Invalid"},
			isErrorExpected: true,
		},
	}
	for i, entry := range tests {
		t.Run(fmt.Sprintf("ChangeVolumeUnixPermissions %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			vol, err := d.ChangeVolumeUnixPermissions(ctx, &entry.volume, "rw")

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
				assert.Equal(t, vol.Name, "newVolume", "unix permission operation failed.")
			}
			srv.Close()
		})
	}

	t.Run("ChangeVolumeUnixPermissions_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.ChangeVolumeUnixPermissions(ctx, &Volume{Name: "oldVolume"}, "ro")
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func TestClient_RelabelVolume(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		volume          Volume
		isErrorExpected bool
	}{
		{
			mockFunction: mockDeleteVolume, volume: Volume{Labels: []string{"ro"}},
			isErrorExpected: false,
		},
		{
			mockFunction: mockUnAuthorizedResponseError, volume: Volume{Labels: []string{"invalid"}},
			isErrorExpected: true,
		},
		{
			mockFunction: mockGetResponseUnMarshalled, volume: Volume{UnixPermissions: "Invalid"},
			isErrorExpected: true,
		},
	}
	for i, entry := range tests {
		t.Run(fmt.Sprintf("RelabelVolume %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			vol, err := d.RelabelVolume(ctx, &entry.volume, []string{"rw"})

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, vol.Name, "deletedVol", "Volume relabeling failed.")
			}
			srv.Close()
		})
	}

	t.Run("RelabelVolume_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.RelabelVolume(ctx, &Volume{Name: "oldVolume"}, []string{"rw"})
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func TestClient_RenameRelabelVolume(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		volume          Volume
		isErrorExpected bool
	}{
		{
			mockFunction: mockDeleteVolume, volume: Volume{Name: "volume", Labels: []string{"ro"}},
			isErrorExpected: false,
		},
		{
			mockFunction: mockUnAuthorizedResponseError, volume: Volume{Name: "volume", Labels: []string{"invalid"}},
			isErrorExpected: true,
		},
		{
			mockFunction: mockGetResponseUnMarshalled, volume: Volume{Name: "volume", Labels: []string{"invalid"}},
			isErrorExpected: true,
		},
	}
	for i, entry := range tests {
		t.Run(fmt.Sprintf("RenameRelabelVolume %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			vol, err := d.RenameRelabelVolume(ctx, &entry.volume, "newVolume", []string{"rw"})

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, vol.Name, "deletedVol", "Volume renaming and relabeling failed.")
			}
			srv.Close()
		})
	}

	t.Run("RenameRelabelVolume_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.RenameRelabelVolume(ctx, &Volume{Name: "oldVolume"}, "newVolume", []string{"rw"})
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func TestClient_ResizeVolume(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		volume          Volume
		isErrorExpected bool
	}{
		{
			mockFunction: mockDeleteVolume, volume: Volume{Name: "volume", Labels: []string{"ro"}},
			isErrorExpected: false,
		},
		{
			mockFunction: mockUnAuthorizedResponseError, volume: Volume{Name: "volume", Labels: []string{"invalid"}},
			isErrorExpected: true,
		},
		{
			mockFunction: mockGetResponseUnMarshalled, volume: Volume{Name: "volume", Labels: []string{"invalid"}},
			isErrorExpected: true,
		},
	}
	for i, entry := range tests {
		t.Run(fmt.Sprintf("ResizeVolume %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			vol, err := d.ResizeVolume(ctx, &entry.volume, 1024)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected")
				assert.Equal(t, vol.Name, "deletedVol", "Volume resizing failed.")
			}
			srv.Close()
		})
	}

	t.Run("ResizeVolume_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.ResizeVolume(ctx, &Volume{Name: "oldVolume"}, 1024)
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func mockDeleteVolume(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := Volume{Name: "deletedVol"}
	createResponse(w, msg, sc)
}

func TestClient_DeleteVolume(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		volume          Volume
		isErrorExpected bool
	}{
		{mockFunction: mockDeleteVolume, volume: Volume{Name: "valid Vol"}, isErrorExpected: false},
		{mockFunction: mockUnAuthorizedResponseError, volume: Volume{Name: "Invalid Vol"}, isErrorExpected: true},
	}
	for i, entry := range tests {
		t.Run(fmt.Sprintf("DeleteVolume %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			err := d.DeleteVolume(ctx, &entry.volume)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected")
			} else {
				assert.NoError(t, err, "No error is expected.")
			}
			srv.Close()
		})
	}

	t.Run("DeleteVolume_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		err := d.DeleteVolume(ctx, &Volume{Name: "oldVolume"})
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func mockSnapshot(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := []Snapshot{{Name: "SnapShot", SnapshotID: "snapshotID"}}
	createResponse(w, msg, sc)
}

func TestClient_GetSnapshotsForVolume(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		volume          *Volume
		isErrorExpected bool
	}{
		{mockFunction: mockSnapshot, volume: &Volume{Name: "valid Vol"}, isErrorExpected: false},
		{mockFunction: mockUnAuthorizedResponseError, volume: &Volume{Name: "Invalid Vol"}, isErrorExpected: true},
		{mockFunction: mockGetResponseUnMarshalled, volume: &Volume{Name: "Invalid Vol"}, isErrorExpected: true},
	}
	for i, entry := range tests {
		t.Run(fmt.Sprintf("GetSnapshotsForVolume %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			snapShot, err := d.GetSnapshotsForVolume(ctx, entry.volume)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
				assert.Equal(t, (*snapShot)[0].Name, "SnapShot", "Snapshot is not found.")

			}
			srv.Close()
		})
	}

	t.Run("GetSnapshotsForVolume_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.GetSnapshotsForVolume(ctx, &Volume{Name: "oldVolume"})
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func mockSnapshotNoMatch(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := []Snapshot{{Name: "SnapShot1", SnapshotID: "snapshotID"}}
	createResponse(w, msg, sc)
}

func TestClient_GetSnapshotForVolume(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		volume          *Volume
		isErrorExpected bool
	}{
		{mockFunction: mockSnapshot, volume: &Volume{Name: "valid Vol"}, isErrorExpected: false},
		{mockFunction: mockUnAuthorizedResponseError, volume: &Volume{Name: "Invalid Vol"}, isErrorExpected: true},
		{mockFunction: mockSnapshotNoMatch, volume: &Volume{Name: "Invalid Vol"}, isErrorExpected: true},
	}
	for i, entry := range tests {
		t.Run(fmt.Sprintf("GetSnapshotForVolume %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			snapShot, err := d.GetSnapshotForVolume(ctx, entry.volume, "SnapShot")

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
				assert.Equal(t, snapShot.Name, "SnapShot", "Snapshot is not found.")
			}
			srv.Close()
		})
	}

	t.Run("GetSnapshotForVolume_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.GetSnapshotsForVolume(ctx, &Volume{Name: "oldVolume"})
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func mockSnapshotByID(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := Snapshot{Name: "SnapShot", SnapshotID: "snapshotID"}
	createResponse(w, msg, sc)
}

func mockSnapshotByIDWithLSDState(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := Snapshot{
		Name: "SnapShot", SnapshotID: "snapshotID", LifeCycleStateDetails: "LSD detail",
		LifeCycleState: "pending",
	}
	createResponse(w, msg, sc)
}

func TestClient_GetSnapshotByID(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		volume          *Volume
		isErrorExpected bool
	}{
		{mockFunction: mockSnapshotByID, volume: &Volume{Name: "valid Vol"}, isErrorExpected: false},
		{mockFunction: mockUnAuthorizedResponseError, volume: &Volume{Name: "Invalid Vol"}, isErrorExpected: true},
		{mockFunction: mockGetResponseUnMarshalled, volume: &Volume{Name: "Invalid Vol"}, isErrorExpected: true},
	}
	for i, entry := range tests {
		t.Run(fmt.Sprintf("GetSnapshotByID %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			snapShot, err := d.GetSnapshotByID(ctx, "snapshotID")

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
				assert.Equal(t, snapShot.Name, "SnapShot", "Snapshot is not found.")
			}
			srv.Close()
		})
	}

	t.Run("GetSnapshotByID_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.GetSnapshotByID(ctx, "snapshotID")
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func mockSnapshotByIDValidState(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := Snapshot{Name: "SnapShot", SnapshotID: "snapshotID", LifeCycleState: "available"}
	createResponse(w, msg, sc)
}

func TestClient_WaitForSnapshotState(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{mockFunction: mockSnapshotByID, isErrorExpected: true},
		{mockFunction: mockSnapshotByIDWithLSDState, isErrorExpected: true},
		{mockFunction: mockSnapshotByIDValidState, isErrorExpected: false},
		{mockFunction: mockResourceNotPresent, isErrorExpected: true},
		{mockFunction: mockUnAuthorizedResponseError, isErrorExpected: true},
	}

	for i, entry := range tests {
		t.Run(fmt.Sprintf("GetVolumeByID %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)

			err := d.WaitForSnapshotState(ctx,
				&Snapshot{Name: "SnapShot", SnapshotID: "snapshotID", LifeCycleState: ""}, "available",
				[]string{"pending"}, 1*time.Second)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
			}
			srv.Close()
		})
	}
}

func TestClient_CreateSnapshot(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		snapShotRequest SnapshotCreateRequest
		isErrorExpected bool
	}{
		{
			mockFunction: mockSnapshotByID, snapShotRequest: SnapshotCreateRequest{Name: "valid Vol"},
			isErrorExpected: false,
		},
		{
			mockFunction:    mockUnAuthorizedResponseError,
			snapShotRequest: SnapshotCreateRequest{Name: "Invalid Vol"}, isErrorExpected: true,
		},
	}
	for i, entry := range tests {
		t.Run(fmt.Sprintf("CreateSnapshot %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			err := d.CreateSnapshot(ctx, &entry.snapShotRequest)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
			}
			srv.Close()
		})
	}

	t.Run("CreateSnapshot_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		err := d.CreateSnapshot(ctx, &SnapshotCreateRequest{Name: "testSnapshot"})
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func TestClient_RestoreSnapshot(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		snapShot        Snapshot
		isErrorExpected bool
	}{
		{
			mockFunction: mockSnapshotByID, snapShot: Snapshot{Name: "valid Vol"},
			isErrorExpected: false,
		},
		{
			mockFunction: mockUnAuthorizedResponseError,
			snapShot:     Snapshot{Name: "Invalid Vol"}, isErrorExpected: true,
		},
	}
	for i, entry := range tests {
		t.Run(fmt.Sprintf("RestoreSnapshot %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			err := d.RestoreSnapshot(ctx, &Volume{Name: "SnapshotVolume"}, &entry.snapShot)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
			}
			srv.Close()
		})
	}

	t.Run("RestoreSnapshot_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		err := d.RestoreSnapshot(ctx, &Volume{Name: "SnapshotVolume"}, &Snapshot{Name: "testSnapshot"})
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func TestClient_DeleteSnapshot(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		snapShot        Snapshot
		isErrorExpected bool
	}{
		{
			mockFunction: mockSnapshotByID, snapShot: Snapshot{Name: "valid Vol"},
			isErrorExpected: false,
		},
		{
			mockFunction: mockUnAuthorizedResponseError,
			snapShot:     Snapshot{Name: "Invalid Vol"}, isErrorExpected: true,
		},
	}
	for i, entry := range tests {
		t.Run(fmt.Sprintf("DeleteSnapshot %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			err := d.DeleteSnapshot(ctx, &Volume{Name: "SnapshotVolume"}, &entry.snapShot)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
			}
			srv.Close()
		})
	}

	t.Run("DeleteSnapshot_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		err := d.DeleteSnapshot(ctx, &Volume{Name: "SnapshotVolume"}, &Snapshot{Name: "testSnapshot"})
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func mockGetPoolsResponse(w http.ResponseWriter, r *http.Request) {
	sc := http.StatusOK
	msg := []Pool{{Name: "pool1"}, {Name: "pool2"}}
	createResponse(w, msg, sc)
}

func TestClient_GetPools(t *testing.T) {
	tests := []struct {
		mockFunction    func(w http.ResponseWriter, r *http.Request)
		isErrorExpected bool
	}{
		{mockFunction: mockGetPoolsResponse, isErrorExpected: false},
		{mockFunction: mockResourceNotPresent, isErrorExpected: false},
		{mockFunction: mockGetResponseUnMarshalled, isErrorExpected: true},
		{mockFunction: mockResourceNotFound, isErrorExpected: true},
	}

	for i, entry := range tests {
		t.Run(fmt.Sprintf("GetPools: %d", i), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(entry.mockFunction))
			d := getGCPClient(srv.URL)
			_, err := d.GetPools(ctx)

			if entry.isErrorExpected {
				assert.Error(t, err, "An error is expected.")
			} else {
				assert.NoError(t, err, "No error is expected.")
			}
			srv.Close()
		})
	}

	t.Run("GetPools_InvalidToken", func(t *testing.T) {
		d := getGCPClientInvalidToken("srv.URL")
		_, err := d.GetVolumes(ctx)
		assert.Error(t, err, "An error is expected when invalid token is used.")
	})
}

func TestIsValidUserServiceLevel(t *testing.T) {
	tests := map[string]bool{
		UserServiceLevel1: true,
		UserServiceLevel2: true,
		UserServiceLevel3: true,
		"default":         false,
	}

	poolTests := map[string]bool{
		PoolServiceLevel1: true,
		PoolServiceLevel2: true,
		"default":         false,
	}

	for entry := range tests {
		assert.Equal(t, IsValidUserServiceLevel(entry, StorageClassHardware), tests[entry])
	}

	for entry := range poolTests {
		assert.Equal(t, IsValidUserServiceLevel(entry, StorageClassSoftware), poolTests[entry])
	}
}

func TestUserServiceLevelFromAPIServiceLevel(t *testing.T) {
	tests := map[string]string{
		APIServiceLevel1: UserServiceLevel1,
		APIServiceLevel2: UserServiceLevel2,
		APIServiceLevel3: UserServiceLevel3,
		"default":        UserServiceLevel1,
	}

	for entry := range tests {
		assert.Equal(t, UserServiceLevelFromAPIServiceLevel(entry), tests[entry])
	}
}

func TestGCPAPIServiceLevelFromUserServiceLevel(t *testing.T) {
	tests := map[string]string{
		UserServiceLevel1: APIServiceLevel1,
		UserServiceLevel2: APIServiceLevel2,
		UserServiceLevel3: APIServiceLevel3,
		PoolServiceLevel1: APIServiceLevel1,
		PoolServiceLevel2: APIServiceLevel1,
		"default":         APIServiceLevel1,
	}

	for entry := range tests {
		assert.Equal(t, GCPAPIServiceLevelFromUserServiceLevel(entry), tests[entry])
	}
}

func TestIsTransitionalState(t *testing.T) {
	tests := map[string]bool{
		StateCreating:  true,
		StateUpdating:  true,
		StateDeleting:  true,
		StateRestoring: true,
		"other":        false,
	}

	for entry := range tests {
		assert.Equal(t, IsTransitionalState(entry), tests[entry])
	}
}

func TestIsValidStorageClass(t *testing.T) {
	tests := map[string]bool{
		StorageClassHardware: true,
		StorageClassSoftware: true,
		"other":              false,
	}

	for entry := range tests {
		assert.Equal(t, IsValidStorageClass(entry), tests[entry])
	}
}

func TestTerminalState(t *testing.T) {
	err := fmt.Errorf("some error")
	result := TerminalState(err)
	assert.Equal(t, "some error", result.Error(), "Wrong error is returned.")
}
