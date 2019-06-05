// Copyright 2019 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/netapp/trident/persistent_store"
)

var (
	// CLI flags
	migrateDryRun bool
	shouldPersist bool
	k8sAPIServer  string
	k8sConfigPath string
	etcdV3        string
	etcdV3Cert    string
	etcdV3CACert  string
	etcdV3Key     string

	etcdClient *persistentstore.EtcdClientV3
	crdClient  *persistentstore.CRDClientV1
	migrator   *persistentstore.CRDDataMigrator
)

func init() {
	RootCmd.AddCommand(migrateCmd)
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Run all the pre-checks, but don't migrate anything.")
	migrateCmd.Flags().BoolVar(&silent, "silent", false, "Disable most output during migration.")

	// Kubernetes
	migrateCmd.Flags().StringVar(&k8sAPIServer, "k8s-api-server", "", "Kubernetes API server address.")
	migrateCmd.Flags().StringVar(&k8sConfigPath, "k8s-config-path", "", "Path to KubeConfig file.")

	// Etcd
	migrateCmd.Flags().StringVar(&etcdV3, "etcd-v3", "", "etcd server (v3 API) for persisting orchestrator state (e.g., --etcd-v3=http://127.0.0.1:8001)")
	migrateCmd.Flags().StringVar(&etcdV3Cert, "etcd-v3-cert", "/root/certs/etcd-client.crt", "etcdV3 client certificate")
	migrateCmd.Flags().StringVar(&etcdV3CACert, "etcd-v3-cacert", "/root/certs/etcd-client-ca.crt", "etcdV3 client CA certificate")
	migrateCmd.Flags().StringVar(&etcdV3Key, "etcd-v3-key", "/root/certs/etcd-client.key", "etcdV3 client private key")
}

var migrateCmd = &cobra.Command{
	Use:    "migrate",
	Short:  "Migrate Trident's data from etcd to Kubernetes CRDs",
	Hidden: true,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {

		initInstallerLogging()

		if err := validateMigrationArguments(); err != nil {
			log.Fatalf("Invalid arguments; %v", err)
		}

		if err := discoverMigrationEnvironment(); err != nil {
			log.Fatalf("Migration pre-checks failed; %v", err)
		}
	},
	Run: func(cmd *cobra.Command, args []string) {

		if err := migrator.Run(); err != nil {
			log.Fatalf("Migration failed; %v.  Resolve the issue and try again.", err)
		}
	},
}

func validateMigrationArguments() error {

	if etcdV3 == "" {
		return fmt.Errorf("Missing required flag: etcd-v3")
	}

	return nil
}

func discoverMigrationEnvironment() error {

	var err error

	// Create CRD client
	log.Debug("Trident is configured with a CRD client.")
	if k8sAPIServer != "" || k8sConfigPath != "" {
		log.Debug("Migrator is configured to use an out-of-cluster Kubernetes client.")
		crdClient, err = persistentstore.NewCRDClientV1(k8sAPIServer, k8sConfigPath)
	} else {
		log.Debug("Migrator is configured to use an in-cluster Kubernetes client.")
		crdClient, err = persistentstore.NewCRDClientV1InCluster()
	}
	if err != nil {
		return fmt.Errorf("could not create the Kubernetes client; %v", err)
	}

	// Create etcd client
	if shouldEnableTLS() {
		log.Debug("Migrator is configured to use an etcdv3 client with TLS.")
		etcdClient, err = persistentstore.NewEtcdClientV3WithTLS(etcdV3, etcdV3Cert, etcdV3CACert, etcdV3Key)
	} else {
		log.Debug("Migrator is configured to use an etcdv3 client without TLS.")
		etcdClient, err = persistentstore.NewEtcdClientV3(etcdV3)
	}
	if err != nil {
		return fmt.Errorf("could not create the etcd V3 client; %v", err)
	}

	shouldPersist := false
	transformer := persistentstore.NewEtcdDataTransformer(etcdClient, migrateDryRun, shouldPersist)
	migrator = persistentstore.NewCRDDataMigrator(etcdClient, crdClient, migrateDryRun, transformer)

	return migrator.RunPrechecks()
}

func shouldEnableTLS() bool {
	// Check for client certificate, client CA certificate, and client private key
	if _, err := os.Stat(etcdV3Cert); err != nil {
		return false
	}
	if _, err := os.Stat(etcdV3CACert); err != nil {
		return false
	}
	if _, err := os.Stat(etcdV3Key); err != nil {
		return false
	}
	return true
}
