// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

import "strings"

// PathState is the state of an NVMe controller (path) as the kernel reports it in sysfs.
type PathState string

// Controller states the kernel reports in /sys/class/nvme-subsystem/<subsys>/<ctrl>/state.
// Only live is classified by this package; the rest are carried through as reported.
const (
	PathStateLive       PathState = "live"
	PathStateConnecting PathState = "connecting"
	PathStateDeleting   PathState = "deleting"
	PathStateUnknown    PathState = ""
)

// IsLive reports whether the controller can currently carry I/O.
func (s PathState) IsLive() bool   { return s == PathStateLive }
func (s PathState) String() string { return string(s) }

// Path is one NVMe controller of a subsystem: a single connection from this host to a target address.
type Path struct {
	name      string    // Controller name, the sysfs directory name. Example: "nvme0"
	address   string    // Target transport address (sysfs: traddr). Example: "10.193.108.74" or "nn-0x2000:pn-0x1000"
	port      string    // Target service id (sysfs: trsvcid). Example: "4420"
	transport Transport // Transport (sysfs: transport). Example: "tcp"
	state     PathState // Controller state (sysfs: state). Example: "live"
}

func (p *Path) Name() string         { return p.name }
func (p *Path) Address() string      { return p.address }
func (p *Path) Port() string         { return p.port }
func (p *Path) Transport() Transport { return p.transport }
func (p *Path) State() PathState     { return p.state }
func (p *Path) IsLive() bool         { return p.state.IsLive() }

// parseAddress extracts the transport address and service id from a controller's sysfs address.
//
// The kernel separates the fields with a comma on some hosts and a space on others:
//
//	RHEL:   "traddr=10.193.108.74,trsvcid=4420"
//	Ubuntu: "traddr=10.193.108.74 trsvcid=4420"
//
// A transport without a service id reports no trsvcid. Fields other than traddr and trsvcid
// (for example host_traddr and src_addr) are ignored. A missing field is returned as "".
func parseAddress(address string) (traddr, port string) {
	fields := strings.FieldsFunc(address, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		switch key {
		case "traddr":
			traddr = value
		case "trsvcid":
			port = value
		}
	}
	return traddr, port
}
