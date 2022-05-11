package installer

// getServiceAccountName returns the name of the service account to use for Trident
// Parameters:
//   csi - if true, use the Trident CSI service account name; otherwise, use the legacy Trident service account name
// Example:
//   getServiceAccountName(true)
// Returns:
//   string - the service account name to use for Trident

func getServiceAccountName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

// getClusterRoleName returns the name of the cluster role to use for the given CSI flag
// Parameters:
//   csi - true if Trident should use the CSI driver
// Returns:
//   string - the name of the cluster role to use
// Example:
//   name := getClusterRoleName(true)

func getClusterRoleName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

// getClusterRoleBindingName returns the name of the cluster role binding to use
// Parameters:
//   csi - true if Trident should use the CSI driver
// Example:
//   getClusterRoleBindingName(true)
//   returns "trident-csi"

func getClusterRoleBindingName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

// getPSPName returns the name of the PSP to use for Trident
// Example:
//     trident-psp

func getPSPName() string {
	return TridentPSP
}

// getServiceName returns the name of the service
// Example:
//   kubectl get service
//   NAME              TYPE        CLUSTER-IP     EXTERNAL-IP   PORT(S)    AGE
//   trident-csi       NodePort    10.100.200.1   <none>        8080/TCP   1d

func getServiceName() string {
	return TridentCSI
}

// getProtocolSecretName returns the name of the secret that contains the protocol
// information for the Trident CSI driver.
// Example:
//   trident-csi-secret

func getProtocolSecretName() string {
	return TridentCSI
}

// getEncryptionSecretName returns the name of the encryption secret
// Example:
//   getEncryptionSecretName()
//   -> "trident-encryption-keys"

func getEncryptionSecretName() string {
	return TridentEncryptionKeys
}

// getDeploymentName returns the name of the Trident deployment
// Parameters:
//   csi - true if the deployment is for the CSI driver
// Returns:
//   string - the name of the Trident deployment
// Example:
//   deploymentName := getDeploymentName(true)

func getDeploymentName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

// getDaemonSetName returns the name of the Trident CSI DaemonSet.
// Example:
//   - trident-csi

func getDaemonSetName() string {
	return TridentCSI
}

// getCSIDriverName returns the name of the CSI driver
// Example:
//   getCSIDriverName() returns "trident"

func getCSIDriverName() string {
	return CSIDriver
}

// getOpenShiftSCCUserName returns the name of the SCC user that Trident should be
// added to.
// Example:
//    TridentCSI
//    TridentLegacy

func getOpenShiftSCCUserName() string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

// getOpenShiftSCCName returns the name of the OpenShift SCC
// Example:
//   getOpenShiftSCCName() -> "privileged"

func getOpenShiftSCCName() string {
	return OpenShiftSCCName
}
