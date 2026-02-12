// Copyright 2026 NetApp, Inc. All Rights Reserved.

package scheduler

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	stderrors "errors"
	"fmt"
	"math/big"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	agCache "github.com/netapp/trident/frontend/autogrow/cache"
	"github.com/netapp/trident/frontend/autogrow/scheduler/assorter"
	"github.com/netapp/trident/frontend/autogrow/scheduler/assorter/periodic"
	assorterTypes "github.com/netapp/trident/frontend/autogrow/scheduler/assorter/types"
	autogrowTypes "github.com/netapp/trident/frontend/autogrow/types"
	crdtypes "github.com/netapp/trident/frontend/crd/types"
	nodehelpers "github.com/netapp/trident/frontend/csi/node_helpers"
	. "github.com/netapp/trident/logging"
	tridentclientset "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned"
	listerv1 "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	"github.com/netapp/trident/pkg/eventbus"
	eventbusTypes "github.com/netapp/trident/pkg/eventbus/types"
	errors "github.com/netapp/trident/utils/errors"
)

// createPublishFunc creates the publish callback function for the assorter
// This function publishes VolumesScheduled events to the EventBus
// The context is provided by the assorter (with source="assorter")
func createPublishFunc() assorterTypes.PublishFunc {
	return func(ctx context.Context, tvpNames []string) error {
		// Use the context provided by the assorter
		// Generate unique event ID
		timestamp := time.Now()
		eventID := generateEventID(timestamp, len(tvpNames))

		// Create VolumesScheduled event
		event := autogrowTypes.VolumesScheduled{
			Ctx:       ctx,
			ID:        eventID,
			TVPNames:  tvpNames,
			Timestamp: timestamp,
		}

		// Publish to EventBus
		if eventbus.VolumesScheduledEventBus == nil {
			Logc(ctx).Error("VolumesScheduled eventbus not initialized")
			return fmt.Errorf("VolumesScheduled eventbus not initialized")
		}

		eventbus.VolumesScheduledEventBus.Publish(ctx, event)

		Logc(ctx).WithFields(LogFields{
			"eventID":  eventID,
			"tvpCount": len(tvpNames),
		}).Debug("Published VolumesScheduled event")

		return nil
	}
}

// applyConfigDefaults validates the config and fills in missing values with defaults
func applyConfigDefaults(ctx context.Context, config *Config) {
	// Apply defaults for any zero-value fields
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
		Logc(ctx).WithField("maxRetries", config.MaxRetries).Debug("Using default max retries")
	}

	if config.AssorterPeriod <= 0 {
		config.AssorterPeriod = DefaultAssorterPeriod
		Logc(ctx).WithField("period", config.AssorterPeriod).Debug("Using default assorter period")
	}

	if config.ReconciliationPeriod <= 0 {
		config.ReconciliationPeriod = DefaultReconciliationPeriod
		Logc(ctx).WithField("period", config.ReconciliationPeriod).Debug("Using default reconciliation period")
	}

	if config.TridentNamespace == "" {
		config.TridentNamespace = DefaultTridentNamespace
		Logc(ctx).WithField("namespace", config.TridentNamespace).Debug("Using default trident namespace")
	}

	// Note: AssorterType has a valid zero value (AssorterTypePeriodic = 0), so we don't need to check it
}

// createAssorter creates an assorter instance based on the scheduler config
// The assorter is always created and owned by the scheduler (no external injection)
func createAssorter(ctx context.Context, config *Config) (assorterTypes.Assorter, error) {
	Logc(ctx).WithField("assorterType", config.AssorterType.String()).Debug("Creating assorter")

	switch config.AssorterType {
	case AssorterTypePeriodic:
		// Create periodic assorter with publish callback
		Logc(ctx).WithField("period", config.AssorterType).Debug("Creating periodic assorter")
		assorterConfig := periodic.NewConfig(
			periodic.WithPeriod(config.AssorterPeriod),
			periodic.WithPublishFunc(createPublishFunc()),
		)
		assorterInstance, err := assorter.New[assorterTypes.Assorter](ctx, assorterConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to create periodic assorter: %w", err)
		}
		Logc(ctx).WithField("period", config.AssorterType).Info("Created periodic assorter")
		return assorterInstance, nil

	default:
		return nil, fmt.Errorf("unknown assorter type: %v", config.AssorterType)
	}
}

// NewScheduler creates a new Scheduler instance
func NewScheduler(
	ctx context.Context,
	scLister storagelisters.StorageClassLister,
	tvpLister listerv1.TridentVolumePublicationLister,
	tridentClient tridentclientset.Interface,
	autogrowCache *agCache.AutogrowCache,
	volumePublishManager nodehelpers.VolumePublishManager,
	config *Config,
) (*Scheduler, error) {

	// Create a deep copy of the config to avoid mutating the caller's config
	var configCopy *Config
	if config == nil {
		configCopy = DefaultConfig()
	} else {
		configCopy = config.Copy()
	}

	// Fill the config copy with the missing default values.
	applyConfigDefaults(ctx, configCopy)

	logFields := LogFields{
		"max_retries":     configCopy.MaxRetries,
		"assorter_type":   configCopy.AssorterType,
		"assorter_period": configCopy.AssorterPeriod,
	}
	Logc(ctx).WithFields(logFields).Debug(">>>> NewScheduler")
	defer Logc(ctx).WithFields(logFields).Debug("<<<< NewScheduler")

	// Create assorter (always owned by scheduler)
	assorterInstance, err := createAssorter(ctx, configCopy)
	if err != nil {
		return nil, fmt.Errorf("failed to create assorter: %w", err)
	}

	s := &Scheduler{
		scLister:             scLister,
		tvpLister:            tvpLister,
		tridentClient:        tridentClient,
		autogrowCache:        autogrowCache,
		volumePublishManager: volumePublishManager,
		config:               configCopy,
		assorter:             assorterInstance,
	}

	return s, nil
}

// Activate starts the Scheduler layer (idempotent)
// Returns error if already running or if activation fails
func (s *Scheduler) Activate(ctx context.Context) error {
	Logc(ctx).Debug(">>>> scheduler.Activate")
	defer Logc(ctx).Debug("<<<< scheduler.Activate")

	// Check if we can transition to Starting state
	if !s.state.compareAndSwap(StateStopped, StateStarting) {
		currentState := s.state.get()
		if currentState == StateReady || currentState == StateRunning {
			Logc(ctx).Debug("Scheduler already activated, skipping activation")
			return nil // Idempotent
		}
		// Return StateError with state and message - caller will decide what to do
		return errors.NewStateError(currentState.String(), fmt.Sprintf("cannot activate scheduler: current state is %s", currentState.String()))
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	Logc(ctx).Info("Activating Scheduler layer...")

	// Create stop channel
	s.stopCh = make(chan struct{})

	// Create workqueue (always created fresh on each activation)
	s.workqueue = workqueue.NewTypedRateLimitingQueue(
		workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
	)
	Logc(ctx).WithField("workQueueName", s.config.WorkQueueName).Debug("Created workqueue")

	// Subscribe to controller events
	if eventbus.ControllerEventBus == nil {
		s.cleanup(ctx)
		return fmt.Errorf("ControllerEventBus not initialized")
	}
	subscriptionID, err := eventbus.ControllerEventBus.Subscribe(s.handleControllerEvent)
	if err != nil {
		s.cleanup(ctx)
		return fmt.Errorf("failed to subscribe to ControllerEventBus: %w", err)
	}
	s.controllerEventSubscriptionID = subscriptionID
	Logc(ctx).WithField("subscriptionID", subscriptionID).
		Info("Subscribed to controller events successfully")

	// Start assorter
	if err := s.assorter.Activate(ctx); err != nil {
		s.cleanup(ctx)
		return fmt.Errorf("failed to activate assorter: %w", err)
	}

	Logc(ctx).Debug("Assorter has been activated")

	// Bootstrap: populate autogrowCache from TVPs
	// Uses direct clientset API call instead of lister to avoid informer indexer race conditions
	if err := s.bootstrap(ctx); err != nil {
		Logc(ctx).WithError(err).Warn("Bootstrap failed, continuing with empty cache")
		// Don't fail activation on bootstrap error - tvp will be added as events come in
	}

	Logc(ctx).Debug("Autogrow cache has been bootstrapped")

	// Start worker loop (processes items synchronously from workqueue)
	go s.workerLoop(ctx)

	Logc(ctx).Debug("Worker loop has been started")

	// Transition to Ready state
	// The schedule loop will be started separately by the orchestrator via StartScheduleLoop()
	s.state.set(StateReady)

	Logc(ctx).WithField("config", s.config).Info("Scheduler layer activated successfully (Ready)")
	return nil
}

// Deactivate gracefully stops the Scheduler layer (idempotent)
// Drains in-flight jobs before returning
func (s *Scheduler) Deactivate(ctx context.Context) error {
	Logc(ctx).Debug(">>>> scheduler.Deactivate")
	defer Logc(ctx).Debug("<<<< scheduler.Deactivate")

	// Try to transition from either Ready or Running state to Stopping
	// First try from Running
	if !s.state.compareAndSwap(StateRunning, StateStopping) {
		// If not Running, try from Ready
		if !s.state.compareAndSwap(StateReady, StateStopping) {
			// Neither worked - check if we're already stopped (idempotent)
			currentState := s.state.get()
			if currentState == StateStopped {
				Logc(ctx).Debug("Scheduler already stopped, skipping deactivation")
				return nil // Idempotent
			}
			// Return StateError with state and message - caller will decide what to do
			return errors.NewStateError(currentState.String(), fmt.Sprintf("cannot deactivate scheduler: current state is %s", currentState.String()))
		}
	}

	s.stateMu.Lock()
	defer s.stateMu.Unlock()

	Logc(ctx).Info("Deactivating Scheduler layer...")

	// Stop assorter (always owned by scheduler, so always deactivate)
	if err := s.assorter.Deactivate(ctx); err != nil {
		Logc(ctx).WithError(err).Warn("Failed to deactivate assorter")
	} else {
		Logc(ctx).Info("Assorter deactivated successfully")
	}

	// Cleanup (handles eventbus unsubscribe, workqueue shutdown, stop channel close, and state reset)
	s.cleanup(ctx)

	Logc(ctx).WithField("config", s.config).Info("Scheduler layer deactivated successfully")
	return nil
}

// cleanup performs final cleanup operations
func (s *Scheduler) cleanup(ctx context.Context) {
	// Unsubscribe from eventbus
	if s.controllerEventSubscriptionID != 0 {
		if eventbus.ControllerEventBus != nil {
			if unsubscribed := eventbus.ControllerEventBus.Unsubscribe(s.controllerEventSubscriptionID); !unsubscribed {
				Logc(ctx).WithField("subscriptionID", s.controllerEventSubscriptionID).
					Warn("Failed to unsubscribe from ControllerEventBus")
			} else {
				Logc(ctx).WithField("subscriptionID", s.controllerEventSubscriptionID).
					Info("Unsubscribed from ControllerEventBus successfully")
			}
		}
		s.controllerEventSubscriptionID = 0
	}

	// Shutdown work queue (no new items will be added)
	if s.workqueue != nil {
		s.workqueue.ShutDown()
	}

	// Close stop channel if it exists
	if s.stopCh != nil {
		close(s.stopCh)
	}

	// Reset state to Stopped
	s.state.set(StateStopped)
}

// workerLoop continuously pulls work items from the queue and processes them synchronously
// No worker pool needed - all operations are fast in-memory cache lookups
func (s *Scheduler) workerLoop(ctx context.Context) {
	Logc(ctx).Debug(">>>> scheduler.workerLoop started")
	defer Logc(ctx).Debug("<<<< scheduler.workerLoop stopped")

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		default:
			// Get work item from queue
			workItem, shutdown := s.workqueue.Get()
			if shutdown {
				return
			}

			// Check if we can still accept work (allows graceful shutdown)
			if !s.IsRunning() {
				// During shutdown, drain the queue without processing
				s.workqueue.Done(workItem)
				continue
			}

			// Process the work item synchronously (in-memory operations are fast)
			if err := s.processWorkItem(workItem.Ctx, workItem); err != nil {
				// Check if this is a retriable error (wrapped in ReconcileDeferredError)
				if errors.IsReconcileDeferredError(err) {
					// Check if we've exceeded max retries
					if workItem.RetryCount >= s.config.MaxRetries {
						Logc(workItem.Ctx).WithError(err).WithFields(LogFields{
							"key":        workItem.Key,
							"retryCount": workItem.RetryCount,
							"maxRetries": s.config.MaxRetries,
						}).Error("Max retries exceeded, discarding work item")
						s.workqueue.Forget(workItem)
						s.workqueue.Done(workItem)
					} else {
						// Transient failure - re-queue with exponential backoff
						Logc(workItem.Ctx).WithError(err).WithFields(LogFields{
							"key":        workItem.Key,
							"retryCount": workItem.RetryCount,
						}).Warn("Retriable error, re-queuing with backoff")

						// Increment retry count for next attempt
						workItem.RetryCount++
						s.workqueue.Done(workItem)

						// Check if error contains rate limit retry-after hint
						var addedWithDelay bool
						if k8serrors.IsTooManyRequests(err) {
							// Unwrap to access the StatusError details
							var statusErr *k8serrors.StatusError
							if stderrors.As(err, &statusErr) && statusErr.Status().Details != nil && statusErr.Status().Details.RetryAfterSeconds > 0 {
								duration := time.Duration(statusErr.Status().Details.RetryAfterSeconds) * time.Second
								Logc(workItem.Ctx).WithFields(LogFields{
									"key":        workItem.Key,
									"retryAfter": duration,
								}).Warn("Re-queuing with API server's suggested delay")
								s.workqueue.AddAfter(workItem, duration)
								addedWithDelay = true
							}
						}

						// Fallback to exponential backoff if not added with specific delay
						if !addedWithDelay {
							s.workqueue.AddRateLimited(workItem)
						}
					}
				} else {
					// Non-retriable error (programming error, unexpected condition)
					// Log as error but don't retry
					Logc(workItem.Ctx).WithError(err).WithFields(LogFields{
						"key":        workItem.Key,
						"retryCount": workItem.RetryCount,
					}).Error("Non-retriable error occurred, not re-queuing")
					s.workqueue.Forget(workItem)
					s.workqueue.Done(workItem)
				}
			} else {
				// Processing succeeded
				s.workqueue.Forget(workItem)
				s.workqueue.Done(workItem)
			}
		}
	}
}

// IsRunning returns true if the layer is currently running or ready
// Both Ready and Running states indicate the scheduler is operational
func (s *Scheduler) IsRunning() bool {
	state := s.state.get()
	return state == StateReady || state == StateRunning
}

// StartScheduleLoop starts the assorter's schedule loop and the reconciliation loop
// This should be called by the orchestrator after all layers are activated
// Transitions the scheduler from Ready to Running state
func (s *Scheduler) StartScheduleLoop(ctx context.Context) error {
	Logc(ctx).Debug(">>>> scheduler.StartScheduleLoop")
	defer Logc(ctx).Debug("<<<< scheduler.StartScheduleLoop")

	// Check current state
	currentState := s.state.get()
	if currentState != StateReady {
		if currentState == StateRunning {
			Logc(ctx).Debug("Schedule loop already running")
			return nil // Idempotent
		}
		return fmt.Errorf("cannot start schedule loop: scheduler is in %s state (expected Ready)", currentState.String())
	}

	// Start the assorter's schedule loop
	if err := s.assorter.StartScheduleLoop(ctx); err != nil {
		return fmt.Errorf("failed to start assorter schedule loop: %w", err)
	}

	// Start the reconciliation loop in a separate goroutine
	go s.reconciliationLoop(ctx)

	// Transition to Running state
	s.state.set(StateRunning)
	Logc(ctx).WithField("reconciliationPeriod", s.config.ReconciliationPeriod).
		Info("Scheduler schedule loop and reconciliation loop started (Running)")

	return nil
}

// bootstrap reads all TridentVolumePublications (TVPs) and populates the autogrowCache
// with effective policies. This is called during Activate to restore state from TVP CRD objects.
//
// Bootstrap uses TVPs as the source of truth (not tracking files) since TVPs now contain
// StorageClass, BackendUUID, and Pool fields. This approach:
// - Reflects actual published volumes on the node
// - Avoids dependency on tracking file persistence
// - Uses reconcileVolumeEffectivePolicy as single source of truth for cache mutations
func (s *Scheduler) bootstrap(ctx context.Context) error {
	Logc(ctx).Debug(">>>> scheduler.bootstrap")
	defer Logc(ctx).Debug("<<<< scheduler.bootstrap")

	// Create context with "bootstrap" source for contextual logging
	// Use GenerateRequestContext similar to CRD controller pattern
	bootstrapCtx := GenerateRequestContext(ctx, "", "scheduler.bootstrap", WorkflowNone, LogLayerAutogrow)

	// Delegate to the generic sync function
	return s.syncCacheWithAPIServer(bootstrapCtx)
}

// reconciliationLoop periodically reconciles the autogrow cache with the actual state
// of TVPs from the Kubernetes API server. This catches any drift that may have occurred
// due to missed events, informer issues, or other edge cases.
//
// The reconciliation loop runs in a separate goroutine and uses the same logic as bootstrap
// by calling the shared syncCacheWithAPIServer function with a context containing source="reconciliation".
// The loop waits for the configured period AFTER each reconciliation completes.
func (s *Scheduler) reconciliationLoop(ctx context.Context) {
	Logc(ctx).WithField("period", s.config.ReconciliationPeriod).
		Debug(">>>> scheduler.reconciliationLoop started")
	defer Logc(ctx).Debug("<<<< scheduler.reconciliationLoop stopped")

	// Use a timer to wait the full period after each reconciliation completes
	// Start with jittered period to avoid thundering herd
	timer := time.NewTimer(s.getJitteredReconciliationPeriod(ctx))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			Logc(ctx).Debug("Reconciliation loop stopped: context cancelled")
			return
		case <-s.stopCh:
			Logc(ctx).Debug("Reconciliation loop stopped: stop channel closed")
			return
		case <-timer.C:
			// Create context with "reconciliation" source for contextual logging
			// Use GenerateRequestContext similar to CRD controller pattern
			reconciliationCtx := GenerateRequestContext(ctx, "", "scheduler.reconciliation", WorkflowNone, LogLayerAutogrow)

			// Perform reconciliation using the shared sync function
			if err := s.syncCacheWithAPIServer(reconciliationCtx); err != nil {
				Logc(reconciliationCtx).WithError(err).Warn("Reconciliation failed")
			}

			// Reset timer with jittered period to avoid thundering herd on the API server
			timer.Reset(s.getJitteredReconciliationPeriod(ctx))
		}
	}
}

// getJitteredReconciliationPeriod returns the reconciliation period with added jitter.
// It introduces randomness (jitter) between reconciliation periods to avoid a thundering herd on the API server.
// The jitter is calculated as a random duration up to 20% of the base reconciliation period.
func (s *Scheduler) getJitteredReconciliationPeriod(ctx context.Context) time.Duration {
	basePeriod := s.config.ReconciliationPeriod

	// Calculate maximum jitter as 20% of the base period
	maxJitter := basePeriod / 5

	jitter := time.Duration(0)
	if maxJitter > 0 {
		if n, err := rand.Int(rand.Reader, big.NewInt(int64(maxJitter))); err == nil {
			jitter = time.Duration(n.Int64())
		} else {
			Logc(ctx).WithError(err).Debug("Failed to generate random jitter, using zero jitter")
		}
	}

	jitteredPeriod := basePeriod + jitter
	Logc(ctx).WithFields(LogFields{
		"basePeriod":     basePeriod,
		"jitter":         jitter,
		"jitteredPeriod": jitteredPeriod,
	}).Debug("Calculated jittered reconciliation period")

	return jitteredPeriod
}

// handleControllerEvent is the eventbus subscription handler for ControllerEvent
// This receives all controller events (SC, TVP, TBE) and enqueues them for processing
func (s *Scheduler) handleControllerEvent(
	ctx context.Context,
	event autogrowTypes.ControllerEvent,
) (eventbusTypes.Result, error) {
	// Use context from event if provided, otherwise use the handler context
	eventCtx := event.Ctx
	if eventCtx == nil {
		eventCtx = ctx
	}

	Logc(eventCtx).WithFields(LogFields{
		"objectType": event.ObjectType,
		"eventType":  event.EventType,
		"key":        event.Key,
	}).Debug("Received controller event")

	// Enqueue the event to the workqueue
	if err := s.EnqueueEvent(eventCtx, event); err != nil {
		Logc(eventCtx).WithError(err).
			WithField("event", event).
			Warn("Failed to enqueue event from eventbus")
		return nil, err
	}

	Logc(eventCtx).WithFields(LogFields{
		"objectType": event.ObjectType,
		"eventType":  event.EventType,
		"key":        event.Key,
	}).Debug("Controller event enqueued for processing")

	return eventbusTypes.Result{
		"accepted":   true,
		"objectType": string(event.ObjectType),
		"eventType":  string(event.EventType),
		"key":        event.Key,
	}, nil
}

// EnqueueEvent adds a controller event to the work queue
func (s *Scheduler) EnqueueEvent(ctx context.Context, event autogrowTypes.ControllerEvent) error {
	Logc(ctx).WithFields(LogFields{
		"objectType": event.ObjectType,
		"eventType":  event.EventType,
		"key":        event.Key,
	}).Debug(">>>> scheduler.EnqueueEvent")
	defer Logc(ctx).WithFields(LogFields{
		"objectType": event.ObjectType,
		"eventType":  event.EventType,
		"key":        event.Key,
	}).Debug("<<<< scheduler.EnqueueEvent")

	// Check if we can accept work
	if !s.IsRunning() {
		return fmt.Errorf("scheduler not running, rejecting event")
	}

	// Basic validation
	if event.Key == "" {
		Logc(ctx).WithField("event", event).Warn("Invalid event: missing key")
		return fmt.Errorf("invalid event: missing key")
	}

	// Create WorkItem and add to workqueue for async processing
	// Workqueue's built-in deduplication ensures same Key won't be queued twice
	workItem := WorkItem{
		Ctx:        ctx,
		Key:        event.Key,
		ObjectType: event.ObjectType,
		RetryCount: 0,
	}
	s.workqueue.Add(workItem)

	Logc(ctx).WithFields(LogFields{
		"objectType": event.ObjectType,
		"eventType":  event.EventType,
		"key":        event.Key,
	}).Debug("Controller event enqueued for processing")

	return nil
}

// processWorkItem processes a controller event from the workqueue
// It routes to the appropriate handler based on ObjectType
func (s *Scheduler) processWorkItem(ctx context.Context, workItem WorkItem) error {
	Logc(ctx).WithFields(LogFields{
		"key":        workItem.Key,
		"objectType": workItem.ObjectType,
	}).Debug(">>>> scheduler.processWorkItem")
	defer Logc(ctx).WithFields(LogFields{
		"key":        workItem.Key,
		"objectType": workItem.ObjectType,
	}).Debug("<<<< scheduler.processWorkItem")

	// Route to appropriate handler based on ObjectType
	switch workItem.ObjectType {
	case crdtypes.ObjectTypeStorageClass:
		// StorageClass event - key is just the SC name
		return s.handleStorageClassEvent(ctx, workItem.Key)

	case crdtypes.ObjectTypeTridentVolumePublication:
		// TVP event - key is namespace/name where name is volumeID.nodeID
		// Split to extract just the name (tvpName)
		_, tvpName, err := cache.SplitMetaNamespaceKey(workItem.Key)
		if err != nil {
			Logc(ctx).WithError(err).WithField("key", workItem.Key).
				Warn("Invalid TVP key format")
			return fmt.Errorf("invalid TVP key format: %w", err)
		}
		return s.handleTVPEvent(ctx, tvpName)

	case crdtypes.ObjectTypeTridentBackend:
		Logc(ctx).WithField("key", workItem.Key).
			Warn("TBE event not implemented")
		return fmt.Errorf("TBE event not implemented, key: %s", workItem.Key)

	default:
		Logc(ctx).WithField("objectType", workItem.ObjectType).
			Warn("Unknown object type in work item")
		return fmt.Errorf("unknown object type: %s", workItem.ObjectType)
	}
}

// generateEventID creates a unique event ID based on timestamp and tvpCount count
func generateEventID(timestamp time.Time, tvpCount int) uint64 {
	// Create composite string: timestamp (nanoseconds) + tvpCount
	composite := fmt.Sprintf("%d:%d", timestamp.UnixNano(), tvpCount)

	// Hash the composite string using SHA256
	hash := sha256.Sum256([]byte(composite))

	// Convert first 8 bytes of hash to uint64
	return binary.BigEndian.Uint64(hash[:8])
}

// GetState returns the current scheduler state.
// This method is primarily used for testing to verify state transitions.
func (s *Scheduler) GetState() State {
	return s.state.get()
}

// SetState sets the scheduler state directly.
// This method is primarily used for testing to simulate specific state conditions.
func (s *Scheduler) SetState(state State) {
	s.state.set(state)
}
