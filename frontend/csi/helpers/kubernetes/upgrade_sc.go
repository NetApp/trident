package kubernetes

import (
	log "github.com/sirupsen/logrus"
	k8sstoragev1 "k8s.io/api/storage/v1"
	k8sstoragev1beta "k8s.io/api/storage/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/netapp/trident/frontend/csi"
)

/////////////////////////////////////////////////////////////////////////////
//
// This file contains the code to replace legacy storage classes.
//
/////////////////////////////////////////////////////////////////////////////

// addLegacyStorageClass is the add handler for the legacy storage class watcher.
func (p *Plugin) addLegacyStorageClass(obj interface{}) {
	switch sc := obj.(type) {
	case *k8sstoragev1beta.StorageClass:
		p.processLegacyStorageClass(convertStorageClassV1BetaToV1(sc), eventAdd)
	case *k8sstoragev1.StorageClass:
		p.processLegacyStorageClass(sc, eventAdd)
	default:
		log.Errorf("K8S helper expected storage.k8s.io/v1beta1 or storage.k8s.io/v1 storage class; got %v", obj)
	}
}

// updateLegacyStorageClass is the update handler for the legacy storage class watcher.
func (p *Plugin) updateLegacyStorageClass(oldObj, newObj interface{}) {
	switch sc := newObj.(type) {
	case *k8sstoragev1beta.StorageClass:
		p.processLegacyStorageClass(convertStorageClassV1BetaToV1(sc), eventUpdate)
	case *k8sstoragev1.StorageClass:
		p.processLegacyStorageClass(sc, eventUpdate)
	default:
		log.Errorf("K8S helper expected storage.k8s.io/v1beta1 or storage.k8s.io/v1 storage class; got %v", newObj)
	}
}

// deleteStorageClass is the delete handler for the storage class watcher.
func (p *Plugin) deleteLegacyStorageClass(obj interface{}) {
	switch sc := obj.(type) {
	case *k8sstoragev1beta.StorageClass:
		p.processLegacyStorageClass(convertStorageClassV1BetaToV1(sc), eventDelete)
	case *k8sstoragev1.StorageClass:
		p.processLegacyStorageClass(sc, eventDelete)
	default:
		log.Errorf("K8S helper expected storage.k8s.io/v1beta1 or storage.k8s.io/v1 storage class; got %v", obj)
	}
}

// processLegacyStorageClass logs and handles add/update/delete events for legacy Trident storage classes.
// Add & Delete events cause the storage class to be replaced with an identical one that references the
// CSI Trident provisioner name.
func (p *Plugin) processLegacyStorageClass(sc *k8sstoragev1.StorageClass, eventType string) {

	// Validate the storage class
	if sc.Provisioner != csi.LegacyProvisioner {
		return
	}

	logFields := log.Fields{
		"name":        sc.Name,
		"provisioner": sc.Provisioner,
		"parameters":  sc.Parameters,
	}

	switch eventType {
	case eventAdd:
		log.WithFields(logFields).Debug("Legacy storage class added to cache.")
		p.replaceLegacyStorageClass(sc)
	case eventUpdate:
		log.WithFields(logFields).Debug("Legacy storage class updated in cache.")
		p.replaceLegacyStorageClass(sc)
	case eventDelete:
		log.WithFields(logFields).Debug("Legacy storage class deleted from cache.")
	}
}

// replaceLegacyStorageClass replaces a storage class with the legacy Trident provisioner name (netapp.io/trident)
// with an identical storage class with the CSI Trident provisioner name (csi.trident.netapp.io).
func (p *Plugin) replaceLegacyStorageClass(oldSC *k8sstoragev1.StorageClass) {

	// Clone the storage class
	newSC := oldSC.DeepCopy()
	newSC.Provisioner = csi.Provisioner
	newSC.ResourceVersion = ""
	newSC.UID = ""

	// Delete the old storage class
	if err := p.kubeClient.StorageV1().StorageClasses().Delete(oldSC.Name, &metav1.DeleteOptions{}); err != nil {
		log.WithFields(log.Fields{
			"name":  oldSC.Name,
			"error": err,
		}).Error("Could not delete legacy storage class.")
		return
	}

	// Create the new storage class
	if _, err := p.kubeClient.StorageV1().StorageClasses().Create(newSC); err != nil {

		log.WithFields(log.Fields{
			"name":  newSC.Name,
			"error": err,
		}).Error("Could not replace storage class, attempting to restore old one.")

		// Failed to create the new storage class, so try to restore the old one
		if _, err := p.kubeClient.StorageV1().StorageClasses().Create(oldSC); err != nil {

			log.WithFields(log.Fields{
				"name":  oldSC.Name,
				"error": err,
			}).Error("Could not restore storage class, please recreate it manually.")
		}

		return
	}

	log.WithFields(log.Fields{
		"name":           newSC.Name,
		"oldProvisioner": oldSC.Provisioner,
		"newProvisioner": newSC.Provisioner,
	}).Info("Replaced storage class so it works with CSI Trident.")
}
