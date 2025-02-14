// Copyright 2024 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	sa "github.com/netapp/trident/storage_attribute"
	drivers "github.com/netapp/trident/storage_drivers"
)

const (
	FlagPersonality   = "personality"
	FlagDisaggregated = "disaggregated"
	FlagSANOptimized  = "sanOptimized"

	PersonalityONTAP9 = "ONTAP9"
	PersonalityASAr2  = "ASAr2"
)

// GetStorageDriver uses a backend config to create an ONTAP API client and constructs the necessary storage driver.
func GetStorageDriver(
	ctx context.Context, configJSON string, commonConfig *drivers.CommonStorageDriverConfig,
	backendSecret map[string]string,
) (storage.Driver, error) {
	driverProtocol, err := GetDriverProtocol(commonConfig.StorageDriverName, configJSON)
	if err != nil {
		Logc(ctx).WithField("error", err).Error("Failed to get driver protocol.")
		return nil, err
	}

	// Parse the config
	ontapConfig, err := InitializeOntapConfig(ctx, config.CurrentDriverContext, configJSON, commonConfig, backendSecret)
	if err != nil {
		return nil, fmt.Errorf("error initializing %s driver: %v", commonConfig.StorageDriverName, err)
	}

	// Initialize the ONTAP API.
	API, err := InitializeOntapDriver(ctx, ontapConfig)
	if err != nil {
		return nil, fmt.Errorf("error initializing %s driver: %v", commonConfig.StorageDriverName, err)
	}

	// Set up driver flags
	ontapConfig.Flags = map[string]string{
		FlagPersonality:   PersonalityONTAP9,
		FlagDisaggregated: strconv.FormatBool(API.IsDisaggregated()),
		FlagSANOptimized:  strconv.FormatBool(API.IsSANOptimized()),
	}

	var storageDriver storage.Driver
	switch ontapConfig.StorageDriverName {

	case config.OntapNASStorageDriverName:
		storageDriver = &NASStorageDriver{API: API, Config: *ontapConfig}
	case config.OntapNASFlexGroupStorageDriverName:
		storageDriver = &NASFlexGroupStorageDriver{API: API, Config: *ontapConfig}
	case config.OntapNASQtreeStorageDriverName:
		storageDriver = &NASQtreeStorageDriver{API: API, Config: *ontapConfig}
	case config.OntapSANEconomyStorageDriverName:
		storageDriver = &SANEconomyStorageDriver{API: API, Config: *ontapConfig}

	// ontap-san uses additional system details to choose the needed driver
	case config.OntapSANStorageDriverName:
		switch driverProtocol {
		case sa.ISCSI:
			if API.IsSANOptimized() && API.IsDisaggregated() {
				ontapConfig.Flags[FlagPersonality] = PersonalityASAr2 // Used by ASUP to distinguish personalities
				storageDriver = &ASAStorageDriver{API: API, Config: *ontapConfig}
			} else if !API.IsSANOptimized() && !API.IsDisaggregated() {
				storageDriver = &SANStorageDriver{API: API, Config: *ontapConfig}
			} else {
				return nil, fmt.Errorf("unsupported ONTAP personality with disaggregated %t and SAN optimized %t",
					API.IsDisaggregated(), API.IsSANOptimized())
			}
		case sa.FCP:
			storageDriver = &SANStorageDriver{API: API, Config: *ontapConfig}
		case sa.NVMe:
			storageDriver = &NVMeStorageDriver{API: API, Config: *ontapConfig}
		default:
			return nil, fmt.Errorf("unsupported SAN protocol %s", driverProtocol)
		}

	default:
		return nil, fmt.Errorf("unsupported ONTAP driver type %s", ontapConfig.StorageDriverName)
	}

	Logc(ctx).WithFields(LogFields{
		"disaggregated": API.IsDisaggregated(),
		"sanOptimized":  API.IsSANOptimized(),
	}).Infof("ONTAP factory creating %T backend.", storageDriver)

	return storageDriver, nil
}

// GetDriverProtocol returns the protocol type for SAN Drivers using the backend config.
// This function can be extended for NAS drivers if required.
func GetDriverProtocol(driverName, configJSON string) (string, error) {
	pool := drivers.OntapStorageDriverPool{}
	if err := json.Unmarshal([]byte(configJSON), &pool); err != nil {
		return "", fmt.Errorf("failed to get pool values: %v", err)
	}

	if driverName == config.OntapSANStorageDriverName {
		SANType := strings.ToLower(pool.SANType)
		switch SANType {
		case sa.ISCSI, sa.NVMe, sa.FCP:
			return SANType, nil
		case "":
			// Old iSCSI backends will have no value for SANType
			return sa.ISCSI, nil
		default:
			return "", fmt.Errorf("unsupported SAN protocol %s", SANType)
		}
	}

	return "", nil
}
