// Copyright 2026 NetApp, Inc. All Rights Reserved.

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	mockcore "github.com/netapp/trident/mocks/mock_core"
	mockk8scontrollerhelper "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_controller_helpers/mock_kubernetes_helper"
	"github.com/netapp/trident/object"
	_ "github.com/netapp/trident/object/drivers/fake"
	"github.com/netapp/trident/utils/errors"
)

const fakeObjectStoreBody = `{"objectStoreDriver":"fake","objectStoreName":"store-a","endpoint":"https://localhost"}`

func TestAddObjectStore(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		setupMock      func(*testing.T, *mockcore.MockOrchestrator, *mockk8scontrollerhelper.MockK8SControllerHelperPlugin)
		expectedStatus int
		expectError    bool
	}{
		{
			name: "success",
			body: fakeObjectStoreBody,
			setupMock: func(t *testing.T, mockOrchestrator *mockcore.MockOrchestrator, mockK8s *mockk8scontrollerhelper.MockK8SControllerHelperPlugin) {
				mockOrchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).Return(mockK8s, nil)
				mockK8s.EXPECT().AddObjectStore(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, store object.Store) error {
						require.NotNil(t, store)
						assert.Equal(t, "fake", store.DriverName())
						assert.Equal(t, "store-a", store.Name())
						return nil
					},
				)
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name: "kubernetes helper error",
			body: fakeObjectStoreBody,
			setupMock: func(_ *testing.T, mockOrchestrator *mockcore.MockOrchestrator, mockK8s *mockk8scontrollerhelper.MockK8SControllerHelperPlugin) {
				mockOrchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).Return(mockK8s, nil)
				mockK8s.EXPECT().AddObjectStore(gomock.Any(), gomock.Any()).Return(errors.New("create failed"))
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name: "frontend not found",
			body: fakeObjectStoreBody,
			setupMock: func(_ *testing.T, mockOrchestrator *mockcore.MockOrchestrator, _ *mockk8scontrollerhelper.MockK8SControllerHelperPlugin) {
				mockOrchestrator.EXPECT().GetFrontend(gomock.Any(), controllerhelpers.KubernetesHelper).
					Return(nil, errors.New("frontend not found"))
			},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "missing driver",
			body:           `{"objectStoreName":"store-a"}`,
			setupMock:      func(*testing.T, *mockcore.MockOrchestrator, *mockk8scontrollerhelper.MockK8SControllerHelperPlugin) {},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "unknown driver",
			body:           `{"objectStoreDriver":"unknown","objectStoreName":"store-a"}`,
			setupMock:      func(*testing.T, *mockcore.MockOrchestrator, *mockk8scontrollerhelper.MockK8SControllerHelperPlugin) {},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
		{
			name:           "invalid json",
			body:           `{`,
			setupMock:      func(*testing.T, *mockcore.MockOrchestrator, *mockk8scontrollerhelper.MockK8SControllerHelperPlugin) {},
			expectedStatus: http.StatusBadRequest,
			expectError:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockController := gomock.NewController(t)
			mockOrchestrator := mockcore.NewMockOrchestrator(mockController)
			mockK8s := mockk8scontrollerhelper.NewMockK8SControllerHelperPlugin(mockController)

			originalOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() {
				orchestrator = originalOrchestrator
			}()

			test.setupMock(t, mockOrchestrator, mockK8s)

			request := httptest.NewRequest(http.MethodPost, "/trident/v1/objectstore", strings.NewReader(test.body))
			recorder := httptest.NewRecorder()
			AddObjectStore(recorder, request)

			assert.Equal(t, test.expectedStatus, recorder.Code)

			var response AddObjectStoreResponse
			assert.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, test.expectError, response.Error != "")
		})
	}
}

func TestListObjectStores(t *testing.T) {
	store, err := objectStoreFromJSON(context.Background(), []byte(fakeObjectStoreBody))
	require.NoError(t, err)

	tests := []struct {
		name           string
		stores         []object.Store
		listErr        error
		expectedStatus int
		expectedNames  []string
	}{
		{
			name:           "success",
			stores:         []object.Store{store},
			expectedStatus: http.StatusOK,
			expectedNames:  []string{"store-a"},
		},
		{
			name:           "orchestrator error",
			listErr:        errors.New("list failed"),
			expectedStatus: http.StatusBadRequest,
			expectedNames:  []string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockOrchestrator := mockcore.NewMockOrchestrator(gomock.NewController(t))
			originalOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() {
				orchestrator = originalOrchestrator
			}()

			mockOrchestrator.EXPECT().ListObjectStores(gomock.Any()).Return(test.stores, test.listErr)

			request := httptest.NewRequest(http.MethodGet, "/trident/v1/objectstore", nil)
			recorder := httptest.NewRecorder()
			ListObjectStores(recorder, request)

			assert.Equal(t, test.expectedStatus, recorder.Code)

			var response ListObjectStoresResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, test.expectedNames, response.ObjectStores)
			assert.Equal(t, test.listErr != nil, response.Error != "")
		})
	}
}

func TestGetObjectStore(t *testing.T) {
	store, err := objectStoreFromJSON(context.Background(), []byte(fakeObjectStoreBody))
	require.NoError(t, err)

	tests := []struct {
		name           string
		store          object.Store
		getErr         error
		expectedStatus int
	}{
		{
			name:           "success",
			store:          store,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "not found",
			getErr:         errors.NotFoundError("object store store-a not found"),
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockOrchestrator := mockcore.NewMockOrchestrator(gomock.NewController(t))
			originalOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() {
				orchestrator = originalOrchestrator
			}()

			mockOrchestrator.EXPECT().GetObjectStore(gomock.Any(), "store-a").Return(test.store, test.getErr)

			request := httptest.NewRequest(http.MethodGet, "/trident/v1/objectstore/store-a", nil)
			request = mux.SetURLVars(request, map[string]string{"objectStore": "store-a"})
			recorder := httptest.NewRecorder()
			GetObjectStore(recorder, request)

			assert.Equal(t, test.expectedStatus, recorder.Code)

			var response struct {
				ObjectStore map[string]interface{} `json:"objectStore"`
				Error       string                 `json:"error"`
			}
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			if test.getErr != nil {
				assert.NotEmpty(t, response.Error)
				assert.Nil(t, response.ObjectStore)
			} else {
				assert.Empty(t, response.Error)
				assert.Equal(t, "store-a", response.ObjectStore["objectStoreName"])
				assert.Equal(t, "fake", response.ObjectStore["objectStoreDriver"])
			}
		})
	}
}

func TestDeleteObjectStore(t *testing.T) {
	tests := []struct {
		name           string
		deleteErr      error
		expectedStatus int
	}{
		{
			name:           "success",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "store has buckets",
			deleteErr:      errors.ConflictError("object store store-a has buckets"),
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "not found",
			deleteErr:      errors.NotFoundError("object store store-a not found"),
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockOrchestrator := mockcore.NewMockOrchestrator(gomock.NewController(t))
			originalOrchestrator := orchestrator
			orchestrator = mockOrchestrator
			defer func() {
				orchestrator = originalOrchestrator
			}()

			mockOrchestrator.EXPECT().DeleteObjectStore(gomock.Any(), "store-a").Return(test.deleteErr)

			request := httptest.NewRequest(http.MethodDelete, "/trident/v1/objectstore/store-a", nil)
			request = mux.SetURLVars(request, map[string]string{"objectStore": "store-a"})
			recorder := httptest.NewRecorder()
			DeleteObjectStore(recorder, request)

			assert.Equal(t, test.expectedStatus, recorder.Code)

			var response DeleteResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, test.expectedStatus != http.StatusOK, response.Error != "")
		})
	}
}
