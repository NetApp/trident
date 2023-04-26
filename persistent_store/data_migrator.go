// Copyright 2021 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	. "github.com/netapp/trident/logging"
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
	Log().Debug("DataMigrator does not currently support any migrations.")

	return nil // crdDataMigrator.Run()
}
