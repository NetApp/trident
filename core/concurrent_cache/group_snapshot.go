// Copyright 2026 NetApp, Inc. All Rights Reserved.

package concurrent_cache

import (
	"github.com/netapp/trident/storage"
)

// GroupSnapshot queries
//
// A group snapshot is a parentless (rank 0) resource in the cache. It acts as an umbrella record over its
// constituent snapshots (which live on their own backend -> volume -> snapshot trees); the cache stores only
// the group record, keyed by its ID. Constituent snapshots are locked separately via the snapshot queries.

func ListGroupSnapshots() Subquery {
	return Subquery{
		res:        groupSnapshot,
		op:         list,
		setResults: listGroupSnapshotsSetResults(func(_ *storage.GroupSnapshot) bool { return true }),
	}
}

func listGroupSnapshotsSetResults(filter func(*storage.GroupSnapshot) bool) func(*Subquery, *Result) error {
	return func(_ *Subquery, r *Result) error {
		groupSnapshots.rlock()
		r.GroupSnapshots = make([]*storage.GroupSnapshot, 0, len(groupSnapshots.data))
		for k := range groupSnapshots.data {
			gs := groupSnapshots.data[k].SmartCopy().(*storage.GroupSnapshot)
			if filter(gs) {
				r.GroupSnapshots = append(r.GroupSnapshots, gs)
			}
		}
		groupSnapshots.runlock()
		return nil
	}
}

func ReadGroupSnapshot(id string) Subquery {
	return Subquery{
		res: groupSnapshot,
		op:  read,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			groupSnapshots.rlock()
			if i, ok := groupSnapshots.data[s.id]; ok {
				r.GroupSnapshot.Read = i.SmartCopy().(*storage.GroupSnapshot)
			}
			groupSnapshots.runlock()
			return nil
		},
	}
}

func InconsistentReadGroupSnapshot(id string) Subquery {
	return Subquery{
		res: groupSnapshot,
		op:  inconsistentRead,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			groupSnapshots.rlock()
			if i, ok := groupSnapshots.data[s.id]; ok {
				r.GroupSnapshot.Read = i.SmartCopy().(*storage.GroupSnapshot)
			}
			groupSnapshots.runlock()
			return nil
		},
	}
}

func UpsertGroupSnapshot(id string) Subquery {
	return Subquery{
		res: groupSnapshot,
		op:  upsert,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			groupSnapshots.rlock()
			oldGroupSnapshotData, ok := groupSnapshots.data[s.id]
			if ok {
				r.GroupSnapshot.Read = oldGroupSnapshotData.SmartCopy().(*storage.GroupSnapshot)
			}
			groupSnapshots.runlock()

			r.GroupSnapshot.Upsert = func(gs *storage.GroupSnapshot) {
				groupSnapshots.lock()
				groupSnapshots.data[s.id] = gs
				groupSnapshots.unlock()
			}
			return nil
		},
	}
}

func DeleteGroupSnapshot(id string) Subquery {
	return Subquery{
		res: groupSnapshot,
		op:  del,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			groupSnapshots.rlock()
			gsData, ok := groupSnapshots.data[s.id]
			if ok {
				r.GroupSnapshot.Read = gsData.SmartCopy().(*storage.GroupSnapshot)
			}
			groupSnapshots.runlock()

			r.GroupSnapshot.Delete = func() {
				groupSnapshots.lock()
				delete(groupSnapshots.data, s.id)
				groupSnapshots.unlock()
			}
			return nil
		},
	}
}
