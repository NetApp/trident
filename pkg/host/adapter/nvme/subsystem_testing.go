// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

// NewPathForTest builds a Path without reading the host, so that callers in other packages can fake
// services that return subsystems in their tests. It must not be used in production code.
func NewPathForTest(name, address string, state PathState) *Path {
	return &Path{name: name, address: address, state: state}
}

// NewSubsystemForTest builds a Subsystem without reading the host, so that callers in other packages
// can fake services that return subsystems in their tests. It must not be used in production code.
func NewSubsystemForTest(nqn string, paths ...*Path) *Subsystem {
	return &Subsystem{nqn: nqn, paths: paths}
}
