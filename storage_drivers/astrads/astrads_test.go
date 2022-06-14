// Copyright 2022 NetApp, Inc. All Rights Reserved.

package astrads

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/netapp/trident/utils"

	"github.com/RoaringBitmap/roaring"
	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tridentconfig "github.com/netapp/trident/config"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_astrads"
	"github.com/netapp/trident/storage"
	storagefake "github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/astrads/api"
	"github.com/netapp/trident/storage_drivers/fake"
)

const (
	BackendName      = "myADSBackend"
	Kubeconfig       = "fake-kubeconfig"
	KubeconfigB64    = "ZmFrZS1rdWJlY29uZmln"
	Cluster          = "fake-cluster"
	ClusterUUID      = "deadbeef-d086-46cf-b08f-945945386d2a"
	Namespace        = "fake-namespace"
	KubeSystemUUID   = "deadbeef-b141-4b37-92ca-f9ac975069b5"
	FakeExportPolicy = "fake-export-policy"
	BackendUUID      = "deadbeef-03af-4394-ace4-e177cdbcaf28"
	VolumeUUID       = "deadbeef-9485-45c3-80fd-1b1d5ad1260b"
	VolumeSizeI64    = int64(107374182400)
	VolumeSizeStr    = "107374182400"
	ExportAddress    = "10.10.10.10"
)

var (
	ctx             = context.TODO()
	errFailed       = errors.New("failed")
	debugTraceFlags = map[string]bool{"method": true, "api": true}
	prefix          = "test-"
	now             = metav1.Now()

	commonConfig = &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "astrads-nas",
		BackendName:       BackendName,
		StoragePrefix:     &prefix,
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	adsCluster = &api.Cluster{
		Name:      Cluster,
		Namespace: Namespace,
		UUID:      ClusterUUID,
		Status:    "created",
		Version:   "2021.10.0",
	}

	qosPolicies = []*api.QosPolicy{
		{
			Name:      "bronze",
			Cluster:   Cluster,
			MinIOPS:   1000,
			MaxIOPS:   2000,
			BurstIOPS: 3000,
		},
		{
			Name:      "silver",
			Cluster:   Cluster,
			MinIOPS:   2000,
			MaxIOPS:   4000,
			BurstIOPS: 6000,
		},
		{
			Name:      "gold",
			Cluster:   Cluster,
			MinIOPS:   4000,
			MaxIOPS:   8000,
			BurstIOPS: 12000,
		},
	}

	staticExportPolicy = &api.ExportPolicy{
		Name:      FakeExportPolicy,
		Namespace: Namespace,
		Rules:     []api.ExportPolicyRule{},
	}
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}

func newTestAstraDSDriver(mockAPI api.AstraDS) *StorageDriver {
	mutableCommonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "astrads-nas",
		BackendName:       BackendName,
		StoragePrefix:     &prefix,
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	config := drivers.AstraDSStorageDriverConfig{
		CommonStorageDriverConfig: mutableCommonConfig,
		Kubeconfig:                KubeconfigB64,
		Cluster:                   Cluster,
		Namespace:                 Namespace,
		NfsMountOptions:           "vers=4.1",
		AutoExportPolicy:          false,
		AstraDSStorageDriverPool: drivers.AstraDSStorageDriverPool{
			AstraDSStorageDriverConfigDefaults: drivers.AstraDSStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: drivers.DefaultVolumeSize,
				},
				QosPolicy:    "silver",
				ExportPolicy: FakeExportPolicy,
			},
		},
	}

	mutableCluster := &api.Cluster{
		Name:      Cluster,
		Namespace: Namespace,
		UUID:      ClusterUUID,
		Status:    "created",
		Version:   "2021.10.0",
	}

	return &StorageDriver{
		initialized:         true,
		Config:              config,
		API:                 mockAPI,
		cluster:             mutableCluster,
		kubeSystemUUID:      KubeSystemUUID,
		volumeCreateTimeout: 30 * time.Second,
	}
}

func newMockAstraDSDriver(t *testing.T) (*mockapi.MockAstraDS, *StorageDriver) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAstraDS(mockCtrl)

	return mockAPI, newTestAstraDSDriver(mockAPI)
}

func TestName(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	result := driver.Name()

	assert.Equal(t, drivers.AstraDSStorageDriverName, result, "driver name mismatch")
}

func TestBackendName_SetInConfig(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)
	driver.Config.BackendName = BackendName

	result := driver.BackendName()

	assert.Equal(t, BackendName, result, "backend name mismatch")
}

func TestBackendName_UseDefault(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)
	driver.Config.BackendName = ""

	result := driver.BackendName()

	assert.Equal(t, Cluster, result, "backend name mismatch")
}

func TestPoolName(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)
	driver.Config.BackendName = "myADSBackend"

	result := driver.poolName("pool-1-A")

	assert.Equal(t, "myADSBackend_pool1A", result, "pool name mismatch")
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		Name  string
		Valid bool
	}{
		// Invalid names
		{"", false},
		{"-volume", false},
		{"volume-", false},
		{".volume", false},
		{"volume.", false},
		{"volume_1", false},
		{"Volume1", false},
		// Valid names
		{"v", true},
		{"volume1", true},
		{"x234567890123456789012345678901234567890123456789012345678901234", true},
		{"1volume", true},
		{"pvc-deadbeef-1234-abcd-5678-0123456789ab", true},
		{"volume.1", true},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			_, driver := newMockAstraDSDriver(t)

			err := driver.validateName(test.Name)

			if test.Valid {
				assert.NoError(t, err, "should be valid")
			} else {
				assert.Error(t, err, "should be invalid")
			}
		})
	}
}

func TestDefaultCreateTimeout(t *testing.T) {
	tests := []struct {
		Context  tridentconfig.DriverContext
		Expected time.Duration
	}{
		{tridentconfig.ContextDocker, tridentconfig.DockerCreateTimeout},
		{tridentconfig.ContextCSI, volumeCreateTimeout},
		{"", volumeCreateTimeout},
	}
	for _, test := range tests {
		t.Run(string(test.Context), func(t *testing.T) {
			_, driver := newMockAstraDSDriver(t)
			driver.Config.DriverContext = test.Context

			result := driver.defaultCreateTimeout()

			assert.Equal(t, test.Expected, result, "mismatched durations")
		})
	}
}

func TestDefaultDeleteTimeout(t *testing.T) {
	tests := []struct {
		Context  tridentconfig.DriverContext
		Expected time.Duration
	}{
		{tridentconfig.ContextDocker, tridentconfig.DockerDefaultTimeout},
		{tridentconfig.ContextCSI, defaultDeleteTimeout},
		{"", defaultDeleteTimeout},
	}
	for _, test := range tests {
		t.Run(string(test.Context), func(t *testing.T) {
			_, driver := newMockAstraDSDriver(t)
			driver.Config.DriverContext = test.Context

			result := driver.defaultDeleteTimeout()

			assert.Equal(t, test.Expected, result, "mismatched durations")
		})
	}
}

func TestInitialize_WithoutSecrets(t *testing.T) {
	configJSON := `
    {
        "version": 1,
        "storageDriverName": "astrads-nas",
        "debugTraceFlags": {"method": true, "api": true},
        "kubeconfig": "ZmFrZS1rdWJlY29uZmln",
        "cluster": "fake-cluster",
        "namespace": "fake-namespace",
        "nfsMountOptions": "vers=4.1",
        "autoExportPolicy": false,
        "defaults": {
            "exportPolicy": "fake-export-policy",
            "size": "1G"
        }
    }`

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAstraDS(mockCtrl)

	driver := &StorageDriver{
		API: mockAPI,
	}

	mockAPI.EXPECT().Init(ctx, Namespace, Cluster, []byte(Kubeconfig)).Return(adsCluster, KubeSystemUUID, nil).Times(1)
	mockAPI.EXPECT().QosPolicies(ctx).Return(qosPolicies, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, staticExportPolicy, nil).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{}, BackendUUID)

	assert.NoError(t, result, "initialize failed")
	assert.NotNil(t, driver.Config, "config is nil")
	assert.Equal(t, adsCluster, driver.cluster)
	assert.Equal(t, KubeSystemUUID, driver.kubeSystemUUID)
	assert.Equal(t, ClusterUUID, driver.Config.ClusterUUID)
	assert.Equal(t, KubeSystemUUID, driver.Config.KubeSystemUUID)
	assert.Equal(t, "test-", *driver.Config.StoragePrefix, "wrong storage prefix")
	assert.Equal(t, 1, len(driver.pools), "wrong number of pools")
	assert.Equal(t, BackendUUID, driver.backendUUID, "wrong backend UUID")
	assert.Equal(t, driver.volumeCreateTimeout, 10*time.Second, "volume create timeout mismatch")
	assert.True(t, driver.Initialized(), "not initialized")
}

func TestInitialize_WithSecrets(t *testing.T) {
	configJSON := `
    {
        "version": 1,
        "storageDriverName": "astrads-nas",
        "debugTraceFlags": {"method": true, "api": true},
        "cluster": "fake-cluster",
        "namespace": "fake-namespace",
        "nfsMountOptions": "vers=4.1",
        "autoExportPolicy": false,
        "defaults": {
            "exportPolicy": "fake-export-policy",
            "size": "1G"
        }
    }`

	secrets := map[string]string{
		"kubeconfig": KubeconfigB64,
	}

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAstraDS(mockCtrl)

	driver := &StorageDriver{
		API: mockAPI,
	}

	mockAPI.EXPECT().Init(ctx, Namespace, Cluster, []byte(Kubeconfig)).Return(adsCluster, KubeSystemUUID, nil).Times(1)
	mockAPI.EXPECT().QosPolicies(ctx).Return(qosPolicies, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, staticExportPolicy, nil).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, secrets, BackendUUID)

	assert.NoError(t, result, "initialize failed")
	assert.NotNil(t, driver.Config, "config is nil")
	assert.Equal(t, adsCluster, driver.cluster)
	assert.Equal(t, KubeconfigB64, driver.Config.Kubeconfig)
	assert.Equal(t, KubeSystemUUID, driver.kubeSystemUUID)
	assert.Equal(t, ClusterUUID, driver.Config.ClusterUUID)
	assert.Equal(t, KubeSystemUUID, driver.Config.KubeSystemUUID)
	assert.Equal(t, "test-", *driver.Config.StoragePrefix, "wrong storage prefix")
	assert.Equal(t, 1, len(driver.pools), "wrong number of pools")
	assert.Equal(t, BackendUUID, driver.backendUUID, "wrong backend UUID")
	assert.Equal(t, driver.volumeCreateTimeout, 10*time.Second, "volume create timeout mismatch")
	assert.True(t, driver.Initialized(), "not initialized")
}

func TestInitialize_WithInvalidSecrets(t *testing.T) {
	configJSON := `
    {
        "version": 1,
        "storageDriverName": "astrads-nas",
        "debugTraceFlags": {"method": true, "api": true},
        "cluster": "fake-cluster",
        "namespace": "fake-namespace",
        "nfsMountOptions": "vers=4.1",
        "autoExportPolicy": false,
        "defaults": {
            "exportPolicy": "fake-export-policy",
            "size": "1G"
        }
    }`

	secrets := map[string]string{
		"KubeConfig": KubeconfigB64, // valid key is "kubeconfig"
	}

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAstraDS(mockCtrl)

	driver := &StorageDriver{
		API: mockAPI,
	}

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, secrets, BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "not initialized")
}

func TestInitialize_InvalidConfigJSON(t *testing.T) {
	configJSON := `
    {
        "version": 1,
        "storageDriverName": "astrads-nas",
        "debugTraceFlags": {"method": true, "api": true},
        "kubeconfig": "ZmFrZS1rdWJlY29uZmln",
        "cluster": "fake-cluster",
        "namespace": "fake-namespace",
        "nfsMountOptions": "vers=4.1",
        "autoExportPolicy": false,
        "defaults": {
            "exportPolicy": "fake-export-policy",
            "size": "1G",
        }
    }` // invalid JSON due to trailing comment after "1G"

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAstraDS(mockCtrl)

	driver := &StorageDriver{
		API: mockAPI,
	}

	driver.Config = drivers.AstraDSStorageDriverConfig{}
	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{}, BackendUUID)

	assert.NotNil(t, driver.Config.CommonStorageDriverConfig, "Driver Config not set")
	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "not initialized")
}

func TestInitialize_NoNamespace(t *testing.T) {
	configJSON := `
    {
        "version": 1,
        "storageDriverName": "astrads-nas",
        "debugTraceFlags": {"method": true, "api": true},
        "kubeconfig": "ZmFrZS1rdWJlY29uZmln",
        "cluster": "fake-cluster",
        "nfsMountOptions": "vers=4.1",
        "autoExportPolicy": false,
        "defaults": {
            "exportPolicy": "fake-export-policy",
            "size": "1G"
        }
    }`

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAstraDS(mockCtrl)

	driver := &StorageDriver{
		API: mockAPI,
	}

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{}, BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestInitialize_NoKubeconfig(t *testing.T) {
	configJSON := `
    {
        "version": 1,
        "storageDriverName": "astrads-nas",
        "debugTraceFlags": {"method": true, "api": true},
        "cluster": "fake-cluster",
        "namespace": "fake-namespace",
        "nfsMountOptions": "vers=4.1",
        "autoExportPolicy": false,
        "defaults": {
            "exportPolicy": "fake-export-policy",
            "size": "1G"
        }
    }`

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAstraDS(mockCtrl)

	driver := &StorageDriver{
		API: mockAPI,
	}

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{}, BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestInitialize_NoCluster(t *testing.T) {
	configJSON := `
    {
        "version": 1,
        "storageDriverName": "astrads-nas",
        "debugTraceFlags": {"method": true, "api": true},
        "kubeconfig": "ZmFrZS1rdWJlY29uZmln",
        "namespace": "fake-namespace",
        "nfsMountOptions": "vers=4.1",
        "autoExportPolicy": false,
        "defaults": {
            "exportPolicy": "fake-export-policy",
            "size": "1G"
        }
    }`

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAstraDS(mockCtrl)

	driver := &StorageDriver{
		API: mockAPI,
	}

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{}, BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestInitialize_APIInitError(t *testing.T) {
	configJSON := `
    {
        "version": 1,
        "storageDriverName": "astrads-nas",
        "debugTraceFlags": {"method": true, "api": true},
        "kubeconfig": "ZmFrZS1rdWJlY29uZmln",
        "cluster": "fake-cluster",
        "namespace": "fake-namespace",
        "nfsMountOptions": "vers=4.1",
        "autoExportPolicy": false,
        "defaults": {
            "exportPolicy": "fake-export-policy",
            "size": "1G"
        }
    }`

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAstraDS(mockCtrl)

	driver := &StorageDriver{
		API: mockAPI,
	}

	mockAPI.EXPECT().Init(ctx, Namespace, Cluster, []byte(Kubeconfig)).Return(nil, "", errFailed).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{}, BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "initialized")
}

func TestInitialize_InvalidStoragePrefix(t *testing.T) {
	badPrefix := "&trident"

	badCommonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "astrads-nas",
		StoragePrefix:     &badPrefix,
		BackendName:       BackendName,
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
        "version": 1,
        "storageDriverName": "astrads-nas",
        "debugTraceFlags": {"method": true, "api": true},
        "kubeconfig": "ZmFrZS1rdWJlY29uZmln",
        "cluster": "fake-cluster",
        "namespace": "fake-namespace",
        "nfsMountOptions": "vers=4.1",
        "autoExportPolicy": false,
        "defaults": {
            "exportPolicy": "fake-export-policy",
            "size": "1G"
        }
    }`

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAstraDS(mockCtrl)

	driver := &StorageDriver{
		API: mockAPI,
	}

	mockAPI.EXPECT().Init(ctx, Namespace, Cluster, []byte(Kubeconfig)).Return(adsCluster, KubeSystemUUID, nil).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextCSI, configJSON, badCommonConfig, map[string]string{}, BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "not initialized")
}

func TestInitialize_InvalidExportPolicyMode(t *testing.T) {
	badCommonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "astrads-nas",
		StoragePrefix:     &prefix,
		BackendName:       BackendName,
		DriverContext:     tridentconfig.ContextDocker,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
        "version": 1,
        "storageDriverName": "astrads-nas",
        "debugTraceFlags": {"method": true, "api": true},
        "kubeconfig": "ZmFrZS1rdWJlY29uZmln",
        "cluster": "fake-cluster",
        "namespace": "fake-namespace",
        "nfsMountOptions": "vers=4.1",
        "autoExportPolicy": true,
        "defaults": {
            "exportPolicy": "fake-export-policy",
            "size": "1G"
        }
    }`

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAstraDS(mockCtrl)

	driver := &StorageDriver{
		API: mockAPI,
	}

	mockAPI.EXPECT().Init(ctx, Namespace, Cluster, []byte(Kubeconfig)).Return(adsCluster, KubeSystemUUID, nil).Times(1)

	result := driver.Initialize(ctx, tridentconfig.ContextDocker, configJSON, badCommonConfig, map[string]string{}, BackendUUID)

	assert.Error(t, result, "initialize did not fail")
	assert.False(t, driver.Initialized(), "not initialized")
}

func TestInitialized(t *testing.T) {
	tests := []struct {
		Expected bool
	}{
		{true},
		{false},
	}
	for _, test := range tests {
		t.Run(strconv.FormatBool(test.Expected), func(t *testing.T) {
			_, driver := newMockAstraDSDriver(t)
			driver.initialized = test.Expected

			result := driver.Initialized()

			assert.Equal(t, test.Expected, result, "mismatched initialized values")
		})
	}
}

func TestTerminate(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)
	driver.initialized = true

	driver.Terminate(context.TODO(), "")

	assert.False(t, driver.initialized, "initialized not false")
}

func TestPopulateConfigurationDefaults_NoneSet(t *testing.T) {
	commonConfig = &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "astrads-nas",
		BackendName:       BackendName,
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	config := &drivers.AstraDSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		Kubeconfig:                KubeconfigB64,
		Cluster:                   Cluster,
		Namespace:                 Namespace,
		AutoExportPolicy:          false,
	}

	_, driver := newMockAstraDSDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	assert.Equal(t, "trident-", *driver.Config.StoragePrefix)
	assert.Equal(t, drivers.DefaultVolumeSize, driver.Config.Size)
	assert.Equal(t, defaultExportPolicy, driver.Config.ExportPolicy)
	assert.Equal(t, defaultUnixPermissions, driver.Config.UnixPermissions)
	assert.Equal(t, defaultSnapshotReserve, driver.Config.SnapshotReserve)
	assert.Equal(t, defaultSnapshotDir, driver.Config.SnapshotDir)
	assert.Equal(t, defaultNfsMountOptions, driver.Config.NfsMountOptions)
	assert.Equal(t, defaultLimitVolumeSize, driver.Config.LimitVolumeSize)
	assert.Equal(t, volumeCreateTimeout, driver.volumeCreateTimeout)
}

func TestPopulateConfigurationDefaults_AllSet(t *testing.T) {
	commonConfig = &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "astrads-nas",
		BackendName:       BackendName,
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
		StoragePrefix:     &prefix,
		LimitVolumeSize:   "123456789000",
	}

	config := &drivers.AstraDSStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		Kubeconfig:                KubeconfigB64,
		Cluster:                   Cluster,
		Namespace:                 Namespace,
		NfsMountOptions:           "vers=3",
		AutoExportPolicy:          false,

		AstraDSStorageDriverPool: drivers.AstraDSStorageDriverPool{
			AstraDSStorageDriverConfigDefaults: drivers.AstraDSStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: "1234567890",
				},
				ExportPolicy:    "my-export-policy",
				UnixPermissions: "0700",
				SnapshotReserve: "10",
				SnapshotDir:     "true",
			},
		},
	}

	_, driver := newMockAstraDSDriver(t)
	driver.Config = *config

	driver.populateConfigurationDefaults(ctx, &driver.Config)

	assert.Equal(t, "test-", *driver.Config.StoragePrefix)
	assert.Equal(t, "1234567890", driver.Config.Size)
	assert.Equal(t, "my-export-policy", driver.Config.ExportPolicy)
	assert.Equal(t, "0700", driver.Config.UnixPermissions)
	assert.Equal(t, "10", driver.Config.SnapshotReserve)
	assert.Equal(t, "true", driver.Config.SnapshotDir)
	assert.Equal(t, "vers=3", driver.Config.NfsMountOptions)
	assert.Equal(t, "123456789000", driver.Config.LimitVolumeSize)
	assert.Equal(t, volumeCreateTimeout, driver.volumeCreateTimeout)
}

func TestInitializeStoragePools_NoVirtualPools(t *testing.T) {
	supportedTopologies := []map[string]string{
		{"topology.kubernetes.io/region": "europe-west1", "topology.kubernetes.io/zone": "us-east-1c"},
	}

	config := &drivers.AstraDSStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			BackendName:     BackendName,
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
			LimitVolumeSize: "123456789000",
		},
		Kubeconfig:       KubeconfigB64,
		Cluster:          Cluster,
		Namespace:        Namespace,
		NfsMountOptions:  "nfsvers=4.1",
		AutoExportPolicy: false,
		AstraDSStorageDriverPool: drivers.AstraDSStorageDriverPool{
			AstraDSStorageDriverConfigDefaults: drivers.AstraDSStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: "1234567890",
				},
				ExportPolicy:    "my-export-policy",
				UnixPermissions: "0700",
				SnapshotReserve: "10",
				SnapshotDir:     "true",
				QosPolicy:       "bronze",
			},
			Region:              "region1",
			Zone:                "zone1",
			SupportedTopologies: supportedTopologies,
		},
	}

	_, driver := newMockAstraDSDriver(t)
	driver.Config = *config

	driver.initializeStoragePools(ctx)

	pool := storage.NewStoragePool(nil, "myADSBackend_pool")
	pool.Attributes()[sa.BackendType] = sa.NewStringOffer(driver.Name())
	pool.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
	pool.Attributes()[sa.ProvisioningType] = sa.NewStringOffer("thin")
	pool.Attributes()[sa.Labels] = sa.NewLabelOffer(driver.Config.Labels)
	pool.Attributes()[sa.Region] = sa.NewStringOffer("region1")
	pool.Attributes()[sa.Zone] = sa.NewStringOffer("zone1")

	pool.InternalAttributes()[Size] = "1234567890"
	pool.InternalAttributes()[ExportPolicy] = "my-export-policy"
	pool.InternalAttributes()[UnixPermissions] = "0700"
	pool.InternalAttributes()[SnapshotReserve] = "10"
	pool.InternalAttributes()[SnapshotDir] = "true"
	pool.InternalAttributes()[QosPolicy] = "bronze"
	pool.InternalAttributes()[Region] = "region1"
	pool.InternalAttributes()[Zone] = "zone1"

	pool.SetSupportedTopologies(supportedTopologies)

	expectedPools := map[string]storage.Pool{"myADSBackend_pool": pool}

	assert.Equal(t, expectedPools, driver.pools, "pools do not match")
}

func TestInitializeStoragePools_VirtualPools(t *testing.T) {
	supportedTopologies1 := []map[string]string{
		{"topology.kubernetes.io/region": "us-east", "topology.kubernetes.io/zone": "us-east-1b"},
	}
	supportedTopologies2 := []map[string]string{
		{"topology.kubernetes.io/region": "es-east", "topology.kubernetes.io/zone": "us-east-1c"},
	}

	config := &drivers.AstraDSStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			BackendName:     BackendName,
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
			LimitVolumeSize: "123456789000",
		},
		Kubeconfig:       KubeconfigB64,
		Cluster:          Cluster,
		Namespace:        Namespace,
		NfsMountOptions:  "nfsvers=4.1",
		AutoExportPolicy: false,
		AstraDSStorageDriverPool: drivers.AstraDSStorageDriverPool{
			AstraDSStorageDriverConfigDefaults: drivers.AstraDSStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: "1234567890",
				},
				ExportPolicy:    "my-export-policy",
				UnixPermissions: "0700",
				SnapshotReserve: "10",
				SnapshotDir:     "true",
				QosPolicy:       "bronze",
			},
			Region: "region1",
			Zone:   "zone1",
		},
		Storage: []drivers.AstraDSStorageDriverPool{
			{
				AstraDSStorageDriverConfigDefaults: drivers.AstraDSStorageDriverConfigDefaults{
					CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
						Size: "123456789000",
					},
					UnixPermissions: "0700",
				},
				Region:              "region1",
				Zone:                "zone2",
				SupportedTopologies: supportedTopologies1,
			},
			{
				AstraDSStorageDriverConfigDefaults: drivers.AstraDSStorageDriverConfigDefaults{
					ExportPolicy:    "gold-export-policy",
					UnixPermissions: "0770",
					SnapshotReserve: "20",
					SnapshotDir:     "false",
					QosPolicy:       "gold",
				},
				SupportedTopologies: supportedTopologies2,
			},
		},
	}

	_, driver := newMockAstraDSDriver(t)
	driver.Config = *config

	driver.initializeStoragePools(ctx)

	pool0 := storage.NewStoragePool(nil, "myADSBackend_pool_0")
	pool0.Attributes()[sa.BackendType] = sa.NewStringOffer(driver.Name())
	pool0.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool0.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool0.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool0.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
	pool0.Attributes()[sa.ProvisioningType] = sa.NewStringOffer("thin")
	pool0.Attributes()[sa.Labels] = sa.NewLabelOffer(driver.Config.Labels)
	pool0.Attributes()[sa.Region] = sa.NewStringOffer("region1")
	pool0.Attributes()[sa.Zone] = sa.NewStringOffer("zone2")

	pool0.InternalAttributes()[Size] = "123456789000"
	pool0.InternalAttributes()[ExportPolicy] = "my-export-policy"
	pool0.InternalAttributes()[UnixPermissions] = "0700"
	pool0.InternalAttributes()[SnapshotReserve] = "10"
	pool0.InternalAttributes()[SnapshotDir] = "true"
	pool0.InternalAttributes()[QosPolicy] = "bronze"
	pool0.InternalAttributes()[Region] = "region1"
	pool0.InternalAttributes()[Zone] = "zone2"

	pool0.SetSupportedTopologies(supportedTopologies1)

	pool1 := storage.NewStoragePool(nil, "myADSBackend_pool_1")
	pool1.Attributes()[sa.BackendType] = sa.NewStringOffer(driver.Name())
	pool1.Attributes()[sa.Snapshots] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Clones] = sa.NewBoolOffer(true)
	pool1.Attributes()[sa.Encryption] = sa.NewBoolOffer(false)
	pool1.Attributes()[sa.Replication] = sa.NewBoolOffer(false)
	pool1.Attributes()[sa.ProvisioningType] = sa.NewStringOffer("thin")
	pool1.Attributes()[sa.Labels] = sa.NewLabelOffer(driver.Config.Labels)
	pool1.Attributes()[sa.Region] = sa.NewStringOffer("region1")
	pool1.Attributes()[sa.Zone] = sa.NewStringOffer("zone1")

	pool1.InternalAttributes()[Size] = "1234567890"
	pool1.InternalAttributes()[ExportPolicy] = "gold-export-policy"
	pool1.InternalAttributes()[UnixPermissions] = "0770"
	pool1.InternalAttributes()[SnapshotReserve] = "20"
	pool1.InternalAttributes()[SnapshotDir] = "false"
	pool1.InternalAttributes()[QosPolicy] = "gold"
	pool1.InternalAttributes()[Region] = "region1"
	pool1.InternalAttributes()[Zone] = "zone1"

	pool1.SetSupportedTopologies(supportedTopologies2)

	expectedPools := map[string]storage.Pool{
		"myADSBackend_pool_0": pool0,
		"myADSBackend_pool_1": pool1,
	}

	assert.Equal(t, expectedPools, driver.pools, "pools do not match")
}

func TestValidate_InvalidClusterVersion(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.cluster.Version = "2022.1"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	mockAPI.EXPECT().QosPolicies(ctx).Return(qosPolicies, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, staticExportPolicy, nil).Times(1)

	result := driver.validate(ctx)

	assert.NoError(t, result, "validate failed")
}

func TestValidate_UnsupportedClusterVersion(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)
	driver.cluster.Version = "2021.1.0"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	result := driver.validate(ctx)

	assert.Error(t, result, "validate failed")
}

func TestValidate_InvalidExportPolicyCIDR(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)
	driver.Config.AutoExportCIDRs = []string{"1.1.1.1.1"}

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	result := driver.validate(ctx)

	assert.Error(t, result, "validate failed")
}

func TestValidate_QosPolicyReadFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	mockAPI.EXPECT().QosPolicies(ctx).Return(nil, errFailed).Times(1)

	result := driver.validate(ctx)

	assert.Error(t, result, "validate failed")
}

func TestValidate_InvalidSnapshotDir(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.SnapshotDir = "yes"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	mockAPI.EXPECT().QosPolicies(ctx).Return(qosPolicies, nil).Times(1)

	result := driver.validate(ctx)

	assert.Error(t, result, "validate did not fail")
}

func TestValidate_InvalidSize(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.Size = "abcde"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	mockAPI.EXPECT().QosPolicies(ctx).Return(qosPolicies, nil).Times(1)

	result := driver.validate(ctx)

	assert.Error(t, result, "validate did not fail")
}

func TestValidate_NonexistentQosPolicy(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.QosPolicy = "platinum"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	mockAPI.EXPECT().QosPolicies(ctx).Return(qosPolicies, nil).Times(1)

	result := driver.validate(ctx)

	assert.Error(t, result, "validate did not fail")
}

func TestValidate_ExportPolicyReadFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	mockAPI.EXPECT().QosPolicies(ctx).Return(qosPolicies, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(false, nil, errFailed).Times(1)

	result := driver.validate(ctx)

	assert.Error(t, result, "validate did not fail")
}

func TestValidate_NonexistentExportPolicy(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	mockAPI.EXPECT().QosPolicies(ctx).Return(qosPolicies, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(false, nil, nil).Times(1)

	result := driver.validate(ctx)

	assert.Error(t, result, "validate did not fail")
}

func TestValidate_InvalidLabel(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.Config.Labels = map[string]string{
		"key1": "1234567890",
		"key2": "1234567890",
		"key3": utils.RandomString(maxAnnotationLength),
	}

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	mockAPI.EXPECT().QosPolicies(ctx).Return(qosPolicies, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, staticExportPolicy, nil).Times(1)

	result := driver.validate(ctx)

	assert.Error(t, result, "validate did not fail")
}

func getStructsForCreate(ctx context.Context, driver *StorageDriver, storagePool storage.Pool) (
	*storage.VolumeConfig, *api.Volume, *api.Volume,
) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
	}

	snapshotDir, _ := strconv.ParseBool(defaultSnapshotDir)

	labels := make(map[string]string)
	poolLabels, _ := storagePool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, maxAnnotationLength)
	labels[storage.ProvisioningLabelTag] = poolLabels

	poolLabelsJSON, _ := storagePool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, 256*1024)
	annotations := map[string]string{
		telemetryAnnotationKey:    driver.getTelemetryLabelsJSON(ctx),
		provisioningAnnotationKey: poolLabelsJSON,
	}

	requestedSize := resource.MustParse(strconv.FormatInt(113025455157, 10))

	createRequest := &api.Volume{
		Name:            volConfig.InternalName,
		Namespace:       driver.Config.Namespace,
		Annotations:     annotations,
		Labels:          make(map[string]string),
		DisplayName:     volConfig.Name,
		RequestedSize:   requestedSize,
		Type:            api.NetappVolumeTypeReadWrite,
		ExportPolicy:    staticExportPolicy.Name,
		QoSPolicy:       "silver",
		VolumePath:      "/" + volConfig.InternalName,
		Permissions:     "0777",
		SnapshotReserve: 5,
		NoSnapDir:       !snapshotDir,
	}

	newVolume := &api.Volume{
		Name:            volConfig.InternalName,
		Namespace:       driver.Config.Namespace,
		ResourceVersion: "1",
		Annotations:     annotations,
		Labels:          make(map[string]string),
		Finalizers:      []string{api.TridentVolumeFinalizer},

		Type:            api.NetappVolumeTypeReadWrite,
		VolumePath:      "/" + volConfig.InternalName,
		Permissions:     "0777",
		DisplayName:     volConfig.Name,
		Created:         true,
		State:           api.VolumeStateOnline,
		RequestedSize:   requestedSize,
		ActualSize:      requestedSize,
		VolumeUUID:      VolumeUUID,
		ExportAddress:   ExportAddress,
		ExportPolicy:    FakeExportPolicy,
		SnapshotReserve: 5,
		NoSnapDir:       !snapshotDir,
		QoSPolicy:       "silver",
	}

	return volConfig, createRequest, newVolume
}

func TestCreate(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, createRequest, volume := getStructsForCreate(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, volume, driver.volumeCreateTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
}

func TestCreate_InvalidName(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)
	volConfig.InternalName = "%trident-testvol1%"

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_NoStoragePool(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)

	result := driver.Create(ctx, volConfig, nil, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_NonexistentStoragePool(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]
	driver.pools = nil

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_VolumeExistsCreateError(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, volume := getStructsForCreate(ctx, driver, storagePool)
	volume.CreateError = api.VolumeCreateError("creation failed")

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_VolumeExistsCreating(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, volume := getStructsForCreate(ctx, driver, storagePool)
	volume.Created = false

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, volume, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.IsType(t, utils.VolumeCreatingError(""), result, "not VolumeCreatingError")
}

func TestCreate_VolumeExistsSetAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, volume := getStructsForCreate(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.IsType(t, drivers.NewVolumeExistsError(volConfig.InternalName), result, "not VolumeExistsError")
}

func TestCreate_VolumeExists(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, volume := getStructsForCreate(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.IsType(t, drivers.NewVolumeExistsError(volConfig.InternalName), result, "not VolumeExistsError")
}

func TestCreate_InvalidSnapshotReserve(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)
	volConfig.SnapshotReserve = "invalid"

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_SnapshotReserveOutOfRange(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)
	volConfig.SnapshotReserve = "95"

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_InvalidSnapshotDir(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)
	volConfig.SnapshotDir = "invalid"

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_AutoExportPolicy(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.AutoExportPolicy = true

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, createRequest, volume := getStructsForCreate(ctx, driver, storagePool)
	createRequest.ExportPolicy = volConfig.InternalName
	volume.ExportPolicy = volConfig.InternalName

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volConfig.InternalName).Return(nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, volume, driver.volumeCreateTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
}

func TestCreate_InvalidSize(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)
	volConfig.Size = "invalid"

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_NegativeSize(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)
	volConfig.Size = "-1M"

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_ZeroSize(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, createRequest, volume := getStructsForCreate(ctx, driver, storagePool)
	volConfig.Size = "0"
	expectedSize := resource.MustParse(strconv.FormatInt(1130254551, 10))
	createRequest.RequestedSize = expectedSize
	volume.RequestedSize = expectedSize
	volume.ActualSize = expectedSize

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, volume, driver.volumeCreateTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
}

func TestCreate_BelowAbsoluteMinimumSize(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)
	volConfig.Size = "1k"

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_AboveMaximumSize(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.LimitVolumeSize = "100Gi"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)
	volConfig.Size = "200Gi"

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_InvalidLabel(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]
	storagePool.Attributes()[sa.Labels] = sa.NewLabelOffer(map[string]string{
		"key1": "1234567890",
		"key2": "1234567890",
		"key3": utils.RandomString(maxAnnotationLength),
	})

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_AutoExportPolicyExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.AutoExportPolicy = true

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, createRequest, volume := getStructsForCreate(ctx, driver, storagePool)
	createRequest.ExportPolicy = volConfig.InternalName
	volume.ExportPolicy = volConfig.InternalName

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volConfig.InternalName).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_StaticExportPolicyExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(false, nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_NonexistentStaticExportPolicy(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, _, _ := getStructsForCreate(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(false, nil, nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_CreateFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, createRequest, _ := getStructsForCreate(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_Creating(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, createRequest, volume := getStructsForCreate(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, volume, driver.volumeCreateTimeout).Return(utils.VolumeCreatingError("creating")).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
	assert.IsType(t, utils.VolumeCreatingError(""), result, "not VolumeCreatingError")
}

func TestCreate_CreateError(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, createRequest, volume := getStructsForCreate(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, volume, driver.volumeCreateTimeout).Return(errFailed).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_CreateError_AutoExportPolicy(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.AutoExportPolicy = true

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, createRequest, volume := getStructsForCreate(ctx, driver, storagePool)
	createRequest.ExportPolicy = volConfig.InternalName
	volume.ExportPolicy = volConfig.InternalName

	autoExportPolicy := &api.ExportPolicy{
		Name:      volume.Name,
		Namespace: Namespace,
		Rules:     []api.ExportPolicyRule{},
	}

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volConfig.InternalName).Return(autoExportPolicy, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, volume, driver.volumeCreateTimeout).Return(errFailed).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, volume.Name).Return(true, autoExportPolicy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, autoExportPolicy, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)
	mockAPI.EXPECT().DeleteExportPolicy(ctx, autoExportPolicy.Name).Return(nil).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreate_SetAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	volConfig, createRequest, volume := getStructsForCreate(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, volConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(volume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, volume, driver.volumeCreateTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(errFailed).Times(1)

	result := driver.Create(ctx, volConfig, storagePool, nil)

	assert.NoError(t, result, "create failed")
}

func TestPadVolumeSizeWithSnapshotReserve(t *testing.T) {
	tests := []struct {
		Name            string
		SnapshotReserve int32
		SizeBytes       uint64
		PaddedSizeBytes uint64
	}{
		{
			Name:            "no reserve",
			SnapshotReserve: 0,
			SizeBytes:       1000000000,
			PaddedSizeBytes: 1000000000,
		},
		{
			Name:            "10% reserve",
			SnapshotReserve: 10,
			SizeBytes:       1000000000,
			PaddedSizeBytes: 1111111111,
		},
		{
			Name:            "reserve too low",
			SnapshotReserve: -1,
			SizeBytes:       1000000000,
			PaddedSizeBytes: 1000000000,
		},
		{
			Name:            "reserve too high",
			SnapshotReserve: 91,
			SizeBytes:       1000000000,
			PaddedSizeBytes: 1000000000,
		},
	}

	d := StorageDriver{}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			paddedSizeBytes := d.padVolumeSizeWithSnapshotReserve(context.TODO(), "volume",
				test.SizeBytes, test.SnapshotReserve)
			assert.Equal(t, test.PaddedSizeBytes, paddedSizeBytes, "incorrect size")
		})
	}
}

func TestUnpadVolumeSizeWithSnapshotReserve(t *testing.T) {
	tests := []struct {
		Name              string
		SnapshotReserve   int32
		SizeBytes         uint64
		UnpaddedSizeBytes uint64
	}{
		{
			Name:              "no reserve",
			SnapshotReserve:   0,
			SizeBytes:         1000000000,
			UnpaddedSizeBytes: 1000000000,
		},
		{
			Name:              "10% reserve",
			SnapshotReserve:   10,
			SizeBytes:         1000000000,
			UnpaddedSizeBytes: 900000000,
		},
		{
			Name:              "reserve too low",
			SnapshotReserve:   -1,
			SizeBytes:         1000000000,
			UnpaddedSizeBytes: 1000000000,
		},
		{
			Name:              "reserve too high",
			SnapshotReserve:   91,
			SizeBytes:         1000000000,
			UnpaddedSizeBytes: 1000000000,
		},
	}

	d := StorageDriver{}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			unpaddedSizeBytes := d.unpadVolumeSizeWithSnapshotReserve(context.TODO(), "volume",
				test.SizeBytes, test.SnapshotReserve)
			assert.Equal(t, test.UnpaddedSizeBytes, unpaddedSizeBytes, "incorrect size")
		})
	}
}

func getStructsForCreateClone(ctx context.Context, driver *StorageDriver, storagePool storage.Pool) (
	*storage.VolumeConfig, *storage.VolumeConfig, *api.Volume, *api.Volume, *api.Volume,
) {
	sourceVolConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
	}

	cloneVolConfig := &storage.VolumeConfig{
		Version:                   "1",
		Name:                      "testvol2",
		InternalName:              "trident-testvol2",
		CloneSourceVolume:         "testvol1",
		CloneSourceVolumeInternal: "trident-testvol1",
	}

	snapshotDir, _ := strconv.ParseBool(defaultSnapshotDir)

	labels := make(map[string]string)
	poolLabels, _ := storagePool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, maxAnnotationLength)
	labels[storage.ProvisioningLabelTag] = poolLabels

	poolLabelsJSON, _ := storagePool.GetLabelsJSON(ctx, storage.ProvisioningLabelTag, 256*1024)
	annotations := map[string]string{
		telemetryAnnotationKey:    driver.getTelemetryLabelsJSON(ctx),
		provisioningAnnotationKey: poolLabelsJSON,
	}

	requestedSize := resource.MustParse(strconv.FormatInt(113025455157, 10))

	createRequest := &api.Volume{
		Name:            cloneVolConfig.InternalName,
		Namespace:       driver.Config.Namespace,
		Annotations:     annotations,
		Labels:          make(map[string]string),
		DisplayName:     cloneVolConfig.Name,
		RequestedSize:   requestedSize,
		Type:            api.NetappVolumeTypeReadWrite,
		ExportPolicy:    staticExportPolicy.Name,
		QoSPolicy:       "silver",
		VolumePath:      "/" + cloneVolConfig.InternalName,
		Permissions:     "0777",
		SnapshotReserve: 5,
		NoSnapDir:       !snapshotDir,
		CloneVolume:     cloneVolConfig.CloneSourceVolumeInternal,
	}

	sourceVolume := &api.Volume{
		Name:            sourceVolConfig.InternalName,
		Namespace:       driver.Config.Namespace,
		ResourceVersion: "1",
		Annotations:     annotations,
		Labels:          make(map[string]string),
		Finalizers:      []string{api.TridentVolumeFinalizer},

		Type:            api.NetappVolumeTypeReadWrite,
		VolumePath:      "/" + sourceVolConfig.InternalName,
		Permissions:     "0777",
		DisplayName:     sourceVolConfig.Name,
		Created:         true,
		State:           api.VolumeStateOnline,
		RequestedSize:   requestedSize,
		ActualSize:      requestedSize,
		VolumeUUID:      VolumeUUID,
		ExportAddress:   ExportAddress,
		ExportPolicy:    FakeExportPolicy,
		SnapshotReserve: 5,
		NoSnapDir:       !snapshotDir,
		QoSPolicy:       "silver",
	}

	cloneVolume := &api.Volume{
		Name:            cloneVolConfig.InternalName,
		Namespace:       driver.Config.Namespace,
		ResourceVersion: "1",
		Annotations:     annotations,
		Labels:          make(map[string]string),
		Finalizers:      []string{api.TridentVolumeFinalizer},

		Type:            api.NetappVolumeTypeReadWrite,
		VolumePath:      "/" + cloneVolConfig.InternalName,
		Permissions:     "0777",
		DisplayName:     cloneVolConfig.Name,
		Created:         true,
		State:           api.VolumeStateOnline,
		RequestedSize:   requestedSize,
		ActualSize:      requestedSize,
		VolumeUUID:      VolumeUUID,
		ExportAddress:   ExportAddress,
		ExportPolicy:    FakeExportPolicy,
		SnapshotReserve: 5,
		NoSnapDir:       !snapshotDir,
		QoSPolicy:       "silver",
		CloneVolume:     cloneVolConfig.CloneSourceVolumeInternal,
	}

	return sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume
}

func TestCreateClone_NoSnapshot(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.QosPolicy = "gold"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneVolume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, cloneVolume, driver.volumeCreateTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, cloneVolume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.NoError(t, result, "create failed")
}

func TestCreateClone_Snapshot(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.QosPolicy = "gold"

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)
	cloneVolConfig.CloneSourceSnapshot = "snap1"
	createRequest.CloneVolume = ""
	createRequest.CloneSnapshot = "snap1"
	cloneVolume.CloneVolume = ""
	cloneVolume.CloneSnapshot = "snap1"

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().Snapshot(ctx, cloneVolConfig.CloneSourceSnapshot).Return(nil, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneVolume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, cloneVolume, driver.volumeCreateTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, cloneVolume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.NoError(t, result, "create failed")
}

func TestCreateClone_InvalidName(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, _, _, _ := getStructsForCreateClone(ctx, driver, storagePool)
	cloneVolConfig.InternalName = "%trident-testvol1%"

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
}

func TestCreateClone_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, _, _, _ := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
}

func TestCreateClone_VolumeExistsCreateError(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, _, _, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)
	cloneVolume.CreateError = api.VolumeCreateError("creation failed")

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(true, cloneVolume, nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, cloneVolume).Return(nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
}

func TestCreateClone_VolumeExistsCreating(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, _, _, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)
	cloneVolume.Created = false

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(true, cloneVolume, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
	assert.IsType(t, utils.VolumeCreatingError(""), result, "not VolumeCreatingError")
}

func TestCreateClone_VolumeExistsSetAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, _, _, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(true, cloneVolume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, cloneVolume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
	assert.IsType(t, drivers.NewVolumeExistsError(cloneVolConfig.InternalName), result, "not VolumeExistsError")
}

func TestCreateClone_VolumeExists(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, _, _, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(true, cloneVolume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, cloneVolume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
	assert.IsType(t, drivers.NewVolumeExistsError(cloneVolConfig.InternalName), result, "not VolumeExistsError")
}

func TestCreateClone_NonexistentSourceVolume(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, _, _, _ := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
}

func TestCreateClone_NonexistentSourceSnapshot(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)
	cloneVolConfig.CloneSourceSnapshot = "snap1"
	createRequest.CloneVolume = ""
	createRequest.CloneSnapshot = "snap1"
	cloneVolume.CloneVolume = ""
	cloneVolume.CloneSnapshot = "snap1"

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().Snapshot(ctx, cloneVolConfig.CloneSourceSnapshot).Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
}

func TestCreateClone_AutoExportPolicy(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.AutoExportPolicy = true

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)
	createRequest.ExportPolicy = cloneVolConfig.InternalName
	cloneVolume.ExportPolicy = cloneVolConfig.InternalName

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, cloneVolConfig.InternalName).Return(nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneVolume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, cloneVolume, driver.volumeCreateTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, cloneVolume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.NoError(t, result, "create failed")
}

func TestCreateClone_InvalidLabel(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.Labels = map[string]string{
		"key1": "1234567890",
		"key2": "1234567890",
		"key3": utils.RandomString(maxAnnotationLength),
	}

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)
	createRequest.ExportPolicy = cloneVolConfig.InternalName
	cloneVolume.ExportPolicy = cloneVolConfig.InternalName

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(sourceVolume, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, nil)

	assert.Error(t, result, "create did not fail")
}

func TestCreateClone_AutoExportPolicyExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.AutoExportPolicy = true

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)
	createRequest.ExportPolicy = cloneVolConfig.InternalName
	cloneVolume.ExportPolicy = cloneVolConfig.InternalName

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, cloneVolConfig.InternalName).Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
}

func TestCreateClone_StaticExportPolicyExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)
	createRequest.ExportPolicy = cloneVolConfig.InternalName
	cloneVolume.ExportPolicy = cloneVolConfig.InternalName

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(false, nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
}

func TestCreateClone_NonexistentStaticExportPolicy(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)
	createRequest.ExportPolicy = cloneVolConfig.InternalName
	cloneVolume.ExportPolicy = cloneVolConfig.InternalName

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(false, nil, nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
}

func TestCreateClone_CreateFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, _ := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(nil, errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
}

func TestCreateClone_Creating(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneVolume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, cloneVolume, driver.volumeCreateTimeout).Return(utils.VolumeCreatingError("creating")).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
	assert.IsType(t, utils.VolumeCreatingError(""), result, "not VolumeCreatingError")
}

func TestCreateClone_CreateError(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneVolume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, cloneVolume, driver.volumeCreateTimeout).Return(errFailed).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, cloneVolume).Return(nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
}

func TestCreateClone_CreateError_AutoExportPolicy(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.AutoExportPolicy = true

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)
	createRequest.ExportPolicy = cloneVolConfig.InternalName
	cloneVolume.ExportPolicy = cloneVolConfig.InternalName

	autoExportPolicy := &api.ExportPolicy{
		Name:      cloneVolume.Name,
		Namespace: Namespace,
		Rules:     []api.ExportPolicyRule{},
	}

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, cloneVolConfig.InternalName).Return(autoExportPolicy, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneVolume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, cloneVolume, driver.volumeCreateTimeout).Return(errFailed).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, cloneVolume).Return(nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, cloneVolume.Name).Return(true, autoExportPolicy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, autoExportPolicy, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)
	mockAPI.EXPECT().DeleteExportPolicy(ctx, autoExportPolicy.Name).Return(nil).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.Error(t, result, "create did not fail")
}

func TestCreateClone_SetAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	sourceVolConfig, cloneVolConfig, createRequest, sourceVolume, cloneVolume := getStructsForCreateClone(ctx, driver, storagePool)

	mockAPI.EXPECT().VolumeExists(ctx, cloneVolConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().Volume(ctx, cloneVolConfig.CloneSourceVolumeInternal).Return(sourceVolume, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, FakeExportPolicy).Return(true, nil, nil).Times(1)
	mockAPI.EXPECT().CreateVolume(ctx, createRequest).Return(cloneVolume, nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeReady(ctx, cloneVolume, driver.volumeCreateTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, cloneVolume, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(errFailed).Times(1)

	result := driver.CreateClone(ctx, sourceVolConfig, cloneVolConfig, storagePool)

	assert.NoError(t, result, "create failed")
}

func TestCleanUpFailedVolume_DeleteFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	_, _, volume := getStructsForCreate(ctx, driver, storagePool)

	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(errFailed).Times(1)

	driver.cleanUpFailedVolume(ctx, volume)
}

func TestCleanUpFailedVolume_AutoExportPolicy(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.AutoExportPolicy = true

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	_, _, volume := getStructsForCreate(ctx, driver, storagePool)
	volume.ExportPolicy = volume.Name

	autoExportPolicy := &api.ExportPolicy{
		Name:      volume.Name,
		Namespace: Namespace,
		Rules:     []api.ExportPolicyRule{},
	}

	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, volume.Name).Return(true, autoExportPolicy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, autoExportPolicy, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)
	mockAPI.EXPECT().DeleteExportPolicy(ctx, autoExportPolicy.Name).Return(nil).Times(1)

	driver.cleanUpFailedVolume(ctx, volume)
}

func TestCleanUpFailedVolume_AutoExportPolicyUnknownPolicy(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.Config.AutoExportPolicy = true

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	storagePool := driver.pools["myADSBackend_pool"]

	_, _, volume := getStructsForCreate(ctx, driver, storagePool)
	volume.ExportPolicy = ""

	autoExportPolicy := &api.ExportPolicy{
		Name:      volume.Name,
		Namespace: Namespace,
		Rules:     []api.ExportPolicyRule{},
	}

	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, volume.Name).Return(true, autoExportPolicy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, autoExportPolicy, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)
	mockAPI.EXPECT().DeleteExportPolicy(ctx, autoExportPolicy.Name).Return(nil).Times(1)

	driver.cleanUpFailedVolume(ctx, volume)
}

func getStructsForImport(_ context.Context, driver *StorageDriver) (*storage.VolumeConfig, *api.Volume) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
	}

	originalVolume := &api.Volume{
		Name:            "importMe",
		Namespace:       driver.Config.Namespace,
		ResourceVersion: "1",
		Annotations: map[string]string{
			provisioningAnnotationKey: "fakeProvisioningLabel",
			"fakeAnnotationKey":       "fakeAnnotationLabel",
		},
		Labels:     make(map[string]string),
		Finalizers: []string{},

		Type:            api.NetappVolumeTypeReadWrite,
		VolumePath:      "/" + volConfig.InternalName,
		Permissions:     "0777",
		DisplayName:     volConfig.Name,
		Created:         true,
		State:           api.VolumeStateOnline,
		RequestedSize:   resource.MustParse(VolumeSizeStr),
		ActualSize:      resource.MustParse(VolumeSizeStr),
		VolumeUUID:      VolumeUUID,
		ExportAddress:   ExportAddress,
		ExportPolicy:    FakeExportPolicy,
		SnapshotReserve: 5,
		NoSnapDir:       false,
		QoSPolicy:       "silver",
	}

	return volConfig, originalVolume
}

func TestImport_Managed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	originalName := "importMe"

	volConfig, originalVolume := getStructsForImport(ctx, driver)
	originalVolume.Name = originalName

	expectedAnnotations := map[string]string{
		telemetryAnnotationKey: driver.getTelemetryLabelsJSON(ctx),
		"fakeAnnotationKey":    "fakeAnnotationLabel",
	}

	mockAPI.EXPECT().VolumeExists(ctx, originalName).Return(true, originalVolume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, originalVolume, roaring.BitmapOf(api.UpdateFlagAnnotations)).Return(nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "102005473280", volConfig.Size, "size mismatch")
	assert.Equal(t, expectedAnnotations, originalVolume.Annotations, "annotations mismatch")
}

func TestImport_ManagedNoAnnotations(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	originalName := "importMe"

	volConfig, originalVolume := getStructsForImport(ctx, driver)
	originalVolume.Name = originalName
	originalVolume.Annotations = nil

	expectedAnnotations := map[string]string{
		telemetryAnnotationKey: driver.getTelemetryLabelsJSON(ctx),
	}

	mockAPI.EXPECT().VolumeExists(ctx, originalName).Return(true, originalVolume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, originalVolume, roaring.BitmapOf(api.UpdateFlagAnnotations)).Return(nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "102005473280", volConfig.Size, "size mismatch")
	assert.Equal(t, expectedAnnotations, originalVolume.Annotations, "annotations mismatch")
}

func TestImport_NotManaged(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	originalName := "importMe"

	volConfig, originalVolume := getStructsForImport(ctx, driver)
	volConfig.ImportNotManaged = true
	originalVolume.Name = originalName

	expectedAnnotations := map[string]string{
		provisioningAnnotationKey: "fakeProvisioningLabel",
		"fakeAnnotationKey":       "fakeAnnotationLabel",
	}

	mockAPI.EXPECT().VolumeExists(ctx, originalName).Return(true, originalVolume, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NoError(t, result, "import failed")
	assert.Equal(t, originalName, volConfig.InternalName, "internal name mismatch")
	assert.Equal(t, "102005473280", volConfig.Size, "size mismatch")
	assert.Equal(t, expectedAnnotations, originalVolume.Annotations, "annotations mismatch")
}

func TestImport_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	originalName := "importMe"

	volConfig, originalVolume := getStructsForImport(ctx, driver)
	originalVolume.Name = originalName

	mockAPI.EXPECT().VolumeExists(ctx, originalName).Return(false, nil, errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NotNil(t, result, "import did not fail")
}

func TestImport_NotFound(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	originalName := "importMe"

	volConfig, originalVolume := getStructsForImport(ctx, driver)
	originalVolume.Name = originalName

	mockAPI.EXPECT().VolumeExists(ctx, originalName).Return(false, nil, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NotNil(t, result, "import did not fail")
}

func TestImport_VolumeDeleting(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	originalName := "importMe"

	volConfig, originalVolume := getStructsForImport(ctx, driver)
	originalVolume.Name = originalName
	originalVolume.DeletionTimestamp = &now

	mockAPI.EXPECT().VolumeExists(ctx, originalName).Return(true, originalVolume, nil).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NotNil(t, result, "import did not fail")
}

func TestImport_SetAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	originalName := "importMe"

	volConfig, originalVolume := getStructsForImport(ctx, driver)
	originalVolume.Name = originalName

	mockAPI.EXPECT().VolumeExists(ctx, originalName).Return(true, originalVolume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, originalVolume, roaring.BitmapOf(api.UpdateFlagAnnotations)).Return(errFailed).Times(1)

	result := driver.Import(ctx, volConfig, originalName)

	assert.NotNil(t, result, "import did not fail")
}

func TestRename(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	result := driver.Rename(ctx, "oldName", "newName")

	assert.Nil(t, result, "rename failed")
}

func TestValidateLabels(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	labels := map[string]string{
		"labelKey1":  "labelValue1",
		"labelKey2":  "labelValue2",
		"_labelKey3": "labelValue3",
	}

	expectedLabels := map[string]string{
		"labelKey1": "labelValue1",
		"labelKey2": "labelValue2",
	}

	result := driver.validateLabels(ctx, labels)

	assert.Equal(t, expectedLabels, result, "label mismatch")
}

func getStructsForDestroy(_ context.Context, driver *StorageDriver) (*storage.VolumeConfig, *api.Volume) {
	requestedSize := resource.MustParse(strconv.FormatInt(113025455157, 10))
	actualSize := resource.MustParse(strconv.FormatInt(113025454080, 10))

	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
	}

	volume := &api.Volume{
		Name:            "trident-testvol1",
		Namespace:       driver.Config.Namespace,
		ResourceVersion: "1",
		Annotations: map[string]string{
			provisioningAnnotationKey: "fakeProvisioningLabel",
			"fakeAnnotationKey":       "fakeAnnotationLabel",
		},
		Labels:     make(map[string]string),
		Finalizers: []string{api.TridentVolumeFinalizer},

		Type:            api.NetappVolumeTypeReadWrite,
		VolumePath:      "/" + volConfig.InternalName,
		Permissions:     "0777",
		DisplayName:     volConfig.Name,
		Created:         true,
		State:           api.VolumeStateOnline,
		RequestedSize:   requestedSize,
		ActualSize:      actualSize,
		VolumeUUID:      VolumeUUID,
		ExportAddress:   ExportAddress,
		ExportPolicy:    FakeExportPolicy,
		SnapshotReserve: 5,
		NoSnapDir:       false,
		QoSPolicy:       "silver",
	}

	return volConfig, volume
}

func TestDestroy(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	volConfig, volume := getStructsForDestroy(ctx, driver)

	mockAPI.EXPECT().VolumeExists(ctx, volume.Name).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeDeleted(ctx, volume, driver.defaultDeleteTimeout()).Return(nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, volume.Name).Return(false, nil, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "destroy failed")
}

func TestDestroy_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	volConfig, volume := getStructsForDestroy(ctx, driver)

	mockAPI.EXPECT().VolumeExists(ctx, volume.Name).Return(false, nil, errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "destroy did not fail")
}

func TestDestroy_NotFound(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	volConfig, volume := getStructsForDestroy(ctx, driver)

	mockAPI.EXPECT().VolumeExists(ctx, volume.Name).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().ExportPolicyExists(ctx, volume.Name).Return(false, nil, nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.Nil(t, result, "destroy failed")
}

func TestDestroy_SetAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	volConfig, volume := getStructsForDestroy(ctx, driver)

	mockAPI.EXPECT().VolumeExists(ctx, volume.Name).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "destroy did not fail")
}

func TestDestroy_AlreadyDeleting(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	volConfig, volume := getStructsForDestroy(ctx, driver)
	volume.DeletionTimestamp = &now

	mockAPI.EXPECT().VolumeExists(ctx, volume.Name).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "destroy did not fail")
}

func TestDestroy_DeleteFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	volConfig, volume := getStructsForDestroy(ctx, driver)

	mockAPI.EXPECT().VolumeExists(ctx, volume.Name).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "destroy did not fail")
}

func TestDestroy_VolumeWaitFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	volConfig, volume := getStructsForDestroy(ctx, driver)

	mockAPI.EXPECT().VolumeExists(ctx, volume.Name).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)
	mockAPI.EXPECT().DeleteVolume(ctx, volume).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeDeleted(ctx, volume, driver.defaultDeleteTimeout()).Return(errFailed).Times(1)

	result := driver.Destroy(ctx, volConfig)

	assert.NotNil(t, result, "destroy did not fail")
}

func TestDestroyExportPolicy(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	exportPolicy := &api.ExportPolicy{
		Name:      "myPolicy",
		Namespace: Namespace,
		Rules:     []api.ExportPolicyRule{},
	}

	mockAPI.EXPECT().ExportPolicyExists(ctx, "myPolicy").Return(true, exportPolicy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, exportPolicy, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)
	mockAPI.EXPECT().DeleteExportPolicy(ctx, exportPolicy.Name).Return(nil).Times(1)

	var err error

	driver.destroyExportPolicy(ctx, "myPolicy", &err)
}

func TestDestroyExportPolicy_NonNilError(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	driver.destroyExportPolicy(ctx, "myPolicy", &errFailed)
}

func TestDestroyExportPolicy_NoPolicy(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	var err error

	driver.destroyExportPolicy(ctx, "", &err)
}

func TestDestroyExportPolicy_SharedPolicy(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	var err error

	driver.destroyExportPolicy(ctx, FakeExportPolicy, &err)
}

func TestDestroyExportPolicy_SharedPoolPolicy(t *testing.T) {
	supportedTopologies1 := []map[string]string{
		{"topology.kubernetes.io/region": "us-east", "topology.kubernetes.io/zone": "us-east-1b"},
	}
	supportedTopologies2 := []map[string]string{
		{"topology.kubernetes.io/region": "es-east", "topology.kubernetes.io/zone": "us-east-1c"},
	}

	config := &drivers.AstraDSStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			BackendName:     BackendName,
			DriverContext:   tridentconfig.ContextCSI,
			DebugTraceFlags: debugTraceFlags,
			LimitVolumeSize: "123456789000",
		},
		Kubeconfig:       KubeconfigB64,
		Cluster:          Cluster,
		Namespace:        Namespace,
		NfsMountOptions:  "nfsvers=4.1",
		AutoExportPolicy: false,
		AstraDSStorageDriverPool: drivers.AstraDSStorageDriverPool{
			AstraDSStorageDriverConfigDefaults: drivers.AstraDSStorageDriverConfigDefaults{
				CommonStorageDriverConfigDefaults: drivers.CommonStorageDriverConfigDefaults{
					Size: "1234567890",
				},
				ExportPolicy:    "my-export-policy",
				UnixPermissions: "0700",
				SnapshotReserve: "10",
				SnapshotDir:     "true",
				QosPolicy:       "bronze",
			},
			Region: "region1",
			Zone:   "zone1",
		},
		Storage: []drivers.AstraDSStorageDriverPool{
			{
				AstraDSStorageDriverConfigDefaults: drivers.AstraDSStorageDriverConfigDefaults{
					ExportPolicy: "gold-export-policy",
				},
				Region:              "region1",
				Zone:                "zone2",
				SupportedTopologies: supportedTopologies1,
			},
			{
				AstraDSStorageDriverConfigDefaults: drivers.AstraDSStorageDriverConfigDefaults{
					ExportPolicy: "silver-export-policy",
				},
				SupportedTopologies: supportedTopologies2,
			},
		},
	}

	_, driver := newMockAstraDSDriver(t)
	driver.Config = *config

	driver.initializeStoragePools(ctx)

	var err error

	driver.destroyExportPolicy(ctx, "silver-export-policy", &err)
}

func TestDestroyExportPolicy_PolicyExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	mockAPI.EXPECT().ExportPolicyExists(ctx, "myPolicy").Return(false, nil, errFailed).Times(1)

	var err error

	driver.destroyExportPolicy(ctx, "myPolicy", &err)
}

func TestDestroyExportPolicy_NotFound(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	mockAPI.EXPECT().ExportPolicyExists(ctx, "myPolicy").Return(false, nil, nil).Times(1)

	var err error

	driver.destroyExportPolicy(ctx, "myPolicy", &err)
}

func TestDestroyExportPolicy_SetAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	exportPolicy := &api.ExportPolicy{
		Name:      "myPolicy",
		Namespace: Namespace,
		Rules:     []api.ExportPolicyRule{},
	}

	mockAPI.EXPECT().ExportPolicyExists(ctx, "myPolicy").Return(true, exportPolicy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, exportPolicy, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(errFailed).Times(1)

	var err error

	driver.destroyExportPolicy(ctx, "myPolicy", &err)
}

func TestDestroyExportPolicy_DeleteFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	exportPolicy := &api.ExportPolicy{
		Name:      "myPolicy",
		Namespace: Namespace,
		Rules:     []api.ExportPolicyRule{},
	}

	mockAPI.EXPECT().ExportPolicyExists(ctx, "myPolicy").Return(true, exportPolicy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, exportPolicy, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)
	mockAPI.EXPECT().DeleteExportPolicy(ctx, exportPolicy.Name).Return(errFailed).Times(1)

	var err error

	driver.destroyExportPolicy(ctx, "myPolicy", &err)
}

func getStructsForPublish(
	_ context.Context, _ *StorageDriver,
) (*storage.VolumeConfig, *api.Volume, *utils.VolumePublishInfo, *api.ExportPolicy) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "myPrefix-testvol1",
		Size:         VolumeSizeStr,
	}

	labels := make(map[string]string)
	labels[storage.ProvisioningLabelTag] = ""

	volume := &api.Volume{
		Name:          "myPrefix-testvol1",
		Namespace:     Namespace,
		ActualSize:    resource.MustParse("1G"),
		Created:       true,
		State:         api.VolumeStateOnline,
		ExportAddress: "1.1.1.1",
		VolumePath:    "myPrefix-testvol1",
	}

	publishInfo := &utils.VolumePublishInfo{
		HostIP: []string{"10.10.10.10", "10.10.10.11"},
	}

	policy := &api.ExportPolicy{
		Name:      volume.Name,
		Namespace: Namespace,
		Rules:     []api.ExportPolicyRule{},
	}

	policy.Rules = append(policy.Rules, api.ExportPolicyRule{
		Clients:   []string{"10.10.20.20"},
		Protocols: []string{"nfs4"},
		RuleIndex: 1,
		RoRules:   []string{"any"},
		RwRules:   []string{"any"},
		SuperUser: []string{"any"},
		AnonUser:  65534,
	})

	return volConfig, volume, publishInfo, policy
}

func TestPublish(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	volConfig, volume, publishInfo, _ := getStructsForPublish(ctx, driver)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(volume, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Nil(t, result, "publish failed")
	assert.Equal(t, volume.ExportAddress, publishInfo.NfsServerIP, "export address mismatch")
	assert.Equal(t, "/myPrefix-testvol1", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "nfs", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, driver.Config.NfsMountOptions, publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_CustomMountOptions(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	volConfig, volume, publishInfo, _ := getStructsForPublish(ctx, driver)
	volConfig.MountOptions = "rw,vers=4"

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(volume, nil).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.Nil(t, result, "publish failed")
	assert.Equal(t, volume.ExportAddress, publishInfo.NfsServerIP, "export address mismatch")
	assert.Equal(t, "/myPrefix-testvol1", publishInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "nfs", publishInfo.FilesystemType, "filesystem type mismatch")
	assert.Equal(t, "rw,vers=4", publishInfo.MountOptions, "mount options mismatch")
}

func TestPublish_NotFound(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true

	volConfig, _, publishInfo, _ := getStructsForPublish(ctx, driver)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(nil, errFailed).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.NotNil(t, result, "publish did not fail")
}

func TestPublish_PublishFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true

	volConfig, volume, publishInfo, _ := getStructsForPublish(ctx, driver)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(volume, nil).Times(1)
	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volume.Name).Return(nil, errFailed).Times(1)

	result := driver.Publish(ctx, volConfig, publishInfo)

	assert.NotNil(t, result, "publish did not fail")
}

func TestPublishNFSShare_Unmanaged(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true

	volConfig, volume, publishInfo, _ := getStructsForPublish(ctx, driver)
	volConfig.ImportNotManaged = true
	publishInfo.Unmanaged = true

	result := driver.publishNFSShare(ctx, volConfig, publishInfo, volume)

	assert.Nil(t, result, "publish NFS share failed")
}

func TestPublishNFSShare_ExportPolicyAlreadySet(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"10.10.10.0/24", "10.10.20.0/24"}

	volConfig, volume, publishInfo, policy := getStructsForPublish(ctx, driver)
	policy.Name = volume.Name
	volume.ExportPolicy = policy.Name

	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volume.Name).Return(policy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, policy, roaring.BitmapOf(api.UpdateFlagExportRules)).Return(nil).Times(1)

	result := driver.publishNFSShare(ctx, volConfig, publishInfo, volume)

	assert.Nil(t, result, "publish NFS share failed")
}

func TestPublishNFSShare_SetExportPolicyAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"10.10.10.0/24", "10.10.20.0/24"}

	volConfig, volume, publishInfo, policy := getStructsForPublish(ctx, driver)
	policy.Name = volume.Name
	volume.ExportPolicy = policy.Name

	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volume.Name).Return(policy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, policy, roaring.BitmapOf(api.UpdateFlagExportRules)).Return(errFailed).Times(1)

	result := driver.publishNFSShare(ctx, volConfig, publishInfo, volume)

	assert.NotNil(t, result, "publish NFS share did not fail")
}

func TestPublishNFSShare_SetVolumeAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"10.10.10.0/24", "10.10.20.0/24"}

	volConfig, volume, publishInfo, policy := getStructsForPublish(ctx, driver)
	policy.Name = volume.Name
	volume.ExportPolicy = ""

	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volume.Name).Return(policy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, policy, roaring.BitmapOf(api.UpdateFlagExportRules)).Return(nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagExportPolicy)).Return(errFailed).Times(1)

	result := driver.publishNFSShare(ctx, volConfig, publishInfo, volume)

	assert.NotNil(t, result, "publish NFS share did not fail")
}

func TestPublishNFSShare_BadCIDR(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"10.10.10.0/33", "10.10.20.0/24"}

	volConfig, volume, publishInfo, policy := getStructsForPublish(ctx, driver)
	policy.Name = volume.Name
	volume.ExportPolicy = ""

	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volume.Name).Return(policy, nil).Times(1)

	result := driver.publishNFSShare(ctx, volConfig, publishInfo, volume)

	assert.NotNil(t, result, "publish NFS share did not fail")
}

func TestPublishNFSShare_VolumeUpdateNeeded(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"10.10.10.0/24", "10.10.20.0/24"}

	volConfig, volume, publishInfo, policy := getStructsForPublish(ctx, driver)
	policy.Name = volume.Name
	volume.ExportPolicy = ""

	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volume.Name).Return(policy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, policy, roaring.BitmapOf(api.UpdateFlagExportRules)).Return(nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagExportPolicy)).Return(nil).Times(1)

	result := driver.publishNFSShare(ctx, volConfig, publishInfo, volume)

	assert.Nil(t, result, "publish NFS share failed")
	assert.Equal(t, volume.Name, volume.ExportPolicy, "export policy mismatch")
	assert.Equal(t, 3, len(policy.Rules), "incorrect number of rules")

	// The order isn't guaranteed, so make a map so we can ensure everything is present
	ruleMap := map[string]api.ExportPolicyRule{
		policy.Rules[0].Clients[0]: policy.Rules[0],
		policy.Rules[1].Clients[0]: policy.Rules[1],
		policy.Rules[2].Clients[0]: policy.Rules[2],
	}
	assert.Contains(t, ruleMap, "10.10.20.20", "missing rule")
	assert.Contains(t, ruleMap, "10.10.10.10", "missing rule")
	assert.Contains(t, ruleMap, "10.10.10.11", "missing rule")

	// Everything else should be uniform, so we can just loop through the rules
	for i := 0; i < 3; i++ {
		assert.Equal(t, "nfs4", policy.Rules[i].Protocols[0], "incorrect rule protocol")
		assert.Equal(t, uint64(i+1), policy.Rules[i].RuleIndex, "incorrect rule index")
		assert.Equal(t, "any", policy.Rules[i].RoRules[0], "incorrect rule ro")
		assert.Equal(t, "any", policy.Rules[i].RwRules[0], "incorrect rule rw")
		assert.Equal(t, "any", policy.Rules[i].SuperUser[0], "incorrect rule superuser")
		assert.Equal(t, 65534, policy.Rules[i].AnonUser, "incorrect rule anon user")
	}
}

func getStructsForUnpublish(
	_ context.Context, _ *StorageDriver,
) (*storage.VolumeConfig, *api.Volume, []*utils.Node, *api.ExportPolicy, *api.ExportPolicy) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "myPrefix-testvol1",
		Size:         VolumeSizeStr,
	}

	labels := make(map[string]string)
	labels[storage.ProvisioningLabelTag] = ""

	volume := &api.Volume{
		Name:          "myPrefix-testvol1",
		Namespace:     Namespace,
		ActualSize:    resource.MustParse("1G"),
		Created:       true,
		State:         api.VolumeStateOnline,
		ExportAddress: "1.1.1.1",
		VolumePath:    "myPrefix-testvol1",
	}

	nodes := []*utils.Node{
		{
			IPs: []string{"10.10.10.10"},
		},
		{
			IPs: []string{"10.10.20.20", "10.10.20.21"},
		},
		{
			IPs: []string{"10.10.30.30"},
		},
	}

	oldPolicy := &api.ExportPolicy{
		Name:      volume.Name,
		Namespace: Namespace,
		Rules:     []api.ExportPolicyRule{},
	}
	oldPolicy.Rules = append(oldPolicy.Rules, api.ExportPolicyRule{
		Clients:   []string{"10.10.10.10"},
		Protocols: []string{"nfs4"},
		RuleIndex: 1,
		RoRules:   []string{"any"},
		RwRules:   []string{"any"},
		SuperUser: []string{"any"},
		AnonUser:  65534,
	})

	newPolicy := &api.ExportPolicy{
		Name:      volume.Name,
		Namespace: Namespace,
		Rules:     []api.ExportPolicyRule{},
	}
	newPolicy.Rules = append(newPolicy.Rules, api.ExportPolicyRule{
		Clients:   []string{"10.10.10.10"},
		Protocols: []string{"nfs4"},
		RuleIndex: 1,
		RoRules:   []string{"any"},
		RwRules:   []string{"any"},
		SuperUser: []string{"any"},
		AnonUser:  65534,
	})
	newPolicy.Rules = append(newPolicy.Rules, api.ExportPolicyRule{
		Clients:   []string{"10.10.20.20"},
		Protocols: []string{"nfs4"},
		RuleIndex: 2,
		RoRules:   []string{"any"},
		RwRules:   []string{"any"},
		SuperUser: []string{"any"},
		AnonUser:  65534,
	})
	newPolicy.Rules = append(newPolicy.Rules, api.ExportPolicyRule{
		Clients:   []string{"10.10.20.21"},
		Protocols: []string{"nfs4"},
		RuleIndex: 3,
		RoRules:   []string{"any"},
		RwRules:   []string{"any"},
		SuperUser: []string{"any"},
		AnonUser:  65534,
	})

	return volConfig, volume, nodes, oldPolicy, newPolicy
}

func TestUnpublish(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	volConfig, volume, nodes, _, _ := getStructsForUnpublish(ctx, driver)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(volume, nil).Times(1)

	result := driver.Unpublish(ctx, volConfig, nodes)

	assert.Nil(t, result, "unpublish failed")
}

func TestUnpublish_NotFound(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	volConfig, _, nodes, _, _ := getStructsForUnpublish(ctx, driver)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(nil, errFailed).Times(1)

	result := driver.Unpublish(ctx, volConfig, nodes)

	assert.NotNil(t, result, "unpublish did not fail")
}

func TestUnpublish_UnpublishFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true

	volConfig, volume, nodes, _, _ := getStructsForUnpublish(ctx, driver)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(volume, nil).Times(1)
	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volume.Name).Return(nil, errFailed).Times(1)

	result := driver.Unpublish(ctx, volConfig, nodes)

	assert.NotNil(t, result, "unpublish did not fail")
}

func TestUnpublishNFSShare_Unmanaged(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true

	volConfig, volume, nodes, _, _ := getStructsForUnpublish(ctx, driver)
	volConfig.ImportNotManaged = true

	result := driver.unpublishNFSShare(ctx, volConfig, nodes, volume)

	assert.Nil(t, result, "unpublish NFS share failed")
}

func TestUnpublishNFSShare_ExportPolicyAlreadySet(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"10.10.10.0/24", "10.10.20.0/24"}

	volConfig, volume, nodes, oldPolicy, newPolicy := getStructsForUnpublish(ctx, driver)
	oldPolicy.Name = volume.Name
	newPolicy.Name = volume.Name
	volume.ExportPolicy = volume.Name

	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volume.Name).Return(oldPolicy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, newPolicy, roaring.BitmapOf(api.UpdateFlagExportRules)).Return(nil).Times(1)

	result := driver.unpublishNFSShare(ctx, volConfig, nodes, volume)

	assert.Nil(t, result, "unpublish NFS share failed")
}

func TestUnpublishNFSShare_SetExportPolicyAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"10.10.10.0/24", "10.10.20.0/24"}

	volConfig, volume, nodes, oldPolicy, newPolicy := getStructsForUnpublish(ctx, driver)
	oldPolicy.Name = volume.Name
	newPolicy.Name = volume.Name
	volume.ExportPolicy = volume.Name

	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volume.Name).Return(oldPolicy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, newPolicy, roaring.BitmapOf(api.UpdateFlagExportRules)).Return(errFailed).Times(1)

	result := driver.unpublishNFSShare(ctx, volConfig, nodes, volume)

	assert.NotNil(t, result, "unpublish NFS share did not fail")
}

func TestUnpublishNFSShare_SetVolumeAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"10.10.10.0/24", "10.10.20.0/24"}

	volConfig, volume, nodes, oldPolicy, newPolicy := getStructsForUnpublish(ctx, driver)
	oldPolicy.Name = volume.Name
	newPolicy.Name = volume.Name
	volume.ExportPolicy = ""

	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volume.Name).Return(oldPolicy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, newPolicy, roaring.BitmapOf(api.UpdateFlagExportRules)).Return(nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagExportPolicy)).Return(errFailed).Times(1)

	result := driver.unpublishNFSShare(ctx, volConfig, nodes, volume)

	assert.NotNil(t, result, "unpublish NFS share did not fail")
}

func TestUnpublishNFSShare_BadCIDR(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"10.10.10.0/33", "10.10.20.0/24"}

	volConfig, volume, nodes, oldPolicy, newPolicy := getStructsForUnpublish(ctx, driver)
	oldPolicy.Name = volume.Name
	newPolicy.Name = volume.Name
	volume.ExportPolicy = ""

	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volume.Name).Return(oldPolicy, nil).Times(1)

	result := driver.unpublishNFSShare(ctx, volConfig, nodes, volume)

	assert.NotNil(t, result, "unpublish NFS share did not fail")
}

func TestUnpublishNFSShare_VolumeUpdateNeeded(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)
	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)
	driver.Config.AutoExportPolicy = true
	driver.Config.AutoExportCIDRs = []string{"10.10.10.0/24", "10.10.20.0/24"}

	volConfig, volume, nodes, oldPolicy, newPolicy := getStructsForUnpublish(ctx, driver)
	oldPolicy.Name = volume.Name
	newPolicy.Name = volume.Name
	volume.ExportPolicy = ""

	mockAPI.EXPECT().EnsureExportPolicyExists(ctx, volume.Name).Return(oldPolicy, nil).Times(1)
	mockAPI.EXPECT().SetExportPolicyAttributes(ctx, newPolicy, roaring.BitmapOf(api.UpdateFlagExportRules)).Return(nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagExportPolicy)).Return(nil).Times(1)

	result := driver.unpublishNFSShare(ctx, volConfig, nodes, volume)

	assert.Nil(t, result, "publish NFS share failed")
	assert.Equal(t, volume.Name, volume.ExportPolicy, "export policy mismatch")
}

func getStructsForCreateSnapshot(
	_ context.Context, _ *StorageDriver, snapTime metav1.Time,
) (*storage.VolumeConfig, *api.Volume, *storage.SnapshotConfig, *api.Snapshot, *api.Snapshot) {
	volConfig := &storage.VolumeConfig{
		Version:      "1",
		Name:         "testvol1",
		InternalName: "trident-testvol1",
		Size:         VolumeSizeStr,
	}

	labels := make(map[string]string)
	labels[storage.ProvisioningLabelTag] = ""

	volume := &api.Volume{
		Name:          "trident-testvol1",
		Namespace:     Namespace,
		ActualSize:    resource.MustParse("1G"),
		Created:       true,
		State:         api.VolumeStateOnline,
		ExportAddress: "1.1.1.1",
		VolumePath:    "trident-testvol1",
	}

	snapConfig := &storage.SnapshotConfig{
		Version:            "1",
		Name:               "snap1",
		InternalName:       "snap1",
		VolumeName:         "testvol1",
		VolumeInternalName: "trident-testvol1",
	}

	createRequest := &api.Snapshot{
		Name:       "snap1",
		Namespace:  Namespace,
		VolumeName: "trident-testvol1",
	}

	snapshot := &api.Snapshot{
		Name:            "snap1",
		Namespace:       Namespace,
		ResourceVersion: "1",
		Annotations:     map[string]string{},
		Labels:          map[string]string{},
		Finalizers:      []string{api.TridentSnapshotFinalizer},
		VolumeName:      "trident-testvol1",
		CreationTime:    &snapTime,
		VolumeUUID:      VolumeUUID,
		ReadyToUse:      true,
	}

	return volConfig, volume, snapConfig, snapshot, createRequest
}

func TestCanSnapshot(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, _, _ := getStructsForCreateSnapshot(ctx, driver, now)

	result := driver.CanSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "not nil")
}

func TestGetSnapshot(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, snapshot, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().Snapshot(ctx, snapConfig.InternalName).Return(snapshot, nil).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, resultErr, "get snapshot failed")
	assert.Equal(t, snapConfig, result.Config, "snapshot config mismatch")
	assert.Equal(t, int64(0), result.SizeBytes, "snapshot size mismatch")
	assert.Equal(t, storage.SnapshotStateOnline, result.State, "snapshot state mismatch")
	assert.Equal(t, now.UTC().Format(storage.SnapshotTimestampFormat), result.Created, "snapshot creation time mismatch")
}

func TestGetSnapshot_SnapshotNotReady(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, snapshot, _ := getStructsForCreateSnapshot(ctx, driver, now)
	snapshot.ReadyToUse = false

	mockAPI.EXPECT().Snapshot(ctx, snapConfig.InternalName).Return(snapshot, nil).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, resultErr, "get snapshot failed")
	assert.Equal(t, snapConfig, result.Config, "snapshot config mismatch")
	assert.Equal(t, int64(0), result.SizeBytes, "snapshot size mismatch")
	assert.Equal(t, storage.SnapshotStateCreating, result.State, "snapshot state mismatch")
	assert.Equal(t, now.UTC().Format(storage.SnapshotTimestampFormat), result.Created, "snapshot creation time mismatch")
}

func TestGetSnapshot_NonexistentSnapshot(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, _, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().Snapshot(ctx, snapConfig.InternalName).Return(nil, utils.NotFoundError("not found")).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "get snapshot failed")
	assert.Nil(t, resultErr, "get snapshot failed")
}

func TestGetSnapshot_GetSnapshotFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, _, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().Snapshot(ctx, snapConfig.InternalName).Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "get snapshot did not fail")
	assert.NotNil(t, resultErr, "get snapshot did not fail")
}

func TestGetSnapshot_WrongVolume(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, snapshot, _ := getStructsForCreateSnapshot(ctx, driver, now)
	snapshot.VolumeName += "xxx"

	mockAPI.EXPECT().Snapshot(ctx, snapConfig.InternalName).Return(snapshot, nil).Times(1)

	result, resultErr := driver.GetSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "get snapshot did not fail")
	assert.NotNil(t, resultErr, "get snapshot did not fail")
}

func getSnapshotsForList(snapTime metav1.Time) []*api.Snapshot {
	return []*api.Snapshot{
		{
			Name:            "snap1",
			Namespace:       Namespace,
			ResourceVersion: "1",
			Annotations:     map[string]string{},
			Labels:          map[string]string{},
			Finalizers:      []string{api.TridentSnapshotFinalizer},
			VolumeName:      "trident-testvol1",
			CreationTime:    &snapTime,
			VolumeUUID:      VolumeUUID,
			ReadyToUse:      true,
		},
		{
			Name:            "snap2",
			Namespace:       Namespace,
			ResourceVersion: "1",
			Annotations:     map[string]string{},
			Labels:          map[string]string{},
			Finalizers:      []string{api.TridentSnapshotFinalizer},
			VolumeName:      "trident-testvol1",
			CreationTime:    &snapTime,
			VolumeUUID:      VolumeUUID,
			ReadyToUse:      true,
		},
		{
			Name:            "snap3",
			Namespace:       Namespace,
			ResourceVersion: "1",
			Annotations:     map[string]string{},
			Labels:          map[string]string{},
			Finalizers:      []string{api.TridentSnapshotFinalizer},
			VolumeName:      "trident-testvol1",
			CreationTime:    &snapTime,
			VolumeUUID:      VolumeUUID,
			ReadyToUse:      false,
		},
	}
}

func TestGetSnapshots(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume, _, _, _ := getStructsForCreateSnapshot(ctx, driver, now)
	snapshots := getSnapshotsForList(now)

	mockAPI.EXPECT().Volume(ctx, volume.Name).Return(volume, nil).Times(1)
	mockAPI.EXPECT().Snapshots(ctx, volume).Return(snapshots, nil).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	expectedSnapshot0 := &storage.Snapshot{
		Config: &storage.SnapshotConfig{
			Version:            tridentconfig.OrchestratorAPIVersion,
			Name:               "snap1",
			InternalName:       "snap1",
			VolumeName:         "testvol1",
			VolumeInternalName: "trident-testvol1",
		},
		Created:   now.UTC().Format(storage.SnapshotTimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}

	expectedSnapshot1 := &storage.Snapshot{
		Config: &storage.SnapshotConfig{
			Version:            tridentconfig.OrchestratorAPIVersion,
			Name:               "snap2",
			InternalName:       "snap2",
			VolumeName:         "testvol1",
			VolumeInternalName: "trident-testvol1",
		},
		Created:   now.UTC().Format(storage.SnapshotTimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}

	assert.Nil(t, resultErr, "get snapshots failed")
	assert.Len(t, result, 2)

	assert.Equal(t, result[0], expectedSnapshot0, "snapshot 0 mismatch")
	assert.Equal(t, result[1], expectedSnapshot1, "snapshot 1 mismatch")
}

func TestGetSnapshots_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume, _, _, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().Volume(ctx, volume.Name).Return(nil, utils.NotFoundError("not found")).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	assert.NotNil(t, resultErr, "get snapshots did not fail")
	assert.Nil(t, result, "get snapshots did not fail")
}

func TestGetSnapshots_GetSnapshotsFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume, _, _, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().Volume(ctx, volume.Name).Return(volume, nil).Times(1)
	mockAPI.EXPECT().Snapshots(ctx, volume).Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetSnapshots(ctx, volConfig)

	assert.NotNil(t, resultErr, "get snapshots did not fail")
	assert.Nil(t, result, "get snapshots did not fail")
}

func TestCreateSnapshot(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume, snapConfig, snapshot, createRequest := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().VolumeExists(ctx, snapConfig.VolumeInternalName).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, createRequest).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotReady(ctx, snapshot, snapshotCreateTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SetSnapshotAttributes(ctx, snapshot, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(nil).Times(1)
	mockAPI.EXPECT().Snapshot(ctx, snapConfig.InternalName).Return(snapshot, nil).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	expectedSnapshot := &storage.Snapshot{
		Config:    snapConfig,
		Created:   now.UTC().Format(storage.SnapshotTimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}

	assert.Nil(t, resultErr, "create snapshot failed")
	assert.Equal(t, expectedSnapshot, result, "snapshot mismatch")
}

func TestCreateSnapshot_VolumeExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, _, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().VolumeExists(ctx, snapConfig.VolumeInternalName).Return(false, nil, errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, resultErr, "create snapshot did not fail")
	assert.Nil(t, result, "create snapshot did not fail")
}

func TestCreateSnapshot_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, _, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().VolumeExists(ctx, snapConfig.VolumeInternalName).Return(false, nil, nil).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, resultErr, "create snapshot did not fail")
	assert.Nil(t, result, "create snapshot did not fail")
}

func TestCreateSnapshot_SnapshotExistsCheckFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume, snapConfig, _, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().VolumeExists(ctx, snapConfig.VolumeInternalName).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(false, nil, errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, resultErr, "create snapshot did not fail")
	assert.Nil(t, result, "create snapshot did not fail")
}

func TestCreateSnapshot_SnapshotExists(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume, snapConfig, snapshot, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().VolumeExists(ctx, snapConfig.VolumeInternalName).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(true, snapshot, nil).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, resultErr, "create snapshot did not fail")
	assert.Nil(t, result, "create snapshot did not fail")
}

func TestCreateSnapshot_SnapshotCreateFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume, snapConfig, _, createRequest := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().VolumeExists(ctx, snapConfig.VolumeInternalName).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, createRequest).Return(nil, errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, resultErr, "create snapshot did not fail")
	assert.Nil(t, result, "create snapshot did not fail")
}

func TestCreateSnapshot_SnapshotWaitFailed_CleanupOK(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume, snapConfig, snapshot, createRequest := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().VolumeExists(ctx, snapConfig.VolumeInternalName).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, createRequest).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotReady(ctx, snapshot, snapshotCreateTimeout).Return(errFailed).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, snapshot).Return(nil).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, resultErr, "create snapshot did not fail")
	assert.Nil(t, result, "create snapshot did not fail")
}

func TestCreateSnapshot_SnapshotWaitFailed_CleanupFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume, snapConfig, snapshot, createRequest := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().VolumeExists(ctx, snapConfig.VolumeInternalName).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, createRequest).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotReady(ctx, snapshot, snapshotCreateTimeout).Return(errFailed).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, snapshot).Return(errFailed).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, resultErr, "create snapshot did not fail")
	assert.Nil(t, result, "create snapshot did not fail")
}

func TestCreateSnapshot_SetAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume, snapConfig, snapshot, createRequest := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().VolumeExists(ctx, snapConfig.VolumeInternalName).Return(true, volume, nil).Times(1)
	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(false, nil, nil).Times(1)
	mockAPI.EXPECT().CreateSnapshot(ctx, createRequest).Return(snapshot, nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotReady(ctx, snapshot, snapshotCreateTimeout).Return(nil).Times(1)
	mockAPI.EXPECT().SetSnapshotAttributes(ctx, snapshot, roaring.BitmapOf(api.UpdateFlagAddFinalizer)).Return(errFailed).Times(1)
	mockAPI.EXPECT().Snapshot(ctx, snapConfig.InternalName).Return(snapshot, nil).Times(1)

	result, resultErr := driver.CreateSnapshot(ctx, snapConfig, volConfig)

	expectedSnapshot := &storage.Snapshot{
		Config:    snapConfig,
		Created:   now.UTC().Format(storage.SnapshotTimestampFormat),
		SizeBytes: 0,
		State:     storage.SnapshotStateOnline,
	}

	assert.Nil(t, resultErr, "create snapshot failed")
	assert.Equal(t, expectedSnapshot, result, "snapshot mismatch")
}

func TestRestoreSnapshot(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, _, _ := getStructsForCreateSnapshot(ctx, driver, now)

	result := driver.RestoreSnapshot(ctx, snapConfig, volConfig)

	assert.Error(t, result, "expected error")
	assert.IsType(t, utils.UnsupportedError(""), result, "not UnsupportedError")
}

func TestDeleteSnapshot(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, snapshot, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(true, snapshot, nil).Times(1)
	mockAPI.EXPECT().SetSnapshotAttributes(ctx, snapshot, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, snapshot).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotDeleted(ctx, snapshot, driver.defaultDeleteTimeout()).Return(nil).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "snapshot delete failed")
}

func TestDeleteSnapshot_NonexistentSnapshot(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, _, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(false, nil, nil).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.Nil(t, result, "snapshot delete failed")
}

func TestDeleteSnapshot_GetSnapshotFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, _, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(false, nil, errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "snapshot delete did not fail")
}

func TestDeleteSnapshot_SetAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, snapshot, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(true, snapshot, nil).Times(1)
	mockAPI.EXPECT().SetSnapshotAttributes(ctx, snapshot, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "snapshot delete did not fail")
}

func TestDeleteSnapshot_AlreadyDeleting(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, snapshot, _ := getStructsForCreateSnapshot(ctx, driver, now)
	snapshot.DeletionTimestamp = &now

	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(true, snapshot, nil).Times(1)
	mockAPI.EXPECT().SetSnapshotAttributes(ctx, snapshot, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "snapshot delete did not fail")
}

func TestDeleteSnapshot_DeleteFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, snapshot, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(true, snapshot, nil).Times(1)
	mockAPI.EXPECT().SetSnapshotAttributes(ctx, snapshot, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, snapshot).Return(errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "snapshot delete did not fail")
}

func TestDeleteSnapshot_SnapshotWaitFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, snapConfig, snapshot, _ := getStructsForCreateSnapshot(ctx, driver, now)

	mockAPI.EXPECT().SnapshotExists(ctx, snapConfig.InternalName).Return(true, snapshot, nil).Times(1)
	mockAPI.EXPECT().SetSnapshotAttributes(ctx, snapshot, roaring.BitmapOf(api.UpdateFlagRemoveFinalizer)).Return(nil).Times(1)
	mockAPI.EXPECT().DeleteSnapshot(ctx, snapshot).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForSnapshotDeleted(ctx, snapshot, driver.defaultDeleteTimeout()).Return(errFailed).Times(1)

	result := driver.DeleteSnapshot(ctx, snapConfig, volConfig)

	assert.NotNil(t, result, "snapshot delete did not fail")
}

func getVolumesForList() []*api.Volume {
	return []*api.Volume{
		{
			Name:          "myPrefix-testvol1",
			Namespace:     Namespace,
			ActualSize:    resource.MustParse("1G"),
			Created:       true,
			State:         api.VolumeStateOnline,
			ExportAddress: "1.1.1.1",
		},
		{
			Name:          "myPrefix-testvol2",
			Namespace:     Namespace,
			ActualSize:    resource.MustParse("1G"),
			Created:       true,
			State:         api.VolumeStateOnline,
			ExportAddress: "1.1.1.1",
		},
		{
			Name:          "myPrefix-testvol3",
			Namespace:     Namespace,
			ActualSize:    resource.MustParse("1G"),
			Created:       false,
			State:         api.VolumeStateOnline,
			ExportAddress: "1.1.1.1",
		},
		{
			Name:          "myPrefix-testvol4",
			Namespace:     Namespace,
			ActualSize:    resource.MustParse("1G"),
			Created:       true,
			State:         "",
			ExportAddress: "1.1.1.1",
		},
		{
			Name:          "myPrefix-testvol5",
			Namespace:     Namespace,
			ActualSize:    resource.MustParse("1G"),
			Created:       true,
			State:         api.VolumeStateOnline,
			ExportAddress: "",
		},
		{
			Name:          "myPrefix-testvol6",
			Namespace:     Namespace,
			ActualSize:    resource.MustParse("1G"),
			Created:       true,
			State:         api.VolumeStateOnline,
			ExportAddress: "1.1.1.1",
			CreateError:   api.VolumeCreateError("failed"),
		},
		{
			Name:          "myPrefix-testvol7",
			Namespace:     Namespace,
			ActualSize:    resource.MustParse("1G"),
			Created:       true,
			State:         api.VolumeStateOnline,
			ExportAddress: "1.1.1.1",
			OnlineError:   api.VolumeOnlineError("failed"),
		},
		{
			Name:              "myPrefix-testvol8",
			Namespace:         Namespace,
			ActualSize:        resource.MustParse("1G"),
			Created:           true,
			State:             api.VolumeStateOnline,
			ExportAddress:     "1.1.1.1",
			DeletionTimestamp: &now,
		},
		{
			Name:          "testvol8",
			Namespace:     Namespace,
			ActualSize:    resource.MustParse("1G"),
			Created:       true,
			State:         api.VolumeStateOnline,
			ExportAddress: "1.1.1.1",
		},
	}
}

func TestList(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volumes := getVolumesForList()

	mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

	list, result := driver.List(ctx)

	assert.Nil(t, result, "list failed")
	assert.Equal(t, list, []string{"testvol1", "testvol2"}, "list mismatch")
}

func TestList_ListFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	mockAPI.EXPECT().Volumes(ctx).Return(nil, errFailed).Times(1)

	list, result := driver.List(ctx)

	assert.Error(t, result, "list did not fail")
	assert.Nil(t, list, "list not nil")
}

func TestList_ListNone(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	mockAPI.EXPECT().Volumes(ctx).Return([]*api.Volume{}, nil).Times(1)

	list, result := driver.List(ctx)

	assert.Nil(t, result, "expected nil")
	assert.Equal(t, []string{}, list, "list not empty")
}

func TestGet(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volume := &api.Volume{}

	mockAPI.EXPECT().Volume(ctx, "volume1").Return(volume, nil).Times(1)

	result := driver.Get(ctx, "volume1")

	assert.NoError(t, result, "expected no error")
}

func TestGet_NotFound(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	mockAPI.EXPECT().Volume(ctx, "volume1").Return(nil, utils.NotFoundError("not found")).Times(1)

	result := driver.Get(ctx, "volume1")

	assert.Error(t, result, "get did not fail")
	assert.IsType(t, utils.NotFoundError(""), result, "not NotFoundError")
}

func TestResize(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume := getStructsForDestroy(ctx, driver)
	requestedNewSize := uint64(VolumeSizeI64 * 2)
	paddedNewSize := uint64(226050910315)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(volume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagRequestedSize)).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeResize(ctx, volume, int64(paddedNewSize), volumeResizeTimeout).Return(nil).Times(1)

	result := driver.Resize(ctx, volConfig, requestedNewSize)

	assert.Nil(t, result, "resize failed")
	assert.Equal(t, strconv.FormatUint(requestedNewSize, 10), volConfig.Size, "size mismatch")
}

func TestResize_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _ := getStructsForDestroy(ctx, driver)
	requestedNewSize := uint64(VolumeSizeI64 * 2)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(nil, errFailed).Times(1)

	result := driver.Resize(ctx, volConfig, requestedNewSize)

	assert.NotNil(t, result, "resize did not fail")
}

func TestResize_VolumeNotReady(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume := getStructsForDestroy(ctx, driver)
	volume.Created = false
	requestedNewSize := uint64(VolumeSizeI64 * 2)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(volume, nil).Times(1)

	result := driver.Resize(ctx, volConfig, requestedNewSize)

	assert.NotNil(t, result, "resize did not fail")
}

func TestResize_AboveMaximumSize(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume := getStructsForDestroy(ctx, driver)
	driver.Config.LimitVolumeSize = strconv.FormatInt(VolumeSizeI64+1, 10)
	requestedNewSize := uint64(VolumeSizeI64 * 2)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(volume, nil).Times(1)

	result := driver.Resize(ctx, volConfig, requestedNewSize)

	assert.NotNil(t, result, "resize did not fail")
	assert.Equal(t, VolumeSizeStr, volConfig.Size, "size mismatch")
}

func TestResize_ShrinkingVolume(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume := getStructsForDestroy(ctx, driver)
	requestedNewSize := uint64(VolumeSizeI64 / 2)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(volume, nil).Times(1)

	result := driver.Resize(ctx, volConfig, requestedNewSize)

	assert.NotNil(t, result, "resize did not fail")
	assert.Equal(t, VolumeSizeStr, volConfig.Size, "size mismatch")
}

func TestResize_NoSizeChange(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume := getStructsForDestroy(ctx, driver)
	requestedNewSize := uint64(VolumeSizeI64)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(volume, nil).Times(1)

	result := driver.Resize(ctx, volConfig, requestedNewSize)

	assert.Nil(t, result, "resize failed")
	assert.Equal(t, VolumeSizeStr, volConfig.Size, "size mismatch")
}

func TestResize_SetAttributesFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume := getStructsForDestroy(ctx, driver)
	requestedNewSize := uint64(VolumeSizeI64 * 2)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(volume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagRequestedSize)).Return(errFailed).Times(1)

	result := driver.Resize(ctx, volConfig, requestedNewSize)

	assert.NotNil(t, result, "resize did not fail")
	assert.Equal(t, VolumeSizeStr, volConfig.Size, "size mismatch")
}

func TestResize_VolumeWaitFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume := getStructsForDestroy(ctx, driver)
	requestedNewSize := uint64(VolumeSizeI64 * 2)
	paddedNewSize := uint64(226050910315)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(volume, nil).Times(1)
	mockAPI.EXPECT().SetVolumeAttributes(ctx, volume, roaring.BitmapOf(api.UpdateFlagRequestedSize)).Return(nil).Times(1)
	mockAPI.EXPECT().WaitForVolumeResize(ctx, volume, int64(paddedNewSize), volumeResizeTimeout).Return(errFailed).Times(1)

	result := driver.Resize(ctx, volConfig, requestedNewSize)

	assert.NotNil(t, result, "resize did not fail")
	assert.Equal(t, VolumeSizeStr, volConfig.Size, "size mismatch")
}

func TestGetStorageBackendSpecs(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	driver.populateConfigurationDefaults(ctx, &driver.Config)
	driver.initializeStoragePools(ctx)

	backend := &storage.StorageBackend{}
	backend.SetStorage(make(map[string]storage.Pool))

	result := driver.GetStorageBackendSpecs(ctx, backend)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, "myADSBackend", backend.Name(), "backend name mismatch")
	for _, pool := range driver.pools {
		assert.Equal(t, backend, pool.Backend(), "pool-backend mismatch")
		assert.Equal(t, pool, backend.Storage()["myADSBackend_pool"], "backend-pool mismatch")
	}
}

func TestCreatePrepare(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	tridentconfig.UsingPassthroughStore = true
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volConfig := &storage.VolumeConfig{Name: "testvol1"}

	driver.CreatePrepare(ctx, volConfig)

	assert.Equal(t, "myPrefix-testvol1", volConfig.InternalName)
}

func TestGetStorageBackendPhysicalPoolNames(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	result := driver.GetStorageBackendPhysicalPoolNames(ctx)

	assert.Equal(t, []string{}, result, "physical pool names mismatch")
}

func TestGetInternalVolumeName_PassthroughStore(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	tridentconfig.UsingPassthroughStore = true
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	result := driver.GetInternalVolumeName(ctx, "testvol1")

	assert.Equal(t, "myPrefix-testvol1", result, "internal name mismatch")
}

func TestGetInternalVolumeName_CSI(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	tridentconfig.UsingPassthroughStore = false
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	result := driver.GetInternalVolumeName(ctx, "pvc-5e522901-b891-41d8-9e83-5496d2e62e71")

	assert.Equal(t, "pvc-5e522901-b891-41d8-9e83-5496d2e62e71", result, "internal name mismatch")
}

func TestGetInternalVolumeName_NonCSIWithPrefix(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	tridentconfig.UsingPassthroughStore = false
	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	result := driver.GetInternalVolumeName(ctx, "testvol1")

	assert.Equal(t, "myPrefix-testvol1", result, "internal name mismatch")
}

func TestGetInternalVolumeName_NonCSIEmptyPrefix(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	tridentconfig.UsingPassthroughStore = false
	storagePrefix := ""
	driver.Config.StoragePrefix = &storagePrefix

	result := driver.GetInternalVolumeName(ctx, "testvol1")

	assert.Equal(t, "testvol1", result, "internal name mismatch")
}

func TestGetInternalVolumeName_NonCSINoPrefix(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	tridentconfig.UsingPassthroughStore = false
	driver.Config.StoragePrefix = nil

	result := driver.GetInternalVolumeName(ctx, "testvol1")

	assert.Equal(t, "trident-testvol1", result, "internal name mismatch")
}

func TestCreateFollowup(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume, _, _ := getStructsForPublish(ctx, driver)

	mockAPI.EXPECT().Volume(ctx, volume.Name).Return(volume, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.Nil(t, result, "not nil")
	assert.Equal(t, volume.ExportAddress, volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "/"+volume.VolumePath, volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "nfs", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_NonexistentVolume(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, _, _, _ := getStructsForPublish(ctx, driver)

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(nil, utils.NotFoundError("not found")).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, "", volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", volConfig.FileSystem, "filesystem type mismatch")
}

func TestCreateFollowup_VolumeNotReady(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	volConfig, volume, _, _ := getStructsForPublish(ctx, driver)
	volume.Created = false

	mockAPI.EXPECT().Volume(ctx, volConfig.InternalName).Return(volume, nil).Times(1)

	result := driver.CreateFollowup(ctx, volConfig)

	assert.NotNil(t, result, "expected error")
	assert.Equal(t, "", volConfig.AccessInfo.NfsServerIP, "NFS server IP mismatch")
	assert.Equal(t, "", volConfig.AccessInfo.NfsPath, "NFS path mismatch")
	assert.Equal(t, "", volConfig.FileSystem, "filesystem type mismatch")
}

func TestGetProtocol(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	result := driver.GetProtocol(ctx)

	assert.Equal(t, tridentconfig.File, result)
}

func TestStoreConfig(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	persistentConfig := &storage.PersistentStorageBackendConfig{}

	driver.StoreConfig(ctx, persistentConfig)

	assert.Equal(t, json.RawMessage("{}"), driver.Config.CommonStorageDriverConfig.StoragePrefixRaw, "raw prefix mismatch")
	assert.Equal(t, driver.Config, *persistentConfig.AstraDSConfig, "azure config mismatch")
}

func TestGetExternalConfig(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	result := driver.GetExternalConfig(ctx)

	assert.IsType(t, drivers.AstraDSStorageDriverConfig{}, result, "config type mismatch")
	assert.Equal(t, driver.Config.CommonStorageDriverConfig,
		result.(drivers.AstraDSStorageDriverConfig).CommonStorageDriverConfig, "config mismatch")
}

func TestGetVolumeExternal(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volume := &api.Volume{
		Name:         "myPrefix-testvol1",
		Namespace:    Namespace,
		ActualSize:   resource.MustParse("1G"),
		ExportPolicy: ExportPolicy,
		NoSnapDir:    false,
		Permissions:  "0777",
	}

	mockAPI.EXPECT().Volume(ctx, "testvol1").Return(volume, nil).Times(1)

	result, resultErr := driver.GetVolumeExternal(ctx, "testvol1")

	assert.Nil(t, resultErr, "not nil")
	assert.IsType(t, &storage.VolumeExternal{}, result, "type mismatch")
	assert.Equal(t, "1", result.Config.Version)
	assert.Equal(t, "testvol1", result.Config.Name)
	assert.Equal(t, "myPrefix-testvol1", result.Config.InternalName)
	assert.Equal(t, "1073741824", result.Config.Size)
	assert.Equal(t, ExportPolicy, result.Config.ExportPolicy)
	assert.Equal(t, "true", result.Config.SnapshotDir)
	assert.Equal(t, "0777", result.Config.UnixPermissions)
}

func TestGetVolumeExternal_GetFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	mockAPI.EXPECT().Volume(ctx, "testvol1").Return(nil, errFailed).Times(1)

	result, resultErr := driver.GetVolumeExternal(ctx, "testvol1")

	assert.Nil(t, result, "not nil")
	assert.NotNil(t, resultErr, "error expected")
}

func TestGetVolumeExternalWrappers(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volumes := getVolumesForList()
	channel := make(chan *storage.VolumeExternalWrapper, len(volumes))

	mockAPI.EXPECT().Volumes(ctx).Return(volumes, nil).Times(1)

	driver.GetVolumeExternalWrappers(ctx, channel)

	// Read the volumes from the channel
	externalVolumes := make([]*storage.VolumeExternal, 0)
	for wrapper := range channel {
		if wrapper.Error != nil {
			t.FailNow()
		} else {
			externalVolumes = append(externalVolumes, wrapper.Volume)
		}
	}

	assert.Len(t, externalVolumes, 2, "wrong number of volumes")
}

func TestGetVolumeExternalWrappers_ListFailed(t *testing.T) {
	mockAPI, driver := newMockAstraDSDriver(t)

	storagePrefix := "myPrefix-"
	driver.Config.StoragePrefix = &storagePrefix

	volumes := getVolumesForList()
	channel := make(chan *storage.VolumeExternalWrapper, len(volumes))

	mockAPI.EXPECT().Volumes(ctx).Return(nil, errFailed).Times(1)

	driver.GetVolumeExternalWrappers(ctx, channel)

	// Read the volumes from the channel
	var result error
	for wrapper := range channel {
		if wrapper.Error != nil {
			result = wrapper.Error
			break
		}
	}

	assert.NotNil(t, result, "expected error")
}

func TestStringAndGoString(t *testing.T) {
	_, driver := newMockAstraDSDriver(t)

	stringFunc := func(d *StorageDriver) string { return d.String() }
	goStringFunc := func(d *StorageDriver) string { return d.GoString() }

	for _, toString := range []func(*StorageDriver) string{stringFunc, goStringFunc} {

		result := toString(driver)

		assert.Contains(t, result, "<REDACTED>", "ADS driver does not contain <REDACTED>")
		assert.Contains(t, result, "API:<REDACTED>", "ADS driver does not redact API information")
		assert.Contains(t, result, "Kubeconfig:<REDACTED>", "ADS driver does not redact Kubeconfig")
		assert.NotContains(t, result, KubeconfigB64, "ADS driver contains Kubeconfig")
	}
}

func TestGetUpdateType_NoFlaggedChanges(t *testing.T) {
	_, oldDriver := newMockAstraDSDriver(t)
	oldDriver.volumeCreateTimeout = 1 * time.Second

	_, newDriver := newMockAstraDSDriver(t)
	newDriver.volumeCreateTimeout = 2 * time.Second

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestGetUpdateType_WrongDriverType(t *testing.T) {
	oldDriver := &fake.StorageDriver{
		Config:             drivers.FakeStorageDriverConfig{},
		Volumes:            make(map[string]storagefake.Volume),
		DestroyedVolumes:   make(map[string]bool),
		Snapshots:          make(map[string]map[string]*storage.Snapshot),
		DestroyedSnapshots: make(map[string]bool),
		Secret:             "secret",
	}

	_, newDriver := newMockAstraDSDriver(t)
	newDriver.volumeCreateTimeout = 2 * time.Second

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.InvalidUpdate)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestGetUpdateType_OtherChanges(t *testing.T) {
	_, oldDriver := newMockAstraDSDriver(t)
	prefix1 := "prefix1-"
	commonConfig1 := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "astrads-nas",
		BackendName:       BackendName,
		StoragePrefix:     &prefix1,
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}
	oldDriver.Config.CommonStorageDriverConfig = commonConfig1
	oldDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret1",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	_, newDriver := newMockAstraDSDriver(t)
	prefix2 := "prefix2-"
	commonConfig2 := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "astrads-nas",
		BackendName:       BackendName,
		StoragePrefix:     &prefix2,
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}
	newDriver.Config.CommonStorageDriverConfig = commonConfig2
	newDriver.Config.Credentials = map[string]string{
		drivers.KeyName: "secret2",
		drivers.KeyType: string(drivers.CredentialStoreK8sSecret),
	}

	result := newDriver.GetUpdateType(ctx, oldDriver)

	expectedBitmap := &roaring.Bitmap{}
	expectedBitmap.Add(storage.PrefixChange)
	expectedBitmap.Add(storage.CredentialsChange)

	assert.Equal(t, expectedBitmap, result, "bitmap mismatch")
}

func TestReconcileNodeAccess(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAstraDS(mockCtrl)

	driver := *newTestAstraDSDriver(mockAPI)

	result := driver.ReconcileNodeAccess(ctx, nil, "")

	assert.Nil(t, result, "not nil")
}

func TestValidateStoragePrefix(t *testing.T) {
	tests := []struct {
		Name          string
		StoragePrefix string
		Valid         bool
	}{
		// Invalid storage prefixes
		{
			Name:          "storage prefix starts with plus",
			StoragePrefix: "+abcd-ABC",
		},
		{
			Name:          "storage prefix starts with digit",
			StoragePrefix: "1abcd-ABC",
		},
		{
			Name:          "storage prefix starts with underscore",
			StoragePrefix: "_abcd-ABC",
		},
		{
			Name:          "storage prefix contains underscore",
			StoragePrefix: "ABCD_abc",
		},
		{
			Name:          "storage prefix has plus",
			StoragePrefix: "abcd+ABC",
		},
		{
			Name:          "storage prefix is single digit",
			StoragePrefix: "1",
		},
		{
			Name:          "storage prefix is single underscore",
			StoragePrefix: "_",
		},
		{
			Name:          "storage prefix is single colon",
			StoragePrefix: ":",
		},
		{
			Name:          "storage prefix is single dash",
			StoragePrefix: "-",
		},
		// Valid storage prefixes
		{
			Name:          "storage prefix is single letter",
			StoragePrefix: "a",
			Valid:         true,
		},
		{
			Name:          "storage prefix contains digits",
			StoragePrefix: "abcd-123-ABC",
			Valid:         true,
		},
		{
			Name:          "storage prefix has only letters and dash",
			StoragePrefix: "abcd-efgh",
			Valid:         true,
		},
		{
			Name:          "storage prefix ends with dash",
			StoragePrefix: "abcd-efgh-",
			Valid:         true,
		},
		{
			Name:          "storage prefix has capital letters",
			StoragePrefix: "ABCD",
			Valid:         true,
		},
		{
			Name:          "storage prefix has letters and capital letters",
			StoragePrefix: "abcd-EFGH",
			Valid:         true,
		},
		{
			Name:          "storage prefix is empty",
			StoragePrefix: "",
			Valid:         true,
		},
	}
	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			err := validateStoragePrefix(test.StoragePrefix)
			if test.Valid {
				assert.NoError(t, err, "should be valid")
			} else {
				assert.Error(t, err, "should be invalid")
			}
		})
	}
}

func TestGetCommonConfig(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockAstraDS(mockCtrl)

	driver := *newTestAstraDSDriver(mockAPI)
	driver.Config.BackendName = "ads"
	driver.Config.QosPolicy = "platinum"

	result := driver.GetCommonConfig(ctx)

	assert.Equal(t, driver.Config.CommonStorageDriverConfig, result, "common config mismatch")
}
