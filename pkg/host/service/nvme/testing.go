// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"github.com/netapp/trident/pkg/host/adapter/block"
	"github.com/netapp/trident/pkg/host/adapter/nvme"
)

// NewDeviceForTest builds a Device for namespace nsUUID at /dev/<name> without reading the host, so that
// callers of Service in other packages can fake it in their tests. It must not be used in production code.
func NewDeviceForTest(nqn, nsUUID, name string) *Device {
	return newDevice(block.NewDeviceForTest(name), nvme.NewNamespaceForTest(name, nsUUID), nqn)
}

// NewSizedDeviceForTest is NewDeviceForTest for a namespace device that reports sizeBytes.
func NewSizedDeviceForTest(nqn, nsUUID, name string, sizeBytes uint64) *Device {
	return newDevice(block.NewSizedDeviceForTest(name, sizeBytes), nvme.NewNamespaceForTest(name, nsUUID), nqn)
}
