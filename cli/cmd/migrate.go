// Copyright 2020 NetApp, Inc. All Rights Reserved.

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/cenkalti/backoff"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	persistentstore "github.com/netapp/trident/persistent_store"
)

var (
	// CLI flags
	migrateDryRun bool
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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {

		initInstallerLogging()

		if err := validateMigrationArguments(); err != nil {
			log.Errorf("Invalid arguments; %v", err)
			return fmt.Errorf("invalid arguments; %v", err)
		}

		if err := discoverMigrationEnvironment(); err != nil {
			log.Errorf("Migration pre-checks failed; %v", err)
			return fmt.Errorf("migration pre-checks failed; %v", err)
		}

		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {

		if err := migrator.Run(); err != nil {
			log.Errorf("Migration failed; %v.  Resolve the issue and try again.", err)
			return fmt.Errorf("migration failed; %v.  Resolve the issue and try again.", err)
		}

		return nil
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

	// Create CRD client.  This could take a few minutes if the CRDs were just registered.
	if err = waitForCRDClient(180 * time.Second); err != nil {
		return err
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

	transformer := persistentstore.NewEtcdDataTransformer(etcdClient, migrateDryRun, false)
	migrator = persistentstore.NewCRDDataMigrator(etcdClient, crdClient, migrateDryRun, transformer)

	return migrator.RunPrechecks()
}

func waitForCRDClient(timeout time.Duration) error {

	createCRDClient := func() error {

		var err error

		if k8sAPIServer != "" || k8sConfigPath != "" {
			log.Debug("Migrator is configured to use an out-of-cluster CRD client.")
			crdClient, err = persistentstore.NewCRDClientV1(k8sAPIServer, k8sConfigPath)
		} else {
			log.Debug("Migrator is configured to use an in-cluster CRD client.")
			crdClient, err = persistentstore.NewCRDClientV1InCluster()
		}
		return err
	}

	createCRDClientNotify := func(err error, duration time.Duration) {
		log.WithFields(log.Fields{
			"increment": duration.Truncate(100 * time.Millisecond),
			"message":   err.Error(),
		}).Debug("CRD client not yet created, waiting.")
	}

	createCRDClientBackoff := backoff.NewExponentialBackOff()
	createCRDClientBackoff.InitialInterval = 1 * time.Second
	createCRDClientBackoff.RandomizationFactor = 0.1
	createCRDClientBackoff.Multiplier = 1.414
	createCRDClientBackoff.MaxInterval = 5 * time.Second
	createCRDClientBackoff.MaxElapsedTime = timeout

	log.Debug("Creating CRD client.")

	if err := backoff.RetryNotify(createCRDClient, createCRDClientBackoff, createCRDClientNotify); err != nil {
		log.Errorf("Could not create CRD client; %v", err)
		return fmt.Errorf("could not create CRD client; %v", err)
	}

	log.Debug("Created CRD client.")

	return nil
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
