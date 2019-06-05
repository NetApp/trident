// Copyright 2019 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"errors"
	"fmt"

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
	// DataMigrator currently only supports etcdv3 to crdv1 migration.
	if m.SourceClient.GetType() != EtcdV3Store || m.DestClient.GetType() != CRDV1Store {
		log.Debug("DataMigrator currently only supports etcdv3 to crdv1 migration.")
		return nil
	}

	etcdClient, ok := m.SourceClient.(EtcdClient)
	if !ok {
		return errors.New("SourceClient is not an ETCDv3 client")
	}
	crdClient, ok := m.DestClient.(CRDClient)
	if !ok {
		return errors.New("DestClient is not a CRDv1 client")
	}

	shouldPersist := false
	transformer := NewEtcdDataTransformer(etcdClient, m.dryRun, shouldPersist)
	crdDataMigrator := NewCRDDataMigrator(etcdClient, crdClient, m.dryRun, transformer)

	if err := crdDataMigrator.RunPrechecks(); err != nil {
		return fmt.Errorf("CRD data migration prechecks failed: %v", err)
	}

	return crdDataMigrator.Run()
}
