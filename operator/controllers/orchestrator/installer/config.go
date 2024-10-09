// Copyright 2023 NetApp, Inc. All Rights Reserved.

package installer

import (
	v1 "k8s.io/api/core/v1"

	"github.com/netapp/trident/config"
)

var (
	DefaultTridentVersion = config.DefaultOrchestratorVersion

	DefaultTridentRepo = "trident:" // Default repo from where to pull Trident Image

	// TridentImage is the image that the operator installs by default
	TridentImage = DefaultTridentRepo + DefaultTridentVersion

	// Version labels are stored in the objects created by the CR

	TridentVersionLabelKey   = "trident_version"
	TridentVersionLabelValue = "v" + DefaultTridentVersion // need to append 'v', otherwise label freaks out
	// TridentVersionLabel      = TridentVersionLabelKey + "=" + TridentVersionLabelValue
)

const (
	TridentAppLabelKey = "app"

	TridentCSILabelKey   = TridentAppLabelKey
	TridentCSILabelValue = "controller.csi.trident.netapp.io"
	TridentCSILabel      = TridentCSILabelKey + "=" + TridentCSILabelValue

	TridentPersistentObjectLabelKey   = "object"
	TridentPersistentObjectLabelValue = "persistent.trident.netapp.io"
	TridentPersistentObjectLabel      = TridentPersistentObjectLabelKey + "=" + TridentPersistentObjectLabelValue

	// Constants used for various resource names

	TridentCSI        = "trident-csi"
	TridentCSIWindows = "trident-csi-windows"
	OpenShiftSCCName  = "trident"

	TridentControllerResourceName  = "trident-controller"
	TridentNodeLinuxResourceName   = "trident-node-linux"
	TridentNodeWindowsResourceName = "trident-node-windows"

	TridentEncryptionKeys = "trident-encryption-keys"

	CSIDriver = "csi.trident.netapp.io"

	TridentNodeLabelKey   = TridentAppLabelKey
	TridentNodeLabelValue = "node.csi.trident.netapp.io"
	TridentNodeLabel      = TridentNodeLabelKey + "=" + TridentNodeLabelValue

	TridentVersionPodLabelKey   = TridentAppLabelKey
	TridentVersionPodLabelValue = "pod.version.trident.netapp.io"
	TridentVersionPodLabel      = TridentVersionPodLabelKey + "=" + TridentVersionPodLabelValue

	// K8sVersionLabelKey is the key stored in the objects created by the CR, value is decided based on the K8s version
	K8sVersionLabelKey = "k8s_version"

	TridentContainer = "trident-main"

	// DefaultLogFormat is the Trident logging format (text, json)
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
