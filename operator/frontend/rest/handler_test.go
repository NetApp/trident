// Copyright 2024 NetApp, Inc. All Rights Reserved.

package rest

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cliapi "github.com/netapp/trident/cli/api"
	"github.com/netapp/trident/logging"
	mockClients "github.com/netapp/trident/mocks/mock_operator/mock_clients"
	"github.com/netapp/trident/operator/clients"
	operatorV1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
)

func getTorcAndTconfList() (*operatorV1.TridentOrchestratorList, *operatorV1.TridentConfiguratorList) {
	torc := operatorV1.TridentOrchestrator{
		Status: operatorV1.TridentOrchestratorStatus{Status: string(operatorV1.AppStatusInstalled)},
	}
	torcList := &operatorV1.TridentOrchestratorList{
		Items: []operatorV1.TridentOrchestrator{torc},
	}

	tconf1 := &operatorV1.TridentConfigurator{}
	tconf2 := &operatorV1.TridentConfigurator{}
	tconfList := &operatorV1.TridentConfiguratorList{
		Items: []*operatorV1.TridentConfigurator{tconf1, tconf2},
	}

	return torcList, tconfList
}

func TestOperatorHandler_GetStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		err      bool
		mockFunc func(*mockClients.MockOperatorCRDClientInterface)
	}{
		{
			name:   "TorcListError",
			status: "",
			err:    true,
			mockFunc: func(client *mockClients.MockOperatorCRDClientInterface) {
				client.EXPECT().GetTorcCRList().Return(nil, fmt.Errorf("failed to get torc list"))
			},
		},
		{
			name:   "MoreThanOneTorc",
			status: string(cliapi.OperatorPhaseUnknown),
			err:    false,
			mockFunc: func(client *mockClients.MockOperatorCRDClientInterface) {
				torcList, _ := getTorcAndTconfList()
				torc2 := operatorV1.TridentOrchestrator{}
				torcList.Items = append(torcList.Items, torc2)
				client.EXPECT().GetTorcCRList().Return(torcList, nil)
			},
		},
		{
			name:   "TconfListError",
			status: "",
			err:    true,
			mockFunc: func(client *mockClients.MockOperatorCRDClientInterface) {
				client.EXPECT().GetTorcCRList().Return(&operatorV1.TridentOrchestratorList{}, nil)
				client.EXPECT().GetTconfCRList().Return(nil, fmt.Errorf("failed to get tconf list"))
			},
		},
		{
			name:   "OperatorInstallingTrident",
			status: string(cliapi.OperatorPhaseProcessing),
			err:    false,
			mockFunc: func(client *mockClients.MockOperatorCRDClientInterface) {
				torcList, tconfList := getTorcAndTconfList()
				torcList.Items[0].Status = operatorV1.TridentOrchestratorStatus{Status: string(operatorV1.AppStatusInstalling)}
				client.EXPECT().GetTorcCRList().Return(torcList, nil)
				client.EXPECT().GetTconfCRList().Return(tconfList, nil)
			},
		},
		{
			name:   "OperatorTridentInstallFailed",
			status: string(cliapi.OperatorPhaseFailed),
			err:    false,
			mockFunc: func(client *mockClients.MockOperatorCRDClientInterface) {
				torcList, tconfList := getTorcAndTconfList()
				torcList.Items[0].Status = operatorV1.TridentOrchestratorStatus{Status: string(operatorV1.AppStatusFailed)}
				client.EXPECT().GetTorcCRList().Return(torcList, nil)
				client.EXPECT().GetTconfCRList().Return(tconfList, nil)
			},
		},
		{
			name:   "OperatorAllConfiguratorsSuccess",
			status: string(cliapi.OperatorPhaseDone),
			err:    false,
			mockFunc: func(client *mockClients.MockOperatorCRDClientInterface) {
				torcList, tconfList := getTorcAndTconfList()
				tconfList.Items[0].Status = operatorV1.TridentConfiguratorStatus{LastOperationStatus: string(operatorV1.Success)}
				tconfList.Items[1].Status = operatorV1.TridentConfiguratorStatus{LastOperationStatus: string(operatorV1.Success)}
				client.EXPECT().GetTorcCRList().Return(torcList, nil)
				client.EXPECT().GetTconfCRList().Return(tconfList, nil)
			},
		},
		{
			name:   "OperatorAtLeastOneConfiguratorProcessing",
			status: string(cliapi.OperatorPhaseProcessing),
			err:    false,
			mockFunc: func(client *mockClients.MockOperatorCRDClientInterface) {
				torcList, tconfList := getTorcAndTconfList()
				tconfList.Items[0].Status = operatorV1.TridentConfiguratorStatus{LastOperationStatus: string(operatorV1.Processing)}
				tconfList.Items[1].Status = operatorV1.TridentConfiguratorStatus{LastOperationStatus: string(operatorV1.Failed)}
				client.EXPECT().GetTorcCRList().Return(torcList, nil)
				client.EXPECT().GetTconfCRList().Return(tconfList, nil)
			},
		},
		{
			name:   "OperatorOneConfiguratorFailedAndOthersSucceeded",
			status: string(cliapi.OperatorPhaseFailed),
			err:    false,
			mockFunc: func(client *mockClients.MockOperatorCRDClientInterface) {
				torcList, tconfList := getTorcAndTconfList()
				tconfList.Items[0].Status = operatorV1.TridentConfiguratorStatus{LastOperationStatus: string(operatorV1.Failed)}
				tconfList.Items[1].Status = operatorV1.TridentConfiguratorStatus{LastOperationStatus: string(operatorV1.Success)}
				client.EXPECT().GetTorcCRList().Return(torcList, nil)
				client.EXPECT().GetTconfCRList().Return(tconfList, nil)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockOperatorClient := mockClients.NewMockOperatorCRDClientInterface(mockCtrl)
			oh := OperatorHandler{oClient: mockOperatorClient}

			test.mockFunc(mockOperatorClient)

			status, err := oh.getStatus()

			if test.err {
				assert.Error(t, err, "Failed to get status.")
			} else {
				assert.Equal(t, test.status, status.Status)
			}
		})
	}
}

func TestMain(m *testing.M) {
	// Disable any standard log output
	logging.InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func TestNewOperatorHandler(t *testing.T) {
	// Create a basic client factory (we'll focus on testing with mocked OperatorCRDClientInterface)
	clientFactory := &clients.Clients{}

	handler := NewOperatorHandler(clientFactory)

	assert.NotNil(t, handler)
	assert.NotNil(t, handler.oClient)
}

func TestOperatorHandler_GetStatus_HTTPEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		setupMock      func(*mockClients.MockOperatorCRDClientInterface)
		expectedStatus int
		expectedPhase  string
	}{
		{
			name: "Successful status retrieval",
			setupMock: func(client *mockClients.MockOperatorCRDClientInterface) {
				torcList := &operatorV1.TridentOrchestratorList{}
				tconfList := &operatorV1.TridentConfiguratorList{}
				client.EXPECT().GetTorcCRList().Return(torcList, nil)
				client.EXPECT().GetTconfCRList().Return(tconfList, nil)
			},
			expectedStatus: http.StatusOK,
			expectedPhase:  string(cliapi.OperatorPhaseDone),
		},
		{
			name: "Error getting TorcCRList",
			setupMock: func(client *mockClients.MockOperatorCRDClientInterface) {
				client.EXPECT().GetTorcCRList().Return(nil, fmt.Errorf("failed to get torc list"))
			},
			expectedStatus: http.StatusInternalServerError,
			expectedPhase:  string(cliapi.OperatorPhaseError),
		},
		{
			name: "Processing state",
			setupMock: func(client *mockClients.MockOperatorCRDClientInterface) {
				torcList, tconfList := getTorcAndTconfList()
				torcList.Items[0].Status = operatorV1.TridentOrchestratorStatus{Status: string(operatorV1.AppStatusInstalling)}
				client.EXPECT().GetTorcCRList().Return(torcList, nil)
				client.EXPECT().GetTconfCRList().Return(tconfList, nil)
			},
			expectedStatus: http.StatusOK,
			expectedPhase:  string(cliapi.OperatorPhaseProcessing),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockOperatorClient := mockClients.NewMockOperatorCRDClientInterface(mockCtrl)
			tt.setupMock(mockOperatorClient)

			handler := &OperatorHandler{oClient: mockOperatorClient}

			req := httptest.NewRequest("GET", "/operator/status", nil)
			w := httptest.NewRecorder()

			handler.GetStatus(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var response cliapi.OperatorStatus
			err := json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedPhase, response.Status)
		})
	}
}

func TestGetStatus_EmptyLists(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockOperatorClient := mockClients.NewMockOperatorCRDClientInterface(mockCtrl)
	mockOperatorClient.EXPECT().GetTorcCRList().Return(&operatorV1.TridentOrchestratorList{}, nil)
	mockOperatorClient.EXPECT().GetTconfCRList().Return(&operatorV1.TridentConfiguratorList{}, nil)

	handler := &OperatorHandler{oClient: mockOperatorClient}

	status, err := handler.getStatus()

	assert.NoError(t, err)
	assert.Equal(t, string(cliapi.OperatorPhaseDone), status.Status)
	assert.NotNil(t, status.TorcStatus)
	assert.NotNil(t, status.TconfStatus)
	assert.Equal(t, 0, len(status.TorcStatus))
	assert.Equal(t, 0, len(status.TconfStatus))
}

func TestGetStatus_ConfiguratorStates(t *testing.T) {
	tests := []struct {
		name           string
		tconfStatuses  []string
		expectedStatus string
	}{
		{
			name:           "All success",
			tconfStatuses:  []string{string(operatorV1.Success), string(operatorV1.Success)},
			expectedStatus: string(cliapi.OperatorPhaseDone),
		},
		{
			name:           "One processing",
			tconfStatuses:  []string{string(operatorV1.Processing), string(operatorV1.Success)},
			expectedStatus: string(cliapi.OperatorPhaseProcessing),
		},
		{
			name:           "One failed, not processing",
			tconfStatuses:  []string{string(operatorV1.Failed), string(operatorV1.Success)},
			expectedStatus: string(cliapi.OperatorPhaseFailed),
		},
		{
			name:           "Empty status (processing)",
			tconfStatuses:  []string{"", string(operatorV1.Success)},
			expectedStatus: string(cliapi.OperatorPhaseProcessing),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockOperatorClient := mockClients.NewMockOperatorCRDClientInterface(mockCtrl)

			torcList := &operatorV1.TridentOrchestratorList{}
			tconfList := &operatorV1.TridentConfiguratorList{}

			for i, status := range tt.tconfStatuses {
				tconf := &operatorV1.TridentConfigurator{}
				tconf.Name = fmt.Sprintf("tconf-%d", i)
				tconf.Status = operatorV1.TridentConfiguratorStatus{LastOperationStatus: status}
				tconfList.Items = append(tconfList.Items, tconf)
			}

			mockOperatorClient.EXPECT().GetTorcCRList().Return(torcList, nil)
			mockOperatorClient.EXPECT().GetTconfCRList().Return(tconfList, nil)

			handler := &OperatorHandler{oClient: mockOperatorClient}

			status, err := handler.getStatus()

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, status.Status)
		})
	}
}

func TestNewHTTPServer(t *testing.T) {
	tests := []struct {
		name         string
		address      string
		port         string
		expectedAddr string
	}{
		{
			name:         "localhost with port",
			address:      "localhost",
			port:         "8002",
			expectedAddr: "localhost:8002",
		},
		{
			name:         "empty address with port",
			address:      "",
			port:         "8002",
			expectedAddr: ":8002",
		},
		{
			name:         "IP address with port",
			address:      "127.0.0.1",
			port:         "9000",
			expectedAddr: "127.0.0.1:9000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientFactory := &clients.Clients{}

			server := NewHTTPServer(tt.address, tt.port, clientFactory)

			assert.NotNil(t, server)
			assert.NotNil(t, server.server)
			assert.Equal(t, tt.expectedAddr, server.server.Addr)
			assert.Equal(t, httpReadTimeout, server.server.ReadTimeout)
			assert.Equal(t, httpWriteTimeout, server.server.WriteTimeout)
			assert.NotNil(t, server.server.Handler)
		})
	}
}

func TestAPIServerHTTP_GetName(t *testing.T) {
	clientFactory := &clients.Clients{}
	server := NewHTTPServer("localhost", "8002", clientFactory)

	name := server.GetName()
	assert.Equal(t, "Operator HTTP REST Server", name)
}

func TestAPIServerHTTP_Version(t *testing.T) {
	clientFactory := &clients.Clients{}
	server := NewHTTPServer("localhost", "8002", clientFactory)

	version := server.Version()
	assert.Equal(t, apiVersion, version)
	assert.Equal(t, "1", version)
}

func TestAPIServerHTTP_Activate(t *testing.T) {
	clientFactory := &clients.Clients{}
	server := NewHTTPServer("localhost", "0", clientFactory) // Use port 0 for random available port

	err := server.Activate()
	assert.NoError(t, err)

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Cleanup
	err = server.Deactivate()
	assert.NoError(t, err)
}

func TestAPIServerHTTP_Deactivate(t *testing.T) {
	tests := []struct {
		name     string
		activate bool
	}{
		{
			name:     "deactivate non-activated server",
			activate: false,
		},
		{
			name:     "deactivate activated server",
			activate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientFactory := &clients.Clients{}
			server := NewHTTPServer("localhost", "0", clientFactory)

			if tt.activate {
				err := server.Activate()
				require.NoError(t, err)
				time.Sleep(100 * time.Millisecond) // Give server time to start
			}

			err := server.Deactivate()
			assert.NoError(t, err)
		})
	}
}

func TestNewRouter(t *testing.T) {
	clientFactory := &clients.Clients{}

	router := NewRouter(clientFactory)

	assert.NotNil(t, router)
	assert.IsType(t, &mux.Router{}, router)

	// Check if the route is registered
	var route *mux.Route
	err := router.Walk(func(r *mux.Route, router *mux.Router, ancestors []*mux.Route) error {
		if name := r.GetName(); name == "readiness" {
			route = r
		}
		return nil
	})

	assert.NoError(t, err)
	assert.NotNil(t, route)

	// Verify route methods
	methods, _ := route.GetMethods()
	assert.Contains(t, methods, "GET")

	// Verify route path
	path, _ := route.GetPathTemplate()
	assert.Equal(t, "/operator/status", path)
}

func TestConstants(t *testing.T) {
	// Test that constants are properly defined
	assert.Equal(t, 90*time.Second, httpReadTimeout)
	assert.Equal(t, 5*time.Second, httpWriteTimeout)
	assert.Equal(t, 90*time.Second, httpTimeout)
	assert.Equal(t, "1", apiVersion)
}

func TestTorcNameInStatus(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockOperatorClient := mockClients.NewMockOperatorCRDClientInterface(mockCtrl)

	// Create a TorcCR with a specific name
	torc := operatorV1.TridentOrchestrator{}
	torc.Name = "trident"
	torc.Status = operatorV1.TridentOrchestratorStatus{
		Status:  string(operatorV1.AppStatusInstalled),
		Message: "Trident installed successfully",
	}

	torcList := &operatorV1.TridentOrchestratorList{
		Items: []operatorV1.TridentOrchestrator{torc},
	}

	tconfList := &operatorV1.TridentConfiguratorList{}

	mockOperatorClient.EXPECT().GetTorcCRList().Return(torcList, nil)
	mockOperatorClient.EXPECT().GetTconfCRList().Return(tconfList, nil)

	handler := &OperatorHandler{oClient: mockOperatorClient}

	status, err := handler.getStatus()

	assert.NoError(t, err)
	assert.Contains(t, status.TorcStatus, "trident")
	assert.Equal(t, string(operatorV1.AppStatusInstalled), status.TorcStatus["trident"].Status)
	assert.Equal(t, "Trident installed successfully", status.TorcStatus["trident"].Message)
}

func TestTconfNamesInStatus(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()

	mockOperatorClient := mockClients.NewMockOperatorCRDClientInterface(mockCtrl)

	// Create multiple TconfCRs
	tconf1 := &operatorV1.TridentConfigurator{}
	tconf1.Name = "backend-ontap"
	tconf1.Status = operatorV1.TridentConfiguratorStatus{
		LastOperationStatus: string(operatorV1.Success),
		Message:             "Backend configured successfully",
	}

	tconf2 := &operatorV1.TridentConfigurator{}
	tconf2.Name = "backend-solidfire"
	tconf2.Status = operatorV1.TridentConfiguratorStatus{
		LastOperationStatus: string(operatorV1.Failed),
		Message:             "Backend configuration failed",
	}

	torcList := &operatorV1.TridentOrchestratorList{}
	tconfList := &operatorV1.TridentConfiguratorList{
		Items: []*operatorV1.TridentConfigurator{tconf1, tconf2},
	}

	mockOperatorClient.EXPECT().GetTorcCRList().Return(torcList, nil)
	mockOperatorClient.EXPECT().GetTconfCRList().Return(tconfList, nil)

	handler := &OperatorHandler{oClient: mockOperatorClient}

	status, err := handler.getStatus()

	assert.NoError(t, err)
	assert.Contains(t, status.TconfStatus, "backend-ontap")
	assert.Contains(t, status.TconfStatus, "backend-solidfire")
	assert.Equal(t, string(operatorV1.Success), status.TconfStatus["backend-ontap"].Status)
	assert.Equal(t, string(operatorV1.Failed), status.TconfStatus["backend-solidfire"].Status)
	assert.Equal(t, "Backend configured successfully", status.TconfStatus["backend-ontap"].Message)
	assert.Equal(t, "Backend configuration failed", status.TconfStatus["backend-solidfire"].Message)
}
