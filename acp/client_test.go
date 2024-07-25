// Copyright 2023 NetApp, Inc. All Rights Reserved.

package acp

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	mock_acp "github.com/netapp/trident/mocks/mock_acp/mock_rest"
	"github.com/netapp/trident/utils/errors"
	"github.com/netapp/trident/utils/version"
)

var ctx = context.Background()

// setupBackoff is a helper to set the backoff values used in the API client's GetVersionWithBackoff method.
func setupBackoff(interval, intervalCeiling, timeCeiling time.Duration, timeMultiplier, randomization float64) {
	initialInterval = interval
	maxInterval = intervalCeiling
	maxElapsedTime = timeCeiling
	multiplier = timeMultiplier
	randomFactor = randomization
}

func TestTridentACP_GetVersionWithBackoff(t *testing.T) {
	t.Run("WithNoServerRunning", func(t *testing.T) {
		// Reset the backoff to the initial values after the test exits.
		defer setupBackoff(initialInterval, maxInterval, maxElapsedTime, multiplier, randomFactor)
		setupBackoff(50*time.Millisecond, 100*time.Millisecond, 250*time.Millisecond, 1.414, 1.0)

		mockCtrl := gomock.NewController(t)
		mockRest := mock_acp.NewMockREST(mockCtrl)
		mockRest.EXPECT().GetVersion(ctx).Return(nil, fmt.Errorf("no server; status 500")).AnyTimes()

		client := newClient(mockRest, true)
		v, err := client.GetVersionWithBackoff(ctx)
		assert.Error(t, err, "expected error")
		assert.Nil(t, v, "expected nil version")
	})

	t.Run("WithACPNotEnabled", func(t *testing.T) {
		// Reset the backoff to the initial values after the test exits.
		defer setupBackoff(initialInterval, maxInterval, maxElapsedTime, multiplier, randomFactor)
		setupBackoff(50*time.Millisecond, 100*time.Millisecond, 250*time.Millisecond, 1.414, 1.0)

		client := newClient(nil, false)
		v, err := client.GetVersionWithBackoff(ctx)
		assert.True(t, errors.IsUnsupportedError(err), "unexpected error")
		assert.Nil(t, v, "expected nil version")
	})

	t.Run("WithCorrectResponseTypeAsync", func(t *testing.T) {
		// Reset the backoff to the initial values after the test exits.
		defer setupBackoff(initialInterval, maxInterval, maxElapsedTime, multiplier, randomFactor)
		setupBackoff(50*time.Millisecond, 100*time.Millisecond, 250*time.Millisecond, 1.414, 1.0)

		expectedVersion := version.MustParseDate("23.07.0")
		mockCtrl := gomock.NewController(t)
		mockRest := mock_acp.NewMockREST(mockCtrl)
		mockRest.EXPECT().GetVersion(ctx).Return(expectedVersion, nil).AnyTimes()

		client := newClient(mockRest, true)

		var wg sync.WaitGroup

		var v *version.Version
		var err error
		func() {
			wg.Add(1)
			defer wg.Done()
			v, err = client.GetVersionWithBackoff(ctx)
			// Give the backoff-retry time to make the API calls.
			time.Sleep(200 * time.Millisecond)
		}()

		wg.Wait()
		// For now expect no error even though one occurs.
		assert.NoError(t, err, "unexpected error")
		assert.NotNil(t, v, "unexpected nil version")
		assert.Equal(t, expectedVersion.String(), v.String(), "expected equal versions")
	})
}

func TestTridentACP_IsFeatureEnabled(t *testing.T) {
	t.Run("WithACPNotEnabled", func(t *testing.T) {
		// Reset the package-level state after the test completes.
		client := newClient(nil, false)
		err := client.IsFeatureEnabled(ctx, FeatureSnapshotRestore)
		assert.NoError(t, err, "expected no error")
	})
}
