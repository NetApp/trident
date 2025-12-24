// Copyright 2025 NetApp, Inc. All Rights Reserved.

package configurator

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RemoveFinalizerFromObjectMeta removes a specific finalizer from an object's metadata.
// Returns true if the finalizer was present and removed, false if it wasn't present.
func RemoveFinalizerFromObjectMeta(objMeta *metav1.ObjectMeta, finalizer string) bool {
	if objMeta == nil || len(objMeta.Finalizers) == 0 {
		return false
	}

	found := false
	finalizers := make([]string, 0, len(objMeta.Finalizers))
	for _, f := range objMeta.Finalizers {
		if f != finalizer {
			finalizers = append(finalizers, f)
		} else {
			found = true
		}
	}

	if found {
		objMeta.Finalizers = finalizers
	}

	return found
}

// AddFinalizerToObjectMeta adds a finalizer to an object's metadata if it's not already present.
// Returns true if the finalizer was added, false if it was already present.
func AddFinalizerToObjectMeta(objMeta *metav1.ObjectMeta, finalizer string) bool {
	if objMeta == nil {
		return false
	}

	// Check if finalizer already exists
	for _, f := range objMeta.Finalizers {
		if f == finalizer {
			return false
		}
	}

	// Add finalizer
	objMeta.Finalizers = append(objMeta.Finalizers, finalizer)
	return true
}

// HasFinalizer checks if an object's metadata contains a specific finalizer.
func HasFinalizer(objMeta *metav1.ObjectMeta, finalizer string) bool {
	if objMeta == nil {
		return false
	}

	for _, f := range objMeta.Finalizers {
		if f == finalizer {
			return true
		}
	}

	return false
}
