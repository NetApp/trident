// Copyright 2026 NetApp, Inc. All Rights Reserved.

package block

import "errors"

// NewDeviceForTest builds a Device view of /dev/<name> without reading the host, so that callers in other
// packages can fake services that return devices in their tests. It must not be used in production code.
func NewDeviceForTest(name string) *Device {
	return &Device{name: name, path: devicePathPrefix + name}
}

// NewCryptDeviceForTest builds a Device for the dm crypt device /dev/<name> with mapper name dmName and
// dm UUID dmUUID, without reading the host. It must not be used in production code.
func NewCryptDeviceForTest(name, dmName, dmUUID string) *Device {
	dev := NewDeviceForTest(name)
	dev.dmInfo = &DMDevice{uuid: dmUUID, name: dmName}
	return dev
}

// NewMapperDeviceForTest builds a Device for the dm device /dev/<name> with dm UUID dmUUID that sits on slaves,
// without reading the host. It must not be used in production code.
func NewMapperDeviceForTest(name, dmUUID string, slaves ...string) *Device {
	dev := NewDeviceForTest(name)
	dev.dmInfo = &DMDevice{uuid: dmUUID}
	dev.slaves = slaves
	return dev
}

// NewDeviceNotFoundErrorForTest builds the error the adapter returns for a device that is not on the host.
// It must not be used in production code.
func NewDeviceNotFoundErrorForTest(dev string) error {
	return &DeviceNotFoundError{dev: dev, err: errors.New("no such device")}
}

// NewSizedDeviceForTest builds a Device view of /dev/<name> that reports sizeBytes, without reading the host.
// It must not be used in production code.
func NewSizedDeviceForTest(name string, sizeBytes uint64) *Device {
	dev := NewDeviceForTest(name)
	dev.size = sizeBytes
	return dev
}
