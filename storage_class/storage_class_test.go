// Copyright 2021 NetApp, Inc. All Rights Reserved.

package storageclass

import (
	"context"
	"testing"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/v21/storage"
)

func TestDoesPoolSupportTopology(t *testing.T) {
	log.Debug("Running TestDoesPoolSupportTopology...")

	supportedTopologies := make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	fakePool1 := storage.Pool{
		Name:                "fake pool 1",
		SupportedTopologies: supportedTopologies,
	}

	supported := isTopologySupportedByPool(context.TODO(), &fakePool1,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	if !supported {
		t.Error("pool should support topology")
	}

	supported = isTopologySupportedByPool(context.TODO(), &fakePool1,
		map[string]string{"topology.kubernetes.io/region": "R1"})
	if !supported {
		t.Error("pool should support topology")
	}

	supported = isTopologySupportedByPool(context.TODO(), &fakePool1,
		map[string]string{"topology.kubernetes.io/region": "Not supported"})
	if supported {
		t.Error("pool should not support topology")
	}

	supportedTopologies = make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	supportedTopologies = append(supportedTopologies, map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	fakePool2 := storage.Pool{
		Name:                "fake pool 2",
		SupportedTopologies: supportedTopologies,
	}

	supported = isTopologySupportedByPool(nil, &fakePool2, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	if !supported {
		t.Error("pool should support topology")
	}

	supportedTopologies = make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies, map[string]string{"topology.kubernetes.io/region": "R1"})
	fakePool3 := storage.Pool{
		Name:                "fake pool 2",
		SupportedTopologies: supportedTopologies,
	}

	supported = isTopologySupportedByPool(nil, &fakePool3, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	if !supported {
		t.Error("pool should support topology")
	}
}

func TestFilterPoolsOnTopology(t *testing.T) {
	log.Debug("Running TestFilterPoolsOnTopology...")

	fakePools := make([]*storage.Pool, 0)
	supportedTopologies := make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	fakePool1 := storage.Pool{
		Name:                "fake pool 1",
		SupportedTopologies: supportedTopologies,
	}
	fakePools = append(fakePools, &fakePool1)

	supportedTopologies = make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	supportedTopologies = append(supportedTopologies, map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	fakePool2 := storage.Pool{
		Name:                "fake pool 2",
		SupportedTopologies: supportedTopologies,
	}
	fakePools = append(fakePools, &fakePool2)

	filteredPools := FilterPoolsOnTopology(nil, make([]*storage.Pool, 0), make([]map[string]string, 0))
	if len(filteredPools) != 0 {
		t.Error("matching pools should be empty")
	}

	filteredPools = FilterPoolsOnTopology(nil, make([]*storage.Pool, 0), nil)
	if len(filteredPools) != 0 {
		t.Error("matching pools should be empty")
	}

	requisiteTopologies := make([]map[string]string, 0)
	filteredPools = FilterPoolsOnTopology(nil, fakePools, requisiteTopologies)
	if len(filteredPools) != 2 {
		t.Error("all pools should be returned when requisite topology is empty")
	}

	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies, map[string]string{"topology.kubernetes.io/region": "R1"})
	filteredPools = FilterPoolsOnTopology(nil, fakePools, requisiteTopologies)
	if len(filteredPools) != 2 {
		t.Error("all pools should be returned")
	}

	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	filteredPools = FilterPoolsOnTopology(nil, fakePools, requisiteTopologies)
	if len(filteredPools) != 2 {
		t.Error("all pools should be returned")
	}

	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies, map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	filteredPools = FilterPoolsOnTopology(nil, fakePools, requisiteTopologies)
	if len(filteredPools) != 1 {
		t.Error("only fake pool 2 should be returned")
	}

	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	requisiteTopologies = append(requisiteTopologies, map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	filteredPools = FilterPoolsOnTopology(nil, fakePools, requisiteTopologies)
	if len(filteredPools) != 2 {
		t.Error("all pools should be returned")
	}

	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies, map[string]string{"topology.kubernetes.io/region": "I do not exist", "topology.kubernetes.io/zone": "Z2"})
	filteredPools = FilterPoolsOnTopology(nil, fakePools, requisiteTopologies)
	if len(filteredPools) != 0 {
		t.Error("no pools should match")
	}

	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	requisiteTopologies = append(requisiteTopologies, map[string]string{"topology.kubernetes.io/region": "R3", "topology.kubernetes.io/zone": "Z3"})
	filteredPools = FilterPoolsOnTopology(nil, fakePools, requisiteTopologies)
	if len(filteredPools) != 2 {
		t.Error("all pools should be returned")
	}

	fakePoolNoRestriction := storage.Pool{
		Name: "fake pool unrestricted",
	}
	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	requisiteTopologies = append(requisiteTopologies, map[string]string{"topology.kubernetes.io/region": "R3", "topology.kubernetes.io/zone": "Z3"})
	filteredPools = FilterPoolsOnTopology(nil, []*storage.Pool{&fakePoolNoRestriction}, requisiteTopologies)
	if len(filteredPools) == 0 {
		t.Error("all pools should be returned")
	}

	fakePoolNoRestriction = storage.Pool{
		Name:                "fake pool unrestricted",
		SupportedTopologies: make([]map[string]string, 0),
	}
	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	requisiteTopologies = append(requisiteTopologies, map[string]string{"topology.kubernetes.io/region": "R3", "topology.kubernetes.io/zone": "Z3"})
	filteredPools = FilterPoolsOnTopology(nil, []*storage.Pool{&fakePoolNoRestriction}, requisiteTopologies)
	if len(filteredPools) == 0 {
		t.Error("all pools should be returned")
	}

}

func TestSortPoolsByPreferredTopologies(t *testing.T) {
	log.Debug("Running TestSortPoolsByPreferredTopologies...")

	fakePools := make([]*storage.Pool, 0)
	supportedTopologies := make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	fakePool1 := storage.Pool{
		Name:                "fake pool 1",
		SupportedTopologies: supportedTopologies,
	}
	fakePools = append(fakePools, &fakePool1)

	supportedTopologies = make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	supportedTopologies = append(supportedTopologies, map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	fakePool2 := storage.Pool{
		Name:                "fake pool 2",
		SupportedTopologies: supportedTopologies,
	}
	fakePools = append(fakePools, &fakePool2)

	orderedPools := SortPoolsByPreferredTopologies(nil, make([]*storage.Pool, 0), make([]map[string]string, 0))
	if len(orderedPools) != 0 {
		t.Error("matching pools should be empty")
	}

	preferredTopologies := make([]map[string]string, 0)
	orderedPools = SortPoolsByPreferredTopologies(nil, fakePools, preferredTopologies)
	if len(orderedPools) != 2 {
		t.Errorf("2 pools should be returned, got %d", len(orderedPools))
	}

	preferredTopologies = make([]map[string]string, 0)
	preferredTopologies = append(preferredTopologies, map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	orderedPools = SortPoolsByPreferredTopologies(nil, fakePools, preferredTopologies)
	if len(orderedPools) != 2 {
		t.Errorf("2 pools should be returned, got %d", len(orderedPools))
	}

	preferredTopologies = make([]map[string]string, 0)
	preferredTopologies = append(preferredTopologies, map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	orderedPools = SortPoolsByPreferredTopologies(nil, fakePools, preferredTopologies)
	if len(orderedPools) != 2 {
		t.Errorf("2 pools should be returned, got %d", len(orderedPools))
	}
	if orderedPools[0].Name != fakePool2.Name {
		t.Errorf("fake pool 2 should be first in the list")
	}

	supportedTopologies = make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies, map[string]string{"topology.kubernetes.io/region": "R3", "topology.kubernetes.io/zone": "Z3"})
	fakePool3 := storage.Pool{
		Name:                "fake pool 3",
		SupportedTopologies: supportedTopologies,
	}
	fakePools = append(fakePools, &fakePool3)

	preferredTopologies = make([]map[string]string, 0)
	preferredTopologies = append(preferredTopologies, map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	orderedPools = SortPoolsByPreferredTopologies(nil, fakePools, preferredTopologies)
	if len(orderedPools) != 3 {
		t.Errorf("2 pools should be returned, got %d", len(orderedPools))
	}
	if orderedPools[0].Name != fakePool2.Name {
		t.Errorf("fake pool 2 should be first in the list")
	}

	preferredTopologies = make([]map[string]string, 0)
	preferredTopologies = append(preferredTopologies, map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	preferredTopologies = append(preferredTopologies, map[string]string{"topology.kubernetes.io/region": "R3", "topology.kubernetes.io/zone": "Z3"})
	orderedPools = SortPoolsByPreferredTopologies(nil, fakePools, preferredTopologies)
	if len(orderedPools) != 3 {
		t.Errorf("3 pools should be returned, got %d", len(orderedPools))
	}
	if orderedPools[0].Name != fakePool2.Name {
		t.Errorf("fake pool 2 should be first in the list")
	}
	if orderedPools[1].Name != fakePool3.Name {
		t.Errorf("fake pool 3 should be second in the list")
	}
	if orderedPools[2].Name != fakePool1.Name {
		t.Errorf("fake pool 1 should be third in the list")
	}

	preferredTopologies = make([]map[string]string, 0)
	preferredTopologies = append(preferredTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	orderedPools = SortPoolsByPreferredTopologies(nil, fakePools, preferredTopologies)
	if len(orderedPools) != 3 {
		t.Errorf("3 pools should be returned, got %d", len(orderedPools))
	}
	if orderedPools[2].Name != fakePool3.Name {
		t.Errorf("fake pool 3 should be third in the list, got %s, %s, %s",
			orderedPools[0].Name, orderedPools[1].Name, orderedPools[2].Name)
	}

	preferredTopologies = make([]map[string]string, 0)
	preferredTopologies = append(preferredTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1"})
	orderedPools = SortPoolsByPreferredTopologies(nil, fakePools, preferredTopologies)
	if len(orderedPools) != 3 {
		t.Errorf("3 pools should be returned, got %d", len(orderedPools))
	}
	if orderedPools[2].Name != fakePool3.Name {
		t.Errorf("fake pool 3 should be third in the list, got %s, %s, %s",
			orderedPools[0].Name, orderedPools[1].Name, orderedPools[2].Name)
	}

	preferredTopologies = make([]map[string]string, 0)
	orderedPools = SortPoolsByPreferredTopologies(nil, fakePools, preferredTopologies)
	if len(orderedPools) != 3 {
		t.Errorf("3 pools should be returned, got %d", len(orderedPools))
	}
}
