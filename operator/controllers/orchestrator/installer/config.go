// Copyright 2021 NetApp, Inc. All Rights Reserved.

package installer

import (
	v1 "k8s.io/api/core/v1"

	"github.com/netapp/trident/config"
)

var (
	DefaultTridentVersion = config.DefaultOrchestratorVersion

	DefaultTridentRepo = "netapp/trident:" // Default repo from where to pull Trident Image

	// TridentImage is the image that the operator installs by default
	TridentImage = DefaultTridentRepo + DefaultTridentVersion

	// This gets stored in the objects created by the CR
	TridentVersionLabelKey   = "trident_version"
	TridentVersionLabelValue = "v" + DefaultTridentVersion // need to append 'v', otherwise label freaks out
	TridentVersionLabel      = TridentVersionLabelKey + "=" + TridentVersionLabelValue

	// Names of Trident deployment/daemonset objects managed by the operator
	TridentDeploymentName       = TridentControllerResourceName
	TridentLinuxDaemonsetName   = TridentNodeLinuxResourceName
	TridentWindowsDaemonsetName = TridentNodeWindowsResourceName
)

const (
	TridentAppLabelKey = "app"

	TridentLegacyLabelKey   = TridentAppLabelKey
	TridentLegacyLabelValue = "trident.netapp.io"
	TridentLegacyLabel      = TridentLegacyLabelKey + "=" + TridentLegacyLabelValue

	TridentCSILabelKey   = TridentAppLabelKey
	TridentCSILabelValue = "controller.csi.trident.netapp.io"
	TridentCSILabel      = TridentCSILabelKey + "=" + TridentCSILabelValue

	TridentPersistentObjectLabelKey   = "object"
	TridentPersistentObjectLabelValue = "persistent.trident.netapp.io"
	TridentPersistentObjectLabel      = TridentPersistentObjectLabelKey + "=" + TridentPersistentObjectLabelValue

	// Constants used for various resource names
	TridentCSI        = "trident-csi"
	TridentCSIWindows = "trident-csi-windows"
	TridentLegacy     = "trident"
	OpenShiftSCCName  = "trident"

	TridentControllerResourceName  = "trident-controller"
	TridentNodeLinuxResourceName   = "trident-node-linux"
	TridentNodeWindowsResourceName = "trident-node-windows"

	TridentEncryptionKeys = "trident-encryption-keys"

	CSIDriver  = "csi.trident.netapp.io"
	TridentPSP = "tridentpods"

	TridentNodeLabelKey   = TridentAppLabelKey
	TridentNodeLabelValue = "node.csi.trident.netapp.io"
	TridentNodeLabel      = TridentNodeLabelKey + "=" + TridentNodeLabelValue

	TridentVersionPodLabelKey   = TridentAppLabelKey
	TridentVersionPodLabelValue = "pod.version.trident.netapp.io"
	TridentVersionPodLabel      = TridentVersionPodLabelKey + "=" + TridentVersionPodLabelValue

	// This is used to Watch and List for deployment matching Trident CSI and Legacy labels
	LabelSelector = TridentAppLabelKey + " in (" + TridentCSILabelValue + ", " + TridentLegacyLabelValue + ")"

	// This is the key that stored in the objects created by the CR, value is decided based on the K8s version
	K8sVersionLabelKey = "k8s_version"

	ControllerServer = "127.0.0.1:8000"
	TridentContainer = "trident-main"

	// DefaultLogFormat is the the Trident logging format (text, json)
	DefaultLogFormat = "text"

	// DefaultProbePort is Trident's default port for K8S liveness/readiness probes
	DefaultProbePort = "17546"

	// DefaultKubeletDir is the host location of kubelet's internal state
	DefaultKubeletDir = "/var/lib/kubelet"

	CRAPIVersionKey = "apiVersion"
	CRController    = "controller"
	CRKind          = "kind"
	CRName          = "name"
	CRUID           = "uid"

	// DefaultImagePullPolicy is the trident image pull policy.
	DefaultImagePullPolicy = string(v1.PullIfNotPresent)
)
