package concurrent_cache

import (
	"fmt"

	"github.com/netapp/trident/storage"
)

// Volume queries

func ListVolumes() Subquery {
	return Subquery{
		res:        volume,
		op:         list,
		setResults: listVolumesSetResults(func(_ *storage.Volume) bool { return true }),
	}
}

func ListVolumesForBackend(backendID string) Subquery {
	return Subquery{
		res: volume,
		op:  list,
		setResults: listVolumesSetResults(func(v *storage.Volume) bool {
			return v.BackendUUID == backendID
		}),
	}
}

func ListReadOnlyCloneVolumes() Subquery {
	return Subquery{
		res: volume,
		op:  list,
		setResults: listVolumesSetResults(func(v *storage.Volume) bool {
			return v.Config.ReadOnlyClone
		}),
	}
}

func ListVolumesByInternalName(internalVolName string) Subquery {
	return Subquery{
		res: volume,
		op:  list,
		setResults: listVolumesSetResults(func(v *storage.Volume) bool {
			return v.Config.InternalName == internalVolName
		}),
	}
}

func listVolumesSetResults(filter func(*storage.Volume) bool) func(*Subquery, *Result) error {
	return func(_ *Subquery, r *Result) error {
		volumes.rlock()
		r.Volumes = make([]*storage.Volume, 0, len(volumes.data))
		for k := range volumes.data {
			volume := volumes.data[k].SmartCopy().(*storage.Volume)
			if filter(volume) {
				r.Volumes = append(r.Volumes, volume)
			}
		}
		volumes.runlock()
		return nil
	}
}

func ReadVolume(id string) Subquery {
	return Subquery{
		res: volume,
		op:  read,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			volumes.rlock()
			if i, ok := volumes.data[s.id]; ok {
				r.Volume.Read = i.SmartCopy().(*storage.Volume)
			}
			volumes.runlock()
			return nil
		},
	}
}

func InconsistentReadVolume(id string) Subquery {
	return Subquery{
		res: volume,
		op:  inconsistentRead,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			volumes.rlock()
			if i, ok := volumes.data[s.id]; ok {
				r.Volume.Read = i.SmartCopy().(*storage.Volume)
			}
			volumes.runlock()
			return nil
		},
	}
}

func UpsertVolumeByInternalName(volumeName, internalVolumeName, newInternalVolumeName, backendID string) Subquery {
	var setOwnerIDs func([]Subquery, int) error
	if backendID != "" {
		setOwnerIDs = func(s []Subquery, i int) error {
			if err := checkDependency(s, i, backend); err != nil {
				return err
			}
			s[s[i].dependencies[0]].id = backendID
			return nil
		}
	}
	return Subquery{
		res:              volume,
		op:               upsert,
		id:               volumeName,
		key:              internalVolumeName,
		newKey:           newInternalVolumeName,
		setDependencyIDs: setOwnerIDs,
		setResults: func(s *Subquery, r *Result) error {
			volumes.rlock()
			if i, ok := volumes.data[s.id]; ok {
				r.Volume.Read = i.SmartCopy().(*storage.Volume)
			}
			volumes.runlock()
			r.Volume.Upsert = func(v *storage.Volume) {
				volumes.lock()
				switch {
				case s.key != "" && s.newKey == s.key:
					volumes.key.data[s.key] = s.id
				case s.key == "" && s.newKey != "":
					volumes.key.data[s.newKey] = s.id
				case s.key != "" && s.newKey == "":
					volumes.key.data[s.key] = s.id
				case s.key != "" && s.newKey != "" && s.key != s.newKey:
					delete(volumes.key.data, s.key)
					volumes.key.data[s.newKey] = s.id
				}
				volumes.data[s.id] = v
				volumes.unlock()
			}
			return nil
		},
	}
}

func UpsertVolume(volumeName, backendID string) Subquery {
	var setOwnerIDs func([]Subquery, int) error
	if backendID != "" {
		setOwnerIDs = func(s []Subquery, i int) error {
			if err := checkDependency(s, i, backend); err != nil {
				return err
			}
			s[s[i].dependencies[0]].id = backendID
			return nil
		}
	}
	return Subquery{
		res:              volume,
		op:               upsert,
		id:               volumeName,
		setDependencyIDs: setOwnerIDs,
		setResults: func(s *Subquery, r *Result) error {
			volumes.rlock()
			if i, ok := volumes.data[s.id]; ok {
				r.Volume.Read = i.SmartCopy().(*storage.Volume)
			}
			volumes.runlock()
			r.Volume.Upsert = func(v *storage.Volume) {
				volumes.lock()
				volumes.data[s.id] = v
				volumes.unlock()
			}
			return nil
		},
	}
}

func UpsertVolumeByBackendName(volumeName, backendName string) Subquery {
	var setOwnerIDs func([]Subquery, int) error

	if backendName != "" {
		setOwnerIDs = func(s []Subquery, i int) error {
			if err := checkDependency(s, i, backend); err != nil {
				return err
			}
			backendID, ok := backends.key.data[backendName]
			if !ok {
				return fmt.Errorf("backend %q not found", backendName)
			}
			s[s[i].dependencies[0]].id = backendID
			return nil
		}
	}
	return Subquery{
		res:              volume,
		op:               upsert,
		id:               volumeName,
		setDependencyIDs: setOwnerIDs,
		setResults: func(s *Subquery, r *Result) error {
			volumes.rlock()
			if i, ok := volumes.data[s.id]; ok {
				r.Volume.Read = i.SmartCopy().(*storage.Volume)
			}
			volumes.runlock()
			r.Volume.Upsert = func(v *storage.Volume) {
				volumes.lock()
				volumes.data[s.id] = v
				volumes.unlock()
			}
			return nil
		},
	}
}

func DeleteVolume(id string) Subquery {
	return Subquery{
		res: volume,
		op:  del,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			volumes.rlock()
			if i, ok := volumes.data[s.id]; ok {
				r.Volume.Read = i.SmartCopy().(*storage.Volume)
			}
			volumes.runlock()
			r.Volume.Delete = func() {
				volumes.lock()
				delete(volumes.data, s.id)
				// delete from the uniqueKey cache as well
				delete(volumes.key.data, s.key)
				volumes.unlock()
			}
			return nil
		},
	}
}
