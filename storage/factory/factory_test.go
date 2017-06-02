// Copyright 2016 NetApp, Inc. All Rights Reserved.

package factory

import (
	"encoding/json"
	"testing"

	dvp "github.com/netapp/netappdvp/storage_drivers"
)

// TestInitializeRecovery intentionally passes a bogus config to
// NewStorageBackendForConfig to test its ability to recover.
func TestInitializeRecovery(t *testing.T) {
	empty := ""
	config := &dvp.OntapStorageDriverConfig{
		CommonStorageDriverConfig: &dvp.CommonStorageDriverConfig{
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
	json, err := json.Marshal(config)
	if err != nil {
		t.Fatal("Unable to marshal ONTAP config:  ", err)
	}
	_, err = NewStorageBackendForConfig(string(json))
	if err == nil {
		t.Error("Failed to get error for invalid configuration.")
	}
}
