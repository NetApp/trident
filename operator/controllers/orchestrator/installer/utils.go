package installer

func getServiceAccountName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
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

func getDeploymentName(csi bool) string {
	if csi {
		return TridentCSI
	} else {
		return TridentLegacy
	}
}

func getDaemonSetName() string {
	return TridentCSI
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
