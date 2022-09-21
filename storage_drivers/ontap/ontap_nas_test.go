// Copyright 2021 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	mockapi "github.com/netapp/trident/mocks/mock_storage_drivers/mock_ontap"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/api"
)

// //////////////////////////////////////////////////////////////////////////////////////////
// /             _____________________
// /            |   <<Interface>>    |
// /            |       ONTAPI       |
// /            |____________________|
// /                ^             ^
// /     Implements |             | Implements
// /   ____________________    ____________________
// /  |  ONTAPAPIREST     |   |  ONTAPAPIZAPI     |
// /  |___________________|   |___________________|
// /  | +API: RestClient  |   | +API: *Client     |
// /  |___________________|   |___________________|
// /
// //////////////////////////////////////////////////////////////////////////////////////////

// //////////////////////////////////////////////////////////////////////////////////////////
// Drivers that offer dual support are to call ONTAP REST or ZAPI's
// via abstraction layer (ONTAPI interface)
// //////////////////////////////////////////////////////////////////////////////////////////

var (
	ctx             = context.TODO()
	debugTraceFlags = map[string]bool{"method": true, "api": true, "discovery": true}
)

const (
	BackendUUID                 = "deadbeef-03af-4394-ace4-e177cdbcaf28"
	ONTAPTEST_LOCALHOST         = "127.0.0.1"
	ONTAPTEST_VSERVER_AGGR_NAME = "data"
)

func TestOntapNasStorageDriverConfigString(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	ontapNasDrivers := []NASStorageDriver{
		*newTestOntapNASDriver(vserverAdminHost, vserverAdminPort, vserverAggrName,
			tridentconfig.DriverContext("CSI"), true),
		*newTestOntapNASDriver(vserverAdminHost, vserverAdminPort, vserverAggrName,
			tridentconfig.DriverContext("CSI"), false),
	}

	sensitiveIncludeList := map[string]string{
		"username":        "ontap-nas-user",
		"password":        "password1!",
		"client username": "client_username",
		"client password": "client_password",
	}

	externalIncludeList := map[string]string{
		"<REDACTED>":                   "<REDACTED>",
		"username":                     "Username:<REDACTED>",
		"password":                     "Password:<REDACTED>",
		"api":                          "API:<REDACTED>",
		"chap username":                "ChapUsername:<REDACTED>",
		"chap initiator secret":        "ChapInitiatorSecret:<REDACTED>",
		"chap target username":         "ChapTargetUsername:<REDACTED>",
		"chap target initiator secret": "ChapTargetInitiatorSecret:<REDACTED>",
		"client private key":           "ClientPrivateKey:<REDACTED>",
	}

	for _, ontapNasDriver := range ontapNasDrivers {
		for key, val := range externalIncludeList {
			assert.Contains(t, ontapNasDriver.String(), val, "ontap-nas driver does not contain %v", key)
			assert.Contains(t, ontapNasDriver.GoString(), val, "ontap-nas driver does not contain %v", key)
		}

		for key, val := range sensitiveIncludeList {
			assert.NotContains(t, ontapNasDriver.String(), val, "ontap-nas driver contains %v", key)
			assert.NotContains(t, ontapNasDriver.GoString(), val, "ontap-nas driver contains %v", key)
		}
	}
}

func newTestOntapNASDriver(
	vserverAdminHost, vserverAdminPort, vserverAggrName string, driverContext tridentconfig.DriverContext, useREST bool,
) *NASStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true
	// config.Labels = map[string]string{"app": "wordpress"}
	config.ManagementLIF = vserverAdminHost + ":" + vserverAdminPort
	config.SVM = "SVM1"
	config.Aggregate = vserverAggrName
	config.Username = "ontap-nas-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-nas"
	config.StoragePrefix = sp("test_")
	config.DriverContext = driverContext
	config.UseREST = useREST

	nasDriver := &NASStorageDriver{}
	nasDriver.Config = *config

	var ontapAPI api.OntapAPI

	nasDriver.API = ontapAPI
	nasDriver.telemetry = &Telemetry{
		Plugin:        nasDriver.Name(),
		SVM:           config.SVM,
		StoragePrefix: *nasDriver.GetConfig().StoragePrefix,
		Driver:        nasDriver,
	}

	return nasDriver
}

func TestInitializeStoragePoolsLabels(t *testing.T) {
	ctx := context.Background()

	mockCtrl := gomock.NewController(t)
	mockAPI := mockapi.NewMockOntapAPI(mockCtrl)
	mockAPI.EXPECT().SVMName().AnyTimes().Return("SVM1")
	mockAPI.EXPECT().GetSVMAggregateNames(gomock.Any()).AnyTimes().Return([]string{ONTAPTEST_VSERVER_AGGR_NAME}, nil)
	mockAPI.EXPECT().GetSVMAggregateAttributes(gomock.Any()).AnyTimes().Return(
		map[string]string{ONTAPTEST_VSERVER_AGGR_NAME: "vmdisk"}, nil,
	)

	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME

	d := newTestOntapNASDriver(vserverAdminHost, "443", vserverAggrName, tridentconfig.DriverContext("CSI"),
		false)
	d.API = mockAPI
	d.Config.Storage = []drivers.OntapStorageDriverPool{
		{
			Region: "us_east_1",
			Zone:   "us_east_1a",
			SupportedTopologies: []map[string]string{
				{
					"topology.kubernetes.io/region": "us_east_1",
					"topology.kubernetes.io/zone":   "us_east_1a",
				},
			},
		},
	}

	poolAttributes := map[string]sa.Offer{
		sa.BackendType:      sa.NewStringOffer(d.Name()),
		sa.Snapshots:        sa.NewBoolOffer(true),
		sa.Clones:           sa.NewBoolOffer(true),
		sa.Encryption:       sa.NewBoolOffer(true),
		sa.ProvisioningType: sa.NewStringOffer("thick", "thin"),
	}

	cases := []struct {
		physicalPoolLabels   map[string]string
		virtualPoolLabels    map[string]string
		physicalExpected     string
		virtualExpected      string
		backendName          string
		physicalErrorMessage string
		virtualErrorMessage  string
	}{
		{
			nil, nil, "", "", "nas-backend",
			"Label is not empty", "Label is not empty",
		}, // no labels
		{
			map[string]string{"base-key": "base-value"},
			nil,
			`{"provisioning":{"base-key":"base-value"}}`,
			`{"provisioning":{"base-key":"base-value"}}`, "nas-backend",
			"Base label is not set correctly", "Base label is not set correctly",
		}, // base label only
		{
			nil,
			map[string]string{"virtual-key": "virtual-value"},
			"",
			`{"provisioning":{"virtual-key":"virtual-value"}}`, "nas-backend",
			"Base label is not empty", "Virtual pool label is not set correctly",
		}, // virtual label only
		{
			map[string]string{"base-key": "base-value"},
			map[string]string{"virtual-key": "virtual-value"},
			`{"provisioning":{"base-key":"base-value"}}`,
			`{"provisioning":{"base-key":"base-value","virtual-key":"virtual-value"}}`,
			"nas-backend",
			"Base label is not set correctly", "Virtual pool label is not set correctly",
		}, // base and virtual labels
	}

	for _, c := range cases {
		d.Config.Labels = c.physicalPoolLabels
		d.Config.Storage[0].Labels = c.virtualPoolLabels
		physicalPools, virtualPools, err := InitializeStoragePoolsCommon(ctx, d, poolAttributes,
			c.backendName)
		assert.Nil(t, err, "Error is not nil")

		physicalPool := physicalPools["data"]
		label, err := physicalPool.GetLabelsJSON(ctx, "provisioning", 1023)
		assert.Nil(t, err, "Error is not nil")
		assert.Equal(t, c.physicalExpected, label, c.physicalErrorMessage)

		virtualPool := virtualPools["nas-backend_pool_0"]
		label, err = virtualPool.GetLabelsJSON(ctx, "provisioning", 1023)
		assert.Nil(t, err, "Error is not nil")
		assert.Equal(t, c.virtualExpected, label, c.virtualErrorMessage)
	}
}

func TestOntapNasStorageDriverInitialize_WithTwoAuthMethods(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME
	ctx := context.TODO()
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas",
		BackendName:       "myOntapNasBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas",
		"managementLIF":     "1.1.1.1:10",
		"svm":               "SVM1",
		"aggregate":         "data",
		"username":          "dummyuser",
		"password":          "dummypassword",
		"clientcertificate": "dummy-certificate",
		"clientprivatekey":  "dummy-client-private-key"
	}`
	ontapNasDriver := newTestOntapNASDriver(vserverAdminHost, vserverAdminPort, vserverAggrName,
		tridentconfig.DriverContext("CSI"), false)

	result := ontapNasDriver.Initialize(ctx, tridentconfig.DriverContext("CSI"), configJSON, commonConfig, map[string]string{}, BackendUUID)

	assert.Error(t, result, "driver initialization succeeded despite more than one authentication methods in config")
	assert.Contains(t, result.Error(), "more than one authentication method", "expected error string not found")
}

func TestOntapNasStorageDriverInitialize_WithTwoAuthMethodsWithSecrets(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME
	ctx := context.TODO()
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas",
		BackendName:       "myOntapNasBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas",
		"managementLIF":     "1.1.1.1:10",
		"svm":               "SVM1",
		"aggregate":         "data"
	}`
	secrets := map[string]string{
		"username":          "dummyuser",
		"password":          "dummypassword",
		"clientprivatekey":  "dummy-client-private-key",
		"clientcertificate": "dummy-certificate",
	}
	ontapNasDriver := newTestOntapNASDriver(vserverAdminHost, vserverAdminPort, vserverAggrName,
		tridentconfig.DriverContext("CSI"), false)

	result := ontapNasDriver.Initialize(ctx, tridentconfig.DriverContext("CSI"), configJSON, commonConfig, secrets,
		BackendUUID)

	assert.Error(t, result, "driver initialization succeeded despite more than one authentication methods in config")
	assert.Contains(t, result.Error(), "more than one authentication method", "expected error string not found")
}

func TestOntapNasStorageDriverInitialize_WithTwoAuthMethodsWithConfigAndSecrets(t *testing.T) {
	vserverAdminHost := ONTAPTEST_LOCALHOST
	vserverAdminPort := "0"
	vserverAggrName := ONTAPTEST_VSERVER_AGGR_NAME
	ctx := context.TODO()
	commonConfig := &drivers.CommonStorageDriverConfig{
		Version:           1,
		StorageDriverName: "ontap-nas",
		BackendName:       "myOntapNasBackend",
		DriverContext:     tridentconfig.ContextCSI,
		DebugTraceFlags:   debugTraceFlags,
	}

	configJSON := `
	{
		"version":           1,
		"storageDriverName": "ontap-nas",
		"managementLIF":     "1.1.1.1:10",
		"svm":               "SVM1",
		"aggregate":         "data",
		"username":          "dummyuser",
		"password":          "dummypassword"
	}`
	secrets := map[string]string{
		"clientprivatekey":  "dummy-client-private-key",
		"clientcertificate": "dummy-certificate",
	}
	ontapNasDriver := newTestOntapNASDriver(vserverAdminHost, vserverAdminPort, vserverAggrName,
		tridentconfig.DriverContext("CSI"), false)

	result := ontapNasDriver.Initialize(ctx, tridentconfig.DriverContext("CSI"), configJSON, commonConfig, secrets,
		BackendUUID)

	assert.Error(t, result, "driver initialization succeeded despite more than one authentication methods in config")
	assert.Contains(t, result.Error(), "more than one authentication method", "expected error string not found")
}
