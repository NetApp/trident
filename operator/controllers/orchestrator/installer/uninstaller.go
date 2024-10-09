// Copyright 2023 NetApp, Inc. All Rights Reserved.

package installer

import (
	"fmt"

	"github.com/netapp/trident/cli/cmd"
	k8sclient "github.com/netapp/trident/cli/k8s_client"
	. "github.com/netapp/trident/logging"
)

func (i *Installer) UninstallTrident() error {
	appLabel = TridentCSILabel
	appLabelKey = TridentCSILabelKey
	appLabelValue = TridentCSILabelValue

	nodeLabel := TridentNodeLabel

	// First handle the deployment
	if err := i.client.DeleteTridentDeployment(appLabel); err != nil {
		return fmt.Errorf("could not delete Trident CSI deployment; %v", err)
	}

	// Next handle all the other common CSI components (daemonset, service).  Some/all of these may
	// not be present if uninstalling legacy Trident or preview CSI Trident, in which case we log
	// warnings only.
	if err := i.client.DeleteTridentDaemonSet(nodeLabel); err != nil {
		return fmt.Errorf("could not delete Trident daemonset; %v", err)
	}

	if err := i.client.DeleteTridentResourceQuota(nodeLabel); err != nil {
		return fmt.Errorf("could not delete Trident resource quota; %v", err)
	}

	if err := i.client.DeleteTridentService(getServiceName(), appLabel, i.namespace); err != nil {
		return fmt.Errorf("could not delete Trident service; %v", err)
	}

	if err := i.client.DeleteTridentSecret(getProtocolSecretName(), appLabel, i.namespace); err != nil {
		return fmt.Errorf("could not delete Trident secret; %v", err)
	}

	csiDriverName := getCSIDriverName()
	if err := i.client.DeleteCSIDriverCR(csiDriverName, appLabel); err != nil {
		return fmt.Errorf("could not delete Trident CSI driver custom resource; %v", err)
	}

	// Delete Trident RBAC objects
	if err := i.removeRBACObjects(); err != nil {
		return fmt.Errorf("could not delete all Trident's RBAC objects; %v", err)
	}

	Log().Info("The uninstaller did not delete Trident's namespace in case it is going to be reused.")

	return nil
}

// removeRBACObjects removes any ClusterRoleBindings, ClusterRoles,
// ServicesAccounts and OpenShiftSCCs associated with legacy Trident or Trident-CSI.
func (i *Installer) removeRBACObjects() error {
	var controllerResourceNames, nodeResourceNames []string

	controllerResourceNames = []string{getControllerRBACResourceName()}
	nodeResourceNames = []string{getNodeRBACResourceName(false), getNodeRBACResourceName(true)}

	// Delete controller cluster role
	if err := i.client.DeleteTridentClusterRole(controllerResourceNames[0], TridentCSILabel); err != nil {
		return err
	}

	// Delete controller cluster role binding
	if err := i.client.DeleteTridentClusterRoleBinding(controllerResourceNames[0], TridentCSILabel); err != nil {
		return err
	}

	// Delete node cluster roles
	// Delete node cluster role bindings
	for _, nodeResourceName := range nodeResourceNames {
		if err := i.client.DeleteTridentClusterRole(nodeResourceName, TridentNodeLabel); err != nil {
			return err
		}
		if err := i.client.DeleteTridentClusterRoleBinding(nodeResourceName, TridentNodeLabel); err != nil {
			return err
		}
	}

	// Delete node role
	if err := i.client.DeleteMultipleTridentRoles(nodeResourceNames, TridentNodeLabel); err != nil {
		return err
	}

	// Delete node role binding
	if err := i.client.DeleteMultipleTridentRoleBindings(nodeResourceNames, TridentNodeLabel); err != nil {
		return err
	}

	// Delete controller service account
	if err := i.client.DeleteMultipleTridentServiceAccounts(controllerResourceNames, TridentCSILabel, i.namespace); err != nil {
		return err
	}

	// Delete node service account
	if err := i.client.DeleteMultipleTridentServiceAccounts(nodeResourceNames, TridentNodeLabel, i.namespace); err != nil {
		return err
	}

	// If OpenShift, delete Trident Security Context Constraint(s)
	if i.client.Flavor() == k8sclient.FlavorOpenShift {
		if err := i.client.DeleteMultipleOpenShiftSCC(controllerResourceNames, controllerResourceNames,
			TridentCSILabel); err != nil {
			return err
		}
		if err := i.client.DeleteMultipleOpenShiftSCC(nodeResourceNames, nodeResourceNames,
			TridentNodeLabel); err != nil {
			return err
		}
	}

	return nil
}

func (i *Installer) ObliviateCRDs() error {
	return cmd.ObliviateCRDs(i.client, i.tridentCRDClient, k8sTimeout)
}
