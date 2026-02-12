package concurrent_cache

import (
	"github.com/netapp/trident/storage"
)

// AutogrowPolicy queries

func ListAutogrowPolicies() Subquery {
	return Subquery{
		res:        autogrowPolicy,
		op:         list,
		setResults: listAutogrowPoliciesSetResults(func(_ *storage.AutogrowPolicy) bool { return true }),
	}
}

func listAutogrowPoliciesSetResults(filter func(*storage.AutogrowPolicy) bool) func(*Subquery, *Result) error {
	return func(_ *Subquery, r *Result) error {
		autogrowPolicies.rlock()
		r.AutogrowPolicies = make([]*storage.AutogrowPolicy, 0, len(autogrowPolicies.data))
		for k := range autogrowPolicies.data {
			autogrowPolicy := autogrowPolicies.data[k].SmartCopy().(*storage.AutogrowPolicy)
			if filter(autogrowPolicy) {
				r.AutogrowPolicies = append(r.AutogrowPolicies, autogrowPolicy)
			}
		}
		autogrowPolicies.runlock()
		return nil
	}
}

func ReadAutogrowPolicy(id string) Subquery {
	return Subquery{
		res: autogrowPolicy,
		op:  read,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			autogrowPolicies.rlock()
			if i, ok := autogrowPolicies.data[s.id]; ok {
				r.AutogrowPolicy.Read = i.SmartCopy().(*storage.AutogrowPolicy)
			}
			autogrowPolicies.runlock()
			return nil
		},
	}
}

func InconsistentReadAutogrowPolicy(id string) Subquery {
	return Subquery{
		res: autogrowPolicy,
		op:  inconsistentRead,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			autogrowPolicies.rlock()
			if i, ok := autogrowPolicies.data[s.id]; ok {
				r.AutogrowPolicy.Read = i.SmartCopy().(*storage.AutogrowPolicy)
			}
			autogrowPolicies.runlock()
			return nil
		},
	}
}

func UpsertAutogrowPolicy(id string) Subquery {
	return Subquery{
		res: autogrowPolicy,
		op:  upsert,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			autogrowPolicies.rlock()
			oldAutogrowPolicyData, ok := autogrowPolicies.data[s.id]
			if ok {
				r.AutogrowPolicy.Read = oldAutogrowPolicyData.SmartCopy().(*storage.AutogrowPolicy)
			}
			autogrowPolicies.runlock()

			r.AutogrowPolicy.Upsert = func(ap *storage.AutogrowPolicy) {
				autogrowPolicies.lock()
				autogrowPolicies.data[s.id] = ap
				autogrowPolicies.unlock()
			}
			return nil
		},
	}
}

func DeleteAutogrowPolicy(id string) Subquery {
	return Subquery{
		res: autogrowPolicy,
		op:  del,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			autogrowPolicies.rlock()
			oldAutogrowPolicyData, ok := autogrowPolicies.data[s.id]
			if ok {
				r.AutogrowPolicy.Read = oldAutogrowPolicyData.SmartCopy().(*storage.AutogrowPolicy)
			}
			autogrowPolicies.runlock()
			r.AutogrowPolicy.Delete = func() {
				autogrowPolicies.lock()
				delete(autogrowPolicies.data, s.id)
				autogrowPolicies.unlock()
			}
			return nil
		},
	}
}
