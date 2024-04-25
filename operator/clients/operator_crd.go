// Copyright 2024 NetApp, Inc. All Rights Reserved.

package clients

import (
	"fmt"
	"reflect"

	operatorV1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	operatorClient "github.com/netapp/trident/operator/crd/client/clientset/versioned"
)

type OperatorCRDClient struct {
	client operatorClient.Interface
}

func NewOperatorCRDClient(client operatorClient.Interface) OperatorCRDClientInterface {
	return &OperatorCRDClient{client: client}
}

func (oc *OperatorCRDClient) GetTconfCR(name string) (*operatorV1.TridentConfigurator, error) {
	return oc.client.TridentV1().TridentConfigurators().Get(ctx, name, getOpts)
}

func (oc *OperatorCRDClient) GetTconfCRList() (*operatorV1.TridentConfiguratorList, error) {
	return oc.client.TridentV1().TridentConfigurators().List(ctx, listOpts)
}

func (oc *OperatorCRDClient) GetTorcCRList() (*operatorV1.TridentOrchestratorList, error) {
	return oc.client.TridentV1().TridentOrchestrators().List(ctx, listOpts)
}

func (oc *OperatorCRDClient) GetControllingTorcCR() (*operatorV1.TridentOrchestrator, error) {
	torcList, err := oc.client.TridentV1().TridentOrchestrators().List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	for _, torc := range torcList.Items {
		if torc.Status.Status == string(operatorV1.AppStatusInstalled) {
			return &torc, nil
		}
	}

	return nil, fmt.Errorf("trident is not installed yet")
}

func (oc *OperatorCRDClient) UpdateTridentConfiguratorStatus(
	tconfCR *operatorV1.TridentConfigurator, newStatus operatorV1.TridentConfiguratorStatus,
) (*operatorV1.TridentConfigurator, bool, error) {
	if reflect.DeepEqual(tconfCR.Status, newStatus) {
		// Nothing to update
		return tconfCR, false, nil
	}

	tconfCRCopy := tconfCR.DeepCopy()
	tconfCRCopy.Status = newStatus

	newTconfCR, err := oc.client.TridentV1().TridentConfigurators().UpdateStatus(ctx, tconfCRCopy, updateOpts)
	if err != nil {
		return tconfCR, true, err
	} else {
		newTconfCR.APIVersion = tconfCR.APIVersion
		newTconfCR.Kind = tconfCR.Kind
	}

	return newTconfCR, true, nil
}
