// Copyright 2025 NetApp, Inc. All Rights Reserved.

package installer

import (
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/ghodss/yaml"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/pkg/collection"
	"github.com/netapp/trident/utils/errors"
)

// K8sClient is a method receiver that implements the ExtendedK8sClient interface
type K8sClient struct {
	k8sclient.KubernetesClient
}

// NewExtendedK8sClient returns a concrete ExtendedK8sClient object
func NewExtendedK8sClient(kubeConfig *rest.Config, namespace string, k8sTimeout time.Duration) (ExtendedK8sClient,
	error,
) {
	kubeClient, err := k8sclient.NewKubeClient(kubeConfig, namespace, k8sTimeout)
	return &K8sClient{kubeClient}, err
}

// CreateCustomResourceDefinition creates a CRD.
func (k *K8sClient) CreateCustomResourceDefinition(crdName, crdYAML string) error {
	if err := k.CreateObjectByYAML(crdYAML); err != nil {
		return fmt.Errorf("could not create CRD %s; err: %v", crdName, err)
	}
	Log().WithField("CRD", crdName).Infof("Created CRD.")
	return nil
}

// PutCustomResourceDefinition ensures a CRD is created and on the most recent CRD version.
func (k *K8sClient) PutCustomResourceDefinition(
	currentCRD *apiextensionv1.CustomResourceDefinition, crdName string, createCRD bool, newCRDYAML string,
) error {
	logFields := LogFields{
		"CRD": crdName,
	}

	// If the CRD exists, patch it with the most current definition to avoid stale CRDs from previous installations.
	if createCRD {
		// Create the CRDs and wait for them to be registered in Kubernetes.
		Log().WithFields(logFields).Infof("Installer will create a fresh CRD.")

		if err := k.CreateCustomResourceDefinition(crdName, newCRDYAML); err != nil {
			return err
		}

		// Wait for the CRD to be fully established.
		if err := k.WaitForCRDEstablished(crdName, k8sTimeout); err != nil {
			// If CRD registration failed *and* we created the CRD, clean up by deleting the CRD.
			Log().WithFields(LogFields{
				"CRD": crdName,
				"err": err,
			}).Errorf("CRD not established.")

			if err = k.DeleteCustomResourceDefinition(crdName, newCRDYAML); err != nil {
				Log().WithFields(LogFields{
					"CRD": crdName,
					"err": err,
				}).Errorf("Could not delete CRD.")
			}
			return err
		}
	} else {
		// Patch the CRD.
		Log().WithFields(logFields).Infof("CRD present; patching to ensure it is not stale.")

		// Generate the deltas between the currentCRD and the new CRD YAML.
		patchBytes, err := k8sclient.GenericPatch(currentCRD, []byte(newCRDYAML))
		if err != nil {
			return fmt.Errorf("error in creating the two-way merge patch for %s CRD; %v", crdName, err)
		}

		// Patch the CRD with latest CRD definition.
		patchType := types.MergePatchType
		if err = k.PatchCRD(crdName, patchBytes, patchType); err != nil {
			Log().WithFields(LogFields{
				"CRD": crdName,
				"err": err,
			}).Errorf("Could not patch CRD.")
			return err
		}
	}

	return nil
}

// DeleteCustomResourceDefinition deletes a CRD.
func (k *K8sClient) DeleteCustomResourceDefinition(crdName, crdYAML string) error {
	if err := k.DeleteObjectByYAML(crdYAML, false); err != nil {
		return fmt.Errorf("could not delete CRD %s; err: %v", crdName, err)
	}
	Log().WithField("CRD", crdName).Infof("Deleted custom resource definitions.")
	return nil
}

// WaitForCRDEstablished waits until a CRD is Established.
func (k *K8sClient) WaitForCRDEstablished(crdName string, timeout time.Duration) error {
	checkCRDEstablished := func() error {
		crd, err := k.GetCRD(crdName)
		if err != nil {
			return err
		}
		for _, condition := range crd.Status.Conditions {
			if condition.Type == apiextensionv1.Established {
				switch condition.Status {
				case apiextensionv1.ConditionTrue:
					return nil
				default:
					return fmt.Errorf("CRD %s Established condition is %s", crdName, condition.Status)
				}
			}
		}
		return fmt.Errorf("CRD %s Established condition is not yet available", crdName)
	}

	checkCRDNotify := func(err error, duration time.Duration) {
		Log().WithFields(LogFields{
			"CRD": crdName,
			"err": err,
		}).Debug("CRD not yet established, waiting.")
	}

	checkCRDBackoff := backoff.NewExponentialBackOff()
	checkCRDBackoff.MaxInterval = 5 * time.Second
	checkCRDBackoff.MaxElapsedTime = timeout

	Log().WithField("CRD", crdName).Trace("Waiting for CRD to be established.")

	if err := backoff.RetryNotify(checkCRDEstablished, checkCRDBackoff, checkCRDNotify); err != nil {
		return fmt.Errorf("CRD was not established after %3.2f seconds", timeout.Seconds())
	}

	Log().WithField("CRD", crdName).Debug("CRD established.")
	return nil
}

// GetCSIDriverInformation gets the CSI drivers info in a cluster associated with Trident.
func (k *K8sClient) GetCSIDriverInformation(csiDriverName, appLabel string, shouldUpdate bool) (*storagev1.CSIDriver,
	[]storagev1.CSIDriver, bool, error,
) {
	createCSIDriver := true
	var currentCSIDriver *storagev1.CSIDriver
	var unwantedCSIDrivers []storagev1.CSIDriver

	if csiDrivers, err := k.GetCSIDriversByLabel(appLabel); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of CSI driver custom resources by label.")
		return nil, nil, true, fmt.Errorf("unable to get list of CSI driver custom resources by label")
	} else if len(csiDrivers) == 0 {
		Log().Info("CSI driver custom resource not found.")

		Log().Debug("Deleting unlabeled Trident CSI Driver by name as it can cause issues during installation.")
		if err = k.DeleteCSIDriver(csiDriverName); err != nil {
			if !errors.IsResourceNotFoundError(err) {
				Log().WithField("error", err).Warning("Could not delete Trident CSI driver custom resource.")
			}
		} else {
			Log().WithField("CSIDriver", csiDriverName).Info("Deleted Trident CSI driver custom resource; " +
				"replacing it with a labeled Trident CSI driver custom resource.")
		}

	} else if shouldUpdate {
		unwantedCSIDrivers = csiDrivers
	} else {
		// Rules:
		// 1. If there is no CSI driver CR named csi.trident.netapp.io and one or many other CSI driver CRs
		//    exist that matches the label then remove all the CSI driver CRs.
		// 2. If there is a CSI driver CR named csi.trident.netapp.io and one or many other CSI driver CRs
		//    exist that matches the label then remove all other CSI driver CRs.
		for _, csiDriver := range csiDrivers {
			if csiDriver.Name == csiDriverName {
				// Found a CSIDriver named csi.trident.netapp.io
				Log().WithField("TridentCSIDriver", csiDriverName).Infof("A Trident CSI driver found by label.")

				// Allocate new memory for currentCSIDriver to avoid unintentional reassignments due to reuse of the
				// csiDriver variable across iterations
				currentCSIDriver = &storagev1.CSIDriver{}
				*currentCSIDriver = csiDriver
				createCSIDriver = false
			} else {
				Log().WithField("CSIDriver", csiDriver.Name).Errorf("A CSI driver was found by label "+
					"but does not meet name '%s' requirement, marking it for deletion.", csiDriverName)

				unwantedCSIDrivers = append(unwantedCSIDrivers, csiDriver)
			}
		}
	}

	return currentCSIDriver, unwantedCSIDrivers, createCSIDriver, nil
}

// PutCSIDriver creates or updates a CSI Driver associated with Trident.
func (k *K8sClient) PutCSIDriver(
	currentCSIDriver *storagev1.CSIDriver, createCSIDriver bool, newCSIDriverYAML, appLabel string,
) error {
	CSIDriverName := getCSIDriverName()
	logFields := LogFields{
		"CSIDriver": CSIDriverName,
	}

	if createCSIDriver {
		Log().WithFields(logFields).Debug("Creating CSI driver custom resource.")

		if err := k.CreateObjectByYAML(newCSIDriverYAML); err != nil {
			return fmt.Errorf("could not create CSI driver; %v", err)
		}

		Log().Info("Created CSI driver custom resource.")
	} else {
		Log().WithFields(logFields).Debug("Patching Trident CSI driver CR.")

		// Identify the deltas
		patchBytes, err := k8sclient.GenericPatch(currentCSIDriver, []byte(newCSIDriverYAML))
		if err != nil {
			return fmt.Errorf("error in creating the two-way merge patch for current CSI driver %q: %v",
				CSIDriverName, err)
		}

		// Apply the patch to the current CSI driver
		patchType := types.MergePatchType
		if err := k.PatchCSIDriverByLabel(appLabel, patchBytes, patchType); err != nil {
			return fmt.Errorf("could not patch CSI driver; %v", err)
		}

		Log().Debug("Patched Trident CSI driver.")
	}

	return nil
}

// DeleteCSIDriverCR deletes a CSI Driver.
func (k *K8sClient) DeleteCSIDriverCR(csiDriverName, appLabel string) error {
	if csiDrivers, err := k.GetCSIDriversByLabel(appLabel); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of CSI driver CRs by label.")
		return fmt.Errorf("unable to get list of CSI driver CRs by label")
	} else if len(csiDrivers) == 0 {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Warning("CSI driver CR not found.")

		Log().Debug("Deleting unlabeled Trident CSI Driver by name as it may have been created outside of the Trident" +
			" Operator.")
		if err = k.DeleteCSIDriver(csiDriverName); err != nil {
			if !errors.IsResourceNotFoundError(err) {
				Log().WithField("error", err).Warning("Could not delete Trident CSI driver custom resource.")
			}
		} else {
			Log().WithField("CSIDriver", csiDriverName).Info("Deleted unlabeled Trident CSI driver custom resource.")
		}
	} else {
		if len(csiDrivers) == 1 {
			Log().WithFields(LogFields{
				"CSIDriver": csiDrivers[0].Name,
				"namespace": csiDrivers[0].Namespace,
			}).Info("Trident CSI driver CR found by label.")
		} else {
			Log().WithField("label", appLabel).Warnf("Multiple CSI driver CRs found matching label; removing all.")
		}

		if err = k.RemoveMultipleCSIDriverCRs(csiDrivers); err != nil {
			return err
		}
	}

	return nil
}

// RemoveMultipleCSIDriverCRs removes a list of unwanted CSI drivers in a cluster.
func (k *K8sClient) RemoveMultipleCSIDriverCRs(unwantedCSIDriverCRs []storagev1.CSIDriver) error {
	var err error
	var anyError bool
	var undeletedCSIDriverCRs []string

	if len(unwantedCSIDriverCRs) > 0 {
		// Delete the CSI driver CRs
		for _, CSIDriverCRToRemove := range unwantedCSIDriverCRs {
			if err = k.DeleteCSIDriver(CSIDriverCRToRemove.Name); err != nil {
				Log().WithFields(LogFields{
					"CSIDriver": CSIDriverCRToRemove.Name,
					"label":     appLabel,
					"error":     err,
				}).Warning("Could not delete CSI driver CR.")

				anyError = true
				undeletedCSIDriverCRs = append(undeletedCSIDriverCRs, fmt.Sprintf("%v", CSIDriverCRToRemove.Name))
			} else {
				Log().WithField("csiDriver", CSIDriverCRToRemove.Name).Infof("Deleted CSI driver.")
			}
		}
	}

	if anyError {
		return fmt.Errorf("unable to delete CSI driver CR(s): %v", undeletedCSIDriverCRs)
	}

	return nil
}

// RemoveMultipleClusterRoles removes a list of unwanted cluster roles in a cluster.
func (k *K8sClient) RemoveMultipleClusterRoles(unwantedClusterRoles []rbacv1.ClusterRole) error {
	var err error
	var anyError bool
	var undeletedClusterRoles []string

	if len(unwantedClusterRoles) > 0 {
		// Delete the cluster roles
		for _, clusterRoleToRemove := range unwantedClusterRoles {
			if err = k.DeleteClusterRole(clusterRoleToRemove.Name); err != nil {
				Log().WithFields(LogFields{
					"clusterRole": clusterRoleToRemove.Name,
					"error":       err,
				}).Warning("Could not delete cluster role.")

				anyError = true
				undeletedClusterRoles = append(undeletedClusterRoles, fmt.Sprintf("%v", clusterRoleToRemove.Name))
			} else {
				Log().WithField("clusterRole", clusterRoleToRemove.Name).Infof("Deleted cluster role.")
			}
		}
	}

	if anyError {
		return fmt.Errorf("unable to delete cluster role(s): %v", undeletedClusterRoles)
	}

	return nil
}

// GetClusterRoleInformation gets the Cluster Role info.
func (k *K8sClient) GetClusterRoleInformation(clusterRoleName, appLabel string, shouldUpdate bool) (*rbacv1.ClusterRole,
	[]rbacv1.ClusterRole, bool, error,
) {
	createClusterRole := true
	var currentClusterRole *rbacv1.ClusterRole
	var unwantedClusterRoles []rbacv1.ClusterRole

	if clusterRoles, err := k.GetClusterRolesByLabel(appLabel); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of cluster roles by label.")
		return nil, nil, true, fmt.Errorf("unable to get list of cluster roles")
	} else if len(clusterRoles) == 0 {
		Log().Info("Trident cluster role not found.")

		Log().Debug("Deleting unlabeled Trident cluster role by name as it can cause issues during installation.")
		if err = k.DeleteClusterRole(clusterRoleName); err != nil {
			if !errors.IsResourceNotFoundError(err) {
				Log().WithField("error", err).Warning("Could not delete Trident cluster role")
			}
		} else {
			Log().WithField("ClusterRole", clusterRoleName).Info(
				"Deleted unlabeled Trident cluster role; replacing it with a labeled Trident cluster role.")
		}
	} else if shouldUpdate {
		unwantedClusterRoles = clusterRoles
	} else {
		// Rules:
		// 1. If there is no cluster role matching the allowed name and one or many other cluster roles
		//    exist that matches the label then remove all the cluster roles.
		// 2. If there is a cluster role named matching the allowed name and one or many other cluster roles
		//    exist that matches the label then remove all other cluster roles.
		for _, clusterRole := range clusterRoles {
			if clusterRole.Name == clusterRoleName {
				// Found a cluster role matching the allowed name
				Log().WithField("clusterRole", clusterRoleName).Infof("A Trident cluster role found by label.")

				// allocate new memory for currentClusterRole to avoid unintentional reassignments due to reuse of the
				// clusterRole variable across iterations
				currentClusterRole = &rbacv1.ClusterRole{}
				*currentClusterRole = clusterRole
				createClusterRole = false
			} else {
				Log().WithField("clusterRole", clusterRole.Name).Errorf("A cluster role was found by label "+
					"but does not meet name '%s' requirement, marking it for deletion.", clusterRoleName)

				unwantedClusterRoles = append(unwantedClusterRoles, clusterRole)
			}
		}
	}

	return currentClusterRole, unwantedClusterRoles, createClusterRole, nil
}

// GetMultipleRoleInformation gets the  Role info.
func (k *K8sClient) GetMultipleRoleInformation(roleNames []string, appLabel string, shouldUpdate bool) (
	map[string]*rbacv1.Role, []rbacv1.Role, map[string]bool, error,
) {
	currentRoleMap := make(map[string]*rbacv1.Role)
	reuseRoleMap := make(map[string]bool)
	var currentRole *rbacv1.Role
	var unwantedRoles []rbacv1.Role

	if roles, err := k.GetRolesByLabel(appLabel); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of roles by label.")
		return currentRoleMap, unwantedRoles, reuseRoleMap,
			fmt.Errorf("unable to get list of roles")
	} else if len(roles) == 0 {
		Log().Info("Trident role not found.")

		Log().Debug("Deleting unlabeled Trident role by name as it can cause issues during installation.")
		for _, roleName := range roleNames {
			if err = k.DeleteRole(roleName); err != nil {
				if !errors.IsResourceNotFoundError(err) {
					Log().WithField("error", err).Warning("Could not delete Trident role")
				}
			} else {
				Log().WithField("role", roleName).Info(
					"Deleted unlabeled Trident role; replacing it with a labeled Trident role.")
			}
		}
	} else if shouldUpdate {
		unwantedRoles = roles
	} else {
		// Rules:
		// 1. If there is no role with any of the allowed role names and one or many other roles
		//    exist that matches the label then remove all the roles.
		// 2. If there is a role with any of the allowed role names and one or many other roles
		//    exist that matches the label then remove all other roles.
		for _, role := range roles {
			// Found a role matching one of the allowed names
			if collection.ContainsString(roleNames, role.Name) {
				Log().WithField("role", role.Name).Infof("A Trident role found by label.")

				// Allocate new memory for currentRole to avoid unintentional reassignments due to reuse of the
				// Role variable across iterations
				currentRole = &rbacv1.Role{}
				*currentRole = role
				currentRoleMap[role.Name] = currentRole
				reuseRoleMap[role.Name] = true
			} else {
				Log().WithField("Role", role.Name).Errorf("A role was found by label "+
					"but does not meet name '%s' requirement, marking it for deletion.", role.Name)

				unwantedRoles = append(unwantedRoles, role)
			}
		}
	}

	return currentRoleMap, unwantedRoles, reuseRoleMap, nil
}

// PutRole creates or updates a Role associated with Trident.
func (k *K8sClient) PutRole(
	currentRole *rbacv1.Role, reuseRole bool, newRoleYAML, appLabel string,
) error {
	var roleName string

	if currentRole != nil {
		roleName = currentRole.Name
	}

	logFields := LogFields{
		"role": roleName,
	}

	if !reuseRole {
		Log().Debug("Creating role.")

		if err := k.CreateObjectByYAML(newRoleYAML); err != nil {
			return fmt.Errorf("could not create role; %v", err)
		}

		Log().Info("Created role.")
	} else {
		Log().WithFields(logFields).Debug("Patching Trident role.")

		// Identify the deltas
		patchBytes, err := k8sclient.GenericPatch(currentRole, []byte(newRoleYAML))
		if err != nil {
			return fmt.Errorf("error in creating the two-way merge patch for current role %q: %v",
				roleName, err)
		}

		// Apply the patch to the current Role
		patchType := types.MergePatchType
		if err = k.PatchRoleByLabelAndName(appLabel, roleName, patchBytes, patchType); err != nil {
			return fmt.Errorf("could not patch Trident role; %v", err)
		}

		Log().Debug("Patched Trident role.")
	}

	return nil
}

// PutClusterRole creates or updates a Cluster Role associated with Trident.
func (k *K8sClient) PutClusterRole(currentClusterRole *rbacv1.ClusterRole, createClusterRole bool, newClusterRoleYAML, appLabel string) error {
	var clusterRoleName string

	if currentClusterRole != nil {
		clusterRoleName = currentClusterRole.Name
	}

	logFields := LogFields{
		"clusterRole": clusterRoleName,
	}

	if createClusterRole {
		Log().WithFields(logFields).Debug("Creating cluster role.")

		if err := k.CreateObjectByYAML(newClusterRoleYAML); err != nil {
			return fmt.Errorf("could not create cluster role; %v", err)
		}

		Log().Info("Created cluster role.")
	} else {
		Log().WithFields(logFields).Debug("Patching Trident Cluster role.")

		// Identify the deltas
		patchBytes, err := k8sclient.GenericPatch(currentClusterRole, []byte(newClusterRoleYAML))
		if err != nil {
			return fmt.Errorf("error in creating the two-way merge patch for current cluster role %q: %v",
				clusterRoleName, err)
		}

		// Apply the patch to the current Cluster Role
		patchType := types.MergePatchType
		if err = k.PatchClusterRoleByLabel(appLabel, patchBytes, patchType); err != nil {
			return fmt.Errorf("could not patch Trident Cluster role; %v", err)
		}

		Log().Debug("Patched Trident Cluster role.")
	}

	return nil
}

// DeleteTridentClusterRole deletes a Cluster Role associated with Trident.
func (k *K8sClient) DeleteTridentClusterRole(clusterRoleName, appLabel string) error {
	// Delete cluster role
	if clusterRoles, err := k.GetClusterRolesByLabel(appLabel); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of cluster roles by label.")
		return fmt.Errorf("unable to get list of cluster roles")
	} else if len(clusterRoles) == 0 {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Warning("Trident cluster role not found.")

		Log().Debug("Deleting unlabeled Trident cluster role by name as it may have been created outside of the Trident" +
			" Operator.")
		if err = k.DeleteClusterRole(clusterRoleName); err != nil {
			if !errors.IsResourceNotFoundError(err) {
				Log().WithField("error", err).Warning("Could not delete Trident cluster role.")
			}
		} else {
			Log().WithField("Cluster Role", clusterRoleName).Info(
				"Deleted unlabeled Trident cluster role.")
		}
	} else {
		for idx := range clusterRoles {
			Log().WithFields(LogFields{
				"clusterRole": clusterRoles[idx].Name,
			}).Info("Trident Cluster role found by label.")
		}

		Log().WithField("label", appLabel).Warnf("Multiple Cluster roles found matching label; removing all.")
		if err = k.RemoveMultipleClusterRoles(clusterRoles); err != nil {
			return err
		}
	}

	return nil
}

// DeleteMultipleTridentRoles deletes Role(s) associated with Trident.
func (k *K8sClient) DeleteMultipleTridentRoles(roleNames []string, appLabel string) error {
	// Delete  role
	if roles, err := k.GetRolesByLabel(appLabel); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of roles by label.")
		return fmt.Errorf("unable to get list of roles")
	} else if len(roles) == 0 {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Warning("Trident role not found.")

		Log().Debug("Deleting unlabeled Trident role by name as it may have been created outside of the Trident" +
			" Operator.")
		for _, roleName := range roleNames {
			if err = k.DeleteRole(roleName); err != nil {
				if !errors.IsResourceNotFoundError(err) {
					Log().WithField("error", err).Warning("Could not delete Trident role.")
				}
			} else {
				Log().WithField("role", roleName).Info(
					"Deleted unlabeled Trident role.")
			}
		}
	} else {
		for idx := range roles {
			Log().WithFields(LogFields{
				"role":      roles[idx].Name,
				"namespace": roles[idx].Namespace,
			}).Debug("Trident role found by label.")
		}

		if err = k.RemoveMultipleRoles(roles); err != nil {
			return err
		}
	}

	return nil
}

// GetClusterRoleBindingInformation gets the info on a Cluster Role Binding associated with Trident.
func (k *K8sClient) GetClusterRoleBindingInformation(clusterRoleBindingName, appLabel string, shouldUpdate bool) (*rbacv1.ClusterRoleBinding,
	[]rbacv1.ClusterRoleBinding, bool, error,
) {
	createClusterRoleBinding := true
	var currentClusterRoleBinding *rbacv1.ClusterRoleBinding
	var unwantedClusterRoleBindings []rbacv1.ClusterRoleBinding

	if clusterRoleBindings, err := k.GetClusterRoleBindingsByLabel(appLabel); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of cluster role bindings by label.")
		return nil, nil, true, fmt.Errorf("unable to get list of cluster role bindings")
	} else if len(clusterRoleBindings) == 0 {
		Log().Info("Trident cluster role binding not found.")

		Log().Debug("Deleting unlabeled Trident cluster role binding by name as it can cause issues during installation.")
		if err = k.DeleteClusterRoleBinding(clusterRoleBindingName); err != nil {
			if !errors.IsResourceNotFoundError(err) {
				Log().WithField("error", err).Warning("Could not delete Trident cluster role binding.")
			}
		} else {
			Log().WithField("Cluster Role Binding", clusterRoleBindingName).Info(
				"Deleted unlabeled Trident cluster role binding; replacing it with a labeled Trident cluster role" +
					" binding.")
		}
	} else if shouldUpdate {
		unwantedClusterRoleBindings = clusterRoleBindings
	} else {
		// Rules:
		// 1. If there is no cluster role binding matching the allowed name and one or many other cluster role bindings
		//    exist that matches the label then remove all the cluster role bindings.
		// 2. If there is a cluster role binding matching the allowed name and one or many other cluster role bindings
		//    exist that matches the label then remove all other cluster role bindings.
		for _, clusterRoleBinding := range clusterRoleBindings {
			if clusterRoleBinding.Name == clusterRoleBindingName {
				// Found a cluster role binding matching the allowed name
				Log().WithField("clusterRoleBinding", clusterRoleBindingName).Infof(
					"A Trident cluster role binding was found by label.")

				// allocate new memory for currentClusterRoleBinding to avoid unintentional reassignments due to reuse of the
				// clusterRoleBinding variable across iterations
				currentClusterRoleBinding = &rbacv1.ClusterRoleBinding{}
				*currentClusterRoleBinding = clusterRoleBinding
				createClusterRoleBinding = false
			} else {
				Log().WithField("clusterRoleBinding", clusterRoleBinding.Name).Errorf(
					"A cluster role binding was found by label "+
						"but does not meet name '%s' requirement, marking it for deletion.", clusterRoleBindingName)

				unwantedClusterRoleBindings = append(unwantedClusterRoleBindings, clusterRoleBinding)
			}
		}
	}
	return currentClusterRoleBinding, unwantedClusterRoleBindings, createClusterRoleBinding, nil
}

// PutClusterRoleBinding creates or updates a Cluster Role Binding associated with Trident.
func (k *K8sClient) PutClusterRoleBinding(currentClusterRoleBinding *rbacv1.ClusterRoleBinding, createClusterRoleBinding bool, newClusterRoleBindingYAML, appLabel string) error {
	var clusterRoleBindingName string

	if currentClusterRoleBinding != nil {
		clusterRoleBindingName = currentClusterRoleBinding.Name
	}

	logFields := LogFields{
		"clusterRoleBinding": clusterRoleBindingName,
	}

	if createClusterRoleBinding {
		Log().WithFields(logFields).Debug("Creating cluster role binding.")

		if err := k.CreateObjectByYAML(newClusterRoleBindingYAML); err != nil {
			return fmt.Errorf("could not create cluster role binding; %v", err)
		}

		Log().Info("Created cluster role binding.")
	} else {
		Log().WithFields(logFields).Debug("Patching Trident Cluster role binding.")

		// Identify the deltas
		patchBytes, err := k8sclient.GenericPatch(currentClusterRoleBinding, []byte(newClusterRoleBindingYAML))
		if err != nil {
			return fmt.Errorf("error in creating the two-way merge patch for current cluster role binding %q: %v",
				clusterRoleBindingName, err)
		}

		// Apply the patch to the current Cluster Role Binding
		patchType := types.MergePatchType
		if err = k.PatchClusterRoleBindingByLabel(appLabel, patchBytes, patchType); err != nil {
			return fmt.Errorf("could not patch cluster role binding; %v", err)
		}

		Log().Debug("Patched Trident Cluster role binding.")
	}

	return nil
}

// DeleteTridentClusterRoleBinding deletes a Cluster Role Binding associated with Trident.
func (k *K8sClient) DeleteTridentClusterRoleBinding(clusterRoleBindingName, appLabel string) error {
	// Fetch cluster role bindings by label
	if clusterRoleBindings, err := k.GetClusterRoleBindingsByLabel(appLabel); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of cluster role bindings by label.")
		return fmt.Errorf("unable to get list of cluster role bindings")
	} else if len(clusterRoleBindings) == 0 {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Warning("Trident cluster role binding not found.")

		Log().Debug("Deleting unlabeled Trident cluster role binding by name as it may have been created outside of the" +
			" Trident Operator.")
		if err = k.DeleteClusterRoleBinding(clusterRoleBindingName); err != nil {
			if !errors.IsResourceNotFoundError(err) {
				Log().WithField("error", err).Warning("Could not delete Trident cluster role binding.")
			}
		} else {
			Log().WithField("Cluster Role Binding", clusterRoleBindingName).Info(
				"Deleted unlabeled Trident cluster role binding.")
		}
	} else {
		for idx := range clusterRoleBindings {
			Log().WithFields(LogFields{
				"clusterRoleBinding": clusterRoleBindings[idx].Name,
			}).Info("Trident Cluster role binding found by label.")
		}

		Log().WithField("label", appLabel).Warnf("Multiple Cluster role bindings found matching label; removing" +
			" all.")

		if err = k.RemoveMultipleClusterRoleBindings(clusterRoleBindings); err != nil {
			return err
		}
	}

	return nil
}

// RemoveMultipleClusterRoleBindings removes a list of unwanted cluster role bindings in a cluster.
func (k *K8sClient) RemoveMultipleClusterRoleBindings(unwantedClusterRoleBindings []rbacv1.ClusterRoleBinding) error {
	var err error
	var anyError bool
	var undeletedClusterRoleBindings []string

	if len(unwantedClusterRoleBindings) > 0 {
		// Delete the cluster roles bindings
		for _, clusterRoleBindingToRemove := range unwantedClusterRoleBindings {
			if err = k.DeleteClusterRoleBinding(clusterRoleBindingToRemove.Name); err != nil {
				Log().WithFields(LogFields{
					"clusterRoleBinding": clusterRoleBindingToRemove.Name,
					"error":              err,
				}).Warning("Could not delete cluster role binding.")

				anyError = true
				undeletedClusterRoleBindings = append(undeletedClusterRoleBindings,
					fmt.Sprintf("%v", clusterRoleBindingToRemove.Name))
			} else {
				Log().WithField("clusterRoleBinding", clusterRoleBindingToRemove.Name).Infof(
					"Deleted cluster role binding.")
			}
		}
	}

	if anyError {
		return fmt.Errorf("unable to delete cluster role binding(s): %v", undeletedClusterRoleBindings)
	}

	return nil
}

// RemoveMultipleRoles removes a list of unwanted roles in a .
func (k *K8sClient) RemoveMultipleRoles(unwantedRoles []rbacv1.Role) error {
	var err error
	var anyError bool
	var undeletedRoles []string

	if len(unwantedRoles) > 0 {
		// Delete the  roles
		for _, roleToRemove := range unwantedRoles {
			if err = k.DeleteRole(roleToRemove.Name); err != nil {
				Log().WithFields(LogFields{
					"role":  roleToRemove.Name,
					"error": err,
				}).Warning("Could not delete  role.")

				anyError = true
				undeletedRoles = append(undeletedRoles, fmt.Sprintf("%v", roleToRemove.Name))
			} else {
				Log().WithField("role", roleToRemove.Name).Infof("Deleted role.")
			}
		}
	}

	if anyError {
		return fmt.Errorf("unable to delete role(s): %v", undeletedRoles)
	}

	return nil
}

// GetMultipleRoleBindingInformation gets the info on a Role Binding associated with Trident.
func (k *K8sClient) GetMultipleRoleBindingInformation(
	roleBindingNames []string, appLabel string, shouldUpdate bool,
) (map[string]*rbacv1.RoleBinding,
	[]rbacv1.RoleBinding, map[string]bool, error,
) {
	reuseRoleBindingMap := make(map[string]bool)
	currentRoleBindingMap := make(map[string]*rbacv1.RoleBinding)
	var currentRoleBinding *rbacv1.RoleBinding
	var unwantedRoleBindings []rbacv1.RoleBinding

	if roleBindings, err := k.GetRoleBindingsByLabel(appLabel); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of role bindings by label.")
		return currentRoleBindingMap, unwantedRoleBindings, reuseRoleBindingMap,
			fmt.Errorf("unable to get list of role bindings")
	} else if len(roleBindings) == 0 {
		Log().Info("Trident role binding not found.")

		Log().Debug("Deleting unlabeled Trident role binding by name as it can cause issues during installation.")
		for _, roleBindingName := range roleBindingNames {
			if err = k.DeleteRoleBinding(roleBindingName); err != nil {
				if !errors.IsResourceNotFoundError(err) {
					Log().WithField("error", err).Warning("Could not delete Trident role binding.")
				}
			} else {
				Log().WithField("roleBinding", roleBindingName).Info(
					"Deleted unlabeled Trident role binding; replacing it with a labeled Trident role" +
						" binding.")
			}
		}
	} else if shouldUpdate {
		unwantedRoleBindings = roleBindings
	} else {
		// Rules:
		// 1. If there is no role binding with any of the allowed names and one or many other  role bindings
		//    exist that matches the label then remove all the  role bindings.
		// 2. If there is a role binding with any of the allowed names and one or many other  role bindings
		//    exist that matches the label then remove all other  role bindings.
		for _, roleBinding := range roleBindings {
			if collection.ContainsString(roleBindingNames, roleBinding.Name) {
				// Found a  role binding matching one of the allowed names
				Log().WithField("roleBinding", roleBinding.Name).Infof(
					"A Trident role binding was found by label.")

				// Allocate new memory for currentRoleBinding to avoid unintentional reassignments due to reuse of the
				// RoleBinding variable across iterations
				currentRoleBinding = &rbacv1.RoleBinding{}
				*currentRoleBinding = roleBinding
				currentRoleBindingMap[roleBinding.Name] = currentRoleBinding
				reuseRoleBindingMap[roleBinding.Name] = true
			} else {
				Log().WithField("roleBinding", roleBinding.Name).Errorf(
					"A role binding was found by label "+
						"but does not meet name '%s' requirement, marking it for deletion.", roleBinding.Name)

				unwantedRoleBindings = append(unwantedRoleBindings, roleBinding)
			}
		}
	}
	return currentRoleBindingMap, unwantedRoleBindings, reuseRoleBindingMap, nil
}

// PutRoleBinding creates or updates a Role Binding associated with Trident.
func (k *K8sClient) PutRoleBinding(
	currentRoleBinding *rbacv1.RoleBinding, reuseRoleBinding bool,
	newRoleBindingYAML, appLabel string,
) error {
	var roleBindingName string

	if currentRoleBinding != nil {
		roleBindingName = currentRoleBinding.Name
	}

	logFields := LogFields{
		"roleBinding": roleBindingName,
	}

	if !reuseRoleBinding {
		Log().Debug("Creating role binding.")

		if err := k.CreateObjectByYAML(newRoleBindingYAML); err != nil {
			return fmt.Errorf("could not create role binding; %v", err)
		}

		Log().Info("Created role binding.")
	} else {
		Log().WithFields(logFields).Debug("Patching Trident role binding.")

		// Identify the deltas
		patchBytes, err := k8sclient.GenericPatch(currentRoleBinding, []byte(newRoleBindingYAML))
		if err != nil {
			return fmt.Errorf("error in creating the two-way merge patch for current role binding %q: %v",
				roleBindingName, err)
		}

		// Apply the patch to the current  Role Binding
		patchType := types.MergePatchType
		if err = k.PatchRoleBindingByLabelAndName(appLabel, roleBindingName, patchBytes, patchType); err != nil {
			return fmt.Errorf("could not patch role binding; %v", err)
		}

		Log().Debug("Patched Trident role binding.")
	}

	return nil
}

// DeleteMultipleTridentRoleBindings deletes Role Binding(s) associated with Trident.
func (k *K8sClient) DeleteMultipleTridentRoleBindings(roleBindingNames []string, appLabel string) error {
	// Delete  role binding
	if roleBindings, err := k.GetRoleBindingsByLabel(appLabel); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of role bindings by label.")
		return fmt.Errorf("unable to get list of role bindings")
	} else if len(roleBindings) == 0 {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Warning("Trident role binding not found.")

		Log().Debug("Deleting unlabeled Trident role binding by name as it may have been created outside of the" +
			" Trident Operator.")
		for _, roleBindingName := range roleBindingNames {
			if err := k.DeleteRoleBinding(roleBindingName); err != nil {
				if !errors.IsResourceNotFoundError(err) {
					Log().WithField("error", err).Warning("Could not delete Trident role binding.")
				}
			} else {
				Log().WithField("roleBinding", roleBindingName).Info(
					"Deleted unlabeled Trident role binding.")
			}
		}
	} else {
		for idx := range roleBindings {
			Log().WithFields(LogFields{
				"roleBinding": roleBindings[idx].Name,
				"namespace":   roleBindings[idx].Namespace,
			}).Debug("Trident role binding found by label.")
		}

		if err = k.RemoveMultipleRoleBindings(roleBindings); err != nil {
			return err
		}
	}

	return nil
}

// RemoveMultipleRoleBindings removes a list of unwanted  role bindings in a .
func (k *K8sClient) RemoveMultipleRoleBindings(unwantedRoleBindings []rbacv1.RoleBinding) error {
	var err error
	var anyError bool
	var undeletedRoleBindings []string

	if len(unwantedRoleBindings) > 0 {
		// Delete the roles bindings
		for _, roleBindingToRemove := range unwantedRoleBindings {
			if err = k.DeleteRoleBinding(roleBindingToRemove.Name); err != nil {
				Log().WithFields(LogFields{
					"roleBinding": roleBindingToRemove.Name,
					"error":       err,
				}).Warning("Could not delete role binding.")

				anyError = true
				undeletedRoleBindings = append(undeletedRoleBindings,
					fmt.Sprintf("%v", roleBindingToRemove.Name))
			} else {
				Log().WithField("roleBinding", roleBindingToRemove.Name).Infof(
					"Deleted role binding.")
			}
		}
	}

	if anyError {
		return fmt.Errorf("unable to delete role binding(s): %v", undeletedRoleBindings)
	}

	return nil
}

// GetResourceQuotaInformation identifies the Operator-based Trident resource quota information and any unwanted
// resource quotas
func (k *K8sClient) GetResourceQuotaInformation(
	resourcequotaName, label, namespace string,
) (*corev1.ResourceQuota, []corev1.ResourceQuota, bool, error) {
	createResourceQuota := true
	var currentResourceQuota *corev1.ResourceQuota
	var unwantedResourceQuotas []corev1.ResourceQuota

	if resourceQuotas, err := k.GetResourceQuotasByLabel(label); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of resource quotas by label.")
		return nil, nil, true, fmt.Errorf("unable to get list of resource quotas")
	} else if len(resourceQuotas) == 0 {
		Log().WithFields(LogFields{
			"label":     label,
			"namespace": namespace,
		}).Info("No Trident resource quotas found by label.")
	} else {
		// Rules:
		// 1. If there are no resource quotas named trident/trident-csi in CR namespace and one or many other resource quotas
		//    exist that matches the label then remove all the resource quotas.
		// 2. If there is a resource quotas named trident/trident-csi in CR namespace and one or many other resource quotas
		//    exist that matches the label then remove all other resource quotas.
		for _, resourcequota := range resourceQuotas {
			if resourcequota.Namespace == namespace && resourcequota.Name == resourcequotaName {
				// Found a resourcequota named in the same namespace
				Log().WithFields(LogFields{
					"resourcequota": resourcequota.Name,
					"namespace":     resourcequota.Namespace,
				}).Info("A Trident resource quota was found by label.")

				// Allocate new memory for currentResourceQuota to avoid unintentional reassignments due to reuse of the
				// resourcequota variable across iterations
				currentResourceQuota = &corev1.ResourceQuota{}
				*currentResourceQuota = resourcequota
				createResourceQuota = false
			} else {
				Log().WithFields(LogFields{
					"resourcequota":          resourcequota.Name,
					"resourcequotaNamespace": resourcequota.Namespace,
				}).Errorf("A resource quota was found by label which does not meet either name %s or namespace"+
					" '%s' requirement, marking it for deletion.", resourcequotaName, namespace)

				unwantedResourceQuotas = append(unwantedResourceQuotas, resourcequota)
			}
		}
	}

	return currentResourceQuota, unwantedResourceQuotas, createResourceQuota, nil
}

// PutResourceQuota will create a new Resource Quota or patch an existing one.
func (k *K8sClient) PutResourceQuota(
	currentResourceQuota *corev1.ResourceQuota, createResourceQuota bool, newResourceQuotaYAML, appLabel string,
) error {
	resourceQuotaName := getResourceQuotaName()
	logFields := LogFields{
		"resourcequota": resourceQuotaName,
		"namespace":     k.Namespace(),
	}

	if createResourceQuota {
		Log().WithFields(logFields).Debug("Creating Trident resource quota.")

		if err := k.CreateObjectByYAML(newResourceQuotaYAML); err != nil {
			return fmt.Errorf("could not create Trident resource quota; %v", err)
		}

		Log().Info("Created Trident resource quota.")
	} else {
		Log().WithFields(logFields).Debug("Patching Trident resource quota.")

		// Identify the deltas
		patchBytes, err := k8sclient.GenericPatch(currentResourceQuota, []byte(newResourceQuotaYAML))
		if err != nil {
			return fmt.Errorf("error in creating the two-way merge patch for current resource quota %q: %v",
				resourceQuotaName, err)
		}

		// Apply the patch to the current ResourceQuota
		patchType := types.MergePatchType
		if err = k.PatchResourceQuotaByLabel(appLabel, patchBytes, patchType); err != nil {
			return fmt.Errorf("could not patch Trident resource quota; %v", err)
		}

		Log().Debug("Patched Trident Resource Quota.")
	}

	return nil
}

// DeleteTridentResourceQuota deletes a Trident Resource Quota.
func (k *K8sClient) DeleteTridentResourceQuota(nodeLabel string) error {
	// Delete Trident resourceQuotas
	if resourceQuotas, err := k.GetResourceQuotasByLabel(nodeLabel); err != nil {
		Log().WithFields(LogFields{
			"label": nodeLabel,
			"error": err,
		}).Errorf("Unable to get list of resource quotas by label.")
		return fmt.Errorf("unable to get list of resource quotas")

	} else if len(resourceQuotas) == 0 {
		Log().WithFields(LogFields{
			"label": nodeLabel,
			"error": err,
		}).Warning("Trident resource quota not found.")
	} else {
		if len(resourceQuotas) == 1 {
			Log().WithFields(LogFields{
				"resourcequota": resourceQuotas[0].Name,
				"namespace":     resourceQuotas[0].Namespace,
			}).Info("Trident resource quota found by label.")
		} else {
			Log().WithField("label", nodeLabel).Warn("Multiple resource quotas found by matching label; removing all.")
		}

		if err = k.RemoveMultipleResourceQuotas(resourceQuotas); err != nil {
			return err
		}
	}

	return nil
}

// RemoveMultipleResourceQuotas removes a list of unwanted resource quotas in a namespace
func (k *K8sClient) RemoveMultipleResourceQuotas(unwantedResourceQuotas []corev1.ResourceQuota) error {
	var err error
	var anyError bool
	var undeletedResourceQuotas []string

	if len(unwantedResourceQuotas) > 0 {
		for _, unwantedResourceQuota := range unwantedResourceQuotas {
			// Delete the resourcequota
			if err = k.DeleteResourceQuota(unwantedResourceQuota.Name); err != nil {
				Log().WithFields(LogFields{
					"resourcequota": unwantedResourceQuota.Name,
					"namespace":     unwantedResourceQuota.Namespace,
					"error":         err,
				}).Warning("Could not delete resource quota.")

				anyError = true
				undeletedResourceQuotas = append(undeletedResourceQuotas,
					fmt.Sprintf("%v/%v", unwantedResourceQuota.Namespace,
						unwantedResourceQuota.Name))
			} else {
				Log().WithFields(LogFields{
					"resourcequota": unwantedResourceQuota.Name,
					"namespace":     unwantedResourceQuota.Namespace,
				}).Info("Deleted Trident resource quota.")
			}
		}
	}

	if anyError {
		return fmt.Errorf("unable to delete resource quota(s): %v", undeletedResourceQuotas)
	}

	return nil
}

// GetDaemonSetInformation identifies the Operator-based Trident daemonset information and any unwanted daemonsets
func (k *K8sClient) GetDaemonSetInformation(nodeLabel, namespace string, isWindows bool) (*appsv1.DaemonSet,
	[]appsv1.DaemonSet, bool, error,
) {
	createDaemonSet := true
	linuxDS := getDaemonSetName(false)
	windowsDS := getDaemonSetName(true)
	var currentDaemonSet *appsv1.DaemonSet
	var unwantedDaemonSets []appsv1.DaemonSet

	daemonSets, err := k.GetDaemonSetsByLabel(nodeLabel, true)
	if err != nil {
		Log().WithFields(LogFields{
			"label": nodeLabel,
			"error": err,
		}).Errorf("Unable to get list of daemonset by label.")
		return nil, nil, true, fmt.Errorf("unable to get list of daemonset")
	} else if len(daemonSets) == 0 {
		Log().WithFields(LogFields{
			"label":     nodeLabel,
			"namespace": namespace,
		}).Info("No Trident daemonsets found by label.")
	} else {
		// Rules
		// 1. If no daemonSet(s) for Windows/Linux node exists in CR namespace and one or many other daemonSets
		//    exist that matches the label then remove all the daemonSet.
		// 2. If daemonSet named Windows/Linux node exists in CR namespace and one or many other daemonSets
		//    exist that matches the label then remove all other daemonSets.
		for _, daemonSet := range daemonSets {
			if daemonSet.Namespace == namespace && (daemonSet.Name == linuxDS || daemonSet.Name == windowsDS) {
				// Found a daemonSet named in the same namespace
				Log().WithFields(LogFields{
					"daemonSet": daemonSet.Name,
					"namespace": daemonSet.Namespace,
				}).Infof("A Trident daemonSet was found by label.")

				if isWindows && daemonSet.Name == windowsDS || !isWindows && daemonSet.Name == linuxDS {
					currentDaemonSet = &appsv1.DaemonSet{}
					*currentDaemonSet = daemonSet
					createDaemonSet = false
				}
			} else {
				Log().WithFields(LogFields{
					"daemonSet":          daemonSet.Name,
					"daemonSetNamespace": daemonSet.Namespace,
				}).Errorf("A daemonSet was found by label which does not meet either names %s, %s or namespace '%s"+
					"' requirement, marking it for deletion.", linuxDS, windowsDS, namespace)

				unwantedDaemonSets = append(unwantedDaemonSets, daemonSet)
			}
		}
	}

	return currentDaemonSet, unwantedDaemonSets, createDaemonSet, nil
}

// PutDaemonSet creates or updates a Trident DaemonSet.
func (k *K8sClient) PutDaemonSet(
	currentDaemonSet *appsv1.DaemonSet, createDaemonSet bool, newDaemonSetYAML, nodeLabel, daemonSetName string,
) error {
	logFields := LogFields{
		"daemonset": daemonSetName,
		"namespace": k.Namespace(),
	}

	if createDaemonSet {
		Log().WithFields(logFields).Debug("Creating Trident daemonset.")
		if err := k.CreateObjectByYAML(newDaemonSetYAML); err != nil {
			return fmt.Errorf("could not create Trident daemonset; %v", err)
		}

		Log().Info("Created Trident daemonset.")
	} else {
		Log().WithFields(logFields).Debug("Patching Trident daemonset.")

		// Identify the deltas
		patchBytes, err := k8sclient.GenericPatch(currentDaemonSet, []byte(newDaemonSetYAML))
		if err != nil {
			return fmt.Errorf("error in creating the two-way merge patch for current DaemonSet %q: %v",
				daemonSetName, err)
		}

		// Apply the patch to the current DaemonSet
		patchType := types.MergePatchType
		if err = k.PatchDaemonSetByLabelAndName(nodeLabel, daemonSetName, patchBytes, patchType); err != nil {
			return fmt.Errorf("could not patch Trident DaemonSet; %v", err)
		}

		Log().Debug("Patched Trident DaemonSet.")
	}

	return nil
}

// DeleteTridentDaemonSet deletes a Trident DaemonSet.
func (k *K8sClient) DeleteTridentDaemonSet(nodeLabel string) error {
	// Delete Trident daemonSets
	if daemonSets, err := k.GetDaemonSetsByLabel(nodeLabel, true); err != nil {
		Log().WithFields(LogFields{
			"label": nodeLabel,
			"error": err,
		}).Errorf("Unable to get list of daemonset by label.")
		return fmt.Errorf("unable to get list of daemonset")
	} else if len(daemonSets) == 0 {
		Log().WithFields(LogFields{
			"label": nodeLabel,
			"error": err,
		}).Warning("Trident daemonset not found.")
	} else {
		if len(daemonSets) == 1 {
			Log().WithFields(LogFields{
				"daemonset": daemonSets[0].Name,
				"namespace": daemonSets[0].Namespace,
			}).Info("Trident daemonSets found by label.")
		} else {
			Log().WithField("label", nodeLabel).Warnf("Multiple daemonSets found matching label; removing all.")
		}

		if err = k.RemoveMultipleDaemonSets(daemonSets); err != nil {
			return err
		}
	}

	return nil
}

// RemoveMultipleDaemonSets removes a list of Trident DaemonSet.
func (k *K8sClient) RemoveMultipleDaemonSets(unwantedDaemonSets []appsv1.DaemonSet) error {
	var err error
	var anyError bool
	var undeletedDaemonSets []string

	if len(unwantedDaemonSets) > 0 {
		for _, daemonSetToRemove := range unwantedDaemonSets {
			// Delete the daemonset
			if err = k.DeleteDaemonSet(daemonSetToRemove.Name, daemonSetToRemove.Namespace, false); err != nil {
				Log().WithFields(LogFields{
					"deployment": daemonSetToRemove.Name,
					"namespace":  daemonSetToRemove.Namespace,
					"error":      err,
				}).Warning("Could not delete daemonset.")

				anyError = true
				undeletedDaemonSets = append(undeletedDaemonSets, fmt.Sprintf("%v/%v", daemonSetToRemove.Namespace,
					daemonSetToRemove.Name))
			} else {
				Log().WithFields(LogFields{
					"daemonset": daemonSetToRemove.Name,
					"namespace": daemonSetToRemove.Namespace,
				}).Infof("Deleted Trident daemonset.")
			}
		}
	}

	if anyError {
		return fmt.Errorf("unable to delete daemonset(s): %v", undeletedDaemonSets)
	}

	return nil
}

// GetDeploymentInformation identifies the Operator-based Trident deployment information and any unwanted
// deployments
func (k *K8sClient) GetDeploymentInformation(deploymentName, appLabel, namespace string) (*appsv1.Deployment,
	[]appsv1.Deployment, bool, error,
) {
	createDeployment := true
	var currentDeployment *appsv1.Deployment
	var unwantedDeployments []appsv1.Deployment

	if deployments, err := k.GetDeploymentsByLabel(appLabel, true); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of deployments by label.")
		return nil, nil, true, fmt.Errorf("unable to get list of deployments")
	} else if len(deployments) == 0 {
		Log().WithFields(LogFields{
			"label":     appLabel,
			"namespace": namespace,
		}).Infof("No Trident deployments found by label.")
	} else {
		// Rules:
		// 1. If there is no deployment named trident/trident-csi in CR namespace and one or many other deployment
		//    exist that matches the label then remove all the deployments.
		// 2. If there is a deployment named trident/trident-csi in CR namespace and one or many other deployment
		//    exist that matches the label then remove all other deployments.
		for _, deployment := range deployments {
			if deployment.Namespace == namespace && deployment.Name == deploymentName {
				// Found a deployment named in the same namespace
				Log().WithFields(LogFields{
					"deployment": deployment.Name,
					"namespace":  deployment.Namespace,
				}).Infof("A Trident deployment was found by label")

				// Allocate new memory for currentDeployment to avoid unintentional reassignments due to reuse of the
				// deployment variable across iterations
				currentDeployment = &appsv1.Deployment{}
				*currentDeployment = deployment
				createDeployment = false
			} else {
				Log().WithFields(LogFields{
					"deployment":          deployment.Name,
					"deploymentNamespace": deployment.Namespace,
				}).Errorf("A deployment was found by label which does not meet either name %s or namespace"+
					" '%s' requirement, marking it for deletion.", deploymentName, namespace)

				unwantedDeployments = append(unwantedDeployments, deployment)
			}
		}
	}

	return currentDeployment, unwantedDeployments, createDeployment, nil
}

// PutDeployment creates or updates a Trident Deployment.
func (k *K8sClient) PutDeployment(
	currentDeployment *appsv1.Deployment, createDeployment bool, newDeploymentYAML, appLabel string,
) error {
	deploymentName := getDeploymentName()
	logFields := LogFields{
		"deployment": deploymentName,
		"namespace":  k.Namespace(),
	}

	// Preserve annotations from current deployment
	updatedDeploymentYAML, err := mergeAnnotationsFromExistingDeployment(currentDeployment, newDeploymentYAML)
	if err != nil {
		return fmt.Errorf("could not merge annotations from current deployment %q: %v", deploymentName, err)
	}

	if createDeployment {
		Log().WithFields(logFields).Debug("Creating Trident deployment.")

		// Create the deployment
		if err := k.CreateObjectByYAML(updatedDeploymentYAML); err != nil {
			return fmt.Errorf("could not create Trident deployment; %v", err)
		}

		Log().Info("Created Trident deployment.")
	} else {
		Log().WithFields(logFields).Debug("Patching Trident deployment.")

		// Identify the deltas
		patchBytes, err := k8sclient.GenericPatch(currentDeployment, []byte(updatedDeploymentYAML))
		if err != nil {
			return fmt.Errorf("error in creating the two-way merge patch for current Deployment %q: %v",
				deploymentName, err)
		}

		// Apply the patch to the current deployment
		patchType := types.MergePatchType
		if err := k.PatchDeploymentByLabel(appLabel, patchBytes, patchType); err != nil {
			return fmt.Errorf("could not patch Trident deployment; %v", err)
		}

		Log().Debug("Patched Trident deployment.")
	}

	return nil
}

// DeleteTridentDeployment deletes a Trident Deployment.
func (k *K8sClient) DeleteTridentDeployment(appLabel string) error {
	// Delete Trident deployments
	if deployments, err := k.GetDeploymentsByLabel(appLabel, true); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of deployments by label.")
		return fmt.Errorf("unable to get list of deployments")
	} else if len(deployments) == 0 {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Warn("Trident deployment not found.")
	} else {

		if len(deployments) == 1 {
			Log().WithFields(LogFields{
				"deployment": deployments[0].Name,
				"namespace":  deployments[0].Namespace,
			}).Info("Trident deployment found by label.")
		} else {
			Log().WithField("label", appLabel).Warnf("Multiple deployments found matching label; removing all.")
		}

		if err = k.RemoveMultipleDeployments(deployments); err != nil {
			return err
		}
	}

	return nil
}

// RemoveMultipleDeployments removes a list of unwanted beta CSI drivers in a namespace
func (k *K8sClient) RemoveMultipleDeployments(unwantedDeployments []appsv1.Deployment) error {
	var err error
	var anyError bool
	var undeletedDeployments []string

	if len(unwantedDeployments) > 0 {
		for _, deploymentToRemove := range unwantedDeployments {
			// Delete the deployment
			if err = k.DeleteDeployment(deploymentToRemove.Name, deploymentToRemove.Namespace,
				true); err != nil {
				Log().WithFields(LogFields{
					"deployment": deploymentToRemove.Name,
					"namespace":  deploymentToRemove.Namespace,
					"error":      err,
				}).Errorf("Could not delete deployment.")

				anyError = true
				undeletedDeployments = append(undeletedDeployments, fmt.Sprintf("%v/%v", deploymentToRemove.Namespace,
					deploymentToRemove.Name))
			} else {
				Log().WithFields(LogFields{
					"deployment": deploymentToRemove.Name,
					"namespace":  deploymentToRemove.Namespace,
				}).Infof("Deleted deployment.")
			}
		}
	}

	if anyError {
		return fmt.Errorf("unable to delete deployment(s): %v", undeletedDeployments)
	}

	return nil
}

// GetSecretInformation identifies the Operator-based Trident Secret information.
func (k *K8sClient) GetSecretInformation(secretName, appLabel, namespace string, shouldUpdate bool) (*corev1.Secret,
	[]corev1.Secret, bool, error,
) {
	createSecret := true
	// var currentSecret *v1.Secret
	var unwantedSecrets []corev1.Secret

	if secrets, err := k.GetSecretsByLabel(appLabel, false); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of secrets by label.")
		return nil, nil, true, fmt.Errorf("unable to get list of secrets by label")
	} else if len(secrets) == 0 {
		Log().Info("Trident secret not found.")

		Log().Debug("Deleting unlabeled Trident secret by name as it can cause issues during installation.")
		if err = k.DeleteSecret(secretName, namespace); err != nil {
			if !errors.IsResourceNotFoundError(err) {
				Log().WithField("error", err).Warning("Could not delete Trident secret.")
			}
		} else {
			Log().WithField("Secret", secretName).Info(
				"Deleted Trident secret; replacing it with a labeled Trident secret.")
		}
	} else if shouldUpdate {
		unwantedSecrets = secrets
	} else {
		// 1. If there is no secret named trident-csi in CR namespace and one or many other secrets
		//    exist that matches the label then remove all the secrets.
		// 2. If there is a secret named trident-csi in CR namespace and one or many other secret
		//    exist that matches the label then remove all other secrets.
		for _, secret := range secrets {
			// Check if the secret has our persistent object label and value. If so, don't issue it for deletion it.
			if value, ok := secret.GetLabels()[TridentPersistentObjectLabelKey]; ok {
				if value == TridentPersistentObjectLabelValue {
					Log().WithFields(LogFields{
						"secret":    secret.Name,
						"namespace": secret.Namespace,
						"label":     TridentPersistentObjectLabel,
					}).Info("Retaining Trident secret.")
					continue
				}
			}

			if secret.Namespace == namespace && secret.Name == secretName {
				// Found a secret named trident-csi in the same namespace
				Log().WithFields(LogFields{
					"secret":    secret.Name,
					"namespace": secret.Namespace,
				}).Infof("A Trident secret was found by label.")

				// currentSecret = &secret
				createSecret = false
			} else {
				Log().WithFields(LogFields{
					"secret":          secret.Name,
					"secretNamespace": secret.Namespace,
				}).Errorf("A secret was found by label which does not meet either name %s or namespace '%s"+
					"' requirement, marking it for deletion.", secretName, namespace)

				unwantedSecrets = append(unwantedSecrets, secret)
			}
		}
	}

	// return nil for the currentSecret for now
	return nil, unwantedSecrets, createSecret, nil
}

// PutSecret creates or updates a Secret associated with Trident.
func (k *K8sClient) PutSecret(createSecret bool, newSecretYAML, secretName string) error {
	// Create Secret
	if createSecret {
		Log().WithFields(LogFields{
			"secret":    secretName,
			"namespace": k.Namespace(),
		}).Debug("Creating Trident secret.")

		if err := k.CreateObjectByYAML(newSecretYAML); err != nil {
			return fmt.Errorf("could not create Trident secret; %v", err)
		}

		Log().Debug("Created Trident secret.")
	}

	return nil
}

// DeleteTridentSecret deletes a Secret associated with Trident.
func (k *K8sClient) DeleteTridentSecret(secretName, appLabel, namespace string) error {
	if secrets, err := k.GetSecretsByLabel(appLabel, false); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of secrets by label.")
		return fmt.Errorf("unable to get list of secrets")
	} else if len(secrets) == 0 {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Warning("Trident secret not found.")

		Log().Debug("Deleting unlabeled Trident secret by name as it may have been created outside of the Trident Operator.")
		if err = k.DeleteSecret(secretName, namespace); err != nil {
			if !errors.IsResourceNotFoundError(err) {
				Log().WithField("error", err).Warning("Could not delete Trident secret.")
			}
		} else {
			Log().WithField("Secret", secretName).Info(
				"Deleted Trident secret.")
		}
	} else {
		if len(secrets) == 1 {
			Log().WithFields(LogFields{
				"secret":    secrets[0].Name,
				"namespace": secrets[0].Namespace,
			}).Info("Trident secret found by label.")
		} else {
			Log().WithField("label", appLabel).Warnf("Multiple secrets found matching label; removing all.")
		}

		if err = k.RemoveMultipleSecrets(secrets); err != nil {
			return err
		}
	}

	return nil
}

// RemoveMultipleSecrets removes a list of unwanted secrets in a namespace.
func (k *K8sClient) RemoveMultipleSecrets(unwantedSecrets []corev1.Secret) error {
	var err error
	var anyError bool
	var undeletedSecrets []string

	if len(unwantedSecrets) > 0 {
		for _, secretToRemove := range unwantedSecrets {
			// Check if the secret has our persistent object label and value. If so, don't remove it.
			if value, ok := secretToRemove.GetLabels()[TridentPersistentObjectLabelKey]; ok {
				if value == TridentPersistentObjectLabelValue {
					Log().WithFields(LogFields{
						"secret":    secretToRemove.Name,
						"namespace": secretToRemove.Namespace,
						"label":     TridentPersistentObjectLabel,
					}).Info("Retaining Trident secret.")
					continue
				}
			}

			// Delete the secret
			if err = k.DeleteSecret(secretToRemove.Name, secretToRemove.Namespace); err != nil {
				Log().WithFields(LogFields{
					"secret":    secretToRemove.Name,
					"namespace": secretToRemove.Namespace,
					"error":     err,
				}).Warning("Could not delete secret.")

				anyError = true
				undeletedSecrets = append(undeletedSecrets, fmt.Sprintf("%v/%v", secretToRemove.Namespace,
					secretToRemove.Name))
			} else {
				Log().WithFields(LogFields{
					"secret":    secretToRemove.Name,
					"namespace": secretToRemove.Namespace,
				}).Infof("Deleted secret.")
			}
		}
	}

	if anyError {
		return fmt.Errorf("unable to delete secret(s): %v", undeletedSecrets)
	}

	return nil
}

// GetServiceInformation identifies the Operator-based Trident Service information.
func (k *K8sClient) GetServiceInformation(serviceName, appLabel, namespace string, shouldUpdate bool) (*corev1.Service,
	[]corev1.Service, bool, error,
) {
	createService := true
	var currentService *corev1.Service
	var unwantedServices []corev1.Service

	if services, err := k.GetServicesByLabel(appLabel, true); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of services by label.")
		return nil, nil, true, fmt.Errorf("unable to get list of services")
	} else if len(services) == 0 {
		Log().Info("Trident service not found.")

		Log().Debug("Deleting unlabeled Trident service by name as it can cause issues during installation.")
		if err = k.DeleteService(serviceName, namespace); err != nil {
			if !errors.IsResourceNotFoundError(err) {
				Log().WithField("error", err).Warning("Could not delete Trident service.")
			}
		} else {
			Log().WithField("service", serviceName).Info(
				"Deleted Trident service; replacing it with a labeled Trident service.")
		}
	} else if shouldUpdate {
		unwantedServices = services
	} else {
		// Rules:
		// 1. If there is no service named trident-csi in CR namespace and one or many other services exist
		//    that matches the label then remove all the services.
		// 2. If there is a service named trident-csi in CR namespace and one or many other services exist
		//    that matches the label then remove all other services.
		for _, service := range services {
			if namespace == service.Namespace && service.Name == serviceName {
				// Found a service named trident-csi in the same namespace
				Log().WithFields(LogFields{
					"service":   service.Name,
					"namespace": service.Namespace,
				}).Infof("A Trident service was found by label.")

				// allocate new memory for currentService to avoid unintentional reassignments due to reuse of the
				// service variable across iterations
				currentService = &corev1.Service{}
				*currentService = service
				createService = false
			} else {
				Log().WithFields(LogFields{
					"service":          service.Name,
					"serviceNamespace": service.Namespace,
				}).Errorf("A service was found by label which does not meet either name %s or namespace '%s'"+
					" requirement, marking it for deletion.", serviceName, namespace)

				unwantedServices = append(unwantedServices, service)
			}
		}
	}

	return currentService, unwantedServices, createService, nil
}

// PutService creates or updates a Service associated with Trident.
func (k *K8sClient) PutService(
	currentService *corev1.Service, createService bool, newServiceYAML, appLabel string,
) error {
	serviceName := getServiceName()
	logFields := LogFields{
		"service":   serviceName,
		"namespace": k.Namespace(),
	}

	if createService {
		Log().WithFields(logFields).Debug("Creating Trident service.")

		if err := k.CreateObjectByYAML(newServiceYAML); err != nil {
			return fmt.Errorf("could not create Trident service; %v", err)
		}

		Log().Info("Created Trident service.")
	} else {
		Log().WithFields(logFields).Debug("Patching Trident service.")

		// Identify the deltas
		patchBytes, err := k8sclient.GenericPatch(currentService, []byte(newServiceYAML))
		if err != nil {
			return fmt.Errorf("error in creating the two-way merge patch for current Service %q: %v",
				serviceName, err)
		}

		// Apply the patch to the current Service
		patchType := types.MergePatchType
		if err = k.PatchServiceByLabel(appLabel, patchBytes, patchType); err != nil {
			return fmt.Errorf("could not patch Trident Service; %v", err)
		}

		Log().Debug("Patched Trident service.")
	}

	return nil
}

// DeleteTridentService deletes an Operator-based Service associated with Trident.
func (k *K8sClient) DeleteTridentService(serviceName, appLabel, namespace string) error {
	// Delete Trident services
	if services, err := k.GetServicesByLabel(appLabel, true); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of services by label.")
		return fmt.Errorf("unable to get list of services")
	} else if len(services) == 0 {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Warning("Trident service not found.")

		Log().Debug("Deleting unlabeled Trident service by name as it may have been created outside of the Trident Operator.")
		if err = k.DeleteService(serviceName, namespace); err != nil {
			if !errors.IsResourceNotFoundError(err) {
				Log().WithField("error", err).Warning("Could not delete Trident service.")
			}
		} else {
			Log().WithField("Service", serviceName).Info("Deleted Trident service.")
		}
	} else {
		if len(services) == 1 {
			Log().WithFields(LogFields{
				"service":   services[0].Name,
				"namespace": services[0].Namespace,
			}).Info("Trident service found by label.")
		} else {
			Log().WithField("label", appLabel).Warnf("Multiple services found matching label; removing all.")
		}

		if err = k.RemoveMultipleServices(services); err != nil {
			return err
		}
	}

	return nil
}

// RemoveMultipleServices removes a list of unwanted services in a namespace
func (k *K8sClient) RemoveMultipleServices(unwantedServices []corev1.Service) error {
	var err error
	var anyError bool
	var undeletedServices []string

	if len(unwantedServices) > 0 {
		for _, serviceToRemove := range unwantedServices {
			// Delete the service
			if err = k.DeleteService(serviceToRemove.Name, serviceToRemove.Namespace); err != nil {
				Log().WithFields(LogFields{
					"service":   serviceToRemove.Name,
					"namespace": serviceToRemove.Namespace,
					"error":     err,
				}).Warning("Could not delete service.")

				anyError = true
				undeletedServices = append(undeletedServices, fmt.Sprintf("%v/%v", serviceToRemove.Namespace,
					serviceToRemove.Name))
			} else {
				Log().WithFields(LogFields{
					"service":   serviceToRemove.Name,
					"namespace": serviceToRemove.Namespace,
				}).Infof("Deleted service.")
			}
		}
	}

	if anyError {
		return fmt.Errorf("unable to delete service(s): %v", undeletedServices)
	}

	return nil
}

// GetMultipleServiceAccountInformation identifies the Operator-based Trident Service Account info and any unwanted
// Service Accounts.
func (k *K8sClient) GetMultipleServiceAccountInformation(
	serviceAccountNames []string, appLabel, namespace string, shouldUpdate bool,
) (map[string]*corev1.ServiceAccount,
	[]corev1.ServiceAccount, map[string][]string, map[string]bool, error,
) {
	secretsMap := make(map[string][]string)
	reuseServiceAccountMap := make(map[string]bool)
	currentServiceAccountMap := make(map[string]*corev1.ServiceAccount)
	var currentServiceAccount *corev1.ServiceAccount
	var unwantedServiceAccounts []corev1.ServiceAccount

	if serviceAccounts, err := k.GetServiceAccountsByLabel(appLabel, false); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of service accounts by label.")
		return currentServiceAccountMap, unwantedServiceAccounts, secretsMap, reuseServiceAccountMap,
			fmt.Errorf("unable to get list of service accounts")
	} else if len(serviceAccounts) == 0 {
		Log().Info("Trident service account not found.")

		Log().Debug("Deleting unlabeled Trident service account by name as it can cause issues during installation.")
		for _, accountName := range serviceAccountNames {
			if err = k.DeleteServiceAccount(accountName, namespace, false); err != nil {
				if !errors.IsResourceNotFoundError(err) {
					Log().WithField("error", err).Warning("Could not delete Trident service account.")
				}
			} else {
				Log().WithField("Service Account", accountName).Info(
					"Deleted Trident service account; replacing it with a labeled Trident service account.")
			}
		}
	} else if shouldUpdate {
		unwantedServiceAccounts = serviceAccounts
	} else {
		// Rules:
		// 1. If there are no service accounts matching the allowed name and one or many other
		//    service accounts exist that matches the label then remove all the service accounts.
		// 2. If there are service accounts matching the allowed name and one or many other service
		//    account exist that matches the label then remove all other service accounts.
		for _, serviceAccount := range serviceAccounts {
			if collection.ContainsString(serviceAccountNames, serviceAccount.Name) {
				// Found a service account matching one of the valid names in the same namespace
				Log().WithFields(LogFields{
					"serviceAccount": serviceAccount.Name,
					"namespace":      serviceAccount.Namespace,
				}).Infof("A Trident service account found by label.")

				// allocate new memory for currentServiceAccount to avoid unintentional reassignments due to reuse of the
				// serviceAccount variable across iterations
				currentServiceAccount = &corev1.ServiceAccount{}
				*currentServiceAccount = serviceAccount
				currentServiceAccountMap[serviceAccount.Name] = currentServiceAccount
				reuseServiceAccountMap[serviceAccount.Name] = true

				for _, serviceAccountSecret := range serviceAccount.Secrets {
					secretsMap[serviceAccount.Name] = append(secretsMap[serviceAccount.Name], serviceAccountSecret.Name)
				}
			} else {
				Log().WithField("serviceAccount", serviceAccount.Name).
					Errorf("A Service account was found by label "+
						"but does not meet name '%s' requirement, marking it for deletion.", serviceAccount.Name)

				unwantedServiceAccounts = append(unwantedServiceAccounts, serviceAccount)
			}
		}
	}

	return currentServiceAccountMap, unwantedServiceAccounts, secretsMap, reuseServiceAccountMap, nil
}

// PutServiceAccount creates or updates a Service Account associated with Trident.
func (k *K8sClient) PutServiceAccount(
	currentServiceAccount *corev1.ServiceAccount, reuseServiceAccount bool, newServiceAccountYAML, appLabel string,
) (bool,
	error,
) {
	var serviceAccountName string
	newServiceAccount := false

	if !reuseServiceAccount {
		Log().Debug("Creating Trident service account.")

		if err := k.CreateObjectByYAML(newServiceAccountYAML); err != nil {
			return false, fmt.Errorf("could not create service account; %v", err)
		}
		// set true so code down the line can use this in it's create or update logic
		newServiceAccount = true

		Log().Info("Created service account.")
	} else {
		if currentServiceAccount != nil {
			serviceAccountName = currentServiceAccount.Name
		}

		logFields := LogFields{
			"serviceAccount": serviceAccountName,
			"namespace":      k.Namespace(),
		}
		Log().WithFields(logFields).Debug("Patching Trident Service account.")

		// Identify the deltas
		patchBytes, err := k8sclient.GenericPatch(currentServiceAccount, []byte(newServiceAccountYAML))
		if err != nil {
			return false, fmt.Errorf("error in creating the two-way merge patch for current Service account %q: %v",
				serviceAccountName, err)
		}

		// Apply the patch to the current Service Account
		patchType := types.MergePatchType
		if err = k.PatchServiceAccountByLabelAndName(appLabel, serviceAccountName, patchBytes, patchType); err != nil {
			return false, fmt.Errorf("could not patch service account; %v", err)
		}

		Log().Debug("Patched Trident service account.")
	}

	return newServiceAccount, nil
}

// DeleteMultipleTridentServiceAccounts deletes an Operator-based Service Account associated with Trident.
func (k *K8sClient) DeleteMultipleTridentServiceAccounts(serviceAccountNames []string, appLabel, namespace string) error {
	// Delete service account
	if serviceAccounts, err := k.GetServiceAccountsByLabel(appLabel, false); err != nil {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Errorf("Unable to get list of service accounts by label.")
		return fmt.Errorf("unable to get list of service accounts")
	} else if len(serviceAccounts) == 0 {
		Log().WithFields(LogFields{
			"label": appLabel,
			"error": err,
		}).Warning("Trident service account not found.")

		Log().Debug("Deleting unlabeled Trident service account by name as it may have been created outside of the" +
			" Trident Operator.")
		for _, serviceAccountName := range serviceAccountNames {
			if err = k.DeleteServiceAccount(serviceAccountName, namespace, false); err != nil {
				if !errors.IsResourceNotFoundError(err) {
					Log().WithField("error", err).Warning("Could not delete Trident service account.")
				}
			} else {
				Log().WithField("Service Account", serviceAccountName).Info(
					"Deleted unlabeled Trident service account.")
			}
		}

	} else {
		for idx := range serviceAccounts {
			Log().WithFields(LogFields{
				"serviceAccount": serviceAccounts[idx].Name,
				"namespace":      serviceAccounts[idx].Namespace,
			}).Debug("Trident Service accounts found by label.")
		}

		if err = k.RemoveMultipleServiceAccounts(serviceAccounts); err != nil {
			return err
		}
	}

	return nil
}

// RemoveMultipleServiceAccounts removes a list of unwanted service accounts in a namespace
func (k *K8sClient) RemoveMultipleServiceAccounts(unwantedServiceAccounts []corev1.ServiceAccount) error {
	var err error
	var anyError bool
	var undeletedServiceAccounts []string

	if len(unwantedServiceAccounts) > 0 {
		// Delete the service accounts
		for _, serviceAccountToRemove := range unwantedServiceAccounts {
			if err = k.DeleteServiceAccount(serviceAccountToRemove.Name,
				serviceAccountToRemove.Namespace, true); err != nil {
				Log().WithFields(LogFields{
					"serviceAccount": serviceAccountToRemove.Name,
					"namespace":      serviceAccountToRemove.Namespace,
					"error":          err,
				}).Errorf("Could not delete service account.")

				anyError = true
				undeletedServiceAccounts = append(undeletedServiceAccounts,
					fmt.Sprintf("%v/%v", serviceAccountToRemove.Namespace,
						serviceAccountToRemove.Name))
			} else {
				Log().WithFields(LogFields{
					"serviceAccount": serviceAccountToRemove.Name,
					"namespace":      serviceAccountToRemove.Namespace,
				}).Infof("Deleted service account.")
			}
		}
	}

	if anyError {
		return fmt.Errorf("unable to delete service account(s): %v", undeletedServiceAccounts)
	}

	return nil
}

// GetMultipleTridentOpenShiftSCCInformation gets OpenShiftSCC info with a supplied name,
// username and determines if new OpenShiftSCC should be created
func (k *K8sClient) GetMultipleTridentOpenShiftSCCInformation(
	openShiftSCCNames, openShiftSCCUserNames []string, shouldUpdate bool,
) (map[string][]byte,
	map[string]bool, map[string]bool, error,
) {
	reuseOpenShiftSCCMap := make(map[string]bool)
	removeExistingSCCMap := make(map[string]bool)
	currentOpenShiftSCCJSONMap := make(map[string][]byte)

	for idx := 0; idx < len(openShiftSCCNames); idx++ {
		logFields := LogFields{"sccUserName": openShiftSCCUserNames[idx], "sccName": openShiftSCCNames[idx]}
		if SCCExist, SCCUserExist, jsonData, err := k.GetOpenShiftSCCByName(openShiftSCCUserNames[idx],
			openShiftSCCNames[idx]); err != nil {
			Log().WithFields(logFields).Errorf("Unable to get OpenShift SCC for Trident; err: %v", err)
			return currentOpenShiftSCCJSONMap, reuseOpenShiftSCCMap, removeExistingSCCMap, fmt.Errorf("unable to get OpenShift SCC for Trident")
		} else if !SCCExist {
			Log().WithFields(logFields).Info("Trident OpenShift SCC not found.")
		} else if !SCCUserExist {
			Log().WithFields(logFields).Info("Trident OpenShift SCC found, but SCC user does not exist.")
			removeExistingSCCMap[openShiftSCCNames[idx]] = true
		} else if shouldUpdate {
			removeExistingSCCMap[openShiftSCCNames[idx]] = true
		} else {
			currentOpenShiftSCCJSONMap[openShiftSCCNames[idx]] = jsonData
			reuseOpenShiftSCCMap[openShiftSCCNames[idx]] = true
		}
	}

	return currentOpenShiftSCCJSONMap, reuseOpenShiftSCCMap, removeExistingSCCMap, nil
}

// PutOpenShiftSCC creates or updates OpenShiftSCCs associated with Trident.
func (k *K8sClient) PutOpenShiftSCC(
	currentOpenShiftSCCJSON []byte,
	reuseOpenShiftSCC bool, newOpenShiftSCCYAML string,
) error {
	openShiftSCCOldUserName := "trident-csi"
	openShiftSCCOldName := "privileged"

	if !reuseOpenShiftSCC {
		Log().Debug("Creating Trident OpenShiftSCCs.")

		// Remove trident user from built-in SCC from previous installation
		if err := k.RemoveTridentUserFromOpenShiftSCC(openShiftSCCOldUserName, openShiftSCCOldName); err != nil {
			Log().WithField("err", err).Debugf("No obsolete Trident SCC found - continuing anyway.")
		}

		if err := k.CreateObjectByYAML(newOpenShiftSCCYAML); err != nil {
			return fmt.Errorf("could not create OpenShift SCC; %v", err)
		}

		Log().Info("Created OpenShift SCC.")
	} else {
		Log().Debug("Patching Trident OpenShift SCC.")

		// Convert new object from YAML to JSON format
		modifiedJSON, err := yaml.YAMLToJSON([]byte(newOpenShiftSCCYAML))
		if err != nil {
			return fmt.Errorf("could not convert new object from YAML to JSON; %v", err)
		}

		// Identify the deltas
		patchBytes, err := jsonpatch.MergePatch(currentOpenShiftSCCJSON, modifiedJSON)
		if err != nil {
			return fmt.Errorf("error in creating the two-way merge patch for OpenShift SCC: %v", err)
		}

		// Apply the patch to the current OpenShift SCC
		if err := k.PatchOpenShiftSCC(patchBytes); err != nil {
			return fmt.Errorf("could not patch Trident OpenShift SCC; %v", err)
		}

		Log().Debug("Patched Trident OpenShift SCC.")
	}

	return nil
}

// DeleteMultipleOpenShiftSCC deletes an Operator-based OpenShiftSCC associated with Trident.
func (k *K8sClient) DeleteMultipleOpenShiftSCC(
	openShiftSCCUserNames, openShiftSCCNames []string,
	appLabelValue string,
) error {
	var removeExistingSCC bool
	var logFields LogFields
	var err error

	// deleting trident SCC if present
	if err = k.DeleteObjectByYAML(k8sclient.GetOpenShiftSCCQueryYAML(getOpenShiftSCCName()), true); err != nil {
		return err
	}

	for idx := 0; idx < len(openShiftSCCNames); idx++ {
		logFields = LogFields{
			"sccUserName": openShiftSCCUserNames[idx],
			"sccName":     openShiftSCCNames[idx],
			"label":       appLabelValue,
		}
		// Delete OpenShift SCC
		if SCCExist, SCCUserExist, _, err := k.GetOpenShiftSCCByName(openShiftSCCUserNames[idx], openShiftSCCNames[idx]); err != nil {
			Log().WithFields(logFields).Errorf("Unable to get OpenShift SCC for Trident; err: %v", err)
			return fmt.Errorf("unable to get OpenShift SCC for Trident")
		} else if !SCCExist {
			Log().WithFields(logFields).Info("Trident OpenShift SCC not found.")
		} else if !SCCUserExist {
			Log().WithFields(logFields).Info("Trident OpenShift SCC found, but SCC user does not exist.")
			removeExistingSCC = true
		} else {
			Log().WithFields(logFields).Info("Trident OpenShift SCC and the SCC user found by label.")
			removeExistingSCC = true
		}

		if removeExistingSCC {
			if err = k.DeleteObjectByYAML(k8sclient.GetOpenShiftSCCQueryYAML(openShiftSCCNames[idx]), true); err != nil {
				return err
			}
		}
	}

	// Remove old objects that may have been created pre-20.04
	if err = k.RemoveTridentUserFromOpenShiftSCC("trident-installer", "privileged"); err != nil {
		Log().Debug(err)
	}
	if err = k.RemoveTridentUserFromOpenShiftSCC("trident-csi", "privileged"); err != nil {
		Log().Debug(err)
	}
	if err = k.RemoveTridentUserFromOpenShiftSCC("trident", "anyuid"); err != nil {
		Log().Debug(err)
	}

	return nil
}

// RemoveMultiplePods removes a list of unwanted pods in a namespace
func (k *K8sClient) RemoveMultiplePods(unwantedPods []corev1.Pod) error {
	var err error
	var anyError bool
	var undeletedPods []string

	if len(unwantedPods) > 0 {
		for _, podToRemove := range unwantedPods {
			// Delete the pod
			if err = k.DeletePod(podToRemove.Name, podToRemove.Namespace); err != nil {
				Log().WithFields(LogFields{
					"pod":       podToRemove.Name,
					"namespace": podToRemove.Namespace,
					"error":     err,
				}).Warning("Could not delete pod.")

				anyError = true
				undeletedPods = append(undeletedPods, fmt.Sprintf("%v/%v", podToRemove.Namespace,
					podToRemove.Name))
			} else {
				Log().WithFields(LogFields{
					"pod":       podToRemove.Name,
					"namespace": podToRemove.Namespace,
				}).Infof("Deleted pod.")
			}
		}
	}

	if anyError {
		return fmt.Errorf("unable to delete pod(s): %v", undeletedPods)
	}

	return nil
}

// DeleteTransientVersionPod deletes the temporary version pod used by the operator
func (k *K8sClient) DeleteTransientVersionPod(versionPodLabel string) error {
	var unwantedPods []corev1.Pod
	var err error
	if pods, err := k.GetPodsByLabel(versionPodLabel, true); err != nil {
		Log().WithFields(LogFields{
			"label":    appLabel,
			"podLabel": versionPodLabel,
			"error":    err,
		}).Errorf("Unable to get list of version pods by label.")
		return fmt.Errorf("unable to get list of version pods")
	} else if len(pods) == 0 {
		Log().Debug("transient version pod not found.")
		return nil
	} else {
		unwantedPods = pods
	}

	if err = k.RemoveMultiplePods(unwantedPods); err != nil {
		return err
	}
	return nil
}

// DeleteTridentNodeRemediationResources removes the resources used for node remediation.
func (k *K8sClient) DeleteTridentNodeRemediationResources(namespace string) error {
	yaml := k8sclient.GetNodeRemediationTemplateYAML(namespace)
	if err := k.DeleteObjectByYAML(yaml, true); err != nil {
		if !errors.IsResourceNotFoundError(err) {
			Log().WithError(err).
				Warning("Could not delete TridentNodeRemediationTemplate CR.")
			return fmt.Errorf("could not delete TridentNodeRemediationTemplate CR: %v", err)
		}
	}

	yaml = k8sclient.GetNodeRemediationClusterRoleYAML()
	if err := k.DeleteObjectByYAML(yaml, true); err != nil {
		if !errors.IsResourceNotFoundError(err) {
			Log().WithError(err).
				Warning("Could not delete trident-node-remediation-access clusterRole.")
			return fmt.Errorf("could not delete tridentnoderemediation-access clusterRole: %v", err)
		}
	}
	return nil
}

// ExecPodForVersionInformation takes the pod name and command to execute the command into the container matching the
// pod name
func (k *K8sClient) ExecPodForVersionInformation(podName string, cmd []string, timeout time.Duration) ([]byte, error) {
	if len(cmd) == 0 {
		return nil, fmt.Errorf("no command supplied")
	}

	var execOutput []byte

	checkExecSuccessful := func() error {
		output, execError := k.Exec(podName, "", cmd)
		if execError != nil {
			return fmt.Errorf("exec error; %v", execError)
		}

		execOutput = output
		return nil
	}

	execNotify := func(err error, duration time.Duration) {
		Log().WithFields(LogFields{
			"increment": duration,
			"message":   err.Error(),
		}).Debugf("Unable to get version information from the Trident version pod yet, waiting.")
	}

	execBackoff := backoff.NewExponentialBackOff()
	execBackoff.MaxElapsedTime = timeout

	Log().Infof("Waiting for Trident version pod to provide information.")

	if err := backoff.RetryNotify(checkExecSuccessful, execBackoff, execNotify); err != nil {
		errMessage := fmt.Sprintf("Trident version pod was unable to provide information after %3."+
			"2f seconds; err: %v", timeout.Seconds(), err)

		Log().Error(errMessage)
		return []byte{}, err
	}

	Log().WithFields(LogFields{
		"pod": podName,
	}).Infof("Trident version pod started.")

	return execOutput, nil
}

// mergeAnnotationsFromExistingDeployment preserves annotations from an existing deployment into a new deployment
// during trident-controller upgrade.
func mergeAnnotationsFromExistingDeployment(existingDeployment *appsv1.Deployment, newDeploymentYAML string) (string,
	error) {
	if existingDeployment == nil {
		return newDeploymentYAML, nil
	}

	existingAnnotations := existingDeployment.GetAnnotations()
	if len(existingAnnotations) == 0 {
		return newDeploymentYAML, nil
	}

	// Parse new deployment YAML into deployment object
	var newDeployment appsv1.Deployment
	if err := yaml.Unmarshal([]byte(newDeploymentYAML), &newDeployment); err != nil {
		return "", fmt.Errorf("failed to unmarshal new deployment yaml: %w", err)
	}

	if newDeployment.Annotations == nil {
		newDeployment.Annotations = existingAnnotations
	} else {
		for annotationKey, annotationValue := range existingAnnotations {
			if _, exists := newDeployment.Annotations[annotationKey]; !exists {
				newDeployment.Annotations[annotationKey] = annotationValue
			}
		}
	}

	// Convert updated new deployment object back to YAML
	updatedNewDeploymentYAMLBytes, err := yaml.Marshal(newDeployment)
	if err != nil {
		return "", fmt.Errorf("failed to marshal updated deployment: %w", err)
	}

	return string(updatedNewDeploymentYAMLBytes), nil
}
