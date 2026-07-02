// Copyright 2026 NetApp, Inc. All Rights Reserved.

package core

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/afero"
	"golang.org/x/time/rate"

	"go.uber.org/mock/gomock"

	"github.com/netapp/trident/core/cache"
	"github.com/netapp/trident/frontend"
	mock_mount "github.com/netapp/trident/mocks/mock_utils/mock_mount"
	persistentstore "github.com/netapp/trident/persistent_store"
	"github.com/netapp/trident/storage"
	storageclass "github.com/netapp/trident/storage_class"
	"github.com/netapp/trident/utils/devices"
	"github.com/netapp/trident/utils/errors"
	tridentexec "github.com/netapp/trident/utils/exec"
	"github.com/netapp/trident/utils/fcp"
	"github.com/netapp/trident/utils/filesystem"
	"github.com/netapp/trident/utils/iscsi"
	"github.com/netapp/trident/utils/mount"
	"github.com/netapp/trident/utils/osutils"
)

// isExpectedMountInitError identifies the known Windows CSI-proxy-unavailable path
// where unit tests should use mock fallbacks instead of external services.
func isExpectedMountInitError(err error) bool {
	if err == nil {
		return false
	}

	errString := strings.ToLower(err.Error())
	knownMountInitSubstrings := []string{
		"csi proxy",
		"host API process is not initialized",
		"create csi proxy client",
		"open \\\\.\\pipe\\csi-proxy",
	}

	for _, substring := range knownMountInitSubstrings {
		if strings.Contains(errString, strings.ToLower(substring)) {
			return true
		}
	}

	return false
}

func newTestISCSIClient() *iscsi.Client {
	client, err := iscsi.New()
	if err == nil {
		return client
	}
	return iscsi.NewDetailed(
		"", tridentexec.NewCommand(), iscsi.DefaultSelfHealingExclusion, osutils.New(), devices.New(),
		filesystem.New(nil), nil, iscsi.NewReconcileUtils(), afero.Afero{Fs: afero.NewMemMapFs()}, osutils.New(),
	)
}

func newWindowsFallbackMountMock(t *testing.T) mount.Mount {
	t.Helper()

	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	m := mock_mount.NewMockMount(ctrl)
	// Hybrid approach: provide only a minimal baseline for common non-asserted calls.
	// Tests that depend on specific mount behavior should set expectations explicitly.
	m.EXPECT().GetSelfMountInfo(gomock.Any()).Return(nil, nil).AnyTimes()
	m.EXPECT().GetHostMountInfo(gomock.Any()).Return(nil, nil).AnyTimes()
	m.EXPECT().IsMounted(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).AnyTimes()
	m.EXPECT().IsLikelyNotMountPoint(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	m.EXPECT().PVMountpointMappings(gomock.Any()).Return(map[string]string{}, nil).AnyTimes()
	return m
}

func newTestMountClient(t *testing.T) mount.Mount {
	t.Helper()

	// TODO: Update this helper to always use a mock mount client in the future,
	// so core unit tests have no host-service dependencies.
	client, err := mount.New()
	if err == nil {
		return client
	}

	if runtime.GOOS == "windows" && isExpectedMountInitError(err) {
		return newWindowsFallbackMountMock(t)
	}

	panic(fmt.Sprintf("mount client unavailable in test setup: %v", err))
}

func newTestFCPClient() *fcp.Client {
	client, err := fcp.New()
	if err == nil {
		return client
	}
	return fcp.NewDetailed(
		"", tridentexec.NewCommand(), fcp.DefaultSelfHealingExclusion, osutils.New(),
		devices.New(), filesystem.New(nil), nil,
		fcp.NewReconcileUtils("", osutils.New()), afero.Afero{Fs: afero.NewMemMapFs()},
	)
}

func newTestTridentOrchestrator(t *testing.T, client persistentstore.Client) *TridentOrchestrator {
	t.Helper()

	orchestrator, err := NewTridentOrchestrator(client)
	if err == nil {
		return orchestrator
	}

	if !(runtime.GOOS == "windows" && isExpectedMountInitError(err)) {
		t.Fatalf("failed to create test Trident orchestrator: %v", err)
	}

	// On Windows CI, CSI proxy may be unavailable. Fall back to test doubles so core unit tests remain isolated.
	return newWindowsFallbackOrchestrator(t, client)
}

func newWindowsFallbackOrchestrator(t *testing.T, client persistentstore.Client) *TridentOrchestrator {
	t.Helper()

	mountClient := newTestMountClient(t)
	if vpSyncRateLimiter == nil {
		vpSyncRateLimiter = rate.NewLimiter(vpUpdateRateLimit, vpUpdateBurst)
	}

	return &TridentOrchestrator{
		backends:           make(map[string]storage.Backend),
		volumes:            make(map[string]*storage.Volume),
		subordinateVolumes: make(map[string]*storage.Volume),
		frontends:          make(map[string]frontend.Plugin),
		storageClasses:     make(map[string]*storageclass.StorageClass),
		nodes:              *cache.NewNodeCache(),
		volumePublications: cache.NewVolumePublicationCache(),
		snapshots:          make(map[string]*storage.Snapshot),
		groupSnapshots:     make(map[string]*storage.GroupSnapshot),
		autogrowPolicies:   make(map[string]*storage.AutogrowPolicy),
		mutex:              &sync.Mutex{},
		storeClient:        client,
		bootstrapped:       false,
		bootstrapError:     errors.NotReadyError(),
		iscsi:              newTestISCSIClient(),
		fcp:                newTestFCPClient(),
		mount:              mountClient,
		fs:                 filesystem.New(mountClient),
	}
}
