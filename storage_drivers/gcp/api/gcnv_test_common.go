// Copyright 2025 NetApp, Inc. All Rights Reserved.

package api

import (
	"context"
	"time"

	"github.com/netapp/trident/pkg/collection"
	drivers "github.com/netapp/trident/storage_drivers"
)

const (
	ProjectNumber           = "123456789"
	Location                = "fake-location"
	Type                    = "fake-service-account"
	ProjectID               = "fake-project"
	PrivateKeyID            = "1234567b3456v44n"
	PrivateKey              = "-----BEGIN PRIVATE KEY-----fake-private-key----END PRIVATE KEY-----"
	ClientEmail             = "fake-client@email"
	ClientID                = "c5677na235896345363"
	AuthURI                 = "https://fake-auth.com/auth"
	TokenURI                = "https://fake-token.com/token" // #nosec
	AuthProviderX509CertURL = "https://fake-auth-provider.com/certs"
	ClientX509CertURL       = "https://fake-client.com/certs"

	BackendUUID     = "abcdefgh-03af-4394-ace4-e177cdbcaf28"
	SnapshotUUID    = "deadbeef-5c0d-4afa-8cd8-afa3fba5665c"
	VolumeSizeI64   = int64(107374182400)
	VolumeSizeStr   = "107374182400"
	StateReady      = "Ready"
	NetworkName     = "fake-network"
	NetworkFullName = "projects/" + ProjectNumber + "/locations/" + Location + "/networks/network"
	FullVolumeName  = "projects/" + ProjectNumber + "/locations/" + Location + "/volumes/"
)

var (
	ctx             = context.Background()
	debugTraceFlags = map[string]bool{"method": true, "api": true, "discovery": true}
)

// getFakeSDK creates a fake SDK client for testing. If withCapacityPools is true,
// it pre-populates capacity pools with different service levels for discovery tests.
func getFakeSDK(withCapacityPools ...bool) *Client {
	privateKey := &drivers.GCPPrivateKey{
		Type:                    Type,
		ProjectID:               ProjectID,
		PrivateKeyID:            PrivateKeyID,
		PrivateKey:              PrivateKey,
		ClientEmail:             ClientEmail,
		ClientID:                ClientID,
		AuthURI:                 AuthURI,
		TokenURI:                TokenURI,
		AuthProviderX509CertURL: AuthProviderX509CertURL,
		ClientX509CertURL:       ClientX509CertURL,
	}

	cfg := &ClientConfig{
		StorageDriverName: "fake",
		ProjectNumber:     ProjectNumber,
		Location:          Location,
		APIKey:            privateKey,
		DebugTraceFlags:   debugTraceFlags,
		SDKTimeout:        5 * time.Second,
		MaxCacheAge:       5 * time.Second,
	}

	client := &Client{
		config:    cfg,
		sdkClient: &GCNVClient{},
	}
	client.sdkClient.resources = newGCNVResources()

	// Optionally populate capacity pools for discovery tests
	if len(withCapacityPools) > 0 && withCapacityPools[0] {
		cPools := map[string]*CapacityPool{
			"projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP1": {
				Name:            "CP1",
				FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP1",
				Location:        Location,
				ServiceLevel:    ServiceLevelPremium,
				State:           StateReady,
				NetworkName:     NetworkName,
				NetworkFullName: NetworkFullName,
			},
			"projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP2": {
				Name:            "CP2",
				FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP2",
				Location:        Location,
				ServiceLevel:    ServiceLevelStandard,
				State:           StateReady,
				NetworkName:     NetworkName,
				NetworkFullName: NetworkFullName,
			},
			"projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP3": {
				Name:            "CP3",
				FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP3",
				Location:        Location,
				ServiceLevel:    ServiceLevelExtreme,
				State:           StateReady,
				NetworkName:     NetworkName,
				NetworkFullName: NetworkFullName,
			},
			"projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP4": {
				Name:            "CP4",
				FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP4",
				Location:        Location,
				ServiceLevel:    ServiceLevelFlex,
				State:           StateReady,
				NetworkName:     NetworkName,
				NetworkFullName: NetworkFullName,
			},
			"projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP6": {
				Name:            "CP6",
				FullName:        "projects/" + ProjectNumber + "/locations/" + Location + "/storagePools/CP6",
				Location:        Location,
				ServiceLevel:    ServiceLevelStandard,
				State:           StateReady,
				NetworkName:     NetworkName,
				NetworkFullName: NetworkFullName,
			},
		}
		client.sdkClient.resources.capacityPools = collection.NewImmutableMap(cPools)
	}

	return client
}
