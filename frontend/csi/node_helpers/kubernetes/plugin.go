// Copyright 2025 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend"
	"github.com/netapp/trident/frontend/csi"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/utils/mount"
)

var osFs = afero.NewOsFs()

const (
	kubeDirEnvVar          = "KUBELET_DIR"
	volumesFilesystemPath  = "/volumes/kubernetes.io~csi/"
	rawDevicePublishedPath = "/plugins/kubernetes.io/csi/volumeDevices/publish/"
)

type helper struct {
	orchestrator                     core.Orchestrator
	podsPath                         string
	kubeConfigPath                   string
	publishedPaths                   map[string]map[string]struct{}
	enableForceDetach                bool
	mount                            mount.Mount
	nodehelpers.VolumePublishManager // Embedded/extended interface
}

// NewHelper instantiates this helper when running outside a pod.
func NewHelper(orchestrator core.Orchestrator, kubeConfigPath string, enableForceDetach bool) (frontend.Plugin, error) {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginCreate, LogLayerCSIFrontend)

	Logc(ctx).Info("Initializing K8S helper frontend.")

	if kubeConfigPath == "" {
		kubeConfigPath = os.Getenv(kubeDirEnvVar)
	}

	mountClient, err := mount.New()
	if err != nil {
		return nil, fmt.Errorf("could not initialize mount client; %v", err)
	}

	publishManager, err := csi.NewVolumePublishManager(config.VolumeTrackingInfoPath)
	if err != nil {
		return nil, fmt.Errorf("could not initialize VolumePublishManager; %v", err)
	}

	h := &helper{
		orchestrator:         orchestrator,
		podsPath:             kubeConfigPath + "/pods",
		kubeConfigPath:       kubeConfigPath,
		publishedPaths:       make(map[string]map[string]struct{}),
		enableForceDetach:    enableForceDetach,
		mount:                mountClient,
		VolumePublishManager: publishManager,
	}

	return h, nil
}

// Activate starts this Trident frontend.
func (h *helper) Activate() error {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginActivate, LogLayerCSIFrontend)

	Logc(ctx).Info("Activating K8S helper frontend.")

	if err := h.reconcileVolumePublishInfo(ctx); err != nil {
		Logc(ctx).WithError(err).Error("Could not reconcile volume publish info.")
		return err
	}

	// Configure telemetry
	config.OrchestratorTelemetry.Platform = string(config.PlatformKubernetes)
	config.OrchestratorTelemetry.PlatformVersion = h.Version()

	return nil
}

// Deactivate stops this Trident frontend.
func (h *helper) Deactivate() error {
	Log().Info("Deactivating K8S helper frontend.")
	return nil
}

// GetName returns the name of this Trident frontend.
func (h *helper) GetName() string {
	return nodehelpers.KubernetesHelper
}

// Version returns the version of this Trident frontend (the detected K8S version).
func (h *helper) Version() string {
	return "unknown"
}

// reconcileVolumePublishInfo checks the /var/lib/trident/tracking directory for volume tracking files and checks
// whether they are still valid. This lives here and not on the VolumePublishTracker because Reconciliation
// needs to be container orchestrator specific.
func (h *helper) reconcileVolumePublishInfo(ctx context.Context) error {
	Logc(ctx).Trace(">>>> ReconcileVolumeTrackingInfo")
	defer Logc(ctx).Trace("<<<< ReconcileVolumeTrackingInfo")

	files, err := h.VolumePublishManager.GetVolumeTrackingFiles()
	if err != nil {
		return fmt.Errorf("could not find volume tracking info files; %s", err)
	}

	if len(files) > 0 {
		publishedPaths, err := h.discoverPVCsToPublishedPathsFilesystemVolumes(ctx)
		if err != nil {
			return fmt.Errorf("could not discover published paths: %v", err)
		}

		err = h.discoverPVCsToPublishedPathsRawDevices(ctx, publishedPaths)
		if err != nil {
			return fmt.Errorf("could not discover published raw devices: %v", err)
		}

		h.publishedPaths = publishedPaths
	}

	pvToDeviceMappings, err := h.mount.PVMountpointMappings(ctx)
	if err != nil {
		Logc(ctx).Errorf("Unable to get devices for mounted PVs.")
	}

	for _, file := range files {
		err := h.reconcileVolumePublishInfoFile(ctx, file.Name(), pvToDeviceMappings)
		if err != nil {
			return err
		}
	}

	return nil
}

func (h *helper) reconcileVolumePublishInfoFile(
	ctx context.Context, file string, pvToDeviceMappings map[string]string,
) error {
	volumeId := strings.ReplaceAll(file, ".json", "")
	paths, ok := h.publishedPaths[volumeId]
	if !ok {
		paths = make(map[string]struct{})
		h.publishedPaths[volumeId] = paths
		Log().Warningf("Could not determine determine published paths for volume: %s", volumeId)
	}

	shouldDelete, err := h.VolumePublishManager.UpgradeVolumeTrackingFile(ctx, volumeId, paths, pvToDeviceMappings)
	if err != nil {
		Log().Infof("Volume tracking file upgrade failed for volume: %s .", volumeId)
		return err
	}

	if !shouldDelete {
		shouldDelete, err = h.VolumePublishManager.ValidateTrackingFile(ctx, volumeId)
		if err != nil {
			Log().Debug(fmt.Sprintf("Volume tracking file for volume: %s failed during validation.", volumeId))
			return err
		}
	}

	// If none of the conditions we expect to be true for a volume to be present are true, even after checking protocol
	// specific conditions, then we delete the tracking file. Even one true condition signifies that either a valid
	// volume is present or that a forced NodeUnstage (which is part of a force detach) needs to occur to ensure
	// everything is cleaned up properly.
	if shouldDelete {
		if err = h.VolumePublishManager.DeleteTrackingInfo(ctx, volumeId); err != nil {
			return csi.TerminalReconciliationError(fmt.Sprintf("could not delete the tracking file: %v", err))
		}
	}

	return nil
}

// AddPublishedPath adds a new published path to the tracking file when called by NodePublishVolume.
func (h *helper) AddPublishedPath(ctx context.Context, volumeID, pathToAdd string) error {
	fields := LogFields{"volumeID": volumeID}
	Logc(ctx).WithFields(fields).Trace(">>>> AddPublishedPath")
	defer Logc(ctx).WithFields(fields).Trace("<<<< AddPublishedPath")

	volTrackingInfo, err := h.ReadTrackingInfo(ctx, volumeID)
	if err != nil {
		return fmt.Errorf("failed to read the tracking file; %v", err)
	}

	volTrackingInfo.PublishedPaths[pathToAdd] = struct{}{}

	if err := h.WriteTrackingInfo(ctx, volumeID, volTrackingInfo); err != nil {
		return fmt.Errorf("failed to update the tracking file; %v", err)
	}

	h.publishedPaths[volumeID] = volTrackingInfo.PublishedPaths
	return nil
}

// RemovePublishedPath is called by NodeUnpublishVolume, removes the provided publish path from the tracking file, while
// leaving all other published paths unmodified.
func (h *helper) RemovePublishedPath(ctx context.Context, volumeID, pathToRemove string) error {
	fields := LogFields{"volumeID": volumeID}
	Logc(ctx).WithFields(fields).Trace(">>>> RemovePublishedPath")
	defer Logc(ctx).WithFields(fields).Trace("<<<< RemovePublishedPath")

	volTrackingInfo, err := h.ReadTrackingInfo(ctx, volumeID)
	if err != nil {
		return fmt.Errorf("failed to read the tracking file; %w", err)
	}

	delete(volTrackingInfo.PublishedPaths, pathToRemove)

	if err := h.WriteTrackingInfo(ctx, volumeID, volTrackingInfo); err != nil {
		return fmt.Errorf("failed to update the tracking file; %w", err)
	}

	h.publishedPaths[volumeID] = volTrackingInfo.PublishedPaths
	return nil
}

// discoverPVCsToPublishedPathsFilesystemVolumes builds a map of PVCs to the Pods they are mounted to and returns it.
func (h *helper) discoverPVCsToPublishedPathsFilesystemVolumes(ctx context.Context) (map[string]map[string]struct{}, error) {
	// VolumeID -> PublishPaths
	mapping := make(map[string]map[string]struct{})

	Logc(ctx).Debug("Discovering PVC mount points...")
	pods, err := afero.ReadDir(osFs, h.podsPath)
	if err != nil {
		fields := LogFields{"helperPodsPath": h.podsPath}
		Logc(ctx).WithFields(fields).Errorf("Error reading pods path; %v", err)
		return nil, err
	}

	for _, pod := range pods {
		podUUIDPath := filepath.Join(h.podsPath, pod.Name(), volumesFilesystemPath)
		fields := LogFields{"podUUID": podUUIDPath}
		Logc(ctx).WithFields(fields).Debug("Current pod UUID path.")
		volumes, err := afero.ReadDir(osFs, podUUIDPath)
		if err != nil && !os.IsNotExist(err) {
			Logc(ctx).WithFields(fields).Errorf("Error reading pod UUID directory; %v", err)
			return mapping, err
		}

		for _, volume := range volumes {
			if mapping[volume.Name()] == nil {
				mapping[volume.Name()] = make(map[string]struct{})
			}
			if strings.Contains(volume.Name(), "pvc-") {
				pubPath := filepath.Join(podUUIDPath, volume.Name(), "mount")
				fields := LogFields{"volumeId": volume.Name(), "publishedPath": pubPath}
				Logc(ctx).WithFields(fields).Debug("Found published path for volume.")
				mapping[volume.Name()][pubPath] = struct{}{}
			}
		}
	}

	Logc(ctx).WithFields(LogFields{"publishedPaths": mapping}).Debug("Discovered PVC mount points.")
	return mapping, nil
}

// discoverPVCsToPublishedPathsRawDevices builds a map of PVCs (
// raw blocks) to the Pods they are mounted to and returns it.
func (h *helper) discoverPVCsToPublishedPathsRawDevices(ctx context.Context, mapping map[string]map[string]struct{}) error {
	// VolumeID -> PublishPaths

	if mapping == nil {
		mapping = make(map[string]map[string]struct{})
	}

	Logc(ctx).Debug("Discovering PVC attachements...")
	publishedRawDevicePath := filepath.Join(h.kubeConfigPath, rawDevicePublishedPath)
	fields := LogFields{"publishedRawDevicePath": publishedRawDevicePath}

	volumes, err := afero.ReadDir(osFs, publishedRawDevicePath)
	if err != nil && !os.IsNotExist(err) {
		Logc(ctx).WithFields(fields).Errorf("Error reading raw device directory; %v", err)
		return err
	}

	for _, volume := range volumes {
		if !strings.Contains(volume.Name(), "pvc-") {
			continue
		}

		if mapping[volume.Name()] == nil {
			mapping[volume.Name()] = make(map[string]struct{})
		}

		volumePath := filepath.Join(publishedRawDevicePath, volume.Name())
		fields = LogFields{"volumePath": volumePath}
		podUUIDs, err := afero.ReadDir(osFs, volumePath)
		if err != nil && !os.IsNotExist(err) {
			Logc(ctx).WithFields(fields).Errorf("Error reading pods path; %v", err)
			return err
		}

		for _, podUUID := range podUUIDs {
			pubPath := filepath.Join(volumePath, podUUID.Name())
			fields := LogFields{"volumeId": volume.Name(), "publishedPath": pubPath}
			Logc(ctx).WithFields(fields).Debug("Found published path for volume.")
			mapping[volume.Name()][pubPath] = struct{}{}
		}

	}

	Logc(ctx).WithFields(LogFields{"publishedPaths": mapping}).Debug("Discovered PVC raw device mount points.")
	return nil
}
