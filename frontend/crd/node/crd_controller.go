// Copyright 2025 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	goinformer "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	clik8sclient "github.com/netapp/trident/cli/k8s_client"
	"github.com/netapp/trident/config"
	"github.com/netapp/trident/frontend/autogrow"
	crdtypes "github.com/netapp/trident/frontend/crd/types"
	"github.com/netapp/trident/frontend/csi"
	. "github.com/netapp/trident/logging"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentv1clientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	tridentscheme "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/scheme"
	tridentinformers "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions"
	listers "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	"github.com/netapp/trident/utils/errors"
)

type (
	EventType  = crdtypes.EventType
	ObjectType = crdtypes.ObjectType
)

const (
	EventAdd         = crdtypes.EventAdd
	EventUpdate      = crdtypes.EventUpdate
	EventForceUpdate = crdtypes.EventForceUpdate
	EventDelete      = crdtypes.EventDelete

	OjbectTypeTridentVolumePublication = crdtypes.ObjectTypeTridentVolumePublication
	ObjectTypeTridentBackend           = crdtypes.ObjectTypeTridentBackend
	ObjectTypeTridentAutogrowPolicy    = crdtypes.ObjectTypeTridentAutogrowPolicy
	ObjectTypeStorageClass             = crdtypes.ObjectTypeStorageClass

	OperationStatusSuccess string = "Success"
	OperationStatusFailed  string = "Failed"

	controllerName         = "node-crd-controller"
	controllerAgentName    = "trident-node-crd-controller"
	crdControllerQueueName = "trident-node-crd-workqueue"
)

type KeyItem struct {
	key        string
	objectType ObjectType
	event      EventType
	ctx        context.Context
	isRetry    bool
}

var (
	informerResyncPeriod = 10 * time.Minute
)

func Logx(ctx context.Context) LogEntry {
	return Logc(ctx).WithField(LogSource, "trident-node-crd-controller")
}

// TridentNodeCrdController is the controller implementation for Trident's Node CRD resources handling
type TridentNodeCrdController struct {
	plugin   *csi.Plugin
	nodeName string

	// kubeClientset is a standard kubernetes clientset
	kubeClientset kubernetes.Interface

	// crdClientset is a clientset for our own API group
	crdClientset tridentv1clientset.Interface

	crdControllerStopChan chan struct{}

	// Various informer factories
	tridentNSCrdInformerFactory tridentinformers.SharedInformerFactory // For namespace-scoped CRs
	allNSCrdInformerFactory     tridentinformers.SharedInformerFactory // For cluster-scoped CRs
	kubeInformerFactory         goinformer.SharedInformerFactory       // For standard K8s resources

	// TVP related informer and lister
	tridentVolumePublicationInformer cache.SharedIndexInformer
	tridentVolumePublicationLister   listers.TridentVolumePublicationLister
	tridentVolumePublicationSynced   cache.InformerSynced

	// TBE related informer and lister
	tridentBackendInformer cache.SharedIndexInformer
	tridentBackendLister   listers.TridentBackendLister
	tridentBackendSynced   cache.InformerSynced

	// TAGP related informer and lister
	tridentAutogrowPolicyInformer cache.SharedIndexInformer
	tridentAutogrowPolicyLister   listers.TridentAutogrowPolicyLister
	tridentAutogrowPolicySynced   cache.InformerSynced

	// StorageClass related informer and lister
	storageClassInformer cache.SharedIndexInformer
	storageClassLister   storagelisters.StorageClassLister
	storageClassSynced   cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.TypedRateLimitingInterface[KeyItem]

	// recorder is an event recorder for recording Event resources to the Kubernetes API.
	recorder record.EventRecorder

	// Autogrow orchestrator manages the Autogrow feature on this node
	// Activated when first AGP exists, deactivated when last AGP is deleted
	autogrowOrchestrator autogrow.AutogrowOrchestrator
}

// NewTridentNodeCrdController returns a new Trident Node CRD controller frontend
func NewTridentNodeCrdController(
	masterURL, kubeConfigPath, nodeName string, plugin *csi.Plugin, autogrowPeriod time.Duration,
) (*TridentNodeCrdController, error) {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowCRDControllerCreate, LogLayerCRDFrontend)

	Logx(ctx).Debug("Creating Trident Node CRDv1 controller.")

	clients, err := clik8sclient.CreateK8SClients(masterURL, kubeConfigPath, "")
	if err != nil {
		return nil, err
	}

	return newTridentNodeCrdController(clients.Namespace, clients.KubeClient, clients.TridentClient,
		nodeName, plugin, autogrowPeriod)
}

// newTridentNodeCrdController returns a new Trident Node CRD controller frontend instance
func newTridentNodeCrdController(
	tridentNamespace string, kubeClientset kubernetes.Interface,
	crdClientset tridentv1clientset.Interface, nodeName string, plugin *csi.Plugin, autogrowPeriod time.Duration,
) (*TridentNodeCrdController, error) {
	ctx := GenerateRequestContext(nil, "", "", WorkflowNone, LogLayerCRDFrontend)
	Logx(ctx).WithFields(LogFields{
		"namespace":      tridentNamespace,
		"nodeName":       nodeName,
		"autogrowPeriod": autogrowPeriod,
	}).Debug("Starting Trident Node CRD controller initialization")

	// ========================================
	// Step: Validate input parameters
	// ========================================
	if kubeClientset == nil {
		return nil, errors.New("kubeClientset cannot be nil")
	}
	if crdClientset == nil {
		return nil, errors.New("crdClientset cannot be nil")
	}
	if plugin == nil {
		return nil, errors.New("plugin cannot be nil")
	}
	if nodeName == "" {
		return nil, errors.New("nodeName cannot be empty")
	}
	if tridentNamespace == "" {
		return nil, errors.New("tridentNamespace cannot be empty")
	}

	// ========================================
	// Step: Set up informer factories
	// ========================================
	// Informers watch Kubernetes resources and notify us of changes (add/update/delete events).
	// We create different factories based on resource scope and filtering needs.

	// Factory for namespace-scoped resources (watches only the Trident namespace)
	// Used for: TridentBackend
	tridentNSCrdInformerFactory := tridentinformers.NewSharedInformerFactoryWithOptions(
		crdClientset,
		informerResyncPeriod,
		tridentinformers.WithNamespace(tridentNamespace),
	)

	// Factory for cluster-scoped resources (watches across entire cluster)
	// Used for: TridentAutogrowPolicy
	allNSCrdInformerFactory := tridentinformers.NewSharedInformerFactory(
		crdClientset,
		informerResyncPeriod,
	)

	// Factory for standard Kubernetes resources
	// Used for: StorageClass
	kubeInformerFactory := goinformer.NewSharedInformerFactory(kubeClientset, informerResyncPeriod)

	// ========================================
	// Step: Set up TridentVolumePublication informer with label filtering
	// ========================================
	// This node only needs to watch TVPs that belong to it, not all TVPs in the cluster.
	// We use a label selector to filter on the server side, reducing memory and network usage.

	tvpLabelSelector := fmt.Sprintf("trident.netapp.io/nodeName=%s", nodeName)
	Logx(ctx).WithFields(LogFields{
		"nodeName":      nodeName,
		"labelSelector": tvpLabelSelector,
	}).Trace("Setting up filtered informer to watch only TridentVolumePublications for this node")

	// Create a custom list-watcher that applies the label filter
	tvpListWatcher := cache.NewFilteredListWatchFromClient(
		crdClientset.TridentV1().RESTClient(),
		"tridentvolumepublications",
		tridentNamespace,
		func(options *metav1.ListOptions) {
			if options != nil {
				options.LabelSelector = tvpLabelSelector
			}
		},
	)

	// Create the informer with empty indexers.
	tvpInformer := cache.NewSharedIndexInformer(
		tvpListWatcher,
		&tridentv1.TridentVolumePublication{},
		informerResyncPeriod,
		cache.Indexers{}, // Empty - no indexes needed for our use case
	)

	// Create the lister for TridentVolumePublications from the informer
	tvpLister := listers.NewTridentVolumePublicationLister(tvpInformer.GetIndexer())

	// ========================================
	// Step: Set up informers and listers for other Trident CRs and K8s objects using the factories
	// ========================================

	// TridentBackend informer and lister - watches all backends in the Trident namespace
	tbeInformer := tridentNSCrdInformerFactory.Trident().V1().TridentBackends().Informer()
	tbeLister := listers.NewTridentBackendLister(tbeInformer.GetIndexer())

	// TridentAutogrowPolicy informer - watches all trident Autogrow policies across the cluster
	tagpInformer := allNSCrdInformerFactory.Trident().V1().TridentAutogrowPolicies().Informer()
	tagpLister := listers.NewTridentAutogrowPolicyLister(tagpInformer.GetIndexer())

	// StorageClass informer and lister - watches all storage classes across the cluster
	scInformer := kubeInformerFactory.Storage().V1().StorageClasses().Informer()
	scLister := kubeInformerFactory.Storage().V1().StorageClasses().Lister()

	// ========================================
	// Step: Set up event recording for Kubernetes events
	// ========================================
	// This allows the controller to create Events that users can see with 'kubectl describe'

	// Register Trident CRD types with Kubernetes scheme so we can create events for them
	utilruntime.Must(tridentscheme.AddToScheme(scheme.Scheme))
	Logx(ctx).Trace("Creating event broadcaster.")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	// ========================================
	// Step: Create the Autogrow orchestrator instance
	// ========================================

	// Initialize Autogrow orchestrator
	// This will be activated/deactivated based on the AGP existence
	// Pass nil configs to use defaults for all layers
	// The autogrow period from command line will be used for scheduler assorter period
	// and for poller worker pool expiry duration
	Logx(ctx).Debug("Creating Autogrow orchestrator")

	// Extract NodeHelper from the CSI plugin
	nodeHelper := plugin.GetNodeHelper()
	if nodeHelper == nil {
		return nil, errors.New("nodeHelper from plugin cannot be nil")
	}

	// Create the Autogrow orchestrator with all dependencies
	autogrowOrch, err := autogrow.New(
		ctx,
		crdClientset,
		tagpLister,
		scLister,
		tvpLister,
		nodeHelper,
		tridentNamespace, // Trident namespace
		autogrowPeriod,   // Autogrow period from command line
		nil,              // schedulerConfig: use defaults
		nil,              // pollerConfig: use defaults
		nil,              // requesterConfig: use defaults
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create Autogrow orchestrator: %w", err)
	}

	Logc(ctx).Debug("Created Autogrow orchestrator.")

	// ========================================
	// Step: Create the controller instance
	// ========================================

	controller := &TridentNodeCrdController{
		plugin:   plugin,
		nodeName: nodeName,

		// Kubernetes clients
		kubeClientset: kubeClientset,
		crdClientset:  crdClientset,

		// Informer factories and direct informers
		crdControllerStopChan:            make(chan struct{}),
		tridentNSCrdInformerFactory:      tridentNSCrdInformerFactory,
		allNSCrdInformerFactory:          allNSCrdInformerFactory,
		kubeInformerFactory:              kubeInformerFactory,
		tridentVolumePublicationInformer: tvpInformer,
		tridentBackendInformer:           tbeInformer,
		tridentAutogrowPolicyInformer:    tagpInformer,
		storageClassInformer:             scInformer,

		// Listers for cached resource lookups
		tridentVolumePublicationLister: tvpLister,
		tridentBackendLister:           tbeLister,
		tridentAutogrowPolicyLister:    tagpLister,
		storageClassLister:             scLister,

		// Informer sync status checkers
		tridentVolumePublicationSynced: tvpInformer.HasSynced,
		tridentBackendSynced:           tbeInformer.HasSynced,
		tridentAutogrowPolicySynced:    tagpInformer.HasSynced,
		storageClassSynced:             scInformer.HasSynced,

		autogrowOrchestrator: autogrowOrch,

		// Work queue and event recorder
		workqueue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[KeyItem](),
			workqueue.TypedRateLimitingQueueConfig[KeyItem]{
				Name: crdControllerQueueName,
			}),
		recorder: recorder,
	}

	// ========================================
	// Step: Register event handlers for each resource type
	// ========================================
	// When Kubernetes resources change (add/update/delete), these handlers are called

	Logx(ctx).Debug("Setting up CRD controller event handlers for the node.")

	// TridentVolumePublication events (filtered by node label)
	_, _ = tvpInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addCRHandler,
		UpdateFunc: controller.updateCRHandler,
		DeleteFunc: controller.deleteCRHandler,
	})

	// TridentBackend events (all backends in Trident namespace)
	_, _ = tbeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addCRHandler,
		UpdateFunc: controller.updateCRHandler,
		DeleteFunc: controller.deleteCRHandler,
	})

	// TridentAutogrowPolicy events (all policies cluster-wide)
	_, _ = tagpInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addCRHandler,
		UpdateFunc: controller.updateCRHandler,
		DeleteFunc: controller.deleteCRHandler,
	})

	// StorageClass events (all storage classes cluster-wide)
	// StorageClass is a standard K8s resource, not a Trident CRD, so we use generic K8s handlers
	_, _ = scInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addK8sResourceHandler(ObjectTypeStorageClass),
		UpdateFunc: controller.updateK8sResourceHandler(ObjectTypeStorageClass),
		DeleteFunc: controller.deleteK8sResourceHandler(ObjectTypeStorageClass),
	})

	return controller, nil
}

func (c *TridentNodeCrdController) Activate() error {
	Log().Debug(">>>> crd_controller.Activate()")
	defer Log().Debug("<<<< crd_controller.Activate()")
	ctx := GenerateRequestContext(nil, "", "", WorkflowNone, LogLayerCRDFrontend)
	// Block until the plugin is ready.
	// This will only err if the plugin fails to activate.
	if err := c.plugin.WaitForActivation(ctx); err != nil {
		return err
	}

	Logx(ctx).WithFields(LogFields{
		"nodeName": c.nodeName,
	}).Debug("Activating Node CRD controller to watch for CRD and K8s resource events.")
	if c.crdControllerStopChan != nil {
		// Start the informer factories (non-blocking - they launch goroutines internally)
		c.tridentNSCrdInformerFactory.Start(c.crdControllerStopChan)
		c.allNSCrdInformerFactory.Start(c.crdControllerStopChan)
		c.kubeInformerFactory.Start(c.crdControllerStopChan)

		// Start the direct informers (blocking, so run as goroutines)
		go c.tridentVolumePublicationInformer.Run(c.crdControllerStopChan)

		// Start the controller worker loop
		go c.Run(ctx, 1, c.crdControllerStopChan)
	}
	return nil
}

func (c *TridentNodeCrdController) Deactivate() error {
	ctx := GenerateRequestContext(nil, "", "", WorkflowNone, LogLayerCRDFrontend)
	Logx(ctx).Debug("Deactivating CRD frontend.")
	if c.crdControllerStopChan != nil {
		close(c.crdControllerStopChan)
	}

	var err error
	if c.autogrowOrchestrator != nil {
		err = c.autogrowOrchestrator.Deactivate(ctx)
	}

	return err
}

func (c *TridentNodeCrdController) GetName() string {
	return controllerName
}

func (c *TridentNodeCrdController) Version() string {
	return config.DefaultOrchestratorVersion
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *TridentNodeCrdController) Run(ctx context.Context, threadiness int, stopCh <-chan struct{}) {
	Logx(ctx).WithFields(LogFields{
		"threadiness": threadiness,
	}).Debug("TridentNodeCrdController#Run")

	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	Log().Info("Starting Trident Node CRD controller.")

	// Wait for the caches to be synced before starting workers
	Logx(ctx).Debug("Waiting for informer caches to sync.")
	if ok := cache.WaitForCacheSync(stopCh,
		c.tridentVolumePublicationSynced,
		c.tridentBackendSynced,
		c.tridentAutogrowPolicySynced,
		c.storageClassSynced,
	); !ok {
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
func (c *TridentNodeCrdController) runWorker() {
	ctx := GenerateRequestContext(nil, "", "", WorkflowNone, LogLayerCRDFrontend)
	Logx(ctx).Trace("TridentNodeCrdController runWorker started.")
	for c.processNextWorkItem() {
	}
}

func (c *TridentNodeCrdController) addEventToWorkqueue(key string, event EventType, ctx context.Context, objKind ObjectType) {
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
func (c *TridentNodeCrdController) addCRHandler(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventAdd))

	Logx(ctx).Trace("TridentNodeCrdController#addCRHandler")

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

	c.addEventToWorkqueue(key, EventAdd, ctx, ObjectType(cr.GetKind()))
}

// updateCRHandler is the update handler for CR watchers.
func (c *TridentNodeCrdController) updateCRHandler(old, new interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventUpdate))

	Logx(ctx).Trace("TridentNodeCrdController#updateCRHandler")

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

	c.addEventToWorkqueue(key, EventUpdate, ctx, ObjectType(newCR.GetKind()))
}

// deleteCRHandler is the delete handler for CR watchers.
func (c *TridentNodeCrdController) deleteCRHandler(obj interface{}) {
	ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventDelete))

	Logx(ctx).Trace("TridentNodeCrdController#deleteCRHandler")

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

	c.addEventToWorkqueue(key, EventDelete, ctx, ObjectType(cr.GetKind()))
}

// addK8sResourceHandler returns a generic add handler for K8s resources.
// This creates a closure that captures the objectType.
func (c *TridentNodeCrdController) addK8sResourceHandler(objectType ObjectType) func(obj interface{}) {
	return func(obj interface{}) {
		ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
		ctx = context.WithValue(ctx, CRDControllerEvent, string(EventAdd))

		Logx(ctx).WithField("objectType", objectType).Trace("TridentNodeCrdController#addK8sResourceHandler")

		var key string
		var err error
		if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
			Logx(ctx).Error(err)
			return
		}

		c.addEventToWorkqueue(key, EventAdd, ctx, objectType)
	}
}

// updateK8sResourceHandler returns a generic update handler for K8s resources.
// This creates a closure that captures the objectType.
func (c *TridentNodeCrdController) updateK8sResourceHandler(objectType ObjectType) func(old, new interface{}) {
	return func(old, new interface{}) {
		ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
		ctx = context.WithValue(ctx, CRDControllerEvent, string(EventUpdate))

		Logx(ctx).WithField("objectType", objectType).Trace("TridentNodeCrdController#updateK8sResourceHandler")

		if new == nil || old == nil {
			Logx(ctx).WithField("objectType", objectType).Warn("No updated resource provided, skipping update")
			return
		}

		var key string
		var err error
		if key, err = cache.MetaNamespaceKeyFunc(new); err != nil {
			Logx(ctx).Error(err)
			return
		}

		c.addEventToWorkqueue(key, EventUpdate, ctx, objectType)
	}
}

// deleteK8sResourceHandler returns a generic delete handler for K8s resources.
// This creates a closure that captures the objectType.
func (c *TridentNodeCrdController) deleteK8sResourceHandler(objectType ObjectType) func(obj interface{}) {
	return func(obj interface{}) {
		ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
		ctx = context.WithValue(ctx, CRDControllerEvent, string(EventDelete))

		Logx(ctx).WithField("objectType", objectType).Trace("TridentNodeCrdController#deleteK8sResourceHandler")

		var key string
		var err error
		if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
			Logx(ctx).Error(err)
			return
		}

		c.addEventToWorkqueue(key, EventDelete, ctx, objectType)
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the respective object handlers.
func (c *TridentNodeCrdController) processNextWorkItem() bool {
	ctx := GenerateRequestContext(nil, "", ContextSourceCRD, WorkflowCRReconcile, LogLayerCRDFrontend)
	Logx(ctx).Trace("TridentNodeCrdController#processNextWorkItem")

	obj, shutdown := c.workqueue.Get()

	if shutdown {
		Logx(ctx).Trace("TridentNodeCrdController#processNextWorkItem shutting down")
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(keyItem KeyItem) error {
		// We call Done here so the workqueue knows we have finished processing
		// this item. We also must remember to call Forget if we do not want
		// this work item being re-queued. For example, we do not call Forget if
		// a transient error occurs, instead the item is put back on the
		// workqueue and attempted again after a back-off period.
		defer c.workqueue.Done(keyItem)

		// TypedRateLimitingQueue ensures type safety - keyItem is already the correct type
		if keyItem == (KeyItem{}) {
			c.workqueue.Forget(keyItem)
			return fmt.Errorf("keyItem item is empty")
		}

		keyItemName := keyItem.key
		var handleFunction func(*KeyItem) error
		switch keyItem.objectType {
		case OjbectTypeTridentVolumePublication:
			handleFunction = c.handleTridentVolumePublications
		case ObjectTypeTridentBackend:
			handleFunction = c.handleTridentBackends
		case ObjectTypeTridentAutogrowPolicy:
			handleFunction = c.handleTridentAutogrowPolicies
		case ObjectTypeStorageClass:
			handleFunction = c.handleStorageClasses
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
		c.workqueue.Forget(keyItem)
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
