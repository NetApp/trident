package main

import (
	"flag"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/persistent_store"
)

var (
	debug     = flag.Bool("debug", false, "Enable debugging output")
	etcdV2Src = flag.String("etcdv2_src", "", "The endpoint for the source "+
		"v2 etcd server (e.g., -etcdv2_src=http://10.0.1.1:8001).")
	etcdV3Src = flag.String("etcdv3_src", "", "The endpoint for the source "+
		"v3 etcd server (e.g., -etcdv3_src=http://10.0.1.1:8001).")
	etcdV3SrcCert = flag.String("etcdv3_src_cert", "/root/certs/etcd-client-src.crt",
		"etcdV3 source certificate")
	etcdV3SrcCACert = flag.String("etcdv3_src_cacert", "/root/certs/etcd-client-src-ca.crt",
		"etcdV3 source CA certificate")
	etcdV3SrcKey = flag.String("etcdv3_src_key", "/root/certs/etcd-client-src.key",
		"etcdV3 source private key")
	etcdV2Dest = flag.String("etcdv2_dest", "", "The endpoint for the "+
		"destination v2 etcd server (e.g., -etcdv2_dest=http://10.0.0.1:8001).")
	etcdV3Dest = flag.String("etcdv3_dest", "", "The endpoint for the "+
		"destination v3 etcd server (e.g., -etcdv3_dest=http://10.0.0.1:8001).")
	etcdV3DestCert = flag.String("etcdv3_dest_cert", "/root/certs/etcd-client-dest.crt",
		"etcdV3 destination certificate")
	etcdV3DestCACert = flag.String("etcdv3_dest_cacert", "/root/certs/etcd-client-dest-ca.crt",
		"etcdV3 destination CA certificate")
	etcdV3DestKey = flag.String("etcdv3_dest_key", "/root/certs/etcd-client-dest.key",
		"etcdV3 destination private key")
	keyPrefix     = flag.String("key_prefix", "/trident/v1/", "The prefix of keys to migrate.")
	deleteSrcData = flag.Bool("delete_src", false, "Delete source cluster data after migration")
	etcdSrc       persistentstore.EtcdClient
	etcdDest      persistentstore.EtcdClient
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

func shouldEnableTLS(clientCert, clientCACert, clientKey string) bool {
	// Check for client certificate, client CA certificate, and client private key
	if _, err := os.Stat(clientCert); err != nil {
		return false
	}
	if _, err := os.Stat(clientCACert); err != nil {
		return false
	}
	if _, err := os.Stat(clientKey); err != nil {
		return false
	}
	return true
}

func main() {
	var err error

	processCmdLineArgs()

	switch {
	case *etcdV2Src != "":
		etcdSrc, err = persistentstore.NewEtcdClientV2(*etcdV2Src)
	case *etcdV3Src != "":
		if shouldEnableTLS(*etcdV3SrcCert, *etcdV3SrcCACert, *etcdV3SrcKey) {
			etcdSrc, err = persistentstore.NewEtcdClientV3WithTLS(*etcdV3Src, *etcdV3SrcCert, *etcdV3SrcCACert, *etcdV3SrcKey)
		} else {
			etcdSrc, err = persistentstore.NewEtcdClientV3(*etcdV3Src)
		}
	}
	if err != nil {
		log.Fatalf("Creating etcd client for the source cluster failed: %v",
			err)
	}
	switch {
	case *etcdV2Dest != "":
		etcdDest, err = persistentstore.NewEtcdClientV2(*etcdV2Dest)
	case *etcdV3Dest != "":
		if shouldEnableTLS(*etcdV3DestCert, *etcdV3DestCACert, *etcdV3DestKey) {
			etcdDest, err = persistentstore.NewEtcdClientV3WithTLS(*etcdV3Dest, *etcdV3DestCert, *etcdV3DestCACert, *etcdV3DestKey)
		} else {
			etcdDest, err = persistentstore.NewEtcdClientV3(*etcdV3Dest)
		}
	}
	if err != nil {
		log.Fatalf("Creating etcd client for the destination cluster failed: %v",
			err)
	}
	dataMigrator := persistentstore.NewEtcdDataMigrator(etcdSrc, etcdDest)
	if err = dataMigrator.Start(*keyPrefix, *deleteSrcData); err != nil {
		log.Error("etcd data migration failed: ", err)
	}
	if err = dataMigrator.Stop(); err != nil {
		log.Error(err)
	}
}
