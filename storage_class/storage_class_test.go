// Copyright 2022 NetApp, Inc. All Rights Reserved.

package storageclass

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	mockstorage "github.com/netapp/trident/mocks/mock_storage"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func TestNewForConfig(t *testing.T) {
	bronzeStorageConfig := &Config{
		Name:            "bronze",
		Attributes:      make(map[string]sa.Request),
		AdditionalPools: make(map[string][]string),
	}
	bronzeStorageConfig.Attributes["media"] = sa.NewStringRequest("hdd")
	bronzeStorageConfig.AdditionalPools["ontapnas"] = []string{"aggr1"}
	bronzeStorageConfig.AdditionalPools["ontapsan"] = []string{"aggr1"}

	bronzeStorageClass := New(bronzeStorageConfig)

	assert.NotNil(t, bronzeStorageClass)
}

func TestNewStorageClass(t *testing.T) {
	jsonInput1 := `
	{
	    "apiVersion": "storage.k8s.io/v1",
	    "kind": "StorageClass",
	    "metadata": {
	        "name": "basic"
	    },
	    "provisioner": "csi.trident.netapp.io",
	    "parameters": {
	        "backendType": "__BACKEND_TYPE__",
	        "fsType": "__FILESYSTEM_TYPE__"
	    }
	}`
	jsonInput2 := "apiVersion: storage.k8s.io/v1\nkind: StorageClass\nmetadata:\n  name: basic\nprovisioner: " +
		"csi.trident.netapp.io\nparameters:\n  backendType: \"__BACKEND_TYPE__\"\n  fsType: \"__FILESYSTEM_TYPE__\""

	newSC1, err1 := NewForConfig(jsonInput1)
	newSC2, err2 := NewForConfig(jsonInput2)

	assert.Nil(t, err1)
	assert.NotNil(t, newSC1)

	assert.NotNil(t, err2)
	assert.Nil(t, newSC2)
}

func TestNewFromPersistent(t *testing.T) {
	bronzeStorageConfig := &Config{
		Name:            "bronze",
		Attributes:      make(map[string]sa.Request),
		AdditionalPools: make(map[string][]string),
	}
	bronzeStorageConfig.Attributes["media"] = sa.NewStringRequest("hdd")
	bronzeStorageConfig.AdditionalPools["ontapnas"] = []string{"aggr1"}
	bronzeStorageConfig.AdditionalPools["ontapsan"] = []string{"aggr1"}

	psc := Persistent{Config: bronzeStorageConfig}

	newSC := NewFromPersistent(&psc)

	assert.NotNil(t, newSC)
}

func TestConstructPersistent(t *testing.T) {
	bronzeStorageConfig := &Config{
		Name:            "bronze",
		Attributes:      make(map[string]sa.Request),
		AdditionalPools: make(map[string][]string),
		Pools:           map[string][]string{},
	}
	bronzeStorageConfig.Attributes["media"] = sa.NewStringRequest("hdd")
	bronzeStorageConfig.AdditionalPools["ontapnas"] = []string{"aggr1"}
	bronzeStorageConfig.AdditionalPools["ontapsan"] = []string{"aggr1"}
	bronzeStorageConfig.Pools["backend1"] = []string{"pool1"}

	bronzeStorageClass := New(bronzeStorageConfig)

	psc := bronzeStorageClass.ConstructPersistent()

	assert.NotNil(t, psc)

	pscName := psc.GetName()
	assert.Equal(t, "bronze", pscName)
}

func TestStorageClassPools(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	fakePool1 := mockstorage.NewMockPool(mockCtrl)
	fakePool1.EXPECT().Name().Return("fake pool 1").AnyTimes()

	fakePool2 := mockstorage.NewMockPool(mockCtrl)
	fakePool2.EXPECT().Name().Return("fake pool 2").AnyTimes()

	sc := New(&Config{
		Name: "invalid-int",
		Attributes: map[string]sa.Request{
			sa.IOPS:             sa.NewIntRequest(300),
			sa.Snapshots:        sa.NewBoolRequest(true),
			sa.ProvisioningType: sa.NewStringRequest("thick"),
		},
	})

	sc.pools = append(sc.pools, fakePool1, fakePool2)

	assert.Equal(t, 2, len(sc.Pools()))
}

func TestNewFromAttributes(t *testing.T) {
	attributes := make(map[string]sa.Request, 0)
	attributes["backendType"] = sa.NewStringRequest("ontap-nas")
	attributes["media"] = sa.NewStringRequest("hdd")
	attributes["encryption"] = sa.NewStringRequest("true")

	newSC := NewFromAttributes(attributes)

	assert.NotNil(t, newSC)
	assert.Equal(t, "ontap-nas", newSC.GetAttributes()["backendType"].String())
}

func TestGetStoragePoolsForProtocolByBackend(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	ctx := context.TODO()

	supportedTopologies1 := make([]map[string]string, 0)
	supportedTopologies1 = append(supportedTopologies1,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	fakePool1 := mockstorage.NewMockPool(mockCtrl)
	fakePool1.EXPECT().Name().Return("fake pool 1").AnyTimes()
	fakePool1.EXPECT().SupportedTopologies().Return(supportedTopologies1).AnyTimes()

	supportedTopologies2 := make([]map[string]string, 0)
	supportedTopologies2 = append(supportedTopologies2,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	fakePool2 := mockstorage.NewMockPool(mockCtrl)
	fakePool2.EXPECT().Name().Return("fake pool 2").AnyTimes()
	fakePool2.EXPECT().SupportedTopologies().Return(supportedTopologies2).AnyTimes()

	sc := New(&Config{
		Name: "sc1",
		Attributes: map[string]sa.Request{
			sa.Snapshots:        sa.NewBoolRequest(true),
			sa.ProvisioningType: sa.NewStringRequest("thick"),
		},
	})

	sc.pools = append(sc.pools, fakePool1, fakePool2)
	requisiteTopologies := make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	preferredTopologies := make([]map[string]string, 0)
	preferredTopologies = append(preferredTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})

	backend1 := mockstorage.NewMockBackend(mockCtrl)
	fakePool1.EXPECT().Backend().Return(backend1)
	backend1.EXPECT().GetProtocol(ctx).Return(config.File).AnyTimes()

	backend2 := mockstorage.NewMockBackend(mockCtrl)
	fakePool2.EXPECT().Backend().Return(backend2)
	backend2.EXPECT().GetProtocol(ctx).Return(config.Block).AnyTimes()

	// Check for valid case
	pools := sc.GetStoragePoolsForProtocolByBackend(ctx, config.File, requisiteTopologies, preferredTopologies,
		config.ReadWriteMany)

	assert.Equal(t, 1, len(pools))
	assert.Equal(t, "fake pool 1", pools[0].Name())

	sc2 := New(&Config{
		Name: "sc2",
		Attributes: map[string]sa.Request{
			sa.Snapshots:        sa.NewBoolRequest(true),
			sa.ProvisioningType: sa.NewStringRequest("thick"),
		},
	})

	// Check for invalid case
	pools2 := sc2.GetStoragePoolsForProtocolByBackend(ctx, config.File, requisiteTopologies, preferredTopologies,
		config.ReadWriteMany)

	assert.Equal(t, 0, len(pools2))
}

func TestStorageClassMatches(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)
	pools := make(map[string][]string, 0)

	fakePool1, backend1 := getFakeStoragePool(mockCtrl, "fakepool1", "backend1",
		"ontap-nas", "sc1", nil)

	fakePool2, _ := getFakeStoragePool(mockCtrl, "fakepool2", "backend1",
		"ontap-nas", "sc1", backend1)

	fakePool3, _ := getFakeStoragePool(mockCtrl, "fakepool3", "backend2",
		"ontap-nas", "sc1", nil)

	fakePool4, _ := getFakeStoragePool(mockCtrl, "fakepool4", "backend3",
		"ontap-nas", "sc1", nil)

	fakePool5, _ := getFakeStoragePool(mockCtrl, "fakepool5", "backend4",
		"selector", "sc1", nil)

	pools["backend1"] = []string{fakePool1.Name(), fakePool2.Name(), fakePool5.Name()}
	pools["backend2"] = []string{fakePool3.Name()}

	excludePools := make(map[string][]string, 0)
	excludePools["backend2"] = []string{fakePool3.Name()}

	additionalPools := make(map[string][]string, 0)
	additionalPools["backend3"] = []string{fakePool4.Name()}

	sc1 := New(&Config{
		Name: "sc1",
		Attributes: map[string]sa.Request{
			sa.BackendType: sa.NewStringRequest("ontap-nas"),
		},
		Pools:           pools,
		AdditionalPools: additionalPools,
		ExcludePools:    excludePools,
	})

	sc2 := New(&Config{
		Name:            "sc2",
		AdditionalPools: additionalPools,
		ExcludePools:    excludePools,
	})

	sc3 := New(&Config{
		Name: "sc3",
		Attributes: map[string]sa.Request{
			"selector": sa.NewStringRequest("ontap-nas"),
		},
		AdditionalPools: additionalPools,
		ExcludePools:    excludePools,
	})

	type storageClassTest struct {
		storagePool  *mockstorage.MockPool
		storageClass *StorageClass
		result       bool
	}

	tests := map[string]storageClassTest{
		"StorageClassSuccess":         {storagePool: fakePool1, storageClass: sc1, result: true},
		"CreatePoolInExistingBackend": {storagePool: fakePool2, storageClass: sc1, result: true},
		"ExcludePool":                 {storagePool: fakePool3, storageClass: sc1, result: false},
		"AddPool":                     {storagePool: fakePool4, storageClass: sc1, result: true},
		"BackendType_selector":        {storagePool: fakePool5, storageClass: sc1, result: false},
		"AttributesNil":               {storagePool: fakePool5, storageClass: sc2, result: false},
		"PoolsNil":                    {storagePool: fakePool5, storageClass: sc3, result: false},
	}

	for testName, test := range tests {
		t.Run(fmt.Sprintf("%s:", testName), func(t *testing.T) {
			assert.Equal(t, test.storageClass.Matches(ctx, test.storagePool), test.result,
				"storage pool does not match ")
		})
	}
}

func TestConstructExternal(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)

	fakePool1, backend1 := getFakeStoragePool(mockCtrl, "fake pool 1", "backend1",
		"ontap-nas", "sc1", nil)

	fakePool2, _ := getFakeStoragePool(mockCtrl, "fake pool 2", "",
		"ontap-nas", "sc1", nil)
	fakePool3, _ := getFakeStoragePool(mockCtrl, "fake pool 2", "",
		"ontap-nas", "sc1", backend1)

	pools := make(map[string][]string, 0)
	pools["backend1"] = []string{"fakepool1"}

	additionalPools := make(map[string][]string, 0)
	additionalPools["backend2"] = []string{"fakepool2"}
	additionalPools["backend2"] = []string{"fakepool3"}

	sc := New(&Config{
		Name: "sc1",
		Attributes: map[string]sa.Request{
			sa.Snapshots:        sa.NewBoolRequest(true),
			sa.ProvisioningType: sa.NewStringRequest("thick"),
		},
		Pools:           pools,
		AdditionalPools: additionalPools,
	})
	sc.pools = append(sc.pools, fakePool1, fakePool2, fakePool3)
	ext := sc.ConstructExternal(ctx)

	assert.NotNil(t, ext, "Storage pool object not found")
	assert.Equal(t, "sc1", ext.GetName(), "storage class name does not match")
}

func TestIsTopologySupportedByTopology(t *testing.T) {
	tests := map[string]struct {
		requisiteTopology, availableTopology map[string]string
		expected                             bool
	}{
		"No entries": {
			requisiteTopology: map[string]string{},
			availableTopology: map[string]string{},
			expected:          true,
		},
		"Identical single entry": {
			requisiteTopology: map[string]string{"region": "R1"},
			availableTopology: map[string]string{"region": "R1"},
			expected:          true,
		},
		"Empty requisite single entry": {
			requisiteTopology: map[string]string{},
			availableTopology: map[string]string{"region": "R1"},
			expected:          true,
		},
		"Empty available single entry": {
			requisiteTopology: map[string]string{"region": "R1"},
			availableTopology: map[string]string{},
			expected:          true,
		},
		"Identical multiple entries": {
			requisiteTopology: map[string]string{"region": "R1", "zone": "Z1"},
			availableTopology: map[string]string{"region": "R1", "zone": "Z1"},
			expected:          true,
		},
		"Fewer requisite entries": {
			requisiteTopology: map[string]string{"region": "R1"},
			availableTopology: map[string]string{"region": "R1", "zone": "Z1"},
			expected:          true,
		},
		"Fewer available entries": {
			requisiteTopology: map[string]string{"region": "R1", "zone": "Z1"},
			availableTopology: map[string]string{"region": "R1"},
			expected:          true,
		},
		"Non-overlapping entries": {
			requisiteTopology: map[string]string{"zone": "Z1"},
			availableTopology: map[string]string{"region": "R1"},
			expected:          true,
		},
		"Mismatch single entry": {
			requisiteTopology: map[string]string{"region": "R1"},
			availableTopology: map[string]string{"region": "R2"},
			expected:          false,
		},
		"Mismatch region, multiple entries": {
			requisiteTopology: map[string]string{"region": "R1", "zone": "Z1"},
			availableTopology: map[string]string{"region": "R2", "zone": "Z1"},
			expected:          false,
		},
		"Mismatch zone, multiple entries": {
			requisiteTopology: map[string]string{"region": "R1", "zone": "Z1"},
			availableTopology: map[string]string{"region": "R1", "zone": "Z2"},
			expected:          false,
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			result := isTopologySupportedByTopology(test.requisiteTopology, test.availableTopology)
			assert.Equal(t, test.expected, result, fmt.Sprintf("%s failed", testName))
		})
	}
}

func TestIsTopologySupportedByPool(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	supportedTopologies := make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	fakePool1 := mockstorage.NewMockPool(mockCtrl)
	fakePool1.EXPECT().Name().Return("fake pool 1").AnyTimes()
	fakePool1.EXPECT().SupportedTopologies().Return(supportedTopologies).AnyTimes()

	supported := isTopologySupportedByPool(fakePool1,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	if !supported {
		t.Error("pool should support topology")
	}

	supported = isTopologySupportedByPool(fakePool1,
		map[string]string{"topology.kubernetes.io/region": "R1"})
	if !supported {
		t.Error("pool should support topology")
	}

	supported = isTopologySupportedByPool(fakePool1,
		map[string]string{"topology.kubernetes.io/region": "Not supported"})
	if supported {
		t.Error("pool should not support topology")
	}

	supportedTopologies = make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	supportedTopologies = append(supportedTopologies,
		map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	fakePool2 := mockstorage.NewMockPool(mockCtrl)
	fakePool2.EXPECT().Name().Return("fake pool 2").AnyTimes()
	fakePool2.EXPECT().SupportedTopologies().Return(supportedTopologies).AnyTimes()

	supported = isTopologySupportedByPool(fakePool2,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	if !supported {
		t.Error("pool should support topology")
	}

	supportedTopologies = make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies, map[string]string{"topology.kubernetes.io/region": "R1"})
	fakePool3 := mockstorage.NewMockPool(mockCtrl)
	fakePool3.EXPECT().Name().Return("fake pool 3").AnyTimes()
	fakePool3.EXPECT().SupportedTopologies().Return(supportedTopologies).AnyTimes()

	supported = isTopologySupportedByPool(fakePool3,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	if !supported {
		t.Error("pool should support topology")
	}
}

func TestFilterPoolsOnTopology(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	fakePools := make([]storage.Pool, 0)
	supportedTopologies := make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	fakePool1 := mockstorage.NewMockPool(mockCtrl)
	fakePool1.EXPECT().Name().Return("fake pool 1").AnyTimes()
	fakePool1.EXPECT().SupportedTopologies().Return(supportedTopologies).AnyTimes()
	fakePools = append(fakePools, fakePool1)

	supportedTopologies = make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	supportedTopologies = append(supportedTopologies,
		map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	fakePool2 := mockstorage.NewMockPool(mockCtrl)
	fakePool2.EXPECT().Name().Return("fake pool 2").AnyTimes()
	fakePool2.EXPECT().SupportedTopologies().Return(supportedTopologies).AnyTimes()
	fakePools = append(fakePools, fakePool2)

	filteredPools := FilterPoolsOnTopology(context.Background(), make([]storage.Pool, 0), make([]map[string]string, 0))
	if len(filteredPools) != 0 {
		t.Error("matching pools should be empty")
	}

	filteredPools = FilterPoolsOnTopology(context.Background(), make([]storage.Pool, 0), nil)
	if len(filteredPools) != 0 {
		t.Error("matching pools should be empty")
	}

	requisiteTopologies := make([]map[string]string, 0)
	filteredPools = FilterPoolsOnTopology(context.Background(), fakePools, requisiteTopologies)
	if len(filteredPools) != 2 {
		t.Error("all pools should be returned when requisite topology is empty")
	}

	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies, map[string]string{"topology.kubernetes.io/region": "R1"})
	filteredPools = FilterPoolsOnTopology(context.Background(), fakePools, requisiteTopologies)
	if len(filteredPools) != 2 {
		t.Error("all pools should be returned")
	}

	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	filteredPools = FilterPoolsOnTopology(context.Background(), fakePools, requisiteTopologies)
	if len(filteredPools) != 2 {
		t.Error("all pools should be returned")
	}

	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies,
		map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	filteredPools = FilterPoolsOnTopology(context.Background(), fakePools, requisiteTopologies)
	if len(filteredPools) != 1 {
		t.Error("only fake pool 2 should be returned")
	}

	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	requisiteTopologies = append(requisiteTopologies,
		map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	filteredPools = FilterPoolsOnTopology(context.Background(), fakePools, requisiteTopologies)
	if len(filteredPools) != 2 {
		t.Error("all pools should be returned")
	}

	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies,
		map[string]string{"topology.kubernetes.io/region": "I do not exist", "topology.kubernetes.io/zone": "Z2"})
	filteredPools = FilterPoolsOnTopology(context.Background(), fakePools, requisiteTopologies)
	if len(filteredPools) != 0 {
		t.Error("no pools should match")
	}

	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	requisiteTopologies = append(requisiteTopologies,
		map[string]string{"topology.kubernetes.io/region": "R3", "topology.kubernetes.io/zone": "Z3"})
	filteredPools = FilterPoolsOnTopology(context.Background(), fakePools, requisiteTopologies)
	if len(filteredPools) != 2 {
		t.Error("all pools should be returned")
	}

	fakePoolNoRestriction := mockstorage.NewMockPool(mockCtrl)
	fakePoolNoRestriction.EXPECT().Name().Return("fake pool unrestricted").AnyTimes()
	fakePoolNoRestriction.EXPECT().SupportedTopologies().Return(nil).AnyTimes()
	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	requisiteTopologies = append(requisiteTopologies,
		map[string]string{"topology.kubernetes.io/region": "R3", "topology.kubernetes.io/zone": "Z3"})
	filteredPools = FilterPoolsOnTopology(context.Background(), []storage.Pool{fakePoolNoRestriction},
		requisiteTopologies)
	if len(filteredPools) == 0 {
		t.Error("all pools should be returned")
	}

	fakePoolNoRestriction.EXPECT().SupportedTopologies().Return(make([]map[string]string, 0)).AnyTimes()
	requisiteTopologies = make([]map[string]string, 0)
	requisiteTopologies = append(requisiteTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	requisiteTopologies = append(requisiteTopologies,
		map[string]string{"topology.kubernetes.io/region": "R3", "topology.kubernetes.io/zone": "Z3"})
	filteredPools = FilterPoolsOnTopology(context.Background(), []storage.Pool{fakePoolNoRestriction},
		requisiteTopologies)
	if len(filteredPools) == 0 {
		t.Error("all pools should be returned")
	}
}

func makeFakePool(mockCtrl *gomock.Controller, name string, topologies []map[string]string) storage.Pool {
	fakePool := mockstorage.NewMockPool(mockCtrl)
	fakePool.EXPECT().Name().Return(name).AnyTimes()
	fakePool.EXPECT().SupportedTopologies().Return(topologies).AnyTimes()
	return fakePool
}

func TestGetTopologyForVolume(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	topologyEmpty := map[string]string{}
	topologyR1 := map[string]string{"topology.kubernetes.io/region": "R1"}
	topologyR2 := map[string]string{"topology.kubernetes.io/region": "R2"}
	topologyR1Z1 := map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"}
	topologyR1Z2 := map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z2"}
	topologyR2Z1 := map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z1"}

	poolNil := makeFakePool(mockCtrl, "poolEmpty", nil)
	poolEmpty := makeFakePool(mockCtrl, "poolEmpty", []map[string]string{})
	poolR1 := makeFakePool(mockCtrl, "poolR1", []map[string]string{topologyR1})
	poolR1Z1 := makeFakePool(mockCtrl, "poolR1", []map[string]string{topologyR1Z1})
	poolR2Z1 := makeFakePool(mockCtrl, "poolR1", []map[string]string{topologyR2Z1})

	tests := map[string]struct {
		volConfig        *storage.VolumeConfig
		pool             storage.Pool
		expectedTopology map[string]string
		expectedError    bool
	}{
		"No entries": {
			volConfig:        &storage.VolumeConfig{},
			pool:             poolEmpty,
			expectedTopology: nil,
			expectedError:    false,
		},
		"Identical requisite, single entry, region only": {
			volConfig:        &storage.VolumeConfig{RequisiteTopologies: []map[string]string{topologyR1}},
			pool:             poolR1,
			expectedTopology: topologyR1,
			expectedError:    false,
		},
		"Identical requisite, single entry, region & zone": {
			volConfig:        &storage.VolumeConfig{RequisiteTopologies: []map[string]string{topologyR1Z1}},
			pool:             poolR1Z1,
			expectedTopology: topologyR1Z1,
			expectedError:    false,
		},
		"Empty requisite, single entry, region only": {
			volConfig:        &storage.VolumeConfig{RequisiteTopologies: []map[string]string{topologyEmpty}},
			pool:             poolR1,
			expectedTopology: topologyEmpty,
			expectedError:    false,
		},
		"Empty requisite, single entry, region & zone": {
			volConfig:        &storage.VolumeConfig{RequisiteTopologies: []map[string]string{topologyEmpty}},
			pool:             poolR1Z1,
			expectedTopology: topologyEmpty,
			expectedError:    false,
		},
		"Empty available, single entry, region only": {
			volConfig:        &storage.VolumeConfig{RequisiteTopologies: []map[string]string{topologyR1}},
			pool:             poolNil,
			expectedTopology: nil,
			expectedError:    true,
		},
		"Empty available, single entry, region & zone": {
			volConfig:        &storage.VolumeConfig{RequisiteTopologies: []map[string]string{topologyR1Z1}},
			pool:             poolNil,
			expectedTopology: nil,
			expectedError:    true,
		},
		"Multiple requisite, single preferred, region only": {
			volConfig: &storage.VolumeConfig{
				RequisiteTopologies: []map[string]string{topologyR2, topologyR1},
				PreferredTopologies: []map[string]string{topologyR1},
			},
			pool:             poolR1,
			expectedTopology: topologyR1,
			expectedError:    false,
		},
		"Multiple requisite, single preferred, region & zone": {
			volConfig: &storage.VolumeConfig{
				RequisiteTopologies: []map[string]string{topologyR2, topologyR1Z1},
				PreferredTopologies: []map[string]string{topologyR1Z1},
			},
			pool:             poolR1Z1,
			expectedTopology: topologyR1Z1,
			expectedError:    false,
		},
		"Multiple requisite, multiple preferred, region only": {
			volConfig: &storage.VolumeConfig{
				RequisiteTopologies: []map[string]string{topologyR2, topologyR1},
				PreferredTopologies: []map[string]string{topologyR1, topologyR2},
			},
			pool:             poolR1,
			expectedTopology: topologyR1,
			expectedError:    false,
		},
		"Multiple requisite, multiple preferred, region & zone": {
			volConfig: &storage.VolumeConfig{
				RequisiteTopologies: []map[string]string{topologyR2Z1, topologyR1Z1},
				PreferredTopologies: []map[string]string{topologyR1Z1, topologyR2Z1},
			},
			pool:             poolR1Z1,
			expectedTopology: topologyR1Z1,
			expectedError:    false,
		},
		"Multiple requisite, multiple preferred, less specific pool": {
			volConfig: &storage.VolumeConfig{
				RequisiteTopologies: []map[string]string{topologyR2, topologyR1},
				PreferredTopologies: []map[string]string{topologyR2Z1, topologyR1Z1},
			},
			pool:             poolR1,
			expectedTopology: topologyR1Z1,
			expectedError:    false,
		},
		"Multiple requisite, multiple preferred, more specific pool": {
			volConfig: &storage.VolumeConfig{
				RequisiteTopologies: []map[string]string{topologyR2, topologyR1},
				PreferredTopologies: []map[string]string{topologyR2, topologyR1},
			},
			pool:             poolR1Z1,
			expectedTopology: topologyR1,
			expectedError:    false,
		},
		"Multiple requisite, no preferred, region only": {
			volConfig: &storage.VolumeConfig{
				RequisiteTopologies: []map[string]string{topologyR2, topologyR1},
				PreferredTopologies: []map[string]string{},
			},
			pool:             poolR1,
			expectedTopology: topologyR1,
			expectedError:    false,
		},
		"Multiple requisite, no preferred, region & zone": {
			volConfig: &storage.VolumeConfig{
				RequisiteTopologies: []map[string]string{topologyR2Z1, topologyR1Z1},
				PreferredTopologies: []map[string]string{},
			},
			pool:             poolR1Z1,
			expectedTopology: topologyR1Z1,
			expectedError:    false,
		},
		"Multiple requisite, single preferred, region & zone, no match": {
			volConfig: &storage.VolumeConfig{
				RequisiteTopologies: []map[string]string{topologyR1Z1, topologyR1Z2},
				PreferredTopologies: []map[string]string{topologyR1Z1},
			},
			pool:             poolR2Z1,
			expectedTopology: nil,
			expectedError:    true,
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			resultTopology, resultError := GetTopologyForVolume(context.TODO(), test.volConfig, test.pool)
			assert.Equal(t, test.expectedTopology, resultTopology, fmt.Sprintf("%s topology mismatch", testName))
			assert.Equal(t, test.expectedError, resultError != nil, fmt.Sprintf("%s error mismatch", testName))
		})
	}
}

func TestGetRegionZoneForTopology(t *testing.T) {
	tests := map[string]struct {
		topology       map[string]string
		expectedRegion string
		expectedZone   string
	}{
		"Nil topology": {
			topology:       nil,
			expectedRegion: "",
			expectedZone:   "",
		},
		"Empty topology": {
			topology:       map[string]string{},
			expectedRegion: "",
			expectedZone:   "",
		},
		"Region only": {
			topology:       map[string]string{"topology.kubernetes.io/region": "R1"},
			expectedRegion: "R1",
			expectedZone:   "",
		},
		"Zone only": {
			topology:       map[string]string{"topology.kubernetes.io/zone": "Z1"},
			expectedRegion: "",
			expectedZone:   "Z1",
		},
		"Region & zone": {
			topology: map[string]string{
				"topology.kubernetes.io/region": "R1",
				"topology.kubernetes.io/zone":   "Z1",
			},
			expectedRegion: "R1",
			expectedZone:   "Z1",
		},
		"Unsupported region & zone": {
			topology: map[string]string{
				"other/region": "R1",
				"other/zone":   "Z1",
			},
			expectedRegion: "",
			expectedZone:   "",
		},
		"Region & unsupported zone": {
			topology: map[string]string{
				"topology.kubernetes.io/region": "R1",
				"other/zone":                    "Z1",
			},
			expectedRegion: "R1",
			expectedZone:   "",
		},
		"Zone & unsupported region": {
			topology: map[string]string{
				"other/region":                "R1",
				"topology.kubernetes.io/zone": "Z1",
			},
			expectedRegion: "",
			expectedZone:   "Z1",
		},
		"Region & zone, other unsupported keys": {
			topology: map[string]string{
				"other/region":                  "R2",
				"other/zone":                    "Z1",
				"topology.kubernetes.io/region": "R1",
				"topology.kubernetes.io/zone":   "Z1",
			},
			expectedRegion: "R1",
			expectedZone:   "Z1",
		},
	}
	for testName, test := range tests {
		t.Run(testName, func(t *testing.T) {
			resultRegion, resultZone := GetRegionZoneForTopology(test.topology)
			assert.Equal(t, test.expectedRegion, resultRegion, fmt.Sprintf("%s region match failed", testName))
			assert.Equal(t, test.expectedZone, resultZone, fmt.Sprintf("%s zone match failed", testName))
		})
	}
}

func TestFilterPoolsOnNasType(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	filteredPools1 := FilterPoolsOnNasType(context.Background(), make([]storage.Pool, 0),
		make(map[string]sa.Request, 0))
	if len(filteredPools1) != 0 {
		t.Error("filtered pools should be empty")
	}

	// Case when offer is "nfs" but request can be anything
	fakePools := make([]storage.Pool, 0)
	poolAttrMap1 := make(map[string]sa.Offer, 0)
	poolAttrMap1["nasType"] = sa.NewStringOffer(sa.NFS)
	fakePool1 := mockstorage.NewMockPool(mockCtrl)
	fakePool1.EXPECT().Name().Return("fake pool 1").AnyTimes()
	fakePool1.EXPECT().Attributes().Return(poolAttrMap1).AnyTimes()
	fakePools = append(fakePools, fakePool1)

	poolAttrMap2 := make(map[string]sa.Offer, 0)
	poolAttrMap2["nasType"] = sa.NewStringOffer(sa.SMB)
	fakePool2 := mockstorage.NewMockPool(mockCtrl)
	fakePool2.EXPECT().Name().Return("fake pool 2").AnyTimes()
	fakePool2.EXPECT().Attributes().Return(poolAttrMap2).AnyTimes()
	fakePools = append(fakePools, fakePool2)

	poolAttrMap3 := make(map[string]sa.Offer, 0)
	poolAttrMap3["nasType"] = nil
	fakePool3 := mockstorage.NewMockPool(mockCtrl)
	fakePool3.EXPECT().Name().Return("fake pool 3").AnyTimes()
	fakePool3.EXPECT().Attributes().Return(poolAttrMap3).AnyTimes()
	fakePools = append(fakePools, fakePool3)

	tests := []struct {
		Key     string
		Value   sa.Request
		NASType string
	}{
		// Cases for NFS behavior
		{"nasType", sa.NewStringRequest(""), "default"},
		{"nasType", sa.NewStringRequest("nfs"), "nfs"},
		{"nasType", sa.NewStringRequest("NFs"), "nfs"},
		{"nasType", sa.NewStringRequest("someValue"), ""},

		// Cases for SMB behavior
		{"nasType", sa.NewStringRequest("smb"), "smb"},
		{"nasType", sa.NewStringRequest("sMB"), "smb"},
	}
	for _, test := range tests {
		t.Run(test.Key, func(t *testing.T) {
			scAttrMap := make(map[string]sa.Request, 0)
			scAttrMap[test.Key] = test.Value
			filteredPools2 := FilterPoolsOnNasType(context.Background(), fakePools, scAttrMap)
			if test.NASType == "nfs" { // Expecting 1 NFS pool here
				assert.Equal(t, 1, len(filteredPools2))
			} else if test.NASType == "smb" { // Expecting 1 SMB pool here
				assert.Equal(t, 1, len(filteredPools2))
			} else if test.NASType == "default" { // Expecting no filtering
				assert.Equal(t, 3, len(filteredPools2))
			} else { // Expecting 0 filtered pools as input is invalid
				assert.Equal(t, 0, len(filteredPools2))
			}
		})
	}

	// Case when request is for a NASType but no corresponding NASType pool is available,
	// hence 0 pools are returned
	poolAttrMap4 := make(map[string]sa.Offer, 0)
	poolAttrMap4["nasType"] = nil
	fakePool4 := mockstorage.NewMockPool(mockCtrl)
	fakePool4.EXPECT().Name().Return("fake pool 3").AnyTimes()
	fakePool4.EXPECT().Attributes().Return(poolAttrMap4).AnyTimes()

	fakePools2 := make([]storage.Pool, 0)
	fakePools2 = append(fakePools2, fakePool3, fakePool4)

	tests2 := []struct {
		Key   string
		Value sa.Request
	}{
		{"nasType", sa.NewStringRequest("nfs")},
		{"nasType", sa.NewStringRequest("smb")},
	}
	for _, test := range tests2 {
		t.Run(test.Key, func(t *testing.T) {
			scAttrMap := make(map[string]sa.Request, 0)
			scAttrMap[test.Key] = test.Value
			filteredPools := FilterPoolsOnNasType(context.Background(), fakePools2, scAttrMap)
			assert.Equal(t, 0, len(filteredPools))
		})
	}
}

func TestSortPoolsByPreferredTopologies(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	fakePools := make([]storage.Pool, 0)
	supportedTopologies := make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	fakePool1 := mockstorage.NewMockPool(mockCtrl)
	fakePool1.EXPECT().Name().Return("fake pool 1").AnyTimes()
	fakePool1.EXPECT().SupportedTopologies().Return(supportedTopologies).AnyTimes()
	fakePools = append(fakePools, fakePool1)

	supportedTopologies = make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	supportedTopologies = append(supportedTopologies,
		map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	fakePool2 := mockstorage.NewMockPool(mockCtrl)
	fakePool2.EXPECT().Name().Return("fake pool 2").AnyTimes()
	fakePool2.EXPECT().SupportedTopologies().Return(supportedTopologies).AnyTimes()
	fakePools = append(fakePools, fakePool2)

	orderedPools := SortPoolsByPreferredTopologies(context.Background(), make([]storage.Pool, 0),
		make([]map[string]string, 0))
	if len(orderedPools) != 0 {
		t.Error("matching pools should be empty")
	}

	preferredTopologies := make([]map[string]string, 0)
	orderedPools = SortPoolsByPreferredTopologies(context.Background(), fakePools, preferredTopologies)
	if len(orderedPools) != 2 {
		t.Errorf("2 pools should be returned, got %d", len(orderedPools))
	}

	preferredTopologies = make([]map[string]string, 0)
	preferredTopologies = append(preferredTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	orderedPools = SortPoolsByPreferredTopologies(context.Background(), fakePools, preferredTopologies)
	if len(orderedPools) != 2 {
		t.Errorf("2 pools should be returned, got %d", len(orderedPools))
	}

	preferredTopologies = make([]map[string]string, 0)
	preferredTopologies = append(preferredTopologies,
		map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	orderedPools = SortPoolsByPreferredTopologies(context.Background(), fakePools, preferredTopologies)
	if len(orderedPools) != 2 {
		t.Errorf("2 pools should be returned, got %d", len(orderedPools))
	}
	if orderedPools[0].Name() != fakePool2.Name() {
		t.Errorf("fake pool 2 should be first in the list")
	}

	supportedTopologies = make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies,
		map[string]string{"topology.kubernetes.io/region": "R3", "topology.kubernetes.io/zone": "Z3"})
	fakePool3 := mockstorage.NewMockPool(mockCtrl)
	fakePool3.EXPECT().Name().Return("fake pool 3").AnyTimes()
	fakePool3.EXPECT().SupportedTopologies().Return(supportedTopologies).AnyTimes()
	fakePools = append(fakePools, fakePool3)

	preferredTopologies = make([]map[string]string, 0)
	preferredTopologies = append(preferredTopologies,
		map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	orderedPools = SortPoolsByPreferredTopologies(context.Background(), fakePools, preferredTopologies)
	if len(orderedPools) != 3 {
		t.Errorf("2 pools should be returned, got %d", len(orderedPools))
	}
	if orderedPools[0].Name() != fakePool2.Name() {
		t.Errorf("fake pool 2 should be first in the list")
	}

	preferredTopologies = make([]map[string]string, 0)
	preferredTopologies = append(preferredTopologies,
		map[string]string{"topology.kubernetes.io/region": "R2", "topology.kubernetes.io/zone": "Z2"})
	preferredTopologies = append(preferredTopologies,
		map[string]string{"topology.kubernetes.io/region": "R3", "topology.kubernetes.io/zone": "Z3"})
	orderedPools = SortPoolsByPreferredTopologies(context.Background(), fakePools, preferredTopologies)
	if len(orderedPools) != 3 {
		t.Errorf("3 pools should be returned, got %d", len(orderedPools))
	}
	if orderedPools[0].Name() != fakePool2.Name() {
		t.Errorf("fake pool 2 should be first in the list")
	}
	if orderedPools[1].Name() != fakePool3.Name() {
		t.Errorf("fake pool 3 should be second in the list")
	}
	if orderedPools[2].Name() != fakePool1.Name() {
		t.Errorf("fake pool 1 should be third in the list")
	}

	preferredTopologies = make([]map[string]string, 0)
	preferredTopologies = append(
		preferredTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"},
	)
	orderedPools = SortPoolsByPreferredTopologies(context.Background(), fakePools, preferredTopologies)
	if len(orderedPools) != 3 {
		t.Errorf("3 pools should be returned, got %d", len(orderedPools))
	}
	if orderedPools[2].Name() != fakePool3.Name() {
		t.Errorf(
			"fake pool 3 should be third in the list, got %s, %s, %s",
			orderedPools[0].Name(), orderedPools[1].Name(), orderedPools[2].Name(),
		)
	}

	preferredTopologies = make([]map[string]string, 0)
	preferredTopologies = append(
		preferredTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1"},
	)
	orderedPools = SortPoolsByPreferredTopologies(context.Background(), fakePools, preferredTopologies)
	if len(orderedPools) != 3 {
		t.Errorf("3 pools should be returned, got %d", len(orderedPools))
	}
	if orderedPools[2].Name() != fakePool3.Name() {
		t.Errorf(
			"fake pool 3 should be third in the list, got %s, %s, %s",
			orderedPools[0].Name(), orderedPools[1].Name(), orderedPools[2].Name(),
		)
	}

	preferredTopologies = make([]map[string]string, 0)
	orderedPools = SortPoolsByPreferredTopologies(context.Background(), fakePools, preferredTopologies)
	if len(orderedPools) != 3 {
		t.Errorf("3 pools should be returned, got %d", len(orderedPools))
	}
}

func TestCheckAndAddBackend(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)

	type addBackendTest struct {
		backendName string
		state       storage.BackendState
		result      int
		excludePool bool
	}

	tests := map[string]addBackendTest{
		"StorageOnline": {
			backendName: "backend", state: storage.Online, result: 3, excludePool: false,
		},
		"StorageOnlineAndExcludePool": {
			backendName: "backend", state: storage.Online, result: 0, excludePool: true,
		},
		"StorageOffline": {
			backendName: "backend", state: storage.Offline, result: 0, excludePool: false,
		},
	}

	for testName, test := range tests {
		t.Run(fmt.Sprintf("%s:", testName), func(t *testing.T) {
			_, excludePool, fakePools := createAndGetPool(mockCtrl, 3)

			excludePools := make(map[string][]string, 0)
			additionalPools := make(map[string][]string, 0)

			backend := storage.NewTestStorageBackend()
			backend.SetName(test.backendName)
			backend.SetState(test.state)
			backend.ClearStoragePools()
			for _, pool := range fakePools {
				backend.AddStoragePool(pool)
			}

			if test.excludePool {
				excludePools = excludePool
			} else {
				additionalPools = excludePool
			}

			sc := New(&Config{
				Name: "sc",
				Attributes: map[string]sa.Request{
					sa.BackendType: sa.NewStringRequest("ontap-nas"),
				},
				AdditionalPools: additionalPools,
				ExcludePools:    excludePools,
			})

			result := sc.CheckAndAddBackend(ctx, backend)
			assert.Equal(t, test.result, result, "Storage pool addition failed")
		})
	}
}

func TestIsAddedToBackend(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	pools, _, fakePools := createAndGetPool(mockCtrl, 3)

	type isAddedPoolTest struct {
		storageClassName string
		backendName      string
		fakePools        map[string]storage.Pool
		result           bool
	}

	tests := map[string]isAddedPoolTest{
		"IsAddedPoolSuccess": {storageClassName: "sc", fakePools: fakePools, backendName: "backend", result: true},
		"IsAddedPoolFailed":  {storageClassName: "sc1", fakePools: fakePools, backendName: "backend", result: false},
	}

	for testName, test := range tests {
		t.Run(fmt.Sprintf("%s:", testName), func(t *testing.T) {
			backend := storage.NewTestStorageBackend()
			backend.SetName(test.backendName)
			backend.ClearStoragePools()
			for _, pool := range test.fakePools {
				backend.AddStoragePool(pool)
			}

			sc := New(&Config{
				Name: "sc",
				Attributes: map[string]sa.Request{
					sa.BackendType: sa.NewStringRequest("ontap-nas"),
				},
				Pools: pools,
			})
			result := sc.IsAddedToBackend(backend, test.storageClassName)
			assert.Equal(t, test.result, result, "Storage pool is not added to backend")
		})
	}
}

func TestRemovePoolsForBackend(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	fakePool1 := mockstorage.NewMockPool(mockCtrl)
	fakePool2 := mockstorage.NewMockPool(mockCtrl)
	backend1 := storage.NewTestStorageBackend()
	backend1.SetName("b1")
	backend2 := storage.NewTestStorageBackend()
	backend2.SetName("b2")

	type removePoolTest struct {
		storagePool *mockstorage.MockPool
		backend     storage.Backend
		result      int
	}

	tests := map[string]removePoolTest{
		"RemovePoolSuccess": {storagePool: fakePool1, backend: backend1, result: 1},
		"RemovePoolFailed":  {storagePool: fakePool2, backend: backend2, result: 0},
	}

	for testName, test := range tests {
		t.Run(fmt.Sprintf("%s:", testName), func(t *testing.T) {
			test.storagePool.EXPECT().Backend().Return(test.backend).Times(1)
			sc := New(&Config{
				Name: "sc",
				Attributes: map[string]sa.Request{
					sa.BackendType: sa.NewStringRequest("ontap-nas"),
				},
			})
			sc.pools = []storage.Pool{test.storagePool}
			sc.RemovePoolsForBackend(backend2)
			assert.Equal(t, test.result, len(sc.pools), "Storage pool is not removed")
		})
	}
}

func TestGetStoragePools(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	pools, _, fakePools := createAndGetPool(mockCtrl, 3)
	backend := storage.NewTestStorageBackend()
	backend.SetName("backend1")
	backend.ClearStoragePools()
	for _, pool := range fakePools {
		backend.AddStoragePool(pool)
	}

	sc := New(&Config{
		Name: "sc",
		Attributes: map[string]sa.Request{
			sa.BackendType: sa.NewStringRequest("ontap-nas"),
		},
		Pools: pools,
	})
	result := sc.GetStoragePools()
	assert.Equal(t, sc.config.Pools, result, "Failed to get storage pools")
}

func TestGetAdditionalStoragePools(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	pools, additionalPools, fakePools := createAndGetPool(mockCtrl, 3)
	backend := storage.NewTestStorageBackend()
	backend.SetName("backend1")
	backend.ClearStoragePools()
	for _, pool := range fakePools {
		backend.AddStoragePool(pool)
	}

	sc := New(&Config{
		Name: "sc",
		Attributes: map[string]sa.Request{
			sa.BackendType: sa.NewStringRequest("ontap-nas"),
		},
		Pools:           pools,
		AdditionalPools: additionalPools,
	})

	result := sc.GetAdditionalStoragePools()
	assert.Equal(t, sc.config.AdditionalPools, result, "Failed to get additional storage pools")
}

func TestRegexMatcherImplNegative(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)
	fakePool1 := mockstorage.NewMockPool(mockCtrl)
	backend1 := mockstorage.NewMockBackend(mockCtrl)
	fakePool1.EXPECT().Backend().Return(backend1).AnyTimes()
	backend1.EXPECT().Name().Return("back").AnyTimes()
	fakePool1.EXPECT().Name().Return("a(").AnyTimes()

	type regexMatcherTest struct {
		storagePool            *mockstorage.MockPool
		storagePoolBackendName string
		storagePoolList        []string
		result                 bool
	}

	storagePoolList := []string{"fakepool", "a("}
	tests := map[string]regexMatcherTest{
		"storagePoolNil":            {storagePool: nil, result: false},
		"storagePoolBackendNameNil": {storagePool: fakePool1, storagePoolBackendName: "", result: false},
		"storagePoolListNil": {
			storagePool: fakePool1, storagePoolBackendName: "backend", storagePoolList: nil, result: false,
		},
		"InvalidBackendName": {
			storagePool: fakePool1, storagePoolBackendName: "a(", storagePoolList: storagePoolList, result: false,
		},
		"InvalidStoragePoolname": {
			storagePool: fakePool1, storagePoolBackendName: "back", storagePoolList: storagePoolList, result: false,
		},
	}

	sc := New(&Config{
		Name: "sc",
		Attributes: map[string]sa.Request{
			sa.BackendType: sa.NewStringRequest("ontap-nas"),
		},
	})

	for testName, test := range tests {
		t.Run(fmt.Sprintf("%s:", testName), func(t *testing.T) {
			var result bool = false
			if test.storagePool == nil {
				result = sc.regexMatcherImpl(ctx, nil, test.storagePoolBackendName, test.storagePoolList)
			} else {
				result = sc.regexMatcherImpl(ctx, test.storagePool, test.storagePoolBackendName, test.storagePoolList)
			}
			assert.Equal(t, test.result, result, "storage pool are matched")
		})
	}
}

func getFakeStoragePool(
	mockCtrl *gomock.Controller, poolName, backendName, backendType, storageClassName string,
	backend *mockstorage.MockBackend,
) (*mockstorage.MockPool, *mockstorage.MockBackend) {
	fakePool1 := mockstorage.NewMockPool(mockCtrl)
	fakePool1.EXPECT().Name().Return(poolName).AnyTimes()
	backend1 := mockstorage.NewMockBackend(mockCtrl)

	if backend == nil {
		fakePool1.EXPECT().Backend().Return(backend1).AnyTimes()
		backend1.EXPECT().Name().Return(backendName).AnyTimes()
	} else {
		fakePool1.EXPECT().Backend().Return(backend).AnyTimes()
	}

	attribute := make(map[string]sa.Offer, 0)
	attribute["name"] = sa.NewStringOffer(poolName)
	attribute["backendType"] = sa.NewStringOffer(backendType)

	fakePool1.EXPECT().Attributes().Return(attribute).AnyTimes()
	fakePool1.EXPECT().AddStorageClass(storageClassName).AnyTimes()
	storageClassList := []string{storageClassName}
	fakePool1.EXPECT().StorageClasses().Return(storageClassList).AnyTimes()
	return fakePool1, backend1
}

func createAndGetPool(
	mockCtrl *gomock.Controller, count int,
) (map[string][]string, map[string][]string, map[string]storage.Pool) {
	fakePool := make(map[string]storage.Pool)
	pools := make(map[string][]string, 0)
	addOrExcludePool := make(map[string][]string, 0)
	for i := 1; i <= count; i++ {
		fakePool["fakepool"+strconv.Itoa(i)], _ = getFakeStoragePool(mockCtrl, "fakepool"+strconv.Itoa(i),
			"backend"+strconv.Itoa(i), "ontap-nas", "sc", nil)

		addOrExcludePool["backend"+strconv.Itoa(i)] = []string{"fakepool" + strconv.Itoa(i)}

		pools["backend"+strconv.Itoa(i)] = []string{"fakepool" + strconv.Itoa(i)}
	}
	return pools, addOrExcludePool, fakePool
}

func TestGetStoragePoolsForProtocolNegative(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)
	fakePool1 := mockstorage.NewMockPool(mockCtrl)
	backend1 := mockstorage.NewMockBackend(mockCtrl)
	fakePool1.EXPECT().Backend().Return(backend1).Times(1)
	backend1.EXPECT().GetProtocol(ctx).Return(config.Block).Times(1)

	sc := New(&Config{
		Name: "sc",
		Attributes: map[string]sa.Request{
			sa.BackendType: sa.NewStringRequest("ontap-nas"),
		},
	})
	sc.pools = []storage.Pool{fakePool1}
	storagePool := sc.GetStoragePoolsForProtocol(ctx, config.File, config.ReadWriteMany)

	assert.Empty(t, storagePool, "unable to get storage pool")
}

func TestStorageClass_SmartCopy(t *testing.T) {
	bronzeStorageConfig := &Config{
		Name:            "bronze",
		Attributes:      make(map[string]sa.Request),
		AdditionalPools: make(map[string][]string),
	}
	bronzeStorageConfig.Attributes["media"] = sa.NewStringRequest("hdd")
	bronzeStorageConfig.AdditionalPools["ontapnas"] = []string{"aggr1"}
	bronzeStorageConfig.AdditionalPools["ontapsan"] = []string{"aggr1"}

	storageClass := New(bronzeStorageConfig)

	// Create a deep copy of the snapshot
	copiedStorageClass := storageClass.SmartCopy().(*StorageClass)

	// Check that the copied volume is deeply equal to the original
	assert.Equal(t, storageClass, copiedStorageClass)

	// Check that the copied volume does not point to the same memory
	assert.False(t, storageClass == copiedStorageClass)
	assert.False(t, storageClass.config == copiedStorageClass.config)
}
