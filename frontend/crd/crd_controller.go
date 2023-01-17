// Copyright 2021 NetApp, Inc. All Rights Reserved.

package crd

import (
	"context"
	"fmt"
	"reflect"
	"time"

	k8ssnapshots "github.com/kubernetes-csi/external-snapshotter/client/v6/clientset/versioned"
	log "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	. "github.com/netapp/trident/logger"
	tridentv1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentv1clientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	tridentscheme "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/scheme"
	tridentinformers "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions"
	tridentinformersv1 "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions/netapp/v1"
	listers "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	"github.com/netapp/trident/storage"
	"github.com/netapp/trident/utils"
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

	ObjectTypeTridentBackendConfig      ObjectType = "trident-backend-config"
	ObjectTypeTridentBackend            ObjectType = "trident-backend"
	ObjectTypeSecret                    ObjectType = "secret"
	ObjectTypeTridentMirrorRelationship ObjectType = "mirror-relationship"
	ObjectTypeTridentSnapshotInfo       ObjectType = "snapshot-info"

	OperationStatusSuccess string = "Success"
	OperationStatusFailed  string = "Failed"

	controllerName                 = "crd"
	controllerAgentName            = "trident-crd-controller"
	tridentBackendConfigsQueueName = "TridentBackendConfigs"
)

type KeyItem struct {
	key        string
	objectType ObjectType
	event      EventType
	ctx        context.Context
}

var (
	listOpts   = metav1.ListOptions{}
	getOpts    = metav1.GetOptions{}
	createOpts = metav1.CreateOptions{}
	updateOpts = metav1.UpdateOptions{}

	ctx = context.Background
)

func Logx(ctx context.Context) *log.Entry {
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

	// TridentSnapshotInfo CRD handling
	snapshotInfoLister listers.TridentSnapshotInfoLister
	snapshotInfoSynced cache.InformerSynced

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

	// TridentVolumePublication CRD handling
	volumePublicationsLister listers.TridentVolumePublicationLister
	volumePublicationsSynced cache.InformerSynced

	// TridentSnapshot CRD handling
	snapshotsLister listers.TridentSnapshotLister
	snapshotsSynced cache.InformerSynced

	// TridentSnapshot CRD handling
	secretsLister v1.SecretLister
	secretsSynced cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface

	// recorder is an event recorder for recording Event resources to the Kubernetes API.
	recorder record.EventRecorder
}

// NewTridentCrdController returns a new Trident CRD controller frontend
func NewTridentCrdController(
	orchestrator core.Orchestrator, masterURL, kubeConfigPath string,
) (*TridentCrdController, error) {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourceInternal)

	Logx(ctx).Debug("Creating CRDv1 controller.")

	clients, err := clik8sclient.CreateK8SClients(masterURL, kubeConfigPath, "")
	if err != nil {
		return nil, err
	}

	return newTridentCrdControllerImpl(orchestrator, clients.Namespace, clients.KubeClient,
		clients.SnapshotClient, clients.TridentClient)
}

// newTridentCrdControllerImpl returns a new Trident CRD controller frontend
func newTridentCrdControllerImpl(
	orchestrator core.Orchestrator, tridentNamespace string,
	kubeClientset kubernetes.Interface, snapshotClientset k8ssnapshots.Interface,
	crdClientset tridentv1clientset.Interface,
) (*TridentCrdController, error) {
	log.WithFields(log.Fields{
		"namespace": tridentNamespace,
	}).Info("Initializing Trident CRD controller frontend.")

	// Set resync to 0 sec so that reconciliation is on demand
	crdInformerFactory := tridentinformers.NewSharedInformerFactory(crdClientset, time.Second*0)
	crdInformer := tridentinformersv1.New(crdInformerFactory, tridentNamespace, nil)
	allNSCrdInformer := tridentinformersv1.New(crdInformerFactory, corev1.NamespaceAll, nil)

	// Set resync to 0 sec so that reconciliation is on demand
	kubeInformerFactory := goinformer.NewSharedInformerFactory(kubeClientset, time.Second*0)
	kubeInformer := goinformerv1.New(kubeInformerFactory, tridentNamespace, nil)

	backendInformer := crdInformer.TridentBackends()
	backendConfigInformer := crdInformer.TridentBackendConfigs()
	mirrorInformer := allNSCrdInformer.TridentMirrorRelationships()
	snapshotInfoInformer := allNSCrdInformer.TridentSnapshotInfos()
	nodeInformer := crdInformer.TridentNodes()
	storageClassInformer := crdInformer.TridentStorageClasses()
	transactionInformer := crdInformer.TridentTransactions()
	versionInformer := crdInformer.TridentVersions()
	volumeInformer := crdInformer.TridentVolumes()
	volumePublicationInformer := crdInformer.TridentVolumePublications()
	snapshotInformer := crdInformer.TridentSnapshots()
	secretInformer := kubeInformer.Secrets()

	// Create event broadcaster
	// Add our types to the default Kubernetes Scheme so Events can be logged.
	utilruntime.Must(tridentscheme.AddToScheme(scheme.Scheme))
	log.Info("Creating event broadcaster.")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeClientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &TridentCrdController{
		orchestrator:             orchestrator,
		kubeClientset:            kubeClientset,
		snapshotClientSet:        snapshotClientset,
		crdClientset:             crdClientset,
		crdControllerStopChan:    make(chan struct{}),
		crdInformerFactory:       crdInformerFactory,
		crdInformer:              crdInformer,
		kubeInformerFactory:      kubeInformerFactory,
		kubeInformer:             kubeInformer,
		backendsLister:           backendInformer.Lister(),
		backendsSynced:           backendInformer.Informer().HasSynced,
		backendConfigsLister:     backendConfigInformer.Lister(),
		backendConfigsSynced:     backendConfigInformer.Informer().HasSynced,
		mirrorLister:             mirrorInformer.Lister(),
		mirrorSynced:             mirrorInformer.Informer().HasSynced,
		snapshotInfoLister:       snapshotInfoInformer.Lister(),
		snapshotInfoSynced:       snapshotInfoInformer.Informer().HasSynced,
		nodesLister:              nodeInformer.Lister(),
		nodesSynced:              nodeInformer.Informer().HasSynced,
		storageClassesLister:     storageClassInformer.Lister(),
		storageClassesSynced:     storageClassInformer.Informer().HasSynced,
		transactionsLister:       transactionInformer.Lister(),
		transactionsSynced:       transactionInformer.Informer().HasSynced,
		versionsLister:           versionInformer.Lister(),
		versionsSynced:           versionInformer.Informer().HasSynced,
		volumesLister:            volumeInformer.Lister(),
		volumesSynced:            volumeInformer.Informer().HasSynced,
		volumePublicationsLister: volumePublicationInformer.Lister(),
		volumePublicationsSynced: volumePublicationInformer.Informer().HasSynced,
		snapshotsLister:          snapshotInformer.Lister(),
		snapshotsSynced:          snapshotInformer.Informer().HasSynced,
		secretsLister:            secretInformer.Lister(),
		secretsSynced:            secretInformer.Informer().HasSynced,
		workqueue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(),
			tridentBackendConfigsQueueName),
		recorder: recorder,
	}

	// Set up event handlers for when our Trident CRDs change
	log.Info("Setting up CRD controller event handlers.")

	_, _ = backendConfigInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addTridentBackendConfigEvent,
		UpdateFunc: controller.updateTridentBackendConfigEvent,
		DeleteFunc: controller.deleteTridentBackendConfigEvent,
	})

	_, _ = backendInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// Do not handle add backends here, otherwise it may results in continuous
		// reconcile loops esp. in cases where backends are created as a result
		// of a backend config.
		UpdateFunc: func(oldCrd, newCrd interface{}) {
			// When a CR has a finalizer those come as update events and not deletes,
			// Do not handle any other update events other than backend deletion
			// otherwise it may results in continuous reconcile loops.
			ctx := GenerateRequestContext(context.Background(), "", ContextSourceCRD)
			if err := controller.removeFinalizers(ctx, newCrd, false); err != nil {
				Logc(ctx).WithError(err).Error("Error removing finalizers")
			}
		},
		DeleteFunc: controller.deleteTridentBackendEvent,
	})

	_, _ = mirrorInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addMirrorRelationship,
		UpdateFunc: controller.updateMirrorRelationship,
		DeleteFunc: controller.deleteMirrorRelationship,
	})

	_, _ = snapshotInfoInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    controller.addSnapshotInfo,
		UpdateFunc: controller.updateSnapshotInfo,
		DeleteFunc: controller.deleteSnapshotInfo,
	})

	_, _ = secretInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		// Do not handle AddFunc here otherwise everytime trident is restarted,
		// there will be unwarranted reconciles and backend initializations
		UpdateFunc: controller.updateSecretEvent,
		// Do not handle DeleteFunc here as these are user created secrets,
		// Trident need not do anything, if there are backends using the secret
		// they will continue to function using the credentials detains in memory
	})

	informers := []cache.SharedIndexInformer{
		nodeInformer.Informer(),
		storageClassInformer.Informer(),
		transactionInformer.Informer(),
		versionInformer.Informer(),
		volumeInformer.Informer(),
		volumePublicationInformer.Informer(),
		snapshotInformer.Informer(),
	}
	for _, informer := range informers {
		_, _ = informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			UpdateFunc: func(oldCrd, newCrd interface{}) {
				ctx := GenerateRequestContext(context.Background(), "", ContextSourceCRD)
				if err := controller.removeFinalizers(ctx, newCrd, false); err != nil {
					Logc(ctx).WithError(err).Error("Error removing finalizers")
				}
			},
		})
	}

	return controller, nil
}

func (c *TridentCrdController) Activate() error {
	log.Info("Activating CRD frontend.")
	if c.crdControllerStopChan != nil {
		c.crdInformerFactory.Start(c.crdControllerStopChan)
		c.kubeInformerFactory.Start(c.crdControllerStopChan)
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
	return controllerName
}

func (c *TridentCrdController) Version() string {
	return config.DefaultOrchestratorVersion
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *TridentCrdController) Run(threadiness int, stopCh <-chan struct{}) {
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
		log.Errorf("Error: %v", waitErr)
		return
	}

	// Launch workers to process CRD resources
	log.Info("Starting workers.")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	log.Info("Started workers.")
	<-stopCh
	log.Info("Shutting down workers.")
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *TridentCrdController) runWorker() {
	log.Debug("TridentCrdController runWorker started.")
	for c.processNextWorkItem() {
	}
}

// addTridentBackendConfigEvent takes a TridentBackendConfig resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than TridentBackendConfig.
func (c *TridentCrdController) addTridentBackendConfigEvent(obj interface{}) {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourceCRD)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventAdd))

	Logx(ctx).Debug("TridentCrdController#addTridentBackendConfigEvent")

	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		Logx(ctx).Error(err)
		return
	}

	keyItem := KeyItem{
		key:        key,
		event:      EventAdd,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackendConfig,
	}

	c.workqueue.Add(keyItem)
}

// updateTridentBackendConfigEvent takes a TridentBackendConfig resource and converts
// it into a namespace/name string which is then put onto the work queue. This method
// should *not* be passed resources of any type other than TridentBackendConfig.
func (c *TridentCrdController) updateTridentBackendConfigEvent(old, new interface{}) {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourceCRD)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventUpdate))

	Logx(ctx).Debug("TridentCrdController#updateTridentBackendConfigEvent")

	newBackendConfig := new.(*tridentv1.TridentBackendConfig)
	oldBackendConfig := old.(*tridentv1.TridentBackendConfig)

	// Ignore metadata and status only updates
	if oldBackendConfig != nil && newBackendConfig != nil {
		if newBackendConfig.GetGeneration() == oldBackendConfig.GetGeneration() && newBackendConfig.GetGeneration() != 0 {
			Logx(ctx).Debugf("No change in the generation, nothing to do.")
			return
		}
	}

	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(new); err != nil {
		Logx(ctx).Error(err)
		return
	}

	keyItem := KeyItem{
		key:        key,
		event:      EventUpdate,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackendConfig,
	}

	c.workqueue.Add(keyItem)
}

// deleteTridentBackendConfigEvent takes a TridentBackendConfig resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than TridentBackendConfig.
func (c *TridentCrdController) deleteTridentBackendConfigEvent(obj interface{}) {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourceCRD)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventDelete))

	Logx(ctx).Debug("TridentCrdController#deleteTridentBackendConfigEvent")

	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		Logx(ctx).Error(err)
		return
	}

	keyItem := KeyItem{
		key:        key,
		event:      EventDelete,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackendConfig,
	}

	c.workqueue.Add(keyItem)
}

// deleteTridentBackendEvent takes a TridentBackend resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than TridentBackend.
func (c *TridentCrdController) deleteTridentBackendEvent(obj interface{}) {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourceCRD)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventDelete))

	Logx(ctx).Debug("TridentCrdController#deleteTridentBackendEvent")

	var key, namespace string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		Logx(ctx).Error(err)
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, _, err = cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logx(ctx).WithField("key", key).Error("Invalid backend key.")
		return
	}

	// Get the BackendInfo.BackendUUID of the object and set it as the key
	backendUUID := obj.(*tridentv1.TridentBackend).BackendUUID

	// Later we are going to rely on the backendUUID to find an associated backend config
	// Relying on backendUUID will allow easy identification of the backend config, name won't
	// as it is of type `tbe-xyz` format, and not stored in the config.
	newKey := namespace + "/" + backendUUID

	keyItem := KeyItem{
		key:        newKey,
		event:      EventDelete,
		ctx:        ctx,
		objectType: ObjectTypeTridentBackend,
	}

	c.workqueue.Add(keyItem)
}

// updateSecretEvent takes a Kubernetes secret resource and converts it
// into a namespace/name string which is then put onto the work queue.
// This method should *not* be passed resources of any type other than Secrets.
func (c *TridentCrdController) updateSecretEvent(old, new interface{}) {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourceCRD)
	ctx = context.WithValue(ctx, CRDControllerEvent, string(EventUpdate))

	Logx(ctx).Debug("TridentCrdController#updateSecretEvent")

	newSecret := new.(*corev1.Secret)
	oldSecret := old.(*corev1.Secret)

	// Ignore metadata and status only updates
	if oldSecret != nil && newSecret != nil {
		if newSecret.GetGeneration() == oldSecret.GetGeneration() && newSecret.GetGeneration() != 0 {
			Logx(ctx).Debugf("No change in the generation, nothing to do.")
			return
		}
	}

	if oldSecret != nil && newSecret != nil {
		if newSecret.ResourceVersion == oldSecret.ResourceVersion {
			Logx(ctx).Debugf("No change in the resource version, nothing to do.")
			return
		}
	}

	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(new); err != nil {
		Logx(ctx).Error(err)
		return
	}

	keyItem := KeyItem{
		key:        key,
		event:      EventUpdate,
		ctx:        ctx,
		objectType: ObjectTypeSecret,
	}

	c.workqueue.Add(keyItem)
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the reconcileBackendConfig.
func (c *TridentCrdController) processNextWorkItem() bool {
	ctx := GenerateRequestContext(context.Background(), "", ContextSourceCRD)
	Logx(ctx).Debug("TridentCrdController#processNextWorkItem")

	obj, shutdown := c.workqueue.Get()

	if shutdown {
		Logx(ctx).Debug("TridentCrdController#processNextWorkItem shutting down")
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
		switch keyItem.objectType {
		case ObjectTypeTridentBackend:
			c.reconcileBackend(&keyItem)
		case ObjectTypeSecret:
			c.reconcileSecret(&keyItem)
			keyItemName = "<REDACTED>"
		case ObjectTypeTridentBackendConfig:
			if err := c.reconcileBackendConfig(&keyItem); err != nil {
				return err
			}
		case ObjectTypeTridentMirrorRelationship:
			if err := c.reconcileTMR(&keyItem); err != nil {
				if utils.IsReconcileDeferredError(err) {
					// If it is a deferred error, then do not remove the object from the queue
					return nil
				} else if utils.IsReconcileIncompleteError(err) {
					// If the reconcile is incomplete, then do not remove the object from the queue
					return nil
				} else {
					return err
				}
			}
		case ObjectTypeTridentSnapshotInfo:
			if err := c.reconcileTSI(&keyItem); err != nil {
				if utils.IsReconcileDeferredError(err) {
					// If it is a deferred error, then do not remove the object from the queue
					return nil
				} else {
					return err
				}
			}
		default:
			return fmt.Errorf("unknown objectType in the workqueue: %v", keyItem.objectType)
		}

		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		Logx(keyItem.ctx).Infof("Synced '%s'", keyItemName)
		log.Info("-------------------------------------------------")
		log.Info("-------------------------------------------------")

		return nil
	}(obj)
	if err != nil {
		Logx(ctx).Error(err)
		return true
	}

	return true
}

func (c *TridentCrdController) reconcileBackend(keyItem *KeyItem) {
	key := keyItem.key
	ctx := keyItem.ctx
	eventType := keyItem.event
	objectType := keyItem.objectType

	Logx(ctx).WithFields(log.Fields{
		"Key":        key,
		"eventType":  eventType,
		"objectType": objectType,
	}).Debug("TridentCrdController#reconcileBackend")

	if eventType != EventDelete {
		Logx(ctx).Error("Wrong backend event triggered a reconcileBackend.")
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logx(ctx).WithField("key", key).Error("Invalid key.")
		return
	}

	// Get the backend config that matches the backendUUID, i.e. the key
	backendConfig, err := c.getBackendConfigWithBackendUUID(ctx, namespace, name)
	if err != nil {
		if utils.IsNotFoundError(err) {
			Logx(ctx).Infof("No backend config is associated with the backendUUID '%v'.", name)
		} else {
			Logx(ctx).Errorf("unable to identify a backend config associated with the backendUUID '%v'.", name)
		}

		return
	}

	var newEventType EventType
	if backendConfig.Status.Phase == string(tridentv1.PhaseUnbound) {
		// Ideally this should not be the case
		Logx(ctx).Debugf("Backend Config '%v' has %v phase, nothing to do", backendConfig.Name,
			tridentv1.PhaseUnbound)
		return
	} else if backendConfig.Status.Phase == string(tridentv1.PhaseLost) {
		// Nothing to do here
		Logx(ctx).Debugf("Backend Config '%v' is already in a %v phase, nothing to do", backendConfig.Name,
			tridentv1.PhaseLost)
		return
	} else if backendConfig.Status.Phase == string(tridentv1.PhaseDeleting) {
		Logx(ctx).Debugf("Backend Config '%v' is already in a deleting phase, ensuring its deletion",
			backendConfig.Name)
		newEventType = EventDelete
	} else {
		Logx(ctx).Debugf("Backend Config '%v' has %v phase, running an update to set it to %v",
			backendConfig.Name, backendConfig.Status.Phase, tridentv1.PhaseLost)
		newEventType = EventForceUpdate
	}

	newKey := backendConfig.Namespace + "/" + backendConfig.Name
	newCtx := context.WithValue(ctx, CRDControllerEvent, string(EventUpdate))

	newKeyItem := KeyItem{
		key:        newKey,
		event:      newEventType,
		ctx:        newCtx,
		objectType: ObjectTypeTridentBackendConfig,
	}
	c.workqueue.Add(newKeyItem)
}

func (c *TridentCrdController) reconcileSecret(keyItem *KeyItem) {
	key := keyItem.key
	ctx := keyItem.ctx
	eventType := keyItem.event
	objectType := keyItem.objectType

	Logx(ctx).WithFields(log.Fields{
		"eventType":  eventType,
		"objectType": objectType,
	}).Debug("TridentCrdController#reconcileSecret")

	if eventType != EventUpdate {
		Logx(ctx).Error("Wrong secret event triggered a reconcileSecret.")
		return
	}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, secretName, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logx(ctx).Error("Invalid key.")
		return
	}

	// Get the backend configs that matches the secret namespace and secretName, i.e. the key
	backendConfigs, err := c.getBackendConfigsWithSecret(ctx, namespace, secretName)
	if err != nil {
		if utils.IsNotFoundError(err) {
			Logx(ctx).Infof("No backend config is associated with the secret update")
		} else {
			Logx(ctx).Errorf("unable to identify a backend config associated with the secret update")
		}
	}

	// Add these backend configs back to the queue and run update on these backends
	for _, backendConfig := range backendConfigs {
		Logx(ctx).WithFields(log.Fields{
			"backendConfig.Name":      backendConfig.Name,
			"backendConfig.Namespace": backendConfig.Namespace,
		}).Info("Running update on backend due to secret update.")

		newKey := backendConfig.Namespace + "/" + backendConfig.Name
		newCtx := context.WithValue(ctx, CRDControllerEvent, string(EventUpdate))

		keyItem := KeyItem{
			key:        newKey,
			event:      EventUpdate,
			ctx:        newCtx,
			objectType: ObjectTypeTridentBackendConfig,
		}
		c.workqueue.Add(keyItem)
	}
}

// reconcileBackendConfig compares the actual state with the desired of the backend config,
// and attempts to converge the two.
// It then updates the Status block of the BackendConfig resource with the current status of the resource.
func (c *TridentCrdController) reconcileBackendConfig(keyItem *KeyItem) error {
	Logx(keyItem.ctx).Debug("TridentCrdController#reconcileBackendConfig")

	// Run the reconcileBackendConfig, passing it the namespace/name string of the resource to be synced.
	if err := c.handleTridentBackendConfig(keyItem); err != nil {

		// Put the item back on the workqueue to handle any transient errors.
		if utils.IsUnsupportedConfigError(err) {
			errMessage := fmt.Sprintf("found unsupported backend configuration, "+
				"needs manual intervention to fix the issue; "+
				"error syncing '%v', not requeuing; %v", keyItem.key, err.Error())

			c.workqueue.Forget(keyItem)

			Logx(keyItem.ctx).Errorf(errMessage)
			log.Info("-------------------------------------------------")
			log.Info("-------------------------------------------------")

			return fmt.Errorf(errMessage)
		} else if utils.IsReconcileIncompleteError(err) {
			c.workqueue.Add(*keyItem)
		} else {
			c.workqueue.AddRateLimited(*keyItem)
		}

		errMessage := fmt.Sprintf("error syncing backend configuration '%v', requeuing; %v",
			keyItem.key, err.Error())
		Logx(keyItem.ctx).Errorf(errMessage)
		log.Info("-------------------------------------------------")
		log.Info("-------------------------------------------------")

		return fmt.Errorf(errMessage)
	}

	return nil
}

// handleTridentBackendConfig compares the actual state with the desired, and attempts to converge the two.
// It then updates the Status block of the TridentBackendConfig resource with the current status of the resource.
func (c *TridentCrdController) handleTridentBackendConfig(keyItem *KeyItem) error {
	if keyItem == nil {
		return fmt.Errorf("keyItem item is nil")
	}

	key := keyItem.key
	ctx := keyItem.ctx
	eventType := keyItem.event
	objectType := keyItem.objectType

	Logx(ctx).WithFields(log.Fields{
		"Key":        key,
		"eventType":  eventType,
		"objectType": objectType,
	}).Debug("TridentCrdController#handleTridentBackendConfig")

	var backendConfig *tridentv1.TridentBackendConfig

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		Logx(ctx).WithField("key", key).Error("Invalid key.")
		return nil
	}

	// Get the CR with this namespace/name
	backendConfig, err = c.backendConfigsLister.TridentBackendConfigs(namespace).Get(name)
	if err != nil {
		// The resource may no longer exist, in which case we stop processing.
		if errors.IsNotFound(err) {
			Logx(ctx).WithField("key", key).Debug("Object in work queue no longer exists.")
			return nil
		}
		return err
	}

	// Ensure backendconfig is not deleting, then ensure it has a finalizer
	if backendConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		backendConfigCopy := backendConfig.DeepCopy()

		if !backendConfigCopy.HasTridentFinalizers() {
			Logx(ctx).WithField("backendConfig.Name", backendConfigCopy.Name).Debugf("Adding finalizer.")
			backendConfigCopy.AddTridentFinalizers()

			if backendConfig, err = c.updateTridentBackendConfigCR(ctx, backendConfigCopy); err != nil {
				return fmt.Errorf("error setting finalizer; %v", err)
			}
		}
	} else {
		Logx(ctx).WithFields(log.Fields{
			"backendConfig.Name":                   backendConfig.Name,
			"backend.ObjectMeta.DeletionTimestamp": backendConfig.ObjectMeta.DeletionTimestamp,
		}).Debug("TridentCrdController#handleTridentBackendConfig CR is being deleted, not updating.")
		eventType = EventDelete
	}

	// Check to see if the backend config deletion was initialized in any of the previous reconcileBackendConfig loops
	if backendConfig.Status.Phase == string(tridentv1.PhaseDeleting) {
		Logx(ctx).WithFields(log.Fields{
			"backendConfig.Name":                         backendConfig.Name,
			"backendConfig.ObjectMeta.DeletionTimestamp": backendConfig.ObjectMeta.DeletionTimestamp,
		}).Debugf("TridentCrdController# CR has %s phase.", string(tridentv1.PhaseDeleting))
		eventType = EventDelete
	}

	// Ensure we have a valid Spec, and fields are properly set
	if err = backendConfig.Validate(); err != nil {
		backendInfo := tridentv1.TridentBackendConfigBackendInfo{
			BackendName: backendConfig.Status.BackendInfo.BackendName,
			BackendUUID: backendConfig.Status.BackendInfo.BackendUUID,
		}

		newStatus := tridentv1.TridentBackendConfigStatus{
			Message:             fmt.Sprintf("Failed to process backend: %v", err),
			BackendInfo:         backendInfo,
			Phase:               backendConfig.Status.Phase,
			DeletionPolicy:      backendConfig.Status.DeletionPolicy,
			LastOperationStatus: OperationStatusFailed,
		}

		if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus, "Failed to process backend.",
			corev1.EventTypeWarning); statusErr != nil {
			err = fmt.Errorf(
				"validation error: %v, Also encountered error while updating the status: %v", err, statusErr)
		}

		return utils.UnsupportedConfigError(err)
	}

	// Retrieve the deletion policy
	deletionPolicy, err := backendConfig.Spec.GetDeletionPolicy()
	if err != nil {
		newStatus := tridentv1.TridentBackendConfigStatus{
			Message:             fmt.Sprintf("Failed to retrieve deletion policy: %v", err),
			BackendInfo:         backendConfig.Status.BackendInfo,
			Phase:               backendConfig.Status.Phase,
			DeletionPolicy:      backendConfig.Status.DeletionPolicy,
			LastOperationStatus: OperationStatusFailed,
		}

		if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus,
			"Failed to retrieve deletion policy.", corev1.EventTypeWarning); statusErr != nil {
			err = fmt.Errorf("errror during deletion policy retrieval: %v, "+
				"Also encountered error while updating the status: %v", err, statusErr)
		}

		return fmt.Errorf("encountered error while retrieving the deletion policy :%v", err)
	}

	Logx(ctx).WithFields(log.Fields{
		"Spec": backendConfig.Spec.ToString(),
	}).Debug("TridentBackendConfig Spec is valid.")

	phase := tridentv1.TridentBackendConfigPhase(backendConfig.Status.Phase)

	if eventType == EventDelete {
		phase = tridentv1.PhaseDeleting
	} else if eventType == EventForceUpdate {
		// could also be Lost or Unknown, aim is to run an update
		phase = tridentv1.PhaseBound
	}

	switch phase {
	case tridentv1.PhaseUnbound:
		// We add a new backend or bind to an existing one
		err = c.addBackendConfig(ctx, backendConfig, deletionPolicy)
		if err != nil {
			return err
		}
	case tridentv1.PhaseBound, tridentv1.PhaseLost, tridentv1.PhaseUnknown:
		// We update the CR
		err = c.updateBackendConfig(ctx, backendConfig, deletionPolicy)
		if err != nil {
			return err
		}
	case tridentv1.PhaseDeleting:
		// We delete the CR
		err = c.deleteBackendConfig(ctx, backendConfig, deletionPolicy)
		if err != nil {
			return err
		}
	default:
		// This should never be the case
		return utils.UnsupportedConfigError(fmt.Errorf("backend config has an unsupported phase: '%v'", phase))
	}

	return nil
}

func (c *TridentCrdController) addBackendConfig(
	ctx context.Context, backendConfig *tridentv1.TridentBackendConfig, deletionPolicy string,
) error {
	Logx(ctx).WithFields(log.Fields{
		"backendConfig.Name": backendConfig.Name,
	}).Debug("TridentCrdController#addBackendConfig")

	rawJSONData := backendConfig.Spec.Raw
	Logx(ctx).WithFields(log.Fields{
		"backendConfig.Name": backendConfig.Name,
		"backendConfig.UID":  backendConfig.UID,
	}).Debug("Adding backend in core.")

	backendDetails, err := c.orchestrator.AddBackend(ctx, string(rawJSONData), string(backendConfig.UID))
	if err != nil {
		newStatus := tridentv1.TridentBackendConfigStatus{
			Message:             fmt.Sprintf("Failed to create backend: %v", err),
			BackendInfo:         c.getBackendInfo(ctx, "", ""),
			Phase:               string(tridentv1.PhaseUnbound),
			DeletionPolicy:      deletionPolicy,
			LastOperationStatus: OperationStatusFailed,
		}

		if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus, "Failed to create backend.",
			corev1.EventTypeWarning); statusErr != nil {
			err = fmt.Errorf("failed to create backend: %v; Also encountered error while updating the status: %v", err,
				statusErr)
		}
	} else {
		newStatus := tridentv1.TridentBackendConfigStatus{
			Message:             fmt.Sprintf("Backend '%v' created", backendDetails.Name),
			BackendInfo:         c.getBackendInfo(ctx, backendDetails.Name, backendDetails.BackendUUID),
			Phase:               string(tridentv1.PhaseBound),
			DeletionPolicy:      deletionPolicy,
			LastOperationStatus: OperationStatusSuccess,
		}

		if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus, "Backend created.",
			corev1.EventTypeNormal); statusErr != nil {
			err = fmt.Errorf("encountered an error while updating the status: %v", statusErr)
		}
	}

	return err
}

func (c *TridentCrdController) updateBackendConfig(
	ctx context.Context, backendConfig *tridentv1.TridentBackendConfig, deletionPolicy string,
) error {
	logFields := log.Fields{
		"backendConfig.Name": backendConfig.Name,
		"backendConfig.UID":  backendConfig.UID,
		"backendName":        backendConfig.Status.BackendInfo.BackendName,
		"backendUUID":        backendConfig.Status.BackendInfo.BackendUUID,
	}

	Logx(ctx).WithFields(logFields).Debug("TridentCrdController#updateBackendConfig")

	var phase tridentv1.TridentBackendConfigPhase
	var backend *tridentv1.TridentBackend
	var err error

	if backend, err = c.getTridentBackend(ctx, backendConfig.Namespace, backendConfig.Status.
		BackendInfo.BackendUUID); err != nil {
		Logx(ctx).WithFields(logFields).Errorf("Unable to identify if the backend exists or not; %v", err)
		err = fmt.Errorf("unable to identify if the backend exists or not; %v", err)

		phase = tridentv1.PhaseUnknown
	} else if backend == nil {
		Logx(ctx).WithFields(logFields).Errorf("Could not find backend during update.")
		err = utils.UnsupportedConfigError(fmt.Errorf("could not find backend during update"))

		phase = tridentv1.PhaseLost
	} else {
		Logx(ctx).WithFields(logFields).Debug("Updating backend in core.")
		rawJSONData := backendConfig.Spec.Raw

		var backendDetails *storage.BackendExternal
		backendDetails, err = c.orchestrator.UpdateBackendByBackendUUID(ctx,
			backendConfig.Status.BackendInfo.BackendName, string(rawJSONData),
			backendConfig.Status.BackendInfo.BackendUUID, string(backendConfig.UID))
		if err != nil {
			phase = tridentv1.TridentBackendConfigPhase(backendConfig.Status.Phase)

			if utils.IsNotFoundError(err) {
				Logx(ctx).WithFields(log.Fields{
					"backendConfig.Name": backendConfig.Name,
				}).Error("Could not find backend during update.")

				phase = tridentv1.PhaseLost
				err = fmt.Errorf("could not find backend during update; %v", err)
			}
		} else {
			newStatus := tridentv1.TridentBackendConfigStatus{
				Message:             fmt.Sprintf("Backend '%v' updated", backendDetails.Name),
				BackendInfo:         c.getBackendInfo(ctx, backendDetails.Name, backendDetails.BackendUUID),
				Phase:               string(tridentv1.PhaseBound),
				DeletionPolicy:      deletionPolicy,
				LastOperationStatus: OperationStatusSuccess,
			}

			if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus, "Backend updated.",
				corev1.EventTypeNormal); statusErr != nil {
				return fmt.Errorf("encountered an error while updating the status: %v", statusErr)
			}
		}
	}

	if err != nil {
		newStatus := tridentv1.TridentBackendConfigStatus{
			Message:             fmt.Sprintf("Failed to apply the backend update; %v", err),
			BackendInfo:         backendConfig.Status.BackendInfo,
			Phase:               string(phase),
			DeletionPolicy:      backendConfig.Status.DeletionPolicy,
			LastOperationStatus: OperationStatusFailed,
		}

		if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus,
			"Failed to apply the backend update.", corev1.EventTypeWarning); statusErr != nil {
			err = fmt.Errorf(
				"failed to update backend: %v; Also encountered error while updating the status: %v", err, statusErr)
		}
	}

	return err
}

func (c *TridentCrdController) deleteBackendConfig(
	ctx context.Context, backendConfig *tridentv1.TridentBackendConfig, deletionPolicy string,
) error {
	logFields := log.Fields{
		"backendConfig.Name": backendConfig.Name,
		"backendName":        backendConfig.Status.BackendInfo.BackendName,
		"backendUUID":        backendConfig.Status.BackendInfo.BackendUUID,
		"phase":              backendConfig.Status.Phase,
		"deletionPolicy":     deletionPolicy,
		"deletionTimeStamp":  backendConfig.ObjectMeta.DeletionTimestamp,
	}

	Logx(ctx).WithFields(logFields).Debug("TridentCrdController#deleteBackendConfig")

	if backendConfig.Status.Phase == string(tridentv1.PhaseUnbound) {
		// Originally we were doing the same for the phase=lost but it does not remove configRef
		// from the in-memory object, thus making it hard to remove it using tridentctl
		Logx(ctx).Debugf("Deleting the CR with the status '%v'", backendConfig.Status.Phase)
	} else {
		Logx(ctx).WithFields(logFields).Debug("Attempting to delete backend.")

		if deletionPolicy == tridentv1.BackendDeletionPolicyDelete {

			message, phase, err := c.deleteBackendConfigUsingPolicyDelete(ctx, backendConfig, logFields)
			if err != nil {
				lastOperationStatus := OperationStatusFailed
				if phase == tridentv1.PhaseDeleting {
					lastOperationStatus = OperationStatusSuccess
				}

				newStatus := tridentv1.TridentBackendConfigStatus{
					Message:             message,
					BackendInfo:         backendConfig.Status.BackendInfo,
					Phase:               string(phase),
					DeletionPolicy:      deletionPolicy,
					LastOperationStatus: lastOperationStatus,
				}

				if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus,
					"Failed to delete backend.", corev1.EventTypeWarning); statusErr != nil {
					return fmt.Errorf("backend err: %v and status update err: %v", err, statusErr)
				}

				return err
			}

			Logx(ctx).Debugf("Backend '%v' deleted.", backendConfig.Status.BackendInfo.BackendName)
		} else if deletionPolicy == tridentv1.BackendDeletionPolicyRetain {

			message, phase, err := c.deleteBackendConfigUsingPolicyRetain(ctx, backendConfig, logFields)
			if err != nil {
				newStatus := tridentv1.TridentBackendConfigStatus{
					Message:             message,
					BackendInfo:         backendConfig.Status.BackendInfo,
					Phase:               string(phase),
					DeletionPolicy:      deletionPolicy,
					LastOperationStatus: OperationStatusFailed,
				}

				if _, statusErr := c.updateTbcEventAndStatus(ctx, backendConfig, newStatus,
					"Failed to remove configRef from backend.", corev1.EventTypeWarning); statusErr != nil {
					return fmt.Errorf("backend err: %v and status update err: %v", err, statusErr)
				}

				return err
			}
		}
	}

	Logx(ctx).Debugf("Removing TridentBackendConfig '%v' finalizers.", backendConfig.Name)
	return c.removeFinalizers(ctx, backendConfig, false)
}

func (c *TridentCrdController) deleteBackendConfigUsingPolicyDelete(
	ctx context.Context, backendConfig *tridentv1.TridentBackendConfig, logFields map[string]interface{},
) (string, tridentv1.TridentBackendConfigPhase, error) {
	var phase tridentv1.TridentBackendConfigPhase
	var backend *tridentv1.TridentBackend
	var message string
	var err error

	// Deletion Logic:
	// 1. First check if the backend exists, if not delete the TBC CR
	// 2. If it does, check if it is in a deleting state, if it is then do not re-run deletion logic in the core.
	// 3. If it is not in a deleting state, then run the deletion logic in the core.
	// 4. After running the deletion logic in the core check again if the backend still exists,
	//    if not delete the TBC CR.
	// 5. If it still exists, then fail and set `Status.phase=Deleting`.
	if backend, err = c.getTridentBackend(ctx, backendConfig.Namespace, backendConfig.Status.
		BackendInfo.BackendUUID); err != nil {

		message = fmt.Sprintf("Unable to identify if the backend is deleted or not; %v", err)
		phase = tridentv1.PhaseUnknown

		Logx(ctx).WithFields(logFields).Errorf(message)
		err = fmt.Errorf("unable to identify if the backend '%v' is deleted or not; %v",
			backendConfig.Status.BackendInfo.BackendName, err)
	} else if backend == nil {
		Logx(ctx).WithFields(logFields).Debug("Backend not found, proceeding with the TridentBackendConfig deletion.")

		// In the lost case ensure the backend is deleted with deletionPolicy `delete`
		if backendConfig.Status.Phase == string(tridentv1.PhaseLost) {
			Logx(ctx).WithFields(logFields).Debugf("Attempting to remove in-memory backend object.")
			if deleteErr := c.orchestrator.DeleteBackendByBackendUUID(ctx, backendConfig.Status.BackendInfo.BackendName,
				backendConfig.Status.BackendInfo.BackendUUID); deleteErr != nil {
				Logx(ctx).WithFields(logFields).Warnf("unable to delete backend: %v: %v",
					backendConfig.Status.BackendInfo.BackendName, deleteErr)
			}
		}

		// Attempt to remove configRef for all the cases because this tbc will be gone
		Logx(ctx).WithFields(logFields).Debugf("Attempting to remove configRef from in-memory backend object.")
		_ = c.orchestrator.RemoveBackendConfigRef(ctx, backendConfig.Status.BackendInfo.BackendUUID,
			string(backendConfig.UID))
	} else if backend.State == string(storage.Deleting) {
		message = "Backend is in a deleting state, cannot proceed with the TridentBackendConfig deletion. "
		phase = tridentv1.PhaseDeleting

		Logx(ctx).WithFields(logFields).Errorf(message + "Re-adding this work item back to the queue.")
		err = fmt.Errorf("backend is in a deleting state, cannot proceed with the TridentBackendConfig deletion")
	} else {
		Logx(ctx).WithFields(logFields).Debug("Backend is present and not in a deleting state, " +
			"proceeding with the backend deletion.")

		if err = c.orchestrator.DeleteBackendByBackendUUID(ctx, backendConfig.Status.BackendInfo.BackendName,
			backendConfig.Status.
				BackendInfo.BackendUUID); err != nil {

			phase = tridentv1.TridentBackendConfigPhase(backendConfig.Status.Phase)
			err = fmt.Errorf("unable to delete backend '%v'; %v", backendConfig.Status.BackendInfo.BackendName, err)

			if !utils.IsNotFoundError(err) {
				message = fmt.Sprintf("Unable to delete backend; %v", err)
				Logx(ctx).WithFields(logFields).Errorf(message)
			} else {
				// In the next reconcile loop the above condition `backend == nil` should be true if backend
				// is not present in-memory as well as the tbe CR
				message = "Could not find backend during deletion."
				Logx(ctx).WithFields(logFields).Debug(message)
			}
		} else {

			// Wait 2 seconds before checking again backend is gone or not.
			time.Sleep(2 * time.Second)

			// Ensure backend does not exist
			if backend, err = c.getTridentBackend(ctx, backendConfig.Namespace, backendConfig.Status.
				BackendInfo.BackendUUID); err != nil {
				message = fmt.Sprintf("Unable to ensure backend deletion.; %v", err)
				phase = tridentv1.PhaseUnknown

				Logx(ctx).WithFields(logFields).Errorf("Unable to ensure backend deletion; %v", err)
				err = fmt.Errorf("unable to ensure backend '%v' deletion; %v",
					backendConfig.Status.BackendInfo.BackendName,
					err)
			} else if backend != nil {
				message = "Backend still present after a deletion attempt"
				phase = tridentv1.PhaseDeleting

				Logx(ctx).WithFields(logFields).Errorf("Backend still present after a deletion attempt. " +
					"Re-adding this work item back to the queue to try again.")
				err = fmt.Errorf("backend '%v' still present after a deletion attempt",
					backendConfig.Status.BackendInfo.BackendName)
			} else {
				Logx(ctx).WithFields(logFields).Info("Backend deleted.")
			}
		}
	}

	return message, phase, err
}

func (c *TridentCrdController) deleteBackendConfigUsingPolicyRetain(
	ctx context.Context, backendConfig *tridentv1.TridentBackendConfig, logFields map[string]interface{},
) (string, tridentv1.TridentBackendConfigPhase, error) {
	var phase tridentv1.TridentBackendConfigPhase
	var backend *tridentv1.TridentBackend
	var message string
	var err error

	// Deletion Logic:
	// 1. First check if the backend exists, if not delete the TBC CR
	// 2. If it does, check if it has a configRef field set, if not delete the TBC CR
	// 3. If the configRef field is set, then run the remove configRef logic in the core.
	if backend, err = c.getTridentBackend(ctx, backendConfig.Namespace, backendConfig.Status.
		BackendInfo.BackendUUID); err != nil {

		message = fmt.Sprintf("Unable to identify if the backend exists or not; %v", err)
		phase = tridentv1.PhaseUnknown

		Logx(ctx).WithFields(logFields).Errorf(message)
		err = fmt.Errorf("unable to identify if the backend '%v' exists or not; %v",
			backendConfig.Status.BackendInfo.BackendName, err)
	} else if backend == nil {
		Logx(ctx).WithFields(logFields).Debug("Backend not found, " +
			"proceeding with the TridentBackendConfig deletion.")

		Logx(ctx).WithFields(logFields).Debugf("Attempting to remove configRef from in-memory backend object.")
		_ = c.orchestrator.RemoveBackendConfigRef(ctx, backendConfig.Status.BackendInfo.BackendUUID,
			string(backendConfig.UID))
	} else if backend.ConfigRef == "" {
		Logx(ctx).WithFields(logFields).Debug("Backend found " +
			"but does not contain a configRef, proceeding with the TridentBackendConfig deletion.")
	} else {
		if err = c.orchestrator.RemoveBackendConfigRef(ctx, backendConfig.Status.BackendInfo.BackendUUID,
			string(backendConfig.UID)); err != nil {

			phase = tridentv1.TridentBackendConfigPhase(backendConfig.Status.Phase)
			err = fmt.Errorf("failed to remove configRef from the backend '%v'; %v",
				backendConfig.Status.BackendInfo.BackendName, err)

			if !utils.IsNotFoundError(err) {
				message = fmt.Sprintf("Failed to remove configRef from the backend; %v", err)
				Logx(ctx).WithFields(logFields).Errorf(message)
			} else {
				// In the next reconcile loop the above condition `backend == nil` should be true if backend
				// is not present in-memory as well as the tbe CR
				message = "Could not find backend during configRef removal."
				Logx(ctx).WithFields(logFields).Debug(message)
			}
		} else {
			Logx(ctx).WithFields(logFields).Info("ConfigRef removed from the backend; backend not deleted.")
		}
	}

	return message, phase, err
}

// getTridentBackend returns a TridentBackend CR with the given backendUUID
func (c *TridentCrdController) getTridentBackend(
	ctx context.Context, namespace, backendUUID string,
) (*tridentv1.TridentBackend, error) {
	// Get list of all the backends
	backendList, err := c.crdClientset.TridentV1().TridentBackends(namespace).List(ctx, listOpts)
	if err != nil {
		Logx(ctx).WithFields(log.Fields{
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

// getBackendConfigWithBackendUUID identifies if the backend config referencing a given backendUUID exists or not
func (c *TridentCrdController) getBackendConfigWithBackendUUID(
	ctx context.Context, namespace, backendUUID string,
) (*tridentv1.TridentBackendConfig, error) {
	// Get list of all the backend configs
	backendConfigList, err := c.crdClientset.TridentV1().TridentBackendConfigs(namespace).List(ctx, listOpts)
	if err != nil {
		Logx(ctx).WithFields(log.Fields{
			"backendUUID": backendUUID,
			"namespace":   namespace,
			"err":         err,
		}).Errorf("Error listing Trident backendConfig configs.")
		return nil, fmt.Errorf("error listing Trident backendConfig configs: %v", err)
	}

	for _, backendConfig := range backendConfigList.Items {
		if backendConfig.Status.BackendInfo.BackendUUID == backendUUID {
			return backendConfig, nil
		}
	}

	return nil, utils.NotFoundError("backend config not found")
}

// getBackendConfigsWithSecret identifies the backend configs referencing a given secret (if any)
func (c *TridentCrdController) getBackendConfigsWithSecret(
	ctx context.Context, namespace, secret string,
) ([]*tridentv1.TridentBackendConfig, error) {
	var backendConfigs []*tridentv1.TridentBackendConfig

	// Get list of all the backend configs
	backendConfigList, err := c.crdClientset.TridentV1().TridentBackendConfigs(namespace).List(ctx, listOpts)
	if err != nil {
		Logx(ctx).WithFields(log.Fields{
			"err": err,
		}).Errorf("Error listing Trident backendConfig configs.")
		return backendConfigs, fmt.Errorf("error listing Trident backendConfig configs: %v", err)
	}

	for _, backendConfig := range backendConfigList.Items {
		if secretName, err := backendConfig.Spec.GetSecretName(); err == nil && secretName != "" {
			if secretName == secret {
				backendConfigs = append(backendConfigs, backendConfig)
			}
		}
	}

	if len(backendConfigs) > 0 {
		return backendConfigs, nil
	}

	return backendConfigs, utils.NotFoundError("no backend config with the matching secret found")
}

// getBackendInfo creates TridentBackendConfigBackendInfo object based on the provided information
func (c *TridentCrdController) getBackendInfo(
	_ context.Context, backendName, backendUUID string,
) tridentv1.TridentBackendConfigBackendInfo {
	return tridentv1.TridentBackendConfigBackendInfo{
		BackendName: backendName,
		BackendUUID: backendUUID,
	}
}

// updateLogAndStatus updates the event logs and status of a TridentOrchestrator CR (if required)
func (c *TridentCrdController) updateTbcEventAndStatus(
	ctx context.Context, tbcCR *tridentv1.TridentBackendConfig,
	newStatus tridentv1.TridentBackendConfigStatus, debugMessage, eventType string,
) (tbcCRNew *tridentv1.TridentBackendConfig, err error) {
	var logEvent bool

	if tbcCRNew, logEvent, err = c.updateTridentBackendConfigCRStatus(ctx, tbcCR, newStatus, debugMessage); err != nil {
		return
	}

	// Log event only when phase has been updated or a event type  warning has occurred
	if logEvent || eventType == corev1.EventTypeWarning {
		c.recorder.Event(tbcCR, eventType, newStatus.LastOperationStatus, newStatus.Message)
	}

	return
}

// updateTridentBackendConfigCRStatus updates the status of a CR if required
func (c *TridentCrdController) updateTridentBackendConfigCRStatus(
	ctx context.Context,
	tbcCR *tridentv1.TridentBackendConfig, newStatusDetails tridentv1.TridentBackendConfigStatus, debugMessage string,
) (*tridentv1.TridentBackendConfig, bool, error) {
	logFields := log.Fields{"TridentBackendConfigCR": tbcCR.Name}

	// Update phase of the tbcCR
	Logx(ctx).WithFields(logFields).Debug(debugMessage)

	if reflect.DeepEqual(tbcCR.Status, newStatusDetails) {
		log.WithFields(logFields).Info("New status is same as the old phase, no status update needed.")

		return tbcCR, false, nil
	}

	crClone := tbcCR.DeepCopy()
	crClone.Status = newStatusDetails

	// client-go does not provide r.Status().Patch which would have been ideal, something to do if we switch to using
	// controller-runtime.
	newTbcCR, err := c.crdClientset.TridentV1().TridentBackendConfigs(tbcCR.Namespace).UpdateStatus(
		ctx, crClone, updateOpts)
	if err != nil {
		Logx(ctx).WithFields(logFields).Errorf("Could not update status of the CR; %v", err)
		// If this is due to CR deletion, ensure it is handled properly for the CRs moving from unbound to bound phase.
		if newTbcCR = c.checkAndHandleNewlyBoundCRDeletion(ctx, tbcCR, newStatusDetails); newTbcCR != nil {
			err = nil
		}
	}

	return newTbcCR, true, err
}

// checkAndHandleNewlyBoundCRDeletion handles a corner cases where tbc is delete immediately after creation
// which may result in tbc-only deletion in next reconcile without the tbe or in-memory backend deletion
func (c *TridentCrdController) checkAndHandleNewlyBoundCRDeletion(
	ctx context.Context,
	tbcCR *tridentv1.TridentBackendConfig, newStatusDetails tridentv1.TridentBackendConfigStatus,
) *tridentv1.TridentBackendConfig {
	logFields := log.Fields{"TridentBackendConfigCR": tbcCR.Name}
	var newTbcCR *tridentv1.TridentBackendConfig

	// Need to handle a scenario where a new TridentBackendConfig gets deleted immediately after creation.
	// When new tbc is created it results in a new tbe creation or binding but tbc however runs into a
	// failure to tbc CR deletion.
	// In this scenario the subsequent reconcile needs to have the knowledge of the backend name,
	// backendUUID to ensure proper deletion of tbe and in-memory backend.
	if tbcCR.Status.Phase == string(tridentv1.PhaseUnbound) && newStatusDetails.Phase == string(tridentv1.
		PhaseBound) {
		// Get the updated copy of the tbc CR and check if it is deleting
		updatedTbcCR, err := c.backendConfigsLister.TridentBackendConfigs(tbcCR.Namespace).Get(tbcCR.Name)
		if err != nil {
			Logx(ctx).WithFields(logFields).Errorf("encountered an error while getting the latest CR update: %v", err)
			return nil
		}

		var updateStatus bool
		if !updatedTbcCR.ObjectMeta.DeletionTimestamp.IsZero() {
			// tbc CR has been marked for deletion
			Logx(ctx).WithFields(logFields).Debugf("This CR is deleting, re-attempting to update the status.")
			updateStatus = true
		} else {
			Logx(ctx).WithFields(logFields).Debugf("CR is not deleting.")
		}

		if updateStatus {
			crClone := updatedTbcCR.DeepCopy()
			crClone.Status = newStatusDetails
			newTbcCR, err = c.crdClientset.TridentV1().TridentBackendConfigs(tbcCR.Namespace).UpdateStatus(
				ctx, crClone, updateOpts)
			if err != nil {
				Logx(ctx).WithFields(logFields).Errorf("another attempt to update the status failed: %v; "+
					"backend (name: %v, backendUUID: %v) requires manual intervention", err,
					newStatusDetails.BackendInfo.BackendName, newStatusDetails.BackendInfo.BackendUUID)
			} else {
				Logx(ctx).WithFields(logFields).Debugf("Status updated.")
			}
		}
	}

	return newTbcCR
}

// updateTridentBackendConfigCR updates the TridentBackendConfigCR
func (c *TridentCrdController) updateTridentBackendConfigCR(
	ctx context.Context, tbcCR *tridentv1.TridentBackendConfig,
) (*tridentv1.TridentBackendConfig, error) {
	logFields := log.Fields{"TridentBackendConfigCR": tbcCR.Name}

	// Update phase of the tbcCR
	Logx(ctx).WithFields(logFields).Debug("Updating the TridentBackendConfig CR")

	newTbcCR, err := c.crdClientset.TridentV1().TridentBackendConfigs(tbcCR.Namespace).Update(ctx, tbcCR, updateOpts)
	if err != nil {
		Logx(ctx).WithFields(logFields).Errorf("could not update TridentBackendConfig CR; %v", err)
	}

	return newTbcCR, err
}

// removeFinalizers removes Trident's finalizers from Trident CRs
func (c *TridentCrdController) removeFinalizers(ctx context.Context, obj interface{}, force bool) error {
	/*
		log.WithFields(log.Fields{
			"obj":   obj,
			"force": force,
		}).Debug("removeFinalizers")
	*/

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
	Logx(ctx).WithFields(log.Fields{
		"backend.ResourceVersion":              backend.ResourceVersion,
		"backend.ObjectMeta.DeletionTimestamp": backend.ObjectMeta.DeletionTimestamp,
	}).Debug("removeBackendFinalizers")

	if backend.HasTridentFinalizers() {
		Logx(ctx).Debug("Has finalizers, removing them.")
		backendCopy := backend.DeepCopy()
		backendCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentBackends(backend.Namespace).Update(ctx, backendCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Debug("No finalizers to remove.")
	}

	return
}

// removeBackendConfigFinalizers removes TridentConfig's finalizers from TridentBackendconfig CRs
func (c *TridentCrdController) removeBackendConfigFinalizers(
	ctx context.Context, tbc *tridentv1.TridentBackendConfig,
) (err error) {
	Logx(ctx).WithFields(log.Fields{
		"tbc.ResourceVersion":              tbc.ResourceVersion,
		"tbc.ObjectMeta.DeletionTimestamp": tbc.ObjectMeta.DeletionTimestamp,
	}).Debug("removeBackendConfigFinalizers")

	if tbc.HasTridentFinalizers() {
		Logx(ctx).Debug("Has finalizers, removing them.")
		backendConfigCopy := tbc.DeepCopy()
		backendConfigCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentBackendConfigs(tbc.Namespace).Update(ctx, backendConfigCopy,
			updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Debug("No finalizers to remove.")
	}

	return
}

// removeNodeFinalizers removes Trident's finalizers from TridentNode CRs
func (c *TridentCrdController) removeNodeFinalizers(ctx context.Context, node *tridentv1.TridentNode) (err error) {
	Logx(ctx).WithFields(log.Fields{
		"node.ResourceVersion":              node.ResourceVersion,
		"node.ObjectMeta.DeletionTimestamp": node.ObjectMeta.DeletionTimestamp,
	}).Debug("removeNodeFinalizers")

	if node.HasTridentFinalizers() {
		Logx(ctx).Debug("Has finalizers, removing them.")
		nodeCopy := node.DeepCopy()
		nodeCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentNodes(node.Namespace).Update(ctx, nodeCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Debug("No finalizers to remove.")
	}

	return
}

// removeStorageClassFinalizers removes Trident's finalizers from TridentStorageClass CRs
func (c *TridentCrdController) removeStorageClassFinalizers(
	ctx context.Context, sc *tridentv1.TridentStorageClass,
) (err error) {
	Logx(ctx).WithFields(log.Fields{
		"sc.ResourceVersion":              sc.ResourceVersion,
		"sc.ObjectMeta.DeletionTimestamp": sc.ObjectMeta.DeletionTimestamp,
	}).Debug("removeStorageClassFinalizers")

	if sc.HasTridentFinalizers() {
		Logx(ctx).Debug("Has finalizers, removing them.")
		scCopy := sc.DeepCopy()
		scCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentStorageClasses(sc.Namespace).Update(ctx, scCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Debug("No finalizers to remove.")
	}

	return
}

// removeTransactionFinalizers removes Trident's finalizers from TridentTransaction CRs
func (c *TridentCrdController) removeTransactionFinalizers(
	ctx context.Context, tx *tridentv1.TridentTransaction,
) (err error) {
	Logx(ctx).WithFields(log.Fields{
		"tx.ResourceVersion":              tx.ResourceVersion,
		"tx.ObjectMeta.DeletionTimestamp": tx.ObjectMeta.DeletionTimestamp,
	}).Debug("removeTransactionFinalizers")

	if tx.HasTridentFinalizers() {
		Logx(ctx).Debug("Has finalizers, removing them.")
		txCopy := tx.DeepCopy()
		txCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentTransactions(tx.Namespace).Update(ctx, txCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Debug("No finalizers to remove.")
	}

	return
}

// removeVersionFinalizers removes Trident's finalizers from TridentVersion CRs
func (c *TridentCrdController) removeVersionFinalizers(ctx context.Context, v *tridentv1.TridentVersion) (err error) {
	Logx(ctx).WithFields(log.Fields{
		"v.ResourceVersion":              v.ResourceVersion,
		"v.ObjectMeta.DeletionTimestamp": v.ObjectMeta.DeletionTimestamp,
	}).Debug("removeVersionFinalizers")

	if v.HasTridentFinalizers() {
		Logx(ctx).Debug("Has finalizers, removing them.")
		vCopy := v.DeepCopy()
		vCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentVersions(v.Namespace).Update(ctx, vCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Debug("No finalizers to remove.")
	}

	return
}

// removeVolumeFinalizers removes Trident's finalizers from TridentVolume CRs
func (c *TridentCrdController) removeVolumeFinalizers(ctx context.Context, vol *tridentv1.TridentVolume) (err error) {
	Logx(ctx).WithFields(log.Fields{
		"vol.ResourceVersion":              vol.ResourceVersion,
		"vol.ObjectMeta.DeletionTimestamp": vol.ObjectMeta.DeletionTimestamp,
	}).Debug("removeVolumeFinalizers")

	if vol.HasTridentFinalizers() {
		Logx(ctx).Debug("Has finalizers, removing them.")
		volCopy := vol.DeepCopy()
		volCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentVolumes(vol.Namespace).Update(ctx, volCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Debug("No finalizers to remove.")
	}

	return
}

// removeVolumePublicationFinalizers removes Trident's finalizers from TridentVolumePublication CRs
func (c *TridentCrdController) removeVolumePublicationFinalizers(
	ctx context.Context, volPub *tridentv1.TridentVolumePublication,
) (err error) {
	Logx(ctx).WithFields(log.Fields{
		"volPub.ResourceVersion":              volPub.ResourceVersion,
		"volPub.ObjectMeta.DeletionTimestamp": volPub.ObjectMeta.DeletionTimestamp,
	}).Debug("removeVolumePublicationFinalizers")

	if volPub.HasTridentFinalizers() {
		Logx(ctx).Debug("Has finalizers, removing them.")
		volCopy := volPub.DeepCopy()
		volCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentVolumePublications(volPub.Namespace).Update(ctx, volCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Debug("No finalizers to remove.")
	}

	return
}

// removeSnapshotFinalizers removes Trident's finalizers from TridentSnapshot CRs
func (c *TridentCrdController) removeSnapshotFinalizers(
	ctx context.Context, snap *tridentv1.TridentSnapshot,
) (err error) {
	Logx(ctx).WithFields(log.Fields{
		"snap.ResourceVersion":              snap.ResourceVersion,
		"snap.ObjectMeta.DeletionTimestamp": snap.ObjectMeta.DeletionTimestamp,
	}).Debug("removeSnapshotFinalizers")

	if snap.HasTridentFinalizers() {
		Logx(ctx).Debug("Has finalizers, removing them.")
		snapCopy := snap.DeepCopy()
		snapCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentSnapshots(snap.Namespace).Update(ctx, snapCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Debug("No finalizers to remove.")
	}

	return
}

// removeTMRFinalizers removes Trident's finalizers from TridentMirrorRelationship CRs
func (c *TridentCrdController) removeTMRFinalizers(
	ctx context.Context, tmr *tridentv1.TridentMirrorRelationship,
) (err error) {
	Logx(ctx).WithFields(log.Fields{
		"tmr.ResourceVersion":              tmr.ResourceVersion,
		"tmr.ObjectMeta.DeletionTimestamp": tmr.ObjectMeta.DeletionTimestamp,
	}).Debug("removeTMRFinalizers")

	if tmr.HasTridentFinalizers() {
		Logx(ctx).Debug("Has finalizers, removing them.")
		tmrCopy := tmr.DeepCopy()
		tmrCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentMirrorRelationships(tmr.Namespace).Update(ctx, tmrCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Debug("No finalizers to remove.")
	}

	return
}

// removeTSIFinalizers removes Trident's finalizers from TridentSnapshotInfo CRs
func (c *TridentCrdController) removeTSIFinalizers(
	ctx context.Context, tsi *tridentv1.TridentSnapshotInfo,
) (err error) {
	Logx(ctx).WithFields(log.Fields{
		"tsi.ResourceVersion":              tsi.ResourceVersion,
		"tsi.ObjectMeta.DeletionTimestamp": tsi.ObjectMeta.DeletionTimestamp,
	}).Debug("removeTSIFinalizers")

	if tsi.HasTridentFinalizers() {
		Logx(ctx).Debug("Has finalizers, removing them.")
		tsiCopy := tsi.DeepCopy()
		tsiCopy.RemoveTridentFinalizers()
		_, err = c.crdClientset.TridentV1().TridentSnapshotInfos(tsi.Namespace).Update(ctx, tsiCopy, updateOpts)
		if err != nil {
			Logx(ctx).Errorf("Problem removing finalizers: %v", err)
			return
		}
	} else {
		Logx(ctx).Debug("No finalizers to remove.")
	}

	return
}
