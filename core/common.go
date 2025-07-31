// Copyright 2025 NetApp, Inc. All Rights Reserved.

package core

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core/metrics"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/models"
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

func generateVolumePublication(volName string, publishInfo *models.VolumePublishInfo) *models.VolumePublication {
	vp := &models.VolumePublication{
		Name:       models.GenerateVolumePublishName(volName, publishInfo.HostName),
		VolumeName: volName,
		NodeName:   publishInfo.HostName,
		ReadOnly:   publishInfo.ReadOnly,
		AccessMode: publishInfo.AccessMode,
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
