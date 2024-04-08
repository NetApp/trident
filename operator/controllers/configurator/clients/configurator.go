// Copyright 2024 NetApp, Inc. All Rights Reserved.

package clients

import (
	"encoding/json"
	"fmt"
	"strings"

	jsonPatch "github.com/evanphx/json-patch/v5"
	"github.com/ghodss/yaml"
	"k8s.io/apimachinery/pkg/types"

	k8sClient "github.com/netapp/trident/cli/k8s_client"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/operator/clients"
	operatorV1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	tridentV1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
)

const (
	ANFClientID     = "ClientID"
	ANFClientSecret = "ClientSecret"
)

type ConfiguratorClient struct {
	kClient ExtendedK8sClientInterface
	tClient clients.TridentCRDClientInterface
	sClient clients.SnapshotCRDClientInterface
	oClient clients.OperatorCRDClientInterface
}

func NewConfiguratorClient(
	kClient ExtendedK8sClientInterface, tClient clients.TridentCRDClientInterface,
	sClient clients.SnapshotCRDClientInterface, oClient clients.OperatorCRDClientInterface,
) ConfiguratorClientInterface {
	return &ConfiguratorClient{kClient: kClient, tClient: tClient, sClient: sClient, oClient: oClient}
}

func (c *ConfiguratorClient) CreateOrPatchObject(objType ObjectType, objName, objNamespace, objYAML string) error {
	objExists, err := c.checkObjectExists(objType, objName, objNamespace)
	if err != nil {
		return err
	}

	if !objExists {
		Log().Debugf("Creating %s %s.", objName, objType)
		return c.kClient.CreateObjectByYAML(objYAML)
	}

	obj, err := c.getObject(objType, objName, objNamespace)
	if err != nil {
		return err
	}

	patchBytes, err := c.getPatch(objType, obj, objYAML)
	if err != nil {
		return err
	}

	if err := c.patchObject(objType, objName, objNamespace, patchBytes); err == nil {
		Log().Debugf("Patched %s %s", objName, objType)
		return nil
	} else {
		Log().Errorf("Patch failed for %s %s: %v", objName, objType, err)
	}

	if err := c.deleteObject(objType, objName, objNamespace); err != nil {
		return err
	}

	Log().Debugf("Creating %s %s after deleting it.", objName, objType)
	return c.kClient.CreateObjectByYAML(objYAML)
}

func (c *ConfiguratorClient) UpdateTridentConfiguratorStatus(
	tconfCR *operatorV1.TridentConfigurator, newStatus operatorV1.TridentConfiguratorStatus,
) (*operatorV1.TridentConfigurator, bool, error) {
	return c.oClient.UpdateTridentConfiguratorStatus(tconfCR, newStatus)
}

func (c *ConfiguratorClient) checkObjectExists(objType ObjectType, objName, objNamespace string) (bool, error) {
	switch objType {
	case OCRD:
		return c.kClient.CheckCRDExists(objName)
	case OBackend:
		return c.tClient.CheckTridentBackendConfigExists(objName, objNamespace)
	case OStorageClass:
		return c.kClient.CheckStorageClassExists(objName)
	case OSnapshotClass:
		return c.sClient.CheckVolumeSnapshotClassExists(objName)
	}

	return false, fmt.Errorf("unsupported object %s of type %s", objName, objType)
}

func (c *ConfiguratorClient) getObject(objType ObjectType, objName, objNamespace string) (interface{}, error) {
	switch objType {
	case OCRD:
		return c.kClient.GetCRD(objName)
	case OBackend:
		return c.tClient.GetTridentBackendConfig(objName, objNamespace)
	case OStorageClass:
		return c.kClient.GetStorageClass(objName)
	case OSnapshotClass:
		return c.sClient.GetVolumeSnapshotClass(objName)
	}

	return nil, fmt.Errorf("unsupported object %s of type %s", objName, objType)
}

func (c *ConfiguratorClient) getPatch(objType ObjectType, obj interface{}, newObjYAML string) ([]byte, error) {
	// For TBC spec, few fields may get added/removed with 'helm upgrade'. This needs special handling at TBC.Spec level.
	if objType == OBackend {
		emptyPatch := []byte("{}")
		oldObj, ok := obj.(*tridentV1.TridentBackendConfig)
		if ok {
			oldSpecBytes, err := json.Marshal(oldObj.Spec)
			if err != nil {
				return emptyPatch, err
			}

			newObj := tridentV1.TridentBackendConfig{}
			if err := yaml.Unmarshal([]byte(newObjYAML), &newObj); err != nil {
				return emptyPatch, err
			}
			newSpecBytes, err := json.Marshal(newObj.Spec)
			if err != nil {
				return emptyPatch, err
			}

			// Getting the Spec difference between old TBC and new TBC.
			patch, err := jsonPatch.CreateMergePatch(oldSpecBytes, newSpecBytes)
			if err != nil {
				return emptyPatch, err
			}

			if len(patch) > 2 {
				newPatch := "{\"spec\": " + string(patch) + "}"
				return []byte(newPatch), nil
			} else {
				return emptyPatch, nil
			}
		} else {
			return emptyPatch, fmt.Errorf("wrong object type in getPatch")
		}
	}

	// We can use k8sClient.GenericPatch for other objects as it combines the old and new objects into one. And
	// currently, these objects have almost static fields.
	return k8sClient.GenericPatch(obj, []byte(newObjYAML))
}

func (c *ConfiguratorClient) patchObject(objType ObjectType, objName, objNamespace string, patchBytes []byte) error {
	switch objType {
	case OCRD:
		return c.kClient.PatchCRD(objName, patchBytes, types.MergePatchType)
	case OBackend:
		return c.tClient.PatchTridentBackendConfig(objName, objNamespace, patchBytes, types.MergePatchType)
	case OStorageClass:
		return c.kClient.PatchStorageClass(objName, patchBytes, types.StrategicMergePatchType)
	case OSnapshotClass:
		return c.sClient.PatchVolumeSnapshotClass(objName, patchBytes, types.MergePatchType)
	}

	return fmt.Errorf("unsupported object %s of type %s", objName, objType)
}

func (c *ConfiguratorClient) deleteObject(objType ObjectType, objName, objNamespace string) error {
	switch objType {
	case OCRD:
		return c.kClient.DeleteCRD(objName)
	case OBackend:
		return c.tClient.DeleteTridentBackendConfig(objName, objNamespace)
	case OStorageClass:
		return c.kClient.DeleteStorageClass(objName)
	case OSnapshotClass:
		return c.sClient.DeleteVolumeSnapshotClass(objName)
	}

	return fmt.Errorf("unsupported object %s of type %s", objName, objType)
}

func (c *ConfiguratorClient) GetControllingTorcCR() (*operatorV1.TridentOrchestrator, error) {
	return c.oClient.GetControllingTorcCR()
}

func (c *ConfiguratorClient) GetTconfCR(name string) (*operatorV1.TridentConfigurator, error) {
	return c.oClient.GetTconfCR(name)
}

func (c *ConfiguratorClient) GetANFSecrets(secretName string) (string, string, error) {
	secret, err := c.kClient.GetSecret(secretName)
	if err != nil {
		return "", "", err
	}

	secretMap := make(map[string]string)
	for key, value := range secret.Data {
		secretMap[strings.ToLower(key)] = string(value)
	}
	for key, value := range secret.StringData {
		secretMap[strings.ToLower(key)] = value
	}

	clientID, ok := secretMap[strings.ToLower(ANFClientID)]
	if !ok {
		return "", "", fmt.Errorf("client id not present in secret")
	}

	clientSecret, ok := secretMap[strings.ToLower(ANFClientSecret)]
	if !ok {
		return "", "", fmt.Errorf("client secret not present in secret")
	}

	return clientID, clientSecret, nil
}
