package gcp

import (
	"context"
	"strconv"

	"github.com/netapp/trident/utils/errors"
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
	NetworkFullName = "projects/" + ProjectNumber + "/locations/" + Location + "/networks/" + Network
	FullVolumeName  = "projects/" + ProjectNumber + "/locations/" + Location + "/volumes/"
)

var (
	ctx                  = context.Background()
	errFailed            = errors.New("failed")
	debugTraceFlags      = map[string]bool{"method": true, "api": true, "discovery": true}
	DefaultVolumeSize, _ = strconv.ParseInt(defaultVolumeSizeStr, 10, 64)
)
