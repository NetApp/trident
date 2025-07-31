package concurrent_cache

import (
	"github.com/netapp/trident/utils/models"
)

// Node queries

func ListNodes() Subquery {
	return Subquery{
		res:        node,
		op:         list,
		setResults: listNodesSetResults(func(_ *models.Node) bool { return true }),
	}
}

func listNodesSetResults(filter func(*models.Node) bool) func(*Subquery, *Result) error {
	return func(_ *Subquery, r *Result) error {
		nodes.rlock()
		r.Nodes = make([]*models.Node, 0, len(nodes.data))
		for k := range nodes.data {
			node := nodes.data[k].SmartCopy().(*models.Node)
			if filter(node) {
				r.Nodes = append(r.Nodes, node)
			}
		}
		nodes.runlock()
		return nil
	}
}

func ReadNode(id string) Subquery {
	return Subquery{
		res: node,
		op:  read,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			nodes.rlock()
			if i, ok := nodes.data[s.id]; ok {
				r.Node.Read = i.SmartCopy().(*models.Node)
			}
			nodes.runlock()
			return nil
		},
	}
}

func InconsistentReadNode(id string) Subquery {
	return Subquery{
		res: node,
		op:  inconsistentRead,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			nodes.rlock()
			if i, ok := nodes.data[s.id]; ok {
				r.Node.Read = i.SmartCopy().(*models.Node)
			}
			nodes.runlock()
			return nil
		},
	}
}

func UpsertNode(id string) Subquery {
	return Subquery{
		res: node,
		op:  upsert,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			nodes.rlock()
			oldNodeData, ok := nodes.data[s.id]
			if ok {
				r.Node.Read = oldNodeData.SmartCopy().(*models.Node)
			}
			nodes.runlock()

			r.Node.Upsert = func(n *models.Node) {
				// Update metrics
				if oldNodeData != nil {
					deleteNodeFromMetrics(oldNodeData.(*models.Node))
				}
				addNodeToMetrics(n)

				nodes.lock()
				nodes.data[s.id] = n
				nodes.unlock()
			}
			return nil
		},
	}
}

func DeleteNode(id string) Subquery {
	return Subquery{
		res: node,
		op:  del,
		id:  id,
		setResults: func(s *Subquery, r *Result) error {
			nodes.rlock()
			nodeData, ok := nodes.data[s.id]
			if ok {
				r.Node.Read = nodeData.SmartCopy().(*models.Node)
			}
			nodes.runlock()

			r.Node.Delete = func() {
				// Update metrics
				if nodeData != nil {
					deleteNodeFromMetrics(nodeData.(*models.Node))
				}

				nodes.lock()
				delete(nodes.data, s.id)
				nodes.unlock()
			}
			return nil
		},
	}
}
