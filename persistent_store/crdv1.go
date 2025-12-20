// Copyright 2025 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	k8sapierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clik8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentv1clientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/models"
)

// Compile time checks to ensure CRDClientV1 implements Client & CRDClient
var (
	_ Client    = &CRDClientV1{}
	_ CRDClient = &CRDClientV1{}
)

const (
	BackendSecretSource = "tridentbackends.trident.netapp.io"
)

var (
	listOpts   = metav1.ListOptions{}
	getOpts    = metav1.GetOptions{}
	createOpts = metav1.CreateOptions{}
	updateOpts = metav1.UpdateOptions{}
)

// CRDClientV1 stores persistent state in CRD objects in Kubernetes
type CRDClientV1 struct {
	crdClient tridentv1clientset.Interface
	k8sClient clik8sclient.KubernetesClient
	version   *config.PersistentStateVersion
	namespace string
}

func NewCRDClientV1(masterURL, kubeConfigPath string) (*CRDClientV1, error) {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowStorageClientCreate,
		LogLayerPersistentStore)

	Logc(ctx).Debug("Creating CRDv1 persistent store client.")

	clients, err := clik8sclient.CreateK8SClients(masterURL, kubeConfigPath, "")
	if err != nil {
		return nil, err
	}

	Logc(ctx).WithFields(LogFields{
		"tridentNamespace": clients.Namespace,
	}).Trace("Created CRDv1 persistence client.")

	return &CRDClientV1{
		crdClient: clients.TridentClient,
		k8sClient: clients.K8SClient,
		version: &config.PersistentStateVersion{
			PersistentStoreVersion: string(CRDV1Store),
			OrchestratorAPIVersion: config.OrchestratorAPIVersion,
		},
		namespace: clients.Namespace,
	}, err
}

func (k *CRDClientV1) GetTridentUUID(ctx context.Context) (string, error) {
	versionList, err := k.crdClient.TridentV1().TridentVersions(k.namespace).List(ctx, listOpts)
	if err != nil {
		if strings.Contains(err.Error(), "the server could not find the requested resource") {
			return "", NewPersistentStoreError(KeyNotFoundErr, v1.PersistentStateVersionName)
		}
		return "", err
	} else if versionList == nil || versionList.Items == nil || len(versionList.Items) == 0 {
		return "", NewPersistentStoreError(KeyNotFoundErr, v1.PersistentStateVersionName)
	}

	return string(versionList.Items[0].ObjectMeta.GetUID()), nil
}

func (k *CRDClientV1) GetVersion(ctx context.Context) (*config.PersistentStateVersion, error) {
	versionList, err := k.crdClient.TridentV1().TridentVersions(k.namespace).List(ctx, listOpts)
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

func (k *CRDClientV1) SetVersion(ctx context.Context, version *config.PersistentStateVersion) error {
	versionList, err := k.crdClient.TridentV1().TridentVersions(k.namespace).List(ctx, listOpts)
	if err != nil {
		return err
	}

	// If version doesn't exist, create it
	if versionList == nil || versionList.Items == nil || len(versionList.Items) == 0 {

		newVersion, err := v1.NewTridentVersion(version)
		if err != nil {
			return err
		}

		_, err = k.crdClient.TridentV1().TridentVersions(k.namespace).Create(ctx, newVersion, createOpts)
		if err != nil {
			return err
		}

		Logc(ctx).WithFields(LogFields{
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

	_, err = k.crdClient.TridentV1().TridentVersions(k.namespace).Update(ctx, existingVersion, updateOpts)
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
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
func (k *CRDClientV1) AddBackend(ctx context.Context, backend storage.Backend) error {
	Logc(ctx).WithFields(LogFields{
		"backend":      backend,
		"backend.Name": backend.Name(),
	}).Debug("AddBackend.")

	return k.addBackendPersistent(ctx, backend.ConstructPersistent(ctx))
}

// addBackendPersistent is the internal method shared by AddBackend and AddBackendPersistent.
func (k *CRDClientV1) addBackendPersistent(ctx context.Context, backendPersistent *storage.BackendPersistent) error {
	// Ensure the backend doesn't already exist
	if crd, err := k.getBackendCRD(ctx, backendPersistent.Name); crd != nil {
		return fmt.Errorf("backend %s already exists", backendPersistent.Name)
	} else if err != nil && !MatchKeyNotFoundErr(err) {
		return err
	}

	// Ensure the backend has a valid UUID and create the secret name
	if backendPersistent.BackendUUID == "" {
		return fmt.Errorf("backend %s does not have a UUID set", backendPersistent.Name)
	}

	secretName := k.backendSecretName(backendPersistent.BackendUUID)

	// Extract sensitive info to a map
	redactedBackendPersistent, secretMap, _, err := backendPersistent.ExtractBackendSecrets(secretName)
	if err != nil {
		return err
	}

	// secretMap should be empty if Credentials field exists, Trident should create Secrets instead
	credentialsFieldNotSet := secretMap != nil

	if credentialsFieldNotSet {
		// In the unlikely event a secret already exists for this new backend, delete it
		if secretExists, err := k.k8sClient.CheckSecretExists(secretName); err != nil {
			return err
		} else if secretExists {
			if err = k.k8sClient.DeleteSecretDefault(secretName); err != nil {
				return err
			}
		}
	}

	// Create the backend resource struct
	tridentBackend, err := v1.NewTridentBackend(ctx, redactedBackendPersistent)
	if err != nil {
		return err
	}

	// Create the backend resource in Kubernetes
	crd, err := k.addBackendCRD(ctx, tridentBackend)
	if err != nil {
		return err
	}
	Logc(ctx).WithFields(LogFields{
		"backendName": crd.BackendName,
		"backendUUID": crd.BackendUUID,
		"backend":     crd.Name,
	}).Debug("Created backend resource.")

	if credentialsFieldNotSet {
		// Create a secret containing the sensitive info
		secret := k.makeBackendSecret(secretName, crd, secretMap)

		if _, err = k.k8sClient.CreateSecret(secret); err != nil {
			Logc(ctx).WithField("secret", secretName).Error(
				"Could not create backend secret, will delete backend resource.")

			// If secret creation failed, clean up by deleting the backend we just created
			deleteErr := k.crdClient.TridentV1().TridentBackends(k.namespace).Delete(ctx, crd.Name, k.deleteOpts())
			if deleteErr != nil {
				Logc(ctx).WithField("backend", crd.Name).Error(
					"Could not delete backend resource after secret create failure.")
			} else {
				Logc(ctx).WithField("backend", crd.Name).Warning(
					"Deleted backend resource after secret create failure.")
			}

			return err
		}
		Logc(ctx).WithField("secret", secretName).Debug("Created backend secret.")
	}

	return nil
}

// backendSecretName is the only method that creates the name of a backend's corresponding secret.
func (k *CRDClientV1) backendSecretName(backendUUID string) string {
	return fmt.Sprintf("tbe-%s", backendUUID)
}

func (k *CRDClientV1) makeBackendSecret(
	name string, crd *v1.TridentBackend, secretMap map[string]string,
) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: k.namespace,
			Labels: map[string]string{
				"source": BackendSecretSource,
				"tbe":    crd.Name,
			},
		},
		StringData: secretMap,
		Type:       corev1.SecretTypeOpaque,
	}
}

// addBackendCRD accepts a backend resource structure and creates it in Kubernetes.
func (k *CRDClientV1) addBackendCRD(ctx context.Context, backend *v1.TridentBackend) (*v1.TridentBackend, error) {
	Logc(ctx).WithField("backendName", backend.Name).Debug("addBackendCRD")

	return k.crdClient.TridentV1().TridentBackends(k.namespace).Create(ctx, backend, createOpts)
}

// HasBackends returns true if any backend objects have been persisted as custom
// resources in the current namespace.
func (k *CRDClientV1) HasBackends(ctx context.Context) (bool, error) {
	listOneOpts := metav1.ListOptions{Limit: 1}
	backendList, err := k.crdClient.TridentV1().TridentBackends(k.namespace).List(ctx, listOneOpts)
	if err != nil {
		return false, err
	}

	return len(backendList.Items) > 0, nil
}

// GetBackend retrieves the list of backends persisted as custom resources, finds the
// one with the specified backend name, and returns the BackendPersistent form of the
// object with all of its sensitive fields filled in from the corresponding K8S secret.
func (k *CRDClientV1) GetBackend(ctx context.Context, backendName string) (*storage.BackendPersistent, error) {
	Logc(ctx).WithField("backendName", backendName).Debug("GetBackend")

	// Get all backend resources
	list, err := k.crdClient.TridentV1().TridentBackends(k.namespace).List(ctx, listOpts)
	if err != nil {
		return nil, err
	} else if list == nil || list.Items == nil || len(list.Items) == 0 {
		return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
	}

	var backendPersistent *storage.BackendPersistent

	// Find the backend with the name we want
	for _, backend := range list.Items {

		Logc(ctx).WithFields(LogFields{
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
	return k.addSecretToBackend(ctx, backendPersistent)
}

// getBackendCRD retrieves the list of backends persisted as custom resources, finds the
// one with the specified backend name, and returns the CRD form of the object.  This is
// an internal method that does not fetch any Secret data.
func (k *CRDClientV1) getBackendCRD(ctx context.Context, backendName string) (*v1.TridentBackend, error) {
	Logc(ctx).WithField("backendName", backendName).Debug("getBackendCRD")

	list, err := k.crdClient.TridentV1().TridentBackends(k.namespace).List(ctx, listOpts)
	if err != nil {
		return nil, err
	} else if list == nil || list.Items == nil || len(list.Items) == 0 {
		return nil, NewPersistentStoreError(KeyNotFoundErr, backendName)
	}

	for _, backend := range list.Items {
		Logc(ctx).WithFields(LogFields{
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
	ctx context.Context, backendPersistent *storage.BackendPersistent,
) (*storage.BackendPersistent, error) {
	logFields := LogFields{
		"persistentBackend.Name":        backendPersistent.Name,
		"persistentBackend.BackendUUID": backendPersistent.BackendUUID,
		"persistentBackend.online":      backendPersistent.Online,
		"persistentBackend.state":       backendPersistent.State,
		"handler":                       "Bootstrap",
	}

	var secretName string
	var secretType string
	var err error

	// Check if user-provided credentials are in use
	if secretName, secretType, err = backendPersistent.GetBackendCredentials(); err != nil {
		Logc(ctx).WithFields(logFields).Errorf("Could determined if credentials field exist; %v", err)
		return nil, err
	}

	if secretName == "" {
		// Credentials field not set, use the default backend secret name
		secretName = k.backendSecretName(backendPersistent.BackendUUID)
	}
	if secretType == "" {
		// Default secret type to K8s secret
		secretType = "secret"
	}

	// If the secret type isn't that of a Kubernetes secret, return here without retrieving anything.
	if secretType != "secret" {
		return backendPersistent, nil
	}

	// Before retrieving the secret, ensure it exists.  If we find the secret does not exist, we
	// log a warning but return the backend object unmodified so that Trident can bootstrap itself
	// normally (albeit with the backend in a failed state due to missing credentials).
	secretMap, err := k.GetBackendSecret(ctx, secretName)
	if err != nil {
		return nil, err
	} else if secretMap == nil {
		return backendPersistent, nil
	}

	// Set all sensitive fields on the backend
	if err := backendPersistent.InjectBackendSecrets(secretMap); err != nil {
		Logc(ctx).WithFields(logFields).Errorf("Could not inject backend secrets; %v", err)
		return nil, err
	}

	return backendPersistent, nil
}

// GetBackendSecret accepts a secret name and retrieves the corresponding secret
// containing any sensitive data.
func (k *CRDClientV1) GetBackendSecret(ctx context.Context, secretName string) (map[string]string, error) {
	// Before retrieving the secret, ensure it exists.  If we find the secret does not exist, we
	// log a warning.
	if exists, err := k.k8sClient.CheckSecretExists(secretName); err != nil {
		Logc(ctx).Errorf("Could not check for backend secret; %v", err)
		return nil, err
	} else if !exists {
		Logc(ctx).Warnf("Backend must be updated because its secret does not exist.")
		return nil, nil
	}

	// Get the secret containing the backend's sensitive info
	secret, err := k.k8sClient.GetSecret(secretName)
	if err != nil {
		Logc(ctx).Errorf("Could not get backend secret; %v", err)
		return nil, err
	}

	// Decode secret data into map.  The fake client returns only StringData while the real
	// API returns only Data, so we must use both here to support the unit tests.
	// Also users can use different cases when providing the secret, so it is better
	// to store them in a uniform manner and later on rely on same case to read them
	secretMap := make(map[string]string)
	for key, value := range secret.Data {
		secretMap[strings.ToLower(key)] = string(value)
	}
	for key, value := range secret.StringData {
		secretMap[strings.ToLower(key)] = value
	}

	Logc(ctx).Debugf("Retrieved backend secret.")

	return secretMap, nil
}

// UpdateBackend uses a Backend object to update a backend's persistent state
func (k *CRDClientV1) UpdateBackend(ctx context.Context, update storage.Backend) error {
	Logc(ctx).WithFields(LogFields{
		"update":      update,
		"update.Name": update.Name(),
	}).Debug("UpdateBackend.")

	return k.updateBackendPersistent(ctx, update.ConstructPersistent(ctx))
}

// updateBackendPersistent is the internal method shared by UpdateBackend and UpdateBackendPersistent.
func (k *CRDClientV1) updateBackendPersistent(ctx context.Context, backendPersistent *storage.BackendPersistent) error {
	// Ensure the backend has a valid UUID and create the secret name
	if backendPersistent.BackendUUID == "" {
		return fmt.Errorf("backend %s does not have a UUID set", backendPersistent.Name)
	}
	secretName := k.backendSecretName(backendPersistent.BackendUUID)

	// Get the CRD that we will update
	crd, err := k.getBackendCRD(ctx, backendPersistent.Name)
	if err != nil {
		return err
	}

	// Make a copy in case we have to roll back
	origCRDCopy := crd.DeepCopy()

	// Extract sensitive info to a map for storage in a secret
	redactedBackendPersistent, secretMap, usingTridentSecretName, err := backendPersistent.ExtractBackendSecrets(
		secretName)
	if err != nil {
		return err
	}

	// Update the backend resource struct
	if err = crd.Apply(ctx, redactedBackendPersistent); err != nil {
		return err
	}

	// Update the backend resource in Kubernetes
	if _, err = k.crdClient.TridentV1().TridentBackends(k.namespace).Update(ctx, crd, updateOpts); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"backendName": crd.BackendName,
		"backendUUID": crd.BackendUUID,
		"backend":     crd.Name,
	}).Debug("Updated backend resource.")

	// secretMap should be empty if Credentials field exists, Trident should update Secrets instead
	credentialsFieldNotSet := secretMap != nil

	if credentialsFieldNotSet {
		// Check if the secret exists, so we can update or create it as needed
		secretExists, err := k.k8sClient.CheckSecretExists(secretName)
		if err != nil {
			return err
		}

		var secretError error
		var secret *corev1.Secret

		if !secretExists {

			// Create a secret containing the backend's sensitive info
			secret = k.makeBackendSecret(secretName, crd, secretMap)

			if _, secretError = k.k8sClient.CreateSecret(secret); secretError != nil {
				Logc(ctx).WithFields(LogFields{
					"secret": secretName,
					"error":  secretError,
				}).Error("Could not create backend secret, will unroll backend update.")
			}

		} else {

			// Get the secret that we will update
			secret, secretError = k.k8sClient.GetSecret(secretName)

			if secretError != nil {
				// No need update if the get failed, so skip to the backend rollback
				Logc(ctx).WithFields(LogFields{
					"secret": secretName,
					"error":  secretError,
				}).Error("Could not get backend secret, will unroll backend update.")
			} else {

				// Copy the backend's sensitive info into the secret
				secret.StringData = secretMap

				// Update the secret
				if _, secretError = k.k8sClient.UpdateSecret(secret); secretError != nil {
					Logc(ctx).WithFields(LogFields{
						"secret": secretName,
						"error":  secretError,
					}).Error("Could not update backend secret, will unroll backend update.")
				}
			}
		}

		// If anything went wrong with the secret, unroll the backend update.
		if secretError != nil {

			// If the secret update failed, unroll the backend update
			_, updateErr := k.crdClient.TridentV1().TridentBackends(k.namespace).Update(ctx, origCRDCopy, updateOpts)
			if updateErr != nil {
				Logc(ctx).WithField("backend", crd.Name).Error("Could not restore backend after secret update failure.")
			} else {
				Logc(ctx).WithField("backend", crd.Name).Warning("Restored backend after secret update failure.")
			}

			return err
		}

		Logc(ctx).WithField("secret", secretName).Debug("Updated backend secret.")
	} else {
		// Do not delete secret if user provided the default tbe-<backendUUID> secret
		if !usingTridentSecretName {
			// If backend has been changed to use credentials field,
			// then attempt to delete the default tbe-<backendUUID> secret
			if secretExists, err := k.k8sClient.CheckSecretExists(secretName); err == nil && secretExists {
				if err := k.k8sClient.DeleteSecretDefault(secretName); err == nil {
					Logc(ctx).WithField("secret", secretName).Debug("Deleted old backend secret.")
				}
			}
		}
	}

	return nil
}

// DeleteBackend accepts a Backend object and deletes the custom resource from Kubernetes along
// with its corresponding secret.
func (k *CRDClientV1) DeleteBackend(ctx context.Context, b storage.Backend) (err error) {
	logFields := LogFields{
		"backendName": b.Name(),
		"backendUUID": b.BackendUUID(),
	}

	Logc(ctx).WithFields(logFields).Debug("DeleteBackend.")

	// Get the CR that needs to be deleted
	var backend *v1.TridentBackend
	backend, err = k.getBackendCRD(ctx, b.Name())
	if err != nil {
		if MatchKeyNotFoundErr(err) {
			keyError, ok := err.(*Error)
			if !ok {
				return errors.TypeAssertionError("err.(*Error)")
			}
			Logc(ctx).WithFields(logFields).Debugf("Unable to find key %s. No backend to remove.", keyError.Key)
		} else {
			Logc(ctx).WithFields(logFields).Errorf("Unable to remove backend: %v", err)
			return err
		}
	} else {
		logFields["resource"] = backend.Name

		// Delete the backend resource
		if err := k.crdClient.TridentV1().TridentBackends(k.namespace).Delete(
			ctx, backend.Name, k.deleteOpts(),
		); err != nil {
			return err
		}

		Logc(ctx).WithFields(logFields).Debug("Deleted backend resource.")
	}

	// Backend is either deleted or not present, now ensure finalizer is removed
	defer func() {
		if finalizerErr := k.removeBackendFinalizer(ctx, b); finalizerErr != nil {
			finalizerErr = fmt.Errorf("unable to remove backend finalizer: %v", finalizerErr)
			if err != nil {
				err = fmt.Errorf("%v; %v", finalizerErr, err)
			}
		}
	}()

	// Delete the secret if created by Trident
	if !b.IsCredentialsFieldSet(ctx) {
		secretName := k.backendSecretName(b.BackendUUID())
		if err := k.k8sClient.DeleteSecretDefault(secretName); err != nil {
			if k8sapierrors.IsNotFound(err) {
				Logc(ctx).WithField("secret", secretName).Warn("Secret not found while deleting backend")
			} else {
				return err
			}
		}
		Logc(ctx).WithField("secret", secretName).Debug("Deleted backend secret.")
	}

	return nil
}

// removeBackendFinalizer accepts a Backend object and removes the finalizer from the corresponding TridentBackend CR
func (k *CRDClientV1) removeBackendFinalizer(ctx context.Context, b storage.Backend) error {
	logFields := LogFields{
		"backendName": b.Name(),
		"backendUUID": b.BackendUUID(),
	}

	// Get the CRD that we will delete
	backend, err := k.getBackendCRD(ctx, b.Name())
	if err != nil {
		if MatchKeyNotFoundErr(err) {
			keyError, ok := err.(*Error)
			if !ok {
				return errors.TypeAssertionError("err.(*Error)")
			}
			Logc(ctx).WithFields(logFields).Debugf("Unable to find key %s. No finalizers to remove.", keyError.Key)
			return nil
		} else {
			Logc(ctx).WithFields(logFields).Errorf("Unable to remove finalizer: %v", err)
			return err
		}
	}
	logFields["resource"] = backend.Name

	if backend.HasTridentFinalizers() {
		Logc(ctx).WithFields(logFields).Debug("Has finalizers, removing them.")

		backendCopy := backend.DeepCopy()
		backendCopy.RemoveTridentFinalizers()
		if _, err := k.crdClient.TridentV1().TridentBackends(backend.Namespace).Update(
			ctx, backendCopy, updateOpts,
		); err != nil {
			Logc(ctx).WithFields(logFields).Errorf("Problem removing finalizers: %v", err)
			return err
		}
	} else {
		Logc(ctx).WithFields(logFields).Debug("No finalizers to remove.")
	}

	return nil
}

// IsBackendDeleting identifies if the backend is a deleting or not based on CR's deletionTimestamp
func (k *CRDClientV1) IsBackendDeleting(ctx context.Context, b storage.Backend) bool {
	logFields := LogFields{
		"backendName": b.Name(),
		"backendUUID": b.BackendUUID(),
	}

	// Get the CR that needs to be verified
	backend, err := k.getBackendCRD(ctx, b.Name())
	if err != nil {
		if MatchKeyNotFoundErr(err) {
			keyError := err.(*Error)
			Logc(ctx).WithFields(logFields).Debugf("Unable to find key %s. Backend may not exist", keyError.Key)
			return false
		} else {
			Logc(ctx).WithFields(logFields).Errorf("Unable identify if the backend exists: %v", err)
			return false
		}
	}
	logFields["resource"] = backend.Name

	// If the backend is deleting return true
	if !backend.ObjectMeta.DeletionTimestamp.IsZero() {
		Logc(ctx).WithFields(logFields).Debugf("Backend is deleting.")
		return true
	}

	return false
}

// GetBackends retrieves the list of backends persisted as custom resources and returns them
// as BackendPersistent objects with all of their sensitive fields filled in from their
// corresponding K8S secrets.
func (k *CRDClientV1) GetBackends(ctx context.Context) ([]*storage.BackendPersistent, error) {
	// Get the backend resources
	backendList, err := k.crdClient.TridentV1().TridentBackends(k.namespace).List(ctx, listOpts)
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
		backendPersistent, err = k.addSecretToBackend(ctx, backendPersistent)
		if err != nil {
			return nil, err
		}

		results = append(results, backendPersistent)
	}

	return results, nil
}

// DeleteBackends deletes all backend custom resources from Kubernetes along
// with their corresponding secrets.
func (k *CRDClientV1) DeleteBackends(ctx context.Context) error {
	Logc(ctx).Debug("DeleteBackends.")

	backendList, err := k.crdClient.TridentV1().TridentBackends(k.namespace).List(ctx, listOpts)
	if err != nil {
		return err
	}

	for _, backend := range backendList.Items {
		// Delete the backend resource
		err = k.crdClient.TridentV1().TridentBackends(k.namespace).Delete(ctx, backend.Name, k.deleteOpts())
		if err != nil {
			Logc(ctx).WithField("error", err).Error("Could not delete backend.")
			return err
		}
		Logc(ctx).WithFields(LogFields{
			"backend":  backend.BackendName,
			"resource": backend.Name,
		}).Debug("Deleted backend resource.")

		// Delete the secret
		secretName := k.backendSecretName(backend.BackendUUID)
		err = k.k8sClient.DeleteSecretDefault(secretName)
		if err != nil {
			Logc(ctx).WithField("error", err).Error("Could not delete secret.")
			return err
		}
		Logc(ctx).WithField("secret", secretName).Debug("Deleted backend secret.")
	}

	Logc(ctx).Debug("Deleted all backend resources and secrets.")

	return nil
}

// ReplaceBackendAndUpdateVolumes accepts two backend objects (origBackend and newBackend) and uses the
// new backend object to replace the original one.
func (k *CRDClientV1) ReplaceBackendAndUpdateVolumes(
	ctx context.Context, origBackend, newBackend storage.Backend,
) error {
	Logc(ctx).WithFields(LogFields{
		"origBackend":             origBackend,
		"origBackend.Name":        origBackend.Name(),
		"origBackend.BackendUUID": origBackend.BackendUUID(),
		"newBackend":              newBackend,
		"newBackend.Name":         newBackend.Name(),
	}).Debug("ReplaceBackendAndUpdateVolumes.")

	// Get the custom resource for the original backend
	origCRD, err := k.getBackendCRD(ctx, origBackend.Name())
	if err != nil {
		return err
	}

	// Make a copy in case we have to roll back
	origCRDCopy := origCRD.DeepCopy()

	Logc(ctx).WithFields(LogFields{
		"backend":                 origCRD,
		"backend.Name":            origCRD.Name,
		"backend.BackendName":     origCRD.BackendName,
		"backend.BackendUUID":     origCRD.BackendUUID,
		"backend.ObjectMeta.Name": origCRD.ObjectMeta.Name,
	}).Debug("Found backend.")

	// Ensure the backend has a valid UUID and create the secret name
	if origCRD.BackendUUID == "" {
		return fmt.Errorf("backend %s does not have a UUID set", origBackend.Name())
	}
	secretName := k.backendSecretName(origCRD.BackendUUID)

	// Get the persistent form of the new backend so we can update the resource with it
	newBackendPersistent := newBackend.ConstructPersistent(ctx)

	// Extract sensitive info to a map and write it to the secret
	redactedNewBackendPersistent, secretMap, usingTridentSecretName, err := newBackendPersistent.ExtractBackendSecrets(
		secretName)
	if err != nil {
		return err
	}

	// secretMap should be empty if Credentials field exists, Trident should create Secrets instead
	credentialsFieldNotSet := secretMap != nil

	var secret *corev1.Secret
	if credentialsFieldNotSet {
		// Get the secret that we will update
		secret, err = k.k8sClient.GetSecret(secretName)
		if err != nil {
			return err
		}
		secret.StringData = secretMap
	}

	// Update the backend resource struct
	if err = origCRD.Apply(ctx, redactedNewBackendPersistent); err != nil {
		return err
	}

	// Update the backend resource in Kubernetes
	newCRD, err := k.crdClient.TridentV1().TridentBackends(k.namespace).Update(ctx, origCRD, updateOpts)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Could not update backend.")
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"backendName": newCRD.BackendName,
		"backendUUID": newCRD.BackendUUID,
		"backend":     newCRD.Name,
	}).Debug("Replaced backend resource.")

	if credentialsFieldNotSet {
		// Update the secret
		if _, err := k.k8sClient.UpdateSecret(secret); err != nil {

			Logc(ctx).WithField("secret", secretName).Error(
				"Could not update backend secret, will unroll backend update.")

			// If the secret update failed, unroll the backend update
			_, updateErr := k.crdClient.TridentV1().TridentBackends(k.namespace).Update(ctx, origCRDCopy, updateOpts)
			if updateErr != nil {
				Logc(ctx).WithField("backend", origCRD.Name).Error(
					"Could not restore backend after secret update failure.")
			} else {
				Logc(ctx).WithField("backend", origCRD.Name).Warning("Restored backend after secret update failure.")
			}

			return err
		}
		Logc(ctx).WithField("secret", secretName).Debug("Updated backend secret.")
	} else {
		// Do not delete secret if user provided the default tbe-<backendUUID> secret
		if !usingTridentSecretName {
			// If backend has been changed to use credentials field,
			// then attempt to delete the default tbe-<backendUUID> secret
			if secretExists, err := k.k8sClient.CheckSecretExists(secretName); err == nil && secretExists {
				if err := k.k8sClient.DeleteSecretDefault(secretName); err == nil {
					Logc(ctx).WithField("secret", secretName).Debug("Deleted old backend secret.")
				}
			}
		}
	}

	return nil
}

// AddVolume accepts a volume, converts it to its persistent form, and writes it to the database.
// Note that if a record already exists for this volume, it is replaced.  Therefore, this method
// should be called after creating a new volume with the expectation that the result is a record
// that matches what was just created.  It is the caller's responsibility to ensure that replacement
// is appropriate.
func (k *CRDClientV1) AddVolume(ctx context.Context, volume *storage.Volume) (err error) {
	ctx = NewContextBuilder(ctx).WithLayer(LogLayerPersistentStore).BuildContext()

	persistentVolume, err := v1.NewTridentVolume(ctx, volume.ConstructExternal())
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"volume":                       volume,
		"volume.BackendUUID":           volume.BackendUUID,
		"persistentVolume":             persistentVolume,
		"persistentVolume.BackendUUID": persistentVolume.BackendUUID,
	}).Debug("AddVolume")

	_, err = k.crdClient.TridentV1().TridentVolumes(k.namespace).Create(ctx, persistentVolume, createOpts)
	if err == nil || !k8sapierrors.IsAlreadyExists(err) {
		return err
	}

	// If the Tvol already exists, it means we failed to create it previously and have now repeated the volume
	// creation workflow, in which case we need to remove finalizers, delete, and recreate the Tvol.

	tvol, getErr := k.crdClient.TridentV1().TridentVolumes(k.namespace).Get(ctx, persistentVolume.Name, getOpts)
	if getErr != nil {
		if !k8sapierrors.IsNotFound(getErr) {
			return getErr
		}
	} else {
		tvol = tvol.DeepCopy()
		tvol.RemoveTridentFinalizers()
		_, updateErr := k.crdClient.TridentV1().TridentVolumes(k.namespace).Update(ctx, tvol, updateOpts)
		if updateErr != nil {
			Logc(ctx).Errorf("Could not remove volume finalizers; %v", updateErr)
			return updateErr
		}

		deleteErr := k.crdClient.TridentV1().TridentVolumes(k.namespace).Delete(ctx, tvol.Name, k.deleteOpts())
		if deleteErr != nil {
			Logc(ctx).Errorf("Could not delete volume; %v", deleteErr)
			return deleteErr
		}
	}

	_, err = k.crdClient.TridentV1().TridentVolumes(k.namespace).Create(ctx, persistentVolume, createOpts)
	return err
}

func (k *CRDClientV1) HasVolumes(ctx context.Context) (hasVols bool, err error) {
	ctx = NewContextBuilder(context.WithoutCancel(ctx)).WithLayer(LogLayerPersistentStore).BuildContext()

	listOneOpts := metav1.ListOptions{Limit: 1}
	volumeList, err := k.crdClient.TridentV1().TridentVolumes(k.namespace).List(ctx, listOneOpts)
	if err != nil {
		return false, err
	}

	return len(volumeList.Items) > 0, nil
}

func (k *CRDClientV1) GetVolume(
	ctx context.Context, volName string,
) (persistentVolume *storage.VolumeExternal, err error) {
	ctx = NewContextBuilder(ctx).WithLayer(LogLayerPersistentStore).BuildContext()

	volume, err := k.crdClient.TridentV1().TridentVolumes(k.namespace).Get(ctx, v1.NameFix(volName), getOpts)
	if err != nil {
		return nil, err
	}

	persistentVolume, err = volume.Persistent()
	if err != nil {
		return nil, err
	}

	return persistentVolume, nil
}

func (k *CRDClientV1) UpdateVolume(ctx context.Context, update *storage.Volume) (err error) {
	ctx = NewContextBuilder(context.WithoutCancel(ctx)).WithLayer(LogLayerPersistentStore).BuildContext()

	volume, err := k.crdClient.TridentV1().TridentVolumes(k.namespace).Get(ctx, v1.NameFix(update.Config.Name), getOpts)
	if err != nil {
		return err
	}

	if err = volume.Apply(ctx, update.ConstructExternal()); err != nil {
		return err
	}

	_, err = k.crdClient.TridentV1().TridentVolumes(k.namespace).Update(ctx, volume, updateOpts)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) DeleteVolume(ctx context.Context, volume *storage.Volume) (err error) {
	ctx = NewContextBuilder(ctx).WithLayer(LogLayerPersistentStore).BuildContext()

	err = k.crdClient.TridentV1().TridentVolumes(k.namespace).Delete(ctx, v1.NameFix(volume.Config.Name),
		k.deleteOpts())

	if k8sapierrors.IsNotFound(err) {
		Logc(ctx).WithField("volume", volume).Debug("Volume already deleted.")
		return nil
	}

	return err
}

func (k *CRDClientV1) GetVolumes(ctx context.Context) (vols []*storage.VolumeExternal, err error) {
	ctx = NewContextBuilder(context.WithoutCancel(ctx)).WithLayer(LogLayerPersistentStore).BuildContext()

	volumeList, err := k.crdClient.TridentV1().TridentVolumes(k.namespace).List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	vols = make([]*storage.VolumeExternal, 0)
	for _, item := range volumeList.Items {
		if !item.ObjectMeta.DeletionTimestamp.IsZero() {
			Logc(ctx).WithFields(LogFields{
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

		vols = append(vols, persistentVolume)
	}

	return vols, nil
}

func (k *CRDClientV1) DeleteVolumes(ctx context.Context) error {
	volumeList, err := k.crdClient.TridentV1().TridentVolumes(k.namespace).List(ctx, listOpts)
	if err != nil {
		return err
	}

	for _, item := range volumeList.Items {
		err := k.crdClient.TridentV1().TridentVolumes(k.namespace).Delete(ctx, item.ObjectMeta.Name, k.deleteOpts())
		if err != nil {
			return err
		}
	}

	return nil
}

func (k *CRDClientV1) AddVolumeTransaction(ctx context.Context, txn *storage.VolumeTransaction) (err error) {
	ctx = NewContextBuilder(context.WithoutCancel(ctx)).WithLayer(LogLayerPersistentStore).BuildContext()

	newTxn, err := v1.NewTridentTransaction(txn)
	if err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"op":   txn.Op,
		"name": v1.NameFix(txn.Name()),
	}).Debug("AddVolumeTransaction")

	_, err = k.crdClient.TridentV1().TridentTransactions(k.namespace).Create(ctx, newTxn, createOpts)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) HasVolumeTransactions(ctx context.Context) (hasTxns bool, err error) {
	ctx = NewContextBuilder(context.WithoutCancel(ctx)).WithLayer(LogLayerPersistentStore).BuildContext()

	listOneOpts := metav1.ListOptions{Limit: 1}
	txnList, err := k.crdClient.TridentV1().TridentTransactions(k.namespace).List(ctx, listOneOpts)
	if err != nil {
		return false, err
	}

	return len(txnList.Items) > 0, nil
}

func (k *CRDClientV1) GetVolumeTransactions(ctx context.Context) (txns []*storage.VolumeTransaction, err error) {
	ctx = NewContextBuilder(context.WithoutCancel(ctx)).WithLayer(LogLayerPersistentStore).BuildContext()

	txnList, err := k.crdClient.TridentV1().TridentTransactions(k.namespace).List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	txns = make([]*storage.VolumeTransaction, 0)
	for _, item := range txnList.Items {
		if !item.ObjectMeta.DeletionTimestamp.IsZero() {
			Logc(ctx).WithFields(LogFields{
				"Name":              item.Name,
				"DeletionTimestamp": item.DeletionTimestamp,
			}).Debug("GetVolumeTransactions skipping deleted VolumeTransaction")
			continue
		}

		if tTxn, err := item.Persistent(); err != nil {
			return nil, err
		} else {
			txns = append(txns, tTxn)
		}
	}

	return txns, nil
}

func (k *CRDClientV1) UpdateVolumeTransaction(ctx context.Context, update *storage.VolumeTransaction) (err error) {
	ctx = NewContextBuilder(context.WithoutCancel(ctx)).WithLayer(LogLayerPersistentStore).BuildContext()

	ttxn, err := k.crdClient.TridentV1().TridentTransactions(k.namespace).Get(ctx, v1.NameFix(update.Name()), getOpts)
	if err != nil {
		return err
	}

	if err = ttxn.Apply(update); err != nil {
		return err
	}

	_, err = k.crdClient.TridentV1().TridentTransactions(k.namespace).Update(ctx, ttxn, updateOpts)
	return err
}

func (k *CRDClientV1) GetVolumeTransaction(
	ctx context.Context, volTxn *storage.VolumeTransaction,
) (pTxn *storage.VolumeTransaction, err error) {
	ctx = NewContextBuilder(context.WithoutCancel(ctx)).WithLayer(LogLayerPersistentStore).BuildContext()

	ttxn, err := k.crdClient.TridentV1().TridentTransactions(k.namespace).Get(ctx, v1.NameFix(volTxn.Name()), getOpts)

	if k8sapierrors.IsNotFound(err) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("error getting volumeTransaction from CRD; %v", err)
	}

	if !ttxn.ObjectMeta.DeletionTimestamp.IsZero() {
		Logc(ctx).WithFields(LogFields{
			"Name":              ttxn.Name,
			"DeletionTimestamp": ttxn.DeletionTimestamp,
		}).Debug("GetVolumeTransaction skipping deleted VolumeTransaction")
		return nil, err
	}

	if pTxn, err = ttxn.Persistent(); err != nil {
		return nil, fmt.Errorf("error parsing volumeTransaction; %v", err)
	}

	return pTxn, nil
}

func (k *CRDClientV1) DeleteVolumeTransaction(ctx context.Context, volTxn *storage.VolumeTransaction) (err error) {
	ctx = NewContextBuilder(context.WithoutCancel(ctx)).WithLayer(LogLayerPersistentStore).BuildContext()

	err = k.crdClient.TridentV1().TridentTransactions(k.namespace).Delete(ctx, v1.NameFix(volTxn.Name()),
		k.deleteOpts())

	if k8sapierrors.IsNotFound(err) {
		return nil
	}

	return err
}

func (k *CRDClientV1) AddStorageClass(ctx context.Context, sc *storageclass.StorageClass) error {
	persistentSC, err := v1.NewTridentStorageClass(sc.ConstructPersistent())
	if err != nil {
		return err
	}

	_, err = k.crdClient.TridentV1().TridentStorageClasses(k.namespace).Create(ctx, persistentSC, createOpts)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) UpdateStorageClass(ctx context.Context, update *storageclass.StorageClass) error {
	scName := update.GetName()

	sc, err := k.crdClient.TridentV1().TridentStorageClasses(k.namespace).Get(ctx, v1.NameFix(scName), getOpts)
	if err != nil {
		return err
	}

	if err = sc.Apply(update.ConstructPersistent()); err != nil {
		return err
	}

	if _, err = k.crdClient.TridentV1().TridentStorageClasses(k.namespace).Update(ctx, sc, updateOpts); err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) HasStorageClasses(ctx context.Context) (bool, error) {
	listOneOpts := metav1.ListOptions{Limit: 1}
	scList, err := k.crdClient.TridentV1().TridentStorageClasses(k.namespace).List(ctx, listOneOpts)
	if err != nil {
		return false, err
	}

	return len(scList.Items) > 0, nil
}

func (k *CRDClientV1) GetStorageClass(ctx context.Context, scName string) (*storageclass.Persistent, error) {
	sc, err := k.crdClient.TridentV1().TridentStorageClasses(k.namespace).Get(ctx, v1.NameFix(scName), getOpts)
	if err != nil {
		return nil, err
	}

	persistentSC, err := sc.Persistent()
	if err != nil {
		return nil, err
	}

	return persistentSC, nil
}

func (k *CRDClientV1) GetStorageClasses(ctx context.Context) ([]*storageclass.Persistent, error) {
	scList, err := k.crdClient.TridentV1().TridentStorageClasses(k.namespace).List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	results := make([]*storageclass.Persistent, 0)

	for _, item := range scList.Items {
		if !item.ObjectMeta.DeletionTimestamp.IsZero() {
			Logc(ctx).WithFields(LogFields{
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

func (k *CRDClientV1) DeleteStorageClass(ctx context.Context, sc *storageclass.StorageClass) error {
	err := k.crdClient.TridentV1().TridentStorageClasses(k.namespace).Delete(ctx, v1.NameFix(sc.GetName()),
		k.deleteOpts())

	if k8sapierrors.IsNotFound(err) {
		return nil
	}

	return err
}

func (k *CRDClientV1) AddOrUpdateNode(ctx context.Context, node *models.Node) error {
	// look to see if it's an existing one we need to update
	existingNode, err := k.crdClient.TridentV1().TridentNodes(k.namespace).Get(ctx, v1.NameFix(node.Name), getOpts)
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
		_, err = k.crdClient.TridentV1().TridentNodes(k.namespace).Update(ctx, existingNode, updateOpts)
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

	_, err = k.crdClient.TridentV1().TridentNodes(k.namespace).Create(ctx, newNode, createOpts)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) GetNode(ctx context.Context, nName string) (*models.Node, error) {
	node, err := k.crdClient.TridentV1().TridentNodes(k.namespace).Get(ctx, v1.NameFix(nName), getOpts)
	if err != nil {
		return nil, err
	}

	persistentNode, err := node.Persistent()
	if err != nil {
		return nil, err
	}

	return persistentNode, nil
}

func (k *CRDClientV1) GetNodes(ctx context.Context) ([]*models.Node, error) {
	nodeList, err := k.crdClient.TridentV1().TridentNodes(k.namespace).List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	results := make([]*models.Node, 0)

	for _, item := range nodeList.Items {
		if !item.ObjectMeta.DeletionTimestamp.IsZero() {
			Logc(ctx).WithFields(LogFields{
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

func (k *CRDClientV1) DeleteNode(ctx context.Context, n *models.Node) error {
	err := k.crdClient.TridentV1().TridentNodes(k.namespace).Delete(ctx, v1.NameFix(n.Name), k.deleteOpts())

	if k8sapierrors.IsNotFound(err) {
		return nil
	}

	return err
}

func (k *CRDClientV1) AddVolumePublication(ctx context.Context, publication *models.VolumePublication) error {
	newPublication, err := v1.NewTridentVolumePublication(publication)
	if err != nil {
		return err
	}

	_, err = k.crdClient.TridentV1().TridentVolumePublications(k.namespace).Create(ctx, newPublication, createOpts)
	if err != nil {
		if k8sapierrors.IsAlreadyExists(err) {
			return NewAlreadyExistsError(newPublication.Kind, newPublication.Name)
		}
		return err
	}

	return nil
}

func (k *CRDClientV1) UpdateVolumePublication(ctx context.Context, publication *models.VolumePublication) error {
	existingPublication, err := k.crdClient.TridentV1().TridentVolumePublications(k.namespace).Get(ctx,
		v1.NameFix(publication.Name), getOpts)
	if err != nil {
		return err
	}

	if err = existingPublication.Apply(publication); err != nil {
		return err
	}
	_, err = k.crdClient.TridentV1().TridentVolumePublications(k.namespace).Update(ctx, existingPublication, updateOpts)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) GetVolumePublication(ctx context.Context, nName string) (*models.VolumePublication, error) {
	publication, err := k.crdClient.TridentV1().TridentVolumePublications(k.namespace).Get(ctx, v1.NameFix(nName),
		getOpts)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			return nil, errors.NotFoundError(err.Error())
		}
		return nil, err
	}

	persistentPublication, err := publication.Persistent()
	if err != nil {
		return nil, err
	}

	return persistentPublication, nil
}

func (k *CRDClientV1) GetVolumePublications(ctx context.Context) ([]*models.VolumePublication, error) {
	publicationList, err := k.crdClient.TridentV1().TridentVolumePublications(k.namespace).List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	results := make([]*models.VolumePublication, 0)

	for _, item := range publicationList.Items {
		if !item.ObjectMeta.DeletionTimestamp.IsZero() {
			Logc(ctx).WithFields(LogFields{
				"Name":              item.Name,
				"DeletionTimestamp": item.DeletionTimestamp,
			}).Debug("GetVolumePublications skipping deleted VolumePublication")
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

func (k *CRDClientV1) DeleteVolumePublication(ctx context.Context, vp *models.VolumePublication) error {
	err := k.crdClient.TridentV1().TridentVolumePublications(k.namespace).Delete(ctx, v1.NameFix(vp.Name),
		k.deleteOpts())

	if k8sapierrors.IsNotFound(err) {
		return errors.NotFoundError(err.Error())
	}

	return err
}

// deleteOpts returns a DeleteOptions struct suitable for most DELETE calls to the K8S REST API.
func (k *CRDClientV1) deleteOpts() metav1.DeleteOptions {
	propagationPolicy := metav1.DeletePropagationBackground
	return metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}
}

// AddSnapshot accepts a snapshot, converts it to its persistent form, and writes it to the database.
// Note that if a record already exists for this snapshot, it is replaced.  Therefore, this method
// should be called after creating a new snapshot with the expectation that the result is a record
// that matches what was just created.  It is the caller's responsibility to ensure that replacement
// is appropriate.
func (k *CRDClientV1) AddSnapshot(ctx context.Context, snapshot *storage.Snapshot) error {
	persistentSnapshot, err := v1.NewTridentSnapshot(snapshot.ConstructPersistent())
	if err != nil {
		return err
	}

	_, err = k.crdClient.TridentV1().TridentSnapshots(k.namespace).Create(ctx, persistentSnapshot, createOpts)
	if err == nil || !k8sapierrors.IsAlreadyExists(err) {
		return err
	}

	// If the Tsnap already exists, it means we failed to create it previously and have now repeated the snapshot
	// creation workflow, in which case we need to delete & recreate the Tsnap.

	tsnap, getErr := k.crdClient.TridentV1().TridentSnapshots(k.namespace).Get(ctx, persistentSnapshot.Name, getOpts)
	if getErr != nil {
		if !k8sapierrors.IsNotFound(getErr) {
			return getErr
		}
	} else {
		tsnap = tsnap.DeepCopy()
		tsnap.RemoveTridentFinalizers()
		_, updateErr := k.crdClient.TridentV1().TridentSnapshots(k.namespace).Update(ctx, tsnap, updateOpts)
		if updateErr != nil {
			Logc(ctx).Errorf("Could not remove snapshot finalizers; %v", updateErr)
			return updateErr
		}

		deleteErr := k.crdClient.TridentV1().TridentSnapshots(k.namespace).Delete(ctx, tsnap.Name, k.deleteOpts())
		if deleteErr != nil {
			Logc(ctx).Errorf("Could not delete snapshot; %v", deleteErr)
			return deleteErr
		}
	}

	_, err = k.crdClient.TridentV1().TridentSnapshots(k.namespace).Create(ctx, persistentSnapshot, createOpts)
	return err
}

func (k *CRDClientV1) GetSnapshot(ctx context.Context, volumeName, snapshotName string) (
	*storage.SnapshotPersistent, error,
) {
	snapshotID := storage.MakeSnapshotID(volumeName, snapshotName)
	snapshot, err := k.crdClient.TridentV1().TridentSnapshots(k.namespace).Get(ctx, v1.NameFix(snapshotID), getOpts)
	if err != nil {
		return nil, err
	}

	persistentSnapshot, err := snapshot.Persistent()
	if err != nil {
		return nil, err
	}

	return persistentSnapshot, nil
}

func (k *CRDClientV1) GetSnapshots(ctx context.Context) ([]*storage.SnapshotPersistent, error) {
	snapshotList, err := k.crdClient.TridentV1().TridentSnapshots(k.namespace).List(ctx, listOpts)
	if err != nil {
		return nil, err
	}

	results := make([]*storage.SnapshotPersistent, 0)

	for _, item := range snapshotList.Items {
		if !item.ObjectMeta.DeletionTimestamp.IsZero() {
			Logc(ctx).WithFields(LogFields{
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

func (k *CRDClientV1) UpdateSnapshot(ctx context.Context, update *storage.Snapshot) error {
	snapshot, err := k.crdClient.TridentV1().TridentSnapshots(k.namespace).Get(ctx, v1.NameFix(update.ID()), getOpts)
	if err != nil {
		return err
	}

	if err = snapshot.Apply(update.ConstructPersistent()); err != nil {
		return err
	}

	_, err = k.crdClient.TridentV1().TridentSnapshots(k.namespace).Update(ctx, snapshot, updateOpts)
	if err != nil {
		return err
	}

	return nil
}

func (k *CRDClientV1) DeleteSnapshot(ctx context.Context, snapshot *storage.Snapshot) error {
	err := k.crdClient.TridentV1().TridentSnapshots(k.namespace).Delete(ctx, v1.NameFix(snapshot.ID()), k.deleteOpts())

	if k8sapierrors.IsNotFound(err) {
		return nil
	}

	return err
}

func (k *CRDClientV1) DeleteSnapshots(ctx context.Context) error {
	snapshotList, err := k.crdClient.TridentV1().TridentSnapshots(k.namespace).List(ctx, listOpts)
	if err != nil {
		return err
	}

	for _, item := range snapshotList.Items {
		err := k.crdClient.TridentV1().TridentSnapshots(k.namespace).Delete(ctx, item.ObjectMeta.Name, k.deleteOpts())
		if err != nil && !k8sapierrors.IsAlreadyExists(err) {
			return err
		}
	}

	return nil
}

func (k *CRDClientV1) AddGroupSnapshot(ctx context.Context, groupSnapshot *storage.GroupSnapshot) error {
	persistedGroupSnapshot, err := v1.NewTridentGroupSnapshot(groupSnapshot.ConstructPersistent())
	if err != nil {
		return err
	}

	_, err = k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).Create(ctx, persistedGroupSnapshot, createOpts)
	if err != nil || !k8sapierrors.IsAlreadyExists(err) {
		return err
	}

	// There could be some case where a group snapshot already exists, in which case we need to remove finalizers,
	tgsnap, getErr := k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).Get(ctx, persistedGroupSnapshot.Name, getOpts)
	if getErr != nil {
		if !k8sapierrors.IsNotFound(getErr) {
			return getErr
		}
	} else {
		tgsnap = tgsnap.DeepCopy()
		tgsnap.RemoveTridentFinalizers()
		_, updateErr := k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).Update(ctx, tgsnap, updateOpts)
		if updateErr != nil {
			Logc(ctx).Errorf("Could not remove snapshot finalizers; %v", updateErr)
			return updateErr
		}

		deleteErr := k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).Delete(ctx, tgsnap.Name, k.deleteOpts())
		if deleteErr != nil {
			Logc(ctx).Errorf("Could not delete snapshot; %v", deleteErr)
			return deleteErr
		}
	}

	_, err = k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).Create(ctx, persistedGroupSnapshot, createOpts)
	return err
}

func (k *CRDClientV1) GetGroupSnapshot(ctx context.Context, groupSnapshotName string) (*storage.GroupSnapshotPersistent, error) {
	tgsnaps, err := k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).Get(ctx, v1.NameFix(groupSnapshotName), getOpts)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			return nil, errors.WrapWithNotFoundError(err, "group snapshot %s not found", groupSnapshotName)
		}
		return nil, err
	}

	return tgsnaps.Persistent()
}

func (k *CRDClientV1) GetGroupSnapshots(ctx context.Context) ([]*storage.GroupSnapshotPersistent, error) {
	tgsnaps, err := k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).List(ctx, listOpts)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			return nil, errors.WrapWithNotFoundError(err, "group snapshots not found")
		}
		return nil, err
	}

	return tgsnaps.Persistent()
}

func (k *CRDClientV1) UpdateGroupSnapshot(ctx context.Context, groupSnapshot *storage.GroupSnapshot) error {
	tgsnap, err := k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).Get(ctx, v1.NameFix(groupSnapshot.ID()), getOpts)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			return errors.WrapWithNotFoundError(err, "group snapshot %s not found", groupSnapshot.ID())
		}
		return err
	}

	if err := tgsnap.Apply(groupSnapshot.ConstructPersistent()); err != nil {
		return err
	}

	_, err = k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).Update(ctx, tgsnap, updateOpts)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			return errors.WrapWithNotFoundError(err, "group snapshot %s not found during update", groupSnapshot.ID())
		}
		return err
	}

	return nil
}

func (k *CRDClientV1) DeleteGroupSnapshot(ctx context.Context, groupSnapshot *storage.GroupSnapshot) error {
	err := k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).Delete(ctx, v1.NameFix(groupSnapshot.ID()), k.deleteOpts())
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			return errors.WrapWithNotFoundError(err, "group snapshot %s not found", groupSnapshot.ID())
		}
		return err
	}

	// Check if the group snapshot still exists. If it does, it probably means that finalizers kept it from deleting.
	tgsnap, getErr := k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).Get(ctx, v1.NameFix(groupSnapshot.ID()), getOpts)
	if getErr != nil && !k8sapierrors.IsNotFound(getErr) {
		return getErr
	}

	// Try removing the finalizers.
	tgsnap = tgsnap.DeepCopy()
	tgsnap.RemoveTridentFinalizers()
	_, updateErr := k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).Update(ctx, tgsnap, updateOpts)
	if updateErr != nil {
		Logc(ctx).Errorf("Could not remove snapshot finalizers; %v", updateErr)
		return updateErr
	}

	// Try deleting the group snapshot again. If it fails for any other reason other than not found, return the error.
	deleteErr := k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).Delete(ctx, tgsnap.Name, k.deleteOpts())
	if deleteErr != nil && !k8sapierrors.IsNotFound(deleteErr) {
		Logc(ctx).Errorf("Failed to delete group snapshot; %v", deleteErr)
		return deleteErr
	}

	return nil
}

func (k *CRDClientV1) DeleteGroupSnapshots(ctx context.Context) error {
	tgsnapList, err := k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).List(ctx, listOpts)
	if err != nil {
		if k8sapierrors.IsNotFound(err) {
			return errors.WrapWithNotFoundError(err, "group snapshots not found")
		}
		return err
	}

	for _, item := range tgsnapList.Items {
		err := k.crdClient.TridentV1().TridentGroupSnapshots(k.namespace).Delete(ctx, item.ObjectMeta.Name, k.deleteOpts())
		if err != nil && !k8sapierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}
