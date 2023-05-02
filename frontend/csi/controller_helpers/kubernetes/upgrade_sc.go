// Copyright 2022 NetApp, Inc. All Rights Reserved.

package kubernetes

import (
	"context"

	k8sstoragev1 "k8s.io/api/storage/v1"
	k8sstoragev1beta "k8s.io/api/storage/v1beta1"

	"github.com/netapp/trident/frontend/csi"
	. "github.com/netapp/trident/logging"
)

/////////////////////////////////////////////////////////////////////////////
//
// This file contains the code to replace legacy storage classes.
//
/////////////////////////////////////////////////////////////////////////////

// addLegacyStorageClass is the add handler for the legacy storage class watcher.
func (h *helper) addLegacyStorageClass(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S, WorkflowStorageClassCreate, LogLayerCSIFrontend)

	switch sc := obj.(type) {
	case *k8sstoragev1beta.StorageClass:
		h.processLegacyStorageClass(ctx, convertStorageClassV1BetaToV1(sc), eventAdd)
	case *k8sstoragev1.StorageClass:
		h.processLegacyStorageClass(ctx, sc, eventAdd)
	default:
		Logc(ctx).Errorf("K8S helper expected storage.k8s.io/v1beta1 or storage.k8s.io/v1 storage class; got %v", obj)
	}
}

// updateLegacyStorageClass is the update handler for the legacy storage class watcher.
func (h *helper) updateLegacyStorageClass(_, newObj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S, WorkflowStorageClassUpdate, LogLayerCSIFrontend)

	switch sc := newObj.(type) {
	case *k8sstoragev1beta.StorageClass:
		h.processLegacyStorageClass(ctx, convertStorageClassV1BetaToV1(sc), eventUpdate)
	case *k8sstoragev1.StorageClass:
		h.processLegacyStorageClass(ctx, sc, eventUpdate)
	default:
		Logc(ctx).Errorf("K8S helper expected storage.k8s.io/v1beta1 or storage.k8s.io/v1 storage class; got %v", newObj)
	}
}

// deleteStorageClass is the delete handler for the storage class watcher.
func (h *helper) deleteLegacyStorageClass(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceK8S, WorkflowStorageClassDelete, LogLayerCSIFrontend)

	switch sc := obj.(type) {
	case *k8sstoragev1beta.StorageClass:
		h.processLegacyStorageClass(ctx, convertStorageClassV1BetaToV1(sc), eventDelete)
	case *k8sstoragev1.StorageClass:
		h.processLegacyStorageClass(ctx, sc, eventDelete)
	default:
		Logc(ctx).Errorf("K8S helper expected storage.k8s.io/v1beta1 or storage.k8s.io/v1 storage class; got %v", obj)
	}
}

// processLegacyStorageClass logs and handles add/update/delete events for legacy Trident storage classes.
// Add & Delete events cause the storage class to be replaced with an identical one that references the
// CSI Trident provisioner name.
func (h *helper) processLegacyStorageClass(ctx context.Context, sc *k8sstoragev1.StorageClass, eventType string) {
	// Validate the storage class
	if sc.Provisioner != csi.LegacyProvisioner {
		return
	}

	logFields := LogFields{
		"name":        sc.Name,
		"provisioner": sc.Provisioner,
		"parameters":  sc.Parameters,
	}

	switch eventType {
	case eventAdd:
		Logc(ctx).WithFields(logFields).Trace("Legacy storage class added to cache.")
		h.replaceLegacyStorageClass(ctx, sc)
	case eventUpdate:
		Logc(ctx).WithFields(logFields).Trace("Legacy storage class updated in cache.")
		h.replaceLegacyStorageClass(ctx, sc)
	case eventDelete:
		Logc(ctx).WithFields(logFields).Trace("Legacy storage class deleted from cache.")
	}
}

// replaceLegacyStorageClass replaces a storage class with the legacy Trident provisioner name (netapp.io/trident)
// with an identical storage class with the CSI Trident provisioner name (csi.trident.netapp.io).
func (h *helper) replaceLegacyStorageClass(ctx context.Context, oldSC *k8sstoragev1.StorageClass) {
	// Clone the storage class
	newSC := oldSC.DeepCopy()
	newSC.Provisioner = csi.Provisioner
	newSC.ResourceVersion = ""
	newSC.UID = ""

	// Delete the old storage class
	if err := h.kubeClient.StorageV1().StorageClasses().Delete(ctx, oldSC.Name, deleteOpts); err != nil {
		Logc(ctx).WithFields(LogFields{
			"name":  oldSC.Name,
			"error": err,
		}).Error("Could not delete legacy storage class.")
		return
	}

	// Create the new storage class
	if _, err := h.kubeClient.StorageV1().StorageClasses().Create(ctx, newSC, createOpts); err != nil {

		Logc(ctx).WithFields(LogFields{
			"name":  newSC.Name,
			"error": err,
		}).Error("Could not replace storage class, attempting to restore old one.")

		// Failed to create the new storage class, so try to restore the old one
		if _, err := h.kubeClient.StorageV1().StorageClasses().Create(ctx, oldSC, createOpts); err != nil {
			Logc(ctx).WithFields(LogFields{
				"name":  oldSC.Name,
				"error": err,
			}).Error("Could not restore storage class, please recreate it manually.")
		}

		return
	}

	Logc(ctx).WithFields(LogFields{
		"name":           newSC.Name,
		"oldProvisioner": oldSC.Provisioner,
		"newProvisioner": newSC.Provisioner,
	}).Info("Replaced storage class so it works with CSI Trident.")
}
