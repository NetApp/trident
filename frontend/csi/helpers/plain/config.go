// Copyright 2019 NetApp, Inc. All Rights Reserved.
package plain

import (
	csiConfig "github.com/netapp/trident/frontend/csi"
	"github.com/netapp/trident/frontend/csi/helpers"
)

var features = map[helpers.Feature]bool{
	csiConfig.ExpandCSIVolumes: true,
	csiConfig.CSIBlockVolumes:  true,
}
