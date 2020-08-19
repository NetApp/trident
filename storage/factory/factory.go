// Copyright 2020 NetApp, Inc. All Rights Reserved.

package factory

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/ghodss/yaml"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/aws"
	"github.com/netapp/trident/storage_drivers/azure"
	"github.com/netapp/trident/storage_drivers/eseries"
	"github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/storage_drivers/gcp"
	"github.com/netapp/trident/storage_drivers/ontap"
	"github.com/netapp/trident/storage_drivers/solidfire"
)

func NewStorageBackendForConfig(ctx context.Context, configJSON string) (sb *storage.Backend, err error) {

	var storageDriver storage.Driver

	// Some drivers may panic during initialize if given invalid parameters,
	// so catch any panics that might occur and return an error.
	defer func() {
		if r := recover(); r != nil {
			Logc(ctx).WithField("Stack trace", string(debug.Stack())).Error("unable to instantiate backend")
			err = fmt.Errorf("unable to instantiate backend: %v", r)
		}
	}()

	// Convert config (JSON or YAML) to JSON
	configJSONBytes, err := yaml.YAMLToJSON([]byte(configJSON))
	if err != nil {
		err = fmt.Errorf("invalid config format: %v", err)
		return nil, err
	}
	configJSON = string(configJSONBytes)

	// Parse the common config struct from JSON
	commonConfig, err := drivers.ValidateCommonSettings(ctx, configJSON)
	if err != nil {
		err = fmt.Errorf("input failed validation: %v", err)
		return nil, err
	}

	// Pre-driver initialization setup
	switch commonConfig.StorageDriverName {
	case drivers.OntapNASStorageDriverName:
		storageDriver = &ontap.NASStorageDriver{}
	case drivers.OntapNASFlexGroupStorageDriverName:
		storageDriver = &ontap.NASFlexGroupStorageDriver{}
	case drivers.OntapNASQtreeStorageDriverName:
		storageDriver = &ontap.NASQtreeStorageDriver{}
	case drivers.OntapSANStorageDriverName:
		storageDriver = &ontap.SANStorageDriver{}
	case drivers.OntapSANEconomyStorageDriverName:
		storageDriver = &ontap.SANEconomyStorageDriver{}
	case drivers.SolidfireSANStorageDriverName:
		storageDriver = &solidfire.SANStorageDriver{}
	case drivers.EseriesIscsiStorageDriverName:
		storageDriver = &eseries.SANStorageDriver{}
	case drivers.AWSNFSStorageDriverName:
		storageDriver = &aws.NFSStorageDriver{}
	case drivers.AzureNFSStorageDriverName:
		storageDriver = &azure.NFSStorageDriver{}
	case drivers.GCPNFSStorageDriverName:
		storageDriver = &gcp.NFSStorageDriver{}
	case drivers.FakeStorageDriverName:
		storageDriver = &fake.StorageDriver{}
	default:
		err = fmt.Errorf("unknown storage driver: %v", commonConfig.StorageDriverName)
		return nil, err
	}

	Logc(ctx).WithField("driver", commonConfig.StorageDriverName).Debug("Initializing storage driver.")

	// Initialize the driver.  If this fails, return a 'failed' backend object.
	if err = storageDriver.Initialize(ctx, config.CurrentDriverContext, configJSON, commonConfig); err != nil {

		Logc(ctx).WithField("error", err).Error("Could not initialize storage driver.")

		return storage.NewFailedStorageBackend(ctx, storageDriver),
			fmt.Errorf("problem initializing storage driver '%s': %v", commonConfig.StorageDriverName, err)
	} else {
		Logc(ctx).WithField("driver", commonConfig.StorageDriverName).Info("Storage driver initialized.")
	}

	// Create the backend object.  If this calls the driver and fails, return a 'failed' backend object.
	if sb, err = storage.NewStorageBackend(ctx, storageDriver); err != nil {

		Logc(ctx).WithField("error", err).Error("Could not create storage backend.")

		return storage.NewFailedStorageBackend(ctx, storageDriver),
			fmt.Errorf("problem creating storage backend '%s': %v", commonConfig.StorageDriverName, err)
	} else {
		Logc(ctx).WithField("backend", sb).Info("Created new storage backend.")
	}

	sb.State = storage.Online

	return sb, err
}
