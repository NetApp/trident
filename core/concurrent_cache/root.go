package concurrent_cache

// LockCache takes a write lock on the cache root, which blocks all other operations.
func LockCache() Subquery {
	return Subquery{
		res: root,
		op:  upsert,
		id:  ".",
		setResults: func(s *Subquery, r *Result) error {
			return nil
		},
	}
}
