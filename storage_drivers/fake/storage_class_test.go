// Copyright 2021 NetApp, Inc. All Rights Reserved.

package fake

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	sc "github.com/netapp/trident/storage_class"
	tu "github.com/netapp/trident/storage_drivers/fake/test_utils"
)

var ctx = context.Background

func TestAttributeMatches(t *testing.T) {
	mockPools := tu.GetFakePools()
	volumes := make([]fake.Volume, 0)
	fakeConfig, err := NewFakeStorageDriverConfigJSON("mock", config.File, mockPools, volumes)
	if err != nil {
		t.Fatalf("Unable to construct config JSON.")
	}
	backend, err := NewFakeStorageBackend(ctx(), fakeConfig, uuid.New().String())
	if err != nil {
		t.Fatalf("Unable to construct backend using mock driver.")
	}
	for _, test := range []struct {
		name          string
		sc            *sc.StorageClass
		expectedPools []string
	}{
		{
			name: "Slow",
			sc: sc.New(&sc.Config{
				Name: "slow",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(40),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
				},
			}),
			expectedPools: []string{tu.SlowSnapshots},
		},
		{
			// Tests that a request for false will return backends offering
			// both true and false
			name: "Bool",
			sc: sc.New(&sc.Config{
				Name: "no-snapshots",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(40),
					sa.Snapshots:        sa.NewBoolRequest(false),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
				},
			}),
			expectedPools: []string{tu.SlowSnapshots, tu.SlowNoSnapshots},
		},
		{
			name: "Fast",
			sc: sc.New(&sc.Config{
				Name: "fast",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(2000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
				},
			}),
			expectedPools: []string{
				tu.FastSmall, tu.FastThinOnly,
				tu.FastUniqueAttr,
			},
		},
		{
			// This tests the correctness of matching string requests, using
			// ProvisioningType
			name: "String",
			sc: sc.New(&sc.Config{
				Name: "string",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(2000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thick"),
				},
			}),
			expectedPools: []string{tu.FastSmall, tu.FastUniqueAttr},
		},
		{
			// This uses sa.UniqueOptions to test that StorageClass only
			// matches backends that have all required attributes.
			name: "Unique",
			sc: sc.New(&sc.Config{
				Name: "unique",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(2000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thick"),
					sa.UniqueOptions:    sa.NewStringRequest("bar"),
				},
			}),
			expectedPools: []string{tu.FastUniqueAttr},
		},
		{
			// Tests overlapping int ranges
			name: "Overlap",
			sc: sc.New(&sc.Config{
				Name: "overlap",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(1000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
				},
			}),
			expectedPools: []string{
				tu.FastSmall, tu.FastThinOnly,
				tu.FastUniqueAttr, tu.MediumOverlap,
			},
		},
		// BEGIN Failure tests
		{
			// Tests non-existent bool attribute
			name: "Bool failure",
			sc: sc.New(&sc.Config{
				Name: "bool-failure",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(1000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thick"),
					sa.NonexistentBool:  sa.NewBoolRequest(false),
				},
			}),
			expectedPools: []string{},
		},
		{
			name: "Invalid string value",
			sc: sc.New(&sc.Config{
				Name: "invalid-string",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(1000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thicker"),
				},
			}),
			expectedPools: []string{},
		},
		{
			name: "Invalid int range",
			sc: sc.New(&sc.Config{
				Name: "invalid-int",
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(300),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thick"),
				},
			}),
			expectedPools: []string{},
		},
	} {
		test.sc.CheckAndAddBackend(ctx(), backend)
		expectedMap := make(map[string]bool, len(test.expectedPools))
		for _, pool := range test.expectedPools {
			expectedMap[pool] = true
		}
		unmatched := make([]string, 0)
		matched := make([]string, 0)
		for _, vc := range test.sc.Pools() {
			if _, ok := expectedMap[vc.Name()]; !ok {
				unmatched = append(unmatched, vc.Name())
			} else {
				delete(expectedMap, vc.Name())
				matched = append(matched, vc.Name())
			}
		}
		if len(unmatched) > 0 {
			t.Errorf("%s:\n\tFailed to match pools: %s\n\tMatched:  %s", test.name, strings.Join(unmatched, ", "),
				strings.Join(matched, ", "))
		}
		if len(expectedMap) > 0 {
			expectedMatches := make([]string, 0)
			for k := range expectedMap {
				expectedMatches = append(expectedMatches, k)
			}
			t.Errorf("%s:\n\tExpected additional matches:  %s\n\tMatched: %s", test.name,
				strings.Join(expectedMatches, ", "), strings.Join(matched, ", "))
		}
	}
}

func TestAttributeMatchesWithVirtualPools(t *testing.T) {
	mockPools := tu.GetFakePools()
	vpool, vpools := tu.GetFakeVirtualPools()
	fakeConfig, err := NewFakeStorageDriverConfigJSONWithVirtualPools("mock", config.File, mockPools, vpool, vpools)
	if err != nil {
		t.Fatalf("Unable to construct config JSON.")
	}
	backend, err := NewFakeStorageBackend(ctx(), fakeConfig, uuid.New().String())
	if err != nil {
		t.Fatalf("Unable to construct backend using mock driver.")
	}
	for _, test := range []struct {
		name          string
		sc            *sc.StorageClass
		expectedPools []string
	}{
		{
			name: "Gold",
			sc: sc.New(&sc.Config{
				Name: "gold",
				Attributes: map[string]sa.Request{
					sa.Selector: sa.NewLabelRequestMustCompile("performance=gold"),
				},
			}),
			expectedPools: []string{"fake_useast_pool_0", "fake_useast_pool_3"},
		},
		{
			name: "Gold-Zone1",
			sc: sc.New(&sc.Config{
				Name: "Gold-Zone1",
				Attributes: map[string]sa.Request{
					sa.Zone:     sa.NewStringRequest("1"),
					sa.Selector: sa.NewLabelRequestMustCompile("performance=gold"),
				},
			}),
			expectedPools: []string{"fake_useast_pool_0"},
		},
		{
			name: "Gold-Zone2-AWS",
			sc: sc.New(&sc.Config{
				Name: "Gold-Zone2-AWS",
				Attributes: map[string]sa.Request{
					sa.Zone:     sa.NewStringRequest("2"),
					sa.Selector: sa.NewLabelRequestMustCompile("performance=gold; cloud in (aws,azure)"),
				},
			}),
			expectedPools: []string{"fake_useast_pool_3"},
		},
		{
			name: "Silver-Zone2-NotAWS",
			sc: sc.New(&sc.Config{
				Name: "Silver-Zone2-NotAWS",
				Attributes: map[string]sa.Request{
					sa.Zone:     sa.NewStringRequest("2"),
					sa.Selector: sa.NewLabelRequestMustCompile("performance=silver; cloud notin (aws)"),
				},
			}),
			expectedPools: []string{},
		},
		{
			name: "Silver-USEast-Zone2-AnyCloud",
			sc: sc.New(&sc.Config{
				Name: "Silver-USEast-Zone2-AnyCloud",
				Attributes: map[string]sa.Request{
					sa.Zone:     sa.NewStringRequest("2"),
					sa.Region:   sa.NewStringRequest("us-east"),
					sa.Selector: sa.NewLabelRequestMustCompile("performance=silver; cloud"),
				},
			}),
			expectedPools: []string{"fake_useast_pool_4"},
		},
		{
			name: "Silver-Zone2-NoCloud",
			sc: sc.New(&sc.Config{
				Name: "Silver-Zone2-NoCloud",
				Attributes: map[string]sa.Request{
					sa.Zone:     sa.NewStringRequest("2"),
					sa.Selector: sa.NewLabelRequestMustCompile("performance=bronze; !cloud"),
				},
			}),
			expectedPools: []string{},
		},
	} {
		test.sc.CheckAndAddBackend(ctx(), backend)
		expectedMap := make(map[string]bool, len(test.expectedPools))
		for _, pool := range test.expectedPools {
			expectedMap[pool] = true
		}
		unmatched := make([]string, 0)
		matched := make([]string, 0)
		for _, vc := range test.sc.Pools() {
			if _, ok := expectedMap[vc.Name()]; !ok {
				unmatched = append(unmatched, vc.Name())
			} else {
				delete(expectedMap, vc.Name())
				matched = append(matched, vc.Name())
			}
		}
		if len(unmatched) > 0 {
			t.Errorf("%s:\n\tFailed to match pools: %s\n\tMatched:  %s",
				test.name, strings.Join(unmatched, ", "), strings.Join(matched, ", "))
		}
		if len(expectedMap) > 0 {
			expectedMatches := make([]string, 0)
			for k := range expectedMap {
				expectedMatches = append(expectedMatches, k)
			}
			t.Errorf("%s:\n\tExpected additional matches:  %s\n\tMatched: %s",
				test.name, strings.Join(expectedMatches, ", "), strings.Join(matched, ", "))
		}
	}
}

func TestSpecificBackends(t *testing.T) {
	backends := make(map[string]*storage.StorageBackend)
	mockPools := tu.GetFakePools()
	for _, c := range []struct {
		name      string
		poolNames []string
	}{
		{
			name:      "fast-a",
			poolNames: []string{tu.FastSmall, tu.FastThinOnly},
		},
		{
			name:      "fast-b",
			poolNames: []string{tu.FastThinOnly, tu.FastUniqueAttr},
		},
		{
			name:      "slow",
			poolNames: []string{tu.SlowSnapshots, tu.SlowNoSnapshots, tu.MediumOverlap},
		},
	} {
		pools := make(map[string]*fake.StoragePool, len(c.poolNames))
		for _, poolName := range c.poolNames {
			pools[poolName] = mockPools[poolName]
		}
		volumes := make([]fake.Volume, 0)
		fakeConfig, err := NewFakeStorageDriverConfigJSON(c.name, config.File, pools, volumes)
		if err != nil {
			t.Fatalf("Unable to generate config JSON for %s:  %v", c.name, err)
		}
		backend, err := NewFakeStorageBackend(ctx(), fakeConfig, uuid.New().String())
		if err != nil {
			t.Fatalf("Unable to construct backend using mock driver.")
		}
		backends[c.name] = backend
	}
	for _, test := range []struct {
		name     string
		sc       *sc.StorageClass
		expected []*tu.PoolMatch
	}{
		{
			name: "Specific backends",
			sc: sc.New(&sc.Config{
				Name: "specific",
				AdditionalPools: map[string][]string{
					"fast-a": {tu.FastThinOnly},
					"slow":   {tu.SlowNoSnapshots, tu.MediumOverlap},
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
			},
		},
		{
			name: "Mixed attributes, backends",
			sc: sc.New(&sc.Config{
				Name: "mixed",
				AdditionalPools: map[string][]string{
					"slow":   {tu.SlowNoSnapshots},
					"fast-b": {tu.FastThinOnly, tu.FastUniqueAttr},
				},
				Attributes: map[string]sa.Request{
					sa.IOPS:             sa.NewIntRequest(2000),
					sa.Snapshots:        sa.NewBoolRequest(true),
					sa.ProvisioningType: sa.NewStringRequest("thin"),
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
			},
		},
	} {
		for _, backend := range backends {
			test.sc.CheckAndAddBackend(ctx(), backend)
		}
		for _, protocol := range []config.Protocol{config.File, config.Block} {
			for _, vc := range test.sc.GetStoragePoolsForProtocol(ctx(), protocol, config.ReadWriteOnce) {
				nameFound := false
				for _, scName := range vc.StorageClasses() {
					if scName == test.sc.GetName() {
						nameFound = true
						break
					}
				}
				if !nameFound {
					t.Errorf("%s: Storage class name not found in storage pool %s", test.name, vc.Name())
				}
				matchIndex := -1
				for i, e := range test.expected {
					if e.Matches(vc) {
						matchIndex = i
						break
					}
				}
				if matchIndex >= 0 {
					// If we match, remove the match from the potential matches.
					test.expected[matchIndex] = test.expected[len(test.expected)-1]
					test.expected[len(test.expected)-1] = nil
					test.expected = test.expected[:len(test.expected)-1]
				} else {
					t.Errorf("%s: Found unexpected match for storage class: %s:%s", test.name, vc.Backend().Name(),
						vc.Name())
				}
			}
		}
		if len(test.expected) > 0 {
			expectedNames := make([]string, len(test.expected))
			for i, e := range test.expected {
				expectedNames[i] = e.String()
			}
			t.Errorf("%s:  Storage class failed to match storage pools %s",
				test.name, strings.Join(expectedNames, ", "))
		}
	}
}

func TestRegex(t *testing.T) {
	backends := make(map[string]*storage.StorageBackend)
	mockPools := tu.GetFakePools()
	for _, c := range []struct {
		backendName string
		poolNames   []string
	}{
		{
			backendName: "fast-a",
			poolNames:   []string{tu.FastSmall, tu.FastThinOnly},
		},
		{
			backendName: "fast-b",
			poolNames:   []string{tu.FastThinOnly, tu.FastUniqueAttr},
		},
		{
			backendName: "slow",
			poolNames:   []string{tu.SlowSnapshots, tu.SlowNoSnapshots, tu.MediumOverlap},
		},
	} {
		pools := make(map[string]*fake.StoragePool, len(c.poolNames))
		for _, poolName := range c.poolNames {
			pools[poolName] = mockPools[poolName]
		}
		volumes := make([]fake.Volume, 0)
		fakeConfig, err := NewFakeStorageDriverConfigJSON(c.backendName, config.File, pools, volumes)
		if err != nil {
			t.Fatalf("Unable to generate config JSON for %s:  %v", c.backendName, err)
		}
		backend, err := NewFakeStorageBackend(ctx(), fakeConfig, uuid.New().String())
		if err != nil {
			t.Fatalf("Unable to construct backend using mock driver.")
		}
		backends[c.backendName] = backend
	}
	for _, test := range []struct {
		description string
		sc          *sc.StorageClass
		expected    []*tu.PoolMatch
	}{
		{
			description: "All pools for the 'slow' backend via regex '.*''",
			sc: sc.New(&sc.Config{
				Name: "regex1",
				Pools: map[string][]string{
					"slow": {".*"},
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
			},
		},
		{
			description: "TBD",
			sc: sc.New(&sc.Config{
				Name: "regex1b",
				Pools: map[string][]string{
					"slow": {"slow.*", tu.MediumOverlap},
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
			},
		},
		{
			description: "Implicitly exclude the medium-overlap pool via regex by only matching those starting with 'slow-'",
			sc: sc.New(&sc.Config{
				Name: "regex2",
				Pools: map[string][]string{
					"slow": {"slow-.*"},
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
			},
		},
		{
			description: "Backends whose names start with 'fast-' and all of their respective pools",
			sc: sc.New(&sc.Config{
				Name: "regex3",
				Pools: map[string][]string{
					"fast-.*": {".*"}, // All fast backends:  all their pools
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
			},
		},
		{
			description: "Backends {fast-a, slow} and all of their respective pools",
			sc: sc.New(&sc.Config{
				Name: "regex4",
				Pools: map[string][]string{
					"fast-a|slow": {".*"}, // fast-a or slow:  all their pools
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
			},
		},
		{
			description: "Backends {fast-a, slow} and all of their respective pools",
			sc: sc.New(&sc.Config{
				Name: "regex5",
				Pools: map[string][]string{
					"(fast-a|slow|NOT_ACTUALLY_THERE)": {".*"}, // fast-a or slow or 1 that doesn't exist:  all their pools
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
			},
		},
		{
			description: "Every backend and every pool",
			sc: sc.New(&sc.Config{
				Name: "regex6",
				Pools: map[string][]string{
					".*": {".*"}, // All backends:  all pools
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
			},
		},
		{
			description: "Exclusion test",
			sc: sc.New(&sc.Config{
				Name: "regex7",
				Pools: map[string][]string{
					".*": {".*"}, // Start off allowing all backends:  all pools
				},
				ExcludePools: map[string][]string{
					"slow": {tu.MediumOverlap}, // Now, specifically exclude backend 'slow':  tu.MediumOverlap pool
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-a", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastThinOnly},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
			},
		},
		{
			description: "Exclude the FastThinOnly pools from the fast backends",
			sc: sc.New(&sc.Config{
				Name: "regex8",
				Pools: map[string][]string{
					"fast-.*": {".*"}, // All fast backends:  all their pools
				},
				ExcludePools: map[string][]string{
					".*": {tu.FastThinOnly}, // Now, specifically exclude backend 'slow':  tu.MediumOverlap pool
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "fast-a", Pool: tu.FastSmall},
				{Backend: "fast-b", Pool: tu.FastUniqueAttr},
			},
		},
	} {
		// add the backends to the test StorageClass
		for _, backend := range backends {
			test.sc.CheckAndAddBackend(ctx(), backend)
		}
		// validate the results for the test StorageClass
		for _, protocol := range []config.Protocol{config.File, config.Block} {
			for _, pool := range test.sc.GetStoragePoolsForProtocol(ctx(), protocol, config.ReadWriteOnce) {
				nameFound := false
				for _, scName := range pool.StorageClasses() {
					if scName == test.sc.GetName() {
						nameFound = true
						break
					}
				}
				if !nameFound {
					t.Errorf("%s: Storage class name not found in storage pool: %s", test.description, pool.Name())
				}
				matchIndex := -1
				for i, e := range test.expected {
					if e.Matches(pool) {
						matchIndex = i
						break
					}
				}
				if matchIndex >= 0 {
					// If we match, remove the match from the potential matches.
					test.expected[matchIndex] = test.expected[len(test.expected)-1]
					test.expected[len(test.expected)-1] = nil
					test.expected = test.expected[:len(test.expected)-1]
				} else {
					t.Errorf("%s:  Found unexpected match for storage class: %s:%s", test.description,
						pool.Backend().Name(), pool.Name())
				}
			}
		}
		if len(test.expected) > 0 {
			expectedNames := make([]string, len(test.expected))
			for i, e := range test.expected {
				expectedNames[i] = e.String()
			}
			t.Errorf("%s:  Storage class failed to match storage pools %s", test.description,
				strings.Join(expectedNames, ", "))
		}
	}
}

// This tests some specific defect scenarios that were found
func TestRegex2(t *testing.T) {
	backends := make(map[string]*storage.StorageBackend)
	mockPools := tu.GetFakePools()
	for _, c := range []struct {
		backendName string
		poolNames   []string
	}{
		{
			backendName: "fast-a",
			poolNames:   []string{tu.FastSmall, tu.FastThinOnly},
		},
		{
			backendName: "fast-b",
			poolNames:   []string{tu.FastThinOnly, tu.FastUniqueAttr},
		},
		{
			backendName: "slow",
			poolNames:   []string{tu.SlowSnapshots, tu.SlowNoSnapshots, tu.MediumOverlap},
		},
		{
			backendName: "slower",
			poolNames:   []string{tu.FastThinOnly},
		},
	} {
		pools := make(map[string]*fake.StoragePool, len(c.poolNames))
		for _, poolName := range c.poolNames {
			pools[poolName] = mockPools[poolName]
		}
		volumes := make([]fake.Volume, 0)
		fakeConfig, err := NewFakeStorageDriverConfigJSON(c.backendName, config.File, pools, volumes)
		if err != nil {
			t.Fatalf("Unable to generate config JSON for %s:  %v", c.backendName, err)
		}
		backend, err := NewFakeStorageBackend(ctx(), fakeConfig, uuid.New().String())
		if err != nil {
			t.Fatalf("Unable to construct backend using mock driver.")
		}
		backends[c.backendName] = backend
	}
	for _, test := range []struct {
		description string
		sc          *sc.StorageClass
		expected    []*tu.PoolMatch
	}{
		{
			description: "slow only, not slow+slower",
			sc: sc.New(&sc.Config{
				Name: "regex1",
				Pools: map[string][]string{
					"slow": {".*"},
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
			},
		},
		{
			description: "slow only, not slow+slower",
			sc: sc.New(&sc.Config{
				Name: "regex2",
				Pools: map[string][]string{
					"^slow$": {".*"},
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
			},
		},
		{
			description: "slow and slower combined",
			sc: sc.New(&sc.Config{
				Name: "regex2",
				Pools: map[string][]string{
					"slow.*": {".*"},
				},
			}),
			expected: []*tu.PoolMatch{
				{Backend: "slow", Pool: tu.SlowSnapshots},
				{Backend: "slow", Pool: tu.SlowNoSnapshots},
				{Backend: "slow", Pool: tu.MediumOverlap},
				{Backend: "slower", Pool: tu.FastThinOnly},
			},
		},
	} {
		// add the backends to the test StorageClass
		for _, backend := range backends {
			test.sc.CheckAndAddBackend(ctx(), backend)
		}
		// validate the results for the test StorageClass
		for _, protocol := range []config.Protocol{config.File, config.Block} {
			for _, pool := range test.sc.GetStoragePoolsForProtocol(ctx(), protocol, config.ReadWriteOnce) {
				nameFound := false
				for _, scName := range pool.StorageClasses() {
					if scName == test.sc.GetName() {
						nameFound = true
						break
					}
				}
				if !nameFound {
					t.Errorf("%s: Storage class name not found in storage pool: %s", test.description, pool.Name())
				}
				matchIndex := -1
				for i, e := range test.expected {
					if e.Matches(pool) {
						matchIndex = i
						break
					}
				}
				if matchIndex >= 0 {
					// If we match, remove the match from the potential matches.
					test.expected[matchIndex] = test.expected[len(test.expected)-1]
					test.expected[len(test.expected)-1] = nil
					test.expected = test.expected[:len(test.expected)-1]
				} else {
					t.Errorf("%s:  Found unexpected match for storage class: %s:%s", test.description,
						pool.Backend().Name(), pool.Name())
				}
			}
		}
		if len(test.expected) > 0 {
			expectedNames := make([]string, len(test.expected))
			for i, e := range test.expected {
				expectedNames[i] = e.String()
			}
			t.Errorf("%s: Storage class failed to match storage pools %s", test.description,
				strings.Join(expectedNames, ", "))
		}
	}
}
