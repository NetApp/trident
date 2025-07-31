package concurrent_cache

import (
	"github.com/netapp/trident/storage"
)

// Backend queries

func ListBackends() Subquery {
	return Subquery{
		res:        backend,
		op:         list,
		setResults: listBackendsSetResults(func(_ storage.Backend) bool { return true }),
	}
}

func listBackendsSetResults(filter func(storage.Backend) bool) func(*Subquery, *Result) error {
	return func(_ *Subquery, r *Result) error {
		backends.rlock()
		r.Backends = make([]storage.Backend, 0, len(backends.data))
		for k := range backends.data {
			backend := backends.data[k].SmartCopy().(storage.Backend)
			if filter(backend) {
				r.Backends = append(r.Backends, backend)
			}
		}
		backends.runlock()
		return nil
	}
}

func ReadBackendByName(name string) Subquery {
	return Subquery{
		res: backend,
		op:  read,
		key: name,
		setResults: func(s *Subquery, r *Result) error {
			backends.rlock()
			if i, ok := backends.data[s.id]; ok {
				r.Backend.Read = i.SmartCopy().(storage.Backend)
			}
			backends.runlock()
			return nil
		},
	}
}

func ReadBackend(id string) Subquery {
	return Subquery{
		res: backend,
		op:  read,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			backends.rlock()
			if i, ok := backends.data[s.id]; ok {
				r.Backend.Read = i.SmartCopy().(storage.Backend)
			}
			backends.runlock()
			return nil
		},
	}
}

func InconsistentReadBackend(id string) Subquery {
	return Subquery{
		res: backend,
		op:  inconsistentRead,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			backends.rlock()
			if i, ok := backends.data[s.id]; ok {
				r.Backend.Read = i.SmartCopy().(storage.Backend)
			}
			backends.runlock()
			return nil
		},
	}
}

func InconsistentReadBackendByName(name string) Subquery {
	return Subquery{
		res: backend,
		op:  inconsistentRead,
		key: name,
		setResults: func(s *Subquery, r *Result) error {
			backends.rlock()
			if i, ok := backends.data[s.id]; ok {
				r.Backend.Read = i.SmartCopy().(storage.Backend)
			}
			backends.runlock()
			return nil
		},
	}
}

func UpsertBackend(id, name, newName string) Subquery {
	return Subquery{
		res:    backend,
		op:     upsert,
		id:     id,
		key:    name,
		newKey: newName,
		setResults: func(s *Subquery, r *Result) error {
			backends.rlock()
			oldBackendData, ok := backends.data[s.id]
			if ok {
				r.Backend.Read = oldBackendData.SmartCopy().(storage.Backend)
			}
			backends.runlock()

			r.Backend.Upsert = func(b storage.Backend) {
				if s.id == "" {
					s.id = b.BackendUUID()
				}

				// Update metrics
				if oldBackendData != nil {
					deleteBackendFromMetrics(oldBackendData.(storage.Backend))
				}
				addBackendToMetrics(b)

				backends.lock()

				// This is the canonical method for upserting with a unique key.
				// There are 4 cases: key and newKey are equal, key is empty, newKey is empty,
				// and key and newKey are different. In all cases expect #3,
				// key must be deleted from the unique keys and newKey must be added pointing to id.
				// If both keys are empty we should have errored out before this point.
				switch {
				case s.key != "" && s.newKey == s.key:
					// Case 1, ensure key is pointing to id
					backends.key.data[s.key] = s.id
				case s.key == "" && s.newKey != "":
					// Case 2, key is empty, so point newKey to id
					backends.key.data[s.newKey] = s.id
				case s.key != "" && s.newKey == "":
					// Case 3, newKey is empty, so just point key to id
					backends.key.data[s.key] = s.id
				case s.key != "" && s.newKey != "" && s.key != s.newKey:
					// Case 4, both keys are different, so delete key and add newKey
					delete(backends.key.data, s.key)
					backends.key.data[s.newKey] = s.id
				}

				backends.data[s.id] = b
				backends.unlock()
			}
			return nil
		},
	}
}

func DeleteBackend(id string) Subquery {
	return Subquery{
		res: backend,
		op:  del,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			backends.rlock()
			backendData, ok := backends.data[s.id]
			if ok {
				r.Backend.Read = backendData.SmartCopy().(storage.Backend)
			}
			backends.runlock()

			r.Backend.Delete = func() {
				// Update metrics
				if backendData != nil {
					deleteBackendFromMetrics(backendData.(storage.Backend))
				}

				backends.lock()
				delete(backends.data, s.id)
				// delete from the uniqueKey cache as well
				delete(backends.key.data, s.key)
				backends.unlock()
			}
			return nil
		},
	}
}
