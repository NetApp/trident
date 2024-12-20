package utils

import (
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/fcp"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/osutils"
)

var (
	command   = exec.NewCommand()
	FcpUtils  = fcp.NewReconcileUtils(osutils.ChrootPathPrefix, osutils.New())
	fcpClient = fcp.NewDetailed(osutils.ChrootPathPrefix, command, fcp.DefaultSelfHealingExclusion, osutils.New(),
		devices.New(), filesystem.New(mountClient), mountClient, FcpUtils)
)
