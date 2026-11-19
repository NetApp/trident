// Copyright 2026 NetApp, Inc. All Rights Reserved.

package luks

import (
	"fmt"

	"github.com/netapp/trident/pkg/host/adapter/block"
	"github.com/netapp/trident/utils/models"
)

// deviceType is a Trident-domain device type for devices managed by the LUKS service.
// All LUKS service Device models
// Do not change it.
const deviceType = models.BlockDeviceCrypt

// Device is a LUKS service model for a block.Device.
// It has a block.DMDevice representation on the host.
type Device struct {
	block *block.Device
	node  *block.DMDevice
	kind  models.BlockDeviceType
	// freshFormat is set when the EnsureDevice call that returned this Device ran luksFormat.
	freshFormat bool
}

// newDevice constructs a Device handle. No I/O is performed here.
func newDevice(bd *block.Device, dm *block.DMDevice) *Device {
	return &Device{
		block: bd,
		node:  dm,
		kind:  deviceType,
	}
}

func (d *Device) ID() string                   { return d.node.UUID() }
func (d *Device) Type() models.BlockDeviceType { return d.kind }
func (d *Device) Name() string                 { return d.block.Name() }
func (d *Device) Path() string                 { return d.block.Path() }
func (d *Device) MapperName() string           { return d.node.Name() }
func (d *Device) MapperPath() string           { return block.MapperPath(d.node.Name()) }
func (d *Device) IsFreshFormat() bool          { return d.freshFormat }
func (d *Device) SourceID() string             { return d.block.RuntimeID() }
func (d *Device) Size() uint64                 { return d.block.Size() }
func (d *Device) String() string {
	return fmt.Sprintf("{device: %s, type: %s}", d.block.String(), d.kind)
}

func (d *Device) Info() models.BlockDeviceInfo {
	return models.BlockDeviceInfo{ID: d.node.UUID(), Name: d.node.Name(), Type: d.kind}
}
