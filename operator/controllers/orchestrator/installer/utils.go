package installer

func getServiceAccountName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

func getRBACResourceNames() []string {
	names := []string{
		TridentCSIController,
		TridentCSILinuxNode,
	}

	if windows {
		names = append(names, TridentCSIWindowsNode)
	}

	return names
}

func getClusterRoleName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

func getClusterRoleBindingName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

func getPSPName() string {
	return TridentPSP
}

func getServiceName() string {
	return TridentCSI
}

func getProtocolSecretName() string {
	return TridentCSI
}

func getEncryptionSecretName() string {
	return TridentEncryptionKeys
}

func getResourceQuotaName() string {
	return TridentCSI
}

func getControllerRBACResourceName(csi bool) string {
	if csi {
		return TridentCSIController
	}
	return TridentLegacy
}

func getNodeRBACResourceName(windows bool) string {
	if windows {
		return TridentCSIWindowsNode
	}
	return TridentCSILinuxNode
}

func getDeploymentName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

func getDaemonSetName(windows bool) string {
	if windows {
		return TridentCSIWindows
	} else {
		return TridentCSI
	}
}

func getCSIDriverName() string {
	return CSIDriver
}

func getOpenShiftSCCUserName() string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

func getOpenShiftSCCName() string {
	return OpenShiftSCCName
}
