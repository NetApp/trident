package concurrent_cache

import (
	"github.com/netapp/trident/storage"
)

// SubordinateVolume queries

func ListSubordinateVolumes() Subquery {
	return Subquery{
		res:        subordinateVolume,
		op:         list,
		setResults: listSubordinateVolumesSetResults(func(_ *storage.Volume) bool { return true }),
	}
}

func ListSubordinateVolumesForVolume(volumeID string) Subquery {
	return Subquery{
		res: subordinateVolume,
		op:  list,
		setResults: listSubordinateVolumesSetResults(func(v *storage.Volume) bool {
			if v.IsSubordinate() && v.Config.ShareSourceVolume == volumeID {
				return true
			}
			return false
		}),
	}
}

func listSubordinateVolumesSetResults(filter func(*storage.Volume) bool) func(*Subquery, *Result) error {
	return func(_ *Subquery, r *Result) error {
		subordinateVolumes.rlock()
		r.SubordinateVolumes = make([]*storage.Volume, 0, len(subordinateVolumes.data))
		for k := range subordinateVolumes.data {
			subordinateVolume := subordinateVolumes.data[k].SmartCopy().(*storage.Volume)
			if filter(subordinateVolume) {
				r.SubordinateVolumes = append(r.SubordinateVolumes, subordinateVolume)
			}
		}
		subordinateVolumes.runlock()
		return nil
	}
}

func ReadSubordinateVolume(id string) Subquery {
	return Subquery{
		res: subordinateVolume,
		op:  read,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			subordinateVolumes.rlock()
			if i, ok := subordinateVolumes.data[s.id]; ok {
				r.SubordinateVolume.Read = i.SmartCopy().(*storage.Volume)
			}
			subordinateVolumes.runlock()
			return nil
		},
	}
}

func InconsistentReadSubordinateVolume(id string) Subquery {
	return Subquery{
		res: subordinateVolume,
		op:  inconsistentRead,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			subordinateVolumes.rlock()
			if i, ok := subordinateVolumes.data[s.id]; ok {
				r.SubordinateVolume.Read = i.SmartCopy().(*storage.Volume)
			}
			subordinateVolumes.runlock()
			return nil
		},
	}
}

func UpsertSubordinateVolume(id, volumeID string) Subquery {
	return Subquery{
		res: subordinateVolume,
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
			subordinateVolumes.rlock()
			if i, ok := subordinateVolumes.data[s.id]; ok {
				r.SubordinateVolume.Read = i.SmartCopy().(*storage.Volume)
			}
			subordinateVolumes.runlock()
			r.SubordinateVolume.Upsert = func(v *storage.Volume) {
				subordinateVolumes.lock()
				subordinateVolumes.data[s.id] = v
				subordinateVolumes.unlock()
			}
			return nil
		},
	}
}

func DeleteSubordinateVolume(id string) Subquery {
	return Subquery{
		res: subordinateVolume,
		op:  del,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			subordinateVolumes.rlock()
			if i, ok := subordinateVolumes.data[s.id]; ok {
				r.SubordinateVolume.Read = i.SmartCopy().(*storage.Volume)
			}
			subordinateVolumes.runlock()
			r.SubordinateVolume.Delete = func() {
				subordinateVolumes.lock()
				delete(subordinateVolumes.data, s.id)
				subordinateVolumes.unlock()
			}
			return nil
		},
	}
}
