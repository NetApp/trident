// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

// Namespace is a namespace head device of a subsystem that is visible on this host.
// A head device name matches nvme<subsystem>n<namespace>, for example "nvme0n1". With native NVMe
// multipath this is the one device for all paths. The hidden per-controller devices
// (nvme0c0n1) are not reported.
type Namespace struct {
	name string // Block device name. Example: "nvme0n1"
	uuid string // Namespace UUID (sysfs: uuid), "" if it could not be read.
}

func (n *Namespace) Name() string { return n.name }
func (n *Namespace) UUID() string { return n.uuid }
