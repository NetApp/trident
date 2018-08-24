// Copyright 2018 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"fmt"
	"io/ioutil"

	cli_k8s_client "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/persistent_store/kubernetes/apis/netapp/v1"
	"github.com/netapp/trident/persistent_store/kubernetes/client/clientset/versioned"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage_class"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Compile time check to ensure KubernetesClient implements Client
var _ Client = &KubernetesClient{}

// KubernetesClient stores persistent state in CRD objects in Kubernetes
type KubernetesClient struct {
	client  versioned.Interface
	version *PersistentStateVersion
}

func NewKubernetesClient(apiServerIP, kubeConfigPath string) (*KubernetesClient, error) {
	kubeConfig, err := clientcmd.BuildConfigFromFlags(apiServerIP, kubeConfigPath)
	if err != nil {
		return nil, err
	}

	// Create the CLI-based Kubernetes client
	client, err := cli_k8s_client.NewKubectlClient("")
	if err != nil {
		return nil, fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	// when running in binary mode, we use the current namespace as determined by the CLI client
	return newKubernetesKubernetesClient(kubeConfig, client.Namespace())
}

func NewKubernetesClientInCluster() (*KubernetesClient, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	// when running in a pod, we use the Trident pod's namespace
	bytes, err := ioutil.ReadFile(config.TridentNamespaceFile)
	if err != nil {
		log.WithFields(log.Fields{
			"error":         err,
			"namespaceFile": config.TridentNamespaceFile,
		}).Fatal("Kubernetes persistence client failed to obtain Trident's namespace!")
	}
	tridentNamespace := string(bytes)

	return newKubernetesKubernetesClient(kubeConfig, tridentNamespace)
}

func newKubernetesKubernetesClient(kubeConfig *rest.Config, tridentNamespace string) (*KubernetesClient, error) {
	client, err := versioned.NewForConfig(kubeConfig)

	return &KubernetesClient{
		client: client,
		version: &PersistentStateVersion{
			"kubernetes", config.OrchestratorAPIVersion,
		},
	}, err
}

func (k *KubernetesClient) GetVersion() (*PersistentStateVersion, error) {
	// TODO
	return k.version, nil
}

func (k *KubernetesClient) SetVersion(version *PersistentStateVersion) error {
	// TODO
	return nil
}

func (k *KubernetesClient) GetConfig() *ClientConfig {
	return &ClientConfig{}
}

func (k *KubernetesClient) GetType() StoreType {
	return KubernetesStore
}

func (k *KubernetesClient) Stop() error {
	return nil
}

func (k *KubernetesClient) AddBackend(b *storage.Backend) error {
	backend, err := v1.BackendFromBackendPersistent(b.ConstructPersistent())
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().Backends().Create(backend)
	if err != nil {
		return err
	}

	return nil
}

func (k *KubernetesClient) GetBackend(backendName string) (*storage.BackendPersistent, error) {
	backend, err := k.client.TridentV1().Backends().Get(backendName, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	persistent, err := v1.BackendPersistentFromBackend(backend)
	if err != nil {
		return nil, err
	}

	return persistent, nil
}

func (k *KubernetesClient) UpdateBackend(b *storage.Backend) error {
	backend, err := v1.BackendFromBackendPersistent(b.ConstructPersistent())
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().Backends().Update(backend)
	if err != nil {
		return err
	}

	return nil
}

func (k *KubernetesClient) DeleteBackend(b *storage.Backend) error {
	err := k.client.TridentV1().Backends().Delete(b.Name, nil)
	if err != nil {
		return err
	}

	return nil
}

func (k *KubernetesClient) GetBackends() ([]*storage.BackendPersistent, error) {
	results := []*storage.BackendPersistent{}

	list, err := k.client.TridentV1().Backends().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, item := range list.Items {
		persistent, err := v1.BackendPersistentFromBackend(item)
		if err != nil {
			return nil, err
		}

		results = append(results, persistent)
	}

	return results, nil
}

func (k *KubernetesClient) DeleteBackends() error {
	return k.client.TridentV1().Backends().DeleteCollection(nil, metav1.ListOptions{})
}

func (k *KubernetesClient) ReplaceBackendAndUpdateVolumes(origBackend, newBackend *storage.Backend) error {
	var err error

	err = k.AddBackend(newBackend)
	if err != nil {
		return err
	}

	vols, err := k.GetVolumes()
	if err != nil {
		return err
	}

	for _, v := range vols {
		v.Backend = newBackend.Name

		volume, err := v1.VolumeFromVolumeExternal(v)
		if err != nil {
			return err
		}

		_, err = k.client.TridentV1().Volumes().Update(volume)
		if err != nil {
			return err
		}
	}

	err = k.DeleteBackend(origBackend)
	if err != nil {
		return err
	}

	return nil
}

func (k *KubernetesClient) AddVolume(vol *storage.Volume) error {
	volume, err := v1.VolumeFromVolumeExternal(vol.ConstructExternal())
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().Volumes().Create(volume)
	if err != nil {
		return err
	}

	return nil
}

func (k *KubernetesClient) GetVolume(volName string) (*storage.VolumeExternal, error) {
	volume, err := k.client.TridentV1().Volumes().Get(volName, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	persistent, err := v1.VolumeExternalFromVolume(volume)
	if err != nil {
		return nil, err
	}

	return persistent, nil
}

func (k *KubernetesClient) UpdateVolume(vol *storage.Volume) error {
	volume, err := v1.VolumeFromVolumeExternal(vol.ConstructExternal())
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().Volumes().Update(volume)
	if err != nil {
		return err
	}

	return nil
}

func (k *KubernetesClient) DeleteVolume(vol *storage.Volume) error {
	return k.client.TridentV1().Volumes().Delete(vol.Config.Name, nil)
}

func (k *KubernetesClient) DeleteVolumeIgnoreNotFound(vol *storage.Volume) error {
	err := k.client.TridentV1().Volumes().Delete(vol.Config.Name, nil)

	if errors.IsNotFound(err) {
		return nil
	}

	return err
}

func (k *KubernetesClient) GetVolumes() ([]*storage.VolumeExternal, error) {
	results := []*storage.VolumeExternal{}

	list, err := k.client.TridentV1().Volumes().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, item := range list.Items {
		persistent, err := v1.VolumeExternalFromVolume(item)
		if err != nil {
			return nil, err
		}

		results = append(results, persistent)
	}

	return results, nil
}

func (k *KubernetesClient) DeleteVolumes() error {
	return k.client.TridentV1().Volumes().DeleteCollection(nil, metav1.ListOptions{})
}

func (k *KubernetesClient) AddVolumeTransaction(volTxn *VolumeTransaction) error {
	volume, err := v1.VolumeTransactionFromPersistentVolumeTransaction(string(volTxn.Op), volTxn.Config)
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().VolumeTransactions().Create(volume)
	if err != nil {
		return err
	}

	return nil
}

func (k *KubernetesClient) GetVolumeTransactions() ([]*VolumeTransaction, error) {
	results := []*VolumeTransaction{}

	list, err := k.client.TridentV1().VolumeTransactions().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, item := range list.Items {
		op, config, err := v1.PersistentVolumeTransactionFromVolumeTransaction(item)
		if err != nil {
			return nil, err
		}

		results = append(results, &VolumeTransaction{
			Op:     VolumeOperation(op),
			Config: config,
		})
	}

	return results, nil
}

func (k *KubernetesClient) GetExistingVolumeTransaction(volTxn *VolumeTransaction) (*VolumeTransaction, error) {
	volume, err := k.client.TridentV1().VolumeTransactions().Get(volTxn.Config.Name, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	op, config, err := v1.PersistentVolumeTransactionFromVolumeTransaction(volume)
	if err != nil {
		return nil, err
	}

	return &VolumeTransaction{
		Op:     VolumeOperation(op),
		Config: config,
	}, nil
}

func (k *KubernetesClient) DeleteVolumeTransaction(volTxn *VolumeTransaction) error {
	return k.client.TridentV1().VolumeTransactions().Delete(volTxn.Config.Name, nil)
}

func (k *KubernetesClient) AddStorageClass(sc *storageclass.StorageClass) error {
	storageclass, err := v1.StorageClassFromPersistentStorageClass(sc.ConstructPersistent())
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().StorageClasses().Create(storageclass)
	if err != nil {
		return err
	}

	return nil
}

func (k *KubernetesClient) GetStorageClass(scName string) (*storageclass.Persistent, error) {
	storageclass, err := k.client.TridentV1().StorageClasses().Get(scName, metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	persistent, err := v1.PersistentStorageClassFromStorageClass(storageclass)
	if err != nil {
		return nil, err
	}

	return persistent, nil
}

func (k *KubernetesClient) GetStorageClasses() ([]*storageclass.Persistent, error) {
	results := []*storageclass.Persistent{}

	list, err := k.client.TridentV1().StorageClasses().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, item := range list.Items {
		persistent, err := v1.PersistentStorageClassFromStorageClass(item)
		if err != nil {
			return nil, err
		}

		results = append(results, persistent)
	}

	return results, nil
}

func (k *KubernetesClient) DeleteStorageClass(sc *storageclass.StorageClass) error {
	return k.client.TridentV1().StorageClasses().Delete(sc.GetName(), nil)
}
