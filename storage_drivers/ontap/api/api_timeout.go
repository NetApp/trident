// Copyright 2026 NetApp, Inc. All Rights Reserved.

package api

import (
	"fmt"
	"time"

	tridentconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/pkg/convert"
	drivers "github.com/netapp/trident/storage_drivers"
)

// StorageAPITimeoutFromOntapConfig returns the per-backend ONTAP storage API timeout.
// An empty storageAPITimeout returns 0 (not configured). ZAPI clients apply the 90s
// default when unset; REST clients leave http.Client.Timeout at 0 (no limit).
func StorageAPITimeoutFromOntapConfig(ontapConfig *drivers.OntapStorageDriverConfig) (time.Duration, error) {
	if ontapConfig == nil || ontapConfig.StorageAPITimeout == "" {
		return 0, nil
	}

	seconds, err := convert.ToPositiveInt64(ontapConfig.StorageAPITimeout)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("invalid value for storageAPITimeout: %v", ontapConfig.StorageAPITimeout)
	}

	return time.Duration(seconds) * time.Second, nil
}

func effectiveZapiStorageAPITimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}
	return tridentconfig.StorageAPITimeout
}

func clientConfigFromOntapConfig(
	ontapConfig *drivers.OntapStorageDriverConfig, numRecords int,
) (ClientConfig, error) {
	storageAPITimeout, err := StorageAPITimeoutFromOntapConfig(ontapConfig)
	if err != nil {
		return ClientConfig{}, err
	}

	return ClientConfig{
		ManagementLIF:           ontapConfig.ManagementLIF,
		Username:                ontapConfig.Username,
		Password:                ontapConfig.Password,
		ClientCertificate:       ontapConfig.ClientCertificate,
		ClientPrivateKey:        ontapConfig.ClientPrivateKey,
		ContextBasedZapiRecords: numRecords,
		TrustedCACertificate:    ontapConfig.TrustedCACertificate,
		DebugTraceFlags:         ontapConfig.DebugTraceFlags,
		StorageAPITimeout:       storageAPITimeout,
	}, nil
}
