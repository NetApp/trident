// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cosi

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"

	"github.com/netapp/trident/config"
	cosicontroller "github.com/netapp/trident/cosi/controller"
	cosisidecar "github.com/netapp/trident/cosi/sidecar"
)

// cosiGroupVersion is the API Trident pins. Checked against the cluster before the manager
// starts, so a missing CRD produces a direct answer rather than a cache-sync timeout.
var cosiGroupVersion = cosiapi.GroupVersion.String()

// newScheme returns the scheme the COSI manager runs with. corev1 is required because the
// sidecar BucketAccess reconciler writes credential Secrets.
func newScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()

	if err := cosiapi.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register COSI types: %w", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register core types: %w", err)
	}

	return scheme, nil
}

// maxConcurrentReconciles is how many objects each COSI controller reconciles at once.
//
// The sidecar calls the driver synchronously, and a slow backend operation (such as a bucket
// delete) blocks every other object behind a single worker. A backend may also cap its own
// parallelism, so this is a ceiling rather than a target, and is kept low enough that queued
// calls stay well under driver request timeouts.
const maxConcurrentReconciles = 10

// managerOptions holds every controller-runtime setting that follows from hosting COSI inside
// Trident rather than as its own process.
func managerOptions(scheme *runtime.Scheme, logger logr.Logger) ctrl.Options {
	return ctrl.Options{
		Scheme: scheme,
		Logger: logger,

		// Trident already owns process lifecycle, health, and metrics; these would collide.
		LeaderElection:         false,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		// Trident traps SIGTERM itself and drives Deactivate.
		GracefulShutdownTimeout: ptr.To(shutdownTimeout),

		Controller: ctrlconfig.Controller{
			// Both reconciler sets register a controller named "bucketaccess"; controller-runtime
			// rejects the duplicate name from its process-wide registry. Metrics are disabled
			// above, so the only visible cost is both logging controller="bucketaccess",
			// distinguishable by message.
			SkipNameValidation: ptr.To(true),

			// Default for every controller in this manager. Set here rather than per controller
			// so the vendored reconcilers stay unedited: controller-runtime applies this to any
			// controller that does not set MaxConcurrentReconciles itself, and none do.
			MaxConcurrentReconciles: maxConcurrentReconciles,
		},

		Client: ctrlclient.Options{
			Cache: &ctrlclient.CacheOptions{
				// Never cache Secrets: the sidecar reads them, and caching would hold every
				// Secret in every COSI-touched namespace in memory and require cluster-wide
				// watch on Secrets.
				DisableFor: []ctrlclient.Object{&corev1.Secret{}},
			},
		},
	}
}

// checkAPIInstalled reports whether the pinned COSI API is served by the cluster, so a missing
// CRD produces a direct error instead of a cache-sync timeout in Activate.
func checkAPIInstalled(restConfig *rest.Config) error {
	client, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("build discovery client: %w", err)
	}

	if err = checkClusterVersion(client); err != nil {
		return err
	}

	if _, err = client.ServerResourcesForGroupVersion(cosiGroupVersion); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("the cluster does not serve %s; install the COSI CRDs Trident ships "+
				"in cosi/crd before enabling COSI: %w", cosiGroupVersion, err)
		}
		return fmt.Errorf("query %s from the cluster: %w", cosiGroupVersion, err)
	}

	return nil
}

// checkClusterVersion rejects clusters too old to run COSI. Checked again here, not just by
// the installers, because --enable_cosi can be set on a controller whose CRDs were installed
// by hand or by an older Trident.
func checkClusterVersion(client discovery.ServerVersionInterface) error {
	versionInfo, err := client.ServerVersion()
	if err != nil {
		return fmt.Errorf("query the Kubernetes server version: %w", err)
	}

	return config.ValidateKubernetesVersionForCOSIFromInfo(versionInfo)
}

// newManager builds the controller-runtime manager that hosts every COSI reconciler: one
// cache, one client, one set of watches, rather than upstream's two processes.
func newManager(restConfig *rest.Config, driverInfo cosisidecar.DriverInfo, logger logr.Logger) (ctrl.Manager, error) {
	scheme, err := newScheme()
	if err != nil {
		return nil, err
	}

	mgr, err := ctrl.NewManager(restConfig, managerOptions(scheme, logger))
	if err != nil {
		return nil, fmt.Errorf("build COSI manager: %w", err)
	}

	if err := registerReconcilers(mgr, driverInfo); err != nil {
		return nil, err
	}

	return mgr, nil
}

// registerReconcilers wires the four live reconcilers into the manager. Deliberately four, not
// five: cosi/controller/bucket.go is still an empty upstream scaffold, left unregistered so a
// re-sync notices if upstream ever fills it in.
func registerReconcilers(mgr ctrl.Manager, driverInfo cosisidecar.DriverInfo) error {
	scheme := mgr.GetScheme()
	client := mgr.GetClient()

	// Central controller: claim-to-Bucket translation and access hand-off. Driver-agnostic.
	claims := &cosicontroller.BucketClaimReconciler{Client: client, Scheme: scheme}
	if err := claims.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("register controller BucketClaim reconciler: %w", err)
	}

	controllerAccess := &cosicontroller.BucketAccessReconciler{Client: client, Scheme: scheme}
	if err := controllerAccess.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("register controller BucketAccess reconciler: %w", err)
	}

	// Sidecar: the halves that call the driver. These filter on driver name, so they ignore
	// objects belonging to any other provisioner.
	buckets := &cosisidecar.BucketReconciler{Client: client, Scheme: scheme, DriverInfo: driverInfo}
	if err := buckets.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("register sidecar Bucket reconciler: %w", err)
	}

	// SkipRegrantWhenGranted: the StorageGRID driver can only answer a grant by minting a new
	// key, so re-granting on every resync and restart would rotate keys under running pods.
	sidecarAccess := &cosisidecar.BucketAccessReconciler{
		Client: client, Scheme: scheme, DriverInfo: driverInfo, SkipRegrantWhenGranted: true,
	}
	if err := sidecarAccess.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("register sidecar BucketAccess reconciler: %w", err)
	}

	return nil
}

// waitForCacheSync blocks until every registered informer has listed its resource, or the
// context is done.
func waitForCacheSync(ctx context.Context, mgr ctrl.Manager) error {
	if !mgr.GetCache().WaitForCacheSync(ctx) {
		return fmt.Errorf("COSI informer caches did not sync: %w", ctx.Err())
	}
	return nil
}
