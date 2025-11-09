// Copyright 2023 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"time"

	k8ssnapshots "github.com/kubernetes-csi/external-snapshotter/client/v8/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	goinformer "k8s.io/client-go/informers"
	goinformerv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	v1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	clik8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/core"
	"github.com/netapp/trident/frontend/crd/indexers"
	. "github.com/netapp/trident/logging"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentv1clientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	tridentscheme "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/scheme"
	tridentinformers "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions"
	tridentinformersv1 "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions/netapp/v1"
	listers "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	"github.com/netapp/trident/utils/errors"
)

type (
	EventType  string
	ObjectType string
)

const (
	EventAdd         EventType = "add"
	EventUpdate      EventType = "update"
	EventForceUpdate EventType = "forceupdate"
	EventDelete      EventType = "delete"

	ObjectTypeTridentBackendConfig         string = "TridentBackendConfig"
	ObjectTypeTridentBackend               string = "TridentBackend"
	ObjectTypeSecret                       string = "secret"
	ObjectTypeTridentMirrorRelationship    string = "TridentMirrorRelationship"
	ObjectTypeTridentActionMirrorUpdate    string = "TridentActionMirrorUpdate"
	ObjectTypeTridentSnapshotInfo          string = "TridentSnapshotInfo"
	ObjectTypeTridentActionSnapshotRestore string = "TridentActionSnapshotRestore"
	ObjectTypeTridentNodeRemediation       string = "TridentNodeRemediation"

	OperationStatusSuccess string = "Success"
	OperationStatusFailed  string = "Failed"

	controllerName         = "crd"
	controllerAgentName    = "trident-crd-controller"
	crdControllerQueueName = "trident-crd-workqueue"

	transactionSyncPeriod = 60 * time.Second
)

type KeyItem struct {
	key        string
	objectType string
	event      EventType
	ctx        context.Context
	isRetry    bool
}

var (
	listOpts   = metav1.ListOptions{}
	getOpts    = metav1.GetOptions{}
	createOpts = metav1.CreateOptions{}
	updateOpts = metav1.UpdateOptions{}
)

func Logx(ctx context.Context) LogEntry {
	return Logc(ctx).WithField(LogSource, "trident-crd-controller")
}

// TridentCrdController is the controller implementation for Trident's CRD resources
type TridentCrdController struct {
	// orchestrator is a reference to the core orchestrator
	orchestrator core.Orchestrator

	// kubeClientset is a standard kubernetes clientset
	kubeClientset     kubernetes.Interface
	snapshotClientSet k8ssnapshots.Interface

	// crdClientset is a clientset for our own API group
	crdClientset tridentv1clientset.Interface

	crdControllerStopChan chan struct{}
	crdInformerFactory    tridentinformers.SharedInformerFactory
	crdInformer           tridentinformersv1.Interface

	txnInformerFactory tridentinformers.SharedInformerFactory
	txnInformer        tridentinformersv1.Interface

	kubeInformerFactory goinformer.SharedInformerFactory
	kubeInformer        goinformerv1.Interface

	// TridentBackend CRD handling
	backendsLister listers.TridentBackendLister
	backendsSynced cache.InformerSynced

	// TridentBackendConfig CRD handling
	backendConfigsLister listers.TridentBackendConfigLister
	backendConfigsSynced cache.InformerSynced

	// TridentMirrorRelationship CRD handling
	mirrorLister listers.TridentMirrorRelationshipLister
	mirrorSynced cache.InformerSynced

	// TridentActionMirrorUpdate CRD handling
	actionMirrorUpdateLister listers.TridentActionMirrorUpdateLister
	actionMirrorUpdateSynced cache.InformerSynced

	// TridentSnapshotInfo CRD handling
	snapshotInfoLister listers.TridentSnapshotInfoLister
	snapshotInfoSynced cache.InformerSynced

	// TridentNode CRD handling
	nodesLister listers.TridentNodeLister
	nodesSynced cache.InformerSynced

	// TridentNodeRemediation CRD handling
	nodeRemediationLister listers.TridentNodeRemediationLister
	nodeRemediationSynced cache.InformerSynced
	nodeRemediationUtils  NodeRemediationUtils

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

	// TridentVolumePublication CRD handling
	volumePublicationsLister listers.TridentVolumePublicationLister
	volumePublicationsSynced cache.InformerSynced

	// TridentSnapshot CRD handling
	snapshotsLister listers.TridentSnapshotLister
	snapshotsSynced cache.InformerSynced

	// Secret handling
	secretsLister v1.SecretLister
	secretsSynced cache.InformerSynced

	// TridentSnapshot CRD handling
	actionSnapshotRestoreLister listers.TridentActionSnapshotRestoreLister
	actionSnapshotRestoreSynced cache.InformerSynced

	// K8s Indexers
	indexers indexers.Indexers

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface

	// recorder is an event recorder for recording Event resources to the Kubernetes API.
	recorder                  record.EventRecorder
	actionMirrorUpdatesSynced func() bool
	enableForceDetach         bool
}

// NewTridentCrdController returns a new Trident CRD controller frontend
func NewTridentCrdController(
	orchestrator core.Orchestrator, masterURL, kubeConfigPath string,
) (*TridentCrdController, error) {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowCRDControllerCreate, LogLayerCRDFrontend)

	Logx(ctx).Trace("Creating CRDv1 controller.")

	clients, err := clik8sclient.CreateK8SClients(masterURL, kubeConfigPath, "")
	if err != nil {
		return nil, err
	}

	indexers := indexers.NewIndexers(clients.KubeClient)
	indexers.Activate()
	remediationUtils := NewNodeRemediationUtils(clients.KubeClient, orchestrator, indexers)

	return newTridentCrdControllerImpl(orchestrator, clients.Namespace, clients.KubeClient,
		clients.SnapshotClient, clients.TridentClient, indexers, remediationUtils)
}

// newTridentCrdControllerImpl returns a new Trident CRD controller frontend
func newTridentCrdControllerImpl(
	orchestrator core.Orchestrator, tridentNamespace string, kubeClientset kubernetes.Interface,
	snapshotClientset k8ssnapshots.Interface, crdClientset tridentv1clientset.Interface, indexers indexers.Indexers,
	nodeRemediationUtils NodeRemediationUtils,
) (*TridentCrdController, error) {
	ctx := GenerateRequestContext(nil, "", "", WorkflowNone, LogLayerCRDFrontend)
	Logx(ctx).WithFields(LogFields{
		"namespace": tridentNamespace,
	}).Trace("Initializing Trident CRD controller frontend.")

	// Set resync to 0 sec so that reconciliation is on demand
	crdInformerFactory := tridentinformers.NewSharedInformerFactory(crdClientset, time.Second*0)
	crdInformer := tridentinformersv1.New(crdInformerFactory, tridentNamespace, nil)
	allNSCrdInformer := tridentinformersv1.New(crdInformerFactory, corev1.NamespaceAll, nil)

	// Set resync to 60 seconds so that transactions never get stuck in place by finalizers
	txnInformerFactory := tridentinformers.NewSharedInformerFactory(crdClientset, transactionSyncPeriod)
	txnInformer := tridentinformersv1.New(txnInformerFactory, tridentNamespace, nil)

	// Set resync to 0 sec so that reconciliation is on demand
	kubeInformerFactory := goinformer.NewSharedInformerFactory(kubeClientset, time.Second*0)
	kubeInformer := goinformerv1.New(kubeInformerFactory, tridentNamespace, nil)

	backendInformer := crdInformer.TridentBackends()
	backendConfigInformer := crdInformer.TridentBackendConfigs()
	mirrorInformer := allNSCrdInformer.TridentMirrorRelationships()
	actionMirrorUpdateInformer := allNSCrdInformer.TridentActionMirrorUpdates()
	snapshotInfoInformer := allNSCrdInformer.TridentSnapshotInfos()
	nodeInformer := crdInformer.TridentNodes()
	nodeRemediationInformer := crdInformer.TridentNodeRemediations()
	storageClassInformer := crdInformer.TridentStorageClasses()
	transactionInformer := txnInformer.TridentTransactions()
	versionInformer := crdInformer.TridentVersions()
	volumeInformer := crdInformer.TridentVolumes()
	volumePublicationInformer := crdInformer.TridentVolumePublications()
	snapshotInformer := crdInformer.TridentSnapshots()
	secretInformer := kubeInformer.Secrets()
	actionSnapshotRestoreInformer := allNSCrdInformer.TridentActionSnapshotRestores()

	// Create event broadcaster
	// Add our types to the default Kubernetes Scheme so Events can be logged.
	utilruntime.Must(tridentscheme.AddToScheme(scheme.Scheme))
	Logx(ctx).Trace("Creating event broadcaster.")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &TridentCrdController{
		orchestrator:                orchestrator,
		kubeClientset:               kubeClientset,
		snapshotClientSet:           snapshotClientset,
		crdClientset:                crdClientset,
		crdControllerStopChan:       make(chan struct{}),
		crdInformerFactory:          crdInformerFactory,
		crdInformer:                 crdInformer,
		txnInformerFactory:          txnInformerFactory,
		txnInformer:                 txnInformer,
		kubeInformerFactory:         kubeInformerFactory,
		kubeInformer:                kubeInformer,
		backendsLister:              backendInformer.Lister(),
		backendsSynced:              backendInformer.Informer().HasSynced,
		backendConfigsLister:        backendConfigInformer.Lister(),
		backendConfigsSynced:        backendConfigInformer.Informer().HasSynced,
		mirrorLister:                mirrorInformer.Lister(),
		mirrorSynced:                mirrorInformer.Informer().HasSynced,
		actionMirrorUpdateLister:    actionMirrorUpdateInformer.Lister(),
		actionMirrorUpdatesSynced:   actionMirrorUpdateInformer.Informer().HasSynced,
		snapshotInfoLister:          snapshotInfoInformer.Lister(),
		snapshotInfoSynced:          snapshotInfoInformer.Informer().HasSynced,
		nodesLister:                 nodeInformer.Lister(),
		nodesSynced:                 nodeInformer.Informer().HasSynced,
		nodeRemediationLister:       nodeRemediationInformer.Lister(),
		nodeRemediationSynced:       nodeRemediationInformer.Informer().HasSynced,
		storageClassesLister:        storageClassInformer.Lister(),
		storageClassesSynced:        storageClassInformer.Informer().HasSynced,
		transactionsLister:          transactionInformer.Lister(),
		transactionsSynced:          transactionInformer.Informer().HasSynced,
		versionsLister:              versionInformer.Lister(),
		versionsSynced:              versionInformer.Informer().HasSynced,
		volumesLister:               volumeInformer.Lister(),
		volumesSynced:               volumeInformer.Informer().HasSynced,
		volumePublicationsLister:    volumePublicationInformer.Lister(),
		volumePublicationsSynced:    volumePublicationInformer.Informer().HasSynced,
		snapshotsLister:             snapshotInformer.Lister(),
		snapshotsSynced:             snapshotInformer.Informer().HasSynced,
		secretsLister:               secretInformer.Lister(),
		secretsSynced:               secretInformer.Informer().HasSynced,
		actionSnapshotRestoreLister: actionSnapshotRestoreInformer.Lister(),
		actionSnapshotRestoreSynced: actionSnapshotRestoreInformer.Informer().HasSynced,
		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(),
			crdControllerQueueName),
		recorder:             recorder,
		indexers:             indexers,
		nodeRemediationUtils: nodeRemediationUtils,
	}

	// Set up event handlers for when a Trident CRs are added, updated, or deleted
	Logx(ctx).Trace("Setting up CRD controller event handlers.")

	_, _ = backendConfigInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addCRHandler,
		UpdateFunc: controller.updateCRHandler,
		DeleteFunc: controller.deleteCRHandler,
	})

	_, _ = backendInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// Do not handle add backends here, otherwise it may result in continuous
		// reconcile loops esp. in cases where backends are created as a result
		// of a backend config.
		UpdateFunc: controller.updateTridentBackendHandler,
		DeleteFunc: controller.deleteTridentBackendHandler,
	})

	_, _ = mirrorInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addCRHandler,
		UpdateFunc: controller.updateTMRHandler,
		DeleteFunc: controller.deleteCRHandler,
	})

	_, _ = actionMirrorUpdateInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.addCRHandler,
	})

	_, _ = nodeRemediationInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addCRHandler,
		UpdateFunc: controller.updateCRHandler,
		DeleteFunc: controller.deleteCRHandler,
	})

	_, _ = snapshotInfoInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addCRHandler,
		UpdateFunc: controller.updateCRHandler,
		DeleteFunc: controller.deleteCRHandler,
	})

	_, _ = actionSnapshotRestoreInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.addCRHandler,
	})

	_, _ = secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// Do not handle AddFunc here otherwise everytime trident is restarted,
		// there will be unwarranted reconciles and backend initializations
		UpdateFunc: controller.updateSecretHandler,
		// Do not handle DeleteFunc here as these are user created secrets,
		// Trident need not do anything, if there are backends using the secret
		// they will continue to function using the credentials detains in memory
	})

	// For the following CRDs, we only care to remove the Trident finalizer when deletion timestamp is set
	informers := []cache.SharedIndexInformer{
		nodeInformer.Informer(),
		storageClassInformer.Informer(),
		versionInformer.Informer(),
		volumeInformer.Informer(),
		volumePublicationInformer.Informer(),
		snapshotInformer.Informer(),
		transactionInformer.Informer(),
	}
	for _, informer := range informers {
		_, _ = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(oldCrd, newCrd interface{}) {
				ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
				if err := controller.removeFinalizers(ctx, newCrd, false); err != nil {
					Logx(ctx).WithError(err).Error("Error removing finalizers")
				}
			},
		})
	}

	return controller, nil
}

func (c *TridentCrdController) Activate() error {
	ctx := GenerateRequestContext(nil, "", "", WorkflowNone, LogLayerCRDFrontend)
	Logx(ctx).Info("Activating CRD frontend.")
	if c.crdControllerStopChan != nil {
		c.crdInformerFactory.Start(c.crdControllerStopChan)
		c.txnInformerFactory.Start(c.crdControllerStopChan)
		c.kubeInformerFactory.Start(c.crdControllerStopChan)
		go c.Run(ctx, 1, c.crdControllerStopChan)
	}
	return nil
}

func (c *TridentCrdController) Deactivate() error {
	ctx := GenerateRequestContext(nil, "", "", WorkflowNone, LogLayerCRDFrontend)
	Logx(ctx).Info("Deactivating CRD frontend.")
	if c.crdControllerStopChan != nil {
		close(c.crdControllerStopChan)
	}
	c.indexers.Deactivate()
	return nil
}

func (c *TridentCrdController) GetName() string {
	return controllerName
}

func (c *TridentCrdController) Version() string {
	return config.DefaultOrchestratorVersion
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *TridentCrdController) Run(ctx context.Context, threadiness int, stopCh <-chan struct{}) {
	Logx(ctx).WithFields(LogFields{
		"threadiness": threadiness,
	}).Trace("TridentCrdController#Run")

	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	Log().Info("Starting Trident CRD controller.")

	// Wait for the caches to be synced before starting workers
	Logx(ctx).Trace("Waiting for informer caches to sync.")
	if ok := cache.WaitForCacheSync(stopCh,
		c.backendsSynced,
		c.backendConfigsSynced,
		c.nodesSynced,
		c.storageClassesSynced,
		c.transactionsSynced,
		c.versionsSynced,
		c.volumesSynced,
		c.volumePublicationsSynced,
		c.mirrorSynced,
		c.snapshotsSynced,
		c.snapshotInfoSynced,
		c.secretsSynced); !ok {
		waitErr := fmt.Errorf("failed to wait for caches to sync")
		Logx(ctx).Errorf("Error: %v", waitErr)
		return
	}

	// Launch workers to process CRD resources
	Logx(ctx).Debug("Starting workers.")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	Logx(ctx).Debug("Started workers.")
	<-stopCh
	Logx(ctx).Debug("Shutting down workers.")
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *TridentCrdController) runWorker() {
	ctx := GenerateRequestContext(nil, "", "", WorkflowNone, LogLayerCRDFrontend)
	Logx(ctx).Trace("TridentCrdController runWorker started.")
	for c.processNextWorkItem() {
	}
}

func (c *TridentCrdController) addEventToWorkqueue(key string, event EventType, ctx context.Context, objKind string) {
	keyItem := KeyItem{
		key:        key,
		event:      event,
		ctx:        ctx,
		objectType: objKind,
	}

	c.workqueue.Add(keyItem)
	fields := LogFields{
		"key":   key,
		"kind":  objKind,
		"event": event,
	}
	Logx(ctx).WithFields(fields).Debug("Added CR event to workqueue.")
}

// addCRHandler is the add handler for CR watchers.
func (c *TridentCrdController) addCRHandler(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventAdd))

	Logx(ctx).Debug("TridentCrdController#addCRHandler")

	cr, ok := obj.(tridentv1.TridentCRD)
	if !ok {
		Logx(ctx).Errorf("Incorrect type (%T) provided to addCRHandler, cannot process CR", obj)
		return
	}

	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		Logx(ctx).Error(err)
		return
	}

	c.addEventToWorkqueue(key, EventAdd, ctx, cr.GetKind())
}

// updateCRHandler is the update handler for CR watchers.
func (c *TridentCrdController) updateCRHandler(old, new interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventUpdate))

	Logx(ctx).Debug("TridentCrdController#updateCRHandler")

	if new == nil || old == nil {
		Logx(ctx).Warn("No updated CR provided, skipping update")
		return
	}

	oldCR, ok := old.(tridentv1.TridentCRD)
	if !ok {
		Logx(ctx).Errorf("Incorrect type (%T) provided to updateCRHandler, cannot process old CR", old)
		return
	}
	newCR, ok := new.(tridentv1.TridentCRD)
	if !ok {
		Logx(ctx).Errorf("Incorrect type (%T) provided to updateCRHandler, cannot process new CR", new)
		return
	}

	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(new); err != nil {
		Logx(ctx).Error(err)
		return
	}

	newCRMeta := newCR.GetObjectMeta()
	oldCRMeta := oldCR.GetObjectMeta()

	// Ignore metadata and status only updates
	if newCRMeta.GetGeneration() == oldCRMeta.GetGeneration() &&
		newCRMeta.GetGeneration() != 0 &&
		newCRMeta.DeletionTimestamp.IsZero() { // CR was deleted and need to remove finalizers
		logFields := LogFields{
			"name":          newCRMeta.GetName(),
			"oldGeneration": oldCRMeta.GetGeneration(),
			"newGeneration": newCRMeta.GetGeneration(),
		}

		Logx(ctx).WithFields(logFields).Trace("No required update for CR")
		return
	}

	c.addEventToWorkqueue(key, EventUpdate, ctx, newCR.GetKind())
}

// deleteCRHandler is the delete handler for CR watchers.
func (c *TridentCrdController) deleteCRHandler(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventDelete))

	Logx(ctx).Debug("TridentCrdController#deleteCRHandler")

	cr, ok := obj.(tridentv1.TridentCRD)
	if !ok {
		Logx(ctx).Errorf("Incorrect type (%T) provided to deleteCRHandler, cannot process CR", obj)
		return
	}

	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		Logx(ctx).Error(err)
		return
	}

	c.addEventToWorkqueue(key, EventDelete, ctx, cr.GetKind())
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the reconcileBackendConfig.
func (c *TridentCrdController) processNextWorkItem() bool {
	ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	Logx(ctx).Trace("TridentCrdController#processNextWorkItem")

	obj, shutdown := c.workqueue.Get()

	if shutdown {
		Logx(ctx).Trace("TridentCrdController#processNextWorkItem shutting down")
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
		var keyItem KeyItem
		var ok bool

		// We expect strings from the workqueue in the form namespace/name.
		// We do this as the delayed nature of the workqueue means the items in
		// the informer cache may actually be more up to date that when the item
		// was initially put onto the workqueue.
		if keyItem, ok = obj.(KeyItem); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			Logx(ctx).Errorf("expected KeyItem in workqueue but got %#v", obj)

			return nil
		}

		if keyItem == (KeyItem{}) {
			c.workqueue.Forget(obj)
			return fmt.Errorf("keyItem item is empty")
		}

		keyItemName := keyItem.key
		var handleFunction func(*KeyItem) error
		switch keyItem.objectType {
		case ObjectTypeTridentBackend:
			handleFunction = c.handleTridentBackend
		case ObjectTypeSecret:
			handleFunction = c.handleSecret
			keyItemName = "<REDACTED>"
		case ObjectTypeTridentBackendConfig:
			handleFunction = c.handleTridentBackendConfig
		case ObjectTypeTridentMirrorRelationship:
			handleFunction = c.handleTridentMirrorRelationship
		case ObjectTypeTridentActionMirrorUpdate:
			handleFunction = c.handleActionMirrorUpdate
		case ObjectTypeTridentSnapshotInfo:
			handleFunction = c.handleTridentSnapshotInfo
		case ObjectTypeTridentActionSnapshotRestore:
			handleFunction = c.handleActionSnapshotRestore
		case ObjectTypeTridentNodeRemediation:
			handleFunction = c.handleTridentNodeRemediation
		default:
			return fmt.Errorf("unknown objectType in the workqueue: %v", keyItem.objectType)
		}
		if handleFunction != nil {
			if err := handleFunction(&keyItem); err != nil {
				if errors.IsUnsupportedConfigError(err) {
					errMessage := fmt.Sprintf("found unsupported configuration, "+
						"needs manual intervention to fix the issue; "+
						"error syncing '%v', not requeuing", keyItem.key)

					c.workqueue.Forget(keyItem)

					Logx(keyItem.ctx).Errorf(errMessage)
					Log().Info("-------------------------------------------------")
					Log().Info("-------------------------------------------------")

					return fmt.Errorf("%s; %w", errMessage, err)
				} else if errors.IsUnlicensedError(err) {
					errMessage := fmt.Sprintf("licensed workflow requires ACP, "+
						"needs manual intervention to fix the issue; "+
						"error syncing '%v', not requeuing", keyItem.key)

					c.workqueue.Forget(keyItem)

					Logx(keyItem.ctx).WithError(err).Errorf(errMessage)
					Log().Info("-------------------------------------------------")
					Log().Info("-------------------------------------------------")

					return fmt.Errorf("%s; %w", errMessage, err)
				} else if errors.IsReconcileDeferredError(err) {
					// If it is a deferred error, then do not remove the object from the queue and retry in due time
					errMessage := fmt.Sprintf("deferred syncing %v '%v', requeuing; %v", keyItem.objectType,
						keyItem.key, err.Error())
					Logx(keyItem.ctx).Info(errMessage)
					keyItem.isRetry = true
					c.workqueue.AddRateLimited(keyItem)
					return nil
				} else if errors.IsReconcileIncompleteError(err) {
					// If it is a reconcile incomplete error, then do not remove the object from the queue and retry immediately
					errMessage := fmt.Sprintf("error syncing %v '%v', requeuing; %v", keyItem.objectType, keyItem.key,
						err.Error())
					Logx(keyItem.ctx).Error(errMessage)
					keyItem.isRetry = true
					c.workqueue.Add(keyItem)
					return nil
				} else {
					return err
				}
			}
		}

		// Finally, we clear the rate limiter for this item
		c.workqueue.Forget(obj)
		Logx(keyItem.ctx).Tracef("Synced '%s'", keyItemName)
		Log().Info("-------------------------------------------------")
		Log().Info("-------------------------------------------------")

		return nil
	}(obj)
	if err != nil {
		Logx(ctx).Error(err)
		return true
	}

	return true
}

// getTridentBackend returns a TridentBackend CR with the given backendUUID
func (c *TridentCrdController) getTridentBackend(
	ctx context.Context, namespace, backendUUID string,
) (*tridentv1.TridentBackend, error) {
	// Get list of all the backends
	backendList, err := c.crdClientset.TridentV1().TridentBackends(namespace).List(ctx, listOpts)
	if err != nil {
		Logx(ctx).WithFields(LogFields{
			"backendUUID": backendUUID,
			"namespace":   namespace,
			"err":         err,
		}).Errorf("Error listing Trident backends.")
		return nil, fmt.Errorf("error listing Trident backends: %v", err)
	}

	for _, backend := range backendList.Items {
		if backend.BackendUUID == backendUUID {
			return backend, nil
		}
	}

	return nil, nil
}

// removeFinalizers removes Trident's finalizers from Trident CRs
func (c *TridentCrdController) removeFinalizers(ctx context.Context, obj interface{}, force bool) error {
	Logx(ctx).WithFields(LogFields{
		"obj":   obj,
		"force": force,
	}).Trace("removeFinalizers")

	switch crd := obj.(type) {
	case *tridentv1.TridentBackend:
		// nothing to do
		return nil
	case *tridentv1.TridentBackendConfig:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			return c.removeBackendConfigFinalizers(ctx, crd)
		}
	case *tridentv1.TridentNode:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			return c.removeNodeFinalizers(ctx, crd)
		}
	case *tridentv1.TridentStorageClass:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			return c.removeStorageClassFinalizers(ctx, crd)
		}
	case *tridentv1.TridentTransaction:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			return c.removeTransactionFinalizers(ctx, crd)
		}
	case *tridentv1.TridentVersion:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			return c.removeVersionFinalizers(ctx, crd)
		}
	case *tridentv1.TridentVolume:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			return c.removeVolumeFinalizers(ctx, crd)
		}
	case *tridentv1.TridentVolumePublication:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			return c.removeVolumePublicationFinalizers(ctx, crd)
		}
	case *tridentv1.TridentSnapshot:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			return c.removeSnapshotFinalizers(ctx, crd)
		}
	case *tridentv1.TridentMirrorRelationship:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			return c.removeTMRFinalizers(ctx, crd)
		}
	case *tridentv1.TridentSnapshotInfo:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			return c.removeTSIFinalizers(ctx, crd)
		}
	case *tridentv1.TridentNodeRemediation:
		if force || !crd.ObjectMeta.DeletionTimestamp.IsZero() {
			return c.removeTNRFinalizers(ctx, crd)
		}
	default:
		Logx(ctx).Warnf("unexpected type %T", crd)
		return fmt.Errorf("unexpected type %T", crd)
	}

	return nil
}

// removeBackendFinalizers removes Trident's finalizers from TridentBackend CRs
func (c *TridentCrdController) removeBackendFinalizers(
	ctx context.Context, backend *tridentv1.TridentBackend,
) (err error) {
	Logx(ctx).WithFields(LogFields{
		"backend.ResourceVersion":              backend.ResourceVersion,
		"backend.ObjectMeta.DeletionTimestamp": backend.ObjectMeta.DeletionTimestamp,
	}).Trace("removeBackendFinalizers")

	if backend.HasTridentFinalizers() {
		Logx(ctx).Trace("Has finalizers, removing them.")
		backendCopy := backend.DeepCopy()
		backendCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentBackends(backend.Namespace).Update(ctx, backendCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Trace("No finalizers to remove.")
	}

	return
}

// removeNodeFinalizers removes Trident's finalizers from TridentNode CRs
func (c *TridentCrdController) removeNodeFinalizers(ctx context.Context, node *tridentv1.TridentNode) (err error) {
	Logx(ctx).WithFields(LogFields{
		"node.ResourceVersion":              node.ResourceVersion,
		"node.ObjectMeta.DeletionTimestamp": node.ObjectMeta.DeletionTimestamp,
	}).Trace("removeNodeFinalizers")

	if node.HasTridentFinalizers() {
		Logx(ctx).Trace("Has finalizers, removing them.")
		nodeCopy := node.DeepCopy()
		nodeCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentNodes(node.Namespace).Update(ctx, nodeCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Trace("No finalizers to remove.")
	}

	return
}

// removeStorageClassFinalizers removes Trident's finalizers from TridentStorageClass CRs
func (c *TridentCrdController) removeStorageClassFinalizers(
	ctx context.Context, sc *tridentv1.TridentStorageClass,
) (err error) {
	Logx(ctx).WithFields(LogFields{
		"sc.ResourceVersion":              sc.ResourceVersion,
		"sc.ObjectMeta.DeletionTimestamp": sc.ObjectMeta.DeletionTimestamp,
	}).Trace("removeStorageClassFinalizers")

	if sc.HasTridentFinalizers() {
		Logx(ctx).Trace("Has finalizers, removing them.")
		scCopy := sc.DeepCopy()
		scCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentStorageClasses(sc.Namespace).Update(ctx, scCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Trace("No finalizers to remove.")
	}

	return
}

// removeTransactionFinalizers removes Trident's finalizers from TridentTransaction CRs
func (c *TridentCrdController) removeTransactionFinalizers(
	ctx context.Context, tx *tridentv1.TridentTransaction,
) (err error) {
	Logx(ctx).WithFields(LogFields{
		"tx.ResourceVersion":              tx.ResourceVersion,
		"tx.ObjectMeta.DeletionTimestamp": tx.ObjectMeta.DeletionTimestamp,
	}).Trace("removeTransactionFinalizers")

	if tx.HasTridentFinalizers() {
		Logx(ctx).Trace("Has finalizers, removing them.")
		txCopy := tx.DeepCopy()
		txCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentTransactions(tx.Namespace).Update(ctx, txCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Trace("No finalizers to remove.")
	}

	return
}

// removeVersionFinalizers removes Trident's finalizers from TridentVersion CRs
func (c *TridentCrdController) removeVersionFinalizers(ctx context.Context, v *tridentv1.TridentVersion) (err error) {
	Logx(ctx).WithFields(LogFields{
		"v.ResourceVersion":              v.ResourceVersion,
		"v.ObjectMeta.DeletionTimestamp": v.ObjectMeta.DeletionTimestamp,
	}).Trace("removeVersionFinalizers")

	if v.HasTridentFinalizers() {
		Logx(ctx).Trace("Has finalizers, removing them.")
		vCopy := v.DeepCopy()
		vCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentVersions(v.Namespace).Update(ctx, vCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Trace("No finalizers to remove.")
	}

	return
}

// removeVolumeFinalizers removes Trident's finalizers from TridentVolume CRs
func (c *TridentCrdController) removeVolumeFinalizers(ctx context.Context, vol *tridentv1.TridentVolume) (err error) {
	Logx(ctx).WithFields(LogFields{
		"vol.ResourceVersion":              vol.ResourceVersion,
		"vol.ObjectMeta.DeletionTimestamp": vol.ObjectMeta.DeletionTimestamp,
	}).Trace("removeVolumeFinalizers")

	if vol.HasTridentFinalizers() {
		Logx(ctx).Trace("Has finalizers, removing them.")
		volCopy := vol.DeepCopy()
		volCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentVolumes(vol.Namespace).Update(ctx, volCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Trace("No finalizers to remove.")
	}

	return
}

// removeVolumePublicationFinalizers removes Trident's finalizers from TridentVolumePublication CRs
func (c *TridentCrdController) removeVolumePublicationFinalizers(
	ctx context.Context, volPub *tridentv1.TridentVolumePublication,
) (err error) {
	Logx(ctx).WithFields(LogFields{
		"volPub.ResourceVersion":              volPub.ResourceVersion,
		"volPub.ObjectMeta.DeletionTimestamp": volPub.ObjectMeta.DeletionTimestamp,
	}).Trace("removeVolumePublicationFinalizers")

	if volPub.HasTridentFinalizers() {
		Logx(ctx).Trace("Has finalizers, removing them.")
		volCopy := volPub.DeepCopy()
		volCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentVolumePublications(volPub.Namespace).Update(ctx, volCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Trace("No finalizers to remove.")
	}

	return
}

// removeSnapshotFinalizers removes Trident's finalizers from TridentSnapshot CRs
func (c *TridentCrdController) removeSnapshotFinalizers(
	ctx context.Context, snap *tridentv1.TridentSnapshot,
) (err error) {
	Logx(ctx).WithFields(LogFields{
		"snap.ResourceVersion":              snap.ResourceVersion,
		"snap.ObjectMeta.DeletionTimestamp": snap.ObjectMeta.DeletionTimestamp,
	}).Trace("removeSnapshotFinalizers")

	if snap.HasTridentFinalizers() {
		Logx(ctx).Trace("Has finalizers, removing them.")
		snapCopy := snap.DeepCopy()
		snapCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentSnapshots(snap.Namespace).Update(ctx, snapCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Trace("No finalizers to remove.")
	}

	return
}

// removeTMRFinalizers removes Trident's finalizers from TridentMirrorRelationship CRs
func (c *TridentCrdController) removeTMRFinalizers(
	ctx context.Context, tmr *tridentv1.TridentMirrorRelationship,
) (err error) {
	Logx(ctx).WithFields(LogFields{
		"tmr.ResourceVersion":              tmr.ResourceVersion,
		"tmr.ObjectMeta.DeletionTimestamp": tmr.ObjectMeta.DeletionTimestamp,
	}).Trace("removeTMRFinalizers")

	if tmr.HasTridentFinalizers() {
		Logx(ctx).Trace("Has finalizers, removing them.")
		tmrCopy := tmr.DeepCopy()
		tmrCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentMirrorRelationships(tmr.Namespace).Update(ctx, tmrCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Trace("No finalizers to remove.")
	}

	return
}

// removeTSIFinalizers removes Trident's finalizers from TridentSnapshotInfo CRs
func (c *TridentCrdController) removeTSIFinalizers(
	ctx context.Context, tsi *tridentv1.TridentSnapshotInfo,
) (err error) {
	Logx(ctx).WithFields(LogFields{
		"tsi.ResourceVersion":              tsi.ResourceVersion,
		"tsi.ObjectMeta.DeletionTimestamp": tsi.ObjectMeta.DeletionTimestamp,
	}).Trace("removeTSIFinalizers")

	if tsi.HasTridentFinalizers() {
		Logx(ctx).Trace("Has finalizers, removing them.")
		tsiCopy := tsi.DeepCopy()
		tsiCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentSnapshotInfos(tsi.Namespace).Update(ctx, tsiCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Trace("No finalizers to remove.")
	}

	return
}

// removeTNR Finalizers removes Trident's finalizers from TridentNodeRemediation CRs
func (c *TridentCrdController) removeTNRFinalizers(
	ctx context.Context, tnr *tridentv1.TridentNodeRemediation,
) (err error) {
	Logx(ctx).WithFields(LogFields{
		"tnr.ResourceVersion":              tnr.ResourceVersion,
		"tnr.ObjectMeta.DeletionTimestamp": tnr.ObjectMeta.DeletionTimestamp,
	}).Trace("removeTNRFinalizers")

	if !tnr.HasTridentFinalizers() {
		Logx(ctx).Trace("No finalizers to remove.")
		return nil
	}

	Logx(ctx).Trace("Has finalizers, removing them.")
	tnrCopy := tnr.DeepCopy()
	tnrCopy.RemoveTridentFinalizers()
	_, err = c.crdClientset.TridentV1().TridentNodeRemediations(tnr.Namespace).Update(ctx, tnrCopy, updateOpts)
	if err != nil {
		Logx(ctx).Errorf("Problem removing finalizers: %v", err)
		return
	}

	return nil
}

func (c *TridentCrdController) SetForceDetach(enableForceDetach bool) {
	c.enableForceDetach = enableForceDetach
}
