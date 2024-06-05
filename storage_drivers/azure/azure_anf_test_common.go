package azure

import (
	"context"
	"strconv"

	"github.com/netapp/trident/utils/errors"
)

const (
	SubscriptionID   = "1-subid-23456789876454321"
	TenantID         = "1-tenantid-23456789876454321"
	ClientID         = "1-clientid-23456789876454321"
	ClientSecret     = "client-secret-23456789876454321"
	Location         = "fake-location"
	BackendUUID      = "deadbeef-03af-4394-ace4-e177cdbcaf28"
	SnapshotID       = "deadbeef-5c0d-4afa-8cd8-afa3fba5665c"
	SnapshotName     = "snapshot-deadbeef-5c0d-4afa-8cd8-afa3fba5665c"
	VolumeSizeI64    = int64(107374182400)
	VolumeSizeStr    = "107374182400"
	SubvolumeSizeI64 = int64(20971520)
	SubvolumeSizeStr = "20971520"
)

var (
	ctx                  = context.Background()
	errFailed            = errors.New("failed")
	debugTraceFlags      = map[string]bool{"method": true, "api": true, "discovery": true}
	DefaultVolumeSize, _ = strconv.ParseInt(defaultVolumeSizeStr, 10, 64)
)
