// Copyright 2019 NetApp, Inc. All Rights Reserved.

package ontap_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/common/log"
	"github.com/stretchr/testify/assert"

	tridentconfig "github.com/netapp/trident/config"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap"
	"github.com/netapp/trident/storage_drivers/ontap/api"
	"github.com/netapp/trident/storage_drivers/ontap/api/azgo"
)

// Copyright 2019 NetApp, Inc. All Rights Reserved.

func newTestOntapNASDriver() *ontap.NASStorageDriver {
	config := &drivers.OntapStorageDriverConfig{}
	sp := func(s string) *string { return &s }

	config.CommonStorageDriverConfig = &drivers.CommonStorageDriverConfig{}
	config.CommonStorageDriverConfig.DebugTraceFlags = make(map[string]bool)
	config.CommonStorageDriverConfig.DebugTraceFlags["method"] = true

	config.ManagementLIF = "127.0.0.1"
	config.SVM = "SVM1"
	config.Aggregate = "aggr1"
	config.Username = "ontap-nas-user"
	config.Password = "password1!"
	config.StorageDriverName = "ontap-nas"
	config.StoragePrefix = sp("test_")

	nasDriver := &ontap.NASStorageDriver{}
	nasDriver.Config = *config

	// ClientConfig holds the configuration data for Client objects
	clientConfig := api.ClientConfig{
		ManagementLIF:           config.ManagementLIF,
		SVM:                     "SVM1",
		Username:                "client_username",
		Password:                "client_password",
		DriverContext:           tridentconfig.DriverContext("driverContext"),
		ContextBasedZapiRecords: 100,
		DebugTraceFlags:         nil,
	}

	nasDriver.API = api.NewClient(clientConfig)
	nasDriver.Telemetry = &ontap.Telemetry{
		Plugin:        nasDriver.Name(),
		SVM:           nasDriver.GetConfig().SVM,
		StoragePrefix: *nasDriver.GetConfig().StoragePrefix,
		Driver:        nasDriver,
	}

	return nasDriver
}

func TestOntapNasStorageDriverConfigString(t *testing.T) {

	var ontapNasDrivers = []ontap.NASStorageDriver{
		*newTestOntapNASDriver(),
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
			assert.Contains(t, ontapNasDriver.String(), val,
				"ontap-nas driver does not contain %v", key)
			assert.Contains(t, ontapNasDriver.GoString(), val,
				"ontap-nas driver does not contain %v", key)
		}

		for key, val := range sensitiveIncludeList {
			assert.NotContains(t, ontapNasDriver.String(), val,
				"ontap-nas driver contains %v", key)
			assert.NotContains(t, ontapNasDriver.GoString(), val,
				"ontap-nas driver contains %v", key)
		}
	}
}

func newTestOntapNASDriverTLS(vserverAdminHost, vserverAdminPort, vserverAggrName string) *ontap.NASStorageDriver {
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

	nasDriver := &ontap.NASStorageDriver{}
	nasDriver.Config = *config

	// ClientConfig holds the configuration data for Client objects
	clientConfig := api.ClientConfig{
		ManagementLIF:           config.ManagementLIF,
		SVM:                     "SVM1",
		Username:                "client_username",
		Password:                "client_password",
		DriverContext:           tridentconfig.DriverContext("driverContext"),
		ContextBasedZapiRecords: 100,
		DebugTraceFlags:         nil,
	}

	nasDriver.API = api.NewClient(clientConfig)
	nasDriver.Telemetry = &ontap.Telemetry{
		Plugin:        nasDriver.Name(),
		SVM:           nasDriver.GetConfig().SVM,
		StoragePrefix: *nasDriver.GetConfig().StoragePrefix,
		Driver:        nasDriver,
	}

	return nasDriver
}

func newTestVserverGetIterResponse(vserverAdminHost, vserverAdminPort, vserverAggrName string) *azgo.
	VserverGetIterResponse {

	vserverAdminUrl := "https://" + vserverAdminHost + ":" + vserverAdminPort + "/filer/admin"

	xmlString := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
			<!DOCTYPE netapp SYSTEM "file:/etc/netapp_gx.dtd">

			<netapp xmlns="%s" version="%s">
			  <results status="passed">
				<attributes-list>
				  <vserver-info>
					<aggr-list>
					  <aggr-name>%s</aggr-name>
					</aggr-list>
					<allowed-protocols>
					  <protocol>nfs</protocol>
					  <protocol>cifs</protocol>
					  <protocol>fcp</protocol>
					  <protocol>iscsi</protocol>
					  <protocol>ndmp</protocol>
					</allowed-protocols>
					<antivirus-on-access-policy>default</antivirus-on-access-policy>
					<comment/>
					<ipspace>Default</ipspace>
					<is-config-locked-for-changes>false</is-config-locked-for-changes>
					<is-repository-vserver>false</is-repository-vserver>
					<is-space-enforcement-logical>false</is-space-enforcement-logical>
					<is-space-reporting-logical>false</is-space-reporting-logical>
					<is-vserver-protected>false</is-vserver-protected>
					<language>c.utf_8</language>
					<max-volumes>unlimited</max-volumes>
					<name-mapping-switch>
					  <nmswitch>file</nmswitch>
					</name-mapping-switch>
					<name-server-switch>
					  <nsswitch>file</nsswitch>
					</name-server-switch>
					<operational-state>running</operational-state>
					<quota-policy>default</quota-policy>
					<root-volume>root</root-volume>
					<root-volume-aggregate>%s</root-volume-aggregate>
					<root-volume-security-style>unix</root-volume-security-style>
					<snapshot-policy>default</snapshot-policy>
					<state>running</state>
					<uuid>7b9c12f2-bfdb-11ea-b366-005056b3362c</uuid>
					<volume-delete-retention-hours>12</volume-delete-retention-hours>
					<vserver-aggr-info-list>
					  <vserver-aggr-info>
						<aggr-availsize>600764416</aggr-availsize>
						<aggr-is-cft-precommit>false</aggr-is-cft-precommit>
						<aggr-name>%s</aggr-name>
					  </vserver-aggr-info>
					</vserver-aggr-info-list>
					<vserver-name>datavserver</vserver-name>
					<vserver-subtype>default</vserver-subtype>
					<vserver-type>data</vserver-type>
				  </vserver-info>
				</attributes-list>
				<num-records>1</num-records>
			  </results>
			</netapp>
				`,
		vserverAdminUrl, "1.170", vserverAggrName, vserverAggrName, vserverAggrName)

	var vserverGetIterResponse azgo.VserverGetIterResponse
	if unmarshalErr := xml.Unmarshal([]byte(xmlString), &vserverGetIterResponse); unmarshalErr != nil {
		fmt.Printf("error: %v", unmarshalErr.Error())
		return nil
	}

	return &vserverGetIterResponse
}

func newUnstartedVserver(vserverAdminHost, vserverAdminPort, vserverAggrName string) *httptest.Server {

	v := *newTestVserverGetIterResponse(vserverAdminHost, vserverAdminPort, vserverAggrName)
	output, err := xml.MarshalIndent(v, "  ", "    ")
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/servlets/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(output)
	})

	server := httptest.NewUnstartedServer(mux)

	listener, err := net.Listen("tcp", vserverAdminHost+":"+vserverAdminPort)
	if err != nil {
		log.Fatal(err)
	}
	server.Listener = listener
	return server
}

func TestInitializeStoragePoolsLabels(t *testing.T) {

	vserverAdminHost := "127.0.0.1"
	vserverAdminPort := "4443" // IT won't let you use 443 locally
	vserverAggrName := "data"

	server := newUnstartedVserver(vserverAdminHost, vserverAdminPort, vserverAggrName)
	server.StartTLS()

	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	assert.Nil(t, err, "Unable to get Web host port %s", port)

	defer func() {
		if r := recover(); r != nil {
			server.Close()
			log.Error("Panic in fake filer", r)
		}
	}()

	ctx := context.Background()
	d := newTestOntapNASDriverTLS(vserverAdminHost, vserverAdminPort, vserverAggrName)
	d.Config.Storage = []drivers.OntapStorageDriverPool{
		{
			Region: "us_east_1",
			Zone:   "us_east_1a",
			SupportedTopologies: []map[string]string{{
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
			map[string]string{"base-key": "base-value"}, nil,
			`{"provisioning":{"base-key":"base-value"}}`,
			`{"provisioning":{"base-key":"base-value"}}`, "nas-backend",
			"Base label is not set correctly", "Base label is not set correctly",
		}, // base label only
		{
			nil, map[string]string{"virtual-key": "virtual-value"},
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
		physicalPools, virtualPools, err := ontap.InitializeStoragePoolsCommon(ctx, d, poolAttributes, c.backendName)
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
