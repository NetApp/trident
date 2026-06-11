// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	tridentconfig "github.com/netapp/trident/config"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/gcpapi"
)

func TestInitializeGCNVDriver_NilConfig(t *testing.T) {
	ctx := context.Background()
	commonConfig := &drivers.CommonStorageDriverConfig{StorageDriverName: "ontap-san"}
	config := &drivers.OntapStorageDriverConfig{CommonStorageDriverConfig: commonConfig, GCNVConfig: nil}

	_, err := initializeGCNVDriver(ctx, config)
	assert.NoError(t, err)
}

func TestInitializeGCNVDriver_MissingFields(t *testing.T) {
	ctx := context.Background()
	commonConfig := &drivers.CommonStorageDriverConfig{StorageDriverName: "ontap-san"}

	tests := []struct {
		name string
		gcnv *drivers.GCNVConfig
	}{
		{"missing projectNumber", &drivers.GCNVConfig{ProxyURL: "https://x.com", ProjectNumber: "", Location: "loc", PoolID: "p1"}},
		{"missing location", &drivers.GCNVConfig{ProxyURL: "https://x.com", ProjectNumber: "1", Location: "", PoolID: "p1"}},
		{"missing poolID", &drivers.GCNVConfig{ProxyURL: "https://x.com", ProjectNumber: "1", Location: "loc", PoolID: ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &drivers.OntapStorageDriverConfig{
				CommonStorageDriverConfig: commonConfig,
				GCNVConfig:                tt.gcnv,
			}
			_, err := initializeGCNVDriver(ctx, config)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "projectNumber, location, and poolID")
		})
	}
}

func TestInitializeGCNVDriver_MissingProxyURL(t *testing.T) {
	ctx := context.Background()
	commonConfig := &drivers.CommonStorageDriverConfig{StorageDriverName: "ontap-san"}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		GCNVConfig: &drivers.GCNVConfig{
			ProjectNumber: "123",
			Location:      "us-central1-a",
			PoolID:        "pool-1",
			ProxyURL:      "",
		},
	}
	_, err := initializeGCNVDriver(ctx, config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "proxyURL")
}

func TestInitializeGCNVDriver_Success(t *testing.T) {
	ctx := context.Background()
	commonConfig := &drivers.CommonStorageDriverConfig{
		StorageDriverName: "ontap-san",
		DriverContext:     tridentconfig.ContextCSI,
	}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		GCNVConfig: &drivers.GCNVConfig{
			ProjectNumber: "123",
			Location:      "us-central1-a",
			PoolID:        "pool-1",
			ProxyURL:      "https://netapp.googleapis.com",
		},
	}

	prev := resolveCredentialsForGCNV
	resolverCalled := false
	resolveCredentialsForGCNV = func(_ context.Context, _ *gcpapi.ClientConfig) (*google.Credentials, error) {
		resolverCalled = true
		return &google.Credentials{
			TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"}),
		}, nil
	}
	defer func() {
		resolveCredentialsForGCNV = prev
	}()

	transport, err := initializeGCNVDriver(ctx, config)
	assert.NoError(t, err)
	assert.NotNil(t, transport)
	assert.True(t, resolverCalled, "expected GCNV credential resolver to be called")
	assert.NotNil(t, config.StoragePrefix, "expected initializeGCNVDriver to populate default storage prefix")
}

func TestInitializeGCNVDriver_ResolverError(t *testing.T) {
	ctx := context.Background()
	commonConfig := &drivers.CommonStorageDriverConfig{StorageDriverName: "ontap-san"}
	config := &drivers.OntapStorageDriverConfig{
		CommonStorageDriverConfig: commonConfig,
		GCNVConfig: &drivers.GCNVConfig{
			ProjectNumber: "123",
			Location:      "us-central1-a",
			PoolID:        "pool-1",
			ProxyURL:      "https://netapp.googleapis.com",
		},
	}

	const stubErr = "stubbed resolver failure"
	prev := resolveCredentialsForGCNV
	resolveCredentialsForGCNV = func(_ context.Context, _ *gcpapi.ClientConfig) (*google.Credentials, error) {
		return nil, fmt.Errorf("%s", stubErr)
	}
	defer func() {
		resolveCredentialsForGCNV = prev
	}()

	transport, err := initializeGCNVDriver(ctx, config)
	assert.Nil(t, transport)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "error resolving GCP credentials")
	assert.Contains(t, err.Error(), stubErr)
}
