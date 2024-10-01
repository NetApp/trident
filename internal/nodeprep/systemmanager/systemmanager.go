// Copyright 2024 NetApp, Inc. All Rights Reserved.

package systemmanager

import "context"

//go:generate mockgen -destination=../../../mocks/mock_internal/mock_nodeprep/mock_systemmanager/mock_systemmanager.go github.com/netapp/trident/internal/nodeprep/systemmanager SystemManager

type SystemManager interface {
	EnableIscsiServices(context.Context) error
}
