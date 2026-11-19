// Copyright 2026 NetApp, Inc. All Rights Reserved.

package luks

import "github.com/netapp/trident/pkg/host/adapter/block"

// NewDeviceForTest builds a Device for the crypt device /dev/<name> with mapper name mapperName and dm UUID
// dmUUID without reading the host, so that callers of Service in other packages can fake it in their tests.
// It must not be used in production code.
func NewDeviceForTest(name, mapperName, dmUUID string) *Device {
	dev := block.NewCryptDeviceForTest(name, mapperName, dmUUID)
	return newDevice(dev, dev.DMInfo())
}
