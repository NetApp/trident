package utils

import (
	"github.com/netapp/trident/utils/fcp"
	"github.com/netapp/trident/utils/filesystem"
)

var (
	FcpUtils  = fcp.NewReconcileUtils(chrootPathPrefix, NewOSClient())
	fcpClient = fcp.NewDetailed(chrootPathPrefix, command, fcp.DefaultSelfHealingExclusion, NewOSClient(),
		devicesClient, filesystem.New(mountClient), mountClient, FcpUtils)
)
