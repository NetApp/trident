package main

import (
	"flag"

	log "github.com/Sirupsen/logrus"

	"github.com/netapp/trident/persistent_store"
)

var (
	debug     = flag.Bool("debug", false, "Enable debugging output")
	etcdV2Src = flag.String("etcdv2_src", "", "The endpoint for the source "+
		"v2 etcd server (e.g., -etcdv2_src=http://10.0.1.1:8001).")
	etcdV3Src = flag.String("etcdv3_src", "", "The endpoint for the source "+
		"v3 etcd server (e.g., -etcdv3_src=http://10.0.1.1:8001).")
	etcdV2Dest = flag.String("etcdv2_dest", "", "The endpoint for the "+
		"destination v2 etcd server (e.g., -etcdv2_dest=http://10.0.0.1:8001).")
	etcdV3Dest = flag.String("etcdv3_dest", "", "The endpoint for the "+
		"destination v3 etcd server (e.g., -etcdv3_dest=http://10.0.0.1:8001).")
	keyPrefix     = flag.String("key_prefix", "/trident/v1/", "The prefix of keys to migrate.")
	deleteSrcData = flag.Bool("delete_src", false, "Delete source cluster data after migration")
	etcdSrc       persistent_store.EtcdClient
	etcdDest      persistent_store.EtcdClient
	//TODO: etcd certifiacates for the source and destination
	//TODO: trident API version source and destination
)

func processCmdLineArgs() {
	flag.Parse()
	if (*etcdV2Src == "" && *etcdV3Src == "") ||
		(*etcdV2Src != "" && *etcdV3Src != "") ||
		(*etcdV2Dest == "" && *etcdV3Dest == "") ||
		(*etcdV2Dest != "" && *etcdV3Dest != "") {
		log.Fatal("One of etcdv2_src or etcdv3_src and one of etcdv2_dest or " +
			"etcdv3_dest must be specified!")
	}
	if *debug {
		log.SetLevel(log.DebugLevel)
	}
}

func main() {
	var err error

	processCmdLineArgs()

	switch {
	case *etcdV2Src != "":
		etcdSrc, err = persistent_store.NewEtcdClientV2(*etcdV2Src)
	case *etcdV3Src != "":
		etcdSrc, err = persistent_store.NewEtcdClientV3(*etcdV3Src)
	}
	if err != nil {
		log.Fatalf("Creating etcd client for the source cluster failed: %v",
			err)
	}
	switch {
	case *etcdV2Dest != "":
		etcdDest, err = persistent_store.NewEtcdClientV2(*etcdV2Dest)
	case *etcdV3Dest != "":
		etcdDest, err = persistent_store.NewEtcdClientV3(*etcdV3Dest)
	}
	if err != nil {
		log.Fatalf("Creating etcd client for the destination cluster failed: %v",
			err)
	}
	dataMigrator := persistent_store.NewEtcdDataMigrator(etcdSrc, etcdDest)
	if err = dataMigrator.Start(*keyPrefix, *deleteSrcData); err != nil {
		log.Error("etcd data migration failed: ", err)
	}
	if err = dataMigrator.Stop(); err != nil {
		log.Error(err)
	}
}
