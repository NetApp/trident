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

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	http_test "github.com/stretchr/testify/http"

	"github.com/netapp/trident/frontend"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	mockk8scontrollerhelper "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_helpers/mock_kubernetes_helper"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
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
	mockOrchestrator.EXPECT().UpdateVolume(request.Context(), volume.Config.Name, &[]string{}).Return(nil)

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
	mockOrchestrator.EXPECT().UpdateVolume(request.Context(), volume.Config.Name, &[]string{"super-secret-passphrase-1"}).Return(nil)

	rc = volumeLUKSPassphraseNamesUpdater(writer, request, response, map[string]string{"volume": volume.Config.Name}, []byte(body))

	assert.Equal(t, http.StatusOK, rc)
	assert.Equal(t, volume, response.Volume)
	assert.Equal(t, "", response.Error)
	mockCtrl.Finish()

	// ////////////////////////////////////////////////////////////////////////////////////////////////////////////
	// Negative case: Invalid response object provided
	volume = &storage.VolumeExternal{Config: &storage.VolumeConfig{Name: "test"}}
	writer = &http_test.TestResponseWriter{}
	invalidResponse := &UpgradeVolumeResponse{} // Wrong type!
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
	mockOrchestrator.EXPECT().UpdateVolume(request.Context(), volume.Config.Name, &[]string{"super-secret-passphrase-1"}).Return(fmt.Errorf("test error"))

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
	mockOrchestrator.EXPECT().UpdateVolume(request.Context(), volume.Config.Name, &[]string{"super-secret-passphrase-1"}).Return(errors.NotFoundError("test error"))

	rc = volumeLUKSPassphraseNamesUpdater(writer, request, response, map[string]string{"volume": volume.Config.Name}, []byte(body))

	assert.Equal(t, http.StatusNotFound, rc)
	assert.Equal(t, volume, response.Volume)
	mockCtrl.Finish()
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
	nodeStateFlags := &utils.NodePublicationStateFlags{
		OrchestratorReady:  utils.Ptr(true),
		AdministratorReady: utils.Ptr(true),
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
			DoAndReturn(func(_ context.Context) ([]utils.Node, error) {
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
			DoAndReturn(func(_ context.Context, _ string, _ *utils.NodePublicationStateFlags) error {
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

		nodeState := utils.NodePublicationStateFlags{ProvisionerReady: utils.Ptr(true)}
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
	nodeExternal := &utils.NodeExternal{Name: nodeName}
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
