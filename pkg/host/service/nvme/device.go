// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import (
	"fmt"

	"github.com/netapp/trident/pkg/host/adapter/block"
	"github.com/netapp/trident/pkg/host/adapter/nvme"
	"github.com/netapp/trident/utils/models"
)

// deviceType is a Trident-domain device type for devices managed by the NVMe service.
// All nvme service Device models share this value.
// Do not change it.
const deviceType = models.BlockDeviceDisk

// Device is a service model of a block.Device.
// It has an nvme.Namespace representation on the host: the namespace head device (for example nvme0n1)
// that the kernel presents for all paths of the subsystem.
type Device struct {
	block *block.Device
	ns    *nvme.Namespace
	nqn   string
	kind  models.BlockDeviceType
}

// newDevice creates a service model of an NVMe namespace block device.
// It stitches the adapters' block device and namespace models together with the subsystem NQN.
func newDevice(bd *block.Device, ns *nvme.Namespace, nqn string) *Device {
	return &Device{
		block: bd,
		ns:    ns,
		nqn:   nqn,
		kind:  deviceType,
	}
}

// ID returns the namespace UUID, which identifies the device across renames of nvmeXnY.
func (d *Device) ID() string                   { return d.ns.UUID() }
func (d *Device) Type() models.BlockDeviceType { return d.kind }
func (d *Device) Name() string                 { return d.block.Name() }
func (d *Device) Path() string                 { return d.block.Path() }
func (d *Device) Size() uint64                 { return d.block.Size() }
func (d *Device) SourceID() string             { return d.block.RuntimeID() }

// SubsystemNQN returns the NQN of the subsystem that serves the namespace.
func (d *Device) SubsystemNQN() string { return d.nqn }

func (d *Device) String() string {
	return fmt.Sprintf("{device: %s, subsystem: %s, namespace: %s, type: %s}",
		d.block.String(), d.nqn, d.ns.UUID(), d.kind)
}

func (d *Device) Info() models.BlockDeviceInfo {
	return models.BlockDeviceInfo{ID: d.ns.UUID(), Name: d.block.Name(), Type: d.kind}
}
