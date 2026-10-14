/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package bucketaccess defines logic for BucketAccess resources needed by both Controller and
// Sidecar. BucketAccesses need to be handed off between the two, and some logic is shared.
package bucketaccess

import (
	"fmt"

	cosiapi "sigs.k8s.io/container-object-storage-interface/client/apis/objectstorage/v1alpha2"
)

// ManagedBySidecar returns true if a BucketAccess should be managed by the Sidecar.
// It returns false if it should be managed by the Controller instead.
//
// In order for COSI Controller and any given Sidecar to work well together, they should avoid
// managing the same BucketAccess resource at the same time. This will help prevent the Controller
// and Sidecar from racing with each other and causing update conflicts.
// Instances where a resource has no manager MUST be avoided without exception.
//
// Version skew between Controller and Sidecar should be assumed. In order for version skew issues
// to be minimized, avoid updating this logic unless it is absolutely critical. If updates are made,
// be sure to carefully consider all version skew cases below. Minimize dual-ownership scenarios,
// and avoid no-owner scenarios.
//
//  1. Sidecar version low, Controller version low
//  2. Sidecar version low, Controller version high
//  3. Sidecar version high, Controller version low
//  4. Sidecar version high, Controller version high
func ManagedBySidecar(ba *cosiapi.BucketAccess) bool {
	// Allow a future-compatible mechanism by which the Controller can override the normal
	// BucketAccess management handoff logic in order to resolve a bug.
	// Instances where this is utilized should be infrequent -- ideally, never used.
	if _, ok := ba.Annotations[cosiapi.ControllerManagementOverrideAnnotation]; ok {
		return false
	}

	// During provisioning, there are several status fields that the Controller needs to initialize
	// before the Sidecar can provision an access. However, tying this function's logic to ALL of
	// the status items could make long-term Controller-Sidecar handoff logic fragile. More logic
	// means more risk of unmanaged resources and more difficulty reasoning about how changes will
	// impact ownership during version skew. Minimize risk by relying on a single status field.
	if ba.Status.DriverName == "" {
		return false
	}

	// During deletion, as long as the access was handed off to the Sidecar at some point, the
	// Sidecar must first clean up the backend bucket, then hand back final deletion to the
	// Controller by setting an annotation.
	if !ba.DeletionTimestamp.IsZero() {
		_, ok := ba.Annotations[cosiapi.SidecarCleanupFinishedAnnotation]
		return !ok // ok means sidecar is done cleaning up
	}

	return true
}

// SidecarRequirementsPresent verifies that BucketAccess status information required by the Sidecar
// to provision the BucketAccess is fully set.
//
// Return true if the fields needed by the Sidecar are all set (Controller initialization finished).
// Return false if the fields needed by the Sidecar are all unset (needs Controller initialization).
// Return an error if required info is only partially set, indicating some sort of degradation/bug.
//
// Do not use this function to determine whether a BucketAccess should be managed by the Sidecar or
// Controller, or whether handoff has occurred. Use ManagedBySidecar() for that purpose instead.
// This function is appropriate for use within a controller to check requirements before/after
// initialization/provisioning.
func SidecarRequirementsPresent(s *cosiapi.BucketAccessStatus) (bool, error) {
	requiredFields := map[string]bool{}
	set := []string{}

	requiredFieldIsSet := func(fieldName string, isSet bool) {
		requiredFields[fieldName] = isSet
		if isSet {
			set = append(set, fieldName)
		}
	}

	requiredFieldIsSet("status.accessedBuckets", len(s.AccessedBuckets) > 0)
	requiredFieldIsSet("status.driverName", s.DriverName != "")
	requiredFieldIsSet("status.authenticationType", string(s.AuthenticationType) != "")

	if len(set) == 0 {
		return false, nil
	}

	if len(set) == len(requiredFields) {
		return true, nil
	}

	return false, fmt.Errorf("fields required for sidecar provisioning are only partially set: %v", requiredFields)
}
