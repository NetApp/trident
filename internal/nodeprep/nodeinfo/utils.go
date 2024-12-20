// Copyright 2024 NetApp, Inc. All Rights Reserved.

package nodeinfo

//go:generate mockgen -destination=../../../mocks/mock_internal/mock_nodeprep/mock_nodeinfo/mock_binary.go github.com/netapp/trident/internal/nodeprep/nodeinfo  Binary

import (
	"github.com/netapp/trident/internal/chwrap"
)

// the code in this file need to live under sub packages of utils.
// TODO remove this file once the refactoring is done.

type Binary interface {
	FindPath(binaryName string) string
}

type BinaryClient struct{}

func NewBinary() *BinaryClient {
	return &BinaryClient{}
}

func (b *BinaryClient) FindPath(binaryName string) string {
	_, fullPath := chwrap.FindBinary(binaryName)
	return fullPath
}
