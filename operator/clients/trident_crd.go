// Copyright 2024 NetApp, Inc. All Rights Reserved.

package clients

import (
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	tridentV1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentClient "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
)

type TridentCRDClient struct {
	client tridentClient.Interface
}

func NewTridentCRDClient(client tridentClient.Interface) TridentCRDClientInterface {
	return &TridentCRDClient{client: client}
}

func (tc *TridentCRDClient) CheckTridentBackendConfigExists(name, namespace string) (bool, error) {
	if _, err := tc.GetTridentBackendConfig(name, namespace); err != nil {
		if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (tc *TridentCRDClient) GetTridentBackendConfig(name, namespace string) (*tridentV1.TridentBackendConfig, error) {
	return tc.client.TridentV1().TridentBackendConfigs(namespace).Get(ctx, name, getOpts)
}

func (tc *TridentCRDClient) PatchTridentBackendConfig(name, namespace string, patchBytes []byte, patchType types.PatchType) error {
	_, err := tc.client.TridentV1().TridentBackendConfigs(namespace).Patch(ctx, name, patchType, patchBytes, patchOpts)
	return err
}

func (tc *TridentCRDClient) DeleteTridentBackendConfig(name, namespace string) error {
	return tc.client.TridentV1().TridentBackendConfigs(namespace).Delete(ctx, name, deleteOpts)
}

func (tc *TridentCRDClient) ListTridentBackend(namespace string) (*tridentV1.TridentBackendList, error) {
	return tc.client.TridentV1().TridentBackends(namespace).List(ctx, listOpts)
}
