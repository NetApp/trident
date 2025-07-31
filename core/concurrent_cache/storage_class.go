package concurrent_cache

import (
	storageclass "github.com/netapp/trident/storage_class"
)

// StorageClass queries

func ListStorageClasses() Subquery {
	return Subquery{
		res:        storageClass,
		op:         list,
		setResults: listStorageClassesSetResults(func(_ *storageclass.StorageClass) bool { return true }),
	}
}

func listStorageClassesSetResults(filter func(*storageclass.StorageClass) bool) func(*Subquery, *Result) error {
	return func(_ *Subquery, r *Result) error {
		storageClasses.rlock()
		r.StorageClasses = make([]*storageclass.StorageClass, 0, len(storageClasses.data))
		for k := range storageClasses.data {
			sc := storageClasses.data[k].SmartCopy().(*storageclass.StorageClass)
			if filter(sc) {
				r.StorageClasses = append(r.StorageClasses, sc)
			}
		}
		storageClasses.runlock()
		return nil
	}
}

func ReadStorageClass(id string) Subquery {
	return Subquery{
		res: storageClass,
		op:  read,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			storageClasses.rlock()
			if i, ok := storageClasses.data[s.id]; ok {
				r.StorageClass.Read = i.SmartCopy().(*storageclass.StorageClass)
			}
			storageClasses.runlock()
			return nil
		},
	}
}

func InconsistentReadStorageClass(id string) Subquery {
	return Subquery{
		res: storageClass,
		op:  inconsistentRead,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			storageClasses.rlock()
			if i, ok := storageClasses.data[s.id]; ok {
				r.StorageClass.Read = i.SmartCopy().(*storageclass.StorageClass)
			}
			storageClasses.runlock()
			return nil
		},
	}
}

func UpsertStorageClass(id string) Subquery {
	return Subquery{
		res: storageClass,
		op:  upsert,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			storageClasses.rlock()
			oldSCData, ok := storageClasses.data[s.id]
			if ok {
				r.StorageClass.Read = oldSCData.SmartCopy().(*storageclass.StorageClass)
			}
			storageClasses.runlock()

			r.StorageClass.Upsert = func(sc *storageclass.StorageClass) {
				// Update metrics
				if oldSCData != nil {
					deleteStorageClassFromMetrics(oldSCData.(*storageclass.StorageClass))
				}
				addStorageClassToMetrics(sc)

				storageClasses.lock()
				storageClasses.data[s.id] = sc
				storageClasses.unlock()
			}
			return nil
		},
	}
}

func DeleteStorageClass(id string) Subquery {
	return Subquery{
		res: storageClass,
		op:  del,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			storageClasses.rlock()
			scData, ok := storageClasses.data[s.id]
			if ok {
				r.StorageClass.Read = scData.SmartCopy().(*storageclass.StorageClass)
			}
			storageClasses.runlock()

			r.StorageClass.Delete = func() {
				// Update metrics
				if scData != nil {
					deleteStorageClassFromMetrics(scData.(*storageclass.StorageClass))
				}

				storageClasses.lock()
				delete(storageClasses.data, id)
				storageClasses.unlock()
			}
			return nil
		},
	}
}
