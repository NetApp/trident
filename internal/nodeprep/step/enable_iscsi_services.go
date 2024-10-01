// Copyright 2024 NetApp, Inc. All Rights Reserved.

package step

import (
	"context"

	"github.com/netapp/trident/internal/nodeprep/systemmanager"
)

type EnableIscsiServices struct {
	DefaultStep
	systemManager systemmanager.SystemManager
}

func NewEnableIscsiServices(systemManager systemmanager.SystemManager) *EnableIscsiServices {
	step := &EnableIscsiServices{}
	step.Name = "enable iSCSI services step"
	step.Required = true
	step.systemManager = systemManager
	return step
}

func (s *EnableIscsiServices) Apply(ctx context.Context) error {
	return s.systemManager.EnableIscsiServices(ctx)
}
