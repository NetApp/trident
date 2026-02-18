package concurrent_cache

import (
	"strings"

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

// ListSubordinateVolumesForAutogrowPolicy returns all subordinate volumes using a specific autogrow policy
func ListSubordinateVolumesForAutogrowPolicy(autogrowPolicyName string) Subquery {
	return Subquery{
		res: subordinateVolume,
		op:  list,
		setResults: listSubordinateVolumesSetResults(func(v *storage.Volume) bool {
			return v.EffectiveAGPolicy.PolicyName == autogrowPolicyName
		}),
	}
}

// ListSubordinateVolumesForStorageClass returns all subordinate volumes using a specific storage class
func ListSubordinateVolumesForStorageClass(scName string) Subquery {
	return Subquery{
		res: subordinateVolume,
		op:  list,
		setResults: listSubordinateVolumesSetResults(func(v *storage.Volume) bool {
			return v.Config.StorageClass == scName
		}),
	}
}

// ListSubordinateVolumesForStorageClassWithoutAutogrowOverride returns subordinate volumes using a specific storage
// class
// that have NO volume-level autogrow policy (i.e., they inherit from StorageClass)
func ListSubordinateVolumesForStorageClassWithoutAutogrowOverride(scName string) Subquery {
	return Subquery{
		res: subordinateVolume,
		op:  list,
		setResults: listSubordinateVolumesSetResults(func(v *storage.Volume) bool {
			return v.Config.StorageClass == scName && v.Config.RequestedAutogrowPolicy == ""
		}),
	}
}

// ListSubordinateVolumesForAutogrowPolicyReevaluation returns subordinate volumes that might need to use the given
// policy.
// This includes subordinate volumes that:
// 1. Have a volume-level autogrow policy matching the policy name (case-insensitive), OR
// 2. Have no volume-level autogrow policy (might inherit from StorageClass)
// AND are not already using the policy.
func ListSubordinateVolumesForAutogrowPolicyReevaluation(policyName string) Subquery {
	return Subquery{
		res: subordinateVolume,
		op:  list,
		setResults: listSubordinateVolumesSetResults(func(v *storage.Volume) bool {
			requestedAutogrowPolicy := v.Config.RequestedAutogrowPolicy

			// Skip if requestedAutogrowPolicy is "none" (any case variation)
			if strings.EqualFold(requestedAutogrowPolicy, "none") {
				return false
			}

			// Skip if requestedAutogrowPolicy explicitly references a different policy
			if requestedAutogrowPolicy != "" && !strings.EqualFold(requestedAutogrowPolicy, policyName) {
				return false
			}

			// Skip if already using this policy
			if v.EffectiveAGPolicy.PolicyName == policyName {
				return false
			}

			// Include: no requestedAutogrowPolicy OR requestedAutogrowPolicy matches policy,
			// and not currently using it
			return true
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
