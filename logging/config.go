// Copyright 2018 NetApp, Inc. All Rights Reserved.

package logging

import "github.com/netapp/trident/v21/config"

const (
	LogRoot              = "/var/log/" + config.OrchestratorName
	LogRotationThreshold = 10485760 // 10 MB
	MaxLogEntryLength    = 64000
	RandomLogcheckEnvVar = "LOGROTATE_FREQUENCY"
)

var (
	randomLogcheckInterval = 20
)
