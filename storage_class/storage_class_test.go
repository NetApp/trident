// Copyright 2022 NetApp, Inc. All Rights Reserved.

package storageclass

import (
	"context"
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"github.com/netapp/trident/config"
	mockstorage "github.com/netapp/trident/mocks/mock_storage"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(ioutil.Discard)
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
	backend2.EXPECT().GetProtocol(ctx).Return(config.BlockOnFile).AnyTimes()

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
	fakePool1 := mockstorage.NewMockPool(mockCtrl)
	fakePool1.EXPECT().Name().Return("fakepool1").AnyTimes()
	backend1 := mockstorage.NewMockBackend(mockCtrl)
	fakePool1.EXPECT().Backend().Return(backend1).AnyTimes()
	backend1.EXPECT().Name().Return("backend1").AnyTimes()
	attr1 := make(map[string]sa.Offer, 0)
	attr1["name"] = sa.NewStringOffer("fakepool1")
	attr1["backendType"] = sa.NewStringOffer("ontap-nas")
	fakePool1.EXPECT().Attributes().Return(attr1).AnyTimes()

	fakePool2 := mockstorage.NewMockPool(mockCtrl)
	fakePool2.EXPECT().Name().Return("fakepool2").AnyTimes()
	fakePool2.EXPECT().Backend().Return(backend1).AnyTimes()
	offer2 := sa.NewStringOffer("fakepool2")
	attr2 := make(map[string]sa.Offer, 0)
	attr2["name"] = offer2
	attr2["backendType"] = sa.NewStringOffer("ontap-nas")
	fakePool2.EXPECT().Attributes().Return(attr2).AnyTimes()

	fakePool3 := mockstorage.NewMockPool(mockCtrl)
	fakePool3.EXPECT().Name().Return("fakepool3").AnyTimes()
	backend2 := mockstorage.NewMockBackend(mockCtrl)
	fakePool3.EXPECT().Backend().Return(backend2).AnyTimes()
	backend2.EXPECT().Name().Return("backend2").AnyTimes()
	offer3 := sa.NewStringOffer("fakepool3")
	attr3 := make(map[string]sa.Offer, 0)
	attr3["name"] = offer3
	attr3["backendType"] = sa.NewStringOffer("ontap-nas")
	fakePool3.EXPECT().Attributes().Return(attr3).AnyTimes()

	fakePool4 := mockstorage.NewMockPool(mockCtrl)
	fakePool4.EXPECT().Name().Return("fakepool4").AnyTimes()
	backend3 := mockstorage.NewMockBackend(mockCtrl)
	fakePool4.EXPECT().Backend().Return(backend3).AnyTimes()
	backend3.EXPECT().Name().Return("backend3").AnyTimes()
	offer4 := sa.NewStringOffer("fakepool4")
	attr4 := make(map[string]sa.Offer, 0)
	attr4["name"] = offer4
	attr4["backendType"] = sa.NewStringOffer("ontap-nas")
	fakePool4.EXPECT().Attributes().Return(attr4).AnyTimes()

	pools["backend1"] = []string{"fakepool1", "fakepool2"}
	pools["backend2"] = []string{"fakepool3"}

	excludePools := make(map[string][]string, 0)
	excludePools["backend2"] = []string{"fakepool3"}

	additionalPools := make(map[string][]string, 0)
	additionalPools["backend3"] = []string{"fakepool4"}

	sc := New(&Config{
		Name: "sc1",
		Attributes: map[string]sa.Request{
			sa.BackendType: sa.NewStringRequest("ontap-nas"),
		},
		Pools:           pools,
		AdditionalPools: additionalPools,
		ExcludePools:    excludePools,
	})

	assert.True(t, sc.Matches(ctx, fakePool1))
	assert.True(t, sc.Matches(ctx, fakePool2))
	assert.False(t, sc.Matches(ctx, fakePool3))
	assert.True(t, sc.Matches(ctx, fakePool4))
}

func TestConstructExternal(t *testing.T) {
	ctx := context.TODO()
	mockCtrl := gomock.NewController(t)

	fakePool1 := mockstorage.NewMockPool(mockCtrl)
	fakePool1.EXPECT().Name().Return("fake pool 1").AnyTimes()
	backend1 := mockstorage.NewMockBackend(mockCtrl)
	fakePool1.EXPECT().Backend().Return(backend1).AnyTimes()
	backend1.EXPECT().Name().Return("backend1").AnyTimes()

	fakePool2 := mockstorage.NewMockPool(mockCtrl)
	fakePool2.EXPECT().Name().Return("fake pool 2").AnyTimes()
	backend2 := mockstorage.NewMockBackend(mockCtrl)
	fakePool2.EXPECT().Backend().Return(backend2).AnyTimes()
	backend2.EXPECT().Name().Return("").AnyTimes()

	pools := make(map[string][]string, 0)
	pools["backend1"] = []string{"fakepool1"}

	additionalPools := make(map[string][]string, 0)
	additionalPools["backend2"] = []string{"fakepool2"}

	sc := New(&Config{
		Name: "sc1",
		Attributes: map[string]sa.Request{
			sa.Snapshots:        sa.NewBoolRequest(true),
			sa.ProvisioningType: sa.NewStringRequest("thick"),
		},
		Pools:           pools,
		AdditionalPools: additionalPools,
	})
	sc.pools = append(sc.pools, fakePool1, fakePool2)
	ext := sc.ConstructExternal(ctx)

	assert.NotNil(t, ext)
	assert.Equal(t, "sc1", ext.GetName())
}

func TestDoesPoolSupportTopology(t *testing.T) {
	mockCtrl := gomock.NewController(t)

	supportedTopologies := make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	fakePool1 := mockstorage.NewMockPool(mockCtrl)
	fakePool1.EXPECT().Name().Return("fake pool 1").AnyTimes()
	fakePool1.EXPECT().SupportedTopologies().Return(supportedTopologies).AnyTimes()

	supported := isTopologySupportedByPool(context.TODO(), fakePool1,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	if !supported {
		t.Error("pool should support topology")
	}

	supported = isTopologySupportedByPool(context.TODO(), fakePool1,
		map[string]string{"topology.kubernetes.io/region": "R1"})
	if !supported {
		t.Error("pool should support topology")
	}

	supported = isTopologySupportedByPool(context.TODO(), fakePool1,
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

	supported = isTopologySupportedByPool(context.Background(), fakePool2,
		map[string]string{"topology.kubernetes.io/region": "R1", "topology.kubernetes.io/zone": "Z1"})
	if !supported {
		t.Error("pool should support topology")
	}

	supportedTopologies = make([]map[string]string, 0)
	supportedTopologies = append(supportedTopologies, map[string]string{"topology.kubernetes.io/region": "R1"})
	fakePool3 := mockstorage.NewMockPool(mockCtrl)
	fakePool3.EXPECT().Name().Return("fake pool 3").AnyTimes()
	fakePool3.EXPECT().SupportedTopologies().Return(supportedTopologies).AnyTimes()

	supported = isTopologySupportedByPool(context.Background(), fakePool3,
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
