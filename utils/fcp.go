package utils

import "github.com/netapp/trident/utils/fcp"

var (
	FcpUtils  = fcp.NewReconcileUtils(chrootPathPrefix, NewOSClient())
	fcpClient = fcp.NewDetailed(chrootPathPrefix, command, fcp.DefaultSelfHealingExclusion, NewOSClient(),
		NewDevicesClient(), NewFilesystemClient(), mountClient, FcpUtils)
)
