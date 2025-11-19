// Copyright 2025 NetApp, Inc. All Rights Reserved.

package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	http_test "github.com/stretchr/testify/http"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/acp"
	"github.com/netapp/trident/frontend"
	mockacp "github.com/netapp/trident/mocks/mock_acp"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	mockk8scontrollerhelper "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_helpers/mock_kubernetes_helper"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
	"github.com/netapp/trident/utils/version"
)

func generateHTTPRequest(method, body string) *http.Request {
	return &http.Request{
		Method: method,
		Body:   io.NopCloser(strings.NewReader(body)),
	}
}

func TestVolumeLUKSPassphraseNamesUpdater(t *testing.T) {
	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Volume found, replace "/config/luksPassphraseNames" with empty list
	volume := &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test"}}
	writer := &http_test.TestResponseWriter{}
	response := &UpdateVolumeResponse{}
	body := `[]`
	request := generateHTTPRequest(http.MethodPut, body)

	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator = mockOrchestrator
	mockOrchestrator.EXPECT().GetVolume(request.Context(), volume.Config.Name).Return(volume, nil)
	mockOrchestrator.EXPECT().UpdateVolumeLUKSPassphraseNames(
		request.Context(), volume.Config.Name, &[]string{}).Return(nil)

	rc := volumeLUKSPassphraseNamesUpdater(writer, request, response, map[string]string{"volume": volume.Config.Name}, []byte(body))

	assert.Equal(t, http.StatusOK, rc)
	assert.Equal(t, volume, response.Volume)
	assert.Equal(t, "", response.Error)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Positive case: Volume found, replace "/config/luksPassphraseNames" with non-empty list
	volume = &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test"}}
	writer = &http_test.TestResponseWriter{}
	response = &UpdateVolumeResponse{}
	body = `["super-secret-passphrase-1"]`
	request = generateHTTPRequest(http.MethodPut, body)

	mockCtrl = gomock.NewController(t)
	mockOrchestrator = mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator = mockOrchestrator
	mockOrchestrator.EXPECT().GetVolume(request.Context(), volume.Config.Name).Return(volume, nil)
	mockOrchestrator.EXPECT().UpdateVolumeLUKSPassphraseNames(
		request.Context(), volume.Config.Name, &[]string{"super-secret-passphrase-1"}).Return(nil)

	rc = volumeLUKSPassphraseNamesUpdater(writer, request, response, map[string]string{"volume": volume.Config.Name}, []byte(body))

	assert.Equal(t, http.StatusOK, rc)
	assert.Equal(t, volume, response.Volume)
	assert.Equal(t, "", response.Error)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Invalid response object provided
	volume = &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test"}}
	writer = &http_test.TestResponseWriter{}
	invalidResponse := &ImportVolumeResponse{} // Wrong type!
	body = `[]`
	request = generateHTTPRequest(http.MethodPut, body)

	mockOrchestrator.EXPECT().GetVolume(request.Context(), volume.Config.Name).Return(volume, nil)

	rc = volumeLUKSPassphraseNamesUpdater(writer, request, invalidResponse, map[string]string{"volume": volume.Config.Name}, []byte(body))

	assert.Equal(t, http.StatusInternalServerError, rc)
	assert.Equal(t, volume, response.Volume)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Patch modify on invalid(integer) /config/luksPassphraseNames value
	volume = &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test"}}
	writer = &http_test.TestResponseWriter{}
	response = &UpdateVolumeResponse{}
	body = `[1]`
	request = generateHTTPRequest(http.MethodPut, body)

	mockOrchestrator.EXPECT().GetVolume(request.Context(), volume.Config.Name).Return(volume, nil)

	rc = volumeLUKSPassphraseNamesUpdater(writer, request, response, map[string]string{"volume": volume.Config.Name}, []byte(body))

	assert.Equal(t, http.StatusBadRequest, rc)
	assert.Equal(t, volume, response.Volume)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Fail to get volume, not found error
	volume = &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test"}}
	writer = &http_test.TestResponseWriter{}
	response = &UpdateVolumeResponse{}
	body = `["super-secret-passphrase-1"]`
	request = generateHTTPRequest(http.MethodPut, body)

	mockCtrl = gomock.NewController(t)
	mockOrchestrator = mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator = mockOrchestrator
	mockOrchestrator.EXPECT().GetVolume(request.Context(), volume.Config.Name).Return(volume, errors.NotFoundError("test error"))

	rc = volumeLUKSPassphraseNamesUpdater(writer, request, response, map[string]string{"volume": volume.Config.Name}, []byte(body))

	assert.Equal(t, http.StatusNotFound, rc)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Fail to get volume, random error
	volume = &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test"}}
	writer = &http_test.TestResponseWriter{}
	response = &UpdateVolumeResponse{}
	body = `["super-secret-passphrase-1"]`
	request = generateHTTPRequest(http.MethodPut, body)

	mockCtrl = gomock.NewController(t)
	mockOrchestrator = mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator = mockOrchestrator
	mockOrchestrator.EXPECT().GetVolume(request.Context(), volume.Config.Name).Return(volume, fmt.Errorf("test error"))

	rc = volumeLUKSPassphraseNamesUpdater(writer, request, response, map[string]string{"volume": volume.Config.Name}, []byte(body))

	assert.Equal(t, http.StatusInternalServerError, rc)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Fail to update LUKS passphrase names
	volume = &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test"}}
	writer = &http_test.TestResponseWriter{}
	response = &UpdateVolumeResponse{}
	body = `["super-secret-passphrase-1"]`
	request = generateHTTPRequest(http.MethodPut, body)

	mockCtrl = gomock.NewController(t)
	mockOrchestrator = mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator = mockOrchestrator
	mockOrchestrator.EXPECT().GetVolume(request.Context(), volume.Config.Name).Return(volume, nil)
	mockOrchestrator.EXPECT().UpdateVolumeLUKSPassphraseNames(
		request.Context(), volume.Config.Name, &[]string{"super-secret-passphrase-1"}).Return(fmt.Errorf("test error"))

	rc = volumeLUKSPassphraseNamesUpdater(writer, request, response, map[string]string{"volume": volume.Config.Name}, []byte(body))

	assert.Equal(t, http.StatusInternalServerError, rc)
	assert.Equal(t, volume, response.Volume)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Fail to update LUKS passphrase names, not found error
	volume = &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test"}}
	writer = &http_test.TestResponseWriter{}
	response = &UpdateVolumeResponse{}
	body = `["super-secret-passphrase-1"]`
	request = generateHTTPRequest(http.MethodPut, body)

	mockCtrl = gomock.NewController(t)
	mockOrchestrator = mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator = mockOrchestrator
	mockOrchestrator.EXPECT().GetVolume(request.Context(), volume.Config.Name).Return(volume, nil)
	mockOrchestrator.EXPECT().UpdateVolumeLUKSPassphraseNames(
		request.Context(), volume.Config.Name, &[]string{"super-secret-passphrase-1"}).Return(errors.NotFoundError("test error"))

	rc = volumeLUKSPassphraseNamesUpdater(writer, request, response, map[string]string{"volume": volume.Config.Name}, []byte(body))

	assert.Equal(t, http.StatusNotFound, rc)
	assert.Equal(t, volume, response.Volume)
	mockCtrl.Finish()
}

func TestUpdateVolume(t *testing.T) {
	volName := "test"
	internalId := "/svm/fakesvm/flexvol/fakevol/qtree/" + volName
	vol := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			Name:        volName,
			InternalID:  internalId,
			SnapshotDir: "true",
		},
	}
	body := `
		{
			"snapshotDirectory" : "true",
			"poolLevel" : true
		}
	`
	r := generateHTTPRequest(http.MethodPut, body)
	w := &http_test.TestResponseWriter{}

	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator = mockOrchestrator
	mockOrchestrator.EXPECT().UpdateVolume(r.Context(), gomock.Any(), &models.VolumeUpdateInfo{
		SnapshotDirectory: "true",
		PoolLevel:         true,
	}).Return(nil)
	mockOrchestrator.EXPECT().GetVolume(r.Context(), gomock.Any()).Return(vol, nil).AnyTimes()

	UpdateVolume(w, r)

	assert.Equal(t, http.StatusOK, w.StatusCode)
}

func TestVolumeUpdater_Success(t *testing.T) {
	// Create request
	body := `
		{
			"snapshotDirectory" : "true",
			"poolLevel" : true
		}
	`
	request := generateHTTPRequest(http.MethodPut, body)

	// Create empty response
	writer := &http_test.TestResponseWriter{}
	response := &UpdateVolumeResponse{}

	// Create updated mock volume to be returned by mockOrchestrator
	volName := "test"
	internalId := "/svm/fakesvm/flexvol/fakevol/qtree/" + volName
	updatedVolume := &storage.VolumeExternal{
		Config: &storage.VolumeConfig{
			Name:        volName,
			InternalID:  internalId,
			SnapshotDir: "true",
		},
	}

	// Creat mock controller and orchestrator and inject it
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator = mockOrchestrator
	mockOrchestrator.EXPECT().UpdateVolume(
		request.Context(), volName, &models.VolumeUpdateInfo{
			SnapshotDirectory: "true",
			PoolLevel:         true,
		}).Return(nil)
	mockOrchestrator.EXPECT().GetVolume(request.Context(), volName).Return(updatedVolume, nil)

	// Call volumeUpdater
	responseCode := volumeUpdater(writer, request, response, map[string]string{"volume": volName}, []byte(body))

	assert.Equal(t, http.StatusOK, responseCode)
	assert.Equal(t, updatedVolume.Config.Name, response.Volume.Config.Name)
	assert.Equal(t, updatedVolume.Config.SnapshotDir, response.Volume.Config.SnapshotDir)
	assert.Equal(t, "", response.Error)
}

func TestVolumeUpdater_Failure(t *testing.T) {
	volName := "test"
	body := `
		{
			"snapshotDirectory" : "true",
			"poolLevel" : true
		}
	`
	// Create request and writer
	request := generateHTTPRequest(http.MethodPost, body)
	writer := &http_test.TestResponseWriter{}

	// Creat mock controller and orchestrator and inject it
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator = mockOrchestrator

	// CASE 1: Invalid response object sent
	responseCode := volumeUpdater(writer, request, &UpdateBackendResponse{}, map[string]string{"volume": volName}, []byte(body))

	assert.Equal(t, http.StatusInternalServerError, responseCode)

	// CASE 2: Invalid body of the request
	invalidBody := `
		{
			fake-key" : "fake-value",
		}
	`
	response := &UpdateVolumeResponse{}
	responseCode = volumeUpdater(writer, request, response, map[string]string{"volume": volName}, []byte(invalidBody))

	assert.Equal(t, http.StatusBadRequest, responseCode)
	assert.Contains(t, response.Error, "invalid JSON:")

	// CASE 3: Failed to update the volume: Invalid input
	invalidInputErr := errors.InvalidInputError("fake invalid input")
	mockCtrl = gomock.NewController(t)
	mockOrchestrator = mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator = mockOrchestrator
	mockOrchestrator.EXPECT().UpdateVolume(request.Context(), volName, &models.VolumeUpdateInfo{
		SnapshotDirectory: "true",
		PoolLevel:         true,
	}).Return(invalidInputErr)
	response = &UpdateVolumeResponse{}

	responseCode = volumeUpdater(writer, request, response, map[string]string{"volume": "test"}, []byte(body))

	assert.Equal(t, http.StatusBadRequest, responseCode)

	// CASE 4: Failed to update the volume: Volume not found
	notFoundErr := errors.NotFoundError("volume %s was not found", volName)
	mockCtrl = gomock.NewController(t)
	mockOrchestrator = mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator = mockOrchestrator
	mockOrchestrator.EXPECT().UpdateVolume(request.Context(), volName, &models.VolumeUpdateInfo{
		SnapshotDirectory: "true",
		PoolLevel:         true,
	}).Return(notFoundErr)
	response = &UpdateVolumeResponse{}

	responseCode = volumeUpdater(writer, request, response, map[string]string{"volume": "test"}, []byte(body))

	assert.Equal(t, http.StatusNotFound, responseCode)
	assert.Equal(t, notFoundErr.Error(), response.Error)

	// Case 5: Failed to update the volume: other error
	fakeErr := errors.New("fake error")
	mockCtrl = gomock.NewController(t)
	mockOrchestrator = mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator = mockOrchestrator
	mockOrchestrator.EXPECT().UpdateVolume(request.Context(), volName, &models.VolumeUpdateInfo{
		SnapshotDirectory: "true",
		PoolLevel:         true,
	}).Return(fakeErr)
	response = &UpdateVolumeResponse{}

	responseCode = volumeUpdater(writer, request, response, map[string]string{"volume": volName}, []byte(body))

	assert.Equal(t, http.StatusInternalServerError, responseCode)
	assert.Equal(t, fakeErr.Error(), response.Error)

	// Case 6: Failed to get updated volume
	mockCtrl = gomock.NewController(t)
	mockOrchestrator = mockcore.NewMockOrchestrator(mockCtrl)
	orchestrator = mockOrchestrator
	mockOrchestrator.EXPECT().UpdateVolume(request.Context(), volName, &models.VolumeUpdateInfo{
		SnapshotDirectory: "true",
		PoolLevel:         true,
	}).Return(nil)
	mockOrchestrator.EXPECT().GetVolume(request.Context(), volName).Return(nil, fakeErr)
	response = &UpdateVolumeResponse{}

	responseCode = volumeUpdater(writer, request, response, map[string]string{"volume": volName}, []byte(body))

	assert.Equal(t, http.StatusInternalServerError, responseCode)
	assert.Equal(t, fakeErr.Error(), response.Error)
}

// TestUpdateNodeIsAsync tests that UpdateNode is called when it can get the core lock, after
// responding to the request, by:
// 1. Requesting another endpoint (ListNodes) that holds core lock for some time
// 2. Requesting UpdateNode
// 3. Waiting for ListNodes and UpdateNode to return
// 4. Asserting response received before UpdateNode returns
func TestUpdateNodeIsAsync(t *testing.T) {
	// Set up mocks and tear down functions.
	oldOrchestrator := orchestrator
	defer func() {
		orchestrator = oldOrchestrator
	}()
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
	mockK8sHelper := mockk8scontrollerhelper.NewMockK8SControllerHelperPlugin(mockCtrl)

	// Setup values to return from mocked calls.
	nodeStateFlags := &models.NodePublicationStateFlags{
		OrchestratorReady:  convert.ToPtr(true),
		AdministratorReady: convert.ToPtr(true),
		ProvisionerReady:   nil,
	}

	// Set up test methods.
	var (
		m                          sync.Mutex
		wg                         sync.WaitGroup
		updateNodeCalled           time.Time
		updateNodeResponseReceived time.Time
	)

	// Setup test http server.
	orchestrator = mockOrchestrator
	ts := httptest.NewServer(NewRouter(false))

	// Set up the expected mock calls and add wait groups to those that are async.
	wg.Add(3)
	gomock.InOrder(
		mockOrchestrator.EXPECT().ListNodes(gomock.Any()).
			DoAndReturn(func(_ context.Context) ([]models.Node, error) {
				m.Lock()
				defer m.Unlock()
				time.Sleep(200 * time.Millisecond)
				return nil, nil
			}),
		mockOrchestrator.EXPECT().GetFrontend(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string) (frontend.Plugin, error) {
				return mockK8sHelper, nil
			}),
		mockK8sHelper.EXPECT().GetNodePublicationState(gomock.Any(), gomock.Any()).Return(nodeStateFlags, nil).Times(1),
		mockOrchestrator.EXPECT().UpdateNode(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, _ *models.NodePublicationStateFlags) error {
				m.Lock()
				defer m.Unlock()
				updateNodeCalled = time.Now()
				wg.Done()
				return nil
			}),
	)

	// Make requests.
	go func() {
		defer wg.Done()
		// ListNodes
		res, err := http.Get(ts.URL + "/trident/v1/node")
		assert.NoError(t, err, "expected no error")
		assert.Equal(t, http.StatusOK, res.StatusCode)
	}()
	go func() {
		defer wg.Done()
		// Ensure this request occurs after first request.
		time.Sleep(20 * time.Millisecond)

		nodeState := models.NodePublicationStateFlags{ProvisionerReady: convert.ToPtr(true)}
		data, err := json.Marshal(nodeState)
		if err != nil {
			t.Error("could not create request body")
		}
		// Make request to UpdateNode state.
		req, err := http.NewRequest(http.MethodPut, ts.URL+"/trident/v1/node/nodeID/publicationState", bytes.NewBuffer(data))
		assert.NoError(t, err, "expected no error")
		res, err := http.DefaultClient.Do(req)
		updateNodeResponseReceived = time.Now()
		assert.NoError(t, err)
		assert.Equal(t, http.StatusAccepted, res.StatusCode)
	}()
	wg.Wait()
	assert.True(t, updateNodeResponseReceived.Before(updateNodeCalled),
		"response must be received before handler called")
}

func TestGetNode(t *testing.T) {
	// Set up mocks and tear down functions.
	oldOrchestrator := orchestrator
	defer func() {
		orchestrator = oldOrchestrator
	}()
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	// Set up the mock orchestrator, test server and test values.
	orchestrator = mockOrchestrator
	server := httptest.NewServer(NewRouter(false))
	nodeName := "foo"
	nodeExternal := &models.NodeExternal{Name: nodeName}
	mockOrchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nodeExternal, nil)

	// Build a new request to the GetNode route.
	url := server.URL + "/trident/v1/node/" + nodeName
	req, err := http.NewRequest(http.MethodGet, url, bytes.NewBuffer([]byte{}))
	assert.NoError(t, err, "expected no error")

	// Make the request and ensure it doesn't fail and the response is valid.
	res, err := http.DefaultClient.Do(req)
	assert.NotNil(t, res, "expected non-nil response")
	assert.NoError(t, err, "expected no error")

	// Parse the response body and ensure it contains the expected values.
	defer res.Body.Close()
	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		t.FailNow()
	}
	updateNodeResponse := GetNodeResponse{}
	if err := json.Unmarshal(responseBody, &updateNodeResponse); err != nil {
		t.FailNow()
	}
	assert.Equal(t, updateNodeResponse.Node.Name, nodeName, "expected equal values")
}

func TestGetNode_FailsWithCoreError(t *testing.T) {
	// Set up mocks and tear down functions.
	oldOrchestrator := orchestrator
	defer func() {
		orchestrator = oldOrchestrator
	}()
	mockCtrl := gomock.NewController(t)
	mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

	// Set up the mock orchestrator, test server and test values.
	orchestrator = mockOrchestrator
	server := httptest.NewServer(NewRouter(false))
	nodeName := "foo"
	mockOrchestrator.EXPECT().GetNode(gomock.Any(), nodeName).Return(nil, errors.New("core error"))

	// Build a new request to the GetNode route.
	url := server.URL + "/trident/v1/node/" + nodeName
	req, err := http.NewRequest(http.MethodGet, url, bytes.NewBuffer([]byte{}))
	assert.NoError(t, err, "expected no error")

	// Make the request and ensure it doesn't fail and the response is valid.
	res, err := http.DefaultClient.Do(req)
	assert.NotNil(t, res, "expected non-nil response")
	assert.Nil(t, err, "expected no error")

	// Parse the response body and ensure it contains the expected values.
	defer res.Body.Close()
	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		t.FailNow()
	}
	updateNodeResponse := GetNodeResponse{}
	if err := json.Unmarshal(responseBody, &updateNodeResponse); err != nil {
		t.FailNow()
	}
	assert.Nil(t, updateNodeResponse.Node, "expected nil Node value in response")
	assert.NotEmpty(t, updateNodeResponse.Error, "expected non-empty Error string in response")
}

// ============================================================================
// Tests for 0% Coverage Functions - Core Helper Functions
// ============================================================================

func TestHttpStatusCodeForAdd(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedCode int
	}{
		{
			name:         "NoError",
			err:          nil,
			expectedCode: http.StatusCreated,
		},
		{
			name:         "NotReadyError",
			err:          errors.NotReadyError(),
			expectedCode: http.StatusServiceUnavailable,
		},
		{
			name:         "BootstrapError",
			err:          errors.BootstrapError(errors.New("bootstrap error")),
			expectedCode: http.StatusInternalServerError,
		},
		{
			name:         "GenericError",
			err:          errors.New("generic error"),
			expectedCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := httpStatusCodeForAdd(tt.err)
			assert.Equal(t, tt.expectedCode, code)
		})
	}
}

func TestHttpStatusCodeForDelete(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedCode int
	}{
		{
			name:         "NoError",
			err:          nil,
			expectedCode: http.StatusOK,
		},
		{
			name:         "NotReadyError",
			err:          errors.NotReadyError(),
			expectedCode: http.StatusServiceUnavailable,
		},
		{
			name:         "BootstrapError",
			err:          errors.BootstrapError(errors.New("bootstrap error")),
			expectedCode: http.StatusInternalServerError,
		},
		{
			name:         "NotFoundError",
			err:          errors.NotFoundError("not found"),
			expectedCode: http.StatusNotFound,
		},
		{
			name:         "ConflictError",
			err:          errors.ConflictError("conflict"),
			expectedCode: http.StatusConflict,
		},
		{
			name:         "GenericError",
			err:          errors.New("generic error"),
			expectedCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code := httpStatusCodeForDelete(tt.err)
			assert.Equal(t, tt.expectedCode, code)
		})
	}
}

func TestWriteHTTPResponse(t *testing.T) {
	tests := []struct {
		name           string
		ctx            context.Context
		response       interface{}
		httpStatusCode int
		expectedBody   string
		expectError    bool
	}{
		{
			name:           "SuccessfulResponse",
			ctx:            context.Background(),
			response:       map[string]string{"message": "success"},
			httpStatusCode: http.StatusOK,
			expectedBody:   `{"message":"success"}`,
			expectError:    false,
		},
		{
			name:           "ErrorResponse",
			ctx:            context.Background(),
			response:       map[string]string{"error": "test error"},
			httpStatusCode: http.StatusInternalServerError,
			expectedBody:   `{"error":"test error"}`,
			expectError:    false,
		},
		{
			name:           "NilResponse",
			ctx:            context.Background(),
			response:       nil,
			httpStatusCode: http.StatusOK,
			expectedBody:   "null",
			expectError:    false,
		},
		{
			name:           "UnmarshalableResponse",
			ctx:            context.Background(),
			response:       make(chan int), // channels cannot be marshaled to JSON
			httpStatusCode: http.StatusOK,
			expectedBody:   "",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			writeHTTPResponse(tt.ctx, recorder, tt.response, tt.httpStatusCode)

			if tt.expectError {
				// For error cases, expect 500 status and empty body
				assert.Equal(t, http.StatusInternalServerError, recorder.Code)
				assert.Empty(t, recorder.Body.String())
			} else {
				assert.Equal(t, tt.httpStatusCode, recorder.Code)
				assert.JSONEq(t, tt.expectedBody, recorder.Body.String())
			}
		})
	}
}

// ============================================================================
// Tests for Core API Functions with 0% Coverage
// ============================================================================

func TestGetVersion(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func(*mockcore.MockOrchestrator)
		expectedStatus int
		expectVersion  bool
	}{
		{
			name: "SuccessfulGetVersion",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetVersion(gomock.Any()).Return("1.0.0", nil).Times(1)
			},
			expectedStatus: http.StatusOK,
			expectVersion:  true,
		},
		{
			name: "OrchestratorError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetVersion(gomock.Any()).Return("", errors.New("version error")).Times(1)
			},
			expectedStatus: http.StatusBadRequest, // httpStatusCodeForGetUpdateList for generic error
			expectVersion:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/version", nil)
			recorder := httptest.NewRecorder()

			// Call function
			GetVersion(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response GetVersionResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectVersion {
				assert.NotEmpty(t, response.Version)
				assert.Empty(t, response.Error)
			} else {
				assert.NotEmpty(t, response.Error)
			}
		})
	}
}

func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "ValidUUID",
			input:    "123e4567-e89b-12d3-a456-426614174000",
			expected: true,
		},
		{
			name:     "InvalidUUID",
			input:    "not-a-uuid",
			expected: false,
		},
		{
			name:     "EmptyString",
			input:    "",
			expected: false,
		},
		{
			name:     "PartialUUID",
			input:    "123e4567-e89b",
			expected: false,
		},
		{
			name:     "UUIDWithoutHyphens",
			input:    "123e4567e89b12d3a456426614174000",
			expected: true, // UUID parse can handle UUIDs without hyphens
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidUUID(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// Tests for Next Phase - Backend, Volume, and Storage Class Operations (0% Coverage)
// ============================================================================

func TestAddBackend(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedBackendID string
	}{
		{
			name: "SuccessfulBackendAdd",
			body: `{"version": 1, "storageDriverName": "ontap-nas", "managementLIF": "1.2.3.4"}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				backend := &storage.BackendExternal{Name: "test-backend-id"}
				m.EXPECT().AddBackend(gomock.Any(), gomock.Any(), "").Return(backend, nil).Times(1)
			},
			expectedStatus:    http.StatusCreated,
			expectErrorInBody: false,
			expectedBackendID: "test-backend-id",
		},
		{
			name: "InvalidJSON",
			body: `{invalid json}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				// AddBackend will still be called but with invalid JSON string
				m.EXPECT().AddBackend(gomock.Any(), `{invalid json}`, "").Return(nil, errors.New("invalid JSON")).Times(1)
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedBackendID: "",
		},
		{
			name: "OrchestratorError",
			body: `{"version": 1, "storageDriverName": "ontap-nas", "managementLIF": "1.2.3.4"}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().AddBackend(gomock.Any(), gomock.Any(), "").Return(nil, errors.New("orchestrator error")).Times(1)
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedBackendID: "",
		},
		{
			name: "AlreadyExistsError",
			body: `{"version": 1, "storageDriverName": "ontap-nas", "managementLIF": "1.2.3.4"}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().AddBackend(gomock.Any(), gomock.Any(), "").Return(nil, errors.AlreadyExistsError("backend already exists")).Times(1)
			},
			expectedStatus:    http.StatusBadRequest, // httpStatusCodeForAdd returns BadRequest for all non-specific errors
			expectErrorInBody: true,
			expectedBackendID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodPost, "/trident/v1/backend", strings.NewReader(tt.body))
			recorder := httptest.NewRecorder()

			// Call function
			AddBackend(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response AddBackendResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.Equal(t, tt.expectedBackendID, response.BackendID)
			}
		})
	}
}

func TestDeleteBackend(t *testing.T) {
	tests := []struct {
		name              string
		backendName       string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
	}{
		{
			name:        "SuccessfulDelete",
			backendName: "test-backend",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteBackend(gomock.Any(), "test-backend").Return(nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:        "BackendNotFound",
			backendName: "nonexistent-backend",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteBackend(gomock.Any(), "nonexistent-backend").Return(errors.NotFoundError("backend not found")).Times(1)
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
		},
		{
			name:        "ConflictError",
			backendName: "busy-backend",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteBackend(gomock.Any(), "busy-backend").Return(errors.ConflictError("backend has volumes")).Times(1)
			},
			expectedStatus:    http.StatusConflict,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodDelete, "/trident/v1/backend/"+tt.backendName, nil)
			req = mux.SetURLVars(req, map[string]string{"backend": tt.backendName})
			recorder := httptest.NewRecorder()

			// Call function
			DeleteBackend(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response DeleteResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
			}
		})
	}
}

func TestAddVolume(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedBackendID string
	}{
		{
			name: "SuccessfulVolumeAdd",
			body: `{"version": "1", "name": "test-volume", "storageClass": "basic", "size": "1GB"}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				volume := &storage.VolumeExternal{BackendUUID: "test-backend-uuid"}
				m.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(volume, nil).Times(1)
			},
			expectedStatus:    http.StatusCreated,
			expectErrorInBody: false,
			expectedBackendID: "test-backend-uuid",
		},
		{
			name: "InvalidJSON",
			body: `{invalid json}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				// No mock calls expected for invalid JSON
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedBackendID: "",
		},
		{
			name: "ValidationError",
			body: `{"version": "1", "name": "", "storageClass": "basic", "size": "1GB"}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				// No mock calls expected for validation failure
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedBackendID: "",
		},
		{
			name: "OrchestratorError",
			body: `{"version": "1", "name": "test-volume", "storageClass": "basic", "size": "1GB"}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().AddVolume(gomock.Any(), gomock.Any()).Return(nil, errors.New("orchestrator error")).Times(1)
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedBackendID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodPost, "/trident/v1/volume", strings.NewReader(tt.body))
			recorder := httptest.NewRecorder()

			// Call function
			AddVolume(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response AddVolumeResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.Equal(t, tt.expectedBackendID, response.BackendID)
			}
		})
	}
}

func TestListVolumes(t *testing.T) {
	tests := []struct {
		name              string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedCount     int
	}{
		{
			name: "SuccessfulList",
			setupMock: func(m *mockcore.MockOrchestrator) {
				volumes := []*storage.VolumeExternal{
					{Config: &storage.VolumeConfig{Name: "volume1"}},
					{Config: &storage.VolumeConfig{Name: "volume2"}},
				}
				m.EXPECT().ListVolumes(gomock.Any()).Return(volumes, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedCount:     2,
		},
		{
			name: "EmptyList",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumes(gomock.Any()).Return([]*storage.VolumeExternal{}, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedCount:     0,
		},
		{
			name: "OrchestratorError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumes(gomock.Any()).Return(nil, errors.New("list error")).Times(1)
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedCount:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/volume", nil)
			recorder := httptest.NewRecorder()

			// Call function
			ListVolumes(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response ListVolumesResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.Len(t, response.Volumes, tt.expectedCount)
			}
		})
	}
}

func TestGetVolume(t *testing.T) {
	tests := []struct {
		name              string
		volumeName        string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
	}{
		{
			name:       "SuccessfulGet",
			volumeName: "test-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				volume := &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test-volume"}}
				m.EXPECT().GetVolume(gomock.Any(), "test-volume").Return(volume, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:       "VolumeNotFound",
			volumeName: "nonexistent-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetVolume(gomock.Any(), "nonexistent-volume").Return(nil, errors.NotFoundError("volume not found")).Times(1)
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
		},
		{
			name:       "OrchestratorError",
			volumeName: "error-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetVolume(gomock.Any(), "error-volume").Return(nil, errors.New("get error")).Times(1)
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/volume/"+tt.volumeName, nil)
			req = mux.SetURLVars(req, map[string]string{"volume": tt.volumeName})
			recorder := httptest.NewRecorder()

			// Call function
			GetVolume(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response GetVolumeResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.NotNil(t, response.Volume)
			}
		})
	}
}

func TestDeleteVolume(t *testing.T) {
	tests := []struct {
		name              string
		volumeName        string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
	}{
		{
			name:       "SuccessfulDelete",
			volumeName: "test-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteVolume(gomock.Any(), "test-volume").Return(nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:       "VolumeNotFound",
			volumeName: "nonexistent-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteVolume(gomock.Any(), "nonexistent-volume").Return(errors.NotFoundError("volume not found")).Times(1)
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
		},
		{
			name:       "ConflictError",
			volumeName: "busy-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteVolume(gomock.Any(), "busy-volume").Return(errors.ConflictError("volume is in use")).Times(1)
			},
			expectedStatus:    http.StatusConflict,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodDelete, "/trident/v1/volume/"+tt.volumeName, nil)
			req = mux.SetURLVars(req, map[string]string{"volume": tt.volumeName})
			recorder := httptest.NewRecorder()

			// Call function
			DeleteVolume(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response DeleteResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
			}
		})
	}
}

func TestAddStorageClass(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
	}{
		{
			name: "SuccessfulStorageClassAdd",
			body: `{"metadata": {"name": "test-sc"}, "spec": {"backendType": "ontap-nas"}}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				sc := &storageclass.External{Config: &storageclass.Config{Name: "test-sc"}}
				m.EXPECT().AddStorageClass(gomock.Any(), gomock.Any()).Return(sc, nil).Times(1)
			},
			expectedStatus:    http.StatusCreated,
			expectErrorInBody: false,
		},
		{
			name: "InvalidJSON",
			body: `{invalid json}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				// No mock calls expected for invalid JSON
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
		},
		{
			name: "OrchestratorError",
			body: `{"metadata": {"name": "test-sc"}, "spec": {"backendType": "ontap-nas"}}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().AddStorageClass(gomock.Any(), gomock.Any()).Return(nil, errors.New("orchestrator error")).Times(1)
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodPost, "/trident/v1/storageclass", strings.NewReader(tt.body))
			recorder := httptest.NewRecorder()

			// Call function
			AddStorageClass(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response AddStorageClassResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.NotEmpty(t, response.StorageClassID)
			}
		})
	}
}

func TestListStorageClasses(t *testing.T) {
	tests := []struct {
		name              string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedCount     int
	}{
		{
			name: "SuccessfulList",
			setupMock: func(m *mockcore.MockOrchestrator) {
				storageClasses := []*storageclass.External{
					{Config: &storageclass.Config{Name: "sc1"}},
					{Config: &storageclass.Config{Name: "sc2"}},
				}
				m.EXPECT().ListStorageClasses(gomock.Any()).Return(storageClasses, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedCount:     2,
		},
		{
			name: "OrchestratorError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListStorageClasses(gomock.Any()).Return(nil, errors.New("list error")).Times(1)
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedCount:     0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/storageclass", nil)
			recorder := httptest.NewRecorder()

			// Call function
			ListStorageClasses(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response ListStorageClassesResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.Len(t, response.StorageClasses, tt.expectedCount)
			}
		})
	}
}

// ============================================================================
// Tests for High Priority Functions with 0% Coverage
// ============================================================================

func TestGetACPVersion(t *testing.T) {
	tests := []struct {
		name           string
		setupACP       func() *mockacp.MockTridentACP
		expectedResult string
	}{
		{
			name: "ACPDisabled",
			setupACP: func() *mockacp.MockTridentACP {
				mockCtrl := gomock.NewController(t)
				mockACP := mockacp.NewMockTridentACP(mockCtrl)
				mockACP.EXPECT().Enabled().Return(false)
				return mockACP
			},
			expectedResult: "",
		},
		{
			name: "ACPEnabledValidVersion",
			setupACP: func() *mockacp.MockTridentACP {
				mockCtrl := gomock.NewController(t)
				mockACP := mockacp.NewMockTridentACP(mockCtrl)
				mockACP.EXPECT().Enabled().Return(true)

				version, _ := version.ParseDate("23.10.0")
				mockACP.EXPECT().GetVersion(gomock.Any()).Return(version, nil)
				return mockACP
			},
			expectedResult: "23.10.0",
		},
		{
			name: "ACPEnabledGetVersionError",
			setupACP: func() *mockacp.MockTridentACP {
				mockCtrl := gomock.NewController(t)
				mockACP := mockacp.NewMockTridentACP(mockCtrl)
				mockACP.EXPECT().Enabled().Return(true)
				mockACP.EXPECT().GetVersion(gomock.Any()).Return(nil, errors.New("ACP connection error"))
				return mockACP
			},
			expectedResult: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockACP := tt.setupACP()

			// Set the mock ACP API
			originalAPI := acp.API()
			acp.SetAPI(mockACP)
			defer acp.SetAPI(originalAPI)

			ctx := context.Background()
			result := GetACPVersion(ctx)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestUpdateBackend(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		backendName       string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedBackendID string
	}{
		{
			name:        "SuccessfulUpdate",
			body:        `{"version": 1, "storageDriverName": "ontap-nas"}`,
			backendName: "test-backend",
			setupMock: func(m *mockcore.MockOrchestrator) {
				backend := &storage.BackendExternal{Name: "test-backend"}
				m.EXPECT().UpdateBackend(gomock.Any(), "test-backend", `{"version": 1, "storageDriverName": "ontap-nas"}`, "").Return(backend, nil)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedBackendID: "test-backend",
		},
		{
			name:        "BackendNotFound",
			body:        `{"version": 1, "storageDriverName": "ontap-nas"}`,
			backendName: "nonexistent-backend",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().UpdateBackend(gomock.Any(), "nonexistent-backend", `{"version": 1, "storageDriverName": "ontap-nas"}`, "").Return(nil, errors.NotFoundError("backend not found"))
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
			expectedBackendID: "",
		},
		{
			name:        "InternalError",
			body:        `{"version": 1, "storageDriverName": "ontap-nas"}`,
			backendName: "error-backend",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().UpdateBackend(gomock.Any(), "error-backend", `{"version": 1, "storageDriverName": "ontap-nas"}`, "").Return(nil, errors.New("internal error"))
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedBackendID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodPut, "/trident/v1/backend/"+tt.backendName, strings.NewReader(tt.body))
			req = mux.SetURLVars(req, map[string]string{"backend": tt.backendName})
			recorder := httptest.NewRecorder()

			// Call function
			UpdateBackend(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response UpdateBackendResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.Equal(t, tt.expectedBackendID, response.BackendID)
			}
		})
	}
}

func TestUpdateBackendState(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		backendName       string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedBackendID string
	}{
		{
			name:        "SuccessfulStateUpdate",
			body:        `{"state": "online", "userState": ""}`,
			backendName: "test-backend",
			setupMock: func(m *mockcore.MockOrchestrator) {
				backend := &storage.BackendExternal{Name: "test-backend"}
				m.EXPECT().UpdateBackendState(gomock.Any(), "test-backend", "online", "").Return(backend, nil)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedBackendID: "test-backend",
		},
		{
			name:        "InvalidJSON",
			body:        `{invalid json}`,
			backendName: "test-backend",
			setupMock: func(m *mockcore.MockOrchestrator) {
				// No mock expectation for invalid JSON
			},
			expectedStatus:    http.StatusBadRequest, // httpStatusCodeForGetUpdateList for JSON error
			expectErrorInBody: true,
			expectedBackendID: "",
		},
		{
			name:        "BackendNotFound",
			body:        `{"state": "offline", "userState": ""}`,
			backendName: "nonexistent-backend",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().UpdateBackendState(gomock.Any(), "nonexistent-backend", "offline", "").Return(nil, errors.NotFoundError("backend not found"))
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
			expectedBackendID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodPut, "/trident/v1/backend/"+tt.backendName+"/state", strings.NewReader(tt.body))
			req = mux.SetURLVars(req, map[string]string{"backend": tt.backendName})
			recorder := httptest.NewRecorder()

			// Call function
			UpdateBackendState(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response UpdateBackendResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.Equal(t, tt.expectedBackendID, response.BackendID)
			}
		})
	}
}

func TestListBackends(t *testing.T) {
	tests := []struct {
		name              string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedCount     int
		expectedBackends  []string
	}{
		{
			name: "SuccessfulList",
			setupMock: func(m *mockcore.MockOrchestrator) {
				backends := []*storage.BackendExternal{
					{Name: "backend1"},
					{Name: "backend2"},
				}
				m.EXPECT().ListBackends(gomock.Any()).Return(backends, nil)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedCount:     2,
			expectedBackends:  []string{"backend1", "backend2"},
		},
		{
			name: "EmptyList",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListBackends(gomock.Any()).Return([]*storage.BackendExternal{}, nil)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedCount:     0,
			expectedBackends:  []string{},
		},
		{
			name: "OrchestratorError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListBackends(gomock.Any()).Return(nil, errors.New("list error"))
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedCount:     0,
			expectedBackends:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/backend", nil)
			recorder := httptest.NewRecorder()

			// Call function
			ListBackends(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response ListBackendsResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.Len(t, response.Backends, tt.expectedCount)
				if tt.expectedCount > 0 {
					assert.ElementsMatch(t, tt.expectedBackends, response.Backends)
				}
			}
		})
	}
}

func TestGetBackend(t *testing.T) {
	tests := []struct {
		name              string
		backendName       string
		isUUID            bool
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedBackend   *storage.BackendExternal
	}{
		{
			name:        "SuccessfulGetByName",
			backendName: "test-backend",
			isUUID:      false,
			setupMock: func(m *mockcore.MockOrchestrator) {
				backend := &storage.BackendExternal{Name: "test-backend"}
				m.EXPECT().GetBackend(gomock.Any(), "test-backend").Return(backend, nil)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedBackend:   &storage.BackendExternal{Name: "test-backend"},
		},
		{
			name:        "SuccessfulGetByUUID",
			backendName: "123e4567-e89b-12d3-a456-426614174000",
			isUUID:      true,
			setupMock: func(m *mockcore.MockOrchestrator) {
				backend := &storage.BackendExternal{Name: "test-backend", BackendUUID: "123e4567-e89b-12d3-a456-426614174000"}
				m.EXPECT().GetBackendByBackendUUID(gomock.Any(), "123e4567-e89b-12d3-a456-426614174000").Return(backend, nil)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedBackend:   &storage.BackendExternal{Name: "test-backend", BackendUUID: "123e4567-e89b-12d3-a456-426614174000"},
		},
		{
			name:        "BackendNotFound",
			backendName: "nonexistent-backend",
			isUUID:      false,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetBackend(gomock.Any(), "nonexistent-backend").Return(nil, errors.NotFoundError("backend not found"))
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
			expectedBackend:   nil,
		},
		{
			name:        "InternalError",
			backendName: "error-backend",
			isUUID:      false,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetBackend(gomock.Any(), "error-backend").Return(nil, errors.New("internal error"))
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedBackend:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/backend/"+tt.backendName, nil)
			req = mux.SetURLVars(req, map[string]string{"backend": tt.backendName})
			recorder := httptest.NewRecorder()

			// Call function
			GetBackend(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response GetBackendResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
				assert.Nil(t, response.Backend)
			} else {
				assert.Empty(t, response.Error)
				assert.NotNil(t, response.Backend)
				assert.Equal(t, tt.expectedBackend.Name, response.Backend.Name)
				if tt.isUUID {
					assert.Equal(t, tt.expectedBackend.BackendUUID, response.Backend.BackendUUID)
				}
			}
		})
	}
}

func TestImportVolume(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		setupMock         func(*mockcore.MockOrchestrator, *mockk8scontrollerhelper.MockK8SControllerHelperPlugin)
		expectedStatus    int
		expectErrorInBody bool
		expectedVolume    *storage.VolumeExternal
	}{
		{
			name: "SuccessfulImport",
			body: `{
				"backend": "test-backend",
				"internalName": "existing-volume",
				"pvcData": "eyJhcGlWZXJzaW9uIjoidjEiLCJraW5kIjoiUGVyc2lzdGVudFZvbHVtZUNsYWltIn0="
			}`,
			setupMock: func(m *mockcore.MockOrchestrator, k8s *mockk8scontrollerhelper.MockK8SControllerHelperPlugin) {
				volume := &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "existing-volume"}}
				m.EXPECT().GetFrontend(gomock.Any(), "k8s_csi_helper").Return(k8s, nil)
				k8s.EXPECT().ImportVolume(gomock.Any(), gomock.Any()).Return(volume, nil)
			},
			expectedStatus:    http.StatusCreated,
			expectErrorInBody: false,
			expectedVolume:    &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "existing-volume"}},
		},
		{
			name: "InvalidJSON",
			body: `{invalid json}`,
			setupMock: func(m *mockcore.MockOrchestrator, k8s *mockk8scontrollerhelper.MockK8SControllerHelperPlugin) {
				// No mock expectations for invalid JSON
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedVolume:    nil,
		},
		{
			name: "ValidationError",
			body: `{
				"backend": "test-backend",
				"pvcData": "eyJhcGlWZXJzaW9uIjoidjEiLCJraW5kIjoiUGVyc2lzdGVudFZvbHVtZUNsYWltIn0="
			}`,
			setupMock: func(m *mockcore.MockOrchestrator, k8s *mockk8scontrollerhelper.MockK8SControllerHelperPlugin) {
				// No mock expectations for validation error
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedVolume:    nil,
		},
		{
			name: "FrontendNotFound",
			body: `{
				"backend": "test-backend",
				"internalName": "existing-volume",
				"pvcData": "eyJhcGlWZXJzaW9uIjoidjEiLCJraW5kIjoiUGVyc2lzdGVudFZvbHVtZUNsYWltIn0="
			}`,
			setupMock: func(m *mockcore.MockOrchestrator, k8s *mockk8scontrollerhelper.MockK8SControllerHelperPlugin) {
				m.EXPECT().GetFrontend(gomock.Any(), "k8s_csi_helper").Return(nil, errors.New("frontend not found"))
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedVolume:    nil,
		},
		{
			name: "ImportError",
			body: `{
				"backend": "test-backend",
				"internalName": "existing-volume",
				"pvcData": "eyJhcGlWZXJzaW9uIjoidjEiLCJraW5kIjoiUGVyc2lzdGVudFZvbHVtZUNsYWltIn0="
			}`,
			setupMock: func(m *mockcore.MockOrchestrator, k8s *mockk8scontrollerhelper.MockK8SControllerHelperPlugin) {
				m.EXPECT().GetFrontend(gomock.Any(), "k8s_csi_helper").Return(k8s, nil)
				k8s.EXPECT().ImportVolume(gomock.Any(), gomock.Any()).Return(nil, errors.New("import failed"))
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedVolume:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)
			mockK8s := mockk8scontrollerhelper.NewMockK8SControllerHelperPlugin(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator, mockK8s)

			// Create request
			req := httptest.NewRequest(http.MethodPost, "/trident/v1/volume/import", strings.NewReader(tt.body))
			recorder := httptest.NewRecorder()

			// Call function
			ImportVolume(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response ImportVolumeResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
				assert.Nil(t, response.Volume)
			} else {
				assert.Empty(t, response.Error)
				assert.NotNil(t, response.Volume)
				assert.Equal(t, tt.expectedVolume.Config.Name, response.Volume.Config.Name)
			}
		})
	}
}

func TestUpdateVolumeLUKSPassphraseNames(t *testing.T) {
	tests := []struct {
		name              string
		volumeName        string
		body              string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
	}{
		{
			name:       "SuccessfulUpdate",
			volumeName: "test-volume",
			body:       `["passphrase1", "passphrase2"]`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				volume := &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test-volume"}}
				m.EXPECT().GetVolume(gomock.Any(), "test-volume").Return(volume, nil)
				m.EXPECT().UpdateVolumeLUKSPassphraseNames(gomock.Any(), "test-volume", &[]string{"passphrase1", "passphrase2"}).Return(nil)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:       "EmptyPassphraseList",
			volumeName: "test-volume",
			body:       `[]`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				volume := &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test-volume"}}
				m.EXPECT().GetVolume(gomock.Any(), "test-volume").Return(volume, nil)
				m.EXPECT().UpdateVolumeLUKSPassphraseNames(gomock.Any(), "test-volume", &[]string{}).Return(nil)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:       "VolumeNotFound",
			volumeName: "nonexistent-volume",
			body:       `["passphrase1"]`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetVolume(gomock.Any(), "nonexistent-volume").Return(nil, errors.NotFoundError("volume not found"))
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
		},
		{
			name:       "InvalidJSON",
			volumeName: "test-volume",
			body:       `{invalid json}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				volume := &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test-volume"}}
				m.EXPECT().GetVolume(gomock.Any(), "test-volume").Return(volume, nil)
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
		},
		{
			name:       "UpdateError",
			volumeName: "test-volume",
			body:       `["passphrase1"]`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				volume := &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test-volume"}}
				m.EXPECT().GetVolume(gomock.Any(), "test-volume").Return(volume, nil)
				m.EXPECT().UpdateVolumeLUKSPassphraseNames(gomock.Any(), "test-volume", &[]string{"passphrase1"}).Return(errors.New("update failed"))
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodPut, "/trident/v1/volume/"+tt.volumeName+"/luksPassphraseNames", strings.NewReader(tt.body))
			req = mux.SetURLVars(req, map[string]string{"volume": tt.volumeName})
			recorder := httptest.NewRecorder()

			// Call function
			UpdateVolumeLUKSPassphraseNames(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response UpdateVolumeResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.NotNil(t, response.Volume)
			}
		})
	}
}

func TestAddNode(t *testing.T) {
	tests := []struct {
		name              string
		body              string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedNodeName  string
	}{
		{
			name: "InvalidJSON",
			body: `{invalid json}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				// No mock expectations for invalid JSON
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedNodeName:  "",
		},
		{
			name: "NoCSIFrontend",
			body: `{
				"name": "test-node",
				"iqn": "iqn.1993-08.org.debian:01:test-node",
				"ips": ["192.168.1.100"]
			}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetFrontend(gomock.Any(), "k8s_csi_helper").Return(nil, errors.New("k8s helper not found"))
				m.EXPECT().GetFrontend(gomock.Any(), "plain_csi_helper").Return(nil, errors.New("plain helper not found"))
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
			expectedNodeName:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodPost, "/trident/v1/node", strings.NewReader(tt.body))
			recorder := httptest.NewRecorder()

			// Call function
			AddNode(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response AddNodeResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.Equal(t, tt.expectedNodeName, response.Name)
			}
		})
	}
}

// ============================================================================
// Tests for Priority 1 Functions with 0% Coverage - Core API Operations
// ============================================================================

func TestGetStorageClass(t *testing.T) {
	tests := []struct {
		name                 string
		storageClassName     string
		setupMock            func(*mockcore.MockOrchestrator)
		expectedStatus       int
		expectErrorInBody    bool
		expectedStorageClass *storageclass.External
	}{
		{
			name:             "SuccessfulGet",
			storageClassName: "test-storage-class",
			setupMock: func(m *mockcore.MockOrchestrator) {
				sc := &storageclass.External{Config: &storageclass.Config{Name: "test-storage-class"}}
				m.EXPECT().GetStorageClass(gomock.Any(), "test-storage-class").Return(sc, nil).Times(1)
			},
			expectedStatus:       http.StatusOK,
			expectErrorInBody:    false,
			expectedStorageClass: &storageclass.External{Config: &storageclass.Config{Name: "test-storage-class"}},
		},
		{
			name:             "StorageClassNotFound",
			storageClassName: "nonexistent-storage-class",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetStorageClass(gomock.Any(), "nonexistent-storage-class").Return(nil, errors.NotFoundError("storage class not found")).Times(1)
			},
			expectedStatus:       http.StatusNotFound,
			expectErrorInBody:    true,
			expectedStorageClass: nil,
		},
		{
			name:             "InternalOrchestratorError",
			storageClassName: "error-storage-class",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetStorageClass(gomock.Any(), "error-storage-class").Return(nil, errors.New("internal orchestrator error")).Times(1)
			},
			expectedStatus:       http.StatusBadRequest,
			expectErrorInBody:    true,
			expectedStorageClass: nil,
		},
		{
			name:             "BootstrapError",
			storageClassName: "bootstrap-error-storage-class",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetStorageClass(gomock.Any(), "bootstrap-error-storage-class").Return(nil, errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:       http.StatusInternalServerError,
			expectErrorInBody:    true,
			expectedStorageClass: nil,
		},
		{
			name:             "NotReadyError",
			storageClassName: "not-ready-storage-class",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetStorageClass(gomock.Any(), "not-ready-storage-class").Return(nil, errors.NotReadyError()).Times(1)
			},
			expectedStatus:       http.StatusServiceUnavailable,
			expectErrorInBody:    true,
			expectedStorageClass: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/storageclass/"+tt.storageClassName, nil)
			req = mux.SetURLVars(req, map[string]string{"storageClass": tt.storageClassName})
			recorder := httptest.NewRecorder()

			// Call function
			GetStorageClass(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response GetStorageClassResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
				assert.Nil(t, response.StorageClass)
			} else {
				assert.Empty(t, response.Error)
				assert.NotNil(t, response.StorageClass)
				assert.Equal(t, tt.expectedStorageClass.Config.Name, response.StorageClass.Config.Name)
			}
		})
	}
}

func TestDeleteStorageClass(t *testing.T) {
	tests := []struct {
		name              string
		storageClassName  string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
	}{
		{
			name:             "SuccessfulDelete",
			storageClassName: "test-storage-class",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteStorageClass(gomock.Any(), "test-storage-class").Return(nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:             "StorageClassNotFound",
			storageClassName: "nonexistent-storage-class",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteStorageClass(gomock.Any(), "nonexistent-storage-class").Return(errors.NotFoundError("storage class not found")).Times(1)
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
		},
		{
			name:             "ConflictError",
			storageClassName: "busy-storage-class",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteStorageClass(gomock.Any(), "busy-storage-class").Return(errors.ConflictError("storage class has volumes")).Times(1)
			},
			expectedStatus:    http.StatusConflict,
			expectErrorInBody: true,
		},
		{
			name:             "InternalOrchestratorError",
			storageClassName: "error-storage-class",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteStorageClass(gomock.Any(), "error-storage-class").Return(errors.New("internal orchestrator error")).Times(1)
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
		},
		{
			name:             "BootstrapError",
			storageClassName: "bootstrap-error-storage-class",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteStorageClass(gomock.Any(), "bootstrap-error-storage-class").Return(errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
		{
			name:             "NotReadyError",
			storageClassName: "not-ready-storage-class",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteStorageClass(gomock.Any(), "not-ready-storage-class").Return(errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodDelete, "/trident/v1/storageclass/"+tt.storageClassName, nil)
			req = mux.SetURLVars(req, map[string]string{"storageClass": tt.storageClassName})
			recorder := httptest.NewRecorder()

			// Call function
			DeleteStorageClass(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response DeleteResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
			}
		})
	}
}

func TestDeleteNode(t *testing.T) {
	tests := []struct {
		name              string
		nodeName          string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
	}{
		{
			name:     "SuccessfulDelete",
			nodeName: "test-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteNode(gomock.Any(), "test-node").Return(nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:     "NodeNotFound",
			nodeName: "nonexistent-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteNode(gomock.Any(), "nonexistent-node").Return(errors.NotFoundError("node not found")).Times(1)
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
		},
		{
			name:     "ConflictError",
			nodeName: "busy-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteNode(gomock.Any(), "busy-node").Return(errors.ConflictError("node has volumes")).Times(1)
			},
			expectedStatus:    http.StatusConflict,
			expectErrorInBody: true,
		},
		{
			name:     "InternalOrchestratorError",
			nodeName: "error-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteNode(gomock.Any(), "error-node").Return(errors.New("internal orchestrator error")).Times(1)
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
		},
		{
			name:     "BootstrapError",
			nodeName: "bootstrap-error-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteNode(gomock.Any(), "bootstrap-error-node").Return(errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
		{
			name:     "NotReadyError",
			nodeName: "not-ready-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteNode(gomock.Any(), "not-ready-node").Return(errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodDelete, "/trident/v1/node/"+tt.nodeName, nil)
			req = mux.SetURLVars(req, map[string]string{"node": tt.nodeName})
			recorder := httptest.NewRecorder()

			// Call function
			DeleteNode(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response DeleteResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
			}
		})
	}
}

// ============================================================================
// Tests for Priority 2 Functions with 0% Coverage - Volume Publication Management
// ============================================================================

func TestGetVolumePublication(t *testing.T) {
	tests := []struct {
		name                      string
		volumeName                string
		nodeName                  string
		setupMock                 func(*mockcore.MockOrchestrator)
		expectedStatus            int
		expectErrorInBody         bool
		expectedVolumePublication *models.VolumePublicationExternal
	}{
		{
			name:       "SuccessfulRetrieval",
			volumeName: "test-volume",
			nodeName:   "test-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				pub := &models.VolumePublication{
					Name:       "test-volume-test-node",
					VolumeName: "test-volume",
					NodeName:   "test-node",
					ReadOnly:   false,
					AccessMode: 1,
				}
				m.EXPECT().GetVolumePublication(gomock.Any(), "test-volume", "test-node").Return(pub, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedVolumePublication: &models.VolumePublicationExternal{
				Name:       "test-volume-test-node",
				VolumeName: "test-volume",
				NodeName:   "test-node",
				ReadOnly:   false,
				AccessMode: 1,
			},
		},
		{
			name:       "PublicationNotFound",
			volumeName: "nonexistent-volume",
			nodeName:   "test-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetVolumePublication(gomock.Any(), "nonexistent-volume", "test-node").Return(nil, errors.NotFoundError("volume publication not found")).Times(1)
			},
			expectedStatus:            http.StatusNotFound,
			expectErrorInBody:         true,
			expectedVolumePublication: nil,
		},
		{
			name:       "ValidationErrorEmptyVolume",
			volumeName: "",
			nodeName:   "test-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetVolumePublication(gomock.Any(), "", "test-node").Return(nil, errors.InvalidInputError("volume name cannot be empty")).Times(1)
			},
			expectedStatus:            http.StatusBadRequest,
			expectErrorInBody:         true,
			expectedVolumePublication: nil,
		},
		{
			name:       "ValidationErrorEmptyNode",
			volumeName: "test-volume",
			nodeName:   "",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetVolumePublication(gomock.Any(), "test-volume", "").Return(nil, errors.InvalidInputError("node name cannot be empty")).Times(1)
			},
			expectedStatus:            http.StatusBadRequest,
			expectErrorInBody:         true,
			expectedVolumePublication: nil,
		},
		{
			name:       "InternalOrchestratorError",
			volumeName: "error-volume",
			nodeName:   "error-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetVolumePublication(gomock.Any(), "error-volume", "error-node").Return(nil, errors.New("internal orchestrator error")).Times(1)
			},
			expectedStatus:            http.StatusBadRequest,
			expectErrorInBody:         true,
			expectedVolumePublication: nil,
		},
		{
			name:       "BootstrapError",
			volumeName: "bootstrap-volume",
			nodeName:   "bootstrap-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetVolumePublication(gomock.Any(), "bootstrap-volume", "bootstrap-node").Return(nil, errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:            http.StatusInternalServerError,
			expectErrorInBody:         true,
			expectedVolumePublication: nil,
		},
		{
			name:       "NotReadyError",
			volumeName: "not-ready-volume",
			nodeName:   "not-ready-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetVolumePublication(gomock.Any(), "not-ready-volume", "not-ready-node").Return(nil, errors.NotReadyError()).Times(1)
			},
			expectedStatus:            http.StatusServiceUnavailable,
			expectErrorInBody:         true,
			expectedVolumePublication: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/volumePublication/"+tt.volumeName+"/"+tt.nodeName, nil)
			req = mux.SetURLVars(req, map[string]string{"volume": tt.volumeName, "node": tt.nodeName})
			recorder := httptest.NewRecorder()

			// Call function
			GetVolumePublication(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response VolumePublicationResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
				assert.Nil(t, response.VolumePublication)
			} else {
				assert.Empty(t, response.Error)
				assert.NotNil(t, response.VolumePublication)
				assert.Equal(t, tt.expectedVolumePublication.Name, response.VolumePublication.Name)
				assert.Equal(t, tt.expectedVolumePublication.VolumeName, response.VolumePublication.VolumeName)
				assert.Equal(t, tt.expectedVolumePublication.NodeName, response.VolumePublication.NodeName)
			}
		})
	}
}

func TestListVolumePublications(t *testing.T) {
	tests := []struct {
		name                       string
		setupMock                  func(*mockcore.MockOrchestrator)
		expectedStatus             int
		expectErrorInBody          bool
		expectedCount              int
		expectedVolumePublications []*models.VolumePublicationExternal
	}{
		{
			name: "SuccessfulListingWithPublications",
			setupMock: func(m *mockcore.MockOrchestrator) {
				pubs := []*models.VolumePublicationExternal{
					{Name: "vol1-node1", VolumeName: "vol1", NodeName: "node1", ReadOnly: false, AccessMode: 1},
					{Name: "vol2-node2", VolumeName: "vol2", NodeName: "node2", ReadOnly: true, AccessMode: 2},
				}
				m.EXPECT().ListVolumePublications(gomock.Any()).Return(pubs, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedCount:     2,
			expectedVolumePublications: []*models.VolumePublicationExternal{
				{Name: "vol1-node1", VolumeName: "vol1", NodeName: "node1", ReadOnly: false, AccessMode: 1},
				{Name: "vol2-node2", VolumeName: "vol2", NodeName: "node2", ReadOnly: true, AccessMode: 2},
			},
		},
		{
			name: "SuccessfulListingEmptyList",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublications(gomock.Any()).Return([]*models.VolumePublicationExternal{}, nil).Times(1)
			},
			expectedStatus:             http.StatusOK,
			expectErrorInBody:          false,
			expectedCount:              0,
			expectedVolumePublications: []*models.VolumePublicationExternal{},
		},
		{
			name: "InternalOrchestratorError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublications(gomock.Any()).Return(nil, errors.New("internal orchestrator error")).Times(1)
			},
			expectedStatus:             http.StatusBadRequest,
			expectErrorInBody:          true,
			expectedCount:              0,
			expectedVolumePublications: nil,
		},
		{
			name: "BootstrapError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublications(gomock.Any()).Return(nil, errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:             http.StatusInternalServerError,
			expectErrorInBody:          true,
			expectedCount:              0,
			expectedVolumePublications: nil,
		},
		{
			name: "NotReadyError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublications(gomock.Any()).Return(nil, errors.NotReadyError()).Times(1)
			},
			expectedStatus:             http.StatusServiceUnavailable,
			expectErrorInBody:          true,
			expectedCount:              0,
			expectedVolumePublications: nil,
		},
		{
			name: "NotFoundError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublications(gomock.Any()).Return(nil, errors.NotFoundError("no publications found")).Times(1)
			},
			expectedStatus:             http.StatusNotFound,
			expectErrorInBody:          true,
			expectedCount:              0,
			expectedVolumePublications: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/volumePublication", nil)
			recorder := httptest.NewRecorder()

			// Call function
			ListVolumePublications(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response VolumePublicationsResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.Len(t, response.VolumePublications, tt.expectedCount)
				if tt.expectedCount > 0 {
					for i, expectedPub := range tt.expectedVolumePublications {
						assert.Equal(t, expectedPub.Name, response.VolumePublications[i].Name)
						assert.Equal(t, expectedPub.VolumeName, response.VolumePublications[i].VolumeName)
						assert.Equal(t, expectedPub.NodeName, response.VolumePublications[i].NodeName)
					}
				}
			}
		})
	}
}

func TestListVolumePublicationsForVolume(t *testing.T) {
	tests := []struct {
		name                       string
		volumeName                 string
		setupMock                  func(*mockcore.MockOrchestrator)
		expectedStatus             int
		expectErrorInBody          bool
		expectedCount              int
		expectedVolumePublications []*models.VolumePublicationExternal
	}{
		{
			name:       "SuccessfulListingForVolume",
			volumeName: "test-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				pubs := []*models.VolumePublicationExternal{
					{Name: "test-volume-node1", VolumeName: "test-volume", NodeName: "node1", ReadOnly: false, AccessMode: 1},
					{Name: "test-volume-node2", VolumeName: "test-volume", NodeName: "node2", ReadOnly: true, AccessMode: 2},
				}
				m.EXPECT().ListVolumePublicationsForVolume(gomock.Any(), "test-volume").Return(pubs, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedCount:     2,
			expectedVolumePublications: []*models.VolumePublicationExternal{
				{Name: "test-volume-node1", VolumeName: "test-volume", NodeName: "node1", ReadOnly: false, AccessMode: 1},
				{Name: "test-volume-node2", VolumeName: "test-volume", NodeName: "node2", ReadOnly: true, AccessMode: 2},
			},
		},
		{
			name:       "VolumeNotFound",
			volumeName: "nonexistent-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublicationsForVolume(gomock.Any(), "nonexistent-volume").Return(nil, errors.NotFoundError("volume not found")).Times(1)
			},
			expectedStatus:             http.StatusNotFound,
			expectErrorInBody:          true,
			expectedCount:              0,
			expectedVolumePublications: nil,
		},
		{
			name:       "EmptyPublicationsList",
			volumeName: "empty-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublicationsForVolume(gomock.Any(), "empty-volume").Return([]*models.VolumePublicationExternal{}, nil).Times(1)
			},
			expectedStatus:             http.StatusOK,
			expectErrorInBody:          false,
			expectedCount:              0,
			expectedVolumePublications: []*models.VolumePublicationExternal{},
		},
		{
			name:       "InternalOrchestratorError",
			volumeName: "error-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublicationsForVolume(gomock.Any(), "error-volume").Return(nil, errors.New("internal orchestrator error")).Times(1)
			},
			expectedStatus:             http.StatusBadRequest,
			expectErrorInBody:          true,
			expectedCount:              0,
			expectedVolumePublications: nil,
		},
		{
			name:       "BootstrapError",
			volumeName: "bootstrap-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublicationsForVolume(gomock.Any(), "bootstrap-volume").Return(nil, errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:             http.StatusInternalServerError,
			expectErrorInBody:          true,
			expectedCount:              0,
			expectedVolumePublications: nil,
		},
		{
			name:       "NotReadyError",
			volumeName: "not-ready-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublicationsForVolume(gomock.Any(), "not-ready-volume").Return(nil, errors.NotReadyError()).Times(1)
			},
			expectedStatus:             http.StatusServiceUnavailable,
			expectErrorInBody:          true,
			expectedCount:              0,
			expectedVolumePublications: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/volumePublication/volume/"+tt.volumeName, nil)
			req = mux.SetURLVars(req, map[string]string{"volume": tt.volumeName})
			recorder := httptest.NewRecorder()

			// Call function
			ListVolumePublicationsForVolume(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response VolumePublicationsResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.Len(t, response.VolumePublications, tt.expectedCount)
				if tt.expectedCount > 0 {
					for i, expectedPub := range tt.expectedVolumePublications {
						assert.Equal(t, expectedPub.Name, response.VolumePublications[i].Name)
						assert.Equal(t, expectedPub.VolumeName, response.VolumePublications[i].VolumeName)
						assert.Equal(t, expectedPub.NodeName, response.VolumePublications[i].NodeName)
					}
				}
			}
		})
	}
}

func TestListVolumePublicationsForNode(t *testing.T) {
	tests := []struct {
		name                       string
		nodeName                   string
		setupMock                  func(*mockcore.MockOrchestrator)
		expectedStatus             int
		expectErrorInBody          bool
		expectedCount              int
		expectedVolumePublications []*models.VolumePublicationExternal
	}{
		{
			name:     "SuccessfulListingForNode",
			nodeName: "test-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				pubs := []*models.VolumePublicationExternal{
					{Name: "vol1-test-node", VolumeName: "vol1", NodeName: "test-node", ReadOnly: false, AccessMode: 1},
					{Name: "vol2-test-node", VolumeName: "vol2", NodeName: "test-node", ReadOnly: true, AccessMode: 2},
				}
				m.EXPECT().ListVolumePublicationsForNode(gomock.Any(), "test-node").Return(pubs, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedCount:     2,
			expectedVolumePublications: []*models.VolumePublicationExternal{
				{Name: "vol1-test-node", VolumeName: "vol1", NodeName: "test-node", ReadOnly: false, AccessMode: 1},
				{Name: "vol2-test-node", VolumeName: "vol2", NodeName: "test-node", ReadOnly: true, AccessMode: 2},
			},
		},
		{
			name:     "NodeNotFound",
			nodeName: "nonexistent-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublicationsForNode(gomock.Any(), "nonexistent-node").Return(nil, errors.NotFoundError("node not found")).Times(1)
			},
			expectedStatus:             http.StatusNotFound,
			expectErrorInBody:          true,
			expectedCount:              0,
			expectedVolumePublications: nil,
		},
		{
			name:     "EmptyPublicationsList",
			nodeName: "empty-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublicationsForNode(gomock.Any(), "empty-node").Return([]*models.VolumePublicationExternal{}, nil).Times(1)
			},
			expectedStatus:             http.StatusOK,
			expectErrorInBody:          false,
			expectedCount:              0,
			expectedVolumePublications: []*models.VolumePublicationExternal{},
		},
		{
			name:     "InternalOrchestratorError",
			nodeName: "error-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublicationsForNode(gomock.Any(), "error-node").Return(nil, errors.New("internal orchestrator error")).Times(1)
			},
			expectedStatus:             http.StatusBadRequest,
			expectErrorInBody:          true,
			expectedCount:              0,
			expectedVolumePublications: nil,
		},
		{
			name:     "BootstrapError",
			nodeName: "bootstrap-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublicationsForNode(gomock.Any(), "bootstrap-node").Return(nil, errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:             http.StatusInternalServerError,
			expectErrorInBody:          true,
			expectedCount:              0,
			expectedVolumePublications: nil,
		},
		{
			name:     "NotReadyError",
			nodeName: "not-ready-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListVolumePublicationsForNode(gomock.Any(), "not-ready-node").Return(nil, errors.NotReadyError()).Times(1)
			},
			expectedStatus:             http.StatusServiceUnavailable,
			expectErrorInBody:          true,
			expectedCount:              0,
			expectedVolumePublications: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/volumePublication/node/"+tt.nodeName, nil)
			req = mux.SetURLVars(req, map[string]string{"node": tt.nodeName})
			recorder := httptest.NewRecorder()

			// Call function
			ListVolumePublicationsForNode(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response VolumePublicationsResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
				assert.Len(t, response.VolumePublications, tt.expectedCount)
				if tt.expectedCount > 0 {
					for i, expectedPub := range tt.expectedVolumePublications {
						assert.Equal(t, expectedPub.Name, response.VolumePublications[i].Name)
						assert.Equal(t, expectedPub.VolumeName, response.VolumePublications[i].VolumeName)
						assert.Equal(t, expectedPub.NodeName, response.VolumePublications[i].NodeName)
					}
				}
			}
		})
	}
}

// ============================================================================
// Tests for Priority 3 Functions with 0% Coverage - Snapshot Management
// ============================================================================

func TestGetSnapshot(t *testing.T) {
	tests := []struct {
		name              string
		volumeName        string
		snapshotName      string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedSnapshot  *storage.SnapshotExternal
	}{
		{
			name:         "SuccessfulSnapshotRetrieval",
			volumeName:   "test-volume",
			snapshotName: "test-snapshot",
			setupMock: func(m *mockcore.MockOrchestrator) {
				snapshot := &storage.SnapshotExternal{
					Snapshot: storage.Snapshot{
						Config: &storage.SnapshotConfig{
							Name:       "test-snapshot",
							VolumeName: "test-volume",
						},
						Created:   "2023-01-01T12:00:00Z",
						SizeBytes: 1073741824,
						State:     storage.SnapshotStateOnline,
					},
				}
				m.EXPECT().GetSnapshot(gomock.Any(), "test-volume", "test-snapshot").Return(snapshot, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedSnapshot: &storage.SnapshotExternal{
				Snapshot: storage.Snapshot{
					Config: &storage.SnapshotConfig{
						Name:       "test-snapshot",
						VolumeName: "test-volume",
					},
					Created:   "2023-01-01T12:00:00Z",
					SizeBytes: 1073741824,
					State:     storage.SnapshotStateOnline,
				},
			},
		},
		{
			name:         "SnapshotNotFound",
			volumeName:   "test-volume",
			snapshotName: "nonexistent-snapshot",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetSnapshot(gomock.Any(), "test-volume", "nonexistent-snapshot").Return(nil, errors.NotFoundError("snapshot not found")).Times(1)
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
			expectedSnapshot:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			req := httptest.NewRequest(http.MethodGet, "/trident/v1/snapshot/"+tt.volumeName+"/"+tt.snapshotName, nil)
			req = mux.SetURLVars(req, map[string]string{"volume": tt.volumeName, "snapshot": tt.snapshotName})
			recorder := httptest.NewRecorder()

			GetSnapshot(recorder, req)

			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response GetSnapshotResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
				assert.Nil(t, response.Snapshot)
			} else {
				assert.Empty(t, response.Error)
				assert.NotNil(t, response.Snapshot)
			}
		})
	}
}

func TestAddSnapshot(t *testing.T) {
	tests := []struct {
		name               string
		requestBody        string
		setupMock          func(*mockcore.MockOrchestrator)
		expectedStatus     int
		expectErrorInBody  bool
		expectedSnapshotID string
	}{
		{
			name:        "SuccessfulSnapshotCreation",
			requestBody: `{"name":"test-snapshot","volumeName":"test-volume"}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				snapshot := &storage.SnapshotExternal{
					Snapshot: storage.Snapshot{
						Config: &storage.SnapshotConfig{
							Name:       "test-snapshot",
							VolumeName: "test-volume",
						},
					},
				}
				m.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any()).Return(snapshot, nil).Times(1)
			},
			expectedStatus:     http.StatusCreated,
			expectErrorInBody:  false,
			expectedSnapshotID: "test-volume/test-snapshot",
		},
		{
			name:               "InvalidJSON",
			requestBody:        `{"invalid json"}`,
			setupMock:          func(m *mockcore.MockOrchestrator) {},
			expectedStatus:     http.StatusBadRequest,
			expectErrorInBody:  true,
			expectedSnapshotID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			req := httptest.NewRequest(http.MethodPost, "/trident/v1/snapshot", strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			AddSnapshot(recorder, req)

			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response AddSnapshotResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
				assert.Empty(t, response.SnapshotID)
			} else {
				assert.Empty(t, response.Error)
				assert.Equal(t, tt.expectedSnapshotID, response.SnapshotID)
			}
		})
	}
}

func TestDeleteSnapshot(t *testing.T) {
	tests := []struct {
		name              string
		volumeName        string
		snapshotName      string
		setupMock         func(*mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
	}{
		{
			name:         "SuccessfulDeletion",
			volumeName:   "test-volume",
			snapshotName: "test-snapshot",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteSnapshot(gomock.Any(), "test-volume", "test-snapshot").Return(nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:         "SnapshotNotFound",
			volumeName:   "test-volume",
			snapshotName: "nonexistent-snapshot",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteSnapshot(gomock.Any(), "test-volume", "nonexistent-snapshot").Return(errors.NotFoundError("snapshot not found")).Times(1)
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			req := httptest.NewRequest(http.MethodDelete, "/trident/v1/snapshot/"+tt.volumeName+"/"+tt.snapshotName, nil)
			req = mux.SetURLVars(req, map[string]string{"volume": tt.volumeName, "snapshot": tt.snapshotName})
			recorder := httptest.NewRecorder()

			DeleteSnapshot(recorder, req)

			assert.Equal(t, tt.expectedStatus, recorder.Code)

			var response DeleteResponse
			err := json.Unmarshal(recorder.Body.Bytes(), &response)
			assert.NoError(t, err)

			if tt.expectErrorInBody {
				assert.NotEmpty(t, response.Error)
			} else {
				assert.Empty(t, response.Error)
			}
		})
	}
}

func TestListSnapshots(t *testing.T) {
	tests := []struct {
		name              string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedContent   string
	}{
		{
			name: "SuccessfulSnapshotListing",
			setupMock: func(m *mockcore.MockOrchestrator) {
				snapshots := []*storage.SnapshotExternal{
					{
						Snapshot: storage.Snapshot{
							Config: &storage.SnapshotConfig{
								Version:    "1.0",
								Name:       "snapshot1",
								VolumeName: "volume1",
							},
							Created: time.Now().Format(time.RFC3339),
						},
					},
					{
						Snapshot: storage.Snapshot{
							Config: &storage.SnapshotConfig{
								Version:    "1.0",
								Name:       "snapshot2",
								VolumeName: "volume2",
							},
							Created: time.Now().Format(time.RFC3339),
						},
					},
				}
				m.EXPECT().ListSnapshots(gomock.Any()).Return(snapshots, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "snapshot1",
		},
		{
			name: "EmptySnapshotList",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListSnapshots(gomock.Any()).Return([]*storage.SnapshotExternal{}, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "[]",
		},
		{
			name: "OrchestratorError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListSnapshots(gomock.Any()).Return(nil, errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name: "BootstrapError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListSnapshots(gomock.Any()).Return(nil, errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/snapshots", nil)
			recorder := httptest.NewRecorder()

			// Call function
			ListSnapshots(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedContent != "" {
				assert.Contains(t, recorder.Body.String(), tt.expectedContent)
			}
		})
	}
}

func TestListSnapshotsForVolume(t *testing.T) {
	tests := []struct {
		name              string
		volumeName        string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedContent   string
	}{
		{
			name:       "SuccessfulSnapshotListingForVolume",
			volumeName: "test-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				snapshots := []*storage.SnapshotExternal{
					{
						Snapshot: storage.Snapshot{
							Config: &storage.SnapshotConfig{
								Version:    "1.0",
								Name:       "snapshot1",
								VolumeName: "test-volume",
							},
							Created: time.Now().Format(time.RFC3339),
						},
					},
					{
						Snapshot: storage.Snapshot{
							Config: &storage.SnapshotConfig{
								Version:    "1.0",
								Name:       "snapshot2",
								VolumeName: "test-volume",
							},
							Created: time.Now().Format(time.RFC3339),
						},
					},
				}
				m.EXPECT().ListSnapshotsForVolume(gomock.Any(), "test-volume").Return(snapshots, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "test-volume/snapshot1",
		},
		{
			name:       "EmptySnapshotListForVolume",
			volumeName: "empty-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListSnapshotsForVolume(gomock.Any(), "empty-volume").Return([]*storage.SnapshotExternal{}, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "[]",
		},
		{
			name:       "VolumeNotFound",
			volumeName: "nonexistent-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListSnapshotsForVolume(gomock.Any(), "nonexistent-volume").Return(nil, errors.NotFoundError("volume not found")).Times(1)
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
		},
		{
			name:       "OrchestratorError",
			volumeName: "error-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListSnapshotsForVolume(gomock.Any(), "error-volume").Return(nil, errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name:       "BootstrapError",
			volumeName: "bootstrap-volume",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListSnapshotsForVolume(gomock.Any(), "bootstrap-volume").Return(nil, errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request with URL vars
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/volume/"+tt.volumeName+"/snapshots", nil)
			req = mux.SetURLVars(req, map[string]string{"volume": tt.volumeName})
			recorder := httptest.NewRecorder()

			// Call function
			ListSnapshotsForVolume(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedContent != "" {
				assert.Contains(t, recorder.Body.String(), tt.expectedContent)
			}
		})
	}
}

// ============================================================================
// Group Snapshot Function Tests
// ============================================================================

func TestGetGroupSnapshot(t *testing.T) {
	tests := []struct {
		name              string
		groupName         string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedContent   string
	}{
		{
			name:      "SuccessfulGroupSnapshotRetrieval",
			groupName: "test-group",
			setupMock: func(m *mockcore.MockOrchestrator) {
				config := &storage.GroupSnapshotConfig{
					Name: "test-group",
				}
				groupSnapshot := storage.NewGroupSnapshot(config, []string{"snap1", "snap2"}, "2023-01-01T00:00:00Z")
				groupSnapshotExternal := &storage.GroupSnapshotExternal{
					GroupSnapshot: *groupSnapshot,
				}
				m.EXPECT().GetGroupSnapshot(gomock.Any(), "test-group").Return(groupSnapshotExternal, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "test-group",
		},
		{
			name:      "GroupSnapshotNotFound",
			groupName: "nonexistent-group",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetGroupSnapshot(gomock.Any(), "nonexistent-group").Return(nil, errors.NotFoundError("group snapshot not found")).Times(1)
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
		},
		{
			name:      "OrchestratorError",
			groupName: "error-group",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetGroupSnapshot(gomock.Any(), "error-group").Return(nil, errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name:      "BootstrapError",
			groupName: "bootstrap-group",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetGroupSnapshot(gomock.Any(), "bootstrap-group").Return(nil, errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request with URL vars
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/groupsnapshot/"+tt.groupName, nil)
			req = mux.SetURLVars(req, map[string]string{"group": tt.groupName})
			recorder := httptest.NewRecorder()

			// Call function
			GetGroupSnapshot(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedContent != "" {
				assert.Contains(t, recorder.Body.String(), tt.expectedContent)
			}
		})
	}
}

func TestListGroupSnapshots(t *testing.T) {
	tests := []struct {
		name              string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedContent   string
	}{
		{
			name: "SuccessfulGroupSnapshotListing",
			setupMock: func(m *mockcore.MockOrchestrator) {
				config1 := &storage.GroupSnapshotConfig{Name: "group1"}
				config2 := &storage.GroupSnapshotConfig{Name: "group2"}
				groupSnapshot1 := storage.NewGroupSnapshot(config1, []string{"snap1"}, "2023-01-01T00:00:00Z")
				groupSnapshot2 := storage.NewGroupSnapshot(config2, []string{"snap2"}, "2023-01-02T00:00:00Z")
				groupSnapshots := []*storage.GroupSnapshotExternal{
					{GroupSnapshot: *groupSnapshot1},
					{GroupSnapshot: *groupSnapshot2},
				}
				m.EXPECT().ListGroupSnapshots(gomock.Any()).Return(groupSnapshots, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "group1",
		},
		{
			name: "EmptyGroupSnapshotList",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListGroupSnapshots(gomock.Any()).Return([]*storage.GroupSnapshotExternal{}, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "[]",
		},
		{
			name: "OrchestratorError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListGroupSnapshots(gomock.Any()).Return(nil, errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name: "BootstrapError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListGroupSnapshots(gomock.Any()).Return(nil, errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/groupsnapshots", nil)
			recorder := httptest.NewRecorder()

			// Call function
			ListGroupSnapshots(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedContent != "" {
				assert.Contains(t, recorder.Body.String(), tt.expectedContent)
			}
		})
	}
}

func TestDeleteGroupSnapshot(t *testing.T) {
	tests := []struct {
		name              string
		groupName         string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
	}{
		{
			name:      "SuccessfulDeletion",
			groupName: "test-group",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteGroupSnapshot(gomock.Any(), "test-group").Return(nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:      "GroupSnapshotNotFound",
			groupName: "nonexistent-group",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteGroupSnapshot(gomock.Any(), "nonexistent-group").Return(errors.NotFoundError("group snapshot not found")).Times(1)
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
		},
		{
			name:      "ConflictError",
			groupName: "conflict-group",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteGroupSnapshot(gomock.Any(), "conflict-group").Return(errors.ConflictError("group snapshot in use")).Times(1)
			},
			expectedStatus:    http.StatusConflict,
			expectErrorInBody: true,
		},
		{
			name:      "OrchestratorError",
			groupName: "error-group",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteGroupSnapshot(gomock.Any(), "error-group").Return(errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name:      "BootstrapError",
			groupName: "bootstrap-group",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().DeleteGroupSnapshot(gomock.Any(), "bootstrap-group").Return(errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request with URL vars
			req := httptest.NewRequest(http.MethodDelete, "/trident/v1/groupsnapshot/"+tt.groupName, nil)
			req = mux.SetURLVars(req, map[string]string{"group": tt.groupName})
			recorder := httptest.NewRecorder()

			// Call function
			DeleteGroupSnapshot(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)
		})
	}
}

// ============================================================================
// CHAP Function Tests
// ============================================================================

func TestGetCHAP(t *testing.T) {
	tests := []struct {
		name              string
		volumeName        string
		nodeName          string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedContent   string
	}{
		{
			name:       "SuccessfulCHAPRetrieval",
			volumeName: "test-volume",
			nodeName:   "test-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				chapInfo := &models.IscsiChapInfo{
					UseCHAP:              true,
					IscsiUsername:        "testuser",
					IscsiInitiatorSecret: "secret123",
					IscsiTargetUsername:  "targetuser",
					IscsiTargetSecret:    "targetsecret456",
				}
				m.EXPECT().GetCHAP(gomock.Any(), "test-volume", "test-node").Return(chapInfo, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "testuser",
		},
		{
			name:       "CHAPNotFound",
			volumeName: "nonexistent-volume",
			nodeName:   "test-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetCHAP(gomock.Any(), "nonexistent-volume", "test-node").Return(nil, errors.NotFoundError("CHAP info not found")).Times(1)
			},
			expectedStatus:    http.StatusNotFound,
			expectErrorInBody: true,
		},
		{
			name:       "OrchestratorError",
			volumeName: "error-volume",
			nodeName:   "test-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetCHAP(gomock.Any(), "error-volume", "test-node").Return(nil, errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name:       "BootstrapError",
			volumeName: "bootstrap-volume",
			nodeName:   "test-node",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetCHAP(gomock.Any(), "bootstrap-volume", "test-node").Return(nil, errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request with URL vars
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/volume/"+tt.volumeName+"/node/"+tt.nodeName+"/chap", nil)
			req = mux.SetURLVars(req, map[string]string{"volume": tt.volumeName, "node": tt.nodeName})
			recorder := httptest.NewRecorder()

			// Call function
			GetCHAP(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedContent != "" {
				assert.Contains(t, recorder.Body.String(), tt.expectedContent)
			}
		})
	}
}

// ============================================================================
// Logging Function Tests
// ============================================================================

func TestGetCurrentLogLevel(t *testing.T) {
	tests := []struct {
		name              string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedContent   string
	}{
		{
			name: "SuccessfulLogLevelRetrieval",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetLogLevel(gomock.Any()).Return("info", nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "info",
		},
		{
			name: "OrchestratorError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetLogLevel(gomock.Any()).Return("", errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name: "BootstrapError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetLogLevel(gomock.Any()).Return("", errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/logs/level", nil)
			recorder := httptest.NewRecorder()

			// Call function
			GetCurrentLogLevel(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedContent != "" {
				assert.Contains(t, recorder.Body.String(), tt.expectedContent)
			}
		})
	}
}

func TestSetLogLevel(t *testing.T) {
	tests := []struct {
		name              string
		logLevel          string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
	}{
		{
			name:     "SuccessfulLogLevelSet",
			logLevel: "debug",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().SetLogLevel(gomock.Any(), "debug").Return(nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:     "InvalidLogLevel",
			logLevel: "invalid",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().SetLogLevel(gomock.Any(), "invalid").Return(errors.InvalidInputError("invalid log level")).Times(1)
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
		},
		{
			name:     "OrchestratorError",
			logLevel: "error",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().SetLogLevel(gomock.Any(), "error").Return(errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name:     "BootstrapError",
			logLevel: "warn",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().SetLogLevel(gomock.Any(), "warn").Return(errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request with URL vars
			req := httptest.NewRequest(http.MethodPut, "/trident/v1/logs/level/"+tt.logLevel, nil)
			req = mux.SetURLVars(req, map[string]string{"level": tt.logLevel})
			recorder := httptest.NewRecorder()

			// Call function
			SetLogLevel(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)
		})
	}
}

func TestGetLoggingWorkflows(t *testing.T) {
	tests := []struct {
		name              string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedContent   string
	}{
		{
			name: "SuccessfulWorkflowsRetrieval",
			setupMock: func(m *mockcore.MockOrchestrator) {
				workflows := "workflow1,workflow2,workflow3"
				m.EXPECT().GetSelectedLoggingWorkflows(gomock.Any()).Return(workflows, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "workflow1",
		},
		{
			name: "EmptyWorkflows",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetSelectedLoggingWorkflows(gomock.Any()).Return("", nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name: "OrchestratorError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetSelectedLoggingWorkflows(gomock.Any()).Return("", errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name: "BootstrapError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetSelectedLoggingWorkflows(gomock.Any()).Return("", errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/logs/workflows", nil)
			recorder := httptest.NewRecorder()

			// Call function
			GetLoggingWorkflows(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedContent != "" {
				assert.Contains(t, recorder.Body.String(), tt.expectedContent)
			}
		})
	}
}

func TestListLoggingWorkflows(t *testing.T) {
	tests := []struct {
		name              string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedContent   string
	}{
		{
			name: "SuccessfulWorkflowListing",
			setupMock: func(m *mockcore.MockOrchestrator) {
				workflows := []string{"workflow1", "workflow2", "workflow3", "workflow4"}
				m.EXPECT().ListLoggingWorkflows(gomock.Any()).Return(workflows, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "workflow1",
		},
		{
			name: "EmptyWorkflowList",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListLoggingWorkflows(gomock.Any()).Return([]string{}, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "[]",
		},
		{
			name: "OrchestratorError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListLoggingWorkflows(gomock.Any()).Return(nil, errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name: "BootstrapError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListLoggingWorkflows(gomock.Any()).Return(nil, errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/logs/workflows/available", nil)
			recorder := httptest.NewRecorder()

			// Call function
			ListLoggingWorkflows(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedContent != "" {
				assert.Contains(t, recorder.Body.String(), tt.expectedContent)
			}
		})
	}
}

func TestSetLoggingWorkflows(t *testing.T) {
	tests := []struct {
		name              string
		requestBody       string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
	}{
		{
			name:        "SuccessfulWorkflowsSet",
			requestBody: `{"logWorkflows": "workflow1,workflow2"}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().SetLoggingWorkflows(gomock.Any(), "workflow1,workflow2").Return(nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:        "InvalidJSON",
			requestBody: `{"invalid": json}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				// No expectation as function should fail before calling orchestrator
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
		},
		{
			name:        "EmptyWorkflows",
			requestBody: `{"logWorkflows": ""}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().SetLoggingWorkflows(gomock.Any(), "").Return(nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:        "OrchestratorError",
			requestBody: `{"logWorkflows": "workflow1"}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().SetLoggingWorkflows(gomock.Any(), "workflow1").Return(errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name:        "BootstrapError",
			requestBody: `{"logWorkflows": "workflow2"}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().SetLoggingWorkflows(gomock.Any(), "workflow2").Return(errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request with body
			req := httptest.NewRequest(http.MethodPut, "/trident/v1/logs/workflows", strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			// Call function
			SetLoggingWorkflows(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)
		})
	}
}

func TestGetLoggingLayers(t *testing.T) {
	tests := []struct {
		name              string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedContent   string
	}{
		{
			name: "SuccessfulLayersRetrieval",
			setupMock: func(m *mockcore.MockOrchestrator) {
				layers := "api,csi,core"
				m.EXPECT().GetSelectedLogLayers(gomock.Any()).Return(layers, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "api",
		},
		{
			name: "EmptyLayers",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetSelectedLogLayers(gomock.Any()).Return("", nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name: "OrchestratorError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetSelectedLogLayers(gomock.Any()).Return("", errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name: "BootstrapError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().GetSelectedLogLayers(gomock.Any()).Return("", errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/logs/layers", nil)
			recorder := httptest.NewRecorder()

			// Call function
			GetLoggingLayers(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedContent != "" {
				assert.Contains(t, recorder.Body.String(), tt.expectedContent)
			}
		})
	}
}

func TestListLoggingLayers(t *testing.T) {
	tests := []struct {
		name              string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
		expectedContent   string
	}{
		{
			name: "SuccessfulLayerListing",
			setupMock: func(m *mockcore.MockOrchestrator) {
				layers := []string{"api", "csi", "core", "storage_drivers"}
				m.EXPECT().ListLogLayers(gomock.Any()).Return(layers, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "api",
		},
		{
			name: "EmptyLayerList",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListLogLayers(gomock.Any()).Return([]string{}, nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
			expectedContent:   "[]",
		},
		{
			name: "OrchestratorError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListLogLayers(gomock.Any()).Return(nil, errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name: "BootstrapError",
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().ListLogLayers(gomock.Any()).Return(nil, errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request
			req := httptest.NewRequest(http.MethodGet, "/trident/v1/logs/layers/available", nil)
			recorder := httptest.NewRecorder()

			// Call function
			ListLoggingLayers(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedContent != "" {
				assert.Contains(t, recorder.Body.String(), tt.expectedContent)
			}
		})
	}
}

func TestSetLoggingLayers(t *testing.T) {
	tests := []struct {
		name              string
		requestBody       string
		setupMock         func(m *mockcore.MockOrchestrator)
		expectedStatus    int
		expectErrorInBody bool
	}{
		{
			name:        "SuccessfulLayersSet",
			requestBody: `{"logLayers": "api,csi,core"}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().SetLogLayers(gomock.Any(), "api,csi,core").Return(nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:        "InvalidJSON",
			requestBody: `{"invalid": json}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				// No expectation as function should fail before calling orchestrator
			},
			expectedStatus:    http.StatusBadRequest,
			expectErrorInBody: true,
		},
		{
			name:        "EmptyLayers",
			requestBody: `{"logLayers": ""}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().SetLogLayers(gomock.Any(), "").Return(nil).Times(1)
			},
			expectedStatus:    http.StatusOK,
			expectErrorInBody: false,
		},
		{
			name:        "OrchestratorError",
			requestBody: `{"logLayers": "api"}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().SetLogLayers(gomock.Any(), "api").Return(errors.NotReadyError()).Times(1)
			},
			expectedStatus:    http.StatusServiceUnavailable,
			expectErrorInBody: true,
		},
		{
			name:        "BootstrapError",
			requestBody: `{"logLayers": "csi"}`,
			setupMock: func(m *mockcore.MockOrchestrator) {
				m.EXPECT().SetLogLayers(gomock.Any(), "csi").Return(errors.BootstrapError(errors.New("bootstrap error"))).Times(1)
			},
			expectedStatus:    http.StatusInternalServerError,
			expectErrorInBody: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up mocks
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockOrchestrator := mockcore.NewMockOrchestrator(mockCtrl)

			oldOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() { orchestrator = oldOrchestrator }()

			tt.setupMock(mockOrchestrator)

			// Create request with body
			req := httptest.NewRequest(http.MethodPut, "/trident/v1/logs/layers", strings.NewReader(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()

			// Call function
			SetLoggingLayers(recorder, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, recorder.Code)
		})
	}
}

// ============================================================================
// Utility Method Tests for Response Structs
// ============================================================================

func TestAddNodeResponse_Methods(t *testing.T) {
	t.Run("setTopologyLabels", func(t *testing.T) {
		response := &AddNodeResponse{}
		labels := map[string]string{
			"zone":   "us-east-1a",
			"region": "us-east-1",
		}

		response.setTopologyLabels(labels)

		assert.Equal(t, labels, response.TopologyLabels)
	})

	t.Run("logSuccess", func(t *testing.T) {
		response := &AddNodeResponse{Name: "test-node"}
		ctx := context.Background()

		// This should not panic
		assert.NotPanics(t, func() {
			response.logSuccess(ctx)
		})
	})
}

func TestUpdateNodeResponse_Methods(t *testing.T) {
	t.Run("setError", func(t *testing.T) {
		response := &UpdateNodeResponse{}
		err := errors.New("test error")

		response.setError(err)

		assert.Equal(t, "test error", response.Error)
	})

	t.Run("isError", func(t *testing.T) {
		response := &UpdateNodeResponse{}

		// Initially no error
		assert.False(t, response.isError())

		// Set error
		response.Error = "some error"
		assert.True(t, response.isError())
	})

	t.Run("logSuccess", func(t *testing.T) {
		response := &UpdateNodeResponse{Name: "test-node"}
		ctx := context.Background()

		// This should not panic
		assert.NotPanics(t, func() {
			response.logSuccess(ctx)
		})
	})

	t.Run("logFailure", func(t *testing.T) {
		response := &UpdateNodeResponse{Name: "test-node", Error: "test error"}
		ctx := context.Background()

		// This should not panic
		assert.NotPanics(t, func() {
			response.logFailure(ctx)
		})
	})
}

func TestVolumePublicationsResponse_Methods(t *testing.T) {
	t.Run("setError", func(t *testing.T) {
		response := &VolumePublicationsResponse{}
		err := errors.New("test error")

		response.setError(err)

		assert.Equal(t, "test error", response.Error)
	})

	t.Run("isError", func(t *testing.T) {
		response := &VolumePublicationsResponse{}

		// Initially no error
		assert.False(t, response.isError())

		// Set error
		response.Error = "some error"
		assert.True(t, response.isError())
	})

	t.Run("logSuccess", func(t *testing.T) {
		response := &VolumePublicationsResponse{}
		ctx := context.Background()

		// This should not panic
		assert.NotPanics(t, func() {
			response.logSuccess(ctx)
		})
	})

	t.Run("logFailure", func(t *testing.T) {
		response := &VolumePublicationsResponse{Error: "test error"}
		ctx := context.Background()

		// This should not panic
		assert.NotPanics(t, func() {
			response.logFailure(ctx)
		})
	})
}
