// Copyright 2024 NetApp, Inc. All Rights Reserved.

package packagemanager

import "context"

//go:generate mockgen -destination=../../../mocks/mock_internal/mock_nodeprep/mock_packagemanager/mock_packagemanager.go github.com/netapp/trident/internal/nodeprep/packagemanager PackageManager

type PackageManager interface {
	MultipathToolsInstalled(context.Context) bool
	InstallIscsiRequirements(ctx context.Context) error
}
