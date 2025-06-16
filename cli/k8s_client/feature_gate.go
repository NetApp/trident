// Copyright 2025 NetApp, Inc. All Rights Reserved.

package k8sclient

import (
	"errors"
	"fmt"
	"strings"

	"github.com/netapp/trident/pkg/collection"
)

const (
	apiV1Alpha1 = "v1alpha1"
	apiV1Beta1  = "v1beta1"
	apiV1Beta2  = "v1beta2"
	apiV1       = "v1"

	// AutoFeatureGateVolumeGroupSnapshot is the feature gate template string for group snapshots.
	AutoFeatureGateVolumeGroupSnapshot = "VolumeGroupSnapshot"

	// volumeGroupSnapshotCRDName is a constant for the VolumeGroupSnapshot CRD name. This CRD may or may not be
	// installed. If it is present, Trident should enable the VolumeGroupSnapshot feature.
	volumeGroupSnapshotCRDName        = "volumegroupsnapshots.groupsnapshot.storage.k8s.io"
	volumeGroupSnapshotClassCRDName   = "volumegroupsnapshotclasses.groupsnapshot.storage.k8s.io"
	volumeGroupSnapshotContentCRDName = "volumegroupsnapshotcontents.groupsnapshot.storage.k8s.io"
)

var (
	// autoFeatureGates is a list of known feature gates that are automatically enabled.
	// These MUST be distinct from the typical feature gates that are set in the Trident configuration file.
	autoFeatureGates []autoFeatureGate

	// autoFeatureGateVolumeGroupSnapshot is the auto-enabling or (disabling) feature gate for VolumeGroupSnapshot.
	autoFeatureGateVolumeGroupSnapshot = autoFeatureGate{
		name: AutoFeatureGateVolumeGroupSnapshot,
		crds: []string{
			volumeGroupSnapshotCRDName,
			volumeGroupSnapshotClassCRDName,
			volumeGroupSnapshotContentCRDName,
		},
		supportedAPIs: []string{
			apiV1Beta1,
			apiV1Beta2,
			apiV1,
		},
		gates: map[string]string{
			// The placeholder key is used to inject the feature gate into the Trident configuration file.
			// It MUST match the placeholder key in the YAML.
			// Placeholders can be duplicated across multiple feature gates, but the values must be unique.
			"{FEATURE_GATES_CSI_SNAPSHOTTER}": "CSIVolumeGroupSnapshot=true",
		},
	}
)

func init() {
	autoFeatureGates = []autoFeatureGate{
		autoFeatureGateVolumeGroupSnapshot,
	}
}

// autoFeatureGate represents the link between a feature, its requirements and YAML placeholders to gates
// that may be replaced in the Trident YAML files.
type autoFeatureGate struct {
	name          string            // name of the feature that's being gated.
	crds          []string          // CRDs that must be present for the feature to be enabled. If no CRDs are required, leave empty.
	supportedAPIs []string          // API versions that must be present. If no supportedAPIs are required, leave empty.
	gates         map[string]string // YAML gates that may be replaced in the Trident configuration file. Always required.
}

func (g autoFeatureGate) GoString() string {
	return fmt.Sprintf("name: %s, requiredCRDs: %v, supportedAPIs: %v", g.name, g.crds, g.supportedAPIs)
}

func (g autoFeatureGate) String() string {
	return fmt.Sprintf("name: %s, requiredCRDs: %v, supportedAPIs: %v", g.name, g.crds, g.supportedAPIs)
}

func (g autoFeatureGate) Gates() map[string]string {
	snippets := make(map[string]string, len(g.gates))
	for k, v := range g.gates {
		snippets[k] = v
	}
	return snippets
}

// ConstructCSIFeatureGateYAMLSnippets looks at all predefined feature gates and builds a list of
// YAML snippets for the Trident Deployment and DaemonSet.
// If any of the feature gates are not safe to enable, it will gather errors and log a warning.
// Importantly, this is not an all or nothing operation.
// If some but not all feature gates are safe to enable, they will be returned along with an error.
func ConstructCSIFeatureGateYAMLSnippets(client KubernetesClient) (map[string]string, error) {
	if client == nil {
		return nil, fmt.Errorf("k8s API client is nil; cannot auto-enable feature gates")
	}

	// Automatically enable feature gates.
	// A given placeholder key can have 1-many feature gates associated with it.
	// Example:
	//  feature gate arg: `- "--feature-gates=CSIVolumeGroupSnapshot=true,CSIAnotherFeature=true"`
	// This means that the `CSIVolumeGroupSnapshot` and `CSIAnotherFeature` feature gates
	// are both enabled, and the `--feature-gates` argument will be both be injected and must be squashed.
	// If there are multiple feature gates that need to be injected, they will be squashed into a single
	// `--feature-gates` argument.
	var errs error
	placeholderToGates := make(map[string]map[string]struct{}) // snippets to inject for feature gates
	for _, autoFeatureGate := range autoFeatureGates {
		canEnable, err := canAutoEnableFeatureGate(client, autoFeatureGate)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		} else if !canEnable {
			errs = errors.Join(errs, fmt.Errorf("feature gate '%s' cannot be enabled", autoFeatureGate.name))
			continue
		}

		for placeholder, featureGate := range autoFeatureGate.Gates() {
			gates, exists := placeholderToGates[placeholder]
			if !exists {
				gates = make(map[string]struct{})
			}
			// Some feature gates may already be specified as a comma-separated list, so we need to split them
			// and join them later with others that share the same placeholder.
			for _, gate := range strings.Split(featureGate, ",") {
				gate = strings.TrimSpace(gate)
				if gate != "" {
					gates[gate] = struct{}{}
				}
			}
			placeholderToGates[placeholder] = gates
		}
	}

	// Now we have a map of placeholders to feature gates. We need to build the snippets. Feature gates sharing the
	// same placeholder will be squashed into a single string delimited by commas.
	snippets := make(map[string]string, len(placeholderToGates))
	for placeholder, gates := range placeholderToGates {
		if len(gates) == 0 {
			continue // No gates for this placeholder, skip it.
		}

		// Join the gates with commas and add to the snippets map.
		gateList := make([]string, 0, len(gates))
		for gate := range gates {
			gateList = append(gateList, gate)
		}
		snippets[placeholder] = strings.Join(gateList, ",")
	}

	return snippets, errs
}

// canAutoEnableFeatureGate checks if all autoFeatureGate requirements are met, such as the presence of
// CRDs and correct API versions. If any precondition is not met, it returns an error indicating which
// requirements are not satisfied and does not allow Trident to automatically enable the feature gate.
func canAutoEnableFeatureGate(client KubernetesClient, gate autoFeatureGate) (bool, error) {
	var errs error
	for _, crdName := range gate.crds {
		crdExists, err := client.CheckCRDExists(crdName)
		if err != nil {
			// This will only fail if the error is not a "not found" error. Not found is OK
			errs = errors.Join(errs, err)
			continue
		} else if !crdExists {
			// If we get here, the CRD is missing.
			err = fmt.Errorf("CRD '%s' is missing for feature '%s'", crdName, gate.name)
			errs = errors.Join(errs, err)
			continue
		}

		// Now we know this CRD exists. Validate the API versions.
		crd, err := client.GetCRD(crdName)
		if err != nil || crd == nil {
			err = fmt.Errorf("failed to get CRD '%s' for feature '%s'; %w", crdName, gate.name, err)
			errs = errors.Join(errs, err)
			continue
		}

		// Check if the discovered CRD version is one of the allowed versions.
		// If the CRD API Version is not at least one of the supported versions, we cannot enable the feature.
		if len(gate.supportedAPIs) > 0 {
			// We need to check both Spec.Versions and Status.StoredVersions.
			found := false
			for _, version := range crd.Spec.Versions {
				if collection.ContainsString(gate.supportedAPIs, version.Name) {
					found = true
					break
				}
			}

			if !found {
				// If we still do not have a matching version, we cannot enable the feature.
				err = fmt.Errorf("CRD '%s' does not have a supported API versions for feature '%s'", crdName, gate.name)
				errs = errors.Join(errs, err)
			}
		}
	}

	// If any CRDs are missing, do not attempt to enable the feature.
	if errs != nil {
		return false, errs
	}

	return true, nil
}
