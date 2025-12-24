// Copyright 2025 NetApp, Inc. All Rights Reserved.

package resourcemonitor

import (
	"fmt"
	"time"

	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/operator/controllers/configurator"
	netappv1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
)

const (
	// Annotations used for StorageClass to TridentConfigurator mapping
	TridentConfiguratorNameAnnotation    = "trident.netapp.io/configuratorName"
	TridentConfiguratorStatusAnnotation  = "trident.netapp.io/configuratorStatus"
	TridentConfiguratorMessageAnnotation = "trident.netapp.io/configuratorMessage"
	AdditionalStoragePoolsAnnotation     = "trident.netapp.io/additionalStoragePools"
	TridentManagedAnnotation             = "trident.netapp.io/managed"

	// Common constants
	TridentCSIProvisioner  = "csi.trident.netapp.io"
	StorageDriverNameParam = "storageDriverName"
	CredentialsNameParam   = "credentialsName"

	// Status values
	StatusSuccess = "Success"
	StatusFailed  = "Failed"
	StatusPending = "Pending"

	// Labels
	ScNameLabel    = "trident.netapp.io/scName"
	CreatedByLabel = "trident.netapp.io/createdBy"

	// Cache refresh period for storage class
	cacheRefreshPeriod = 30 * time.Minute
)

// StorageClassHandler handles StorageClass resources
type StorageClassHandler struct {
	controller *Controller

	storageClassIndexer  cache.Indexer
	storageClassInformer cache.SharedIndexInformer
	storageClassWatcher  cache.ListerWatcher

	tconfIndexer  cache.Indexer
	tconfInformer cache.SharedIndexInformer
	tconfWatcher  cache.ListerWatcher

	driverHandler StorageDriverHandler
}

// NewStorageClassHandler creates a new StorageClass handler
func NewStorageClassHandler(controller *Controller) (*StorageClassHandler, error) {
	h := &StorageClassHandler{
		controller:    controller,
		driverHandler: NewFsxStorageDriverHandler(),
	}

	// Set up a watch for StorageClass
	h.storageClassWatcher = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			// List all StorageClasses - we'll filter by provisioner in the handler
			// Field selectors don't support 'provisioner', only metadata.name and metadata.namespace
			return controller.Clients.KubeClient.StorageV1().StorageClasses().List(ctx(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			// Watch all StorageClasses - we'll filter by provisioner in the handler
			return controller.Clients.KubeClient.StorageV1().StorageClasses().Watch(ctx(), options)
		},
	}

	// Set up the StorageClass indexing controller
	h.storageClassInformer = cache.NewSharedIndexInformer(
		h.storageClassWatcher,
		&storagev1.StorageClass{},
		cacheRefreshPeriod,
		cache.Indexers{},
	)
	h.storageClassIndexer = h.storageClassInformer.GetIndexer()

	// Add event handlers
	_, _ = h.storageClassInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    h.addStorageClass,
			UpdateFunc: h.updateStorageClass,
			DeleteFunc: h.deleteStorageClass,
		},
	)

	// Set up a watch for TridentConfigurator
	h.tconfWatcher = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return controller.Clients.CRDClient.TridentV1().TridentConfigurators().List(ctx(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return controller.Clients.CRDClient.TridentV1().TridentConfigurators().Watch(ctx(), options)
		},
	}

	// Set up the TridentConfigurator indexing controller
	h.tconfInformer = cache.NewSharedIndexInformer(
		h.tconfWatcher,
		&netappv1.TridentConfigurator{},
		0,
		cache.Indexers{},
	)
	h.tconfIndexer = h.tconfInformer.GetIndexer()

	// Add event handlers for TridentConfigurator
	_, _ = h.tconfInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			UpdateFunc: h.updateTridentConfigurator,
		},
	)

	return h, nil
}

// Start starts the StorageClass watcher
func (h *StorageClassHandler) Start(stopChan chan struct{}) error {
	go h.storageClassInformer.Run(stopChan)
	go h.tconfInformer.Run(stopChan)

	// Wait for cache sync
	if !cache.WaitForCacheSync(stopChan, h.storageClassInformer.HasSynced, h.tconfInformer.HasSynced) {
		return fmt.Errorf("failed to sync caches")
	}

	Log().Info("StorageClass handler started and caches synced")
	return nil
}

// GetResourceType returns the resource type
func (h *StorageClassHandler) GetResourceType() ResourceType {
	return ResourceTypeStorageClass
}

// addStorageClass is the add handler for StorageClass
func (h *StorageClassHandler) addStorageClass(obj interface{}) {
	sc, ok := obj.(*storagev1.StorageClass)
	if !ok {
		Log().Errorf("expected StorageClass but got %T", obj)
		return
	}

	// Check if this StorageClass should be managed by Trident Resource Monitor
	if !h.shouldManageStorageClass(sc) {
		Log().WithField("storageClass", sc.Name).Debug("StorageClass does not qualify for resource monitoring")
		return
	}

	Log().WithField("storageClass", sc.Name).Info("StorageClass added")

	workItem := &WorkItem{
		ResourceType: ResourceTypeStorageClass,
		Key:          sc.Name,
		Operation:    OperationAdd,
		Object:       sc,
	}

	h.controller.EnqueueWork(workItem)
}

// updateStorageClass is the update handler for StorageClass
func (h *StorageClassHandler) updateStorageClass(oldObj, newObj interface{}) {
	oldSC, ok := oldObj.(*storagev1.StorageClass)
	if !ok {
		Log().Errorf("expected StorageClass but got %T", oldObj)
		return
	}

	newSC, ok := newObj.(*storagev1.StorageClass)
	if !ok {
		Log().Errorf("expected StorageClass but got %T", newObj)
		return
	}

	// Check if this StorageClass should be managed
	if !h.shouldManageStorageClass(newSC) {
		Log().WithField("storageClass", newSC.Name).Debug("StorageClass does not qualify for resource monitoring")
		return
	}

	// Check if relevant fields have changed
	if !h.hasRelevantChanges(oldSC, newSC) {
		Log().WithField("storageClass", newSC.Name).Debug("No relevant changes detected")
		return
	}

	Log().WithField("storageClass", newSC.Name).Info("StorageClass updated")

	workItem := &WorkItem{
		ResourceType: ResourceTypeStorageClass,
		Key:          newSC.Name,
		Operation:    OperationUpdate,
		Object:       newSC,
	}

	h.controller.EnqueueWork(workItem)
}

// deleteStorageClass is the delete handler for StorageClass
func (h *StorageClassHandler) deleteStorageClass(obj interface{}) {
	sc, ok := obj.(*storagev1.StorageClass)
	if !ok {
		// Handle tombstone
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			Log().Errorf("couldn't get object from tombstone %#v", obj)
			return
		}
		sc, ok = tombstone.Obj.(*storagev1.StorageClass)
		if !ok {
			Log().Errorf("tombstone contained object that is not a StorageClass %#v", obj)
			return
		}
	}

	// Check if this StorageClass should be managed by Trident Resource Monitor
	if !h.shouldManageStorageClass(sc) {
		Log().WithField("storageClass", sc.Name).Debug("StorageClass does not qualify for resource monitoring")
		return
	}

	Log().WithField("storageClass", sc.Name).Info("StorageClass deleted")

	workItem := &WorkItem{
		ResourceType: ResourceTypeStorageClass,
		Key:          sc.Name,
		Operation:    OperationDelete,
		Object:       sc,
	}

	h.controller.EnqueueWork(workItem)
}

// updateTridentConfigurator is the update handler for TridentConfigurator
func (h *StorageClassHandler) updateTridentConfigurator(oldObj, newObj interface{}) {
	oldTconf, ok := oldObj.(*netappv1.TridentConfigurator)
	if !ok {
		Log().Errorf("expected TridentConfigurator but got %T", oldObj)
		return
	}

	newTconf, ok := newObj.(*netappv1.TridentConfigurator)
	if !ok {
		Log().Errorf("expected TridentConfigurator but got %T", newObj)
		return
	}

	// Check if status changed
	if oldTconf.Status.LastOperationStatus == newTconf.Status.LastOperationStatus &&
		oldTconf.Status.Message == newTconf.Status.Message {
		return
	}

	Log().WithFields(LogFields{
		"tridentConfigurator": newTconf.Name,
		"status":              newTconf.Status.LastOperationStatus,
	}).Info("TridentConfigurator status changed")

	// Find the associated StorageClass
	scName, ok := newTconf.Labels[ScNameLabel]
	if !ok {
		Log().WithField("tridentConfigurator", newTconf.Name).Debugf("TridentConfigurator has no %s label", ScNameLabel)
		return
	}

	// Get the StorageClass
	sc, err := h.controller.Clients.KubeClient.StorageV1().StorageClasses().Get(ctx(), scName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			Log().WithField("storageClass", scName).Debug("StorageClass not found for TridentConfigurator update")
			return
		}
		Log().WithField("storageClass", scName).Errorf("Failed to get StorageClass: %v", err)
		return
	}

	// Sync the status to the StorageClass
	if err := h.syncTridentConfiguratorStatus(sc, newTconf); err != nil {
		Log().WithField("storageClass", scName).Errorf("Failed to sync TridentConfigurator status to StorageClass: %v", err)
	}
}

// Reconcile reconciles a StorageClass
func (h *StorageClassHandler) Reconcile(workItem *WorkItem) error {
	Log().WithFields(LogFields{
		"storageClass": workItem.Key,
		"operation":    workItem.Operation,
	}).Info("Reconciling StorageClass")

	sc, ok := workItem.Object.(*storagev1.StorageClass)
	if !ok {
		return fmt.Errorf("expected StorageClass but got %T", workItem.Object)
	}

	switch workItem.Operation {
	case OperationAdd, OperationUpdate:
		return h.reconcileStorageClass(sc)
	case OperationDelete:
		return h.handleStorageClassDeletion(sc)
	default:
		return fmt.Errorf("unknown operation: %s", workItem.Operation)
	}
}

// reconcileStorageClass handles the creation/update of TridentConfigurator from StorageClass
func (h *StorageClassHandler) reconcileStorageClass(sc *storagev1.StorageClass) error {

	if err := h.updateAdditionalStoragePoolsAnnotation(sc); err != nil {
		Log().WithField("storageClass", sc.Name).Warnf("Failed to update additionalStoragePools annotation: %v", err)
	}

	// Validate that the StorageClass has required parameters
	if err := h.validateStorageClass(sc); err != nil {
		Log().WithField("storageClass", sc.Name).Errorf("StorageClass validation failed: %v", err)
		return h.updateStorageClassStatus(sc, StatusFailed, fmt.Sprintf("Validation failed: %v", err))
	}

	// Get or create TridentConfigurator name
	tconfName := h.getTridentConfiguratorName(sc)

	// Check if TridentConfigurator already exists
	existingTconf, getErr := h.controller.Clients.CRDClient.TridentV1().TridentConfigurators().Get(ctx(), tconfName, metav1.GetOptions{})
	if getErr != nil && !k8serrors.IsNotFound(getErr) {
		return fmt.Errorf("failed to get TridentConfigurator: %v", getErr)
	}

	// Build TridentConfigurator spec from StorageClass
	tconfSpec, err := h.buildTridentConfiguratorSpec(sc)
	if err != nil {
		return h.updateStorageClassStatus(sc, StatusFailed, fmt.Sprintf("Failed to build TridentConfigurator spec: %v", err))
	}

	if k8serrors.IsNotFound(getErr) {
		// Create new TridentConfigurator
		tconf := &netappv1.TridentConfigurator{
			ObjectMeta: metav1.ObjectMeta{
				Name: tconfName,
				Labels: map[string]string{
					"app":          "trident.netapp.io",
					CreatedByLabel: "resource-monitor",
					ScNameLabel:    sc.Name,
				},
				Finalizers: []string{configurator.TridentConfiguratorFinalizer},
			},
			Spec: *tconfSpec,
		}

		createdTconf, err := h.controller.Clients.CRDClient.TridentV1().TridentConfigurators().Create(ctx(), tconf, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("failed to create TridentConfigurator: %v", err)
		}

		Log().WithFields(LogFields{
			"storageClass":        sc.Name,
			"tridentConfigurator": tconfName,
		}).Info("Created TridentConfigurator from StorageClass")

		// Update StorageClass with TridentConfigurator reference and sync initial status
		if err := h.syncTridentConfiguratorStatus(sc, createdTconf); err != nil {
			Log().WithField("storageClass", sc.Name).Warnf("Failed to sync initial status: %v", err)
		}
		return nil
	}

	// Fetch the TridentConfigurator fresh to ensure we have the latest version
	existingTconf, err = h.controller.Clients.CRDClient.TridentV1().TridentConfigurators().Get(ctx(), tconfName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get TridentConfigurator for update: %v", err)
	}

	// Update existing TridentConfigurator if needed
	if h.needsTridentConfiguratorUpdate(existingTconf, tconfSpec) {
		// Immediately update StorageClass status to Pending since we're changing the config
		if err := h.updateStorageClassStatus(sc, StatusPending, "Updating backend configuration"); err != nil {
			Log().WithField("storageClass", sc.Name).Warnf("Failed to update StorageClass status to Pending: %v", err)
		}

		// Create a copy to avoid modifying the cached object
		tconfToUpdate := existingTconf.DeepCopy()
		tconfToUpdate.Spec = *tconfSpec

		_, err := h.controller.Clients.CRDClient.TridentV1().TridentConfigurators().Update(ctx(), tconfToUpdate, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update TridentConfigurator: %v", err)
		}

		Log().WithFields(LogFields{
			"storageClass":        sc.Name,
			"tridentConfigurator": tconfName,
		}).Info("Updated TridentConfigurator from StorageClass, status set to Pending")

		// Don't sync stale status here - the TridentConfigurator watcher will sync it
		// once the configurator controller processes the update
		return nil
	}

	// Monitor TridentConfigurator status and propagate back to StorageClass
	// (only reached if no update was needed)
	return h.syncTridentConfiguratorStatus(sc, existingTconf)
}

// handleStorageClassDeletion handles the cleanup when a StorageClass is deleted
func (h *StorageClassHandler) handleStorageClassDeletion(sc *storagev1.StorageClass) error {
	tconfName := h.getTridentConfiguratorName(sc)

	// Check if TridentConfigurator exists
	tconf, err := h.controller.Clients.CRDClient.TridentV1().TridentConfigurators().Get(ctx(), tconfName, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Already deleted, nothing to do
			return nil
		}
		return fmt.Errorf("failed to get TridentConfigurator: %v", err)
	}

	// Delete TridentConfigurator
	err = h.controller.Clients.CRDClient.TridentV1().TridentConfigurators().Delete(ctx(), tconfName, metav1.DeleteOptions{})
	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete TridentConfigurator: %v", err)
	}

	Log().WithFields(LogFields{
		"storageClass":        sc.Name,
		"tridentConfigurator": tconf.Name,
	}).Info("Deleted TridentConfigurator for StorageClass")

	return nil
}

// shouldManageStorageClass determines if a StorageClass should be managed by this controller
func (h *StorageClassHandler) shouldManageStorageClass(sc *storagev1.StorageClass) bool {
	// Must be provisioned by Trident
	if sc.Provisioner != TridentCSIProvisioner {
		return false
	}

	// Delegate to driver handler for driver-specific checks
	return h.driverHandler.ShouldManageStorageClass(sc)
}

// hasRelevantChanges checks if there are relevant changes between old and new StorageClass
func (h *StorageClassHandler) hasRelevantChanges(oldSC, newSC *storagev1.StorageClass) bool {
	// Delegate to driver handler for driver-specific change detection
	return h.driverHandler.HasRelevantChanges(oldSC, newSC)
}

// buildAdditionalStoragePoolsValue constructs the additionalStoragePools annotation value
func (h *StorageClassHandler) buildAdditionalStoragePoolsValue(sc *storagev1.StorageClass) (string, error) {
	// Delegate to driver handler for driver-specific storage pool building
	return h.driverHandler.BuildAdditionalStoragePoolsValue(sc)
}

// validateStorageClass validates the StorageClass configuration
func (h *StorageClassHandler) validateStorageClass(sc *storagev1.StorageClass) error {
	// Delegate to driver handler for driver-specific validation
	return h.driverHandler.ValidateStorageClass(sc)
}

// getTridentConfiguratorName generates the TridentConfigurator name from StorageClass
func (h *StorageClassHandler) getTridentConfiguratorName(sc *storagev1.StorageClass) string {
	// Use annotation if provided, otherwise generate from SC name
	if name, ok := sc.Annotations[TridentConfiguratorNameAnnotation]; ok && name != "" {
		return name
	}
	return fmt.Sprintf("tconf-%s", sc.Name)
}

// buildTridentConfiguratorSpec builds a TridentConfigurator spec from StorageClass
func (h *StorageClassHandler) buildTridentConfiguratorSpec(sc *storagev1.StorageClass) (*netappv1.TridentConfiguratorSpec, error) {
	// Delegate to driver handler for driver-specific spec building
	return h.driverHandler.BuildTridentConfiguratorSpec(sc)
}

// needsTridentConfiguratorUpdate checks if TridentConfigurator needs to be updated
func (h *StorageClassHandler) needsTridentConfiguratorUpdate(existing *netappv1.TridentConfigurator, newSpec *netappv1.TridentConfiguratorSpec) bool {
	return string(existing.Spec.Raw) != string(newSpec.Raw)
}

// updateAdditionalStoragePoolsAnnotation updates only the additionalStoragePools annotation
func (h *StorageClassHandler) updateAdditionalStoragePoolsAnnotation(sc *storagev1.StorageClass) error {
	// Get the latest version of the StorageClass
	latestSC, err := h.controller.Clients.KubeClient.StorageV1().StorageClasses().Get(ctx(), sc.Name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// StorageClass was deleted, nothing to update
			return nil
		}
		return fmt.Errorf("failed to get StorageClass: %v", err)
	}

	// Initialize annotations if nil
	if latestSC.Annotations == nil {
		latestSC.Annotations = make(map[string]string)
	}

	// Get current additionalStoragePools annotation value
	currentValue := latestSC.Annotations[AdditionalStoragePoolsAnnotation]

	// Build the new additionalStoragePools annotation value
	newValue, err := h.buildAdditionalStoragePoolsValue(latestSC)
	if err != nil {
		return fmt.Errorf("failed to build additionalStoragePools annotation: %v", err)
	}

	// Only update if the value has changed
	if currentValue != newValue {
		if newValue != "" {
			latestSC.Annotations[AdditionalStoragePoolsAnnotation] = newValue
			Log().WithFields(LogFields{
				"storageClass":           sc.Name,
				"additionalStoragePools": newValue,
			}).Info("Updating additionalStoragePools annotation")
		} else {
			// Remove the annotation if there are no additional pools
			delete(latestSC.Annotations, AdditionalStoragePoolsAnnotation)
			Log().WithField("storageClass", sc.Name).Debug("Removing additionalStoragePools annotation")
		}

		// Update the StorageClass
		_, err = h.controller.Clients.KubeClient.StorageV1().StorageClasses().Update(ctx(), latestSC, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update StorageClass annotations: %v", err)
		}

		Log().WithFields(LogFields{
			"storageClass": sc.Name,
			"oldValue":     currentValue,
			"newValue":     newValue,
		}).Debug("Updated additionalStoragePools annotation")
	}

	return nil
}

// syncTridentConfiguratorStatus syncs the TridentConfigurator status back to StorageClass
func (h *StorageClassHandler) syncTridentConfiguratorStatus(sc *storagev1.StorageClass, tconf *netappv1.TridentConfigurator) error {
	status := StatusPending
	message := "Processing"

	if tconf.Status.LastOperationStatus == string(netappv1.Success) {
		status = StatusSuccess
		message = tconf.Status.Message
	} else if tconf.Status.LastOperationStatus == string(netappv1.Failed) {
		status = StatusFailed
		message = tconf.Status.Message
	} else if tconf.Status.LastOperationStatus == string(netappv1.Processing) {
		status = StatusPending
		message = tconf.Status.Message
	}

	return h.updateStorageClassStatus(sc, status, message)
}

// updateStorageClassStatus updates the StorageClass annotations with status
func (h *StorageClassHandler) updateStorageClassStatus(sc *storagev1.StorageClass, status, message string) error {
	// Get the latest version of the StorageClass
	latestSC, err := h.controller.Clients.KubeClient.StorageV1().StorageClasses().Get(ctx(), sc.Name, metav1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// StorageClass was deleted, nothing to update
			return nil
		}
		return fmt.Errorf("failed to get StorageClass: %v", err)
	}

	// Initialize annotations if nil
	if latestSC.Annotations == nil {
		latestSC.Annotations = make(map[string]string)
	}

	// Update status annotations
	tconfName := h.getTridentConfiguratorName(sc)
	latestSC.Annotations[TridentConfiguratorNameAnnotation] = tconfName
	latestSC.Annotations[TridentConfiguratorStatusAnnotation] = status
	latestSC.Annotations[TridentConfiguratorMessageAnnotation] = message
	latestSC.Annotations[TridentManagedAnnotation] = "true"

	// Update the StorageClass
	_, err = h.controller.Clients.KubeClient.StorageV1().StorageClasses().Update(ctx(), latestSC, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update StorageClass annotations: %v", err)
	}

	Log().WithFields(LogFields{
		"storageClass": sc.Name,
		"status":       status,
		"message":      message,
	}).Debug("Updated StorageClass status")

	return nil
}

// mapsEqual checks if two string maps are equal
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
