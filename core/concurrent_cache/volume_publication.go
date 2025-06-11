package concurrent_cache

import (
	"fmt"

	"github.com/netapp/trident/utils/models"
)

// VolumePublication queries

func ListVolumePublications() Subquery {
	return Subquery{
		res:        volumePublication,
		op:         list,
		setResults: listVolumePublicationsSetResults(func(_ *models.VolumePublication) bool { return true }),
	}
}

func ListVolumePublicationsForVolume(volumeName string) Subquery {
	return Subquery{
		res: volumePublication,
		op:  list,
		setResults: listVolumePublicationsSetResults(func(v *models.VolumePublication) bool {
			return v.VolumeName == volumeName
		}),
	}
}

func ListVolumePublicationsForNode(nodeName string) Subquery {
	return Subquery{
		res: volumePublication,
		op:  list,
		setResults: listVolumePublicationsSetResults(func(v *models.VolumePublication) bool {
			return v.NodeName == nodeName
		}),
	}
}

func listVolumePublicationsSetResults(filter func(*models.VolumePublication) bool) func(*Subquery, *Result) error {
	return func(_ *Subquery, r *Result) error {
		volumePublications.rlock()
		r.VolumePublications = make([]*models.VolumePublication, 0, len(volumePublications.data))
		for k := range volumePublications.data {
			volumePublication := volumePublications.data[k].SmartCopy().(*models.VolumePublication)
			if filter(volumePublication) {
				r.VolumePublications = append(r.VolumePublications, volumePublication)
			}
		}
		volumePublications.runlock()
		return nil
	}
}

func ReadVolumePublication(volumeID, nodeID string) Subquery {
	return Subquery{
		res: volumePublication,
		op:  read,
		id:  volumeID + "." + nodeID,
		setResults: func(s *Subquery, r *Result) error {
			volumePublications.rlock()
			if i, ok := volumePublications.data[s.id]; ok {
				r.VolumePublication.Read = i.SmartCopy().(*models.VolumePublication)
			}
			volumePublications.runlock()
			return nil
		},
	}
}

func InconsistentReadVolumePublication(volumeID, nodeID string) Subquery {
	return Subquery{
		res: volumePublication,
		op:  inconsistentRead,
		id:  volumeID + "." + nodeID,
		setResults: func(s *Subquery, r *Result) error {
			volumePublications.rlock()
			if i, ok := volumePublications.data[s.id]; ok {
				r.VolumePublication.Read = i.SmartCopy().(*models.VolumePublication)
			}
			volumePublications.runlock()
			return nil
		},
	}
}

func UpsertVolumePublication(volumeID, nodeID string) Subquery {
	return Subquery{
		res: volumePublication,
		op:  upsert,
		id:  volumeID + "." + nodeID,
		setDependencyIDs: func(s []Subquery, i int) error {
			if len(s[i].dependencies) != 2 {
				return fmt.Errorf("expected two parents, got %d", len(s[i].dependencies))
			}
			for _, p := range s[i].dependencies {
				switch s[p].res {
				case volume:
					s[p].id = volumeID
				case node:
					s[p].id = nodeID
				default:
					return fmt.Errorf("unexpected parent resource %s", s[p].res)
				}
			}
			return nil
		},
		setResults: func(s *Subquery, r *Result) error {
			volumePublications.rlock()
			if i, ok := volumePublications.data[s.id]; ok {
				r.VolumePublication.Read = i.SmartCopy().(*models.VolumePublication)
			}
			volumePublications.runlock()
			r.VolumePublication.Upsert = func(vp *models.VolumePublication) {
				volumePublications.lock()
				volumePublications.data[s.id] = vp
				volumePublications.unlock()
			}
			return nil
		},
	}
}

func DeleteVolumePublication(volumeID, nodeID string) Subquery {
	return Subquery{
		res: volumePublication,
		op:  del,
		id:  volumeID + "." + nodeID,
		setResults: func(s *Subquery, r *Result) error {
			volumePublications.rlock()
			if i, ok := volumePublications.data[s.id]; ok {
				r.VolumePublication.Read = i.SmartCopy().(*models.VolumePublication)
			}
			volumePublications.runlock()
			r.VolumePublication.Delete = func() {
				volumePublications.lock()
				delete(volumePublications.data, s.id)
				volumePublications.unlock()
			}
			return nil
		},
	}
}
