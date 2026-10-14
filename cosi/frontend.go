// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cosi

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	cosiproto "sigs.k8s.io/container-object-storage-interface/proto"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/cosi/adapter"
	"github.com/netapp/trident/cosi/provisioner"
	cosisidecar "github.com/netapp/trident/cosi/sidecar"
	"github.com/netapp/trident/frontend"
	. "github.com/netapp/trident/logging"
)

const (
	// frontendName is what Trident calls this plugin in logs and frontend listings.
	frontendName = "cosi"
)

var (
	// activateTimeout bounds how long Activate waits for informer caches to sync. The common
	// cause of never syncing is missing RBAC, not a slow API server.
	activateTimeout = 60 * time.Second

	// shutdownTimeout bounds how long Deactivate waits for in-flight reconciles to finish.
	shutdownTimeout = 30 * time.Second

	// deactivateWaitSlack is extra time Deactivate waits after the manager's own
	// GracefulShutdownTimeout before giving up.
	deactivateWaitSlack = 5 * time.Second
)

// setLoggerOnce guards controller-runtime's process-global logger, set once per process.
var setLoggerOnce sync.Once

var _ frontend.Plugin = &Frontend{}

// Frontend runs the COSI reconcilers inside the Trident controller process, with no sidecar
// container or gRPC hop. See design-docs/cosi.
type Frontend struct {
	restConfig  *rest.Config
	provisioner provisioner.Provisioner

	// mutex guards everything below, which is written by Activate and read by Deactivate.
	mutex   sync.Mutex
	cancel  context.CancelFunc
	stopped chan struct{}
	// driverName is only known after the provisioner has been asked, which happens in
	// Activate. Kept for logging.
	driverName string
}

// NewFrontend returns a COSI frontend that provisions through p. Nothing is contacted until
// Activate, since Trident constructs frontends before the core is bootstrapped.
func NewFrontend(p provisioner.Provisioner, restConfig *rest.Config) (*Frontend, error) {
	if p == nil {
		return nil, fmt.Errorf("cosi: provisioner is required")
	}
	if restConfig == nil {
		return nil, fmt.Errorf("cosi: rest config is required")
	}

	setLoggerOnce.Do(func() { ctrl.SetLogger(newLogger()) })

	return &Frontend{restConfig: restConfig, provisioner: p}, nil
}

// Activate starts the COSI reconcilers and returns once they are watching.
func (f *Frontend) Activate() error {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginActivate, LogLayerCOSIFrontend)

	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.stopped != nil {
		return fmt.Errorf("cosi: frontend is already active")
	}

	Logc(ctx).Info("Activating COSI frontend.")

	if err := checkAPIInstalled(f.restConfig); err != nil {
		return err
	}

	driverInfo, err := f.newDriverInfo(ctx)
	if err != nil {
		return err
	}
	f.driverName = driverInfo.Name

	mgr, err := newManager(f.restConfig, driverInfo, newLogger())
	if err != nil {
		return err
	}

	// The manager's context outlives Activate: it is the run context, cancelled by Deactivate.
	runCtx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		if err := mgr.Start(runCtx); err != nil {
			Logc(ctx).WithError(err).Error("COSI manager stopped with an error.")
		}
	}()

	// Wait for caches to sync before declaring success, so a missing RBAC rule or unreachable
	// API server is reported here rather than as buckets that silently never provision.
	syncCtx, cancelSync := context.WithTimeout(runCtx, activateTimeout)
	defer cancelSync()

	if err := waitForCacheSync(syncCtx, mgr); err != nil {
		cancel()
		<-stopped
		return fmt.Errorf("cosi: %w; check that Trident's RBAC grants list/watch on %s",
			err, cosiGroupVersion)
	}

	f.cancel = cancel
	f.stopped = stopped

	Logc(ctx).WithFields(LogFields{
		"driverName": f.driverName,
		"apiVersion": cosiGroupVersion,
	}).Info("Activated COSI frontend.")

	return nil
}

// Deactivate stops the reconcilers and waits for in-flight work to finish.
func (f *Frontend) Deactivate() error {
	ctx := GenerateRequestContext(nil, "", ContextSourceInternal, WorkflowPluginDeactivate, LogLayerCOSIFrontend)

	f.mutex.Lock()
	defer f.mutex.Unlock()

	if f.stopped == nil {
		// Never activated, or already stopped. Deactivate is called unconditionally on
		// shutdown, so this is a normal path, not an error.
		return nil
	}

	Logc(ctx).Info("Deactivating COSI frontend.")

	f.cancel()

	select {
	case <-f.stopped:
		Logc(ctx).Info("Deactivated COSI frontend.")
	case <-time.After(shutdownTimeout + deactivateWaitSlack):
		// The manager has its own GracefulShutdownTimeout; exceeding it here means a
		// reconcile is wedged. Report rather than block Trident's shutdown indefinitely.
		Logc(ctx).Warning("COSI frontend did not stop within the shutdown timeout.")
	}

	f.cancel = nil
	f.stopped = nil

	return nil
}

func (f *Frontend) GetName() string {
	return frontendName
}

func (f *Frontend) Version() string {
	return config.OrchestratorAPIVersion
}

// newDriverInfo performs the driver handshake: ask the provisioner its name and supported
// protocols, then validate the answer. A name that fails validation would make the
// reconcilers' driver-name predicate match nothing, so COSI would appear healthy while
// ignoring every Bucket.
func (f *Frontend) newDriverInfo(ctx context.Context) (cosisidecar.DriverInfo, error) {
	client := adapter.New(f.provisioner)

	reported, err := client.DriverGetInfo(ctx, &cosiproto.DriverGetInfoRequest{})
	if err != nil {
		return cosisidecar.DriverInfo{}, fmt.Errorf("cosi: query provisioner driver info: %w", err)
	}

	driverInfo, err := cosisidecar.NewDriverInfo(reported, client)
	if err != nil {
		return cosisidecar.DriverInfo{}, fmt.Errorf("cosi: provisioner reported unusable driver info: %w", err)
	}

	return *driverInfo, nil
}
