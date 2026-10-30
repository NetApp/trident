package kubernetes

// Concurrent map-write repro for helper.AddPublishedPath.
//
// Customer signature (concurrent map writes on AddPublishedPath):
//
//	fatal error: concurrent map writes
//	github.com/netapp/trident/frontend/csi/node_helpers/kubernetes.(*helper).AddPublishedPath
//
// NodePublishVolume locks per volumeID, so the SAME volume is serialised.
// DIFFERENT volumes publish in parallel and all execute the unlocked write:
//
//	h.publishedPaths[volumeID] = volTrackingInfo.PublishedPaths
//
// This test fans out AddPublishedPath across many distinct volumeIDs on one
// helper. It is expected to panic with "concurrent map writes" on an unfixed
// build. Run:
//
//	go test -count=1 -timeout 60s ./frontend/csi/node_helpers/kubernetes \
//	  -run TestAddPublishedPath_ConcurrentMapWritePanic
//
// Do NOT run under -race if you want the native panic; -race may abort earlier
// with a race report instead of "fatal error: concurrent map writes".

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mockNodeHelpers "github.com/netapp/trident/mocks/mock_frontend/mock_csi/mock_node_helpers"
	"github.com/netapp/trident/utils/models"
)

func TestAddPublishedPath_ConcurrentMapWritePanic(t *testing.T) {
	mockCtrl := gomock.NewController(t)
	mockVPM := mockNodeHelpers.NewMockVolumePublishManager(mockCtrl)

	const n = 64
	const iters = 200

	// Each ReadTrackingInfo returns a FRESH PublishedPaths map, matching the
	// production path (JSON unmarshal into a new struct). The race is on the
	// helper's shared publishedPaths outer map, not on a shared inner map.
	mockVPM.EXPECT().
		ReadTrackingInfo(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, volumeID string) (*models.VolumeTrackingInfo, error) {
			return &models.VolumeTrackingInfo{
				PublishedPaths: map[string]struct{}{},
			}, nil
		}).AnyTimes()
	mockVPM.EXPECT().
		WriteTrackingInfo(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).AnyTimes()

	h := &helper{
		VolumePublishManager: mockVPM,
		publishedPaths:       make(map[string]map[string]struct{}),
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(id int) {
			defer wg.Done()
			vol := fmt.Sprintf("pvc-%08d-aaaa-bbbb-cccc-dddddddddddd", id)
			for j := 0; j < iters; j++ {
				path := fmt.Sprintf("/var/lib/kubelet/plugins/kubernetes.io/csi/volumeDevices/publish/%s/%d", vol, j)
				// On an unfixed build this eventually panics the process with
				// "fatal error: concurrent map writes" inside AddPublishedPath.
				_ = h.AddPublishedPath(ctx, vol, path)
			}
		}(i)
	}
	wg.Wait()

	// If we get here the race did not fire this run — retry or raise n/iters.
	// The test still "passes" only when the bug is absent (fixed build).
	require.Greater(t, len(h.publishedPaths), 0)
}
