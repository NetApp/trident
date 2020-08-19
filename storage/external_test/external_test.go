// Copyright 2020 NetApp, Inc. All Rights Reserved.

package externaltest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/storage/factory"
	"github.com/netapp/trident/storage/fake"
	sa "github.com/netapp/trident/storage_attribute"
	fakedriver "github.com/netapp/trident/storage_drivers/fake"
)

func TestConstructExternalBackend(t *testing.T) {
	configJSON, err := fakedriver.NewFakeStorageDriverConfigJSON(
		"external-test",
		config.File,
		map[string]*fake.StoragePool{
			"test-1": {
				Attrs: map[string]sa.Offer{
					sa.Media: sa.NewStringOffer(sa.HDD),
				},
			},
			"test-2": {
				Attrs: map[string]sa.Offer{
					sa.Media:       sa.NewStringOffer(sa.SSD, sa.Hybrid),
					sa.IOPS:        sa.NewIntOffer(1000, 2000),
					sa.BackendType: sa.NewStringOffer("fake"),
				},
			},
		},
		[]fake.Volume{},
	)
	if err != nil {
		t.Fatal("Unable to construct config JSON.")
	}
	fakeBackend, err := factory.NewStorageBackendForConfig(context.Background(), configJSON)
	if err != nil {
		t.Fatal("Unable to construct backend:  ", err)
	}
	externalBackend := fakeBackend.ConstructExternal(context.Background())
	externalJSON, err := json.Marshal(externalBackend)
	if err != nil {
		t.Fatal("Unable to marshal JSON:  ", err)
	}
	// Test whether the JSON contains "e30="
	if strings.Contains(string(externalJSON), "e30=") {
		t.Error("Found base64 encoding in JSON:  ", string(externalJSON))
	}
}
