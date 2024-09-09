// Copyright 2022 NetApp, Inc. All Rights Reserved.

package csi

import controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"

const (
	Version     = "1.1"
	Provisioner = "csi.trident.netapp.io"

	// CSI supported features
	CSIBlockVolumes  controllerhelpers.Feature = "CSI_BLOCK_VOLUMES"
	ExpandCSIVolumes controllerhelpers.Feature = "EXPAND_CSI_VOLUMES"

	// Kubernetes topology labels
	K8sTopologyRegionLabel = "topology.kubernetes.io/region"
)
