// Copyright 2024 NetApp, Inc. All Rights Reserved.

package nodeinfo

//go:generate mockgen -destination=../../../mocks/mock_internal/mock_nodeprep/mock_nodeinfo/mock_os.go  github.com/netapp/trident/internal/nodeprep/nodeinfo OS
//go:generate mockgen -destination=../../../mocks/mock_internal/mock_nodeprep/mock_nodeinfo/mock_binary.go github.com/netapp/trident/internal/nodeprep/nodeinfo  Binary

import (
	"context"

	"github.com/netapp/trident/internal/chwrap"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/models"
)

// the code in this file need to live under sub packages of utils.
// TODO remove this file once the refactoring is done.

type OS interface {
	GetHostSystemInfo(ctx context.Context) (*models.HostSystem, error)
}

type Binary interface {
	FindPath(binaryName string) string
}

type OSClient struct{}

func NewOSClient() *OSClient {
	return &OSClient{}
}

func (o *OSClient) GetHostSystemInfo(ctx context.Context) (*models.HostSystem, error) {
	return utils.GetHostSystemInfo(ctx)
}

type BinaryClient struct{}

func NewBinary() *BinaryClient {
	return &BinaryClient{}
}

func (b *BinaryClient) FindPath(binaryName string) string {
	_, fullPath := chwrap.FindBinary(binaryName)
	return fullPath
}
