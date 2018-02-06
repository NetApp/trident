// Copyright 2018 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

type EtcdDataMigrator struct {
	SourceClient EtcdClient
	DestClient   EtcdClient
}

func NewEtcdDataMigrator(SourceClient, DestClient EtcdClient) *EtcdDataMigrator {
	return &EtcdDataMigrator{
		SourceClient: SourceClient,
		DestClient:   DestClient,
	}
}

func (m *EtcdDataMigrator) Start(keyPrefix string, deleteSrc bool) error {
	keys, err := m.SourceClient.ReadKeys(keyPrefix)
	if err != nil {
		if MatchKeyNotFoundErr(err) {
			log.Infof("No key with prefix %v to migrate.", keyPrefix)
			return nil
		}
		return fmt.Errorf("reading keys from the source client failed: %v", err)
	}
	for _, key := range keys {
		val, err := m.SourceClient.Read(key)
		if err != nil {
			return fmt.Errorf("reading key %v by the source client failed: %v",
				key, err)
		}
		log.WithFields(log.Fields{
			"key": key,
		}).Debug("Read key from the source.")
		err = m.DestClient.Set(key, val)
		if err != nil {
			return fmt.Errorf("setting key %v by the destination client failed: %v",
				key, err)
		}
		log.WithFields(log.Fields{
			"key": key,
		}).Debug("Wrote key to the destination.")
		if deleteSrc {
			err = m.SourceClient.Delete(key)
			if err != nil {
				return fmt.Errorf("deleting key %v by the source client failed: %v",
					key, err)
			}
			log.WithFields(log.Fields{
				"key": key,
			}).Debug("Deleted key from the source.")
		}
	}
	return nil
}

func (m *EtcdDataMigrator) Stop() error {
	if err := m.SourceClient.Stop(); err != nil {
		return fmt.Errorf("closing the source etcd client failed: %v", err)
	}
	if err := m.DestClient.Stop(); err != nil {
		return fmt.Errorf("closing the destination etcd client failed: %v",
			err)
	}
	return nil
}
