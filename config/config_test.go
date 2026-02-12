// Copyright 2021 NetApp, Inc. All Rights Reserved.

package config

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	k8sversion "k8s.io/apimachinery/pkg/version"

	versionutils "github.com/netapp/trident/utils/version"
)

func TestMain(m *testing.M) {
	// Disable any standard log output; don't import our logging package here or else an import cycle will occur.
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

func TestIsValidProtocol(t *testing.T) {
	validProtocols := []Protocol{
		File,
		Block,
		ProtocolAny,
	}

	for _, protocol := range validProtocols {
		isValidProtocol := IsValidProtocol(protocol)
		assert.True(t, isValidProtocol, "expected valid protocol")
	}

	invalidProtocols := []Protocol{
		"Object",
		"!@#$%^&*()",
	}

	for _, protocol := range invalidProtocols {
		isValidProtocol := IsValidProtocol(protocol)
		assert.False(t, isValidProtocol, "expected invalid protocol")
	}
}

func TestGetValidProtocolNames(t *testing.T) {
	// will need updated if or when we support new protocols
	expectedProtocols := map[string]bool{
		"file":  true,
		"block": true,
		"":      true, // ProtocolAny
	}

	for _, protocol := range GetValidProtocolNames() {
		_, ok := expectedProtocols[protocol]
		assert.True(t, ok, "expected valid protocol names only")
	}
}

func TestPlatformAtLeast(t *testing.T) {
	tests := []struct {
		platformName string
		version      string
		causedErr    bool
	}{
		{platformName: "docker", version: "v1.6.0", causedErr: true},
		{platformName: "kubernetes", version: "v1.7.0", causedErr: true},
		{platformName: "kubernetes", version: "v1.6.0", causedErr: true},
		{platformName: "kubernetes", version: "v1.9.0", causedErr: true},
		{platformName: "kubernetes", version: "v1.12.0", causedErr: true},
		{platformName: "kubernetes", version: "x123", causedErr: false},
	}

	OrchestratorTelemetry.Platform = "kubernetes"
	OrchestratorTelemetry.PlatformVersion = "v1.9.0"
	for _, test := range tests {
		err := PlatformAtLeast(test.platformName, test.version)
		isErr := err != nil
		if isErr == test.causedErr {
			t.Errorf("Failed platform test. %s %s result: %v", test.platformName, test.version, test.causedErr)
		}
	}
}

func TestVersion(t *testing.T) {
	// reset package level variables to their default value
	defer func() {
		BuildType = "custom"
	}()

	BuildType = "stable"
	actualVersion := version()
	expectedVersion := DefaultOrchestratorVersion
	assert.Equal(t, actualVersion, expectedVersion, "expected equal versions")

	BuildType = "custom"
	actualVersion = version()
	expectedVersion = fmt.Sprintf("%v-%v+%v", DefaultOrchestratorVersion, BuildType, BuildHash)
	assert.Equal(t, actualVersion, expectedVersion, "expected equal versions")

	BuildType = "not-custom-or-stable"
	actualVersion = version()
	expectedVersion = fmt.Sprintf("%v-%v.%v+%v", DefaultOrchestratorVersion, BuildType, BuildTypeRev, BuildHash)
	assert.Equal(t, actualVersion, expectedVersion, "expected equal versions")
}

func TestValidateKubernetesVersion(t *testing.T) {
	minK8sVersion := KubernetesVersionMin
	currentK8sVersion, _ := versionutils.ParseGeneric(KubernetesVersionMax)
	err := ValidateKubernetesVersion(minK8sVersion, currentK8sVersion)
	assert.Nil(t, err, "expected nil error")

	minK8sVersion = KubernetesVersionMin
	currentK8sVersion, _ = versionutils.ParseGeneric("v1.18") // any version older than KubernetesVersionMin may be used.
	err = ValidateKubernetesVersion(minK8sVersion, currentK8sVersion)
	assert.NotNil(t, err, "expected non-nil error")
}

func TestValidateKubernetesVersionFromInfo(t *testing.T) {
	// happy path
	k8sMinVersion := KubernetesVersionMin
	k8sCurrentVersionParts := strings.Split(KubernetesVersionMax, ".") // v1.24 -> [v1, 24]
	currentK8sVersionInfo := &k8sversion.Info{
		Major:      k8sCurrentVersionParts[0],
		Minor:      k8sCurrentVersionParts[1],
		GitVersion: strings.Join(append(k8sCurrentVersionParts, "10"), "."),
	}
	err := ValidateKubernetesVersionFromInfo(k8sMinVersion, currentK8sVersionInfo)
	assert.NoError(t, err, "expected no error")

	// mismatched versions
	k8sMinVersion = KubernetesVersionMax
	k8sCurrentVersionParts = strings.Split(KubernetesVersionMin, ".") // v1.21 -> [v1, 21]
	currentK8sVersionInfo = &k8sversion.Info{
		Major:      k8sCurrentVersionParts[0],
		Minor:      k8sCurrentVersionParts[1],
		GitVersion: strings.Join(append(k8sCurrentVersionParts, "10"), "."),
	}
	err = ValidateKubernetesVersionFromInfo(k8sMinVersion, currentK8sVersionInfo)
	assert.Error(t, err, "expected no error")

	// improperly formatted version
	k8sMinVersion = KubernetesVersionMax
	k8sCurrentVersionParts = strings.Split(KubernetesVersionMin, ".") // v1.21 -> [v1, 21]
	currentK8sVersionInfo = &k8sversion.Info{
		Major:      k8sCurrentVersionParts[0],
		Minor:      k8sCurrentVersionParts[1],
		GitVersion: "v21.07.0",
	}
	err = ValidateKubernetesVersionFromInfo(k8sMinVersion, currentK8sVersionInfo)
	assert.Error(t, err, "expected no error")
}

func TestIsValidContainerName(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		expected      bool
	}{
		// Valid controller containers
		{
			name:          "Valid trident-main",
			containerName: "trident-main",
			expected:      true,
		},
		{
			name:          "Valid csi-provisioner",
			containerName: "csi-provisioner",
			expected:      true,
		},
		{
			name:          "Valid csi-resizer",
			containerName: "csi-resizer",
			expected:      true,
		},
		{
			name:          "Valid csi-snapshotter",
			containerName: "csi-snapshotter",
			expected:      true,
		},
		{
			name:          "Valid csi-attacher",
			containerName: "csi-attacher",
			expected:      true,
		},
		{
			name:          "Valid trident-autosupport",
			containerName: "trident-autosupport",
			expected:      true,
		},
		// Valid node containers
		{
			name:          "Valid node driver-registrar",
			containerName: "node-driver-registrar",
			expected:      true,
		},
		{
			name:          "Valid windows driver-registrar",
			containerName: "node-driver-registrar",
			expected:      true,
		},
		{
			name:          "Valid windows liveness probe",
			containerName: "liveness-probe",
			expected:      true,
		},
		// Case sensitivity tests (should NOT match - exact match required)
		{
			name:          "Uppercase TRIDENT-MAIN",
			containerName: "TRIDENT-MAIN",
			expected:      false,
		},
		{
			name:          "Mixed case Csi-Provisioner",
			containerName: "Csi-Provisioner",
			expected:      false,
		},
		{
			name:          "Mixed case CSI-Attacher",
			containerName: "CSI-Attacher",
			expected:      false,
		},
		// Whitespace tests (should NOT match - exact match required)
		{
			name:          "Leading whitespace",
			containerName: "  trident-main",
			expected:      false,
		},
		{
			name:          "Trailing whitespace",
			containerName: "csi-provisioner  ",
			expected:      false,
		},
		{
			name:          "Both leading and trailing whitespace",
			containerName: "  driver-registrar  ",
			expected:      false,
		},
		{
			name:          "Tab characters",
			containerName: "\ttrident-autosupport\t",
			expected:      false,
		},
		// Combined case and whitespace
		{
			name:          "Uppercase with whitespace",
			containerName: "  TRIDENT-MAIN  ",
			expected:      false,
		},
		// Invalid containers
		{
			name:          "Invalid container name",
			containerName: "invalid-container",
			expected:      false,
		},
		{
			name:          "Empty string",
			containerName: "",
			expected:      false,
		},
		{
			name:          "Only whitespace",
			containerName: "   ",
			expected:      false,
		},
		{
			name:          "Typo in name - trident-mian",
			containerName: "trident-mian",
			expected:      false,
		},
		{
			name:          "Typo in name - csi-provisoner",
			containerName: "csi-provisoner",
			expected:      false,
		},
		{
			name:          "Similar but wrong - trident-node",
			containerName: "trident-node",
			expected:      false,
		},
		{
			name:          "Random string",
			containerName: "random-container-name",
			expected:      false,
		},
		{
			name:          "Special characters",
			containerName: "trident@main",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsValidContainerName(tt.containerName)
			assert.Equal(t, tt.expected, result, "IsValidContainerName(%q) = %v, expected %v", tt.containerName, result, tt.expected)
		})
	}
}

func TestIsValidControllerContainerName(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		expected      bool
	}{
		// Valid controller containers
		{
			name:          "Valid controller trident-main",
			containerName: "trident-main",
			expected:      true,
		},
		{
			name:          "Valid controller csi-provisioner",
			containerName: "csi-provisioner",
			expected:      true,
		},
		{
			name:          "Valid controller csi-resizer",
			containerName: "csi-resizer",
			expected:      true,
		},
		{
			name:          "Valid controller csi-snapshotter",
			containerName: "csi-snapshotter",
			expected:      true,
		},
		{
			name:          "Valid controller csi-attacher",
			containerName: "csi-attacher",
			expected:      true,
		},
		{
			name:          "Valid controller trident-autosupport",
			containerName: "trident-autosupport",
			expected:      true,
		},
		// Case sensitivity tests (should NOT match - exact match required)
		{
			name:          "Uppercase controller TRIDENT-MAIN",
			containerName: "TRIDENT-MAIN",
			expected:      false,
		},
		{
			name:          "Mixed case controller CSI-Provisioner",
			containerName: "CSI-Provisioner",
			expected:      false,
		},
		// Whitespace tests (should NOT match - exact match required)
		{
			name:          "Controller with leading whitespace",
			containerName: "  trident-main",
			expected:      false,
		},
		{
			name:          "Controller with trailing whitespace",
			containerName: "csi-attacher  ",
			expected:      false,
		},
		{
			name:          "Controller with both whitespace",
			containerName: "  trident-autosupport  ",
			expected:      false,
		},
		// Node containers (should return false)
		{
			name:          "Node container driver-registrar",
			containerName: "driver-registrar",
			expected:      false,
		},
		{
			name:          "Node container driver-registrar uppercase",
			containerName: "DRIVER-REGISTRAR",
			expected:      false,
		},
		// Invalid containers
		{
			name:          "Invalid container",
			containerName: "invalid-container",
			expected:      false,
		},
		{
			name:          "Empty string",
			containerName: "",
			expected:      false,
		},
		{
			name:          "Only whitespace",
			containerName: "   ",
			expected:      false,
		},
		{
			name:          "Typo - csi-provisoner",
			containerName: "csi-provisoner",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsValidControllerContainerName(tt.containerName)
			assert.Equal(t, tt.expected, result, "IsValidControllerContainerName(%q) = %v, expected %v", tt.containerName, result, tt.expected)
		})
	}
}

func TestIsValidLinuxNodeContainerName(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		expected      bool
	}{
		// Valid node containers
		{
			name:          "Valid node trident-main",
			containerName: "trident-main",
			expected:      true,
		},
		{
			name:          "Valid node driver-registrar",
			containerName: "node-driver-registrar",
			expected:      true,
		},
		// Case sensitivity tests (should NOT match - exact match required)
		{
			name:          "Uppercase node TRIDENT-MAIN",
			containerName: "TRIDENT-MAIN",
			expected:      false,
		},
		{
			name:          "Mixed case node Driver-Registrar",
			containerName: "Driver-Registrar",
			expected:      false,
		},
		{
			name:          "Uppercase DRIVER-REGISTRAR",
			containerName: "DRIVER-REGISTRAR",
			expected:      false,
		},
		// Whitespace tests (should NOT match - exact match required)
		{
			name:          "Node with leading whitespace",
			containerName: "  trident-main",
			expected:      false,
		},
		{
			name:          "Node with trailing whitespace",
			containerName: "driver-registrar  ",
			expected:      false,
		},
		{
			name:          "Node with both whitespace",
			containerName: "  trident-main  ",
			expected:      false,
		},
		{
			name:          "Node with tabs",
			containerName: "\tdriver-registrar\t",
			expected:      false,
		},
		// Controller-only containers (should return false)
		{
			name:          "Controller-only csi-provisioner",
			containerName: "csi-provisioner",
			expected:      false,
		},
		{
			name:          "Controller-only csi-attacher",
			containerName: "csi-attacher",
			expected:      false,
		},
		{
			name:          "Controller-only csi-resizer",
			containerName: "csi-resizer",
			expected:      false,
		},
		{
			name:          "Controller-only csi-snapshotter",
			containerName: "csi-snapshotter",
			expected:      false,
		},
		{
			name:          "Controller-only trident-autosupport",
			containerName: "trident-autosupport",
			expected:      false,
		},
		{
			name:          "Controller-only uppercase CSI-PROVISIONER",
			containerName: "CSI-PROVISIONER",
			expected:      false,
		},
		// Windows node pod extra container (should return false)
		{
			name:          "Windows Node Pod container",
			containerName: "livenessprobe",
			expected:      false,
		},
		// Invalid containers
		{
			name:          "Invalid container",
			containerName: "invalid-container",
			expected:      false,
		},
		{
			name:          "Empty string",
			containerName: "",
			expected:      false,
		},
		{
			name:          "Only whitespace",
			containerName: "   ",
			expected:      false,
		},
		{
			name:          "Typo - driver-regitrar",
			containerName: "driver-regitrar",
			expected:      false,
		},
		{
			name:          "Random container",
			containerName: "some-random-container",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsValidLinuxNodeContainerName(tt.containerName)
			assert.Equal(t, tt.expected, result, "IsValidNodeContainerName(%q) = %v, expected %v", tt.containerName, result, tt.expected)
		})
	}
}

func TestIsValidWindowsNodeContainerName(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		expected      bool
	}{
		// Valid node containers
		{
			name:          "Valid node trident-main",
			containerName: "trident-main",
			expected:      true,
		},
		{
			name:          "Valid node driver-registrar",
			containerName: "node-driver-registrar",
			expected:      true,
		},
		{
			name:          "Valid node livenessprobe",
			containerName: "liveness-probe",
			expected:      true,
		},
		// Case sensitivity tests (should NOT match - exact match required)
		{
			name:          "Uppercase node TRIDENT-MAIN",
			containerName: "TRIDENT-MAIN",
			expected:      false,
		},
		{
			name:          "Mixed case node Driver-Registrar",
			containerName: "Driver-Registrar",
			expected:      false,
		},
		{
			name:          "Uppercase DRIVER-REGISTRAR",
			containerName: "DRIVER-REGISTRAR",
			expected:      false,
		},
		// Whitespace tests (should NOT match - exact match required)
		{
			name:          "Node with leading whitespace",
			containerName: "  trident-main",
			expected:      false,
		},
		{
			name:          "Node with trailing whitespace",
			containerName: "driver-registrar  ",
			expected:      false,
		},
		{
			name:          "Node with both whitespace",
			containerName: "  trident-main  ",
			expected:      false,
		},
		{
			name:          "Node with tabs",
			containerName: "\tdriver-registrar\t",
			expected:      false,
		},
		// Controller-only containers (should return false)
		{
			name:          "Controller-only csi-provisioner",
			containerName: "csi-provisioner",
			expected:      false,
		},
		{
			name:          "Controller-only csi-attacher",
			containerName: "csi-attacher",
			expected:      false,
		},
		{
			name:          "Controller-only csi-resizer",
			containerName: "csi-resizer",
			expected:      false,
		},
		{
			name:          "Controller-only csi-snapshotter",
			containerName: "csi-snapshotter",
			expected:      false,
		},
		{
			name:          "Controller-only trident-autosupport",
			containerName: "trident-autosupport",
			expected:      false,
		},
		{
			name:          "Controller-only uppercase CSI-PROVISIONER",
			containerName: "CSI-PROVISIONER",
			expected:      false,
		},
		// Invalid containers
		{
			name:          "Invalid container",
			containerName: "invalid-container",
			expected:      false,
		},
		{
			name:          "Empty string",
			containerName: "",
			expected:      false,
		},
		{
			name:          "Only whitespace",
			containerName: "   ",
			expected:      false,
		},
		{
			name:          "Typo - driver-regitrar",
			containerName: "driver-regitrar",
			expected:      false,
		},
		{
			name:          "Random container",
			containerName: "some-random-container",
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsValidWindowsNodeContainerName(tt.containerName)
			assert.Equal(t, tt.expected, result, "IsValidNodeContainerName(%q) = %v, expected %v", tt.containerName, result, tt.expected)
		})
	}
}

func TestRegisterTelemetryUpdater(t *testing.T) {
	// Reset global state before test
	defer func() {
		telemetryUpdaterMutex.Lock()
		dynamicTelemetryUpdater = nil
		cachedTelemetryData = struct {
			PlatformUID           string
			PlatformNodeCount     int
			PlatformVersion       string
			TridentProtectVersion string
		}{}
		telemetryUpdaterMutex.Unlock()
	}()

	ctx := context.Background()

	// Test 1: Register a valid updater
	called := false
	validUpdater := func(ctx context.Context, telemetry *Telemetry) {
		called = true
		telemetry.Platform = "test-platform"
	}

	RegisterTelemetryUpdater(validUpdater)

	// Verify registration state
	telemetryUpdaterMutex.RLock()
	assert.NotNil(t, dynamicTelemetryUpdater, "telemetry updater should be registered")
	telemetryUpdaterMutex.RUnlock()

	// Test the updater works
	testTelemetry := &Telemetry{}
	telemetryUpdaterMutex.RLock()
	dynamicTelemetryUpdater(ctx, testTelemetry)
	telemetryUpdaterMutex.RUnlock()
	assert.True(t, called, "updater should have been called")
	assert.Equal(t, "test-platform", testTelemetry.Platform, "updater should have modified telemetry")

	// Test 2: Register nil updater (should be ignored)
	RegisterTelemetryUpdater(nil)

	telemetryUpdaterMutex.RLock()
	assert.NotNil(t, dynamicTelemetryUpdater, "original updater should still be registered after nil")
	telemetryUpdaterMutex.RUnlock()

	// Test 3: Register second updater (should be ignored due to single registration limit)
	secondCalled := false
	secondUpdater := func(ctx context.Context, telemetry *Telemetry) {
		secondCalled = true
	}

	RegisterTelemetryUpdater(secondUpdater)

	telemetryUpdaterMutex.RLock()
	assert.NotNil(t, dynamicTelemetryUpdater, "updater should still be registered")
	telemetryUpdaterMutex.RUnlock()

	// Verify only first updater is registered
	testTelemetry2 := &Telemetry{}
	called = false
	telemetryUpdaterMutex.RLock()
	dynamicTelemetryUpdater(ctx, testTelemetry2)
	telemetryUpdaterMutex.RUnlock()
	assert.True(t, called, "first updater should still work")
	assert.False(t, secondCalled, "second updater should not have been called")
}

func TestUpdateDynamicTelemetry(t *testing.T) {
	// Reset global state before test
	defer func() {
		telemetryUpdaterMutex.Lock()
		dynamicTelemetryUpdater = nil
		lastTelemetryUpdate = time.Time{}
		cachedTelemetryData = struct {
			PlatformUID           string
			PlatformNodeCount     int
			PlatformVersion       string
			TridentProtectVersion string
		}{}
		telemetryUpdaterMutex.Unlock()
	}()

	ctx := context.Background()

	// Test 1: Update with nil telemetry (should return early)
	UpdateDynamicTelemetry(ctx, nil)
	// No assertions needed - should not panic

	// Test 2: Update with no registered updaters
	testTelemetry := &Telemetry{
		TridentVersion: "test-version",
		Platform:       "original-platform",
	}
	UpdateDynamicTelemetry(ctx, testTelemetry)
	assert.Equal(t, "original-platform", testTelemetry.Platform, "telemetry should not be modified")

	// Test 3: Update with registered updater
	updateCalled := false
	updater := func(ctx context.Context, telemetry *Telemetry) {
		updateCalled = true
		telemetry.Platform = "updated-platform"
		telemetry.PlatformVersion = "v1.0.0"
		telemetry.PlatformUID = "test-cluster-uid"
		telemetry.PlatformNodeCount = 5
		telemetry.TridentProtectVersion = "100.0.0"
	}

	// Reset registration state and register updater
	telemetryUpdaterMutex.Lock()
	dynamicTelemetryUpdater = nil
	telemetryUpdaterMutex.Unlock()

	RegisterTelemetryUpdater(updater)

	// Set last update time to past to ensure update happens
	telemetryUpdaterMutex.Lock()
	lastTelemetryUpdate = time.Now().Add(-5 * time.Hour)
	telemetryUpdaterMutex.Unlock()

	UpdateDynamicTelemetry(ctx, testTelemetry)

	assert.True(t, updateCalled, "updater should have been called")
	assert.Equal(t, "updated-platform", testTelemetry.Platform)
	assert.Equal(t, "v1.0.0", testTelemetry.PlatformVersion)
	assert.Equal(t, "test-cluster-uid", testTelemetry.PlatformUID)
	assert.Equal(t, 5, testTelemetry.PlatformNodeCount)
	assert.Equal(t, "100.0.0", testTelemetry.TridentProtectVersion)

	// Test 4: Update with recent last update (should use cached data)
	updateCalled = false
	testTelemetry2 := &Telemetry{Platform: "original-platform"}

	UpdateDynamicTelemetry(ctx, testTelemetry2)

	assert.False(t, updateCalled, "updater should not be called due to recent update")
	// Should get cached data from previous update
	assert.Equal(t, "test-cluster-uid", testTelemetry2.PlatformUID, "should use cached cluster UID")
	assert.Equal(t, 5, testTelemetry2.PlatformNodeCount, "should use cached node count")
	assert.Equal(t, "v1.0.0", testTelemetry2.PlatformVersion, "should use cached platform version")
	assert.Equal(t, "100.0.0", testTelemetry2.TridentProtectVersion, "should use cached protect version")

	// Test 5: Update with panicking updater (should recover)
	panicUpdater := func(ctx context.Context, telemetry *Telemetry) {
		panic("test panic")
	}

	// Reset and register panicking updater
	telemetryUpdaterMutex.Lock()
	dynamicTelemetryUpdater = panicUpdater
	lastTelemetryUpdate = time.Now().Add(-5 * time.Hour)
	telemetryUpdaterMutex.Unlock()

	testTelemetry3 := &Telemetry{}

	// Should not panic - recovery should handle it
	assert.NotPanics(t, func() {
		UpdateDynamicTelemetry(ctx, testTelemetry3)
	}, "UpdateDynamicTelemetry should recover from panicking updater")
}

func TestUpdateDynamicTelemetryTimeWindow(t *testing.T) {
	// Reset global state before test
	defer func() {
		telemetryUpdaterMutex.Lock()
		dynamicTelemetryUpdater = nil
		lastTelemetryUpdate = time.Time{}
		cachedTelemetryData = struct {
			PlatformUID           string
			PlatformNodeCount     int
			PlatformVersion       string
			TridentProtectVersion string
		}{}
		telemetryUpdaterMutex.Unlock()
	}()

	ctx := context.Background()

	updateCalled := false
	updater := func(ctx context.Context, telemetry *Telemetry) {
		updateCalled = true
		telemetry.Platform = "updated"
	}

	// Register updater
	telemetryUpdaterMutex.Lock()
	dynamicTelemetryUpdater = nil
	telemetryUpdaterMutex.Unlock()
	RegisterTelemetryUpdater(updater)

	testTelemetry := &Telemetry{}

	// Test 1: First update (no previous timestamp) - should update
	UpdateDynamicTelemetry(ctx, testTelemetry)
	assert.True(t, updateCalled, "first update should proceed")
	assert.Equal(t, "updated", testTelemetry.Platform)

	// Test 2: Immediate second update - cache not populated so treated as miss
	updateCalled = false
	testTelemetry2 := &Telemetry{}
	UpdateDynamicTelemetry(ctx, testTelemetry2)
	assert.True(t, updateCalled, "second update should proceed when cache not populated")
	assert.Equal(t, "updated", testTelemetry2.Platform, "should get fresh data when cache not populated")

	// Test 3: Update after interval has passed - should update
	telemetryUpdaterMutex.Lock()
	lastTelemetryUpdate = time.Now().Add(-5 * time.Hour) // Force past the 4-hour interval
	telemetryUpdaterMutex.Unlock()

	updateCalled = false
	testTelemetry3 := &Telemetry{}
	UpdateDynamicTelemetry(ctx, testTelemetry3)
	assert.True(t, updateCalled, "update after interval should proceed")
	assert.Equal(t, "updated", testTelemetry3.Platform)
}

func TestGetTagriTimeout(t *testing.T) {
	originalTimeout := TagriTimeout
	defer func() { TagriTimeout = originalTimeout }()

	tests := []struct {
		name       string
		setSeconds int // 0 = unset (use default), else set TagriTimeout to this many seconds
		expected   time.Duration
	}{
		{
			name:       "Default when unset (0)",
			setSeconds: 0,
			expected:   DefaultTagriTimeoutSeconds * time.Second,
		},
		{
			name:       "User value 30s",
			setSeconds: 30,
			expected:   30 * time.Second,
		},
		{
			name:       "User value 60s",
			setSeconds: 60,
			expected:   60 * time.Second,
		},
		{
			name:       "User value 120s",
			setSeconds: 120,
			expected:   120 * time.Second,
		},
		{
			name:       "User value 3600s",
			setSeconds: 3600,
			expected:   3600 * time.Second,
		},
		{
			name:       "Very large value 86400s",
			setSeconds: 86400,
			expected:   86400 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setSeconds == 0 {
				TagriTimeout = 0
			} else {
				TagriTimeout = time.Duration(tt.setSeconds) * time.Second
			}
			result := GetTagriTimeout()
			assert.Equal(t, tt.expected, result, "GetTagriTimeout() with setSeconds=%d", tt.setSeconds)
		})
	}
}
