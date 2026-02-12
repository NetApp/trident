package concurrent_cache

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/storage"
)

// TestListAutogrowPolicies tests the ListAutogrowPolicies function
func TestListAutogrowPolicies(t *testing.T) {
	tests := []struct {
		name               string
		setupPolicies      map[string]*storage.AutogrowPolicy
		expectedCount      int
		expectedPolicyName string
	}{
		{
			name:          "no autogrow policies",
			setupPolicies: map[string]*storage.AutogrowPolicy{},
			expectedCount: 0,
		},
		{
			name: "single autogrow policy",
			setupPolicies: map[string]*storage.AutogrowPolicy{
				"policy1": storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess),
			},
			expectedCount:      1,
			expectedPolicyName: "policy1",
		},
		{
			name: "multiple autogrow policies",
			setupPolicies: map[string]*storage.AutogrowPolicy{
				"policy1": storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess),
				"policy2": storage.NewAutogrowPolicy("policy2", "90", "30", "2000Gi", storage.AutogrowPolicyStateSuccess),
				"policy3": storage.NewAutogrowPolicy("policy3", "75", "25", "1500Gi", storage.AutogrowPolicyStateFailed),
			},
			expectedCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			autogrowPolicies.lock()
			autogrowPolicies.data = make(map[string]SmartCopier)
			for k, v := range tt.setupPolicies {
				autogrowPolicies.data[k] = v
			}
			autogrowPolicies.unlock()

			// Execute
			subquery := ListAutogrowPolicies()
			result := &Result{}
			err := subquery.setResults(&subquery, result)

			// Assert
			assert.NoError(t, err, "ListAutogrowPolicies setResults should not error")
			assert.Len(t, result.AutogrowPolicies, tt.expectedCount, "Number of autogrow policies should match")

			if tt.expectedCount > 0 && tt.expectedPolicyName != "" {
				found := false
				for _, policy := range result.AutogrowPolicies {
					if policy.Name() == tt.expectedPolicyName {
						found = true
						break
					}
				}
				assert.True(t, found, "Expected policy should be in result")
			}

			// Cleanup
			autogrowPolicies.lock()
			autogrowPolicies.data = make(map[string]SmartCopier)
			autogrowPolicies.unlock()
		})
	}
}

// TestReadAutogrowPolicy tests the ReadAutogrowPolicy function
func TestReadAutogrowPolicy(t *testing.T) {
	tests := []struct {
		name           string
		setupPolicies  map[string]*storage.AutogrowPolicy
		policyID       string
		expectFound    bool
		expectedPolicy *storage.AutogrowPolicy
	}{
		{
			name:          "policy not found",
			setupPolicies: map[string]*storage.AutogrowPolicy{},
			policyID:      "nonexistent",
			expectFound:   false,
		},
		{
			name: "policy found",
			setupPolicies: map[string]*storage.AutogrowPolicy{
				"policy1": storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess),
			},
			policyID:       "policy1",
			expectFound:    true,
			expectedPolicy: storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess),
		},
		{
			name: "policy found with different state",
			setupPolicies: map[string]*storage.AutogrowPolicy{
				"policy2": storage.NewAutogrowPolicy("policy2", "90", "30", "2000Gi", storage.AutogrowPolicyStateFailed),
			},
			policyID:       "policy2",
			expectFound:    true,
			expectedPolicy: storage.NewAutogrowPolicy("policy2", "90", "30", "2000Gi", storage.AutogrowPolicyStateFailed),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			autogrowPolicies.lock()
			autogrowPolicies.data = make(map[string]SmartCopier)
			for k, v := range tt.setupPolicies {
				autogrowPolicies.data[k] = v
			}
			autogrowPolicies.unlock()

			// Execute
			subquery := ReadAutogrowPolicy(tt.policyID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)

			// Assert
			assert.NoError(t, err, "ReadAutogrowPolicy setResults should not error")

			if tt.expectFound {
				assert.NotNil(t, result.AutogrowPolicy.Read, "Policy should be found")
				assert.Equal(t, tt.expectedPolicy.Name(), result.AutogrowPolicy.Read.Name(), "Policy name should match")
				assert.Equal(t, tt.expectedPolicy.UsedThreshold(), result.AutogrowPolicy.Read.UsedThreshold(), "UsedThreshold should match")
				assert.Equal(t, tt.expectedPolicy.GrowthAmount(), result.AutogrowPolicy.Read.GrowthAmount(), "GrowthAmount should match")
				assert.Equal(t, tt.expectedPolicy.MaxSize(), result.AutogrowPolicy.Read.MaxSize(), "MaxSize should match")
				assert.Equal(t, tt.expectedPolicy.State(), result.AutogrowPolicy.Read.State(), "State should match")
			} else {
				assert.Nil(t, result.AutogrowPolicy.Read, "Policy should not be found")
			}

			// Cleanup
			autogrowPolicies.lock()
			autogrowPolicies.data = make(map[string]SmartCopier)
			autogrowPolicies.unlock()
		})
	}
}

// TestInconsistentReadAutogrowPolicy tests the InconsistentReadAutogrowPolicy function
func TestInconsistentReadAutogrowPolicy(t *testing.T) {
	tests := []struct {
		name           string
		setupPolicies  map[string]*storage.AutogrowPolicy
		policyID       string
		expectFound    bool
		expectedPolicy *storage.AutogrowPolicy
	}{
		{
			name:          "policy not found",
			setupPolicies: map[string]*storage.AutogrowPolicy{},
			policyID:      "nonexistent",
			expectFound:   false,
		},
		{
			name: "policy found",
			setupPolicies: map[string]*storage.AutogrowPolicy{
				"policy1": storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess),
			},
			policyID:       "policy1",
			expectFound:    true,
			expectedPolicy: storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess),
		},
		{
			name: "policy found with Deleting state",
			setupPolicies: map[string]*storage.AutogrowPolicy{
				"policy3": storage.NewAutogrowPolicy("policy3", "75", "25", "1500Gi", storage.AutogrowPolicyStateDeleting),
			},
			policyID:       "policy3",
			expectFound:    true,
			expectedPolicy: storage.NewAutogrowPolicy("policy3", "75", "25", "1500Gi", storage.AutogrowPolicyStateDeleting),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			autogrowPolicies.lock()
			autogrowPolicies.data = make(map[string]SmartCopier)
			for k, v := range tt.setupPolicies {
				autogrowPolicies.data[k] = v
			}
			autogrowPolicies.unlock()

			// Execute
			subquery := InconsistentReadAutogrowPolicy(tt.policyID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)

			// Assert
			assert.NoError(t, err, "InconsistentReadAutogrowPolicy setResults should not error")

			if tt.expectFound {
				assert.NotNil(t, result.AutogrowPolicy.Read, "Policy should be found")
				assert.Equal(t, tt.expectedPolicy.Name(), result.AutogrowPolicy.Read.Name(), "Policy name should match")
			} else {
				assert.Nil(t, result.AutogrowPolicy.Read, "Policy should not be found")
			}

			// Cleanup
			autogrowPolicies.lock()
			autogrowPolicies.data = make(map[string]SmartCopier)
			autogrowPolicies.unlock()
		})
	}
}

// TestUpsertAutogrowPolicy tests the UpsertAutogrowPolicy function
func TestUpsertAutogrowPolicy(t *testing.T) {
	tests := []struct {
		name            string
		setupPolicies   map[string]*storage.AutogrowPolicy
		policyID        string
		newPolicy       *storage.AutogrowPolicy
		expectRead      bool
		expectedOldName string
	}{
		{
			name:          "insert new policy",
			setupPolicies: map[string]*storage.AutogrowPolicy{},
			policyID:      "policy1",
			newPolicy:     storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess),
			expectRead:    false,
		},
		{
			name: "update existing policy",
			setupPolicies: map[string]*storage.AutogrowPolicy{
				"policy1": storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess),
			},
			policyID:        "policy1",
			newPolicy:       storage.NewAutogrowPolicy("policy1", "90", "30", "2000Gi", storage.AutogrowPolicyStateSuccess),
			expectRead:      true,
			expectedOldName: "policy1",
		},
		{
			name: "update policy state to Deleting",
			setupPolicies: map[string]*storage.AutogrowPolicy{
				"policy2": storage.NewAutogrowPolicy("policy2", "75", "25", "1500Gi", storage.AutogrowPolicyStateSuccess),
			},
			policyID:        "policy2",
			newPolicy:       storage.NewAutogrowPolicy("policy2", "75", "25", "1500Gi", storage.AutogrowPolicyStateDeleting),
			expectRead:      true,
			expectedOldName: "policy2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			autogrowPolicies.lock()
			autogrowPolicies.data = make(map[string]SmartCopier)
			for k, v := range tt.setupPolicies {
				autogrowPolicies.data[k] = v
			}
			autogrowPolicies.unlock()

			// Execute
			subquery := UpsertAutogrowPolicy(tt.policyID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)

			// Assert
			assert.NoError(t, err, "UpsertAutogrowPolicy setResults should not error")
			assert.NotNil(t, result.AutogrowPolicy.Upsert, "Upsert function should be created")

			if tt.expectRead {
				assert.NotNil(t, result.AutogrowPolicy.Read, "Old policy should be read")
				assert.Equal(t, tt.expectedOldName, result.AutogrowPolicy.Read.Name(), "Old policy name should match")
			} else {
				assert.Nil(t, result.AutogrowPolicy.Read, "Old policy should not exist")
			}

			// Execute upsert
			result.AutogrowPolicy.Upsert(tt.newPolicy)

			// Verify upsert worked
			autogrowPolicies.rlock()
			storedPolicy, exists := autogrowPolicies.data[tt.policyID]
			autogrowPolicies.runlock()

			assert.True(t, exists, "Policy should exist after upsert")
			assert.Equal(t, tt.newPolicy, storedPolicy, "Stored policy should match new policy")

			// Cleanup
			autogrowPolicies.lock()
			autogrowPolicies.data = make(map[string]SmartCopier)
			autogrowPolicies.unlock()
		})
	}
}

// TestDeleteAutogrowPolicy tests the DeleteAutogrowPolicy function
func TestDeleteAutogrowPolicy(t *testing.T) {
	tests := []struct {
		name          string
		setupPolicies map[string]*storage.AutogrowPolicy
		policyID      string
		expectRead    bool
	}{
		{
			name:          "delete non-existing policy",
			setupPolicies: map[string]*storage.AutogrowPolicy{},
			policyID:      "nonexistent",
			expectRead:    false,
		},
		{
			name: "delete existing policy",
			setupPolicies: map[string]*storage.AutogrowPolicy{
				"policy1": storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess),
			},
			policyID:   "policy1",
			expectRead: true,
		},
		{
			name: "delete policy in Deleting state",
			setupPolicies: map[string]*storage.AutogrowPolicy{
				"policy2": storage.NewAutogrowPolicy("policy2", "90", "30", "2000Gi", storage.AutogrowPolicyStateDeleting),
			},
			policyID:   "policy2",
			expectRead: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			autogrowPolicies.lock()
			autogrowPolicies.data = make(map[string]SmartCopier)
			for k, v := range tt.setupPolicies {
				autogrowPolicies.data[k] = v
			}
			autogrowPolicies.unlock()

			// Execute
			subquery := DeleteAutogrowPolicy(tt.policyID)
			result := &Result{}
			err := subquery.setResults(&subquery, result)

			// Assert
			assert.NoError(t, err, "DeleteAutogrowPolicy setResults should not error")
			assert.NotNil(t, result.AutogrowPolicy.Delete, "Delete function should be created")

			if tt.expectRead {
				assert.NotNil(t, result.AutogrowPolicy.Read, "Policy should be read before delete")
				assert.Equal(t, tt.policyID, result.AutogrowPolicy.Read.Name(), "Read policy name should match")
			} else {
				assert.Nil(t, result.AutogrowPolicy.Read, "Policy should not be found")
			}

			// Execute delete
			result.AutogrowPolicy.Delete()

			// Verify delete worked
			autogrowPolicies.rlock()
			_, exists := autogrowPolicies.data[tt.policyID]
			autogrowPolicies.runlock()

			assert.False(t, exists, "Policy should not exist after delete")

			// Cleanup
			autogrowPolicies.lock()
			autogrowPolicies.data = make(map[string]SmartCopier)
			autogrowPolicies.unlock()
		})
	}
}

// TestAutogrowPolicySubqueryProperties tests that autogrow policy subqueries have correct properties
func TestAutogrowPolicySubqueryProperties(t *testing.T) {
	tests := []struct {
		name          string
		subqueryFunc  func() Subquery
		expectedRes   resource
		expectedOp    operation
		expectedID    string
		hasSetResults bool
	}{
		{
			name:          "ListAutogrowPolicies",
			subqueryFunc:  ListAutogrowPolicies,
			expectedRes:   autogrowPolicy,
			expectedOp:    list,
			hasSetResults: true,
		},
		{
			name:          "ReadAutogrowPolicy",
			subqueryFunc:  func() Subquery { return ReadAutogrowPolicy("policy1") },
			expectedRes:   autogrowPolicy,
			expectedOp:    read,
			expectedID:    "policy1",
			hasSetResults: true,
		},
		{
			name:          "InconsistentReadAutogrowPolicy",
			subqueryFunc:  func() Subquery { return InconsistentReadAutogrowPolicy("policy1") },
			expectedRes:   autogrowPolicy,
			expectedOp:    inconsistentRead,
			expectedID:    "policy1",
			hasSetResults: true,
		},
		{
			name:          "UpsertAutogrowPolicy",
			subqueryFunc:  func() Subquery { return UpsertAutogrowPolicy("policy1") },
			expectedRes:   autogrowPolicy,
			expectedOp:    upsert,
			expectedID:    "policy1",
			hasSetResults: true,
		},
		{
			name:          "DeleteAutogrowPolicy",
			subqueryFunc:  func() Subquery { return DeleteAutogrowPolicy("policy1") },
			expectedRes:   autogrowPolicy,
			expectedOp:    del,
			expectedID:    "policy1",
			hasSetResults: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			subquery := tt.subqueryFunc()

			assert.Equal(t, tt.expectedRes, subquery.res, "Resource type should match")
			assert.Equal(t, tt.expectedOp, subquery.op, "Operation type should match")
			assert.Equal(t, tt.expectedID, subquery.id, "ID should match")

			if tt.hasSetResults {
				assert.NotNil(t, subquery.setResults, "setResults function should be set")
			} else {
				assert.Nil(t, subquery.setResults, "setResults function should not be set")
			}
		})
	}
}

// TestListAutogrowPoliciesWithFilter tests filtering logic in ListAutogrowPolicies
func TestListAutogrowPoliciesWithFilter(t *testing.T) {
	// Setup multiple policies with different states
	autogrowPolicies.lock()
	autogrowPolicies.data = make(map[string]SmartCopier)
	autogrowPolicies.data["policy1"] = storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
	autogrowPolicies.data["policy2"] = storage.NewAutogrowPolicy("policy2", "90", "30", "2000Gi", storage.AutogrowPolicyStateFailed)
	autogrowPolicies.data["policy3"] = storage.NewAutogrowPolicy("policy3", "75", "25", "1500Gi", storage.AutogrowPolicyStateDeleting)
	autogrowPolicies.unlock()

	// Execute
	subquery := ListAutogrowPolicies()
	result := &Result{}
	err := subquery.setResults(&subquery, result)

	// Assert - should return all policies regardless of state (filter returns true for all)
	assert.NoError(t, err)
	assert.Len(t, result.AutogrowPolicies, 3, "Should return all policies")

	// Verify we got all three states
	stateMap := make(map[storage.AutogrowPolicyState]bool)
	for _, policy := range result.AutogrowPolicies {
		stateMap[policy.State()] = true
	}
	assert.True(t, stateMap[storage.AutogrowPolicyStateSuccess], "Should have Success state")
	assert.True(t, stateMap[storage.AutogrowPolicyStateFailed], "Should have Failed state")
	assert.True(t, stateMap[storage.AutogrowPolicyStateDeleting], "Should have Deleting state")

	// Cleanup
	autogrowPolicies.lock()
	autogrowPolicies.data = make(map[string]SmartCopier)
	autogrowPolicies.unlock()
}

// TestAutogrowPolicySmartCopy tests that SmartCopy is used correctly
func TestAutogrowPolicySmartCopy(t *testing.T) {
	// Create a policy and add it to cache
	originalPolicy := storage.NewAutogrowPolicy("policy1", "80", "20", "1000Gi", storage.AutogrowPolicyStateSuccess)
	originalPolicy.AddVolume("vol1")
	originalPolicy.AddVolume("vol2")

	autogrowPolicies.lock()
	autogrowPolicies.data = make(map[string]SmartCopier)
	autogrowPolicies.data["policy1"] = originalPolicy
	autogrowPolicies.unlock()

	// Read the policy
	subquery := ReadAutogrowPolicy("policy1")
	result := &Result{}
	err := subquery.setResults(&subquery, result)

	assert.NoError(t, err)
	assert.NotNil(t, result.AutogrowPolicy.Read)

	// Since SmartCopy returns self for interior mutability, they should be the same pointer
	assert.Equal(t, originalPolicy, result.AutogrowPolicy.Read, "SmartCopy should return the same instance for interior mutability")

	// Verify volume associations are accessible
	assert.True(t, result.AutogrowPolicy.Read.HasVolume("vol1"))
	assert.True(t, result.AutogrowPolicy.Read.HasVolume("vol2"))
	assert.Equal(t, 2, result.AutogrowPolicy.Read.VolumeCount())

	// Cleanup
	autogrowPolicies.lock()
	autogrowPolicies.data = make(map[string]SmartCopier)
	autogrowPolicies.unlock()
}
