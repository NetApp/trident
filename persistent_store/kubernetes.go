// Copyright 2018 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"fmt"
	"io/ioutil"
	"sync"

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
	mu      sync.Mutex
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
	k.mu.Lock()
	defer k.mu.Unlock()

	backend, err := v1.NewBackend(b.ConstructPersistent())
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
	k.mu.Lock()
	defer k.mu.Unlock()

	backend, err := k.client.TridentV1().Backends().Get(v1.NameFix(backendName), metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	persistent, err := backend.Persistent()
	if err != nil {
		return nil, err
	}

	return persistent, nil
}

func (k *KubernetesClient) UpdateBackend(update *storage.Backend) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	backend, err := k.client.TridentV1().Backends().Get(v1.NameFix(update.Name), metav1.GetOptions{})
	if err != nil {
		return err
	}

	err = backend.Apply(update.ConstructPersistent())
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
	k.mu.Lock()
	defer k.mu.Unlock()

	err := k.client.TridentV1().Backends().Delete(v1.NameFix(b.Name), nil)
	if err != nil {
		return err
	}

	return nil
}

func (k *KubernetesClient) GetBackends() ([]*storage.BackendPersistent, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	results := []*storage.BackendPersistent{}

	list, err := k.client.TridentV1().Backends().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, item := range list.Items {
		persistent, err := item.Persistent()
		if err != nil {
			return nil, err
		}

		results = append(results, persistent)
	}

	return results, nil
}

func (k *KubernetesClient) DeleteBackends() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	list, err := k.client.TridentV1().Backends().List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, item := range list.Items {
		err := k.client.TridentV1().Backends().Delete(item.ObjectMeta.Name, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (k *KubernetesClient) ReplaceBackendAndUpdateVolumes(origBackend, newBackend *storage.Backend) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	var err error

	backend, err := v1.NewBackend(newBackend.ConstructPersistent())
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().Backends().Create(backend)
	if err != nil {
		return err
	}

	vols, err := k.client.TridentV1().Volumes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, v := range vols.Items {
		if v.Backend != origBackend.Name {
			continue
		}

		v.Backend = newBackend.Name

		_, err = k.client.TridentV1().Volumes().Update(v)
		if err != nil {
			return err
		}
	}

	err = k.client.TridentV1().Backends().Delete(v1.NameFix(origBackend.Name), nil)
	if err != nil {
		return err
	}

	return nil
}

func (k *KubernetesClient) AddVolume(vol *storage.Volume) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	volume, err := v1.NewVolume(vol.ConstructExternal())
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
	k.mu.Lock()
	defer k.mu.Unlock()

	volume, err := k.client.TridentV1().Volumes().Get(v1.NameFix(volName), metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	persistent, err := volume.Persistent()
	if err != nil {
		return nil, err
	}

	return persistent, nil
}

func (k *KubernetesClient) UpdateVolume(update *storage.Volume) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	volume, err := k.client.TridentV1().Volumes().Get(v1.NameFix(update.Config.Name), metav1.GetOptions{})
	if err != nil {
		return err
	}

	err = volume.Apply(update.ConstructExternal())
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
	k.mu.Lock()
	defer k.mu.Unlock()

	return k.client.TridentV1().Volumes().Delete(v1.NameFix(vol.Config.Name), nil)
}

func (k *KubernetesClient) DeleteVolumeIgnoreNotFound(vol *storage.Volume) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	err := k.client.TridentV1().Volumes().Delete(v1.NameFix(vol.Config.Name), nil)

	if errors.IsNotFound(err) {
		return nil
	}

	return err
}

func (k *KubernetesClient) GetVolumes() ([]*storage.VolumeExternal, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	results := []*storage.VolumeExternal{}

	list, err := k.client.TridentV1().Volumes().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, item := range list.Items {
		persistent, err := item.Persistent()
		if err != nil {
			return nil, err
		}

		results = append(results, persistent)
	}

	return results, nil
}

func (k *KubernetesClient) DeleteVolumes() error {
	k.mu.Lock()
	defer k.mu.Unlock()

	list, err := k.client.TridentV1().Volumes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, item := range list.Items {
		err := k.client.TridentV1().Volumes().Delete(item.ObjectMeta.Name, nil)
		if err != nil {
			return err
		}
	}

	return nil
}

func (k *KubernetesClient) AddVolumeTransaction(volTxn *VolumeTransaction) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	volume, err := v1.NewVolumeTransaction(string(volTxn.Op), volTxn.Config)
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().VolumeTransactions().Create(volume)

	// Update if already exists
	if errors.IsAlreadyExists(err) {
		existing, err := k.client.TridentV1().VolumeTransactions().Get(volume.ObjectMeta.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		err = existing.Apply(string(volTxn.Op), volTxn.Config)
		if err != nil {
			return err
		}

		_, err = k.client.TridentV1().VolumeTransactions().Update(existing)
		if err != nil {
			return err
		}

		return nil
	}

	return err
}

func (k *KubernetesClient) GetVolumeTransactions() ([]*VolumeTransaction, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	results := []*VolumeTransaction{}

	list, err := k.client.TridentV1().VolumeTransactions().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, item := range list.Items {
		op, config, err := item.Persistent()
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
	k.mu.Lock()
	defer k.mu.Unlock()

	volume, err := k.client.TridentV1().VolumeTransactions().Get(v1.NameFix(volTxn.Config.Name), metav1.GetOptions{})

	if errors.IsNotFound(err) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	op, config, err := volume.Persistent()

	if err != nil {
		return nil, err
	}

	return &VolumeTransaction{
		Op:     VolumeOperation(op),
		Config: config,
	}, nil
}

func (k *KubernetesClient) DeleteVolumeTransaction(volTxn *VolumeTransaction) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	return k.client.TridentV1().VolumeTransactions().Delete(v1.NameFix(volTxn.Config.Name), nil)
}

func (k *KubernetesClient) AddStorageClass(sc *storageclass.StorageClass) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	storageclass, err := v1.NewStorageClass(sc.ConstructPersistent())
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
	k.mu.Lock()
	defer k.mu.Unlock()

	storageclass, err := k.client.TridentV1().StorageClasses().Get(v1.NameFix(scName), metav1.GetOptions{})

	if err != nil {
		return nil, err
	}

	persistent, err := storageclass.Persistent()
	if err != nil {
		return nil, err
	}

	return persistent, nil
}

func (k *KubernetesClient) GetStorageClasses() ([]*storageclass.Persistent, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	results := []*storageclass.Persistent{}

	list, err := k.client.TridentV1().StorageClasses().List(metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, item := range list.Items {
		persistent, err := item.Persistent()
		if err != nil {
			return nil, err
		}

		results = append(results, persistent)
	}

	return results, nil
}

func (k *KubernetesClient) DeleteStorageClass(sc *storageclass.StorageClass) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	return k.client.TridentV1().StorageClasses().Delete(v1.NameFix(sc.GetName()), nil)
}
