// Copyright 2021 NetApp, Inc. All Rights Reserved.

package gcp

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"testing"

	"github.com/golang/mock/gomock"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	mockGCPClient "github.com/netapp/trident/mocks/mock_storage_drivers/mock_gcp"
	"github.com/netapp/trident/storage"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/gcp/api"
	"github.com/netapp/trident/utils"
)

const (
	ProjectNumber     = "123456"
	PrivateKeyId      = "12345678987654321"
	PrivateKey        = "-----BEGIN PRIVATE KEY-----AAAABBBCCCCDDDDDDEEEEEEFFFF----END PRIVATE KEY-----"
	ClientEmail       = "random@random.com"
	ClientID          = "98765432123456789"
	ClientX509CertURL = "https://random.com/x509Cert"
)

var ctx = context.Background

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}

func newTestGCPDriver() *NFSStorageDriver {
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

	API := api.NewDriver(api.ClientConfig{
		ProjectNumber:   config.ProjectNumber,
		APIKey:          config.APIKey,
		APIRegion:       config.APIRegion,
		ProxyURL:        config.ProxyURL,
		DebugTraceFlags: config.DebugTraceFlags,
	})

	GCPDriver := &NFSStorageDriver{}
	GCPDriver.Config = *config
	GCPDriver.API = API
	GCPDriver.apiVersion = utils.MustParseSemantic("20.5.1")
	GCPDriver.sdeVersion = utils.MustParseSemantic("20.6.2")
	GCPDriver.tokenRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,79}$`)
	GCPDriver.csiRegexp = regexp.MustCompile(`^pvc-[0-9a-fA-F]{12}$`)
	GCPDriver.apiRegions = []string{"us-central-9", "us-central-10"}

	return GCPDriver
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
		StorageClass:    api.PoolServiceLevel1,
		NumberOfVolumes: 2,
		ServiceLevel:    api.PoolServiceLevel1,
		SizeInBytes:     1000,
		AllocatedBytes:  500,
	}
	pool2 := &api.Pool{
		PoolID:          "abc2",
		Name:            "pool2",
		StorageClass:    api.PoolServiceLevel1,
		NumberOfVolumes: 1,
		ServiceLevel:    api.PoolServiceLevel2,
		SizeInBytes:     1000,
		AllocatedBytes:  900,
	}
	pool3 := &api.Pool{
		PoolID:          "abc3",
		Name:            "pool3",
		StorageClass:    api.PoolServiceLevel2,
		NumberOfVolumes: 50,
		ServiceLevel:    api.PoolServiceLevel1,
		SizeInBytes:     1000,
		AllocatedBytes:  500,
	}
	pool4 := &api.Pool{
		PoolID:          "abc4",
		Name:            "pool4",
		StorageClass:    api.PoolServiceLevel2,
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
		*newTestGCPDriver(),
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
	driver := newTestGCPDriver()

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
	d := newTestGCPDriver()

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
			err := d.initializeStoragePools(ctx)
			assert.Nil(t, err, "Error is not nil")

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
	d := newTestGCPDriver()

	d.Config.UnixPermissions = "0111"
	d.Config.Storage = []drivers.GCPNFSStorageDriverPool{
		{
			Region: "us_east_1",
			Zone:   "us_east_1a",
		},
	}
	err := d.initializeStoragePools(ctx)
	assert.Nil(t, err, "Error is not nil")

	virtualPool := d.pools["gcpcvs_12345_pool_0"]
	assert.Equal(t, "0111", virtualPool.InternalAttributes()[UnixPermissions])

	// 2. Test with no vpool defined
	ctx = context.Background()
	d = newTestGCPDriver()

	d.Config.UnixPermissions = "0111"
	err = d.initializeStoragePools(ctx)
	assert.Nil(t, err, "Error is not nil")

	virtualPool = d.pools["gcpcvs_12345_pool"]
	assert.NotNil(t, virtualPool, "Could not find pool")
	assert.NotNil(t, virtualPool.InternalAttributes(), "Could not find pool attributes")
	assert.Equal(t, "0111", virtualPool.InternalAttributes()[UnixPermissions])

	// 3. Test with multiple vpools defined, one with unix permissions set
	ctx = context.Background()
	d = newTestGCPDriver()

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
	err = d.initializeStoragePools(ctx)
	assert.Nil(t, err, "Error is not nil")

	virtualPool = d.pools["gcpcvs_12345_pool_0"]
	assert.Equal(t, "0111", virtualPool.InternalAttributes()[UnixPermissions])

	virtualPool2 := d.pools["gcpcvs_12345_pool_1"]
	assert.Equal(t, "0222", virtualPool2.InternalAttributes()[UnixPermissions])

	// 4. Test defaults
	ctx = context.Background()
	d = newTestGCPDriver()

	err = d.populateConfigurationDefaults(ctx, &d.Config)
	assert.Nil(t, err, "Error is not nil")
	err = d.initializeStoragePools(ctx)
	assert.Nil(t, err, "Error is not nil")

	virtualPool = d.pools["gcpcvs_12345_pool"]
	assert.NotNil(t, virtualPool, "Could not find pool")
	assert.NotNil(t, virtualPool.InternalAttributes(), "Could not find pool attributes")
	assert.Equal(t, "0777", virtualPool.InternalAttributes()[UnixPermissions])
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
			d := newTestGCPDriver()
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
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			gcpClient := mockGCPClient.NewMockGCPClient(mockCtrl)
			tt.mockFunc(gcpClient)

			sPool.InternalAttributes()[StoragePools] = tt.poolConfig
			sPool.InternalAttributes()[StorageClass] = tt.storageClass

			d := newTestGCPDriver()
			d.API = gcpClient

			gPool, err := d.GetPoolsForCreate(ctx(), sPool, api.PoolServiceLevel1, tt.volSize)
			tt.wantErr(t, err, "Unexpected error")
			fmt.Println(err)
			if err == nil && tt.Name != "GetPoolsNoMatch" && tt.Name != "GetPoolsForPO" {
				assert.NotNil(t, gPool, "gPool is empty")
			}
		})
	}
}
