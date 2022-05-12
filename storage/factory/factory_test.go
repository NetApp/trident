// Copyright 2022 NetApp, Inc. All Rights Reserved.

package factory

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/storage"
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
		// These should be bogus yet valid connection parameters
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
		t.Error("Failed to validate settings for configuration.")
	}

	_, err = NewStorageBackendForConfig(context.Background(), configInJSON, "fakeConfigRef", uuid.New().String(), commonConfig, nil)
	if err == nil {
		t.Error("Failed to get error for incorrect configuration.")
	}
}

func TestNewStorageBackendForConfig(t *testing.T) {
	backendUUID := uuid.New().String()
	empty := ""
	config := &drivers.FakeStorageDriverConfig{
		CommonStorageDriverConfig: &drivers.CommonStorageDriverConfig{
			Version:           1,
			StorageDriverName: "fake",
			StoragePrefixRaw:  json.RawMessage("{}"),
			StoragePrefix:     &empty,
		},
		Username: "none",
		Password: "none",
	}
	marshaledJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatal("Unable to marshal ONTAP config:  ", err)
	}

	commonConfig, configInJSON, err := ValidateCommonSettings(context.Background(), string(marshaledJSON))
	if err != nil {
		t.Error("Failed to validate settings for invalid configuration: ", err)
	}

	storageBackend, err := NewStorageBackendForConfig(context.Background(), configInJSON, "fakeConfigRef", backendUUID, commonConfig, nil)
	if err != nil {
		t.Error("Got error for invalid configuration: ", err)
	}
	if storageBackend.Driver().Name() != "fake" {
		t.Log("Driver name is not 'fake'")
		t.Fail()
	}
	if storageBackend.State() != storage.Online {
		t.Log("Storage backend is not online")
		t.Fail()
	}
	if storageBackend.BackendUUID() != backendUUID {
		t.Logf("backendUUID does not match; actual: %v, expected: %v", storageBackend.BackendUUID(), backendUUID)
		t.Fail()
	}
	if storageBackend.ConfigRef() != "fakeConfigRef" {
		t.Logf("Storage backend configRef does not match; actual: %v, expected: %v", storageBackend.ConfigRef(), "fakeConfigRef")
		t.Fail()
	}
}
