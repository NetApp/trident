// Copyright 2016 NetApp, Inc. All Rights Reserved.

package storage

import (
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
	if c.StoragePrefix == nil {
		prefix := ""
		log.WithFields(log.Fields{
			"name": c.StorageDriverName,
		}).Debug("Setting nil storage prefix to contain an empty string.")
		c.StoragePrefix = &prefix
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

	if c.StoragePrefix != nil {
		prefixToUse = *c.StoragePrefix
	}

	return fmt.Sprintf("%s-%s", prefixToUse, name)
}
