// Copyright 2024 NetApp, Inc. All Rights Reserved.

package packagemanager

//go:generate mockgen -destination=../../../mocks/mock_nodeprep/mock_packagemanager/mock_packagemanager.go github.com/netapp/trident/internal/nodeprep/packagemanager PackageManager

import "github.com/netapp/trident/internal/nodeprep/packagemanager/yum"

type PackageManager interface {
	MultipathToolsInstalled() bool
}

func Factory() PackageManager {
	return yum.New()
}
