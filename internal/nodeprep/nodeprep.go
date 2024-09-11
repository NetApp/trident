// Copyright 2024 NetApp, Inc. All Rights Reserved.

package nodeprep

import (
	. "github.com/netapp/trident/logging"
)

func PrepareNode(requestedProtocols []string) (exitCode int) {
	Log().WithField("requestedProtocols", requestedProtocols).Info("Preparing node")
	defer func() {
		Log().WithField("exitCode", exitCode).Info("Node preparation complete")
	}()

	if len(requestedProtocols) == 0 {
		Log().Info("No protocols requested, exiting")
		return 0
	}

	return 0
}
