// Copyright 2024 NetApp, Inc. All Rights Reserved.

package rest

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	cliapi "github.com/netapp/trident/cli/api"
	mockClients "github.com/netapp/trident/mocks/mock_operator/mock_clients"
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
