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
	Version           int     `json:"version"`
	StorageDriverName string  `json:"storageDriverName"`
	StoragePrefix     *string `json:"storagePrefix"`
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
}

// SanitizeCommonConfig makes sure none of the fields in
// dvp.CommonStorageDriverConfig are nil. If this is not done, any
// attempts to marshal the config object will error.
func GetCommonStorageDriverConfigExternal(
	c *dvp.CommonStorageDriverConfig,
) *CommonStorageDriverConfigExternal {
	SanitizeCommonStorageDriverConfig(c)
	return &CommonStorageDriverConfigExternal{
		Version:           c.Version,
		StorageDriverName: c.StorageDriverName,
		StoragePrefix:     c.StoragePrefix,
	}
}

func GetCommonInternalVolumeName(
	c *dvp.CommonStorageDriverConfig, name string,
) string {
	prefixToUse := config.OrchestratorName

	// If a prefix was specified in the configuration, use that.
	if c.StoragePrefix != nil {
		prefixToUse = *c.StoragePrefix
	}

	// Special case an empty prefix so that we don't get a delimiter in front.
	if prefixToUse == "" {
		return name
	}

	return fmt.Sprintf("%s-%s", prefixToUse, name)
}
