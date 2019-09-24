// Copyright 2018 NetApp, Inc. All Rights Reserved.

package persistentstore

import (
	"flag"
	"fmt"
	"testing"

	log "github.com/sirupsen/logrus"
)

var (
	etcdSrc   = flag.String("etcd_src", "http://127.0.0.1:8001", "The source etcd server")
	etcdDest  = flag.String("etcd_dest", "http://127.0.0.1:8001", "The destination etcd server")
	keyPrefix = flag.String("key_prefix", "/trident_test/v1", "The key prefix for the test")
)

func init() {
	testing.Init()
	if *debug {
		log.SetLevel(log.DebugLevel)
	}
}

func PopulateSourceCluster(client EtcdClient, prefix string) error {
	for i := 1; i <= 5; i++ {
		if err := client.Create(fmt.Sprintf("%s/backend/b%d", prefix, i), fmt.Sprintf("BACKEND%d", i)); err != nil {
			return err
		}
	}
	for i := 1; i <= 3; i++ {
		if err := client.Create(fmt.Sprintf("%s/volume/v%d", prefix, i), fmt.Sprintf("VOLUME%d", i)); err != nil {
			return err
		}
	}
	for i := 1; i <= 2; i++ {
		if err := client.Create(fmt.Sprintf("%s/storageclass/s%d", prefix, i), fmt.Sprintf("CLASS%d",
			i)); err != nil {
			return err
		}
	}
	for i := 1; i <= 4; i++ {
		if err := client.Create(fmt.Sprintf("%s/parent/child/grandchild%d", prefix, i), fmt.Sprintf("GCHILD%d",
			i)); err != nil {
			return err
		}
	}
	return client.Create(fmt.Sprintf("%s/version", prefix), fmt.Sprintf("V1"))
}

func TestEtcdV2ToEtcdV3Migration(t *testing.T) {
	// Set up etcd clients
	srcClient, err := NewEtcdClientV2(*etcdSrc)
	if err != nil {
		t.Fatalf("Creating etcd client for the source cluster failed: %v",
			err)
	}
	destClient, err := NewEtcdClientV3(*etcdDest)
	if err != nil {
		t.Fatalf("Creating etcd client for the destination cluster failed: %v",
			err)
	}

	// Populate the source cluster
	if err = PopulateSourceCluster(srcClient, *keyPrefix); err != nil {
		t.Fatalf("Populating the source cluster failed: %v", err)
	}

	// Migrate the data
	dataMigrator := NewEtcdDataMigrator(srcClient, destClient)
	if err = dataMigrator.Start(*keyPrefix, false); err != nil {
		t.Fatalf("etcd data migration failed: %v", err)
	}

	// Compare the keys on the source cluster with those on the destination cluster
	keysSrc, err := srcClient.ReadKeys(*keyPrefix)
	if err != nil {
		t.Fatalf("Reading the keys from the source cluster failed: %v", err)
	}
	log.Debug("Source keys: ", keysSrc)
	keysDest, err := destClient.ReadKeys(*keyPrefix)
	if err != nil {
		t.Fatalf("Reading the keys from the destination cluster failed: %v", err)
	}
	log.Debug("Destination keys: ", keysDest)
	for i, key := range keysSrc {
		if key != keysDest[i] {
			t.Fatalf("Source key (%v) didn't match the destination key (%v)!",
				key, keysDest[i])
		}
	}

	// Delete the keys on both clusters
	if err = srcClient.DeleteKeys(*keyPrefix); err != nil {
		t.Fatalf("Deleting the keys from the source cluster failed: %v", err)
	}
	if err = destClient.DeleteKeys(*keyPrefix); err != nil {
		t.Fatalf("Deleting the keys from the destination cluster failed: %v", err)
	}

	// Validate the delete
	keysSrc, err = srcClient.ReadKeys(*keyPrefix)
	if err != nil && MatchKeyNotFoundErr(err) {
	} else {
		log.Debug("Source keys after delete: ", keysSrc)
		t.Fatalf("Deleting the keys from the source cluster failed: %v", err)
	}
	keysDest, err = destClient.ReadKeys(*keyPrefix)
	if err != nil && MatchKeyNotFoundErr(err) {
	} else {
		log.Debug("Destination keys after delete: ", keysDest)
		t.Fatalf("Deleting the keys from the destination cluster failed: %v", err)
	}

	// Shut down
	if err = dataMigrator.Stop(); err != nil {
		t.Fatal(err)
	}
}
