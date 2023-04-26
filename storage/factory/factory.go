// Copyright 2022 NetApp, Inc. All Rights Reserved.

package factory

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"

	"github.com/ghodss/yaml"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/azure"
	"github.com/netapp/trident/storage_drivers/fake"
	"github.com/netapp/trident/storage_drivers/gcp"
	"github.com/netapp/trident/storage_drivers/ontap"
	"github.com/netapp/trident/storage_drivers/solidfire"
)

// SpecOnlyValidation applies to values supplied through the CRD controller, this ensures that
// forbidden attributes are not in the spec, and contains the credentials field.
func SpecOnlyValidation(
	ctx context.Context, commonConfig *drivers.CommonStorageDriverConfig,
	configInJSON string,
) error {
	storageDriverConfig, err := drivers.GetDriverConfigByName(commonConfig.StorageDriverName)
	if err != nil {
		return err
	}

	err = json.Unmarshal([]byte(configInJSON), storageDriverConfig)
	if err != nil {
		Logc(ctx).Errorf("could not parse JSON configuration: %v", err)
		return err
	}
	return storageDriverConfig.SpecOnlyValidation()
}

func ValidateCommonSettings(ctx context.Context, configJSON string) (commonConfig *drivers.CommonStorageDriverConfig,
	configInJSON string, err error,
) {
	// Convert config (JSON or YAML) to JSON
	configJSONBytes, err := yaml.YAMLToJSON([]byte(configJSON))
	if err != nil {
		err = fmt.Errorf("invalid config format: %v", err)
		return nil, "", err
	}
	configInJSON = string(configJSONBytes)

	// Parse the common config struct from JSON
	commonConfig, err = drivers.ValidateCommonSettings(ctx, configInJSON)
	if err != nil {
		err = fmt.Errorf("input failed validation: %v", err)
		return nil, "", err
	}

	return commonConfig, configInJSON, nil
}

func NewStorageBackendForConfig(
	ctx context.Context, configJSON, configRef, backendUUID string,
	commonConfig *drivers.CommonStorageDriverConfig, backendSecret map[string]string,
) (sb storage.Backend,
	err error,
) {
	// Some drivers may panic during initialize if given invalid parameters,
	// so catch any panics that might occur and return an error.
	defer func() {
		if r := recover(); r != nil {
			Logc(ctx).WithField("Stack trace", string(debug.Stack())).Error("unable to instantiate backend")
			err = fmt.Errorf("unable to instantiate backend: %v", r)
		}
	}()

	var storageDriver storage.Driver
	if storageDriver, err = GetStorageDriver(commonConfig.StorageDriverName); err != nil {
		Logc(ctx).WithField("error", err).Error("Invalid or unknown storage driver found.")
		return nil, err
	}

	Logc(ctx).WithField("driver", commonConfig.StorageDriverName).Debug("Initializing storage driver.")

	// Initialize the driver.  If this fails, return a 'failed' backend object.
	if err = storageDriver.Initialize(ctx, config.CurrentDriverContext, configJSON, commonConfig,
		backendSecret, backendUUID); err != nil {

		Logc(ctx).WithField("error", err).Error("Could not initialize storage driver.")

		sb = storage.NewFailedStorageBackend(ctx, storageDriver)
		err = fmt.Errorf("problem initializing storage driver '%s': %v", commonConfig.StorageDriverName, err)
	} else {
		Logc(ctx).WithField("driver", commonConfig.StorageDriverName).Info("Storage driver initialized.")

		// Create the backend object.  If this calls the driver and fails, return a 'failed' backend object.
		if sb, err = CreateNewStorageBackend(ctx, storageDriver); err != nil {
			Logc(ctx).WithField("error", err).Error("Could not create storage backend.")
		}
	}

	if err == nil {
		sb.SetState(storage.Online)
	}
	sb.SetBackendUUID(backendUUID)
	sb.SetConfigRef(configRef)

	return sb, err
}

func GetStorageDriver(driverName string) (storage.Driver, error) {
	var storageDriver storage.Driver

	// Pre-driver initialization setup
	switch driverName {
	case config.OntapNASStorageDriverName:
		storageDriver = &ontap.NASStorageDriver{}
	case config.OntapNASFlexGroupStorageDriverName:
		storageDriver = &ontap.NASFlexGroupStorageDriver{}
	case config.OntapNASQtreeStorageDriverName:
		storageDriver = &ontap.NASQtreeStorageDriver{}
	case config.OntapSANStorageDriverName:
		storageDriver = &ontap.SANStorageDriver{}
	case config.OntapSANEconomyStorageDriverName:
		storageDriver = &ontap.SANEconomyStorageDriver{}
	case config.SolidfireSANStorageDriverName:
		storageDriver = &solidfire.SANStorageDriver{}
	case config.AzureNASStorageDriverName:
		storageDriver = &azure.NASStorageDriver{}
	case config.AzureNASBlockStorageDriverName:
		storageDriver = &azure.NASBlockStorageDriver{}
	case config.GCPNFSStorageDriverName:
		storageDriver = &gcp.NFSStorageDriver{}
	case config.FakeStorageDriverName:
		storageDriver = &fake.StorageDriver{}
	default:
		return nil, fmt.Errorf("unknown storage driver: %v", driverName)
	}

	return storageDriver, nil
}

func CreateNewStorageBackend(ctx context.Context, storageDriver storage.Driver) (storage.Backend, error) {
	var sb storage.Backend
	var err error
	if sb, err = storage.NewStorageBackend(ctx, storageDriver); err != nil {
		Logc(ctx).WithField("error", err).Error("Could not create storage backend.")

		sb = storage.NewFailedStorageBackend(ctx, storageDriver)
		err = fmt.Errorf("problem creating storage backend '%s': %v", storageDriver.Name(), err)
		return sb, err
	}

	Logc(ctx).WithField("backend", sb).Info("Created new storage backend.")
	return sb, nil
}
