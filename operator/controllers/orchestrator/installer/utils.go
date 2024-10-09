// Copyright 2023 NetApp, Inc. All Rights Reserved.

package installer

func getServiceAccountName() string {
	return TridentCSI
}

func getRBACResourceNames() []string {
	names := []string{
		TridentControllerResourceName,
		TridentNodeLinuxResourceName,
	}

	if windows {
		names = append(names, TridentNodeWindowsResourceName)
	}

	return names
}

func getNodeResourceNames() []string {
	var resourceNames []string
	resourceNames = append(resourceNames, TridentNodeLinuxResourceName)
	if windows {
		resourceNames = append(resourceNames, TridentNodeWindowsResourceName)
	}
	return resourceNames
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

func getControllerRBACResourceName() string {
	return TridentControllerResourceName
}

func getNodeRBACResourceName(windows bool) string {
	if windows {
		return TridentNodeWindowsResourceName
	} else {
		return TridentNodeLinuxResourceName
	}
}

func getDeploymentName() string {
	return TridentControllerResourceName
}

func getDaemonSetName(windows bool) string {
	if windows {
		return TridentNodeWindowsResourceName
	} else {
		return TridentNodeLinuxResourceName
	}
}

func getCSIDriverName() string {
	return CSIDriver
}

func getOpenShiftSCCUserName() string {
	return TridentCSI
}

func getOpenShiftSCCName() string {
	return OpenShiftSCCName
}
