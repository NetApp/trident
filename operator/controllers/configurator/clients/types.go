// Copyright 2024 NetApp, Inc. All Rights Reserved.

package clients

import (
	"context"
	"time"

	v1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	operatorV1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
)

//go:generate mockgen -destination=../../../../mocks/mock_operator/mock_controllers/mock_configurator/mock_clients/mock_api.go github.com/netapp/trident/operator/controllers/configurator/clients ConfiguratorClientInterface,ExtendedK8sClientInterface

const DefaultK8sClientTimeout = 30

var (
	k8sTimeout time.Duration

	ctx = context.TODO()

	deleteOpts = metav1.DeleteOptions{}
	getOpts    = metav1.GetOptions{}
	listOpts   = metav1.ListOptions{}
	patchOpts  = metav1.PatchOptions{}
	updateOpts = metav1.UpdateOptions{}
)

type ObjectType string

const (
	OCRD           ObjectType = "CRD"
	OBackend       ObjectType = "Backend"
	OStorageClass  ObjectType = "StorageClass"
	OSnapshotClass ObjectType = "SnapshotClass"
)

type ConfiguratorClientInterface interface {
	CreateOrPatchObject(objType ObjectType, objName, objNamespace, objYAML string) error
	UpdateTridentConfiguratorStatus(
		tconfCR *operatorV1.TridentConfigurator, newStatus operatorV1.TridentConfiguratorStatus,
	) (*operatorV1.TridentConfigurator, bool, error)
	GetControllingTorcCR() (*operatorV1.TridentOrchestrator, error)
	GetTconfCR(name string) (*operatorV1.TridentConfigurator, error)
	GetANFSecrets(secretName string) (string, string, error)
	DeleteObject(objType ObjectType, objName, objNamespace string) error
	ListObjects(objType ObjectType, objNamespace string) (interface{}, error)
}

type ExtendedK8sClientInterface interface {
	k8sclient.KubernetesClient

	CheckStorageClassExists(name string) (bool, error)
	GetStorageClass(name string) (*v1.StorageClass, error)
	PatchStorageClass(name string, patchBytes []byte, patchType types.PatchType) error
	DeleteStorageClass(name string) error
}
