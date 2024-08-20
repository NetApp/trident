package rest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/internal/fiji/store"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/mocks/mock_internal/mock_fiji/mock_rest"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

type fakeFault struct {
	Name     string `json:"name,omitempty"`
	Location string `json:"location,omitempty"`
	Handler  any    `json:"handler,omitempty"`
}

func (f *fakeFault) Inject() error {
	return nil
}

func (f *fakeFault) Reset() {
	f.Handler = nil
}

func (f *fakeFault) IsHandlerSet() bool {
	return f.Handler != nil
}

func (f *fakeFault) SetHandler(m []byte) error {
	f.Handler = string(m)
	return nil
}

// newHttpTestServer sets up a httptest server with a supplied handler and pattern.
func newHttpTestServer(t *testing.T, pattern string, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	router := mux.NewRouter()
	router.HandleFunc(pattern, handler)
	return httptest.NewServer(router)
}

func TestHandlers_makeListFaultsHandler(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	t.Run("with 1 fault found", func(t *testing.T) {
		fake := &fakeFault{
			Name:     "fault",
			Location: "location",
			Handler:  `{"name":"handler-name"}`,
		}

		mockStore := mock_rest.NewMockFaultStore(mockCtrl)
		mockStore.EXPECT().List().Return([]store.Fault{fake})

		handler := makeListFaultsHandler(mockStore)
		server := httptest.NewServer(handler)
		defer server.Close()

		resp, err := http.Get(server.URL)
		assert.NotNil(t, resp, "expected non-nil response")
		assert.NoError(t, err, "expected error")

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			assert.FailNow(t, err.Error())
		}

		var respBody []fakeFault
		if err := json.Unmarshal(body, &respBody); err != nil {
			assert.FailNow(t, err.Error())
		}
		assert.Len(t, respBody, 1)

		externalFault := respBody[0]
		assert.Equal(t, fake.Name, externalFault.Name)
		assert.Equal(t, fake.Location, externalFault.Location)
		assert.EqualValues(t, fake.Handler, externalFault.Handler)
	})

	t.Run("with 1 enabled fault found", func(t *testing.T) {
		fake := &fakeFault{
			Name:     "fault",
			Location: "location",
			Handler:  `{"name":"handler-name"}`,
		}

		fakeFaults := []store.Fault{
			fake,
			&fakeFault{
				Name:     "fault2",
				Location: "location",
				// model:    []byte(`{"name":"handler-name"}`),
			},
		}

		mockStore := mock_rest.NewMockFaultStore(mockCtrl)
		mockStore.EXPECT().List().Return(fakeFaults)

		handler := makeListFaultsHandler(mockStore)
		server := httptest.NewServer(handler)
		defer server.Close()

		resp, err := http.Get(server.URL + "?enabled=true")
		assert.NotNil(t, resp, "expected non-nil response")
		assert.NoError(t, err, "expected error")

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			assert.FailNow(t, err.Error())
		}

		var listFaultRespBody []fakeFault
		if err := json.Unmarshal(body, &listFaultRespBody); err != nil {
			assert.FailNow(t, err.Error())
		}
		assert.Len(t, listFaultRespBody, 1)

		externalFault := listFaultRespBody[0]
		assert.Equal(t, fake.Name, externalFault.Name)
		assert.Equal(t, fake.Location, externalFault.Location)
		assert.EqualValues(t, fake.Handler, externalFault.Handler)
		assert.Equal(t, fake.IsHandlerSet(), externalFault.IsHandlerSet())
	})

	t.Run("with no enabled fault found", func(t *testing.T) {
		// Create fake faults with no handler specified.
		fakeFaults := []store.Fault{
			&fakeFault{
				Name:     "fault",
				Location: "location",
			},
			&fakeFault{
				Name:     "fault",
				Location: "location",
			},
		}

		mockStore := mock_rest.NewMockFaultStore(mockCtrl)
		mockStore.EXPECT().List().Return(fakeFaults)

		handler := makeListFaultsHandler(mockStore)
		server := httptest.NewServer(handler)
		defer server.Close()

		resp, err := http.Get(server.URL + "?enabled=true")
		assert.NotNil(t, resp, "expected non-nil response")
		assert.NoError(t, err, "expected error")

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			assert.FailNow(t, err.Error())
		}

		var listFaultRespBody []fakeFault
		if err := json.Unmarshal(body, &listFaultRespBody); err != nil {
			assert.FailNow(t, err.Error())
		}
		assert.Len(t, listFaultRespBody, 0)
	})
}

func TestHandlers_makeGetFaultHandler(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	t.Run("with no name id specified", func(t *testing.T) {
		mockStore := mock_rest.NewMockFaultStore(mockCtrl)
		server := newHttpTestServer(t, "/{name}", makeGetFaultHandler(mockStore))
		defer server.Close()

		resp, _ := http.Get(fmt.Sprintf("%s/%s", server.URL, ""))
		assert.NotNil(t, resp, "expected non-nil response")
	})

	t.Run("with name id and no fault", func(t *testing.T) {
		fake := &fakeFault{
			Name:     "fault",
			Location: "location",
			Handler:  `{"name":"handler-name"}`,
		}

		mockStore := mock_rest.NewMockFaultStore(mockCtrl)
		mockStore.EXPECT().Get(fake.Name).Return(nil, false)

		server := newHttpTestServer(t, "/{name}", makeGetFaultHandler(mockStore))
		defer server.Close()

		resp, _ := http.Get(fmt.Sprintf("%s/%s", server.URL, fake.Name))
		assert.NotNil(t, resp, "expected non-nil response")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "expected status codes to match")
	})

	t.Run("with name id", func(t *testing.T) {
		fake := &fakeFault{
			Name:     "fault",
			Location: "location",
			Handler:  `{"name":"handler-name"}`,
		}

		mockStore := mock_rest.NewMockFaultStore(mockCtrl)
		mockStore.EXPECT().Get(fake.Name).Return(fake, true)

		server := newHttpTestServer(t, "/{name}", makeGetFaultHandler(mockStore))
		defer server.Close()

		resp, _ := http.Get(fmt.Sprintf("%s/%s", server.URL, fake.Name))
		assert.NotNil(t, resp, "expected non-nil response")
		assert.Equal(t, http.StatusOK, resp.StatusCode, "expected status codes to match")

		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			assert.FailNow(t, err.Error())
		}

		var getFaultRespBody fakeFault
		if err := json.Unmarshal(body, &getFaultRespBody); err != nil {
			assert.FailNow(t, err.Error())
		}

		assert.Equal(t, fake.Name, getFaultRespBody.Name)
		assert.Equal(t, fake.Location, getFaultRespBody.Location)
		assert.EqualValues(t, fake.Handler, getFaultRespBody.Handler)
	})
}

func TestHandlers_makeSetFaultConfigHandler(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	t.Run("with empty name id specified", func(t *testing.T) {
		mockStore := mock_rest.NewMockFaultStore(mockCtrl)
		server := newHttpTestServer(t, "/{name}", makeSetFaultConfigHandler(mockStore))
		defer server.Close()

		resp, _ := http.Get(fmt.Sprintf("%s/%s", server.URL, ""))
		assert.NotNil(t, resp, "expected non-nil response")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "expected status codes to match")
	})

	t.Run("with name id and no fault", func(t *testing.T) {
		fake := &fakeFault{
			Name:     "fault",
			Location: "location",
			Handler:  `{"name":"handler-name"}`,
		}

		mockStore := mock_rest.NewMockFaultStore(mockCtrl)
		mockStore.EXPECT().Exists(fake.Name).Return(false)

		server := newHttpTestServer(t, "/{name}", makeSetFaultConfigHandler(mockStore))
		defer server.Close()

		url := fmt.Sprintf("%s/%s", server.URL, fake.Name)
		request, err := http.NewRequest("PATCH", url, nil)
		if err != nil {
			assert.FailNow(t, err.Error())
		}

		resp, err := server.Client().Do(request)
		if err != nil {
			assert.FailNow(t, err.Error())
		}
		assert.NotNil(t, resp, "expected non-nil response")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "expected status codes to match")
	})

	t.Run("with name id and valid handler config", func(t *testing.T) {
		name := "fault-name"
		mockStore := mock_rest.NewMockFaultStore(mockCtrl)
		mockStore.EXPECT().Exists(name).Return(true)
		mockStore.EXPECT().Set(name, gomock.Any()).Return(nil)

		server := newHttpTestServer(t, "/{name}", makeSetFaultConfigHandler(mockStore))
		defer server.Close()

		// Simulate a PATCH request with a payload.
		configRequest := `{"name": "pause", "duration": "400ms"}`
		body, err := json.Marshal(configRequest)
		if err != nil {
			t.Fatalf("Failed to marshal JSON: %v", err)
		}

		url := fmt.Sprintf("%s/%s", server.URL, name)
		request, err := http.NewRequest("PATCH", url, bytes.NewBuffer(body))
		if err != nil {
			assert.FailNow(t, err.Error())
		}

		// Invoke the API.
		resp, err := server.Client().Do(request)
		if err != nil {
			assert.FailNow(t, err.Error())
		}
		assert.NotNil(t, resp, "expected non-nil response")
		assert.Equal(t, http.StatusAccepted, resp.StatusCode, "expected status codes to match")
	})

	t.Run("with name id and empty handler config", func(t *testing.T) {
		name := "fault-name"
		mockStore := mock_rest.NewMockFaultStore(mockCtrl)
		mockStore.EXPECT().Exists(name).Return(true)

		server := newHttpTestServer(t, "/{name}", makeSetFaultConfigHandler(mockStore))
		defer server.Close()

		// Simulate a PATCH request with a payload.
		url := fmt.Sprintf("%s/%s", server.URL, name)
		request, err := http.NewRequest("PATCH", url, bytes.NewBuffer(nil))
		if err != nil {
			assert.FailNow(t, err.Error())
		}

		// Invoke the API.
		resp, err := server.Client().Do(request)
		if err != nil {
			assert.FailNow(t, err.Error())
		}
		assert.NotNil(t, resp, "expected non-nil response")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "expected status codes to match")
	})

	t.Run("with name id and error when setting fault handler config", func(t *testing.T) {
		name := "fault-name"
		mockStore := mock_rest.NewMockFaultStore(mockCtrl)
		mockStore.EXPECT().Exists(name).Return(true)
		mockStore.EXPECT().Set(name, gomock.Any()).Return(errors.New("failed to set handler config"))

		server := newHttpTestServer(t, "/{name}", makeSetFaultConfigHandler(mockStore))
		defer server.Close()

		// Simulate a PATCH request with a payload.
		configRequest := `{"name": "pause", "duration": "400ms"}`
		body, err := json.Marshal(configRequest)
		if err != nil {
			t.Fatalf("Failed to marshal JSON: %v", err)
		}

		url := fmt.Sprintf("%s/%s", server.URL, name)
		request, err := http.NewRequest("PATCH", url, bytes.NewBuffer(body))
		if err != nil {
			assert.FailNow(t, err.Error())
		}

		// Invoke the API.
		resp, err := server.Client().Do(request)
		if err != nil {
			assert.FailNow(t, err.Error())
		}
		assert.NotNil(t, resp, "expected non-nil response")
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode, "expected status codes to match")
	})
}

func TestHandlers_makeResetFaultConfigHandler(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	tt := map[string]struct {
		status         int
		name           string
		setMocks       func(store *mock_rest.MockFaultStore)
		assertNilOrNot assert.ValueAssertionFunc
	}{
		"with name id but no fault found": {
			status:         http.StatusNotFound,
			name:           "name",
			assertNilOrNot: assert.NotNil,
			setMocks: func(store *mock_rest.MockFaultStore) {
				store.EXPECT().Exists("name").Return(false)
			},
		},
		"with name id and error during reset": {
			status:         http.StatusInternalServerError,
			name:           "name",
			assertNilOrNot: assert.NotNil,
			setMocks: func(store *mock_rest.MockFaultStore) {
				store.EXPECT().Exists("name").Return(true)
				store.EXPECT().Reset("name").Return(fmt.Errorf("reset failure"))
			},
		},
		"with name id and no error during reset": {
			name:           "name",
			status:         http.StatusAccepted,
			assertNilOrNot: assert.NotNil,
			setMocks: func(store *mock_rest.MockFaultStore) {
				store.EXPECT().Exists("name").Return(true)
				store.EXPECT().Reset("name").Return(nil)
			},
		},
	}

	for name, test := range tt {
		t.Run(name, func(t *testing.T) {
			mockStore := mock_rest.NewMockFaultStore(mockCtrl)
			test.setMocks(mockStore)

			server := newHttpTestServer(t, "/{name}", makeResetFaultConfigHandler(mockStore))
			defer server.Close()

			request, err := http.NewRequest("DELETE", fmt.Sprintf("%s/%s", server.URL, test.name), nil)
			if err != nil {
				assert.FailNow(t, err.Error())
			}
			resp, err := server.Client().Do(request)
			if err != nil {
				assert.FailNow(t, err.Error())
			}

			test.assertNilOrNot(t, resp)
			assert.Equal(t, test.status, resp.StatusCode, "expected status codes to match")
		})
	}
}
