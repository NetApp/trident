// Copyright 2026 NetApp, Inc. All Rights Reserved.

// Package gcpapi provides GCP credential resolution and the GCNV ONTAP proxy transport
// for ONTAP drivers running in GCNV ontap-mode.
package gcpapi

import (
	"context"
	"encoding/json"
	"fmt"

	netapp "cloud.google.com/go/netapp/apiv1"
	"golang.org/x/oauth2/google"

	. "github.com/netapp/trident/logging"
	drivers "github.com/netapp/trident/storage_drivers"
)

// ClientConfig holds configuration for GCP credential resolution.
type ClientConfig struct {
	ProjectNumber   string
	Location        string
	APIKey          *drivers.GCPPrivateKey
	WIPCredential   *drivers.GCPWIPCredential
	Credentials     *google.Credentials
	ProxyURL        string
	DebugTraceFlags map[string]bool
}

// ResolveCredentials resolves GCP credentials in priority order:
// 1. Workload Identity Pool (WIP) credential config
// 2. Service account key (GCPPrivateKey)
// 3. Application Default Credentials (ADC) - GKE workload identity, metadata server, etc.
// OAuth scopes use netapp.DefaultAuthScopes() to align with the existing GCNV driver (storage_drivers/gcp).
// Exported so callers can resolve once and share the TokenSource with the ONTAP proxy.
func ResolveCredentials(ctx context.Context, config *ClientConfig) (*google.Credentials, error) {
	if config == nil {
		return nil, fmt.Errorf("client config cannot be nil")
	}
	if config.WIPCredential != nil {
		Logc(ctx).Debug("gcpapi: Using GCP Workload Identity Pool credentials.")
		credBytes, err := json.Marshal(config.WIPCredential)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal WIP credential config: %w", err)
		}
		return google.CredentialsFromJSONWithType(
			ctx, credBytes, google.ExternalAccount, netapp.DefaultAuthScopes()...,
		)
	}

	if config.APIKey != nil && *config.APIKey != (drivers.GCPPrivateKey{}) {
		Logc(ctx).Debug("gcpapi: Using GCP service account key credentials.")
		keyBytes, err := json.Marshal(config.APIKey) //nolint:gosec // serialized for google.CredentialsFromJSON only
		if err != nil {
			return nil, fmt.Errorf("failed to marshal service account key: %w", err)
		}
		return google.CredentialsFromJSONWithType(
			ctx, keyBytes, google.ServiceAccount, netapp.DefaultAuthScopes()...,
		)
	}

	Logc(ctx).Debug("gcpapi: Using application default credentials.")
	return google.FindDefaultCredentials(ctx, netapp.DefaultAuthScopes()...)
}
