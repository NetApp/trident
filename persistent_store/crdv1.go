// Copyright 2019 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"fmt"
	"io/ioutil"
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	cliclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils"
)

// Compile time checks to ensure CRDClientV1 implements Client & CRDClient
var _ Client = &CRDClientV1{}
var _ CRDClient = &CRDClientV1{}

var (
	listOpts = metav1.ListOptions{}
	getOpts  = metav1.GetOptions{}
)

// CRDClientV1 stores persistent state in CRD objects in Kubernetes
type CRDClientV1 struct {
	client    versioned.Interface
	version   *config.PersistentStateVersion
	namespace string
}

func NewCRDClientV1(apiServerIP, kubeConfigPath string) (*CRDClientV1, error) {

	log.WithFields(log.Fields{
		"apiServerIP":    apiServerIP,
		"kubeConfigPath": kubeConfigPath,
	}).Debug("Creating CRDv1 persistent store client.")

	kubeConfig, err := clientcmd.BuildConfigFromFlags(apiServerIP, kubeConfigPath)
	if err != nil {
		return nil, err
	}

	// Create the CLI-based Kubernetes client
	client, err := cliclient.NewKubectlClient("")
	if err != nil {
		return nil, fmt.Errorf("could not initialize CRD client; %v", err)
	}

	// When running in binary mode, we use the current namespace as determined by the CLI client
	return newCRDClientV1(kubeConfig, client.Namespace())
}

func NewCRDClientV1InCluster() (*CRDClientV1, error) {

	log.Debug("Creating CRDv1 persistent store client.")

	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	// When running in a pod, we use the Trident pod's namespace
	namespaceBytes, err := ioutil.ReadFile(config.TridentNamespaceFile)
	if err != nil {
		log.WithFields(log.Fields{
			"error":         err,
			"namespaceFile": config.TridentNamespaceFile,
		}).Error("CRDv1 persistence client failed to obtain Trident's namespace!")
		return nil, err
	}

	return newCRDClientV1(kubeConfig, string(namespaceBytes))
}

func newCRDClientV1(kubeConfig *rest.Config, tridentNamespace string) (*CRDClientV1, error) {

	client, err := versioned.NewForConfig(kubeConfig)

	log.WithFields(log.Fields{
		"tridentNamespace": tridentNamespace,
	}).Debug("Created CRDv1 persistence client.")

	return &CRDClientV1{
		client: client,
		version: &config.PersistentStateVersion{
			PersistentStoreVersion: string(CRDV1Store),
			OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		},
		namespace: tridentNamespace,
	}, err
}

func (k *CRDClientV1) GetVersion() (*config.PersistentStateVersion, error) {

	versionList, err := k.client.TridentV1().TridentVersions(k.namespace).List(listOpts)
	if err != nil {
		if strings.Contains(err.Error(), "the server could not find the requested resource") {
			return nil, NewPersistentStoreError(KeyNotFoundErr, v1.PersistentStateVersionName)
		}
		return nil, err
	} else if versionList == nil || versionList.Items == nil || len(versionList.Items) == 0 {
		return nil, NewPersistentStoreError(KeyNotFoundErr, v1.PersistentStateVersionName)
	}

	persistentVersion, err := versionList.Items[0].Persistent()
	if err != nil {
		return nil, err
	}

	return persistentVersion, nil
}

func (k *CRDClientV1) SetVersion(version *config.PersistentStateVersion) error {

	versionList, err := k.client.TridentV1().TridentVersions(k.namespace).List(listOpts)
	if err != nil {
		return err
	}

	// If version doesn't exist, create it
	if versionList == nil || versionList.Items == nil || len(versionList.Items) == 0 {

		newVersion, err := v1.NewTridentVersion(version)
		if err != nil {
			return err
		}

		_, err = k.client.TridentV1().TridentVersions(k.namespace).Create(newVersion)
		if err != nil {
			return err
		}

		log.WithFields(log.Fields{
			"PersistentStoreVersion": newVersion.PersistentStoreVersion,
			"OrchestratorAPIVersion": newVersion.OrchestratorAPIVersion,
		}).Debug("Created persistent state version.")

		return nil
	}

	// Version exists, so update it
	existingVersion := versionList.Items[0]

	if err = existingVersion.Apply(version); err != nil {
		return err
	}

	_, err = k.client.TridentV1().TridentVersions(k.namespace).Update(existingVersion)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"PersistentStoreVersion": existingVersion.PersistentStoreVersion,
		"OrchestratorAPIVersion": existingVersion.OrchestratorAPIVersion,
	}).Debug("Updated persistent state version.")

	return nil
}

func (k *CRDClientV1) GetConfig() *ClientConfig {
	return &ClientConfig{}
}

func (k *CRDClientV1) GetType() StoreType {
	return CRDV1Store
}

func (k *CRDClientV1) Stop() error {
	return nil
}

func (k *CRDClientV1) AddBackend(backend *storage.Backend) error {
	log.WithFields(log.Fields{
		"backend":      backend,
		"backend.Name": backend.Name,
	}).Debug("AddBackend.")

	persistentBackend, err := v1.NewTridentBackend(backend.ConstructPersistent())
	if err != nil {
		return err
	}

	return k.addBackendCRD(persistentBackend)
}

func (k *CRDClientV1) addBackendCRD(backend *v1.TridentBackend) error {
	log.WithFields(log.Fields{
		"backend.Name": backend.Name,
	}).Debug("addBackendCRD.")

	_, err := k.client.TridentV1().TridentBackends(k.namespace).Create(backend)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) AddBackendPersistent(backend *storage.BackendPersistent) error {

	persistentBackend, err := v1.NewTridentBackend(backend)
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().TridentBackends(k.namespace).Create(persistentBackend)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) HasBackends() (bool, error) {

	listOneOpts := metav1.ListOptions{Limit: 1}
	backendList, err := k.client.TridentV1().TridentBackends(k.namespace).List(listOneOpts)
	if err != nil {
		return false, err
	}

	return len(backendList.Items) > 0, nil
}

func (k *CRDClientV1) GetBackend(backendName string) (*storage.BackendPersistent, error) {

	log.WithFields(log.Fields{
		"backendName": backendName,
	}).Debug("GetBackend.")

	list, err := k.client.TridentV1().TridentBackends(k.namespace).List(listOpts)
	if err != nil {
		return nil, err
	} else if list == nil || list.Items == nil || len(list.Items) == 0 {
		return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
	}

	for _, backend := range list.Items {
		log.WithFields(log.Fields{
			"backend.Name":        backend.Name,
			"backend.BackendName": backend.BackendName,
			"backendName":         backendName,
		}).Debug("Checking.")
		if backend.BackendName == backendName {
			return backend.Persistent()
		}
	}

	return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
}

func (k *CRDClientV1) getBackendCRD(backendName string) (*v1.TridentBackend, error) {

	log.WithFields(log.Fields{
		"backendName": backendName,
	}).Debug("getBackend.")

	list, err := k.client.TridentV1().TridentBackends(k.namespace).List(listOpts)
	if err != nil {
		return nil, err
	} else if list == nil || list.Items == nil || len(list.Items) == 0 {
		return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
	}

	for _, backend := range list.Items {
		log.WithFields(log.Fields{
			"backend.Name":        backend.Name,
			"backend.BackendName": backend.BackendName,
			"backendName":         backendName,
		}).Debug("Checking.")
		if backend.BackendName == backendName {
			return backend, nil
		}
	}

	return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
}

func (k *CRDClientV1) UpdateBackend(update *storage.Backend) error {

	log.WithFields(log.Fields{
		"update":      update,
		"update.Name": update.Name,
	}).Debug("UpdateBackend.")

	backend, err := k.getBackendCRD(update.Name)
	if err != nil {
		return err
	}

	if err = backend.Apply(update.ConstructPersistent()); err != nil {
		return err
	}

	_, err = k.client.TridentV1().TridentBackends(k.namespace).Update(backend)
	if err != nil {
		return err
	}

	return nil
}

// UpdateBackendPersistent updates a backend's persistent state
func (k *CRDClientV1) UpdateBackendPersistent(update *storage.BackendPersistent) error {
	log.WithFields(log.Fields{
		"update":      update,
		"update.Name": update.Name,
	}).Debug("UpdateBackendPersistent.")

	backend, err := k.getBackendCRD(update.Name)
	if err != nil {
		return err
	}

	if err = backend.Apply(update); err != nil {
		return err
	}

	_, err = k.client.TridentV1().TridentBackends(k.namespace).Update(backend)
	if err != nil {
		return err
	}
	return nil
}

func (k *CRDClientV1) DeleteBackend(b *storage.Backend) error {
	log.WithFields(log.Fields{
		"b.Name":        b.Name,
		"b.BackendUUID": b.BackendUUID,
	}).Debug("DeleteBackend.")

	backend, err := k.getBackendCRD(b.Name)
	if err != nil {
		return err
	}

	return k.client.TridentV1().TridentBackends(k.namespace).Delete(backend.ObjectMeta.Name, k.deleteOpts())
}

func (k *CRDClientV1) GetBackends() ([]*storage.BackendPersistent, error) {

	backendList, err := k.client.TridentV1().TridentBackends(k.namespace).List(listOpts)
	if err != nil {
		return nil, err
	}

	results := make([]*storage.BackendPersistent, 0)

	for _, item := range backendList.Items {
		persistentBackend, err := item.Persistent()
		if err != nil {
			return nil, err
		}

		results = append(results, persistentBackend)
	}

	return results, nil
}

func (k *CRDClientV1) DeleteBackends() error {

	backendList, err := k.client.TridentV1().TridentBackends(k.namespace).List(listOpts)
	if err != nil {
		return err
	}

	for _, item := range backendList.Items {
		err = k.client.TridentV1().TridentBackends(k.namespace).Delete(item.ObjectMeta.Name, k.deleteOpts())
		if err != nil {
			return err
		}
	}

	return nil
}

func (k *CRDClientV1) ReplaceBackendAndUpdateVolumes(origBackend, newBackend *storage.Backend) error {

	log.WithFields(log.Fields{
		"origBackend":             origBackend,
		"origBackend.Name":        origBackend.Name,
		"origBackend.BackendUUID": origBackend.BackendUUID,
		"newBackend":              newBackend,
		"newBackend.Name":         newBackend.Name,
	}).Debug("ReplaceBackendAndUpdateVolumes.")

	backend, err := k.getBackendCRD(origBackend.Name)
	if err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"backend":                 backend,
		"backend.Name":            backend.Name,
		"backend.BackendName":     backend.BackendName,
		"backend.BackendUUID":     backend.BackendUUID,
		"backend.ObjectMeta.Name": backend.ObjectMeta.Name,
	}).Debug("Found backend.")

	if err = backend.Apply(newBackend.ConstructPersistent()); err != nil {
		return err
	}

	_, err = k.client.TridentV1().TridentBackends(k.namespace).Update(backend)
	if err != nil {
		log.WithFields(log.Fields{
			"err": err.Error(),
		}).Error("Problem with update.")
		return err
	}

	return nil
}

func (k *CRDClientV1) AddVolume(volume *storage.Volume) error {

	persistentVolume, err := v1.NewTridentVolume(volume.ConstructExternal())
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"volume":                       volume,
		"volume.BackendUUID":           volume.BackendUUID,
		"persistentVolume":             persistentVolume,
		"persistentVolume.BackendUUID": persistentVolume.BackendUUID,
	}).Debug("AddVolume")

	_, err = k.client.TridentV1().TridentVolumes(k.namespace).Create(persistentVolume)
	if err != nil {
		return err
	}

	return nil
}

// AddVolumePersistent saves a volume's persistent state to the persistent store
func (k *CRDClientV1) AddVolumePersistent(volume *storage.VolumeExternal) error {

	persistentVolume, err := v1.NewTridentVolume(volume)
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().TridentVolumes(k.namespace).Create(persistentVolume)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) HasVolumes() (bool, error) {

	listOneOpts := metav1.ListOptions{Limit: 1}
	volumeList, err := k.client.TridentV1().TridentVolumes(k.namespace).List(listOneOpts)
	if err != nil {
		return false, err
	}

	return len(volumeList.Items) > 0, nil
}

func (k *CRDClientV1) GetVolume(volName string) (*storage.VolumeExternal, error) {

	volume, err := k.client.TridentV1().TridentVolumes(k.namespace).Get(v1.NameFix(volName), getOpts)

	if err != nil {
		return nil, err
	}

	persistentVolume, err := volume.Persistent()
	if err != nil {
		return nil, err
	}

	return persistentVolume, nil
}

func (k *CRDClientV1) UpdateVolume(update *storage.Volume) error {

	volume, err := k.client.TridentV1().TridentVolumes(k.namespace).Get(v1.NameFix(update.Config.Name), getOpts)
	if err != nil {
		return err
	}

	if err = volume.Apply(update.ConstructExternal()); err != nil {
		return err
	}

	_, err = k.client.TridentV1().TridentVolumes(k.namespace).Update(volume)
	if err != nil {
		return err
	}

	return nil
}

// UpdateVolumePersistent updates a volume's persistent state
func (k *CRDClientV1) UpdateVolumePersistent(update *storage.VolumeExternal) error {
	volume, err := k.client.TridentV1().TridentVolumes(k.namespace).Get(v1.NameFix(update.Config.Name), getOpts)
	if err != nil {
		return err
	}

	if err = volume.Apply(update); err != nil {
		return err
	}

	_, err = k.client.TridentV1().TridentVolumes(k.namespace).Update(volume)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) DeleteVolume(volume *storage.Volume) error {
	return k.client.TridentV1().TridentVolumes(k.namespace).Delete(v1.NameFix(volume.Config.Name), k.deleteOpts())
}

func (k *CRDClientV1) DeleteVolumeIgnoreNotFound(volume *storage.Volume) error {

	err := k.client.TridentV1().TridentVolumes(k.namespace).Delete(v1.NameFix(volume.Config.Name), k.deleteOpts())

	if errors.IsNotFound(err) {
		return nil
	}

	return err
}

func (k *CRDClientV1) GetVolumes() ([]*storage.VolumeExternal, error) {

	volumeList, err := k.client.TridentV1().TridentVolumes(k.namespace).List(listOpts)
	if err != nil {
		return nil, err
	}

	results := make([]*storage.VolumeExternal, 0)

	for _, item := range volumeList.Items {
		if !item.ObjectMeta.DeletionTimestamp.IsZero() {
			log.WithFields(log.Fields{
				"Name":              item.Name,
				"BackendUUID":       item.BackendUUID,
				"DeletionTimestamp": item.DeletionTimestamp,
			}).Debug("GetVolumes skipping deleted Volume")
			continue
		}

		persistentVolume, err := item.Persistent()
		if err != nil {
			return nil, err
		}

		results = append(results, persistentVolume)
	}

	return results, nil
}

func (k *CRDClientV1) DeleteVolumes() error {

	volumeList, err := k.client.TridentV1().TridentVolumes(k.namespace).List(listOpts)
	if err != nil {
		return err
	}

	for _, item := range volumeList.Items {
		err := k.client.TridentV1().TridentVolumes(k.namespace).Delete(item.ObjectMeta.Name, k.deleteOpts())
		if err != nil {
			return err
		}
	}

	return nil
}

func (k *CRDClientV1) AddVolumeTransaction(txn *storage.VolumeTransaction) error {

	newTxn, err := v1.NewTridentTransaction(txn)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"op":   txn.Op,
		"name": v1.NameFix(txn.Name()),
	}).Debug("AddVolumeTransaction")

	if _, err = k.client.TridentV1().TridentTransactions(k.namespace).Create(newTxn); err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) HasVolumeTransactions() (bool, error) {

	listOneOpts := metav1.ListOptions{Limit: 1}
	txnList, err := k.client.TridentV1().TridentTransactions(k.namespace).List(listOneOpts)
	if err != nil {
		return false, err
	}

	return len(txnList.Items) > 0, nil
}

func (k *CRDClientV1) GetVolumeTransactions() ([]*storage.VolumeTransaction, error) {

	txnList, err := k.client.TridentV1().TridentTransactions(k.namespace).List(listOpts)
	if err != nil {
		return nil, err
	}

	results := make([]*storage.VolumeTransaction, 0)

	for _, item := range txnList.Items {
		if !item.ObjectMeta.DeletionTimestamp.IsZero() {
			log.WithFields(log.Fields{
				"Name":              item.Name,
				"DeletionTimestamp": item.DeletionTimestamp,
			}).Debug("GetVolumeTransactions skipping deleted VolumeTransaction")
			continue
		}

		if txn, err := item.Persistent(); err != nil {
			return nil, err
		} else {
			results = append(results, txn)
		}
	}

	return results, nil
}

func (k *CRDClientV1) GetExistingVolumeTransaction(
	volTxn *storage.VolumeTransaction,
) (*storage.VolumeTransaction, error) {

	ttxn, err := k.client.TridentV1().TridentTransactions(k.namespace).Get(v1.NameFix(volTxn.Name()), getOpts)

	if errors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	if !ttxn.ObjectMeta.DeletionTimestamp.IsZero() {
		log.WithFields(log.Fields{
			"Name":              ttxn.Name,
			"DeletionTimestamp": ttxn.DeletionTimestamp,
		}).Debug("GetExistingVolumeTransaction skipping deleted VolumeTransaction")
		return nil, err
	}

	if txn, err := ttxn.Persistent(); err != nil {
		return nil, err
	} else {
		return txn, nil
	}
}

func (k *CRDClientV1) DeleteVolumeTransaction(volTxn *storage.VolumeTransaction) error {
	return k.client.TridentV1().TridentTransactions(k.namespace).Delete(v1.NameFix(volTxn.Name()), k.deleteOpts())
}

func (k *CRDClientV1) AddStorageClass(sc *storageclass.StorageClass) error {

	persistentSC, err := v1.NewTridentStorageClass(sc.ConstructPersistent())
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().TridentStorageClasses(k.namespace).Create(persistentSC)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) AddStorageClassPersistent(scp *storageclass.Persistent) error {

	persistentSC, err := v1.NewTridentStorageClass(scp)
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().TridentStorageClasses(k.namespace).Create(persistentSC)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) HasStorageClasses() (bool, error) {

	listOneOpts := metav1.ListOptions{Limit: 1}
	scList, err := k.client.TridentV1().TridentStorageClasses(k.namespace).List(listOneOpts)
	if err != nil {
		return false, err
	}

	return len(scList.Items) > 0, nil
}

func (k *CRDClientV1) GetStorageClass(scName string) (*storageclass.Persistent, error) {

	sc, err := k.client.TridentV1().TridentStorageClasses(k.namespace).Get(v1.NameFix(scName), getOpts)
	if err != nil {
		return nil, err
	}

	persistentSC, err := sc.Persistent()
	if err != nil {
		return nil, err
	}

	return persistentSC, nil
}

func (k *CRDClientV1) GetStorageClasses() ([]*storageclass.Persistent, error) {

	scList, err := k.client.TridentV1().TridentStorageClasses(k.namespace).List(listOpts)
	if err != nil {
		return nil, err
	}

	results := make([]*storageclass.Persistent, 0)

	for _, item := range scList.Items {
		if !item.ObjectMeta.DeletionTimestamp.IsZero() {
			log.WithFields(log.Fields{
				"Name":              item.Name,
				"DeletionTimestamp": item.DeletionTimestamp,
			}).Debug("GetStorageClasses skipping deleted Storageclass")
			continue
		}

		persistentSC, err := item.Persistent()
		if err != nil {
			return nil, err
		}

		results = append(results, persistentSC)
	}

	return results, nil
}

func (k *CRDClientV1) DeleteStorageClass(sc *storageclass.StorageClass) error {
	return k.client.TridentV1().TridentStorageClasses(k.namespace).Delete(v1.NameFix(sc.GetName()), k.deleteOpts())
}

func (k *CRDClientV1) AddOrUpdateNode(node *utils.Node) error {

	// look to see if it's an existing one we need to update
	existingNode, err := k.client.TridentV1().TridentNodes(k.namespace).Get(v1.NameFix(node.Name), getOpts)
	if err != nil {
		if !IsStatusNotFoundError(err) {
			return err
		} else {
			existingNode = nil
		}
	}
	if existingNode != nil {
		// found it, apply the updated changes
		if err = existingNode.Apply(node); err != nil {
			return err
		}
		_, err = k.client.TridentV1().TridentNodes(k.namespace).Update(existingNode)
		if err != nil {
			return err
		}
		return nil
	}

	// could not find an existing TridentNode, create a new one
	newNode, err := v1.NewTridentNode(node)
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().TridentNodes(k.namespace).Create(newNode)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) GetNode(nName string) (*utils.Node, error) {

	node, err := k.client.TridentV1().TridentNodes(k.namespace).Get(v1.NameFix(nName), getOpts)
	if err != nil {
		return nil, err
	}

	persistentNode, err := node.Persistent()
	if err != nil {
		return nil, err
	}

	return persistentNode, nil
}

func (k *CRDClientV1) GetNodes() ([]*utils.Node, error) {

	nodeList, err := k.client.TridentV1().TridentNodes(k.namespace).List(listOpts)
	if err != nil {
		return nil, err
	}

	results := make([]*utils.Node, 0)

	for _, item := range nodeList.Items {
		if !item.ObjectMeta.DeletionTimestamp.IsZero() {
			log.WithFields(log.Fields{
				"Name":              item.Name,
				"DeletionTimestamp": item.DeletionTimestamp,
			}).Debug("GetNodes skipping deleted Node")
			continue
		}

		persistent, err := item.Persistent()
		if err != nil {
			return nil, err
		}

		results = append(results, persistent)
	}

	return results, nil
}

func (k *CRDClientV1) DeleteNode(n *utils.Node) error {
	return k.client.TridentV1().TridentNodes(k.namespace).Delete(v1.NameFix(n.Name), k.deleteOpts())
}

// deleteOpts returns a DeleteOptions struct suitable for most DELETE calls to the K8S REST API.
func (k *CRDClientV1) deleteOpts() *metav1.DeleteOptions {

	propagationPolicy := metav1.DeletePropagationBackground
	deleteOptions := &metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}

	return deleteOptions
}

func (k *CRDClientV1) AddSnapshot(snapshot *storage.Snapshot) error {

	persistentSnapshot, err := v1.NewTridentSnapshot(snapshot.ConstructPersistent())
	if err != nil {
		return err
	}

	_, err = k.client.TridentV1().TridentSnapshots(k.namespace).Create(persistentSnapshot)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) GetSnapshot(volumeName, snapshotName string) (*storage.SnapshotPersistent, error) {

	snapshotID := storage.MakeSnapshotID(volumeName, snapshotName)
	snapshot, err := k.client.TridentV1().TridentSnapshots(k.namespace).Get(v1.NameFix(snapshotID), getOpts)
	if err != nil {
		return nil, err
	}

	persistentSnapshot, err := snapshot.Persistent()
	if err != nil {
		return nil, err
	}

	return persistentSnapshot, nil
}

func (k *CRDClientV1) GetSnapshots() ([]*storage.SnapshotPersistent, error) {

	snapshotList, err := k.client.TridentV1().TridentSnapshots(k.namespace).List(listOpts)
	if err != nil {
		return nil, err
	}

	results := make([]*storage.SnapshotPersistent, 0)

	for _, item := range snapshotList.Items {
		if !item.ObjectMeta.DeletionTimestamp.IsZero() {
			log.WithFields(log.Fields{
				"Name":              item.Name,
				"DeletionTimestamp": item.DeletionTimestamp,
			}).Debug("GetSnapshots skipping deleted Snapshot")
			continue
		}

		persistentSnapshot, err := item.Persistent()
		if err != nil {
			return nil, err
		}

		results = append(results, persistentSnapshot)
	}

	return results, nil
}

func (k *CRDClientV1) DeleteSnapshot(snapshot *storage.Snapshot) error {
	return k.client.TridentV1().TridentSnapshots(k.namespace).Delete(v1.NameFix(snapshot.ID()), k.deleteOpts())
}

func (k *CRDClientV1) DeleteSnapshotIgnoreNotFound(snapshot *storage.Snapshot) error {

	err := k.client.TridentV1().TridentSnapshots(k.namespace).Delete(v1.NameFix(snapshot.ID()), k.deleteOpts())

	if errors.IsNotFound(err) {
		return nil
	}

	return err
}

func (k *CRDClientV1) DeleteSnapshots() error {

	snapshotList, err := k.client.TridentV1().TridentSnapshots(k.namespace).List(listOpts)
	if err != nil {
		return err
	}

	for _, item := range snapshotList.Items {
		err := k.client.TridentV1().TridentSnapshots(k.namespace).Delete(item.ObjectMeta.Name, k.deleteOpts())
		if err != nil {
			return err
		}
	}

	return nil
}
