package requester

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/workqueue"

	agCache "github.com/netapp/trident/frontend/autogrow/cache"
	agTypes "github.com/netapp/trident/frontend/autogrow/types"
	. "github.com/netapp/trident/logging"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	tridentclientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	listerv1 "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	"github.com/netapp/trident/pkg/eventbus"
	eventbusTypes "github.com/netapp/trident/pkg/eventbus/types"
	"github.com/netapp/trident/pkg/workerpool"
	"github.com/netapp/trident/pkg/workerpool/ants"
	poolTypes "github.com/netapp/trident/pkg/workerpool/types"
	"github.com/netapp/trident/utils/autogrow"
	"github.com/netapp/trident/utils/errors"
)

// applyConfigDefaults validates the config and fills in missing values with defaults
func applyConfigDefaults(ctx context.Context, config *Config) {
	if config.ShutdownTimeout <= 0 {
		config.ShutdownTimeout = DefaultShutdownTimeout
		Logc(ctx).WithField("timeout", config.ShutdownTimeout).Debug("Using default shutdown timeout")
	}

	if config.WorkQueueName == "" {
		config.WorkQueueName = DefaultWorkQueueName
		Logc(ctx).WithField("name", config.WorkQueueName).Debug("Using default work queue name")
	}

	if config.TridentNamespace == "" {
		config.TridentNamespace = DefaultTridentNamespace
		Logc(ctx).WithField("namespace", config.TridentNamespace).Debug("Using default Trident namespace")
	}

	if config.MaxRetries <= 0 {
		config.MaxRetries = DefaultMaxRetries
		Logc(ctx).WithField("retries", config.MaxRetries).Debug("Using default Max Retries")
	}

	// Note: WorkerPool is optional (nil is valid, we'll create one with defaults)
}

// NewRequester creates a new Requester instance
func NewRequester(
	ctx context.Context,
	tridentClient tridentclientset.Interface,
	agpLister listerv1.TridentAutogrowPolicyLister,
	tvpLister listerv1.TridentVolumePublicationLister,
	autogrowCache *agCache.AutogrowCache,
	config *Config,
) (*Requester, error) {
	// Create a deep copy of the config to avoid mutating the caller's config
	var configCopy *Config
	if config == nil {
		configCopy = DefaultConfig()
	} else {
		configCopy = config.Copy()
	}

	logFields := LogFields{
		"trident_namespace": configCopy.TridentNamespace,
	}
	Logc(ctx).WithFields(logFields).Debug(">>>> NewRequester")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< NewRequester")

	// Validate config copy and apply defaults for any missing values
	applyConfigDefaults(ctx, configCopy)

	Logc(ctx).WithField("config", configCopy).Debug("Requester config has been initialized")

	// Determine if we'll own the worker pool or use a provided one
	// If a worker pool is provided in config, use it; otherwise we'll create one in Activate()
	var pool poolTypes.Pool
	var ownPool bool
	if configCopy.WorkerPool != nil {
		// Use the provided worker pool (external ownership)
		pool = configCopy.WorkerPool
		ownPool = false
		Logc(ctx).Debug("Using provided worker pool")
	} else {
		// Worker pool will be created in Activate() (we own it)
		pool = nil
		ownPool = true
		Logc(ctx).Debug("Worker pool will be created in Activate()")
	}

	r := &Requester{
		tridentClient: tridentClient,
		agpLister:     agpLister,
		tvpLister:     tvpLister,
		autogrowCache: autogrowCache,
		config:        configCopy,
		workerPool:    pool,
		ownWorkerPool: ownPool,
		latestEvents:  make(map[string]agTypes.VolumeThresholdBreached),
	}

	return r, nil
}

// Activate starts the Requester layer (idempotent)
// Returns error if already running or if activation fails
func (r *Requester) Activate(ctx context.Context) error {
	Logc(ctx).Debug(">>>> requester.Activate")
	defer Logc(ctx).Debug("<<<< requester.Activate")

	// Check if we can transition to Starting state
	if !r.state.compareAndSwap(StateStopped, StateStarting) {
		currentState := r.state.get()
		if currentState == StateRunning {
			Logc(ctx).Debug("Requester already running, skipping activation")
			return nil // Idempotent
		}
		return errors.NewStateError(currentState.String(), fmt.Sprintf("cannot activate requester: current state is %s", currentState.String()))
	}

	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	Logc(ctx).Info("Activating Requester layer...")

	// Create stop channel
	r.stopCh = make(chan struct{})

	// Create rate limiter combining exponential backoff with bucket rate limiting
	rateLimiter := workqueue.NewTypedMaxOfRateLimiter(
		workqueue.NewTypedItemExponentialFailureRateLimiter[WorkItem](WorkqueueBaseDelay, WorkqueueMaxDelay),
		// QPS and bucket size for retry speed and overall throughput control (not per item)
		&workqueue.TypedBucketRateLimiter[WorkItem]{Limiter: rate.NewLimiter(rate.Limit(WorkqueueBucketQPS), WorkqueueBucketBurst)},
	)

	// Create workqueue with custom rate limiting configuration
	r.workqueue = workqueue.NewTypedRateLimitingQueueWithConfig(
		rateLimiter,
		workqueue.TypedRateLimitingQueueConfig[WorkItem]{
			Name: r.config.WorkQueueName,
		},
	)

	Logc(ctx).WithFields(LogFields{
		"workQueueName": r.config.WorkQueueName,
		"baseDelay":     WorkqueueBaseDelay,
		"maxDelay":      WorkqueueMaxDelay,
		"qps":           WorkqueueBucketQPS,
		"burst":         WorkqueueBucketBurst,
	}).Debug("Created workqueue with exponential backoff and bucket rate limiting")

	// Create worker pool if we own it
	if r.ownWorkerPool {
		Logc(ctx).Info("Creating worker pool for requester")
		pool, err := workerpool.New[poolTypes.Pool](
			ctx,
			ants.NewConfig(
				ants.WithNumWorkers(defaultWorkerPoolSize),
				ants.WithPreAlloc(DefaultWorkerPoolPreAlloc),
				ants.WithNonBlocking(DefaultWorkerPoolNonBlocking),
			),
		)
		if err != nil {
			r.cleanup(ctx)
			r.state.set(StateStopped)
			return fmt.Errorf("failed to create worker pool: %w", err)
		}
		r.workerPool = pool
		Logc(ctx).WithFields(LogFields{
			"size":        defaultWorkerPoolSize,
			"preAlloc":    DefaultWorkerPoolPreAlloc,
			"nonBlocking": DefaultWorkerPoolNonBlocking,
		}).Info("Created worker pool for requester")

		// Start the worker pool (only if we own it)
		if err := r.workerPool.Start(ctx); err != nil {
			r.cleanup(ctx)
			r.state.set(StateStopped)
			return fmt.Errorf("failed to start worker pool: %w", err)
		}
		Logc(ctx).Info("Worker pool started")
	}

	// Subscribe to VolumeThresholdBreached events from the eventbus
	if eventbus.VolumeThresholdBreachedEventBus == nil {
		r.cleanup(ctx)
		r.state.set(StateStopped)
		return fmt.Errorf("eventbus not initialized")
	}
	subscriptionID, err := eventbus.VolumeThresholdBreachedEventBus.Subscribe(r.handleVolumeThresholdBreached)
	if err != nil {
		r.cleanup(ctx)
		r.state.set(StateStopped)
		return fmt.Errorf("failed to subscribe to VolumeThresholdBreached eventbus: %w", err)
	}
	r.subscriptionID = subscriptionID
	Logc(ctx).WithField("subscriptionID", subscriptionID).
		Info("Subscribed to VolumeThresholdBreached eventbus")

	// Start worker loop that submits tasks to the pool
	go r.workerLoop(ctx)

	// Transition to Running state
	r.state.set(StateRunning)

	Logc(ctx).WithField("config", r.config).Info("Requester layer activated successfully")
	return nil
}

// Deactivate gracefully stops the Requester layer (idempotent)
// Drains in-flight jobs before returning
func (r *Requester) Deactivate(ctx context.Context) error {
	Logc(ctx).Debug(">>>> requester.Deactivate")
	defer Logc(ctx).Debug("<<<< requester.Deactivate")

	// Check if we can transition to Stopping state
	if !r.state.compareAndSwap(StateRunning, StateStopping) {
		currentState := r.state.get()
		if currentState == StateStopped {
			Logc(ctx).Debug("Requester already stopped, skipping deactivation")
			return nil // Idempotent
		}
		// Return StateError with state and message - caller will decide what to do
		return errors.NewStateError(currentState.String(), fmt.Sprintf("cannot deactivate requester: current state is %s", currentState.String()))
	}

	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	Logc(ctx).Info("Deactivating Requester layer...")

	// Cleanup (handles eventbus unsubscribe, workqueue shutdown, worker pool shutdown, stop channel close, and state reset)
	r.cleanup(ctx)

	Logc(ctx).WithField("config", r.config).Info("Requester layer deactivated successfully")
	return nil
}

// cleanup performs final cleanup operations
func (r *Requester) cleanup(ctx context.Context) {
	// Unsubscribe from eventbus
	if r.subscriptionID != 0 {
		if eventbus.VolumeThresholdBreachedEventBus != nil {
			if unsubscribed := eventbus.VolumeThresholdBreachedEventBus.Unsubscribe(r.subscriptionID); !unsubscribed {
				Logc(ctx).WithField("subscriptionID", r.subscriptionID).
					Warn("Failed to unsubscribe from VolumeThresholdBreached eventbus")
			} else {
				Logc(ctx).WithField("subscriptionID", r.subscriptionID).
					Info("Unsubscribed from VolumeThresholdBreached eventbus successfully")
			}
		}
		r.subscriptionID = 0
	}

	// Shutdown work queue (no new items will be added)
	// Note: Don't set to nil here - the workerLoop goroutine may still be accessing it
	// It will be overwritten on next Activate() anyway
	if r.workqueue != nil {
		r.workqueue.ShutDown()
	}

	// Close stop channel if it exists
	// Note: Don't set to nil here - the workerLoop goroutine may still be reading from it
	// It will be overwritten on next Activate() anyway
	if r.stopCh != nil {
		close(r.stopCh)
	}

	// Gracefully shutdown worker pool with timeout (only if we own it)
	if r.ownWorkerPool && r.workerPool != nil {
		if err := r.workerPool.ShutdownWithTimeout(r.config.ShutdownTimeout); err != nil {
			Logc(ctx).WithError(err).Warn("Worker pool shutdown timeout exceeded")
		} else {
			Logc(ctx).Info("Worker pool shutdown successfully")
		}
	} else if !r.ownWorkerPool {
		Logc(ctx).Debug("Worker pool not owned by requester, skipping shutdown")
	}

	// Clear latest events map
	r.latestEventsMu.Lock()
	r.latestEvents = make(map[string]agTypes.VolumeThresholdBreached)
	r.latestEventsMu.Unlock()

	// Reset state to Stopped
	r.state.set(StateStopped)
}

// workerLoop continuously pulls work items from the queue and submits them to the worker pool
func (r *Requester) workerLoop(ctx context.Context) {
	Logc(ctx).Debug(">>>> requester.workerloop started")
	defer Logc(ctx).Debug("<<<< requester.workerloop stopped")

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		default:
			// Get work item from queue
			workItem, shutdown := r.workqueue.Get()
			if shutdown {
				return
			}

			// Check if we can still accept work (allows graceful shutdown)
			if !r.IsRunning() {
				// During shutdown, drain the queue without processing
				r.workqueue.Done(workItem)
				continue
			}

			// Submit work to pool
			// Note: We must call Done() before re-queuing to release the "in-flight" marker
			if err := r.workerPool.Submit(ctx, func() {
				// Process the work item
				if err := r.processWorkItem(workItem.Ctx, workItem); err != nil {
					// Check if this is a retriable error (wrapped in ReconcileDeferredError)
					if errors.IsReconcileDeferredError(err) {
						// Check if we've exceeded max retries
						if workItem.RetryCount >= r.config.MaxRetries {
							Logc(workItem.Ctx).WithError(err).WithFields(LogFields{
								"tvpName":    workItem.TVPName,
								"retryCount": workItem.RetryCount,
								"maxRetries": r.config.MaxRetries,
							}).Error("Max retries exceeded, discarding work item")
							r.cleanupEvent(workItem.TVPName)
							r.workqueue.Forget(workItem)
							r.workqueue.Done(workItem)
						} else {
							// Transient failure - re-queue with exponential backoff
							// Keep event in latestEvents map for retry
							Logc(workItem.Ctx).WithError(err).WithFields(LogFields{
								"tvpName":    workItem.TVPName,
								"retryCount": workItem.RetryCount,
							}).Warn("Retriable error, re-queuing with backoff")

							// Increment retry count for next attempt
							workItem.RetryCount++
							r.workqueue.Done(workItem)

							// Check if error contains rate limit retry-after hint
							if k8serrors.IsTooManyRequests(err) {
								// Unwrap to access the StatusError details
								var statusErr *k8serrors.StatusError
								if stderrors.As(err, &statusErr) && statusErr.Status().Details != nil && statusErr.Status().Details.RetryAfterSeconds > 0 {
									duration := time.Duration(statusErr.Status().Details.RetryAfterSeconds) * time.Second
									Logc(workItem.Ctx).WithFields(LogFields{
										"tvpName":    workItem.TVPName,
										"retryAfter": duration,
									}).Warn("Re-queuing with API server's suggested delay")
									r.workqueue.AddAfter(workItem, duration)
									return
								}
							}

							// Fallback to exponential backoff for other retriable errors
							r.workqueue.AddRateLimited(workItem)
						}
					} else {
						// Non-retriable error (programming error, unexpected condition)
						// Log as error but don't retry - clean up the event
						Logc(workItem.Ctx).WithError(err).WithFields(LogFields{
							"tvpName":    workItem.TVPName,
							"retryCount": workItem.RetryCount,
						}).Error("Non-retriable error occurred, not re-queuing")
						r.cleanupEvent(workItem.TVPName)
						r.workqueue.Forget(workItem)
						r.workqueue.Done(workItem)
					}
				} else {
					// Processing succeeded
					// Clean up event and remove from queue
					r.cleanupEvent(workItem.TVPName)
					r.workqueue.Forget(workItem)
					r.workqueue.Done(workItem)
				}
			}); err != nil {
				// Failed to submit to pool - mark as done and re-queue with backoff
				// Keep event in latestEvents map for retry
				Logc(workItem.Ctx).WithError(err).WithField("tvpName", workItem.TVPName).
					Warn("Failed to submit work to pool, re-queuing with backoff")
				workItem.RetryCount++
				r.workqueue.Done(workItem)
				r.workqueue.AddRateLimited(workItem)
			}
		}
	}
}

// processWorkItem validates and creates a TAGRI CR
// Returns:
//   - nil: Processing succeeded (event will be cleaned up)
//   - ReconcileDeferredError: Transient failure, should retry with backoff (event kept for retry)
//   - other error: Non-retriable error, log and skip (event will be cleaned up)
//
// Note: Event cleanup is handled by workerLoop based on return value
// Note: workItem contains the TVP name and context from the VolumeThresholdBreached event
func (r *Requester) processWorkItem(ctx context.Context, workItem WorkItem) (err error) {
	tvpName := workItem.TVPName

	Logc(ctx).WithField("tvpName", tvpName).Debug(">>>> requester.processWorkItem")
	defer Logc(ctx).WithField("tvpName", tvpName).Debug("<<<< requester.processWorkItem")

	// Step 1: Retrieve the latest event for this TVP (indexed by tvpName)
	r.latestEventsMu.RLock()
	event, exists := r.latestEvents[tvpName]
	r.latestEventsMu.RUnlock()

	if !exists {
		Logc(ctx).WithField("tvpName", tvpName).Warn("No event found for TVP, skipping")
		return fmt.Errorf("no event found for TVP %q, skipping", tvpName)
	}

	// Step 2: Get TVP from lister to extract volumeID
	// Do this early to fail fast if TVP has been deleted
	tvp, err := r.tvpLister.TridentVolumePublications(r.config.TridentNamespace).Get(tvpName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			Logc(ctx).WithField("tvpName", tvpName).
				Debug("TVP not found in lister, may have been deleted - skipping TAGRI creation")
			return nil // TVP deleted, don't retry
		}
		Logc(ctx).WithFields(LogFields{
			"tvpName": tvpName,
		}).WithError(err).Warn("Failed to get TVP from lister")
		return errors.WrapWithReconcileDeferredError(err, "failed to get TVP from lister")
	}

	volumeID := tvp.VolumeID
	if volumeID == "" {
		Logc(ctx).WithField("tvpName", tvpName).
			Warn("TVP has empty VolumeID")
		return nil // Invalid TVP, don't retry
	}

	// Step 3: Get effective policy name from event, fallback to cache if missing
	policyName := event.TAGPName
	if policyName == "" {
		Logc(ctx).WithFields(LogFields{
			"tvpName": tvpName,
		}).Debug("Event missing policy name, fetching from cache")

		policyName, err = r.autogrowCache.GetEffectivePolicyName(tvpName)
		if err != nil {
			Logc(ctx).WithField("tvpName", tvpName).
				WithError(err).
				Warn("No effective policy found for TVP, skipping")
			return fmt.Errorf("no effective policy found for TVP %q, skipping", tvpName) // Don't retry - TVP has no policy
		}
	}

	// Step 4: Fetch the policy using the name from cache
	policy, err := r.agpLister.Get(autogrow.NameFix(policyName))
	if err != nil {
		if k8serrors.IsNotFound(err) {
			Logc(ctx).WithFields(LogFields{
				"policyName": policyName,
				"tvpName":    tvpName,
			}).Warn("Policy no longer exists, skipping request")
			return fmt.Errorf("policy %q no longer exists attached to TVP %q, skipping", policyName, tvpName)
		}
		// Transient API error - wrap in ReconcileDeferredError to trigger retry
		return errors.WrapWithReconcileDeferredError(err, "failed to get policy from lister")
	}

	// Step 5: Checking policy isn't in failed state
	if policy.Status.State == string(v1.TridentAutogrowPolicyStateFailed) || policy.Status.State == string(v1.TridentAutogrowPolicyStateDeleting) {
		Logc(ctx).WithFields(LogFields{
			"policyName":  policyName,
			"tvpName":     tvpName,
			"policyPhase": policy.Status.State,
		}).Warn("Policy is either in deleting or failed state, skipping request")
		return nil
	}

	// Step 6: Calculate current usage percentage
	var currentUsedPercent float32
	if event.CurrentTotalSize > 0 {
		if event.UsedPercentage > 0 {
			currentUsedPercent = event.UsedPercentage
		} else {
			// UsedPercentage is not set, calculate it from UsedSize and CurrentTotalSize
			currentUsedPercent = (float32(event.UsedSize) / float32(event.CurrentTotalSize)) * 100
		}
	} else {
		Logc(ctx).WithFields(LogFields{
			"tvpName":  tvpName,
			"volumeID": volumeID,
		}).Warn("Volume has zero capacity, cannot calculate usage percentage, skipping")
		return fmt.Errorf("volume %q has zero capacity, cannot validate threshold", volumeID)
	}

	// Step 7: Check if TAGRI already exists
	_, err = r.getTAGRI(ctx, volumeID)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			// API error (not NotFound) - return it (already wrapped as ReconcileDeferredError)
			return err
		}
		// NotFound is expected - proceed to create TAGRI
	} else {
		// TAGRI already exists - return non-retriable error
		return fmt.Errorf("TAGRI already exists for volumeID %q (TVP %q)", volumeID, tvpName)
	}

	// Step 8: Create new TAGRI CR with the validated usage percentage
	if err = r.createTAGRI(ctx, event, policy, volumeID, currentUsedPercent); err != nil {
		return err
	}

	Logc(ctx).WithFields(LogFields{
		"tvpName":     tvpName,
		"volumeID":    volumeID,
		"currentSize": event.CurrentTotalSize,
		"policyName":  policyName,
	}).Info("TridentAutogrowRequestInternal created successfully")

	return nil
}

// IsRunning returns true if the layer is currently running
func (r *Requester) IsRunning() bool {
	return r.state.get() == StateRunning
}

// handleVolumeThresholdBreached is the eventbus subscription handler
// It receives VolumeThresholdBreached events and enqueues them to the workqueue
func (r *Requester) handleVolumeThresholdBreached(
	ctx context.Context,
	event agTypes.VolumeThresholdBreached,
) (eventbusTypes.Result, error) {
	// Use context from event if provided, otherwise use the handler context
	// The handler context, could have a timeout embedded into it.
	// But, as we are not relying on any timeout here in this function, we can safely ignore it.
	eventCtx := event.Ctx
	if eventCtx == nil {
		eventCtx = ctx
	}

	// Enqueue the event to the workqueue
	if err := r.EnqueueEvent(eventCtx, event); err != nil {
		Logc(eventCtx).WithError(err).
			WithField("event", event).
			Warn("Failed to enqueue event from eventbus")
		return nil, err
	}

	Logc(eventCtx).WithField("event", event).Debug("Successfully enqueued event from eventbus")

	// Return success result for eventbus metrics
	return eventbusTypes.Result{
		"tvpName":  event.TVPName,
		"enqueued": true,
	}, nil
}

// EnqueueEvent adds a threshold breach event to the work queue
func (r *Requester) EnqueueEvent(ctx context.Context, event agTypes.VolumeThresholdBreached) error {
	Logc(ctx).WithField("event", event).Debug(">>>> requester.EnqueueEvent")
	defer Logc(ctx).WithField("event", event).Debug("<<<< requester.EnqueueEvent")

	// Check if we can accept work
	if !r.IsRunning() {
		return fmt.Errorf("requester not running, rejecting event")
	}

	// Basic validation
	if event.TVPName == "" || event.UsedSize == 0 {
		Logc(ctx).WithField("event", event).Warn("Invalid event: either missing TVP name or used size")
		return fmt.Errorf("invalid event: either missing TVP name or used size")
	}

	// Store the latest event for this TVP
	// This ensures we process the most recent event when multiple events arrive
	r.latestEventsMu.Lock()
	r.latestEvents[event.TVPName] = event
	r.latestEventsMu.Unlock()

	// Add work item to queue with context and RetryCount = 0
	// Workqueue's built-in deduplication ensures same TVPName won't be queued twice
	r.workqueue.Add(WorkItem{
		Ctx:        ctx,
		TVPName:    event.TVPName,
		RetryCount: 0,
	})

	Logc(ctx).WithField("event", event).Debug("Event enqueued for processing")

	return nil
}

// cleanupEvent removes an event from the latestEvents map
func (r *Requester) cleanupEvent(tvpName string) {
	r.latestEventsMu.Lock()
	defer r.latestEventsMu.Unlock()

	delete(r.latestEvents, tvpName)
}

// GetState returns the current requester state.
// This method is primarily used for testing to verify state transitions.
func (r *Requester) GetState() State {
	return r.state.get()
}

// SetState sets the requester state directly.
// This method is primarily used for testing to simulate specific state conditions.
func (r *Requester) SetState(state State) {
	r.state.set(state)
}
