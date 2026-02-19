// Copyright 2025 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"
	"fmt"
	"os"
	"time"

	"golang.org/x/time/rate"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core/metrics"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/models"
)

const (
	NodeAccessReconcilePeriod      = time.Second * 30
	NodeRegistrationCooldownPeriod = time.Second * 30
	AttachISCSIVolumeTimeoutLong   = time.Second * 90

	// VP sync rate limiter constants
	// Limits VP field propagation to 1 QPS with burst of 2
	vpUpdateRateLimit = 1
	vpUpdateBurst     = 2
)

var (
	vpSyncRateLimiter *rate.Limiter // Rate limiter for VP field updates

	FlagConcurrent = "concurrent"
)

// recordTiming is used to record in Prometheus the total time taken for an operation as follows:
//
//	defer recordTiming("backend_add")()
//
// see also: https://play.golang.org/p/6xRXlhFdqBd
func recordTiming(operation string, err *error) func() {
	startTime := time.Now()
	return func() {
		endTime := time.Since(startTime)
		endTimeMS := float64(endTime.Milliseconds())
		success := "true"
		if *err != nil {
			success = "false"
		}
		metrics.OperationDurationInMsSummary.WithLabelValues(operation, success).Observe(endTimeMS)
	}
}

func recordTransactionTiming(txn *storage.VolumeTransaction, err *error) {
	if txn == nil || txn.VolumeCreatingConfig == nil {
		// for unit tests, there will be no txn to record
		return
	}

	operation := "transaction_volume_finish"

	startTime := txn.VolumeCreatingConfig.StartTime
	endTime := time.Since(startTime)
	endTimeMS := float64(endTime.Milliseconds())
	success := "true"
	if *err != nil {
		success = "false"
	}
	metrics.OperationDurationInMsSummary.WithLabelValues(operation, success).Observe(endTimeMS)
}

// getProtocol returns the appropriate protocol based on a specified volume mode, access mode and protocol, or
// an error if the two settings are incompatible.
// NOTE: We do not support raw block volumes with NFS protocol.
func getProtocol(
	ctx context.Context, volumeMode config.VolumeMode, accessMode config.AccessMode, protocol config.Protocol,
) (config.Protocol, error) {
	Logc(ctx).WithFields(LogFields{
		"volumeMode": volumeMode,
		"accessMode": accessMode,
		"protocol":   protocol,
	}).Debug("Orchestrator#getProtocol")

	resultProtocol := protocol
	var err error = nil

	defer Logc(ctx).WithFields(LogFields{
		"resultProtocol": resultProtocol,
		"err":            err,
	}).Debug("Orchestrator#getProtocol")

	type accessVariables struct {
		volumeMode config.VolumeMode
		accessMode config.AccessMode
		protocol   config.Protocol
	}
	type protocolResult struct {
		protocol config.Protocol
		err      error
	}

	err = fmt.Errorf("incompatible volume mode (%s), access mode (%s) and protocol (%s)", volumeMode, accessMode,
		protocol)

	protocolTable := map[accessVariables]protocolResult{
		{config.Filesystem, config.ModeAny, config.ProtocolAny}: {config.ProtocolAny, nil},
		{config.Filesystem, config.ModeAny, config.File}:        {config.File, nil},
		{config.Filesystem, config.ModeAny, config.Block}:       {config.Block, nil},

		{config.Filesystem, config.ReadWriteOnce, config.ProtocolAny}: {config.ProtocolAny, nil},
		{config.Filesystem, config.ReadWriteOnce, config.File}:        {config.File, nil},
		{config.Filesystem, config.ReadWriteOnce, config.Block}:       {config.Block, nil},

		{config.Filesystem, config.ReadWriteOncePod, config.ProtocolAny}: {config.ProtocolAny, nil},
		{config.Filesystem, config.ReadWriteOncePod, config.File}:        {config.File, nil},
		{config.Filesystem, config.ReadWriteOncePod, config.Block}:       {config.Block, nil},

		{config.Filesystem, config.ReadOnlyMany, config.ProtocolAny}: {config.ProtocolAny, nil},
		{config.Filesystem, config.ReadOnlyMany, config.File}:        {config.File, nil},
		{config.Filesystem, config.ReadOnlyMany, config.Block}:       {config.Block, nil},

		{config.Filesystem, config.ReadWriteMany, config.ProtocolAny}: {config.File, nil},
		{config.Filesystem, config.ReadWriteMany, config.File}:        {config.File, nil},
		{config.Filesystem, config.ReadWriteMany, config.Block}:       {config.ProtocolAny, err},

		{config.RawBlock, config.ModeAny, config.ProtocolAny}: {config.Block, nil},
		{config.RawBlock, config.ModeAny, config.File}:        {config.ProtocolAny, err},
		{config.RawBlock, config.ModeAny, config.Block}:       {config.Block, nil},

		{config.RawBlock, config.ReadWriteOnce, config.ProtocolAny}: {config.Block, nil},
		{config.RawBlock, config.ReadWriteOnce, config.File}:        {config.ProtocolAny, err},
		{config.RawBlock, config.ReadWriteOnce, config.Block}:       {config.Block, nil},

		{config.RawBlock, config.ReadWriteOncePod, config.ProtocolAny}: {config.Block, nil},
		{config.RawBlock, config.ReadWriteOncePod, config.File}:        {config.ProtocolAny, err},
		{config.RawBlock, config.ReadWriteOncePod, config.Block}:       {config.Block, nil},

		{config.RawBlock, config.ReadOnlyMany, config.ProtocolAny}: {config.Block, nil},
		{config.RawBlock, config.ReadOnlyMany, config.File}:        {config.ProtocolAny, err},
		{config.RawBlock, config.ReadOnlyMany, config.Block}:       {config.Block, nil},

		{config.RawBlock, config.ReadWriteMany, config.ProtocolAny}: {config.Block, nil},
		{config.RawBlock, config.ReadWriteMany, config.File}:        {config.ProtocolAny, err},
		{config.RawBlock, config.ReadWriteMany, config.Block}:       {config.Block, nil},
	}

	res, isValid := protocolTable[accessVariables{volumeMode, accessMode, protocol}]

	if !isValid {
		return config.ProtocolAny, fmt.Errorf("invalid volume mode (%s), access mode (%s) or protocol (%s)", volumeMode,
			accessMode, protocol)
	}

	return res.protocol, res.err
}

// isVolumeAutogrowIneligible returns true if the volume should be excluded
// from autogrow monitoring based on its configuration.
//
// A volume is ineligible if:
// 1. Unmanaged Import - Trident doesn't manage it, can't resize
// 2. Thick Provisioning on SAN volumes - space-reserved volumes can't be autogrown (NAS/file can autogrow)
// 3. Read-Only Clone - Immutable, can't be resized
func isVolumeAutogrowIneligible(volConfig *storage.VolumeConfig) bool {
	// Unmanaged imports are ineligible
	if volConfig.ImportNotManaged {
		return true
	}

	// Read-only clones are ineligible
	if volConfig.ReadOnlyClone {
		return true
	}

	// Thick provisioning (SpaceReserve == "volume") is ineligible for SAN volumes
	// NAS volumes can be autogrown regardless of thick/thin provisioning
	if volConfig.SpaceReserve == "volume" {
		protocol, err := getProtocol(context.Background(), volConfig.VolumeMode, volConfig.AccessMode, volConfig.Protocol)
		// If protocol determination succeeds and it's NOT file (i.e., it's block/SAN), the volume is ineligible
		if err == nil && protocol != config.File {
			return true
		}
	}

	return false
}

func generateVolumePublication(volName string, publishInfo *models.VolumePublishInfo, volConfig *storage.VolumeConfig) *models.VolumePublication {
	vp := &models.VolumePublication{
		Name:               models.GenerateVolumePublishName(volName, publishInfo.HostName),
		VolumeName:         volName,
		NodeName:           publishInfo.HostName,
		ReadOnly:           publishInfo.ReadOnly,
		AccessMode:         publishInfo.AccessMode,
		StorageClass:       publishInfo.StorageClass,
		BackendUUID:        publishInfo.BackendUUID,
		Pool:               publishInfo.Pool,
		AutogrowPolicy:     volConfig.RequestedAutogrowPolicy,
		AutogrowIneligible: isVolumeAutogrowIneligible(volConfig),
	}

	// Initialize labels with node name label
	vp.Labels = map[string]string{
		config.TridentNodeNameLabel: publishInfo.HostName,
	}

	return vp
}

// isDockerPluginMode returns true if the ENV variable config.DockerPluginModeEnvVariable is set
func isDockerPluginMode() bool {
	return os.Getenv(config.DockerPluginModeEnvVariable) != ""
}

func isCRDContext(ctx context.Context) bool {
	ctxSource := ctx.Value(ContextKeyRequestSource)
	return ctxSource != nil && ctxSource == ContextSourceCRD
}

func isPeriodicContext(ctx context.Context) bool {
	ctxSource := ctx.Value(ContextKeyRequestSource)
	return ctxSource != nil && ctxSource == ContextSourcePeriodic
}

// syncVolumePublicationFields compares TVol and VP fields and returns which fields need syncing.
// Modifies the VP in place to match the source volume's current state.
func syncVolumePublicationFields(vol *storage.Volume, vp *models.VolumePublication) bool {
	syncNeeded := false

	// Sync legacy fields (remove this logic once 26.02 reaches its EOL)
	{
		if vp.StorageClass != vol.Config.StorageClass {
			vp.StorageClass = vol.Config.StorageClass
			syncNeeded = true
		}

		if vp.BackendUUID != vol.BackendUUID {
			vp.BackendUUID = vol.BackendUUID
			syncNeeded = true
		}

		if vp.Pool != vol.Pool {
			vp.Pool = vol.Pool
			syncNeeded = true
		}

		isIneligible := isVolumeAutogrowIneligible(vol.Config)
		if vp.AutogrowIneligible != isIneligible {
			vp.AutogrowIneligible = isIneligible
			syncNeeded = true
		}

		// Check if labels need sync
		expectedLabels := map[string]string{
			config.TridentNodeNameLabel: vp.NodeName,
		}

		if labelsNeedSync(vp.Labels, expectedLabels) {
			// Initialize labels if nil
			if vp.Labels == nil {
				vp.Labels = make(map[string]string)
			}

			// Update labels
			for k, v := range expectedLabels {
				vp.Labels[k] = v
			}

			syncNeeded = true
		}
	}

	if vp.AutogrowPolicy != vol.Config.RequestedAutogrowPolicy {
		vp.AutogrowPolicy = vol.Config.RequestedAutogrowPolicy
		syncNeeded = true
	}

	return syncNeeded
}

// labelsNeedSync returns true if any desired label is missing from current or has a different value
func labelsNeedSync(current, desired map[string]string) bool {
	if current == nil && desired == nil {
		return false
	}

	// Check if all desired labels are present and have correct values
	for k, desiredVal := range desired {
		if currentVal, exists := current[k]; !exists || currentVal != desiredVal {
			return true
		}
	}

	return false
}
