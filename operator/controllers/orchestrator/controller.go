// Copyright 2021 NetApp, Inc. All Rights Reserved.

package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	commonconfig "github.com/netapp/trident/config"
	"github.com/netapp/trident/operator/clients"
	netappv1 "github.com/netapp/trident/operator/controllers/orchestrator/apis/netapp/v1"
	"github.com/netapp/trident/operator/controllers/orchestrator/client/clientset/versioned/scheme"
	"github.com/netapp/trident/operator/controllers/orchestrator/installer"
	tprovv1 "github.com/netapp/trident/operator/controllers/provisioner/apis/netapp/v1"
	"github.com/netapp/trident/utils"
)

type (
	AppStatus    string
	ResourceType string // If Operator starts to List and Watch other CR types, this can be used to differentiate.
)

const (
	ControllerName    = "Trident Orchestrator"
	ControllerVersion = "0.1"
	CRDName           = "TridentOrchestrator"
	Operator          = "trident-operator.netapp.io"
	CacheSyncPeriod   = 300 * time.Second

	AppStatusNotInstalled AppStatus = ""             // default
	AppStatusInstalling   AppStatus = "Installing"   // Set only on controlling CR
	AppStatusInstalled    AppStatus = "Installed"    // Set only on controlling CR
	AppStatusUninstalling AppStatus = "Uninstalling" // Set only on controlling CR
	AppStatusUninstalled  AppStatus = "Uninstalled"  // Set only on controlling CR
	AppStatusFailed       AppStatus = "Failed"       // Set only on controlling CR
	AppStatusUpdating     AppStatus = "Updating"     // Set only on controlling CR
	AppStatusError        AppStatus = "Error"        // Should not be set on controlling CR

	ResourceTridentOrchestratorCR ResourceType = "resourceTridentOrchestratorCR"
	ResourceDeployment            ResourceType = "resourceDeployment"
	ResourceDaemonSet             ResourceType = "resourceDaemonset"

	TridentOrchestratorCRDName = "tridentorchestrators.trident.netapp.io"
	TridentProvisionersCRDName = "tridentprovisioners.trident.netapp.io"

	UninstallationNote = ". NOTE: This CR has uninstalled status; delete this CR to allow new Trident installation."

	K8sVersionCheckSupportWarning = "Warning: Trident is running on an unsupported version of Kubernetes; %s. " +
		"NetApp will not take Support calls or open Support tickets when using Trident with an unsupported version " +
		"of Kubernetes."
)

var (
	createOpts = metav1.CreateOptions{}
	deleteOpts = metav1.DeleteOptions{}
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
	log.WithField("Controller", ControllerName).Info("Initializing controller.")

	c := &Controller{
		Clients:   clients,
		mutex:     &sync.Mutex{},
		stopChan:  make(chan struct{}),
		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "TridentOrchestrator"),
	}

	// Set up event broadcaster
	utilruntime.Must(scheme.AddToScheme(scheme.Scheme))
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&clientv1.EventSinkImpl{Interface: c.KubeClient.CoreV1().Events("")})
	c.eventRecorder = broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: Operator})

	// Set up a watch for TridentOrchestrator CRs
	c.watcherCR = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return c.CRDClient.TridentV1().TridentOrchestrators().List(ctx(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return c.CRDClient.TridentV1().TridentOrchestrators().Watch(ctx(), options)
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
		&netappv1.TridentOrchestrator{},
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

	// Add handlers for TridentOrchestrator CRs
	c.informerCR.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.addOrchestrator,
			UpdateFunc: c.updateOrchestrator,
			DeleteFunc: c.deleteOrchestrator,
		},
	)

	// Add handlers for TridentOrchestrator CRs
	c.deploymentInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.deploymentAddedOrDeleted,
			UpdateFunc: c.deploymentUpdated,
			DeleteFunc: c.deploymentAddedOrDeleted,
		},
	)

	// Add handlers for TridentOrchestrator CRs
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
	log.WithField("Controller", ControllerName).Infof("Activating controller.")

	// The reason we have this here is to ensure that by the time Trident Orchestrator's List and Watcher
	// start they do not throw any error for unable to list/watch this CRD.
	if err := c.ensureTridentOrchestratorCRDExist(); err != nil {
		log.WithField("err", err).Warnf("Unable to ensure TridentOrchestrator exist.")
	}

	go c.informerCR.Run(c.stopChan)
	go c.deploymentInformer.Run(c.stopChan)
	go c.daemonsetInformer.Run(c.stopChan)

	log.Info("Starting workers")
	go wait.Until(c.runWorker, time.Second, c.stopChan)

	log.Info("Started workers")

	return nil
}

func (c *Controller) Deactivate() error {
	log.WithField("Controller", ControllerName).Infof("Deactivating controller.")

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

// addOrchestrator is the add handler for the TridentOrchestrator watcher.
func (c *Controller) addOrchestrator(obj interface{}) {
	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		log.Error(err)
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: %s", key)
		return
	}

	log.WithFields(log.Fields{
		"CR":  name,
		"CRD": CRDName,
	}).Infof("CR added.")

	keyItem := KeyItem{
		keyDetails:   key,
		resourceType: ResourceTridentOrchestratorCR,
	}

	c.workqueue.Add(keyItem)
}

// updateOrchestrator is the update handler for the TridentOrchestrator watcher.
func (c *Controller) updateOrchestrator(oldObj, newObj interface{}) {
	_, ok := oldObj.(*netappv1.TridentOrchestrator)
	if !ok {
		log.Errorf("'%s' controller expected '%s' CR; got '%v'", ControllerName, CRDName, oldObj)
		return
	}

	newCR, ok := newObj.(*netappv1.TridentOrchestrator)
	if !ok {
		log.Errorf("'%s' controller expected '%s' CR; got '%v'", ControllerName, CRDName, newObj)
		return
	}

	if !newCR.ObjectMeta.DeletionTimestamp.IsZero() {
		log.WithFields(log.Fields{
			"name":              newCR.Name,
			"deletionTimestamp": newCR.ObjectMeta.DeletionTimestamp,
		}).Infof("'%s' CR is being deleted, not updated.", CRDName)
		return
	}

	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(newObj); err != nil {
		log.Error(err)
		return
	}

	log.WithFields(log.Fields{
		"CR":  newCR.Name,
		"CRD": CRDName,
	}).Infof("CR updated.")

	keyItem := KeyItem{
		keyDetails:   key,
		resourceType: ResourceTridentOrchestratorCR,
	}

	c.workqueue.Add(keyItem)
}

// deleteOrchestrator is the delete handler for the TridentOrchestrator watcher.
func (c *Controller) deleteOrchestrator(obj interface{}) {
	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		log.Error(err)
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		log.Errorf("invalid resource key: '%s'", key)
		return
	}

	log.WithFields(log.Fields{
		"CR":  name,
		"CRD": CRDName,
	}).Infof("CR deleted.")

	keyItem := KeyItem{
		keyDetails:   key,
		resourceType: ResourceTridentOrchestratorCR,
	}

	c.workqueue.Add(keyItem)
}

// deploymentAddedOrDeleted is a handler for the trident-csi deployment watcher.
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
	}).Infof("Trident deployment added or deleted.")

	keyItem := KeyItem{
		keyDetails:   key,
		resourceType: ResourceDeployment,
	}

	c.workqueue.Add(keyItem)
}

// deploymentUpdated is the handler for the trident-csi deployment watcher.
func (c *Controller) deploymentUpdated(oldObj, newObj interface{}) {
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
	}).Infof("Trident deployment changed.")

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
	}).Infof("Trident daemonset added or deleted.")

	keyItem := KeyItem{
		keyDetails:   key,
		resourceType: ResourceDaemonSet,
	}

	c.workqueue.Add(keyItem)
}

// daemonsetUpdated is the handler for the trident-csi daemonset watcher.
func (c *Controller) daemonsetUpdated(_, newObj interface{}) {
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
	}).Infof("Trident daemonset changed.")

	keyItem := KeyItem{
		keyDetails:   key,
		resourceType: ResourceDaemonSet,
	}

	c.workqueue.Add(keyItem)
}

/********************************************
 * Checks pre- and post- Trident installation
 ********************************************/
// unsupportedInstallationsPrechecks identifies the state of the Trident installation(s)
// 1. If we are in a Greenfield scenario and come across legacy, CSI Preview or tridentctl based
//    Trident installations we send a configuration error.
// 2. However, if legacy, CSI Preview or tridentctl based Trident are installed after installing the
//    operator, these will be removed eventually.
func (c *Controller) unsupportedInstallationsPrechecks(controllingCRBasedOnStatusExists,
	operatorCSIDeploymentFound bool,
) error {
	// CSI Preview Trident should not be present
	if csiPreviewTridentInstalled, CSIPreviewNamespace, err := c.isPreviewCSITridentInstalled(); err != nil {
		return utils.ReconcileFailedError(err)
	} else if csiPreviewTridentInstalled {
		if controllingCRBasedOnStatusExists || operatorCSIDeploymentFound {
			log.Debug("Identified that CSI Preview Trident was installed after Trident Operator created installation.")
			if err := c.uninstallCSIPreviewTrident(CSIPreviewNamespace); err != nil {
				log.WithField("err", err).Error("Unable to remove CSI Preview Trident.")
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
			if crErr := c.updateAllCRs(errorMessage); crErr != nil {
				log.Error(crErr)
			}
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
				log.WithField("err", err).Error("Unable to remove legacy Trident.")
				return utils.ReconcileFailedError(err)
			} else {
				log.Debug("Removed legacy Trident.")
				return utils.ReconcileIncompleteError()
			}
		} else {
			errorMessage := fmt.Sprintf("Operator cannot proceed with the installation, "+
				"found non-CSI Trident already installed in namespace '%v'.", legacyTridentNamespace)
			log.Error(errorMessage)
			if crErr := c.updateAllCRs(errorMessage); crErr != nil {
				log.Error(crErr)
			}
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
		if crErr := c.updateAllCRs(errorMessage); crErr != nil {
			log.Error(crErr)
		}
		return utils.UnsupportedConfigError(fmt.Errorf(errorMessage))
	}

	return nil
}

// alphaSnapshotCRDsPostinstallationCheck identifies if the alpha snapshot CRDs are present after any Trident
// installation by the operator, this should only be called once controllingCR has been identified and before
// installing, patching, updating the Trident
func (c *Controller) alphaSnapshotCRDsPostinstallationCheck(tridentCR *netappv1.TridentOrchestrator,
	currentInstalledTridentVersion string,
) error {
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
		debugMessage := "Updating Trident Orchestrator CR after failed alpha snapshot CRDs check."

		if _, crErr := c.updateTorcEventAndStatus(tridentCR, debugMessage, errorMessage, string(AppStatusFailed),
			currentInstalledTridentVersion, tridentCR.Status.Namespace, corev1.EventTypeWarning,
			&tridentCR.Status.CurrentInstallationParams); crErr != nil {
			log.Error(crErr)
		}
		// Alpha snapshot CRDs check failed, so fail the reconcile loop
		return utils.ReconcileFailedError(fmt.Errorf(errorMessage))
	}

	return nil
}

// k8sVersionPreinstallationCheck identifies if K8s version is valid or not
func (c *Controller) k8sVersionPreinstallationCheck() error {
	isCurrentK8sVersionValid, warningMessage := c.validateCurrentK8sVersion()

	if !isCurrentK8sVersionValid {
		if crErr := c.updateAllCRs(warningMessage); crErr != nil {
			log.Error(crErr)
		}
	}

	return nil
}

/*****************
 * Migration Logic
 *****************/
// handleMigrationScenario identifies if there is a valid Tprov CR with `Installed` status and
// if true then operator copies its config to a new Torc CR.
func (c *Controller) handleMigrationScenario() error {
	log.Debug("Verify migration from TridentProvisioner to TridentOrchestrator required.")

	// Add this sleep to ensure if both thee operator bundle and the Tprov CR were updated simultaneously,
	// we discover and consider the latest Tprov's spec for the migration.
	time.Sleep(5 * time.Second)

	// Check if there already exists a controllingCR - if deployment is deleted it is possible that we may run
	// into a situation where there is no deployment but there is a controlling CR
	controllingTprovWithInstalledStatus, err := c.identifyTprovCRWithInstalledStatus()
	if err != nil {
		return utils.ReconcileFailedError(fmt.Errorf(
			"unable to identify if TridentProvisioner CR with Installed status exists; err: %v", err))
	}

	// If Tprov with "Installed" status exist
	if controllingTprovWithInstalledStatus != nil {
		name := controllingTprovWithInstalledStatus.Name
		spec := controllingTprovWithInstalledStatus.Spec
		namespace := controllingTprovWithInstalledStatus.Namespace

		log.WithFields(log.Fields{
			"TridentProvisionerCR":          name,
			"TridentProvisionerCRNamespace": namespace,
			"TridentOrchestratorCR":         name,
		}).Infof("Found a TridentProvisioner CR with Installed status; migrating information to TridentOrchestrator CR")

		specJson, err := json.Marshal(spec)
		if err != nil {
			msg := fmt.Sprintf("unable to convert existing TridentProvisioner(%v/%v) spec to JSON; err: %v", name,
				namespace, err)
			log.Errorf(msg)
			return utils.ReconcileFailedError(fmt.Errorf(msg))
		}

		var tridentOrchestratorSpec netappv1.TridentOrchestratorSpec
		err = json.Unmarshal(specJson, &tridentOrchestratorSpec)
		if err != nil {
			msg := fmt.Sprintf("unable to convert existing TridentProvisioner("+
				"%v/%v) spec to TridentOrchestrator spec; err: %v", name, namespace, err)
			log.Errorf(msg)
			return utils.ReconcileFailedError(fmt.Errorf(msg))
		}

		tridentOrchestratorSpec.Namespace = namespace

		torc := &netappv1.TridentOrchestrator{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "trident.netapp.io/v1",
				Kind:       "TridentOrchestrator",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: tridentOrchestratorSpec,
		}

		if err = c.createTridentOrchestratorCR(torc); err != nil {
			msg := fmt.Sprintf("unable to create existing TridentOrchestrator(%v) from TridentProvisioner("+
				"%v/%v) spec; err: %v", name, namespace, name, err)
			log.Errorf(msg)
			return utils.ReconcileFailedError(fmt.Errorf(msg))
		}

		log.WithField("TridentOrchestratorCR", name).Info("Created TridentOrchestrator CR.")

		if err = c.deleteTridentTprovCR(name, namespace); err != nil {
			msg := fmt.Sprintf("unable to delete existing TridentProvisioner(%v/%v) CR; err: %v", namespace, name, err)
			log.Errorf(msg)
			return utils.ReconcileFailedError(fmt.Errorf(msg))
		}

		log.WithFields(log.Fields{
			"TridentProvisionerCR":          name,
			"TridentProvisionerCRNamespace": namespace,
		}).Infof("Removed TridentProvisioner CR.")
		return utils.ReconcileIncompleteError()
	}

	log.Debug("Migration from TridentProvisioner to TridentOrchestrator not required.")

	return nil
}

/**********************
 * Reconciliation Logic
 **********************/
// reconcile runs the reconcile logic and ensures we move to the desired state and the desired state is
// maintained
func (c *Controller) reconcile(key KeyItem) error {
	// Check if there already exists a controllingCR - if deployment is deleted it is possible that we may run
	// into a situation where there is no deployment but there is a controlling CR
	controllingCRBasedOnStatusExists, controllingCRBasedOnStatus, err := c.identifyControllingCRBasedOnStatus()
	if err != nil {
		return utils.ReconcileFailedError(fmt.Errorf("unable to identify if controlling CR exists; err: %v", err))
	}

	// Check if Trident Orchestrator based CSI Trident installed
	var torcCSIDeploymentFound bool
	torcCSIDeployments, torcCSIDeploymentNamespace, err := c.getTridentOrchestratorCSIDeployments()
	if err != nil {
		return utils.ReconcileFailedError(fmt.Errorf(
			"unable to identify if Operator based CSI Trident installation(s) exist; err: %v", err))
	} else if len(torcCSIDeployments) > 0 {
		log.WithField("deploymentNamespace",
			torcCSIDeploymentNamespace).Debug("Found atleast one CSI Trident deployment created by the operator")
		torcCSIDeploymentFound = true
	} else {
		log.Debugf("No operator based CSI Trident deployment found.")
	}

	if err := c.unsupportedInstallationsPrechecks(controllingCRBasedOnStatusExists,
		torcCSIDeploymentFound); err != nil {
		return err
	}

	if torcCSIDeploymentFound || controllingCRBasedOnStatusExists {

		// Always delete the TridentProvisioner CRDs and CRs
		if err := c.cleanupTridentProvisioner(); err != nil {
			log.WithField("err", err).Warnf("Unable to clean TridentProvisioner objects.")
		}

		return c.reconcileTridentPresent(key, torcCSIDeployments, torcCSIDeploymentNamespace, true,
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

		// Handle the migration scenario from tprov to torc (if applicable)
		if exists, err := c.doesCRDExist(TridentProvisionersCRDName); err == nil && exists {
			if err := c.handleMigrationScenario(); err != nil {
				return err
			}
		}

		// Always delete the TridentProvisioner CRDs and CRs
		if err := c.cleanupTridentProvisioner(); err != nil {
			log.WithField("err", err).Warnf("Unable to clean TridentProvisioner objects")
		}

		return c.reconcileTridentNotPresent()
	}
}

func (c *Controller) reconcileTridentNotPresent() error {
	log.Info("Reconciler found no operator-based Trident installation.")

	// Get all TridentOrchestrator CRs
	tridentCRs, err := c.getTridentOrchestratorCRsAll()
	if err != nil {
		return err
	} else if len(tridentCRs) == 0 {
		log.Info("Reconciler found no TridentOrchestrator CRs, nothing to do.")
		return nil
	}

	// Iterate through all the CRs, identify if any of the CRs has status "Uninstalled" then return,
	// until this CR is removed cannot perform new Trident installation.
	for _, cr := range tridentCRs {
		if cr.Status.Status == string(AppStatusUninstalled) {
			log.WithField("controllingCR",
				cr.Name).Warnf("Remove TridentOrchestrator CR with uninstalled status to allow new Trident installation.")

			return nil
		}
	}

	// Iterate through all the CRs, and follow this preference logic:
	// 1. If no CR has Uninstalled status, then identify CR that has Uninstall not set to true
	// 2. Prefer a CR with .Spec.Namespace same as the Operator namespace
	var tridentCR *netappv1.TridentOrchestrator
	for _, cr := range tridentCRs {
		if !cr.Spec.Uninstall {
			tridentCR = cr
			if cr.Spec.Namespace == c.Namespace {
				break
			}
		}
	}

	if tridentCR == nil {
		log.Warnf("Reconciler found no valid TridentOrchestrator CRs, nothing to do.")
		return nil
	}

	if tridentCR.Spec.Namespace == "" {
		tridentCR.Spec.Namespace = metav1.NamespaceDefault
	}

	// Update status of the tridentCR to `Installing`
	debugMessage := "Updating TridentOrchestrator CR before new installation"
	statusMessage := "Installing Trident"

	newTridentCR, err := c.updateTorcEventAndStatus(tridentCR, debugMessage, statusMessage,
		string(AppStatusInstalling), "", tridentCR.Spec.Namespace, corev1.EventTypeNormal, nil)
	if err != nil {
		return utils.ReconcileFailedError(fmt.Errorf(
			"unable to update status of the CR '%v' to installing", tridentCR.Name))
	}

	// At this stage we have identified a valid controlling CR for Operator-based Trident installation,
	// now we should also check if any Tprov-based or tridentctl-based CSI Trident installation exists,
	// if one does exists we can safely remove it to make way for the new Operator-based Trident installation.
	//
	// NOTE: If this method is placed before this point, then we can run into a risk where existing
	// tridentctl-based installation would get removed and may not be replaced with an Operator-based
	// Trident installation.
	if err := c.removeNonTorcBasedCSIInstallation(newTridentCR); err != nil {
		return err
	}

	if newTridentCR.Spec.Namespace == "" {
		newTridentCR.Spec.Namespace = metav1.NamespaceDefault
	}

	if err := c.installTridentAndUpdateStatus(*newTridentCR, "", "", false); err != nil {
		// Install failed, so fail the reconcile loop
		return utils.ReconcileFailedError(fmt.Errorf(
			"error installing Trident using CR '%v' in namespace '%v'; err: %v",
			newTridentCR.Name, tridentCR.Spec.Namespace, err))
	}

	err = c.updateOtherCRs(newTridentCR.Name)

	return err
}

func (c *Controller) reconcileTridentPresent(key KeyItem, operatorCSIDeployments []appsv1.Deployment,
	deploymentNamespace string, isCSI bool,
	controllingCRBasedOnStatus *netappv1.TridentOrchestrator,
) error {
	var controllingCRBasedOnStatusName string
	if controllingCRBasedOnStatus != nil {
		controllingCRBasedOnStatusName = controllingCRBasedOnStatus.Name
	}

	// Convert the key CR's namespace/name string into a distinct namespace and name
	_, callingCRName, err := cache.SplitMetaNamespaceKey(key.keyDetails)
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
		"callingCRName":               callingCRName,
		"callingResourceType":         callingResourceType,
		"namespace":                   deploymentNamespace,
		"operatorBasedCSIDeployments": operatorCSIDeploymentNames,
		"isCSI":                       isCSI,
		"controllingCRBasedOnStatus":  controllingCRBasedOnStatusName,
	}).Info("Reconciler found Trident installation.")

	// Identify current Trident installation CR if deployment already exist
	var controllingCRBasedOnInstall *netappv1.TridentOrchestrator
	operatorCSIDeploymentsExist := len(operatorCSIDeployments) > 0
	if operatorCSIDeploymentsExist {
		controllingCRBasedOnInstall, err = c.identifyControllingCRForTridentDeployments(operatorCSIDeployments)
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

	var controllingCR *netappv1.TridentOrchestrator

	if controllingCRBasedOnStatus != nil {
		if controllingCRBasedOnInstall != nil {
			if controllingCRBasedOnInstall.Name != controllingCRBasedOnStatus.Name {
				// We should not be in this situation
				// This cannot be set in an event log as we are not clear which of the CR is a controllingCR
				return utils.ReconcileFailedError(fmt.Errorf("current Trident installation CR "+
					"'%v' and identified controlling CR '%v' are different", controllingCRBasedOnInstall.Name, controllingCRBasedOnStatus.Name))
			}
		}

		controllingCR = controllingCRBasedOnStatus
	} else {
		if controllingCRBasedOnInstall == nil {
			// We should not be in this situation, removing all the Trident deployments and other remnants
			if err := c.uninstallTridentAll(metav1.NamespaceDefault); err != nil {
				return utils.ReconcileFailedError(err)
			}
			// Uninstall succeeded, so re-run the reconcile loop
			return utils.ReconcileIncompleteError()
		}

		controllingCR = controllingCRBasedOnInstall
	}

	log.WithField("controllingCR", controllingCR.Name).Debugf("Controlling CR identified.")

	// Ensure Controlling CR has namespace field set
	if controllingCR.Spec.Namespace == "" {
		controllingCR.Spec.Namespace = metav1.NamespaceDefault
	}

	// Perform controllingCRBasedReconcile i.e install/patch/update/uninstall only if:
	// 1. callingResourceType IS NOT ResourceTridentOrchestratorCR, to allow changes to deployment/daemonset to trigger reconcile
	// 2. callingResourceType IS ResourceTridentOrchestratorCR, then callingCR should be same as controllingCR
	if callingResourceType != ResourceTridentOrchestratorCR || (callingCRName == controllingCR.Name) {
		log.WithField("controllingCR", controllingCR.Name).Debug("Performing reconcile.")
		if err = c.controllingCRBasedReconcile(controllingCR, operatorCSIDeploymentsExist); err != nil {
			return err
		}
	} else {
		log.WithField("callingCR", callingCRName).Debugf(
			"Reconcile not initiated by a controlling CR, skipping reconcile.")
	}

	err = c.updateOtherCRs(controllingCR.Name)

	return err
}

// controllingCRBasedReconcile is the core reconciliation or in other words maintenance logic i.e.
// we know the ControllingCR, therefore we know the Specs of the ControllingCR, use that spec to
// ensure we are maintaining the desired state.
func (c *Controller) controllingCRBasedReconcile(controllingCR *netappv1.TridentOrchestrator,
	deploymentExist bool,
) error {
	// Check to see if controllingCR status is uninstalled, if this is the case installation/patch should not be run
	if controllingCR.Status.Status == string(AppStatusUninstalled) {

		// If for some reason deployment exists remove Trident installation to make sure Trident remains in
		// uninstalled state
		if deploymentExist {
			log.WithField("controllingCR", controllingCR.Name).Warnf("Even though controlling CR has status %v, "+
				"there exists Trident installation; re-running Trident uninstallation", controllingCR.Status.Status)
		}

		// This uninstallation would merely fix the state by removing deployment and match the status
		// Uninstalled, there is no need to update status after deployment is removed successfully.
		if err := c.uninstallTridentAll(controllingCR.Status.Namespace); err != nil {
			return utils.ReconcileFailedError(err)
		}

		var crdNote string
		if deletedCRDs, err := c.wipeout(*controllingCR); err != nil {
			return err
		} else if deletedCRDs {
			log.Info("Trident CRDs removed.")
			crdNote = " and removed CRDs"
		}

		debugMessage := "Updating TridentOrchestrator CR after uninstallation."
		statusMessage := "Uninstalled Trident" + crdNote + UninstallationNote

		if _, crErr := c.updateTorcEventAndStatus(controllingCR, debugMessage, statusMessage,
			string(AppStatusUninstalled), "", controllingCR.Status.Namespace, corev1.EventTypeNormal,
			nil); crErr != nil {
			log.Error(crErr)
		}
		log.WithField("TridentOrchestratorCR", controllingCR.Name).Warnf(
			"Remove TridentOrchestrator CR with uninstalled status to allow new Trident installation.")

		return c.updateOtherCRs(controllingCR.Name)
	}

	// Check to see if controllingCR spec has changed and is requesting for uninstallation
	uninstall := controllingCR.Spec.Uninstall

	// Get current Version information, to update CRs with the correct version information and in K8s case
	// identify if update might be required (for installation only) due to change in K8s version.
	currentInstalledTridentVersion, tridentK8sConfigVersion, err := c.getCurrentTridentAndK8sVersion(controllingCR)
	if err != nil {
		// Failed to identify current trident version and K8s version
		log.WithFields(log.Fields{
			"controllingCR": controllingCR.Name,
			"err":           err,
		}).Errorf("Error identifying update scenario.")

		// If the orchestrator CR spec has been issued for uninstallation, allow the attempted uninstall regardless
		if !uninstall {
			// Update the Trident ControllingCR with a Failed status and bail out of reconciliation if the there was an
			// error in the k8s_client, Trident resources couldn't be detected,
			// and the controlling CR isn't scheduled for uninstallation
			debugMessage := "Updating Trident Orchestrator CR after failing to detect Trident Deployment and/or DaemonSet."
			statusMessage := fmt.Sprintf("Failed to detect installed Trident resources; err: %s", err.Error())

			if _, crErr := c.updateTorcEventAndStatus(controllingCR, debugMessage, statusMessage, string(AppStatusFailed),
				controllingCR.Status.Version, controllingCR.Status.Namespace, corev1.EventTypeWarning,
				&controllingCR.Status.CurrentInstallationParams); crErr != nil {
				log.Error(crErr)
			}

			return utils.ReconcileFailedError(err)
		}
	}

	if uninstall {
		if _, err := c.uninstallTridentAndUpdateStatus(*controllingCR, currentInstalledTridentVersion); err != nil {
			// Install failed, so fail the reconcile loop
			return utils.ReconcileFailedError(fmt.Errorf(
				"error uninstalling Trident in namespace '%v', controlled by CR '%v'; err: %v",
				controllingCR.Status.Namespace, controllingCR.Name, err))
		}

		log.WithField("controllingCR", controllingCR.Name).Warnf(
			"Remove TridentOrchestrator CR with uninstalled status to allow new Trident installation.")
	} else {

		// There are certain checks that should be run before each install, update, patch

		// Check: Namespace change - not allowed
		currentInstallationNamespace := controllingCR.Status.Namespace
		if currentInstallationNamespace != "" && controllingCR.Spec.Namespace != currentInstallationNamespace {

			// Update status of the tridentCR  to `Failed`
			debugMessage := "Updating Trident Orchestrator CR after failed namespace check."
			errorMessage := fmt.Sprintf("Operator cannot proceed with the installation due to namespace change from"+
				" `%v` to `%v`; namespace change is not allowed.", currentInstallationNamespace,
				controllingCR.Spec.Namespace)

			if _, crErr := c.updateTorcEventAndStatus(controllingCR, debugMessage, errorMessage, string(AppStatusFailed),
				currentInstalledTridentVersion, currentInstallationNamespace, corev1.EventTypeWarning,
				&controllingCR.Status.CurrentInstallationParams); crErr != nil {
				log.Error(crErr)
			}

			return utils.ReconcileFailedError(fmt.Errorf(errorMessage))
		}

		// Check: Alpha-snapshot CRDs should not be present
		if err = c.alphaSnapshotCRDsPostinstallationCheck(controllingCR, currentInstalledTridentVersion); err != nil {
			return err
		}

		// Check: Current K8s version should be supported, if not is there a warning message to notify users
		isCurrentK8sVersionSupported, warningMessage := c.validateCurrentK8sVersion()
		eventType := corev1.EventTypeNormal
		if warningMessage != "" {
			eventType = corev1.EventTypeWarning
		}

		// Check: If we have a valid K8s version
		// Unfortunately, it is not possible to verify tridentImage version at this stage,
		// until we are inside the installation code we cannot perform some of the checks.
		// This only identifies changes in the K8s version
		// If we are skipping k8s version check, isCurrentK8sVersionSupported is irrelevant
		var shouldUpdate bool
		if isCurrentK8sVersionSupported {
			shouldUpdate = c.tridentUpgradeNeeded(tridentK8sConfigVersion)
		}

		if shouldUpdate {
			// Update status of the tridentCR to `Updating`
			debugMessage := "Updating Trident Orchestrator CR before updating"
			statusMessage := "Updating Trident"
			controllingCRName := controllingCR.Name

			controllingCR, err = c.updateTorcEventAndStatus(controllingCR, debugMessage, statusMessage,
				string(AppStatusUpdating), currentInstalledTridentVersion, currentInstallationNamespace,
				eventType, &controllingCR.Status.CurrentInstallationParams)
			if err != nil {
				return utils.ReconcileFailedError(fmt.Errorf(
					"unable to update status of the CR '%v' to installing", controllingCRName))
			}
		}

		if err := c.installTridentAndUpdateStatus(*controllingCR, currentInstalledTridentVersion, warningMessage,
			shouldUpdate); err != nil {
			// Install failed, so fail the reconcile loop
			return utils.ReconcileFailedError(fmt.Errorf("error re-installing Trident '%v' ; err: %v",
				controllingCR.Name, err))
		}
	}

	return nil
}

/************************************************
 * Installer & Uninstaller based helper functions
 ************************************************/
// installTridentAndUpdateStatus installs Trident and updates status of the ControllingCR accordingly
// based on success or failure
func (c *Controller) installTridentAndUpdateStatus(tridentCR netappv1.TridentOrchestrator,
	currentInstalledTridentVersion, warningMessage string, shouldUpdate bool,
) error {
	var identifiedTridentVersion string
	var identifiedSpecValues *netappv1.TridentOrchestratorSpecValues

	// Install or Patch or Update Trident
	i, err := installer.NewInstaller(c.KubeConfig, tridentCR.Spec.Namespace, tridentCR.Spec.K8sTimeout)
	if err != nil {
		return utils.ReconcileFailedError(err)
	}

	if identifiedSpecValues, identifiedTridentVersion, err = i.InstallOrPatchTrident(tridentCR, currentInstalledTridentVersion,
		shouldUpdate); err != nil {
		// Update status of the tridentCR  to `Failed`
		debugMessage := "Updating Trident Orchestrator CR after failed installation."
		statusMessage := fmt.Sprintf("Failed to install Trident; err: %s", err.Error())

		if warningMessage != "" {
			statusMessage = statusMessage + "; " + warningMessage
		}

		if _, crErr := c.updateTorcEventAndStatus(&tridentCR, debugMessage, statusMessage,
			string(AppStatusFailed), "", tridentCR.Spec.Namespace, corev1.EventTypeWarning,
			identifiedSpecValues); crErr != nil {
			log.Error(crErr)
		}

		// Install failed, so fail the reconcile loop
		return utils.ReconcileFailedError(err)
	}

	// Update status of the tridentCR  to `Installed`
	debugMessage := "Updating TridentOrchestrator CR after installation."
	statusMessage := "Trident installed"

	eventType := corev1.EventTypeNormal
	if warningMessage != "" {
		statusMessage = statusMessage + "; " + warningMessage
		eventType = corev1.EventTypeWarning
	}

	_, err = c.updateTorcEventAndStatus(&tridentCR, debugMessage, statusMessage, string(AppStatusInstalled),
		identifiedTridentVersion, tridentCR.Spec.Namespace, eventType, identifiedSpecValues)

	return err
}

// uninstallTridentAndUpdateStatus uninstalls Trident and updates status of the ControllingCR accordingly
// based on success or failure
func (c *Controller) uninstallTridentAndUpdateStatus(tridentCR netappv1.TridentOrchestrator,
	currentInstalledTridentVersion string) (*netappv1.
	TridentOrchestrator, error,
) {
	// Update status of the tridentCR  to `Uninstalling`
	debugMessage := "Updating TridentOrchestrator CR before uninstallation"
	statusMessage := "Uninstalling Trident"

	newTridentCR, err := c.updateTorcEventAndStatus(&tridentCR, debugMessage, statusMessage, string(AppStatusUninstalling),
		currentInstalledTridentVersion, tridentCR.Status.Namespace, corev1.EventTypeNormal,
		&tridentCR.Status.CurrentInstallationParams)
	if err != nil {
		return nil, utils.ReconcileFailedError(fmt.Errorf(
			"unable to update status of CR '%v' to uninstalling", tridentCR.Name))
	}

	// Uninstall Trident
	if err := c.uninstallTridentAll(tridentCR.Status.Namespace); err != nil {
		// Update status of the tridentCR  to `Failed`
		debugMessage := "Updating TridentOrchestrator CR after failed uninstallation."
		statusMessage := fmt.Sprintf("Failed to uninstall Trident; err: %s", err.Error())

		if _, crErr := c.updateTorcEventAndStatus(newTridentCR, debugMessage, statusMessage, string(AppStatusFailed),
			currentInstalledTridentVersion, tridentCR.Status.Namespace, corev1.EventTypeWarning,
			&tridentCR.Status.CurrentInstallationParams); crErr != nil {
			log.Error(crErr)
		}
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
	debugMessage = "Updating TridentOrchestrator CR after uninstallation."
	statusMessage = "Uninstalled Trident" + crdNote + UninstallationNote

	return c.updateTorcEventAndStatus(newTridentCR, debugMessage, statusMessage,
		string(AppStatusUninstalled), "", newTridentCR.Status.Namespace, corev1.EventTypeNormal, nil)
}

// uninstallTridentAll uninstalls Trident CSI, Trident CSI Preview, Trident Legacy
func (c *Controller) uninstallTridentAll(namespace string) error {
	i, err := installer.NewInstaller(c.KubeConfig, namespace, 0)
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

// wipeout removes Trident object specifies in the wipeout list
func (c *Controller) wipeout(tridentCR netappv1.TridentOrchestrator) (bool, error) {
	var deletedCRDs bool
	if len(tridentCR.Spec.Wipeout) > 0 {
		log.Infof("Wipeout list contains elements to be removed.")

		for _, itemToRemove := range tridentCR.Spec.Wipeout {
			switch strings.ToLower(itemToRemove) {
			case "crds":
				log.Info("Wipeout list contains CRDs; removing CRDs.")
				if err := c.obliviateCRDs(tridentCR); err != nil {
					return deletedCRDs, utils.ReconcileFailedError(fmt.Errorf(
						"error removing CRDs for the Trident installation in namespace '%v', controlled by the CR"+
							" '%v'; err: %v", tridentCR.Status.Namespace, tridentCR.Name, err))
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

// obliviateCRDs calls obliviate functionality, equivalent to:
// $ tridentctl obliviate crds
func (c *Controller) obliviateCRDs(tridentCR netappv1.TridentOrchestrator) error {
	// Obliviate CRDs Trident
	i, err := installer.NewInstaller(c.KubeConfig, tridentCR.Status.Namespace, tridentCR.Spec.K8sTimeout)
	if err != nil {
		return err
	}
	if err := i.ObliviateCRDs(); err != nil {
		return err
	}

	log.Info("CRDs removed.")

	return nil
}

/************************************************
 * Trident Orchestrator CRD & CR helper functions
 ************************************************/
// ensureTridentOrchestratorCRDExist ensures TridentOrchestrator CRD exist
func (c *Controller) ensureTridentOrchestratorCRDExist() error {
	i, err := installer.NewInstaller(c.KubeConfig, metav1.NamespaceDefault, 0)
	if err != nil {
		return err
	}

	return i.CreateCRD(TridentOrchestratorCRDName, k8sclient.GetOrchestratorCRDYAML())
}

// createTridentOrchestratorCR creates a new TridentOrchestrator CR
func (c *Controller) createTridentOrchestratorCR(tridentCR *netappv1.TridentOrchestrator) error {
	_, err := c.CRDClient.TridentV1().TridentOrchestrators().Create(ctx(), tridentCR, createOpts)
	if err != nil {
		return err
	}
	return nil
}

// getTridentOrchestratorCRsAll gets all the TridentOrchestrator CRs across all namespaces
func (c *Controller) getTridentOrchestratorCRsAll() ([]*netappv1.TridentOrchestrator, error) {
	list, err := c.CRDClient.TridentV1().TridentOrchestrators().List(ctx(), listOpts)
	if err != nil {
		return nil, err
	}
	plist := make([]*netappv1.TridentOrchestrator, len(list.Items))
	for i := range list.Items {
		plist[i] = &list.Items[i]
	}
	return plist, nil
}

// identifyControllingCRBasedOnStatus identified the controllingCR purely on status and independent of any deployment
// logic involved
func (c *Controller) identifyControllingCRBasedOnStatus() (bool, *netappv1.TridentOrchestrator, error) {
	// Get all TridentOrchestrator CRs
	tridentCRs, err := c.getTridentOrchestratorCRsAll()
	if err != nil {
		return false, nil, err
	} else if len(tridentCRs) == 0 {
		log.Info("Reconciler found no TridentOrchestrator CRs.")
		return false, nil, nil
	}

	// Identify and return the CR that has status neither "NotInstalled" not "Error"
	for _, cr := range tridentCRs {
		if cr.Status.Status == string(AppStatusNotInstalled) || cr.Status.Status == string(AppStatusError) {
			continue
		}

		return true, cr, nil
	}

	return false, nil, nil
}

// identifyControllingCRForTridentDeployments identifies controllingCR for deployment and reports nil if length of the
// operatorCSIDeployments is more than 1
func (c *Controller) identifyControllingCRForTridentDeployments(operatorCSIDeployments []appsv1.Deployment) (*netappv1.
	TridentOrchestrator,
	error,
) {
	// If multiple Trident deployments are found, we will let self-heal logic fix it
	if len(operatorCSIDeployments) > 1 {
		log.Debugf("Found multiple Trident deployments.")
		return nil, nil
	}

	operatorCSIDeployment := operatorCSIDeployments[0]

	// Look for CRs in the deployment's namespace
	tridentCRs, err := c.getTridentOrchestratorCRsAll()
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

	log.Debug("Identifying controlling CRs from the list of all the CRs.")

	// Get CR that controls current Trident deployment
	deploymentCR, err := c.matchDeploymentControllingCR(tridentCRs, operatorCSIDeployment)
	if err != nil {
		return nil, err
	} else if deploymentCR == nil {
		log.WithFields(log.Fields{
			"deployment":          operatorCSIDeployment.Name,
			"deploymentNamespace": operatorCSIDeployment.Namespace,
		}).Debugf("No CR found that controls the Trident deployment.")
		return nil, nil
	}

	return deploymentCR, nil
}

// updateAllCRs get called only when no ControllingCR exist to report a configuration error
func (c *Controller) updateAllCRs(message string) error {
	allCRs, err := c.getTridentOrchestratorCRsAll()
	if err != nil {
		return utils.ReconcileFailedError(fmt.Errorf(
			"unable to get list of all the TridentOrchestrator CRs; err: %v", err))
	}

	// Update status on all the TridentOrchestrator CR(s)
	var debugMessage string
	for _, cr := range allCRs {
		debugMessage = "Updating " + cr.Name + " TridentOrchestrator CR."
		_, err = c.updateTorcEventAndStatus(cr, debugMessage, message, string(AppStatusError), "", cr.Spec.Namespace,
			corev1.EventTypeWarning, nil)
	}

	return nil
}

// updateOtherCRs get called only when a ControllingCR exist to set error state on the non-ControllingCRs
func (c *Controller) updateOtherCRs(controllingCRName string) error {
	allCRs, err := c.getTridentOrchestratorCRsAll()
	if err != nil {
		return utils.ReconcileFailedError(fmt.Errorf(
			"unable to get list of TridentOrchestrator CRs; err: %v", err))
	}

	// Update status on all other TridentOrchestrator CR(s)
	var debugMessage string
	for _, cr := range allCRs {
		if cr.Name != controllingCRName {
			debugMessage = "Updating '" + cr.Name + "' TridentOrchestrator CR."
			statusMessage := fmt.Sprintf("Trident is bound to another CR '%v'",
				controllingCRName)

			_, err = c.updateTorcEventAndStatus(cr, debugMessage, statusMessage, string(AppStatusError), "",
				cr.Spec.Namespace, corev1.EventTypeWarning, nil)
		}
	}

	return nil
}

// updateLogAndStatus updates the event logs and status of a TridentOrchestrator CR (if required)
func (c *Controller) updateTorcEventAndStatus(
	tridentCR *netappv1.TridentOrchestrator, debugMessage, message, status, version, namespace, eventType string,
	specValues *netappv1.TridentOrchestratorSpecValues,
) (torcCR *netappv1.TridentOrchestrator, err error) {
	var logEvent bool

	if torcCR, logEvent, err = c.updateTridentOrchestratorCRStatus(tridentCR, debugMessage, message, status,
		version, namespace, specValues); err != nil {
		return
	}

	// Log event only when status has beeen updated or a event type  warning has occurred
	if logEvent || eventType == corev1.EventTypeWarning {
		c.eventRecorder.Event(tridentCR, eventType, status, message)
	}

	return
}

// updateTridentOrchestratorCRStatus updates the status of a CR if required
func (c *Controller) updateTridentOrchestratorCRStatus(
	tridentCR *netappv1.TridentOrchestrator, debugMessage, message, status, version, namespace string,
	specValues *netappv1.TridentOrchestratorSpecValues,
) (*netappv1.TridentOrchestrator, bool, error) {
	logFields := log.Fields{"tridentOrchestratorCR": tridentCR.Name}

	// Update status of the tridentCR
	log.WithFields(logFields).Debug(debugMessage)

	var installParams netappv1.TridentOrchestratorSpecValues
	if specValues != nil {
		installParams = *specValues
	}

	newStatusDetails := netappv1.TridentOrchestratorStatus{
		Message:                   message,
		Status:                    status,
		Version:                   version,
		Namespace:                 namespace,
		CurrentInstallationParams: installParams,
	}

	if reflect.DeepEqual(tridentCR.Status, newStatusDetails) {
		log.WithFields(logFields).Info("New status is same as the old status, no update needed.")

		return tridentCR, false, nil
	}

	prClone := tridentCR.DeepCopy()
	prClone.Status = newStatusDetails

	newTridentCR, err := c.CRDClient.TridentV1().TridentOrchestrators().UpdateStatus(
		ctx(), prClone, updateOpts)
	if err != nil {
		log.WithFields(logFields).Errorf("could not update status of the CR; err: %v", err)
	} else {
		// Setting explicitly as this is a Client-go bug, fixed in the newest version of client-go
		newTridentCR.APIVersion = tridentCR.APIVersion
		newTridentCR.Kind = tridentCR.Kind
	}

	return newTridentCR, true, err
}

/***********************************************
 * Trident Provisioner CRD & CR helper functions
 ***********************************************/
// getTridentTprovCRsAll gets all the TridentProvisioner CRs across all namespaces
func (c *Controller) getTridentTprovCRsAll() ([]tprovv1.TridentProvisioner, error) {
	list, err := c.CRDTprovClient.TridentV1().TridentProvisioners(corev1.NamespaceAll).List(ctx(), listOpts)
	if err != nil {
		return nil, err
	}
	return list.Items, nil
}

// identifyTprovCRWithInstalledStatus identified the controllingCR with installed status
func (c *Controller) identifyTprovCRWithInstalledStatus() (*tprovv1.TridentProvisioner, error) {
	// Get all TridentOrchestrator CRs
	tridentTprovCRs, err := c.getTridentTprovCRsAll()
	if err != nil {
		return nil, err
	} else if len(tridentTprovCRs) == 0 {
		log.Info("Reconciler found no TridentProvisioner CRs.")
	}

	// Identify and return the CR that has status neither "Installed"
	for _, cr := range tridentTprovCRs {
		if cr.Status.Status == string(AppStatusInstalled) {
			return &cr, nil
		}
	}

	return nil, nil
}

// deleteTridentTprovCRD deletes TridentProvisioner CRD
func (c *Controller) deleteTridentTprovCRD() error {
	log.Debug("Starting TridentProvisioner CRD cleanup.")

	logFields := log.Fields{"CRD": TridentProvisionersCRDName}

	// Get the CRD and check for finalizers
	crd, err := c.K8SClient.GetCRD(TridentProvisionersCRDName)
	if isNotFoundError(err) {
		log.WithFields(logFields).Info("CRD not found.")
		return nil
	}

	log.WithFields(logFields).Info("CRD found; removing the CRD.")

	// Remove finalizers if present
	if len(crd.Finalizers) > 0 {
		if err := c.K8SClient.RemoveFinalizerFromCRD(TridentProvisionersCRDName); err != nil {
			log.WithFields(logFields).Errorf("Could not remove finalizer from CRD; %v", err)
			return err
		} else {
			log.WithFields(logFields).Debug("Removed finalizers from CRD.")
		}
	} else {
		log.WithFields(logFields).Debug("No finalizers found on CRD.")
	}

	// Try deleting CRD
	if crd.DeletionTimestamp.IsZero() {
		log.WithFields(logFields).Debug("Deleting CRD.")

		err := c.K8SClient.DeleteCRD(TridentProvisionersCRDName)
		if isNotFoundError(err) {
			log.WithFields(logFields).Info("CRD not found during deletion.")
			return nil
		} else if err != nil {
			log.WithFields(logFields).Errorf("Could not delete CRD; %v", err)
			return err
		}
	} else {
		log.WithFields(logFields).Debug("CRD already has deletion timestamp.")
	}

	log.WithFields(logFields).Info("CRD deleted.")

	return nil
}

// deleteTridentTprovCR deletes TridentProvisioner CR
func (c *Controller) deleteTridentTprovCR(name, namespace string) error {
	return c.CRDTprovClient.TridentV1().TridentProvisioners(namespace).Delete(ctx(), name, deleteOpts)
}

// deleteTridentTprovCRAll deletes all the TridentProvisioner CRs
func (c *Controller) deleteTridentTprovCRAll() error {
	log.Debug("Starting TridentProvisioner CR cleanup.")

	// See if CRD exists
	exists, err := c.doesCRDExist(TridentProvisionersCRDName)
	if err != nil {
		return err
	} else if !exists {
		log.WithField("CRDName", TridentProvisionersCRDName).Debugf("CRD not present.")
		return nil
	}

	tridentTprovCRs, err := c.getTridentTprovCRsAll()
	if err != nil {
		return err
	} else if len(tridentTprovCRs) == 0 {
		log.Debug("No TridentProvisioner CRs found.")
		return nil
	}

	for _, tridentTprovCR := range tridentTprovCRs {
		if tridentTprovCR.DeletionTimestamp.IsZero() {
			_ = c.CRDTprovClient.TridentV1().TridentProvisioners(tridentTprovCR.Namespace).Delete(ctx(), tridentTprovCR.Name,
				deleteOpts)
		}
	}

	log.Info("TridentProvisioner CRs deleted.")
	return nil
}

func (c *Controller) cleanupTridentProvisioner() error {
	if err := c.deleteTridentTprovCRAll(); err != nil {
		return err
	}

	if err := c.deleteTridentTprovCRD(); err != nil {
		return err
	}

	return nil
}

/*************************************
 * Trident deployment helper functions
 *************************************/
// getTridentOrchestratorCSIDeployments returns CSI Trident deployments (if any) created by Trident Orchestrator CR
func (c *Controller) getTridentOrchestratorCSIDeployments() ([]appsv1.Deployment, string, error) {
	return c.getCRDBasedCSIDeployments(CRDName)
}

// getCRDBasedCSIDeployments returns CSI Trident deployments (if any) created by CR based of crdName
func (c *Controller) getCRDBasedCSIDeployments(crdName string) ([]appsv1.Deployment, string, error) {
	var tridentCSIDeployments []appsv1.Deployment
	var tridentCSIExist bool
	var tridentCSINamespace string
	var returnErr error

	deploymentLabel := installer.TridentCSILabel

	if deployments, err := c.K8SClient.GetDeploymentsByLabel(installer.TridentCSILabel, true); err != nil {
		log.Errorf("Unable to get list of deployments by label %v", deploymentLabel)
		returnErr = fmt.Errorf("unable to get list of deployments; err: %v", err)
	} else if len(deployments) == 0 {
		log.Info("Trident deployment not found.")
	} else {
		for _, deployment := range deployments {
			if deployment.OwnerReferences != nil && strings.ToLower(deployment.OwnerReferences[0].
				Kind) == strings.ToLower(crdName) {
				// Found an operator based Trident CSI deployment
				log.WithFields(log.Fields{
					"deploymentName":      deployment.Name,
					"deploymentNamespace": deployment.Namespace,
				}).Infof("An operator based Trident CSI deployment was found.")

				if tridentCSIExist {
					// Not the first time encountering Operator based CSI Trident, hopefully this is never the case.
					tridentCSINamespace = "<multiple>"
				} else {
					tridentCSINamespace = deployment.Namespace
				}

				tridentCSIDeployments = append(tridentCSIDeployments, deployment)
				tridentCSIExist = true

				break
			}
		}
	}
	return tridentCSIDeployments, tridentCSINamespace, returnErr
}

// matchDeploymentControllingCR identified the controllingCR for an operator based deployment from the list of CRs
func (c *Controller) matchDeploymentControllingCR(tridentCRs []*netappv1.TridentOrchestrator,
	operatorCSIDeployment appsv1.Deployment,
) (*netappv1.TridentOrchestrator, error) {
	// Identify Trident CR that controls the deployment
	var controllingCR *netappv1.TridentOrchestrator
	for _, cr := range tridentCRs {
		if metav1.IsControlledBy(&operatorCSIDeployment, cr) {
			controllingCR = cr
			log.WithFields(log.Fields{
				"name": cr.Name,
			}).Info("Found CR that controls current Trident deployment.")

			break
		}
	}

	return controllingCR, nil
}

/**************************
 * Trident helper functions
 **************************/
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

// uninstallCSIPreviewTrident uninstalls Trident CSI Preview
func (c *Controller) uninstallCSIPreviewTrident(namespace string) error {
	i, err := installer.NewInstaller(c.KubeConfig, namespace, 0)
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
	i, err := installer.NewInstaller(c.KubeConfig, namespace, 0)
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

// removeNonTorcBasedCSIInstallation identifies if the Tprov-based or tridentctl-based CSI Trident is installed,
// if it is installed it will be uninstalled.
func (c *Controller) removeNonTorcBasedCSIInstallation(tridentCR *netappv1.TridentOrchestrator) error {
	var uninstallRequired bool
	// Check for the CSI based Trident installation
	csiDeploymentFound, csiTridentNamespace, err := c.isCSITridentInstalled()
	if err != nil {
		return utils.ReconcileFailedError(err)
	} else if csiDeploymentFound {
		eventMessage := fmt.Sprintf("Removing a non-Trident Orchestrator based CSI Trident installation found in the"+
			" namespace '%v'.", csiTridentNamespace)

		log.Info(eventMessage)
		c.eventRecorder.Event(tridentCR, corev1.EventTypeNormal, string(AppStatusInstalling), eventMessage)

		uninstallRequired = true
	}

	if uninstallRequired {
		if err := c.uninstallTridentAll(csiTridentNamespace); err != nil {
			// Update status of the tridentCR  to `Failed`
			debugMessage := "Updating Trident Orchestrator CR after failed installation."
			failureMessage := fmt.Sprintf("Failed to install Trident; failed to remove existing non-"+
				"TridentOrchestrator CSI Trident installation; err: %v", err)

			log.Error(failureMessage)

			if _, crErr := c.updateTorcEventAndStatus(tridentCR, debugMessage, failureMessage,
				string(AppStatusFailed), "", tridentCR.Spec.Namespace, corev1.EventTypeWarning, nil); crErr != nil {
				log.Error(crErr)
			}

			// Install failed, so fail the reconcile loop
			return utils.ReconcileFailedError(err)
		}

		// Uninstall succeeded, so re-run the reconcile loop
		eventMessage := "Non-Trident Orchestrator based CSI Trident installation removed."

		log.Info(eventMessage)
		c.eventRecorder.Event(tridentCR, corev1.EventTypeNormal, string(AppStatusInstalling), eventMessage)
	}

	return nil
}

/*****************************
 * Versioning helper functions
 *****************************/
// getCurrentTridentAndK8sVersion reports current Trident version installed and K8s version according
// to which Trident was installed
func (c *Controller) getCurrentTridentAndK8sVersion(tridentCR *netappv1.TridentOrchestrator) (string, string, error) {
	var currentTridentVersionString string
	var currentK8sVersionString string

	i, err := installer.NewInstaller(c.KubeConfig, tridentCR.Status.Namespace, tridentCR.Spec.K8sTimeout)
	if err != nil {
		return "", "", err
	}

	currentDeployment, _, _, err := i.TridentDeploymentInformation(installer.TridentCSILabel)
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

// validateCurrentK8sVersion identifies any changes in the K8s version, if it is valid, and if not valid should
// user be warned about it
func (c *Controller) validateCurrentK8sVersion() (bool, string) {
	var isValid bool
	var warning string

	currentK8sVersion, err := c.Clients.KubeClient.Discovery().ServerVersion()

	if err != nil {
		log.WithField("err", err).Error("Could not get Kubernetes version; unable to verify if update is required.")
		return isValid, ""
	} else if currentK8sVersion == nil {
		log.WithField("currentK8sVersion", "nil").
			Error("Could not identify Kubernetes version; unable to verify if update is required.")
		return isValid, ""
	}

	if currentK8sVersion != c.K8SVersion {
		c.K8SVersion = currentK8sVersion
	}
	// Validate the Kubernetes server version
	if err := commonconfig.ValidateKubernetesVersionFromInfo(commonconfig.KubernetesVersionMin, currentK8sVersion); err != nil {
		log.Warningf(K8sVersionCheckSupportWarning, c.K8SVersion.String())
		warning = fmt.Sprintf(K8sVersionCheckSupportWarning, c.K8SVersion.String())
	} else {
		log.WithField("version", currentK8sVersion.String()).Debugf("Kubernetes version is supported.")
		isValid = true
	}

	return isValid, warning
}

// tridentUpgradeNeeded compares the K8's version as per which Trident is installed with the current K8s version,
// if it has changed we need to update Trident as well
func (c *Controller) tridentUpgradeNeeded(tridentK8sConfigVersion string) bool {
	var shouldUpdate bool

	if tridentK8sConfigVersion == "" {
		return shouldUpdate
	}

	currentTridentConfigK8sVersion := utils.MustParseSemantic(tridentK8sConfigVersion).ToMajorMinorVersion()
	K8sVersion := utils.MustParseSemantic(c.K8SVersion.GitVersion).ToMajorMinorVersion()

	if currentTridentConfigK8sVersion.LessThan(K8sVersion) || currentTridentConfigK8sVersion.GreaterThan(K8sVersion) {
		log.WithFields(log.Fields{
			"currentTridentConfigK8sVersion": currentTridentConfigK8sVersion.String(),
			"newK8sVersion":                  K8sVersion.String(),
		}).Infof("Kubernetes version has changed; Trident operator" +
			" should change the Trident installation as per the new Kubernetes version")

		shouldUpdate = true
	}

	return shouldUpdate
}

/************************
 * Misc. helper functions
 ************************/
// doesCRDExist checks if the given CRD exist
func (c *Controller) doesCRDExist(crdName string) (bool, error) {
	// Discover CRD data
	crdExist, returnError := c.K8SClient.CheckCRDExists(crdName)
	if returnError != nil {
		return false, fmt.Errorf("unable to identify if %v CRD exists; err: %v", crdName, returnError)
	}

	return crdExist, returnError
}

func isNotFoundError(err error) bool {
	if statusErr, ok := err.(*apierrors.StatusError); ok && statusErr.Status().Reason == metav1.StatusReasonNotFound {
		return true
	}
	return false
}

// resourceTypeToK8sKind translates resources to corresponding native Kinds
func (c *Controller) resourceTypeToK8sKind(resourceType ResourceType) string {
	switch resourceType {
	case ResourceTridentOrchestratorCR:
		return "Trident Orchestrator CR"
	case ResourceDeployment:
		return "deployment"
	case ResourceDaemonSet:
		return "daemonset"
	default:
		return "invalid object"
	}
}
