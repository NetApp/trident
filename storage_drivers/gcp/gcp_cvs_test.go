// Copyright 2021 NetApp, Inc. All Rights Reserved.

package gcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	mockGCPClient "github.com/netapp/trident/mocks/mock_storage_drivers/mock_gcp"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/storage_drivers/gcp/api"
	"github.com/netapp/trident/utils"
	"github.com/netapp/trident/utils/errors"
	versionutils "github.com/netapp/trident/utils/version"
)

const (
	ProjectNumber     = "123456"
	PrivateKeyId      = "12345678987654321"
	PrivateKey        = "-----BEGIN PRIVATE KEY-----AAAABBBCCCCDDDDDDEEEEEEFFFF----END PRIVATE KEY-----"
	ClientEmail       = "random@random.com"
	ClientID          = "98765432123456789"
	ClientX509CertURL = "https://random.com/x509Cert"
)

var (
	ctx             = context.Background
	debugTraceFlags = map[string]bool{"method": true, "api": true, "discovery": true}
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	InitLogOutput(io.Discard)
	os.Exit(m.Run())
}

func newTestGCPDriver(client api.GCPClient) *NFSStorageDriver {
	config := &drivers.GCPNFSStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

	APIKey := drivers.GCPPrivateKey{
		Type:                    "random_account",
		ProjectID:               "random_project",
		PrivateKeyID:            PrivateKeyId,
		PrivateKey:              PrivateKey,
		ClientEmail:             ClientEmail,
		ClientID:                ClientID,
		AuthURI:                 "https://random.com/o/oauth2/auth",
		TokenURI:                "https://random.com/token",
		AuthProviderX509CertURL: "https://random.com/certs",
		ClientX509CertURL:       ClientX509CertURL,
	}

	config.ProjectNumber = ProjectNumber
	config.APIKey = APIKey
	config.Region = "us-central-10"
	config.ProxyURL = "https://random.com"
	config.NfsMountOptions = "nfsvers=3"
	config.VolumeCreateTimeout = "30"
	config.StorageDriverName = "gcp-cvs"
	config.StoragePrefix = sp("test_")
	config.StoragePools = []string{"pool1"}
	config.DebugTraceFlags[discovery] = true
	config.APIRegion = "us-central-10"

	API := api.NewDriver(api.ClientConfig{
		ProjectNumber:   config.ProjectNumber,
		APIKey:          config.APIKey,
		APIRegion:       config.APIRegion,
		ProxyURL:        config.ProxyURL,
		DebugTraceFlags: config.DebugTraceFlags,
	})

	GCPDriver := &NFSStorageDriver{}
	GCPDriver.Config = *config
	GCPDriver.apiVersion = versionutils.MustParseSemantic("20.5.1")
	GCPDriver.sdeVersion = versionutils.MustParseSemantic("20.6.2")
	GCPDriver.tokenRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,79}$`)
	GCPDriver.csiRegexp = regexp.MustCompile(`^pvc-[0-9a-fA-F]{12}$`)
	GCPDriver.apiRegions = []string{"us-central-9", "us-central-10"}

	telemetry := tridentconfig.OrchestratorTelemetry
	telemetry.TridentBackendUUID = "backendUUID"
	GCPDriver.telemetry = &Telemetry{
		Telemetry: telemetry,
		Plugin:    tridentconfig.GCPNFSStorageDriverName,
	}

	if client != nil {
		GCPDriver.API = client
	} else {
		GCPDriver.API = API
	}

	_, vPool := getCreateVolumeStructs()
	GCPDriver.pools = map[string]storage.Pool{vPool.Name(): vPool}

	return GCPDriver
}

func newMockGCPDriver(t *testing.T) (*mockGCPClient.MockGCPClient, *NFSStorageDriver) {
	mockCtrl := gomock.NewController(t)
	gcpClient := mockGCPClient.NewMockGCPClient(mockCtrl)

	return gcpClient, newTestGCPDriver(gcpClient)
}

func callString(s NFSStorageDriver) string {
	return s.String()
}

func callGoString(s NFSStorageDriver) string {
	return s.GoString()
}

func getDummyGCPPools() *[]*api.Pool {
	pool1 := &api.Pool{
		PoolID:          "abc1",
		Name:            "pool1",
		NumberOfVolumes: 2,
		ServiceLevel:    api.PoolServiceLevel1,
		SizeInBytes:     10000000000,
		AllocatedBytes:  500,
	}
	pool2 := &api.Pool{
		PoolID:          "abc2",
		Name:            "pool2",
		NumberOfVolumes: 1,
		ServiceLevel:    api.PoolServiceLevel2,
		SizeInBytes:     1000,
		AllocatedBytes:  900,
	}
	pool3 := &api.Pool{
		PoolID:          "abc3",
		Name:            "pool3",
		NumberOfVolumes: 50,
		ServiceLevel:    api.PoolServiceLevel1,
		SizeInBytes:     1000,
		AllocatedBytes:  500,
	}
	pool4 := &api.Pool{
		PoolID:          "abc4",
		Name:            "pool4",
		NumberOfVolumes: 10,
		ServiceLevel:    api.PoolServiceLevel1,
		SizeInBytes:     1000,
		AllocatedBytes:  500,
	}

	pools := []*api.Pool{pool1, pool2, pool3, pool4}
	return &pools
}

func TestGCPStorageDriverConfigString(t *testing.T) {
	GCPDrivers := []NFSStorageDriver{
		*newTestGCPDriver(nil),
	}

	for _, toString := range []func(NFSStorageDriver) string{callString, callGoString} {
		for _, GCPDriver := range GCPDrivers {
			assert.Contains(t, toString(GCPDriver), "<REDACTED>",
				"GCP driver does not contain <REDACTED>")
			assert.Contains(t, toString(GCPDriver), "API:<REDACTED>",
				"GCP driver does not redact API information")
			assert.Contains(t, toString(GCPDriver), "ProjectNumber:<REDACTED>",
				"GCP driver does not redact Project Number")
			assert.Contains(t, toString(GCPDriver), "APIKey:<REDACTED>",
				"GCP driver does not redact APIKey")
			assert.NotContains(t, toString(GCPDriver), ProjectNumber,
				"GCP driver contains project number")
			assert.NotContains(t, toString(GCPDriver), PrivateKeyId,
				"GCP driver contains Private Key Id")
			assert.NotContains(t, toString(GCPDriver), PrivateKey,
				"GCP driver contains Private Key")
			assert.NotContains(t, toString(GCPDriver), ClientEmail,
				"GCP driver contains Client Email")
			assert.NotContains(t, toString(GCPDriver), ClientID,
				"GCP driver contains Client ID")
			assert.NotContains(t, toString(GCPDriver), ClientX509CertURL,
				"GCP driver contains Client X509 Cert URL")
		}
	}
}

func TestMakeNetworkPath(t *testing.T) {
	driver := newTestGCPDriver(nil)

	// Without shared VPC host project
	driver.Config = drivers.GCPNFSStorageDriverConfig{
		ProjectNumber: "737253775480",
	}
	assert.Equal(t, "projects/737253775480/global/networks/myNetwork", driver.makeNetworkPath("myNetwork"))

	// With shared VPC host project
	driver.Config = drivers.GCPNFSStorageDriverConfig{
		ProjectNumber:     "737253775480",
		HostProjectNumber: "527303026223",
	}
	assert.Equal(t, "projects/527303026223/global/networks/myNetwork", driver.makeNetworkPath("myNetwork"))
}

func TestApplyMinimumVolumeSizeHW(t *testing.T) {
	tests := []struct {
		Name               string
		RequestedSizeBytes uint64
		SizeBytes          uint64
	}{
		{
			Name:               "size 1",
			RequestedSizeBytes: 1,
			SizeBytes:          MinimumCVSVolumeSizeBytesHW,
		},
		{
			Name:               "size just below minimum",
			RequestedSizeBytes: MinimumCVSVolumeSizeBytesHW - 1,
			SizeBytes:          MinimumCVSVolumeSizeBytesHW,
		},
		{
			Name:               "size at minimum",
			RequestedSizeBytes: MinimumCVSVolumeSizeBytesHW,
			SizeBytes:          MinimumCVSVolumeSizeBytesHW,
		},
		{
			Name:               "size just above minimum",
			RequestedSizeBytes: MinimumCVSVolumeSizeBytesHW + 1,
			SizeBytes:          MinimumCVSVolumeSizeBytesHW + 1,
		},
		{
			Name:               "size far above minimum",
			RequestedSizeBytes: MinimumCVSVolumeSizeBytesHW * 100,
			SizeBytes:          MinimumCVSVolumeSizeBytesHW * 100,
		},
	}

	d := NFSStorageDriver{}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			sizeBytes := d.applyMinimumVolumeSizeHW(test.RequestedSizeBytes)
			assert.Equal(t, test.SizeBytes, sizeBytes, "incorrect size")
		})
	}
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
			StoragePrefix: "+abcd_123_ABC",
		},
		{
			Name:          "storage prefix starts with digit",
			StoragePrefix: "1abcd_123_ABC",
		},
		{
			Name:          "storage prefix starts with underscore",
			StoragePrefix: "_abcd_123_ABC",
		},
		{
			Name:          "storage prefix ends capitalized",
			StoragePrefix: "abcd_123_ABC",
		},
		{
			Name:          "storage prefix starts capitalized",
			StoragePrefix: "ABCD_123_abc",
		},
		{
			Name:          "storage prefix has plus",
			StoragePrefix: "abcd+123_ABC",
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
		{
			Name:          "storage prefix is empty",
			StoragePrefix: "",
		},
		// Valid storage prefixes
		{
			Name:          "storage prefix has dash",
			StoragePrefix: "abcd-123",
			Valid:         true,
		},
		{
			Name:          "storage prefix is single letter",
			StoragePrefix: "a",
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

func TestInitializeStoragePoolsLabels(t *testing.T) {
	ctx := context.Background()
	d := newTestGCPDriver(nil)

	cases := []struct {
		Name                 string
		physicalPoolLabels   map[string]string
		virtualPoolLabels    map[string]string
		physicalExpected     string
		virtualExpected      string
		backendName          string
		physicalErrorMessage string
		virtualErrorMessage  string
	}{
		{
			"no labels",
			nil, nil, "", "", "gcp-cvs",
			"Label is not empty", "Label is not empty",
		},
		{
			"base label only",
			map[string]string{"base-key": "base-value"},
			nil,
			`{"provisioning":{"base-key":"base-value"}}`,
			`{"provisioning":{"base-key":"base-value"}}`, "gcp-cvs",
			"Base label is not set correctly", "Base label is not set correctly",
		},
		{
			"virtual label only",
			nil,
			map[string]string{"virtual-key": "virtual-value"},
			"",
			`{"provisioning":{"virtual-key":"virtual-value"}}`, "gcp-cvs",
			"Base label is not empty", "Virtual pool label is not set correctly",
		},
		{
			"base and virtual labels",
			map[string]string{"base-key": "base-value"},
			map[string]string{"virtual-key": "virtual-value"},
			`{"provisioning":{"base-key":"base-value"}}`,
			`{"provisioning":{"base-key":"base-value","virtual-key":"virtual-value"}}`,
			"gcp-cvs",
			"Base label is not set correctly", "Virtual pool label is not set correctly",
		},
	}

	for _, c := range cases {
		c := c // capture range variable
		t.Run(c.Name, func(t *testing.T) {
			d.Config.Labels = c.physicalPoolLabels
			if c.virtualPoolLabels != nil {
				d.Config.Storage = []drivers.GCPNFSStorageDriverPool{
					{
						Region: "us_east_1",
						Zone:   "us_east_1a",
						SupportedTopologies: []map[string]string{
							{
								"topology.kubernetes.io/region": "us_east_1",
								"topology.kubernetes.io/zone":   "us_east_1a",
							},
						},
						Labels: c.virtualPoolLabels,
					},
				}
			}
			d.initializeStoragePools(ctx)

			if c.virtualPoolLabels == nil {
				physicalPool := d.pools["gcpcvs_12345_pool"]
				label, err := physicalPool.GetLabelsJSON(ctx, "provisioning", 255)
				assert.Nil(t, err, "Error is not nil")
				assert.Equal(t, c.physicalExpected, label, c.physicalErrorMessage)
			} else {
				virtualPool := d.pools["gcpcvs_12345_pool_0"]
				label, err := virtualPool.GetLabelsJSON(ctx, "provisioning", 255)
				assert.Nil(t, err, "Error is not nil")
				assert.Equal(t, c.virtualExpected, label, c.virtualErrorMessage)
			}
		})
	}
}

func TestInitializeStoragePoolsUnixPermissions(t *testing.T) {
	// 1. Test with one vpool defined
	ctx := context.Background()
	d := newTestGCPDriver(nil)

	d.Config.UnixPermissions = "0111"
	d.Config.Storage = []drivers.GCPNFSStorageDriverPool{
		{
			Region: "us_east_1",
			Zone:   "us_east_1a",
		},
	}
	d.initializeStoragePools(ctx)

	virtualPool := d.pools["gcpcvs_12345_pool_0"]
	assert.Equal(t, "0111", virtualPool.InternalAttributes()[UnixPermissions])

	// 2. Test with no vpool defined
	ctx = context.Background()
	d = newTestGCPDriver(nil)

	d.Config.UnixPermissions = "0111"
	d.initializeStoragePools(ctx)

	virtualPool = d.pools["gcpcvs_12345_pool"]
	assert.NotNil(t, virtualPool, "Could not find pool")
	assert.NotNil(t, virtualPool.InternalAttributes(), "Could not find pool attributes")
	assert.Equal(t, "0111", virtualPool.InternalAttributes()[UnixPermissions])

	// 3. Test with multiple vpools defined, one with unix permissions set
	ctx = context.Background()
	d = newTestGCPDriver(nil)

	d.Config.UnixPermissions = "0111"
	d.Config.Storage = []drivers.GCPNFSStorageDriverPool{
		{
			Region: "us_east_1",
			Zone:   "us_east_1a",
		},
		{
			GCPNFSStorageDriverConfigDefaults: drivers.GCPNFSStorageDriverConfigDefaults{UnixPermissions: "0222"},
			Region:                            "us_east_1",
			Zone:                              "us_east_1a",
		},
	}
	d.initializeStoragePools(ctx)

	virtualPool = d.pools["gcpcvs_12345_pool_0"]
	assert.Equal(t, "0111", virtualPool.InternalAttributes()[UnixPermissions])

	virtualPool2 := d.pools["gcpcvs_12345_pool_1"]
	assert.Equal(t, "0222", virtualPool2.InternalAttributes()[UnixPermissions])

	// 4. Test defaults
	ctx = context.Background()
	d = newTestGCPDriver(nil)

	err := d.populateConfigurationDefaults(ctx, &d.Config)
	assert.Nil(t, err, "Error is not nil")
	d.initializeStoragePools(ctx)

	virtualPool = d.pools["gcpcvs_12345_pool"]
	assert.NotNil(t, virtualPool, "Could not find pool")
	assert.NotNil(t, virtualPool.InternalAttributes(), "Could not find pool attributes")
	assert.Equal(t, "0777", virtualPool.InternalAttributes()[UnixPermissions])
}

func TestPopulateConfigurationDefaultsSnapshotDir(t *testing.T) {
	ctx := context.Background()
	d := newTestGCPDriver(nil)

	tests := []struct {
		name                string
		inputSnapshotDir    string
		expectedSnapshotDir string
		expectErr           bool
	}{
		{"Default snapshotDir", "", "false", false},
		{"Uppercase snapshotDir", "TRUE", "true", false},
		{"Invalid snapshotDir", "TrUE", "", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			d.Config.SnapshotDir = test.inputSnapshotDir
			response := d.populateConfigurationDefaults(ctx, &d.Config)
			if test.expectErr {
				assert.Error(t, response)
			} else {
				assert.Equal(t, test.expectedSnapshotDir, d.Config.SnapshotDir)
			}
		})
	}
}

func TestInitializeStoragePoolsSnapshotDir(t *testing.T) {
	// Test with multiple vpools defined, one with snapshotDir set
	ctx := context.Background()
	d := newTestGCPDriver(nil)

	d.Config.SnapshotDir = "TRUE"
	d.Config.Storage = []drivers.GCPNFSStorageDriverPool{
		{
			Region: "us_east_1",
			Zone:   "us_east_1a",
		},
		{
			GCPNFSStorageDriverConfigDefaults: drivers.GCPNFSStorageDriverConfigDefaults{SnapshotDir: "False"},
			Region:                            "us_east_1",
			Zone:                              "us_east_1a",
		},
		{
			GCPNFSStorageDriverConfigDefaults: drivers.GCPNFSStorageDriverConfigDefaults{SnapshotDir: "Invalid"},
			Region:                            "us_east_1",
			Zone:                              "us_east_1a",
		},
	}
	d.initializeStoragePools(ctx)

	// Case1: Ensure snapshotDir is picked from driver config and is lower case if valid
	virtualPool1 := d.pools["gcpcvs_12345_pool_0"]
	assert.Equal(t, "true", virtualPool1.InternalAttributes()[SnapshotDir])

	// Case2: Ensure snapshotDir is picked from virtual pool and is lower case if valid
	virtualPool2 := d.pools["gcpcvs_12345_pool_1"]
	assert.Equal(t, "false", virtualPool2.InternalAttributes()[SnapshotDir])

	// Case3: Ensure snapshotDir is picked from virtual pool and is same as passed if invalid bool
	virtualPool3 := d.pools["gcpcvs_12345_pool_2"]
	assert.Equal(t, "Invalid", virtualPool3.InternalAttributes()[SnapshotDir])
}

func TestEnsureTopologyRegionAndZone(t *testing.T) {
	tests := []struct {
		Name            string
		ConfigRegion    string
		ConfigZone      string
		PreferredRegion string
		PreferredZone   string
		ExpectedRegion  string
		ExpectedZone    string
	}{
		{
			Name:            "matchingRegionAndZone",
			ConfigRegion:    "foo",
			ConfigZone:      "bar",
			PreferredRegion: "foo",
			PreferredZone:   "bar",
			ExpectedZone:    "bar",
		},
		{
			Name:            "matchingRegionDifferentZone",
			ConfigRegion:    "foo",
			ConfigZone:      "bar",
			PreferredRegion: "foo",
			PreferredZone:   "baz",
			ExpectedZone:    "bar",
		},
		{
			Name:            "matchingRegionNoZone",
			ConfigRegion:    "foo",
			ConfigZone:      "",
			PreferredRegion: "foo",
			PreferredZone:   "bar",
			ExpectedZone:    "bar",
		},
		{
			Name:            "differentRegionMatchingZone",
			ConfigRegion:    "foo",
			ConfigZone:      "bar",
			PreferredRegion: "baz",
			PreferredZone:   "bar",
			ExpectedZone:    "bar",
		},
		{
			Name:            "differentRegionAndZone",
			ConfigRegion:    "foo",
			ConfigZone:      "bar",
			PreferredRegion: "baz",
			PreferredZone:   "biz",
			ExpectedZone:    "bar",
		},
		{
			Name:            "differentRegionNoZone",
			ConfigRegion:    "foo",
			ConfigZone:      "",
			PreferredRegion: "baz",
			PreferredZone:   "bar",
			ExpectedZone:    "bar",
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			d := newTestGCPDriver(nil)
			d.Config.Region = test.ConfigRegion
			volConfig := &storage.VolumeConfig{
				PreferredTopologies: []map[string]string{
					{
						topologyRegionLabel: test.PreferredRegion,
						topologyZoneLabel:   test.PreferredZone,
					},
				},
			}
			zone := d.ensureTopologyRegionAndZone(context.Background(), volConfig, api.StorageClassSoftware,
				test.ConfigZone)
			// Verify the allowedTopologies hasn't been modified
			assert.Nil(t, volConfig.AllowedTopologies, "Unexpected allowed topologies")
			assert.Equal(t, test.ExpectedZone, zone, "Unexpected zone")
		})
	}
}

func TestGetPoolsForCreate(t *testing.T) {
	sPool := storage.NewStoragePool(nil, "spool1")

	tests := []struct {
		Name         string
		poolConfig   string
		storageClass string
		volSize      int64
		mockFunc     func(client *mockGCPClient.MockGCPClient)
		wantErr      assert.ErrorAssertionFunc
	}{
		{
			Name:         "GetPoolsApiError",
			poolConfig:   "abc1",
			storageClass: api.StorageClassSoftware,
			volSize:      100,
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetPools(gomock.Any()).Return(nil, errors.New("failed to get pools"))
			},
			wantErr: assert.Error,
		},
		{
			// getDummyGCPPools doesn't have pool4, so we get empty pool array in this case
			Name:         "GetPoolsNoMatch",
			poolConfig:   "abc5",
			storageClass: api.StorageClassSoftware,
			volSize:      200,
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetPools(gomock.Any()).Return(getDummyGCPPools(), nil)
			},
			wantErr: assert.Error,
		},
		{
			Name:         "GetPoolsWithLeastVols",
			poolConfig:   "",
			storageClass: api.StorageClassSoftware,
			volSize:      300,
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetPools(gomock.Any()).Return(getDummyGCPPools(), nil)
			},
			wantErr: assert.NoError,
		},
		{
			Name:         "VolumeLimitReachedError",
			poolConfig:   "abc3",
			storageClass: api.StorageClassSoftware,
			volSize:      300,
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetPools(gomock.Any()).Return(getDummyGCPPools(), nil)
			},
			wantErr: assert.Error,
		},
		{
			Name:         "UnsupportedCapacityError",
			poolConfig:   "abc2",
			storageClass: api.StorageClassSoftware,
			volSize:      300,
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetPools(gomock.Any()).Return(getDummyGCPPools(), nil)
			},
			wantErr: assert.Error,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			sPool.InternalAttributes()[StoragePools] = tt.poolConfig
			sPool.InternalAttributes()[StorageClass] = tt.storageClass

			gcpClient, d := newMockGCPDriver(t)
			tt.mockFunc(gcpClient)

			gPool, err := d.GetPoolsForCreate(ctx(), sPool, api.PoolServiceLevel1, tt.volSize)
			tt.wantErr(t, err, "Unexpected error")
			if err == nil && tt.Name != "GetPoolsNoMatch" && tt.Name != "GetPoolsForPO" {
				assert.NotNil(t, gPool, "gPool is empty")
			}
		})
	}
}

func TestInitialize(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "gcp-cvs",
		BackendName:       "myGCPBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "gcp-cvs",
        "apiRegion": "fake-region",
		"zone": "fake-zone",
		"projectNumber": "123456",
		"storageClass": "software",
		"serviceLevel": "standardsw",
		"network": "fake-network",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true}
    }`

	gcpClient, d := newMockGCPDriver(t)

	apiVersion, _ := versionutils.ParseSemantic("1.4.0")
	sdeVersion, _ := versionutils.ParseSemantic("2023.1.2")
	gcpClient.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, sdeVersion, nil)
	gcpClient.EXPECT().GetVolumes(gomock.Any()).Return(nil, nil)
	gcpClient.EXPECT().GetServiceLevels(gomock.Any()).Return(nil, nil)

	err := d.Initialize(ctx(), tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{}, "abcd")
	assert.NoError(t, err, "Not initialized")
	assert.True(t, d.initialized, "Driver not initialized")
}

func TestInitialize_MultipleVPools(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "gcp-cvs",
		BackendName:       "myGCPBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "gcp-cvs",
        "apiRegion": "fake-region",
		"zone": "fake-zone",
		"projectNumber": "123456",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
		"storage": [
			{
				"storageClass": "software",
				"serviceLevel": "standardsw",
				"defaults": {
					"size": "1Gi",
					"snapshotDir": "true",
					"snapshotReserve": "5",
					"exportRule": "0.0.0.0/0"
				},
				"network": "test-network",
				"storagePools": ["abcd"]
			}
		]
    }`

	gcpClient, d := newMockGCPDriver(t)

	apiVersion, _ := versionutils.ParseSemantic("1.4.0")
	sdeVersion, _ := versionutils.ParseSemantic("2023.1.2")
	gcpClient.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, sdeVersion, nil)
	gcpClient.EXPECT().GetVolumes(gomock.Any()).Return(nil, nil)
	gcpClient.EXPECT().GetServiceLevels(gomock.Any()).Return(nil, nil)

	err := d.Initialize(ctx(), tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{}, "abcd")
	assert.NoError(t, err, "Not initialized")
	assert.True(t, d.initialized, "Driver not initialized")
	assert.NotEmpty(t, d.Config.BackendPools, "backend pools were empty")
	assert.Equal(t, len(d.pools), len(d.Config.BackendPools), "backend and storage pools were not equal")
}

func TestInitialize_ValidateError(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "gcp-cvs",
		BackendName:       "myGCPBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "gcp-cvs",
		"zone": "fake-zone",
		"projectNumber": "123456",
		"storageClass": "software",
		"serviceLevel": "standardsw",
		"network": "fake-network",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true}
    }`

	d := newTestGCPDriver(nil)

	err := d.Initialize(ctx(), tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{}, "abcd")
	assert.Error(t, err, "Initialized")
	assert.False(t, d.initialized, "Driver initialized")
}

func TestInitialize_InvalidConfigJSON(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "gcp-cvs",
		BackendName:       "myGCPBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "gcp-cvs",
        "apiRegion": "fake-region",
		"zone": "fake-zone,
		"projectNumber": "123456",
		"storageClass": "software",
		"serviceLevel": "standardsw",
		"network": "fake-network",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true}
    }`

	d := newTestGCPDriver(nil)

	err := d.Initialize(ctx(), tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{}, "abcd")
	assert.ErrorContains(t, err, "could not decode JSON configuration", "Valid json")
	assert.False(t, d.initialized, "Driver initialized")
}

func TestInitialize_InvalidSecrets(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "gcp-cvs",
		BackendName:       "myGCPBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "gcp-cvs",
        "apiRegion": "fake-region",
		"zone": "fake-zone",
		"projectNumber": "123456",
		"storageClass": "software",
		"serviceLevel": "standardsw",
		"network": "fake-network",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true}
    }`

	secrets := map[string]string{
		"dummy": "dummy",
	}

	d := newTestGCPDriver(nil)

	err := d.Initialize(ctx(), tridentconfig.ContextCSI, configJSON, commonConfig, secrets, "abcd")
	assert.ErrorContains(t, err, "could not inject backend secret", "Valid secret")
	assert.False(t, d.Initialized(), "Driver initialized")
}

func TestInitialize_InvalidVolumeCreateTimeout(t *testing.T) {
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "gcp-cvs",
		BackendName:       "myGCPBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
    {
		"version": 1,
        "storageDriverName": "gcp-cvs",
        "apiRegion": "fake-region",
		"zone": "fake-zone",
		"projectNumber": "123456",
		"storageClass": "software",
		"network": "fake-network",
        "debugTraceFlags": {"method": true, "api": true, "discovery": true},
		"volumeCreateTimeout": "10.1"
    }`

	d := newTestGCPDriver(nil)

	err := d.Initialize(ctx(), tridentconfig.ContextCSI, configJSON, commonConfig, map[string]string{}, "abcd")
	assert.ErrorContains(t, err, "strconv.ParseUint", "Valid volume create timeout")
	assert.False(t, d.initialized, "Driver initialized")
}

func TestTerminate(t *testing.T) {
	d := newTestGCPDriver(nil)
	d.Terminate(ctx(), "test")
	assert.False(t, d.Initialized(), "Driver not terminated")
}

func TestValidate(t *testing.T) {
	apiVersion, _ := versionutils.ParseSemantic(MinimumAPIVersion)
	sdeVersion, _ := versionutils.ParseSemantic(MinimumSDEVersion)
	invalidVersion, _ := versionutils.ParseSemantic("0.0.0")

	tests := []struct {
		name      string
		apiRegion string
		prefix    string
		mockFunc  func(client *mockGCPClient.MockGCPClient)
		wantErr   assert.ErrorAssertionFunc
		errStr    string
	}{
		{
			name:     "EmptyApiRegion",
			prefix:   "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {},
			wantErr:  assert.Error,
			errStr:   "apiRegion in config must be specified",
		},
		{
			name:      "GetVersionError",
			apiRegion: "us-east1",
			prefix:    "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).
					Return(apiVersion, sdeVersion, errors.New("failed to get version"))
			},
			wantErr: assert.Error,
			errStr:  "failed to get version",
		},
		{
			name:      "GetVolumesError",
			apiRegion: "us-east1",
			prefix:    "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, sdeVersion, nil)
				client.EXPECT().GetServiceLevels(ctx()).Return(nil, nil)
				client.EXPECT().GetVolumes(ctx()).Return(nil, errors.New("failed to get volumes"))
			},
			wantErr: assert.Error,
			errStr:  "failed to get volumes",
		},
		{
			name:      "InvalidApiVersion",
			apiRegion: "us-east1",
			prefix:    "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).Return(invalidVersion, sdeVersion, nil)
				client.EXPECT().GetServiceLevels(ctx()).Return(nil, nil)
				client.EXPECT().GetVolumes(ctx()).Return(nil, nil)
			},
			wantErr: assert.Error,
			errStr:  "API version",
		},
		{
			name:      "InvalidSDEVersion",
			apiRegion: "us-east1",
			prefix:    "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, invalidVersion, nil)
				client.EXPECT().GetServiceLevels(ctx()).Return(nil, nil)
				client.EXPECT().GetVolumes(ctx()).Return(nil, nil)
			},
			wantErr: assert.Error,
			errStr:  "SDE version",
		},
		{
			name:      "InvalidStoragePrefix",
			apiRegion: "us-east1",
			prefix:    "_a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, sdeVersion, nil)
				client.EXPECT().GetServiceLevels(ctx()).Return(nil, nil)
				client.EXPECT().GetVolumes(ctx()).Return(nil, nil)
			},
			wantErr: assert.Error,
			errStr:  "storage prefix",
		},
		{
			name:      "InvalidStorageClass",
			apiRegion: "us-east1",
			prefix:    "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, sdeVersion, nil)
				client.EXPECT().GetServiceLevels(ctx()).Return(nil, nil)
				client.EXPECT().GetVolumes(ctx()).Return(nil, nil)
			},
			wantErr: assert.Error,
			errStr:  "invalid storage class in pool",
		},
		{
			name:      "InvalidServiceLevel",
			apiRegion: "us-east1",
			prefix:    "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, sdeVersion, nil)
				client.EXPECT().GetServiceLevels(ctx()).Return(nil, nil)
				client.EXPECT().GetVolumes(ctx()).Return(nil, nil)
			},
			wantErr: assert.Error,
			errStr:  "invalid service level in pool",
		},
		{
			name:      "InvalidStoragePools",
			apiRegion: "us-east1",
			prefix:    "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, sdeVersion, nil)
				client.EXPECT().GetServiceLevels(ctx()).Return(nil, nil)
				client.EXPECT().GetVolumes(ctx()).Return(nil, nil)
			},
			wantErr: assert.Error,
			errStr:  "storagePools not expected for hardware type storage class pool",
		},
		{
			name:      "InvalidExportRule",
			apiRegion: "us-east1",
			prefix:    "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, sdeVersion, nil)
				client.EXPECT().GetServiceLevels(ctx()).Return(nil, nil)
				client.EXPECT().GetVolumes(ctx()).Return(nil, nil)
			},
			wantErr: assert.Error,
			errStr:  "invalid address/CIDR for exportRule in pool",
		},
		{
			name:      "InvalidSnapshotDir",
			apiRegion: "us-east1",
			prefix:    "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, sdeVersion, nil)
				client.EXPECT().GetServiceLevels(ctx()).Return(nil, nil)
				client.EXPECT().GetVolumes(ctx()).Return(nil, nil)
			},
			wantErr: assert.Error,
			errStr:  "invalid value for snapshotDir in pool",
		},
		{
			name:      "InvalidSnapshotReserve",
			apiRegion: "us-east1",
			prefix:    "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, sdeVersion, nil)
				client.EXPECT().GetServiceLevels(ctx()).Return(nil, nil)
				client.EXPECT().GetVolumes(ctx()).Return(nil, nil)
			},
			wantErr: assert.Error,
			errStr:  "invalid value for snapshotReserve in pool",
		},
		{
			name:      "OutOfRangeSnapshotReserve",
			apiRegion: "us-east1",
			prefix:    "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, sdeVersion, nil)
				client.EXPECT().GetServiceLevels(ctx()).Return(nil, nil)
				client.EXPECT().GetVolumes(ctx()).Return(nil, nil)
			},
			wantErr: assert.Error,
			errStr:  "invalid value for snapshotReserve in pool",
		},
		{
			name:      "InvalidVolumeSize",
			apiRegion: "us-east1",
			prefix:    "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, sdeVersion, nil)
				client.EXPECT().GetServiceLevels(ctx()).Return(nil, nil)
				client.EXPECT().GetVolumes(ctx()).Return(nil, nil)
			},
			wantErr: assert.Error,
			errStr:  "invalid value for default volume size in pool",
		},
		{
			name:      "InvalidLabels",
			apiRegion: "us-east1",
			prefix:    "a",
			mockFunc: func(client *mockGCPClient.MockGCPClient) {
				client.EXPECT().GetVersion(gomock.Any()).Return(apiVersion, sdeVersion, nil)
				client.EXPECT().GetServiceLevels(ctx()).Return(nil, nil)
				client.EXPECT().GetVolumes(ctx()).Return(nil, nil)
			},
			wantErr: assert.Error,
			errStr:  "invalid value for label in pool",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gcpClient, d := newMockGCPDriver(t)
			tt.mockFunc(gcpClient)
			d.populateConfigurationDefaults(ctx(), &d.Config)
			d.Config.APIRegion = tt.apiRegion
			d.Config.StoragePrefix = &tt.prefix
			d.Config.StoragePools = []string{}

			if tt.name == "InvalidStorageClass" {
				d.Config.StorageClass = "softwareAndHardware"
			}

			if tt.name == "InvalidServiceLevel" {
				d.Config.ServiceLevel = "testServiceLevel"
			}

			if tt.name == "InvalidStoragePools" {
				d.Config.StoragePools = []string{"pool1"}
			}

			if tt.name == "InvalidExportRule" {
				d.Config.ExportRule = "fake-rule"
			}

			if tt.name == "InvalidSnapshotDir" {
				d.Config.SnapshotDir = "fake-bool"
			}

			if tt.name == "InvalidSnapshotReserve" {
				d.Config.SnapshotReserve = "a"
			}

			if tt.name == "OutOfRangeSnapshotReserve" {
				d.Config.SnapshotReserve = "100"
			}

			if tt.name == "InvalidVolumeSize" {
				d.Config.Size = "1.1"
			}

			if tt.name == "InvalidLabels" {
				d.Config.Labels = map[string]string{
					"key1": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
					"key2": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
					"key3": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
				}
			}

			// Initialize pools with above configs
			d.initializeStoragePools(ctx())

			err := d.validate(ctx())
			tt.wantErr(t, err, "Validate did not fail")
			assert.ErrorContains(t, err, tt.errStr, "Unexpected error")
		})
	}
}

func TestCreate_InvalidVolumeName(t *testing.T) {
	volConfig := &storage.VolumeConfig{InternalName: "_invalid_vol_name"}
	d := newTestGCPDriver(nil)
	err := d.Create(ctx(), volConfig, nil, nil)

	assert.Error(t, err, "Valid volume name")
}

func TestCreate_VPoolNotSpecified(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	d := newTestGCPDriver(nil)
	err := d.Create(ctx(), volConfig, nil, nil)

	assert.ErrorContains(t, err, "pool not specified", "Valid vpool")
}

func TestCreate_VPoolNotPresent(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	vPool := storage.NewStoragePool(nil, "vpool-name2")
	d := newTestGCPDriver(nil)
	err := d.Create(ctx(), volConfig, vPool, nil)

	assert.ErrorContains(t, err, "pool vpool-name2 does not exist", "VPool present")
}

func TestCreate_VolumeExistsErrors(t *testing.T) {
	volConfig, vPool := getCreateVolumeStructs()
	extantVol := &api.Volume{Region: "random-region", LifeCycleState: api.StateCreating}
	gcpClient, d := newMockGCPDriver(t)

	// Volume exists API error
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(true, nil, errors.New("API call failed"))
	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "API call failed", "Volume exists API call succeeded")

	// Volume exists in other region error
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(true, extantVol, nil)
	err = d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "already exists in region", "Volume doesn't exist")

	// Volume is in Creating state error
	extantVol.Region = d.Config.APIRegion
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(true, extantVol, nil)
	err = d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "volume state is still creating", "Volume doesn't exist")

	// Volume exists error
	extantVol.LifeCycleState = api.StateAvailable
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(true, extantVol, nil)
	err = d.Create(ctx(), volConfig, vPool, nil)
	assert.True(t, drivers.IsVolumeExistsError(err), "Volume doesn't exist")
}

func TestCreate_VolumeSizeErrors(t *testing.T) {
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil).AnyTimes()

	// Convert size to bytes error
	volConfig.Size = "1i"
	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "could not convert volume size", "Valid volume size")

	// Invalid volume size
	volConfig.Size = "-1Gi"
	err = d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "an invalid volume size", "Valid volume size")

	// Minimum volume size error
	volConfig.Size = "0"
	err = d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "the minimum volume size is", "Valid volume size")

	// Config volume size limit error
	volConfig.Size = "1Gi"
	d.Config.CommonStorageDriverConfig.LimitVolumeSize = "1Mi"
	err = d.Create(ctx(), volConfig, vPool, nil)
	isErr, _ := errors.HasUnsupportedCapacityRangeError(err)
	assert.True(t, isErr, "Valid volume size")
}

func getCreateVolumeStructs() (*storage.VolumeConfig, storage.Pool) {
	vPool := storage.NewStoragePool(nil, "vpool-name")
	vPool.InternalAttributes()[StorageClass] = api.StorageClassHardware
	vPool.InternalAttributes()[StoragePools] = "abc1"
	vPool.InternalAttributes()[UnixPermissions] = "0755"
	vPool.InternalAttributes()[Size] = "0"
	vPool.InternalAttributes()[ServiceLevel] = api.UserServiceLevel1
	vPool.InternalAttributes()[SnapshotDir] = "true"
	vPool.InternalAttributes()[SnapshotReserve] = "5"
	vPool.InternalAttributes()[Network] = "fake-network"
	vPool.InternalAttributes()[Zone] = "fake-zone"
	vPool.Attributes()[sa.Labels] = sa.NewLabelOffer(map[string]string{"key1": "1234"})

	volConfig := &storage.VolumeConfig{Name: "valid-vol-name", InternalName: "valid-vol-name", Size: "1Gi"}

	return volConfig, vPool
}

func TestCreate_InvalidServiceLevel(t *testing.T) {
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)

	d.pools[vPool.Name()].InternalAttributes()[StorageClass] = api.StorageClassHardware
	d.pools[vPool.Name()].InternalAttributes()[ServiceLevel] = api.PoolServiceLevel1
	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "invalid service level", "Valid service level")
}

func TestCreate_InvalidSnapshotDir(t *testing.T) {
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)

	d.pools[vPool.Name()].InternalAttributes()[SnapshotDir] = "test"
	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "invalid value for snapshotDir", "Valid snapshot dir")
}

func TestCreate_InvalidSnapshotReserve(t *testing.T) {
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)

	d.pools[vPool.Name()].InternalAttributes()[SnapshotReserve] = "test"
	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "invalid value for snapshotReserve", "Valid snapshot reserve")
}

func TestCreate_InvalidZone(t *testing.T) {
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)

	d.pools[vPool.Name()].InternalAttributes()[Zone] = ""
	d.pools[vPool.Name()].InternalAttributes()[StorageClass] = api.StorageClassSoftware
	d.pools[vPool.Name()].InternalAttributes()[ServiceLevel] = api.PoolServiceLevel1
	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "software volumes require zone", "Valid zone value")
}

func TestCreate_InvalidPoolLabels(t *testing.T) {
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)

	d.Config.Labels = map[string]string{
		"key1": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
		"key2": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
		"key3": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
	}

	d.pools[vPool.Name()].InternalAttributes()[SnapshotReserve] = "10"
	d.pools[vPool.Name()].Attributes()[sa.Labels] = sa.NewLabelOffer(d.Config.Labels)
	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "exceeds the character limit", "Valid pool labels")
}

func TestCreate_POVolumeError(t *testing.T) {
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)
	gcpClient.EXPECT().CreateVolume(ctx(), gomock.Any()).Return(errors.New("volume creation failed"))

	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "volume creation failed", "Volume created")
}

func TestCreate_GetPOVolumeByTokenError(t *testing.T) {
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)
	gcpClient.EXPECT().CreateVolume(ctx(), gomock.Any()).Return(nil)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).
		Return(nil, errors.New("failed to get volume"))

	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "failed to get volume", "Volume created")
}

func TestCreate_POVolume(t *testing.T) {
	volume := &api.Volume{}
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)
	gcpClient.EXPECT().CreateVolume(ctx(), gomock.Any()).Return(nil)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(volume, nil)
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", nil)

	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.NoError(t, err, "failed to get volume", "Volume creation failed")
	assert.Equal(t, api.StorageClassHardware, volConfig.CVSStorageClass)
	assert.Equal(t, "0755", volConfig.UnixPermissions)
	assert.Equal(t, "107374182400", volConfig.Size)
	assert.Equal(t, api.UserServiceLevel1, volConfig.ServiceLevel)
	assert.Equal(t, "true", volConfig.SnapshotDir)
	assert.Equal(t, "5", volConfig.SnapshotReserve)
	assert.Equal(t, "fake-network", volConfig.Network)
	assert.Equal(t, "fake-zone", volConfig.Zone)
}

func TestCreate_SOVolumePoolsError(t *testing.T) {
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	d.pools[vPool.Name()].InternalAttributes()[StorageClass] = api.StorageClassSoftware
	d.pools[vPool.Name()].InternalAttributes()[ServiceLevel] = api.PoolServiceLevel1

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)
	gcpClient.EXPECT().GetPools(ctx()).Return(nil, errors.New("failed to get pools"))

	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "failed to get pools", "Volume created")
}

func TestCreate_SOVolumeError(t *testing.T) {
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	volConfig.StorageClass = api.StorageClassSoftware
	volConfig.ServiceLevel = api.PoolServiceLevel1
	volConfig.Size = "1Gi"
	d.pools[vPool.Name()].InternalAttributes()[StorageClass] = api.StorageClassSoftware
	d.pools[vPool.Name()].InternalAttributes()[ServiceLevel] = api.PoolServiceLevel1

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)
	gcpClient.EXPECT().GetPools(ctx()).Return(getDummyGCPPools(), nil)
	gcpClient.EXPECT().CreateVolume(ctx(), gomock.Any()).Return(errors.New("volume create failed"))

	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.ErrorContains(t, err, "volume create failed", "Volume created")
}

func TestCreate_SOVolumeCreatingError(t *testing.T) {
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)
	volume := &api.Volume{}

	volConfig.StorageClass = api.StorageClassSoftware
	volConfig.ServiceLevel = api.PoolServiceLevel1
	volConfig.Size = "1Gi"
	d.pools[vPool.Name()].InternalAttributes()[StorageClass] = api.StorageClassSoftware
	d.pools[vPool.Name()].InternalAttributes()[ServiceLevel] = api.PoolServiceLevel1

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)
	gcpClient.EXPECT().GetPools(ctx()).Return(getDummyGCPPools(), nil)
	gcpClient.EXPECT().CreateVolume(ctx(), gomock.Any()).Return(nil)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(volume, nil)
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(api.StateCreating, errors.New("failed"))

	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.True(t, errors.IsVolumeCreatingError(err), "Volume created")
}

func TestCreate_SOVolume(t *testing.T) {
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)
	pools := getDummyGCPPools()
	volume := &api.Volume{}

	volConfig.StorageClass = api.StorageClassSoftware
	volConfig.ServiceLevel = api.PoolServiceLevel2
	volConfig.Size = "1Gi"
	d.pools[vPool.Name()].InternalAttributes()[StorageClass] = api.StorageClassSoftware
	d.pools[vPool.Name()].InternalAttributes()[ServiceLevel] = api.PoolServiceLevel2
	// Setting service level of "abc1" pool to zone redundant for covering RegionalHA condition
	(*pools)[0].ServiceLevel = api.PoolServiceLevel2

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)
	gcpClient.EXPECT().GetPools(ctx()).Return(pools, nil)
	gcpClient.EXPECT().CreateVolume(ctx(), gomock.Any()).Return(nil)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(volume, nil)
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", nil)

	err := d.Create(ctx(), volConfig, vPool, nil)
	assert.NoError(t, err, "Volume creation failed")
	assert.Equal(t, api.StorageClassSoftware, volConfig.CVSStorageClass)
	assert.Equal(t, "0755", volConfig.UnixPermissions)
	assert.Equal(t, "1073741824", volConfig.Size)
	assert.Equal(t, api.PoolServiceLevel2, volConfig.ServiceLevel)
	assert.Equal(t, "true", volConfig.SnapshotDir)
	assert.Equal(t, "5", volConfig.SnapshotReserve)
	assert.Equal(t, "fake-network", volConfig.Network)
	assert.Equal(t, "fake-zone", volConfig.Zone)
}

func TestCreateClone_InvalidName(t *testing.T) {
	cloneVolConfig := &storage.VolumeConfig{}
	volConfig, vPool := getCreateVolumeStructs()
	_, d := newMockGCPDriver(t)

	err := d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "volume name '' is not allowed", "Valid clone name")
}

func TestCreateClone_VolumeExistsErrors(t *testing.T) {
	cloneVolConfig := &storage.VolumeConfig{InternalName: "clone-name"}
	volConfig, vPool := getCreateVolumeStructs()
	extantVol := &api.Volume{Region: "random-region", LifeCycleState: api.StateCreating}
	gcpClient, d := newMockGCPDriver(t)

	// Volume exists API call error
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(true, extantVol, errors.New("API call failed"))
	err := d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "API call failed", "Volume exists API call succeeded")

	// Volume creating/restoring error
	extantVol.LifeCycleState = api.StateRestoring
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(true, extantVol, nil)
	err = d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.True(t, errors.IsVolumeCreatingError(err), "Volume doesn't exist")

	// Volume available/updating error
	extantVol.LifeCycleState = api.StateUpdating
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(true, extantVol, nil)
	err = d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.True(t, drivers.IsVolumeExistsError(err), "Volume doesn't exist")

	// Volume unexpected state error
	extantVol.LifeCycleState = api.StateError
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(true, extantVol, nil)
	err = d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "volume state", "Volume doesn't exist")
}

func TestCreateClone_GetVolumeByTokenError(t *testing.T) {
	cloneVolConfig := &storage.VolumeConfig{InternalName: "clone-name"}
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).
		Return(nil, errors.New("failed to get volume"))

	err := d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "failed to get volume", "Found volume")
}

func TestCreateClone_DifferentRegionError(t *testing.T) {
	cloneVolConfig := &storage.VolumeConfig{InternalName: "clone-name"}
	srcVol := &api.Volume{Region: "wrong-region"}
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(srcVol, nil)

	err := d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "already exists in region", "Volume doesn't exist")
}

func TestCreateClone_GetVolumeByIDError(t *testing.T) {
	cloneVolConfig := &storage.VolumeConfig{InternalName: "clone-name"}
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)
	srcVol := &api.Volume{Region: d.Config.Region}

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(srcVol, nil)
	gcpClient.EXPECT().GetVolumeByID(ctx(), gomock.Any()).Return(srcVol, errors.New("failed to get volume"))

	err := d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "failed to get volume", "Found volume")
}

func TestCreateClone_NoSnapshot_CreateSnapshotErrors(t *testing.T) {
	cloneVolConfig := &storage.VolumeConfig{InternalName: "clone-name"}
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)
	srcVol := &api.Volume{Region: d.Config.Region, Name: "vol-name"}
	srcSnap := &api.Snapshot{Name: "snap-name"}

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil).Times(4)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(srcVol, nil).Times(4)
	gcpClient.EXPECT().GetVolumeByID(ctx(), gomock.Any()).Return(srcVol, nil).Times(4)

	// Create snapshot API failed
	gcpClient.EXPECT().CreateSnapshot(ctx(), gomock.Any()).Return(errors.New("failed to create snapshot"))
	err := d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "failed to create snapshot", "Snapshot created")

	// Get snapshot for volume failed
	gcpClient.EXPECT().CreateSnapshot(ctx(), gomock.Any()).Return(nil)
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("failed to get snapshot"))
	err = d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "failed to get snapshot", "Snapshot created")

	// Wait for snapshot state failed
	gcpClient.EXPECT().CreateSnapshot(ctx(), gomock.Any()).Return(nil)
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).Return(srcSnap, nil)
	gcpClient.EXPECT().WaitForSnapshotState(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("failed to get snapshot state"))
	err = d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "failed to get snapshot state", "Snapshot created")

	// Empty Zone value for software source volume
	srcVol.Zone = ""
	srcVol.StorageClass = api.StorageClassSoftware
	gcpClient.EXPECT().CreateSnapshot(ctx(), gomock.Any()).Return(nil)
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).Return(srcSnap, nil)
	gcpClient.EXPECT().WaitForSnapshotState(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	err = d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "software volumes require zone", "Snapshot created")
}

func TestCreateClone_ExistingSnapshot_GetSnapshotErrors(t *testing.T) {
	cloneVolConfig := &storage.VolumeConfig{InternalName: "clone-name", CloneSourceSnapshotInternal: "present"}
	volConfig, vPool := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)
	srcVol := &api.Volume{Region: d.Config.Region, Name: "vol-name"}
	srcSnap := &api.Snapshot{Name: "snap-name"}

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil).Times(2)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(srcVol, nil).Times(2)
	gcpClient.EXPECT().GetVolumeByID(ctx(), gomock.Any()).Return(srcVol, nil).Times(2)

	// Get snapshot from volume failed
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("failed to get snapshot"))
	err := d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "failed to get snapshot", "Snapshot created")

	// Existing Snapshot state error
	srcSnap.LifeCycleState = api.StateError
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).Return(srcSnap, nil)
	err = d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "source snapshot state", "Snapshot created")
}

func TestCreateClone_UnsetPool(t *testing.T) {
	cloneVolConfig := &storage.VolumeConfig{InternalName: "clone-name", CloneSourceSnapshotInternal: "present"}
	volConfig, _ := getCreateVolumeStructs()

	gcpClient, d := newMockGCPDriver(t)
	srcVol := &api.Volume{Region: d.Config.Region, Name: "vol-name"}
	srcSnap := &api.Snapshot{Name: "snap-name"}

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil).Times(2)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(srcVol, nil).Times(2)
	gcpClient.EXPECT().GetVolumeByID(ctx(), gomock.Any()).Return(srcVol, nil).Times(2)

	// Unset pool label error
	vPool := storage.NewStoragePool(nil, "")
	srcSnap.LifeCycleState = api.StateAvailable
	d.Config.Labels = map[string]string{
		"key1": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
		"key2": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
		"key3": "1234567890123456789012345678901234567890123456789012345678901234567890123456789012345678901234567890",
	}
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).Return(srcSnap, nil)
	err := d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "exceeds the character limit", "Valid pool label")

	// Clone create failed
	d.Config.Labels = map[string]string{"key1": "1234"}
	cloneVolConfig.ServiceLevel = api.PoolServiceLevel2
	volConfig.ServiceLevel = api.PoolServiceLevel2
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).Return(srcSnap, nil)
	gcpClient.EXPECT().CreateVolume(ctx(), gomock.Any()).Return(errors.New("clone create failed"))
	err = d.CreateClone(ctx(), volConfig, cloneVolConfig, vPool)
	assert.ErrorContains(t, err, "clone create failed", "Clone created")
}

func TestImport_GetVolumeByTokenError(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).
		Return(nil, errors.New("failed to get volume"))

	err := d.Import(ctx(), volConfig, "test")
	assert.ErrorContains(t, err, "failed to get volume", "Found volume")
}

func TestImport_NotManagedVolume(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	volume := &api.Volume{QuotaInBytes: 1000, PoolID: "abc1", VolumeID: "xyz", StorageClass: api.StorageClassSoftware}
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(volume, nil)
	volConfig.ImportNotManaged = true

	err := d.Import(ctx(), volConfig, "test")
	assert.NoError(t, err, "Volume import failed")
}

func TestImport_NotManagedZoneRedundantVolume(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	volume := &api.Volume{QuotaInBytes: 1000, PoolID: "abc1", VolumeID: "xyz", StorageClass: api.StorageClassSoftware}
	gcpClient, d := newMockGCPDriver(t)

	// Zone Redundant Volume
	volume.RegionalHA = true
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(volume, nil)
	volConfig.ImportNotManaged = true

	err := d.Import(ctx(), volConfig, "test")
	assert.NoError(t, err, "Volume import failed")
}

func getTelemetryMapString(key string) string {
	telemetry := tridentconfig.OrchestratorTelemetry
	telemetry.TridentBackendUUID = "backend-id"
	telemetryStruct := Telemetry{
		Telemetry: telemetry,
		Plugin:    tridentconfig.GCPNFSStorageDriverName,
	}

	telemetryMap := map[string]Telemetry{key: telemetryStruct}
	telemetryJSON, _ := json.Marshal(telemetryMap)

	return strings.ReplaceAll(string(telemetryJSON), " ", "")
}

func TestImport_ManagedVolume(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	labels := []string{getTelemetryMapString(drivers.TridentLabelTag), getTelemetryMapString("test-key"), "fake"}
	volume := &api.Volume{
		QuotaInBytes: 1000,
		PoolID:       "abc1",
		VolumeID:     "xyz",
		Labels:       labels,
	}
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(volume, nil).Times(4)

	// Relabel API error
	gcpClient.EXPECT().RelabelVolume(ctx(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("relabel failed"))
	err := d.Import(ctx(), volConfig, "test")
	assert.ErrorContains(t, err, "relabel failed", "Volume imported")

	// Wait for volume state error
	gcpClient.EXPECT().RelabelVolume(ctx(), gomock.Any(), gomock.Any()).Return(nil, nil)
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", errors.New("failed to get volume state"))
	err = d.Import(ctx(), volConfig, "test")
	assert.ErrorContains(t, err, "failed to get volume state", "Volume imported")

	// Change volume unix permission error
	gcpClient.EXPECT().RelabelVolume(ctx(), gomock.Any(), gomock.Any()).Return(nil, nil)
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", nil)
	gcpClient.EXPECT().ChangeVolumeUnixPermissions(ctx(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("failed to change unix permissions"))
	err = d.Import(ctx(), volConfig, "test")
	assert.ErrorContains(t, err, "failed to change unix permissions", "Volume imported")

	// Wait for volume state error again
	gcpClient.EXPECT().RelabelVolume(ctx(), gomock.Any(), gomock.Any()).Return(nil, nil)
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", nil)
	gcpClient.EXPECT().ChangeVolumeUnixPermissions(ctx(), gomock.Any(), gomock.Any()).Return(nil, nil)
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", errors.New("failed to get volume state"))
	err = d.Import(ctx(), volConfig, "test")
	assert.ErrorContains(t, err, "failed to get volume state", "Volume imported")
}

func TestRename(t *testing.T) {
	d := newTestGCPDriver(nil)
	err := d.Rename(ctx(), "oldName", "newName")
	assert.NoError(t, err, "Volume rename failed")
}

func TestWaitForVolumeCreate_NonTransitionalState(t *testing.T) {
	volume := &api.Volume{}
	gcpClient, d := newMockGCPDriver(t)

	// Delete volume error log
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(api.StateError, errors.New("failed to get transitional state"))
	gcpClient.EXPECT().DeleteVolume(ctx(), gomock.Any()).Return(errors.New("delete volume failed"))
	err := d.waitForVolumeCreate(ctx(), volume, "vol-name")
	assert.ErrorContains(t, err, "failed to get transitional state", "Volume deleted")

	// Wait for volume state failure after deleting the volume
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(api.StateError, errors.New("failed to get transitional state"))
	gcpClient.EXPECT().DeleteVolume(ctx(), gomock.Any()).Return(nil)
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(api.StateError, errors.New("failed again"))
	err = d.waitForVolumeCreate(ctx(), volume, "vol-name")
	assert.ErrorContains(t, err, "failed to get transitional state", "Volume deleted")
}

func TestDestroy_VolumeExistsErrors(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	// Volume exists by token API failure
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(false, nil, errors.New("api failed"))
	err := d.Destroy(ctx(), volConfig)
	assert.ErrorContains(t, err, "api failed", "Volume destroyed")

	// Volume doesn't exists
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)
	err = d.Destroy(ctx(), volConfig)
	assert.NoError(t, err, "Volume exists")

	// Wait for volume state failure for deleting volume
	extantVol := &api.Volume{LifeCycleState: api.StateDeleting}
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(true, extantVol, nil)
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", errors.New("failed to get volume state"))
	err = d.Destroy(ctx(), volConfig)
	assert.ErrorContains(t, err, "failed to get volume state", "Volume destroyed")
}

func TestDestroy(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	extantVol := &api.Volume{LifeCycleState: api.StateAvailable}
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(true, extantVol, nil).Times(3)

	// Get volume by token API error
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).
		Return(nil, errors.New("failed to get volume"))
	err := d.Destroy(ctx(), volConfig)
	assert.ErrorContains(t, err, "failed to get volume", "Volume destroyed")

	// Delete volume API error
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(extantVol, nil)
	gcpClient.EXPECT().DeleteVolume(ctx(), gomock.Any()).Return(errors.New("failed to delete volume"))
	err = d.Destroy(ctx(), volConfig)
	assert.ErrorContains(t, err, "failed to delete volume", "Volume destroyed")

	// Wait for volume state API error
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(extantVol, nil)
	gcpClient.EXPECT().DeleteVolume(ctx(), gomock.Any()).Return(nil)
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", errors.New("failed to get volume state"))
	err = d.Destroy(ctx(), volConfig)
	assert.ErrorContains(t, err, "failed to get volume state", "Volume destroyed")
}

func TestPublish_GetVolumeByTokenError(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).
		Return(nil, errors.New("failed to get volume"))

	err := d.Publish(ctx(), volConfig, nil)
	assert.ErrorContains(t, err, "failed to get volume", "Volume found")
}

func TestPublish_NoMountsError(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&api.Volume{}, nil)

	err := d.Publish(ctx(), volConfig, nil)
	assert.ErrorContains(t, err, "has no mount targets", "Volume has mount targets")
}

func TestPublish(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)
	volConfig.MountOptions = d.Config.NfsMountOptions
	d.Config.NfsMountOptions = ""
	mountPoint := api.MountPoint{Server: "0.0.0.0", Export: "/tmp"}
	volume := &api.Volume{MountPoints: []api.MountPoint{mountPoint}}
	publishInfo := &utils.VolumePublishInfo{}

	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(volume, nil)

	err := d.Publish(ctx(), volConfig, publishInfo)
	assert.NoError(t, err, "Volume not published")
	assert.Equal(t, publishInfo.NfsServerIP, "0.0.0.0")
	assert.Equal(t, publishInfo.NfsPath, "/tmp")
	assert.Equal(t, publishInfo.FilesystemType, "nfs")
	assert.Equal(t, publishInfo.MountOptions, "nfsvers=3")
}

func TestCanSnapshot(t *testing.T) {
	d := newTestGCPDriver(nil)
	err := d.CanSnapshot(ctx(), nil, nil)
	assert.NoError(t, err, "Cannot take snapshot")
}

func getSnapshotVolumeStructs() (*storage.SnapshotConfig, *[]api.Snapshot) {
	snapConfig := &storage.SnapshotConfig{
		InternalName:       "snap1",
		VolumeInternalName: "vol1",
	}

	snap1 := api.Snapshot{Name: "snap1", LifeCycleState: api.StateAvailable}
	snap2 := api.Snapshot{Name: "snap2", LifeCycleState: api.StateUpdating}
	snapList := []api.Snapshot{snap1, snap2}
	return snapConfig, &snapList
}

func TestGetSnapshot_VolumeExistsErrors(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	// Volume exists by token API error
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(false, nil, errors.New("failed to get volume"))
	_, err := d.GetSnapshot(ctx(), snapConfig, nil)
	assert.ErrorContains(t, err, "failed to get volume", "Found volume")

	// Volume doesn't exist
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)
	_, err = d.GetSnapshot(ctx(), snapConfig, nil)
	assert.NoError(t, err, "Found volume")
}

func TestGetSnapshot_GetSnapshotForVolumeError(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(true, nil, nil)
	gcpClient.EXPECT().GetSnapshotsForVolume(ctx(), gomock.Any()).Return(nil, errors.New("failed to get snapshots"))

	_, err := d.GetSnapshot(ctx(), snapConfig, nil)
	assert.ErrorContains(t, err, "failed to get snapshots", "Got snapshots")
}

func TestGetSnapshot_Found(t *testing.T) {
	snapConfig, snapList := getSnapshotVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(true, &api.Volume{QuotaInBytes: 100}, nil)
	gcpClient.EXPECT().GetSnapshotsForVolume(ctx(), gomock.Any()).Return(snapList, nil)

	snap, err := d.GetSnapshot(ctx(), snapConfig, nil)
	assert.NoError(t, err, "Found snapshot")
	assert.Equal(t, snap.Config, snapConfig, "Config are not same")
	assert.Equal(t, snap.SizeBytes, int64(100), "Snapshot size is different")
	assert.Equal(t, snap.State, storage.SnapshotStateOnline, "Snapshot state incorrect")
}

func TestGetSnapshot_NotFound(t *testing.T) {
	snapConfig, snapList := getSnapshotVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)
	(*snapList)[0].Name = "fake-snap"

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(true, &api.Volume{QuotaInBytes: 100}, nil)
	gcpClient.EXPECT().GetSnapshotsForVolume(ctx(), gomock.Any()).Return(snapList, nil)

	snap, err := d.GetSnapshot(ctx(), snapConfig, nil)
	assert.NoError(t, err, "Found snapshot")
	assert.Nil(t, snap)
}

func TestGetSnapshots_GetVolumeByTokenError(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(nil, errors.New("failed to get volume"))

	_, err := d.GetSnapshots(ctx(), volConfig)
	assert.ErrorContains(t, err, "failed to get volume", "Found volume")
}

func TestGetSnapshots_GetSnapshotsForVolumeError(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(nil, nil)
	gcpClient.EXPECT().GetSnapshotsForVolume(ctx(), gomock.Any()).Return(nil, errors.New("failed to get snapshots"))

	_, err := d.GetSnapshots(ctx(), volConfig)
	assert.ErrorContains(t, err, "failed to get snapshots", "Found snapshots")
}

func TestGetSnapshots(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	_, snapList := getSnapshotVolumeStructs()
	volume := &api.Volume{QuotaInBytes: 100}
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(volume, nil)
	gcpClient.EXPECT().GetSnapshotsForVolume(ctx(), gomock.Any()).Return(snapList, nil)

	snapResults, err := d.GetSnapshots(ctx(), volConfig)
	assert.NoError(t, err, "Couldn't get snapshots")
	assert.Len(t, snapResults, 1, "Total number of snapshots found is not 1")
}

func TestCreateSnapshot_GetVolumeErrors(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	// Get volume by token API failed
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(false, nil, errors.New("failed to get volume"))
	_, err := d.CreateSnapshot(ctx(), snapConfig, nil)
	assert.ErrorContains(t, err, "failed to get volume", "Volume found")

	// Volume doesn't exist error
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(false, nil, nil)
	_, err = d.CreateSnapshot(ctx(), snapConfig, nil)
	assert.ErrorContains(t, err, "does not exist", "Volume found")

	// Get volume by ID API failed
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(true, &api.Volume{VolumeID: "123"}, nil)
	gcpClient.EXPECT().GetVolumeByID(ctx(), gomock.Any()).Return(nil, errors.New("failed to get volume by ID"))
	_, err = d.CreateSnapshot(ctx(), snapConfig, nil)
	assert.ErrorContains(t, err, "failed to get volume by ID", "Volume found")
}

func TestCreateSnapshot_SnapshotCreateFailed(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	volume := &api.Volume{VolumeID: "123"}
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(true, volume, nil)
	gcpClient.EXPECT().GetVolumeByID(ctx(), gomock.Any()).Return(volume, nil)
	gcpClient.EXPECT().CreateSnapshot(ctx(), gomock.Any()).Return(errors.New("snapshot creation failed"))

	_, err := d.CreateSnapshot(ctx(), snapConfig, nil)
	assert.ErrorContains(t, err, "snapshot creation failed", "Snapshot created")
}

func TestCreateSnapshot_GetSnapshotFailed(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	volume := &api.Volume{VolumeID: "123"}
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(true, volume, nil)
	gcpClient.EXPECT().GetVolumeByID(ctx(), gomock.Any()).Return(volume, nil)
	gcpClient.EXPECT().CreateSnapshot(ctx(), gomock.Any()).Return(nil)
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("failed to get snapshot"))

	_, err := d.CreateSnapshot(ctx(), snapConfig, nil)
	assert.ErrorContains(t, err, "failed to get snapshot", "Snapshot created")
}

func TestCreateSnapshot_WaitForSnapshotFailed(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	volume := &api.Volume{VolumeID: "123"}
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(true, volume, nil)
	gcpClient.EXPECT().GetVolumeByID(ctx(), gomock.Any()).Return(volume, nil)
	gcpClient.EXPECT().CreateSnapshot(ctx(), gomock.Any()).Return(nil)
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).Return(nil, nil)
	gcpClient.EXPECT().WaitForSnapshotState(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errors.New("failed to get snapshot state"))

	_, err := d.CreateSnapshot(ctx(), snapConfig, nil)
	assert.ErrorContains(t, err, "failed to get snapshot state", "Snapshot created")
}

func TestCreateSnapshot(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	volume := &api.Volume{VolumeID: "123"}
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(true, volume, nil)
	gcpClient.EXPECT().GetVolumeByID(ctx(), gomock.Any()).Return(volume, nil)
	gcpClient.EXPECT().CreateSnapshot(ctx(), gomock.Any()).Return(nil)
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).Return(&api.Snapshot{}, nil)
	gcpClient.EXPECT().WaitForSnapshotState(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	snap, err := d.CreateSnapshot(ctx(), snapConfig, nil)
	assert.NoError(t, err, "Snapshot creation failed")
	assert.NotNil(t, snap, "Snapshot is empty")
}

func TestRestoreSnapshot_GetVolumeError(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(nil, errors.New("failed to get volume"))

	err := d.RestoreSnapshot(ctx(), snapConfig, nil)
	assert.ErrorContains(t, err, "failed to get volume", "Found volume")
}

func TestRestoreSnapshot_GetSnapshotError(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&api.Volume{}, nil)
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("failed to get snapshot"))

	err := d.RestoreSnapshot(ctx(), snapConfig, nil)
	assert.ErrorContains(t, err, "failed to get snapshot", "Found snapshot")
}

func TestRestoreSnapshot(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&api.Volume{}, nil)
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).Return(nil, nil)
	gcpClient.EXPECT().RestoreSnapshot(ctx(), gomock.Any(), gomock.Any()).Return(nil)

	err := d.RestoreSnapshot(ctx(), snapConfig, nil)
	assert.NoError(t, err, "Snapshot not restored")
}

func TestDeleteSnapshot_VolumeExistsErrors(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	// Get existing volume by token API error
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).
		Return(false, nil, errors.New("failed to get volume"))
	err := d.DeleteSnapshot(ctx(), snapConfig, nil)
	assert.ErrorContains(t, err, "failed to get volume", "Volume found")

	// Volume whose snapshot we need to delete, doesn't exist
	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(false, nil, nil)
	err = d.DeleteSnapshot(ctx(), snapConfig, nil)
	assert.NoError(t, err, "Volume found")
}

func TestDeleteSnapshot_GetSnapshotError(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(true, nil, nil)
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("failed to get snapshot"))

	err := d.DeleteSnapshot(ctx(), snapConfig, nil)
	assert.ErrorContains(t, err, "failed to get snapshot", "Snapshot found")
}

func TestDeleteSnapshot_DeleteAPIError(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(true, nil, nil)
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).Return(nil, nil)
	gcpClient.EXPECT().DeleteSnapshot(ctx(), gomock.Any(), gomock.Any()).Return(errors.New("failed to delete snapshot"))

	err := d.DeleteSnapshot(ctx(), snapConfig, nil)
	assert.ErrorContains(t, err, "failed to delete snapshot", "Snapshot got deleted")
}

func TestDeleteSnapshot(t *testing.T) {
	snapConfig, _ := getSnapshotVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().VolumeExistsByCreationToken(ctx(), gomock.Any()).Return(true, nil, nil)
	gcpClient.EXPECT().GetSnapshotForVolume(ctx(), gomock.Any(), gomock.Any()).Return(nil, nil)
	gcpClient.EXPECT().DeleteSnapshot(ctx(), gomock.Any(), gomock.Any()).Return(nil)
	gcpClient.EXPECT().WaitForSnapshotState(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	err := d.DeleteSnapshot(ctx(), snapConfig, nil)
	assert.NoError(t, err, "Snapshot deletion failed")
}

func getVolumes() *[]api.Volume {
	vol1 := api.Volume{CreationToken: "test_vol1", LifeCycleState: api.StateAvailable, QuotaInBytes: 1000}
	vol2 := api.Volume{CreationToken: "test_vol2", LifeCycleState: api.StateError, QuotaInBytes: 2000}
	vol3 := api.Volume{CreationToken: "vol3", LifeCycleState: api.StateAvailable, QuotaInBytes: 3000}
	vols := []api.Volume{vol1, vol2, vol3}
	return &vols
}

func TestList_GetVolumesError(t *testing.T) {
	gcpClient, d := newMockGCPDriver(t)
	gcpClient.EXPECT().GetVolumes(ctx()).Return(nil, errors.New("failed to get volumes"))
	_, err := d.List(ctx())
	assert.ErrorContains(t, err, "failed to get volumes", "Found volumes")
}

func TestList(t *testing.T) {
	gcpClient, d := newMockGCPDriver(t)
	gcpClient.EXPECT().GetVolumes(ctx()).Return(getVolumes(), nil)
	volumes, err := d.List(ctx())
	assert.NoError(t, err, "Failed to get volumes")
	assert.Equal(t, volumes, []string{"vol1"}, "Found unexpected volumes in the result")
}

func TestGet(t *testing.T) {
	gcpClient, d := newMockGCPDriver(t)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(nil, nil)
	err := d.Get(ctx(), "vol1")
	assert.NoError(t, err, "Error getting volume")
}

func TestResize_GetVolumeError(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(nil, errors.New("failed to get volume"))
	err := d.Resize(ctx(), volConfig, 100)
	assert.ErrorContains(t, err, "failed to get volume", "Found volume")
}

func TestResize_VolumeStateError(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	volume := (*getVolumes())[1]
	gcpClient, d := newMockGCPDriver(t)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&volume, nil)
	err := d.Resize(ctx(), volConfig, 100)
	assert.ErrorContains(t, err, "not available", "Found volume")
}

func TestResize_NoSizeChange(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	volume := (*getVolumes())[0]
	gcpClient, d := newMockGCPDriver(t)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&volume, nil)
	err := d.Resize(ctx(), volConfig, uint64(volume.QuotaInBytes))
	assert.NoError(t, err, "Resize failed")
}

func TestResize_UnsupportedCapacity(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	volume := (*getVolumes())[0]
	gcpClient, d := newMockGCPDriver(t)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&volume, nil)
	err := d.Resize(ctx(), volConfig, uint64(volume.QuotaInBytes)-1)
	isErr, _ := errors.HasUnsupportedCapacityRangeError(err)
	assert.True(t, isErr, "Valid new volume size")
}

func TestResize_VolumeSizeLimitError(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	volume := (*getVolumes())[0]
	gcpClient, d := newMockGCPDriver(t)

	d.Config.LimitVolumeSize = fmt.Sprintf("%v", uint64(volume.QuotaInBytes))
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&volume, nil)

	err := d.Resize(ctx(), volConfig, uint64(volume.QuotaInBytes)+10)
	assert.ErrorContains(t, err, "size limit", "Valid new volume size")
}

func TestResize_ResizeAPIError(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	volume := (*getVolumes())[0]
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&volume, nil)
	gcpClient.EXPECT().ResizeVolume(ctx(), gomock.Any(), gomock.Any()).Return(nil, errors.New("resize failed"))

	err := d.Resize(ctx(), volConfig, uint64(volume.QuotaInBytes)+10)
	assert.ErrorContains(t, err, "resize failed", "Resize call succeeded")
}

func TestResize_WaitForVolumeStateError(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	volume := (*getVolumes())[0]
	gcpClient, d := newMockGCPDriver(t)

	// Wait for volume state error
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&volume, nil)
	gcpClient.EXPECT().ResizeVolume(ctx(), gomock.Any(), gomock.Any()).Return(nil, nil)
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", errors.New("wait for volume state failed"))

	err := d.Resize(ctx(), volConfig, uint64(volume.QuotaInBytes)+10)
	assert.ErrorContains(t, err, "wait for volume state failed", "Resize call succeeded")

	// Wait for volume state success
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&volume, nil)
	gcpClient.EXPECT().ResizeVolume(ctx(), gomock.Any(), gomock.Any()).Return(nil, nil)
	gcpClient.EXPECT().WaitForVolumeStates(ctx(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", nil)

	err = d.Resize(ctx(), volConfig, uint64(volume.QuotaInBytes)+10)
	assert.NoError(t, err, "Resize failed")
}

func TestGetStorageBackendSpecs(t *testing.T) {
	d := newTestGCPDriver(nil)
	backend, _ := storage.NewStorageBackend(ctx(), d)
	err := d.GetStorageBackendSpecs(ctx(), backend)
	assert.NoError(t, err, "Failed to update storage backend specs")
}

func TestCreatePrepare(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	d := newTestGCPDriver(nil)
	pool := storage.NewStoragePool(nil, "pool1")
	d.CreatePrepare(ctx(), volConfig, pool)
	name := d.GetInternalVolumeName(ctx(), volConfig, pool)
	assert.Equal(t, volConfig.InternalName, name, "Different internal name for volume")
}

func TestGetStorageBackendPhysicalPoolNames(t *testing.T) {
	d := newTestGCPDriver(nil)
	poolNames := d.GetStorageBackendPhysicalPoolNames(ctx())
	assert.Equal(t, len(poolNames), 0, "Found physical pools")
}

func TestGetStorageBackendPools(t *testing.T) {
	_, d := newMockGCPDriver(t)
	d.pools = map[string]storage.Pool{
		"pool1": storage.NewStoragePool(nil, "pool1"),
		"pool2": storage.NewStoragePool(nil, "pool2"),
	}

	pools := d.getStorageBackendPools(context.Background())
	assert.NotEmpty(t, pools)
	assert.Equal(t, len(d.pools), len(pools))

	d.pools = nil

	pools = d.getStorageBackendPools(context.Background())
	assert.Empty(t, pools)
	assert.Equal(t, len(d.pools), len(pools))
}

func TestGetInternalVolumeName(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	d := newTestGCPDriver(nil)
	pool := storage.NewStoragePool(nil, "pool1")

	// Using pass through store
	tridentconfig.UsingPassthroughStore = true
	volConfig.Name = "test-vol"
	name := d.GetInternalVolumeName(ctx(), volConfig, pool)
	assert.Equal(t, name, "test_test-vol", "Wrong internal name")

	// Using existing UUID as volume name
	tridentconfig.UsingPassthroughStore = false
	volConfig.Name = "pvc-9d77b6f75f24"
	name = d.GetInternalVolumeName(ctx(), volConfig, pool)
	assert.Equal(t, name, "pvc-9d77b6f75f24", "Wrong internal name")
}

func TestCreateFollowup_GetVolumeByTokenError(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	gcpClient, d := newMockGCPDriver(t)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(nil, errors.New("failed to get volume"))
	err := d.CreateFollowup(ctx(), volConfig)
	assert.ErrorContains(t, err, "failed to get volume", "Found volume")
}

func TestCreateFollowup_ErrorStateVolume(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	volume := (*getVolumes())[1]
	gcpClient, d := newMockGCPDriver(t)

	volume.LifeCycleStateDetails = "error while creating"
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&volume, nil)

	err := d.CreateFollowup(ctx(), volConfig)
	assert.ErrorContains(t, err, volume.LifeCycleStateDetails, "Found volume")
}

func TestCreateFollowup_VolumeMountPoints(t *testing.T) {
	volConfig, _ := getCreateVolumeStructs()
	volume := (*getVolumes())[0]
	gcpClient, d := newMockGCPDriver(t)

	// Zero volume mount points
	volume.MountPoints = []api.MountPoint{}
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&volume, nil)
	err := d.CreateFollowup(ctx(), volConfig)
	assert.ErrorContains(t, err, "has no mount points", "Volume has mount points")

	// Valid volume mount points
	volume.MountPoints = []api.MountPoint{{Server: "0.0.0.0", Export: "/tmp"}}
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&volume, nil)
	err = d.CreateFollowup(ctx(), volConfig)
	assert.NoError(t, err, "Create followup failed")
	assert.Equal(t, volConfig.AccessInfo.NfsServerIP, "0.0.0.0", "Incorrect server IP")
	assert.Equal(t, volConfig.AccessInfo.NfsPath, "/tmp", "Incorrect nfs path")
	assert.Equal(t, volConfig.FileSystem, "nfs", "Incorrect filesystem")
}

func TestGetProtocol(t *testing.T) {
	d := newTestGCPDriver(nil)
	assert.Equal(t, d.GetProtocol(ctx()), tridentconfig.File, "Incorrect protocol")
}

func TestStoreConfig(t *testing.T) {
	backendConfig := storage.PersistentStorageBackendConfig{}
	d := newTestGCPDriver(nil)
	d.StoreConfig(ctx(), &backendConfig)
	assert.Equal(t, backendConfig.GCPConfig, &d.Config, "Stored config is different")
}

func TestGetExternalConfig(t *testing.T) {
	d := newTestGCPDriver(nil)
	config := d.GetExternalConfig(ctx())
	assert.Equal(t, config.(drivers.GCPNFSStorageDriverConfig).StorageDriverName,
		d.Config.StorageDriverName, "Incorrect config")
}

func TestGetVolumeForImport_GetVolumeError(t *testing.T) {
	gcpClient, d := newMockGCPDriver(t)
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(nil, errors.New("failed to get volume"))
	_, err := d.GetVolumeForImport(ctx(), "vol-name")
	assert.ErrorContains(t, err, "failed to get volume", "Found volume")
}

func TestGetVolumeForImport(t *testing.T) {
	gcpClient, d := newMockGCPDriver(t)
	volume := api.Volume{
		Name:              "vol-name",
		CreationToken:     "vol-name",
		ServiceLevel:      api.APIServiceLevel1,
		SnapshotDirectory: false,
		QuotaInBytes:      100,
	}
	gcpClient.EXPECT().GetVolumeByCreationToken(ctx(), gomock.Any()).Return(&volume, nil)
	extVol, err := d.GetVolumeForImport(ctx(), "vol-name")
	assert.NoError(t, err, "Failed to get external volume")
	assert.Equal(t, extVol.Config.Name, volume.Name)
}

func TestGetVolumeExternalWrappers_GetVolumeError(t *testing.T) {
	channel := make(chan *storage.VolumeExternalWrapper)
	gcpClient, d := newMockGCPDriver(t)
	gcpClient.EXPECT().GetVolumes(ctx()).Return(nil, errors.New("failed to get volumes"))
	go d.GetVolumeExternalWrappers(ctx(), channel)
	volumeWrapper, _ := <-channel
	assert.ErrorContains(t, volumeWrapper.Error, "failed to get volumes", "Found volumes")
}

func TestGetVolumeExternalWrappers(t *testing.T) {
	channel := make(chan *storage.VolumeExternalWrapper)
	gcpClient, d := newMockGCPDriver(t)

	gcpClient.EXPECT().GetVolumes(ctx()).Return(getVolumes(), nil)

	go d.GetVolumeExternalWrappers(ctx(), channel)
	volumeWrapper, _ := <-channel
	assert.NoError(t, volumeWrapper.Error, "Failed to get volumes")
}

func TestGetUpdateType_InvalidUpdate(t *testing.T) {
	fakeDriverConfig := drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			DebugTraceFlags: map[string]bool{"method": true},
		},
	}

	fakeDriver := fake.NewFakeStorageDriver(ctx(), fakeDriverConfig)
	d := newTestGCPDriver(nil)

	bitmap := d.GetUpdateType(ctx(), fakeDriver)
	assert.True(t, bitmap.Contains(storage.InvalidUpdate), "Not an invalid update")
}

func TestGetUpdate(t *testing.T) {
	d1 := newTestGCPDriver(nil)
	d2 := newTestGCPDriver(nil)

	*d2.Config.StoragePrefix = "trident"
	d2.Config.Credentials = map[string]string{"key1": "value1"}

	bitmap := d1.GetUpdateType(ctx(), d2)
	assert.True(t, bitmap.Contains(storage.PrefixChange))
	assert.True(t, bitmap.Contains(storage.CredentialsChange))
}

func TestReconcileNodeAccess(t *testing.T) {
	d := newTestGCPDriver(nil)
	node1 := utils.Node{Name: "node-name"}
	nodes := []*utils.Node{&node1}
	err := d.ReconcileNodeAccess(ctx(), nodes, "", "")
	assert.NoError(t, err, "Reconcile node access failed")
}

func TestGetCommonConfig(t *testing.T) {
	d := newTestGCPDriver(nil)
	config := d.GetCommonConfig(ctx())
	assert.Equal(t, config, d.Config.CommonStorageDriverConfig, "Configs not same")
}

func TestGetAPI(t *testing.T) {
	d := newTestGCPDriver(nil)
	assert.Equal(t, d.GetAPI(), d.API, "GCP clients not same")
}

func TestDefaultTimeout_DockerContext(t *testing.T) {
	d := newTestGCPDriver(nil)
	d.Config.DriverContext = tridentconfig.ContextDocker

	testTime := d.defaultTimeout()
	assert.Equal(t, testTime, tridentconfig.DockerDefaultTimeout, "Incorrect timout")

	testTime = d.defaultCreateTimeout()
	assert.Equal(t, testTime, tridentconfig.DockerCreateTimeout, "Incorrect timeout")
}
