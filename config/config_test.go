// Copyright 2021 NetApp, Inc. All Rights Reserved.

package config

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

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
