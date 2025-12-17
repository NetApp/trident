package concurrent_cache

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/core/metrics"
	"github.com/netapp/trident/utils/models"
)

func TestUpsertNode_Metrics(t *testing.T) {
	tests := []struct {
		name        string
		nodeExists  bool
		initialNode *models.Node
		upsertNode  *models.Node
	}{
		{
			name:       "insert new node",
			nodeExists: false,
			upsertNode: &models.Node{
				Name: "new-node",
				IQN:  "iqn.2023.com.example:new-node",
			},
		},
		{
			name:       "update existing node",
			nodeExists: true,
			initialNode: &models.Node{
				Name: "existing-node",
				IQN:  "iqn.2023.com.example:existing-node",
			},
			upsertNode: &models.Node{
				Name: "existing-node",
				IQN:  "iqn.2023.com.example:existing-node-updated",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.NodeGauge.Set(0)

			// Set up initial state if node exists
			if tt.nodeExists {
				nodes.lock()
				nodes.data["test-node"] = tt.initialNode
				nodes.unlock()
				// Add the existing node to metrics to simulate realistic state
				addNodeToMetrics(tt.initialNode)
			}

			// Get initial metric value
			initialNodeGauge := testutil.ToFloat64(metrics.NodeGauge)

			// Execute upsert operation
			subquery := UpsertNode("test-node")
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "UpsertNode setResults should not error")

			// Verify the upsert function was created
			assert.NotNil(t, result.Node.Upsert, "Upsert function should be created")

			// Call the upsert function
			result.Node.Upsert(tt.upsertNode)

			// Verify metrics were updated correctly
			afterUpsertNodeGauge := testutil.ToFloat64(metrics.NodeGauge)

			if tt.nodeExists {
				// For existing node: delete old (dec) + add new (inc) = no net change
				assert.Equal(t, initialNodeGauge, afterUpsertNodeGauge, "NodeGauge should remain unchanged when updating existing node")
			} else {
				// For new node: only add (inc) = increment by 1
				assert.Equal(t, initialNodeGauge+1, afterUpsertNodeGauge, "NodeGauge should be incremented by 1 when adding new node")
			}

			// Verify the node was actually stored
			nodes.rlock()
			storedNode, exists := nodes.data["test-node"]
			nodes.runlock()
			assert.True(t, exists, "Node should exist in storage after upsert")
			assert.Equal(t, tt.upsertNode, storedNode, "Stored node should match upserted node")

			// Clean up
			nodes.lock()
			delete(nodes.data, "test-node")
			nodes.unlock()
		})
	}
}

func TestDeleteNode_Metrics(t *testing.T) {
	tests := []struct {
		name         string
		nodeExists   bool
		nodeToDelete *models.Node
	}{
		{
			name:       "delete existing node",
			nodeExists: true,
			nodeToDelete: &models.Node{
				Name: "node-to-delete",
				IQN:  "iqn.2023.com.example:node-to-delete",
			},
		},
		{
			name:         "delete non-existing node",
			nodeExists:   false,
			nodeToDelete: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset metrics before each test
			metrics.NodeGauge.Set(0)

			// Set up initial state if node exists
			if tt.nodeExists {
				nodes.lock()
				nodes.data["test-node"] = tt.nodeToDelete
				nodes.unlock()
				// Add the existing node to metrics to simulate realistic state
				addNodeToMetrics(tt.nodeToDelete)
			}

			// Get initial metric value
			initialNodeGauge := testutil.ToFloat64(metrics.NodeGauge)

			// Execute delete operation
			subquery := DeleteNode("test-node")
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "DeleteNode setResults should not error")

			if tt.nodeExists {
				// Verify the delete function was created and the node was read
				assert.NotNil(t, result.Node.Delete, "Delete function should be created")
				assert.Equal(t, tt.nodeToDelete, result.Node.Read, "Read node should match the node that exists")

				// Call the delete function
				result.Node.Delete()

				// Verify metrics were updated correctly (decremented by 1)
				afterDeleteNodeGauge := testutil.ToFloat64(metrics.NodeGauge)
				assert.Equal(t, initialNodeGauge-1, afterDeleteNodeGauge, "NodeGauge should be decremented by 1 when deleting existing node")

				// Verify the node was actually removed from storage
				nodes.rlock()
				_, exists := nodes.data["test-node"]
				nodes.runlock()
				assert.False(t, exists, "Node should not exist in storage after delete")
			} else {
				// For non-existing node, delete function should still be created but Read should be nil
				assert.NotNil(t, result.Node.Delete, "Delete function should be created even for non-existing node")
				assert.Nil(t, result.Node.Read, "Read node should be nil for non-existing node")

				// Call the delete function
				result.Node.Delete()

				// Verify metrics were NOT updated (no change since node didn't exist)
				afterDeleteNodeGauge := testutil.ToFloat64(metrics.NodeGauge)
				assert.Equal(t, initialNodeGauge, afterDeleteNodeGauge, "NodeGauge should remain unchanged when deleting non-existing node")
			}
		})
	}
}

func TestListNodes(t *testing.T) {
	tests := []struct {
		name     string
		nodes    map[string]*models.Node
		expected int
	}{
		{
			name:     "empty nodes",
			nodes:    map[string]*models.Node{},
			expected: 0,
		},
		{
			name: "single node",
			nodes: map[string]*models.Node{
				"node1": {
					Name: "node1",
					IQN:  "iqn.2023.com.example:node1",
				},
			},
			expected: 1,
		},
		{
			name: "multiple nodes",
			nodes: map[string]*models.Node{
				"node1": {
					Name: "node1",
					IQN:  "iqn.2023.com.example:node1",
				},
				"node2": {
					Name: "node2",
					IQN:  "iqn.2023.com.example:node2",
				},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			nodes.lock()
			nodes.data = make(map[string]SmartCopier)
			for k, v := range tt.nodes {
				nodes.data[k] = v
			}
			nodes.unlock()

			// Execute ListNodes
			subquery := ListNodes()
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ListNodes setResults should not error")

			// Verify results
			assert.Len(t, result.Nodes, tt.expected, "Number of nodes should match expected")

			// Clean up
			nodes.lock()
			nodes.data = make(map[string]SmartCopier)
			nodes.unlock()
		})
	}
}

func TestReadNode(t *testing.T) {
	tests := []struct {
		name         string
		setupNode    bool
		nodeID       string
		expectedNode *models.Node
	}{
		{
			name:      "existing node",
			setupNode: true,
			nodeID:    "test-node-id",
			expectedNode: &models.Node{
				Name: "test-node",
				IQN:  "iqn.2023.com.example:test-node",
			},
		},
		{
			name:      "non-existing node",
			setupNode: false,
			nodeID:    "non-existing-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			nodes.lock()
			nodes.data = make(map[string]SmartCopier)
			if tt.setupNode {
				nodes.data[tt.nodeID] = tt.expectedNode
			}
			nodes.unlock()

			// Execute ReadNode
			subquery := ReadNode(tt.nodeID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "ReadNode setResults should not error")

			// Verify results
			if tt.setupNode {
				assert.NotNil(t, result.Node.Read, "Node should be found")
				assert.Equal(t, tt.expectedNode, result.Node.Read, "Node should match expected")
			} else {
				assert.Nil(t, result.Node.Read, "Node should not be found")
			}

			// Clean up
			nodes.lock()
			nodes.data = make(map[string]SmartCopier)
			nodes.unlock()
		})
	}
}

func TestInconsistentReadNode(t *testing.T) {
	tests := []struct {
		name         string
		setupNode    bool
		nodeID       string
		expectedNode *models.Node
	}{
		{
			name:      "existing node",
			setupNode: true,
			nodeID:    "test-node-id",
			expectedNode: &models.Node{
				Name: "test-node",
				IQN:  "iqn.2023.com.example:test-node",
			},
		},
		{
			name:      "non-existing node",
			setupNode: false,
			nodeID:    "non-existing-id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up initial state
			nodes.lock()
			nodes.data = make(map[string]SmartCopier)
			if tt.setupNode {
				nodes.data[tt.nodeID] = tt.expectedNode
			}
			nodes.unlock()

			// Execute InconsistentReadNode
			subquery := InconsistentReadNode(tt.nodeID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)
			assert.NoError(t, err, "InconsistentReadNode setResults should not error")

			// Verify results
			if tt.setupNode {
				assert.NotNil(t, result.Node.Read, "Node should be found")
				assert.Equal(t, tt.expectedNode, result.Node.Read, "Node should match expected")
			} else {
				assert.Nil(t, result.Node.Read, "Node should not be found")
			}

			// Clean up
			nodes.lock()
			nodes.data = make(map[string]SmartCopier)
			nodes.unlock()
		})
	}
}
