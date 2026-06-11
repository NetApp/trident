// Copyright 2026 NetApp, Inc. All Rights Reserved.

package ontap

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/oauth2/google"

	. "github.com/netapp/trident/logging"
	drivers "github.com/netapp/trident/storage_drivers"
	"github.com/netapp/trident/storage_drivers/ontap/gcpapi"
)

// resolveCredentialsForGCNV is used by initializeGCNVDriver. When non-nil (e.g. in tests),
// it is called instead of gcpapi.ResolveCredentials so credential resolution can be stubbed
// without touching ADC or the network.
var resolveCredentialsForGCNV func(context.Context, *gcpapi.ClientConfig) (*google.Credentials, error)

// initializeGCNVDriver builds the GCNV ONTAP-mode transport from config. The returned
// http.RoundTripper rewrites all ONTAP REST URLs to the GCNV ONTAP-mode endpoint and
// injects GCP Bearer tokens. No management LIF, no ONTAP credentials, no ZAPI.
func initializeGCNVDriver(
	ctx context.Context, config *drivers.OntapStorageDriverConfig,
) (http.RoundTripper, error) {
	if config.GCNVConfig == nil {
		return nil, nil
	}

	g := config.GCNVConfig
	if g.ProjectNumber == "" || g.Location == "" || g.PoolID == "" {
		return nil, fmt.Errorf("gcnv ONTAP config requires projectNumber, location, and poolID")
	}
	if g.ProxyURL == "" {
		return nil, fmt.Errorf("gcnv ONTAP config requires proxyURL")
	}

	resolver := gcpapi.ResolveCredentials
	if resolveCredentialsForGCNV != nil {
		resolver = resolveCredentialsForGCNV
	}
	creds, err := resolver(ctx, &gcpapi.ClientConfig{
		ProjectNumber:   g.ProjectNumber,
		Location:        g.Location,
		ProxyURL:        g.ProxyURL,
		APIKey:          g.APIKey,
		WIPCredential:   g.WIPCredential,
		DebugTraceFlags: config.DebugTraceFlags,
	})
	if err != nil {
		return nil, fmt.Errorf("error resolving GCP credentials; %w", err)
	}

	if config.StoragePrefix == nil {
		prefix := drivers.GetDefaultStoragePrefix(config.DriverContext)
		config.StoragePrefix = &prefix
	}

	gcnvTransport, err := gcpapi.NewGCNVOntapModeTransport(&gcpapi.GCNVOntapModeConfig{
		ProxyURL:          g.ProxyURL,
		ProjectNumber:     g.ProjectNumber,
		Location:          g.Location,
		PoolID:            g.PoolID,
		StorageDriverName: config.StorageDriverName,
		DebugTraceFlags:   config.DebugTraceFlags,
		TokenSource:       creds.TokenSource,
	})
	if err != nil {
		return nil, fmt.Errorf("could not create GCNV ONTAP-mode transport: %v", err)
	}

	Logc(ctx).WithFields(LogFields{
		"proxyURL": g.ProxyURL,
		"location": g.Location,
		"poolID":   g.PoolID,
	}).Info("GCNV ONTAP-mode transport configured.")

	return gcnvTransport, nil
}
