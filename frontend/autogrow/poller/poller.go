package poller

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	stderrors "errors"
	"fmt"
	"time"

	"golang.org/x/time/rate"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/workqueue"

	agCache "github.com/netapp/trident/frontend/autogrow/cache"
	agTypes "github.com/netapp/trident/frontend/autogrow/types"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	agInternals "github.com/netapp/trident/internal/autogrow"
	. "github.com/netapp/trident/logging"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	listerv1 "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	"github.com/netapp/trident/pkg/eventbus"
	eventbusTypes "github.com/netapp/trident/pkg/eventbus/types"
	"github.com/netapp/trident/pkg/workerpool"
	"github.com/netapp/trident/pkg/workerpool/ants"
	poolTypes "github.com/netapp/trident/pkg/workerpool/types"
	agUtils "github.com/netapp/trident/utils/autogrow"
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

	if config.MaxRetries <= 0 {
		config.MaxRetries = DefaultMaxRetries
		Logc(ctx).WithField("retries", config.MaxRetries).Debug("Using default Max Retries")
	}

	// Note: WorkerPool is optional (nil is valid, we'll create one with defaults)
}

// NewPoller creates a new Poller instance
func NewPoller(
	ctx context.Context,
	agpLister listerv1.TridentAutogrowPolicyLister,
	tvpLister listerv1.TridentVolumePublicationLister,
	autogrowCache *agCache.AutogrowCache,
	volumeStatsProvider nodehelpers.VolumeStatsManager,
	config *Config,
) (*Poller, error) {
	Logc(ctx).Debug(">>>> NewPoller")
	defer Logc(ctx).Debug("<<<< NewPoller")

	// Create a deep copy of the config to avoid mutating the caller's config
	var configCopy *Config
	if config == nil {
		configCopy = DefaultConfig()
	} else {
		configCopy = config.Copy()
	}

	// Validate config copy and apply defaults for any missing values
	applyConfigDefaults(ctx, configCopy)

	Logc(ctx).WithField("config", configCopy).Debug("Poller config has been initialized")

	// Validate volumeStatsProvider
	if volumeStatsProvider == nil {
		return nil, errors.NotFoundError("volumeStatsProvider is required")
	}

	// Validate tvpLister
	if tvpLister == nil {
		return nil, errors.NotFoundError("tvpLister is required")
	}

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

	p := &Poller{
		agpLister:           agpLister,
		tvpLister:           tvpLister,
		autogrowCache:       autogrowCache,
		volumeStatsProvider: volumeStatsProvider,
		config:              configCopy,
		workerPool:          pool,
		ownWorkerPool:       ownPool,
	}

	return p, nil
}

// Activate starts the Poller layer (idempotent)
// Returns error if already running or if activation fails
func (p *Poller) Activate(ctx context.Context) error {
	Logc(ctx).Debug(">>>> poller.Activate")
	defer Logc(ctx).Debug("<<<< poller.Activate")

	// Check if we can transition to Starting state
	if !p.state.compareAndSwap(StateStopped, StateStarting) {
		currentState := p.state.get()
		if currentState == StateRunning {
			Logc(ctx).Debug("Poller already running, skipping activation")
			return nil // Idempotent
		}
		return errors.NewStateError(currentState.String(), fmt.Sprintf("cannot activate poller: current state is %s", currentState.String()))
	}

	p.stateMu.Lock()
	defer p.stateMu.Unlock()

	Logc(ctx).Info("Activating Poller layer...")

	// Create stop channel
	p.stopCh = make(chan struct{})

	// Create rate limiter combining exponential backoff with bucket rate limiting
	rateLimiter := workqueue.NewTypedMaxOfRateLimiter(
		workqueue.NewTypedItemExponentialFailureRateLimiter[WorkItem](WorkqueueBaseDelay, WorkqueueMaxDelay),
		// QPS and bucket size for retry speed and overall throughput control (not per item)
		&workqueue.TypedBucketRateLimiter[WorkItem]{Limiter: rate.NewLimiter(rate.Limit(WorkqueueBucketQPS), WorkqueueBucketBurst)},
	)

	// Create workqueue with custom rate limiting configuration
	p.workqueue = workqueue.NewTypedRateLimitingQueueWithConfig(
		rateLimiter,
		workqueue.TypedRateLimitingQueueConfig[WorkItem]{
			Name: p.config.WorkQueueName,
		},
	)

	Logc(ctx).WithFields(LogFields{
		"workQueueName": p.config.WorkQueueName,
		"baseDelay":     WorkqueueBaseDelay,
		"maxDelay":      WorkqueueMaxDelay,
		"qps":           WorkqueueBucketQPS,
		"burst":         WorkqueueBucketBurst,
	}).Debug("Created workqueue with exponential backoff and bucket rate limiting")

	// Create worker pool if we own it
	if p.ownWorkerPool {
		Logc(ctx).Info("Creating worker pool for poller")

		// Calculate expiry duration if AutogrowPeriod is set
		// This prevents goroutines from being destroyed between polling cycles
		var expiryDuration time.Duration
		if p.config.AutogrowPeriod != nil && *p.config.AutogrowPeriod > 0 {
			// Set expiry to Autogrow period + 30 seconds buffer
			expiryDuration = *p.config.AutogrowPeriod + (30 * time.Second)
			Logc(ctx).WithField("expiryDuration", expiryDuration).
				Debug("Configuring worker pool with expiry duration based on Autogrow period")
		}

		// Build log fields
		logFields := LogFields{
			"size":        DefaultWorkerPoolSize,
			"preAlloc":    DefaultWorkerPoolPreAlloc,
			"nonBlocking": DefaultWorkerPoolNonBlocking,
		}
		if expiryDuration > 0 {
			logFields["expiryDuration"] = expiryDuration
		}

		// Create worker pool with appropriate configuration
		var pool poolTypes.Pool
		var err error
		if expiryDuration > 0 {
			pool, err = workerpool.New[poolTypes.Pool](
				ctx,
				ants.NewConfig(
					ants.WithNumWorkers(DefaultWorkerPoolSize),
					ants.WithPreAlloc(DefaultWorkerPoolPreAlloc),
					ants.WithNonBlocking(DefaultWorkerPoolNonBlocking),
					ants.WithExpiryDuration(expiryDuration),
				),
			)
		} else {
			pool, err = workerpool.New[poolTypes.Pool](
				ctx,
				ants.NewConfig(
					ants.WithNumWorkers(DefaultWorkerPoolSize),
					ants.WithPreAlloc(DefaultWorkerPoolPreAlloc),
					ants.WithNonBlocking(DefaultWorkerPoolNonBlocking),
				),
			)
		}
		if err != nil {
			p.cleanup(ctx)
			p.state.set(StateStopped)
			return fmt.Errorf("failed to create worker pool: %w", err)
		}
		p.workerPool = pool
		Logc(ctx).WithFields(logFields).Info("Created worker pool for poller")

		// Start the worker pool (only if we own it)
		if err := p.workerPool.Start(ctx); err != nil {
			p.cleanup(ctx)
			p.state.set(StateStopped)
			return fmt.Errorf("failed to start worker pool: %w", err)
		}
		Logc(ctx).Info("Worker pool started")
	}

	// Subscribe to VolumesScheduled events from the eventbus
	if eventbus.VolumesScheduledEventBus == nil {
		p.cleanup(ctx)
		p.state.set(StateStopped)
		return fmt.Errorf("VolumesScheduled eventbus not initialized")
	}
	subscriptionID, err := eventbus.VolumesScheduledEventBus.Subscribe(p.handleVolumesScheduled)
	if err != nil {
		p.cleanup(ctx)
		p.state.set(StateStopped)
		return fmt.Errorf("failed to subscribe to VolumesScheduled eventbus: %w", err)
	}
	p.subscriptionID = subscriptionID
	Logc(ctx).WithField("subscriptionID", subscriptionID).
		Info("Subscribed to VolumesScheduled eventbus")

	// Start worker loop that submits tasks to the pool
	go p.workerLoop(ctx)

	// Transition to Running state
	p.state.set(StateRunning)

	Logc(ctx).WithField("config", p.config).Info("Poller layer activated successfully")
	return nil
}

// Deactivate gracefully stops the Poller layer (idempotent)
// Drains in-flight jobs before returning
func (p *Poller) Deactivate(ctx context.Context) error {
	Logc(ctx).Debug(">>>> poller.Deactivate")
	defer Logc(ctx).Debug("<<<< poller.Deactivate")

	// Check if we can transition to Stopping state
	if !p.state.compareAndSwap(StateRunning, StateStopping) {
		currentState := p.state.get()
		if currentState == StateStopped {
			Logc(ctx).Debug("Poller already stopped, skipping deactivation")
			return nil // Idempotent
		}
		// Return StateError with state and message - caller will decide what to do
		return errors.NewStateError(currentState.String(), fmt.Sprintf("cannot deactivate poller: current state is %s", currentState.String()))
	}

	p.stateMu.Lock()
	defer p.stateMu.Unlock()

	Logc(ctx).Info("Deactivating Poller layer...")

	// Cleanup (handles eventbus unsubscribe, workqueue shutdown, worker pool shutdown, stop channel close, and state reset)
	p.cleanup(ctx)

	Logc(ctx).WithField("config", p.config).Info("Poller layer deactivated successfully")
	return nil
}

// cleanup performs final cleanup operations
func (p *Poller) cleanup(ctx context.Context) {
	// Unsubscribe from eventbus
	if p.subscriptionID != 0 {
		if eventbus.VolumesScheduledEventBus != nil {
			if unsubscribed := eventbus.VolumesScheduledEventBus.Unsubscribe(p.subscriptionID); !unsubscribed {
				Logc(ctx).WithField("subscriptionID", p.subscriptionID).
					Warn("Failed to unsubscribe from VolumesScheduled eventbus")
			} else {
				Logc(ctx).WithField("subscriptionID", p.subscriptionID).
					Info("Unsubscribed from VolumesScheduled eventbus successfully")
			}
		}
		p.subscriptionID = 0
	}

	// Shutdown work queue (no new items will be added)
	if p.workqueue != nil {
		p.workqueue.ShutDown()
	}

	// Close stop channel if it exists
	if p.stopCh != nil {
		close(p.stopCh)
	}

	// Gracefully shutdown worker pool with timeout (only if we own it)
	if p.ownWorkerPool && p.workerPool != nil {
		if err := p.workerPool.ShutdownWithTimeout(p.config.ShutdownTimeout); err != nil {
			Logc(ctx).WithError(err).Warn("Worker pool shutdown timeout exceeded")
		} else {
			Logc(ctx).Info("Worker pool shutdown successfully")
		}
	} else if !p.ownWorkerPool {
		Logc(ctx).Debug("Worker pool not owned by poller, skipping shutdown")
	}

	// Reset state to Stopped
	p.state.set(StateStopped)
}

// workerLoop continuously pulls work items from the queue and submits them to the worker pool
func (p *Poller) workerLoop(ctx context.Context) {
	Logc(ctx).Debug(">>>> poller.workerLoop started")
	defer Logc(ctx).Debug("<<<< poller.workerLoop stopped")

	for {
		select {
		case <-ctx.Done():
			return
		case <-p.stopCh:
			return
		default:
			// Get work item from queue
			workItem, shutdown := p.workqueue.Get()
			if shutdown {
				return
			}

			// Check if we can still accept work (allows graceful shutdown)
			if !p.IsRunning() {
				// During shutdown, drain the queue without processing
				p.workqueue.Done(workItem)
				continue
			}

			// Submit work to pool
			// Note: We must call Done() before re-queuing to release the "in-flight" marker
			if err := p.workerPool.Submit(ctx, func() {
				// Process the work item
				if err := p.processWorkItem(workItem.Ctx, workItem); err != nil {
					// Check if this is a retriable error (wrapped in ReconcileDeferredError)
					if errors.IsReconcileDeferredError(err) {
						// Check if we've exceeded max retries
						if workItem.RetryCount >= p.config.MaxRetries {
							Logc(workItem.Ctx).WithError(err).WithFields(LogFields{
								"tvpName":    workItem.TVPName,
								"retryCount": workItem.RetryCount,
								"maxRetries": p.config.MaxRetries,
							}).Error("Max retries exceeded, discarding work item")
							p.workqueue.Forget(workItem)
							p.workqueue.Done(workItem)
						} else {
							// Transient failure - re-queue with exponential backoff
							Logc(workItem.Ctx).WithError(err).WithFields(LogFields{
								"tvpName":    workItem.TVPName,
								"retryCount": workItem.RetryCount,
							}).Warn("Retriable error, re-queuing with backoff")

							// Increment retry count for next attempt
							workItem.RetryCount++
							p.workqueue.Done(workItem)

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
									p.workqueue.AddAfter(workItem, duration)
									return
								}
							}

							// Fallback to exponential backoff for other retriable errors
							p.workqueue.AddRateLimited(workItem)
						}
					} else {
						// Non-retriable error (programming error, unexpected condition)
						// Log as error but don't retry
						Logc(workItem.Ctx).WithError(err).WithFields(LogFields{
							"tvpName":    workItem.TVPName,
							"retryCount": workItem.RetryCount,
						}).Error("Non-retriable error occurred, not re-queuing")
						p.workqueue.Forget(workItem)
						p.workqueue.Done(workItem)
					}
				} else {
					// Processing succeeded or permanently failed (returned nil)
					// Clean up and remove from queue
					p.workqueue.Forget(workItem)
					p.workqueue.Done(workItem)
				}
			}); err != nil {
				// Failed to submit to pool - mark as done and re-queue with backoff
				Logc(workItem.Ctx).WithError(err).WithField("tvpName", workItem.TVPName).
					Warn("Failed to submit work to pool, re-queuing with backoff")
				workItem.RetryCount++
				p.workqueue.Done(workItem)
				p.workqueue.AddRateLimited(workItem)
			}
		}
	}
}

// IsRunning returns true if the layer is currently running
func (p *Poller) IsRunning() bool {
	return p.state.get() == StateRunning
}

// handleVolumesScheduled is the eventbus subscription handler
// It receives VolumesScheduled events and enqueues the event to the workqueue
func (p *Poller) handleVolumesScheduled(
	ctx context.Context,
	event agTypes.VolumesScheduled,
) (eventbusTypes.Result, error) {
	// Use context from event if provided, otherwise use the handler context
	// The handler context, could have a timeout embedded into it.
	// But, as we are not relying on any timeout here in this function, we can safely ignore it.
	eventCtx := event.Ctx
	if eventCtx == nil {
		eventCtx = ctx
	}

	Logc(eventCtx).WithField("event", event).Debug("Received VolumesScheduled event")

	// Enqueue volumes to the work queue
	enqueued, skipped, err := p.EnqueueEvent(eventCtx, event.TVPNames)
	if err != nil {
		Logc(eventCtx).WithError(err).
			WithField("event", event).
			Warn("Failed to enqueue event from eventbus")
		return nil, err
	}

	Logc(eventCtx).WithFields(LogFields{
		"eventID":  event.ID,
		"total":    len(event.TVPNames),
		"enqueued": enqueued,
		"skipped":  skipped,
	}).Info("Successfully enqueued events from VolumesScheduled event")

	// Return success result for eventbus metrics
	return eventbusTypes.Result{
		"eventID":  event.ID,
		"enqueued": enqueued,
		"skipped":  skipped,
	}, nil
}

// EnqueueEvent adds multiple TVPs to the work queue for polling
// Returns: (enqueued count, skipped count, error)
func (p *Poller) EnqueueEvent(ctx context.Context, tvpNames []string) (int, int, error) {
	Logc(ctx).WithField("tvpCount", len(tvpNames)).Debug(">>>> poller.EnqueueEvent")
	defer Logc(ctx).WithField("tvpCount", len(tvpNames)).Debug("<<<< poller.EnqueueEvent")

	// Check if we can accept work
	if !p.IsRunning() {
		return 0, 0, fmt.Errorf("poller not running, rejecting TVPs")
	}

	// Enqueue all TVPs from the scheduled event
	enqueued := 0
	skipped := 0
	for _, tvpName := range tvpNames {
		if tvpName == "" {
			skipped++
			continue
		}

		// Check if TVP has an effective policy in cache
		if !p.autogrowCache.HasEffectivePolicyName(tvpName) {
			Logc(ctx).WithField("tvpName", tvpName).
				Debug("TVP has no effective policy, skipping")
			skipped++
			continue
		}

		// Add work item to queue with context and RetryCount = 0
		// Workqueue's built-in deduplication ensures same TVPName won't be queued twice
		p.workqueue.Add(WorkItem{
			Ctx:        ctx,
			TVPName:    tvpName,
			RetryCount: 0,
		})
		enqueued++
	}

	Logc(ctx).WithFields(LogFields{
		"total":    len(tvpNames),
		"enqueued": enqueued,
		"skipped":  skipped,
	}).Debug("TVPs enqueued for polling")

	return enqueued, skipped, nil
}

// processWorkItem polls a volume and publishes VolumeThresholdBreached event if threshold is crossed
// Returns:
//   - nil: Processing succeeded or permanently failed (no retry needed)
//   - ReconcileDeferredError: Transient failure, should retry with backoff
//   - other error: Non-retriable error, log and skip
//
// Note: workItem contains the TVP name and context from the VolumesScheduled event
func (p *Poller) processWorkItem(ctx context.Context, workItem WorkItem) error {
	tvpName := workItem.TVPName

	Logc(ctx).WithField("tvpName", tvpName).Debug(">>>> poller.processWorkItem")
	defer Logc(ctx).WithField("tvpName", tvpName).Debug("<<<< poller.processWorkItem")

	// Step 1: Check if tvpName still has an effective policy (cache uses tvpName)
	policyName, err := p.autogrowCache.GetEffectivePolicyName(tvpName)
	if err != nil {
		Logc(ctx).WithField("tvpName", tvpName).
			WithError(err).
			Debug("No effective policy found for volume, skipping polling")
		return nil // Don't retry - TVP has no policy
	}

	Logc(ctx).WithFields(LogFields{
		"tvpName":    tvpName,
		"policyName": policyName,
	}).Debug("Volume has effective policy, proceeding with polling")

	// Step 2: Get TVP from lister to extract volumeID
	// We need volumeID to query the storage backend
	tvp, err := p.tvpLister.TridentVolumePublications(p.config.TridentNamespace).Get(tvpName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			Logc(ctx).WithField("tvpName", tvpName).
				Debug("TVP not found in lister, may have been deleted")
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

	Logc(ctx).WithFields(LogFields{
		"tvpName":  tvpName,
		"volumeID": volumeID,
	}).Debug("Extracted volumeID from TVP")

	// Step 3: Get the policy from the lister
	policy, err := p.agpLister.Get(agUtils.NameFix(policyName))
	if err != nil {
		if k8serrors.IsNotFound(err) {
			Logc(ctx).WithFields(LogFields{
				"policyName": policyName,
				"tvpName":    tvpName,
			}).Warn("Policy no longer exists, skipping request")
			return fmt.Errorf("policy %q no longer exists that was attached to volume %q, skipping", policyName, tvpName)
		}
		// Transient API error - wrap in ReconcileDeferredError to trigger retry
		return errors.WrapWithReconcileDeferredError(err, "failed to get policy from lister")
	}

	// Step 4: Checking policy isn't in either failed state or deleting state
	if policy.Status.State == string(v1.TridentAutogrowPolicyStateFailed) || policy.Status.State == string(v1.TridentAutogrowPolicyStateDeleting) {
		Logc(ctx).WithFields(LogFields{
			"policyName":  policyName,
			"tvpName":     tvpName,
			"policyPhase": policy.Status.State,
		}).Warn("Policy is either in deleting or failed state, skipping request")
		return nil
	}

	// Step 5: Get volume used size (in bytes and percentage) using volumeID
	// This function queries the storage backend and returns total size, used bytes, and used percentage
	currentTotalSizeBytes, usedSizeBytes, usedSizePercent, err := p.getVolumeUsedSize(ctx, volumeID)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"tvpName":  tvpName,
			"volumeID": volumeID,
		}).WithError(err).Warn("Failed to get volume used size")
		// Storage backend error - wrap in ReconcileDeferredError to trigger retry
		return errors.WrapWithReconcileDeferredError(err, "failed to get volume used size")
	}

	Logc(ctx).WithFields(LogFields{
		"policyName":            policyName,
		"tvpName":               tvpName,
		"volumeID":              volumeID,
		"currentTotalSizeBytes": currentTotalSizeBytes,
		"usedSizeBytes":         usedSizeBytes,
		"usedSizePercent":       usedSizePercent,
	}).Debug("Retrieved volume metrics")

	// Get the normalized values
	autogrowSpec, err := agInternals.ValidateAutogrowPolicySpec(policy.Spec.UsedThreshold, policy.Spec.GrowthAmount, policy.Spec.MaxSize)
	if err != nil {
		Logc(ctx).WithFields(LogFields{
			"tvpName":    tvpName,
			"policyName": policyName,
		}).WithError(err).Warn("Failed to extract Autogrow policy spec")
		return nil
	}

	// Step 6: Check if threshold is breached
	breached := isThresholdBreached(autogrowSpec, usedSizePercent)

	if !breached {
		Logc(ctx).WithFields(LogFields{
			"policyName":       policyName,
			"tvpName":          tvpName,
			"usedSizeBytes":    usedSizeBytes,
			"usedSizePercent":  usedSizePercent,
			"thresholdPercent": autogrowSpec.UsedThresholdPercent,
		}).Debug("Volume usage below threshold, no action needed")
		return nil // Success - no breach
	}

	// Step 7: Threshold breached - publish event
	Logc(ctx).WithFields(LogFields{
		"policyName":            policyName,
		"tvpName":               tvpName,
		"volumeID":              volumeID,
		"currentTotalSizeBytes": currentTotalSizeBytes,
		"usedSizeBytes":         usedSizeBytes,
		"usedSizePercent":       usedSizePercent,
		"thresholdPercent":      autogrowSpec.UsedThresholdPercent,
	}).Info("Volume threshold breached, publishing event")

	timestamp := time.Now()

	// Generate unique event ID: hash(timestamp + tvpName)
	eventID := generateEventID(tvpName, timestamp)

	// Create VolumeThresholdBreached event
	event := agTypes.VolumeThresholdBreached{
		Ctx:              ctx,
		ID:               eventID,
		TVPName:          tvpName,
		TAGPName:         policyName,
		CurrentTotalSize: currentTotalSizeBytes, // Current total capacity of the volume
		UsedSize:         usedSizeBytes,         // Actual used bytes
		UsedPercentage:   usedSizePercent,
		Timestamp:        timestamp,
	}

	// Publish to VolumeThresholdBreached eventbus
	if eventbus.VolumeThresholdBreachedEventBus == nil {
		Logc(ctx).Error("VolumeThresholdBreached eventbus not initialized - this is a programming error")
		// This is a programming/configuration error, not a transient failure
		// Don't retry - the eventbus should be initialized before poller activates
		return fmt.Errorf("VolumeThresholdBreached eventbus not initialized")
	}

	eventbus.VolumeThresholdBreachedEventBus.Publish(ctx, event)

	Logc(ctx).WithFields(LogFields{
		"tvpName": tvpName,
		"eventID": eventID,
		"event":   event,
	}).Info("Successfully published VolumeThresholdBreached event")

	return nil
}

// getVolumeUsedSize retrieves the used size and total size of a volume
func (p *Poller) getVolumeUsedSize(ctx context.Context, volumeID string) (
	currentTotalSizeBytes int64,
	usedSizeBytes int64,
	usedSizePercent float32,
	err error,
) {
	volumeStats, err := p.volumeStatsProvider.GetVolumeStatsByID(ctx, volumeID)
	if err != nil {
		return 0, 0, 0.0, fmt.Errorf("failed to get volume stats: %w", err)
	}

	currentTotalSizeBytes = volumeStats.Total
	usedSizeBytes = volumeStats.Used
	usedSizePercent = volumeStats.UsedPercentage
	return currentTotalSizeBytes, usedSizeBytes, usedSizePercent, nil
}

// isThresholdBreached checks if the volume usage percentage has breached the policy threshold
// The function compares the current used percentage against the normalized threshold value
func isThresholdBreached(threshold *agInternals.NormalizedAutogrowPolicySpec, usedSizePercent float32) bool {
	return usedSizePercent > threshold.UsedThresholdPercent
}

// generateEventID creates a unique event ID based on timestamp and PV name
// The ID is generated using: hash(timestamp + tvpName) for uniqueness and traceability
// Composite of time + resource identifier
func generateEventID(tvpName string, timestamp time.Time) uint64 {
	// Create composite string: timestamp (nanoseconds) + tvpName
	composite := fmt.Sprintf("%d:%s", timestamp.UnixNano(), tvpName)

	// Hash the composite string using SHA256
	hash := sha256.Sum256([]byte(composite))

	// Convert first 8 bytes of hash to uint64
	// This gives us a 64-bit unique identifier
	eventID := binary.BigEndian.Uint64(hash[:8])

	return eventID
}

// GetState returns the current poller state.
// This method is primarily used for testing to verify state transitions.
func (p *Poller) GetState() State {
	return p.state.get()
}

// SetState sets the poller state directly.
// This method is primarily used for testing to simulate specific state conditions.
func (p *Poller) SetState(state State) {
	p.state.set(state)
}
