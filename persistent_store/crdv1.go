// Copyright 2019 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
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

const (
	BackendSecretSource = "tridentbackends.trident.netapp.io"
)

var (
	listOpts = metav1.ListOptions{}
	getOpts  = metav1.GetOptions{}
)

// CRDClientV1 stores persistent state in CRD objects in Kubernetes
type CRDClientV1 struct {
	crdClient versioned.Interface
	k8sClient k8sclient.Interface
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
	k8sClient, err := k8sclient.NewKubectlClient("")
	if err != nil {
		return nil, fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	// When running in binary mode, we use the current namespace as determined by the CLI client
	return newCRDClientV1(kubeConfig, k8sClient)
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

	k8sClient, err := k8sclient.NewKubeClient(kubeConfig, string(namespaceBytes), 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	return newCRDClientV1(kubeConfig, k8sClient)
}

func newCRDClientV1(kubeConfig *rest.Config, k8sClient k8sclient.Interface) (*CRDClientV1, error) {

	crdClient, err := versioned.NewForConfig(kubeConfig)

	log.WithFields(log.Fields{
		"tridentNamespace": k8sClient.Namespace(),
	}).Debug("Created CRDv1 persistence client.")

	return &CRDClientV1{
		crdClient: crdClient,
		k8sClient: k8sClient,
		version: &config.PersistentStateVersion{
			PersistentStoreVersion: string(CRDV1Store),
			OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		},
		namespace: k8sClient.Namespace(),
	}, err
}

func (k *CRDClientV1) GetVersion() (*config.PersistentStateVersion, error) {

	versionList, err := k.crdClient.TridentV1().TridentVersions(k.namespace).List(listOpts)
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

	versionList, err := k.crdClient.TridentV1().TridentVersions(k.namespace).List(listOpts)
	if err != nil {
		return err
	}

	// If version doesn't exist, create it
	if versionList == nil || versionList.Items == nil || len(versionList.Items) == 0 {

		newVersion, err := v1.NewTridentVersion(version)
		if err != nil {
			return err
		}

		_, err = k.crdClient.TridentV1().TridentVersions(k.namespace).Create(newVersion)
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

	_, err = k.crdClient.TridentV1().TridentVersions(k.namespace).Update(existingVersion)
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

// AddBackend accepts a Backend object and persists it in a custom resource with all of its
// sensitive data redacted and written to a corresponding K8S Secret.
func (k *CRDClientV1) AddBackend(backend *storage.Backend) error {

	log.WithFields(log.Fields{
		"backend":      backend,
		"backend.Name": backend.Name,
	}).Debug("AddBackend.")

	return k.addBackendPersistent(backend.ConstructPersistent())
}

// AddBackendPersistent accepts a BackendPersistent object and persists it in a custom resource
// with all of its sensitive data redacted and written to a corresponding K8S Secret.
func (k *CRDClientV1) AddBackendPersistent(backendPersistent *storage.BackendPersistent) error {

	log.WithFields(log.Fields{
		"backend":      backendPersistent,
		"backend.Name": backendPersistent.Name,
	}).Trace("AddBackendPersistent.")

	return k.addBackendPersistent(backendPersistent)
}

// addBackendPersistent is the internal method shared by AddBackend and AddBackendPersistent.
func (k *CRDClientV1) addBackendPersistent(backendPersistent *storage.BackendPersistent) error {

	// Ensure the backend doesn't already exist
	if crd, err := k.getBackendCRD(backendPersistent.Name); crd != nil {
		return fmt.Errorf("backend %s already exists", backendPersistent.Name)
	} else if err != nil && !MatchKeyNotFoundErr(err) {
		return err
	}

	// Ensure the backend has a valid UUID and create the secret name
	if backendPersistent.BackendUUID == "" {
		return fmt.Errorf("backend %s does not have a UUID set", backendPersistent.Name)
	}
	secretName := k.backendSecretName(backendPersistent.BackendUUID)

	// In the unlikely event a secret already exists for this new backend, delete it
	if secretExists, err := k.k8sClient.CheckSecretExists(secretName); err != nil {
		return err
	} else if secretExists {
		if err = k.k8sClient.DeleteSecret(secretName); err != nil {
			return err
		}
	}

	// Extract sensitive info to a map
	redactedBackendPersistent, secretMap, err := backendPersistent.ExtractBackendSecrets(secretName)
	if err != nil {
		return err
	}

	// Create the backend resource struct
	tridentBackend, err := v1.NewTridentBackend(redactedBackendPersistent)
	if err != nil {
		return err
	}

	// Create the backend resource in Kubernetes
	crd, err := k.addBackendCRD(tridentBackend)
	if err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"backendName": crd.BackendName,
		"backendUUID": crd.BackendUUID,
		"backend":     crd.Name,
	}).Debug("Created backend resource.")

	// Create a secret containing the sensitive info
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: k.namespace,
			Labels: map[string]string{
				"source": BackendSecretSource,
				"tbe":    crd.Name,
			},
		},
		StringData: secretMap,
		Type:       corev1.SecretTypeOpaque,
	}

	if _, err = k.k8sClient.CreateSecret(secret); err != nil {
		log.WithField("secret", secretName).Error("Could not create backend secret, will delete backend resource.")

		// If secret creation failed, clean up by deleting the backend we just created
		deleteErr := k.crdClient.TridentV1().TridentBackends(k.namespace).Delete(crd.Name, k.deleteOpts())
		if deleteErr != nil {
			log.WithField("backend", crd.Name).Error("Could not delete backend resource after secret create failure.")
		} else {
			log.WithField("backend", crd.Name).Warning("Deleted backend resource after secret create failure.")
		}

		return err
	}
	log.WithField("secret", secretName).Debug("Created backend secret.")

	return nil
}

// backendSecretName is the only method that creates the name of a backend's corresponding secret.
func (k *CRDClientV1) backendSecretName(backendUUID string) string {
	return fmt.Sprintf("tbe-%s", backendUUID)
}

// addBackendCRD accepts a backend resource structure and creates it in Kubernetes.
func (k *CRDClientV1) addBackendCRD(backend *v1.TridentBackend) (*v1.TridentBackend, error) {

	log.WithField("backendName", backend.Name).Debug("addBackendCRD")

	return k.crdClient.TridentV1().TridentBackends(k.namespace).Create(backend)
}

// HasBackends returns true if any backend objects have been persisted as custom
// resources in the current namespace.
func (k *CRDClientV1) HasBackends() (bool, error) {

	listOneOpts := metav1.ListOptions{Limit: 1}
	backendList, err := k.crdClient.TridentV1().TridentBackends(k.namespace).List(listOneOpts)
	if err != nil {
		return false, err
	}

	return len(backendList.Items) > 0, nil
}

// GetBackend retrieves the list of backends persisted as custom resources, finds the
// one with the specified backend name, and returns the BackendPersistent form of the
// object with all of its sensitive fields filled in from the corresponding K8S secret.
func (k *CRDClientV1) GetBackend(backendName string) (*storage.BackendPersistent, error) {

	log.WithField("backendName", backendName).Debug("GetBackend")

	// Get all backend resources
	list, err := k.crdClient.TridentV1().TridentBackends(k.namespace).List(listOpts)
	if err != nil {
		return nil, err
	} else if list == nil || list.Items == nil || len(list.Items) == 0 {
		return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
	}

	var backendPersistent *storage.BackendPersistent

	// Find the backend with the name we want
	for _, backend := range list.Items {

		log.WithFields(log.Fields{
			"name":        backend.Name,
			"backendName": backend.BackendName,
		}).Debugf("Checking for backend %s.", backendName)

		if backend.BackendName == backendName {
			if backendPersistent, err = backend.Persistent(); err != nil {
				return nil, err
			}
			break
		}
	}

	// If we didn't find anything, just return
	if backendPersistent == nil {
		return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
	}

	// Fetch the corresponding secret and merge it into the BackendPersistent object
	return k.addSecretToBackend(backendPersistent)
}

// getBackendCRD retrieves the list of backends persisted as custom resources, finds the
// one with the specified backend name, and returns the CRD form of the object.  This is
// an internal method that does not fetch any Secret data.
func (k *CRDClientV1) getBackendCRD(backendName string) (*v1.TridentBackend, error) {

	log.WithField("backendName", backendName).Debug("getBackendCRD")

	list, err := k.crdClient.TridentV1().TridentBackends(k.namespace).List(listOpts)
	if err != nil {
		return nil, err
	} else if list == nil || list.Items == nil || len(list.Items) == 0 {
		return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
	}

	for _, backend := range list.Items {
		log.WithFields(log.Fields{
			"name":        backend.Name,
			"backendName": backend.BackendName,
		}).Debugf("Checking for backend %s.", backendName)

		if backend.BackendName == backendName {
			return backend, nil
		}
	}

	return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
}

// addSecretToBackend accepts a BackendPersistent object, retrieves the corresponding secret
// containing any sensitive data for that backend, and returns the same object with all of its
// sensitive fields filled in.
func (k *CRDClientV1) addSecretToBackend(
	backendPersistent *storage.BackendPersistent,
) (*storage.BackendPersistent, error) {

	// Get the secret containing the backend's sensitive info
	secret, err := k.k8sClient.GetSecret(k.backendSecretName(backendPersistent.BackendUUID))
	if err != nil {
		return nil, err
	}

	// Decode secret data into map.  The fake client returns only StringData while the real
	// API returns only Data, so we must use both here to support the unit tests.
	secretMap := make(map[string]string)
	for key, value := range secret.Data {
		secretMap[key] = string(value)
	}
	for key, value := range secret.StringData {
		secretMap[key] = string(value)
	}
	// Set all sensitive fields on the backend
	if err := backendPersistent.InjectBackendSecrets(secretMap); err != nil {
		return nil, err
	}

	return backendPersistent, nil
}

// UpdateBackend uses a Backend object to update a backend's persistent state
func (k *CRDClientV1) UpdateBackend(update *storage.Backend) error {

	log.WithFields(log.Fields{
		"update":      update,
		"update.Name": update.Name,
	}).Debug("UpdateBackend.")

	return k.updateBackendPersistent(update.ConstructPersistent())
}

// UpdateBackendPersistent uses a BackendPersistent object to update a backend's persistent state
func (k *CRDClientV1) UpdateBackendPersistent(update *storage.BackendPersistent) error {

	log.WithFields(log.Fields{
		"update":      update,
		"update.Name": update.Name,
	}).Debug("UpdateBackendPersistent.")

	return k.updateBackendPersistent(update)
}

// updateBackendPersistent is the internal method shared by UpdateBackend and UpdateBackendPersistent.
func (k *CRDClientV1) updateBackendPersistent(backendPersistent *storage.BackendPersistent) error {

	// Ensure the backend has a valid UUID and create the secret name
	if backendPersistent.BackendUUID == "" {
		return fmt.Errorf("backend %s does not have a UUID set", backendPersistent.Name)
	}
	secretName := k.backendSecretName(backendPersistent.BackendUUID)

	// Get the CRD that we will update
	crd, err := k.getBackendCRD(backendPersistent.Name)
	if err != nil {
		return err
	}

	// Make a copy in case we have to roll back
	origCRDCopy := crd.DeepCopy()

	// Get the secret that we will update
	secret, err := k.k8sClient.GetSecret(secretName)
	if err != nil {
		return err
	}

	// Extract sensitive info to a map and write it to the secret
	redactedBackendPersistent, secretMap, err := backendPersistent.ExtractBackendSecrets(secretName)
	if err != nil {
		return err
	}
	secret.StringData = secretMap

	// Update the backend resource struct
	if err = crd.Apply(redactedBackendPersistent); err != nil {
		return err
	}

	// Update the backend resource in Kubernetes
	if _, err = k.crdClient.TridentV1().TridentBackends(k.namespace).Update(crd); err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"backendName": crd.BackendName,
		"backendUUID": crd.BackendUUID,
		"backend":     crd.Name,
	}).Debug("Updated backend resource.")

	// Update the secret
	if _, err := k.k8sClient.UpdateSecret(secret); err != nil {
		log.WithField("secret", secretName).Error("Could not update backend secret, will unroll backend update.")

		// If the secret update failed, unroll the backend update
		if _, updateErr := k.crdClient.TridentV1().TridentBackends(k.namespace).Update(origCRDCopy); updateErr != nil {
			log.WithField("backend", crd.Name).Error("Could not restore backend after secret update failure.")
		} else {
			log.WithField("backend", crd.Name).Warning("Restored backend after secret update failure.")
		}

		return err
	}
	log.WithField("secret", secretName).Debug("Updated backend secret.")

	return nil
}

// DeleteBackend accepts a Backend object and deletes the custom resource from Kubernetes along
// with its corresponding secret.
func (k *CRDClientV1) DeleteBackend(b *storage.Backend) error {

	log.WithFields(log.Fields{
		"b.Name":        b.Name,
		"b.BackendUUID": b.BackendUUID,
	}).Debug("DeleteBackend.")

	// Get the CRD that we will delete
	backend, err := k.getBackendCRD(b.Name)
	if err != nil {
		return err
	}

	// Delete the backend resource
	if err := k.crdClient.TridentV1().TridentBackends(k.namespace).Delete(backend.Name, k.deleteOpts()); err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"backend":  backend.BackendName,
		"resource": backend.Name,
	}).Debug("Deleted backend resource.")

	// Delete the secret
	secretName := k.backendSecretName(b.BackendUUID)
	if err := k.k8sClient.DeleteSecret(secretName); err != nil {
		return err
	}
	log.WithField("secret", secretName).Debug("Deleted backend secret.")

	return nil
}

// GetBackends retrieves the list of backends persisted as custom resources and returns them
// as BackendPersistent objects with all of their sensitive fields filled in from their
// corresponding K8S secrets.
func (k *CRDClientV1) GetBackends() ([]*storage.BackendPersistent, error) {

	// Get the backend resources
	backendList, err := k.crdClient.TridentV1().TridentBackends(k.namespace).List(listOpts)
	if err != nil {
		return nil, err
	}

	results := make([]*storage.BackendPersistent, 0)

	for _, backend := range backendList.Items {

		// Convert backend resource into BackendPersistent object
		backendPersistent, err := backend.Persistent()
		if err != nil {
			return nil, err
		}

		// Fetch the corresponding secret and merge it into the BackendPersistent object
		backendPersistent, err = k.addSecretToBackend(backendPersistent)
		if err != nil {
			return nil, err
		}

		results = append(results, backendPersistent)
	}

	return results, nil
}

// DeleteBackend deletes all backend custom resources from Kubernetes along
// with their corresponding secrets.
func (k *CRDClientV1) DeleteBackends() error {

	log.Debug("DeleteBackends.")

	backendList, err := k.crdClient.TridentV1().TridentBackends(k.namespace).List(listOpts)
	if err != nil {
		return err
	}

	for _, backend := range backendList.Items {

		// Delete the backend resource
		err = k.crdClient.TridentV1().TridentBackends(k.namespace).Delete(backend.Name, k.deleteOpts())
		if err != nil {
			log.WithField("error", err).Error("Could not delete backend.")
			return err
		}
		log.WithFields(log.Fields{
			"backend":  backend.BackendName,
			"resource": backend.Name,
		}).Debug("Deleted backend resource.")

		// Delete the secret
		secretName := k.backendSecretName(backend.BackendUUID)
		err = k.k8sClient.DeleteSecret(secretName)
		if err != nil {
			log.WithField("error", err).Error("Could not delete secret.")
			return err
		}
		log.WithField("secret", secretName).Debug("Deleted backend secret.")
	}

	log.Debug("Deleted all backend resources and secrets.")

	return nil
}

// ReplaceBackendAndUpdateVolumes accepts two backend objects (origBackend and newBackend) and uses the
// new backend object to replace the original one.
func (k *CRDClientV1) ReplaceBackendAndUpdateVolumes(origBackend, newBackend *storage.Backend) error {

	log.WithFields(log.Fields{
		"origBackend":             origBackend,
		"origBackend.Name":        origBackend.Name,
		"origBackend.BackendUUID": origBackend.BackendUUID,
		"newBackend":              newBackend,
		"newBackend.Name":         newBackend.Name,
	}).Debug("ReplaceBackendAndUpdateVolumes.")

	// Get the custom resource for the original backend
	origCRD, err := k.getBackendCRD(origBackend.Name)
	if err != nil {
		return err
	}

	// Make a copy in case we have to roll back
	origCRDCopy := origCRD.DeepCopy()

	log.WithFields(log.Fields{
		"backend":                 origCRD,
		"backend.Name":            origCRD.Name,
		"backend.BackendName":     origCRD.BackendName,
		"backend.BackendUUID":     origCRD.BackendUUID,
		"backend.ObjectMeta.Name": origCRD.ObjectMeta.Name,
	}).Debug("Found backend.")

	// Ensure the backend has a valid UUID and create the secret name
	if origCRD.BackendUUID == "" {
		return fmt.Errorf("backend %s does not have a UUID set", origBackend.Name)
	}
	secretName := k.backendSecretName(origCRD.BackendUUID)

	// Get the secret that we will update
	secret, err := k.k8sClient.GetSecret(secretName)
	if err != nil {
		return err
	}

	// Get the persistent form of the new backend so we can update the resource with it
	newBackendPersistent := newBackend.ConstructPersistent()

	// Extract sensitive info to a map and write it to the secret
	redactedNewBackendPersistent, secretMap, err := newBackendPersistent.ExtractBackendSecrets(secretName)
	if err != nil {
		return err
	}
	secret.StringData = secretMap

	// Update the backend resource struct
	if err = origCRD.Apply(redactedNewBackendPersistent); err != nil {
		return err
	}

	// Update the backend resource in Kubernetes
	newCRD, err := k.crdClient.TridentV1().TridentBackends(k.namespace).Update(origCRD)
	if err != nil {
		log.WithField("error", err).Error("Could not update backend.")
		return err
	}

	log.WithFields(log.Fields{
		"backendName": newCRD.BackendName,
		"backendUUID": newCRD.BackendUUID,
		"backend":     newCRD.Name,
	}).Debug("Replaced backend resource.")

	// Update the secret
	if _, err := k.k8sClient.UpdateSecret(secret); err != nil {

		log.WithField("secret", secretName).Error("Could not update backend secret, will unroll backend update.")

		// If the secret update failed, unroll the backend update
		if _, updateErr := k.crdClient.TridentV1().TridentBackends(k.namespace).Update(origCRDCopy); updateErr != nil {
			log.WithField("backend", origCRD.Name).Error("Could not restore backend after secret update failure.")
		} else {
			log.WithField("backend", origCRD.Name).Warning("Restored backend after secret update failure.")
		}

		return err
	}
	log.WithField("secret", secretName).Debug("Updated backend secret.")

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

	_, err = k.crdClient.TridentV1().TridentVolumes(k.namespace).Create(persistentVolume)
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

	_, err = k.crdClient.TridentV1().TridentVolumes(k.namespace).Create(persistentVolume)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) HasVolumes() (bool, error) {

	listOneOpts := metav1.ListOptions{Limit: 1}
	volumeList, err := k.crdClient.TridentV1().TridentVolumes(k.namespace).List(listOneOpts)
	if err != nil {
		return false, err
	}

	return len(volumeList.Items) > 0, nil
}

func (k *CRDClientV1) GetVolume(volName string) (*storage.VolumeExternal, error) {

	volume, err := k.crdClient.TridentV1().TridentVolumes(k.namespace).Get(v1.NameFix(volName), getOpts)

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

	volume, err := k.crdClient.TridentV1().TridentVolumes(k.namespace).Get(v1.NameFix(update.Config.Name), getOpts)
	if err != nil {
		return err
	}

	if err = volume.Apply(update.ConstructExternal()); err != nil {
		return err
	}

	_, err = k.crdClient.TridentV1().TridentVolumes(k.namespace).Update(volume)
	if err != nil {
		return err
	}

	return nil
}

// UpdateVolumePersistent updates a volume's persistent state
func (k *CRDClientV1) UpdateVolumePersistent(update *storage.VolumeExternal) error {
	volume, err := k.crdClient.TridentV1().TridentVolumes(k.namespace).Get(v1.NameFix(update.Config.Name), getOpts)
	if err != nil {
		return err
	}

	if err = volume.Apply(update); err != nil {
		return err
	}

	_, err = k.crdClient.TridentV1().TridentVolumes(k.namespace).Update(volume)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) DeleteVolume(volume *storage.Volume) error {
	return k.crdClient.TridentV1().TridentVolumes(k.namespace).Delete(v1.NameFix(volume.Config.Name), k.deleteOpts())
}

func (k *CRDClientV1) DeleteVolumeIgnoreNotFound(volume *storage.Volume) error {

	err := k.crdClient.TridentV1().TridentVolumes(k.namespace).Delete(v1.NameFix(volume.Config.Name), k.deleteOpts())

	if errors.IsNotFound(err) {
		return nil
	}

	return err
}

func (k *CRDClientV1) GetVolumes() ([]*storage.VolumeExternal, error) {

	volumeList, err := k.crdClient.TridentV1().TridentVolumes(k.namespace).List(listOpts)
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

	volumeList, err := k.crdClient.TridentV1().TridentVolumes(k.namespace).List(listOpts)
	if err != nil {
		return err
	}

	for _, item := range volumeList.Items {
		err := k.crdClient.TridentV1().TridentVolumes(k.namespace).Delete(item.ObjectMeta.Name, k.deleteOpts())
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

	if _, err = k.crdClient.TridentV1().TridentTransactions(k.namespace).Create(newTxn); err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) HasVolumeTransactions() (bool, error) {

	listOneOpts := metav1.ListOptions{Limit: 1}
	txnList, err := k.crdClient.TridentV1().TridentTransactions(k.namespace).List(listOneOpts)
	if err != nil {
		return false, err
	}

	return len(txnList.Items) > 0, nil
}

func (k *CRDClientV1) GetVolumeTransactions() ([]*storage.VolumeTransaction, error) {

	txnList, err := k.crdClient.TridentV1().TridentTransactions(k.namespace).List(listOpts)
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

	ttxn, err := k.crdClient.TridentV1().TridentTransactions(k.namespace).Get(v1.NameFix(volTxn.Name()), getOpts)

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
	return k.crdClient.TridentV1().TridentTransactions(k.namespace).Delete(v1.NameFix(volTxn.Name()), k.deleteOpts())
}

func (k *CRDClientV1) AddStorageClass(sc *storageclass.StorageClass) error {

	persistentSC, err := v1.NewTridentStorageClass(sc.ConstructPersistent())
	if err != nil {
		return err
	}

	_, err = k.crdClient.TridentV1().TridentStorageClasses(k.namespace).Create(persistentSC)
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

	_, err = k.crdClient.TridentV1().TridentStorageClasses(k.namespace).Create(persistentSC)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) HasStorageClasses() (bool, error) {

	listOneOpts := metav1.ListOptions{Limit: 1}
	scList, err := k.crdClient.TridentV1().TridentStorageClasses(k.namespace).List(listOneOpts)
	if err != nil {
		return false, err
	}

	return len(scList.Items) > 0, nil
}

func (k *CRDClientV1) GetStorageClass(scName string) (*storageclass.Persistent, error) {

	sc, err := k.crdClient.TridentV1().TridentStorageClasses(k.namespace).Get(v1.NameFix(scName), getOpts)
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

	scList, err := k.crdClient.TridentV1().TridentStorageClasses(k.namespace).List(listOpts)
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
	return k.crdClient.TridentV1().TridentStorageClasses(k.namespace).Delete(v1.NameFix(sc.GetName()), k.deleteOpts())
}

func (k *CRDClientV1) AddOrUpdateNode(node *utils.Node) error {

	// look to see if it's an existing one we need to update
	existingNode, err := k.crdClient.TridentV1().TridentNodes(k.namespace).Get(v1.NameFix(node.Name), getOpts)
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
		_, err = k.crdClient.TridentV1().TridentNodes(k.namespace).Update(existingNode)
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

	_, err = k.crdClient.TridentV1().TridentNodes(k.namespace).Create(newNode)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) GetNode(nName string) (*utils.Node, error) {

	node, err := k.crdClient.TridentV1().TridentNodes(k.namespace).Get(v1.NameFix(nName), getOpts)
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

	nodeList, err := k.crdClient.TridentV1().TridentNodes(k.namespace).List(listOpts)
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
	return k.crdClient.TridentV1().TridentNodes(k.namespace).Delete(v1.NameFix(n.Name), k.deleteOpts())
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

	_, err = k.crdClient.TridentV1().TridentSnapshots(k.namespace).Create(persistentSnapshot)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) GetSnapshot(volumeName, snapshotName string) (*storage.SnapshotPersistent, error) {

	snapshotID := storage.MakeSnapshotID(volumeName, snapshotName)
	snapshot, err := k.crdClient.TridentV1().TridentSnapshots(k.namespace).Get(v1.NameFix(snapshotID), getOpts)
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

	snapshotList, err := k.crdClient.TridentV1().TridentSnapshots(k.namespace).List(listOpts)
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
	return k.crdClient.TridentV1().TridentSnapshots(k.namespace).Delete(v1.NameFix(snapshot.ID()), k.deleteOpts())
}

func (k *CRDClientV1) DeleteSnapshotIgnoreNotFound(snapshot *storage.Snapshot) error {

	err := k.crdClient.TridentV1().TridentSnapshots(k.namespace).Delete(v1.NameFix(snapshot.ID()), k.deleteOpts())

	if errors.IsNotFound(err) {
		return nil
	}

	return err
}

func (k *CRDClientV1) DeleteSnapshots() error {

	snapshotList, err := k.crdClient.TridentV1().TridentSnapshots(k.namespace).List(listOpts)
	if err != nil {
		return err
	}

	for _, item := range snapshotList.Items {
		err := k.crdClient.TridentV1().TridentSnapshots(k.namespace).Delete(item.ObjectMeta.Name, k.deleteOpts())
		if err != nil {
			return err
		}
	}

	return nil
}
