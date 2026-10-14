// Copyright 2026 NetApp, Inc. All Rights Reserved.

package cosi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netapp/trident/config"
	"github.com/netapp/trident/cosi/provisioner"
	"github.com/netapp/trident/cosi/provisioner/fake"
	"github.com/netapp/trident/frontend"
)

func TestNewFrontendRequiresProvisionerAndRESTConfig(t *testing.T) {
	_, err := NewFrontend(nil, dummyRESTConfig())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provisioner is required")

	_, err = NewFrontend(fake.New(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rest config is required")

	created, err := NewFrontend(fake.New(), dummyRESTConfig())
	require.NoError(t, err)
	require.NotNil(t, created)
}

func TestFrontendPluginIdentity(t *testing.T) {
	f, err := NewFrontend(fake.New(), dummyRESTConfig())
	require.NoError(t, err)

	var plugin frontend.Plugin = f
	assert.Equal(t, "cosi", plugin.GetName())
	assert.Equal(t, config.OrchestratorAPIVersion, plugin.Version())
}

func TestActivateRejectsASecondCall(t *testing.T) {
	f, err := NewFrontend(fake.New(), dummyRESTConfig())
	require.NoError(t, err)

	// Pretend Activate already succeeded. The guard is the first thing Activate
	// checks, so this does not need a cluster.
	f.stopped = make(chan struct{})

	err = f.Activate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already active")
}

func TestDeactivateIsANoOpWhenInactive(t *testing.T) {
	f, err := NewFrontend(fake.New(), dummyRESTConfig())
	require.NoError(t, err)

	assert.NoError(t, f.Deactivate())
	assert.NoError(t, f.Deactivate(), "Deactivate must be idempotent")
}

// TestDeactivateStopsTheRunContext is the lifecycle Deactivate actually implements:
// cancel the manager's context and wait for its goroutine to exit. The manager itself
// is not involved; the channel handshake is.
func TestDeactivateStopsTheRunContext(t *testing.T) {
	f, err := NewFrontend(fake.New(), dummyRESTConfig())
	require.NoError(t, err)

	runCtx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-runCtx.Done()
	}()

	f.mutex.Lock()
	f.cancel = cancel
	f.stopped = stopped
	f.mutex.Unlock()

	require.NoError(t, f.Deactivate())

	assert.Nil(t, f.cancel)
	assert.Nil(t, f.stopped)
	assert.NoError(t, f.Deactivate(), "a second Deactivate after a successful stop is a no-op")
}

func TestDeactivateTimesOutWhenTheManagerIsWedged(t *testing.T) {
	originalTimeout, originalSlack := shutdownTimeout, deactivateWaitSlack
	shutdownTimeout = 10 * time.Millisecond
	deactivateWaitSlack = 10 * time.Millisecond
	t.Cleanup(func() {
		shutdownTimeout = originalTimeout
		deactivateWaitSlack = originalSlack
	})

	f, err := NewFrontend(fake.New(), dummyRESTConfig())
	require.NoError(t, err)

	// Never close stopped, and ignore the cancel: this is a reconcile that will not
	// finish. Deactivate must return rather than block Trident's shutdown.
	f.mutex.Lock()
	f.cancel = func() {}
	f.stopped = make(chan struct{})
	f.mutex.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		assert.NoError(t, f.Deactivate())
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Deactivate blocked past the shutdown timeout")
	}

	assert.Nil(t, f.stopped)
}

func TestActivateRefusesAClusterWithoutTheCRDs(t *testing.T) {
	f, err := NewFrontend(fake.New(), discoveryServer(t, "v1.36.0", false))
	require.NoError(t, err)

	err = f.Activate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "install the COSI CRDs")
	assert.NoError(t, f.Deactivate(), "a refused Activate must leave nothing running")
}

func TestActivateRefusesAClusterBelowTheCOSIFloor(t *testing.T) {
	f, err := NewFrontend(fake.New(), discoveryServer(t, "v1.31.0", true))
	require.NoError(t, err)

	err = f.Activate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "COSI requires Kubernetes")
	assert.NoError(t, f.Deactivate())
}

func TestActivateFailsWhenTheProvisionerCannotIdentifyItself(t *testing.T) {
	backend := fake.New()
	backend.Errors["DriverInfo"] = errors.New("backend unreachable")

	f, err := NewFrontend(backend, discoveryServer(t, "v1.36.0", true))
	require.NoError(t, err)

	err = f.Activate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query provisioner driver info")
	assert.NoError(t, f.Deactivate())
}

func TestActivateFailsWhenTheProvisionerReportsAnUnusableName(t *testing.T) {
	backend := fake.New()
	backend.SupportedProtocols = []provisioner.Protocol{}

	f, err := NewFrontend(backend, discoveryServer(t, "v1.36.0", true))
	require.NoError(t, err)

	err = f.Activate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unusable driver info")
	assert.NoError(t, f.Deactivate())
}

func TestNewDriverInfoFromTheFakeProvisioner(t *testing.T) {
	f, err := NewFrontend(fake.New(), dummyRESTConfig())
	require.NoError(t, err)

	info, err := f.newDriverInfo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, fake.DriverName, info.Name)
	assert.NotEmpty(t, info.SupportedProtocols)
}
