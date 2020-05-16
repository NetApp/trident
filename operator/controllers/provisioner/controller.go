// Copyright 2020 NetApp, Inc. All Rights Reserved.

package provisioner

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	"github.com/netapp/trident/operator/clients"
	netappv1 "github.com/netapp/trident/operator/controllers/provisioner/apis/netapp/v1"
	"github.com/netapp/trident/operator/controllers/provisioner/client/clientset/versioned/scheme"
	"github.com/netapp/trident/operator/controllers/provisioner/installer"
	"github.com/netapp/trident/utils"
)

type AppStatus string
type ResourceType string //If Operator starts to List and Watch other CR types, this can be used to differentiate.

const (
	ControllerName    = "Trident Provisioner"
	ControllerVersion = "0.1"
	CRName            = "TridentProvisioner"
	Operator          = "trident-operator.netapp.io"
	CacheSyncPeriod   = 300 * time.Second
	installTimeout    = 30 * time.Second

	AppStatusNotInstalled AppStatus = ""             // default
	AppStatusInstalling   AppStatus = "Installing"   // Set only on controlling CR
	AppStatusInstalled    AppStatus = "Installed"    // Set only on controlling CR
	AppStatusUninstalling AppStatus = "Uninstalling" // Set only on controlling CR
	AppStatusUninstalled  AppStatus = "Uninstalled"  // Set only on controlling CR
	AppStatusFailed       AppStatus = "Failed"       // Set only on controlling CR
	AppStatusUpdating     AppStatus = "Updating"     // Set only on controlling CR
	AppStatusError        AppStatus = "Error"        // Should not be set on controlling CR

	ResourceTridentProvisionerCR ResourceType = "resourceTridentProvisionerCR"
	ResourceDeployment           ResourceType = "resourceDeployment"
	ResourceDaemonSet            ResourceType = "resourceDaemonset"

	UninstallationNote = ". NOTE: This CR has uninstalled status; delete this CR to allow new Trident installation."
)

var (
	listOpts   = metav1.ListOptions{}
	updateOpts = metav1.UpdateOptions{}

	ctx = context.TODO
)

type KeyItem struct {
	keyDetails   string
	resourceType ResourceType
}

type Controller struct {
	*clients.Clients
	mutex *sync.Mutex

	eventRecorder      record.EventRecorder
	indexerCR          cache.Indexer
	deploymentIndexer  cache.Indexer
	daemonsetIndexer   cache.Indexer
	informerCR         cache.SharedIndexInformer
	deploymentInformer cache.SharedIndexInformer
	daemonsetInformer  cache.SharedIndexInformer
	watcherCR          cache.ListerWatcher
	deploymentWatcher  cache.ListerWatcher
	daemonsetWatcher   cache.ListerWatcher
	stopChan           chan struct{}

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
}

func NewController(clients *clients.Clients) (*Controller, error) {

	log.Infof("Initializing %s controller.", ControllerName)

	c := &Controller{
		Clients:   clients,
		mutex:     &sync.Mutex{},
		stopChan:  make(chan struct{}),
		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "TridentProvisioner"),
	}

	// Set up event broadcaster
	utilruntime.Must(scheme.AddToScheme(scheme.Scheme))
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&clientv1.EventSinkImpl{Interface: c.KubeClient.CoreV1().Events("")})
	c.eventRecorder = broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: Operator})

	// Set up a watch for TridentProvisioner CRs
	c.watcherCR = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return c.CRDClient.TridentV1().TridentProvisioners(corev1.NamespaceAll).List(ctx(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return c.CRDClient.TridentV1().TridentProvisioners(corev1.NamespaceAll).Watch(ctx(), options)
		},
	}

	// Set up a watch for Trident Deployment
	c.deploymentWatcher = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.LabelSelector = installer.LabelSelector
			return c.KubeClient.AppsV1().Deployments(corev1.NamespaceAll).List(ctx(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.LabelSelector = installer.LabelSelector
			return c.KubeClient.AppsV1().Deployments(corev1.NamespaceAll).Watch(ctx(), options)
		},
	}

	// Set up a watch for Trident Daemonset
	c.daemonsetWatcher = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			options.LabelSelector = installer.TridentNodeLabel
			return c.KubeClient.AppsV1().DaemonSets(corev1.NamespaceAll).List(ctx(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			options.LabelSelector = installer.TridentNodeLabel
			return c.KubeClient.AppsV1().DaemonSets(corev1.NamespaceAll).Watch(ctx(), options)
		},
	}

	// Set up the CR indexing controller
	c.informerCR = cache.NewSharedIndexInformer(
		c.watcherCR,
		&netappv1.TridentProvisioner{},
		CacheSyncPeriod,
		cache.Indexers{},
	)
	c.indexerCR = c.informerCR.GetIndexer()

	// Set up the deployment indexing controller
	c.deploymentInformer = cache.NewSharedIndexInformer(
		c.deploymentWatcher,
		&appsv1.Deployment{},
		0,
		cache.Indexers{},
	)
	c.deploymentIndexer = c.deploymentInformer.GetIndexer()

	// Set up the deployment indexing controller
	c.daemonsetInformer = cache.NewSharedIndexInformer(
		c.daemonsetWatcher,
		&appsv1.DaemonSet{},
		0,
		cache.Indexers{},
	)
	c.daemonsetIndexer = c.daemonsetInformer.GetIndexer()

	// Add handlers for TridentProvisioner CRs
	c.informerCR.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.addProvisioner,
			UpdateFunc: c.updateProvisioner,
			DeleteFunc: c.deleteProvisioner,
		},
	)

	// Add handlers for TridentProvisioner CRs
	c.deploymentInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.deploymentAddedOrDeleted,
			UpdateFunc: c.deploymentUpdated,
			DeleteFunc: c.deploymentAddedOrDeleted,
		},
	)

	// Add handlers for TridentProvisioner CRs
	c.daemonsetInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.daemonsetAddedOrDeleted,
			UpdateFunc: c.daemonsetUpdated,
			DeleteFunc: c.daemonsetAddedOrDeleted,
		},
	)

	return c, nil
}

func (c *Controller) Activate() error {
	log.Infof("Activating %s controller.", ControllerName)
	go c.informerCR.Run(c.stopChan)
	go c.deploymentInformer.Run(c.stopChan)
	go c.daemonsetInformer.Run(c.stopChan)

	log.Info("Starting workers")
	go wait.Until(c.runWorker, time.Second, c.stopChan)

	log.Info("Started workers")

	return nil
}

func (c *Controller) Deactivate() error {
	log.Infof("Deactivating %s controller.", ControllerName)

	close(c.stopChan)

	c.workqueue.ShutDown()
	utilruntime.HandleCrash()
	return nil
}

func (c *Controller) GetName() string {
	return ControllerName
}

func (c *Controller) Version() string {
	return ControllerVersion
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var keyItem KeyItem
		var ok bool
		// We expect KeyItem to come off the workqueue. We do this as the
		// delayed nature of the workqueue means the items in the informer
		// cache may actually be more up to date that when the item was
		// initially put onto the workqueue.
		if keyItem, ok = obj.(KeyItem); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			log.Errorf("expected string in workqueue but got %#v", obj)
			return nil
		}
		// Run the reconcile, passing it the keyItems struct to be synced.
		if err := c.reconcile(keyItem); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			if utils.IsUnsupportedConfigError(err) {
				errMessage := fmt.Sprintf("found unsupported configuration, "+
					"needs manual intervention to fix the issue;"+
					"error syncing '%s': %s, not requeuing", keyItem.keyDetails, err.Error())

				c.workqueue.Forget(keyItem)

				log.Errorf(errMessage)
				log.Info("-------------------------------------------------")
				log.Info("-------------------------------------------------")

				return fmt.Errorf(errMessage)
			} else if utils.IsReconcileIncompleteError(err) {
				c.workqueue.Add(keyItem)
			} else {
				c.workqueue.AddRateLimited(keyItem)
			}

			errMessage := fmt.Sprintf("error syncing '%s': %s, requeuing", keyItem.keyDetails, err.Error())
			log.Errorf(errMessage)
			log.Info("-------------------------------------------------")
			log.Info("-------------------------------------------------")

			return fmt.Errorf(errMessage)
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		log.Infof("Synced %s '%s'", c.resourceTypeToK8sKind(keyItem.resourceType), keyItem.keyDetails)
		log.Info("-------------------------------------------------")
		log.Info("-------------------------------------------------")

		return nil
	}(obj)

	if err != nil {
		log.Error(err)
		return true
	}

	return true
}

// addProvisioner is the add handler for the TridentProvisioner watcher.
func (c *Controller) addProvisioner(obj interface{}) {
	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		log.Error(err)
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: %s", key)
		return
	}

	log.WithFields(log.Fields{
		"name":      name,
		"namespace": namespace,
	}).Infof("%s CR added.", CRName)

	keyItem := KeyItem{
		keyDetails:   key,
		resourceType: ResourceTridentProvisionerCR,
	}

	c.workqueue.Add(keyItem)
}

// updateProvisioner is the update handler for the TridentProvisioner watcher.
func (c *Controller) updateProvisioner(oldObj, newObj interface{}) {

	_, ok := oldObj.(*netappv1.TridentProvisioner)
	if !ok {
		log.Errorf("'%s' controller expected '%s' CR; got '%v'", ControllerName, CRName, oldObj)
		return
	}

	newCR, ok := newObj.(*netappv1.TridentProvisioner)
	if !ok {
		log.Errorf("'%s' controller expected '%s' CR; got '%v'", ControllerName, CRName, newObj)
		return
	}

	if !newCR.ObjectMeta.DeletionTimestamp.IsZero() {
		log.WithFields(log.Fields{
			"name":              newCR.Name,
			"namespace":         newCR.Namespace,
			"deletionTimestamp": newCR.ObjectMeta.DeletionTimestamp,
		}).Infof("'%s' CR is being deleted, not updated.", CRName)
		return
	}

	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(newObj); err != nil {
		log.Error(err)
		return
	}

	log.WithFields(log.Fields{
		"name":      newCR.Name,
		"namespace": newCR.Namespace,
	}).Infof("'%s' CR updated.", CRName)

	keyItem := KeyItem{
		keyDetails:   key,
		resourceType: ResourceTridentProvisionerCR,
	}

	c.workqueue.Add(keyItem)
}

// deleteProvisioner is the delete handler for the TridentProvisioner watcher.
func (c *Controller) deleteProvisioner(obj interface{}) {
	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		log.Error(err)
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: '%s'", key)
		return
	}

	log.WithFields(log.Fields{
		"name":      name,
		"namespace": namespace,
	}).Infof("'%s' CR deleted.", CRName)

	keyItem := KeyItem{
		keyDetails:   key,
		resourceType: ResourceTridentProvisionerCR,
	}

	c.workqueue.Add(keyItem)
}

// deploymentAddedOrDeleted is the handler for the trident-csi deployment watcher.
func (c *Controller) deploymentAddedOrDeleted(obj interface{}) {
	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		log.Error(err)
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: '%s'", key)
		return
	}

	log.WithFields(log.Fields{
		"name":      name,
		"namespace": namespace,
	}).Infof("Deployment changed.")

	keyItem := KeyItem{
		keyDetails:   key,
		resourceType: ResourceDeployment,
	}

	c.workqueue.Add(keyItem)
}

// deploymentUpdated is the handler for the trident-csi deployment watcher.
func (c *Controller) deploymentUpdated(newObj interface{}, oldObj interface{}) {
	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(newObj); err != nil {
		log.Error(err)
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: '%s'", key)
		return
	}

	newDepl := newObj.(*appsv1.Deployment)
	oldDepl := oldObj.(*appsv1.Deployment)

	// Periodic resync will send update events for all known Deployments.
	// Two different versions of the same Deployment will always have different RVs.
	if newDepl.ResourceVersion == oldDepl.ResourceVersion {
		log.Debugf("Deployment new and old resource version are same")
		return
	}

	newDeplCopy := newDepl.DeepCopy()
	oldDeplCopy := oldDepl.DeepCopy()

	// This strategy identifies if deployment was updated as a result of something other than
	// no-op reconciliation patch
	newDeplCopy.Generation = oldDeplCopy.Generation
	newDeplCopy.ResourceVersion = oldDeplCopy.ResourceVersion
	newDeplCopy.Status.ObservedGeneration = oldDeplCopy.Status.ObservedGeneration
	newDeplCopy.Annotations = oldDeplCopy.Annotations

	if reflect.DeepEqual(newDeplCopy, oldDeplCopy) {
		log.Debugf("Ignoring deployment resource event handler updates due to reconcile no-ops patch.")
		return
	}

	log.WithFields(log.Fields{
		"name":      name,
		"namespace": namespace,
	}).Infof("Deployment changed.")

	keyItem := KeyItem{
		keyDetails:   key,
		resourceType: ResourceDeployment,
	}

	c.workqueue.Add(keyItem)
}

// daemonsetAddedOrDeleted is the handler for the trident-csi daemonset watcher.
func (c *Controller) daemonsetAddedOrDeleted(obj interface{}) {
	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		log.Error(err)
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: '%s'", key)
		return
	}

	log.WithFields(log.Fields{
		"name":      name,
		"namespace": namespace,
	}).Infof("Daemonset changed.")

	keyItem := KeyItem{
		keyDetails:   key,
		resourceType: ResourceDaemonSet,
	}

	c.workqueue.Add(keyItem)
}

// daemonsetUpdated is the handler for the trident-csi daemonset watcher.
func (c *Controller) daemonsetUpdated(newObj interface{}, oldObj interface{}) {
	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(newObj); err != nil {
		log.Error(err)
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: '%s'", key)
		return
	}

	log.WithFields(log.Fields{
		"name":      name,
		"namespace": namespace,
	}).Infof("Daemonset changed.")

	keyItem := KeyItem{
		keyDetails:   key,
		resourceType: ResourceDaemonSet,
	}

	c.workqueue.Add(keyItem)
}

// unsupportedInstallationsPrechecks identifies the state of the Trident installation(s)
// 1. If we are in a Greenfield scenario and come across legacy, CSI Preview or tridentctl based
//    Trident installations we send a configuration error.
// 2. However, if legacy, CSI Preview or tridentctl based Trident are installed after installing the
//    operator, these will be removed eventually.
func (c *Controller) unsupportedInstallationsPrechecks(controllingCRBasedOnStatusExists,
	operatorCSIDeploymentFound bool) error {

	// CSI Preview Trident should not be present
	if csiPreviewTridentInstalled, CSIPreviewNamespace, err := c.isPreviewCSITridentInstalled(); err != nil {
		return utils.ReconcileFailedError(err)
	} else if csiPreviewTridentInstalled {
		if controllingCRBasedOnStatusExists || operatorCSIDeploymentFound {
			log.Debug("Identified that CSI Preview Trident was installed after Trident Operator created installation.")
			if err := c.uninstallCSIPreviewTrident(CSIPreviewNamespace); err != nil {
				log.Errorf("Unable to remove CSI Preview Trident; err: %v", err)
				return utils.ReconcileFailedError(err)
			} else {
				log.Debug("Removed CSI Preview Trident.")
				return utils.ReconcileIncompleteError()
			}
		} else {
			// This should have been a greenfield scenario but CSI Preview Trident already exists
			errorMessage := fmt.Sprintf("Operator cannot proceed with the installation, "+
				"found CSI Preview Trident already installed in namespace '%v'.", CSIPreviewNamespace)
			log.Error(errorMessage)
			c.updateAllCRs(errorMessage)
			return utils.UnsupportedConfigError(fmt.Errorf(errorMessage))
		}
	}

	// Legacy Trident should not be present
	legacyDeploymentFound, legacyTridentNamespace, err := c.isLegacyTridentInstalled()
	if err != nil {
		return utils.ReconcileFailedError(err)
	} else if legacyDeploymentFound {
		log.Debug("Identified that legacy Trident was installed after Trident Operator created installation.")
		if controllingCRBasedOnStatusExists || operatorCSIDeploymentFound {
			if err := c.uninstallLegacyTrident(legacyTridentNamespace); err != nil {
				log.Errorf("Unable to remove legacy Trident; err: %v", err)
				return utils.ReconcileFailedError(err)
			} else {
				log.Debug("Removed legacy Trident.")
				return utils.ReconcileIncompleteError()
			}
		} else {
			errorMessage := fmt.Sprintf("Operator cannot proceed with the installation, "+
				"found non-CSI Trident already installed in namespace '%v'.", legacyTridentNamespace)
			log.Error(errorMessage)
			c.updateAllCRs(errorMessage)
			return utils.UnsupportedConfigError(fmt.Errorf(errorMessage))
		}
	}

	// Check for the CSI based Trident installation
	csiDeploymentFound, csiTridentNamespace, err := c.isCSITridentInstalled()
	if err != nil {
		return utils.ReconcileFailedError(err)
	} else if csiDeploymentFound {
		if controllingCRBasedOnStatusExists || operatorCSIDeploymentFound {
			// nothing to do, if we have any invalid Trident deployments, these will get removed as part of the
			// auto-heal code
			log.Debug("CSI based Trident installation exist.")
		} else {
			errorMessage := fmt.Sprintf("Operator cannot proceed with the installation, "+
				"found CSI Trident already installed in namespace '%v'.", csiTridentNamespace)
			log.Error(errorMessage)
			c.updateAllCRs(errorMessage)
			return utils.UnsupportedConfigError(fmt.Errorf(errorMessage))
		}
	}

	return nil
}

// alphaSnapshotCRDsExist identifies if the alpha snapshot CRDs not present
func (c *Controller) alphaSnapshotCRDsExist() (bool, []string, error) {
	var alphaSnapshotCRDsExist bool
	var alphaSnapshotCRDsList []string

	for _, crdName := range installer.AlphaCRDNames {

		// See if CRD exists
		crdsExist, returnError := c.K8SClient.CheckCRDExists(crdName)
		if returnError != nil {
			return alphaSnapshotCRDsExist, alphaSnapshotCRDsList, utils.ReconcileFailedError(returnError)
		}
		if !crdsExist {
			log.WithField("CRD", crdName).Debug("Alpha snapshot CRD not present.")
			continue
		}

		// Get the CRD and check version
		crd, returnError := c.K8SClient.GetCRD(crdName)
		if returnError != nil {
			return alphaSnapshotCRDsExist, alphaSnapshotCRDsList, utils.ReconcileFailedError(returnError)
		}

		for _, version := range crd.Spec.Versions {
			if strings.ToLower(version.Name) == "v1alpha1" {
				alphaSnapshotCRDsExist = true
				alphaSnapshotCRDsList = append(alphaSnapshotCRDsList, crdName)
				log.WithField("CRD", crdName).Debug("Alpha snapshot CRD present.")
			}
		}
	}

	return alphaSnapshotCRDsExist, alphaSnapshotCRDsList, nil
}

// alphaSnapshotCRDsPreinstallationCheck identifies if the alpha snapshot CRDs are present before any Trident
// installation by the operator
func (c *Controller) alphaSnapshotCRDsPreinstallationCheck() error {

	alphaSnapshotCRDsExist, alphaSnapshotCRDsList, err := c.alphaSnapshotCRDsExist()
	if err != nil {
		return err
	}

	errorMessage := fmt.Sprintf("Operator cannot proceed with the installation due to alpha snapshot"+
		" CRDs %v; run `tridentctl obliviate alpha-snapshot-crd` to remove previous kubernetes snapshot"+
		" CRDs; for details, please refer to Trident's online documentation", alphaSnapshotCRDsList)

	if alphaSnapshotCRDsExist {
		log.Error(errorMessage)
		c.updateAllCRs(errorMessage)
		return utils.UnsupportedConfigError(fmt.Errorf(errorMessage))
	}

	return nil
}

// alphaSnapshotCRDsPostinstallationCheck identifies if the alpha snapshot CRDs are present after any Trident
// installation by the operator, this should only be called once controllingCR has been identified and before
// installing, patching, updating the Trident
func (c *Controller) alphaSnapshotCRDsPostinstallationCheck(tridentCR *netappv1.TridentProvisioner,
	currentInstalledTridentVersion string) error {

	alphaSnapshotCRDsExist, alphaSnapshotCRDsList, err := c.alphaSnapshotCRDsExist()
	if err != nil {
		return err
	}

	errorMessage := fmt.Sprintf("Operator cannot proceed with the installation due to alpha snapshot"+
		" CRDs %v; run `tridentctl obliviate alpha-snapshot-crd` to remove previous kubernetes snapshot"+
		" CRDs; for details, please refer to Trident's online documentation", alphaSnapshotCRDsList)

	if alphaSnapshotCRDsExist {

		log.Error(errorMessage)

		// Update status of the tridentCR  to `Failed`
		logMessage := "Updating Trident Provisioner CR after failed alpha snapshot CRDs check."

		// Log error in the event recorder and return error message
		c.eventRecorder.Event(tridentCR, corev1.EventTypeWarning, string(AppStatusFailed), errorMessage)

		c.updateCRStatus(tridentCR, logMessage, errorMessage, string(AppStatusFailed), currentInstalledTridentVersion)

		// Alpha snapshot CRDs check failed, so fail the reconcile loop
		return utils.ReconcileFailedError(fmt.Errorf(errorMessage))
	}

	return nil
}

// k8sVersionPreinstallationCheck identifies if K8s version is valid or not
func (c *Controller) k8sVersionPreinstallationCheck() error {

	if isCurrentK8sVersionValid, warningMessage := c.validateCurrentK8sVersion(); !isCurrentK8sVersionValid {
		log.Error(warningMessage)
		c.updateAllCRs(warningMessage)
		return utils.UnsupportedConfigError(fmt.Errorf(warningMessage))
	}

	return nil
}

// reconcile runs the reconcile logic and ensures we move to the desired state of the desired state is
// maintained
func (c *Controller) reconcile(key KeyItem) error {

	// Check if there already exists a controllingCR - if deployment is deleted it is possible that we may run
	// into a situation where there is no deployment but there is a controlling CR
	controllingCRBasedOnStatusExists, controllingCRBasedOnStatus, err := c.identifyControllingCRBasedOnStatus()
	if err != nil {
		return utils.ReconcileFailedError(fmt.Errorf("unable to identify if controlling CR exists; err: %v", err))
	}

	// Check if Operator based CSI Trident installed
	var operatorCSIDeploymentFound bool
	operatorCSIDeployments, operatorCSIDeploymentNamespace, err := c.operatorCSIDeployments()
	if err != nil {
		return utils.ReconcileFailedError(fmt.Errorf(
			"unable to identify if Operator based CSI Trident installation(s) exist; err: %v", err))
	} else if len(operatorCSIDeployments) > 0 {
		log.Debugf("Found atleast one CSI Trident deployment created by the operator in namespace: %s.",
			operatorCSIDeploymentNamespace)
		operatorCSIDeploymentFound = true
	} else {
		log.Debugf("No operator based CSI Trident deployment found.")
	}

	if err := c.unsupportedInstallationsPrechecks(controllingCRBasedOnStatusExists,
		operatorCSIDeploymentFound); err != nil {
		return err
	}

	if operatorCSIDeploymentFound || controllingCRBasedOnStatusExists {
		return c.reconcileTridentPresent(key, operatorCSIDeployments, operatorCSIDeploymentNamespace, true,
			controllingCRBasedOnStatus)
	} else {

		// These are the pre-installation checks that have different behavior depending upon Trident via Operator is
		// already installed or not.

		// Before the installation ensure K8s version is valid
		if err := c.k8sVersionPreinstallationCheck(); err != nil {
			return err
		}

		// Ensure no alpha-snapshot CRDs are present
		if err := c.alphaSnapshotCRDsPreinstallationCheck(); err != nil {
			return err
		}

		return c.reconcileTridentNotPresent()
	}
}

func (c *Controller) reconcileTridentPresent(key KeyItem, operatorCSIDeployments []appsv1.Deployment,
	deploymentNamespace string, isCSI bool,
	controllingCRBasedOnStatus *netappv1.TridentProvisioner) error {

	var controllingCRBasedOnStatusName, controllingCRBasedOnStatusNamespace string
	if controllingCRBasedOnStatus != nil {
		controllingCRBasedOnStatusName = controllingCRBasedOnStatus.Name
		controllingCRBasedOnStatusNamespace = controllingCRBasedOnStatus.Namespace
	}

	// Convert the key CR's namespace/name string into a distinct namespace and name
	callingCRNamespace, callingCRName, err := cache.SplitMetaNamespaceKey(key.keyDetails)
	if err != nil {
		return utils.ReconcileFailedError(fmt.Errorf("invalid resource key: '%s'", key))
	}

	// Name of all operator based Trident CSI Deployments
	var operatorCSIDeploymentNames []string
	for _, deployment := range operatorCSIDeployments {
		operatorCSIDeploymentNames = append(operatorCSIDeploymentNames, deployment.Name)
	}

	callingResourceType := key.resourceType

	log.WithFields(log.Fields{
		"callingCRName":                       callingCRName,
		"callingCRNamespace":                  callingCRNamespace,
		"callingResourceType":                 callingResourceType,
		"namespace":                           deploymentNamespace,
		"operatorBasedCSIDeployments":         operatorCSIDeploymentNames,
		"isCSI":                               isCSI,
		"controllingCRBasedOnStatus":          controllingCRBasedOnStatusName,
		"controllingCRBasedOnStatusNamespace": controllingCRBasedOnStatusNamespace,
	}).Info("Reconciler found Trident installation.")

	// Identify current Trident installation CR if deployment already exist
	var controllingCRBasedOnInstall *netappv1.TridentProvisioner
	operatorCSIDeploymentsExist := len(operatorCSIDeployments) > 0
	if operatorCSIDeploymentsExist {
		controllingCRBasedOnInstall, err = c.getControllingCRForTridentDeployments(operatorCSIDeployments)
		if err != nil {
			return err
		}
	}

	// If we are here, one of the following scenarios could be true:
	//
	// 1. controllingCRBasedOnStatus exist, and Trident deployment (one controlled by a CR)
	//    also exists so we should have a controllingCRBasedOnInstall, then controllingCRBasedOnInstall
	//    and the controllingCRBasedOnStatus should be same, this controllingCRBasedOnStatus should be
	//    used for Trident installation and other operation. This should be the case most of the time.
	//
	// 2. controllingCRBasedOnStatus exist, but Trident deployment does not exist or if it exits it
	//    doesn't have a ControllingCR then this controllingCRBasedOnStatus should be used for
	//    Trident installation and other operation. This situation occurs if the Trident deployment
	//    was deleted or invalid deployment matching Trident label was installed.
	//
	// 3. controllingCRBasedOnStatus does not exist, but Trident deployment (one controlled by a CR)
	//    exist so we should have a controllingCRBasedOnInstall, then controllingCRBasedOnInstall
	//    should be assigned to the controllingCRBasedOnStatus and should be used for Trident
	//    installation and other operation. This situation should ideally not occur.
	//
	// 4. controllingCRBasedOnStatus does not exist, but there exist a deployment(s) without a
	//    controllingCR, this could mean ControllingCR was just deleted and deployment is going
	//    to be deleted soon, therefore we need to run uninstallation logic to make sure any
	//    custom uninstallation logic is being triggered.

	var controllingCR *netappv1.TridentProvisioner

	if controllingCRBasedOnStatus != nil {
		if controllingCRBasedOnInstall != nil {
			if controllingCRBasedOnInstall.Name != controllingCRBasedOnStatus.Name ||
				controllingCRBasedOnInstall.Namespace != controllingCRBasedOnStatus.Namespace {
				// We should not be in this situation
				// This cannot be set in an event log as we are not clear which of the CR is a controllingCR
				return utils.ReconcileFailedError(fmt.Errorf("current Trident installation CR "+
					"'%v/%v' and identified controlling CR '%v/%v' are different", controllingCRBasedOnInstall.Namespace,
					controllingCRBasedOnInstall.Name, controllingCRBasedOnStatus.Namespace, controllingCRBasedOnStatus.Name))
			}
		}

		controllingCR = controllingCRBasedOnStatus
	} else {
		if controllingCRBasedOnInstall == nil {
			// We should not be in this situation, removing all the Trident deployments and other remanents
			if err := c.uninstallTridentAll(metav1.NamespaceDefault); err != nil {
				return utils.ReconcileFailedError(err)
			}
			// Uninstall succeeded, so re-run the reconcile loop
			return utils.ReconcileIncompleteError()
		}

		controllingCR = controllingCRBasedOnInstall
	}

	// If we are here we have identified the controllingCR,
	// also if we also have a deployment then controllingCR and
	// deployment are in the same namespace
	log.Debugf("Controlling CR: '%v', Controlling CR Namespace: '%v'", controllingCR.Name, controllingCR.Namespace)

	// Perform controllingCRBasedReconcile i.e install/patch/update/uninstall only if:
	// 1. callingResourceType IS NOT ResourceTridentProvisionerCR, to allow changes to deployment/daemonset to trigger reconcile
	// 2. callingResourceType IS ResourceTridentProvisionerCR, then callingCR should be same as controllingCR
	if callingResourceType != ResourceTridentProvisionerCR || (callingCRName == controllingCR.
		Name && callingCRNamespace == controllingCR.Namespace) {
		log.Debugf("'%v' in namespace '%v' is a controlling CR based on status.", controllingCR.Name,
			controllingCR.Namespace)
		if err = c.controllingCRBasedReconcile(controllingCR, operatorCSIDeploymentsExist); err != nil {
			return err
		}
	} else {
		log.Debugf("'%v' in namespace '%v' is not a controlling CR based on status; skipping install/uninstall re-run",
			callingCRName, callingCRNamespace)
	}

	err = c.updateOtherCRs(controllingCR.Name, controllingCR.Namespace)

	return err
}

func (c *Controller) reconcileTridentNotPresent() error {

	log.Info("Reconciler found no Trident installation.")

	// Get all TridentProvisioner CRs
	tridentCRs, err := c.getTridentCRsInNamespace(corev1.NamespaceAll)
	if err != nil {
		return err
	} else if len(tridentCRs) == 0 {
		log.Info("Reconciler found no TridentProvisioner CRs, nothing to do.")
		return nil
	}

	// Iterate through all the CRs, identify if any of the CRs has status "Uninstalled" then return,
	// until this CR is removed cannot perform new Trident installation.
	for _, cr := range tridentCRs {
		if cr.Status.Status == string(AppStatusUninstalled) {
			statusMessage := fmt.Sprintf("Remove CR '%v' in namespace '%v' with uninstalled status to allow new Trident"+
				" installation.", cr.Name, cr.Namespace)
			c.eventRecorder.Event(&cr, corev1.EventTypeWarning, cr.Status.Status, statusMessage)
			log.Warnf(statusMessage)

			return nil
		}
	}

	// Iterate through all the CRs, and follow this preference logic:
	// 1. If no CR has Uninstalled status, then identify CR that has Uninstall not set to true
	// 2. Prefer a CR in the same namespace as the Operator
	var tridentCR *netappv1.TridentProvisioner
	for _, cr := range tridentCRs {
		if cr.Spec.Uninstall != true {
			tridentCR = &cr
			if cr.Namespace == c.Namespace {
				break
			}
		}
	}

	if tridentCR == nil {
		log.Warnf("Reconciler found no valid TridentProvisioner CRs, nothing to do.")
		return nil
	}

	// Update status of the tridentCR  to `Installing`
	logMessage := "Updating TridentProvisioner CR before new installation"
	statusMessage := "Installing Trident"

	c.eventRecorder.Event(tridentCR, corev1.EventTypeNormal, string(AppStatusInstalling), statusMessage)
	newTridentCR, err := c.updateCRStatus(tridentCR, logMessage, statusMessage, string(AppStatusInstalling), "")
	if err != nil {
		return utils.ReconcileFailedError(fmt.Errorf(
			"unable to update status of the CR '%v' in namespace '%v' to installing",
			tridentCR.Name, tridentCR.Namespace))
	}

	if err := c.installTridentAndUpdateStatus(*newTridentCR, "", "", false); err != nil {
		// Install failed, so fail the reconcile loop
		return utils.ReconcileFailedError(fmt.Errorf(
			"error installing Trident using CR '%v' in namespace '%v'; err: %v",
			newTridentCR.Name, tridentCR.Namespace, err))
	}

	err = c.updateOtherCRs(newTridentCR.Name, newTridentCR.Namespace)

	return err
}

// controllingCRBasedReconcile is the core reconciliation or in other words maintenance logic i.e.
// we know the ControllingCR, therefore we know the Specs of the ControllingCR, use that spec to
// ensure we are maintaining the desired state.
func (c *Controller) controllingCRBasedReconcile(controllingCR *netappv1.TridentProvisioner,
	deploymentExist bool) error {
	// Check to see if controllingCR status is uninstalled, if this is the case installation/patch should not be run
	if controllingCR.Status.Status == string(AppStatusUninstalled) {

		// If for some reason deployment exists remove Trident installation to make sure Trident remains in
		// uninstalled state
		if deploymentExist {
			log.Warnf("Even though controlling CR '%v' in namespace '%v' has status %v, "+
				"there exists Trident installation; re-running Trident uninstallation",
				controllingCR.Name, controllingCR.Namespace, controllingCR.Status.Status)

		}

		// This uninstallation would merely fix the state by removing deployment and match the status
		// Uninstalled, there is no need to update status after deployment is removed successfully.
		if err := c.uninstallTridentAll(controllingCR.Namespace); err != nil {
			return utils.ReconcileFailedError(err)
		}

		var crdNote string
		if deletedCRDs, err := c.wipeout(*controllingCR); err != nil {
			return err
		} else if deletedCRDs {
			log.Info("Trident CRDs removed.")
			crdNote = " and removed CRDs"
		}

		logMessage := "Updating TridentProvisioner CR after uninstallation."
		statusMessage := "Uninstalled Trident" + crdNote + UninstallationNote

		c.eventRecorder.Event(controllingCR, corev1.EventTypeWarning, controllingCR.Status.Status, statusMessage)
		c.updateCRStatus(controllingCR, logMessage, statusMessage, string(AppStatusUninstalled), "")
		log.Warnf(fmt.Sprintf("Remove CR '%v' in namespace '%v' with uninstalled status to allow new Trident"+
			" installation.", controllingCR.Name, controllingCR.Namespace))

		return c.updateOtherCRs(controllingCR.Name, controllingCR.Namespace)
	}

	// Get current Version information, to update CRs with the correct version information and in K8s case
	// identify if update might be required (for installation only) due to change in K8s version.
	currentInstalledTridentVersion, tridentK8sConfigVersion, err := c.getCurrentTridentAndK8sVersion(controllingCR)
	if err != nil {
		// Failed to identify current trident version and K8s version
		log.Errorf("error identifying update scenario for CR named '%v' in namespace '%v'; err: %v",
			controllingCR.Name, controllingCR.Namespace, err)
	}

	// Check to see if controllingCR spec has changed and is requesting for uninstallation
	uninstall := controllingCR.Spec.Uninstall

	if uninstall {
		if _, err := c.uninstallTridentAndUpdateStatus(*controllingCR, currentInstalledTridentVersion); err != nil {
			// Install failed, so fail the reconcile loop
			return utils.ReconcileFailedError(fmt.Errorf(
				"error uninstalling Trident controlled by CR '%v' in namespace '%v'; err: %v",
				controllingCR.Name, controllingCR.Namespace, err))
		}

		log.Warnf(fmt.Sprintf("Remove CR '%v' in namespace '%v' with uninstalled status to allow new Trident"+
			" installation.", controllingCR.Name, controllingCR.Namespace))
	} else {

		// There are certain checks that should be run before each install, update, patch

		// Check: Alpha-snapshot CRDs should not be present
		if err = c.alphaSnapshotCRDsPostinstallationCheck(controllingCR, currentInstalledTridentVersion); err != nil {
			return err
		}

		// Check: Current K8s version should be supported, if not is there a warning message to notify users
		isCurrentK8sVersionSupported, warningMessage := c.validateCurrentK8sVersion()

		// Check: If we have a valid K8s version
		// Unfortunately, it is not possible to verify tridentImage version at this stage,
		// until we are inside the installation code we cannot perform some of the checks.
		// This only identifies changes in the K8s version
		var shouldUpdate bool
		if isCurrentK8sVersionSupported {
			shouldUpdate = c.updateNeeded(tridentK8sConfigVersion)
		}

		if shouldUpdate {
			// Update status of the tridentCR to `Updating`
			logMessage := "Updating Trident Provisioner CR before updating"
			statusMessage := "Updating Trident"

			c.eventRecorder.Event(controllingCR, corev1.EventTypeNormal, string(AppStatusUpdating), statusMessage)
			controllingCR, err = c.updateCRStatus(controllingCR, logMessage, statusMessage, string(AppStatusUpdating), currentInstalledTridentVersion)
			if err != nil {
				return utils.ReconcileFailedError(fmt.Errorf(
					"unable to update status of the CR '%v' in namespace '%v' to installing",
					controllingCR.Name, controllingCR.Namespace))
			}
		}

		if err := c.installTridentAndUpdateStatus(*controllingCR, currentInstalledTridentVersion, warningMessage,
			shouldUpdate); err != nil {
			// Install failed, so fail the reconcile loop
			return utils.ReconcileFailedError(fmt.Errorf("error re-installing Trident '%v' in namespace '%v'; err: %v",
				controllingCR.Name, controllingCR.Namespace, err))
		}
	}

	return nil
}

// updateAllCRs get called only when no ControllingCR exist to report a configuration error
func (c *Controller) updateAllCRs(message string) error {

	allCRs, err := c.getTridentCRsAll()
	if err != nil {
		return utils.ReconcileFailedError(fmt.Errorf(
			"unable to get list of all the TridentProvisioner CRs; err: %v", err))
	}

	// Update status on all the TridentProvisioner CR(s)
	logMessage := "Updating all the TridentProvisioner CRs."
	for _, cr := range allCRs {
		// Log error in the event recorder
		c.eventRecorder.Event(&cr, corev1.EventTypeWarning, string(AppStatusError), message)
		_, err = c.updateCRStatus(&cr, logMessage, message, string(AppStatusError), "")
	}

	return nil
}

// updateOtherCRs get called only when a ControllingCR exist to set error state on the non-ControllingCRs
func (c *Controller) updateOtherCRs(controllingCRName, controllingCRNamespace string) error {

	sameNamespaceCRs, err := c.getTridentCRsInNamespace(controllingCRNamespace)
	if err != nil {
		return utils.ReconcileFailedError(fmt.Errorf(
			"unable to get list of TridentProvisioner CRs in the same namespace '%v'; err: %v",
			controllingCRNamespace, err))
	}

	// Update status on all other TridentProvisioner CR(s) in the same namespace
	for _, cr := range sameNamespaceCRs {
		if cr.Name != controllingCRName {
			logMessage := "Updating other TridentProvisioner CRs in the same namespace."
			statusMessage := fmt.Sprintf("Trident is bound to another CR '%v' in the same namespace",
				controllingCRName)
			_, err = c.updateCRStatus(&cr, logMessage, statusMessage, string(AppStatusError), "")
		}
	}

	// Get list of all the CRs in different namespaces
	otherNamespaceCRs, err := c.getTridentCRsNotInNamespace(controllingCRNamespace)
	if err != nil {
		return utils.ReconcileFailedError(fmt.Errorf(
			"unable to get list of TridentProvisioner CRs in the namespace other than '%v'; err: %v",
			controllingCRNamespace, err))
	}

	// Update status on all other TridentProvisioner CR(s) in different namespace(s)
	for _, cr := range otherNamespaceCRs {
		logMessage := "Updating other TridentProvisioner CRs in the different namespace."
		statusMessage := fmt.Sprintf("Trident is bound to another CR '%v' in a different namespace '%v'",
			controllingCRName, controllingCRNamespace)
		_, err = c.updateCRStatus(&cr, logMessage, statusMessage, string(AppStatusError), "")
	}

	return nil
}

// updateNeeded compares the K8's version as per which Trident is installed with the current K8s version,
// if it has changed we need to update Trident as well
func (c *Controller) updateNeeded(tridentK8sConfigVersion string) bool {

	var shouldUpdate bool

	if tridentK8sConfigVersion == "" {
		return shouldUpdate
	}

	currentTridentConfigK8sVersion := utils.MustParseSemantic(tridentK8sConfigVersion).ToMajorMinorVersion()
	K8sVersion := utils.MustParseSemantic(c.K8SVersion.GitVersion).
		ToMajorMinorVersion()

	if currentTridentConfigK8sVersion.LessThan(K8sVersion) || currentTridentConfigK8sVersion.GreaterThan(K8sVersion) {
		log.Infof("Kubernetes version has changed from: %v to: %v; Trident operator"+
			" should change the Trident installation as per the new Kubernetes version",
			currentTridentConfigK8sVersion.String(), K8sVersion.String())

		shouldUpdate = true
	}

	return shouldUpdate
}

// validateCurrentK8sVersion identifies any changes in the K8s version, if it is valid, and if not valid should
// user be warned about it
func (c *Controller) validateCurrentK8sVersion() (bool, string) {

	var isValid bool
	var warning string

	currentK8sVersion, err := c.Clients.KubeClient.Discovery().ServerVersion()
	if err != nil {
		log.Errorf("Could not get Kubernetes version; unable to verify if update is required; err: %v", err)
		return isValid, ""
	} else if currentK8sVersion == nil {
		log.Errorf("Could not identify Kubernetes version; unable to verify if update is required; err: %v", err)
		return isValid, ""
	}

	if currentK8sVersion != c.K8SVersion {
		c.K8SVersion = currentK8sVersion
	}

	// Validate the Kubernetes server version
	if err := clients.ValidateKubernetesVersion(currentK8sVersion); err != nil {
		errMessage := fmt.Sprintf("Warning: Kubernetes version '%s' is unsupported; err: %v",
			currentK8sVersion.String(), err)
		log.Warnf(errMessage)
		warning = errMessage
	} else {
		log.Debugf("Kubernetes version '%s' is supported.", currentK8sVersion.String())
		isValid = true
	}

	return isValid, warning
}

// getCurrentTridentAndK8sVersion reports current Trident version installed and K8s version according
// to which Trident was installed
func (c *Controller) getCurrentTridentAndK8sVersion(tridentCR *netappv1.TridentProvisioner) (string, string, error) {
	var currentTridentVersionString string
	var currentK8sVersionString string

	i, err := installer.NewInstaller(c.KubeConfig, tridentCR.Namespace, installTimeout)
	if err != nil {
		return "", "", utils.ReconcileFailedError(err)
	}

	currentDeployment, _, _, err := i.TridentDeploymentInformation(installer.TridentCSILabel, true)
	if err != nil {
		return "", "", err
	}

	if currentDeployment != nil {
		currentTridentVersionString = currentDeployment.Labels[installer.TridentVersionLabelKey]
		currentK8sVersionString = currentDeployment.Labels[installer.K8sVersionLabelKey]
	} else {
		// For a case where deployment may have been deleted check for daemonset version also
		currentDaemonSet, _, _, err := i.TridentDaemonSetInformation()
		if err != nil {
			return "", "", err
		}

		if currentDaemonSet != nil {
			currentTridentVersionString = currentDaemonSet.Labels[installer.TridentVersionLabelKey]
			currentK8sVersionString = currentDaemonSet.Labels[installer.K8sVersionLabelKey]
		}
	}

	return currentTridentVersionString, currentK8sVersionString, nil
}

// installTridentAndUpdateStatus installs Trident and updates status of the ControllingCR accordingly
// based on sucess or failure
func (c *Controller) installTridentAndUpdateStatus(tridentCR netappv1.TridentProvisioner,
	currentInstalledTridentVersion, warningMessage string, shouldUpdate bool) error {

	var identifiedTridentVersion string

	// Install or Patch or Update Trident
	i, err := installer.NewInstaller(c.KubeConfig, tridentCR.Namespace, installTimeout)
	if err != nil {
		return utils.ReconcileFailedError(err)
	}
	if identifiedTridentVersion, err = i.InstallOrPatchTrident(tridentCR, currentInstalledTridentVersion, shouldUpdate); err != nil {
		// Update status of the tridentCR  to `Failed`
		logMessage := "Updating Trident Provisioner CR after failed installation."
		statusMessage := fmt.Sprintf("Failed to install Trident; err: %s", err.Error())

		if warningMessage != "" {
			statusMessage = statusMessage + "; " + warningMessage
		}

		// Log error in the event recorder and return error message
		c.eventRecorder.Event(&tridentCR, corev1.EventTypeWarning, string(AppStatusFailed), statusMessage)

		c.updateCRStatus(&tridentCR, logMessage, statusMessage, string(AppStatusFailed), currentInstalledTridentVersion)

		// Install failed, so fail the reconcile loop
		return utils.ReconcileFailedError(err)
	}

	// Update status of the tridentCR  to `Installed`
	logMessage := "Updating TridentProvisioner CR after installation."
	statusMessage := "Trident installed"

	if warningMessage != "" {
		statusMessage = statusMessage + "; " + warningMessage
	}

	// Log success in the event recorder and return success message
	c.eventRecorder.Event(&tridentCR, corev1.EventTypeNormal, string(AppStatusInstalled), statusMessage)

	_, err = c.updateCRStatus(&tridentCR, logMessage, statusMessage, string(AppStatusInstalled), identifiedTridentVersion)

	return err
}

// uninstallTridentAndUpdateStatus uninstalls Trident and updates status of the ControllingCR accordingly
// based on sucess or failure
func (c *Controller) uninstallTridentAndUpdateStatus(tridentCR netappv1.TridentProvisioner, currentInstalledTridentVersion string) (*netappv1.
	TridentProvisioner, error) {

	// Update status of the tridentCR  to `Uninstalling`
	logMessage := "Updating TridentProvisioner CR before uninstallation"
	statusMessage := "Uninstalling Trident"

	c.eventRecorder.Event(&tridentCR, corev1.EventTypeNormal, string(AppStatusUninstalling), statusMessage)
	newTridentCR, err := c.updateCRStatus(&tridentCR, logMessage, statusMessage, string(AppStatusUninstalling), currentInstalledTridentVersion)
	if err != nil {
		return nil, utils.ReconcileFailedError(fmt.Errorf(
			"unable to update status of CR '%v' in namespace '%v' to uninstalling", tridentCR.Name,
			tridentCR.Namespace))
	}

	// Uninstall Trident
	if err := c.uninstallTridentAll(tridentCR.Namespace); err != nil {
		// Update status of the tridentCR  to `Failed`
		logMessage := "Updating TridentProvisioner CR after failed uninstallation."
		statusMessage := fmt.Sprintf("Failed to uninstall Trident; err: %s", err.Error())

		// Log error in the event recorder and return error message
		c.eventRecorder.Event(newTridentCR, corev1.EventTypeWarning, string(AppStatusFailed), statusMessage)

		c.updateCRStatus(newTridentCR, logMessage, statusMessage, string(AppStatusFailed), currentInstalledTridentVersion)

		// Uninstall failed, so fail the reconcile loop
		return nil, utils.ReconcileFailedError(err)
	}

	log.Info("Trident installation removed.")

	var crdNote string
	if deletedCRDs, err := c.wipeout(tridentCR); err != nil {
		return &tridentCR, err
	} else if deletedCRDs {
		log.Info("Trident CRDs removed.")
		crdNote = " and removed CRDs"
	}

	// Update status of the tridentCR  to `Uninstalled`
	logMessage = "Updating TridentProvisioner CR after uninstallation."
	statusMessage = "Uninstalled Trident" + crdNote + UninstallationNote

	// Log successful uninstall in the event recorder and return success message
	c.eventRecorder.Event(newTridentCR, corev1.EventTypeNormal, string(AppStatusUninstalled), statusMessage)

	return c.updateCRStatus(newTridentCR, logMessage, statusMessage, string(AppStatusUninstalled), "")
}

// wipeout removes Trident object specifies in the wipeout list
func (c *Controller) wipeout(tridentCR netappv1.TridentProvisioner) (bool, error) {

	var deletedCRDs bool
	if len(tridentCR.Spec.Wipeout) > 0 {
		log.Infof("Wipeout list contains elements to be removed.")

		for _, itemToRemove := range tridentCR.Spec.Wipeout {
			switch strings.ToLower(itemToRemove) {
			case "crds":
				log.Info("Wipeout list contains CRDs; removing CRDs.")
				if err := c.obliviateCRDs(tridentCR); err != nil {
					return deletedCRDs, utils.ReconcileFailedError(fmt.Errorf(
						"error removing CRDs for the Trident installation controlled by the CR '%v' in namespace"+
							" '%v'; err: %v", tridentCR.Name, tridentCR.Namespace, err))
				}

				deletedCRDs = true
				log.Info("CRDs removed.")
			default:
				log.Warnf("Wipeout list contains an invalid entry: %s; no action required for this entry.",
					itemToRemove)
			}
		}
	}

	return deletedCRDs, nil
}

// uninstallTridentAll uninstalls Trident CSI, Trident CSI Preview, Trident Legacy
func (c *Controller) uninstallTridentAll(namespace string) error {
	i, err := installer.NewInstaller(c.KubeConfig, namespace, installTimeout)
	if err != nil {
		return err
	}

	if err := i.UninstallTrident(); err != nil {
		// Uninstall failed, so fail the reconcile loop
		return err
	}

	// Uninstall succeeded
	return nil
}

// uninstallCSIPreviewTrident uninstalls Trident CSI Preview
func (c *Controller) uninstallCSIPreviewTrident(namespace string) error {
	i, err := installer.NewInstaller(c.KubeConfig, namespace, installTimeout)
	if err != nil {
		return err
	}

	// Removes only the statefulset other object will be fixed or removed as part of the auto-heal code.
	if err := i.UninstallCSIPreviewTrident(); err != nil {
		// Uninstall failed, so fail the reconcile loop
		return err
	}

	// Uninstall succeeded
	return nil
}

// uninstallLegacyTrident uninstalls Trident CSI Legacy
func (c *Controller) uninstallLegacyTrident(namespace string) error {
	i, err := installer.NewInstaller(c.KubeConfig, namespace, installTimeout)
	if err != nil {
		return err
	}

	if err := i.UninstallLegacyTrident(); err != nil {
		// Uninstall failed, so fail the reconcile loop
		return err
	}

	// Uninstall succeeded
	return nil
}

// obliviateCRDs calls obliviate functionality, equivalent to:
// $ tridentctl obliviate crds
func (c *Controller) obliviateCRDs(tridentCR netappv1.TridentProvisioner) error {
	// Obliviate CRDs Trident
	i, err := installer.NewInstaller(c.KubeConfig, tridentCR.Namespace, installTimeout)
	if err != nil {
		return err
	}
	if err := i.ObliviateCRDs(); err != nil {
		return err
	}

	log.Info("CRDs removed.")

	return nil
}

// getTridentCRsAll gets all the TridentProvisioner CRs across all namespaces
func (c *Controller) getTridentCRsAll() ([]netappv1.TridentProvisioner, error) {
	list, err := c.CRDClient.TridentV1().TridentProvisioners(corev1.NamespaceAll).List(ctx(), listOpts)
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// getTridentCRsInNamespace gets all the TridentProvisioner CRs in the specified namespace
func (c *Controller) getTridentCRsInNamespace(namespace string) ([]netappv1.TridentProvisioner, error) {
	list, err := c.CRDClient.TridentV1().TridentProvisioners(namespace).List(ctx(), listOpts)
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// getTridentCRsNotInNamespace gets all the TridentProvisioner CRs in the namespace other than the one specified
func (c *Controller) getTridentCRsNotInNamespace(namespace string) ([]netappv1.TridentProvisioner, error) {
	list, err := c.CRDClient.TridentV1().TridentProvisioners(corev1.NamespaceAll).List(ctx(), listOpts)
	if err != nil {
		return nil, err
	}

	otherCRs := make([]netappv1.TridentProvisioner, 0)
	for _, cr := range list.Items {
		if cr.Namespace != namespace {
			otherCRs = append(otherCRs, cr)
		}
	}
	return otherCRs, nil
}

// isLegacyTridentInstalled identifies if legacy Trident is installed
func (c *Controller) isLegacyTridentInstalled() (installed bool, namespace string, err error) {
	return c.K8SClient.CheckDeploymentExistsByLabel(installer.TridentLegacyLabel, true)
}

// isPreviewCSITridentInstalled identifies if CSI Preview Trident is installed
func (c *Controller) isPreviewCSITridentInstalled() (installed bool, namespace string, err error) {
	return c.K8SClient.CheckStatefulSetExistsByLabel(installer.TridentCSILabel, true)
}

// isCSITridentInstalled identifies if CSI Trident is installed
func (c *Controller) isCSITridentInstalled() (installed bool, namespace string, err error) {
	return c.K8SClient.CheckDeploymentExistsByLabel(installer.TridentCSILabel, true)
}

// isCSITridentInstalled identifies if CSI Trident deployments created by Trident Operator exists
func (c *Controller) operatorCSIDeployments() ([]appsv1.Deployment, string, error) {
	var operatorBasedTridentCSIDeployments []appsv1.Deployment
	var operatorBasedTridentCSIExist bool
	var operatorBasedTridentCSINamespace string
	var returnErr error

	deploymentLabel := installer.TridentCSILabel
	allNamespaces := true

	if deployments, err := c.K8SClient.GetDeploymentsByLabel(installer.TridentCSILabel, allNamespaces); err != nil {
		log.Errorf("Unable to get list of deployments by label %v", deploymentLabel)
		returnErr = fmt.Errorf("unable to get list of deployments; err: %v", err)
	} else if len(deployments) == 0 {
		log.Info("Trident deployment not found.")
	} else {
		for _, deployment := range deployments {
			if deployment.OwnerReferences != nil && strings.ToLower(deployment.OwnerReferences[0].
				Kind) == "tridentprovisioner" {
				// Found an operator based Trident CSI deployment
				log.Infof("An operator based Trident CSI deployment named '%s' was found in the namespace '%s'.",
					deployment.Name, deployment.Namespace)

				if operatorBasedTridentCSIExist {
					// Not the first time encountering Operator based CSI Trident, hopefully this is never the case.
					operatorBasedTridentCSINamespace = "<multiple>"
				} else {
					operatorBasedTridentCSINamespace = deployment.Namespace
				}

				operatorBasedTridentCSIDeployments = append(operatorBasedTridentCSIDeployments, deployment)
				operatorBasedTridentCSIExist = true

				break
			}
		}
	}
	return operatorBasedTridentCSIDeployments, operatorBasedTridentCSINamespace, returnErr
}

// getControllingCRForTridentDeployments identifies controllingCR for deployment and reports nil if length of the
// operatorCSIDeployments is more than 1
func (c *Controller) getControllingCRForTridentDeployments(operatorCSIDeployments []appsv1.Deployment) (*netappv1.TridentProvisioner,
	error) {

	// If multiple Trident deployments are found, we will let self-heal logic fix it
	if len(operatorCSIDeployments) > 1 {
		log.Debugf("Found multiple Trident deployments.")
		return nil, nil
	}

	operatorCSIDeployment := operatorCSIDeployments[0]

	// Look for CRs in the deployment's namespace
	tridentCRs, err := c.getTridentCRsInNamespace(operatorCSIDeployment.Namespace)
	if err != nil {
		return nil, utils.ReconcileFailedError(err)
	} else if tridentCRs == nil {
		return nil, utils.ReconcileFailedError(errors.New("nil list of Trident custom resources"))
	}

	// Check if the number of CRs in the Trident installation namespace is zero
	if len(tridentCRs) == 0 {
		log.Debugf("No CR found in the Trident deployment namespace.")
		return nil, nil
	}

	log.Debug("Identifying controlling CRs from the list of CRs found in Trident installation namespace.")

	// Get CR that controls current Trident deployment
	deploymentCR, err := c.matchDeploymentControllingCR(tridentCRs, operatorCSIDeployment)
	if err != nil {
		return nil, err
	} else if deploymentCR == nil {
		log.Debugf("No CR found that controls the Trident deployment '%v' in namespace '%v'",
			operatorCSIDeployment.Name, operatorCSIDeployment.Namespace)
		return nil, nil
	}

	return deploymentCR, nil
}

// matchDeploymentControllingCR identified the controllingCR for an operator based deployment from the list of CRs
func (c *Controller) matchDeploymentControllingCR(tridentCRs []netappv1.TridentProvisioner,
	operatorCSIDeployment appsv1.Deployment) (*netappv1.TridentProvisioner, error) {

	// Identify Trident CR that controls the deployment
	var controllingCR *netappv1.TridentProvisioner
	for _, cr := range tridentCRs {
		if metav1.IsControlledBy(&operatorCSIDeployment, &cr) {
			controllingCR = &cr
			log.WithFields(log.Fields{
				"name":      cr.Name,
				"namespace": cr.Namespace,
			}).Info("Found CR that controls current Trident deployment.")

			break
		}
	}

	return controllingCR, nil
}

// identifyControllingCRBasedOnStatus identified the controllingCR purely on status and independent of any deployment
// logic involved
func (c *Controller) identifyControllingCRBasedOnStatus() (bool, *netappv1.TridentProvisioner, error) {

	// Get all TridentProvisioner CRs
	tridentCRs, err := c.getTridentCRsInNamespace(corev1.NamespaceAll)
	if err != nil {
		return false, nil, err
	} else if len(tridentCRs) == 0 {
		log.Info("Reconciler found no TridentProvisioner CRs.")
		return false, nil, nil
	}

	// Identify and return the CR that has status neither "NotInstalled" not "Error"
	for _, cr := range tridentCRs {
		if cr.Status.Status == string(AppStatusNotInstalled) || cr.Status.Status == string(AppStatusError) {
			continue
		}

		return true, &cr, nil
	}

	return false, nil, nil
}

// updateCRStatus updates the status of a CR if required
func (c *Controller) updateCRStatus(
	tridentCR *netappv1.TridentProvisioner, logMessage, message, status, version string,
) (*netappv1.TridentProvisioner, error) {

	// Update status of the tridentCR
	log.WithFields(log.Fields{
		"name":      tridentCR.Name,
		"namespace": tridentCR.Namespace,
	}).Debug(logMessage)

	newStatusDetails := netappv1.TridentProvisionerStatus{
		Message: message,
		Status:  status,
		Version: version,
	}

	if reflect.DeepEqual(tridentCR.Status, newStatusDetails) {
		log.WithFields(log.Fields{
			"name":      tridentCR.Name,
			"namespace": tridentCR.Namespace,
		}).Info("New status is same as the old status, no update needed.")

		return tridentCR, nil
	}

	prClone := tridentCR.DeepCopy()
	prClone.Status = newStatusDetails

	newTridentCR, err := c.CRDClient.TridentV1().TridentProvisioners(prClone.Namespace).UpdateStatus(
		ctx(), prClone, updateOpts)
	if err != nil {
		log.WithFields(log.Fields{
			"name":      tridentCR.Name,
			"namespace": tridentCR.Namespace,
		}).Errorf("could not update status of the CR; err: %v", err)
	} else {
		// Setting explicitly as this is a Client-go bug, fixed in the newest version of client-go
		newTridentCR.APIVersion = tridentCR.APIVersion
		newTridentCR.Kind = tridentCR.Kind
	}

	return newTridentCR, err
}

// resourceTypeToK8sKind translates resources to corresponding native Kinds
func (c *Controller) resourceTypeToK8sKind(resourceType ResourceType) string {
	switch resourceType {
	case ResourceTridentProvisionerCR:
		return "Trident Provisioner CR"
	case ResourceDeployment:
		return "deployment"
	case ResourceDaemonSet:
		return "daemonset"
	default:
		return "invalid object"
	}
}
