// Copyright 2024 NetApp, Inc. All Rights Reserved.

package clients

import (
	"context"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	operatorV1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	tridentV1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
)

//go:generate mockgen -destination=../../mocks/mock_operator/mock_clients/mock_api.go github.com/netapp/trident/operator/clients OperatorCRDClientInterface,TridentCRDClientInterface,SnapshotCRDClientInterface

var (
	ctx = context.TODO()

	deleteOpts = metav1.DeleteOptions{}
	getOpts    = metav1.GetOptions{}
	listOpts   = metav1.ListOptions{}
	patchOpts  = metav1.PatchOptions{}
	updateOpts = metav1.UpdateOptions{}
)

type OperatorCRDClientInterface interface {
	GetTconfCR(name string) (*operatorV1.TridentConfigurator, error)
	GetTconfCRList() (*operatorV1.TridentConfiguratorList, error)
	GetTorcCRList() (*operatorV1.TridentOrchestratorList, error)
	GetControllingTorcCR() (*operatorV1.TridentOrchestrator, error)
	UpdateTridentConfiguratorStatus(
		tconfCR *operatorV1.TridentConfigurator, newStatus operatorV1.TridentConfiguratorStatus,
	) (*operatorV1.TridentConfigurator, bool, error)
	UpdateTridentConfigurator(tconfCR *operatorV1.TridentConfigurator) error
}

type TridentCRDClientInterface interface {
	CheckTridentBackendConfigExists(name, namespace string) (bool, error)
	GetTridentBackendConfig(name, namespace string) (*tridentV1.TridentBackendConfig, error)
	ListTridentBackend(namespace string) (*tridentV1.TridentBackendList, error)
	ListTridentBackendsByLabel(namespace, labelKey, labelValue string) ([]*tridentV1.TridentBackendConfig, error)
	PatchTridentBackendConfig(name, namespace string, patchBytes []byte, patchType types.PatchType) error
	DeleteTridentBackendConfig(name, namespace string) error
}

type SnapshotCRDClientInterface interface {
	CheckVolumeSnapshotClassExists(name string) (bool, error)
	GetVolumeSnapshotClass(name string) (*snapshotv1.VolumeSnapshotClass, error)
	PatchVolumeSnapshotClass(name string, patchBytes []byte, patchType types.PatchType) error
	DeleteVolumeSnapshotClass(name string) error
}
