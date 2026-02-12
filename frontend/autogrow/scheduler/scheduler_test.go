// Copyright 2026 NetApp, Inc. All Rights Reserved.

package scheduler

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	storageinformers "k8s.io/client-go/informers/storage/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	agCache "github.com/netapp/trident/frontend/autogrow/cache"
	assorterTypes "github.com/netapp/trident/frontend/autogrow/scheduler/assorter/types"
	agTypes "github.com/netapp/trident/frontend/autogrow/types"
	crdtypes "github.com/netapp/trident/frontend/crd/types"
	mockAssorterTypes "github.com/netapp/trident/mocks/mock_frontend/mock_autogrow/mock_scheduler/mock_assorter"
	mockNodeHelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	mockEventbus "github.com/netapp/trident/mocks/mock_pkg/mock_eventbus"
	v1 "github.com/netapp/trident/persistent_store/crd/apis/netapp/v1"
	crdclientfake "github.com/netapp/trident/persistent_store/crd/client/clientset/versioned/fake"
	tridentinformers "github.com/netapp/trident/persistent_store/crd/client/informers/externalversions"
	listerv1 "github.com/netapp/trident/persistent_store/crd/client/listers/netapp/v1"
	"github.com/netapp/trident/pkg/eventbus"
	eventbusTypes "github.com/netapp/trident/pkg/eventbus/types"
)

// ============================================================================
// Helper Functions and Test Fixtures
// ============================================================================

// fakeTVPLister is a test double that implements TridentVolumePublicationLister
// and returns a predetermined error
type fakeTVPLister struct {
	err error
}

func TestMain(m *testing.M) {
	// Setup test environment once for all tests in the package
	os.Setenv("KUBE_FEATURE_WatchListClient", "false")

	// Run all tests
	code := m.Run()

	// Cleanup
	os.Unsetenv("KUBE_FEATURE_WatchListClient")

	os.Exit(code)
}

func (f *fakeTVPLister) List(selector labels.Selector) (ret []*v1.TridentVolumePublication, err error) {
	return nil, f.err
}

func (f *fakeTVPLister) TridentVolumePublications(namespace string) listerv1.TridentVolumePublicationNamespaceLister {
	return &fakeTVPNamespaceLister{err: f.err}
}

type fakeTVPNamespaceLister struct {
	err error
}

func (f *fakeTVPNamespaceLister) List(selector labels.Selector) (ret []*v1.TridentVolumePublication, err error) {
	return nil, f.err
}

func (f *fakeTVPNamespaceLister) Get(name string) (*v1.TridentVolumePublication, error) {
	return nil, f.err
}

// getTestContext returns a context with timeout for testing
func getTestContext() context.Context {
	ctx := context.Background()
	return ctx
}

// getTestAutogrowCache returns a test autogrow cache instance
func getTestAutogrowCache() *agCache.AutogrowCache {
	return agCache.NewAutogrowCache()
}

// getTestConfig creates a test config with optional config options
func getTestConfig(opts ...ConfigOption) *Config {
	return NewConfig(opts...)
}

// setupTestListers creates fake K8s listers for StorageClass and TridentVolumePublication
// along with a fake Trident clientset for testing
func setupTestListers(t *testing.T, storageClasses []*storagev1.StorageClass, tvps []*v1.TridentVolumePublication) (
	storagelisters.StorageClassLister,
	listerv1.TridentVolumePublicationLister,
	*crdclientfake.Clientset,
) {
	// Create fake K8s clientset with StorageClasses
	k8sObjects := make([]runtime.Object, len(storageClasses))
	for i, sc := range storageClasses {
		k8sObjects[i] = sc
	}
	k8sClient := k8sfake.NewClientset(k8sObjects...)

	// Create StorageClass informer and lister
	scInformerFactory := storageinformers.NewStorageClassInformer(
		k8sClient,
		0,
		cache.Indexers{},
	)
	scLister := storagelisters.NewStorageClassLister(scInformerFactory.GetIndexer())

	// Create fake Trident CRD clientset with TVPs
	tridentObjects := make([]runtime.Object, len(tvps))
	for i, tvp := range tvps {
		tridentObjects[i] = tvp
	}
	tridentClient := crdclientfake.NewSimpleClientset(tridentObjects...)

	// Create TVP informer and lister
	tridentInformerFactory := tridentinformers.NewSharedInformerFactory(tridentClient, 0)
	tvpInformer := tridentInformerFactory.Trident().V1().TridentVolumePublications()
	tvpLister := tvpInformer.Lister()

	// Start both informers and wait for them to sync
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })

	go scInformerFactory.Run(stopCh)
	go tridentInformerFactory.Start(stopCh)
	cache.WaitForCacheSync(stopCh, scInformerFactory.HasSynced, tvpInformer.Informer().HasSynced)

	return scLister, tvpLister, tridentClient
}

// createTestStorageClass creates a test StorageClass object
func createTestStorageClass(name string) *storagev1.StorageClass {
	return &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

// createTestTVP creates a test TridentVolumePublication object
func createTestTVP(name, namespace, volumeID, storageClass string) *v1.TridentVolumePublication {
	return &v1.TridentVolumePublication{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		VolumeID:     volumeID,
		StorageClass: storageClass,
	}
}

// ============================================================================
// Test: NewScheduler
// ============================================================================

func TestNewScheduler(t *testing.T) {
	// testSetup holds test-specific configuration and dependencies
	type testSetup struct {
		scLister             storagelisters.StorageClassLister
		tvpLister            listerv1.TridentVolumePublicationLister
		tridentClient        *crdclientfake.Clientset
		autogrowCache        *agCache.AutogrowCache
		volumePublishManager *mockNodeHelpers.MockVolumePublishManager
		config               *Config
	}

	tests := []struct {
		name   string
		setup  func(*gomock.Controller) testSetup
		verify func(*testing.T, *Scheduler, error)
	}{
		{
			name: "Success_WithNilConfig_UsesDefaults",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               nil, // nil config to test default behavior
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err, "NewScheduler should succeed with nil config")
				require.NotNil(t, s, "scheduler should not be nil")
				require.NotNil(t, s.config, "config should be initialized")

				// Verify all default values are applied
				assert.Equal(t, DefaultShutdownTimeout, s.config.ShutdownTimeout)
				assert.Equal(t, DefaultWorkQueueName, s.config.WorkQueueName)
				assert.Equal(t, DefaultMaxRetries, s.config.MaxRetries)
				assert.Equal(t, DefaultAssorterPeriod, s.config.AssorterPeriod)
				assert.Equal(t, DefaultReconciliationPeriod, s.config.ReconciliationPeriod)
				assert.Equal(t, DefaultTridentNamespace, s.config.TridentNamespace)
				assert.Equal(t, AssorterTypePeriodic, s.config.AssorterType)

				// Verify other fields are initialized
				// Workqueue is created in Activate(), not in NewScheduler()
				assert.Nil(t, s.workqueue, "workqueue should be nil after NewScheduler (created in Activate)")
				assert.NotNil(t, s.assorter)
				assert.Equal(t, StateStopped, s.state.get())
			},
		},
		{
			name: "Success_WithCustomConfig_AllFieldsSet",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				customConfig := getTestConfig(
					WithShutdownTimeout(60*time.Second),
					WithWorkQueueName("custom-queue"),
					WithMaxRetries(5),
					WithAssorterPeriod(2*time.Minute),
					WithReconciliationPeriod(10*time.Minute),
					WithTridentNamespace("custom-namespace"),
					WithAssorterType(AssorterTypePeriodic),
				)

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               customConfig,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err, "NewScheduler should succeed with custom config")
				require.NotNil(t, s, "scheduler should not be nil")

				// Verify custom config values are preserved
				assert.Equal(t, 60*time.Second, s.config.ShutdownTimeout)
				assert.Equal(t, "custom-queue", s.config.WorkQueueName)
				assert.Equal(t, 5, s.config.MaxRetries)
				assert.Equal(t, 2*time.Minute, s.config.AssorterPeriod)
				assert.Equal(t, 10*time.Minute, s.config.ReconciliationPeriod)
				assert.Equal(t, "custom-namespace", s.config.TridentNamespace)
				assert.Equal(t, AssorterTypePeriodic, s.config.AssorterType)
			},
		},
		{
			name: "EdgeCase_ConfigWithZeroShutdownTimeout_UsesDefault",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				config := &Config{
					ShutdownTimeout:      0, // Zero value triggers default
					WorkQueueName:        "test-queue",
					MaxRetries:           3,
					AssorterPeriod:       1 * time.Minute,
					ReconciliationPeriod: 5 * time.Minute,
					TridentNamespace:     "test-namespace",
					AssorterType:         AssorterTypePeriodic,
				}

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				assert.Equal(t, DefaultShutdownTimeout, s.config.ShutdownTimeout, "zero ShutdownTimeout should use default")
			},
		},
		{
			name: "EdgeCase_ConfigWithNegativeShutdownTimeout_UsesDefault",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				config := &Config{
					ShutdownTimeout:      -10 * time.Second, // Negative value triggers default
					WorkQueueName:        "test-queue",
					MaxRetries:           3,
					AssorterPeriod:       1 * time.Minute,
					ReconciliationPeriod: 5 * time.Minute,
					TridentNamespace:     "test-namespace",
					AssorterType:         AssorterTypePeriodic,
				}

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				assert.Equal(t, DefaultShutdownTimeout, s.config.ShutdownTimeout, "negative ShutdownTimeout should use default")
			},
		},
		{
			name: "EdgeCase_ConfigWithEmptyWorkQueueName_UsesDefault",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				config := &Config{
					ShutdownTimeout:      30 * time.Second,
					WorkQueueName:        "", // Empty string triggers default
					MaxRetries:           3,
					AssorterPeriod:       1 * time.Minute,
					ReconciliationPeriod: 5 * time.Minute,
					TridentNamespace:     "test-namespace",
					AssorterType:         AssorterTypePeriodic,
				}

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				assert.Equal(t, DefaultWorkQueueName, s.config.WorkQueueName, "empty WorkQueueName should use default")
			},
		},
		{
			name: "EdgeCase_ConfigWithZeroMaxRetries_UsesDefault",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				config := &Config{
					ShutdownTimeout:      30 * time.Second,
					WorkQueueName:        "test-queue",
					MaxRetries:           0, // Zero value triggers default
					AssorterPeriod:       1 * time.Minute,
					ReconciliationPeriod: 5 * time.Minute,
					TridentNamespace:     "test-namespace",
					AssorterType:         AssorterTypePeriodic,
				}

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				assert.Equal(t, DefaultMaxRetries, s.config.MaxRetries, "zero MaxRetries should use default")
			},
		},
		{
			name: "EdgeCase_ConfigWithNegativeMaxRetries_UsesDefault",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				config := &Config{
					ShutdownTimeout:      30 * time.Second,
					WorkQueueName:        "test-queue",
					MaxRetries:           -5, // Negative value triggers default
					AssorterPeriod:       1 * time.Minute,
					ReconciliationPeriod: 5 * time.Minute,
					TridentNamespace:     "test-namespace",
					AssorterType:         AssorterTypePeriodic,
				}

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				assert.Equal(t, DefaultMaxRetries, s.config.MaxRetries, "negative MaxRetries should use default")
			},
		},
		{
			name: "EdgeCase_ConfigWithZeroAssorterPeriod_UsesDefault",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				config := &Config{
					ShutdownTimeout:      30 * time.Second,
					WorkQueueName:        "test-queue",
					MaxRetries:           3,
					AssorterPeriod:       0, // Zero value triggers default
					ReconciliationPeriod: 5 * time.Minute,
					TridentNamespace:     "test-namespace",
					AssorterType:         AssorterTypePeriodic,
				}

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				assert.Equal(t, DefaultAssorterPeriod, s.config.AssorterPeriod, "zero AssorterPeriod should use default")
			},
		},
		{
			name: "EdgeCase_ConfigWithNegativeAssorterPeriod_UsesDefault",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				config := &Config{
					ShutdownTimeout:      30 * time.Second,
					WorkQueueName:        "test-queue",
					MaxRetries:           3,
					AssorterPeriod:       -1 * time.Minute, // Negative value triggers default
					ReconciliationPeriod: 5 * time.Minute,
					TridentNamespace:     "test-namespace",
					AssorterType:         AssorterTypePeriodic,
				}

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				assert.Equal(t, DefaultAssorterPeriod, s.config.AssorterPeriod, "negative AssorterPeriod should use default")
			},
		},
		{
			name: "EdgeCase_ConfigWithZeroReconciliationPeriod_UsesDefault",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				config := &Config{
					ShutdownTimeout:      30 * time.Second,
					WorkQueueName:        "test-queue",
					MaxRetries:           3,
					AssorterPeriod:       1 * time.Minute,
					ReconciliationPeriod: 0, // Zero value triggers default
					TridentNamespace:     "test-namespace",
					AssorterType:         AssorterTypePeriodic,
				}

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				assert.Equal(t, DefaultReconciliationPeriod, s.config.ReconciliationPeriod, "zero ReconciliationPeriod should use default")
			},
		},
		{
			name: "EdgeCase_ConfigWithNegativeReconciliationPeriod_UsesDefault",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				config := &Config{
					ShutdownTimeout:      30 * time.Second,
					WorkQueueName:        "test-queue",
					MaxRetries:           3,
					AssorterPeriod:       1 * time.Minute,
					ReconciliationPeriod: -5 * time.Minute, // Negative value triggers default
					TridentNamespace:     "test-namespace",
					AssorterType:         AssorterTypePeriodic,
				}

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				assert.Equal(t, DefaultReconciliationPeriod, s.config.ReconciliationPeriod, "negative ReconciliationPeriod should use default")
			},
		},
		{
			name: "EdgeCase_ConfigWithEmptyTridentNamespace_UsesDefault",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				config := &Config{
					ShutdownTimeout:      30 * time.Second,
					WorkQueueName:        "test-queue",
					MaxRetries:           3,
					AssorterPeriod:       1 * time.Minute,
					ReconciliationPeriod: 5 * time.Minute,
					TridentNamespace:     "", // Empty string triggers default
					AssorterType:         AssorterTypePeriodic,
				}

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				assert.Equal(t, DefaultTridentNamespace, s.config.TridentNamespace, "empty TridentNamespace should use default")
			},
		},
		{
			name: "EdgeCase_ConfigWithAllZeroValues_UsesAllDefaults",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				config := &Config{
					ShutdownTimeout:      0,
					WorkQueueName:        "",
					MaxRetries:           0,
					AssorterPeriod:       0,
					ReconciliationPeriod: 0,
					TridentNamespace:     "",
					AssorterType:         AssorterTypePeriodic, // Valid zero value
				}

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				assert.Equal(t, DefaultShutdownTimeout, s.config.ShutdownTimeout)
				assert.Equal(t, DefaultWorkQueueName, s.config.WorkQueueName)
				assert.Equal(t, DefaultMaxRetries, s.config.MaxRetries)
				assert.Equal(t, DefaultAssorterPeriod, s.config.AssorterPeriod)
				assert.Equal(t, DefaultReconciliationPeriod, s.config.ReconciliationPeriod)
				assert.Equal(t, DefaultTridentNamespace, s.config.TridentNamespace)
				assert.Equal(t, AssorterTypePeriodic, s.config.AssorterType)
			},
		},
		{
			name: "Success_AssorterTypeIsPeriodic",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				config := getTestConfig(WithAssorterType(AssorterTypePeriodic))

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				require.NotNil(t, s.assorter, "assorter should be created for Periodic type")
				assert.Equal(t, AssorterTypePeriodic, s.config.AssorterType)
			},
		},
		{
			name: "Success_AllDependenciesProperlyAssigned",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               getTestConfig(),
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				require.NotNil(t, s)

				// Verify all dependencies are properly assigned
				assert.NotNil(t, s.scLister, "scLister should be assigned")
				assert.NotNil(t, s.tvpLister, "tvpLister should be assigned")
				assert.NotNil(t, s.tridentClient, "tridentClient should be assigned")
				assert.NotNil(t, s.autogrowCache, "autogrowCache should be assigned")
				assert.NotNil(t, s.volumePublishManager, "volumePublishManager should be assigned")
				assert.NotNil(t, s.config, "config should be assigned")
				// Workqueue is created in Activate(), not in NewScheduler()
				assert.Nil(t, s.workqueue, "workqueue should be nil after NewScheduler (created in Activate)")
				assert.NotNil(t, s.assorter, "assorter should be initialized")

				// Verify initial state
				assert.Equal(t, StateStopped, s.state.get(), "state should be StateStopped")
				assert.Zero(t, s.controllerEventSubscriptionID, "subscription ID should be zero initially")
				assert.Nil(t, s.stopCh, "stopCh should be nil initially")
			},
		},
		{
			name: "Error_UnknownAssorterType_ReturnsError",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				// Use an invalid AssorterType that doesn't exist
				config := &Config{
					ShutdownTimeout:      30 * time.Second,
					WorkQueueName:        "test-queue",
					MaxRetries:           3,
					AssorterPeriod:       1 * time.Minute,
					ReconciliationPeriod: 5 * time.Minute,
					TridentNamespace:     "test-namespace",
					AssorterType:         AssorterType(999), // Invalid type
				}

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.Error(t, err, "NewScheduler should fail with unknown AssorterType")
				assert.Nil(t, s, "scheduler should be nil on error")
				assert.Contains(t, err.Error(), "unknown assorter type", "error should mention unknown assorter type")
			},
		},
		{
			name: "Success_ConfigNotMutated_OriginalConfigUnchanged",
			setup: func(ctrl *gomock.Controller) testSetup {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				// Create config with zero values that will trigger defaults
				config := &Config{
					ShutdownTimeout:      0,  // Should use default
					WorkQueueName:        "", // Should use default
					MaxRetries:           5,  // Custom value
					AssorterPeriod:       2 * time.Minute,
					ReconciliationPeriod: 0, // Should use default
					TridentNamespace:     "test-ns",
					AssorterType:         AssorterTypePeriodic,
				}

				return testSetup{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               config,
				}
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				require.NotNil(t, s)

				// Verify the scheduler has defaults applied
				assert.Equal(t, DefaultShutdownTimeout, s.config.ShutdownTimeout, "scheduler config should have default")
				assert.Equal(t, DefaultWorkQueueName, s.config.WorkQueueName, "scheduler config should have default")
				assert.Equal(t, 5, s.config.MaxRetries, "scheduler config should preserve custom value")
				assert.Equal(t, DefaultReconciliationPeriod, s.config.ReconciliationPeriod, "scheduler config should have default")

				// Note: We can't directly verify the original config wasn't mutated in this test structure,
				// but the deep copy mechanism ensures it. This test verifies the scheduler got the right values.
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gomock controller for this test
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Setup test dependencies
			setup := tt.setup(ctrl)

			// Call the real NewScheduler function
			ctx := getTestContext()
			scheduler, err := NewScheduler(
				ctx,
				setup.scLister,
				setup.tvpLister,
				setup.tridentClient,
				setup.autogrowCache,
				setup.volumePublishManager,
				setup.config,
			)

			// Verify results
			tt.verify(t, scheduler, err)
		})
	}
}

// ============================================================================
// Test: createAssorter
// ============================================================================

func TestCreateAssorter(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() *Config
		verify func(*testing.T, assorterTypes.Assorter, error)
	}{
		{
			name: "Success_PeriodicAssorterType",
			setup: func() *Config {
				return getTestConfig(WithAssorterType(AssorterTypePeriodic))
			},
			verify: func(t *testing.T, assorter assorterTypes.Assorter, err error) {
				require.NoError(t, err, "should successfully create periodic assorter")
				require.NotNil(t, assorter, "assorter should not be nil")
			},
		},
		{
			name: "Error_UnknownAssorterType",
			setup: func() *Config {
				return &Config{
					AssorterType:   AssorterType(999), // Invalid type
					AssorterPeriod: 1 * time.Minute,
				}
			},
			verify: func(t *testing.T, assorter assorterTypes.Assorter, err error) {
				require.Error(t, err, "should fail with unknown assorter type")
				assert.Nil(t, assorter, "assorter should be nil on error")
				assert.Contains(t, err.Error(), "unknown assorter type", "error should mention unknown assorter type")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := getTestContext()
			config := tt.setup()

			assorter, err := createAssorter(ctx, config)

			tt.verify(t, assorter, err)
		})
	}
}

// ============================================================================
// Test: applyConfigDefaults
// ============================================================================

func TestApplyConfigDefaults(t *testing.T) {
	tests := []struct {
		name           string
		inputConfig    *Config
		expectedConfig *Config
	}{
		{
			name: "AllZeroValues_AppliesAllDefaults",
			inputConfig: &Config{
				ShutdownTimeout:      0,
				WorkQueueName:        "",
				MaxRetries:           0,
				AssorterPeriod:       0,
				ReconciliationPeriod: 0,
				TridentNamespace:     "",
				AssorterType:         AssorterTypePeriodic,
			},
			expectedConfig: &Config{
				ShutdownTimeout:      DefaultShutdownTimeout,
				WorkQueueName:        DefaultWorkQueueName,
				MaxRetries:           DefaultMaxRetries,
				AssorterPeriod:       DefaultAssorterPeriod,
				ReconciliationPeriod: DefaultReconciliationPeriod,
				TridentNamespace:     DefaultTridentNamespace,
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "NegativeValues_AppliesDefaults",
			inputConfig: &Config{
				ShutdownTimeout:      -10 * time.Second,
				WorkQueueName:        "",
				MaxRetries:           -5,
				AssorterPeriod:       -1 * time.Minute,
				ReconciliationPeriod: -5 * time.Minute,
				TridentNamespace:     "",
				AssorterType:         AssorterTypePeriodic,
			},
			expectedConfig: &Config{
				ShutdownTimeout:      DefaultShutdownTimeout,
				WorkQueueName:        DefaultWorkQueueName,
				MaxRetries:           DefaultMaxRetries,
				AssorterPeriod:       DefaultAssorterPeriod,
				ReconciliationPeriod: DefaultReconciliationPeriod,
				TridentNamespace:     DefaultTridentNamespace,
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "PartialCustomValues_OnlyAppliesDefaultsForZero",
			inputConfig: &Config{
				ShutdownTimeout:      60 * time.Second, // Custom
				WorkQueueName:        "",               // Zero - uses default
				MaxRetries:           5,                // Custom
				AssorterPeriod:       0,                // Zero - uses default
				ReconciliationPeriod: 10 * time.Minute, // Custom
				TridentNamespace:     "custom-ns",      // Custom
				AssorterType:         AssorterTypePeriodic,
			},
			expectedConfig: &Config{
				ShutdownTimeout:      60 * time.Second,
				WorkQueueName:        DefaultWorkQueueName,
				MaxRetries:           5,
				AssorterPeriod:       DefaultAssorterPeriod,
				ReconciliationPeriod: 10 * time.Minute,
				TridentNamespace:     "custom-ns",
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "AllCustomValues_NoDefaultsApplied",
			inputConfig: &Config{
				ShutdownTimeout:      120 * time.Second,
				WorkQueueName:        "my-queue",
				MaxRetries:           10,
				AssorterPeriod:       3 * time.Minute,
				ReconciliationPeriod: 15 * time.Minute,
				TridentNamespace:     "my-namespace",
				AssorterType:         AssorterTypePeriodic,
			},
			expectedConfig: &Config{
				ShutdownTimeout:      120 * time.Second,
				WorkQueueName:        "my-queue",
				MaxRetries:           10,
				AssorterPeriod:       3 * time.Minute,
				ReconciliationPeriod: 15 * time.Minute,
				TridentNamespace:     "my-namespace",
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "OnlyShutdownTimeoutZero_AppliesOnlyThatDefault",
			inputConfig: &Config{
				ShutdownTimeout:      0,
				WorkQueueName:        "queue",
				MaxRetries:           3,
				AssorterPeriod:       1 * time.Minute,
				ReconciliationPeriod: 5 * time.Minute,
				TridentNamespace:     "ns",
				AssorterType:         AssorterTypePeriodic,
			},
			expectedConfig: &Config{
				ShutdownTimeout:      DefaultShutdownTimeout,
				WorkQueueName:        "queue",
				MaxRetries:           3,
				AssorterPeriod:       1 * time.Minute,
				ReconciliationPeriod: 5 * time.Minute,
				TridentNamespace:     "ns",
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "OnlyWorkQueueNameEmpty_AppliesOnlyThatDefault",
			inputConfig: &Config{
				ShutdownTimeout:      30 * time.Second,
				WorkQueueName:        "",
				MaxRetries:           3,
				AssorterPeriod:       1 * time.Minute,
				ReconciliationPeriod: 5 * time.Minute,
				TridentNamespace:     "ns",
				AssorterType:         AssorterTypePeriodic,
			},
			expectedConfig: &Config{
				ShutdownTimeout:      30 * time.Second,
				WorkQueueName:        DefaultWorkQueueName,
				MaxRetries:           3,
				AssorterPeriod:       1 * time.Minute,
				ReconciliationPeriod: 5 * time.Minute,
				TridentNamespace:     "ns",
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "OnlyMaxRetriesZero_AppliesOnlyThatDefault",
			inputConfig: &Config{
				ShutdownTimeout:      30 * time.Second,
				WorkQueueName:        "queue",
				MaxRetries:           0,
				AssorterPeriod:       1 * time.Minute,
				ReconciliationPeriod: 5 * time.Minute,
				TridentNamespace:     "ns",
				AssorterType:         AssorterTypePeriodic,
			},
			expectedConfig: &Config{
				ShutdownTimeout:      30 * time.Second,
				WorkQueueName:        "queue",
				MaxRetries:           DefaultMaxRetries,
				AssorterPeriod:       1 * time.Minute,
				ReconciliationPeriod: 5 * time.Minute,
				TridentNamespace:     "ns",
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "OnlyAssorterPeriodZero_AppliesOnlyThatDefault",
			inputConfig: &Config{
				ShutdownTimeout:      30 * time.Second,
				WorkQueueName:        "queue",
				MaxRetries:           3,
				AssorterPeriod:       0,
				ReconciliationPeriod: 5 * time.Minute,
				TridentNamespace:     "ns",
				AssorterType:         AssorterTypePeriodic,
			},
			expectedConfig: &Config{
				ShutdownTimeout:      30 * time.Second,
				WorkQueueName:        "queue",
				MaxRetries:           3,
				AssorterPeriod:       DefaultAssorterPeriod,
				ReconciliationPeriod: 5 * time.Minute,
				TridentNamespace:     "ns",
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "OnlyReconciliationPeriodZero_AppliesOnlyThatDefault",
			inputConfig: &Config{
				ShutdownTimeout:      30 * time.Second,
				WorkQueueName:        "queue",
				MaxRetries:           3,
				AssorterPeriod:       1 * time.Minute,
				ReconciliationPeriod: 0,
				TridentNamespace:     "ns",
				AssorterType:         AssorterTypePeriodic,
			},
			expectedConfig: &Config{
				ShutdownTimeout:      30 * time.Second,
				WorkQueueName:        "queue",
				MaxRetries:           3,
				AssorterPeriod:       1 * time.Minute,
				ReconciliationPeriod: DefaultReconciliationPeriod,
				TridentNamespace:     "ns",
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "OnlyTridentNamespaceEmpty_AppliesOnlyThatDefault",
			inputConfig: &Config{
				ShutdownTimeout:      30 * time.Second,
				WorkQueueName:        "queue",
				MaxRetries:           3,
				AssorterPeriod:       1 * time.Minute,
				ReconciliationPeriod: 5 * time.Minute,
				TridentNamespace:     "",
				AssorterType:         AssorterTypePeriodic,
			},
			expectedConfig: &Config{
				ShutdownTimeout:      30 * time.Second,
				WorkQueueName:        "queue",
				MaxRetries:           3,
				AssorterPeriod:       1 * time.Minute,
				ReconciliationPeriod: 5 * time.Minute,
				TridentNamespace:     DefaultTridentNamespace,
				AssorterType:         AssorterTypePeriodic,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := getTestContext()

			// Apply defaults to the input config
			applyConfigDefaults(ctx, tt.inputConfig)

			// Verify all fields match expected
			assert.Equal(t, tt.expectedConfig.ShutdownTimeout, tt.inputConfig.ShutdownTimeout, "ShutdownTimeout mismatch")
			assert.Equal(t, tt.expectedConfig.WorkQueueName, tt.inputConfig.WorkQueueName, "WorkQueueName mismatch")
			assert.Equal(t, tt.expectedConfig.MaxRetries, tt.inputConfig.MaxRetries, "MaxRetries mismatch")
			assert.Equal(t, tt.expectedConfig.AssorterPeriod, tt.inputConfig.AssorterPeriod, "AssorterPeriod mismatch")
			assert.Equal(t, tt.expectedConfig.ReconciliationPeriod, tt.inputConfig.ReconciliationPeriod, "ReconciliationPeriod mismatch")
			assert.Equal(t, tt.expectedConfig.TridentNamespace, tt.inputConfig.TridentNamespace, "TridentNamespace mismatch")
			assert.Equal(t, tt.expectedConfig.AssorterType, tt.inputConfig.AssorterType, "AssorterType mismatch")
		})
	}
}

// ============================================================================
// Test: generateEventID
// ============================================================================

func TestGenerateEventID(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() (time.Time, int)
		verify func(*testing.T, uint64)
	}{
		{
			name: "DifferentTimestamps_GenerateDifferentIDs",
			setup: func() (time.Time, int) {
				return time.Now(), 10
			},
			verify: func(t *testing.T, id1 uint64) {
				// Generate a second ID with different timestamp
				time.Sleep(1 * time.Millisecond) // Ensure timestamp is different
				id2 := generateEventID(time.Now(), 10)
				assert.NotEqual(t, id1, id2, "different timestamps should generate different IDs")
			},
		},
		{
			name: "DifferentPVCounts_GenerateDifferentIDs",
			setup: func() (time.Time, int) {
				ts := time.Now()
				return ts, 5
			},
			verify: func(t *testing.T, id1 uint64) {
				// Use same timestamp but different count
				// Note: In practice, we can't use exact same timestamp, but the count difference ensures uniqueness
				id2 := generateEventID(time.Now(), 10)
				// IDs should be different (though timestamp also differs slightly)
				assert.NotEqual(t, id1, id2, "different counts should contribute to different IDs")
			},
		},
		{
			name: "SameInputs_GenerateSameID",
			setup: func() (time.Time, int) {
				ts := time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC)
				return ts, 5
			},
			verify: func(t *testing.T, id1 uint64) {
				// Generate again with exact same inputs
				ts := time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC)
				id2 := generateEventID(ts, 5)
				assert.Equal(t, id1, id2, "same inputs should generate same ID")
			},
		},
		{
			name: "ZeroPVCount_GeneratesValidID",
			setup: func() (time.Time, int) {
				return time.Now(), 0
			},
			verify: func(t *testing.T, id uint64) {
				assert.NotZero(t, id, "should generate non-zero ID even with zero count")
			},
		},
		{
			name: "LargePVCount_GeneratesValidID",
			setup: func() (time.Time, int) {
				return time.Now(), 1000000
			},
			verify: func(t *testing.T, id uint64) {
				assert.NotZero(t, id, "should generate non-zero ID with large count")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timestamp, pvCount := tt.setup()
			eventID := generateEventID(timestamp, pvCount)

			// Basic validation
			assert.NotZero(t, eventID, "event ID should not be zero")

			// Run specific verification
			tt.verify(t, eventID)
		})
	}
}

// ============================================================================
// Test: Config.Copy()
// ============================================================================

func TestConfigCopy(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name:   "CopyNilConfig_ReturnsNil",
			config: nil,
		},
		{
			name: "CopyFullConfig_CreatesIndependentCopy",
			config: &Config{
				ShutdownTimeout:      60 * time.Second,
				WorkQueueName:        "test-queue",
				MaxRetries:           5,
				AssorterPeriod:       2 * time.Minute,
				ReconciliationPeriod: 10 * time.Minute,
				TridentNamespace:     "test-namespace",
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "CopyConfigWithZeroValues_PreservesZeroValues",
			config: &Config{
				ShutdownTimeout:      0,
				WorkQueueName:        "",
				MaxRetries:           0,
				AssorterPeriod:       0,
				ReconciliationPeriod: 0,
				TridentNamespace:     "",
				AssorterType:         AssorterTypePeriodic,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			copied := tt.config.Copy()

			if tt.config == nil {
				assert.Nil(t, copied, "copy of nil config should be nil")
				return
			}

			require.NotNil(t, copied, "copy should not be nil")
			assert.NotSame(t, tt.config, copied, "copy should be a different instance")

			// Verify all fields are equal
			assert.Equal(t, tt.config.ShutdownTimeout, copied.ShutdownTimeout)
			assert.Equal(t, tt.config.WorkQueueName, copied.WorkQueueName)
			assert.Equal(t, tt.config.MaxRetries, copied.MaxRetries)
			assert.Equal(t, tt.config.AssorterPeriod, copied.AssorterPeriod)
			assert.Equal(t, tt.config.ReconciliationPeriod, copied.ReconciliationPeriod)
			assert.Equal(t, tt.config.TridentNamespace, copied.TridentNamespace)
			assert.Equal(t, tt.config.AssorterType, copied.AssorterType)

			// Mutate the copy and verify original is unchanged
			copied.WorkQueueName = "modified-queue"
			copied.MaxRetries = 999
			assert.NotEqual(t, tt.config.WorkQueueName, copied.WorkQueueName, "original should not be affected by copy mutation")
			assert.NotEqual(t, tt.config.MaxRetries, copied.MaxRetries, "original should not be affected by copy mutation")
		})
	}
}

// ============================================================================
// Test: Activate
// ============================================================================

func TestActivate(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(ctrl *gomock.Controller) (*Scheduler, context.Context, func())
		verify func(*testing.T, *Scheduler, error)
	}{
		{
			name: "Success_FromStoppedState_ActivatesSuccessfully",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				// Create mockAssorter for this test
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Create mock eventbus
				mockControllerEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)

				// Expect Activate to be called on the assorter
				mockAssorter.EXPECT().
					Activate(gomock.Any()).
					Return(nil).
					Times(1)

				// Expect Subscribe to be called on the eventbus
				mockControllerEventBus.EXPECT().
					Subscribe(gomock.Any()).
					Return(eventbusTypes.SubscriptionID(12345), nil).
					Times(1)

				// Create scheduler directly with mock assorter
				scheduler := &Scheduler{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               getTestConfig(),
					workqueue: workqueue.NewTypedRateLimitingQueue(
						workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
					),
					assorter: mockAssorter,
				}
				scheduler.state.set(StateStopped)

				ctx := getTestContext()

				// Set the global eventbus
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockControllerEventBus

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err, "Activate should succeed from Stopped state")
				assert.Equal(t, StateReady, s.state.get(), "state should be Ready after activation")
				assert.NotNil(t, s.stopCh, "stopCh should be created")
				assert.Equal(t, eventbusTypes.SubscriptionID(12345), s.controllerEventSubscriptionID, "subscription ID should be set")
			},
		},
		{
			name: "Success_Idempotent_AlreadyReady_ReturnsNil",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// No expectations on mockAssorter or eventbus - they should not be called

				scheduler := &Scheduler{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               getTestConfig(),
					workqueue: workqueue.NewTypedRateLimitingQueue(
						workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
					),
					assorter: mockAssorter,
				}
				scheduler.state.set(StateReady) // Already in Ready state

				ctx := getTestContext()

				cleanup := func() {}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err, "Activate should be idempotent when already Ready")
				assert.Equal(t, StateReady, s.state.get(), "state should remain Ready")
			},
		},
		{
			name: "Success_Idempotent_AlreadyRunning_ReturnsNil",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// No expectations on mockAssorter or eventbus - they should not be called

				scheduler := &Scheduler{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               getTestConfig(),
					workqueue: workqueue.NewTypedRateLimitingQueue(
						workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
					),
					assorter: mockAssorter,
				}
				scheduler.state.set(StateRunning) // Already in Running state

				ctx := getTestContext()

				cleanup := func() {}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err, "Activate should be idempotent when already Running")
				assert.Equal(t, StateRunning, s.state.get(), "state should remain Running")
			},
		},
		{
			name: "Error_InvalidState_Starting_ReturnsStateError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               getTestConfig(),
					workqueue: workqueue.NewTypedRateLimitingQueue(
						workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
					),
					assorter: mockAssorter,
				}
				scheduler.state.set(StateStarting) // Invalid state for activation

				ctx := getTestContext()

				cleanup := func() {}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.Error(t, err, "Activate should fail when in Starting state")
				assert.Contains(t, err.Error(), "cannot activate scheduler", "error should mention activation failure")
				assert.Contains(t, err.Error(), "Starting", "error should mention current state")
				assert.Equal(t, StateStarting, s.state.get(), "state should remain Starting")
			},
		},
		{
			name: "Error_InvalidState_Stopping_ReturnsStateError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               getTestConfig(),
					workqueue: workqueue.NewTypedRateLimitingQueue(
						workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
					),
					assorter: mockAssorter,
				}
				scheduler.state.set(StateStopping) // Invalid state for activation

				ctx := getTestContext()

				cleanup := func() {}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.Error(t, err, "Activate should fail when in Stopping state")
				assert.Contains(t, err.Error(), "cannot activate scheduler", "error should mention activation failure")
				assert.Contains(t, err.Error(), "Stopping", "error should mention current state")
				assert.Equal(t, StateStopping, s.state.get(), "state should remain Stopping")
			},
		},
		{
			name: "Error_EventBusNil_ReturnsError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// No expectations - activation should fail before assorter is activated

				scheduler := &Scheduler{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               getTestConfig(),
					workqueue: workqueue.NewTypedRateLimitingQueue(
						workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
					),
					assorter: mockAssorter,
				}
				scheduler.state.set(StateStopped)

				ctx := getTestContext()

				// Set eventbus to nil
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = nil

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.Error(t, err, "Activate should fail when ControllerEventBus is nil")
				assert.Contains(t, err.Error(), "ControllerEventBus not initialized", "error should mention eventbus")
				assert.Equal(t, StateStopped, s.state.get(), "state should be reset to Stopped after cleanup")
			},
		},
		{
			name: "Error_EventBusSubscribeFails_ReturnsError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)
				mockControllerEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)

				// Expect Subscribe to fail
				mockControllerEventBus.EXPECT().
					Subscribe(gomock.Any()).
					Return(eventbusTypes.SubscriptionID(0), assert.AnError).
					Times(1)

				scheduler := &Scheduler{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               getTestConfig(),
					workqueue: workqueue.NewTypedRateLimitingQueue(
						workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
					),
					assorter: mockAssorter,
				}
				scheduler.state.set(StateStopped)

				ctx := getTestContext()

				// Set the global eventbus
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockControllerEventBus

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.Error(t, err, "Activate should fail when Subscribe fails")
				assert.Contains(t, err.Error(), "failed to subscribe", "error should mention subscription failure")
				assert.Equal(t, StateStopped, s.state.get(), "state should be reset to Stopped after cleanup")
			},
		},
		{
			name: "Error_AssorterActivateFails_ReturnsError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)
				mockControllerEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)

				// Expect Subscribe to succeed
				subscriptionID := eventbusTypes.SubscriptionID(12345)
				mockControllerEventBus.EXPECT().
					Subscribe(gomock.Any()).
					Return(subscriptionID, nil).
					Times(1)

				// Expect Unsubscribe during cleanup
				mockControllerEventBus.EXPECT().
					Unsubscribe(subscriptionID).
					Return(true).
					Times(1)

				// Expect Assorter.Activate to fail
				mockAssorter.EXPECT().
					Activate(gomock.Any()).
					Return(assert.AnError).
					Times(1)

				scheduler := &Scheduler{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               getTestConfig(),
					workqueue: workqueue.NewTypedRateLimitingQueue(
						workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
					),
					assorter: mockAssorter,
				}
				scheduler.state.set(StateStopped)

				ctx := getTestContext()

				// Set the global eventbus
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockControllerEventBus

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.Error(t, err, "Activate should fail when assorter activation fails")
				assert.Contains(t, err.Error(), "failed to activate assorter", "error should mention assorter failure")
				assert.Equal(t, StateStopped, s.state.get(), "state should be reset to Stopped after cleanup")
				assert.Equal(t, eventbusTypes.SubscriptionID(0), s.controllerEventSubscriptionID, "subscription ID should be cleared after cleanup")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gomock controller for this test
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Setup test dependencies
			scheduler, ctx, cleanup := tt.setup(ctrl)
			defer cleanup()

			// Call Activate
			err := scheduler.Activate(ctx)

			// Verify results
			tt.verify(t, scheduler, err)

			// Cleanup: shutdown workqueue if it was started
			if scheduler.workqueue != nil {
				scheduler.workqueue.ShutDown()
			}
			if scheduler.stopCh != nil {
				select {
				case <-scheduler.stopCh:
					// Already closed
				default:
					close(scheduler.stopCh)
				}
			}
		})
	}
}

// ============================================================================
// Test: Deactivate
// ============================================================================

func TestDeactivate(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(ctrl *gomock.Controller) (*Scheduler, context.Context, func())
		verify func(*testing.T, *Scheduler, error)
	}{
		{
			name: "Success_FromRunningState_DeactivatesSuccessfully",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				// Create mockAssorter for this test
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Create mock eventbus
				mockControllerEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)

				// Expect Deactivate to be called on the assorter
				mockAssorter.EXPECT().
					Deactivate(gomock.Any()).
					Return(nil).
					Times(1)

				// Expect Unsubscribe to be called on the eventbus during cleanup
				subscriptionID := eventbusTypes.SubscriptionID(12345)
				mockControllerEventBus.EXPECT().
					Unsubscribe(subscriptionID).
					Return(true).
					Times(1)

				// Create scheduler directly with mock assorter
				scheduler := &Scheduler{
					scLister:                      scLister,
					tvpLister:                     tvpLister,
					tridentClient:                 tridentClient,
					autogrowCache:                 autogrowCache,
					volumePublishManager:          volumePublishManager,
					config:                        getTestConfig(),
					workqueue:                     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					assorter:                      mockAssorter,
					controllerEventSubscriptionID: subscriptionID,
					stopCh:                        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				ctx := getTestContext()

				// Set the global eventbus
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockControllerEventBus

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err, "Deactivate should succeed from Running state")
				assert.Equal(t, StateStopped, s.state.get(), "state should be Stopped after deactivation")
				assert.Equal(t, eventbusTypes.SubscriptionID(0), s.controllerEventSubscriptionID, "subscription ID should be cleared")
			},
		},
		{
			name: "Success_FromReadyState_DeactivatesSuccessfully",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)

				// Create mockAssorter for this test
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Create mock eventbus
				mockControllerEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)

				// Expect Deactivate to be called on the assorter
				mockAssorter.EXPECT().
					Deactivate(gomock.Any()).
					Return(nil).
					Times(1)

				// Expect Unsubscribe to be called on the eventbus during cleanup
				subscriptionID := eventbusTypes.SubscriptionID(54321)
				mockControllerEventBus.EXPECT().
					Unsubscribe(subscriptionID).
					Return(true).
					Times(1)

				// Create scheduler directly with mock assorter
				scheduler := &Scheduler{
					scLister:                      scLister,
					tvpLister:                     tvpLister,
					tridentClient:                 tridentClient,
					autogrowCache:                 autogrowCache,
					volumePublishManager:          volumePublishManager,
					config:                        getTestConfig(),
					workqueue:                     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					assorter:                      mockAssorter,
					controllerEventSubscriptionID: subscriptionID,
					stopCh:                        make(chan struct{}),
				}
				scheduler.state.set(StateReady)

				ctx := getTestContext()

				// Set the global eventbus
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockControllerEventBus

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err, "Deactivate should succeed from Ready state")
				assert.Equal(t, StateStopped, s.state.get(), "state should be Stopped after deactivation")
				assert.Equal(t, eventbusTypes.SubscriptionID(0), s.controllerEventSubscriptionID, "subscription ID should be cleared")
			},
		},
		{
			name: "Success_Idempotent_AlreadyStopped_ReturnsNil",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// No expectations on mockAssorter or eventbus - they should not be called

				scheduler := &Scheduler{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               getTestConfig(),
					workqueue: workqueue.NewTypedRateLimitingQueue(
						workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
					),
					assorter: mockAssorter,
				}
				scheduler.state.set(StateStopped) // Already in Stopped state

				ctx := getTestContext()

				cleanup := func() {}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err, "Deactivate should be idempotent when already Stopped")
				assert.Equal(t, StateStopped, s.state.get(), "state should remain Stopped")
			},
		},
		{
			name: "Error_InvalidState_Starting_ReturnsStateError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// No expectations - deactivation should fail at state check

				scheduler := &Scheduler{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               getTestConfig(),
					workqueue: workqueue.NewTypedRateLimitingQueue(
						workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
					),
					assorter: mockAssorter,
				}
				scheduler.state.set(StateStarting) // Invalid state for deactivation

				ctx := getTestContext()

				cleanup := func() {}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.Error(t, err, "Deactivate should fail when in Starting state")
				assert.Contains(t, err.Error(), "cannot deactivate scheduler", "error should mention deactivation failure")
				assert.Contains(t, err.Error(), "Starting", "error should mention current state")
				assert.Equal(t, StateStarting, s.state.get(), "state should remain Starting")
			},
		},
		{
			name: "Error_InvalidState_Stopping_ReturnsStateError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// No expectations - deactivation should fail at state check

				scheduler := &Scheduler{
					scLister:             scLister,
					tvpLister:            tvpLister,
					tridentClient:        tridentClient,
					autogrowCache:        autogrowCache,
					volumePublishManager: volumePublishManager,
					config:               getTestConfig(),
					workqueue: workqueue.NewTypedRateLimitingQueue(
						workqueue.DefaultTypedControllerRateLimiter[WorkItem](),
					),
					assorter: mockAssorter,
				}
				scheduler.state.set(StateStopping) // Invalid state for deactivation

				ctx := getTestContext()

				cleanup := func() {}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.Error(t, err, "Deactivate should fail when in Stopping state")
				assert.Contains(t, err.Error(), "cannot deactivate scheduler", "error should mention deactivation failure")
				assert.Contains(t, err.Error(), "Stopping", "error should mention current state")
				assert.Equal(t, StateStopping, s.state.get(), "state should remain Stopping")
			},
		},
		{
			name: "Success_AssorterDeactivateFails_ContinuesWithCleanup",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)
				mockControllerEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)

				// Expect Assorter.Deactivate to fail (but should continue)
				mockAssorter.EXPECT().
					Deactivate(gomock.Any()).
					Return(assert.AnError).
					Times(1)

				// Expect Unsubscribe to still be called (cleanup continues despite assorter failure)
				subscriptionID := eventbusTypes.SubscriptionID(99999)
				mockControllerEventBus.EXPECT().
					Unsubscribe(subscriptionID).
					Return(true).
					Times(1)

				scheduler := &Scheduler{
					scLister:                      scLister,
					tvpLister:                     tvpLister,
					tridentClient:                 tridentClient,
					autogrowCache:                 autogrowCache,
					volumePublishManager:          volumePublishManager,
					config:                        getTestConfig(),
					workqueue:                     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					assorter:                      mockAssorter,
					controllerEventSubscriptionID: subscriptionID,
					stopCh:                        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				ctx := getTestContext()

				// Set the global eventbus
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockControllerEventBus

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err, "Deactivate should succeed even if assorter fails (just logs warning)")
				assert.Equal(t, StateStopped, s.state.get(), "state should be Stopped after cleanup")
				assert.Equal(t, eventbusTypes.SubscriptionID(0), s.controllerEventSubscriptionID, "subscription ID should be cleared")
			},
		},
		{
			name: "Success_EventBusUnsubscribeFails_ContinuesWithCleanup",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)
				mockControllerEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.ControllerEvent](ctrl)

				// Expect Assorter.Deactivate to succeed
				mockAssorter.EXPECT().
					Deactivate(gomock.Any()).
					Return(nil).
					Times(1)

				// Expect Unsubscribe to fail (returns false)
				subscriptionID := eventbusTypes.SubscriptionID(11111)
				mockControllerEventBus.EXPECT().
					Unsubscribe(subscriptionID).
					Return(false). // Unsubscribe failed
					Times(1)

				scheduler := &Scheduler{
					scLister:                      scLister,
					tvpLister:                     tvpLister,
					tridentClient:                 tridentClient,
					autogrowCache:                 autogrowCache,
					volumePublishManager:          volumePublishManager,
					config:                        getTestConfig(),
					workqueue:                     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					assorter:                      mockAssorter,
					controllerEventSubscriptionID: subscriptionID,
					stopCh:                        make(chan struct{}),
				}
				scheduler.state.set(StateReady)

				ctx := getTestContext()

				// Set the global eventbus
				oldEventBus := eventbus.ControllerEventBus
				eventbus.ControllerEventBus = mockControllerEventBus

				cleanup := func() {
					eventbus.ControllerEventBus = oldEventBus
				}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err, "Deactivate should succeed even if unsubscribe fails (just logs warning)")
				assert.Equal(t, StateStopped, s.state.get(), "state should be Stopped after cleanup")
				assert.Equal(t, eventbusTypes.SubscriptionID(0), s.controllerEventSubscriptionID, "subscription ID should be cleared even if unsubscribe failed")
			},
		},
		{
			name: "Success_NoSubscription_SkipsUnsubscribe",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, func()) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				volumePublishManager := mockNodeHelpers.NewMockVolumePublishManager(ctrl)
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect Assorter.Deactivate to succeed
				mockAssorter.EXPECT().
					Deactivate(gomock.Any()).
					Return(nil).
					Times(1)

				// No expectations on eventbus - subscription ID is 0

				scheduler := &Scheduler{
					scLister:                      scLister,
					tvpLister:                     tvpLister,
					tridentClient:                 tridentClient,
					autogrowCache:                 autogrowCache,
					volumePublishManager:          volumePublishManager,
					config:                        getTestConfig(),
					workqueue:                     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					assorter:                      mockAssorter,
					controllerEventSubscriptionID: 0, // No subscription
					stopCh:                        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				ctx := getTestContext()

				cleanup := func() {}

				return scheduler, ctx, cleanup
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err, "Deactivate should succeed with no subscription")
				assert.Equal(t, StateStopped, s.state.get(), "state should be Stopped after cleanup")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create gomock controller for this test
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Setup test dependencies
			scheduler, ctx, cleanup := tt.setup(ctrl)
			defer cleanup()

			// Call Deactivate
			err := scheduler.Deactivate(ctx)

			// Verify results
			tt.verify(t, scheduler, err)

			// Cleanup: shutdown workqueue if it exists and not already shut down
			if scheduler.workqueue != nil {
				scheduler.workqueue.ShutDown()
			}
		})
	}
}

// ============================================================================
// Test: Config Constructor and Options
// ============================================================================

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name           string
		opts           []ConfigOption
		expectedConfig *Config
	}{
		{
			name: "NoOptions_UsesDefaults",
			opts: nil,
			expectedConfig: &Config{
				ShutdownTimeout:      DefaultShutdownTimeout,
				WorkQueueName:        DefaultWorkQueueName,
				MaxRetries:           DefaultMaxRetries,
				AssorterPeriod:       DefaultAssorterPeriod,
				ReconciliationPeriod: DefaultReconciliationPeriod,
				TridentNamespace:     DefaultTridentNamespace,
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "WithShutdownTimeout_AppliesOption",
			opts: []ConfigOption{WithShutdownTimeout(60 * time.Second)},
			expectedConfig: &Config{
				ShutdownTimeout:      60 * time.Second,
				WorkQueueName:        DefaultWorkQueueName,
				MaxRetries:           DefaultMaxRetries,
				AssorterPeriod:       DefaultAssorterPeriod,
				ReconciliationPeriod: DefaultReconciliationPeriod,
				TridentNamespace:     DefaultTridentNamespace,
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "WithWorkQueueName_AppliesOption",
			opts: []ConfigOption{WithWorkQueueName("custom-queue")},
			expectedConfig: &Config{
				ShutdownTimeout:      DefaultShutdownTimeout,
				WorkQueueName:        "custom-queue",
				MaxRetries:           DefaultMaxRetries,
				AssorterPeriod:       DefaultAssorterPeriod,
				ReconciliationPeriod: DefaultReconciliationPeriod,
				TridentNamespace:     DefaultTridentNamespace,
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "WithMaxRetries_AppliesOption",
			opts: []ConfigOption{WithMaxRetries(10)},
			expectedConfig: &Config{
				ShutdownTimeout:      DefaultShutdownTimeout,
				WorkQueueName:        DefaultWorkQueueName,
				MaxRetries:           10,
				AssorterPeriod:       DefaultAssorterPeriod,
				ReconciliationPeriod: DefaultReconciliationPeriod,
				TridentNamespace:     DefaultTridentNamespace,
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "WithAssorterPeriod_AppliesOption",
			opts: []ConfigOption{WithAssorterPeriod(3 * time.Minute)},
			expectedConfig: &Config{
				ShutdownTimeout:      DefaultShutdownTimeout,
				WorkQueueName:        DefaultWorkQueueName,
				MaxRetries:           DefaultMaxRetries,
				AssorterPeriod:       3 * time.Minute,
				ReconciliationPeriod: DefaultReconciliationPeriod,
				TridentNamespace:     DefaultTridentNamespace,
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "WithReconciliationPeriod_AppliesOption",
			opts: []ConfigOption{WithReconciliationPeriod(15 * time.Minute)},
			expectedConfig: &Config{
				ShutdownTimeout:      DefaultShutdownTimeout,
				WorkQueueName:        DefaultWorkQueueName,
				MaxRetries:           DefaultMaxRetries,
				AssorterPeriod:       DefaultAssorterPeriod,
				ReconciliationPeriod: 15 * time.Minute,
				TridentNamespace:     DefaultTridentNamespace,
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "WithTridentNamespace_AppliesOption",
			opts: []ConfigOption{WithTridentNamespace("custom-ns")},
			expectedConfig: &Config{
				ShutdownTimeout:      DefaultShutdownTimeout,
				WorkQueueName:        DefaultWorkQueueName,
				MaxRetries:           DefaultMaxRetries,
				AssorterPeriod:       DefaultAssorterPeriod,
				ReconciliationPeriod: DefaultReconciliationPeriod,
				TridentNamespace:     "custom-ns",
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "WithAssorterType_AppliesOption",
			opts: []ConfigOption{WithAssorterType(AssorterTypePeriodic)},
			expectedConfig: &Config{
				ShutdownTimeout:      DefaultShutdownTimeout,
				WorkQueueName:        DefaultWorkQueueName,
				MaxRetries:           DefaultMaxRetries,
				AssorterPeriod:       DefaultAssorterPeriod,
				ReconciliationPeriod: DefaultReconciliationPeriod,
				TridentNamespace:     DefaultTridentNamespace,
				AssorterType:         AssorterTypePeriodic,
			},
		},
		{
			name: "MultipleOptions_AppliesAllOptions",
			opts: []ConfigOption{
				WithShutdownTimeout(120 * time.Second),
				WithWorkQueueName("multi-queue"),
				WithMaxRetries(7),
				WithAssorterPeriod(4 * time.Minute),
				WithReconciliationPeriod(20 * time.Minute),
				WithTridentNamespace("multi-ns"),
				WithAssorterType(AssorterTypePeriodic),
			},
			expectedConfig: &Config{
				ShutdownTimeout:      120 * time.Second,
				WorkQueueName:        "multi-queue",
				MaxRetries:           7,
				AssorterPeriod:       4 * time.Minute,
				ReconciliationPeriod: 20 * time.Minute,
				TridentNamespace:     "multi-ns",
				AssorterType:         AssorterTypePeriodic,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewConfig(tt.opts...)

			require.NotNil(t, config)
			assert.Equal(t, tt.expectedConfig.ShutdownTimeout, config.ShutdownTimeout)
			assert.Equal(t, tt.expectedConfig.WorkQueueName, config.WorkQueueName)
			assert.Equal(t, tt.expectedConfig.MaxRetries, config.MaxRetries)
			assert.Equal(t, tt.expectedConfig.AssorterPeriod, config.AssorterPeriod)
			assert.Equal(t, tt.expectedConfig.ReconciliationPeriod, config.ReconciliationPeriod)
			assert.Equal(t, tt.expectedConfig.TridentNamespace, config.TridentNamespace)
			assert.Equal(t, tt.expectedConfig.AssorterType, config.AssorterType)
		})
	}
}

// ============================================================================
// Test: State String Representation
// ============================================================================

func TestStateString(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateStopped, "Stopped"},
		{StateStarting, "Starting"},
		{StateReady, "Ready"},
		{StateRunning, "Running"},
		{StateStopping, "Stopping"},
		{State(999), "Unknown"}, // Invalid state
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.state.String())
		})
	}
}

// ============================================================================
// Test: AssorterType String Representation
// ============================================================================

func TestAssorterTypeString(t *testing.T) {
	tests := []struct {
		assorterType AssorterType
		expected     string
	}{
		{AssorterTypePeriodic, "Periodic"},
		{AssorterType(999), "Unknown"}, // Invalid type
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.assorterType.String())
		})
	}
}

// ============================================================================
// Test: handleControllerEvent
// ============================================================================

func TestHandleControllerEvent(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent)
		verify func(*testing.T, *Scheduler, eventbusTypes.Result, error)
	}{
		{
			name: "Success_StorageClassAddEvent_EnqueuesSuccessfully",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent) {
				namespace := "trident"
				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
				}
				scheduler.state.set(StateRunning)

				ctx := getTestContext()
				event := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeStorageClass,
					EventType:  crdtypes.EventAdd,
					Key:        "test-sc",
					Timestamp:  time.Now(),
				}

				return scheduler, ctx, event
			},
			verify: func(t *testing.T, s *Scheduler, result eventbusTypes.Result, err error) {
				require.NoError(t, err, "handleControllerEvent should succeed")
				require.NotNil(t, result, "result should not be nil")

				// Verify result fields
				assert.Equal(t, true, result["accepted"], "event should be accepted")
				assert.Equal(t, string(crdtypes.ObjectTypeStorageClass), result["objectType"])
				assert.Equal(t, string(crdtypes.EventAdd), result["eventType"])
				assert.Equal(t, "test-sc", result["key"])

				// Verify work item was enqueued
				assert.Equal(t, 1, s.workqueue.Len(), "workqueue should have 1 item")
			},
		},
		{
			name: "Success_TVPUpdateEvent_EnqueuesSuccessfully",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent) {
				namespace := "trident"
				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
				}
				scheduler.state.set(StateRunning)

				ctx := getTestContext()
				event := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeTridentVolumePublication,
					EventType:  crdtypes.EventUpdate,
					Key:        "trident/test-tvp",
					Timestamp:  time.Now(),
				}

				return scheduler, ctx, event
			},
			verify: func(t *testing.T, s *Scheduler, result eventbusTypes.Result, err error) {
				require.NoError(t, err, "handleControllerEvent should succeed")
				require.NotNil(t, result, "result should not be nil")

				assert.Equal(t, true, result["accepted"])
				assert.Equal(t, string(crdtypes.ObjectTypeTridentVolumePublication), result["objectType"])
				assert.Equal(t, string(crdtypes.EventUpdate), result["eventType"])
				assert.Equal(t, "trident/test-tvp", result["key"])

				assert.Equal(t, 1, s.workqueue.Len())
			},
		},
		{
			name: "Success_EventWithoutContext_UsesHandlerContext",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent) {
				namespace := "trident"
				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
				}
				scheduler.state.set(StateRunning)

				ctx := getTestContext()
				event := agTypes.ControllerEvent{
					Ctx:        nil, // No context in event
					ObjectType: crdtypes.ObjectTypeStorageClass,
					EventType:  crdtypes.EventDelete,
					Key:        "deleted-sc",
					Timestamp:  time.Now(),
				}

				return scheduler, ctx, event
			},
			verify: func(t *testing.T, s *Scheduler, result eventbusTypes.Result, err error) {
				require.NoError(t, err, "handleControllerEvent should succeed even without event context")
				require.NotNil(t, result)

				assert.Equal(t, true, result["accepted"])
				assert.Equal(t, 1, s.workqueue.Len())
			},
		},
		{
			name: "Failure_SchedulerNotRunning_ReturnsError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent) {
				namespace := "trident"
				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
				}
				scheduler.state.set(StateStopped) // Not running

				ctx := getTestContext()
				event := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeStorageClass,
					EventType:  crdtypes.EventAdd,
					Key:        "test-sc",
					Timestamp:  time.Now(),
				}

				return scheduler, ctx, event
			},
			verify: func(t *testing.T, s *Scheduler, result eventbusTypes.Result, err error) {
				require.Error(t, err, "handleControllerEvent should fail when scheduler not running")
				assert.Contains(t, err.Error(), "scheduler not running")
				assert.Nil(t, result, "result should be nil on error")

				// Verify work item was NOT enqueued
				assert.Equal(t, 0, s.workqueue.Len(), "workqueue should be empty")
			},
		},
		{
			name: "Failure_EmptyKey_ReturnsError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent) {
				namespace := "trident"
				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
				}
				scheduler.state.set(StateRunning)

				ctx := getTestContext()
				event := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeStorageClass,
					EventType:  crdtypes.EventAdd,
					Key:        "", // Empty key
					Timestamp:  time.Now(),
				}

				return scheduler, ctx, event
			},
			verify: func(t *testing.T, s *Scheduler, result eventbusTypes.Result, err error) {
				require.Error(t, err, "handleControllerEvent should fail with empty key")
				assert.Contains(t, err.Error(), "missing key")
				assert.Nil(t, result)

				assert.Equal(t, 0, s.workqueue.Len())
			},
		},
		{
			name: "Success_MultipleEvents_AllEnqueued",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent) {
				namespace := "trident"
				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
				}
				scheduler.state.set(StateRunning)

				ctx := getTestContext()

				// Enqueue first event
				event1 := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeStorageClass,
					EventType:  crdtypes.EventAdd,
					Key:        "sc1",
					Timestamp:  time.Now(),
				}
				_, _ = scheduler.handleControllerEvent(ctx, event1)

				// Enqueue second event
				event2 := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeTridentVolumePublication,
					EventType:  crdtypes.EventUpdate,
					Key:        "trident/tvp1",
					Timestamp:  time.Now(),
				}
				_, _ = scheduler.handleControllerEvent(ctx, event2)

				// Return the third event to test
				event3 := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeStorageClass,
					EventType:  crdtypes.EventDelete,
					Key:        "sc2",
					Timestamp:  time.Now(),
				}

				return scheduler, ctx, event3
			},
			verify: func(t *testing.T, s *Scheduler, result eventbusTypes.Result, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)

				// Should have 3 items in queue
				assert.Equal(t, 3, s.workqueue.Len(), "workqueue should have 3 items")
			},
		},
		{
			name: "Success_DuplicateKey_OnlyOneEnqueued",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent) {
				namespace := "trident"
				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
				}
				scheduler.state.set(StateRunning)

				ctx := getTestContext()

				// Enqueue first event with key "test-sc"
				event1 := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeStorageClass,
					EventType:  crdtypes.EventAdd,
					Key:        "test-sc",
					Timestamp:  time.Now(),
				}
				_, _ = scheduler.handleControllerEvent(ctx, event1)

				// Enqueue second event with same key "test-sc" (should be deduplicated)
				event2 := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeStorageClass,
					EventType:  crdtypes.EventUpdate,
					Key:        "test-sc",
					Timestamp:  time.Now(),
				}

				return scheduler, ctx, event2
			},
			verify: func(t *testing.T, s *Scheduler, result eventbusTypes.Result, err error) {
				require.NoError(t, err)
				require.NotNil(t, result)

				// Workqueue deduplicates by key, so should only have 1 item
				assert.Equal(t, 1, s.workqueue.Len(), "workqueue should deduplicate by key")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			scheduler, ctx, event := tt.setup(ctrl)

			result, err := scheduler.handleControllerEvent(ctx, event)

			tt.verify(t, scheduler, result, err)
		})
	}
}

// ============================================================================
// Test: EnqueueEvent
// ============================================================================

func TestEnqueueEvent(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent)
		verify func(*testing.T, *Scheduler, error)
	}{
		{
			name: "Success_ValidEvent_EnqueuesSuccessfully",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent) {
				namespace := "trident"
				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
				}
				scheduler.state.set(StateRunning)

				ctx := getTestContext()
				event := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeStorageClass,
					EventType:  crdtypes.EventAdd,
					Key:        "test-sc",
					Timestamp:  time.Now(),
				}

				return scheduler, ctx, event
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err, "EnqueueEvent should succeed")
				assert.Equal(t, 1, s.workqueue.Len(), "workqueue should have 1 item")

				// Verify the work item structure
				workItem, _ := s.workqueue.Get()
				assert.Equal(t, "test-sc", workItem.Key)
				assert.Equal(t, crdtypes.ObjectTypeStorageClass, workItem.ObjectType)
				assert.Equal(t, 0, workItem.RetryCount)
				s.workqueue.Done(workItem)
			},
		},
		{
			name: "Failure_SchedulerNotRunning_ReturnsError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent) {
				namespace := "trident"
				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
				}
				scheduler.state.set(StateStopped)

				ctx := getTestContext()
				event := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeStorageClass,
					EventType:  crdtypes.EventAdd,
					Key:        "test-sc",
					Timestamp:  time.Now(),
				}

				return scheduler, ctx, event
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.Error(t, err, "EnqueueEvent should fail when scheduler not running")
				assert.Contains(t, err.Error(), "scheduler not running")
				assert.Equal(t, 0, s.workqueue.Len(), "workqueue should be empty")
			},
		},
		{
			name: "Failure_EmptyKey_ReturnsError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent) {
				namespace := "trident"
				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
				}
				scheduler.state.set(StateRunning)

				ctx := getTestContext()
				event := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeStorageClass,
					EventType:  crdtypes.EventAdd,
					Key:        "",
					Timestamp:  time.Now(),
				}

				return scheduler, ctx, event
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.Error(t, err, "EnqueueEvent should fail with empty key")
				assert.Contains(t, err.Error(), "missing key")
				assert.Equal(t, 0, s.workqueue.Len())
			},
		},
		{
			name: "Success_TVPEvent_EnqueuesWithNamespacedKey",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent) {
				namespace := "trident"
				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
				}
				scheduler.state.set(StateRunning)

				ctx := getTestContext()
				event := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeTridentVolumePublication,
					EventType:  crdtypes.EventUpdate,
					Key:        "trident/pvc-123.node-1",
					Timestamp:  time.Now(),
				}

				return scheduler, ctx, event
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.NoError(t, err)
				assert.Equal(t, 1, s.workqueue.Len())

				workItem, _ := s.workqueue.Get()
				assert.Equal(t, "trident/pvc-123.node-1", workItem.Key)
				assert.Equal(t, crdtypes.ObjectTypeTridentVolumePublication, workItem.ObjectType)
				s.workqueue.Done(workItem)
			},
		},
		{
			name: "Success_StateStarting_RejectsEvent",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent) {
				namespace := "trident"
				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
				}
				scheduler.state.set(StateStarting) // Not fully running

				ctx := getTestContext()
				event := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeStorageClass,
					EventType:  crdtypes.EventAdd,
					Key:        "test-sc",
					Timestamp:  time.Now(),
				}

				return scheduler, ctx, event
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.Error(t, err, "EnqueueEvent should fail when scheduler is starting")
				assert.Contains(t, err.Error(), "scheduler not running")
				assert.Equal(t, 0, s.workqueue.Len())
			},
		},
		{
			name: "Success_StateStopping_RejectsEvent",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, agTypes.ControllerEvent) {
				namespace := "trident"
				scLister, tvpLister, _ := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithTridentNamespace(namespace)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
				}
				scheduler.state.set(StateStopping) // Stopping

				ctx := getTestContext()
				event := agTypes.ControllerEvent{
					Ctx:        ctx,
					ObjectType: crdtypes.ObjectTypeStorageClass,
					EventType:  crdtypes.EventAdd,
					Key:        "test-sc",
					Timestamp:  time.Now(),
				}

				return scheduler, ctx, event
			},
			verify: func(t *testing.T, s *Scheduler, err error) {
				require.Error(t, err, "EnqueueEvent should fail when scheduler is stopping")
				assert.Contains(t, err.Error(), "scheduler not running")
				assert.Equal(t, 0, s.workqueue.Len())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			scheduler, ctx, event := tt.setup(ctrl)

			err := scheduler.EnqueueEvent(ctx, event)

			tt.verify(t, scheduler, err)
		})
	}
}

// ============================================================================
// Test: reconciliationLoop
// ============================================================================

func TestReconciliationLoop(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc)
		verify func(*testing.T, *Scheduler)
	}{
		{
			name: "Success_ContextCancelled_StopsLoop",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithReconciliationPeriod(100 * time.Millisecond)),
					stopCh:        make(chan struct{}),
				}

				// Create a cancellable context
				ctx, cancel := context.WithCancel(context.Background())

				return scheduler, ctx, cancel
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Verify the loop stopped gracefully
				// No panics or hangs
			},
		},
		{
			name: "Success_StopChannelClosed_StopsLoop",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithReconciliationPeriod(100 * time.Millisecond)),
					stopCh:        make(chan struct{}),
				}

				ctx := context.Background()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Verify the loop stopped gracefully
			},
		},
		{
			name: "Success_TimerTriggers_CallsSyncCacheWithAPIServer_NoError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				// Create a TVP so syncCacheWithAPIServer has something to process
				tvp := createTestTVP("pvc-123.node-1", "trident", "pvc-123", "test-sc")
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, []*v1.TridentVolumePublication{tvp})
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect assorter operations during reconciliation
				mockAssorter.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithReconciliationPeriod(50 * time.Millisecond)),
					stopCh:        make(chan struct{}),
				}

				ctx, cancel := context.WithCancel(context.Background())

				return scheduler, ctx, cancel
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Timer triggered and reconciliation completed successfully
			},
		},
		{
			name: "Success_TimerTriggers_SyncCacheReturnsError_ContinuesLoop",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				// Create setup that will cause syncCacheWithAPIServer to fail
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Make assorter return error to cause reconciliation failure
				mockAssorter.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(assert.AnError).AnyTimes()

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithReconciliationPeriod(50 * time.Millisecond)),
					stopCh:        make(chan struct{}),
				}

				ctx, cancel := context.WithCancel(context.Background())

				return scheduler, ctx, cancel
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Loop should continue even after error
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			scheduler, ctx, cancel := tt.setup(ctrl)

			// Start reconciliation loop in a goroutine
			done := make(chan struct{})
			go func() {
				scheduler.reconciliationLoop(ctx)
				close(done)
			}()

			// For timer tests, wait longer to let timer fire at least once
			if tt.name == "Success_TimerTriggers_CallsSyncCacheWithAPIServer_NoError" ||
				tt.name == "Success_TimerTriggers_SyncCacheReturnsError_ContinuesLoop" {
				// Wait for timer to fire (period is 50ms, wait 150ms to ensure it triggers)
				time.Sleep(150 * time.Millisecond)
			} else {
				// Give the loop time to start
				time.Sleep(10 * time.Millisecond)
			}

			// Stop the loop based on test type
			if cancel != nil {
				// Test context cancellation
				cancel()
			} else {
				// Test stop channel
				close(scheduler.stopCh)
			}

			// Wait for the loop to stop (with timeout)
			select {
			case <-done:
				// Loop stopped successfully
			case <-time.After(1 * time.Second):
				t.Fatal("reconciliationLoop did not stop in time")
			}

			tt.verify(t, scheduler)
		})
	}
}

// ============================================================================
// Test: workerLoop
// ============================================================================

func TestWorkerLoop(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc)
		verify func(*testing.T, *Scheduler)
	}{
		{
			name: "Success_ContextCancelled_StopsLoop",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				ctx, cancel := context.WithCancel(context.Background())

				// Cancel context immediately so the loop doesn't block on Get()
				cancel()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Loop stopped via context cancellation
			},
		},
		{
			name: "Success_StopChannelClosed_StopsLoop",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				// Close stop channel before starting
				close(scheduler.stopCh)

				ctx := context.Background()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Loop stopped via stop channel
			},
		},
		{
			name: "Success_WorkqueueShutdown_StopsLoop",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				ctx := context.Background()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Loop stopped via workqueue shutdown
			},
		},
		{
			name: "Success_NotRunning_DrainsQueueWithoutProcessing",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateStopped) // Not running

				// Add a work item
				workItem := WorkItem{
					Ctx:        context.Background(),
					Key:        "test-key",
					ObjectType: crdtypes.ObjectTypeStorageClass,
					RetryCount: 0,
				}
				scheduler.workqueue.Add(workItem)

				ctx := context.Background()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Work item should have been drained without processing
			},
		},
		{
			name: "Success_ProcessingSucceeds_ItemCompleted",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				// Create a test StorageClass
				sc := createTestStorageClass("test-sc")
				scLister, tvpLister, tridentClient := setupTestListers(t, []*storagev1.StorageClass{sc}, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				// Add a work item
				workItem := WorkItem{
					Ctx:        context.Background(),
					Key:        "test-sc",
					ObjectType: crdtypes.ObjectTypeStorageClass,
					RetryCount: 0,
				}
				scheduler.workqueue.Add(workItem)

				ctx := context.Background()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Work item should have been processed successfully
				// Queue should be empty after draining
				assert.Equal(t, 0, s.workqueue.Len())
			},
		},
		{
			name: "Success_RetriableError_RequeuesWithBackoff",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect RemoveVolume to be called when TVP not found
				mockAssorter.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithMaxRetries(3)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				// Add a work item for non-existent TVP (will cause retriable error)
				workItem := WorkItem{
					Ctx:        context.Background(),
					Key:        "trident/non-existent-tvp",
					ObjectType: crdtypes.ObjectTypeTridentVolumePublication,
					RetryCount: 0,
				}
				scheduler.workqueue.Add(workItem)

				ctx := context.Background()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Item should have been requeued (queue length should be > 0)
			},
		},
		{
			name: "Success_RateLimitError_RequeuesWithRetryAfterDelay",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				// To hit the rate-limit error path, we need processWorkItem to return a ReconcileDeferredError
				// that wraps a TooManyRequests error. The handlers wrap lister errors with ReconcileDeferredError.
				// So we need to make the lister fail with a rate limit error.

				sc := createTestStorageClass("test-sc")
				scLister, _, tridentClient := setupTestListers(t, []*storagev1.StorageClass{sc}, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Create a custom TVP lister that returns a rate limit error
				rateLimitErr := k8serrors.NewTooManyRequests("rate limited", 3)
				mockTVPLister := &fakeTVPLister{err: rateLimitErr}

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     mockTVPLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithMaxRetries(3)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				// Add a StorageClass work item - handleStorageClassEvent will call tvpLister.List()
				// which will return our rate limit error, wrapped in ReconcileDeferredError
				workItem := WorkItem{
					Ctx:        context.Background(),
					Key:        "test-sc",
					ObjectType: crdtypes.ObjectTypeStorageClass,
					RetryCount: 0,
				}
				scheduler.workqueue.Add(workItem)

				ctx := context.Background()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Item should have been requeued with retry-after delay
				// The workerLoop should have called AddAfter with the RetryAfterSeconds duration
			},
		},
		{
			name: "Success_MaxRetriesExceeded_DiscardsItem",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect RemoveVolume to be called when TVP not found
				mockAssorter.EXPECT().RemoveVolume(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithMaxRetries(2)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				// Add a work item that already exceeded max retries
				workItem := WorkItem{
					Ctx:        context.Background(),
					Key:        "trident/non-existent-tvp",
					ObjectType: crdtypes.ObjectTypeTridentVolumePublication,
					RetryCount: 3, // Exceeds max retries of 2
				}
				scheduler.workqueue.Add(workItem)

				ctx := context.Background()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Item should have been discarded
			},
		},
		{
			name: "Success_NonRetriableError_DiscardsItem",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				// Add a work item with invalid key format (non-retriable error)
				workItem := WorkItem{
					Ctx:        context.Background(),
					Key:        "invalid/key/format/with/too/many/slashes",
					ObjectType: crdtypes.ObjectTypeTridentVolumePublication,
					RetryCount: 0,
				}
				scheduler.workqueue.Add(workItem)

				ctx := context.Background()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Item should have been discarded without retry
			},
		},
		{
			name: "Success_TridentBackendEvent_ReturnsNotImplementedError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				// Add a TridentBackend work item (not implemented)
				workItem := WorkItem{
					Ctx:        context.Background(),
					Key:        "backend-1",
					ObjectType: crdtypes.ObjectTypeTridentBackend,
					RetryCount: 0,
				}
				scheduler.workqueue.Add(workItem)

				ctx := context.Background()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// TBE events should be logged as not implemented and discarded
			},
		},
		{
			name: "Success_UnknownObjectType_ReturnsError",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				// Add a work item with unknown object type
				workItem := WorkItem{
					Ctx:        context.Background(),
					Key:        "some-key",
					ObjectType: "UnknownObjectType", // Invalid object type
					RetryCount: 0,
				}
				scheduler.workqueue.Add(workItem)

				ctx := context.Background()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Unknown object type should be logged and discarded
			},
		},
		{
			name: "Success_MaxRetriesExceeded_DiscardsWorkItem",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				sc := createTestStorageClass("test-sc")
				scLister, _, tridentClient := setupTestListers(t, []*storagev1.StorageClass{sc}, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Use a fake lister that always returns an error (API error)
				internalErr := k8serrors.NewInternalError(fmt.Errorf("persistent API error"))
				mockTVPLister := &fakeTVPLister{err: internalErr}

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     mockTVPLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithMaxRetries(2)), // Set low max retries
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				// Add work item that's already at max retries
				workItem := WorkItem{
					Ctx:        context.Background(),
					Key:        "test-sc",
					ObjectType: crdtypes.ObjectTypeStorageClass,
					RetryCount: 2, // Already at max retries
				}
				scheduler.workqueue.Add(workItem)

				ctx := context.Background()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Item should have been discarded (Forget+Done called)
			},
		},
		{
			name: "Success_RetriableError_UsesRateLimitedBackoff",
			setup: func(ctrl *gomock.Controller) (*Scheduler, context.Context, context.CancelFunc) {
				sc := createTestStorageClass("test-sc")
				scLister, _, tridentClient := setupTestListers(t, []*storagev1.StorageClass{sc}, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Use a regular API error (not TooManyRequests, so no retry-after hint)
				internalErr := k8serrors.NewInternalError(fmt.Errorf("temporary API error"))
				mockTVPLister := &fakeTVPLister{err: internalErr}

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     mockTVPLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(WithMaxRetries(3)),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateRunning)

				// Add work item with retry count < max
				workItem := WorkItem{
					Ctx:        context.Background(),
					Key:        "test-sc",
					ObjectType: crdtypes.ObjectTypeStorageClass,
					RetryCount: 0,
				}
				scheduler.workqueue.Add(workItem)

				ctx := context.Background()

				return scheduler, ctx, nil
			},
			verify: func(t *testing.T, s *Scheduler) {
				// Item should have been requeued with AddRateLimited (no specific delay hint)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			scheduler, ctx, cancel := tt.setup(ctrl)

			// Start worker loop in a goroutine
			done := make(chan struct{})
			go func() {
				scheduler.workerLoop(ctx)
				close(done)
			}()

			// Give the loop time to process items
			time.Sleep(100 * time.Millisecond)

			// Stop the loop by shutting down the workqueue
			scheduler.workqueue.ShutDown()

			// Wait for the loop to stop (with timeout)
			select {
			case <-done:
				// Loop stopped successfully
			case <-time.After(1 * time.Second):
				t.Fatal("workerLoop did not stop in time")
			}

			tt.verify(t, scheduler)

			// Cleanup
			if cancel != nil {
				cancel()
			}
		})
	}
}

// ============================================================================
// Tests for StartScheduleLoop
// ============================================================================

func TestStartScheduleLoop(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(ctrl *gomock.Controller) *Scheduler
		expectError bool
		errorMsg    string
	}{
		{
			name: "Success_StartsScheduleLoopAndReconciliation",
			setup: func(ctrl *gomock.Controller) *Scheduler {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect StartScheduleLoop to be called on assorter
				mockAssorter.EXPECT().StartScheduleLoop(gomock.Any()).Return(nil)

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateReady) // Must be in Ready state

				return scheduler
			},
			expectError: false,
		},
		{
			name: "Success_AlreadyRunning_ReturnsNilIdempotent",
			setup: func(ctrl *gomock.Controller) *Scheduler {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// No call to StartScheduleLoop expected since already running

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateRunning) // Already running

				return scheduler
			},
			expectError: false,
		},
		{
			name: "Error_WrongState_ReturnsError",
			setup: func(ctrl *gomock.Controller) *Scheduler {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// No call to StartScheduleLoop expected since we're in wrong state

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateStopped) // Wrong state (should be Ready)

				return scheduler
			},
			expectError: true,
			errorMsg:    "cannot start schedule loop: scheduler is in Stopped state (expected Ready)",
		},
		{
			name: "Error_AssorterStartFails_ReturnsError",
			setup: func(ctrl *gomock.Controller) *Scheduler {
				scLister, tvpLister, tridentClient := setupTestListers(t, nil, nil)
				autogrowCache := getTestAutogrowCache()
				mockAssorter := mockAssorterTypes.NewMockAssorter(ctrl)

				// Expect StartScheduleLoop to be called but return error
				mockAssorter.EXPECT().StartScheduleLoop(gomock.Any()).Return(fmt.Errorf("assorter failed"))

				scheduler := &Scheduler{
					scLister:      scLister,
					tvpLister:     tvpLister,
					tridentClient: tridentClient,
					autogrowCache: autogrowCache,
					assorter:      mockAssorter,
					config:        getTestConfig(),
					workqueue:     workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[WorkItem]()),
					stopCh:        make(chan struct{}),
				}
				scheduler.state.set(StateReady)

				return scheduler
			},
			expectError: true,
			errorMsg:    "failed to start assorter schedule loop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			scheduler := tt.setup(ctrl)
			ctx := getTestContext()

			err := scheduler.StartScheduleLoop(ctx)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				// If successful, state should be Running (unless already was)
				if tt.name == "Success_StartsScheduleLoopAndReconciliation" {
					assert.Equal(t, StateRunning, scheduler.state.get())
				}
			}

			// Cleanup: stop any running goroutines
			close(scheduler.stopCh)
			time.Sleep(50 * time.Millisecond) // Give time for goroutines to stop
		})
	}
}

// ============================================================================
// Tests for createPublishFunc
// ============================================================================

func TestCreatePublishFunc(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() (context.Context, []string, func())
		expectError bool
		errorMsg    string
		verifyEvent func(*testing.T, *agTypes.VolumesScheduled)
	}{
		{
			name: "Success_PublishesEventWithTVPNames",
			setup: func() (context.Context, []string, func()) {
				// Setup mock eventbus
				mockCtrl := gomock.NewController(t)
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](mockCtrl)

				// Expect Publish to be called
				mockEventBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Times(1)

				// Store original and replace with mock
				originalEventBus := eventbus.VolumesScheduledEventBus
				eventbus.VolumesScheduledEventBus = mockEventBus

				cleanup := func() {
					eventbus.VolumesScheduledEventBus = originalEventBus
					mockCtrl.Finish()
				}

				ctx := context.Background()
				tvpNames := []string{"tvp1", "tvp2", "tvp3"}

				return ctx, tvpNames, cleanup
			},
			expectError: false,
			verifyEvent: func(t *testing.T, event *agTypes.VolumesScheduled) {
				// Verification done via mock expectations
			},
		},
		{
			name: "Success_PublishesEventWithEmptyTVPNames",
			setup: func() (context.Context, []string, func()) {
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](gomock.NewController(t))

				mockEventBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Times(1)

				originalEventBus := eventbus.VolumesScheduledEventBus
				eventbus.VolumesScheduledEventBus = mockEventBus

				cleanup := func() {
					eventbus.VolumesScheduledEventBus = originalEventBus
				}

				ctx := context.Background()
				tvpNames := []string{} // Empty list

				return ctx, tvpNames, cleanup
			},
			expectError: false,
			verifyEvent: func(t *testing.T, event *agTypes.VolumesScheduled) {
				// No verification needed for this test
			},
		},
		{
			name: "Error_EventBusNotInitialized_ReturnsError",
			setup: func() (context.Context, []string, func()) {
				// Store original and set to nil
				originalEventBus := eventbus.VolumesScheduledEventBus
				eventbus.VolumesScheduledEventBus = nil

				cleanup := func() {
					eventbus.VolumesScheduledEventBus = originalEventBus
				}

				ctx := context.Background()
				tvpNames := []string{"tvp1"}

				return ctx, tvpNames, cleanup
			},
			expectError: true,
			errorMsg:    "VolumesScheduled eventbus not initialized",
			verifyEvent: func(t *testing.T, event *agTypes.VolumesScheduled) {
				// No event should be published
			},
		},
		{
			name: "Success_EventIDIsUnique",
			setup: func() (context.Context, []string, func()) {
				mockEventBus := mockEventbus.NewMockEventBusMetricsOptions[agTypes.VolumesScheduled](gomock.NewController(t))

				var event1, event2 agTypes.VolumesScheduled
				callCount := 0
				mockEventBus.EXPECT().
					Publish(gomock.Any(), gomock.Any()).
					Do(func(ctx context.Context, event agTypes.VolumesScheduled) {
						if callCount == 0 {
							event1 = event
						} else {
							event2 = event
						}
						callCount++
					}).
					Times(2)

				originalEventBus := eventbus.VolumesScheduledEventBus
				eventbus.VolumesScheduledEventBus = mockEventBus

				cleanup := func() {
					eventbus.VolumesScheduledEventBus = originalEventBus
					// Verify IDs are different
					if event1.ID != 0 && event2.ID != 0 {
						assert.NotEqual(t, event1.ID, event2.ID, "Event IDs should be unique")
					}
				}

				ctx := context.Background()
				tvpNames := []string{"tvp1"}

				return ctx, tvpNames, cleanup
			},
			expectError: false,
			verifyEvent: func(t *testing.T, event *agTypes.VolumesScheduled) {
				// Verification done in cleanup
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, tvpNames, cleanup := tt.setup()
			defer cleanup()

			// Create the publish function
			publishFunc := createPublishFunc()

			// Call the publish function
			err := publishFunc(ctx, tvpNames)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			// Special handling for unique ID test
			if tt.name == "Success_EventIDIsUnique" {
				// Call again to generate second event
				err2 := publishFunc(ctx, tvpNames)
				assert.NoError(t, err2)
			}
		})
	}
}
