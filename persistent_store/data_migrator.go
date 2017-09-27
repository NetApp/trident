// Copyright 2017 NetApp, Inc. All Rights Reserved.

package persistent_store

import (
	"fmt"

	log "github.com/Sirupsen/logrus"
)

type DataMigrator struct {
	SourceType   StoreType
	SourceClient Client
	DestClient   Client
}

func NewDataMigrator(destClient Client, sourceType StoreType) *DataMigrator {
	return &DataMigrator{
		SourceType: sourceType,
		DestClient: destClient,
	}
}

func (m *DataMigrator) Run(keyPrefix string, deleteSrc bool) error {
	var err error
	if m.DestClient.GetType() == m.SourceType {
		return nil
	}
	// Determine if this is a supported data migration
	// 1) DataMigrator currently only supports etcdv2 to etcdv3 migration
	if m.DestClient.GetType() != EtcdV3Store &&
		m.SourceType != EtcdV2Store {
		// No transformation
		log.Debug("DataMigrator currently only supports etcdv2 to etcdv3 migration.")
		return nil
	}
	// 2) No transformation from etcdv2 to etcdv3 when the etcd server is
	// configured with TLS because etcdv2 doesn't support TLS.
	if m.DestClient.GetConfig().TLSConfig != nil {
		log.Debug("No persistent state transformation happens between etcdv2 " +
			"and etcdv3 when the etcd server requires client certificates!")
		return nil
	}

	log.WithFields(log.Fields{
		"current_store_version": string(m.SourceType),
		"desired_store_version": string(m.DestClient.GetType()),
	}).Info("Transforming persistent state.")
	var srcClient, destinationClient EtcdClient
	switch m.DestClient.GetType() {
	case EtcdV3Store:
		destinationClient, err = NewEtcdClientV3FromConfig(
			m.DestClient.GetConfig())
	default:
		return fmt.Errorf("Didn't recognize %v as a valid persistent store version!",
			m.DestClient.GetType())
	}
	if err != nil {
		return fmt.Errorf("Failed in creating the destination etcd client for data migration: %v",
			err)
	}
	switch m.SourceType {
	case EtcdV2Store:
		srcClient, err = NewEtcdClientV2FromConfig(
			m.DestClient.GetConfig())
	default:
		return fmt.Errorf("Didn't recognize %v as a valid persistent store version!",
			m.SourceType)
	}
	if err != nil {
		return fmt.Errorf("Failed in creating the source etcd client for data migration: %v",
			err)
	}
	etcdDataMigrator := NewEtcdDataMigrator(srcClient,
		destinationClient)
	// Moving all Trident objects for all API versions
	if err = etcdDataMigrator.Start(keyPrefix, deleteSrc); err != nil {
		return fmt.Errorf("etcd data migration failed: %v", err)
	}
	if err = etcdDataMigrator.Stop(); err != nil {
		return fmt.Errorf("Failed to shut down the etcd data migrator: %v",
			err)
	}
	return nil
}
