// Copyright 2020 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/netapp/trident/config"
	frontendcommon "github.com/netapp/trident/frontend/common"
	k8sclient "github.com/netapp/trident/k8s_client"
	. "github.com/netapp/trident/logger"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
)

// canPVMatchWithPVC verifies that the volumeSize and volumeAccessModes
// are capable of fulfilling the corresponding claimSize and claimAccessModes.
// For this to be true, volumeSize >= claimSize and every access mode in
// claimAccessModes must be present in volumeAccessModes.
// Note that this allows volumes to exceed the attributes requested by the
// claim; this is acceptable, though undesirable, and helps us avoid racing
// with the binder.
func canPVMatchWithPVC(ctx context.Context, pv *v1.PersistentVolume, claim *v1.PersistentVolumeClaim) bool {

	claimSize, _ := claim.Spec.Resources.Requests[v1.ResourceStorage]
	claimAccessModes := claim.Spec.AccessModes
	volumeAccessModes := pv.Spec.AccessModes
	volumeSize, ok := pv.Spec.Capacity[v1.ResourceStorage]
	if !ok {
		Logc(ctx).WithFields(log.Fields{
			"PV":  pv.Name,
			"PVC": claim.Name,
		}).Error("Kubernetes frontend detected a corrupted PV with no size!")
		return false
	}
	// Do the easy check first.  These should be whole numbers, so value
	// *should* be safe.
	if volumeSize.Value() < claimSize.Value() {
		return false
	}
	for _, claimMode := range claimAccessModes {
		found := false
		for _, volumeMode := range volumeAccessModes {
			if claimMode == volumeMode {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func getAnnotation(annotations map[string]string, key string) string {
	if val, ok := annotations[key]; ok {
		return val
	}
	return ""
}

// getVolumeConfig generates a NetApp DVP volume config from the specs pulled
// from the PVC.
func getVolumeConfig(
	pvcAccessModes []v1.PersistentVolumeAccessMode,
	volumeMode *v1.PersistentVolumeMode,
	name string,
	size resource.Quantity,
	annotations map[string]string,
) *storage.VolumeConfig {
	var accessModes []config.AccessMode

	if volumeMode == nil {
		volumeModeVal := v1.PersistentVolumeFilesystem
		volumeMode = &volumeModeVal
	}

	for _, pvcAccessMode := range pvcAccessModes {
		accessModes = append(accessModes, config.AccessMode(pvcAccessMode))
	}

	accessMode := frontendcommon.CombineAccessModes(accessModes)

	if getAnnotation(annotations, AnnFileSystem) == "" {
		annotations[AnnFileSystem] = "ext4"
	}

	return &storage.VolumeConfig{
		Name:              name,
		Size:              fmt.Sprintf("%d", size.Value()),
		Protocol:          config.Protocol(getAnnotation(annotations, AnnProtocol)),
		SnapshotPolicy:    getAnnotation(annotations, AnnSnapshotPolicy),
		SnapshotReserve:   getAnnotation(annotations, AnnSnapshotReserve),
		SnapshotDir:       getAnnotation(annotations, AnnSnapshotDir),
		ExportPolicy:      getAnnotation(annotations, AnnExportPolicy),
		UnixPermissions:   getAnnotation(annotations, AnnUnixPermissions),
		StorageClass:      getAnnotation(annotations, AnnClass),
		BlockSize:         getAnnotation(annotations, AnnBlockSize),
		FileSystem:        getAnnotation(annotations, AnnFileSystem),
		CloneSourceVolume: getAnnotation(annotations, AnnCloneFromPVC),
		SplitOnClone:      getAnnotation(annotations, AnnSplitOnClone),
		VolumeMode:        config.VolumeMode(*volumeMode),
		AccessMode:        accessMode,
	}
}

func CreateNFSVolumeSource(vol *storage.VolumeExternal) *v1.NFSVolumeSource {
	volConfig := vol.Config
	return &v1.NFSVolumeSource{
		Server: volConfig.AccessInfo.NfsServerIP,
		Path:   volConfig.AccessInfo.NfsPath,
	}
}

func findOrCreateCHAPSecret(
	ctx context.Context, k8sClient k8sclient.Interface, kubeVersion *utils.Version, vol *storage.VolumeExternal,
) (string, error) {

	volConfig := vol.Config
	secretName := vol.GetCHAPSecretName()
	Logc(ctx).Debugf("Using secret: %v", secretName)

	if !kubeVersion.AtLeast(utils.MustParseSemantic("v1.7.0")) {
		versionErr := fmt.Errorf("cannot use CHAP with Kubernetes version < v1.7.0")
		return secretName, versionErr
	}

	secretExists, _ := k8sClient.CheckSecretExists(secretName)
	if !secretExists {
		Logc(ctx).Infof("Creating secret: %v", secretName)
		_, creationErr := k8sClient.CreateCHAPSecret(
			secretName,
			volConfig.AccessInfo.IscsiUsername,
			volConfig.AccessInfo.IscsiInitiatorSecret,
			volConfig.AccessInfo.IscsiTargetSecret)
		if creationErr != nil {
			return secretName, creationErr
		} else {
			Logc(ctx).Infof("Created secret: %v", secretName)
		}
	}
	return secretName, nil
}

func CreateISCSIPersistentVolumeSource(
	ctx context.Context, k8sClient k8sclient.Interface, kubeVersion *utils.Version, vol *storage.VolumeExternal,
) (*v1.ISCSIPersistentVolumeSource, error) {

	namespace := ""
	switch {
	case kubeVersion.AtLeast(utils.MustParseSemantic("v1.9.0")):
		namespace = k8sClient.Namespace()
	}
	volConfig := vol.Config
	var portals []string
	switch {
	case kubeVersion.AtLeast(utils.MustParseSemantic("v1.6.0")):
		portals = volConfig.AccessInfo.IscsiPortals
	}
	if volConfig.AccessInfo.IscsiTargetSecret != "" {
		// CHAP logic
		secretName, chapError := findOrCreateCHAPSecret(ctx, k8sClient, kubeVersion, vol)
		if chapError != nil {
			Logc(ctx).Errorf("Could not create secret: %v error: %v", secretName, chapError.Error())
			return nil, chapError
		}

		return &v1.ISCSIPersistentVolumeSource{
			TargetPortal:      volConfig.AccessInfo.IscsiTargetPortal,
			Portals:           portals,
			IQN:               volConfig.AccessInfo.IscsiTargetIQN,
			Lun:               volConfig.AccessInfo.IscsiLunNumber,
			ISCSIInterface:    volConfig.AccessInfo.IscsiInterface,
			FSType:            volConfig.FileSystem,
			DiscoveryCHAPAuth: true,
			SessionCHAPAuth:   true,
			SecretRef:         &v1.SecretReference{Name: secretName, Namespace: namespace},
		}, nil
	} else {
		// non-CHAP logic
		return &v1.ISCSIPersistentVolumeSource{
			TargetPortal:   volConfig.AccessInfo.IscsiTargetPortal,
			Portals:        portals,
			IQN:            volConfig.AccessInfo.IscsiTargetIQN,
			Lun:            volConfig.AccessInfo.IscsiLunNumber,
			ISCSIInterface: volConfig.AccessInfo.IscsiInterface,
			FSType:         volConfig.FileSystem,
		}, nil
	}
}
