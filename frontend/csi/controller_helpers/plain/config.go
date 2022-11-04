// Copyright 2022 NetApp, Inc. All Rights Reserved.
package plain

import (
	csiConfig "github.com/netapp/trident/frontend/csi"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
)

var features = map[controllerhelpers.Feature]bool{
	csiConfig.ExpandCSIVolumes: true,
	csiConfig.CSIBlockVolumes:  true,
}
