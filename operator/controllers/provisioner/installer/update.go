// Copyright 2020 NetApp, Inc. All Rights Reserved.

package installer

import (
	"encoding/json"
	"fmt"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/ghodss/yaml"
	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/api/policy/v1beta1"
	v12 "k8s.io/api/rbac/v1"
	v1beta12 "k8s.io/api/storage/v1beta1"
)

func (i *Installer) patchK8sCSIDriver(currentK8sCSIDriver *v1beta12.CSIDriver, newK8sCSIDriverYAML []byte) error {

	// Identify the deltas
	patchBytes, err := i.genericPatch(currentK8sCSIDriver, newK8sCSIDriverYAML, &v1beta12.CSIDriver{})
	if err != nil {
		return fmt.Errorf("error in creating the two-way merge patch for current CSI driver %q: %v",
			currentK8sCSIDriver.Name, err)
	}

	// Apply the patch to the current CSI driver
	err = i.client.PatchCSIDriverByLabel(appLabel, patchBytes)
	if err != nil {
		return fmt.Errorf("could not patch Trident CSI driver; %v", err)
	}
	log.Debug("Patched Trident CSI driver.")

	return nil
}

func (i *Installer) patchTridentServiceAccount(currentServiceAccount *v1.ServiceAccount,
	newServiceAccountYAML []byte) error {

	// Identify the deltas
	patchBytes, err := i.genericPatch(currentServiceAccount, newServiceAccountYAML, &v1.ServiceAccount{})
	if err != nil {
		return fmt.Errorf("error in creating the two-way merge patch for current Service account %q: %v",
			currentServiceAccount.Name, err)
	}

	// Apply the patch to the current Service Account
	err = i.client.PatchServiceAccountByLabel(appLabel, patchBytes)
	if err != nil {
		return fmt.Errorf("could not patch Trident Service account; %v", err)
	}
	log.Debug("Patched Trident service account.")

	return nil
}

func (i *Installer) patchTridentClusterRole(currentClusterRole *v12.ClusterRole,
	newClusterRoleYAML []byte) error {

	// Identify the deltas
	patchBytes, err := i.genericPatch(currentClusterRole, newClusterRoleYAML, &v12.ClusterRole{})
	if err != nil {
		return fmt.Errorf("error in creating the two-way merge patch for current cluster role %q: %v",
			currentClusterRole.Name, err)
	}

	// Apply the patch to the current Cluster Role
	err = i.client.PatchClusterRoleByLabel(appLabel, patchBytes)
	if err != nil {
		return fmt.Errorf("could not patch Trident Cluster role; %v", err)
	}
	log.Debug("Patched Trident Cluster role.")

	return nil
}

func (i *Installer) patchTridentClusterRoleBinding(currentClusterRoleBinding *v12.ClusterRoleBinding,
	newClusterRoleBindingYAML []byte) error {

	// Identify the deltas
	patchBytes, err := i.genericPatch(currentClusterRoleBinding, newClusterRoleBindingYAML, &v12.ClusterRoleBinding{})
	if err != nil {
		return fmt.Errorf("error in creating the two-way merge patch for current cluster role binding %q: %v",
			currentClusterRoleBinding.Name, err)
	}

	// Apply the patch to the current Cluster Role Binding
	err = i.client.PatchClusterRoleBindingByLabel(appLabel, patchBytes)
	if err != nil {
		return fmt.Errorf("could not patch Trident Cluster role binding; %v", err)
	}
	log.Debug("Patched Trident Cluster role binding.")

	return nil
}

func (i *Installer) patchTridentPodSecurityPolicy(currentPSP *v1beta1.PodSecurityPolicy, newPSPYAML []byte) error {

	// Identify the deltas
	patchBytes, err := i.genericPatch(currentPSP, newPSPYAML, &v1beta1.PodSecurityPolicy{})
	if err != nil {
		return fmt.Errorf("error in creating the two-way merge patch for current Pod security policy %q: %v",
			currentPSP.Name, err)
	}

	// Apply the patch to the current Pod Security Policy
	err = i.client.PatchPodSecurityPolicyByLabel(appLabel, patchBytes)
	if err != nil {
		return fmt.Errorf("could not patch Trident Pod security policy; %v", err)
	}
	log.Debug("Patched Trident Pod security policy.")

	return nil
}

func (i *Installer) patchTridentService(currentService *v1.Service, newServiceYAML []byte) error {

	// Identify the deltas
	patchBytes, err := i.genericPatch(currentService, newServiceYAML, &v1.Service{})
	if err != nil {
		return fmt.Errorf("error in creating the two-way merge patch for current Service %q: %v",
			currentService.Name, err)
	}

	// Apply the patch to the current Service
	err = i.client.PatchServiceByLabel(appLabel, patchBytes)
	if err != nil {
		return fmt.Errorf("could not patch Trident Service; %v", err)
	}
	log.Debug("Patched Trident Service.")

	return nil
}

func (i *Installer) patchTridentSecret(currentSecret *v1.Secret, newSecretYAML []byte) error {

	// Identify the deltas
	patchBytes, err := i.genericPatch(currentSecret, newSecretYAML, &v1.Secret{})
	if err != nil {
		return fmt.Errorf("error in creating the two-way merge patch for current Secret %q: %v",
			currentSecret.Name, err)
	}

	// Apply the patch to the current Secret
	err = i.client.PatchSecretByLabel(appLabel, patchBytes)
	if err != nil {
		return fmt.Errorf("could not patch Trident Secret; %v", err)
	}
	log.Debug("Patched Trident Secret.")

	return nil
}

func (i *Installer) patchTridentDeployment(currentDeployment *appsv1.Deployment, newDeploymentYAML []byte) error {

	// Identify the deltas
	patchBytes, err := i.genericPatch(currentDeployment, newDeploymentYAML, &appsv1.Deployment{})
	if err != nil {
		return fmt.Errorf("error in creating the two-way merge patch for current Deployment %q: %v",
			currentDeployment.Name, err)
	}

	// Apply the patch to the current deployment
	err = i.client.PatchDeploymentByLabel(appLabel, patchBytes)
	if err != nil {
		return fmt.Errorf("could not patch Trident deployment; %v", err)
	}
	log.Debug("Patched Trident deployment.")

	return nil
}

func (i *Installer) patchTridentDaemonSet(currentDaemonSet *appsv1.DaemonSet, newDaemonSetYAML []byte) error {

	// Identify the deltas
	patchBytes, err := i.genericPatch(currentDaemonSet, newDaemonSetYAML, &appsv1.DaemonSet{})
	if err != nil {
		return fmt.Errorf("error in creating the two-way merge patch for current DaemonSet %q: %v",
			currentDaemonSet.Name, err)
	}

	// Apply the patch to the current DaemonSet
	err = i.client.PatchDaemonSetByLabel(TridentNodeLabel, patchBytes)
	if err != nil {
		return fmt.Errorf("could not patch Trident DaemonSet; %v", err)
	}
	log.Debug("Patched Trident DaemonSet.")

	return nil
}

// genericPatch takes current object, corresponding YAML to identify the changes and the patch that should be created
func (i *Installer) genericPatch(original interface{}, modifiedYAML []byte, dataStruct interface{}) ([]byte, error) {

	// Get existing object in JSON format
	originalJSON, err := json.Marshal(original)
	if err != nil {
		return nil, fmt.Errorf("error in marshaling current object; %v", err)
	}

	// Convert new object from YAML to JSON format
	modifiedJSON, err := yaml.YAMLToJSON(modifiedYAML)
	if err != nil {
		return nil, fmt.Errorf("could not convert new object from YAML to JSON; %v", err)
	}

	// Identify the deltas
	return jsonpatch.MergePatch(originalJSON, modifiedJSON)

	// Old alternative:
	// return strategicpatch.CreateTwoWayMergePatch(originalJSON, modifiedJSON, dataStruct)
}
