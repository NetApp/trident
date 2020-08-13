// Copyright 2019 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	log "github.com/sirupsen/logrus"
)

type DataMigrator struct {
	SourceClient Client
	DestClient   Client
	dryRun       bool
}

func NewDataMigrator(SourceClient, DestClient Client, dryRun bool) *DataMigrator {
	return &DataMigrator{
		SourceClient: SourceClient,
		DestClient:   DestClient,
		dryRun:       dryRun,
	}
}

func (m *DataMigrator) Run() error {

	// Determine if this is a supported data migration.
	log.Debug("DataMigrator does not currently support any migrations.")

	return nil //crdDataMigrator.Run()
}
