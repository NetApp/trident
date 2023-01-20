package rest

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	http_test "github.com/stretchr/testify/http"

	mockcore "github.com/netapp/trident/mocks/mock_core"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
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
	mockOrchestrator.EXPECT().GetVolume(request.Context(), volume.Config.Name).Return(volume, utils.NotFoundError("test error"))

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
	mockOrchestrator.EXPECT().UpdateVolume(request.Context(), volume.Config.Name, &[]string{"super-secret-passphrase-1"}).Return(utils.NotFoundError("test error"))

	rc = volumeLUKSPassphraseNamesUpdater(writer, request, response, map[string]string{"volume": volume.Config.Name}, []byte(body))

	assert.Equal(t, http.StatusNotFound, rc)
	assert.Equal(t, volume, response.Volume)
	mockCtrl.Finish()
}
