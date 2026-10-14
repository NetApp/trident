// Copyright 2026 NetApp, Inc. All Rights Reserved.

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	k8sversion "k8s.io/apimachinery/pkg/version"

	versionutils "github.com/netapp/trident/utils/version"
)

// TestValidateKubernetesVersionForCOSI pins the floor that exists because the COSI CRDs use the
// CEL format library, which no API server before 1.32 provides. The boundary cases are the
// point: 1.31 must fail even though Trident itself supports it.
func TestValidateKubernetesVersionForCOSI(t *testing.T) {
	tests := []struct {
		version string
		wantErr bool
	}{
		{"v1.27.0", true}, // Trident's floor, too old for COSI
		{"v1.30.5", true},
		{"v1.31.9", true},  // last unsupported
		{"v1.32.0", false}, // first supported
		{"v1.33.1", false},
		{"v1.36.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			version, err := versionutils.ParseSemantic(tt.version)
			require.NoError(t, err)

			err = ValidateKubernetesVersionForCOSI(version)
			if tt.wantErr {
				require.Error(t, err)
				// The message has to name the floor; the whole reason this check exists is
				// that the API server's own error says nothing about versions.
				assert.Contains(t, err.Error(), "1.32")
				return
			}
			assert.NoError(t, err)
		})
	}
}

// TestCOSIFloorIsWithinTridentSupportedRange guards against the floor drifting out of the
// supported window. If Trident's minimum ever rises past the COSI floor, the gate is dead code
// and should be removed rather than left to mislead.
func TestCOSIFloorIsWithinTridentSupportedRange(t *testing.T) {
	cosiFloor := versionutils.MustParseMajorMinorVersion(KubernetesVersionMinCOSI)
	tridentMin := versionutils.MustParseMajorMinorVersion(KubernetesVersionMin)
	tridentMax := versionutils.MustParseMajorMinorVersion(KubernetesVersionMax)

	assert.True(t, cosiFloor.GreaterThan(tridentMin),
		"COSI floor %s no longer exceeds Trident's minimum %s; the gate is now unreachable",
		KubernetesVersionMinCOSI, KubernetesVersionMin)
	assert.False(t, cosiFloor.GreaterThan(tridentMax),
		"COSI floor %s is above Trident's maximum %s; COSI could never be enabled",
		KubernetesVersionMinCOSI, KubernetesVersionMax)
}

func TestValidateKubernetesVersionForCOSIFromInfo(t *testing.T) {
	assert.NoError(t, ValidateKubernetesVersionForCOSIFromInfo(&k8sversion.Info{GitVersion: "v1.33.1"}))

	err := ValidateKubernetesVersionForCOSIFromInfo(&k8sversion.Info{GitVersion: "v1.30.0"})
	assert.ErrorContains(t, err, "1.32")

	assert.Error(t, ValidateKubernetesVersionForCOSIFromInfo(&k8sversion.Info{GitVersion: "not-a-version"}))
}
