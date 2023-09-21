// Copyright 2023 NetApp, Inc. All Rights Reserved.

package acp

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// setupBackoff is a helper to set the backoff values used in the API client's Activate method.
func setupBackoff(interval, intervalCeiling, timeCeiling time.Duration, timeMultiplier, randomization float64) {
	initialInterval = interval
	maxInterval = intervalCeiling
	maxElapsedTime = timeCeiling
	multiplier = timeMultiplier
	randomFactor = randomization
}

func TestClient_Activate(t *testing.T) {
	t.Run("WithNoServerRunning", func(t *testing.T) {
		// Reset the backoff to the initial values after the test exits.
		defer setupBackoff(initialInterval, maxInterval, maxElapsedTime, multiplier, randomFactor)
		setupBackoff(50*time.Millisecond, 100*time.Millisecond, 250*time.Millisecond, 1.414, 1.0)

		server := newHttpTestServer(versionEndpoint, func(w http.ResponseWriter, request *http.Request) {})
		// Close the server immediately.
		server.Close()

		client := &Client{baseURL: server.URL, httpClient: *server.Client()}
		err := client.Activate()
		// For now expect no error even though one occurs.
		assert.Error(t, err, "expected error")
	})

	t.Run("WithCorrectResponseTypeAsync", func(t *testing.T) {
		// Reset the backoff to the initial values after the test exits.
		defer setupBackoff(initialInterval, maxInterval, maxElapsedTime, multiplier, randomFactor)
		setupBackoff(50*time.Millisecond, 100*time.Millisecond, 250*time.Millisecond, 1.414, 1.0)

		handler := func(w http.ResponseWriter, request *http.Request) {
			bytes, err := json.Marshal(&getVersionResponse{Version: "23.10.0-custom+unknown"})
			if err != nil {
				t.Fatalf("failed to create fake response for GetVersion")
			}
			if _, err = w.Write(bytes); err != nil {
				t.Fatalf("failed to write response for GetVersion")
			}
		}
		server := newHttpTestServer(versionEndpoint, handler)
		defer server.Close()

		client := &Client{baseURL: server.URL, httpClient: http.Client{Timeout: httpClientTimeout}}

		var wg sync.WaitGroup
		var err error
		func() {
			wg.Add(1)
			defer wg.Done()
			err = client.Activate()
			// Give the backoff-retry time to make the API calls.
			time.Sleep(200 * time.Millisecond)
		}()

		wg.Wait()
		assert.NoError(t, err, "unexpected error")
	})
}

func TestClient_Deactivate(t *testing.T) {
	client := &Client{baseURL: "", httpClient: http.Client{}}
	err := client.Deactivate()
	assert.NoError(t, err)
}

func TestClient_GetName(t *testing.T) {
	client := &Client{baseURL: "", httpClient: http.Client{}}
	name := client.GetName()
	assert.Equal(t, appName, name)
}

func TestClient_Version(t *testing.T) {
	client := &Client{baseURL: "", httpClient: http.Client{}}
	name := client.Version()
	assert.Equal(t, apiVersion, name)
}
