// Copyright 2016 NetApp, Inc. All Rights Reserved.

package external_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/drivers/fake"
	"github.com/netapp/trident/storage/factory"
	sa "github.com/netapp/trident/storage_attribute"
)

func TestConstructExternalBackend(t *testing.T) {
	configJSON, err := fake.NewFakeStorageDriverConfigJSON(
		"external-test",
		config.File,
		map[string]*fake.FakeStoragePool{
			"test-1": &fake.FakeStoragePool{
				Attrs: map[string]sa.Offer{
					sa.Media: sa.NewStringOffer(sa.HDD),
				},
			},
			"test-2": &fake.FakeStoragePool{
				Attrs: map[string]sa.Offer{
					sa.Media:       sa.NewStringOffer(sa.SSD, sa.Hybrid),
					sa.IOPS:        sa.NewIntOffer(1000, 2000),
					sa.BackendType: sa.NewStringOffer("fake"),
				},
			},
		},
	)
	if err != nil {
		t.Fatal("Unable to construct config JSON.")
	}
	fakeBackend, err := factory.NewStorageBackendForConfig(configJSON)
	if err != nil {
		t.Fatal("Unable to construct backend:  ", err)
	}
	externalBackend := fakeBackend.ConstructExternal()
	externalJSON, err := json.Marshal(externalBackend)
	if err != nil {
		t.Fatal("Unable to marshal JSON:  ", err)
	}
	// Test whether the JSON contains "e30="
	if strings.Contains(string(externalJSON), "e30=") {
		t.Error("Found base64 encoding in JSON:  ", string(externalJSON))
	}
}
