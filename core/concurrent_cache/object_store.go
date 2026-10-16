package concurrent_cache

import "github.com/netapp/trident/object"

// Object Store queries

func ListObjectStores() Subquery {
	return Subquery{
		res:        objectStore,
		op:         list,
		setResults: listObjectStoresSetResults(func(_ object.Store) bool { return true }),
	}
}

func FilterObjectStoresForBucket(bucketName string) Subquery {
	return Subquery{
		res: objectStore,
		op:  list,
		setResults: func(_ *Subquery, r *Result) error {
			unlocker, err := rLockCaches([]resource{objectStore, bucket})
			defer unlocker()
			if err != nil {
				return err
			}
			r.ObjectStores = make([]object.Store, 0)

			var bucketStoreID string
			if cached, ok := buckets.data[bucketName]; ok {
				stored := cached.(*object.Bucket)
				if stored.StoreUUID != "" {
					bucketStoreID = stored.StoreUUID
					if store, found := objectStores.data[bucketStoreID]; found {
						r.ObjectStores = append(r.ObjectStores, store.SmartCopy().(object.Store))
					} else {
						r.ObjectStores = append(r.ObjectStores, nil)
					}
				}
			}

			for _, store := range objectStores.data {
				if store.(object.Store).StoreUUID() != bucketStoreID {
					r.ObjectStores = append(r.ObjectStores, store.SmartCopy().(object.Store))
				}
			}
			return nil
		},
	}
}

func listObjectStoresSetResults(filter func(object.Store) bool) func(*Subquery, *Result) error {
	return func(_ *Subquery, r *Result) error {
		objectStores.rlock()
		r.ObjectStores = make([]object.Store, 0, len(objectStores.data))
		for k := range objectStores.data {
			store := objectStores.data[k].SmartCopy().(object.Store)
			if filter(store) {
				r.ObjectStores = append(r.ObjectStores, store)
			}
		}
		objectStores.runlock()
		return nil
	}
}

func ReadObjectStoreByName(name string) Subquery {
	return Subquery{
		res: objectStore,
		op:  read,
		key: name,
		setResults: func(s *Subquery, r *Result) error {
			objectStores.rlock()
			if i, ok := objectStores.data[s.id]; ok {
				r.ObjectStore.Read = i.SmartCopy().(object.Store)
			}
			objectStores.runlock()
			return nil
		},
	}
}

func ReadObjectStore(id string) Subquery {
	return Subquery{
		res: objectStore,
		op:  read,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			objectStores.rlock()
			if i, ok := objectStores.data[s.id]; ok {
				r.ObjectStore.Read = i.SmartCopy().(object.Store)
			}
			objectStores.runlock()
			return nil
		},
	}
}

func InconsistentReadObjectStore(id string) Subquery {
	return Subquery{
		res: objectStore,
		op:  inconsistentRead,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			objectStores.rlock()
			if i, ok := objectStores.data[s.id]; ok {
				r.ObjectStore.Read = i.SmartCopy().(object.Store)
			}
			objectStores.runlock()
			return nil
		},
	}
}

func InconsistentReadObjectStoreByName(name string) Subquery {
	return Subquery{
		res: objectStore,
		op:  inconsistentRead,
		key: name,
		setResults: func(s *Subquery, r *Result) error {
			objectStores.rlock()
			if i, ok := objectStores.data[s.id]; ok {
				r.ObjectStore.Read = i.SmartCopy().(object.Store)
			}
			objectStores.runlock()
			return nil
		},
	}
}

func UpsertObjectStore(id, name, newName string) Subquery {
	return Subquery{
		res:    objectStore,
		op:     upsert,
		id:     id,
		key:    name,
		newKey: newName,
		setResults: func(s *Subquery, r *Result) error {
			objectStores.rlock()
			if storeData, ok := objectStores.data[s.id]; ok {
				r.ObjectStore.Read = storeData.SmartCopy().(object.Store)
			}
			objectStores.runlock()

			r.ObjectStore.Upsert = func(store object.Store) {
				if s.id == "" {
					s.id = store.StoreUUID()
				}

				objectStores.lock()
				switch {
				case s.key != "" && s.newKey == s.key:
					objectStores.key.data[s.key] = s.id
				case s.key == "" && s.newKey != "":
					objectStores.key.data[s.newKey] = s.id
				case s.key != "" && s.newKey == "":
					objectStores.key.data[s.key] = s.id
				case s.key != "" && s.newKey != "" && s.key != s.newKey:
					delete(objectStores.key.data, s.key)
					objectStores.key.data[s.newKey] = s.id
				}
				objectStores.data[s.id] = store
				objectStores.unlock()
			}
			return nil
		},
	}
}

func DeleteObjectStoreByName(name string) Subquery {
	subquery := DeleteObjectStore("")
	subquery.key = name
	return subquery
}

func DeleteObjectStore(id string) Subquery {
	return Subquery{
		res: objectStore,
		op:  del,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			objectStores.rlock()
			if storeData, ok := objectStores.data[s.id]; ok {
				r.ObjectStore.Read = storeData.SmartCopy().(object.Store)
			}
			objectStores.runlock()

			r.ObjectStore.Delete = func() {
				objectStores.lock()
				delete(objectStores.data, s.id)
				delete(objectStores.key.data, s.key)
				objectStores.unlock()
			}
			return nil
		},
	}
}
