// Copyright 2024 NetApp, Inc. All Rights Reserved.

package clients

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"

	operatorV1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	fakeOperatorClient "github.com/netapp/trident/operator/crd/client/clientset/versioned/fake"
)

func getMockOperatorClient() (OperatorCRDClientInterface, *fakeOperatorClient.Clientset) {
	cs := fakeOperatorClient.NewSimpleClientset()
	return &OperatorCRDClient{client: cs}, cs
}

func TestNewOperatorCRDClient_Success(t *testing.T) {
	client := fakeOperatorClient.NewSimpleClientset()
	operatorClient := NewOperatorCRDClient(client)

	assert.NotNil(t, operatorClient, "expected OperatorCRDClient, got nil")
}

func TestOperatorCRDClient_GetTconfCR_Success(t *testing.T) {
	client, fakeClient := getMockOperatorClient()

	fakeClient.PrependReactor("get", "tridentconfigurators", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &operatorV1.TridentConfigurator{}, nil
	})

	_, err := client.GetTconfCR("test")

	assert.NoError(t, err, "failed to get TridentConfigurator CR")
}

func TestOperatorCRDClient_GetTconfCR_NotFound(t *testing.T) {
	client, fakeClient := getMockOperatorClient()

	fakeClient.PrependReactor("get", "tridentconfigurators", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "trident.netapp.io", Resource: "tridentconfigurators"}, "test")
	})

	_, err := client.GetTconfCR("test")

	assert.Error(t, err, "no error returned for missing TridentConfigurator CR")
}

func TestOperatorCRDClient_GetTconfCRList_Success(t *testing.T) {
	client, fakeClient := getMockOperatorClient()

	fakeClient.PrependReactor("list", "tridentconfigurators", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &operatorV1.TridentConfiguratorList{}, nil
	})

	_, err := client.GetTconfCRList()

	assert.NoError(t, err, "failed to get TridentConfigurator CR list")
}

func TestOperatorCRDClient_GetTconfCRList_Error(t *testing.T) {
	client, fakeClient := getMockOperatorClient()

	fakeClient.PrependReactor("list", "tridentconfigurators", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("random error")
	})

	_, err := client.GetTconfCRList()

	assert.Error(t, err, "no error returned for failed TridentConfigurator CR list call")
}

func TestOperatorCRDClient_GetTorcCRList_Success(t *testing.T) {
	client, fakeClient := getMockOperatorClient()

	fakeClient.PrependReactor("list", "tridentorchestrators", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &operatorV1.TridentOrchestratorList{}, nil
	})

	_, err := client.GetTorcCRList()

	assert.NoError(t, err, "failed to get TridentOrchestrator CR list")
}

func TestOperatorCRDClient_GetTorcCRList_Error(t *testing.T) {
	client, fakeClient := getMockOperatorClient()

	fakeClient.PrependReactor("list", "tridentorchestrators", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("random error")
	})

	_, err := client.GetTorcCRList()

	assert.Error(t, err, "no error returned for failed TridentOrchestrator CR list call")
}

func TestOperatorCRDClient_GetControllingTorcCR_Success(t *testing.T) {
	client, fakeClient := getMockOperatorClient()

	fakeClient.PrependReactor("list", "tridentorchestrators", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &operatorV1.TridentOrchestratorList{
			Items: []operatorV1.TridentOrchestrator{
				{Status: operatorV1.TridentOrchestratorStatus{Status: string(operatorV1.AppStatusInstalled)}},
			},
		}, nil
	})

	_, err := client.GetControllingTorcCR()

	assert.NoError(t, err, "failed to get controlling TridentOrchestrator CR")
}

func TestOperatorCRDClient_GetControllingTorcCR_NotInstalled(t *testing.T) {
	client, fakeClient := getMockOperatorClient()

	fakeClient.PrependReactor("list", "tridentorchestrators", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, &operatorV1.TridentOrchestratorList{}, nil
	})

	_, err := client.GetControllingTorcCR()

	assert.Error(t, err, "no error returned for missing controlling TridentOrchestrator CR")
}

func TestOperatorCRDClient_UpdateTridentConfiguratorStatus_NoChange(t *testing.T) {
	client, fakeClient := getMockOperatorClient()
	tconfCR := &operatorV1.TridentConfigurator{Status: operatorV1.TridentConfiguratorStatus{Phase: "test"}}
	newStatus := operatorV1.TridentConfiguratorStatus{Phase: "test"}

	fakeClient.PrependReactor("update", "tridentconfigurators", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, tconfCR, nil
	})

	_, updated, err := client.UpdateTridentConfiguratorStatus(tconfCR, newStatus)

	assert.NoError(t, err, "failed to update TridentConfigurator CR status")
	assert.False(t, updated, "status of TridentConfigurator CR was updated")
}

func TestOperatorCRDClient_UpdateTridentConfiguratorStatus_Success(t *testing.T) {
	client, fakeClient := getMockOperatorClient()
	tconfCR := &operatorV1.TridentConfigurator{Status: operatorV1.TridentConfiguratorStatus{Phase: "test"}}
	newStatus := operatorV1.TridentConfiguratorStatus{Phase: "updated"}

	fakeClient.PrependReactor("update", "tridentconfigurators", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, tconfCR, nil
	})

	_, updated, err := client.UpdateTridentConfiguratorStatus(tconfCR, newStatus)

	assert.NoError(t, err, "failed to update TridentConfigurator CR status")
	assert.True(t, updated, "status of TridentConfigurator CR was not updated")
}

func TestOperatorCRDClient_UpdateTridentConfiguratorStatus_Error(t *testing.T) {
	client, fakeClient := getMockOperatorClient()
	tconfCR := &operatorV1.TridentConfigurator{Status: operatorV1.TridentConfiguratorStatus{Phase: "test"}}
	newStatus := operatorV1.TridentConfiguratorStatus{Phase: "updated"}

	fakeClient.PrependReactor("update", "tridentconfigurators", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, nil, fmt.Errorf("random error")
	})

	_, updated, err := client.UpdateTridentConfiguratorStatus(tconfCR, newStatus)

	assert.Error(t, err, "no error returned for failed TridentConfigurator CR status update")
	assert.True(t, updated, "status of TridentConfigurator CR was not updated")
}
