// Copyright 2016 NetApp, Inc. All Rights Reserved.

package storage

import (
	"encoding/json"
	"fmt"

	log "github.com/Sirupsen/logrus"
	dvp "github.com/netapp/netappdvp/storage_drivers"

	"github.com/netapp/trident/config"
)

type CommonStorageDriverConfigExternal struct {
	Version           int             `json:"version"`
	StorageDriverName string          `json:"storageDriverName"`
	StoragePrefixRaw  json.RawMessage `json:"storagePrefix,string"`
}

func SanitizeCommonStorageDriverConfig(c *dvp.CommonStorageDriverConfig) {
	log.WithFields(log.Fields{
		"name": c.StorageDriverName,
	}).Debug("Sanitizing common config.")
	if c.StoragePrefixRaw == nil {
		log.WithFields(log.Fields{
			"name": c.StorageDriverName,
		}).Debug("Setting nil raw storage prefix to contain an empty set.")
		c.StoragePrefixRaw = json.RawMessage("{}")
	}
	if c.SnapshotPrefixRaw == nil {
		log.WithFields(log.Fields{
			"name": c.StorageDriverName,
		}).Debug("Setting nil raw snapshot prefix to contain an empty set.")
		c.SnapshotPrefixRaw = json.RawMessage("{}")
	}
}

// SanitizeCommonConfig sets the json.RawMessage fields in
// dvp.CommonStorageDriverConfig to contain empty JSON objects if they are
// currently nil.  If this is not done, any attempts to marshal the config
// object will error.
func GetCommonStorageDriverConfigExternal(
	c *dvp.CommonStorageDriverConfig,
) *CommonStorageDriverConfigExternal {
	SanitizeCommonStorageDriverConfig(c)
	return &CommonStorageDriverConfigExternal{
		Version:           c.Version,
		StorageDriverName: c.StorageDriverName,
		StoragePrefixRaw:  c.StoragePrefixRaw,
	}
}

func GetCommonInternalVolumeName(
	c *dvp.CommonStorageDriverConfig, name string,
) string {
	prefixToUse := ""
	// BEGIN Copied from the NetApp DVP.
	storagePrefixRaw := c.StoragePrefixRaw // this is a raw version of the json value, we will get quotes in it
	if len(storagePrefixRaw) >= 2 {
		s := string(storagePrefixRaw)
		if s == "\"\"" || s == "" {
			prefixToUse = ""
		} else {
			// trim quotes from start and end of string
			prefixToUse = s[1 : len(s)-1]
		}
	}
	// END copying
	if prefixToUse == "" {
		prefixToUse = config.OrchestratorName
	}
	return fmt.Sprintf("%s-%s", prefixToUse, name)
}
