// Copyright 2021 NetApp, Inc. All Rights Reserved.

package factory

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	drivers "github.com/netapp/trident/storage_drivers"
)

func TestMain(m *testing.M) {
	// Disable any standard log output
	log.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}

// TestInitializeRecovery intentionally passes a bogus config to
// NewStorageBackendForConfig to test its ability to recover.
func TestInitializeRecovery(t *testing.T) {
	empty := ""
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "ontap-nas",
			StoragePrefixRaw:  json.RawMessage("{}"),
			StoragePrefix:     &empty,
		},
		// These should be invalid connection parameters
		ManagementLIF: "127.0.0.1",
		DataLIF:       "127.0.0.1",
		IgroupName:    "nonexistent",
		SVM:           "nonexistent",
		Username:      "none",
		Password:      "none",
	}
	marshaledJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatal("Unable to marshal ONTAP config:  ", err)
	}

	commonConfig, configInJSON, err := ValidateCommonSettings(context.Background(), string(marshaledJSON))
	if err != nil {
		t.Error("Failed to validate settings for invalid configuration.")
	}

	_, err = NewStorageBackendForConfig(context.Background(), configInJSON, uuid.New().String(), commonConfig, nil)
	if err == nil {
		t.Error("Failed to get error for invalid configuration.")
	}
}
