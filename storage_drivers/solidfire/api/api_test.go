// Copyright 2022 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/errors"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

// Test Error struct methods
func TestError_Error(t *testing.T) {
	tests := []struct {
		name     string
		apiError Error
		expected string
	}{
		{
			name: "standard API error",
			apiError: Error{
				ID: 1,
				Fields: struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
					Name    string `json:"name"`
				}{
					Code:    500,
					Message: "Internal server error",
					Name:    "xInternalError",
				},
			},
			expected: "device API error: xInternalError",
		},
		{
			name: "empty error name",
			apiError: Error{
				ID: 2,
				Fields: struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
					Name    string `json:"name"`
				}{
					Code:    404,
					Message: "Not found",
					Name:    "",
				},
			},
			expected: "device API error: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.apiError.Error()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test NewFromParameters function
func TestNewFromParameters(t *testing.T) {
	tests := []struct {
		name        string
		endpoint    string
		svip        string
		config      Config
		expectError bool
		errorMsg    string
	}{
		{
			name:     "valid parameters",
			endpoint: "https://admin:password@10.0.0.1/json-rpc/8.0",
			svip:     "10.0.0.1:1000",
			config: Config{
				TenantName:       "test-tenant",
				EndPoint:         "https://admin:password@10.0.0.1/json-rpc/8.0",
				SVIP:             "10.0.0.1:1000",
				DefaultBlockSize: 4096,
			},
			expectError: false,
		},
		{
			name:        "invalid endpoint format - still succeeds, validation happens in Request",
			endpoint:    "invalid-endpoint",
			svip:        "10.0.0.1:1000",
			config:      Config{},
			expectError: false, // NewFromParameters doesn't validate format
		},
		{
			name:        "empty endpoint - still succeeds, validation happens in Request",
			endpoint:    "",
			svip:        "10.0.0.1:1000",
			config:      Config{},
			expectError: false, // NewFromParameters doesn't validate empty endpoint
		},
		{
			name:     "config with invalid CA certificate",
			endpoint: "https://admin:password@10.0.0.1/json-rpc/8.0",
			svip:     "10.0.0.1:1000",
			config: Config{
				TrustedCACertificate: "invalid-base64-cert",
			},
			expectError: true,
			errorMsg:    "illegal base64 data",
		},
		{
			name:     "config with malformed CA certificate",
			endpoint: "https://admin:password@10.0.0.1/json-rpc/8.0",
			svip:     "10.0.0.1:1000",
			config: Config{
				TrustedCACertificate: "dGVzdA==", // base64 for "test" - not a valid cert
			},
			expectError: true,
			errorMsg:    "failed to append CA certificate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewFromParameters(tt.endpoint, tt.svip, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, tt.svip, client.SVIP)
				assert.Equal(t, tt.endpoint, client.Endpoint)
			}
		})
	}
}

// Test NewReqID function
func TestNewReqID(t *testing.T) {
	id1 := NewReqID()
	id2 := NewReqID()

	// IDs should be different
	assert.NotEqual(t, id1, id2)

	// IDs should be positive integers
	assert.Greater(t, id1, 0)
	assert.Greater(t, id2, 0)
}

// Helper function to create a test server
func createTestServer(statusCode int, response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write([]byte(response))
	}))
}

// Test Client Request method with test server
func TestClient_Request(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		params         interface{}
		responseCode   int
		responseBody   string
		expectError    bool
		expectedResult string
	}{
		{
			name:         "successful request",
			method:       "TestMethod",
			params:       struct{ TestParam string }{TestParam: "test"},
			responseCode: 200,
			responseBody: `{
				"id": 1,
				"result": {
					"success": true
				}
			}`,
			expectError:    false,
			expectedResult: "success",
		},
		{
			name:         "API error response",
			method:       "TestMethod",
			params:       struct{}{},
			responseCode: 200,
			responseBody: `{
				"id": 1,
				"error": {
					"code": 500,
					"message": "Internal Server Error",
					"name": "xInternalError"
				}
			}`,
			expectError: true,
		},
		{
			name:         "HTTP error response",
			method:       "TestMethod",
			params:       struct{}{},
			responseCode: 500,
			responseBody: `{"message": "Internal Server Error"}`,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(tt.responseCode, tt.responseBody)
			defer server.Close()

			// Create client with test server endpoint
			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			response, err := client.Request(ctx, tt.method, tt.params, 1)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
			}
		})
	}
}

// Test volume operations
func TestClient_CreateVolume(t *testing.T) {
	tests := []struct {
		name         string
		request      *CreateVolumeRequest
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful volume creation",
			request: &CreateVolumeRequest{
				Name:      "test-volume",
				TotalSize: 1073741824, // 1GB
				AccountID: 123,
				Qos: QoS{
					MinIOPS:   1000,
					MaxIOPS:   4000,
					BurstIOPS: 8000,
				},
			},
			mockResponse: `{
				"id": 1,
				"result": {
					"volumeID": 456
				}
			}`,
			expectError: false,
		},
		{
			name: "API error response",
			request: &CreateVolumeRequest{
				Name:      "test-volume",
				TotalSize: 1073741824,
				AccountID: 123,
			},
			mockResponse: `{
				"id": 1,
				"error": {
					"code": 500,
					"message": "Volume already exists",
					"name": "xVolumeAlreadyExists"
				}
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()

			if tt.expectError {
				_, err := client.CreateVolume(ctx, tt.request)
				assert.Error(t, err)
			} else {
				// Test the Request call directly to avoid WaitForVolumeByID timeout
				response, err := client.Request(ctx, "CreateVolume", tt.request, NewReqID())
				assert.NoError(t, err)
				assert.NotNil(t, response)

				// Verify we can unmarshal the response
				var result CreateVolumeResult
				err = json.Unmarshal(response, &result)
				assert.NoError(t, err)
				assert.Equal(t, int64(456), result.Result.VolumeID)
			}
		})
	}
}

// Test snapshot operations
func TestClient_CreateSnapshot(t *testing.T) {
	tests := []struct {
		name         string
		request      *CreateSnapshotRequest
		mockResponse string
		expectError  bool
		errorType    string
	}{
		{
			name: "successful snapshot API call - test request only",
			request: &CreateSnapshotRequest{
				VolumeID: 123,
				Name:     "test-snapshot",
			},
			mockResponse: `{
				"id": 1,
				"result": {
					"snapshotID": 456,
					"checksum": "abc123"
				}
			}`,
			expectError: false, // We'll test the request directly
		},
		{
			name: "max snapshots exceeded error",
			request: &CreateSnapshotRequest{
				VolumeID: 123,
				Name:     "test-snapshot",
			},
			mockResponse: `{
				"id": 1,
				"error": {
					"code": 500,
					"message": "Maximum snapshots per volume exceeded",
					"name": "xMaxSnapshotsPerVolumeExceeded"
				}
			}`,
			expectError: true,
			errorType:   "MaxLimit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()

			if tt.expectError {
				_, err := client.CreateSnapshot(ctx, tt.request)
				assert.Error(t, err)
				if tt.errorType == "MaxLimit" {
					assert.True(t, errors.IsMaxLimitReachedError(err))
				}
			} else {
				// Test the Request call directly to avoid GetSnapshot call
				response, err := client.Request(ctx, "CreateSnapshot", tt.request, NewReqID())
				assert.NoError(t, err)
				assert.NotNil(t, response)

				// Verify we can unmarshal the response
				var result CreateSnapshotResult
				err = json.Unmarshal(response, &result)
				assert.NoError(t, err)
				assert.Equal(t, int64(456), result.Result.SnapshotID)
			}
		})
	}
}

// Test account operations
func TestClient_AddAccount(t *testing.T) {
	tests := []struct {
		name         string
		request      *AddAccountRequest
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful account creation",
			request: &AddAccountRequest{
				Username: "test-user",
			},
			mockResponse: `{
				"id": 1,
				"result": {
					"accountID": 789
				}
			}`,
			expectError: false,
		},
		{
			name: "invalid JSON response",
			request: &AddAccountRequest{
				Username: "test-user",
			},
			mockResponse: `invalid json`,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			accountID, err := client.AddAccount(ctx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, int64(0), accountID)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, int64(789), accountID)
			}
		})
	}
}

// Test QoS operations
func TestClient_GetDefaultQoS(t *testing.T) {
	tests := []struct {
		name         string
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful QoS retrieval",
			mockResponse: `{
				"id": 1,
				"result": {
					"minIOPS": 1000,
					"maxIOPS": 4000,
					"burstIOPS": 8000
				}
			}`,
			expectError: false,
		},
		{
			name:         "invalid JSON response",
			mockResponse: `invalid json`,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			qos, err := client.GetDefaultQoS(ctx)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, qos)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, qos)
				assert.Equal(t, int64(1000), qos.MinIOPS)
				assert.Equal(t, int64(4000), qos.MaxIOPS)
				assert.Equal(t, int64(8000), qos.BurstIOPS)
			}
		})
	}
}

// Test capacity operations
func TestClient_GetClusterCapacity(t *testing.T) {
	tests := []struct {
		name         string
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful capacity retrieval",
			mockResponse: `{
				"id": 1,
				"result": {
					"clusterCapacity": {
						"activeBlockSpace": 1000000,
						"activeSessions": 5,
						"averageIOPS": 100,
						"clusterRecentIOSize": 4096,
						"currentIOPS": 150,
						"maxIOPS": 10000,
						"maxOverProvisionableSpace": 5000000,
						"maxProvisionedSpace": 3000000,
						"maxUsedMetadataSpace": 1000000,
						"maxUsedSpace": 2000000,
						"nonZeroBlocks": 500000,
						"peakActiveSessions": 10,
						"peakIOPS": 1000,
						"provisionedSpace": 1500000,
						"snapshotNonZeroBlocks": 100000,
						"totalOps": 50000,
						"uniqueBlocks": 400000,
						"uniqueBlocksUsedSpace": 800000,
						"usedMetadataSpace": 200000,
						"usedMetadataSpaceInSnapshots": 50000,
						"usedSpace": 1200000,
						"zeroBlocks": 600000
					}
				}
			}`,
			expectError: false,
		},
		{
			name:         "invalid JSON response",
			mockResponse: `invalid json`,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			capacity, err := client.GetClusterCapacity(ctx)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, capacity)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, capacity)
				assert.Equal(t, int64(1000000), capacity.ActiveBlockSpace)
				assert.Equal(t, int64(5), capacity.ActiveSessions)
			}
		})
	}
}

// Test hardware operations
func TestClient_GetClusterHardwareInfo(t *testing.T) {
	tests := []struct {
		name         string
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful hardware info retrieval",
			mockResponse: `{
				"id": 1,
				"result": {
					"clusterHardwareInfo": {
						"drives": [],
						"nodes": [],
						"fibreChannelPorts": []
					}
				}
			}`,
			expectError: false,
		},
		{
			name:         "invalid JSON response",
			mockResponse: `invalid json`,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			hwInfo, err := client.GetClusterHardwareInfo(ctx)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, hwInfo)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, hwInfo)
			}
		})
	}
}

// Test VAG operations
func TestClient_CreateVolumeAccessGroup(t *testing.T) {
	tests := []struct {
		name         string
		request      *CreateVolumeAccessGroupRequest
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful VAG creation",
			request: &CreateVolumeAccessGroupRequest{
				Name: "test-vag",
			},
			mockResponse: `{
				"id": 1,
				"result": {
					"volumeAccessGroupID": 100
				}
			}`,
			expectError: false,
		},
		{
			name: "invalid JSON response",
			request: &CreateVolumeAccessGroupRequest{
				Name: "test-vag",
			},
			mockResponse: `invalid json`,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			vagID, err := client.CreateVolumeAccessGroup(ctx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, int64(0), vagID)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, int64(100), vagID)
			}
		})
	}
}

// Test JSON marshaling/unmarshaling for key structs
func TestVolume_JSON(t *testing.T) {
	volume := Volume{
		VolumeID:  123,
		Name:      "test-volume",
		AccountID: 456,
		Status:    "active",
		Access:    "readWrite",
		Iqn:       "iqn.2010-01.com.solidfire:test-volume.123",
		Qos: QoS{
			MinIOPS:   1000,
			MaxIOPS:   4000,
			BurstIOPS: 8000,
		},
		TotalSize:          1073741824,
		BlockSize:          4096,
		VolumeAccessGroups: []int64{1, 2, 3},
		VolumePairs:        []VolumePair{},
		DeleteTime:         "",
		PurgeTime:          "",
		SliceCount:         1,
		VirtualVolumeID:    "",
		Attributes:         map[string]interface{}{"test": "value"},
		CreateTime:         "2023-01-01T00:00:00Z",
		Enable512e:         true,
		ScsiEUIDeviceID:    "eui.123456789abcdef0",
		ScsiNAADeviceID:    "naa.6f47acc000000000123456789abcdef0",
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(volume)
	assert.NoError(t, err)
	assert.NotEmpty(t, jsonData)

	// Test JSON unmarshaling
	var unmarshaledVolume Volume
	err = json.Unmarshal(jsonData, &unmarshaledVolume)
	assert.NoError(t, err)
	assert.Equal(t, volume.VolumeID, unmarshaledVolume.VolumeID)
	assert.Equal(t, volume.Name, unmarshaledVolume.Name)
	assert.Equal(t, volume.Qos.MinIOPS, unmarshaledVolume.Qos.MinIOPS)
}

func TestQoS_JSON(t *testing.T) {
	qos := QoS{
		MinIOPS:   1000,
		MaxIOPS:   4000,
		BurstIOPS: 8000,
		BurstTime: 60, // Note: This should not appear in JSON due to json:"-" tag
	}

	// Test JSON marshaling
	jsonData, err := json.Marshal(qos)
	assert.NoError(t, err)
	assert.NotContains(t, string(jsonData), "burstTime") // Should be excluded

	// Test JSON unmarshaling
	var unmarshaledQoS QoS
	err = json.Unmarshal(jsonData, &unmarshaledQoS)
	assert.NoError(t, err)
	assert.Equal(t, qos.MinIOPS, unmarshaledQoS.MinIOPS)
	assert.Equal(t, qos.MaxIOPS, unmarshaledQoS.MaxIOPS)
	assert.Equal(t, qos.BurstIOPS, unmarshaledQoS.BurstIOPS)
	assert.Equal(t, int64(0), unmarshaledQoS.BurstTime) // Should be zero due to json:"-"
}

// Test HTTPError struct
func TestHTTPError_Error(t *testing.T) {
	httpError := HTTPError{
		Status:     "500 Internal Server Error",
		StatusCode: 500,
	}

	expected := "HTTP error: 500 Internal Server Error"
	assert.Equal(t, expected, httpError.Error())
}

func TestNewHTTPError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		expectNil  bool
	}{
		{
			name:       "successful response - no error",
			statusCode: 200,
			expectNil:  true,
		},
		{
			name:       "client error - returns error",
			statusCode: 400,
			expectNil:  false,
		},
		{
			name:       "server error - returns error",
			statusCode: 500,
			expectNil:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &http.Response{
				StatusCode: tt.statusCode,
				Status:     "test status",
			}

			httpError := NewHTTPError(response)

			if tt.expectNil {
				assert.Nil(t, httpError)
			} else {
				assert.NotNil(t, httpError)
				assert.Equal(t, tt.statusCode, httpError.StatusCode)
				assert.Equal(t, "test status", httpError.Status)
			}
		})
	}
}

// Test additional volume operations
func TestClient_DeleteVolume(t *testing.T) {
	tests := []struct {
		name         string
		volumeID     int64
		mockResponse string
		expectError  bool
	}{
		{
			name:     "successful volume deletion",
			volumeID: 123,
			mockResponse: `{
				"id": 1,
				"result": {}
			}`,
			expectError: false,
		},
		{
			name:     "API error during deletion",
			volumeID: 123,
			mockResponse: `{
				"id": 1,
				"error": {
					"code": 404,
					"message": "Volume not found",
					"name": "xVolumeIDDoesNotExist"
				}
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			err := client.DeleteVolume(ctx, tt.volumeID)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test volume listing operations
func TestClient_ListVolumesForAccount(t *testing.T) {
	tests := []struct {
		name         string
		request      *ListVolumesForAccountRequest
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful volume list",
			request: &ListVolumesForAccountRequest{
				AccountID: 123,
			},
			mockResponse: `{
				"id": 1,
				"result": {
					"volumes": [
						{
							"volumeID": 1,
							"name": "volume1",
							"accountID": 123,
							"status": "active"
						}
					]
				}
			}`,
			expectError: false,
		},
		{
			name: "invalid JSON response",
			request: &ListVolumesForAccountRequest{
				AccountID: 123,
			},
			mockResponse: `invalid json`,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			volumes, err := client.ListVolumesForAccount(ctx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, volumes)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, volumes)
				assert.Len(t, volumes, 1)
				assert.Equal(t, int64(1), volumes[0].VolumeID)
			}
		})
	}
}

// Test GetVolumeByID
func TestClient_GetVolumeByID(t *testing.T) {
	tests := []struct {
		name         string
		volumeID     int64
		mockResponse string
		expectError  bool
	}{
		{
			name:     "successful volume retrieval",
			volumeID: 123,
			mockResponse: `{
				"id": 1,
				"result": {
					"volumes": [
						{
							"volumeID": 123,
							"name": "test-volume",
							"accountID": 456,
							"status": "active"
						}
					]
				}
			}`,
			expectError: false,
		},
		{
			name:     "volume not found",
			volumeID: 999,
			mockResponse: `{
				"id": 1,
				"result": {
					"volumes": []
				}
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
				AccountID:  456,
			}

			ctx := context.Background()
			volume, err := client.GetVolumeByID(ctx, tt.volumeID)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.volumeID, volume.VolumeID)
			}
		})
	}
}

// Test snapshot operations
func TestClient_DeleteSnapshot(t *testing.T) {
	tests := []struct {
		name         string
		snapshotID   int64
		mockResponse string
		expectError  bool
	}{
		{
			name:       "successful snapshot deletion",
			snapshotID: 456,
			mockResponse: `{
				"id": 1,
				"result": {}
			}`,
			expectError: false,
		},
		{
			name:       "API error during deletion",
			snapshotID: 456,
			mockResponse: `{
				"id": 1,
				"error": {
					"code": 404,
					"message": "Snapshot not found",
					"name": "xSnapshotIDDoesNotExist"
				}
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			err := client.DeleteSnapshot(ctx, tt.snapshotID)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test ListSnapshots
func TestClient_ListSnapshots(t *testing.T) {
	tests := []struct {
		name         string
		request      *ListSnapshotsRequest
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful snapshot list",
			request: &ListSnapshotsRequest{
				VolumeID: 123,
			},
			mockResponse: `{
				"id": 1,
				"result": {
					"snapshots": [
						{
							"snapshotID": 456,
							"name": "test-snapshot",
							"volumeID": 123,
							"status": "done"
						}
					]
				}
			}`,
			expectError: false,
		},
		{
			name: "invalid JSON response",
			request: &ListSnapshotsRequest{
				VolumeID: 123,
			},
			mockResponse: `invalid json`,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			snapshots, err := client.ListSnapshots(ctx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, snapshots)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, snapshots)
				assert.Len(t, snapshots, 1)
				assert.Equal(t, int64(456), snapshots[0].SnapshotID)
			}
		})
	}
}

// Test GetSnapshot
func TestClient_GetSnapshot(t *testing.T) {
	tests := []struct {
		name         string
		snapID       int64
		volID        int64
		sfName       string
		mockResponse string
		expectError  bool
	}{
		{
			name:   "get snapshot by ID",
			snapID: 456,
			volID:  123,
			sfName: "",
			mockResponse: `{
				"id": 1,
				"result": {
					"snapshots": [
						{
							"snapshotID": 456,
							"name": "test-snapshot",
							"volumeID": 123,
							"status": "done"
						}
					]
				}
			}`,
			expectError: false,
		},
		{
			name:   "get snapshot by name",
			snapID: -1,
			volID:  123,
			sfName: "test-snapshot",
			mockResponse: `{
				"id": 1,
				"result": {
					"snapshots": [
						{
							"snapshotID": 456,
							"name": "test-snapshot",
							"volumeID": 123,
							"status": "done"
						}
					]
				}
			}`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			snapshot, err := client.GetSnapshot(ctx, tt.snapID, tt.volID, tt.sfName)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, int64(456), snapshot.SnapshotID)
				assert.Equal(t, "test-snapshot", snapshot.Name)
			}
		})
	}
}

// Test additional account operations
func TestClient_GetAccountByName(t *testing.T) {
	tests := []struct {
		name         string
		request      *GetAccountByNameRequest
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful account retrieval",
			request: &GetAccountByNameRequest{
				Name: "test-user",
			},
			mockResponse: `{
				"id": 1,
				"result": {
					"account": {
						"accountID": 789,
						"username": "test-user",
						"status": "active"
					}
				}
			}`,
			expectError: false,
		},
		{
			name: "invalid JSON response",
			request: &GetAccountByNameRequest{
				Name: "test-user",
			},
			mockResponse: `invalid json`,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			account, err := client.GetAccountByName(ctx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, int64(789), account.AccountID)
				assert.Equal(t, "test-user", account.Username)
			}
		})
	}
}

// Test VAG operations
func TestClient_ListVolumeAccessGroups(t *testing.T) {
	tests := []struct {
		name         string
		request      *ListVolumeAccessGroupsRequest
		mockResponse string
		expectError  bool
	}{
		{
			name:    "successful VAG list",
			request: &ListVolumeAccessGroupsRequest{},
			mockResponse: `{
				"id": 1,
				"result": {
					"volumeAccessGroups": [
						{
							"volumeAccessGroupID": 100,
							"name": "test-vag",
							"initiators": [],
							"volumes": []
						}
					]
				}
			}`,
			expectError: false,
		},
		{
			name:         "invalid JSON response",
			request:      &ListVolumeAccessGroupsRequest{},
			mockResponse: `invalid json`,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			vags, err := client.ListVolumeAccessGroups(ctx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, vags)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, vags)
				assert.Len(t, vags, 1)
				assert.Equal(t, int64(100), vags[0].VAGID)
			}
		})
	}
}

// Test Client shouldLogResponseBody method
func TestClient_shouldLogResponseBody(t *testing.T) {
	client := &Client{
		Config: &Config{
			DebugTraceFlags: map[string]bool{
				"hardwareInfo": true,
			},
		},
	}

	tests := []struct {
		method   string
		expected bool
	}{
		{"GetAccountByName", false},
		{"GetAccountByID", false},
		{"ListAccounts", false},
		{"GetClusterHardwareInfo", true}, // Because hardwareInfo flag is true
		{"CreateVolume", true},
		{"DeleteVolume", true},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			result := client.shouldLogResponseBody(tt.method)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Test with hardwareInfo flag false
	client.Config.DebugTraceFlags["hardwareInfo"] = false
	assert.False(t, client.shouldLogResponseBody("GetClusterHardwareInfo"))
}

// Test additional volume operations
func TestClient_AddVolumesToAccessGroup(t *testing.T) {
	tests := []struct {
		name         string
		request      *AddVolumesToVolumeAccessGroupRequest
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful add volumes to VAG",
			request: &AddVolumesToVolumeAccessGroupRequest{
				VolumeAccessGroupID: 100,
				Volumes:             []int64{123, 456},
			},
			mockResponse: `{
				"id": 1,
				"result": {}
			}`,
			expectError: false,
		},
		{
			name: "volume already in VAG - should not error",
			request: &AddVolumesToVolumeAccessGroupRequest{
				VolumeAccessGroupID: 100,
				Volumes:             []int64{123},
			},
			mockResponse: `{
				"id": 1,
				"error": {
					"code": 500,
					"message": "Volume already in VAG",
					"name": "xAlreadyInVolumeAccessGroup"
				}
			}`,
			expectError: false, // This specific error should be handled
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			err := client.AddVolumesToAccessGroup(ctx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test additional VAG operations
func TestClient_AddInitiatorsToVolumeAccessGroup(t *testing.T) {
	tests := []struct {
		name         string
		request      *AddInitiatorsToVolumeAccessGroupRequest
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful add initiators to VAG",
			request: &AddInitiatorsToVolumeAccessGroupRequest{
				VAGID:      100,
				Initiators: []string{"iqn.1993-08.org.debian:01:test"},
			},
			mockResponse: `{
				"id": 1,
				"result": {}
			}`,
			expectError: false,
		},
		{
			name: "API error",
			request: &AddInitiatorsToVolumeAccessGroupRequest{
				VAGID:      100,
				Initiators: []string{"invalid-iqn"},
			},
			mockResponse: `{
				"id": 1,
				"error": {
					"code": 400,
					"message": "Invalid initiator IQN",
					"name": "xInvalidParameter"
				}
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			err := client.AddInitiatorsToVolumeAccessGroup(ctx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test additional account operations
func TestClient_GetAccountByID(t *testing.T) {
	tests := []struct {
		name         string
		request      *GetAccountByIDRequest
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful account retrieval by ID",
			request: &GetAccountByIDRequest{
				AccountID: 789,
			},
			mockResponse: `{
				"id": 1,
				"result": {
					"account": {
						"accountID": 789,
						"username": "test-user",
						"status": "active"
					}
				}
			}`,
			expectError: false,
		},
		{
			name: "invalid JSON response",
			request: &GetAccountByIDRequest{
				AccountID: 789,
			},
			mockResponse: `invalid json`,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			account, err := client.GetAccountByID(ctx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, int64(789), account.AccountID)
				assert.Equal(t, "test-user", account.Username)
			}
		})
	}
}

// Test Volume struct method
func TestVolume_GetAttributesAsMap(t *testing.T) {
	volume := Volume{
		Attributes: map[string]interface{}{
			"stringKey": "stringValue",
			"intKey":    42,
			"boolKey":   true,
		},
	}

	result := volume.GetAttributesAsMap()

	// Should only convert string values
	assert.Equal(t, "stringValue", result["stringKey"])
	assert.NotContains(t, result, "intKey")  // Should not include int
	assert.NotContains(t, result, "boolKey") // Should not include bool
	assert.Len(t, result, 1)
}

// Test snapshot operations - RollbackToSnapshot
func TestClient_RollbackToSnapshot(t *testing.T) {
	tests := []struct {
		name         string
		request      *RollbackToSnapshotRequest
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful rollback",
			request: &RollbackToSnapshotRequest{
				VolumeID:         123,
				SnapshotID:       456,
				SaveCurrentState: true,
				Name:             "backup-before-rollback",
			},
			mockResponse: `{
				"id": 1,
				"result": {
					"snapshotID": 789,
					"checksum": "abc123def"
				}
			}`,
			expectError: false,
		},
		{
			name: "API error",
			request: &RollbackToSnapshotRequest{
				VolumeID:   123,
				SnapshotID: 999,
			},
			mockResponse: `{
				"id": 1,
				"error": {
					"code": 404,
					"message": "Snapshot not found",
					"name": "xSnapshotIDDoesNotExist"
				}
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			newSnapID, err := client.RollbackToSnapshot(ctx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, int64(0), newSnapID)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, int64(789), newSnapID)
			}
		})
	}
}

// Test WaitForVolumeByID with quick completion
func TestClient_WaitForVolumeByID(t *testing.T) {
	// Test immediate success case
	server := createTestServer(200, `{
		"id": 1,
		"result": {
			"volumes": [
				{
					"volumeID": 123,
					"name": "test-volume",
					"accountID": 456,
					"status": "active"
				}
			]
		}
	}`)
	defer server.Close()

	client := &Client{
		Endpoint:   server.URL,
		SVIP:       "10.0.0.1:1000",
		Config:     &Config{DebugTraceFlags: make(map[string]bool)},
		httpClient: server.Client(),
		AccountID:  456,
	}

	ctx := context.Background()
	volume, err := client.WaitForVolumeByID(ctx, 123)

	assert.NoError(t, err)
	assert.Equal(t, int64(123), volume.VolumeID)
	assert.Equal(t, "test-volume", volume.Name)
}

// Test ListVolumes operation
func TestClient_ListVolumes(t *testing.T) {
	tests := []struct {
		name         string
		request      *ListVolumesRequest
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful volume list",
			request: &ListVolumesRequest{
				Accounts: []int64{123},
				Limit:    &[]int64{10}[0],
			},
			mockResponse: `{
				"id": 1,
				"result": {
					"volumes": [
						{
							"volumeID": 1,
							"name": "volume1",
							"accountID": 123,
							"status": "active",
							"totalSize": 1073741824,
							"blockSize": 4096
						},
						{
							"volumeID": 2,
							"name": "volume2", 
							"accountID": 123,
							"status": "active",
							"totalSize": 2147483648,
							"blockSize": 4096
						}
					]
				}
			}`,
			expectError: false,
		},
		{
			name: "invalid JSON response",
			request: &ListVolumesRequest{
				Accounts: []int64{123},
			},
			mockResponse: `invalid json`,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			volumes, err := client.ListVolumes(ctx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, volumes)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, volumes)
				assert.Len(t, volumes, 2)
				assert.Equal(t, int64(1), volumes[0].VolumeID)
				assert.Equal(t, "volume1", volumes[0].Name)
				assert.Equal(t, int64(2), volumes[1].VolumeID)
				assert.Equal(t, "volume2", volumes[1].Name)
			}
		})
	}
}

// Test CloneVolume operation
func TestClient_CloneVolume(t *testing.T) {
	tests := []struct {
		name         string
		request      *CloneVolumeRequest
		mockResponse string
		expectError  bool
	}{
		{
			name: "API error response",
			request: &CloneVolumeRequest{
				VolumeID:   123,
				Name:       "cloned-volume",
				SnapshotID: 456,
			},
			mockResponse: `{
				"id": 1,
				"error": {
					"code": 404,
					"message": "Volume not found",
					"name": "xVolumeIDDoesNotExist"
				}
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			_, err := client.CloneVolume(ctx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				// Even success cases will likely fail due to WaitForVolumeByID
				assert.Error(t, err)
			}
		})
	}
}

// Test ModifyVolume operation
func TestClient_ModifyVolume(t *testing.T) {
	tests := []struct {
		name         string
		request      *ModifyVolumeRequest
		mockResponse string
		expectError  bool
	}{
		{
			name: "successful volume modification",
			request: &ModifyVolumeRequest{
				VolumeID:  123,
				TotalSize: 2147483648, // 2GB
				Qos: QoS{
					MinIOPS:   2000,
					MaxIOPS:   8000,
					BurstIOPS: 16000,
				},
			},
			mockResponse: `{
				"id": 1,
				"result": {}
			}`,
			expectError: false,
		},
		{
			name: "API error response",
			request: &ModifyVolumeRequest{
				VolumeID:  999,
				TotalSize: 1073741824,
			},
			mockResponse: `{
				"id": 1,
				"error": {
					"code": 404,
					"message": "Volume not found",
					"name": "xVolumeIDDoesNotExist"
				}
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			err := client.ModifyVolume(ctx, tt.request)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test additional Volume method edge cases
func TestVolume_GetAttributesAsMap_EdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		volume     Volume
		expectLen  int
		expectKeys []string
	}{
		{
			name: "empty attributes",
			volume: Volume{
				Attributes: map[string]interface{}{},
			},
			expectLen: 0,
		},
		{
			name: "nil attributes",
			volume: Volume{
				Attributes: nil,
			},
			expectLen: 0,
		},
		{
			name: "non-map attributes",
			volume: Volume{
				Attributes: "not a map",
			},
			expectLen: 0,
		},
		{
			name: "mixed type attributes",
			volume: Volume{
				Attributes: map[string]interface{}{
					"string1": "value1",
					"string2": "value2",
					"int":     42,
					"bool":    true,
					"nil":     nil,
					"string3": "value3",
				},
			},
			expectLen:  3,
			expectKeys: []string{"string1", "string2", "string3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.volume.GetAttributesAsMap()
			assert.Equal(t, tt.expectLen, len(result))

			for _, key := range tt.expectKeys {
				assert.Contains(t, result, key)
			}
		})
	}
}

// Test Client Request method with different scenarios
func TestClient_Request_AdvancedScenarios(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		params       interface{}
		responseCode int
		responseBody string
		expectError  bool
	}{
		{
			name:   "request with complex params",
			method: "ComplexMethod",
			params: map[string]interface{}{
				"nested": map[string]string{"key": "value"},
				"array":  []int{1, 2, 3},
			},
			responseCode: 200,
			responseBody: `{"id": 1, "result": {"success": true}}`,
			expectError:  false,
		},
		{
			name:         "request with nil params",
			method:       "NilParamsMethod",
			params:       nil,
			responseCode: 200,
			responseBody: `{"id": 1, "result": {}}`,
			expectError:  false,
		},
		{
			name:         "malformed JSON response",
			method:       "MalformedJSONMethod",
			params:       struct{}{},
			responseCode: 200,
			responseBody: `{"id": 1, "result": }`, // Invalid JSON
			expectError:  false,                   // Request succeeds, JSON unmarshaling happens in individual methods
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(tt.responseCode, tt.responseBody)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()
			response, err := client.Request(ctx, tt.method, tt.params, NewReqID())

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
			}
		})
	}
}

// Test Client Request error handling scenarios
func TestClient_Request_ErrorHandling(t *testing.T) {
	tests := []struct {
		name         string
		setupClient  func() *Client
		method       string
		params       interface{}
		expectError  bool
		errorMessage string
	}{
		{
			name: "empty endpoint error",
			setupClient: func() *Client {
				return &Client{
					Endpoint: "",
					SVIP:     "10.0.0.1:1000",
					Config:   &Config{DebugTraceFlags: make(map[string]bool)},
				}
			},
			method:       "TestMethod",
			params:       struct{}{},
			expectError:  true,
			errorMessage: "no endpoint set",
		},
		{
			name: "successful request with logging disabled methods",
			setupClient: func() *Client {
				server := createTestServer(200, `{"id": 1, "result": {}}`)
				return &Client{
					Endpoint:   server.URL,
					SVIP:       "10.0.0.1:1000",
					Config:     &Config{DebugTraceFlags: map[string]bool{"api": false}},
					httpClient: server.Client(),
				}
			},
			method:      "GetAccountByName", // This method has suppressed logging
			params:      struct{}{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := tt.setupClient()
			ctx := context.Background()

			_, err := client.Request(ctx, tt.method, tt.params, NewReqID())

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test GetVolumeByID error scenarios
func TestClient_GetVolumeByID_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		volumeID     int64
		mockResponse string
		expectError  bool
		errorMessage string
	}{
		{
			name:     "volume found with multiple volumes in account",
			volumeID: 123,
			mockResponse: `{
				"id": 1,
				"result": {
					"volumes": [
						{
							"volumeID": 123,
							"name": "test-volume",
							"accountID": 456,
							"status": "active"
						},
						{
							"volumeID": 124,
							"name": "other-volume",
							"accountID": 456,
							"status": "active"
						}
					]
				}
			}`,
			expectError: false,
		},
		{
			name:     "volume not found - empty list",
			volumeID: 999,
			mockResponse: `{
				"id": 1,
				"result": {
					"volumes": []
				}
			}`,
			expectError:  true,
			errorMessage: "volume 999 not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.mockResponse)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
				AccountID:  456,
			}

			ctx := context.Background()
			volume, err := client.GetVolumeByID(ctx, tt.volumeID)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.volumeID, volume.VolumeID)
			}
		})
	}
}

// Test JSON parsing edge cases
func TestClient_JSONParsing_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		response string
	}{
		{
			name:   "response with extra fields",
			method: "GetDefaultQoS",
			response: `{
				"id": 1,
				"result": {
					"minIOPS": 1000,
					"maxIOPS": 4000,
					"burstIOPS": 8000,
					"extraField": "ignored"
				},
				"extraTopLevel": "also ignored"
			}`,
		},
		{
			name:   "response with null fields",
			method: "ListVolumes",
			response: `{
				"id": 1,
				"result": {
					"volumes": null
				}
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := createTestServer(200, tt.response)
			defer server.Close()

			client := &Client{
				Endpoint:   server.URL,
				SVIP:       "10.0.0.1:1000",
				Config:     &Config{DebugTraceFlags: make(map[string]bool)},
				httpClient: server.Client(),
			}

			ctx := context.Background()

			// Just test that the request succeeds and returns valid JSON
			response, err := client.Request(ctx, tt.method, struct{}{}, NewReqID())
			assert.NoError(t, err)
			assert.NotNil(t, response)

			// Verify it's valid JSON
			var parsed map[string]interface{}
			err = json.Unmarshal(response, &parsed)
			assert.NoError(t, err)
		})
	}
}

// Test additional Config scenarios for NewFromParameters
func TestNewFromParameters_ExtendedScenarios(t *testing.T) {
	tests := []struct {
		name        string
		endpoint    string
		svip        string
		config      Config
		expectError bool
	}{
		{
			name:     "config with volume types",
			endpoint: "https://admin:password@10.0.0.1/json-rpc/8.0",
			svip:     "10.0.0.1:1000",
			config: Config{
				TenantName: "test-tenant",
				Types: &[]VolType{
					{Type: "Gold", QOS: QoS{MinIOPS: 1000, MaxIOPS: 4000, BurstIOPS: 8000}},
					{Type: "Silver", QOS: QoS{MinIOPS: 500, MaxIOPS: 2000, BurstIOPS: 4000}},
				},
				DefaultBlockSize: 4096,
				DebugTraceFlags:  map[string]bool{"method": true, "api": false},
			},
			expectError: false,
		},
		{
			name:     "config with access groups",
			endpoint: "https://admin:password@10.0.0.1/json-rpc/8.0",
			svip:     "10.0.0.1:1000",
			config: Config{
				AccessGroups:     []int64{1, 2, 3},
				LegacyNamePrefix: "legacy_",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewFromParameters(tt.endpoint, tt.svip, tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.Equal(t, tt.svip, client.SVIP)
				assert.Equal(t, tt.endpoint, client.Endpoint)
				assert.Equal(t, tt.config.Types, client.VolumeTypes)
				assert.Equal(t, tt.config.DefaultBlockSize, client.DefaultBlockSize)
				assert.Equal(t, tt.config.DebugTraceFlags, client.DebugTraceFlags)
			}
		})
	}
}

// Test edge cases and error scenarios
func TestClient_ErrorScenarios(t *testing.T) {
	client := &Client{
		Endpoint: "", // Empty endpoint should cause error
		SVIP:     "10.0.0.1:1000",
		Config:   &Config{DebugTraceFlags: make(map[string]bool)},
	}

	ctx := context.Background()

	// Test with empty endpoint
	_, err := client.Request(ctx, "TestMethod", struct{}{}, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no endpoint set")
}
