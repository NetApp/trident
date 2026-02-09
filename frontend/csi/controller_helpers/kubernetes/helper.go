// Copyright 2025 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	"gopkg.in/yaml.v2"
	v1 "k8s.io/api/core/v1"
	k8sstoragev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/config"
	frontendcommon "github.com/netapp/trident/frontend/common"
	"github.com/netapp/trident/frontend/csi"
	controllerhelpers "github.com/netapp/trident/frontend/csi/controller_helpers"
	. "github.com/netapp/trident/logging"
	netappv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/pkg/convert"
	"github.com/netapp/trident/pkg/network"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils/errors"
)

// ///////////////////////////////////////////////////////////////////////////
//
// This file contains the methods that implement the ControllerHelper interface.
//
// ///////////////////////////////////////////////////////////////////////////

// GetVolumeConfig accepts the attributes of a volume being requested by
// the CSI provisioner, combines those with PVC and storage class info
// retrieved from the K8S API server, and returns a VolumeConfig structure
// as needed by Trident to create a new volume.
func (h *helper) GetVolumeConfig(
	ctx context.Context, name string, _ int64, _ map[string]string, _ config.Protocol, _ []config.AccessMode,
	_ config.VolumeMode, fsType string, requisiteTopology, preferredTopology, _ []map[string]string,
	secrets map[string]string,
) (*storage.VolumeConfig, error) {
	// Kubernetes CSI passes us the name of what will become a new PV
	pvName := name

	fields := LogFields{"Method": "GetVolumeConfig", "Type": "K8S helper", "name": pvName}
	Logc(ctx).WithFields(fields).Trace(">>>> GetVolumeConfig")
	defer Logc(ctx).WithFields(fields).Trace("<<<< GetVolumeConfig")

	// Get the PVC corresponding to the new PV being provisioned
	pvc, err := h.getPVCForCSIVolume(ctx, pvName)
	if err != nil {
		return nil, err
	}
	Logc(ctx).WithFields(LogFields{
		"name":         pvc.Name,
		"namespace":    pvc.Namespace,
		"UID":          pvc.UID,
		"size":         pvc.Spec.Resources.Requests[v1.ResourceStorage],
		"storageClass": getStorageClassForPVC(pvc),
	}).Debugf("Found PVC for requested volume %s.", pvName)

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
	sc, err := h.getStorageClass(ctx, scName)
	if err != nil {
		return nil, err
	}
	Logc(ctx).WithField("name", sc.Name).Debugf("Found storage class for requested volume %s.", pvName)

	// Validate the storage class
	if sc.Provisioner != csi.Provisioner {
		return nil, fmt.Errorf("the provisioner for storage class %s is not %s", sc.Name, csi.Provisioner)
	}

	// Create the volume config
	annotations := processPVCAnnotations(pvc, fsType)

	// Process annotations from the storage class
	scAnnotations := processSCAnnotations(sc)

	volumeConfig := getVolumeConfig(ctx, pvc, pvName, pvcSize, annotations, sc, requisiteTopology, preferredTopology)

	// Update the volume config with the Access Control only if the storage class nasType parameter is SMB
	if sc.Parameters[SCParameterNASType] == NASTypeSMB {
		err = h.updateVolumeConfigWithSecureSMBAccessControl(ctx, volumeConfig, sc, annotations, scAnnotations, secrets)
		if err != nil {
			return nil, err
		}
	}

	// Get clone annotations
	sourceSnapshotName := getAnnotation(annotations, AnnCloneFromSnapshot)
	sourcePVCName := getAnnotation(annotations, AnnCloneFromPVC)

	// Check if we're cloning a PVC, and if so, do some further validation
	if sourcePVCName != "" && sourceSnapshotName == "" {
		if cloneSourcePVName, err := h.getCloneSourceInfo(ctx, pvc); err != nil {
			return nil, err
		} else if cloneSourcePVName != "" {
			volumeConfig.CloneSourceVolume = cloneSourcePVName
		}
	}

	// Check if we're cloning from a snapshot, and if so, do some further validation
	if sourceSnapshotName != "" {
		cloneSourceVolume, cloneSourceSnapshot, err := h.getSnapshotCloneSourceInfo(ctx, pvc)
		if err != nil {
			return nil, err
		} else if cloneSourceVolume != "" && cloneSourceSnapshot != "" {
			volumeConfig.CloneSourceVolume = cloneSourceVolume
			volumeConfig.CloneSourceSnapshot = cloneSourceSnapshot
		}
	}

	// Check if we're importing a volume and do some further validation
	if volumeConfig.ImportOriginalName != "" {
		// If this is a LUKS encrypted volume, ensure the storage class contains the expected parameters
		if volumeConfig.LUKSEncryption != "" {
			err = h.validateStorageClassParameters(sc, CSIParameterNodeStageSecretName, CSIParameterNodeStageSecretNamespace)
			if err != nil {
				Logc(ctx).WithError(err).Error("Invalid storage class parameters for LUKS volume import.")
				return nil, err
			}
		}
	}

	// Check for TMRs pointing to this PVC
	mirrorRelationshipName := getAnnotation(annotations, AnnMirrorRelationship)
	volumeConfig.PeerVolumeHandle, err = h.getPVCMirrorPeer(pvc, mirrorRelationshipName)
	if err != nil {
		Logc(ctx).WithError(err).Errorf("Issue discovering potential mirroring peers for PVC %v", pvc.Name)
		return nil, err
	}
	if mirrorRelationshipName != "" || volumeConfig.PeerVolumeHandle != "" {
		volumeConfig.IsMirrorDestination = true
	}

	// Check for subordinate volume PVC, denoted by an annotation that points to a TridentVolumeReference CR in the
	// same namespace as the new PVC having the same name as the source PV.  If the new volume is a subordinate, this
	// function updates the volume config with a reference to its parent.
	if err = h.validateSubordinateVolumeConfig(ctx, pvc, volumeConfig); err != nil {
		return nil, err
	}

	return volumeConfig, nil
}

// validateStorageClassParameters looks through a storage class and reports if any of the expected keys are not found.
func (h *helper) validateStorageClassParameters(sc *k8sstoragev1.StorageClass, keys ...string) error {
	var errs error
	for _, key := range keys {
		if _, ok := sc.Parameters[key]; !ok {
			errs = errors.Join(errs, errors.NotFoundError("storage class parameter [%s] not found", key))
		}
	}
	return errs
}

// getPVCMirrorPeer returns the spec.remoteVolumeHandle of the TMR pointing to this PVC
// or returns an error if the TMR cannot be discovered
func (h *helper) getPVCMirrorPeer(pvc *v1.PersistentVolumeClaim, mirrorRelationshipName string) (string, error) {
	var relationship *netappv1.TridentMirrorRelationship
	if mirrorRelationshipName != "" {
		relationshipObj, exists, err := h.mrIndexer.GetByKey(pvc.Namespace + "/" + mirrorRelationshipName)
		if err != nil {
			return "", err
		}
		if !exists {
			return "", fmt.Errorf("the TMR %s for PVC %s does not exist", mirrorRelationshipName, pvc.Name)
		}
		var ok bool
		relationship, ok = relationshipObj.(*netappv1.TridentMirrorRelationship)
		if !ok {
			return "", errors.TypeAssertionError("relationshipObj.(*netappv1.TridentMirrorRelationship)")
		}
	}
	// If TMR is pointing to PVC but PVC is missing annotation, we search for a single TMR pointing to the PVC
	relationships := h.mrIndexer.List()
	for index := range relationships {
		rel, ok := relationships[index].(*netappv1.TridentMirrorRelationship)
		if !ok {
			return "", errors.TypeAssertionError("relationships[index].(*netappv1.TridentMirrorRelationship)")
		}
		mappings := rel.Spec.VolumeMappings
		for index := range mappings {
			volMap := mappings[index]
			if volMap.LocalPVCName == pvc.Name && rel.Namespace == pvc.Namespace {
				if relationship == nil {
					relationship = rel
				} else if mirrorRelationshipName != "" && mirrorRelationshipName != relationship.Name {
					return "", fmt.Errorf("a different TMR refers to the PVC %v", pvc.Name)
				} else if mirrorRelationshipName == "" {
					return "", fmt.Errorf("multiple TMRs refer to PVC %v", pvc.Name)
				}
			}
		}
	}

	if relationship != nil {
		// If the desired state is established and a remote volume handle is set
		if len(relationship.Spec.VolumeMappings) > 0 && relationship.Spec.VolumeMappings[0].RemoteVolumeHandle != "" {
			if relationship.Spec.MirrorState == netappv1.MirrorStateEstablished ||
				relationship.Spec.MirrorState == netappv1.MirrorStateReestablished {

				remoteVolumeHandle := relationship.Spec.VolumeMappings[0].RemoteVolumeHandle
				if remoteVolumeHandle == "" {
					return "", fmt.Errorf("PVC '%v' linked to invalid TMR '%v'", pvc.Name, relationship.Name)
				}
				return remoteVolumeHandle, nil
			}
		}
	}
	return "", nil
}

// getPVCForCSIVolume accepts the name of a volume being requested by the CSI provisioner,
// extracts the PVC name from the volume name, and returns the PVC object as read from the
// Kubernetes API server.  The method waits for the object to appear in cache, resyncs the
// cache if not found after an initial wait, and waits again after the resync.  This strategy
// is necessary because CSI only provides us with the PVC's UID, and the Kubernetes API does
// not support querying objects by UID.
func (h *helper) getPVCForCSIVolume(ctx context.Context, name string) (*v1.PersistentVolumeClaim, error) {
	// Get the PVC UID from the volume name.  The CSI provisioner sidecar creates
	// volume names of the form "pvc-<PVC_UID>".
	pvcUID, err := getPVCUIDFromCSIVolumeName(name)
	if err != nil {
		return nil, err
	}

	// Get the cached PVC that started this workflow
	pvc, err := h.waitForCachedPVCByUID(ctx, pvcUID, PreSyncCacheWaitPeriod)
	if err != nil {
		Logc(ctx).WithError(err).WithField("uid", pvcUID).Warning("PVC not found in local cache.")

		// Not found immediately, so re-sync and try again
		if err = h.pvcIndexer.Resync(); err != nil {
			return nil, fmt.Errorf("could not refresh local PVC cache: %v", err)
		}

		if pvc, err = h.waitForCachedPVCByUID(ctx, pvcUID, PostSyncCacheWaitPeriod); err != nil {
			Logc(ctx).WithError(err).WithField("uid", pvcUID).Error("PVC not found in local cache after resync.")
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
func (h *helper) getStorageClass(ctx context.Context, name string) (*k8sstoragev1.StorageClass, error) {
	sc, err := h.waitForCachedStorageClassByName(ctx, name, PreSyncCacheWaitPeriod)
	if err != nil {
		Logc(ctx).WithError(err).WithField("name", name).Warning("Storage class not found in local cache.")

		// Not found immediately, so re-sync and try again
		if err = h.scIndexer.Resync(); err != nil {
			return nil, fmt.Errorf("could not refresh local storage class cache: %v", err)
		}

		if sc, err = h.waitForCachedStorageClassByName(ctx, name, PostSyncCacheWaitPeriod); err != nil {
			Logc(ctx).WithError(err).WithField("name", name).Error(
				"Storage class not found in local cache after resync.")
			return nil, fmt.Errorf("could not find storage class %s: %v", name, err)
		}
	}

	return sc, nil
}

// getSnapshotCloneSourceInfo accepts the PVC of a volume being provisioned by CSI and inspects it
// for the annotations indicating a snapshot clone operation (of which CSI is unaware).
// The method completes several checks on the source snapshot, and PVC if provided, and returns the
// name of the source PV and snapshot as needed by Trident to clone a volume.
func (h *helper) getSnapshotCloneSourceInfo(
	ctx context.Context, clonePVC *v1.PersistentVolumeClaim,
) (string, string, error) {
	// Check if this is a snapshot clone operation
	annotations := processPVCAnnotations(clonePVC, "")
	sourceSnapshotName := getAnnotation(annotations, AnnCloneFromSnapshot)
	if sourceSnapshotName == "" {
		return "", "", fmt.Errorf("annotation 'cloneFromSnapshot' is empty")
	}
	namespace := clonePVC.Namespace
	cloneFromNamespace := getAnnotation(annotations, AnnVolumeCloneFromNS)
	if cloneFromNamespace != "" {
		// Source snapshot is in a different namespace
		namespace = cloneFromNamespace
	}

	// Get the VolumeSnapshot
	snapshot, err := h.getVolumeSnapshot(ctx, sourceSnapshotName, namespace)
	if err != nil {
		return "", "", err
	}

	// If cloning from another namespace, ensure the source PVC has the required annotation
	if cloneFromNamespace != "" {
		snapSourcePVC, err := h.waitForCachedPVCByName(ctx, *snapshot.Spec.Source.PersistentVolumeClaimName, namespace, PreSyncCacheWaitPeriod)
		if err != nil {
			Logc(ctx).WithFields(LogFields{
				"sourcePVCName": snapSourcePVC.Name,
				"namespace":     namespace,
			}).Errorf("Clone source PVC not found in local cache: %v", err)
			return "", "", err
		}
		sourceAnnotations := processPVCAnnotations(snapSourcePVC, "")
		sourceCloneToNamespaces := getAnnotation(sourceAnnotations, AnnVolumeCloneToNS)
		// Ensure the source PVC has been explicitly allowed to clone to the subordinate PVC namespace
		if !h.matchNamespaceToAnnotation(clonePVC.Namespace, sourceCloneToNamespaces) {
			return "", "", fmt.Errorf("cloning to namespace %s is not allowed, it is not listed in cloneToNamespace annotation", clonePVC.Namespace)
		}
		// Get the volume reference CR
		_, err = h.getCachedVolumeReference(ctx, clonePVC.Namespace, snapSourcePVC.Name, namespace)
		if err != nil {
			return "", "", fmt.Errorf("volume reference not found: %v", err)
		}

	}
	// If the clone from PVC annotation is also set, ensure it matches the snapshot
	sourcePVCName := getAnnotation(annotations, AnnCloneFromPVC)
	if sourcePVCName != "" {
		snapSourcePVC := snapshot.Spec.Source.PersistentVolumeClaimName
		if snapSourcePVC == nil {
			return "", "", fmt.Errorf("cannot verify clone source PVC for snapshot '%s', "+
				"PersistentVolumeClaimName is not set in the snapshot spec", sourceSnapshotName)
		}
		if sourcePVCName != *snapSourcePVC {
			return "", "", fmt.Errorf("clone source snapshot '%s' does not originate from the given source "+
				"PVC '%s'", sourceSnapshotName, sourcePVCName)
		}
	}

	// Ensure the VolumeSnapshot is ready to use
	if snapshot.Status.ReadyToUse == nil || !*snapshot.Status.ReadyToUse {
		return "", "", fmt.Errorf("snapshot '%s' is not ready to use", snapshot.Name)
	}

	snapshotContent, err := h.getSnapshotContentFromSnapshot(ctx, snapshot)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"sourceSnapshotName": sourceSnapshotName,
		}).Errorf("Clone source snapshot content not found: %v", err)
		return "", "", err
	}

	// Ensure the VolumeSnapshotContent is ready to use
	if snapshotContent.Status.ReadyToUse == nil || !*snapshotContent.Status.ReadyToUse {
		return "", "", fmt.Errorf("volumeSnapshotContent '%s' is not ready to use", snapshotContent.Name)
	}

	if snapshotContent.Status.SnapshotHandle == nil {
		return "", "", fmt.Errorf("volumeSnapshotContent '%s' does not have a snapshot handle",
			snapshotContent.Name)
	}

	volumeName, snapshotName, err := storage.ParseSnapshotID(*snapshotContent.Status.SnapshotHandle)
	if err != nil {
		return "", "", err
	}

	return volumeName, snapshotName, nil
}

// getCloneSourceInfo accepts the PVC of a volume being provisioned by CSI and inspects it
// for the annotations indicating a PVC clone operation (of which CSI is unaware). If a clone is
// being created, the method completes several checks on the source PVC/PV and returns the
// name of the source PV as needed by Trident to clone a volume as well as an optional
// snapshot name (also potentially unknown to CSI).  Note that these legacy clone annotations
// will be overridden if the VolumeContentSource is set in the CSI CreateVolume request.
func (h *helper) getCloneSourceInfo(ctx context.Context, clonePVC *v1.PersistentVolumeClaim) (string, error) {
	// Check if this is a clone operation
	annotations := processPVCAnnotations(clonePVC, "")
	sourcePVCName := getAnnotation(annotations, AnnCloneFromPVC)
	if sourcePVCName == "" {
		return "", fmt.Errorf("annotation 'cloneFromPVC' is empty")
	}
	namespace := clonePVC.Namespace
	cloneFromNamespace := getAnnotation(annotations, AnnVolumeCloneFromNS)

	// Determine what namespace to look for the source PVC in
	if cloneFromNamespace != "" {
		// Source PVC is in a different namespace
		namespace = cloneFromNamespace
	}

	// Retrieve the source PVC from the cache
	sourcePVC, err := h.waitForCachedPVCByName(ctx, sourcePVCName, namespace, PreSyncCacheWaitPeriod)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"sourcePVCName": sourcePVCName,
			"namespace":     namespace,
		}).Errorf("Clone source PVC not found in local cache: %v", err)
		return "", fmt.Errorf("source PVC not found in namespace: %v", err)
	}

	// If cloning from another namespace, ensure the source PVC has the required annotation
	if cloneFromNamespace != "" {
		sourceAnnotations := processPVCAnnotations(sourcePVC, "")
		sourceCloneToNamespaces := getAnnotation(sourceAnnotations, AnnVolumeCloneToNS)
		// Ensure the source PVC has been explicitly allowed to clone to the subordinate PVC namespace
		if !h.matchNamespaceToAnnotation(clonePVC.Namespace, sourceCloneToNamespaces) {
			return "", fmt.Errorf("cloning to namespace %s is not allowed, it is not listed in cloneToNamespace annotation", clonePVC.Namespace)
		}
		// Get the volume reference CR
		_, err := h.getCachedVolumeReference(ctx, clonePVC.Namespace, sourcePVCName, namespace)
		if err != nil {
			return "", fmt.Errorf("volume reference not found: %v", err)
		}
	}

	// Check that both source and clone PVCs have the same storage class
	// NOTE: For VolumeContentSource this check is performed by CSI
	if getStorageClassForPVC(sourcePVC) != getStorageClassForPVC(clonePVC) {
		Logc(ctx).WithFields(LogFields{
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
		Logc(ctx).WithFields(LogFields{
			"sourcePVCName":      sourcePVC.Name,
			"sourcePVCNamespace": sourcePVC.Namespace,
		}).Error("Cloning from a PVC requires the source to be bound to a PV.")
		return "", fmt.Errorf("cloning from a PVC requires the source to be bound to a PV")
	}

	return sourcePVName, nil
}

func (h *helper) validateSubordinateVolumeConfig(
	ctx context.Context, subordinatePVC *v1.PersistentVolumeClaim, volConfig *storage.VolumeConfig,
) error {
	annotations := subordinatePVC.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Check if this is a subordinate volume.  If not, there is nothing to do and no reason to return an error.
	shareAnnotation := getAnnotation(annotations, AnnVolumeShareFromPVC)
	if shareAnnotation == "" {
		return nil
	}
	sourcePVCPathComponents := strings.Split(shareAnnotation, "/")
	if len(sourcePVCPathComponents) != 2 || sourcePVCPathComponents[0] == "" || sourcePVCPathComponents[1] == "" {
		return fmt.Errorf("%s annotation must have the format <pvcNamespace>/<pvcName>", AnnVolumeShareFromPVC)
	}
	sourcePVCNamespace := sourcePVCPathComponents[0]
	sourcePVCName := sourcePVCPathComponents[1]

	// Get the volume reference CR
	_, err := h.getCachedVolumeReference(ctx, subordinatePVC.Namespace, sourcePVCName, sourcePVCNamespace)
	if err != nil {
		return err
	}

	// Get the source PVC
	sourcePVC, err := h.getCachedPVCByName(ctx, sourcePVCName, sourcePVCNamespace)
	if err != nil {
		return err
	}

	// Ensure the source PVC has shared itself with the subordinate volume's namespace
	sourceAnnotations := sourcePVC.Annotations
	if sourceAnnotations == nil {
		sourceAnnotations = make(map[string]string)
	}
	shareToAnnotation := sourceAnnotations[AnnVolumeShareToNS]

	// Ensure the source PVC has been explicitly shared with the subordinate PVC namespace
	if !h.matchNamespaceToAnnotation(subordinatePVC.Namespace, shareToAnnotation) {
		return fmt.Errorf("subordinate volume source PVC is not shared with namespace %s", subordinatePVC.Namespace)
	}

	// Ensure the subordinate volume and its source volume are the same storage class
	if subordinatePVC.Spec.StorageClassName != nil && sourcePVC.Spec.StorageClassName != nil &&
		*subordinatePVC.Spec.StorageClassName != *sourcePVC.Spec.StorageClassName {
		return fmt.Errorf("subordinate PVC %s must have the same storage class as its source PVC %s",
			subordinatePVC.Name, sourcePVC.Name)
	}

	// Validate source PVC status
	if sourcePVC.Status.Phase != v1.ClaimBound || sourcePVC.Spec.VolumeName == "" {
		return fmt.Errorf("subordinate volume source PVC is not bound")
	}

	// Get the PV to which the PVC is bound and validate its status
	sourcePV, err := h.getCachedPVByName(ctx, sourcePVC.Spec.VolumeName)
	if err != nil {
		return err
	}
	if sourcePV.Status.Phase != v1.VolumeBound || sourcePV.Spec.ClaimRef == nil ||
		sourcePV.Spec.ClaimRef.Namespace != sourcePVCNamespace || sourcePV.Spec.ClaimRef.Name != sourcePVCName {
		return fmt.Errorf("subordinate volume source PV is not bound to the expected PVC")
	}

	// Update the subordinate volume config to refer to its source volume/PV
	volConfig.ShareSourceVolume = sourcePV.Name
	return nil
}

func (h *helper) matchNamespaceToAnnotation(namespace, annotation string) bool {
	availableToNamespaces := strings.Split(annotation, ",")
	for _, availableToNamespace := range availableToNamespaces {
		if availableToNamespace == namespace || availableToNamespace == "*" {
			return true
		}
	}
	return false
}

// GetSnapshotConfigForCreate accepts the attributes of a snapshot being requested by the CSI
// provisioner and returns a SnapshotConfig structure as needed by Trident to create a new snapshot.
func (h *helper) GetSnapshotConfigForCreate(volumeName, snapshotName string) (*storage.SnapshotConfig, error) {
	// Fill in what we already know about the snapshot config.
	return &storage.SnapshotConfig{
		Version:    config.OrchestratorAPIVersion,
		Name:       snapshotName,
		VolumeName: volumeName,
	}, nil
}

// GetSnapshotConfigForImport accepts the attributes of a snapshot being imported by the CSI
// provisioner, gathers additional information from Kubernetes, and returns a SnapshotConfig
// structure as needed by Trident to import a snapshot.
func (h *helper) GetSnapshotConfigForImport(
	ctx context.Context, volumeName, snapshotContentName string,
) (*storage.SnapshotConfig, error) {
	if volumeName == "" || snapshotContentName == "" {
		return nil, fmt.Errorf("invalid volume or snapshot name supplied")
	}

	fields := LogFields{
		"volumeName":          volumeName,
		"snapshotContentName": snapshotContentName,
	}

	// Get any additional information required for import.
	var internalName string
	snapshotContent, err := h.getSnapshotContentByName(ctx, snapshotContentName)
	if err != nil || snapshotContent == nil {
		Logc(ctx).WithFields(fields).WithError(err).Trace("Failed to get snapshot content by name.")
		return nil, err
	}

	// Get the snapshot internal name from the snap content.
	internalName, err = h.getSnapshotInternalNameFromAnnotation(ctx, snapshotContent)
	if err != nil {
		Logc(ctx).WithFields(fields).WithError(err).Error("Failed to get internal name from snapshot content.")
		return nil, err
	}

	return &storage.SnapshotConfig{
		Version:      config.OrchestratorAPIVersion,
		Name:         snapshotContentName,
		VolumeName:   volumeName,
		InternalName: internalName,
	}, nil
}

// getSnapshotContentByName returns a VolumeSnapshotContent by name if it exists.
func (h *helper) getSnapshotContentByName(ctx context.Context, name string) (*vsv1.VolumeSnapshotContent, error) {
	fields := LogFields{"snapshotContentName": name}
	Logc(ctx).WithFields(fields).Trace(">>>> getSnapshotContentByName")
	defer Logc(ctx).WithFields(fields).Trace("<<<< getSnapshotContentByName")

	vsc, err := h.snapClient.SnapshotV1().VolumeSnapshotContents().Get(ctx, name, getOpts)
	if err != nil || vsc == nil {
		statusErr, ok := err.(*apierrors.StatusError)
		if ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return nil, errors.NotFoundError("snapshot content %s not found; %v", name, statusErr)
		}
		return nil, err
	}

	return vsc, nil
}

// getVolumeSnapshot returns a VolumeSnapshot if it exists.
func (h *helper) getVolumeSnapshot(
	ctx context.Context, name, namespace string,
) (*vsv1.VolumeSnapshot, error) {
	fields := LogFields{"snapshotName": name, "namespace": namespace}
	Logc(ctx).WithFields(fields).Trace(">>>> getVolumeSnapshot")
	defer Logc(ctx).WithFields(fields).Trace("<<<< getVolumeSnapshot")

	// Get the VolumeSnapshot
	snapshot, err := h.snapClient.SnapshotV1().VolumeSnapshots(namespace).Get(ctx, name, getOpts)
	if err != nil {
		statusErr, ok := err.(*apierrors.StatusError)
		if ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
			return nil, errors.NotFoundError("snapshot %s not found; %v", name, statusErr)
		}
		return nil, err
	}
	return snapshot, nil
}

// getSnapshotContentBySnapshotName returns the VolumeSnapshotContent referenced by a VolumeSnapshot
func (h *helper) getSnapshotContentFromSnapshot(
	ctx context.Context, snapshot *vsv1.VolumeSnapshot,
) (*vsv1.VolumeSnapshotContent, error) {
	fields := LogFields{"snapshotName": snapshot.Name}
	Logc(ctx).WithFields(fields).Trace(">>>> getSnapshotContentFromSnapshot")
	defer Logc(ctx).WithFields(fields).Trace("<<<< getSnapshotContentFromSnapshot")

	// Extract the VolumeSnapshotContent name from the VolumeSnapshot status
	snapshotContentName := snapshot.Status.BoundVolumeSnapshotContentName
	if snapshotContentName == nil || *snapshotContentName == "" {
		return nil, errors.NotFoundError("boundVolumeSnapshotContentName not found for snapshot %s", snapshot.Name)
	}

	// Get the VolumeSnapshotContent
	vsc, err := h.getSnapshotContentByName(ctx, *snapshotContentName)
	if err != nil {
		return nil, err
	}

	return vsc, nil
}

// getSnapshotInternalNameFromAnnotation gets the snapshotInternalName from an annotation on a VolumeSnapshotContent
// for snapshot import.
func (h *helper) getSnapshotInternalNameFromAnnotation(
	ctx context.Context, vsc *vsv1.VolumeSnapshotContent,
) (string, error) {
	if vsc == nil {
		return "", fmt.Errorf("cannot get snapshot internal name")
	}

	fields := LogFields{"snapshotContentName": vsc.Name}
	Logc(ctx).WithFields(fields).Trace(">>>> getSnapshotInternalNameFromAnnotation")
	defer Logc(ctx).WithFields(fields).Trace("<<<< getSnapshotInternalNameFromAnnotation")

	// If the internal name annotation is not present, fail.
	internalName, ok := vsc.Annotations[AnnInternalSnapshotName]
	if !ok || internalName == "" {
		return "", errors.NotFoundError(
			fmt.Sprintf("internal snapshot name not present on snapshot content %s", vsc.Name),
		)
	}

	fields["internalName"] = internalName
	Logc(ctx).WithFields(fields).Debug("Discovered snapshot internal name.")
	return internalName, nil
}

func (h *helper) GetGroupSnapshotConfigForCreate(
	ctx context.Context, groupSnapshotName string, volumeNames []string,
) (*storage.GroupSnapshotConfig, error) {
	// Fill in what we already know about the snapshot config.
	return &storage.GroupSnapshotConfig{
		Version:     config.OrchestratorAPIVersion,
		Name:        groupSnapshotName,
		VolumeNames: volumeNames,
	}, nil
}

// updateVolumeConfigWithSecureSMBAccessControl update the volume config with the secure SMB access control from the
// PVC annotations and the storage class annotations
func (h *helper) updateVolumeConfigWithSecureSMBAccessControl(ctx context.Context, volumeConfig *storage.VolumeConfig,
	sc *k8sstoragev1.StorageClass, pvcAnnotations, scAnnotations, secret map[string]string,
) error {
	if sc.Parameters[CSIParameterNodeStageSecretName] == "" || sc.Parameters[CSIParameterNodeStageSecretNamespace] == "" {
		return fmt.Errorf("missing required parameters %s and %s for secure SMB access control",
			CSIParameterNodeStageSecretName, CSIParameterNodeStageSecretNamespace)
	}

	// Get the smb share Access Control from PVC annotations
	smbSharePVCAccessControlAnn := getAnnotation(pvcAnnotations, AnnSMBShareAccessControl)
	smbShareACL, err := getSMBShareAccessControlFromPVCAnnotation(smbSharePVCAccessControlAnn)
	if err != nil {
		return fmt.Errorf("failed to parse smb share access control from annotation: %v", err)
	}

	adUserFromSC := getAnnotation(scAnnotations, AnnSMBShareAdUser)
	adUserPermissionFromSC := getAnnotation(scAnnotations, AnnSMBShareAdUserPermission)

	// Set SecureSMBEnabled to true if adUserFromSC is present. This ensures Secure SMB is enabled only when required.
	if adUserFromSC != "" {
		volumeConfig.SecureSMBEnabled = true
	}

	// Check if the adUserPermissionAnn is set,	if not set it to the full control
	if adUserPermissionFromSC == "" {
		adUserPermissionFromSC = SMBShareFullControlPermission
	}

	// Check if the adUserPermissionAnn is valid
	if !isValidAccessControlPermission(adUserPermissionFromSC) {
		return fmt.Errorf("invalid adUserPermission: %s. Valid adUserPermissions are %s, %s, %s, %s",
			adUserPermissionFromSC, SMBShareFullControlPermission, SMBShareReadPermission, SMBShareChangePermission, SMBShareNoPermission)
	}

	// Check if adUser already exists in the smbShareACL, if not add it
	if _, exists := smbShareACL[adUserFromSC]; !exists {
		smbShareACL[adUserFromSC] = adUserPermissionFromSC
	}

	volumeConfig.SMBShareACL = smbShareACL

	return nil
}

// RecordVolumeEvent accepts the name of a CSI volume (i.e. a PV name), finds the associated
// PVC, and posts and event message on the PVC object with the K8S API server.
func (h *helper) RecordVolumeEvent(ctx context.Context, name, eventType, reason, message string) {
	Logc(ctx).WithFields(LogFields{
		"name":      name,
		"eventType": eventType,
		"reason":    reason,
		"message":   message,
	}).Debug("Volume event.")

	if pvc, err := h.getPVCForCSIVolume(ctx, name); err != nil {
		Logc(ctx).WithError(err).Debug("Failed to find PVC for event.")
	} else {
		h.eventRecorder.Event(pvc, mapEventType(eventType), reason, message)
	}
}

// RecordNodeEvent accepts the name of a CSI volume (i.e. a PV name), finds the associated
// PVC, and posts and event message on the PVC object with the K8S API server.
func (h *helper) RecordNodeEvent(ctx context.Context, name, eventType, reason, message string) {
	Logc(ctx).WithFields(LogFields{
		"name":      name,
		"eventType": eventType,
		"reason":    reason,
		"message":   message,
	}).Debug("Node event.")

	if node, err := h.GetNode(ctx, name); err != nil {
		Logc(ctx).WithError(err).Debug("Failed to find Node for event.")
	} else {
		h.eventRecorder.Event(node, mapEventType(eventType), reason, message)
	}
}

// IsValidResourceName determines if a string meets the Kubernetes requirements for object names.
func (h *helper) IsValidResourceName(name string) bool {
	if len(name) > maxResourceNameLength {
		return false
	}
	return network.DNS1123DomainRegex.MatchString(name)
}

// mapEventType maps between K8S API event types and Trident CSI helper event types.  The
// two sets of types may be identical, but the CSI helper interface should not be tightly
// coupled to Kubernetes.
func mapEventType(eventType string) string {
	switch eventType {
	case controllerhelpers.EventTypeNormal:
		return v1.EventTypeNormal
	case controllerhelpers.EventTypeWarning:
		return v1.EventTypeWarning
	default:
		return v1.EventTypeWarning
	}
}

// getVolumeConfig accepts the attributes of a new volume and assembles them into a
// VolumeConfig structure as needed by Trident to create a new volume.
func getVolumeConfig(
	ctx context.Context, pvc *v1.PersistentVolumeClaim, name string, size resource.Quantity,
	annotations map[string]string, storageClass *k8sstoragev1.StorageClass,
	requisiteTopology, preferredTopology []map[string]string,
) *storage.VolumeConfig {
	var (
		accessModes []config.AccessMode
		noRename    bool
	)
	smbShareACL := make(map[string]string)

	for _, pvcAccessMode := range pvc.Spec.AccessModes {
		accessModes = append(accessModes, config.AccessMode(pvcAccessMode))
	}

	accessMode := frontendcommon.CombineAccessModes(ctx, accessModes)

	volumeMode := pvc.Spec.VolumeMode
	if volumeMode == nil {
		volumeModeVal := v1.PersistentVolumeFilesystem
		volumeMode = &volumeModeVal
	}

	// If snapshotDir annotation is provided, ensure it is lower case
	snapshotDirAnn := getAnnotation(annotations, AnnSnapshotDir)
	if snapshotDirAnn != "" {
		snapDirFormatted, err := convert.ToFormattedBool(snapshotDirAnn)
		if err != nil {
			Logc(ctx).WithError(err).Errorf("Invalid boolean value for snapshotDir annotation: %v.", snapshotDirAnn)
		}
		snapshotDirAnn = snapDirFormatted
	}

	if getAnnotation(annotations, AnnReadOnlyClone) == "" {
		annotations[AnnReadOnlyClone] = "false"
	}

	readOnlyClone, err := strconv.ParseBool(getAnnotation(annotations, AnnReadOnlyClone))
	if err != nil {
		Logc(ctx).WithError(err).Warning("Unable to parse readOnlyClone annotation into bool.")
	}

	if getAnnotation(annotations, AnnNotManaged) == "" {
		annotations[AnnNotManaged] = "false"
	}
	notManaged, err := strconv.ParseBool(getAnnotation(annotations, AnnNotManaged))
	if err != nil {
		Logc(ctx).WithError(err).Warning("Unable to parse notManaged annotation into bool.")
	}

	importOriginalName := getAnnotation(annotations, AnnImportOriginalName)

	// parse importNoRename annotation only for import workflow, if not present, default to false
	if importOriginalName != "" {
		importNoRename := getAnnotation(annotations, AnnImportNoRename)
		if importNoRename != "" {
			noRename, err = strconv.ParseBool(importNoRename)
			if err != nil {
				Logc(ctx).WithError(err).Errorf("Invalid value '%s' for %s annotation; must be a valid boolean, defaulting to false.",
					importNoRename, AnnImportNoRename)
			}
		}
	}

	luksEncryption := ""
	if importOriginalName != "" {
		luksEncryption = getAnnotation(annotations, AnnLUKSEncryption)
		if luksEncryption != "" {
			if _, err = strconv.ParseBool(luksEncryption); err != nil {
				Logc(ctx).WithError(err).Warning("Unable to parse luks annotation into bool.")
			}
		}
	}

	return &storage.VolumeConfig{
		Name:                      name,
		Size:                      fmt.Sprintf("%d", size.Value()),
		Protocol:                  config.Protocol(getAnnotation(annotations, AnnProtocol)),
		SnapshotPolicy:            getAnnotation(annotations, AnnSnapshotPolicy),
		SnapshotReserve:           getAnnotation(annotations, AnnSnapshotReserve),
		SnapshotDir:               snapshotDirAnn,
		ExportPolicy:              getAnnotation(annotations, AnnExportPolicy),
		UnixPermissions:           getAnnotation(annotations, AnnUnixPermissions),
		StorageClass:              storageClass.Name,
		BlockSize:                 getAnnotation(annotations, AnnBlockSize),
		FileSystem:                getAnnotation(annotations, AnnFileSystem),
		LUKSEncryption:            luksEncryption,
		SplitOnClone:              getAnnotation(annotations, AnnSplitOnClone),
		ReadOnlyClone:             readOnlyClone,
		VolumeMode:                config.VolumeMode(*volumeMode),
		AccessMode:                accessMode,
		ImportOriginalName:        importOriginalName,
		ImportBackendUUID:         getAnnotation(annotations, AnnImportBackendUUID),
		ImportNotManaged:          notManaged,
		ImportNoRename:            noRename,
		MountOptions:              strings.Join(storageClass.MountOptions, ","),
		RequisiteTopologies:       requisiteTopology,
		PreferredTopologies:       preferredTopology,
		Namespace:                 pvc.Namespace,
		RequestName:               pvc.Name,
		SkipRecoveryQueue:         getAnnotation(annotations, AnnSkipRecoveryQueue),
		SMBShareACL:               smbShareACL,
		TieringPolicy:             getAnnotation(annotations, AnnTieringPolicy),
		TieringMinimumCoolingDays: getAnnotation(annotations, AnnTieringMinimumCoolingDays),
	}
}

// getAnnotation returns an annotation from a map, or an empty string if not found.
func getAnnotation(annotations map[string]string, key string) string {
	if val, ok := annotations[key]; ok {
		return val
	}
	return ""
}

// getCaseFoldedAnnotation returns an annotation from a map, or an empty string if not found. Ignores annotation case, so luksEncryption == LUKSEncryption.
func getCaseFoldedAnnotation(annotations map[string]string, key string) string {
	for k, v := range annotations {
		if strings.EqualFold(k, key) {
			return v
		}
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

// processSCAnnotations updates the annotations map with SC annotations.
func processSCAnnotations(sc *k8sstoragev1.StorageClass) map[string]string {
	annotations := sc.Annotations
	if sc.Annotations == nil {
		annotations = make(map[string]string)
	}

	return annotations
}

// getSMBShareAccessControlFromPVCAnnotation parses the smbShareAccessControl annotation and updates the smbShareACL map
func getSMBShareAccessControlFromPVCAnnotation(smbShareAccessControlAnn string) (map[string]string, error) {
	// Structure to hold the parsed smbShareAccessControlAnnotation
	parsedData := make(map[string][]string)

	// Parse the smbShareAccessControl annotation
	if err := yaml.Unmarshal([]byte(smbShareAccessControlAnn), &parsedData); err != nil {
		return nil, fmt.Errorf("failed to parse smbShareAccessControl annotation: %v", err)
	}

	// Convert the parsed data into the smbShareACL map
	smbShareACL := make(map[string]string)

	// Define a priority map for access control permissions,
	// to determine the permission for user who is associated with multiple permissions
	permissionPriority := map[string]int{
		SMBShareFullControlPermission: 4, // Highest priority
		SMBShareChangePermission:      3,
		SMBShareReadPermission:        2,
		SMBShareNoPermission:          1, // Lowest priority
	}

	for accessControlPermission, users := range parsedData {
		if !isValidAccessControlPermission(accessControlPermission) {
			return nil, fmt.Errorf("invalid access control permission %s in smbShareAccessControl annotation, "+
				"valid access control permissions are %s, %s,%s,%s ", accessControlPermission, SMBShareFullControlPermission,
				SMBShareReadPermission, SMBShareChangePermission, SMBShareNoPermission)
		} else {
			for _, user := range users {
				// Check if the user already exists in the smbShareACL map
				if existingPermission, exists := smbShareACL[user]; exists {
					// Compare the priority of the existing permission with the new one
					if permissionPriority[accessControlPermission] > permissionPriority[existingPermission] {
						smbShareACL[user] = accessControlPermission
					}
				} else {
					// If the user doesn't exist, add it to the map
					smbShareACL[user] = accessControlPermission
				}
			}
		}
	}
	return smbShareACL, nil
}

// isValidAccessControlPermission checks if the provided permission is valid
func isValidAccessControlPermission(permission string) bool {
	switch permission {
	case SMBShareFullControlPermission, SMBShareReadPermission, SMBShareChangePermission, SMBShareNoPermission:
		return true
	default:
		return false
	}
}
