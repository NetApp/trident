// Copyright 2026 NetApp, Inc. All Rights Reserved.

package nvme

// Subsystem is a read-only view of an NVMe subsystem connected to this host, read from sysfs.
// Subsystems are always passed by pointer across the adapter boundary; a nil *Subsystem means
// "no such subsystem." The view is a snapshot; read it again to see changes.
type Subsystem struct {
	nqn        string       // Subsystem NQN (sysfs: subsysnqn).
	dir        string       // Sysfs directory of the subsystem. Example: "/sys/class/nvme-subsystem/nvme-subsys0"
	paths      []*Path      // Controllers connected to this subsystem.
	namespaces []*Namespace // Namespace head devices visible on this host.
}

func (s *Subsystem) NQN() string { return s.nqn }
func (s *Subsystem) Dir() string { return s.dir }

// Paths returns a copy of the subsystem's paths.
func (s *Subsystem) Paths() []*Path {
	paths := make([]*Path, len(s.paths))
	copy(paths, s.paths)
	return paths
}

// Namespaces returns a copy of the subsystem's namespaces.
func (s *Subsystem) Namespaces() []*Namespace {
	namespaces := make([]*Namespace, len(s.namespaces))
	copy(namespaces, s.namespaces)
	return namespaces
}

// HasLivePath reports whether at least one path of the subsystem is live.
func (s *Subsystem) HasLivePath() bool {
	for _, p := range s.paths {
		if p.IsLive() {
			return true
		}
	}
	return false
}
