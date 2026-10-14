package concurrent_cache

import (
	"github.com/netapp/trident/object"
)

// Bucket Access queries

func ListBucketAccesses() Subquery {
	return Subquery{
		res:        bucketAccess,
		op:         list,
		setResults: listBucketAccessesSetResults(func(_ *object.BucketAccess) bool { return true }),
	}
}

func ListBucketAccessesForObjectStore(storeName string) Subquery {
	return Subquery{
		res: bucketAccess,
		op:  list,
		setResults: listBucketAccessesSetResults(func(access *object.BucketAccess) bool {
			return access.GetStoreName() == storeName
		}),
	}
}

func listBucketAccessesSetResults(filter func(*object.BucketAccess) bool) func(*Subquery, *Result) error {
	return func(_ *Subquery, r *Result) error {
		bucketAccesses.rlock()
		r.BucketAccesses = make([]*object.BucketAccess, 0, len(bucketAccesses.data))
		for k := range bucketAccesses.data {
			access := bucketAccesses.data[k].SmartCopy().(*object.BucketAccess)
			if filter(access) {
				r.BucketAccesses = append(r.BucketAccesses, access)
			}
		}
		bucketAccesses.runlock()
		return nil
	}
}

func ReadBucketAccess(bucketAccessID string) Subquery {
	return Subquery{
		res: bucketAccess,
		op:  read,
		id:  bucketAccessID,
		setResults: func(s *Subquery, r *Result) error {
			bucketAccesses.rlock()
			if accessData, ok := bucketAccesses.data[s.id]; ok {
				r.BucketAccess.Read = accessData.SmartCopy().(*object.BucketAccess)
			}
			bucketAccesses.runlock()
			return nil
		},
	}
}

func InconsistentReadBucketAccess(bucketAccessID string) Subquery {
	return Subquery{
		res: bucketAccess,
		op:  inconsistentRead,
		id:  bucketAccessID,
		setResults: func(s *Subquery, r *Result) error {
			bucketAccesses.rlock()
			if accessData, ok := bucketAccesses.data[s.id]; ok {
				r.BucketAccess.Read = accessData.SmartCopy().(*object.BucketAccess)
			}
			bucketAccesses.runlock()
			return nil
		},
	}
}

func UpsertBucketAccess(bucketAccessID string, storeName string) Subquery {
	var setOwnerIDs func([]Subquery, int) error

	if storeName != "" {
		setOwnerIDs = func(s []Subquery, i int) error {
			if err := checkDependency(s, i, objectStore); err != nil {
				return err
			}
			s[s[i].dependencies[0]].id = storeName
			return nil
		}
	}
	return Subquery{
		res:              bucketAccess,
		op:               upsert,
		id:               bucketAccessID,
		setDependencyIDs: setOwnerIDs,
		setResults: func(s *Subquery, r *Result) error {
			bucketAccesses.rlock()
			if accessData, ok := bucketAccesses.data[s.id]; ok {
				r.BucketAccess.Read = accessData.SmartCopy().(*object.BucketAccess)
			}
			bucketAccesses.runlock()

			r.BucketAccess.Upsert = func(access *object.BucketAccess) {
				bucketAccesses.lock()
				bucketAccesses.data[s.id] = access
				bucketAccesses.unlock()
			}
			return nil
		},
	}
}

func DeleteBucketAccess(bucketAccessID string) Subquery {
	return Subquery{
		res: bucketAccess,
		op:  del,
		id:  bucketAccessID,
		setResults: func(s *Subquery, r *Result) error {
			bucketAccesses.rlock()
			if accessData, ok := bucketAccesses.data[s.id]; ok {
				r.BucketAccess.Read = accessData.SmartCopy().(*object.BucketAccess)
			}
			bucketAccesses.runlock()

			r.BucketAccess.Delete = func() {
				bucketAccesses.lock()
				delete(bucketAccesses.data, s.id)
				bucketAccesses.unlock()
			}
			return nil
		},
	}
}
