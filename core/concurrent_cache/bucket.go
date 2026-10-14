package concurrent_cache

import "github.com/netapp/trident/object"

// Bucket queries

func ListBuckets() Subquery {
	return Subquery{
		res:        bucket,
		op:         list,
		setResults: listBucketsSetResults(func(_ *object.Bucket) bool { return true }),
	}
}

func ListBucketsForObjectStore(objectStoreID string) Subquery {
	return Subquery{
		res: bucket,
		op:  list,
		setResults: listBucketsSetResults(func(b *object.Bucket) bool {
			return b.GetObjectStoreID() == objectStoreID
		}),
	}
}

func listBucketsSetResults(filter func(*object.Bucket) bool) func(*Subquery, *Result) error {
	return func(_ *Subquery, r *Result) error {
		buckets.rlock()
		r.Buckets = make([]*object.Bucket, 0, len(buckets.data))
		for k := range buckets.data {
			cachedBucket := buckets.data[k].SmartCopy().(*object.Bucket)
			if filter(cachedBucket) {
				r.Buckets = append(r.Buckets, cachedBucket)
			}
		}
		buckets.runlock()
		return nil
	}
}

func ReadBucket(bucketID string) Subquery {
	return Subquery{
		res: bucket,
		op:  read,
		id:  bucketID,
		setResults: func(s *Subquery, r *Result) error {
			buckets.rlock()
			if bucketData, ok := buckets.data[s.id]; ok {
				r.Bucket.Read = bucketData.SmartCopy().(*object.Bucket)
			}
			buckets.runlock()
			return nil
		},
	}
}

func InconsistentReadBucket(bucketID string) Subquery {
	return Subquery{
		res: bucket,
		op:  inconsistentRead,
		id:  bucketID,
		setResults: func(s *Subquery, r *Result) error {
			buckets.rlock()
			if bucketData, ok := buckets.data[s.id]; ok {
				r.Bucket.Read = bucketData.SmartCopy().(*object.Bucket)
			}
			buckets.runlock()
			return nil
		},
	}
}

func UpsertBucket(bucketID, objectStoreID string) Subquery {
	var setOwnerIDs func([]Subquery, int) error

	if objectStoreID != "" {
		setOwnerIDs = func(s []Subquery, i int) error {
			if err := checkDependency(s, i, objectStore); err != nil {
				return err
			}
			s[s[i].dependencies[0]].id = objectStoreID
			return nil
		}
	}
	return Subquery{
		res:              bucket,
		op:               upsert,
		id:               bucketID,
		setDependencyIDs: setOwnerIDs,
		setResults: func(s *Subquery, r *Result) error {
			buckets.rlock()
			if bucketData, ok := buckets.data[s.id]; ok {
				r.Bucket.Read = bucketData.SmartCopy().(*object.Bucket)
			}
			buckets.runlock()

			r.Bucket.Upsert = func(cachedBucket *object.Bucket) {
				buckets.lock()
				buckets.data[s.id] = cachedBucket
				buckets.unlock()
			}
			return nil
		},
	}
}

func DeleteBucket(bucketID string) Subquery {
	return Subquery{
		res: bucket,
		op:  del,
		id:  bucketID,
		setResults: func(s *Subquery, r *Result) error {
			buckets.rlock()
			if bucketData, ok := buckets.data[s.id]; ok {
				r.Bucket.Read = bucketData.SmartCopy().(*object.Bucket)
			}
			buckets.runlock()

			r.Bucket.Delete = func() {
				buckets.lock()
				delete(buckets.data, s.id)
				buckets.unlock()
			}
			return nil
		},
	}
}
