// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/models"
)

func TestErrorResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    ErrorResponse
		expected string
	}{
		{
			name:     "simple error",
			input:    ErrorResponse{Error: "test error"},
			expected: `{"error":"test error"}`,
		},
		{
			name:     "empty error",
			input:    ErrorResponse{Error: ""},
			expected: `{"error":""}`,
		},
		{
			name:     "error with special characters",
			input:    ErrorResponse{Error: "error with \"quotes\" and \n newlines"},
			expected: `{"error":"error with \"quotes\" and \n newlines"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			jsonData, err := json.Marshal(tt.input)
			require.NoError(t, err)
			assert.JSONEq(t, tt.expected, string(jsonData))

			// Test unmarshaling
			var result ErrorResponse
			err = json.Unmarshal(jsonData, &result)
			require.NoError(t, err)
			assert.Equal(t, tt.input, result)
		})
	}
}

func TestGetBackendResponse(t *testing.T) {
	backend := storage.BackendExternal{
		Name:        "test-backend",
		BackendUUID: "uuid-123",
		Protocol:    "nfs",
		State:       storage.Online,
	}

	tests := []struct {
		name  string
		input GetBackendResponse
	}{
		{
			name: "successful backend response",
			input: GetBackendResponse{
				Backend: backend,
				Error:   "",
			},
		},
		{
			name: "error backend response",
			input: GetBackendResponse{
				Backend: storage.BackendExternal{},
				Error:   "backend not found",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			jsonData, err := json.Marshal(tt.input)
			require.NoError(t, err)
			assert.Contains(t, string(jsonData), "backend")
			assert.Contains(t, string(jsonData), "error")

			// Test unmarshaling
			var result GetBackendResponse
			err = json.Unmarshal(jsonData, &result)
			require.NoError(t, err)
			assert.Equal(t, tt.input.Error, result.Error)
		})
	}
}

func TestMultipleBackendResponse(t *testing.T) {
	backends := []storage.BackendExternal{
		{
			Name:        "backend1",
			BackendUUID: "uuid-1",
			Protocol:    "nfs",
			State:       storage.Online,
		},
		{
			Name:        "backend2",
			BackendUUID: "uuid-2",
			Protocol:    "iscsi",
			State:       storage.Offline,
		},
	}

	response := MultipleBackendResponse{Items: backends}

	// Test marshaling
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "items")

	// Test unmarshaling
	var result MultipleBackendResponse
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Len(t, result.Items, 2)
}

func TestStorageClass(t *testing.T) {
	sc := StorageClass{
		Config: struct {
			Version         string              `json:"version"`
			Name            string              `json:"name"`
			Attributes      interface{}         `json:"attributes"`
			Pools           map[string][]string `json:"storagePools"`
			AdditionalPools map[string][]string `json:"additionalStoragePools"`
		}{
			Version:         "1.0",
			Name:            "test-sc",
			Attributes:      map[string]interface{}{"performance": "high"},
			Pools:           map[string][]string{"backend1": {"pool1", "pool2"}},
			AdditionalPools: map[string][]string{"backend2": {"pool3"}},
		},
		Storage: map[string]interface{}{"type": "ssd"},
	}

	// Test marshaling
	jsonData, err := json.Marshal(sc)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "Config")
	assert.Contains(t, string(jsonData), "storage")

	// Test unmarshaling
	var result StorageClass
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Equal(t, "test-sc", result.Config.Name)
	assert.Equal(t, "1.0", result.Config.Version)
}

func TestGetStorageClassResponse(t *testing.T) {
	sc := StorageClass{
		Config: struct {
			Version         string              `json:"version"`
			Name            string              `json:"name"`
			Attributes      interface{}         `json:"attributes"`
			Pools           map[string][]string `json:"storagePools"`
			AdditionalPools map[string][]string `json:"additionalStoragePools"`
		}{
			Name: "test-sc",
		},
	}

	tests := []struct {
		name  string
		input GetStorageClassResponse
	}{
		{
			name: "successful response",
			input: GetStorageClassResponse{
				StorageClass: sc,
				Error:        "",
			},
		},
		{
			name: "error response",
			input: GetStorageClassResponse{
				StorageClass: StorageClass{},
				Error:        "storage class not found",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			jsonData, err := json.Marshal(tt.input)
			require.NoError(t, err)
			assert.Contains(t, string(jsonData), "storageClass")
			assert.Contains(t, string(jsonData), "error")

			// Test unmarshaling
			var result GetStorageClassResponse
			err = json.Unmarshal(jsonData, &result)
			require.NoError(t, err)
			assert.Equal(t, tt.input.Error, result.Error)
		})
	}
}

func TestMultipleStorageClassResponse(t *testing.T) {
	storageClasses := []StorageClass{
		{
			Config: struct {
				Version         string              `json:"version"`
				Name            string              `json:"name"`
				Attributes      interface{}         `json:"attributes"`
				Pools           map[string][]string `json:"storagePools"`
				AdditionalPools map[string][]string `json:"additionalStoragePools"`
			}{Name: "sc1"},
		},
		{
			Config: struct {
				Version         string              `json:"version"`
				Name            string              `json:"name"`
				Attributes      interface{}         `json:"attributes"`
				Pools           map[string][]string `json:"storagePools"`
				AdditionalPools map[string][]string `json:"additionalStoragePools"`
			}{Name: "sc2"},
		},
	}

	response := MultipleStorageClassResponse{Items: storageClasses}

	// Test marshaling
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "items")

	// Test unmarshaling
	var result MultipleStorageClassResponse
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Len(t, result.Items, 2)
}

func TestMultipleVolumeResponse(t *testing.T) {
	volumes := []storage.VolumeExternal{
		{
			Config: &storage.VolumeConfig{
				Name: "volume1",
				Size: "1Gi",
			},
		},
		{
			Config: &storage.VolumeConfig{
				Name: "volume2",
				Size: "2Gi",
			},
		},
	}

	response := MultipleVolumeResponse{Items: volumes}

	// Test marshaling
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "items")

	// Test unmarshaling
	var result MultipleVolumeResponse
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Len(t, result.Items, 2)
}

func TestMultipleVolumePublicationResponse(t *testing.T) {
	publications := []models.VolumePublicationExternal{
		{
			Name:       "pub1",
			VolumeName: "vol1",
			NodeName:   "node1",
			ReadOnly:   false,
			AccessMode: 1,
		},
		{
			Name:       "pub2",
			VolumeName: "vol2",
			NodeName:   "node2",
			ReadOnly:   true,
			AccessMode: 2,
		},
	}

	response := MultipleVolumePublicationResponse{Items: publications}

	// Test marshaling
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "items")

	// Test unmarshaling
	var result MultipleVolumePublicationResponse
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Len(t, result.Items, 2)
}

func TestMultipleNodeResponse(t *testing.T) {
	nodes := []models.NodeExternal{
		{
			Name: "node1",
			IQN:  "iqn.1993-08.org.debian:01:node1",
			IPs:  []string{"192.168.1.10"},
		},
		{
			Name: "node2",
			IQN:  "iqn.1993-08.org.debian:01:node2",
			IPs:  []string{"192.168.1.11"},
		},
	}

	response := MultipleNodeResponse{Items: nodes}

	// Test marshaling
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "items")

	// Test unmarshaling
	var result MultipleNodeResponse
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Len(t, result.Items, 2)
}

func TestMultipleSnapshotResponse(t *testing.T) {
	snapshots := []storage.SnapshotExternal{
		{
			Snapshot: storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name:       "snap1",
					VolumeName: "vol1",
				},
				Created:   "2023-01-01T00:00:00Z",
				SizeBytes: 1024,
				State:     storage.SnapshotStateOnline,
			},
		},
		{
			Snapshot: storage.Snapshot{
				Config: &storage.SnapshotConfig{
					Name:       "snap2",
					VolumeName: "vol2",
				},
				Created:   "2023-01-02T00:00:00Z",
				SizeBytes: 2048,
				State:     storage.SnapshotStateOnline,
			},
		},
	}

	response := MultipleSnapshotResponse{Items: snapshots}

	// Test marshaling
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "items")

	// Test unmarshaling
	var result MultipleSnapshotResponse
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Len(t, result.Items, 2)
}

func TestMultipleGroupSnapshotResponse(t *testing.T) {
	groupSnapshots := []storage.GroupSnapshotExternal{
		{
			GroupSnapshot: storage.GroupSnapshot{
				GroupSnapshotConfig: &storage.GroupSnapshotConfig{
					Name:         "group-snap1",
					InternalName: "group-snap1-internal",
					VolumeNames:  []string{"vol1", "vol2"},
				},
				SnapshotIDs: []string{"vol1/snap1", "vol2/snap1"},
				Created:     "2023-01-01T00:00:00Z",
			},
		},
		{
			GroupSnapshot: storage.GroupSnapshot{
				GroupSnapshotConfig: &storage.GroupSnapshotConfig{
					Name:         "group-snap2",
					InternalName: "group-snap2-internal",
					VolumeNames:  []string{"vol3", "vol4"},
				},
				SnapshotIDs: []string{"vol3/snap2", "vol4/snap2"},
				Created:     "2023-01-02T00:00:00Z",
			},
		},
	}

	response := MultipleGroupSnapshotResponse{Items: groupSnapshots}

	// Test marshaling
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "items")

	// Test unmarshaling
	var result MultipleGroupSnapshotResponse
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Len(t, result.Items, 2)
}

func TestVersion(t *testing.T) {
	version := Version{
		Version:       "25.10.0",
		MajorVersion:  25,
		MinorVersion:  10,
		PatchVersion:  0,
		PreRelease:    "rc1",
		BuildMetadata: "build123",
		APIVersion:    "1.0",
		GoVersion:     "1.21.0",
	}

	// Test marshaling
	jsonData, err := json.Marshal(version)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "version")
	assert.Contains(t, string(jsonData), "majorVersion")
	assert.Contains(t, string(jsonData), "minorVersion")

	// Test unmarshaling
	var result Version
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Equal(t, version, result)
}

func TestVersionResponse(t *testing.T) {
	serverVersion := &Version{
		Version:      "25.10.0",
		MajorVersion: 25,
		MinorVersion: 10,
		PatchVersion: 0,
	}

	clientVersion := &Version{
		Version:      "25.10.0",
		MajorVersion: 25,
		MinorVersion: 10,
		PatchVersion: 0,
	}

	acpVersion := &Version{
		Version:      "1.0.0",
		MajorVersion: 1,
		MinorVersion: 0,
		PatchVersion: 0,
	}

	tests := []struct {
		name  string
		input VersionResponse
	}{
		{
			name: "complete version response",
			input: VersionResponse{
				Server:    serverVersion,
				Client:    clientVersion,
				ACPServer: acpVersion,
			},
		},
		{
			name: "version response without ACP",
			input: VersionResponse{
				Server: serverVersion,
				Client: clientVersion,
			},
		},
		{
			name: "minimal version response",
			input: VersionResponse{
				Server: &Version{Version: "1.0.0"},
				Client: &Version{Version: "1.0.0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			jsonData, err := json.Marshal(tt.input)
			require.NoError(t, err)
			assert.Contains(t, string(jsonData), "server")
			assert.Contains(t, string(jsonData), "client")

			// Test unmarshaling
			var result VersionResponse
			err = json.Unmarshal(jsonData, &result)
			require.NoError(t, err)

			if tt.input.Server != nil {
				assert.Equal(t, tt.input.Server.Version, result.Server.Version)
			}
			if tt.input.Client != nil {
				assert.Equal(t, tt.input.Client.Version, result.Client.Version)
			}
			if tt.input.ACPServer != nil {
				assert.Equal(t, tt.input.ACPServer.Version, result.ACPServer.Version)
			}
		})
	}
}

func TestClientVersionResponse(t *testing.T) {
	clientVersion := Version{
		Version:       "25.10.0",
		MajorVersion:  25,
		MinorVersion:  10,
		PatchVersion:  0,
		PreRelease:    "",
		BuildMetadata: "",
		APIVersion:    "1.0",
		GoVersion:     "1.21.0",
	}

	response := ClientVersionResponse{Client: clientVersion}

	// Test marshaling
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "client")

	// Test unmarshaling
	var result ClientVersionResponse
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Equal(t, clientVersion, result.Client)
}

func TestMetadata(t *testing.T) {
	tests := []struct {
		name  string
		input Metadata
	}{
		{
			name:  "metadata with name",
			input: Metadata{Name: "test-metadata"},
		},
		{
			name:  "empty metadata",
			input: Metadata{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test marshaling
			jsonData, err := json.Marshal(tt.input)
			require.NoError(t, err)

			// Test unmarshaling
			var result Metadata
			err = json.Unmarshal(jsonData, &result)
			require.NoError(t, err)
			assert.Equal(t, tt.input, result)
		})
	}
}

func TestKubernetesNamespace(t *testing.T) {
	namespace := KubernetesNamespace{
		APIVersion: "v1",
		Kind:       "Namespace",
		Metadata:   Metadata{Name: "trident"},
	}

	// Test marshaling
	jsonData, err := json.Marshal(namespace)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "apiVersion")
	assert.Contains(t, string(jsonData), "kind")
	assert.Contains(t, string(jsonData), "metadata")

	// Test unmarshaling
	var result KubernetesNamespace
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Equal(t, namespace, result)
}

func TestCRStatus(t *testing.T) {
	status := CRStatus{
		Status:  "Ready",
		Message: "Custom resource is ready",
	}

	// Test marshaling
	jsonData, err := json.Marshal(status)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "status")
	assert.Contains(t, string(jsonData), "message")

	// Test unmarshaling
	var result CRStatus
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Equal(t, status, result)
}

func TestOperatorPhaseStatus(t *testing.T) {
	// Test all operator phase status constants
	assert.Equal(t, OperatorPhaseStatus("Done"), OperatorPhaseDone)
	assert.Equal(t, OperatorPhaseStatus("Processing"), OperatorPhaseProcessing)
	assert.Equal(t, OperatorPhaseStatus("Unknown"), OperatorPhaseUnknown)
	assert.Equal(t, OperatorPhaseStatus("Failed"), OperatorPhaseFailed)
	assert.Equal(t, OperatorPhaseStatus("Error"), OperatorPhaseError)

	// Test string conversion
	assert.Equal(t, "Done", string(OperatorPhaseDone))
	assert.Equal(t, "Processing", string(OperatorPhaseProcessing))
	assert.Equal(t, "Unknown", string(OperatorPhaseUnknown))
	assert.Equal(t, "Failed", string(OperatorPhaseFailed))
	assert.Equal(t, "Error", string(OperatorPhaseError))
}

func TestOperatorStatus(t *testing.T) {
	torcStatus := map[string]CRStatus{
		"torc1": {Status: "Ready", Message: "TORC is ready"},
		"torc2": {Status: "Processing", Message: "TORC is processing"},
	}

	tconfStatus := map[string]CRStatus{
		"tconf1": {Status: "Ready", Message: "TCONF is ready"},
	}

	operatorStatus := OperatorStatus{
		ErrorMessage: "No errors",
		Status:       "Ready",
		TorcStatus:   torcStatus,
		TconfStatus:  tconfStatus,
	}

	// Test marshaling
	jsonData, err := json.Marshal(operatorStatus)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "errorMessage")
	assert.Contains(t, string(jsonData), "operatorStatus")
	assert.Contains(t, string(jsonData), "torcStatus")
	assert.Contains(t, string(jsonData), "tconfStatus")

	// Test unmarshaling
	var result OperatorStatus
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Equal(t, operatorStatus.ErrorMessage, result.ErrorMessage)
	assert.Equal(t, operatorStatus.Status, result.Status)
	assert.Len(t, result.TorcStatus, 2)
	assert.Len(t, result.TconfStatus, 1)
}

func TestOperatorStatus_EmptyMaps(t *testing.T) {
	operatorStatus := OperatorStatus{
		ErrorMessage: "Test error",
		Status:       "Failed",
		TorcStatus:   map[string]CRStatus{},
		TconfStatus:  map[string]CRStatus{},
	}

	// Test marshaling
	jsonData, err := json.Marshal(operatorStatus)
	require.NoError(t, err)

	// Test unmarshaling
	var result OperatorStatus
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Equal(t, operatorStatus, result)
}

func TestOperatorStatus_NilMaps(t *testing.T) {
	operatorStatus := OperatorStatus{
		ErrorMessage: "Test error",
		Status:       "Failed",
		TorcStatus:   nil,
		TconfStatus:  nil,
	}

	// Test marshaling
	jsonData, err := json.Marshal(operatorStatus)
	require.NoError(t, err)

	// Test unmarshaling
	var result OperatorStatus
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)
	assert.Equal(t, "Test error", result.ErrorMessage)
	assert.Equal(t, "Failed", result.Status)
	// Note: nil maps become empty maps after JSON round-trip
}
