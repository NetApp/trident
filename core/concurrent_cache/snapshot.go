package concurrent_cache

import (
	"github.com/netapp/trident/storage"
)

// Snapshot queries

func ListSnapshots() Subquery {
	return Subquery{
		res:        snapshot,
		op:         list,
		setResults: listSnapshotsSetResults(func(_ *storage.Snapshot) bool { return true }),
	}
}

func ListSnapshotsByName(snapshotName string) Subquery {
	return Subquery{
		res: snapshot,
		op:  list,
		setResults: listSnapshotsSetResults(func(s *storage.Snapshot) bool {
			return s.Config.Name == snapshotName
		}),
	}
}

func ListSnapshotsForVolume(volumeName string) Subquery {
	return Subquery{
		res: snapshot,
		op:  list,
		setResults: listSnapshotsSetResults(func(s *storage.Snapshot) bool {
			return s.Config.VolumeName == volumeName
		}),
	}
}

func listSnapshotsSetResults(filter func(*storage.Snapshot) bool) func(*Subquery, *Result) error {
	return func(_ *Subquery, r *Result) error {
		snapshots.rlock()
		r.Snapshots = make([]*storage.Snapshot, 0, len(snapshots.data))
		for k := range snapshots.data {
			snapshot := snapshots.data[k].SmartCopy().(*storage.Snapshot)
			if filter(snapshot) {
				r.Snapshots = append(r.Snapshots, snapshot)
			}
		}
		snapshots.runlock()
		return nil
	}
}

func ReadSnapshot(id string) Subquery {
	return Subquery{
		res: snapshot,
		op:  read,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			snapshots.rlock()
			if i, ok := snapshots.data[s.id]; ok {
				r.Snapshot.Read = i.SmartCopy().(*storage.Snapshot)
			}
			snapshots.runlock()
			return nil
		},
	}
}

func InconsistentReadSnapshot(id string) Subquery {
	return Subquery{
		res: snapshot,
		op:  inconsistentRead,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			snapshots.rlock()
			if i, ok := snapshots.data[s.id]; ok {
				r.Snapshot.Read = i.SmartCopy().(*storage.Snapshot)
			}
			snapshots.runlock()
			return nil
		},
	}
}

func UpsertSnapshot(volumeID, id string) Subquery {
	return Subquery{
		res: snapshot,
		op:  upsert,
		id:  id,
		setDependencyIDs: func(s []Subquery, i int) error {
			if err := checkDependency(s, i, volume); err != nil {
				return err
			}
			s[s[i].dependencies[0]].id = volumeID
			return nil
		},
		setResults: func(s *Subquery, r *Result) error {
			snapshots.rlock()
			if i, ok := snapshots.data[s.id]; ok {
				r.Snapshot.Read = i.SmartCopy().(*storage.Snapshot)
			}
			snapshots.runlock()
			r.Snapshot.Upsert = func(snap *storage.Snapshot) {
				snapshots.lock()
				snapshots.data[s.id] = snap
				snapshots.unlock()
			}
			return nil
		},
	}
}

func DeleteSnapshot(id string) Subquery {
	return Subquery{
		res: snapshot,
		op:  del,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			snapshots.rlock()
			if i, ok := snapshots.data[s.id]; ok {
				r.Snapshot.Read = i.SmartCopy().(*storage.Snapshot)
			}
			snapshots.runlock()
			r.Snapshot.Delete = func() {
				snapshots.lock()
				delete(snapshots.data, s.id)
				snapshots.unlock()
			}
			return nil
		},
	}
}
