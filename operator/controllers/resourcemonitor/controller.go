// Copyright 2025 NetApp, Inc. All Rights Reserved.

package resourcemonitor

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"

	. "github.com/netapp/trident/logging"
	"github.com/netapp/trident/operator/clients"
	"github.com/netapp/trident/operator/crd/client/clientset/versioned/scheme"
	"github.com/netapp/trident/utils/errors"
)

const (
	ControllerName    = "Trident Resource Monitor"
	ControllerVersion = "0.1"
)

var ctx = func() context.Context { return context.TODO() }

type ResourceType string

const (
	ResourceTypeStorageClass ResourceType = "StorageClass"
	// Add more resource types here for future extensibility
)

type Controller struct {
	Clients *clients.Clients

	eventRecorder record.EventRecorder
	stopChan      chan struct{}

	resourceHandlers map[ResourceType]ResourceHandler

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens.
	workqueue workqueue.RateLimitingInterface
}

// NewController creates a new resource monitor controller
func NewController(clientFactory *clients.Clients) (*Controller, error) {
	c := &Controller{
		Clients:          clientFactory,
		stopChan:         make(chan struct{}),
		resourceHandlers: make(map[ResourceType]ResourceHandler),
		workqueue: workqueue.NewRateLimitingQueueWithConfig(
			workqueue.DefaultControllerRateLimiter(),
			workqueue.RateLimitingQueueConfig{Name: "ResourceMonitor"},
		),
	}

	// Add our types to the default Kubernetes Scheme so Events can be logged.
	utilruntime.Must(scheme.AddToScheme(scheme.Scheme))

	// Set up event broadcaster
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&clientv1.EventSinkImpl{Interface: clientFactory.KubeClient.CoreV1().Events("")})
	c.eventRecorder = broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "trident-resource-monitor.netapp.io"})

	// Initialize StorageClass handler
	scHandler, err := NewStorageClassHandler(c)
	if err != nil {
		return nil, fmt.Errorf("failed to create StorageClass handler: %v", err)
	}
	c.resourceHandlers[ResourceTypeStorageClass] = scHandler

	return c, nil
}

// Activate starts the controller
func (c *Controller) Activate() error {
	Log().WithField("Controller", ControllerName).Info("Activating controller.")

	// Start all resource handlers
	for resourceType, handler := range c.resourceHandlers {
		Log().WithField("resourceType", resourceType).Info("Starting resource handler.")
		if err := handler.Start(c.stopChan); err != nil {
			return fmt.Errorf("failed to start handler for %s: %v", resourceType, err)
		}
	}

	// Start worker
	Log().Info("Starting workers")
	go wait.Until(c.runWorker, time.Second, c.stopChan)
	Log().Info("Started workers")

	return nil
}

// Deactivate stops the controller
func (c *Controller) Deactivate() error {
	Log().WithField("Controller", ControllerName).Info("Deactivating controller.")

	close(c.stopChan)

	c.workqueue.ShutDown()
	utilruntime.HandleCrash()
	return nil
}

// GetName returns the controller name
func (c *Controller) GetName() string {
	return ControllerName
}

// Version returns the controller version
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
// attempt to process it, by calling the reconcile handler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		defer c.workqueue.Done(obj)

		var workItem *WorkItem
		var ok bool

		if workItem, ok = obj.(*WorkItem); !ok {
			c.workqueue.Forget(obj)
			Log().Errorf("expected WorkItem in workqueue but got %#v", obj)
			return nil
		}

		// Get the handler for this resource type
		handler, exists := c.resourceHandlers[workItem.ResourceType]
		if !exists {
			c.workqueue.Forget(workItem)
			return fmt.Errorf("no handler found for resource type %s", workItem.ResourceType)
		}

		// Run the reconcile
		if err := handler.Reconcile(workItem); err != nil {
			if errors.IsUnsupportedConfigError(err) {
				errMessage := fmt.Sprintf("found unsupported configuration, "+
					"needs manual intervention; error syncing '%s/%s': %s, not requeuing",
					workItem.ResourceType, workItem.Key, err.Error())

				c.workqueue.Forget(workItem)
				Log().Errorf(errMessage)
				return errors.New(errMessage)
			} else if errors.IsReconcileIncompleteError(err) {
				c.workqueue.Add(workItem)
			} else {
				c.workqueue.AddRateLimited(workItem)
			}

			errMessage := fmt.Sprintf("error syncing '%s/%s': %s, requeuing",
				workItem.ResourceType, workItem.Key, err.Error())
			Log().Errorf(errMessage)
			return errors.New(errMessage)
		}

		c.workqueue.Forget(workItem)
		Log().Infof("Successfully synced %s '%s'", workItem.ResourceType, workItem.Key)
		return nil
	}(obj)

	if err != nil {
		Log().Error(err)
		return true
	}

	return true
}

// EnqueueWork adds a work item to the queue
func (c *Controller) EnqueueWork(workItem *WorkItem) {
	c.workqueue.Add(workItem)
}

// WorkItem represents a work item to be processed
type WorkItem struct {
	ResourceType ResourceType
	Key          string
	Operation    OperationType
	Object       runtime.Object
}

// OperationType represents the type of operation
type OperationType string

const (
	OperationAdd    OperationType = "Add"
	OperationUpdate OperationType = "Update"
	OperationDelete OperationType = "Delete"
)

// ResourceHandler interface defines methods that each resource handler must implement
type ResourceHandler interface {
	// Start starts watching the resource
	Start(stopChan chan struct{}) error

	// Reconcile reconciles the resource
	Reconcile(workItem *WorkItem) error

	// GetResourceType returns the type of resource this handler manages
	GetResourceType() ResourceType
}
