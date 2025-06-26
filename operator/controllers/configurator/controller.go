// Copyright 2024 NetApp, Inc. All Rights Reserved.

package configurator

import (
	"context"
	"fmt"
	"reflect"
	"time"

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

	k8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/operator/clients"
	confClients "github.com/netapp/trident/operator/controllers/configurator/clients"
	"github.com/netapp/trident/operator/controllers/configurator/storage_drivers"
	netappv1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	operatorV1 "github.com/netapp/trident/operator/crd/apis/netapp/v1"
	"github.com/netapp/trident/operator/crd/client/clientset/versioned/scheme"
	"github.com/netapp/trident/utils/errors"
)

const (
	ControllerName    = "Trident Configurator"
	ControllerVersion = "0.1"
	CRDName           = "TridentConfigurator"
	Operator          = "trident-operator.netapp.io"

	TridentConfiguratorCRDName = "tridentconfigurators.trident.netapp.io"
)

var ctx = context.TODO

type Controller struct {
	Clients confClients.ConfiguratorClientInterface

	eventRecorder record.EventRecorder

	configuratorIndexer  cache.Indexer
	configuratorInformer cache.SharedIndexInformer
	configuratorWatcher  cache.ListerWatcher

	stopChan chan struct{}

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
}

func NewController(clientFactory *clients.Clients, cacheRefreshPeriod time.Duration) (*Controller, error) {
	kClient, err := confClients.NewExtendedK8sClient(clientFactory.KubeConfig, "trident",
		0, clientFactory.KubeClient)
	if err != nil {
		return nil, err
	}

	c := &Controller{
		Clients: confClients.NewConfiguratorClient(kClient,
			clients.NewTridentCRDClient(clientFactory.TridentCRDClient),
			clients.NewSnapshotCRDClient(clientFactory.SnapshotClient),
			clients.NewOperatorCRDClient(clientFactory.CRDClient)),
		stopChan: make(chan struct{}),
		workqueue: workqueue.NewRateLimitingQueueWithConfig(workqueue.DefaultControllerRateLimiter(),
			workqueue.RateLimitingQueueConfig{Name: CRDName}),
	}

	// Add our types to the default Kubernetes Scheme so Events can be logged.
	utilruntime.Must(scheme.AddToScheme(scheme.Scheme))

	// Set up event broadcaster
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&clientv1.EventSinkImpl{Interface: clientFactory.KubeClient.CoreV1().Events("")})
	c.eventRecorder = broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: Operator})

	// Set up a watch for TridentConfigurator CRs
	c.configuratorWatcher = &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return clientFactory.CRDClient.TridentV1().TridentConfigurators().List(ctx(), options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return clientFactory.CRDClient.TridentV1().TridentConfigurators().Watch(ctx(), options)
		},
	}

	// Set up the CR indexing controller
	c.configuratorInformer = cache.NewSharedIndexInformer(
		c.configuratorWatcher,
		&netappv1.TridentConfigurator{},
		cacheRefreshPeriod,
		cache.Indexers{},
	)
	c.configuratorIndexer = c.configuratorInformer.GetIndexer()

	// Add handlers for TridentConfigurator CRs
	_, _ = c.configuratorInformer.AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.addConfigurator,
			UpdateFunc: c.updateConfigurator,
			DeleteFunc: c.deleteConfigurator,
		},
	)

	return c, nil
}

// Activate controller.
func (c *Controller) Activate() error {
	Log().WithField("Controller", ControllerName).Infof("Activating controller.")

	// The reason we have this here is to ensure that by the time Trident Orchestrator's List and Watcher
	// start they do not throw any error for unable to list/watch this CRD.
	if err := c.ensureTridentConfiguratorCRDExists(); err != nil {
		Log().WithField("err", err).Warnf("Unable to ensure TridentOrchestrator exist.")
	}

	go c.configuratorInformer.Run(c.stopChan)

	Log().Info("Starting workers")
	go wait.Until(c.runWorker, time.Second, c.stopChan)

	Log().Info("Started workers")

	return nil
}

// Deactivate controller.
func (c *Controller) Deactivate() error {
	Log().WithField("Controller", ControllerName).Infof("Deactivating controller.")

	close(c.stopChan)

	c.workqueue.ShutDown()
	utilruntime.HandleCrash()
	return nil
}

// GetName returns controller name.
func (c *Controller) GetName() string {
	return ControllerName
}

// Version returns the controller version.
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
		var keyItem string
		var ok bool
		// We expect KeyItem to come off the workqueue. We do this as the
		// delayed nature of the workqueue means the items in the informer
		// cache may actually be more up to date that when the item was
		// initially put onto the workqueue.
		if keyItem, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			Log().Errorf("expected string in workqueue but got %#v", obj)
			return nil
		}
		// Run the reconcile, passing it the keyItems struct to be synced.
		if err := c.reconcile(keyItem); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			if errors.IsUnsupportedConfigError(err) {
				errMessage := fmt.Sprintf("found unsupported configuration, "+
					"needs manual intervention to fix the issue;"+
					"error syncing tconf '%s': %s, not requeuing", keyItem, err.Error())

				c.workqueue.Forget(keyItem)

				Log().Errorf(errMessage)
				Log().Info("-------------------------------------------------")
				Log().Info("-------------------------------------------------")

				return errors.New(errMessage)
			} else if errors.IsNotFoundError(err) {
				errMessage := fmt.Sprintf("resource not found, needs manual intervention to fix it;"+
					"error syncing tconf '%s': %s, not requeuing", keyItem, err.Error())

				c.workqueue.Forget(keyItem)

				Log().Errorf(errMessage)
				Log().Info("-------------------------------------------------")
				Log().Info("-------------------------------------------------")

				return errors.New(errMessage)
			} else if errors.IsReconcileIncompleteError(err) {
				c.workqueue.Add(keyItem)
			} else {
				c.workqueue.AddRateLimited(keyItem)
			}

			errMessage := fmt.Sprintf("error syncing tconf '%s': %s, requeuing", keyItem, err.Error())
			Log().Errorf(errMessage)
			Log().Info("-------------------------------------------------")
			Log().Info("-------------------------------------------------")

			return errors.New(errMessage)
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		Log().Infof("Synced TridentConfigurator '%s'", keyItem)
		Log().Info("-------------------------------------------------")
		Log().Info("-------------------------------------------------")

		return nil
	}(obj)
	if err != nil {
		Log().Error(err)
		return true
	}

	return true
}

// addConfigurator is the add handler for the TridentConfigurator watcher.
func (c *Controller) addConfigurator(obj interface{}) {
	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		Log().Error(err)
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Log().Errorf("invalid resource key: %s", key)
		return
	}

	Log().WithFields(LogFields{
		"CR":  name,
		"CRD": CRDName,
	}).Infof("CR added.")

	c.workqueue.Add(key)
}

// updateConfigurator is the update handler for the TridentConfigurator watcher.
func (c *Controller) updateConfigurator(oldObj, newObj interface{}) {
	oldCR, ok := oldObj.(*netappv1.TridentConfigurator)
	if !ok {
		Log().Errorf("'%s' controller expected '%s' CR; got '%v'", ControllerName, CRDName, oldObj)
		return
	}

	newCR, ok := newObj.(*netappv1.TridentConfigurator)
	if !ok {
		Log().Errorf("'%s' controller expected '%s' CR; got '%v'", ControllerName, CRDName, newObj)
		return
	}

	if !newCR.ObjectMeta.DeletionTimestamp.IsZero() {
		Log().WithFields(LogFields{
			"name":              newCR.Name,
			"deletionTimestamp": newCR.ObjectMeta.DeletionTimestamp,
		}).Infof("'%s' CR is being deleted, not updated.", CRDName)
		return
	}

	if !reflect.DeepEqual(oldCR.Status, newCR.Status) {
		// Update request comes when we UpdateStatus of tconfCR
		Log().Debug("Update request came for tconfCR after we updated its status; this doesn't need processing; skipping")
		return
	}

	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(newObj); err != nil {
		Log().Error(err)
		return
	}

	Log().WithFields(LogFields{
		"CR":  newCR.Name,
		"CRD": CRDName,
	}).Infof("CR updated.")

	c.workqueue.Add(key)
}

// deleteConfigurator is the delete handler for the TridentConfigurator watcher.
func (c *Controller) deleteConfigurator(obj interface{}) {
	var key string
	var err error

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		Log().Error(err)
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Log().Errorf("invalid resource key: '%s'", key)
		return
	}

	Log().WithFields(LogFields{
		"CR":  name,
		"CRD": CRDName,
	}).Infof("CR deleted.")

	c.workqueue.Add(key)
}

// reconcile runs the reconcile logic and ensures we move to the desired state and the desired state is
// maintained
func (c *Controller) reconcile(keyItem string) error {
	Log().Infof("Reconcile request came for TridentConfigurator CR %s", keyItem)
	// Get Controlling Trident Orchestrator CR and wait till trident is installed.
	torcCR, err := c.Clients.GetControllingTorcCR()
	if err != nil {
		Log().Error("Failed to get controlling torcCR", err)
		return err
	}

	_, tconfCRName, err := cache.SplitMetaNamespaceKey(keyItem)
	if err != nil {
		Log().Errorf("Invalid resource key: '%s'", keyItem)
		return err
	}

	tconfCR, err := c.Clients.GetTconfCR(tconfCRName)
	if err != nil {
		Log().Error("Failed to get tconfCR: ", err)
		return errors.NotFoundError(err.Error())
	}

	if err = tconfCR.Validate(); err != nil {
		Log().Error("Invalid tconfCR: ", err)
		return err
	}

	driverName, err := tconfCR.GetStorageDriverName()
	if err != nil {
		Log().Error("Failed to get storage driver name: ", err)
		return err
	}

	switch driverName {
	case config.AzureNASStorageDriverName:
		anf, err := storage_drivers.NewANFInstance(torcCR, tconfCR, c.Clients)
		if err != nil {
			Log().Info("Failed to create ANF backend instance: ", err)
			return err
		}
		if err := c.ProcessBackend(anf, tconfCR); err != nil {
			Log().Error("Failed to process ANF backend: ", err)
			return err
		}
	case config.OntapNASStorageDriverName, config.OntapSANStorageDriverName:
		isAwsFSxN, err := tconfCR.IsAWSFSxNTconf()
		if err != nil {
			Log().Error("Failed to check if tconf is for AWS FSxN: ", err)
			return err
		}
		if isAwsFSxN {
			Log().Debugf("Tconf indicates auto-backend config is for AWS FSxN")
			fsxn, err := storage_drivers.NewFSxNInstance(torcCR, tconfCR, c.Clients)
			if err != nil {
				Log().Info("Failed to create FsxN backend instance: ", err)
				return err
			}
			if err := c.ProcessBackend(fsxn, tconfCR); err != nil {
				Log().Error("Failed to process FSxN backend: ", err)
				return err
			}
		}
	default:
		return fmt.Errorf("backend not supported")
	}

	return nil
}

// ensureTridentConfiguratorCRDExists creates TridentConfigurator CRD if it doesn't exist.
func (c *Controller) ensureTridentConfiguratorCRDExists() error {
	if err := c.Clients.CreateOrPatchObject(confClients.OCRD, TridentConfiguratorCRDName,
		"", k8sclient.GetConfiguratorCRDYAML()); err != nil {
		Log().Error("Failed to create/patch TConf CRD", err)
		return err
	}

	return nil
}

// ProcessBackend does validate backend, create backend, create storage class and create snapshot class operations
// on the trident configurator CR in phases.
func (c *Controller) ProcessBackend(backend storage_drivers.Backend, tconfCR *operatorV1.TridentConfigurator) error {
	if tconfCR == nil {
		return fmt.Errorf("invalid trident configurator CR")
	}

	currentPhase := operatorV1.TConfPhase("")
	currentStatus := operatorV1.Processing
	var processErr, updateErr error
	cProvider := backend.GetCloudProvider()

	for currentStatus == operatorV1.Processing {
		currentPhase = c.getNextProcessingPhase(currentPhase)

		switch currentPhase {
		case operatorV1.ValidatingConfig:
			tconfCR, processErr, updateErr = c.backendGenericOperation(tconfCR, currentPhase, backend.Validate, cProvider)
			if processErr != nil {
				Log().Error("Backend config validation failed", processErr)
				return errors.UnsupportedConfigError(processErr.Error())
			}
			if updateErr != nil {
				Log().Error("Updating backend config validation status failed", updateErr)
				return updateErr
			}
		case operatorV1.CreatingBackend:
			tconfCR, processErr, updateErr = c.backendCreateOperation(tconfCR, currentPhase, backend.Create, cProvider)
			if processErr != nil {
				Log().Error("Backend creation failed", processErr)
				return processErr
			}
			if updateErr != nil {
				Log().Error("Updating backend creation status failed", updateErr)
				return updateErr
			}
		case operatorV1.CreatingSC:
			tconfCR, processErr, updateErr = c.backendGenericOperation(tconfCR, currentPhase, backend.CreateStorageClass,
				cProvider)
			if processErr != nil {
				Log().Error("Backend storage class creation failed", processErr)
				return processErr
			}
			if updateErr != nil {
				Log().Error("Updating TConf status for backend storage class failed", updateErr)
				return updateErr
			}
		case operatorV1.CreatingSnapClass:
			tconfCR, processErr, updateErr = c.backendGenericOperation(tconfCR, currentPhase, backend.CreateSnapshotClass,
				cProvider)
			if processErr != nil {
				Log().Error("Backend snapshot class creation failed", processErr)
				return processErr
			}
			if updateErr != nil {
				Log().Error("Updating TConf status for backend snapshot class failed", updateErr)
				return updateErr
			}
			currentStatus = operatorV1.Success
		default:
			Log().Error("Encountered incorrect processing phase.")
			return fmt.Errorf("encountered incorrect processing phase; exiting ProcessBackend")
		}
	}

	return nil
}

// backendGenericOperation does operationFunc and updates tconfCR status accordingly.
// This function is common for Backends.Validate(), Backends.CreateStorageClass(), Backends.CreateSnapshotClass().
func (c *Controller) backendGenericOperation(
	tconfCR *operatorV1.TridentConfigurator, currPhase operatorV1.TConfPhase,
	operationFunc func() error, cloudProvider string,
) (newTconfCR *operatorV1.TridentConfigurator, operationErr, updateErr error) {
	newTconfCR, updateErr = c.updateEventAndStatus(tconfCR, currPhase, nil, cloudProvider, tconfCR.Status.BackendNames)
	if updateErr != nil {
		return
	}

	operationErr = operationFunc()

	newPhase := c.getProcessedPhase(currPhase)
	newTconfCR, updateErr = c.updateEventAndStatus(newTconfCR, newPhase, operationErr, cloudProvider, newTconfCR.Status.BackendNames)
	return
}

// backendCreateOperation does operationFunc and updates tconfCR status accordingly.
// This function is implemented for Backends.Create().
func (c *Controller) backendCreateOperation(
	tconfCR *operatorV1.TridentConfigurator, currPhase operatorV1.TConfPhase,
	operationFunc func() ([]string, error), cloudProvider string,
) (newTconfCR *operatorV1.TridentConfigurator, operationErr, updateErr error) {
	newTconfCR, updateErr = c.updateEventAndStatus(tconfCR, currPhase, nil, cloudProvider, tconfCR.Status.BackendNames)
	if updateErr != nil {
		return
	}

	backendNames, operationErr := operationFunc()

	newPhase := c.getProcessedPhase(currPhase)
	newTconfCR, updateErr = c.updateEventAndStatus(newTconfCR, newPhase, operationErr, cloudProvider, backendNames)
	return
}

// updateEventAndStatus updates events and status of trident configurator CR.
func (c *Controller) updateEventAndStatus(
	tconfCR *operatorV1.TridentConfigurator, currentPhase operatorV1.TConfPhase,
	operationError error, cloudProvider string, backendNames []string,
) (*operatorV1.TridentConfigurator, error) {
	newStatus := c.getNewConfiguratorStatus(currentPhase, tconfCR.Status.Phase, operationError, cloudProvider, backendNames)

	newTconfCR, updateEvent, err := c.Clients.UpdateTridentConfiguratorStatus(tconfCR, newStatus)
	if err != nil {
		return newTconfCR, err
	}

	c.updateTridentConfiguratorEvent(newTconfCR, operationError, updateEvent)

	return newTconfCR, nil
}

// updateTridentConfiguratorEvent updates the events for TConf object in event recorder.
func (c *Controller) updateTridentConfiguratorEvent(
	tconfCR *operatorV1.TridentConfigurator, updateError error, updateEvent bool,
) {
	if updateError != nil {
		c.eventRecorder.Event(tconfCR, corev1.EventTypeWarning, tconfCR.Status.LastOperationStatus, tconfCR.Status.Message)
		return
	}
	if updateEvent {
		c.eventRecorder.Event(tconfCR, corev1.EventTypeNormal, tconfCR.Status.LastOperationStatus, tconfCR.Status.Message)
		return
	}
}

// getNewConfiguratorStatus returns the new configurator status that we update on the CR.
// We set LastOperationStatus as success when we reach operatorV1.Done phase without errors.
// If any error occurs, we return the LastOperationStatus as failed.
func (c *Controller) getNewConfiguratorStatus(
	currentPhase operatorV1.TConfPhase, lastPhase string, err error, cloudProvider string, backendNames []string,
) operatorV1.TridentConfiguratorStatus {
	newStatus := operatorV1.TridentConfiguratorStatus{
		BackendNames:        backendNames,
		Phase:               string(currentPhase),
		LastOperationStatus: string(operatorV1.Processing),
		CloudProvider:       cloudProvider,
	}

	if err != nil {
		newStatus.Message = fmt.Sprintf("Failed: %v", err)
		newStatus.Phase = lastPhase
		newStatus.LastOperationStatus = string(operatorV1.Failed)
		return newStatus
	}

	switch currentPhase {
	case operatorV1.ValidatingConfig:
		newStatus.Message = "Validating backend configuration"
	case operatorV1.ValidatedConfig:
		newStatus.Message = "Provided backend configuration is correct"
	case operatorV1.CreatingBackend:
		newStatus.Message = "Creating backend with the provided configuration"
	case operatorV1.CreatedBackend:
		newStatus.Message = "Backend creation successful"
	case operatorV1.CreatingSC:
		newStatus.Message = "Creating storage classes for the backend"
	case operatorV1.CreatedSC:
		newStatus.Message = "Storage class creation successful"
	case operatorV1.CreatingSnapClass:
		newStatus.Message = "Validating backend configuration"
	case operatorV1.Done:
		newStatus.Message = "Completed Trident backend configuration"
		newStatus.LastOperationStatus = string(operatorV1.Success)
	}

	return newStatus
}

// getNextProcessingPhase returns the next phase that needs to be processed.
func (c *Controller) getNextProcessingPhase(currPhase operatorV1.TConfPhase) operatorV1.TConfPhase {
	switch currPhase {
	case operatorV1.ValidatingConfig:
		return operatorV1.CreatingBackend
	case operatorV1.CreatingBackend:
		return operatorV1.CreatingSC
	case operatorV1.CreatingSC:
		return operatorV1.CreatingSnapClass
	default:
		// When we start processing tconfCR, we send it's currPhase as empty.
		return operatorV1.ValidatingConfig
	}
}

// getProcessedPhase returns the corresponding done phase for the given processing phase.
// This function should always be called with an "ing" Phase to get the correct processed phase.
func (c *Controller) getProcessedPhase(currPhase operatorV1.TConfPhase) operatorV1.TConfPhase {
	switch currPhase {
	case operatorV1.ValidatingConfig:
		return operatorV1.ValidatedConfig
	case operatorV1.CreatingBackend:
		return operatorV1.CreatedBackend
	case operatorV1.CreatingSC:
		return operatorV1.CreatedSC
	case operatorV1.CreatingSnapClass:
		return operatorV1.Done
	default:
		return ""
	}
}
