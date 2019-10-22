// Copyright 2019 NetApp, Inc. All Rights Reserved.

package csi

import "github.com/netapp/trident/frontend/csi/helpers"

const (
	Version           = "1.1"
	Provisioner       = "csi.trident.netapp.io"
	LegacyProvisioner = "netapp.io/trident"

	// CSI supported features
	CSIBlockVolumes  helpers.Feature = "CSI_BLOCK_VOLUMES"
	ExpandCSIVolumes helpers.Feature = "EXPAND_CSI_VOLUMES"
)
