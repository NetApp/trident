// Copyright 2020 NetApp, Inc. All Rights Reserved.
package kubernetes

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	k8sstoragev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/netapp/trident/config"
	frontendcommon "github.com/netapp/trident/frontend/common"
	"github.com/netapp/trident/frontend/csi"
	"github.com/netapp/trident/frontend/csi/helpers"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
)

/////////////////////////////////////////////////////////////////////////////
//
// This file contains the methods that implement the HybridPlugin interface.
//
/////////////////////////////////////////////////////////////////////////////

// GetVolumeConfig accepts the attributes of a volume being requested by
// the CSI provisioner, combines those with PVC and storage class info
// retrieved from the K8S API server, and returns a VolumeConfig structure
// as needed by Trident to create a new volume.
func (p *Plugin) GetVolumeConfig(
	ctx context.Context, name string, sizeBytes int64, parameters map[string]string,
	protocol config.Protocol, accessModes []config.AccessMode, volumeMode config.VolumeMode, fsType string,
	requisiteTopology, preferredTopology, accessibleTopology []map[string]string,
) (*storage.VolumeConfig, error) {

	// Kubernetes CSI passes us the name of what will become a new PV
	pvName := name

	fields := log.Fields{"Method": "GetVolumeConfig", "Type": "K8S helper", "name": pvName}
	Logc(ctx).WithFields(fields).Debug(">>>> GetVolumeConfig")
	defer Logc(ctx).WithFields(fields).Debug("<<<< GetVolumeConfig")

	// Get the PVC corresponding to the new PV being provisioned
	pvc, err := p.getPVCForCSIVolume(ctx, pvName)
	if err != nil {
		return nil, err
	}
	Logc(ctx).WithFields(log.Fields{
		"name":         pvc.Name,
		"namespace":    pvc.Namespace,
		"UID":          pvc.UID,
		"size":         pvc.Spec.Resources.Requests[v1.ResourceStorage],
		"storageClass": getStorageClassForPVC(pvc),
	}).Infof("Found PVC for requested volume %s.", pvName)

	// Validate the PVC
	if pvc.Status.Phase != v1.ClaimPending {
		return nil, fmt.Errorf("PVC %s is not in Pending state", pvc.Name)
	}
	pvcSize, ok := pvc.Spec.Resources.Requests[v1.ResourceStorage]
	if !ok {
		return nil, fmt.Errorf("PVC %s does not have a valid size", pvc.Name)
	}

	// Get the storage class requested by the PVC.  It should always be set by this time,
	// even if a default storage class was applied by the CSI provisioner.
	scName := getStorageClassForPVC(pvc)
	if scName == "" {
		return nil, fmt.Errorf("PVC %s does not specify a storage class", pvc.Name)
	}

	// Get the cached storage class for this PVC
	sc, err := p.getStorageClass(ctx, scName)
	if err != nil {
		return nil, err
	}
	Logc(ctx).WithField("name", sc.Name).Infof("Found storage class for requested volume %s.", pvName)

	// Validate the storage class
	if sc.Provisioner != csi.Provisioner {
		return nil, fmt.Errorf("the provisioner for storage class %s is not %s", sc.Name, csi.Provisioner)
	}

	// Create the volume config
	volumeConfig := getVolumeConfig(ctx, pvc.Spec.AccessModes, pvc.Spec.VolumeMode, pvName, pvcSize,
		processPVCAnnotations(pvc, fsType), sc, requisiteTopology, preferredTopology)

	// Check if we're cloning a PVC, and if so, do some further validation
	if cloneSourcePVName, err := p.getCloneSourceInfo(ctx, pvc); err != nil {
		return nil, err
	} else if cloneSourcePVName != "" {
		volumeConfig.CloneSourceVolume = cloneSourcePVName
	}

	return volumeConfig, nil
}

// getPVCForCSIVolume accepts the name of a volume being requested by the CSI provisioner,
// extracts the PVC name from the volume name, and returns the PVC object as read from the
// Kubernetes API server.  The method waits for the object to appear in cache, resyncs the
// cache if not found after an initial wait, and waits again after the resync.  This strategy
// is necessary because CSI only provides us with the PVC's UID, and the Kubernetes API does
// not support querying objects by UID.
func (p *Plugin) getPVCForCSIVolume(ctx context.Context, name string) (*v1.PersistentVolumeClaim, error) {

	// Get the PVC UID from the volume name.  The CSI provisioner sidecar creates
	// volume names of the form "pvc-<PVC_UID>".
	pvcUID, err := getPVCUIDFromCSIVolumeName(name)
	if err != nil {
		return nil, err
	}

	// Get the cached PVC that started this workflow
	pvc, err := p.waitForCachedPVCByUID(ctx, pvcUID, PreSyncCacheWaitPeriod)
	if err != nil {
		Logc(ctx).WithField("uid", pvcUID).Warningf("PVC not found in local cache: %v", err)

		// Not found immediately, so re-sync and try again
		if err = p.pvcIndexer.Resync(); err != nil {
			return nil, fmt.Errorf("could not refresh local PVC cache: %v", err)
		}

		if pvc, err = p.waitForCachedPVCByUID(ctx, pvcUID, PostSyncCacheWaitPeriod); err != nil {
			Logc(ctx).WithField("uid", pvcUID).Errorf("PVC not found in local cache after resync: %v", err)
			return nil, fmt.Errorf("could not find PVC with UID %s: %v", pvcUID, err)
		}
	}

	return pvc, nil
}

// getPVCUIDFromCSIVolumeName accepts the name of a CSI-requested volume and extracts
// the UID of the corresponding PVC using regex matching.  This obviously assumes that
// the Kubernetes CSI provisioner continues this naming scheme indefinitely.
func getPVCUIDFromCSIVolumeName(volumeName string) (string, error) {
	match := pvcRegex.FindStringSubmatch(volumeName)

	paramsMap := make(map[string]string)
	for i, name := range pvcRegex.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}

	if uid, ok := paramsMap["uid"]; !ok {
		return "", fmt.Errorf("volume name %s does not contain a uid", volumeName)
	} else {
		return uid, nil
	}
}

// getStorageClass accepts the name of a storage class and returns the storage class object
// as read from the Kubernetes API server.  The method waits for the object to appear in cache,
// resyncs the cache if not found after an initial wait, and waits again after the resync.
func (p *Plugin) getStorageClass(ctx context.Context, name string) (*k8sstoragev1.StorageClass, error) {

	sc, err := p.waitForCachedStorageClassByName(ctx, name, PreSyncCacheWaitPeriod)
	if err != nil {
		Logc(ctx).WithField("name", name).Warningf("Storage class not found in local cache: %v", err)

		// Not found immediately, so re-sync and try again
		if err = p.scIndexer.Resync(); err != nil {
			return nil, fmt.Errorf("could not refresh local storage class cache: %v", err)
		}

		if sc, err = p.waitForCachedStorageClassByName(ctx, name, PostSyncCacheWaitPeriod); err != nil {
			Logc(ctx).WithField("name", name).Errorf("Storage class not found in local cache after resync: %v", err)
			return nil, fmt.Errorf("could not find storage class %s: %v", name, err)
		}
	}

	return sc, nil
}

// getCloneSourceInfo accepts the PVC of a volume being provisioned by CSI and inspects it
// for the annotations indicating a clone operation (of which CSI is unaware). If a clone is
// being created, the method completes several checks on the source PVC/PV and returns the
// name of the source PV as needed by Trident to clone a volume as well as an optional
// snapshot name (also potentially unknown to CSI).  Note that these legacy clone annotations
// will be overridden if the VolumeContentSource is set in the CSI CreateVolume request.
func (p *Plugin) getCloneSourceInfo(ctx context.Context, clonePVC *v1.PersistentVolumeClaim) (string, error) {

	// Check if this is a clone operation
	annotations := processPVCAnnotations(clonePVC, "")
	sourcePVCName := getAnnotation(annotations, AnnCloneFromPVC)
	if sourcePVCName == "" {
		return "", nil
	}

	// Check that the source PVC is in the same namespace.
	// NOTE: For VolumeContentSource this check is performed by CSI
	sourcePVC, err := p.waitForCachedPVCByName(ctx, sourcePVCName, clonePVC.Namespace, PreSyncCacheWaitPeriod)
	if err != nil {
		Logc(ctx).WithFields(log.Fields{
			"sourcePVCName": sourcePVCName,
			"namespace":     clonePVC.Namespace,
		}).Errorf("Clone source PVC not found in local cache: %v", err)
		return "", fmt.Errorf("cloning from a PVC requires both PVCs be in the same namespace: %v", err)
	}

	// Check that both source and clone PVCs have the same storage class
	// NOTE: For VolumeContentSource this check is performed by CSI
	if getStorageClassForPVC(sourcePVC) != getStorageClassForPVC(clonePVC) {
		Logc(ctx).WithFields(log.Fields{
			"clonePVCName":          clonePVC.Name,
			"clonePVCNamespace":     clonePVC.Namespace,
			"clonePVCStorageClass":  getStorageClassForPVC(clonePVC),
			"sourcePVCName":         sourcePVC.Name,
			"sourcePVCNamespace":    sourcePVC.Namespace,
			"sourcePVCStorageClass": getStorageClassForPVC(sourcePVC),
		}).Error("Cloning from a PVC requires both PVCs have the same storage class.")
		return "", fmt.Errorf("cloning from a PVC requires both PVCs have the same storage class")
	}

	// Check that the source PVC has an associated PV
	sourcePVName := sourcePVC.Spec.VolumeName
	if sourcePVName == "" {
		Logc(ctx).WithFields(log.Fields{
			"sourcePVCName":      sourcePVC.Name,
			"sourcePVCNamespace": sourcePVC.Namespace,
		}).Error("Cloning from a PVC requires the source to be bound to a PV.")
		return "", fmt.Errorf("cloning from a PVC requires the source to be bound to a PV")
	}

	return sourcePVName, nil
}

// GetSnapshotConfig accepts the attributes of a snapshot being requested by the CSI
// provisioner and returns a SnapshotConfig structure as needed by Trident to create a new snapshot.
func (p *Plugin) GetSnapshotConfig(volumeName, snapshotName string) (*storage.SnapshotConfig, error) {
	return &storage.SnapshotConfig{
		Version:    config.OrchestratorAPIVersion,
		Name:       snapshotName,
		VolumeName: volumeName,
	}, nil
}

// RecordVolumeEvent accepts the name of a CSI volume (i.e. a PV name), finds the associated
// PVC, and posts and event message on the PVC object with the K8S API server.
func (p *Plugin) RecordVolumeEvent(ctx context.Context, name, eventType, reason, message string) {

	Logc(ctx).WithFields(log.Fields{
		"name":      name,
		"eventType": eventType,
		"reason":    reason,
		"message":   message,
	}).Debug("Volume event.")

	if pvc, err := p.getPVCForCSIVolume(ctx, name); err != nil {
		Logc(ctx).WithField("error", err).Debug("Failed to find PVC for event.")
	} else {
		p.eventRecorder.Event(pvc, mapEventType(eventType), reason, message)
	}
}

// mapEventType maps between K8S API event types and Trident CSI helper event types.  The
// two sets of types may be identical, but the CSI helper interface should not be tightly
// coupled to Kubernetes.
func mapEventType(eventType string) string {
	switch eventType {
	case helpers.EventTypeNormal:
		return v1.EventTypeNormal
	case helpers.EventTypeWarning:
		return v1.EventTypeWarning
	default:
		return v1.EventTypeWarning
	}
}

// getVolumeConfig accepts the attributes of a new volume and assembles them into a
// VolumeConfig structure as needed by Trident to create a new volume.
func getVolumeConfig(
	ctx context.Context, pvcAccessModes []v1.PersistentVolumeAccessMode, volumeMode *v1.PersistentVolumeMode,
	name string, size resource.Quantity, annotations map[string]string, storageClass *k8sstoragev1.StorageClass,
	requisiteTopology, preferredTopology []map[string]string,
) *storage.VolumeConfig {

	var accessModes []config.AccessMode

	for _, pvcAccessMode := range pvcAccessModes {
		accessModes = append(accessModes, config.AccessMode(pvcAccessMode))
	}

	accessMode := frontendcommon.CombineAccessModes(accessModes)

	if volumeMode == nil {
		volumeModeVal := v1.PersistentVolumeFilesystem
		volumeMode = &volumeModeVal
	}

	if getAnnotation(annotations, AnnFileSystem) == "" {
		annotations[AnnFileSystem] = "ext4"
	}

	if getAnnotation(annotations, AnnNotManaged) == "" {
		annotations[AnnNotManaged] = "false"
	}
	notManaged, err := strconv.ParseBool(getAnnotation(annotations, AnnNotManaged))
	if err != nil {
		Logc(ctx).Warnf("unable to parse notManaged annotation into bool; %v", err)
	}

	return &storage.VolumeConfig{
		Name:               name,
		Size:               fmt.Sprintf("%d", size.Value()),
		Protocol:           config.Protocol(getAnnotation(annotations, AnnProtocol)),
		SnapshotPolicy:     getAnnotation(annotations, AnnSnapshotPolicy),
		SnapshotReserve:    getAnnotation(annotations, AnnSnapshotReserve),
		SnapshotDir:        getAnnotation(annotations, AnnSnapshotDir),
		ExportPolicy:       getAnnotation(annotations, AnnExportPolicy),
		UnixPermissions:    getAnnotation(annotations, AnnUnixPermissions),
		StorageClass:        storageClass.Name,
		BlockSize:           getAnnotation(annotations, AnnBlockSize),
		FileSystem:          getAnnotation(annotations, AnnFileSystem),
		SplitOnClone:        getAnnotation(annotations, AnnSplitOnClone),
		VolumeMode:          config.VolumeMode(*volumeMode),
		AccessMode:          accessMode,
		ImportOriginalName:  getAnnotation(annotations, AnnImportOriginalName),
		ImportBackendUUID:   getAnnotation(annotations, AnnImportBackendUUID),
		ImportNotManaged:    notManaged,
		MountOptions:        strings.Join(storageClass.MountOptions, ","),
		RequisiteTopologies: requisiteTopology,
		PreferredTopologies: preferredTopology,
	}
}

// getAnnotation returns an annotation from a map, or an empty string if not found.
func getAnnotation(annotations map[string]string, key string) string {
	if val, ok := annotations[key]; ok {
		return val
	}
	return ""
}

// processPVCAnnotations returns the annotations from a PVC (ensuring a valid map even
// if empty). It also mixes in a Trident-standard fsType annotation using the value supplied
// *if* one isn't already set in the PVC annotation map.
func processPVCAnnotations(pvc *v1.PersistentVolumeClaim, fsType string) map[string]string {

	annotations := pvc.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Set the file system from the PVC annotations.  If not present, fall back to the CSI request,
	// which should have read the file system from the storage class (if specified there).
	if _, found := annotations[AnnFileSystem]; !found && fsType != "" {
		annotations[AnnFileSystem] = fsType
	}

	return annotations
}
