// Copyright 2020 NetApp, Inc. All Rights Reserved.

package installer

const (
	MinSupportedTridentVersion = "20.04.0" // ParseDate needs this format but should be used with ToMajorMinorVersion
	MaxSupportedTridentVersion = "20.04.0" // ParseDate needs this format but should be used with ToMajorMinorVersion
	DefaultTridentVersion      = "20.04"   // This will ensure we get the latest dot release

	TridentAppLabelKey = "app"

	TridentLegacyLabelKey   = TridentAppLabelKey
	TridentLegacyLabelValue = "trident.netapp.io"
	TridentLegacyLabel      = TridentLegacyLabelKey + "=" + TridentLegacyLabelValue

	TridentCSILabelKey   = TridentAppLabelKey
	TridentCSILabelValue = "controller.csi.trident.netapp.io"
	TridentCSILabel      = TridentCSILabelKey + "=" + TridentCSILabelValue

	TridentNodeLabelKey   = TridentAppLabelKey
	TridentNodeLabelValue = "node.csi.trident.netapp.io"
	TridentNodeLabel      = TridentNodeLabelKey + "=" + TridentNodeLabelValue

	TridentVersionPodLabelKey   = TridentAppLabelKey
	TridentVersionPodLabelValue = "pod.version.trident.netapp.io"
	TridentVersionPodLabel      = TridentVersionPodLabelKey + "=" + TridentVersionPodLabelValue

	// This is used to Watch and List for deployment matching Trident CSI and Legacy labels
	LabelSelector = TridentAppLabelKey + " in (" + TridentCSILabelValue + ", " + TridentLegacyLabelValue + ")"

	// This gets stored in the objects created by the CR
	TridentVersionLabelKey   = "trident_version"
	TridentVersionLabelValue = "v" + DefaultTridentVersion // need to append 'v', otherwise label freaks out
	TridentVersionLabel      = TridentVersionLabelKey + "=" + TridentVersionLabelValue

	// This is the key that stored in the objects created by the CR, value is decided based on the K8s version
	K8sVersionLabelKey = "k8s_version"

	ControllerServer = "127.0.0.1:8000"
	TridentContainer = "trident-main"

	// TridentImage is the image that the operator installs by default
	TridentImage = "netapp/trident:" + DefaultTridentVersion

	// DefaultImageRegistry is the address/port of an internal image registry.
	DefaultImageRegistry = "quay.io"

	// DefaultLogFormat is the the Trident logging format (text, json)
	DefaultLogFormat = "text"

	// DefaultKubeletDir is the host location of kubelet's internal state
	DefaultKubeletDir = "/var/lib/kubelet"

	CRAPIVersionKey = "apiVersion"
	CRController    = "controller"
	CRKind          = "kind"
	CRName          = "name"
	CRUID           = "uid"
)
