// Copyright 2019 NetApp, Inc. All Rights Reserved.

package crd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"time"

	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	clik8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	"github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	clientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	tridentscheme "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/scheme"
	"github.com/netapp/trident/persistent_store/crd/client/informers/externalversions"
	trident_informers "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions"
	informers "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions/netapp/v1"
	trident_informers_v1 "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions/netapp/v1"
	listers "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	"github.com/netapp/trident/storage"
)

type CrdPlugin interface {
	frontend.Plugin
}

const controllerAgentName = "trident-crd-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a CRD is synced
	SuccessSynced = "Synced"

	// MessageResourceSynced is the message used for an Event fired when a CRD is synced successfully
	MessageResourceSynced = "CRD synced successfully"
)

// TridentCrdController is the controller implementation for Trident's CRD resources
type TridentCrdController struct {
	// orchestrator is a reference to the core orchestrator
	orchestrator core.Orchestrator

	// kubeClientset is a standard kubernetes clientset
	kubeClientset kubernetes.Interface

	// crdClientset is a clientset for our own API group
	crdClientset clientset.Interface

	crdControllerStopChan chan struct{}
	crdInformerFactory    externalversions.SharedInformerFactory
	crdInformer           informers.Interface

	// TridentBackend CRD handling
	backendsLister listers.TridentBackendLister
	backendsSynced cache.InformerSynced

	// TridentNode CRD handling
	nodesLister listers.TridentNodeLister
	nodesSynced cache.InformerSynced

	// TridentStorageClass CRD handling
	storageClassesLister listers.TridentStorageClassLister
	storageClassesSynced cache.InformerSynced

	// TridentTransaction CRD handling
	transactionsLister listers.TridentTransactionLister
	transactionsSynced cache.InformerSynced

	// TridentVersion CRD handling
	versionsLister listers.TridentVersionLister
	versionsSynced cache.InformerSynced

	// TridentVolume CRD handling
	volumesLister listers.TridentVolumeLister
	volumesSynced cache.InformerSynced

	// TridentSnapshot CRD handling
	snapshotsLister listers.TridentSnapshotLister
	snapshotsSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface

	// recorder is an event recorder for recording Event resources to the Kubernetes API.
	recorder record.EventRecorder
}

func NewTridentCrdController(o core.Orchestrator, apiServerIP, kubeConfigPath string) (*TridentCrdController, error) {
	kubeConfig, err := clientcmd.BuildConfigFromFlags(apiServerIP, kubeConfigPath)
	if err != nil {
		return nil, err
	}

	// Create the CLI-based Kubernetes client
	client, err := clik8sclient.NewKubectlClient("", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Kubernetes client; %v", err)
	}

	// when running in binary mode, we use the current namespace as determined by the CLI client
	return newTridentCrdController(o, kubeConfig, client.Namespace())
}

func NewTridentCrdControllerInCluster(o core.Orchestrator) (*TridentCrdController, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	// when running in a pod, we use the Trident pod's namespace
	bytes, err := ioutil.ReadFile(config.TridentNamespaceFile)
	if err != nil {
		log.WithFields(log.Fields{
			"error":         err,
			"namespaceFile": config.TridentNamespaceFile,
		}).Fatal("Trident CRD Controller frontend failed to obtain Trident's namespace.")
	}
	tridentNamespace := string(bytes)

	return newTridentCrdController(o, kubeConfig, tridentNamespace)
}

// newTridentCrdController returns a new Trident CRD controller frontend
func newTridentCrdController(
	orchestrator core.Orchestrator, kubeConfig *rest.Config, tridentNamespace string,
) (*TridentCrdController, error) {
	kubeClientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	crdClientset, err := versioned.NewForConfig(kubeConfig)
	if err != nil {
		return nil, err
	}

	return newTridentCrdControllerImpl(orchestrator, tridentNamespace, kubeClientset, crdClientset)
}

// newTridentCrdControllerImpl returns a new Trident CRD controller frontend
func newTridentCrdControllerImpl(
	orchestrator core.Orchestrator, tridentNamespace string,
	kubeClientset kubernetes.Interface, crdClientset clientset.Interface,
) (*TridentCrdController, error) {

	log.WithFields(log.Fields{
		"namespace": tridentNamespace,
	}).Info("Initializing Trident CRD controller frontend.")

	crdInformerFactory := trident_informers.NewSharedInformerFactory(crdClientset, time.Second*30)
	crdInformer := trident_informers_v1.New(crdInformerFactory, tridentNamespace, nil)

	backendInformer := crdInformer.TridentBackends()
	nodeInformer := crdInformer.TridentNodes()
	storageClassInformer := crdInformer.TridentStorageClasses()
	transactionInformer := crdInformer.TridentTransactions()
	versionInformer := crdInformer.TridentVersions()
	volumeInformer := crdInformer.TridentVolumes()
	snapshotInformer := crdInformer.TridentSnapshots()

	// Create event broadcaster
	// Add our types to the default Kubernetes Scheme so Events can be logged.
	utilruntime.Must(tridentscheme.AddToScheme(scheme.Scheme))
	log.Info("Creating event broadcaster.")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(log.Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &TridentCrdController{
		orchestrator:          orchestrator,
		kubeClientset:         kubeClientset,
		crdClientset:          crdClientset,
		crdControllerStopChan: make(chan struct{}),
		crdInformerFactory:    crdInformerFactory,
		crdInformer:           crdInformer,
		backendsLister:        backendInformer.Lister(),
		backendsSynced:        backendInformer.Informer().HasSynced,
		nodesLister:           nodeInformer.Lister(),
		nodesSynced:           nodeInformer.Informer().HasSynced,
		storageClassesLister:  storageClassInformer.Lister(),
		storageClassesSynced:  storageClassInformer.Informer().HasSynced,
		transactionsLister:    transactionInformer.Lister(),
		transactionsSynced:    transactionInformer.Informer().HasSynced,
		versionsLister:        versionInformer.Lister(),
		versionsSynced:        versionInformer.Informer().HasSynced,
		volumesLister:         volumeInformer.Lister(),
		volumesSynced:         volumeInformer.Informer().HasSynced,
		snapshotsLister:       snapshotInformer.Lister(),
		snapshotsSynced:       snapshotInformer.Informer().HasSynced,
		workqueue:             workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "TridentBackends"),
		recorder:              recorder,
	}

	// Set up event handlers for when our Trident CRDs change
	log.Info("Setting up CRD event handlers.")

	// TODO RIPPY not using this right now, which means, no support for a 'kubectl edit' of a backend
	useComplicatedBackendUpdateHandling := false
	if useComplicatedBackendUpdateHandling {
		backendInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: controller.enqueueCRD,
			UpdateFunc: func(old, new interface{}) {
				newBackend := new.(*tridentv1.TridentBackend)
				oldBackend := old.(*tridentv1.TridentBackend)

				if !newBackend.ObjectMeta.DeletionTimestamp.IsZero() {
					log.WithFields(log.Fields{
						"newBackend.ResourceVersion":              newBackend.ResourceVersion,
						"newBackend.ObjectMeta.DeletionTimestamp": newBackend.ObjectMeta.DeletionTimestamp,
						"oldBackend.ResourceVersion":              oldBackend.ResourceVersion,
					}).Debug("backendInformer#UpdateFunc backend CRD object is deleting")

					b, err := controller.orchestrator.GetBackend(newBackend.BackendName)
					if err != nil {
						if core.IsNotFoundError(err) {
							// it's gone from core, has no volumes, time to let it go
							log.Debug("backendInformer#UpdateFunc no reason to keep CRD object, removing finalizers for deletion")
							controller.removeFinalizers(newBackend, false)
							return
						} else {
							log.Warnf("backendInformer#UpdateFunc could not find backend %v; error: %v", newBackend.BackendName, err.Error())
							return
						}
					}
					if b == nil {
						log.Warnf("backendInformer#UpdateFunc could not find backend %v", newBackend.BackendName)
						return
					}
					log.WithFields(log.Fields{
						"b.State.IsDeleting()":   b.State.IsDeleting(),
						"len(b.Volumes)":         len(b.Volumes),
						"newBackend.Name":        newBackend.Name,
						"newBackend.BackendName": newBackend.BackendName,
						"newBackend.BackendUUID": newBackend.BackendUUID,
						"b.Name":                 b.Name,
						"b.BackendUUID":          b.BackendUUID,
					}).Debug("backendInformer#UpdateFunc found backend object in core")
					if !b.State.IsDeleting() {
						log.Debug("backendInformer#UpdateFunc invoking deletion logic in core")
						controller.deleteCRD(newBackend)
						return
					} else {
						log.Debug("backendInformer#UpdateFunc waiting")
						return
					}
				}

				if newBackend.ResourceVersion == oldBackend.ResourceVersion {
					log.WithFields(log.Fields{
						"newBackend.ResourceVersion":              newBackend.ResourceVersion,
						"newBackend.ObjectMeta.DeletionTimestamp": newBackend.ObjectMeta.DeletionTimestamp,
						"oldBackend.ResourceVersion":              oldBackend.ResourceVersion,
					}).Debug("backendInformer#UpdateFunc nothing to do")
					return
				}

				if newBackend.CurrentState().IsFailed() {
					log.Debug("backendInformer#UpdateFunc ignoring this update because newBackend.CurrentState().IsFailed() was already processed through core")
					return
				}
				controller.enqueueCRD(new)
			},
			DeleteFunc: func(obj interface{}) {
				backend := obj.(*tridentv1.TridentBackend)
				// TODO this is called as it's about to be purged from the system (after any finalizers have been removed)
				log.WithFields(log.Fields{
					"backend.ResourceVersion":              backend.ResourceVersion,
					"backend.ObjectMeta.DeletionTimestamp": backend.ObjectMeta.DeletionTimestamp,
				}).Debug("backendInformer#DeleteFunc")
				//controller.deleteCRD(obj) // TODO not needed? it's already deleted above
			},
		})
	}

	informers := []cache.SharedIndexInformer{
		backendInformer.Informer(), // TODO what we do depends on useComplicatedBackendUpdateHandling above
		nodeInformer.Informer(),
		storageClassInformer.Informer(),
		transactionInformer.Informer(),
		versionInformer.Informer(),
		volumeInformer.Informer(),
		snapshotInformer.Informer(),
	}
	for _, informer := range informers {
		informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(oldCrd, newCrd interface{}) {
				controller.removeFinalizers(newCrd, false)
			},
		})
	}

	return controller, nil
}

func (c *TridentCrdController) Activate() error {
	log.Info("Activating CRD frontend.")
	if c.crdControllerStopChan != nil {
		c.crdInformerFactory.Start(c.crdControllerStopChan)
		go c.Run(1, c.crdControllerStopChan)
	}
	return nil
}

func (c *TridentCrdController) Deactivate() error {
	log.Info("Deactivating CRD frontend.")
	if c.crdControllerStopChan != nil {
		close(c.crdControllerStopChan)
	}
	return nil
}

func (c *TridentCrdController) GetName() string {
	return string(config.ContextCRD)
}

func (c *TridentCrdController) Version() string {
	// TODO implement me
	return "0.1"
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *TridentCrdController) Run(threadiness int, stopCh <-chan struct{}) error {
	log.WithFields(log.Fields{
		"threadiness": threadiness,
	}).Debug("TridentCrdController#Run")

	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	log.Info("Starting Trident CRD controller.")

	// Wait for the caches to be synced before starting workers
	log.Info("Waiting for informer caches to sync.")
	if ok := cache.WaitForCacheSync(stopCh,
		c.backendsSynced,
		c.nodesSynced,
		c.storageClassesSynced,
		c.transactionsSynced,
		c.versionsSynced,
		c.volumesSynced,
		c.snapshotsSynced); !ok {
		waitErr := fmt.Errorf("failed to wait for caches to sync")
		log.Errorf("Error: %v", waitErr)
		return waitErr
	}

	// Launch workers to process CRD resources
	log.Info("Starting workers.")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	log.Info("Started workers.")
	<-stopCh
	log.Info("Shutting down workers.")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *TridentCrdController) runWorker() {
	log.Debug("TridentCrdController runWorker started.")
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *TridentCrdController) processNextWorkItem() bool {

	log.Debug("TridentCrdController#processNextWorkItem")
	obj, shutdown := c.workqueue.Get()

	log.WithFields(log.Fields{
		"obj":      obj,
		"shutdown": shutdown,
	}).Debug("TridentCrdController#processNextWorkItem found")

	if shutdown {
		log.Debug("TridentCrdController#processNextWorkItem shutting down")
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {

		// We call Done here so the workqueue knows we have finished processing
		// this item. We also must remember to call Forget if we do not want
		// this work item being re-queued. For example, we do not call Forget if
		// a transient error occurs, instead the item is put back on the
		// workqueue and attempted again after a back-off period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool

		// We expect strings from the workqueue in the form namespace/name.
		// We do this as the delayed nature of the workqueue means the items in
		// the informer cache may actually be more up to date that when the item
		// was initially put onto the workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}

		// Run the syncHandler, passing it the namespace/name string of the resource to be synced.
		if err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}

		// Finally, if no error occurs we Forget this item so it does not get queued again until another change happens.
		c.workqueue.Forget(obj)
		log.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to converge the two.
// It then updates the Status block of the Foo resource with the current status of the resource.
func (c *TridentCrdController) syncHandler(key string) error {

	log.WithFields(log.Fields{
		"key": key,
	}).Debug("TridentCrdController#syncHandler")

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the CRD resource with this namespace/name
	backend, err := c.backendsLister.TridentBackends(namespace).Get(name)
	if err != nil {
		// The resource may no longer exist, in which case we stop processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("object '%s' in work queue no longer exists", key))
			return nil
		}
		return err
	}

	if !backend.ObjectMeta.DeletionTimestamp.IsZero() {
		log.WithFields(log.Fields{
			"key":                                  key,
			"backend.ObjectMeta.DeletionTimestamp": backend.ObjectMeta.DeletionTimestamp,
		}).Debug("TridentCrdController#syncHandler CRD object is being deleted, not updating.")
		// TODO refactor the "updatebackend" logic above to a shared set of code?
		//c.removeFinalizers(backend, false)
		return nil
	}

	// Finally, we update the CRD
	err = c.updateBackend(backend)
	if err != nil {
		return err
	}

	// TODO reenable this eventually?
	// it's causing trouble with unit tests; seems to work OK with real code but not sure we actually want this
	//c.recorder.Event(backend, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *TridentCrdController) updateBackend(backend *tridentv1.TridentBackend) error {

	log.WithFields(log.Fields{
		"backend.Name":                         backend.Name,
		"backend.BackendName":                  backend.BackendName,
		"backend.BackendUUID":                  backend.BackendUUID,
		"backend.State":                        backend.State,
		"backend.ObjectMeta.DeletionTimestamp": backend.ObjectMeta.DeletionTimestamp,
	}).Debug("TridentCrdController#updateBackend")

	if !backend.ObjectMeta.DeletionTimestamp.IsZero() {
		log.WithFields(log.Fields{
			"backend.Name":                         backend.Name,
			"backend.BackendName":                  backend.BackendName,
			"backend.BackendUUID":                  backend.BackendUUID,
			"backend.ObjectMeta.DeletionTimestamp": backend.ObjectMeta.DeletionTimestamp,
		}).Debug("TridentCrdController#updateBackend CRD object is being deleted, not updating.")
		// TODO refactor the "updatebackend" logic above to a shared set of code?
		//c.removeFinalizers(backend, false)
		return nil
	}

	if backend.CurrentState().IsDeleting() {
		log.WithFields(log.Fields{
			"backend.Name":                         backend.Name,
			"backend.BackendName":                  backend.BackendName,
			"backend.BackendUUID":                  backend.BackendUUID,
			"backend.ObjectMeta.DeletionTimestamp": backend.ObjectMeta.DeletionTimestamp,
		}).Debug("TridentCrdController#updateBackend CRD object is being deleted, not updating.")
		// TODO refactor the "updatebackend" logic above to a shared set of code?
		//c.removeFinalizers(backend, false)
		return nil
	}

	backendCopy := backend.DeepCopy()
	_, err := c.crdClientset.TridentV1().TridentBackends(backend.Namespace).Update(backendCopy)

	rawJSONData := backend.Config.Raw
	log.WithFields(log.Fields{
		"Name":                       backend.Name,
		"BackendName":                backend.BackendName,
		"string(backend.Config.Raw)": string(rawJSONData),
	}).Debug("Updating backend in core.")

	psbc := &storage.PersistentStorageBackendConfig{}
	err = json.Unmarshal(rawJSONData, &psbc)
	if err != nil {
		log.Errorf("could not parse JSON backend configuration: %v", err)
		return err
	}

	var marshalErr error
	var configJSON []byte
	if psbc.AWSConfig != nil {
		configJSON, marshalErr = json.Marshal(psbc.AWSConfig)
	} else if psbc.EseriesConfig != nil {
		configJSON, marshalErr = json.Marshal(psbc.EseriesConfig)
	} else if psbc.FakeStorageDriverConfig != nil {
		configJSON, marshalErr = json.Marshal(psbc.FakeStorageDriverConfig)
	} else if psbc.OntapConfig != nil {
		configJSON, marshalErr = json.Marshal(psbc.OntapConfig)
	} else if psbc.SolidfireConfig != nil {
		configJSON, marshalErr = json.Marshal(psbc.SolidfireConfig)
	} else {
		err = fmt.Errorf("problem parsing JSON backend configuration")
		log.Errorf("%v", err)
		return err
	}
	if marshalErr != nil {
		log.Errorf("could not marshall JSON backend configuration: %v", marshalErr)
		return marshalErr
	}

	log.WithFields(log.Fields{
		"configJSON": string(configJSON),
	}).Debug("Parsed into...")

	result, err := c.orchestrator.UpdateBackendByBackendUUID(backend.BackendName, string(configJSON), backend.BackendUUID)

	log.WithFields(log.Fields{
		"err":     err,
		"backend": result,
	}).Debug("Result")

	return err
}

// enqueueCRD takes a Foo resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Foo.
func (c *TridentCrdController) enqueueCRD(obj interface{}) {

	log.WithFields(log.Fields{
		"obj": obj,
	}).Debug("TridentCrdController#enqueueCRD")

	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

func (c *TridentCrdController) deleteCRD(obj interface{}) {

	backend := obj.(*tridentv1.TridentBackend)

	log.WithFields(log.Fields{
		"backend.ResourceVersion":              backend.ResourceVersion,
		"backend.ObjectMeta.DeletionTimestamp": backend.ObjectMeta.DeletionTimestamp,
	}).Debug("TridentCrdController#deleteCRD")

	if backend.ObjectMeta.DeletionTimestamp.IsZero() {
		// TODO shouldn't happen?
		log.Info("deleteCRD DeletionTimestamp is zero, should not happen???")
		return
	}

	if _, getBackendErr := c.orchestrator.GetBackend(backend.BackendName); core.IsNotFoundError(getBackendErr) {
		log.WithFields(log.Fields{
			"BackendName": backend.BackendName,
		}).Warn("Could not find backend.")
	} else {
		log.WithFields(log.Fields{
			"BackendName": backend.BackendName,
		}).Debug("Deleting backend.")
		if deleteBackendErr := c.orchestrator.DeleteBackend(backend.BackendName); core.IsNotFoundError(deleteBackendErr) {
			log.WithFields(log.Fields{
				"BackendName": backend.BackendName,
			}).Warn("Could not find backend.")
		}
	}
}

// removeFinalizers removes Trident's finalizers from Trident CRD objects
func (c *TridentCrdController) removeFinalizers(obj interface{}, force bool) {
	/*
		log.WithFields(log.Fields{
			"obj":   obj,
			"force": force,
		}).Debug("removeFinalizers")
	*/

	switch crd := obj.(type) {
	case *tridentv1.TridentBackend:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			c.removeBackendFinalizers(crd)
		}
	case *tridentv1.TridentNode:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			c.removeNodeFinalizers(crd)
		}
	case *tridentv1.TridentStorageClass:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			c.removeStorageClassFinalizers(crd)
		}
	case *tridentv1.TridentTransaction:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			c.removeTransactionFinalizers(crd)
		}
	case *tridentv1.TridentVersion:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			c.removeVersionFinalizers(crd)
		}
	case *tridentv1.TridentVolume:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			c.removeVolumeFinalizers(crd)
		}
	case *tridentv1.TridentSnapshot:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			c.removeSnapshotFinalizers(crd)
		}
	default:
		log.Warnf("unexpected type %T", crd)
	}
}

// removeBackendFinalizers removes Trident's finalizers from TridentBackend CRD objects
func (c *TridentCrdController) removeBackendFinalizers(backend *tridentv1.TridentBackend) {
	log.WithFields(log.Fields{
		"backend.ResourceVersion":              backend.ResourceVersion,
		"backend.ObjectMeta.DeletionTimestamp": backend.ObjectMeta.DeletionTimestamp,
	}).Debug("removeBackendFinalizers")

	if backend.HasTridentFinalizers() {
		log.Debug("Has finalizers, removing them.")
		backendCopy := backend.DeepCopy()
		backendCopy.RemoveTridentFinalizers()
		_, err := c.crdClientset.TridentV1().TridentBackends(backend.Namespace).Update(backendCopy)
		if err != nil {
			log.Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		log.Debug("No finalizers to remove.")
	}
}

// removeNodeFinalizers removes Trident's finalizers from TridentNode CRD objects
func (c *TridentCrdController) removeNodeFinalizers(node *tridentv1.TridentNode) {
	log.WithFields(log.Fields{
		"node.ResourceVersion":              node.ResourceVersion,
		"node.ObjectMeta.DeletionTimestamp": node.ObjectMeta.DeletionTimestamp,
	}).Debug("removeNodeFinalizers")

	if node.HasTridentFinalizers() {
		log.Debug("Has finalizers, removing them.")
		nodeCopy := node.DeepCopy()
		nodeCopy.RemoveTridentFinalizers()
		_, err := c.crdClientset.TridentV1().TridentNodes(node.Namespace).Update(nodeCopy)
		if err != nil {
			log.Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		log.Debug("No finalizers to remove.")
	}
}

// removeStorageClassFinalizers removes Trident's finalizers from TridentStorageClass CRD objects
func (c *TridentCrdController) removeStorageClassFinalizers(sc *tridentv1.TridentStorageClass) {
	log.WithFields(log.Fields{
		"sc.ResourceVersion":              sc.ResourceVersion,
		"sc.ObjectMeta.DeletionTimestamp": sc.ObjectMeta.DeletionTimestamp,
	}).Debug("removeStorageClassFinalizers")

	if sc.HasTridentFinalizers() {
		log.Debug("Has finalizers, removing them.")
		scCopy := sc.DeepCopy()
		scCopy.RemoveTridentFinalizers()
		_, err := c.crdClientset.TridentV1().TridentStorageClasses(sc.Namespace).Update(scCopy)
		if err != nil {
			log.Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		log.Debug("No finalizers to remove.")
	}
}

// removeTransactionFinalizers removes Trident's finalizers from TridentTransaction CRD objects
func (c *TridentCrdController) removeTransactionFinalizers(tx *tridentv1.TridentTransaction) {
	log.WithFields(log.Fields{
		"tx.ResourceVersion":              tx.ResourceVersion,
		"tx.ObjectMeta.DeletionTimestamp": tx.ObjectMeta.DeletionTimestamp,
	}).Debug("removeTransactionFinalizers")

	if tx.HasTridentFinalizers() {
		log.Debug("Has finalizers, removing them.")
		txCopy := tx.DeepCopy()
		txCopy.RemoveTridentFinalizers()
		_, err := c.crdClientset.TridentV1().TridentTransactions(tx.Namespace).Update(txCopy)
		if err != nil {
			log.Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		log.Debug("No finalizers to remove.")
	}
}

// removeVersionFinalizers removes Trident's finalizers from TridentVersion CRD objects
func (c *TridentCrdController) removeVersionFinalizers(v *tridentv1.TridentVersion) {
	log.WithFields(log.Fields{
		"v.ResourceVersion":              v.ResourceVersion,
		"v.ObjectMeta.DeletionTimestamp": v.ObjectMeta.DeletionTimestamp,
	}).Debug("removeVersionFinalizers")

	if v.HasTridentFinalizers() {
		log.Debug("Has finalizers, removing them.")
		vCopy := v.DeepCopy()
		vCopy.RemoveTridentFinalizers()
		_, err := c.crdClientset.TridentV1().TridentVersions(v.Namespace).Update(vCopy)
		if err != nil {
			log.Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		log.Debug("No finalizers to remove.")
	}
}

// removeVolumeFinalizers removes Trident's finalizers from TridentVolume CRD objects
func (c *TridentCrdController) removeVolumeFinalizers(vol *tridentv1.TridentVolume) {
	log.WithFields(log.Fields{
		"vol.ResourceVersion":              vol.ResourceVersion,
		"vol.ObjectMeta.DeletionTimestamp": vol.ObjectMeta.DeletionTimestamp,
	}).Debug("removeVolumeFinalizers")

	if vol.HasTridentFinalizers() {
		log.Debug("Has finalizers, removing them.")
		volCopy := vol.DeepCopy()
		volCopy.RemoveTridentFinalizers()
		_, err := c.crdClientset.TridentV1().TridentVolumes(vol.Namespace).Update(volCopy)
		if err != nil {
			log.Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		log.Debug("No finalizers to remove.")
	}
}

// removeSnapshotFinalizers removes Trident's finalizers from TridentSnapshot CRD objects
func (c *TridentCrdController) removeSnapshotFinalizers(snap *tridentv1.TridentSnapshot) {
	log.WithFields(log.Fields{
		"snap.ResourceVersion":              snap.ResourceVersion,
		"snap.ObjectMeta.DeletionTimestamp": snap.ObjectMeta.DeletionTimestamp,
	}).Debug("removeSnapshotFinalizers")

	if snap.HasTridentFinalizers() {
		log.Debug("Has finalizers, removing them.")
		snapCopy := snap.DeepCopy()
		snapCopy.RemoveTridentFinalizers()
		_, err := c.crdClientset.TridentV1().TridentSnapshots(snap.Namespace).Update(snapCopy)
		if err != nil {
			log.Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		log.Debug("No finalizers to remove.")
	}
}
