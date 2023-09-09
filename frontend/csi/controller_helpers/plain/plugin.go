// Copyright 2022 NetApp, Inc. All Rights Reserved.

package plain

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend"
	frontendcommon "github.com/netapp/trident/frontend/common"
	"github.com/netapp/trident/frontend/csi"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
)

type helper struct {
	orchestrator core.Orchestrator
}

// NewHelper instantiates this plugin.
func NewHelper(orchestrator core.Orchestrator) frontend.Plugin {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginCreate, LogLayerCSIFrontend)
	Logc(ctx).Info("Initializing plain CSI helper frontend.")

	return &helper{
		orchestrator: orchestrator,
	}
}

// Activate starts this Trident frontend.
func (h *helper) Activate() error {
	Log().Info("Activating plain CSI helper frontend.")

	// Configure telemetry
	config.OrchestratorTelemetry.Platform = string(config.PlatformCSI)
	config.OrchestratorTelemetry.PlatformVersion = h.Version()

	// TODO (websterj): Revisit or remove this reconcile once Trident v21.10.1 has reached EOL;
	// At that point, all supported Trident versions will include volume publications.
	err := h.orchestrator.ReconcileVolumePublications(context.Background(), nil)
	if err != nil {
		return csi.TerminalReconciliationError(err.Error())
	}

	return nil
}

// Deactivate stops this Trident frontend.
func (h *helper) Deactivate() error {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginDeactivate, LogLayerCSIFrontend)
	Logc(ctx).Info("Deactivating plain CSI helper frontend.")
	return nil
}

// GetName returns the name of this Trident frontend.
func (h *helper) GetName() string {
	return controllerhelpers.PlainCSIHelper
}

// Version returns the version of this Trident frontend (the Trident version).
func (h *helper) Version() string {
	return csi.Version
}

// GetVolumeConfig accepts the attributes of a volume being requested by the CSI
// provisioner, finds or creates/registers a matching storage class, and returns
// a VolumeConfig structure as needed by Trident to create a new volume.
func (h *helper) GetVolumeConfig(
	ctx context.Context, name string, sizeBytes int64, parameters map[string]string,
	protocol config.Protocol, accessModes []config.AccessMode, volumeMode config.VolumeMode, fsType string,
	requisiteTopology, preferredTopology, _ []map[string]string,
) (*storage.VolumeConfig, error) {
	accessMode := frontendcommon.CombineAccessModes(ctx, accessModes)

	if parameters == nil {
		parameters = make(map[string]string)
	}

	if _, ok := parameters["fstype"]; !ok {
		parameters["fstype"] = fsType
	}

	// Find a matching storage class, or register a new one
	scConfig, err := frontendcommon.GetStorageClass(ctx, parameters, h.orchestrator)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "could not create a storage class from volume request")
	}

	// Create the volume config from all available info from the CSI request
	return frontendcommon.GetVolumeConfig(ctx, name, scConfig.Name, sizeBytes, parameters, protocol, accessMode,
		volumeMode, requisiteTopology, preferredTopology)
}

// GetSnapshotConfigForCreate accepts the attributes of a snapshot being requested by the CSI
// provisioner and returns a SnapshotConfig structure as needed by Trident to create a new snapshot.
func (h *helper) GetSnapshotConfigForCreate(volumeName, snapshotName string) (*storage.SnapshotConfig, error) {
	return &storage.SnapshotConfig{
		Version:    config.OrchestratorAPIVersion,
		Name:       snapshotName,
		VolumeName: volumeName,
	}, nil
}

// GetSnapshotConfigForImport accepts the attributes of a snapshot being requested by the CSI
// provisioner and returns a SnapshotConfig structure as needed by Trident to import a snapshot.
func (h *helper) GetSnapshotConfigForImport(
	_ context.Context, volumeName, snapshotName string,
) (*storage.SnapshotConfig, error) {
	return &storage.SnapshotConfig{
		Version:    config.OrchestratorAPIVersion,
		Name:       snapshotName,
		VolumeName: volumeName,
	}, nil
}

func (h *helper) GetNodeTopologyLabels(_ context.Context, _ string) (map[string]string, error) {
	return map[string]string{}, nil
}

// GetNodePublicationState returns a set of flags that indicate whether, in certain circumstances,
// a node may safely publish volumes.  If such checking is not enabled or otherwise appropriate,
// this function returns nil.
func (h *helper) GetNodePublicationState(_ context.Context, _ string) (*utils.NodePublicationStateFlags, error) {
	return nil, nil
}

// RecordVolumeEvent accepts the name of a CSI volume and writes the specified
// event message to the debug Log().
func (h *helper) RecordVolumeEvent(ctx context.Context, name, eventType, reason, message string) {
	Logc(ctx).WithFields(LogFields{
		"name":      name,
		"eventType": eventType,
		"reason":    reason,
		"message":   message,
	}).Trace("Volume event.")
}

// RecordNodeEvent accepts the name of a CSI node and writes the specified
// event message to the debug Log().
func (h *helper) RecordNodeEvent(ctx context.Context, name, eventType, reason, message string) {
	Logc(ctx).WithFields(LogFields{
		"name":      name,
		"eventType": eventType,
		"reason":    reason,
		"message":   message,
	}).Trace("Node event.")
}

// IsValidResourceName determines if a string meets the CO schema for naming objects.
func (h *helper) IsValidResourceName(_ string) bool {
	return true
}

// SupportsFeature accepts a CSI feature and returns true if the
// feature exists and is supported.
func (h *helper) SupportsFeature(_ context.Context, feature controllerhelpers.Feature) bool {
	if supported, ok := features[feature]; ok {
		return supported
	} else {
		return false
	}
}
