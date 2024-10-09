// Copyright 2024 NetApp, Inc. All Rights Reserved.

package instruction

import (
	"github.com/netapp/trident/internal/nodeprep/packagemanager"
	"github.com/netapp/trident/internal/nodeprep/packagemanager/apt"
	"github.com/netapp/trident/internal/nodeprep/packagemanager/yum"
	"github.com/netapp/trident/internal/nodeprep/step"
	"github.com/netapp/trident/internal/nodeprep/systemmanager"
	"github.com/netapp/trident/internal/nodeprep/systemmanager/amzn"
	"github.com/netapp/trident/internal/nodeprep/systemmanager/debian"
)

type ISCSI struct {
	Default
}

func newDebianAptISCSI() (instruction Instructions) {
	return newISCSI(apt.New(), debian.New())
}

func newAmznYumISCSI() (instruction Instructions) {
	return newISCSI(yum.New(), amzn.New())
}

func newYumISCSI() (instruction Instructions) {
	return newISCSI(yum.New(), amzn.New())
}

func newAptISCSI() (instruction Instructions) {
	return newISCSI(apt.New(), debian.New())
}

func newISCSI(packageManager packagemanager.PackageManager, systemManager systemmanager.SystemManager) (instruction *ISCSI) {
	instruction = &ISCSI{}
	instruction.name = "iscsi instructions"
	// ordering of steps matter here, multipath must be configured before installing iscsi tools to be idempotent
	instruction.steps = []step.Step{
		step.NewMultipathConfigureStep(packageManager),
		step.NewInstallIscsiTools(packageManager),
		step.NewEnableIscsiServices(systemManager),
	}
	return
}
