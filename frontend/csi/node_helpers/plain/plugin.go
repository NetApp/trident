// Copyright 2022 NetApp, Inc. All Rights Reserved.

package plain

import (
	"context"

	log "github.com/sirupsen/logrus"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/frontend/csi"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	. "github.com/netapp/trident/logger"
)

type helper struct {
	orchestrator                     core.Orchestrator
	nodehelpers.VolumePublishManager // Embedded/extended interface
}

// NewHelper instantiates this plugin.
func NewHelper(orchestrator core.Orchestrator) frontend.Plugin {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourceInternal)
	Logc(ctx).Info("Initializing plain CSI helper frontend.")

	volPubManager := csi.NewVolumePublishManager("")

	return &helper{
		orchestrator:         orchestrator,
		VolumePublishManager: volPubManager,
	}
}

// Activate starts this Trident frontend.
func (h *helper) Activate() error {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourceInternal)

	Logc(ctx).Info("Activating plain CSI helper frontend.")

	// Configure telemetry
	config.OrchestratorTelemetry.Platform = string(config.PlatformCSI)
	config.OrchestratorTelemetry.PlatformVersion = h.Version()

	return nil
}

// Deactivate stops this Trident frontend.
func (h *helper) Deactivate() error {
	log.Info("Deactivating plain CSI helper frontend.")
	return nil
}

// GetName returns the name of this Trident frontend.
func (h *helper) GetName() string {
	return nodehelpers.PlainCSIHelper
}

// Version returns the version of this Trident frontend (the Trident version).
func (h *helper) Version() string {
	return csi.Version
}

func (h *helper) AddPublishedPath(ctx context.Context, volumeID, pathToAdd string) error {
	trackingInfo, err := h.ReadTrackingInfo(ctx, volumeID)
	if err != nil {
		return err
	}
	trackingInfo.PublishedPaths[pathToAdd] = struct{}{}
	return h.WriteTrackingInfo(ctx, volumeID, trackingInfo)
}

func (h *helper) RemovePublishedPath(ctx context.Context, volumeID, pathToRemove string) error {
	trackingInfo, err := h.ReadTrackingInfo(ctx, volumeID)
	if err != nil {
		return err
	}
	delete(trackingInfo.PublishedPaths, pathToRemove)
	return h.WriteTrackingInfo(ctx, volumeID, trackingInfo)
}
